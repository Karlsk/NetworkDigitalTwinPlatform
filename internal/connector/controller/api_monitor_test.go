// Package controller 实现 Controller Connector 测试。
// api_monitor_test.go 验证监控 API 方法集的实现正确性。
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
// Mock Server Setup
// ──────────────────────────────

// setupMonitorMockServer 创建监控 API 测试所需的 mock server。
func setupMonitorMockServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()

	// Token 接口
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "mock-token-monitor",
			"expires_in":   3600,
		})
	})

	// 设备级监控
	mux.HandleFunc("/monitor/controller/history", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		// 验证参数
		assert.Equal(t, "system", r.URL.Query().Get("namespace"))
		assert.NotEmpty(t, r.URL.Query().Get("metricNames"))
		assert.NotEmpty(t, r.URL.Query().Get("startTime"))
		assert.NotEmpty(t, r.URL.Query().Get("endTime"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]monitorRawSeries{
			{
				Metric: "cpu_usage",
				Data: []struct {
					Time  int64   `json:"time"`
					Value float64 `json:"value"`
				}{
					{Time: 1713700800, Value: 45.2},
					{Time: 1713704400, Value: 52.8},
				},
			},
		})
	})

	// 端口级监控
	mux.HandleFunc("/monitor/switch/history", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "port", r.URL.Query().Get("namespace"))
		assert.Equal(t, "switch", r.URL.Query().Get("dimensions.0.name"))
		assert.NotEmpty(t, r.URL.Query().Get("dimensions.0.value"))
		assert.Equal(t, "port", r.URL.Query().Get("dimensions.1.name"))
		assert.NotEmpty(t, r.URL.Query().Get("dimensions.1.value"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]monitorRawSeries{
			{
				Metric: "in_traffic",
				Data: []struct {
					Time  int64   `json:"time"`
					Value float64 `json:"value"`
				}{
					{Time: 1713700800, Value: 1024000},
				},
			},
		})
	})

	// VPN 流量
	mux.HandleFunc("/monitor/vpn/history", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "traffic", r.URL.Query().Get("namespace"))
		assert.Equal(t, "vpnId", r.URL.Query().Get("dimensions.0.name"))
		assert.NotEmpty(t, r.URL.Query().Get("dimensions.0.value"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]monitorRawSeries{
			{
				Metric: "in_traffic",
				Data: []struct {
					Time  int64   `json:"time"`
					Value float64 `json:"value"`
				}{
					{Time: 1713700800, Value: 2048000},
				},
			},
		})
	})

	// 隧道流量
	mux.HandleFunc("/monitor/te/history", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "traffic", r.URL.Query().Get("namespace"))
		assert.Equal(t, "deviceName", r.URL.Query().Get("dimensions.0.name"))
		assert.Equal(t, "tunnelName", r.URL.Query().Get("dimensions.1.name"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]monitorRawSeries{
			{
				Metric: "bandwidth",
				Data: []struct {
					Time  int64   `json:"time"`
					Value float64 `json:"value"`
				}{
					{Time: 1713700800, Value: 50000},
				},
			},
		})
	})

	// 系统日志
	mux.HandleFunc("/monitor/logs", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logPageResponse{
			Content: []map[string]any{
				{"id": "log-001", "level": "INFO", "message": "system started"},
			},
			TotalElements: 1,
			PageNum:       1,
			PageSize:      20,
		})
	})

	// 登录日志
	mux.HandleFunc("/monitor/logs/login", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logPageResponse{
			Content: []map[string]any{
				{"id": "login-001", "username": "admin", "ip": "10.0.0.1"},
			},
			TotalElements: 1,
			PageNum:       1,
			PageSize:      20,
		})
	})

	// 完整拓扑
	mux.HandleFunc("/api/sr/config/network-topology:network-topology", func(w http.ResponseWriter, r *http.Request) {
		// 只匹配精确路径，不匹配子路径
		if r.URL.Path != "/api/sr/config/network-topology:network-topology" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"nodes": []map[string]any{
				{"node-id": "NJ-SCT-R01", "type": "PE"},
				{"node-id": "NJ-SCT-R02", "type": "P"},
			},
			"links": []map[string]any{
				{"link-id": "link-001", "status": "UP"},
			},
		})
	})

	// 拓扑节点
	mux.HandleFunc("/api/sr/config/network-topology:network-topology/nodes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"node-id": "NJ-SCT-R01", "type": "PE"},
		})
	})

	// 拓扑链路
	mux.HandleFunc("/api/sr/config/network-topology:network-topology/links", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"link-id": "link-001", "status": "UP"},
		})
	})

	// 链路指标
	mux.HandleFunc("/api/sr/config/network-topology:network-topology/links-metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"link-id": "link-001", "utilization": 0.15},
		})
	})

	// 二层链路（使用前缀匹配）
	mux.HandleFunc("/api/sr/config/network-topology:network-topology/topology/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"l2link-id": "l2-001", "src-port": "GE1/0/1", "dst-port": "GE2/0/1"},
		})
	})

	return httptest.NewServer(mux)
}

// ──────────────────────────────
// 辅助函数测试
// ──────────────────────────────

func TestFormatMonitorTime(t *testing.T) {
	tm := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	result := formatMonitorTime(tm)
	assert.Equal(t, "2026-04-21 10:00:00", result)
}

