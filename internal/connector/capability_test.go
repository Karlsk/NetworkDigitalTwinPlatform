package connector

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ──────────────────────────────
// mock 实现：验证类型断言机制
// ──────────────────────────────

// stubMonitorQuerier 实现 MonitorQuerier，所有方法返回 ErrNotImplemented。
type stubMonitorQuerier struct{}

var _ MonitorQuerier = (*stubMonitorQuerier)(nil)

func (s *stubMonitorQuerier) QueryDeviceMetrics(_ context.Context, _ string, _ []string, _, _ time.Time) (*MetricsResult, error) {
	return nil, ErrNotImplemented
}
func (s *stubMonitorQuerier) QueryPortMetrics(_ context.Context, _, _ string, _ []string, _, _ time.Time) (*MetricsResult, error) {
	return nil, ErrNotImplemented
}
func (s *stubMonitorQuerier) QueryVPNTraffic(_ context.Context, _ string, _ []string, _, _ time.Time) (*MetricsResult, error) {
	return nil, ErrNotImplemented
}
func (s *stubMonitorQuerier) QueryTunnelTraffic(_ context.Context, _, _ string, _ []string, _, _ time.Time) (*MetricsResult, error) {
	return nil, ErrNotImplemented
}
func (s *stubMonitorQuerier) QueryAlerts(_ context.Context, _, _ string) ([]map[string]any, error) {
	return nil, ErrNotImplemented
}
func (s *stubMonitorQuerier) QueryLogs(_ context.Context, _ string, _ LogQueryOptions) (*LogResult, error) {
	return nil, ErrNotImplemented
}

// stubDeviceOperator 实现 DeviceOperator，所有方法返回 ErrNotImplemented。
type stubDeviceOperator struct{}

var _ DeviceOperator = (*stubDeviceOperator)(nil)

func (s *stubDeviceOperator) QueryDeviceConfig(_ context.Context, _ string) (map[string]any, error) {
	return nil, ErrNotImplemented
}
func (s *stubDeviceOperator) QueryISISNeighbors(_ context.Context, _ string) ([]map[string]any, error) {
	return nil, ErrNotImplemented
}
func (s *stubDeviceOperator) QueryBGPPeers(_ context.Context, _ string) ([]map[string]any, error) {
	return nil, ErrNotImplemented
}
func (s *stubDeviceOperator) QueryVPNConfig(_ context.Context, _ string) (map[string]any, error) {
	return nil, ErrNotImplemented
}
func (s *stubDeviceOperator) QueryGlobalRoute(_ context.Context, _ string) ([]map[string]any, error) {
	return nil, ErrNotImplemented
}
func (s *stubDeviceOperator) QueryTopologyLive(_ context.Context) (*TopologyLiveResult, error) {
	return nil, ErrNotImplemented
}

// ──────────────────────────────
// 公共数据结构测试
// ──────────────────────────────

func TestMetricsResultZeroValue(t *testing.T) {
	var m MetricsResult
	if m.Device != "" {
		t.Errorf("zero value Device = %q, want empty", m.Device)
	}
	if m.Metrics != nil {
		t.Errorf("zero value Metrics = %v, want nil", m.Metrics)
	}
}

func TestMetricSeriesZeroValue(t *testing.T) {
	var s MetricSeries
	if s.Name != "" {
		t.Errorf("zero value Name = %q, want empty", s.Name)
	}
	if s.DataPoints != nil {
		t.Errorf("zero value DataPoints = %v, want nil", s.DataPoints)
	}
}

func TestDataPointZeroValue(t *testing.T) {
	var d DataPoint
	if !d.Timestamp.IsZero() {
		t.Errorf("zero value Timestamp = %v, want zero", d.Timestamp)
	}
	if d.Value != 0 {
		t.Errorf("zero value Value = %v, want 0", d.Value)
	}
}

func TestLogQueryOptionsZeroValue(t *testing.T) {
	var opts LogQueryOptions
	if opts.Interval != "" {
		t.Errorf("zero value Interval = %q, want empty", opts.Interval)
	}
	if opts.PageNum != 0 {
		t.Errorf("zero value PageNum = %d, want 0", opts.PageNum)
	}
}

func TestLogResultZeroValue(t *testing.T) {
	var r LogResult
	if r.Logs != nil {
		t.Errorf("zero value Logs = %v, want nil", r.Logs)
	}
	if r.TotalCount != 0 {
		t.Errorf("zero value TotalCount = %d, want 0", r.TotalCount)
	}
}

