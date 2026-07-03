// Package service 实现业务编排层。
// DeviceService 编排设备按需操作，通过类型断言发现 Connector 能力。
package service

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// DeviceService 设备按需操作编排层。
// 通过 ConnectorRegistry 获取 Connector，类型断言检查能力接口，
// 编排 MonitorQuerier / DeviceOperator 的调用。
type DeviceService struct {
	registry *connector.ConnectorRegistry
}

// NewDeviceService 创建 DeviceService 实例。
func NewDeviceService(registry *connector.ConnectorRegistry) *DeviceService {
	return &DeviceService{
		registry: registry,
	}
}

// ──────────────────────────────
// MonitorQuerier 编排方法
// ──────────────────────────────

// MonitorRequest 监控查询请求。
type MonitorRequest struct {
	ConnectorName string    `json:"connector_name"` // 目标 Connector 名称
	QueryType     string    `json:"query_type"`     // "device" / "port" / "vpn" / "tunnel" / "alerts" / "logs"
	Device        string    `json:"device,omitempty"`
	Port          string    `json:"port,omitempty"`
	VPNID         string    `json:"vpn_id,omitempty"`
	Tunnel        string    `json:"tunnel,omitempty"`
	Metrics       []string  `json:"metrics"`        // 指标名列表
	Namespace     string    `json:"namespace,omitempty"`
	Interval      string    `json:"interval,omitempty"`
	StartTime     time.Time `json:"start_time,omitempty"`
	EndTime       time.Time `json:"end_time,omitempty"`
	LogType       string    `json:"log_type,omitempty"` // "system" / "login"
}

// QueryMonitor 查询指定 Connector 的监控数据。
func (s *DeviceService) QueryMonitor(ctx context.Context, req MonitorRequest) (any, error) {
	conn, err := s.registry.Get(req.ConnectorName)
	if err != nil {
		return nil, fmt.Errorf("get connector: %w", err)
	}

	mq, ok := conn.(connector.MonitorQuerier)
	if !ok {
		return nil, fmt.Errorf("connector %q does not support monitoring (MonitorQuerier not implemented)", req.ConnectorName)
	}

	switch req.QueryType {
	case "device":
		result, err := mq.QueryDeviceMetrics(ctx, req.Device, req.Metrics, req.StartTime, req.EndTime)
		if err != nil {
			return nil, fmt.Errorf("query device metrics: %w", err)
		}
		return result, nil

	case "port":
		result, err := mq.QueryPortMetrics(ctx, req.Device, req.Port, req.Metrics, req.StartTime, req.EndTime)
		if err != nil {
			return nil, fmt.Errorf("query port metrics: %w", err)
		}
		return result, nil

	case "vpn":
		result, err := mq.QueryVPNTraffic(ctx, req.VPNID, req.Metrics, req.StartTime, req.EndTime)
		if err != nil {
			return nil, fmt.Errorf("query vpn traffic: %w", err)
		}
		return result, nil

	case "tunnel":
		result, err := mq.QueryTunnelTraffic(ctx, req.Device, req.Tunnel, req.Metrics, req.StartTime, req.EndTime)
		if err != nil {
			return nil, fmt.Errorf("query tunnel traffic: %w", err)
		}
		return result, nil

	case "alerts":
		alerts, err := mq.QueryAlerts(ctx, req.Namespace, req.Interval)
		if err != nil {
			return nil, fmt.Errorf("query alerts: %w", err)
		}
		return alerts, nil

	case "logs":
		logs, err := mq.QueryLogs(ctx, req.LogType, connector.LogQueryOptions{
			Interval:  req.Interval,
			StartTime: req.StartTime,
			EndTime:   req.EndTime,
		})
		if err != nil {
			return nil, fmt.Errorf("query logs: %w", err)
		}
		return logs, nil

	default:
		return nil, fmt.Errorf("unknown monitor query_type %q, expected device/port/vpn/tunnel/alerts/logs", req.QueryType)
	}
}

// ──────────────────────────────
// DeviceOperator 编排方法
// ──────────────────────────────

// DeviceInfoRequest 设备信息查询请求。
type DeviceInfoRequest struct {
	ConnectorName string `json:"connector_name"`
	QueryType     string `json:"query_type"` // "config" / "isis" / "bgp" / "vpn_config" / "route" / "topology" / "flexe" / "srv6" / "detnet"
	Device        string `json:"device,omitempty"`
}

// QueryDeviceInfo 查询指定 Connector 的设备信息。
func (s *DeviceService) QueryDeviceInfo(ctx context.Context, req DeviceInfoRequest) (any, error) {
	conn, err := s.registry.Get(req.ConnectorName)
	if err != nil {
		return nil, fmt.Errorf("get connector: %w", err)
	}

	op, ok := conn.(connector.DeviceOperator)
	if !ok {
		return nil, fmt.Errorf("connector %q does not support device operations (DeviceOperator not implemented)", req.ConnectorName)
	}

	switch req.QueryType {
	case "config":
		return op.QueryDeviceConfig(ctx, req.Device)
	case "isis":
		return op.QueryISISNeighbors(ctx, req.Device)
	case "bgp":
		return op.QueryBGPPeers(ctx, req.Device)
	case "vpn_config":
		return op.QueryVPNConfig(ctx, req.Device)
	case "route":
		return op.QueryGlobalRoute(ctx, req.Device)
	case "topology":
		return op.QueryTopologyLive(ctx)
	case "flexe":
		return op.ListFlexEGroups(ctx, connector.FilterOptions{DeviceName: req.Device})
	case "srv6":
		return op.ListSRv6Slices(ctx, connector.FilterOptions{DeviceName: req.Device})
	case "detnet":
		return op.ListDetNetInstances(ctx)
	default:
		return nil, fmt.Errorf("unknown device query_type %q", req.QueryType)
	}
}
