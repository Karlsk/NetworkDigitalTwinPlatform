package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// BenchmarkHealthEndpoint 测量 GET /health 端点的响应时间。
// 目标: < 50ms
func BenchmarkHealthEndpoint(b *testing.B) {
	engine := gin.New()
	engine.GET("/health", Health)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d", w.Code)
		}
	}
}

// BenchmarkHealthEndpoint_Parallel 并行测量 /health 端点吞吐量。
func BenchmarkHealthEndpoint_Parallel(b *testing.B) {
	engine := gin.New()
	engine.GET("/health", Health)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				b.Fatalf("expected status 200, got %d", w.Code)
			}
		}
	})
}
