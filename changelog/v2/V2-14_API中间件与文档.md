# V2-14: API 中间件 + Swagger 文档生成

**工时**: 1 天
**前置**: V2-13
**风险等级**: 低
**Phase**: Phase 3 — Gin HTTP API

---

## 背景

V2-11~V2-13 完成了全部 API 端点。本任务添加生产级中间件和 API 文档：
1. CORS 跨域中间件
2. RequestID 请求追踪中间件
3. 请求日志中间件
4. Swagger/OpenAPI 文档自动生成

---

## 实现步骤

### Step 1: CORS 中间件

新建 `internal/api/middleware/cors.go`：

```go
package middleware

import (
    "net/http"

    "github.com/gin-gonic/gin"
)

// CORS 跨域中间件。
// 开发环境允许所有来源，生产环境通过配置限制。
func CORS() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", "*")
        c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Request-ID")
        c.Header("Access-Control-Expose-Headers", "X-Request-ID")
        c.Header("Access-Control-Max-Age", "86400")

        if c.Request.Method == http.MethodOptions {
            c.AbortWithStatus(http.StatusNoContent)
            return
        }

        c.Next()
    }
}
```

### Step 2: RequestID 中间件

新建 `internal/api/middleware/request_id.go`：

```go
package middleware

import (
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

// RequestID 为每个请求生成唯一 ID，写入 Header 和 Context。
func RequestID() gin.HandlerFunc {
    return func(c *gin.Context) {
        rid := c.GetHeader(requestIDHeader)
        if rid == "" {
            rid = uuid.New().String()
        }
        c.Set("request_id", rid)
        c.Header(requestIDHeader, rid)
        c.Next()
    }
}
```

### Step 3: 请求日志中间件

新建 `internal/api/middleware/logger.go`：

```go
package middleware

import (
    "log/slog"
    "time"

    "github.com/gin-gonic/gin"
)

// Logger 请求日志中间件，输出 method/path/status/duration。
func Logger() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path
        query := c.Request.URL.RawQuery

        c.Next()

        duration := time.Since(start)
        status := c.Writer.Status()

        attrs := []any{
            "method", c.Request.Method,
            "path", path,
            "query", query,
            "status", status,
            "duration_ms", duration.Milliseconds(),
        }

        if rid, exists := c.Get("request_id"); exists {
            attrs = append(attrs, "request_id", rid)
        }

        if status >= 500 {
            slog.Error("HTTP request", attrs...)
        } else if status >= 400 {
            slog.Warn("HTTP request", attrs...)
        } else {
            slog.Info("HTTP request", attrs...)
        }
    }
}
```

### Step 4: 注册中间件

修改 `internal/api/server.go`：

```go
import (
    "gitlab.com/pml/network-digital-twin/internal/api/middleware"
)

func NewServer() *Server {
    if os.Getenv("GIN_MODE") == "" {
        gin.SetMode(gin.ReleaseMode)
    }
    engine := gin.New()

    // 全局中间件
    engine.Use(
        gin.Recovery(),         // panic 恢复
        middleware.CORS(),      // 跨域
        middleware.RequestID(), // 请求 ID
        middleware.Logger(),    // 请求日志
    )

    v1 := engine.Group("/api/v1")
    return &Server{engine: engine, router: v1}
}
```

### Step 5: Swagger 文档生成

使用 `swag` 工具自动生成 OpenAPI 文档。

#### 5.1 安装 swag

```bash
go install github.com/swaggo/swag/cmd/swag@latest
```

#### 5.2 引入依赖

```bash
go get github.com/swaggo/gin-swagger
go get github.com/swaggo/files
go get github.com/swaggo/swag
```

#### 5.3 添加 Swagger 注解

在每个 Handler 方法上添加注解（示例）：

```go
// FullSync godoc
// @Summary      触发全量同步
// @Description  从所有 Connector 全量拉取数据并写入 Neo4j
// @Tags         sync
// @Accept       json
// @Produce      json
// @Success      200 {object} SyncResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/v1/sync [post]
func (h *SyncHandler) FullSync(c *gin.Context) { ... }

// ListSnapshots godoc
// @Summary      列出所有快照
// @Tags         snapshot
// @Produce      json
// @Success      200 {array} SnapshotMeta
// @Router       /api/v1/snapshot [get]
func (h *SnapshotHandler) ListSnapshots(c *gin.Context) { ... }
```

#### 5.4 生成文档

```bash
# 在项目根目录执行
swag init -g cmd/server/main.go -o docs/swagger
```

#### 5.5 注册 Swagger UI 路由

```go
import (
    swaggerFiles "github.com/swaggo/files"
    ginSwagger "github.com/swaggo/gin-swagger"
    _ "gitlab.com/pml/network-digital-twin/docs/swagger"
)

// 在 NewServer 中注册
engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
```

### Step 6: 单元测试

| 测试 | 验证点 |
|------|--------|
| `TestCORS` | 预检请求返回正确 Header |
| `TestRequestIDGenerated` | 未传 X-Request-ID 时自动生成 |
| `TestRequestIDPassthrough` | 已有 X-Request-ID 时透传 |
| `TestLoggerOutput` | 日志包含 method/path/status/duration |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `go.mod` | 修改 | 新增 google/uuid、swaggo 依赖 |
| `internal/api/middleware/cors.go` | 新增 | CORS 跨域中间件 |
| `internal/api/middleware/request_id.go` | 新增 | RequestID 中间件 |
| `internal/api/middleware/logger.go` | 新增 | 请求日志中间件 |
| `internal/api/middleware/middleware_test.go` | 新增 | 中间件单元测试 |
| `internal/api/server.go` | 修改 | 注册全局中间件 |
| `docs/swagger/` | 新增（自动生成） | Swagger JSON/YAML |
| `cmd/server/main.go` | 修改 | Swagger 注解入口 |

---

## 注意事项

1. **中间件顺序**: Recovery → CORS → RequestID → Logger，顺序不可调换
2. **Swagger 可选**: Swagger 为辅助开发工具，生产环境可通过 `SWAGGER_ENABLED=false` 关闭
3. **CORS 安全**: 生产环境应将 `Allow-Origin` 改为具体域名，而非 `*`
4. **RequestID**: 使用 UUID v4，与 V2-16 OpenTelemetry TraceID 互补（RequestID 用于日志关联，TraceID 用于分布式追踪）
5. **MCP 路由**: `/mcp/*` 路径不经过 Gin 中间件（MCP 使用自己的 HTTP 处理链）

---

## 验收标准

- [ ] CORS 中间件正确处理预检请求（OPTIONS 返回 204）
- [ ] 每个请求自动携带 X-Request-ID Header
- [ ] 请求日志包含 method/path/status/duration_ms/request_id
- [ ] Swagger UI 可通过 `/swagger/index.html` 访问
- [ ] `swag init` 生成文档无错误
- [ ] `go test ./internal/api/...` 全部通过
