// Package service 实现业务编排层
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/graph"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
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

// SyncService 同步服务编排层。
// 编排 Connector → Normalizer → GraphAssembler → GraphDB 的完整数据流。
// 通过 GraphLock 与 SnapshotManager 共享并发保护。
type SyncService struct {
	registry   *connector.ConnectorRegistry
	normalizer *normalizer.Normalizer
	assembler  *assembler.GraphAssembler
	graph      graph.GraphDB
	lock       *snapshot.GraphLock
	eventChan  chan SyncEvent
}

// NewSyncService 创建 SyncService 实例。
// bufferSize 控制 Webhook 事件缓冲 channel 容量。
func NewSyncService(
	registry *connector.ConnectorRegistry,
	norm *normalizer.Normalizer,
	asm *assembler.GraphAssembler,
	gdb graph.GraphDB,
	lock *snapshot.GraphLock,
	bufferSize int,
) *SyncService {
	return &SyncService{
		registry:   registry,
		normalizer: norm,
		assembler:  asm,
		graph:      gdb,
		lock:       lock,
		eventChan:  make(chan SyncEvent, bufferSize),
	}
}

// FullSync 全量同步：持有写锁 → ClearDB → 全量拉取 → Normalizer → Assembler → BulkCreate。
// 单个 Connector/Normalizer 失败不阻断整个同步（容错策略）。
func (s *SyncService) FullSync(ctx context.Context) (*SyncResult, error) {
	start := time.Now()

	// 1. 持有写锁（defer 确保异常时也释放）
	s.lock.Lock()
	defer s.lock.Unlock()

	// 2. ClearDB
	if err := s.graph.ClearDB(ctx, "default"); err != nil {
		return nil, fmt.Errorf("clear db: %w", err)
	}

	// 3. 全量拉取所有 Connector 的所有实体
	var allResources []connector.Resource
	for _, meta := range s.registry.List() {
		conn, err := s.registry.Get(meta.Name)
		if err != nil {
			slog.Error("get connector failed", "connector", meta.Name, "error", err)
			continue
		}
		for _, et := range meta.EntityTypes {
			resources, err := conn.Collect(ctx, et)
			if err != nil {
				slog.Error("collect failed", "connector", meta.Name, "entityType", et, "error", err)
				continue // 单个 Connector 失败不阻断其他
			}
			allResources = append(allResources, resources...)
		}
	}

	// 4. 归一化
	var allNormalized []normalizer.NormalizedResource
	for _, r := range allResources {
		norm, err := s.normalizer.Normalize(r)
		if err != nil {
			slog.Warn("normalize failed", "kind", r.Kind, "id", r.ID, "error", err)
			continue
		}
		allNormalized = append(allNormalized, *norm)
	}

	// 5. 组装图模型
	model, warnings, err := s.assembler.Assemble(allNormalized)
	if err != nil {
		return nil, fmt.Errorf("assemble graph: %w", err)
	}

	// 6. 批量写入 Neo4j
	if err := s.graph.BulkCreate(ctx, "default", model.Nodes, model.Relations); err != nil {
		return nil, fmt.Errorf("bulk create: %w", err)
	}

	slog.Info("full sync completed",
		"nodes", len(model.Nodes),
		"relations", len(model.Relations),
		"orphan_edges", len(warnings),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return &SyncResult{
		NodesCreated:       len(model.Nodes),
		RelationsCreated:  len(model.Relations),
		OrphanEdgesSkipped: len(warnings),
		Warnings:           warnings,
		Duration:           time.Since(start),
	}, nil
}
