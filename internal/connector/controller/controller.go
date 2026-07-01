// Package controller 实现 Controller Connector，对接网络控制器 REST API。
// 支持 8 种实体类型的采集：Device, Interface, Link, Alarm, VPN, Tunnel, ISIS, BGP。
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

// DeviceInfo 缓存的设备基本信息，供 ISIS/BGP 采集时查询厂商信息。
type DeviceInfo struct {
	Name   string // 设备名称（pe-name）
	Vendor string // 厂商（H3C/ZTE/Huawei）
}

// ControllerConnector 从控制器 REST API 采集 8 种实体类型数据。
type ControllerConnector struct {
	http     *connector.HTTPClient
	name     string
	types    []string
	cfg      map[string]any
	baseURL  string

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
	cacheTTL      time.Duration // 默认 5 分钟

	mu sync.Mutex // 保护 token 和缓存字段
}

// 编译时接口满足检查
var _ connector.Connector = (*ControllerConnector)(nil)

// NewControllerConnector 创建 ControllerConnector 实例。
func NewControllerConnector(name string, client *connector.HTTPClient, entityTypes []string, cfg map[string]any) *ControllerConnector {
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

	return &ControllerConnector{
		http:         client,
		name:         name,
		types:        entityTypes,
		cfg:          cfg,
		baseURL:      baseURL,
		tokenURL:     tokenURL,
		username:     username,
		password:     password,
		deviceID:     deviceID,
		desSecretKey: desSecretKey,
		cacheTTL:     5 * time.Minute,
	}
}



// Metadata 返回连接器元信息。
func (c *ControllerConnector) Metadata() connector.ConnectorMetadata {
	return connector.ConnectorMetadata{
		Name:        c.name,
		Type:        "controller",
		EntityTypes: c.types,
		BaseURL:     c.baseURL,
		AuthType:    "bearer",
	}
}

// Ping 验证 Controller 可达性。
// 通过获取 Token 来验证连通性和认证信息。
func (c *ControllerConnector) Ping(ctx context.Context) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("controller ping: %w", err)
	}
	return nil
}

// Collect 全量拉取指定实体类型的数据。
// 支持 8 种实体类型：Device, Interface, Link, Alarm, VPN, Tunnel, ISIS, BGP。
func (c *ControllerConnector) Collect(ctx context.Context, entityType string) ([]connector.Resource, error) {
	// 确保 Token 有效
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("controller collect %s: %w", entityType, err)
	}

	switch entityType {
	case "Device":
		return c.collectDevices(ctx)
	case "Interface":
		return c.collectInterfaces(ctx)
	case "Link":
		return c.collectLinks(ctx)
	case "Alarm":
		return c.collectAlarms(ctx)
	case "VPN":
		return c.collectVPNs(ctx)
	case "Tunnel":
		return c.collectTunnels(ctx)
	case "ISIS":
		return c.collectISIS(ctx)
	case "BGP":
		return c.collectBGP(ctx)
	default:
		return nil, fmt.Errorf("controller connector: unsupported entity type %q", entityType)
	}
}

// Stream 返回 ErrNotImplemented，增量同步功能保留接口骨架，后续补充实现。
func (c *ControllerConnector) Stream(ctx context.Context, entityType string) (<-chan connector.Resource, error) {
	return nil, connector.ErrNotImplemented
}

// ──────────────────────────────
// Token 管理
// ──────────────────────────────

// tokenRequest Token 请求体。
type tokenRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	DeviceID string `json:"device_id"`
}

// tokenResponse Token 响应体。
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"` // 秒
}

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
func (c *ControllerConnector) ensureToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExp.Add(-60*time.Second)) {
		return nil // Token 仍有效
	}

	return c.fetchTokenLocked(ctx)
}

