package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// --- CircuitBreaker 单元测试 ---

func TestNewCircuitBreaker(t *testing.T) {
	cb := newCircuitBreaker(5, 30*time.Second)
	assert.Equal(t, StateClosed, cb.state)
	assert.Equal(t, 5, cb.threshold)
	assert.Equal(t, 30*time.Second, cb.timeout)
	assert.Equal(t, 0, cb.failures)
}

func TestCircuitBreaker_Allow_Closed(t *testing.T) {
	cb := newCircuitBreaker(3, time.Second)
	assert.True(t, cb.allow(), "Closed 状态应允许请求通过")
}

func TestCircuitBreaker_Allow_Open_BeforeTimeout(t *testing.T) {
	cb := newCircuitBreaker(1, 500*time.Millisecond)
	cb.state = StateOpen
	cb.lastFailureTime = time.Now()

	assert.False(t, cb.allow(), "Open 状态且未超时应拒绝请求")
}

func TestCircuitBreaker_Allow_Open_AfterTimeout(t *testing.T) {
	cb := newCircuitBreaker(1, 50*time.Millisecond)
	cb.state = StateOpen
	cb.lastFailureTime = time.Now().Add(-100 * time.Millisecond)

	assert.True(t, cb.allow(), "Open 状态超时应进入 HalfOpen 并允许请求")
	assert.Equal(t, StateHalfOpen, cb.state)
	assert.False(t, cb.halfOpenPending, "进入 HalfOpen 时 halfOpenPending 应为 false")
}

func TestCircuitBreaker_Allow_HalfOpen_First(t *testing.T) {
	cb := newCircuitBreaker(1, time.Second)
	cb.state = StateHalfOpen
	cb.halfOpenPending = false

	assert.True(t, cb.allow(), "HalfOpen 状态第一个请求应允许探测")
	assert.True(t, cb.halfOpenPending, "允许后 halfOpenPending 应为 true")
}

func TestCircuitBreaker_Allow_HalfOpen_Second(t *testing.T) {
	cb := newCircuitBreaker(1, time.Second)
	cb.state = StateHalfOpen
	cb.halfOpenPending = true

	assert.False(t, cb.allow(), "HalfOpen 状态第二个请求应被拒绝")
}

func TestCircuitBreaker_RecordSuccess_Closed(t *testing.T) {
	cb := newCircuitBreaker(3, time.Second)
	cb.failures = 2
	cb.recordSuccess()

	assert.Equal(t, StateClosed, cb.state, "Closed 状态下 recordSuccess 不应改变状态")
	assert.Equal(t, 2, cb.failures, "Closed 状态下 recordSuccess 不应重置 failures")
}

func TestCircuitBreaker_RecordSuccess_HalfOpen(t *testing.T) {
	cb := newCircuitBreaker(3, time.Second)
	cb.state = StateHalfOpen
	cb.failures = 5
	cb.halfOpenPending = true

	cb.recordSuccess()

	assert.Equal(t, StateClosed, cb.state, "HalfOpen 成功应转为 Closed")
	assert.Equal(t, 0, cb.failures, "HalfOpen 成功应重置 failures")
	assert.False(t, cb.halfOpenPending)
}

func TestCircuitBreaker_RecordFailure_Closed_BelowThreshold(t *testing.T) {
	cb := newCircuitBreaker(3, time.Second)
	cb.recordFailure()
	cb.recordFailure()

	assert.Equal(t, StateClosed, cb.state, "未达阈值应保持 Closed")
	assert.Equal(t, 2, cb.failures)
}

func TestCircuitBreaker_RecordFailure_Closed_AboveThreshold(t *testing.T) {
	cb := newCircuitBreaker(3, time.Second)
	for i := 0; i < 3; i++ {
		cb.recordFailure()
	}

	assert.Equal(t, StateOpen, cb.state, "达到阈值应转为 Open")
	assert.False(t, time.Since(cb.lastFailureTime) > time.Second)
}

func TestCircuitBreaker_RecordFailure_HalfOpen(t *testing.T) {
	cb := newCircuitBreaker(3, time.Second)
	cb.state = StateHalfOpen
	cb.halfOpenPending = true

	cb.recordFailure()

	assert.Equal(t, StateOpen, cb.state, "HalfOpen 失败应转回 Open")
	assert.False(t, cb.halfOpenPending)
}

// --- CircuitBreaker 中间件集成测试 ---

func TestCircuitBreaker_Middleware_FullCycle(t *testing.T) {
	engine := gin.New()
	engine.Use(CircuitBreaker(3, 100*time.Millisecond))

	engine.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	engine.GET("/fail", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fail"})
	})

	// 1. Closed → 成功请求不影响熔断器
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 2. Closed → 累计 3 次失败触发熔断
	for i := 0; i < 3; i++ {
		req = httptest.NewRequest(http.MethodGet, "/fail", nil)
		w = httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	}

	// 3. Open → 请求被拒绝
	req = httptest.NewRequest(http.MethodGet, "/ok", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, float64(50301), body["code"])
	assert.Contains(t, body["message"], "circuit breaker")

	// 4. 等待超时 → HalfOpen
	time.Sleep(150 * time.Millisecond)

	// 5. HalfOpen → 探测成功 → 恢复 Closed
	req = httptest.NewRequest(http.MethodGet, "/ok", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "HalfOpen 探测成功应恢复")

	// 6. 验证已恢复，请求正常通过
	req = httptest.NewRequest(http.MethodGet, "/ok", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCircuitBreaker_Middleware_HalfOpen_FailBack(t *testing.T) {
	engine := gin.New()
	engine.Use(CircuitBreaker(2, 80*time.Millisecond))

	engine.GET("/fail", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{})
	})

	// 触发熔断
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/fail", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
	}

	// 等待超时进入 HalfOpen
	time.Sleep(120 * time.Millisecond)

	// HalfOpen 探测失败 → 转回 Open
	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// 验证 Open 状态：请求被拒绝
	req = httptest.NewRequest(http.MethodGet, "/fail", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// --- RateLimit 中间件测试 ---

func TestRateLimit_AllowWithinLimit(t *testing.T) {
	engine := gin.New()
	engine.Use(RateLimit(100, 100))
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimit_ExceedBurst(t *testing.T) {
	engine := gin.New()
	engine.Use(RateLimit(1, 2)) // burst=2
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// burst=2，前两个应成功
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 第 3 个应被限流
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, float64(42901), body["code"])
	assert.Equal(t, "rate limit exceeded", body["message"])
}

func TestRateLimit_RecoversAfterWait(t *testing.T) {
	engine := gin.New()
	engine.Use(RateLimit(10, 1)) // 10 req/s, burst=1
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 消耗 burst
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 立即第二个应被限流
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// 等待恢复
	time.Sleep(150 * time.Millisecond)

	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
