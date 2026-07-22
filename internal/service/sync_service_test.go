package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/events"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/repository"
	"gitlab.com/pml/network-digital-twin/internal/schema"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// nopEventBus 空操作事件总线，同时提供 EventPublisher 和 EventConsumer（用于不需要事件流的测试）。
type nopEventBus struct{}

func (nopEventBus) Publish(_ context.Context, _ events.SyncEvent) error { return nil }
func (nopEventBus) Close() error                                        { return nil }
func (nopEventBus) Consume(ctx context.Context, handler func(ctx context.Context, event events.SyncEvent) error) error {
	<-ctx.Done()
	return ctx.Err()
}

// testEventBus 基于 Channel 的事件总线测试辅助（用于 HandleWebhook / StartConsumer 测试）。
type testEventBus struct {
	pub events.EventPublisher
	con events.EventConsumer
}

func newTestEventBus(bufferSize int) *testEventBus {
	pub, con := events.NewChannelEventBus(bufferSize)
	return &testEventBus{pub: pub, con: con}
}

func TestSyncResultFields(t *testing.T) {
	sr := SyncResult{
		NodesCreated:       10,
		RelationsCreated:   5,
		OrphanEdgesSkipped: 2,
		Warnings: []assembler.ValidationWarning{
			{Type: "orphan_edge", Detail: "HAS_INTERFACE: device:A → iface:missing"},
		},
		Duration: 3 * time.Second,
	}

	if sr.NodesCreated != 10 {
		t.Errorf("NodesCreated = %d, want 10", sr.NodesCreated)
	}
	if sr.RelationsCreated != 5 {
		t.Errorf("RelationsCreated = %d, want 5", sr.RelationsCreated)
	}
	if sr.OrphanEdgesSkipped != 2 {
		t.Errorf("OrphanEdgesSkipped = %d, want 2", sr.OrphanEdgesSkipped)
	}
	if len(sr.Warnings) != 1 {
		t.Fatalf("Warnings count = %d, want 1", len(sr.Warnings))
	}
	if sr.Warnings[0].Type != "orphan_edge" {
		t.Errorf("Warnings[0].Type = %q, want %q", sr.Warnings[0].Type, "orphan_edge")
	}
	if sr.Duration != 3*time.Second {
		t.Errorf("Duration = %v, want 3s", sr.Duration)
	}
}

func TestSyncEventActions(t *testing.T) {
	actions := []string{"update", "delete", "delete_relation"}
	for _, action := range actions {
		e := SyncEvent{Action: action}
		if e.Action != action {
			t.Errorf("Action = %q, want %q", e.Action, action)
		}
	}
}

func TestSyncEventUpdateData(t *testing.T) {
	e := SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Connector:  "mock-netbox",
		Data: []map[string]any{
			{"hostname": "router-01", "vendor": "Huawei"},
			{"hostname": "router-02", "vendor": "Cisco"},
		},
	}

	if e.EntityType != "Device" {
		t.Errorf("EntityType = %q, want %q", e.EntityType, "Device")
	}
	if e.Connector != "mock-netbox" {
		t.Errorf("Connector = %q, want %q", e.Connector, "mock-netbox")
	}
	if len(e.Data) != 2 {
		t.Fatalf("Data count = %d, want 2", len(e.Data))
	}
	if e.Data[0]["hostname"] != "router-01" {
		t.Errorf("Data[0][hostname] = %v, want %q", e.Data[0]["hostname"], "router-01")
	}
}

func TestSyncEventDeleteURIs(t *testing.T) {
	e := SyncEvent{
		Action: "delete",
		URIs:   []string{"device:SN001", "device:SN002"},
	}

	if len(e.URIs) != 2 {
		t.Fatalf("URIs count = %d, want 2", len(e.URIs))
	}
	if e.URIs[0] != "device:SN001" {
		t.Errorf("URIs[0] = %q, want %q", e.URIs[0], "device:SN001")
	}
}

