package netbox

import (
	"time"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// Builder 返回 NetboxConnector 的工厂函数，供 ConnectorFactory 注册。
// 使用方式:
//
//	factory.RegisterBuilder("netbox", netbox.Builder())
func Builder() connector.ConnectorBuilder {
	return func(name string, cfg map[string]any, entityTypes []string) (connector.Connector, error) {
		baseURL, _ := cfg["base_url"].(string)
		timeout := 30 * time.Second
		if t, ok := cfg["timeout"].(string); ok {
			timeout, _ = time.ParseDuration(t)
		}
		client := connector.NewHTTPClient(
			connector.WithBaseURL(baseURL),
			connector.WithTimeout(timeout),
			connector.WithRateLimit(10),
		)
		return NewNetboxConnector(name, client, entityTypes), nil
	}
}
