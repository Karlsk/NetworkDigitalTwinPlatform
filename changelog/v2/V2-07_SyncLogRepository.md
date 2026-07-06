# V2-07: SyncLogRepository 同步历史 + SyncService 集成

**工时**: 1 天
**前置**: V2-05
**风险等级**: 低
**Phase**: Phase 2 — PostgreSQL 元数据存储

---

## 背景

V1 现状：同步结果仅通过 `slog.Info` 输出到日志，无法结构化查询历史同步记录。
V2 目标：每次 FullSync / IncrementalSync 完成后写入 `sync_logs` 表，支持历史查询和统计分析。

---

## 实现步骤

### Step 1: Repository 接口与实现

新建 `internal/repository/sync_log_repo.go`：

```go
package repository

import (
    "context"
    "time"
)

// SyncLogRecord 同步日志记录。
type SyncLogRecord struct {
    ID               int64
    SyncType         string    // "full" / "incremental"
    Status           string    // "success" / "failed"
    NodesCreated     int
    RelationsCreated int
    OrphanEdges      int
    Warnings         []byte    // JSON
    ErrorMessage     string
    StartedAt        time.Time
    CompletedAt      time.Time
    DurationMs       int64
}

// SyncLogRepository 同步日志 CRUD 接口。
type SyncLogRepository interface {
    Create(ctx context.Context, r SyncLogRecord) error
    List(ctx context.Context, limit int) ([]SyncLogRecord, error)
    ListByType(ctx context.Context, syncType string, limit int) ([]SyncLogRecord, error)
    Count(ctx context.Context) (int64, error)
}
```

PostgreSQL 实现 + 内存 Fallback 实现同 V2-06 模式。

### Step 2: 集成到 SyncService

修改 `internal/service/sync_service.go`：

```go
type SyncService struct {
    // ... 现有字段
    syncLogRepo repository.SyncLogRepository  // 新增，可为 nil
}

// FullSync 完成后记录同步日志
func (s *SyncService) FullSync(ctx context.Context) (*SyncResult, error) {
    start := time.Now()
    // ... 现有逻辑
    result := &SyncResult{...}

    // 记录同步日志
    if s.syncLogRepo != nil {
        s.syncLogRepo.Create(ctx, repository.SyncLogRecord{
            SyncType:         "full",
            Status:           "success",
            NodesCreated:     result.NodesCreated,
            RelationsCreated: result.RelationsCreated,
            OrphanEdges:      result.OrphanEdgesSkipped,
            StartedAt:        start,
            CompletedAt:      time.Now(),
            DurationMs:       result.Duration.Milliseconds(),
        })
    }
    return result, nil
}

// IncrementalSync 同理
```

### Step 3: 单元测试

| 测试 | 验证点 |
|------|--------|
| `TestPGSyncLogCreate` | 创建记录成功 |
| `TestPGSyncLogList` | 列表按 started_at DESC 排序 |
| `TestPGSyncLogListByType` | 按类型过滤 |
| `TestPGSyncLogCount` | 计数正确 |
| `TestSyncServiceLogsOnFullSync` | FullSync 后 repo 有记录 |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/repository/sync_log_repo.go` | 新增 | 接口 + PG + 内存实现 |
| `internal/repository/sync_log_repo_test.go` | 新增 | CRUD 单元测试 |
| `internal/service/sync_service.go` | 修改 | FullSync/IncrementalSync 记录日志 |

---

## 验收标准

- [ ] SyncLogRepository 接口定义完整
- [ ] PostgreSQL + 内存实现 CRUD 正确
- [ ] FullSync 完成后 sync_logs 表有记录
- [ ] IncrementalSync 完成后 sync_logs 表有记录
- [ ] `go test ./internal/repository/... ./internal/service/...` 全部通过
