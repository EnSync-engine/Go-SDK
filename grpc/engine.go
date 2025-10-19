package grpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/EnSync-engine/Go-SDK/common"
	pb "github.com/EnSync-engine/Go-SDK/internal/proto"
)

const (
	keepaliveTime         = 60 * time.Second
	keepaliveTimeout      = 20 * time.Second
	keepalivePermit       = false
	defaultMaxMessageSize = 1024 * 1024
	defaultMaxRecvSize    = 2 * 1024 * 1024
)

type GRPCEngine struct {
	*common.BaseEngine
	client          pb.EnSyncServiceClient
	conn            *grpc.ClientConn
	cancel          context.CancelFunc
	closed          atomic.Bool
	closeMu         sync.Mutex
	subscriptionMgr *common.SubscriptionManager
}

type grpcSubscription struct {
	*common.BaseSubscription
	stream       pb.EnSyncService_SubscribeClient
	autoAck      bool
	appSecretKey string
	engine       *GRPCEngine
	cancel       context.CancelFunc
	closed       atomic.Bool
}

func NewGRPCEngine(
	ctx context.Context,
	endpoint string,
	opts ...common.Option,
) (*GRPCEngine, error) {
	if ctx == nil {
		return nil, common.NewEnSyncError("context cannot be nil", common.ErrTypeValidation, nil)
	}

	host, secure, err := parseGRPCURL(endpoint)
	if err != nil {
		return nil, err
	}

	serverName := extractServerName(host)

	engineCtx, cancel := context.WithCancel(ctx)

	baseEngine, err := common.NewBaseEngine(engineCtx, opts...)
	if err != nil {
		cancel()
		return nil, common.NewEnSyncError("failed to create base engine", common.ErrTypeConnection, err)
	}

	var creds credentials.TransportCredentials
	if secure {
		creds = credentials.NewTLS(&tls.Config{
			ServerName: serverName,
			MinVersion: tls.VersionTLS12,
		})
	} else {
		creds = insecure.NewCredentials()
	}

	grpcOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(defaultMaxRecvSize),
			grpc.MaxCallSendMsgSize(defaultMaxMessageSize),
		),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                keepaliveTime,
			Timeout:             keepaliveTimeout,
			PermitWithoutStream: keepalivePermit,
		}),
	}

	conn, err := grpc.NewClient(host, grpcOpts...)
	if err != nil {
		cancel()
		return nil, common.NewEnSyncError("failed to connect to gRPC server", common.ErrTypeConnection, err)
	}

	engine := &GRPCEngine{
		BaseEngine:      baseEngine,
		client:          pb.NewEnSyncServiceClient(conn),
		conn:            conn,
		cancel:          cancel,
		subscriptionMgr: nil,
	}

	return engine, nil
}

func (e *GRPCEngine) CreateClient(accessKey string, options ...common.ClientOption) error {
	if accessKey == "" {
		return common.NewEnSyncError("access key cannot be empty", common.ErrTypeValidation, nil)
	}

	e.AccessKey = accessKey

	config := &common.ClientConfig{}
	for _, opt := range options {
		opt(config)
	}

	if err := e.authenticate(); err != nil {
		return err
	}
	return nil
}

func (e *GRPCEngine) authenticate() error {
	return e.ExecuteOperation(common.Operation{
		Name: "authenticate",
		Execute: func(ctx context.Context) error {
			e.Logger.Info("sending authentication request")

			resp, err := e.client.Connect(ctx, &pb.ConnectRequest{
				AccessKey: e.AccessKey,
			})
			if err != nil {
				return common.NewEnSyncError("connect request failed", common.ErrTypeConnection, err)
			}

			if !resp.Success {
				return common.NewEnSyncError("authentication rejected: "+resp.ErrorMessage, common.ErrTypeAuth, nil)
			}

			e.setAuthenticationResult(resp.ClientId, resp.ClientHash)
			return nil
		},
	})
}

