// Package observability 提供 Prometheus 指标定义与采集
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
