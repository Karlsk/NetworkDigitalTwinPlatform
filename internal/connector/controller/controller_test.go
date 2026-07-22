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
// Mock Server Setup
// ──────────────────────────────

// setupMockServer 创建一个模拟 Controller API 的 httptest server。
func setupMockServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()

	// Token 接口
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "mock-token-12345",
			"expires_in":   3600,
		})
	})

	// Device 全量接口
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/peInfos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"peInfo": []map[string]any{
				{
					"id":             "dev-001",
					"name":           "NJ-SCT-R01",
					"pe-alias":       "NJ-PE01",
					"node-type":      "PE",
					"vendor-id":      "H3C",
					"platform-id":    "CR16000",
					"product-name":   "CR16000-F",
					"version":        "7.1.075",
					"management-ip":  "10.0.0.1",
					"connect-status": "UP",
					"pe-as":          137749,
					"peports": map[string]any{
						"peport-info": []any{
							map[string]any{
								"id":               "port-001",
								"name":             "HundredGigE1/0/25",
								"port-type":        "NNI",
								"port-speed":       "100G",
								"status":           "UP",
								"total-bandwidth":  100000,
								"intf-description": "to NJ-SCT-R02",
								"ipv4-addr":        "10.1.1.1/30",
							},
						},
					},
				},
				{
					"id":             "dev-002",
					"name":           "NJ-SCT-R02",
					"pe-alias":       "NJ-PE02",
					"node-type":      "P",
					"vendor-id":      "ZTE",
					"product-name":   "ZXR10 9908",
					"management-ip":  "10.0.0.2",
					"connect-status": "UP",
					"pe-as":          137749,
				},
			},
		})
	})

	// Link 全量接口
	mux.HandleFunc("/api/sr/config/network-topology:network-topology/topology/linksInfo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"link-id":           "NJ-SCT-R01:HundredGigE1/0/25>NJ-SCT-R02:HundredGigE1/0/25",
				"link-status":       "UP",
				"link-type":         "P2P",
				"cfg-bw":            100000,
				"utilization-ratio": 0.15,
				"delay":             2.5,
				"loss":              0.001,
				"source": map[string]any{
					"source-node": "NJ-SCT-R01",
					"source-tp":   "HundredGigE1/0/25",
				},
				"destination": map[string]any{
					"dest-node": "NJ-SCT-R02",
					"dest-tp":   "HundredGigE1/0/25",
				},
			},
		})
	})

	// Alarm 接口
	mux.HandleFunc("/monitor/alert/list", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"code":    0,
			"message": "请求成功",
			"data": []map[string]any{
				{
					"id":       "alarm-001",
					"level":    "MAJOR",
					"category": "ISIS邻居Down",
					"msg":      "ISIS neighbor 10.0.0.2 is down",
					"source":   "NJ-SCT-R01",
					"time":     "2024-01-15 10:30:00",
				},
			},
		})
	})

	// L3VPN 分页接口
	mux.HandleFunc("/api/no/config/ietf-l3vpn-ntw:l3vpn-ntw/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"page_num":       1,
			"page_size":      100,
			"total_elements": 1,
			"total_pages":    1,
			"content": []map[string]any{
				{
					"_id": "6a44b85427cf1100170d5409",
					"vpn-services": map[string]any{
						"vpn-service": []any{
							map[string]any{
								"vpn-id":               "l3_3307",
								"svc-name":             "VPDN-TEST",
								"vpn-svc-type":         "mpls-vpn",
								"vpn-tunnel-type":      "sr-mpls",
								"vpn-service-topology": "any-to-any",
								"site-count":           5,
								"sna-count":            2,
								"pre-create-status":    "up",
								"create-time":          "2024-01-01 00:00:00",
							},
						},
					},
				},
			},
		})
	})

	// L2VPN 分页接口
	mux.HandleFunc("/api/no/config/ietf-l2vpn-svc:l2vpn-svc/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"page_num":       1,
			"page_size":      100,
			"total_elements": 1,
			"total_pages":    1,
			"content": []map[string]any{
				{
					"_id": "6a44b85427cf1100170d5410",
					"vpn-services": map[string]any{
						"vpn-service": []any{
							map[string]any{
								"vpn-id":            "l2_3310",
								"svc-name":          "E-LINE-TEST",
								"vpn-svc-type":      "vpws",
								"vpn-tunnel-type":   "srv6",
								"svc-topo":          "any-to-any",
								"pre-create-status": "up",
							},
						},
					},
				},
			},
		})
	})

	// Tunnel 全量接口
	mux.HandleFunc("/api/sr/config/terra-te-svc:te-policy-instance/all", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"instance-id":          "tunnel-001",
				"policy-template-name": "SRv6-TE-POLICY",
				"cfg-status":           "COMPLETED",
				"te-policy-targets": map[string]any{
					"src-device": "NJ-SCT-R01",
					"dst-device": "NJ-SCT-R02",
					"l3-vpn-id":  "l3_3307",
				},
				"te-tuples": []any{
					map[string]any{
						"color": 100,
						"explicit-tunnel": []any{
							map[string]any{
								"tunnel-id":   "tun-001",
								"tunnel-name": "SRv6-Tunnel-1",
								"src-device":  "NJ-SCT-R01",
								"dst-device":  "NJ-SCT-R02",
								"te-path": []any{
									map[string]any{
										"oper-status": "up",
										"delay":       2.5,
									},
								},
							},
						},
					},
				},
			},
		})
	})

	// ISIS 文本回显接口
	mux.HandleFunc("/restconf/operations/oper-rpc:isis-neighbor", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{
				"isis-neighbor-result": `display isis peer verbose 10
Peer information for IS-IS(10)
------------------------------
System ID: NJ-SCT-R02
Interface: RAGG3                   Circuit Id:  151
State: Up     HoldTime: 25s        Type: L2           PRI: --
Area address(es): 49.0001

System ID: NJ-SCT-R01
Interface: HGE2/1/1.1005           Circuit Id:  001
State: Up     HoldTime: 25s        Type: L2           PRI: --
Area address(es): 49.0001
`,
			},
		})
	})

	// BGP 文本回显接口
	mux.HandleFunc("/restconf/operations/oper-rpc:bgp-peer-config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{
				"current-config-result": `display bgp peer ipv4
 BGP local router ID: 172.16.11.2
 Local AS number: 137749
 Total number of peers: 2       Peers in established state: 1
 Peer                    AS  MsgRcvd  MsgSent OutQ  PrefRcv Up/Down  State
 172.16.11.4         137749   115005    18053    0      514 0230h43m Established
 10.0.0.5            65000        0        0    0        0 5523h36m Connect
`,
			},
		})
	})

	// POP 点列表接口
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/popInfos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"pop-id": "pop-001", "pop-name": "NJ-POP01", "location": "Nanjing"},
		})
	})

	// 厂商型号接口
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/getAllVendorProdModel", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{
			{"vendor": "H3C", "model": "CR16000-F"},
			{"vendor": "ZTE", "model": "ZXR10 9908"},
		})
	})

	// 分页设备接口
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"page_num": 1, "page_size": 10, "total_elements": 1, "total_pages": 1,
			"content": []map[string]any{
				{"id": "dev-paged-001", "name": "PAGED-PE01", "vendor-id": "H3C"},
			},
		})
	})

	// VPN Config Restconf 接口
	mux.HandleFunc("/restconf/operations/oper-rpc:vpn-config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{"current-config-result": "vpn-config-output"},
		})
	})

	// Current Config Restconf 接口
	mux.HandleFunc("/restconf/operations/oper-rpc:current-config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{"current-config-result": "current-config-output"},
		})
	})

	// Global Route Restconf 接口
	mux.HandleFunc("/restconf/operations/oper-rpc:global-route", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{"current-config-result": "global-route-output"},
		})
	})

	return httptest.NewServer(mux)
}

