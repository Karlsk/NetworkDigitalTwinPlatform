// Package service 实现业务编排层
package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ---------------------------------------------------------------------------
// capabilityConnector — 同时实现 Connector + MonitorQuerier + DeviceOperator
// ---------------------------------------------------------------------------

// capabilityConnector 用于正向测试，实现全部能力接口。
type capabilityConnector struct {
	mockConnector // 嵌入基础 Connector mock

	// MonitorQuerier 可配置返回
	metricsResult *connector.MetricsResult
	metricsErr    error
	alertsResult  []map[string]any
	alertsErr     error
	logsResult    *connector.LogResult
	logsErr       error

	// DeviceOperator 可配置返回
	configResult   map[string]any
	configErr      error
	isisResult     []map[string]any
	isisErr        error
	bgpResult      []map[string]any
	bgpErr         error
	vpnResult      map[string]any
	vpnErr         error
	routeResult    []map[string]any
	routeErr       error
	topologyResult *connector.TopologyLiveResult
	topologyErr    error
}

// 编译时接口满足检查
var _ connector.Connector = (*capabilityConnector)(nil)
var _ connector.MonitorQuerier = (*capabilityConnector)(nil)
var _ connector.DeviceOperator = (*capabilityConnector)(nil)

// MonitorQuerier 实现
func (c *capabilityConnector) QueryDeviceMetrics(_ context.Context, _ string, _ []string, _, _ time.Time) (*connector.MetricsResult, error) {
	return c.metricsResult, c.metricsErr
}
func (c *capabilityConnector) QueryPortMetrics(_ context.Context, _, _ string, _ []string, _, _ time.Time) (*connector.MetricsResult, error) {
	return c.metricsResult, c.metricsErr
}
func (c *capabilityConnector) QueryVPNTraffic(_ context.Context, _ string, _ []string, _, _ time.Time) (*connector.MetricsResult, error) {
	return c.metricsResult, c.metricsErr
}
func (c *capabilityConnector) QueryTunnelTraffic(_ context.Context, _, _ string, _ []string, _, _ time.Time) (*connector.MetricsResult, error) {
	return c.metricsResult, c.metricsErr
}
func (c *capabilityConnector) QueryAlerts(_ context.Context, _, _ string) ([]map[string]any, error) {
	return c.alertsResult, c.alertsErr
}
func (c *capabilityConnector) QueryLogs(_ context.Context, _ string, _ connector.LogQueryOptions) (*connector.LogResult, error) {
	return c.logsResult, c.logsErr
}

