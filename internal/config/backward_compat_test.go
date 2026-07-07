package config

import (
	"testing"
)

// TestBackwardCompatibility_NoPostgresSection 验证不含 postgres 段的配置文件仍能正确加载。
func TestBackwardCompatibility_NoPostgresSection(t *testing.T) {
	// 模拟 V1 配置文件（不含 postgres 段）
	v1YAML := `
neo4j:
  uri: "bolt://localhost:7687"
server:
  port: 8080
snapshot:
  dir: "snapshots"
  max_active: 5
schema:
  ontology_dir: "ontology"
`
	path := writeTempConfig(t, v1YAML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() with V1 config (no postgres section) should not fail: %v", err)
	}

	// 验证 postgres 使用默认值
	if cfg.Postgres.Enabled != false {
		t.Errorf("Postgres.Enabled should default to false, got %v", cfg.Postgres.Enabled)
	}
	if cfg.Postgres.MaxConns != 10 {
		t.Errorf("Postgres.MaxConns should default to 10, got %d", cfg.Postgres.MaxConns)
	}
	if cfg.Postgres.MinConns != 2 {
		t.Errorf("Postgres.MinConns should default to 2, got %d", cfg.Postgres.MinConns)
	}

	// 验证 Neo4j 等现有配置不受影响
	if cfg.Neo4J.URI != "bolt://localhost:7687" {
		t.Errorf("Neo4J.URI = %q, want %q", cfg.Neo4J.URI, "bolt://localhost:7687")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
}

// TestBackwardCompatibility_PostgresEnvOverrides 验证 postgres 环境变量覆盖。
func TestBackwardCompatibility_PostgresEnvOverrides(t *testing.T) {
	v1YAML := `
neo4j:
  uri: "bolt://localhost:7687"
server:
  port: 8080
`
	path := writeTempConfig(t, v1YAML)

	t.Setenv("POSTGRES_URL", "postgres://env-test@remote:5432/db")
	t.Setenv("POSTGRES_ENABLED", "true")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Postgres.URL != "postgres://env-test@remote:5432/db" {
		t.Errorf("POSTGRES_URL env override = %q, want %q", cfg.Postgres.URL, "postgres://env-test@remote:5432/db")
	}
	if !cfg.Postgres.Enabled {
		t.Errorf("POSTGRES_ENABLED env override = %v, want true", cfg.Postgres.Enabled)
	}
}

// TestBackwardCompatibility_ExistingConfigUnchanged 验证现有配置文件加载后 postgres 段正确解析。
func TestBackwardCompatibility_ExistingConfigUnchanged(t *testing.T) {
	// 用包含 postgres 段的完整配置加载，验证现有配置段不受影响
	path := writeTempConfig(t, fullYAML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// postgres 段应被正确解析
	if !cfg.Postgres.Enabled {
		t.Errorf("postgres.enabled = %v, want true", cfg.Postgres.Enabled)
	}

	// 现有配置不应受影响
	if cfg.Neo4J.URI != "bolt://test-host:7687" {
		t.Errorf("neo4j.uri = %q, want %q", cfg.Neo4J.URI, "bolt://test-host:7687")
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("server.port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Kafka.Topic != "test-events" {
		t.Errorf("kafka.topic = %q, want %q", cfg.Kafka.Topic, "test-events")
	}
	if cfg.EventBus.Mode != "kafka" {
		t.Errorf("event_bus.mode = %q, want %q", cfg.EventBus.Mode, "kafka")
	}
}
