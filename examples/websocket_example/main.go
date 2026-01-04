package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/EnSync-engine/Go-SDK/common"
	ensync "github.com/EnSync-engine/Go-SDK/websocket"
)

func main() {
	ctx := context.Background()

	// Create engine - this establishes the WebSocket connection
	engine, err := ensync.NewWebSocketEngine(ctx, "ws://localhost:8080")
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
	}

	// Authenticate with EnSync protocol over the established WebSocket
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

	// Subscribe
	subscription, err := engine.Subscribe("test-event", &common.SubscribeOptions{
		AutoAck: true,
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// Register event handler
	subscription.AddMessageHandler(func(ctx *common.MessageContext) {
		log.Printf("Received event: %s", ctx.Message.MessageName)
		log.Printf("Event ID: %s", ctx.Message.Idem)
		log.Printf("Event Block: %d", ctx.Message.Block)
		log.Printf("Event Data: %+v", ctx.Message.Payload)
		log.Printf("Event Timestamp: %v", ctx.Message.Timestamp)
		log.Printf("Event Sender: %s", ctx.Message.Sender)

		// Process the event
		// For example, update database, send notification, etc.
		processPaymentEvent(ctx.Message)
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

func processPaymentEvent(event *common.MessagePayload) {
	// Extract payment data
	transactionID, _ := event.Payload["transactionId"].(string)
	amount, _ := event.Payload["amount"].(float64)
	terminal, _ := event.Payload["terminal"].(string)

	log.Printf("Processing payment: ID=%s, Amount=%.2f, Terminal=%s",
		transactionID, amount, terminal)

	// Add your business logic here:
	// - Update transaction status in database
	// - Send confirmation email
	// - Trigger inventory update
	// - etc.
}
