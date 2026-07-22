// Package events 事件总线单元测试
package events

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestChannelPublishConsume 验证 Publish -> Consume 全链路，消息顺序正确且字段完整（含 Relations）。
func TestChannelPublishConsume(t *testing.T) {
	pub, con := NewChannelEventBus(10)
	defer pub.Close()
	defer con.Close()

	// 准备测试事件（含 Relations 字段）
	events := []SyncEvent{
		{
			Action:     "update",
			EntityType: "Device",
			Connector:  "mock-netbox",
			Data: []map[string]any{
				{"hostname": "router-01", "vendor": "Huawei"},
			},
		},
		{
			Action:     "delete",
			EntityType: "Interface",
			Connector:  "mock-cmdb",
			URIs:       []string{"iface:SN001_eth0", "iface:SN002_eth0"},
		},
		{
			Action:     "delete_relation",
			EntityType: "Device",
			Connector:  "mock-netbox",
			Relations: []Relation{
				{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
				{Type: "CONNECTS_TO", From: "iface:SN001_eth0", To: "iface:SN002_eth0"},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Publish 所有事件
	for _, e := range events {
		if err := pub.Publish(ctx, e); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	}

	// Consume 收集事件
	var received []SyncEvent
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		con.Consume(ctx, func(_ context.Context, event SyncEvent) error {
			mu.Lock()
			received = append(received, event)
			count := len(received)
			mu.Unlock()

			if count == len(events) {
				close(done)
			}
			return nil
		})
	}()

	// 等待所有事件消费完成
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Consume timeout: not all events received")
	}

	// 验证消息数量和顺序
	mu.Lock()
	defer mu.Unlock()

	if len(received) != len(events) {
		t.Fatalf("received count = %d, want %d", len(received), len(events))
	}

	for i, want := range events {
		got := received[i]
		if got.Action != want.Action {
			t.Errorf("received[%d].Action = %q, want %q", i, got.Action, want.Action)
		}
		if got.EntityType != want.EntityType {
			t.Errorf("received[%d].EntityType = %q, want %q", i, got.EntityType, want.EntityType)
		}
		if got.Connector != want.Connector {
			t.Errorf("received[%d].Connector = %q, want %q", i, got.Connector, want.Connector)
		}

		// 验证 Relations 字段
		if len(got.Relations) != len(want.Relations) {
			t.Errorf("received[%d].Relations count = %d, want %d", i, len(got.Relations), len(want.Relations))
			continue
		}
		for j, wantRel := range want.Relations {
			gotRel := got.Relations[j]
			if gotRel.Type != wantRel.Type {
				t.Errorf("received[%d].Relations[%d].Type = %q, want %q", i, j, gotRel.Type, wantRel.Type)
			}
			if gotRel.From != wantRel.From {
				t.Errorf("received[%d].Relations[%d].From = %q, want %q", i, j, gotRel.From, wantRel.From)
			}
			if gotRel.To != wantRel.To {
				t.Errorf("received[%d].Relations[%d].To = %q, want %q", i, j, gotRel.To, wantRel.To)
			}
		}
	}
}

// TestChannelPublishFull 验证缓冲区满时 Publish 返回 error。
func TestChannelPublishFull(t *testing.T) {
	pub, con := NewChannelEventBus(1)
	defer pub.Close()
	defer con.Close()

	ctx := context.Background()

	// 填满缓冲区
	if err := pub.Publish(ctx, SyncEvent{Action: "update"}); err != nil {
		t.Fatalf("first Publish() error = %v, want nil", err)
	}

	// 缓冲区已满，应返回 error
	err := pub.Publish(ctx, SyncEvent{Action: "delete"})
	if err == nil {
		t.Fatal("Publish() should return error when buffer is full")
	}
}

// TestChannelConsumeCancel 验证 ctx 取消后 Consume 返回 ctx.Err()。
func TestChannelConsumeCancel(t *testing.T) {
	pub, con := NewChannelEventBus(10)
	defer pub.Close()
	defer con.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- con.Consume(ctx, func(_ context.Context, event SyncEvent) error {
			return nil
		})
	}()

	// 取消 context
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Consume() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Consume() did not return after ctx cancel")
	}
}
