package mcp

import (
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"gitlab.com/pml/network-digital-twin/internal/graph"
	"gitlab.com/pml/network-digital-twin/internal/service"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// NewNetworkTwinServer 创建网络数字孪生 MCP Server 并注册 4 个工具。
//
// 依赖注入:
//   - gdb: 图数据库驱动（graph.GraphDB 接口）
//   - lock: 并发保护锁（*snapshot.GraphLock）
//   - manager: 快照管理器（满足 snapshotManager 接口）
//   - syncSvc: 同步服务（满足 syncService 接口）
func NewNetworkTwinServer(
	gdb graph.GraphDB,
	lock *snapshot.GraphLock,
	manager *snapshot.SnapshotManager,
	syncSvc *service.SyncService,
) *mcpsdk.Server {
	h := &toolHandlers{
		graph:   gdb,
		lock:    lock,
		manager: manager,
		syncSvc: syncSvc,
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

	// 注册 4 个工具
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

	return s
}
