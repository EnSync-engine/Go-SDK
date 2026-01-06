package grpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/EnSync-engine/Go-SDK/common"
	pb "github.com/EnSync-engine/Go-SDK/internal/proto"
)

const (
	statusDeferred  = "deferred"
	statusSuccess   = "success"
	actionDiscarded = "discarded"
)

type GRPCEngine struct {
	*common.BaseEngine
	client           pb.EnSyncServiceClient
	conn             *grpc.ClientConn
	closeOnce        sync.Once
	closed           atomic.Bool
	heartbeatRunning atomic.Bool
}

func NewGRPCEngine(
	ctx context.Context,
	endpoint string,
	opts ...common.Option,
) (*GRPCEngine, error) {
	if ctx == nil {
		return nil, common.NewEnSyncError("context cannot be nil", common.ErrTypeValidation, nil)
	}

	if endpoint == "" {
		return nil, common.NewEnSyncError("endpoint cannot be empty", common.ErrTypeValidation, nil)
	}

	host, secure, err := parseGRPCURL(endpoint)
	if err != nil {
		return nil, err
	}

	baseEngine, err := common.NewBaseEngine(ctx, opts...)
	if err != nil {
		return nil, common.NewEnSyncError("failed to create base engine", common.ErrTypeConnection, err)
	}

	conn, err := createClientConn(host, secure)
	if err != nil {
		return nil, common.NewEnSyncError("failed to connect to gRPC server", common.ErrTypeConnection, err)
	}

	engine := &GRPCEngine{
		BaseEngine: baseEngine,
		client:     pb.NewEnSyncServiceClient(conn),
		conn:       conn,
	}

	return engine, nil
}

func (e *GRPCEngine) CreateClient(accessKey string) error {
	e.SetAccessKey(accessKey)

	if err := e.authenticate(); err != nil {
		return err
	}
	return nil
}

func (e *GRPCEngine) authenticate() error {
	ctx, cancel := context.WithTimeout(e.GetConnectionContext(), e.GetConnectionTimeout())
	defer cancel()

	resp, err := e.client.Connect(ctx, &pb.ConnectRequest{
		AccessKey: e.GetAccessKey(),
	})
	if err != nil {
		return common.NewEnSyncError("connect request failed", common.ErrTypeConnection, err)
	}

	if !resp.Success {
		return common.NewEnSyncError(fmt.Sprintf("authentication rejected: %s", resp.ErrorMessage), common.ErrTypeAuth, nil)
	}

	e.SetAuthenticated(resp.ClientId)
	e.Logger().Info("Authentication successful")

	go e.startHeartbeat()

	return nil
}

func (e *GRPCEngine) startHeartbeat() {
	if !e.heartbeatRunning.CompareAndSwap(false, true) {
		e.Logger().Debug("Heartbeat already running")
		return
	}
	defer e.heartbeatRunning.Store(false)

	ticker := time.NewTicker(e.GetPingInterval())
	defer ticker.Stop()

	e.Logger().Debug("Heartbeat started")

	for {
		select {
		case <-e.Ctx.Done():
			e.Logger().Info("Heartbeat stopped due to engine shutdown")
			return
		case <-ticker.C:
			connCtx := e.GetConnectionContext()
			if connCtx.Err() != nil {
				e.Logger().Debug("Heartbeat stopping: connection context canceled")
				return
			}

			if !e.IsConnected() || e.Reconnecting.Load() {
				continue
			}

			if err := e.sendHeartbeat(); err != nil {
				e.Logger().Error("Heartbeat failed", zap.Error(err))
			}
		}
	}
}

func (e *GRPCEngine) sendHeartbeat() error {
	if !e.IsConnected() {
		return nil
	}

	err := e.WithRetry(e.GetConnectionContext(), func() error {
		ctx, cancel := context.WithTimeout(e.GetConnectionContext(), e.GetOperationTimeout())
		defer cancel()

		resp, err := e.client.Heartbeat(ctx, &pb.HeartbeatRequest{
			ClientId: e.GetClientID(),
		})
		if err != nil {
			return common.NewEnSyncError("heartbeat request failed", common.ErrTypeConnection, err)
		}
		if !resp.Success {
			return common.NewEnSyncError("heartbeat rejected: "+resp.String(), common.ErrTypeConnection, nil)
		}
		return nil
	})

	if err != nil {
		e.Logger().Warn("Heartbeat failed, triggering reconnection", zap.Error(err))
		e.SetConnectionState(false)
		go e.reconnect()
	}

	return err
}

func (e *GRPCEngine) reconnect() {
	if e.closed.Load() {
		return
	}

	if !e.Reconnecting.CompareAndSwap(false, true) {
		e.Logger().Debug("Reconnection already in progress")
		return
	}
	defer e.Reconnecting.Store(false)

	e.ReconnectLoop(
		common.DefaultReconnectConfig(),
		func() {
			e.ResetConnectionContext()
		},
		func() error {
			if err := e.authenticate(); err != nil {
				return err
			}
			return e.restoreSubscriptions()
		},
	)
}

