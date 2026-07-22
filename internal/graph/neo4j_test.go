package graph

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/auth"
	"gitlab.com/pml/network-digital-twin/internal/assembler"
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
	driverFactory = func(_ string, _ auth.TokenManager, _ ...func(*neo4j.Config)) (neo4j.DriverWithContext, error) { //nolint:staticcheck // neo4j.Config 将在 6.0 废弃
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
	gdb, err := NewNeo4jClient(testCfg())
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}
	return gdb.(*neo4jClient)
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

	gdb, err := NewNeo4jClient(testCfg())
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}
	client := gdb.(*neo4jClient)
	if client.defaultDB != "testdb" {
		t.Errorf("defaultDB = %q, want %q", client.defaultDB, "testdb")
	}
}

func TestNewNeo4jClient_DefaultDBEmpty(t *testing.T) {
	withMockDriver(t, &mockDriver{})

	cfg := testCfg()
	cfg.DefaultDB = ""

	gdb, err := NewNeo4jClient(cfg)
	if err != nil {
		t.Fatalf("NewNeo4jClient() unexpected error: %v", err)
	}
	client := gdb.(*neo4jClient)
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

// ---------------------------------------------------------------------------
// runCall — BulkCreate 多次 Run 调用捕获辅助结构体
// ---------------------------------------------------------------------------

// runCall 记录单次 session.Run 调用的 cypher 和 params。
type runCall struct {
	cypher string
	params map[string]any
}

// captureSessionFactory 替换 sessionFactory 为记录所有 Run 调用的闭包，
// 同时捕获 SessionConfig.AccessMode，测试结束时自动恢复。
func captureSessionFactory(t *testing.T, calls *[]runCall, accessMode *neo4j.AccessMode, runErr func(callIndex int) error) {
	t.Helper()
	orig := sessionFactory
	callIdx := 0
	sessionFactory = func(_ context.Context, _ neo4j.DriverWithContext, cfg neo4j.SessionConfig) session {
		if accessMode != nil {
			*accessMode = cfg.AccessMode
		}
		return &mockSession{
			runFn: func(_ context.Context, cypher string, params map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
				idx := callIdx
				callIdx++
				*calls = append(*calls, runCall{cypher: cypher, params: params})
				if runErr != nil {
					if err := runErr(idx); err != nil {
						return nil, err
					}
				}
				return &mockResult{}, nil
			},
		}
	}
	t.Cleanup(func() { sessionFactory = orig })
}

// ---------------------------------------------------------------------------
// TestBulkCreate — BulkCreate 方法测试
// ---------------------------------------------------------------------------

func TestBulkCreate_Success(t *testing.T) {
	var calls []runCall
	var accessMode neo4j.AccessMode
	captureSessionFactory(t, &calls, &accessMode, nil)

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}

	err := client.BulkCreate(context.Background(), "testdb", nodes, rels)
	if err != nil {
		t.Fatalf("BulkCreate() unexpected error: %v", err)
	}

	// 验证 AccessMode
	if accessMode != neo4j.AccessModeWrite {
		t.Errorf("AccessMode = %v, want Write", accessMode)
	}

	// 应有 2 次 Run 调用：1 次节点 + 1 次关系
	if len(calls) != 2 {
		t.Fatalf("expected 2 Run calls, got %d", len(calls))
	}

	// 验证节点 Cypher
	nodeCypher := calls[0].cypher
	if !strings.Contains(nodeCypher, "UNWIND $nodes AS n") {
		t.Errorf("node cypher should contain 'UNWIND $nodes AS n', got: %s", nodeCypher)
	}
	if !strings.Contains(nodeCypher, "CREATE (x:Device") {
		t.Errorf("node cypher should contain 'CREATE (x:Device', got: %s", nodeCypher)
	}
	if !strings.Contains(nodeCypher, "SET x += n") {
		t.Errorf("node cypher should contain 'SET x += n', got: %s", nodeCypher)
	}

	// 验证节点 params
	nodeParams := calls[0].params
	if nodeParams["_db"] != "testdb" {
		t.Errorf("node params[_db] = %v, want 'testdb'", nodeParams["_db"])
	}
	nodeData, ok := nodeParams["nodes"].([]map[string]any)
	if !ok || len(nodeData) != 1 {
		t.Fatalf("node params[nodes] should be []map[string]any with length 1, got: %v", nodeParams["nodes"])
	}
	if nodeData[0]["uri"] != "device:SN001" {
		t.Errorf("nodeData[0][uri] = %v, want 'device:SN001'", nodeData[0]["uri"])
	}
	if nodeData[0]["hostname"] != "r1" {
		t.Errorf("nodeData[0][hostname] = %v, want 'r1'", nodeData[0]["hostname"])
	}

	// 验证关系 Cypher
	relCypher := calls[1].cypher
	if !strings.Contains(relCypher, "UNWIND $rels AS r") {
		t.Errorf("rel cypher should contain 'UNWIND $rels AS r', got: %s", relCypher)
	}
	if !strings.Contains(relCypher, "MATCH (a {_db: $_db, uri: r.from})") {
		t.Errorf("rel cypher should contain 'MATCH (a {_db: $_db, uri: r.from})', got: %s", relCypher)
	}
	if !strings.Contains(relCypher, "CREATE (a)-[:HAS_INTERFACE]->(b)") {
		t.Errorf("rel cypher should contain 'CREATE (a)-[:HAS_INTERFACE]->(b)', got: %s", relCypher)
	}

	// 验证关系 params
	relParams := calls[1].params
	if relParams["_db"] != "testdb" {
		t.Errorf("rel params[_db] = %v, want 'testdb'", relParams["_db"])
	}
	relData, ok := relParams["rels"].([]map[string]any)
	if !ok || len(relData) != 1 {
		t.Fatalf("rel params[rels] should be []map[string]any with length 1, got: %v", relParams["rels"])
	}
	if relData[0]["from"] != "device:SN001" {
		t.Errorf("relData[0][from] = %v, want 'device:SN001'", relData[0]["from"])
	}
	if relData[0]["to"] != "iface:SN001_eth0" {
		t.Errorf("relData[0][to] = %v, want 'iface:SN001_eth0'", relData[0]["to"])
	}
}

func TestBulkCreate_EmptyNodes(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}

	err := client.BulkCreate(context.Background(), "testdb", nil, rels)
	if err != nil {
		t.Fatalf("BulkCreate() unexpected error: %v", err)
	}

	// 只有关系创建，应调用 1 次 Run
	if len(calls) != 1 {
		t.Fatalf("expected 1 Run call (rels only), got %d", len(calls))
	}
	if !strings.Contains(calls[0].cypher, "UNWIND $rels AS r") {
		t.Errorf("expected rel cypher, got: %s", calls[0].cypher)
	}
}

func TestBulkCreate_EmptyRels(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}

	err := client.BulkCreate(context.Background(), "testdb", nodes, nil)
	if err != nil {
		t.Fatalf("BulkCreate() unexpected error: %v", err)
	}

	// 只有节点创建，应调用 1 次 Run
	if len(calls) != 1 {
		t.Fatalf("expected 1 Run call (nodes only), got %d", len(calls))
	}
	if !strings.Contains(calls[0].cypher, "UNWIND $nodes AS n") {
		t.Errorf("expected node cypher, got: %s", calls[0].cypher)
	}
}

