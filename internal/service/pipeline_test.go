// Package service 实现业务编排层
package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/schema"
)

// loadTestOntology 加载 ontology/ 目录用于集成测试。
func loadTestOntology(t *testing.T) schema.SchemaRegistry {
	t.Helper()
	ontologyDir := filepath.Join("..", "..", "ontology")
	if _, err := os.Stat(ontologyDir); os.IsNotExist(err) {
		t.Skipf("ontology directory not found at %s, skipping", ontologyDir)
	}
	r := schema.NewSchemaRegistry()
	if err := r.Load(ontologyDir); err != nil {
		t.Fatalf("Load(%q) error = %v", ontologyDir, err)
	}
	return r
}

// runPipeline 执行 Connector → Normalizer → Assembler 全管线处理。
func runPipeline(t *testing.T, reg schema.SchemaRegistry, dataDir string, entityTypes []string) (*assembler.GraphModel, []assembler.ValidationWarning) {
	t.Helper()

	conn := mock.NewMockConnector("test-mock", dataDir, entityTypes)
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)

	var allResources []normalizer.NormalizedResource
	ctx := context.Background()

	for _, et := range entityTypes {
		resources, err := conn.Collect(ctx, et)
		if err != nil {
			t.Fatalf("Collect(%s) error = %v", et, err)
		}
		for _, res := range resources {
			nr, err := norm.Normalize(res)
			if err != nil {
				t.Fatalf("Normalize(%s/%s) error = %v", res.Kind, res.ID, err)
			}
			allResources = append(allResources, *nr)
		}
	}

	gm, warnings, err := asm.Assemble(allResources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	return gm, warnings
}

// TestPipeline_MockNetbox_FullPipeline 测试 mock_netbox 数据的完整管线:
// Connector → Normalizer → Assembler → GraphModel + Warnings。
func TestPipeline_MockNetbox_FullPipeline(t *testing.T) {
	reg := loadTestOntology(t)

	dataDir := filepath.Join("..", "..", "testdata", "mock_netbox")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Skipf("testdata/mock_netbox not found at %s, skipping", dataDir)
	}

	entityTypes := []string{"Device", "Interface", "ISIS", "Link", "Network_Slice"}
	gm, warnings := runPipeline(t, reg, dataDir, entityTypes)

	// === 验证节点数 ===
	// Device:3 + Interface:12 + ISIS:3 + Link:2 + Network_Slice:1 = 21
	expectedNodes := 21
	if len(gm.Nodes) != expectedNodes {
		t.Errorf("Nodes count = %d, want %d", len(gm.Nodes), expectedNodes)
	}

	// 按 MostSpecificLabel 统计节点
	nodeCountByLabel := make(map[string]int)
	for _, n := range gm.Nodes {
		nodeCountByLabel[n.MostSpecificLabel()]++
	}
	expectedByLabel := map[string]int{
		"Device": 3, "Interface": 12, "ISIS": 3, "Link": 2, "Network_Slice": 1,
	}
	for label, expected := range expectedByLabel {
		if nodeCountByLabel[label] != expected {
			t.Errorf("Nodes[%s] count = %d, want %d", label, nodeCountByLabel[label], expected)
		}
	}

	// === 验证关系数 ===
	// HAS_INTERFACE: 12 (4 per device * 3 devices)
	// CONNECTS_TO: 2 (SN12346→SN12345, SN12347→SN12346)
	// RUNS_ON: 3 (ISIS-001, ISIS-002, ISIS-003)
	// ENDPOINT: 4 (LINK-001 2 endpoints + LINK-002 2 endpoints)
	// Total: 21
	expectedRels := 21
	if len(gm.Relations) != expectedRels {
		t.Errorf("Relations count = %d, want %d", len(gm.Relations), expectedRels)
	}

	// 按 Type 统计关系
	relCountByType := make(map[string]int)
	for _, r := range gm.Relations {
		relCountByType[r.Type]++
	}
	expectedRelsByType := map[string]int{
		"HAS_INTERFACE": 12,
		"CONNECTS_TO":   2,
		"RUNS_ON":       3,
		"ENDPOINT":      4,
	}
	for relType, expected := range expectedRelsByType {
		if relCountByType[relType] != expected {
			t.Errorf("Relations[%s] count = %d, want %d", relType, relCountByType[relType], expected)
		}
	}

	// === 验证无孤儿边 ===
	if len(warnings) != 0 {
		t.Errorf("warnings count = %d, want 0 (no orphans in mock_netbox)", len(warnings))
		for _, w := range warnings {
			t.Logf("  warning: %s - %s", w.Type, w.Detail)
		}
	}
}

// TestPipeline_MockNetbox_OrphanEdgeDetection 验证管线对孤儿边的检测能力:
// 构造部分数据（只有 Device 没有 Interface），验证 HAS_INTERFACE 被识别为孤儿边。
func TestPipeline_MockNetbox_OrphanEdgeDetection(t *testing.T) {
	reg := loadTestOntology(t)

	dataDir := filepath.Join("..", "..", "testdata", "mock_netbox")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Skipf("testdata/mock_netbox not found at %s, skipping", dataDir)
	}

	// 只加载 Device，不加载 Interface → 所有 HAS_INTERFACE 都是孤儿边
	entityTypes := []string{"Device"}
	gm, warnings := runPipeline(t, reg, dataDir, entityTypes)

	// 3 个 Device 节点
	if len(gm.Nodes) != 3 {
		t.Errorf("Nodes count = %d, want 3", len(gm.Nodes))
	}

	// CONNECTS_TO 有效 (指向 device:SN12345 和 device:SN12346，都在批次中)
	// HAS_INTERFACE 全部孤儿 (12 个 interface 引用都不在批次中)
	if len(gm.Relations) != 2 {
		t.Errorf("Relations count = %d, want 2 (only CONNECTS_TO valid)", len(gm.Relations))
	}

	// 12 个孤儿边警告 (所有 HAS_INTERFACE)
	if len(warnings) != 12 {
		t.Errorf("warnings count = %d, want 12 (all HAS_INTERFACE are orphans)", len(warnings))
	}

	// 验证所有警告都是 orphan_edge 类型
	for _, w := range warnings {
		if w.Type != "orphan_edge" {
			t.Errorf("unexpected warning type %q, want orphan_edge", w.Type)
		}
	}
}
