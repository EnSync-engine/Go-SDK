package ensync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	pb "github.com/EnSync-engine/Go-SDK/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	serviceName = "EnSync:"
)

type grpcConfig struct {
	heartbeatInterval time.Duration
}

type GRPCEngine struct {
	baseEngine
	client pb.EnSyncServiceClient
	conn   *grpc.ClientConn

	grpcConfig grpcConfig

	subscriptions grpcSubscriptions

	heartbeatCancel context.CancelFunc
}

type grpcSubscription struct {
	baseSubscription
	eventName    string
	stream       pb.EnSyncService_SubscribeClient
	autoAck      bool
	appSecretKey string
	engine       *GRPCEngine
	cancel       context.CancelFunc
}

type GRPCOption func(*GRPCEngine)

func WithHeartbeatInterval(interval time.Duration) GRPCOption {
	return func(e *GRPCEngine) {
		e.grpcConfig.heartbeatInterval = interval
	}
}

func WithMaxReconnectAttempts(attempts int) GRPCOption {
	return func(e *GRPCEngine) {
		e.config.maxReconnectAttempts = attempts
	}
}

func WithReconnectDelay(delay time.Duration) GRPCOption {
	return func(e *GRPCEngine) {
		e.config.reconnectDelay = delay
	}
}

func WithAutoReconnect(enabled bool) GRPCOption {
	return func(e *GRPCEngine) {
		e.config.autoReconnect = enabled
	}
}

func WithLogger(logger Logger) GRPCOption {
	return func(e *GRPCEngine) {
		e.SetLogger(logger)
	}
}

func WithGRPCReconnectEnabled(enabled bool) GRPCOption {
	return func(e *GRPCEngine) {
		e.state.shouldReconnect = enabled
		e.config.autoReconnect = enabled
	}
}

// NewGRPCEngine creates a new gRPC engine with the provided context and URL.
// It applies sensible defaults and allows customization through functional options.
func NewGRPCEngine(ctx context.Context, url string, opts ...GRPCOption) (*GRPCEngine, error) {
	if ctx == nil {
		return nil, NewEnSyncError("context cannot be nil", ErrTypeValidation, nil)
	}

	if url == "" {
		return nil, NewEnSyncError("URL cannot be empty", ErrTypeValidation, nil)
	}

	// Parse and validate URL earlyxw
	serverAddress, creds, err := buildGRPCConnection(url)
	if err != nil {
		return nil, err
	}

	// Create engine with defaults
	engine := &GRPCEngine{
		grpcConfig: grpcConfig{
			heartbeatInterval: 30 * time.Second,
		},
		subscriptions: grpcSubscriptions{
			subs: make(map[string]*grpcSubscription),
		},
	}

	// Initialize base engine
	engine.SetContext(ctx)
	engine.config.url = serverAddress
	engine.state.shouldReconnect = true

	// Apply custom options
	for _, opt := range opts {
		if opt != nil {
			opt(engine)
		}
	}

	// Establish gRPC connection
	conn, err := grpc.Dial(serverAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, NewEnSyncError(fmt.Sprintf("failed to connect to %s", serverAddress), ErrTypeConnection, err)
	}

	engine.conn = conn
	engine.client = pb.NewEnSyncServiceClient(conn)

	return engine, nil
}

// buildGRPCConnection parses the URL and determines the appropriate gRPC credentials.
// It validates the URL format and returns the server address and transport credentials.
func buildGRPCConnection(urlStr string) (string, credentials.TransportCredentials, error) {
	var serverAddress string
	var creds credentials.TransportCredentials

	// Handle different URL schemes
	switch {
	case strings.HasPrefix(urlStr, "grpcs://"):
		serverAddress = strings.TrimPrefix(urlStr, "grpcs://")
		creds = credentials.NewTLS(nil) // Use system cert pool
	case strings.HasPrefix(urlStr, "grpc://"):
		serverAddress = strings.TrimPrefix(urlStr, "grpc://")
		creds = insecure.NewCredentials()
	case strings.Contains(urlStr, "://"):
		return "", nil, NewEnSyncError("unsupported URL scheme: must be grpc:// or grpcs://", ErrTypeValidation, nil)
	default:
		// Default to insecure for backward compatibility (plain address)
		serverAddress = urlStr
		creds = insecure.NewCredentials()
	}

	if serverAddress == "" {
		return "", nil, NewEnSyncError("invalid server address", ErrTypeValidation, nil)
	}

	return serverAddress, creds, nil
}

