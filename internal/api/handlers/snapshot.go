package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// snapshotService 快照服务接口（薄接口，解耦 Handler 与具体实现）。
type snapshotService interface {
	List(ctx context.Context) ([]snapshot.SnapshotMeta, error)
	Diff(ctx context.Context, a, b string) (*snapshot.SnapshotDiff, error)
	Restore(ctx context.Context, name string) error
	Create(ctx context.Context, name string) (snapshot.SnapshotMeta, error)
	Delete(ctx context.Context, name string) error
	AuditQuery(filter snapshot.AuditFilter) []snapshot.AuditEntry
	AuditRecent(n int) []snapshot.AuditEntry
}

// SnapshotHandler 快照相关请求处理器。
type SnapshotHandler struct {
	svc snapshotService
}

// NewSnapshotHandler 创建 SnapshotHandler。
func NewSnapshotHandler(svc snapshotService) *SnapshotHandler {
	return &SnapshotHandler{svc: svc}
}

// snapshotItem 快照列表项（API 响应）。
type snapshotItem struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	NodeCount int    `json:"node_count"`
	RelCount  int    `json:"rel_count"`
}

// ListSnapshots 列出所有快照。
//
// @Summary 列出所有快照
// @Description 返回当前所有快照元数据
// @Tags snapshot
// @Produce json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/snapshot [get]
//
// GET /api/v1/snapshot
func (h *SnapshotHandler) ListSnapshots(c *gin.Context) {
	metas, err := h.svc.List(c.Request.Context())
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, err.Error())
		return
	}

	var list []snapshotItem
	for _, m := range metas {
		list = append(list, snapshotItem{
			Name:      m.Name,
			CreatedAt: m.CreatedAt.Format(time.RFC3339),
			NodeCount: m.NodeCount,
			RelCount:  m.RelCount,
		})
	}
	response.OK(c, gin.H{"snapshots": list})
}

// CreateSnapshotRequest 创建快照请求。
type CreateSnapshotRequest struct {
	Name string `json:"name" binding:"required"`
}

// CreateSnapshot 创建快照。
//
// @Summary 创建快照
// @Description 创建新的图快照
// @Tags snapshot
// @Accept json
// @Produce json
// @Success 201 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/snapshot [post]
//
// POST /api/v1/snapshot
func (h *SnapshotHandler) CreateSnapshot(c *gin.Context) {
	var req CreateSnapshotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeSnapshotBadRequest, "name is required")
		return
	}

	meta, err := h.svc.Create(c.Request.Context(), req.Name)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, err.Error())
		return
	}

	response.Created(c, gin.H{
		"name":       meta.Name,
		"node_count": meta.NodeCount,
		"rel_count":  meta.RelCount,
	})
}

// DeleteSnapshot 删除快照。
//
// @Summary 删除快照
// @Description 根据名称删除指定快照
// @Tags snapshot
// @Produce json
// @Param name path string true "snapshot name"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/snapshot/{name} [delete]
//
// DELETE /api/v1/snapshot/:name
func (h *SnapshotHandler) DeleteSnapshot(c *gin.Context) {
	name := c.Param("name")
	if err := h.svc.Delete(c.Request.Context(), name); err != nil {
		response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "deleted", "name": name})
}

// RestoreSnapshotRequest 恢复快照请求。
type RestoreSnapshotRequest struct {
	Name string `json:"name" binding:"required"`
}

// RestoreSnapshot 恢复快照。
//
// @Summary 恢复快照
// @Description 根据名称恢复指定快照
// @Tags snapshot
// @Accept json
// @Produce json
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/snapshot/restore [post]
//
// POST /api/v1/snapshot/restore
func (h *SnapshotHandler) RestoreSnapshot(c *gin.Context) {
	var req RestoreSnapshotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeSnapshotBadRequest, "name is required")
		return
	}

	if err := h.svc.Restore(c.Request.Context(), req.Name); err != nil {
		response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "restored", "name": req.Name})
}

// DiffSnapshots 对比两个快照。
//
// @Summary 对比快照
// @Description 对比两个快照之间的差异
// @Tags snapshot
// @Produce json
// @Param a query string true "snapshot A name"
// @Param b query string true "snapshot B name"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/v1/snapshot/diff [get]
//
// GET /api/v1/snapshot/diff?a=snap1&b=snap2
func (h *SnapshotHandler) DiffSnapshots(c *gin.Context) {
	a, b := c.Query("a"), c.Query("b")
	if a == "" || b == "" {
		response.Fail(c, http.StatusBadRequest, response.CodeSnapshotBadRequest,
			"query params a and b are required")
		return
	}

	diff, err := h.svc.Diff(c.Request.Context(), a, b)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, response.CodeInternalError, err.Error())
		return
	}

	response.OK(c, gin.H{
		"added_nodes":   len(diff.AddedNodes),
		"removed_nodes": len(diff.RemovedNodes),
		"added_rels":    len(diff.AddedRels),
		"removed_rels":  len(diff.RemovedRels),
		"changed_nodes": len(diff.ChangedNodes),
		"changed_rels":  len(diff.ChangedRels),
	})
}

// QueryAudit 查询审计日志。
//
// @Summary 查询审计日志
// @Description 查询快照操作审计日志
// @Tags audit
// @Produce json
// @Param limit query int false "max items"
// @Param action query string false "action filter"
// @Param snapshot query string false "snapshot filter"
// @Success 200 {object} response.Response
// @Router /api/v1/audit [get]
//
// GET /api/v1/audit?limit=50&action=create&snapshot=snap1
func (h *SnapshotHandler) QueryAudit(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 50
	}

	action := c.Query("action")
	snapName := c.Query("snapshot")

	var entries []snapshot.AuditEntry
	if action != "" || snapName != "" {
		filter := snapshot.AuditFilter{
			Action:   action,
			Snapshot: snapName,
		}
		entries = h.svc.AuditQuery(filter)
	} else {
		entries = h.svc.AuditRecent(limit)
	}

	response.OK(c, gin.H{"audit": entries, "count": len(entries)})
}
