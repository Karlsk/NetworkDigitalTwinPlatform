// Package controller 实现 Controller Connector，对接网络控制器 REST API。
// operator.go 实现 DeviceOperator 能力接口，提供设备操作与配置查询能力。
// V1.2-02: 已实现方法委托 ControllerClient，未实现方法返回 ErrNotImplemented（V1.2-04 补充）。
package controller

import (
	"context"
	"fmt"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// 编译时接口满足检查
var _ connector.DeviceOperator = (*ControllerConnector)(nil)

// QueryDeviceConfig 查询设备当前运行配置（Restconf RPC）。
// API: POST /restconf/operations/oper-rpc:current-config
// 委托 ControllerClient.FetchCurrentConfig() 获取原始配置文本。
func (c *ControllerConnector) QueryDeviceConfig(
	ctx context.Context, device string,
) (map[string]any, error) {
	text, err := c.client.FetchCurrentConfig(ctx, device)
	if err != nil {
		return nil, fmt.Errorf("query device config for %s: %w", device, err)
	}
	return map[string]any{"config": text, "device": device}, nil
}

// QueryISISNeighbors 查询 ISIS 邻居（实时，返回解析后的邻居列表）。
// API: POST /restconf/operations/oper-rpc:isis-neighbor
// 委托 ControllerClient.FetchISISNeighbors() 获取回显文本，再通过 ParseISISText 解析。
func (c *ControllerConnector) QueryISISNeighbors(
	ctx context.Context, device string,
) ([]map[string]any, error) {
	text, err := c.client.FetchISISNeighbors(ctx, device)
	if err != nil {
		return nil, fmt.Errorf("query isis neighbors for %s: %w", device, err)
	}

	// 需要 vendor 信息来解析文本
	devices, err := c.client.getDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("query isis neighbors for %s: get devices for vendor lookup: %w", device, err)
	}
	vendor := lookupVendor(devices, device)

	peers, err := ParseISISText(vendor, text)
	if err != nil {
		return nil, fmt.Errorf("query isis neighbors for %s: parse isis text: %w", device, err)
	}

	var result []map[string]any
	for _, peer := range peers {
		result = append(result, transformISISPeer(device, peer))
	}
	return result, nil
}

// QueryBGPPeers 查询 BGP 邻居（实时，返回解析后的邻居列表）。
// API: POST /restconf/operations/oper-rpc:bgp-peer-config
// 委托 ControllerClient.FetchBGPPeers() 获取回显文本，再通过 ParseBGPText 解析。
func (c *ControllerConnector) QueryBGPPeers(
	ctx context.Context, device string,
) ([]map[string]any, error) {
	text, err := c.client.FetchBGPPeers(ctx, device)
	if err != nil {
		return nil, fmt.Errorf("query bgp peers for %s: %w", device, err)
	}

	// 需要 vendor 信息来解析文本
	devices, err := c.client.getDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("query bgp peers for %s: get devices for vendor lookup: %w", device, err)
	}
	vendor := lookupVendor(devices, device)

	peers, err := ParseBGPText(vendor, text)
	if err != nil {
		return nil, fmt.Errorf("query bgp peers for %s: parse bgp text: %w", device, err)
	}

	var result []map[string]any
	for _, peer := range peers {
		result = append(result, transformBGPPeer(device, peer))
	}
	return result, nil
}

// QueryVPNConfig 查询设备 VPN 配置。
// API: POST /restconf/operations/oper-rpc:vpn-config
// 委托 ControllerClient.FetchVPNConfig() 获取原始配置文本。
func (c *ControllerConnector) QueryVPNConfig(
	ctx context.Context, device string,
) (map[string]any, error) {
	text, err := c.client.FetchVPNConfig(ctx, device)
	if err != nil {
		return nil, fmt.Errorf("query vpn config for %s: %w", device, err)
	}
	return map[string]any{"config": text, "device": device}, nil
}

// QueryGlobalRoute 查询全局路由表。
// API: POST /restconf/operations/oper-rpc:global-route
// 委托 ControllerClient.FetchGlobalRoute() 获取原始路由文本。
func (c *ControllerConnector) QueryGlobalRoute(
	ctx context.Context, device string,
) ([]map[string]any, error) {
	text, err := c.client.FetchGlobalRoute(ctx, device)
	if err != nil {
		return nil, fmt.Errorf("query global route for %s: %w", device, err)
	}
	return []map[string]any{{"route": text, "device": device}}, nil
}

// ListFlexEGroups 查询 FlexE Group 列表。
// API: GET /api/no/config/terra-flexe:flexe/flexe-group
// V1.2-04 实现。
func (c *ControllerConnector) ListFlexEGroups(
	_ context.Context, _ connector.FilterOptions,
) ([]map[string]any, error) {
	return nil, connector.ErrNotImplemented
}

// ListSRv6Slices 查询 SRv6 网络切片列表。
// API: GET /api/no/config/terra-slicing:srv6-network-slices/srv6-network-slice
// V1.2-04 实现。
func (c *ControllerConnector) ListSRv6Slices(
	_ context.Context, _ connector.FilterOptions,
) ([]map[string]any, error) {
	return nil, connector.ErrNotImplemented
}

// ListDetNetInstances 查询确定性网络探测实例列表。
// API: GET /api/no/config/terra-h3c-detnet/ip/service/all
// V1.2-04 实现。
func (c *ControllerConnector) ListDetNetInstances(
	_ context.Context,
) ([]map[string]any, error) {
	return nil, connector.ErrNotImplemented
}

// QueryTopologyLive 查询实时拓扑视图（节点+链路），不依赖 Neo4j。
// API: GET /api/sr/config/network-topology:network-topology
// V1.2-04 实现。
func (c *ControllerConnector) QueryTopologyLive(
	_ context.Context,
) (*connector.TopologyLiveResult, error) {
	return nil, connector.ErrNotImplemented
}

// ──────────────────────────────
// 内部辅助函数
// ──────────────────────────────

// lookupVendor 从设备列表中查找指定设备的厂商信息，未找到时返回 "unknown"。
func lookupVendor(devices []DeviceInfo, deviceName string) string {
	for _, d := range devices {
		if d.Name == deviceName {
			return d.Vendor
		}
	}
	return "unknown"
}
