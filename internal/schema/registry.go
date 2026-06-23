// Package schema 实现 Schema Registry，负责加载和管理本体定义
package schema

// SchemaRegistry 本体注册表接口。
// 系统启动时加载 ontology/ 目录下所有 YAML，提供 EntityType 和 RelationType
// 的查询与校验能力。下游消费者包括 Normalizer、GraphAssembler、Validator。
//
// 注意: Get 前缀是本项目的命名例外，为了语义清晰。
type SchemaRegistry interface {
	// Load 加载目录下所有 YAML 文件（支持多文档 YAML）。
	// 目录为空或包含无效 YAML 时返回错误。
	Load(dir string) error

	// GetEntityType 按名称获取 EntityType。
	// 未找到时返回 ErrSchemaNotFound。
	GetEntityType(name string) (*EntityType, error)

	// GetRelationType 按名称获取 RelationType。
	// 未找到时返回 ErrSchemaNotFound。
	GetRelationType(name string) (*RelationType, error)

	// ListEntityTypes 列出所有已注册的 EntityType。
	ListEntityTypes() []*EntityType

	// ListRelationTypes 列出所有已注册的 RelationType。
	ListRelationTypes() []*RelationType

	// Validate 校验数据合法性（属性类型/必填/枚举/stableKeys 非空）。
	// 不修改输入 map，只返回校验错误。
	// 多个校验失败时返回聚合错误（以 "; " 分隔）。
	// entityKind 不存在时返回 ErrSchemaNotFound。
	Validate(entityKind string, props map[string]any) error

	// ApplyDefaults 返回一个新 map，将 schema 中定义的默认值填充到缺失的可选字段。
	// 原始 map 不被修改。
	// entityKind 不存在时返回 ErrSchemaNotFound。
	ApplyDefaults(entityKind string, props map[string]any) (map[string]any, error)
}
