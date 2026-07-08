//go:build integration

// Package repository_test PostgreSQL ConnectorConfigRepository 集成测试（需 Docker）。
package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/repository"
)

// setupConnectorRepo 启动容器、迁移并返回 PG ConnectorConfigRepository 和清理函数。
func setupConnectorRepo(t *testing.T) (repository.ConnectorConfigRepository, func()) {
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

	repo := repository.NewPGConnectorRepository(pool)
	return repo, func() {
		pool.Close()
		cleanup()
	}
}

// TestPGConnectorUpsert 验证创建/更新成功，UPSERT 全量覆盖。
func TestPGConnectorUpsert(t *testing.T) {
	repo, cleanup := setupConnectorRepo(t)
	defer cleanup()

	ctx := context.Background()

	// 第一次 Upsert：创建
	rec := repository.ConnectorConfigRecord{
		Name:        "mock-001",
		Type:        "mock",
		Config:      []byte(`{"data_dir":"testdata"}`),
		EntityTypes: []byte(`["Device"]`),
		Priority:    10,
		Status:      "active",
	}
	err := repo.Upsert(ctx, rec)
	require.NoError(t, err)

	// 验证创建成功
	got, err := repo.GetByName(ctx, "mock-001")
	require.NoError(t, err)
	require.Equal(t, "mock-001", got.Name)
	require.Equal(t, "mock", got.Type)
	require.Equal(t, "active", got.Status)
	require.Equal(t, 10, got.Priority)

	// 第二次 Upsert：更新（全量覆盖）
	updatedRec := repository.ConnectorConfigRecord{
		Name:        "mock-001",
		Type:        "mock",
		Config:      []byte(`{"data_dir":"new"}`),
		EntityTypes: []byte(`["Device","Interface"]`),
		Priority:    20,
		Status:      "disabled",
	}
	err = repo.Upsert(ctx, updatedRec)
	require.NoError(t, err)

	// 验证更新成功
	got2, err := repo.GetByName(ctx, "mock-001")
	require.NoError(t, err)
	require.Equal(t, "disabled", got2.Status)
	require.Equal(t, 20, got2.Priority)
}

// TestPGConnectorGetByNameNotFound 验证不存在返回 ErrConnectorConfigNotFound。
func TestPGConnectorGetByNameNotFound(t *testing.T) {
	repo, cleanup := setupConnectorRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.GetByName(ctx, "nonexistent")
	require.Error(t, err)
	require.True(t, errors.Is(err, repository.ErrConnectorConfigNotFound),
		"expected ErrConnectorConfigNotFound, got: %v", err)
}

// TestPGConnectorList 验证列表返回所有连接器，按 name ASC 排序。
func TestPGConnectorList(t *testing.T) {
	repo, cleanup := setupConnectorRepo(t)
	defer cleanup()

	ctx := context.Background()

	records := []repository.ConnectorConfigRecord{
		{Name: "zebra", Type: "mock", Status: "active"},
		{Name: "alpha", Type: "netbox", Status: "active"},
		{Name: "middle", Type: "controller", Status: "disabled"},
	}
	for _, rec := range records {
		err := repo.Upsert(ctx, rec)
		require.NoError(t, err)
	}

	got, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, got, 3)

	// 按 name ASC：alpha → middle → zebra
	expected := []string{"alpha", "middle", "zebra"}
	for i, name := range expected {
		require.Equal(t, name, got[i].Name, "List()[%d].Name", i)
	}
}

// TestPGConnectorListEmpty 验证空表返回空切片。
func TestPGConnectorListEmpty(t *testing.T) {
	repo, cleanup := setupConnectorRepo(t)
	defer cleanup()

	ctx := context.Background()
	got, err := repo.List(ctx)
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestPGConnectorUpdateStatus 验证状态更新正确。
func TestPGConnectorUpdateStatus(t *testing.T) {
	repo, cleanup := setupConnectorRepo(t)
	defer cleanup()

	ctx := context.Background()

	rec := repository.ConnectorConfigRecord{
		Name:   "status-test",
		Type:   "mock",
		Status: "active",
	}
	err := repo.Upsert(ctx, rec)
	require.NoError(t, err)

	err = repo.UpdateStatus(ctx, "status-test", "error")
	require.NoError(t, err)

	got, err := repo.GetByName(ctx, "status-test")
	require.NoError(t, err)
	require.Equal(t, "error", got.Status)
}

// TestPGConnectorUpdateStatusNotFound 验证更新不存在的记录返回错误。
func TestPGConnectorUpdateStatusNotFound(t *testing.T) {
	repo, cleanup := setupConnectorRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.UpdateStatus(ctx, "nonexistent", "active")
	require.Error(t, err)
	require.True(t, errors.Is(err, repository.ErrConnectorConfigNotFound))
}

// TestPGConnectorUpdateLastPing 验证 LastPing 更新正确。
func TestPGConnectorUpdateLastPing(t *testing.T) {
	repo, cleanup := setupConnectorRepo(t)
	defer cleanup()

	ctx := context.Background()

	rec := repository.ConnectorConfigRecord{
		Name:   "ping-test",
		Type:   "mock",
		Status: "active",
	}
	err := repo.Upsert(ctx, rec)
	require.NoError(t, err)

	pingTime := time.Now().Truncate(time.Millisecond) // PG 微秒精度截断
	err = repo.UpdateLastPing(ctx, "ping-test", pingTime)
	require.NoError(t, err)

	got, err := repo.GetByName(ctx, "ping-test")
	require.NoError(t, err)
	require.NotNil(t, got.LastPing)
	require.True(t, got.LastPing.Equal(pingTime),
		"LastPing = %v, want %v", got.LastPing, pingTime)
}

// TestPGConnectorUpdateLastPingNotFound 验证更新不存在的记录返回错误。
func TestPGConnectorUpdateLastPingNotFound(t *testing.T) {
	repo, cleanup := setupConnectorRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.UpdateLastPing(ctx, "nonexistent", time.Now())
	require.Error(t, err)
	require.True(t, errors.Is(err, repository.ErrConnectorConfigNotFound))
}

// TestPGConnectorDelete 验证删除成功。
func TestPGConnectorDelete(t *testing.T) {
	repo, cleanup := setupConnectorRepo(t)
	defer cleanup()

	ctx := context.Background()

	rec := repository.ConnectorConfigRecord{
		Name:   "to-delete",
		Type:   "mock",
		Status: "active",
	}
	err := repo.Upsert(ctx, rec)
	require.NoError(t, err)

	err = repo.Delete(ctx, "to-delete")
	require.NoError(t, err)

	_, err = repo.GetByName(ctx, "to-delete")
	require.True(t, errors.Is(err, repository.ErrConnectorConfigNotFound))
}

// TestPGConnectorDeleteNotFound 验证删除不存在的记录返回错误。
func TestPGConnectorDeleteNotFound(t *testing.T) {
	repo, cleanup := setupConnectorRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.Delete(ctx, "nonexistent")
	require.Error(t, err)
	require.True(t, errors.Is(err, repository.ErrConnectorConfigNotFound))
}
