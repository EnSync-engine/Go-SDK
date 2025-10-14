package ensync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type wsConfig struct {
	pingInterval      time.Duration
	reconnectInterval time.Duration
}

type messageCallbacks struct {
	mu        sync.RWMutex
	callbacks map[string]*messageCallback
}

type WebSocketEngine struct {
	baseEngine
	conn *websocket.Conn

	wsConfig wsConfig

	subscriptions wsSubscriptions

	messageCallbacks messageCallbacks
}

type wsSubscription struct {
	baseSubscription
	eventName    string
	autoAck      bool
	appSecretKey string
	engine       *WebSocketEngine
}

type messageCallback struct {
	resolve chan string
	reject  chan error
	timeout *time.Timer
}

type WebSocketOption func(*WebSocketEngine)

func WithPingInterval(interval time.Duration) WebSocketOption {
	return func(e *WebSocketEngine) {
		e.wsConfig.pingInterval = interval
	}
}

func WithReconnectInterval(interval time.Duration) WebSocketOption {
	return func(e *WebSocketEngine) {
		e.wsConfig.reconnectInterval = interval
	}
}

func WithWSMaxReconnectAttempts(attempts int) WebSocketOption {
	return func(e *WebSocketEngine) {
		e.config.maxReconnectAttempts = attempts
	}
}

func WithWSReconnectDelay(delay time.Duration) WebSocketOption {
	return func(e *WebSocketEngine) {
		e.config.reconnectDelay = delay
	}
}

func WithWSAutoReconnect(enabled bool) WebSocketOption {
	return func(e *WebSocketEngine) {
		e.config.autoReconnect = enabled
	}
}

func WithWSLogger(logger Logger) WebSocketOption {
	return func(e *WebSocketEngine) {
		e.SetLogger(logger)
	}
}

// WithWSPath allows customizing the WebSocket endpoint path (default: "/message")
func WithWSPath(path string) WebSocketOption {
	return func(e *WebSocketEngine) {
		if parsedURL, err := url.Parse(e.config.url); err == nil {
			parsedURL.Path = path
			e.config.url = parsedURL.String()
		}
	}
}

// WithWSReconnectEnabled controls whether automatic reconnection is enabled
func WithWSReconnectEnabled(enabled bool) WebSocketOption {
	return func(e *WebSocketEngine) {
		e.state.shouldReconnect = enabled
		e.config.autoReconnect = enabled
	}
}

// NewWebSocketEngine creates a new WebSocket engine with the provided context and URL.
// It applies sensible defaults and allows customization through functional options.
func NewWebSocketEngine(ctx context.Context, urlStr string, opts ...WebSocketOption) (*WebSocketEngine, error) {
	if ctx == nil {
		return nil, NewEnSyncError("context cannot be nil", ErrTypeValidation, nil)
	}

	if urlStr == "" {
		return nil, NewEnSyncError("URL cannot be empty", ErrTypeValidation, nil)
	}

	// Parse and validate URL early
	wsURL, err := buildWebSocketURL(urlStr)
	if err != nil {
		return nil, err
	}

	// Create engine with defaults
	engine := &WebSocketEngine{
		wsConfig: wsConfig{
			pingInterval:      30 * time.Second,
			reconnectInterval: 5 * time.Second,
		},
		subscriptions: wsSubscriptions{
			subs: make(map[string]*wsSubscription),
		},
		messageCallbacks: messageCallbacks{
			callbacks: make(map[string]*messageCallback),
		},
	}

	// Initialize base engine
	engine.SetContext(ctx)
	engine.config.url = wsURL
	engine.state.shouldReconnect = true

	// Apply custom options
	for _, opt := range opts {
		if opt != nil {
			opt(engine)
		}
	}

	return engine, nil
}

// buildWebSocketURL converts HTTP/HTTPS URLs to WebSocket URLs and adds the message path.
// It validates the URL format and ensures proper WebSocket scheme conversion.
func buildWebSocketURL(urlStr string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", NewEnSyncError("invalid URL format", ErrTypeConnection, err)
	}

	// Validate and convert scheme
	switch parsedURL.Scheme {
	case "http":
		parsedURL.Scheme = "ws"
	case "https":
		parsedURL.Scheme = "wss"
	case "ws", "wss":
		// Already correct WebSocket schemes
	default:
		return "", NewEnSyncError("unsupported URL scheme: must be http, https, ws, or wss", ErrTypeValidation, nil)
	}

	// Set the WebSocket message endpoint
	parsedURL.Path = "/message"

	return parsedURL.String(), nil
}