func (e *GRPCEngine) CreateClient(accessKey string, options ...ClientOption) error {
	cfg := &clientConfig{}
	for _, opt := range options {
		opt(cfg)
	}

	e.config.accessKey = accessKey
	if cfg.appSecretKey != "" {
		e.config.appSecretKey = cfg.appSecretKey
	}

	return e.authenticate()
}

func (e *GRPCEngine) authenticate() error {
	e.logger.Info("Sending authentication request")

	ctx, cancel := context.WithTimeout(e.ctx, 10*time.Second)
	defer cancel()

	resp, err := e.client.Connect(ctx, &pb.ConnectRequest{
		AccessKey: e.config.accessKey,
	})

	if err != nil {
		return NewEnSyncError("authentication failed", ErrTypeAuth, err)
	}

	if !resp.Success {
		return NewEnSyncError(resp.ErrorMessage, ErrTypeAuth, nil)
	}

	e.logger.Info("Authentication successful")
	e.config.clientID = resp.ClientId
	e.config.clientHash = resp.ClientHash

	e.state.mu.Lock()
	e.state.isAuthenticated = true
	e.state.isConnected = true
	e.state.mu.Unlock()

	// Start heartbeat
	e.startHeartbeat()

	return nil
}

func (e *GRPCEngine) Publish(eventName string, recipients []string, payload map[string]interface{}, metadata *EventMetadata, options *PublishOptions) (string, error) {
	e.state.mu.RLock()
	isAuth := e.state.isAuthenticated
	e.state.mu.RUnlock()

	if !isAuth {
		return "", NewEnSyncError(NotAuthenticated, ErrTypeAuth, nil)
	}

	if len(recipients) == 0 {
		return "", NewEnSyncError(RecipientsRequired, ErrTypeValidation, nil)
	}

	useHybridEncryption := true
	if options != nil {
		useHybridEncryption = options.UseHybridEncryption
	}

	if metadata == nil {
		metadata = &EventMetadata{
			Persist: true,
			Headers: make(map[string]string),
		}
	}

	// Calculate payload metadata
	payloadMeta := e.AnalyzePayload(payload)
	payloadMetaJSON, _ := json.Marshal(payloadMeta)

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", NewEnSyncError("failed to marshal payload", ErrTypePublish, err)
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", NewEnSyncError("failed to marshal metadata", ErrTypePublish, err)
	}

	var responses []string

	// Use hybrid encryption for multiple recipients
	if useHybridEncryption && len(recipients) > 1 {
		hybridMsg, err := HybridEncrypt(string(payloadJSON), recipients)
		if err != nil {
			return "", NewEnSyncError("hybrid encryption failed", ErrTypePublish, err)
		}

		hybridJSON, err := json.Marshal(hybridMsg)
		if err != nil {
			return "", NewEnSyncError("failed to marshal hybrid message", ErrTypePublish, err)
		}

		encryptedBase64 := base64.StdEncoding.EncodeToString(hybridJSON)

		// Send to all recipients with same encrypted payload
		for _, recipient := range recipients {
			resp, err := e.publishEvent(&pb.PublishEventRequest{
				ClientId:        e.config.clientID,
				EventName:       eventName,
				Payload:         encryptedBase64,
				DeliveryTo:      recipient,
				Metadata:        string(metadataJSON),
				PayloadMetadata: (*string)(&[]string{string(payloadMetaJSON)}[0]),
			})

			if err != nil {
				return "", err
			}

			responses = append(responses, resp.EventIdem)
		}
	} else {
		// Use traditional encryption (separate for each recipient)
		for _, recipient := range recipients {
			recipientKey, err := base64.StdEncoding.DecodeString(recipient)
			if err != nil {
				return "", NewEnSyncError("invalid recipient key", ErrTypePublish, err)
			}

			encrypted, err := EncryptEd25519(string(payloadJSON), recipientKey)
			if err != nil {
				return "", NewEnSyncError("encryption failed", ErrTypePublish, err)
			}

			encryptedJSON, err := json.Marshal(encrypted)
			if err != nil {
				return "", NewEnSyncError("failed to marshal encrypted message", ErrTypePublish, err)
			}

			encryptedBase64 := base64.StdEncoding.EncodeToString(encryptedJSON)

			resp, err := e.publishEvent(&pb.PublishEventRequest{
				ClientId:        e.config.clientID,
				EventName:       eventName,
				Payload:         encryptedBase64,
				DeliveryTo:      recipient,
				Metadata:        string(metadataJSON),
				PayloadMetadata: (*string)(&[]string{string(payloadMetaJSON)}[0]),
			})

			if err != nil {
				return "", err
			}

			responses = append(responses, resp.EventIdem)
		}
	}

	return strings.Join(responses, ","), nil
}

