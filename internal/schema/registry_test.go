package schema

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// --- Compile-time interface satisfaction check ---

// stubRegistry is a minimal implementation to verify the SchemaRegistry
// interface can be satisfied. Full implementation is deferred to I-01.
type stubRegistry struct{}

func (s *stubRegistry) Load(_ string) error                              { return nil }
func (s *stubRegistry) GetEntityType(_ string) (*EntityType, error)      { return nil, ErrSchemaNotFound }
func (s *stubRegistry) GetRelationType(_ string) (*RelationType, error)  { return nil, ErrSchemaNotFound }
func (s *stubRegistry) ListEntityTypes() []*EntityType                   { return nil }
func (s *stubRegistry) ListRelationTypes() []*RelationType               { return nil }
func (s *stubRegistry) Validate(_ string, _ map[string]any) error        { return nil }
func (s *stubRegistry) ApplyDefaults(_ string, props map[string]any) (map[string]any, error) {
	return props, nil
}

// Compile-time check: stubRegistry must satisfy SchemaRegistry interface.
var _ SchemaRegistry = (*stubRegistry)(nil)

// --- Interface method count documentation ---

func TestSchemaRegistryMethodCount(t *testing.T) {
	var r SchemaRegistry = &stubRegistry{}
	_ = r.Load("dir")
	_, _ = r.GetEntityType("Device")
	_, _ = r.GetRelationType("HAS_INTERFACE")
	_ = r.ListEntityTypes()
	_ = r.ListRelationTypes()
	_ = r.Validate("Device", map[string]any{"serial_number": "SN001"})
	_, _ = r.ApplyDefaults("Device", map[string]any{"serial_number": "SN001"})
}

// --- Validate immutability contract ---

func TestValidateDoesNotMutateInput(t *testing.T) {
	r := &stubRegistry{}
	original := map[string]any{
		"serial_number": "SN001",
		"hostname":      "router-01",
	}
	snapshot := make(map[string]any, len(original))
	for k, v := range original {
		snapshot[k] = v
	}
	_ = r.Validate("Device", original)
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
	r := &stubRegistry{}
	original := map[string]any{"serial_number": "SN001"}
	result, err := r.ApplyDefaults("Device", original)
	if err != nil {
		t.Fatalf("ApplyDefaults() error = %v", err)
	}
	if result == nil {
		t.Error("ApplyDefaults() returned nil, want non-nil map")
	}
}

// ============================================================
// registryImpl tests (Load / Get / List / CrossValidate)
// ============================================================

// loadTestOntology loads the real ontology/ directory for test reuse.
func loadTestOntology(t *testing.T) SchemaRegistry {
	t.Helper()
	ontologyDir := filepath.Join("..", "..", "ontology")
	if _, err := os.Stat(ontologyDir); os.IsNotExist(err) {
		t.Skipf("ontology directory not found at %s, skipping", ontologyDir)
	}
	r := NewSchemaRegistry()
	if err := r.Load(ontologyDir); err != nil {
		t.Fatalf("Load(%q) error = %v", ontologyDir, err)
	}
	return r
}

func TestNewSchemaRegistry(t *testing.T) {
	r := NewSchemaRegistry()
	if r == nil {
		t.Fatal("NewSchemaRegistry() returned nil")
	}
	if got := len(r.ListEntityTypes()); got != 0 {
		t.Errorf("initial ListEntityTypes() len = %d, want 0", got)
	}
	if got := len(r.ListRelationTypes()); got != 0 {
		t.Errorf("initial ListRelationTypes() len = %d, want 0", got)
	}
}

func TestLoad_ValidOntology(t *testing.T) {
	r := loadTestOntology(t)
	if got := len(r.ListEntityTypes()); got != 6 {
		t.Errorf("ListEntityTypes() len = %d, want 6", got)
	}
	if got := len(r.ListRelationTypes()); got != 5 {
		t.Errorf("ListRelationTypes() len = %d, want 5", got)
	}
}

func TestLoad_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	r := NewSchemaRegistry()
	if err := r.Load(dir); err == nil {
		t.Fatal("Load(empty dir) expected error, got nil")
	}
}

