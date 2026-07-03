// Package service 实现业务编排层
package service

import (
	"context"
	"errors"
	"fmt"
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
// TC-SS08: List 缓存效果验证 — MetaCache 在 Manager 层透明工作
// ---------------------------------------------------------------------------

func TestSnapshotService_List_MetaCacheEffect(t *testing.T) {
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()
	snapDir := t.TempDir()

	// 写入两个快照文件
	writeSvcTestSnapshot(t, snapDir, "snap-cache-a",
		[]svcYamlNodeItem{{Labels: []string{"Device"}, URI: "device:001"}}, nil)
	writeSvcTestSnapshot(t, snapDir, "snap-cache-b",
		[]svcYamlNodeItem{{Labels: []string{"Device"}, URI: "device:002"}}, nil)

	mgr := snapshot.NewSnapshotManager(gdb, lock, snapDir, 5)
	svc := NewSnapshotService(mgr)

	// 第一次 List() — 触发 warmCache
	first, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first List() returned %d snapshots, want 2", len(first))
	}

	// 第二次 List() — 应命中 MetaCache
	second, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("second List() error = %v", err)
	}
	if len(second) != 2 {
		t.Errorf("second List() returned %d snapshots, want 2", len(second))
	}

	// 验证两次结果快照名称集合一致
	nameSet := func(metas []snapshot.SnapshotMeta) map[string]bool {
		s := make(map[string]bool, len(metas))
		for _, m := range metas {
			s[m.Name] = true
		}
		return s
	}
	firstNames := nameSet(first)
	secondNames := nameSet(second)
	for name := range firstNames {
		if !secondNames[name] {
			t.Errorf("snapshot %q in first result but missing in second", name)
		}
	}
}

// ---------------------------------------------------------------------------
// TC-SS09: AuditQuery 过滤查询 — 通过 Service 层查询审计日志
// ---------------------------------------------------------------------------

func TestSnapshotService_AuditQuery(t *testing.T) {
	gdb := &mockGraphDB{
		queryResults: []map[string]any{
			{"labels": []any{"Device"}, "uri": "device:AQ001", "props": map[string]any{"hostname": "r1"}},
		},
	}
	lock := snapshot.NewGraphLock()
	snapDir := t.TempDir()
	mgr := snapshot.NewSnapshotManager(gdb, lock, snapDir, 5)
	svc := NewSnapshotService(mgr)

	// 触发审计记录
	if _, err := svc.Create(context.Background(), "snap-audit-q"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// 按 action=create 查询，应返回非空
	entries := svc.AuditQuery(snapshot.AuditFilter{Action: "create"})
	if len(entries) == 0 {
		t.Fatal("AuditQuery(create) returned 0 entries, want >= 1")
	}
	if entries[0].Action != "create" {
		t.Errorf("entry.Action = %q, want %q", entries[0].Action, "create")
	}

	// 按不存在的 action 查询，应返回空
	entries = svc.AuditQuery(snapshot.AuditFilter{Action: "nonexistent"})
	if len(entries) != 0 {
		t.Errorf("AuditQuery(nonexistent) returned %d entries, want 0", len(entries))
	}

	// 按快照名过滤
	entries = svc.AuditQuery(snapshot.AuditFilter{Snapshot: "snap-audit-q"})
	if len(entries) == 0 {
		t.Fatal("AuditQuery(snapshot=snap-audit-q) returned 0 entries")
	}
	for _, e := range entries {
		if e.Snapshot != "snap-audit-q" {
			t.Errorf("entry.Snapshot = %q, want %q", e.Snapshot, "snap-audit-q")
		}
	}
}

// ---------------------------------------------------------------------------
// TC-SS10: AuditRecent 最近 N 条审计记录
// ---------------------------------------------------------------------------

func TestSnapshotService_AuditRecent(t *testing.T) {
	gdb := &mockGraphDB{
		queryResults: []map[string]any{
			{"labels": []any{"Device"}, "uri": "device:AR001", "props": map[string]any{"hostname": "r1"}},
		},
	}
	lock := snapshot.NewGraphLock()
	snapDir := t.TempDir()
	mgr := snapshot.NewSnapshotManager(gdb, lock, snapDir, 5)
	svc := NewSnapshotService(mgr)

	// 触发多条审计记录
	for i := 0; i < 3; i++ {
		if _, err := svc.Create(context.Background(), fmt.Sprintf("snap-recent-%d", i)); err != nil {
			t.Fatalf("Create(%d) error = %v", i, err)
		}
	}

	// AuditRecent(1) — 只返回 1 条
	recent := svc.AuditRecent(1)
	if len(recent) != 1 {
		t.Errorf("AuditRecent(1) returned %d entries, want 1", len(recent))
	}

	// AuditRecent(100) — 返回实际条目数，不超过 100
	recent = svc.AuditRecent(100)
	if len(recent) != 3 {
		t.Errorf("AuditRecent(100) returned %d entries, want 3", len(recent))
	}

	// AuditRecent(0) — 返回空
	recent = svc.AuditRecent(0)
	if len(recent) != 0 {
		t.Errorf("AuditRecent(0) returned %d entries, want 0", len(recent))
	}
}

// ---------------------------------------------------------------------------
// TC-SS11: List 空目录 MetaCache 预热 — cacheReady 标记正确
// ---------------------------------------------------------------------------

func TestSnapshotService_List_EmptyDir_CacheReady(t *testing.T) {
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()
	snapDir := t.TempDir()
	mgr := snapshot.NewSnapshotManager(gdb, lock, snapDir, 5)
	svc := NewSnapshotService(mgr)

	// 第一次 List() — 空目录，触发 warmCache
	first, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if len(first) != 0 {
		t.Fatalf("first List() returned %d snapshots, want 0", len(first))
	}

	// 第二次 List() — cacheReady=true，仍返回空
	second, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("second List() error = %v", err)
	}
	if len(second) != 0 {
		t.Errorf("second List() returned %d snapshots, want 0", len(second))
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
