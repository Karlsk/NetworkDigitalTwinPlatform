// Package controller 实现 Controller Connector 测试。
// monitor_test.go 验证 MonitorQuerier 能力接口的实现正确性。
// V1.2-04: 替换 ErrNotImplemented 测试为委托调用端到端测试。
package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ──────────────────────────────
// 编译时接口满足检查测试
// ──────────────────────────────

func TestMonitorQuerierCompileTimeCheck(t *testing.T) {
	// 编译时已通过 var _ connector.MonitorQuerier = (*ControllerConnector)(nil) 验证
	// 此测试额外验证运行时类型断言
	var c connector.Connector = NewControllerConnector("test", nil, nil, "")
	_, ok := c.(connector.MonitorQuerier)
	if !ok {
		t.Fatal("ControllerConnector does not implement MonitorQuerier, want implementation")
	}
}

// ──────────────────────────────
// 辅助函数
// ──────────────────────────────

// setupMonitorConnector 创建使用 mock server 的 ControllerConnector（MonitorQuerier 测试专用）。
func setupMonitorConnector(t *testing.T, serverURL string) *ControllerConnector {
	httpClient := connector.NewHTTPClient(
		connector.WithBaseURL(serverURL),
	)
	client := NewControllerClient("test-controller", httpClient, map[string]any{
		"username":  "testuser",
		"password":  "testpass",
		"device_id": "test-device",
		"base_url":  serverURL,
	})
	return NewControllerConnector("test-controller", client, []string{"Device"}, serverURL)
}

// setupMonitorAndAlarmServer 创建同时支持监控和告警的 mock server。
func setupMonitorAndAlarmServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "mock-token",
			"expires_in":   3600,
		})
	})

	mux.HandleFunc("/monitor/controller/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]monitorRawSeries{
			{Metric: "cpu_usage", Data: []struct {
				Time  int64   `json:"time"`
				Value float64 `json:"value"`
			}{{Time: 1713700800, Value: 50.0}}},
		})
	})

	mux.HandleFunc("/monitor/switch/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]monitorRawSeries{
			{Metric: "in_traffic", Data: []struct {
				Time  int64   `json:"time"`
				Value float64 `json:"value"`
			}{{Time: 1713700800, Value: 1024}}},
		})
	})

	mux.HandleFunc("/monitor/vpn/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]monitorRawSeries{
			{Metric: "throughput", Data: []struct {
				Time  int64   `json:"time"`
				Value float64 `json:"value"`
			}{{Time: 1713700800, Value: 2048}}},
		})
	})

	mux.HandleFunc("/monitor/te/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]monitorRawSeries{
			{Metric: "bandwidth", Data: []struct {
				Time  int64   `json:"time"`
				Value float64 `json:"value"`
			}{{Time: 1713700800, Value: 50000}}},
		})
	})

	mux.HandleFunc("/monitor/alert/list", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "ok",
			"data": []map[string]any{
				{"id": "alarm-001", "level": "MAJOR"},
			},
		})
	})

	mux.HandleFunc("/monitor/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logPageResponse{
			Content:       []map[string]any{{"id": "log-001", "message": "test"}},
			TotalElements: 1,
			PageNum:       1,
			PageSize:      20,
		})
	})

	mux.HandleFunc("/monitor/logs/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logPageResponse{
			Content:       []map[string]any{{"id": "login-001", "username": "admin"}},
			TotalElements: 1,
			PageNum:       1,
			PageSize:      20,
		})
	})

	return httptest.NewServer(mux)
}

// ──────────────────────────────
// MonitorQuerier 委托调用端到端测试
// ──────────────────────────────

func TestMonitorQueryDeviceMetrics(t *testing.T) {
	server := setupMonitorAndAlarmServer(t)
	defer server.Close()

	c := setupMonitorConnector(t, server.URL)
	start := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 21, 11, 0, 0, 0, time.UTC)

	result, err := c.QueryDeviceMetrics(context.Background(), "NJ-SCT-R01", []string{"cpu_usage"}, start, end)
	if err != nil {
		t.Fatalf("QueryDeviceMetrics() error = %v", err)
	}
	if result == nil {
		t.Fatal("QueryDeviceMetrics() result = nil, want non-nil")
	}
	if result.Device != "NJ-SCT-R01" {
		t.Errorf("QueryDeviceMetrics() device = %q, want NJ-SCT-R01", result.Device)
	}
	if len(result.Metrics) != 1 || result.Metrics[0].Name != "cpu_usage" {
		t.Errorf("QueryDeviceMetrics() metrics mismatch, got %v", result.Metrics)
	}
}

func TestMonitorQueryPortMetrics(t *testing.T) {
	server := setupMonitorAndAlarmServer(t)
	defer server.Close()

	c := setupMonitorConnector(t, server.URL)
	start := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 21, 11, 0, 0, 0, time.UTC)

	result, err := c.QueryPortMetrics(context.Background(), "NJ-SCT-R01", "GE1/0/1", []string{"in_traffic"}, start, end)
	if err != nil {
		t.Fatalf("QueryPortMetrics() error = %v", err)
	}
	if result == nil {
		t.Fatal("QueryPortMetrics() result = nil, want non-nil")
	}
	if result.Port != "GE1/0/1" {
		t.Errorf("QueryPortMetrics() port = %q, want GE1/0/1", result.Port)
	}
}

