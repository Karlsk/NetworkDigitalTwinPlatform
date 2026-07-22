package snapshot

import (
	"sync"
	"testing"
	"time"
)

// TestNewGraphLock 验证 GraphLock 构造函数返回非 nil 指针。
func TestNewGraphLock(t *testing.T) {
	lock := NewGraphLock()
	if lock == nil {
		t.Fatal("NewGraphLock() returned nil")
	}
}

// TestGraphLock_WriteLockMutualExclusion 验证写锁互斥：
// goroutine A 持有 Lock → B 的 Lock 阻塞直到 A 释放。
func TestGraphLock_WriteLockMutualExclusion(t *testing.T) {
	lock := NewGraphLock()

	held := make(chan struct{}) // A 持有锁后通知
	done := make(chan struct{}) // B 获取到锁后通知

	// goroutine A: 获取锁 → 通知 → sleep 50ms → 释放锁
	go func() {
		lock.Lock()
		close(held)
		time.Sleep(50 * time.Millisecond)
		lock.Unlock()
	}()

	// 等待 A 持有锁
	<-held

	// goroutine B: 尝试获取锁
	go func() {
		lock.Lock()
		close(done)
		lock.Unlock()
	}()

	// 验证 B 在 A 释放前被阻塞（30ms < 50ms）
	select {
	case <-done:
		t.Fatal("B should block while A holds the lock")
	case <-time.After(30 * time.Millisecond):
		// 预期：B 被阻塞
	}

	// 等待 B 最终获取到锁（A 释放后）
	select {
	case <-done:
		// 成功：B 在 A 释放后获取到锁
	case <-time.After(100 * time.Millisecond):
		t.Fatal("B should acquire lock after A releases")
	}
}

// TestGraphLock_ReadLockSharing 验证读锁共享：
// 多个 goroutine 可同时持有 RLock。
func TestGraphLock_ReadLockSharing(t *testing.T) {
	lock := NewGraphLock()
	const N = 5

	acquired := make(chan int, N)
	var wg sync.WaitGroup

	// 启动 N 个 goroutine，每个获取 RLock 后发送自身序号
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lock.RLock()
			acquired <- id
			// 不立即释放，等待所有 goroutine 都获取到锁
		}(i)
	}

	// 收集所有 N 个值（超时 1 秒）
	received := make(map[int]bool)
	timeout := time.After(1 * time.Second)
	for len(received) < N {
		select {
		case id := <-acquired:
			received[id] = true
		case <-timeout:
			t.Fatalf("ReadLock sharing failed: only %d/%d goroutines acquired", len(received), N)
		}
	}

	// 全部成功：读锁可共享
	t.Logf("All %d goroutines acquired RLock simultaneously", N)

	// 清理：逐一释放（需要等待所有 goroutine 完成）
	wg.Wait()
	for i := 0; i < N; i++ {
		lock.RUnlock()
	}
}

// TestGraphLock_ReadWriteExclusion 验证读写互斥：
// RLock 持有期间 Lock 阻塞，RUnlock 后 Lock 获得。
func TestGraphLock_ReadWriteExclusion(t *testing.T) {
	lock := NewGraphLock()

	held := make(chan struct{}) // A 持有读锁后通知
	done := make(chan struct{}) // B 获取到写锁后通知

	// goroutine A: 获取读锁 → 通知 → sleep 50ms → 释放读锁
	go func() {
		lock.RLock()
		close(held)
		time.Sleep(50 * time.Millisecond)
		lock.RUnlock()
	}()

	// 等待 A 持有读锁
	<-held

	// goroutine B: 尝试获取写锁
	go func() {
		lock.Lock()
		close(done)
		lock.Unlock()
	}()

	// 验证 B 在 A 释放读锁前被阻塞
	select {
	case <-done:
		t.Fatal("Write lock should block while read lock is held")
	case <-time.After(30 * time.Millisecond):
		// 预期：B 被阻塞
	}

	// 等待 B 最终获取到写锁
	select {
	case <-done:
		// 成功：B 在 A 释放读锁后获取到写锁
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Write lock should acquire after read lock releases")
	}
}

// TestGraphLock_SharedInstance 验证共享实例：
// 同一 GraphLock 指针赋值给两个变量，模拟 SyncService 和 SnapshotManager 共享同一把锁。
func TestGraphLock_SharedInstance(t *testing.T) {
	// 创建单个 GraphLock 实例
	lock := NewGraphLock()

	// 模拟注入到两个 service（共享同一指针）
	serviceA := lock
	serviceB := lock

	// serviceA 获取写锁
	serviceA.Lock()

	// serviceB 尝试获取读锁应阻塞（因为同一实例的写锁已持有）
	done := make(chan struct{})
	go func() {
		serviceB.RLock()
		close(done)
		serviceB.RUnlock()
	}()

	// 验证 B 被阻塞
	select {
	case <-done:
		t.Fatal("RLock should block while Lock is held on the same instance")
	case <-time.After(30 * time.Millisecond):
		// 预期：B 被阻塞
	}

	// 释放 A 的写锁
	serviceA.Unlock()

	// 等待 B 获取到读锁
	select {
	case <-done:
		// 成功：共享实例验证通过
	case <-time.After(100 * time.Millisecond):
		t.Fatal("RLock should acquire after Lock releases on the same instance")
	}
}
