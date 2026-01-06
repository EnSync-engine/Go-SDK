package websocket

import (
	"context"
	"crypto/tls"
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

	wsMsgPing      = "PING"
	wsMsgPong      = "PONG"
	wsPrefixRecord = "+RECORD:"
	wsPrefixReplay = "+REPLAY:"
	wsPrefixPass   = "+PASS:"
	wsPrefixFail   = "-FAIL:"
)

type WebSocketEngine struct {
	*common.BaseEngine
	conn             *websocket.Conn
	wsURL            string
	messageCallbacks sync.Map
	writeChan        chan []byte
	connMu           sync.RWMutex
	closeOnce        sync.Once
	closed           atomic.Bool
	pingRunning      atomic.Bool
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

	if endpoint == "" {
		return nil, common.NewEnSyncError("endpoint cannot be empty", common.ErrTypeValidation, nil)
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
		wsURL:            wsURL,
		messageCallbacks: sync.Map{},
		writeChan:        make(chan []byte, writeChanBufferSize),
	}

	if err := e.connect(wsURL); err != nil {
		return nil, common.NewEnSyncError("failed to establish WebSocket connection", common.ErrTypeConnection, err)
	}

	return e, nil
}

func (e *WebSocketEngine) CreateClient(accessKey string) error {
	e.SetAccessKey(accessKey)

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

	e.SetConnectionState(true)

	go e.writePump()
	go e.readPump()
	go e.startPingInterval()

	return nil
}

func (e *WebSocketEngine) authenticate() error {
	authMessage := fmt.Sprintf("CONN;ACCESS_KEY=:%s", e.GetAccessKey())
	response, err := e.sendRequest(authMessage)
	if err != nil {
		return common.NewEnSyncError("authentication failed", common.ErrTypeAuth, err)
	}

	if !strings.HasPrefix(response, wsPrefixPass) {
		return common.NewEnSyncError("authentication failed: "+response, common.ErrTypeAuth, nil)
	}

	e.Logger().Info("Authentication successful")

	content := strings.TrimPrefix(response, wsPrefixPass)
	resp := parseKeyValue(content)

	clientId := resp["clientId"]

	e.SetAuthenticated(clientId)

	return nil
}

func (e *WebSocketEngine) startPingInterval() {
	if !e.pingRunning.CompareAndSwap(false, true) {
		e.Logger().Debug("Ping goroutine already running")
		return
	}
	defer e.pingRunning.Store(false)

	ticker := time.NewTicker(e.GetPingInterval())
	defer ticker.Stop()

	for {
		select {
		case <-e.Ctx.Done():
			return
		case <-ticker.C:
			if e.GetConnectionContext().Err() != nil {
				e.Logger().Debug("Ping stopping: connection context canceled")
				return
			}

			if !e.IsConnected() {
				continue
			}

			e.connMu.Lock()
			if e.conn != nil {
				if err := e.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					e.Logger().Error("Ping failed", zap.Error(err))
					e.handleClose()
				}
			}
			e.connMu.Unlock()
		}
	}
}

func (e *WebSocketEngine) writePump() {
	ctx := e.GetConnectionContext()

	defer func() {
		if r := recover(); r != nil {
			e.Logger().Error("write pump panic", zap.Any("panic", r))
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
					_ = e.conn.WriteMessage(websocket.CloseMessage, []byte{})
				}
				e.connMu.Unlock()
				return
			}

			e.connMu.Lock()
			if e.conn != nil {
				if err := e.conn.WriteMessage(websocket.TextMessage, message); err != nil {
					e.Logger().Error("websocket write error", zap.Error(err))
					e.connMu.Unlock()
					return
				}
			} else {
				e.connMu.Unlock()
				return
			}
			e.connMu.Unlock()

		case <-ctx.Done():
			return
		case <-e.Ctx.Done():
			return
		}
	}
}

