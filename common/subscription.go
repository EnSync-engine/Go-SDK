package common

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

func generateID() string {
	b := make([]byte, uuidByteLength)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

type BaseSubscription struct {
	MessageName string
	handlers    map[string]MessageHandler
	mu          sync.RWMutex
	active      atomic.Bool
	workerPool  *WorkerPool
	autoAck     bool
}

func NewBaseSubscription(messageName string, autoAck bool, workerCount int) *BaseSubscription {
	s := &BaseSubscription{
		MessageName: messageName,
		handlers:    make(map[string]MessageHandler),
		workerPool:  NewWorkerPool(workerCount),
		autoAck:     autoAck,
	}
	s.active.Store(true)
	return s
}

func (s *BaseSubscription) AddMessageHandler(handler MessageHandler) func() {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateID()
	if id == "" {
		id = fmt.Sprintf("%p", handler)
	}

	s.handlers[id] = handler

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.handlers, id)
	}
}

func (s *BaseSubscription) ProcessMessage(msg *MessagePayload, logger Logger, ackFunc func(string, int64) error) {
	if !s.active.Load() {
		return
	}

	submitted := s.workerPool.Submit(func() {
		s.mu.RLock()
		handlers := make([]MessageHandler, 0, len(s.handlers))
		for _, h := range s.handlers {
			handlers = append(handlers, h)
		}
		s.mu.RUnlock()

		for _, handler := range handlers {
			ctx := &MessageContext{
				Message: msg,
				ack: func() error {
					if ackFunc != nil {
						return ackFunc(msg.Idem, msg.Block)
					}
					return nil
				},
			}

			handler(ctx)

			if s.autoAck && !ctx.WasControlCalled() && ackFunc != nil {
				if err := ackFunc(msg.Idem, msg.Block); err != nil {
					if logger != nil {
						logger.Error("Failed to auto-ACK message", "idem", msg.Idem, "error", err)
					}
				}
			}
		}
	})

	if !submitted {
		if logger != nil {
			logger.Warn("Message dropped: worker pool full or stopped", "messageName", s.MessageName, "idem", msg.Idem)
		}
	}
}

func (s *BaseSubscription) Close() {
	s.active.Store(false)
	if s.workerPool != nil {
		s.workerPool.Stop()
	}
}

type SubscriptionManager struct {
	subscriptions sync.Map
}

func newSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{}
}

func (sm *SubscriptionManager) Store(messageName string, subscription Subscription) {
	sm.subscriptions.Store(messageName, subscription)
}

func (sm *SubscriptionManager) Load(messageName string) (Subscription, bool) {
	if val, exists := sm.subscriptions.Load(messageName); exists {
		return val.(Subscription), true
	}
	return nil, false
}

func (sm *SubscriptionManager) Delete(messageName string) {
	sm.subscriptions.Delete(messageName)
}

func (sm *SubscriptionManager) Range(fn func(key, value interface{}) bool) {
	sm.subscriptions.Range(fn)
}

func (sm *SubscriptionManager) Exists(messageName string) bool {
	_, exists := sm.subscriptions.Load(messageName)
	return exists
}

// DecryptedMessage holds the decrypted payload and metadata from a message
type DecryptedMessage struct {
	Payload  map[string]interface{}
	Metadata MessageMetadata
}

// DecryptAndParseMessage decrypts an encrypted payload using the provided secret key
// and returns the decrypted payload and metadata. This is shared logic used by both
// WebSocket and gRPC subscriptions.
func DecryptAndParseMessage(
	encryptedPayloadB64 string,
	metadataStr string,
	appSecretKey string,
	logger Logger,
) (*DecryptedMessage, error) {
	if appSecretKey == "" {
		if logger != nil {
			logger.Error("No decryption key available")
		}
		return nil, NewEnSyncError("no decryption key available", ErrTypeSubscription, nil)
	}

	// Decode base64 payload
	decodedPayload, err := base64.StdEncoding.DecodeString(encryptedPayloadB64)
	if err != nil {
		return nil, NewEnSyncError("failed to decode payload", ErrTypeSubscription, err)
	}

	// Parse encrypted payload structure
	encryptedPayload, err := parseEncryptedPayload(string(decodedPayload))
	if err != nil {
		return nil, NewEnSyncError("failed to parse encrypted payload", ErrTypeSubscription, err)
	}

	// Decode decryption key
	keyBytes, err := base64.StdEncoding.DecodeString(appSecretKey)
	if err != nil {
		return nil, NewEnSyncError("failed to decode decryption key", ErrTypeSubscription, err)
	}

	// Decrypt based on payload type
	var payloadStr string
	switch v := encryptedPayload.(type) {
	case *HybridEncryptedMessage:
		payloadStr, err = decryptHybridMessage(v, keyBytes)
		if err != nil {
			return nil, NewEnSyncError("hybrid decryption failed", ErrTypeSubscription, err)
		}
	case *EncryptedMessage:
		payloadStr, err = DecryptEd25519(v, keyBytes)
		if err != nil {
			return nil, NewEnSyncError("ed25519 decryption failed", ErrTypeSubscription, err)
		}
	default:
		return nil, NewEnSyncError("unknown encrypted payload type: "+fmt.Sprintf("%T", v), ErrTypeSubscription, nil)
	}

	// Unmarshal decrypted payload
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return nil, NewEnSyncError("failed to unmarshal decrypted payload", ErrTypeSubscription, err)
	}

	// Parse metadata
	var metadata MessageMetadata
	if metadataStr != "" {
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			if logger != nil {
				logger.Warn("Failed to unmarshal metadata", "error", err)
			}
			metadata = MessageMetadata{}
		}
	} else {
		metadata = MessageMetadata{}
	}

	return &DecryptedMessage{
		Payload:  payload,
		Metadata: metadata,
	}, nil
}
