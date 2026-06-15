# I-05: GraphAssembler 节点转换 + 关系推导

## 1. 任务概述

实现 GraphAssembler 的核心逻辑：将一批 `NormalizedResource` 组装为纯图元素 `GraphModel`。分两阶段批量处理——先建所有节点，再推导所有关系。这是数据流的关键解耦点：上游是归一化节点记录，下游是纯图模型。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 2: 实现阶段 — 数据流管线 |
| 预估工时 | 2 天 |
| 前置任务 | I-01, D-04 |
| 交付物 | `internal/assembler/assembler.go` |

## 2. 详细实现步骤

### Day 1: 节点转换

**文件**: `internal/assembler/assembler.go`

```go
package assembler

import (
    "fmt"
    "log/slog"

    "gitlab.com/pml/network-digital-twin/internal/normalizer"
    "gitlab.com/pml/network-digital-twin/internal/schema"
)

type GraphAssembler struct {
    registry schema.SchemaRegistry
}

func NewGraphAssembler(registry schema.SchemaRegistry) *GraphAssembler {
    return &GraphAssembler{registry: registry}
}

// Assemble []NormalizedResource → GraphModel (纯图节点+图边)
func (a *GraphAssembler) Assemble(resources []normalizer.NormalizedResource) (*GraphModel, []ValidationWarning, error) {
    var (
        nodes     []Node
        uriIndex  = make(map[string]bool)       // 全量 URI 索引
        warnings  []ValidationWarning
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
            Label: res.Kind,
            URI:   res.URI,
            Props: props,
        }
        nodes = append(nodes, node)
        uriIndex[res.URI] = true
    }

    // === 阶段 2: 关系推导 ===
    var relations []Relation
    for _, res := range resources {
        et, _ := a.registry.GetEntityType(res.Kind)
        if len(et.Spec.RelationFields) == 0 {
            continue
        }

        for fieldName, relSpec := range et.Spec.RelationFields {
            // 从 Properties 提取关系字段的值（URI 列表）
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

// toStringSlice 尝试将 any 转换为 []string
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

func containsStr(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}
```

## 3. 设计原理

### 两阶段批量处理（无先后依赖）

- **阶段 1**：所有资源 → 节点，构建全量 URI 索引
- **阶段 2**：从全量 URI 索引推导关系，不依赖处理顺序

这解决了"Device 依赖 Interface，Interface 要先处理"的问题：
- 不管谁先谁后，阶段 1 建好所有节点
- 阶段 2 基于全量 URI 索引推导关系

### 关系字段过滤

- 节点转换时，从 Properties 中**过滤掉** `relationFields` 中声明的字段
- 这些字段不应该作为节点属性存入 Neo4j
- 它们已经转化为图边

### 孤儿边检测

- 关系目标节点不在 URI 索引中 → 跳过该关系 + Warn
- 不阻断同步，一个 Interface 缺失不应该导致整个批次失败
- SyncResult.OrphanEdgesSkipped 可观测

### 与其他模块的交互

- **输入**：`[]NormalizedResource`（来自 Normalizer）
- **依赖**：`SchemaRegistry`（读 `relationFields` + `RelationType`）
- **输出**：`*GraphModel`（传给 GraphDB）

## 4. 验收标准

- [ ] Device → Node 转换正确，Props 不含关系字段（interfaces/upstream_links 被过滤）
- [ ] Interface → Node 转换正确（无 relationFields 的实体不过滤）
- [ ] relationFields 正确推导为 Relation（HAS_INTERFACE/CONNECTS_TO/RUNS_ON/ENDPOINT）
- [ ] RelationType 的 source 类型校验生效（Device 不能产生 RUNS_ON 关系）
- [ ] 孤儿边被跳过，返回 ValidationWarning
- [ ] 多类型混合批量处理（Device + Interface + ISIS 同时传入）
- [ ] 空 relationFields 的实体只生成节点，无关系

## 5. 注意事项

- `toStringSlice` 需要处理 `[]any` 类型（JSON 反序列化后的默认类型）
- RelationType 校验只检查 source 类型，target 类型在孤儿边检测中间接验证
- 关系字段在 Properties 中的值是 URI 字符串列表（已经由 Normalizer 生成）
- 孤儿边只是 Warn + 跳过，不报错不阻断
- 阶段 1 和阶段 2 遍历 resources 的顺序相同，不需要额外排序
- GraphModel 是纯图元素，与数据源和 Schema 完全解耦
