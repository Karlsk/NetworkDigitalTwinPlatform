package connector

import (
	"testing"
	"time"
)

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
		BaseURL:     "http://netbox.local/api",
		Timeout:     10 * time.Second,
		AuthType:    "token",
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
	if meta.BaseURL != "http://netbox.local/api" {
		t.Errorf("BaseURL = %q, want %q", meta.BaseURL, "http://netbox.local/api")
	}
	if meta.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want %v", meta.Timeout, 10*time.Second)
	}
	if meta.AuthType != "token" {
		t.Errorf("AuthType = %q, want %q", meta.AuthType, "token")
	}
}

func TestConnectorMetadataEmptyEntityTypes(t *testing.T) {
	meta := ConnectorMetadata{Name: "empty", Type: "test"}
	if meta.EntityTypes != nil {
		t.Errorf("expected nil EntityTypes for zero-value, got %v", meta.EntityTypes)
	}
}

func TestConnectorMetadataNewFields(t *testing.T) {
	// 验证新字段默认值（零值）
	meta := ConnectorMetadata{Name: "zero", Type: "mock"}
	if meta.BaseURL != "" {
		t.Errorf("BaseURL zero-value = %q, want empty", meta.BaseURL)
	}
	if meta.Timeout != 0 {
		t.Errorf("Timeout zero-value = %v, want 0", meta.Timeout)
	}
	if meta.AuthType != "" {
		t.Errorf("AuthType zero-value = %q, want empty", meta.AuthType)
	}

	// 验证新字段赋值
	meta2 := ConnectorMetadata{
		Name:     "rest-connector",
		Type:     "netbox",
		BaseURL:  "https://netbox.example.com/api",
		Timeout:  30 * time.Second,
		AuthType: "basic",
	}
	if meta2.BaseURL != "https://netbox.example.com/api" {
		t.Errorf("BaseURL = %q, want %q", meta2.BaseURL, "https://netbox.example.com/api")
	}
	if meta2.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want %v", meta2.Timeout, 30*time.Second)
	}
	if meta2.AuthType != "basic" {
		t.Errorf("AuthType = %q, want %q", meta2.AuthType, "basic")
	}
}
