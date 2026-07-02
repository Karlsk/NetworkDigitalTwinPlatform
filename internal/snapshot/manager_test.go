package snapshot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gopkg.in/yaml.v3"
)

func TestSnapshotMetaFields(t *testing.T) {
	createdAt := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	meta := SnapshotMeta{
		Name:      "snap-20240615",
		CreatedAt: createdAt,
		NodeCount: 42,
		RelCount:  18,
		FilePath:  "/data/snapshots/snap-20240615.yaml",
	}

	if meta.Name != "snap-20240615" {
		t.Errorf("Name = %q, want %q", meta.Name, "snap-20240615")
	}
	if !meta.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v", meta.CreatedAt, createdAt)
	}
	if meta.NodeCount != 42 {
		t.Errorf("NodeCount = %d, want 42", meta.NodeCount)
	}
	if meta.RelCount != 18 {
		t.Errorf("RelCount = %d, want 18", meta.RelCount)
	}
	if meta.FilePath != "/data/snapshots/snap-20240615.yaml" {
		t.Errorf("FilePath = %q, want %q", meta.FilePath, "/data/snapshots/snap-20240615.yaml")
	}
}

func TestSnapshotDiffFields(t *testing.T) {
	diff := SnapshotDiff{
		AddedNodes: []assembler.Node{
			{Labels: []string{"Device"}, URI: "device:NEW001", Props: map[string]any{"hostname": "new-router"}},
		},
		RemovedNodes: []assembler.Node{
			{Labels: []string{"Device"}, URI: "device:OLD001"},
		},
		AddedRels: []assembler.Relation{
			{Type: "HAS_INTERFACE", From: "device:NEW001", To: "iface:NEW001_eth0"},
		},
		RemovedRels: []assembler.Relation{
			{Type: "HAS_INTERFACE", From: "device:OLD001", To: "iface:OLD001_eth0"},
		},
	}

	if len(diff.AddedNodes) != 1 {
		t.Fatalf("AddedNodes count = %d, want 1", len(diff.AddedNodes))
	}
	if diff.AddedNodes[0].URI != "device:NEW001" {
		t.Errorf("AddedNodes[0].URI = %q, want %q", diff.AddedNodes[0].URI, "device:NEW001")
	}
	if len(diff.RemovedNodes) != 1 {
		t.Fatalf("RemovedNodes count = %d, want 1", len(diff.RemovedNodes))
	}
	if diff.RemovedNodes[0].URI != "device:OLD001" {
		t.Errorf("RemovedNodes[0].URI = %q, want %q", diff.RemovedNodes[0].URI, "device:OLD001")
	}
	if len(diff.AddedRels) != 1 {
		t.Fatalf("AddedRels count = %d, want 1", len(diff.AddedRels))
	}
	if diff.AddedRels[0].Type != "HAS_INTERFACE" {
		t.Errorf("AddedRels[0].Type = %q, want %q", diff.AddedRels[0].Type, "HAS_INTERFACE")
	}
	if len(diff.RemovedRels) != 1 {
		t.Fatalf("RemovedRels count = %d, want 1", len(diff.RemovedRels))
	}
	if diff.RemovedRels[0].From != "device:OLD001" {
		t.Errorf("RemovedRels[0].From = %q, want %q", diff.RemovedRels[0].From, "device:OLD001")
	}
}

func TestSnapshotDiffEmpty(t *testing.T) {
	diff := SnapshotDiff{}
	if diff.AddedNodes != nil {
		t.Errorf("expected nil AddedNodes for zero-value, got %v", diff.AddedNodes)
	}
	if diff.RemovedNodes != nil {
		t.Errorf("expected nil RemovedNodes for zero-value, got %v", diff.RemovedNodes)
	}
	if diff.AddedRels != nil {
		t.Errorf("expected nil AddedRels for zero-value, got %v", diff.AddedRels)
	}
	if diff.RemovedRels != nil {
		t.Errorf("expected nil RemovedRels for zero-value, got %v", diff.RemovedRels)
	}
}

// ---------------------------------------------------------------------------
// I-16: SnapshotManager 测试
// ---------------------------------------------------------------------------

// TestNewSnapshotManager 验证构造函数。
func TestNewSnapshotManager(t *testing.T) {
	gdb := &mockGraphDB{}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)
	if mgr == nil {
		t.Fatal("NewSnapshotManager() returned nil")
	}
}

// TestSnapshotManager_Create 验证 Create 导出快照为 YAML 文件。
func TestSnapshotManager_Create(t *testing.T) {
	// mock Query 返回节点和关系数据
	gdb := &mockGraphDB{
		queryResults: []map[string]any{
			{"labels": []any{"Device"}, "uri": "device:SN001", "props": map[string]any{"hostname": "router-01"}},
		},
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)

	meta, err := mgr.Create(context.Background(), "snap-001")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// 验证元数据
	if meta.Name != "snap-001" {
		t.Errorf("meta.Name = %q, want %q", meta.Name, "snap-001")
	}
	if meta.NodeCount < 0 {
		t.Errorf("meta.NodeCount = %d, want >= 0", meta.NodeCount)
	}

	// 验证 YAML 文件写入
	filePath := filepath.Join(snapDir, "snap-001.yaml")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("YAML file not created at %s", filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if len(data) == 0 {
		t.Error("YAML file is empty")
	}
}

// TestSnapshotManager_Create_QueryError 验证 Query 失败时 Create 返回错误。
func TestSnapshotManager_Create_QueryError(t *testing.T) {
	wantErr := errors.New("neo4j connection refused")
	gdb := &mockGraphDB{queryErr: wantErr}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)

	_, err := mgr.Create(context.Background(), "snap-err")
	if err == nil {
		t.Fatal("Create() should return error when Query fails")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original, got: %v", err)
	}
}

