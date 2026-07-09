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
	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// -----------------------------------------------------------------------
// Mock snapshotService
// -----------------------------------------------------------------------

type mockSnapshotService struct {
	listResult  []snapshot.SnapshotMeta
	listErr     error
	createMeta  snapshot.SnapshotMeta
	createErr   error
	deleteErr   error
	restoreErr  error
	diffResult  *snapshot.SnapshotDiff
	diffErr     error
	auditQuery  []snapshot.AuditEntry
	auditRecent []snapshot.AuditEntry
	lastFilter  snapshot.AuditFilter
	lastLimit   int
}

func (m *mockSnapshotService) List(_ context.Context) ([]snapshot.SnapshotMeta, error) {
	return m.listResult, m.listErr
}

func (m *mockSnapshotService) Diff(_ context.Context, _, _ string) (*snapshot.SnapshotDiff, error) {
	return m.diffResult, m.diffErr
}

func (m *mockSnapshotService) Restore(_ context.Context, _ string) error {
	return m.restoreErr
}

func (m *mockSnapshotService) Create(_ context.Context, name string) (snapshot.SnapshotMeta, error) {
	return m.createMeta, m.createErr
}

func (m *mockSnapshotService) Delete(_ context.Context, _ string) error {
	return m.deleteErr
}

func (m *mockSnapshotService) AuditQuery(filter snapshot.AuditFilter) []snapshot.AuditEntry {
	m.lastFilter = filter
	return m.auditQuery
}

func (m *mockSnapshotService) AuditRecent(n int) []snapshot.AuditEntry {
	m.lastLimit = n
	return m.auditRecent
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func newSnapshotRouter(h *SnapshotHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/v1/snapshot", h.ListSnapshots)
	engine.POST("/api/v1/snapshot", h.CreateSnapshot)
	engine.DELETE("/api/v1/snapshot/:name", h.DeleteSnapshot)
	engine.POST("/api/v1/snapshot/restore", h.RestoreSnapshot)
	engine.GET("/api/v1/snapshot/diff", h.DiffSnapshots)
	engine.GET("/api/v1/audit", h.QueryAudit)
	return engine
}

// -----------------------------------------------------------------------
// ListSnapshots 测试
// -----------------------------------------------------------------------

func TestListSnapshots_Success(t *testing.T) {
	svc := &mockSnapshotService{
		listResult: []snapshot.SnapshotMeta{
			{Name: "snap-1", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), NodeCount: 10, RelCount: 5},
			{Name: "snap-2", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), NodeCount: 20, RelCount: 8},
		},
	}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, resp.Code)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	snapshots, ok := data["snapshots"].([]any)
	require.True(t, ok)
	assert.Len(t, snapshots, 2)
}

func TestListSnapshots_Error(t *testing.T) {
	svc := &mockSnapshotService{listErr: errors.New("list failed")}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeInternalError, resp.Code)
}

// -----------------------------------------------------------------------
// CreateSnapshot 测试
// -----------------------------------------------------------------------

func TestCreateSnapshot_Success(t *testing.T) {
	svc := &mockSnapshotService{
		createMeta: snapshot.SnapshotMeta{Name: "new-snap", NodeCount: 15, RelCount: 7},
	}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	body := `{"name":"new-snap"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshot", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, resp.Code)
	assert.Equal(t, "created", resp.Message)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "new-snap", data["name"])
	assert.Equal(t, float64(15), data["node_count"])
}

func TestCreateSnapshot_MissingName(t *testing.T) {
	svc := &mockSnapshotService{}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshot", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSnapshotBadRequest, resp.Code)
	assert.Contains(t, resp.Message, "name is required")
}

// -----------------------------------------------------------------------
// DeleteSnapshot 测试
// -----------------------------------------------------------------------

func TestDeleteSnapshot_Success(t *testing.T) {
	svc := &mockSnapshotService{}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/snapshot/snap-1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, resp.Code)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "deleted", data["message"])
	assert.Equal(t, "snap-1", data["name"])
}

func TestDeleteSnapshot_Error(t *testing.T) {
	svc := &mockSnapshotService{deleteErr: errors.New("snapshot not found")}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/snapshot/missing", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeInternalError, resp.Code)
}

// -----------------------------------------------------------------------
// RestoreSnapshot 测试
// -----------------------------------------------------------------------

func TestRestoreSnapshot_Success(t *testing.T) {
	svc := &mockSnapshotService{}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	body := `{"name":"snap-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshot/restore", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, resp.Code)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "restored", data["message"])
	assert.Equal(t, "snap-1", data["name"])
}

func TestRestoreSnapshot_MissingName(t *testing.T) {
	svc := &mockSnapshotService{}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshot/restore", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSnapshotBadRequest, resp.Code)
}

// -----------------------------------------------------------------------
// DiffSnapshots 测试
// -----------------------------------------------------------------------

func TestDiffSnapshots_Success(t *testing.T) {
	svc := &mockSnapshotService{
		diffResult: &snapshot.SnapshotDiff{
			AddedNodes:   []assembler.Node{{}, {}},
			RemovedNodes: []assembler.Node{{}},
			AddedRels:    []assembler.Relation{{}},
			ChangedNodes: []snapshot.NodeChange{{URI: "urn:dev:1"}},
		},
	}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshot/diff?a=snap-1&b=snap-2", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, resp.Code)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(2), data["added_nodes"])
	assert.Equal(t, float64(1), data["removed_nodes"])
	assert.Equal(t, float64(1), data["added_rels"])
	assert.Equal(t, float64(0), data["removed_rels"])
	assert.Equal(t, float64(1), data["changed_nodes"])
}

func TestDiffSnapshots_MissingParams(t *testing.T) {
	svc := &mockSnapshotService{}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	tests := []struct {
		name string
		url  string
	}{
		{"missing both", "/api/v1/snapshot/diff"},
		{"missing b", "/api/v1/snapshot/diff?a=snap-1"},
		{"missing a", "/api/v1/snapshot/diff?b=snap-2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp response.Response
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, response.CodeSnapshotBadRequest, resp.Code)
			assert.Contains(t, resp.Message, "required")
		})
	}
}

// -----------------------------------------------------------------------
// QueryAudit 测试
// -----------------------------------------------------------------------

func TestQueryAudit_Recent(t *testing.T) {
	svc := &mockSnapshotService{
		auditRecent: []snapshot.AuditEntry{
			{Timestamp: time.Now(), Action: "create", Snapshot: "snap-1"},
			{Timestamp: time.Now(), Action: "restore", Snapshot: "snap-2"},
		},
	}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	// 无 filter → 调用 AuditRecent
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit?limit=10", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 10, svc.lastLimit)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, resp.Code)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(2), data["count"])
}

func TestQueryAudit_Filter(t *testing.T) {
	svc := &mockSnapshotService{
		auditQuery: []snapshot.AuditEntry{
			{Timestamp: time.Now(), Action: "create", Snapshot: "snap-1"},
		},
	}
	h := NewSnapshotHandler(svc)
	engine := newSnapshotRouter(h)

	// 有 filter → 调用 AuditQuery
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit?action=create&snapshot=snap-1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "create", svc.lastFilter.Action)
	assert.Equal(t, "snap-1", svc.lastFilter.Snapshot)

	var resp response.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, response.CodeSuccess, resp.Code)

	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), data["count"])
}
