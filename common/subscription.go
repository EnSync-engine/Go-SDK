package common

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
)

const (
	handlerWorkerCount = 10
	jobBufferSize      = 100
)

type SubscriptionManager struct {
	subscriptions sync.Map
	logger        Logger
	closed        atomic.Bool
}

func NewSubscriptionManager(logger Logger) *SubscriptionManager {
	return &SubscriptionManager{
		logger: logger,
	}
}

func (sm *SubscriptionManager) Register(eventName string, sub Subscription) error {
	if sm.closed.Load() {
		return NewEnSyncError("subscription manager is closed", ErrTypeSubscription, nil)
	}

	if _, exists := sm.subscriptions.Load(eventName); exists {
		return NewEnSyncError(
			fmt.Sprintf("already subscribed to event %s", eventName),
			ErrTypeValidation,
			nil,
		)
	}

	sm.subscriptions.Store(eventName, sub)

	if sm.logger != nil {
		sm.logger.Debug("subscription registered", zap.String("eventName", eventName))
	}

	return nil
}

func (sm *SubscriptionManager) Unregister(eventName string) error {
	if _, exists := sm.subscriptions.Load(eventName); !exists {
		return NewEnSyncError(
			fmt.Sprintf("subscription not found for event %s", eventName),
			ErrTypeSubscription,
			nil,
		)
	}

	sm.subscriptions.Delete(eventName)

	if sm.logger != nil {
		sm.logger.Debug("subscription unregistered", zap.String("eventName", eventName))
	}

	return nil
}

func (sm *SubscriptionManager) Get(eventName string) (Subscription, bool) {
	if val, exists := sm.subscriptions.Load(eventName); exists {
		return val.(Subscription), true
	}
	return nil, false
}

func (sm *SubscriptionManager) GetAll() []Subscription {
	var subs []Subscription
	sm.subscriptions.Range(func(key, value interface{}) bool {
		subs = append(subs, value.(Subscription))
		return true
	})
	return subs
}

func (sm *SubscriptionManager) Count() int {
	count := 0
	sm.subscriptions.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

func (sm *SubscriptionManager) Exists(eventName string) bool {
	_, exists := sm.subscriptions.Load(eventName)
	return exists
}

func (sm *SubscriptionManager) Range(fn func(key, value interface{}) bool) {
	sm.subscriptions.Range(fn)
}

func (sm *SubscriptionManager) CloseAll(ctx context.Context) error {
	if !sm.closed.CompareAndSwap(false, true) {
		return nil
	}

	var lastErr error
	sm.subscriptions.Range(func(key, value interface{}) bool {
		sub := value.(Subscription)
		eventName := key.(string)

		if err := sub.Unsubscribe(); err != nil {
			if sm.logger != nil {
				sm.logger.Error("failed to unsubscribe during close",
					zap.String("eventName", eventName),
					zap.Error(err))
			}
			lastErr = err
		}
		return true
	})

	return lastErr
}

type BaseSubscription struct {
	EventName string
	Handlers  []EventHandler
	Mu        sync.RWMutex
	jobs      chan *EventPayload
	workerWg  sync.WaitGroup
	stopped   atomic.Bool
	logger    Logger
}

func NewBaseSubscription(eventName string, logger Logger) *BaseSubscription {
	return &BaseSubscription{
		EventName: eventName,
		logger:    logger,
	}
}

func (s *BaseSubscription) AddHandler(handler EventHandler) func() {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	s.Handlers = append(s.Handlers, handler)

	if s.logger != nil {
		s.logger.Debug("handler added",
			zap.String("eventName", s.EventName),
			zap.Int("handlerCount", len(s.Handlers)))
	}

	return func() {
		s.Mu.Lock()
		defer s.Mu.Unlock()

		for i, h := range s.Handlers {
			if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
				s.Handlers = append(s.Handlers[:i], s.Handlers[i+1:]...)
				if s.logger != nil {
					s.logger.Debug("handler removed",
						zap.String("eventName", s.EventName),
						zap.Int("handlerCount", len(s.Handlers)))
				}
				break
			}
		}
	}
}

func (s *BaseSubscription) GetHandlers() []EventHandler {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	handlers := make([]EventHandler, len(s.Handlers))
	copy(handlers, s.Handlers)
	return handlers
}

func (s *BaseSubscription) SubmitJob(event *EventPayload) error {
	if s.stopped.Load() {
		return NewEnSyncError("subscription worker pool is stopped", ErrTypeSubscription, nil)
	}

	select {
	case s.jobs <- event:
		return nil
	default:
		if s.logger != nil {
			s.logger.Warn("job queue full, event dropped",
				zap.String("eventName", s.EventName),
				zap.String("eventId", event.Idem),
				zap.Int("queueSize", jobBufferSize))
		}
		return NewEnSyncError(
			fmt.Sprintf("job queue full for event %s", s.EventName),
			ErrTypeSubscription,
			nil,
		)
	}
}

func (s *BaseSubscription) CallHandlers(event *EventPayload) {
	handlers := s.GetHandlers()

	if len(handlers) == 0 {
		if s.logger != nil {
			s.logger.Debug("no handlers registered for event",
				zap.String("eventName", s.EventName),
				zap.String("eventId", event.Idem))
		}
		return
	}

	var wg sync.WaitGroup
	for _, handler := range handlers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if s.logger != nil {
						s.logger.Error("handler panic recovered",
							zap.String("eventName", s.EventName),
							zap.String("eventId", event.Idem),
							zap.Any("panic", r))
					}
				}
			}()

			if err := handler(event); err != nil {
				if s.logger != nil {
					s.logger.Error("handler returned error",
						zap.String("eventName", s.EventName),
						zap.String("eventId", event.Idem),
						zap.Error(err))
				}
			}
		}()
	}

	wg.Wait()
}

func (s *BaseSubscription) StartWorkerPool() error {
	s.Mu.Lock()
	if s.jobs != nil {
		s.Mu.Unlock()
		return nil
	}
	s.jobs = make(chan *EventPayload, jobBufferSize)
	s.Mu.Unlock()

	if s.logger != nil {
		s.logger.Info("starting worker pool",
			zap.String("eventName", s.EventName),
			zap.Int("workerCount", handlerWorkerCount))
	}

	for i := 0; i < handlerWorkerCount; i++ {
		s.workerWg.Add(1)
		go func() {
			defer s.workerWg.Done()
			for event := range s.jobs {
				s.CallHandlers(event)
			}
		}()
	}

	return nil
}

func (s *BaseSubscription) StopWorkerPool() {
	if !s.stopped.CompareAndSwap(false, true) {
		return // Already stopped
	}

	if s.logger != nil {
		s.logger.Info("stopping worker pool",
			zap.String("eventName", s.EventName))
	}

	s.Mu.Lock()
	if s.jobs != nil {
		close(s.jobs)
	}
	s.Mu.Unlock()

	s.workerWg.Wait()

	if s.logger != nil {
		s.logger.Debug("worker pool stopped",
			zap.String("eventName", s.EventName))
	}
}
