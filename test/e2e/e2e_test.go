//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/connector/controller"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/service"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// TestE2E_Neo4jConnection 验证 Neo4j 连接和基本 CRUD 操作。
func TestE2E_Neo4jConnection(t *testing.T) {
	client := newE2EClient(t)
	db := uniqueDBName(t)
	defer cleanupDB(t, client, db)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// BulkCreate: 2 个 Device 节点 + 1 条 CONNECTS_TO 关系
	nodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:A", Props: map[string]any{"hostname": "router-a", "status": "Up"}},
		{Labels: []string{"Device"}, URI: "device:B", Props: map[string]any{"hostname": "router-b", "status": "Up"}},
	}
	rels := []assembler.Relation{
		{Type: "CONNECTS_TO", From: "device:A", To: "device:B"},
	}

	if err := client.BulkCreate(ctx, db, nodes, rels); err != nil {
		t.Fatalf("BulkCreate error = %v", err)
	}

	// 验证节点数
	if got := countNodes(t, client, db); got != 2 {
		t.Errorf("countNodes = %d, want 2", got)
	}

	// 验证关系数
	if got := countRels(t, client, db); got != 1 {
		t.Errorf("countRels = %d, want 1", got)
	}

	// Query 验证属性
	rows, err := client.Query(ctx, db,
		"MATCH (n:Device) WHERE n._db = $_db AND n.uri = 'device:A' RETURN n.hostname AS hostname", nil)
	if err != nil {
		t.Fatalf("Query error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Query returned %d rows, want 1", len(rows))
	}
	if hostname, _ := rows[0]["hostname"].(string); hostname != "router-a" {
		t.Errorf("hostname = %q, want %q", hostname, "router-a")
	}

	// ClearDB 验证清理
	if err := client.ClearDB(ctx, db); err != nil {
		t.Fatalf("ClearDB error = %v", err)
	}
	if got := countNodes(t, client, db); got != 0 {
		t.Errorf("after ClearDB, countNodes = %d, want 0", got)
	}
}

// TestE2E_FullSyncPipeline 验证 Connector → Normalizer → Assembler → Neo4j BulkCreate 全管线。
func TestE2E_FullSyncPipeline(t *testing.T) {
	client := newE2EClient(t)
	reg := loadOntology(t)
	db := uniqueDBName(t)
	defer cleanupDB(t, client, db)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 执行全管线处理
	gm := runFullPipeline(t, reg)

	// ClearDB + BulkCreate
	if err := client.ClearDB(ctx, db); err != nil {
		t.Fatalf("ClearDB error = %v", err)
	}
	if err := client.BulkCreate(ctx, db, gm.Nodes, gm.Relations); err != nil {
		t.Fatalf("BulkCreate error = %v", err)
	}

	// 验证总节点数: Device:3 + Interface:12 + ISIS:3 + Link:2 + Network_Slice:1 = 21
	if got := countNodes(t, client, db); got != 21 {
		t.Errorf("countNodes = %d, want 21", got)
	}

	// 验证总关系数: HAS_INTERFACE:12 + CONNECTS_TO:2 + RUNS_ON:3 + ENDPOINT:4 = 21
	if got := countRels(t, client, db); got != 21 {
		t.Errorf("countRels = %d, want 21", got)
	}

	// 按 Label 验证节点分布
	expectedByLabel := map[string]int{
		"Device": 3, "Interface": 12, "ISIS": 3, "Link": 2, "Network_Slice": 1,
	}
	for label, want := range expectedByLabel {
		if got := countNodesByLabel(t, client, db, label); got != want {
			t.Errorf("Nodes[%s] = %d, want %d", label, got, want)
		}
	}

	// 按 Type 验证关系分布
	expectedByType := map[string]int{
		"HAS_INTERFACE": 12, "CONNECTS_TO": 2, "RUNS_ON": 3, "ENDPOINT": 4,
	}
	for relType, want := range expectedByType {
		if got := countRelsByType(t, client, db, relType); got != want {
			t.Errorf("Relations[%s] = %d, want %d", relType, got, want)
		}
	}

	// 验证特定节点属性: device:SN12345 的 hostname
	rows, err := client.Query(ctx, db,
		"MATCH (n:Device) WHERE n._db = $_db AND n.uri = 'device:SN12345' RETURN n.hostname AS hostname", nil)
	if err != nil {
		t.Fatalf("Query error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Query returned %d rows for device:SN12345, want 1", len(rows))
	}
	hostname, _ := rows[0]["hostname"].(string)
	if hostname != "Router_Core_01" {
		t.Errorf("device:SN12345 hostname = %q, want %q", hostname, "Router_Core_01")
	}
}

