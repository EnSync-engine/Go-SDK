# Go SDK Design Documentation

## Overview

This document explains the design decisions and Go-specific patterns used in EnSync SDK.

## Key Design Principles

### 1. Idiomatic Go Code

The SDK follows Go best practices and conventions:

- **Exported vs Unexported**: Public API uses capitalized names, internal implementation uses lowercase
- **Error Handling**: Returns `error` as the last return value instead of throwing exceptions
- **Interfaces**: Defines behavior contracts (`Engine`, `Subscription`) for flexibility and testability
- **Package Structure**: Single package `ensync` with clear separation of concerns

### 2. Concurrency Model

#### Go Approach
```go
// Goroutines and channels
go subscription.listen()

// Synchronous functions with error returns
eventID, err := engine.Publish(...)
if err != nil {
    return err
}

// Mutex-protected shared state
type GRPCEngine struct {
    state struct {
        mu sync.RWMutex
        isAuthenticated bool
    }
}
```

**Rationale**: Go's goroutines and channels provide better concurrency primitives than callbacks. Explicit error handling is more robust than promise rejection.

### 3. Type Safety

#### Go Approach
```go
// Strong typing with structs and interfaces
func (e *GRPCEngine) Publish(
    eventName string,
    recipients []string,
    payload map[string]interface{},
    metadata *EventMetadata,
    options *PublishOptions,
) (string, error)

type EventPayload struct {
    EventName string
    Idem      string
    Block     int64
    Timestamp time.Time
    Payload   map[string]interface{}
    Metadata  map[string]interface{}
    Sender    string
}
```

**Rationale**: Go's type system catches errors at compile time. Structs provide clear data contracts.

### 4. Interface-Based Design

```go
// Engine interface allows multiple implementations
type Engine interface {
    CreateClient(accessKey string, options ...ClientOption) error
    Publish(...) (string, error)
    Subscribe(...) (Subscription, error)
    Close() error
    GetClientPublicKey() string
    AnalyzePayload(payload map[string]interface{}) *PayloadMetadata
}

// Subscription interface for event handling
type Subscription interface {
    On(handler EventHandler) func()
    Ack(eventIdem string, block int64) error
    Resume() error
    Pause(reason string) error
    Defer(...) (*DeferResponse, error)
    Discard(...) (*DiscardResponse, error)
    Rollback(eventIdem string, block int64) error
    Replay(eventIdem string) (*EventPayload, error)
    Unsubscribe() error
}
```

**Benefits**:
- Easy to mock for testing
- Multiple implementations (gRPC, WebSocket)
- Clear API contracts
- Future extensibility

### 5. Functional Options Pattern

####
```go
engine, err := ensync.NewGRPCEngine("grpc://localhost:50051",
    ensync.WithHeartbeatInterval(30 * time.Second),
    ensync.WithMaxReconnectAttempts(5),
)

// Option functions
type GRPCOption func(*GRPCEngine)

func WithHeartbeatInterval(interval time.Duration) GRPCOption {
    return func(e *GRPCEngine) {
        e.config.heartbeatInterval = interval
    }
}
```

**Rationale**: Functional options provide:
- Backward compatibility when adding new options
- Clear, self-documenting API
- Type-safe configuration
- Optional parameters without overloading

### 6. Error Handling

####
```go
type EnSyncError struct {
    Message string
    Type    string
    Err     error
}

func (e *EnSyncError) Error() string {
    return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

func (e *EnSyncError) Unwrap() error {
    return e.Err
}

// Usage
eventID, err := engine.Publish(...)
if err != nil {
    if ensyncErr, ok := err.(*ensync.EnSyncError); ok {
        switch ensyncErr.Type {
        case ensync.ErrTypeAuth:
            // Handle auth error
        }
    }
}
```

#### 
```go
type EnSyncError struct {
    Message string
    Type    string
    Err     error
}

func (e *EnSyncError) Error() string {
    return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

func (e *EnSyncError) Unwrap() error {
    return e.Err
}

// Usage
eventID, err := engine.Publish(...)
if err != nil {
    if ensyncErr, ok := err.(*ensync.EnSyncError); ok {
        switch ensyncErr.Type {
        case ensync.ErrTypeAuth:
            // Handle auth error
        }
    }
}
```

**Rationale**: 
- Go's error interface is idiomatic
- Type assertions for specific error handling
- `Unwrap()` supports error chains (Go 1.13+)
- No stack traces needed (use logging)

### 7. Resource Management

#### 
```go
// Explicit cleanup with defer
engine, err := ensync.NewGRPCEngine(...)
if err != nil {
    return err
}
defer engine.Close()

// Context-based cancellation
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Graceful shutdown
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
<-sigChan
```

**Rationale**:
- `defer` ensures cleanup happens
- Context provides cancellation propagation
- Explicit resource management is clearer

### 8. Concurrency Safety

All shared state is protected with mutexes:

