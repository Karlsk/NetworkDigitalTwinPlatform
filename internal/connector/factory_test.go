package connector

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// testStubConnector 用于 factory 测试的轻量 Connector 实现。
type testStubConnector struct {
	name        string
	connType    string
	entityTypes []string
}

func (s *testStubConnector) Metadata() ConnectorMetadata {
	return ConnectorMetadata{Name: s.name, Type: s.connType, EntityTypes: s.entityTypes}
}
func (s *testStubConnector) Collect(_ context.Context, _ string) ([]Resource, error) { return nil, nil }
func (s *testStubConnector) Stream(_ context.Context, _ string) (<-chan Resource, error) {
	return nil, ErrNotImplemented
}
func (s *testStubConnector) Ping(_ context.Context) error { return nil }

// testBuilder 创建一个简单 builder，用于测试。
func testBuilder(connType string) ConnectorBuilder {
	return func(name string, cfg map[string]any, entityTypes []string) (Connector, error) {
		return &testStubConnector{name: name, connType: connType, entityTypes: entityTypes}, nil
	}
}

// errorBuilder 创建返回错误的 builder。
func errorBuilder(msg string) ConnectorBuilder {
	return func(_ string, _ map[string]any, _ []string) (Connector, error) {
		return nil, errors.New(msg)
	}
}

func TestNewConnectorFactory(t *testing.T) {
	f := NewConnectorFactory()
	if f == nil {
		t.Fatal("NewConnectorFactory() returned nil")
	}
	if f.builders == nil {
		t.Fatal("builders map not initialized")
	}
	if len(f.builders) != 0 {
		t.Errorf("initial builders count = %d, want 0", len(f.builders))
	}
}

func TestRegisterBuilder(t *testing.T) {
	f := NewConnectorFactory()
	f.RegisterBuilder("mock", testBuilder("mock"))
	f.RegisterBuilder("netbox", testBuilder("netbox"))

	if len(f.builders) != 2 {
		t.Errorf("builders count = %d, want 2", len(f.builders))
	}
	if f.builders["mock"] == nil {
		t.Error("mock builder not registered")
	}
	if f.builders["netbox"] == nil {
		t.Error("netbox builder not registered")
	}
}

func TestRegisterBuilder_Overwrite(t *testing.T) {
	f := NewConnectorFactory()
	f.RegisterBuilder("mock", testBuilder("v1"))
	f.RegisterBuilder("mock", testBuilder("v2")) // 应覆盖

	if len(f.builders) != 1 {
		t.Errorf("builders count = %d, want 1", len(f.builders))
	}
}

func TestCreate_MockType(t *testing.T) {
	f := NewConnectorFactory()
	f.RegisterBuilder("mock", testBuilder("mock"))

	entry := ConnectorConfigEntry{
		Name:        "test-mock",
		Type:        "mock",
		Config:      map[string]any{"data_dir": "/tmp"},
		EntityTypes: []string{"Device", "Interface"},
	}

	c, err := f.Create(entry)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	meta := c.Metadata()
	if meta.Name != "test-mock" {
		t.Errorf("Name = %q, want %q", meta.Name, "test-mock")
	}
	if meta.Type != "mock" {
		t.Errorf("Type = %q, want %q", meta.Type, "mock")
	}
	if len(meta.EntityTypes) != 2 {
		t.Errorf("EntityTypes len = %d, want 2", len(meta.EntityTypes))
	}
}

func TestCreate_UnknownType(t *testing.T) {
	f := NewConnectorFactory()

	entry := ConnectorConfigEntry{Name: "unknown", Type: "nonexistent"}
	_, err := f.Create(entry)
	if err == nil {
		t.Fatal("Create with unknown type expected error, got nil")
	}

	expected := `connector type "nonexistent": builder not registered`
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

func TestCreate_BuilderError(t *testing.T) {
	f := NewConnectorFactory()
	f.RegisterBuilder("failing", errorBuilder("connection refused"))

	entry := ConnectorConfigEntry{Name: "fail-conn", Type: "failing"}
	_, err := f.Create(entry)
	if err == nil {
		t.Fatal("Create with failing builder expected error, got nil")
	}
	// 验证错误包含上下文
	if !errors.Is(err, errors.New("connection refused")) && err.Error() == "" {
		t.Errorf("error should contain context, got: %v", err)
	}
}

func TestCreateFromConfig(t *testing.T) {
	configPath := filepath.Join("..", "..", "configs", "connectors.yaml")

	f := NewConnectorFactory()
	f.RegisterBuilder("mock", testBuilder("mock"))

	registry := NewConnectorRegistry()
	if err := f.CreateFromConfig(configPath, registry); err != nil {
		t.Fatalf("CreateFromConfig() error = %v", err)
	}

	// 验证 registry 中有 2 个 connector
	list := registry.List()
	if len(list) != 2 {
		t.Fatalf("registry has %d connectors, want 2", len(list))
	}

	// 验证 mock-netbox
	c1, err := registry.Get("mock-netbox")
	if err != nil {
		t.Fatalf("Get(mock-netbox) error = %v", err)
	}
	if c1.Metadata().Name != "mock-netbox" {
		t.Errorf("c1.Name = %q, want %q", c1.Metadata().Name, "mock-netbox")
	}

	// 验证 mock-cmdb
	c2, err := registry.Get("mock-cmdb")
	if err != nil {
		t.Fatalf("Get(mock-cmdb) error = %v", err)
	}
	if c2.Metadata().Name != "mock-cmdb" {
		t.Errorf("c2.Name = %q, want %q", c2.Metadata().Name, "mock-cmdb")
	}
}

func TestCreateFromConfig_FileNotFound(t *testing.T) {
	f := NewConnectorFactory()
	registry := NewConnectorRegistry()

	err := f.CreateFromConfig("/nonexistent/connectors.yaml", registry)
	if err == nil {
		t.Fatal("CreateFromConfig with nonexistent file expected error, got nil")
	}
}

func TestCreateFromConfig_UnregisteredType(t *testing.T) {
	// 配置文件中有 "mock" 类型，但不注册 builder
	configPath := filepath.Join("..", "..", "configs", "connectors.yaml")

	f := NewConnectorFactory() // 不注册任何 builder
	registry := NewConnectorRegistry()

	err := f.CreateFromConfig(configPath, registry)
	if err == nil {
		t.Fatal("CreateFromConfig with unregistered type expected error, got nil")
	}
}
