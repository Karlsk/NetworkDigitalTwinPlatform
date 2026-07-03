// Package mcp 实现 MCP Server，通过官方 SDK 暴露网络数字孪生工具。
// MCP 层不包含业务逻辑，只做参数解析 → 调用 Service → 构造结果。
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"gitlab.com/pml/network-digital-twin/internal/service"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// ---------------------------------------------------------------------------
// 内部薄接口 — 解耦 MCP 层与具体实现，便于测试 mock
// ---------------------------------------------------------------------------

// analysisService 封装分析服务操作，由 *service.AnalysisService 隐式满足。
type analysisService interface {
	QueryTopology(ctx context.Context, label string, limit int) (*service.TopologyResult, error)
}

// snapshotService 封装快照服务操作，由 *service.SnapshotService 隐式满足。
type snapshotService interface {
	List(ctx context.Context) ([]snapshot.SnapshotMeta, error)
	Diff(ctx context.Context, a, b string) (*snapshot.SnapshotDiff, error)
	Restore(ctx context.Context, name string) error
	AuditQuery(filter snapshot.AuditFilter) []snapshot.AuditEntry
	AuditRecent(n int) []snapshot.AuditEntry
}

// syncService 封装同步服务操作，由 *service.SyncService 隐式满足。
type syncService interface {
	FullSync(ctx context.Context) (*service.SyncResult, error)
}

// deviceService 封装设备操作服务，由 *service.DeviceService 隐式满足。
type deviceService interface {
	QueryMonitor(ctx context.Context, req service.MonitorRequest) (any, error)
	QueryDeviceInfo(ctx context.Context, req service.DeviceInfoRequest) (any, error)
}

// ---------------------------------------------------------------------------
// toolHandlers — 持有依赖，每个方法对应一个 MCP 工具 handler
// ---------------------------------------------------------------------------

// toolHandlers 封装 MCP 工具所需的全部依赖。
type toolHandlers struct {
	analysisSvc analysisService
	snapshotSvc snapshotService
	syncSvc     syncService
	deviceSvc   deviceService
}

// ---------------------------------------------------------------------------
// query_topology — 只读，使用 RLock
// ---------------------------------------------------------------------------

// QueryTopologyInput 定义 query_topology 工具的输入参数。
type QueryTopologyInput struct {
	Label string `json:"label" jsonschema:"节点标签 (Device/Interface/ISIS/Link/Network_Slice)"`
	Limit int    `json:"limit,omitempty" jsonschema:"返回节点数上限，默认 100"`
}

// QueryTopologyOutput 定义 query_topology 工具的结构化输出。
type QueryTopologyOutput struct {
	Nodes []map[string]any `json:"nodes"`
	Count int              `json:"count"`
}

// handleQueryTopology 查询网络拓扑数据。
func (h *toolHandlers) handleQueryTopology(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in QueryTopologyInput,
) (*mcpsdk.CallToolResult, QueryTopologyOutput, error) {
	result, err := h.analysisSvc.QueryTopology(ctx, in.Label, in.Limit)
	if err != nil {
		return nil, QueryTopologyOutput{}, fmt.Errorf("query topology: %w", err)
	}

	return nil, QueryTopologyOutput{Nodes: result.Nodes, Count: result.Count}, nil
}

// ---------------------------------------------------------------------------
// query_snapshot — 只读，不加锁（List 只读文件系统，Diff 内部自行处理）
// ---------------------------------------------------------------------------

// QuerySnapshotInput 定义 query_snapshot 工具的输入参数。
type QuerySnapshotInput struct {
	Action string `json:"action" jsonschema:"操作类型: list (列出快照) 或 diff (对比快照) 或 audit (审计日志)"`
	SnapA  string `json:"snap_a,omitempty" jsonschema:"diff 模式下的第一个快照名称"`
	SnapB  string `json:"snap_b,omitempty" jsonschema:"diff 模式下的第二个快照名称"`
	Limit  int    `json:"limit,omitempty" jsonschema:"audit 模式下返回的最近条目数，默认 50"`
}