func (e *WebSocketEngine) CreateClient(accessKey string, options ...ClientOption) error {
	cfg := &clientConfig{}
	for _, opt := range options {
		opt(cfg)
	}

	e.config.accessKey = accessKey
	if cfg.appSecretKey != "" {
		e.config.appSecretKey = cfg.appSecretKey
	}

	return e.connect()
}

func (e *WebSocketEngine) connect() error {
	e.logger.Info("Connecting to WebSocket", "url", e.config.url)

	conn, _, err := websocket.DefaultDialer.Dial(e.config.url, nil)
	if err != nil {
		return NewEnSyncError("connection failed", ErrTypeConnection, err)
	}

	e.conn = conn

	e.logger.Info("WebSocket connection established")

	e.state.mu.Lock()
	e.state.isConnected = true
	e.state.reconnectAttempts = 0
	e.state.mu.Unlock()

	// Start message handler
	go e.handleMessages()

	// Start ping interval
	go e.startPingInterval()

	// Authenticate
	e.logger.Info("Attempting authentication")
	return e.authenticate()
}

func (e *WebSocketEngine) authenticate() error {
	authMessage := fmt.Sprintf("CONN;ACCESS_KEY=:%s", e.config.accessKey)
	response, err := e.sendMessage(authMessage)
	if err != nil {
		return NewEnSyncError("authentication failed", ErrTypeAuth, err)
	}

	if !strings.HasPrefix(response, "+PASS:") {
		return NewEnSyncError("authentication failed: "+response, ErrTypeAuth, nil)
	}

	e.logger.Info("Authentication successful")

	content := strings.TrimPrefix(response, "+PASS:")
	resp := parseKeyValue(content)

	e.config.clientID = resp["clientId"]
	e.config.clientHash = resp["clientHash"]

	e.state.mu.Lock()
	e.state.isAuthenticated = true
	e.state.mu.Unlock()

	return nil
}

func (e *WebSocketEngine) Publish(eventName string, recipients []string, payload map[string]interface{}, metadata *EventMetadata, options *PublishOptions) (string, error) {
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

		for _, recipient := range recipients {
			message := fmt.Sprintf("PUB;CLIENT_ID=:%s;EVENT_NAME=:%s;PAYLOAD=:%s;DELIVERY_TO=:%s;METADATA=:%s;PAYLOAD_METADATA=:%s",
				e.config.clientID, eventName, encryptedBase64, recipient, string(metadataJSON), string(payloadMetaJSON))

			resp, err := e.sendMessage(message)
			if err != nil {
				return "", err
			}
			responses = append(responses, resp)
		}
	} else {
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

			message := fmt.Sprintf("PUB;CLIENT_ID=:%s;EVENT_NAME=:%s;PAYLOAD=:%s;DELIVERY_TO=:%s;METADATA=:%s;PAYLOAD_METADATA=:%s",
				e.config.clientID, eventName, encryptedBase64, recipient, string(metadataJSON), string(payloadMetaJSON))

			resp, err := e.sendMessage(message)
			if err != nil {
				return "", err
			}
			responses = append(responses, resp)
		}
	}

	return strings.Join(responses, ","), nil
}

func (e *WebSocketEngine) Subscribe(eventName string, options *SubscribeOptions) (Subscription, error) {
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

	message := fmt.Sprintf("SUB;CLIENT_ID=:%s;EVENT_NAME=:%s", e.config.clientID, eventName)
	response, err := e.sendMessage(message)
	if err != nil {
		return nil, NewEnSyncError("subscription failed", ErrTypeSubscription, err)
	}

	if !strings.HasPrefix(response, "+PASS:") {
		return nil, NewEnSyncError("subscription failed: "+response, ErrTypeSubscription, nil)
	}

	sub := &wsSubscription{
		baseSubscription: baseSubscription{
			handlers: make([]EventHandler, 0),
		},
		eventName:    eventName,
		autoAck:      options.AutoAck,
		appSecretKey: options.AppSecretKey,
		engine:       e,
	}

	e.subscriptions.mu.Lock()
	e.subscriptions.subs[eventName] = sub
	e.subscriptions.mu.Unlock()

	e.logger.Info("Successfully subscribed", "eventName", eventName)

	return sub, nil
}

