// Package repository 提供 PG CRUD 函数的 mock 单元测试。
// pg_mock_test.go 使用内部 pgQuerier 接口模拟 PG 操作。
package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ──────────────────────────────
// Mock 实现
// ──────────────────────────────

// mockRow 实现 pgx.Row 接口。
type mockRow struct {
	values []any
	err    error
}

func (r *mockRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("mock row: scan column count mismatch")
	}
	for i, v := range r.values {
		switch d := dest[i].(type) {
		case *int64:
			*d = v.(int64)
		case *string:
			*d = v.(string)
		case *int:
			*d = v.(int)
		case *time.Time:
			*d = v.(time.Time)
		case *[]byte:
			if v == nil {
				*d = nil
			} else {
				*d = v.([]byte)
			}
		}
	}
	return nil
}

// mockRows 实现 pgx.Rows 接口。
type mockRows struct {
	data    [][]any
	idx     int
	closed  bool
	scanErr error
}

func (r *mockRows) Close()                                       { r.closed = true }
func (r *mockRows) Err() error                                   { return nil }
func (r *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *mockRows) Conn() *pgx.Conn                              { return nil }
func (r *mockRows) RawValues() [][]byte                          { return nil }
func (r *mockRows) Values() ([]any, error)                       { return nil, nil }
func (r *mockRows) Next() bool {
	r.idx++
	return r.idx <= len(r.data)
}
func (r *mockRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.data[r.idx-1]
	for i, v := range row {
		if i >= len(dest) {
			break
		}
		switch d := dest[i].(type) {
		case *int64:
			*d = v.(int64)
		case *string:
			*d = v.(string)
		case *int:
			*d = v.(int)
		case *time.Time:
			*d = v.(time.Time)
		case *[]byte:
			if v == nil {
				*d = nil
			} else {
				*d = v.([]byte)
			}
		}
	}
	return nil
}

// mockPG 实现 pgQuerier 接口。
type mockPG struct {
	queryRowResult pgx.Row
	queryResult    pgx.Rows
	queryErr       error
	execTag        pgconn.CommandTag
	execErr        error
}

func (m *mockPG) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return m.queryRowResult
}

func (m *mockPG) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return m.queryResult, nil
}

func (m *mockPG) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return m.execTag, m.execErr
}

// newCommandTag 创建一个模拟的 CommandTag。
func newCommandTag(rowsAffected int64) pgconn.CommandTag {
	return pgconn.NewCommandTag("UPDATE " + string(rune('0'+rowsAffected)))
}

// ──────────────────────────────
// SnapshotRepository PG 测试
// ──────────────────────────────

