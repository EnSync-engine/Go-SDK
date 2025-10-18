package common

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"time"
)

const (
	operationTimeout = 1 * time.Second
)

type engineState struct {
	Mu              sync.RWMutex
	IsConnected     bool
	IsAuthenticated bool
}

type BaseEngineBuilder struct {
	config *engineConfig
}

func NewBaseEngineBuilder() *BaseEngineBuilder {
	return &BaseEngineBuilder{
		config: &engineConfig{},
	}
}

func (b *BaseEngineBuilder) WithConfigOptions(opts ...Option) *BaseEngineBuilder {
	for _, opt := range opts {
		opt(b.config)
	}
	return b
}

func (b *BaseEngineBuilder) Build(ctx context.Context) (*BaseEngine, error) {
	if b.config == nil {
		b.config = defaultEngineConfig()
	}

	if b.config.logger == nil {
		b.config.logger = &noopLogger{}
	}

	if b.config.retryConfig == nil {
		b.config.retryConfig = defaultRetryConfig()
	}

	engine := &BaseEngine{
		config:          b.config,
		State:           engineState{},
		Ctx:             ctx,
		Logger:          b.config.logger,
		SubscriptionMgr: newSubscriptionManager(),
		retryConfig:     b.config.retryConfig,
	}

	if b.config.circuitBreaker != nil {
		engine.circuitBreaker = newCircuitBreaker(b.config.circuitBreaker, b.config.logger)
	}

	return engine, nil
}

type Operation struct {
	Name    string
	Execute func(context.Context) error
}

type BaseEngine struct {
	AccessKey       string
	AppSecretKey    string
	State           engineState
	ClientID        string
	Logger          Logger
	ClientHash      string
	Ctx             context.Context
	config          *engineConfig
	circuitBreaker  *circuitBreaker
	retryConfig     *retryConfig
	SubscriptionMgr *SubscriptionManager
}

func NewBaseEngine(ctx context.Context, opts ...Option) (*BaseEngine, error) {
	baseEngine, err := NewBaseEngineBuilder().
		WithConfigOptions(opts...).
		Build(ctx)

	if err != nil {
		return nil, err
	}

	return baseEngine, nil
}

func (b *BaseEngine) recordFailure() {
	if b.circuitBreaker == nil {
		return
	}

	oldState := b.circuitBreaker.state
	b.circuitBreaker.RecordFailure()
	newState := b.circuitBreaker.state

	if oldState != newState && b.Logger != nil {
		b.Logger.Info("Circuit breaker state changed",
			"oldState", oldState,
			"newState", newState,
			"failures", b.circuitBreaker.failures)
	}
}

func (b *BaseEngine) recordSuccess() {
	if b.circuitBreaker == nil {
		return
	}

	oldState := b.circuitBreaker.state
	b.circuitBreaker.RecordSuccess()
	newState := b.circuitBreaker.state

	if oldState != newState && b.Logger != nil {
		b.Logger.Info("Circuit breaker state changed",
			"oldState", oldState,
			"newState", newState)
	}
}

func (b *BaseEngine) canAttemptConnection() bool {
	if b.circuitBreaker == nil {
		return true
	}
	return b.circuitBreaker.canAttempt()
}

func (e *BaseEngine) DecryptEventPayload(
	encryptedPayloadBase64 string,
	decryptionKey string,
) (map[string]interface{}, error) {
	if decryptionKey == "" {
		return nil, NewEnSyncError("no decryption key available", ErrTypeGeneric, nil)
	}

	decodedPayload, err := base64.StdEncoding.DecodeString(encryptedPayloadBase64)
	if err != nil {
		return nil, NewEnSyncError("failed to decode payload", ErrTypeGeneric, err)
	}

	encryptedPayload, err := parseEncryptedPayload(string(decodedPayload))
	if err != nil {
		return nil, NewEnSyncError("failed to parse encrypted payload", ErrTypeGeneric, err)
	}

	keyBytes, err := base64.StdEncoding.DecodeString(decryptionKey)
	if err != nil {
		return nil, NewEnSyncError("failed to decode decryption key", ErrTypeGeneric, err)
	}

	var payloadStr string

	switch v := encryptedPayload.(type) {
	case *HybridEncryptedMessage:
		payloadStr, err = decryptHybridMessage(v, keyBytes)
		if err != nil {
			return nil, NewEnSyncError("hybrid decryption failed", ErrTypeGeneric, err)
		}
	case *EncryptedMessage:
		payloadStr, err = DecryptEd25519(v, keyBytes)
		if err != nil {
			return nil, NewEnSyncError("ed25519 decryption failed", ErrTypeGeneric, err)
		}
	default:
		return nil, NewEnSyncError("unknown encrypted payload type", ErrTypeGeneric, nil)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return nil, NewEnSyncError("failed to unmarshal decrypted payload", ErrTypeGeneric, err)
	}

	return payload, nil
}

func (e *BaseEngine) ParseMetadata(
	metadata map[string]interface{},
) map[string]interface{} {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	return metadata
}

func (e *BaseEngine) PreparePublishData(
	payload map[string]interface{},
	metadata *EventMetadata,
) (*PublishData, error) {
	if metadata == nil {
		metadata = &EventMetadata{
			Persist: true,
			Headers: make(map[string]string),
		}
	}

	payloadMeta := analyzePayload(payload)
	payloadMetaJSON, err := json.Marshal(payloadMeta)
	if err != nil {
		return nil, NewEnSyncError("failed to marshal payload metadata", ErrTypePublish, err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, NewEnSyncError("failed to marshal payload", ErrTypePublish, err)
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, NewEnSyncError("failed to marshal metadata", ErrTypePublish, err)
	}

	return &PublishData{
		PayloadJSON:     payloadJSON,
		MetadataJSON:    metadataJSON,
		PayloadMetaJSON: payloadMetaJSON,
	}, nil
}

func (e *BaseEngine) ValidatePublishInput(eventName string, recipients []string) error {
	e.State.Mu.RLock()
	isAuth := e.State.IsAuthenticated
	e.State.Mu.RUnlock()

	if !isAuth {
		return NewEnSyncError("client not authenticated", ErrTypeAuth, nil)
	}

	if len(recipients) == 0 {
		return NewEnSyncError("recipients required", ErrTypeValidation, nil)
	}

	return nil
}

func (e *BaseEngine) ValidateSubscribeInput(eventName string) error {
	e.State.Mu.RLock()
	isAuth := e.State.IsAuthenticated
	e.State.Mu.RUnlock()

	if !isAuth {
		return ErrNotAuthenticated
	}

	if e.SubscriptionMgr.Exists(eventName) {
		return NewEnSyncError("already subscribed to event "+eventName, ErrTypeSubscription, nil)
	}

	return nil
}

func (e *BaseEngine) ExecuteOperation(op Operation) error {
	return e.WithRetry(e.Ctx, func() error {
		timeout := e.config.operationTimeout
		ctx, cancel := context.WithTimeout(e.Ctx, timeout)
		defer cancel()
		err := op.Execute(ctx)
		if err != nil {
			if e.Logger != nil {
				if ctx.Err() != nil {
					e.Logger.Warn("Operation failed due to context cancellation",
						"operation", op.Name,
						"ctxErr", ctx.Err().Error(),
						"error", err,
					)
				} else {
					e.Logger.Warn("Operation failed",
						"operation", op.Name,
						"error", err,
					)
				}
			}
		}
		return err
	})
}
