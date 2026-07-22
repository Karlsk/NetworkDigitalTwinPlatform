package assembler

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/schema"
)

// loadTestOntology loads the real ontology/ directory for test reuse.
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

// findRelation 在 Relation 切片中查找匹配 type+from+to 的关系。
// map 遍历顺序不确定，测试不应依赖 Relations 的顺序。
func findRelation(rels []Relation, relType, from, to string) (Relation, bool) {
	for _, r := range rels {
		if r.Type == relType && r.From == from && r.To == to {
			return r, true
		}
	}
	return Relation{}, false
}

// findWarningDetail 检查警告切片中是否有包含指定子串的 Detail。
func findWarningDetail(warnings []ValidationWarning, substr string) bool {
	for _, w := range warnings {
		for i := 0; i <= len(w.Detail)-len(substr); i++ {
			if w.Detail[i:i+len(substr)] == substr {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------
// TC-A01: Device → Node 转换正确，Props 不含关系字段
// ---------------------------------------------------------------------

func TestAssemble_DeviceNodeConversion(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN12345",
			Properties: map[string]any{
				"serial_number":  "SN12345",
				"hostname":       "router-01",
				"vendor":         "Huawei",
				"interfaces":     []string{"iface:SN12345_GE1/0/1"},
				"upstream_links": []string{"device:SN002"},
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	if len(gm.Nodes) != 1 {
		t.Fatalf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	node := gm.Nodes[0]
	if node.MostSpecificLabel() != "Device" {
		t.Errorf("MostSpecificLabel() = %q, want %q", node.MostSpecificLabel(), "Device")
	}
	if node.URI != "device:SN12345" {
		t.Errorf("URI = %q, want %q", node.URI, "device:SN12345")
	}

	// 普通属性保留
	if v := node.Props["serial_number"]; v != "SN12345" {
		t.Errorf("Props[serial_number] = %v, want %q", v, "SN12345")
	}
	if v := node.Props["hostname"]; v != "router-01" {
		t.Errorf("Props[hostname] = %v, want %q", v, "router-01")
	}
	if v := node.Props["vendor"]; v != "Huawei" {
		t.Errorf("Props[vendor] = %v, want %q", v, "Huawei")
	}

	// 关系字段被过滤
	if _, ok := node.Props["interfaces"]; ok {
		t.Error("Props should not contain relation field 'interfaces'")
	}
	if _, ok := node.Props["upstream_links"]; ok {
		t.Error("Props should not contain relation field 'upstream_links'")
	}
}

// ---------------------------------------------------------------------
// TC-A02: Interface → Node 转换正确（无 relationFields 的实体不过滤）
// ---------------------------------------------------------------------

func TestAssemble_InterfaceNodeConversion(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
				"status":        "Up",
				"bandwidth":     1000,
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	if len(gm.Nodes) != 1 {
		t.Fatalf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	node := gm.Nodes[0]
	if node.MostSpecificLabel() != "Interface" {
		t.Errorf("MostSpecificLabel() = %q, want %q", node.MostSpecificLabel(), "Interface")
	}
	if node.URI != "iface:SN001_GE1/0/1" {
		t.Errorf("URI = %q, want %q", node.URI, "iface:SN001_GE1/0/1")
	}

	// 所有属性保留（Interface 无 relationFields）
	if len(node.Props) != 4 {
		t.Errorf("Props count = %d, want 4 (no filtering for Interface)", len(node.Props))
	}
	for _, key := range []string{"device_serial", "if_name", "status", "bandwidth"} {
		if _, ok := node.Props[key]; !ok {
			t.Errorf("Props should contain %q", key)
		}
	}

	// 无关系
	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0", len(gm.Relations))
	}
}

// ---------------------------------------------------------------------
// TC-A03: relationFields 正确推导为 Relation（4 种关系类型）
// ---------------------------------------------------------------------

func TestAssemble_RelationDerivation_HAS_INTERFACE(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number": "SN001",
				"hostname":      "router-01",
				"interfaces":    []string{"iface:SN001_GE1/0/1"},
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	rel, ok := findRelation(gm.Relations, "HAS_INTERFACE", "device:SN001", "iface:SN001_GE1/0/1")
	if !ok {
		t.Fatal("expected HAS_INTERFACE relation from device:SN001 to iface:SN001_GE1/0/1")
	}
	if rel.Type != "HAS_INTERFACE" {
		t.Errorf("Type = %q, want %q", rel.Type, "HAS_INTERFACE")
	}
}

func TestAssemble_RelationDerivation_CONNECTS_TO(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number":  "SN001",
				"hostname":       "router-01",
				"upstream_links": []string{"device:SN002"},
			},
		},
		{
			Kind: "Device",
			URI:  "device:SN002",
			Properties: map[string]any{
				"serial_number": "SN002",
				"hostname":      "router-02",
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	_, ok := findRelation(gm.Relations, "CONNECTS_TO", "device:SN001", "device:SN002")
	if !ok {
		t.Fatal("expected CONNECTS_TO relation from device:SN001 to device:SN002")
	}
}

func TestAssemble_RelationDerivation_RUNS_ON(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "ISIS",
			URI:  "isis:isis-001",
			Properties: map[string]any{
				"isis_id":   "isis-001",
				"system_id": "0000.0000.0001",
				"run_on":    []string{"iface:SN001_GE1/0/1"},
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	_, ok := findRelation(gm.Relations, "RUNS_ON", "isis:isis-001", "iface:SN001_GE1/0/1")
	if !ok {
		t.Fatal("expected RUNS_ON relation from isis:isis-001 to iface:SN001_GE1/0/1")
	}
}

func TestAssemble_RelationDerivation_ENDPOINT(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Link",
			URI:  "link:LNK001",
			Properties: map[string]any{
				"link_id":   "LNK001",
				"endpoints": []string{"iface:SN001_GE1/0/1", "iface:SN002_GE1/0/1"},
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN002_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN002",
				"if_name":       "GE1/0/1",
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	_, ok1 := findRelation(gm.Relations, "ENDPOINT", "link:LNK001", "iface:SN001_GE1/0/1")
	if !ok1 {
		t.Error("expected ENDPOINT relation from link:LNK001 to iface:SN001_GE1/0/1")
	}
	_, ok2 := findRelation(gm.Relations, "ENDPOINT", "link:LNK001", "iface:SN002_GE1/0/1")
	if !ok2 {
		t.Error("expected ENDPOINT relation from link:LNK001 to iface:SN002_GE1/0/1")
	}
}

func TestAssemble_RelationDerivation_OCCURRED_ON(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Alarm",
			URI:  "alarm:ALM001",
			Properties: map[string]any{
				"alarm_id":    "ALM001",
				"occurred_on": []string{"iface:SN001_GE1/0/1"},
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	_, ok := findRelation(gm.Relations, "OCCURRED_ON", "alarm:ALM001", "iface:SN001_GE1/0/1")
	if !ok {
		t.Fatal("expected OCCURRED_ON relation from alarm:ALM001 to iface:SN001_GE1/0/1")
	}
}

// ---------------------------------------------------------------------
// TC-A04: RelationType 的 source 类型校验生效
// 使用 mock SchemaRegistry，其中 Device 拥有 run_on -> RUNS_ON 关系字段，
// 但 RUNS_ON 的 source 只允许 [ISIS]，Device 不在其中。
// ---------------------------------------------------------------------

// mockRegistry 是轻量级 SchemaRegistry mock，仅实现 Assemble 需要的方法。
type mockRegistry struct {
	entityTypes   map[string]*schema.EntityType
	relationTypes map[string]*schema.RelationType
}

func (m *mockRegistry) Load(_ string) error                       { return nil }
func (m *mockRegistry) ListEntityTypes() []*schema.EntityType     { return nil }
func (m *mockRegistry) ListRelationTypes() []*schema.RelationType { return nil }
func (m *mockRegistry) Validate(_ string, _ map[string]any) error { return nil }
func (m *mockRegistry) ApplyDefaults(_ string, p map[string]any) (map[string]any, error) {
	return p, nil
}
func (m *mockRegistry) GetLabels(kind string) []string { return []string{kind} }

func (m *mockRegistry) GetEntityType(name string) (*schema.EntityType, error) {
	et, ok := m.entityTypes[name]
	if !ok {
		return nil, schema.ErrSchemaNotFound
	}
	return et, nil
}

func (m *mockRegistry) GetRelationType(name string) (*schema.RelationType, error) {
	rt, ok := m.relationTypes[name]
	if !ok {
		return nil, schema.ErrSchemaNotFound
	}
	return rt, nil
}

func TestAssemble_SourceTypeMismatch(t *testing.T) {
	// Device 的 relationFields 包含 run_on -> RUNS_ON，
	// 但 RUNS_ON 的 source 只允许 [ISIS]，Device 不在其中。
	reg := &mockRegistry{
		entityTypes: map[string]*schema.EntityType{
			"Device": {
				Metadata: schema.Metadata{Name: "Device"},
				Spec: schema.EntityTypeSpec{
					RelationFields: map[string]schema.RelationFieldSpec{
						"run_on": {RelationType: "RUNS_ON"},
					},
				},
			},
		},
		relationTypes: map[string]*schema.RelationType{
			"RUNS_ON": {
				Metadata: schema.Metadata{Name: "RUNS_ON"},
				Spec: schema.RelationTypeSpec{
					Source: []string{"ISIS"},
					Target: []string{"Interface"},
				},
			},
		},
	}

	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"run_on": []string{"iface:SN001_GE1/0/1"},
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 节点正常生成
	if len(gm.Nodes) != 1 {
		t.Errorf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	// source 类型不匹配，关系不被推导
	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0 (source type mismatch should skip)", len(gm.Relations))
	}
}

// ---------------------------------------------------------------------
// TC-A05: 孤儿边被跳过，返回 ValidationWarning
// ---------------------------------------------------------------------

func TestAssemble_OrphanEdge(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN12345",
			Properties: map[string]any{
				"serial_number": "SN12345",
				"hostname":      "router-01",
				// 目标 Interface 不存在于批次中
				"interfaces": []string{"iface:SN12345_GE1/0/2"},
			},
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 节点正常创建
	if len(gm.Nodes) != 1 {
		t.Errorf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	// 孤儿边被跳过
	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0 (orphan edge skipped)", len(gm.Relations))
	}

	// 警告
	if len(warnings) != 1 {
		t.Fatalf("warnings count = %d, want 1", len(warnings))
	}
	if warnings[0].Type != "orphan_edge" {
		t.Errorf("warning Type = %q, want %q", warnings[0].Type, "orphan_edge")
	}
	if warnings[0].Detail == "" {
		t.Error("warning Detail should not be empty")
	}
}

func TestAssemble_OrphanEdge_MixedValidAndOrphan(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	// Device 引用 3 个 Interface，只有 1 个存在
	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number": "SN001",
				"hostname":      "router-01",
				"interfaces": []string{
					"iface:SN001_GE1/0/1", // 存在
					"iface:SN001_GE1/0/2", // 孤儿
					"iface:SN001_GE1/0/3", // 孤儿
				},
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
			},
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 2 个节点
	if len(gm.Nodes) != 2 {
		t.Errorf("Nodes count = %d, want 2", len(gm.Nodes))
	}

	// 1 个有效关系
	if len(gm.Relations) != 1 {
		t.Errorf("Relations count = %d, want 1", len(gm.Relations))
	}

	// 2 个孤儿边警告
	if len(warnings) != 2 {
		t.Errorf("warnings count = %d, want 2", len(warnings))
	}
}

// ---------------------------------------------------------------------
// TC-A06: 多类型混合批量处理（Device + Interface + ISIS 同时传入）
// ---------------------------------------------------------------------

func TestAssemble_MixedBatch(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number":  "SN001",
				"hostname":       "router-01",
				"interfaces":     []string{"iface:SN001_GE1/0/1"},
				"upstream_links": []string{"device:SN002"},
			},
		},
		{
			Kind: "Device",
			URI:  "device:SN002",
			Properties: map[string]any{
				"serial_number": "SN002",
				"hostname":      "router-02",
				"interfaces":    []string{"iface:SN002_GE1/0/1"},
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN002_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN002",
				"if_name":       "GE1/0/1",
			},
		},
		{
			Kind: "ISIS",
			URI:  "isis:isis-001",
			Properties: map[string]any{
				"isis_id":   "isis-001",
				"system_id": "0000.0000.0001",
				"run_on":    []string{"iface:SN001_GE1/0/1"},
			},
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 5 个节点
	if len(gm.Nodes) != 5 {
		t.Errorf("Nodes count = %d, want 5", len(gm.Nodes))
	}

	// 4 个关系
	expectedRels := []struct {
		relType, from, to string
	}{
		{"HAS_INTERFACE", "device:SN001", "iface:SN001_GE1/0/1"},
		{"HAS_INTERFACE", "device:SN002", "iface:SN002_GE1/0/1"},
		{"CONNECTS_TO", "device:SN001", "device:SN002"},
		{"RUNS_ON", "isis:isis-001", "iface:SN001_GE1/0/1"},
	}

	if len(gm.Relations) != len(expectedRels) {
		t.Fatalf("Relations count = %d, want %d", len(gm.Relations), len(expectedRels))
	}

	for _, expected := range expectedRels {
		if _, ok := findRelation(gm.Relations, expected.relType, expected.from, expected.to); !ok {
			t.Errorf("missing relation %s: %s -> %s", expected.relType, expected.from, expected.to)
		}
	}

	// 无孤儿边
	if len(warnings) != 0 {
		t.Errorf("warnings count = %d, want 0", len(warnings))
	}
}

// ---------------------------------------------------------------------
// TC-A07: 空 relationFields 的实体只生成节点，无关系
// ---------------------------------------------------------------------

func TestAssemble_EmptyRelationFields(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
				"status":        "Up",
			},
		},
		{
			Kind: "Network_Slice",
			URI:  "slice:SL001",
			Properties: map[string]any{
				"slice_id":      "SL001",
				"name":          "slice-1",
				"sla_bandwidth": 1000,
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 2 个节点
	if len(gm.Nodes) != 2 {
		t.Errorf("Nodes count = %d, want 2", len(gm.Nodes))
	}

	// 无关系
	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0", len(gm.Relations))
	}

	// 所有属性保留（无过滤）
	if len(gm.Nodes[0].Props) != 3 {
		t.Errorf("Interface Props count = %d, want 3", len(gm.Nodes[0].Props))
	}
	if len(gm.Nodes[1].Props) != 3 {
		t.Errorf("Network_Slice Props count = %d, want 3", len(gm.Nodes[1].Props))
	}
}

// ---------------------------------------------------------------------
// 边界测试
// ---------------------------------------------------------------------

func TestAssemble_EmptyInput(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	gm, warnings, err := a.Assemble([]normalizer.NormalizedResource{})
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	if len(gm.Nodes) != 0 {
		t.Errorf("Nodes count = %d, want 0", len(gm.Nodes))
	}
	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0", len(gm.Relations))
	}
	if len(warnings) != 0 {
		t.Errorf("warnings count = %d, want 0", len(warnings))
	}
}

func TestAssemble_NilInput(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	gm, warnings, err := a.Assemble(nil)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	if len(gm.Nodes) != 0 {
		t.Errorf("Nodes count = %d, want 0", len(gm.Nodes))
	}
	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0", len(gm.Relations))
	}
	if len(warnings) != 0 {
		t.Errorf("warnings count = %d, want 0", len(warnings))
	}
}

func TestAssemble_UnknownKind(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind:       "UnknownType",
			URI:        "unknown:001",
			Properties: map[string]any{"foo": "bar"},
		},
	}

	_, _, err := a.Assemble(resources)
	if err == nil {
		t.Fatal("expected error for unknown kind, got nil")
	}
	if !errors.Is(err, schema.ErrSchemaNotFound) {
		t.Errorf("error should wrap ErrSchemaNotFound, got: %v", err)
	}
}

func TestAssemble_RelationFieldNotStringSlice(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number": "SN001",
				"hostname":      "router-01",
				// interfaces 是字符串而非切片，应该被跳过
				"interfaces": "not-a-slice",
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 节点正常生成
	if len(gm.Nodes) != 1 {
		t.Errorf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	// 关系不被推导（类型不匹配）
	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0 (non-slice field skipped)", len(gm.Relations))
	}
}

func TestAssemble_RelationFieldEmptySlice(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number": "SN001",
				"hostname":      "router-01",
				"interfaces":    []string{}, // 空切片
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0 (empty slice)", len(gm.Relations))
	}
}

func TestAssemble_RelationFieldMissingFromProperties(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	// Device 有 relationFields 定义，但 Properties 中没有提供 interfaces 字段
	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number": "SN001",
				"hostname":      "router-01",
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0 (relation field not in properties)", len(gm.Relations))
	}
}

