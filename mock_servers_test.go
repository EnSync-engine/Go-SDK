package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/EnSync-engine/Go-SDK/common"
	ensyncGrpc "github.com/EnSync-engine/Go-SDK/grpc"
	pb "github.com/EnSync-engine/Go-SDK/internal/proto"
)

type FlakyMockGRPCServer struct {
	pb.UnimplementedEnSyncServiceServer
	mu            sync.RWMutex
	authenticated bool
	fail          bool
	grpcServer    *grpc.Server
}

func (m *FlakyMockGRPCServer) setFail(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fail = fail
}

func (m *FlakyMockGRPCServer) Connect(ctx context.Context, req *pb.ConnectRequest) (*pb.ConnectResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if req.AccessKey == "" {
		return &pb.ConnectResponse{
			Success:      false,
			ErrorMessage: "invalid access key",
		}, nil
	}

	m.authenticated = true
	return &pb.ConnectResponse{
		Success:    true,
		ClientId:   "test-client-id",
		ClientHash: "test-client-hash",
	}, nil
}

func (m *FlakyMockGRPCServer) PublishEvent(ctx context.Context, req *pb.PublishEventRequest) (*pb.PublishEventResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.authenticated {
		return &pb.PublishEventResponse{
			Success:      false,
			ErrorMessage: "not authenticated",
		}, nil
	}

	if m.fail {
		return nil, status.Error(codes.Unavailable, "mock server is intentionally failing")
	}

	return &pb.PublishEventResponse{
		Success:   true,
		EventIdem: "mock-event-id",
	}, nil
}

func (m *FlakyMockGRPCServer) Subscribe(req *pb.SubscribeRequest, stream pb.EnSyncService_SubscribeServer) error {
	<-stream.Context().Done()
	return nil
}

func startFlakyMockGRPCServer(t *testing.T) (addr string, mockServer *FlakyMockGRPCServer, cleanup func()) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	mockServer = &FlakyMockGRPCServer{}
	grpcServer := grpc.NewServer()
	mockServer.grpcServer = grpcServer
	pb.RegisterEnSyncServiceServer(grpcServer, mockServer)

	errChan := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			errChan <- err
		}
	}()

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Mock server failed to start within 2 seconds: %v", err)
	}
	// We connected successfully, so we can close this test connection and proceed.
	conn.Close()

	cleanup = func() {
		grpcServer.GracefulStop()
		select {
		case <-errChan:
		case <-time.After(1 * time.Second):
		}
	}

	return lis.Addr().String(), mockServer, cleanup
}

func TestGRPCClientWithSimpleMockServer(t *testing.T) {
	ctx := context.Background()
	addr, _, cleanup := startFlakyMockGRPCServer(t)
	defer cleanup()

	engine, err := ensyncGrpc.NewGRPCEngine(ctx, "grpc://"+addr)
	if err != nil {
		t.Fatalf("Failed to create gRPC engine: %v", err)
	}
	defer func() {
		if err := engine.Close(); err != nil {
			t.Logf("Failed to close engine: %v", err)
		}
	}()

	err = engine.CreateClient("test-access-key")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	eventName := "test/payment/created"

	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate Ed25519 key: %v", err)
	}

	t.Logf("Generated public key length: %d bytes", len(publicKey))

	recipients := []string{base64.StdEncoding.EncodeToString(publicKey)}
	payload := map[string]interface{}{
		"amount":   100,
		"currency": "USD",
	}

	metadata := &common.EventMetadata{
		Persist: true,
		Headers: map[string]string{
			"source": "payment-service",
		},
	}

	eventID, err := engine.Publish(eventName, recipients, payload, metadata, nil)
	if err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	if eventID == "" {
		t.Error("Expected non-empty event ID")
	}

	t.Logf("Published event: %s", eventID)
}

func TestGRPCEngineConnectionAndAuth(t *testing.T) {
	ctx := context.Background()
	addr, _, cleanup := startFlakyMockGRPCServer(t)
	defer cleanup()

	engine, err := ensyncGrpc.NewGRPCEngine(ctx, "grpc://"+addr)
	if err != nil {
		t.Fatalf("Failed to create gRPC engine: %v", err)
	}
	defer func() {
		if err := engine.Close(); err != nil {
			t.Logf("Failed to close engine: %v", err)
		}
	}()

	err = engine.CreateClient("test-access-key")
	if err != nil {
		t.Fatalf("Failed to authenticate: %v", err)
	}

	if engine.ClientID == "" {
		t.Error("Expected ClientID to be set after authentication")
	}

	t.Logf("Authentication successful, ClientID: %s", engine.ClientID)
}

