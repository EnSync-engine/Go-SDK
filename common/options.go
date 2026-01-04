package common

import "time"

// TimeoutConfig holds timeout-related settings for the engine
type TimeoutConfig struct {
	OperationTimeout        time.Duration
	GracefulShutdownTimeout time.Duration
	PingInterval            time.Duration
	ConnectionTimeout       time.Duration
}

// DefaultTimeoutConfig returns sensible default timeout values
func defaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		OperationTimeout:        5 * time.Second,
		GracefulShutdownTimeout: 10 * time.Second,
		PingInterval:            30 * time.Second,
		ConnectionTimeout:       10 * time.Second,
	}
}

type ConcurrencyConfig struct {
	PublishConcurrency int
	RecvBufferSize     int
	RecvTimeout        time.Duration
	CleanupDelay       time.Duration
}

func defaultConcurrencyConfig() *ConcurrencyConfig {
	return &ConcurrencyConfig{
		PublishConcurrency: 10,
		RecvBufferSize:     100,
		RecvTimeout:        100 * time.Millisecond,
		CleanupDelay:       100 * time.Millisecond,
	}
}

type engineConfig struct {
	logger            Logger
	circuitBreaker    *circuitBreakerConfig
	retryConfig       *retryConfig
	timeoutConfig     *TimeoutConfig
	concurrencyConfig *ConcurrencyConfig
}

func defaultEngineConfig() *engineConfig {
	return &engineConfig{
		circuitBreaker:    defaultCircuitBreakerConfig(),
		retryConfig:       defaultRetryConfig(),
		logger:            &noopLogger{},
		timeoutConfig:     defaultTimeoutConfig(),
		concurrencyConfig: defaultConcurrencyConfig(),
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

// WithDefaultRetryConfig sets the default retry configuration
func WithDefaultRetryConfig() Option {
	return func(c *engineConfig) {
		c.retryConfig = defaultRetryConfig()
	}
}

// WithTimeoutOptions combines multiple timeout-related options into one.
// Usage: common.WithTimeoutOptions(
//
//	common.WithOperationTimeout(5*time.Second),
//	common.WithGracefulShutdownTimeout(10*time.Second),
//	common.WithPingInterval(30*time.Second),
//
// )
func WithTimeoutOptions(opts ...Option) Option {
	return func(c *engineConfig) {
		// Apply all provided options to configure timeouts
		for _, opt := range opts {
			opt(c)
		}
	}
}

// WithOperationTimeout sets the timeout for individual operations
func WithOperationTimeout(timeout time.Duration) Option {
	return func(c *engineConfig) {
		if c.timeoutConfig == nil {
			c.timeoutConfig = defaultTimeoutConfig()
		}
		c.timeoutConfig.OperationTimeout = timeout
	}
}

// WithGracefulShutdownTimeout sets the timeout for graceful shutdown
func WithGracefulShutdownTimeout(timeout time.Duration) Option {
	return func(c *engineConfig) {
		if c.timeoutConfig == nil {
			c.timeoutConfig = defaultTimeoutConfig()
		}
		c.timeoutConfig.GracefulShutdownTimeout = timeout
	}
}

// WithPingInterval sets the interval between heartbeat pings
func WithPingInterval(interval time.Duration) Option {
	return func(c *engineConfig) {
		if c.timeoutConfig == nil {
			c.timeoutConfig = defaultTimeoutConfig()
		}
		c.timeoutConfig.PingInterval = interval
	}
}

// WithConnectionTimeout sets the timeout for connection operations
func WithConnectionTimeout(timeout time.Duration) Option {
	return func(c *engineConfig) {
		if c.timeoutConfig == nil {
			c.timeoutConfig = defaultTimeoutConfig()
		}
		c.timeoutConfig.ConnectionTimeout = timeout
	}
}
