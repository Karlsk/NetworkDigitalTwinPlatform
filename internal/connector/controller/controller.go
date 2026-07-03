// Package controller 实现 Controller Connector，对接网络控制器 REST API。
// V1.2 重构：API 调用委托给 ControllerClient，自身仅编排 Collect + Transform。
// 支持 8 种实体类型的采集：Device, Interface, Link, Alarm, VPN, Tunnel, ISIS, BGP。
package controller

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ControllerConnector 从控制器 REST API 采集 8 种实体类型数据。
// V1.2 重构：API 调用委托给 ControllerClient，自身仅编排 Collect + Transform。
type ControllerConnector struct {
	client  *ControllerClient
	name    string
	types   []string
	baseURL string // 仅用于 Metadata() 返回
}

// 编译时接口满足检查
var _ connector.Connector = (*ControllerConnector)(nil)

// NewControllerConnector 创建 ControllerConnector 实例（注入 ControllerClient）。
func NewControllerConnector(name string, client *ControllerClient, entityTypes []string, baseURL string) *ControllerConnector {
	return &ControllerConnector{
		client:  client,
		name:    name,
		types:   entityTypes,
		baseURL: baseURL,
	}
}

// Metadata 返回连接器元信息。
func (c *ControllerConnector) Metadata() connector.ConnectorMetadata {
	return connector.ConnectorMetadata{
		Name:        c.name,
		Type:        "controller",
		EntityTypes: c.types,
		BaseURL:     c.baseURL,
		AuthType:    "bearer",
	}
}

// Ping 验证 Controller 可达性（委托给 ControllerClient）。
func (c *ControllerConnector) Ping(ctx context.Context) error {
	if err := c.client.Ping(ctx); err != nil {
		return fmt.Errorf("controller ping: %w", err)
	}
	return nil
}

// Collect 全量拉取指定实体类型的数据。
// 支持 8 种实体类型：Device, Interface, Link, Alarm, VPN, Tunnel, ISIS, BGP。
func (c *ControllerConnector) Collect(ctx context.Context, entityType string) ([]connector.Resource, error) {
	switch entityType {
	case "Device":
		return c.collectDevices(ctx)
	case "Interface":
		return c.collectInterfaces(ctx)
	case "Link":
		return c.collectLinks(ctx)
	case "Alarm":
		return c.collectAlarms(ctx)
	case "VPN":
		return c.collectVPNs(ctx)
	case "Tunnel":
		return c.collectTunnels(ctx)
	case "ISIS":
		return c.collectISIS(ctx)
	case "BGP":
		return c.collectBGP(ctx)
	default:
		return nil, fmt.Errorf("controller connector: unsupported entity type %q", entityType)
	}
}

// Stream 返回 ErrNotImplemented，增量同步功能保留接口骨架，后续补充实现。
func (c *ControllerConnector) Stream(ctx context.Context, entityType string) (<-chan connector.Resource, error) {
	return nil, connector.ErrNotImplemented
}

// ──────────────────────────────
// 各实体采集方法（编排 client.FetchXxx + transformXxx）
// ──────────────────────────────

// collectDevices 编排：client.FetchDevices() → transformDevice()
func (c *ControllerConnector) collectDevices(ctx context.Context) ([]connector.Resource, error) {
	rawList, err := c.client.FetchDevices(ctx)
	if err != nil {
		return nil, err
	}

	var resources []connector.Resource
	for _, raw := range rawList {
		props := transformDevice(raw)
		id, _ := raw["id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "Device",
			ID:         id,
			Properties: props,
		})
	}

	slog.Info("controller devices collected", "count", len(resources))
	return resources, nil
}

// collectInterfaces 编排：client.FetchDevices() → extractInterfaces()
func (c *ControllerConnector) collectInterfaces(ctx context.Context) ([]connector.Resource, error) {
	rawList, err := c.client.FetchDevices(ctx)
	if err != nil {
		return nil, err
	}

	var resources []connector.Resource
	for _, raw := range rawList {
		deviceName, _ := raw["name"].(string)
		ifaces := extractInterfaces(deviceName, raw)
		resources = append(resources, ifaces...)
	}

	slog.Info("controller interfaces collected", "count", len(resources))
	return resources, nil
}

// collectLinks 编排：client.FetchLinks() → transformLink()
func (c *ControllerConnector) collectLinks(ctx context.Context) ([]connector.Resource, error) {
	rawList, err := c.client.FetchLinks(ctx)
	if err != nil {
		return nil, err
	}

	var resources []connector.Resource
	for _, raw := range rawList {
		props := transformLink(raw)
		id, _ := raw["link-id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "Link",
			ID:         id,
			Properties: props,
		})
	}

	slog.Info("controller links collected", "count", len(resources))
	return resources, nil
}

// collectAlarms 编排：client.FetchAlarms() → transformAlarm()
func (c *ControllerConnector) collectAlarms(ctx context.Context) ([]connector.Resource, error) {
	rawList, err := c.client.FetchAlarms(ctx)
	if err != nil {
		return nil, err
	}

	var resources []connector.Resource
	for _, raw := range rawList {
		props := transformAlarm(raw)
		id, _ := raw["id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "Alarm",
			ID:         id,
			Properties: props,
		})
	}

	slog.Info("controller alarms collected", "count", len(resources))
	return resources, nil
}