// fetchTokenLocked 调用 POST /oauth/token 获取新 Token。
// 调用方必须持有 c.mu 锁。
func (c *ControllerConnector) fetchTokenLocked(ctx context.Context) error {
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

// ──────────────────────────────
// 设备缓存
// ──────────────────────────────

// getDevices 获取设备列表（优先从缓存读取）。
func (c *ControllerConnector) getDevices(ctx context.Context) ([]DeviceInfo, error) {
	c.mu.Lock()
	if time.Since(c.cacheTime) < c.cacheTTL && len(c.cachedDevices) > 0 {
		devices := c.cachedDevices
		c.mu.Unlock()
		return devices, nil
	}
	c.mu.Unlock()

	// 缓存过期或为空，重新采集
	resources, err := c.collectDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("get devices for cache: %w", err)
	}

	var devices []DeviceInfo
	for _, r := range resources {
		name, _ := r.Properties["hostname"].(string)
		vendor, _ := r.Properties["vendor"].(string)
		devices = append(devices, DeviceInfo{Name: name, Vendor: vendor})
	}

	c.mu.Lock()
	c.cachedDevices = devices
	c.cacheTime = time.Now()
	c.mu.Unlock()

	return devices, nil
}

// ──────────────────────────────
// 各实体采集方法（骨架）
// ──────────────────────────────

// collectDevices 采集 Device 实体。
func (c *ControllerConnector) collectDevices(ctx context.Context) ([]connector.Resource, error) {
	rawList, err := c.fetchDevicesRaw(ctx)
	if err != nil {
		return nil, err
	}

	var resources []connector.Resource
	for _, raw := range rawList {
		props := transformDevice(raw)
		id, _ := raw["id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "Device",
			ID:         id,
			Properties: props,
		})
	}

	slog.Info("controller devices collected", "count", len(resources))
	return resources, nil
}

// collectInterfaces 采集 Interface 实体（从 Device 缓存提取）。
func (c *ControllerConnector) collectInterfaces(ctx context.Context) ([]connector.Resource, error) {
	rawList, err := c.fetchDevicesRaw(ctx)
	if err != nil {
		return nil, err
	}

	var resources []connector.Resource
	for _, raw := range rawList {
		deviceName, _ := raw["name"].(string)
		ifaces := extractInterfaces(deviceName, raw)
		resources = append(resources, ifaces...)
	}

	slog.Info("controller interfaces collected", "count", len(resources))
	return resources, nil
}

// collectLinks 采集 Link 实体。
func (c *ControllerConnector) collectLinks(ctx context.Context) ([]connector.Resource, error) {
	rawList, err := c.fetchLinksRaw(ctx)
	if err != nil {
		return nil, err
	}

	var resources []connector.Resource
	for _, raw := range rawList {
		props := transformLink(raw)
		id, _ := raw["link-id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "Link",
			ID:         id,
			Properties: props,
		})
	}

	slog.Info("controller links collected", "count", len(resources))
	return resources, nil
}

// collectAlarms 采集 Alarm 实体。
func (c *ControllerConnector) collectAlarms(ctx context.Context) ([]connector.Resource, error) {
	rawList, err := c.fetchAlarmsRaw(ctx)
	if err != nil {
		return nil, err
	}

	var resources []connector.Resource
	for _, raw := range rawList {
		props := transformAlarm(raw)
		id, _ := raw["id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "Alarm",
			ID:         id,
			Properties: props,
		})
	}

	slog.Info("controller alarms collected", "count", len(resources))
	return resources, nil
}

// collectVPNs 采集 VPN 实体（L3VPN + L2VPN 合并）。
func (c *ControllerConnector) collectVPNs(ctx context.Context) ([]connector.Resource, error) {
	l3, err := c.fetchL3VPNsRaw(ctx)
	if err != nil {
		slog.Warn("controller l3vpn fetch failed", "error", err)
	}
	l2, err2 := c.fetchL2VPNsRaw(ctx)
	if err2 != nil {
		slog.Warn("controller l2vpn fetch failed", "error", err2)
	}

	if len(l3) == 0 && len(l2) == 0 {
		if err != nil {
			return nil, err
		}
		if err2 != nil {
			return nil, err2
		}
	}

	var resources []connector.Resource
	for _, raw := range l3 {
		props := transformVPN(raw, "L3")
		id, _ := raw["vpn-id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "VPN",
			ID:         id,
			Properties: props,
		})
	}
	for _, raw := range l2 {
		props := transformVPN(raw, "L2")
		id, _ := raw["vpn-id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "VPN",
			ID:         id,
			Properties: props,
		})
	}

	slog.Info("controller vpns collected", "count", len(resources), "l3", len(l3), "l2", len(l2))
	return resources, nil
}

