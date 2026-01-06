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
    "context"
    "log"
    
    ensync "github.com/EnSync-engine/Go-SDK"
)
```

### 2. Create an Engine

Choose your transport protocol:

```go
ctx := context.Background()

// gRPC (recommended for server-to-server)
engine, err := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")
if err != nil {
    log.Fatal(err)
}
defer engine.Close()

// WebSocket (recommended for client applications)  
engine, err := ensync.NewWebSocketEngine(ctx, "ws://localhost:8080")
if err != nil {
    log.Fatal(err)
}
defer engine.Close()

// Auto-detect protocol from URL
engine, err := ensync.NewEngine(ctx, "grpc://localhost:50051")
if err != nil {
    log.Fatal(err)
}
defer engine.Close()
```

**Note**: The engine constructor establishes the transport connection (WebSocket/gRPC).

### 3. Authenticate Client

```go
// Authenticate with EnSync protocol over the established connection
err = engine.CreateClient("access-key")
if err != nil {
    log.Fatal(err)
}
```

**Note**: `CreateClient` handles EnSync protocol authentication, not connection establishment.

### 4. Publish Events

```go
eventName := "company/payment/created"
recipients := []string{"recipient-public-key-base64"}
payload := map[string]interface{}{
    "amount":   100,
    "currency": "USD",
}

metadata := &ensync.MessageMetadata{
    Persist: true,
    Headers: map[string]string{
        "source": "payment-service",
    },
}

if err != nil {
    log.Fatal(err)
}

log.Printf("Published event: %s", eventID)
```

### 5. Subscribe to Events

```go
subscription, err := engine.Subscribe("company/payment/created", &ensync.SubscribeOptions{
    AutoAck: true,
})
if err != nil {
    log.Fatal(err)
}
defer subscription.Unsubscribe()

// Register event handler
removeHandler := subscription.AddHandler(func(event *ensync.MessagePayload) error {
    log.Printf("Received: %s", event.EventName)
    log.Printf("Data: %+v", event.Payload)
    return nil
})

// Remove handler when needed
defer removeHandler()
```

## Complete Example

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    ensync "github.com/EnSync-engine/Go-SDK"
)

func main() {
    ctx := context.Background()
    
    // Create engine - establishes WebSocket connection
    engine, err := ensync.NewWebSocketEngine(ctx, "ws://localhost:8080")
    if err != nil {
        log.Fatalf("Failed to create engine: %v", err)
    }
    defer engine.Close()

    // Authenticate with EnSync protocol
    err = engine.CreateClient("your-access-key")
    if err != nil {
        log.Fatalf("Failed to authenticate: %v", err)
    }

    // Subscribe to events
    subscription, err := engine.Subscribe("test/events", &ensync.SubscribeOptions{
        AutoAck: true,
    })
    if err != nil {
        log.Fatalf("Failed to subscribe: %v", err)
    }
    defer subscription.Unsubscribe()

    // Handle events
    subscription.AddHandler(func(event *ensync.MessagePayload) error {
        log.Printf("Event: %s, Data: %+v", event.EventName, event.Payload)
        return nil
    })

    // Publish an event
    go func() {
        time.Sleep(2 * time.Second)
        eventID, err := engine.Publish(
            "test/events",
            []string{"recipient-key"},
            map[string]interface{}{"message": "Hello, World!"},
            nil,
            nil,
        )
        if err != nil {
            log.Printf("Publish error: %v", err)
        } else {
            log.Printf("Published event: %s", eventID)
        }
    }()

    // Wait for interrupt
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan
    
    log.Println("Shutting down...")
}
```

## Key Concepts

### Connection Flow
1. **Engine Creation** → Establishes transport connection (WebSocket/gRPC)
2. **CreateClient** → Authenticates with EnSync protocol  
3. **Publish/Subscribe** → Ready for event operations

### Transport Protocols
- **gRPC**: Best for server-to-server communication
- **WebSocket**: Best for client applications and real-time updates
- **Auto-detect**: Use `NewEngine()` with URL scheme detection

### Error Handling
```go
eventID, err := engine.Publish(...)
if err != nil {
    if ensyncErr, ok := err.(*ensync.EnSyncError); ok {
        switch ensyncErr.Type {
        case ensync.ErrTypeAuth:
            log.Println("Authentication error")
        case ensync.ErrTypeConnection:
            log.Println("Connection error")
        }
    }
}
```

### Event Lifecycle
```go
subscription, _ := engine.Subscribe("events", &ensync.SubscribeOptions{
    AutoAck: false, // Manual acknowledgment
})

subscription.AddHandler(func(event *ensync.EventPayload) error {
    // Process event
    if processSuccessfully(event) {
        return subscription.Ack(event.Idem, event.Block)
    }
    
    // Defer for later processing
    _, err := subscription.Defer(event.Idem, 30000, "retry later")
    return err
})
```

## Next Steps

- Read the complete documentation
- Explore example applications
- Check the design decisions
- Learn about contributing

## Need Help?

- Check the troubleshooting guide
- Review common patterns
- Open an issue on GitHub

Happy coding! 🚀
    ensync.WithClientID("custom-client-id"),
)
```

## Configuration Options

### Basic Configuration

```go
import (
    "context"
    "time"
    
    "github.com/EnSync-engine/Go-SDK/common"
    ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

ctx := context.Background()
engine, err := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051",
    common.WithMaxReconnectAttempts(5),
    common.WithReconnectDelay(3 * time.Second),
    common.WithCircuitBreaker(5, 30 * time.Second),
)

// Client options
err = engine.CreateClient("your-access-key",
    common.WithAppSecretKey("your-secret"),
    common.WithClientID("my-service"),
)
```

### Available Options

**Common Options (both gRPC and WebSocket):**
- `common.WithMaxReconnectAttempts(int)` - Max reconnection attempts
- `common.WithReconnectDelay(time.Duration)` - Delay between reconnects
- `common.WithAutoReconnect(bool)` - Enable/disable auto-reconnect
- `common.WithCircuitBreaker(threshold, resetTimeout)` - Circuit breaker config
- `common.WithRetryConfig(attempts, initial, max, jitter)` - Retry logic
- `common.WithLogger(logger)` - Custom logger

**Client Options:**
- `common.WithAppSecretKey(string)` - App secret for decryption
- `common.WithClientID(string)` - Custom client identifier

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
