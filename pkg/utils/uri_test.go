package utils

import "testing"

func TestGenerateURI_SingleKey(t *testing.T) {
	props := map[string]any{"serial_number": "SN12345"}
	uri, err := GenerateURI("device:{serial_number}", []string{"serial_number"}, props)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "device:SN12345" {
		t.Errorf("URI = %q, want %q", uri, "device:SN12345")
	}
}

func TestGenerateURI_CompositeKeys(t *testing.T) {
	props := map[string]any{"device_serial": "SN12345", "if_name": "GE1/0/1"}
	uri, err := GenerateURI("iface:{device_serial}_{if_name}", []string{"device_serial", "if_name"}, props)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "iface:SN12345_GE1/0/1" {
		t.Errorf("URI = %q, want %q", uri, "iface:SN12345_GE1/0/1")
	}
}

func TestGenerateURI_MissingKey(t *testing.T) {
	props := map[string]any{"hostname": "router-01"}
	_, err := GenerateURI("device:{serial_number}", []string{"serial_number"}, props)
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestGenerateURI_NilValue(t *testing.T) {
	props := map[string]any{"serial_number": nil}
	_, err := GenerateURI("device:{serial_number}", []string{"serial_number"}, props)
	if err == nil {
		t.Fatal("expected error for nil value, got nil")
	}
}

func TestGenerateURI_EmptyValue(t *testing.T) {
	props := map[string]any{"serial_number": ""}
	_, err := GenerateURI("device:{serial_number}", []string{"serial_number"}, props)
	if err == nil {
		t.Fatal("expected error for empty value, got nil")
	}
}

func TestGenerateURI_NonStringValue(t *testing.T) {
	props := map[string]any{"port": 42}
	uri, err := GenerateURI("port:{port}", []string{"port"}, props)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "port:42" {
		t.Errorf("URI = %q, want %q", uri, "port:42")
	}
}

func TestGenerateURI_NoKeys(t *testing.T) {
	props := map[string]any{}
	uri, err := GenerateURI("static:template", nil, props)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "static:template" {
		t.Errorf("URI = %q, want %q", uri, "static:template")
	}
}

func TestGenerateURI_SpecialCharsInValue(t *testing.T) {
	props := map[string]any{"if_name": "GE1/0/1"}
	uri, err := GenerateURI("iface:{if_name}", []string{"if_name"}, props)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "iface:GE1/0/1" {
		t.Errorf("URI = %q, want %q", uri, "iface:GE1/0/1")
	}
}