// TestSnapshotManager_List 验证列出所有 YAML 归档。
func TestSnapshotManager_List(t *testing.T) {
	gdb := &mockGraphDB{
		queryResults: []map[string]any{
			{"labels": []any{"Device"}, "uri": "device:SN001", "props": map[string]any{"hostname": "r1"}},
		},
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)

	// 创建两个快照
	if _, err := mgr.Create(context.Background(), "snap-a"); err != nil {
		t.Fatalf("Create(snap-a) error = %v", err)
	}
	if _, err := mgr.Create(context.Background(), "snap-b"); err != nil {
		t.Fatalf("Create(snap-b) error = %v", err)
	}

	metas, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(metas) != 2 {
		t.Errorf("List() returned %d snapshots, want 2", len(metas))
	}
}

// TestSnapshotManager_List_EmptyDir 验证空目录返回空切片。
func TestSnapshotManager_List_EmptyDir(t *testing.T) {
	gdb := &mockGraphDB{}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)

	metas, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(metas) != 0 {
		t.Errorf("List() returned %d snapshots, want 0", len(metas))
	}
}

// TestSnapshotManager_Delete_ClearsNeo4j 验证 Delete 清理 Neo4j 逻辑 DB。
func TestSnapshotManager_Delete_ClearsNeo4j(t *testing.T) {
	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-001": true},
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)

	err := mgr.Delete(context.Background(), "snap-001")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// 验证 ClearDB 被调用
	if len(gdb.clearDBCalls) != 1 || gdb.clearDBCalls[0] != "snap-001" {
		t.Errorf("ClearDB calls = %v, want [snap-001]", gdb.clearDBCalls)
	}
}

// TestSnapshotManager_Delete_PreservesYAML 验证 Delete 保留 YAML 文件。
func TestSnapshotManager_Delete_PreservesYAML(t *testing.T) {
	gdb := &mockGraphDB{
		queryResults: []map[string]any{
			{"labels": []any{"Device"}, "uri": "device:SN001", "props": map[string]any{"hostname": "r1"}},
		},
		hasDBResult: map[string]bool{"snap-keep": true},
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)

	// 先创建快照
	if _, err := mgr.Create(context.Background(), "snap-keep"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	filePath := filepath.Join(snapDir, "snap-keep.yaml")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("YAML file not created")
	}

	// 删除快照
	if err := mgr.Delete(context.Background(), "snap-keep"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// YAML 文件应仍存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("YAML file should be preserved after Delete")
	}
}

// TestSnapshotManager_Delete_NonExistentDB 验证 DB 不存在时不报错。
func TestSnapshotManager_Delete_NonExistentDB(t *testing.T) {
	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-gone": false},
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)

	err := mgr.Delete(context.Background(), "snap-gone")
	if err != nil {
		t.Fatalf("Delete() error = %v, want nil (DB does not exist)", err)
	}

	// ClearDB 不应被调用
	if len(gdb.clearDBCalls) != 0 {
		t.Errorf("ClearDB should not be called when DB doesn't exist, got %d calls", len(gdb.clearDBCalls))
	}
}

// ---------------------------------------------------------------------------
// YAML 快照测试辅助
// ---------------------------------------------------------------------------

// writeTestSnapshot 在指定目录创建测试用 YAML 快照文件。
func writeTestSnapshot(t *testing.T, dir, name string, nodes []yamlNodeItem, rels []yamlRelItem) {
	t.Helper()
	filePath := filepath.Join(dir, name+".yaml")
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	defer f.Close()

	enc := yaml.NewEncoder(f)
	defer enc.Close()

	meta := yamlMetaDoc{
		Kind: "SnapshotMeta", Name: name, CreatedAt: time.Now(),
		NodeCount: len(nodes), RelCount: len(rels),
	}
	if err := enc.Encode(meta); err != nil {
		t.Fatalf("encode meta: %v", err)
	}
	if err := enc.Encode(yamlNodesDoc{Kind: "Nodes", Items: nodes}); err != nil {
		t.Fatalf("encode nodes: %v", err)
	}
	if err := enc.Encode(yamlRelsDoc{Kind: "Relations", Items: rels}); err != nil {
		t.Fatalf("encode rels: %v", err)
	}
}

// ---------------------------------------------------------------------------
// I-17: EnsureLoaded / Restore / Diff / LocalDiff 测试
// ---------------------------------------------------------------------------

// TestSnapshotManager_EnsureLoaded_FromYAML HasDB=false → importFromYAML → BulkCreate。
func TestSnapshotManager_EnsureLoaded_FromYAML(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{{Labels: []string{"Device"}, URI: "device:001"}},
		nil,
	)

	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-a": false},
	}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	err := mgr.EnsureLoaded(context.Background(), "snap-a")
	if err != nil {
		t.Fatalf("EnsureLoaded() error = %v", err)
	}

	if len(gdb.bulkCreateCalls) != 1 {
		t.Fatalf("BulkCreate calls = %d, want 1", len(gdb.bulkCreateCalls))
	}
	call := gdb.bulkCreateCalls[0]
	if call.DB != "snap-a" {
		t.Errorf("BulkCreate DB = %q, want %q", call.DB, "snap-a")
	}
	if len(call.Nodes) != 1 || call.Nodes[0].URI != "device:001" {
		t.Errorf("BulkCreate nodes = %v, want 1 node with URI device:001", call.Nodes)
	}
}

