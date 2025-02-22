package main

import (
	"context"
	"fmt"
	"log"
	"time"

	ensync "github.com/ensync/golang-sdk"
)

func main() {
	ctx := context.Background()
	// Create a new EnSync engine with TLS disabled for local development
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

	// Event name (should be created using ensync-cli)
	eventName := "eventName"

	// Simulate sending multiple events
	for i := 0; i < 5; i++ {
		start := time.Now()

		// Create the event payload
		payload := ensync.EnSyncEventPayload{
			Data: map[string]interface{}{
				"sentAt": 1 + i,
			},
		}

		// Publish the event
		err := client.Publish(ctx, eventName, payload, ensync.EnSyncPublishOptions{
			Persist: true,
		})
		if err != nil {
			log.Printf("Error publishing event: %v", err)
			continue
		}

		duration := time.Since(start)
		fmt.Printf("Event %d published successfully. Duration: %v\n", i, duration)
	}
}
