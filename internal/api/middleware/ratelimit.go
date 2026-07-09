// Package middleware 提供 Gin HTTP API 中间件
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
)

// RateLimit 返回令牌桶限流中间件。
// rps: 每秒允许的请求数；burst: 突发请求上限。
func RateLimit(rps int, burst int) gin.HandlerFunc {
	limiter := rate.NewLimiter(rate.Limit(rps), burst)

	return func(c *gin.Context) {
		if !limiter.Allow() {
			response.Fail(c, http.StatusTooManyRequests, response.CodeRateLimitExceed, "rate limit exceeded")
			c.Abort()
			return
		}
		c.Next()
	}
}
