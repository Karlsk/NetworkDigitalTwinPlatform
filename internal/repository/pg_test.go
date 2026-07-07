//go:build integration

// Package repository PostgreSQL 集成测试（需 Docker 环境，通过 -tags=integration 触发）。
package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"gitlab.com/pml/network-digital-twin/internal/repository"
)

// setupPostgresContainer 启动 PostgreSQL 容器并返回连接 URL 和清理函数。
func setupPostgresContainer(t *testing.T) (string, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithUsername("twin"),
		postgres.WithPassword("twin"),
		postgres.WithDatabase("twin"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err, "start postgres container")

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "get connection string")

	return connStr, func() {
		_ = container.Terminate(ctx)
	}
}

// TestNewPGPoolSuccess 验证连接池创建成功。
func TestNewPGPoolSuccess(t *testing.T) {
	connStr, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()
	pool, err := repository.NewPGPool(ctx, repository.PGConfig{
		URL:      connStr,
		MaxConns: 5,
		MinConns: 1,
	})
	require.NoError(t, err)
	defer pool.Close()

	// 验证 Ping 成功
	err = pool.Ping(ctx)
	require.NoError(t, err)

	// 验证连接池配置
	stat := pool.Stat()
	require.Equal(t, int32(5), stat.MaxConns())
}

// TestNewPGPoolInvalidURLIntegration 验证无效 URL 返回 error（无需容器）。
func TestNewPGPoolInvalidURLIntegration(t *testing.T) {
	ctx := context.Background()
	_, err := repository.NewPGPool(ctx, repository.PGConfig{
		URL: "not-a-valid-url",
	})
	require.Error(t, err)
}

// TestRunMigrations 验证 migrate up 创建 5 张表。
func TestRunMigrations(t *testing.T) {
	connStr, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()
	pool, err := repository.NewPGPool(ctx, repository.PGConfig{
		URL:      connStr,
		MaxConns: 5,
		MinConns: 1,
	})
	require.NoError(t, err)
	defer pool.Close()

	// 执行迁移
	err = repository.RunMigrations(pool)
	require.NoError(t, err)

	// 验证 5 张表已创建
	expectedTables := []string{
		"snapshots",
		"sync_logs",
		"connector_configs",
		"audit_logs",
		"schema_versions",
	}

	for _, table := range expectedTables {
		var exists bool
		err = pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table,
		).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "table %s should exist after migration", table)
	}
}

// TestRunMigrationsIdempotent 验证重复执行不报错。
func TestRunMigrationsIdempotent(t *testing.T) {
	connStr, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()
	pool, err := repository.NewPGPool(ctx, repository.PGConfig{
		URL:      connStr,
		MaxConns: 5,
		MinConns: 1,
	})
	require.NoError(t, err)
	defer pool.Close()

	// 第一次迁移
	err = repository.RunMigrations(pool)
	require.NoError(t, err)

	// 第二次迁移（应该幂等，不报错）
	err = repository.RunMigrations(pool)
	require.NoError(t, err, "second migration run should be idempotent")

	// 验证表仍然存在
	var count int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('snapshots','sync_logs','connector_configs','audit_logs','schema_versions')`,
	).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 5, count)
}

// TestMigrationsCreateIndexes 验证迁移创建了预期的索引。
func TestMigrationsCreateIndexes(t *testing.T) {
	connStr, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()
	pool, err := repository.NewPGPool(ctx, repository.PGConfig{
		URL:      connStr,
		MaxConns: 5,
		MinConns: 1,
	})
	require.NoError(t, err)
	defer pool.Close()

	err = repository.RunMigrations(pool)
	require.NoError(t, err)

	// 验证关键索引存在
	expectedIndexes := []string{
		"idx_snapshots_created_at",
		"idx_snapshots_status",
		"idx_sync_logs_started_at",
		"idx_sync_logs_type",
		"idx_connector_configs_type",
		"idx_audit_logs_timestamp",
		"idx_audit_logs_action",
		"idx_audit_logs_snapshot",
	}

	for _, idx := range expectedIndexes {
		var exists bool
		err = pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`,
			idx,
		).Scan(&exists)
		require.NoError(t, err)
		require.True(t, exists, "index %s should exist after migration", idx)
	}

	// 验证表可以正常插入数据（snapshots 表）
	_, err = pool.Exec(ctx,
		`INSERT INTO snapshots (name, node_count, rel_count, file_path) VALUES ($1, $2, $3, $4)`,
		"test-snap", 10, 5, "/tmp/test.yaml",
	)
	require.NoError(t, err)

	// 验证查询
	var name string
	var nodeCount int
	err = pool.QueryRow(ctx, `SELECT name, node_count FROM snapshots WHERE name = $1`, "test-snap").Scan(&name, &nodeCount)
	require.NoError(t, err)
	require.Equal(t, "test-snap", name)
	require.Equal(t, 10, nodeCount)
}

// TestPGPoolMaxConnsApplied 验证 MaxConns/MinConns 正确应用。
func TestPGPoolMaxConnsApplied(t *testing.T) {
	connStr, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()
	pool, err := repository.NewPGPool(ctx, repository.PGConfig{
		URL:      connStr,
		MaxConns: 3,
		MinConns: 1,
	})
	require.NoError(t, err)
	defer pool.Close()

	stat := pool.Stat()
	require.Equal(t, int32(3), stat.MaxConns())
}

// TestPGPoolZeroConnsUseDefaults 验证 MaxConns=0 时使用 pgxpool 默认值。
func TestPGPoolZeroConnsUseDefaults(t *testing.T) {
	connStr, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()
	pool, err := repository.NewPGPool(ctx, repository.PGConfig{
		URL: connStr,
		// MaxConns 和 MinConns 留零值，使用 pgxpool 默认
	})
	require.NoError(t, err)
	defer pool.Close()

	// pgxpool 默认 MaxConns = 4（或系统 CPU 数）
	stat := pool.Stat()
	require.Greater(t, stat.MaxConns(), int32(0))

	// 验证仍然可以正常查询
	var result int
	err = pool.QueryRow(ctx, `SELECT 1`).Scan(&result)
	require.NoError(t, err)
	require.Equal(t, 1, result)
}
