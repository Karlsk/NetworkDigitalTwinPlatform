// Package service 实现业务编排层
package service

import (
	"context"
	"fmt"
	"log/slog"

	"gitlab.com/pml/network-digital-twin/internal/graph"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// TopologyResult 拓扑查询结果。
type TopologyResult struct {
	Nodes []map[string]any // 查询返回的节点数据
	Count int              // 节点数量
}

// AnalysisService 分析服务编排层。
// 封装图数据库查询操作，内部管理读锁（RLock），对外提供拓扑查询能力。
type AnalysisService struct {
	graph graph.GraphDB
	lock  *snapshot.GraphLock
}

// NewAnalysisService 创建 AnalysisService 实例。
func NewAnalysisService(gdb graph.GraphDB, lock *snapshot.GraphLock) *AnalysisService {
	return &AnalysisService{
		graph: gdb,
		lock:  lock,
	}
}

// QueryTopology 查询网络拓扑数据。
// 内部管理 RLock，构建 Cypher 并调用 graph.Query。
// label 为空时默认 "Device"，limit <= 0 时默认 100。
func (s *AnalysisService) QueryTopology(ctx context.Context, label string, limit int) (*TopologyResult, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if label == "" {
		label = "Device"
	}
	if limit <= 0 {
		limit = 100
	}

	cypher := fmt.Sprintf(
		"MATCH (n:%s) WHERE n._db = $_db RETURN n LIMIT %d", label, limit,
	)
	rows, err := s.graph.Query(ctx, "default", cypher, map[string]any{
		"_db": "default",
	})
	if err != nil {
		return nil, fmt.Errorf("query topology: %w", err)
	}

	slog.Info("query_topology completed", "label", label, "count", len(rows))
	return &TopologyResult{Nodes: rows, Count: len(rows)}, nil
}
