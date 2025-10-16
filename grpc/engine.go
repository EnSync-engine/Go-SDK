package grpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/EnSync-engine/Go-SDK/common"
	pb "github.com/EnSync-engine/Go-SDK/internal/proto"
)

const (
	defaultConnectionTimeout = 10 * time.Second
	defaultOperationTimeout  = 5 * time.Second
	heartbeatInterval        = 30 * time.Second
	shutdownTimeout          = 5 * time.Second
	cleanupDelay             = 100 * time.Millisecond

	defaultMaxMessageSize = 1024 * 1024
	defaultMaxRecvSize    = 2 * 1024 * 1024
	maxSafeMessageSize    = 10 * 1024 * 1024
)

type GRPCEngine struct {
	*common.BaseEngine
	client pb.EnSyncServiceClient
	conn   *grpc.ClientConn
}

type baseSubscription struct {
	eventName string
	handlers  []common.EventHandler
	mu        sync.RWMutex
}

type grpcSubscription struct {
	baseSubscription
	stream       pb.EnSyncService_SubscribeClient
	autoAck      bool
	appSecretKey string
	engine       *GRPCEngine
	cancel       context.CancelFunc
}

type publishData struct {
	PayloadJSON     []byte
	MetadataJSON    []byte
	PayloadMetaJSON []byte
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

	baseEngine, err := common.NewBaseEngine(ctx, opts...)
	if err != nil {
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
	}

	conn, err := grpc.NewClient(host, grpcOpts...)
	if err != nil {
		return nil, common.NewEnSyncError("failed to connect to gRPC server", common.ErrTypeConnection, err)
	}

	engine := &GRPCEngine{
		BaseEngine: baseEngine,
		client:     pb.NewEnSyncServiceClient(conn),
		conn:       conn,
	}

	return engine, nil
}

func (e *GRPCEngine) CreateClient(accessKey string, options ...common.ClientOption) error {
	e.AccessKey = accessKey

	config := &common.ClientConfig{}
	for _, opt := range options {
		opt(config)
	}

	if config.ClientID != "" {
		e.ClientID = config.ClientID
	}

	if err := e.authenticate(); err != nil {
		return err
	}
	return nil
}

func (e *GRPCEngine) authenticate() error {
	return e.WithRetry(e.Ctx, func() error {
		e.Logger.Info("Sending authentication request")

		ctx, cancel := context.WithTimeout(e.Ctx, defaultConnectionTimeout)
		defer cancel()

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
	})
}

func (e *GRPCEngine) setAuthenticationResult(clientID, clientHash string) {
	e.Logger.Info("Authentication successful")
	e.ClientID = clientID
	e.ClientHash = clientHash

	e.State.Mu.Lock()
	e.State.IsAuthenticated = true
	e.State.IsConnected = true
	e.State.Mu.Unlock()

	go e.startHeartbeat()
}

func (e *GRPCEngine) startHeartbeat() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.Ctx.Done():
			e.Logger.Info("Heartbeat stopped due to context cancellation")
			return
		case <-ticker.C:
			if err := e.sendHeartbeat(); err != nil {
				e.Logger.Error("Heartbeat failed", zap.Error(err))
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

	return e.WithRetry(e.Ctx, func() error {
		ctx, cancel := context.WithTimeout(e.Ctx, defaultOperationTimeout)
		defer cancel()

		resp, err := e.client.Heartbeat(ctx, &pb.HeartbeatRequest{
			ClientId: e.ClientID,
		})
		if err != nil {
			return common.NewEnSyncError("heartbeat request failed", common.ErrTypeConnection, err)
		}
		if !resp.Success {
			return common.NewEnSyncError("heartbeat rejected: "+resp.String(), common.ErrTypeConnection, nil)
		}
		return nil
	})
}

