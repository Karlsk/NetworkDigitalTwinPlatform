package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/observability"
)

// Metrics Prometheus 指标采集中间件。
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath() // 使用路由模板（如 /api/v1/snapshot/:name）
		if path == "" {
			path = "unknown"
		}

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		observability.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		observability.HTTPRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}
