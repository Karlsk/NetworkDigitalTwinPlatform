package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"time"

	"golang.org/x/time/rate"
)

// HTTPClient 封装认证、重试、限流、超时、分页的 HTTP 客户端。
// 供 Netbox/CMDB/Controller 等 REST Connector 复用。
type HTTPClient struct {
	client  *http.Client
	baseURL string
	auth    AuthConfig
	limiter *rate.Limiter
}

// HTTPOption 函数式配置选项。
type HTTPOption func(*HTTPClient)

// WithBaseURL 设置 API 基地址。
func WithBaseURL(baseURL string) HTTPOption {
	return func(c *HTTPClient) {
		c.baseURL = baseURL
	}
}

// WithAuth 设置认证配置。
func WithAuth(auth AuthConfig) HTTPOption {
	return func(c *HTTPClient) {
		c.auth = auth
	}
}

// WithTimeout 设置 HTTP 请求超时。
func WithTimeout(d time.Duration) HTTPOption {
	return func(c *HTTPClient) {
		c.client.Timeout = d
	}
}

// WithRateLimit 设置 QPS 限流（burst = qps）。
func WithRateLimit(qps float64) HTTPOption {
	return func(c *HTTPClient) {
		burst := int(math.Ceil(qps))
		if burst < 1 {
			burst = 1
		}
		c.limiter = rate.NewLimiter(rate.Limit(qps), burst)
	}
}

// WithTransport 设置自定义 HTTP Transport（用于跳过 TLS 验证等场景）。
func WithTransport(t http.RoundTripper) HTTPOption {
	return func(c *HTTPClient) {
		c.client.Transport = t
	}
}

