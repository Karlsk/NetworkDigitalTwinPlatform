package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ──────────────────────────────
// Helper
// ──────────────────────────────

// newTestClient 创建指向 mock server 的测试用 ControllerClient。
func newTestClient(t *testing.T, serverURL string) *ControllerClient {
	httpClient := connector.NewHTTPClient(
		connector.WithBaseURL(serverURL),
		connector.WithAuth(connector.AuthConfig{Type: "bearer", Token: "mock-token"}),
	)
	cfg := map[string]any{
		"base_url":  serverURL,
		"token_url": "/oauth/token",
		"username":  "admin",
		"password":  "test123",
		"device_id": "test-device-id",
	}
	return NewControllerClient("test-controller", httpClient, cfg)
}

// ──────────────────────────────
// ControllerClient 单元测试
// ──────────────────────────────

func TestNewControllerClient(t *testing.T) {
	httpClient := connector.NewHTTPClient(
		connector.WithBaseURL("http://localhost:8080"),
	)
	cfg := map[string]any{
		"base_url":       "http://localhost:8080",
		"token_url":      "/custom/token",
		"username":       "user1",
		"password":       "pass1",
		"device_id":      "dev-123",
		"des_secret_key": "9mng65v8jf4lxn93nabf981m",
	}

	c := NewControllerClient("my-client", httpClient, cfg)
	assert.Equal(t, "my-client", c.name)
	assert.Equal(t, "/custom/token", c.tokenURL)
	assert.Equal(t, "user1", c.username)
	assert.Equal(t, "pass1", c.password)
	assert.Equal(t, "dev-123", c.deviceID)
	assert.Equal(t, "9mng65v8jf4lxn93nabf981m", c.desSecretKey)
	assert.Equal(t, "http://localhost:8080", c.baseURL)
	assert.Equal(t, 5*time.Minute, c.cacheTTL)
}

func TestNewControllerClient_Defaults(t *testing.T) {
	httpClient := connector.NewHTTPClient()
	cfg := map[string]any{
		"username": "user1",
	}

	c := NewControllerClient("default-client", httpClient, cfg)
	assert.Equal(t, "/oauth/token", c.tokenURL)
	assert.Equal(t, "9mng65v8jf4lxn93nabf981m", c.desSecretKey)
	assert.Equal(t, "", c.baseURL)
}

func TestClientPing(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.Ping(context.Background())
	require.NoError(t, err)
}

func TestClientFetchDevices(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	devices, err := client.FetchDevices(context.Background())
	require.NoError(t, err)
	require.Len(t, devices, 2)

	assert.Equal(t, "dev-001", devices[0]["id"])
	assert.Equal(t, "NJ-SCT-R01", devices[0]["name"])
	assert.Equal(t, "H3C", devices[0]["vendor-id"])
	assert.Equal(t, "dev-002", devices[1]["id"])
}

func TestClientFetchDevicesPaged(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchDevicesPaged(context.Background(), 1, 10)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.PageNum)
	assert.Equal(t, 10, result.PageSize)
	assert.Equal(t, 1, result.TotalPages)
	assert.Len(t, result.Content, 1)
	assert.Equal(t, "dev-paged-001", result.Content[0]["id"])
}

func TestClientFetchPOPList(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	pops, err := client.FetchPOPList(context.Background())
	require.NoError(t, err)
	require.Len(t, pops, 1)

	assert.Equal(t, "pop-001", pops[0]["pop-id"])
	assert.Equal(t, "NJ-POP01", pops[0]["pop-name"])
}

func TestClientFetchVendors(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	vendors, err := client.FetchVendors(context.Background())
	require.NoError(t, err)
	require.Len(t, vendors, 2)

	assert.Equal(t, "H3C", vendors[0]["vendor"])
	assert.Equal(t, "ZTE", vendors[1]["vendor"])
}

func TestClientFetchLinks(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	links, err := client.FetchLinks(context.Background())
	require.NoError(t, err)
	require.Len(t, links, 1)

	assert.Contains(t, links[0]["link-id"], "NJ-SCT-R01")
}

func TestClientFetchAlarms(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	alarms, err := client.FetchAlarms(context.Background())
	require.NoError(t, err)
	require.Len(t, alarms, 1)

	assert.Equal(t, "alarm-001", alarms[0]["id"])
	assert.Equal(t, "MAJOR", alarms[0]["level"])
}

