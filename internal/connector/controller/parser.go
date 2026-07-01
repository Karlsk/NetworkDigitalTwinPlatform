// Package controller 实现 Controller Connector，对接网络控制器 REST API。
// parser.go 负责解析 ISIS/BGP 设备 CLI 回显文本，支持多厂商格式（H3C/ZTE/Huawei）。
package controller

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ──────────────────────────────
// BGP 文本解析
// ──────────────────────────────

// ParseBGPText 根据厂商解析 BGP 邻居回显文本。
// 支持 H3C、ZTE、Huawei 格式，未知厂商使用通用解析。
func ParseBGPText(vendor string, text string) ([]map[string]any, error) {
	if text == "" {
		return nil, nil
	}
	switch strings.ToUpper(vendor) {
	case "H3C":
		return parseBGPH3C(text)
	case "ZTE":
		return parseBGPZTE(text)
	default:
		return parseBGPGeneric(text)
	}
}

// parseBGPH3C 解析 H3C 厂商的 BGP 回显文本。
// 格式示例:
//
//	BGP local router ID: 172.16.11.2
//	Local AS number: 137749
//	Total number of peers: 8       Peers in established state: 1
//	Peer                    AS  MsgRcvd  MsgSent OutQ  PrefRcv Up/Down  State
//	1.1.1.3             137749        0        0    0        0 5523h36m Connect
//	172.16.11.4         137749   115005    18053    0      514 0230h43m Established
func parseBGPH3C(text string) ([]map[string]any, error) {
	return parseBGPTabular(text)
}

// parseBGPZTE 解析 ZTE 厂商的 BGP 回显文本。
// ZTE 格式与 H3C 类似，使用表格式输出。
func parseBGPZTE(text string) ([]map[string]any, error) {
	return parseBGPTabular(text)
}

// parseBGPGeneric 通用 BGP 回显解析（兜底）。
func parseBGPGeneric(text string) ([]map[string]any, error) {
	return parseBGPTabular(text)
}

// parseBGPTabular 解析表格式 BGP 回显文本（H3C/ZTE 通用）。
func parseBGPTabular(text string) ([]map[string]any, error) {
	lines := strings.Split(text, "\n")

	// 提取 header 信息
	routerID := extractBGPField(lines, `BGP local router ID:\s*(\S+)`)
	localAS := extractBGPField(lines, `Local AS number:\s*(\S+)`)

	localASInt, _ := strconv.Atoi(localAS)

	// 找到表头行 "Peer ... State"
	headerIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "Peer") && strings.Contains(line, "State") {
			headerIdx = i
			break
		}
	}

	if headerIdx == -1 {
		return nil, nil // 无表头，无数据
	}

	var peers []map[string]any
	for i := headerIdx + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// 跳过空行或非数据行
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}

		// 解析数据行: peer_ip, peer_as, msg_rcvd, msg_sent, outq, pref_rcv, uptime, state
		peerIP := fields[0]
		peerAS, _ := strconv.Atoi(fields[1])
		uptime := fields[6]
		state := fields[7]
		if len(fields) > 7 {
			state = fields[len(fields)-1] // State 是最后一列
		}

		peer := map[string]any{
			"peer_ip":   peerIP,
			"peer_as":   peerAS,
			"state":     state,
			"uptime":    uptime,
			"router_id": routerID,
			"local_as":  localASInt,
		}
		peers = append(peers, peer)
	}

	return peers, nil
}

