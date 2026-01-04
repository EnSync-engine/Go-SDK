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

func (cb *circuitBreaker) getResetTimeout() time.Duration {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

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

func (cb *circuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.successes = 0 // Reset success counter on failure
	cb.lastFailureTime = time.Now()

	// Check if we should open the circuit
	if cb.failures >= cb.config.FailureThreshold && cb.state != circuitStateOpen {
		prevState := cb.state
		cb.state = circuitStateOpen

		if cb.logger != nil {
			cb.logger.Warn("Circuit breaker opened",
				"failures", cb.failures,
				"prev_state", prevState,
				"reset_timeout", cb.getResetTimeout())
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
		cb.resetCounters()
	}
}

func (cb *circuitBreaker) canAttempt() bool {
	cb.mu.RLock()

	switch cb.state {
	case circuitStateClosed:
		cb.mu.RUnlock()
		return true

	case circuitStateHalfOpen:
		cb.mu.RUnlock()
		return true

	case circuitStateOpen:
		resetTimeout := cb.getResetTimeout()
		timeSinceFailure := time.Since(cb.lastFailureTime)

		if timeSinceFailure > resetTimeout {
			cb.mu.RUnlock()
			cb.mu.Lock()

			// Double-check with write lock
			if cb.state == circuitStateOpen && time.Since(cb.lastFailureTime) > resetTimeout {
				cb.state = circuitStateHalfOpen
				if cb.logger != nil {
					cb.logger.Info("Circuit breaker half-open after timeout",
						"timeout", resetTimeout,
						"time_since_failure", timeSinceFailure)
				}
			}
			cb.mu.Unlock()
			return true
		}

		// Still in open state
		cb.mu.RUnlock()

		if cb.logger != nil {
			cb.logger.Debug("Circuit breaker blocked",
				"state", "open",
				"time_remaining", resetTimeout-timeSinceFailure,
				"failures", cb.failures)
		}
		return false

	default:
		cb.mu.RUnlock()
		return false
	}
}

func (cb *circuitBreaker) resetCounters() {
	cb.failures = 0
	cb.successes = 0
	cb.lastFailureTime = time.Time{}
}
