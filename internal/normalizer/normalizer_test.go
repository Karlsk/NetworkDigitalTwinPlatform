package normalizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/connector"
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

// --- NormalizedResource struct tests ---

func TestNormalizedResourceFields(t *testing.T) {
	nr := NormalizedResource{
		Kind: "Device",
		URI:  "device:SN001",
		Properties: map[string]any{
			"hostname":   "router-01",
			"vendor":     "Huawei",
			"interfaces": []string{"iface:SN001_eth0", "iface:SN001_eth1"},
		},
	}

	if nr.Kind != "Device" {
		t.Errorf("Kind = %q, want %q", nr.Kind, "Device")
	}
	if nr.URI != "device:SN001" {
		t.Errorf("URI = %q, want %q", nr.URI, "device:SN001")
	}
	if len(nr.Properties) != 3 {
		t.Errorf("Properties count = %d, want 3", len(nr.Properties))
	}
	ifaces, ok := nr.Properties["interfaces"].([]string)
	if !ok {
		t.Fatalf("Properties[interfaces] is not []string")
	}
	if len(ifaces) != 2 {
		t.Errorf("interfaces count = %d, want 2", len(ifaces))
	}
}

func TestNormalizedResourceEmptyProperties(t *testing.T) {
	nr := NormalizedResource{Kind: "Interface", URI: "iface:test"}
	if nr.Properties != nil {
		t.Errorf("expected nil Properties for zero-value, got %v", nr.Properties)
	}
}

// --- fieldMapping tests ---

func TestNormalize_FieldMapping(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-0",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      "router-01",
			"mgmt_ip":       "10.0.0.1",
			"hw_model":      "NE40E",
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	// fieldMapping: mgmt_ip → management_ip
	if _, ok := nr.Properties["mgmt_ip"]; ok {
		t.Error("fieldMapping: source key mgmt_ip should be deleted")
	}
	if v, ok := nr.Properties["management_ip"]; !ok || v != "10.0.0.1" {
		t.Errorf("fieldMapping: management_ip = %v, want %q", v, "10.0.0.1")
	}

	// fieldMapping: hw_model → model
	if _, ok := nr.Properties["hw_model"]; ok {
		t.Error("fieldMapping: source key hw_model should be deleted")
	}
	if v, ok := nr.Properties["model"]; !ok || v != "NE40E" {
		t.Errorf("fieldMapping: model = %v, want %q", v, "NE40E")
	}
}

func TestNormalize_FieldMapping_SourceNotPresent(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-1",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      "router-01",
			"management_ip": "10.0.0.1", // already using standard name
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	// management_ip should remain, mgmt_ip should not appear
	if v, ok := nr.Properties["management_ip"]; !ok || v != "10.0.0.1" {
		t.Errorf("management_ip = %v, want %q", v, "10.0.0.1")
	}
	if _, ok := nr.Properties["mgmt_ip"]; ok {
		t.Error("mgmt_ip should not appear when source was not present")
	}
}

func TestNormalize_FieldMapping_BothPresent(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-2",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      "router-01",
			"mgmt_ip":       "10.0.0.99",
			"management_ip": "10.0.0.1",
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	// source key value should override destination
	if v := nr.Properties["management_ip"]; v != "10.0.0.99" {
		t.Errorf("management_ip = %v, want %q (source overrides destination)", v, "10.0.0.99")
	}
	if _, ok := nr.Properties["mgmt_ip"]; ok {
		t.Error("mgmt_ip should be deleted after fieldMapping")
	}
}

// --- normalize tests ---

func TestNormalize_NormalizeRule(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-3",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      "Router Core 01",
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if v := nr.Properties["hostname"]; v != "Router_Core_01" {
		t.Errorf("hostname = %v, want %q (spaces replaced with underscores)", v, "Router_Core_01")
	}
}

func TestNormalize_NormalizeRule_FieldMissing(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-4",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      "router-01", // no spaces, normalize still runs but no-op
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if v := nr.Properties["hostname"]; v != "router-01" {
		t.Errorf("hostname = %v, want %q", v, "router-01")
	}
}

func TestNormalize_NormalizeRule_NonStringField(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	// hostname is int instead of string — normalize rule should skip silently,
	// but Validate will catch the type mismatch
	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-5",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      12345,
		},
	}

	_, err := n.Normalize(res)
	if err == nil {
		t.Fatal("expected error for non-string hostname, got nil")
	}
}

