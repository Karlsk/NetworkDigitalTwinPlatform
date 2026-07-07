# V1-02: Connector 接口增强 + Ping

**工时**: 0.5 天
**前置**: 无
**风险等级**: 低
**Phase**: Phase 1 — 基础迁移

---

## 背景

当前 `Connector` 接口只有 `Metadata()/Collect()/Stream()` 三个方法，缺少健康检查能力。V1 真实 REST Connector 需要 `Ping()` 验证数据源连通性。

---

## 实现内容

### 1. connector/interface.go — Connector 接口新增 Ping

```go
type Connector interface {
    Metadata() ConnectorMetadata
    Collect(ctx context.Context, entityType string) ([]Resource, error)
    Stream(ctx context.Context, entityType string) (<-chan Resource, error)
    Ping(ctx context.Context) error  // 新增: 健康检查
}
```

### 2. connector/types.go — ConnectorMetadata 扩展

```go
type ConnectorMetadata struct {
    Name        string
    Type        string         // "mock" / "netbox" / "cmdb" / "controller"
    EntityTypes []string
    BaseURL     string         // REST API 基地址（mock 可留空）
    Timeout     time.Duration  // 请求超时（mock 可留空）
    AuthType    string         // "none" / "basic" / "token"
}
```

### 3. connector/mock/mock.go — MockConnector 实现 Ping

```go
func (c *MockConnector) Ping(ctx context.Context) error {
    // Mock 始终返回 nil（数据目录存在性已在构造时验证）
    return nil
}
```

### 4. service/testhelper_test.go — 测试 mock 适配

`testhelper_test.go` 中如果有 mockConnector 实现，也需新增 `Ping()` 方法。

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/connector/interface.go` | 修改 | Connector 接口新增 Ping() |
| `internal/connector/types.go` | 修改 | ConnectorMetadata 新增 3 个字段 |
| `internal/connector/interface_test.go` | 修改 | 接口测试适配 |
| `internal/connector/mock/mock.go` | 修改 | 实现 Ping() 返回 nil |
| `internal/connector/mock/mock_test.go` | 修改 | Ping 测试 |
| `internal/service/testhelper_test.go` | 修改 | 测试 mock 适配 |

---

## 验收标准

- [x] 编译通过，所有现有 Connector 实现均满足新接口
- [x] `MockConnector.Ping()` 返回 nil
- [x] `ConnectorMetadata` 新字段 `Type`/`BaseURL`/`Timeout`/`AuthType` 可正确赋值
- [x] `go test ./internal/connector/...` 全部通过
