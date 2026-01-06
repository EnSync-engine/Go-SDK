package common

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type engineState struct {
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
		logger:          b.config.logger,
		SubscriptionMgr: newSubscriptionManager(),
		retryConfig:     b.config.retryConfig,
	}

	if b.config.circuitBreaker != nil {
		engine.circuitBreaker = newCircuitBreaker(b.config.circuitBreaker, b.config.logger)
	}

	return engine, nil
}

type BaseEngine struct {
	// Configuration
	config         *engineConfig
	circuitBreaker *circuitBreaker
	retryConfig    *retryConfig
	logger         Logger

	// State
	stateMu      sync.RWMutex
	State        engineState
	closed       atomic.Bool
	Reconnecting atomic.Bool
	clientID     string
	accessKey    string

	// Context Management
	Ctx           context.Context
	ctxCancel     context.CancelFunc
	sessionMu     sync.RWMutex
	sessionCtx    context.Context
	sessionCancel context.CancelFunc

	// Subsystems
	SubscriptionMgr *SubscriptionManager
}

func NewBaseEngine(ctx context.Context, opts ...Option) (*BaseEngine, error) {
	baseEngine, err := NewBaseEngineBuilder().
		WithConfigOptions(opts...).
		Build(ctx)

	if err != nil {
		return nil, err
	}

	baseEngine.Ctx, baseEngine.ctxCancel = context.WithCancel(ctx)
	baseEngine.ResetConnectionContext()

	return baseEngine, nil
}

// Connection Context Management

func (b *BaseEngine) ResetConnectionContext() context.Context {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()

	if b.sessionCancel != nil {
		b.sessionCancel()
	}

	b.sessionCtx, b.sessionCancel = context.WithCancel(b.Ctx)
	return b.sessionCtx
}

func (b *BaseEngine) GetConnectionContext() context.Context {
	b.sessionMu.RLock()
	defer b.sessionMu.RUnlock()
	return b.sessionCtx
}

func (b *BaseEngine) CancelConnectionContext() {
	b.sessionMu.RLock()
	defer b.sessionMu.RUnlock()
	if b.sessionCancel != nil {
		b.sessionCancel()
	}
}

func (b *BaseEngine) GetAccessKey() string {
	b.stateMu.RLock()
	defer b.stateMu.RUnlock()
	return b.accessKey
}

func (b *BaseEngine) SetAccessKey(key string) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	b.accessKey = key
}

func (b *BaseEngine) GetClientID() string {
	b.stateMu.RLock()
	defer b.stateMu.RUnlock()
	return b.clientID
}

func (b *BaseEngine) recordFailure() {
	if b.circuitBreaker == nil {
		return
	}

	oldState := b.circuitBreaker.state
	b.circuitBreaker.RecordFailure()
	newState := b.circuitBreaker.state

	if oldState != newState {
		b.Logger().Info("Circuit breaker state changed",
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

	if oldState != newState {
		b.Logger().Info("Circuit breaker state changed",
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
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()
	return e.State.IsConnected && e.State.IsAuthenticated
}

func (e *BaseEngine) SetAuthenticated(clientID string) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	e.clientID = clientID
	e.State.IsAuthenticated = true
	e.State.IsConnected = true
}

func (e *BaseEngine) SetConnectionState(connected bool) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	e.State.IsConnected = connected
	if !connected {
		e.State.IsAuthenticated = false
	}
}

func (e *BaseEngine) ResetState() {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	e.State.IsAuthenticated = false
	e.State.IsConnected = false

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

func (e *BaseEngine) GetConcurrencyCount() int {
	return e.config.concurrencyConfig.ConcurrencyCount
}

func (e *BaseEngine) GetRecvBufferSize() int {
	return e.config.concurrencyConfig.RecvBufferSize
}

func (e *BaseEngine) GetRecvTimeout() time.Duration {
	return e.config.concurrencyConfig.RecvTimeout
}

func (e *BaseEngine) GetSubscriptionWorkerCount() int {
	return e.config.concurrencyConfig.SubscriptionWorkerCount
}

func (e *BaseEngine) IsClosed() bool {
	return e.closed.Load()
}

func (e *BaseEngine) Logger() Logger {
	return e.logger
}

func (e *BaseEngine) Context() context.Context {
	return e.Ctx
}

type ReconnectConfig struct {
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		InitialBackoff: initialReconnectDelay * time.Second,
		MaxBackoff:     maxReconnectDelay * time.Second,
	}
}

func (e *BaseEngine) ReconnectLoop(cfg ReconnectConfig, beforeAttempt func(), attemptFn func() error) {
	backoff := cfg.InitialBackoff

	for {
		if e.closed.Load() {
			return
		}

		if beforeAttempt != nil {
			beforeAttempt()
		}

		if err := attemptFn(); err == nil {
			e.Logger().Info("Reconnection successful")
			return
		} else {
			e.Logger().Error("Reconnection failed", "error", err, "retry_in", backoff)
			e.SetConnectionState(false)
		}

		select {
		case <-time.After(backoff):
			backoff = min(backoff*reconnectBackoffMultiplier, cfg.MaxBackoff)
		case <-e.Ctx.Done():
			return
		}
	}
}
