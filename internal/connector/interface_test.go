package connector

import (
	"context"
	"errors"
	"testing"
)

// --- Compile-time interface satisfaction check ---

// mockConnector implements Connector for testing.
type mockConnector struct {
	meta      ConnectorMetadata
	resources []Resource
}

func (m *mockConnector) Metadata() ConnectorMetadata {
	return m.meta
}

func (m *mockConnector) Collect(_ context.Context, _ string) ([]Resource, error) {
	return m.resources, nil
}

func (m *mockConnector) Stream(_ context.Context, _ string) (<-chan Resource, error) {
	return nil, ErrNotImplemented
}

// Compile-time check: mockConnector must satisfy Connector interface.
var _ Connector = (*mockConnector)(nil)

// --- Connector interface method count verification ---

func TestConnectorInterfaceMethodCount(t *testing.T) {
	// This test documents the expected 3 methods.
	// If a method is added or removed, the mockConnector above
	// will fail to compile, and this test serves as documentation.
	c := &mockConnector{
		meta: ConnectorMetadata{Name: "test", Type: "test", EntityTypes: []string{"Device"}},
	}
	// Method 1: Metadata
	_ = c.Metadata()
	// Method 2: Collect
	_, _ = c.Collect(context.Background(), "Device")
	// Method 3: Stream
	_, _ = c.Stream(context.Background(), "Device")
}

// --- Sentinel error tests ---

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{
			name: "ErrNotImplemented",
			err:  ErrNotImplemented,
			msg:  "not implemented",
		},
		{
			name: "ErrConnectorNotFound",
			err:  ErrConnectorNotFound,
			msg:  "connector not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.msg {
				t.Errorf("Error() = %q, want %q", tt.err.Error(), tt.msg)
			}
		})
	}
}

func TestSentinelErrorsWrapping(t *testing.T) {
	// ErrNotImplemented should be detectable via errors.Is after wrapping
	wrapped := errors.Join(errors.New("stream failed"), ErrNotImplemented)
	if !errors.Is(wrapped, ErrNotImplemented) {
		t.Error("errors.Is(wrapped, ErrNotImplemented) = false, want true")
	}

	// ErrConnectorNotFound should be detectable via errors.Is after wrapping
	wrapped2 := errors.Join(errors.New("lookup failed"), ErrConnectorNotFound)
	if !errors.Is(wrapped2, ErrConnectorNotFound) {
		t.Error("errors.Is(wrapped2, ErrConnectorNotFound) = false, want true")
	}
}

func TestStreamReturnsErrNotImplemented(t *testing.T) {
	c := &mockConnector{
		meta: ConnectorMetadata{Name: "test", Type: "mock"},
	}
	ch, err := c.Stream(context.Background(), "Device")
	if ch != nil {
		t.Error("expected nil channel from unimplemented Stream")
	}
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Stream error = %v, want ErrNotImplemented", err)
	}
}

// --- ConnectorRegistry tests ---

func TestNewConnectorRegistry(t *testing.T) {
	r := NewConnectorRegistry()
	if r == nil {
		t.Fatal("NewConnectorRegistry() returned nil")
	}
	if r.connectors == nil {
		t.Fatal("connectors map not initialized")
	}
}

func TestConnectorRegistryRegisterAndGet(t *testing.T) {
	r := NewConnectorRegistry()

	c := &mockConnector{
		meta: ConnectorMetadata{
			Name:        "mock-netbox",
			Type:        "mock",
			EntityTypes: []string{"Device"},
		},
	}

	r.Register(c)

	got, err := r.Get("mock-netbox")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Metadata().Name != "mock-netbox" {
		t.Errorf("Get().Metadata().Name = %q, want %q", got.Metadata().Name, "mock-netbox")
	}
}

func TestConnectorRegistryGetNotFound(t *testing.T) {
	r := NewConnectorRegistry()

	_, err := r.Get("nonexistent")
	if !errors.Is(err, ErrConnectorNotFound) {
		t.Errorf("Get() error = %v, want ErrConnectorNotFound", err)
	}
}

func TestConnectorRegistryList(t *testing.T) {
	r := NewConnectorRegistry()

	connectors := []*mockConnector{
		{meta: ConnectorMetadata{Name: "netbox", Type: "netbox", EntityTypes: []string{"Device"}}},
		{meta: ConnectorMetadata{Name: "controller", Type: "controller", EntityTypes: []string{"Device", "Interface"}}},
		{meta: ConnectorMetadata{Name: "cmdb", Type: "cmdb", EntityTypes: []string{"Device"}}},
	}

	for _, c := range connectors {
		r.Register(c)
	}

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("List() len = %d, want 3", len(list))
	}

	// Verify all names are present (order not guaranteed)
	names := make(map[string]bool)
	for _, m := range list {
		names[m.Name] = true
	}
	for _, expected := range []string{"netbox", "controller", "cmdb"} {
		if !names[expected] {
			t.Errorf("List() missing connector %q", expected)
		}
	}
}

func TestConnectorRegistryListEmpty(t *testing.T) {
	r := NewConnectorRegistry()
	list := r.List()
	if len(list) != 0 {
		t.Errorf("List() on empty registry = %d items, want 0", len(list))
	}
}

func TestConnectorRegistryDuplicateOverwrites(t *testing.T) {
	r := NewConnectorRegistry()

	c1 := &mockConnector{
		meta:      ConnectorMetadata{Name: "test", Type: "v1"},
		resources: []Resource{{Kind: "Device", ID: "old"}},
	}
	c2 := &mockConnector{
		meta:      ConnectorMetadata{Name: "test", Type: "v2"},
		resources: []Resource{{Kind: "Device", ID: "new"}},
	}

	r.Register(c1)
	r.Register(c2) // Should overwrite c1

	got, err := r.Get("test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Metadata().Type != "v2" {
		t.Errorf("Get().Metadata().Type = %q, want %q (last-write-wins)", got.Metadata().Type, "v2")
	}

	// Verify only one entry in list
	list := r.List()
	if len(list) != 1 {
		t.Errorf("List() len = %d, want 1 (duplicate name should overwrite)", len(list))
	}
}

func TestConnectorRegistryCollectIntegration(t *testing.T) {
	r := NewConnectorRegistry()

	resources := []Resource{
		{Kind: "Device", ID: "d1", Properties: map[string]any{"hostname": "r1"}},
		{Kind: "Device", ID: "d2", Properties: map[string]any{"hostname": "r2"}},
	}

	c := &mockConnector{
		meta:      ConnectorMetadata{Name: "mock", Type: "mock", EntityTypes: []string{"Device"}},
		resources: resources,
	}
	r.Register(c)

	got, err := r.Get("mock")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	collected, err := got.Collect(context.Background(), "Device")
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(collected) != 2 {
		t.Fatalf("Collect() returned %d resources, want 2", len(collected))
	}
	if collected[0].ID != "d1" {
		t.Errorf("collected[0].ID = %q, want %q", collected[0].ID, "d1")
	}
}
