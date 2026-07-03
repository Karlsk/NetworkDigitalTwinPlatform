// Package controller 实现 Controller Connector，对接网络控制器 REST API。
// monitor.go 实现 MonitorQuerier 能力接口，提供监控数据查询能力。
// V1.2-02: 接口骨架，所有方法返回 ErrNotImplemented，V1.2-04 补充 ControllerClient Fetch 方法后委托调用。
package controller

import (
	"context"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// 编译时接口满足检查
var _ connector.MonitorQuerier = (*ControllerConnector)(nil)

// QueryDeviceMetrics 查询设备级监控指标（CPU/内存/磁盘等）。
// API: GET /monitor/controller/history
// V1.2-04 实现：委托 ControllerClient.FetchDeviceMetrics() 获取原始数据。
func (c *ControllerConnector) QueryDeviceMetrics(
	_ context.Context, _ string, _ []string, _, _ time.Time,
) (*connector.MetricsResult, error) {
	return nil, connector.ErrNotImplemented
}

// QueryPortMetrics 查询端口级监控指标（流量/包量/丢包/抖动/RTT 等）。
// API: GET /monitor/switch/history (namespace=port)
// V1.2-04 实现：委托 ControllerClient.FetchPortMetrics() 获取原始数据。
func (c *ControllerConnector) QueryPortMetrics(
	_ context.Context, _, _ string, _ []string, _, _ time.Time,
) (*connector.MetricsResult, error) {
	return nil, connector.ErrNotImplemented
}

// QueryVPNTraffic 查询 VPN 流量指标。
// API: GET /monitor/vpn/history
// V1.2-04 实现：委托 ControllerClient.FetchVPNTraffic() 获取原始数据。
func (c *ControllerConnector) QueryVPNTraffic(
	_ context.Context, _ string, _ []string, _, _ time.Time,
) (*connector.MetricsResult, error) {
	return nil, connector.ErrNotImplemented
}

// QueryTunnelTraffic 查询隧道流量指标。
// API: GET /monitor/te/history
// V1.2-04 实现：委托 ControllerClient.FetchTunnelTraffic() 获取原始数据。
func (c *ControllerConnector) QueryTunnelTraffic(
	_ context.Context, _, _ string, _ []string, _, _ time.Time,
) (*connector.MetricsResult, error) {
	return nil, connector.ErrNotImplemented
}

// QueryAlerts 查询告警列表。
// API: GET /monitor/alert/list
// V1.2-04 实现：委托 ControllerClient.FetchAlerts() 获取原始数据。
func (c *ControllerConnector) QueryAlerts(
	_ context.Context, _, _ string,
) ([]map[string]any, error) {
	return nil, connector.ErrNotImplemented
}

// QueryLogs 查询系统操作日志或登录日志。
// API: GET /monitor/logs 或 GET /monitor/logs/login
// V1.2-04 实现：委托 ControllerClient.FetchLogs() 获取原始数据。
func (c *ControllerConnector) QueryLogs(
	_ context.Context, _ string, _ connector.LogQueryOptions,
) (*connector.LogResult, error) {
	return nil, connector.ErrNotImplemented
}
