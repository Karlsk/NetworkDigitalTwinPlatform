// Package controller 实现 Controller Connector 测试。
// monitor_test.go 验证 MonitorQuerier 能力接口的实现正确性。
package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ──────────────────────────────
// 编译时接口满足检查测试
// ──────────────────────────────

func TestMonitorQuerierCompileTimeCheck(t *testing.T) {
	// 编译时已通过 var _ connector.MonitorQuerier = (*ControllerConnector)(nil) 验证
	// 此测试额外验证运行时类型断言
	var c connector.Connector = NewControllerConnector("test", nil, nil, "")
	_, ok := c.(connector.MonitorQuerier)
	if !ok {
		t.Fatal("ControllerConnector does not implement MonitorQuerier, want implementation")
	}
}

// ──────────────────────────────
// MonitorQuerier 方法返回 ErrNotImplemented 测试
// ──────────────────────────────

func newMonitorTestConnector() *ControllerConnector {
	return &ControllerConnector{
		name: "test-controller",
	}
}

func TestQueryDeviceMetricsReturnsErrNotImplemented(t *testing.T) {
	c := newMonitorTestConnector()
	result, err := c.QueryDeviceMetrics(context.Background(), "device1", []string{"cpu_usage"}, time.Now(), time.Now())
	if result != nil {
		t.Errorf("QueryDeviceMetrics() result = %v, want nil", result)
	}
	if !errors.Is(err, connector.ErrNotImplemented) {
		t.Errorf("QueryDeviceMetrics() error = %v, want ErrNotImplemented", err)
	}
}

func TestQueryPortMetricsReturnsErrNotImplemented(t *testing.T) {
	c := newMonitorTestConnector()
	result, err := c.QueryPortMetrics(context.Background(), "device1", "port1", []string{"in_traffic"}, time.Now(), time.Now())
	if result != nil {
		t.Errorf("QueryPortMetrics() result = %v, want nil", result)
	}
	if !errors.Is(err, connector.ErrNotImplemented) {
		t.Errorf("QueryPortMetrics() error = %v, want ErrNotImplemented", err)
	}
}

func TestQueryVPNTrafficReturnsErrNotImplemented(t *testing.T) {
	c := newMonitorTestConnector()
	result, err := c.QueryVPNTraffic(context.Background(), "vpn-001", []string{"throughput"}, time.Now(), time.Now())
	if result != nil {
		t.Errorf("QueryVPNTraffic() result = %v, want nil", result)
	}
	if !errors.Is(err, connector.ErrNotImplemented) {
		t.Errorf("QueryVPNTraffic() error = %v, want ErrNotImplemented", err)
	}
}

func TestQueryTunnelTrafficReturnsErrNotImplemented(t *testing.T) {
	c := newMonitorTestConnector()
	result, err := c.QueryTunnelTraffic(context.Background(), "device1", "tunnel1", []string{"bandwidth"}, time.Now(), time.Now())
	if result != nil {
		t.Errorf("QueryTunnelTraffic() result = %v, want nil", result)
	}
	if !errors.Is(err, connector.ErrNotImplemented) {
		t.Errorf("QueryTunnelTraffic() error = %v, want ErrNotImplemented", err)
	}
}

func TestQueryAlertsReturnsErrNotImplemented(t *testing.T) {
	c := newMonitorTestConnector()
	result, err := c.QueryAlerts(context.Background(), "business", "1h")
	if result != nil {
		t.Errorf("QueryAlerts() result = %v, want nil", result)
	}
	if !errors.Is(err, connector.ErrNotImplemented) {
		t.Errorf("QueryAlerts() error = %v, want ErrNotImplemented", err)
	}
}

func TestQueryLogsReturnsErrNotImplemented(t *testing.T) {
	c := newMonitorTestConnector()
	result, err := c.QueryLogs(context.Background(), "system", connector.LogQueryOptions{
		Interval: "1h",
		PageNum:  1,
		PageSize: 20,
	})
	if result != nil {
		t.Errorf("QueryLogs() result = %v, want nil", result)
	}
	if !errors.Is(err, connector.ErrNotImplemented) {
		t.Errorf("QueryLogs() error = %v, want ErrNotImplemented", err)
	}
}

// ──────────────────────────────
// 类型断言发现能力测试（Service/MCP 层使用模式）
// ──────────────────────────────

func TestTypeAssertionDiscoverMonitorQuerier(t *testing.T) {
	// 模拟 Service/MCP 层通过类型断言发现 MonitorQuerier 能力
	c := NewControllerConnector("test-controller", nil, []string{"Device"}, "http://localhost")
	var conn connector.Connector = c

	// ControllerConnector 实现了 MonitorQuerier，类型断言应成功
	mq, ok := conn.(connector.MonitorQuerier)
	if !ok {
		t.Fatal("type assertion to MonitorQuerier failed, want success")
	}
	if mq == nil {
		t.Fatal("MonitorQuerier is nil after type assertion")
	}
}

func TestTypeAssertionFromInterfaceToMonitorQuerier(t *testing.T) {
	// 验证从 any 类型通过类型断言发现 MonitorQuerier
	c := NewControllerConnector("test-controller", nil, nil, "")
	var obj any = c

	mq, ok := obj.(connector.MonitorQuerier)
	if !ok {
		t.Fatal("type assertion from any to MonitorQuerier failed, want success")
	}
	if mq == nil {
		t.Fatal("MonitorQuerier is nil after type assertion from any")
	}
}

// ──────────────────────────────
// MonitorQuerier 方法数量验证
// ──────────────────────────────

func TestMonitorQuerierMethodCount(t *testing.T) {
	// 文档定义 MonitorQuerier 有 6 个方法
	c := newMonitorTestConnector()
	ctx := context.Background()
	now := time.Now()

	// Method 1: QueryDeviceMetrics
	_, _ = c.QueryDeviceMetrics(ctx, "d", nil, now, now)
	// Method 2: QueryPortMetrics
	_, _ = c.QueryPortMetrics(ctx, "d", "p", nil, now, now)
	// Method 3: QueryVPNTraffic
	_, _ = c.QueryVPNTraffic(ctx, "v", nil, now, now)
	// Method 4: QueryTunnelTraffic
	_, _ = c.QueryTunnelTraffic(ctx, "d", "t", nil, now, now)
	// Method 5: QueryAlerts
	_, _ = c.QueryAlerts(ctx, "ns", "1h")
	// Method 6: QueryLogs
	_, _ = c.QueryLogs(ctx, "system", connector.LogQueryOptions{})

	// 如果编译通过，说明 6 个方法签名均正确
}