func (e *GRPCEngine) publishEvent(req *pb.PublishEventRequest) (*pb.PublishEventResponse, error) {
	ctx, cancel := context.WithTimeout(e.ctx, 10*time.Second)
	defer cancel()

	resp, err := e.client.PublishEvent(ctx, req)
	if err != nil {
		return nil, NewEnSyncError("publish failed", ErrTypePublish, err)
	}

	if !resp.Success {
		return nil, NewEnSyncError(resp.ErrorMessage, ErrTypePublish, nil)
	}

	return resp, nil
}

func (e *GRPCEngine) Subscribe(eventName string, options *SubscribeOptions) (Subscription, error) {
	e.state.mu.RLock()
	isAuth := e.state.isAuthenticated
	e.state.mu.RUnlock()

	if !isAuth {
		return nil, NewEnSyncError(NotAuthenticated, ErrTypeAuth, nil)
	}

	if options == nil {
		options = &SubscribeOptions{
			AutoAck: true,
		}
	}

	ctx, cancel := context.WithCancel(e.ctx)
	stream, err := e.client.Subscribe(ctx, &pb.SubscribeRequest{
		ClientId:  e.config.clientID,
		EventName: eventName,
	})

	if err != nil {
		cancel()
		return nil, NewEnSyncError("subscription failed", ErrTypeSubscription, err)
	}

	sub := &grpcSubscription{
		baseSubscription: baseSubscription{
			handlers: make([]EventHandler, 0),
		},
		eventName:    eventName,
		stream:       stream,
		autoAck:      options.AutoAck,
		appSecretKey: options.AppSecretKey,
		engine:       e,
		cancel:       cancel,
	}

	e.subscriptions.mu.Lock()
	e.subscriptions.subs[eventName] = sub
	e.subscriptions.mu.Unlock()

	// Start listening for events
	go sub.listen()

	e.logger.Info("Successfully subscribed", "eventName", eventName)

	return sub, nil
}

func (s *grpcSubscription) listen() {
	for {
		event, err := s.stream.Recv()
		if err == io.EOF {
			s.engine.logger.Info("Subscription stream ended", "eventName", s.eventName)
			return
		}
		if err != nil {
			s.engine.logger.Error("Subscription error", "eventName", s.eventName, "error", err)
			return
		}

		// Process the event
		processedEvent, err := s.processEvent(event)
		if err != nil {
			s.engine.logger.Error("Failed to process event", "error", err)
			continue
		}

		if processedEvent == nil {
			continue
		}

		// Call all handlers
		handlers := s.GetHandlers()

		for _, handler := range handlers {
			if err := handler(processedEvent); err != nil {
				s.engine.logger.Error("Handler error", "error", err)
			}

			// Auto-acknowledge if enabled
			if s.autoAck && processedEvent.Idem != "" && processedEvent.Block != 0 {
				if err := s.Ack(processedEvent.Idem, processedEvent.Block); err != nil {
					s.engine.logger.Error("Auto-acknowledge error", "error", err)
				}
			}
		}
	}
}

