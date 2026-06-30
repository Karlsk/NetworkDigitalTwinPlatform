// Package mock 提供 Mock Connector 实现，从 JSON 文件读取模拟数据。
// 用于开发调试和集成测试，模拟 Netbox/CMDB 等多源数据场景。
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// MockConnector 从本地 JSON 文件读取模拟数据，实现 connector.Connector 接口。
// 每个实体类型映射到一个 JSON 文件（如 Device → devices.json）。
type MockConnector struct {
	name    string
	dataDir string
	types   []string
}

// 编译时接口满足检查
var _ connector.Connector = (*MockConnector)(nil)

// entityTypeToFile 实体类型到 JSON 文件名的映射。
var entityTypeToFile = map[string]string{
	"Device":        "devices.json",
	"Interface":     "interfaces.json",
	"ISIS":          "isis.json",
	"Link":          "links.json",
	"Network_Slice": "network_slices.json",
	"Alarm":         "alarms.json",
}

// NewMockConnector 创建一个 Mock Connector 实例。
// name 为连接器名称，dataDir 为 JSON 文件所在目录，types 为支持的实体类型列表。
func NewMockConnector(name, dataDir string, types []string) *MockConnector {
	return &MockConnector{
		name:    name,
		dataDir: dataDir,
		types:   types,
	}
}

// Metadata 返回连接器元信息。
func (m *MockConnector) Metadata() connector.ConnectorMetadata {
	return connector.ConnectorMetadata{
		Name:        m.name,
		Type:        "mock",
		EntityTypes: m.types,
	}
}

// Collect 从 JSON 文件全量拉取指定实体类型的数据。
// entityType 不在映射中时返回错误。文件不存在时返回带路径上下文的错误。
func (m *MockConnector) Collect(ctx context.Context, entityType string) ([]connector.Resource, error) {
	fileName, ok := entityTypeToFile[entityType]
	if !ok {
		return nil, fmt.Errorf("unsupported entity type %q", entityType)
	}

	filePath := filepath.Join(m.dataDir, fileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read mock data %s: %w", filePath, err)
	}

	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse mock data %s: %w", filePath, err)
	}

	resources := make([]connector.Resource, 0, len(items))
	for i, item := range items {
		id, _ := item["id"].(string)
		if id == "" {
			id = fmt.Sprintf("%s-%d", entityType, i)
		}
		resources = append(resources, connector.Resource{
			Kind:       entityType,
			ID:         id,
			Properties: item,
		})
	}

	return resources, nil
}

// Stream 返回 ErrNotImplemented，MVP 阶段不实现流式推送。
func (m *MockConnector) Stream(ctx context.Context, entityType string) (<-chan connector.Resource, error) {
	return nil, connector.ErrNotImplemented
}

// Ping 健康检查。Mock 始终返回 nil（数据目录存在性已在构造时验证）。
func (m *MockConnector) Ping(_ context.Context) error {
	return nil
}
