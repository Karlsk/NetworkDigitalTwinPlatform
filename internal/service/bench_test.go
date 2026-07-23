package service

import (
	"context"
	"fmt"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/events"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/schema"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// createMockResources 生成指定数量的 mock Device 资源。
func createMockResources(count int) []connector.Resource {
	resources := make([]connector.Resource, 0, count)
	for i := 0; i < count; i++ {
		resources = append(resources, connector.Resource{
			Kind: "Device",
			ID:   fmt.Sprintf("device-%04d", i),
			Properties: map[string]any{
				"serial_number": fmt.Sprintf("SN-%04d", i),
				"hostname":      fmt.Sprintf("PE-Router-%04d", i),
				"vendor":        "TestVendor",
				"hw_model":      "TestModel",
				"status":        "Up",
			},
		})
	}
	return resources
}

// BenchmarkFullSync 测量 FullSync 100 节点的耗时。
// 使用 mock connector + mock GraphDB，不依赖真实 Neo4j。
func BenchmarkFullSync(b *testing.B) {
	// 加载 ontology
	reg := schema.NewSchemaRegistry()
	if err := reg.Load("../../ontology"); err != nil {
		b.Skipf("ontology not available: %v", err)
	}

	// 创建 mock connector
	mockConn := &mockConnector{
		name:        "bench-mock",
		entityTypes: []string{"Device"},
		resources: map[string][]connector.Resource{
			"Device": createMockResources(100),
		},
	}

	connReg := connector.NewConnectorRegistry()
	connReg.Register(mockConn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()
	pub, con := events.NewChannelEventBus(100)

	syncSvc := NewSyncService(connReg, norm, asm, gdb, lock, pub, con)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := syncSvc.FullSync(ctx)
		if err != nil {
			b.Fatalf("FullSync error: %v", err)
		}
		if result.NodesCreated == 0 {
			b.Fatal("FullSync created 0 nodes")
		}
		b.ReportMetric(float64(result.NodesCreated), "nodes/op")
	}
}

// BenchmarkIncrementalSync 测量增量同步单事件的耗时。
func BenchmarkIncrementalSync(b *testing.B) {
	reg := schema.NewSchemaRegistry()
	if err := reg.Load("../../ontology"); err != nil {
		b.Skipf("ontology not available: %v", err)
	}

	mockConn := &mockConnector{
		name:        "bench-mock",
		entityTypes: []string{"Device"},
		resources:   map[string][]connector.Resource{},
	}

	connReg := connector.NewConnectorRegistry()
	connReg.Register(mockConn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()
	pub, con := events.NewChannelEventBus(100)

	syncSvc := NewSyncService(connReg, norm, asm, gdb, lock, pub, con)

	ctx := context.Background()

	event := SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Data: []map[string]any{
			{
				"serial_number": "SN-BENCH",
				"hostname":      "Bench-Device",
				"vendor":        "TestVendor",
				"hw_model":      "TestModel",
				"status":        "Up",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := syncSvc.IncrementalSync(ctx, event)
		if err != nil {
			b.Fatalf("IncrementalSync error: %v", err)
		}
	}
}