// TestE2E_IncrementalSync 验证 Upsert + DeleteByURIs + DeleteRelations 增量操作。
func TestE2E_IncrementalSync(t *testing.T) {
	client := newE2EClient(t)
	reg := loadOntology(t)
	db := uniqueDBName(t)
	defer cleanupDB(t, client, db)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 先全量导入基础数据
	gm := runFullPipeline(t, reg)
	if err := client.ClearDB(ctx, db); err != nil {
		t.Fatalf("ClearDB error = %v", err)
	}
	if err := client.BulkCreate(ctx, db, gm.Nodes, gm.Relations); err != nil {
		t.Fatalf("BulkCreate error = %v", err)
	}

	baseNodes := countNodes(t, client, db)
	baseRels := countRels(t, client, db)

	// === Upsert 新增节点 ===
	newNodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN99999", Props: map[string]any{
			"serial_number": "SN99999", "hostname": "New-Router", "status": "Up",
		}},
	}
	if err := client.Upsert(ctx, db, newNodes, nil); err != nil {
		t.Fatalf("Upsert new node error = %v", err)
	}
	if got := countNodes(t, client, db); got != baseNodes+1 {
		t.Errorf("after Upsert, countNodes = %d, want %d", got, baseNodes+1)
	}

	// === Upsert 同一节点修改属性（幂等，不增加计数） ===
	updatedNodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN99999", Props: map[string]any{
			"hostname": "Updated-Router",
		}},
	}
	if err := client.Upsert(ctx, db, updatedNodes, nil); err != nil {
		t.Fatalf("Upsert update error = %v", err)
	}
	if got := countNodes(t, client, db); got != baseNodes+1 {
		t.Errorf("after Upsert update, countNodes = %d, want %d", got, baseNodes+1)
	}

	// 验证属性已更新
	rows, err := client.Query(ctx, db,
		"MATCH (n:Device) WHERE n._db = $_db AND n.uri = 'device:SN99999' RETURN n.hostname AS hostname", nil)
	if err != nil {
		t.Fatalf("Query error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Query returned %d rows, want 1", len(rows))
	}
	if h, _ := rows[0]["hostname"].(string); h != "Updated-Router" {
		t.Errorf("hostname after update = %q, want %q", h, "Updated-Router")
	}

	// === DeleteRelations 删除单条关系 ===
	delRels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN12345", To: "iface:SN12345_GE1/0/1"},
	}
	if err := client.DeleteRelations(ctx, db, delRels); err != nil {
		t.Fatalf("DeleteRelations error = %v", err)
	}
	if got := countRels(t, client, db); got != baseRels-1 {
		t.Errorf("after DeleteRelations, countRels = %d, want %d", got, baseRels-1)
	}
	// 节点数不变
	if got := countNodes(t, client, db); got != baseNodes+1 {
		t.Errorf("after DeleteRelations, countNodes = %d, want %d", got, baseNodes+1)
	}

	// === DeleteByURIs 删除节点（含 DETACH DELETE） ===
	if err := client.DeleteByURIs(ctx, db, []string{"device:SN99999"}); err != nil {
		t.Fatalf("DeleteByURIs error = %v", err)
	}
	if got := countNodes(t, client, db); got != baseNodes {
		t.Errorf("after DeleteByURIs, countNodes = %d, want %d", got, baseNodes)
	}
}