func TestBulkCreate_MultipleLabels(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
		{Labels: []string{"Device"}, URI: "device:SN002", Props: map[string]any{"hostname": "r2"}},
		{Labels: []string{"Interface"}, URI: "iface:SN001_eth0", Props: map[string]any{"status": "Up"}},
		{Labels: []string{"Interface"}, URI: "iface:SN001_eth1", Props: map[string]any{"status": "Down"}},
		{Labels: []string{"Interface"}, URI: "iface:SN002_eth0", Props: map[string]any{"status": "Up"}},
	}

	err := client.BulkCreate(context.Background(), "testdb", nodes, nil)
	if err != nil {
		t.Fatalf("BulkCreate() unexpected error: %v", err)
	}

	// 2 个 Label 应产生 2 次 Run 调用
	if len(calls) != 2 {
		t.Fatalf("expected 2 Run calls (Device + Interface), got %d", len(calls))
	}

	// 收集每次调用的 label 和节点数
	labelCounts := make(map[string]int)
	for _, call := range calls {
		if strings.Contains(call.cypher, ":Device") {
			nd := call.params["nodes"].([]map[string]any)
			labelCounts["Device"] = len(nd)
		}
		if strings.Contains(call.cypher, ":Interface") {
			nd := call.params["nodes"].([]map[string]any)
			labelCounts["Interface"] = len(nd)
		}
	}
	if labelCounts["Device"] != 2 {
		t.Errorf("Device group should have 2 nodes, got %d", labelCounts["Device"])
	}
	if labelCounts["Interface"] != 3 {
		t.Errorf("Interface group should have 3 nodes, got %d", labelCounts["Interface"])
	}
}

func TestBulkCreate_MultipleRelTypes(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
		{Type: "CONNECTS_TO", From: "iface:SN001_eth0", To: "iface:SN002_eth0"},
		{Type: "HAS_INTERFACE", From: "device:SN002", To: "iface:SN002_eth0"},
	}

	err := client.BulkCreate(context.Background(), "testdb", nil, rels)
	if err != nil {
		t.Fatalf("BulkCreate() unexpected error: %v", err)
	}

	// 2 个 RelType 应产生 2 次 Run 调用
	if len(calls) != 2 {
		t.Fatalf("expected 2 Run calls (HAS_INTERFACE + CONNECTS_TO), got %d", len(calls))
	}

	// 验证两种关系类型都出现
	foundHI := false
	foundCT := false
	for _, call := range calls {
		if strings.Contains(call.cypher, "[:HAS_INTERFACE]") {
			foundHI = true
			rd := call.params["rels"].([]map[string]any)
			if len(rd) != 2 {
				t.Errorf("HAS_INTERFACE group should have 2 rels, got %d", len(rd))
			}
		}
		if strings.Contains(call.cypher, "[:CONNECTS_TO]") {
			foundCT = true
			rd := call.params["rels"].([]map[string]any)
			if len(rd) != 1 {
				t.Errorf("CONNECTS_TO group should have 1 rel, got %d", len(rd))
			}
		}
	}
	if !foundHI {
		t.Error("expected HAS_INTERFACE rel cypher not found")
	}
	if !foundCT {
		t.Error("expected CONNECTS_TO rel cypher not found")
	}
}

func TestBulkCreate_NodeRunError(t *testing.T) {
	wantErr := errors.New("write failed")
	captureSessionFactory(t, &[]runCall{}, nil, func(callIndex int) error {
		return wantErr // 节点 Run 立即失败
	})

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}

	err := client.BulkCreate(context.Background(), "testdb", nodes, nil)
	if err == nil {
		t.Fatal("BulkCreate() should return error when node Run fails")
	}
	if !strings.Contains(err.Error(), "bulk create nodes") {
		t.Errorf("error should contain 'bulk create nodes', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestBulkCreate_RelRunError(t *testing.T) {
	wantErr := errors.New("rel write failed")
	captureSessionFactory(t, &[]runCall{}, nil, func(callIndex int) error {
		if callIndex > 0 { // 第二次调用（关系）失败
			return wantErr
		}
		return nil
	})

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}

	err := client.BulkCreate(context.Background(), "testdb", nodes, rels)
	if err == nil {
		t.Fatal("BulkCreate() should return error when rel Run fails")
	}
	if !strings.Contains(err.Error(), "bulk create rels") {
		t.Errorf("error should contain 'bulk create rels', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestBulkCreate_DBPropertyInjected(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
		{Labels: []string{"Interface"}, URI: "iface:SN001_eth0", Props: map[string]any{"status": "Up"}},
	}

	err := client.BulkCreate(context.Background(), "mydb", nodes, nil)
	if err != nil {
		t.Fatalf("BulkCreate() unexpected error: %v", err)
	}

	// 遍历所有节点 Run 调用，验证每个 node 都含 _db 和 uri
	for _, call := range calls {
		if !strings.Contains(call.cypher, "UNWIND $nodes") {
			continue
		}
		nd, ok := call.params["nodes"].([]map[string]any)
		if !ok {
			t.Fatalf("params[nodes] is not []map[string]any: %v", call.params["nodes"])
		}
		for i, n := range nd {
			if n["_db"] != "mydb" {
				t.Errorf("node[%d][_db] = %v, want 'mydb'", i, n["_db"])
			}
			if _, hasURI := n["uri"]; !hasURI {
				t.Errorf("node[%d] missing 'uri' key", i)
			}
		}
	}
}

func TestBulkCreate_NoMutateCallerProps(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	originalProps := map[string]any{"hostname": "r1"}
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: originalProps},
	}

	err := client.BulkCreate(context.Background(), "testdb", nodes, nil)
	if err != nil {
		t.Fatalf("BulkCreate() unexpected error: %v", err)
	}

	// 验证原始 Props 未被注入 _db 或 uri
	if _, hasDB := originalProps["_db"]; hasDB {
		t.Errorf("BulkCreate() should not mutate caller's Props, but original now contains _db: %v", originalProps)
	}
	if _, hasURI := originalProps["uri"]; hasURI {
		t.Errorf("BulkCreate() should not mutate caller's Props, but original now contains uri: %v", originalProps)
	}
}

// ---------------------------------------------------------------------------
// TestGroupNodesByLabels / TestGroupRelsByType — 辅助函数单元测试
// ---------------------------------------------------------------------------

func TestGroupNodesByLabels(t *testing.T) {
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001"},
		{Labels: []string{"Interface"}, URI: "iface:SN001_eth0"},
		{Labels: []string{"Device"}, URI: "device:SN002"},
	}

	groups := groupNodesByLabels(nodes)
	if len(groups) != 2 {
		t.Fatalf("groupNodesByLabels() returned %d groups, want 2", len(groups))
	}
	if len(groups["Device"]) != 2 {
		t.Errorf("Device group length = %d, want 2", len(groups["Device"]))
	}
	if len(groups["Interface"]) != 1 {
		t.Errorf("Interface group length = %d, want 1", len(groups["Interface"]))
	}
}

func TestGroupNodesByLabels_Empty(t *testing.T) {
	groups := groupNodesByLabels(nil)
	if len(groups) != 0 {
		t.Errorf("groupNodesByLabels(nil) returned %d groups, want 0", len(groups))
	}
}

func TestGroupRelsByType(t *testing.T) {
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
		{Type: "CONNECTS_TO", From: "iface:SN001_eth0", To: "iface:SN002_eth0"},
		{Type: "HAS_INTERFACE", From: "device:SN002", To: "iface:SN002_eth0"},
	}

	groups := groupRelsByType(rels)
	if len(groups) != 2 {
		t.Fatalf("groupRelsByType() returned %d groups, want 2", len(groups))
	}
	if len(groups["HAS_INTERFACE"]) != 2 {
		t.Errorf("HAS_INTERFACE group length = %d, want 2", len(groups["HAS_INTERFACE"]))
	}
	if len(groups["CONNECTS_TO"]) != 1 {
		t.Errorf("CONNECTS_TO group length = %d, want 1", len(groups["CONNECTS_TO"]))
	}
}

func TestGroupRelsByType_Empty(t *testing.T) {
	groups := groupRelsByType(nil)
	if len(groups) != 0 {
		t.Errorf("groupRelsByType(nil) returned %d groups, want 0", len(groups))
	}
}

// ---------------------------------------------------------------------------
// TestUpsert — Upsert 方法测试
// ---------------------------------------------------------------------------

