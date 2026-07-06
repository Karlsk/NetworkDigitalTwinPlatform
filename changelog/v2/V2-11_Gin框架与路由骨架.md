# V2-11: Gin 框架引入 + 路由注册 + 启动集成

**工时**: 1 天
**前置**: V2-10
**风险等级**: 中
**Phase**: Phase 3 — Gin HTTP API

---

## 背景

V1 现状：仅通过 MCP Streamable HTTP Server 暴露工具（`internal/mcp/server.go`），端口 8080。
V2 目标：引入 Gin 框架暴露 REST API，与 MCP Server 并行提供服务。

### 端口方案

MCP Server 和 Gin HTTP API 共用同一端口（8080），通过路由前缀区分：
- `/mcp/*` → MCP Streamable HTTP
- `/api/v1/*` → Gin REST API
- `/health` → 健康检查（Gin）
- `/metrics` → Prometheus 指标（Gin，V2-15）

---

## 实现步骤

### Step 1: 引入 Gin 依赖

```bash
go get github.com/gin-gonic/gin@latest
```

### Step 2: API Server 封装

新建 `internal/api/server.go`：

```go
package api

import (
    "context"
    "errors"
    "log/slog"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
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
    engine.Use(gin.Recovery())

    v1 := engine.Group("/api/v1")
    return &Server{engine: engine, router: v1}
}

// Engine 返回 gin.Engine，供外部注册路由。
func (s *Server) Engine() *gin.Engine { return s.engine }

// V1 返回 /api/v1 路由组。
func (s *Server) V1() *gin.RouterGroup { return s.router }

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
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        return srv.Shutdown(shutdownCtx)
    case err := <-errCh:
        return err
    }
}
```

### Step 3: Handler 注册

新建 `internal/api/handlers/` 目录结构：

```
internal/api/
├── server.go              # Gin Server 封装
└── handlers/
    ├── sync.go            # POST /api/v1/sync
    ├── snapshot.go        # GET/POST/DELETE /api/v1/snapshot
    ├── topology.go        # GET /api/v1/topology
    ├── device.go          # GET /api/v1/device/*
    ├── monitor.go         # GET /api/v1/monitor/*
    └── health.go          # GET /api/v1/health
```

`internal/api/handlers/health.go`：

```go
package handlers

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
)

// HealthResponse 健康检查响应。
type HealthResponse struct {
    Status    string `json:"status"`
    Timestamp string `json:"timestamp"`
    Version   string `json:"version"`
}

// Health 健康检查 handler。
func Health(c *gin.Context) {
    c.JSON(http.StatusOK, HealthResponse{
        Status:    "ok",
        Timestamp: time.Now().Format(time.RFC3339),
        Version:   "v2.0.0",
    })
}
```

### Step 4: 路由注册

`internal/api/server.go` 新增路由注册方法：

```go
// RegisterRoutes 注册全部 API 路由。
func (s *Server) RegisterRoutes(h *HandlerDeps) {
    // 健康检查（不在 /api/v1 下）
    s.engine.GET("/health", handlers.Health)

    // V1 API
    s.router.POST("/sync", h.Sync)
    s.router.GET("/snapshot", h.ListSnapshots)
    s.router.POST("/snapshot", h.CreateSnapshot)
    s.router.DELETE("/snapshot/:name", h.DeleteSnapshot)
    s.router.POST("/snapshot/restore", h.RestoreSnapshot)
    s.router.GET("/topology", h.QueryTopology)
    s.router.GET("/device/:connector/:query_type", h.QueryDeviceInfo)
    s.router.GET("/monitor/:connector/:query_type", h.QueryMonitor)
}

// HandlerDeps 注入 Service 层依赖。
type HandlerDeps struct {
    SyncSvc     *service.SyncService
    SnapshotSvc *service.SnapshotService
    AnalysisSvc *service.AnalysisService
    DeviceSvc   *service.DeviceService
}
```

### Step 5: main.go 集成

修改 `cmd/server/main.go`：

```go
// 创建 Gin Server
apiServer := api.NewServer()
apiServer.RegisterRoutes(&api.HandlerDeps{
    SyncSvc:     syncSvc,
    SnapshotSvc: snapshotSvc,
    AnalysisSvc: analysisSvc,
    DeviceSvc:   deviceSvc,
})

// 统一启动：MCP + Gin 共用端口
// 方案：Gin 作为主服务器，MCP 路由到 /mcp/*
apiServer.Engine().Any("/mcp/*path", gin.WrapH(mcpsdk.NewStreamableHTTPHandler(...)))

// 启动
if err := apiServer.Run(ctx, fmt.Sprintf(":%d", cfg.Server.Port)); err != nil {
    slog.Error("API server error", "error", err)
    os.Exit(1)
}
```

### Step 6: 单元测试

`internal/api/server_test.go`：

| 测试 | 验证点 |
|------|--------|
| `TestHealthEndpoint` | `GET /health` 返回 200 + JSON |
| `TestV1RouteRegistration` | 路由注册无 panic |
| `TestNotFound` | 未注册路由返回 404 |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `go.mod` | 修改 | 新增 gin-gonic/gin |
| `internal/api/server.go` | 新增 | Gin Server 封装 + 路由注册 |
| `internal/api/handlers/health.go` | 新增 | 健康检查 handler |
| `internal/api/server_test.go` | 新增 | Server 单元测试 |
| `cmd/server/main.go` | 修改 | Gin + MCP 统一启动 |

---

## 注意事项

1. **端口共用**: Gin 作为主 HTTP 服务器，MCP 通过 `gin.WrapH` 嵌入到 `/mcp/*` 路由
2. **Gin Release Mode**: 生产环境设置 `GIN_MODE=release`
3. **向后兼容**: MCP 工具的 URL 和协议不变，只是路由前缀变为 `/mcp/*`
4. **错误处理**: `gin.Recovery()` 中间件捕获 panic，返回 500

---

## 验收标准

- [ ] Gin Server 编译通过
- [ ] `GET /health` 返回 200 + JSON
- [ ] `/api/v1/*` 路由注册正确
- [ ] MCP 工具通过 `/mcp/*` 路由仍可访问
- [ ] `go build ./...` 无错误
- [ ] `go test ./internal/api/...` 全部通过
