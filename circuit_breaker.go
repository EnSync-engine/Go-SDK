package ensync

import (
	"sync"
	"time"
)

type circuitState string

const (
	closed   circuitState = "closed"
	open     circuitState = "open"
	halfOpen circuitState = "half-open"
)

type circuitBreaker struct {
	state            circuitState
	failures         int
	lastFailureTime  time.Time
	lastSuccessTime  time.Time
	failureThreshold int
	resetTimeout     time.Duration
	mu               sync.RWMutex
	logger           Logger
}

func newCircuitBreaker(logger Logger) *circuitBreaker {
	return &circuitBreaker{
		state:            closed,
		failureThreshold: 5,
		resetTimeout:     60 * time.Second,
		logger:           logger,
	}
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.failures >= cb.failureThreshold {
		cb.state = "open"
		cb.logger.Warn("Circuit breaker opened",
			"failures", cb.failures,
			"threshold", cb.failureThreshold)
	}
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.lastSuccessTime = time.Now()
	cb.state = "closed"
}

func (cb *circuitBreaker) canAttempt() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state == "closed" {
		return true
	}

	if time.Since(cb.lastFailureTime) > cb.resetTimeout {
		cb.mu.RUnlock()
		cb.mu.Lock()
		cb.state = "half-open"
		cb.mu.Unlock()
		cb.mu.RLock()
		cb.logger.Info("Circuit breaker entering half-open state")
		return true
	}

	return false
}

func (cb *circuitBreaker) getState() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return string(cb.state)
}

func (cb *circuitBreaker) getFailures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

func (cb *circuitBreaker) getResetTimeout() time.Duration {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.resetTimeout
}