// newTestConnector 创建指向 mock server 的测试用 ControllerConnector。
func newTestConnector(t *testing.T, serverURL string) *ControllerConnector {
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
	client := NewControllerClient("test-controller", httpClient, cfg)
	return NewControllerConnector("test-controller", client,
		[]string{"Device", "Interface", "Link", "Alarm", "VPN", "Tunnel", "ISIS", "BGP"},
		serverURL)
}

// ──────────────────────────────
// 单元测试
// ──────────────────────────────

func TestMetadata(t *testing.T) {
	c := newTestConnector(t, "http://localhost:8080")
	meta := c.Metadata()

	assert.Equal(t, "test-controller", meta.Name)
	assert.Equal(t, "controller", meta.Type)
	assert.Equal(t, "bearer", meta.AuthType)
	assert.Len(t, meta.EntityTypes, 8)
}

func TestPing(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	err := c.Ping(context.Background())
	require.NoError(t, err)
}

func TestCollectDevices(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "Device")
	require.NoError(t, err)
	require.Len(t, resources, 2)

	// 验证第一个设备
	dev1 := resources[0]
	assert.Equal(t, "Device", dev1.Kind)
	assert.Equal(t, "dev-001", dev1.ID)
	assert.Equal(t, "NJ-SCT-R01", dev1.Properties["serial_number"])
	assert.Equal(t, "NJ-SCT-R01", dev1.Properties["hostname"])
	assert.Equal(t, "H3C", dev1.Properties["vendor"])
	assert.Equal(t, "CR16000-F", dev1.Properties["model"])
	assert.Equal(t, "10.0.0.1", dev1.Properties["management_ip"])
	assert.Equal(t, "Up", dev1.Properties["status"])
	assert.Equal(t, "Edge", dev1.Properties["device_type"]) // PE -> Edge
}