// TestSnapshotManager_EnsureLoaded_AlreadyLoaded HasDB=true → 不调用 BulkCreate。
func TestSnapshotManager_EnsureLoaded_AlreadyLoaded(t *testing.T) {
	snapDir := t.TempDir()
	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-a": true},
	}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	err := mgr.EnsureLoaded(context.Background(), "snap-a")
	if err != nil {
		t.Fatalf("EnsureLoaded() error = %v", err)
	}

	if len(gdb.bulkCreateCalls) != 0 {
		t.Errorf("BulkCreate should not be called when DB exists, got %d calls", len(gdb.bulkCreateCalls))
	}
}

// TestSnapshotManager_EnsureLoaded_FileNotFound 不存在文件 → 返回 error。
func TestSnapshotManager_EnsureLoaded_FileNotFound(t *testing.T) {
	snapDir := t.TempDir()
	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"nonexistent": false},
	}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	err := mgr.EnsureLoaded(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("EnsureLoaded() should return error for missing file")
	}
}

// TestSnapshotManager_Restore EnsureLoaded → ClearDB("default") → CloneDB(name, "default")。
func TestSnapshotManager_Restore(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{{Labels: []string{"Device"}, URI: "device:001"}},
		nil,
	)

	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-a": false},
	}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	err := mgr.Restore(context.Background(), "snap-a")
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// ClearDB("default") 应被调用
	if len(gdb.clearDBCalls) != 1 || gdb.clearDBCalls[0] != "default" {
		t.Errorf("ClearDB calls = %v, want [default]", gdb.clearDBCalls)
	}

	// CloneDB("snap-a", "default") 应被调用
	if len(gdb.cloneDBCalls) != 1 {
		t.Fatalf("CloneDB calls = %d, want 1", len(gdb.cloneDBCalls))
	}
	if gdb.cloneDBCalls[0].From != "snap-a" || gdb.cloneDBCalls[0].To != "default" {
		t.Errorf("CloneDB = %v, want {snap-a default}", gdb.cloneDBCalls[0])
	}
}

// TestSnapshotManager_Restore_LockAcquired Restore 期间外部 Lock 被阻塞。
func TestSnapshotManager_Restore_LockAcquired(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{{Labels: []string{"Device"}, URI: "device:001"}},
		nil,
	)

	hold := make(chan struct{})
	during := make(chan struct{})
	gdb := &mockGraphDB{
		hasDBResult:    map[string]bool{"snap-a": false},
		bulkCreateHold: hold,
		bulkCreateDuring: func() {
			close(during)
		},
	}
	lock := NewGraphLock()
	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = mgr.Restore(context.Background(), "snap-a")
	}()

	// 等待 BulkCreate 开始（Restore 持有写锁）
	<-during

	// 尝试获取写锁，应被阻塞
	acquired := make(chan struct{})
	go func() {
		lock.Lock()
		close(acquired)
		lock.Unlock()
	}()

	select {
	case <-acquired:
		t.Error("Lock should be blocked during Restore")
	case <-time.After(100 * time.Millisecond):
		// 预期：锁被阻塞
	}

	// 释放 BulkCreate，让 Restore 完成
	close(hold)
	wg.Wait()

	// Restore 完成后锁应释放
	done := make(chan struct{})
	go func() {
		lock.Lock()
		lock.Unlock()
		close(done)
	}()
	select {
	case <-done:
		// 预期：锁已释放
	case <-time.After(time.Second):
		t.Error("Lock should be released after Restore completes")
	}
}

// TestSnapshotManager_Restore_LockReleasedOnError error 后锁释放。
func TestSnapshotManager_Restore_LockReleasedOnError(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{{Labels: []string{"Device"}, URI: "device:001"}},
		nil,
	)

	gdb := &mockGraphDB{
		hasDBResult:   map[string]bool{"snap-a": false},
		bulkCreateErr: errors.New("bulk create failed"),
	}
	lock := NewGraphLock()
	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)

	err := mgr.Restore(context.Background(), "snap-a")
	if err == nil {
		t.Fatal("Restore() should return error")
	}

	// 锁应已释放
	done := make(chan struct{})
	go func() {
		lock.Lock()
		lock.Unlock()
		close(done)
	}()
	select {
	case <-done:
		// 预期
	case <-time.After(time.Second):
		t.Error("Lock should be released after Restore error")
	}
}

// TestSnapshotManager_Diff Cypher 差集查询。
func TestSnapshotManager_Diff(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{{Labels: []string{"Device"}, URI: "device:001"}},
		nil,
	)
	writeTestSnapshot(t, snapDir, "snap-b",
		[]yamlNodeItem{{Labels: []string{"Device"}, URI: "device:002"}},
		nil,
	)

	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-a": false, "snap-b": false},
	}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	diff, err := mgr.Diff(context.Background(), "snap-a", "snap-b")
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if diff == nil {
		t.Fatal("Diff() returned nil")
	}

	// EnsureLoaded 被调用两次
	if len(gdb.bulkCreateCalls) != 2 {
		t.Errorf("BulkCreate calls = %d, want 2", len(gdb.bulkCreateCalls))
	}

	// Query 被调用 4 次（added nodes, removed nodes, added rels, removed rels）
	if len(gdb.queryCalls) != 4 {
		t.Errorf("Query calls = %d, want 4", len(gdb.queryCalls))
	}
}