// DeviceOperator 实现
func (c *capabilityConnector) QueryDeviceConfig(_ context.Context, _ string) (map[string]any, error) {
	return c.configResult, c.configErr
}
func (c *capabilityConnector) QueryISISNeighbors(_ context.Context, _ string) ([]map[string]any, error) {
	return c.isisResult, c.isisErr
}
func (c *capabilityConnector) QueryBGPPeers(_ context.Context, _ string) ([]map[string]any, error) {
	return c.bgpResult, c.bgpErr
}
func (c *capabilityConnector) QueryVPNConfig(_ context.Context, _ string) (map[string]any, error) {
	return c.vpnResult, c.vpnErr
}
func (c *capabilityConnector) QueryGlobalRoute(_ context.Context, _ string) ([]map[string]any, error) {
	return c.routeResult, c.routeErr
}
func (c *capabilityConnector) QueryTopologyLive(_ context.Context) (*connector.TopologyLiveResult, error) {
	return c.topologyResult, c.topologyErr
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// setupDeviceService 创建带有 capabilityConnector 的 DeviceService。
func setupDeviceService(t *testing.T, cc *capabilityConnector) *DeviceService {
	t.Helper()
	reg := connector.NewConnectorRegistry()
	reg.Register(cc)
	return NewDeviceService(reg)
}

// setupDeviceServiceNoCapability 创建带有普通 mockConnector 的 DeviceService（不实现能力接口）。
func setupDeviceServiceNoCapability(t *testing.T) *DeviceService {
	t.Helper()
	reg := connector.NewConnectorRegistry()
	mc := &mockConnector{name: "basic", entityTypes: []string{"Device"}}
	reg.Register(mc)
	return NewDeviceService(reg)
}

// ---------------------------------------------------------------------------
// TC-D01: QueryMonitor — Connector 实现 MonitorQuerier → 返回结果
// ---------------------------------------------------------------------------

func TestQueryMonitor_DeviceMetrics(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		metricsResult: &connector.MetricsResult{
			Device:  "router-01",
			Metrics: []connector.MetricSeries{{Name: "cpu_usage", DataPoints: []connector.DataPoint{{Value: 42.5}}}},
		},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryMonitor(context.Background(), MonitorRequest{
		ConnectorName: "ctrl",
		QueryType:     "device",
		Device:        "router-01",
		Metrics:       []string{"cpu_usage"},
	})
	if err != nil {
		t.Fatalf("QueryMonitor(device) error = %v", err)
	}
	mr, ok := result.(*connector.MetricsResult)
	if !ok {
		t.Fatalf("expected *MetricsResult, got %T", result)
	}
	if mr.Device != "router-01" {
		t.Errorf("Device = %q, want router-01", mr.Device)
	}
	if len(mr.Metrics) != 1 || mr.Metrics[0].Name != "cpu_usage" {
		t.Errorf("Metrics mismatch: %+v", mr.Metrics)
	}
}

func TestQueryMonitor_PortMetrics(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		metricsResult: &connector.MetricsResult{Device: "router-01", Port: "eth0"},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryMonitor(context.Background(), MonitorRequest{
		ConnectorName: "ctrl", QueryType: "port", Device: "router-01", Port: "eth0",
	})
	if err != nil {
		t.Fatalf("QueryMonitor(port) error = %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

func TestQueryMonitor_VPNTraffic(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		metricsResult: &connector.MetricsResult{VPN: "vpn-001"},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryMonitor(context.Background(), MonitorRequest{
		ConnectorName: "ctrl", QueryType: "vpn", VPNID: "vpn-001",
	})
	if err != nil {
		t.Fatalf("QueryMonitor(vpn) error = %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

func TestQueryMonitor_TunnelTraffic(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		metricsResult: &connector.MetricsResult{Tunnel: "tunnel-001"},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryMonitor(context.Background(), MonitorRequest{
		ConnectorName: "ctrl", QueryType: "tunnel", Device: "router-01", Tunnel: "tunnel-001",
	})
	if err != nil {
		t.Fatalf("QueryMonitor(tunnel) error = %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

func TestQueryMonitor_Alerts(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		alertsResult:  []map[string]any{{"alert_id": "A001", "severity": "critical"}},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryMonitor(context.Background(), MonitorRequest{
		ConnectorName: "ctrl", QueryType: "alerts", Namespace: "default", Interval: "1h",
	})
	if err != nil {
		t.Fatalf("QueryMonitor(alerts) error = %v", err)
	}
	alerts, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(alerts) != 1 {
		t.Errorf("len(alerts) = %d, want 1", len(alerts))
	}
}

func TestQueryMonitor_Logs(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		logsResult:    &connector.LogResult{Logs: []map[string]any{{"msg": "login success"}}, TotalCount: 1},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryMonitor(context.Background(), MonitorRequest{
		ConnectorName: "ctrl", QueryType: "logs", LogType: "system", Interval: "24h",
	})
	if err != nil {
		t.Fatalf("QueryMonitor(logs) error = %v", err)
	}
	lr, ok := result.(*connector.LogResult)
	if !ok {
		t.Fatalf("expected *LogResult, got %T", result)
	}
	if lr.TotalCount != 1 {
		t.Errorf("TotalCount = %d, want 1", lr.TotalCount)
	}
}

// ---------------------------------------------------------------------------
// TC-D02: QueryMonitor — Connector 未实现 MonitorQuerier → 错误
// ---------------------------------------------------------------------------

func TestQueryMonitor_UnsupportedConnector(t *testing.T) {
	svc := setupDeviceServiceNoCapability(t)

	_, err := svc.QueryMonitor(context.Background(), MonitorRequest{
		ConnectorName: "basic", QueryType: "device",
	})
	if err == nil {
		t.Fatal("expected error for unsupported connector")
	}
	if !strings.Contains(err.Error(), "does not support monitoring") {
		t.Errorf("error = %q, want 'does not support monitoring'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TC-D03: QueryMonitor — 无效 query_type → 错误
// ---------------------------------------------------------------------------

func TestQueryMonitor_InvalidQueryType(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
	}
	svc := setupDeviceService(t, cc)

	_, err := svc.QueryMonitor(context.Background(), MonitorRequest{
		ConnectorName: "ctrl", QueryType: "invalid_type",
	})
	if err == nil {
		t.Fatal("expected error for invalid query_type")
	}
	if !strings.Contains(err.Error(), "unknown monitor query_type") {
		t.Errorf("error = %q, want 'unknown monitor query_type'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TC-D04: QueryMonitor — Connector 不存在 → 错误
// ---------------------------------------------------------------------------

func TestQueryMonitor_ConnectorNotFound(t *testing.T) {
	svc := setupDeviceServiceNoCapability(t)

	_, err := svc.QueryMonitor(context.Background(), MonitorRequest{
		ConnectorName: "nonexistent", QueryType: "device",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent connector")
	}
	if !errors.Is(err, connector.ErrConnectorNotFound) {
		t.Errorf("error = %v, want ErrConnectorNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// TC-D05: QueryDeviceInfo — Connector 实现 DeviceOperator → 返回结果
// ---------------------------------------------------------------------------

func TestQueryDeviceInfo_Config(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		configResult:  map[string]any{"config": "hostname router-01", "device": "router-01"},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryDeviceInfo(context.Background(), DeviceInfoRequest{
		ConnectorName: "ctrl", QueryType: "config", Device: "router-01",
	})
	if err != nil {
		t.Fatalf("QueryDeviceInfo(config) error = %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["device"] != "router-01" {
		t.Errorf("device = %v, want router-01", m["device"])
	}
}

func TestQueryDeviceInfo_ISIS(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		isisResult:    []map[string]any{{"neighbor": "router-02", "state": "up"}},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryDeviceInfo(context.Background(), DeviceInfoRequest{
		ConnectorName: "ctrl", QueryType: "isis", Device: "router-01",
	})
	if err != nil {
		t.Fatalf("QueryDeviceInfo(isis) error = %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

func TestQueryDeviceInfo_BGP(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		bgpResult:     []map[string]any{{"peer": "10.0.0.1", "state": "Established"}},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryDeviceInfo(context.Background(), DeviceInfoRequest{
		ConnectorName: "ctrl", QueryType: "bgp", Device: "router-01",
	})
	if err != nil {
		t.Fatalf("QueryDeviceInfo(bgp) error = %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

func TestQueryDeviceInfo_VPNConfig(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		vpnResult:     map[string]any{"vpn_name": "l3vpn-01"},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryDeviceInfo(context.Background(), DeviceInfoRequest{
		ConnectorName: "ctrl", QueryType: "vpn_config", Device: "router-01",
	})
	if err != nil {
		t.Fatalf("QueryDeviceInfo(vpn_config) error = %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

func TestQueryDeviceInfo_Route(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		routeResult:   []map[string]any{{"prefix": "10.0.0.0/24", "nexthop": "192.168.1.1"}},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryDeviceInfo(context.Background(), DeviceInfoRequest{
		ConnectorName: "ctrl", QueryType: "route", Device: "router-01",
	})
	if err != nil {
		t.Fatalf("QueryDeviceInfo(route) error = %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

func TestQueryDeviceInfo_Topology(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector:  mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		topologyResult: &connector.TopologyLiveResult{Nodes: []map[string]any{{"id": "r1"}}, Links: []map[string]any{{"from": "r1", "to": "r2"}}},
	}
	svc := setupDeviceService(t, cc)

	result, err := svc.QueryDeviceInfo(context.Background(), DeviceInfoRequest{
		ConnectorName: "ctrl", QueryType: "topology",
	})
	if err != nil {
		t.Fatalf("QueryDeviceInfo(topology) error = %v", err)
	}
	tr, ok := result.(*connector.TopologyLiveResult)
	if !ok {
		t.Fatalf("expected *TopologyLiveResult, got %T", result)
	}
	if len(tr.Nodes) != 1 || len(tr.Links) != 1 {
		t.Errorf("topology mismatch: nodes=%d, links=%d", len(tr.Nodes), len(tr.Links))
	}
}

// ---------------------------------------------------------------------------
// TC-D06: QueryDeviceInfo — Connector 未实现 DeviceOperator → 错误
// ---------------------------------------------------------------------------

func TestQueryDeviceInfo_UnsupportedConnector(t *testing.T) {
	svc := setupDeviceServiceNoCapability(t)

	_, err := svc.QueryDeviceInfo(context.Background(), DeviceInfoRequest{
		ConnectorName: "basic", QueryType: "config", Device: "router-01",
	})
	if err == nil {
		t.Fatal("expected error for unsupported connector")
	}
	if !strings.Contains(err.Error(), "does not support device operations") {
		t.Errorf("error = %q, want 'does not support device operations'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TC-D07: QueryDeviceInfo — 无效 query_type → 错误
// ---------------------------------------------------------------------------

func TestQueryDeviceInfo_InvalidQueryType(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
	}
	svc := setupDeviceService(t, cc)

	_, err := svc.QueryDeviceInfo(context.Background(), DeviceInfoRequest{
		ConnectorName: "ctrl", QueryType: "invalid_type",
	})
	if err == nil {
		t.Fatal("expected error for invalid query_type")
	}
	if !strings.Contains(err.Error(), "unknown device query_type") {
		t.Errorf("error = %q, want 'unknown device query_type'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TC-D08: QueryMonitor — 下层方法返回错误 → 向上传播
// ---------------------------------------------------------------------------

func TestQueryMonitor_MetricsError(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		metricsErr:    errors.New("controller timeout"),
	}
	svc := setupDeviceService(t, cc)

	_, err := svc.QueryMonitor(context.Background(), MonitorRequest{
		ConnectorName: "ctrl", QueryType: "device", Device: "router-01",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "controller timeout") {
		t.Errorf("error = %q, want containing 'controller timeout'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TC-D09: QueryDeviceInfo — 下层方法返回错误 → 向上传播
// ---------------------------------------------------------------------------

func TestQueryDeviceInfo_ConfigError(t *testing.T) {
	cc := &capabilityConnector{
		mockConnector: mockConnector{name: "ctrl", entityTypes: []string{"Device"}},
		configErr:     errors.New("device unreachable"),
	}
	svc := setupDeviceService(t, cc)

	_, err := svc.QueryDeviceInfo(context.Background(), DeviceInfoRequest{
		ConnectorName: "ctrl", QueryType: "config", Device: "router-01",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "device unreachable") {
		t.Errorf("error = %q, want containing 'device unreachable'", err.Error())
	}
}
