// Package controller 实现 Controller Connector 及统一 API 适配层。
// client.go 定义 ControllerClient 主体：Token 自动管理 + 设备缓存。
// Connector（拓扑同步）和 MCP（按需查询）共享此 Client。
package controller

import (
	"context"
	"crypto/cipher"
	"crypto/des"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ControllerClient 骨干网操作系统 REST API 统一客户端。
// Token 自动管理（过期刷新 + 60s 缓冲），所有 API 调用前自动 ensureToken。
type ControllerClient struct {
	http    *connector.HTTPClient
	name    string
	baseURL string // API 基地址，供 Metadata 使用

	// Token 管理
	tokenURL     string
	username     string
	password     string // 明文密码
	deviceID     string
	desSecretKey string // 3DES 密钥（24 字节）
	token        string
	tokenExp     time.Time

	// 设备缓存（ISIS/BGP 依赖 Device 的 vendor 信息）
	cachedDevices []DeviceInfo
	cacheTime     time.Time
	cacheTTL      time.Duration

	mu sync.Mutex // 保护 token 和缓存字段
}

// NewControllerClient 创建 ControllerClient 实例。
// 从 cfg 解析: token_url, username, password/password_env, device_id, des_secret_key。
func NewControllerClient(name string, client *connector.HTTPClient, cfg map[string]any) *ControllerClient {
	tokenURL, _ := cfg["token_url"].(string)
	if tokenURL == "" {
		tokenURL = "/oauth/token"
	}
	username, _ := cfg["username"].(string)
	// password 优先直接值，其次环境变量
	password, _ := cfg["password"].(string)
	if password == "" {
		if env, ok := cfg["password_env"].(string); ok && env != "" {
			password = os.Getenv(env)
		}
	}
	deviceID, _ := cfg["device_id"].(string)
	baseURL, _ := cfg["base_url"].(string)
	desSecretKey, _ := cfg["des_secret_key"].(string)
	if desSecretKey == "" {
		desSecretKey = "9mng65v8jf4lxn93nabf981m" // 默认密钥
	}

	return &ControllerClient{
		http:         client,
		name:         name,
		baseURL:      baseURL,
		tokenURL:     tokenURL,
		username:     username,
		password:     password,
		deviceID:     deviceID,
		desSecretKey: desSecretKey,
		cacheTTL:     5 * time.Minute,
	}
}

// ──────────────────────────────
// Token 管理
// ──────────────────────────────

// encryptPassword 使用 3DES/CBC/PKCS5Padding 加密密码。
// IV = SHA256(secretKey)[:8]
func encryptPassword(secretKey, plaintext string) (string, error) {
	key := []byte(secretKey)
	if len(key) != 24 {
		return "", fmt.Errorf("des secret key must be 24 bytes, got %d", len(key))
	}

	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return "", fmt.Errorf("create 3des cipher: %w", err)
	}

	// IV = SHA256(key)[:8]
	hash := sha256.Sum256(key)
	iv := hash[:8]

	// PKCS5 padding
	padding := block.BlockSize() - len(plaintext)%block.BlockSize()
	padded := make([]byte, len(plaintext)+padding)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padding)
	}

	// CBC encrypt
	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// ensureToken 检查 Token 是否有效，过期则自动刷新。
// 预留 60 秒缓冲，避免临界过期。
func (c *ControllerClient) ensureToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExp.Add(-60*time.Second)) {
		return nil // Token 仍有效
	}

	return c.fetchTokenLocked(ctx)
}

// fetchTokenLocked 调用 POST /oauth/token 获取新 Token。
// 调用方必须持有 c.mu 锁。
func (c *ControllerClient) fetchTokenLocked(ctx context.Context) error {
	// 3DES 加密密码
	encryptedPassword, err := encryptPassword(c.desSecretKey, c.password)
	if err != nil {
		return fmt.Errorf("encrypt password: %w", err)
	}

	body := tokenRequest{
		Username: c.username,
		Password: encryptedPassword,
		DeviceID: c.deviceID,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal token request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, c.tokenURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return fmt.Errorf("post token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token request failed: status %d", resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("empty access token in response")
	}

	c.token = tokenResp.AccessToken
	c.tokenExp = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// 动态更新 HTTPClient 的认证 Token
	c.http.SetAuthToken(c.token)

	slog.Info("controller token refreshed", "expires_in", tokenResp.ExpiresIn)
	return nil
}

// Ping 验证 Controller 可达性（通过 ensureToken 验证）。
func (c *ControllerClient) Ping(ctx context.Context) error {
	return c.ensureToken(ctx)
}

// ──────────────────────────────
// 设备缓存
// ──────────────────────────────

// getDevices 获取设备列表（优先从缓存读取，TTL 5 分钟）。
func (c *ControllerClient) getDevices(ctx context.Context) ([]DeviceInfo, error) {
	c.mu.Lock()
	if time.Since(c.cacheTime) < c.cacheTTL && len(c.cachedDevices) > 0 {
		devices := c.cachedDevices
		c.mu.Unlock()
		return devices, nil
	}
	c.mu.Unlock()

	// 缓存过期或为空，直接调用 FetchDevices 获取原始数据
	rawList, err := c.FetchDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("get devices for cache: %w", err)
	}

	var devices []DeviceInfo
	for _, raw := range rawList {
		name, _ := raw["name"].(string)
		vendor, _ := raw["vendor-id"].(string)
		devices = append(devices, DeviceInfo{Name: name, Vendor: vendor})
	}

	c.mu.Lock()
	c.cachedDevices = devices
	c.cacheTime = time.Now()
	c.mu.Unlock()

	return devices, nil
}