func TestCollectInterfaces(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "Interface")
	require.NoError(t, err)
	require.Len(t, resources, 1) // 只有 dev-001 有 peports

	iface := resources[0]
	assert.Equal(t, "Interface", iface.Kind)
	assert.Equal(t, "NJ-SCT-R01", iface.Properties["device_serial"])
	assert.Equal(t, "HundredGigE1/0/25", iface.Properties["if_name"])
	assert.Equal(t, "Up", iface.Properties["status"])
	assert.Equal(t, float64(100000), iface.Properties["bandwidth"])
}

func TestCollectLinks(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "Link")
	require.NoError(t, err)
	require.Len(t, resources, 1)

	link := resources[0]
	assert.Equal(t, "Link", link.Kind)
	assert.Contains(t, link.Properties["link_id"], "NJ-SCT-R01")
	assert.Equal(t, "Up", link.Properties["status"])
	assert.Equal(t, "NJ-SCT-R01", link.Properties["source_node"])
	assert.Equal(t, "NJ-SCT-R02", link.Properties["dest_node"])
}

func TestCollectAlarms(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "Alarm")
	require.NoError(t, err)
	require.Len(t, resources, 1)

	alarm := resources[0]
	assert.Equal(t, "Alarm", alarm.Kind)
	assert.Equal(t, "alarm-001", alarm.Properties["alarm_id"])
	assert.Equal(t, "Major", alarm.Properties["severity"]) // MAJOR -> Major
	assert.Equal(t, "NJ-SCT-R01", alarm.Properties["source"])
}

func TestCollectVPNs(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "VPN")
	require.NoError(t, err)
	require.Len(t, resources, 2) // L3VPN + L2VPN

	// 验证 L3VPN
	l3vpn := resources[0]
	assert.Equal(t, "VPN", l3vpn.Kind)
	assert.Equal(t, "l3_3307", l3vpn.Properties["vpn_id"])
	assert.Equal(t, "VPDN-TEST", l3vpn.Properties["name"])
	assert.Equal(t, "mpls-vpn", l3vpn.Properties["svc_type"])
	assert.Equal(t, "any-to-any", l3vpn.Properties["topology"])

	// 验证 L2VPN
	l2vpn := resources[1]
	assert.Equal(t, "l2_3310", l2vpn.Properties["vpn_id"])
	assert.Equal(t, "vpws", l2vpn.Properties["svc_type"])
}

func TestCollectTunnels(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "Tunnel")
	require.NoError(t, err)
	require.Len(t, resources, 1)

	tunnel := resources[0]
	assert.Equal(t, "Tunnel", tunnel.Kind)
	assert.Equal(t, "tunnel-001", tunnel.Properties["tunnel_id"])
	assert.Equal(t, "SRv6-TE-POLICY", tunnel.Properties["name"])
	assert.Equal(t, "Up", tunnel.Properties["status"]) // COMPLETED -> Up
	assert.Equal(t, "NJ-SCT-R01", tunnel.Properties["src_device"])
	assert.Equal(t, "l3_3307", tunnel.Properties["vpn_id"])
	assert.Equal(t, 1, tunnel.Properties["path_count"])
	assert.Equal(t, "SRv6-Tunnel-1", tunnel.Properties["tunnel_name"])
}

