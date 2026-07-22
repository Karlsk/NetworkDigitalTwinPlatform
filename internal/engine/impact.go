// Package engine 实现分析引擎 (纯算法)
package engine

import (
	"context"
	"errors"
)

// ErrNotImplemented 表示功能尚未实现。
var ErrNotImplemented = errors.New("engine: not implemented")

// ImpactResult 影响分析结果。
type ImpactResult struct {
	SourceNode    string         // 源节点 URI
	AffectedNodes []AffectedNode // 受影响节点列表
	AffectedRels  []AffectedRel  // 受影响关系列表
	MaxDepth      int            // 分析深度
}

// AffectedNode 受影响节点。
type AffectedNode struct {
	URI      string // 节点 URI
	Label    string // 节点标签
	Distance int    // 距源节点距离
}

// AffectedRel 受影响关系。
type AffectedRel struct {
	Type string // 关系类型
	From string // 起始节点 URI
	To   string // 终止节点 URI
}

// ImpactEngine 影响分析引擎。
// V1 实现图遍历算法（BFS/DFS），当前仅有接口骨架。
type ImpactEngine struct{}

// NewImpactEngine 创建 ImpactEngine 实例。
func NewImpactEngine() *ImpactEngine {
	return &ImpactEngine{}
}

// Analyze 分析指定节点的影响范围。
// nodeURI: 源节点 URI，depth: 分析深度。
func (e *ImpactEngine) Analyze(ctx context.Context, nodeURI string, depth int) (*ImpactResult, error) {
	return nil, ErrNotImplemented
}
