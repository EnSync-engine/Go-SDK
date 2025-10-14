# EnSync Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/EnSync-engine/Go-SDK.svg)](https://pkg.go.dev/github.com/EnSync-engine/Go-SDK)
[![Go Report Card](https://goreportcard.com/badge/github.com/EnSync-engine/Go-SDK)](https://goreportcard.com/report/github.com/EnSync-engine/Go-SDK)

Go client SDK for [EnSync Engine](https://ensync.cloud) - an event-delivery based integration engine that enables you to integrate with third-party apps as though they were native to your system and in real-time.

## Features

- ✅ **Dual Transport Support**: gRPC (high-performance) and WebSocket (real-time)
- ✅ **End-to-End Encryption**: Ed25519/Curve25519 encryption with hybrid encryption support
- ✅ **Event Management**: Publish, subscribe, acknowledge, defer, discard, and replay events
- ✅ **Flow Control**: Pause and resume event processing
- ✅ **Automatic Reconnection**: Built-in reconnection logic with configurable retry
- ✅ **Type-Safe**: Idiomatic Go interfaces and strong typing
- ✅ **Concurrent-Safe**: Thread-safe operations with proper synchronization
- ✅ **Context Support**: Full context.Context integration for cancellation

## Installation

```bash
go get github.com/EnSync-engine/Go-SDK
```

## Quick Start

### gRPC Client (Recommended for Server-to-Server)

```go
package main

import (
    "log"
    ensync "github.com/EnSync-engine/Go-SDK"
)

func main() {
    // Create gRPC engine
    engine, err := ensync.NewGRPCEngine("grpc://localhost:50051")
    if err != nil {
        log.Fatal(err)
    }
    defer engine.Close()

    // Authenticate
    err = engine.CreateClient("your-access-key")
    if err != nil {
        log.Fatal(err)
    }

    // Publish an event
    eventID, err := engine.Publish(
        "yourcompany/payment/created",
        []string{"recipient-public-key-base64"},
        map[string]interface{}{
            "amount": 100,
            "currency": "USD",
        },
        nil,
        nil,
    )
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Event published: %s", eventID)
}
```

### WebSocket Client

```go
package main

import (
    "log"
    ensync "github.com/EnSync-engine/Go-SDK"
)

func main() {
    // Create WebSocket engine
    engine, err := ensync.NewWebSocketEngine("ws://localhost:8082")
    if err != nil {
        log.Fatal(err)
    }
    defer engine.Close()

    // Authenticate
    err = engine.CreateClient("your-access-key")
    if err != nil {
        log.Fatal(err)
    }

    // Subscribe to events
    subscription, err := engine.Subscribe("yourcompany/payment/created", &ensync.SubscribeOptions{
        AutoAck: true,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Register event handler
    subscription.AddHandler(func(event *ensync.EventPayload) error {
        log.Printf("Received: %+v", event.Payload)
        return nil
    })

    // Keep running...
    select {}
}
```

## Transport Options

### gRPC (Default)

High-performance binary protocol with HTTP/2, ideal for server-to-server communication.

```go
// Insecure connection
engine, _ := ensync.NewGRPCEngine("grpc://localhost:50051")

// Secure connection (TLS)
engine, _ := ensync.NewGRPCEngine("grpcs://node.ensync.cloud:50051")
```

### WebSocket

Real-time bidirectional communication, great for browser and Node.js applications.

```go
// Insecure connection
engine, _ := ensync.NewWebSocketEngine("ws://localhost:8082")

// Secure connection (WSS)
engine, _ := ensync.NewWebSocketEngine("wss://node.ensync.cloud:8082")
```

## API Reference

### Creating a Client

```go
// Basic client creation
engine, err := ensync.NewGRPCEngine("grpc://localhost:50051")
err = engine.CreateClient("your-access-key")

// With options
err = engine.CreateClient("your-access-key",
    ensync.WithAppSecretKey("your-app-secret"),
    ensync.WithClientID("custom-client-id"),
)
```

### Publishing Events

```go
eventID, err := engine.Publish(
    eventName string,                      // Event name
    recipients []string,                   // Recipient public keys (base64)
    payload map[string]interface{},        // Event payload
    metadata *ensync.EventMetadata,        // Optional metadata
    options *ensync.PublishOptions,        // Optional publish options
)
```

**Example:**

```go
metadata := &ensync.EventMetadata{
    Persist: true,
    Headers: map[string]string{
        "source": "payment-service",
    },
}

options := &ensync.PublishOptions{
    UseHybridEncryption: true, // Use hybrid encryption for multiple recipients
}

eventID, err := engine.Publish(
    "yourcompany/payment/created",
    []string{"recipient1-key", "recipient2-key"},
    map[string]interface{}{
        "transactionId": "txn-123",
        "amount": 100.50,
        "currency": "USD",
    },
    metadata,
    options,
)
```

### Subscribing to Events

```go
subscription, err := engine.Subscribe(
    eventName string,
    options *ensync.SubscribeOptions,
)
```

**Subscribe Options:**

```go
options := &ensync.SubscribeOptions{
    AutoAck: true,                    // Auto-acknowledge events
    AppSecretKey: "your-secret-key",  // Override decryption key
}
```

### Event Handlers

```go
// Register a handler
unsubscribe := subscription.AddHandler(func(event *ensync.EventPayload) error {
    log.Printf("Event: %s", event.EventName)
    log.Printf("ID: %s", event.Idem)
    log.Printf("Block: %d", event.Block)
    log.Printf("Payload: %+v", event.Payload)
    log.Printf("Timestamp: %v", event.Timestamp)
    log.Printf("Sender: %s", event.Sender)
    
    // Process event...
    
    return nil
})

// Unregister handler
unsubscribe()
```

### Event Management

#### Acknowledge Event

```go
err := subscription.Ack(eventIdem string, block int64)
```

#### Defer Event

Postpone event processing for later delivery:

```go
response, err := subscription.Defer(
    eventIdem string,
    delayMs int64,      // Delay in milliseconds (1000ms to 24h)
    reason string,      // Optional reason
)
```

#### Discard Event

Permanently reject an event without processing:

```go
response, err := subscription.Discard(
    eventIdem string,
    reason string,      // Optional reason
)
```

#### Replay Event

Request a specific event to be sent again:

```go
event, err := subscription.Replay(eventIdem string)
```

### Flow Control

#### Pause Processing

```go
err := subscription.Pause("System maintenance")
```

#### Resume Processing

```go
err := subscription.Resume()
```

### Unsubscribe

```go
err := subscription.Unsubscribe()
```

### Close Connection

```go
err := engine.Close()
```

## Event Structure

```go
type EventPayload struct {
    EventName string                 // Event name
    Idem      string                 // Unique event ID
    Block     int64                  // Block ID for acknowledgment
    Timestamp time.Time              // Event timestamp
    Payload   map[string]interface{} // Event data
    Metadata  map[string]interface{} // Event metadata
    Sender    string                 // Sender client ID
}
```

## Error Handling

The SDK uses typed errors for better error handling:

```go
eventID, err := engine.Publish(...)
if err != nil {
    if ensyncErr, ok := err.(*ensync.EnSyncError); ok {
        switch ensyncErr.Type {
        case ensync.ErrTypeAuth:
            log.Println("Authentication error:", ensyncErr.Message)
        case ensync.ErrTypePublish:
            log.Println("Publish error:", ensyncErr.Message)
        case ensync.ErrTypeConnection:
            log.Println("Connection error:", ensyncErr.Message)
        default:
            log.Println("Error:", ensyncErr.Message)
        }
    }
}
```

**Error Types:**

- `ErrTypeGeneric` - Generic errors
- `ErrTypeAuth` - Authentication errors
- `ErrTypeConnection` - Connection errors
- `ErrTypePublish` - Publishing errors
- `ErrTypeSubscription` - Subscription errors
- `ErrTypeTimeout` - Timeout errors
- `ErrTypeReplay` - Replay errors
- `ErrTypeDefer` - Defer errors
- `ErrTypeDiscard` - Discard errors
- `ErrTypePause` - Pause errors
- `ErrTypeContinue` - Continue errors
- `ErrTypeValidation` - Validation errors

## Complete Examples

### Event Producer

```go
package main

import (
    "log"
    ensync "github.com/EnSync-engine/Go-SDK"
)

func main() {
    engine, err := ensync.NewGRPCEngine("grpc://localhost:50051")
    if err != nil {
        log.Fatal(err)
    }
    defer engine.Close()

    err = engine.CreateClient("your-access-key")
    if err != nil {
        log.Fatal(err)
    }

    eventName := "yourcompany/payment/POS/PAYMENT_SUCCESSFUL"
    recipients := []string{"recipient-public-key-base64"}
    
    payload := map[string]interface{}{
        "transactionId": "123",
        "amount":        100,
        "terminal":      "pos-1",
        "timestamp":     1234567890,
    }

    eventID, err := engine.Publish(eventName, recipients, payload, nil, nil)
    if err != nil {
        log.Fatalf("Publish failed: %v", err)
    }

    log.Printf("Event published: %s", eventID)
}
```

### Event Subscriber

```go
package main

import (
    "log"
    "os"
    "os/signal"
    "syscall"
    
    ensync "github.com/EnSync-engine/Go-SDK"
)

func main() {
    engine, err := ensync.NewGRPCEngine("grpc://localhost:50051")
    if err != nil {
        log.Fatal(err)
    }
    defer engine.Close()

    err = engine.CreateClient("your-access-key")
    if err != nil {
        log.Fatal(err)
    }

    eventName := "yourcompany/payment/POS/PAYMENT_SUCCESSFUL"
    subscription, err := engine.Subscribe(eventName, &ensync.SubscribeOptions{
        AutoAck: false, // Manual acknowledgment
    })
    if err != nil {
        log.Fatal(err)
    }

    subscription.AddHandler(func(event *ensync.EventPayload) error {
        log.Printf("Event ID: %s", event.Idem)
        log.Printf("Event Block: %d", event.Block)
        log.Printf("Event Data: %+v", event.Payload)
        
        // Process the event
        if err := processPayment(event.Payload); err != nil {
            log.Printf("Processing failed: %v", err)
            // Defer for retry
            subscription.Defer(event.Idem, 5000, "Processing failed")
            return err
        }
        
        // Acknowledge successful processing
        return subscription.Ack(event.Idem, event.Block)
    })

    log.Println("Listening for events... Press Ctrl+C to exit")

    // Graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan

    log.Println("Shutting down...")
    subscription.Unsubscribe()
}

func processPayment(payload map[string]interface{}) error {
    // Process payment logic
    return nil
}
```

## Configuration Options

### gRPC Engine Options

```go
engine, err := ensync.NewGRPCEngine("grpc://localhost:50051",
    ensync.WithHeartbeatInterval(30 * time.Second),
    ensync.WithMaxReconnectAttempts(5),
)
```

### WebSocket Engine Options

```go
engine, err := ensync.NewWebSocketEngine("ws://localhost:8082",
    ensync.WithPingInterval(30 * time.Second),
    ensync.WithReconnectInterval(5 * time.Second),
)
```

## Best Practices

### Connection Management

```go
// Use environment variables for credentials
import "os"

accessKey := os.Getenv("ENSYNC_ACCESS_KEY")
engine, _ := ensync.NewGRPCEngine(os.Getenv("ENSYNC_URL"))
engine.CreateClient(accessKey)

// Always close connections
defer engine.Close()
```

### Event Design

```go
// Use hierarchical event names
eventName := "domain/entity/action"

// Example: "inventory/product/created"
// Example: "payment/transaction/completed"
```

### Error Handling

```go
subscription.AddHandler(func(event *ensync.EventPayload) error {
    if err := processEvent(event); err != nil {
        log.Printf("Processing error: %v", err)
        
        // Defer for retry
        subscription.Defer(event.Idem, 5000, err.Error())
        return err
    }
    
    return subscription.Ack(event.Idem, event.Block)
})
```

### Graceful Shutdown

```go
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

go func() {
    <-sigChan
    log.Println("Shutting down...")
    subscription.Unsubscribe()
    engine.Close()
    os.Exit(0)
}()
```

## Generating gRPC Code

If you need to regenerate the gRPC code from the proto file:

```bash
# Install protoc plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate code
chmod +x generate.sh
./generate.sh
```

## Project Structure

```
Go-SDK/
├── crypto.go              # Encryption/decryption utilities
├── errors.go              # Error types and handling
├── types.go               # Core types and interfaces
├── grpc_client.go         # gRPC client implementation
├── websocket_client.go    # WebSocket client implementation
├── example_test.go        # Example usage tests
├── proto/
│   └── ensync.proto       # Protocol buffer definitions
├── examples/
│   ├── grpc_publisher/    # gRPC publisher example
│   ├── grpc_subscriber/   # gRPC subscriber example
│   └── websocket_example/ # WebSocket example
├── go.mod                 # Go module definition
└── README.md              # This file
```

## Design Decisions

### Interfaces Over Concrete Types

The SDK uses interfaces (`Engine`, `Subscription`) to allow easy mocking for testing and future extensibility.

### Goroutines and Channels

Event handling uses goroutines for concurrent processing, with proper synchronization using mutexes and channels.

### Context Support

All blocking operations support `context.Context` for cancellation and timeout control.

### Error Handling

Go-idiomatic error handling with typed errors instead of try-catch blocks.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

ISC License

## Links

- [EnSync Engine](https://ensync.cloud)
- [Documentation](https://docs.tryensync.com)

## Support

For issues and questions:
- GitHub Issues: [https://github.com/EnSync-engine/Go-SDK/issues](https://github.com/EnSync-engine/Go-SDK/issues)
- Documentation: [https://docs.tryensync.com](https://docs.tryensync.com)