func TestUpsert_Success(t *testing.T) {
	var calls []runCall
	var accessMode neo4j.AccessMode
	captureSessionFactory(t, &calls, &accessMode, nil)

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}

	err := client.Upsert(context.Background(), "testdb", nodes, rels)
	if err != nil {
		t.Fatalf("Upsert() unexpected error: %v", err)
	}

	// 验证 AccessMode
	if accessMode != neo4j.AccessModeWrite {
		t.Errorf("AccessMode = %v, want Write", accessMode)
	}

	// 应有 2 次 Run 调用：1 次节点 + 1 次关系
	if len(calls) != 2 {
		t.Fatalf("expected 2 Run calls, got %d", len(calls))
	}

	// 验证节点 Cypher
	nodeCypher := calls[0].cypher
	if !strings.Contains(nodeCypher, "UNWIND $nodes AS n") {
		t.Errorf("node cypher should contain 'UNWIND $nodes AS n', got: %s", nodeCypher)
	}
	if !strings.Contains(nodeCypher, "MERGE (x:Device") {
		t.Errorf("node cypher should contain 'MERGE (x:Device', got: %s", nodeCypher)
	}
	if !strings.Contains(nodeCypher, "SET x += n.props") {
		t.Errorf("node cypher should contain 'SET x += n.props', got: %s", nodeCypher)
	}
	if !strings.Contains(nodeCypher, "_db: $_db, uri: n.uri") {
		t.Errorf("node cypher should contain '_db: $_db, uri: n.uri', got: %s", nodeCypher)
	}

	// 验证节点 params
	nodeParams := calls[0].params
	if nodeParams["_db"] != "testdb" {
		t.Errorf("node params[_db] = %v, want 'testdb'", nodeParams["_db"])
	}
	nodeData, ok := nodeParams["nodes"].([]map[string]any)
	if !ok || len(nodeData) != 1 {
		t.Fatalf("node params[nodes] should be []map[string]any with length 1, got: %v", nodeParams["nodes"])
	}
	if nodeData[0]["uri"] != "device:SN001" {
		t.Errorf("nodeData[0][uri] = %v, want 'device:SN001'", nodeData[0]["uri"])
	}
	// 验证嵌套 props 结构
	props, ok := nodeData[0]["props"].(map[string]any)
	if !ok {
		t.Fatalf("nodeData[0][props] should be map[string]any, got: %T", nodeData[0]["props"])
	}
	if props["hostname"] != "r1" {
		t.Errorf("props[hostname] = %v, want 'r1'", props["hostname"])
	}
	if props["_db"] != "testdb" {
		t.Errorf("props[_db] = %v, want 'testdb'", props["_db"])
	}
	// props 不应包含 uri（MERGE 匹配键已设置）
	if _, hasURI := props["uri"]; hasURI {
		t.Errorf("props should not contain 'uri', but got: %v", props)
	}

	// 验证关系 Cypher
	relCypher := calls[1].cypher
	if !strings.Contains(relCypher, "UNWIND $rels AS r") {
		t.Errorf("rel cypher should contain 'UNWIND $rels AS r', got: %s", relCypher)
	}
	if !strings.Contains(relCypher, "MATCH (a {_db: $_db, uri: r.from})") {
		t.Errorf("rel cypher should contain 'MATCH (a {_db: $_db, uri: r.from})', got: %s", relCypher)
	}
	if !strings.Contains(relCypher, "MERGE (a)-[:HAS_INTERFACE]->(b)") {
		t.Errorf("rel cypher should contain 'MERGE (a)-[:HAS_INTERFACE]->(b)', got: %s", relCypher)
	}

	// 验证关系 params
	relParams := calls[1].params
	if relParams["_db"] != "testdb" {
		t.Errorf("rel params[_db] = %v, want 'testdb'", relParams["_db"])
	}
	relData, ok := relParams["rels"].([]map[string]any)
	if !ok || len(relData) != 1 {
		t.Fatalf("rel params[rels] should be []map[string]any with length 1, got: %v", relParams["rels"])
	}
	if relData[0]["from"] != "device:SN001" {
		t.Errorf("relData[0][from] = %v, want 'device:SN001'", relData[0]["from"])
	}
	if relData[0]["to"] != "iface:SN001_eth0" {
		t.Errorf("relData[0][to] = %v, want 'iface:SN001_eth0'", relData[0]["to"])
	}
}

func TestUpsert_EmptyNodes(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}

	err := client.Upsert(context.Background(), "testdb", nil, rels)
	if err != nil {
		t.Fatalf("Upsert() unexpected error: %v", err)
	}

	// 只有关系 Upsert，应调用 1 次 Run
	if len(calls) != 1 {
		t.Fatalf("expected 1 Run call (rels only), got %d", len(calls))
	}
	if !strings.Contains(calls[0].cypher, "UNWIND $rels AS r") {
		t.Errorf("expected rel cypher, got: %s", calls[0].cypher)
	}
	if !strings.Contains(calls[0].cypher, "MERGE") {
		t.Errorf("rel cypher should contain MERGE, got: %s", calls[0].cypher)
	}
}

func TestUpsert_EmptyRels(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}

	err := client.Upsert(context.Background(), "testdb", nodes, nil)
	if err != nil {
		t.Fatalf("Upsert() unexpected error: %v", err)
	}

	// 只有节点 Upsert，应调用 1 次 Run
	if len(calls) != 1 {
		t.Fatalf("expected 1 Run call (nodes only), got %d", len(calls))
	}
	if !strings.Contains(calls[0].cypher, "UNWIND $nodes AS n") {
		t.Errorf("expected node cypher, got: %s", calls[0].cypher)
	}
	if !strings.Contains(calls[0].cypher, "MERGE") {
		t.Errorf("node cypher should contain MERGE, got: %s", calls[0].cypher)
	}
}

func TestUpsert_MultipleLabels(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
		{Labels: []string{"Device"}, URI: "device:SN002", Props: map[string]any{"hostname": "r2"}},
		{Labels: []string{"Interface"}, URI: "iface:SN001_eth0", Props: map[string]any{"status": "Up"}},
		{Labels: []string{"Interface"}, URI: "iface:SN001_eth1", Props: map[string]any{"status": "Down"}},
		{Labels: []string{"Interface"}, URI: "iface:SN002_eth0", Props: map[string]any{"status": "Up"}},
	}

	err := client.Upsert(context.Background(), "testdb", nodes, nil)
	if err != nil {
		t.Fatalf("Upsert() unexpected error: %v", err)
	}

	// 2 个 Label 应产生 2 次 Run 调用
	if len(calls) != 2 {
		t.Fatalf("expected 2 Run calls (Device + Interface), got %d", len(calls))
	}

	// 收集每次调用的 label 和节点数
	labelCounts := make(map[string]int)
	for _, call := range calls {
		if strings.Contains(call.cypher, ":Device") {
			nd := call.params["nodes"].([]map[string]any)
			labelCounts["Device"] = len(nd)
		}
		if strings.Contains(call.cypher, ":Interface") {
			nd := call.params["nodes"].([]map[string]any)
			labelCounts["Interface"] = len(nd)
		}
	}
	if labelCounts["Device"] != 2 {
		t.Errorf("Device group should have 2 nodes, got %d", labelCounts["Device"])
	}
	if labelCounts["Interface"] != 3 {
		t.Errorf("Interface group should have 3 nodes, got %d", labelCounts["Interface"])
	}
}

