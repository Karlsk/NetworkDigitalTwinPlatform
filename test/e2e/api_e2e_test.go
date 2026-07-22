//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/api"
	"gitlab.com/pml/network-digital-twin/internal/api/handlers"
	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/service"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// newE2EAPIServer 创建完整的 API 测试服务器，连接真实 Neo4j。
func newE2EAPIServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	client := newE2EClient(t)
	lock := snapshot.NewGraphLock()
	snapDir := t.TempDir()
	snapMgr := snapshot.NewSnapshotManager(client, lock, snapDir, 5)

	connReg := connector.NewConnectorRegistry()
	connReg.Register(mock.NewMockConnector("api-e2e-mock", testdataDir(t),
		[]string{"Device", "Interface", "ISIS", "Link", "Network_Slice"}))

	// 创建含 normalizer+assembler 的完整 SyncService
	fullSyncSvc := newE2ESyncService(t, client, lock)
	snapshotSvc := service.NewSnapshotService(snapMgr)
	analysisSvc := service.NewAnalysisService(client, lock)
	deviceSvc := service.NewDeviceService(connReg)

	// 构建 Server 并注册路由
	srv := api.NewServer()
	srv.RegisterRoutes(&api.HandlerDeps{
		SyncSvc:     fullSyncSvc,
		SnapshotSvc: snapshotSvc,
		AnalysisSvc: analysisSvc,
		DeviceSvc:   deviceSvc,
	})

	ts := httptest.NewServer(srv.Engine())
	cleanup := func() {
		ts.Close()
	}
	return ts, cleanup
}

// doRequest 发送 HTTP 请求并返回 status code + body bytes。
func doRequest(t *testing.T, method, url string, body io.Reader, headers map[string]string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do request error = %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

// parseResponse 解析标准 response.Response。
func parseResponse(t *testing.T, body []byte) response.Response {
	t.Helper()
	var resp response.Response
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("parse response JSON error = %v, body = %s", err, string(body))
	}
	return resp
}

// TestAPI_HealthEndpoint 验证 GET /health 返回 200 + 正确字段。
func TestAPI_HealthEndpoint(t *testing.T) {
	ts, cleanup := newE2EAPIServer(t)
	defer cleanup()

	code, body := doRequest(t, http.MethodGet, ts.URL+"/health", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("GET /health status = %d, want 200, body = %s", code, body)
	}

	var resp handlers.HealthResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("parse HealthResponse error = %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("Status = %q, want %q", resp.Status, "ok")
	}
	if resp.Version != "v2.0.0" {
		t.Errorf("Version = %q, want %q", resp.Version, "v2.0.0")
	}
	if resp.Timestamp == "" {
		t.Error("Timestamp is empty")
	}
}

// TestAPI_MetricsEndpoint 验证 GET /metrics 返回 Prometheus 格式指标。
func TestAPI_MetricsEndpoint(t *testing.T) {
	ts, cleanup := newE2EAPIServer(t)
	defer cleanup()

	code, body := doRequest(t, http.MethodGet, ts.URL+"/metrics", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", code)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "go_") {
		t.Error("/metrics 响应中缺少 go_ 指标（Prometheus 格式异常）")
	}
}

// TestAPI_FullSync 验证 POST /api/v1/sync 全量同步。
func TestAPI_FullSync(t *testing.T) {
	ts, cleanup := newE2EAPIServer(t)
	defer cleanup()

	// 备份/恢复 default DB
	client := newE2EClient(t)
	defer backupAndRestoreDefault(t, client)()

	code, body := doRequest(t, http.MethodPost, ts.URL+"/api/v1/sync", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("POST /api/v1/sync status = %d, want 200, body = %s", code, body)
	}

	resp := parseResponse(t, body)
	if resp.Code != response.CodeSuccess {
		t.Errorf("response code = %v, want %v, message = %s", resp.Code, response.CodeSuccess, resp.Message)
	}
}

