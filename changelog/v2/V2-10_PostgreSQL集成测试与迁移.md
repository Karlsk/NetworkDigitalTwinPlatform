# V2-10: PostgreSQL 集成测试 + docker-compose 更新 + 数据迁移

**工时**: 1 天
**前置**: V2-06 ~ V2-09
**风险等级**: 高
**Phase**: Phase 2 — PostgreSQL 元数据存储

---

## 背景

V2-05~V2-09 完成了所有 Repository 接口和实现。本任务完成：
1. testcontainers 集成测试（真实 PostgreSQL 容器）
2. docker-compose 新增 PostgreSQL 服务
3. 数据迁移工具（从内存/YAML 迁移到 PostgreSQL）

---

## 实现步骤

### Step 1: testcontainers 集成测试

新建 `internal/repository/pg_integration_test.go`：

```go
//go:build integration

package repository_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"

    "gitlab.com/pml/network-digital-twin/internal/repository"
)

func setupPostgresContainer(t *testing.T) (*pgxpool.Pool, func()) {
    ctx := context.Background()
    container, err := postgres.Run(ctx, "postgres:16-alpine",
        postgres.WithDatabase("twin_test"),
        postgres.WithUsername("twin"),
        postgres.WithPassword("twin"),
    )
    require.NoError(t, err)

    connStr, err := container.ConnectionString(ctx, "sslmode=disable")
    require.NoError(t, err)

    pool, err := repository.NewPGPool(ctx, repository.PGConfig{URL: connStr})
    require.NoError(t, err)

    err = repository.RunMigrations(pool)
    require.NoError(t, err)

    return pool, func() {
        pool.Close()
        container.Terminate(ctx)
    }
}

func TestPGSnapshotRepoIntegration(t *testing.T) {
    pool, cleanup := setupPostgresContainer(t)
    defer cleanup()

    repo := repository.NewPGSnapshotRepository(pool)
    ctx := context.Background()

    // Create
    err := repo.Create(ctx, repository.SnapshotRecord{
        Name: "test-snap", NodeCount: 10, RelCount: 5,
        FilePath: "/tmp/test.yaml", Status: "active",
    })
    require.NoError(t, err)

    // List
    records, err := repo.List(ctx)
    require.NoError(t, err)
    require.Len(t, records, 1)
    require.Equal(t, "test-snap", records[0].Name)

    // Delete
    err = repo.Delete(ctx, "test-snap")
    require.NoError(t, err)
}

// ... 其他 Repository 集成测试
```

### Step 2: docker-compose 更新

```yaml
# deploy/docker-compose.yml 新增 PostgreSQL
  postgres:
    image: postgres:16-alpine
    container_name: postgres-twin
    ports:
      - "5432:5432"
    environment:
      POSTGRES_USER: twin
      POSTGRES_PASSWORD: twin
      POSTGRES_DB: twin
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U twin"]
      interval: 5s
      timeout: 3s
      retries: 5

  app:
    environment:
      POSTGRES_ENABLED: "true"
      POSTGRES_URL: "postgres://twin:twin@postgres:5432/twin?sslmode=disable"
    depends_on:
      neo4j:
        condition: service_healthy
      postgres:
        condition: service_healthy

volumes:
  neo4j_data:
  postgres_data:    # 新增
  snapshot_data:
```

### Step 3: 数据迁移工具

新建 `cmd/migrate-data/main.go`：

```go
package main

// 从 YAML 快照元数据迁移到 PostgreSQL
func main() {
    // 1. 扫描 snapshots/ 目录
    // 2. 解析每个 YAML 文件的 meta 头
    // 3. 写入 snapshots 表
    // 4. 迁移 audit_logs（如果内存中有）
    // 5. 输出迁移统计
}
```

### Step 4: main.go PostgreSQL 初始化

修改 `cmd/server/main.go`：

```go
// 可选：初始化 PostgreSQL
var pgPool *pgxpool.Pool
if cfg.Postgres.Enabled {
    pgPool, err = repository.NewPGPool(ctx, repository.PGConfig{
        URL: cfg.Postgres.URL, MaxConns: cfg.Postgres.MaxConns,
    })
    if err != nil {
        slog.Error("connect postgres", "error", err)
        os.Exit(1)
    }
    defer pgPool.Close()

    if err := repository.RunMigrations(pgPool); err != nil {
        slog.Error("run migrations", "error", err)
        os.Exit(1)
    }
}
```

### Step 5: 单元测试

| 测试 | 验证点 |
|------|--------|
| `TestPGIntegration` | 全部 Repository 在真实 PG 上 CRUD 正确 |
| `TestMigrationsIdempotent` | 重复执行迁移不报错 |
| `TestDataMigrationTool` | YAML 元数据正确迁移到 PG |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/repository/pg_integration_test.go` | 新增 | testcontainers 集成测试 |
| `cmd/migrate-data/main.go` | 新增 | 数据迁移工具 |
| `deploy/docker-compose.yml` | 修改 | 新增 PostgreSQL 服务 |
| `cmd/server/main.go` | 修改 | PostgreSQL 初始化 + 迁移执行 |

---

## 验收标准

- [ ] testcontainers 集成测试全部通过
- [ ] docker-compose up 启动包含 PostgreSQL 服务
- [ ] 应用启动时自动执行数据库迁移
- [ ] 数据迁移工具可正确迁移 YAML 元数据到 PostgreSQL
- [ ] `go build ./...` 无错误
