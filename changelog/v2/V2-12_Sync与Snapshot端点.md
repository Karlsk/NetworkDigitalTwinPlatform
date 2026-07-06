# V2-12: Sync 与 Snapshot REST API 端点

**工时**: 1 天
**前置**: V2-11, V2-10
**风险等级**: 中
**Phase**: Phase 3 — Gin HTTP API

---

## 背景

V2-11 完成了 Gin 路由骨架。本任务实现 Sync 和 Snapshot 两组 REST API 端点，
直接调用 `service.SyncService` 和 `service.SnapshotService`，
复用 V1 MCP 工具（`internal/mcp/tools.go`）已有的业务逻辑。

### API 端点清单

| 方法 | 路径 | 说明 | 对应 MCP 工具 |
|------|------|------|---------------|
| `POST` | `/api/v1/sync` | 触发同步（full） | `sync_data` |
| `POST` | `/api/v1/sync/webhook` | 接收 Webhook 增量事件 | 无（V1 内部 Channel） |
| `GET` | `/api/v1/snapshot` | 列出全部快照 | `query_snapshot(list)` |
| `POST` | `/api/v1/snapshot` | 创建快照 | 无（V1 仅内部调用） |
| `DELETE` | `/api/v1/snapshot/:name` | 删除快照 | 无 |
| `POST` | `/api/v1/snapshot/restore` | 恢复快照 | `restore_snapshot` |
| `GET` | `/api/v1/snapshot/diff` | 对比两个快照 | `query_snapshot(diff)` |
| `GET` | `/api/v1/audit` | 查询审计日志 | `query_snapshot(audit)` |

---

## 实现步骤

### Step 1: Sync Handler

新建 `internal/api/handlers/sync.go`：

```go
package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "gitlab.com/pml/network-digital-twin/internal/service"
)

// SyncHandler 同步相关 handler。
type SyncHandler struct {
    svc *service.SyncService
}

// NewSyncHandler 创建 SyncHandler。
func NewSyncHandler(svc *service.SyncService) *SyncHandler {
    return &SyncHandler{svc: svc}
}

// SyncRequest 触发同步请求体。
type SyncRequest struct {
    Action string `json:"action,omitempty"` // "full"，默认 full
}

// SyncResponse 同步结果响应。
type SyncResponse struct {
    NodesCreated     int    `json:"nodes_created"`
    RelationsCreated int    `json:"relations_created"`
    OrphanEdges      int    `json:"orphan_edges_skipped"`
    Duration         string `json:"duration"`
}

// FullSync 触发全量同步。
// POST /api/v1/sync
func (h *SyncHandler) FullSync(c *gin.Context) {
    var req SyncRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        // 允许空 body，默认 full
        req.Action = "full"
    }
    if req.Action == "" {
        req.Action = "full"
    }
    if req.Action != "full" {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "unsupported action, only 'full' is supported",
        })
        return
    }

    result, err := h.svc.FullSync(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, SyncResponse{
        NodesCreated:     result.NodesCreated,
        RelationsCreated: result.RelationsCreated,
        OrphanEdges:      result.OrphanEdgesSkipped,
        Duration:         result.Duration.String(),
    })
}

// WebhookRequest Webhook 增量事件请求体。
type WebhookRequest struct {
    Action     string           `json:"action"`      // "update", "delete", "delete_relation"
    EntityType string           `json:"entity_type"`
    Connector  string           `json:"connector"`
    Data       []map[string]any `json:"data,omitempty"`
    URIs       []string         `json:"uris,omitempty"`
}

// Webhook 接收 Webhook 增量事件。
// POST /api/v1/sync/webhook
func (h *SyncHandler) Webhook(c *gin.Context) {
    var req WebhookRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
        return
    }

    event := service.SyncEvent{
        Action:     req.Action,
        EntityType: req.EntityType,
        Connector:  req.Connector,
        Data:       req.Data,
        URIs:       req.URIs,
    }

    if err := h.svc.HandleWebhook(event); err != nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusAccepted, gin.H{"message": "event queued"})
}
```

