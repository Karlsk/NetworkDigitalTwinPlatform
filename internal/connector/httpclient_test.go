package connector

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TC-H01: Token Auth Header 正确注入
func TestHTTPClient_TokenAuth(t *testing.T) {
	t.Setenv("TEST_TOKEN_ENV", "abc123secret")

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithAuth(AuthConfig{
			Type:     "token",
			TokenEnv: "TEST_TOKEN_ENV",
		}),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/devices", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	want := "Token abc123secret"
	if gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
}

// TC-H02: Basic Auth Header 正确注入
func TestHTTPClient_BasicAuth(t *testing.T) {
	t.Setenv("TEST_PASSWORD_ENV", "p@ssw0rd")

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithAuth(AuthConfig{
			Type:        "basic",
			Username:    "admin",
			PasswordEnv: "TEST_PASSWORD_ENV",
		}),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/devices", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	// Basic auth header 格式: "Basic base64(user:pass)"
	expected := base64.StdEncoding.EncodeToString([]byte("admin:p@ssw0rd"))
	want := "Basic " + expected
	if gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
}

// TC-H03: 5xx 触发指数退避重试
func TestHTTPClient_RetryOn5xx(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		if n <= int32(maxRetries) {
			// 前 3 次返回 500，第 4 次返回 200
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL))

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/test", nil)

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	totalRequests := count.Load()
	wantRequests := int32(maxRetries + 1) // 首次 + 重试次数
	if totalRequests != wantRequests {
		t.Errorf("total requests = %d, want %d", totalRequests, wantRequests)
	}
}

// TC-H03b: 5xx 全部重试后仍失败
func TestHTTPClient_RetryExhausted(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL))

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/test", nil)

	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("Do() expected error, got nil")
	}

	// 验证总共请求了 4 次（首次 + 3 次重试）
	totalRequests := count.Load()
	if totalRequests != int32(maxRetries+1) {
		t.Errorf("total requests = %d, want %d", totalRequests, maxRetries+1)
	}
}

// TC-H04: 4xx 不触发重试
func TestHTTPClient_NoRetryOn4xx(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL))

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/test", nil)

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	// 4xx 不应重试，只请求 1 次
	if got := count.Load(); got != 1 {
		t.Errorf("total requests = %d, want 1", got)
	}
}

// TC-H05: 限流器正确控制 QPS
func TestHTTPClient_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	// 设置 1 QPS，burst=1
	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithRateLimit(1),
	)

	ctx := context.Background()

	// 快速发送 3 个请求，验证限流器控制了 QPS
	// 第 1 个请求立即可用，第 2 个等待 ~1s，第 3 个等待 ~2s
	start := time.Now()
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/api/test", nil)
		resp, err := c.Do(ctx, req)
		if err != nil {
			t.Fatalf("Do() request %d error = %v", i, err)
		}
		resp.Body.Close()
	}
	elapsed := time.Since(start)

	// 3 个请求至少需要 ~2 秒（burst=1，后续每秒 1 个）
	if elapsed < 1500*time.Millisecond {
		t.Errorf("elapsed = %v, expected >= 1.5s for 3 requests at 1 QPS", elapsed)
	}
}

// TC-H06: Paginate 遍历所有页（多页 + 单页 + 空结果）

// helper: 创建分页测试服务器
func newPaginateServer(t *testing.T, pages [][]map[string]any) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		idx := int(n) - 1
		if idx >= len(pages) {
			// 超出页数，返回空页
			json.NewEncoder(w).Encode(PageResult{Results: []map[string]any{}, Next: ""})
			return
		}

		var nextURL string
		if idx+1 < len(pages) {
			nextURL = fmt.Sprintf("/api/items?limit=2&offset=%d", (idx+1)*2)
		}

		resp := PageResult{
			Results: pages[idx],
			Next:    nextURL,
			Count:   len(pages) * 2,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	return srv, &reqCount
}

