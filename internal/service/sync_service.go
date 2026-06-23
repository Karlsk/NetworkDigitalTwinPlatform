// Package service 实现业务编排层
package service

import (
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
)

// SyncResult 同步结果统计。
// 由 SyncService.FullSync / IncrementalSync 产出，
// 上报给 MCP 层或日志系统。
type SyncResult struct {
	NodesCreated       int
	RelationsCreated   int
	OrphanEdgesSkipped int                           // 孤儿边计数 (可观测)
	Warnings           []assembler.ValidationWarning // 校验警告
	Duration           time.Duration
}

// SyncEvent 同步事件 (Webhook 触发)。
// 由外部系统通过 Webhook 推送，经 Channel 缓冲后由 SyncService 串行消费。
// Action 支持三种值: "update", "delete", "delete_relation"。
type SyncEvent struct {
	Action     string               // "update", "delete", "delete_relation"
	EntityType string               // 实体类型
	Connector  string               // 连接器名称
	Data       []map[string]any     // update 时的数据 (Webhook 原始 JSON)
	URIs       []string             // delete 时的 URI 列表
	Relations  []assembler.Relation // delete_relation 时的关系列表
}
