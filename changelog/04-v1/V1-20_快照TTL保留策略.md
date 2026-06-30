# V1-20: 快照 TTL/保留策略 (可选)

**工时**: 0.5 天
**前置**: V1-17
**风险等级**: 低
**Phase**: Phase 3 — 缓存审计 + 验收

> **注意**: 本任务为可选项，根据实际需求决定是否实施。

---

## 背景

当前快照 YAML 文件永不过期，长期运行后可能积累大量快照。V1 可选引入 TTL 保留策略，自动清理超期快照。

---

## 实现内容

### 1. SnapshotManager 新增字段

```go
type SnapshotManager struct {
    // ... 现有字段
    retentionDays int  // 新增: 0 = 不自动清理
}
```

### 2. cleanupExpired

```go
func (m *SnapshotManager) cleanupExpired(ctx context.Context) {
    if m.retentionDays <= 0 {
        return
    }
    cutoff := time.Now().AddDate(0, 0, -m.retentionDays)

    m.cacheMu.RLock()
    var expired []string
    for name, meta := range m.metaCache {
        if meta.CreatedAt.Before(cutoff) {
            expired = append(expired, name)
        }
    }
    m.cacheMu.RUnlock()

    for _, name := range expired {
        if err := m.Delete(ctx, name); err != nil {
            slog.Warn("cleanup expired snapshot failed", "snapshot", name, "error", err)
        } else {
            slog.Info("cleaned up expired snapshot", "snapshot", name)
            m.auditLog.Record(AuditEntry{
                Action:   "auto_delete",
                Snapshot: name,
                Actor:    "system",
                Detail:   "expired TTL cleanup",
            })
        }
    }
}
```

### 3. 在 Create 后触发

```go
func (m *SnapshotManager) Create(ctx context.Context, name string) (SnapshotMeta, error) {
    meta, err := m.doCreate(ctx, name)
    // ... 缓存更新、审计记录
    m.cleanupExpired(ctx)  // Create 后触发过期清理
    return meta, err
}
```

### 4. configs/config.yaml

```yaml
snapshot:
  retention_days: 30  # 0 = 不自动清理
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/snapshot/manager.go` | 修改 | retentionDays + cleanupExpired |
| `internal/config/config.go` | 修改 | 新增 snapshot.retention_days 配置 |
| `configs/config.yaml` | 修改 | 新增 snapshot.retention_days |

---

## 验收标准

- [ ] 编译通过
- [ ] `retentionDays > 0` 时，超期快照在 Create 后被自动清理
- [ ] `retentionDays = 0` 时不自动清理
- [ ] 审计日志记录自动清理事件（action: "auto_delete"）
- [ ] `go test ./internal/snapshot/...` 全部通过
