package handlers

import (
	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// MonitorHandler 监控相关请求处理器。
type MonitorHandler struct {
	Svc *service.DeviceService
}

// QueryMonitor 查询监控数据。
// GET /api/v1/monitor/:connector/:query_type
func (h *MonitorHandler) QueryMonitor(c *gin.Context) {
	response.NotImplemented(c, "monitor endpoint not yet implemented, coming in V2-13")
}
