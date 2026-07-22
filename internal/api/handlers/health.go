// Package handlers 提供 Gin HTTP API 的请求处理器
package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthResponse 健康检查响应。
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

// Health 健康检查 handler。
//
// @Summary 健康检查
// @Description 返回服务健康状态、时间戳和版本号
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /health [get]
//
// GET /health
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().Format(time.RFC3339),
		Version:   "v2.0.0",
	})
}