func TestFormatMonitorTime_WithSeconds(t *testing.T) {
	tm := time.Date(2026, 1, 15, 14, 30, 45, 0, time.UTC)
	result := formatMonitorTime(tm)
	assert.Equal(t, "2026-01-15 14:30:45", result)
}

func TestBuildMonitorURL(t *testing.T) {
	params := map[string]string{
		"namespace":   "system",
		"metricNames": "cpu_usage",
	}
	result := buildMonitorURL("/monitor/controller/history", params)
	assert.Contains(t, result, "/monitor/controller/history?")
	assert.Contains(t, result, "namespace=system")
	assert.Contains(t, result, "metricNames=cpu_usage")
}

func TestBuildMonitorURL_EmptyParams(t *testing.T) {
	result := buildMonitorURL("/api/test", nil)
	assert.Equal(t, "/api/test", result)
}

// ──────────────────────────────
// 监控 API 正向测试
// ──────────────────────────────

func TestFetchDeviceMetrics(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	start := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 21, 11, 0, 0, 0, time.UTC)

	result, err := client.FetchDeviceMetrics(context.Background(), "NJ-SCT-R01", []string{"cpu_usage"}, start, end)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "NJ-SCT-R01", result.Device)
	require.Len(t, result.Metrics, 1)
	assert.Equal(t, "cpu_usage", result.Metrics[0].Name)
	require.Len(t, result.Metrics[0].DataPoints, 2)
	assert.Equal(t, 45.2, result.Metrics[0].DataPoints[0].Value)
}

func TestFetchDeviceMetrics_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/monitor/controller/history", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchDeviceMetrics(context.Background(), "dev1", []string{"cpu"}, time.Now(), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestFetchPortMetrics(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	start := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 21, 11, 0, 0, 0, time.UTC)

	result, err := client.FetchPortMetrics(context.Background(), "NJ-SCT-R01", "GE1/0/1", []string{"in_traffic"}, start, end)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "NJ-SCT-R01", result.Device)
	assert.Equal(t, "GE1/0/1", result.Port)
	require.Len(t, result.Metrics, 1)
	assert.Equal(t, "in_traffic", result.Metrics[0].Name)
}

func TestFetchPortMetrics_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/monitor/switch/history", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchPortMetrics(context.Background(), "dev1", "port1", []string{"in_traffic"}, time.Now(), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestFetchVPNTraffic(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	start := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 21, 11, 0, 0, 0, time.UTC)

	result, err := client.FetchVPNTraffic(context.Background(), "vpn-001", []string{"in_traffic"}, start, end)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "vpn-001", result.VPN)
	require.Len(t, result.Metrics, 1)
}

func TestFetchTunnelTraffic(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	start := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 21, 11, 0, 0, 0, time.UTC)

	result, err := client.FetchTunnelTraffic(context.Background(), "NJ-SCT-R01", "tunnel-001", []string{"bandwidth"}, start, end)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "NJ-SCT-R01", result.Device)
	assert.Equal(t, "tunnel-001", result.Tunnel)
	require.Len(t, result.Metrics, 1)
	assert.Equal(t, "bandwidth", result.Metrics[0].Name)
}

func TestFetchSystemLogs(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchSystemLogs(context.Background(), connector.LogQueryOptions{
		PageNum:  1,
		PageSize: 20,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, result.Logs, 1)
	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, "log-001", result.Logs[0]["id"])
}

func TestFetchLoginLogs(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchLoginLogs(context.Background(), connector.LogQueryOptions{
		Interval: "1h",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, result.Logs, 1)
	assert.Equal(t, "login-001", result.Logs[0]["id"])
}

func TestFetchLogs_System(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchLogs(context.Background(), "system", connector.LogQueryOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Logs, 1)
}

func TestFetchLogs_Login(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchLogs(context.Background(), "login", connector.LogQueryOptions{Interval: "1h"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Logs, 1)
}

func TestFetchLogs_Default(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	// 空 logType 走 system 分支
	result, err := client.FetchLogs(context.Background(), "", connector.LogQueryOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Logs, 1)
}

func TestFetchTopology(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchTopology(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, result.Nodes, 2)
	assert.Len(t, result.Links, 1)
	assert.Equal(t, "NJ-SCT-R01", result.Nodes[0]["node-id"])
}

func TestFetchTopologyNodes(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchTopologyNodes(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "NJ-SCT-R01", result[0]["node-id"])
}

func TestFetchTopologyLinks(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchTopologyLinks(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "link-001", result[0]["link-id"])
}

func TestFetchLinkMetrics(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchLinkMetrics(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "link-001", result[0]["link-id"])
}

func TestFetchL2Links(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchL2Links(context.Background(), "default")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "l2-001", result[0]["l2link-id"])
}

// ──────────────────────────────
// 错误路径测试
// ──────────────────────────────

func TestFetchTopology_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/sr/config/network-topology:network-topology", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchTopology(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestFetchSystemLogs_DefaultPagination(t *testing.T) {
	server := setupMonitorMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	// PageNum/PageSize 为 0 时使用默认值
	result, err := client.FetchSystemLogs(context.Background(), connector.LogQueryOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)
}