```go
type GRPCEngine struct {
    state struct {
        isConnected      bool
        isAuthenticated  bool
        mu               sync.RWMutex
    }
    
    subscriptions struct {
        mu   sync.RWMutex
        subs map[string]*grpcSubscription
    }
}

// Read lock for reading
e.state.mu.RLock()
isAuth := e.state.isAuthenticated
e.state.mu.RUnlock()

// Write lock for writing
e.state.mu.Lock()
e.state.isAuthenticated = true
e.state.mu.Unlock()
```

**Rationale**: Prevents race conditions in concurrent access.

### 9. Event Handler Pattern

#### 
```go
type EventHandler func(*EventPayload) error

subscription.AddHandler(func(event *EventPayload) error {
    log.Printf("Event: %+v", event)
    return subscription.Ack(event.Idem, event.Block)
})
```

**Rationale**:
- Function type is idiomatic Go
- Error return allows handler to signal failures
- No need for async/await syntax

### 10. Streaming with gRPC

```go
// Start listening in goroutine
go func() {
    for {
        event, err := stream.Recv()
        if err == io.EOF {
            return
        }
        if err != nil {
            log.Printf("Stream error: %v", err)
            return
        }
        
        // Process event
        processEvent(event)
    }
}()
```

**Rationale**: gRPC streams map naturally to goroutines.

### 11. WebSocket Message Handling

```go
// Message callbacks with channels
type messageCallback struct {
    resolve chan string
    reject  chan error
    timeout *time.Timer
}

func (e *WebSocketEngine) sendMessage(message string) (string, error) {
    callback := &messageCallback{
        resolve: make(chan string, 1),
        reject:  make(chan error, 1),
    }
    
    // Send message
    e.conn.WriteMessage(websocket.TextMessage, []byte(message))
    
    // Wait for response
    select {
    case resp := <-callback.resolve:
        return resp, nil
    case err := <-callback.reject:
        return "", err
    }
}
```

**Rationale**: Channels replace promise-based async patterns.


**Go:**
```go
import (
    "google.golang.org/grpc"
)

// Exported types are automatically available
type Engine interface { ... }
```

### 2. EventEmitter

**Go:**
```go
// Direct function calls or channels
subscription.AddHandler(handler)

// Or channels for events
errorChan := make(chan error)
```



**Go:**
```go
func publish() (string, error) {
    result, err := client.Publish(...)
    if err != nil {
        return "", err
    }
    return result, nil
}
```

### 4. Callbacks

**Go:**
```go
resp, err := client.Connect(ctx, request)
if err != nil {
    return err
}
// Use resp
```

### 5. Private Fields with `#`

**Go:**
```go
type GRPCEngine struct {
    client pb.EnSyncServiceClient  // unexported (lowercase)
    config struct {                // unexported
        url string
    }
}
```

**Go:**
```go
type GRPCEngine struct {
    client pb.EnSyncServiceClient  // unexported (lowercase)
    config struct {                // unexported
        url string
    }
}
```

### 6. Dynamic Typing

**Go:**
```go
payload := map[string]interface{}{
    "any": "structure",
}
```

## Package Dependencies


| Go Equivalent | Purpose |
|---------------|---------|
| `google.golang.org/grpc` | gRPC client |
| `google.golang.org/protobuf` | Protocol buffers |
| `github.com/gorilla/websocket` | WebSocket client |
| `golang.org/x/crypto/nacl` | Cryptography |
| `tweetnacl-util` | `encoding/base64` | Base64 encoding |
| `ed2curve` | Custom implementation | Ed25519 to Curve25519 |

## Testing Strategy

```go
// Mock interfaces for testing
type MockEngine struct {
    PublishFunc func(...) (string, error)
}

func (m *MockEngine) Publish(...) (string, error) {
    if m.PublishFunc != nil {
        return m.PublishFunc(...)
    }
    return "", nil
}

// Test with mock
func TestPublish(t *testing.T) {
    mock := &MockEngine{
        PublishFunc: func(...) (string, error) {
            return "event-123", nil
        },
    }
    
    eventID, err := mock.Publish(...)
    if err != nil {
        t.Fatal(err)
    }
    if eventID != "event-123" {
        t.Errorf("expected event-123, got %s", eventID)
    }
}
```

## Performance Considerations

1. **Goroutine Pooling**: Consider using worker pools for high-volume event processing
2. **Buffer Sizes**: Channels use buffered channels (size 1) to prevent blocking
3. **Connection Pooling**: gRPC handles connection pooling internally
4. **Memory Management**: Go's GC handles cleanup, but be mindful of goroutine leaks

## Future Enhancements

1. **Metrics**: Add Prometheus metrics support
2. **Tracing**: OpenTelemetry integration
3. **Retry Logic**: Exponential backoff for failed operations
4. **Connection Pool**: Custom connection pooling for WebSocket
5. **Batch Operations**: Batch publish for multiple events

## Conclusion

The Go SDK embracing Go's idioms:

- Interfaces over classes
- Goroutines over callbacks/promises
- Channels over EventEmitter
- Explicit error handling over exceptions
- Strong typing over dynamic typing
- Functional options over config objects

This results in a more type-safe, concurrent, and idiomatic Go library.
