package mock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// testdataDir returns the path to the test data directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "testdata", "mock_netbox")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("testdata directory not found at %s, skipping", dir)
	}
	return dir
}

// newTestConnector creates a MockConnector for testing with all entity types.
func newTestConnector(t *testing.T) *MockConnector {
	t.Helper()
	return NewMockConnector("mock-netbox", testdataDir(t), []string{
		"Device", "Interface", "ISIS", "Link", "Network_Slice",
	})
}

// Compile-time interface check
var _ connector.Connector = (*MockConnector)(nil)

// ============================================================
// Metadata tests
// ============================================================

func TestMetadata(t *testing.T) {
	c := NewMockConnector("mock-netbox", "/tmp/data", []string{"Device", "Interface"})
	meta := c.Metadata()

	if meta.Name != "mock-netbox" {
		t.Errorf("Metadata().Name = %q, want %q", meta.Name, "mock-netbox")
	}
	if meta.Type != "mock" {
		t.Errorf("Metadata().Type = %q, want %q", meta.Type, "mock")
	}
	if len(meta.EntityTypes) != 2 {
		t.Fatalf("Metadata().EntityTypes len = %d, want 2", len(meta.EntityTypes))
	}
	if meta.EntityTypes[0] != "Device" {
		t.Errorf("EntityTypes[0] = %q, want %q", meta.EntityTypes[0], "Device")
	}
}

// ============================================================
// Collect tests — 验收标准覆盖
// ============================================================

func TestCollect_Device(t *testing.T) {
	c := newTestConnector(t)
	resources, err := c.Collect(context.Background(), "Device")
	if err != nil {
		t.Fatalf("Collect(Device) error = %v", err)
	}
	if len(resources) != 3 {
		t.Fatalf("Collect(Device) returned %d resources, want 3", len(resources))
	}

	// 验证所有 Resource.Kind == "Device"
	for i, r := range resources {
		if r.Kind != "Device" {
			t.Errorf("resources[%d].Kind = %q, want %q", i, r.Kind, "Device")
		}
	}

	// 验证第一个设备的源字段名（mgmt_ip, hw_model）
	first := resources[0].Properties
	if _, ok := first["mgmt_ip"]; !ok {
		t.Error("expected field 'mgmt_ip' (source name) in Device properties")
	}
	if _, ok := first["hw_model"]; !ok {
		t.Error("expected field 'hw_model' (source name) in Device properties")
	}

	// 验证 interfaces 字段存在且为 URI 列表
	ifaces, ok := first["interfaces"]
	if !ok {
		t.Fatal("expected 'interfaces' relation field in Device properties")
	}
	ifaceList, ok := ifaces.([]any)
	if !ok {
		t.Fatalf("interfaces type = %T, want []any", ifaces)
	}
	if len(ifaceList) == 0 {
		t.Error("interfaces list should not be empty")
	}
	// 验证 URI 格式
	if s, ok := ifaceList[0].(string); !ok || s == "" {
		t.Errorf("interfaces[0] = %v, want non-empty URI string", ifaceList[0])
	}
}

func TestCollect_Interface(t *testing.T) {
	c := newTestConnector(t)
	resources, err := c.Collect(context.Background(), "Interface")
	if err != nil {
		t.Fatalf("Collect(Interface) error = %v", err)
	}
	if len(resources) != 12 {
		t.Fatalf("Collect(Interface) returned %d resources, want 12", len(resources))
	}

	// 验证 stableKeys 字段存在
	for i, r := range resources {
		if r.Kind != "Interface" {
			t.Errorf("resources[%d].Kind = %q, want %q", i, r.Kind, "Interface")
		}
		if _, ok := r.Properties["device_serial"]; !ok {
			t.Errorf("resources[%d] missing stableKey 'device_serial'", i)
		}
		if _, ok := r.Properties["if_name"]; !ok {
			t.Errorf("resources[%d] missing stableKey 'if_name'", i)
		}
	}
}

