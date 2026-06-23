package normalizer

import "testing"

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
