# V1-09: Connector 集成测试 (httptest mock server)

**工时**: 1.5 天
**前置**: V1-06, V1-07, V1-08
**风险等级**: 低
**Phase**: Phase 2a — 真实数据源 Connector

---

## 背景

三个真实 REST Connector 均已实现，需使用 `net/http/httptest` 创建 mock REST API server 测试完整的 Collect 流程，覆盖正常路径、错误路径和边界条件。

---

## 实现内容

### 1. 测试架构

每个 Connector 包下创建 `_test.go`，内部启动 `httptest.NewServer` 模拟真实 API 响应。

```go
func setupMockNetboxServer() *httptest.Server {
    mux := http.NewServeMux()

    // GET /api/status/ → 200 OK
    mux.HandleFunc("/api/status/", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]any{"netbox-version": "3.6"})
    })

    // GET /api/dcim/devices/ → 分页响应
    mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
        page := r.URL.Query().Get("offset")
        // 模拟第一页 + 第二页
        ...
    })

    return httptest.NewServer(mux)
}
```

### 2. 测试用例

#### NetboxConnector (netbox_test.go)

| 测试ID | 场景 | 预期 |
|--------|------|------|
| TC-C10 | `Collect("Device")` 正常分页 | 返回正确数量的 `[]Resource`，字段展平正确 |
| TC-C11 | `Ping()` 成功 | 返回 nil |
| TC-C11b | `Ping()` 服务器不可达 | 返回带上下文的 error |
| TC-C12 | `Collect("Interface")` | 返回正确的 Interface 资源 |

#### CMDBConnector (cmdb_test.go)

| 测试ID | 场景 | 预期 |
|--------|------|------|
| TC-C13 | `Collect("ISIS")` / `Collect("Link")` / `Collect("Network_Slice")` | 三种类型均正确 |

#### ControllerConnector (controller_test.go)

| 测试ID | 场景 | 预期 |
|--------|------|------|
| TC-C14 | `Collect("Device_Status")` | 返回正确的动态状态资源 |

#### 通用错误路径（每个 Connector 均需覆盖）

| 测试ID | 场景 | 预期 |
|--------|------|------|
| TC-C15 | 401/403 认证错误 | 返回 `fmt.Errorf("...: status 401")` |
| TC-C16 | 500 服务端错误 | 触发重试后仍失败，返回明确错误 |
| TC-C17 | 超时 | 返回 `context.DeadlineExceeded` 包装的错误 |

#### ConnectorFactory (factory_test.go)

| 测试ID | 场景 | 预期 |
|--------|------|------|
| TC-C18 | 从配置创建各类型 Connector | Netbox/CMDB/Controller 均可正确创建 |

### 3. 边界条件

- 0 结果（空 `results[]`）
- 单页结果（无 `next` URL）
- 多页结果（有 `next` URL，需翻页）
- API 返回非预期 JSON 结构（字段缺失/类型错误）

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/connector/netbox/netbox_test.go` | 新建/修改 | NetboxConnector httptest 测试 |
| `internal/connector/cmdb/cmdb_test.go` | 新建/修改 | CMDBConnector httptest 测试 |
| `internal/connector/controller/controller_test.go` | 新建/修改 | ControllerConnector httptest 测试 |
| `internal/connector/factory_test.go` | 修改 | Factory 创建各类型 Connector 测试 |

---

## 验收标准

- [x] 全部测试用例通过
- [x] 覆盖正常路径 + 错误路径 + 边界条件
- [x] 分页正确遍历所有页（单页、多页、空结果）
- [x] 认证错误返回清晰错误信息
- [x] 500 错误触发重试
- [x] 超时正确处理
- [x] `go test -race ./internal/connector/...` 全部通过
