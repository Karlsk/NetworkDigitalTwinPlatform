# V2-16: OpenTelemetry 分布式追踪

**工时**: 1.5 天
**前置**: V2-11
**风险等级**: 中
**Phase**: Phase 4 — 可观测性 + CI/CD

---

## 背景

V1 无分布式追踪能力，跨组件调用（HTTP → Service → GraphDB → Neo4j）无法关联。
V2 引入 OpenTelemetry SDK，在关键路径注入 Span，支持导出到 Jaeger/Tempo 等后端。

---

## 实现步骤

### Step 1: 引入依赖

```bash
go get go.opentelemetry.io/otel
go get go.opentelemetry.io/otel/sdk
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp
go get go.opentelemetry.io/otel/instrumentation
go get go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin
```

### Step 2: OTel 初始化

新建 `internal/observability/tracing.go`：

```go
package observability

import (
    "context"
    "fmt"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/propagation"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// TracerName 服务 Tracer 名称。
const TracerName = "network-digital-twin"

// InitTracer 初始化 OTel TracerProvider。
// endpoint: OTLP Collector 地址（如 localhost:4318）
// 如果 endpoint 为空，使用 Noop Tracer（不导出）。
func InitTracer(ctx context.Context, serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
    if endpoint == "" {
        // Noop：不启用追踪
        tp := sdktrace.NewTracerProvider()
        otel.SetTracerProvider(tp)
        return tp, nil
    }

    exporter, err := otlptracehttp.New(ctx,
        otlptracehttp.WithEndpoint(endpoint),
        otlptracehttp.WithInsecure(),
    )
    if err != nil {
        return nil, fmt.Errorf("create OTLP exporter: %w", err)
    }

    res, err := resource.New(ctx,
        resource.WithAttributes(
            semconv.ServiceNameKey.String(serviceName),
            semconv.ServiceVersionKey.String("v2.0.0"),
        ),
    )
    if err != nil {
        return nil, fmt.Errorf("create resource: %w", err)
    }

    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(res),
        sdktrace.WithSampler(sdktrace.AlwaysSample()),
    )

    otel.SetTracerProvider(tp)
    otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{},
        propagation.Baggage{},
    ))

    return tp, nil
}

// Tracer 获取服务 Tracer。
func Tracer() sdktrace.Tracer {
    return otel.GetTracerProvider().Tracer(TracerName)
}
```

### Step 3: Gin OTel 中间件

修改 `internal/api/server.go`，集成 `otelgin` 中间件：

```go
import (
    "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func NewServer() *Server {
    engine := gin.New()
    engine.Use(
        gin.Recovery(),
        middleware.CORS(),
        middleware.RequestID(),
        otelgin.Middleware(TracerName),  // OTel 自动追踪
        middleware.Logger(),
        middleware.Metrics(),
    )
    // ...
}
```

### Step 4: 关键路径手动 Span

#### 4.1 FullSync Span

修改 `internal/service/sync_service.go`：

```go
import (
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/trace"
    "gitlab.com/pml/network-digital-twin/internal/observability"
)

func (s *SyncService) FullSync(ctx context.Context) (*SyncResult, error) {
    ctx, span := observability.Tracer().Start(ctx, "sync.full_sync",
        trace.WithAttributes(attribute.String("sync.type", "full")),
    )
    defer span.End()

    // ... 现有逻辑 ...

    span.SetAttributes(
        attribute.Int("sync.nodes_created", result.NodesCreated),
        attribute.Int("sync.relations_created", result.RelationsCreated),
        attribute.Int64("sync.duration_ms", result.Duration.Milliseconds()),
    )

    return result, nil
}
```

#### 4.2 Snapshot 操作 Span

修改 `internal/snapshot/manager.go`：

```go
func (m *SnapshotManager) Create(ctx context.Context, name string) (SnapshotMeta, error) {
    ctx, span := observability.Tracer().Start(ctx, "snapshot.create",
        trace.WithAttributes(attribute.String("snapshot.name", name)),
    )
    defer span.End()

    // ... 现有逻辑 ...

    span.SetAttributes(
        attribute.Int("snapshot.node_count", meta.NodeCount),
        attribute.Int("snapshot.rel_count", meta.RelCount),
    )
    return meta, nil
}

func (m *SnapshotManager) Restore(ctx context.Context, name string) error {
    _, span := observability.Tracer().Start(ctx, "snapshot.restore",
        trace.WithAttributes(attribute.String("snapshot.name", name)),
    )
    defer span.End()

    // ... 现有逻辑 ...
}
```

#### 4.3 Neo4j 查询 Span

修改 `internal/graph/neo4j_client.go`（核心查询方法）：

