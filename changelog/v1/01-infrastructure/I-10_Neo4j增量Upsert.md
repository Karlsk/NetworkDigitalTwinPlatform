# I-10: Neo4j Upsert (MERGE + SET +=)

## 1. 任务概述

实现增量同步的 Upsert 操作：节点使用 MERGE + SET += 增量合并属性，关系使用 MERGE 幂等创建。这是增量同步 `action: "update"` 的核心操作。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 2: 实现阶段 — 图数据库驱动 |
| 预估工时 | 1 天 |
| 前置任务 | I-09 |
| 交付物 | `internal/graph/neo4j.go` Upsert 方法 |

## 2. 详细实现步骤

```go
func (c *neo4jClient) Upsert(ctx context.Context, db string, nodes []assembler.Node, rels []assembler.Relation) error {
    session := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
    defer session.Close(ctx)

    params := map[string]any{"_db": db}

    // 节点 Upsert: MERGE + SET += (属性增量合并)
    nodeGroups := groupNodesByLabel(nodes)
    for label, group := range nodeGroups {
        nodeData := make([]map[string]any, 0, len(group))
        for _, n := range group {
            props := make(map[string]any, len(n.Props)+1)
            for k, v := range n.Props {
                props[k] = v
            }
            props["_db"] = db
            nodeData = append(nodeData, map[string]any{
                "uri":   n.URI,
                "props": props,
            })
        }
        params["nodes"] = nodeData

        cypher := fmt.Sprintf(
            `UNWIND $nodes AS n
             MERGE (x:%s {_db: $_db, uri: n.uri})
             SET x += n.props`,
            label,
        )
        if _, err := session.Run(ctx, cypher, params); err != nil {
            return fmt.Errorf("upsert nodes %s: %w", label, err)
        }
    }

    // 关系 Upsert: MERGE (幂等，已存在则跳过)
    relGroups := groupRelsByType(rels)
    for relType, group := range relGroups {
        relData := make([]map[string]any, 0, len(group))
        for _, r := range group {
            relData = append(relData, map[string]any{
                "from": r.From,
                "to":   r.To,
            })
        }
        params["rels"] = relData

        cypher := fmt.Sprintf(
            `UNWIND $rels AS r
             MATCH (a {_db: $_db, uri: r.from})
             MATCH (b {_db: $_db, uri: r.to})
             MERGE (a)-[:%s]->(b)`,
            relType,
        )
        if _, err := session.Run(ctx, cypher, params); err != nil {
            return fmt.Errorf("upsert rels %s: %w", relType, err)
        }
    }

    return nil
}
```

## 3. 设计原理

### 属性增量合并 (`SET x += n.props`)

- `+=` 语义：只更新传入的属性，未传入的属性保持不变
- 示例：第一次 `{hostname: "R1", status: "Up", bandwidth: 100}`，第二次 `{hostname: "R1", status: "Down"}`（不传 bandwidth），结果 `{hostname: "R1", status: "Down", bandwidth: 100}`（bandwidth 保留）

### 关系 MERGE 幂等

- `MERGE (a)-[:HAS_INTERFACE]->(b)` 已存在则跳过，不存在则创建
- 增量更新时不删除旧关系（只有 `delete_relation` 事件才删除）
- 保证幂等性：多次相同的 Upsert 不会产生重复关系

### 先节点后关系

- 先 MERGE 所有节点（确保目标节点存在）
- 再 MERGE 关系（MATCH 能找到源/目标节点）

## 4. 验收标准

- [ ] 新增节点：MERGE 创建成功
- [ ] 更新节点：属性增量合并（新属性添加，旧属性保留，已传属性更新）
- [ ] 新增关系：MERGE 创建成功
- [ ] 已存在关系：MERGE 幂等，不重复创建
- [ ] `_db` 属性正确设置

## 5. 注意事项

- MERGE 匹配键是 `(_db, uri)`，不要用其他属性匹配（否则会创建重复节点）
- `SET x += n.props` 中 `n.props` 不应包含 `uri` 字段（URI 已在 MERGE 中设置）
- 节点和关系分开执行，先节点后关系
- 如果目标节点不存在（关系 MERGE 中 MATCH 失败），该关系会被跳过