func TestLoad_InvalidDir(t *testing.T) {
	r := NewSchemaRegistry()
	if err := r.Load("/nonexistent/path/to/ontology"); err == nil {
		t.Fatal("Load(invalid dir) expected error, got nil")
	}
}

func TestGetEntityType_Found(t *testing.T) {
	r := loadTestOntology(t)
	et, err := r.GetEntityType("Device")
	if err != nil {
		t.Fatalf("GetEntityType(Device) error = %v", err)
	}
	if et == nil {
		t.Fatal("GetEntityType(Device) returned nil")
	}
	if et.Metadata.Name != "Device" {
		t.Errorf("Metadata.Name = %q, want %q", et.Metadata.Name, "Device")
	}
	if et.Spec.URITemplate != "device:{serial_number}" {
		t.Errorf("URITemplate = %q, want %q", et.Spec.URITemplate, "device:{serial_number}")
	}
	if len(et.Spec.Identity.StableKeys) != 1 || et.Spec.Identity.StableKeys[0] != "serial_number" {
		t.Errorf("StableKeys = %v, want [serial_number]", et.Spec.Identity.StableKeys)
	}
	if len(et.Spec.Properties) != 8 {
		t.Errorf("Properties count = %d, want 8", len(et.Spec.Properties))
	}
	if len(et.Spec.RelationFields) != 2 {
		t.Errorf("RelationFields count = %d, want 2", len(et.Spec.RelationFields))
	}
}

func TestGetEntityType_NotFound(t *testing.T) {
	r := loadTestOntology(t)
	_, err := r.GetEntityType("NotExist")
	if err == nil {
		t.Fatal("GetEntityType(NotExist) expected error, got nil")
	}
	if !errors.Is(err, ErrSchemaNotFound) {
		t.Errorf("error = %v, want ErrSchemaNotFound", err)
	}
}

func TestGetRelationType_Found(t *testing.T) {
	r := loadTestOntology(t)
	rt, err := r.GetRelationType("HAS_INTERFACE")
	if err != nil {
		t.Fatalf("GetRelationType(HAS_INTERFACE) error = %v", err)
	}
	if rt == nil {
		t.Fatal("GetRelationType(HAS_INTERFACE) returned nil")
	}
	if rt.Metadata.Name != "HAS_INTERFACE" {
		t.Errorf("Metadata.Name = %q, want %q", rt.Metadata.Name, "HAS_INTERFACE")
	}
	if len(rt.Spec.Source) != 1 || rt.Spec.Source[0] != "Device" {
		t.Errorf("Source = %v, want [Device]", rt.Spec.Source)
	}
	if len(rt.Spec.Target) != 1 || rt.Spec.Target[0] != "Interface" {
		t.Errorf("Target = %v, want [Interface]", rt.Spec.Target)
	}
}

func TestGetRelationType_NotFound(t *testing.T) {
	r := loadTestOntology(t)
	_, err := r.GetRelationType("NotExist")
	if err == nil {
		t.Fatal("GetRelationType(NotExist) expected error, got nil")
	}
	if !errors.Is(err, ErrSchemaNotFound) {
		t.Errorf("error = %v, want ErrSchemaNotFound", err)
	}
}

func TestListEntityTypes_Content(t *testing.T) {
	r := loadTestOntology(t)
	ets := r.ListEntityTypes()
	if len(ets) != 6 {
		t.Fatalf("ListEntityTypes() len = %d, want 6", len(ets))
	}
	names := make(map[string]bool)
	for _, et := range ets {
		names[et.Metadata.Name] = true
	}
	for _, name := range []string{"Device", "Interface", "ISIS", "Link", "Network_Slice", "Alarm"} {
		if !names[name] {
			t.Errorf("EntityType %q not found in list", name)
		}
	}
}

func TestListRelationTypes_Content(t *testing.T) {
	r := loadTestOntology(t)
	rts := r.ListRelationTypes()
	if len(rts) != 5 {
		t.Fatalf("ListRelationTypes() len = %d, want 5", len(rts))
	}
	names := make(map[string]bool)
	for _, rt := range rts {
		names[rt.Metadata.Name] = true
	}
	for _, name := range []string{"HAS_INTERFACE", "RUNS_ON", "ENDPOINT", "CONNECTS_TO", "OCCURRED_ON"} {
		if !names[name] {
			t.Errorf("RelationType %q not found in list", name)
		}
	}
}