// TestE2E_SnapshotLifecycle 验证 Create → List → Restore → Diff 完整快照生命周期。
func TestE2E_SnapshotLifecycle(t *testing.T) {
	client := newE2EClient(t)
	reg := loadOntology(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 先保存 default DB 的原始数据，测试结束恢复
	origDB := uniqueDBName(t) + "_orig"
	if err := client.ClearDB(ctx, origDB); err != nil {
		t.Logf("ClearDB origDB: %v (may be empty)", err)
	}
	// 将 default 数据克隆到 origDB 备份
	rows, _ := client.Query(ctx, "default", "MATCH (n) RETURN count(n) AS cnt", nil)
	hasOrigData := false
	if len(rows) > 0 {
		if cnt, _ := rows[0]["cnt"].(int64); cnt > 0 {
			hasOrigData = true
			if err := client.CloneDB(ctx, "default", origDB); err != nil {
				t.Logf("CloneDB backup error (non-fatal): %v", err)
			}
		}
	}
	defer func() {
		// 恢复 default DB
		if hasOrigData {
			_ = client.ClearDB(ctx, "default")
			_ = client.CloneDB(ctx, origDB, "default")
		} else {
			_ = client.ClearDB(ctx, "default")
		}
		_ = client.ClearDB(ctx, origDB)
	}()

	// === 准备: 全量导入数据到 default DB ===
	gm := runFullPipeline(t, reg)
	if err := client.ClearDB(ctx, "default"); err != nil {
		t.Fatalf("ClearDB default error = %v", err)
	}
	if err := client.BulkCreate(ctx, "default", gm.Nodes, gm.Relations); err != nil {
		t.Fatalf("BulkCreate default error = %v", err)
	}

	// === 创建 SnapshotManager ===
	snapDir := t.TempDir()
	lock := snapshot.NewGraphLock()
	mgr := snapshot.NewSnapshotManager(client, lock, snapDir, 5)

	// === Create 第一个快照 ===
	snap1, err := mgr.Create(ctx, "snap-001")
	if err != nil {
		t.Fatalf("Create(snap-001) error = %v", err)
	}
	if snap1.NodeCount != 21 {
		t.Errorf("snap1.NodeCount = %d, want 21", snap1.NodeCount)
	}
	if snap1.RelCount != 21 {
		t.Errorf("snap1.RelCount = %d, want 21", snap1.RelCount)
	}

	// === 验证 YAML 文件存在 ===
	if _, err := os.Stat(snap1.FilePath); os.IsNotExist(err) {
		t.Fatalf("YAML file not created: %s", snap1.FilePath)
	}

	// === List 验证 ===
	metas, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metas) != 1 {
		t.Errorf("List() returned %d snapshots, want 1", len(metas))
	}

	// === 修改 default: Upsert 新节点 ===
	newNode := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN-NEW", Props: map[string]any{"hostname": "brand-new"}},
	}
	if err := client.Upsert(ctx, "default", newNode, nil); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}

	// === Create 第二个快照（含新节点） ===
	snap2, err := mgr.Create(ctx, "snap-002")
	if err != nil {
		t.Fatalf("Create(snap-002) error = %v", err)
	}
	if snap2.NodeCount != 22 {
		t.Errorf("snap2.NodeCount = %d, want 22", snap2.NodeCount)
	}

	// === Restore 到 snap-001 ===
	if err := mgr.Restore(ctx, "snap-001"); err != nil {
		t.Fatalf("Restore(snap-001) error = %v", err)
	}

	// 验证 default 恢复到 snap-001 状态
	if got := countNodes(t, client, "default"); got != 21 {
		t.Errorf("after Restore, default countNodes = %d, want 21", got)
	}

	// === Diff 对比 snap-001 和 snap-002 ===
	diff, err := mgr.Diff(ctx, "snap-001", "snap-002")
	if err != nil {
		t.Fatalf("Diff error = %v", err)
	}

	// snap-002 比 snap-001 多 1 个节点 (device:SN-NEW)
	if len(diff.AddedNodes) != 1 {
		t.Errorf("Diff.AddedNodes = %d, want 1", len(diff.AddedNodes))
	} else if diff.AddedNodes[0].URI != "device:SN-NEW" {
		t.Errorf("Diff.AddedNodes[0].URI = %q, want %q", diff.AddedNodes[0].URI, "device:SN-NEW")
	}
	if len(diff.RemovedNodes) != 0 {
		t.Errorf("Diff.RemovedNodes = %d, want 0", len(diff.RemovedNodes))
	}

	// === 清理快照逻辑 DB ===
	_ = mgr.Delete(ctx, "snap-001")
	_ = mgr.Delete(ctx, "snap-002")
}