func TestUpsert_MultipleRelTypes(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
		{Type: "CONNECTS_TO", From: "iface:SN001_eth0", To: "iface:SN002_eth0"},
		{Type: "HAS_INTERFACE", From: "device:SN002", To: "iface:SN002_eth0"},
	}

	err := client.Upsert(context.Background(), "testdb", nil, rels)
	if err != nil {
		t.Fatalf("Upsert() unexpected error: %v", err)
	}

	// 2 个 RelType 应产生 2 次 Run 调用
	if len(calls) != 2 {
		t.Fatalf("expected 2 Run calls (HAS_INTERFACE + CONNECTS_TO), got %d", len(calls))
	}

	// 验证两种关系类型都出现
	foundHI := false
	foundCT := false
	for _, call := range calls {
		if strings.Contains(call.cypher, "[:HAS_INTERFACE]") {
			foundHI = true
			rd := call.params["rels"].([]map[string]any)
			if len(rd) != 2 {
				t.Errorf("HAS_INTERFACE group should have 2 rels, got %d", len(rd))
			}
		}
		if strings.Contains(call.cypher, "[:CONNECTS_TO]") {
			foundCT = true
			rd := call.params["rels"].([]map[string]any)
			if len(rd) != 1 {
				t.Errorf("CONNECTS_TO group should have 1 rel, got %d", len(rd))
			}
		}
	}
	if !foundHI {
		t.Error("expected HAS_INTERFACE rel cypher not found")
	}
	if !foundCT {
		t.Error("expected CONNECTS_TO rel cypher not found")
	}
}

func TestUpsert_NodeRunError(t *testing.T) {
	wantErr := errors.New("write failed")
	captureSessionFactory(t, &[]runCall{}, nil, func(callIndex int) error {
		return wantErr // 节点 Run 立即失败
	})

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}

	err := client.Upsert(context.Background(), "testdb", nodes, nil)
	if err == nil {
		t.Fatal("Upsert() should return error when node Run fails")
	}
	if !strings.Contains(err.Error(), "upsert nodes") {
		t.Errorf("error should contain 'upsert nodes', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestUpsert_RelRunError(t *testing.T) {
	wantErr := errors.New("rel write failed")
	captureSessionFactory(t, &[]runCall{}, nil, func(callIndex int) error {
		if callIndex > 0 { // 第二次调用（关系）失败
			return wantErr
		}
		return nil
	})

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}

	err := client.Upsert(context.Background(), "testdb", nodes, rels)
	if err == nil {
		t.Fatal("Upsert() should return error when rel Run fails")
	}
	if !strings.Contains(err.Error(), "upsert rels") {
		t.Errorf("error should contain 'upsert rels', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestUpsert_DBPropertyInjected(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
		{Labels: []string{"Interface"}, URI: "iface:SN001_eth0", Props: map[string]any{"status": "Up"}},
	}

	err := client.Upsert(context.Background(), "mydb", nodes, nil)
	if err != nil {
		t.Fatalf("Upsert() unexpected error: %v", err)
	}

	// 遍历所有节点 Run 调用，验证每个 node 都含 _db 和 uri
	for _, call := range calls {
		if !strings.Contains(call.cypher, "UNWIND $nodes") {
			continue
		}
		nd, ok := call.params["nodes"].([]map[string]any)
		if !ok {
			t.Fatalf("params[nodes] is not []map[string]any: %v", call.params["nodes"])
		}
		for i, n := range nd {
			// 顶层应含 uri
			if _, hasURI := n["uri"]; !hasURI {
				t.Errorf("node[%d] missing 'uri' key at top level", i)
			}
			// props 应含 _db
			props, ok := n["props"].(map[string]any)
			if !ok {
				t.Errorf("node[%d][props] is not map[string]any", i)
				continue
			}
			if props["_db"] != "mydb" {
				t.Errorf("node[%d].props[_db] = %v, want 'mydb'", i, props["_db"])
			}
		}
	}
}

func TestUpsert_NoMutateCallerProps(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	originalProps := map[string]any{"hostname": "r1"}
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: originalProps},
	}

	err := client.Upsert(context.Background(), "testdb", nodes, nil)
	if err != nil {
		t.Fatalf("Upsert() unexpected error: %v", err)
	}

	// 验证原始 Props 未被注入 _db 或 uri
	if _, hasDB := originalProps["_db"]; hasDB {
		t.Errorf("Upsert() should not mutate caller's Props, but original now contains _db: %v", originalProps)
	}
	if _, hasURI := originalProps["uri"]; hasURI {
		t.Errorf("Upsert() should not mutate caller's Props, but original now contains uri: %v", originalProps)
	}
	if originalProps["hostname"] != "r1" {
		t.Errorf("original map modified: hostname = %v, want 'r1'", originalProps["hostname"])
	}
}

func TestUpsert_MERGEsemantics(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}

	err := client.Upsert(context.Background(), "testdb", nodes, rels)
	if err != nil {
		t.Fatalf("Upsert() unexpected error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 Run calls, got %d", len(calls))
	}

	// 验证节点 Cypher 使用 MERGE 而非 CREATE
	nodeCypher := calls[0].cypher
	if !strings.Contains(nodeCypher, "MERGE") {
		t.Errorf("node cypher should contain MERGE, got: %s", nodeCypher)
	}
	if strings.Contains(nodeCypher, "CREATE") {
		t.Errorf("node cypher should NOT contain CREATE, got: %s", nodeCypher)
	}
	// 验证 MERGE 匹配键
	if !strings.Contains(nodeCypher, "_db: $_db, uri: n.uri") {
		t.Errorf("node cypher MERGE key should be {_db: $_db, uri: n.uri}, got: %s", nodeCypher)
	}
	// 验证 SET 使用 n.props 而非 n
	if !strings.Contains(nodeCypher, "SET x += n.props") {
		t.Errorf("node cypher should use 'SET x += n.props', got: %s", nodeCypher)
	}

	// 验证关系 Cypher 使用 MERGE 而非 CREATE
	relCypher := calls[1].cypher
	if !strings.Contains(relCypher, "MERGE") {
		t.Errorf("rel cypher should contain MERGE, got: %s", relCypher)
	}
	if strings.Contains(relCypher, "CREATE") {
		t.Errorf("rel cypher should NOT contain CREATE, got: %s", relCypher)
	}

	// 验证 params 中 nodes 是嵌套结构（非扁平 map）
	nodeData, ok := calls[0].params["nodes"].([]map[string]any)
	if !ok || len(nodeData) != 1 {
		t.Fatalf("params[nodes] should be []map[string]any with length 1, got: %v", calls[0].params["nodes"])
	}
	// 顶层应有 uri 和 props 两个键
	if _, hasURI := nodeData[0]["uri"]; !hasURI {
		t.Errorf("nodeData[0] should have 'uri' at top level, got: %v", nodeData[0])
	}
	if _, hasProps := nodeData[0]["props"]; !hasProps {
		t.Errorf("nodeData[0] should have 'props' at top level, got: %v", nodeData[0])
	}
}

// ---------------------------------------------------------------------------
// cloneDBCall — CloneDB 多步 session 捕获辅助结构体
// ---------------------------------------------------------------------------

// cloneDBCall 记录 CloneDB 执行过程中每次 Run 调用的 cypher 和 params。
type cloneDBCall struct {
	cypher string
	params map[string]any
}

// cloneSessionFactory 为 CloneDB 测试构建 session 工厂。
// 前 queryCount 次 Run 调用返回 queryResults 中对应的 mockResult（用于 Query 读阶段），
// 后续 Run 调用记录到 writeCalls 并根据 writeErrFn 决定是否返回错误。
type cloneSessionFactory struct {
	queryCount   int
	queryResults []*mockResult
	writeCalls   *[]cloneDBCall
	writeErrFn   func(callIndex int) error
}

// install 替换 sessionFactory 为测试专用工厂，测试结束自动恢复。
func (f *cloneSessionFactory) install(t *testing.T) {
	t.Helper()
	orig := sessionFactory
	runIdx := 0
	sessionFactory = func(_ context.Context, _ neo4j.DriverWithContext, _ neo4j.SessionConfig) session {
		return &mockSession{
			runFn: func(_ context.Context, cypher string, params map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
				idx := runIdx
				runIdx++
				if idx < f.queryCount {
					return f.queryResults[idx], nil
				}
				writeIdx := idx - f.queryCount
				*f.writeCalls = append(*f.writeCalls, cloneDBCall{cypher: cypher, params: params})
				if f.writeErrFn != nil {
					if err := f.writeErrFn(writeIdx); err != nil {
						return nil, err
					}
				}
				return &mockResult{}, nil
			},
		}
	}
	t.Cleanup(func() { sessionFactory = orig })
}

