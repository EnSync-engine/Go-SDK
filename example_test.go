package ensync_test

import (
	"context"
	"fmt"
	"log"

	ensync "github.com/EnSync-engine/Go-SDK"
)

// Example demonstrates basic usage of the gRPC client
func Example_grpcClient() {
	ctx := context.Background()
	
	// Create a new gRPC engine
	engine, err := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()

	// Create and authenticate client
	err = engine.CreateClient("your-access-key")
	if err != nil {
		log.Fatal(err)
	}

	// Publish an event
	eventName := "yourcompany/payment/created"
	recipients := []string{"recipient-public-key-base64"}
	payload := map[string]interface{}{
		"amount": 100,
		"currency": "USD",
	}

	eventID, err := engine.Publish(eventName, recipients, payload, nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Published event: %s\n", eventID)
}

// Example demonstrates subscribing to events with gRPC
func Example_grpcSubscriber() {
	ctx := context.Background()
	engine, err := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()

	err = engine.CreateClient("your-access-key")
	if err != nil {
		log.Fatal(err)
	}

	// Subscribe to event
	subscription, err := engine.Subscribe("yourcompany/payment/created", &ensync.SubscribeOptions{
		AutoAck: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Unsubscribe()

	// Register event handler
	subscription.AddHandler(func(event *ensync.EventPayload) error {
		fmt.Printf("Received event: %s\n", event.EventName)
		fmt.Printf("Payload: %+v\n", event.Payload)
		return nil
	})
}

// Example demonstrates using WebSocket client
func Example_webSocketClient() {
	ctx := context.Background()
	engine, err := ensync.NewWebSocketEngine(ctx, "ws://localhost:8082")
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()

	err = engine.CreateClient("your-access-key")
	if err != nil {
		log.Fatal(err)
	}

	// Publish an event
	eventName := "yourcompany/notification/sent"
	recipients := []string{"recipient-public-key-base64"}
	payload := map[string]interface{}{
		"message": "Hello, World!",
		"priority": "high",
	}

	eventID, err := engine.Publish(eventName, recipients, payload, nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Published event: %s\n", eventID)
}

// Example demonstrates advanced subscription features
func Example_advancedSubscription() {
	ctx := context.Background()
	engine, err := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()

	err = engine.CreateClient("your-access-key")
	if err != nil {
		log.Fatal(err)
	}

	subscription, err := engine.Subscribe("yourcompany/order/created", &ensync.SubscribeOptions{
		AutoAck: false, // Manual acknowledgment
	})
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Unsubscribe()

	subscription.AddHandler(func(event *ensync.EventPayload) error {
		fmt.Printf("Processing order: %+v\n", event.Payload)

		// Process the event
		// If processing succeeds, acknowledge
		if err := subscription.Ack(event.Idem, event.Block); err != nil {
			log.Printf("Failed to acknowledge: %v", err)
			return err
		}

		return nil
	})
}

// Example demonstrates event deferral
func Example_deferEvent() {
	ctx := context.Background()
	engine, err := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()

	err = engine.CreateClient("your-access-key")
	if err != nil {
		log.Fatal(err)
	}

	subscription, err := engine.Subscribe("yourcompany/task/pending", &ensync.SubscribeOptions{
		AutoAck: false,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Unsubscribe()

	subscription.AddHandler(func(event *ensync.EventPayload) error {
		// Check if we can process now
		canProcess := checkResourceAvailability()
		
		if !canProcess {
			// Defer for 5 seconds
			resp, err := subscription.Defer(event.Idem, 5000, "Resource unavailable")
			if err != nil {
				return err
			}
			fmt.Printf("Event deferred until: %v\n", resp.ScheduledDelivery)
			return nil
		}

		// Process the event
		processEvent(event)
		return subscription.Ack(event.Idem, event.Block)
	})
}

// Example demonstrates pause and resume
func Example_pauseResume() {
	ctx := context.Background()
	engine, err := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()

	err = engine.CreateClient("your-access-key")
	if err != nil {
		log.Fatal(err)
	}

	subscription, err := engine.Subscribe("yourcompany/data/import", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Unsubscribe()

	// Pause processing during maintenance
	if err := subscription.Pause("System maintenance"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Event processing paused")

	// Perform maintenance
	// ...

	// Resume processing
	if err := subscription.Resume(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Event processing resumed")
}

// Helper functions for examples
func checkResourceAvailability() bool {
	return true
}

func processEvent(event *ensync.EventPayload) {
	// Process event logic
}
