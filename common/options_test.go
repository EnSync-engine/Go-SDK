package common

import (
	"testing"
	"time"
)

func TestDefaultEngineConfig(t *testing.T) {
	config := defaultEngineConfig()

	if config == nil {
		t.Fatal("defaultEngineConfig() returned nil")
	}

	if config.logger == nil {
		t.Error("Expected logger to be initialized")
	}

	if config.circuitBreaker == nil {
		t.Error("Expected circuitBreaker to be initialized")
	}

	if config.retryConfig == nil {
		t.Error("Expected retryConfig to be initialized")
	}
}

func TestWithLogger(t *testing.T) {
	config := &engineConfig{}
	logger := &testLogger{}
	option := WithLogger(logger)
	option(config)

	if config.logger == nil {
		t.Error("Expected logger to be set")
	}

	// Test with nil logger (should not change config)
	originalLogger := config.logger
	option = WithLogger(nil)
	option(config)
	if config.logger != originalLogger {
		t.Error("Expected logger to remain unchanged when nil is passed")
	}
}

func TestWithCircuitBreaker(t *testing.T) {
	config := &engineConfig{}
	threshold := 5
	resetTimeout := 30 * time.Second

	option := WithCircuitBreaker(threshold, resetTimeout)
	option(config)

	if config.circuitBreaker == nil {
		t.Fatal("Expected circuitBreaker to be initialized")
	}

	if config.circuitBreaker.FailureThreshold != threshold {
		t.Errorf("Expected FailureThreshold %d, got %d", threshold, config.circuitBreaker.FailureThreshold)
	}

	if config.circuitBreaker.ResetTimeout != resetTimeout {
		t.Errorf("Expected ResetTimeout %v, got %v", resetTimeout, config.circuitBreaker.ResetTimeout)
	}
}

func TestWithRetryConfig(t *testing.T) {
	config := &engineConfig{}
	maxAttempts := 3
	initialBackoff := time.Second
	maxBackoff := 10 * time.Second
	jitter := 0.1

	option := WithRetryConfig(maxAttempts, initialBackoff, maxBackoff, jitter)
	option(config)

	if config.retryConfig == nil {
		t.Fatal("Expected retryConfig to be initialized")
	}

	if config.retryConfig.MaxAttempts != maxAttempts {
		t.Errorf("Expected MaxAttempts %d, got %d", maxAttempts, config.retryConfig.MaxAttempts)
	}

	if config.retryConfig.InitialBackoff != initialBackoff {
		t.Errorf("Expected InitialBackoff %v, got %v", initialBackoff, config.retryConfig.InitialBackoff)
	}

	if config.retryConfig.MaxBackoff != maxBackoff {
		t.Errorf("Expected MaxBackoff %v, got %v", maxBackoff, config.retryConfig.MaxBackoff)
	}

	if config.retryConfig.Jitter != jitter {
		t.Errorf("Expected Jitter %f, got %f", jitter, config.retryConfig.Jitter)
	}
}

func TestWithDefaultRetryConfig(t *testing.T) {
	config := &engineConfig{}
	option := WithDefaultRetryConfig()
	option(config)

	if config.retryConfig == nil {
		t.Fatal("Expected retryConfig to be initialized")
	}

	defaultConfig := defaultRetryConfig()
	if config.retryConfig.MaxAttempts != defaultConfig.MaxAttempts {
		t.Errorf("Expected MaxAttempts %d, got %d", defaultConfig.MaxAttempts, config.retryConfig.MaxAttempts)
	}
}

// Test logger implementation for testing
type testLogger struct {
	logs []string
}

func (l *testLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logs = append(l.logs, "INFO: "+msg)
}

func (l *testLogger) Error(msg string, keysAndValues ...interface{}) {
	l.logs = append(l.logs, "ERROR: "+msg)
}

func (l *testLogger) Debug(msg string, keysAndValues ...interface{}) {
	l.logs = append(l.logs, "DEBUG: "+msg)
}

func (l *testLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.logs = append(l.logs, "WARN: "+msg)
}
