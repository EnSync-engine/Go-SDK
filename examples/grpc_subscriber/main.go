package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/EnSync-engine/Go-SDK/common"
	ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

func main() {
	ctx := context.Background()

	// Create engine - this establishes the gRPC connection
	engine, err := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
	}

	// Authenticate with EnSync protocol over the established gRPC connection
	err = engine.CreateClient("your-access-key")
	if err != nil {
		log.Fatalf("Failed to authenticate client: %v", err)
	}

	// Set up cleanup after all potential fatal errors
	defer func() {
		if err := engine.Close(); err != nil {
			log.Printf("Failed to close engine: %v", err)
		}
	}()

	// Subscribe to event
	eventName := "yourcompany/payment/POS/PAYMENT_SUCCESSFUL"
	subscription, err := engine.Subscribe(eventName, &common.SubscribeOptions{
		AutoAck: true,
	})
	if err != nil {
		log.Printf("Failed to subscribe: %v", err)
		return
	}

	// Register event handler
	subscription.AddHandler(func(event *common.EventPayload) error {
		log.Printf("Received event: %s", event.EventName)
		log.Printf("Event ID: %s", event.Idem)
		log.Printf("Event Block: %d", event.Block)
		log.Printf("Event Data: %+v", event.Payload)
		log.Printf("Event Timestamp: %v", event.Timestamp)
		log.Printf("Event Sender: %s", event.Sender)

		// Process the event
		// For example, update database, send notification, etc.
		processPaymentEvent(event)

		return nil
	})

	log.Println("Listening for events... Press Ctrl+C to exit")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	if err := subscription.Unsubscribe(); err != nil {
		log.Printf("Failed to unsubscribe: %v", err)
	}
}

func processPaymentEvent(event *common.EventPayload) {
	// Extract payment data
	transactionID, _ := event.Payload["transactionId"].(string)
	amount, _ := event.Payload["amount"].(float64)
	terminal, _ := event.Payload["terminal"].(string)

	log.Printf("Processing payment: ID=%s, Amount=%.2f, Terminal=%s",
		transactionID, amount, terminal)

	// Add your business logic here:
	// - Update transaction status in database
	// - Send confirmation email
	// - trigger inventory update
	// - etc.
}
