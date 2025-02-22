# EnSync Golang SDK

This is the official Golang SDK for EnSync, providing a robust and type-safe interface for interacting with EnSync servers.

## Features

- Modern functional options pattern for configuration
- Native HTTP/2 support with automatic fallback to HTTP/1.1
- Event subscription and publishing
- Record acknowledgment and rollback
- Automatic client renewal
- TLS support with optional verification
- Error handling with custom error types

## Installation

```bash
go get github.com/ensync/golang-sdk
```

## Usage

Here's a basic example of using the SDK:

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    ensync "github.com/ensync/golang-sdk"
)

func main() {
    // Create a new EnSync engine with functional options
    engine, err := ensync.NewEnSyncEngine(
        "https://your-ensync-server.com",
        ensync.WithVersion("v1"),
        ensync.WithAccessKey("your-access-key"),
        ensync.WithHTTP2(),              // Enable HTTP/2 support
        ensync.WithTLSConfig(false),     // Enable TLS verification
        ensync.WithRenewAt(420000),      // Set renewal time
    )
    if err != nil {
        log.Fatal(err)
    }

    // Subscribe to events
    ctx := context.Background()
    err = engine.Subscribe(ctx, []string{"myEvent"}, ensync.EnSyncSubscribeOptions{
        AutoAck: true,
    }, func(record ensync.EnSyncRecord) error {
        fmt.Printf("Received record: %+v\n", record)
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }

    // Publish an event
    payload := ensync.EnSyncEventPayload{
        Name: "myEvent",
        Data: map[string]interface{}{
            "message": "Hello, EnSync!",
        },
    }
    err = engine.Publish(ctx, "myEvent", payload, ensync.EnSyncPublishOptions{
        Persist: true,
    })
    if err != nil {
        log.Fatal(err)
    }
}
```

## Configuration Options

The SDK uses the functional options pattern for configuration. Here are the available options:

- `WithAccessKey(key string)`: Sets the access key for authentication
- `WithVersion(version string)`: Sets the API version (default: "v1")
- `WithHTTP2()`: Enables HTTP/2 support
- `WithTLSConfig(disableTLS bool)`: Configures TLS verification
- `WithRenewAt(renewAt int64)`: Sets the client renewal time in milliseconds

## Error Handling

The SDK includes a custom error type `EnSyncError` that provides detailed error information:

```go
if err != nil {
    if enSyncErr, ok := err.(*ensync.EnSyncError); ok {
        fmt.Printf("EnSync error: %s (type: %s)\n", enSyncErr.Message, enSyncErr.Name)
    }
}
```

## HTTP/2 Support

The SDK automatically uses HTTP/2 when enabled via `WithHTTP2()`. This provides:
- Improved performance through multiplexing
- Header compression
- Server push capabilities
- Automatic fallback to HTTP/1.1 if needed

## License

MIT License
