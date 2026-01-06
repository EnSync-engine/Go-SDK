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
		MaxAttempts:    defaultMaxRetries,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Jitter:         0.2,
	}
}

func (b *BaseEngine) Retry(ctx context.Context, fn func() error, cfg *retryConfig) error {
	var attempt int
	var lastErr error

	for {
		attempt++

		// Check circuit breaker
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
			if attempt == 1 {
				b.recordSuccess() // Only record success on first attempt
			}
			return nil
		}
		lastErr = err

		if !isRetryableError(err) {
			b.recordFailure()
			return err
		}

		if attempt >= cfg.MaxAttempts {
			b.recordFailure()
			return NewEnSyncError(
				"max retry attempts reached",
				ErrTypeMaxRetries,
				lastErr,
			)
		}

		backoff := b.calculateBackoff(cfg, attempt)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
}

func (b *BaseEngine) calculateBackoff(cfg *retryConfig, attempt int) time.Duration {
	if attempt == 0 {
		return 0
	}

	backoff := float64(cfg.InitialBackoff) * math.Pow(retryBackoffBase, float64(attempt-1))
	if maxBackoff := float64(cfg.MaxBackoff); backoff > maxBackoff {
		backoff = maxBackoff
	}

	backoff *= (1 + cfg.Jitter*(2*secureRandFloat64()-1))
	return time.Duration(backoff)
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return true
	}

	if errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		strings.Contains(err.Error(), "connection") {
		return true
	}

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
