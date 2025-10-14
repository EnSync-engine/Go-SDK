package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	ensync "github.com/EnSync-engine/Go-SDK"
)

func main() {
	ctx := context.Background()
	
	// Create a new WebSocket engine
	engine, err := ensync.NewWebSocketEngine(ctx, "ws://localhost:8082")
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	// Create and authenticate client
	err = engine.CreateClient("your-access-key")
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Subscribe to event
	eventName := "yourcompany/payment/POS/PAYMENT_SUCCESSFUL"
	subscription, err := engine.Subscribe(eventName, &ensync.SubscribeOptions{
		AutoAck: true,
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// Register event handler
	subscription.AddHandler(func(event *ensync.EventPayload) error {
		log.Printf("Received event: %s", event.EventName)
		log.Printf("Event Data: %+v", event.Payload)
		return nil
	})

	log.Println("Listening for events... Press Ctrl+C to exit")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	subscription.Unsubscribe()
}
