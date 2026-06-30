// Package netbox 实现对接 Netbox REST API 的 Connector。
// transform.go 负责将 Netbox 嵌套 JSON 响应展平为 Resource.Properties。
package netbox

// transformDevice 将 Netbox Device API 响应展平为 Resource.Properties。
// Netbox 返回嵌套结构: {"device_type": {"slug": "xxx"}, "site": {"name": "xxx"}}
// 展平为: {"device_type": "xxx", "site": "xxx"}
func transformDevice(raw map[string]any) map[string]any {
	props := make(map[string]any)

	// 直接字段
	for _, key := range []string{"serial", "name", "status", "platform", "role"} {
		if v, ok := raw[key]; ok {
			props[key] = v
		}
	}

	// 展平嵌套字段: device_type.slug
	if dt, ok := raw["device_type"].(map[string]any); ok {
		props["device_type"] = dt["slug"]
	}

	// 展平嵌套字段: site.name
	if site, ok := raw["site"].(map[string]any); ok {
		props["site"] = site["name"]
	}

	return props
}

// transformInterface 展平 Netbox Interface API 响应。
// 展平嵌套字段: device.name → device_name
func transformInterface(raw map[string]any) map[string]any {
	props := make(map[string]any)

	// 直接字段
	for _, key := range []string{"name", "type", "enabled", "mtu", "mac_address", "description"} {
		if v, ok := raw[key]; ok {
			props[key] = v
		}
	}

	// 展平嵌套字段: device.name → device_name
	if device, ok := raw["device"].(map[string]any); ok {
		props["device_name"] = device["name"]
	}

	return props
}