// ---------------------------------------------------------------------------
// TestDeleteByURIs — DeleteByURIs 方法测试
// ---------------------------------------------------------------------------

func TestDeleteByURIs_Success(t *testing.T) {
	var calls []runCall
	var accessMode neo4j.AccessMode
	captureSessionFactory(t, &calls, &accessMode, nil)

	client := newTestClient(t)
	err := client.DeleteByURIs(context.Background(), "testdb", []string{"device:SN001", "device:SN002"})
	if err != nil {
		t.Fatalf("DeleteByURIs() unexpected error: %v", err)
	}

	// 应有 1 次 Run 调用
	if len(calls) != 1 {
		t.Fatalf("expected 1 Run call, got %d", len(calls))
	}

	// 验证 AccessMode
	if accessMode != neo4j.AccessModeWrite {
		t.Errorf("AccessMode = %v, want Write", accessMode)
	}

	// 验证 Cypher
	cypher := calls[0].cypher
	if !strings.Contains(cypher, "UNWIND $uris AS uri") {
		t.Errorf("cypher should contain 'UNWIND $uris AS uri', got: %s", cypher)
	}
	if !strings.Contains(cypher, "DETACH DELETE n") {
		t.Errorf("cypher should contain 'DETACH DELETE n', got: %s", cypher)
	}
	if !strings.Contains(cypher, "{_db: $_db, uri: uri}") {
		t.Errorf("cypher should contain '{_db: $_db, uri: uri}', got: %s", cypher)
	}

	// 验证 params
	if calls[0].params["_db"] != "testdb" {
		t.Errorf("params[_db] = %v, want 'testdb'", calls[0].params["_db"])
	}
	uris, ok := calls[0].params["uris"].([]string)
	if !ok {
		t.Fatalf("params[uris] should be []string, got: %T", calls[0].params["uris"])
	}
	if len(uris) != 2 || uris[0] != "device:SN001" || uris[1] != "device:SN002" {
		t.Errorf("params[uris] = %v, want [device:SN001 device:SN002]", uris)
	}
}

func TestDeleteByURIs_EmptyURIs(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	err := client.DeleteByURIs(context.Background(), "testdb", []string{})
	if err != nil {
		t.Fatalf("DeleteByURIs() unexpected error: %v", err)
	}

	// 空 URI 列表仍执行 1 次 Run（UNWIND 空列表 → 无操作）
	if len(calls) != 1 {
		t.Fatalf("expected 1 Run call, got %d", len(calls))
	}
}

func TestDeleteByURIs_WriteAccessMode(t *testing.T) {
	var accessMode neo4j.AccessMode
	orig := sessionFactory
	sessionFactory = func(_ context.Context, _ neo4j.DriverWithContext, cfg neo4j.SessionConfig) session {
		accessMode = cfg.AccessMode
		return &mockSession{}
	}
	t.Cleanup(func() { sessionFactory = orig })

	client := newTestClient(t)
	_ = client.DeleteByURIs(context.Background(), "testdb", []string{"device:SN001"})

	if accessMode != neo4j.AccessModeWrite {
		t.Errorf("AccessMode = %v, want Write", accessMode)
	}
}

func TestDeleteByURIs_RunError(t *testing.T) {
	wantErr := errors.New("write failed")
	captureSessionFactory(t, &[]runCall{}, nil, func(callIndex int) error {
		return wantErr
	})

	client := newTestClient(t)
	err := client.DeleteByURIs(context.Background(), "testdb", []string{"device:SN001"})
	if err == nil {
		t.Fatal("DeleteByURIs() should return error when Run fails")
	}
	if !strings.Contains(err.Error(), "delete by uris") {
		t.Errorf("error should contain 'delete by uris', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestDeleteByURIs_DETACHsemantics(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	_ = client.DeleteByURIs(context.Background(), "testdb", []string{"device:SN001"})

	if len(calls) != 1 {
		t.Fatalf("expected 1 Run call, got %d", len(calls))
	}
	cypher := calls[0].cypher
	// 必须含 DETACH DELETE
	if !strings.Contains(cypher, "DETACH DELETE") {
		t.Errorf("cypher should contain 'DETACH DELETE', got: %s", cypher)
	}
	// 不应含孤立的 DELETE n（即不含 DETACH DELETE n 但含 DELETE n）
	// 通过检查 DELETE n 前面是否有 DETACH 来判断
	if !strings.Contains(cypher, "DETACH DELETE n") {
		t.Errorf("cypher should contain 'DETACH DELETE n' (not plain DELETE), got: %s", cypher)
	}
}

// ---------------------------------------------------------------------------
// TestDeleteRelations — DeleteRelations 方法测试
// ---------------------------------------------------------------------------

func TestDeleteRelations_Success(t *testing.T) {
	var calls []runCall
	var accessMode neo4j.AccessMode
	captureSessionFactory(t, &calls, &accessMode, nil)

	client := newTestClient(t)
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}

	err := client.DeleteRelations(context.Background(), "testdb", rels)
	if err != nil {
		t.Fatalf("DeleteRelations() unexpected error: %v", err)
	}

	if accessMode != neo4j.AccessModeWrite {
		t.Errorf("AccessMode = %v, want Write", accessMode)
	}

	if len(calls) != 1 {
		t.Fatalf("expected 1 Run call, got %d", len(calls))
	}

	// 验证 Cypher
	cypher := calls[0].cypher
	if !strings.Contains(cypher, "UNWIND $rels AS r") {
		t.Errorf("cypher should contain 'UNWIND $rels AS r', got: %s", cypher)
	}
	if !strings.Contains(cypher, "MATCH (a {_db: $_db, uri: r.from})") {
		t.Errorf("cypher should contain 'MATCH (a {_db: $_db, uri: r.from})', got: %s", cypher)
	}
	if !strings.Contains(cypher, "-[x:HAS_INTERFACE]->") {
		t.Errorf("cypher should contain '-[x:HAS_INTERFACE]->', got: %s", cypher)
	}
	if !strings.Contains(cypher, "(b {_db: $_db, uri: r.to})") {
		t.Errorf("cypher should contain '(b {_db: $_db, uri: r.to})', got: %s", cypher)
	}
	if !strings.Contains(cypher, "DELETE x") {
		t.Errorf("cypher should contain 'DELETE x', got: %s", cypher)
	}

	// 验证 params
	if calls[0].params["_db"] != "testdb" {
		t.Errorf("params[_db] = %v, want 'testdb'", calls[0].params["_db"])
	}
	relData, ok := calls[0].params["rels"].([]map[string]any)
	if !ok || len(relData) != 1 {
		t.Fatalf("params[rels] should be []map[string]any with length 1, got: %v", calls[0].params["rels"])
	}
	if relData[0]["from"] != "device:SN001" {
		t.Errorf("relData[0][from] = %v, want 'device:SN001'", relData[0]["from"])
	}
	if relData[0]["to"] != "iface:SN001_eth0" {
		t.Errorf("relData[0][to] = %v, want 'iface:SN001_eth0'", relData[0]["to"])
	}
}

func TestDeleteRelations_MultipleRelTypes(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
		{Type: "CONNECTS_TO", From: "iface:SN001_eth0", To: "iface:SN002_eth0"},
		{Type: "HAS_INTERFACE", From: "device:SN002", To: "iface:SN002_eth0"},
	}

	err := client.DeleteRelations(context.Background(), "testdb", rels)
	if err != nil {
		t.Fatalf("DeleteRelations() unexpected error: %v", err)
	}

	// 2 个 RelType 应产生 2 次 Run 调用
	if len(calls) != 2 {
		t.Fatalf("expected 2 Run calls (HAS_INTERFACE + CONNECTS_TO), got %d", len(calls))
	}

	foundHI := false
	foundCT := false
	for _, call := range calls {
		if strings.Contains(call.cypher, "-[x:HAS_INTERFACE]->") {
			foundHI = true
			rd := call.params["rels"].([]map[string]any)
			if len(rd) != 2 {
				t.Errorf("HAS_INTERFACE group should have 2 rels, got %d", len(rd))
			}
		}
		if strings.Contains(call.cypher, "-[x:CONNECTS_TO]->") {
			foundCT = true
			rd := call.params["rels"].([]map[string]any)
			if len(rd) != 1 {
				t.Errorf("CONNECTS_TO group should have 1 rel, got %d", len(rd))
			}
		}
	}
	if !foundHI {
		t.Error("expected HAS_INTERFACE rel cypher not found")
	}
	if !foundCT {
		t.Error("expected CONNECTS_TO rel cypher not found")
	}
}

