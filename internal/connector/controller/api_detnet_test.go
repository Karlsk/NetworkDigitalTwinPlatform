// Package controller 实现 Controller Connector 测试。
// api_detnet_test.go 验证确定性网络（DetNet）API 方法集的实现正确性。
package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/connector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ──────────────────────────────
// Mock Server Setup
// ──────────────────────────────

// setupDetNetMockServer 创建确定性网络 API 测试所需的 mock server。
func setupDetNetMockServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()

	// Token 接口
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "mock-token-detnet",
			"expires_in":   3600,
		})
	})

	// 实例列表 + 创建
	mux.HandleFunc("/api/no/config/terra-h3c-detnet/ip/service", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "detnet-new", "status": "created"})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/no/config/terra-h3c-detnet/ip/service/all", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "detnet-001", "name": "path-probe-1", "status": "active"},
			{"id": "detnet-002", "name": "path-probe-2", "status": "inactive"},
		})
	})

	// 更新 / 删除（带 id 路径）
	mux.HandleFunc("/api/no/config/terra-h3c-detnet/ip/service/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// OAM 数据查询
	mux.HandleFunc("/api/no/config/terra-h3c-detnet/ip/service/oam", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "detnet-001", r.URL.Query().Get("id"))
		assert.NotEmpty(t, r.URL.Query().Get("interval-minutes"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"timestamp": 1713700800, "delay_ns": 1500, "jitter_ns": 200, "loss_rate": 0.001},
			{"timestamp": 1713704400, "delay_ns": 1600, "jitter_ns": 180, "loss_rate": 0.0005},
		})
	})

	// 时隙重启
	mux.HandleFunc("/api/no/config/terra-h3c-detnet/ip/service/restart-timeslot-calculation", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.NotEmpty(t, r.URL.Query().Get("id"))
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"status": "restarted"})
	})

	// SR-TE 路径详情
	mux.HandleFunc("/api/sr/config/terra-te-svc:te-policy-instance/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"instance-id": "tunnel-001",
			"name":        "SR-TE-Policy-1",
			"status":      "active",
			"head-end":    "NJ-SCT-R01",
			"tail-end":    "NJ-SCT-R02",
		})
	})

	return httptest.NewServer(mux)
}

// ──────────────────────────────
// DetNet 正向测试
// ──────────────────────────────

func TestListDetNetInstancesClient(t *testing.T) {
	server := setupDetNetMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListDetNetInstances(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "detnet-001", result[0]["id"])
	assert.Equal(t, "path-probe-1", result[0]["name"])
}

func TestCreateDetNetInstance(t *testing.T) {
	server := setupDetNetMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.CreateDetNetInstance(context.Background(), map[string]any{
		"name":       "new-probe",
		"src-device": "NJ-SCT-R01",
		"dst-device": "NJ-SCT-R02",
	})
	require.NoError(t, err)
	assert.Equal(t, "detnet-new", result["id"])
}

func TestUpdateDetNetInstance(t *testing.T) {
	server := setupDetNetMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.UpdateDetNetInstance(context.Background(), "detnet-001", map[string]any{
		"name": "updated-probe",
	})
	require.NoError(t, err)
}

func TestDeleteDetNetInstance(t *testing.T) {
	server := setupDetNetMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteDetNetInstance(context.Background(), "detnet-001")
	require.NoError(t, err)
}

func TestFetchDetNetOAMData(t *testing.T) {
	server := setupDetNetMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchDetNetOAMData(context.Background(), "detnet-001", 5)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, 1713700800, int(result[0]["timestamp"].(float64)))
}

func TestRestartDetNetTimeslot(t *testing.T) {
	server := setupDetNetMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.RestartDetNetTimeslot(context.Background(), "detnet-001")
	require.NoError(t, err)
}

// ──────────────────────────────
// SR-TE 测试
// ──────────────────────────────

func TestFetchSRTEPathDetail(t *testing.T) {
	server := setupDetNetMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.FetchSRTEPathDetail(context.Background(), "tunnel-001")
	require.NoError(t, err)
	assert.Equal(t, "tunnel-001", result["instance-id"])
	assert.Equal(t, "SR-TE-Policy-1", result["name"])
}

func TestComputeSRTEPath_NotImplemented(t *testing.T) {
	client := newTestClient(t, "http://unused")
	_, err := client.ComputeSRTEPath(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, connector.ErrNotImplemented)
}

func TestCreateSRTEPolicy_NotImplemented(t *testing.T) {
	client := newTestClient(t, "http://unused")
	_, err := client.CreateSRTEPolicy(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, connector.ErrNotImplemented)
}

// ──────────────────────────────
// 错误路径测试
// ──────────────────────────────

func TestListDetNetInstances_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-h3c-detnet/ip/service/all", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListDetNetInstances(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestFetchDetNetOAMData_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-h3c-detnet/ip/service/oam", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.FetchDetNetOAMData(context.Background(), "nonexistent", 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestDeleteDetNetInstance_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-h3c-detnet/ip/service/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteDetNetInstance(context.Background(), "detnet-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
