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
		log.Printf("Failed to create engine: %v", err)
		return
	}

	// Authenticate with EnSync protocol over the established gRPC connection
	err = engine.CreateClient("access-key")
	if err != nil {
		log.Printf("Failed to authenticate client: %v", err)
		return
	}

	// Set up cleanup after all potential fatal errors
	defer func() {
		if err := engine.Close(); err != nil {
			log.Printf("Failed to close engine: %v", err)
		}
	}()

	// Subscribe to event
	subscription, err := engine.Subscribe("test-event", &common.SubscribeOptions{
		AutoAck:      true,
		AppSecretKey: "app-secret-key",
	})
	if err != nil {
		log.Printf("Subscribe failed: %v", err)
		return
	}

	// Register event handler
	subscription.AddMessageHandler(func(ctx *common.MessageContext) {
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
	// - trigger inventory update
	// - etc.
}
