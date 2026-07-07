//go:build integration

// Package repository_test PostgreSQL SyncLogRepository 集成测试。
package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/repository"
)

// setupSyncLogRepo 启动容器、迁移并返回 PG SyncLogRepository 和清理函数。
func setupSyncLogRepo(t *testing.T) (repository.SyncLogRepository, func()) {
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

	repo := repository.NewPGSyncLogRepository(pool)
	return repo, func() {
		pool.Close()
		cleanup()
	}
}

// TestPGSyncLogCreate 验证 INSERT 后 SELECT 数据一致。
func TestPGSyncLogCreate(t *testing.T) {
	repo, cleanup := setupSyncLogRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	rec := repository.SyncLogRecord{
		SyncType:         "full",
		Status:           "success",
		NodesCreated:     42,
		RelationsCreated: 18,
		OrphanEdges:      2,
		Warnings:         []byte(`["warn1"]`),
		ErrorMessage:     "",
		StartedAt:        now,
		CompletedAt:      now.Add(3 * time.Second),
		DurationMs:       3000,
	}
	err := repo.Create(ctx, rec)
	require.NoError(t, err)

	// 通过 List 查询验证
	got, err := repo.List(ctx, 0)
	require.NoError(t, err)
	require.Len(t, got, 1)

	r := got[0]
	require.NotZero(t, r.ID)
	require.Equal(t, "full", r.SyncType)
	require.Equal(t, "success", r.Status)
	require.Equal(t, 42, r.NodesCreated)
	require.Equal(t, 18, r.RelationsCreated)
	require.Equal(t, 2, r.OrphanEdges)
	require.Equal(t, int64(3000), r.DurationMs)
}

// TestPGSyncLogList 验证按 started_at DESC 排序 + limit 截断。
func TestPGSyncLogList(t *testing.T) {
	repo, cleanup := setupSyncLogRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	records := []repository.SyncLogRecord{
		{SyncType: "full", Status: "success", StartedAt: now.Add(-2 * time.Hour), CompletedAt: now},
		{SyncType: "full", Status: "success", StartedAt: now, CompletedAt: now},
		{SyncType: "full", Status: "success", StartedAt: now.Add(-1 * time.Hour), CompletedAt: now},
	}
	for _, rec := range records {
		err := repo.Create(ctx, rec)
		require.NoError(t, err)
	}

	// 无 limit
	got, err := repo.List(ctx, 0)
	require.NoError(t, err)
	require.Len(t, got, 3)
	// DESC: now → now-1h → now-2h
	require.True(t, got[0].StartedAt.After(got[1].StartedAt) || got[0].StartedAt.Equal(got[1].StartedAt))
	require.True(t, got[1].StartedAt.After(got[2].StartedAt) || got[1].StartedAt.Equal(got[2].StartedAt))

	// limit = 2
	got2, err := repo.List(ctx, 2)
	require.NoError(t, err)
	require.Len(t, got2, 2)
}

// TestPGSyncLogListByType 验证按 sync_type 过滤。
func TestPGSyncLogListByType(t *testing.T) {
	repo, cleanup := setupSyncLogRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	_ = repo.Create(ctx, repository.SyncLogRecord{SyncType: "full", Status: "success", StartedAt: now, CompletedAt: now})
	_ = repo.Create(ctx, repository.SyncLogRecord{SyncType: "incremental", Status: "success", StartedAt: now, CompletedAt: now})
	_ = repo.Create(ctx, repository.SyncLogRecord{SyncType: "full", Status: "failed", StartedAt: now, CompletedAt: now})

	got, err := repo.ListByType(ctx, "full", 0)
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, r := range got {
		require.Equal(t, "full", r.SyncType)
	}

	// limit = 1
	got2, err := repo.ListByType(ctx, "full", 1)
	require.NoError(t, err)
	require.Len(t, got2, 1)
}

// TestPGSyncLogListByTypeEmpty 验证无匹配返回空切片。
func TestPGSyncLogListByTypeEmpty(t *testing.T) {
	repo, cleanup := setupSyncLogRepo(t)
	defer cleanup()

	ctx := context.Background()
	_ = repo.Create(ctx, repository.SyncLogRecord{SyncType: "full", Status: "success", StartedAt: time.Now(), CompletedAt: time.Now()})

	got, err := repo.ListByType(ctx, "incremental", 0)
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestPGSyncLogCount 验证 COUNT(*) 正确。
func TestPGSyncLogCount(t *testing.T) {
	repo, cleanup := setupSyncLogRepo(t)
	defer cleanup()

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = repo.Create(ctx, repository.SyncLogRecord{SyncType: "full", Status: "success", StartedAt: time.Now(), CompletedAt: time.Now()})
	}

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(5), count)
}

// TestPGSyncLogCountEmpty 验证空表返回 0。
func TestPGSyncLogCountEmpty(t *testing.T) {
	repo, cleanup := setupSyncLogRepo(t)
	defer cleanup()

	ctx := context.Background()
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}
