package netbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// newTestServer creates a httptest.Server simulating Netbox REST API.
// routes maps URL paths to handler functions.
func newTestServer(t *testing.T, routes map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, handler := range routes {
		mux.HandleFunc(path, handler)
	}
	return httptest.NewServer(mux)
}

// newTestConnector creates a NetboxConnector with the given httptest.Server.
func newTestConnector(t *testing.T, srv *httptest.Server) *NetboxConnector {
	t.Helper()
	client := connector.NewHTTPClient(
		connector.WithBaseURL(srv.URL),
		connector.WithRateLimit(100),
	)
	return NewNetboxConnector("test-netbox", client, []string{"Device", "Interface"})
}

// ============================================================
// TC-NB01: Metadata
// ============================================================

func TestMetadata(t *testing.T) {
	client := connector.NewHTTPClient(connector.WithBaseURL("http://example.com"))
	c := NewNetboxConnector("my-netbox", client, []string{"Device", "Interface"})

	meta := c.Metadata()
	if meta.Name != "my-netbox" {
		t.Errorf("Name = %q, want %q", meta.Name, "my-netbox")
	}
	if meta.Type != "netbox" {
		t.Errorf("Type = %q, want %q", meta.Type, "netbox")
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

// ============================================================
// TC-NB02: Ping OK
// ============================================================

func TestPing_OK(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"/api/status/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"netbox-version":"3.6.0"}`))
		},
	})
	defer srv.Close()

	c := newTestConnector(t, srv)
	err := c.Ping(context.Background())
	if err != nil {
		t.Errorf("Ping() error = %v, want nil", err)
	}
}

// ============================================================
// TC-NB03: Ping Unreachable
// ============================================================

func TestPing_Unreachable(t *testing.T) {
	client := connector.NewHTTPClient(
		connector.WithBaseURL("http://127.0.0.1:1"),
		connector.WithRateLimit(100),
	)
	c := NewNetboxConnector("bad-netbox", client, []string{"Device"})

	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("Ping() expected error for unreachable URL, got nil")
	}
}

// ============================================================
// TC-NB04: Ping Non-200
// ============================================================

func TestPing_Non200(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"/api/status/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		},
	})
	defer srv.Close()

	c := newTestConnector(t, srv)
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("Ping() expected error for non-200 status, got nil")
	}
	// Verify error contains context
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("Ping() error = %q, want to contain status code 403", err.Error())
	}
}

// ============================================================
// TC-NB05: Collect Device
// ============================================================

