// Package snapshot 实现快照管理。
package snapshot

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/repository"
)

// AuditEntry 审计日志条目。
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`           // "create", "restore", "delete", "diff"
	Snapshot  string    `json:"snapshot"`          // 快照名称
	Actor     string    `json:"actor"`             // 操作来源 ("mcp", "webhook", "system")
	Detail    string    `json:"detail"`            // 详情 (如 "nodes=21, rels=30")
	Error     string    `json:"error,omitempty"`   // 如果有错误
}

// AuditFilter 审计查询过滤器。
type AuditFilter struct {
	Action   string    // 按操作类型过滤（空表示不过滤）
	Snapshot string    // 按快照名称过滤
	Since    time.Time // 按时间过滤（零值表示不过滤）
	Until    time.Time // 按时间过滤（零值表示不过滤）
}

// AuditLog 审计日志（内存 FIFO，maxEntries 淘汰）。
// V2-09: 支持可选的 PG 持久化，双写模式：内存同步写 + PG 异步写。
type AuditLog struct {
	entries    []AuditEntry
	mu         sync.RWMutex
	maxEntries int
	repo       repository.AuditLogRepository // V2-09: 可选，nil 时仅内存模式
}

// NewAuditLog 创建审计日志实例。
func NewAuditLog(maxEntries int) *AuditLog {
	return &AuditLog{
		entries:    make([]AuditEntry, 0),
		maxEntries: maxEntries,
	}
}

// SetRepository 设置持久化 Repository，启用后 Record 会异步写入 PG。
// V2-09: 由 SnapshotManager.WithAuditRepository Option 调用。
func (l *AuditLog) SetRepository(repo repository.AuditLogRepository) {
	l.repo = repo
}

// Record 记录一条审计日志。自动设置 Timestamp，超出 maxEntries 时 FIFO 淘汰旧条目。
// V2-09: 先同步写内存 FIFO，再异步写 PG（不阻塞主流程）。
func (l *AuditLog) Record(entry AuditEntry) {
	// 1. 内存 FIFO（同步，快速查询）
	l.mu.Lock()
	entry.Timestamp = time.Now()
	l.entries = append(l.entries, entry)
	if len(l.entries) > l.maxEntries {
		l.entries = l.entries[len(l.entries)-l.maxEntries:]
	}
	l.mu.Unlock()

	// 2. PostgreSQL 持久化（异步，不阻塞主流程）
	if l.repo != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := l.repo.Create(ctx, toAuditRecord(entry)); err != nil {
				slog.Error("persist audit log failed", "action", entry.Action, "snapshot", entry.Snapshot, "error", err)
			}
		}()
	}
}

// toAuditRecord 将 AuditEntry 转换为 repository.AuditLogRecord。
func toAuditRecord(e AuditEntry) repository.AuditLogRecord {
	return repository.AuditLogRecord{
		Timestamp: e.Timestamp,
		Action:    e.Action,
		Snapshot:  e.Snapshot,
		Actor:     e.Actor,
		Detail:    e.Detail,
		Error:     e.Error,
	}
}

// Query 按过滤条件查询审计日志。
func (l *AuditLog) Query(filter AuditFilter) []AuditEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []AuditEntry
	for _, e := range l.entries {
		if filter.Action != "" && e.Action != filter.Action {
			continue
		}
		if filter.Snapshot != "" && e.Snapshot != filter.Snapshot {
			continue
		}
		if !filter.Since.IsZero() && e.Timestamp.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && e.Timestamp.After(filter.Until) {
			continue
		}
		result = append(result, e)
	}
	return result
}

// Recent 返回最近 n 条审计日志。
func (l *AuditLog) Recent(n int) []AuditEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if n <= 0 || len(l.entries) == 0 {
		return nil
	}
	if n > len(l.entries) {
		n = len(l.entries)
	}
	// 返回最新的 n 条（尾部 n 条）
	start := len(l.entries) - n
	result := make([]AuditEntry, n)
	copy(result, l.entries[start:])
	return result
}

// errStr 将 error 转为字符串，nil 返回空串。
func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
