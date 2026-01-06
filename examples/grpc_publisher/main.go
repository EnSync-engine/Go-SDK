package main

import (
	"context"
	"log"

	"github.com/EnSync-engine/Go-SDK/common"
	ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

func main() {
	ctx := context.Background()

	// Create a new gRPC engine
	engine, err := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
	}

	// Create and authenticate client
	err = engine.CreateClient("access-key")
	if err != nil {
		log.Printf("Failed to subscribe: %v", err)
		return
	}

	defer func() {
		if err := engine.Close(); err != nil {
			log.Printf("Failed to close engine: %v", err)
		}
	}()

	// Prepare event data
	eventName := "company/payment/POS/PAYMENT_SUCCESSFUL"
	recipients := []string{"recipient-public-key-base64"}
	payload := map[string]interface{}{
		"transactionId": "123",
		"amount":        100,
		"terminal":      "pos-1",
		"timestamp":     1234567890,
	}

	metadata := &common.MessageMetadata{
		Persist: true,
		Headers: map[string]string{
			"source": "pos-system",
		},
	}

	// Publish event
	eventID, err := engine.Publish(ctx, eventName, recipients, payload, metadata, nil)
	if err != nil {
		log.Printf("Failed to publish event: %v", err)
		return
	}

	log.Printf("Event published successfully with ID: %s", eventID)
}