func TestHTTPClient_Paginate_MultiPage(t *testing.T) {
	page1 := []map[string]any{{"id": 1}, {"id": 2}}
	page2 := []map[string]any{{"id": 3}, {"id": 4}}
	page3 := []map[string]any{{"id": 5}}
	srv, _ := newPaginateServer(t, [][]map[string]any{page1, page2, page3})
	defer srv.Close()

	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithRateLimit(100), // 高 QPS 避免测试慢
	)

	var collected []int
	err := c.Paginate(context.Background(), "/api/items", 2, func(page []map[string]any) error {
		for _, item := range page {
			if id, ok := item["id"].(float64); ok {
				collected = append(collected, int(id))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Paginate() error = %v", err)
	}

	want := []int{1, 2, 3, 4, 5}
	if len(collected) != len(want) {
		t.Fatalf("collected %d items, want %d", len(collected), len(want))
	}
	for i, v := range want {
		if collected[i] != v {
			t.Errorf("collected[%d] = %d, want %d", i, collected[i], v)
		}
	}
}

func TestHTTPClient_Paginate_SinglePage(t *testing.T) {
	page1 := []map[string]any{{"id": 1}}
	srv, _ := newPaginateServer(t, [][]map[string]any{page1})
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL), WithRateLimit(100))

	var count int
	err := c.Paginate(context.Background(), "/api/items", 10, func(page []map[string]any) error {
		count += len(page)
		return nil
	})
	if err != nil {
		t.Fatalf("Paginate() error = %v", err)
	}
	if count != 1 {
		t.Errorf("collected %d items, want 1", count)
	}
}

func TestHTTPClient_Paginate_EmptyResult(t *testing.T) {
	// 返回空结果页
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PageResult{
			Results: []map[string]any{},
			Next:    "",
			Count:   0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL), WithRateLimit(100))

	var count int
	err := c.Paginate(context.Background(), "/api/items", 10, func(page []map[string]any) error {
		count += len(page)
		return nil
	})
	if err != nil {
		t.Fatalf("Paginate() error = %v", err)
	}
	if count != 0 {
		t.Errorf("collected %d items, want 0", count)
	}
}

func TestHTTPClient_Paginate_CallbackError(t *testing.T) {
	page1 := []map[string]any{{"id": 1}}
	srv, _ := newPaginateServer(t, [][]map[string]any{page1})
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL), WithRateLimit(100))

	stopErr := fmt.Errorf("stop iteration")
	err := c.Paginate(context.Background(), "/api/items", 2, func(page []map[string]any) error {
		return stopErr
	})
	if err == nil {
		t.Fatal("Paginate() expected error, got nil")
	}
	if err.Error() == "" {
		t.Error("Paginate() error should contain callback error context")
	}
}

// TC-H07: 超时正确返回错误
func TestHTTPClient_Timeout(t *testing.T) {
	// 使用 channel 控制 handler，避免 srv.Close() 等待 sleep 完成
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(3 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-handlerDone:
			return
		}
	}))
	defer func() {
		close(handlerDone)
		srv.Close()
	}()

	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithTimeout(100*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/api/slow", nil)

	start := time.Now()
	_, err := c.Do(ctx, req)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Do() expected timeout error, got nil")
	}

	// errors.Is 检测到 context.DeadlineExceeded 后应快速返回（< 1s）
	if elapsed > 1*time.Second {
		t.Errorf("elapsed = %v, expected < 1s (no retry on context error)", elapsed)
	}
}

// TC-H08: None Auth 不注入认证头
func TestHTTPClient_NoAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL))

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/test", nil)

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if gotAuth != "" {
		t.Errorf("Authorization header = %q, want empty", gotAuth)
	}
}

// TC-H09: NewHTTPClient 默认配置验证
func TestNewHTTPClient_Defaults(t *testing.T) {
	c := NewHTTPClient()

	if c.client == nil {
		t.Error("client is nil")
	}
	if c.client.Timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", c.client.Timeout)
	}
	if c.limiter == nil {
		t.Error("limiter is nil")
	}
	// 默认 10 QPS，burst 10
	// 快速获取 10 个令牌应立即可用
	for i := 0; i < 10; i++ {
		if !c.limiter.Allow() {
			t.Errorf("limiter burst should allow 10 immediate requests, failed at %d", i)
		}
	}
}