func (e *GRPCEngine) setAuthenticationResult(clientID, clientHash string) {
	e.Logger.Info("authentication successful",
		zap.String("clientId", clientID))
	e.ClientID = clientID
	e.ClientHash = clientHash

	e.State.Mu.Lock()
	e.State.IsAuthenticated = true
	e.State.IsConnected = true
	e.State.Mu.Unlock()

	go e.startHeartbeat()
}

func (e *GRPCEngine) startHeartbeat() {
	ticker := time.NewTicker(e.GetPingIntervalTimeout())
	defer ticker.Stop()

	for {
		select {
		case <-e.Ctx.Done():
			e.Logger.Info("heartbeat stopped due to context cancellation")
			return
		case <-ticker.C:
			if err := e.sendHeartbeat(); err != nil {
				e.Logger.Error("heartbeat failed", zap.Error(err))
				e.State.Mu.Lock()
				e.State.IsConnected = false
				e.State.Mu.Unlock()
			}
		}
	}
}

func (e *GRPCEngine) sendHeartbeat() error {
	if !e.State.IsAuthenticated {
		return nil
	}

	return e.ExecuteOperation(common.Operation{
		Name: "heartbeat",
		Execute: func(ctx context.Context) error {
			resp, err := e.client.Heartbeat(ctx, &pb.HeartbeatRequest{
				ClientId: e.ClientID,
			})
			if err != nil {
				return common.NewEnSyncError("heartbeat request failed", common.ErrTypeConnection, err)
			}
			if !resp.Success {
				return common.NewEnSyncError("heartbeat rejected", common.ErrTypeConnection, nil)
			}
			return nil
		},
	})
}

func (e *GRPCEngine) Publish(
	eventName string,
	recipients []string,
	payload map[string]interface{},
	metadata *common.EventMetadata,
	options *common.PublishOptions,
) (string, error) {
	if err := e.ValidatePublishInput(eventName, recipients); err != nil {
		return "", err
	}

	if metadata == nil {
		metadata = &common.EventMetadata{
			Persist: true,
			Headers: make(map[string]string),
		}
	}

	useHybridEncryption := true
	if options != nil {
		useHybridEncryption = options.UseHybridEncryption
	}

	publishData, err := e.PreparePublishData(payload, metadata)
	if err != nil {
		return "", err
	}

	return e.executePublish(
		eventName, recipients,
		publishData.PayloadJSON, publishData.MetadataJSON, publishData.PayloadMetaJSON,
		useHybridEncryption,
	)
}

func (e *GRPCEngine) executePublish(
	eventName string,
	recipients []string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	useHybridEncryption bool,
) (string, error) {
	var responses []string

	if useHybridEncryption && len(recipients) > 1 {
		err := e.publishHybrid(eventName, recipients, payloadJSON, metadataJSON, payloadMetaJSON, &responses)
		if err != nil {
			return "", common.NewEnSyncError("publish failed after retries", common.ErrTypePublish, err)
		}
	} else {
		err := e.publishIndividual(eventName, recipients, payloadJSON, metadataJSON, payloadMetaJSON, &responses)
		if err != nil {
			return "", common.NewEnSyncError("publish failed after retries", common.ErrTypePublish, err)
		}
	}

	return strings.Join(responses, ","), nil
}

func (e *GRPCEngine) publishHybrid(
	eventName string,
	recipients []string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	responses *[]string,
) error {
	hybridMsg, err := common.HybridEncrypt(string(payloadJSON), recipients)
	if err != nil {
		return common.NewEnSyncError("hybrid encryption failed", common.ErrTypePublish, err)
	}

	hybridJSON, err := json.Marshal(hybridMsg)
	if err != nil {
		return common.NewEnSyncError("failed to marshal hybrid message", common.ErrTypePublish, err)
	}

	encryptedBase64 := base64.StdEncoding.EncodeToString(hybridJSON)

	var eg errgroup.Group
	resultsChan := make(chan string, len(recipients))

	for _, recipient := range recipients {
		eg.Go(func() error {
			resp, err := e.sendPublishRequest(eventName, recipient, encryptedBase64, string(metadataJSON), string(payloadMetaJSON))
			if err != nil {
				return err
			}
			resultsChan <- resp
			return nil
		})
	}

	err = eg.Wait()
	close(resultsChan)

	if err != nil {
		return err
	}

	for res := range resultsChan {
		*responses = append(*responses, res)
	}
	return nil
}

