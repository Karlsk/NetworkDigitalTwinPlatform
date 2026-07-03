package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/service"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// ---------------------------------------------------------------------------
// Mock 实现
// ---------------------------------------------------------------------------

// mockAnalysisService 实现 analysisService 接口。
type mockAnalysisService struct {
	queryResult *service.TopologyResult
	queryErr    error
}

var _ analysisService = (*mockAnalysisService)(nil)

func (m *mockAnalysisService) QueryTopology(_ context.Context, _ string, _ int) (*service.TopologyResult, error) {
	return m.queryResult, m.queryErr
}

// mockSnapshotService 实现 snapshotService 接口。
type mockSnapshotService struct {
	listResult  []snapshot.SnapshotMeta
	listErr     error
	diffResult  *snapshot.SnapshotDiff
	diffErr     error
	restoreErr  error
	restoreName string // 记录 Restore 调用时的 name 参数
}

var _ snapshotService = (*mockSnapshotService)(nil)

func (m *mockSnapshotService) List(_ context.Context) ([]snapshot.SnapshotMeta, error) {
	return m.listResult, m.listErr
}
func (m *mockSnapshotService) Diff(_ context.Context, _, _ string) (*snapshot.SnapshotDiff, error) {
	return m.diffResult, m.diffErr
}
func (m *mockSnapshotService) Restore(_ context.Context, name string) error {
	m.restoreName = name
	return m.restoreErr
}
func (m *mockSnapshotService) AuditQuery(_ snapshot.AuditFilter) []snapshot.AuditEntry {
	return nil
}
func (m *mockSnapshotService) AuditRecent(_ int) []snapshot.AuditEntry {
	return nil
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

// mockDeviceService 实现 deviceService 接口。
type mockDeviceService struct {
	monitorResult any
	monitorErr    error
	deviceResult  any
	deviceErr     error
}

var _ deviceService = (*mockDeviceService)(nil)

func (m *mockDeviceService) QueryMonitor(_ context.Context, _ service.MonitorRequest) (any, error) {
	return m.monitorResult, m.monitorErr
}
func (m *mockDeviceService) QueryDeviceInfo(_ context.Context, _ service.DeviceInfoRequest) (any, error) {
	return m.deviceResult, m.deviceErr
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
// TC-M01: ListTools — 返回 7 个工具，名称正确
// ---------------------------------------------------------------------------

func TestListTools(t *testing.T) {
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	if len(res.Tools) != 7 {
		t.Fatalf("ListTools() returned %d tools, want 7", len(res.Tools))
	}

	wantNames := map[string]bool{
		"query_topology":    false,
		"query_snapshot":    false,
		"sync_data":         false,
		"restore_snapshot":  false,
		"query_monitor":     false,
		"query_device_info": false,
		"query_topology_live": false,
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
	mockResult := &service.TopologyResult{
		Nodes: []map[string]any{
			{"n": map[string]any{"uri": "device:SN001", "hostname": "router-01"}},
			{"n": map[string]any{"uri": "device:SN002", "hostname": "router-02"}},
			{"n": map[string]any{"uri": "device:SN003", "hostname": "router-03"}},
		},
		Count: 3,
	}
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{queryResult: mockResult},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{},
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
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{listResult: metas},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{},
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
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{fullSyncResult: mockResult},
		deviceSvc:   &mockDeviceService{},
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
	mockSvc := &mockSnapshotService{}
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: mockSvc,
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{},
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
	if mockSvc.restoreName != "snap-001" {
		t.Errorf("Restore called with name=%q, want snap-001", mockSvc.restoreName)
	}
}

// ---------------------------------------------------------------------------
// TC-M06: restore_snapshot 缺参数 — IsError=true
// ---------------------------------------------------------------------------

func TestToolInvalidParams(t *testing.T) {
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{},
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
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{},
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

// ---------------------------------------------------------------------------
// TC-M08: query_snapshot diff action — snap_a + snap_b 参数
// ---------------------------------------------------------------------------

func TestQuerySnapshotDiff(t *testing.T) {
	mockDiff := &snapshot.SnapshotDiff{
		AddedNodes: []assembler.Node{{URI: "device:NEW", Labels: []string{"Device"}}},
		RemovedNodes: []assembler.Node{
			{URI: "device:OLD", Labels: []string{"Device"}},
			{URI: "device:OLD2", Labels: []string{"Device"}},
		},
		AddedRels: []assembler.Relation{{Type: "CONNECTS_TO", From: "device:A", To: "device:B"}},
	}
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{diffResult: mockDiff},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_snapshot",
		Arguments: map[string]any{"action": "diff", "snap_a": "snap-001", "snap_b": "snap-002"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_snapshot) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(query_snapshot) IsError=true, content=%v", res.Content)
	}

	var out QuerySnapshotOutput
	extractStructuredOutput(t, res.StructuredContent, &out)
	if out.Diff == nil {
		t.Fatal("Diff is nil")
	}
	if out.Diff.AddedNodes != 1 {
		t.Errorf("Diff.AddedNodes = %d, want 1", out.Diff.AddedNodes)
	}
	if out.Diff.RemovedNodes != 2 {
		t.Errorf("Diff.RemovedNodes = %d, want 2", out.Diff.RemovedNodes)
	}
	if out.Diff.AddedRels != 1 {
		t.Errorf("Diff.AddedRels = %d, want 1", out.Diff.AddedRels)
	}
}

// ---------------------------------------------------------------------------
// TC-M09: query_topology 错误路径 — mock 返回 error
// ---------------------------------------------------------------------------

func TestQueryTopologyError(t *testing.T) {
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{queryErr: errors.New("neo4j timeout")},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_topology",
		Arguments: map[string]any{"label": "Device"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_topology) error = %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for query topology error")
	}
}

// ---------------------------------------------------------------------------
// TC-M10: sync_data 错误路径 — mock FullSync 返回 error
// ---------------------------------------------------------------------------

func TestSyncDataError(t *testing.T) {
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{fullSyncErr: errors.New("sync failed")},
		deviceSvc:   &mockDeviceService{},
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
	if !res.IsError {
		t.Error("expected IsError=true for sync failure")
	}
}

// ---------------------------------------------------------------------------
// TC-M11: query_snapshot list 错误路径 — mock List 返回 error
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// V1-13: query_snapshot diff — ChangedNodes/ChangedRels 统计
// ---------------------------------------------------------------------------

func TestQuerySnapshotDiff_ChangedStats(t *testing.T) {
	mockDiff := &snapshot.SnapshotDiff{
		ChangedNodes: []snapshot.NodeChange{
			{URI: "device:001", Label: "Device", ModifiedFields: map[string]snapshot.FieldChange{"status": {OldValue: "up", NewValue: "down"}}},
			{URI: "device:002", Label: "Device", AddedFields: map[string]any{"mtu": 9000}},
		},
		ChangedRels: []snapshot.RelChange{
			{Type: "CONNECTS", From: "device:001", To: "device:002", ModifiedFields: map[string]snapshot.FieldChange{"bandwidth": {OldValue: 100, NewValue: 200}}},
		},
	}
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{diffResult: mockDiff},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_snapshot",
		Arguments: map[string]any{"action": "diff", "snap_a": "snap-001", "snap_b": "snap-002"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_snapshot) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(query_snapshot) IsError=true, content=%v", res.Content)
	}

	var out QuerySnapshotOutput
	extractStructuredOutput(t, res.StructuredContent, &out)
	if out.Diff == nil {
		t.Fatal("Diff is nil")
	}
	if out.Diff.ChangedNodes != 2 {
		t.Errorf("Diff.ChangedNodes = %d, want 2", out.Diff.ChangedNodes)
	}
	if out.Diff.ChangedRels != 1 {
		t.Errorf("Diff.ChangedRels = %d, want 1", out.Diff.ChangedRels)
	}
}

func TestQuerySnapshotListError(t *testing.T) {
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{listErr: errors.New("filesystem error")},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{},
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
	if !res.IsError {
		t.Error("expected IsError=true for list snapshot error")
	}
}

// ---------------------------------------------------------------------------
// TC-M12: query_monitor — 正向调用
// ---------------------------------------------------------------------------

func TestQueryMonitor(t *testing.T) {
	mockMetrics := map[string]any{"device": "router-01", "metrics": []any{map[string]any{"name": "cpu_usage"}}}
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{monitorResult: mockMetrics},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_monitor",
		Arguments: map[string]any{"connector_name": "ctrl", "query_type": "device", "device": "router-01", "metrics": []any{"cpu_usage"}},
	})
	if err != nil {
		t.Fatalf("CallTool(query_monitor) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(query_monitor) IsError=true, content=%v", res.Content)
	}
	if res.StructuredContent == nil {
		t.Error("StructuredContent is nil")
	}
}

