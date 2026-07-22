package schema

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Compile-time interface satisfaction check ---

// stubRegistry is a minimal implementation to verify the SchemaRegistry
// interface can be satisfied. Full implementation is deferred to I-01.
type stubRegistry struct{}

func (s *stubRegistry) Load(_ string) error                         { return nil }
func (s *stubRegistry) GetEntityType(_ string) (*EntityType, error) { return nil, ErrSchemaNotFound }
func (s *stubRegistry) GetRelationType(_ string) (*RelationType, error) {
	return nil, ErrSchemaNotFound
}
func (s *stubRegistry) ListEntityTypes() []*EntityType            { return nil }
func (s *stubRegistry) ListRelationTypes() []*RelationType        { return nil }
func (s *stubRegistry) Validate(_ string, _ map[string]any) error { return nil }
func (s *stubRegistry) ApplyDefaults(_ string, props map[string]any) (map[string]any, error) {
	return props, nil
}
func (s *stubRegistry) GetLabels(kind string) []string { return []string{kind} }

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
	_ = r.GetLabels("Device")
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
	if got := len(r.ListEntityTypes()); got != 12 {
		t.Errorf("ListEntityTypes() len = %d, want 12", got)
	}
	if got := len(r.ListRelationTypes()); got != 7 {
		t.Errorf("ListRelationTypes() len = %d, want 7", got)
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
	if len(ets) != 12 {
		t.Fatalf("ListEntityTypes() len = %d, want 12", len(ets))
	}
	names := make(map[string]bool)
	for _, et := range ets {
		names[et.Metadata.Name] = true
	}
	for _, name := range []string{"Device", "Interface", "ISIS", "Link", "Network_Slice", "Alarm", "VPN", "BGP", "Tunnel", "Resource", "Service", "Event"} {
		if !names[name] {
			t.Errorf("EntityType %q not found in list", name)
		}
	}
}

