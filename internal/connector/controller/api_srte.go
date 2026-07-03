// Package controller 实现 Controller Connector 及统一 API 适配层。
// api_srte.go 定义 SR-TE 路径管理相关 API（文档第 3/4 章）。
// 查询类方法已实现，写操作（路径计算/策略创建）仅预留接口骨架。
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"gitlab.com/pml/network-digital-twin/internal/connector"
)

// ──────────────────────────────
// SR-TE 路径管理（文档第 3/4 章）
// ──────────────────────────────

// FetchSRTEPathDetail 查询单个 SR-TE 隧道策略详情。
// API: GET /api/sr/config/terra-te-svc:te-policy-instance/{id}
func (c *ControllerClient) FetchSRTEPathDetail(
	ctx context.Context, instanceID string,
) (map[string]any, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("fetch srte path detail: %w", err)
	}

	path := fmt.Sprintf("/api/sr/config/terra-te-svc:te-policy-instance/%s", url.PathEscape(instanceID))
	resp, err := c.http.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch srte path detail for %s: %w", instanceID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch srte path detail for %s: status %d", instanceID, resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode srte path detail response: %w", err)
	}
	return result, nil
}

// ComputeSRTEPath 计算 SR-TE 路径（写操作，预留骨架）。
// 注意：此为写操作，V1.2 仅预留接口，实际实现需评估安全影响。
// V2 引入 HTTP API 后再开放写操作。
func (c *ControllerClient) ComputeSRTEPath(
	_ context.Context, _ map[string]any,
) (map[string]any, error) {
	return nil, connector.ErrNotImplemented
}

// CreateSRTEPolicy 创建 SR-TE 策略（写操作，预留骨架）。
// 注意：此为写操作，V1.2 仅预留接口，实际实现需评估安全影响。
// V2 引入 HTTP API 后再开放写操作。
func (c *ControllerClient) CreateSRTEPolicy(
	_ context.Context, _ map[string]any,
) (map[string]any, error) {
	return nil, connector.ErrNotImplemented
}
