package common

import (
	"fmt"
	"sync"
)

type BaseSubscription struct {
	MessageName string
	handlers    []MessageHandler
	mu          sync.RWMutex
	active      bool
	workerPool  *WorkerPool
	autoAck     bool
}

func NewBaseSubscription(messageName string, autoAck bool, workerCount int) BaseSubscription {
	return BaseSubscription{
		MessageName: messageName,
		handlers:    make([]MessageHandler, 0),
		active:      true,
		workerPool:  NewWorkerPool(workerCount),
		autoAck:     autoAck,
	}
}

func (s *BaseSubscription) AddMessageHandler(handler MessageHandler) func() {
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

func (s *BaseSubscription) ProcessMessage(msg *MessagePayload, ackFunc func(string, string, int64) error) {
	if !s.active {
		return
	}

	s.workerPool.Submit(func() {
		s.mu.RLock()
		handlers := make([]MessageHandler, len(s.handlers))
		copy(handlers, s.handlers)
		s.mu.RUnlock()

		for _, handler := range handlers {
			if err := handler(msg); err != nil {
				continue
			}
		}

		if s.autoAck && msg.Idem != "" && msg.Block != 0 && ackFunc != nil {
			_ = ackFunc(msg.MessageName, msg.Idem, msg.Block)
		}
	})
}

func (s *BaseSubscription) Close() {
	s.active = false
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
