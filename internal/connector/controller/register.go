package controller

import (
	"crypto/tls"
	"net/http"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// Builder 返回 ControllerConnector 的工厂函数，供 ConnectorFactory 注册。
// 使用方式:
//
//	factory.RegisterBuilder("controller", controller.Builder())
func Builder() connector.ConnectorBuilder {
	return func(name string, cfg map[string]any, entityTypes []string) (connector.Connector, error) {
		baseURL, _ := cfg["base_url"].(string)
		timeout := 60 * time.Second
		if t, ok := cfg["timeout"].(string); ok {
			if d, err := time.ParseDuration(t); err == nil {
				timeout = d
			}
		}

		// Controller 使用自签名证书，跳过 TLS 验证
		insecureTransport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		httpClient := connector.NewHTTPClient(
			connector.WithBaseURL(baseURL),
			connector.WithTimeout(timeout),
			connector.WithRateLimit(10),
			connector.WithAuth(connector.AuthConfig{Type: "bearer"}),
			connector.WithTransport(insecureTransport),
		)

		// Step 1: 创建 ControllerClient（统一 API 适配层）
		client := NewControllerClient(name, httpClient, cfg)

		// Step 2: 创建 ControllerConnector（Collect 编排层）
		return NewControllerConnector(name, client, entityTypes, baseURL), nil
	}
}
