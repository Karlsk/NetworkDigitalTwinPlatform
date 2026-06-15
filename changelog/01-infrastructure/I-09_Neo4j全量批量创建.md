# I-09: Neo4j BulkCreate (全量 CREATE)

## 1. 任务概述

实现图数据库的全量批量创建：按 Label 分组使用 UNWIND + CREATE 批量创建节点，按 RelType 分组批量创建关系。用于 FullSync 的 ClearDB + BulkCreate 流程。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 2: 实现阶段 — 图数据库驱动 |
| 预估工时 | 1.5 天 |
| 前置任务 | I-08 |
| 交付物 | `internal/graph/neo4j.go` BulkCreate 方法 |

## 2. 详细实现步骤

### BulkCreate 方法

```go
func (c *neo4jClient) BulkCreate(ctx context.Context, db string, nodes []assembler.Node, rels []assembler.Relation) error {
    session := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
    defer session.Close(ctx)

    params := map[string]any{"_db": db}

    // 按 Label 分组批量创建节点
    nodeGroups := groupNodesByLabel(nodes)
    for label, group := range nodeGroups {
        nodeData := make([]map[string]any, 0, len(group))
        for _, n := range group {
            props := make(map[string]any, len(n.Props)+1)
            for k, v := range n.Props {
                props[k] = v
            }
            props["_db"] = db
            props["uri"] = n.URI
            nodeData = append(nodeData, props)
        }
        params["nodes"] = nodeData

        cypher := fmt.Sprintf(
            "UNWIND $nodes AS n CREATE (x:%s {_db: $_db, uri: n.uri}) SET x += n",
            label,
        )
        if _, err := session.Run(ctx, cypher, params); err != nil {
            return fmt.Errorf("bulk create nodes %s: %w", label, err)
        }
    }

    // 按 RelType 分组批量创建关系
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
             CREATE (a)-[:%s]->(b)`,
            relType,
        )
        if _, err := session.Run(ctx, cypher, params); err != nil {
            return fmt.Errorf("bulk create rels %s: %w", relType, err)
        }
    }

    return nil
}

// groupNodesByLabel 按 Label 分组节点
func groupNodesByLabel(nodes []assembler.Node) map[string][]assembler.Node {
    groups := make(map[string][]assembler.Node)
    for _, n := range nodes {
        groups[n.Label] = append(groups[n.Label], n)
    }
    return groups
}

// groupRelsByType 按关系类型分组
func groupRelsByType(rels []assembler.Relation) map[string][]assembler.Relation {
    groups := make(map[string][]assembler.Relation)
    for _, r := range rels {
        groups[r.Type] = append(groups[r.Type], r)
    }
    return groups
}
```

## 3. 设计原理

### 为什么按 Label/RelType 分组？

- `UNWIND $nodes AS n CREATE (x:Label ...)` 中 Label 不能参数化，只能硬编码
- 每个 Label 一条 Cypher 语句，减少 round-trip
- 同理 RelType 也不能参数化

### UNWIND 批量操作 vs 逐条 INSERT

- UNWIND 一次事务处理所有节点/关系，性能远优于逐条操作
- 减少网络 round-trip 次数
- 事务原子性：一组节点要么全部创建成功，要么全部失败

### `_db` 属性注入

- 节点创建时 `props["_db"] = db` 强制写入
- 关系创建时通过 MATCH 的 `_db` 条件保证在正确的逻辑 DB 中

## 4. 验收标准

- [ ] 批量创建 ~20 个节点（3 Device + 12 Interface + 3 SRv6_Policy + 2 EVPN + 1 Slice）
- [ ] 批量创建 ~30 个关系（HAS_INTERFACE + CONNECTS_TO + RUNS_ON_INTERFACE + CARRIED_BY + BELONGS_TO_SLICE）
- [ ] Cypher 查询可正确返回创建的节点和关系
- [ ] 每个节点都有 `_db` 属性
- [ ] BulkCreate 前必须先 ClearDB（否则会重复）

## 5. 注意事项

- `SET x += n` 会把 `n` map 中的所有键值设为节点属性（包括 `_db` 和 `uri`），需要注意去重
- Label 和 RelType 是字符串拼接（不能参数化），需要确保没有注入风险（来自 Schema 定义，安全）
- 关系创建使用 MATCH 查找源/目标节点，依赖 `(_db, uri)` 复合索引
- BulkCreate 不检查重复（MERGE 才检查），调用前必须 ClearDB