func (e *WebSocketEngine) readPump() {
	ctx := e.GetConnectionContext()

	defer func() {
		if r := recover(); r != nil {
			e.Logger().Error("Message handler panic",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.Ctx.Done():
			return
		default:
			if e.closed.Load() {
				return
			}

			if !e.handleNextMessage() {
				return
			}
		}
	}
}

func (e *WebSocketEngine) handleNextMessage() bool {
	_, message, err := e.conn.ReadMessage()
	if err != nil {
		if !e.closed.Load() {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				e.Logger().Error("WebSocket read error", zap.Error(err))
			} else {
				e.Logger().Info("WebSocket closed", zap.Error(err))
			}
			e.handleClose()
		}
		return false
	}

	msg := string(message)

	if msg == wsMsgPing {
		if err := e.conn.WriteMessage(websocket.TextMessage, []byte(wsMsgPong)); err != nil {
			e.Logger().Error("Failed to send PONG", "error", err)
		}
		return true
	}

	if strings.HasPrefix(msg, wsPrefixRecord) || strings.HasPrefix(msg, wsPrefixReplay) {
		e.handleEventMessage(msg)
		return true
	}

	if strings.HasPrefix(msg, wsPrefixPass) || strings.HasPrefix(msg, wsPrefixFail) {
		e.handleResponseMessage(msg)
	}

	return true
}

func (e *WebSocketEngine) handleClose() {
	e.SetConnectionState(false)

	e.connMu.Lock()
	e.conn = nil
	e.connMu.Unlock()

	e.Logger().Info("WebSocket connection closed")

	if !e.closed.Load() {
		e.Logger().Info("Triggering reconnection")
		go e.reconnect()
	}
}

func (e *WebSocketEngine) reconnect() {
	if e.closed.Load() {
		return
	}

	if !e.Reconnecting.CompareAndSwap(false, true) {
		e.Logger().Debug("Reconnection already in progress")
		return
	}
	defer e.Reconnecting.Store(false)

	e.ReconnectLoop(
		common.DefaultReconnectConfig(),
		func() {
			e.ResetConnectionContext()
		},
		func() error {
			if err := e.connect(e.wsURL); err != nil {
				return err
			}
			if err := e.authenticate(); err != nil {
				return err
			}
			return e.restoreSubscriptions()
		},
	)
}

func (e *WebSocketEngine) restoreSubscriptions() error {
	var failedCount int
	var totalCount int

	e.SubscriptionMgr.Range(func(key, value interface{}) bool {
		sub, ok := value.(*wsSubscription)
		if !ok {
			return true
		}
		totalCount++

		subMessage := fmt.Sprintf("SUB;CLIENT_ID=:%s;EVENT_NAME=:%s", e.GetClientID(), sub.MessageName)
		_, err := e.sendRequest(subMessage)

		if err != nil {
			e.Logger().Error("Failed to restore subscription",
				zap.String("messageName", sub.MessageName),
				zap.Error(err))
			failedCount++
		}
		return true
	})

	if failedCount > 0 {
		return common.NewEnSyncError(
			fmt.Sprintf("failed to restore %d/%d subscriptions", failedCount, totalCount),
			common.ErrTypeSubscription,
			nil,
		)
	}

	return nil
}

func (e *WebSocketEngine) Publish(
	ctx context.Context,
	eventName string,
	recipients []string,
	payload map[string]interface{},
	metadata *common.MessageMetadata,
	options *common.PublishOptions,
) ([]string, error) {
	return common.Publish(e, e.publishToRecipient, ctx, eventName, recipients, payload, metadata, options)
}

func (e *WebSocketEngine) publishToRecipient(
	ctx context.Context,
	eventName, recipient string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	useHybrid bool,
) (string, error) {
	encryptedBase64, err := common.EncryptPayload(payloadJSON, recipient, useHybrid)
	if err != nil {
		return "", err
	}

	clientID := e.GetClientID()

	message := fmt.Sprintf("PUB;CLIENT_ID=:%s;EVENT_NAME=:%s;PAYLOAD=:%s;DELIVERY_TO=:%s;METADATA=:%s;PAYLOAD_METADATA=:%s",
		clientID, eventName, encryptedBase64, recipient, string(metadataJSON), string(payloadMetaJSON))

	return e.sendRequest(message)
}

func (e *WebSocketEngine) sendRequest(message string) (string, error) {
	var response string
	err := e.WithRetry(e.GetConnectionContext(), func() error {
		resp, err := e.sendMessage(message)
		if err != nil {
			return err
		}
		response = resp
		return nil
	})

	return response, err
}

