package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ──────────────────────────────
// Builder 测试
// ──────────────────────────────

func TestBuilder_ReturnsConnector(t *testing.T) {
	builder := Builder()
	require.NotNil(t, builder)

	cfg := map[string]any{
		"base_url":  "http://localhost:9999",
		"token_url": "/oauth/token",
		"username":  "admin",
		"password":  "pass",
	}
	conn, err := builder("test-ctrl", cfg, []string{"Device", "Interface"})
	require.NoError(t, err)
	require.NotNil(t, conn)
	assert.Equal(t, "test-ctrl", conn.Metadata().Name)
}

func TestBuilder_WithTimeout(t *testing.T) {
	builder := Builder()
	cfg := map[string]any{
		"base_url": "http://localhost:9999",
		"timeout":  "30s",
	}
	conn, err := builder("ctrl-timeout", cfg, []string{"Device"})
	require.NoError(t, err)
	require.NotNil(t, conn)
}

func TestBuilder_InvalidTimeout(t *testing.T) {
	builder := Builder()
	cfg := map[string]any{
		"base_url": "http://localhost:9999",
		"timeout":  "invalid-duration",
	}
	// 无效 timeout 应回退到默认 60s，不报错
	conn, err := builder("ctrl-bad-timeout", cfg, []string{"Device"})
	require.NoError(t, err)
	require.NotNil(t, conn)
}

// ──────────────────────────────
// parseISISGeneric 测试
// ──────────────────────────────

func TestParseISISGeneric(t *testing.T) {
	text := `Peer information for IS-IS(10)
------------------------------
System ID: NJ-SCT-R02
Interface: RAGG3                   Circuit Id:  151
State: Up     HoldTime: 25s        Type: L2           PRI: --
Area address(es): 49.0001
`
	peers, err := parseISISGeneric(text)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	assert.Equal(t, "NJ-SCT-R02", peers[0]["system_id"])
	assert.Equal(t, "Active", peers[0]["status"]) // "Up" maps to "Active"
}

func TestParseISISGeneric_Empty(t *testing.T) {
	peers, err := parseISISGeneric("")
	require.NoError(t, err)
	assert.Empty(t, peers)
}

// ──────────────────────────────
// mapBGPState 补全覆盖
// ──────────────────────────────

func TestMapBGPState_AllCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"established", "Established"},
		{"Established", "Established"},
		{"connect", "Connect"},
		{"Connect", "Connect"},
		{"idle", "Idle"},
		{"Idle", "Idle"},
		{"active", "Active"},
		{"Active", "Active"},
		{"opensent", "OpenSent"},
		{"OpenSent", "OpenSent"},
		{"openconfirm", "OpenConfirm"},
		{"OpenConfirm", "OpenConfirm"},
		{"", "Connect"},
		{"unknown_state", "Connect"},
		{"6", "Connect"}, // 未知数字 → 默认 Connect
		{"0", "Idle"},
		{"1", "Connect"},
		{"2", "Active"},
		{"3", "OpenSent"},
		{"4", "OpenConfirm"},
		{"5", "Established"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := mapBGPState(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ──────────────────────────────
// API 错误路径测试（补充覆盖率）
// ──────────────────────────────

// setupErrorServer 创建一个所有 API 返回 500 的 mock server。
func setupErrorServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	// 所有其他路径返回 500
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	})
	return httptest.NewServer(mux)
}

func TestClientFetchDevicesPaged_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchDevicesPaged(context.Background(), 1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchPOPList_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchPOPList(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchVendors_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchVendors(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchL3VPNs_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchL3VPNs(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchL2VPNs_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchL2VPNs(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchTunnels_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchTunnels(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchTopology_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchTopology(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchTopologyNodes_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchTopologyNodes(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchTopologyLinks_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchTopologyLinks(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchLinkMetrics_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchLinkMetrics(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchL2Links_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchL2Links(context.Background(), "default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchSRTEPathDetail_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchSRTEPathDetail(context.Background(), "test-policy")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// ──────────────────────────────
// Token 获取失败路径
// ──────────────────────────────

func TestClient_TokenFetchError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid credentials"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchDevices(context.Background())
	require.Error(t, err)
}

// ──────────────────────────────
// JSON 解码失败路径
// ──────────────────────────────

func TestClientFetchDevicesPaged_InvalidJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not valid json{{{`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchDevicesPaged(context.Background(), 1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestClientFetchPOPList_InvalidJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/popInfos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`invalid`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchPOPList(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestClientFetchVendors_InvalidJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/getAllVendorProdModel", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{broken`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchVendors(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

// ──────────────────────────────
// api_config.go 错误路径
// ──────────────────────────────

func TestClientFetchISISNeighbors_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchISISNeighbors(context.Background(), "NJ-SCT-R01")
	require.Error(t, err)
}

func TestClientFetchBGPPeers_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchBGPPeers(context.Background(), "NJ-SCT-R01")
	require.Error(t, err)
}

func TestClientFetchVPNConfig_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchVPNConfig(context.Background(), "NJ-SCT-R01")
	require.Error(t, err)
}

func TestClientFetchCurrentConfig_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchCurrentConfig(context.Background(), "NJ-SCT-R01")
	require.Error(t, err)
}

func TestClientFetchGlobalRoute_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchGlobalRoute(context.Background(), "NJ-SCT-R01")
	require.Error(t, err)
}

// ──────────────────────────────
// api_monitor.go 错误路径
// ──────────────────────────────

func TestClientFetchDeviceMetrics_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchDeviceMetrics(context.Background(), "NJ-SCT-R01", []string{"cpu_usage"}, time.Now().Add(-time.Hour), time.Now())
	require.Error(t, err)
}

func TestClientFetchPortMetrics_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchPortMetrics(context.Background(), "NJ-SCT-R01", "GE0/0/0", []string{"in_traffic"}, time.Now().Add(-time.Hour), time.Now())
	require.Error(t, err)
}

func TestClientFetchVPNTraffic_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchVPNTraffic(context.Background(), "vpn-001", []string{"in_bytes"}, time.Now().Add(-time.Hour), time.Now())
	require.Error(t, err)
}

func TestClientFetchTunnelTraffic_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchTunnelTraffic(context.Background(), "NJ-SCT-R01", "tunnel-1", []string{"in_bytes"}, time.Now().Add(-time.Hour), time.Now())
	require.Error(t, err)
}

func TestClientFetchSystemLogs_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchSystemLogs(context.Background(), connector.LogQueryOptions{Interval: "1h", PageNum: 1, PageSize: 10})
	require.Error(t, err)
}

func TestClientFetchLoginLogs_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchLoginLogs(context.Background(), connector.LogQueryOptions{Interval: "1h", PageNum: 1, PageSize: 10})
	require.Error(t, err)
}

func TestClientFetchLogs_Error(t *testing.T) {
	server := setupErrorServer(t)
	defer server.Close()
	client := newTestClient(t, server.URL)

	_, err := client.FetchLogs(context.Background(), "system", connector.LogQueryOptions{Interval: "1h", PageNum: 1, PageSize: 10})
	require.Error(t, err)
}
