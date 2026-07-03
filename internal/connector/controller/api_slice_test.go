// Package controller 实现 Controller Connector 测试。
// api_slice_test.go 验证切片管理 API 方法集的实现正确性。
package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ──────────────────────────────
// Mock Server Setup
// ──────────────────────────────

// setupSliceMockServer 创建切片管理 API 测试所需的 mock server。
func setupSliceMockServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()

	// Token 接口
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "mock-token-slice",
			"expires_in":   3600,
		})
	})

	// ── FlexE Group ──

	mux.HandleFunc("/api/no/config/terra-flexe:flexe/flexe-group", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": "fg-001", "deviceName": "NJ-SCT-R01", "groupName": "FG1"},
			})
		case http.MethodPost:
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "fg-new", "status": "created"})
		case http.MethodPut:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// FlexE Group DELETE（带尾部斜杠）
	mux.HandleFunc("/api/no/config/terra-flexe:flexe/flexe-group/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// ── FlexE Client ──

	mux.HandleFunc("/api/no/config/terra-flexe:flexe-interfaces/flexe-interface", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "fc-new", "status": "created"})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// FlexE Client PUT（带尾部斜杠）
	mux.HandleFunc("/api/no/config/terra-flexe:flexe-interfaces/flexe-interface/", func(w http.ResponseWriter, r *http.Request) {
		// 匹配 getByGroup/{groupId} 子路径
		if strings.Contains(r.URL.Path, "/getByGroup/") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": "fc-001", "groupId": "fg-001", "clientName": "Client1"},
			})
			return
		}
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// Port Info 下载
	mux.HandleFunc("/api/no/config/terra-flexe:flexe/download/txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("port1,GE1/0/1,100G\nport2,GE2/0/1,10G\n"))
	})

	// ── 信道化子接口 ──

	mux.HandleFunc("/api/no/config/terra-slicing:sub-interfaces-slicing/sub-interface-slicing", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "si-new", "status": "created"})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// 子接口 DELETE（带 id 路径）
	mux.HandleFunc("/api/no/config/terra-slicing:sub-interfaces-slicing/sub-interface-slicing/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/no/config/terra-slicing:sub-interfaces-slicing/sub-interface/getAll", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": "si-001", "deviceName": "NJ-SCT-R01"},
		})
	})

	mux.HandleFunc("/api/no/config/terra-slicing:sub-interfaces-slicing/update/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// ── SRv6 网络切片 ──

	mux.HandleFunc("/api/no/config/terra-slicing:srv6-network-slices/srv6-network-slice", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]any{
				{"sliceId": "slice-001", "device": "NJ-SCT-R01"},
			})
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "slice-new", "status": "created"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// SRv6 切片 DELETE（带 id 路径）
	mux.HandleFunc("/api/no/config/terra-slicing:srv6-network-slices/srv6-network-slice/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/no/config/terra-slicing:srv6-network-slices/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	return httptest.NewServer(mux)
}

// ──────────────────────────────
// FlexE Group 测试
// ──────────────────────────────

func TestListFlexEGroups(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListFlexEGroups(context.Background(), "NJ-SCT-R01", "")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "fg-001", result[0]["id"])
}

func TestCreateFlexEGroup(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.CreateFlexEGroup(context.Background(), map[string]any{
		"deviceName": "NJ-SCT-R01",
		"groupName":  "FG1",
	})
	require.NoError(t, err)
	assert.Equal(t, "fg-new", result["id"])
}

func TestUpdateFlexEGroup(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.UpdateFlexEGroup(context.Background(), map[string]any{
		"id": "fg-001", "groupName": "FG1-updated",
	})
	require.NoError(t, err)
}

func TestDeleteFlexEGroup(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteFlexEGroup(context.Background(), "fg-001")
	require.NoError(t, err)
}

// ──────────────────────────────
// FlexE Client 测试
// ──────────────────────────────

func TestListFlexEClients(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListFlexEClients(context.Background(), "fg-001")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "fc-001", result[0]["id"])
}

func TestCreateFlexEClient(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.CreateFlexEClient(context.Background(), map[string]any{
		"groupId": "fg-001", "clientName": "Client1",
	})
	require.NoError(t, err)
	assert.Equal(t, "fc-new", result["id"])
}

func TestUpdateFlexEClient(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.UpdateFlexEClient(context.Background(), map[string]any{
		"id": "fc-001", "clientName": "Client1-updated",
	})
	require.NoError(t, err)
}

func TestDeleteFlexEClient(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteFlexEClient(context.Background(), "fc-001")
	require.NoError(t, err)
}

func TestDownloadPortInfo(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.DownloadPortInfo(context.Background())
	require.NoError(t, err)
	assert.Contains(t, result, "port1")
	assert.Contains(t, result, "GE1/0/1")
}

// ──────────────────────────────
// 信道化子接口测试
// ──────────────────────────────

func TestListSubInterfaceSlicings(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListSubInterfaceSlicings(context.Background(), "NJ-SCT-R01", "")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "si-001", result[0]["id"])
}

func TestCreateSubInterfaceSlicing(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.CreateSubInterfaceSlicing(context.Background(), map[string]any{
		"deviceName": "NJ-SCT-R01",
	})
	require.NoError(t, err)
	assert.Equal(t, "si-new", result["id"])
}

func TestUpdateSubInterfaceSlicing(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.UpdateSubInterfaceSlicing(context.Background(), map[string]any{
		"id": "si-001", "bandwidth": 1000,
	})
	require.NoError(t, err)
}

func TestDeleteSubInterfaceSlicing(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteSubInterfaceSlicing(context.Background(), "si-001")
	require.NoError(t, err)
}

// ──────────────────────────────
// SRv6 网络切片测试
// ──────────────────────────────

func TestListSRv6Slices(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListSRv6Slices(context.Background(), "", "NJ-SCT-R01")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "slice-001", result[0]["sliceId"])
}

func TestCreateSRv6Slice(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.CreateSRv6Slice(context.Background(), map[string]any{
		"sliceId": "slice-new", "device": "NJ-SCT-R01",
	})
	require.NoError(t, err)
	assert.Equal(t, "slice-new", result["id"])
}

func TestUpdateSRv6Slice(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.UpdateSRv6Slice(context.Background(), map[string]any{
		"sliceId": "slice-001", "bandwidth": 5000,
	})
	require.NoError(t, err)
}

func TestDeleteSRv6Slice(t *testing.T) {
	server := setupSliceMockServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteSRv6Slice(context.Background(), "slice-001")
	require.NoError(t, err)
}

// ──────────────────────────────
// 错误路径测试
// ──────────────────────────────

func TestListFlexEGroups_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-flexe:flexe/flexe-group", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListFlexEGroups(context.Background(), "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestDeleteSRv6Slice_Error(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-slicing:srv6-network-slices/srv6-network-slice/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteSRv6Slice(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}
