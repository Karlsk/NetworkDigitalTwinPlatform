// Package normalizer 实现归一化引擎
package normalizer

// NormalizedResource 归一化后的节点记录 (不含关系)。
// 由 Normalizer 产出，被 GraphAssembler 消费。
// 关系字段保留在 Properties 中，由 GraphAssembler 读取 relationFields 映射后推导为图边。
type NormalizedResource struct {
	Kind       string         // 实体类型，如 "Device"
	URI        string         // 标准本体 URI (由 uriTemplate 生成)
	Properties map[string]any // 标准化后的属性（类型已校验，含原始关系字段）
}
