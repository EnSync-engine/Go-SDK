package common

import "time"

type TimeoutConfig struct {
	OperationTimeout        time.Duration
	GracefulShutdownTimeout time.Duration
	PingInterval            time.Duration
	ConnectionTimeout       time.Duration
}

func defaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		OperationTimeout:        5 * time.Second,
		GracefulShutdownTimeout: 10 * time.Second,
		PingInterval:            30 * time.Second,
		ConnectionTimeout:       10 * time.Second,
	}
}

type ConcurrencyConfig struct {
	ConcurrencyCount        int
	RecvBufferSize          int
	RecvTimeout             time.Duration
	CleanupDelay            time.Duration
	SubscriptionWorkerCount int
}

func defaultConcurrencyConfig() *ConcurrencyConfig {
	return &ConcurrencyConfig{
		ConcurrencyCount:        10,
		RecvBufferSize:          100,
		RecvTimeout:             100 * time.Millisecond,
		CleanupDelay:            100 * time.Millisecond,
		SubscriptionWorkerCount: 10,
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

func WithCircuitBreaker(threshold int, resetTimeout, maxResetTimeout time.Duration) Option {
	return func(c *engineConfig) {
		c.circuitBreaker = &circuitBreakerConfig{
			FailureThreshold: threshold,
			ResetTimeout:     resetTimeout,
			MaxResetTimeout:  maxResetTimeout,
		}
	}
}

func WithTimeouts(cfg *TimeoutConfig) Option {
	return func(c *engineConfig) {
		if cfg == nil {
			return
		}
		if c.timeoutConfig == nil {
			c.timeoutConfig = defaultTimeoutConfig()
		}
		if cfg.OperationTimeout != 0 {
			c.timeoutConfig.OperationTimeout = cfg.OperationTimeout
		}
		if cfg.ConnectionTimeout != 0 {
			c.timeoutConfig.ConnectionTimeout = cfg.ConnectionTimeout
		}
		if cfg.PingInterval != 0 {
			c.timeoutConfig.PingInterval = cfg.PingInterval
		}
		if cfg.GracefulShutdownTimeout != 0 {
			c.timeoutConfig.GracefulShutdownTimeout = cfg.GracefulShutdownTimeout
		}
	}
}

func WithConcurrency(cfg *ConcurrencyConfig) Option {
	return func(c *engineConfig) {
		if cfg == nil {
			return
		}
		if c.concurrencyConfig == nil {
			c.concurrencyConfig = defaultConcurrencyConfig()
		}
		if cfg.ConcurrencyCount > 0 {
			c.concurrencyConfig.ConcurrencyCount = cfg.ConcurrencyCount
		}
		if cfg.RecvBufferSize > 0 {
			c.concurrencyConfig.RecvBufferSize = cfg.RecvBufferSize
		}
		if cfg.RecvTimeout > 0 {
			c.concurrencyConfig.RecvTimeout = cfg.RecvTimeout
		}
		if cfg.CleanupDelay > 0 {
			c.concurrencyConfig.CleanupDelay = cfg.CleanupDelay
		}
		if cfg.SubscriptionWorkerCount > 0 {
			c.concurrencyConfig.SubscriptionWorkerCount = cfg.SubscriptionWorkerCount
		}
	}
}