// NewHTTPClient 创建带默认配置的 HTTPClient。
// 默认: 超时 30s，QPS 10（burst 10）。
func NewHTTPClient(opts ...HTTPOption) *HTTPClient {
	c := &HTTPClient{
		client:  &http.Client{Timeout: 30 * time.Second},
		limiter: rate.NewLimiter(rate.Limit(10), 10), // 默认 10 QPS
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

const (
	maxRetries        = 3              // 最大重试次数（不含首次请求）
	retryBaseDelay    = 500 * time.Millisecond // 指数退避基准延迟
)

// applyAuth 注入认证 Header。
// 双模式：直接值（token/password）优先，env 引用（token_env/password_env）兜底。
func (c *HTTPClient) applyAuth(req *http.Request) {
	switch c.auth.Type {
	case "token":
		token := c.auth.Token // 直接值优先
		if token == "" && c.auth.TokenEnv != "" {
			token = os.Getenv(c.auth.TokenEnv)
		}
		req.Header.Set("Authorization", "Token "+token)
	case "basic":
		password := c.auth.Password // 直接值优先
		if password == "" && c.auth.PasswordEnv != "" {
			password = os.Getenv(c.auth.PasswordEnv)
		}
		req.SetBasicAuth(c.auth.Username, password)
	case "bearer":
		token := c.auth.Token // 直接值优先
		if token == "" && c.auth.TokenEnv != "" {
			token = os.Getenv(c.auth.TokenEnv)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// Do 执行 HTTP 请求，支持指数退避重试和限流。
// 重试条件: 5xx 状态码或网络错误。
// 不重试: 4xx 状态码（客户端错误）。
// 退避策略: 500ms / 1s / 2s / 4s，最多 3 次重试。
func (c *HTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	// 解析完整 URL：请求中的相对路径拼接 baseURL
	if c.baseURL != "" && req.URL.Host == "" {
		fullURL := c.resolveURL(req.URL.String())
		parsed, err := url.Parse(fullURL)
		if err != nil {
			return nil, fmt.Errorf("resolve URL %s: %w", fullURL, err)
		}
		req.URL = parsed
	}

	// 注入认证头
	c.applyAuth(req)

	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 重试前退避等待（首次请求不等待）
		if attempt > 0 {
			delay := retryBaseDelay << (attempt - 1) // 500ms * 2^(attempt-1)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		// 限流等待
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		// 每次重试需要重建 request body（body 已被消费）
		// 对于 GET 请求（无 body），直接复用；对于有 body 的请求由调用方负责
		resp, err := c.client.Do(req.Clone(ctx))
		if err != nil {
			// 上下文超时/取消不重试
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			lastErr = err
			lastResp = nil
			slog.Warn("http request failed, retrying",
				"method", req.Method,
				"url", req.URL.String(),
				"attempt", attempt+1,
				"error", err,
			)
			continue
		}

		// 5xx 触发重试
		if resp.StatusCode >= 500 {
			// drain and close before retry
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lastResp = nil
			lastErr = fmt.Errorf("server error: status %d", resp.StatusCode)
			slog.Warn("http 5xx, retrying",
				"method", req.Method,
				"url", req.URL.String(),
				"status", resp.StatusCode,
				"attempt", attempt+1,
			)
			continue
		}

		// 成功或 4xx（不重试）
		return resp, nil
	}

	return lastResp, lastErr
}

// Get 发送 GET 请求。path 可以是相对路径（拼接 baseURL）或完整 URL。
func (c *HTTPClient) Get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("create GET request %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	return c.Do(ctx, req)
}

// Post 发送 POST 请求。body 为请求体（可为 nil）。
func (c *HTTPClient) Post(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, fmt.Errorf("create POST request %s: %w", path, err)
	}
	return c.Do(ctx, req)
}

// PostJSON 发送 JSON POST 请求，自动设置 Content-Type 头。
func (c *HTTPClient) PostJSON(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, fmt.Errorf("create POST request %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.Do(ctx, req)
}

// Put 发送 PUT 请求。body 为请求体（可为 nil）。
func (c *HTTPClient) Put(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, path, body)
	if err != nil {
		return nil, fmt.Errorf("create PUT request %s: %w", path, err)
	}
	return c.Do(ctx, req)
}

// Delete 发送 DELETE 请求。
func (c *HTTPClient) Delete(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, fmt.Errorf("create DELETE request %s: %w", path, err)
	}
	return c.Do(ctx, req)
}

// SetAuthToken 动态更新认证 Token。
// 用于 Bearer Token 自动刷新场景，线程安全由调用方保证。
func (c *HTTPClient) SetAuthToken(token string) {
	c.auth.Token = token
}

// resolveURL 将 baseURL + relativeURL 拼接为完整 URL。
func (c *HTTPClient) resolveURL(rawURL string) string {
	if c.baseURL == "" {
		return rawURL
	}
	// 如果 rawURL 已经是完整 URL，直接返回
	if _, err := url.Parse(rawURL); err == nil && len(rawURL) > 0 && rawURL[0:4] == "http" {
		return rawURL
	}
	return c.baseURL + rawURL
}

// PageResult 分页响应（兼容 Netbox/DRF 风格）。
type PageResult struct {
	Results []map[string]any `json:"results"`
	Next    string           `json:"next"`
	Count   int              `json:"count"`
}

// Paginate 自动遍历所有页并回调。
// path 为初始路径（相对 baseURL），pageSize 为每页大小。
// 每页数据通过 callback 回调，callback 返回 error 则中止遍历。
func (c *HTTPClient) Paginate(ctx context.Context, path string, pageSize int,
	callback func(page []map[string]any) error) error {

	nextURL := fmt.Sprintf("%s?limit=%d", path, pageSize)

	for nextURL != "" {
		fullURL := c.resolveURL(nextURL)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			return fmt.Errorf("create request %s: %w", fullURL, err)
		}
		// Do 方法会处理认证注入、限流和重试

		resp, err := c.Do(ctx, req)
		if err != nil {
			return fmt.Errorf("paginate request %s: %w", fullURL, err)
		}

		var page PageResult
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return fmt.Errorf("decode page response %s: %w", fullURL, err)
		}
		resp.Body.Close()

		// 非标准分页结构兜底：如果 results 为空但 body 有数据，尝试当作纯数组处理
		// （此处假设标准结构，results 为空就是空页）

		if len(page.Results) > 0 {
			if err := callback(page.Results); err != nil {
				return fmt.Errorf("callback for %s: %w", fullURL, err)
			}
		}

		nextURL = page.Next
	}

	return nil
}
