# EnSync SDK Examples

This directory contains examples demonstrating how to use the EnSync Golang SDK.

## Producer Example

The producer example (`producer/main.go`) shows how to:
- Create an EnSync engine
- Publish events to a specific event name
- Handle errors and measure performance

To run the producer:
```bash
cd producer
go run main.go
```

## Consumer Example

The consumer example (`consumer/main.go`) shows how to:
- Create an EnSync engine
- Subscribe to specific events
- Handle received records
- Implement graceful shutdown

To run the consumer:
```bash
cd consumer
go run main.go
```

## Notes

- Both examples assume a local EnSync server running on `https://localhost:8443`
- TLS verification is disabled for local development (`DisableTLS: true`)
- The consumer example uses graceful shutdown handling with OS signals
- The producer example includes timing measurements for event publishing

## Running the Examples

1. Start your local EnSync server
2. Open two terminal windows
3. In the first window, run the consumer:
   ```bash
   cd consumer
   go run main.go
   ```
4. In the second window, run the producer:
   ```bash
   cd producer
   go run main.go
   ```
5. You should see the consumer receiving the events published by the producer

To stop the consumer, press Ctrl+C for graceful shutdown.