// TC-H10: WithRateLimit 自定义 QPS 验证
func TestWithRateLimit(t *testing.T) {
	c := NewHTTPClient(WithRateLimit(5))

	if c.limiter == nil {
		t.Fatal("limiter is nil")
	}

	// burst=5，快速获取 5 个应成功
	for i := 0; i < 5; i++ {
		if !c.limiter.Allow() {
			t.Errorf("rate limit burst 5 should allow 5 immediate requests, failed at %d", i)
		}
	}
	// 第 6 个应被限流
	if c.limiter.Allow() {
		t.Error("6th request should be rate limited")
	}
}

// TC-H11: WithTimeout 自定义超时验证
func TestWithTimeout(t *testing.T) {
	c := NewHTTPClient(WithTimeout(5 * time.Second))

	if c.client.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.client.Timeout)
	}
}

// TC-H12: WithBaseURL 验证
func TestWithBaseURL(t *testing.T) {
	c := NewHTTPClient(WithBaseURL("https://api.example.com"))

	if c.baseURL != "https://api.example.com" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "https://api.example.com")
	}
}

// TC-H13: PageResult JSON 解码验证
func TestPageResult_Decode(t *testing.T) {
	raw := `{"results":[{"id":1,"name":"dev1"}],"next":"http://api/items?offset=1","count":10}`
	var pr PageResult
	if err := json.Unmarshal([]byte(raw), &pr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(pr.Results) != 1 {
		t.Errorf("results len = %d, want 1", len(pr.Results))
	}
	if pr.Count != 10 {
		t.Errorf("count = %d, want 10", pr.Count)
	}
	if pr.Next == "" {
		t.Error("next should not be empty")
	}
}

// TC-H14: resolveURL 拼接验证
func TestHTTPClient_resolveURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		rawURL  string
		want    string
	}{
		{"base + relative", "https://api.example.com", "/api/items", "https://api.example.com/api/items"},
		{"empty base", "", "/api/items", "/api/items"},
		{"absolute url passthrough", "https://api.example.com", "https://other.com/items", "https://other.com/items"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &HTTPClient{baseURL: tt.baseURL}
			got := c.resolveURL(tt.rawURL)
			if got != tt.want {
				t.Errorf("resolveURL(%q) = %q, want %q", tt.rawURL, got, tt.want)
			}
		})
	}
}

