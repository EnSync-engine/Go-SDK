package websocket

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/EnSync-engine/Go-SDK/common"
)

const (
	defaultConnectionTimeout = 10 * time.Second

	pingInterval = 30 * time.Second

	keyValueParts = 2
)

type WebSocketEngine struct {
	*common.BaseEngine
	conn             *websocket.Conn
	messageCallbacks sync.Map
	mu               sync.Mutex
}

type wsSubscription struct {
	baseSubscription
	eventName    string
	autoAck      bool
	appSecretKey string
	engine       *WebSocketEngine
}

type baseSubscription struct {
	handlers []common.EventHandler
	mu       sync.RWMutex
}

type messageCallback struct {
	resolve chan string
	reject  chan error
}

func NewWebSocketEngine(
	ctx context.Context,
	endpoint string,
	opts ...common.Option,
) (*WebSocketEngine, error) {
	if ctx == nil {
		return nil, common.NewEnSyncError("context cannot be nil", common.ErrTypeValidation, nil)
	}

	wsURL, err := parseWebSocketURL(endpoint)
	if err != nil {
		return nil, err
	}

	baseEngine, err := common.NewBaseEngine(ctx, opts...)
	if err != nil {
		return nil, common.NewEnSyncError("failed to create base engine", common.ErrTypeConnection, err)
	}

	e := &WebSocketEngine{
		BaseEngine:       baseEngine,
		messageCallbacks: sync.Map{},
	}

	if err := e.connect(wsURL); err != nil {
		return nil, common.NewEnSyncError("failed to establish WebSocket connection", common.ErrTypeConnection, err)
	}

	return e, nil
}

func (e *WebSocketEngine) CreateClient(accessKey string, options ...common.ClientOption) error {
	if accessKey == "" {
		return common.NewEnSyncError("access key cannot be empty", common.ErrTypeValidation, nil)
	}

	e.AccessKey = accessKey

	config := &common.ClientConfig{}
	for _, opt := range options {
		opt(config)
	}

	if config.AppSecretKey != "" {
		e.AppSecretKey = config.AppSecretKey
	}
	if config.ClientID != "" {
		e.ClientID = config.ClientID
	}

	// Ensure WebSocket is connected
	e.State.Mu.RLock()
	isConnected := e.State.IsConnected
	e.State.Mu.RUnlock()

	if !isConnected {
		return common.NewEnSyncError("WebSocket not connected", common.ErrTypeConnection, nil)
	}

	if err := e.authenticate(); err != nil {
		return err
	}
	e.Logger.Info("Client created successfully", zap.String("clientId", e.ClientID))
	return nil
}

func (e *WebSocketEngine) connect(wsURL string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.conn != nil {
		return nil
	}

	u, err := url.Parse(wsURL)
	if err != nil {
		return common.NewEnSyncError("invalid WebSocket URL", common.ErrTypeValidation, err)
	}

	ctx, cancel := context.WithTimeout(e.Ctx, defaultConnectionTimeout)
	defer cancel()

	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
	}

	if u.Scheme == schemeWSS {
		dialer.TLSClientConfig = &tls.Config{
			ServerName: u.Hostname(),
			MinVersion: tls.VersionTLS12,
		}
		e.Logger.Info("Establishing secure WebSocket connection", zap.String("url", wsURL))
	} else {
		e.Logger.Warn("Establishing insecure WebSocket connection - development mode", zap.String("url", wsURL))
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return common.NewEnSyncError("WebSocket connection failed", common.ErrTypeConnection, err)
	}

	e.conn = conn
	e.Logger.Info("WebSocket connection established", zap.String("url", wsURL))

	e.State.Mu.Lock()
	e.State.IsConnected = true
	e.State.Mu.Unlock()

	go e.handleMessages()
	go e.startPingInterval()

	return nil
}