func TestSyncEventDeleteRelations(t *testing.T) {
	e := SyncEvent{
		Action: "delete_relation",
		Relations: []assembler.Relation{
			{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
		},
	}

	if len(e.Relations) != 1 {
		t.Fatalf("Relations count = %d, want 1", len(e.Relations))
	}
	if e.Relations[0].Type != "HAS_INTERFACE" {
		t.Errorf("Relations[0].Type = %q, want %q", e.Relations[0].Type, "HAS_INTERFACE")
	}
}

// ---------------------------------------------------------------------------
// SyncService 构造函数和 FullSync 测试
// ---------------------------------------------------------------------------

// TestNewSyncService 验证 SyncService 构造函数。
func TestNewSyncService(t *testing.T) {
	reg := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(schema.NewSchemaRegistry())
	asm := assembler.NewGraphAssembler(schema.NewSchemaRegistry())
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(reg, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})
	if svc == nil {
		t.Fatal("NewSyncService() returned nil")
	}
}

// TestFullSync_Success 验证全量同步成功路径。
// 使用真实 ontology + testdata/mock_netbox 数据。
func TestFullSync_Success(t *testing.T) {
	reg := loadTestOntology(t)

	dataDir := filepath.Join("..", "..", "testdata", "mock_netbox")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Skipf("testdata/mock_netbox not found at %s, skipping", dataDir)
	}

	// 创建真实 MockConnector（读取 JSON 文件）
	conn := mock.NewMockConnector("mock-netbox", dataDir, []string{
		"Device", "Interface", "ISIS", "Link", "Network_Slice",
	})
	registry := connector.NewConnectorRegistry()
	registry.Register(conn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})
	result, err := svc.FullSync(context.Background())

	if err != nil {
		t.Fatalf("FullSync() error = %v", err)
	}

	// 验证 SyncResult 统计
	// Device:3 + Interface:12 + ISIS:3 + Link:2 + Network_Slice:1 = 21
	expectedNodes := 21
	if result.NodesCreated != expectedNodes {
		t.Errorf("NodesCreated = %d, want %d", result.NodesCreated, expectedNodes)
	}

	// HAS_INTERFACE:12 + CONNECTS_TO:2 + RUNS_ON:3 + ENDPOINT:4 = 21
	expectedRels := 21
	if result.RelationsCreated != expectedRels {
		t.Errorf("RelationsCreated = %d, want %d", result.RelationsCreated, expectedRels)
	}

	if result.OrphanEdgesSkipped != 0 {
		t.Errorf("OrphanEdgesSkipped = %d, want 0", result.OrphanEdgesSkipped)
	}

	if result.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", result.Duration)
	}

	// 验证 ClearDB 被调用
	if len(gdb.clearDBCalls) != 1 || gdb.clearDBCalls[0] != "default" {
		t.Errorf("ClearDB calls = %v, want [default]", gdb.clearDBCalls)
	}

	// 验证 BulkCreate 接收到正确数量的数据
	if len(gdb.bulkCreateNodes) != expectedNodes {
		t.Errorf("BulkCreate nodes = %d, want %d", len(gdb.bulkCreateNodes), expectedNodes)
	}
	if len(gdb.bulkCreateRels) != expectedRels {
		t.Errorf("BulkCreate rels = %d, want %d", len(gdb.bulkCreateRels), expectedRels)
	}
}

// TestFullSync_ConnectorFailureTolerance 验证单个 Connector 失败不阻断整个同步。
func TestFullSync_ConnectorFailureTolerance(t *testing.T) {
	reg := loadTestOntology(t)

	// Connector A: 正常返回数据
	connA := &mockConnector{
		name:        "conn-a",
		entityTypes: []string{"Device"},
		resources: map[string][]connector.Resource{
			"Device": {
				{Kind: "Device", ID: "1", Properties: map[string]any{
					"serial_number": "SN001", "hostname": "router-01", "vendor": "Huawei",
					"model": "NE40E", "status": "Up",
				}},
			},
		},
	}

	// Connector B: Collect 返回错误
	connB := &mockConnector{
		name:        "conn-b",
		entityTypes: []string{"Device"},
		collectErr:  errors.New("connection refused"),
	}

	registry := connector.NewConnectorRegistry()
	registry.Register(connA)
	registry.Register(connB)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})
	result, err := svc.FullSync(context.Background())

	// 不应返回错误（单个 Connector 失败被容忍）
	if err != nil {
		t.Fatalf("FullSync() error = %v, want nil (connector failure tolerated)", err)
	}

	// 只统计 Connector A 的数据（1 个 Device）
	if result.NodesCreated != 1 {
		t.Errorf("NodesCreated = %d, want 1 (only conn-a data)", result.NodesCreated)
	}
}

