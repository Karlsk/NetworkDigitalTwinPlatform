// Package connector 定义数据源能力接口。
// Capability 接口扩展 Connector 的按需能力，
// 不同 Connector 按自身数据源特性选择性实现。
// Service/MCP 层通过类型断言发现能力，未实现的 Connector 返回明确错误。
package connector

import (
	"context"
	"time"
)

// ──────────────────────────────
// MonitorQuerier — 监控数据查询能力
// ──────────────────────────────

// MonitorQuerier 监控数据查询能力。
// 查询设备/端口/VPN/Tunnel 的实时或历史指标，以及告警和日志。
type MonitorQuerier interface {
	// QueryDeviceMetrics 查询设备级监控指标（CPU/内存/磁盘等）。
	// API: GET /monitor/controller/history
	QueryDeviceMetrics(ctx context.Context, device string, metrics []string, start, end time.Time) (*MetricsResult, error)

	// QueryPortMetrics 查询端口级监控指标（流量/包量/丢包/抖动/RTT 等）。
	// API: GET /monitor/switch/history (namespace=port)
	QueryPortMetrics(ctx context.Context, device, port string, metrics []string, start, end time.Time) (*MetricsResult, error)

	// QueryVPNTraffic 查询 VPN 流量指标。
	// API: GET /monitor/vpn/history
	QueryVPNTraffic(ctx context.Context, vpnID string, metrics []string, start, end time.Time) (*MetricsResult, error)

	// QueryTunnelTraffic 查询隧道流量指标。
	// API: GET /monitor/te/history
	QueryTunnelTraffic(ctx context.Context, device, tunnel string, metrics []string, start, end time.Time) (*MetricsResult, error)

	// QueryAlerts 查询告警列表。
	// API: GET /monitor/alert/list
	QueryAlerts(ctx context.Context, namespace, interval string) ([]map[string]any, error)

	// QueryLogs 查询系统操作日志或登录日志。
	// API: GET /monitor/logs 或 GET /monitor/logs/login
	QueryLogs(ctx context.Context, logType string, opts LogQueryOptions) (*LogResult, error)
}

// ──────────────────────────────
// DeviceOperator — 设备操作/配置查询能力
// ──────────────────────────────

// DeviceOperator 设备操作与配置查询能力。
// 查询设备实时运行配置、协议邻居、切片/SR-TE 资源管理。
type DeviceOperator interface {
	// QueryDeviceConfig 查询设备当前运行配置（Restconf RPC）。
	// API: POST /restconf/operations/oper-rpc:current-config
	QueryDeviceConfig(ctx context.Context, device string) (map[string]any, error)

	// QueryISISNeighbors 查询 ISIS 邻居（实时，返回解析后的邻居列表）。
	// API: POST /restconf/operations/oper-rpc:isis-neighbor
	QueryISISNeighbors(ctx context.Context, device string) ([]map[string]any, error)

	// QueryBGPPeers 查询 BGP 邻居（实时，返回解析后的邻居列表）。
	// API: POST /restconf/operations/oper-rpc:bgp-peer-config
	QueryBGPPeers(ctx context.Context, device string) ([]map[string]any, error)

	// QueryVPNConfig 查询设备 VPN 配置。
	// API: POST /restconf/operations/oper-rpc:vpn-config
	QueryVPNConfig(ctx context.Context, device string) (map[string]any, error)

	// QueryGlobalRoute 查询全局路由表。
	// API: POST /restconf/operations/oper-rpc:global-route
	QueryGlobalRoute(ctx context.Context, device string) ([]map[string]any, error)

	// ListFlexEGroups 查询 FlexE Group 列表。
	// API: GET /api/no/config/terra-flexe:flexe/flexe-group
	ListFlexEGroups(ctx context.Context, opts FilterOptions) ([]map[string]any, error)

	// ListSRv6Slices 查询 SRv6 网络切片列表。
	// API: GET /api/no/config/terra-slicing:srv6-network-slices/srv6-network-slice
	ListSRv6Slices(ctx context.Context, opts FilterOptions) ([]map[string]any, error)

	// ListDetNetInstances 查询确定性网络探测实例列表。
	// API: GET /api/no/config/terra-h3c-detnet/ip/service/all
	ListDetNetInstances(ctx context.Context) ([]map[string]any, error)

	// QueryTopologyLive 查询实时拓扑视图（节点+链路），不依赖 Neo4j。
	// API: GET /api/sr/config/network-topology:network-topology
	QueryTopologyLive(ctx context.Context) (*TopologyLiveResult, error)
}

// ──────────────────────────────
// 公共数据结构
// ──────────────────────────────

// MetricsResult 监控指标查询结果。
type MetricsResult struct {
	Device  string         `json:"device"`
	Port    string         `json:"port,omitempty"`
	VPN     string         `json:"vpn_id,omitempty"`
	Tunnel  string         `json:"tunnel_name,omitempty"`
	Metrics []MetricSeries `json:"metrics"`
}

// MetricSeries 单个指标的时间序列数据。
type MetricSeries struct {
	Name       string      `json:"name"`        // 指标名（如 cpu_usage, in_traffic）
	DataPoints []DataPoint `json:"data_points"` // 时间序列数据点
}

// DataPoint 单个数据点。
type DataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// LogQueryOptions 日志查询选项。
type LogQueryOptions struct {
	Interval  string    // 时间区间（如 "1h", "24h"）
	StartTime time.Time // 起始时间（零值时使用 Interval）
	EndTime   time.Time // 结束时间
	PageNum   int       // 页码（从 1 开始）
	PageSize  int       // 每页条数
}

// LogResult 日志查询结果。
type LogResult struct {
	Logs       []map[string]any `json:"logs"`
	TotalCount int              `json:"total_count"`
	PageNum    int              `json:"page_num"`
	PageSize   int              `json:"page_size"`
}

// FilterOptions 通用过滤选项。
type FilterOptions struct {
	DeviceName    string `json:"device_name,omitempty"`
	DstDeviceName string `json:"dst_device_name,omitempty"`
	SliceID       string `json:"slice_id,omitempty"`
}

// TopologyLiveResult 实时拓扑查询结果。
type TopologyLiveResult struct {
	Nodes []map[string]any `json:"nodes"`
	Links []map[string]any `json:"links"`
}