func TestClientFetchAlarms_NullData(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/monitor/alert/list", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": nil})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	alarms, err := client.FetchAlarms(context.Background())
	require.NoError(t, err)
	assert.Nil(t, alarms) // null data 应返回 nil
}

func TestClientFetchTunnels(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	tunnels, err := client.FetchTunnels(context.Background())
	require.NoError(t, err)
	require.Len(t, tunnels, 1)

	assert.Equal(t, "tunnel-001", tunnels[0]["instance-id"])
}

func TestClientFetchL3VPNs(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	vpns, err := client.FetchL3VPNs(context.Background())
	require.NoError(t, err)
	require.Len(t, vpns, 1) // flattenVPNItems 提取出 1 条

	assert.Equal(t, "l3_3307", vpns[0]["vpn-id"])
}

func TestClientFetchL2VPNs(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	vpns, err := client.FetchL2VPNs(context.Background())
	require.NoError(t, err)
	require.Len(t, vpns, 1)

	assert.Equal(t, "l2_3310", vpns[0]["vpn-id"])
}

func TestClientFetchISISNeighbors(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	text, err := client.FetchISISNeighbors(context.Background(), "NJ-SCT-R01")
	require.NoError(t, err)
	assert.Contains(t, text, "NJ-SCT-R02")
	assert.Contains(t, text, "System ID")
}

func TestClientFetchBGPPeers(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	text, err := client.FetchBGPPeers(context.Background(), "NJ-SCT-R01")
	require.NoError(t, err)
	assert.Contains(t, text, "172.16.11.4")
	assert.Contains(t, text, "Established")
}

func TestClientFetchVPNConfig(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	text, err := client.FetchVPNConfig(context.Background(), "NJ-SCT-R01")
	require.NoError(t, err)
	assert.Equal(t, "vpn-config-output", text)
}

func TestClientFetchCurrentConfig(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	text, err := client.FetchCurrentConfig(context.Background(), "NJ-SCT-R01")
	require.NoError(t, err)
	assert.Equal(t, "current-config-output", text)
}

func TestClientFetchGlobalRoute(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	text, err := client.FetchGlobalRoute(context.Background(), "NJ-SCT-R01")
	require.NoError(t, err)
	assert.Equal(t, "global-route-output", text)
}

// ──────────────────────────────
// 错误路径测试
// ──────────────────────────────

func TestClientFetchDevices_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/peInfos", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchDevices(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestClientFetchLinks_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/sr/config/network-topology:network-topology/topology/linksInfo", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchLinks(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestClientTokenAutoRefresh(t *testing.T) {
	var tokenCount int
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		tokenCount++
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("token-%d", tokenCount),
			"expires_in":   1, // 1 秒后过期，触发自动刷新
		})
	})
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/peInfos", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"peInfo": []map[string]any{}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)

	// 第一次调用触发 Token 获取
	_, err := client.FetchDevices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, tokenCount)

	// 等待 Token 过期（1s 的 token 立刻被认为过期）
	time.Sleep(100 * time.Millisecond)

	// 第二次调用应触发刷新
	_, err = client.FetchDevices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, tokenCount)
}

func TestClientPing_Unreachable(t *testing.T) {
	httpClient := connector.NewHTTPClient(
		connector.WithBaseURL("http://127.0.0.1:1"),
		connector.WithAuth(connector.AuthConfig{Type: "bearer", Token: "tok"}),
	)
	cfg := map[string]any{
		"base_url":  "http://127.0.0.1:1",
		"token_url": "/oauth/token",
		"username":  "u",
		"password":  "p",
		"device_id": "d",
	}
	client := NewControllerClient("test-unreachable", httpClient, cfg)

	err := client.Ping(context.Background())
	require.Error(t, err)
}

// ──────────────────────────────
// 加密测试（从 controller_test.go 迁移）
// ──────────────────────────────

func TestClientEncryptPassword(t *testing.T) {
	secretKey := "9mng65v8jf4lxn93nabf981m"
	password := "tgb.258"
	expected := "gXpi7pWvZNA="

	result, err := encryptPassword(secretKey, password)
	require.NoError(t, err)
	assert.Equal(t, expected, result, "3DES-CBC-SHA256 IV encryption mismatch")
}

func TestClientEncryptPasswordInvalidKeyLength(t *testing.T) {
	_, err := encryptPassword("short", "password")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "24 bytes")
}