// --- ApplyDefaults tests ---

func TestNormalize_DefaultsFilled(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-6",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      "router-01",
			// status not provided, should default to "Up"
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if v := nr.Properties["status"]; v != "Up" {
		t.Errorf("status = %v, want %q (default)", v, "Up")
	}
}

func TestNormalize_DefaultsPreserved(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-7",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      "router-01",
			"status":        "Down",
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if v := nr.Properties["status"]; v != "Down" {
		t.Errorf("status = %v, want %q (preserved)", v, "Down")
	}
}

// --- Validate tests ---

func TestNormalize_RequiredMissing(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-8",
		Properties: map[string]any{
			"serial_number": "SN12345",
			// hostname is required but missing
		},
	}

	_, err := n.Normalize(res)
	if err == nil {
		t.Fatal("expected error for missing required field hostname, got nil")
	}
}

func TestNormalize_EnumInvalid(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-9",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      "router-01",
			"status":        "InvalidStatus",
		},
	}

	_, err := n.Normalize(res)
	if err == nil {
		t.Fatal("expected error for invalid enum value, got nil")
	}
}

func TestNormalize_StableKeyEmpty(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-10",
		Properties: map[string]any{
			"serial_number": "", // stableKey is empty
			"hostname":      "router-01",
		},
	}

	_, err := n.Normalize(res)
	if err == nil {
		t.Fatal("expected error for empty stableKey, got nil")
	}
}

// --- URI tests ---

func TestNormalize_URI_SingleKey(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-11",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      "router-01",
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if nr.URI != "device:SN12345" {
		t.Errorf("URI = %q, want %q", nr.URI, "device:SN12345")
	}
}

func TestNormalize_URI_CompositeKeys(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Interface",
		ID:   "Interface-0",
		Properties: map[string]any{
			"device_serial": "SN001",
			"if_name":       "GE1/0/1",
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if nr.URI != "iface:SN001_GE1/0/1" {
		t.Errorf("URI = %q, want %q", nr.URI, "iface:SN001_GE1/0/1")
	}
}

func TestNormalize_URI_StableKeyMissingFromProps(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-12",
		Properties: map[string]any{
			// serial_number is required+stableKey, missing here
			"hostname": "router-01",
		},
	}

	_, err := n.Normalize(res)
	if err == nil {
		t.Fatal("expected error for missing stableKey, got nil")
	}
}

// --- Immutability test ---

func TestNormalize_DoesNotMutateOriginal(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	originalProps := map[string]any{
		"serial_number": "SN12345",
		"hostname":      "Router Core 01",
		"vendor":        "Huawei",
		"hw_model":      "NE40E",
		"mgmt_ip":       "10.0.0.1",
		"interfaces":    []string{"iface:SN12345_GE1/0/1"},
	}

	// snapshot original
	snapshot := make(map[string]any, len(originalProps))
	for k, v := range originalProps {
		snapshot[k] = v
	}

	res := connector.Resource{
		Kind:       "Device",
		ID:         "Device-13",
		Properties: originalProps,
	}

	_, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	// verify original map was not modified
	if len(originalProps) != len(snapshot) {
		t.Errorf("original props length changed: got %d, want %d", len(originalProps), len(snapshot))
	}
	for k, v := range snapshot {
		if fmt.Sprintf("%v", originalProps[k]) != fmt.Sprintf("%v", v) {
			t.Errorf("original props[%q] changed: got %v, want %v", k, originalProps[k], v)
		}
	}
}

// --- Schema not found test ---

func TestNormalize_UnknownKind(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind:       "UnknownKind",
		ID:         "X-0",
		Properties: map[string]any{"foo": "bar"},
	}

	_, err := n.Normalize(res)
	if err == nil {
		t.Fatal("expected error for unknown kind, got nil")
	}
	if !errors.Is(err, schema.ErrSchemaNotFound) {
		t.Errorf("error should wrap ErrSchemaNotFound, got: %v", err)
	}
}

// --- Relation fields preserved test ---

func TestNormalize_RelationFieldsPreserved(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	ifaces := []string{"iface:SN12345_GE1/0/1", "iface:SN12345_GE1/0/2"}
	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-14",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      "router-01",
			"interfaces":    ifaces,
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	v, ok := nr.Properties["interfaces"]
	if !ok {
		t.Fatal("relation field interfaces should be preserved in Properties")
	}
	gotIfaces, ok := v.([]string)
	if !ok {
		t.Fatalf("interfaces should be []string, got %T", v)
	}
	if len(gotIfaces) != 2 {
		t.Errorf("interfaces count = %d, want 2", len(gotIfaces))
	}
}

