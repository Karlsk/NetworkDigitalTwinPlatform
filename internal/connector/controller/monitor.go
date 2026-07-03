// Package controller 实现 Controller Connector，对接网络控制器 REST API。
// monitor.go 实现 MonitorQuerier 能力接口，提供监控数据查询能力。
// V1.2-02: 接口骨架。V1.2-04: 委托 ControllerClient Fetch 方法实现。
package controller

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// 编译时接口满足检查
var _ connector.MonitorQuerier = (*ControllerConnector)(nil)

// QueryDeviceMetrics 查询设备级监控指标（CPU/内存/磁盘等）。
// API: GET /monitor/controller/history
// V1.2-04: 委托 ControllerClient.FetchDeviceMetrics()。
func (c *ControllerConnector) QueryDeviceMetrics(
	ctx context.Context, device string, metrics []string, start, end time.Time,
) (*connector.MetricsResult, error) {
	result, err := c.client.FetchDeviceMetrics(ctx, device, metrics, start, end)
	if err != nil {
		return nil, fmt.Errorf("query device metrics: %w", err)
	}
	return result, nil
}

// QueryPortMetrics 查询端口级监控指标（流量/包量/丢包/抖动/RTT 等）。
// API: GET /monitor/switch/history (namespace=port)
// V1.2-04: 委托 ControllerClient.FetchPortMetrics()。
func (c *ControllerConnector) QueryPortMetrics(
	ctx context.Context, device, port string, metrics []string, start, end time.Time,
) (*connector.MetricsResult, error) {
	result, err := c.client.FetchPortMetrics(ctx, device, port, metrics, start, end)
	if err != nil {
		return nil, fmt.Errorf("query port metrics: %w", err)
	}
	return result, nil
}

// QueryVPNTraffic 查询 VPN 流量指标。
// API: GET /monitor/vpn/history
// V1.2-04: 委托 ControllerClient.FetchVPNTraffic()。
func (c *ControllerConnector) QueryVPNTraffic(
	ctx context.Context, vpnID string, metrics []string, start, end time.Time,
) (*connector.MetricsResult, error) {
	result, err := c.client.FetchVPNTraffic(ctx, vpnID, metrics, start, end)
	if err != nil {
		return nil, fmt.Errorf("query vpn traffic: %w", err)
	}
	return result, nil
}

// QueryTunnelTraffic 查询隧道流量指标。
// API: GET /monitor/te/history
// V1.2-04: 委托 ControllerClient.FetchTunnelTraffic()。
func (c *ControllerConnector) QueryTunnelTraffic(
	ctx context.Context, device, tunnel string, metrics []string, start, end time.Time,
) (*connector.MetricsResult, error) {
	result, err := c.client.FetchTunnelTraffic(ctx, device, tunnel, metrics, start, end)
	if err != nil {
		return nil, fmt.Errorf("query tunnel traffic: %w", err)
	}
	return result, nil
}

// QueryAlerts 查询告警列表。
// API: GET /monitor/alert/list
// V1.2-04: 委托 ControllerClient.FetchAlarms()（已有实现）。
func (c *ControllerConnector) QueryAlerts(
	ctx context.Context, namespace, interval string,
) ([]map[string]any, error) {
	alerts, err := c.client.FetchAlarms(ctx)
	if err != nil {
		return nil, fmt.Errorf("query alerts: %w", err)
	}
	return alerts, nil
}

// QueryLogs 查询系统操作日志或登录日志。
// API: GET /monitor/logs 或 GET /monitor/logs/login
// V1.2-04: 委托 ControllerClient.FetchLogs()。
func (c *ControllerConnector) QueryLogs(
	ctx context.Context, logType string, opts connector.LogQueryOptions,
) (*connector.LogResult, error) {
	result, err := c.client.FetchLogs(ctx, logType, opts)
	if err != nil {
		return nil, fmt.Errorf("query logs: %w", err)
	}
	return result, nil
}