func (e *GRPCEngine) publishIndividual(
	eventName string,
	recipients []string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	responses *[]string,
) error {
	metadataStr := string(metadataJSON)
	payloadMetaStr := string(payloadMetaJSON)

	var eg errgroup.Group
	resultsChan := make(chan string, len(recipients))

	for _, recipient := range recipients {
		eg.Go(func() error {
			recipientPubKey, decErr := base64.StdEncoding.DecodeString(recipient)
			if decErr != nil {
				return common.NewEnSyncError("invalid recipient public key format", common.ErrTypePublish, decErr)
			}

			encrypted, encErr := common.EncryptEd25519(string(payloadJSON), recipientPubKey)
			if encErr != nil {
				return common.NewEnSyncError("encryption failed", common.ErrTypePublish, encErr)
			}

			encryptedJSON, encErr := json.Marshal(encrypted)
			if encErr != nil {
				return common.NewEnSyncError("failed to marshal encrypted message", common.ErrTypePublish, encErr)
			}

			encryptedBase64 := base64.StdEncoding.EncodeToString(encryptedJSON)
			resp, err := e.sendPublishRequest(eventName, recipient, encryptedBase64, metadataStr, payloadMetaStr)
			if err != nil {
				return err
			}
			resultsChan <- resp
			return nil
		})
	}

	err := eg.Wait()
	close(resultsChan)

	if err != nil {
		return err
	}

	for res := range resultsChan {
		*responses = append(*responses, res)
	}
	return nil
}

func (e *GRPCEngine) sendPublishRequest(
	eventName, recipient, payload, metadata, payloadMeta string,
) (string, error) {
	var eventIdem string
	err := e.ExecuteOperation(common.Operation{
		Name: "publish",
		Execute: func(ctx context.Context) error {
			req := &pb.PublishEventRequest{
				ClientId:  e.ClientID,
				EventName: eventName,
				Payload:   payload,
			}

			if recipient != "" {
				req.DeliveryTo = recipient
			}
			if metadata != "" {
				req.Metadata = metadata
			}
			if payloadMeta != "" {
				req.PayloadMetadata = &payloadMeta
			}

			resp, err := e.client.PublishEvent(ctx, req)
			if err != nil {
				return common.NewEnSyncError("publish request failed", common.ErrTypePublish, err)
			}

			if !resp.Success {
				return common.NewEnSyncError("publish rejected: "+resp.ErrorMessage, common.ErrTypePublish, nil)
			}

			eventIdem = resp.EventIdem
			return nil
		},
	})
	return eventIdem, err
}

func (e *GRPCEngine) Subscribe(eventName string, options *common.SubscribeOptions) (common.Subscription, error) {
	if err := e.ValidateSubscribeInput(eventName); err != nil {
		return nil, err
	}

	if e.subscriptionMgr == nil {
		e.subscriptionMgr = common.NewSubscriptionManager(e.Logger)
	}

	if options == nil {
		options = &common.SubscribeOptions{
			AutoAck: true,
		}
	}

	var sub *grpcSubscription

	err := e.ExecuteOperation(common.Operation{
		Name: "subscribe",
		Execute: func(ctx context.Context) error {
			subCtx, cancel := context.WithCancel(e.Ctx)

			stream, err := e.client.Subscribe(subCtx, &pb.SubscribeRequest{
				ClientId:  e.ClientID,
				EventName: eventName,
			})

			if err != nil {
				cancel()
				return common.NewEnSyncError("subscription stream error", common.ErrTypeSubscription, err)
			}

			sub = &grpcSubscription{
				BaseSubscription: common.NewBaseSubscription(eventName, e.Logger),
				stream:           stream,
				autoAck:          options.AutoAck,
				appSecretKey:     options.AppSecretKey,
				engine:           e,
				cancel:           cancel,
			}

			if err := e.subscriptionMgr.Register(eventName, sub); err != nil {
				cancel()
				return err
			}

			return nil
		},
	})

	if err != nil {
		return nil, common.NewEnSyncError("subscription failed after retries", common.ErrTypeSubscription, err)
	}

	if err := sub.StartWorkerPool(); err != nil {
		e.subscriptionMgr.Unregister(eventName) //nolint
		return nil, err
	}

	go sub.listen()
	e.Logger.Info("successfully subscribed",
		"eventName", eventName)

	return sub, nil
}

