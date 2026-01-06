package websocket

import (
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/EnSync-engine/Go-SDK/common"
)

type wsSubscription struct {
	*common.BaseSubscription
	engine       *WebSocketEngine
	appSecretKey string
}

func (s *wsSubscription) Ack(messageIdem string, block int64) error {
	message := fmt.Sprintf("ACK;CLIENT_ID=:%s;EVENT_IDEM=:%s;BLOCK=:%d;EVENT_NAME=:%s",
		s.engine.GetClientID(), messageIdem, block, s.MessageName)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("ack failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Defer(messageIdem string, delayMs int64, reason string) (*common.DeferResponse, error) {
	message := fmt.Sprintf("DEFER;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s;DELAY=:%d;REASON=:%s",
		s.engine.GetClientID(), messageIdem, s.MessageName, delayMs, reason)

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

func (s *wsSubscription) Discard(messageIdem, reason string) (*common.DiscardResponse, error) {
	message := fmt.Sprintf("DISCARD;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s;REASON=:%s",
		s.engine.GetClientID(), messageIdem, s.MessageName, reason)

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
		s.engine.GetClientID(), s.MessageName, reason)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("pause failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Resume() error {
	message := fmt.Sprintf("CONTINUE;CLIENT_ID=:%s;EVENT_NAME=:%s", s.engine.GetClientID(), s.MessageName)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("resume failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Replay(messageIdem string) (*common.MessagePayload, error) {
	message := fmt.Sprintf("REPLAY;CLIENT_ID=:%s;EVENT_IDEM=:%s;EVENT_NAME=:%s",
		s.engine.GetClientID(), messageIdem, s.MessageName)

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
		s.engine.GetClientID(), messageIdem, block)

	_, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("rollback failed", common.ErrTypeGeneric, err)
	}
	return nil
}

func (s *wsSubscription) Unsubscribe() error {
	message := fmt.Sprintf("UNSUB;CLIENT_ID=:%s;EVENT_NAME=:%s", s.engine.GetClientID(), s.MessageName)

	response, err := s.engine.sendRequest(message)
	if err != nil {
		return common.NewEnSyncError("unsubscribe failed", common.ErrTypeSubscription, err)
	}

	if !strings.HasPrefix(response, "+PASS") {
		return common.NewEnSyncError("unsubscribe failed: "+response, common.ErrTypeSubscription, nil)
	}

	s.engine.SubscriptionMgr.Delete(s.MessageName)
	s.engine.Logger().Info("Successfully unsubscribed", zap.String("eventName", s.MessageName))
	return nil
}

func (s *wsSubscription) processMessage(message *wsMessageEvent) (*common.MessagePayload, error) {
	decrypted, err := common.DecryptAndParseMessage(
		message.Payload,
		message.Metadata,
		s.appSecretKey,
		s.engine.Logger(),
	)
	if err != nil {
		return nil, err
	}

	return &common.MessagePayload{
		MessageName: message.MessageName,
		Idem:        message.MessageIdem,
		Block:       message.PartitionBlock,
		Timestamp:   time.Now().UnixMilli(),
		Payload:     decrypted.Payload,
		Metadata:    decrypted.Metadata,
		Sender:      message.Sender,
	}, nil
}
