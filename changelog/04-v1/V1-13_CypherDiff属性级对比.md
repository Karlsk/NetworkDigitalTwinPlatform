# V1-13: Cypher Diff 属性级对比

**工时**: 1.5 天
**前置**: V1-11
**风险等级**: 中
**Phase**: Phase 2b — 属性级 Diff + 本体继承

---

## 背景

当前 Cypher `Diff()` 方法使用 4 条 Cypher 差集查询，只检测 URI 增删。本任务新增第 5 条 Cypher 查找同 URI 但属性不同的节点，并在 MCP 输出层适配。

---

## 实现内容

### 1. snapshot/manager.go — Diff 方法增强

在现有 4 条差集 Cypher 之后新增第 5 条：

```cypher
-- 查找相同 URI 但属性不同的节点
MATCH (a {_db: $a}) MATCH (b {_db: $b})
WHERE a.uri = b.uri AND labels(a) = labels(b)
WITH a, b, [k IN keys(a) WHERE k <> '_db' AND a[k] <> b[k]] AS diffKeys
WHERE size(diffKeys) > 0
RETURN a.uri AS uri, labels(a) AS labels,
       properties(a) AS aProps, properties(b) AS bProps
```

对返回结果调用 `compareProps` 构建 `NodeChange`：

```go
// 处理属性级变更结果
for _, record := range results {
    uri := record["uri"].(string)
    aProps := record["aProps"].(map[string]any)
    bProps := record["bProps"].(map[string]any)

    added, removed, modified := compareProps(aProps, bProps)
    if len(added) > 0 || len(removed) > 0 || len(modified) > 0 {
        diff.ChangedNodes = append(diff.ChangedNodes, NodeChange{
            URI:            uri,
            Label:          getMostSpecificLabel(record["labels"]),
            AddedFields:    added,
            RemovedFields:  removed,
            ModifiedFields: modified,
        })
    }
}
```

**关系属性对比**（第 6 条 Cypher，可选）:

```cypher
-- 查找相同关系但属性不同的记录
MATCH (sa)-[ra]->(ea), (sb)-[rb]->(eb)
WHERE sa._db = $a AND sb._db = $b
  AND sa.uri = sb.uri AND ea.uri = eb.uri
  AND type(ra) = type(rb)
WITH ra, rb, [k IN keys(ra) WHERE ra[k] <> rb[k]] AS diffKeys
WHERE size(diffKeys) > 0
RETURN type(ra) AS type, startNode(ra).uri AS from, endNode(ra).uri AS to,
       properties(ra) AS aProps, properties(rb) AS bProps
```

### 2. mcp/tools.go — MCP 输出适配

`SnapshotDiffOutput` 新增变更统计：

```go
type SnapshotDiffOutput struct {
    AddedNodes   int `json:"added_nodes"`
    RemovedNodes int `json:"removed_nodes"`
    AddedRels    int `json:"added_relations"`
    RemovedRels  int `json:"removed_relations"`
    ChangedNodes int `json:"changed_nodes"`   // 新增
    ChangedRels  int `json:"changed_relations"` // 新增
}
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/snapshot/manager.go` | 修改 | Diff 增强属性级 Cypher 查询 |
| `internal/mcp/tools.go` | 修改 | SnapshotDiffOutput 新增字段 |

---

## 验收标准

- [ ] 编译通过
- [ ] Cypher Diff 对两个逻辑 DB 中同 URI 节点正确报告属性级变更
- [ ] 与 LocalDiff 结果一致（对相同数据）
- [ ] MCP 输出包含 `changed_nodes` / `changed_relations` 统计
- [ ] 现有 AddedNodes/RemovedNodes 逻辑不受影响
- [ ] Cypher 性能可接受（利用 `(_db, uri)` 复合索引）
