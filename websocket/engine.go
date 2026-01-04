package websocket

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/EnSync-engine/Go-SDK/common"
)

const (
	keyValueParts       = 2
	writeChanBufferSize = 256
)

type WebSocketEngine struct {
	*common.BaseEngine
	conn             *websocket.Conn
	messageCallbacks sync.Map
	writeChan        chan []byte
	connMu           sync.RWMutex
	requestMu        sync.Mutex
	closeOnce        sync.Once
	closed           atomic.Bool
}

type wsSubscription struct {
	*common.BaseSubscription
	engine       *WebSocketEngine
	appSecretKey string
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
		writeChan:        make(chan []byte, writeChanBufferSize),
	}

	if err := e.connect(wsURL); err != nil {
		return nil, common.NewEnSyncError("failed to establish WebSocket connection", common.ErrTypeConnection, err)
	}

	return e, nil
}

func (e *WebSocketEngine) CreateClient(accessKey string) error {
	e.AccessKey = accessKey

	if err := e.authenticate(); err != nil {
		return err
	}
	return nil
}

func (e *WebSocketEngine) connect(wsURL string) error {
	e.connMu.Lock()
	defer e.connMu.Unlock()

	if e.conn != nil {
		return nil
	}

	u, err := url.Parse(wsURL)
	if err != nil {
		return common.NewEnSyncError("invalid WebSocket URL", common.ErrTypeValidation, err)
	}

	ctx, cancel := context.WithTimeout(e.Ctx, e.GetConnectionTimeout())
	defer cancel()

	dialer := &websocket.Dialer{
		HandshakeTimeout: e.GetConnectionTimeout(),
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
	}

	if u.Scheme == schemeWSS {
		dialer.TLSClientConfig = &tls.Config{
			ServerName: u.Hostname(),
			MinVersion: tls.VersionTLS12,
		}
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return common.NewEnSyncError("WebSocket connection failed", common.ErrTypeConnection, err)
	}

	e.conn = conn
	e.Logger.Info("WebSocket connection established", zap.String("url", wsURL))

	e.SetConnectionState(true)

	go e.writePump()
	go e.readPump()
	go e.startPingInterval()

	return nil
}

func (e *WebSocketEngine) authenticate() error {
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

	clientId := resp["clientId"]
	clientHash := resp["clientHash"]

	e.SetAuthenticated(clientId, clientHash)

	return nil
}

func (e *WebSocketEngine) startPingInterval() {
	ticker := time.NewTicker(e.GetPingInterval())
	defer ticker.Stop()

	for {
		select {
		case <-e.Ctx.Done():
			return
		case <-ticker.C:
			if !e.IsConnected() {
				continue
			}

			e.connMu.Lock()
			if e.conn != nil {
				if err := e.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					e.Logger.Error("Ping failed", zap.Error(err))
					e.handleClose()
				}
			}
			e.connMu.Unlock()
		}
	}
}

func (e *WebSocketEngine) writePump() {
	defer func() {
		if r := recover(); r != nil {
			e.Logger.Error("write pump panic", zap.Any("panic", r))
		}
	}()
	defer func() {
		e.connMu.Lock()
		if e.conn != nil {
			e.conn.Close()
		}
		e.connMu.Unlock()
		e.handleClose()
	}()

	for {
		select {
		case message, ok := <-e.writeChan:
			if !ok {
				e.connMu.Lock()
				if e.conn != nil {
					e.conn.WriteMessage(websocket.CloseMessage, []byte{})
				}
				e.connMu.Unlock()
				return
			}

			e.connMu.Lock()
			if e.conn != nil {
				if err := e.conn.WriteMessage(websocket.TextMessage, message); err != nil {
					e.Logger.Error("websocket write error", zap.Error(err))
					e.connMu.Unlock()
					return
				}
			} else {
				e.connMu.Unlock()
				return
			}
			e.connMu.Unlock()

		case <-e.Ctx.Done():
			return
		}
	}
}