func (e *WebSocketEngine) authenticate() error {
	e.Logger.Info("Attempting authentication")

	// Use custom string protocol
	authMessage := fmt.Sprintf("CONN;ACCESS_KEY=:%s", e.AccessKey)
	response, err := e.sendRequest(authMessage)
	if err != nil {
		return common.NewEnSyncError("authentication failed", common.ErrTypeAuth, err)
	}

	if !strings.HasPrefix(response, "+PASS:") {
		return common.NewEnSyncError("authentication failed: "+response, common.ErrTypeAuth, nil)
	}

	e.Logger.Info("Authentication successful")

	content := strings.TrimPrefix(response, "+PASS:")
	resp := parseKeyValue(content)

	e.ClientID = resp["clientId"]
	e.ClientHash = resp["clientHash"]

	e.State.Mu.Lock()
	e.State.IsAuthenticated = true
	e.State.Mu.Unlock()

	return nil
}

func (e *WebSocketEngine) startPingInterval() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.Ctx.Done():
			return
		case <-ticker.C:
			e.State.Mu.RLock()
			isConnected := e.State.IsConnected
			e.State.Mu.RUnlock()

			if !isConnected {
				continue
			}

			e.mu.Lock()
			if e.conn != nil {
				if err := e.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					e.Logger.Error("Ping failed", zap.Error(err))
					e.handleClose()
				}
			}
			e.mu.Unlock()
		}
	}
}

func (e *WebSocketEngine) handleMessages() {
	defer func() {
		if r := recover(); r != nil {
			e.Logger.Error("Message handler panic", zap.Any("panic", r))
		}
	}()

	for {
		select {
		case <-e.Ctx.Done():
			return
		default:
			_, message, err := e.conn.ReadMessage()
			if err != nil {
				e.Logger.Error("WebSocket read error", zap.Error(err))
				e.handleClose()
				return
			}

			msg := string(message)

			// Handle PING
			if msg == "PING" {
				if err := e.conn.WriteMessage(websocket.TextMessage, []byte("PONG")); err != nil {
					e.Logger.Error("Failed to send PONG", map[string]interface{}{"error": err.Error()})
				}
				continue
			}

			// Handle event messages
			if strings.HasPrefix(msg, "+RECORD:") {
				e.handleEventMessage(msg)
				continue
			}

			// Handle responses
			if strings.HasPrefix(msg, "+PASS:") || strings.HasPrefix(msg, "+REPLAY:") || strings.HasPrefix(msg, "-FAIL:") {
				e.handleResponseMessage(msg)
			}
		}
	}
}

func (e *WebSocketEngine) handleClose() {
	e.State.Mu.Lock()
	e.State.IsConnected = false
	e.State.IsAuthenticated = false
	e.State.Mu.Unlock()

	e.Logger.Info("WebSocket closed")
}

func (e *WebSocketEngine) Publish(
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
		return "", common.NewEnSyncError("not authenticated", common.ErrTypeAuth, nil)
	}

	if len(recipients) == 0 {
		return "", common.NewEnSyncError("recipients required", common.ErrTypeValidation, nil)
	}

	useHybridEncryption := true
	if options != nil {
		useHybridEncryption = options.UseHybridEncryption
	}

	if metadata == nil {
		metadata = &common.EventMetadata{
			Persist: true,
			Headers: make(map[string]string),
		}
	}

	payloadMeta := common.AnalyzePayload(payload)
	payloadMetaJSON, _ := json.Marshal(payloadMeta)

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", common.NewEnSyncError("failed to marshal payload", common.ErrTypePublish, err)
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", common.NewEnSyncError("failed to marshal metadata", common.ErrTypePublish, err)
	}

	var responses []string

	if useHybridEncryption && len(recipients) > 1 {
		err = e.publishHybrid(eventName, recipients, payloadJSON, metadataJSON, payloadMetaJSON, &responses)
	} else {
		err = e.publishIndividual(eventName, recipients, payloadJSON, metadataJSON, payloadMetaJSON, &responses)
	}

	if err != nil {
		return "", err
	}

	return strings.Join(responses, ","), nil
}

func (e *WebSocketEngine) publishHybrid(
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
		message := fmt.Sprintf("PUB;CLIENT_ID=:%s;EVENT_NAME=:%s;PAYLOAD=:%s;DELIVERY_TO=:%s;METADATA=:%s;PAYLOAD_METADATA=:%s",
			e.ClientID, eventName, encryptedBase64, recipient, string(metadataJSON), string(payloadMetaJSON))

		resp, err := e.sendRequest(message)
		if err != nil {
			return err
		}
		*responses = append(*responses, resp)
	}
	return nil
}

