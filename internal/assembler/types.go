// Package assembler 实现 GraphAssembler (IR 层)，
// 负责将 NormalizedResource 转换为 GraphModel。
// Node 和 Relation 是 GraphModel IR 的核心数据类型，
// 被 GraphDB 驱动层消费。
package assembler

// Node 图节点（GraphModel IR）。
// 由 GraphAssembler 产出，被 GraphDB 消费。
// Labels 对应 EntityType 继承链（如 ["Resource", "Device"]），从基类到具体类，
// URI 由 uriTemplate + stableKeys 生成，是节点在逻辑 DB 内的唯一标识。
type Node struct {
	Labels []string     // 节点标签列表，如 ["Resource", "Device"]
	URI    string       // 唯一资源标识符
	Props  map[string]any // 节点属性
}

// NewNode 创建单标签节点的便捷构造函数，保持向后兼容。
// V1-15 本体继承体系引入后，Labels 可能包含多个元素。
func NewNode(label string, uri string, props map[string]any) Node {
	return Node{Labels: []string{label}, URI: uri, Props: props}
}

// MostSpecificLabel 返回最具体的标签（最后一个）。
// 用于 Cypher MERGE/CREATE 按最具体 Label 分组，以及查询/展示场景。
func (n Node) MostSpecificLabel() string {
	if len(n.Labels) == 0 {
		return ""
	}
	return n.Labels[len(n.Labels)-1]
}

// Relation 图边（GraphModel IR）。
// 由 GraphAssembler 根据 EntityType.relationFields 推导产出，
// 被 GraphDB 消费。
type Relation struct {
	Type  string         // 关系类型，如 "HAS_INTERFACE"
	From  string         // 源节点 URI
	To    string         // 目标节点 URI
	Props map[string]any // 关系属性（通常为空）
}

// GraphModel 纯图元素的中间表示，与数据源和 Schema 完全解耦。
// 由 GraphAssembler 产出，被 GraphDB 消费。
type GraphModel struct {
	Nodes     []Node     // 图节点集合
	Relations []Relation // 图边集合
}

// ValidationWarning 图模型校验警告。
// GraphAssembler 在组装过程中发现孤儿边等异常时产出，
// 不阻断同步流程，由 SyncResult 汇总上报。
type ValidationWarning struct {
	Type   string // 警告类型，如 "orphan_edge"
	Detail string // 警告详情，如 "HAS_INTERFACE: device:SN12345 → iface:SN12345_GE1/0/2"
}
