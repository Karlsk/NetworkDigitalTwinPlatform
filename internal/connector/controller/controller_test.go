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
			"page_num":        1,
			"page_size":       100,
			"total_elements":  1,
			"total_pages":     1,
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

	return httptest.NewServer(mux)
}

// newTestConnector 创建指向 mock server 的测试用 ControllerConnector。
func newTestConnector(t *testing.T, serverURL string) *ControllerConnector {
	client := connector.NewHTTPClient(
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
	return NewControllerConnector("test-controller", client,
		[]string{"Device", "Interface", "Link", "Alarm", "VPN", "Tunnel", "ISIS", "BGP"}, cfg)
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

func TestEncryptPassword(t *testing.T) {
	// 使用真实 Controller 的密钥和密码进行验证
	secretKey := "9mng65v8jf4lxn93nabf981m"
	password := "tgb.258"
	expected := "gXpi7pWvZNA="

	result, err := encryptPassword(secretKey, password)
	require.NoError(t, err)
	assert.Equal(t, expected, result, "3DES-CBC-SHA256 IV encryption mismatch")
}

func TestEncryptPasswordInvalidKeyLength(t *testing.T) {
	_, err := encryptPassword("short", "password")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "24 bytes")
}
