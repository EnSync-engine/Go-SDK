package grpc

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/EnSync-engine/Go-SDK/common"
	pb "github.com/EnSync-engine/Go-SDK/internal/proto"
)

type grpcSubscription struct {
	*common.BaseSubscription
	stream       pb.EnSyncService_SubscribeClient
	appSecretKey string
	engine       *GRPCEngine
	cancel       context.CancelFunc
	mu           sync.Mutex
	listenerWg   sync.WaitGroup
}

func (s *grpcSubscription) Defer(messageIdem string, delayMs int64, reason string) (*common.DeferResponse, error) {
	var resp *common.DeferResponse

	err := s.engine.WithRetry(s.engine.GetConnectionContext(), func() error {
		ctx, cancel := context.WithTimeout(s.engine.GetConnectionContext(), s.engine.GetOperationTimeout())
		defer cancel()

		deferResp, deferErr := s.engine.client.DeferMessage(ctx, &pb.DeferRequest{
			ClientId:    s.engine.GetClientID(),
			MessageIdem: messageIdem,
			DelayMs:     delayMs,
			Reason:      reason,
		})

		if deferErr != nil {
			return common.NewEnSyncError("defer request failed", common.ErrTypeDefer, deferErr)
		}

		if !deferResp.Success {
			return common.NewEnSyncError("defer rejected: "+deferResp.Message, common.ErrTypeDefer, nil)
		}

		resp = &common.DeferResponse{
			MessageID:         messageIdem,
			Status:            statusDeferred,
			DelayMs:           delayMs,
			ScheduledDelivery: time.Now().Add(time.Duration(delayMs) * time.Millisecond),
			Timestamp:         time.Now(),
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *grpcSubscription) Discard(messageIdem, reason string) (*common.DiscardResponse, error) {
	var resp *common.DiscardResponse

	err := s.engine.WithRetry(s.engine.GetConnectionContext(), func() error {
		ctx, cancel := context.WithTimeout(s.engine.GetConnectionContext(), s.engine.GetOperationTimeout())
		defer cancel()

		discardResp, discardErr := s.engine.client.DiscardMessage(ctx, &pb.DiscardRequest{
			ClientId:    s.engine.GetClientID(),
			MessageIdem: messageIdem,
			Reason:      reason,
		})

		if discardErr != nil {
			return common.NewEnSyncError("discard request failed", common.ErrTypeDiscard, discardErr)
		}

		if !discardResp.Success {
			return common.NewEnSyncError("discard rejected: "+discardResp.Message, common.ErrTypeDiscard, nil)
		}

		resp = &common.DiscardResponse{
			MessageID: messageIdem,
			Status:    statusSuccess,
			Action:    actionDiscarded,
			Timestamp: time.Now(),
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *grpcSubscription) Pause(reason string) error {
	return s.engine.WithRetry(s.engine.GetConnectionContext(), func() error {
		ctx, cancel := context.WithTimeout(s.engine.GetConnectionContext(), s.engine.GetOperationTimeout())
		defer cancel()

		resp, err := s.engine.client.PauseMessages(ctx, &pb.PauseRequest{
			ClientId:    s.engine.GetClientID(),
			MessageName: s.MessageName,
			Reason:      reason,
		})

		if err != nil {
			return common.NewEnSyncError("pause request failed", common.ErrTypePause, err)
		}

		if !resp.Success {
			return common.NewEnSyncError("pause rejected: "+resp.Message, common.ErrTypePause, nil)
		}

		return nil
	})
}

func (s *grpcSubscription) Resume() error {
	return s.engine.WithRetry(s.engine.GetConnectionContext(), func() error {
		ctx, cancel := context.WithTimeout(s.engine.GetConnectionContext(), s.engine.GetOperationTimeout())
		defer cancel()

		resp, err := s.engine.client.ContinueMessages(ctx, &pb.ContinueRequest{
			ClientId:    s.engine.GetClientID(),
			MessageName: s.MessageName,
		})

		if err != nil {
			return common.NewEnSyncError("resume request failed", common.ErrTypeResume, err)
		}

		if !resp.Success {
			return common.NewEnSyncError("resume rejected: "+resp.Message, common.ErrTypeResume, nil)
		}

		return nil
	})
}

func (s *grpcSubscription) Replay(messageIdem string) (*common.MessagePayload, error) {
	var result *common.MessagePayload

	err := s.engine.WithRetry(s.engine.GetConnectionContext(), func() error {
		ctx, cancel := context.WithTimeout(s.engine.GetConnectionContext(), s.engine.GetOperationTimeout())
		defer cancel()

		resp, replayErr := s.engine.client.ReplayMessage(
			ctx, &pb.ReplayRequest{
				ClientId:    s.engine.GetClientID(),
				MessageName: s.MessageName,
				MessageIdem: messageIdem,
			})

		if replayErr != nil {
			return common.NewEnSyncError("replay request failed", common.ErrTypeReplay, replayErr)
		}

		if !resp.Success {
			return common.NewEnSyncError("replay rejected: "+resp.Message, common.ErrTypeReplay, nil)
		}

		var messageData pb.MessageStreamResponse
		if err := protojson.Unmarshal([]byte(resp.MessageData), &messageData); err != nil {
			return common.NewEnSyncError("failed to parse replayed message", common.ErrTypeReplay, err)
		}

		processed, processErr := s.processMessage(&messageData)
		if processErr != nil {
			return processErr
		}

		result = processed
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *grpcSubscription) Rollback(messageIdem string, block int64) error {
	return common.NewEnSyncError("rollback not implemented in gRPC version", common.ErrTypeGeneric, nil)
}

func (s *grpcSubscription) listen(stream pb.EnSyncService_SubscribeClient) {
	s.listenerWg.Add(1)
	defer s.listenerWg.Done()

	defer func() {
		if r := recover(); r != nil {
			s.engine.Logger().Error("Subscription listener panic",
				zap.String("eventName", s.MessageName),
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	recvChan := make(chan *pb.MessageStreamResponse, s.engine.GetRecvBufferSize())
	errChan := make(chan error, 1)
	recvDone := make(chan struct{})

	go func() {
		defer close(recvDone)
		for {
			event, err := stream.Recv()
			if err != nil {
				select {
				case errChan <- err:
				case <-s.engine.GetConnectionContext().Done():
				}
				return
			}
			select {
			case recvChan <- event:
			case <-time.After(s.engine.GetRecvTimeout()):
				s.engine.Logger().Warn("RecvChan full, dropping message",
					zap.String("eventName", s.MessageName),
					zap.String("messageId", event.MessageIdem))
			case <-s.engine.GetConnectionContext().Done():
				return
			}
		}
	}()

	connCtx := s.engine.GetConnectionContext()

	for {
		select {
		case <-connCtx.Done():
			s.engine.Logger().Debug("Subscription listener stopped (connection closed)",
				zap.String("eventName", s.MessageName))
			return

		case <-s.engine.Ctx.Done():
			return

		case event := <-recvChan:
			s.handleMessage(event)

		case err := <-errChan:
			s.handleStreamError(err)
			return
		}
	}
}

func (s *grpcSubscription) handleStreamError(err error) {
	if s.engine.GetConnectionContext().Err() != nil {
		s.engine.Logger().Debug("Stream closed due to connection context cancellation",
			zap.String("eventName", s.MessageName))
		return
	}

	if st, ok := status.FromError(err); ok && st.Code() == codes.Canceled {
		s.engine.Logger().Info("Stream canceled",
			zap.String("eventName", s.MessageName),
			zap.String("reason", st.Message()))
		return
	}

	s.engine.Logger().Error("Stream receive error",
		zap.String("eventName", s.MessageName),
		zap.Error(err))

	if s.engine.IsConnected() {
		s.engine.Logger().Debug("Attempting local resubscription", zap.String("eventName", s.MessageName))
		if err := s.resubscribe(); err == nil {
			s.engine.Logger().Debug("Local resubscription successful", zap.String("eventName", s.MessageName))
			return
		} else {
			s.engine.Logger().Warn("Local resubscription failed",
				zap.String("eventName", s.MessageName),
				zap.Error(err))
		}
	}

	s.engine.SetConnectionState(false)
	go s.engine.reconnect()
}

func (s *grpcSubscription) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()

	timeout := s.engine.GetGracefulShutdownTimeout()

	done := make(chan struct{})
	go func() {
		s.listenerWg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		s.engine.Logger().Warn("Stop() timed out",
			zap.String("eventName", s.MessageName))
	}
}

func (s *grpcSubscription) resubscribe() error {
	s.Stop()

	return s.engine.WithRetry(s.engine.Ctx, func() error {
		if !s.engine.IsConnected() {
			return common.NewEnSyncError("engine disconnected", common.ErrTypeConnection, nil)
		}

		connCtx := s.engine.GetConnectionContext()
		if connCtx.Err() != nil {
			return common.NewEnSyncError("connection session ended", common.ErrTypeConnection, connCtx.Err())
		}

		subCtx, cancel := context.WithCancel(connCtx)

		stream, err := s.engine.client.Subscribe(subCtx, &pb.SubscribeRequest{
			ClientId:    s.engine.GetClientID(),
			MessageName: s.MessageName,
		})

		if err != nil {
			cancel()
			return err
		}

		s.mu.Lock()
		s.stream = stream
		s.cancel = cancel
		s.mu.Unlock()

		go s.listen(stream)
		return nil
	})
}

func (s *grpcSubscription) handleMessage(event *pb.MessageStreamResponse) {
	processedMessage, err := s.processMessage(event)
	if err != nil {
		s.engine.Logger().Error("Message processing failed",
			zap.String("messageName", s.MessageName),
			zap.String("messageId", event.MessageIdem),
			zap.Error(err))
		return
	}
	if processedMessage == nil {
		return
	}
	logger := s.engine.Logger()

	s.ProcessMessage(processedMessage, logger, s.Ack)
}

func (s *grpcSubscription) processMessage(message *pb.MessageStreamResponse) (*common.MessagePayload, error) {
	logger := s.engine.Logger()
	decrypted, err := common.DecryptAndParseMessage(
		message.Payload,
		message.Metadata,
		s.appSecretKey,
		logger,
	)
	if err != nil {
		return nil, err
	}

	return &common.MessagePayload{
		MessageName: message.MessageName,
		Idem:        message.MessageIdem,
		Block:       message.PartitionBlock,
		Timestamp:   time.Now().UnixMilli(),
		Payload:     decrypted.Payload,
		Metadata:    decrypted.Metadata,
		Sender:      message.Sender,
	}, nil
}

func (s *grpcSubscription) Ack(messageID string, block int64) error {
	return s.engine.WithRetry(s.engine.GetConnectionContext(), func() error {
		ctx, cancel := context.WithTimeout(s.engine.GetConnectionContext(), s.engine.GetOperationTimeout())
		defer cancel()

		resp, err := s.engine.client.AcknowledgeMessage(ctx, &pb.AcknowledgeRequest{
			ClientId:       s.engine.GetClientID(),
			MessageIdem:    messageID,
			MessageName:    s.MessageName,
			PartitionBlock: block,
		})

		if err != nil {
			return common.NewEnSyncError("ack request failed", common.ErrTypeSubscription, err)
		}

		if !resp.Success {
			return common.NewEnSyncError("ack rejected: "+resp.Message, common.ErrTypeSubscription, nil)
		}

		return nil
	})
}

func (s *grpcSubscription) Unsubscribe() error {
	return s.engine.WithRetry(s.engine.GetConnectionContext(), func() error {
		ctx, cancel := context.WithTimeout(s.engine.GetConnectionContext(), s.engine.GetOperationTimeout())
		defer cancel()

		resp, err := s.engine.client.Unsubscribe(ctx, &pb.UnsubscribeRequest{
			ClientId:    s.engine.GetClientID(),
			MessageName: s.MessageName,
		})

		if err != nil {
			return common.NewEnSyncError("unsubscribe request failed", common.ErrTypeSubscription, err)
		}

		if !resp.Success {
			return common.NewEnSyncError("unsubscribe rejected: "+resp.Message, common.ErrTypeSubscription, nil)
		}

		s.cancel()
		s.engine.SubscriptionMgr.Delete(s.MessageName)

		s.engine.Logger().Info("Successfully unsubscribed", zap.String("eventName", s.MessageName))
		return nil
	})
}