// extractBGPField 从行列表中正则提取指定字段值。
func extractBGPField(lines []string, pattern string) string {
	re := regexp.MustCompile(pattern)
	for _, line := range lines {
		if matches := re.FindStringSubmatch(line); len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

// ──────────────────────────────
// ISIS 文本解析
// ──────────────────────────────

// ParseISISText 根据厂商解析 ISIS 邻居回显文本。
// 支持 H3C、ZTE、Huawei 格式，未知厂商使用通用解析。
func ParseISISText(vendor string, text string) ([]map[string]any, error) {
	if text == "" {
		return nil, nil
	}
	switch strings.ToUpper(vendor) {
	case "H3C":
		return parseISISH3C(text)
	case "ZTE":
		return parseISISZTE(text)
	default:
		return parseISISGeneric(text)
	}
}

// parseISISH3C 解析 H3C 厂商的 ISIS 回显文本。
// 格式示例:
//
//	System ID: 1720.1601.0002
//	Interface: GE1/0/1
//	Circuit ID: 01
//	State: Up
//	HoldTime: 24s
//	Type: L1
//	Priority: --
//	Area ID: 49.0001
func parseISISH3C(text string) ([]map[string]any, error) {
	return parseISISTabular(text)
}

// parseISISZTE 解析 ZTE 厂商的 ISIS 回显文本。
func parseISISZTE(text string) ([]map[string]any, error) {
	return parseISISTabular(text)
}

// parseISISGeneric 通用 ISIS 回显解析（兜底）。
func parseISISGeneric(text string) ([]map[string]any, error) {
	return parseISISTabular(text)
}

// parseISISTabular 解析 ISIS 邻居回显文本（通用）。
// 真实 Controller 格式示例：
//
//	display isis peer verbose 10
//	Peer information for IS-IS(10)
//	------------------------------
//	System ID: NJ-SCT-R02
//	Interface: RAGG3                   Circuit Id:  151
//	State: Up     HoldTime: 25s        Type: L2           PRI: --
//	Area address(es): 49.0001
func parseISISTabular(text string) ([]map[string]any, error) {
	lines := strings.Split(text, "\n")

	// 提取 process_id: 从 "IS-IS(10)" 或 "is-is(10)" 中解析
	processID := ""
	for _, line := range lines {
		if idx := strings.Index(strings.ToUpper(line), "IS-IS("); idx != -1 {
			rest := line[idx+6:]
			if endIdx := strings.Index(rest, ")"); endIdx != -1 {
				processID = rest[:endIdx]
			}
			break
		}
	}

	var peers []map[string]any
	var current map[string]any

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 跳过头部信息行
		if strings.HasPrefix(line, "display ") || strings.HasPrefix(line, "Peer information") || strings.HasPrefix(line, "---") {
			continue
		}

		if line == "" {
			// 空行表示一个邻居块结束
			if current != nil && current["system_id"] != nil {
				if processID != "" {
					current["process_id"] = processID
				}
				peers = append(peers, current)
			}
			current = nil
			continue
		}

		if current == nil {
			current = make(map[string]any)
		}

		// 解析同一行中的多个 key:value 对（以空格分隔）
		parseISISKeyValuePairs(line, current)
	}

	// 处理最后一个块
	if current != nil && current["system_id"] != nil {
		if processID != "" {
			current["process_id"] = processID
		}
		peers = append(peers, current)
	}

	return peers, nil
}

// isisFieldSep 匹配两个或以上连续空格，用于分隔同一行中的多个 key:value 对。
var isisFieldSep = regexp.MustCompile(` {3,}`) // 3+ spaces

// parseISISKeyValuePairs 解析一行中可能包含的多个 key:value 对。
// 例如: "State: Up     HoldTime: 25s        Type: L2           PRI: --"
// 规则：同一行中不同字段之间用至少 2 个空格分隔，而 key 内部的单词间只有 1 个空格（如 "System ID"）。
func parseISISKeyValuePairs(line string, current map[string]any) {
	// 按多个空格分割成独立的 "key: value" 块
	chunks := isisFieldSep.Split(line, -1)
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		colonIdx := strings.Index(chunk, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(chunk[:colonIdx])
		val := strings.TrimSpace(chunk[colonIdx+1:])
		if key != "" {
			setISISField(key, val, current)
		}
	}
}

// setISISField 根据 key 名设置 ISIS 字段。
func setISISField(key, val string, current map[string]any) {
	lowerKey := strings.ToLower(key)
	switch {
	case lowerKey == "system id":
		current["system_id"] = val
	case lowerKey == "area id" || lowerKey == "area address(es)":
		current["area_id"] = val
	case lowerKey == "state":
		current["status"] = mapISISStatus(val)
	case lowerKey == "type" || lowerKey == "level":
		current["level"] = mapISISLevel(val)
	case lowerKey == "interface":
		current["interface"] = val
	case lowerKey == "holdtime":
		current["holdtime"] = val
	case lowerKey == "circuit id":
		current["circuit_id"] = val
	case lowerKey == "pri":
		current["priority"] = val
	}
}

// mapISISStatus 映射 ISIS 状态值。
func mapISISStatus(val string) string {
	switch strings.ToLower(val) {
	case "up", "active":
		return "Active"
	default:
		return "Inactive"
	}
}

// mapISISLevel 映射 ISIS 级别值。
func mapISISLevel(val string) string {
	val = strings.ToUpper(val)
	switch {
	case strings.Contains(val, "L1L2") || strings.Contains(val, "L1/L2"):
		return "L1L2"
	case strings.Contains(val, "L2"):
		return "L2"
	case strings.Contains(val, "L1"):
		return "L1"
	default:
		return "L1L2"
	}
}

// ──────────────────────────────
// 确保 fmt 被引用
// ──────────────────────────────

var _ = fmt.Sprintf
