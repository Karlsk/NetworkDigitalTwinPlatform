// Package schema 实现 Schema Registry，负责加载和管理本体定义
package schema

import (
	"fmt"
	"log/slog"
)

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

// registryImpl 是 SchemaRegistry 接口的默认实现。
// 将 EntityType 和 RelationType 以 Metadata.Name 为键存储在内存 map 中。
type registryImpl struct {
	entityTypes   map[string]*EntityType
	relationTypes map[string]*RelationType
}

// 编译时接口满足检查
var _ SchemaRegistry = (*registryImpl)(nil)

// NewSchemaRegistry 创建一个新的 SchemaRegistry 实例。
func NewSchemaRegistry() SchemaRegistry {
	return &registryImpl{
		entityTypes:   make(map[string]*EntityType),
		relationTypes: make(map[string]*RelationType),
	}
}

// Load 从指定目录加载所有 YAML 文件到内存，并执行交叉校验。
// 复用 LoadFromDir 进行文件扫描和多文档解析。
// 空目录（无 EntityType 和 RelationType）时返回错误。
func (r *registryImpl) Load(dir string) error {
	ets, rts, err := LoadFromDir(dir)
	if err != nil {
		return fmt.Errorf("load ontology: %w", err)
	}
	if len(ets) == 0 && len(rts) == 0 {
		return fmt.Errorf("no schemas found in directory %q", dir)
	}

	for i := range ets {
		r.entityTypes[ets[i].Metadata.Name] = &ets[i]
	}
	for i := range rts {
		r.relationTypes[rts[i].Metadata.Name] = &rts[i]
	}

	r.crossValidate()
	return nil
}

// GetEntityType 按名称获取 EntityType。
// 未找到时返回包装了 ErrSchemaNotFound 的错误。
func (r *registryImpl) GetEntityType(name string) (*EntityType, error) {
	et, ok := r.entityTypes[name]
	if !ok {
		return nil, fmt.Errorf("entity type %q: %w", name, ErrSchemaNotFound)
	}
	return et, nil
}

// GetRelationType 按名称获取 RelationType。
// 未找到时返回包装了 ErrSchemaNotFound 的错误。
func (r *registryImpl) GetRelationType(name string) (*RelationType, error) {
	rt, ok := r.relationTypes[name]
	if !ok {
		return nil, fmt.Errorf("relation type %q: %w", name, ErrSchemaNotFound)
	}
	return rt, nil
}

// ListEntityTypes 列出所有已注册的 EntityType，返回顺序不保证。
func (r *registryImpl) ListEntityTypes() []*EntityType {
	result := make([]*EntityType, 0, len(r.entityTypes))
	for _, et := range r.entityTypes {
		result = append(result, et)
	}
	return result
}

// ListRelationTypes 列出所有已注册的 RelationType，返回顺序不保证。
func (r *registryImpl) ListRelationTypes() []*RelationType {
	result := make([]*RelationType, 0, len(r.relationTypes))
	for _, rt := range r.relationTypes {
		result = append(result, rt)
	}
	return result
}

// Validate 校验 props 是否符合 entityKind 对应的 EntityType 定义。
// 不修改输入 map，多个校验失败时返回聚合错误。
func (r *registryImpl) Validate(entityKind string, props map[string]any) error {
	et, err := r.GetEntityType(entityKind)
	if err != nil {
		return err
	}
	return validateProps(et, props)
}

// ApplyDefaults 返回一个新 map，将 schema 中定义的默认值填充到缺失的可选字段。
// 原始 map 不被修改。
func (r *registryImpl) ApplyDefaults(entityKind string, props map[string]any) (map[string]any, error) {
	et, err := r.GetEntityType(entityKind)
	if err != nil {
		return nil, err
	}
	return applyDefaults(et, props), nil
}

// crossValidate 交叉校验：检查 relationFields 引用的 RelationType 是否都已定义。
// 仅输出 Warn 日志，不阻止加载。
func (r *registryImpl) crossValidate() {
	for _, et := range r.entityTypes {
		for field, spec := range et.Spec.RelationFields {
			if _, ok := r.relationTypes[spec.RelationType]; !ok {
				slog.Warn("relationFields references undefined RelationType",
					"entity", et.Metadata.Name,
					"field", field,
					"relationType", spec.RelationType,
				)
			}
		}
	}
}
