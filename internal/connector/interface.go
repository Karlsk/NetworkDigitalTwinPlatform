// Package connector 定义数据源适配器接口。
// 每个数据源（Netbox/Controller/CMDB）实现一个 Connector，
// 由 SyncService 通过 ConnectorRegistry 发现和调用。
package connector

import (
	"context"
	"errors"
)

// Sentinel errors for connector operations.
var (
	// ErrNotImplemented indicates a capability is not yet available.
	// Connector.Stream returns this error during MVP phase.
	ErrNotImplemented = errors.New("not implemented")

	// ErrConnectorNotFound indicates the requested connector
	// does not exist in the registry.
	ErrConnectorNotFound = errors.New("connector not found")
)

// Connector 数据源适配器接口。
// 每个数据源（Netbox/Controller/CMDB/Mock）实现一个 Connector。
// Connector 只输出 Resource，不做字段映射或校验。
type Connector interface {
	// Metadata 返回连接器元信息。
	Metadata() ConnectorMetadata

	// Collect 全量拉取指定实体类型的数据。
	// 返回空切片表示该类型无数据（非错误）。
	// 超时或不可达时返回带上下文的错误。
	Collect(ctx context.Context, entityType string) ([]Resource, error)

	// Stream 流式推送增量变更。
	// MVP 阶段返回 (nil, ErrNotImplemented)，V1 接入 Kafka。
	Stream(ctx context.Context, entityType string) (<-chan Resource, error)
}

// ConnectorRegistry 连接器注册中心。
// 以 Connector 的 Metadata().Name 为 key 存储连接器。
type ConnectorRegistry struct {
	connectors map[string]Connector
}

// NewConnectorRegistry 创建空的连接器注册中心。
func NewConnectorRegistry() *ConnectorRegistry {
	return &ConnectorRegistry{
		connectors: make(map[string]Connector),
	}
}

// Register 注册连接器，以 Metadata().Name 为 key。
func (r *ConnectorRegistry) Register(c Connector) {
	r.connectors[c.Metadata().Name] = c
}

// Get 按名称获取连接器。
// 未找到时返回 ErrConnectorNotFound。
func (r *ConnectorRegistry) Get(name string) (Connector, error) {
	c, ok := r.connectors[name]
	if !ok {
		return nil, ErrConnectorNotFound
	}
	return c, nil
}

// List 列出所有已注册连接器的元信息。
func (r *ConnectorRegistry) List() []ConnectorMetadata {
	result := make([]ConnectorMetadata, 0, len(r.connectors))
	for _, c := range r.connectors {
		result = append(result, c.Metadata())
	}
	return result
}