// collectTunnels 采集 Tunnel 实体。
func (c *ControllerConnector) collectTunnels(ctx context.Context) ([]connector.Resource, error) {
	rawList, err := c.fetchTunnelsRaw(ctx)
	if err != nil {
		return nil, err
	}

	var resources []connector.Resource
	for _, raw := range rawList {
		props := transformTunnel(raw)
		id, _ := raw["instance-id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "Tunnel",
			ID:         id,
			Properties: props,
		})
	}

	slog.Info("controller tunnels collected", "count", len(resources))
	return resources, nil
}

// collectISIS 采集 ISIS 实体（N+1 调用 + 文本解析）。
func (c *ControllerConnector) collectISIS(ctx context.Context) ([]connector.Resource, error) {
	devices, err := c.getDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect isis: get devices: %w", err)
	}

	var resources []connector.Resource
	var mu sync.Mutex

	// 使用 goroutine 并发采集，最大并发数 5
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, dev := range devices {
		if dev.Name == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(d DeviceInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			text, err := c.fetchISISText(ctx, d.Name)
			if err != nil {
				slog.Warn("isis fetch failed", "device", d.Name, "error", err)
				return
			}

			peers, err := ParseISISText(d.Vendor, text)
			if err != nil {
				slog.Warn("isis parse failed", "device", d.Name, "vendor", d.Vendor, "error", err)
				return
			}

			mu.Lock()
			for _, peer := range peers {
				props := transformISISPeer(d.Name, peer)
				isisID, _ := props["isis_id"].(string)
				resources = append(resources, connector.Resource{
					Kind:       "ISIS",
					ID:         isisID,
					Properties: props,
				})
			}
			mu.Unlock()
		}(dev)
	}
	wg.Wait()

	slog.Info("controller isis collected", "count", len(resources), "devices", len(devices))
	return resources, nil
}

// collectBGP 采集 BGP 实体（N+1 调用 + 文本解析）。
func (c *ControllerConnector) collectBGP(ctx context.Context) ([]connector.Resource, error) {
	devices, err := c.getDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect bgp: get devices: %w", err)
	}

	var resources []connector.Resource
	var mu sync.Mutex

	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, dev := range devices {
		if dev.Name == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(d DeviceInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			text, err := c.fetchBGPText(ctx, d.Name)
			if err != nil {
				slog.Warn("bgp fetch failed", "device", d.Name, "error", err)
				return
			}

			peers, err := ParseBGPText(d.Vendor, text)
			if err != nil {
				slog.Warn("bgp parse failed", "device", d.Name, "vendor", d.Vendor, "error", err)
				return
			}

			mu.Lock()
			for _, peer := range peers {
				props := transformBGPPeer(d.Name, peer)
				bgpID, _ := props["bgp_id"].(string)
				resources = append(resources, connector.Resource{
					Kind:       "BGP",
					ID:         bgpID,
					Properties: props,
				})
			}
			mu.Unlock()
		}(dev)
	}
	wg.Wait()

	slog.Info("controller bgp collected", "count", len(resources), "devices", len(devices))
	return resources, nil
}

// ──────────────────────────────
// 原始 API 调用方法
// ──────────────────────────────

// deviceResponse Device 接口响应包装。
type deviceResponse struct {
	PeInfo []map[string]any `json:"peInfo"`
}

