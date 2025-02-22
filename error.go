package ensync

const (
	GenericMessage  = "Verify your EnSync engine is still operating"
	GeneralResponse = "Failed to establish a connection with an EnSync engine"
)

// EnSyncError represents a custom error type for EnSync operations
type EnSyncError struct {
	Name    string
	Message string
}

// Error implements the error interface
func (e *EnSyncError) Error() string {
	return e.Message
}

// NewEnSyncError creates a new EnSync error with the given message and name
func NewEnSyncError(msg interface{}, name string) *EnSyncError {
	var message string

	switch v := msg.(type) {
	case string:
		message = v
	case error:
		message = v.Error()
	default:
		message = GeneralResponse
	}

	if name == "" {
		name = "EnSyncGenericError"
	}

	return &EnSyncError{
		Name:    name,
		Message: message,
	}
}
