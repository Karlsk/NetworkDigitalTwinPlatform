//go:build integration

// Package repository_test PostgreSQL AuditLogRepository 集成测试（需 Docker，通过 -tags=integration 触发）。
package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/repository"
)

// setupAuditRepo 启动容器、迁移并返回 PG AuditLogRepository 和清理函数。
func setupAuditRepo(t *testing.T) (repository.AuditLogRepository, func()) {
	t.Helper()
	connStr, cleanup := setupPostgresContainer(t)

	ctx := context.Background()
	pool, err := repository.NewPGPool(ctx, repository.PGConfig{
		URL:      connStr,
		MaxConns: 5,
		MinConns: 1,
	})
	require.NoError(t, err, "create pg pool")

	err = repository.RunMigrations(pool)
	require.NoError(t, err, "run migrations")

	repo := repository.NewPGAuditLogRepository(pool)
	return repo, func() {
		pool.Close()
		cleanup()
	}
}

// TestPGAuditLogCreate 验证 INSERT 成功。
func TestPGAuditLogCreate(t *testing.T) {
	repo, cleanup := setupAuditRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	err := repo.Create(ctx, repository.AuditLogRecord{
		Timestamp: now,
		Action:    "create",
		Snapshot:  "snap-001",
		Actor:     "system",
		Detail:    "nodes=10, rels=5",
		Error:     "",
	})
	require.NoError(t, err)

	// 验证查询
	records, err := repo.List(ctx, 10)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "create", records[0].Action)
	require.Equal(t, "snap-001", records[0].Snapshot)
}

// TestPGAuditLogList 验证多条记录按 timestamp DESC 排序。
func TestPGAuditLogList(t *testing.T) {
	repo, cleanup := setupAuditRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	records := []repository.AuditLogRecord{
		{Timestamp: now.Add(-2 * time.Hour), Action: "create", Snapshot: "old", Actor: "system"},
		{Timestamp: now, Action: "restore", Snapshot: "new", Actor: "system"},
		{Timestamp: now.Add(-1 * time.Hour), Action: "delete", Snapshot: "mid", Actor: "system"},
	}
	for _, r := range records {
		err := repo.Create(ctx, r)
		require.NoError(t, err)
	}

	got, err := repo.List(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 3)

	// 按 timestamp DESC: new → mid → old
	expected := []string{"new", "mid", "old"}
	for i, name := range expected {
		require.Equal(t, name, got[i].Snapshot, "List()[%d].Snapshot", i)
	}
}

// TestPGAuditLogListEmpty 验证空表返回空切片。
func TestPGAuditLogListEmpty(t *testing.T) {
	repo, cleanup := setupAuditRepo(t)
	defer cleanup()

	ctx := context.Background()
	got, err := repo.List(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestPGAuditLogCount 验证计数正确。
func TestPGAuditLogCount(t *testing.T) {
	repo, cleanup := setupAuditRepo(t)
	defer cleanup()

	ctx := context.Background()

	// 空表
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)

	// 插入 3 条
	for i := 0; i < 3; i++ {
		err := repo.Create(ctx, repository.AuditLogRecord{
			Timestamp: time.Now(),
			Action:    "create",
			Snapshot:  "snap",
			Actor:     "system",
		})
		require.NoError(t, err)
	}

	count, err = repo.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
}

// TestPGAuditLogQueryByAction 验证按 action 过滤。
func TestPGAuditLogQueryByAction(t *testing.T) {
	repo, cleanup := setupAuditRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	records := []repository.AuditLogRecord{
		{Timestamp: now, Action: "create", Snapshot: "snap-a", Actor: "system"},
		{Timestamp: now, Action: "restore", Snapshot: "snap-b", Actor: "system"},
		{Timestamp: now, Action: "create", Snapshot: "snap-c", Actor: "system"},
	}
	for _, r := range records {
		err := repo.Create(ctx, r)
		require.NoError(t, err)
	}

	got, err := repo.Query(ctx, repository.AuditFilter{Action: "create"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, r := range got {
		require.Equal(t, "create", r.Action)
	}
}

// TestPGAuditLogQueryBySnapshot 验证按 snapshot 过滤。
func TestPGAuditLogQueryBySnapshot(t *testing.T) {
	repo, cleanup := setupAuditRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	records := []repository.AuditLogRecord{
		{Timestamp: now, Action: "create", Snapshot: "target", Actor: "system"},
		{Timestamp: now, Action: "create", Snapshot: "other", Actor: "system"},
		{Timestamp: now, Action: "delete", Snapshot: "target", Actor: "system"},
	}
	for _, r := range records {
		err := repo.Create(ctx, r)
		require.NoError(t, err)
	}

	got, err := repo.Query(ctx, repository.AuditFilter{Snapshot: "target"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, r := range got {
		require.Equal(t, "target", r.Snapshot)
	}
}

// TestPGAuditLogQueryByTimeRange 验证按时间范围过滤。
func TestPGAuditLogQueryByTimeRange(t *testing.T) {
	repo, cleanup := setupAuditRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	records := []repository.AuditLogRecord{
		{Timestamp: now.Add(-3 * time.Hour), Action: "create", Snapshot: "old", Actor: "system"},
		{Timestamp: now.Add(-30 * time.Minute), Action: "create", Snapshot: "recent", Actor: "system"},
		{Timestamp: now, Action: "create", Snapshot: "latest", Actor: "system"},
	}
	for _, r := range records {
		err := repo.Create(ctx, r)
		require.NoError(t, err)
	}

	// Since: 最近 1 小时
	got, err := repo.Query(ctx, repository.AuditFilter{Since: now.Add(-1 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Until: 1 小时前
	got, err = repo.Query(ctx, repository.AuditFilter{Until: now.Add(-1 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "old", got[0].Snapshot)
}

// TestPGAuditLogQueryCombined 验证多条件组合过滤。
func TestPGAuditLogQueryCombined(t *testing.T) {
	repo, cleanup := setupAuditRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	records := []repository.AuditLogRecord{
		{Timestamp: now, Action: "create", Snapshot: "snap-a", Actor: "system"},
		{Timestamp: now, Action: "restore", Snapshot: "snap-a", Actor: "system"},
		{Timestamp: now, Action: "create", Snapshot: "snap-b", Actor: "system"},
		{Timestamp: now.Add(-3 * time.Hour), Action: "create", Snapshot: "snap-a", Actor: "system"},
	}
	for _, r := range records {
		err := repo.Create(ctx, r)
		require.NoError(t, err)
	}

	// Action + Snapshot + Since 组合
	got, err := repo.Query(ctx, repository.AuditFilter{
		Action:   "create",
		Snapshot: "snap-a",
		Since:    now.Add(-1 * time.Hour),
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "snap-a", got[0].Snapshot)
	require.Equal(t, "create", got[0].Action)
}
