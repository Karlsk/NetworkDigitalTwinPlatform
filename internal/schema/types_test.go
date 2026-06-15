package schema

import (
	"bytes"
	"io"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseEntityType(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    EntityType
		wantErr bool
	}{
		{
			name: "valid Device EntityType",
			yaml: `
apiVersion: twin.io/v1
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
    hw_model: model
  normalize:
    - field: hostname
      pattern: " "
      replace: "_"
  relationFields:
    interfaces:
      relationType: HAS_INTERFACE
    upstream_links:
      relationType: CONNECTS_TO
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
`,
			want: EntityType{
				APIVersion: "twin.io/v1",
				Kind:       "EntityType",
				Metadata: Metadata{
					Name:   "Device",
					Labels: []string{"Resource", "Network"},
				},
				Spec: EntityTypeSpec{
					Identity:    IdentitySpec{StableKeys: []string{"serial_number"}},
					URITemplate: "device:{serial_number}",
					FieldMapping: map[string]string{
						"mgmt_ip":  "management_ip",
						"hw_model": "model",
					},
					Normalize: []NormalizeRule{
						{Field: "hostname", Pattern: " ", Replace: "_"},
					},
					RelationFields: map[string]RelationFieldSpec{
						"interfaces":     {RelationType: "HAS_INTERFACE"},
						"upstream_links": {RelationType: "CONNECTS_TO"},
					},
					Properties: map[string]PropertySpec{
						"serial_number": {Type: "string", Required: true},
						"hostname":      {Type: "string", Required: true},
						"vendor":        {Type: "string"},
						"model":         {Type: "string"},
						"management_ip": {Type: "string"},
						"chassis_mac":   {Type: "string"},
						"status":        {Type: "string", Enum: []string{"Up", "Down", "Maintenance"}, Default: "Up"},
						"device_type":   {Type: "string", Enum: []string{"Core", "Edge", "Access"}},
					},
				},
			},
		},
		{
			name: "valid Interface EntityType with composite stableKeys",
			yaml: `
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Interface
  labels: [Resource, Network]
spec:
  identity:
    stableKeys: [device_serial, if_name]
  uriTemplate: "iface:{device_serial}_{if_name}"
  fieldMapping: {}
  normalize: []
  relationFields: {}
  properties:
    device_serial:
      type: string
      required: true
    if_name:
      type: string
      required: true
    status:
      type: string
      enum: [Up, Down]
      default: "Up"
    bandwidth:
      type: int
    description:
      type: string
`,
			want: EntityType{
				APIVersion: "twin.io/v1",
				Kind:       "EntityType",
				Metadata: Metadata{
					Name:   "Interface",
					Labels: []string{"Resource", "Network"},
				},
				Spec: EntityTypeSpec{
					Identity:       IdentitySpec{StableKeys: []string{"device_serial", "if_name"}},
					URITemplate:    "iface:{device_serial}_{if_name}",
					FieldMapping:   map[string]string{},
					Normalize:      []NormalizeRule{},
					RelationFields: map[string]RelationFieldSpec{},
					Properties: map[string]PropertySpec{
						"device_serial": {Type: "string", Required: true},
						"if_name":       {Type: "string", Required: true},
						"status":        {Type: "string", Enum: []string{"Up", "Down"}, Default: "Up"},
						"bandwidth":     {Type: "int"},
						"description":   {Type: "string"},
					},
				},
			},
		},
		{
			name: "empty optional fields produce nil maps and slices",
			yaml: `
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Minimal
spec:
  identity:
    stableKeys: [id]
  uriTemplate: "min:{id}"
  properties:
    id:
      type: string
      required: true
`,
			want: EntityType{
				APIVersion: "twin.io/v1",
				Kind:       "EntityType",
				Metadata:   Metadata{Name: "Minimal"},
				Spec: EntityTypeSpec{
					Identity:    IdentitySpec{StableKeys: []string{"id"}},
					URITemplate: "min:{id}",
					Properties: map[string]PropertySpec{
						"id": {Type: "string", Required: true},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got EntityType
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("yaml.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Verify top-level fields
			if got.APIVersion != tt.want.APIVersion {
				t.Errorf("APIVersion = %q, want %q", got.APIVersion, tt.want.APIVersion)
			}
			if got.Kind != tt.want.Kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.want.Kind)
			}
			if got.Metadata.Name != tt.want.Metadata.Name {
				t.Errorf("Metadata.Name = %q, want %q", got.Metadata.Name, tt.want.Metadata.Name)
			}

			// Verify labels
			if len(got.Metadata.Labels) != len(tt.want.Metadata.Labels) {
				t.Errorf("Metadata.Labels len = %d, want %d", len(got.Metadata.Labels), len(tt.want.Metadata.Labels))
			}

			// Verify identity
			if len(got.Spec.Identity.StableKeys) != len(tt.want.Spec.Identity.StableKeys) {
				t.Errorf("StableKeys len = %d, want %d", len(got.Spec.Identity.StableKeys), len(tt.want.Spec.Identity.StableKeys))
			}

			// Verify URI template
			if got.Spec.URITemplate != tt.want.Spec.URITemplate {
				t.Errorf("URITemplate = %q, want %q", got.Spec.URITemplate, tt.want.Spec.URITemplate)
			}

			// Verify property count
			if len(got.Spec.Properties) != len(tt.want.Spec.Properties) {
				t.Errorf("Properties count = %d, want %d", len(got.Spec.Properties), len(tt.want.Spec.Properties))
			}

			// Verify each property
			for name, wantProp := range tt.want.Spec.Properties {
				gotProp, ok := got.Spec.Properties[name]
				if !ok {
					t.Errorf("Property %q missing", name)
					continue
				}
				if gotProp.Type != wantProp.Type {
					t.Errorf("Property %q.Type = %q, want %q", name, gotProp.Type, wantProp.Type)
				}
				if gotProp.Required != wantProp.Required {
					t.Errorf("Property %q.Required = %v, want %v", name, gotProp.Required, wantProp.Required)
				}
				if len(gotProp.Enum) != len(wantProp.Enum) {
					t.Errorf("Property %q.Enum len = %d, want %d", name, len(gotProp.Enum), len(wantProp.Enum))
				}
			}

			// Verify field mapping count
			if len(got.Spec.FieldMapping) != len(tt.want.Spec.FieldMapping) {
				t.Errorf("FieldMapping count = %d, want %d", len(got.Spec.FieldMapping), len(tt.want.Spec.FieldMapping))
			}

			// Verify normalize rules count
			if len(got.Spec.Normalize) != len(tt.want.Spec.Normalize) {
				t.Errorf("Normalize count = %d, want %d", len(got.Spec.Normalize), len(tt.want.Spec.Normalize))
			}

			// Verify relation fields count
			if len(got.Spec.RelationFields) != len(tt.want.Spec.RelationFields) {
				t.Errorf("RelationFields count = %d, want %d", len(got.Spec.RelationFields), len(tt.want.Spec.RelationFields))
			}
		})
	}
}

func TestParseRelationType(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want RelationType
	}{
		{
			name: "valid HAS_INTERFACE RelationType",
			yaml: `
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: HAS_INTERFACE
spec:
  source: [Device]
  target: [Interface]
`,
			want: RelationType{
				APIVersion: "twin.io/v1",
				Kind:       "RelationType",
				Metadata:   Metadata{Name: "HAS_INTERFACE"},
				Spec: RelationTypeSpec{
					Source: []string{"Device"},
					Target: []string{"Interface"},
				},
			},
		},
		{
			name: "valid RUNS_ON RelationType",
			yaml: `
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: RUNS_ON
spec:
  source: [ISIS]
  target: [Interface]
`,
			want: RelationType{
				APIVersion: "twin.io/v1",
				Kind:       "RelationType",
				Metadata:   Metadata{Name: "RUNS_ON"},
				Spec: RelationTypeSpec{
					Source: []string{"ISIS"},
					Target: []string{"Interface"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got RelationType
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if err != nil {
				t.Fatalf("yaml.Unmarshal() error = %v", err)
			}

			if got.APIVersion != tt.want.APIVersion {
				t.Errorf("APIVersion = %q, want %q", got.APIVersion, tt.want.APIVersion)
			}
			if got.Kind != tt.want.Kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.want.Kind)
			}
			if got.Metadata.Name != tt.want.Metadata.Name {
				t.Errorf("Metadata.Name = %q, want %q", got.Metadata.Name, tt.want.Metadata.Name)
			}
			if len(got.Spec.Source) != len(tt.want.Spec.Source) {
				t.Errorf("Source len = %d, want %d", len(got.Spec.Source), len(tt.want.Spec.Source))
			}
			if len(got.Spec.Target) != len(tt.want.Spec.Target) {
				t.Errorf("Target len = %d, want %d", len(got.Spec.Target), len(tt.want.Spec.Target))
			}

			for i, s := range tt.want.Spec.Source {
				if got.Spec.Source[i] != s {
					t.Errorf("Source[%d] = %q, want %q", i, got.Spec.Source[i], s)
				}
			}
			for i, s := range tt.want.Spec.Target {
				if got.Spec.Target[i] != s {
					t.Errorf("Target[%d] = %q, want %q", i, got.Spec.Target[i], s)
				}
			}
		})
	}
}