// ---------------------------------------------------------------------
// 辅助函数单元测试
// ---------------------------------------------------------------------

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		want   []string
		wantOK bool
	}{
		{
			name:   "[]string",
			input:  []string{"a", "b"},
			want:   []string{"a", "b"},
			wantOK: true,
		},
		{
			name:   "[]any with strings",
			input:  []any{"a", "b"},
			want:   []string{"a", "b"},
			wantOK: true,
		},
		{
			name:   "[]any with mixed types",
			input:  []any{"a", 123, "b"},
			want:   []string{"a", "b"},
			wantOK: true,
		},
		{
			name:   "empty []any",
			input:  []any{},
			want:   []string{},
			wantOK: true,
		},
		{
			name:   "string (not a slice)",
			input:  "not-a-slice",
			want:   nil,
			wantOK: false,
		},
		{
			name:   "int (not a slice)",
			input:  42,
			want:   nil,
			wantOK: false,
		},
		{
			name:   "nil",
			input:  nil,
			want:   nil,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toStringSlice(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if len(got) != len(tt.want) {
				t.Errorf("len = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestContainsStr(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		item  string
		want  bool
	}{
		{
			name:  "found",
			slice: []string{"Device", "Interface"},
			item:  "Device",
			want:  true,
		},
		{
			name:  "not found",
			slice: []string{"Device", "Interface"},
			item:  "ISIS",
			want:  false,
		},
		{
			name:  "empty slice",
			slice: []string{},
			item:  "Device",
			want:  false,
		},
		{
			name:  "nil slice",
			slice: nil,
			item:  "Device",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsStr(tt.slice, tt.item)
			if got != tt.want {
				t.Errorf("containsStr(%v, %q) = %v, want %v", tt.slice, tt.item, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// P0 边界测试: 孤儿边扩展场景
// ---------------------------------------------------------------------

// TestAssemble_OrphanEdge_CONNECTS_TO 测试 CONNECTS_TO 孤儿场景:
// Device 的 upstream_links 引用不存在的 Device URI。
func TestAssemble_OrphanEdge_CONNECTS_TO(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number":  "SN001",
				"hostname":       "router-01",
				"upstream_links": []string{"device:SN999"}, // 目标不存在
			},
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 节点正常创建
	if len(gm.Nodes) != 1 {
		t.Errorf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	// CONNECTS_TO 孤儿边被跳过
	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0 (CONNECTS_TO orphan skipped)", len(gm.Relations))
	}

	// 1 个孤儿边警告
	if len(warnings) != 1 {
		t.Fatalf("warnings count = %d, want 1", len(warnings))
	}
	if warnings[0].Type != "orphan_edge" {
		t.Errorf("warning Type = %q, want %q", warnings[0].Type, "orphan_edge")
	}
	if !findWarningDetail(warnings, "CONNECTS_TO") {
		t.Errorf("warning Detail should contain CONNECTS_TO, got %q", warnings[0].Detail)
	}
}

// TestAssemble_OrphanEdge_MultiRelationTypePartialOrphan 测试同一节点多关系类型部分孤儿:
// Device 的 HAS_INTERFACE 有效 + CONNECTS_TO 孤儿。
func TestAssemble_OrphanEdge_MultiRelationTypePartialOrphan(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number":  "SN001",
				"hostname":       "router-01",
				"interfaces":     []string{"iface:SN001_GE1/0/1"}, // 有效
				"upstream_links": []string{"device:SN999"},        // 孤儿
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
			},
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 2 个节点
	if len(gm.Nodes) != 2 {
		t.Errorf("Nodes count = %d, want 2", len(gm.Nodes))
	}

	// 1 个有效关系 (HAS_INTERFACE)
	if len(gm.Relations) != 1 {
		t.Errorf("Relations count = %d, want 1 (only HAS_INTERFACE valid)", len(gm.Relations))
	}
	if len(gm.Relations) > 0 && gm.Relations[0].Type != "HAS_INTERFACE" {
		t.Errorf("Relation Type = %q, want HAS_INTERFACE", gm.Relations[0].Type)
	}

	// 1 个孤儿边警告 (CONNECTS_TO)
	if len(warnings) != 1 {
		t.Fatalf("warnings count = %d, want 1", len(warnings))
	}
	if !findWarningDetail(warnings, "CONNECTS_TO") {
		t.Errorf("warning should be about CONNECTS_TO, got %q", warnings[0].Detail)
	}
}

// TestAssemble_SelfReferencingRelation 测试自引用关系:
// Device CONNECTS_TO 自身，不应被误判为孤儿。
func TestAssemble_SelfReferencingRelation(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number":  "SN001",
				"hostname":       "router-01",
				"upstream_links": []string{"device:SN001"}, // 指向自身
			},
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 1 个节点
	if len(gm.Nodes) != 1 {
		t.Errorf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	// 1 个关系 (自引用不是孤儿)
	if len(gm.Relations) != 1 {
		t.Errorf("Relations count = %d, want 1 (self-ref is not orphan)", len(gm.Relations))
	}

	// 无孤儿边警告
	if len(warnings) != 0 {
		t.Errorf("warnings count = %d, want 0 (self-ref should not trigger orphan)", len(warnings))
	}
}

// TestAssemble_DuplicateTargetURI 测试重复目标 URI:
// interfaces: ["iface:X", "iface:X"] 应生成 2 个重复的 Relation。
func TestAssemble_DuplicateTargetURI(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number": "SN001",
				"hostname":      "router-01",
				"interfaces":    []string{"iface:SN001_GE1/0/1", "iface:SN001_GE1/0/1"}, // 重复
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
			},
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 2 个节点
	if len(gm.Nodes) != 2 {
		t.Errorf("Nodes count = %d, want 2", len(gm.Nodes))
	}

	// 当前行为: 生成 2 个重复的 Relation (文档化此行为)
	if len(gm.Relations) != 2 {
		t.Errorf("Relations count = %d, want 2 (duplicate targets produce duplicate relations)", len(gm.Relations))
	}

	// 无孤儿边警告
	if len(warnings) != 0 {
		t.Errorf("warnings count = %d, want 0", len(warnings))
	}
}

// TestAssemble_OrphanEdge_EmptyStringTarget 测试空字符串目标 URI:
// interfaces: [""] 应被识别为孤儿边。
func TestAssemble_OrphanEdge_EmptyStringTarget(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number": "SN001",
				"hostname":      "router-01",
				"interfaces":    []string{""}, // 空字符串目标
			},
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 节点正常创建
	if len(gm.Nodes) != 1 {
		t.Errorf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	// 空字符串目标不在 uriIndex 中，被识别为孤儿
	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0 (empty string target is orphan)", len(gm.Relations))
	}

	// 1 个孤儿边警告
	if len(warnings) != 1 {
		t.Fatalf("warnings count = %d, want 1", len(warnings))
	}
	if warnings[0].Type != "orphan_edge" {
		t.Errorf("warning Type = %q, want %q", warnings[0].Type, "orphan_edge")
	}
}

// ---------------------------------------------------------------------
// P1 防御性测试: 孤儿边扩展场景
// ---------------------------------------------------------------------

// TestAssemble_OrphanEdge_AnySliceWithEmptyString 测试 []any 中混入空字符串:
// []any{"iface:X", "", "iface:Y"}，空字符串应被识别为孤儿。
func TestAssemble_OrphanEdge_AnySliceWithEmptyString(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number": "SN001",
				"hostname":      "router-01",
				// []any 类型，混入空字符串
				"interfaces": []any{"iface:SN001_GE1/0/1", "", "iface:SN001_GE1/0/2"},
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/2",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/2",
			},
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 3 个节点
	if len(gm.Nodes) != 3 {
		t.Errorf("Nodes count = %d, want 3", len(gm.Nodes))
	}

	// 2 个有效关系 (空字符串被过滤)
	if len(gm.Relations) != 2 {
		t.Errorf("Relations count = %d, want 2", len(gm.Relations))
	}

	// 1 个孤儿边警告 (空字符串)
	if len(warnings) != 1 {
		t.Fatalf("warnings count = %d, want 1", len(warnings))
	}
	if warnings[0].Type != "orphan_edge" {
		t.Errorf("warning Type = %q, want %q", warnings[0].Type, "orphan_edge")
	}
}

// TestAssemble_OrphanEdge_CrossEntityChain 测试跨实体类型混合孤儿检测:
// Device+ISIS+Link 多实体类型同时出现在一个批次中。
func TestAssemble_OrphanEdge_CrossEntityChain(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number":  "SN001",
				"hostname":       "router-01",
				"interfaces":     []string{"iface:SN001_GE1/0/1"}, // 有效
				"upstream_links": []string{"device:SN999"},        // 孤儿
			},
		},
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
			},
		},
		{
			Kind: "ISIS",
			URI:  "isis:isis-001",
			Properties: map[string]any{
				"isis_id":   "isis-001",
				"system_id": "0000.0000.0001",
				"run_on":    []string{"iface:SN999_GE1/0/1"}, // 孤儿
			},
		},
		{
			Kind: "Link",
			URI:  "link:LNK001",
			Properties: map[string]any{
				"link_id":   "LNK001",
				"endpoints": []string{"iface:SN001_GE1/0/1", "iface:SN999_GE1/0/2"}, // 1 有效 + 1 孤儿
			},
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 4 个节点
	if len(gm.Nodes) != 4 {
		t.Errorf("Nodes count = %d, want 4", len(gm.Nodes))
	}

	// 2 个有效关系: HAS_INTERFACE + ENDPOINT(iface:SN001_GE1/0/1)
	if len(gm.Relations) != 2 {
		t.Errorf("Relations count = %d, want 2", len(gm.Relations))
	}

	// 3 个孤儿边警告: CONNECTS_TO + RUNS_ON + ENDPOINT(iface:SN999_GE1/0/2)
	if len(warnings) != 3 {
		t.Errorf("warnings count = %d, want 3", len(warnings))
	}
}