// TestFullSync_NormalizerFailureTolerance 验证 Normalizer 失败不阻断同步。
func TestFullSync_NormalizerFailureTolerance(t *testing.T) {
	reg := loadTestOntology(t)

	// Connector 返回混合数据：合法 Device + 不合法 Kind（不存在于 ontology）
	conn := &mockConnector{
		name:        "mixed-conn",
		entityTypes: []string{"Device", "UnknownType"},
		resources: map[string][]connector.Resource{
			"Device": {
				{Kind: "Device", ID: "1", Properties: map[string]any{
					"serial_number": "SN001", "hostname": "router-01", "vendor": "Huawei",
					"model": "NE40E", "status": "Up",
				}},
			},
			"UnknownType": {
				{Kind: "UnknownType", ID: "x1", Properties: map[string]any{"name": "unknown"}},
			},
		},
	}

	registry := connector.NewConnectorRegistry()
	registry.Register(conn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})
	result, err := svc.FullSync(context.Background())

	// 不应返回错误（UnknownType 被 Normalizer 跳过）
	if err != nil {
		t.Fatalf("FullSync() error = %v, want nil (normalizer failure tolerated)", err)
	}

	// 只统计合法 Device 数据（1 个节点）
	if result.NodesCreated != 1 {
		t.Errorf("NodesCreated = %d, want 1 (only valid Device)", result.NodesCreated)
	}
}

// TestFullSync_ClearDBError 验证 ClearDB 错误传播。
func TestFullSync_ClearDBError(t *testing.T) {
	reg := loadTestOntology(t)

	wantErr := errors.New("neo4j connection refused")
	gdb := &mockGraphDB{clearDBErr: wantErr}

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})
	_, err := svc.FullSync(context.Background())

	// 应返回错误
	if err == nil {
		t.Fatal("FullSync() should return error when ClearDB fails")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}

	// BulkCreate 不应被调用
	if gdb.bulkCreateNodes != nil {
		t.Errorf("BulkCreate should not be called after ClearDB failure, got %d nodes", len(gdb.bulkCreateNodes))
	}
}

// TestFullSync_BulkCreateError 验证 BulkCreate 错误传播。
func TestFullSync_BulkCreateError(t *testing.T) {
	reg := loadTestOntology(t)

	wantErr := errors.New("neo4j write timeout")
	gdb := &mockGraphDB{bulkCreateErr: wantErr}

	// 注册一个有数据的 connector
	conn := &mockConnector{
		name:        "test-conn",
		entityTypes: []string{"Device"},
		resources: map[string][]connector.Resource{
			"Device": {
				{Kind: "Device", ID: "1", Properties: map[string]any{
					"serial_number": "SN001", "hostname": "router-01", "vendor": "Huawei",
					"model": "NE40E", "status": "Up",
				}},
			},
		},
	}
	registry := connector.NewConnectorRegistry()
	registry.Register(conn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})
	_, err := svc.FullSync(context.Background())

	// 应返回错误
	if err == nil {
		t.Fatal("FullSync() should return error when BulkCreate fails")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

// TestFullSync_ConcurrentMutualExclusion 验证并发 FullSync 互斥。
func TestFullSync_ConcurrentMutualExclusion(t *testing.T) {
	reg := loadTestOntology(t)

	// 使用 atomic 计数器跟踪并发度
	var activeCount int32
	var maxActive int32

	// 创建特殊的 mockGraphDB，在 BulkCreate 中跟踪并发度
	gdb := &concurrentMockGraphDB{
		onBulkCreate: func() {
			current := atomic.AddInt32(&activeCount, 1)
			// 更新最大并发数
			for {
				old := atomic.LoadInt32(&maxActive)
				if current <= old {
					break
				}
				if atomic.CompareAndSwapInt32(&maxActive, old, current) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond) // 模拟工作负载
			atomic.AddInt32(&activeCount, -1)
		},
	}

	// 注册一个有数据的 connector
	conn := &mockConnector{
		name:        "test-conn",
		entityTypes: []string{"Device"},
		resources: map[string][]connector.Resource{
			"Device": {
				{Kind: "Device", ID: "1", Properties: map[string]any{
					"serial_number": "SN001", "hostname": "router-01", "vendor": "Huawei",
					"model": "NE40E", "status": "Up",
				}},
			},
		},
	}
	registry := connector.NewConnectorRegistry()
	registry.Register(conn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})

	// 启动两个 goroutine 同时调用 FullSync
	done := make(chan error, 2)
	go func() {
		_, err := svc.FullSync(context.Background())
		done <- err
	}()
	go func() {
		_, err := svc.FullSync(context.Background())
		done <- err
	}()

	// 等待两个都完成
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Errorf("FullSync() goroutine %d error = %v", i, err)
		}
	}

	// 验证最大并发数始终 <= 1
	if max := atomic.LoadInt32(&maxActive); max > 1 {
		t.Errorf("max concurrent FullSync = %d, want <= 1 (mutual exclusion failed)", max)
	}
}

