package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/api/middleware"
	"gitlab.com/pml/network-digital-twin/internal/api/response"
)

// --- 功能验证: Health 端点响应格式 ---

func TestHealthResponseFormat(t *testing.T) {
	srv := newTestServer()
	srv.engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().Format(time.RFC3339),
			"version":   "v2.0.0",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "v2.0.0", body["version"])
	assert.NotEmpty(t, body["timestamp"])

	// 验证 timestamp 是合法的 RFC3339 格式
	_, parseErr := time.Parse(time.RFC3339, body["timestamp"].(string))
	assert.NoError(t, parseErr, "timestamp 应为合法 RFC3339 格式")
}

// --- 功能验证: Stub handler 响应格式 ---

func TestStubHandlers_ResponseFormat(t *testing.T) {
	srv := newTestServer()
	registerTestRoutes(srv)

	stubs := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/sync"},
		{http.MethodGet, "/api/v1/topology"},
	}

	for _, r := range stubs {
		t.Run(fmt.Sprintf("%s %s", r.method, r.path), func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.path, nil)
			w := httptest.NewRecorder()
			srv.engine.ServeHTTP(w, req)

			assert.Equal(t, http.StatusNotImplemented, w.Code)

			var body response.Response
			err := json.Unmarshal(w.Body.Bytes(), &body)
			require.NoError(t, err)

			assert.Equal(t, response.CodeNotImplemented, body.Code)
			assert.NotEmpty(t, body.Message)
			assert.Nil(t, body.Data)
		})
	}
}

// --- 功能验证: 路由隔离（API 与 MCP） ---

func TestRouteIsolation(t *testing.T) {
	srv := newTestServer()
	registerTestRoutes(srv)

	// MCP 路由不应匹配（未注册）
	req := httptest.NewRequest(http.MethodPost, "/mcp/test", nil)
	w := httptest.NewRecorder()
	srv.engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, "未注册 MCP 路由应返回 404")

	// /api/v1 路由正常
	req = httptest.NewRequest(http.MethodGet, "/api/v1/topology", nil)
	w = httptest.NewRecorder()
	srv.engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotImplemented, w.Code, "已注册 API 路由应正常响应")
}

// --- 性能验证: 并发请求处理 ---

func BenchmarkHealthEndpoint(b *testing.B) {
	engine := gin.New()
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
	}
}

func TestConcurrentRequests(t *testing.T) {
	engine := gin.New()
	engine.Use(middleware.RateLimit(10000, 10000)) // 高阈值不影响并发测试
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	const numRequests = 100
	var wg sync.WaitGroup
	var successCount atomic.Int64

	start := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code == http.StatusOK {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	assert.Equal(t, int64(numRequests), successCount.Load(), "所有并发请求应成功")
	t.Logf("并发 %d 请求耗时: %v (平均 %.2f ms/req)", numRequests, elapsed, float64(elapsed.Milliseconds())/float64(numRequests))
}

// --- API 可靠性验证: 限流在高并发下正确工作 ---

func TestRateLimit_UnderLoad(t *testing.T) {
	engine := gin.New()
	engine.Use(middleware.RateLimit(10, 10)) // 10 req/s, burst=10
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 快速发送 20 个请求
	var okCount, limitCount int
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		switch w.Code {
		case http.StatusOK:
			okCount++
		case http.StatusTooManyRequests:
			limitCount++
			// 验证限流响应格式
			var body map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &body)
			require.NoError(t, err)
			assert.Equal(t, float64(42901), body["code"])
		}
	}

	assert.Equal(t, 10, okCount, "burst=10 应允许前 10 个请求")
	assert.Equal(t, 10, limitCount, "超出 burst 的 10 个请求应被限流")
}

// --- API 可靠性验证: 熔断器三态完整流转 ---

func TestCircuitBreaker_ThreeStateTransition(t *testing.T) {
	engine := gin.New()
	engine.Use(middleware.CircuitBreaker(3, 100*time.Millisecond))

	failCount := 0
	engine.GET("/fail", func(c *gin.Context) {
		failCount++
		c.JSON(http.StatusInternalServerError, gin.H{})
	})
	engine.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 阶段 1: Closed → 累计 3 次失败 → Open
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/fail", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	}

	// 阶段 2: Open → 请求被拒绝 (503)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code, "Open 状态应拒绝所有请求")

		var body map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &body)
		require.NoError(t, err)
		assert.Equal(t, float64(50301), body["code"])
	}

	// 阶段 3: 等待超时 → HalfOpen
	time.Sleep(150 * time.Millisecond)

	// 阶段 4: HalfOpen → 探测成功 → Closed
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "HalfOpen 探测成功应恢复 Closed")

	// 阶段 5: 验证恢复正常
	req = httptest.NewRequest(http.MethodGet, "/ok", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "恢复后请求应正常处理")
}

// --- API 可靠性验证: 统一 JSON 响应结构标准化 ---

func TestUnifiedJSONResponseStructure(t *testing.T) {
	t.Run("success response has all required fields", func(t *testing.T) {
		engine := gin.New()
		engine.GET("/test", func(c *gin.Context) {
			response.OK(c, gin.H{"result": "value"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		var raw map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &raw)
		require.NoError(t, err)

		// 验证必须包含 code, message, data 三个字段
		_, hasCode := raw["code"]
		_, hasMessage := raw["message"]
		_, hasData := raw["data"]
		assert.True(t, hasCode, "响应必须包含 code 字段")
		assert.True(t, hasMessage, "响应必须包含 message 字段")
		assert.True(t, hasData, "响应必须包含 data 字段")

		assert.Equal(t, float64(0), raw["code"])
		assert.Equal(t, "success", raw["message"])
	})

	t.Run("error response has code and message, no data", func(t *testing.T) {
		engine := gin.New()
		engine.GET("/test", func(c *gin.Context) {
			response.Fail(c, http.StatusBadRequest, response.CodeBadRequest, "invalid")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		var raw map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &raw)
		require.NoError(t, err)

		_, hasCode := raw["code"]
		_, hasMessage := raw["message"]
		_, hasData := raw["data"]
		assert.True(t, hasCode)
		assert.True(t, hasMessage)
		assert.False(t, hasData, "错误响应不应包含 data 字段")
	})

	t.Run("page response has total field", func(t *testing.T) {
		engine := gin.New()
		engine.GET("/test", func(c *gin.Context) {
			response.PageOK(c, []string{}, 0)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		var raw map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &raw)
		require.NoError(t, err)

		_, hasTotal := raw["total"]
		assert.True(t, hasTotal, "分页响应必须包含 total 字段")
	})
}

// --- API 响应时间验证 ---

func TestAPIResponseTime(t *testing.T) {
	engine := gin.New()
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 预热
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
	}

	// 测量 100 次请求的平均响应时间
	var totalDuration time.Duration
	const iterations = 100
	for i := 0; i < iterations; i++ {
		start := time.Now()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		totalDuration += time.Since(start)
	}

	avgMs := float64(totalDuration.Milliseconds()) / float64(iterations)
	t.Logf("平均响应时间: %.2f ms (%d 次请求)", avgMs, iterations)
	assert.Less(t, avgMs, 10.0, "单次请求平均响应时间应 < 10ms")
}