// TestAssemble_OrphanEdge_AllOrphans 测试所有关系都是孤儿的场景。
func TestAssemble_OrphanEdge_AllOrphans(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number":  "SN001",
				"hostname":       "router-01",
				"interfaces":     []string{"iface:SN001_GE1/0/1", "iface:SN001_GE1/0/2"}, // 全部孤儿
				"upstream_links": []string{"device:SN888", "device:SN999"},               // 全部孤儿
			},
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 1 个节点
	if len(gm.Nodes) != 1 {
		t.Errorf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	// 无有效关系
	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0 (all orphans)", len(gm.Relations))
	}

	// 4 个孤儿边警告
	if len(warnings) != 4 {
		t.Errorf("warnings count = %d, want 4", len(warnings))
	}
}

// TestAssemble_NilProperties 测试 Properties 为 nil 的场景，应不 panic。
func TestAssemble_NilProperties(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind:       "Device",
			URI:        "device:SN001",
			Properties: nil, // nil Properties
		},
	}

	gm, warnings, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	// 1 个节点
	if len(gm.Nodes) != 1 {
		t.Errorf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	// 无关系 (nil Properties 无法推导关系)
	if len(gm.Relations) != 0 {
		t.Errorf("Relations count = %d, want 0", len(gm.Relations))
	}

	// 无警告
	if len(warnings) != 0 {
		t.Errorf("warnings count = %d, want 0", len(warnings))
	}
}