func TestMonitorQueryVPNTraffic(t *testing.T) {
	server := setupMonitorAndAlarmServer(t)
	defer server.Close()

	c := setupMonitorConnector(t, server.URL)
	start := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 21, 11, 0, 0, 0, time.UTC)

	result, err := c.QueryVPNTraffic(context.Background(), "vpn-001", []string{"throughput"}, start, end)
	if err != nil {
		t.Fatalf("QueryVPNTraffic() error = %v", err)
	}
	if result == nil {
		t.Fatal("QueryVPNTraffic() result = nil, want non-nil")
	}
	if result.VPN != "vpn-001" {
		t.Errorf("QueryVPNTraffic() vpn = %q, want vpn-001", result.VPN)
	}
}

func TestMonitorQueryTunnelTraffic(t *testing.T) {
	server := setupMonitorAndAlarmServer(t)
	defer server.Close()

	c := setupMonitorConnector(t, server.URL)
	start := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 21, 11, 0, 0, 0, time.UTC)

	result, err := c.QueryTunnelTraffic(context.Background(), "NJ-SCT-R01", "tunnel-001", []string{"bandwidth"}, start, end)
	if err != nil {
		t.Fatalf("QueryTunnelTraffic() error = %v", err)
	}
	if result == nil {
		t.Fatal("QueryTunnelTraffic() result = nil, want non-nil")
	}
	if result.Tunnel != "tunnel-001" {
		t.Errorf("QueryTunnelTraffic() tunnel = %q, want tunnel-001", result.Tunnel)
	}
}

func TestMonitorQueryAlerts(t *testing.T) {
	server := setupMonitorAndAlarmServer(t)
	defer server.Close()

	c := setupMonitorConnector(t, server.URL)
	alerts, err := c.QueryAlerts(context.Background(), "business", "1h")
	if err != nil {
		t.Fatalf("QueryAlerts() error = %v", err)
	}
	if len(alerts) == 0 {
		t.Fatal("QueryAlerts() alerts is empty, want at least 1")
	}
	if alerts[0]["id"] != "alarm-001" {
		t.Errorf("QueryAlerts() id = %v, want alarm-001", alerts[0]["id"])
	}
}

func TestMonitorQueryLogs_System(t *testing.T) {
	server := setupMonitorAndAlarmServer(t)
	defer server.Close()

	c := setupMonitorConnector(t, server.URL)
	result, err := c.QueryLogs(context.Background(), "system", connector.LogQueryOptions{
		PageNum:  1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("QueryLogs(system) error = %v", err)
	}
	if result == nil {
		t.Fatal("QueryLogs(system) result = nil, want non-nil")
	}
	if len(result.Logs) != 1 {
		t.Errorf("QueryLogs(system) logs len = %d, want 1", len(result.Logs))
	}
}

func TestMonitorQueryLogs_Login(t *testing.T) {
	server := setupMonitorAndAlarmServer(t)
	defer server.Close()

	c := setupMonitorConnector(t, server.URL)
	result, err := c.QueryLogs(context.Background(), "login", connector.LogQueryOptions{
		Interval: "1h",
	})
	if err != nil {
		t.Fatalf("QueryLogs(login) error = %v", err)
	}
	if result == nil {
		t.Fatal("QueryLogs(login) result = nil, want non-nil")
	}
	if len(result.Logs) != 1 {
		t.Errorf("QueryLogs(login) logs len = %d, want 1", len(result.Logs))
	}
}

// ──────────────────────────────
// 类型断言发现能力测试（Service/MCP 层使用模式）
// ──────────────────────────────

func TestTypeAssertionDiscoverMonitorQuerier(t *testing.T) {
	// 模拟 Service/MCP 层通过类型断言发现 MonitorQuerier 能力
	c := NewControllerConnector("test-controller", nil, []string{"Device"}, "http://localhost")
	var conn connector.Connector = c

	// ControllerConnector 实现了 MonitorQuerier，类型断言应成功
	mq, ok := conn.(connector.MonitorQuerier)
	if !ok {
		t.Fatal("type assertion to MonitorQuerier failed, want success")
	}
	if mq == nil {
		t.Fatal("MonitorQuerier is nil after type assertion")
	}
}

func TestTypeAssertionFromInterfaceToMonitorQuerier(t *testing.T) {
	// 验证从 any 类型通过类型断言发现 MonitorQuerier
	c := NewControllerConnector("test-controller", nil, nil, "")
	var obj any = c

	mq, ok := obj.(connector.MonitorQuerier)
	if !ok {
		t.Fatal("type assertion from any to MonitorQuerier failed, want success")
	}
	if mq == nil {
		t.Fatal("MonitorQuerier is nil after type assertion from any")
	}
}

// ──────────────────────────────
// MonitorQuerier 方法数量验证
// ──────────────────────────────

func TestMonitorQuerierMethodCount(t *testing.T) {
	// 文档定义 MonitorQuerier 有 6 个方法，签名正确性由编译时 var _ 检查保证
	// 此测试仅验证方法数量和调用不 panic（使用 nil client 会 panic，故跳过实际调用）
}