func TestTopologyLiveResultZeroValue(t *testing.T) {
	var r TopologyLiveResult
	if r.Nodes != nil {
		t.Errorf("zero value Nodes = %v, want nil", r.Nodes)
	}
	if r.Links != nil {
		t.Errorf("zero value Links = %v, want nil", r.Links)
	}
}

// ──────────────────────────────
// ErrNotImplemented 检测测试
// ──────────────────────────────

func TestErrNotImplementedDetectableAfterWrapping(t *testing.T) {
	wrapped := errors.Join(errors.New("capability call failed"), ErrNotImplemented)
	if !errors.Is(wrapped, ErrNotImplemented) {
		t.Error("errors.Is(wrapped, ErrNotImplemented) = false, want true")
	}
}

// ──────────────────────────────
// 类型断言发现能力测试
// ──────────────────────────────

func TestMonitorQuerierTypeAssertion(t *testing.T) {
	// stubMonitorQuerier 实现了 MonitorQuerier，类型断言应成功
	var s any = &stubMonitorQuerier{}
	mq, ok := s.(MonitorQuerier)
	if !ok {
		t.Fatal("type assertion to MonitorQuerier failed, want success")
	}

	// 调用应返回 ErrNotImplemented
	_, err := mq.QueryDeviceMetrics(context.Background(), "device1", nil, time.Time{}, time.Time{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryDeviceMetrics() error = %v, want ErrNotImplemented", err)
	}
	_, err = mq.QueryPortMetrics(context.Background(), "device1", "port1", nil, time.Time{}, time.Time{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryPortMetrics() error = %v, want ErrNotImplemented", err)
	}
	_, err = mq.QueryVPNTraffic(context.Background(), "vpn1", nil, time.Time{}, time.Time{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryVPNTraffic() error = %v, want ErrNotImplemented", err)
	}
	_, err = mq.QueryTunnelTraffic(context.Background(), "device1", "tunnel1", nil, time.Time{}, time.Time{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryTunnelTraffic() error = %v, want ErrNotImplemented", err)
	}
	_, err = mq.QueryAlerts(context.Background(), "business", "1h")
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryAlerts() error = %v, want ErrNotImplemented", err)
	}
	_, err = mq.QueryLogs(context.Background(), "system", LogQueryOptions{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryLogs() error = %v, want ErrNotImplemented", err)
	}
}

func TestDeviceOperatorTypeAssertion(t *testing.T) {
	// stubDeviceOperator 实现了 DeviceOperator，类型断言应成功
	var s any = &stubDeviceOperator{}
	dop, ok := s.(DeviceOperator)
	if !ok {
		t.Fatal("type assertion to DeviceOperator failed, want success")
	}

	// 调用应返回 ErrNotImplemented
	ctx := context.Background()
	if _, err := dop.QueryDeviceConfig(ctx, "device1"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryDeviceConfig() error = %v, want ErrNotImplemented", err)
	}
	if _, err := dop.QueryISISNeighbors(ctx, "device1"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryISISNeighbors() error = %v, want ErrNotImplemented", err)
	}
	if _, err := dop.QueryBGPPeers(ctx, "device1"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryBGPPeers() error = %v, want ErrNotImplemented", err)
	}
	if _, err := dop.QueryVPNConfig(ctx, "device1"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryVPNConfig() error = %v, want ErrNotImplemented", err)
	}
	if _, err := dop.QueryGlobalRoute(ctx, "device1"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryGlobalRoute() error = %v, want ErrNotImplemented", err)
	}
	if _, err := dop.QueryTopologyLive(ctx); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("QueryTopologyLive() error = %v, want ErrNotImplemented", err)
	}
}

// 不支持能力的类型（仅实现 Connector），类型断言应失败
func TestUnsupportedConnectorTypeAssertion(t *testing.T) {
	// mockConnector（来自 interface_test.go）只实现 Connector，不实现能力接口
	c := &mockConnector{meta: ConnectorMetadata{Name: "test", Type: "test"}}
	var s any = c

	if _, ok := s.(MonitorQuerier); ok {
		t.Error("type assertion to MonitorQuerier succeeded for unsupported type, want failure")
	}
	if _, ok := s.(DeviceOperator); ok {
		t.Error("type assertion to DeviceOperator succeeded for unsupported type, want failure")
	}
}