// collectVPNs 编排：client.FetchL3VPNs() + client.FetchL2VPNs() → transformVPN()
func (c *ControllerConnector) collectVPNs(ctx context.Context) ([]connector.Resource, error) {
	l3, err := c.client.FetchL3VPNs(ctx)
	if err != nil {
		slog.Warn("controller l3vpn fetch failed", "error", err)
	}
	l2, err2 := c.client.FetchL2VPNs(ctx)
	if err2 != nil {
		slog.Warn("controller l2vpn fetch failed", "error", err2)
	}

	if len(l3) == 0 && len(l2) == 0 {
		if err != nil {
			return nil, err
		}
		if err2 != nil {
			return nil, err2
		}
	}

	var resources []connector.Resource
	for _, raw := range l3 {
		props := transformVPN(raw, "L3")
		id, _ := raw["vpn-id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "VPN",
			ID:         id,
			Properties: props,
		})
	}
	for _, raw := range l2 {
		props := transformVPN(raw, "L2")
		id, _ := raw["vpn-id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "VPN",
			ID:         id,
			Properties: props,
		})
	}

	slog.Info("controller vpns collected", "count", len(resources), "l3", len(l3), "l2", len(l2))
	return resources, nil
}

// collectTunnels 编排：client.FetchTunnels() → transformTunnel()
func (c *ControllerConnector) collectTunnels(ctx context.Context) ([]connector.Resource, error) {
	rawList, err := c.client.FetchTunnels(ctx)
	if err != nil {
		return nil, err
	}

	var resources []connector.Resource
	for _, raw := range rawList {
		props := transformTunnel(raw)
		id, _ := raw["instance-id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "Tunnel",
			ID:         id,
			Properties: props,
		})
	}

	slog.Info("controller tunnels collected", "count", len(resources))
	return resources, nil
}

// collectISIS 编排：client.getDevices() + client.FetchISISNeighbors() → transformISISPeer()
func (c *ControllerConnector) collectISIS(ctx context.Context) ([]connector.Resource, error) {
	devices, err := c.client.getDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect isis: get devices: %w", err)
	}

	var resources []connector.Resource
	var mu sync.Mutex

	// 使用 goroutine 并发采集，最大并发数 5
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, dev := range devices {
		if dev.Name == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(d DeviceInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			text, err := c.client.FetchISISNeighbors(ctx, d.Name)
			if err != nil {
				slog.Warn("isis fetch failed", "device", d.Name, "error", err)
				return
			}

			peers, err := ParseISISText(d.Vendor, text)
			if err != nil {
				slog.Warn("isis parse failed", "device", d.Name, "vendor", d.Vendor, "error", err)
				return
			}

			mu.Lock()
			for _, peer := range peers {
				props := transformISISPeer(d.Name, peer)
				isisID, _ := props["isis_id"].(string)
				resources = append(resources, connector.Resource{
					Kind:       "ISIS",
					ID:         isisID,
					Properties: props,
				})
			}
			mu.Unlock()
		}(dev)
	}
	wg.Wait()

	slog.Info("controller isis collected", "count", len(resources), "devices", len(devices))
	return resources, nil
}

// collectBGP 编排：client.getDevices() + client.FetchBGPPeers() → transformBGPPeer()
func (c *ControllerConnector) collectBGP(ctx context.Context) ([]connector.Resource, error) {
	devices, err := c.client.getDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect bgp: get devices: %w", err)
	}

	var resources []connector.Resource
	var mu sync.Mutex

	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, dev := range devices {
		if dev.Name == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(d DeviceInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			text, err := c.client.FetchBGPPeers(ctx, d.Name)
			if err != nil {
				slog.Warn("bgp fetch failed", "device", d.Name, "error", err)
				return
			}

			peers, err := ParseBGPText(d.Vendor, text)
			if err != nil {
				slog.Warn("bgp parse failed", "device", d.Name, "vendor", d.Vendor, "error", err)
				return
			}

			mu.Lock()
			for _, peer := range peers {
				props := transformBGPPeer(d.Name, peer)
				bgpID, _ := props["bgp_id"].(string)
				resources = append(resources, connector.Resource{
					Kind:       "BGP",
					ID:         bgpID,
					Properties: props,
				})
			}
			mu.Unlock()
		}(dev)
	}
	wg.Wait()

	slog.Info("controller bgp collected", "count", len(resources), "devices", len(devices))
	return resources, nil
}

// ──────────────────────────────
// Interface 提取（从 Device 响应）
// ──────────────────────────────

// extractInterfaces 从 Device 响应中提取嵌套的 Interface 数据。
func extractInterfaces(deviceName string, raw map[string]any) []connector.Resource {
	var resources []connector.Resource

	peports, ok := raw["peports"].(map[string]any)
	if !ok {
		return resources
	}

	peportInfo, ok := peports["peport-info"].([]any)
	if !ok {
		return resources
	}

	for _, item := range peportInfo {
		port, ok := item.(map[string]any)
		if !ok {
			continue
		}
		props := transformInterface(deviceName, port)
		id, _ := port["id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "Interface",
			ID:         id,
			Properties: props,
		})
	}

	return resources
}