func TestParseMultiDocumentYAML(t *testing.T) {
	yamlContent := `
apiVersion: twin.io/v1
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
---
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: ENDPOINT
spec:
  source: [Link]
  target: [Interface]
---
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: OCCURRED_ON
spec:
  source: [Alarm]
  target: [Interface]
`

	decoder := yaml.NewDecoder(bytes.NewReader([]byte(yamlContent)))
	var results []RelationType
	for {
		var doc RelationType
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decoder.Decode() error = %v", err)
		}
		results = append(results, doc)
	}

	if len(results) != 4 {
		t.Fatalf("parsed %d documents, want 4", len(results))
	}

	expectedNames := []string{"HAS_INTERFACE", "RUNS_ON", "ENDPOINT", "OCCURRED_ON"}
	for i, name := range expectedNames {
		if results[i].Metadata.Name != name {
			t.Errorf("document[%d].Metadata.Name = %q, want %q", i, results[i].Metadata.Name, name)
		}
		if results[i].APIVersion != "twin.io/v1" {
			t.Errorf("document[%d].APIVersion = %q, want %q", i, results[i].APIVersion, "twin.io/v1")
		}
		if results[i].Kind != "RelationType" {
			t.Errorf("document[%d].Kind = %q, want %q", i, results[i].Kind, "RelationType")
		}
	}
}