func (e *WebSocketEngine) Subscribe(messageName string, options *common.SubscribeOptions) (common.Subscription, error) {
	if !e.IsConnected() {
		return nil, common.NewEnSyncError("not authenticated", common.ErrTypeAuth, nil)
	}

	if e.SubscriptionMgr.Exists(messageName) {
		return nil, common.NewEnSyncError("already subscribed to message "+messageName, common.ErrTypeSubscription, nil)
	}

	if options != nil && options.AppSecretKey == "" {
		return nil, common.NewEnSyncError("app secret key is required for websockets", common.ErrTypeSubscription, nil)
	}

	message := fmt.Sprintf("SUB;CLIENT_ID=:%s;EVENT_NAME=:%s", e.GetClientID(), messageName)
	response, err := e.sendMessage(message)
	if err != nil {
		return nil, common.NewEnSyncError("subscription request failed", common.ErrTypeSubscription, err)
	}

	if !strings.HasPrefix(response, wsPrefixPass) {
		return nil, common.NewEnSyncError("subscription failed: "+response, common.ErrTypeSubscription, nil)
	}

	sub := &wsSubscription{
		BaseSubscription: common.NewBaseSubscription(messageName, options.AutoAck, e.GetSubscriptionWorkerCount()),
		engine:           e,
		appSecretKey:     options.AppSecretKey,
	}

	e.SubscriptionMgr.Store(messageName, sub)

	e.Logger().Info("Successfully subscribed", zap.String("messageName", messageName))
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

	if e.closed.Load() {
		return "", common.NewEnSyncError("connection is closed", common.ErrTypeConnection, nil)
	}

	select {
	case e.writeChan <- []byte(message):
	case <-time.After(e.GetOperationTimeout()):
		return "", common.NewEnSyncError("write channel blocked (timeout)", common.ErrTypeTimeout, nil)
	case <-e.GetConnectionContext().Done():
		return "", common.NewEnSyncError("write channel blocked", common.ErrTypeConnection, e.GetConnectionContext().Err())
	}

	select {
	case resp := <-callback.resolve:
		return resp, nil
	case err := <-callback.reject:
		return "", err
	case <-time.After(e.GetOperationTimeout()):
		return "", common.NewEnSyncError("operation timed out", common.ErrTypeTimeout, nil)
	case <-e.GetConnectionContext().Done():
		return "", e.GetConnectionContext().Err()
	}
}

func (e *WebSocketEngine) handleResponseMessage(msg string) {
	e.messageCallbacks.Range(func(key, value interface{}) bool {
		callback := value.(*messageCallback)
		e.messageCallbacks.Delete(key)

		if strings.HasPrefix(msg, wsPrefixFail) {
			select {
			case callback.reject <- common.NewEnSyncError(strings.TrimPrefix(msg, wsPrefixFail), common.ErrTypeGeneric, nil):
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
			e.Logger().Error("Message processing failed",
				zap.String("messageName", sub.MessageName),
				zap.String("messageId", message.MessageIdem),
				zap.Error(err))
			return
		}
		if processedMessage == nil {
			return
		}

		logger := e.Logger()

		sub.ProcessMessage(processedMessage, logger, sub.Ack)
	}
}

func (e *WebSocketEngine) Close() error {
	e.closeOnce.Do(func() {
		e.closed.Store(true)
		e.Logger().Info("Shutting down WebSocket engine")

		e.ResetState()

		e.connMu.Lock()
		defer e.connMu.Unlock()

		if e.conn != nil {
			e.SubscriptionMgr.Range(func(key, value interface{}) bool {
				if sub, ok := value.(*wsSubscription); ok {
					sub.Close()
				}
				e.SubscriptionMgr.Delete(key.(string))
				return true
			})

			if e.writeChan != nil {
				close(e.writeChan)
			}

			if err := e.conn.Close(); err != nil {
				e.Logger().Warn("Error closing WebSocket connection", "error", err)
			}
			e.conn = nil
		}

		e.Logger().Info("WebSocket engine shutdown complete")
	})

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
	if !strings.HasPrefix(message, wsPrefixRecord) && !strings.HasPrefix(message, wsPrefixReplay) {
		return nil
	}

	content := strings.TrimPrefix(message, wsPrefixRecord)
	content = strings.TrimPrefix(content, wsPrefixReplay)

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
