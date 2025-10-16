package common

import (
	"errors"
	"testing"
)

func TestNewEnSyncError(t *testing.T) {
	message := "test error message"
	errorType := ErrTypeAuth
	cause := errors.New("underlying error")

	err := NewEnSyncError(message, errorType, cause)

	if err == nil {
		t.Fatal("NewEnSyncError returned nil")
	}

	if err.Message != message {
		t.Errorf("Expected message '%s', got '%s'", message, err.Message)
	}

	if err.Type != errorType {
		t.Errorf("Expected type '%s', got '%s'", errorType, err.Type)
	}
}

func TestEnSyncErrorError(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		errorType   string
		cause       error
		expectedStr string
	}{
		{
			name:        "without cause",
			message:     "auth failed",
			errorType:   ErrTypeAuth,
			cause:       nil,
			expectedStr: "EnSyncAuthError: auth failed",
		},
		{
			name:        "with cause",
			message:     "connection failed",
			errorType:   ErrTypeConnection,
			cause:       errors.New("network error"),
			expectedStr: "EnSyncConnectionError: connection failed (caused by: network error)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewEnSyncError(tt.message, tt.errorType, tt.cause)
			if err.Error() != tt.expectedStr {
				t.Errorf("Expected error string '%s', got '%s'", tt.expectedStr, err.Error())
			}
		})
	}
}

func TestErrorConstants(t *testing.T) {
	// Test that all error type constants are defined
	errorTypes := []string{
		ErrTypeGeneric,
		ErrTypeAuth,
		ErrTypeConnection,
		ErrTypePublish,
		ErrTypeSubscription,
		ErrTypeTimeout,
		ErrTypeReplay,
		ErrTypeDefer,
		ErrTypeDiscard,
		ErrTypePause,
		ErrTypeContinue,
		ErrTypeValidation,
		ErrTypeCircuitBreaker,
		ErrTypeMaxRetries,
		ErrTypeResume,
	}

	for _, errorType := range errorTypes {
		if errorType == "" {
			t.Error("Error type constant is empty")
		}
	}
}

func TestErrorMessageConstants(t *testing.T) {
	// Test that all error message constants are defined
	messages := []string{
		GenericMessage,
		GeneralResponse,
		EventNotFound,
		InvalidDelay,
		NotAuthenticated,
		RecipientsRequired,
		RecipientsMustBeArray,
	}

	for _, message := range messages {
		if message == "" {
			t.Error("Error message constant is empty")
		}
	}
}

func TestPredefinedErrors(t *testing.T) {
	predefinedErrors := []error{
		ErrNotAuthenticated,
		ErrNotConnected,
		ErrMaxRetriesExceeded,
		ErrCircuitBreakerOpen,
		ErrInvalidRecipients,
		ErrConnectionClosed,
		ErrSubscriptionNotFound,
		ErrInvalidURL,
		ErrInvalidAccessKey,
	}

	for i, err := range predefinedErrors {
		if err == nil {
			t.Errorf("Predefined error at index %d is nil", i)
		}
		if err.Error() == "" {
			t.Errorf("Predefined error at index %d has empty message", i)
		}
	}
}

func TestIsEnSyncError(t *testing.T) {
	ensyncErr := NewEnSyncError("test", ErrTypeAuth, nil)
	if ensyncErr.Type != ErrTypeAuth {
		t.Errorf("Expected error type %s, got %s", ErrTypeAuth, ensyncErr.Type)
	}

	regularErr := errors.New("regular error")
	var ensyncErrPtr *EnSyncError
	if errors.As(regularErr, &ensyncErrPtr) {
		t.Error("Regular error should not be of type *EnSyncError")
	}
}

func TestErrorWrapping(t *testing.T) {
	originalErr := errors.New("original error")
	wrappedErr := NewEnSyncError("wrapped", ErrTypeGeneric, originalErr)

	errorStr := wrappedErr.Error()
	if errorStr != "EnSyncGenericError: wrapped (caused by: original error)" {
		t.Errorf("Expected wrapped error message, got: %s", errorStr)
	}
}