func TestDeleteRelations_EmptyRels(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	err := client.DeleteRelations(context.Background(), "testdb", nil)
	if err != nil {
		t.Fatalf("DeleteRelations() unexpected error: %v", err)
	}

	// 空关系列表不执行任何 Run
	if len(calls) != 0 {
		t.Fatalf("expected 0 Run calls for empty rels, got %d", len(calls))
	}
}

func TestDeleteRelations_RunError(t *testing.T) {
	wantErr := errors.New("delete failed")
	captureSessionFactory(t, &[]runCall{}, nil, func(callIndex int) error {
		return wantErr
	})

	client := newTestClient(t)
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}

	err := client.DeleteRelations(context.Background(), "testdb", rels)
	if err == nil {
		t.Fatal("DeleteRelations() should return error when Run fails")
	}
	if !strings.Contains(err.Error(), "delete rels") {
		t.Errorf("error should contain 'delete rels', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestCloneDB — CloneDB 方法测试
// ---------------------------------------------------------------------------

func TestCloneDB_Success(t *testing.T) {
	var writeCalls []cloneDBCall
	f := &cloneSessionFactory{
		queryCount: 2,
		queryResults: []*mockResult{
			{
				records: []*neo4j.Record{
					{Keys: []string{"labels", "uri", "props"}, Values: []any{[]string{"Device"}, "device:SN001", map[string]any{"hostname": "r1", "_db": "snap1"}}},
					{Keys: []string{"labels", "uri", "props"}, Values: []any{[]string{"Interface"}, "iface:eth0", map[string]any{"status": "Up", "_db": "snap1"}}},
				},
			},
			{
				records: []*neo4j.Record{
					{Keys: []string{"type", "from", "to"}, Values: []any{"HAS_INTERFACE", "device:SN001", "iface:eth0"}},
				},
			},
		},
		writeCalls: &writeCalls,
	}
	f.install(t)

	client := newTestClient(t)
	err := client.CloneDB(context.Background(), "snap1", "default")
	if err != nil {
		t.Fatalf("CloneDB() unexpected error: %v", err)
	}

	// 写入阶段应有 2 次 Run：1 节点 + 1 关系（2 个不同 Label 可能 2 次节点写入）
	if len(writeCalls) < 2 {
		t.Fatalf("expected at least 2 write calls (2 node labels + 1 rel), got %d", len(writeCalls))
	}

	// 验证节点写入 Cypher
	foundDevice := false
	foundIface := false
	foundRel := false
	for _, call := range writeCalls {
		if strings.Contains(call.cypher, ":Device") {
			foundDevice = true
			if !strings.Contains(call.cypher, "CREATE (x:Device") {
				t.Errorf("device cypher should contain CREATE, got: %s", call.cypher)
			}
			if !strings.Contains(call.cypher, "_db: $to") {
				t.Errorf("cypher should use $to for _db, got: %s", call.cypher)
			}
			if call.params["to"] != "default" {
				t.Errorf("params[to] = %v, want 'default'", call.params["to"])
			}
		}
		if strings.Contains(call.cypher, ":Interface") {
			foundIface = true
		}
		if strings.Contains(call.cypher, "[:HAS_INTERFACE]") {
			foundRel = true
			if !strings.Contains(call.cypher, "MATCH (a {_db: $to") {
				t.Errorf("rel cypher should use $to for _db, got: %s", call.cypher)
			}
			if !strings.Contains(call.cypher, "CREATE (a)-[:HAS_INTERFACE]->(b)") {
				t.Errorf("rel cypher should CREATE relation, got: %s", call.cypher)
			}
		}
	}
	if !foundDevice {
		t.Error("expected Device node write cypher not found")
	}
	if !foundIface {
		t.Error("expected Interface node write cypher not found")
	}
	if !foundRel {
		t.Error("expected HAS_INTERFACE rel write cypher not found")
	}
}

func TestCloneDB_EmptySource(t *testing.T) {
	var writeCalls []cloneDBCall
	f := &cloneSessionFactory{
		queryCount: 2,
		queryResults: []*mockResult{
			{}, // 空节点
			{}, // 空关系
		},
		writeCalls: &writeCalls,
	}
	f.install(t)

	client := newTestClient(t)
	err := client.CloneDB(context.Background(), "empty-snap", "default")
	if err != nil {
		t.Fatalf("CloneDB() unexpected error: %v", err)
	}

	// 源 DB 无数据，不应有任何写入
	if len(writeCalls) != 0 {
		t.Errorf("expected 0 write calls for empty source, got %d", len(writeCalls))
	}
}

func TestCloneDB_NodeQueryError(t *testing.T) {
	wantErr := errors.New("query failed")
	f := &cloneSessionFactory{
		queryCount: 2,
		queryResults: []*mockResult{
			{err: wantErr}, // 节点查询失败
		},
		writeCalls: &[]cloneDBCall{},
	}
	f.install(t)

	client := newTestClient(t)
	err := client.CloneDB(context.Background(), "snap1", "default")
	if err == nil {
		t.Fatal("CloneDB() should return error when node query fails")
	}
	if !strings.Contains(err.Error(), "clone db query nodes") {
		t.Errorf("error should contain 'clone db query nodes', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestCloneDB_RelQueryError(t *testing.T) {
	wantErr := errors.New("rel query failed")
	f := &cloneSessionFactory{
		queryCount: 2,
		queryResults: []*mockResult{
			{records: []*neo4j.Record{}}, // 节点查询成功（空）
			{err: wantErr},               // 关系查询失败
		},
		writeCalls: &[]cloneDBCall{},
	}
	f.install(t)

	client := newTestClient(t)
	err := client.CloneDB(context.Background(), "snap1", "default")
	if err == nil {
		t.Fatal("CloneDB() should return error when rel query fails")
	}
	if !strings.Contains(err.Error(), "clone db query rels") {
		t.Errorf("error should contain 'clone db query rels', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestCloneDB_NodeWriteError(t *testing.T) {
	wantErr := errors.New("write failed")
	f := &cloneSessionFactory{
		queryCount: 2,
		queryResults: []*mockResult{
			{
				records: []*neo4j.Record{
					{Keys: []string{"labels", "uri", "props"}, Values: []any{[]string{"Device"}, "device:SN001", map[string]any{"hostname": "r1"}}},
				},
			},
			{records: []*neo4j.Record{}}, // 空关系
		},
		writeCalls: &[]cloneDBCall{},
		writeErrFn: func(callIndex int) error {
			return wantErr
		},
	}
	f.install(t)

	client := newTestClient(t)
	err := client.CloneDB(context.Background(), "snap1", "default")
	if err == nil {
		t.Fatal("CloneDB() should return error when node write fails")
	}
	if !strings.Contains(err.Error(), "clone db create nodes") {
		t.Errorf("error should contain 'clone db create nodes', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestCloneDB_DBPropertyOverride(t *testing.T) {
	var writeCalls []cloneDBCall
	f := &cloneSessionFactory{
		queryCount: 2,
		queryResults: []*mockResult{
			{
				records: []*neo4j.Record{
					{Keys: []string{"labels", "uri", "props"}, Values: []any{[]string{"Device"}, "device:SN001", map[string]any{"hostname": "r1", "_db": "snap1"}}},
				},
			},
			{records: []*neo4j.Record{}}, // 空关系
		},
		writeCalls: &writeCalls,
	}
	f.install(t)

	client := newTestClient(t)
	_ = client.CloneDB(context.Background(), "snap1", "default")

	// 验证写入 params 使用 $to 而非 $from
	for _, call := range writeCalls {
		if !strings.Contains(call.cypher, "UNWIND $nodes") {
			continue
		}
		if call.params["to"] != "default" {
			t.Errorf("params[to] = %v, want 'default'", call.params["to"])
		}
		// 验证 Cypher 使用 $to 设置 _db
		if !strings.Contains(call.cypher, "_db: $to") {
			t.Errorf("cypher should use $to for _db override, got: %s", call.cypher)
		}
		// nodeData 中的 props 的 _db 应该是 "default"
		nd, ok := call.params["nodes"].([]map[string]any)
		if !ok || len(nd) == 0 {
			continue
		}
		props, ok := nd[0]["props"].(map[string]any)
		if ok {
			if props["_db"] != "default" {
				t.Errorf("node props[_db] = %v, want 'default' (overridden from source)", props["_db"])
			}
		}
	}
}

func TestCloneDB_SessionClosed(t *testing.T) {
	closeCalled := false
	orig := sessionFactory
	callIdx := 0
	sessionFactory = func(_ context.Context, _ neo4j.DriverWithContext, _ neo4j.SessionConfig) session {
		idx := callIdx
		callIdx++
		return &mockSession{
			runFn: func(_ context.Context, _ string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (result, error) {
				if idx < 2 {
					return &mockResult{}, nil // Query 阶段返回空
				}
				return &mockResult{}, nil
			},
			closeFn: func(_ context.Context) error {
				if idx >= 2 { // 写阶段的 session
					closeCalled = true
				}
				return nil
			},
		}
	}
	t.Cleanup(func() { sessionFactory = orig })

	client := newTestClient(t)
	_ = client.CloneDB(context.Background(), "snap1", "default")

	if !closeCalled {
		t.Error("CloneDB() should call session.Close via defer")
	}
}

// ---------------------------------------------------------------------------
// TestBuildCypher — BuildCypher 方法测试
// ---------------------------------------------------------------------------

func TestBuildCypher_Create(t *testing.T) {
	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}

	cypher, params := client.BuildCypher("create", "testdb", nodes, nil, nil)

	if !strings.Contains(cypher, "UNWIND $nodes_Device AS n") {
		t.Errorf("cypher should contain 'UNWIND $nodes_Device AS n', got: %s", cypher)
	}
	if !strings.Contains(cypher, "CREATE (x:Device") {
		t.Errorf("cypher should contain 'CREATE (x:Device', got: %s", cypher)
	}
	if !strings.Contains(cypher, "_db: $_db") {
		t.Errorf("cypher should contain '_db: $_db', got: %s", cypher)
	}
	if params["_db"] != "testdb" {
		t.Errorf("params[_db] = %v, want 'testdb'", params["_db"])
	}
	if _, ok := params["nodes_Device"]; !ok {
		t.Errorf("params should contain 'nodes_Device' key, got: %v", params)
	}
}

func TestBuildCypher_Upsert(t *testing.T) {
	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}

	cypher, params := client.BuildCypher("upsert", "testdb", nodes, nil, nil)

	if !strings.Contains(cypher, "MERGE (x:Device") {
		t.Errorf("cypher should contain 'MERGE (x:Device', got: %s", cypher)
	}
	if !strings.Contains(cypher, "SET x += n.props") {
		t.Errorf("cypher should contain 'SET x += n.props', got: %s", cypher)
	}
	if params["_db"] != "testdb" {
		t.Errorf("params[_db] = %v, want 'testdb'", params["_db"])
	}
	if _, ok := params["nodes_Device"]; !ok {
		t.Errorf("params should contain 'nodes_Device' key, got: %v", params)
	}
}

func TestBuildCypher_Delete(t *testing.T) {
	client := newTestClient(t)
	uris := []string{"device:SN001", "device:SN002"}

	cypher, params := client.BuildCypher("delete", "testdb", nil, nil, uris)

	if !strings.Contains(cypher, "UNWIND $uris AS uri") {
		t.Errorf("cypher should contain 'UNWIND $uris AS uri', got: %s", cypher)
	}
	if !strings.Contains(cypher, "DETACH DELETE n") {
		t.Errorf("cypher should contain 'DETACH DELETE n', got: %s", cypher)
	}
	if params["_db"] != "testdb" {
		t.Errorf("params[_db] = %v, want 'testdb'", params["_db"])
	}
	pUris, ok := params["uris"].([]string)
	if !ok || len(pUris) != 2 {
		t.Errorf("params[uris] should be []string with length 2, got: %v", params["uris"])
	}
}

func TestBuildCypher_DeleteRelations(t *testing.T) {
	client := newTestClient(t)
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}

	cypher, params := client.BuildCypher("delete_relations", "testdb", nil, rels, nil)

	if !strings.Contains(cypher, "UNWIND $rels_HAS_INTERFACE AS r") {
		t.Errorf("cypher should contain 'UNWIND $rels_HAS_INTERFACE AS r', got: %s", cypher)
	}
	if !strings.Contains(cypher, "-[x:HAS_INTERFACE]->") {
		t.Errorf("cypher should contain '-[x:HAS_INTERFACE]->', got: %s", cypher)
	}
	if !strings.Contains(cypher, "DELETE x") {
		t.Errorf("cypher should contain 'DELETE x', got: %s", cypher)
	}
	if params["_db"] != "testdb" {
		t.Errorf("params[_db] = %v, want 'testdb'", params["_db"])
	}
	if _, ok := params["rels_HAS_INTERFACE"]; !ok {
		t.Errorf("params should contain 'rels_HAS_INTERFACE' key, got: %v", params)
	}
}

func TestBuildCypher_UnknownAction(t *testing.T) {
	client := newTestClient(t)

	cypher, params := client.BuildCypher("unknown", "testdb", nil, nil, nil)

	if cypher != "" {
		t.Errorf("cypher should be empty for unknown action, got: %s", cypher)
	}
	if params["_db"] != "testdb" {
		t.Errorf("params[_db] = %v, want 'testdb'", params["_db"])
	}
	// 仅含 _db
	if len(params) != 1 {
		t.Errorf("params should only contain _db, got: %v", params)
	}
}

func TestBuildCypher_MultipleLabels(t *testing.T) {
	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
		{Labels: []string{"Interface"}, URI: "iface:SN001_eth0", Props: map[string]any{"status": "Up"}},
	}

	cypher, params := client.BuildCypher("create", "testdb", nodes, nil, nil)

	// 多 Label 应产生多条 Cypher 语句，用 ;\n 分隔
	if !strings.Contains(cypher, ";\n") {
		t.Errorf("cypher should contain ';\\n' separator for multiple labels, got: %s", cypher)
	}
	if !strings.Contains(cypher, ":Device") {
		t.Errorf("cypher should contain ':Device', got: %s", cypher)
	}
	if !strings.Contains(cypher, ":Interface") {
		t.Errorf("cypher should contain ':Interface', got: %s", cypher)
	}
	if _, ok := params["nodes_Device"]; !ok {
		t.Errorf("params should contain 'nodes_Device' key")
	}
	if _, ok := params["nodes_Interface"]; !ok {
		t.Errorf("params should contain 'nodes_Interface' key")
	}
}

func TestBuildCypher_DBParamPresent(t *testing.T) {
	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:A", Props: map[string]any{"hostname": "r1"}},
	}
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:A", To: "iface:eth0"},
	}
	uris := []string{"device:A"}

	actions := []string{"create", "upsert", "delete", "delete_relations"}
	for _, action := range actions {
		_, params := client.BuildCypher(action, "mydb", nodes, rels, uris)
		if params["_db"] != "mydb" {
			t.Errorf("BuildCypher(%q) params[_db] = %v, want 'mydb'", action, params["_db"])
		}
	}
}

func TestBuildCypher_EmptyInput(t *testing.T) {
	client := newTestClient(t)

	// 所有 action 传入空数据不应 panic
	actions := []string{"create", "upsert", "delete", "delete_relations"}
	for _, action := range actions {
		cypher, params := client.BuildCypher(action, "testdb", nil, nil, nil)
		_ = cypher
		if params == nil {
			t.Errorf("BuildCypher(%q) params should not be nil", action)
		}
	}
}

func TestBuildCypher_ParamsKeyFormat(t *testing.T) {
	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:A"},
		{Labels: []string{"Device"}, URI: "device:B"},
	}

	_, params := client.BuildCypher("create", "testdb", nodes, nil, nil)

	// key 应为 "nodes_Device" 格式
	nd, ok := params["nodes_Device"].([]map[string]any)
	if !ok {
		t.Fatalf("params[nodes_Device] should be []map[string]any, got: %T", params["nodes_Device"])
	}
	if len(nd) != 2 {
		t.Errorf("params[nodes_Device] length = %d, want 2", len(nd))
	}
}

func TestBuildCypher_NoSession(t *testing.T) {
	// BuildCypher 是纯函数，不应调用 sessionFactory
	// 通过替换 sessionFactory 为 panic 函数来验证
	orig := sessionFactory
	sessionFactory = func(_ context.Context, _ neo4j.DriverWithContext, _ neo4j.SessionConfig) session {
		panic("BuildCypher should NOT call sessionFactory")
	}
	t.Cleanup(func() { sessionFactory = orig })

	client := newTestClient(t)
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:A"},
	}
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:A", To: "iface:eth0"},
	}

	// 调用所有 action，都不应 panic
	for _, action := range []string{"create", "upsert", "delete", "delete_relations"} {
		cypher, params := client.BuildCypher(action, "testdb", nodes, rels, []string{"device:A"})
		_ = cypher
		_ = params
	}
}

