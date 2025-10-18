package common

import (
	"testing"
	"time"
)

func TestAnalyzePayload(t *testing.T) {
	tests := []struct {
		name     string
		payload  map[string]interface{}
		expected map[string]string
	}{
		{
			name: "simple types",
			payload: map[string]interface{}{
				"string_field":  "test",
				"number_field":  42,
				"float_field":   3.14,
				"boolean_field": true,
				"null_field":    nil,
			},
			expected: map[string]string{
				"string_field":  "string",
				"number_field":  "number",
				"float_field":   "number",
				"boolean_field": "boolean",
				"null_field":    "null",
			},
		},
		{
			name: "complex types",
			payload: map[string]interface{}{
				"array_field":   []interface{}{1, 2, 3},
				"object_field":  map[string]interface{}{"nested": "value"},
				"unknown_field": struct{ Field string }{Field: "test"},
			},
			expected: map[string]string{
				"array_field":   "array",
				"object_field":  "object",
				"unknown_field": "unknown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzePayload(tt.payload)

			if result == nil {
				t.Fatal("AnalyzePayload returned nil")
			}

			if result.ByteSize <= 0 {
				t.Error("Expected ByteSize to be greater than 0")
			}

			if len(result.Skeleton) != len(tt.expected) {
				t.Errorf("Expected skeleton length %d, got %d", len(tt.expected), len(result.Skeleton))
			}

			for key, expectedType := range tt.expected {
				if actualType, exists := result.Skeleton[key]; !exists {
					t.Errorf("Expected key %s not found in skeleton", key)
				} else if actualType != expectedType {
					t.Errorf("Expected type %s for key %s, got %s", expectedType, key, actualType)
				}
			}
		})
	}
}

func TestAnalyzePayloadEmptyPayload(t *testing.T) {
	result := analyzePayload(map[string]interface{}{})

	if result == nil {
		t.Fatal("AnalyzePayload returned nil")
	}

	if result.ByteSize <= 0 {
		t.Error("Expected ByteSize to be greater than 0 even for empty payload")
	}

	if len(result.Skeleton) != 0 {
		t.Error("Expected empty skeleton for empty payload")
	}
}

func TestEventPayloadStructure(t *testing.T) {
	now := time.Now()
	payload := EventPayload{
		EventName: "test/event",
		Idem:      "test-id",
		Block:     123,
		Timestamp: now,
		Payload:   map[string]interface{}{"test": "data"},
		Metadata:  map[string]interface{}{"source": "test"},
		Sender:    "test-sender",
	}

	if payload.EventName != "test/event" {
		t.Errorf("Expected EventName 'test/event', got %s", payload.EventName)
	}

	if payload.Idem != "test-id" {
		t.Errorf("Expected Idem 'test-id', got %s", payload.Idem)
	}

	if payload.Block != 123 {
		t.Errorf("Expected Block 123, got %d", payload.Block)
	}

	if !payload.Timestamp.Equal(now) {
		t.Errorf("Expected Timestamp %v, got %v", now, payload.Timestamp)
	}

	if payload.Sender != "test-sender" {
		t.Errorf("Expected Sender 'test-sender', got %s", payload.Sender)
	}
}

func TestEventMetadata(t *testing.T) {
	metadata := EventMetadata{
		Persist: true,
		Headers: map[string]string{
			"content-type": "application/json",
			"source":       "test-service",
		},
	}

	if !metadata.Persist {
		t.Error("Expected Persist to be true")
	}

	if len(metadata.Headers) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(metadata.Headers))
	}

	if metadata.Headers["content-type"] != "application/json" {
		t.Error("Expected content-type header to be application/json")
	}

	if metadata.Headers["source"] != "test-service" {
		t.Error("Expected source header to be test-service")
	}
}

func TestPublishOptions(t *testing.T) {
	options := PublishOptions{}
	if options.UseHybridEncryption {
		t.Error("Expected UseHybridEncryption to be false by default")
	}

	options.UseHybridEncryption = true
	if !options.UseHybridEncryption {
		t.Error("Expected UseHybridEncryption to be true")
	}
}

func TestSubscribeOptions(t *testing.T) {
	options := SubscribeOptions{
		AutoAck:      true,
		AppSecretKey: "test-secret",
	}

	if !options.AutoAck {
		t.Error("Expected AutoAck to be true")
	}

	if options.AppSecretKey != "test-secret" {
		t.Errorf("Expected AppSecretKey 'test-secret', got %s", options.AppSecretKey)
	}
}

func TestResponseTypes(t *testing.T) {
	now := time.Now()

	deferResp := DeferResponse{
		Status:            "success",
		Action:            "defer",
		EventID:           "test-event",
		DelayMs:           5000,
		ScheduledDelivery: now,
		Timestamp:         now,
	}

	if deferResp.Status != "success" {
		t.Errorf("Expected Status 'success', got %s", deferResp.Status)
	}

	if deferResp.DelayMs != 5000 {
		t.Errorf("Expected DelayMs 5000, got %d", deferResp.DelayMs)
	}

	discardResp := DiscardResponse{
		Status:    "success",
		Action:    "discard",
		EventID:   "test-event",
		Timestamp: now,
	}

	if discardResp.Action != "discard" {
		t.Errorf("Expected Action 'discard', got %s", discardResp.Action)
	}

	pauseResp := PauseResponse{
		Status:    "success",
		Action:    "pause",
		EventName: "test/event",
		Reason:    "maintenance",
	}

	if pauseResp.Reason != "maintenance" {
		t.Errorf("Expected Reason 'maintenance', got %s", pauseResp.Reason)
	}

	continueResp := ContinueResponse{
		Status:    "success",
		Action:    "continue",
		EventName: "test/event",
	}

	if continueResp.Action != "continue" {
		t.Errorf("Expected Action 'continue', got %s", continueResp.Action)
	}
}

func TestPayloadMetadata(t *testing.T) {
	metadata := PayloadMetadata{
		ByteSize: 1024,
		Skeleton: map[string]string{
			"field1": "string",
			"field2": "number",
		},
	}

	if metadata.ByteSize != 1024 {
		t.Errorf("Expected ByteSize 1024, got %d", metadata.ByteSize)
	}

	if len(metadata.Skeleton) != 2 {
		t.Errorf("Expected 2 skeleton fields, got %d", len(metadata.Skeleton))
	}
}
