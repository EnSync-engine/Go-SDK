# Quick Start Guide

Get started with the EnSync Go SDK in 5 minutes.

## Installation

```bash
go get github.com/EnSync-engine/Go-SDK
```

## Basic Usage

### 1. Import the SDK

```go
package main

import (
    "log"
    ensync "github.com/EnSync-engine/Go-SDK"
)
```

### 2. Create an Engine

Choose between gRPC (recommended for servers) or WebSocket:

```go
// gRPC (recommended for server-to-server)
engine, err := ensync.NewGRPCEngine("grpc://localhost:50051")
if err != nil {
    log.Fatal(err)
}
defer engine.Close()

// OR WebSocket (for real-time apps)
engine, err := ensync.NewWebSocketEngine("ws://localhost:8082")
if err != nil {
    log.Fatal(err)
}
defer engine.Close()
```

### 3. Authenticate

```go
err = engine.CreateClient("your-access-key")
if err != nil {
    log.Fatal(err)
}
```

### 4. Publish Events

```go
eventID, err := engine.Publish(
    "yourcompany/payment/created",           // Event name
    []string{"recipient-public-key-base64"}, // Recipients
    map[string]interface{}{                  // Payload
        "amount": 100,
        "currency": "USD",
    },
    nil, // Metadata (optional)
    nil, // Options (optional)
)
if err != nil {
    log.Fatal(err)
}

log.Printf("Event published: %s", eventID)
```

### 5. Subscribe to Events

```go
subscription, err := engine.Subscribe(
    "yourcompany/payment/created",
    &ensync.SubscribeOptions{
        AutoAck: true, // Automatically acknowledge events
    },
)
if err != nil {
    log.Fatal(err)
}

// Register event handler
subscription.AddHandler(func(event *ensync.EventPayload) error {
    log.Printf("Received event: %s", event.EventName)
    log.Printf("Payload: %+v", event.Payload)
    
    // Process the event
    // ...
    
    return nil
})
```

## Complete Example

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
    // Create engine
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

    // Subscribe to events
    subscription, err := engine.Subscribe(
        "yourcompany/payment/created",
        &ensync.SubscribeOptions{AutoAck: true},
    )
    if err != nil {
        log.Fatal(err)
    }

    // Handle events
    subscription.AddHandler(func(event *ensync.EventPayload) error {
        log.Printf("Event: %+v", event.Payload)
        return nil
    })

    log.Println("Listening for events... Press Ctrl+C to exit")

    // Wait for interrupt
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan

    log.Println("Shutting down...")
    subscription.Unsubscribe()
}
```

## Next Steps

- Read the [full documentation](README.md)
- Check out [examples](examples/)
- Learn about [design decisions](DESIGN.md)
- See [API reference](README.md#api-reference)

## Common Patterns

### Publishing with Metadata

```go
metadata := &ensync.EventMetadata{
    Persist: true,
    Headers: map[string]string{
        "source": "payment-service",
        "version": "1.0",
    },
}

eventID, err := engine.Publish(
    "yourcompany/payment/created",
    []string{"recipient-key"},
    map[string]interface{}{"amount": 100},
    metadata,
    nil,
)
```

### Manual Acknowledgment

```go
subscription, err := engine.Subscribe(
    "yourcompany/order/created",
    &ensync.SubscribeOptions{
        AutoAck: false, // Manual acknowledgment
    },
)

subscription.AddHandler(func(event *ensync.EventPayload) error {
    // Process event
    if err := processOrder(event.Payload); err != nil {
        log.Printf("Processing failed: %v", err)
        return err
    }
    
    // Acknowledge after successful processing
    return subscription.Ack(event.Idem, event.Block)
})
```

### Deferring Events

```go
subscription.AddHandler(func(event *ensync.EventPayload) error {
    // Check if we can process now
    if !canProcessNow() {
        // Defer for 5 seconds
        _, err := subscription.Defer(event.Idem, 5000, "Resource busy")
        return err
    }
    
    // Process event
    return processEvent(event)
})
```

### Error Handling

```go
eventID, err := engine.Publish(...)
if err != nil {
    if ensyncErr, ok := err.(*ensync.EnSyncError); ok {
        switch ensyncErr.Type {
        case ensync.ErrTypeAuth:
            log.Println("Authentication failed")
        case ensync.ErrTypeConnection:
            log.Println("Connection error")
        default:
            log.Printf("Error: %v", ensyncErr)
        }
    }
}
```

## Configuration

### gRPC Options

```go
engine, err := ensync.NewGRPCEngine(
    "grpc://localhost:50051",
    ensync.WithHeartbeatInterval(30 * time.Second),
    ensync.WithMaxReconnectAttempts(5),
)
```

### WebSocket Options

```go
engine, err := ensync.NewWebSocketEngine(
    "ws://localhost:8082",
    ensync.WithPingInterval(30 * time.Second),
    ensync.WithReconnectInterval(5 * time.Second),
)
```

### Client Options

```go
err = engine.CreateClient(
    "your-access-key",
    ensync.WithAppSecretKey("your-secret-key"),
    ensync.WithClientID("custom-client-id"),
)
```

## Troubleshooting

### Connection Issues

```go
// Use secure connection for production
engine, err := ensync.NewGRPCEngine("grpcs://node.ensync.cloud:50051")

// Or for WebSocket
engine, err := ensync.NewWebSocketEngine("wss://node.ensync.cloud:8082")
```

### Authentication Errors

```go
err = engine.CreateClient("your-access-key")
if err != nil {
    if ensyncErr, ok := err.(*ensync.EnSyncError); ok {
        if ensyncErr.Type == ensync.ErrTypeAuth {
            log.Println("Check your access key")
        }
    }
}
```

### Event Not Received

```go
// Make sure to keep the program running
subscription.AddHandler(func(event *ensync.EventPayload) error {
    log.Printf("Event: %+v", event)
    return nil
})

// Keep running
select {} // Block forever

// Or use signal handling for graceful shutdown
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt)
<-sigChan
```

## Support

- [GitHub Issues](https://github.com/EnSync-engine/Go-SDK/issues)
- [Documentation](https://docs.tryensync.com)
- [EnSync Cloud](https://ensync.cloud)

Happy coding! 🚀
