// Package controller 实现 Controller Connector 及统一 API 适配层。
// api_topology.go 定义拓扑相关 API 方法集（文档第 2 章：网络资源北向接口）。
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ──────────────────────────────
// 拓扑 API（文档第 2 章：网络资源北向接口）
// ──────────────────────────────

// FetchDevices 获取设备全量列表。
// API: GET /api/no/config/terra-pe:peInfos/peInfos
func (c *ControllerClient) FetchDevices(ctx context.Context) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch devices: %w", err)
	}

	resp, err := c.http.Get(ctx, "/api/no/config/terra-pe:peInfos/peInfos")
	if err != nil {
		return nil, fmt.Errorf("fetch devices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch devices: status %d", resp.StatusCode)
	}

	var devResp deviceResponse
	if err := json.NewDecoder(resp.Body).Decode(&devResp); err != nil {
		return nil, fmt.Errorf("decode devices response: %w", err)
	}
	return devResp.PeInfo, nil
}

// FetchDevicesPaged 分页获取设备列表。
// API: GET /api/no/config/terra-pe:peInfos/page?pageNumber=N&pageSize=N
func (c *ControllerClient) FetchDevicesPaged(ctx context.Context, page, size int) (*PagedResult, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch devices paged: %w", err)
	}

	path := fmt.Sprintf("/api/no/config/terra-pe:peInfos/page?pageNumber=%d&pageSize=%d", page, size)
	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch devices paged: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch devices paged: status %d", resp.StatusCode)
	}

	var result PagedResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode devices paged response: %w", err)
	}
	return &result, nil
}

// FetchPOPList 获取 POP 点列表。
// API: GET /api/no/config/terra-pe:peInfos/popInfos
func (c *ControllerClient) FetchPOPList(ctx context.Context) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch pop list: %w", err)
	}

	resp, err := c.http.Get(ctx, "/api/no/config/terra-pe:peInfos/popInfos")
	if err != nil {
		return nil, fmt.Errorf("fetch pop list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch pop list: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode pop list response: %w", err)
	}
	return result, nil
}

// FetchVendors 获取所有厂商及型号。
// API: GET /api/no/config/terra-pe:peInfos/getAllVendorProdModel
func (c *ControllerClient) FetchVendors(ctx context.Context) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch vendors: %w", err)
	}

	resp, err := c.http.Get(ctx, "/api/no/config/terra-pe:peInfos/getAllVendorProdModel")
	if err != nil {
		return nil, fmt.Errorf("fetch vendors: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch vendors: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode vendors response: %w", err)
	}
	return result, nil
}

// FetchLinks 获取链路全量列表。
// API: GET /api/sr/config/network-topology:network-topology/topology/linksInfo
func (c *ControllerClient) FetchLinks(ctx context.Context) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch links: %w", err)
	}

	resp, err := c.http.Get(ctx, "/api/sr/config/network-topology:network-topology/topology/linksInfo")
	if err != nil {
		return nil, fmt.Errorf("fetch links: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch links: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode links response: %w", err)
	}
	return result, nil
}

// FetchAlarms 获取告警列表。
// API: GET /monitor/alert/list?namespace=business&interval=1h
func (c *ControllerClient) FetchAlarms(ctx context.Context) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch alarms: %w", err)
	}

	resp, err := c.http.Get(ctx, "/monitor/alert/list?namespace=business&interval=1h")
	if err != nil {
		return nil, fmt.Errorf("fetch alarms: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch alarms: status %d", resp.StatusCode)
	}

	var alarmResp alarmResponse
	if err := json.NewDecoder(resp.Body).Decode(&alarmResp); err != nil {
		return nil, fmt.Errorf("decode alarms response: %w", err)
	}
	// data 可能为 null（无告警时）
	if alarmResp.Data == nil {
		return nil, nil
	}
	return alarmResp.Data, nil
}

// FetchL3VPNs 分页获取 L3VPN 列表。
// API: GET /api/no/config/ietf-l3vpn-ntw:l3vpn-ntw/page
func (c *ControllerClient) FetchL3VPNs(ctx context.Context) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch l3vpns: %w", err)
	}
	return c.paginateVPN(ctx, "/api/no/config/ietf-l3vpn-ntw:l3vpn-ntw/page", 100)
}

// FetchL2VPNs 分页获取 L2VPN 列表。
// API: GET /api/no/config/ietf-l2vpn-svc:l2vpn-svc/page
func (c *ControllerClient) FetchL2VPNs(ctx context.Context) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch l2vpns: %w", err)
	}
	return c.paginateVPN(ctx, "/api/no/config/ietf-l2vpn-svc:l2vpn-svc/page", 100)
}

// FetchTunnels 获取 SR-TE 隧道全量列表。
// API: GET /api/sr/config/terra-te-svc:te-policy-instance/all
func (c *ControllerClient) FetchTunnels(ctx context.Context) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch tunnels: %w", err)
	}

	resp, err := c.http.Get(ctx, "/api/sr/config/terra-te-svc:te-policy-instance/all")
	if err != nil {
		return nil, fmt.Errorf("fetch tunnels: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch tunnels: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode tunnels response: %w", err)
	}
	return result, nil
}

// ──────────────────────────────
// 内部辅助方法
// ──────────────────────────────

// paginateVPN 遍历 VPN 自定义分页（1-based page_num），返回展平后的 vpn-service 列表。
func (c *ControllerClient) paginateVPN(ctx context.Context, baseURL string, pageSize int) ([]map[string]any, error) {
	var allItems []map[string]any
	pageNum := 1

	for {
		path := fmt.Sprintf("%s?pageNo=%d&pageSize=%d", baseURL, pageNum-1, pageSize)
		resp, err := c.http.Get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("paginate vpn page %d: %w", pageNum, err)
		}

		var pageResp vpnPageResponse
		if err := json.NewDecoder(resp.Body).Decode(&pageResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode vpn page %d: %w", pageNum, err)
		}
		resp.Body.Close()

		// 展平 vpn-services.vpn-service[] 嵌套结构
		items := flattenVPNItems(pageResp.Content)
		allItems = append(allItems, items...)

		if pageNum >= pageResp.TotalPages || len(pageResp.Content) == 0 {
			break
		}
		pageNum++
	}

	return allItems, nil
}

// flattenVPNItems 从 VPN 分页响应中提取所有 vpn-service 条目。
// 真实 API 结构: content[].vpn-services.vpn-service[]
func flattenVPNItems(content []map[string]any) []map[string]any {
	var items []map[string]any
	for _, entry := range content {
		vpnServices, ok := entry["vpn-services"].(map[string]any)
		if !ok {
			continue
		}
		svcList, ok := vpnServices["vpn-service"].([]any)
		if !ok {
			continue
		}
		for _, svc := range svcList {
			if m, ok := svc.(map[string]any); ok {
				items = append(items, m)
			}
		}
	}
	return items
}
