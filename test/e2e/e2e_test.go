//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
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
		{Label: "Device", URI: "device:A", Props: map[string]any{"hostname": "router-a", "status": "Up"}},
		{Label: "Device", URI: "device:B", Props: map[string]any{"hostname": "router-b", "status": "Up"}},
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
		{Label: "Device", URI: "device:SN99999", Props: map[string]any{
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
		{Label: "Device", URI: "device:SN99999", Props: map[string]any{
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
		{Label: "Device", URI: "device:SN-NEW", Props: map[string]any{"hostname": "brand-new"}},
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
		{Label: "Device", URI: "device:A1"},
		{Label: "Device", URI: "device:A2"},
		{Label: "Device", URI: "device:A3"},
	}
	if err := client.BulkCreate(ctx, dbA, nodesA, nil); err != nil {
		t.Fatalf("BulkCreate dbA error = %v", err)
	}

	// dbB: 5 个节点
	nodesB := []assembler.Node{
		{Label: "Device", URI: "device:B1"},
		{Label: "Device", URI: "device:B2"},
		{Label: "Device", URI: "device:B3"},
		{Label: "Device", URI: "device:B4"},
		{Label: "Device", URI: "device:B5"},
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
