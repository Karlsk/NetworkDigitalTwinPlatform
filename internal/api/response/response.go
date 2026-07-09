// Package response 提供 Gin HTTP API 的统一响应格式和错误码定义
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorCode 6 位模块级错误码枚举（HHHMMX 格式，国内大厂风格）。
//
// HHH = HTTP 状态码类别（400/401/404/429/500/501/503）
// MM  = 模块编号（00=通用, 01=Sync, 02=Snapshot, 03=Topology, 04=Device, 05=Monitor）
// X   = 序号（0-9）
type ErrorCode int

// 通用模块错误码 (MM=00)。
const (
	CodeSuccess          ErrorCode = 0      // 成功
	CodeBadRequest       ErrorCode = 400001 // 请求参数错误
	CodeUnauthorized     ErrorCode = 401001 // 未认证
	CodeNotFound         ErrorCode = 404001 // 资源未找到
	CodeRateLimitExceed  ErrorCode = 429001 // 请求限流
	CodeInternalError    ErrorCode = 500001 // 服务内部错误
	CodeNotImplemented   ErrorCode = 501001 // 功能未实现
	CodeCircuitBreakOpen ErrorCode = 503001 // 服务不可用（熔断）
)

// Sync 模块错误码 (MM=01)。
const (
	CodeSyncUnsupportedAction ErrorCode = 400011 // 不支持的同步动作
	CodeSyncQueueFull         ErrorCode = 503011 // Webhook 队列满
)

// Snapshot 模块错误码 (MM=02)。
const (
	CodeSnapshotBadRequest ErrorCode = 400021 // 快照请求参数错误
	CodeSnapshotNotFound   ErrorCode = 404021 // 快照未找到
)

// Topology 模块错误码 (MM=03)。
const (
	CodeTopologyBadRequest  ErrorCode = 400031 // 拓扑请求参数错误
	CodeTopologyQueryFailed ErrorCode = 500031 // 拓扑查询失败
)

// Device 模块错误码 (MM=04)。
const (
	CodeDeviceBadRequest  ErrorCode = 400041 // 设备请求参数错误
	CodeDeviceNotFound    ErrorCode = 404041 // Connector 未找到
	CodeDeviceUnsupported ErrorCode = 501041 // 设备操作不支持
)

// Monitor 模块错误码 (MM=05)。
const (
	CodeMonitorBadRequest  ErrorCode = 400051 // 监控请求参数错误
	CodeMonitorNotFound    ErrorCode = 404051 // 监控 Connector 未找到
	CodeMonitorUnsupported ErrorCode = 501051 // 监控操作不支持
)

// Response 统一 JSON 响应结构。
type Response struct {
	Code    ErrorCode `json:"code"`    // 业务错误码，0 = 成功
	Message string    `json:"message"` // 人类可读描述
	Data    any       `json:"data,omitempty"`
}

// PageResponse 分页响应结构。
type PageResponse struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Data    any       `json:"data,omitempty"`
	Total   int       `json:"total"`
}

// OK 返回 200 成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: "success",
		Data:    data,
	})
}

// Fail 返回错误响应。
func Fail(c *gin.Context, httpCode int, errCode ErrorCode, msg string) {
	c.JSON(httpCode, Response{
		Code:    errCode,
		Message: msg,
	})
}

// Created 返回 201 创建成功响应。
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Response{
		Code:    CodeSuccess,
		Message: "created",
		Data:    data,
	})
}

// Accepted 返回 202 异步接受响应。
func Accepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, Response{
		Code:    CodeSuccess,
		Message: "accepted",
		Data:    data,
	})
}

// PageOK 返回分页成功响应。
func PageOK(c *gin.Context, list any, total int) {
	c.JSON(http.StatusOK, PageResponse{
		Code:    CodeSuccess,
		Message: "success",
		Data:    list,
		Total:   total,
	})
}

// NotImplemented 返回 501 功能未实现响应。
func NotImplemented(c *gin.Context, msg string) {
	Fail(c, http.StatusNotImplemented, CodeNotImplemented, msg)
}
