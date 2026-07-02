// Package schema 实现 Schema Registry，负责加载和管理本体定义
package schema

import (
	"fmt"
	"log/slog"
	"sort"
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
	// 多个校验失败时通过 errors.Join 聚合返回。
	// entityKind 不存在时返回 ErrSchemaNotFound。
	Validate(entityKind string, props map[string]any) error

	// ApplyDefaults 返回一个新 map，将 schema 中定义的默认值填充到缺失的可选字段。
	// 原始 map 不被修改。
	// entityKind 不存在时返回 ErrSchemaNotFound。
	ApplyDefaults(entityKind string, props map[string]any) (map[string]any, error)

	// GetLabels 返回完整标签链（从基类到具体类）。
	// 如 Device extends Resource → ["Resource", "Device"]
	// 无继承 → ["Device"]
	// 未知实体 → ["UnknownName"]
	GetLabels(entityKind string) []string
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

	if err := r.resolveInheritance(); err != nil {
		return err
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
// 不修改输入 map，多个校验失败时通过 errors.Join 聚合返回。
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

// GetLabels 返回完整标签链（从基类到具体类）。
// 递归向上遍历 extends 链，构建 [base, ..., child] 的标签列表。
// 使用 visited map 防止循环（正常流程不应触发，因为 resolveInheritance 已做环检测）。
func (r *registryImpl) GetLabels(entityKind string) []string {
	et, ok := r.entityTypes[entityKind]
	if !ok {
		return []string{entityKind}
	}
	if et.Spec.Extends == "" {
		return []string{entityKind}
	}

	// 递归构建标签链
	labels := []string{}
	current := entityKind
	visited := make(map[string]bool)
	for current != "" {
		if visited[current] {
			break // 防止循环
		}
		visited[current] = true
		labels = append([]string{current}, labels...)
		if parentET, ok := r.entityTypes[current]; ok {
			current = parentET.Spec.Extends
		} else {
			break
		}
	}
	return labels
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

// resolveInheritance 解析 EntityType 之间的继承关系。
// 构建继承图 → 环检测 → 拓扑排序 → 按序合并父类型属性到子类型。
// 在 Load() 中、crossValidate() 之前调用。
func (r *registryImpl) resolveInheritance() error {
	// Step 1: 构建继承图 child → parent
	inheritance := make(map[string]string)
	for name, et := range r.entityTypes {
		if et.Spec.Extends != "" {
			inheritance[name] = et.Spec.Extends
		}
	}
	if len(inheritance) == 0 {
		return nil // 无继承关系，直接返回
	}

	// Step 2: 环检测 (DFS)
	if err := detectCycle(inheritance); err != nil {
		return fmt.Errorf("schema inheritance: %w", err)
	}

	// Step 3: 按拓扑序合并（先处理祖先，再处理后代）
	order := topoSort(inheritance)
	for _, child := range order {
		parent := inheritance[child]
		if parent == "" {
			continue
		}
		parentET, ok := r.entityTypes[parent]
		if !ok {
			return fmt.Errorf("entity type %q extends unknown parent %q", child, parent)
		}
		mergeEntityType(r.entityTypes[child], parentET)
	}

	return nil
}

// detectCycle 使用 DFS 检测继承图中的环。
// inheritance 是 child → parent 的映射。
func detectCycle(inheritance map[string]string) error {
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var dfs func(node string) error
	dfs = func(node string) error {
		if inStack[node] {
			return fmt.Errorf("circular inheritance detected: %s", node)
		}
		if visited[node] {
			return nil
		}
		visited[node] = true
		inStack[node] = true
		if parent, ok := inheritance[node]; ok {
			if err := dfs(parent); err != nil {
				return err
			}
		}
		inStack[node] = false
		return nil
	}

	// 对排序后的 keys 进行 DFS，确保结果确定性
	keys := make([]string, 0, len(inheritance))
	for k := range inheritance {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, node := range keys {
		if err := dfs(node); err != nil {
			return err
		}
	}
	return nil
}

// topoSort 对继承图进行拓扑排序，返回处理顺序。
// 保证祖先先于后代被处理，使得多级继承时合并结果正确。
func topoSort(inheritance map[string]string) []string {
	// 收集所有涉及的节点
	allNodes := make(map[string]bool)
	for child, parent := range inheritance {
		allNodes[child] = true
		allNodes[parent] = true
	}

	visited := make(map[string]bool)
	var result []string

	var visit func(node string)
	visit = func(node string) {
		if visited[node] {
			return
		}
		visited[node] = true
		// 先递归处理父节点（祖先优先）
		if parent, ok := inheritance[node]; ok {
			visit(parent)
		}
		result = append(result, node)
	}

	// 对排序后的 keys 进行遍历，确保结果确定性
	keys := make([]string, 0, len(allNodes))
	for k := range allNodes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, node := range keys {
		visit(node)
	}
	return result
}

// mergeEntityType 将父类型的属性合并到子类型。
// 合并规则：
//   - Properties: 子类型同名覆盖父类型
//   - FieldMapping: 子类型同名覆盖父类型
//   - Normalize: 父类型规则追加到子类型前面
//   - RelationFields: 子类型同名覆盖父类型
//   - Identity 和 URITemplate: 不继承，子类型独立定义
func mergeEntityType(child, parent *EntityType) {
	// Properties: 合并，子类型同名覆盖父类型
	if child.Spec.Properties == nil {
		child.Spec.Properties = make(map[string]PropertySpec)
	}
	for k, v := range parent.Spec.Properties {
		if _, exists := child.Spec.Properties[k]; !exists {
			child.Spec.Properties[k] = v
		}
	}

	// FieldMapping: 合并，子类型覆盖
	if child.Spec.FieldMapping == nil {
		child.Spec.FieldMapping = make(map[string]string)
	}
	for k, v := range parent.Spec.FieldMapping {
		if _, exists := child.Spec.FieldMapping[k]; !exists {
			child.Spec.FieldMapping[k] = v
		}
	}

	// Normalize: 父类型规则追加到子类型前面
	child.Spec.Normalize = append(parent.Spec.Normalize, child.Spec.Normalize...)

	// RelationFields: 合并
	if child.Spec.RelationFields == nil {
		child.Spec.RelationFields = make(map[string]RelationFieldSpec)
	}
	for k, v := range parent.Spec.RelationFields {
		if _, exists := child.Spec.RelationFields[k]; !exists {
			child.Spec.RelationFields[k] = v
		}
	}

	// Identity 和 URITemplate: 不继承，子类型独立定义
}