func TestListRelationTypes_Content(t *testing.T) {
	r := loadTestOntology(t)
	rts := r.ListRelationTypes()
	if len(rts) != 7 {
		t.Fatalf("ListRelationTypes() len = %d, want 7", len(rts))
	}
	names := make(map[string]bool)
	for _, rt := range rts {
		names[rt.Metadata.Name] = true
	}
	for _, name := range []string{"HAS_INTERFACE", "RUNS_ON", "ENDPOINT", "CONNECTS_TO", "OCCURRED_ON", "HAS_BGP_PEER", "TUNNEL_FOR"} {
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
	if !strings.Contains(err.Error(), "\n") {
		t.Errorf("error = %q, expected aggregated errors separated by '\\n' (errors.Join)", err.Error())
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

// ============================================================
// Inheritance tests (V1-15)
// ============================================================

// loadTestRegistry creates a SchemaRegistry from inline YAML files in a temp directory.
// files is a map of filename -> YAML content.
func loadTestRegistry(t *testing.T, files map[string]string) (SchemaRegistry, error) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	r := NewSchemaRegistry()
	err := r.Load(dir)
	return r, err
}

const (
	resourceBaseYAML = `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Resource
  labels: [Resource]
spec:
  identity:
    stableKeys: []
  uriTemplate: ""
  properties:
    status:
      type: string
      enum: [Up, Down, Maintenance]
    vendor:
      type: string
`
	serviceBaseYAML = `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Service
  labels: [Service]
spec:
  identity:
    stableKeys: []
  uriTemplate: ""
  properties:
    status:
      type: string
    name:
      type: string
`
	eventBaseYAML = `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Event
  labels: [Event]
spec:
  identity:
    stableKeys: []
  uriTemplate: ""
  properties:
    timestamp:
      type: string
    severity:
      type: string
    message:
      type: string
`
)

func TestInheritance_SimpleExtends(t *testing.T) {
	files := map[string]string{
		"resource.yaml": resourceBaseYAML,
		"device.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
  labels: [Resource, Network]
spec:
  extends: Resource
  identity:
    stableKeys: [serial_number]
  uriTemplate: "device:{serial_number}"
  properties:
    serial_number:
      type: string
      required: true
    hostname:
      type: string
      required: true
`,
	}
	r, err := loadTestRegistry(t, files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	et, err := r.GetEntityType("Device")
	if err != nil {
		t.Fatalf("GetEntityType(Device) error = %v", err)
	}

	// Device should have its own properties + inherited from Resource
	if _, ok := et.Spec.Properties["serial_number"]; !ok {
		t.Error("missing own property serial_number")
	}
	if _, ok := et.Spec.Properties["hostname"]; !ok {
		t.Error("missing own property hostname")
	}
	if _, ok := et.Spec.Properties["status"]; !ok {
		t.Error("missing inherited property status from Resource")
	}
	if _, ok := et.Spec.Properties["vendor"]; !ok {
		t.Error("missing inherited property vendor from Resource")
	}
}

func TestInheritance_ChildOverridesParent(t *testing.T) {
	files := map[string]string{
		"resource.yaml": resourceBaseYAML,
		"device.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
spec:
  extends: Resource
  identity:
    stableKeys: [serial_number]
  uriTemplate: "device:{serial_number}"
  properties:
    serial_number:
      type: string
      required: true
    status:
      type: string
      enum: [Active, Inactive]
      default: "Active"
`,
	}
	r, err := loadTestRegistry(t, files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	et, err := r.GetEntityType("Device")
	if err != nil {
		t.Fatalf("GetEntityType(Device) error = %v", err)
	}

	// Child's status should override parent's status
	statusProp, ok := et.Spec.Properties["status"]
	if !ok {
		t.Fatal("missing status property")
	}
	if len(statusProp.Enum) != 2 || statusProp.Enum[0] != "Active" || statusProp.Enum[1] != "Inactive" {
		t.Errorf("status enum = %v, want [Active, Inactive] (child override)", statusProp.Enum)
	}
	if statusProp.Default != "Active" {
		t.Errorf("status default = %v, want Active (child override)", statusProp.Default)
	}

	// Parent's vendor should still be inherited
	if _, ok := et.Spec.Properties["vendor"]; !ok {
		t.Error("missing inherited property vendor from Resource")
	}
}

func TestInheritance_FieldMappingMerge(t *testing.T) {
	files := map[string]string{
		"base.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Base
spec:
  identity:
    stableKeys: []
  uriTemplate: ""
  fieldMapping:
    base_field: canonical_base
    shared: base_canonical
  properties: {}
`,
		"child.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Child
spec:
  extends: Base
  identity:
    stableKeys: []
  uriTemplate: ""
  fieldMapping:
    child_field: canonical_child
    shared: child_canonical
  properties: {}
`,
	}
	r, err := loadTestRegistry(t, files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	et, err := r.GetEntityType("Child")
	if err != nil {
		t.Fatalf("GetEntityType(Child) error = %v", err)
	}

	// child_field: own
	if et.Spec.FieldMapping["child_field"] != "canonical_child" {
		t.Errorf("FieldMapping[child_field] = %q, want canonical_child", et.Spec.FieldMapping["child_field"])
	}
	// base_field: inherited
	if et.Spec.FieldMapping["base_field"] != "canonical_base" {
		t.Errorf("FieldMapping[base_field] = %q, want canonical_base", et.Spec.FieldMapping["base_field"])
	}
	// shared: child overrides parent
	if et.Spec.FieldMapping["shared"] != "child_canonical" {
		t.Errorf("FieldMapping[shared] = %q, want child_canonical (child override)", et.Spec.FieldMapping["shared"])
	}
}

func TestInheritance_NormalizeMerge(t *testing.T) {
	files := map[string]string{
		"base.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Base
spec:
  identity:
    stableKeys: []
  uriTemplate: ""
  normalize:
    - field: name
      pattern: " "
      replace: "_"
  properties: {}
`,
		"child.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Child
spec:
  extends: Base
  identity:
    stableKeys: []
  uriTemplate: ""
  normalize:
    - field: hostname
      pattern: "-"
      replace: "_"
  properties: {}
`,
	}
	r, err := loadTestRegistry(t, files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	et, err := r.GetEntityType("Child")
	if err != nil {
		t.Fatalf("GetEntityType(Child) error = %v", err)
	}

	// Parent rules prepended before child rules
	if len(et.Spec.Normalize) != 2 {
		t.Fatalf("Normalize len = %d, want 2", len(et.Spec.Normalize))
	}
	if et.Spec.Normalize[0].Field != "name" {
		t.Errorf("Normalize[0].Field = %q, want name (parent rule first)", et.Spec.Normalize[0].Field)
	}
	if et.Spec.Normalize[1].Field != "hostname" {
		t.Errorf("Normalize[1].Field = %q, want hostname (child rule second)", et.Spec.Normalize[1].Field)
	}
}

func TestInheritance_RelationFieldsMerge(t *testing.T) {
	files := map[string]string{
		"base.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Base
spec:
  identity:
    stableKeys: []
  uriTemplate: ""
  relationFields:
    base_rel:
      relationType: BASE_REL
  properties: {}
`,
		"child.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Child
spec:
  extends: Base
  identity:
    stableKeys: []
  uriTemplate: ""
  relationFields:
    child_rel:
      relationType: CHILD_REL
  properties: {}
`,
	}
	r, err := loadTestRegistry(t, files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	et, err := r.GetEntityType("Child")
	if err != nil {
		t.Fatalf("GetEntityType(Child) error = %v", err)
	}

	if len(et.Spec.RelationFields) != 2 {
		t.Fatalf("RelationFields count = %d, want 2", len(et.Spec.RelationFields))
	}
	if et.Spec.RelationFields["child_rel"].RelationType != "CHILD_REL" {
		t.Errorf("RelationFields[child_rel] = %q, want CHILD_REL", et.Spec.RelationFields["child_rel"].RelationType)
	}
	if et.Spec.RelationFields["base_rel"].RelationType != "BASE_REL" {
		t.Errorf("RelationFields[base_rel] = %q, want BASE_REL (inherited)", et.Spec.RelationFields["base_rel"].RelationType)
	}
}

func TestInheritance_IdentityNotInherited(t *testing.T) {
	files := map[string]string{
		"resource.yaml": resourceBaseYAML,
		"device.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
spec:
  extends: Resource
  identity:
    stableKeys: [serial_number]
  uriTemplate: "device:{serial_number}"
  properties:
    serial_number:
      type: string
      required: true
`,
	}
	r, err := loadTestRegistry(t, files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	et, err := r.GetEntityType("Device")
	if err != nil {
		t.Fatalf("GetEntityType(Device) error = %v", err)
	}

	// Device should keep its own identity
	if len(et.Spec.Identity.StableKeys) != 1 || et.Spec.Identity.StableKeys[0] != "serial_number" {
		t.Errorf("StableKeys = %v, want [serial_number]", et.Spec.Identity.StableKeys)
	}
	if et.Spec.URITemplate != "device:{serial_number}" {
		t.Errorf("URITemplate = %q, want device:{serial_number}", et.Spec.URITemplate)
	}

	// Resource should still have its own (empty) identity
	res, _ := r.GetEntityType("Resource")
	if len(res.Spec.Identity.StableKeys) != 0 {
		t.Errorf("Resource StableKeys = %v, want []", res.Spec.Identity.StableKeys)
	}
}

func TestInheritance_CircularDetection(t *testing.T) {
	files := map[string]string{
		"a.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: A
spec:
  extends: B
  identity:
    stableKeys: []
  uriTemplate: ""
  properties: {}
`,
		"b.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: B
spec:
  extends: A
  identity:
    stableKeys: []
  uriTemplate: ""
  properties: {}
`,
	}
	_, err := loadTestRegistry(t, files)
	if err == nil {
		t.Fatal("Load() expected error for circular inheritance, got nil")
	}
	if !strings.Contains(err.Error(), "circular inheritance") {
		t.Errorf("error = %q, want mention of circular inheritance", err.Error())
	}
}

func TestInheritance_MultiLevel(t *testing.T) {
	files := map[string]string{
		"c.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: C
spec:
  identity:
    stableKeys: []
  uriTemplate: ""
  properties:
    prop_c:
      type: string
    shared:
      type: string
`,
		"b.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: B
spec:
  extends: C
  identity:
    stableKeys: []
  uriTemplate: ""
  properties:
    prop_b:
      type: string
`,
		"a.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: A
spec:
  extends: B
  identity:
    stableKeys: []
  uriTemplate: ""
  properties:
    prop_a:
      type: string
`,
	}
	r, err := loadTestRegistry(t, files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	et, err := r.GetEntityType("A")
	if err != nil {
		t.Fatalf("GetEntityType(A) error = %v", err)
	}

	// A should have properties from B and C via topological merge
	for _, prop := range []string{"prop_a", "prop_b", "prop_c", "shared"} {
		if _, ok := et.Spec.Properties[prop]; !ok {
			t.Errorf("missing property %q in multi-level inheritance", prop)
		}
	}
}

func TestInheritance_UnknownParent(t *testing.T) {
	files := map[string]string{
		"child.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Child
spec:
  extends: NonExistent
  identity:
    stableKeys: []
  uriTemplate: ""
  properties: {}
`,
	}
	_, err := loadTestRegistry(t, files)
	if err == nil {
		t.Fatal("Load() expected error for unknown parent, got nil")
	}
	if !strings.Contains(err.Error(), "NonExistent") {
		t.Errorf("error = %q, want mention of NonExistent", err.Error())
	}
}

func TestInheritance_BackwardCompatible(t *testing.T) {
	// No extends field - should work exactly as before
	files := map[string]string{
		"device.yaml": `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
spec:
  identity:
    stableKeys: [serial_number]
  uriTemplate: "device:{serial_number}"
  properties:
    serial_number:
      type: string
      required: true
    hostname:
      type: string
`,
	}
	r, err := loadTestRegistry(t, files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	et, err := r.GetEntityType("Device")
	if err != nil {
		t.Fatalf("GetEntityType(Device) error = %v", err)
	}
	if len(et.Spec.Properties) != 2 {
		t.Errorf("Properties count = %d, want 2", len(et.Spec.Properties))
	}
	if et.Spec.Extends != "" {
		t.Errorf("Extends = %q, want empty", et.Spec.Extends)
	}
}

func TestInheritance_RealOntology(t *testing.T) {
	r := loadTestOntology(t)

	// Device extends Resource, should inherit vendor property if not already defined
	et, err := r.GetEntityType("Device")
	if err != nil {
		t.Fatalf("GetEntityType(Device) error = %v", err)
	}

	// Device already defines vendor and status, so count stays 8
	if len(et.Spec.Properties) != 8 {
		t.Errorf("Device Properties count = %d, want 8 (own properties override parent)", len(et.Spec.Properties))
	}

	// Interface extends Resource, should inherit vendor if not defined
	iface, err := r.GetEntityType("Interface")
	if err != nil {
		t.Fatalf("GetEntityType(Interface) error = %v", err)
	}
	if _, ok := iface.Spec.Properties["vendor"]; !ok {
		t.Error("Interface should inherit vendor from Resource")
	}
	if _, ok := iface.Spec.Properties["status"]; !ok {
		t.Error("Interface should have status (own definition)")
	}
}

// ============================================================
// GetLabels tests (V1-16)
// ============================================================

func TestGetLabels_NoExtends(t *testing.T) {
	r := loadTestOntology(t)
	labels := r.GetLabels("BGP")
	if len(labels) != 1 || labels[0] != "BGP" {
		t.Errorf("GetLabels(BGP) = %v, want [BGP]", labels)
	}
}

func TestGetLabels_SingleLevel(t *testing.T) {
	r := loadTestOntology(t)
	labels := r.GetLabels("Device")
	if len(labels) != 2 {
		t.Fatalf("GetLabels(Device) len = %d, want 2", len(labels))
	}
	if labels[0] != "Resource" || labels[1] != "Device" {
		t.Errorf("GetLabels(Device) = %v, want [Resource Device]", labels)
	}
}

func TestGetLabels_UnknownEntity(t *testing.T) {
	r := loadTestOntology(t)
	labels := r.GetLabels("NonExistent")
	if len(labels) != 1 || labels[0] != "NonExistent" {
		t.Errorf("GetLabels(NonExistent) = %v, want [NonExistent]", labels)
	}
}

func TestGetLabels_AllRealOntology(t *testing.T) {
	r := loadTestOntology(t)
	cases := []struct {
		kind string
		want []string
	}{
		{"Device", []string{"Resource", "Device"}},
		{"Interface", []string{"Resource", "Interface"}},
		{"ISIS", []string{"Resource", "ISIS"}},
		{"Link", []string{"Resource", "Link"}},
		{"Network_Slice", []string{"Service", "Network_Slice"}},
		{"Alarm", []string{"Event", "Alarm"}},
		{"BGP", []string{"BGP"}},
		{"Tunnel", []string{"Tunnel"}},
		{"VPN", []string{"VPN"}},
	}
	for _, tc := range cases {
		got := r.GetLabels(tc.kind)
		if len(got) != len(tc.want) {
			t.Errorf("GetLabels(%s) = %v, want %v", tc.kind, got, tc.want)
			continue
		}
		for i, l := range got {
			if l != tc.want[i] {
				t.Errorf("GetLabels(%s)[%d] = %q, want %q", tc.kind, i, l, tc.want[i])
			}
		}
	}
}

func TestGetLabels_CycleProtection(t *testing.T) {
	// 直接操作内部 map 构造环（绕过 Load 的环检测）
	r := NewSchemaRegistry().(*registryImpl)
	r.entityTypes["A"] = &EntityType{Spec: EntityTypeSpec{Extends: "B"}}
	r.entityTypes["B"] = &EntityType{Spec: EntityTypeSpec{Extends: "A"}}

	labels := r.GetLabels("A")
	// 不死循环即通过（break 保护生效）
	if len(labels) == 0 {
		t.Error("GetLabels(A) should not return empty slice")
	}
	t.Logf("GetLabels(A) with cycle = %v", labels)
}
