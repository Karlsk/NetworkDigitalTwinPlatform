package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// ── mock analysisService ──

type mockAnalysisService struct {
	result *service.TopologyResult
	err    error
	// 捕获调用参数
	lastLabel string
	lastLimit int
}

func (m *mockAnalysisService) QueryTopology(ctx context.Context, label string, limit int) (*service.TopologyResult, error) {
	m.lastLabel = label
	m.lastLimit = limit
	return m.result, m.err
}

// ── mock deviceService（供 TopologyHandler.QueryTopologyLive 使用）──

type mockTopologyDeviceService struct {
	result any
	err    error
	// 捕获调用参数
	lastReq service.DeviceInfoRequest
}

func (m *mockTopologyDeviceService) QueryDeviceInfo(ctx context.Context, req service.DeviceInfoRequest) (any, error) {
	m.lastReq = req
	return m.result, m.err
}

func (m *mockTopologyDeviceService) QueryMonitor(ctx context.Context, req service.MonitorRequest) (any, error) {
	return nil, nil
}

// ── newTopologyTestRouter 创建测试用路由 ──

func newTopologyTestRouter(a *mockAnalysisService, d *mockTopologyDeviceService) *gin.Engine {
	engine := gin.New()
	h := NewTopologyHandler(a, d)
	engine.GET("/api/v1/topology", h.QueryTopology)
	engine.GET("/api/v1/topology/live", h.QueryTopologyLive)
	return engine
}

// ── QueryTopology 测试 ──

func TestQueryTopology_Success(t *testing.T) {
	nodes := []map[string]any{
		{"uri": "urn:device:1", "name": "R1"},
		{"uri": "urn:device:2", "name": "R2"},
	}
	mock := &mockAnalysisService{
		result: &service.TopologyResult{Nodes: nodes, Count: 2},
	}
	engine := newTopologyTestRouter(mock, &mockTopologyDeviceService{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topology?label=Interface&limit=50", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, response.CodeSuccess, body.Code)
	assert.Equal(t, "success", body.Message)
	assert.NotNil(t, body.Data)

	// 验证参数传递
	assert.Equal(t, "Interface", mock.lastLabel)
	assert.Equal(t, 50, mock.lastLimit)

	// 验证 data 结构包含 nodes 和 count
	dataMap, ok := body.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(2), dataMap["count"])
}

func TestQueryTopology_DefaultParams(t *testing.T) {
	mock := &mockAnalysisService{
		result: &service.TopologyResult{Nodes: []map[string]any{}, Count: 0},
	}
	engine := newTopologyTestRouter(mock, &mockTopologyDeviceService{})

	// 不传 label 和 limit，应使用默认值 "Device" 和 100
	req := httptest.NewRequest(http.MethodGet, "/api/v1/topology", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Device", mock.lastLabel)
	assert.Equal(t, 100, mock.lastLimit)
}

func TestQueryTopology_ServiceError(t *testing.T) {
	mock := &mockAnalysisService{
		err: errors.New("neo4j connection failed"),
	}
	engine := newTopologyTestRouter(mock, &mockTopologyDeviceService{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topology?label=Device", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, response.CodeTopologyQueryFailed, body.Code)
	assert.Contains(t, body.Message, "neo4j connection failed")
}

// ── QueryTopologyLive 测试 ──

func TestQueryTopologyLive_Success(t *testing.T) {
	liveResult := &connector.TopologyLiveResult{
		Nodes: []map[string]any{{"id": "node-1"}},
		Links: []map[string]any{{"src": "node-1", "dst": "node-2"}},
	}
	deviceMock := &mockTopologyDeviceService{result: liveResult}
	engine := newTopologyTestRouter(&mockAnalysisService{}, deviceMock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topology/live?connector=controller-1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, body.Code)
	assert.NotNil(t, body.Data)

	// 验证 QueryDeviceInfo 被调用，QueryType=topology
	assert.Equal(t, "controller-1", deviceMock.lastReq.ConnectorName)
	assert.Equal(t, "topology", deviceMock.lastReq.QueryType)
}

func TestQueryTopologyLive_MissingConnector(t *testing.T) {
	engine := newTopologyTestRouter(&mockAnalysisService{}, &mockTopologyDeviceService{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topology/live", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, response.CodeTopologyBadRequest, body.Code)
	assert.Contains(t, body.Message, "connector")
}

func TestQueryTopologyLive_ServiceError(t *testing.T) {
	deviceMock := &mockTopologyDeviceService{
		err: errors.New("connector \"unknown-1\" not found"),
	}
	engine := newTopologyTestRouter(&mockAnalysisService{}, deviceMock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topology/live?connector=unknown-1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, response.CodeTopologyQueryFailed, body.Code)
	assert.Contains(t, body.Message, "not found")
}
