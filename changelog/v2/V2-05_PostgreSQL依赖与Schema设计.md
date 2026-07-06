# V2-05: PostgreSQL 依赖引入 + Schema DDL + 迁移工具

**工时**: 1.5 天
**前置**: 无（与 Phase 1 可并行）
**风险等级**: 中
**Phase**: Phase 2 — PostgreSQL 元数据存储

---

## 背景

V1 现状：元数据（快照记录、同步历史、连接器配置、审计日志）存于内存或 YAML 文件，进程重启丢失。
V2 目标：引入 PostgreSQL 存储结构化元数据，进程重启后数据完整保留。

### V1 元数据存储现状

| 数据 | V1 存储 | 问题 |
|------|---------|------|
| 快照元数据 | `SnapshotManager.metaCache` (内存 map) + YAML 文件头 | 进程重启需重建缓存 |
| 同步历史 | 仅 slog 日志 | 无法结构化查询历史 |
| 连接器配置 | `configs/connectors.yaml` 文件 | 无版本管理，无运行时状态 |
| 审计日志 | `AuditLog.entries` (内存 FIFO) | 进程重启全部丢失 |
| Schema 版本 | 无 | 无法追踪本体变更历史 |

---

## 实现步骤

### Step 1: 引入 PostgreSQL 依赖

```bash
go get github.com/jackc/pgx/v5@latest
go get github.com/golang-migrate/migrate/v4@latest
go get github.com/golang-migrate/migrate/v4/database/postgres@latest
go get github.com/golang-migrate/migrate/v4/source/iofs@latest
```

**选型理由**：
- `pgx/v5` 是高性能纯 Go PostgreSQL 驱动，支持连接池（pgxpool）
- `golang-migrate` 是成熟的 Schema 迁移工具，支持嵌入文件系统

### Step 2: Schema DDL 设计

新建 `migrations/000001_init.up.sql`：

```sql
-- 快照元数据
CREATE TABLE IF NOT EXISTS snapshots (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    node_count  INTEGER NOT NULL DEFAULT 0,
    rel_count   INTEGER NOT NULL DEFAULT 0,
    file_path   VARCHAR(1024) NOT NULL,
    status      VARCHAR(32) NOT NULL DEFAULT 'active',
    metadata    JSONB DEFAULT '{}'
);

CREATE INDEX idx_snapshots_created_at ON snapshots(created_at);
CREATE INDEX idx_snapshots_status ON snapshots(status);

-- 同步历史日志
CREATE TABLE IF NOT EXISTS sync_logs (
    id              BIGSERIAL PRIMARY KEY,
    sync_type       VARCHAR(32) NOT NULL,          -- "full" / "incremental"
    status          VARCHAR(32) NOT NULL,          -- "success" / "failed"
    nodes_created   INTEGER NOT NULL DEFAULT 0,
    relations_created INTEGER NOT NULL DEFAULT 0,
    orphan_edges    INTEGER NOT NULL DEFAULT 0,
    warnings        JSONB DEFAULT '[]',
    error_message   TEXT,
    started_at      TIMESTAMPTZ NOT NULL,
    completed_at    TIMESTAMPTZ,
    duration_ms     BIGINT
);

CREATE INDEX idx_sync_logs_started_at ON sync_logs(started_at);
CREATE INDEX idx_sync_logs_type ON sync_logs(sync_type);

-- 连接器配置
CREATE TABLE IF NOT EXISTS connector_configs (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL UNIQUE,
    type        VARCHAR(64) NOT NULL,              -- "mock" / "netbox" / "controller" / "cmdb"
    config      JSONB NOT NULL DEFAULT '{}',
    entity_types JSONB NOT NULL DEFAULT '[]',
    priority    INTEGER NOT NULL DEFAULT 0,
    status      VARCHAR(32) NOT NULL DEFAULT 'active',
    last_ping   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_connector_configs_type ON connector_configs(type);

-- 审计日志
CREATE TABLE IF NOT EXISTS audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    action      VARCHAR(64) NOT NULL,              -- "create" / "restore" / "delete" / "diff" / "sync"
    snapshot    VARCHAR(255),
    actor       VARCHAR(64) NOT NULL,              -- "mcp" / "webhook" / "system" / "http_api"
    detail      TEXT,
    error       TEXT
);

CREATE INDEX idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_snapshot ON audit_logs(snapshot);

-- Schema 版本追踪
CREATE TABLE IF NOT EXISTS schema_versions (
    id              BIGSERIAL PRIMARY KEY,
    version         INTEGER NOT NULL UNIQUE,
    entity_types    JSONB NOT NULL DEFAULT '[]',
    relation_types  JSONB NOT NULL DEFAULT '[]',
    applied_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    description     TEXT
);
```

