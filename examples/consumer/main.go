package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	ensync "github.com/ensync/golang-sdk"
)

func main() {
	ctx := context.Background()
	engine, err := ensync.NewEnSyncEngine(
		"url",
		ensync.WithAccessKey("key"),
		ensync.WithVersion("v1"),
		ensync.WithHTTP2(),
		ensync.WithTLSConfig(true),
		ensync.WithRenewAt(420000),
	)

	if err != nil {
		log.Fatalf("Failed to create EnSync engine: %v", err)
	}
	defer engine.Close()

	// Create a client with access key
	client, err := engine.CreateClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close(ctx)

	// Event name to subscribe to
	eventName := "eventName"

	// Subscribe to the event
	sub, err := client.Subscribe(ctx, eventName)
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// Create a channel to handle OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start pulling records in a goroutine
	go func() {
		err := sub.Pull(ctx, ensync.EnSyncSubscribeOptions{
			AutoAck: false,
		}, func(record ensync.EnSyncRecord) error {
			// Handle the received record
			fmt.Printf("Payment received successfully: %+v\n", record)

			// Acknowledge message read
			ack, err := client.Ack(ctx, record.ID, record.Block)
			if err != nil {
				return fmt.Errorf("failed to acknowledge record: %v", err)
			}
			fmt.Printf("Acknowledged record %s: %s\n", record.ID, ack)

			return nil
		})
		if err != nil {
			log.Printf("Pull error: %v", err)
		}
	}()

	// Wait for OS signal
	<-sigChan
	fmt.Println("\nShutting down gracefully...")

	// Unsubscribe before shutting down
	if err := sub.Unsubscribe(); err != nil {
		log.Printf("Failed to unsubscribe: %v", err)
	}
}
