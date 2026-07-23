package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/repository"
	"gopkg.in/yaml.v3"
)

// ──────────────────────────────
// List with Repository
// ──────────────────────────────

func TestSnapshotManager_List_WithRepo(t *testing.T) {
	gdb := &mockGraphDB{}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	repo := repository.NewMemSnapshotRepository()
	// 预填充 repo 数据
	repo.Create(context.Background(), &repository.SnapshotRecord{
		Name:      "snap-repo-001",
		CreatedAt: time.Now(),
		NodeCount: 10,
		RelCount:  5,
		FilePath:  "/data/snap-repo-001.yaml",
		Status:    "active",
	})

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5, WithSnapshotRepository(repo))

	metas, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(metas))
	}
	if metas[0].Name != "snap-repo-001" {
		t.Errorf("name = %q, want snap-repo-001", metas[0].Name)
	}
	if metas[0].NodeCount != 10 {
		t.Errorf("node_count = %d, want 10", metas[0].NodeCount)
	}
}

func TestSnapshotManager_List_RepoError_FallbackCache(t *testing.T) {
	gdb := &mockGraphDB{}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	// 写入一个有效的 YAML 快照文件
	writeTestSnapshotYAML(t, snapDir, "snap-cache-001", 3, 2)

	// 使用一个会失败的 repo
	mgr := NewSnapshotManager(gdb, lock, snapDir, 5, WithSnapshotRepository(&failListRepo{}))

	metas, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List should fallback to cache, got error: %v", err)
	}
	// 应该从 warmCache 获取
	if len(metas) != 1 {
		t.Fatalf("expected 1 snapshot from cache, got %d", len(metas))
	}
	if metas[0].Name != "snap-cache-001" {
		t.Errorf("name = %q, want snap-cache-001", metas[0].Name)
	}
}

// failListRepo 模拟 List 失败的 SnapshotRepository。
type failListRepo struct{}

func (f *failListRepo) Create(ctx context.Context, rec *repository.SnapshotRecord) error {
	return nil
}
func (f *failListRepo) GetByName(ctx context.Context, name string) (*repository.SnapshotRecord, error) {
	return nil, repository.ErrSnapshotNotFound
}
func (f *failListRepo) List(ctx context.Context) ([]repository.SnapshotRecord, error) {
	return nil, fmt.Errorf("connection refused")
}
func (f *failListRepo) Delete(ctx context.Context, name string) error { return nil }
func (f *failListRepo) UpdateStatus(ctx context.Context, name, status string) error {
	return nil
}

// ──────────────────────────────
// Delete error paths
// ──────────────────────────────

func TestSnapshotManager_Delete_HasDBError(t *testing.T) {
	gdb := &mockGraphDB{hasDBErr: fmt.Errorf("neo4j unreachable")}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)
	err := mgr.Delete(context.Background(), "snap-x")
	if err == nil {
		t.Fatal("expected error when HasDB fails")
	}
}

func TestSnapshotManager_Delete_ClearDBError(t *testing.T) {
	gdb := &mockGraphDB{
		hasDBResult:  map[string]bool{"snap-x": true},
		clearDBErr:   fmt.Errorf("clear failed"),
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)
	err := mgr.Delete(context.Background(), "snap-x")
	if err == nil {
		t.Fatal("expected error when ClearDB fails")
	}
}

func TestSnapshotManager_Delete_WithRepo(t *testing.T) {
	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-del": true},
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	repo := repository.NewMemSnapshotRepository()
	repo.Create(context.Background(), &repository.SnapshotRecord{
		Name:      "snap-del",
		CreatedAt: time.Now(),
		NodeCount: 5,
		RelCount:  3,
		Status:    "active",
	})

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5, WithSnapshotRepository(repo))
	err := mgr.Delete(context.Background(), "snap-del")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	// 验证 repo 中已删除
	_, getErr := repo.GetByName(context.Background(), "snap-del")
	if getErr == nil {
		t.Error("expected snapshot to be deleted from repo")
	}
}

// ──────────────────────────────
// Diff with data
// ──────────────────────────────

