// Package api 提供 Gin HTTP API 服务器封装和路由注册
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"gitlab.com/pml/network-digital-twin/internal/api/handlers"
	"gitlab.com/pml/network-digital-twin/internal/api/middleware"
	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// Server Gin HTTP API 服务器。
type Server struct {
	engine *gin.Engine
	router *gin.RouterGroup
}

// NewServer 创建 Gin Server。
func NewServer() *Server {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()

	// 全局中间件链：Recovery → CORS → RequestID → Logger → Metrics → RateLimit → CircuitBreaker
	engine.Use(
		gin.Recovery(),                               // panic 恢复
		middleware.CORS(),                            // 跨域
		middleware.RequestID(),                       // 请求 ID
		middleware.Logger(),                          // 请求日志
		middleware.Metrics(),                         // Prometheus 指标
		middleware.RateLimit(100, 200),               // 限流
		middleware.CircuitBreaker(5, 30*time.Second), // 熔断
	)

	// /metrics 端点（Prometheus 抓取）
	engine.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Swagger UI 路由（通过环境变量 SWAGGER_ENABLED=false 关闭）
	if os.Getenv("SWAGGER_ENABLED") != "false" {
		engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
		slog.Info("swagger UI enabled", "path", "/swagger/index.html")
	}

	v1 := engine.Group("/api/v1")
	return &Server{engine: engine, router: v1}
}

// Engine 返回 gin.Engine，供外部注册路由（如 MCP）。
func (s *Server) Engine() *gin.Engine { return s.engine }

// V1 返回 /api/v1 路由组。
func (s *Server) V1() *gin.RouterGroup { return s.router }

// HandlerDeps 注入 Service 层依赖。
type HandlerDeps struct {
	SyncSvc     *service.SyncService
	SnapshotSvc *service.SnapshotService
	AnalysisSvc *service.AnalysisService
	DeviceSvc   *service.DeviceService
}

// RegisterRoutes 注册全部 API 路由。
func (s *Server) RegisterRoutes(deps *HandlerDeps) {
	// 健康检查（不在 /api/v1 下）
	s.engine.GET("/health", handlers.Health)

	// 404 处理
	s.engine.NoRoute(func(c *gin.Context) {
		response.Fail(c, http.StatusNotFound, response.CodeNotFound, "not found")
	})

	// 创建 Handler 实例
	syncH := handlers.NewSyncHandler(deps.SyncSvc)
	snapshotH := handlers.NewSnapshotHandler(deps.SnapshotSvc)
	topologyH := handlers.NewTopologyHandler(deps.AnalysisSvc, deps.DeviceSvc)
	deviceH := handlers.NewDeviceHandler(deps.DeviceSvc)

	// V1 API 路由
	// Sync
	s.router.POST("/sync", syncH.FullSync)
	s.router.POST("/sync/webhook", syncH.Webhook)

	// Snapshot
	s.router.GET("/snapshot", snapshotH.ListSnapshots)
	s.router.POST("/snapshot", snapshotH.CreateSnapshot)
	s.router.DELETE("/snapshot/:name", snapshotH.DeleteSnapshot)
	s.router.POST("/snapshot/restore", snapshotH.RestoreSnapshot)
	s.router.GET("/snapshot/diff", snapshotH.DiffSnapshots)

	// Audit
	s.router.GET("/audit", snapshotH.QueryAudit)

	// Topology
	s.router.GET("/topology", topologyH.QueryTopology)
	s.router.GET("/topology/live", topologyH.QueryTopologyLive)

	// Device / Monitor
	s.router.GET("/device/:connector/:query_type", deviceH.QueryDeviceInfo)
	s.router.GET("/monitor/:connector/:query_type", deviceH.QueryMonitor)
}

// Run 启动 HTTP 服务器。
func (s *Server) Run(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("HTTP API server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		slog.Info("HTTP API server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:contextcheck // 新 context 用于 shutdown，不应继承已取消的父 context
		defer cancel()
		return srv.Shutdown(shutdownCtx) //nolint:contextcheck // shutdownCtx 是新 context，不继承已取消的父 context
	case err := <-errCh:
		return err
	}
}
