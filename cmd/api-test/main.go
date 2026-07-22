// cmd/api-test 真实 Controller API 端到端测试工具。
// 用法: go run cmd/api-test/main.go
// 覆盖: V1.2-04 API（监控/SR-TE + MonitorQuerier/DeviceOperator 委托）
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/connector/controller"
)

const (
	baseURL      = "https://192.168.118.176:8711"
	username     = "zhangsan"
	password     = "tgb.258"
	desSecretKey = "9mng65v8jf4lxn93nabf981m"
	deviceID     = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
)

// ──────────────────────────────
// 测试结果跟踪
// ──────────────────────────────

type testResult struct {
	name    string
	status  string // "PASS", "FAIL", "SKIP"
	detail  string
	elapsed time.Duration
}

type testRunner struct {
	results []testResult
	start   time.Time
}

func newTestRunner() *testRunner {
	return &testRunner{start: time.Now()}
}

func (r *testRunner) run(name string, fn func() (string, error)) {
	t0 := time.Now()
	detail, err := fn()
	elapsed := time.Since(t0)

	status := "PASS"
	if err != nil {
		if strings.Contains(err.Error(), "not implemented") {
			status = "SKIP"
			detail = "ErrNotImplemented (预期行为)"
		} else {
			status = "FAIL"
			detail = err.Error()
		}
	}

	icon := "✓"
	if status == "FAIL" {
		icon = "✗"
	} else if status == "SKIP" {
		icon = "⊘"
	}

	mark := ""
	if status == "FAIL" {
		mark = fmt.Sprintf(" — %s", detail)
	} else if status == "SKIP" {
		mark = fmt.Sprintf(" — %s", detail)
	} else if detail != "" {
		mark = fmt.Sprintf(" — %s", detail)
	}

	fmt.Printf("  %s %-56s [%v]%s\n", icon, name, elapsed.Round(time.Millisecond), mark)
	r.results = append(r.results, testResult{name: name, status: status, detail: detail, elapsed: elapsed})
}

func (r *testRunner) section(title string) {
	fmt.Printf("\n━━━ %s ━━━\n", title)
}

func (r *testRunner) summary() {
	passed, failed, skipped := 0, 0, 0
	for _, res := range r.results {
		switch res.status {
		case "PASS":
			passed++
		case "FAIL":
			failed++
		case "SKIP":
			skipped++
		}
	}
	total := passed + failed + skipped

	fmt.Println("\n╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║                      测试汇总报告                        ║")
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Printf("║  总数:  %-48d║\n", total)
	fmt.Printf("║  通过:  %-48d║\n", passed)
	fmt.Printf("║  失败:  %-48d║\n", failed)
	fmt.Printf("║  跳过:  %-48d║\n", skipped)
	fmt.Printf("║  耗时:  %-48s║\n", time.Since(r.start).Round(time.Millisecond))
	fmt.Println("╚══════════════════════════════════════════════════════════╝")

	if failed > 0 {
		fmt.Println("\n── 失败详情 ──")
		for _, res := range r.results {
			if res.status == "FAIL" {
				fmt.Printf("  ✗ %s: %s\n", res.name, res.detail)
			}
		}
	}
}

// ──────────────────────────────
// 辅助函数
// ──────────────────────────────

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func countStr(items any) string {
	switch v := items.(type) {
	case []map[string]any:
		return fmt.Sprintf("%d items", len(v))
	case *connector.MetricsResult:
		return fmt.Sprintf("%d series", len(v.Metrics))
	case *connector.LogResult:
		return fmt.Sprintf("%d logs (total=%d)", len(v.Logs), v.TotalCount)
	case *connector.TopologyLiveResult:
		return fmt.Sprintf("%d nodes, %d links", len(v.Nodes), len(v.Links))
	case map[string]any:
		return fmt.Sprintf("%d fields", len(v))
	case string:
		return fmt.Sprintf("%d chars", len(v))
	default:
		return "OK"
	}
}

