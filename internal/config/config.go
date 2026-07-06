// Package config 提供全局配置加载功能，基于 Viper 实现 YAML 文件加载与环境变量覆盖
package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// Config 服务全局配置
type Config struct {
	Neo4J    Neo4JConfig    `mapstructure:"neo4j"`
	Server   ServerConfig   `mapstructure:"server"`
	Snapshot SnapshotConfig `mapstructure:"snapshot"`
	Schema   SchemaConfig   `mapstructure:"schema"`
	Channel  ChannelConfig  `mapstructure:"channel"`
	Kafka    KafkaConfig    `mapstructure:"kafka"`
}

// Neo4JConfig 是 Neo4j 连接配置
type Neo4JConfig struct {
	URI       string `mapstructure:"uri"`
	User      string `mapstructure:"user"`
	Password  string `mapstructure:"password"`
	DefaultDB string `mapstructure:"default_db"`
}

// ServerConfig 是 HTTP 服务配置
type ServerConfig struct {
	Port int `mapstructure:"port"`
}

// SnapshotConfig 是快照管理配置
type SnapshotConfig struct {
	Dir           string `mapstructure:"dir"`
	MaxActive     int    `mapstructure:"max_active"`
	RetentionDays int    `mapstructure:"retention_days"` // V1-20: TTL 保留天数，0 = 不自动清理
}

// SchemaConfig 是本体 Schema 配置
type SchemaConfig struct {
	OntologyDir string `mapstructure:"ontology_dir"`
}

// ChannelConfig 是事件 Channel 缓冲配置
type ChannelConfig struct {
	BufferSize int `mapstructure:"buffer_size"`
}

// KafkaConfig Kafka 连接配置。
// Enabled=false 时使用内存 Channel（V1 兼容），Enabled=true 时使用 Kafka 持久化。
type KafkaConfig struct {
	Enabled  bool     `mapstructure:"enabled"`    // false = 使用内存 Channel
	Brokers  []string `mapstructure:"brokers"`    // ["localhost:9092"]
	Topic    string   `mapstructure:"topic"`      // "sync-events"
	GroupID  string   `mapstructure:"group_id"`   // "network-twin"
	SASLUser string   `mapstructure:"sasl_user"`  // 可选
	SASLPass string   `mapstructure:"sasl_pass"`  // 可选
}

// Load 从 YAML 文件加载配置，支持环境变量覆盖
// path: 配置文件路径（如 configs/config.yaml）
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.AutomaticEnv() // 支持环境变量覆盖

	// 设置默认值
	v.SetDefault("neo4j.default_db", "default")
	v.SetDefault("server.port", 8080)
	v.SetDefault("snapshot.dir", "snapshots")
	v.SetDefault("snapshot.max_active", 5)
	v.SetDefault("snapshot.retention_days", 0) // V1-20: 默认不自动清理
	v.SetDefault("schema.ontology_dir", "ontology")
	v.SetDefault("channel.buffer_size", 100)
	v.SetDefault("kafka.enabled", false)
	v.SetDefault("kafka.topic", "sync-events")
	v.SetDefault("kafka.group_id", "network-twin")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// 手动应用环境变量覆盖（viper.Unmarshal 对嵌套 key 的 env 支持不完整）
	applyEnvOverrides(&cfg)

	return &cfg, nil
}

// applyEnvOverrides 从环境变量覆盖配置值
func applyEnvOverrides(cfg *Config) {
	if v := envStr("NEO4J_URI"); v != "" {
		cfg.Neo4J.URI = v
	}
	if v := envStr("NEO4J_USER"); v != "" {
		cfg.Neo4J.User = v
	}
	if v := envStr("NEO4J_PASSWORD"); v != "" {
		cfg.Neo4J.Password = v
	}
	if v := envStr("NEO4J_DEFAULT_DB"); v != "" {
		cfg.Neo4J.DefaultDB = v
	}
}

func envStr(key string) string {
	v, _ := os.LookupEnv(key)
	return v
}