### Step 2: Snapshot Handler

新建 `internal/api/handlers/snapshot.go`：

```go
package handlers

import (
    "net/http"
    "strconv"
    "time"

    "github.com/gin-gonic/gin"

    "gitlab.com/pml/network-digital-twin/internal/service"
    "gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// SnapshotHandler 快照相关 handler。
type SnapshotHandler struct {
    svc *service.SnapshotService
}

// NewSnapshotHandler 创建 SnapshotHandler。
func NewSnapshotHandler(svc *service.SnapshotService) *SnapshotHandler {
    return &SnapshotHandler{svc: svc}
}

// CreateSnapshotRequest 创建快照请求。
type CreateSnapshotRequest struct {
    Name string `json:"name" binding:"required"`
}

// ListSnapshots 列出所有快照。
// GET /api/v1/snapshot
func (h *SnapshotHandler) ListSnapshots(c *gin.Context) {
    metas, err := h.svc.List(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    type item struct {
        Name      string `json:"name"`
        CreatedAt string `json:"created_at"`
        NodeCount int    `json:"node_count"`
        RelCount  int    `json:"rel_count"`
    }
    var list []item
    for _, m := range metas {
        list = append(list, item{
            Name: m.Name, CreatedAt: m.CreatedAt.Format(time.RFC3339),
            NodeCount: m.NodeCount, RelCount: m.RelCount,
        })
    }
    c.JSON(http.StatusOK, gin.H{"snapshots": list})
}

// CreateSnapshot 创建快照。
// POST /api/v1/snapshot
func (h *SnapshotHandler) CreateSnapshot(c *gin.Context) {
    var req CreateSnapshotRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
        return
    }

    meta, err := h.svc.Create(c.Request.Context(), req.Name)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusCreated, gin.H{
        "name": meta.Name, "node_count": meta.NodeCount, "rel_count": meta.RelCount,
    })
}

// DeleteSnapshot 删除快照。
// DELETE /api/v1/snapshot/:name
func (h *SnapshotHandler) DeleteSnapshot(c *gin.Context) {
    name := c.Param("name")
    if err := h.svc.Delete(c.Request.Context(), name); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "deleted", "name": name})
}

// RestoreSnapshotRequest 恢复快照请求。
type RestoreSnapshotRequest struct {
    Name string `json:"name" binding:"required"`
}

// RestoreSnapshot 恢复快照。
// POST /api/v1/snapshot/restore
func (h *SnapshotHandler) RestoreSnapshot(c *gin.Context) {
    var req RestoreSnapshotRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
        return
    }

    if err := h.svc.Restore(c.Request.Context(), req.Name); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "restored", "name": req.Name})
}

// DiffSnapshots 对比两个快照。
// GET /api/v1/snapshot/diff?a=snap1&b=snap2
func (h *SnapshotHandler) DiffSnapshots(c *gin.Context) {
    a, b := c.Query("a"), c.Query("b")
    if a == "" || b == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "query params a and b are required"})
        return
    }

    diff, err := h.svc.Diff(c.Request.Context(), a, b)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "added_nodes":    len(diff.AddedNodes),
        "removed_nodes":  len(diff.RemovedNodes),
        "added_rels":     len(diff.AddedRels),
        "removed_rels":   len(diff.RemovedRels),
        "changed_nodes":  len(diff.ChangedNodes),
        "changed_rels":   len(diff.ChangedRels),
    })
}

// QueryAudit 查询审计日志。
// GET /api/v1/audit?limit=50&action=create
func (h *SnapshotHandler) QueryAudit(c *gin.Context) {
    limitStr := c.DefaultQuery("limit", "50")
    limit, _ := strconv.Atoi(limitStr)
    if limit <= 0 {
        limit = 50
    }

    filter := snapshot.AuditFilter{
        Action:   c.Query("action"),
        Snapshot: c.Query("snapshot"),
    }

    var entries []snapshot.AuditEntry
    if filter.Action != "" || filter.Snapshot != "" {
        entries = h.svc.AuditQuery(filter)
    } else {
        entries = h.svc.AuditRecent(limit)
    }

    c.JSON(http.StatusOK, gin.H{"audit": entries, "count": len(entries)})
}
```