// concurrentMockGraphDB 用于并发测试的 GraphDB mock。
type concurrentMockGraphDB struct {
	mockGraphDB
	onBulkCreate   func()
	onDeleteByURIs func()
}

func (m *concurrentMockGraphDB) BulkCreate(_ context.Context, _ string, nodes []assembler.Node, rels []assembler.Relation) error {
	m.bulkCreateNodes = nodes
	m.bulkCreateRels = rels
	if m.onBulkCreate != nil {
		m.onBulkCreate()
	}
	return m.bulkCreateErr
}

func (m *concurrentMockGraphDB) DeleteByURIs(_ context.Context, _ string, uris []string) error {
	m.deleteByURIsCalls = append(m.deleteByURIsCalls, uris)
	m.deleteByURIsCount.Add(1)
	if m.onDeleteByURIs != nil {
		m.onDeleteByURIs()
	}
	return m.deleteByURIsErr
}

// TestFullSync_LockReleaseOnError 验证错误时锁释放（defer unlock）。
func TestFullSync_LockReleaseOnError(t *testing.T) {
	reg := loadTestOntology(t)

	gdb := &mockGraphDB{bulkCreateErr: errors.New("write failed")}

	conn := &mockConnector{
		name:        "test-conn",
		entityTypes: []string{"Device"},
		resources: map[string][]connector.Resource{
			"Device": {
				{Kind: "Device", ID: "1", Properties: map[string]any{
					"serial_number": "SN001", "hostname": "router-01", "vendor": "Huawei",
					"model": "NE40E", "status": "Up",
				}},
			},
		},
	}
	registry := connector.NewConnectorRegistry()
	registry.Register(conn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})
	_, err := svc.FullSync(context.Background())

	// 确认 FullSync 返回错误
	if err == nil {
		t.Fatal("FullSync() should return error")
	}

	// 验证锁已释放：立即获取 Lock 应成功（1 秒超时检测死锁）
	lockDone := make(chan struct{})
	go func() {
		lock.Lock()
		close(lockDone)
		lock.Unlock()
	}()

	select {
	case <-lockDone:
		// 成功：锁已释放
	case <-time.After(1 * time.Second):
		t.Fatal("Lock not released after FullSync error (possible deadlock)")
	}
}

// TestFullSync_EmptyRegistry 验证空注册表场景。
func TestFullSync_EmptyRegistry(t *testing.T) {
	reg := loadTestOntology(t)

	// 不注册任何 connector
	registry := connector.NewConnectorRegistry()

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})
	result, err := svc.FullSync(context.Background())

	// 不应返回错误
	if err != nil {
		t.Fatalf("FullSync() error = %v", err)
	}

	// 统计为零
	if result.NodesCreated != 0 {
		t.Errorf("NodesCreated = %d, want 0", result.NodesCreated)
	}
	if result.RelationsCreated != 0 {
		t.Errorf("RelationsCreated = %d, want 0", result.RelationsCreated)
	}

	// ClearDB 仍被调用
	if len(gdb.clearDBCalls) != 1 {
		t.Errorf("ClearDB calls = %d, want 1 (should still clear even with empty registry)", len(gdb.clearDBCalls))
	}
}

// ---------------------------------------------------------------------------
// I-15: IncrementalSync 测试
// ---------------------------------------------------------------------------

// TestIncrementalSync_Update_Success 验证 update 事件正确走 Normalize → Assemble → Upsert 路径。
func TestIncrementalSync_Update_Success(t *testing.T) {
	reg := loadTestOntology(t)

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})

	event := SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Data: []map[string]any{
			{"serial_number": "SN001", "hostname": "router-01", "vendor": "Huawei", "model": "NE40E", "status": "Up"},
		},
	}

	result, err := svc.IncrementalSync(context.Background(), event)
	if err != nil {
		t.Fatalf("IncrementalSync(update) error = %v", err)
	}

	if result.NodesCreated != 1 {
		t.Errorf("NodesCreated = %d, want 1", result.NodesCreated)
	}
	if len(gdb.upsertNodes) != 1 {
		t.Errorf("Upsert nodes = %d, want 1", len(gdb.upsertNodes))
	}
	if result.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", result.Duration)
	}
}

