package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
)

// CircuitState 熔断器状态。
type CircuitState int

const (
	StateClosed   CircuitState = iota // 正常状态
	StateOpen                         // 熔断状态
	StateHalfOpen                     // 半开状态
)

// circuitBreaker 三态熔断器实现。
type circuitBreaker struct {
	mu              sync.Mutex
	state           CircuitState
	failures        int
	threshold       int
	timeout         time.Duration
	lastFailureTime time.Time
	halfOpenPending bool
}

// newCircuitBreaker 创建熔断器实例。
func newCircuitBreaker(threshold int, timeout time.Duration) *circuitBreaker {
	return &circuitBreaker{
		state:     StateClosed,
		threshold: threshold,
		timeout:   timeout,
	}
}

// allow 检查请求是否允许通过。
func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.state = StateHalfOpen
			cb.halfOpenPending = false
			return true
		}
		return false
	case StateHalfOpen:
		if !cb.halfOpenPending {
			cb.halfOpenPending = true
			return true
		}
		return false
	}
	return false
}

// recordSuccess 记录成功请求。
func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.failures = 0
		cb.halfOpenPending = false
	}
}

// recordFailure 记录失败请求。
func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.state == StateHalfOpen {
		cb.state = StateOpen
		cb.halfOpenPending = false
	} else if cb.failures >= cb.threshold {
		cb.state = StateOpen
	}
}

// CircuitBreaker 返回三态熔断器中间件。
// threshold: 连续失败次数触发熔断；timeout: 熔断后多久进入半开状态。
func CircuitBreaker(threshold int, timeout time.Duration) gin.HandlerFunc {
	cb := newCircuitBreaker(threshold, timeout)

	return func(c *gin.Context) {
		if !cb.allow() {
			response.Fail(c, http.StatusServiceUnavailable, response.CodeCircuitBreakOpen, "service unavailable, circuit breaker open")
			c.Abort()
			return
		}

		c.Next()

		status := c.Writer.Status()
		if status >= 500 && status < 600 {
			cb.recordFailure()
		} else {
			cb.recordSuccess()
		}
	}
}