// TestSnapshotManager_LocalDiff Go 内存对比两个 YAML。
func TestSnapshotManager_LocalDiff(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001"},
			{Labels: []string{"Device"}, URI: "device:002"},
		},
		[]yamlRelItem{
			{Type: "CONNECTS", From: "device:001", To: "device:002"},
		},
	)
	writeTestSnapshot(t, snapDir, "snap-b",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:002"},
			{Labels: []string{"Device"}, URI: "device:003"},
		},
		[]yamlRelItem{
			{Type: "CONNECTS", From: "device:002", To: "device:003"},
		},
	)

	gdb := &mockGraphDB{}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	diff, err := mgr.LocalDiff("snap-a", "snap-b")
	if err != nil {
		t.Fatalf("LocalDiff() error = %v", err)
	}

	// 新增节点: device:003
	if len(diff.AddedNodes) != 1 {
		t.Fatalf("AddedNodes = %d, want 1", len(diff.AddedNodes))
	}
	if diff.AddedNodes[0].URI != "device:003" {
		t.Errorf("AddedNodes[0].URI = %q, want %q", diff.AddedNodes[0].URI, "device:003")
	}

	// 删除节点: device:001
	if len(diff.RemovedNodes) != 1 {
		t.Fatalf("RemovedNodes = %d, want 1", len(diff.RemovedNodes))
	}
	if diff.RemovedNodes[0].URI != "device:001" {
		t.Errorf("RemovedNodes[0].URI = %q, want %q", diff.RemovedNodes[0].URI, "device:001")
	}

	// 新增关系: CONNECTS device:002→device:003
	if len(diff.AddedRels) != 1 {
		t.Fatalf("AddedRels = %d, want 1", len(diff.AddedRels))
	}
	if diff.AddedRels[0].From != "device:002" {
		t.Errorf("AddedRels[0].From = %q, want %q", diff.AddedRels[0].From, "device:002")
	}

	// 删除关系: CONNECTS device:001→device:002
	if len(diff.RemovedRels) != 1 {
		t.Fatalf("RemovedRels = %d, want 1", len(diff.RemovedRels))
	}
	if diff.RemovedRels[0].From != "device:001" {
		t.Errorf("RemovedRels[0].From = %q, want %q", diff.RemovedRels[0].From, "device:001")
	}
}

// ---------------------------------------------------------------------------
// I-18: cleanup LRU 测试
// ---------------------------------------------------------------------------

// TestCleanup_UnderLimit 低于 maxActive 不清理。
func TestCleanup_UnderLimit(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{{Labels: []string{"Device"}, URI: "device:001"}}, nil,
	)

	gdb := &mockGraphDB{
		hasDBResult:   map[string]bool{"snap-a": false},
		listDBsResult: []string{"default", "snap-a"},
	}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 3)

	err := mgr.EnsureLoaded(context.Background(), "snap-a")
	if err != nil {
		t.Fatalf("EnsureLoaded() error = %v", err)
	}

	if len(gdb.clearDBCalls) != 0 {
		t.Errorf("ClearDB should not be called under limit, got %v", gdb.clearDBCalls)
	}
}

// TestCleanup_OverLimit 超过 maxActive 清理最旧。
func TestCleanup_OverLimit(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-c",
		[]yamlNodeItem{{Labels: []string{"Device"}, URI: "device:003"}}, nil,
	)

	gdb := &mockGraphDB{
		hasDBResult:   map[string]bool{"snap-c": false},
		listDBsResult: []string{"default", "snap-a", "snap-b", "snap-c"},
	}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 2)

	err := mgr.EnsureLoaded(context.Background(), "snap-c")
	if err != nil {
		t.Fatalf("EnsureLoaded() error = %v", err)
	}

	// snap-a 和 snap-b 的 lastAccess 为零值（最旧），应被清理
	if len(gdb.clearDBCalls) != 1 {
		t.Fatalf("ClearDB calls = %d, want 1", len(gdb.clearDBCalls))
	}
	if gdb.clearDBCalls[0] != "snap-a" {
		t.Errorf("ClearDB should clear oldest (snap-a), got %q", gdb.clearDBCalls[0])
	}
}

// TestCleanup_NeverCleansDefault "default" 永不清理。
func TestCleanup_NeverCleansDefault(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{{Labels: []string{"Device"}, URI: "device:001"}}, nil,
	)

	gdb := &mockGraphDB{
		hasDBResult:   map[string]bool{"snap-a": false},
		listDBsResult: []string{"default", "snap-a"},
	}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 1)

	err := mgr.EnsureLoaded(context.Background(), "snap-a")
	if err != nil {
		t.Fatalf("EnsureLoaded() error = %v", err)
	}

	// 只有 1 个快照 DB（snap-a），maxActive=1，无需清理
	// "default" 无论如何不应被清理
	for _, call := range gdb.clearDBCalls {
		if call == "default" {
			t.Error("cleanup should never clear 'default'")
		}
	}
}

// TestCleanup_TriggeredByEnsureLoaded EnsureLoaded 后自动触发 cleanup。
func TestCleanup_TriggeredByEnsureLoaded(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-c",
		[]yamlNodeItem{{Labels: []string{"Device"}, URI: "device:003"}}, nil,
	)

	gdb := &mockGraphDB{
		hasDBResult:   map[string]bool{"snap-c": false},
		listDBsResult: []string{"default", "snap-a", "snap-b", "snap-c"},
	}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 2)

	err := mgr.EnsureLoaded(context.Background(), "snap-c")
	if err != nil {
		t.Fatalf("EnsureLoaded() error = %v", err)
	}

	// cleanup 应被触发，清理最旧的 snap-a
	found := false
	for _, call := range gdb.clearDBCalls {
		if call == "snap-a" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cleanup should have been triggered, ClearDB calls = %v", gdb.clearDBCalls)
	}
}

