package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/observability"
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
	assert.Equal(t, float64(503001), body["code"])
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
	assert.Equal(t, float64(429001), body["code"])
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

// --- CORS 中间件测试 ---

func TestCORS_PreflightRequest(t *testing.T) {
	engine := gin.New()
	engine.Use(CORS())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "X-Request-ID")
	assert.Equal(t, "X-Request-ID", w.Header().Get("Access-Control-Expose-Headers"))
	assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
}

func TestCORS_NormalRequest(t *testing.T) {
	engine := gin.New()
	engine.Use(CORS())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

// --- RequestID 中间件测试 ---

func TestRequestID_Generated(t *testing.T) {
	engine := gin.New()
	engine.Use(RequestID())
	engine.GET("/test", func(c *gin.Context) {
		rid, _ := c.Get("request_id")
		c.JSON(http.StatusOK, gin.H{"rid": rid})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Header 应包含 X-Request-ID
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
	// Context 中的 request_id 应与 Header 一致
	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, w.Header().Get("X-Request-ID"), body["rid"])
	// UUID 格式：包含 4 个 '-' 分隔
	rid := w.Header().Get("X-Request-ID")
	assert.Len(t, strings.Split(rid, "-"), 5)
}

func TestRequestID_Passthrough(t *testing.T) {
	engine := gin.New()
	engine.Use(RequestID())
	engine.GET("/test", func(c *gin.Context) {
		rid, _ := c.Get("request_id")
		c.JSON(http.StatusOK, gin.H{"rid": rid})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "my-custom-id-123")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "my-custom-id-123", w.Header().Get("X-Request-ID"))
	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, "my-custom-id-123", body["rid"])
}

// --- Logger 中间件测试 ---

func TestLogger_Output(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

	engine := gin.New()
	engine.Use(RequestID())
	engine.Use(Logger())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test?q=1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	logOutput := buf.String()
	assert.Contains(t, logOutput, "method=GET")
	assert.Contains(t, logOutput, "path=/test")
	assert.Contains(t, logOutput, "status=200")
	assert.Contains(t, logOutput, "duration_ms=")
	assert.Contains(t, logOutput, "request_id=")
}

func TestLogger_LevelRouting(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		contains string
	}{
		{"200_Info", http.StatusOK, "INFO"},
		{"404_Warn", http.StatusNotFound, "WARN"},
		{"500_Error", http.StatusInternalServerError, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

			engine := gin.New()
			engine.Use(Logger())
			engine.GET("/test", func(c *gin.Context) {
				c.Status(tt.status)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			assert.Equal(t, tt.status, w.Code)
			assert.Contains(t, buf.String(), "level="+tt.contains)
		})
	}
}

// --- 中间件顺序测试 ---

func TestMiddlewareOrder(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(CORS())
	engine.Use(RequestID())
	engine.Use(Logger())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// CORS header 应存在
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	// RequestID header 应存在
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
	// Logger 应输出
	assert.Contains(t, buf.String(), "HTTP request")
}

// --- Metrics 中间件测试 ---

// getCounterVecValue 从 HTTPRequestsTotal CounterVec 获取指定标签的当前值。
func getCounterVecValue(_ string, labels ...string) float64 {
	m := &dto.Metric{}
	if err := observability.HTTPRequestsTotal.WithLabelValues(labels...).Write(m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

func TestMetricsMiddleware_CounterIncrement(t *testing.T) {
	engine := gin.New()
	engine.Use(Metrics())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 记录请求前的值
	before := getCounterVecValue("ndt_http_requests_total", "GET", "/test", "200")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	after := getCounterVecValue("ndt_http_requests_total", "GET", "/test", "200")
	assert.Equal(t, before+1, after, "counter should increment by 1")
}

func TestMetricsMiddleware_DurationObserved(t *testing.T) {
	engine := gin.New()
	engine.Use(Metrics())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 记录请求前的 histogram count
	m := &dto.Metric{}
	observer := observability.HTTPRequestDuration.WithLabelValues("GET", "/test").(prometheus.Histogram)
	observer.Write(m)
	beforeCount := m.GetHistogram().GetSampleCount()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	m2 := &dto.Metric{}
	observer2 := observability.HTTPRequestDuration.WithLabelValues("GET", "/test").(prometheus.Histogram)
	observer2.Write(m2)
	afterCount := m2.GetHistogram().GetSampleCount()

	assert.Equal(t, beforeCount+1, afterCount, "histogram sample count should increment")
}

func TestMetricsMiddleware_UnknownPath(t *testing.T) {
	engine := gin.New()
	engine.Use(Metrics())
	// 不注册任何路由，FullPath() 将返回空字符串

	before := getCounterVecValue("ndt_http_requests_total", "GET", "unknown", "404")

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	after := getCounterVecValue("ndt_http_requests_total", "GET", "unknown", "404")
	assert.Equal(t, before+1, after, "unknown path should use 'unknown' label")
}