func TestCollectISIS(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "ISIS")
	require.NoError(t, err)

	// 2 台设备 * 2 个 ISIS 邻居 = 4 条记录
	assert.Len(t, resources, 4)

	for _, r := range resources {
		assert.Equal(t, "ISIS", r.Kind)
		assert.NotEmpty(t, r.Properties["isis_id"])
		assert.NotEmpty(t, r.Properties["system_id"])
		assert.Contains(t, []string{"Active", "Inactive"}, r.Properties["status"])
		assert.Contains(t, []string{"L1", "L2", "L1L2"}, r.Properties["level"])
	}
}

func TestCollectBGP(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "BGP")
	require.NoError(t, err)

	// 2 台设备 * 2 个 BGP peer = 4 条记录
	assert.Len(t, resources, 4)

	for _, r := range resources {
		assert.Equal(t, "BGP", r.Kind)
		assert.NotEmpty(t, r.Properties["bgp_id"])
		assert.NotEmpty(t, r.Properties["peer_ip"])
		assert.Contains(t, []string{"Established", "Connect", "Idle", "Active"}, r.Properties["state"])
	}
}

func TestCollectUnsupportedEntityType(t *testing.T) {
	server := setupMockServer(t)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	_, err := c.Collect(context.Background(), "Unknown")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported entity type")
}

func TestStreamNotImplemented(t *testing.T) {
	c := newTestConnector(t, "http://localhost:8080")
	_, err := c.Stream(context.Background(), "Device")
	assert.ErrorIs(t, err, connector.ErrNotImplemented)
}

// TestEncryptPassword 已迁移到 client_test.go

// ──────────────────────────────
// 错误路径测试 (TC-C15/C16/C17)
// ──────────────────────────────

// TestCollect_AuthError TC-C15: Token 接口返回 401，Collect 应返回含 401 的错误。
func TestCollect_AuthError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"error": "invalid credentials"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	httpClient := connector.NewHTTPClient(
		connector.WithBaseURL(server.URL),
		connector.WithAuth(connector.AuthConfig{Type: "bearer", Token: "bad-token"}),
	)
	cfg := map[string]any{
		"base_url":  server.URL,
		"token_url": "/oauth/token",
		"username":  "bad-user",
		"password":  "bad-pass",
		"device_id": "bad-device",
	}
	client := NewControllerClient("test-auth-err", httpClient, cfg)
	c := NewControllerConnector("test-auth-err", client, []string{"Device"}, server.URL)

	_, err := c.Collect(context.Background(), "Device")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

// TestCollect_ServerError TC-C16: Device API 返回 500，Collect 应返回含 500 的错误。
func TestCollect_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	// Token 正常
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	// Device API 返回 500
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/peInfos", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	_, err := c.Collect(context.Background(), "Device")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// TestCollect_Timeout TC-C17: 使用短超时 context，应返回超时错误。
func TestCollect_Timeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		// 模拟慢响应
		time.Sleep(2 * time.Second)
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.Collect(ctx, "Device")
	require.Error(t, err)
	// 超时错误可能是 context.DeadlineExceeded 或 connection refused (server 关闭后)
}

// TestPing_Unreachable: server 不可达时 Ping 应返回错误。
func TestPing_Unreachable(t *testing.T) {
	httpClient := connector.NewHTTPClient(
		connector.WithBaseURL("http://127.0.0.1:1"), // 不可达端口
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
	c := NewControllerConnector("test-unreachable", client, []string{"Device"}, "http://127.0.0.1:1")

	err := c.Ping(context.Background())
	require.Error(t, err)
}

// ──────────────────────────────
// 边界条件测试
// ──────────────────────────────

// TestCollect_EmptyDevices: Device API 返回空列表，应返回 0 条 resource，无错误。
func TestCollect_EmptyDevices(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/peInfos", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"peInfo": []map[string]any{}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "Device")
	require.NoError(t, err)
	assert.Len(t, resources, 0)
}

// TestCollect_EmptyAlarms: Alarm data 为 null，应返回 0 条 resource。
func TestCollect_EmptyAlarms(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/monitor/alert/list", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": nil})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "Alarm")
	require.NoError(t, err)
	assert.Len(t, resources, 0)
}

// TestCollect_MissingFields: Device 响应缺少关键字段，不应 panic，缺失字段跳过。
func TestCollect_MissingFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/peInfos", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"peInfo": []map[string]any{
				{"id": "dev-minimal"}, // 缺少 name/vendor/product-name 等
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "Device")
	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Equal(t, "dev-minimal", resources[0].ID)
	// 缺失字段不应存在
	_, hasHostname := resources[0].Properties["hostname"]
	assert.False(t, hasHostname)
}

