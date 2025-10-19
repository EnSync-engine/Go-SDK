package websocket

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/gorilla/websocket"

	"github.com/EnSync-engine/Go-SDK/common"
)

const (
	keyValueParts       = 2
	writeChanBufferSize = 256
)

type messageCallback struct {
	resolve chan string
	reject  chan error
}

type WebSocketEngine struct {
	*common.BaseEngine
	conn             *websocket.Conn
	messageCallbacks sync.Map
	writeChan        chan []byte
	connMu           sync.RWMutex
	requestMu        sync.Mutex
	closed           atomic.Bool
	closeMu          sync.Mutex
	subscriptionMgr  *common.SubscriptionManager
	subMgrMu         sync.Mutex
}

type wsSubscription struct {
	*common.BaseSubscription
	autoAck      bool
	appSecretKey string
	eventName    string
	engine       *WebSocketEngine
	closed       atomic.Bool
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

	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, common.NewEnSyncError("invalid WebSocket URL", common.ErrTypeValidation, err)
	}

	dialCtx, cancel := context.WithTimeout(ctx, baseEngine.GetOperationTimeout())
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
	}

	conn, _, err := dialer.DialContext(dialCtx, wsURL, nil)
	if err != nil {
		return nil, common.NewEnSyncError("WebSocket connection failed", common.ErrTypeConnection, err)
	}

	e := &WebSocketEngine{
		BaseEngine:      baseEngine,
		conn:            conn,
		writeChan:       make(chan []byte, writeChanBufferSize),
		subscriptionMgr: nil,
	}

	go e.writePump()
	go e.readPump()

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

	if err := e.authenticate(); err != nil {
		return err
	}
	e.Logger.Info("client created successfully", "clientId", e.ClientID)
	return nil
}

func (e *WebSocketEngine) authenticate() error {
	return e.ExecuteOperation(common.Operation{
		Name: "authenticate",
		Execute: func(ctx context.Context) error {
			authMessage := fmt.Sprintf("CONN;ACCESS_KEY=:%s", e.AccessKey)
			response, err := e.sendRequest(authMessage)
			if err != nil {
				return common.NewEnSyncError("authentication failed", common.ErrTypeAuth, err)
			}

			content := strings.TrimPrefix(response, "+PASS:")
			resp := parseKeyValue(content)
			clientID := resp["clientId"]
			clientHash := resp["clientHash"]

			if clientID == "" || clientHash == "" {
				return common.NewEnSyncError(
					fmt.Sprintf("invalid auth response - clientId: '%s', clientHash: '%s'", clientID, clientHash),
					common.ErrTypeAuth,
					nil,
				)
			}

			e.setAuthenticationResult(clientID, clientHash)
			return nil
		},
	})
}

func (e *WebSocketEngine) setAuthenticationResult(clientID, clientHash string) {
	e.Logger.Info("authentication successful", "clientId", clientID)

	e.ClientID = clientID
	e.ClientHash = clientHash

	e.State.Mu.Lock()
	e.State.IsAuthenticated = true
	e.State.IsConnected = true
	e.State.Mu.Unlock()

	go e.startPingInterval()
}

func (e *WebSocketEngine) writePump() {
	defer func() {
		if r := recover(); r != nil {
			e.Logger.Error("write pump panic", "panic", r)
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
					e.conn.WriteMessage(websocket.CloseMessage, []byte{}) //nolint
				}
				e.connMu.Unlock()
				return
			}

			e.connMu.Lock()
			if e.conn != nil {
				if err := e.conn.WriteMessage(websocket.TextMessage, message); err != nil {
					e.Logger.Error("websocket write error", "error", err)
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

func (e *WebSocketEngine) startPingInterval() {
	ticker := time.NewTicker(e.GetPingIntervalTimeout())
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

			e.connMu.RLock()
			conn := e.conn
			e.connMu.RUnlock()

			if conn != nil {
				deadline := time.Now().Add(e.GetPingIntervalTimeout())
				if err := conn.SetWriteDeadline(deadline); err != nil {
					e.Logger.Error("failed to set write deadline for ping", "error", err)
					e.handleClose()
					return
				}

				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					e.Logger.Error("ping failed", "error", err)
					e.handleClose()
					return
				}
			}
		}
	}
}