func (e *GRPCEngine) restoreSubscriptions() error {
	var subs []*grpcSubscription
	e.SubscriptionMgr.Range(func(key, value interface{}) bool {
		if sub, ok := value.(*grpcSubscription); ok {
			subs = append(subs, sub)
		}
		return true
	})

	if len(subs) == 0 {
		return nil
	}

	for _, sub := range subs {
		sub.Stop()
	}

	connCtx := e.GetConnectionContext()
	var failedCount int

	for _, sub := range subs {
		subCtx, cancel := context.WithCancel(connCtx)

		stream, err := e.client.Subscribe(subCtx, &pb.SubscribeRequest{
			ClientId:    e.GetClientID(),
			MessageName: sub.MessageName,
		})
		if err != nil {
			cancel()
			e.Logger().Error("Failed to restore subscription",
				zap.String("messageName", sub.MessageName),
				zap.Error(err))
			failedCount++
			continue
		}

		sub.mu.Lock()
		sub.stream = stream
		sub.cancel = cancel
		sub.mu.Unlock()

		go sub.listen(stream)
	}

	if failedCount > 0 {
		return common.NewEnSyncError(
			fmt.Sprintf("failed to restore %d/%d subscriptions", failedCount, len(subs)),
			common.ErrTypeSubscription,
			nil,
		)
	}

	return nil
}

func (e *GRPCEngine) Publish(
	ctx context.Context,
	eventName string,
	recipients []string,
	payload map[string]interface{},
	metadata *common.MessageMetadata,
	options *common.PublishOptions,
) ([]string, error) {
	return common.Publish(e, e.publishToRecipient, ctx, eventName, recipients, payload, metadata, options)
}

func (e *GRPCEngine) publishToRecipient(
	ctx context.Context,
	eventName, recipient string,
	payloadJSON, metadataJSON, payloadMetaJSON []byte,
	useHybrid bool,
) (string, error) {
	encryptedBase64, err := common.EncryptPayload(payloadJSON, recipient, useHybrid)
	if err != nil {
		return "", err
	}

	var messageID string
	err = e.WithRetry(ctx, func() error {
		id, sendErr := e.sendPublishRequest(ctx, eventName, recipient, encryptedBase64, string(metadataJSON), string(payloadMetaJSON))
		if sendErr != nil {
			return sendErr
		}
		messageID = id
		return nil
	})

	if err != nil {
		e.Logger().Error("Failed to publish message",
			zap.String("recipient", recipient),
			zap.String("eventName", eventName),
			zap.Bool("useHybrid", useHybrid),
			zap.Error(err))
		return "", err
	}

	return messageID, nil
}

func (e *GRPCEngine) sendPublishRequest(
	ctx context.Context,
	eventName, recipient, payload, metadata, payloadMeta string,
) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, e.GetOperationTimeout())
	defer cancel()

	req := &pb.PublishMessageRequest{
		ClientId:    e.GetClientID(),
		MessageName: eventName,
		Payload:     payload,
	}

	if recipient != "" {
		req.DeliveryTo = recipient
	}

	if metadata != "" {
		req.Metadata = metadata
	}

	if payloadMeta != "" {
		req.PayloadMetadata = &payloadMeta
	}

	resp, err := e.client.PublishMessage(ctx, req)
	if err != nil {
		return "", common.NewEnSyncError("publish request failed", common.ErrTypePublish, err)
	}

	if !resp.Success {
		return "", common.NewEnSyncError("publish rejected: "+resp.ErrorMessage, common.ErrTypePublish, nil)
	}

	return resp.MessageIdem, nil
}

func (e *GRPCEngine) Subscribe(messageName string, options *common.SubscribeOptions) (common.Subscription, error) {
	if !e.IsConnected() {
		return nil, common.ErrNotAuthenticated
	}

	if e.SubscriptionMgr.Exists(messageName) {
		return nil, common.NewEnSyncError("already subscribed to message "+messageName, common.ErrTypeSubscription, nil)
	}

	if options != nil && options.AppSecretKey == "" {
		return nil, common.NewEnSyncError("app secret key is required for websockets", common.ErrTypeSubscription, nil)
	}

	connCtx := e.GetConnectionContext()
	subCtx, cancel := context.WithCancel(connCtx)

	stream, err := e.client.Subscribe(subCtx, &pb.SubscribeRequest{
		ClientId:    e.GetClientID(),
		MessageName: messageName,
	})

	if err != nil {
		cancel()
		return nil, common.NewEnSyncError("subscription stream error", common.ErrTypeSubscription, err)
	}

	sub := &grpcSubscription{
		BaseSubscription: common.NewBaseSubscription(messageName, options.AutoAck, e.GetSubscriptionWorkerCount()),
		stream:           stream,
		appSecretKey:     options.AppSecretKey,
		engine:           e,
		cancel:           cancel,
	}

	e.SubscriptionMgr.Store(messageName, sub)
	go sub.listen(stream)

	e.Logger().Info("Successfully subscribed", zap.String("messageName", messageName))
	return sub, nil
}

func (e *GRPCEngine) Close() error {
	var closeErr error

	e.closeOnce.Do(func() {
		e.closed.Store(true)
		e.Logger().Info("Shutting down gRPC engine")

		e.ResetState()

		e.SubscriptionMgr.Range(func(key, value interface{}) bool {
			if sub, ok := value.(*grpcSubscription); ok {
				sub.Stop()
			}
			e.SubscriptionMgr.Delete(key.(string))
			return true
		})
		e.Logger().Info("All subscriptions stopped")

		if e.conn != nil {
			if err := e.conn.Close(); err != nil {
				e.Logger().Error("Error closing gRPC connection", zap.Error(err))
				closeErr = common.NewEnSyncError("failed to close gRPC connection", common.ErrTypeConnection, err)
				return
			}
		}

		e.Logger().Info("gRPC engine shutdown complete")
	})

	return closeErr
}