// TestCollectVPN_MultiPage: VPN 分页多页遍历。
func TestCollectVPN_MultiPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	var reqCount int
	mux.HandleFunc("/api/no/config/ietf-l3vpn-ntw:l3vpn-ntw/page", func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		json.NewEncoder(w).Encode(map[string]any{
			"page_num": reqCount, "page_size": 1, "total_elements": 2, "total_pages": 2,
			"content": []map[string]any{
				{
					"vpn-services": map[string]any{
						"vpn-service": []any{
							map[string]any{"vpn-id": fmt.Sprintf("vpn-%d", reqCount), "svc-name": fmt.Sprintf("VPN-%d", reqCount)},
						},
					},
				},
			},
		})
	})
	mux.HandleFunc("/api/no/config/ietf-l2vpn-svc:l2vpn-svc/page", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"page_num": 1, "page_size": 100, "total_elements": 0, "total_pages": 1,
			"content": []map[string]any{},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "VPN")
	require.NoError(t, err)
	assert.Len(t, resources, 2) // 2 页 * 1 条 = 2 条 L3VPN
	assert.Equal(t, "vpn-1", resources[0].Properties["vpn_id"])
	assert.Equal(t, "vpn-2", resources[1].Properties["vpn_id"])
}

// TestCollectISIS_DeviceFailure: 部分设备 ISIS 失败，其他设备继续采集。
func TestCollectISIS_DeviceFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/peInfos", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"peInfo": []map[string]any{
				{"id": "d1", "name": "DEV-OK", "vendor-id": "H3C"},
				{"id": "d2", "name": "DEV-FAIL", "vendor-id": "H3C"},
			},
		})
	})
	// ISIS 接口：DEV-OK 返回正常，DEV-FAIL 返回 500
	mux.HandleFunc("/restconf/operations/oper-rpc:isis-neighbor", func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		input, _ := reqBody["input"].(map[string]any)
		peName, _ := input["pe-name"].(string)

		if peName == "DEV-FAIL" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{
				"isis-neighbor-result": "System ID: PEER1\nInterface: eth0     Circuit Id:  001\nState: Up     HoldTime: 25s        Type: L2           PRI: --\nArea address(es): 49.0001\n",
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "ISIS")
	require.NoError(t, err)     // 整体不应返回错误，只跳过失败设备
	assert.Len(t, resources, 1) // 只有 DEV-OK 的 1 个邻居
}

// TestCollectBGP_DeviceFailure: 部分设备 BGP 失败，其他设备继续采集。
func TestCollectBGP_DeviceFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	})
	mux.HandleFunc("/api/no/config/terra-pe:peInfos/peInfos", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"peInfo": []map[string]any{
				{"id": "d1", "name": "DEV-OK", "vendor-id": "H3C"},
				{"id": "d2", "name": "DEV-FAIL", "vendor-id": "H3C"},
			},
		})
	})
	mux.HandleFunc("/restconf/operations/oper-rpc:bgp-peer-config", func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		input, _ := reqBody["input"].(map[string]any)
		peName, _ := input["pe-name"].(string)

		if peName == "DEV-FAIL" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{
				"current-config-result": " BGP local router ID: 10.0.0.1\n Local AS number: 65000\n Peer                    AS  MsgRcvd  MsgSent OutQ  PrefRcv Up/Down  State\n 10.0.0.2              65000      100       90    0       10 100h20m Established\n",
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestConnector(t, server.URL)
	resources, err := c.Collect(context.Background(), "BGP")
	require.NoError(t, err)
	assert.Len(t, resources, 1) // 只有 DEV-OK 的 1 个 peer
}

// TestTokenAutoRefresh: Token 过期后自动刷新。
func TestTokenAutoRefresh(t *testing.T) {
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

	c := newTestConnector(t, server.URL)

	// 第一次调用触发 Token 获取
	_, err := c.Collect(context.Background(), "Device")
	require.NoError(t, err)
	assert.Equal(t, 1, tokenCount)

	// 等待 Token 过期（1s + 60s 缓冲内，但 ensureToken 检查 tokenExp.Add(-60s)，1s 的 token 立刻被认为过期）
	time.Sleep(100 * time.Millisecond)

	// 第二次调用应触发刷新
	_, err = c.Collect(context.Background(), "Device")
	require.NoError(t, err)
	assert.Equal(t, 2, tokenCount) // Token 被刷新了
}
