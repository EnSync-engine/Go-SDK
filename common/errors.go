package common

import (
	"errors"
	"fmt"
)

const (
	ErrTypeGeneric        = "EnSyncGenericError"
	ErrTypeAuth           = "EnSyncAuthError"
	ErrTypeConnection     = "EnSyncConnectionError"
	ErrTypePublish        = "EnSyncPublishError"
	ErrTypeSubscription   = "EnSyncSubscriptionError"
	ErrTypeTimeout        = "EnSyncTimeoutError"
	ErrTypeReplay         = "EnSyncReplayError"
	ErrTypeDefer          = "EnSyncDeferError"
	ErrTypeDiscard        = "EnSyncDiscardError"
	ErrTypePause          = "EnSyncPauseError"
	ErrTypeContinue       = "EnSyncContinueError"
	ErrTypeValidation     = "EnSyncValidationError"
	ErrTypeCircuitBreaker = "EnSyncCircuitBreakerError"
	ErrTypeMaxRetries     = "EnSyncMaxRetriesError"
	ErrTypeResume         = "EnSyncResumeError"
)

const (
	GenericMessage        = "Verify your EnSync engine is still operating"
	GeneralResponse       = "Failed to establish a connection with an EnSync engine"
	EventNotFound         = "Event not found or no longer available"
	InvalidDelay          = "Delay must be between 1000ms and 24 hours"
	NotAuthenticated      = "Not authenticated"
	RecipientsRequired    = "recipients array cannot be empty"
	RecipientsMustBeArray = "recipients must be an array"
)

var (
	ErrNotAuthenticated     = errors.New("not authenticated")
	ErrNotConnected         = errors.New("not connected")
	ErrMaxRetriesExceeded   = errors.New("max reconnection attempts exceeded")
	ErrCircuitBreakerOpen   = errors.New("circuit breaker is open")
	ErrInvalidRecipients    = errors.New("recipients array cannot be empty")
	ErrConnectionClosed     = errors.New("connection closed")
	ErrSubscriptionNotFound = errors.New("subscription not found")
	ErrInvalidURL           = errors.New("invalid URL")
	ErrInvalidAccessKey     = errors.New("invalid access key")
)

type EnSyncError struct {
	Message string
	Type    string
	Err     error
}

func (e *EnSyncError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

func (e *EnSyncError) Unwrap() error {
	return e.Err
}

func NewEnSyncError(message, errType string, err error) *EnSyncError {
	return &EnSyncError{
		Message: message,
		Type:    errType,
		Err:     err,
	}
}

type ConnectionError struct {
	Addr string
	Err  error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("connection to %s failed: %v", e.Addr, e.Err)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, ErrNotAuthenticated) {
		return false
	}

	if errors.Is(err, ErrCircuitBreakerOpen) {
		return false
	}

	var connErr *ConnectionError
	if errors.As(err, &connErr) {
		return true
	}

	var ensyncErr *EnSyncError
	if errors.As(err, &ensyncErr) {
		return ensyncErr.Type == ErrTypeConnection ||
			ensyncErr.Type == ErrTypeTimeout
	}

	return true
}
