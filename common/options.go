package common

import "time"

type engineConfig struct {
	logger           Logger
	circuitBreaker   *circuitBreakerConfig
	retryConfig      *retryConfig
	operationTimeout time.Duration
}

func defaultEngineConfig() *engineConfig {
	return &engineConfig{
		circuitBreaker:   defaultCircuitBreakerConfig(),
		retryConfig:      defaultRetryConfig(),
		logger:           &noopLogger{},
		operationTimeout: operationTimeout,
	}
}

type Option func(*engineConfig)

func WithLogger(logger Logger) Option {
	return func(c *engineConfig) {
		if logger != nil {
			c.logger = logger
		}
	}
}

// WithCircuitBreaker sets the circuit breaker configuration
func WithCircuitBreaker(threshold int, resetTimeout time.Duration) Option {
	return func(c *engineConfig) {
		c.circuitBreaker = &circuitBreakerConfig{
			FailureThreshold: threshold,
			ResetTimeout:     resetTimeout,
		}
	}
}

// WithRetryConfig sets the retry configuration
func WithRetryConfig(maxAttempts int, initialBackoff, maxBackoff time.Duration, jitter float64) Option {
	return func(c *engineConfig) {
		c.retryConfig = &retryConfig{
			MaxAttempts:    maxAttempts,
			InitialBackoff: initialBackoff,
			MaxBackoff:     maxBackoff,
			Jitter:         jitter,
		}
	}
}

func WithOperationTimeout(timeout time.Duration) Option {
	return func(c *engineConfig) {
		c.operationTimeout = timeout
	}
}

// WithDefaultRetryConfig sets the default retry configuration
func WithDefaultRetryConfig() Option {
	return func(c *engineConfig) {
		c.retryConfig = defaultRetryConfig()
	}
}

// Client configuration options
type ClientOption func(*ClientConfig)

type ClientConfig struct {
	AppSecretKey string
	ClientID     string
}

func WithAppSecretKey(secretKey string) ClientOption {
	return func(c *ClientConfig) {
		c.AppSecretKey = secretKey
	}
}

func WithClientID(clientID string) ClientOption {
	return func(c *ClientConfig) {
		c.ClientID = clientID
	}
}