func (e *GRPCEngine) Publish(
	eventName string,
	recipients []string,
	payload map[string]interface{},
	metadata *common.EventMetadata,
	options *common.PublishOptions,
) (string, error) {
	e.State.Mu.RLock()
	isAuth := e.State.IsAuthenticated
	e.State.Mu.RUnlock()

	if !isAuth {
		return "", fmt.Errorf("client not authenticated")
	}

	if len(recipients) == 0 {
		return "", fmt.Errorf("recipients required")
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

	// Prepare data
	publishData, err := e.preparePublishData(payload, metadata)
	if err != nil {
		return "", err
	}

	// Execute publish
	return e.executePublish(
		eventName, recipients,
		publishData.PayloadJSON, publishData.MetadataJSON, publishData.PayloadMetaJSON,
		useHybridEncryption,
	)
}

func (e *GRPCEngine) preparePublishData(
	payload map[string]interface{},
	metadata *common.EventMetadata,
) (*publishData, error) {
	if metadata == nil {
		metadata = &common.EventMetadata{
			Persist: true,
			Headers: make(map[string]string),
		}
	}

	payloadMeta := common.AnalyzePayload(payload)
	payloadMetaJSON, err := json.Marshal(payloadMeta)
	if err != nil {
		return nil, common.NewEnSyncError("failed to marshal payload metadata", common.ErrTypePublish, err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, common.NewEnSyncError("failed to marshal payload", common.ErrTypePublish, err)
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, common.NewEnSyncError("failed to marshal metadata", common.ErrTypePublish, err)
	}

	return &publishData{
		PayloadJSON:     payloadJSON,
		MetadataJSON:    metadataJSON,
		PayloadMetaJSON: payloadMetaJSON,
	}, nil
}

func (e *GRPCEngine) executePublish(
	eventName string,
	recipients []string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	useHybrid bool,
) (string, error) {
	var responses []string
	err := e.WithRetry(e.Ctx, func() error {
		if useHybrid {
			return e.publishHybrid(eventName, recipients, payloadJSON, metadataJSON, payloadMetaJSON, &responses)
		}
		return e.publishIndividual(eventName, recipients, payloadJSON, metadataJSON, payloadMetaJSON, &responses)
	})

	if err != nil {
		return "", common.NewEnSyncError("publish failed after retries", common.ErrTypePublish, err)
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

	for _, recipient := range recipients {
		err := e.WithRetry(e.Ctx, func() error {
			return e.sendPublishRequest(eventName, recipient, encryptedBase64, string(metadataJSON), string(payloadMetaJSON), responses)
		})
		if err != nil {
			e.Logger.Error("Failed to publish to recipient",
				zap.String("recipient", recipient),
				zap.String("eventName", eventName),
				zap.Error(err))
			return common.NewEnSyncError("failed to publish to recipient "+recipient, common.ErrTypePublish, err)
		}
	}
	return nil
}

func (e *GRPCEngine) publishIndividual(
	eventName string,
	recipients []string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	responses *[]string,
) error {
	for _, recipient := range recipients {
		err := e.WithRetry(e.Ctx, func() error {
			// Decode base64-encoded recipient public key
			recipientPubKey, decErr := base64.StdEncoding.DecodeString(recipient)
			if decErr != nil {
				return common.NewEnSyncError("invalid recipient public key format: "+recipient, common.ErrTypePublish, decErr)
			}

			encrypted, encErr := common.EncryptEd25519(string(payloadJSON), recipientPubKey)
			if encErr != nil {
				return common.NewEnSyncError("encryption failed for recipient "+recipient, common.ErrTypePublish, encErr)
			}

			encryptedJSON, encErr := json.Marshal(encrypted)
			if encErr != nil {
				return common.NewEnSyncError("failed to marshal encrypted message", common.ErrTypePublish, encErr)
			}

			encryptedBase64 := base64.StdEncoding.EncodeToString(encryptedJSON)
			return e.sendPublishRequest(eventName, recipient, encryptedBase64, string(metadataJSON), string(payloadMetaJSON), responses)
		})

		if err != nil {
			e.Logger.Error("Failed to publish to recipient",
				zap.String("recipient", recipient),
				zap.String("eventName", eventName),
				zap.Error(err))
			return common.NewEnSyncError("failed to publish to recipient "+recipient, common.ErrTypePublish, err)
		}
	}
	return nil
}

func (e *GRPCEngine) sendPublishRequest(
	eventName, recipient, payload, metadata, payloadMeta string,
	responses *[]string,
) error {
	ctx, cancel := context.WithTimeout(e.Ctx, defaultOperationTimeout)
	defer cancel()

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

	*responses = append(*responses, resp.EventIdem)
	return nil
}

func (e *GRPCEngine) Subscribe(eventName string, options *common.SubscribeOptions) (common.Subscription, error) {
	e.State.Mu.RLock()
	isAuth := e.State.IsAuthenticated
	e.State.Mu.RUnlock()

	if !isAuth {
		return nil, common.ErrNotAuthenticated
	}

	if e.SubscriptionMgr.Exists(eventName) {
		return nil, common.NewEnSyncError("already subscribed to event "+eventName, common.ErrTypeSubscription, nil)
	}

	if options == nil {
		options = &common.SubscribeOptions{
			AutoAck: true,
		}
	}

	var sub *grpcSubscription

	retryErr := e.WithRetry(e.Ctx, func() error {
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
			baseSubscription: baseSubscription{
				eventName: eventName,
				handlers:  make([]common.EventHandler, 0),
			},
			stream:       stream,
			autoAck:      options.AutoAck,
			appSecretKey: options.AppSecretKey,
			engine:       e,
			cancel:       cancel,
		}

		e.SubscriptionMgr.Store(eventName, sub)
		go sub.listen()

		e.Logger.Info("Successfully subscribed", zap.String("eventName", eventName))
		return nil
	})

	if retryErr != nil {
		return nil, common.NewEnSyncError("subscription failed after retries", common.ErrTypeSubscription, retryErr)
	}

	return sub, nil
}

// AddHandler adds an event handler to the subscription and returns a function to remove it
func (s *grpcSubscription) AddHandler(handler common.EventHandler) func() {
	s.mu.Lock()
	s.handlers = append(s.handlers, handler)
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, h := range s.handlers {
			if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
				s.handlers = append(s.handlers[:i], s.handlers[i+1:]...)
				break
			}
		}
	}
}

