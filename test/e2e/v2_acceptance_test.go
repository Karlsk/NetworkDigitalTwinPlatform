//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/events"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/service"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// TestV2_Acceptance_FullSyncAndSnapshot 验证 V2 核心流程：
// FullSync → Snapshot.Create → IncrementalSync → Snapshot.Create → Diff → Restore → 验证恢复。
func TestV2_Acceptance_FullSyncAndSnapshot(t *testing.T) {
	client := newE2EClient(t)
	lock := snapshot.NewGraphLock()
	syncSvc := newE2ESyncService(t, client, lock)
	snapMgr := newE2ESnapshotManager(t, client, lock)
	defer backupAndRestoreDefault(t, client)()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 清理前次遗留
	_ = snapMgr.Delete(ctx, "v2-accept-snap1")
	_ = snapMgr.Delete(ctx, "v2-accept-snap2")

	// === 1. FullSync ===
	result, err := syncSvc.FullSync(ctx)
	if err != nil {
		t.Fatalf("FullSync error = %v", err)
	}
	if result.NodesCreated == 0 {
		t.Fatal("FullSync created 0 nodes")
	}
	t.Logf("FullSync: nodes=%d, rels=%d", result.NodesCreated, result.RelationsCreated)

	// === 2. Create snapshot "v2-accept-snap1" ===
	snap1, err := snapMgr.Create(ctx, "v2-accept-snap1")
	if err != nil {
		t.Fatalf("Create(snap1) error = %v", err)
	}
	if _, err := os.Stat(snap1.FilePath); os.IsNotExist(err) {
		t.Fatalf("snapshot YAML not created: %s", snap1.FilePath)
	}
	t.Logf("snap1: nodes=%d, rels=%d", snap1.NodeCount, snap1.RelCount)

	// === 3. IncrementalSync — 新增设备 ===
	incEvent := service.SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Data: []map[string]any{
			{
				"serial_number": "SN-V2-ACCEPT",
				"hostname":      "V2-Acceptance-Device",
				"vendor":        "TestVendor",
				"hw_model":      "TestModel",
				"status":        "Up",
			},
		},
	}
	incResult, err := syncSvc.IncrementalSync(ctx, incEvent)
	if err != nil {
		t.Fatalf("IncrementalSync error = %v", err)
	}
	if incResult.NodesCreated != 1 {
		t.Errorf("IncrementalSync NodesCreated = %d, want 1", incResult.NodesCreated)
	}

	// === 4. Create snapshot "v2-accept-snap2" ===
	snap2, err := snapMgr.Create(ctx, "v2-accept-snap2")
	if err != nil {
		t.Fatalf("Create(snap2) error = %v", err)
	}
	if snap2.NodeCount <= snap1.NodeCount {
		t.Errorf("snap2.NodeCount (%d) should be > snap1.NodeCount (%d)", snap2.NodeCount, snap1.NodeCount)
	}

	// === 5. Diff ===
	diff, err := snapMgr.Diff(ctx, "v2-accept-snap1", "v2-accept-snap2")
	if err != nil {
		t.Fatalf("Diff error = %v", err)
	}
	if len(diff.AddedNodes) == 0 {
		t.Error("Diff.AddedNodes is empty, expected at least 1")
	}

	// === 6. Restore to snap1 ===
	if err := snapMgr.Restore(ctx, "v2-accept-snap1"); err != nil {
		t.Fatalf("Restore error = %v", err)
	}
	restoredNodes := countNodes(t, client, "default")
	if restoredNodes != snap1.NodeCount {
		t.Errorf("after Restore, nodes=%d, want %d", restoredNodes, snap1.NodeCount)
	}

	// === 7. Audit log verification ===
	entries := snapMgr.AuditLog().Recent(20)
	if len(entries) == 0 {
		t.Error("AuditRecent returned 0 entries, expected audit log entries")
	}
	actionsFound := make(map[string]bool)
	for _, e := range entries {
		actionsFound[e.Action] = true
	}
	for _, expected := range []string{"create", "restore"} {
		if !actionsFound[expected] {
			t.Errorf("audit log missing action %q", expected)
		}
	}

	// cleanup
	_ = snapMgr.Delete(ctx, "v2-accept-snap1")
	_ = snapMgr.Delete(ctx, "v2-accept-snap2")
}

// TestV2_Acceptance_ChannelFallback 验证 kafka.enabled=false 时使用内存 Channel，行为与 V1 一致。
func TestV2_Acceptance_ChannelFallback(t *testing.T) {
	client := newE2EClient(t)
	reg := loadOntology(t)
	lock := snapshot.NewGraphLock()
	defer backupAndRestoreDefault(t, client)()

	// 使用 Channel EventBus（无 Kafka）
	pub, con := events.NewChannelEventBus(50)

	dataDir := testdataDir(t)
	entityTypes := []string{"Device", "Interface", "ISIS", "Link", "Network_Slice"}
	conn := mock.NewMockConnector("acceptance-mock", dataDir, entityTypes)
	connReg := connector.NewConnectorRegistry()
	connReg.Register(conn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	syncSvc := service.NewSyncService(connReg, norm, asm, client, lock, pub, con)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// FullSync 通过 Channel EventBus 应正常工作
	result, err := syncSvc.FullSync(ctx)
	if err != nil {
		t.Fatalf("FullSync with Channel EventBus error = %v", err)
	}
	if result.NodesCreated == 0 {
		t.Fatal("FullSync with Channel created 0 nodes")
	}
	t.Logf("Channel fallback FullSync: nodes=%d, rels=%d", result.NodesCreated, result.RelationsCreated)

	// 验证 Webhook 通过 Channel 投递正常
	event := events.SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Data: []map[string]any{
			{
				"serial_number": "SN-CHANNEL-FB",
				"hostname":      "ChannelFallbackDevice",
				"vendor":        "V", "hw_model": "M", "status": "Up",
			},
		},
	}
	if err := syncSvc.HandleWebhook(ctx, event); err != nil {
		t.Fatalf("HandleWebhook with Channel error = %v", err)
	}
}