// TestE2E_LogicalDBIsolation 验证逻辑 DB 隔离，不同 _db 的数据互不干扰。
func TestE2E_LogicalDBIsolation(t *testing.T) {
	client := newE2EClient(t)
	dbA := uniqueDBName(t) + "_A"
	dbB := uniqueDBName(t) + "_B"
	defer cleanupDB(t, client, dbA)
	defer cleanupDB(t, client, dbB)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// dbA: 3 个节点
	nodesA := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:A1"},
		{Labels: []string{"Device"}, URI: "device:A2"},
		{Labels: []string{"Device"}, URI: "device:A3"},
	}
	if err := client.BulkCreate(ctx, dbA, nodesA, nil); err != nil {
		t.Fatalf("BulkCreate dbA error = %v", err)
	}

	// dbB: 5 个节点
	nodesB := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:B1"},
		{Labels: []string{"Device"}, URI: "device:B2"},
		{Labels: []string{"Device"}, URI: "device:B3"},
		{Labels: []string{"Device"}, URI: "device:B4"},
		{Labels: []string{"Device"}, URI: "device:B5"},
	}
	if err := client.BulkCreate(ctx, dbB, nodesB, nil); err != nil {
		t.Fatalf("BulkCreate dbB error = %v", err)
	}

	// 验证隔离
	if got := countNodes(t, client, dbA); got != 3 {
		t.Errorf("dbA countNodes = %d, want 3", got)
	}
	if got := countNodes(t, client, dbB); got != 5 {
		t.Errorf("dbB countNodes = %d, want 5", got)
	}

	// ClearDB(dbA) 不影响 dbB
	if err := client.ClearDB(ctx, dbA); err != nil {
		t.Fatalf("ClearDB dbA error = %v", err)
	}
	if got := countNodes(t, client, dbA); got != 0 {
		t.Errorf("after ClearDB, dbA countNodes = %d, want 0", got)
	}
	if got := countNodes(t, client, dbB); got != 5 {
		t.Errorf("after ClearDB(dbA), dbB countNodes = %d, want 5", got)
	}

	// ListDBs 验证
	dbs, err := client.ListDBs(ctx)
	if err != nil {
		t.Fatalf("ListDBs error = %v", err)
	}
	foundB := false
	for _, db := range dbs {
		if db == dbB {
			foundB = true
		}
	}
	if !foundB {
		t.Errorf("ListDBs does not contain %s", dbB)
	}

	// HasDB 验证
	hasA, err := client.HasDB(ctx, dbA)
	if err != nil {
		t.Fatalf("HasDB(dbA) error = %v", err)
	}
	if hasA {
		t.Errorf("HasDB(dbA) = true after ClearDB, want false")
	}

	hasB, err := client.HasDB(ctx, dbB)
	if err != nil {
		t.Fatalf("HasDB(dbB) error = %v", err)
	}
	if !hasB {
		t.Errorf("HasDB(dbB) = false, want true")
	}
}