func (e *WebSocketEngine) publishIndividual(
	eventName string,
	recipients []string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	responses *[]string,
) error {
	for _, recipient := range recipients {
		recipientKey, err := base64.StdEncoding.DecodeString(recipient)
		if err != nil {
			return common.NewEnSyncError("invalid recipient key", common.ErrTypePublish, err)
		}

		encrypted, err := common.EncryptEd25519(string(payloadJSON), recipientKey)
		if err != nil {
			return common.NewEnSyncError("encryption failed", common.ErrTypePublish, err)
		}

		encryptedJSON, err := json.Marshal(encrypted)
		if err != nil {
			return common.NewEnSyncError("failed to marshal encrypted message", common.ErrTypePublish, err)
		}

		encryptedBase64 := base64.StdEncoding.EncodeToString(encryptedJSON)

		message := fmt.Sprintf("PUB;CLIENT_ID=:%s;EVENT_NAME=:%s;PAYLOAD=:%s;DELIVERY_TO=:%s;METADATA=:%s;PAYLOAD_METADATA=:%s",
			e.ClientID, eventName, encryptedBase64, recipient, string(metadataJSON), string(payloadMetaJSON))

		resp, err := e.sendRequest(message)
		if err != nil {
			return err
		}
		*responses = append(*responses, resp)
	}
	return nil
}

func (e *WebSocketEngine) Subscribe(eventName string, options *common.SubscribeOptions) (common.Subscription, error) {
	e.State.Mu.RLock()
	isAuth := e.State.IsAuthenticated
	e.State.Mu.RUnlock()

	if !isAuth {
		return nil, common.NewEnSyncError("not authenticated", common.ErrTypeAuth, nil)
	}

	if options == nil {
		options = &common.SubscribeOptions{
			AutoAck: true,
		}
	}

	if e.SubscriptionMgr.Exists(eventName) {
		return nil, common.NewEnSyncError("already subscribed to event: "+eventName, common.ErrTypeSubscription, nil)
	}

	message := fmt.Sprintf("SUB;CLIENT_ID=:%s;EVENT_NAME=:%s", e.ClientID, eventName)
	response, err := e.sendRequest(message)
	if err != nil {
		return nil, common.NewEnSyncError("subscription failed", common.ErrTypeSubscription, err)
	}

	if !strings.HasPrefix(response, "+PASS:") {
		return nil, common.NewEnSyncError("subscription failed: "+response, common.ErrTypeSubscription, nil)
	}

	sub := &wsSubscription{
		baseSubscription: baseSubscription{
			handlers: make([]common.EventHandler, 0),
		},
		eventName:    eventName,
		autoAck:      options.AutoAck,
		appSecretKey: options.AppSecretKey,
		engine:       e,
	}

	e.SubscriptionMgr.Store(eventName, sub)
	e.Logger.Info("Successfully subscribed", zap.String("eventName", eventName))

	return sub, nil
}

func (e *WebSocketEngine) sendMessage(message string) (string, error) {
	messageID := fmt.Sprintf("%d", time.Now().UnixNano())

	callback := &messageCallback{
		resolve: make(chan string, 1),
		reject:  make(chan error, 1),
	}

	e.messageCallbacks.Store(messageID, callback)
	defer e.messageCallbacks.Delete(messageID)

	// Send the message
	e.mu.Lock()
	err := e.conn.WriteMessage(websocket.TextMessage, []byte(message))
	e.mu.Unlock()

	if err != nil {
		return "", common.NewEnSyncError("failed to send message", common.ErrTypeConnection, err)
	}

	// Wait for response without timeout - let retry logic handle failures
	select {
	case resp := <-callback.resolve:
		return resp, nil
	case err := <-callback.reject:
		return "", err
	case <-e.Ctx.Done():
		return "", e.Ctx.Err()
	}
}

