# I-02: Schema 校验器 (Validator)

## 1. 任务概述

实现 SchemaRegistry 的 `Validate` 方法，根据 EntityType 的 `properties` 定义校验数据合法性：必填字段、类型匹配、枚举值、stableKeys 非空、默认值填充。在 Normalizer 中被调用，拦截不合法数据。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 2: 实现阶段 — 数据流管线 |
| 预估工时 | 1 天 |
| 前置任务 | I-01 |
| 交付物 | `internal/schema/validator.go` |

## 2. 详细实现步骤

**文件**: `internal/schema/validator.go`（实现在 `registryImpl` 上）

```go
// Validate 校验数据合法性
func (r *registryImpl) Validate(entityKind string, props map[string]any) error {
    et, err := r.GetEntityType(entityKind)
    if err != nil {
        return err
    }

    var errs []string

    for propName, propSpec := range et.Spec.Properties {
        val, exists := props[propName]

        // 1. required 校验
        if propSpec.Required && (!exists || isEmpty(val)) {
            errs = append(errs, fmt.Sprintf("required field %q is missing or empty", propName))
            continue
        }

        // 2. 默认值填充
        if !exists && propSpec.Default != nil {
            props[propName] = propSpec.Default
            val = propSpec.Default
        }

        if !exists {
            continue
        }

        // 3. 类型校验
        if err := checkType(propName, val, propSpec.Type); err != nil {
            errs = append(errs, err.Error())
        }

        // 4. enum 校验
        if len(propSpec.Enum) > 0 {
            strVal, ok := val.(string)
            if !ok || !contains(propSpec.Enum, strVal) {
                errs = append(errs, fmt.Sprintf("field %q value %q not in enum %v", propName, val, propSpec.Enum))
            }
        }
    }

    // 5. stableKeys 非空校验
    for _, key := range et.Spec.Identity.StableKeys {
        val, exists := props[key]
        if !exists || isEmpty(val) {
            errs = append(errs, fmt.Sprintf("stableKey %q must not be empty", key))
        }
    }

    if len(errs) > 0 {
        return fmt.Errorf("validation failed for %s: %s", entityKind, strings.Join(errs, "; "))
    }
    return nil
}

func checkType(name string, val any, expectedType string) error {
    switch expectedType {
    case "string":
        if _, ok := val.(string); !ok {
            return fmt.Errorf("field %q expected string, got %T", name, val)
        }
    case "int":
        switch val.(type) {
        case int, int64, float64: // JSON 数字可能是 float64
            // ok
        default:
            return fmt.Errorf("field %q expected int, got %T", name, val)
        }
    case "float":
        switch val.(type) {
        case float64, float32, int, int64:
            // ok
        default:
            return fmt.Errorf("field %q expected float, got %T", name, val)
        }
    case "bool":
        if _, ok := val.(bool); !ok {
            return fmt.Errorf("field %q expected bool, got %T", name, val)
        }
    }
    return nil
}

func isEmpty(val any) bool {
    if val == nil {
        return true
    }
    if s, ok := val.(string); ok && s == "" {
        return true
    }
    return false
}

func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}
```

## 3. 设计原理

### 为什么 Validate 内嵌在 SchemaRegistry 而非独立接口？

- Validate 需要读取 EntityType 的 properties 定义，与 SchemaRegistry 天然耦合
- 不需要独立的 Validator 接口，减少抽象层
- 调用方（Normalizer）只需要依赖 SchemaRegistry 一个接口

### 校验时机

- Normalizer.Normalize() 内部在生成 NormalizedResource **之前**调用 Validate
- 不合法数据被拦截，不进入后续管线
- 避免脏数据污染图数据库

### 默认值填充策略

- 在 Validate 中完成默认值填充（`props[propName] = propSpec.Default`）
- 修改传入的 map 是有意为之，调用方拿到的 props 已经填充完毕

## 4. 验收标准

- [ ] required 字段缺失 → 返回明确的 error 信息（包含字段名）
- [ ] enum 非法值 → 返回 error（包含实际值和合法枚举列表）
- [ ] 默认值正确填充（字段缺失 + Default 不为空 → 自动填充）
- [ ] stableKeys 非空校验（对应字段为空 → 返回 error）
- [ ] 类型不匹配 → 返回 error（包含期望类型和实际类型）
- [ ] 多个校验失败 → error 信息包含所有失败项（`; ` 分隔）

## 5. 注意事项

- JSON 反序列化后数字类型是 `float64`，checkType("int") 需要兼容
- `isEmpty` 只检查 nil 和空字符串，不检查 0（数字 0 是合法值）
- Validate 方法修改传入的 `props` map（填充默认值），调用方需要注意
- error 信息应该包含足够的上下文（字段名、期望值、实际值），方便调试
- stableKeys 校验在 properties 校验之后，确保 key 字段已通过 required 检查