// ---------------------------------------------------------------------------
// V1-11: SnapshotDiff 数据结构扩展测试
// ---------------------------------------------------------------------------

// TestNodeChangeFields 验证 NodeChange 结构体字段赋值和读取。
func TestNodeChangeFields(t *testing.T) {
	nc := NodeChange{
		URI:   "device:001",
		Label: "Device",
		AddedFields: map[string]any{
			"hostname": "router-01",
		},
		RemovedFields: map[string]any{
			"old_field": "value",
		},
		ModifiedFields: map[string]FieldChange{
			"status": {OldValue: "up", NewValue: "down"},
		},
	}

	if nc.URI != "device:001" {
		t.Errorf("URI = %q, want %q", nc.URI, "device:001")
	}
	if nc.Label != "Device" {
		t.Errorf("Label = %q, want %q", nc.Label, "Device")
	}
	if len(nc.AddedFields) != 1 {
		t.Errorf("AddedFields len = %d, want 1", len(nc.AddedFields))
	}
	if len(nc.RemovedFields) != 1 {
		t.Errorf("RemovedFields len = %d, want 1", len(nc.RemovedFields))
	}
	if len(nc.ModifiedFields) != 1 {
		t.Errorf("ModifiedFields len = %d, want 1", len(nc.ModifiedFields))
	}
	if nc.ModifiedFields["status"].OldValue != "up" {
		t.Errorf("ModifiedFields[status].OldValue = %v, want %q", nc.ModifiedFields["status"].OldValue, "up")
	}
}

// TestFieldChangeFields 验证 FieldChange 结构体字段。
func TestFieldChangeFields(t *testing.T) {
	fc := FieldChange{
		OldValue: "active",
		NewValue: "inactive",
	}

	if fc.OldValue != "active" {
		t.Errorf("OldValue = %v, want %q", fc.OldValue, "active")
	}
	if fc.NewValue != "inactive" {
		t.Errorf("NewValue = %v, want %q", fc.NewValue, "inactive")
	}

	// 数值类型
	fcNum := FieldChange{OldValue: int(42), NewValue: float64(99.5)}
	if fcNum.OldValue != int(42) {
		t.Errorf("OldValue = %v, want 42", fcNum.OldValue)
	}
}

// TestRelChangeFields 验证 RelChange 结构体字段。
func TestRelChangeFields(t *testing.T) {
	rc := RelChange{
		Type: "HAS_INTERFACE",
		From: "device:001",
		To:   "iface:001_eth0",
		AddedFields: map[string]any{
			"bandwidth": 1000,
		},
		RemovedFields: map[string]any{
			"old_prop": "val",
		},
		ModifiedFields: map[string]FieldChange{
			"mtu": {OldValue: 1500, NewValue: 9000},
		},
	}

	if rc.Type != "HAS_INTERFACE" {
		t.Errorf("Type = %q, want %q", rc.Type, "HAS_INTERFACE")
	}
	if rc.From != "device:001" {
		t.Errorf("From = %q, want %q", rc.From, "device:001")
	}
	if rc.To != "iface:001_eth0" {
		t.Errorf("To = %q, want %q", rc.To, "iface:001_eth0")
	}
	if len(rc.AddedFields) != 1 {
		t.Errorf("AddedFields len = %d, want 1", len(rc.AddedFields))
	}
	if len(rc.RemovedFields) != 1 {
		t.Errorf("RemovedFields len = %d, want 1", len(rc.RemovedFields))
	}
	if len(rc.ModifiedFields) != 1 {
		t.Errorf("ModifiedFields len = %d, want 1", len(rc.ModifiedFields))
	}
}

// TestSnapshotDiffExtendedFields 验证 SnapshotDiff 的 ChangedNodes/ChangedRels 新字段。
func TestSnapshotDiffExtendedFields(t *testing.T) {
	diff := SnapshotDiff{
		AddedNodes:   []assembler.Node{{Labels: []string{"Device"}, URI: "device:NEW"}},
		RemovedNodes: []assembler.Node{{Labels: []string{"Device"}, URI: "device:OLD"}},
		AddedRels:    []assembler.Relation{{Type: "CONNECTS", From: "a", To: "b"}},
		RemovedRels:  []assembler.Relation{{Type: "CONNECTS", From: "c", To: "d"}},
		ChangedNodes: []NodeChange{
			{URI: "device:001", Label: "Device", ModifiedFields: map[string]FieldChange{
				"status": {OldValue: "up", NewValue: "down"},
			}},
		},
		ChangedRels: []RelChange{
			{Type: "HAS_INTERFACE", From: "device:001", To: "iface:001_eth0"},
		},
	}

	// 验证原有字段不受影响
	if len(diff.AddedNodes) != 1 {
		t.Errorf("AddedNodes = %d, want 1", len(diff.AddedNodes))
	}
	if len(diff.RemovedNodes) != 1 {
		t.Errorf("RemovedNodes = %d, want 1", len(diff.RemovedNodes))
	}

	// 验证新字段
	if len(diff.ChangedNodes) != 1 {
		t.Fatalf("ChangedNodes = %d, want 1", len(diff.ChangedNodes))
	}
	if diff.ChangedNodes[0].URI != "device:001" {
		t.Errorf("ChangedNodes[0].URI = %q, want %q", diff.ChangedNodes[0].URI, "device:001")
	}
	if diff.ChangedNodes[0].Label != "Device" {
		t.Errorf("ChangedNodes[0].Label = %q, want %q", diff.ChangedNodes[0].Label, "Device")
	}

	if len(diff.ChangedRels) != 1 {
		t.Fatalf("ChangedRels = %d, want 1", len(diff.ChangedRels))
	}
	if diff.ChangedRels[0].Type != "HAS_INTERFACE" {
		t.Errorf("ChangedRels[0].Type = %q, want %q", diff.ChangedRels[0].Type, "HAS_INTERFACE")
	}
}