func (s *grpcSubscription) processEvent(event *pb.EventStreamResponse) (*EventPayload, error) {
	decryptionKey := s.appSecretKey
	if decryptionKey == "" {
		decryptionKey = s.engine.config.appSecretKey
	}
	if decryptionKey == "" {
		decryptionKey = s.engine.config.clientHash
	}

	if decryptionKey == "" {
		return nil, NewEnSyncError("no decryption key available", ErrTypeGeneric, nil)
	}

	// Decode and decrypt payload
	decodedPayload, err := base64.StdEncoding.DecodeString(event.Payload)
	if err != nil {
		return nil, NewEnSyncError("failed to decode payload", ErrTypeGeneric, err)
	}

	encryptedPayload, err := ParseEncryptedPayload(string(decodedPayload))
	if err != nil {
		return nil, NewEnSyncError("failed to parse encrypted payload", ErrTypeGeneric, err)
	}

	var payloadStr string

	// Check if hybrid encrypted
	if hybridMsg, ok := encryptedPayload.(*HybridEncryptedMessage); ok {
		keyBytes, err := base64.StdEncoding.DecodeString(decryptionKey)
		if err != nil {
			return nil, NewEnSyncError("failed to decode decryption key", ErrTypeGeneric, err)
		}

		payloadStr, err = DecryptHybridMessage(hybridMsg, keyBytes)
		if err != nil {
			return nil, NewEnSyncError("failed to decrypt hybrid message", ErrTypeGeneric, err)
		}
	} else if encMsg, ok := encryptedPayload.(*EncryptedMessage); ok {
		keyBytes, err := base64.StdEncoding.DecodeString(decryptionKey)
		if err != nil {
			return nil, NewEnSyncError("failed to decode decryption key", ErrTypeGeneric, err)
		}

		payloadStr, err = DecryptEd25519(encMsg, keyBytes)
		if err != nil {
			return nil, NewEnSyncError("failed to decrypt message", ErrTypeGeneric, err)
		}
	} else {
		return nil, NewEnSyncError("unknown encrypted payload type", ErrTypeGeneric, nil)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return nil, NewEnSyncError("failed to unmarshal payload", ErrTypeGeneric, err)
	}

	var metadata map[string]interface{}
	if event.Metadata != "" {
		if err := json.Unmarshal([]byte(event.Metadata), &metadata); err != nil {
			metadata = make(map[string]interface{})
		}
	} else {
		metadata = make(map[string]interface{})
	}

	return &EventPayload{
		EventName: event.EventName,
		Idem:      event.EventIdem,
		Block:     event.PartitionBlock,
		Timestamp: time.Now(),
		Payload:   payload,
		Metadata:  metadata,
		Sender:    event.Sender,
	}, nil
}

func (s *grpcSubscription) Ack(eventIdem string, block int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := s.engine.client.AcknowledgeEvent(ctx, &pb.AcknowledgeRequest{
		ClientId:       s.engine.config.clientID,
		EventIdem:      eventIdem,
		EventName:      s.eventName,
		PartitionBlock: block,
	})

	if err != nil {
		return NewEnSyncError("acknowledge failed", ErrTypeGeneric, err)
	}

	if !resp.Success {
		return NewEnSyncError(resp.Message, ErrTypeGeneric, nil)
	}

	return nil
}

func (s *grpcSubscription) Resume() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := s.engine.client.ContinueEvents(ctx, &pb.ContinueRequest{
		ClientId:  s.engine.config.clientID,
		EventName: s.eventName,
	})

	if err != nil {
		return NewEnSyncError("continue failed", ErrTypeContinue, err)
	}

	if !resp.Success {
		return NewEnSyncError(resp.Message, ErrTypeContinue, nil)
	}

	return nil
}

func (s *grpcSubscription) Pause(reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := s.engine.client.PauseEvents(ctx, &pb.PauseRequest{
		ClientId:  s.engine.config.clientID,
		EventName: s.eventName,
		Reason:    reason,
	})

	if err != nil {
		return NewEnSyncError("pause failed", ErrTypePause, err)
	}

	if !resp.Success {
		return NewEnSyncError(resp.Message, ErrTypePause, nil)
	}

	return nil
}

func (s *grpcSubscription) Defer(eventIdem string, delayMs int64, reason string) (*DeferResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := s.engine.client.DeferEvent(ctx, &pb.DeferRequest{
		ClientId:  s.engine.config.clientID,
		EventIdem: eventIdem,
		EventName: s.eventName,
		DelayMs:   delayMs,
		Priority:  0,
		Reason:    reason,
	})

	if err != nil {
		return nil, NewEnSyncError("defer failed", ErrTypeDefer, err)
	}

	if !resp.Success {
		return nil, NewEnSyncError(resp.Message, ErrTypeDefer, nil)
	}

	return &DeferResponse{
		Status:            "success",
		Action:            "deferred",
		EventID:           eventIdem,
		DelayMs:           delayMs,
		ScheduledDelivery: time.UnixMilli(resp.DeliveryTime),
		Timestamp:         time.Now(),
	}, nil
}

func (s *grpcSubscription) Discard(eventIdem string, reason string) (*DiscardResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := s.engine.client.DiscardEvent(ctx, &pb.DiscardRequest{
		ClientId:  s.engine.config.clientID,
		EventIdem: eventIdem,
		EventName: s.eventName,
		Reason:    reason,
	})

	if err != nil {
		return nil, NewEnSyncError("discard failed", ErrTypeDiscard, err)
	}

	if !resp.Success {
		return nil, NewEnSyncError(resp.Message, ErrTypeDiscard, nil)
	}

	return &DiscardResponse{
		Status:    "success",
		Action:    "discarded",
		EventID:   eventIdem,
		Timestamp: time.Now(),
	}, nil
}