// TestV2_Acceptance_ConnectorFactory 验证 ConnectorFactory 工厂模式 + mock/controller 双 Connector。
func TestV2_Acceptance_ConnectorFactory(t *testing.T) {
	client := newE2EClient(t)
	reg := loadOntology(t)
	lock := snapshot.NewGraphLock()
	defer backupAndRestoreDefault(t, client)()

	// 创建 ConnectorFactory
	factory := connector.NewConnectorFactory()

	// 注册 mock builder
	factory.RegisterBuilder("mock", func(name string, cfg map[string]any, entityTypes []string) (connector.Connector, error) {
		dataDir, _ := cfg["data_dir"].(string)
		return mock.NewMockConnector(name, dataDir, entityTypes), nil
	})

	// 通过 factory 创建 mock connector
	mockConn, err := factory.Create(connector.ConnectorConfigEntry{
		Name:        "acceptance-mock",
		Type:        "mock",
		Config:      map[string]any{"data_dir": testdataDir(t)},
		EntityTypes: []string{"Device", "Interface", "ISIS", "Link", "Network_Slice"},
	})
	if err != nil {
		t.Fatalf("Factory.Create(mock) error = %v", err)
	}

	connReg := connector.NewConnectorRegistry()
	connReg.Register(mockConn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	pub, _ := events.NewChannelEventBus(50)
	syncSvc := service.NewSyncService(connReg, norm, asm, client, lock, pub, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := syncSvc.FullSync(ctx)
	if err != nil {
		t.Fatalf("FullSync via factory connector error = %v", err)
	}
	if result.NodesCreated == 0 {
		t.Fatal("FullSync via factory created 0 nodes")
	}
	t.Logf("Factory FullSync: nodes=%d, rels=%d", result.NodesCreated, result.RelationsCreated)
}

// TestV2_Acceptance_SnapshotListAndDelete 验证快照 List/Delete 生命周期。
func TestV2_Acceptance_SnapshotListAndDelete(t *testing.T) {
	client := newE2EClient(t)
	lock := snapshot.NewGraphLock()
	syncSvc := newE2ESyncService(t, client, lock)
	snapMgr := newE2ESnapshotManager(t, client, lock)
	defer backupAndRestoreDefault(t, client)()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 准备数据
	if _, err := syncSvc.FullSync(ctx); err != nil {
		t.Fatalf("FullSync error = %v", err)
	}

	// Create 3 snapshots
	for _, name := range []string{"list-del-1", "list-del-2", "list-del-3"} {
		if _, err := snapMgr.Create(ctx, name); err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
	}

	// List — 应至少有 3 个
	metas, err := snapMgr.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(metas) < 3 {
		t.Errorf("List() returned %d snapshots, want >= 3", len(metas))
	}

	// Delete 1
	if err := snapMgr.Delete(ctx, "list-del-1"); err != nil {
		t.Fatalf("Delete(list-del-1) error = %v", err)
	}

	// List again
	metas2, err := snapMgr.List(ctx)
	if err != nil {
		t.Fatalf("List() after delete error = %v", err)
	}
	for _, m := range metas2 {
		if m.Name == "list-del-1" {
			t.Error("list-del-1 still in List() after Delete")
		}
	}

	// cleanup
	_ = snapMgr.Delete(ctx, "list-del-2")
	_ = snapMgr.Delete(ctx, "list-del-3")
}

// TestV2_Acceptance_LogicalDBIsolation 验证逻辑 DB 隔离。
func TestV2_Acceptance_LogicalDBIsolation(t *testing.T) {
	client := newE2EClient(t)
	dbA := uniqueDBName(t) + "_isoA"
	dbB := uniqueDBName(t) + "_isoB"
	defer cleanupDB(t, client, dbA)
	defer cleanupDB(t, client, dbB)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// dbA: 2 个节点
	nodesA := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:iso-A1"},
		{Labels: []string{"Device"}, URI: "device:iso-A2"},
	}
	if err := client.BulkCreate(ctx, dbA, nodesA, nil); err != nil {
		t.Fatalf("BulkCreate dbA error = %v", err)
	}

	// dbB: 4 个节点
	nodesB := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:iso-B1"},
		{Labels: []string{"Device"}, URI: "device:iso-B2"},
		{Labels: []string{"Device"}, URI: "device:iso-B3"},
		{Labels: []string{"Device"}, URI: "device:iso-B4"},
	}
	if err := client.BulkCreate(ctx, dbB, nodesB, nil); err != nil {
		t.Fatalf("BulkCreate dbB error = %v", err)
	}

	// 验证隔离
	if got := countNodes(t, client, dbA); got != 2 {
		t.Errorf("dbA countNodes = %d, want 2", got)
	}
	if got := countNodes(t, client, dbB); got != 4 {
		t.Errorf("dbB countNodes = %d, want 4", got)
	}

	// ClearDB(dbA) 不影响 dbB
	if err := client.ClearDB(ctx, dbA); err != nil {
		t.Fatalf("ClearDB dbA error = %v", err)
	}
	if got := countNodes(t, client, dbB); got != 4 {
		t.Errorf("after ClearDB(dbA), dbB = %d, want 4", got)
	}
}