// TestCompareProps_AllCases 验证 added/removed/modified 三分类。
func TestCompareProps_AllCases(t *testing.T) {
	a := map[string]any{
		"hostname": "router-01",
		"status":   "up",
		"mtu":      1500,
		"removed":  "gone",
	}
	b := map[string]any{
		"hostname": "router-01", // 相同
		"status":   "down",      // modified
		"mtu":      1500,        // 相同
		"added":    "new",       // added
	}

	added, removed, modified := compareProps(a, b)

	// added: b 有 a 无
	if len(added) != 1 {
		t.Errorf("added len = %d, want 1", len(added))
	}
	if added["added"] != "new" {
		t.Errorf("added[added] = %v, want %q", added["added"], "new")
	}

	// removed: a 有 b 无
	if len(removed) != 1 {
		t.Errorf("removed len = %d, want 1", len(removed))
	}
	if removed["removed"] != "gone" {
		t.Errorf("removed[removed] = %v, want %q", removed["removed"], "gone")
	}

	// modified: 两边都有但值不同
	if len(modified) != 1 {
		t.Errorf("modified len = %d, want 1", len(modified))
	}
	fc, ok := modified["status"]
	if !ok {
		t.Fatal("modified[status] not found")
	}
	if fc.OldValue != "up" {
		t.Errorf("modified[status].OldValue = %v, want %q", fc.OldValue, "up")
	}
	if fc.NewValue != "down" {
		t.Errorf("modified[status].NewValue = %v, want %q", fc.NewValue, "down")
	}
}

// TestCompareProps_EmptyMaps 空 map 对比返回三个空 map。
func TestCompareProps_EmptyMaps(t *testing.T) {
	a := map[string]any{}
	b := map[string]any{}

	added, removed, modified := compareProps(a, b)

	if len(added) != 0 {
		t.Errorf("added len = %d, want 0", len(added))
	}
	if len(removed) != 0 {
		t.Errorf("removed len = %d, want 0", len(removed))
	}
	if len(modified) != 0 {
		t.Errorf("modified len = %d, want 0", len(modified))
	}
}

// TestValuesEqual_NumericNormalization int(42) vs float64(42.0) 相等。
func TestValuesEqual_NumericNormalization(t *testing.T) {
	if !valuesEqual(int(42), float64(42.0)) {
		t.Error("valuesEqual(int(42), float64(42.0)) = false, want true")
	}
	if !valuesEqual(int64(42), float64(42.0)) {
		t.Error("valuesEqual(int64(42), float64(42.0)) = false, want true")
	}
	if !valuesEqual(int(42), int64(42)) {
		t.Error("valuesEqual(int(42), int64(42)) = false, want true")
	}
}

// TestValuesEqual_StringInequality "up" vs "down" 不相等。
func TestValuesEqual_StringInequality(t *testing.T) {
	if valuesEqual("up", "down") {
		t.Error("valuesEqual(\"up\", \"down\") = true, want false")
	}
}

// TestValuesEqual_SameString 相同字符串相等。
func TestValuesEqual_SameString(t *testing.T) {
	if !valuesEqual("up", "up") {
		t.Error("valuesEqual(\"up\", \"up\") = false, want true")
	}
}

// TestToFloat64_Types int/int64/float64/string 的转换。
func TestToFloat64_Types(t *testing.T) {
	// int
	v, ok := toFloat64(int(42))
	if !ok || v != 42.0 {
		t.Errorf("toFloat64(int(42)) = (%v, %v), want (42.0, true)", v, ok)
	}

	// int64
	v, ok = toFloat64(int64(100))
	if !ok || v != 100.0 {
		t.Errorf("toFloat64(int64(100)) = (%v, %v), want (100.0, true)", v, ok)
	}

	// float64
	v, ok = toFloat64(float64(3.14))
	if !ok || v != 3.14 {
		t.Errorf("toFloat64(float64(3.14)) = (%v, %v), want (3.14, true)", v, ok)
	}

	// string (不支持)
	_, ok = toFloat64("hello")
	if ok {
		t.Error("toFloat64(\"hello\") should return false")
	}

	// nil (不支持)
	_, ok = toFloat64(nil)
	if ok {
		t.Error("toFloat64(nil) should return false")
	}
}

// ---------------------------------------------------------------------------
// V1-12: LocalDiff 属性级对比测试
// ---------------------------------------------------------------------------

