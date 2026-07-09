package handlers

import (
	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// TopologyHandler 拓扑相关请求处理器。
type TopologyHandler struct {
	Svc *service.AnalysisService
}

// QueryTopology 查询网络拓扑。
// GET /api/v1/topology
func (h *TopologyHandler) QueryTopology(c *gin.Context) {
	response.NotImplemented(c, "topology endpoint not yet implemented, coming in V2-13")
}