// fetchDevicesRaw 调用 Device 全量接口，返回原始 JSON 数组。
func (c *ControllerConnector) fetchDevicesRaw(ctx context.Context) ([]map[string]any, error) {
	resp, err := c.http.Get(ctx, "/api/no/config/terra-pe:peInfos/peInfos")
	if err != nil {
		return nil, fmt.Errorf("fetch devices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch devices: status %d", resp.StatusCode)
	}

	var devResp deviceResponse
	if err := json.NewDecoder(resp.Body).Decode(&devResp); err != nil {
		return nil, fmt.Errorf("decode devices response: %w", err)
	}
	return devResp.PeInfo, nil
}

// fetchLinksRaw 调用 Link 全量接口，返回原始 JSON 数组。
func (c *ControllerConnector) fetchLinksRaw(ctx context.Context) ([]map[string]any, error) {
	resp, err := c.http.Get(ctx, "/api/sr/config/network-topology:network-topology/topology/linksInfo")
	if err != nil {
		return nil, fmt.Errorf("fetch links: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch links: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode links response: %w", err)
	}
	return result, nil
}

// alarmResponse 告警接口响应包装。
type alarmResponse struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    []map[string]any `json:"data"`
}

// fetchAlarmsRaw 调用 Alarm 接口，返回原始 JSON 数组。
func (c *ControllerConnector) fetchAlarmsRaw(ctx context.Context) ([]map[string]any, error) {
	resp, err := c.http.Get(ctx, "/monitor/alert/list?namespace=business&interval=1h")
	if err != nil {
		return nil, fmt.Errorf("fetch alarms: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch alarms: status %d", resp.StatusCode)
	}

	var alarmResp alarmResponse
	if err := json.NewDecoder(resp.Body).Decode(&alarmResp); err != nil {
		return nil, fmt.Errorf("decode alarms response: %w", err)
	}
	// data 可能为 null（无告警时）
	if alarmResp.Data == nil {
		return nil, nil
	}
	return alarmResp.Data, nil
}

// vpnPageResponse VPN 自定义分页响应。
type vpnPageResponse struct {
	PageNum       int              `json:"page_num"`
	PageSize      int              `json:"page_size"`
	TotalElements int              `json:"total_elements"`
	TotalPages    int              `json:"total_pages"`
	Content       []map[string]any `json:"content"`
}

// flattenVPNItems 从 VPN 分页响应中提取所有 vpn-service 条目。
// 真实 API 结构: content[].vpn-services.vpn-service[]
func flattenVPNItems(content []map[string]any) []map[string]any {
	var items []map[string]any
	for _, entry := range content {
		vpnServices, ok := entry["vpn-services"].(map[string]any)
		if !ok {
			continue
		}
		svcList, ok := vpnServices["vpn-service"].([]any)
		if !ok {
			continue
		}
		for _, svc := range svcList {
			if m, ok := svc.(map[string]any); ok {
				items = append(items, m)
			}
		}
	}
	return items
}

// fetchL3VPNsRaw 分页采集 L3VPN 数据。
func (c *ControllerConnector) fetchL3VPNsRaw(ctx context.Context) ([]map[string]any, error) {
	return c.paginateVPN(ctx, "/api/no/config/ietf-l3vpn-ntw:l3vpn-ntw/page", 100)
}

// fetchL2VPNsRaw 分页采集 L2VPN 数据。
func (c *ControllerConnector) fetchL2VPNsRaw(ctx context.Context) ([]map[string]any, error) {
	return c.paginateVPN(ctx, "/api/no/config/ietf-l2vpn-svc:l2vpn-svc/page", 100)
}

// paginateVPN 遍历 VPN 自定义分页（1-based page_num），返回展平后的 vpn-service 列表。
func (c *ControllerConnector) paginateVPN(ctx context.Context, baseURL string, pageSize int) ([]map[string]any, error) {
	var allItems []map[string]any
	pageNum := 1

	for {
		path := fmt.Sprintf("%s?pageNo=%d&pageSize=%d", baseURL, pageNum-1, pageSize)
		resp, err := c.http.Get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("paginate vpn page %d: %w", pageNum, err)
		}

		var pageResp vpnPageResponse
		if err := json.NewDecoder(resp.Body).Decode(&pageResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode vpn page %d: %w", pageNum, err)
		}
		resp.Body.Close()

		// 展平 vpn-services.vpn-service[] 嵌套结构
		items := flattenVPNItems(pageResp.Content)
		allItems = append(allItems, items...)

		if pageNum >= pageResp.TotalPages || len(pageResp.Content) == 0 {
			break
		}
		pageNum++
	}

	return allItems, nil
}

