package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// deviceQueryService 设备查询服务接口（薄接口，解耦 Handler 与具体实现）。
type deviceQueryService interface {
	QueryDeviceInfo(ctx context.Context, req service.DeviceInfoRequest) (any, error)
	QueryMonitor(ctx context.Context, req service.MonitorRequest) (any, error)
}

// DeviceHandler 设备与监控请求处理器。
type DeviceHandler struct {
	svc deviceQueryService
}

// NewDeviceHandler 创建 DeviceHandler。
func NewDeviceHandler(svc deviceQueryService) *DeviceHandler {
	return &DeviceHandler{svc: svc}
}

// QueryDeviceInfo 查询设备信息。
//
// @Summary 查询设备信息
// @Description 根据连接器和查询类型查询设备信息
// @Tags device
// @Produce json
// @Param connector path string true "connector name"
// @Param query_type path string true "query type"
// @Param device query string false "device name"
// @Success 200 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 501 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/device/{connector}/{query_type} [get]
//
// GET /api/v1/device/:connector/:query_type?device=xxx
func (h *DeviceHandler) QueryDeviceInfo(c *gin.Context) {
	connectorName := c.Param("connector")
	queryType := c.Param("query_type")
	device := c.Query("device")

	result, err := h.svc.QueryDeviceInfo(c.Request.Context(), service.DeviceInfoRequest{
		ConnectorName: connectorName,
		QueryType:     queryType,
		Device:        device,
	})
	if err != nil {
		respondDeviceError(c, err)
		return
	}

	response.OK(c, result)
}

// QueryMonitor 查询监控数据。
//
// @Summary 查询监控数据
// @Description 根据连接器和查询类型查询监控数据
// @Tags monitor
// @Produce json
// @Param connector path string true "connector name"
// @Param query_type path string true "query type"
// @Param device query string false "device name"
// @Param port query string false "port name"
// @Param metrics query string false "metrics list"
// @Param start_time query string false "start time"
// @Param end_time query string false "end time"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 501 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/monitor/{connector}/{query_type} [get]
//
// GET /api/v1/monitor/:connector/:query_type?device=xxx&port=xxx&metrics=cpu,memory
func (h *DeviceHandler) QueryMonitor(c *gin.Context) {
	connectorName := c.Param("connector")
	queryType := c.Param("query_type")

	req := service.MonitorRequest{
		ConnectorName: connectorName,
		QueryType:     queryType,
		Device:        c.Query("device"),
		Port:          c.Query("port"),
		VPNID:         c.Query("vpn_id"),
		Tunnel:        c.Query("tunnel"),
		Namespace:     c.Query("namespace"),
		Interval:      c.Query("interval"),
		LogType:       c.Query("log_type"),
	}

	// 解析 metrics（逗号分隔）
	if m := c.Query("metrics"); m != "" {
		req.Metrics = parseMetrics(m)
	}

	// 解析时间范围
	if v := c.Query("start_time"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeMonitorBadRequest,
				"invalid start_time format, expected RFC3339")
			return
		}
		req.StartTime = t
	}
	if v := c.Query("end_time"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			response.Fail(c, http.StatusBadRequest, response.CodeMonitorBadRequest,
				"invalid end_time format, expected RFC3339")
			return
		}
		req.EndTime = t
	}

	result, err := h.svc.QueryMonitor(c.Request.Context(), req)
	if err != nil {
		respondMonitorError(c, err)
		return
	}

	response.OK(c, result)
}

// parseMetrics 将逗号分隔的 metrics 字符串拆分为切片。
func parseMetrics(s string) []string {
	var result []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

// respondDeviceError 根据错误类型返回对应的 HTTP 状态码和错误码。
func respondDeviceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, connector.ErrConnectorNotFound):
		response.Fail(c, http.StatusNotFound, response.CodeDeviceNotFound, err.Error())
	case strings.Contains(err.Error(), "does not support"):
		response.Fail(c, http.StatusNotImplemented, response.CodeDeviceUnsupported, err.Error())
	default:
		response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, err.Error())
	}
}

// respondMonitorError 根据错误类型返回对应的 HTTP 状态码和错误码（Monitor 模块）。
func respondMonitorError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, connector.ErrConnectorNotFound):
		response.Fail(c, http.StatusNotFound, response.CodeMonitorNotFound, err.Error())
	case strings.Contains(err.Error(), "does not support"):
		response.Fail(c, http.StatusNotImplemented, response.CodeMonitorUnsupported, err.Error())
	default:
		response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, err.Error())
	}
}
