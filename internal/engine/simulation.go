package engine

import "context"

// OperationPlan 操作计划。
type OperationPlan struct {
	TargetURI string            // 目标节点 URI
	Action    string            // 操作类型 (e.g. "disable", "enable", "update")
	Params    map[string]string // 操作参数
}

// SimulationResult 仿真结果。
type SimulationResult struct {
	ImpactedNodes  []string // 受影响节点 URI 列表
	RiskScore      float64  // 风险评分 (0.0~1.0)
	Recommendation string   // 建议
}

// SimulationEngine 仿真引擎。
// V1 实现基于图影响域的仿真算法，当前仅有接口骨架。
type SimulationEngine struct{}

// NewSimulationEngine 创建 SimulationEngine 实例。
func NewSimulationEngine() *SimulationEngine {
	return &SimulationEngine{}
}

// Simulate 模拟执行操作计划并评估影响。
func (e *SimulationEngine) Simulate(ctx context.Context, plan OperationPlan) (*SimulationResult, error) {
	return nil, ErrNotImplemented
}