// ---------------------------------------------------------------------
// V1-16: 多标签节点测试
// ---------------------------------------------------------------------

// TestAssemble_MultiLabel_Device 验证 Device 节点的 Labels 包含完整继承链。
func TestAssemble_MultiLabel_Device(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Device",
			URI:  "device:SN001",
			Properties: map[string]any{
				"serial_number": "SN001",
				"hostname":      "router-01",
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	if len(gm.Nodes) != 1 {
		t.Fatalf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	node := gm.Nodes[0]
	if len(node.Labels) != 2 {
		t.Fatalf("Labels len = %d, want 2", len(node.Labels))
	}
	if node.Labels[0] != "Resource" || node.Labels[1] != "Device" {
		t.Errorf("Labels = %v, want [Resource Device]", node.Labels)
	}
	if node.MostSpecificLabel() != "Device" {
		t.Errorf("MostSpecificLabel = %q, want Device", node.MostSpecificLabel())
	}
}

// TestAssemble_MultiLabel_Interface 验证 Interface 节点的 Labels 包含完整继承链。
func TestAssemble_MultiLabel_Interface(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Interface",
			URI:  "iface:SN001_GE1/0/1",
			Properties: map[string]any{
				"device_serial": "SN001",
				"if_name":       "GE1/0/1",
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	if len(gm.Nodes) != 1 {
		t.Fatalf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	node := gm.Nodes[0]
	if len(node.Labels) != 2 {
		t.Fatalf("Labels len = %d, want 2", len(node.Labels))
	}
	if node.Labels[0] != "Resource" || node.Labels[1] != "Interface" {
		t.Errorf("Labels = %v, want [Resource Interface]", node.Labels)
	}
}

// TestAssemble_MultiLabel_NetworkSlice 验证 Network_Slice 节点的 Labels 包含 Service 基类。
func TestAssemble_MultiLabel_NetworkSlice(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "Network_Slice",
			URI:  "slice:SL001",
			Properties: map[string]any{
				"slice_id":      "SL001",
				"name":          "slice-1",
				"sla_bandwidth": 1000,
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	if len(gm.Nodes) != 1 {
		t.Fatalf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	node := gm.Nodes[0]
	if len(node.Labels) != 2 {
		t.Fatalf("Labels len = %d, want 2", len(node.Labels))
	}
	if node.Labels[0] != "Service" || node.Labels[1] != "Network_Slice" {
		t.Errorf("Labels = %v, want [Service Network_Slice]", node.Labels)
	}
}

// TestAssemble_SingleLabel_NoExtends 验证无继承的实体仍为单标签（向后兼容）。
func TestAssemble_SingleLabel_NoExtends(t *testing.T) {
	reg := loadTestOntology(t)
	a := NewGraphAssembler(reg)

	resources := []normalizer.NormalizedResource{
		{
			Kind: "BGP",
			URI:  "bgp:BGP001",
			Properties: map[string]any{
				"bgp_id":    "BGP001",
				"as_number": 65001,
			},
		},
	}

	gm, _, err := a.Assemble(resources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	if len(gm.Nodes) != 1 {
		t.Fatalf("Nodes count = %d, want 1", len(gm.Nodes))
	}

	node := gm.Nodes[0]
	if len(node.Labels) != 1 || node.Labels[0] != "BGP" {
		t.Errorf("Labels = %v, want [BGP]", node.Labels)
	}
}
