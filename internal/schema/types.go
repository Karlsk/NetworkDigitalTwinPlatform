// Package schema 定义网络本体的数据结构 (K8s CRD 风格)。
// EntityType 和 RelationType 是 Schema Registry 的核心类型，
// 被 Normalizer、GraphAssembler、Validator 等下游模块消费。
package schema

// EntityType 本体实体类型定义 (K8s CRD 风格)。
// 每个 EntityType 对应一个 YAML 本体定义文件，描述一种网络实体的
// 身份标识、URI 模板、字段映射、标准化规则、关系字段和属性定义。
type EntityType struct {
	APIVersion string         `yaml:"apiVersion"` // 必须为 "twin.io/v1"
	Kind       string         `yaml:"kind"`       // 必须为 "EntityType"
	Metadata   Metadata       `yaml:"metadata"`
	Spec       EntityTypeSpec `yaml:"spec"`
}

// EntityTypeSpec 是 EntityType 的详细规格。
type EntityTypeSpec struct {
	Extends        string                       `yaml:"extends"`        // 父类型名称（可空，空字符串表示无继承）
	Identity       IdentitySpec                 `yaml:"identity"`       // 不可变身份标识
	URITemplate    string                       `yaml:"uriTemplate"`    // URI 模板，引用 stableKeys 中的字段
	FieldMapping   map[string]string            `yaml:"fieldMapping"`   // 遗留字段名 → 规范字段名映射
	Normalize      []NormalizeRule              `yaml:"normalize"`      // 字段级字符串转换规则
	RelationFields map[string]RelationFieldSpec `yaml:"relationFields"` // 推导关系的属性字段
	Properties     map[string]PropertySpec      `yaml:"properties"`     // 属性定义
}

// IdentitySpec 定义实体的不可变身份标识。
// StableKeys 中的字段名必须在 Properties 中存在，且标记为 required: true。
// URI 基于这些不可变标识生成，确保设备改名/换 IP 后 URI 不会失效。
type IdentitySpec struct {
	StableKeys []string `yaml:"stableKeys"` // 构成永久身份的不可变属性名列表
}

// NormalizeRule 定义字段级字符串转换规则。
// 用于将原始数据中的字段值标准化为统一格式。
type NormalizeRule struct {
	Field   string `yaml:"field"`   // 目标属性名
	Pattern string `yaml:"pattern"` // 匹配模式
	Replace string `yaml:"replace"` // 替换字符串
}

// RelationFieldSpec 定义属性字段到关系类型的映射。
// 表示 EntityType 的某个属性字段可以推导出特定类型的关系。
type RelationFieldSpec struct {
	RelationType string `yaml:"relationType"` // 关联的 RelationType 名称
}

// PropertySpec 定义 EntityType 中的单个属性。
type PropertySpec struct {
	Type     string   `yaml:"type"`     // 数据类型: "string", "int", "bool"
	Required bool     `yaml:"required"` // 是否必填 (默认 false)
	Enum     []string `yaml:"enum"`     // 允许的枚举值 (空 = 无约束)
	Default  any      `yaml:"default"`  // 默认值 (string, int, 或 bool)
}

// RelationType 关系类型定义 (K8s CRD 风格)。
// 定义两个 EntityType 之间的有向关系。
type RelationType struct {
	APIVersion string           `yaml:"apiVersion"` // 必须为 "twin.io/v1"
	Kind       string           `yaml:"kind"`       // 必须为 "RelationType"
	Metadata   Metadata         `yaml:"metadata"`
	Spec       RelationTypeSpec `yaml:"spec"`
}

// RelationTypeSpec 定义关系的源和目标 EntityType。
type RelationTypeSpec struct {
	Source []string `yaml:"source"` // 允许的源 EntityType 名称列表
	Target []string `yaml:"target"` // 允许的目标 EntityType 名称列表
}

// Metadata 是 EntityType 和 RelationType 共享的元数据结构。
type Metadata struct {
	Name   string   `yaml:"name"`   // 唯一标识名称 (如 "Device", "HAS_INTERFACE")
	Labels []string `yaml:"labels"` // 分类标签 (如 [Resource, Network])
}
