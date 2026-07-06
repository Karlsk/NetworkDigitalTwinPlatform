// Package events Fallback Publisher 单元测试（TDD RED 阶段）
package events

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mock: mockPingPublisher（实现 EventPublisher + pinger 接口）
// ---------------------------------------------------------------------------

type mockPingPublisher struct {
	mu          sync.Mutex
	publishFunc func(ctx context.Context, event SyncEvent) error
	closeFunc   func() error
	pingFunc    func() error
	publishCalls int
	closeCalls   int
	closed       bool
}

func (m *mockPingPublisher) Publish(ctx context.Context, event SyncEvent) error {
	m.mu.Lock()
	m.publishCalls++
	m.mu.Unlock()
	if m.publishFunc != nil {
		return m.publishFunc(ctx, event)
	}
	return nil
}

func (m *mockPingPublisher) Close() error {
	m.mu.Lock()
	m.closeCalls++
	m.closed = true
	m.mu.Unlock()
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockPingPublisher) Ping() error {
	if m.pingFunc != nil {
		return m.pingFunc()
	}
	return nil
}

// ---------------------------------------------------------------------------
// 测试用例
// ---------------------------------------------------------------------------

// TestFallbackPrimarySuccess 验证 primary 正常时使用 primary，fallback 不被调用。
func TestFallbackPrimarySuccess(t *testing.T) {
	primary := &mockPingPublisher{}
	fallback := &mockPingPublisher{}

	pub := NewFallbackPublisher(primary, fallback, time.Second)

	event := SyncEvent{Action: "update", EntityType: "Device", Connector: "mock"}
	err := pub.Publish(context.Background(), event)
	if err != nil {
		t.Fatalf("Publish() error = %v, want nil", err)
	}

	primary.mu.Lock()
	if primary.publishCalls != 1 {
		t.Errorf("primary publishCalls = %d, want 1", primary.publishCalls)
	}
	primary.mu.Unlock()

	fallback.mu.Lock()
	if fallback.publishCalls != 0 {
		t.Errorf("fallback publishCalls = %d, want 0", fallback.publishCalls)
	}
	fallback.mu.Unlock()
}

// TestFallbackPrimaryFails 验证 primary 失败时自动切换到 fallback。
func TestFallbackPrimaryFails(t *testing.T) {
	primary := &mockPingPublisher{
		publishFunc: func(_ context.Context, _ SyncEvent) error {
			return errors.New("kafka broker unavailable")
		},
		pingFunc: func() error {
			return errors.New("still down") // 不恢复
		},
	}
	fallback := &mockPingPublisher{}

	pub := NewFallbackPublisher(primary, fallback, time.Second)

	event := SyncEvent{Action: "update", EntityType: "Device"}
	err := pub.Publish(context.Background(), event)
	if err != nil {
		t.Fatalf("Publish() error = %v, want nil (fallback should succeed)", err)
	}

	// primary 被调用一次（失败），fallback 被调用一次
	primary.mu.Lock()
	if primary.publishCalls != 1 {
		t.Errorf("primary publishCalls = %d, want 1", primary.publishCalls)
	}
	primary.mu.Unlock()

	fallback.mu.Lock()
	if fallback.publishCalls != 1 {
		t.Errorf("fallback publishCalls = %d, want 1", fallback.publishCalls)
	}
	fallback.mu.Unlock()

	// 再次 Publish 应该直接走 fallback（primaryOK=false）
	event2 := SyncEvent{Action: "delete", EntityType: "Link"}
	err = pub.Publish(context.Background(), event2)
	if err != nil {
		t.Fatalf("second Publish() error = %v", err)
	}

	fallback.mu.Lock()
	if fallback.publishCalls != 2 {
		t.Errorf("fallback publishCalls after 2nd call = %d, want 2", fallback.publishCalls)
	}
	fallback.mu.Unlock()
}

// TestFallbackRecover 验证 primary 先失败，Ping 恢复后自动切回 primary。
func TestFallbackRecover(t *testing.T) {
	var primaryDown atomic.Bool
	primaryDown.Store(true)

	primary := &mockPingPublisher{
		publishFunc: func(_ context.Context, _ SyncEvent) error {
			if primaryDown.Load() {
				return errors.New("kafka down")
			}
			return nil
		},
		pingFunc: func() error {
			if primaryDown.Load() {
				return errors.New("still down")
			}
			return nil // 恢复
		},
	}
	fallback := &mockPingPublisher{}

	pub := NewFallbackPublisher(primary, fallback, 100*time.Millisecond)

	// 第一次调用：primary 失败，触发 fallback + tryRecover goroutine
	err := pub.Publish(context.Background(), SyncEvent{Action: "update"})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	// 等 tryRecover 的第一次 ticker 触发（此时 Ping 仍失败）
	time.Sleep(150 * time.Millisecond)

	// 模拟 primary 恢复
	primaryDown.Store(false)

	// 等 tryRecover 的第二次 ticker 触发（Ping 成功，恢复 primaryOK）
	time.Sleep(150 * time.Millisecond)

	// 重置计数
	primary.mu.Lock()
	primary.publishCalls = 0
	primary.mu.Unlock()
	fallback.mu.Lock()
	fallback.publishCalls = 0
	fallback.mu.Unlock()

	// 再次 Publish：应该走 primary（已恢复）
	err = pub.Publish(context.Background(), SyncEvent{Action: "update"})
	if err != nil {
		t.Fatalf("Publish() after recovery error = %v", err)
	}

	primary.mu.Lock()
	if primary.publishCalls != 1 {
		t.Errorf("primary publishCalls after recovery = %d, want 1", primary.publishCalls)
	}
	primary.mu.Unlock()

	fallback.mu.Lock()
	if fallback.publishCalls != 0 {
		t.Errorf("fallback publishCalls after recovery = %d, want 0", fallback.publishCalls)
	}
	fallback.mu.Unlock()
}

// TestFallbackClose 验证 Close 同时关闭 primary 和 fallback，任一失败返回 error。
func TestFallbackClose(t *testing.T) {
	t.Run("both succeed", func(t *testing.T) {
		primary := &mockPingPublisher{}
		fallback := &mockPingPublisher{}
		pub := NewFallbackPublisher(primary, fallback, time.Second)

		err := pub.Close()
		if err != nil {
			t.Fatalf("Close() error = %v, want nil", err)
		}

		primary.mu.Lock()
		if !primary.closed {
			t.Error("primary not closed")
		}
		primary.mu.Unlock()

		fallback.mu.Lock()
		if !fallback.closed {
			t.Error("fallback not closed")
		}
		fallback.mu.Unlock()
	})

	t.Run("primary close fails", func(t *testing.T) {
		primary := &mockPingPublisher{
			closeFunc: func() error { return errors.New("primary close error") },
		}
		fallback := &mockPingPublisher{}
		pub := NewFallbackPublisher(primary, fallback, time.Second)

		err := pub.Close()
		if err == nil {
			t.Fatal("Close() should return error when primary close fails")
		}
	})

	t.Run("fallback close fails", func(t *testing.T) {
		primary := &mockPingPublisher{}
		fallback := &mockPingPublisher{
			closeFunc: func() error { return errors.New("fallback close error") },
		}
		pub := NewFallbackPublisher(primary, fallback, time.Second)

		err := pub.Close()
		if err == nil {
			t.Fatal("Close() should return error when fallback close fails")
		}
	})
}
