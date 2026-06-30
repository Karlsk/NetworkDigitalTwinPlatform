package connector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConnectorConfig(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "connectors.yaml")
	entries, err := LoadConnectorConfig(path)
	if err != nil {
		t.Fatalf("LoadConnectorConfig() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("LoadConnectorConfig() returned %d entries, want 2", len(entries))
	}

	// 验证第一个 entry: mock-netbox
	e := entries[0]
	if e.Name != "mock-netbox" {
		t.Errorf("entries[0].Name = %q, want %q", e.Name, "mock-netbox")
	}
	if e.Type != "mock" {
		t.Errorf("entries[0].Type = %q, want %q", e.Type, "mock")
	}
	if e.Config["data_dir"] != "testdata/mock_netbox" {
		t.Errorf("entries[0].Config[data_dir] = %v, want %q", e.Config["data_dir"], "testdata/mock_netbox")
	}
	if len(e.EntityTypes) != 2 || e.EntityTypes[0] != "Device" || e.EntityTypes[1] != "Interface" {
		t.Errorf("entries[0].EntityTypes = %v, want [Device Interface]", e.EntityTypes)
	}

	// 验证第二个 entry: mock-cmdb
	e2 := entries[1]
	if e2.Name != "mock-cmdb" {
		t.Errorf("entries[1].Name = %q, want %q", e2.Name, "mock-cmdb")
	}
	if e2.Type != "mock" {
		t.Errorf("entries[1].Type = %q, want %q", e2.Type, "mock")
	}
	if e2.Config["data_dir"] != "testdata/mock_cmdb" {
		t.Errorf("entries[1].Config[data_dir] = %v, want %q", e2.Config["data_dir"], "testdata/mock_cmdb")
	}
	if len(e2.EntityTypes) != 3 {
		t.Fatalf("entries[1].EntityTypes len = %d, want 3", len(e2.EntityTypes))
	}
}

func TestLoadConnectorConfig_FileNotFound(t *testing.T) {
	_, err := LoadConnectorConfig("/nonexistent/connectors.yaml")
	if err == nil {
		t.Fatal("LoadConnectorConfig with nonexistent file expected error, got nil")
	}
}

func TestLoadConnectorConfig_EmptyFile(t *testing.T) {
	// 创建临时空文件
	dir := t.TempDir()
	emptyPath := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	entries, err := LoadConnectorConfig(emptyPath)
	if err != nil {
		t.Fatalf("LoadConnectorConfig() on empty file error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("LoadConnectorConfig() on empty file returned %d entries, want 0", len(entries))
	}
}

func TestLoadConnectorConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(badPath, []byte("{{invalid yaml}}"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	_, err := LoadConnectorConfig(badPath)
	if err == nil {
		t.Fatal("LoadConnectorConfig with invalid YAML expected error, got nil")
	}
}

func TestConnectorConfigEntryFields(t *testing.T) {
	// 零值验证
	entry := ConnectorConfigEntry{}
	if entry.Name != "" {
		t.Errorf("zero-value Name = %q, want empty", entry.Name)
	}
	if entry.Type != "" {
		t.Errorf("zero-value Type = %q, want empty", entry.Type)
	}
	if entry.Config != nil {
		t.Errorf("zero-value Config = %v, want nil", entry.Config)
	}
	if entry.EntityTypes != nil {
		t.Errorf("zero-value EntityTypes = %v, want nil", entry.EntityTypes)
	}

	// 赋值验证
	entry2 := ConnectorConfigEntry{
		Name:        "test-conn",
		Type:        "netbox",
		Config:      map[string]any{"base_url": "http://example.com"},
		EntityTypes: []string{"Device"},
		Auth:        AuthConfig{Type: "token", TokenEnv: "NETBOX_TOKEN"},
	}
	if entry2.Name != "test-conn" {
		t.Errorf("Name = %q, want %q", entry2.Name, "test-conn")
	}
	if entry2.Type != "netbox" {
		t.Errorf("Type = %q, want %q", entry2.Type, "netbox")
	}
	if entry2.Config["base_url"] != "http://example.com" {
		t.Errorf("Config[base_url] = %v, want %q", entry2.Config["base_url"], "http://example.com")
	}
}

func TestAuthConfigFields(t *testing.T) {
	// 零值验证
	auth := AuthConfig{}
	if auth.Type != "" {
		t.Errorf("zero-value Type = %q, want empty", auth.Type)
	}
	if auth.TokenEnv != "" {
		t.Errorf("zero-value TokenEnv = %q, want empty", auth.TokenEnv)
	}
	if auth.Username != "" {
		t.Errorf("zero-value Username = %q, want empty", auth.Username)
	}
	if auth.PasswordEnv != "" {
		t.Errorf("zero-value PasswordEnv = %q, want empty", auth.PasswordEnv)
	}

	// 赋值验证
	auth2 := AuthConfig{
		Type:        "basic",
		TokenEnv:    "MY_TOKEN",
		Username:    "admin",
		PasswordEnv: "MY_PASSWORD",
	}
	if auth2.Type != "basic" {
		t.Errorf("Type = %q, want %q", auth2.Type, "basic")
	}
	if auth2.TokenEnv != "MY_TOKEN" {
		t.Errorf("TokenEnv = %q, want %q", auth2.TokenEnv, "MY_TOKEN")
	}
	if auth2.Username != "admin" {
		t.Errorf("Username = %q, want %q", auth2.Username, "admin")
	}
	if auth2.PasswordEnv != "MY_PASSWORD" {
		t.Errorf("PasswordEnv = %q, want %q", auth2.PasswordEnv, "MY_PASSWORD")
	}
}
