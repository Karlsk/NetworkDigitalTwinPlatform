// Package controller 实现 Controller Connector 及统一 API 适配层。
// api_slice.go 定义切片管理相关 API 方法集（文档第 8 章：切片管理）。
// 覆盖 FlexE Group / FlexE Client / 信道化子接口 / SRv6 网络切片的 CRUD 操作。
package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ──────────────────────────────
// FlexE Group CRUD（文档 8.1-8.4）
// ──────────────────────────────

// CreateFlexEGroup 创建 FlexE Group。
// API: POST /api/no/config/terra-flexe:flexe/flexe-group
func (c *ControllerClient) CreateFlexEGroup(ctx context.Context, body map[string]any) (map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("create flexe group: %w", err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal flexe group request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/api/no/config/terra-flexe:flexe/flexe-group", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create flexe group: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create flexe group: status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode create flexe group response: %w", err)
	}
	return result, nil
}

// ListFlexEGroups 查询 FlexE Group 列表。
// API: GET /api/no/config/terra-flexe:flexe/flexe-group?deviceName=&dstDeviceName=
func (c *ControllerClient) ListFlexEGroups(ctx context.Context, device, dstDevice string) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("list flexe groups: %w", err)
	}

	params := url.Values{}
	if device != "" {
		params.Set("deviceName", device)
	}
	if dstDevice != "" {
		params.Set("dstDeviceName", dstDevice)
	}

	path := "/api/no/config/terra-flexe:flexe/flexe-group"
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}

	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list flexe groups: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list flexe groups: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode list flexe groups response: %w", err)
	}
	return result, nil
}

// DeleteFlexEGroup 删除 FlexE Group。
// API: DELETE /api/no/config/terra-flexe:flexe/flexe-group/{id}/
func (c *ControllerClient) DeleteFlexEGroup(ctx context.Context, id string) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("delete flexe group: %w", err)
	}

	path := fmt.Sprintf("/api/no/config/terra-flexe:flexe/flexe-group/%s/", url.PathEscape(id))
	resp, err := c.http.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete flexe group %s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete flexe group %s: status %d", id, resp.StatusCode)
	}
	return nil
}

// UpdateFlexEGroup 更新 FlexE Group。
// API: PUT /api/no/config/terra-flexe:flexe/flexe-group/
func (c *ControllerClient) UpdateFlexEGroup(ctx context.Context, body map[string]any) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("update flexe group: %w", err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal flexe group update: %w", err)
	}

	resp, err := c.http.Put(ctx, "/api/no/config/terra-flexe:flexe/flexe-group/", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("update flexe group: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("update flexe group: status %d", resp.StatusCode)
	}
	return nil
}

// ──────────────────────────────
// FlexE Client CRUD（文档 8.5-8.9）
// ──────────────────────────────

// CreateFlexEClient 创建 FlexE Client。
// API: POST /api/no/config/terra-flexe:flexe-interfaces/flexe-interface
func (c *ControllerClient) CreateFlexEClient(ctx context.Context, body map[string]any) (map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("create flexe client: %w", err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal flexe client request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/api/no/config/terra-flexe:flexe-interfaces/flexe-interface", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create flexe client: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create flexe client: status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode create flexe client response: %w", err)
	}
	return result, nil
}

// ListFlexEClients 查询 FlexE Client 列表。
// API: GET /api/no/config/terra-flexe:flexe-interfaces/flexe-interface/getByGroup/{groupId}
func (c *ControllerClient) ListFlexEClients(ctx context.Context, groupID string) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("list flexe clients: %w", err)
	}

	path := fmt.Sprintf("/api/no/config/terra-flexe:flexe-interfaces/flexe-interface/getByGroup/%s", url.PathEscape(groupID))
	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list flexe clients for group %s: %w", groupID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list flexe clients for group %s: status %d", groupID, resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode list flexe clients response: %w", err)
	}
	return result, nil
}

// DeleteFlexEClient 删除 FlexE Client。
// API: DELETE /api/no/config/terra-flexe:flexe-interfaces/flexe-interface/{id}
func (c *ControllerClient) DeleteFlexEClient(ctx context.Context, id string) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("delete flexe client: %w", err)
	}

	path := fmt.Sprintf("/api/no/config/terra-flexe:flexe-interfaces/flexe-interface/%s", url.PathEscape(id))
	resp, err := c.http.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete flexe client %s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete flexe client %s: status %d", id, resp.StatusCode)
	}
	return nil
}

// UpdateFlexEClient 更新 FlexE Client。
// API: PUT /api/no/config/terra-flexe:flexe-interfaces/flexe-interface/
func (c *ControllerClient) UpdateFlexEClient(ctx context.Context, body map[string]any) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("update flexe client: %w", err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal flexe client update: %w", err)
	}

	resp, err := c.http.Put(ctx, "/api/no/config/terra-flexe:flexe-interfaces/flexe-interface/", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("update flexe client: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("update flexe client: status %d", resp.StatusCode)
	}
	return nil
}

// DownloadPortInfo 下载设备所有端口信息。
// API: GET /api/no/config/terra-flexe:flexe/download/txt
func (c *ControllerClient) DownloadPortInfo(ctx context.Context) (string, error) {
	if err := c.ensureToken(ctx); err != nil {
		return "", fmt.Errorf("download port info: %w", err)
	}

	resp, err := c.http.Get(ctx, "/api/no/config/terra-flexe:flexe/download/txt")
	if err != nil {
		return "", fmt.Errorf("download port info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download port info: status %d", resp.StatusCode)
	}

	// 响应为纯文本
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return "", fmt.Errorf("read port info response: %w", err)
	}
	return buf.String(), nil
}

