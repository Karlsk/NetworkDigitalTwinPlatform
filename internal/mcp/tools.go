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

// ---------------------------------------------------------------------------
// toolHandlers — 持有依赖，每个方法对应一个 MCP 工具 handler
// ---------------------------------------------------------------------------

// toolHandlers 封装 MCP 工具所需的全部依赖。
type toolHandlers struct {
	analysisSvc analysisService
	snapshotSvc snapshotService
	syncSvc     syncService
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