func TestCrossValidate_WarnsOnMissing(t *testing.T) {
	dir := t.TempDir()
	etContent := `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
spec:
  identity:
    stableKeys: [id]
  uriTemplate: "d:{id}"
  relationFields:
    links:
      relationType: NONEXISTENT_RELATION
  properties:
    id:
      type: string
      required: true
`
	if err := os.WriteFile(filepath.Join(dir, "device.yaml"), []byte(etContent), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	slog.SetDefault(logger)
	defer slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	r := NewSchemaRegistry()
	if err := r.Load(dir); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("NONEXISTENT_RELATION")) {
		t.Errorf("expected slog.Warn to mention NONEXISTENT_RELATION, got: %s", buf.String())
	}
}

// ============================================================
// Validate tests
// ============================================================

func TestValidate_UnknownKind(t *testing.T) {
	r := loadTestOntology(t)
	err := r.Validate("UnknownKind", map[string]any{"foo": "bar"})
	if !errors.Is(err, ErrSchemaNotFound) {
		t.Errorf("Validate(UnknownKind) error = %v, want ErrSchemaNotFound", err)
	}
}

func TestValidate_AllValid(t *testing.T) {
	r := loadTestOntology(t)
	props := map[string]any{
		"serial_number": "SN001",
		"hostname":      "router-01",
		"status":        "Up",
	}
	if err := r.Validate("Device", props); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_RequiredMissing(t *testing.T) {
	r := loadTestOntology(t)
	props := map[string]any{
		"serial_number": "SN001",
		// missing required "hostname"
	}
	err := r.Validate("Device", props)
	if err == nil {
		t.Fatal("Validate() expected error for missing required field")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("hostname")) {
		t.Errorf("error = %q, want mention of hostname", err.Error())
	}
}

func TestValidate_TypeMismatch(t *testing.T) {
	r := loadTestOntology(t)
	props := map[string]any{
		"serial_number": 12345, // string field, got int
		"hostname":      "router-01",
	}
	if err := r.Validate("Device", props); err == nil {
		t.Fatal("Validate() expected error for type mismatch")
	}
}

func TestValidate_TypeMismatch_IntAcceptsFloat64(t *testing.T) {
	dir := t.TempDir()
	content := `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Counter
spec:
  identity:
    stableKeys: [id]
  uriTemplate: "c:{id}"
  properties:
    id:
      type: string
      required: true
    count:
      type: int
`
	if err := os.WriteFile(filepath.Join(dir, "counter.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewSchemaRegistry()
	if err := r.Load(dir); err != nil {
		t.Fatal(err)
	}
	// float64 is acceptable for int field (JSON compatibility)
	props := map[string]any{"id": "c1", "count": float64(42)}
	if err := r.Validate("Counter", props); err != nil {
		t.Errorf("Validate() error = %v, want nil (int accepts float64)", err)
	}
}

func TestValidate_EnumInvalid(t *testing.T) {
	r := loadTestOntology(t)
	props := map[string]any{
		"serial_number": "SN001",
		"hostname":      "router-01",
		"status":        "InvalidStatus",
	}
	if err := r.Validate("Device", props); err == nil {
		t.Fatal("Validate() expected error for invalid enum value")
	}
}

func TestValidate_EnumValid(t *testing.T) {
	r := loadTestOntology(t)
	props := map[string]any{
		"serial_number": "SN001",
		"hostname":      "router-01",
		"status":        "Down",
	}
	if err := r.Validate("Device", props); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidate_StableKeyEmpty(t *testing.T) {
	r := loadTestOntology(t)
	props := map[string]any{
		"serial_number": "", // stableKey is empty
		"hostname":      "router-01",
	}
	err := r.Validate("Device", props)
	if err == nil {
		t.Fatal("Validate() expected error for empty stableKey")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("serial_number")) {
		t.Errorf("error = %q, want mention of serial_number", err.Error())
	}
}

func TestValidate_StableKeyMissing(t *testing.T) {
	r := loadTestOntology(t)
	props := map[string]any{
		"hostname": "router-01",
		// serial_number (stableKey) completely absent
	}
	if err := r.Validate("Device", props); err == nil {
		t.Fatal("Validate() expected error for missing stableKey")
	}
}

func TestValidate_MultipleFailures(t *testing.T) {
	r := loadTestOntology(t)
	props := map[string]any{
		// missing serial_number (required + stableKey)
		// missing hostname (required)
		"status": "BadValue", // invalid enum
	}
	err := r.Validate("Device", props)
	if err == nil {
		t.Fatal("Validate() expected error for multiple failures")
	}
	if !bytes.Contains([]byte(err.Error()), []byte(";")) {
		t.Errorf("error = %q, expected aggregated errors separated by ';'", err.Error())
	}
}

func TestValidate_DoesNotMutateInput(t *testing.T) {
	r := loadTestOntology(t)
	original := map[string]any{
		"serial_number": "SN001",
		"hostname":      "router-01",
		"status":        "Up",
	}
	snapshot := make(map[string]any, len(original))
	for k, v := range original {
		snapshot[k] = v
	}
	_ = r.Validate("Device", original)
	if len(original) != len(snapshot) {
		t.Errorf("Validate modified map size: got %d, want %d", len(original), len(snapshot))
	}
	for k, v := range snapshot {
		if original[k] != v {
			t.Errorf("Validate modified key %q: got %v, want %v", k, original[k], v)
		}
	}
}

// ============================================================
// ApplyDefaults tests
// ============================================================

func TestApplyDefaults_UnknownKind(t *testing.T) {
	r := loadTestOntology(t)
	result, err := r.ApplyDefaults("UnknownKind", map[string]any{"foo": "bar"})
	if result != nil {
		t.Errorf("ApplyDefaults(UnknownKind) result = %v, want nil", result)
	}
	if !errors.Is(err, ErrSchemaNotFound) {
		t.Errorf("ApplyDefaults(UnknownKind) error = %v, want ErrSchemaNotFound", err)
	}
}

func TestApplyDefaults_FillsDefaults(t *testing.T) {
	r := loadTestOntology(t)
	props := map[string]any{
		"serial_number": "SN001",
		"hostname":      "router-01",
		// status has default "Up"
	}
	result, err := r.ApplyDefaults("Device", props)
	if err != nil {
		t.Fatalf("ApplyDefaults() error = %v", err)
	}
	if result["status"] != "Up" {
		t.Errorf("result[status] = %v, want %q", result["status"], "Up")
	}
	// Original props should not be modified
	if _, ok := props["status"]; ok {
		t.Error("ApplyDefaults modified original props map")
	}
}

func TestApplyDefaults_PreservesExisting(t *testing.T) {
	r := loadTestOntology(t)
	props := map[string]any{
		"serial_number": "SN001",
		"hostname":      "router-01",
		"status":        "Down", // explicitly set, should not be overridden
	}
	result, err := r.ApplyDefaults("Device", props)
	if err != nil {
		t.Fatalf("ApplyDefaults() error = %v", err)
	}
	if result["status"] != "Down" {
		t.Errorf("result[status] = %v, want %q (should preserve existing)", result["status"], "Down")
	}
}

func TestApplyDefaults_ReturnsNewMap(t *testing.T) {
	r := loadTestOntology(t)
	props := map[string]any{
		"serial_number": "SN001",
		"hostname":      "router-01",
	}
	result, err := r.ApplyDefaults("Device", props)
	if err != nil {
		t.Fatalf("ApplyDefaults() error = %v", err)
	}
	// Original must not be mutated
	if len(props) != 2 {
		t.Errorf("original props len = %d, want 2 (ApplyDefaults should not mutate)", len(props))
	}
	// Result should have more keys (defaults filled)
	if len(result) <= len(props) {
		t.Errorf("result len = %d, want > %d (defaults should be filled)", len(result), len(props))
	}
}

func TestApplyDefaults_NoDefaultFields(t *testing.T) {
	r := loadTestOntology(t)
	// Alarm has no fields with default values
	props := map[string]any{
		"alarm_id": "A001",
		"severity": "Critical",
		"message":  "test",
	}
	result, err := r.ApplyDefaults("Alarm", props)
	if err != nil {
		t.Fatalf("ApplyDefaults() error = %v", err)
	}
	for k, v := range props {
		if result[k] != v {
			t.Errorf("result[%q] = %v, want %v", k, result[k], v)
		}
	}
}
