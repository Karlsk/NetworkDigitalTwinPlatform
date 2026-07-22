// Package controller 实现 Controller Connector 测试。
// api_srte_test.go 验证 SR-TE API 和 parseMonitorResponse 边界路径。
package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ──────────────────────────────
// SRTE API 测试
// ──────────────────────────────

func TestFetchSRTEPathDetail_Object(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/sr/config/terra-te-svc:te-policy-instance/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"name": "policy-1", "status": "UP",
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchSRTEPathDetail(context.Background(), "policy-1")
	require.NoError(t, err)
	assert.Equal(t, "policy-1", result["name"])
}

func TestFetchSRTEPathDetail_Array(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/sr/config/terra-te-svc:te-policy-instance/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"name": "first"}, {"name": "second"},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchSRTEPathDetail(context.Background(), "policy-1")
	require.NoError(t, err)
	assert.Equal(t, "first", result["name"])
}

func TestFetchSRTEPathDetail_EmptyArray(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/sr/config/terra-te-svc:te-policy-instance/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchSRTEPathDetail(context.Background(), "policy-1")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestFetchSRTEPathDetail_InvalidJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/sr/config/terra-te-svc:te-policy-instance/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("not json at all"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchSRTEPathDetail(context.Background(), "policy-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected format")
}

func TestFetchSRTEPathDetail_StatusError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/sr/config/terra-te-svc:te-policy-instance/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchSRTEPathDetail(context.Background(), "policy-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestComputeSRTEPath(t *testing.T) {
	client := &ControllerClient{}
	result, err := client.ComputeSRTEPath(context.Background(), nil)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, connector.ErrNotImplemented)
}

func TestCreateSRTEPolicy(t *testing.T) {
	client := &ControllerClient{}
	result, err := client.CreateSRTEPolicy(context.Background(), nil)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, connector.ErrNotImplemented)
}

// ──────────────────────────────
// parseMonitorResponse 边界测试
// ──────────────────────────────

func TestParseMonitorResponse_EmptyBody(t *testing.T) {
	body := io.NopCloser(strings.NewReader(""))
	result, err := parseMonitorResponse(body, "dev1")
	require.NoError(t, err)
	assert.Equal(t, "dev1", result.Device)
	assert.Empty(t, result.Metrics)
}

func TestParseMonitorResponse_ArrayFormat(t *testing.T) {
	data := `[{"metric":"cpu","data":[{"time":1713700800,"value":42.0}]}]`
	body := io.NopCloser(strings.NewReader(data))
	result, err := parseMonitorResponse(body, "dev1")
	require.NoError(t, err)
	require.Len(t, result.Metrics, 1)
	assert.Equal(t, "cpu", result.Metrics[0].Name)
	require.Len(t, result.Metrics[0].DataPoints, 1)
	assert.Equal(t, 42.0, result.Metrics[0].DataPoints[0].Value)
}

func TestParseMonitorResponse_ObjectWrapper(t *testing.T) {
	data := `{"series":[{"metric":"mem","data":[{"time":1713700800,"value":80.0}]}]}`
	body := io.NopCloser(strings.NewReader(data))
	result, err := parseMonitorResponse(body, "dev1")
	require.NoError(t, err)
	require.Len(t, result.Metrics, 1)
	assert.Equal(t, "mem", result.Metrics[0].Name)
}

func TestParseMonitorResponse_InvalidJSON(t *testing.T) {
	body := io.NopCloser(strings.NewReader("not json"))
	_, err := parseMonitorResponse(body, "dev1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode monitor response")
}

func TestParseMonitorResponse_ObjectNoArray(t *testing.T) {
	// Object format but no array fields → empty result
	data := `{"info":"some metadata","count":5}`
	body := io.NopCloser(strings.NewReader(data))
	result, err := parseMonitorResponse(body, "dev1")
	require.NoError(t, err)
	assert.Equal(t, "dev1", result.Device)
	assert.Empty(t, result.Metrics)
}

// ──────────────────────────────
// api_config 错误路径测试
// ──────────────────────────────

