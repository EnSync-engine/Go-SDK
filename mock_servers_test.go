package ensync

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

	"github.com/EnSync-engine/Go-SDK/common"
	ensyncGrpc "github.com/EnSync-engine/Go-SDK/grpc"
	pb "github.com/EnSync-engine/Go-SDK/internal/proto"
)

type SimpleMockGRPCServer struct {
	pb.UnimplementedEnSyncServiceServer
	mu            sync.RWMutex
	authenticated bool
}

func (m *SimpleMockGRPCServer) Connect(ctx context.Context, req *pb.ConnectRequest) (*pb.ConnectResponse, error) {
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

func (m *SimpleMockGRPCServer) PublishEvent(ctx context.Context, req *pb.PublishEventRequest) (*pb.PublishEventResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.authenticated {
		return &pb.PublishEventResponse{
			Success:      false,
			ErrorMessage: "not authenticated",
		}, nil
	}

	return &pb.PublishEventResponse{
		Success:   true,
		EventIdem: "mock-event-id",
	}, nil
}

func (m *SimpleMockGRPCServer) Subscribe(req *pb.SubscribeRequest, stream pb.EnSyncService_SubscribeServer) error {
	// Just keep the stream open and don't send events to avoid complexity
	<-stream.Context().Done()
	return nil
}

// startSimpleMockGRPCServer starts a simple mock gRPC server
func startSimpleMockGRPCServer(t *testing.T) (addr string) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	mockServer := &SimpleMockGRPCServer{}
	grpcServer := grpc.NewServer()
	pb.RegisterEnSyncServiceServer(grpcServer, mockServer)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	return lis.Addr().String()
}

func TestGRPCClientWithSimpleMockServer(t *testing.T) {
	ctx := context.Background()

	// Start mock gRPC server
	addr := startSimpleMockGRPCServer(t)

	// Create a new gRPC engine
	engine, err := ensyncGrpc.NewGRPCEngine(ctx, "grpc://"+addr)
	if err != nil {
		t.Fatalf("Failed to create gRPC engine: %v", err)
	}
	defer func() {
		if err := engine.Close(); err != nil {
			t.Logf("Failed to close engine: %v", err)
		}
	}()

	// Authenticate with EnSync protocol
	err = engine.CreateClient("test-access-key")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test basic publish with a valid Ed25519 public key
	eventName := "test/payment/created"

	// Generate a real Ed25519 key pair for testing
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

	// Start mock gRPC server
	addr := startSimpleMockGRPCServer(t)

	// Test connection creation
	engine, err := ensyncGrpc.NewGRPCEngine(ctx, "grpc://"+addr)
	if err != nil {
		t.Fatalf("Failed to create gRPC engine: %v", err)
	}
	defer func() {
		if err := engine.Close(); err != nil {
			t.Logf("Failed to close engine: %v", err)
		}
	}()

	// Test authentication
	err = engine.CreateClient("test-access-key")
	if err != nil {
		t.Fatalf("Failed to authenticate: %v", err)
	}

	// Verify engine state
	if engine.ClientID == "" {
		t.Error("Expected ClientID to be set after authentication")
	}

	t.Logf("Authentication successful, ClientID: %s", engine.ClientID)
}
