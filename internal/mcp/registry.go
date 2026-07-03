package mcp

import (
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"gitlab.com/pml/network-digital-twin/internal/service"
)

// NewNetworkTwinServer 创建网络数字孪生 MCP Server 并注册 7 个工具。
//
// 依赖注入:
//   - analysisSvc: 分析服务（满足 analysisService 接口）
//   - snapshotSvc: 快照服务（满足 snapshotService 接口）
//   - syncSvc: 同步服务（满足 syncService 接口）
//   - deviceSvc: 设备服务（满足 deviceService 接口）
func NewNetworkTwinServer(
	analysisSvc *service.AnalysisService,
	snapshotSvc *service.SnapshotService,
	syncSvc *service.SyncService,
	deviceSvc *service.DeviceService,
) *mcpsdk.Server {
	h := &toolHandlers{
		analysisSvc: analysisSvc,
		snapshotSvc: snapshotSvc,
		syncSvc:     syncSvc,
		deviceSvc:   deviceSvc,
	}
	return newServer(h)
}

// newServer 创建 MCP Server 并注册全部工具（内部工厂，便于测试复用）。
func newServer(h *toolHandlers) *mcpsdk.Server {
	s := mcpsdk.NewServer(
		&mcpsdk.Implementation{
			Name:    "network-digital-twin",
			Version: "v0.1.0",
		},
		nil,
	)

	// 注册 7 个工具
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_topology",
		Description: "查询网络拓扑数据，支持按标签过滤和数量限制",
	}, h.handleQueryTopology)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_snapshot",
		Description: "查询快照列表或对比两个快照的差异",
	}, h.handleQuerySnapshot)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "sync_data",
		Description: "触发数据同步（默认全量同步）",
	}, h.handleSyncData)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "restore_snapshot",
		Description: "恢复指定快照到 default 逻辑 DB",
	}, h.handleRestoreSnapshot)

	// V1.2 新增 3 个工具
	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_monitor",
		Description: "查询设备/端口/VPN/隧道的监控指标（CPU、流量、丢包等），或查询告警和日志",
	}, h.handleQueryMonitor)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_device_info",
		Description: "实时查询设备配置、ISIS/BGP邻居、VPN配置、路由表、拓扑视图、切片信息等",
	}, h.handleQueryDeviceInfo)

	mcpsdk.AddTool(s, &mcpsdk.Tool{
		Name:        "query_topology_live",
		Description: "查询实时网络拓扑视图（节点+链路+链路指标），直接从控制器获取，不依赖图数据库",
	}, h.handleQueryTopologyLive)

	return s
}
