// Package controller 实现 Controller Connector 及统一 API 适配层。
// api_detnet.go 定义确定性网络（DetNet）相关 API（文档第 7 章：确定性网络北向接口）。
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
// 确定性网络 DetNet（文档第 7 章）
// ──────────────────────────────

// ListDetNetInstances 查询全部确定性路径探测实例。
// API: GET /api/no/config/terra-h3c-detnet/ip/service/all
func (c *ControllerClient) ListDetNetInstances(
	ctx context.Context,
) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("list detnet instances: %w", err)
	}

	resp, err := c.http.Get(ctx, "/api/no/config/terra-h3c-detnet/ip/service/all")
	if err != nil {
		return nil, fmt.Errorf("list detnet instances: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list detnet instances: status %d", resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode list detnet instances response: %w", err)
	}
	return result, nil
}

// CreateDetNetInstance 创建确定性路径探测实例。
// API: POST /api/no/config/terra-h3c-detnet/ip/service
func (c *ControllerClient) CreateDetNetInstance(
	ctx context.Context, body map[string]any,
) (map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("create detnet instance: %w", err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal detnet instance request: %w", err)
	}

	resp, err := c.http.PostJSON(ctx, "/api/no/config/terra-h3c-detnet/ip/service", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create detnet instance: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create detnet instance: status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode create detnet instance response: %w", err)
	}
	return result, nil
}

// UpdateDetNetInstance 更新确定性路径探测实例。
// API: PUT /api/no/config/terra-h3c-detnet/ip/service/{id}
func (c *ControllerClient) UpdateDetNetInstance(
	ctx context.Context, id string, body map[string]any,
) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("update detnet instance: %w", err)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal detnet instance update: %w", err)
	}

	path := fmt.Sprintf("/api/no/config/terra-h3c-detnet/ip/service/%s", url.PathEscape(id))
	resp, err := c.http.Put(ctx, path, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("update detnet instance %s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("update detnet instance %s: status %d", id, resp.StatusCode)
	}
	return nil
}

// DeleteDetNetInstance 删除确定性路径探测实例。
// API: DELETE /api/no/config/terra-h3c-detnet/ip/service/{id}
func (c *ControllerClient) DeleteDetNetInstance(
	ctx context.Context, id string,
) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("delete detnet instance: %w", err)
	}

	path := fmt.Sprintf("/api/no/config/terra-h3c-detnet/ip/service/%s", url.PathEscape(id))
	resp, err := c.http.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete detnet instance %s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete detnet instance %s: status %d", id, resp.StatusCode)
	}
	return nil
}

// FetchDetNetOAMData 查询探测实例的 OAM 探测数据。
// API: GET /api/no/config/terra-h3c-detnet/ip/service/oam?id={id}&interval-minutes={interval}
func (c *ControllerClient) FetchDetNetOAMData(
	ctx context.Context, instanceID string, intervalMinutes int,
) ([]map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch detnet oam data: %w", err)
	}

	params := url.Values{}
	params.Set("id", instanceID)
	params.Set("interval-minutes", fmt.Sprintf("%d", intervalMinutes))
	path := "/api/no/config/terra-h3c-detnet/ip/service/oam?" + params.Encode()

	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch detnet oam data for %s: %w", instanceID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch detnet oam data for %s: status %d", instanceID, resp.StatusCode)
	}

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode detnet oam data response: %w", err)
	}
	return result, nil
}

// RestartDetNetTimeslot 重启探测实例的时隙计算。
// API: POST /api/no/config/terra-h3c-detnet/ip/service/restart-timeslot-calculation?id={id}
func (c *ControllerClient) RestartDetNetTimeslot(
	ctx context.Context, instanceID string,
) error {
	if err := c.ensureToken(ctx); err != nil {
		return fmt.Errorf("restart detnet timeslot: %w", err)
	}

	path := fmt.Sprintf("/api/no/config/terra-h3c-detnet/ip/service/restart-timeslot-calculation?id=%s", url.QueryEscape(instanceID))
	resp, err := c.http.PostJSON(ctx, path, nil)
	if err != nil {
		return fmt.Errorf("restart detnet timeslot for %s: %w", instanceID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("restart detnet timeslot for %s: status %d", instanceID, resp.StatusCode)
	}
	return nil
}
