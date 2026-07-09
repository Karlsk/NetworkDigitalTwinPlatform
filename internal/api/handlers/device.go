package handlers

import (
	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// DeviceHandler 设备相关请求处理器。
type DeviceHandler struct {
	Svc *service.DeviceService
}

// QueryDeviceInfo 查询设备信息。
// GET /api/v1/device/:connector/:query_type
func (h *DeviceHandler) QueryDeviceInfo(c *gin.Context) {
	response.NotImplemented(c, "device endpoint not yet implemented, coming in V2-13")
}