// TC-H15: 成功请求返回响应体
func TestHTTPClient_SuccessBody(t *testing.T) {
	body := `{"message":"hello world"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL))

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/hello", nil)

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if string(respBody) != body {
		t.Errorf("body = %q, want %q", string(respBody), body)
	}
}

// TC-H16: Token Auth 直接值模式（不通过 env）
func TestHTTPClient_TokenAuth_DirectValue(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithAuth(AuthConfig{
			Type:  "token",
			Token: "my-direct-token-123", // 直接值
		}),
	)

	resp, err := c.Get(context.Background(), "/api/test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()

	want := "Token my-direct-token-123"
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}

// TC-H17: Token Auth 直接值优先于 env
func TestHTTPClient_TokenAuth_DirectValuePriority(t *testing.T) {
	t.Setenv("MY_PRIORITY_TOKEN", "env-value")

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithAuth(AuthConfig{
			Type:     "token",
			Token:    "direct-wins",       // 直接值
			TokenEnv: "MY_PRIORITY_TOKEN", // env 兜底
		}),
	)

	resp, err := c.Get(context.Background(), "/api/test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()

	// 直接值应优先
	want := "Token direct-wins"
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q (direct value should take priority)", gotAuth, want)
	}
}

// TC-H18: Token Auth env 兜底（直接值为空时）
func TestHTTPClient_TokenAuth_EnvFallback(t *testing.T) {
	t.Setenv("MY_FALLBACK_TOKEN", "env-fallback-value")

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithAuth(AuthConfig{
			Type:     "token",
			TokenEnv: "MY_FALLBACK_TOKEN", // 仅 env 引用
		}),
	)

	resp, err := c.Get(context.Background(), "/api/test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()

	want := "Token env-fallback-value"
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}

// TC-H19: Basic Auth 直接值模式
func TestHTTPClient_BasicAuth_DirectValue(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithAuth(AuthConfig{
			Type:     "basic",
			Username: "admin",
			Password: "direct-pass-456", // 直接值
		}),
	)

	resp, err := c.Get(context.Background(), "/api/test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()

	expected := base64.StdEncoding.EncodeToString([]byte("admin:direct-pass-456"))
	want := "Basic " + expected
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}

// TC-H20: Get 便捷方法 + 认证 + baseURL 拼接
func TestHTTPClient_Get(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL))
	resp, err := c.Get(context.Background(), "/api/devices")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()

	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodGet)
	}
	if gotPath != "/api/devices" {
		t.Errorf("path = %q, want %q", gotPath, "/api/devices")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TC-H21: Post 便捷方法
func TestHTTPClient_Post(t *testing.T) {
	var gotMethod string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL))
	resp, err := c.Post(context.Background(), "/api/devices", strings.NewReader(`{"name":"dev1"}`))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	defer resp.Body.Close()

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotBody != `{"name":"dev1"}` {
		t.Errorf("body = %q, want %q", gotBody, `{"name":"dev1"}`)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

// TC-H22: Put 便捷方法
func TestHTTPClient_Put(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL))
	resp, err := c.Put(context.Background(), "/api/devices/1", strings.NewReader(`{"name":"updated"}`))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	defer resp.Body.Close()

	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodPut)
	}
}

// TC-H23: Delete 便捷方法
func TestHTTPClient_Delete(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL))
	resp, err := c.Delete(context.Background(), "/api/devices/1")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	defer resp.Body.Close()

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodDelete)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

// TC-H24: Bearer Auth 直接值模式
func TestHTTPClient_BearerAuth_DirectValue(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithAuth(AuthConfig{
			Type:  "bearer",
			Token: "my-bearer-token-123",
		}),
	)

	resp, err := c.Get(context.Background(), "/api/test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()

	want := "Bearer my-bearer-token-123"
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}

// TC-H25: Bearer Auth env 兜底（直接值为空时）
func TestHTTPClient_BearerAuth_EnvFallback(t *testing.T) {
	t.Setenv("MY_BEARER_TOKEN", "env-bearer-value")

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithAuth(AuthConfig{
			Type:     "bearer",
			TokenEnv: "MY_BEARER_TOKEN",
		}),
	)

	resp, err := c.Get(context.Background(), "/api/test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()

	want := "Bearer env-bearer-value"
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}

// TC-H26: Bearer Auth 直接值优先于 env
func TestHTTPClient_BearerAuth_DirectValuePriority(t *testing.T) {
	t.Setenv("MY_BEARER_PRIORITY", "env-value")

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(
		WithBaseURL(srv.URL),
		WithAuth(AuthConfig{
			Type:     "bearer",
			Token:    "direct-bearer-wins",
			TokenEnv: "MY_BEARER_PRIORITY",
		}),
	)

	resp, err := c.Get(context.Background(), "/api/test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()

	want := "Bearer direct-bearer-wins"
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q (direct value should take priority)", gotAuth, want)
	}
}

// TestWithTransport 验证 WithTransport 选项正确设置自定义 Transport。
func TestWithTransport(t *testing.T) {
	customTransport := http.DefaultTransport
	c := NewHTTPClient(WithTransport(customTransport))
	if c.client.Transport != customTransport {
		t.Error("WithTransport did not set custom transport")
	}
}

// TestPostJSON 验证 PostJSON 自动设置 Content-Type 并发送请求。
func TestPostJSON(t *testing.T) {
	var gotContentType string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithBaseURL(srv.URL))
	resp, err := c.PostJSON(context.Background(), "/api/test", strings.NewReader(`{"key":"val"}`))
	if err != nil {
		t.Fatalf("PostJSON() error = %v", err)
	}
	defer resp.Body.Close()

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody != `{"key":"val"}` {
		t.Errorf("body = %q, want {\"key\":\"val\"}", gotBody)
	}
}

// TestSetAuthToken 验证 SetAuthToken 动态更新 Token。
func TestSetAuthToken(t *testing.T) {
	c := NewHTTPClient(WithAuth(AuthConfig{Type: "token", Token: "old"}))
	c.SetAuthToken("new-token-123")
	if c.auth.Token != "new-token-123" {
		t.Errorf("Token = %q, want new-token-123", c.auth.Token)
	}
}
