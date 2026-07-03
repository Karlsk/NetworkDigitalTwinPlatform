// Package controller 实现 Controller Connector 及统一 API 适配层。
// api_monitor.go 定义监控相关 API 方法集（文档第 6 章：网络资源北向接口 — 监控部分）。
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ──────────────────────────────
// 辅助函数
// ──────────────────────────────

// formatMonitorTime 将 Go time.Time 格式化为 Controller API 所需的时间格式。
// 示例: "2026-04-21 10:00:00"
func formatMonitorTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// buildMonitorURL 构建监控 API URL（通用）。
// 示例: /monitor/controller/history?namespace=system&metricNames=cpu_usage&startTime=...
func buildMonitorURL(basePath string, params map[string]string) string {
	if len(params) == 0 {
		return basePath
	}
	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}
	return basePath + "?" + v.Encode()
}

// monitorRawSeries 监控 API 原始响应中的单个指标序列。
type monitorRawSeries struct {
	Metric string `json:"metric"`
	Data   []struct {
		Time  int64   `json:"time"`
		Value float64 `json:"value"`
	} `json:"data"`
}

// parseMonitorResponse 解析监控响应为 MetricsResult。
// 兼容两种响应格式:
//   - 数组: [{"metric": "cpu_usage", "data": [{"time": 123, "value": 45.2}]}]
//   - 对象: {"data": [...], "series": [...]} 等 wrapper 结构
func parseMonitorResponse(body io.ReadCloser, device string) (*connector.MetricsResult, error) {
	defer body.Close()

	rawBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read monitor response: %w", err)
	}
	if len(rawBytes) == 0 {
		return &connector.MetricsResult{Device: device}, nil
	}

	// 先尝试数组格式 [{metric, data}]
	var rawSeries []monitorRawSeries
	if err := json.Unmarshal(rawBytes, &rawSeries); err == nil {
		return buildMetricsFromArray(rawSeries, device), nil
	}

	// 回退: 对象格式，搜索第一个数组类型字段
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(rawBytes, &rawMap); err != nil {
		return nil, fmt.Errorf("decode monitor response: %w", err)
	}

	for _, v := range rawMap {
		var series []monitorRawSeries
		if err := json.Unmarshal(v, &series); err == nil {
			return buildMetricsFromArray(series, device), nil
		}
	}

	// 无法解析时返回空结果（服务端返回格式不匹配）
	return &connector.MetricsResult{Device: device}, nil
}

// buildMetricsFromArray 将 monitorRawSeries 切片转换为 MetricsResult。
func buildMetricsFromArray(rawSeries []monitorRawSeries, device string) *connector.MetricsResult {
	result := &connector.MetricsResult{Device: device}
	for _, rs := range rawSeries {
		series := connector.MetricSeries{Name: rs.Metric}
		for _, dp := range rs.Data {
			series.DataPoints = append(series.DataPoints, connector.DataPoint{
				Timestamp: time.Unix(dp.Time, 0),
				Value:     dp.Value,
			})
		}
		result.Metrics = append(result.Metrics, series)
	}
	return result
}

// logPageResponse 日志分页响应结构。
type logPageResponse struct {
	Content       []map[string]any `json:"content"`
	TotalElements int              `json:"total_elements"`
	PageNum       int              `json:"page_num"`
	PageSize      int              `json:"page_size"`
}

// ──────────────────────────────
// 监控 API（文档第 6 章）
// ──────────────────────────────

// FetchDeviceMetrics 查询设备级监控指标。
// API: GET /monitor/controller/history?namespace=system&metricNames=cpu_usage&startTime=...&endTime=...
func (c *ControllerClient) FetchDeviceMetrics(
	ctx context.Context, device string, metrics []string, start, end time.Time,
) (*connector.MetricsResult, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch device metrics: %w", err)
	}

	params := map[string]string{
		"namespace":  "system",
		"metricNames": strings.Join(metrics, ","),
		"startTime":  formatMonitorTime(start),
		"endTime":    formatMonitorTime(end),
	}
	path := buildMonitorURL("/monitor/controller/history", params)

	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch device metrics for %s: %w", device, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("fetch device metrics for %s: status %d", device, resp.StatusCode)
	}

	result, err := parseMonitorResponse(resp.Body, device)
	if err != nil {
		return nil, fmt.Errorf("fetch device metrics for %s: %w", device, err)
	}
	return result, nil
}

