package common

import (
	"fmt"
	"sync"
)

type BaseSubscription struct {
	EventName string
	Handlers  []EventHandler
	mu        sync.RWMutex
}

func (s *BaseSubscription) AddHandler(handler EventHandler) func() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Handlers = append(s.Handlers, handler)

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		for i, h := range s.Handlers {
			if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
				s.Handlers = append(s.Handlers[:i], s.Handlers[i+1:]...)
				break
			}
		}
	}
}

func (s *BaseSubscription) GetHandlers() []EventHandler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	handlers := make([]EventHandler, len(s.Handlers))
	copy(handlers, s.Handlers)
	return handlers
}

func (s *BaseSubscription) CallHandlers(event *EventPayload, logger Logger) {
	handlers := s.GetHandlers()
	for _, handler := range handlers {
		go func(h EventHandler) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Handler panic", map[string]interface{}{
						"eventName": s.EventName,
						"eventId":   event.Idem,
						"panic":     r,
					})
				}
			}()
			if err := h(event); err != nil {
				logger.Error("Event handler error", map[string]interface{}{
					"eventName": s.EventName,
					"eventId":   event.Idem,
					"error":     err.Error(),
				})
			}
		}(handler)
	}
}

type SubscriptionManager struct {
	subscriptions sync.Map
}

func newSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{}
}

func (sm *SubscriptionManager) Store(eventName string, subscription Subscription) {
	sm.subscriptions.Store(eventName, subscription)
}

func (sm *SubscriptionManager) Load(eventName string) (Subscription, bool) {
	if val, exists := sm.subscriptions.Load(eventName); exists {
		return val.(Subscription), true
	}
	return nil, false
}

func (sm *SubscriptionManager) Delete(eventName string) {
	sm.subscriptions.Delete(eventName)
}

func (sm *SubscriptionManager) Range(fn func(key, value interface{}) bool) {
	sm.subscriptions.Range(fn)
}

func (sm *SubscriptionManager) Exists(eventName string) bool {
	_, exists := sm.subscriptions.Load(eventName)
	return exists
}