func (s *grpcSubscription) GetHandlers() []common.EventHandler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	handlers := make([]common.EventHandler, len(s.handlers))
	copy(handlers, s.handlers)
	return handlers
}

func (s *grpcSubscription) Defer(eventIdem string, delayMs int64, reason string) (*common.DeferResponse, error) {
	var resp *common.DeferResponse

	err := s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, defaultOperationTimeout)
		defer cancel()

		deferResp, deferErr := s.engine.client.DeferEvent(ctx, &pb.DeferRequest{
			ClientId:  s.engine.ClientID,
			EventIdem: eventIdem,
			DelayMs:   delayMs,
			Reason:    reason,
		})

		if deferErr != nil {
			return common.NewEnSyncError("defer request failed", common.ErrTypeDefer, deferErr)
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
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *grpcSubscription) Discard(eventIdem, reason string) (*common.DiscardResponse, error) {
	var resp *common.DiscardResponse

	err := s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, defaultOperationTimeout)
		defer cancel()

		discardResp, discardErr := s.engine.client.DiscardEvent(ctx, &pb.DiscardRequest{
			ClientId:  s.engine.ClientID,
			EventIdem: eventIdem,
			Reason:    reason,
		})

		if discardErr != nil {
			return common.NewEnSyncError("discard request failed", common.ErrTypeDiscard, discardErr)
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
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *grpcSubscription) Pause(reason string) error {
	return s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, defaultOperationTimeout)
		defer cancel()

		resp, err := s.engine.client.PauseEvents(ctx, &pb.PauseRequest{
			ClientId:  s.engine.ClientID,
			EventName: s.eventName,
			Reason:    reason,
		})

		if err != nil {
			return common.NewEnSyncError("pause request failed", common.ErrTypePause, err)
		}

		if !resp.Success {
			return common.NewEnSyncError("pause rejected: "+resp.Message, common.ErrTypePause, nil)
		}

		return nil
	})
}

func (s *grpcSubscription) Resume() error {
	return s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, defaultOperationTimeout)
		defer cancel()

		resp, err := s.engine.client.ContinueEvents(ctx, &pb.ContinueRequest{
			ClientId:  s.engine.ClientID,
			EventName: s.eventName,
		})

		if err != nil {
			return common.NewEnSyncError("resume request failed", common.ErrTypeResume, err)
		}

		if !resp.Success {
			return common.NewEnSyncError("resume rejected: "+resp.Message, common.ErrTypeResume, nil)
		}

		return nil
	})
}