func TestCollect_Device(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"/api/dcim/devices/": func(w http.ResponseWriter, r *http.Request) {
			resp := connector.PageResult{
				Results: []map[string]any{
					{
						"id":          float64(1),
						"serial":      "SN001",
						"name":        "router-01",
						"status":      "active",
						"device_type": map[string]any{"slug": "csr1000v"},
						"site":        map[string]any{"name": "DC-Beijing"},
					},
					{
						"id":          float64(2),
						"serial":      "SN002",
						"name":        "router-02",
						"status":      "active",
						"device_type": map[string]any{"slug": "mx240"},
						"site":        map[string]any{"name": "DC-Shanghai"},
					},
				},
				Next:  "",
				Count: 2,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	})
	defer srv.Close()

	c := newTestConnector(t, srv)
	resources, err := c.Collect(context.Background(), "Device")
	if err != nil {
		t.Fatalf("Collect(Device) error = %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("Collect(Device) returned %d resources, want 2", len(resources))
	}

	// Verify first resource
	r0 := resources[0]
	if r0.Kind != "Device" {
		t.Errorf("resources[0].Kind = %q, want %q", r0.Kind, "Device")
	}
	if r0.ID != "1" {
		t.Errorf("resources[0].ID = %q, want %q", r0.ID, "1")
	}
	if r0.Properties["serial"] != "SN001" {
		t.Errorf("resources[0].serial = %v, want %q", r0.Properties["serial"], "SN001")
	}
	// Verify flattened fields
	if r0.Properties["device_type"] != "csr1000v" {
		t.Errorf("resources[0].device_type = %v, want %q", r0.Properties["device_type"], "csr1000v")
	}
	if r0.Properties["site"] != "DC-Beijing" {
		t.Errorf("resources[0].site = %v, want %q", r0.Properties["site"], "DC-Beijing")
	}
}

// ============================================================
// TC-NB06: Collect Interface
// ============================================================

func TestCollect_Interface(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"/api/dcim/interfaces/": func(w http.ResponseWriter, r *http.Request) {
			resp := connector.PageResult{
				Results: []map[string]any{
					{
						"id":          float64(101),
						"name":        "GigabitEthernet0/0",
						"type":        "1000base-t",
						"enabled":     true,
						"mtu":         float64(1500),
						"mac_address": "aa:bb:cc:dd:ee:ff",
						"description": "Uplink",
						"device":      map[string]any{"name": "router-01"},
					},
				},
				Next:  "",
				Count: 1,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	})
	defer srv.Close()

	c := newTestConnector(t, srv)
	resources, err := c.Collect(context.Background(), "Interface")
	if err != nil {
		t.Fatalf("Collect(Interface) error = %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("Collect(Interface) returned %d resources, want 1", len(resources))
	}

	r0 := resources[0]
	if r0.Kind != "Interface" {
		t.Errorf("Kind = %q, want %q", r0.Kind, "Interface")
	}
	if r0.ID != "101" {
		t.Errorf("ID = %q, want %q", r0.ID, "101")
	}
	if r0.Properties["name"] != "GigabitEthernet0/0" {
		t.Errorf("name = %v, want %q", r0.Properties["name"], "GigabitEthernet0/0")
	}
	// Verify flattened device_name
	if r0.Properties["device_name"] != "router-01" {
		t.Errorf("device_name = %v, want %q", r0.Properties["device_name"], "router-01")
	}
}

// ============================================================
// TC-NB07: Collect Unsupported Type
// ============================================================

func TestCollect_UnsupportedType(t *testing.T) {
	srv := newTestServer(t, nil)
	defer srv.Close()

	c := newTestConnector(t, srv)
	_, err := c.Collect(context.Background(), "UnknownEntity")
	if err == nil {
		t.Fatal("Collect(UnknownEntity) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error = %q, want to contain 'unsupported'", err.Error())
	}
}

// ============================================================
// TC-NB08: Collect Multi-Page
// ============================================================

func TestCollect_MultiPage(t *testing.T) {
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		var resp connector.PageResult
		switch n {
		case 1:
			resp = connector.PageResult{
				Results: []map[string]any{
					{"id": float64(1), "serial": "SN001", "name": "dev-1"},
					{"id": float64(2), "serial": "SN002", "name": "dev-2"},
				},
				Next:  "/api/dcim/devices/?limit=100&offset=2",
				Count: 3,
			}
		case 2:
			resp = connector.PageResult{
				Results: []map[string]any{
					{"id": float64(3), "serial": "SN003", "name": "dev-3"},
				},
				Next:  "",
				Count: 3,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestConnector(t, srv)
	resources, err := c.Collect(context.Background(), "Device")
	if err != nil {
		t.Fatalf("Collect(Device) error = %v", err)
	}
	if len(resources) != 3 {
		t.Errorf("collected %d resources, want 3", len(resources))
	}

	// Verify all IDs
	wantIDs := []string{"1", "2", "3"}
	for i, wantID := range wantIDs {
		if i >= len(resources) {
			break
		}
		if resources[i].ID != wantID {
			t.Errorf("resources[%d].ID = %q, want %q", i, resources[i].ID, wantID)
		}
	}
}

// ============================================================
// TC-NB09: Collect Empty Result
// ============================================================

func TestCollect_EmptyResult(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"/api/dcim/devices/": func(w http.ResponseWriter, r *http.Request) {
			resp := connector.PageResult{
				Results: []map[string]any{},
				Next:    "",
				Count:   0,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		},
	})
	defer srv.Close()

	c := newTestConnector(t, srv)
	resources, err := c.Collect(context.Background(), "Device")
	if err != nil {
		t.Fatalf("Collect(Device) error = %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("collected %d resources, want 0", len(resources))
	}
}

// ============================================================
// TC-NB10: Stream
// ============================================================

func TestStream(t *testing.T) {
	client := connector.NewHTTPClient(connector.WithBaseURL("http://example.com"))
	c := NewNetboxConnector("netbox", client, []string{"Device"})

	ch, err := c.Stream(context.Background(), "Device")
	if ch != nil {
		t.Error("Stream() channel should be nil")
	}
	if !errors.Is(err, connector.ErrNotImplemented) {
		t.Errorf("Stream() error = %v, want ErrNotImplemented", err)
	}
}

// ============================================================
// TC-NB11: TransformDevice
// ============================================================

func TestTransformDevice(t *testing.T) {
	raw := map[string]any{
		"id":          float64(1),
		"serial":      "SN001",
		"name":        "router-01",
		"status":      "active",
		"platform":    "ios-xe",
		"role":        "router",
		"device_type": map[string]any{"slug": "csr1000v", "model": "CSR"},
		"site":        map[string]any{"name": "DC-Beijing", "slug": "dc-bj"},
	}

	props := transformDevice(raw)

	// Direct fields
	for _, key := range []string{"serial", "name", "status", "platform", "role"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing direct field %q", key)
		}
	}

	// Flattened fields
	if props["device_type"] != "csr1000v" {
		t.Errorf("device_type = %v, want %q", props["device_type"], "csr1000v")
	}
	if props["site"] != "DC-Beijing" {
		t.Errorf("site = %v, want %q", props["site"], "DC-Beijing")
	}
}

// ============================================================
// TC-NB12: TransformInterface
// ============================================================

func TestTransformInterface(t *testing.T) {
	raw := map[string]any{
		"id":          float64(101),
		"name":        "GigabitEthernet0/0",
		"type":        "1000base-t",
		"enabled":     true,
		"mtu":         float64(1500),
		"mac_address": "aa:bb:cc:dd:ee:ff",
		"description": "Uplink to spine",
		"device":      map[string]any{"name": "router-01", "id": float64(1)},
	}

	props := transformInterface(raw)

	for _, key := range []string{"name", "type", "enabled", "mtu", "mac_address", "description"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing direct field %q", key)
		}
	}
	if props["device_name"] != "router-01" {
		t.Errorf("device_name = %v, want %q", props["device_name"], "router-01")
	}
}

// ============================================================
// TC-NB13: TransformDevice Missing Nested Fields
// ============================================================

func TestTransformDevice_MissingNested(t *testing.T) {
	// No nested fields - should not panic
	raw := map[string]any{
		"id":     float64(1),
		"serial": "SN001",
		"name":   "router-01",
	}

	props := transformDevice(raw)
	if props["serial"] != "SN001" {
		t.Errorf("serial = %v, want %q", props["serial"], "SN001")
	}
	// Nested fields should not be present
	if _, ok := props["device_type"]; ok {
		t.Error("device_type should not be present when nested field is missing")
	}
	if _, ok := props["site"]; ok {
		t.Error("site should not be present when nested field is missing")
	}
}


// Compile-time interface check
var _ connector.Connector = (*NetboxConnector)(nil)

// ============================================================
// TC-NBF01: Builder 返回有效的 ConnectorBuilder
// ============================================================

func TestBuilder(t *testing.T) {
	builder := Builder()
	if builder == nil {
		t.Fatal("Builder() returned nil")
	}

	cfg := map[string]any{
		"base_url": "http://netbox.example.com",
		"timeout":  "10s",
	}
	c, err := builder("netbox-1", cfg, []string{"Device", "Interface"})
	if err != nil {
		t.Fatalf("Builder() error = %v", err)
	}

	meta := c.Metadata()
	if meta.Name != "netbox-1" {
		t.Errorf("Name = %q, want %q", meta.Name, "netbox-1")
	}
	if meta.Type != "netbox" {
		t.Errorf("Type = %q, want %q", meta.Type, "netbox")
	}
	if len(meta.EntityTypes) != 2 {
		t.Errorf("EntityTypes len = %d, want 2", len(meta.EntityTypes))
	}
}

// ============================================================
// TC-NBF02: Builder 与 ConnectorFactory 集成
// ============================================================

func TestBuilder_WithFactory(t *testing.T) {
	factory := connector.NewConnectorFactory()
	factory.RegisterBuilder("netbox", Builder())

	entry := connector.ConnectorConfigEntry{
		Name:        "netbox-prod",
		Type:        "netbox",
		Config:      map[string]any{"base_url": "http://netbox.local"},
		EntityTypes: []string{"Device"},
	}

	c, err := factory.Create(entry)
	if err != nil {
		t.Fatalf("factory.Create() error = %v", err)
	}
	if c.Metadata().Name != "netbox-prod" {
		t.Errorf("Name = %q, want %q", c.Metadata().Name, "netbox-prod")
	}
}
