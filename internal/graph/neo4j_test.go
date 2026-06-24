package graph

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/auth"
	"gitlab.com/pml/network-digital-twin/internal/config"
)

// ---------------------------------------------------------------------------
// mockDriver — neo4j.DriverWithContext 的最小 mock 实现
// ---------------------------------------------------------------------------

// mockDriver 实现 neo4j.DriverWithContext 接口。
// VerifyConnectivity 和 Close 使用可注入函数，其余方法 panic（I-07 不会调用）。
type mockDriver struct {
	verifyConnectivityFn func(ctx context.Context) error
	closeFn              func(ctx context.Context) error
}

func (m *mockDriver) VerifyConnectivity(ctx context.Context) error {
	if m.verifyConnectivityFn != nil {
		return m.verifyConnectivityFn(ctx)
	}
	return nil
}

func (m *mockDriver) Close(ctx context.Context) error {
	if m.closeFn != nil {
		return m.closeFn(ctx)
	}
	return nil
}

// --- 以下方法 I-07 不会调用，提供 panic 占位 ---

func (m *mockDriver) ExecuteQueryBookmarkManager() neo4j.BookmarkManager {
	panic("not implemented: ExecuteQueryBookmarkManager")
}

func (m *mockDriver) Target() url.URL {
	panic("not implemented: Target")
}

func (m *mockDriver) NewSession(_ context.Context, _ neo4j.SessionConfig) neo4j.SessionWithContext {
	panic("not implemented: NewSession")
}

func (m *mockDriver) VerifyAuthentication(_ context.Context, _ *neo4j.AuthToken) error {
	panic("not implemented: VerifyAuthentication")
}

func (m *mockDriver) IsEncrypted() bool {
	panic("not implemented: IsEncrypted")
}

func (m *mockDriver) GetServerInfo(_ context.Context) (neo4j.ServerInfo, error) {
	panic("not implemented: GetServerInfo")
}

// 编译时检查 mockDriver 满足 DriverWithContext 接口
var _ neo4j.DriverWithContext = (*mockDriver)(nil)

// ---------------------------------------------------------------------------
// helper: 替换 driverFactory 并在测试结束自动恢复
// ---------------------------------------------------------------------------

// withMockDriver 将 driverFactory 替换为返回指定 mockDriver 的工厂函数，
// 返回的 cleanup 函数需在测试结束时调用（通常 defer）。
func withMockDriver(t *testing.T, md *mockDriver) {
	t.Helper()
	origFactory := driverFactory
	driverFactory = func(_ string, _ auth.TokenManager, _ ...func(*neo4j.Config)) (neo4j.DriverWithContext, error) {
		return md, nil
	}
	t.Cleanup(func() { driverFactory = origFactory })
}

// testCfg 返回用于测试的 Neo4JConfig
func testCfg() config.Neo4JConfig {
	return config.Neo4JConfig{
		URI:       "bolt://mock-host:7687",
		User:      "neo4j",
		Password:  "password",
		DefaultDB: "testdb",
	}
}

// ---------------------------------------------------------------------------
// mockSession / mockResult — 内部 session/result 接口的测试 mock
// ---------------------------------------------------------------------------

// mockSession 实现内部 session 接口，可注入 runFn / closeFn。
type mockSession struct {
	runFn   func(ctx context.Context, cypher string, params map[string]any, configurers ...func(*neo4j.TransactionConfig)) (result, error)
	closeFn func(ctx context.Context) error
}

func (m *mockSession) Run(ctx context.Context, cypher string, params map[string]any, configurers ...func(*neo4j.TransactionConfig)) (result, error) {
	if m.runFn != nil {
		return m.runFn(ctx, cypher, params, configurers...)
	}
	return &mockResult{}, nil
}

func (m *mockSession) Close(ctx context.Context) error {
	if m.closeFn != nil {
		return m.closeFn(ctx)
	}
	return nil
}

// mockResult 实现内部 result 接口，基于 []*neo4j.Record + 游标索引。
type mockResult struct {
	records []*neo4j.Record
	idx     int
	err     error
}

func (m *mockResult) Next(_ context.Context) bool {
	if m.idx < len(m.records) {
		m.idx++
		return true
	}
	return false
}

func (m *mockResult) Record() *neo4j.Record {
	if m.idx > 0 && m.idx <= len(m.records) {
		return m.records[m.idx-1]
	}
	return nil
}

func (m *mockResult) Err() error {
	return m.err
}

// 编译时检查 mockSession / mockResult 满足内部接口
var _ session = (*mockSession)(nil)
var _ result = (*mockResult)(nil)

