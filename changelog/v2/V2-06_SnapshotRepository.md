# V2-06: SnapshotRepository 快照元数据 CRUD

**工时**: 1.5 天
**前置**: V2-05
**风险等级**: 低
**Phase**: Phase 2 — PostgreSQL 元数据存储

---

## 背景

V1 现状：快照元数据存于 `SnapshotManager.metaCache`（内存 map）+ YAML 文件头。
进程重启时需 `warmCache` 扫描所有 YAML 文件重建缓存，100 个快照时启动较慢。

V2 目标：快照元数据持久化到 PostgreSQL `snapshots` 表，替代文件扫描。

---

## 实现步骤

### Step 1: Repository 接口

新建 `internal/repository/snapshot_repo.go`：

```go
package repository

import (
    "context"
    "time"
)

// SnapshotRecord 快照数据库记录。
type SnapshotRecord struct {
    ID        int64
    Name      string
    CreatedAt time.Time
    NodeCount int
    RelCount  int
    FilePath  string
    Status    string
}

// SnapshotRepository 快照元数据 CRUD 接口。
type SnapshotRepository interface {
    Create(ctx context.Context, r SnapshotRecord) error
    GetByName(ctx context.Context, name string) (*SnapshotRecord, error)
    List(ctx context.Context) ([]SnapshotRecord, error)
    Delete(ctx context.Context, name string) error
    UpdateStatus(ctx context.Context, name, status string) error
}
```

### Step 2: PostgreSQL 实现

```go
type pgSnapshotRepo struct {
    pool *pgxpool.Pool
}

func NewPGSnapshotRepository(pool *pgxpool.Pool) SnapshotRepository {
    return &pgSnapshotRepo{pool: pool}
}

func (r *pgSnapshotRepo) Create(ctx context.Context, rec SnapshotRecord) error {
    _, err := r.pool.Exec(ctx,
        `INSERT INTO snapshots (name, created_at, node_count, rel_count, file_path, status)
         VALUES ($1, $2, $3, $4, $5, $6)`,
        rec.Name, rec.CreatedAt, rec.NodeCount, rec.RelCount, rec.FilePath, rec.Status)
    return err
}

func (r *pgSnapshotRepo) List(ctx context.Context) ([]SnapshotRecord, error) {
    rows, err := r.pool.Query(ctx,
        `SELECT id, name, created_at, node_count, rel_count, file_path, status
         FROM snapshots ORDER BY created_at DESC`)
    // ... scan rows
}

func (r *pgSnapshotRepo) GetByName(ctx context.Context, name string) (*SnapshotRecord, error) {
    // SELECT ... WHERE name = $1
}

func (r *pgSnapshotRepo) Delete(ctx context.Context, name string) error {
    // DELETE FROM snapshots WHERE name = $1
}

func (r *pgSnapshotRepo) UpdateStatus(ctx context.Context, name, status string) error {
    // UPDATE snapshots SET status = $2 WHERE name = $1
}
```

### Step 3: 内存 Fallback 实现

```go
// memSnapshotRepository 内存实现（postgres.enabled: false 时使用）。
type memSnapshotRepository struct {
    records map[string]SnapshotRecord
    mu      sync.RWMutex
}

func NewMemSnapshotRepository() SnapshotRepository {
    return &memSnapshotRepository{records: make(map[string]SnapshotRecord)}
}
// ... CRUD 实现
```

### Step 4: 集成到 SnapshotManager

修改 `internal/snapshot/manager.go`：

```go
type SnapshotManager struct {
    // ... 现有字段
    repo repository.SnapshotRepository  // 新增
}

// Create 完成后写入 Repository
func (sm *SnapshotManager) Create(ctx context.Context, name string) (SnapshotMeta, error) {
    // ... 现有逻辑
    meta, err := exportToYAML(...)

    // 写入 Repository
    if sm.repo != nil {
        sm.repo.Create(ctx, repository.SnapshotRecord{
            Name: meta.Name, CreatedAt: meta.CreatedAt,
            NodeCount: meta.NodeCount, RelCount: meta.RelCount,
            FilePath: meta.FilePath, Status: "active",
        })
    }
    return meta, nil
}

// List 优先从 Repository 读取
func (sm *SnapshotManager) List(ctx context.Context) ([]SnapshotMeta, error) {
    if sm.repo != nil {
        records, err := sm.repo.List(ctx)
        if err == nil {
            return recordsToMetas(records), nil
        }
        slog.Warn("repo list failed, falling back to cache", "error", err)
    }
    // Fallback: metaCache
    return sm.warmCache(ctx)
}
```

### Step 5: 单元测试

`internal/repository/snapshot_repo_test.go`：

| 测试 | 验证点 |
|------|--------|
| `TestPGSnapshotCreate` | 创建记录成功 |
| `TestPGSnapshotList` | 列表按 created_at DESC 排序 |
| `TestPGSnapshotGetByName` | 按名称查找 |
| `TestPGSnapshotDelete` | 删除记录成功 |
| `TestPGSnapshotUpdateStatus` | 状态更新正确 |
| `TestMemSnapshotCRUD` | 内存实现 CRUD 正确 |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/repository/snapshot_repo.go` | 新增 | 接口 + PG + 内存实现 |
| `internal/repository/snapshot_repo_test.go` | 新增 | CRUD 单元测试 |
| `internal/snapshot/manager.go` | 修改 | 集成 Repository |

---

## 验收标准

- [ ] SnapshotRepository 接口定义完整
- [ ] PostgreSQL 实现 CRUD 正确
- [ ] 内存 Fallback 实现 CRUD 正确
- [ ] SnapshotManager.Create 后 Repository 有记录
- [ ] SnapshotManager.List 优先从 Repository 读取
- [ ] `go test ./internal/repository/...` 全部通过
