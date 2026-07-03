# V1-06: NetboxConnector (Device + Interface)

**工时**: 2 天
**前置**: V1-04, V1-03
**风险等级**: 中
**Phase**: Phase 2a — 真实数据源 Connector

---

## 背景

Netbox 是网络资产管理的事实标准工具，提供 REST API 管理设备（Device）和接口（Interface）数据。本任务实现对接 Netbox REST API 的 Connector。

---

## 实现内容

### 1. connector/netbox/netbox.go (新文件)

```go
// NetboxConnector 从 Netbox REST API 采集 Device 和 Interface 数据。
type NetboxConnector struct {
    http    *connector.HTTPClient
    name    string
    types   []string
}

func NewNetboxConnector(name string, client *connector.HTTPClient, entityTypes []string) *NetboxConnector

func (c *NetboxConnector) Metadata() connector.ConnectorMetadata {
    return connector.ConnectorMetadata{
        Name:        c.name,
        Type:        "netbox",
        EntityTypes: c.types,
    }
}

func (c *NetboxConnector) Ping(ctx context.Context) error {
    // GET /api/status/ 验证 Netbox 可达性
    req, _ := http.NewRequestWithContext(ctx, "GET", c.http.BaseURL()+"/api/status/", nil)
    resp, err := c.http.Do(ctx, req)
    if err != nil {
        return fmt.Errorf("netbox ping: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("netbox ping: status %d", resp.StatusCode)
    }
    return nil
}

func (c *NetboxConnector) Collect(ctx context.Context, entityType string) ([]connector.Resource, error) {
    switch entityType {
    case "Device":
        return c.collectDevices(ctx)
    case "Interface":
        return c.collectInterfaces(ctx)
    default:
        return nil, fmt.Errorf("netbox connector: unsupported entity type %q", entityType)
    }
}

func (c *NetboxConnector) Stream(ctx context.Context, entityType string) (<-chan connector.Resource, error) {
    return nil, connector.ErrNotImplemented
}
```

**Collect 实现细节**:

```go
func (c *NetboxConnector) collectDevices(ctx context.Context) ([]connector.Resource, error) {
    var resources []connector.Resource
    err := c.http.Paginate(ctx, "/api/dcim/devices/", 100, func(page []map[string]any) error {
        for _, raw := range page {
            props := transformDevice(raw)
            id := fmt.Sprintf("%v", raw["id"])
            resources = append(resources, connector.Resource{
                Kind:       "Device",
                ID:         id,
                Properties: props,
            })
        }
        return nil
    })
    return resources, err
}
```

### 2. connector/netbox/transform.go (新文件)

字段展平逻辑：

```go
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
    // 展平嵌套字段
    if dt, ok := raw["device_type"].(map[string]any); ok {
        props["device_type"] = dt["slug"]
    }
    if site, ok := raw["site"].(map[string]any); ok {
        props["site"] = site["name"]
    }
    return props
}

// transformInterface 展平 Netbox Interface API 响应。
func transformInterface(raw map[string]any) map[string]any {
    props := make(map[string]any)
    for _, key := range []string{"name", "type", "enabled", "mtu", "mac_address", "description"} {
        if v, ok := raw[key]; ok {
            props[key] = v
        }
    }
    if device, ok := raw["device"].(map[string]any); ok {
        props["device_name"] = device["name"]
    }
    return props
}
```

### 3. 注册到 ConnectorFactory

在 `connector/factory.go` 中注册 `"netbox"` builder：

```go
f.RegisterBuilder("netbox", func(name string, cfg map[string]any, entityTypes []string) (Connector, error) {
    baseURL, _ := cfg["base_url"].(string)
    timeout := 30 * time.Second
    if t, ok := cfg["timeout"].(string); ok {
        timeout, _ = time.ParseDuration(t)
    }
    client := connector.NewHTTPClient(
        connector.WithBaseURL(baseURL),
        connector.WithTimeout(timeout),
        // auth 由 ConnectorFactory 统一注入
    )
    return netbox.NewNetboxConnector(name, client, entityTypes), nil
})
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/connector/netbox/netbox.go` | 新建 | NetboxConnector 主体 |
| `internal/connector/netbox/transform.go` | 新建 | 字段展平逻辑 |
| `internal/connector/netbox/netbox_test.go` | 新建 | httptest 测试 |
| `internal/connector/factory.go` | 修改 | 注册 "netbox" builder |
| `configs/connectors.yaml` | 修改 | 新增 netbox 配置段（注释状态） |

---

## 验收标准

- [x] 编译通过
- [x] `Collect("Device")` 返回正确的 `[]Resource`，字段与 Schema `properties` 对应
- [x] `Collect("Interface")` 同理
- [x] `Ping()` 对不可达 URL 返回带上下文的 error
- [x] 分页正确遍历所有页
- [x] 超时/重试正常工作
- [x] `Collect` 对不支持的 entityType 返回清晰错误
- [x] 在 ConnectorFactory 中正确注册 "netbox" builder