// SnapshotMetaOutput 快照元数据输出。
type SnapshotMetaOutput struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	NodeCount int    `json:"node_count"`
	RelCount  int    `json:"rel_count"`
}

// QuerySnapshotOutput 定义 query_snapshot 工具的结构化输出。
type QuerySnapshotOutput struct {
	Snapshots []SnapshotMetaOutput `json:"snapshots,omitempty"`
	Diff      *SnapshotDiffOutput  `json:"diff,omitempty"`
	Audit     []AuditEntryOutput   `json:"audit,omitempty"`
}

// AuditEntryOutput 审计日志条目输出。
type AuditEntryOutput struct {
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"`
	Snapshot  string `json:"snapshot"`
	Actor     string `json:"actor"`
	Detail    string `json:"detail"`
	Error     string `json:"error,omitempty"`
}

// SnapshotDiffOutput 快照对比差异输出。
type SnapshotDiffOutput struct {
	AddedNodes   int `json:"added_nodes"`
	RemovedNodes int `json:"removed_nodes"`
	AddedRels    int `json:"added_rels"`
	RemovedRels  int `json:"removed_rels"`
	ChangedNodes int `json:"changed_nodes"`
	ChangedRels  int `json:"changed_relations"`
}

// handleQuerySnapshot 查询快照列表或对比快照差异。
func (h *toolHandlers) handleQuerySnapshot(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in QuerySnapshotInput,
) (*mcpsdk.CallToolResult, QuerySnapshotOutput, error) {
	switch in.Action {
	case "list":
		metas, err := h.snapshotSvc.List(ctx)
		if err != nil {
			return nil, QuerySnapshotOutput{}, fmt.Errorf("list snapshots: %w", err)
		}
		var out []SnapshotMetaOutput
		for _, m := range metas {
			out = append(out, SnapshotMetaOutput{
				Name:      m.Name,
				CreatedAt: m.CreatedAt.Format(time.RFC3339),
				NodeCount: m.NodeCount,
				RelCount:  m.RelCount,
			})
		}
		slog.Info("query_snapshot list completed", "count", len(out))
		return nil, QuerySnapshotOutput{Snapshots: out}, nil

	case "diff":
		if in.SnapA == "" || in.SnapB == "" {
			return nil, QuerySnapshotOutput{}, fmt.Errorf("diff requires snap_a and snap_b parameters")
		}
		diff, err := h.snapshotSvc.Diff(ctx, in.SnapA, in.SnapB)
		if err != nil {
			return nil, QuerySnapshotOutput{}, fmt.Errorf("diff snapshots: %w", err)
		}
		out := SnapshotDiffOutput{
			AddedNodes:   len(diff.AddedNodes),
			RemovedNodes: len(diff.RemovedNodes),
			AddedRels:    len(diff.AddedRels),
			RemovedRels:  len(diff.RemovedRels),
			ChangedNodes: len(diff.ChangedNodes),
			ChangedRels:  len(diff.ChangedRels),
		}
		slog.Info("query_snapshot diff completed", "snap_a", in.SnapA, "snap_b", in.SnapB)
		return nil, QuerySnapshotOutput{Diff: &out}, nil

	case "audit":
		limit := in.Limit
		if limit <= 0 {
			limit = 50
		}
		entries := h.snapshotSvc.AuditRecent(limit)
		var out []AuditEntryOutput
		for _, e := range entries {
			out = append(out, AuditEntryOutput{
				Timestamp: e.Timestamp.Format(time.RFC3339),
				Action:    e.Action,
				Snapshot:  e.Snapshot,
				Actor:     e.Actor,
				Detail:    e.Detail,
				Error:     e.Error,
			})
		}
		slog.Info("query_snapshot audit completed", "count", len(out))
		return nil, QuerySnapshotOutput{Audit: out}, nil

	default:
		return nil, QuerySnapshotOutput{}, fmt.Errorf("unknown action %q, expected list, diff, or audit", in.Action)
	}
}

