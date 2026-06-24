package graph

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
)

// ---------------------------------------------------------------------------
// mockLogicalDB — GraphDB 接口的可注入 mock，仅用于 logical_db 辅助函数测试
// ---------------------------------------------------------------------------

// mockLogicalDB 实现 GraphDB 接口，ClearDB 和 ListDBs 可注入，其余 panic。
type mockLogicalDB struct {
	clearDBFn  func(ctx context.Context, db string) error
	listDBsFn  func(ctx context.Context) ([]string, error)
	clearedDBs []string // 记录被 ClearDB 调用的 db 名
}

func (m *mockLogicalDB) Ping(_ context.Context) error { panic("not needed") }
func (m *mockLogicalDB) Close() error                 { panic("not needed") }
func (m *mockLogicalDB) BulkCreate(_ context.Context, _ string, _ []assembler.Node, _ []assembler.Relation) error {
	panic("not needed")
}
func (m *mockLogicalDB) Upsert(_ context.Context, _ string, _ []assembler.Node, _ []assembler.Relation) error {
	panic("not needed")
}
func (m *mockLogicalDB) DeleteRelations(_ context.Context, _ string, _ []assembler.Relation) error {
	panic("not needed")
}
func (m *mockLogicalDB) DeleteByURIs(_ context.Context, _ string, _ []string) error {
	panic("not needed")
}
func (m *mockLogicalDB) Query(_ context.Context, _ string, _ string, _ map[string]any) ([]map[string]any, error) {
	panic("not needed")
}
func (m *mockLogicalDB) BuildCypher(_ string, _ string, _ []assembler.Node, _ []assembler.Relation, _ []string) (string, map[string]any) {
	panic("not needed")
}
func (m *mockLogicalDB) CloneDB(_ context.Context, _, _ string) error { panic("not needed") }

func (m *mockLogicalDB) ClearDB(_ context.Context, db string) error {
	m.clearedDBs = append(m.clearedDBs, db)
	if m.clearDBFn != nil {
		return m.clearDBFn(context.Background(), db)
	}
	return nil
}

func (m *mockLogicalDB) ListDBs(_ context.Context) ([]string, error) {
	if m.listDBsFn != nil {
		return m.listDBsFn(context.Background())
	}
	return nil, nil
}

func (m *mockLogicalDB) HasDB(_ context.Context, _ string) (bool, error) {
	panic("not needed")
}

// 编译时检查
var _ GraphDB = (*mockLogicalDB)(nil)

// ---------------------------------------------------------------------------
// TestEnsureDBReady — ensureDBReady 辅助函数测试
// ---------------------------------------------------------------------------

func TestEnsureDBReady_Success(t *testing.T) {
	mock := &mockLogicalDB{}
	err := ensureDBReady(context.Background(), mock, "testdb")
	if err != nil {
		t.Fatalf("ensureDBReady() unexpected error: %v", err)
	}
	if len(mock.clearedDBs) != 1 || mock.clearedDBs[0] != "testdb" {
		t.Errorf("ensureDBReady() cleared DBs = %v, want [testdb]", mock.clearedDBs)
	}
}

func TestEnsureDBReady_Error(t *testing.T) {
	wantErr := errors.New("clear failed")
	mock := &mockLogicalDB{
		clearDBFn: func(_ context.Context, _ string) error {
			return wantErr
		},
	}
	err := ensureDBReady(context.Background(), mock, "testdb")
	if err == nil {
		t.Fatal("ensureDBReady() should return error when ClearDB fails")
	}
	if !strings.Contains(err.Error(), "ensure db ready") {
		t.Errorf("error should contain 'ensure db ready', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestCleanStaleDBs — cleanStaleDBs 辅助函数测试
// ---------------------------------------------------------------------------

func TestCleanStaleDBs_RemovesStale(t *testing.T) {
	mock := &mockLogicalDB{
		listDBsFn: func(_ context.Context) ([]string, error) {
			return []string{"default", "snapshot-1", "snapshot-2", "old-snapshot"}, nil
		},
	}
	keepDBs := map[string]bool{
		"snapshot-1": true,
		"snapshot-2": true,
	}
	err := cleanStaleDBs(context.Background(), mock, keepDBs)
	if err != nil {
		t.Fatalf("cleanStaleDBs() unexpected error: %v", err)
	}
	// old-snapshot 应被清理，default / snapshot-1 / snapshot-2 不应被清理
	if len(mock.clearedDBs) != 1 || mock.clearedDBs[0] != "old-snapshot" {
		t.Errorf("cleanStaleDBs() cleared DBs = %v, want [old-snapshot]", mock.clearedDBs)
	}
}

func TestCleanStaleDBs_KeepsDefault(t *testing.T) {
	mock := &mockLogicalDB{
		listDBsFn: func(_ context.Context) ([]string, error) {
			return []string{"default", "other"}, nil
		},
	}
	// keepDBs 中不包含 "default" 和 "other"，但 "default" 应被跳过
	keepDBs := map[string]bool{}
	err := cleanStaleDBs(context.Background(), mock, keepDBs)
	if err != nil {
		t.Fatalf("cleanStaleDBs() unexpected error: %v", err)
	}
	// 只有 "other" 被清理，"default" 被跳过
	if len(mock.clearedDBs) != 1 || mock.clearedDBs[0] != "other" {
		t.Errorf("cleanStaleDBs() cleared DBs = %v, want [other]", mock.clearedDBs)
	}
}

func TestCleanStaleDBs_ListError(t *testing.T) {
	wantErr := errors.New("list failed")
	mock := &mockLogicalDB{
		listDBsFn: func(_ context.Context) ([]string, error) {
			return nil, wantErr
		},
	}
	err := cleanStaleDBs(context.Background(), mock, map[string]bool{})
	if err == nil {
		t.Fatal("cleanStaleDBs() should return error when ListDBs fails")
	}
	if !strings.Contains(err.Error(), "clean stale dbs") {
		t.Errorf("error should contain 'clean stale dbs', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}