// TestAPI_WebhookIncrementalSync 验证 POST /api/v1/sync/webhook 增量事件。
func TestAPI_WebhookIncrementalSync(t *testing.T) {
	ts, cleanup := newE2EAPIServer(t)
	defer cleanup()

	client := newE2EClient(t)
	defer backupAndRestoreDefault(t, client)()

	// 先全量同步建立基线
	doRequest(t, http.MethodPost, ts.URL+"/api/v1/sync", nil, nil)

	// 发送 Webhook 增量事件
	webhookBody := `{
		"action": "update",
		"entity_type": "Device",
		"data": [{"serial_number": "SN-API-E2E", "hostname": "API-E2E-Device", "vendor": "TestVendor", "hw_model": "M", "status": "Up"}]
	}`
	code, body := doRequest(t, http.MethodPost, ts.URL+"/api/v1/sync/webhook",
		strings.NewReader(webhookBody),
		map[string]string{"Content-Type": "application/json"})

	if code != http.StatusAccepted && code != http.StatusOK {
		t.Fatalf("POST /api/v1/sync/webhook status = %d, want 200/202, body = %s", code, body)
	}
}

// TestAPI_SnapshotCRUD 验证快照 CRUD 全生命周期：Create → List → Restore → Delete。
func TestAPI_SnapshotCRUD(t *testing.T) {
	ts, cleanup := newE2EAPIServer(t)
	defer cleanup()

	client := newE2EClient(t)
	defer backupAndRestoreDefault(t, client)()

	// 先 FullSync 填充数据
	doRequest(t, http.MethodPost, ts.URL+"/api/v1/sync", nil, nil)

	// === 1. Create Snapshot ===
	createBody := `{"name": "api-e2e-snap"}`
	code, body := doRequest(t, http.MethodPost, ts.URL+"/api/v1/snapshot",
		strings.NewReader(createBody),
		map[string]string{"Content-Type": "application/json"})

	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("POST /api/v1/snapshot status = %d, body = %s", code, body)
	}

	// === 2. List Snapshots ===
	code, body = doRequest(t, http.MethodGet, ts.URL+"/api/v1/snapshot", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("GET /api/v1/snapshot status = %d, body = %s", code, body)
	}

	resp := parseResponse(t, body)
	if resp.Code != response.CodeSuccess {
		t.Errorf("List response code = %v, want %v", resp.Code, response.CodeSuccess)
	}

	// === 3. Restore Snapshot ===
	restoreBody := `{"name": "api-e2e-snap"}`
	code, body = doRequest(t, http.MethodPost, ts.URL+"/api/v1/snapshot/restore",
		strings.NewReader(restoreBody),
		map[string]string{"Content-Type": "application/json"})

	if code != http.StatusOK {
		t.Fatalf("POST /api/v1/snapshot/restore status = %d, body = %s", code, body)
	}

	// === 4. Delete Snapshot ===
	code, body = doRequest(t, http.MethodDelete, ts.URL+"/api/v1/snapshot/api-e2e-snap", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("DELETE /api/v1/snapshot/api-e2e-snap status = %d, body = %s", code, body)
	}
}

// TestAPI_Topology 验证 GET /api/v1/topology 返回拓扑数据。
func TestAPI_Topology(t *testing.T) {
	ts, cleanup := newE2EAPIServer(t)
	defer cleanup()

	client := newE2EClient(t)
	defer backupAndRestoreDefault(t, client)()

	// FullSync 填充数据
	doRequest(t, http.MethodPost, ts.URL+"/api/v1/sync", nil, nil)

	// 查询拓扑
	code, body := doRequest(t, http.MethodGet, ts.URL+"/api/v1/topology?label=Device&limit=10", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("GET /api/v1/topology status = %d, body = %s", code, body)
	}

	resp := parseResponse(t, body)
	if resp.Code != response.CodeSuccess {
		t.Errorf("Topology response code = %v, want %v, message = %s", resp.Code, response.CodeSuccess, resp.Message)
	}
}

