// Package events 性能基准测试与并发验证
package events

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// BenchmarkKafkaPublish benchmarks Kafka Publisher 的 Publish 性能
func BenchmarkKafkaPublish(b *testing.B) {
	mock := &mockSyncProducer{
		sendFunc: func(msg []byte) (int32, int64, error) {
			return 0, 1, nil
		},
	}
	pub := &kafkaPublisher{producer: mock, topic: "bench-topic"}
	event := SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Connector:  "bench-connector",
		Data: []map[string]any{
			{"hostname": "router-01", "vendor": "Huawei", "status": "Up"},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = pub.Publish(context.Background(), event)
	}
}

// BenchmarkChannelPublish benchmarks Channel Publisher 的 Publish 性能
func BenchmarkChannelPublish(b *testing.B) {
	pub, _ := NewChannelEventBus(b.N + 100)
	defer pub.Close()

	event := SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Connector:  "bench-connector",
		Data: []map[string]any{
			{"hostname": "router-01"},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = pub.Publish(context.Background(), event)
	}
}

// TestKafkaPublishConcurrency 验证 Kafka Publisher 在并发场景下的正确性
func TestKafkaPublishConcurrency(t *testing.T) {
	var count atomic.Int64
	mock := &mockSyncProducer{
		sendFunc: func(msg []byte) (int32, int64, error) {
			count.Add(1)
			return 0, 1, nil
		},
	}
	pub := &kafkaPublisher{producer: mock, topic: "concurrent-topic"}

	const goroutines = 50
	const eventsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				event := SyncEvent{
					Action:     "update",
					EntityType: "Device",
					Connector:  "concurrent-conn",
					Data: []map[string]any{
						{"hostname": "concurrent-device"},
					},
				}
				if err := pub.Publish(context.Background(), event); err != nil {
					t.Errorf("Publish() error = %v", err)
				}
			}
		}()
	}

	wg.Wait()

	expected := int64(goroutines * eventsPerGoroutine)
	if got := count.Load(); got != expected {
		t.Errorf("concurrent publish count = %d, want %d", got, expected)
	}
}

// TestChannelPublishConcurrency 验证 Channel Publisher 在并发场景下的正确性
func TestChannelPublishConcurrency(t *testing.T) {
	const goroutines = 20
	const eventsPerGoroutine = 50
	totalEvents := goroutines * eventsPerGoroutine

	pub, con := NewChannelEventBus(totalEvents)
	defer pub.Close()
	defer con.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var received atomic.Int64
	done := make(chan struct{})

	go func() {
		con.Consume(ctx, func(_ context.Context, event SyncEvent) error {
			if received.Add(1) == int64(totalEvents) {
				close(done)
			}
			return nil
		})
	}()

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				event := SyncEvent{
					Action:     "update",
					EntityType: "Device",
					Connector:  "concurrent-conn",
				}
				_ = pub.Publish(ctx, event)
			}
		}()
	}

	wg.Wait()

	select {
	case <-done:
		// All events received
	case <-ctx.Done():
		t.Fatal("timeout waiting for all events to be consumed")
	}

	if got := received.Load(); got != int64(totalEvents) {
		t.Errorf("received count = %d, want %d", got, totalEvents)
	}
}