func (e *WebSocketEngine) sendMessage(message string) (string, error) {
	messageID := fmt.Sprintf("%d", time.Now().UnixNano())

	callback := &messageCallback{
		resolve: make(chan string, 1),
		reject:  make(chan error, 1),
	}

	callback.timeout = time.AfterFunc(30*time.Second, func() {
		e.messageCallbacks.mu.Lock()
		delete(e.messageCallbacks.callbacks, messageID)
		e.messageCallbacks.mu.Unlock()

		select {
		case callback.reject <- NewEnSyncError("message timeout", ErrTypeTimeout, nil):
		default:
		}
	})

	e.messageCallbacks.mu.Lock()
	e.messageCallbacks.callbacks[messageID] = callback
	e.messageCallbacks.mu.Unlock()

	if err := e.conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		callback.timeout.Stop()
		e.messageCallbacks.mu.Lock()
		delete(e.messageCallbacks.callbacks, messageID)
		e.messageCallbacks.mu.Unlock()
		return "", NewEnSyncError("failed to send message", ErrTypeConnection, err)
	}

	select {
	case resp := <-callback.resolve:
		return resp, nil
	case err := <-callback.reject:
		return "", err
	}
}

func (e *WebSocketEngine) handleMessages() {
	for {
		_, message, err := e.conn.ReadMessage()
		if err != nil {
			e.logger.Error("WebSocket read error", "error", err)
			e.handleClose()
			return
		}

		msg := string(message)

		// Handle PING
		if msg == "PING" {
			e.conn.WriteMessage(websocket.TextMessage, []byte("PONG"))
			continue
		}

		// Handle event messages
		if strings.HasPrefix(msg, "+RECORD:") {
			e.handleEventMessage(msg)
			continue
		}

		// Handle responses
		if strings.HasPrefix(msg, "+PASS:") || strings.HasPrefix(msg, "+REPLAY:") || strings.HasPrefix(msg, "-FAIL:") {
			e.messageCallbacks.mu.Lock()
			if len(e.messageCallbacks.callbacks) > 0 {
				// Get the oldest callback
				var oldestID string
				var oldestTime int64 = 9223372036854775807

				for id := range e.messageCallbacks.callbacks {
					// Parse timestamp from ID
					var ts int64
					fmt.Sscanf(id, "%d", &ts)
					if ts < oldestTime {
						oldestTime = ts
						oldestID = id
					}
				}

				if oldestID != "" {
					callback := e.messageCallbacks.callbacks[oldestID]
					callback.timeout.Stop()
					delete(e.messageCallbacks.callbacks, oldestID)
					e.messageCallbacks.mu.Unlock()

					if strings.HasPrefix(msg, "-FAIL:") {
						select {
						case callback.reject <- NewEnSyncError(strings.TrimPrefix(msg, "-FAIL:"), ErrTypeGeneric, nil):
						default:
						}
					} else {
						select {
						case callback.resolve <- msg:
						default:
						}
					}
				} else {
					e.messageCallbacks.mu.Unlock()
				}
			} else {
				e.messageCallbacks.mu.Unlock()
			}
		}
	}
}

func (e *WebSocketEngine) handleEventMessage(msg string) {
	event := parseEventMessage(msg)
	if event == nil {
		return
	}

	e.subscriptions.mu.RLock()
	sub, exists := e.subscriptions.subs[event.EventName]
	e.subscriptions.mu.RUnlock()

	if !exists {
		return
	}

	// Decrypt the event
	processedEvent, err := sub.decryptEvent(event)
	if err != nil {
		sub.engine.logger.Error("Failed to decrypt event", "error", err)
		return
	}

	// Call handlers
	handlers := sub.GetHandlers()

	for _, handler := range handlers {
		if err := handler(processedEvent); err != nil {
			sub.engine.logger.Error("Handler error", "error", err)
		}

		if sub.autoAck && processedEvent.Idem != "" && processedEvent.Block != 0 {
			if err := sub.Ack(processedEvent.Idem, processedEvent.Block); err != nil {
				sub.engine.logger.Error("Auto-acknowledge error", "error", err)
			}
		}
	}
}

func (e *WebSocketEngine) handleClose() {
	e.state.mu.Lock()
	e.state.isConnected = false
	e.state.isAuthenticated = false
	e.state.mu.Unlock()

	e.logger.Info("WebSocket closed")
}

func (e *WebSocketEngine) startPingInterval() {
	ticker := time.NewTicker(e.wsConfig.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.state.mu.RLock()
			isConnected := e.state.isConnected
			e.state.mu.RUnlock()

			if !isConnected {
				continue
			}

			if err := e.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				e.logger.Error("Ping failed", "error", err)
				e.handleClose()
			}
		}
	}
}