// withMockSessionFactory 替换 sessionFactory 为返回指定 mockSession 的函数，
// 测试结束时自动恢复。
func withMockSessionFactory(t *testing.T, ms *mockSession) {
	t.Helper()
	orig := sessionFactory
	sessionFactory = func(_ context.Context, _ neo4j.DriverWithContext, _ neo4j.SessionConfig) session {
		return ms
	}
	t.Cleanup(func() { sessionFactory = orig })
}

// newTestClient 创建用于测试的 neo4jClient（使用 mockDriver）。
func newTestClient(t *testing.T) *neo4jClient {
	t.Helper()
	withMockDriver(t, &mockDriver{})
	client, err := NewNeo4jClient(testCfg())
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}
	return client
}

// ---------------------------------------------------------------------------
// TestNewNeo4jClient — 构造函数测试
// ---------------------------------------------------------------------------

func TestNewNeo4jClient_Success(t *testing.T) {
	// 使用真实 driverFactory：NewDriverWithContext 创建时不建立网络连接，
	// 合法 URI 应成功创建客户端。
	client, err := NewNeo4jClient(testCfg())
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("NewNeo4jClient() returned nil client")
	}
	// 清理：关闭底层 driver
	defer client.Close()
}

func TestNewNeo4jClient_InvalidURI(t *testing.T) {
	cfg := testCfg()
	cfg.URI = "://invalid-uri" // url.Parse 会失败的格式

	client, err := NewNeo4jClient(cfg)
	if err == nil {
		client.Close()
		t.Fatal("NewNeo4jClient() should return error for invalid URI")
	}
	if !strings.Contains(err.Error(), "create neo4j driver") {
		t.Errorf("error should contain 'create neo4j driver', got: %v", err)
	}
}

func TestNewNeo4jClient_DefaultDB(t *testing.T) {
	withMockDriver(t, &mockDriver{})

	client, err := NewNeo4jClient(testCfg())
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}
	if client.defaultDB != "testdb" {
		t.Errorf("defaultDB = %q, want %q", client.defaultDB, "testdb")
	}
}

func TestNewNeo4jClient_DefaultDBEmpty(t *testing.T) {
	withMockDriver(t, &mockDriver{})

	cfg := testCfg()
	cfg.DefaultDB = ""

	client, err := NewNeo4jClient(cfg)
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}
	if client.defaultDB != "" {
		t.Errorf("defaultDB = %q, want empty string", client.defaultDB)
	}
}

// ---------------------------------------------------------------------------
// TestPing — 连接验证测试
// ---------------------------------------------------------------------------

func TestPing_Success(t *testing.T) {
	withMockDriver(t, &mockDriver{
		verifyConnectivityFn: func(_ context.Context) error {
			return nil // 模拟连接成功
		},
	})

	client, err := NewNeo4jClient(testCfg())
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}

	if err := client.Ping(context.Background()); err != nil {
		t.Errorf("Ping() unexpected error: %v", err)
	}
}