// ──────────────────────────────
// 信道化子接口 CRUD（文档 8.10-8.13）
// ──────────────────────────────

// CreateSubInterfaceSlicing 创建信道化子接口。
// API: POST /api/no/config/terra-slicing:sub-interfaces-slicing/sub-interface-slicing
func (c *ControllerClient) CreateSubInterfaceSlicing(ctx context.Context, body map[string]any) (map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("create sub-interface slicing: %w", err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal sub-interface slicing request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/api/no/config/terra-slicing:sub-interfaces-slicing/sub-interface-slicing", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create sub-interface slicing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create sub-interface slicing: status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode create sub-interface slicing response: %w", err)
	}
	return result, nil
}

// ListSubInterfaceSlicings 查询信道化子接口列表。
// API: GET /api/no/config/terra-slicing:sub-interfaces-slicing/sub-interface/getAll
func (c *ControllerClient) ListSubInterfaceSlicings(ctx context.Context, device, dstDevice string) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("list sub-interface slicings: %w", err)
	}

	params := url.Values{}
	if device != "" {
		params.Set("deviceName", device)
	}
	if dstDevice != "" {
		params.Set("dstDeviceName", dstDevice)
	}

	path := "/api/no/config/terra-slicing:sub-interfaces-slicing/sub-interface/getAll"
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}

	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list sub-interface slicings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list sub-interface slicings: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode list sub-interface slicings response: %w", err)
	}
	return result, nil
}

// UpdateSubInterfaceSlicing 更新信道化子接口。
// API: PUT /api/no/config/terra-slicing:sub-interfaces-slicing/update/
func (c *ControllerClient) UpdateSubInterfaceSlicing(ctx context.Context, body map[string]any) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("update sub-interface slicing: %w", err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal sub-interface slicing update: %w", err)
	}

	resp, err := c.http.Put(ctx, "/api/no/config/terra-slicing:sub-interfaces-slicing/update/", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("update sub-interface slicing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("update sub-interface slicing: status %d", resp.StatusCode)
	}
	return nil
}

// DeleteSubInterfaceSlicing 删除信道化子接口。
// API: DELETE /api/no/config/terra-slicing:sub-interfaces-slicing/sub-interface-slicing/{id}
func (c *ControllerClient) DeleteSubInterfaceSlicing(ctx context.Context, id string) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("delete sub-interface slicing: %w", err)
	}

	path := fmt.Sprintf("/api/no/config/terra-slicing:sub-interfaces-slicing/sub-interface-slicing/%s", url.PathEscape(id))
	resp, err := c.http.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete sub-interface slicing %s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete sub-interface slicing %s: status %d", id, resp.StatusCode)
	}
	return nil
}

// ──────────────────────────────
// SRv6 网络切片 CRUD（文档 8.14-8.17）
// ──────────────────────────────

// CreateSRv6Slice 创建 SRv6 网络切片。
// API: POST /api/no/config/terra-slicing:srv6-network-slices/srv6-network-slice
func (c *ControllerClient) CreateSRv6Slice(ctx context.Context, body map[string]any) (map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("create srv6 slice: %w", err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal srv6 slice request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/api/no/config/terra-slicing:srv6-network-slices/srv6-network-slice", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create srv6 slice: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create srv6 slice: status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode create srv6 slice response: %w", err)
	}
	return result, nil
}

// ListSRv6Slices 查询 SRv6 网络切片列表。
// API: GET /api/no/config/terra-slicing:srv6-network-slices/srv6-network-slice?sliceId=&device=
func (c *ControllerClient) ListSRv6Slices(ctx context.Context, sliceID, device string) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("list srv6 slices: %w", err)
	}

	params := url.Values{}
	if sliceID != "" {
		params.Set("sliceId", sliceID)
	}
	if device != "" {
		params.Set("device", device)
	}

	path := "/api/no/config/terra-slicing:srv6-network-slices/srv6-network-slice"
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}

	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list srv6 slices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list srv6 slices: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode list srv6 slices response: %w", err)
	}
	return result, nil
}

// UpdateSRv6Slice 更新 SRv6 网络切片。
// API: PUT /api/no/config/terra-slicing:srv6-network-slices/update
func (c *ControllerClient) UpdateSRv6Slice(ctx context.Context, body map[string]any) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("update srv6 slice: %w", err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal srv6 slice update: %w", err)
	}

	resp, err := c.http.Put(ctx, "/api/no/config/terra-slicing:srv6-network-slices/update", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("update srv6 slice: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("update srv6 slice: status %d", resp.StatusCode)
	}
	return nil
}

// DeleteSRv6Slice 删除 SRv6 网络切片。
// API: DELETE /api/no/config/terra-slicing:srv6-network-slices/srv6-network-slice/{id}
func (c *ControllerClient) DeleteSRv6Slice(ctx context.Context, id string) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("delete srv6 slice: %w", err)
	}

	path := fmt.Sprintf("/api/no/config/terra-slicing:srv6-network-slices/srv6-network-slice/%s", url.PathEscape(id))
	resp, err := c.http.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete srv6 slice %s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete srv6 slice %s: status %d", id, resp.StatusCode)
	}
	return nil
}
