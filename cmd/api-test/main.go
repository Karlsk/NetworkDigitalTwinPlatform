// cmd/api-test 真实 Controller API 端到端测试工具。
// 用法: go run cmd/api-test/main.go
// 验证: 获取 → 解析 → Transform → 打印全流程
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

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  Controller Connector — 真实 API 端到端测试             ║")
	fmt.Printf("║  Target: %-48s║\n", baseURL)
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()

	cfg := map[string]any{
		"base_url":       baseURL,
		"token_url":      "/oauth/token",
		"username":       username,
		"password":       password,
		"des_secret_key": desSecretKey,
		"device_id":      deviceID,
	}

	// 创建跳过 TLS 验证的 HTTPClient
	httpTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := connector.NewHTTPClient(
		connector.WithBaseURL(baseURL),
		connector.WithTimeout(60*time.Second),
		connector.WithRateLimit(10),
		connector.WithAuth(connector.AuthConfig{Type: "bearer"}),
		connector.WithTransport(httpTransport),
	)

	conn := controller.NewControllerConnector(
		"controller-api-test",
		client,
		[]string{"Device", "Interface", "Link", "Alarm", "VPN", "Tunnel", "ISIS", "BGP"},
		cfg,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Ping 测试
	fmt.Println("▶ Ping...")
	if err := conn.Ping(ctx); err != nil {
		log.Fatalf("✗ Ping failed: %v", err)
	}
	fmt.Println("✓ Ping OK — Token acquired")
	fmt.Println()

	// 逐个测试 8 种实体
	entityTypes := []string{"Device", "Interface", "Link", "Alarm", "VPN", "Tunnel", "ISIS", "BGP"}

	for _, et := range entityTypes {
		fmt.Printf("━━━ %s ━━━\n", et)
		resources, err := conn.Collect(ctx, et)
		if err != nil {
			fmt.Printf("✗ Error: %v\n\n", err)
			continue
		}

		fmt.Printf("✓ Collected: %d resources\n", len(resources))

		// 打印前 3 个资源的详情
		limit := 3
		if len(resources) < limit {
			limit = len(resources)
		}
		for i := 0; i < limit; i++ {
			r := resources[i]
			fmt.Printf("  [%d] Kind=%s, ID=%s\n", i, r.Kind, r.ID)
			printProps(r.Properties)
		}
		if len(resources) > 3 {
			fmt.Printf("  ... and %d more\n", len(resources)-3)
		}
		fmt.Println()
	}

	// 汇总
	fmt.Println("━━━ 汇总 ━━━")
	for _, et := range entityTypes {
		resources, err := conn.Collect(ctx, et)
		if err != nil {
			fmt.Printf("  %-12s: ERROR (%v)\n", et, err)
		} else {
			fmt.Printf("  %-12s: %d\n", et, len(resources))
		}
	}
}

func printProps(props map[string]any) {
	// 按 key 排序打印
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	// 简单排序
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

func init() {
	// 设置 stderr 日志级别（过滤 slog 日志输出）
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime)
}
