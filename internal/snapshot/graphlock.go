// Package snapshot 实现快照管理。
package snapshot

import "sync"

// GraphLock 图数据库并发保护锁。
// Restore/FullSync/IncrementalSync 持有写锁，阻塞其他写操作。
// MCP Query/Snapshot.Create 持有读锁，允许并发读。
// SyncService 和 SnapshotManager 必须共享同一个 GraphLock 实例。
type GraphLock struct {
	mu sync.RWMutex
}

// NewGraphLock 创建新的 GraphLock 实例。
func NewGraphLock() *GraphLock {
	return &GraphLock{}
}

// Lock 获取排他写锁（Restore/FullSync/IncrementalSync 使用）。
// 使用方应通过 defer Unlock() 确保释放。
func (l *GraphLock) Lock() { l.mu.Lock() }

// Unlock 释放排他写锁。
func (l *GraphLock) Unlock() { l.mu.Unlock() }

// RLock 获取共享读锁（MCP Query/Snapshot.Create 使用）。
// 多个读锁可同时持有，但会阻塞写锁。
// 使用方应通过 defer RUnlock() 确保释放。
func (l *GraphLock) RLock() { l.mu.RLock() }

// RUnlock 释放共享读锁。
func (l *GraphLock) RUnlock() { l.mu.RUnlock() }
