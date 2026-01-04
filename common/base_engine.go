package common

import (
	"context"
	"sync"
	"time"
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

	if b.config.timeoutConfig == nil {
		b.config.timeoutConfig = defaultTimeoutConfig()
	}

	if b.config.concurrencyConfig == nil {
		b.config.concurrencyConfig = defaultConcurrencyConfig()
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
	appSecretKey    string
	keyMu           sync.RWMutex
	State           engineState
	ClientID        string
	Logger          Logger
	ClientHash      string
	SubscriptionMgr *SubscriptionManager
	Ctx             context.Context
	ctxCancel       context.CancelFunc
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

	baseEngine.Ctx, baseEngine.ctxCancel = context.WithCancel(ctx)

	return baseEngine, nil
}

func (b *BaseEngine) GetAppSecretKey() string {
	b.keyMu.RLock()
	defer b.keyMu.RUnlock()
	return b.appSecretKey
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

func (e *BaseEngine) IsConnected() bool {
	e.State.Mu.RLock()
	defer e.State.Mu.RUnlock()
	return e.State.IsConnected && e.State.IsAuthenticated
}

func (e *BaseEngine) GetClientID() string {
	return e.ClientID
}

func (e *BaseEngine) SetAuthenticated(clientID, clientHash string) {
	e.ClientID = clientID
	e.ClientHash = clientHash

	e.State.Mu.Lock()
	e.State.IsAuthenticated = true
	e.State.IsConnected = true
	e.State.Mu.Unlock()
}

func (e *BaseEngine) SetConnectionState(connected bool) {
	e.State.Mu.Lock()
	e.State.IsConnected = connected
	if !connected {
		e.State.IsAuthenticated = false
	}
	e.State.Mu.Unlock()
}

func (e *BaseEngine) ResetState() {
	e.State.Mu.Lock()
	e.State.IsAuthenticated = false
	e.State.IsConnected = false
	e.State.Mu.Unlock()

	if e.ctxCancel != nil {
		e.ctxCancel()
	}
}

func (e *BaseEngine) GetOperationTimeout() time.Duration {
	return e.config.timeoutConfig.OperationTimeout
}

func (e *BaseEngine) GetConnectionTimeout() time.Duration {
	return e.config.timeoutConfig.ConnectionTimeout
}

func (e *BaseEngine) GetPingInterval() time.Duration {
	return e.config.timeoutConfig.PingInterval
}

func (e *BaseEngine) GetGracefulShutdownTimeout() time.Duration {
	return e.config.timeoutConfig.GracefulShutdownTimeout
}

func (e *BaseEngine) GetPublishConcurrency() int {
	return e.config.concurrencyConfig.PublishConcurrency
}

func (e *BaseEngine) GetRecvBufferSize() int {
	return e.config.concurrencyConfig.RecvBufferSize
}

func (e *BaseEngine) GetRecvTimeout() time.Duration {
	return e.config.concurrencyConfig.RecvTimeout
}

func (e *BaseEngine) GetCleanupDelay() time.Duration {
	return e.config.concurrencyConfig.CleanupDelay
}
