package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"gitlab.com/pml/network-digital-twin/internal/observability"
)

func TestTracingMiddleware_Noop(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 初始化 Noop Tracer
	ctx := context.Background()
	tp, err := observability.InitTracer(ctx, "test-tracing-mw", "")
	require.NoError(t, err)
	defer func() { _ = tp.Shutdown(ctx) }()

	router := gin.New()
	router.Use(Tracing())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTracingMiddleware_WithTraceHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	tp, err := observability.InitTracer(ctx, "test-tracing-header", "")
	require.NoError(t, err)
	defer func() { _ = tp.Shutdown(ctx) }()

	router := gin.New()
	router.Use(Tracing())
	router.GET("/trace-test", func(c *gin.Context) {
		// 验证 context 中有 span
		span := otel.GetTracerProvider().Tracer("test")
		_, s := span.Start(c.Request.Context(), "inner-op")
		defer s.End()
		c.JSON(http.StatusOK, gin.H{"traced": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/trace-test", nil)
	// 模拟 W3C TraceContext Header
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTracingMiddleware_ErrorHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	tp, err := observability.InitTracer(ctx, "test-tracing-err", "")
	require.NoError(t, err)
	defer func() { _ = tp.Shutdown(ctx) }()

	router := gin.New()
	router.Use(Tracing())
	router.GET("/error-test", func(c *gin.Context) {
		_ = c.Error(gin.Error{Err: assert.AnError, Type: gin.ErrorTypePrivate})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "test"})
	})

	req := httptest.NewRequest(http.MethodGet, "/error-test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestTracingMiddleware_POST(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	tp, err := observability.InitTracer(ctx, "test-tracing-post", "")
	require.NoError(t, err)
	defer func() { _ = tp.Shutdown(ctx) }()

	router := gin.New()
	router.Use(Tracing())
	router.POST("/api/v1/sync", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"synced": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