func (e *WebSocketEngine) Close() error {
	e.state.mu.Lock()
	e.state.shouldReconnect = false
	e.state.mu.Unlock()

	if e.conn != nil {
		return e.conn.Close()
	}

	return nil
}

func (e *WebSocketEngine) GetClientPublicKey() string {
	return e.GetClientHash()
}

func (e *WebSocketEngine) AnalyzePayload(payload map[string]interface{}) *PayloadMetadata {
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

func parseKeyValue(data string) map[string]string {
	result := make(map[string]string)

	// Remove curly braces if present
	data = strings.TrimPrefix(data, "{")
	data = strings.TrimSuffix(data, "}")

	items := strings.Split(data, ",")
	for _, item := range items {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return result
}

func parseEventMessage(message string) *EventPayload {
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

	return &EventPayload{
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
		s.engine.config.clientID, eventIdem, s.eventName, block)

	_, err := s.engine.sendMessage(message)
	return err
}

func (s *wsSubscription) Resume() error {
	message := fmt.Sprintf("CONTINUE;CLIENT_ID=:%s;EVENT_NAME=:%s", s.engine.config.clientID, s.eventName)
	_, err := s.engine.sendMessage(message)
	return err
}

func (s *wsSubscription) Pause(reason string) error {
	message := fmt.Sprintf("PAUSE;CLIENT_ID=:%s;EVENT_NAME=:%s;REASON=:%s",
		s.engine.config.clientID, s.eventName, reason)
	_, err := s.engine.sendMessage(message)
	return err
}

func (s *wsSubscription) Defer(eventIdem string, delayMs int64, reason string) (*DeferResponse, error) {
	message := fmt.Sprintf("DEFER;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s;DELAY=:%d;REASON=:%s",
		s.engine.config.clientID, eventIdem, s.eventName, delayMs, reason)

	_, err := s.engine.sendMessage(message)
	if err != nil {
		return nil, err
	}

	return &DeferResponse{
		Status:            "success",
		Action:            "deferred",
		EventID:           eventIdem,
		DelayMs:           delayMs,
		ScheduledDelivery: time.Now().Add(time.Duration(delayMs) * time.Millisecond),
		Timestamp:         time.Now(),
	}, nil
}

func (s *wsSubscription) Discard(eventIdem string, reason string) (*DiscardResponse, error) {
	message := fmt.Sprintf("DISCARD;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s;REASON=:%s",
		s.engine.config.clientID, eventIdem, s.eventName, reason)

	_, err := s.engine.sendMessage(message)
	if err != nil {
		return nil, err
	}

	return &DiscardResponse{
		Status:    "success",
		Action:    "discarded",
		EventID:   eventIdem,
		Timestamp: time.Now(),
	}, nil
}

func (s *wsSubscription) Rollback(eventIdem string, block int64) error {
	message := fmt.Sprintf("ROLLBACK;CLIENT_ID=:%s;EVENT_IDEM=:%s;PARTITION_BLOCK=:%d",
		s.engine.config.clientID, eventIdem, block)

	_, err := s.engine.sendMessage(message)
	return err
}

func (s *wsSubscription) Replay(eventIdem string) (*EventPayload, error) {
	message := fmt.Sprintf("REPLAY;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s",
		s.engine.config.clientID, eventIdem, s.eventName)

	response, err := s.engine.sendMessage(message)
	if err != nil {
		return nil, err
	}

	event := parseEventMessage(response)
	if event == nil {
		return nil, NewEnSyncError("failed to parse replayed event", ErrTypeReplay, nil)
	}

	return s.decryptEvent(event)
}

func (s *wsSubscription) Unsubscribe() error {
	message := fmt.Sprintf("UNSUB;CLIENT_ID=:%s;EVENT_NAME=:%s", s.engine.config.clientID, s.eventName)

	response, err := s.engine.sendMessage(message)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(response, "+PASS:") {
		return NewEnSyncError("unsubscribe failed: "+response, ErrTypeSubscription, nil)
	}

	s.engine.subscriptions.mu.Lock()
	delete(s.engine.subscriptions.subs, s.eventName)
	s.engine.subscriptions.mu.Unlock()

	s.engine.logger.Info("Successfully unsubscribed", "eventName", s.eventName)

	return nil
}

func (s *wsSubscription) decryptEvent(event *EventPayload) (*EventPayload, error) {
	// This is a placeholder - implement full decryption logic
	return event, nil
}
