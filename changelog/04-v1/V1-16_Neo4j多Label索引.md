# V1-16: Neo4j 多 Label + EnsureIndexes 继承

**工时**: 1 天
**前置**: V1-15, V1-01
**风险等级**: 中
**Phase**: Phase 2b — 属性级 Diff + 本体继承

---

## 背景

V1-01 已将 `Node.Label` 改为 `Node.Labels []string`（暂时单标签），V1-15 实现了 `extends` 继承合并。本任务将两者结合：SchemaRegistry 提供完整标签链，Assembler 使用多标签创建节点，Neo4j 为所有 Label（含父 Label）创建索引。

---

## 实现内容

### 1. schema/registry.go — 新增 GetLabels 方法

```go
// GetLabels 返回完整标签链（从基类到具体类）。
// 如 Device extends Resource → ["Resource", "Device"]
// 无继承 → ["Device"]
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
            break  // 防止循环（正常流程不应触发）
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
```

### 2. assembler/assembler.go — 多标签节点转换

```go
// 在 Assemble() 中使用 registry.GetLabels 获取多标签
labels := registry.GetLabels(res.Kind)
node := assembler.Node{
    Labels: labels,  // ["Resource", "Device"]
    URI:    res.URI,
    Props:  res.Properties,
}
```

**注意**: Assembler 需要持有 SchemaRegistry 引用（或通过回调传入 GetLabels 函数）。

### 3. graph/neo4j.go — EnsureIndexes 改造

为所有 Label（包括父 Label）创建索引：

```go
func (n *neo4jClient) EnsureIndexes(ctx context.Context, db string, labels []string) error {
    for _, label := range labels {
        cypher := fmt.Sprintf(
            "CREATE INDEX IF NOT EXISTS FOR (n:%s) ON (n._db, n.uri)", label,
        )
        if err := n.executeCypher(ctx, db, cypher, nil); err != nil {
            return fmt.Errorf("ensure index for %s: %w", label, err)
        }
    }
    return nil
}
```

### 4. cmd/server/main.go — 启动时传入所有 Label

```go
// 收集所有 Label（含基类）
allLabels := collectAllLabels(registry)
if err := gdb.EnsureIndexes(ctx, "default", allLabels); err != nil {
    log.Fatal("ensure indexes: ", err)
}
```

### 5. Cypher 验证

创建 Device 节点时 Neo4j 中应有两个 Label：

```cypher
-- 创建后验证
MATCH (n:Device {uri: "device:SN12345"}) WHERE n._db = $_db
RETURN labels(n)
-- 预期: ["Resource", "Device"]

-- 按基类查询覆盖所有子类型
MATCH (n:Resource) WHERE n._db = $_db RETURN n
-- 预期: 返回所有 Device、Interface、ISIS、Link 等节点

-- 按具体类型查询
MATCH (n:Device) WHERE n._db = $_db RETURN n
-- 预期: 只返回 Device 节点
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/schema/registry.go` | 修改 | 新增 GetLabels 方法 |
| `internal/assembler/assembler.go` | 修改 | 使用 GetLabels 多标签节点转换 |
| `internal/graph/neo4j.go` | 修改 | EnsureIndexes 为每个 Label 创建索引 |
| `cmd/server/main.go` | 修改 | 启动时传入所有 Label |

---

## 验收标准

- [ ] 编译通过
- [ ] Device 节点在 Neo4j 中有两个 Label: `:Resource:Device`
- [ ] `MATCH (n:Resource) WHERE n._db = $_db RETURN n` 返回所有 Resource 子类型
- [ ] `MATCH (n:Device) WHERE n._db = $_db RETURN n` 只返回 Device
- [ ] 父 Label 索引（Resource）和子 Label 索引（Device）均存在
- [ ] 无继承的实体仍然只有单标签
- [ ] `go test -race ./...` 全部通过