// TestIncrementalSync_Delete_Success 验证 delete 事件调用 DeleteByURIs。
func TestIncrementalSync_Delete_Success(t *testing.T) {
	reg := loadTestOntology(t)

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})

	event := SyncEvent{
		Action: "delete",
		URIs:   []string{"device:SN001", "device:SN002"},
	}

	result, err := svc.IncrementalSync(context.Background(), event)
	if err != nil {
		t.Fatalf("IncrementalSync(delete) error = %v", err)
	}

	if len(gdb.deleteByURIsCalls) != 1 {
		t.Fatalf("DeleteByURIs calls = %d, want 1", len(gdb.deleteByURIsCalls))
	}
	if len(gdb.deleteByURIsCalls[0]) != 2 {
		t.Errorf("DeleteByURIs URIs count = %d, want 2", len(gdb.deleteByURIsCalls[0]))
	}
	if gdb.deleteByURIsCalls[0][0] != "device:SN001" {
		t.Errorf("DeleteByURIs URIs[0] = %q, want %q", gdb.deleteByURIsCalls[0][0], "device:SN001")
	}
	if result.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", result.Duration)
	}
}

// TestIncrementalSync_DeleteRelation_Success 验证 delete_relation 事件调用 DeleteRelations。
func TestIncrementalSync_DeleteRelation_Success(t *testing.T) {
	reg := loadTestOntology(t)

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})

	event := SyncEvent{
		Action: "delete_relation",
		Relations: []assembler.Relation{
			{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
		},
	}

	_, err := svc.IncrementalSync(context.Background(), event)
	if err != nil {
		t.Fatalf("IncrementalSync(delete_relation) error = %v", err)
	}

	if len(gdb.deleteRelationsCalls) != 1 {
		t.Fatalf("DeleteRelations calls = %d, want 1", len(gdb.deleteRelationsCalls))
	}
	if len(gdb.deleteRelationsCalls[0]) != 1 {
		t.Fatalf("DeleteRelations rels count = %d, want 1", len(gdb.deleteRelationsCalls[0]))
	}
	if gdb.deleteRelationsCalls[0][0].Type != "HAS_INTERFACE" {
		t.Errorf("Relation type = %q, want HAS_INTERFACE", gdb.deleteRelationsCalls[0][0].Type)
	}
}

// TestIncrementalSync_UnknownAction 验证未知 Action 返回 error。
func TestIncrementalSync_UnknownAction(t *testing.T) {
	reg := loadTestOntology(t)

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})

	event := SyncEvent{Action: "invalid_action"}
	_, err := svc.IncrementalSync(context.Background(), event)
	if err == nil {
		t.Fatal("IncrementalSync(invalid) should return error")
	}
}

// TestIncrementalSync_NormalizerFailureTolerance 验证 update 中部分数据 normalize 失败不阻断。
func TestIncrementalSync_NormalizerFailureTolerance(t *testing.T) {
	reg := loadTestOntology(t)

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})

	event := SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Data: []map[string]any{
			{"serial_number": "SN001", "hostname": "router-01", "vendor": "Huawei", "model": "NE40E", "status": "Up"},
			{"serial_number": "", "hostname": "bad-device"}, // 缺 stableKey
		},
	}

	result, err := svc.IncrementalSync(context.Background(), event)
	if err != nil {
		t.Fatalf("IncrementalSync() error = %v, want nil (normalizer failure tolerated)", err)
	}

	// 只有合法数据被 Upsert
	if result.NodesCreated != 1 {
		t.Errorf("NodesCreated = %d, want 1 (bad data skipped)", result.NodesCreated)
	}
}

// TestIncrementalSync_UpsertError 验证 Upsert 错误传播。
func TestIncrementalSync_UpsertError(t *testing.T) {
	reg := loadTestOntology(t)

	wantErr := errors.New("neo4j upsert failed")
	gdb := &mockGraphDB{upsertErr: wantErr}

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})

	event := SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Data: []map[string]any{
			{"serial_number": "SN001", "hostname": "router-01", "vendor": "Huawei", "model": "NE40E", "status": "Up"},
		},
	}

	_, err := svc.IncrementalSync(context.Background(), event)
	if err == nil {
		t.Fatal("IncrementalSync() should return error when Upsert fails")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original, got: %v", err)
	}
}