func TestSnapshotManager_Diff_WithResults(t *testing.T) {
	// 模拟 Diff 查询返回数据
	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-a": true, "snap-b": true},
		queryResultsSeq: [][]map[string]any{
			// 1. added nodes (b has, a doesn't)
			{
				{"labels": []any{"Device"}, "uri": "device:NEW01", "props": map[string]any{"hostname": "new-r1"}},
			},
			// 2. removed nodes (a has, b doesn't)
			{
				{"labels": []any{"Device"}, "uri": "device:OLD01", "props": map[string]any{"hostname": "old-r1"}},
			},
			// 3. added rels
			{
				{"type": "HAS_INTERFACE", "from": "device:NEW01", "to": "iface:ETH0", "props": map[string]any{}},
			},
			// 4. removed rels
			{
				{"type": "CONNECTS_TO", "from": "device:OLD01", "to": "device:OLD02", "props": map[string]any{}},
			},
			// 5. changed nodes
			{
				{"uri": "device:CHANGED01", "labels": []any{"Device"},
					"aProps": map[string]any{"hostname": "old-name"}, "bProps": map[string]any{"hostname": "new-name"}},
			},
		},
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	// 创建两个快照 YAML 文件
	writeTestSnapshotYAML(t, snapDir, "snap-a", 2, 1)
	writeTestSnapshotYAML(t, snapDir, "snap-b", 3, 2)

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)
	diff, err := mgr.Diff(context.Background(), "snap-a", "snap-b")
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if len(diff.AddedNodes) != 1 {
		t.Errorf("AddedNodes = %d, want 1", len(diff.AddedNodes))
	}
	if len(diff.RemovedNodes) != 1 {
		t.Errorf("RemovedNodes = %d, want 1", len(diff.RemovedNodes))
	}
	if len(diff.AddedRels) != 1 {
		t.Errorf("AddedRels = %d, want 1", len(diff.AddedRels))
	}
	if len(diff.RemovedRels) != 1 {
		t.Errorf("RemovedRels = %d, want 1", len(diff.RemovedRels))
	}
	if len(diff.ChangedNodes) != 1 {
		t.Errorf("ChangedNodes = %d, want 1", len(diff.ChangedNodes))
	}
}

func TestSnapshotManager_Diff_QueryError(t *testing.T) {
	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-a": true, "snap-b": true},
		queryErr:    fmt.Errorf("cypher execution failed"),
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	writeTestSnapshotYAML(t, snapDir, "snap-a", 2, 1)
	writeTestSnapshotYAML(t, snapDir, "snap-b", 3, 2)

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)
	_, err := mgr.Diff(context.Background(), "snap-a", "snap-b")
	if err == nil {
		t.Fatal("expected error when query fails")
	}
}

func TestSnapshotManager_Diff_EnsureLoadedError(t *testing.T) {
	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{}, // snap-a 不存在
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	// 不创建 snap-a YAML → EnsureLoaded 会失败
	writeTestSnapshotYAML(t, snapDir, "snap-b", 3, 2)

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)
	_, err := mgr.Diff(context.Background(), "snap-a", "snap-b")
	if err == nil {
		t.Fatal("expected error when EnsureLoaded fails")
	}
}

// ──────────────────────────────
// Restore error paths
// ──────────────────────────────

func TestSnapshotManager_Restore_BulkCreateError(t *testing.T) {
	// HasDB 返回 false → EnsureLoaded 尝试从 YAML 导入 → BulkCreate 失败
	gdb := &mockGraphDB{
		hasDBResult:   map[string]bool{"snap-r": false},
		bulkCreateErr: fmt.Errorf("bulk create failed"),
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()
	writeTestSnapshotYAML(t, snapDir, "snap-r", 1, 0)

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)
	err := mgr.Restore(context.Background(), "snap-r")
	if err == nil {
		t.Fatal("expected error when BulkCreate fails")
	}
}

func TestSnapshotManager_Restore_ClearDBError(t *testing.T) {
	// HasDB 返回 true → EnsureLoaded 成功 → ClearDB("default") 失败
	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-r": true},
		clearDBErr:  fmt.Errorf("clear failed"),
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()
	writeTestSnapshotYAML(t, snapDir, "snap-r", 1, 0)

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)
	err := mgr.Restore(context.Background(), "snap-r")
	if err == nil {
		t.Fatal("expected error when ClearDB fails")
	}
}

func TestSnapshotManager_Restore_CloneDBError(t *testing.T) {
	// HasDB 返回 true → EnsureLoaded 成功 → ClearDB 成功 → CloneDB 失败
	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-r": true},
		cloneDBErr:  fmt.Errorf("clone failed"),
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()
	writeTestSnapshotYAML(t, snapDir, "snap-r", 1, 0)

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)
	err := mgr.Restore(context.Background(), "snap-r")
	if err == nil {
		t.Fatal("expected error when CloneDB fails")
	}
}

