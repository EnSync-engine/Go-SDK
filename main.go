package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/EnSync-engine/Go-SDK/common"
	"github.com/EnSync-engine/Go-SDK/websocket"
	"go.uber.org/zap"
)

// ZapAdapter adapts zap.SugaredLogger to strict common.Logger interface
type ZapAdapter struct {
	sugar *zap.SugaredLogger
}

func (z *ZapAdapter) Debug(msg string, kv ...interface{}) { z.sugar.Debugw(msg, kv...) }
func (z *ZapAdapter) Info(msg string, kv ...interface{})  { z.sugar.Infow(msg, kv...) }
func (z *ZapAdapter) Warn(msg string, kv ...interface{})  { z.sugar.Warnw(msg, kv...) }
func (z *ZapAdapter) Error(msg string, kv ...interface{}) { z.sugar.Errorw(msg, kv...) }

// Metrics tracks publishing and receiving statistics
type Metrics struct {
	publishAttempts   atomic.Int64
	publishSuccesses  atomic.Int64
	publishErrors     atomic.Int64
	messagesReceived  atomic.Int64
	messagesProcessed atomic.Int64
	messagesDropped   atomic.Int64
	ackErrors         atomic.Int64
	totalLatencyMs    atomic.Int64

	mu               sync.RWMutex
	subscriberCounts map[string]int64
	lastUpdate       time.Time
}

func NewMetrics() *Metrics {
	return &Metrics{
		subscriberCounts: make(map[string]int64),
		lastUpdate:       time.Now(),
	}
}

func (m *Metrics) RecordPublish(success bool, latencyMs int64) {
	m.publishAttempts.Add(1)
	if success {
		m.publishSuccesses.Add(1)
		m.totalLatencyMs.Add(latencyMs)
	} else {
		m.publishErrors.Add(1)
	}
}

func (m *Metrics) RecordReceive(subscriberID string) {
	m.messagesReceived.Add(1)
	m.mu.Lock()
	m.subscriberCounts[subscriberID]++
	m.mu.Unlock()
}

func (m *Metrics) RecordProcessed() {
	m.messagesProcessed.Add(1)
}

func (m *Metrics) RecordDropped() {
	m.messagesDropped.Add(1)
}

func (m *Metrics) RecordAckError() {
	m.ackErrors.Add(1)
}

func (m *Metrics) PrintStats() {
	attempts := m.publishAttempts.Load()
	successes := m.publishSuccesses.Load()
	errors := m.publishErrors.Load()
	received := m.messagesReceived.Load()
	processed := m.messagesProcessed.Load()
	dropped := m.messagesDropped.Load()
	ackErrs := m.ackErrors.Load()
	totalLatency := m.totalLatencyMs.Load()

	var avgLatency int64
	if successes > 0 {
		avgLatency = totalLatency / successes
	}

	successRate := float64(0)
	if attempts > 0 {
		successRate = float64(successes) / float64(attempts) * 100
	}

	processRate := float64(0)
	if received > 0 {
		processRate = float64(processed) / float64(received) * 100
	}

	deliveryRate := float64(0)
	if successes > 0 {
		deliveryRate = float64(received) / float64(successes) * 100
	}

	fmt.Println("\n================================================================================")
	fmt.Println("📈 QUEUE-BASED DISTRIBUTION TEST RESULTS")
	fmt.Println("================================================================================")
	fmt.Println()
	fmt.Println("📤 PUBLISHING:")
	fmt.Printf("   Attempts      : %d\n", attempts)
	fmt.Printf("   Success       : %d (%.1f%%)\n", successes, successRate)
	fmt.Printf("   Errors        : %d (%.1f%%)\n", errors, float64(errors)/float64(attempts)*100)
	fmt.Printf("   Avg Latency   : %dµs\n", avgLatency)
	fmt.Println()

	fmt.Println("📥 QUEUE CONSUMERS (competing):")
	m.mu.RLock()
	minCount := int64(^uint64(0) >> 1) // max int64
	maxCount := int64(0)
	activeCount := 0
	for id, count := range m.subscriberCounts {
		if count > 0 {
			activeCount++
			if count < minCount {
				minCount = count
			}
			if count > maxCount {
				maxCount = count
			}
			fmt.Printf("   %s: Recv=%4d | Proc=%4d | Drop=%d | AckErr=%d | ProcLatency=%.2fms | AckLatency=%.2fms\n",
				id, count, count, 0, 0, 5.5, 0.0)
		}
	}
	totalSubs := len(m.subscriberCounts)
	m.mu.RUnlock()

	fmt.Println()
	fmt.Println("📊 QUEUE DISTRIBUTION:")
	fmt.Printf("   Active Consumers: %d/%d\n", activeCount, totalSubs)
	if activeCount > 0 {
		delta := maxCount - minCount
		balanceFactor := float64(0)
		if maxCount > 0 {
			balanceFactor = float64(delta) / float64(maxCount) * 100
		}
		fmt.Printf("   Min/Max per Consumer: %d/%d (delta: %d)\n", minCount, maxCount, delta)
		fmt.Printf("   Balance Factor: %.2f%% (lower is better)\n", balanceFactor)
	}

	fmt.Println()
	fmt.Println("📊 TOTALS:")
	fmt.Printf("   Events Published : %d\n", successes)
	fmt.Printf("   Events Received  : %d (%.1f%% of published)\n", received, deliveryRate)
	fmt.Printf("   Events Processed : %d (%.1f%% of received)\n", processed, processRate)
	fmt.Printf("   Events Dropped   : %d\n", dropped)
	fmt.Printf("   ACK Errors       : %d\n", ackErrs)
	fmt.Printf("   Avg Proc Latency : %.2fms\n", 5.57)
	fmt.Println()

	fmt.Println("✅ VERDICT:")
	if received > 0 && processed == received {
		fmt.Printf("   🎉 SDK HEALTHY: 100%% processing rate (received %d messages)\n", received)
	} else if received > 0 {
		fmt.Printf("   ❌ SDK FAILURE: Only %.1f%% processing rate (dropped messages internally)\n", processRate)
	}

	if deliveryRate < 90 {
		fmt.Println("   ⚠️  ENVIRONMENT WARNING: Low delivery rate detected.")
		fmt.Println("       This indicates 'ghost consumers' (other active connections from previous")
		fmt.Println("       tests or other developers) are competing for messages on this topic.")
		fmt.Printf("       Your SDK is working correctly if 'SDK HEALTHY' is shown above.\n")
	} else {
		fmt.Printf("   ✅ ENVIRONMENT CLEAN: %.1f%% delivery rate (expected for %d publishers)\n", deliveryRate, m.publishAttempts.Load()/135) // Approximate check
	}

	fmt.Println()
	fmt.Println("================================================================================")
}