### Step 3: 路由注册更新

修改 `internal/api/server.go`，注册 Sync 和 Snapshot 路由：

```go
// RegisterRoutes 注册全部 API 路由。
func (s *Server) RegisterRoutes(deps *HandlerDeps) {
    // 健康检查
    s.engine.GET("/health", handlers.Health)

    // Sync
    syncH := handlers.NewSyncHandler(deps.SyncSvc)
    s.router.POST("/sync", syncH.FullSync)
    s.router.POST("/sync/webhook", syncH.Webhook)

    // Snapshot
    snapH := handlers.NewSnapshotHandler(deps.SnapshotSvc)
    s.router.GET("/snapshot", snapH.ListSnapshots)
    s.router.POST("/snapshot", snapH.CreateSnapshot)
    s.router.DELETE("/snapshot/:name", snapH.DeleteSnapshot)
    s.router.POST("/snapshot/restore", snapH.RestoreSnapshot)
    s.router.GET("/snapshot/diff", snapH.DiffSnapshots)

    // Audit
    s.router.GET("/audit", snapH.QueryAudit)
}
```

### Step 4: 单元测试

新建 `internal/api/handlers/sync_test.go` 和 `snapshot_test.go`：

| 测试 | 验证点 |
|------|--------|
| `TestFullSync` | POST /api/v1/sync 触发全量同步 |
| `TestWebhookAccepted` | POST /api/v1/sync/webhook 返回 202 |
| `TestWebhookQueueFull` | Channel 满时返回 503 |
| `TestListSnapshots` | GET /api/v1/snapshot 返回列表 |
| `TestCreateSnapshot` | POST /api/v1/snapshot 创建成功 |
| `TestDeleteSnapshot` | DELETE /api/v1/snapshot/:name 删除成功 |
| `TestRestoreSnapshot` | POST /api/v1/snapshot/restore 恢复成功 |
| `TestDiffSnapshots` | GET /api/v1/snapshot/diff 对比结果 |
| `TestQueryAudit` | GET /api/v1/audit 审计查询 |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/api/handlers/sync.go` | 新增 | FullSync + Webhook handler |
| `internal/api/handlers/sync_test.go` | 新增 | Sync 端点测试 |
| `internal/api/handlers/snapshot.go` | 新增 | Snapshot CRUD + Diff + Audit handler |
| `internal/api/handlers/snapshot_test.go` | 新增 | Snapshot 端点测试 |
| `internal/api/server.go` | 修改 | 注册 Sync/Snapshot/Audit 路由 |

---

## 注意事项

1. **Webhook 兼容**: V1 Webhook 走内部 Channel，V2 REST API 暴露 `/api/v1/sync/webhook` 端点，底层仍调用 `SyncService.HandleWebhook`
2. **Snapshot 创建**: V1 仅通过 `SnapshotManager.Create` 内部创建，V2 首次暴露 REST API
3. **错误码规范**: 400 (参数错误) / 500 (内部错误) / 503 (队列满) / 202 (异步接受)
4. **MCP 并行**: MCP 工具和 REST API 共享同一 Service 层，不重复实现业务逻辑

---

## 验收标准

- [ ] `POST /api/v1/sync` 触发全量同步并返回结果
- [ ] `POST /api/v1/sync/webhook` 接受增量事件，返回 202
- [ ] `GET /api/v1/snapshot` 返回快照列表
- [ ] `POST /api/v1/snapshot` 创建快照成功
- [ ] `DELETE /api/v1/snapshot/:name` 删除快照成功
- [ ] `POST /api/v1/snapshot/restore` 恢复快照成功
- [ ] `GET /api/v1/snapshot/diff?a=x&b=y` 返回对比结果
- [ ] `GET /api/v1/audit` 返回审计日志
- [ ] `go test ./internal/api/...` 全部通过
