// Package netbox 实现对接 Netbox REST API 的 Connector。
// NetboxConnector 从 Netbox 采集 Device 和 Interface 数据，
// 通过 HTTPClient 复用认证、重试、限流、分页能力。
package netbox

import (
	"context"
	"fmt"
	"net/http"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// NetboxConnector 从 Netbox REST API 采集 Device 和 Interface 数据。
type NetboxConnector struct {
	http  *connector.HTTPClient
	name  string
	types []string
}

// 编译时接口满足检查
var _ connector.Connector = (*NetboxConnector)(nil)

// NewNetboxConnector 创建 NetboxConnector 实例。
// name 为连接器名称，client 为预配置的 HTTPClient，entityTypes 为支持的实体类型列表。
func NewNetboxConnector(name string, client *connector.HTTPClient, entityTypes []string) *NetboxConnector {
	return &NetboxConnector{
		http:  client,
		name:  name,
		types: entityTypes,
	}
}

// Metadata 返回连接器元信息。
func (c *NetboxConnector) Metadata() connector.ConnectorMetadata {
	return connector.ConnectorMetadata{
		Name:        c.name,
		Type:        "netbox",
		EntityTypes: c.types,
	}
}

// Ping 验证 Netbox 可达性。
// 调用 GET /api/status/ 检查 200 响应。
func (c *NetboxConnector) Ping(ctx context.Context) error {
	resp, err := c.http.Get(ctx, "/api/status/")
	if err != nil {
		return fmt.Errorf("netbox ping: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("netbox ping: status %d", resp.StatusCode)
	}
	return nil
}

// Collect 全量拉取指定实体类型的数据。
// 支持 "Device" 和 "Interface" 两种实体类型。
func (c *NetboxConnector) Collect(ctx context.Context, entityType string) ([]connector.Resource, error) {
	switch entityType {
	case "Device":
		return c.collectDevices(ctx)
	case "Interface":
		return c.collectInterfaces(ctx)
	default:
		return nil, fmt.Errorf("netbox connector: unsupported entity type %q", entityType)
	}
}

// Stream 返回 ErrNotImplemented，MVP 阶段不实现流式推送。
func (c *NetboxConnector) Stream(ctx context.Context, entityType string) (<-chan connector.Resource, error) {
	return nil, connector.ErrNotImplemented
}

// collectDevices 分页采集 Device 数据。
func (c *NetboxConnector) collectDevices(ctx context.Context) ([]connector.Resource, error) {
	var resources []connector.Resource
	err := c.http.Paginate(ctx, "/api/dcim/devices/", 100, func(page []map[string]any) error {
		for _, raw := range page {
			props := transformDevice(raw)
			id := fmt.Sprintf("%v", raw["id"])
			resources = append(resources, connector.Resource{
				Kind:       "Device",
				ID:         id,
				Properties: props,
			})
		}
		return nil
	})
	return resources, err
}

// collectInterfaces 分页采集 Interface 数据。
func (c *NetboxConnector) collectInterfaces(ctx context.Context) ([]connector.Resource, error) {
	var resources []connector.Resource
	err := c.http.Paginate(ctx, "/api/dcim/interfaces/", 100, func(page []map[string]any) error {
		for _, raw := range page {
			props := transformInterface(raw)
			id := fmt.Sprintf("%v", raw["id"])
			resources = append(resources, connector.Resource{
				Kind:       "Interface",
				ID:         id,
				Properties: props,
			})
		}
		return nil
	})
	return resources, err
}
