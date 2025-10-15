# Getting Started with EnSync Go SDK

Welcome! This guide will help you get up and running with the EnSync Go SDK in minutes.

## 📋 Prerequisites

- **Go 1.21 or later** - [Download Go](https://golang.org/dl/)
- **Access to EnSync server** - Local or cloud instance
- **Access key** - Obtain from your EnSync dashboard

## 🚀 Installation

### Step 1: Install the SDK

```bash
go get github.com/EnSync-engine/Go-SDK
```

### Step 2: Initialize Your Project

```bash
mkdir my-ensync-app
cd my-ensync-app
go mod init my-ensync-app
```

### Step 3: Create Your First Program

Create a file named `main.go`:

```go
package main

import (
    "context"
    "log"
    
    ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

func main() {
    ctx := context.Background()
    
    // Create engine - establishes gRPC connection
    engine, err := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")
    if err != nil {
        log.Fatal(err)
    }
    defer engine.Close()

    // Authenticate with EnSync protocol
    err = engine.CreateClient("your-access-key")
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Connected to EnSync!")
}
```

### Step 4: Run Your Program

```bash
go run main.go
```

## 🎯 Your First Event

### Publishing an Event

```go
package main

import (
    "context"
    "log"
    
    "github.com/EnSync-engine/Go-SDK/common"
    ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

func main() {
    ctx := context.Background()
    
    // Create engine - establishes gRPC connection
    engine, _ := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")
    defer engine.Close()
    
    // Authenticate with EnSync protocol
    engine.CreateClient("your-access-key")
    
    // Publish an event
    eventID, err := engine.Publish(
        "myapp/user/created",                    // Event name
        []string{"recipient-public-key-base64"}, // Recipients
        map[string]interface{}{                  // Payload
            "userId": "12345",
            "email": "user@example.com",
            "name": "John Doe",
        },
        nil, // Metadata (optional)
        nil, // Options (optional)
    )
    
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Event published with ID: %s", eventID)
}
```

### Subscribing to Events

```go
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
    
    // Create engine - establishes gRPC connection
    engine, _ := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")
    defer engine.Close()
    
    // Authenticate with EnSync protocol
    engine.CreateClient("your-access-key")
    
    // Subscribe to events
    subscription, err := engine.Subscribe(
        "myapp/user/created",
        &common.SubscribeOptions{AutoAck: true},
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // Handle events
    subscription.AddHandler(func(event *common.EventPayload) error {
        log.Printf("New user created!")
        log.Printf("User ID: %v", event.Payload["userId"])
        log.Printf("Email: %v", event.Payload["email"])
        log.Printf("Name: %v", event.Payload["name"])
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

## 🔐 Working with Encryption

EnSync uses end-to-end encryption. Here's how to get recipient public keys:

```go
// Get your client's public key
publicKey := engine.GetClientPublicKey()
log.Printf("My public key: %s", publicKey)

// Share this key with others who want to send you events
// Use their public keys when publishing events to them
```

## 🌐 Choosing a Transport

### gRPC (Recommended for Servers)

```go
import (
    "context"
    ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

ctx := context.Background()

// Insecure (development)
engine, _ := ensync.NewGRPCEngine(ctx, "grpc://localhost:50051")

// Secure (production)
engine, _ := ensync.NewGRPCEngine(ctx, "grpcs://node.ensync.cloud:50051")
```

**Best for:**
- Server-to-server communication
- High-performance requirements
- Binary protocol efficiency

### WebSocket (Great for Real-time)

```go
import (
    "context"
    ensync "github.com/EnSync-engine/Go-SDK/websocket"
)

ctx := context.Background()

// Insecure (development)
engine, _ := ensync.NewWebSocketEngine(ctx, "ws://localhost:8082")

// Secure (production)
engine, _ := ensync.NewWebSocketEngine(ctx, "wss://node.ensync.cloud:8082")
```

**Best for:**
- Real-time applications
- Browser compatibility
- Bidirectional streaming

## 📝 Common Patterns

### Pattern 1: Request-Response

```go
// Publisher
eventID, _ := engine.Publish(
    "myapp/request/data",
    []string{recipientKey},
    map[string]interface{}{"requestId": "123"},
    nil, nil,
)

// Subscriber
subscription.AddHandler(func(event *common.EventPayload) error {
    requestId := event.Payload["requestId"]
    
    // Process request and send response
    engine.Publish(
        "myapp/response/data",
        []string{event.Sender},
        map[string]interface{}{
            "requestId": requestId,
            "data": "response data",
        },
        nil, nil,
    )
    
    return nil
})
```

### Pattern 2: Fan-out

```go
// Send to multiple recipients
recipients := []string{
    "recipient1-key",
    "recipient2-key",
    "recipient3-key",
}

eventID, _ := engine.Publish(
    "myapp/notification/broadcast",
    recipients,
    map[string]interface{}{"message": "Hello everyone!"},
    nil,
    &common.PublishOptions{UseHybridEncryption: true}, // Efficient for multiple recipients
)
```

### Pattern 3: Event Processing with Retry

```go
subscription.AddHandler(func(event *common.EventPayload) error {
    // Try to process
    if err := processEvent(event); err != nil {
        // Defer for retry in 5 seconds
        subscription.Defer(event.Idem, 5000, "Processing failed, will retry")
        return err
    }
    
    // Success - acknowledge
    return subscription.Ack(event.Idem, event.Block)
})
```

### Pattern 4: Circuit Breaker

```go
var failureCount int
var maxFailures = 5

subscription.AddHandler(func(event *common.EventPayload) error {
    if failureCount >= maxFailures {
        // Pause processing
        subscription.Pause("Too many failures")
        log.Println("Circuit breaker opened - pausing events")
        
        // Schedule resume after cooldown
        go func() {
            time.Sleep(1 * time.Minute)
            subscription.Resume()
            failureCount = 0
            log.Println("Circuit breaker closed - resuming events")
        }()
        
        return nil
    }
    
    if err := processEvent(event); err != nil {
        failureCount++
        return err
    }
    
    failureCount = 0 // Reset on success
    return nil
})
```

## 🛠️ Configuration

### Environment Variables

```bash
export ENSYNC_URL="grpc://localhost:50051"
export ENSYNC_ACCESS_KEY="your-access-key"
export ENSYNC_APP_SECRET="your-app-secret"
```

```go
import (
    "context"
    "os"
    
    "github.com/EnSync-engine/Go-SDK/common"
    ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

ctx := context.Background()
engine, _ := ensync.NewGRPCEngine(ctx, os.Getenv("ENSYNC_URL"))
engine.CreateClient(
    os.Getenv("ENSYNC_ACCESS_KEY"),
    common.WithAppSecretKey(os.Getenv("ENSYNC_APP_SECRET")),
)
```

### Custom Configuration

#### Basic Configuration

```go
import (
    "context"
    "time"
    
    "github.com/EnSync-engine/Go-SDK/common"
    ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

ctx := context.Background()
engine, _ := ensync.NewGRPCEngine(
    ctx,
    "grpc://localhost:50051",
    common.WithMaxReconnectAttempts(10),
    common.WithReconnectDelay(3 * time.Second),
)
```

#### Advanced Configuration

```go
import (
    "context"
    "time"
    
    "github.com/EnSync-engine/Go-SDK/common"
    ensync "github.com/EnSync-engine/Go-SDK/grpc"
)

ctx := context.Background()
engine, _ := ensync.NewGRPCEngine(
    ctx,
    "grpc://localhost:50051",
    // Reconnection settings
    common.WithMaxReconnectAttempts(10),
    common.WithReconnectDelay(2 * time.Second),
    common.WithAutoReconnect(true),
    
    // Circuit breaker - pause after 5 failures for 30 seconds
    common.WithCircuitBreaker(5, 30 * time.Second),
    
    // Retry logic - 3 attempts with exponential backoff
    common.WithRetryConfig(3, time.Second, 10 * time.Second, 0.1),
    
    // Custom logger (implement common.Logger interface)
    // common.WithLogger(myCustomLogger),
)

// Client authentication with options
engine.CreateClient(
    "your-access-key",
    common.WithAppSecretKey("your-app-secret"),
    common.WithClientID("my-service-v1"),
)
```

#### WebSocket Configuration

```go
import (
    "context"
    "time"
    
    "github.com/EnSync-engine/Go-SDK/common"
    ensync "github.com/EnSync-engine/Go-SDK/websocket"
)

ctx := context.Background()
engine, _ := ensync.NewWebSocketEngine(
    ctx,
    "ws://localhost:8082",
    // Same common options as gRPC
    common.WithMaxReconnectAttempts(5),
    common.WithReconnectDelay(2 * time.Second),
    common.WithCircuitBreaker(3, 20 * time.Second),
)
```

## 🐛 Debugging

### Enable Logging

```go
import "log"

// Set log flags for detailed output
log.SetFlags(log.LstdFlags | log.Lshortfile)

// The SDK logs important events automatically
```

### Check Connection Status

```go
err := engine.CreateClient(accessKey)
if err != nil {
    if ensyncErr, ok := err.(*common.EnSyncError); ok {
        log.Printf("Error type: %s", ensyncErr.Type)
        log.Printf("Error message: %s", ensyncErr.Message)
    }
}
```

### Test Event Publishing

```go
eventID, err := engine.Publish(
    "test/event",
    []string{engine.GetClientPublicKey()}, // Send to yourself
    map[string]interface{}{"test": "data"},
    nil, nil,
)

if err != nil {
    log.Printf("Publish failed: %v", err)
} else {
    log.Printf("Published: %s", eventID)
}
```

## 📚 Next Steps

1. **Read the full documentation**: [README.md](README.md)
2. **Explore examples**: Check the `examples/` directory
3. **Learn design patterns**: Read [DESIGN.md](DESIGN.md)
4. **Build something**: Start with the [QUICKSTART.md](QUICKSTART.md)

## 🆘 Troubleshooting

### "Connection refused"

- Ensure EnSync server is running
- Check the URL and port
- Verify firewall settings

### "Authentication failed"

- Verify your access key is correct
- Check if the key has expired
- Ensure you're connecting to the right server

### "No events received"

- Verify subscription is active
- Check event name matches exactly
- Ensure your program keeps running (use signal handling)
- Verify you have the correct decryption key

### "Encryption failed"

- Verify recipient public key is valid base64
- Ensure key is 32 bytes when decoded
- Check if using the correct key format

## 💡 Tips

1. **Always use `defer engine.Close()`** to ensure cleanup
2. **Handle errors explicitly** - don't ignore them
3. **Use environment variables** for credentials
4. **Test with self-publishing** first (send to your own public key)
5. **Start with AutoAck: true** then move to manual acknowledgment
6. **Use hybrid encryption** for multiple recipients

## 🎓 Learning Resources

- **Quick Start**: [QUICKSTART.md](QUICKSTART.md)
- **Full API Reference**: [README.md](README.md)
- **Design Patterns**: [DESIGN.md](DESIGN.md)
- **Examples**: `examples/` directory
- **Contributing**: [CONTRIBUTING.md](CONTRIBUTING.md)

## 📞 Getting Help

- **GitHub Issues**: Report bugs or request features
- **Documentation**: https://docs.tryensync.com
- **EnSync Cloud**: https://ensync.cloud

---

**Ready to build?** Start with the [QUICKSTART.md](QUICKSTART.md) guide! 🚀
