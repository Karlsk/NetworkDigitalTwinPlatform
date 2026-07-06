// Package config 提供全局配置加载功能，基于 Viper 实现 YAML 文件加载与环境变量覆盖
package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// Config 服务全局配置。
// 事件处理架构分为两层：
//   - DataSource 层（Kafka）：从外部系统接收事件，cfg.Kafka.Enabled 控制是否启用
//   - EventBus 层（Channel/Kafka）：内部事件管道，cfg.EventBus.Mode 控制实现模式
type Config struct {
	Neo4J    Neo4JConfig    `mapstructure:"neo4j"`
	Server   ServerConfig   `mapstructure:"server"`
	Snapshot SnapshotConfig `mapstructure:"snapshot"`
	Schema   SchemaConfig   `mapstructure:"schema"`
	Channel  ChannelConfig  `mapstructure:"channel"`
	Kafka    KafkaConfig    `mapstructure:"kafka"`     // 数据源层（DataSource Layer）
	EventBus EventBusConfig `mapstructure:"event_bus"` // 事件总线层（EventBus Layer）
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

// ChannelConfig 是内存 Channel 缓冲配置（EventBus 层 Channel 模式使用）。
type ChannelConfig struct {
	BufferSize int `mapstructure:"buffer_size"`
}

// KafkaConfig 数据源层 Kafka 配置（DataSource Layer）。
// Enabled=true 时启动 Kafka DataSource Consumer，从外部 Kafka Topic 消费事件，
// 通过 publisher.Publish(event) 写入 EventBus。
// Enabled=false 时仅使用 Webhook 作为唯一数据源。
type KafkaConfig struct {
	Enabled  bool     `mapstructure:"enabled"`    // 是否启用 Kafka 数据源
	Brokers  []string `mapstructure:"brokers"`    // Kafka broker 地址
	Topic    string   `mapstructure:"topic"`      // 外部事件 Topic（数据源 Topic）
	GroupID  string   `mapstructure:"group_id"`   // Consumer Group ID
	SASLUser string   `mapstructure:"sasl_user"`  // 可选 SASL 认证
	SASLPass string   `mapstructure:"sasl_pass"`  // 可选 SASL 认证
}

// EventBusConfig 事件总线层配置（EventBus Layer）。
// Mode 决定事件总线实现：
//   - "channel"：内存 Channel（默认，V1 兼容，进程重启丢失事件）
//   - "kafka"：Kafka Topic（持久化，进程重启不丢事件）
//
// Fallback 机制仅作用于 EventBus 层：当 Mode="kafka" 且 Kafka 不可用时，
// 自动降级到内存 Channel，确保事件不丢失。
type EventBusConfig struct {
	Mode  string              `mapstructure:"mode"`   // "channel" | "kafka"
	Kafka EventBusKafkaConfig `mapstructure:"kafka"`  // EventBus Kafka 配置
}

// EventBusKafkaConfig EventBus 层 Kafka 实现配置。
// 用于 EventBus 的 Publisher/Consumer 共享同一个 Kafka Topic。
type EventBusKafkaConfig struct {
	Brokers  []string `mapstructure:"brokers"`    // Kafka broker 地址
	Topic    string   `mapstructure:"topic"`      // EventBus 内部 Topic
	GroupID  string   `mapstructure:"group_id"`   // Consumer Group ID
	SASLUser string   `mapstructure:"sasl_user"`  // 可选 SASL 认证
	SASLPass string   `mapstructure:"sasl_pass"`  // 可选 SASL 认证
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
	v.SetDefault("kafka.topic", "external-sync-events")
	v.SetDefault("kafka.group_id", "network-twin")
	v.SetDefault("event_bus.mode", "channel")
	v.SetDefault("event_bus.kafka.topic", "internal-sync-events")
	v.SetDefault("event_bus.kafka.group_id", "network-twin")

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
	// 数据源层（DataSource Layer）
	if v := envStr("KAFKA_ENABLED"); v == "true" {
		cfg.Kafka.Enabled = true
	}
	// 事件总线层（EventBus Layer）
	if v := envStr("EVENT_BUS_MODE"); v != "" {
		cfg.EventBus.Mode = v
	}
}

func envStr(key string) string {
	v, _ := os.LookupEnv(key)
	return v
}
