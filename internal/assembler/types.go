// Package assembler 实现 GraphAssembler (IR 层)，
// 负责将 NormalizedResource 转换为 GraphModel。
// Node 和 Relation 是 GraphModel IR 的核心数据类型，
// 被 GraphDB 驱动层消费。
package assembler

// Node 图节点（GraphModel IR）。
// 由 GraphAssembler 产出，被 GraphDB 消费。
// Label 对应 EntityType 名称（如 "Device"、"Interface"），
// URI 由 uriTemplate + stableKeys 生成，是节点在逻辑 DB 内的唯一标识。
type Node struct {
	Label string         // 节点标签，如 "Device"
	URI   string         // 唯一资源标识符
	Props map[string]any // 节点属性
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
