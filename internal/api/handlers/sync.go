package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/events"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// syncService 同步服务接口（薄接口，解耦 Handler 与具体实现）。
type syncService interface {
	FullSync(ctx context.Context) (*service.SyncResult, error)
	HandleWebhook(ctx context.Context, event events.SyncEvent) error
}

// SyncHandler 同步相关请求处理器。
type SyncHandler struct {
	svc syncService
}

// NewSyncHandler 创建 SyncHandler。
func NewSyncHandler(svc syncService) *SyncHandler {
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
//
// @Summary 触发全量同步
// @Description 从所有 Connector 全量拉取数据并写入 Neo4j
// @Tags sync
// @Accept json
// @Produce json
// @Success 200 {object} SyncResponse
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/sync [post]
//
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
		response.Fail(c, http.StatusBadRequest, response.CodeSyncUnsupportedAction,
			"unsupported action, only 'full' is supported")
		return
	}

	result, err := h.svc.FullSync(c.Request.Context())
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, err.Error())
		return
	}

	response.OK(c, SyncResponse{
		NodesCreated:     result.NodesCreated,
		RelationsCreated: result.RelationsCreated,
		OrphanEdges:      result.OrphanEdgesSkipped,
		Duration:         result.Duration.String(),
	})
}

// WebhookRequest Webhook 增量事件请求体。
type WebhookRequest struct {
	Action     string           `json:"action"`      // "update", "delete", "delete_relation"
	EntityType string           `json:"entity_type"` // 实体类型
	Connector  string           `json:"connector"`   // 连接器名称
	Data       []map[string]any `json:"data,omitempty"`
	URIs       []string         `json:"uris,omitempty"`
}

// Webhook 接收 Webhook 增量事件。
//
// @Summary 接收增量事件
// @Description 接收 Webhook 增量事件，写入事件队列
// @Tags sync
// @Accept json
// @Produce json
// @Success 202 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 503 {object} response.Response
// @Router /api/v1/sync/webhook [post]
//
// POST /api/v1/sync/webhook
func (h *SyncHandler) Webhook(c *gin.Context) {
	var req WebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeBadRequest, "invalid request body")
		return
	}

	event := events.SyncEvent{
		Action:     req.Action,
		EntityType: req.EntityType,
		Connector:  req.Connector,
		Data:       req.Data,
		URIs:       req.URIs,
	}

	if err := h.svc.HandleWebhook(c.Request.Context(), event); err != nil {
		response.Fail(c, http.StatusServiceUnavailable, response.CodeSyncQueueFull, err.Error())
		return
	}

	response.Accepted(c, gin.H{"message": "event queued"})
}
