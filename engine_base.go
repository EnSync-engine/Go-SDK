package ensync

import (
	"context"
	"sync"
	"time"
)

type engineConfig struct {
	url                  string
	accessKey            string
	clientID             string
	clientHash           string
	appSecretKey         string
	maxReconnectAttempts int
	reconnectDelay       time.Duration
	autoReconnect        bool
}

type state struct {
	isConnected       bool
	isAuthenticated   bool
	reconnectAttempts int
	shouldReconnect   bool
	lastError         error
	lastConnectTime   time.Time
	totalReconnects   int
	mu                sync.RWMutex
}

type baseEngine struct {
	config         engineConfig
	state          state
	ctx            context.Context
	reconnectMu    sync.Mutex
	circuitBreaker *circuitBreaker
	logger         Logger
}

func (b *baseEngine) GetClientID() string {
	return b.config.clientID
}

func (b *baseEngine) GetClientHash() string {
	return b.config.clientHash
}

func (b *baseEngine) GetAppSecretKey() string {
	return b.config.appSecretKey
}

func (b *baseEngine) IsConnected() bool {
	b.state.mu.RLock()
	defer b.state.mu.RUnlock()
	return b.state.isConnected
}

func (b *baseEngine) IsAuthenticated() bool {
	b.state.mu.RLock()
	defer b.state.mu.RUnlock()
	return b.state.isAuthenticated
}

func (b *baseEngine) SetConnected(connected bool) {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()
	b.state.isConnected = connected
	if connected {
		b.state.lastConnectTime = time.Now()
		b.state.reconnectAttempts = 0
	}
}

func (b *baseEngine) SetAuthenticated(authenticated bool) {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()
	b.state.isAuthenticated = authenticated
}

func (b *baseEngine) SetContext(ctx context.Context) {
	b.ctx = ctx
	if b.config.reconnectDelay == 0 {
		b.config.reconnectDelay = 5 * time.Second
	}
	if b.config.maxReconnectAttempts == 0 {
		b.config.maxReconnectAttempts = 5
	}
	b.config.autoReconnect = true
	
	if b.logger == nil {
		b.logger = &noopLogger{}
	}
	
	if b.circuitBreaker == nil {
		b.circuitBreaker = newCircuitBreaker(b.logger)
	}
}

func (b *baseEngine) SetLogger(logger Logger) {
	if logger != nil {
		b.logger = logger
		if b.circuitBreaker != nil {
			b.circuitBreaker.logger = logger
		}
	} else {
		b.logger = &noopLogger{}
	}
}

func (b *baseEngine) ShouldReconnect() bool {
	b.state.mu.RLock()
	defer b.state.mu.RUnlock()
	return b.config.autoReconnect &&
		b.state.shouldReconnect &&
		b.state.reconnectAttempts < b.config.maxReconnectAttempts
}

func (b *baseEngine) IncrementReconnectAttempts() int {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()
	b.state.reconnectAttempts++
	return b.state.reconnectAttempts
}

func (b *baseEngine) ResetReconnectAttempts() {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()
	b.state.reconnectAttempts = 0
}

func (b *baseEngine) SetLastError(err error) {
	b.state.mu.Lock()
	defer b.state.mu.Unlock()
	b.state.lastError = err
}

func (b *baseEngine) GetLastError() error {
	b.state.mu.RLock()
	defer b.state.mu.RUnlock()
	return b.state.lastError
}

func (b *baseEngine) GetReconnectDelay() time.Duration {
	b.state.mu.RLock()
	attempts := b.state.reconnectAttempts
	delay := b.config.reconnectDelay
	b.state.mu.RUnlock()

	for i := 0; i < attempts && delay < 60*time.Second; i++ {
		delay *= 2
	}
	if delay > 60*time.Second {
		delay = 60 * time.Second
	}
	return delay
}

func (b *baseEngine) recordFailure() {
	b.circuitBreaker.recordFailure()
}

func (b *baseEngine) recordSuccess() {
	b.circuitBreaker.recordSuccess()
	
	b.state.mu.Lock()
	b.state.totalReconnects++
	b.state.mu.Unlock()
}

func (b *baseEngine) canAttemptConnection() bool {
	return b.circuitBreaker.canAttempt()
}

func (b *baseEngine) HandleConnectionError(err error, reconnectFn func() error) {
	b.SetLastError(err)
	b.SetConnected(false)
	b.SetAuthenticated(false)
	b.recordFailure()

	if !b.ShouldReconnect() {
		b.logger.Warn("Connection lost and reconnection disabled or max attempts reached")
		return
	}

	b.reconnectMu.Lock()
	defer b.reconnectMu.Unlock()

	if !b.IsConnected() {
		go b.attemptReconnect(reconnectFn)
	}
}

func (b *baseEngine) attemptReconnect(reconnectFn func() error) {
	for b.ShouldReconnect() {
		if !b.canAttemptConnection() {
			resetTimeout := b.circuitBreaker.getResetTimeout()
			b.logger.Warn("Circuit breaker is open, waiting before retry",
				"resetTimeout", resetTimeout)
			select {
			case <-b.ctx.Done():
				b.logger.Info("Reconnection cancelled", "reason", b.ctx.Err())
				return
			case <-time.After(resetTimeout):
				continue
			}
		}

		attempts := b.IncrementReconnectAttempts()
		delay := b.GetReconnectDelay()

		b.logger.Info("Attempting reconnection",
			"attempt", attempts,
			"maxAttempts", b.config.maxReconnectAttempts,
			"delay", delay)

		select {
		case <-b.ctx.Done():
			b.logger.Info("Reconnection cancelled", "reason", b.ctx.Err())
			return
		case <-time.After(delay):
		}

		if b.ctx.Err() != nil {
			b.logger.Info("Context cancelled, stopping reconnection")
			return
		}

		if err := reconnectFn(); err != nil {
			b.logger.Error("Reconnection attempt failed",
				"attempt", attempts,
				"error", err)
			b.SetLastError(err)
			b.recordFailure()

			if !IsRetryableError(err) {
				b.logger.Warn("Non-retryable error encountered", "error", err)
				return
			}
			continue
		}

		b.logger.Info("Reconnection successful")
		b.ResetReconnectAttempts()
		b.recordSuccess()
		return
	}

	b.logger.Warn("Max reconnection attempts reached",
		"maxAttempts", b.config.maxReconnectAttempts)
}

type grpcSubscriptions struct {
	mu   sync.RWMutex
	subs map[string]*grpcSubscription
}

type wsSubscriptions struct {
	mu   sync.RWMutex
	subs map[string]*wsSubscription
}