func (e *WebSocketEngine) sendRequest(message string) (string, error) {
	var response string
	err := e.WithRetry(e.Ctx, func() error {
		resp, err := e.sendMessage(message)
		if err != nil {
			return err
		}
		response = resp
		return nil
	})

	return response, err
}

func (e *WebSocketEngine) handleResponseMessage(msg string) {
	// Handle responses using FIFO approach (oldest callback first)
	e.messageCallbacks.Range(func(key, value interface{}) bool {
		callback := value.(*messageCallback)
		e.messageCallbacks.Delete(key)

		if strings.HasPrefix(msg, "-FAIL:") {
			select {
			case callback.reject <- common.NewEnSyncError(strings.TrimPrefix(msg, "-FAIL:"), common.ErrTypeGeneric, nil):
			default:
			}
		} else {
			select {
			case callback.resolve <- msg:
			default:
			}
		}
		return false // Only handle the first (oldest) callback
	})
}

func (e *WebSocketEngine) handleEventMessage(msg string) {
	event := parseEventMessage(msg)
	if event == nil {
		return
	}

	if val, exists := e.SubscriptionMgr.Load(event.EventName); exists {
		sub := val.(*wsSubscription)

		processedEvent, err := sub.decryptEvent(event)
		if err != nil {
			sub.engine.Logger.Error("Failed to decrypt event", zap.Error(err))
			return
		}

		handlers := sub.GetHandlers()
		for _, handler := range handlers {
			go func(h common.EventHandler) {
				defer func() {
					if r := recover(); r != nil {
						sub.engine.Logger.Error("Handler panic", zap.Any("panic", r))
					}
				}()
				if err := h(processedEvent); err != nil {
					sub.engine.Logger.Error("Event handler error", zap.Error(err))
				}
			}(handler)
		}

		if sub.autoAck && processedEvent.Idem != "" && processedEvent.Block != 0 {
			if err := sub.Ack(processedEvent.Idem, processedEvent.Block); err != nil {
				sub.engine.Logger.Error("Auto-acknowledge error", zap.Error(err))
			}
		}
	}
}

func (e *WebSocketEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.conn != nil {
		e.SubscriptionMgr.Range(func(key, value interface{}) bool {
			if sub, ok := value.(*wsSubscription); ok {
				if err := sub.Unsubscribe(); err != nil {
					e.Logger.Error("Failed to unsubscribe", map[string]interface{}{"error": err.Error()})
				}
			}
			return true
		})

		err := e.conn.Close()
		e.conn = nil

		e.State.Mu.Lock()
		e.State.IsConnected = false
		e.State.IsAuthenticated = false
		e.State.Mu.Unlock()

		return err
	}
	e.Logger.Info("WebSocket already closed")
	return nil
}

