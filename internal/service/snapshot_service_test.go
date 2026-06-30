// Package service 实现业务编排层
package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/snapshot"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// TC-SS01: NewSnapshotService 构造验证
// ---------------------------------------------------------------------------

func TestNewSnapshotService(t *testing.T) {
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()
	mgr := snapshot.NewSnapshotManager(gdb, lock, t.TempDir(), 5)

	svc := NewSnapshotService(mgr)
	if svc == nil {
		t.Fatal("NewSnapshotService() returned nil")
	}
}

// ---------------------------------------------------------------------------
// TC-SS02: List 调用 manager.List 透传
// ---------------------------------------------------------------------------

func TestSnapshotService_List(t *testing.T) {
	// 无快照时返回空列表
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()
	snapDir := t.TempDir()
	mgr := snapshot.NewSnapshotManager(gdb, lock, snapDir, 5)
	svc := NewSnapshotService(mgr)

	metas, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metas) != 0 {
		t.Errorf("List() returned %d snapshots, want 0", len(metas))
	}
}

// ---------------------------------------------------------------------------
// TC-SS03: Diff 调用 manager.Diff 透传
// ---------------------------------------------------------------------------

func TestSnapshotService_Diff(t *testing.T) {
	gdb := &mockGraphDB{
		queryResults: []map[string]any{},
	}
	lock := snapshot.NewGraphLock()
	snapDir := t.TempDir()

	// 写入两个快照文件
	writeSvcTestSnapshot(t, snapDir, "snap-a",
		[]svcYamlNodeItem{{Labels: []string{"Device"}, URI: "device:001"}}, nil)
	writeSvcTestSnapshot(t, snapDir, "snap-b",
		[]svcYamlNodeItem{{Labels: []string{"Device"}, URI: "device:002"}}, nil)

	mgr := snapshot.NewSnapshotManager(gdb, lock, snapDir, 5)
	svc := NewSnapshotService(mgr)

	diff, err := svc.Diff(context.Background(), "snap-a", "snap-b")
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if diff == nil {
		t.Fatal("Diff() returned nil")
	}
}

// ---------------------------------------------------------------------------
// TC-SS04: Restore 调用 manager.Restore 透传
// ---------------------------------------------------------------------------

func TestSnapshotService_Restore(t *testing.T) {
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()
	snapDir := t.TempDir()

	// 写入快照文件供 EnsureLoaded 读取
	writeSvcTestSnapshot(t, snapDir, "snap-001",
		[]svcYamlNodeItem{{Labels: []string{"Device"}, URI: "device:001"}}, nil)

	mgr := snapshot.NewSnapshotManager(gdb, lock, snapDir, 5)
	svc := NewSnapshotService(mgr)

	err := svc.Restore(context.Background(), "snap-001")
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// ClearDB("default") 应被调用（Restore 内部逻辑）
	if len(gdb.clearDBCalls) != 1 || gdb.clearDBCalls[0] != "default" {
		t.Errorf("ClearDB calls = %v, want [default]", gdb.clearDBCalls)
	}
}

// ---------------------------------------------------------------------------
// TC-SS05: Create 成功路径
// ---------------------------------------------------------------------------

func TestSnapshotService_Create_Success(t *testing.T) {
	gdb := &mockGraphDB{
		queryResults: []map[string]any{
			{"labels": []any{"Device"}, "uri": "device:SN001", "props": map[string]any{"hostname": "router-01"}},
		},
	}
	lock := snapshot.NewGraphLock()
	snapDir := t.TempDir()

	mgr := snapshot.NewSnapshotManager(gdb, lock, snapDir, 5)
	svc := NewSnapshotService(mgr)

	meta, err := svc.Create(context.Background(), "snap-svc-001")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if meta.Name != "snap-svc-001" {
		t.Errorf("meta.Name = %q, want %q", meta.Name, "snap-svc-001")
	}
}

// ---------------------------------------------------------------------------
// TC-SS06: Create 失败路径
// ---------------------------------------------------------------------------

func TestSnapshotService_Create_QueryError(t *testing.T) {
	wantErr := errors.New("neo4j connection refused")
	gdb := &mockGraphDB{queryErr: wantErr}
	lock := snapshot.NewGraphLock()
	snapDir := t.TempDir()

	mgr := snapshot.NewSnapshotManager(gdb, lock, snapDir, 5)
	svc := NewSnapshotService(mgr)

	_, err := svc.Create(context.Background(), "snap-err")
	if err == nil {
		t.Fatal("Create() should return error when Query fails")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TC-SS07: Delete 调用 manager.Delete 透传
// ---------------------------------------------------------------------------

func TestSnapshotService_Delete(t *testing.T) {
	gdb := &mockGraphDB{
		queryResults: []map[string]any{
			{"labels": []any{"Device"}, "uri": "device:SN001", "props": map[string]any{"hostname": "r1"}},
		},
		hasDBResult: map[string]bool{"snap-del": true},
	}
	lock := snapshot.NewGraphLock()
	snapDir := t.TempDir()

	mgr := snapshot.NewSnapshotManager(gdb, lock, snapDir, 5)
	svc := NewSnapshotService(mgr)

	// 先创建快照文件
	if _, err := mgr.Create(context.Background(), "snap-del"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err := svc.Delete(context.Background(), "snap-del")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// ClearDB 应被调用
	found := false
	for _, call := range gdb.clearDBCalls {
		if call == "snap-del" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ClearDB should clear snap-del, calls = %v", gdb.clearDBCalls)
	}
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

type svcYamlNodeItem struct {
	Labels []string       `yaml:"labels"`
	URI    string         `yaml:"uri"`
	Props  map[string]any `yaml:"props,omitempty"`
}

type svcYamlRelItem struct {
	Type string `yaml:"type"`
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type svcYamlMetaDoc struct {
	Kind      string    `yaml:"kind"`
	Name      string    `yaml:"name"`
	CreatedAt time.Time `yaml:"created_at"`
	NodeCount int       `yaml:"node_count"`
	RelCount  int       `yaml:"rel_count"`
}

type svcYamlNodesDoc struct {
	Kind  string            `yaml:"kind"`
	Items []svcYamlNodeItem `yaml:"items"`
}

type svcYamlRelsDoc struct {
	Kind  string           `yaml:"kind"`
	Items []svcYamlRelItem `yaml:"items"`
}

// writeSvcTestSnapshot 写入测试用 YAML 快照文件（与 snapshot 包的多文档格式一致）。
func writeSvcTestSnapshot(t *testing.T, dir, name string, nodes []svcYamlNodeItem, rels []svcYamlRelItem) {
	t.Helper()
	filePath := filepath.Join(dir, name+".yaml")
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	defer f.Close()

	enc := yaml.NewEncoder(f)
	defer enc.Close()

	meta := svcYamlMetaDoc{
		Kind: "SnapshotMeta", Name: name, CreatedAt: time.Now(),
		NodeCount: len(nodes), RelCount: len(rels),
	}
	if err := enc.Encode(meta); err != nil {
		t.Fatalf("encode meta: %v", err)
	}
	if err := enc.Encode(svcYamlNodesDoc{Kind: "Nodes", Items: nodes}); err != nil {
		t.Fatalf("encode nodes: %v", err)
	}
	if err := enc.Encode(svcYamlRelsDoc{Kind: "Relations", Items: rels}); err != nil {
		t.Fatalf("encode rels: %v", err)
	}
}
