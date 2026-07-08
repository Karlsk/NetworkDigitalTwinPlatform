//go:build integration

// Package repository_test 统一 PostgreSQL 集成测试（共享单个 PG 容器，测试全部 5 个 Repository）。
package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/repository"
)

// setupSharedPG 启动一个 PG 容器、执行迁移并返回 pool 和 cleanup。
// 复用 pg_test.go 中已有的 setupPostgresContainer。
func setupSharedPG(t *testing.T) (cleanup func(), snapRepo repository.SnapshotRepository, syncRepo repository.SyncLogRepository, connRepo repository.ConnectorConfigRepository, auditRepo repository.AuditLogRepository, schemaRepo repository.SchemaVersionRepository) {
	t.Helper()
	connStr, containerCleanup := setupPostgresContainer(t)

	ctx := context.Background()
	pool, err := repository.NewPGPool(ctx, repository.PGConfig{
		URL:      connStr,
		MaxConns: 10,
		MinConns: 1,
	})
	require.NoError(t, err, "create pg pool")

	err = repository.RunMigrations(pool)
	require.NoError(t, err, "run migrations")

	snapRepo = repository.NewPGSnapshotRepository(pool)
	syncRepo = repository.NewPGSyncLogRepository(pool)
	connRepo = repository.NewPGConnectorRepository(pool)
	auditRepo = repository.NewPGAuditLogRepository(pool)
	schemaRepo = repository.NewPGSchemaVersionRepository(pool)

	cleanup = func() {
		pool.Close()
		containerCleanup()
	}
	return
}

// ---------------------------------------------------------------------------
// TestIntegration_SnapshotCRUD
// ---------------------------------------------------------------------------

func TestIntegration_SnapshotCRUD(t *testing.T) {
	cleanup, repo, _, _, _, _ := setupSharedPG(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	// Create
	rec := &repository.SnapshotRecord{
		Name:      "integ-snap-001",
		CreatedAt: now,
		NodeCount: 42,
		RelCount:  18,
		FilePath:  "/data/snapshots/integ-snap-001.yaml",
		Status:    "active",
	}
	err := repo.Create(ctx, rec)
	require.NoError(t, err)
	require.NotZero(t, rec.ID, "ID should be populated after Create")

	// GetByName
	got, err := repo.GetByName(ctx, "integ-snap-001")
	require.NoError(t, err)
	require.Equal(t, "integ-snap-001", got.Name)
	require.Equal(t, 42, got.NodeCount)
	require.Equal(t, 18, got.RelCount)
	require.Equal(t, "/data/snapshots/integ-snap-001.yaml", got.FilePath)
	require.Equal(t, "active", got.Status)

	// UpdateStatus
	err = repo.UpdateStatus(ctx, "integ-snap-001", "archived")
	require.NoError(t, err)
	got2, err := repo.GetByName(ctx, "integ-snap-001")
	require.NoError(t, err)
	require.Equal(t, "archived", got2.Status)

	// List
	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "integ-snap-001", list[0].Name)

	// Delete
	err = repo.Delete(ctx, "integ-snap-001")
	require.NoError(t, err)

	_, err = repo.GetByName(ctx, "integ-snap-001")
	require.True(t, errors.Is(err, repository.ErrSnapshotNotFound))
}

// ---------------------------------------------------------------------------
// TestIntegration_SyncLogCRUD
// ---------------------------------------------------------------------------