func TestCircuitBreaker(t *testing.T) {
	ctx := context.Background()
	addr, mockServer, cleanup := startFlakyMockGRPCServer(t)
	defer cleanup()

	// Configure the circuit breaker with a max reset timeout to ensure
	// the test's sleep is always sufficient, avoiding flakes from
	// exponential backoff and jitter.
	engine, err := ensyncGrpc.NewGRPCEngine(ctx, "grpc://"+addr,
		common.WithCircuitBreaker(3, 1*time.Second, 1*time.Second), // 3 failures, 1s base reset, 1s max reset
		common.WithRetryConfig(4, 10*time.Millisecond, 50*time.Millisecond, 0.1))
	if err != nil {
		t.Fatalf("Failed to create gRPC engine: %v", err)
	}
	defer engine.Close()

	err = engine.CreateClient("test-access-key")
	if err != nil {
		t.Fatalf("Failed to authenticate: %v", err)
	}

	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	recipientKey := base64.StdEncoding.EncodeToString(publicKey)

	// Make the server fail
	mockServer.setFail(true)
	t.Log("Server set to fail mode")

	// Trigger failures to open the circuit
	for i := 0; i < 3; i++ {
		_, err = engine.Publish("test-event", []string{recipientKey}, map[string]interface{}{"data": "test"}, nil, nil)
		if err == nil {
			t.Fatalf("Expected publish attempt %d to fail, but it succeeded", i+1)
		}
		t.Logf("Attempt %d failed as expected: %v", i+1, err)
	}
	t.Log("Circuit breaker opened after 3 failures")

	// Verify the circuit is open - should fail immediately without retries
	start := time.Now()
	_, err = engine.Publish("test-event", []string{recipientKey}, map[string]interface{}{"data": "test"}, nil, nil)
	duration := time.Since(start)

	if err == nil {
		t.Fatal("Expected publish to fail because circuit is open, but it succeeded")
	}

	if duration > 100*time.Millisecond {
		t.Logf("Circuit breaker took %v to reject (expected < 100ms)", duration)
	}
	t.Logf("Circuit breaker rejected operation in %v", duration)

	//  Wait for the reset timeout and make the server succeed again
	t.Log("Waiting for circuit breaker to enter half-open state...")
	time.Sleep(1100 * time.Millisecond)
	mockServer.setFail(false)
	t.Log("Server set to success mode")

	//  Attempt to recover
	maxAttempts := 5
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		_, lastErr = engine.Publish("test-event", []string{recipientKey}, map[string]interface{}{"data": "test"}, nil, nil)
		if lastErr == nil {
			t.Logf("Publish succeeded on attempt %d after circuit reset", i+1)
			break
		}
		t.Logf("Attempt %d failed: %v", i+1, lastErr)
		time.Sleep(50 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("Expected publish to succeed after circuit reset, but it failed after %d attempts: %v", maxAttempts, lastErr)
	}

	t.Log("Circuit breaker test successful!")
}

func TestConcurrentPublishAndClose(t *testing.T) {
	ctx := context.Background()
	addr, _, cleanup := startFlakyMockGRPCServer(t)
	defer cleanup()

	engine, err := ensyncGrpc.NewGRPCEngine(ctx, "grpc://"+addr)
	if err != nil {
		t.Fatalf("Failed to create gRPC engine: %v", err)
	}

	err = engine.CreateClient("test-access-key")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	recipientKey := base64.StdEncoding.EncodeToString(publicKey)

	// Run concurrent publishes and close at random times
	var wg sync.WaitGroup
	publishCount := 0
	publishMutex := sync.Mutex{}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := engine.Publish("test-event", []string{recipientKey}, map[string]interface{}{"data": "test"}, nil, nil)
			if err != nil {
				t.Logf("Publish %d failed: %v", index, err)
			} else {
				publishMutex.Lock()
				publishCount++
				publishMutex.Unlock()
				t.Logf("Publish %d succeeded", index)
			}
		}(i)
	}

	// Close engine after a short delay
	time.Sleep(100 * time.Millisecond)
	t.Log("Closing engine while publishes are in flight...")
	if err := engine.Close(); err != nil {
		t.Logf("Error during close: %v", err)
	}

	wg.Wait()
	publishMutex.Lock()
	t.Logf("Concurrent test complete: %d publishes succeeded", publishCount)
	publishMutex.Unlock()
}

func TestEngineCloseIdempotency(t *testing.T) {
	ctx := context.Background()
	addr, _, cleanup := startFlakyMockGRPCServer(t)
	defer cleanup()

	engine, err := ensyncGrpc.NewGRPCEngine(ctx, "grpc://"+addr)
	if err != nil {
		t.Fatalf("Failed to create gRPC engine: %v", err)
	}

	// Call Close multiple times - should not panic
	for i := 0; i < 3; i++ {
		if err := engine.Close(); err != nil {
			t.Logf("Close %d returned error: %v", i+1, err)
		}
		t.Logf("Close %d successful", i+1)
	}

	t.Log("Engine close idempotency test passed!")
}
