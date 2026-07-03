# V1-15: EntityTypeSpec Extends + 继承合并

**工时**: 1.5 天
**前置**: V1-01
**风险等级**: 高（继承语义定义 + 环检测）
**Phase**: Phase 2b — 属性级 Diff + 本体继承

---

## 背景

当前 `EntityTypeSpec` 无任何继承相关字段。V1 需支持 `extends` 机制，允许 EntityType 继承父类型的属性定义（Properties/FieldMapping/Normalize/RelationFields），实现类型层级（如 `Device extends Resource`）。

---

## 实现内容

### 1. schema/types.go — EntityTypeSpec 新增 Extends 字段

```go
type EntityTypeSpec struct {
    Extends        string                          `yaml:"extends"`        // 新增: 父类型名称（可空）
    Identity       IdentitySpec                    `yaml:"identity"`
    URITemplate    string                          `yaml:"uriTemplate"`
    FieldMapping   map[string]string               `yaml:"fieldMapping"`
    Normalize      []NormalizeRule                  `yaml:"normalize"`
    RelationFields map[string]RelationFieldSpec     `yaml:"relationFields"`
    Properties     map[string]PropertySpec          `yaml:"properties"`
}
```

### 2. schema/registry.go — 加载后处理 resolveInheritance

在 `Load()` 完成后（`crossValidate()` 之前）调用 `resolveInheritance()`：

```go
func (r *registryImpl) resolveInheritance() error {
    // Step 1: 构建继承图 child → parent
    inheritance := make(map[string]string)
    for name, et := range r.entityTypes {
        if et.Spec.Extends != "" {
            inheritance[name] = et.Spec.Extends
        }
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
```

**合并规则**:

```go
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
```

**环检测**:

```go
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

    for node := range inheritance {
        if err := dfs(node); err != nil {
            return err
        }
    }
    return nil
}
```

### 3. 新增基类 YAML 文件

#### ontology/resource.yaml

```yaml
apiVersion: schema.networktwin.io/v1
kind: EntityType
metadata:
  name: Resource
spec:
  identity:
    stableKeys: []
  uriTemplate: ""
  properties:
    status:
      type: string
      enum: [Up, Down, Maintenance]
    vendor:
      type: string
```

#### ontology/service.yaml, ontology/event.yaml

类似结构，按需定义。

### 4. 更新现有本体文件

```yaml
# ontology/device.yaml
metadata:
  name: Device
spec:
  extends: Resource        # 继承 Resource 基类
  # ... 其余不变
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/schema/types.go` | 修改 | EntityTypeSpec 新增 Extends 字段 |
| `internal/schema/registry.go` | 修改 | resolveInheritance + mergeEntityType + detectCycle |
| `internal/schema/registry_test.go` | 修改 | 继承测试 |
| `ontology/resource.yaml` | 新建 | Resource 基类 |
| `ontology/service.yaml` | 新建 | Service 基类 |
| `ontology/event.yaml` | 新建 | Event 基类 |
| `ontology/device.yaml` | 修改 | 添加 `extends: Resource` |
| `ontology/interface.yaml` | 修改 | 添加 `extends: Resource` |
| `ontology/isis.yaml` | 修改 | 添加 `extends: Resource` |
| `ontology/link.yaml` | 修改 | 添加 `extends: Resource` |
| `ontology/network_slice.yaml` | 修改 | 添加 `extends: Service` |
| `ontology/alarm.yaml` | 修改 | 添加 `extends: Event` |

---

## 验收标准

- [x] 编译通过
- [x] 无 `extends` 字段的 YAML 仍然正常工作（向后兼容）
- [x] `extends: Resource` 的 Device 自动继承 Resource 的 Properties（status, vendor）
- [x] 子类型同名属性覆盖父类型
- [x] FieldMapping/Normalize/RelationFields 正确合并
- [x] Identity 和 URITemplate 不被继承
- [x] 循环继承（A extends B extends A）被检测并返回 error
- [x] 多级继承（A extends B extends C）按拓扑序正确合并
- [x] `go test ./internal/schema/...` 全部通过