func TestIntegration_SyncLogCRUD(t *testing.T) {
	cleanup, _, repo, _, _, _ := setupSharedPG(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	// Create 3 records
	records := []repository.SyncLogRecord{
		{SyncType: "full", Status: "success", NodesCreated: 42, RelationsCreated: 18, StartedAt: now.Add(-2 * time.Hour), CompletedAt: now, DurationMs: 3000},
		{SyncType: "incremental", Status: "success", NodesCreated: 5, RelationsCreated: 2, StartedAt: now.Add(-1 * time.Hour), CompletedAt: now, DurationMs: 500},
		{SyncType: "full", Status: "failed", StartedAt: now, CompletedAt: now, ErrorMessage: "timeout"},
	}
	for _, rec := range records {
		err := repo.Create(ctx, rec)
		require.NoError(t, err)
	}

	// List (all)
	list, err := repo.List(ctx, 0)
	require.NoError(t, err)
	require.Len(t, list, 3)

	// List with limit
	list2, err := repo.List(ctx, 2)
	require.NoError(t, err)
	require.Len(t, list2, 2)

	// ListByType
	fullList, err := repo.ListByType(ctx, "full", 0)
	require.NoError(t, err)
	require.Len(t, fullList, 2)
	for _, r := range fullList {
		require.Equal(t, "full", r.SyncType)
	}

	// Count
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
}

// ---------------------------------------------------------------------------
// TestIntegration_ConnectorCRUD
// ---------------------------------------------------------------------------

func TestIntegration_ConnectorCRUD(t *testing.T) {
	cleanup, _, _, repo, _, _ := setupSharedPG(t)
	defer cleanup()

	ctx := context.Background()

	// Upsert (create)
	rec := repository.ConnectorConfigRecord{
		Name:        "integ-mock",
		Type:        "mock",
		Config:      []byte(`{"data_dir":"testdata"}`),
		EntityTypes: []byte(`["Device"]`),
		Priority:    10,
		Status:      "active",
	}
	err := repo.Upsert(ctx, rec)
	require.NoError(t, err)

	// GetByName
	got, err := repo.GetByName(ctx, "integ-mock")
	require.NoError(t, err)
	require.Equal(t, "integ-mock", got.Name)
	require.Equal(t, "mock", got.Type)
	require.Equal(t, 10, got.Priority)

	// Upsert (update)
	updated := repository.ConnectorConfigRecord{
		Name:        "integ-mock",
		Type:        "mock",
		Config:      []byte(`{}`),
		EntityTypes: []byte(`[]`),
		Status:      "disabled",
		Priority:    20,
	}
	err = repo.Upsert(ctx, updated)
	require.NoError(t, err)
	got2, err := repo.GetByName(ctx, "integ-mock")
	require.NoError(t, err)
	require.Equal(t, "disabled", got2.Status)
	require.Equal(t, 20, got2.Priority)

	// UpdateStatus
	err = repo.UpdateStatus(ctx, "integ-mock", "error")
	require.NoError(t, err)

	// UpdateLastPing
	pingTime := time.Now().Truncate(time.Millisecond)
	err = repo.UpdateLastPing(ctx, "integ-mock", pingTime)
	require.NoError(t, err)
	got3, err := repo.GetByName(ctx, "integ-mock")
	require.NoError(t, err)
	require.NotNil(t, got3.LastPing)

	// List
	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	// Delete
	err = repo.Delete(ctx, "integ-mock")
	require.NoError(t, err)
	_, err = repo.GetByName(ctx, "integ-mock")
	require.True(t, errors.Is(err, repository.ErrConnectorConfigNotFound))
}

// ---------------------------------------------------------------------------
// TestIntegration_AuditLogCRUD
// ---------------------------------------------------------------------------

func TestIntegration_AuditLogCRUD(t *testing.T) {
	cleanup, _, _, _, repo, _ := setupSharedPG(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create
	records := []repository.AuditLogRecord{
		{Timestamp: now.Add(-2 * time.Hour), Action: "create", Snapshot: "snap-001", Actor: "mcp", Detail: "nodes=10"},
		{Timestamp: now.Add(-1 * time.Hour), Action: "restore", Snapshot: "snap-001", Actor: "system", Detail: "restore to default"},
		{Timestamp: now, Action: "delete", Snapshot: "snap-002", Actor: "http_api", Detail: "delete snapshot", Error: "not found"},
	}
	for _, rec := range records {
		err := repo.Create(ctx, rec)
		require.NoError(t, err)
	}

	// List
	list, err := repo.List(ctx, 10)
	require.NoError(t, err)
	require.Len(t, list, 3)
	// DESC order
	require.Equal(t, "delete", list[0].Action)
	require.Equal(t, "restore", list[1].Action)
	require.Equal(t, "create", list[2].Action)

	// Count
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)

	// Query by action
	byAction, err := repo.Query(ctx, repository.AuditFilter{Action: "create"})
	require.NoError(t, err)
	require.Len(t, byAction, 1)
	require.Equal(t, "create", byAction[0].Action)

	// Query by snapshot
	bySnap, err := repo.Query(ctx, repository.AuditFilter{Snapshot: "snap-001"})
	require.NoError(t, err)
	require.Len(t, bySnap, 2)

	// Query by time range
	byTime, err := repo.Query(ctx, repository.AuditFilter{
		Since: now.Add(-90 * time.Minute),
		Until: now.Add(-30 * time.Minute),
	})
	require.NoError(t, err)
	require.Len(t, byTime, 1)
	require.Equal(t, "restore", byTime[0].Action)
}

// ---------------------------------------------------------------------------
// TestIntegration_SchemaVersionCRUD
// ---------------------------------------------------------------------------

func TestIntegration_SchemaVersionCRUD(t *testing.T) {
	cleanup, _, _, _, _, repo := setupSharedPG(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	// Create
	rec1 := &repository.SchemaVersionRecord{
		Version:       1,
		EntityTypes:   []byte(`[{"name":"Device"}]`),
		RelationTypes: []byte(`[{"name":"HAS_INTERFACE"}]`),
		AppliedAt:     now,
		Description:   "initial schema",
	}
	err := repo.Create(ctx, rec1)
	require.NoError(t, err)
	require.NotZero(t, rec1.ID)

	rec2 := &repository.SchemaVersionRecord{
		Version:       2,
		EntityTypes:   []byte(`[{"name":"Device"},{"name":"Interface"}]`),
		RelationTypes: []byte(`[]`),
		AppliedAt:     now.Add(time.Hour),
		Description:   "added Interface",
	}
	err = repo.Create(ctx, rec2)
	require.NoError(t, err)

	// Latest
	latest, err := repo.Latest(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, latest.Version)
	require.Equal(t, "added Interface", latest.Description)

	// List
	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	// DESC: 2 → 1
	require.Equal(t, 2, list[0].Version)
	require.Equal(t, 1, list[1].Version)
}

// ---------------------------------------------------------------------------
// TestIntegration_CrossRepoIsolation
// ---------------------------------------------------------------------------

func TestIntegration_CrossRepoIsolation(t *testing.T) {
	cleanup, snapRepo, syncRepo, connRepo, auditRepo, schemaRepo := setupSharedPG(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	// 写入各 Repository
	err := snapRepo.Create(ctx, &repository.SnapshotRecord{
		Name: "cross-snap", CreatedAt: now, FilePath: "/cross.yaml", Status: "active",
	})
	require.NoError(t, err)

	err = syncRepo.Create(ctx, repository.SyncLogRecord{
		SyncType: "full", Status: "success", StartedAt: now, CompletedAt: now,
	})
	require.NoError(t, err)

	err = connRepo.Upsert(ctx, repository.ConnectorConfigRecord{
		Name: "cross-conn", Type: "mock", Config: []byte(`{}`), EntityTypes: []byte(`[]`), Status: "active",
	})
	require.NoError(t, err)

	err = auditRepo.Create(ctx, repository.AuditLogRecord{
		Timestamp: now, Action: "create", Snapshot: "cross-snap", Actor: "system",
	})
	require.NoError(t, err)

	err = schemaRepo.Create(ctx, &repository.SchemaVersionRecord{
		Version: 1, EntityTypes: []byte(`[]`), RelationTypes: []byte(`[]`), AppliedAt: now, Description: "v1",
	})
	require.NoError(t, err)

	// 验证各 Repository 数据独立
	snaps, _ := snapRepo.List(ctx)
	require.Len(t, snaps, 1)

	syncCount, _ := syncRepo.Count(ctx)
	require.Equal(t, int64(1), syncCount)

	conns, _ := connRepo.List(ctx)
	require.Len(t, conns, 1)

	auditCount, _ := auditRepo.Count(ctx)
	require.Equal(t, int64(1), auditCount)

	latest, _ := schemaRepo.Latest(ctx)
	require.Equal(t, 1, latest.Version)
}

// ---------------------------------------------------------------------------
// TestIntegration_MigrationsIdempotent
// ---------------------------------------------------------------------------

func TestIntegration_MigrationsIdempotent(t *testing.T) {
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

	// 第二次迁移（应幂等，不报错）
	err = repository.RunMigrations(pool)
	require.NoError(t, err, "second migration run should be idempotent")

	// 第三次迁移
	err = repository.RunMigrations(pool)
	require.NoError(t, err, "third migration run should be idempotent")

	// 验证表仍然存在且可写入
	_, err = pool.Exec(ctx,
		`INSERT INTO snapshots (name, node_count, rel_count, file_path) VALUES ($1, $2, $3, $4)`,
		"idempotent-test", 1, 0, "/test.yaml",
	)
	require.NoError(t, err)

	var name string
	err = pool.QueryRow(ctx, `SELECT name FROM snapshots WHERE name = $1`, "idempotent-test").Scan(&name)
	require.NoError(t, err)
	require.Equal(t, "idempotent-test", name)
}