func TestFetchISISNeighbors_StatusError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/restconf/operations/oper-rpc:isis-neighbor", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchISISNeighbors(context.Background(), "PE-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestFetchBGPPeers_StatusError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/restconf/operations/oper-rpc:bgp-peer-config", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchBGPPeers(context.Background(), "PE-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestFetchVPNConfig_StatusError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/restconf/operations/oper-rpc:vpn-config", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchVPNConfig(context.Background(), "PE-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestFetchCurrentConfig_StatusError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/restconf/operations/oper-rpc:current-config", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchCurrentConfig(context.Background(), "PE-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestFetchGlobalRoute_StatusError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/restconf/operations/oper-rpc:global-route", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchGlobalRoute(context.Background(), "PE-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

// ──────────────────────────────
// 监控 API 更多错误路径
// ──────────────────────────────

func TestFetchVPNTraffic_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/monitor/vpn/history", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchVPNTraffic(context.Background(), "vpn-1", []string{"in"}, time.Now(), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestFetchTunnelTraffic_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/monitor/te/history", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchTunnelTraffic(context.Background(), "dev1", "tun1", []string{"bw"}, time.Now(), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestFetchLoginLogs_404ReturnsEmpty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/monitor/logs/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchLoginLogs(context.Background(), connector.LogQueryOptions{})
	require.NoError(t, err)
	assert.Empty(t, result.Logs)
}

func TestFetchLoginLogs_StatusError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/monitor/logs/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchLoginLogs(context.Background(), connector.LogQueryOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestFetchSystemLogs_StatusError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/monitor/logs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchSystemLogs(context.Background(), connector.LogQueryOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestFetchTopologyNodes_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/sr/config/network-topology:network-topology/nodes", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchTopologyNodes(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestFetchTopologyLinks_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/sr/config/network-topology:network-topology/links", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchTopologyLinks(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestFetchLinkMetrics_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/sr/config/network-topology:network-topology/links-metrics", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchLinkMetrics(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestFetchL2Links_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/sr/config/network-topology:network-topology/topology/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchL2Links(context.Background(), "default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// ──────────────────────────────
// buildMetricsFromArray 测试
// ──────────────────────────────

func TestBuildMetricsFromArray(t *testing.T) {
	series := []monitorRawSeries{
		{
			Metric: "cpu",
			Data: []struct {
				Time  int64   `json:"time"`
				Value float64 `json:"value"`
			}{
				{Time: 1713700800, Value: 50.0},
				{Time: 1713704400, Value: 60.0},
			},
		},
		{
			Metric: "mem",
			Data: []struct {
				Time  int64   `json:"time"`
				Value float64 `json:"value"`
			}{
				{Time: 1713700800, Value: 80.0},
			},
		},
	}
	result := buildMetricsFromArray(series, "router-1")
	assert.Equal(t, "router-1", result.Device)
	require.Len(t, result.Metrics, 2)
	assert.Equal(t, "cpu", result.Metrics[0].Name)
	assert.Len(t, result.Metrics[0].DataPoints, 2)
	assert.Equal(t, "mem", result.Metrics[1].Name)
	assert.Len(t, result.Metrics[1].DataPoints, 1)
}

func TestBuildMetricsFromArray_Empty(t *testing.T) {
	result := buildMetricsFromArray(nil, "dev1")
	assert.Equal(t, "dev1", result.Device)
	assert.Empty(t, result.Metrics)
}

// ──────────────────────────────
// FetchSystemLogs/FetchLoginLogs 时间默认值
// ──────────────────────────────

func TestFetchSystemLogs_WithExplicitTime(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/monitor/logs", func(w http.ResponseWriter, r *http.Request) {
		// 验证传入了显式时间
		assert.NotEmpty(t, r.URL.Query().Get("startTime"))
		assert.NotEmpty(t, r.URL.Query().Get("endTime"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logPageResponse{
			Content:       []map[string]any{},
			TotalElements: 0,
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchSystemLogs(context.Background(), connector.LogQueryOptions{
		StartTime: time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 21, 11, 0, 0, 0, time.UTC),
		PageNum:   2,
		PageSize:  50,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.TotalCount)
}

func TestFetchLoginLogs_WithExplicitTime(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/monitor/logs/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logPageResponse{
			Content:       []map[string]any{},
			TotalElements: 0,
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchLoginLogs(context.Background(), connector.LogQueryOptions{
		StartTime: time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 4, 21, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	assert.Empty(t, result.Logs)
}