// TestLocalDiff_ChangedNodes_AddedFields 验证 b 比 a 多出的属性被正确归入 AddedFields。
func TestLocalDiff_ChangedNodes_AddedFields(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{
			{Labels: []string{"Resource", "Device"}, URI: "device:001", Props: map[string]any{"hostname": "r1"}},
		},
		nil,
	)
	writeTestSnapshot(t, snapDir, "snap-b",
		[]yamlNodeItem{
			{Labels: []string{"Resource", "Device"}, URI: "device:001", Props: map[string]any{"hostname": "r1", "status": "up"}},
		},
		nil,
	)

	gdb := &mockGraphDB{}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	diff, err := mgr.LocalDiff("snap-a", "snap-b")
	if err != nil {
		t.Fatalf("LocalDiff() error = %v", err)
	}

	if len(diff.ChangedNodes) != 1 {
		t.Fatalf("ChangedNodes = %d, want 1", len(diff.ChangedNodes))
	}
	nc := diff.ChangedNodes[0]
	if nc.URI != "device:001" {
		t.Errorf("URI = %q, want %q", nc.URI, "device:001")
	}
	if nc.Label != "Device" {
		t.Errorf("Label = %q, want %q (MostSpecificLabel)", nc.Label, "Device")
	}
	if len(nc.AddedFields) != 1 {
		t.Errorf("AddedFields len = %d, want 1", len(nc.AddedFields))
	}
	if nc.AddedFields["status"] != "up" {
		t.Errorf("AddedFields[status] = %v, want %q", nc.AddedFields["status"], "up")
	}
	if len(nc.RemovedFields) != 0 {
		t.Errorf("RemovedFields should be empty, got %v", nc.RemovedFields)
	}
	if len(nc.ModifiedFields) != 0 {
		t.Errorf("ModifiedFields should be empty, got %v", nc.ModifiedFields)
	}
}

// TestLocalDiff_ChangedNodes_RemovedFields 验证 b 比 a 少的属性被正确归入 RemovedFields。
func TestLocalDiff_ChangedNodes_RemovedFields(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001", Props: map[string]any{"hostname": "r1", "old_field": "val"}},
		},
		nil,
	)
	writeTestSnapshot(t, snapDir, "snap-b",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001", Props: map[string]any{"hostname": "r1"}},
		},
		nil,
	)

	gdb := &mockGraphDB{}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	diff, err := mgr.LocalDiff("snap-a", "snap-b")
	if err != nil {
		t.Fatalf("LocalDiff() error = %v", err)
	}

	if len(diff.ChangedNodes) != 1 {
		t.Fatalf("ChangedNodes = %d, want 1", len(diff.ChangedNodes))
	}
	nc := diff.ChangedNodes[0]
	if len(nc.RemovedFields) != 1 {
		t.Errorf("RemovedFields len = %d, want 1", len(nc.RemovedFields))
	}
	if nc.RemovedFields["old_field"] != "val" {
		t.Errorf("RemovedFields[old_field] = %v, want %q", nc.RemovedFields["old_field"], "val")
	}
	if len(nc.AddedFields) != 0 {
		t.Errorf("AddedFields should be empty, got %v", nc.AddedFields)
	}
}

// TestLocalDiff_ChangedNodes_ModifiedFields 验证值不同的属性被正确归入 ModifiedFields，
// 并验证 int vs float64 数值归一化（int(42) vs float64(42.0) 不应被视为 modified）。
func TestLocalDiff_ChangedNodes_ModifiedFields(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001", Props: map[string]any{"status": "up", "mtu": 1500}},
		},
		nil,
	)
	writeTestSnapshot(t, snapDir, "snap-b",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001", Props: map[string]any{"status": "down", "mtu": 9000}},
		},
		nil,
	)

	gdb := &mockGraphDB{}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	diff, err := mgr.LocalDiff("snap-a", "snap-b")
	if err != nil {
		t.Fatalf("LocalDiff() error = %v", err)
	}

	if len(diff.ChangedNodes) != 1 {
		t.Fatalf("ChangedNodes = %d, want 1", len(diff.ChangedNodes))
	}
	nc := diff.ChangedNodes[0]
	if len(nc.ModifiedFields) != 2 {
		t.Fatalf("ModifiedFields len = %d, want 2", len(nc.ModifiedFields))
	}
	statusFC, ok := nc.ModifiedFields["status"]
	if !ok {
		t.Fatal("ModifiedFields[status] not found")
	}
	if statusFC.OldValue != "up" || statusFC.NewValue != "down" {
		t.Errorf("status FieldChange = {%v, %v}, want {up, down}", statusFC.OldValue, statusFC.NewValue)
	}
}

// TestLocalDiff_ChangedNodes_NumericNormalization 验证 int(42) vs float64(42.0) 不被视为 modified。
func TestLocalDiff_ChangedNodes_NumericNormalization(t *testing.T) {
	snapDir := t.TempDir()
	// YAML 加载整数为 int，这里手动构造 float64 验证归一化
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001", Props: map[string]any{"count": 42}},
		},
		nil,
	)
	writeTestSnapshot(t, snapDir, "snap-b",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001", Props: map[string]any{"count": 42}},
		},
		nil,
	)

	gdb := &mockGraphDB{}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	diff, err := mgr.LocalDiff("snap-a", "snap-b")
	if err != nil {
		t.Fatalf("LocalDiff() error = %v", err)
	}

	// 值相同，不应出现在 ChangedNodes
	if len(diff.ChangedNodes) != 0 {
		t.Errorf("ChangedNodes should be empty for identical props, got %d", len(diff.ChangedNodes))
	}
}

