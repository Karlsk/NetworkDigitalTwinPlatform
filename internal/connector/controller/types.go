// Package controller 实现 Controller Connector 及统一 API 适配层。
// types.go 集中定义所有请求/响应结构体，供 ControllerClient 和 ControllerConnector 共享。
package controller

import "time"

// DeviceInfo 缓存的设备基本信息，供 ISIS/BGP 采集时查询厂商信息。
type DeviceInfo struct {
	Name   string // 设备名称（pe-name）
	Vendor string // 厂商（H3C/ZTE/Huawei）
}

// ──────────────────────────────
// Token 认证结构体
// ──────────────────────────────

// tokenRequest Token 请求体。
type tokenRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	DeviceID string `json:"device_id"`
}

// tokenResponse Token 响应体。
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"` // 秒
}

// ──────────────────────────────
// 拓扑 API 响应结构体
// ──────────────────────────────

// deviceResponse Device 接口响应包装。
type deviceResponse struct {
	PeInfo []map[string]any `json:"peInfo"`
}

// alarmResponse 告警接口响应包装。
type alarmResponse struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    []map[string]any `json:"data"`
}

// vpnPageResponse VPN 自定义分页响应。
type vpnPageResponse struct {
	PageNum       int              `json:"page_num"`
	PageSize      int              `json:"page_size"`
	TotalElements int              `json:"total_elements"`
	TotalPages    int              `json:"total_pages"`
	Content       []map[string]any `json:"content"`
}

// ──────────────────────────────
// Restconf 请求/响应结构体
// ──────────────────────────────

// isisRequest ISIS 接口请求体。
type isisRequest struct {
	Input struct {
		PeName  string `json:"pe-name"`
		Process int    `json:"process"`
		Verbose bool   `json:"verbose"`
		Scope   string `json:"scope"`
	} `json:"input"`
}

// bgpRequest BGP 接口请求体。
type bgpRequest struct {
	Input struct {
		PeName string `json:"pe-name"`
		Scope  string `json:"scope"`
	} `json:"input"`
}

// restconfResponse Restconf 文本回显响应。
type restconfResponse struct {
	Output struct {
		CurrentConfigResult string `json:"current-config-result"`
		ISISNeighborResult  string `json:"isis-neighbor-result"`
	} `json:"output"`
}

// ──────────────────────────────
// 通用公共结构体（供后续 Phase 使用）
// ──────────────────────────────

// PagedResult 通用分页响应。
type PagedResult struct {
	Content       []map[string]any `json:"content"`
	PageNum       int              `json:"page_num"`
	PageSize      int              `json:"page_size"`
	TotalElements int              `json:"total_elements"`
	TotalPages    int              `json:"total_pages"`
}

// MonitorTimeRange 监控查询时间范围。
type MonitorTimeRange struct {
	Start time.Time
	End   time.Time
}

// RestconfRequest Restconf RPC 通用请求体。
type RestconfRequest struct {
	Input map[string]any `json:"input"`
}

// RestconfResponse Restconf RPC 通用响应体。
type RestconfResponse struct {
	Output map[string]any `json:"output"`
}