// ---------------------------------------------------------------------------
// TestEnsureIndexes — EnsureIndexes 方法测试
// ---------------------------------------------------------------------------

func TestEnsureIndexes_Success(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	labels := []string{"Device", "Interface"}

	err := client.EnsureIndexes(context.Background(), labels)
	if err != nil {
		t.Fatalf("EnsureIndexes() unexpected error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 Run calls for 2 labels, got %d", len(calls))
	}

	for _, call := range calls {
		if !strings.Contains(call.cypher, "CREATE INDEX") {
			t.Errorf("cypher should contain 'CREATE INDEX', got: %s", call.cypher)
		}
		if !strings.Contains(call.cypher, "IF NOT EXISTS") {
			t.Errorf("cypher should contain 'IF NOT EXISTS', got: %s", call.cypher)
		}
		if !strings.Contains(call.cypher, "ON (n._db, n.uri)") {
			t.Errorf("cypher should contain 'ON (n._db, n.uri)', got: %s", call.cypher)
		}
	}

	// 验证索引名格式
	foundDevice := false
	foundInterface := false
	for _, call := range calls {
		if strings.Contains(call.cypher, "idx_device_db_uri") {
			foundDevice = true
			if !strings.Contains(call.cypher, "FOR (n:Device)") {
				t.Errorf("cypher should contain 'FOR (n:Device)', got: %s", call.cypher)
			}
		}
		if strings.Contains(call.cypher, "idx_interface_db_uri") {
			foundInterface = true
			if !strings.Contains(call.cypher, "FOR (n:Interface)") {
				t.Errorf("cypher should contain 'FOR (n:Interface)', got: %s", call.cypher)
			}
		}
	}
	if !foundDevice {
		t.Error("expected Device index cypher not found")
	}
	if !foundInterface {
		t.Error("expected Interface index cypher not found")
	}
}