func (s *grpcSubscription) Replay(eventIdem string) (*common.EventPayload, error) {
	var result *common.EventPayload

	err := s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, defaultOperationTimeout)
		defer cancel()

		resp, replayErr := s.engine.client.ReplayEvent(
			ctx, &pb.ReplayRequest{
				ClientId:  s.engine.ClientID,
				EventName: s.eventName,
				EventIdem: eventIdem,
			})

		if replayErr != nil {
			return common.NewEnSyncError("replay request failed", common.ErrTypeReplay, replayErr)
		}

		if !resp.Success {
			return common.NewEnSyncError("replay rejected: "+resp.Message, common.ErrTypeReplay, nil)
		}

		var eventData pb.EventStreamResponse
		if err := json.Unmarshal([]byte(resp.EventData), &eventData); err != nil {
			return common.NewEnSyncError("failed to parse replayed event", common.ErrTypeReplay, err)
		}

		processed, processErr := s.processEvent(&eventData)
		if processErr != nil {
			return processErr
		}

		result = processed
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *grpcSubscription) Rollback(eventIdem string, block int64) error {
	return common.NewEnSyncError("rollback not implemented in gRPC version", common.ErrTypeGeneric, nil)
}

func (s *grpcSubscription) listen() {
	defer func() {
		if r := recover(); r != nil {
			s.engine.Logger.Error("Subscription listener panic",
				zap.String("eventName", s.eventName),
				zap.Any("panic", r))
		}
	}()

	for {
		select {
		case <-s.engine.Ctx.Done():
			s.engine.Logger.Info("Subscription listener stopped due to context cancellation",
				zap.String("eventName", s.eventName))
			return
		default:
			event, err := s.stream.Recv()
			if err != nil {
				s.handleStreamError(err)
				return
			}
			s.handleEvent(event)
		}
	}
}

func (s *grpcSubscription) handleStreamError(err error) {
	if s.engine.Ctx.Err() != nil {
		s.engine.Logger.Info("Stream closed due to context cancellation",
			zap.String("eventName", s.eventName))
		return
	}

	if st, ok := status.FromError(err); ok && st.Code() == codes.Canceled {
		s.engine.Logger.Info("Stream canceled",
			zap.String("eventName", s.eventName),
			zap.String("reason", st.Message()))
		return
	}

	// Log as error only if it's not a expected cancellation
	s.engine.Logger.Error("Stream receive error",
		zap.String("eventName", s.eventName),
		zap.Error(err))

	s.engine.State.Mu.Lock()
	s.engine.State.IsConnected = false
	s.engine.State.Mu.Unlock()
}

func (s *grpcSubscription) handleEvent(event *pb.EventStreamResponse) {
	processedEvent, err := s.processEvent(event)
	if err != nil {
		s.engine.Logger.Error("Event processing failed",
			zap.String("eventName", s.eventName),
			zap.String("eventId", event.EventIdem),
			zap.Error(err))
		return
	}
	if processedEvent == nil {
		return
	}

	s.callHandlers(processedEvent)

	if s.autoAck {
		if err := s.Ack(processedEvent.Idem, processedEvent.Block); err != nil {
			s.engine.Logger.Error("Auto-ack failed",
				zap.String("eventId", processedEvent.Idem),
				zap.Error(err))
		}
	}
}

func (s *grpcSubscription) callHandlers(event *common.EventPayload) {
	handlers := s.GetHandlers()
	for _, handler := range handlers {
		go func(h common.EventHandler) {
			defer func() {
				if r := recover(); r != nil {
					s.engine.Logger.Error("Handler panic",
						zap.String("eventName", s.eventName),
						zap.String("eventId", event.Idem),
						zap.Any("panic", r))
				}
			}()
			if err := h(event); err != nil {
				s.engine.Logger.Error("Event handler error",
					zap.String("eventName", s.eventName),
					zap.String("eventId", event.Idem),
					zap.Error(err))
			}
		}(handler)
	}
}

