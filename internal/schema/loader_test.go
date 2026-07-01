package schema

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromFile_EntityType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "device.yaml")

	content := `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
  labels: [Resource, Network]
spec:
  identity:
    stableKeys: [serial_number]
  uriTemplate: "device:{serial_number}"
  fieldMapping:
    mgmt_ip: management_ip
  normalize:
    - field: hostname
      pattern: " "
      replace: "_"
  relationFields:
    interfaces:
      relationType: HAS_INTERFACE
  properties:
    serial_number:
      type: string
      required: true
    hostname:
      type: string
      required: true
    vendor:
      type: string
    model:
      type: string
    management_ip:
      type: string
    chassis_mac:
      type: string
    status:
      type: string
      enum: [Up, Down, Maintenance]
      default: "Up"
    device_type:
      type: string
      enum: [Core, Edge, Access]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	et, rt, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}
	if rt != nil {
		t.Fatal("expected relationType to be nil for EntityType file")
	}
	if et == nil {
		t.Fatal("expected entityType to be non-nil")
	}
	if et.Metadata.Name != "Device" {
		t.Errorf("Metadata.Name = %q, want %q", et.Metadata.Name, "Device")
	}
	if et.APIVersion != "twin.io/v1" {
		t.Errorf("APIVersion = %q, want %q", et.APIVersion, "twin.io/v1")
	}
	if len(et.Spec.Properties) != 8 {
		t.Errorf("Properties count = %d, want 8", len(et.Spec.Properties))
	}
	if len(et.Spec.RelationFields) != 1 {
		t.Errorf("RelationFields count = %d, want 1", len(et.Spec.RelationFields))
	}
}

func TestLoadFromFile_RelationType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "has_interface.yaml")

	content := `apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: HAS_INTERFACE
spec:
  source: [Device]
  target: [Interface]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	et, rt, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}
	if et != nil {
		t.Fatal("expected entityType to be nil for RelationType file")
	}
	if rt == nil {
		t.Fatal("expected relationType to be non-nil")
	}
	if rt.Metadata.Name != "HAS_INTERFACE" {
		t.Errorf("Metadata.Name = %q, want %q", rt.Metadata.Name, "HAS_INTERFACE")
	}
}

func TestLoadFromFile_UnsupportedKind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")

	content := `apiVersion: twin.io/v1
kind: UnknownKind
metadata:
  name: Something
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := LoadFromFile(path)
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Errorf("error = %v, want ErrUnsupportedKind", err)
	}
}

func TestLoadFromFile_InvalidAPIVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_version.yaml")

	content := `apiVersion: twin.io/v2
kind: EntityType
metadata:
  name: Device
spec:
  identity:
    stableKeys: [id]
  uriTemplate: "d:{id}"
  properties:
    id:
      type: string
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := LoadFromFile(path)
	if err == nil {
		t.Fatal("expected error for invalid apiVersion")
	}
	if !errors.Is(err, ErrInvalidAPIVersion) {
		t.Errorf("error = %v, want ErrInvalidAPIVersion", err)
	}
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")

	content := `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: [invalid yaml
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := LoadFromFile(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
	if !errors.Is(err, ErrInvalidSchema) {
		t.Errorf("error = %v, want ErrInvalidSchema", err)
	}
}

func TestLoadFromFile_FileNotFound(t *testing.T) {
	_, _, err := LoadFromFile("/nonexistent/path/schema.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

// TestLoadFromFile_MultiDocumentReturnsFirstOnly verifies that LoadFromFile
// returns only the first document from a multi-document YAML file.
// Multi-document expansion is handled by LoadFromDir.
func TestLoadFromFile_MultiDocumentReturnsFirstOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relations.yaml")

	content := `apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: HAS_INTERFACE
spec:
  source: [Device]
  target: [Interface]
---
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: RUNS_ON
spec:
  source: [ISIS]
  target: [Interface]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	et, rt, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}
	if et != nil {
		t.Error("expected entityType to be nil for RelationType file")
	}
	if rt == nil {
		t.Fatal("expected relationType to be non-nil")
	}
	// Only the first document is returned
	if rt.Metadata.Name != "HAS_INTERFACE" {
		t.Errorf("Metadata.Name = %q, want %q", rt.Metadata.Name, "HAS_INTERFACE")
	}
}

func TestLoadFromDir_MixedFiles(t *testing.T) {
	dir := t.TempDir()

	// Write an EntityType file
	etContent := `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
  labels: [Resource]
spec:
  identity:
    stableKeys: [serial_number]
  uriTemplate: "device:{serial_number}"
  properties:
    serial_number:
      type: string
      required: true
`
	if err := os.WriteFile(filepath.Join(dir, "device.yaml"), []byte(etContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a RelationType file
	rtContent := `apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: HAS_INTERFACE
spec:
  source: [Device]
  target: [Interface]
`
	if err := os.WriteFile(filepath.Join(dir, "has_interface.yaml"), []byte(rtContent), 0644); err != nil {
		t.Fatal(err)
	}

	entityTypes, relationTypes, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}
	if len(entityTypes) != 1 {
		t.Errorf("entityTypes count = %d, want 1", len(entityTypes))
	}
	if len(relationTypes) != 1 {
		t.Errorf("relationTypes count = %d, want 1", len(relationTypes))
	}
	if entityTypes[0].Metadata.Name != "Device" {
		t.Errorf("entityTypes[0].Name = %q, want %q", entityTypes[0].Metadata.Name, "Device")
	}
	if relationTypes[0].Metadata.Name != "HAS_INTERFACE" {
		t.Errorf("relationTypes[0].Name = %q, want %q", relationTypes[0].Metadata.Name, "HAS_INTERFACE")
	}
}

func TestLoadFromDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	entityTypes, relationTypes, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}
	if len(entityTypes) != 0 {
		t.Errorf("entityTypes count = %d, want 0", len(entityTypes))
	}
	if len(relationTypes) != 0 {
		t.Errorf("relationTypes count = %d, want 0", len(relationTypes))
	}
}

