package config

import (
	"os"
	"path/filepath"
	"testing"
)

// fullYAML 包含所有配置项的完整 YAML 内容
const fullYAML = `
neo4j:
  uri: "bolt://test-host:7687"
  user: "testuser"
  password: "testpass"
  default_db: "testdb"

server:
  port: 9090

snapshot:
  dir: "/tmp/test-snapshots"
  max_active: 10
  retention_days: 30

schema:
  ontology_dir: "/tmp/test-ontology"

channel:
  buffer_size: 200

kafka:
  enabled: true
  brokers: ["kafka1:9092", "kafka2:9092"]
  topic: "test-events"
  group_id: "test-group"
  sasl_user: "testuser"
  sasl_pass: "testpass"

event_bus:
  mode: "kafka"
  kafka:
    brokers: ["eb-kafka1:9092"]
    topic: "eb-events"
    group_id: "eb-group"
    sasl_user: "eb-user"
    sasl_pass: "eb-pass"
`

// writeTempConfig 将内容写入临时目录并返回文件路径
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoad_ValidConfig(t *testing.T) {
	path := writeTempConfig(t, fullYAML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Neo4J
	if cfg.Neo4J.URI != "bolt://test-host:7687" {
		t.Errorf("Neo4J.URI = %q, want %q", cfg.Neo4J.URI, "bolt://test-host:7687")
	}
	if cfg.Neo4J.User != "testuser" {
		t.Errorf("Neo4J.User = %q, want %q", cfg.Neo4J.User, "testuser")
	}
	if cfg.Neo4J.Password != "testpass" {
		t.Errorf("Neo4J.Password = %q, want %q", cfg.Neo4J.Password, "testpass")
	}
	if cfg.Neo4J.DefaultDB != "testdb" {
		t.Errorf("Neo4J.DefaultDB = %q, want %q", cfg.Neo4J.DefaultDB, "testdb")
	}

	// Server
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 9090)
	}

	// Snapshot
	if cfg.Snapshot.Dir != "/tmp/test-snapshots" {
		t.Errorf("Snapshot.Dir = %q, want %q", cfg.Snapshot.Dir, "/tmp/test-snapshots")
	}
	if cfg.Snapshot.MaxActive != 10 {
		t.Errorf("Snapshot.MaxActive = %d, want %d", cfg.Snapshot.MaxActive, 10)
	}
	if cfg.Snapshot.RetentionDays != 30 {
		t.Errorf("Snapshot.RetentionDays = %d, want %d", cfg.Snapshot.RetentionDays, 30)
	}

	// Schema
	if cfg.Schema.OntologyDir != "/tmp/test-ontology" {
		t.Errorf("Schema.OntologyDir = %q, want %q", cfg.Schema.OntologyDir, "/tmp/test-ontology")
	}

	// Channel
	if cfg.Channel.BufferSize != 200 {
		t.Errorf("Channel.BufferSize = %d, want %d", cfg.Channel.BufferSize, 200)
	}

	// Kafka (DataSource Layer)
	if !cfg.Kafka.Enabled {
		t.Errorf("Kafka.Enabled = %v, want true", cfg.Kafka.Enabled)
	}
	if len(cfg.Kafka.Brokers) != 2 {
		t.Errorf("Kafka.Brokers count = %d, want 2", len(cfg.Kafka.Brokers))
	}
	if cfg.Kafka.Topic != "test-events" {
		t.Errorf("Kafka.Topic = %q, want %q", cfg.Kafka.Topic, "test-events")
	}
	if cfg.Kafka.GroupID != "test-group" {
		t.Errorf("Kafka.GroupID = %q, want %q", cfg.Kafka.GroupID, "test-group")
	}

	// EventBus Layer
	if cfg.EventBus.Mode != "kafka" {
		t.Errorf("EventBus.Mode = %q, want %q", cfg.EventBus.Mode, "kafka")
	}
	if len(cfg.EventBus.Kafka.Brokers) != 1 {
		t.Errorf("EventBus.Kafka.Brokers count = %d, want 1", len(cfg.EventBus.Kafka.Brokers))
	}
	if cfg.EventBus.Kafka.Topic != "eb-events" {
		t.Errorf("EventBus.Kafka.Topic = %q, want %q", cfg.EventBus.Kafka.Topic, "eb-events")
	}
	if cfg.EventBus.Kafka.GroupID != "eb-group" {
		t.Errorf("EventBus.Kafka.GroupID = %q, want %q", cfg.EventBus.Kafka.GroupID, "eb-group")
	}
}