// TestIncrementalSync_DeleteByURIsError 验证 DeleteByURIs 错误传播。
func TestIncrementalSync_DeleteByURIsError(t *testing.T) {
	reg := loadTestOntology(t)

	wantErr := errors.New("neo4j delete failed")
	gdb := &mockGraphDB{deleteByURIsErr: wantErr}

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})

	event := SyncEvent{
		Action: "delete",
		URIs:   []string{"device:SN001"},
	}

	_, err := svc.IncrementalSync(context.Background(), event)
	if err == nil {
		t.Fatal("IncrementalSync() should return error when DeleteByURIs fails")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original, got: %v", err)
	}
}

// TestIncrementalSync_DeleteRelationsError 验证 DeleteRelations 错误传播。
func TestIncrementalSync_DeleteRelationsError(t *testing.T) {
	reg := loadTestOntology(t)

	wantErr := errors.New("neo4j delete relations failed")
	gdb := &mockGraphDB{deleteRelationsErr: wantErr}

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	lock := snapshot.NewGraphLock()

	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{})

	event := SyncEvent{
		Action: "delete_relation",
		Relations: []assembler.Relation{
			{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:eth0"},
		},
	}

	_, err := svc.IncrementalSync(context.Background(), event)
	if err == nil {
		t.Fatal("IncrementalSync() should return error when DeleteRelations fails")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// I-15: HandleWebhook 测试
// ---------------------------------------------------------------------------

// TestHandleWebhook_EnqueueSuccess 验证入队成功返回 nil。
func TestHandleWebhook_EnqueueSuccess(t *testing.T) {
	reg := loadTestOntology(t)

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	bus := newTestEventBus(2)
	svc := NewSyncService(registry, norm, asm, gdb, lock, bus.pub, bus.con)

	event := events.SyncEvent{Action: "delete", URIs: []string{"device:SN001"}}
	err := svc.HandleWebhook(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v, want nil", err)
	}
}

// TestHandleWebhook_ChannelFull 验证 publisher 通道满时返回 error。
func TestHandleWebhook_ChannelFull(t *testing.T) {
	reg := loadTestOntology(t)

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	// buffer=1 的共享通道，不启动 consumer，填满后 Publish 返回 error
	bus := newTestEventBus(1)
	svc := NewSyncService(registry, norm, asm, gdb, lock, bus.pub, bus.con)

	// 填满共享通道
	_ = bus.pub.Publish(context.Background(), events.SyncEvent{Action: "delete"})

	// 再次 Publish 应失败
	err := svc.HandleWebhook(context.Background(), events.SyncEvent{Action: "delete"})
	if err == nil {
		t.Fatal("HandleWebhook() should return error when channel is full")
	}
}

// ---------------------------------------------------------------------------
// I-15: StartConsumer 测试
// ---------------------------------------------------------------------------

// TestStartConsumer_ProcessesEvents 验证消费者协程处理事件。
func TestStartConsumer_ProcessesEvents(t *testing.T) {
	reg := loadTestOntology(t)

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	bus := newTestEventBus(10)
	svc := NewSyncService(registry, norm, asm, gdb, lock, bus.pub, bus.con)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.StartConsumer(ctx)

	// 通过 publisher 发送 2 个 delete 事件到共享通道
	bus.pub.Publish(context.Background(), events.SyncEvent{Action: "delete", URIs: []string{"device:SN001"}})
	bus.pub.Publish(context.Background(), events.SyncEvent{Action: "delete", URIs: []string{"device:SN002"}})

	// 等待处理完成
	deadline := time.After(2 * time.Second)
	for {
		if gdb.deleteByURIsCount.Load() >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("consumer processed %d events, want 2", gdb.deleteByURIsCount.Load())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// TestStartConsumer_SerialProcessing 验证消费者串行处理（同一时刻最多 1 个事件在处理）。
func TestStartConsumer_SerialProcessing(t *testing.T) {
	reg := loadTestOntology(t)

	var activeCount int32
	var maxActive int32

	gdb := &concurrentMockGraphDB{
		onDeleteByURIs: func() {
			current := atomic.AddInt32(&activeCount, 1)
			for {
				old := atomic.LoadInt32(&maxActive)
				if current <= old {
					break
				}
				if atomic.CompareAndSwapInt32(&maxActive, old, current) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&activeCount, -1)
		},
	}

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	lock := snapshot.NewGraphLock()

	bus := newTestEventBus(10)
	svc := NewSyncService(registry, norm, asm, gdb, lock, bus.pub, bus.con)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.StartConsumer(ctx)

	// 通过 publisher 发送 3 个事件到共享通道
	for i := 0; i < 3; i++ {
		bus.pub.Publish(context.Background(), events.SyncEvent{Action: "delete", URIs: []string{"device:SN00" + string(rune('1'+i))}})
	}

	// 等待处理完成
	deadline := time.After(2 * time.Second)
	for {
		if gdb.deleteByURIsCount.Load() >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("consumer processed %d/3 events", gdb.deleteByURIsCount.Load())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if max := atomic.LoadInt32(&maxActive); max > 1 {
		t.Errorf("max concurrent processing = %d, want <= 1 (serial processing violated)", max)
	}
}

// TestStartConsumer_LockAcquiredPerEvent 验证消费者处理事件时持有 GraphLock。
func TestStartConsumer_LockAcquiredPerEvent(t *testing.T) {
	reg := loadTestOntology(t)

	processing := make(chan struct{})

	gdb := &concurrentMockGraphDB{
		onDeleteByURIs: func() {
			close(processing)
			time.Sleep(100 * time.Millisecond)
		},
	}

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	lock := snapshot.NewGraphLock()

	bus := newTestEventBus(10)
	svc := NewSyncService(registry, norm, asm, gdb, lock, bus.pub, bus.con)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.StartConsumer(ctx)

	bus.pub.Publish(context.Background(), events.SyncEvent{Action: "delete", URIs: []string{"device:SN001"}})

	// 等待事件开始处理
	select {
	case <-processing:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer did not start processing within timeout")
	}

	// 尝试获取锁 — 应被阻塞（消费者持有锁）
	acquired := make(chan struct{})
	go func() {
		lock.Lock()
		close(acquired)
		lock.Unlock()
	}()

	select {
	case <-acquired:
		t.Fatal("external Lock should block while consumer holds the lock")
	case <-time.After(50 * time.Millisecond):
		// 预期：被阻塞
	}

	// 等待处理完成，锁应被释放
	select {
	case <-acquired:
		// 成功：锁在处理完成后释放
	case <-time.After(2 * time.Second):
		t.Fatal("lock not released after consumer finished processing")
	}
}

// TestStartConsumer_StopsOnContextCancel 验证 context 取消后消费者停止。
func TestStartConsumer_StopsOnContextCancel(t *testing.T) {
	reg := loadTestOntology(t)

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	bus := newTestEventBus(10)
	svc := NewSyncService(registry, norm, asm, gdb, lock, bus.pub, bus.con)

	ctx, cancel := context.WithCancel(context.Background())
	svc.StartConsumer(ctx)

	// 取消 context
	cancel()

	// 消费者应在短时间内停止（Consume 返回 context.Canceled）
	time.Sleep(50 * time.Millisecond)

	// 通过 publisher 发送事件（consumer 已停止，不会处理）
	bus.pub.Publish(context.Background(), events.SyncEvent{Action: "delete", URIs: []string{"device:SN001"}})

	// 等待一小段时间，验证事件未被处理
	time.Sleep(100 * time.Millisecond)
	if len(gdb.deleteByURIsCalls) != 0 {
		t.Errorf("consumer should not process events after context cancel, but processed %d", len(gdb.deleteByURIsCalls))
	}
}

// ---------------------------------------------------------------------------
// V2-07: SyncLogRepository 集成测试
// ---------------------------------------------------------------------------

// mockSyncLogRepo 用于测试的 SyncLogRepository mock。
type mockSyncLogRepo struct {
	mu        sync.Mutex
	records   []repository.SyncLogRecord
	createErr error // 注入错误
}

var _ repository.SyncLogRepository = (*mockSyncLogRepo)(nil)

func (m *mockSyncLogRepo) Create(_ context.Context, r repository.SyncLogRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	m.records = append(m.records, r)
	return nil
}

func (m *mockSyncLogRepo) List(_ context.Context, _ int) ([]repository.SyncLogRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.records, nil
}

func (m *mockSyncLogRepo) ListByType(_ context.Context, syncType string, _ int) ([]repository.SyncLogRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var filtered []repository.SyncLogRecord
	for _, r := range m.records {
		if r.SyncType == syncType {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

func (m *mockSyncLogRepo) Count(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.records)), nil
}

// TestFullSync_WithSyncLogRepo 验证 FullSync 成功后 syncLogRepo 有记录。
func TestFullSync_WithSyncLogRepo(t *testing.T) {
	reg := loadTestOntology(t)

	dataDir := filepath.Join("..", "..", "testdata", "mock_netbox")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Skipf("testdata/mock_netbox not found at %s, skipping", dataDir)
	}

	conn := mock.NewMockConnector("mock-netbox", dataDir, []string{
		"Device", "Interface", "ISIS", "Link", "Network_Slice",
	})
	registry := connector.NewConnectorRegistry()
	registry.Register(conn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	syncLog := &mockSyncLogRepo{}
	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{},
		WithSyncLogRepository(syncLog))

	_, err := svc.FullSync(context.Background())
	if err != nil {
		t.Fatalf("FullSync() error = %v", err)
	}

	// 验证 syncLogRepo 有 1 条记录
	syncLog.mu.Lock()
	defer syncLog.mu.Unlock()
	if len(syncLog.records) != 1 {
		t.Fatalf("expected 1 sync log record, got %d", len(syncLog.records))
	}

	rec := syncLog.records[0]
	if rec.SyncType != "full" {
		t.Errorf("SyncType = %q, want %q", rec.SyncType, "full")
	}
	if rec.Status != "success" {
		t.Errorf("Status = %q, want %q", rec.Status, "success")
	}
	if rec.NodesCreated != 21 {
		t.Errorf("NodesCreated = %d, want 21", rec.NodesCreated)
	}
	if rec.DurationMs < 0 {
		t.Errorf("DurationMs = %d, want >= 0", rec.DurationMs)
	}
}

// TestFullSync_SyncLogRepoError 验证 syncLogRepo.Create 失败不阻断 FullSync。
func TestFullSync_SyncLogRepoError(t *testing.T) {
	reg := loadTestOntology(t)

	registry := connector.NewConnectorRegistry()
	// 注册一个有数据的 connector
	conn := &mockConnector{
		name:        "test-conn",
		entityTypes: []string{"Device"},
		resources: map[string][]connector.Resource{
			"Device": {
				{Kind: "Device", ID: "1", Properties: map[string]any{
					"serial_number": "SN001", "hostname": "router-01", "vendor": "Huawei",
					"model": "NE40E", "status": "Up",
				}},
			},
		},
	}
	registry.Register(conn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	// 注入错误的 syncLogRepo
	syncLog := &mockSyncLogRepo{createErr: errors.New("pg connection lost")}
	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{},
		WithSyncLogRepository(syncLog))

	result, err := svc.FullSync(context.Background())

	// FullSync 应成功（syncLogRepo 失败仅 slog.Warn）
	if err != nil {
		t.Fatalf("FullSync() error = %v, want nil (sync log error should not block)", err)
	}
	if result.NodesCreated != 1 {
		t.Errorf("NodesCreated = %d, want 1", result.NodesCreated)
	}
}

// TestIncrementalSync_WithSyncLogRepo 验证 IncrementalSync 成功后有同步日志。
func TestIncrementalSync_WithSyncLogRepo(t *testing.T) {
	reg := loadTestOntology(t)

	registry := connector.NewConnectorRegistry()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	syncLog := &mockSyncLogRepo{}
	svc := NewSyncService(registry, norm, asm, gdb, lock, nopEventBus{}, nopEventBus{},
		WithSyncLogRepository(syncLog))

	event := SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Data: []map[string]any{
			{"serial_number": "SN001", "hostname": "router-01", "vendor": "Huawei", "model": "NE40E", "status": "Up"},
		},
	}

	_, err := svc.IncrementalSync(context.Background(), event)
	if err != nil {
		t.Fatalf("IncrementalSync() error = %v", err)
	}

	syncLog.mu.Lock()
	defer syncLog.mu.Unlock()
	if len(syncLog.records) != 1 {
		t.Fatalf("expected 1 sync log record, got %d", len(syncLog.records))
	}

	rec := syncLog.records[0]
	if rec.SyncType != "incremental" {
		t.Errorf("SyncType = %q, want %q", rec.SyncType, "incremental")
	}
	if rec.Status != "success" {
		t.Errorf("Status = %q, want %q", rec.Status, "success")
	}
	if rec.NodesCreated != 1 {
		t.Errorf("NodesCreated = %d, want 1", rec.NodesCreated)
	}
}
