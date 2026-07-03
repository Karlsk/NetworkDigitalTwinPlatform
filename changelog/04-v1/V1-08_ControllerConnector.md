# V1-08: ControllerConnector (Device_Status + Telemetry)

**工时**: 1.5 天
**前置**: V1-04, V1-03
**风险等级**: 中
**Phase**: Phase 2a — 真实数据源 Connector

---

## 背景

网络控制器管理设备的动态运行状态（Device_Status）和遥测数据（Telemetry）。与静态资产 Connector 的关键区别：数据变化频繁，需配合增量同步使用。

---

## 实现内容

### 1. connector/controller/controller.go (新文件)

```go
// ControllerConnector 从网络控制器 REST API 采集动态状态数据。
type ControllerConnector struct {
    http    *connector.HTTPClient
    name    string
    types   []string
}

func NewControllerConnector(name string, client *connector.HTTPClient, entityTypes []string) *ControllerConnector

func (c *ControllerConnector) Metadata() connector.ConnectorMetadata
func (c *ControllerConnector) Ping(ctx context.Context) error
func (c *ControllerConnector) Collect(ctx context.Context, entityType string) ([]connector.Resource, error)
func (c *ControllerConnector) Stream(ctx context.Context, entityType string) (<-chan connector.Resource, error)
```

**API 路径映射**:

| 实体类型 | API 路径 | 说明 |
|---------|---------|------|
| `Device_Status` | `GET /api/v1/device-status/` | 设备运行状态（CPU/内存/接口状态） |
| `Telemetry` | `GET /api/v1/telemetry/` | 遥测指标数据（可选，V1 预留） |

**超时策略差异**: 控制器数据时效性要求高，默认超时应更短（15s），QPS 可更高（20）。

### 2. connector/controller/transform.go (新文件)

```go
func transformDeviceStatus(raw map[string]any) map[string]any
```

### 3. 注册到 ConnectorFactory

```go
f.RegisterBuilder("controller", func(name string, cfg map[string]any, entityTypes []string) (Connector, error) {
    baseURL, _ := cfg["base_url"].(string)
    client := connector.NewHTTPClient(
        connector.WithBaseURL(baseURL),
        connector.WithTimeout(15*time.Second),
        connector.WithRateLimit(20),
    )
    return controller.NewControllerConnector(name, client, entityTypes), nil
})
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/connector/controller/controller.go` | 新建 | ControllerConnector 主体 |
| `internal/connector/controller/transform.go` | 新建 | 字段展平逻辑 |
| `internal/connector/controller/controller_test.go` | 新建 | httptest 测试 |
| `internal/connector/factory.go` | 修改 | 注册 "controller" builder |

---

## 验收标准

- [x] 编译通过
- [x] `Collect("Device_Status")` 返回正确的动态状态 `[]Resource`
- [x] `Ping()` 验证控制器可达性
- [x] 超时策略更短（15s），QPS 更高（20）
- [x] 不支持的 entityType 返回清晰错误
- [x] `Stream()` 返回 `ErrNotImplemented`（V2 实现 Kafka 消费）
