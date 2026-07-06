# V2-15: Prometheus 指标集成

**工时**: 1 天
**前置**: V2-11
**风险等级**: 低
**Phase**: Phase 4 — 可观测性 + CI/CD

---

## 背景

V1 无任何指标暴露。V2 引入 `github.com/prometheus/client_golang` 采集关键业务指标和 HTTP 指标，
通过 `/metrics` 端点暴露给 Prometheus 抓取。

---

## 实现步骤

### Step 1: 引入依赖

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

### Step 2: 指标定义

新建 `internal/observability/metrics.go`：

```go
package observability

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

// ──────────────────────────────
// HTTP 指标
// ──────────────────────────────

var (
    // HTTPRequestsTotal HTTP 请求总数（按 method/path/status 分类）
    HTTPRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ndt_http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"method", "path", "status"},
    )

    // HTTPRequestDuration HTTP 请求耗时分布
    HTTPRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "ndt_http_request_duration_seconds",
            Help:    "HTTP request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path"},
    )
)

// ──────────────────────────────
// 同步指标
// ──────────────────────────────

var (
    // SyncOperationsTotal 同步操作总数
    SyncOperationsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ndt_sync_operations_total",
            Help: "Total number of sync operations",
        },
        []string{"type", "status"}, // type: full/incremental, status: success/error
    )

    // SyncDuration 同步耗时
    SyncDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "ndt_sync_duration_seconds",
            Help:    "Sync operation duration in seconds",
            Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120},
        },
        []string{"type"}, // full/incremental
    )

    // SyncNodesCreated 同步创建的节点数
    SyncNodesCreated = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "ndt_sync_nodes_created_total",
            Help: "Total nodes created by sync",
        },
    )

    // SyncRelationsCreated 同步创建的关系数
    SyncRelationsCreated = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "ndt_sync_relations_created_total",
            Help: "Total relations created by sync",
        },
    )
)

// ──────────────────────────────
// 快照指标
// ──────────────────────────────

var (
    // SnapshotOperationsTotal 快照操作总数
    SnapshotOperationsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ndt_snapshot_operations_total",
            Help: "Total number of snapshot operations",
        },
        []string{"operation", "status"}, // operation: create/restore/delete/diff
    )

    // SnapshotCount 当前活跃快照数（Gauge）
    SnapshotCount = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "ndt_snapshot_active_count",
            Help: "Current number of active snapshots",
        },
    )
)

// ──────────────────────────────
// Kafka 指标（V2-02/03 使用）
// ──────────────────────────────

var (
    // KafkaMessagesTotal Kafka 消息总数
    KafkaMessagesTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ndt_kafka_messages_total",
            Help: "Total Kafka messages produced/consumed",
        },
        []string{"direction", "topic"}, // direction: produced/consumed
    )

    // KafkaConsumerLag Kafka Consumer 延迟
    KafkaConsumerLag = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "ndt_kafka_consumer_lag",
            Help: "Kafka consumer group lag per partition",
        },
        []string{"topic", "partition"},
    )
)

// ──────────────────────────────
// PostgreSQL 指标（V2-05+ 使用）
// ──────────────────────────────

var (
    // PGQueryDuration PostgreSQL 查询耗时
    PGQueryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "ndt_pg_query_duration_seconds",
            Help:    "PostgreSQL query duration",
            Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
        },
        []string{"table", "operation"}, // table: snapshots/sync_logs/..., operation: select/insert/...
    )

    // PGPoolConnections PG 连接池状态
    PGPoolConnections = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "ndt_pg_pool_connections",
            Help: "PostgreSQL connection pool status",
        },
        []string{"state"}, // state: acquired/idle/total
    )
)
```

### Step 3: HTTP 指标中间件

新建 `internal/api/middleware/metrics.go`：

```go
package middleware

import (
    "strconv"
    "time"

    "github.com/gin-gonic/gin"

    "gitlab.com/pml/network-digital-twin/internal/observability"
)

// Metrics Prometheus 指标采集中间件。
func Metrics() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.FullPath() // 使用路由模板（如 /api/v1/snapshot/:name）
        if path == "" {
            path = "unknown"
        }

        c.Next()

        duration := time.Since(start).Seconds()
        status := strconv.Itoa(c.Writer.Status())

        observability.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
        observability.HTTPRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
    }
}
```

### Step 4: 注册 /metrics 端点

修改 `internal/api/server.go`：