// ---------------------------------------------------------------------------
// TC-M13: query_monitor — 错误路径
// ---------------------------------------------------------------------------

func TestQueryMonitorError(t *testing.T) {
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{monitorErr: errors.New("connector unavailable")},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_monitor",
		Arguments: map[string]any{"connector_name": "ctrl", "query_type": "device"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_monitor) error = %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for query monitor error")
	}
}

// ---------------------------------------------------------------------------
// TC-M13b: query_monitor — 时间解析错误
// ---------------------------------------------------------------------------

func TestQueryMonitorInvalidTime(t *testing.T) {
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_monitor",
		Arguments: map[string]any{"connector_name": "ctrl", "query_type": "device", "start_time": "not-a-time"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_monitor) error = %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for invalid start_time")
	}
}

// ---------------------------------------------------------------------------
// TC-M14: query_device_info — 正向调用
// ---------------------------------------------------------------------------

func TestQueryDeviceInfo(t *testing.T) {
	mockConfig := map[string]any{"config": "hostname router-01", "device": "router-01"}
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{deviceResult: mockConfig},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_device_info",
		Arguments: map[string]any{"connector_name": "ctrl", "query_type": "config", "device": "router-01"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_device_info) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(query_device_info) IsError=true, content=%v", res.Content)
	}
	if res.StructuredContent == nil {
		t.Error("StructuredContent is nil")
	}
}

