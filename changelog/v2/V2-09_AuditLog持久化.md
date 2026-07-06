# V2-09: AuditLog 从内存 FIFO 迁移到 PostgreSQL

**工时**: 1 天
**前置**: V2-05
**风险等级**: 中
**Phase**: Phase 2 — PostgreSQL 元数据存储

---

## 背景

V1 现状：`AuditLog` 使用内存 FIFO 队列（`maxEntries=1000`），进程重启后全部审计记录丢失。
V2 目标：审计日志持久化到 PostgreSQL `audit_logs` 表，进程重启后历史记录完整保留。

### V1 现状代码

```go
// internal/snapshot/audit.go (V1)
type AuditLog struct {
    entries    []AuditEntry
    mu         sync.RWMutex
    maxEntries int  // FIFO 淘汰
}
```

---

## 实现步骤

### Step 1: AuditLogRepository 接口

新建 `internal/repository/audit_repo.go`：

```go
package repository

// AuditLogRecord 审计日志数据库记录。
type AuditLogRecord struct {
    ID        int64
    Timestamp time.Time
    Action    string
    Snapshot  string
    Actor     string
    Detail    string
    Error     string
}

// AuditLogRepository 审计日志 CRUD。
type AuditLogRepository interface {
    Create(ctx context.Context, r AuditLogRecord) error
    List(ctx context.Context, limit int) ([]AuditLogRecord, error)
    Query(ctx context.Context, filter AuditFilter) ([]AuditLogRecord, error)
    Count(ctx context.Context) (int64, error)
}

// AuditFilter 审计查询过滤器。
type AuditFilter struct {
    Action   string
    Snapshot string
    Since    time.Time
    Until    time.Time
}
```

### Step 2: PostgreSQL 实现

```go
type pgAuditLogRepo struct {
    pool *pgxpool.Pool
}

func (r *pgAuditLogRepo) Create(ctx context.Context, rec AuditLogRecord) error {
    _, err := r.pool.Exec(ctx,
        `INSERT INTO audit_logs (timestamp, action, snapshot, actor, detail, error)
         VALUES ($1, $2, $3, $4, $5, $6)`,
        rec.Timestamp, rec.Action, rec.Snapshot, rec.Actor, rec.Detail, rec.Error)
    return err
}

func (r *pgAuditLogRepo) Query(ctx context.Context, f AuditFilter) ([]AuditLogRecord, error) {
    // 动态构建 WHERE 条件
    query := `SELECT id, timestamp, action, snapshot, actor, detail, error FROM audit_logs WHERE 1=1`
    args := []any{}
    argIdx := 1
    if f.Action != "" {
        query += fmt.Sprintf(" AND action = $%d", argIdx)
        args = append(args, f.Action)
        argIdx++
    }
    // ... 其他过滤条件
    query += " ORDER BY timestamp DESC LIMIT 1000"
    // ... scan rows
}
```

### Step 3: 改造 AuditLog 支持双写

修改 `internal/snapshot/audit.go`：

```go
type AuditLog struct {
    // ... 现有字段（保留作为 Fallback）
    repo repository.AuditLogRepository  // 新增，可为 nil
}

// SetRepository 设置持久化 Repository。
func (l *AuditLog) SetRepository(repo repository.AuditLogRepository) {
    l.repo = repo
}

// Record 双写：内存 + PostgreSQL。
func (l *AuditLog) Record(entry AuditEntry) {
    // 1. 内存 FIFO（快速查询）
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
                slog.Error("persist audit log failed", "error", err)
            }
        }()
    }
}
```

### Step 4: SnapshotManager 集成

```go
// NewSnapshotManager 接收可选的 AuditLogRepository
func NewSnapshotManager(g graph.GraphDB, lock *GraphLock, snapDir string, maxActive int, opts ...Option) *SnapshotManager {
    sm := &SnapshotManager{...}
    for _, opt := range opts {
        opt(sm)
    }
    return sm
}

// Option 函数模式
type Option func(*SnapshotManager)

func WithAuditRepository(repo repository.AuditLogRepository) Option {
    return func(sm *SnapshotManager) {
        sm.auditLog.SetRepository(repo)
    }
}
```

### Step 5: 单元测试

| 测试 | 验证点 |
|------|--------|
| `TestPGAuditLogCreate` | 创建审计记录成功 |
| `TestPGAuditLogQuery` | 按 Action/Snapshot/时间过滤 |
| `TestAuditLogDualWrite` | Record 同时写入内存和 PG |
| `TestAuditLogPGFailure` | PG 失败时内存记录不丢失 |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/repository/audit_repo.go` | 新增 | 接口 + PG 实现 |
| `internal/repository/audit_repo_test.go` | 新增 | CRUD + 过滤测试 |
| `internal/snapshot/audit.go` | 修改 | 双写：内存 + PG |
| `internal/snapshot/manager.go` | 修改 | Option 模式注入 Repository |

---

## 验收标准

- [ ] AuditLogRepository 接口定义完整
- [ ] PostgreSQL 实现 CRUD + 过滤正确
- [ ] AuditLog.Record 双写成功（内存 + PG）
- [ ] PG 写入失败时内存记录不丢失（异步写，不阻塞）
- [ ] 进程重启后审计日志从 PostgreSQL 可查
- [ ] `go test ./internal/repository/... ./internal/snapshot/...` 全部通过
