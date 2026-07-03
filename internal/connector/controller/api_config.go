// Package controller 实现 Controller Connector 及统一 API 适配层。
// api_config.go 定义设备配置相关 API 方法集（文档第 5 章：设备配置北向接口 / Restconf RPC）。
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ──────────────────────────────
// 设备配置 API（文档第 5 章：设备配置北向接口 / Restconf RPC）
// ──────────────────────────────

// FetchISISNeighbors 查询单台设备的 ISIS 邻居，返回回显文本。
// API: POST /restconf/operations/oper-rpc:isis-neighbor
func (c *ControllerClient) FetchISISNeighbors(ctx context.Context, peName string) (string, error) {
	if err := c.ensureToken(ctx); err != nil {
		return "", fmt.Errorf("fetch isis neighbors for %s: %w", peName, err)
	}

	reqBody := isisRequest{}
	reqBody.Input.PeName = peName
	reqBody.Input.Process = 10
	reqBody.Input.Verbose = true
	reqBody.Input.Scope = "isis"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal isis request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/restconf/operations/oper-rpc:isis-neighbor", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("fetch isis text for %s: %w", peName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch isis text for %s: status %d", peName, resp.StatusCode)
	}

	var restResp restconfResponse
	if err := json.NewDecoder(resp.Body).Decode(&restResp); err != nil {
		return "", fmt.Errorf("decode isis response for %s: %w", peName, err)
	}

	return restResp.Output.ISISNeighborResult, nil
}

// FetchBGPPeers 查询单台设备的 BGP 邻居，返回回显文本。
// API: POST /restconf/operations/oper-rpc:bgp-peer-config
func (c *ControllerClient) FetchBGPPeers(ctx context.Context, peName string) (string, error) {
	if err := c.ensureToken(ctx); err != nil {
		return "", fmt.Errorf("fetch bgp peers for %s: %w", peName, err)
	}

	reqBody := bgpRequest{}
	reqBody.Input.PeName = peName
	reqBody.Input.Scope = "IPv4"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal bgp request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/restconf/operations/oper-rpc:bgp-peer-config", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("fetch bgp text for %s: %w", peName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch bgp text for %s: status %d", peName, resp.StatusCode)
	}

	var restResp restconfResponse
	if err := json.NewDecoder(resp.Body).Decode(&restResp); err != nil {
		return "", fmt.Errorf("decode bgp response for %s: %w", peName, err)
	}

	return restResp.Output.CurrentConfigResult, nil
}

// FetchVPNConfig 查询单台设备的 VPN 配置。
// API: POST /restconf/operations/oper-rpc:vpn-config
func (c *ControllerClient) FetchVPNConfig(ctx context.Context, peName string) (string, error) {
	if err := c.ensureToken(ctx); err != nil {
		return "", fmt.Errorf("fetch vpn config for %s: %w", peName, err)
	}

	reqBody := RestconfRequest{
		Input: map[string]any{"pe-name": peName},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal vpn config request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/restconf/operations/oper-rpc:vpn-config", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("fetch vpn config for %s: %w", peName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch vpn config for %s: status %d", peName, resp.StatusCode)
	}

	var restResp restconfResponse
	if err := json.NewDecoder(resp.Body).Decode(&restResp); err != nil {
		return "", fmt.Errorf("decode vpn config response for %s: %w", peName, err)
	}

	return restResp.Output.CurrentConfigResult, nil
}

// FetchCurrentConfig 查询单台设备当前运行配置。
// API: POST /restconf/operations/oper-rpc:current-config
func (c *ControllerClient) FetchCurrentConfig(ctx context.Context, peName string) (string, error) {
	if err := c.ensureToken(ctx); err != nil {
		return "", fmt.Errorf("fetch current config for %s: %w", peName, err)
	}

	reqBody := RestconfRequest{
		Input: map[string]any{"pe-name": peName},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal current config request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/restconf/operations/oper-rpc:current-config", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("fetch current config for %s: %w", peName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch current config for %s: status %d", peName, resp.StatusCode)
	}

	var restResp restconfResponse
	if err := json.NewDecoder(resp.Body).Decode(&restResp); err != nil {
		return "", fmt.Errorf("decode current config response for %s: %w", peName, err)
	}

	return restResp.Output.CurrentConfigResult, nil
}

// FetchGlobalRoute 查询全局路由表。
// API: POST /restconf/operations/oper-rpc:global-route
func (c *ControllerClient) FetchGlobalRoute(ctx context.Context, peName string) (string, error) {
	if err := c.ensureToken(ctx); err != nil {
		return "", fmt.Errorf("fetch global route for %s: %w", peName, err)
	}

	reqBody := RestconfRequest{
		Input: map[string]any{"pe-name": peName},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal global route request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/restconf/operations/oper-rpc:global-route", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("fetch global route for %s: %w", peName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch global route for %s: status %d", peName, resp.StatusCode)
	}

	var restResp restconfResponse
	if err := json.NewDecoder(resp.Body).Decode(&restResp); err != nil {
		return "", fmt.Errorf("decode global route response for %s: %w", peName, err)
	}

	return restResp.Output.CurrentConfigResult, nil
}