// ---------------------------------------------------------------------------
// sync_data — 写操作，锁由 SyncService.FullSync 内部管理
// ---------------------------------------------------------------------------

// SyncDataInput 定义 sync_data 工具的输入参数。
type SyncDataInput struct {
	Action string `json:"action,omitempty" jsonschema:"同步模式: full (全量同步)，默认 full"`
}

// SyncDataOutput 定义 sync_data 工具的结构化输出。
type SyncDataOutput struct {
	NodesCreated     int    `json:"nodes_created"`
	RelationsCreated int    `json:"relations_created"`
	OrphanEdges      int    `json:"orphan_edges_skipped"`
	Duration         string `json:"duration"`
}

// handleSyncData 触发数据同步。
func (h *toolHandlers) handleSyncData(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in SyncDataInput,
) (*mcpsdk.CallToolResult, SyncDataOutput, error) {
	action := in.Action
	if action == "" {
		action = "full"
	}
	if action != "full" {
		return nil, SyncDataOutput{}, fmt.Errorf("unsupported sync action %q, only full is supported", action)
	}

	result, err := h.syncSvc.FullSync(ctx)
	if err != nil {
		return nil, SyncDataOutput{}, fmt.Errorf("full sync: %w", err)
	}

	slog.Info("sync_data completed",
		"nodes", result.NodesCreated,
		"relations", result.RelationsCreated,
		"duration_ms", result.Duration.Milliseconds(),
	)
	return nil, SyncDataOutput{
		NodesCreated:     result.NodesCreated,
		RelationsCreated: result.RelationsCreated,
		OrphanEdges:      result.OrphanEdgesSkipped,
		Duration:         result.Duration.String(),
	}, nil
}

// ---------------------------------------------------------------------------
// restore_snapshot — 写操作，锁由 SnapshotManager.Restore 内部管理
// ---------------------------------------------------------------------------

// RestoreSnapshotInput 定义 restore_snapshot 工具的输入参数。
type RestoreSnapshotInput struct {
	SnapshotName string `json:"snapshot_name" jsonschema:"要恢复的快照名称"`
}

// RestoreSnapshotOutput 定义 restore_snapshot 工具的结构化输出。
type RestoreSnapshotOutput struct {
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// query_monitor — 只读，查询监控指标/告警/日志
// ---------------------------------------------------------------------------

// QueryMonitorInput 定义 query_monitor 工具的输入参数。
type QueryMonitorInput struct {
	ConnectorName string   `json:"connector_name" jsonschema:"目标 Connector 名称"`
	QueryType     string   `json:"query_type" jsonschema:"查询类型: device/port/vpn/tunnel/alerts/logs"`
	Device        string   `json:"device,omitempty" jsonschema:"设备名称"`
	Port          string   `json:"port,omitempty" jsonschema:"端口名称"`
	VPNID         string   `json:"vpn_id,omitempty" jsonschema:"VPN ID"`
	Tunnel        string   `json:"tunnel,omitempty" jsonschema:"隧道名称"`
	Metrics       []string `json:"metrics,omitempty" jsonschema:"指标名列表"`
	Namespace     string   `json:"namespace,omitempty" jsonschema:"告警命名空间"`
	Interval      string   `json:"interval,omitempty" jsonschema:"时间区间（如 1h, 24h）"`
	StartTime     string   `json:"start_time,omitempty" jsonschema:"起始时间 (RFC3339)"`
	EndTime       string   `json:"end_time,omitempty" jsonschema:"结束时间 (RFC3339)"`
	LogType       string   `json:"log_type,omitempty" jsonschema:"日志类型: system/login"`
}

// handleQueryMonitor 查询设备/端口/VPN/隧道的监控指标或告警/日志。
func (h *toolHandlers) handleQueryMonitor(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in QueryMonitorInput,
) (*mcpsdk.CallToolResult, any, error) {
	req := service.MonitorRequest{
		ConnectorName: in.ConnectorName,
		QueryType:     in.QueryType,
		Device:        in.Device,
		Port:          in.Port,
		VPNID:         in.VPNID,
		Tunnel:        in.Tunnel,
		Metrics:       in.Metrics,
		Namespace:     in.Namespace,
		Interval:      in.Interval,
		LogType:       in.LogType,
	}
	// 解析时间
	if in.StartTime != "" {
		t, err := time.Parse(time.RFC3339, in.StartTime)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid start_time: %w", err)
		}
		req.StartTime = t
	}
	if in.EndTime != "" {
		t, err := time.Parse(time.RFC3339, in.EndTime)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid end_time: %w", err)
		}
		req.EndTime = t
	}

	result, err := h.deviceSvc.QueryMonitor(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("query monitor: %w", err)
	}
	slog.Info("query_monitor completed", "type", in.QueryType, "connector", in.ConnectorName)
	return nil, result, nil
}

