package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/graph"
	"gitlab.com/pml/network-digital-twin/internal/service"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// ---------------------------------------------------------------------------
// Mock 实现
// ---------------------------------------------------------------------------

// mockGraphDB 实现 graph.GraphDB 接口，Query 返回可配置数据。
type mockGraphDB struct {
	queryResult []map[string]any
	queryErr    error
}

var _ graph.GraphDB = (*mockGraphDB)(nil)

func (m *mockGraphDB) Ping(_ context.Context) error { return nil }
func (m *mockGraphDB) Close() error                 { return nil }
func (m *mockGraphDB) BulkCreate(_ context.Context, _ string, _ []assembler.Node, _ []assembler.Relation) error {
	return nil
}
func (m *mockGraphDB) Upsert(_ context.Context, _ string, _ []assembler.Node, _ []assembler.Relation) error {
	return nil
}
func (m *mockGraphDB) DeleteRelations(_ context.Context, _ string, _ []assembler.Relation) error {
	return nil
}
func (m *mockGraphDB) DeleteByURIs(_ context.Context, _ string, _ []string) error { return nil }
func (m *mockGraphDB) Query(_ context.Context, _ string, _ string, _ map[string]any) ([]map[string]any, error) {
	return m.queryResult, m.queryErr
}
func (m *mockGraphDB) BuildCypher(_ string, _ string, _ []assembler.Node, _ []assembler.Relation, _ []string) (string, map[string]any) {
	return "", nil
}
func (m *mockGraphDB) ClearDB(_ context.Context, _ string) error             { return nil }
func (m *mockGraphDB) CloneDB(_ context.Context, _, _ string) error          { return nil }
func (m *mockGraphDB) ListDBs(_ context.Context) ([]string, error)           { return nil, nil }
func (m *mockGraphDB) HasDB(_ context.Context, _ string) (bool, error)       { return false, nil }
func (m *mockGraphDB) EnsureIndexes(_ context.Context, _ []string) error     { return nil }

// mockSnapshotManager 实现 snapshotManager 接口。
type mockSnapshotManager struct {
	listResult  []snapshot.SnapshotMeta
	listErr     error
	diffResult  *snapshot.SnapshotDiff
	diffErr     error
	restoreErr  error
	restoreName string // 记录 Restore 调用时的 name 参数
}

var _ snapshotManager = (*mockSnapshotManager)(nil)

func (m *mockSnapshotManager) List(_ context.Context) ([]snapshot.SnapshotMeta, error) {
	return m.listResult, m.listErr
}
func (m *mockSnapshotManager) Diff(_ context.Context, _, _ string) (*snapshot.SnapshotDiff, error) {
	return m.diffResult, m.diffErr
}
func (m *mockSnapshotManager) Restore(_ context.Context, name string) error {
	m.restoreName = name
	return m.restoreErr
}

// mockSyncService 实现 syncService 接口。
type mockSyncService struct {
	fullSyncResult *service.SyncResult
	fullSyncErr    error
	fullSyncCalls  int
}

var _ syncService = (*mockSyncService)(nil)

func (m *mockSyncService) FullSync(_ context.Context) (*service.SyncResult, error) {
	m.fullSyncCalls++
	return m.fullSyncResult, m.fullSyncErr
}

// ---------------------------------------------------------------------------
// 测试辅助
// ---------------------------------------------------------------------------

// extractStructuredOutput 将 StructuredContent 通过 JSON round-trip 解码到目标结构体。
func extractStructuredOutput(t *testing.T, raw any, dst any) {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal StructuredContent error = %v", err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatalf("unmarshal StructuredContent error = %v", err)
	}
}

// newTestServer 创建带有 mock 依赖的 MCP Server + ClientSession。
// 返回 ClientSession 和清理函数。
func newTestServer(t *testing.T, h *toolHandlers) *mcpsdk.ClientSession {
	t.Helper()

	ctx := context.Background()
	server := newServer(h)

	ct, st := mcpsdk.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}

	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: "test-client", Version: "v1.0.0"},
		nil,
	)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}

	t.Cleanup(func() { cs.Close() })
	return cs
}

// ---------------------------------------------------------------------------
// TC-M01: ListTools — 返回 4 个工具，名称正确
// ---------------------------------------------------------------------------

func TestListTools(t *testing.T) {
	h := &toolHandlers{
		graph:   &mockGraphDB{},
		lock:    snapshot.NewGraphLock(),
		manager: &mockSnapshotManager{},
		syncSvc: &mockSyncService{},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	if len(res.Tools) != 4 {
		t.Fatalf("ListTools() returned %d tools, want 4", len(res.Tools))
	}

	wantNames := map[string]bool{
		"query_topology":   false,
		"query_snapshot":   false,
		"sync_data":        false,
		"restore_snapshot": false,
	}
	for _, tool := range res.Tools {
		if _, ok := wantNames[tool.Name]; ok {
			wantNames[tool.Name] = true
		} else {
			t.Errorf("unexpected tool name %q", tool.Name)
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("tool %q not found in ListTools result", name)
		}
	}
}

// ---------------------------------------------------------------------------
// TC-M02: query_topology — mock Query 返回 3 条数据
// ---------------------------------------------------------------------------

func TestQueryTopology(t *testing.T) {
	mockRows := []map[string]any{
		{"n": map[string]any{"uri": "device:SN001", "hostname": "router-01"}},
		{"n": map[string]any{"uri": "device:SN002", "hostname": "router-02"}},
		{"n": map[string]any{"uri": "device:SN003", "hostname": "router-03"}},
	}
	h := &toolHandlers{
		graph:   &mockGraphDB{queryResult: mockRows},
		lock:    snapshot.NewGraphLock(),
		manager: &mockSnapshotManager{},
		syncSvc: &mockSyncService{},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_topology",
		Arguments: map[string]any{"label": "Device", "limit": 100},
	})
	if err != nil {
		t.Fatalf("CallTool(query_topology) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(query_topology) IsError=true, content=%v", res.Content)
	}

	// 解析结构化输出
	var out QueryTopologyOutput
	extractStructuredOutput(t, res.StructuredContent, &out)
	if out.Count != 3 {
		t.Errorf("Count = %d, want 3", out.Count)
	}
	if len(out.Nodes) != 3 {
		t.Errorf("len(Nodes) = %d, want 3", len(out.Nodes))
	}
}

