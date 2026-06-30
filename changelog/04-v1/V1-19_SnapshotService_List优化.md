# V1-19: SnapshotService List 优化

**工时**: 0.5 天
**前置**: V1-17
**风险等级**: 低
**Phase**: Phase 3 — 缓存审计 + 验收

---

## 背景

V1-17 在 SnapshotManager 层实现了 MetaCache。本任务确保 Service 层正确适配，并暴露 AuditLog 查询能力给 MCP 层。

---

## 实现内容

### 1. snapshot_service.go — 适配 MetaCache

`List()` 直接调用 `manager.List()` — MetaCache 在 Manager 层透明工作，Service 层无需额外逻辑。

### 2. 新增 AuditLog 方法

```go
// AuditLog 查询审计日志。
func (s *SnapshotService) AuditLog(filter snapshot.AuditFilter) []snapshot.AuditEntry {
    return s.manager.AuditLog().Query(filter)
}

// RecentAudit 获取最近 N 条审计记录。
func (s *SnapshotService) RecentAudit(n int) []snapshot.AuditEntry {
    return s.manager.AuditLog().Recent(n)
}
```

### 3. SnapshotManager 暴露 AuditLog

```go
func (m *SnapshotManager) AuditLog() *AuditLog {
    return m.auditLog
}
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/service/snapshot_service.go` | 修改 | 新增 AuditLog/RecentAudit 方法 |
| `internal/snapshot/manager.go` | 修改 | 暴露 AuditLog() getter |
| `internal/mcp/tools.go` | 修改 | 可选: MCP 工具调用审计查询 |

---

## 验收标准

- [ ] 编译通过
- [ ] MCP `query_snapshot list` 第二次调用明显快于首次（MetaCache 生效）
- [ ] AuditLog 可通过 Service 层查询
- [ ] `SnapshotService.AuditLog(filter)` 返回正确结果
- [ ] `go test ./internal/service/...` 全部通过
