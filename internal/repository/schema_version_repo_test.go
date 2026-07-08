//go:build integration

// Package repository_test PostgreSQL SchemaVersionRepository 集成测试（需 Docker）。
package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/repository"
)

// setupSchemaVersionRepo 启动容器、迁移并返回 PG SchemaVersionRepository 和清理函数。
func setupSchemaVersionRepo(t *testing.T) (repository.SchemaVersionRepository, func()) {
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

	repo := repository.NewPGSchemaVersionRepository(pool)
	return repo, func() {
		pool.Close()
		cleanup()
	}
}

// TestPGSchemaVersionCreate 验证版本记录创建成功。
func TestPGSchemaVersionCreate(t *testing.T) {
	repo, cleanup := setupSchemaVersionRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	rec := &repository.SchemaVersionRecord{
		Version:       1,
		EntityTypes:   []byte(`[{"name":"Device"}]`),
		RelationTypes: []byte(`[{"name":"HAS_INTERFACE"}]`),
		AppliedAt:     now,
		Description:   "initial schema",
	}
	err := repo.Create(ctx, rec)
	require.NoError(t, err)
	require.NotZero(t, rec.ID, "ID should be populated after Create")

	// 查询验证
	latest, err := repo.Latest(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, latest.Version)
	require.Equal(t, "initial schema", latest.Description)
}

// TestPGSchemaVersionLatest 验证返回最新版本。
func TestPGSchemaVersionLatest(t *testing.T) {
	repo, cleanup := setupSchemaVersionRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	records := []repository.SchemaVersionRecord{
		{Version: 1, AppliedAt: now.Add(-2 * time.Hour), Description: "v1"},
		{Version: 3, AppliedAt: now, Description: "v3"},
		{Version: 2, AppliedAt: now.Add(-1 * time.Hour), Description: "v2"},
	}
	for i := range records {
		err := repo.Create(ctx, &records[i])
		require.NoError(t, err)
	}

	latest, err := repo.Latest(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, latest.Version, "Latest should return highest version")
	require.Equal(t, "v3", latest.Description)
}

// TestPGSchemaVersionLatestEmpty 验证空表返回 ErrSchemaVersionNotFound。
func TestPGSchemaVersionLatestEmpty(t *testing.T) {
	repo, cleanup := setupSchemaVersionRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.Latest(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, repository.ErrSchemaVersionNotFound),
		"expected ErrSchemaVersionNotFound, got: %v", err)
}

// TestPGSchemaVersionList 验证按 version DESC 排序。
func TestPGSchemaVersionList(t *testing.T) {
	repo, cleanup := setupSchemaVersionRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	records := []repository.SchemaVersionRecord{
		{Version: 1, AppliedAt: now, Description: "v1"},
		{Version: 5, AppliedAt: now, Description: "v5"},
		{Version: 3, AppliedAt: now, Description: "v3"},
	}
	for i := range records {
		err := repo.Create(ctx, &records[i])
		require.NoError(t, err)
	}

	got, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, got, 3)

	// 按 version DESC：5 → 3 → 1
	expected := []int{5, 3, 1}
	for i, v := range expected {
		require.Equal(t, v, got[i].Version, "List()[%d].Version", i)
	}
}

// TestPGSchemaVersionListEmpty 验证空表返回空切片。
func TestPGSchemaVersionListEmpty(t *testing.T) {
	repo, cleanup := setupSchemaVersionRepo(t)
	defer cleanup()

	ctx := context.Background()
	got, err := repo.List(ctx)
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestPGSchemaVersionCreateDuplicate 验证 UNIQUE 约束冲突返回错误。
func TestPGSchemaVersionCreateDuplicate(t *testing.T) {
	repo, cleanup := setupSchemaVersionRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	rec1 := &repository.SchemaVersionRecord{
		Version:   1,
		AppliedAt: now,
	}
	err := repo.Create(ctx, rec1)
	require.NoError(t, err)

	rec2 := &repository.SchemaVersionRecord{
		Version:   1, // 重复版本号
		AppliedAt: now,
	}
	err = repo.Create(ctx, rec2)
	require.Error(t, err, "duplicate version should fail UNIQUE constraint")
}
