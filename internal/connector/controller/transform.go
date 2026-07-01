// Package controller 实现 Controller Connector，对接网络控制器 REST API。
// transform.go 负责将 Controller API 的 kebab-case 响应字段展平并转换为 ontology 的 snake_case 格式。
package controller

import (
	"fmt"
	"strings"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ──────────────────────────────
// 通用工具函数
// ──────────────────────────────

// bgpStateMap BGP FSM 数字状态码 → ontology 状态值。
// 用于 ZTE 等厂商回显中直接输出数字状态码的场景。
var bgpStateMap = map[string]string{
	"0": "Idle",
	"1": "Connect",
	"2": "Active",
	"3": "OpenSent",
	"4": "OpenConfirm",
	"5": "Established",
}

// mapBGPState 映射 BGP 状态值，支持字符串和数字两种格式。
func mapBGPState(val string) string {
	// 先尝试数字状态码映射
	if mapped, ok := bgpStateMap[val]; ok {
		return mapped
	}
	// 尝试标准字符串状态（首字母大写）
	if val == "" {
		return "Connect"
	}
	// 标准 FSM 字符串：Established, Connect, Idle, Active, OpenSent, OpenConfirm
	switch strings.ToLower(val) {
	case "established":
		return "Established"
	case "connect":
		return "Connect"
	case "idle":
		return "Idle"
	case "active":
		return "Active"
	case "opensent":
		return "OpenSent"
	case "openconfirm":
		return "OpenConfirm"
	default:
		return "Connect" // 未知数字或非法状态 → 默认 Connect
	}
}

// kebabToSnake 将 kebab-case 键名转换为 snake_case。
func kebabToSnake(s string) string {
	return strings.ReplaceAll(s, "-", "_")
}

// statusMap API 状态值 → ontology 状态值。
var statusMap = map[string]string{
	"UP":        "Up",
	"DOWN":      "Down",
	"UNKNOWN":   "Down",
	"up":        "Up",
	"down":      "Down",
	"COMPLETED": "Up",
	"PENDING":   "Down",
	"FAILED":    "Down",
}

// severityMap 告警严重级别映射。
var severityMap = map[string]string{
	"CRITICAL": "Critical",
	"MAJOR":    "Major",
	"MINOR":    "Minor",
	"WARNING":  "Warning",
}

// deviceTypeMap 设备类型映射（Controller node-type → ontology device_type）。
var deviceTypeMap = map[string]string{
	"PE":   "Edge",
	"P":    "Core",
	"CE":   "Access",
	"ASBR": "Edge", // Autonomous System Border Router → 边界设备
}

// mapStatus 安全地映射状态值，未知值返回默认值。
func mapStatus(val string, defaultVal string) string {
	if mapped, ok := statusMap[val]; ok {
		return mapped
	}
	return defaultVal
}

// ──────────────────────────────
// 各实体 Transform 函数
// ──────────────────────────────

// transformDevice 将 Controller Device API 响应转换为 Resource.Properties。
func transformDevice(raw map[string]any) map[string]any {
	props := make(map[string]any)

	// 稳定键 + 核心字段
	if v, ok := raw["name"].(string); ok {
		props["serial_number"] = v
		props["hostname"] = v
	}
	if v, ok := raw["vendor-id"].(string); ok {
		props["vendor"] = v
	}
	if v, ok := raw["product-name"].(string); ok {
		props["model"] = v
	}
	if v, ok := raw["management-ip"].(string); ok {
		props["management_ip"] = v
	}
	if v, ok := raw["connect-status"].(string); ok {
		props["status"] = mapStatus(v, "Up")
	}
	if v, ok := raw["node-type"].(string); ok {
		if mapped, ok := deviceTypeMap[v]; ok {
			props["device_type"] = mapped
		} else {
			props["device_type"] = v
		}
	}

	// 额外属性
	if v, ok := raw["pe-alias"].(string); ok {
		props["alias"] = v
	}
	if v, ok := raw["platform-id"].(string); ok {
		props["platform"] = v
	}
	if v, ok := raw["pe-as"]; ok {
		props["as_number"] = v
	}
	if v, ok := raw["version"].(string); ok {
		props["version"] = v
	}
	if v, ok := raw["pop-id"].(string); ok {
		props["pop_id"] = v
	}

	return props
}

// transformInterface 将 Controller Interface 嵌套数据转换为 Resource.Properties。
// deviceName 为所属设备名称，port 为 peport-info 中的单个端口对象。
func transformInterface(deviceName string, port map[string]any) map[string]any {
	props := make(map[string]any)

	props["device_serial"] = deviceName

	if v, ok := port["name"].(string); ok {
		props["if_name"] = v
	}
	if v, ok := port["status"].(string); ok {
		props["status"] = mapStatus(v, "Up")
	}
	if v, ok := port["total-bandwidth"]; ok {
		props["bandwidth"] = v
	}
	if v, ok := port["intf-description"].(string); ok {
		props["description"] = v
	}

	// 额外属性
	if v, ok := port["port-type"].(string); ok {
		props["port_type"] = v
	}
	if v, ok := port["port-speed"].(string); ok {
		props["port_speed"] = v
	}
	if v, ok := port["ipv4-addr"].(string); ok {
		props["ipv4_address"] = v
	}

	return props
}

// transformLink 将 Controller Link API 响应转换为 Resource.Properties。
func transformLink(raw map[string]any) map[string]any {
	props := make(map[string]any)

	if v, ok := raw["link-id"].(string); ok {
		props["link_id"] = v
		props["name"] = v
	}
	if v, ok := raw["cfg-bw"]; ok {
		props["bandwidth"] = v
	}
	if v, ok := raw["link-status"].(string); ok {
		props["status"] = mapStatus(v, "Up")
	}
	if v, ok := raw["link-type"].(string); ok {
		props["link_type"] = v
	}
	if v, ok := raw["utilization-ratio"]; ok {
		props["utilization_ratio"] = v
	}
	if v, ok := raw["delay"]; ok {
		props["delay"] = v
	}
	if v, ok := raw["loss"]; ok {
		props["loss"] = v
	}
	if v, ok := raw["jitter"]; ok {
		props["jitter"] = v
	}

	// 关系推导字段：source/destination
	if src, ok := raw["source"].(map[string]any); ok {
		if v, ok := src["source-node"].(string); ok {
			props["source_node"] = v
		}
		if v, ok := src["source-tp"].(string); ok {
			props["source_tp"] = v
		}
	}
	if dst, ok := raw["destination"].(map[string]any); ok {
		if v, ok := dst["dest-node"].(string); ok {
			props["dest_node"] = v
		}
		if v, ok := dst["dest-tp"].(string); ok {
			props["dest_tp"] = v
		}
	}

	return props
}

// transformAlarm 将 Controller Alarm API 响应转换为 Resource.Properties。
func transformAlarm(raw map[string]any) map[string]any {
	props := make(map[string]any)

	if v, ok := raw["id"].(string); ok {
		props["alarm_id"] = v
	}
	if v, ok := raw["level"].(string); ok {
		if mapped, ok := severityMap[v]; ok {
			props["severity"] = mapped
		} else {
			props["severity"] = v
		}
	}
	if v, ok := raw["msg"].(string); ok {
		props["message"] = v
	}
	if v, ok := raw["time"].(string); ok {
		props["timestamp"] = v
	}
	if v, ok := raw["source"].(string); ok {
		props["source"] = v
	}
	if v, ok := raw["category"].(string); ok {
		props["category"] = v
	}
	if v, ok := raw["component"].(string); ok {
		props["component"] = v
	}
	if v, ok := raw["recoveryTime"]; ok {
		props["recovery_time"] = v
	}

	return props
}

// normalizeVPNTopology 规范化 VPN 拓扑值：去除 "ietf-l3vpn-svc:" 前缀。
// Controller 返回 "ietf-l3vpn-svc:any-to-any"，ontology 期望 "any-to-any"。
func normalizeVPNTopology(val string) string {
	if idx := strings.Index(val, ":"); idx != -1 {
		return val[idx+1:]
	}
	return val
}

// normalizeVPNSvcType 规范化 VPN 服务类型：转小写，下划线替换为短横线。
// Controller 返回 "MPLS_VPN"，ontology 期望 "mpls-vpn"。
func normalizeVPNSvcType(val string) string {
	return strings.ReplaceAll(strings.ToLower(val), "_", "-")
}

// transformVPN 将 Controller VPN API 响应转换为 Resource.Properties。
// vpnType 为 "L3" 或 "L2"，用于区分 L3VPN 和 L2VPN 的字段差异。
func transformVPN(raw map[string]any, vpnType string) map[string]any {
	props := make(map[string]any)

	if v, ok := raw["vpn-id"].(string); ok {
		props["vpn_id"] = v
	}
	if v, ok := raw["svc-name"].(string); ok {
		props["name"] = v
	}
	if v, ok := raw["vpn-svc-type"].(string); ok {
		props["svc_type"] = normalizeVPNSvcType(v)
	}
	if v, ok := raw["vpn-tunnel-type"].(string); ok {
		props["tunnel_type"] = v
	}

	// L3VPN 使用 vpn-service-topology，L2VPN 使用 svc-topo
	if vpnType == "L3" {
		if v, ok := raw["vpn-service-topology"].(string); ok {
			props["topology"] = normalizeVPNTopology(v)
		}
	} else {
		if v, ok := raw["svc-topo"].(string); ok {
			props["topology"] = normalizeVPNTopology(v)
		}
	}

	if v, ok := raw["site-count"]; ok {
		props["site_count"] = v
	}
	if v, ok := raw["sna-count"]; ok {
		props["sna_count"] = v
	}
	if v, ok := raw["pre-create-status"].(string); ok {
		props["status"] = mapStatus(v, "Up")
	}
	if v, ok := raw["create-time"].(string); ok {
		props["create_time"] = v
	}
	if v, ok := raw["update-time"].(string); ok {
		props["update_time"] = v
	}

	return props
}

// transformTunnel 将 Controller Tunnel (policy-instance) API 响应转换为 Resource.Properties。
// 深度嵌套结构展平：te-tuples[].explicit-tunnel[].te-path[]
func transformTunnel(raw map[string]any) map[string]any {
	props := make(map[string]any)

	if v, ok := raw["instance-id"].(string); ok {
		props["tunnel_id"] = v
	}
	if v, ok := raw["policy-template-name"].(string); ok {
		props["name"] = v
	}
	if v, ok := raw["cfg-status"].(string); ok {
		props["status"] = mapStatus(v, "Down")
	}

	// te-policy-targets
	if targets, ok := raw["te-policy-targets"].(map[string]any); ok {
		if v, ok := targets["src-device"].(string); ok {
			props["src_device"] = v
		}
		if v, ok := targets["dst-device"].(string); ok {
			props["dst_device"] = v
		}
		if v, ok := targets["l3-vpn-id"].(string); ok {
			props["vpn_id"] = v
		}
	}

	// 统计 te-tuples 中的 tunnel 数量和首个 tunnel-name
	pathCount := 0
	var firstTunnelName string
	if tuples, ok := raw["te-tuples"].([]any); ok {
		for _, t := range tuples {
			tuple, ok := t.(map[string]any)
			if !ok {
				continue
			}
			if tunnels, ok := tuple["explicit-tunnel"].([]any); ok {
				for _, et := range tunnels {
					tunnel, ok := et.(map[string]any)
					if !ok {
						continue
					}
					pathCount++
					if firstTunnelName == "" {
						if name, ok := tunnel["tunnel-name"].(string); ok {
							firstTunnelName = name
						}
					}
				}
			}
		}
	}
	props["path_count"] = pathCount
	if firstTunnelName != "" {
		props["tunnel_name"] = firstTunnelName
	}

	return props
}

// ──────────────────────────────
// ISIS / BGP 解析后的 Transform
// ──────────────────────────────

// transformISISPeer 将解析后的 ISIS 邻居数据补充设备上下文信息。
// peName 为设备名，用于生成 isis_id 稳定键。
func transformISISPeer(peName string, peer map[string]any) map[string]any {
	props := make(map[string]any)

	// 复制解析出的字段
	for k, v := range peer {
		props[k] = v
	}

	// 生成 isis_id 稳定键: deviceName_neighborSystemID
	systemID := ""
	if v, ok := peer["system_id"].(string); ok {
		systemID = v
	}
	props["isis_id"] = fmt.Sprintf("%s_%s", peName, systemID)

	return props
}

// transformBGPPeer 将解析后的 BGP 邻居数据补充设备上下文信息。
// peName 为设备名，用于生成 bgp_id 稳定键。
func transformBGPPeer(peName string, peer map[string]any) map[string]any {
	props := make(map[string]any)

	// 复制解析出的字段
	for k, v := range peer {
		props[k] = v
	}

	// 规范化 BGP state（支持数字状态码和字符串两种格式）
	if v, ok := peer["state"]; ok {
		stateStr := ""
		switch sv := v.(type) {
		case string:
			stateStr = sv
		case int:
			stateStr = fmt.Sprintf("%d", sv)
		case float64:
			stateStr = fmt.Sprintf("%d", int(sv))
		}
		props["state"] = mapBGPState(stateStr)
	}

	// 生成 bgp_id 稳定键
	peerIP := ""
	if v, ok := peer["peer_ip"].(string); ok {
		peerIP = v
	}
	props["bgp_id"] = fmt.Sprintf("%s_%s", peName, peerIP)

	return props
}

// ──────────────────────────────
// 占位：确保 transform 函数被 collect 方法正确调用
// ──────────────────────────────

// 确保 connector.Resource 被引用
var _ = connector.Resource{}
