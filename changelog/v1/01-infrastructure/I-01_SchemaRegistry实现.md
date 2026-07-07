# I-01: SchemaRegistry.Load + Get/List 实现

## 1. 任务概述

实现 SchemaRegistry 接口的核心方法：从 `ontology/` 目录加载所有 YAML 文件到内存，提供 EntityType 和 RelationType 的查询接口。这是所有后续模块（Normalizer、GraphAssembler、Validator）依赖的基础。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 2: 实现阶段 — 数据流管线 |
| 预估工时 | 1.5 天 |
| 前置任务 | D-02, D-03 |
| 交付物 | `internal/schema/registry.go` 完整实现 |

## 2. 详细实现步骤

### Day 1: Load 方法

**文件**: `internal/schema/registry.go`

```go
package schema

import (
    "fmt"
    "log/slog"
    "os"
    "path/filepath"

    "gopkg.in/yaml.v3"
)

type registryImpl struct {
    entityTypes   map[string]*EntityType
    relationTypes map[string]*RelationType
}

func NewSchemaRegistry() SchemaRegistry {
    return &registryImpl{
        entityTypes:   make(map[string]*EntityType),
        relationTypes: make(map[string]*RelationType),
    }
}

func (r *registryImpl) Load(dir string) error {
    // 1. 扫描目录下所有 .yaml 文件
    entries, err := os.ReadDir(dir)
    if err != nil {
        return fmt.Errorf("read ontology dir: %w", err)
    }

    for _, entry := range entries {
        if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
            continue
        }

        filePath := filepath.Join(dir, entry.Name())
        if err := r.loadFile(filePath); err != nil {
            return fmt.Errorf("load %s: %w", filePath, err)
        }
    }

    // 2. 加载完成后交叉校验
    r.crossValidate()

    return nil
}

func (r *registryImpl) loadFile(filePath string) error {
    data, err := os.ReadFile(filePath)
    if err != nil {
        return err
    }

    // 支持多文档 YAML（用 --- 分隔）
    decoder := yaml.NewDecoder(bytes.NewReader(data))
    for {
        var doc map[string]any
        if err := decoder.Decode(&doc); err != nil {
            if err == io.EOF {
                break
            }
            return fmt.Errorf("decode yaml: %w", err)
        }

        // 按 kind 字段区分 EntityType 和 RelationType
        kind, _ := doc["kind"].(string)
        switch kind {
        case "EntityType":
            var et EntityType
            if err := yaml.Unmarshal(mustMarshal(doc), &et); err != nil {
                return err
            }
            r.entityTypes[et.Metadata.Name] = &et

        case "RelationType":
            var rt RelationType
            if err := yaml.Unmarshal(mustMarshal(doc), &rt); err != nil {
                return err
            }
            r.relationTypes[rt.Metadata.Name] = &rt

        default:
            slog.Warn("unknown kind in yaml", "kind", kind, "file", filePath)
        }
    }

    return nil
}

// crossValidate 交叉校验：检查 relationFields 引用的 RelationType 是否都已定义
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
```

### Day 2 (半天): Get/List 方法

```go
func (r *registryImpl) GetEntityType(name string) (*EntityType, error) {
    et, ok := r.entityTypes[name]
    if !ok {
        return nil, fmt.Errorf("entity type %q not found", name)
    }
    return et, nil
}

func (r *registryImpl) GetRelationType(name string) (*RelationType, error) {
    rt, ok := r.relationTypes[name]
    if !ok {
        return nil, fmt.Errorf("relation type %q not found", name)
    }
    return rt, nil
}

func (r *registryImpl) ListEntityTypes() []*EntityType {
    result := make([]*EntityType, 0, len(r.entityTypes))
    for _, et := range r.entityTypes {
        result = append(result, et)
    }
    return result
}

func (r *registryImpl) ListRelationTypes() []*RelationType {
    result := make([]*RelationType, 0, len(r.relationTypes))
    for _, rt := range r.relationTypes {
        result = append(result, rt)
    }
    return result
}
```

## 3. 设计原理

### 为什么用 `yaml.Decoder` 循环读取多文档？

- `relations.yaml` 使用 `---` 分隔多个 RelationType 定义
- `yaml.Unmarshal` 只能解析第一个文档，多文档需要 `Decoder.Decode` 循环
- 这是 K8s CRD 的标准做法

### 为什么交叉校验只 Warn 不报错？

- 允许先定义 `relationFields` 再补 RelationType（开发迭代）
- 运行时孤儿边检测会兜底（目标节点不存在 → 跳过）
- V1 可升级为 Error 模式

### 与其他模块的交互

- Normalizer 调用 `GetEntityType()` 获取字段映射规则
- GraphAssembler 调用 `GetEntityType()` + `GetRelationType()` 获取关系推导规则
- Validator 调用 `Validate()` 校验数据合法性

## 4. 验收标准

- [ ] Load 成功加载 6 个 EntityType + 5 个 RelationType
- [ ] `GetEntityType("Device")` 返回完整的 EntityType 结构体（含 identity/uriTemplate/fieldMapping/normalize/relationFields/properties）
- [ ] `GetEntityType("NotExist")` 返回明确的 error
- [ ] `GetRelationType("HAS_INTERFACE")` 返回 source=[Device], target=[Interface]
- [ ] `ListEntityTypes()` 返回 6 个
- [ ] `ListRelationTypes()` 返回 5 个
- [ ] 交叉校验：relationFields 引用不存在的 RelationType 时输出 Warn 日志

## 5. 注意事项

- YAML 解析使用 `gopkg.in/yaml.v3`，不要用 `encoding/json`
- 多文档 YAML 需要 `bytes.NewReader` + `yaml.NewDecoder` + `Decode` 循环
- 交叉校验只做 Warn（slog.Warn），不阻止加载
- `loadFile` 中的 doc 先 decode 到 `map[string]any` 判断 kind，再 unmarshal 到具体结构体
- map 遍历顺序不确定，List 方法返回的切片顺序可能不同，测试时不要断言顺序