// ---------------------------------------------------------------------------
// TC-M15: query_device_info — 错误路径
// ---------------------------------------------------------------------------

func TestQueryDeviceInfoError(t *testing.T) {
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{deviceErr: errors.New("device unreachable")},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_device_info",
		Arguments: map[string]any{"connector_name": "ctrl", "query_type": "config", "device": "router-01"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_device_info) error = %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for query device info error")
	}
}

// ---------------------------------------------------------------------------
// TC-M16: query_topology_live — 正向调用
// ---------------------------------------------------------------------------

func TestQueryTopologyLive(t *testing.T) {
	mockTopology := map[string]any{
		"nodes": []any{map[string]any{"id": "r1"}, map[string]any{"id": "r2"}},
		"links": []any{map[string]any{"from": "r1", "to": "r2"}},
	}
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{deviceResult: mockTopology},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_topology_live",
		Arguments: map[string]any{"connector_name": "ctrl"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_topology_live) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(query_topology_live) IsError=true, content=%v", res.Content)
	}
	if res.StructuredContent == nil {
		t.Error("StructuredContent is nil")
	}
}

// ---------------------------------------------------------------------------
// TC-M17: query_topology_live — 错误路径
// ---------------------------------------------------------------------------

func TestQueryTopologyLiveError(t *testing.T) {
	h := &toolHandlers{
		analysisSvc: &mockAnalysisService{},
		snapshotSvc: &mockSnapshotService{},
		syncSvc:     &mockSyncService{},
		deviceSvc:   &mockDeviceService{deviceErr: errors.New("controller timeout")},
	}
	cs := newTestServer(t, h)

	ctx := context.Background()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "query_topology_live",
		Arguments: map[string]any{"connector_name": "ctrl"},
	})
	if err != nil {
		t.Fatalf("CallTool(query_topology_live) error = %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for query topology live error")
	}
}
