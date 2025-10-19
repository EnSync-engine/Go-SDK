package common

import (
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	retryBackoffBase = 2.0
)

type retryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Jitter         float64
}

func defaultRetryConfig() *retryConfig {
	return &retryConfig{
		MaxAttempts:    3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Jitter:         0.2,
	}
}

func (b *BaseEngine) Retry(ctx context.Context, fn func() error, cfg *retryConfig) error {
	if cfg.MaxAttempts <= 0 {
		return NewEnSyncError("invalid retry config: MaxAttempts must be > 0", ErrTypeValidation, nil)
	}

	var attempt int
	var lastErr error

	for attempt < cfg.MaxAttempts {
		// Check circuit breaker *before* any attempt. This ensures we fail fast.
		if !b.canAttemptConnection() {
			return NewEnSyncError(
				"circuit breaker open",
				ErrTypeCircuitBreaker,
				lastErr,
			)
		}

		// Execute the operation
		err := fn()
		if err == nil {
			b.recordSuccess()
			return nil
		}
		lastErr = err

		// Don't retry non-retryable errors
		if !isRetryableError(err) {
			b.recordFailure()
			return err
		}

		attempt++
		if attempt >= cfg.MaxAttempts {
			break // Exit loop to return max retries error
		}

		// Calculate backoff with jitter
		backoff := b.calculateBackoff(cfg, attempt)

		// Wait or return if context is done
		select {
		case <-ctx.Done():
			b.recordFailure()
			return ctx.Err()
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}

	b.recordFailure()
	return NewEnSyncError(
		"max retry attempts reached",
		ErrTypeMaxRetries,
		lastErr,
	)
}

func (b *BaseEngine) calculateBackoff(cfg *retryConfig, attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// Ensure attempt is bounded to prevent overflow
	maxExponent := 30
	exponent := attempt - 1
	if exponent > maxExponent {
		exponent = maxExponent
	}

	backoff := float64(cfg.InitialBackoff) * math.Pow(retryBackoffBase, float64(exponent))
	if maxBackoff := float64(cfg.MaxBackoff); backoff > maxBackoff {
		backoff = maxBackoff
	}

	jitterFactor := 1 + cfg.Jitter*(2*secureRandFloat64()-1)
	backoff *= jitterFactor

	// Ensure backoff doesn't go negative due to jitter
	if backoff < 0 {
		backoff = 0
	}

	return time.Duration(backoff)
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check context errors first
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return true
	}

	// Check I/O errors
	if errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// Check for connection-related errors in the error message
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "connection") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "temporary") {
		return true
	}

	// Check gRPC status codes
	if grpcStatus, ok := status.FromError(err); ok {
		//nolint:exhaustive // We intentionally handle only specific retryable gRPC error codes
		switch grpcStatus.Code() {
		case
			codes.Unavailable,
			codes.DeadlineExceeded,
			codes.ResourceExhausted,
			codes.Aborted,
			codes.Internal:
			return true
		default:
			return false
		}
	}

	return false
}

func (b *BaseEngine) WithRetry(ctx context.Context, fn func() error) error {
	return b.Retry(ctx, fn, b.retryConfig)
}