// ──────────────────────────────
// main
// ──────────────────────────────

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  Controller API — V1.2-04 全面端到端测试                ║")
	fmt.Printf("║  Target: %-48s║\n", baseURL)
	fmt.Println("╚══════════════════════════════════════════════════════════╝")

	r := newTestRunner()

	// ── 初始化 ──
	httpTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := connector.NewHTTPClient(
		connector.WithBaseURL(baseURL),
		connector.WithTimeout(60*time.Second),
		connector.WithRateLimit(10),
		connector.WithAuth(connector.AuthConfig{Type: "bearer"}),
		connector.WithTransport(httpTransport),
	)

	cfg := map[string]any{
		"base_url":       baseURL,
		"token_url":      "/oauth/token",
		"username":       username,
		"password":       password,
		"des_secret_key": desSecretKey,
		"device_id":      deviceID,
	}

	apiClient := controller.NewControllerClient("api-test", httpClient, cfg)
	conn := controller.NewControllerConnector(
		"api-test",
		apiClient,
		[]string{"Device", "Interface", "Link", "Alarm", "VPN", "Tunnel", "ISIS", "BGP"},
		baseURL,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// ── Section 1: 基础连接 ──
	r.section("1. 基础连接测试")

	r.run("Ping", func() (string, error) {
		err := conn.Ping(ctx)
		return "Token acquired", err
	})

	// ── Section 2: 原始 Collect 测试（8 种实体）──
	r.section("2. Collect 采集测试 (8 种实体)")

	entityTypes := []string{"Device", "Interface", "Link", "Alarm", "VPN", "Tunnel", "ISIS", "BGP"}
	for _, et := range entityTypes {
		etCopy := et
		r.run(fmt.Sprintf("Collect(%s)", etCopy), func() (string, error) {
			resources, err := conn.Collect(ctx, etCopy)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%d resources", len(resources)), nil
		})
	}

	// ── Section 3: 监控 API (ControllerClient 直接调用) ──
	r.section("3. 监控 API (ControllerClient)")

	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	testDevice := "" // 动态获取第一个设备名
	testPort := "GigabitEthernet0/0/0"
	testVPN := ""          // 动态获取第一个真实 VPN ID (svc-name)
	testTunnel := ""       // 动态获取第一个真实 Tunnel 名 (tunnel-name)
	testTunnelDevice := "" // 动态获取 Tunnel 所属设备名 (src_device)

	// 先获取一个真实设备名
	r.run("获取测试设备名", func() (string, error) {
		resources, err := conn.Collect(ctx, "Device")
		if err != nil {
			return "", err
		}
		if len(resources) > 0 {
			if name, ok := resources[0].Properties["name"].(string); ok {
				testDevice = name
			}
		}
		if testDevice == "" {
			testDevice = "NJ-SCT-R01" // fallback
		}
		return fmt.Sprintf("device=%s", testDevice), nil
	})

	// 获取真实 VPN ID（监控 API 使用 svc-name 作为 vpnId 维度值）
	r.run("获取测试VPN ID", func() (string, error) {
		resources, err := conn.Collect(ctx, "VPN")
		if err != nil {
			return "", err
		}
		if len(resources) > 0 {
			// 优先使用 svc-name（监控维度值），回退到 vpn-id
			if name, ok := resources[0].Properties["name"].(string); ok && name != "" {
				testVPN = name
			} else {
				testVPN = resources[0].ID
			}
		}
		if testVPN == "" {
			return "no VPN resources found", nil
		}
		return fmt.Sprintf("vpnId=%s (id=%s)", testVPN, resources[0].ID), nil
	})

	// 获取真实 Tunnel 名（监控 API 使用 tunnel-name 而非 instance-id）
	r.run("获取测试Tunnel名", func() (string, error) {
		resources, err := conn.Collect(ctx, "Tunnel")
		if err != nil {
			return "", err
		}
		if len(resources) > 0 {
			// 优先使用 tunnel_name（来自 te-tuples 嵌套结构），回退到 policy-template-name
			if tn, ok := resources[0].Properties["tunnel_name"].(string); ok && tn != "" {
				testTunnel = tn
			} else if name, ok := resources[0].Properties["name"].(string); ok && name != "" {
				testTunnel = name
			} else {
				testTunnel = resources[0].ID
			}
		}
		if testTunnel == "" {
			return "no Tunnel resources found", nil
		}
		// 同时获取 Tunnel 所属设备名
		if sd, ok := resources[0].Properties["src_device"].(string); ok && sd != "" {
			testTunnelDevice = sd
		}
		return fmt.Sprintf("tunnelName=%s device=%s (id=%s)", testTunnel, testTunnelDevice, resources[0].ID), nil
	})

	testMonitorAPIs(r, apiClient, ctx, testDevice, testPort, testVPN, testTunnel, testTunnelDevice, hourAgo, now)

	// ── Section 4: SR-TE API ──
	r.section("4. SR-TE API (ControllerClient)")
	testSRTEAPIs(r, apiClient, ctx)

	// ── Section 5: MonitorQuerier 委托调用 ──
	r.section("5. MonitorQuerier 委托调用 (ControllerConnector)")
	testMonitorQuerier(r, conn, ctx, testDevice, testPort, testVPN, testTunnel, testTunnelDevice, hourAgo, now)

	// ── Section 6: DeviceOperator 委托调用 ──
	r.section("6. DeviceOperator 委托调用 (ControllerConnector)")
	testDeviceOperator(r, conn, ctx, testDevice)

	// ── 汇总 ──
	r.summary()
}

// ══════════════════════════════════════════════════════════
// 3. 监控 API 测试
// ══════════════════════════════════════════════════════════