func (s *grpcSubscription) Ack(eventID string, block int64) error {
	return s.engine.ExecuteOperation(common.Operation{
		Name: "ack",
		Execute: func(ctx context.Context) error {
			resp, err := s.engine.client.AcknowledgeEvent(ctx, &pb.AcknowledgeRequest{
				ClientId:       s.engine.ClientID,
				EventIdem:      eventID,
				PartitionBlock: block,
			})
			if err != nil {
				return common.NewEnSyncError("ack request failed", common.ErrTypeSubscription, err)
			}
			if !resp.Success {
				return common.NewEnSyncError("ack rejected: "+resp.Message, common.ErrTypeSubscription, nil)
			}
			return nil
		},
	})
}

func (s *grpcSubscription) Defer(eventIdem string, delayMs int64, reason string) (*common.DeferResponse, error) {
	var resp *common.DeferResponse

	err := s.engine.ExecuteOperation(common.Operation{
		Name: "defer",
		Execute: func(ctx context.Context) error {
			deferResp, err := s.engine.client.DeferEvent(ctx, &pb.DeferRequest{
				ClientId:  s.engine.ClientID,
				EventIdem: eventIdem,
				EventName: s.EventName,
				DelayMs:   delayMs,
				Reason:    reason,
			})
			if err != nil {
				return common.NewEnSyncError("defer request failed", common.ErrTypeDefer, err)
			}
			if !deferResp.Success {
				return common.NewEnSyncError("defer rejected: "+deferResp.Message, common.ErrTypeDefer, nil)
			}
			resp = &common.DeferResponse{
				EventID:           eventIdem,
				Status:            "deferred",
				DelayMs:           delayMs,
				ScheduledDelivery: time.Now().Add(time.Duration(delayMs) * time.Millisecond),
				Timestamp:         time.Now(),
			}
			return nil
		},
	})
	return resp, err
}

func (s *grpcSubscription) Discard(eventIdem, reason string) (*common.DiscardResponse, error) {
	var resp *common.DiscardResponse

	err := s.engine.ExecuteOperation(common.Operation{
		Name: "discard",
		Execute: func(ctx context.Context) error {
			discardResp, err := s.engine.client.DiscardEvent(ctx, &pb.DiscardRequest{
				ClientId:  s.engine.ClientID,
				EventIdem: eventIdem,
				Reason:    reason,
			})
			if err != nil {
				return common.NewEnSyncError("discard request failed", common.ErrTypeDiscard, err)
			}
			if !discardResp.Success {
				return common.NewEnSyncError("discard rejected: "+discardResp.Message, common.ErrTypeDiscard, nil)
			}
			resp = &common.DiscardResponse{
				EventID:   eventIdem,
				Status:    "success",
				Action:    "discarded",
				Timestamp: time.Now(),
			}
			return nil
		},
	})
	return resp, err
}

func (s *grpcSubscription) Pause(reason string) error {
	return s.engine.ExecuteOperation(common.Operation{
		Name: "pause",
		Execute: func(ctx context.Context) error {
			resp, err := s.engine.client.PauseEvents(ctx, &pb.PauseRequest{
				ClientId:  s.engine.ClientID,
				EventName: s.EventName,
				Reason:    reason,
			})
			if err != nil {
				return common.NewEnSyncError("pause request failed", common.ErrTypePause, err)
			}
			if !resp.Success {
				return common.NewEnSyncError("pause rejected: "+resp.Message, common.ErrTypePause, nil)
			}
			return nil
		},
	})
}

