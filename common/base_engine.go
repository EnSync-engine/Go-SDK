package common

import (
	"context"
	"sync"
)

type engineState struct {
	Mu              sync.RWMutex
	IsConnected     bool
	IsAuthenticated bool
}

type BaseEngineBuilder struct {
	config *engineConfig
}

func NewBaseEngineBuilder() *BaseEngineBuilder {
	return &BaseEngineBuilder{
		config: &engineConfig{},
	}
}

func (b *BaseEngineBuilder) WithConfigOptions(opts ...Option) *BaseEngineBuilder {
	for _, opt := range opts {
		opt(b.config)
	}
	return b
}

func (b *BaseEngineBuilder) Build(ctx context.Context) (*BaseEngine, error) {
	if b.config == nil {
		b.config = defaultEngineConfig()
	}

	if b.config.logger == nil {
		b.config.logger = &noopLogger{}
	}

	if b.config.retryConfig == nil {
		b.config.retryConfig = defaultRetryConfig()
	}

	engine := &BaseEngine{
		config:          b.config,
		State:           engineState{},
		Ctx:             ctx,
		Logger:          b.config.logger,
		SubscriptionMgr: newSubscriptionManager(),
		retryConfig:     b.config.retryConfig,
	}

	if b.config.circuitBreaker != nil {
		engine.circuitBreaker = newCircuitBreaker(b.config.circuitBreaker, b.config.logger)
	}

	return engine, nil
}

type BaseEngine struct {
	AccessKey       string
	AppSecretKey    string
	State           engineState
	ClientID        string
	Logger          Logger
	ClientHash      string
	SubscriptionMgr *SubscriptionManager
	Ctx             context.Context
	config          *engineConfig
	circuitBreaker  *circuitBreaker
	retryConfig     *retryConfig
}

func NewBaseEngine(ctx context.Context, opts ...Option) (*BaseEngine, error) {
	baseEngine, err := NewBaseEngineBuilder().
		WithConfigOptions(opts...).
		Build(ctx)

	if err != nil {
		return nil, err
	}

	return baseEngine, nil
}

func (b *BaseEngine) recordFailure() {
	if b.circuitBreaker == nil {
		return
	}

	oldState := b.circuitBreaker.state
	b.circuitBreaker.RecordFailure()
	newState := b.circuitBreaker.state

	if oldState != newState && b.Logger != nil {
		b.Logger.Info("Circuit breaker state changed",
			"oldState", oldState,
			"newState", newState,
			"failures", b.circuitBreaker.failures)
	}
}

func (b *BaseEngine) recordSuccess() {
	if b.circuitBreaker == nil {
		return
	}

	oldState := b.circuitBreaker.state
	b.circuitBreaker.RecordSuccess()
	newState := b.circuitBreaker.state

	if oldState != newState && b.Logger != nil {
		b.Logger.Info("Circuit breaker state changed",
			"oldState", oldState,
			"newState", newState)
	}
}

func (b *BaseEngine) canAttemptConnection() bool {
	if b.circuitBreaker == nil {
		return true
	}
	return b.circuitBreaker.canAttempt()
}

// IsConnected returns whether the engine is currently connected
func (e *BaseEngine) IsConnected() bool {
	e.State.Mu.RLock()
	defer e.State.Mu.RUnlock()
	return e.State.IsConnected && e.State.IsAuthenticated
}

// GetClientID returns the client's ID
func (e *BaseEngine) GetClientID() string {
	return e.ClientID
}

// SetAuthenticated sets the engine as authenticated and connected
func (e *BaseEngine) SetAuthenticated(clientID, clientHash string) {
	e.ClientID = clientID
	e.ClientHash = clientHash

	e.State.Mu.Lock()
	e.State.IsAuthenticated = true
	e.State.IsConnected = true
	e.State.Mu.Unlock()
}

// SetConnectionState updates the connection state
func (e *BaseEngine) SetConnectionState(connected bool) {
	e.State.Mu.Lock()
	e.State.IsConnected = connected
	if !connected {
		e.State.IsAuthenticated = false // Ensure auth is cleared on disconnect
	}
	e.State.Mu.Unlock()
}

// ResetState resets the engine state (e.g. on close)
func (e *BaseEngine) ResetState() {
	e.State.Mu.Lock()
	e.State.IsAuthenticated = false
	e.State.IsConnected = false
	e.State.Mu.Unlock()
}
