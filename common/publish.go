package common

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

type PublishToRecipientFunc func(ctx context.Context, eventName, recipient string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte, useHybrid bool) (string, error)

type publisherEngine interface {
	IsConnected() bool
	IsClosed() bool
	GetClientID() string
	GetConcurrencyCount() int
	Logger() Logger
}

func Publish(
	e publisherEngine,
	publishToRecipient PublishToRecipientFunc,
	ctx context.Context,
	eventName string,
	recipients []string,
	payload map[string]interface{},
	metadata *MessageMetadata,
	options *PublishOptions,
) ([]string, error) {
	if e.IsClosed() {
		return nil, NewEnSyncError("engine is closed", ErrTypeConnection, nil)
	}

	if !e.IsConnected() {
		return nil, NewEnSyncError("client not connected/authenticated", ErrTypeConnection, nil)
	}

	if len(recipients) == 0 {
		return nil, NewEnSyncError("recipients required", ErrTypeValidation, nil)
	}

	useHybridEncryption := true
	if options != nil {
		useHybridEncryption = options.UseHybridEncryption
	}

	publishData, err := preparePublishData(payload, metadata)
	if err != nil {
		return nil, err
	}

	return executePublish(
		e,
		publishToRecipient,
		ctx,
		eventName, recipients,
		publishData.PayloadJSON, publishData.MetadataJSON, publishData.PayloadMetaJSON,
		useHybridEncryption,
	)
}

func preparePublishData(
	payload map[string]interface{},
	metadata *MessageMetadata,
) (*PublishData, error) {
	if metadata == nil {
		metadata = &MessageMetadata{
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

func executePublish(
	e publisherEngine,
	publishToRecipient PublishToRecipientFunc,
	ctx context.Context,
	eventName string,
	recipients []string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	useHybrid bool,
) ([]string, error) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, e.GetConcurrencyCount())

	responses := make([]string, len(recipients))
	errors := make([]error, len(recipients))

	for i, recipient := range recipients {
		wg.Add(1)
		go func(i int, recipient string) {
			defer wg.Done()

			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				errors[i] = ctx.Err()
				return
			}
			defer func() { <-semaphore }()

			var messageID string
			var err error

			messageID, err = publishToRecipient(ctx, eventName, recipient, payloadJSON, metadataJSON, payloadMetaJSON, useHybrid)

			if err != nil {
				errors[i] = err
				logger := e.Logger()
				if logger != nil {
					logger.Error("Publish to recipient failed",
						zap.String("recipient", recipient),
						zap.Error(err))
				}
				return
			}

			responses[i] = messageID
		}(i, recipient)
	}

	wg.Wait()

	errorCount := 0
	for _, err := range errors {
		if err != nil {
			errorCount++
		}
	}

	if errorCount > 0 {
		if errorCount == len(recipients) {
			return responses, NewEnSyncError(fmt.Sprintf("all %d publish requests failed", errorCount), ErrTypePublish, nil)
		}
		return responses, NewEnSyncError(fmt.Sprintf("%d/%d publish requests failed", errorCount, len(recipients)), ErrTypePublish, nil)
	}

	return responses, nil
}
