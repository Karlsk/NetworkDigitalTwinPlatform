package handlers

import (
	"github.com/gin-gonic/gin"

	"gitlab.com/pml/network-digital-twin/internal/api/response"
	"gitlab.com/pml/network-digital-twin/internal/service"
)

// SnapshotHandler 快照相关请求处理器。
type SnapshotHandler struct {
	Svc *service.SnapshotService
}

// ListSnapshots 列出所有快照。
// GET /api/v1/snapshot
func (h *SnapshotHandler) ListSnapshots(c *gin.Context) {
	response.NotImplemented(c, "snapshot list endpoint not yet implemented, coming in V2-12")
}

// CreateSnapshot 创建快照。
// POST /api/v1/snapshot
func (h *SnapshotHandler) CreateSnapshot(c *gin.Context) {
	response.NotImplemented(c, "snapshot create endpoint not yet implemented, coming in V2-12")
}

// DeleteSnapshot 删除快照。
// DELETE /api/v1/snapshot/:name
func (h *SnapshotHandler) DeleteSnapshot(c *gin.Context) {
	response.NotImplemented(c, "snapshot delete endpoint not yet implemented, coming in V2-12")
}

// RestoreSnapshot 恢复快照。
// POST /api/v1/snapshot/restore
func (h *SnapshotHandler) RestoreSnapshot(c *gin.Context) {
	response.NotImplemented(c, "snapshot restore endpoint not yet implemented, coming in V2-12")
}
