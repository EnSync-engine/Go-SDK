package grpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/EnSync-engine/Go-SDK/common"
	pb "github.com/EnSync-engine/Go-SDK/internal/proto"
)

const (
	statusDeferred  = "deferred"
	statusSuccess   = "success"
	actionDiscarded = "discarded"
)

type GRPCEngine struct {
	*common.BaseEngine
	client    pb.EnSyncServiceClient
	conn      *grpc.ClientConn
	closeOnce sync.Once
	closed    atomic.Bool
}

type grpcSubscription struct {
	*common.BaseSubscription
	stream       pb.EnSyncService_SubscribeClient
	appSecretKey string
	engine       *GRPCEngine
	cancel       context.CancelFunc
}

func NewGRPCEngine(
	ctx context.Context,
	endpoint string,
	opts ...common.Option,
) (*GRPCEngine, error) {
	if ctx == nil {
		return nil, common.NewEnSyncError("context cannot be nil", common.ErrTypeValidation, nil)
	}

	if endpoint == "" {
		return nil, common.NewEnSyncError("endpoint cannot be empty", common.ErrTypeValidation, nil)
	}

	host, secure, err := parseGRPCURL(endpoint)
	if err != nil {
		return nil, err
	}

	baseEngine, err := common.NewBaseEngine(ctx, opts...)
	if err != nil {
		return nil, common.NewEnSyncError("failed to create base engine", common.ErrTypeConnection, err)
	}

	conn, err := createClientConn(host, secure)
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

func (e *GRPCEngine) CreateClient(accessKey string) error {
	e.AccessKey = accessKey

	if err := e.authenticate(); err != nil {
		return err
	}
	return nil
}

func (e *GRPCEngine) authenticate() error {
	ctx, cancel := context.WithTimeout(e.Ctx, e.GetConnectionTimeout())
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

	e.SetAuthenticated(resp.ClientId, resp.ClientHash)
	e.Logger.Info("Authentication successful")
	go e.startHeartbeat()
	return nil
}

func (e *GRPCEngine) startHeartbeat() {
	ticker := time.NewTicker(e.GetPingInterval())
	defer ticker.Stop()

	for {
		select {
		case <-e.Ctx.Done():
			e.Logger.Info("Heartbeat stopped due to context cancellation")
			return
		case <-ticker.C:
			if err := e.sendHeartbeat(); err != nil {
				e.Logger.Error("Heartbeat failed", zap.Error(err))
				e.SetConnectionState(false)
			}
		}
	}
}

func (e *GRPCEngine) sendHeartbeat() error {
	if !e.IsConnected() {
		return nil
	}

	return e.WithRetry(e.Ctx, func() error {
		ctx, cancel := context.WithTimeout(e.Ctx, e.GetOperationTimeout())
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
	metadata *common.MessageMetadata,
	options *common.PublishOptions,
) (string, error) {
	if e.closed.Load() {
		return "", common.NewEnSyncError("engine is closed", common.ErrTypeConnection, nil)
	}
	if !e.IsConnected() {
		return "", common.NewEnSyncError("client not connected/authenticated", common.ErrTypeConnection, nil)
	}
	if len(recipients) == 0 {
		return "", common.NewEnSyncError("recipients required", common.ErrTypeValidation, nil)
	}

	if metadata == nil {
		metadata = &common.MessageMetadata{
			Persist: true,
			Headers: make(map[string]string),
		}
	}

	useHybridEncryption := true
	if options != nil {
		useHybridEncryption = options.UseHybridEncryption
	}

	publishData, err := e.preparePublishData(payload, metadata)
	if err != nil {
		return "", err
	}

	return e.executePublish(
		eventName, recipients,
		publishData.PayloadJSON, publishData.MetadataJSON, publishData.PayloadMetaJSON,
		useHybridEncryption,
	)
}

func (e *GRPCEngine) preparePublishData(
	payload map[string]interface{},
	metadata *common.MessageMetadata,
) (*common.PublishData, error) {
	if metadata == nil {
		metadata = &common.MessageMetadata{
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

	return &common.PublishData{
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
	responseChan := make(chan string, len(recipients))
	var err error

	if useHybrid {
		err = e.publishHybrid(eventName, recipients, payloadJSON, metadataJSON, payloadMetaJSON, responseChan)
	} else {
		err = e.publishIndividual(eventName, recipients, payloadJSON, metadataJSON, payloadMetaJSON, responseChan)
	}

	close(responseChan)

	responses := make([]string, 0, len(recipients))
	for id := range responseChan {
		responses = append(responses, id)
	}

	if err != nil {
		return "", common.NewEnSyncError("publish failed", common.ErrTypePublish, err)
	}

	return strings.Join(responses, ","), nil
}

func (e *GRPCEngine) publishHybrid(
	eventName string,
	recipients []string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	responseChan chan<- string,
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
			return e.sendPublishRequest(eventName, recipient, encryptedBase64, string(metadataJSON), string(payloadMetaJSON), responseChan)
		})
		if err != nil {
			e.Logger.Error(fmt.Sprintf("Failed to publish to recipient: %v", err),
				"recipient", recipient,
				"eventName", eventName)
			return err
		}
	}
	return nil
}

func (e *GRPCEngine) publishIndividual(
	eventName string,
	recipients []string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	responseChan chan<- string,
) error {
	for _, recipient := range recipients {
		err := e.WithRetry(e.Ctx, func() error {
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
			return e.sendPublishRequest(eventName, recipient, encryptedBase64, string(metadataJSON), string(payloadMetaJSON), responseChan)
		})

		if err != nil {
			e.Logger.Error(fmt.Sprintf("Failed to publish to recipient: %v", err),
				"recipient", recipient,
				"eventName", eventName)
			return err
		}
	}
	return nil
}

func (e *GRPCEngine) sendPublishRequest(
	eventName, recipient, payload, metadata, payloadMeta string,
	responseChan chan<- string,
) error {
	ctx, cancel := context.WithTimeout(e.Ctx, e.GetOperationTimeout())
	defer cancel()

	req := &pb.PublishMessageRequest{
		ClientId:    e.ClientID,
		MessageName: eventName,
		Payload:     payload,
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

	resp, err := e.client.PublishMessage(ctx, req)
	if err != nil {
		return common.NewEnSyncError("publish request failed", common.ErrTypePublish, err)
	}

	if !resp.Success {
		return common.NewEnSyncError("publish rejected: "+resp.ErrorMessage, common.ErrTypePublish, nil)
	}

	responseChan <- resp.MessageIdem
	return nil
}

func (e *GRPCEngine) Subscribe(messageName string, options *common.SubscribeOptions) (common.Subscription, error) {
	if !e.IsConnected() {
		return nil, common.ErrNotAuthenticated
	}

	if e.SubscriptionMgr.Exists(messageName) {
		return nil, common.NewEnSyncError("already subscribed to message "+messageName, common.ErrTypeSubscription, nil)
	}

	if options == nil {
		options = &common.SubscribeOptions{
			AutoAck: true,
		}
	}

	subCtx, cancel := context.WithCancel(e.Ctx)

	stream, err := e.client.Subscribe(subCtx, &pb.SubscribeRequest{
		ClientId:    e.ClientID,
		MessageName: messageName,
	})

	if err != nil {
		cancel()
		return nil, common.NewEnSyncError("subscription stream error", common.ErrTypeSubscription, err)
	}

	sub := &grpcSubscription{
		BaseSubscription: common.NewBaseSubscription(messageName, options.AutoAck, 10),
		stream:           stream,
		appSecretKey:     options.AppSecretKey,
		engine:           e,
		cancel:           cancel,
	}

	e.SubscriptionMgr.Store(messageName, sub)
	go sub.listen()

	e.Logger.Info("Successfully subscribed", zap.String("messageName", messageName))
	return sub, nil
}

func (s *grpcSubscription) Defer(messageIdem string, delayMs int64, reason string) (*common.DeferResponse, error) {
	var resp *common.DeferResponse

	err := s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, s.engine.GetOperationTimeout())
		defer cancel()

		deferResp, deferErr := s.engine.client.DeferMessage(ctx, &pb.DeferRequest{
			ClientId:    s.engine.ClientID,
			MessageIdem: messageIdem,
			DelayMs:     delayMs,
			Reason:      reason,
		})

		if deferErr != nil {
			return common.NewEnSyncError("defer request failed", common.ErrTypeDefer, deferErr)
		}

		if !deferResp.Success {
			return common.NewEnSyncError("defer rejected: "+deferResp.Message, common.ErrTypeDefer, nil)
		}

		resp = &common.DeferResponse{
			MessageID:         messageIdem,
			Status:            statusDeferred,
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

func (s *grpcSubscription) Discard(messageIdem, reason string) (*common.DiscardResponse, error) {
	var resp *common.DiscardResponse

	err := s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, s.engine.GetOperationTimeout())
		defer cancel()

		discardResp, discardErr := s.engine.client.DiscardMessage(ctx, &pb.DiscardRequest{
			ClientId:    s.engine.ClientID,
			MessageIdem: messageIdem,
			Reason:      reason,
		})

		if discardErr != nil {
			return common.NewEnSyncError("discard request failed", common.ErrTypeDiscard, discardErr)
		}

		if !discardResp.Success {
			return common.NewEnSyncError("discard rejected: "+discardResp.Message, common.ErrTypeDiscard, nil)
		}

		resp = &common.DiscardResponse{
			MessageID: messageIdem,
			Status:    statusSuccess,
			Action:    actionDiscarded,
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
		ctx, cancel := context.WithTimeout(s.engine.Ctx, s.engine.GetOperationTimeout())
		defer cancel()

		resp, err := s.engine.client.PauseMessages(ctx, &pb.PauseRequest{
			ClientId:    s.engine.ClientID,
			MessageName: s.MessageName,
			Reason:      reason,
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
		ctx, cancel := context.WithTimeout(s.engine.Ctx, s.engine.GetOperationTimeout())
		defer cancel()

		resp, err := s.engine.client.ContinueMessages(ctx, &pb.ContinueRequest{
			ClientId:    s.engine.ClientID,
			MessageName: s.MessageName,
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

func (s *grpcSubscription) Replay(messageIdem string) (*common.MessagePayload, error) {
	var result *common.MessagePayload

	err := s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, s.engine.GetOperationTimeout())
		defer cancel()

		resp, replayErr := s.engine.client.ReplayMessage(
			ctx, &pb.ReplayRequest{
				ClientId:    s.engine.ClientID,
				MessageName: s.MessageName,
				MessageIdem: messageIdem,
			})

		if replayErr != nil {
			return common.NewEnSyncError("replay request failed", common.ErrTypeReplay, replayErr)
		}

		if !resp.Success {
			return common.NewEnSyncError("replay rejected: "+resp.Message, common.ErrTypeReplay, nil)
		}

		var messageData pb.MessageStreamResponse
		if err := json.Unmarshal([]byte(resp.MessageData), &messageData); err != nil {
			return common.NewEnSyncError("failed to parse replayed message", common.ErrTypeReplay, err)
		}

		processed, processErr := s.processMessage(&messageData)
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
				zap.String("eventName", s.MessageName),
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	recvChan := make(chan *pb.MessageStreamResponse, s.engine.GetRecvBufferSize())
	errChan := make(chan error, 1)
	recvDone := make(chan struct{})

	go func() {
		defer close(recvDone)
		for {
			event, err := s.stream.Recv()
			if err != nil {
				select {
				case errChan <- err:
				case <-s.engine.Ctx.Done():
				}
				return
			}
			select {
			case recvChan <- event:
			case <-s.engine.Ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-s.engine.Ctx.Done():
			s.engine.Logger.Info("Subscription listener stopped due to context cancellation",
				zap.String("eventName", s.MessageName))

			s.cancel()

			select {
			case <-recvDone:
			case <-time.After(s.engine.GetRecvTimeout()):
				s.engine.Logger.Warn("Stream receiver still blocked after timeout, may indicate stuck gRPC stream",
					zap.String("eventName", s.MessageName),
					zap.Duration("timeout", s.engine.GetRecvTimeout()))
			}
			return

		case event := <-recvChan:
			s.handleMessage(event)

		case err := <-errChan:
			s.handleStreamError(err)
			return
		}
	}
}

func (s *grpcSubscription) handleStreamError(err error) {
	if s.engine.Ctx.Err() != nil {
		s.engine.Logger.Info("Stream closed due to context cancellation",
			zap.String("eventName", s.MessageName))
		return
	}

	if st, ok := status.FromError(err); ok && st.Code() == codes.Canceled {
		s.engine.Logger.Info("Stream canceled",
			zap.String("eventName", s.MessageName),
			zap.String("reason", st.Message()))
		return
	}

	s.engine.Logger.Error("Stream receive error",
		zap.String("eventName", s.MessageName),
		zap.Error(err))

	s.engine.SetConnectionState(false)
}

func (s *grpcSubscription) handleMessage(event *pb.MessageStreamResponse) {
	processedMessage, err := s.processMessage(event)
	if err != nil {
		s.engine.Logger.Error("Message processing failed",
			zap.String("messageName", s.MessageName),
			zap.String("messageId", event.MessageIdem),
			zap.Error(err))
		return
	}
	if processedMessage == nil {
		return
	}

	s.BaseSubscription.ProcessMessage(processedMessage, s.engine.Logger, s.Ack)
}

func (s *grpcSubscription) processMessage(message *pb.MessageStreamResponse) (*common.MessagePayload, error) {
	decryptionKey := s.appSecretKey
	if decryptionKey == "" {
		s.engine.Logger.Error("No decryption key available")
		return nil, common.NewEnSyncError("no decryption key available", common.ErrTypeSubscription, nil)
	}

	decodedPayload, err := base64.StdEncoding.DecodeString(message.Payload)
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
	if message.Metadata != "" {
		if err := json.Unmarshal([]byte(message.Metadata), &metadata); err != nil {
			s.engine.Logger.Warn("Failed to unmarshal metadata", zap.Error(err))
			metadata = make(map[string]interface{})
		}
	} else {
		metadata = make(map[string]interface{})
	}

	return &common.MessagePayload{
		MessageName: message.MessageName,
		Idem:        message.MessageIdem,
		Block:       message.PartitionBlock,
		Timestamp:   time.Now().UnixMilli(),
		Payload:     payload,
		Metadata:    metadata,
		Sender:      message.Sender,
	}, nil
}

func (s *grpcSubscription) Ack(eventName string, eventID string, block int64) error {
	return s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, s.engine.GetOperationTimeout())
		defer cancel()

		resp, err := s.engine.client.AcknowledgeMessage(ctx, &pb.AcknowledgeRequest{
			ClientId:       s.engine.ClientID,
			MessageIdem:    eventID,
			MessageName:    eventName,
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

func (s *grpcSubscription) Unsubscribe() error {
	return s.engine.WithRetry(s.engine.Ctx, func() error {
		ctx, cancel := context.WithTimeout(s.engine.Ctx, s.engine.GetOperationTimeout())
		defer cancel()

		resp, err := s.engine.client.Unsubscribe(ctx, &pb.UnsubscribeRequest{
			ClientId:    s.engine.ClientID,
			MessageName: s.MessageName,
		})

		if err != nil {
			return common.NewEnSyncError("unsubscribe request failed", common.ErrTypeSubscription, err)
		}

		if !resp.Success {
			return common.NewEnSyncError("unsubscribe rejected: "+resp.Message, common.ErrTypeSubscription, nil)
		}

		s.cancel()
		s.engine.SubscriptionMgr.Delete(s.MessageName)

		s.engine.Logger.Info("Successfully unsubscribed", zap.String("eventName", s.MessageName))
		return nil
	})
}

func (e *GRPCEngine) Close() error {
	var closeErr error

	e.closeOnce.Do(func() {
		e.closed.Store(true)
		e.Logger.Info("Shutting down gRPC engine")

		e.ResetState()

		// Close all subscriptions with timeout
		done := make(chan bool)
		go func() {
			e.SubscriptionMgr.Range(func(key, value interface{}) bool {
				if sub, ok := value.(*grpcSubscription); ok {
					sub.cancel()
					// Give some time for cleanup
					time.Sleep(e.GetCleanupDelay())
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
		case <-time.After(e.GetGracefulShutdownTimeout()):
			e.Logger.Warn("Graceful shutdown timed out, forcing close")
		}

		// Close gRPC connection
		if e.conn != nil {
			if err := e.conn.Close(); err != nil {
				e.Logger.Error("Error closing gRPC connection", zap.Error(err))
				closeErr = common.NewEnSyncError("failed to close gRPC connection", common.ErrTypeConnection, err)
				return
			}
		}

		e.Logger.Info("gRPC engine shutdown complete")
	})

	return closeErr
}