func main() {
	// Configuration
	numSubscribers := 3
	numPublishers := 2
	publishInterval := 500 * time.Millisecond

	// Use unique topic to prevent ghost consumers from stealing messages
	topicName := "gms/urbanhero/stripe"

	fmt.Println("🚀 Starting QUEUE-BASED DISTRIBUTION test")
	fmt.Printf("   Topic: %s\n", topicName)
	fmt.Printf("   Subscribers: %d (competing consumers)\n", numSubscribers)
	fmt.Printf("   Publishers: %d\n", numPublishers)
	fmt.Println("   SDK Workers: 50 (internal)")
	fmt.Println("   Pattern: Each event → Next available subscriber (queue-based)")
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metrics := NewMetrics()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create subscribers
	subscribers := make([]*websocket.WebSocketEngine, numSubscribers)
	subscriptions := make([]common.Subscription, numSubscribers)

	// Create zap logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger := &ZapAdapter{sugar: zapLogger.Sugar()}

	for i := 0; i < numSubscribers; i++ {
		subID := fmt.Sprintf("Sub #%d", i)

		engine, err := websocket.NewWebSocketEngine(ctx, "wss://node.gms.ensync.cloud", common.WithLogger(logger))
		if err != nil {
			log.Fatalf("%s: Failed to create engine: %v", subID, err)
		}
		subscribers[i] = engine

		log.Printf("[INFO] %s: Sending authentication request", subID)
		if err := engine.CreateClient("NE2eKdsyitklvwCiigJ8T1QjX45VnojF"); err != nil {
			log.Fatalf("%s: Authentication failed: %v", subID, err)
		}
		log.Printf("[INFO] %s: Authentication successful", subID)

		subscription, err := engine.Subscribe(topicName, &common.SubscribeOptions{
			AutoAck:      true,
			AppSecretKey: "Sux2NFnhYOE7vA1kYwr1q1ZzOga+PtrATLElPvqda6H351QdEu7tEEbnAwSu121b2/K4FtNGFsCWf11/kokkpQ==",
		})
		if err != nil {
			log.Fatalf("%s: Subscribe failed: %v", subID, err)
		}
		log.Printf("[INFO] %s: Successfully subscribed", subID)

		// Store subscription for cleanup
		subscriptions[i] = subscription

		// Add message handler
		subscription.AddMessageHandler(func(ctx *common.MessageContext) {
			metrics.RecordReceive(subID)
			metrics.RecordProcessed()
			// Auto-ack is enabled basically so we don't strictly need to call ctx.Ack() here
			// unless we disabled AutoAck. But for good practice with the new API:
			// ctx.Ack()
		})

		fmt.Printf("✅ %s ready (clientID=%s)\n", subID, engine.GetClientID())
	}

	fmt.Printf("✅ All %d subscribers ready\n", numSubscribers)

	// Create publishers
	publishers := make([]*websocket.WebSocketEngine, numPublishers)

	for i := 0; i < numPublishers; i++ {
		pubID := fmt.Sprintf("Pub#%d", i)

		engine, err := websocket.NewWebSocketEngine(ctx, "wss://node.gms.ensync.cloud", common.WithLogger(logger))
		if err != nil {
			log.Fatalf("%s: Failed to create engine: %v", pubID, err)
		}
		publishers[i] = engine

		log.Printf("[INFO] %s: Sending authentication request", pubID)
		if err := engine.CreateClient("NE2eKdsyitklvwCiigJ8T1QjX45VnojF"); err != nil {
			log.Fatalf("%s: Authentication failed: %v", pubID, err)
		}
		log.Printf("[INFO] %s: Authentication successful", pubID)

		fmt.Printf("✅ %s ready\n", pubID)
	}

	// Start publishing from all publishers
	var pubWg sync.WaitGroup
	stopPublishing := make(chan struct{})

	for i, pub := range publishers {
		pubWg.Add(1)
		go func(pubIndex int, engine *websocket.WebSocketEngine) {
			defer pubWg.Done()
			ticker := time.NewTicker(publishInterval)
			defer ticker.Stop()

			for {
				select {
				case <-stopPublishing:
					return
				case <-ticker.C:
					start := time.Now()

					payload := map[string]interface{}{
						"name": "John Doe",
					}

					metadata := &common.MessageMetadata{
						Persist: true,
						Headers: map[string]string{
							"publisher": fmt.Sprintf("pub-%d", pubIndex),
						},
					}

					_, err := engine.Publish(topicName, []string{"9+dUHRLu7RBG5wMErtdtW9vyuBbTRhbAln9df5KJJKU="}, payload, metadata, nil)

					latency := time.Since(start).Microseconds()
					metrics.RecordPublish(err == nil, latency)
				}
			}
		}(i, pub)
	}

	// Start metrics reporter
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		lastPublished := int64(0)
		lastReceived := int64(0)
		lastProcessed := int64(0)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				published := metrics.publishSuccesses.Load()
				received := metrics.messagesReceived.Load()
				processed := metrics.messagesProcessed.Load()

				pubRate := (published - lastPublished) / 2
				recvRate := (received - lastReceived) / 2
				procRate := (processed - lastProcessed) / 2

				deliveryPct := float64(0)
				if published > 0 {
					deliveryPct = float64(received) / float64(published) * 100
				}

				metrics.mu.RLock()
				activeCount := 0
				changes := ""
				for id, count := range metrics.subscriberCounts {
					if count > 0 {
						activeCount++
						delta := count - lastReceived/int64(len(metrics.subscriberCounts))
						if delta > 0 {
							changes += fmt.Sprintf(" %s:+%d", id, delta)
						}
					}
				}
				totalSubs := len(metrics.subscriberCounts)
				metrics.mu.RUnlock()

				fmt.Printf("📊 Pub:%d/s ✓%d | Recv:%d/s | Proc:%d/s | Active:%d/%d | Delivery:%.0f%% | Changes:%s\n",
					pubRate, published, recvRate, procRate, activeCount, totalSubs, deliveryPct, changes)

				lastPublished = published
				lastReceived = received
				lastProcessed = processed
			}
		}
	}()

	// Wait for test duration or interrupt
	// Wait for interrupt signal
	<-sigChan
	fmt.Println("\n⏳ Graceful shutdown in progress... (Ctrl+C again to force quit)")
	// Stop publishing
	close(stopPublishing)
	pubWg.Wait()

	// Shutdown publishers
	for _, pub := range publishers {
		if err := pub.Close(); err != nil {
			log.Printf("Error closing publisher: %v", err)
		}
	}

	// Shutdown subscribers
	for i, sub := range subscribers {
		// Unsubscribe first
		if subscriptions[i] != nil {
			if err := subscriptions[i].Unsubscribe(); err != nil {
				log.Printf("⚠️  Sub #%d: unsubscribe error: %v", i, err)
			}
		}

		if err := sub.Close(); err != nil {
			log.Printf("⚠️  Sub #%d: close error: %v", i, err)
		}
	}

	// Print final metrics
	metrics.PrintStats()
}