新建 `migrations/000001_init.down.sql`：

```sql
DROP TABLE IF EXISTS schema_versions;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS connector_configs;
DROP TABLE IF EXISTS sync_logs;
DROP TABLE IF EXISTS snapshots;
```

### Step 3: 数据库连接层

新建 `internal/repository/pg.go`：

```go
package repository

import (
    "context"
    "embed"
    "fmt"
    "log/slog"

    "github.com/golang-migrate/migrate/v4"
    "github.com/golang-migrate/migrate/v4/database/postgres"
    "github.com/golang-migrate/migrate/v4/source/iofs"
    "github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// PGConfig PostgreSQL 连接配置。
type PGConfig struct {
    URL          string // postgres://user:pass@host:port/db?sslmode=disable
    MaxConns     int32  // 最大连接数，默认 10
    MinConns     int32  // 最小连接数，默认 2
}

// NewPGPool 创建 PostgreSQL 连接池。
func NewPGPool(ctx context.Context, cfg PGConfig) (*pgxpool.Pool, error) {
    poolCfg, err := pgxpool.ParseConfig(cfg.URL)
    if err != nil {
        return nil, fmt.Errorf("parse pg config: %w", err)
    }
    if cfg.MaxConns > 0 {
        poolCfg.MaxConns = cfg.MaxConns
    }
    if cfg.MinConns > 0 {
        poolCfg.MinConns = cfg.MinConns
    }

    pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
    if err != nil {
        return nil, fmt.Errorf("create pg pool: %w", err)
    }

    if err := pool.Ping(ctx); err != nil {
        pool.Close()
        return nil, fmt.Errorf("ping pg: %w", err)
    }

    slog.Info("postgresql connected", "max_conns", poolCfg.MaxConns)
    return pool, nil
}

// RunMigrations 执行数据库迁移。
func RunMigrations(pool *pgxpool.Pool) error {
    source, err := iofs.New(migrationsFS, "migrations")
    if err != nil {
        return fmt.Errorf("create migration source: %w", err)
    }

    driver, err := postgres.WithInstance(pool.Acquire(context.Background()).Conn().PgConn(), &postgres.Config{})
    if err != nil {
        return fmt.Errorf("create migration driver: %w", err)
    }

    m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
    if err != nil {
        return fmt.Errorf("create migrate instance: %w", err)
    }

    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("run migrations: %w", err)
    }

    slog.Info("database migrations completed")
    return nil
}
```

### Step 4: 配置扩展

`internal/config/config.go` 新增：

```go
// PGConfig PostgreSQL 配置。
type PGConfig struct {
    Enabled  bool   `mapstructure:"enabled"`    // false = 不启用 PostgreSQL
    URL      string `mapstructure:"url"`         // postgres://user:pass@host:port/db
    MaxConns int32  `mapstructure:"max_conns"`   // 默认 10
    MinConns int32  `mapstructure:"min_conns"`   // 默认 2
}
```

`configs/config.yaml` 新增：

```yaml
postgres:
  enabled: false                          # V2 默认关闭
  url: "postgres://twin:twin@localhost:5432/twin?sslmode=disable"
  max_conns: 10
  min_conns: 2
```

### Step 5: 单元测试

`internal/repository/pg_test.go`：

| 测试 | 验证点 |
|------|--------|
| `TestNewPGPoolSuccess` | 连接池创建成功（testcontainers PostgreSQL） |
| `TestNewPGPoolInvalidURL` | 无效 URL 返回 error |
| `TestRunMigrations` | migrate up 创建 5 张表 |
| `TestRunMigrationsIdempotent` | 重复执行不报错 |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `go.mod` | 修改 | 新增 pgx/v5 + golang-migrate |
| `migrations/000001_init.up.sql` | 新增 | DDL up 迁移 |
| `migrations/000001_init.down.sql` | 新增 | DDL down 迁移 |
| `internal/repository/pg.go` | 新增 | PG 连接池 + 迁移执行 |
| `internal/repository/pg_test.go` | 新增 | 连接池 + 迁移测试 |
| `internal/config/config.go` | 修改 | 新增 PGConfig |
| `configs/config.yaml` | 修改 | 新增 postgres 段 |

---

## 验收标准

- [ ] `go get` 引入 pgx/v5 + golang-migrate 成功
- [ ] DDL 迁移文件语法正确
- [ ] `NewPGPool` 连接 PostgreSQL 成功
- [ ] `RunMigrations` 创建 5 张表，重复执行不报错
- [ ] `postgres.enabled: false` 时不影响现有功能
- [ ] `go build ./...` 无错误