// TestAPI_Audit 验证 GET /api/v1/audit 返回审计日志。
func TestAPI_Audit(t *testing.T) {
	ts, cleanup := newE2EAPIServer(t)
	defer cleanup()

	client := newE2EClient(t)
	defer backupAndRestoreDefault(t, client)()

	// FullSync + Create Snapshot 产生审计记录
	doRequest(t, http.MethodPost, ts.URL+"/api/v1/sync", nil, nil)
	doRequest(t, http.MethodPost, ts.URL+"/api/v1/snapshot",
		strings.NewReader(`{"name":"audit-test-snap"}`),
		map[string]string{"Content-Type": "application/json"})

	code, body := doRequest(t, http.MethodGet, ts.URL+"/api/v1/audit?limit=10", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("GET /api/v1/audit status = %d, body = %s", code, body)
	}

	resp := parseResponse(t, body)
	if resp.Code != response.CodeSuccess {
		t.Errorf("Audit response code = %v, want %v", resp.Code, response.CodeSuccess)
	}

	// 清理
	doRequest(t, http.MethodDelete, ts.URL+"/api/v1/snapshot/audit-test-snap", nil, nil)
}

// TestAPI_NotFound 验证未知路由返回 404 标准响应。
func TestAPI_NotFound(t *testing.T) {
	ts, cleanup := newE2EAPIServer(t)
	defer cleanup()

	code, body := doRequest(t, http.MethodGet, ts.URL+"/api/v1/nonexistent", nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("GET /api/v1/nonexistent status = %d, want 404, body = %s", code, body)
	}

	resp := parseResponse(t, body)
	if resp.Code != response.CodeNotFound {
		t.Errorf("code = %v, want %v", resp.Code, response.CodeNotFound)
	}
}

// TestAPI_CORSHeaders 验证 CORS 中间件添加正确响应头。
func TestAPI_CORSHeaders(t *testing.T) {
	ts, cleanup := newE2EAPIServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/health", nil)
	req.Header.Set("Origin", "http://example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error = %v", err)
	}
	defer resp.Body.Close()

	if cors := resp.Header.Get("Access-Control-Allow-Origin"); cors == "" {
		t.Error("missing Access-Control-Allow-Origin header")
	}
}

// TestAPI_RequestIDHeader 验证 RequestID 中间件添加 X-Request-ID 响应头。
func TestAPI_RequestIDHeader(t *testing.T) {
	ts, cleanup := newE2EAPIServer(t)
	defer cleanup()

	code, _ := doRequest(t, http.MethodGet, ts.URL+"/health", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}

	// 使用自定义 client 获取完整 response headers
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET error = %v", err)
	}
	defer resp.Body.Close()

	if reqID := resp.Header.Get("X-Request-ID"); reqID == "" {
		t.Error("missing X-Request-ID header from RequestID middleware")
	}
}

// TestAPI_SnapshotDiffEndpoint 验证 GET /api/v1/snapshot/diff 端点。
func TestAPI_SnapshotDiffEndpoint(t *testing.T) {
	ts, cleanup := newE2EAPIServer(t)
	defer cleanup()

	client := newE2EClient(t)
	defer backupAndRestoreDefault(t, client)()

	// FullSync
	doRequest(t, http.MethodPost, ts.URL+"/api/v1/sync", nil, nil)

	// Create 2 snapshots
	doRequest(t, http.MethodPost, ts.URL+"/api/v1/snapshot",
		strings.NewReader(`{"name":"diff-a"}`),
		map[string]string{"Content-Type": "application/json"})

	// Upsert 一个节点使数据变化
	doRequest(t, http.MethodPost, ts.URL+"/api/v1/sync/webhook",
		strings.NewReader(`{"action":"update","entity_type":"Device","data":[{"serial_number":"SN-DIFF","hostname":"DiffDev","vendor":"V","hw_model":"M","status":"Up"}]}`),
		map[string]string{"Content-Type": "application/json"})

	// 等待 webhook 处理
	time.Sleep(500 * time.Millisecond)

	doRequest(t, http.MethodPost, ts.URL+"/api/v1/snapshot",
		strings.NewReader(`{"name":"diff-b"}`),
		map[string]string{"Content-Type": "application/json"})

	// Diff
	code, body := doRequest(t, http.MethodGet, ts.URL+"/api/v1/snapshot/diff?a=diff-a&b=diff-b", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("GET /api/v1/snapshot/diff status = %d, body = %s", code, body)
	}

	// Cleanup
	doRequest(t, http.MethodDelete, ts.URL+"/api/v1/snapshot/diff-a", nil, nil)
	doRequest(t, http.MethodDelete, ts.URL+"/api/v1/snapshot/diff-b", nil, nil)
}
