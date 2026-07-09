package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/api/middleware"
	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestServer 创建不带 Service 依赖的测试用 Server。
func newTestServer() *Server {
	engine := gin.New()
	engine.Use(gin.Recovery())
	v1 := engine.Group("/api/v1")
	return &Server{engine: engine, router: v1}
}

// registerTestRoutes 注册最小路由集合用于测试。
func registerTestRoutes(s *Server) {
	s.engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().Format(time.RFC3339),
			"version":   "v2.0.0",
		})
	})

	s.engine.NoRoute(func(c *gin.Context) {
		response.Fail(c, http.StatusNotFound, response.CodeNotFound, "not found")
	})

	// V1 stub 路由
	s.router.POST("/sync", func(c *gin.Context) {
		response.NotImplemented(c, "sync endpoint not yet implemented")
	})
	s.router.GET("/topology", func(c *gin.Context) {
		response.NotImplemented(c, "topology endpoint not yet implemented")
	})
}

func TestNewServer(t *testing.T) {
	srv := NewServer()
	require.NotNil(t, srv)
	require.NotNil(t, srv.Engine())
	require.NotNil(t, srv.V1())
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer()
	registerTestRoutes(srv)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "v2.0.0", body["version"])
	assert.NotEmpty(t, body["timestamp"])
}

func TestV1RouteRegistration(t *testing.T) {
	srv := newTestServer()
	registerTestRoutes(srv)

	// POST /api/v1/sync 应该返回 501（stub）
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
	w := httptest.NewRecorder()
	srv.engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotImplemented, w.Code)

	// GET /api/v1/topology 应该返回 501（stub）
	req = httptest.NewRequest(http.MethodGet, "/api/v1/topology", nil)
	w = httptest.NewRecorder()
	srv.engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

func TestNotFound(t *testing.T) {
	srv := newTestServer()
	registerTestRoutes(srv)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, response.CodeNotFound, body.Code)
	assert.Equal(t, "not found", body.Message)
}

func TestStandardizedErrorResponse(t *testing.T) {
	tests := []struct {
		name     string
		httpCode int
		errCode  response.ErrorCode
		msg      string
	}{
		{"bad request", http.StatusBadRequest, response.CodeBadRequest, "invalid parameter"},
		{"unauthorized", http.StatusUnauthorized, response.CodeUnauthorized, "authentication required"},
		{"not found", http.StatusNotFound, response.CodeNotFound, "resource not found"},
		{"internal error", http.StatusInternalServerError, response.CodeInternalError, "internal server error"},
		{"not implemented", http.StatusNotImplemented, response.CodeNotImplemented, "feature not implemented"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := gin.New()
			engine.GET("/test", func(c *gin.Context) {
				response.Fail(c, tt.httpCode, tt.errCode, tt.msg)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			assert.Equal(t, tt.httpCode, w.Code)

			var body response.Response
			err := json.Unmarshal(w.Body.Bytes(), &body)
			require.NoError(t, err)

			assert.Equal(t, tt.errCode, body.Code)
			assert.Equal(t, tt.msg, body.Message)
			assert.Nil(t, body.Data)
		})
	}
}

func TestOKResponse(t *testing.T) {
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		response.OK(c, gin.H{"key": "value"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, response.CodeSuccess, body.Code)
	assert.Equal(t, "success", body.Message)
	assert.NotNil(t, body.Data)
}

func TestPageOKResponse(t *testing.T) {
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		response.PageOK(c, []string{"a", "b"}, 100)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body response.PageResponse
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, response.CodeSuccess, body.Code)
	assert.Equal(t, 100, body.Total)
}

func TestRateLimitMiddleware(t *testing.T) {
	engine := gin.New()
	// 极低限流：1 req/s, burst=1，便于测试
	engine.Use(middleware.RateLimit(1, 1))
	engine.GET("/test", func(c *gin.Context) {
		response.OK(c, nil)
	})

	// 第一个请求应该成功
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 快速发送第二个请求应该被限流
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, float64(429001), body["code"])
	assert.Equal(t, "rate limit exceeded", body["message"])
}

func TestRegisterRoutes(t *testing.T) {
	// 使用不含熔断器的 server，避免多个 501 stub 触发熔断
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(middleware.RateLimit(100, 200))
	srv := &Server{engine: engine, router: engine.Group("/api/v1")}

	srv.RegisterRoutes(&HandlerDeps{
		SyncSvc:     &service.SyncService{},
		SnapshotSvc: &service.SnapshotService{},
		AnalysisSvc: &service.AnalysisService{},
		DeviceSvc:   &service.DeviceService{},
	})

	// 验证 /health
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 验证仍为 stub 的路由返回 501
	stubRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/topology"},
		{http.MethodGet, "/api/v1/device/netbox/devices"},
		{http.MethodGet, "/api/v1/monitor/controller/telemetry"},
	}

	for _, r := range stubRoutes {
		t.Run(fmt.Sprintf("%s %s", r.method, r.path), func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.path, nil)
			w := httptest.NewRecorder()
			srv.engine.ServeHTTP(w, req)
			assert.Equal(t, http.StatusNotImplemented, w.Code, "%s %s should return 501", r.method, r.path)
		})
	}

	// 验证 404 处理
	req = httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w = httptest.NewRecorder()
	srv.engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	var body response.Response
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, response.CodeNotFound, body.Code)
}

func TestRun_GracefulShutdown(t *testing.T) {
	srv := NewServer()
	srv.engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 获取一个可用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx, addr)
	}()

	// 等待服务启动
	time.Sleep(100 * time.Millisecond)

	// 发送请求验证服务正常
	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// 触发优雅关闭
	cancel()

	// 等待关闭完成
	select {
	case runErr := <-errCh:
		assert.NoError(t, runErr, "graceful shutdown 应返回 nil error")
	case <-time.After(5 * time.Second):
		t.Fatal("graceful shutdown 超时")
	}
}

func TestCircuitBreaker(t *testing.T) {
	engine := gin.New()
	// 阈值=2，超时=100ms
	engine.Use(middleware.CircuitBreaker(2, 100*time.Millisecond))

	callCount := 0
	engine.GET("/test", func(c *gin.Context) {
		callCount++
		c.JSON(http.StatusInternalServerError, gin.H{"error": "simulated failure"})
	})

	// 发送 2 个请求触发熔断
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	}

	// 第 3 个请求应该被熔断器拦截，返回 503
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, float64(503001), body["code"])
	assert.Contains(t, body["message"], "circuit breaker")

	// 等待超时后，熔断器应进入半开状态
	time.Sleep(150 * time.Millisecond)

	// 半开状态的探测请求（handler 需要返回成功才能关闭熔断器）
	engine2 := gin.New()
	engine2.Use(middleware.CircuitBreaker(2, 100*time.Millisecond))

	failHandler := func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{})
	}
	okHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "recovered"})
	}
	engine2.GET("/fail", failHandler)
	engine2.GET("/ok", okHandler)

	// 先触发熔断
	for i := 0; i < 2; i++ {
		req = httptest.NewRequest(http.MethodGet, "/fail", nil)
		w = httptest.NewRecorder()
		engine2.ServeHTTP(w, req)
	}

	// 等待超时
	time.Sleep(150 * time.Millisecond)

	// 半开状态探测请求
	req = httptest.NewRequest(http.MethodGet, "/ok", nil)
	w = httptest.NewRecorder()
	engine2.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
