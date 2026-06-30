# V1-18: AuditLog 审计日志

**工时**: 1 天
**前置**: V1-17
**风险等级**: 低
**Phase**: Phase 3 — 缓存审计 + 验收

---

## 背景

TD-03 要求为快照操作添加审计能力。当前只有 `slog.Info` 级别日志，无法追溯操作历史。需要结构化审计记录，跟踪 Create/Restore/Delete/Diff 操作。

---

## 实现内容

### 1. snapshot/audit.go (新文件)

```go
// AuditEntry 审计日志条目。
type AuditEntry struct {
    Timestamp time.Time `json:"timestamp"`
    Action    string    `json:"action"`    // "create", "restore", "delete", "diff"
    Snapshot  string    `json:"snapshot"`  // 快照名称
    Actor     string    `json:"actor"`     // 操作来源 ("mcp", "webhook", "system")
    Detail    string    `json:"detail"`    // 详情 (如 "nodes=21, rels=30")
    Error     string    `json:"error,omitempty"` // 如果有错误
}

// AuditFilter 审计查询过滤器。
type AuditFilter struct {
    Action   string    // 按操作类型过滤（空表示不过滤）
    Snapshot string    // 按快照名称过滤
    Since    time.Time // 按时间过滤（零值表示不过滤）
    Until    time.Time // 按时间过滤（零值表示不过滤）
}

// AuditLog 审计日志（内存 FIFO，maxEntries 淘汰）。
type AuditLog struct {
    entries    []AuditEntry
    mu         sync.RWMutex
    maxEntries int
}

func NewAuditLog(maxEntries int) *AuditLog
func (l *AuditLog) Record(entry AuditEntry)
func (l *AuditLog) Query(filter AuditFilter) []AuditEntry
func (l *AuditLog) Recent(n int) []AuditEntry
```

**Record 实现**:

```go
func (l *AuditLog) Record(entry AuditEntry) {
    l.mu.Lock()
    defer l.mu.Unlock()

    entry.Timestamp = time.Now()
    l.entries = append(l.entries, entry)

    // FIFO 淘汰
    if len(l.entries) > l.maxEntries {
        l.entries = l.entries[len(l.entries)-l.maxEntries:]
    }
}
```

**Query 实现**:

```go
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
```

### 2. SnapshotManager 集成

```go
type SnapshotManager struct {
    // ... 现有字段
    auditLog *AuditLog  // 新增
}

// 在 Create/Restore/Delete 操作中调用 audit.Record()
func (m *SnapshotManager) Create(ctx context.Context, name string) (SnapshotMeta, error) {
    meta, err := m.doCreate(ctx, name)
    m.auditLog.Record(AuditEntry{
        Action:   "create",
        Snapshot: name,
        Actor:    "system",
        Detail:   fmt.Sprintf("nodes=%d, rels=%d", meta.NodeCount, meta.RelationCount),
        Error:    errStr(err),
    })
    return meta, err
}
```

### 3. MCP 层可选暴露

MCP `query_snapshot` 工具新增 `action=audit` 参数查看审计日志。

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/snapshot/audit.go` | 新建 | AuditEntry/AuditFilter/AuditLog |
| `internal/snapshot/audit_test.go` | 新建 | 审计日志测试 |
| `internal/snapshot/manager.go` | 修改 | 集成 AuditLog，Create/Restore/Delete 记录审计 |
| `internal/mcp/tools.go` | 修改 | 可选: audit action |

---

## 验收标准

- [ ] 编译通过
- [ ] Create/Restore/Delete 操作均记录审计日志
- [ ] 审计日志按时间序排列
- [ ] 超出 maxEntries 时正确 FIFO 淘汰旧条目
- [ ] 按 Action/Snapshot/时间范围过滤查询正确
- [ ] `Recent(n)` 返回最近 N 条
- [ ] `go test -race ./internal/snapshot/...` 全部通过