func (s *grpcSubscription) Resume() error {
	return s.engine.ExecuteOperation(common.Operation{
		Name: "resume",
		Execute: func(ctx context.Context) error {
			resp, err := s.engine.client.ContinueEvents(ctx, &pb.ContinueRequest{
				ClientId:  s.engine.ClientID,
				EventName: s.EventName,
			})
			if err != nil {
				return common.NewEnSyncError("resume request failed", common.ErrTypeResume, err)
			}
			if !resp.Success {
				return common.NewEnSyncError("resume rejected: "+resp.Message, common.ErrTypeResume, nil)
			}
			return nil
		},
	})
}

func (s *grpcSubscription) Replay(eventIdem string) (*common.EventPayload, error) {
	var result *common.EventPayload

	err := s.engine.ExecuteOperation(common.Operation{
		Name: "replay",
		Execute: func(ctx context.Context) error {
			resp, replayErr := s.engine.client.ReplayEvent(ctx, &pb.ReplayRequest{
				ClientId:  s.engine.ClientID,
				EventName: s.EventName,
				EventIdem: eventIdem,
			})

			if replayErr != nil {
				return common.NewEnSyncError("replay request failed", common.ErrTypeReplay, replayErr)
			}

			if !resp.Success {
				return common.NewEnSyncError("replay rejected: "+resp.Message, common.ErrTypeReplay, nil)
			}

			var eventData struct {
				EventIdem      string                 `json:"idem"`
				EventName      string                 `json:"name"`
				Payload        string                 `json:"payload"`
				PartitionBlock string                 `json:"block"`
				Metadata       map[string]interface{} `json:"metadata"`
				Sender         string                 `json:"sender"`
			}

			if err := json.Unmarshal([]byte(resp.EventData), &eventData); err != nil {
				return common.NewEnSyncError("failed to parse event stream response", common.ErrTypeReplay, err)
			}

			partitionBlock, err := strconv.ParseInt(eventData.PartitionBlock, 10, 64)
			if err != nil {
				return common.NewEnSyncError("failed to parse partition block", common.ErrTypeReplay, err)
			}

			processed, processErr := s.decryptPayload(
				eventData.EventIdem,
				eventData.EventName,
				eventData.Payload,
				eventData.Metadata,
				partitionBlock,
				eventData.Sender,
			)
			if processErr != nil {
				return processErr
			}

			result = processed
			return nil
		},
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *grpcSubscription) Rollback(eventIdem string, block int64) error {
	return common.NewEnSyncError("rollback not implemented in gRPC version", common.ErrTypeGeneric, nil)
}

func (s *grpcSubscription) Unsubscribe() error {
	ctx, cancel := context.WithTimeout(s.engine.Ctx, s.engine.GetGracefulShutdownTimeout())
	defer cancel()
	return s.unsubscribeWithContext(ctx)
}

func (s *grpcSubscription) unsubscribeWithContext(ctx context.Context) error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	return s.engine.ExecuteOperation(common.Operation{
		Name: "unsubscribe",
		Execute: func(opCtx context.Context) error {
			resp, err := s.engine.client.Unsubscribe(ctx, &pb.UnsubscribeRequest{
				ClientId:  s.engine.ClientID,
				EventName: s.EventName,
			})

			if err != nil {
				return common.NewEnSyncError("unsubscribe request failed", common.ErrTypeSubscription, err)
			}

			if !resp.Success {
				return common.NewEnSyncError("unsubscribe rejected: "+resp.Message, common.ErrTypeSubscription, nil)
			}

			s.cancel()
			s.engine.subscriptionMgr.Unregister(s.EventName) //nolint:errcheck
			s.StopWorkerPool()
			s.Mu.Lock()
			s.Handlers = nil
			s.Mu.Unlock()

			s.engine.Logger.Info("successfully unsubscribed",
				zap.String("eventName", s.EventName))

			return nil
		},
	})
}

func (s *grpcSubscription) listen() {
	defer func() {
		if r := recover(); r != nil {
			s.engine.Logger.Error("subscription listener panic",
				zap.String("eventName", s.EventName),
				zap.Any("panic", r))
		}
	}()

	s.engine.Logger.Debug("subscription listener started",
		zap.String("eventName", s.EventName))

	for {
		select {
		case <-s.engine.Ctx.Done():
			s.engine.Logger.Info("subscription listener stopping due to engine context cancellation",
				zap.String("eventName", s.EventName))
			return

		default:
			event, err := s.stream.Recv()
			if err != nil {
				s.engine.Logger.Warn("stream receive error",
					zap.String("eventName", s.EventName),
					zap.Error(err))
				s.handleStreamError(err)
				return
			}
			s.handleEvent(event)
		}
	}
}
func (s *grpcSubscription) handleStreamError(err error) {
	if st, ok := status.FromError(err); ok {
		s.engine.Logger.Info("stream error details",
			zap.String("eventName", s.EventName),
			zap.String("grpcCode", st.Code().String()),
			zap.String("grpcMessage", st.Message()))

		if st.Code() == codes.Canceled {
			s.engine.Logger.Info("stream canceled",
				zap.String("eventName", s.EventName),
				zap.String("reason", st.Message()))
			return
		}
	}

	s.engine.Logger.Warn("stream error (non-gRPC or unknown)",
		zap.String("eventName", s.EventName),
		zap.Error(err))

	s.engine.State.Mu.Lock()
	s.engine.State.IsConnected = false
	s.engine.State.Mu.Unlock()
}

func (s *grpcSubscription) handleEvent(event *pb.EventStreamResponse) {
	metadata := make(map[string]interface{})
	if event.Metadata != "" {
		if err := json.Unmarshal([]byte(event.Metadata), &metadata); err != nil {
			s.engine.Logger.Warn("failed to unmarshal metadata", zap.Error(err))
		}
	}

	processedEvent, err := s.decryptPayload(
		event.EventIdem,
		event.EventName,
		event.Payload,
		metadata,
		event.PartitionBlock,
		event.Sender,
	)
	if err != nil {
		s.engine.Logger.Error("event processing failed",
			zap.String("eventName", s.EventName),
			zap.String("eventId", event.EventIdem),
			zap.Error(err))
		return
	}
	if processedEvent == nil {
		return
	}

	s.CallHandlers(processedEvent)
	if s.autoAck {
		if err := s.Ack(processedEvent.Idem, processedEvent.Block); err != nil {
			s.engine.Logger.Error("auto-ack failed",
				zap.String("eventId", processedEvent.Idem),
				zap.Error(err))
		}
	}
}

func (s *grpcSubscription) decryptPayload(
	eventIdem, eventName, payload string,
	metadata map[string]interface{},
	partitionBlock int64,
	sender string,
) (*common.EventPayload, error) {
	decryptedPayload, err := s.engine.DecryptEventPayload(payload, s.appSecretKey)
	if err != nil {
		return nil, err
	}

	metadata = s.engine.ParseMetadata(metadata)

	return &common.EventPayload{
		EventName: eventName,
		Idem:      eventIdem,
		Block:     partitionBlock,
		Timestamp: time.Now(),
		Payload:   decryptedPayload,
		Metadata:  metadata,
		Sender:    sender,
	}, nil
}

func (e *GRPCEngine) Close() error {
	e.closeMu.Lock()
	defer e.closeMu.Unlock()

	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}

	e.Logger.Info("shutting down gRPC engine")
	e.cancel()

	e.State.Mu.Lock()
	e.State.IsAuthenticated = false
	e.State.IsConnected = false
	e.State.Mu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), e.GetGracefulShutdownTimeout())
	defer cancel()

	if e.subscriptionMgr != nil {
		if err := e.subscriptionMgr.CloseAll(shutdownCtx); err != nil {
			e.Logger.Warn("error closing subscriptions",
				"error", err)
		}
	}

	if e.conn != nil {
		if err := e.conn.Close(); err != nil {
			e.Logger.Error("error closing gRPC connection",
				"error", err)
			return common.NewEnSyncError("failed to close gRPC connection", common.ErrTypeConnection, err)
		}
	}

	e.Logger.Info("gRPC engine shutdown complete")
	return nil
}