func TestLoadFromDir_SkipsNonYAML(t *testing.T) {
	dir := t.TempDir()

	// Write a valid YAML file
	yamlContent := `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
spec:
  identity:
    stableKeys: [id]
  uriTemplate: "d:{id}"
  properties:
    id:
      type: string
`
	if err := os.WriteFile(filepath.Join(dir, "device.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a non-YAML file (should be skipped)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Not YAML"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"key": "value"}`), 0644); err != nil {
		t.Fatal(err)
	}

	entityTypes, _, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}
	if len(entityTypes) != 1 {
		t.Errorf("entityTypes count = %d, want 1 (non-YAML files should be skipped)", len(entityTypes))
	}
}

func TestLoadFromDir_InvalidFileReturnsError(t *testing.T) {
	dir := t.TempDir()

	// Write an invalid YAML file
	invalidContent := `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: [invalid
`
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(invalidContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := LoadFromDir(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML file")
	}
}

// TestLoadOntologyDir loads the actual ontology/ directory and verifies
// all 6 EntityTypes and 4 RelationTypes are parsed correctly.
// This is an integration test that depends on the ontology YAML files.
func TestLoadOntologyDir(t *testing.T) {
	// Resolve ontology directory relative to this test file
	ontologyDir := filepath.Join("..", "..", "ontology")

	// Verify directory exists
	if _, err := os.Stat(ontologyDir); os.IsNotExist(err) {
		t.Skipf("ontology directory not found at %s, skipping", ontologyDir)
	}

	entityTypes, relationTypes, err := LoadFromDir(ontologyDir)
	if err != nil {
		t.Fatalf("LoadFromDir(%q) error = %v", ontologyDir, err)
	}

	// Verify counts per spec SC-002 and SC-003
	if len(entityTypes) != 9 {
		t.Errorf("entityTypes count = %d, want 9", len(entityTypes))
	}
	if len(relationTypes) != 7 {
		t.Errorf("relationTypes count = %d, want 7", len(relationTypes))
	}

	// Build lookup maps
	etByName := make(map[string]EntityType)
	for _, et := range entityTypes {
		etByName[et.Metadata.Name] = et
	}

	// Verify expected entity type names
	expectedEntityTypes := []string{"Device", "Interface", "ISIS", "Link", "Network_Slice", "Alarm"}
	for _, name := range expectedEntityTypes {
		if _, ok := etByName[name]; !ok {
			t.Errorf("EntityType %q not found", name)
		}
	}

	// Verify property counts per SC-002
	expectedPropCounts := map[string]int{
		"Device":        8,
		"Interface":     5,
		"ISIS":          5,
		"Link":          4,
		"Network_Slice": 4,
		"Alarm":         4,
	}
	for name, wantCount := range expectedPropCounts {
		et, ok := etByName[name]
		if !ok {
			continue
		}
		if got := len(et.Spec.Properties); got != wantCount {
			t.Errorf("EntityType %q properties = %d, want %d", name, got, wantCount)
		}
	}

	// Verify stableKeys fields are required: true per SC-004
	for _, et := range entityTypes {
		for _, key := range et.Spec.Identity.StableKeys {
			prop, ok := et.Spec.Properties[key]
			if !ok {
				t.Errorf("EntityType %q: stableKey %q not found in properties", et.Metadata.Name, key)
				continue
			}
			if !prop.Required {
				t.Errorf("EntityType %q: stableKey property %q is not required: true", et.Metadata.Name, key)
			}
		}
	}

	// Verify relation type names
	rtByName := make(map[string]RelationType)
	for _, rt := range relationTypes {
		rtByName[rt.Metadata.Name] = rt
	}

	expectedRelationTypes := []string{"HAS_INTERFACE", "RUNS_ON", "ENDPOINT", "OCCURRED_ON"}
	for _, name := range expectedRelationTypes {
		if _, ok := rtByName[name]; !ok {
			t.Errorf("RelationType %q not found", name)
		}
	}

	// Verify all relation types use twin.io/v1
	for _, rt := range relationTypes {
		if rt.APIVersion != "twin.io/v1" {
			t.Errorf("RelationType %q APIVersion = %q, want %q", rt.Metadata.Name, rt.APIVersion, "twin.io/v1")
		}
	}
}

func TestLoadFromDir_MultiDocRelations(t *testing.T) {
	dir := t.TempDir()

	// Write an EntityType file
	etContent := `apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
spec:
  identity:
    stableKeys: [id]
  uriTemplate: "d:{id}"
  properties:
    id:
      type: string
`
	if err := os.WriteFile(filepath.Join(dir, "device.yaml"), []byte(etContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a multi-document RelationType file
	rtContent := `apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: HAS_INTERFACE
spec:
  source: [Device]
  target: [Interface]
---
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: RUNS_ON
spec:
  source: [ISIS]
  target: [Interface]
`
	if err := os.WriteFile(filepath.Join(dir, "relations.yaml"), []byte(rtContent), 0644); err != nil {
		t.Fatal(err)
	}

	entityTypes, relationTypes, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}
	if len(entityTypes) != 1 {
		t.Errorf("entityTypes count = %d, want 1", len(entityTypes))
	}
	if len(relationTypes) != 2 {
		t.Errorf("relationTypes count = %d, want 2 (multi-doc should expand)", len(relationTypes))
	}
}