```go
import (
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "gitlab.com/pml/network-digital-twin/internal/api/middleware"
)

func NewServer() *Server {
    // ... 现有代码 ...

    // 全局中间件新增 Metrics
    engine.Use(
        gin.Recovery(),
        middleware.CORS(),
        middleware.RequestID(),
        middleware.Logger(),
        middleware.Metrics(),    // 新增
    )

    // /metrics 端点（Prometheus 抓取）
    engine.GET("/metrics", gin.WrapH(promhttp.Handler()))

    v1 := engine.Group("/api/v1")
    return &Server{engine: engine, router: v1}
}
```

### Step 5: 业务指标埋点

修改 `internal/service/sync_service.go`，添加指标上报：

```go
import "gitlab.com/pml/network-digital-twin/internal/observability"

func (s *SyncService) FullSync(ctx context.Context) (*SyncResult, error) {
    start := time.Now()

    result, err := s.doFullSync(ctx)

    duration := time.Since(start).Seconds()
    if err != nil {
        observability.SyncOperationsTotal.WithLabelValues("full", "error").Inc()
        return nil, err
    }

    observability.SyncOperationsTotal.WithLabelValues("full", "success").Inc()
    observability.SyncDuration.WithLabelValues("full").Observe(duration)
    observability.SyncNodesCreated.Add(float64(result.NodesCreated))
    observability.SyncRelationsCreated.Add(float64(result.RelationsCreated))

    return result, nil
}
```

类似地，在 `IncrementalSync`、`SnapshotManager.Create/Restore/Delete` 中添加指标埋点。

### Step 6: 单元测试

新建 `internal/observability/metrics_test.go`：

| 测试 | 验证点 |
|------|--------|
| `TestMetricsRegistered` | 所有指标注册成功，无 panic |
| `TestHTTPMetricsMiddleware` | 请求后 Counter/Histogram 值正确递增 |
| `TestSyncMetrics` | FullSync 后 sync 指标正确上报 |
| `TestMetricsEndpoint` | GET /metrics 返回 Prometheus 格式文本 |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `go.mod` | 修改 | 新增 prometheus/client_golang |
| `internal/observability/metrics.go` | 新增 | 全部指标定义 |
| `internal/observability/metrics_test.go` | 新增 | 指标注册和上报测试 |
| `internal/api/middleware/metrics.go` | 新增 | HTTP 指标中间件 |
| `internal/api/server.go` | 修改 | 注册 /metrics + Metrics 中间件 |
| `internal/service/sync_service.go` | 修改 | FullSync/IncrementalSync 指标埋点 |
| `internal/snapshot/manager.go` | 修改 | Create/Restore/Delete 指标埋点 |

---

## 指标一览表

| 指标名 | 类型 | 标签 | 说明 |
|--------|------|------|------|
| `ndt_http_requests_total` | Counter | method, path, status | HTTP 请求总数 |
| `ndt_http_request_duration_seconds` | Histogram | method, path | HTTP 请求耗时 |
| `ndt_sync_operations_total` | Counter | type, status | 同步操作总数 |
| `ndt_sync_duration_seconds` | Histogram | type | 同步耗时 |
| `ndt_sync_nodes_created_total` | Counter | - | 同步创建节点数 |
| `ndt_sync_relations_created_total` | Counter | - | 同步创建关系数 |
| `ndt_snapshot_operations_total` | Counter | operation, status | 快照操作总数 |
| `ndt_snapshot_active_count` | Gauge | - | 当前活跃快照数 |
| `ndt_kafka_messages_total` | Counter | direction, topic | Kafka 消息总数 |
| `ndt_kafka_consumer_lag` | Gauge | topic, partition | Consumer 延迟 |
| `ndt_pg_query_duration_seconds` | Histogram | table, operation | PG 查询耗时 |
| `ndt_pg_pool_connections` | Gauge | state | PG 连接池状态 |

---

## 注意事项

1. **指标前缀**: 统一使用 `ndt_` (Network Digital Twin) 前缀，避免与其他服务冲突
2. **path 标签**: 使用 `c.FullPath()` 获取路由模板（如 `/api/v1/snapshot/:name`），避免高基数（每个快照名一个标签）
3. **Histogram Buckets**: 根据业务场景自定义桶分布，HTTP 用默认桶，Sync 用大步长桶
4. **/metrics 安全**: 生产环境可通过 Basic Auth 或 IP 白名单限制访问
5. **性能开销**: Prometheus Counter/Histogram 操作为原子操作，对性能影响可忽略

---

## 验收标准

- [ ] `GET /metrics` 返回 Prometheus 格式指标文本
- [ ] HTTP 请求后 `ndt_http_requests_total` 正确递增
- [ ] FullSync 后 `ndt_sync_operations_total` 和 `ndt_sync_duration_seconds` 正确上报
- [ ] 快照操作后 `ndt_snapshot_operations_total` 正确递增
- [ ] `go test ./internal/observability/... ./internal/api/...` 全部通过
