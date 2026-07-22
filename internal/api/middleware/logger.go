package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger 请求日志中间件，输出 method/path/status/duration/request_id。
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		attrs := []any{
			"method", c.Request.Method,
			"path", path,
			"query", query,
			"status", status,
			"duration_ms", duration.Milliseconds(),
		}

		if rid, exists := c.Get("request_id"); exists {
			attrs = append(attrs, "request_id", rid)
		}

		if status >= 500 {
			slog.Error("HTTP request", attrs...)
		} else if status >= 400 {
			slog.Warn("HTTP request", attrs...)
		} else {
			slog.Info("HTTP request", attrs...)
		}
	}
}
