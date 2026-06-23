// Package normalizer 实现归一化引擎
package normalizer

import (
	"fmt"
	"strings"

	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/schema"
	"gitlab.com/pml/network-digital-twin/pkg/utils"
)

// NormalizedResource 归一化后的节点记录 (不含关系)。
// 由 Normalizer 产出，被 GraphAssembler 消费。
// 关系字段保留在 Properties 中，由 GraphAssembler 读取 relationFields 映射后推导为图边。
type NormalizedResource struct {
	Kind       string         // 实体类型，如 "Device"
	URI        string         // 标准本体 URI (由 uriTemplate 生成)
	Properties map[string]any // 标准化后的属性（类型已校验，含原始关系字段）
}

// Normalizer 归一化引擎，将 Connector 输出的原始 Resource 转换为 NormalizedResource。
// 处理顺序: fieldMapping → normalize → ApplyDefaults → Validate → GenerateURI。
// 只处理节点标准化，不处理关系推导。
type Normalizer struct {
	registry schema.SchemaRegistry
}

// NewNormalizer 创建一个新的 Normalizer 实例。
func NewNormalizer(registry schema.SchemaRegistry) *Normalizer {
	return &Normalizer{registry: registry}
}

// Normalize 将 connector.Resource 转换为 NormalizedResource。
// 六步流程: 获取 Schema → 拷贝属性 → 字段映射 → 值标准化 → 默认值+校验 → URI 生成。
// 不修改原始 Resource 的 Properties。
func (n *Normalizer) Normalize(resource connector.Resource) (*NormalizedResource, error) {
	// 1. 获取 EntityType
	et, err := n.registry.GetEntityType(resource.Kind)
	if err != nil {
		return nil, fmt.Errorf("get entity type %s: %w", resource.Kind, err)
	}

	// 2. 浅拷贝 Properties（避免修改原始数据）
	props := make(map[string]any, len(resource.Properties))
	for k, v := range resource.Properties {
		props[k] = v
	}

	// 3. fieldMapping: 源字段名 → 标准字段名
	for srcField, dstField := range et.Spec.FieldMapping {
		if val, ok := props[srcField]; ok {
			props[dstField] = val
			delete(props, srcField)
		}
	}

	// 4. normalize: 字段值字符串替换
	for _, rule := range et.Spec.Normalize {
		if val, ok := props[rule.Field]; ok {
			if strVal, ok := val.(string); ok {
				props[rule.Field] = strings.ReplaceAll(strVal, rule.Pattern, rule.Replace)
			}
		}
	}

	// 5. ApplyDefaults + Validate
	props, err = n.registry.ApplyDefaults(resource.Kind, props)
	if err != nil {
		return nil, fmt.Errorf("apply defaults for %s: %w", resource.Kind, err)
	}

	if err := n.registry.Validate(resource.Kind, props); err != nil {
		return nil, fmt.Errorf("validate %s: %w", resource.Kind, err)
	}

	// 6. GenerateURI
	uri, err := utils.GenerateURI(et.Spec.URITemplate, et.Spec.Identity.StableKeys, props)
	if err != nil {
		return nil, fmt.Errorf("generate URI for %s: %w", resource.Kind, err)
	}

	return &NormalizedResource{
		Kind:       resource.Kind,
		URI:        uri,
		Properties: props,
	}, nil
}