// ──────────────────────────────
// warmCache error paths
// ──────────────────────────────

func TestSnapshotManager_WarmCache_ReadDirError(t *testing.T) {
	gdb := &mockGraphDB{}
	lock := NewGraphLock()
	// 使用不存在的目录
	snapDir := "/nonexistent/snapshot/dir/xyz"

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)
	_, err := mgr.List(context.Background())
	if err == nil {
		t.Fatal("expected error when snap dir doesn't exist")
	}
}

func TestSnapshotManager_WarmCache_SkipsInvalidYAML(t *testing.T) {
	gdb := &mockGraphDB{}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	// 写入一个损坏的 YAML
	os.WriteFile(filepath.Join(snapDir, "bad.yaml"), []byte("not: valid: yaml: {{{"), 0644)
	// 写入一个有效的 YAML
	writeTestSnapshotYAML(t, snapDir, "good-snap", 2, 1)

	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)
	metas, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	// 应该只有 1 个有效快照
	if len(metas) != 1 {
		t.Errorf("expected 1 valid snapshot, got %d", len(metas))
	}
}

// ──────────────────────────────
// Create with repo
// ──────────────────────────────

func TestSnapshotManager_Create_WithRepo(t *testing.T) {
	gdb := &mockGraphDB{
		queryResults: []map[string]any{
			{"labels": []any{"Device"}, "uri": "device:C01", "props": map[string]any{"hostname": "r1"}},
		},
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()

	repo := repository.NewMemSnapshotRepository()
	mgr := NewSnapshotManager(gdb, lock, snapDir, 5, WithSnapshotRepository(repo))

	meta, err := mgr.Create(context.Background(), "snap-with-repo")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if meta.Name != "snap-with-repo" {
		t.Errorf("name = %q", meta.Name)
	}

	// 验证 repo 中有记录
	rec, getErr := repo.GetByName(context.Background(), "snap-with-repo")
	if getErr != nil {
		t.Fatalf("repo GetByName failed: %v", getErr)
	}
	if rec.Name != "snap-with-repo" {
		t.Errorf("repo record name = %q", rec.Name)
	}
}

// ──────────────────────────────
// 辅助函数
// ──────────────────────────────

func writeTestSnapshotYAML(t *testing.T, dir, name string, nodeCount, relCount int) {
	t.Helper()

	type yamlNode struct {
		Labels []string       `yaml:"labels"`
		URI    string         `yaml:"uri"`
		Props  map[string]any `yaml:"props"`
	}
	type yamlRel struct {
		Type  string `yaml:"type"`
		From  string `yaml:"from"`
		To    string `yaml:"to"`
		Props map[string]any `yaml:"props,omitempty"`
	}

	nodes := make([]yamlNode, 0, nodeCount)
	for i := 0; i < nodeCount; i++ {
		nodes = append(nodes, yamlNode{
			Labels: []string{"Device"},
			URI:    fmt.Sprintf("device:%s-%02d", name, i+1),
			Props:  map[string]any{"hostname": fmt.Sprintf("router-%02d", i+1)},
		})
	}
	rels := make([]yamlRel, 0, relCount)
	for i := 0; i < relCount; i++ {
		rels = append(rels, yamlRel{
			Type: "HAS_INTERFACE",
			From: fmt.Sprintf("device:%s-%02d", name, i+1),
			To:   fmt.Sprintf("iface:%s-%02d", name, i+1),
		})
	}

	// 构建多文档 YAML
	metaDoc := map[string]any{
		"kind":       "SnapshotMeta",
		"name":       name,
		"created_at": time.Now().Format(time.RFC3339),
		"node_count": nodeCount,
		"rel_count":  relCount,
	}

	f, err := os.Create(filepath.Join(dir, name+".yaml"))
	if err != nil {
		t.Fatalf("create yaml: %v", err)
	}
	defer f.Close()

	enc := yaml.NewEncoder(f)
	enc.Encode(metaDoc)
	enc.Encode(map[string]any{"kind": "Nodes", "items": nodes})
	enc.Encode(map[string]any{"kind": "Relations", "items": rels})
	enc.Close()
}

// 确保 assembler 包被使用
var _ assembler.Node
