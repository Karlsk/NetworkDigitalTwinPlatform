# V1-04: HTTP 客户端公共层

**工时**: 1 天
**前置**: V1-02
**风险等级**: 中
**Phase**: Phase 1 — 基础迁移

---

## 背景

真实 REST Connector（Netbox/CMDB/Controller）共用认证、重试、限流、超时、分页逻辑。需要抽取为公共 HTTP 客户端层，避免在每个 Connector 中重复实现。

---

## 实现内容

### 1. connector/httpclient.go (新文件)

```go
// HTTPClient 封装认证、重试、限流、超时、分页的 HTTP 客户端。
type HTTPClient struct {
    client  *http.Client
    baseURL string
    auth    AuthConfig
    limiter *rate.Limiter
}

// HTTPOption 函数式配置。
type HTTPOption func(*HTTPClient)

func WithBaseURL(url string) HTTPOption
func WithAuth(auth AuthConfig) HTTPOption
func WithTimeout(d time.Duration) HTTPOption
func WithRateLimit(qps float64) HTTPOption

func NewHTTPClient(opts ...HTTPOption) *HTTPClient
```

#### 认证中间件

```go
// 注入到每个请求的 Header。
func (c *HTTPClient) applyAuth(req *http.Request) {
    switch c.auth.Type {
    case "token":
        token := os.Getenv(c.auth.TokenEnv)
        req.Header.Set("Authorization", "Token "+token)
    case "basic":
        password := os.Getenv(c.auth.PasswordEnv)
        req.SetBasicAuth(c.auth.Username, password)
    }
}
```

#### 重试策略

```go
// Do 执行 HTTP 请求，支持指数退避重试。
// 重试条件: 5xx 状态码 + 网络错误
// 不重试: 4xx 状态码（客户端错误）
// 策略: 500ms / 1s / 2s / 4s，最多 3 次
func (c *HTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error)
```

#### 限流

```go
// 每次请求前调用 limiter.Wait(ctx)，阻塞直到令牌可用。
// 默认 QPS: 10（可通过 WithRateLimit 配置）
```

#### 分页

```go
// PageResult 分页响应。
type PageResult struct {
    Results []map[string]any // 当前页数据
    Next    string           // 下一页 URL（空表示最后一页）
    Count   int              // 总记录数
}

// Paginate 自动遍历所有页并回调。
func (c *HTTPClient) Paginate(ctx context.Context, path string, pageSize int,
    callback func(page []map[string]any) error) error
```

### 2. 新增依赖

```
golang.org/x/time/rate  // 限流器
```

运行 `go get golang.org/x/time/rate`。

### 3. connector/httpclient_test.go (新文件)

使用 `net/http/httptest` 测试:

- TC-H01: Token Auth Header 正确注入
- TC-H02: Basic Auth Header 正确注入
- TC-H03: 5xx 触发重试（验证请求次数 = 重试次数 + 1）
- TC-H04: 4xx 不触发重试
- TC-H05: 限流器正确控制 QPS（burst 测试）
- TC-H06: 分页遍历所有页（单页 + 多页 + 空结果）
- TC-H07: 超时正确返回 `context.DeadlineExceeded`

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/connector/httpclient.go` | 新建 | HTTPClient + Auth + Retry + RateLimit + Paginate |
| `internal/connector/httpclient_test.go` | 新建 | httptest 测试 |
| `go.mod` | 修改 | 新增 `golang.org/x/time` |
| `go.sum` | 修改 | 新增依赖校验 |

---

## 验收标准

- [x] 编译通过
- [x] Token Auth 正确注入 `Authorization: Token xxx` Header
- [x] Basic Auth 正确注入 `Authorization: Basic xxx` Header
- [x] 5xx 错误触发指数退避重试，4xx 不重试
- [x] 限流器正确控制 QPS，默认 10 QPS
- [x] `Paginate()` 能遍历所有页并回调，处理空结果/单页/多页
- [x] 超时正确返回 context deadline exceeded
- [x] `go test ./internal/connector/...` 全部通过