func TestEnsureIndexes_Idempotent(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	_ = client.EnsureIndexes(context.Background(), []string{"Device"})

	if len(calls) != 1 {
		t.Fatalf("expected 1 Run call, got %d", len(calls))
	}
	// 验证幂等性：Cypher 含 IF NOT EXISTS
	if !strings.Contains(calls[0].cypher, "IF NOT EXISTS") {
		t.Errorf("cypher should contain 'IF NOT EXISTS' for idempotency, got: %s", calls[0].cypher)
	}
}

func TestEnsureIndexes_EmptyLabels(t *testing.T) {
	var calls []runCall
	captureSessionFactory(t, &calls, nil, nil)

	client := newTestClient(t)
	err := client.EnsureIndexes(context.Background(), nil)
	if err != nil {
		t.Fatalf("EnsureIndexes() unexpected error: %v", err)
	}

	// 空 labels 不应执行任何 Run
	if len(calls) != 0 {
		t.Errorf("expected 0 Run calls for empty labels, got %d", len(calls))
	}
}

func TestEnsureIndexes_RunError(t *testing.T) {
	wantErr := errors.New("index creation failed")
	captureSessionFactory(t, &[]runCall{}, nil, func(callIndex int) error {
		return wantErr
	})

	client := newTestClient(t)
	err := client.EnsureIndexes(context.Background(), []string{"Device"})
	if err == nil {
		t.Fatal("EnsureIndexes() should return error when Run fails")
	}
	if !strings.Contains(err.Error(), "ensure indexes") {
		t.Errorf("error should contain 'ensure indexes', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

// TestJoinLabels 验证 joinLabels 各分支。
func TestJoinLabels(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := joinLabels(nil); got != "" {
			t.Errorf("joinLabels(nil) = %q, want empty", got)
		}
	})
	t.Run("single", func(t *testing.T) {
		if got := joinLabels([]string{"Device"}); got != ":Device" {
			t.Errorf("joinLabels([Device]) = %q, want :Device", got)
		}
	})
	t.Run("multi", func(t *testing.T) {
		if got := joinLabels([]string{"Device", "Router"}); got != ":Device:Router" {
			t.Errorf("joinLabels([Device,Router]) = %q, want :Device:Router", got)
		}
	})
}

// TestGroupCloneNodesByLabels 验证 groupCloneNodesByLabels 各分支。
func TestGroupCloneNodesByLabels(t *testing.T) {
	t.Run("string_labels", func(t *testing.T) {
		nodes := []map[string]any{
			{"labels": []string{"Device"}, "uri": "d:1"},
			{"labels": []string{"Device"}, "uri": "d:2"},
			{"labels": []string{"Interface"}, "uri": "i:1"},
		}
		groups := groupCloneNodesByLabels(nodes)
		if len(groups) != 2 {
			t.Errorf("expected 2 groups, got %d", len(groups))
		}
		if len(groups["Device"]) != 2 {
			t.Errorf("Device group should have 2 nodes, got %d", len(groups["Device"]))
		}
	})
	t.Run("any_labels", func(t *testing.T) {
		nodes := []map[string]any{
			{"labels": []any{"Router", "Device"}, "uri": "r:1"},
		}
		groups := groupCloneNodesByLabels(nodes)
		if len(groups) != 1 {
			t.Errorf("expected 1 group, got %d", len(groups))
		}
		if _, ok := groups["Router:Device"]; !ok {
			t.Errorf("expected key Router:Device, got keys: %v", groups)
		}
	})
	t.Run("no_labels", func(t *testing.T) {
		nodes := []map[string]any{
			{"uri": "x:1"},
		}
		groups := groupCloneNodesByLabels(nodes)
		if len(groups) != 0 {
			t.Errorf("expected 0 groups for node without labels, got %d", len(groups))
		}
	})
}
