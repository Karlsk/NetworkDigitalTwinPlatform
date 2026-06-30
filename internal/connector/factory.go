package connector

import (
	"fmt"
	"log/slog"
)

// ConnectorBuilder 工厂函数类型。
// name 为连接器名称，cfg 为类型特定配置，entityTypes 为采集的实体类型列表。
type ConnectorBuilder func(name string, cfg map[string]any, entityTypes []string) (Connector, error)

// ConnectorFactory 按 type 查找 Builder 并创建 Connector。
type ConnectorFactory struct {
	builders map[string]ConnectorBuilder
}

// NewConnectorFactory 创建空的 ConnectorFactory。
// 注意: mock builder 需由调用方注册（因 connector 包不能反向导入 connector/mock）。
// 使用方式:
//
//	factory := NewConnectorFactory()
//	factory.RegisterBuilder("mock", mockBuilder)
func NewConnectorFactory() *ConnectorFactory {
	return &ConnectorFactory{
		builders: make(map[string]ConnectorBuilder),
	}
}

// RegisterBuilder 注册某种 type 的构建器。
// 若同类型重复注册，后者覆盖前者。
func (f *ConnectorFactory) RegisterBuilder(connType string, builder ConnectorBuilder) {
	f.builders[connType] = builder
}

// Create 按 ConnectorConfigEntry 创建单个 Connector。
// 未注册的 type 返回 "connector type %q: builder not registered" 错误。
func (f *ConnectorFactory) Create(entry ConnectorConfigEntry) (Connector, error) {
	builder, ok := f.builders[entry.Type]
	if !ok {
		return nil, fmt.Errorf("connector type %q: builder not registered", entry.Type)
	}

	c, err := builder(entry.Name, entry.Config, entry.EntityTypes)
	if err != nil {
		return nil, fmt.Errorf("create connector %q (type=%s): %w", entry.Name, entry.Type, err)
	}
	return c, nil
}

// CreateFromConfig 加载 YAML 配置并批量注册到 ConnectorRegistry。
// 遍历配置中每个 entry，调用 Create 创建后注册到 registry。
// 任一 entry 创建失败则返回错误（已注册的 connector 保留在 registry 中）。
func (f *ConnectorFactory) CreateFromConfig(configPath string, registry *ConnectorRegistry) error {
	entries, err := LoadConnectorConfig(configPath)
	if err != nil {
		return fmt.Errorf("create from config: %w", err)
	}

	for _, entry := range entries {
		c, err := f.Create(entry)
		if err != nil {
			return fmt.Errorf("create from config: %w", err)
		}
		registry.Register(c)
		slog.Info("connector registered", "name", entry.Name, "type", entry.Type, "entity_types", entry.EntityTypes)
	}

	return nil
}
