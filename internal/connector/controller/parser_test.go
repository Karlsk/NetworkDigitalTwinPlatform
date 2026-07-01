package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ──────────────────────────────
// BGP 文本解析测试
// ──────────────────────────────

func TestParseBGPTextH3C(t *testing.T) {
	input := `display bgp peer ipv4
 BGP local router ID: 172.16.11.2
 Local AS number: 137749
 Total number of peers: 3       Peers in established state: 1
 * - Dynamically created peer
 Peer                    AS  MsgRcvd  MsgSent OutQ  PrefRcv Up/Down  State
 1.1.1.3             137749        0        0    0        0 5523h36m Connect
 172.16.11.4         137749   115005    18053    0      514 0230h43m Established
 2001:DB8:4:100::      65000        0        0    0        0 5523h39m Idle
`
	peers, err := ParseBGPText("H3C", input)
	require.NoError(t, err)
	require.Len(t, peers, 3)

	// 验证第一个 peer
	assert.Equal(t, "1.1.1.3", peers[0]["peer_ip"])
	assert.Equal(t, 137749, peers[0]["peer_as"])
	assert.Equal(t, "Connect", peers[0]["state"])
	assert.Equal(t, "5523h36m", peers[0]["uptime"])
	assert.Equal(t, "172.16.11.2", peers[0]["router_id"])
	assert.Equal(t, 137749, peers[0]["local_as"])

	// 验证第二个 peer（Established）
	assert.Equal(t, "172.16.11.4", peers[1]["peer_ip"])
	assert.Equal(t, "Established", peers[1]["state"])

	// 验证第三个 peer（IPv6）
	assert.Equal(t, "2001:DB8:4:100::", peers[2]["peer_ip"])
	assert.Equal(t, 65000, peers[2]["peer_as"])
	assert.Equal(t, "Idle", peers[2]["state"])
}

func TestParseBGPTextZTE(t *testing.T) {
	input := ` BGP local router ID: 10.0.0.1
 Local AS number: 65000
 Peer                    AS  MsgRcvd  MsgSent OutQ  PrefRcv Up/Down  State
 10.0.0.2              65000      100       90    0       10 100h20m Established
`
	peers, err := ParseBGPText("ZTE", input)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	assert.Equal(t, "10.0.0.2", peers[0]["peer_ip"])
	assert.Equal(t, "Established", peers[0]["state"])
}

func TestParseBGPTextEmpty(t *testing.T) {
	peers, err := ParseBGPText("H3C", "")
	require.NoError(t, err)
	assert.Nil(t, peers)
}

func TestParseBGPTextNoData(t *testing.T) {
	input := `display bgp peer ipv4
 BGP local router ID: 172.16.11.2
 Local AS number: 137749
 Total number of peers: 0       Peers in established state: 0
`
	peers, err := ParseBGPText("H3C", input)
	require.NoError(t, err)
	assert.Nil(t, peers)
}

func TestParseBGPTextGeneric(t *testing.T) {
	// 未知厂商使用通用解析
	input := ` BGP local router ID: 192.168.1.1
 Local AS number: 64512
 Peer                    AS  MsgRcvd  MsgSent OutQ  PrefRcv Up/Down  State
 192.168.1.2           64512       50       45    0        5 10h30m Established
`
	peers, err := ParseBGPText("UnknownVendor", input)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	assert.Equal(t, "192.168.1.2", peers[0]["peer_ip"])
}

// ──────────────────────────────
// ISIS 文本解析测试
// ──────────────────────────────

func TestParseISISTextH3C(t *testing.T) {
	// 真实 Controller ISIS 回显格式
	input := `display isis peer verbose 10
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
`
	peers, err := ParseISISText("H3C", input)
	require.NoError(t, err)
	require.Len(t, peers, 2)

	// 验证第一个邻居
	assert.Equal(t, "NJ-SCT-R02", peers[0]["system_id"])
	assert.Equal(t, "49.0001", peers[0]["area_id"])
	assert.Equal(t, "Active", peers[0]["status"])
	assert.Equal(t, "L2", peers[0]["level"])
	assert.Equal(t, "RAGG3", peers[0]["interface"])
	assert.Equal(t, "151", peers[0]["circuit_id"])
	assert.Equal(t, "10", peers[0]["process_id"])

	// 验证第二个邻居
	assert.Equal(t, "NJ-SCT-R01", peers[1]["system_id"])
	assert.Equal(t, "49.0001", peers[1]["area_id"])
	assert.Equal(t, "L2", peers[1]["level"])
}

func TestParseISISTextZTE(t *testing.T) {
	input := `display isis peer verbose 10
Peer information for IS-IS(10)
------------------------------
System ID: ZTE-PE01
Interface: eth-0/0/0               Circuit Id:  001
State: Up     HoldTime: 25s        Type: L1L2         PRI: --
Area address(es): 49.0001
`
	peers, err := ParseISISText("ZTE", input)
	require.NoError(t, err)
	require.Len(t, peers, 1)
	assert.Equal(t, "ZTE-PE01", peers[0]["system_id"])
	assert.Equal(t, "L1L2", peers[0]["level"])
	assert.Equal(t, "10", peers[0]["process_id"])
}

func TestParseISISTextEmpty(t *testing.T) {
	peers, err := ParseISISText("H3C", "")
	require.NoError(t, err)
	assert.Nil(t, peers)
}

func TestParseISISTextNoNeighbors(t *testing.T) {
	input := `No ISIS neighbors found.`
	peers, err := ParseISISText("H3C", input)
	require.NoError(t, err)
	assert.Nil(t, peers)
}

// ──────────────────────────────
// 辅助函数测试
// ──────────────────────────────

func TestMapISISStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Up", "Active"},
		{"up", "Active"},
		{"Active", "Active"},
		{"Down", "Inactive"},
		{"Inactive", "Inactive"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, mapISISStatus(tt.input))
	}
}

func TestMapISISLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"L1", "L1"},
		{"L2", "L2"},
		{"L1L2", "L1L2"},
		{"L1/L2", "L1L2"},
		{"l1", "L1"},
		{"l2", "L2"},
		{"l1l2", "L1L2"},
		{"unknown", "L1L2"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, mapISISLevel(tt.input))
	}
}

func TestMapStatus(t *testing.T) {
	tests := []struct {
		input      string
		defaultVal string
		expected   string
	}{
		{"UP", "Up", "Up"},
		{"DOWN", "Up", "Down"},
		{"UNKNOWN", "Up", "Down"},
		{"up", "Up", "Up"},
		{"down", "Up", "Down"},
		{"COMPLETED", "Down", "Up"},
		{"PENDING", "Down", "Down"},
		{"FAILED", "Down", "Down"},
		{"OTHER", "Up", "Up"}, // 未知值返回默认值
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, mapStatus(tt.input, tt.defaultVal))
	}
}

func TestKebabToSnake(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"management-ip", "management_ip"},
		{"connect-status", "connect_status"},
		{"pe-alias", "pe_alias"},
		{"no-dash", "no_dash"},
		{"already_snake", "already_snake"},
		{"", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, kebabToSnake(tt.input))
	}
}
