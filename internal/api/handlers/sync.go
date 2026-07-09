package handlers

import (
	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// SyncHandler 同步相关请求处理器。
type SyncHandler struct {
	Svc *service.SyncService
}

// Sync 触发数据同步。
// POST /api/v1/sync
func (h *SyncHandler) Sync(c *gin.Context) {
	response.NotImplemented(c, "sync endpoint not yet implemented, coming in V2-12")
}