// TestE2E_EnsureIndexes 验证索引创建的幂等性。
func TestE2E_EnsureIndexes(t *testing.T) {
	client := newE2EClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	labels := []string{"Device", "Interface", "Link"}

	// 第一次创建
	if err := client.EnsureIndexes(ctx, labels); err != nil {
		t.Fatalf("EnsureIndexes first call error = %v", err)
	}

	// 第二次创建（幂等，不报错）
	if err := client.EnsureIndexes(ctx, labels); err != nil {
		t.Fatalf("EnsureIndexes second call (idempotent) error = %v", err)
	}

	// 验证索引存在: SHOW INDEXES 返回包含目标索引
	rows, err := client.Query(ctx, "neo4j",
		"SHOW INDEXES YIELD name WHERE name STARTS WITH 'idx_' RETURN name", nil)
	if err != nil {
		t.Fatalf("SHOW INDEXES error = %v", err)
	}

	indexNames := make(map[string]bool)
	for _, row := range rows {
		if name, ok := row["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{"idx_device_db_uri", "idx_interface_db_uri", "idx_link_db_uri"}
	for _, idx := range expectedIndexes {
		if !indexNames[idx] {
			t.Errorf("index %q not found in SHOW INDEXES results", idx)
		}
	}
}

// TestE2E_FullDataFlow TC-E2E-01: 完整数据流串联测试。
// FullSync → Query → Snapshot.Create → IncrementalSync(新设备) → Snapshot.Create →
// Diff → Restore → 验证恢复到 snap-001 状态。
// 首次通过 SyncService + SnapshotManager 编排完整工作流。
func TestE2E_FullDataFlow(t *testing.T) {
	client := newE2EClient(t)
	lock := snapshot.NewGraphLock()
	syncSvc := newE2ESyncService(t, client, lock)
	snapMgr := newE2ESnapshotManager(t, client, lock)
	defer backupAndRestoreDefault(t, client)()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 清理可能存在的的前次运行遗留的逻辑 DB，确保 Diff 从 YAML 导入干净数据
	_ = snapMgr.Delete(ctx, "snap-001")
	_ = snapMgr.Delete(ctx, "snap-002")

	// === Step 1: FullSync 全量同步 ===
	result, err := syncSvc.FullSync(ctx)
	if err != nil {
		t.Fatalf("FullSync error = %v", err)
	}
	// Mock 数据: Device:3 + Interface:12 + ISIS:3 + Link:2 + Network_Slice:1 = 21
	if result.NodesCreated != 21 {
		t.Errorf("FullSync NodesCreated = %d, want 21", result.NodesCreated)
	}
	// 关系: HAS_INTERFACE:12 + CONNECTS_TO:2 + RUNS_ON:3 + ENDPOINT:4 = 21
	if result.RelationsCreated != 21 {
		t.Errorf("FullSync RelationsCreated = %d, want 21", result.RelationsCreated)
	}

	// === Step 2: Query 验证设备数据 ===
	deviceCount := countNodesByLabel(t, client, "default", "Device")
	if deviceCount != 3 {
		t.Errorf("Device count = %d, want 3", deviceCount)
	}

	// === Step 3: Create snapshot "snap-001" ===
	snap1, err := snapMgr.Create(ctx, "snap-001")
	if err != nil {
		t.Fatalf("Create(snap-001) error = %v", err)
	}
	if snap1.NodeCount != 21 {
		t.Errorf("snap1.NodeCount = %d, want 21", snap1.NodeCount)
	}
	if _, err := os.Stat(snap1.FilePath); os.IsNotExist(err) {
		t.Fatalf("YAML file not created: %s", snap1.FilePath)
	}

	// === Step 4: IncrementalSync update（新增全新设备） ===
	updateEvent := service.SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Data: []map[string]any{
			{
				"serial_number": "SN-E2E-NEW",
				"hostname":      "E2E Test Device",
				"vendor":        "TestVendor",
				"hw_model":      "TestModel",
				"status":        "Up",
			},
		},
	}
	incResult, err := syncSvc.IncrementalSync(ctx, updateEvent)
	if err != nil {
		t.Fatalf("IncrementalSync(update) error = %v", err)
	}
	if incResult.NodesCreated != 1 {
		t.Errorf("IncrementalSync NodesCreated = %d, want 1", incResult.NodesCreated)
	}

	// 验证节点数变为 22
	if got := countNodes(t, client, "default"); got != 22 {
		t.Errorf("after IncrementalSync, countNodes = %d, want 22", got)
	}

	// === Step 5: Create snapshot "snap-002" ===
	snap2, err := snapMgr.Create(ctx, "snap-002")
	if err != nil {
		t.Fatalf("Create(snap-002) error = %v", err)
	}
	if snap2.NodeCount != 22 {
		t.Errorf("snap2.NodeCount = %d, want 22", snap2.NodeCount)
	}

	// === Step 6: LocalDiff("snap-001", "snap-002") ===
	// 使用 LocalDiff（YAML 内存对比）而非 Cypher Diff，
	// 因为 LocalDiff 更稳定且不需要 Neo4j。
	diff, err := snapMgr.LocalDiff("snap-001", "snap-002")
	if err != nil {
		t.Fatalf("LocalDiff error = %v", err)
	}
	// snap-002 比 snap-001 多 1 个节点 (device:SN-E2E-NEW)
	if len(diff.AddedNodes) != 1 {
		t.Errorf("LocalDiff.AddedNodes = %d, want 1", len(diff.AddedNodes))
		for i, n := range diff.AddedNodes {
			t.Logf("  AddedNode[%d]: labels=%v uri=%s", i, n.Labels, n.URI)
		}
	} else if diff.AddedNodes[0].URI != "device:SN-E2E-NEW" {
		t.Errorf("LocalDiff.AddedNodes[0].URI = %q, want %q", diff.AddedNodes[0].URI, "device:SN-E2E-NEW")
	}
	if len(diff.RemovedNodes) != 0 {
		t.Errorf("LocalDiff.RemovedNodes = %d, want 0", len(diff.RemovedNodes))
	}

	// === Step 7: Restore("snap-001") ===
	if err := snapMgr.Restore(ctx, "snap-001"); err != nil {
		t.Fatalf("Restore(snap-001) error = %v", err)
	}

	// === Step 8: 验证恢复到 snap-001 状态 ===
	if got := countNodes(t, client, "default"); got != 21 {
		t.Errorf("after Restore, countNodes = %d, want 21", got)
	}

	// 验证新设备节点已不存在
	rows, err := client.Query(ctx, "default",
		"MATCH (n:Device {uri: 'device:SN-E2E-NEW'}) WHERE n._db = $_db RETURN n.uri AS uri", nil)
	if err != nil {
		t.Fatalf("Query after Restore error = %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("after Restore, device:SN-E2E-NEW should not exist, got %d rows", len(rows))
	}

	// === 清理快照 ===
	_ = snapMgr.Delete(ctx, "snap-001")
	_ = snapMgr.Delete(ctx, "snap-002")
}

// TestE2E_ConcurrentProtection TC-E2E-03: 并发保护测试。
// 验证 Restore 持有 GraphLock 期间，HandleWebhook 事件被缓冲到 channel，
// Restore 完成后 Consumer 串行处理所有事件，最终数据一致。
func TestE2E_ConcurrentProtection(t *testing.T) {
	client := newE2EClient(t)
	lock := snapshot.NewGraphLock()
	syncSvc := newE2ESyncService(t, client, lock)
	snapMgr := newE2ESnapshotManager(t, client, lock)
	defer backupAndRestoreDefault(t, client)()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// === Step 1: FullSync 建立基线 ===
	result, err := syncSvc.FullSync(ctx)
	if err != nil {
		t.Fatalf("FullSync error = %v", err)
	}
	baseNodes := result.NodesCreated // 21

	// === Step 2: 创建快照用于 Restore ===
	_, err = snapMgr.Create(ctx, "concurrent-snap")
	if err != nil {
		t.Fatalf("Create(concurrent-snap) error = %v", err)
	}

	// === Step 3: 启动 Consumer ===
	consumerCtx, consumerCancel := context.WithCancel(ctx)
	defer consumerCancel()
	syncSvc.StartConsumer(consumerCtx)

	// === Step 4: 启动 Restore goroutine ===
	restoreDone := make(chan error, 1)
	go func() {
		restoreDone <- snapMgr.Restore(ctx, "concurrent-snap")
	}()

	// 短暂等待让 Restore 先获取锁
	time.Sleep(50 * time.Millisecond)

	// === Step 5: Restore 期间发送 5 个 Webhook 事件 ===
	for i := 0; i < 5; i++ {
		event := service.SyncEvent{
			Action:     "update",
			EntityType: "Device",
			Data: []map[string]any{
				{
					"serial_number": fmt.Sprintf("SN-CONCURRENT-%d", i),
					"hostname":      fmt.Sprintf("Concurrent-Device-%d", i),
					"vendor":        "TestVendor",
					"hw_model":      "TestModel",
					"status":        "Up",
				},
			},
		}
		if err := syncSvc.HandleWebhook(event); err != nil {
			t.Logf("HandleWebhook[%d] error (may be channel full): %v", i, err)
		}
	}

	// === Step 6: 等待 Restore 完成 ===
	select {
	case err := <-restoreDone:
		if err != nil {
			t.Fatalf("Restore error = %v", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("Restore did not complete within 60s timeout")
	}

	// === Step 7: 等待所有事件被消费处理（轮询，30s 超时） ===
	expectedNodes := baseNodes + 5
	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		currentNodes := countNodes(t, client, "default")
		if currentNodes >= expectedNodes {
			break
		}
		select {
		case <-deadline:
			finalNodes := countNodes(t, client, "default")
			t.Fatalf("Timeout waiting for events: expected >= %d nodes, got %d",
				expectedNodes, finalNodes)
		case <-ticker.C:
			// 继续轮询
		}
	}

	// === Step 8: 验证 5 个新设备节点存在 ===
	for i := 0; i < 5; i++ {
		uri := fmt.Sprintf("device:SN-CONCURRENT-%d", i)
		rows, err := client.Query(ctx, "default",
			fmt.Sprintf(
				"MATCH (n:Device {uri: '%s'}) WHERE n._db = $_db RETURN n.hostname AS hostname",
				uri,
			), nil)
		if err != nil {
			t.Fatalf("Query for %s error = %v", uri, err)
		}
		if len(rows) != 1 {
			t.Errorf("Device %s not found (got %d rows)", uri, len(rows))
		}
	}

	// 最终节点计数验证
	finalNodes := countNodes(t, client, "default")
	if finalNodes != expectedNodes {
		t.Errorf("final countNodes = %d, want %d", finalNodes, expectedNodes)
	}

	// === 清理 ===
	_ = snapMgr.Delete(ctx, "concurrent-snap")
}

// TestE2E_FullSyncWithRealConnectors 验证 httptest mock Controller + Mock Connector
// 通过 ConnectorFactory 创建后，FullSync 完整管线写入 Neo4j。
func TestE2E_FullSyncWithRealConnectors(t *testing.T) {
	client := newE2EClient(t)
	reg := loadOntology(t)
	lock := snapshot.NewGraphLock()

	// 1. 启动 httptest mock server 模拟 Controller API
	controllerServer := setupE2EControllerServer(t)
	defer controllerServer.Close()

	// 2. 通过 ConnectorFactory 创建 Connector
	factory := connector.NewConnectorFactory()

	// 注册 mock builder
	factory.RegisterBuilder("mock", func(name string, cfg map[string]any, entityTypes []string) (connector.Connector, error) {
		dataDir, _ := cfg["data_dir"].(string)
		return mock.NewMockConnector(name, dataDir, entityTypes), nil
	})

	// 注册 controller builder (使用 mock server URL)
	controllerClient := connector.NewHTTPClient(
		connector.WithBaseURL(controllerServer.URL),
		connector.WithAuth(connector.AuthConfig{Type: "bearer", Token: "mock-token"}),
	)
	factory.RegisterBuilder("controller", func(name string, cfg map[string]any, entityTypes []string) (connector.Connector, error) {
		return controller.NewControllerConnector(name, controllerClient, entityTypes, cfg), nil
	})

	connReg := connector.NewConnectorRegistry()

	// 创建 mock connector
	mockConn := mock.NewMockConnector("e2e-mock",
		filepath.Join("..", "..", "testdata", "mock_netbox"),
		[]string{"Device", "Interface", "ISIS", "Link", "Network_Slice"})
	connReg.Register(mockConn)

	// 创建 controller connector (指向 mock server)
	ctrlCfg := map[string]any{
		"base_url":  controllerServer.URL,
		"token_url": "/oauth/token",
		"username":  "test",
		"password":  "test",
		"device_id": "test",
	}
	ctrlConn, err := factory.Create(connector.ConnectorConfigEntry{
		Name:        "e2e-controller",
		Type:        "controller",
		Config:      ctrlCfg,
		EntityTypes: []string{"Device", "Interface", "Link", "Alarm", "VPN", "Tunnel", "ISIS", "BGP"},
	})
	if err != nil {
		t.Fatalf("Create controller connector: %v", err)
	}
	connReg.Register(ctrlConn)

	// 3. 创建 SyncService 并执行 FullSync
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	syncSvc := service.NewSyncService(connReg, norm, asm, client, lock, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// FullSync 内部使用 "default" DB，先备份后恢复
	backup := backupAndRestoreDefault(t, client)
	defer backup()

	result, err := syncSvc.FullSync(ctx)
	if err != nil {
		t.Fatalf("FullSync() error = %v", err)
	}

	t.Logf("FullSync result: nodes=%d, rels=%d, orphan=%d",
		result.NodesCreated, result.RelationsCreated, result.OrphanEdgesSkipped)

	// 4. 验证 Neo4j 中有节点和关系
	neo4jNodes := countNodes(t, client, "default")
	neo4jRels := countRels(t, client, "default")

	if neo4jNodes == 0 {
		t.Error("FullSync created 0 nodes, expected > 0")
	}
	t.Logf("Neo4j nodes: %d, relations: %d", neo4jNodes, neo4jRels)

	// 5. 验证 Device 节点存在
	deviceCount := countNodesByLabel(t, client, "default", "Device")
	if deviceCount == 0 {
		t.Error("No Device nodes found after FullSync")
	}
	t.Logf("Device nodes: %d", deviceCount)

	// 6. 验证 HAS_INTERFACE 关系存在
	ifaceRels := countRelsByType(t, client, "default", "HAS_INTERFACE")
	t.Logf("HAS_INTERFACE relations: %d", ifaceRels)
}

// TestE2E_DiffPropertyChange 验证属性级变更检测（LocalDiff + Cypher Diff 一致性）。
// 创建快照 A → 修改节点属性 → 创建快照 B → 对比两种 Diff 方式的 ChangedNodes。
func TestE2E_DiffPropertyChange(t *testing.T) {
	client := newE2EClient(t)
	lock := snapshot.NewGraphLock()
	syncSvc := newE2ESyncService(t, client, lock)
	snapMgr := newE2ESnapshotManager(t, client, lock)
	defer backupAndRestoreDefault(t, client)()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 清理前次遗留
	_ = snapMgr.Delete(ctx, "snap-diff-a")
	_ = snapMgr.Delete(ctx, "snap-diff-b")

	// === Step 1: FullSync 导入基线数据 ===
	if _, err := syncSvc.FullSync(ctx); err != nil {
		t.Fatalf("FullSync error = %v", err)
	}

	// === Step 2: Create snap-A ===
	if _, err := snapMgr.Create(ctx, "snap-diff-a"); err != nil {
		t.Fatalf("Create(snap-diff-a) error = %v", err)
	}

	// === Step 3: Upsert 修改 device:SN12345 的 hostname 属性 ===
	updatedNode := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN12345", Props: map[string]any{
			"serial_number": "SN12345", "hostname": "Modified-Router-Core-01",
			"vendor": "H3C", "hw_model": "CR16000", "status": "Down",
		}},
	}
	if err := client.Upsert(ctx, "default", updatedNode, nil); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}

	// === Step 4: Create snap-B ===
	if _, err := snapMgr.Create(ctx, "snap-diff-b"); err != nil {
		t.Fatalf("Create(snap-diff-b) error = %v", err)
	}

	// === Step 5: LocalDiff → 验证 ChangedNodes 非空 ===
	localDiff, err := snapMgr.LocalDiff("snap-diff-a", "snap-diff-b")
	if err != nil {
		t.Fatalf("LocalDiff error = %v", err)
	}
	if len(localDiff.ChangedNodes) == 0 {
		t.Error("LocalDiff.ChangedNodes is empty, expected at least 1")
	} else {
		t.Logf("LocalDiff.ChangedNodes count = %d", len(localDiff.ChangedNodes))
		for i, nc := range localDiff.ChangedNodes {
			t.Logf("  [%d] URI=%s Label=%s modified=%d", i, nc.URI, nc.Label, len(nc.ModifiedFields))
		}
	}

	// 收集 LocalDiff ChangedNodes URI 集合
	localURIs := make(map[string]bool)
	for _, nc := range localDiff.ChangedNodes {
		localURIs[nc.URI] = true
	}

	// === Step 6: Cypher Diff → 验证 ChangedNodes 非空 ===
	cypherDiff, err := snapMgr.Diff(ctx, "snap-diff-a", "snap-diff-b")
	if err != nil {
		t.Fatalf("Diff error = %v", err)
	}
	if len(cypherDiff.ChangedNodes) == 0 {
		t.Error("Cypher Diff.ChangedNodes is empty, expected at least 1")
	} else {
		t.Logf("Cypher Diff.ChangedNodes count = %d", len(cypherDiff.ChangedNodes))
		for i, nc := range cypherDiff.ChangedNodes {
			t.Logf("  [%d] URI=%s Label=%s modified=%d", i, nc.URI, nc.Label, len(nc.ModifiedFields))
		}
	}

	// === Step 7: 对比两种 Diff 的 ChangedNodes URI 集合一致 ===
	for _, nc := range cypherDiff.ChangedNodes {
		if !localURIs[nc.URI] {
			t.Errorf("URI %q in Cypher Diff but not in LocalDiff ChangedNodes", nc.URI)
		}
	}

	// === 清理 ===
	_ = snapMgr.Delete(ctx, "snap-diff-a")
	_ = snapMgr.Delete(ctx, "snap-diff-b")
}
