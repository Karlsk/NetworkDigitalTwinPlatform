# V1-07: CMDBConnector (ISIS + Link + Network_Slice)

**工时**: 1.5 天
**前置**: V1-04, V1-03
**风险等级**: 中
**Phase**: Phase 2a — 真实数据源 Connector

---

## 背景

CMDB（配置管理数据库）管理网络协议和服务配置数据。本任务实现对接 CMDB REST API 采集 ISIS 实例、链路（Link）和网络切片（Network_Slice）数据。

---

## 实现内容

### 1. connector/cmdb/cmdb.go (新文件)

```go
// CMDBConnector 从 CMDB REST API 采集 ISIS、Link、Network_Slice 数据。
type CMDBConnector struct {
    http    *connector.HTTPClient
    name    string
    types   []string
}

func NewCMDBConnector(name string, client *connector.HTTPClient, entityTypes []string) *CMDBConnector

func (c *CMDBConnector) Metadata() connector.ConnectorMetadata
func (c *CMDBConnector) Ping(ctx context.Context) error
func (c *CMDBConnector) Collect(ctx context.Context, entityType string) ([]connector.Resource, error)
func (c *CMDBConnector) Stream(ctx context.Context, entityType string) (<-chan connector.Resource, error)
```

**API 路径映射**:

| 实体类型 | API 路径 | 说明 |
|---------|---------|------|
| `ISIS` | `GET /api/v1/isis-instances/` | ISIS 路由协议实例 |
| `Link` | `GET /api/v1/links/` | 物理/逻辑链路 |
| `Network_Slice` | `GET /api/v1/network-slices/` | 网络切片配置 |

### 2. connector/cmdb/transform.go (新文件)

```go
func transformISIS(raw map[string]any) map[string]any
func transformLink(raw map[string]any) map[string]any
func transformNetworkSlice(raw map[string]any) map[string]any
```

**注意**: CMDB 响应格式可能与 Netbox 不同（分页结构、字段嵌套方式），需适配不同的响应解析逻辑。

### 3. 注册到 ConnectorFactory

```go
f.RegisterBuilder("cmdb", func(name string, cfg map[string]any, entityTypes []string) (Connector, error) {
    baseURL, _ := cfg["base_url"].(string)
    client := connector.NewHTTPClient(
        connector.WithBaseURL(baseURL),
    )
    return cmdb.NewCMDBConnector(name, client, entityTypes), nil
})
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/connector/cmdb/cmdb.go` | 新建 | CMDBConnector 主体 |
| `internal/connector/cmdb/transform.go` | 新建 | 字段展平逻辑 |
| `internal/connector/cmdb/cmdb_test.go` | 新建 | httptest 测试 |
| `internal/connector/factory.go` | 修改 | 注册 "cmdb" builder |

---

## 验收标准

- [x] 编译通过
- [x] `Collect("ISIS")` 返回正确的 `[]Resource`
- [x] `Collect("Link")` 返回正确的 `[]Resource`
- [x] `Collect("Network_Slice")` 返回正确的 `[]Resource`
- [x] 三种实体类型均正确采集和转换
- [x] 与 NetboxConnector 共享 HTTPClient 基础设施（认证、重试、限流）
- [x] `Ping()` 验证 CMDB 可达性
- [x] 不支持的 entityType 返回清晰错误