func (e *WebSocketEngine) readPump() {
	defer func() {
		if r := recover(); r != nil {
			e.Logger.Error("read pump panic", "panic", r)
		}
	}()
	defer e.handleClose()

	e.connMu.RLock()
	conn := e.conn
	e.connMu.RUnlock()

	if conn != nil {
		conn.SetPongHandler(func(appData string) error {
			return nil
		})
	}

	for {
		select {
		case <-e.Ctx.Done():
			return
		default:
			e.connMu.RLock()
			conn := e.conn
			e.connMu.RUnlock()

			if conn == nil {
				return
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				e.Logger.Error("websocket read error", "error", err)
				e.State.Mu.Lock()
				e.State.IsConnected = false
				e.State.Mu.Unlock()
				return
			}

			msg := string(message)

			if msg == "PING" {
				e.connMu.RLock()
				conn := e.conn
				e.connMu.RUnlock()

				if conn != nil {
					if err := conn.WriteMessage(websocket.TextMessage, []byte("PONG")); err != nil {
						e.Logger.Error("failed to send PONG", "error", err)
						return
					}
				}
				continue
			}

			if strings.HasPrefix(msg, "+RECORD:") {
				e.handleEvent(msg)
				continue
			}

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
	e.Logger.Info("websocket closed")
}

func (e *WebSocketEngine) Publish(
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

	var responses []string

	if useHybridEncryption && len(recipients) > 1 {
		err = e.publishHybrid(eventName, recipients, publishData.PayloadJSON, publishData.MetadataJSON, publishData.PayloadMetaJSON, &responses)
	} else {
		err = e.publishIndividual(
			eventName,
			recipients,
			publishData.PayloadJSON,
			publishData.MetadataJSON,
			publishData.PayloadMetaJSON,
			&responses,
		)
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

	baseMessage := fmt.Sprintf("PUB;CLIENT_ID=:%s;EVENT_NAME=:%s;PAYLOAD=%s;METADATA=:%s;PAYLOAD_METADATA=:%s",
		e.ClientID, eventName, encryptedBase64, string(metadataJSON), string(payloadMetaJSON))

	var eg errgroup.Group
	resultsChan := make(chan string, len(recipients))

	for _, recipient := range recipients {
		eg.Go(func() error {
			fullMessage := fmt.Sprintf("%s;DELIVERY_TO=%s", baseMessage, recipient)
			resp, err := e.sendRequest(fullMessage)
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

func (e *WebSocketEngine) publishIndividual(
	eventName string,
	recipients []string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	responses *[]string,
) error {
	baseMessage := fmt.Sprintf("PUB;CLIENT_ID=:%s;EVENT_NAME=:%s;METADATA=:%s;PAYLOAD_METADATA=:%s",
		e.ClientID, eventName, string(metadataJSON), string(payloadMetaJSON))

	var eg errgroup.Group
	resultsChan := make(chan string, len(recipients))

	for _, recipient := range recipients {
		eg.Go(func() error {
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

			fullMessage := fmt.Sprintf("%s;PAYLOAD=:%s;DELIVERY_TO=:%s", baseMessage, encryptedBase64, recipient)
			resp, err := e.sendRequest(fullMessage)
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

func (e *WebSocketEngine) Subscribe(eventName string, options *common.SubscribeOptions) (common.Subscription, error) {
	if err := e.ValidateSubscribeInput(eventName); err != nil {
		return nil, err
	}

	e.subMgrMu.Lock()
	if e.subscriptionMgr == nil {
		e.subscriptionMgr = common.NewSubscriptionManager(e.Logger)
	}
	e.subMgrMu.Unlock()

	if options == nil {
		options = &common.SubscribeOptions{AutoAck: true}
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
		BaseSubscription: common.NewBaseSubscription(eventName, e.Logger),
		eventName:        eventName,
		autoAck:          options.AutoAck,
		appSecretKey:     options.AppSecretKey,
		engine:           e,
	}

	if err := e.subscriptionMgr.Register(eventName, sub); err != nil {
		return nil, err
	}

	if err := sub.StartWorkerPool(); err != nil {
		e.subscriptionMgr.Unregister(eventName) //nolint:errcheck
		return nil, err
	}

	e.Logger.Info("successfully subscribed", "eventName", eventName)
	return sub, nil
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

func (e *WebSocketEngine) sendRequest(message string) (string, error) {
	e.requestMu.Lock()
	defer e.requestMu.Unlock()
	ctx, cancel := context.WithTimeout(e.Ctx, e.GetOperationTimeout())
	defer cancel()

	return e.sendMessageWithContext(ctx, message)
}

func (e *WebSocketEngine) handleEvent(msg string) {
	event := parseEventMessage(msg)
	if event == nil {
		e.Logger.Error("failed to parse event message", "message", msg)
		return
	}

	// Only handle if subscriptions exist
	if e.subscriptionMgr == nil {
		e.Logger.Error("no subscription found for event", "eventName", event.EventName)
		return
	}

	if val, exists := e.subscriptionMgr.Get(event.EventName); exists {
		sub := val.(*wsSubscription)
		processedEvent, err := sub.decryptEvent(event)
		if err != nil {
			sub.engine.Logger.Error("failed to decrypt event", "error", err)
			return
		}

		if err := sub.SubmitJob(processedEvent); err != nil {
			sub.engine.Logger.Error("failed to submit job", "error", err)
			return
		}

		if sub.autoAck && processedEvent.Idem != "" && processedEvent.Block != 0 {
			if err := sub.Ack(processedEvent.Idem, processedEvent.Block); err != nil {
				sub.engine.Logger.Error("auto-ack error", "eventId", processedEvent.Idem, "error", err)
			}
		}
	} else {
		e.Logger.Error("no subscription found for event", "eventName", event.EventName)
	}
}

func (e *WebSocketEngine) Close() error {
	e.closeMu.Lock()
	defer e.closeMu.Unlock()

	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}

	e.Logger.Info("closing websocket engine")

	e.State.Mu.Lock()
	e.State.IsConnected = false
	e.State.IsAuthenticated = false
	e.State.Mu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), e.GetGracefulShutdownTimeout())
	defer cancel()

	e.subMgrMu.Lock()
	if e.subscriptionMgr != nil {
		if err := e.subscriptionMgr.CloseAll(shutdownCtx); err != nil {
			e.Logger.Warn("error closing subscriptions", "error", err)
		}
	}
	e.subMgrMu.Unlock()

	e.connMu.Lock()
	if e.writeChan != nil {
		close(e.writeChan)
	}
	if e.conn != nil {
		e.conn.Close()
	}
	e.connMu.Unlock()

	e.Logger.Info("websocket engine shutdown complete")
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
		Block    interface{}            `json:"block"`
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
	if record.Metadata == nil {
		record.Metadata = make(map[string]interface{})
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
	if record.Payload != "" {
		record.Metadata["_encryptedPayload"] = record.Payload
	}
	return &common.EventPayload{
		EventName: record.Name,
		Idem:      idem,
		Block:     blockNum,
		Timestamp: time.UnixMilli(record.LoggedAt),
		Payload:   nil,
		Metadata:  record.Metadata,
		Sender:    record.Sender,
	}
}

func (s *wsSubscription) Ack(eventIdem string, block int64) error {
	message := fmt.Sprintf("ACK;CLIENT_ID=:%s;EVENT_IDEM=:%s;BLOCK=:%d;EVENT_NAME=:%s",
		s.engine.ClientID, eventIdem, block, s.eventName)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("ack failed", common.ErrTypeGeneric, err)
	}
	return nil
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
	message := fmt.Sprintf("ROLLBACK;CLIENT_ID=:%s;EVENT_IDEM=:%s;PARTITION_BLOCK=%d",
		s.engine.ClientID, eventIdem, block)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("rollback failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Unsubscribe() error {
	return s.unsubscribe()
}

func (s *wsSubscription) decryptEvent(event *common.EventPayload) (*common.EventPayload, error) {
	encryptedPayloadRaw, exists := event.Metadata["_encryptedPayload"]
	if !exists {
		return event, nil
	}

	encryptedPayloadBase64, ok := encryptedPayloadRaw.(string)
	if !ok || encryptedPayloadBase64 == "" {
		return event, nil
	}

	decryptedPayload, err := s.engine.DecryptEventPayload(encryptedPayloadBase64, s.appSecretKey)
	if err != nil {
		return nil, err
	}

	decryptedEvent := &common.EventPayload{
		EventName: event.EventName,
		Idem:      event.Idem,
		Block:     event.Block,
		Timestamp: event.Timestamp,
		Payload:   decryptedPayload,
		Metadata:  event.Metadata,
		Sender:    event.Sender,
	}

	delete(decryptedEvent.Metadata, "_encryptedPayload")

	return decryptedEvent, nil
}

func (s *wsSubscription) unsubscribe() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	message := fmt.Sprintf("UNSUB;CLIENT_ID=:%s;EVENT_NAME=:%s", s.engine.ClientID, s.eventName)
	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("unsubscribe failed", common.ErrTypeSubscription, err)
	}
	s.engine.subscriptionMgr.Unregister(s.eventName) //nolint:errcheck
	s.StopWorkerPool()
	s.Mu.Lock()
	s.Handlers = nil
	s.Mu.Unlock()

	s.engine.Logger.Info("successfully unsubscribed", "eventName", s.eventName)

	return nil
}

func (e *WebSocketEngine) sendMessageWithContext(ctx context.Context, message string) (string, error) {
	messageID := fmt.Sprintf("%d", time.Now().UnixNano())
	callback := &messageCallback{resolve: make(chan string, 1), reject: make(chan error, 1)}

	e.messageCallbacks.Store(messageID, callback)
	defer e.messageCallbacks.Delete(messageID)

	fullMessage := fmt.Sprintf("%s;MSG_ID=:%s", message, messageID)

	e.State.Mu.RLock()
	isConnected := e.State.IsConnected
	e.State.Mu.RUnlock()

	if !isConnected {
		return "", common.NewEnSyncError("connection not established", common.ErrTypeConnection, nil)
	}

	defer func() {
		if r := recover(); r != nil {
			e.Logger.Error("recovered from panic in sendMessageWithContext", "panic", r)
		}
	}()

	select {
	case e.writeChan <- []byte(fullMessage):
	case <-ctx.Done():
		return "", common.NewEnSyncError("write channel blocked", common.ErrTypeConnection, ctx.Err())
	case <-e.Ctx.Done():
		return "", common.NewEnSyncError("engine context canceled", common.ErrTypeConnection, e.Ctx.Err())
	}

	select {
	case resp := <-callback.resolve:
		return resp, nil
	case err := <-callback.reject:
		return "", err
	case <-ctx.Done():
		return "", common.NewEnSyncError("message timeout", common.ErrTypeConnection, ctx.Err())
	case <-e.Ctx.Done():
		return "", common.NewEnSyncError("engine context canceled", common.ErrTypeConnection, e.Ctx.Err())
	}
}
