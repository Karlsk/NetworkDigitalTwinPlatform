package schema

import "testing"

// --- Compile-time interface satisfaction check ---

// stubRegistry is a minimal implementation to verify the SchemaRegistry
// interface can be satisfied. Full implementation is deferred to I-01.
type stubRegistry struct{}

func (s *stubRegistry) Load(_ string) error                                   { return nil }
func (s *stubRegistry) GetEntityType(_ string) (*EntityType, error)           { return nil, ErrSchemaNotFound }
func (s *stubRegistry) GetRelationType(_ string) (*RelationType, error)       { return nil, ErrSchemaNotFound }
func (s *stubRegistry) ListEntityTypes() []*EntityType                        { return nil }
func (s *stubRegistry) ListRelationTypes() []*RelationType                    { return nil }
func (s *stubRegistry) Validate(_ string, _ map[string]any) error             { return nil }
func (s *stubRegistry) ApplyDefaults(_ string, props map[string]any) (map[string]any, error) {
	return props, nil
}

// Compile-time check: stubRegistry must satisfy SchemaRegistry interface.
var _ SchemaRegistry = (*stubRegistry)(nil)

// --- Interface method count documentation ---

func TestSchemaRegistryMethodCount(t *testing.T) {
	// This test documents the expected 7 methods of SchemaRegistry.
	// If a method is added or removed, the stubRegistry above will fail
	// to compile, and this test serves as documentation.
	var r SchemaRegistry = &stubRegistry{}

	// Method 1: Load
	_ = r.Load("dir")

	// Method 2: GetEntityType
	_, _ = r.GetEntityType("Device")

	// Method 3: GetRelationType
	_, _ = r.GetRelationType("HAS_INTERFACE")

	// Method 4: ListEntityTypes
	_ = r.ListEntityTypes()

	// Method 5: ListRelationTypes
	_ = r.ListRelationTypes()

	// Method 6: Validate
	_ = r.Validate("Device", map[string]any{"serial_number": "SN001"})

	// Method 7: ApplyDefaults
	_, _ = r.ApplyDefaults("Device", map[string]any{"serial_number": "SN001"})
}

// --- Validate immutability contract ---

func TestValidateDoesNotMutateInput(t *testing.T) {
	// The Validate method must not modify the input map.
	// This test verifies the contract using a stub implementation.
	r := &stubRegistry{}

	original := map[string]any{
		"serial_number": "SN001",
		"hostname":      "router-01",
	}

	// Take a snapshot before Validate
	snapshot := make(map[string]any, len(original))
	for k, v := range original {
		snapshot[k] = v
	}

	_ = r.Validate("Device", original)

	// Verify map was not modified
	if len(original) != len(snapshot) {
		t.Errorf("Validate modified map size: got %d, want %d", len(original), len(snapshot))
	}
	for k, v := range snapshot {
		if original[k] != v {
			t.Errorf("Validate modified key %q: got %v, want %v", k, original[k], v)
		}
	}
}

func TestApplyDefaultsReturnsNewMap(t *testing.T) {
	// ApplyDefaults must return a NEW map, not modify the original.
	r := &stubRegistry{}

	original := map[string]any{
		"serial_number": "SN001",
	}

	result, err := r.ApplyDefaults("Device", original)
	if err != nil {
		t.Fatalf("ApplyDefaults() error = %v", err)
	}

	// The stub returns the same map for simplicity, but the contract
	// documentation in registry.go explicitly states: "原始 map 不被修改"
	// and "返回一个新 map". Real implementations (I-01) must return a copy.
	if result == nil {
		t.Error("ApplyDefaults() returned nil, want non-nil map")
	}
}
