// Package assembler 实现 GraphAssembler (IR 层)，
// 负责将 NormalizedResource 转换为 GraphModel。
// Node 和 Relation 是 GraphModel IR 的核心数据类型，
// 被 GraphDB 驱动层消费。
package assembler

import (
	"fmt"
	"log/slog"

	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/schema"
)

// GraphAssembler 将 NormalizedResource 组装为纯图元素 GraphModel。
// 依赖注入：通过构造函数传入 SchemaRegistry，读取 relationFields + RelationType。
// 不操作数据库，只产出 GraphModel。
type GraphAssembler struct {
	registry schema.SchemaRegistry
}

// NewGraphAssembler 创建一个新的 GraphAssembler 实例。
func NewGraphAssembler(registry schema.SchemaRegistry) *GraphAssembler {
	return &GraphAssembler{registry: registry}
}

// Assemble 将 []NormalizedResource 转换为 GraphModel（纯图节点 + 图边）。
// 三阶段处理：
//  1. 节点转换：过滤 relationFields，构建 URI 索引
//  2. 关系推导：从 relationFields 推导 Relation，校验 source 类型
//  3. 孤儿边校验：目标节点不在 URI 索引中的关系被跳过 + Warning
//
// 阶段 2/3 的问题都是 Warn 级别，不返回 error（单条关系的问题不阻断整个批次）。
func (a *GraphAssembler) Assemble(resources []normalizer.NormalizedResource) (*GraphModel, []ValidationWarning, error) {
	var (
		nodes    []Node
		uriIndex = make(map[string]bool)
		warnings []ValidationWarning
	)

	// === 阶段 1: 节点转换 ===
	for _, res := range resources {
		et, err := a.registry.GetEntityType(res.Kind)
		if err != nil {
			return nil, nil, fmt.Errorf("get entity type %s: %w", res.Kind, err)
		}

		// 过滤掉关系字段，只保留非关系属性
		props := make(map[string]any)
		for k, v := range res.Properties {
			if _, isRelField := et.Spec.RelationFields[k]; !isRelField {
				props[k] = v
			}
		}

		node := Node{
			Labels: a.registry.GetLabels(res.Kind),
			URI:    res.URI,
			Props:  props,
		}
		nodes = append(nodes, node)
		uriIndex[res.URI] = true
	}

	// === 阶段 2: 关系推导 ===
	var relations []Relation
	for _, res := range resources {
		et, err := a.registry.GetEntityType(res.Kind)
		if err != nil {
			return nil, nil, fmt.Errorf("get entity type %s: %w", res.Kind, err)
		}
		if len(et.Spec.RelationFields) == 0 {
			continue
		}

		for fieldName, relSpec := range et.Spec.RelationFields {
			// 从原始 Properties 提取关系字段的值（URI 列表）
			val, ok := res.Properties[fieldName]
			if !ok {
				continue
			}

			targetURIs, ok := toStringSlice(val)
			if !ok {
				slog.Warn("relation field is not a string slice",
					"entity", res.Kind, "field", fieldName)
				continue
			}

			// 获取 RelationType 校验源/目标类型
			rt, err := a.registry.GetRelationType(relSpec.RelationType)
			if err != nil {
				slog.Warn("undefined relation type", "type", relSpec.RelationType)
				continue
			}

			// 校验 source 类型
			if !containsStr(rt.Spec.Source, res.Kind) {
				slog.Warn("source type mismatch",
					"relationType", relSpec.RelationType,
					"expected", rt.Spec.Source,
					"actual", res.Kind)
				continue
			}

			// 生成 Relation
			for _, targetURI := range targetURIs {
				relations = append(relations, Relation{
					Type: relSpec.RelationType,
					From: res.URI,
					To:   targetURI,
				})
			}
		}
	}

	// === 阶段 3: 孤儿边校验 ===
	var validRelations []Relation
	for _, rel := range relations {
		if !uriIndex[rel.To] {
			warnings = append(warnings, ValidationWarning{
				Type:   "orphan_edge",
				Detail: fmt.Sprintf("%s: %s → %s", rel.Type, rel.From, rel.To),
			})
			slog.Warn("orphan edge skipped",
				"type", rel.Type, "from", rel.From, "to", rel.To)
			continue
		}
		validRelations = append(validRelations, rel)
	}

	return &GraphModel{
		Nodes:     nodes,
		Relations: validRelations,
	}, warnings, nil
}

// toStringSlice 尝试将 any 转换为 []string。
// 处理两种切片类型：[]string（Go 原生）和 []any（JSON 反序列化后的默认类型）。
// []any 中非字符串元素会被静默跳过。
func toStringSlice(val any) ([]string, bool) {
	switch v := val.(type) {
	case []string:
		return v, true
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result, true
	default:
		return nil, false
	}
}

// containsStr 检查字符串切片是否包含指定元素。
func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