func TestLoad_Defaults(t *testing.T) {
	// 空 YAML，只有注释，触发所有默认值
	path := writeTempConfig(t, "# empty config\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Neo4J.DefaultDB != "default" {
		t.Errorf("Neo4J.DefaultDB default = %q, want %q", cfg.Neo4J.DefaultDB, "default")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port default = %d, want %d", cfg.Server.Port, 8080)
	}
	if cfg.Snapshot.Dir != "snapshots" {
		t.Errorf("Snapshot.Dir default = %q, want %q", cfg.Snapshot.Dir, "snapshots")
	}
	if cfg.Snapshot.MaxActive != 5 {
		t.Errorf("Snapshot.MaxActive default = %d, want %d", cfg.Snapshot.MaxActive, 5)
	}
	if cfg.Snapshot.RetentionDays != 0 {
		t.Errorf("Snapshot.RetentionDays default = %d, want %d", cfg.Snapshot.RetentionDays, 0)
	}
	if cfg.Schema.OntologyDir != "ontology" {
		t.Errorf("Schema.OntologyDir default = %q, want %q", cfg.Schema.OntologyDir, "ontology")
	}
	if cfg.Channel.BufferSize != 100 {
		t.Errorf("Channel.BufferSize default = %d, want %d", cfg.Channel.BufferSize, 100)
	}
	// Kafka defaults (DataSource Layer)
	if cfg.Kafka.Enabled != false {
		t.Errorf("Kafka.Enabled default = %v, want false", cfg.Kafka.Enabled)
	}
	if cfg.Kafka.Topic != "external-sync-events" {
		t.Errorf("Kafka.Topic default = %q, want %q", cfg.Kafka.Topic, "external-sync-events")
	}
	if cfg.Kafka.GroupID != "network-twin" {
		t.Errorf("Kafka.GroupID default = %q, want %q", cfg.Kafka.GroupID, "network-twin")
	}
	// EventBus defaults
	if cfg.EventBus.Mode != "channel" {
		t.Errorf("EventBus.Mode default = %q, want %q", cfg.EventBus.Mode, "channel")
	}
	if cfg.EventBus.Kafka.Topic != "internal-sync-events" {
		t.Errorf("EventBus.Kafka.Topic default = %q, want %q", cfg.EventBus.Kafka.Topic, "internal-sync-events")
	}
	if cfg.EventBus.Kafka.GroupID != "network-twin" {
		t.Errorf("EventBus.Kafka.GroupID default = %q, want %q", cfg.EventBus.Kafka.GroupID, "network-twin")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	path := writeTempConfig(t, fullYAML)

	// 设置环境变量覆盖 NEO4J_URI
	t.Setenv("NEO4J_URI", "bolt://env-host:7687")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Neo4J.URI != "bolt://env-host:7687" {
		t.Errorf("Neo4J.URI after env override = %q, want %q", cfg.Neo4J.URI, "bolt://env-host:7687")
	}
	// 未覆盖的字段保持不变
	if cfg.Neo4J.User != "testuser" {
		t.Errorf("Neo4J.User should remain %q, got %q", "testuser", cfg.Neo4J.User)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("Load() should return error for nonexistent file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTempConfig(t, "{{invalid yaml content::: [}")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() should return error for invalid YAML")
	}
}
