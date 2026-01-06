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
    ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

func main() {
    // Create gRPC engine
    engine, err := ensync.NewGRPCEngine("grpc://localhost:50051")
    if err != nil {
        log.Fatal(err)
    }
    defer engine.Close()

    // Authenticate
    err = engine.CreateClient("access-key")
    if err != nil {
        log.Fatal(err)
    }

    // Publish an event
    eventID, err := engine.Publish(
        "company/payment/created",
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
    ensync "github.com/EnSync-engine/Go-SDK/websocket"
)

func main() {
    // Create WebSocket engine
    engine, err := ensync.NewWebSocketEngine("ws://localhost:8082")
    if err != nil {
        log.Fatal(err)
    }
    defer engine.Close()

    // Authenticate
    err = engine.CreateClient("access-key")
    if err != nil {
        log.Fatal(err)
    }

    // Subscribe to events
    subscription, err := engine.Subscribe("company/payment/created", &ensync.SubscribeOptions{
        AutoAck: true,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Register event handler
    subscription.AddHandler(func(event *ensync.MessagePayload) error {
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
    metadata *ensync.MessageMetadata,        // Optional metadata
    options *ensync.PublishOptions,        // Optional publish options
)
```

**Example:**

```go
metadata := &ensync.MessageMetadata{
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
unsubscribe := subscription.AddHandler(func(event *ensync.MessagePayload) error {
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
type MessagePayload struct {
    EventName string                 // Event name
    Idem      string                 // Unique event ID
    Block     int64                  // Block ID for acknowledgment
    Timestamp time.Time              // Event timestamp
    Payload   map[string]interface{} // Event data
    Metadata  ensync.MessageMetadata // Event metadata
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
    ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

func main() {
    engine, err := ensync.NewGRPCEngine("grpc://localhost:50051")
    if err != nil {
        log.Fatal(err)
    }
    defer engine.Close()

    err = engine.CreateClient("access-key")
    if err != nil {
        log.Fatal(err)
    }

    eventName := "company/payment/POS/PAYMENT_SUCCESSFUL"
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
    
    ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

func main() {
    engine, err := ensync.NewGRPCEngine("grpc://localhost:50051")
    if err != nil {
        log.Fatal(err)
    }
    defer engine.Close()

    err = engine.CreateClient("access-key")
    if err != nil {
        log.Fatal(err)
    }

    eventName := "company/payment/POS/PAYMENT_SUCCESSFUL"
    subscription, err := engine.Subscribe(eventName, &ensync.SubscribeOptions{
        AutoAck: false, // Manual acknowledgment
    })
    if err != nil {
        log.Fatal(err)
    }

    subscription.AddHandler(func(event *ensync.MessagePayload) error {
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

### Common Configuration Options

Both gRPC and WebSocket engines support common configuration options:

```go
import (
    "context"
    "time"
    
    "github.com/EnSync-engine/Go-SDK/common"
    ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

ctx := context.Background()
engine, err := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051",
    // Circuit breaker: 5 failures, 10s base reset, 60s max reset
    common.WithCircuitBreaker(5, 10*time.Second, 60*time.Second),
    
    // Retry: 3 attempts, 1s initial backoff, 10s max backoff, 10% jitter
    common.WithRetryConfig(3, time.Second, 10*time.Second, 0.1),
    
    // Custom timeouts
    common.WithTimeoutOptions(
        common.WithOperationTimeout(15*time.Second),
        common.WithGracefulShutdownTimeout(5*time.Second),
    ),

    // Custom logger
    common.WithLogger(customLogger),
)
```

### WebSocket-Specific Options

WebSocket engines have additional configuration options:

```go
import (
    "context"
    "time"
    
    "github.com/EnSync-engine/Go-SDK/common"
    ensync "github.com/EnSync-engine/Go-SDK/websocket"
)

ctx := context.Background()
engine, err := ensync.NewWebSocketEngine(ctx, "ws://localhost:8082",
    // WebSocket-specific options would go here
    // (currently using embedded common config)
)
```

### Client Authentication Options

When creating a client, you can pass additional options:

```go
err = engine.CreateClient("access-key",
    common.WithAppSecretKey("app-secret-key"),
    common.WithClientID("custom-client-id"),
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
subscription.AddMessageHandler(func(event *ensync.MessagePayload) error {
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
├── common/                # Shared utilities and types
│   ├── base_engine.go     # Base engine functionality
│   ├── circuit_breaker.go # Circuit breaker implementation
│   ├── crypto.go          # Encryption/decryption utilities
│   ├── errors.go          # Error types and handling
│   ├── logger.go          # Logging utilities
│   ├── options.go         # Configuration options
│   ├── retry.go           # Retry logic
│   ├── subscription.go    # Subscription management
│   └── types.go           # Core types and interfaces
├── grpc/                  # gRPC transport implementation
│   ├── engine.go          # gRPC engine
│   └── options.go         # gRPC-specific options
├── websocket/             # WebSocket transport implementation
│   ├── engine.go          # WebSocket engine
│   └── options.go         # WebSocket-specific options
├── proto/                 # Protocol buffer definitions
│   ├── ensync.proto       # Protocol definitions
│   ├── ensync.pb.go       # Generated Go code
│   └── ensync_grpc.pb.go  # Generated gRPC code
├── examples/              # Example applications
│   ├── grpc_publisher/    # gRPC publisher example
│   ├── grpc_subscriber/   # gRPC subscriber example
│   └── websocket_example/ # WebSocket example
├── example_test.go        # Example usage tests
├── go.mod                 # Go module definition
├── go.sum                 # Go checksum file
├── Makefile               # Build automation
├── generate.sh            # gRPC code generation
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

### Development Workflow

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/new-feature`
3. Make your changes and add tests
4. Run tests: `make test`
5. Submit a pull request

### Releases

We use automated semantic versioning:

```bash
# Bump version and update changelog
make version-patch   # Bug fixes
make version-minor   # New features  
make version-major   # Breaking changes

# Commit and push to trigger automated release
git add . && git commit -m "chore: bump version to v1.2.3"
git push origin main
```

See [VERSIONING.md](VERSIONING.md) for detailed release instructions.

## License

ISC License

## Links

- [EnSync Engine](https://ensync.cloud)
- [Documentation](https://docs.tryensync.com)

## Support

For issues and questions:
- GitHub Issues: [https://github.com/EnSync-engine/Go-SDK/issues](https://github.com/EnSync-engine/Go-SDK/issues)
- Documentation: [https://docs.tryensync.com](https://docs.tryensync.com)
