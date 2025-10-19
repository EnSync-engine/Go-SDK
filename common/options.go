package common

import "time"

const (
	defaultOperationTimeout        = 5 * time.Second
	defaultGracefulShutdownTimeout = 5 * time.Second
	defaultPingInterval            = 10 * time.Second
	defaultPingTimeout             = 5 * time.Second
)

type timeoutConfig struct {
	OperationTimeout        time.Duration
	GracefulShutdownTimeout time.Duration
	PingInterval            time.Duration
	PingTimeout             time.Duration
}

type TimeoutOption func(*timeoutConfig)

func WithOperationTimeout(d time.Duration) TimeoutOption {
	return func(c *timeoutConfig) { c.OperationTimeout = d }
}

func WithGracefulShutdownTimeout(d time.Duration) TimeoutOption {
	return func(c *timeoutConfig) {
		c.GracefulShutdownTimeout = d
	}
}

func WithPingTimeout(d time.Duration) TimeoutOption {
	return func(c *timeoutConfig) {
		c.PingTimeout = d
	}
}

func WithPingInterval(d time.Duration) TimeoutOption {
	return func(c *timeoutConfig) {
		c.PingInterval = d
	}
}

func defaultTimeoutConfig() *timeoutConfig {
	return &timeoutConfig{
		OperationTimeout:        defaultOperationTimeout,
		GracefulShutdownTimeout: defaultGracefulShutdownTimeout,
		PingInterval:            defaultPingInterval,
		PingTimeout:             defaultPingTimeout,
	}
}

type engineConfig struct {
	logger         Logger
	circuitBreaker *circuitBreakerConfig
	retryConfig    *retryConfig
	timeoutConfig  *timeoutConfig
}

type Option func(*engineConfig)

func defaultEngineConfig() *engineConfig {
	return &engineConfig{
		logger:         &noopLogger{},
		circuitBreaker: defaultCircuitBreakerConfig(),
		retryConfig:    defaultRetryConfig(),
		timeoutConfig:  defaultTimeoutConfig(),
	}
}

func WithLogger(logger Logger) Option {
	return func(c *engineConfig) {
		if logger != nil {
			c.logger = logger
		}
	}
}

func WithCircuitBreaker(failureThreshold int, resetTimeout, maxResetTimeout time.Duration) Option {
	return func(c *engineConfig) {
		if c.circuitBreaker == nil {
			c.circuitBreaker = defaultCircuitBreakerConfig()
		}
		c.circuitBreaker.FailureThreshold = failureThreshold
		c.circuitBreaker.ResetTimeout = resetTimeout
		c.circuitBreaker.MaxResetTimeout = maxResetTimeout
	}
}

func WithRetryConfig(maxAttempts int, initialBackoff, maxBackoff time.Duration, jitter float64) Option {
	return func(c *engineConfig) {
		if c.retryConfig == nil {
			c.retryConfig = defaultRetryConfig()
		}
		c.retryConfig.MaxAttempts = maxAttempts
		c.retryConfig.InitialBackoff = initialBackoff
		c.retryConfig.MaxBackoff = maxBackoff
		c.retryConfig.Jitter = jitter
	}
}

func WithDefaultRetryConfig() Option {
	return func(c *engineConfig) {
		c.retryConfig = defaultRetryConfig()
	}
}

func WithTimeoutOptions(opts ...TimeoutOption) Option {
	return func(c *engineConfig) {
		for _, opt := range opts {
			opt(c.timeoutConfig)
		}
	}
}

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