func TestCollect_ISIS(t *testing.T) {
	c := newTestConnector(t)
	resources, err := c.Collect(context.Background(), "ISIS")
	if err != nil {
		t.Fatalf("Collect(ISIS) error = %v", err)
	}
	if len(resources) != 3 {
		t.Fatalf("Collect(ISIS) returned %d resources, want 3", len(resources))
	}

	// 验证 run_on 关系字段为 URI 列表
	first := resources[0].Properties
	runOn, ok := first["run_on"]
	if !ok {
		t.Fatal("expected 'run_on' relation field in ISIS properties")
	}
	runOnList, ok := runOn.([]any)
	if !ok {
		t.Fatalf("run_on type = %T, want []any", runOn)
	}
	if len(runOnList) == 0 {
		t.Error("run_on list should not be empty")
	}
}

func TestCollect_Link(t *testing.T) {
	c := newTestConnector(t)
	resources, err := c.Collect(context.Background(), "Link")
	if err != nil {
		t.Fatalf("Collect(Link) error = %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("Collect(Link) returned %d resources, want 2", len(resources))
	}

	// 验证 endpoints 关系字段为 URI 列表
	first := resources[0].Properties
	endpoints, ok := first["endpoints"]
	if !ok {
		t.Fatal("expected 'endpoints' relation field in Link properties")
	}
	epList, ok := endpoints.([]any)
	if !ok {
		t.Fatalf("endpoints type = %T, want []any", endpoints)
	}
	if len(epList) != 2 {
		t.Errorf("endpoints len = %d, want 2", len(epList))
	}
}

func TestCollect_NetworkSlice(t *testing.T) {
	c := newTestConnector(t)
	resources, err := c.Collect(context.Background(), "Network_Slice")
	if err != nil {
		t.Fatalf("Collect(Network_Slice) error = %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("Collect(Network_Slice) returned %d resources, want 1", len(resources))
	}

	r := resources[0]
	if r.Kind != "Network_Slice" {
		t.Errorf("Kind = %q, want %q", r.Kind, "Network_Slice")
	}
	if r.Properties["slice_id"] != "SLICE-001" {
		t.Errorf("slice_id = %v, want %q", r.Properties["slice_id"], "SLICE-001")
	}
}

// ============================================================
// Collect error cases
// ============================================================

func TestCollect_FileNotFound(t *testing.T) {
	c := NewMockConnector("mock", "/nonexistent/path", []string{"Device"})
	_, err := c.Collect(context.Background(), "Device")
	if err == nil {
		t.Fatal("Collect with invalid dataDir expected error, got nil")
	}
}

func TestCollect_UnsupportedType(t *testing.T) {
	c := newTestConnector(t)
	_, err := c.Collect(context.Background(), "UnknownEntity")
	if err == nil {
		t.Fatal("Collect with unsupported entityType expected error, got nil")
	}
}

// ============================================================
// Resource Kind and ID
// ============================================================

func TestResourceKindAndID(t *testing.T) {
	c := newTestConnector(t)

	// Device: ID should be extracted from properties (serial_number or fallback)
	devices, err := c.Collect(context.Background(), "Device")
	if err != nil {
		t.Fatalf("Collect(Device) error = %v", err)
	}
	for i, r := range devices {
		if r.Kind != "Device" {
			t.Errorf("devices[%d].Kind = %q, want %q", i, r.Kind, "Device")
		}
		if r.ID == "" {
			t.Errorf("devices[%d].ID is empty", i)
		}
		if r.Properties == nil {
			t.Errorf("devices[%d].Properties is nil", i)
		}
	}
}

// ============================================================
// Stream tests
// ============================================================

func TestStream_ReturnsNotImplemented(t *testing.T) {
	c := newTestConnector(t)
	ch, err := c.Stream(context.Background(), "Device")
	if ch != nil {
		t.Error("Stream() channel should be nil")
	}
	if !errors.Is(err, connector.ErrNotImplemented) {
		t.Errorf("Stream() error = %v, want ErrNotImplemented", err)
	}
}

// ============================================================
// Ping tests
// ============================================================

func TestPing(t *testing.T) {
	c := newTestConnector(t)
	err := c.Ping(context.Background())
	if err != nil {
		t.Errorf("Ping() = %v, want nil", err)
	}
}

func TestPing_NilContext(t *testing.T) {
	c := NewMockConnector("mock", "/tmp", []string{"Device"})
	// 即使 dataDir 不存在，Ping 也应返回 nil（仅检查连通性）
	err := c.Ping(context.Background())
	if err != nil {
		t.Errorf("Ping() = %v, want nil", err)
	}
}
