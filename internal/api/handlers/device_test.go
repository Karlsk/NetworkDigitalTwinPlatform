package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// ── mock deviceQueryService ──

type mockDeviceQueryService struct {
	deviceResult  any
	deviceErr     error
	lastDeviceReq service.DeviceInfoRequest

	monitorResult  any
	monitorErr     error
	lastMonitorReq service.MonitorRequest
}

func (m *mockDeviceQueryService) QueryDeviceInfo(ctx context.Context, req service.DeviceInfoRequest) (any, error) {
	m.lastDeviceReq = req
	return m.deviceResult, m.deviceErr
}

func (m *mockDeviceQueryService) QueryMonitor(ctx context.Context, req service.MonitorRequest) (any, error) {
	m.lastMonitorReq = req
	return m.monitorResult, m.monitorErr
}

// ── newDeviceTestRouter 创建测试用路由 ──

func newDeviceTestRouter(svc *mockDeviceQueryService) *gin.Engine {
	engine := gin.New()
	h := NewDeviceHandler(svc)
	engine.GET("/api/v1/device/:connector/:query_type", h.QueryDeviceInfo)
	engine.GET("/api/v1/monitor/:connector/:query_type", h.QueryMonitor)
	return engine
}

// ── QueryDeviceInfo 测试 ──

func TestQueryDeviceInfo_Success(t *testing.T) {
	configData := map[string]any{
		"hostname": "R1",
		"mgmt_ip":  "10.0.0.1",
		"vendor":   "Huawei",
	}
	mock := &mockDeviceQueryService{deviceResult: configData}
	engine := newDeviceTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/device/controller-1/config?device=R1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, body.Code)
	assert.NotNil(t, body.Data)

	// 验证参数传递
	assert.Equal(t, "controller-1", mock.lastDeviceReq.ConnectorName)
	assert.Equal(t, "config", mock.lastDeviceReq.QueryType)
	assert.Equal(t, "R1", mock.lastDeviceReq.Device)
}

func TestQueryDeviceInfo_ConnectorNotFound(t *testing.T) {
	// ErrConnectorNotFound -> 404 + CodeDeviceNotFound
	mock := &mockDeviceQueryService{
		deviceErr: errors.Join(errors.New("get connector"), connector.ErrConnectorNotFound),
	}
	engine := newDeviceTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/device/unknown-conn/config", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, response.CodeDeviceNotFound, body.Code)
}

func TestQueryDeviceInfo_Unsupported(t *testing.T) {
	// DeviceOperator not implemented -> 501 + CodeDeviceUnsupported
	mock := &mockDeviceQueryService{
		deviceErr: errors.New("connector \"netbox-1\" does not support device operations (DeviceOperator not implemented)"),
	}
	engine := newDeviceTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/device/netbox-1/isis?device=R1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, response.CodeDeviceUnsupported, body.Code)
	assert.Contains(t, body.Message, "not support")
}

func TestQueryDeviceInfo_InternalError(t *testing.T) {
	mock := &mockDeviceQueryService{
		deviceErr: errors.New("timeout connecting to device"),
	}
	engine := newDeviceTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/device/controller-1/bgp?device=R1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, response.CodeInternalError, body.Code)
}

// ── QueryMonitor 测试 ──

func TestQueryMonitor_Success(t *testing.T) {
	metricsResult := &connector.MetricsResult{
		Device: "R1",
		Metrics: []connector.MetricSeries{
			{Name: "cpu_usage", DataPoints: []connector.DataPoint{
				{Timestamp: time.Now(), Value: 45.2},
			}},
		},
	}
	mock := &mockDeviceQueryService{monitorResult: metricsResult}
	engine := newDeviceTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/monitor/controller-1/device?device=R1&metrics=cpu,memory", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, body.Code)
	assert.NotNil(t, body.Data)

	// 验证参数传递
	assert.Equal(t, "controller-1", mock.lastMonitorReq.ConnectorName)
	assert.Equal(t, "device", mock.lastMonitorReq.QueryType)
	assert.Equal(t, "R1", mock.lastMonitorReq.Device)
	assert.Equal(t, []string{"cpu", "memory"}, mock.lastMonitorReq.Metrics)
}

func TestQueryMonitor_WithMetrics(t *testing.T) {
	mock := &mockDeviceQueryService{monitorResult: map[string]any{"ok": true}}
	engine := newDeviceTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/monitor/controller-1/port?device=R1&port=eth0&metrics=in_traffic,out_traffic,drop_rate", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"in_traffic", "out_traffic", "drop_rate"}, mock.lastMonitorReq.Metrics)
	assert.Equal(t, "eth0", mock.lastMonitorReq.Port)
}

func TestQueryMonitor_InvalidStartTime(t *testing.T) {
	mock := &mockDeviceQueryService{monitorResult: map[string]any{}}
	engine := newDeviceTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/monitor/controller-1/device?start_time=invalid-time", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, response.CodeMonitorBadRequest, body.Code)
	assert.Contains(t, body.Message, "start_time")
}

func TestQueryMonitor_InvalidEndTime(t *testing.T) {
	mock := &mockDeviceQueryService{monitorResult: map[string]any{}}
	engine := newDeviceTestRouter(mock)

	// 使用 UTC 时间避免 URL 中的 '+' 被解析为空格
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/monitor/controller-1/device?start_time="+start+"&end_time=bad-time", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, response.CodeMonitorBadRequest, body.Code)
	assert.Contains(t, body.Message, "end_time")
}

func TestQueryMonitor_ConnectorNotFound(t *testing.T) {
	mock := &mockDeviceQueryService{
		monitorErr: errors.Join(errors.New("get connector"), connector.ErrConnectorNotFound),
	}
	engine := newDeviceTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/monitor/unknown-conn/device", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, response.CodeMonitorNotFound, body.Code)
}

func TestQueryMonitor_Unsupported(t *testing.T) {
	mock := &mockDeviceQueryService{
		monitorErr: errors.New("connector \"netbox-1\" does not support monitoring (MonitorQuerier not implemented)"),
	}
	engine := newDeviceTestRouter(mock)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/monitor/netbox-1/device", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, response.CodeMonitorUnsupported, body.Code)
}

func TestQueryMonitor_ValidTimeRange(t *testing.T) {
	mock := &mockDeviceQueryService{monitorResult: map[string]any{"ok": true}}
	engine := newDeviceTestRouter(mock)

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	end := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/monitor/controller-1/device?device=R1&start_time="+start+"&end_time="+end, nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, mock.lastMonitorReq.StartTime.IsZero(), "StartTime 应被解析")
	assert.False(t, mock.lastMonitorReq.EndTime.IsZero(), "EndTime 应被解析")
}
