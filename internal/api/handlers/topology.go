package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// analysisService 分析服务接口（薄接口，解耦 Handler 与具体实现）。
type analysisService interface {
	QueryTopology(ctx context.Context, label string, limit int) (*service.TopologyResult, error)
}

// TopologyHandler 拓扑相关请求处理器。
type TopologyHandler struct {
	analysisSvc analysisService
	deviceSvc   deviceQueryService
}

// NewTopologyHandler 创建 TopologyHandler。
func NewTopologyHandler(a analysisService, d deviceQueryService) *TopologyHandler {
	return &TopologyHandler{analysisSvc: a, deviceSvc: d}
}

// QueryTopology 查询图数据库拓扑。
//
// @Summary 查询图拓扑
// @Description 从 Neo4j 查询图数据库拓扑数据
// @Tags topology
// @Produce json
// @Param label query string false "entity label"
// @Param limit query int false "max items"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/topology [get]
//
// GET /api/v1/topology?label=Device&limit=100
func (h *TopologyHandler) QueryTopology(c *gin.Context) {
	label := c.DefaultQuery("label", "Device")
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 100
	}

	result, err := h.analysisSvc.QueryTopology(c.Request.Context(), label, limit)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, response.CodeTopologyQueryFailed, err.Error())
		return
	}

	response.OK(c, gin.H{
		"nodes": result.Nodes,
		"count": result.Count,
	})
}

// QueryTopologyLive 查询实时拓扑（直连控制器，不经过 Neo4j）。
//
// @Summary 查询实时拓扑
// @Description 直连控制器获取实时拓扑数据
// @Tags topology
// @Produce json
// @Param connector query string true "connector name"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/topology/live [get]
//
// GET /api/v1/topology/live?connector=controller-1
func (h *TopologyHandler) QueryTopologyLive(c *gin.Context) {
	connectorName := c.Query("connector")
	if connectorName == "" {
		response.Fail(c, http.StatusBadRequest, response.CodeTopologyBadRequest,
			"connector query param is required")
		return
	}

	result, err := h.deviceSvc.QueryDeviceInfo(c.Request.Context(), service.DeviceInfoRequest{
		ConnectorName: connectorName,
		QueryType:     "topology",
	})
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, response.CodeTopologyQueryFailed, err.Error())
		return
	}

	response.OK(c, result)
}