func parseKeyValue(data string) map[string]string {
	result := make(map[string]string)

	data = strings.TrimPrefix(data, "{")
	data = strings.TrimSuffix(data, "}")

	items := strings.Split(data, ",")
	for _, item := range items {
		parts := strings.SplitN(item, "=", keyValueParts)
		if len(parts) == keyValueParts {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return result
}

func parseEventMessage(message string) *common.EventPayload {
	if !strings.HasPrefix(message, "+RECORD:") && !strings.HasPrefix(message, "+REPLAY:") {
		return nil
	}

	content := strings.TrimPrefix(message, "+RECORD:")
	content = strings.TrimPrefix(content, "+REPLAY:")

	var record struct {
		Name     string                 `json:"name"`
		Idem     string                 `json:"idem"`
		ID       string                 `json:"id"`
		Block    int64                  `json:"block"`
		Payload  string                 `json:"payload"`
		LoggedAt int64                  `json:"loggedAt"`
		Metadata map[string]interface{} `json:"metadata"`
		Sender   string                 `json:"sender"`
	}

	if err := json.Unmarshal([]byte(content), &record); err != nil {
		return nil
	}

	idem := record.Idem
	if idem == "" {
		idem = record.ID
	}

	return &common.EventPayload{
		EventName: record.Name,
		Idem:      idem,
		Block:     record.Block,
		Timestamp: time.UnixMilli(record.LoggedAt),
		Payload:   nil, // Will be decrypted later
		Metadata:  record.Metadata,
		Sender:    record.Sender,
	}
}

func (s *wsSubscription) Ack(eventIdem string, block int64) error {
	message := fmt.Sprintf("ACK;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s;PARTITION_BLOCK=:%d",
		s.engine.ClientID, eventIdem, s.eventName, block)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("ack failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) AddHandler(handler common.EventHandler) func() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.handlers = append(s.handlers, handler)

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

func (s *wsSubscription) GetHandlers() []common.EventHandler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	handlers := make([]common.EventHandler, len(s.handlers))
	copy(handlers, s.handlers)
	return handlers
}

func (s *wsSubscription) Defer(eventIdem string, delayMs int64, reason string) (*common.DeferResponse, error) {
	message := fmt.Sprintf("DEFER;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s;DELAY=:%d;REASON=:%s",
		s.engine.ClientID, eventIdem, s.eventName, delayMs, reason)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return nil, common.NewEnSyncError("defer failed", common.ErrTypeGeneric, err)
	}

	return &common.DeferResponse{
		Status:            "success",
		Action:            "deferred",
		EventID:           eventIdem,
		DelayMs:           delayMs,
		ScheduledDelivery: time.Now().Add(time.Duration(delayMs) * time.Millisecond),
		Timestamp:         time.Now(),
	}, nil
}

func (s *wsSubscription) Discard(eventIdem, reason string) (*common.DiscardResponse, error) {
	message := fmt.Sprintf("DISCARD;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s;REASON=:%s",
		s.engine.ClientID, eventIdem, s.eventName, reason)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return nil, common.NewEnSyncError("discard failed", common.ErrTypeGeneric, err)
	}

	return &common.DiscardResponse{
		Status:    "success",
		Action:    "discarded",
		EventID:   eventIdem,
		Timestamp: time.Now(),
	}, nil
}

func (s *wsSubscription) Pause(reason string) error {
	message := fmt.Sprintf("PAUSE;CLIENT_ID=:%s;EVENT_NAME=:%s;REASON=:%s",
		s.engine.ClientID, s.eventName, reason)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("pause failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Resume() error {
	message := fmt.Sprintf("CONTINUE;CLIENT_ID=:%s;EVENT_NAME=:%s", s.engine.ClientID, s.eventName)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("resume failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Replay(eventIdem string) (*common.EventPayload, error) {
	message := fmt.Sprintf("REPLAY;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s",
		s.engine.ClientID, eventIdem, s.eventName)

	response, err := s.engine.sendRequest(message)
	if err != nil {
		return nil, common.NewEnSyncError("replay failed", common.ErrTypeReplay, err)
	}

	event := parseEventMessage(response)
	if event == nil {
		return nil, common.NewEnSyncError("failed to parse replayed event", common.ErrTypeReplay, nil)
	}

	return s.decryptEvent(event)
}

func (s *wsSubscription) Rollback(eventIdem string, block int64) error {
	message := fmt.Sprintf("ROLLBACK;CLIENT_ID=:%s;EVENT_IDEM=:%s;PARTITION_BLOCK=:%d",
		s.engine.ClientID, eventIdem, block)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("rollback failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Unsubscribe() error {
	message := fmt.Sprintf("UNSUB;CLIENT_ID=:%s;EVENT_NAME=:%s", s.engine.ClientID, s.eventName)

	response, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("unsubscribe failed", common.ErrTypeSubscription, err)
	}

	if !strings.HasPrefix(response, "+PASS:") {
		return common.NewEnSyncError("unsubscribe failed: "+response, common.ErrTypeSubscription, nil)
	}

	s.engine.SubscriptionMgr.Delete(s.eventName)
	s.engine.Logger.Info("Successfully unsubscribed", zap.String("eventName", s.eventName))
	return nil
}

func (s *wsSubscription) decryptEvent(event *common.EventPayload) (*common.EventPayload, error) {
	// This would implement the full decryption logic
	// For now, returning the event as-is
	return event, nil
}

// AnalyzePayload analyzes a payload and returns metadata
func (e *WebSocketEngine) AnalyzePayload(payload map[string]interface{}) *common.PayloadMetadata {
	return common.AnalyzePayload(payload)
}
