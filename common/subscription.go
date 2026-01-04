package common

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
)

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

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
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

func (s *BaseSubscription) ProcessMessage(msg *MessagePayload, logger Logger, ackFunc func(string, string, int64) error) {
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
						return ackFunc(msg.MessageName, msg.Idem, msg.Block)
					}
					return nil
				},
			}

			handler(ctx)

			if s.autoAck && !ctx.WasControlCalled() && ackFunc != nil {
				if err := ackFunc(msg.MessageName, msg.Idem, msg.Block); err != nil {
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