// ---------------------------------------------------------------------------
// query_device_info — 只读，查询设备配置/邻居/切片/路由
// ---------------------------------------------------------------------------

// QueryDeviceInfoInput 定义 query_device_info 工具的输入参数。
type QueryDeviceInfoInput struct {
	ConnectorName string `json:"connector_name" jsonschema:"目标 Connector 名称"`
	QueryType     string `json:"query_type" jsonschema:"查询类型: config/isis/bgp/vpn_config/route/topology/flexe/srv6/detnet"`
	Device        string `json:"device,omitempty" jsonschema:"设备名称"`
}

// handleQueryDeviceInfo 查询设备实时配置、协议邻居、切片等信息。
func (h *toolHandlers) handleQueryDeviceInfo(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in QueryDeviceInfoInput,
) (*mcpsdk.CallToolResult, any, error) {
	result, err := h.deviceSvc.QueryDeviceInfo(ctx, service.DeviceInfoRequest{
		ConnectorName: in.ConnectorName,
		QueryType:     in.QueryType,
		Device:        in.Device,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("query device info: %w", err)
	}
	slog.Info("query_device_info completed", "type", in.QueryType, "connector", in.ConnectorName)
	return nil, result, nil
}

// ---------------------------------------------------------------------------
// query_topology_live — 只读，查询实时拓扑（不依赖 Neo4j）
// ---------------------------------------------------------------------------

// QueryTopologyLiveInput 定义 query_topology_live 工具的输入参数。
type QueryTopologyLiveInput struct {
	ConnectorName string `json:"connector_name" jsonschema:"目标 Connector 名称"`
}

// handleQueryTopologyLive 查询实时网络拓扑视图，直接从控制器获取。
func (h *toolHandlers) handleQueryTopologyLive(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in QueryTopologyLiveInput,
) (*mcpsdk.CallToolResult, any, error) {
	result, err := h.deviceSvc.QueryDeviceInfo(ctx, service.DeviceInfoRequest{
		ConnectorName: in.ConnectorName,
		QueryType:     "topology",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("query topology live: %w", err)
	}
	slog.Info("query_topology_live completed", "connector", in.ConnectorName)
	return nil, result, nil
}

// handleRestoreSnapshot 恢复快照到 default 逻辑 DB。
func (h *toolHandlers) handleRestoreSnapshot(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in RestoreSnapshotInput,
) (*mcpsdk.CallToolResult, RestoreSnapshotOutput, error) {
	if in.SnapshotName == "" {
		return nil, RestoreSnapshotOutput{}, fmt.Errorf("missing required parameter: snapshot_name")
	}

	if err := h.snapshotSvc.Restore(ctx, in.SnapshotName); err != nil {
		return nil, RestoreSnapshotOutput{}, fmt.Errorf("restore snapshot %q: %w", in.SnapshotName, err)
	}

	msg := fmt.Sprintf("snapshot %q restored to default DB", in.SnapshotName)
	slog.Info("restore_snapshot completed", "snapshot", in.SnapshotName)
	return nil, RestoreSnapshotOutput{Message: msg}, nil
}
