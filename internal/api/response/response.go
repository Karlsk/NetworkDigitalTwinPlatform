// Package response 提供 Gin HTTP API 的统一响应格式和错误码定义
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一 JSON 响应结构。
type Response struct {
	Code    int    `json:"code"`    // 业务错误码，0 = 成功
	Message string `json:"message"` // 人类可读描述
	Data    any    `json:"data,omitempty"`
}

// PageResponse 分页响应结构。
type PageResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Total   int    `json:"total"`
}

// 错误码定义。
const (
	CodeSuccess          = 0     // 成功
	CodeBadRequest       = 40001 // 请求参数错误
	CodeUnauthorized     = 40101 // 未认证
	CodeNotFound         = 40401 // 资源未找到
	CodeNotImplemented   = 50101 // 功能未实现
	CodeRateLimitExceed  = 42901 // 请求限流
	CodeInternalError    = 50001 // 服务内部错误
	CodeCircuitBreakOpen = 50301 // 服务不可用（熔断）
)

// OK 返回 200 成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: "success",
		Data:    data,
	})
}

// Fail 返回错误响应。
func Fail(c *gin.Context, httpCode, errCode int, msg string) {
	c.JSON(httpCode, Response{
		Code:    errCode,
		Message: msg,
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
