// Package config 提供全局配置加载功能
package config

// Config 是全局配置结构体
type Config struct {
	Neo4j    Neo4jConfig
	Ontology OntologyConfig
	Snapshot SnapshotConfig
}

// Neo4jConfig 是 Neo4j 连接配置
type Neo4jConfig struct {
	URI      string `yaml:"uri"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// OntologyConfig 是本体 Schema 配置
type OntologyConfig struct {
	Dir string `yaml:"dir"`
}

// SnapshotConfig 是快照管理配置
type SnapshotConfig struct {
	Dir       string `yaml:"dir"`
	MaxActive int    `yaml:"max_active"`
}

// Load 从指定路径加载配置文件
// path: 配置文件路径 (如 configs/config.yaml)
func Load(path string) (*Config, error) {
	// TODO: D-05 实现 Viper 配置加载
	return &Config{
		Neo4j: Neo4jConfig{
			URI:      "bolt://localhost:7687",
			User:     "neo4j",
			Password: "password",
		},
		Ontology: OntologyConfig{Dir: "ontology"},
		Snapshot: SnapshotConfig{Dir: "snapshots", MaxActive: 5},
	}, nil
}
