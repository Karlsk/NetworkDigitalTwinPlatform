package connector

import "testing"

func TestResourceFields(t *testing.T) {
	r := Resource{
		Kind: "Device",
		ID:   "netbox-123",
		Properties: map[string]any{
			"hostname": "router-01",
			"vendor":   "Huawei",
			"port":     8080,
		},
	}

	if r.Kind != "Device" {
		t.Errorf("Kind = %q, want %q", r.Kind, "Device")
	}
	if r.ID != "netbox-123" {
		t.Errorf("ID = %q, want %q", r.ID, "netbox-123")
	}
	if len(r.Properties) != 3 {
		t.Errorf("Properties count = %d, want 3", len(r.Properties))
	}
	if r.Properties["port"] != 8080 {
		t.Errorf("Properties[port] = %v, want 8080", r.Properties["port"])
	}
}

func TestResourceEmptyProperties(t *testing.T) {
	r := Resource{Kind: "Interface", ID: "iface-1"}
	if r.Properties != nil {
		t.Errorf("expected nil Properties for zero-value, got %v", r.Properties)
	}
}

func TestConnectorMetadataFields(t *testing.T) {
	meta := ConnectorMetadata{
		Name:        "mock-netbox",
		Type:        "mock",
		EntityTypes: []string{"Device", "Interface"},
	}

	if meta.Name != "mock-netbox" {
		t.Errorf("Name = %q, want %q", meta.Name, "mock-netbox")
	}
	if meta.Type != "mock" {
		t.Errorf("Type = %q, want %q", meta.Type, "mock")
	}
	if len(meta.EntityTypes) != 2 {
		t.Fatalf("EntityTypes len = %d, want 2", len(meta.EntityTypes))
	}
	if meta.EntityTypes[0] != "Device" {
		t.Errorf("EntityTypes[0] = %q, want %q", meta.EntityTypes[0], "Device")
	}
	if meta.EntityTypes[1] != "Interface" {
		t.Errorf("EntityTypes[1] = %q, want %q", meta.EntityTypes[1], "Interface")
	}
}

func TestConnectorMetadataEmptyEntityTypes(t *testing.T) {
	meta := ConnectorMetadata{Name: "empty", Type: "test"}
	if meta.EntityTypes != nil {
		t.Errorf("expected nil EntityTypes for zero-value, got %v", meta.EntityTypes)
	}
}
