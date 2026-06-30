# V1-11: SnapshotDiff 数据结构扩展

**工时**: 0.5 天
**前置**: V1-01
**风险等级**: 低（追加式扩展，不修改现有字段）
**Phase**: Phase 2b — 属性级 Diff + 本体继承

---

## 背景

当前 `SnapshotDiff` 只包含 `AddedNodes/RemovedNodes/AddedRels/RemovedRels`（URI 级增删）。TD-04 要求扩展到属性级变更检测。本任务定义新类型，V1-12/V1-13 分别实现 LocalDiff 和 Cypher Diff 的属性级对比逻辑。

---

## 实现内容

### 1. snapshot/manager.go — 新增类型定义

```go
// NodeChange 节点属性级变更。
type NodeChange struct {
    URI            string                   // 节点 URI
    Label          string                   // 节点标签（MostSpecificLabel）
    AddedFields    map[string]any           // 新增的属性 (b 有 a 无)
    RemovedFields  map[string]any           // 删除的属性 (a 有 b 无)
    ModifiedFields map[string]FieldChange   // 修改的属性 (a 和 b 都有但值不同)
}

// FieldChange 单个字段的变更详情。
type FieldChange struct {
    OldValue any  // 旧值 (快照 a)
    NewValue any  // 新值 (快照 b)
}

// RelChange 关系属性级变更。
type RelChange struct {
    Type           string
    From           string
    To             string
    AddedFields    map[string]any
    RemovedFields  map[string]any
    ModifiedFields map[string]FieldChange
}
```

### 2. SnapshotDiff 新增字段

```go
type SnapshotDiff struct {
    AddedNodes   []assembler.Node
    RemovedNodes []assembler.Node
    AddedRels    []assembler.Relation
    RemovedRels  []assembler.Relation
    ChangedNodes []NodeChange  // 新增
    ChangedRels  []RelChange   // 新增
}
```

### 3. compareProps 辅助函数

```go
// compareProps 对比两个属性 map，返回 added/removed/modified 三个分类。
// 数值归一化: int(42) vs float64(42.0) 视为相等。
func compareProps(a, b map[string]any) (added, removed map[string]any, modified map[string]FieldChange) {
    added = make(map[string]any)
    removed = make(map[string]any)
    modified = make(map[string]FieldChange)

    // b 有 a 无 → added
    for k, v := range b {
        if _, ok := a[k]; !ok {
            added[k] = v
        }
    }

    // a 有 b 无 → removed
    for k, v := range a {
        if _, ok := b[k]; !ok {
            removed[k] = v
        }
    }

    // 两边都有但值不同 → modified
    for k, aVal := range a {
        if bVal, ok := b[k]; ok {
            if !valuesEqual(aVal, bVal) {
                modified[k] = FieldChange{OldValue: aVal, NewValue: bVal}
            }
        }
    }

    return
}

// valuesEqual 比较两个 any 值，处理 int/float64 归一化。
func valuesEqual(a, b any) bool {
    // 数值归一化: 都转 float64 比较
    aFloat, aOK := toFloat64(a)
    bFloat, bOK := toFloat64(b)
    if aOK && bOK {
        return aFloat == bFloat
    }
    // 兜底: 字符串表示比较
    return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func toFloat64(v any) (float64, bool) {
    switch n := v.(type) {
    case int:
        return float64(n), true
    case int64:
        return float64(n), true
    case float64:
        return n, true
    default:
        return 0, false
    }
}
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/snapshot/manager.go` | 修改 | 新增 NodeChange/RelChange/FieldChange/SnapshotDiff 扩展/compareProps |

---

## 验收标准

- [ ] 编译通过
- [ ] `NodeChange`/`RelChange`/`FieldChange` 类型可被其他包引用
- [ ] `compareProps` 正确分类 added/removed/modified
- [ ] `valuesEqual` 正确处理 int(42) vs float64(42.0) → 相等
- [ ] `valuesEqual` 正确处理 "up" vs "down" → 不相等
- [ ] 空 map 对比返回三个空 map