func TestPing_Failure(t *testing.T) {
	wantErr := errors.New("connection refused")
	withMockDriver(t, &mockDriver{
		verifyConnectivityFn: func(_ context.Context) error {
			return wantErr
		},
	})

	client, err := NewNeo4jClient(testCfg())
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}

	err = client.Ping(context.Background())
	if err == nil {
		t.Fatal("Ping() should return error when connection fails")
	}
	if !strings.Contains(err.Error(), "neo4j ping") {
		t.Errorf("error should contain 'neo4j ping', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestPing_ContextCanceled(t *testing.T) {
	withMockDriver(t, &mockDriver{
		verifyConnectivityFn: func(ctx context.Context) error {
			return ctx.Err() // 模拟 context 已取消
		},
	})

	client, err := NewNeo4jClient(testCfg())
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	err = client.Ping(ctx)
	if err == nil {
		t.Fatal("Ping() should return error when context is canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error should wrap context.Canceled, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestClose — 关闭连接测试
// ---------------------------------------------------------------------------

func TestClose_Success(t *testing.T) {
	closeCalled := false
	withMockDriver(t, &mockDriver{
		closeFn: func(_ context.Context) error {
			closeCalled = true
			return nil
		},
	})

	client, err := NewNeo4jClient(testCfg())
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Errorf("Close() unexpected error: %v", err)
	}
	if !closeCalled {
		t.Error("Close() should call driver.Close")
	}
}

func TestClose_Failure(t *testing.T) {
	wantErr := errors.New("close failed")
	withMockDriver(t, &mockDriver{
		closeFn: func(_ context.Context) error {
			return wantErr
		},
	})

	client, err := NewNeo4jClient(testCfg())
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}

	err = client.Close()
	if err == nil {
		t.Fatal("Close() should return error when driver close fails")
	}
	if !strings.Contains(err.Error(), "neo4j close") {
		t.Errorf("error should contain 'neo4j close', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestQuery — Query 方法测试
// ---------------------------------------------------------------------------

func TestQuery_Success(t *testing.T) {
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return &mockResult{
				records: []*neo4j.Record{
					{Keys: []string{"name", "age"}, Values: []any{"alice", 30}},
					{Keys: []string{"name", "age"}, Values: []any{"bob", 25}},
				},
			}, nil
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	records, err := client.Query(context.Background(), "default", "MATCH (n) RETURN n", map[string]any{"limit": 10})
	if err != nil {
		t.Fatalf("Query() unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Query() returned %d records, want 2", len(records))
	}
	if records[0]["name"] != "alice" || records[0]["age"] != 30 {
		t.Errorf("records[0] = %v, want {name:alice, age:30}", records[0])
	}
	if records[1]["name"] != "bob" || records[1]["age"] != 25 {
		t.Errorf("records[1] = %v, want {name:bob, age:25}", records[1])
	}
}

func TestQuery_NilParams(t *testing.T) {
	var gotParams map[string]any
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, params map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			gotParams = params
			return &mockResult{}, nil
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	_, err := client.Query(context.Background(), "mydb", "MATCH (n) RETURN n", nil)
	if err != nil {
		t.Fatalf("Query() unexpected error: %v", err)
	}
	if gotParams == nil {
		t.Fatal("Query() should initialize params when nil")
	}
	if gotParams["_db"] != "mydb" {
		t.Errorf("params[_db] = %v, want 'mydb'", gotParams["_db"])
	}
}

func TestQuery_InjectsDBParam(t *testing.T) {
	var gotParams map[string]any
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, params map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			gotParams = params
			return &mockResult{}, nil
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	_, err := client.Query(context.Background(), "testdb", "MATCH (n) RETURN n", map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("Query() unexpected error: %v", err)
	}
	if gotParams["_db"] != "testdb" {
		t.Errorf("params[_db] = %v, want 'testdb'", gotParams["_db"])
	}
	if gotParams["key"] != "value" {
		t.Errorf("params[key] = %v, want 'value'", gotParams["key"])
	}
}

func TestQuery_NoMutateCallerParams(t *testing.T) {
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return &mockResult{}, nil
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	original := map[string]any{"key": "value"}
	_, err := client.Query(context.Background(), "testdb", "MATCH (n) RETURN n", original)
	if err != nil {
		t.Fatalf("Query() unexpected error: %v", err)
	}
	// 验证原始 map 未被修改（不应包含 _db 键）
	if _, hasDB := original["_db"]; hasDB {
		t.Errorf("Query() should not mutate caller's params, but original now contains _db: %v", original)
	}
	if original["key"] != "value" {
		t.Errorf("original map modified: key = %v, want 'value'", original["key"])
	}
}

func TestQuery_EmptyResult(t *testing.T) {
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return &mockResult{}, nil
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	records, err := client.Query(context.Background(), "default", "MATCH (n) RETURN n", nil)
	if err != nil {
		t.Fatalf("Query() unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("Query() returned %d records, want 0", len(records))
	}
}

func TestQuery_RunError(t *testing.T) {
	wantErr := errors.New("connection lost")
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return nil, wantErr
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	_, err := client.Query(context.Background(), "default", "MATCH (n) RETURN n", nil)
	if err == nil {
		t.Fatal("Query() should return error when Run fails")
	}
	if !strings.Contains(err.Error(), "run cypher") {
		t.Errorf("error should contain 'run cypher', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestQuery_IterateError(t *testing.T) {
	wantErr := errors.New("result stream failed")
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return &mockResult{
				records: []*neo4j.Record{
					{Keys: []string{"n"}, Values: []any{"node1"}},
				},
				err: wantErr,
			}, nil
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	_, err := client.Query(context.Background(), "default", "MATCH (n) RETURN n", nil)
	if err == nil {
		t.Fatal("Query() should return error when result.Err() is non-nil")
	}
	if !strings.Contains(err.Error(), "iterate result") {
		t.Errorf("error should contain 'iterate result', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestClearDB — ClearDB 方法测试
// ---------------------------------------------------------------------------

func TestClearDB_Success(t *testing.T) {
	var gotCypher string
	var gotParams map[string]any
	var gotAccessMode neo4j.AccessMode

	ms := &mockSession{
		runFn: func(_ context.Context, cypher string, params map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			gotCypher = cypher
			gotParams = params
			return &mockResult{}, nil
		},
	}
	// 捕获 sessionFactory 被调用时的 SessionConfig
	origFactory := sessionFactory
	defer func() { sessionFactory = origFactory }()
	sessionFactory = func(_ context.Context, _ neo4j.DriverWithContext, cfg neo4j.SessionConfig) session {
		gotAccessMode = cfg.AccessMode
		return ms
	}

	client := newTestClient(t)
	err := client.ClearDB(context.Background(), "mydb")
	if err != nil {
		t.Fatalf("ClearDB() unexpected error: %v", err)
	}
	if gotCypher != "MATCH (n {_db: $_db}) DETACH DELETE n" {
		t.Errorf("Cypher = %q, want 'MATCH (n {_db: $_db}) DETACH DELETE n'", gotCypher)
	}
	if gotParams["_db"] != "mydb" {
		t.Errorf("params[_db] = %v, want 'mydb'", gotParams["_db"])
	}
	if gotAccessMode != neo4j.AccessModeWrite {
		t.Errorf("AccessMode = %v, want Write", gotAccessMode)
	}
}

func TestClearDB_RunError(t *testing.T) {
	wantErr := errors.New("write failed")
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return nil, wantErr
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	err := client.ClearDB(context.Background(), "testdb")
	if err == nil {
		t.Fatal("ClearDB() should return error when Run fails")
	}
	if !strings.Contains(err.Error(), "clear db") {
		t.Errorf("error should contain 'clear db', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestClearDB_SessionClosed(t *testing.T) {
	closeCalled := false
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return &mockResult{}, nil
		},
		closeFn: func(_ context.Context) error {
			closeCalled = true
			return nil
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	_ = client.ClearDB(context.Background(), "default")
	if !closeCalled {
		t.Error("ClearDB() should call session.Close via defer")
	}
}

// ---------------------------------------------------------------------------
// TestListDBs — ListDBs 方法测试
// ---------------------------------------------------------------------------

func TestListDBs_Success(t *testing.T) {
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return &mockResult{
				records: []*neo4j.Record{
					{Keys: []string{"db"}, Values: []any{"default"}},
					{Keys: []string{"db"}, Values: []any{"snapshot-1"}},
					{Keys: []string{"db"}, Values: []any{"snapshot-2"}},
				},
			}, nil
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	dbs, err := client.ListDBs(context.Background())
	if err != nil {
		t.Fatalf("ListDBs() unexpected error: %v", err)
	}
	if len(dbs) != 3 {
		t.Fatalf("ListDBs() returned %d dbs, want 3", len(dbs))
	}
	expected := []string{"default", "snapshot-1", "snapshot-2"}
	for i, want := range expected {
		if dbs[i] != want {
			t.Errorf("dbs[%d] = %q, want %q", i, dbs[i], want)
		}
	}
}

func TestListDBs_Empty(t *testing.T) {
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return &mockResult{}, nil
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	dbs, err := client.ListDBs(context.Background())
	if err != nil {
		t.Fatalf("ListDBs() unexpected error: %v", err)
	}
	if len(dbs) != 0 {
		t.Errorf("ListDBs() returned %d dbs, want 0", len(dbs))
	}
}

func TestListDBs_RunError(t *testing.T) {
	wantErr := errors.New("query failed")
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return nil, wantErr
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	_, err := client.ListDBs(context.Background())
	if err == nil {
		t.Fatal("ListDBs() should return error when Run fails")
	}
	if !strings.Contains(err.Error(), "list dbs") {
		t.Errorf("error should contain 'list dbs', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestHasDB — HasDB 方法测试
// ---------------------------------------------------------------------------

func TestHasDB_Exists(t *testing.T) {
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return &mockResult{
				records: []*neo4j.Record{
					{Keys: []string{"exists"}, Values: []any{true}},
				},
			}, nil
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	exists, err := client.HasDB(context.Background(), "default")
	if err != nil {
		t.Fatalf("HasDB() unexpected error: %v", err)
	}
	if !exists {
		t.Error("HasDB() = false, want true")
	}
}

func TestHasDB_NotExists(t *testing.T) {
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return &mockResult{
				records: []*neo4j.Record{
					{Keys: []string{"exists"}, Values: []any{false}},
				},
			}, nil
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	exists, err := client.HasDB(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("HasDB() unexpected error: %v", err)
	}
	if exists {
		t.Error("HasDB() = true, want false")
	}
}

func TestHasDB_RunError(t *testing.T) {
	wantErr := errors.New("query failed")
	ms := &mockSession{
		runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
			return nil, wantErr
		},
	}
	withMockSessionFactory(t, ms)

	client := newTestClient(t)
	_, err := client.HasDB(context.Background(), "testdb")
	if err == nil {
		t.Fatal("HasDB() should return error when Run fails")
	}
	if !strings.Contains(err.Error(), "has db") {
		t.Errorf("error should contain 'has db', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}