// readPump handles reading messages from the WebSocket connection
func (e *WebSocketEngine) readPump() {
	defer func() {
		if r := recover(); r != nil {
			e.Logger.Error("Message handler panic",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	for {
		select {
		case <-e.Ctx.Done():
			return
		default:
			if e.closed.Load() {
				return
			}

			_, message, err := e.conn.ReadMessage()
			if err != nil {
				if !e.closed.Load() {
					e.Logger.Error("WebSocket read error", zap.Error(err))
					e.handleClose()
				}
				return
			}

			msg := string(message)

			if msg == "PING" {
				if err := e.conn.WriteMessage(websocket.TextMessage, []byte("PONG")); err != nil {
					e.Logger.Error("Failed to send PONG", "error", err)
				}
				continue
			}

			if strings.HasPrefix(msg, "+RECORD:") || strings.HasPrefix(msg, "+REPLAY:") {
				e.handleEventMessage(msg)
				continue
			}

			if strings.HasPrefix(msg, "+PASS:") || strings.HasPrefix(msg, "-FAIL:") {
				e.handleResponseMessage(msg)
			}
		}
	}
}

func (e *WebSocketEngine) handleClose() {
	e.SetConnectionState(false)
	e.Logger.Info("WebSocket connection closed")
}

func (e *WebSocketEngine) Publish(
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

func (e *WebSocketEngine) preparePublishData(
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

func (e *WebSocketEngine) executePublish(
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

func (e *WebSocketEngine) publishHybrid(
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
		message := fmt.Sprintf("PUB;CLIENT_ID=:%s;EVENT_NAME=:%s;PAYLOAD=:%s;DELIVERY_TO=:%s;METADATA=:%s;PAYLOAD_METADATA=:%s",
			e.ClientID, eventName, encryptedBase64, recipient, string(metadataJSON), string(payloadMetaJSON))

		resp, err := e.sendRequest(message)
		if err != nil {
			return err
		}
		responseChan <- resp
	}
	return nil
}

func (e *WebSocketEngine) publishIndividual(
	eventName string,
	recipients []string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	responseChan chan<- string,
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
		responseChan <- resp
	}
	return nil
}

func (e *WebSocketEngine) Subscribe(messageName string, options *common.SubscribeOptions) (common.Subscription, error) {
	if !e.IsConnected() {
		return nil, common.NewEnSyncError("not authenticated", common.ErrTypeAuth, nil)
	}

	if e.SubscriptionMgr.Exists(messageName) {
		return nil, common.NewEnSyncError("already subscribed to message "+messageName, common.ErrTypeSubscription, nil)
	}

	if options == nil {
		options = &common.SubscribeOptions{
			AutoAck: true,
		}
	}

	message := fmt.Sprintf("SUB;CLIENT_ID=:%s;EVENT_NAME=:%s", e.ClientID, messageName)
	response, err := e.sendMessage(message)
	if err != nil {
		return nil, common.NewEnSyncError("subscription request failed", common.ErrTypeSubscription, err)
	}

	if !strings.HasPrefix(response, "+PASS:") {
		return nil, common.NewEnSyncError("subscription failed: "+response, common.ErrTypeSubscription, nil)
	}

	sub := &wsSubscription{
		BaseSubscription: common.NewBaseSubscription(messageName, options.AutoAck, 10),
		engine:           e,
		appSecretKey:     options.AppSecretKey,
	}

	e.SubscriptionMgr.Store(messageName, sub)

	e.Logger.Info("Successfully subscribed", zap.String("messageName", messageName))
	return sub, nil
}

func (e *WebSocketEngine) sendMessage(message string) (string, error) {
	messageID := fmt.Sprintf("%d", time.Now().UnixNano())
	fullMessage := fmt.Sprintf("%s;MSG_ID=:%s", message, messageID)

	callback := &messageCallback{
		resolve: make(chan string, 1),
		reject:  make(chan error, 1),
	}

	e.messageCallbacks.Store(messageID, callback)
	defer e.messageCallbacks.Delete(messageID)

	select {
	case e.writeChan <- []byte(fullMessage):
	case <-time.After(e.GetOperationTimeout()):
		return "", common.NewEnSyncError("write channel blocked (timeout)", common.ErrTypeTimeout, nil)
	case <-e.Ctx.Done():
		return "", common.NewEnSyncError("write channel blocked", common.ErrTypeConnection, e.Ctx.Err())
	}

	select {
	case resp := <-callback.resolve:
		return resp, nil
	case err := <-callback.reject:
		return "", err
	case <-time.After(e.GetOperationTimeout()):
		return "", common.NewEnSyncError("operation timed out", common.ErrTypeTimeout, nil)
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
		return false
	})
}

func (e *WebSocketEngine) handleEventMessage(msg string) {
	message := parseEventMessage(msg)
	if message == nil {
		return
	}

	if val, exists := e.SubscriptionMgr.Load(message.MessageName); exists {
		sub := val.(*wsSubscription)

		processedMessage, err := sub.processMessage(message)
		if err != nil {
			e.Logger.Error("Message processing failed",
				zap.String("messageName", sub.MessageName),
				zap.String("messageId", message.MessageIdem),
				zap.Error(err))
			return
		}
		if processedMessage == nil {
			return
		}

		sub.BaseSubscription.ProcessMessage(processedMessage, e.Logger, sub.Ack)
	}
}

func (e *WebSocketEngine) Close() error {
	var closeErr error

	e.closeOnce.Do(func() {
		e.closed.Store(true)
		e.connMu.Lock()
		defer e.connMu.Unlock()

		if e.conn != nil {
			e.SubscriptionMgr.Range(func(key, value interface{}) bool {
				if sub, ok := value.(*wsSubscription); ok {
					if err := sub.Unsubscribe(); err != nil {
						e.Logger.Error("Failed to unsubscribe", "error", err)
					}
				}
				return true
			})

			if e.writeChan != nil {
				close(e.writeChan)
			}

			closeErr = e.conn.Close()
			e.conn = nil

			e.State.Mu.Lock()
			e.State.IsConnected = false
			e.State.IsAuthenticated = false
			e.State.Mu.Unlock()
		}
	})

	if closeErr != nil {
		return closeErr
	}
	e.Logger.Info("WebSocket closed")
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

type wsMessageEvent struct {
	MessageName    string
	MessageIdem    string
	PartitionBlock int64
	Payload        string
	Metadata       string
	Sender         string
	LoggedAt       int64
}

func parseEventMessage(message string) *wsMessageEvent {
	if !strings.HasPrefix(message, "+RECORD:") && !strings.HasPrefix(message, "+REPLAY:") {
		return nil
	}

	content := strings.TrimPrefix(message, "+RECORD:")
	content = strings.TrimPrefix(content, "+REPLAY:")

	var record struct {
		Name     string      `json:"name"`
		Idem     string      `json:"idem"`
		ID       string      `json:"id"`
		Block    interface{} `json:"block"`
		Payload  string      `json:"payload"`
		LoggedAt int64       `json:"loggedAt"`
		Metadata interface{} `json:"metadata"`
		Sender   string      `json:"sender"`
	}

	if err := json.Unmarshal([]byte(content), &record); err != nil {
		return nil
	}

	idem := record.Idem
	if idem == "" {
		idem = record.ID
	}

	var blockNum int64
	switch v := record.Block.(type) {
	case string:
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			blockNum = parsed
		} else {
			return nil
		}
	case float64:
		blockNum = int64(v)
	case int64:
		blockNum = v
	case int:
		blockNum = int64(v)
	default:
		return nil
	}

	var metadataStr string
	if record.Metadata != nil {
		if str, ok := record.Metadata.(string); ok {
			metadataStr = str
		} else {
			if metadataBytes, err := json.Marshal(record.Metadata); err == nil {
				metadataStr = string(metadataBytes)
			}
		}
	}

	return &wsMessageEvent{
		MessageName:    record.Name,
		MessageIdem:    idem,
		PartitionBlock: blockNum,
		Payload:        record.Payload,
		Metadata:       metadataStr,
		Sender:         record.Sender,
		LoggedAt:       record.LoggedAt,
	}
}

func (s *wsSubscription) Ack(messageName string, messageIdem string, block int64) error {
	message := fmt.Sprintf("ACK;CLIENT_ID=:%s;EVENT_IDEM=:%s;BLOCK=:%d;EVENT_NAME=:%s",
		s.engine.ClientID, messageIdem, block, messageName)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("ack failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Defer(messageIdem string, delayMs int64, reason string) (*common.DeferResponse, error) {
	message := fmt.Sprintf("DEFER;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s;DELAY=:%d;REASON=:%s",
		s.engine.ClientID, messageIdem, s.MessageName, delayMs, reason)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return nil, common.NewEnSyncError("defer failed", common.ErrTypeGeneric, err)
	}

	return &common.DeferResponse{
		Status:            "success",
		Action:            "deferred",
		MessageID:         messageIdem,
		DelayMs:           delayMs,
		ScheduledDelivery: time.Now().Add(time.Duration(delayMs) * time.Millisecond),
		Timestamp:         time.Now(),
	}, nil
}

func (s *wsSubscription) Discard(messageIdem string, reason string) (*common.DiscardResponse, error) {
	message := fmt.Sprintf("DISCARD;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s;REASON=:%s",
		s.engine.ClientID, messageIdem, s.MessageName, reason)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return nil, common.NewEnSyncError("discard failed", common.ErrTypeGeneric, err)
	}

	return &common.DiscardResponse{
		Status:    "success",
		Action:    "discarded",
		MessageID: messageIdem,
		Timestamp: time.Now(),
	}, nil
}

func (s *wsSubscription) Pause(reason string) error {
	message := fmt.Sprintf("PAUSE;CLIENT_ID=:%s;EVENT_NAME=:%s;REASON=:%s",
		s.engine.ClientID, s.MessageName, reason)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("pause failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Resume() error {
	message := fmt.Sprintf("CONTINUE;CLIENT_ID=:%s;EVENT_NAME=:%s", s.engine.ClientID, s.MessageName)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("resume failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Replay(messageIdem string) (*common.MessagePayload, error) {
	message := fmt.Sprintf("REPLAY;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s",
		s.engine.ClientID, messageIdem, s.MessageName)

	response, err := s.engine.sendRequest(message)
	if err != nil {
		return nil, common.NewEnSyncError("replay failed", common.ErrTypeReplay, err)
	}

	parsedMessage := parseEventMessage(response)
	if parsedMessage == nil {
		return nil, common.NewEnSyncError("failed to parse replayed message", common.ErrTypeReplay, nil)
	}

	return s.processMessage(parsedMessage)
}

func (s *wsSubscription) Rollback(messageIdem string, block int64) error {
	message := fmt.Sprintf("ROLLBACK;CLIENT_ID=:%s;EVENT_IDEM=:%s;PARTITION_BLOCK=:%d",
		s.engine.ClientID, messageIdem, block)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("rollback failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Unsubscribe() error {
	message := fmt.Sprintf("UNSUB;CLIENT_ID=:%s;EVENT_NAME=:%s", s.engine.ClientID, s.MessageName)

	response, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("unsubscribe failed", common.ErrTypeSubscription, err)
	}

	if !strings.HasPrefix(response, "+PASS:") {
		return common.NewEnSyncError("unsubscribe failed: "+response, common.ErrTypeSubscription, nil)
	}

	s.engine.SubscriptionMgr.Delete(s.MessageName)
	s.engine.Logger.Info("Successfully unsubscribed", zap.String("eventName", s.MessageName))
	return nil
}

func (s *wsSubscription) processMessage(message *wsMessageEvent) (*common.MessagePayload, error) {
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
