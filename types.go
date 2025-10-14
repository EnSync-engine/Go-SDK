package ensync

import "time"

// EventMetadata represents metadata for an event
type EventMetadata struct {
	Persist bool              `json:"persist"`
	Headers map[string]string `json:"headers"`
}

// EventPayload represents a received event
type EventPayload struct {
	EventName string                 `json:"eventName"`
	Idem      string                 `json:"idem"`
	Block     int64                  `json:"block"`
	Timestamp time.Time              `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"`
	Metadata  map[string]interface{} `json:"metadata"`
	Sender    string                 `json:"sender"`
}

// PublishOptions contains options for publishing events
type PublishOptions struct {
	UseHybridEncryption bool
}

// SubscribeOptions contains options for subscribing to events
type SubscribeOptions struct {
	AutoAck      bool
	AppSecretKey string
}

// DeferResponse represents the response from a defer operation
type DeferResponse struct {
	Status            string    `json:"status"`
	Action            string    `json:"action"`
	EventID           string    `json:"eventId"`
	DelayMs           int64     `json:"delayMs"`
	ScheduledDelivery time.Time `json:"scheduledDelivery"`
	Timestamp         time.Time `json:"timestamp"`
}

// DiscardResponse represents the response from a discard operation
type DiscardResponse struct {
	Status    string    `json:"status"`
	Action    string    `json:"action"`
	EventID   string    `json:"eventId"`
	Timestamp time.Time `json:"timestamp"`
}

// PauseResponse represents the response from a pause operation
type PauseResponse struct {
	Status    string `json:"status"`
	Action    string `json:"action"`
	EventName string `json:"eventName"`
	Reason    string `json:"reason,omitempty"`
}

// ContinueResponse represents the response from a continue operation
type ContinueResponse struct {
	Status    string `json:"status"`
	Action    string `json:"action"`
	EventName string `json:"eventName"`
}

// PayloadMetadata represents metadata about a payload
type PayloadMetadata struct {
	ByteSize int               `json:"byte_size"`
	Skeleton map[string]string `json:"skeleton"`
}

// EventHandler is a function that handles incoming events
type EventHandler func(*EventPayload) error

// Subscription represents an active subscription to an event
type Subscription interface {
	// AddHandler registers an event handler for this subscription
	AddHandler(handler EventHandler) func()

	// Ack acknowledges an event
	Ack(eventIdem string, block int64) error

	// Resume resumes event processing
	Resume() error

	// Pause pauses event processing
	Pause(reason string) error

	// Defer defers an event for later processing
	Defer(eventIdem string, delayMs int64, reason string) (*DeferResponse, error)

	// Discard permanently discards an event
	Discard(eventIdem string, reason string) (*DiscardResponse, error)

	// Rollback rolls back an event
	Rollback(eventIdem string, block int64) error

	// Replay requests a specific event to be sent again
	Replay(eventIdem string) (*EventPayload, error)

	// Unsubscribe unsubscribes from the event
	Unsubscribe() error
}

// Engine is the main interface for EnSync clients
type Engine interface {
	// CreateClient creates and authenticates a new client
	CreateClient(accessKey string, options ...ClientOption) error

	// Publish publishes an event to the EnSync system
	Publish(eventName string, recipients []string, payload map[string]interface{}, metadata *EventMetadata, options *PublishOptions) (string, error)

	// Subscribe subscribes to an event
	Subscribe(eventName string, options *SubscribeOptions) (Subscription, error)

	// Close closes the connection
	Close() error

	// GetClientPublicKey returns the client's public key
	GetClientPublicKey() string

	// AnalyzePayload analyzes a payload and returns metadata
	AnalyzePayload(payload map[string]interface{}) *PayloadMetadata
}

// ClientOption is a function that configures a client
type ClientOption func(*clientConfig)

type clientConfig struct {
	appSecretKey string
	clientID     string
}

// WithAppSecretKey sets the app secret key for the client
func WithAppSecretKey(key string) ClientOption {
	return func(c *clientConfig) {
		c.appSecretKey = key
	}
}

// WithClientID sets a custom client ID
func WithClientID(id string) ClientOption {
	return func(c *clientConfig) {
		c.clientID = id
	}
}
