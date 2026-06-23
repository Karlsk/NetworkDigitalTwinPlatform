// Package snapshot 实现快照管理
package snapshot

import (
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
)

// SnapshotMeta 快照元数据。
// 由 SnapshotManager.Create 产出，用于 List / Delete / Restore 操作。
type SnapshotMeta struct {
	Name      string    // 快照名称
	CreatedAt time.Time // 创建时间
	NodeCount int       // 节点数
	RelCount  int       // 关系数
	FilePath  string    // YAML 归档文件路径
}

// SnapshotDiff 快照对比结果。
// 由 SnapshotManager.Diff 产出，用于两个快照之间的差异分析。
type SnapshotDiff struct {
	AddedNodes   []assembler.Node     // 新增节点
	RemovedNodes []assembler.Node     // 删除节点
	AddedRels    []assembler.Relation // 新增关系
	RemovedRels  []assembler.Relation // 删除关系
}