func testMonitorAPIs(r *testRunner, c *controller.ControllerClient, ctx context.Context,
	device, port, vpnID, tunnel, tunnelDevice string, start, end time.Time) {

	// 3.1 FetchDeviceMetrics
	r.run("FetchDeviceMetrics(cpu_usage)", func() (string, error) {
		result, err := c.FetchDeviceMetrics(ctx, device, []string{"cpu_usage"}, start, end)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	// 3.2 FetchDeviceMetrics 多指标
	r.run("FetchDeviceMetrics(cpu,mem,disk)", func() (string, error) {
		result, err := c.FetchDeviceMetrics(ctx, device, []string{"cpu_usage", "memory_usage", "disk_usage"}, start, end)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	// 3.3 FetchPortMetrics
	r.run("FetchPortMetrics(in_traffic,out_traffic)", func() (string, error) {
		result, err := c.FetchPortMetrics(ctx, device, port, []string{"in_traffic", "out_traffic"}, start, end)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	// 3.4 FetchVPNTraffic（使用真实 VPN ID）
	r.run("FetchVPNTraffic", func() (string, error) {
		if vpnID == "" {
			return "skip: no VPN resources", nil
		}
		result, err := c.FetchVPNTraffic(ctx, vpnID, []string{"in_bytes", "out_bytes"}, start, end)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	// 3.5 FetchTunnelTraffic（使用真实 Tunnel 名 + Tunnel 所属设备）
	r.run("FetchTunnelTraffic", func() (string, error) {
		if tunnel == "" {
			return "skip: no Tunnel resources", nil
		}
		tDev := tunnelDevice
		if tDev == "" {
			tDev = device // fallback 到通用设备名
		}
		result, err := c.FetchTunnelTraffic(ctx, tDev, tunnel, []string{"in_bytes"}, start, end)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	// 3.6 FetchSystemLogs
	r.run("FetchSystemLogs(interval=1h)", func() (string, error) {
		result, err := c.FetchSystemLogs(ctx, connector.LogQueryOptions{
			Interval: "1h", PageNum: 1, PageSize: 10,
		})
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	// 3.7 FetchSystemLogs 时间范围
	r.run("FetchSystemLogs(startTime/endTime)", func() (string, error) {
		result, err := c.FetchSystemLogs(ctx, connector.LogQueryOptions{
			StartTime: start, EndTime: end, PageNum: 1, PageSize: 5,
		})
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	// 3.8 FetchLoginLogs
	r.run("FetchLoginLogs(interval=1h)", func() (string, error) {
		result, err := c.FetchLoginLogs(ctx, connector.LogQueryOptions{
			Interval: "1h", PageNum: 1, PageSize: 10,
		})
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	// 3.9 FetchLogs(system)
	r.run("FetchLogs(system)", func() (string, error) {
		result, err := c.FetchLogs(ctx, "system", connector.LogQueryOptions{
			Interval: "1h", PageNum: 1, PageSize: 5,
		})
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	// 3.10 FetchLogs(login)
	r.run("FetchLogs(login)", func() (string, error) {
		result, err := c.FetchLogs(ctx, "login", connector.LogQueryOptions{
			Interval: "1h", PageNum: 1, PageSize: 5,
		})
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	// 3.11 FetchTopology
	r.run("FetchTopology", func() (string, error) {
		result, err := c.FetchTopology(ctx)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	// 3.12 FetchTopologyNodes
	r.run("FetchTopologyNodes", func() (string, error) {
		nodes, err := c.FetchTopologyNodes(ctx)
		if err != nil {
			return "", err
		}
		return countStr(nodes), nil
	})

	// 3.13 FetchTopologyLinks
	r.run("FetchTopologyLinks", func() (string, error) {
		links, err := c.FetchTopologyLinks(ctx)
		if err != nil {
			return "", err
		}
		return countStr(links), nil
	})

	// 3.14 FetchLinkMetrics
	r.run("FetchLinkMetrics", func() (string, error) {
		metrics, err := c.FetchLinkMetrics(ctx)
		if err != nil {
			return "", err
		}
		return countStr(metrics), nil
	})

	// 3.15 FetchL2Links
	r.run("FetchL2Links(topology=default)", func() (string, error) {
		links, err := c.FetchL2Links(ctx, "default")
		if err != nil {
			return "", err
		}
		return countStr(links), nil
	})
}

// ══════════════════════════════════════════════════════════
// 4. SR-TE API 测试
// ══════════════════════════════════════════════════════════

func testSRTEAPIs(r *testRunner, c *controller.ControllerClient, ctx context.Context) {
	r.run("FetchSRTEPathDetail(test-id)", func() (string, error) {
		result, err := c.FetchSRTEPathDetail(ctx, "test-policy-instance")
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	r.run("ComputeSRTEPath(ErrNotImplemented)", func() (string, error) {
		_, err := c.ComputeSRTEPath(ctx, map[string]any{"src": "A", "dst": "B"})
		if err == nil {
			return "", fmt.Errorf("expected ErrNotImplemented but got nil")
		}
		if !strings.Contains(err.Error(), "not implemented") {
			return "", fmt.Errorf("expected ErrNotImplemented, got: %w", err)
		}
		return "ErrNotImplemented (预期)", nil
	})

	r.run("CreateSRTEPolicy(ErrNotImplemented)", func() (string, error) {
		_, err := c.CreateSRTEPolicy(ctx, map[string]any{"name": "test"})
		if err == nil {
			return "", fmt.Errorf("expected ErrNotImplemented but got nil")
		}
		if !strings.Contains(err.Error(), "not implemented") {
			return "", fmt.Errorf("expected ErrNotImplemented, got: %w", err)
		}
		return "ErrNotImplemented (预期)", nil
	})
}

// ══════════════════════════════════════════════════════════
// 5. MonitorQuerier 委托调用测试
// ══════════════════════════════════════════════════════════

func testMonitorQuerier(r *testRunner, conn *controller.ControllerConnector, ctx context.Context,
	device, port, vpnID, tunnel, tunnelDevice string, start, end time.Time) {

	r.run("QueryDeviceMetrics(cpu_usage)", func() (string, error) {
		result, err := conn.QueryDeviceMetrics(ctx, device, []string{"cpu_usage"}, start, end)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	r.run("QueryPortMetrics(in_traffic)", func() (string, error) {
		result, err := conn.QueryPortMetrics(ctx, device, port, []string{"in_traffic"}, start, end)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	r.run("QueryVPNTraffic", func() (string, error) {
		if vpnID == "" {
			return "skip: no VPN resources", nil
		}
		result, err := conn.QueryVPNTraffic(ctx, vpnID, []string{"in_bytes"}, start, end)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	r.run("QueryTunnelTraffic", func() (string, error) {
		if tunnel == "" {
			return "skip: no Tunnel resources", nil
		}
		tDev := tunnelDevice
		if tDev == "" {
			tDev = device // fallback
		}
		result, err := conn.QueryTunnelTraffic(ctx, tDev, tunnel, []string{"in_bytes"}, start, end)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	r.run("QueryAlerts", func() (string, error) {
		alerts, err := conn.QueryAlerts(ctx, "system", "1h")
		if err != nil {
			return "", err
		}
		return countStr(alerts), nil
	})

	r.run("QueryLogs(system)", func() (string, error) {
		result, err := conn.QueryLogs(ctx, "system", connector.LogQueryOptions{
			Interval: "1h", PageNum: 1, PageSize: 5,
		})
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})
}

// ══════════════════════════════════════════════════════════
// 6. DeviceOperator 委托调用测试
// ══════════════════════════════════════════════════════════

func testDeviceOperator(r *testRunner, conn *controller.ControllerConnector, ctx context.Context, device string) {
	r.run("QueryDeviceConfig", func() (string, error) {
		result, err := conn.QueryDeviceConfig(ctx, device)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	r.run("QueryISISNeighbors", func() (string, error) {
		neighbors, err := conn.QueryISISNeighbors(ctx, device)
		if err != nil {
			return "", err
		}
		return countStr(neighbors), nil
	})

	r.run("QueryBGPPeers", func() (string, error) {
		peers, err := conn.QueryBGPPeers(ctx, device)
		if err != nil {
			return "", err
		}
		return countStr(peers), nil
	})

	r.run("QueryVPNConfig", func() (string, error) {
		result, err := conn.QueryVPNConfig(ctx, device)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})

	r.run("QueryGlobalRoute", func() (string, error) {
		routes, err := conn.QueryGlobalRoute(ctx, device)
		if err != nil {
			return "", err
		}
		return countStr(routes), nil
	})

	r.run("QueryTopologyLive(DeviceOperator)", func() (string, error) {
		result, err := conn.QueryTopologyLive(ctx)
		if err != nil {
			return "", err
		}
		return countStr(result), nil
	})
}

// ──────────────────────────────
// 工具函数（保留原有功能）
// ──────────────────────────────

func printProps(props map[string]any) {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	for _, k := range keys {
		v := props[k]
		s := formatValue(v)
		if len(s) > 80 {
			s = s[:77] + "..."
		}
		fmt.Printf("       %-22s= %s\n", k, s)
	}
}

func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%v", val)
	case nil:
		return "<nil>"
	default:
		b, _ := json.Marshal(v)
		s := string(b)
		s = strings.ReplaceAll(s, "\n", " ")
		return s
	}
}

// 抑制未使用警告
var _ = truncate
var _ = printProps

func init() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime)
}