// TestLocalDiff_ChangedNodes_NoChange 验证属性完全相同的节点不出现在 ChangedNodes 中。
func TestLocalDiff_ChangedNodes_NoChange(t *testing.T) {
	snapDir := t.TempDir()
	props := map[string]any{"hostname": "r1", "status": "up", "mtu": 1500}
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001", Props: props},
			{Labels: []string{"Device"}, URI: "device:002", Props: map[string]any{"hostname": "r2"}},
		},
		nil,
	)
	writeTestSnapshot(t, snapDir, "snap-b",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001", Props: props},
			{Labels: []string{"Device"}, URI: "device:002", Props: map[string]any{"hostname": "r2"}},
		},
		nil,
	)

	gdb := &mockGraphDB{}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	diff, err := mgr.LocalDiff("snap-a", "snap-b")
	if err != nil {
		t.Fatalf("LocalDiff() error = %v", err)
	}

	if len(diff.ChangedNodes) != 0 {
		t.Errorf("ChangedNodes = %d, want 0 (no property diff)", len(diff.ChangedNodes))
	}
	if len(diff.ChangedRels) != 0 {
		t.Errorf("ChangedRels = %d, want 0", len(diff.ChangedRels))
	}
	// 原有逻辑：无增删
	if len(diff.AddedNodes) != 0 {
		t.Errorf("AddedNodes = %d, want 0", len(diff.AddedNodes))
	}
	if len(diff.RemovedNodes) != 0 {
		t.Errorf("RemovedNodes = %d, want 0", len(diff.RemovedNodes))
	}
}

// TestLocalDiff_ChangedRels_ModifiedProps 验证关系 Props 差异被正确归入 ChangedRels。
func TestLocalDiff_ChangedRels_ModifiedProps(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001"},
			{Labels: []string{"Device"}, URI: "device:002"},
		},
		[]yamlRelItem{
			{Type: "CONNECTS", From: "device:001", To: "device:002", Props: map[string]any{"bandwidth": 100}},
		},
	)
	writeTestSnapshot(t, snapDir, "snap-b",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001"},
			{Labels: []string{"Device"}, URI: "device:002"},
		},
		[]yamlRelItem{
			{Type: "CONNECTS", From: "device:001", To: "device:002", Props: map[string]any{"bandwidth": 200}},
		},
	)

	gdb := &mockGraphDB{}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	diff, err := mgr.LocalDiff("snap-a", "snap-b")
	if err != nil {
		t.Fatalf("LocalDiff() error = %v", err)
	}

	if len(diff.ChangedRels) != 1 {
		t.Fatalf("ChangedRels = %d, want 1", len(diff.ChangedRels))
	}
	rc := diff.ChangedRels[0]
	if rc.Type != "CONNECTS" {
		t.Errorf("Type = %q, want %q", rc.Type, "CONNECTS")
	}
	if rc.From != "device:001" {
		t.Errorf("From = %q, want %q", rc.From, "device:001")
	}
	if rc.To != "device:002" {
		t.Errorf("To = %q, want %q", rc.To, "device:002")
	}
	if len(rc.ModifiedFields) != 1 {
		t.Fatalf("ModifiedFields len = %d, want 1", len(rc.ModifiedFields))
	}
	bwFC, ok := rc.ModifiedFields["bandwidth"]
	if !ok {
		t.Fatal("ModifiedFields[bandwidth] not found")
	}
	if bwFC.OldValue != 100 || bwFC.NewValue != 200 {
		t.Errorf("bandwidth FieldChange = {%v, %v}, want {100, 200}", bwFC.OldValue, bwFC.NewValue)
	}
}

// TestLocalDiff_ExistingLogicPreserved 验证属性级对比不影响原有 AddedNodes/RemovedNodes 逻辑。
func TestLocalDiff_ExistingLogicPreserved(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-a",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:001", Props: map[string]any{"hostname": "r1"}},
			{Labels: []string{"Device"}, URI: "device:002", Props: map[string]any{"hostname": "r2"}},
		},
		[]yamlRelItem{
			{Type: "CONNECTS", From: "device:001", To: "device:002"},
		},
	)
	writeTestSnapshot(t, snapDir, "snap-b",
		[]yamlNodeItem{
			{Labels: []string{"Device"}, URI: "device:002", Props: map[string]any{"hostname": "r2"}},
			{Labels: []string{"Device"}, URI: "device:003", Props: map[string]any{"hostname": "r3"}},
		},
		[]yamlRelItem{
			{Type: "CONNECTS", From: "device:002", To: "device:003"},
		},
	)

	gdb := &mockGraphDB{}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	diff, err := mgr.LocalDiff("snap-a", "snap-b")
	if err != nil {
		t.Fatalf("LocalDiff() error = %v", err)
	}

	// 原有增删逻辑
	if len(diff.AddedNodes) != 1 || diff.AddedNodes[0].URI != "device:003" {
		t.Errorf("AddedNodes = %v, want [device:003]", diff.AddedNodes)
	}
	if len(diff.RemovedNodes) != 1 || diff.RemovedNodes[0].URI != "device:001" {
		t.Errorf("RemovedNodes = %v, want [device:001]", diff.RemovedNodes)
	}
	if len(diff.AddedRels) != 1 {
		t.Errorf("AddedRels = %d, want 1", len(diff.AddedRels))
	}
	if len(diff.RemovedRels) != 1 {
		t.Errorf("RemovedRels = %d, want 1", len(diff.RemovedRels))
	}

	// device:002 存在于两边且属性相同，ChangedNodes 应为空
	if len(diff.ChangedNodes) != 0 {
		t.Errorf("ChangedNodes = %d, want 0 (device:002 props identical)", len(diff.ChangedNodes))
	}
}