// --- End-to-end full pipeline tests ---

func TestNormalize_FullPipeline_Device(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Device",
		ID:   "Device-0",
		Properties: map[string]any{
			"serial_number": "SN12345",
			"hostname":      "Router Core 01",
			"vendor":        "Huawei",
			"hw_model":      "NE40E",
			"mgmt_ip":       "10.0.0.1",
			"chassis_mac":   "AA:BB:CC:01:02:03",
			"status":        "Up",
			"device_type":   "Core",
			"interfaces":    []string{"iface:SN12345_GE1/0/1"},
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	// Kind
	if nr.Kind != "Device" {
		t.Errorf("Kind = %q, want %q", nr.Kind, "Device")
	}

	// URI
	if nr.URI != "device:SN12345" {
		t.Errorf("URI = %q, want %q", nr.URI, "device:SN12345")
	}

	// fieldMapping: hw_model → model
	if _, ok := nr.Properties["hw_model"]; ok {
		t.Error("hw_model should be removed by fieldMapping")
	}
	if v := nr.Properties["model"]; v != "NE40E" {
		t.Errorf("model = %v, want %q", v, "NE40E")
	}

	// fieldMapping: mgmt_ip → management_ip
	if _, ok := nr.Properties["mgmt_ip"]; ok {
		t.Error("mgmt_ip should be removed by fieldMapping")
	}
	if v := nr.Properties["management_ip"]; v != "10.0.0.1" {
		t.Errorf("management_ip = %v, want %q", v, "10.0.0.1")
	}

	// normalize: hostname spaces → underscores
	if v := nr.Properties["hostname"]; v != "Router_Core_01" {
		t.Errorf("hostname = %v, want %q", v, "Router_Core_01")
	}

	// relation fields preserved
	if _, ok := nr.Properties["interfaces"]; !ok {
		t.Error("interfaces relation field should be preserved")
	}

	// unchanged fields
	if v := nr.Properties["vendor"]; v != "Huawei" {
		t.Errorf("vendor = %v, want %q", v, "Huawei")
	}
	if v := nr.Properties["chassis_mac"]; v != "AA:BB:CC:01:02:03" {
		t.Errorf("chassis_mac = %v, want %q", v, "AA:BB:CC:01:02:03")
	}
}

func TestNormalize_FullPipeline_Interface(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "Interface",
		ID:   "Interface-0",
		Properties: map[string]any{
			"device_serial": "SN001",
			"if_name":       "GE1/0/1",
			"bandwidth":     1000,
			// status missing → should default to "Up"
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if nr.Kind != "Interface" {
		t.Errorf("Kind = %q, want %q", nr.Kind, "Interface")
	}
	if nr.URI != "iface:SN001_GE1/0/1" {
		t.Errorf("URI = %q, want %q", nr.URI, "iface:SN001_GE1/0/1")
	}
	if v := nr.Properties["status"]; v != "Up" {
		t.Errorf("status = %v, want %q (default)", v, "Up")
	}
	if v := nr.Properties["bandwidth"]; v != 1000 {
		t.Errorf("bandwidth = %v, want %v", v, 1000)
	}
}

func TestNormalize_FullPipeline_ISIS(t *testing.T) {
	reg := loadTestOntology(t)
	n := NewNormalizer(reg)

	res := connector.Resource{
		Kind: "ISIS",
		ID:   "ISIS-0",
		Properties: map[string]any{
			"isis_id":   "isis-001",
			"system_id": "0000.0000.0001",
			"area_id":   "49.0001",
			// level and status missing → should get defaults
		},
	}

	nr, err := n.Normalize(res)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if nr.URI != "isis:isis-001" {
		t.Errorf("URI = %q, want %q", nr.URI, "isis:isis-001")
	}
	if v := nr.Properties["level"]; v != "L1L2" {
		t.Errorf("level = %v, want %q (default)", v, "L1L2")
	}
	if v := nr.Properties["status"]; v != "Active" {
		t.Errorf("status = %v, want %q (default)", v, "Active")
	}
	// relation field preserved (run_on not provided → not in props)
}
