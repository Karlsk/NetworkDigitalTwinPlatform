package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getCounterValue 从 Counter 获取当前值。
func getCounterValue(c prometheus.Counter) float64 {
	m := &dto.Metric{}
	if err := c.Write(m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

// getCounterVecValue 从 CounterVec 获取指定标签组合的当前值。
func getCounterVecValue(cv *prometheus.CounterVec, labels ...string) float64 {
	m := &dto.Metric{}
	if err := cv.WithLabelValues(labels...).Write(m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

// getGaugeValue 从 Gauge 获取当前值。
func getGaugeValue(g prometheus.Gauge) float64 {
	m := &dto.Metric{}
	if err := g.Write(m); err != nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

// getGaugeVecValue 从 GaugeVec 获取指定标签组合的当前值。
func getGaugeVecValue(gv *prometheus.GaugeVec, labels ...string) float64 {
	m := &dto.Metric{}
	if err := gv.WithLabelValues(labels...).Write(m); err != nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

// getHistogramCount 从 HistogramVec 获取指定标签组合的样本计数。
func getHistogramCount(hv *prometheus.HistogramVec, labels ...string) uint64 {
	m := &dto.Metric{}
	observer := hv.WithLabelValues(labels...).(prometheus.Histogram)
	if err := observer.Write(m); err != nil {
		return 0
	}
	return m.GetHistogram().GetSampleCount()
}

func TestMetricsRegistered(t *testing.T) {
	// 所有指标变量应非 nil
	assert.NotNil(t, HTTPRequestsTotal, "HTTPRequestsTotal should be registered")
	assert.NotNil(t, HTTPRequestDuration, "HTTPRequestDuration should be registered")
	assert.NotNil(t, SyncOperationsTotal, "SyncOperationsTotal should be registered")
	assert.NotNil(t, SyncDuration, "SyncDuration should be registered")
	assert.NotNil(t, SyncNodesCreated, "SyncNodesCreated should be registered")
	assert.NotNil(t, SyncRelationsCreated, "SyncRelationsCreated should be registered")
	assert.NotNil(t, SnapshotOperationsTotal, "SnapshotOperationsTotal should be registered")
	assert.NotNil(t, SnapshotCount, "SnapshotCount should be registered")
	assert.NotNil(t, KafkaMessagesTotal, "KafkaMessagesTotal should be registered")
	assert.NotNil(t, KafkaConsumerLag, "KafkaConsumerLag should be registered")
	assert.NotNil(t, PGQueryDuration, "PGQueryDuration should be registered")
	assert.NotNil(t, PGPoolConnections, "PGPoolConnections should be registered")

	// 触发所有 Vec 指标初始化，确保 Gather 可见
	HTTPRequestsTotal.WithLabelValues("_init", "_init", "_init")
	HTTPRequestDuration.WithLabelValues("_init", "_init")
	SyncOperationsTotal.WithLabelValues("_init", "_init")
	SyncDuration.WithLabelValues("_init")
	SnapshotOperationsTotal.WithLabelValues("_init", "_init")
	KafkaMessagesTotal.WithLabelValues("_init", "_init")
	KafkaConsumerLag.WithLabelValues("_init", "_init")
	PGQueryDuration.WithLabelValues("_init", "_init")
	PGPoolConnections.WithLabelValues("_init")
	SyncNodesCreated.Add(0)
	SyncRelationsCreated.Add(0)
	SnapshotCount.Add(0)

	// 验证指标已在默认 registry 注册
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	// 检查至少有 12 个 ndt_ 前缀的指标族
	var ndtCount int
	for _, f := range families {
		if len(f.GetName()) >= 4 && f.GetName()[:4] == "ndt_" {
			ndtCount++
		}
	}
	assert.GreaterOrEqual(t, ndtCount, 12, "should have at least 12 ndt_ metrics registered")
}

func TestSyncMetrics(t *testing.T) {
	// 记录初始值
	initFullSuccess := getCounterVecValue(SyncOperationsTotal, "full", "success")

	// 模拟 FullSync 成功
	SyncOperationsTotal.WithLabelValues("full", "success").Inc()
	val := getCounterVecValue(SyncOperationsTotal, "full", "success")
	assert.Equal(t, initFullSuccess+1, val)

	// 模拟节点和关系创建
	initNodes := getCounterValue(SyncNodesCreated)
	SyncNodesCreated.Add(float64(10))
	assert.Equal(t, initNodes+10, getCounterValue(SyncNodesCreated))

	initRels := getCounterValue(SyncRelationsCreated)
	SyncRelationsCreated.Add(float64(5))
	assert.Equal(t, initRels+5, getCounterValue(SyncRelationsCreated))

	// 模拟耗时观测
	initCount := getHistogramCount(SyncDuration, "full")
	SyncDuration.WithLabelValues("full").Observe(1.5)
	assert.Equal(t, initCount+1, getHistogramCount(SyncDuration, "full"))
}

func TestSnapshotMetrics(t *testing.T) {
	// Counter 递增
	initCreate := getCounterVecValue(SnapshotOperationsTotal, "create", "success")
	SnapshotOperationsTotal.WithLabelValues("create", "success").Inc()
	assert.Equal(t, initCreate+1, getCounterVecValue(SnapshotOperationsTotal, "create", "success"))

	// Gauge 操作
	initCount := getGaugeValue(SnapshotCount)
	SnapshotCount.Inc()
	assert.Equal(t, initCount+1, getGaugeValue(SnapshotCount))
	SnapshotCount.Dec()
	assert.Equal(t, initCount, getGaugeValue(SnapshotCount))
}

func TestKafkaMetrics(t *testing.T) {
	initProduced := getCounterVecValue(KafkaMessagesTotal, "produced", "sync-events")
	KafkaMessagesTotal.WithLabelValues("produced", "sync-events").Inc()
	assert.Equal(t, initProduced+1, getCounterVecValue(KafkaMessagesTotal, "produced", "sync-events"))

	KafkaConsumerLag.WithLabelValues("sync-events", "0").Set(42)
	assert.Equal(t, float64(42), getGaugeVecValue(KafkaConsumerLag, "sync-events", "0"))
}

func TestPGMetrics(t *testing.T) {
	initCount := getHistogramCount(PGQueryDuration, "snapshots", "select")
	PGQueryDuration.WithLabelValues("snapshots", "select").Observe(0.05)
	assert.Equal(t, initCount+1, getHistogramCount(PGQueryDuration, "snapshots", "select"))

	PGPoolConnections.WithLabelValues("acquired").Set(5)
	assert.Equal(t, float64(5), getGaugeVecValue(PGPoolConnections, "acquired"))
}