// fetchTunnelsRaw 调用 Tunnel 全量接口。
func (c *ControllerConnector) fetchTunnelsRaw(ctx context.Context) ([]map[string]any, error) {
	resp, err := c.http.Get(ctx, "/api/sr/config/terra-te-svc:te-policy-instance/all")
	if err != nil {
		return nil, fmt.Errorf("fetch tunnels: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch tunnels: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode tunnels response: %w", err)
	}
	return result, nil
}

// isisRequest ISIS 接口请求体。
type isisRequest struct {
	Input struct {
		PeName  string `json:"pe-name"`
		Process int    `json:"process"`
		Verbose bool   `json:"verbose"`
		Scope   string `json:"scope"`
	} `json:"input"`
}

// restconfResponse Restconf 文本回显响应。
type restconfResponse struct {
	Output struct {
		CurrentConfigResult string `json:"current-config-result"`
		ISISNeighborResult  string `json:"isis-neighbor-result"`
	} `json:"output"`
}

// fetchISISText 调用单台设备的 ISIS 邻居接口，返回回显文本。
func (c *ControllerConnector) fetchISISText(ctx context.Context, peName string) (string, error) {
	reqBody := isisRequest{}
	reqBody.Input.PeName = peName
	reqBody.Input.Process = 10
	reqBody.Input.Verbose = true
	reqBody.Input.Scope = "isis"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal isis request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/restconf/operations/oper-rpc:isis-neighbor", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("fetch isis text for %s: %w", peName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch isis text for %s: status %d", peName, resp.StatusCode)
	}

	var restResp restconfResponse
	if err := json.NewDecoder(resp.Body).Decode(&restResp); err != nil {
		return "", fmt.Errorf("decode isis response for %s: %w", peName, err)
	}

	return restResp.Output.ISISNeighborResult, nil
}

// bgpRequest BGP 接口请求体。
type bgpRequest struct {
	Input struct {
		PeName string `json:"pe-name"`
		Scope  string `json:"scope"`
	} `json:"input"`
}

// fetchBGPText 调用单台设备的 BGP 邻居接口，返回回显文本。
func (c *ControllerConnector) fetchBGPText(ctx context.Context, peName string) (string, error) {
	reqBody := bgpRequest{}
	reqBody.Input.PeName = peName
	reqBody.Input.Scope = "IPv4"

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal bgp request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/restconf/operations/oper-rpc:bgp-peer-config", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("fetch bgp text for %s: %w", peName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch bgp text for %s: status %d", peName, resp.StatusCode)
	}

	var restResp restconfResponse
	if err := json.NewDecoder(resp.Body).Decode(&restResp); err != nil {
		return "", fmt.Errorf("decode bgp response for %s: %w", peName, err)
	}

	return restResp.Output.CurrentConfigResult, nil
}

// ──────────────────────────────
// Interface 提取（从 Device 响应）
// ──────────────────────────────

// extractInterfaces 从 Device 响应中提取嵌套的 Interface 数据。
func extractInterfaces(deviceName string, raw map[string]any) []connector.Resource {
	var resources []connector.Resource

	peports, ok := raw["peports"].(map[string]any)
	if !ok {
		return resources
	}

	peportInfo, ok := peports["peport-info"].([]any)
	if !ok {
		return resources
	}

	for _, item := range peportInfo {
		port, ok := item.(map[string]any)
		if !ok {
			continue
		}
		props := transformInterface(deviceName, port)
		id, _ := port["id"].(string)
		resources = append(resources, connector.Resource{
			Kind:       "Interface",
			ID:         id,
			Properties: props,
		})
	}

	return resources
}