// TestParseMultiDocumentYAMLWithTrailingSeparator verifies that yaml.v3
// produces an empty document for a trailing "---". The loader must filter
// these out by checking for empty Kind fields.
func TestParseMultiDocumentYAMLWithTrailingSeparator(t *testing.T) {
	yamlContent := `
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: HAS_INTERFACE
spec:
  source: [Device]
  target: [Interface]
---
`

	decoder := yaml.NewDecoder(bytes.NewReader([]byte(yamlContent)))
	var all []RelationType
	for {
		var doc RelationType
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decoder.Decode() unexpected error = %v", err)
		}
		all = append(all, doc)
	}

	// yaml.v3 produces 2 documents: one valid + one empty from trailing ---
	if len(all) != 2 {
		t.Fatalf("yaml.v3 decoded %d documents, want 2", len(all))
	}

	// Filter out empty documents (Kind == "") — this is what the loader must do
	var results []RelationType
	for _, doc := range all {
		if doc.Kind != "" {
			results = append(results, doc)
		}
	}

	if len(results) != 1 {
		t.Fatalf("after filtering: %d documents, want 1", len(results))
	}
	if results[0].Metadata.Name != "HAS_INTERFACE" {
		t.Errorf("Metadata.Name = %q, want %q", results[0].Metadata.Name, "HAS_INTERFACE")
	}
}