// FetchPortMetrics 查询端口级监控指标。
// API: GET /monitor/switch/history?namespace=port&metricNames=in_traffic&dimensions.0.name=switch&dimensions.0.value={device}&dimensions.1.name=port&dimensions.1.value={port}
func (c *ControllerClient) FetchPortMetrics(
	ctx context.Context, device, port string, metrics []string, start, end time.Time,
) (*connector.MetricsResult, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch port metrics: %w", err)
	}

	params := map[string]string{
		"namespace":            "port",
		"metricNames":          strings.Join(metrics, ","),
		"startTime":            formatMonitorTime(start),
		"endTime":              formatMonitorTime(end),
		"dimensions.0.name":    "switch",
		"dimensions.0.value":   device,
		"dimensions.1.name":    "port",
		"dimensions.1.value":   port,
	}
	path := buildMonitorURL("/monitor/switch/history", params)

	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch port metrics for %s/%s: %w", device, port, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("fetch port metrics for %s/%s: status %d", device, port, resp.StatusCode)
	}

	result, err := parseMonitorResponse(resp.Body, device)
	if err != nil {
		return nil, fmt.Errorf("fetch port metrics for %s/%s: %w", device, port, err)
	}
	result.Port = port
	return result, nil
}

// FetchVPNTraffic 查询 VPN 流量指标。
// API: GET /monitor/vpn/history?namespace=traffic&metricNames=...&dimensions.0.name=vpnId&dimensions.0.value={vpnId}
func (c *ControllerClient) FetchVPNTraffic(
	ctx context.Context, vpnID string, metrics []string, start, end time.Time,
) (*connector.MetricsResult, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch vpn traffic: %w", err)
	}

	params := map[string]string{
		"namespace":          "traffic",
		"metricNames":        strings.Join(metrics, ","),
		"startTime":          formatMonitorTime(start),
		"endTime":            formatMonitorTime(end),
		"dimensions.0.name":  "vpnId",
		"dimensions.0.value": vpnID,
	}
	path := buildMonitorURL("/monitor/vpn/history", params)

	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch vpn traffic for %s: %w", vpnID, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("fetch vpn traffic for %s: status %d", vpnID, resp.StatusCode)
	}

	result, err := parseMonitorResponse(resp.Body, "")
	if err != nil {
		return nil, fmt.Errorf("fetch vpn traffic for %s: %w", vpnID, err)
	}
	result.VPN = vpnID
	return result, nil
}

// FetchTunnelTraffic 查询 SR-TE 隧道流量指标。
// API: GET /monitor/te/history?namespace=traffic&metricNames=...&dimensions.0.name=deviceName&dimensions.1.name=tunnelName
func (c *ControllerClient) FetchTunnelTraffic(
	ctx context.Context, device, tunnel string, metrics []string, start, end time.Time,
) (*connector.MetricsResult, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch tunnel traffic: %w", err)
	}

	params := map[string]string{
		"namespace":            "traffic",
		"metricNames":          strings.Join(metrics, ","),
		"startTime":            formatMonitorTime(start),
		"endTime":              formatMonitorTime(end),
		"dimensions.0.name":    "deviceName",
		"dimensions.0.value":   device,
		"dimensions.1.name":    "tunnelName",
		"dimensions.1.value":   tunnel,
	}
	path := buildMonitorURL("/monitor/te/history", params)

	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch tunnel traffic for %s/%s: %w", device, tunnel, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("fetch tunnel traffic for %s/%s: status %d", device, tunnel, resp.StatusCode)
	}

	result, err := parseMonitorResponse(resp.Body, device)
	if err != nil {
		return nil, fmt.Errorf("fetch tunnel traffic for %s/%s: %w", device, tunnel, err)
	}
	result.Tunnel = tunnel
	return result, nil
}

// FetchSystemLogs 分页查询系统操作日志。
// API: GET /monitor/logs?startTime=...&endTime=...&pageNum=1&pageSize=10
// 注意: 服务端不支持 interval 参数，未指定 startTime 时默认查询最近 1 小时。
func (c *ControllerClient) FetchSystemLogs(
	ctx context.Context, opts connector.LogQueryOptions,
) (*connector.LogResult, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch system logs: %w", err)
	}

	params := map[string]string{}
	startTime := opts.StartTime
	endTime := opts.EndTime
	// 未指定时间范围时默认最近 1 小时
	if startTime.IsZero() {
		endTime = time.Now()
		startTime = endTime.Add(-1 * time.Hour)
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}
	params["startTime"] = formatMonitorTime(startTime)
	params["endTime"] = formatMonitorTime(endTime)

	pageNum := opts.PageNum
	if pageNum < 1 {
		pageNum = 1
	}
	pageSize := opts.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	params["pageNum"] = fmt.Sprintf("%d", pageNum)
	params["pageSize"] = fmt.Sprintf("%d", pageSize)

	path := buildMonitorURL("/monitor/logs", params)

	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch system logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch system logs: status %d", resp.StatusCode)
	}

	var logResp logPageResponse
	if err := json.NewDecoder(resp.Body).Decode(&logResp); err != nil {
		return nil, fmt.Errorf("decode system logs response: %w", err)
	}

	return &connector.LogResult{
		Logs:       logResp.Content,
		TotalCount: logResp.TotalElements,
		PageNum:    logResp.PageNum,
		PageSize:   logResp.PageSize,
	}, nil
}

