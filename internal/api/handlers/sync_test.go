package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/events"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// -----------------------------------------------------------------------
// Mock syncService
// -----------------------------------------------------------------------

type mockSyncService struct {
	fullSyncResult *service.SyncResult
	fullSyncErr    error
	webhookErr     error
	lastEvent      events.SyncEvent
}

func (m *mockSyncService) FullSync(_ context.Context) (*service.SyncResult, error) {
	return m.fullSyncResult, m.fullSyncErr
}

func (m *mockSyncService) HandleWebhook(_ context.Context, event events.SyncEvent) error {
	m.lastEvent = event
	return m.webhookErr
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func newSyncRouter(h *SyncHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v1/sync", h.FullSync)
	engine.POST("/api/v1/sync/webhook", h.Webhook)
	return engine
}

// -----------------------------------------------------------------------
// FullSync 测试
// -----------------------------------------------------------------------

func TestFullSync_Success(t *testing.T) {
	svc := &mockSyncService{
		fullSyncResult: &service.SyncResult{
			NodesCreated:       10,
			RelationsCreated:   5,
			OrphanEdgesSkipped: 1,
			Duration:           2 * time.Second,
		},
	}
	h := NewSyncHandler(svc)
	engine := newSyncRouter(h)

	body := `{"action":"full"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, resp.Code)
	assert.Equal(t, "success", resp.Message)

	// 验证 data 字段
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(10), data["nodes_created"])
	assert.Equal(t, float64(5), data["relations_created"])
	assert.Equal(t, float64(1), data["orphan_edges_skipped"])
	assert.NotEmpty(t, data["duration"])
}

func TestFullSync_EmptyBody(t *testing.T) {
	svc := &mockSyncService{
		fullSyncResult: &service.SyncResult{
			NodesCreated: 3,
			Duration:     time.Second,
		},
	}
	h := NewSyncHandler(svc)
	engine := newSyncRouter(h)

	// 空 body，默认 action=full
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, resp.Code)
}

func TestFullSync_UnsupportedAction(t *testing.T) {
	svc := &mockSyncService{}
	h := NewSyncHandler(svc)
	engine := newSyncRouter(h)

	body := `{"action":"incremental"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSyncUnsupportedAction, resp.Code)
	assert.Contains(t, resp.Message, "unsupported action")
}

func TestFullSync_ServiceError(t *testing.T) {
	svc := &mockSyncService{
		fullSyncErr: errors.New("neo4j connection failed"),
	}
	h := NewSyncHandler(svc)
	engine := newSyncRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeInternalError, resp.Code)
	assert.Contains(t, resp.Message, "neo4j connection failed")
}

// -----------------------------------------------------------------------
// Webhook 测试
// -----------------------------------------------------------------------

func TestWebhook_Accepted(t *testing.T) {
	svc := &mockSyncService{}
	h := NewSyncHandler(svc)
	engine := newSyncRouter(h)

	body := `{
		"action": "update",
		"entity_type": "Device",
		"connector": "netbox",
		"data": [{"name": "router1"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, resp.Code)
	assert.Equal(t, "accepted", resp.Message)

	// 验证事件传递正确
	assert.Equal(t, "update", svc.lastEvent.Action)
	assert.Equal(t, "Device", svc.lastEvent.EntityType)
	assert.Equal(t, "netbox", svc.lastEvent.Connector)
	assert.Len(t, svc.lastEvent.Data, 1)
}

func TestWebhook_InvalidBody(t *testing.T) {
	svc := &mockSyncService{}
	h := NewSyncHandler(svc)
	engine := newSyncRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/webhook", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeBadRequest, resp.Code)
}

func TestWebhook_QueueFull(t *testing.T) {
	svc := &mockSyncService{
		webhookErr: errors.New("event queue full"),
	}
	h := NewSyncHandler(svc)
	engine := newSyncRouter(h)

	body := `{"action":"delete","entity_type":"Device","connector":"netbox","uris":["urn:dev:1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSyncQueueFull, resp.Code)
	assert.Contains(t, resp.Message, "event queue full")
}
