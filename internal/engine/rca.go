package engine

import "context"

// Alarm 告警信息。
type Alarm struct {
	SourceURI string // 告警源节点 URI
	Type      string // 告警类型
	Severity  string // 告警级别
	Message   string // 告警消息
}

// RCAResult 根因分析结果。
type RCAResult struct {
	RootCause     string   // 根因节点 URI
	Confidence    float64  // 置信度 (0.0~1.0)
	RelatedAlarms []string // 关联告警
}

// RCAEngine 根因分析引擎。
// V1 实现基于图传播的 RCA 算法，当前仅有接口骨架。
type RCAEngine struct{}

// NewRCAEngine 创建 RCAEngine 实例。
func NewRCAEngine() *RCAEngine {
	return &RCAEngine{}
}

// Analyze 对一组告警进行根因分析。
func (e *RCAEngine) Analyze(ctx context.Context, alarms []Alarm) (*RCAResult, error) {
	return nil, ErrNotImplemented
}
