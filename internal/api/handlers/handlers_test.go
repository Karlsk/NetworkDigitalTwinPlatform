package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestHealth(t *testing.T) {
	engine := gin.New()
	engine.GET("/health", Health)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, "ok", body.Status)
	assert.Equal(t, "v2.0.0", body.Version)
	assert.NotEmpty(t, body.Timestamp)
}

func TestTopologyHandler(t *testing.T) {
	engine := gin.New()
	h := &TopologyHandler{}
	engine.GET("/api/v1/topology", h.QueryTopology)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topology", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, float64(501001), body["code"])
	assert.Contains(t, body["message"], "V2-13")
}

func TestDeviceHandler(t *testing.T) {
	engine := gin.New()
	h := &DeviceHandler{}
	engine.GET("/api/v1/device/:connector/:query_type", h.QueryDeviceInfo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/device/netbox/devices", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, float64(501001), body["code"])
}

func TestMonitorHandler(t *testing.T) {
	engine := gin.New()
	h := &MonitorHandler{}
	engine.GET("/api/v1/monitor/:connector/:query_type", h.QueryMonitor)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/controller/telemetry", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, float64(501001), body["code"])
}