```go
func (c *Neo4jClient) Query(ctx context.Context, db, cypher string, params map[string]any) ([]map[string]any, error) {
    _, span := observability.Tracer().Start(ctx, "neo4j.query",
        trace.WithAttributes(
            attribute.String("db.system", "neo4j"),
            attribute.String("db.statement", cypher),
        ),
    )
    defer span.End()

    // ... 现有逻辑 ...
}
```

#### 4.4 Connector 调用 Span

修改 `internal/connector/` 中关键 Collect 方法：

```go
func (c *ControllerConnector) Collect(ctx context.Context, entityType string) ([]Resource, error) {
    ctx, span := observability.Tracer().Start(ctx, "connector.collect",
        trace.WithAttributes(
            attribute.String("connector.name", c.name),
            attribute.String("connector.entity_type", entityType),
        ),
    )
    defer span.End()

    // ... 现有逻辑 ...
}
```

### Step 5: main.go 初始化

修改 `cmd/server/main.go`：

```go
import "gitlab.com/pml/network-digital-twin/internal/observability"

func main() {
    // ... 现有初始化 ...

    // OTel Tracer（可选，endpoint 为空时 Noop）
    tp, err := observability.InitTracer(ctx, "network-digital-twin", os.Getenv("OTEL_EXPORTER_ENDPOINT"))
    if err != nil {
        slog.Error("init tracer", "error", err)
        os.Exit(1)
    }
    defer func() { _ = tp.Shutdown(context.Background()) }()
}
```

### Step 6: 配置扩展

`configs/config.yaml` 新增：

```yaml
observability:
  otel_endpoint: ""  # 空 = 不启用追踪，如 "localhost:4318"
```

`internal/config/config.go` 新增：

```go
type ObservabilityConfig struct {
    OtelEndpoint string `mapstructure:"otel_endpoint"`
}

// Config 新增字段
Observability ObservabilityConfig `mapstructure:"observability"`
```

### Step 7: 单元测试

| 测试 | 验证点 |
|------|--------|
| `TestTracerInit` | TracerProvider 初始化成功 |
| `TestTracerNoop` | endpoint 为空时使用 Noop |
| `TestSpanCreation` | FullSync/Restore 产生正确 Span |
| `TestSpanAttributes` | Span 包含预期 Attributes |
| `TestGinOTelMiddleware` | HTTP 请求自动产生 Span |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `go.mod` | 修改 | 新增 OTel 依赖 |
| `internal/observability/tracing.go` | 新增 | TracerProvider 初始化 |
| `internal/observability/tracing_test.go` | 新增 | OTel 初始化测试 |
| `internal/api/server.go` | 修改 | 集成 otelgin 中间件 |
| `internal/service/sync_service.go` | 修改 | FullSync/IncrementalSync Span |
| `internal/snapshot/manager.go` | 修改 | Create/Restore/Delete Span |
| `internal/graph/neo4j_client.go` | 修改 | Query/BulkCreate Span |
| `internal/config/config.go` | 修改 | 新增 ObservabilityConfig |
| `configs/config.yaml` | 修改 | 新增 observability 段 |
| `cmd/server/main.go` | 修改 | Tracer 初始化 |

---

## Span 层次结构

```
HTTP Request (otelgin 自动)
└── sync.full_sync
    ├── connector.collect (mock-controller)
    │   └── http.request (外部 API)
    ├── connector.collect (netbox-1)
    ├── normalizer.normalize
    ├── assembler.assemble
    └── neo4j.bulk_create
        └── neo4j.query

HTTP Request
└── snapshot.create
    ├── neo4j.query (read nodes/rels)
    └── snapshot.write_file

HTTP Request
└── snapshot.restore
    ├── snapshot.read_file
    └── neo4j.query (write back)
```

---

## 注意事项

1. **Noop 默认**: `otel_endpoint` 为空时使用 Noop Tracer，零性能开销
2. **采样率**: 开发环境 `AlwaysSample`，生产环境可改为 `TraceIDRatioBased(0.1)` 减少数据量
3. **Context 传播**: 所有方法必须传递 `ctx`，确保 Span 父子关系正确
4. **Span 命名**: 使用 `package.operation` 格式（如 `sync.full_sync`、`neo4j.query`）
5. **错误标记**: Span 中使用 `span.RecordError(err)` 和 `span.SetStatus(codes.Error, ...)` 标记错误
6. **与 Prometheus 互补**: Prometheus 提供聚合指标，OTel 提供单次请求的详细追踪

---

## 验收标准

- [ ] OTel TracerProvider 初始化成功
- [ ] `otel_endpoint` 为空时 Noop 模式正常工作
- [ ] HTTP 请求自动产生 Span（otelgin）
- [ ] FullSync 链路追踪完整（HTTP → Sync → Connector → Neo4j）
- [ ] Snapshot 操作产生正确 Span
- [ ] Span Attributes 包含关键业务信息
- [ ] `go test ./internal/observability/...` 全部通过
