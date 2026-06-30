package connector

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ConnectorConfigEntry connectors.yaml 中单个 Connector 的配置。
type ConnectorConfigEntry struct {
	Name        string         `yaml:"name"`
	Type        string         `yaml:"type"`          // "mock" / "netbox" / "cmdb" / "controller"
	Config      map[string]any `yaml:"config"`        // 类型特定配置
	EntityTypes []string       `yaml:"entity_types"`  // 采集的实体类型列表
	Auth        AuthConfig     `yaml:"auth,omitempty"` // 认证配置
}

// AuthConfig 认证配置。
// 密钥支持双模式：直接值（token/password）优先，env 引用（token_env/password_env）兜底。
// 生产环境推荐使用 env 引用，避免密钥写入配置文件；
// 开发环境可直接在 YAML 中写 token/password 字段方便调试。
type AuthConfig struct {
	Type        string `yaml:"type"`          // "none" / "basic" / "token"
	Token       string `yaml:"token"`         // 直接值（dev 便捷，优先于 token_env）
	TokenEnv    string `yaml:"token_env"`     // 环境变量名，生产环境推荐
	Username    string `yaml:"username"`      // basic auth 用户名
	Password    string `yaml:"password"`      // 直接值（dev 便捷，优先于 password_env）
	PasswordEnv string `yaml:"password_env"`  // 环境变量名，生产环境推荐
}

// connectorConfigFile connectors.yaml 顶层包装结构。
type connectorConfigFile struct {
	Connectors []ConnectorConfigEntry `yaml:"connectors"`
}

// LoadConnectorConfig 从 YAML 文件加载 Connector 配置列表。
// path 为 YAML 文件路径。空文件返回空切片（非错误）。
func LoadConnectorConfig(path string) ([]ConnectorConfigEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load connector config %s: %w", path, err)
	}

	var file connectorConfigFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("load connector config %s: %w", path, err)
	}

	if file.Connectors == nil {
		return []ConnectorConfigEntry{}, nil
	}
	return file.Connectors, nil
}