func (s *grpcSubscription) Rollback(eventIdem string, block int64) error {
	return NewEnSyncError("rollback not implemented in gRPC version", ErrTypeGeneric, nil)
}

func (s *grpcSubscription) Replay(eventIdem string) (*EventPayload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := s.engine.client.ReplayEvent(ctx, &pb.ReplayRequest{
		ClientId:  s.engine.config.clientID,
		EventIdem: eventIdem,
		EventName: s.eventName,
	})

	if err != nil {
		return nil, NewEnSyncError("replay failed", ErrTypeReplay, err)
	}

	if !resp.Success {
		return nil, NewEnSyncError(resp.Message, ErrTypeReplay, nil)
	}

	// Parse and process the replayed event data
	var eventData pb.EventStreamResponse
	if err := json.Unmarshal([]byte(resp.EventData), &eventData); err != nil {
		return nil, NewEnSyncError("failed to parse replayed event", ErrTypeReplay, err)
	}

	return s.processEvent(&eventData)
}

func (s *grpcSubscription) Unsubscribe() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := s.engine.client.Unsubscribe(ctx, &pb.UnsubscribeRequest{
		ClientId:  s.engine.config.clientID,
		EventName: s.eventName,
	})

	if err != nil {
		return NewEnSyncError("unsubscribe failed", ErrTypeSubscription, err)
	}

	if !resp.Success {
		return NewEnSyncError(resp.Message, ErrTypeSubscription, nil)
	}

	// Cancel the stream context
	s.cancel()

	// Remove from engine's subscriptions
	s.engine.subscriptions.mu.Lock()
	delete(s.engine.subscriptions.subs, s.eventName)
	s.engine.subscriptions.mu.Unlock()

	s.engine.logger.Info("Successfully unsubscribed", "eventName", s.eventName)

	return nil
}

func (e *GRPCEngine) Close() error {
	e.state.mu.Lock()
	e.state.shouldReconnect = false
	e.state.mu.Unlock()

	// Stop heartbeat
	if e.heartbeatCancel != nil {
		e.heartbeatCancel()
	}

	// Close all subscriptions
	e.subscriptions.mu.Lock()
	for _, sub := range e.subscriptions.subs {
		sub.cancel()
	}
	e.subscriptions.subs = make(map[string]*grpcSubscription)
	e.subscriptions.mu.Unlock()

	// Close gRPC connection
	if e.conn != nil {
		return e.conn.Close()
	}

	return nil
}

func (e *GRPCEngine) GetClientPublicKey() string {
	return e.GetClientHash()
}

func (e *GRPCEngine) AnalyzePayload(payload map[string]interface{}) *PayloadMetadata {
	payloadJSON, _ := json.Marshal(payload)
	byteSize := len(payloadJSON)

	skeleton := make(map[string]string)
	for key, value := range payload {
		if value == nil {
			skeleton[key] = "null"
		} else {
			switch value.(type) {
			case []interface{}:
				skeleton[key] = "array"
			case map[string]interface{}:
				skeleton[key] = "object"
			case string:
				skeleton[key] = "string"
			case float64, int, int64:
				skeleton[key] = "number"
			case bool:
				skeleton[key] = "boolean"
			default:
				skeleton[key] = "unknown"
			}
		}
	}

	return &PayloadMetadata{
		ByteSize: byteSize,
		Skeleton: skeleton,
	}
}

func (e *GRPCEngine) startHeartbeat() {
	ctx, cancel := context.WithCancel(e.ctx)
	e.heartbeatCancel = cancel

	go func() {
		ticker := time.NewTicker(e.grpcConfig.heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				e.logger.Info("Heartbeat stopped")
				return
			case <-ticker.C:
				e.state.mu.RLock()
				isAuth := e.state.isAuthenticated
				e.state.mu.RUnlock()

				if !isAuth {
					continue
				}

				hbCtx, hbCancel := context.WithTimeout(e.ctx, 5*time.Second)
				resp, err := e.client.Heartbeat(hbCtx, &pb.HeartbeatRequest{
					ClientId: e.config.clientID,
				})
				hbCancel()

				if err != nil {
					e.logger.Error("Heartbeat failed", "error", err)
				} else if resp.Success {
					e.logger.Debug("Heartbeat successful")
				}
			}
		}
	}()
}