// FetchLoginLogs 查询用户登录日志。
// API: GET /monitor/logs/login?startTime=...&endTime=...&pageNum=1&pageSize=10
// 注意: 服务端可能未实现此接口，404 时返回空结果。
func (c *ControllerClient) FetchLoginLogs(
	ctx context.Context, opts connector.LogQueryOptions,
) (*connector.LogResult, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch login logs: %w", err)
	}

	params := map[string]string{}
	startTime := opts.StartTime
	endTime := opts.EndTime
	if startTime.IsZero() {
		endTime = time.Now()
		startTime = endTime.Add(-1 * time.Hour)
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}
	params["startTime"] = formatMonitorTime(startTime)
	params["endTime"] = formatMonitorTime(endTime)

	pageNum := opts.PageNum
	if pageNum < 1 {
		pageNum = 1
	}
	pageSize := opts.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	params["pageNum"] = fmt.Sprintf("%d", pageNum)
	params["pageSize"] = fmt.Sprintf("%d", pageSize)

	path := buildMonitorURL("/monitor/logs/login", params)

	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch login logs: %w", err)
	}
	defer resp.Body.Close()

	// 服务端未实现登录日志接口时返回空结果
	if resp.StatusCode == http.StatusNotFound {
		return &connector.LogResult{
			Logs:     []map[string]any{},
			PageNum:  pageNum,
			PageSize: pageSize,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch login logs: status %d", resp.StatusCode)
	}

	var logResp logPageResponse
	if err := json.NewDecoder(resp.Body).Decode(&logResp); err != nil {
		return nil, fmt.Errorf("decode login logs response: %w", err)
	}

	return &connector.LogResult{
		Logs:       logResp.Content,
		TotalCount: logResp.TotalElements,
		PageNum:    logResp.PageNum,
		PageSize:   logResp.PageSize,
	}, nil
}

// FetchLogs 统一日志查询入口。
func (c *ControllerClient) FetchLogs(
	ctx context.Context, logType string, opts connector.LogQueryOptions,
) (*connector.LogResult, error) {
	switch logType {
	case "login":
		return c.FetchLoginLogs(ctx, opts)
	default: // "system" 或空
		return c.FetchSystemLogs(ctx, opts)
	}
}

// FetchTopology 查询完整网络拓扑（节点+链路）。
// API: GET /api/sr/config/network-topology:network-topology
func (c *ControllerClient) FetchTopology(
	ctx context.Context,
) (*connector.TopologyLiveResult, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch topology: %w", err)
	}

	resp, err := c.http.Get(ctx, "/api/sr/config/network-topology:network-topology")
	if err != nil {
		return nil, fmt.Errorf("fetch topology: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch topology: status %d", resp.StatusCode)
	}

	// 响应格式: {"nodes": [...], "links": [...]}
	var result struct {
		Nodes []map[string]any `json:"nodes"`
		Links []map[string]any `json:"links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode topology response: %w", err)
	}

	return &connector.TopologyLiveResult{
		Nodes: result.Nodes,
		Links: result.Links,
	}, nil
}

// FetchTopologyNodes 查询拓扑节点列表。
// API: GET /api/sr/config/network-topology:network-topology/nodes
func (c *ControllerClient) FetchTopologyNodes(
	ctx context.Context,
) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch topology nodes: %w", err)
	}

	resp, err := c.http.Get(ctx, "/api/sr/config/network-topology:network-topology/nodes")
	if err != nil {
		return nil, fmt.Errorf("fetch topology nodes: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch topology nodes: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode topology nodes response: %w", err)
	}
	return result, nil
}

// FetchTopologyLinks 查询拓扑链路列表。
// API: GET /api/sr/config/network-topology:network-topology/links
func (c *ControllerClient) FetchTopologyLinks(
	ctx context.Context,
) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch topology links: %w", err)
	}

	resp, err := c.http.Get(ctx, "/api/sr/config/network-topology:network-topology/links")
	if err != nil {
		return nil, fmt.Errorf("fetch topology links: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch topology links: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode topology links response: %w", err)
	}
	return result, nil
}

// FetchLinkMetrics 查询链路指标。
// API: GET /api/sr/config/network-topology:network-topology/links-metrics
func (c *ControllerClient) FetchLinkMetrics(
	ctx context.Context,
) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch link metrics: %w", err)
	}

	resp, err := c.http.Get(ctx, "/api/sr/config/network-topology:network-topology/links-metrics")
	if err != nil {
		return nil, fmt.Errorf("fetch link metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch link metrics: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode link metrics response: %w", err)
	}
	return result, nil
}

// FetchL2Links 查询二层链路拓扑。
// API: GET /api/sr/config/network-topology:network-topology/topology/{name}/l2link
func (c *ControllerClient) FetchL2Links(
	ctx context.Context, topologyName string,
) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch l2 links: %w", err)
	}

	path := fmt.Sprintf("/api/sr/config/network-topology:network-topology/topology/%s/l2link", url.PathEscape(topologyName))
	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch l2 links for %s: %w", topologyName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch l2 links for %s: status %d", topologyName, resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode l2 links response: %w", err)
	}
	return result, nil
}
