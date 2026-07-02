package service

import (
	"context"
	"log/slog"

	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// SnapshotService 快照服务编排层。
// 薄封装 *snapshot.SnapshotManager，不额外管理锁（Manager 内部已管理）。
type SnapshotService struct {
	manager *snapshot.SnapshotManager
}

// NewSnapshotService 创建 SnapshotService 实例。
func NewSnapshotService(manager *snapshot.SnapshotManager) *SnapshotService {
	return &SnapshotService{manager: manager}
}

// List 列出所有快照元数据。
func (s *SnapshotService) List(ctx context.Context) ([]snapshot.SnapshotMeta, error) {
	return s.manager.List(ctx)
}

// Diff 对比两个快照的差异。
func (s *SnapshotService) Diff(ctx context.Context, a, b string) (*snapshot.SnapshotDiff, error) {
	return s.manager.Diff(ctx, a, b)
}

// Restore 恢复指定快照到 default 逻辑 DB。
func (s *SnapshotService) Restore(ctx context.Context, name string) error {
	slog.Info("snapshot restore requested", "name", name)
	return s.manager.Restore(ctx, name)
}

// Create 创建快照。
func (s *SnapshotService) Create(ctx context.Context, name string) (snapshot.SnapshotMeta, error) {
	meta, err := s.manager.Create(ctx, name)
	if err != nil {
		return snapshot.SnapshotMeta{}, err
	}
	slog.Info("snapshot created", "name", name, "nodes", meta.NodeCount, "rels", meta.RelCount)
	return meta, nil
}

// Delete 删除指定快照。
func (s *SnapshotService) Delete(ctx context.Context, name string) error {
	return s.manager.Delete(ctx, name)
}

// AuditQuery 按过滤条件查询审计日志。
func (s *SnapshotService) AuditQuery(filter snapshot.AuditFilter) []snapshot.AuditEntry {
	return s.manager.AuditLog().Query(filter)
}

// AuditRecent 返回最近 n 条审计日志。
func (s *SnapshotService) AuditRecent(n int) []snapshot.AuditEntry {
	return s.manager.AuditLog().Recent(n)
}