func TestPGSnapshotRepo_Create(t *testing.T) {
	mock := &mockPG{
		queryRowResult: &mockRow{values: []any{int64(42)}},
	}
	repo := &pgSnapshotRepo{db: mock}
	rec := &SnapshotRecord{Name: "snap-1", NodeCount: 10, RelCount: 5, Status: "active"}
	err := repo.Create(context.Background(), rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ID != 42 {
		t.Errorf("expected ID=42, got %d", rec.ID)
	}
}

func TestPGSnapshotRepo_CreateError(t *testing.T) {
	mock := &mockPG{
		queryRowResult: &mockRow{err: errors.New("pg error")},
	}
	repo := &pgSnapshotRepo{db: mock}
	err := repo.Create(context.Background(), &SnapshotRecord{Name: "snap-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPGSnapshotRepo_GetByName(t *testing.T) {
	now := time.Now()
	mock := &mockPG{
		queryRowResult: &mockRow{values: []any{int64(1), "snap-1", now, 10, 5, "/path", "active"}},
	}
	repo := &pgSnapshotRepo{db: mock}
	rec, err := repo.GetByName(context.Background(), "snap-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Name != "snap-1" {
		t.Errorf("expected name=snap-1, got %s", rec.Name)
	}
}

func TestPGSnapshotRepo_GetByName_NotFound(t *testing.T) {
	mock := &mockPG{
		queryRowResult: &mockRow{err: pgx.ErrNoRows},
	}
	repo := &pgSnapshotRepo{db: mock}
	_, err := repo.GetByName(context.Background(), "missing")
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound, got %v", err)
	}
}

func TestPGSnapshotRepo_List(t *testing.T) {
	now := time.Now()
	mock := &mockPG{
		queryResult: &mockRows{
			data: [][]any{
				{int64(1), "snap-1", now, 10, 5, "/p1", "active"},
				{int64(2), "snap-2", now, 20, 10, "/p2", "active"},
			},
		},
	}
	repo := &pgSnapshotRepo{db: mock}
	records, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
}

func TestPGSnapshotRepo_ListError(t *testing.T) {
	mock := &mockPG{queryErr: errors.New("db error")}
	repo := &pgSnapshotRepo{db: mock}
	_, err := repo.List(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPGSnapshotRepo_Delete(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(1)}
	repo := &pgSnapshotRepo{db: mock}
	err := repo.Delete(context.Background(), "snap-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPGSnapshotRepo_Delete_NotFound(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(0)}
	repo := &pgSnapshotRepo{db: mock}
	err := repo.Delete(context.Background(), "missing")
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound, got %v", err)
	}
}

func TestPGSnapshotRepo_UpdateStatus(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(1)}
	repo := &pgSnapshotRepo{db: mock}
	err := repo.UpdateStatus(context.Background(), "snap-1", "archived")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPGSnapshotRepo_UpdateStatus_NotFound(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(0)}
	repo := &pgSnapshotRepo{db: mock}
	err := repo.UpdateStatus(context.Background(), "missing", "archived")
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound, got %v", err)
	}
}

// ──────────────────────────────
// ConnectorConfigRepository PG 测试
// ──────────────────────────────

func TestPGConnectorRepo_Upsert(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(1)}
	repo := &pgConnectorConfigRepo{db: mock}
	err := repo.Upsert(context.Background(), ConnectorConfigRecord{Name: "mock-1", Type: "mock"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPGConnectorRepo_UpsertError(t *testing.T) {
	mock := &mockPG{execErr: errors.New("pg error")}
	repo := &pgConnectorConfigRepo{db: mock}
	err := repo.Upsert(context.Background(), ConnectorConfigRecord{Name: "mock-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPGConnectorRepo_Delete(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(1)}
	repo := &pgConnectorConfigRepo{db: mock}
	err := repo.Delete(context.Background(), "mock-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPGConnectorRepo_Delete_NotFound(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(0)}
	repo := &pgConnectorConfigRepo{db: mock}
	err := repo.Delete(context.Background(), "missing")
	if !errors.Is(err, ErrConnectorConfigNotFound) {
		t.Fatalf("expected ErrConnectorConfigNotFound, got %v", err)
	}
}

func TestPGConnectorRepo_UpdateStatus(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(1)}
	repo := &pgConnectorConfigRepo{db: mock}
	err := repo.UpdateStatus(context.Background(), "mock-1", "active")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPGConnectorRepo_UpdateStatus_NotFound(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(0)}
	repo := &pgConnectorConfigRepo{db: mock}
	err := repo.UpdateStatus(context.Background(), "missing", "active")
	if !errors.Is(err, ErrConnectorConfigNotFound) {
		t.Fatalf("expected ErrConnectorConfigNotFound, got %v", err)
	}
}

func TestPGConnectorRepo_UpdateLastPing(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(1)}
	repo := &pgConnectorConfigRepo{db: mock}
	err := repo.UpdateLastPing(context.Background(), "mock-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPGConnectorRepo_UpdateLastPing_NotFound(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(0)}
	repo := &pgConnectorConfigRepo{db: mock}
	err := repo.UpdateLastPing(context.Background(), "missing", time.Now())
	if !errors.Is(err, ErrConnectorConfigNotFound) {
		t.Fatalf("expected ErrConnectorConfigNotFound, got %v", err)
	}
}

// ──────────────────────────────
// AuditLogRepository PG 测试
// ──────────────────────────────

func TestPGAuditRepo_Create(t *testing.T) {
	mock := &mockPG{execTag: newCommandTag(1)}
	repo := &pgAuditLogRepo{db: mock}
	err := repo.Create(context.Background(), AuditLogRecord{Action: "create", Snapshot: "snap-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPGAuditRepo_CreateError(t *testing.T) {
	mock := &mockPG{execErr: errors.New("pg error")}
	repo := &pgAuditLogRepo{db: mock}
	err := repo.Create(context.Background(), AuditLogRecord{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPGAuditRepo_Count(t *testing.T) {
	mock := &mockPG{
		queryRowResult: &mockRow{values: []any{int64(42)}},
	}
	repo := &pgAuditLogRepo{db: mock}
	count, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 42 {
		t.Errorf("expected count=42, got %d", count)
	}
}

// ──────────────────────────────
// SyncLogRepository PG 测试
// ──────────────────────────────

func TestPGSyncLogRepo_Create(t *testing.T) {
	mock := &mockPG{
		queryRowResult: &mockRow{values: []any{int64(99)}},
	}
	repo := &pgSyncLogRepo{db: mock}
	rec := SyncLogRecord{SyncType: "full", Status: "success"}
	// Create 接受值参数，不回填调用者的 ID；只验证不报错
	err := repo.Create(context.Background(), rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPGSyncLogRepo_Count(t *testing.T) {
	mock := &mockPG{
		queryRowResult: &mockRow{values: []any{int64(10)}},
	}
	repo := &pgSyncLogRepo{db: mock}
	count, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 10 {
		t.Errorf("expected count=10, got %d", count)
	}
}

// ──────────────────────────────
// SchemaVersionRepository PG 测试
// ──────────────────────────────

func TestPGSchemaVersionRepo_Create(t *testing.T) {
	mock := &mockPG{
		queryRowResult: &mockRow{values: []any{int64(1)}},
	}
	repo := &pgSchemaVersionRepo{db: mock}
	rec := &SchemaVersionRecord{Version: 100}
	err := repo.Create(context.Background(), rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ID != 1 {
		t.Errorf("expected ID=1, got %d", rec.ID)
	}
}

func TestPGSchemaVersionRepo_Latest(t *testing.T) {
	now := time.Now()
	mock := &mockPG{
		queryRowResult: &mockRow{values: []any{int64(1), 42, []byte("[]"), []byte("[]"), now, "test"}},
	}
	repo := &pgSchemaVersionRepo{db: mock}
	rec, err := repo.Latest(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Version != 42 {
		t.Errorf("expected version=42, got %d", rec.Version)
	}
}

func TestPGSchemaVersionRepo_Latest_NotFound(t *testing.T) {
	mock := &mockPG{
		queryRowResult: &mockRow{err: pgx.ErrNoRows},
	}
	repo := &pgSchemaVersionRepo{db: mock}
	_, err := repo.Latest(context.Background())
	if !errors.Is(err, ErrSchemaVersionNotFound) {
		t.Fatalf("expected ErrSchemaVersionNotFound, got %v", err)
	}
}

func TestPGSchemaVersionRepo_List(t *testing.T) {
	now := time.Now()
	mock := &mockPG{
		queryResult: &mockRows{
			data: [][]any{
				{int64(1), 1, []byte("[]"), []byte("[]"), now, "v1"},
				{int64(2), 2, []byte("[]"), []byte("[]"), now, "v2"},
			},
		},
	}
	repo := &pgSchemaVersionRepo{db: mock}
	records, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
}