// processEvent processes a single event
func (s *grpcSubscription) processEvent(event *pb.EventStreamResponse) (*common.EventPayload, error) {
	decryptionKey := s.appSecretKey
	if decryptionKey == "" {
		s.engine.Logger.Error("No decryption key available")
		return nil, common.NewEnSyncError("no decryption key available", common.ErrTypeSubscription, nil)
	}

	// Decode and decrypt payload
	decodedPayload, err := base64.StdEncoding.DecodeString(event.Payload)
	if err != nil {
		return nil, common.NewEnSyncError("failed to decode payload", common.ErrTypeSubscription, err)
	}

	encryptedPayload, err := common.ParseEncryptedPayload(string(decodedPayload))
	if err != nil {
		return nil, common.NewEnSyncError("failed to parse encrypted payload", common.ErrTypeSubscription, err)
	}

	keyBytes, err := base64.StdEncoding.DecodeString(decryptionKey)
	if err != nil {
		return nil, common.NewEnSyncError("failed to decode decryption key", common.ErrTypeSubscription, err)
	}

	var payloadStr string

	switch v := encryptedPayload.(type) {
	case *common.HybridEncryptedMessage:
		payloadStr, err = common.DecryptHybridMessage(v, keyBytes)
		if err != nil {
			return nil, common.NewEnSyncError("hybrid decryption failed", common.ErrTypeSubscription, err)
		}
	case *common.EncryptedMessage:
		payloadStr, err = common.DecryptEd25519(v, keyBytes)
		if err != nil {
			return nil, common.NewEnSyncError("ed25519 decryption failed", common.ErrTypeSubscription, err)
		}
	default:
		return nil, common.NewEnSyncError("unknown encrypted payload type: "+fmt.Sprintf("%T", v), common.ErrTypeSubscription, nil)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return nil, common.NewEnSyncError("failed to unmarshal decrypted payload", common.ErrTypeSubscription, err)
	}

	var metadata map[string]interface{}
	if event.Metadata != "" {
		if err := json.Unmarshal([]byte(event.Metadata), &metadata); err != nil {
			s.engine.Logger.Warn("Failed to unmarshal metadata", zap.Error(err))
			metadata = make(map[string]interface{})
		}
	} else {
		metadata = make(map[string]interface{})
	}

	return &common.EventPayload{
		EventName: event.EventName,
		Idem:      event.EventIdem,
		Block:     event.PartitionBlock,
		Timestamp: time.Now(),
		Payload:   payload,
		Metadata:  metadata,
		Sender:    event.Sender,
	}, nil
}

func (s *grpcSubscription) Ack(eventID string, block int64) error {
	return s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, defaultOperationTimeout)
		defer cancel()

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
	})
}

// Unsubscribe cancels the subscription
func (s *grpcSubscription) Unsubscribe() error {
	return s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, defaultOperationTimeout)
		defer cancel()

		resp, err := s.engine.client.Unsubscribe(ctx, &pb.UnsubscribeRequest{
			ClientId:  s.engine.ClientID,
			EventName: s.eventName,
		})

		if err != nil {
			return common.NewEnSyncError("unsubscribe request failed", common.ErrTypeSubscription, err)
		}

		if !resp.Success {
			return common.NewEnSyncError("unsubscribe rejected: "+resp.Message, common.ErrTypeSubscription, nil)
		}

		s.cancel()
		s.engine.SubscriptionMgr.Delete(s.eventName)

		s.engine.Logger.Info("Successfully unsubscribed", zap.String("eventName", s.eventName))
		return nil
	})
}

// Close closes the gRPC connection
func (e *GRPCEngine) Close() error {
	e.Logger.Info("Shutting down gRPC engine")

	e.State.Mu.Lock()
	e.State.IsAuthenticated = false
	e.State.IsConnected = false
	e.State.Mu.Unlock()

	// Close all subscriptions with timeout
	done := make(chan bool)
	go func() {
		e.SubscriptionMgr.Range(func(key, value interface{}) bool {
			if sub, ok := value.(*grpcSubscription); ok {
				sub.cancel()
				// Give some time for cleanup
				time.Sleep(cleanupDelay)
			}
			e.SubscriptionMgr.Delete(key.(string))
			return true
		})
		close(done)
	}()

	// Wait for cleanup with timeout
	select {
	case <-done:
		e.Logger.Info("All subscriptions closed successfully")
	case <-time.After(shutdownTimeout):
		e.Logger.Warn("Timeout waiting for subscriptions to close")
	}

	// Close gRPC connection
	if e.conn != nil {
		if err := e.conn.Close(); err != nil {
			e.Logger.Error("Error closing gRPC connection", zap.Error(err))
			return common.NewEnSyncError("failed to close gRPC connection", common.ErrTypeConnection, err)
		}
	}

	e.Logger.Info("gRPC engine shutdown complete")
	return nil
}
