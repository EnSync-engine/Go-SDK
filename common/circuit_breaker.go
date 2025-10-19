package common

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"sync"
	"time"
)

type circuitState string

const (
	circuitStateClosed   circuitState = "closed"
	circuitStateOpen     circuitState = "open"
	circuitStateHalfOpen circuitState = "half-open"

	exponentialBase = 2.0

	fallbackRandomMod = 1000000
)

func secureRandFloat64() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return float64(time.Now().UnixNano()%fallbackRandomMod) / float64(fallbackRandomMod)
	}
	return float64(binary.LittleEndian.Uint64(buf[:])) / float64(^uint64(0))
}

type circuitBreakerConfig struct {
	FailureThreshold int
	SuccessThreshold int
	ResetTimeout     time.Duration
	MaxResetTimeout  time.Duration
}

func defaultCircuitBreakerConfig() *circuitBreakerConfig {
	return &circuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 3,
		ResetTimeout:     5 * time.Second,
		MaxResetTimeout:  60 * time.Second,
	}
}

type circuitBreaker struct {
	state           circuitState
	failures        int
	successes       int
	lastFailureTime time.Time
	mu              sync.RWMutex
	config          *circuitBreakerConfig
	logger          Logger
}

func newCircuitBreaker(config *circuitBreakerConfig, logger Logger) *circuitBreaker {
	return &circuitBreaker{
		state:  circuitStateClosed,
		config: config,
		logger: logger,
	}
}

func (cb *circuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.successes = 0 // Reset success counter on failure
	cb.lastFailureTime = time.Now()

	if cb.failures >= cb.config.FailureThreshold && cb.state != circuitStateOpen {
		prevState := cb.state
		cb.state = circuitStateOpen

		// Calculate the reset timeout *inside* the write lock to avoid a deadlock.
		exp := math.Pow(exponentialBase, float64(cb.failures-1))
		backoff := float64(cb.config.ResetTimeout) * exp
		if backoff > float64(cb.config.MaxResetTimeout) {
			backoff = float64(cb.config.MaxResetTimeout)
		}
		resetTimeout := time.Duration(backoff)

		if cb.logger != nil {
			cb.logger.Warn("Circuit breaker opened",
				"failures", cb.failures,
				"prev_state", prevState,
				"reset_timeout", resetTimeout)
		}
	}
}

func (cb *circuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case circuitStateHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.state = circuitStateClosed
			cb.resetCounters()

			if cb.logger != nil {
				cb.logger.Info("Circuit breaker reset to closed state",
					"successes", cb.successes)
			}
		}

	case circuitStateOpen:
		// First success after timeout, move to half-open
		cb.state = circuitStateHalfOpen
		cb.successes = 1

		if cb.logger != nil {
			cb.logger.Info("Circuit breaker half-open, testing connection")
		}

	case circuitStateClosed:
		// Already in closed state, reset counters to be safe
		cb.resetCounters()
	}
}

func (cb *circuitBreaker) canAttempt() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case circuitStateClosed:
		return true

	case circuitStateHalfOpen:
		return true

	case circuitStateOpen:
		resetTimeout := cb.getResetTimeout()
		timeSinceFailure := time.Since(cb.lastFailureTime)
		if timeSinceFailure > resetTimeout {
			// Transition to half-open while holding the lock
			cb.state = circuitStateHalfOpen
			cb.successes = 0

			if cb.logger != nil {
				cb.logger.Info("Circuit breaker moving to half-open after timeout")
			}
			return true
		}

		if cb.logger != nil {
			cb.logger.Debug("Circuit breaker blocked",
				"state", "open",
				"time_remaining", resetTimeout-timeSinceFailure,
				"failures", cb.failures)
		}
		return false

	default:
		return false
	}
}

func (cb *circuitBreaker) getResetTimeout() time.Duration {
	if cb.failures == 0 {
		return 0
	}

	// Calculate exponential backoff
	exp := math.Pow(exponentialBase, float64(cb.failures-1))
	backoff := float64(cb.config.ResetTimeout) * exp

	// Cap at max reset timeout
	if backoff > float64(cb.config.MaxResetTimeout) {
		backoff = float64(cb.config.MaxResetTimeout)
	}

	// Add jitter (0.8x to 1.2x of backoff)
	jitter := 0.8 + 0.4*secureRandFloat64()
	return time.Duration(backoff * jitter)
}

func (cb *circuitBreaker) resetCounters() {
	cb.failures = 0
	cb.successes = 0
	cb.lastFailureTime = time.Time{}
}