// ---------------------------------------------------------------------------
// TC-M03: query_snapshot (list) — mock List 返回 2 个 SnapshotMeta
// ---------------------------------------------------------------------------

func TestQuerySnapshotList(t *testing.T) {
	metas := []snapshot.SnapshotMeta{
		{Name: "snap-001", CreatedAt: time.Now(), NodeCount: 10, RelCount: 5},
		{Name: "snap-002", CreatedAt: time.Now(), NodeCount: 20, RelCount: 15},
	}
	h := &toolHandlers{
		graph:   &mockGraphDB{},
		lock:    snapshot.NewGraphLock(),
		manager: &mockSnapshotManager{listResult: metas},
		syncSvc: &mockSyncService{},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_snapshot",
		Arguments: map[string]any{"action": "list"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_snapshot) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(query_snapshot) IsError=true, content=%v", res.Content)
	}

	var out QuerySnapshotOutput
	extractStructuredOutput(t, res.StructuredContent, &out)
	if len(out.Snapshots) != 2 {
		t.Errorf("len(Snapshots) = %d, want 2", len(out.Snapshots))
	}
	if out.Snapshots[0].Name != "snap-001" {
		t.Errorf("Snapshots[0].Name = %q, want snap-001", out.Snapshots[0].Name)
	}
}

// ---------------------------------------------------------------------------
// TC-M04: sync_data (full) — mock FullSync 返回 NodesCreated:21
// ---------------------------------------------------------------------------

func TestSyncDataFull(t *testing.T) {
	mockResult := &service.SyncResult{
		NodesCreated:     21,
		RelationsCreated: 30,
		Duration:         500 * time.Millisecond,
	}
	h := &toolHandlers{
		graph:   &mockGraphDB{},
		lock:    snapshot.NewGraphLock(),
		manager: &mockSnapshotManager{},
		syncSvc: &mockSyncService{fullSyncResult: mockResult},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "sync_data",
		Arguments: map[string]any{"action": "full"},
	})
	if err != nil {
		t.Fatalf("CallTool(sync_data) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(sync_data) IsError=true, content=%v", res.Content)
	}

	var out SyncDataOutput
	extractStructuredOutput(t, res.StructuredContent, &out)
	if out.NodesCreated != 21 {
		t.Errorf("NodesCreated = %d, want 21", out.NodesCreated)
	}
	if out.RelationsCreated != 30 {
		t.Errorf("RelationsCreated = %d, want 30", out.RelationsCreated)
	}
}

// ---------------------------------------------------------------------------
// TC-M05: restore_snapshot — mock Restore 返回 nil
// ---------------------------------------------------------------------------

func TestRestoreSnapshot(t *testing.T) {
	mockMgr := &mockSnapshotManager{}
	h := &toolHandlers{
		graph:   &mockGraphDB{},
		lock:    snapshot.NewGraphLock(),
		manager: mockMgr,
		syncSvc: &mockSyncService{},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "restore_snapshot",
		Arguments: map[string]any{"snapshot_name": "snap-001"},
	})
	if err != nil {
		t.Fatalf("CallTool(restore_snapshot) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(restore_snapshot) IsError=true, content=%v", res.Content)
	}

	var out RestoreSnapshotOutput
	extractStructuredOutput(t, res.StructuredContent, &out)
	if out.Message == "" {
		t.Error("Message is empty")
	}
	if mockMgr.restoreName != "snap-001" {
		t.Errorf("Restore called with name=%q, want snap-001", mockMgr.restoreName)
	}
}

// ---------------------------------------------------------------------------
// TC-M06: restore_snapshot 缺参数 — IsError=true
// ---------------------------------------------------------------------------

func TestToolInvalidParams(t *testing.T) {
	h := &toolHandlers{
		graph:   &mockGraphDB{},
		lock:    snapshot.NewGraphLock(),
		manager: &mockSnapshotManager{},
		syncSvc: &mockSyncService{},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "restore_snapshot",
		Arguments: map[string]any{}, // 缺少 snapshot_name
	})
	if err != nil {
		t.Fatalf("CallTool(restore_snapshot) error = %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for missing snapshot_name")
	}
}

// ---------------------------------------------------------------------------
// TC-M07: 调用不存在的工具 — error
// ---------------------------------------------------------------------------

func TestCallNonExistentTool(t *testing.T) {
	h := &toolHandlers{
		graph:   &mockGraphDB{},
		lock:    snapshot.NewGraphLock(),
		manager: &mockSnapshotManager{},
		syncSvc: &mockSyncService{},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	_, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "nonexistent_tool",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Error("expected error for calling nonexistent tool")
	}
}
