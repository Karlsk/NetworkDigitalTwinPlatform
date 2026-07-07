//go:build integration

// Package repository_test PostgreSQL SnapshotRepository 集成测试（需 Docker，通过 -tags=integration 触发）。
package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/repository"
)

// setupSnapshotRepo 启动容器、迁移并返回 PG SnapshotRepository 和清理函数。
func setupSnapshotRepo(t *testing.T) (repository.SnapshotRepository, func()) {
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

	repo := repository.NewPGSnapshotRepository(pool)
	return repo, func() {
		pool.Close()
		cleanup()
	}
}

// TestPGSnapshotCreate 验证 INSERT 后 SELECT 数据一致。
func TestPGSnapshotCreate(t *testing.T) {
	repo, cleanup := setupSnapshotRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond) // PG 微秒精度截断

	rec := &repository.SnapshotRecord{
		Name:      "pg-snap-001",
		CreatedAt: now,
		NodeCount: 42,
		RelCount:  18,
		FilePath:  "/data/snapshots/pg-snap-001.yaml",
		Status:    "active",
	}
	err := repo.Create(ctx, rec)
	require.NoError(t, err)
	require.NotZero(t, rec.ID, "ID should be populated after Create")

	// 查询验证
	got, err := repo.GetByName(ctx, "pg-snap-001")
	require.NoError(t, err)
	require.Equal(t, "pg-snap-001", got.Name)
	require.Equal(t, 42, got.NodeCount)
	require.Equal(t, 18, got.RelCount)
	require.Equal(t, "/data/snapshots/pg-snap-001.yaml", got.FilePath)
	require.Equal(t, "active", got.Status)
}

// TestPGSnapshotList 验证多条记录按 created_at DESC 排序。
func TestPGSnapshotList(t *testing.T) {
	repo, cleanup := setupSnapshotRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	records := []repository.SnapshotRecord{
		{Name: "old", CreatedAt: now.Add(-2 * time.Hour), Status: "active", FilePath: "/old.yaml"},
		{Name: "new", CreatedAt: now, Status: "active", FilePath: "/new.yaml"},
		{Name: "mid", CreatedAt: now.Add(-1 * time.Hour), Status: "active", FilePath: "/mid.yaml"},
	}
	for i := range records {
		err := repo.Create(ctx, &records[i])
		require.NoError(t, err)
	}

	got, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, got, 3)

	// 按 created_at DESC：new → mid → old
	expected := []string{"new", "mid", "old"}
	for i, name := range expected {
		require.Equal(t, name, got[i].Name, "List()[%d].Name", i)
	}
}

// TestPGSnapshotGetByName 验证精确名称查找。
func TestPGSnapshotGetByName(t *testing.T) {
	repo, cleanup := setupSnapshotRepo(t)
	defer cleanup()

	ctx := context.Background()
	rec := &repository.SnapshotRecord{
		Name:      "find-me",
		CreatedAt: time.Now(),
		NodeCount: 7,
		RelCount:  3,
		FilePath:  "/find.yaml",
		Status:    "active",
	}
	err := repo.Create(ctx, rec)
	require.NoError(t, err)

	got, err := repo.GetByName(ctx, "find-me")
	require.NoError(t, err)
	require.Equal(t, "find-me", got.Name)
	require.Equal(t, 7, got.NodeCount)
}

// TestPGSnapshotGetByNameNotFound 验证不存在记录返回 ErrSnapshotNotFound。
func TestPGSnapshotGetByNameNotFound(t *testing.T) {
	repo, cleanup := setupSnapshotRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.GetByName(ctx, "does-not-exist")
	require.Error(t, err)
	require.True(t, errors.Is(err, repository.ErrSnapshotNotFound),
		"expected ErrSnapshotNotFound, got: %v", err)
}

// TestPGSnapshotDelete 验证删除成功。
func TestPGSnapshotDelete(t *testing.T) {
	repo, cleanup := setupSnapshotRepo(t)
	defer cleanup()

	ctx := context.Background()
	rec := &repository.SnapshotRecord{
		Name:      "to-delete",
		CreatedAt: time.Now(),
		FilePath:  "/del.yaml",
		Status:    "active",
	}
	err := repo.Create(ctx, rec)
	require.NoError(t, err)

	err = repo.Delete(ctx, "to-delete")
	require.NoError(t, err)

	// 删除后查询应返回 NotFound
	_, err = repo.GetByName(ctx, "to-delete")
	require.True(t, errors.Is(err, repository.ErrSnapshotNotFound))
}

// TestPGSnapshotDeleteNotFound 验证删除不存在的记录返回 ErrSnapshotNotFound。
func TestPGSnapshotDeleteNotFound(t *testing.T) {
	repo, cleanup := setupSnapshotRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.Delete(ctx, "nonexistent")
	require.Error(t, err)
	require.True(t, errors.Is(err, repository.ErrSnapshotNotFound))
}

// TestPGSnapshotUpdateStatus 验证状态更新正确。
func TestPGSnapshotUpdateStatus(t *testing.T) {
	repo, cleanup := setupSnapshotRepo(t)
	defer cleanup()

	ctx := context.Background()
	rec := &repository.SnapshotRecord{
		Name:      "status-upd",
		CreatedAt: time.Now(),
		FilePath:  "/upd.yaml",
		Status:    "active",
	}
	err := repo.Create(ctx, rec)
	require.NoError(t, err)

	err = repo.UpdateStatus(ctx, "status-upd", "archived")
	require.NoError(t, err)

	got, err := repo.GetByName(ctx, "status-upd")
	require.NoError(t, err)
	require.Equal(t, "archived", got.Status)
}

// TestPGSnapshotUpdateStatusNotFound 验证更新不存在记录返回 ErrSnapshotNotFound。
func TestPGSnapshotUpdateStatusNotFound(t *testing.T) {
	repo, cleanup := setupSnapshotRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.UpdateStatus(ctx, "nonexistent", "active")
	require.Error(t, err)
	require.True(t, errors.Is(err, repository.ErrSnapshotNotFound))
}

// TestPGSnapshotCreateDuplicate 验证 UNIQUE 约束冲突返回错误。
func TestPGSnapshotCreateDuplicate(t *testing.T) {
	repo, cleanup := setupSnapshotRepo(t)
	defer cleanup()

	ctx := context.Background()
	rec1 := &repository.SnapshotRecord{
		Name:      "dup-name",
		CreatedAt: time.Now(),
		FilePath:  "/dup1.yaml",
		Status:    "active",
	}
	err := repo.Create(ctx, rec1)
	require.NoError(t, err)

	rec2 := &repository.SnapshotRecord{
		Name:      "dup-name",
		CreatedAt: time.Now(),
		FilePath:  "/dup2.yaml",
		Status:    "active",
	}
	err = repo.Create(ctx, rec2)
	require.Error(t, err, "duplicate name should fail UNIQUE constraint")
}

// TestPGSnapshotListEmpty 验证空表返回空切片。
func TestPGSnapshotListEmpty(t *testing.T) {
	repo, cleanup := setupSnapshotRepo(t)
	defer cleanup()

	ctx := context.Background()
	got, err := repo.List(ctx)
	require.NoError(t, err)
	require.Empty(t, got)
}
