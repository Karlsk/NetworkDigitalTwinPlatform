# I-11: Neo4j Delete 方法 (DeleteByURIs + DeleteRelations + CloneDB)

## 1. 任务概述

实现图数据库的删除操作和克隆操作：按 URI 删除节点（DETACH DELETE）、仅删除指定关系、逻辑 DB 克隆（快照恢复用）。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 2: 实现阶段 — 图数据库驱动 |
| 预估工时 | 1 天 |
| 前置任务 | I-10 |
| 交付物 | `internal/graph/neo4j.go` DeleteByURIs / DeleteRelations / CloneDB 方法 |

## 2. 详细实现步骤

### DeleteByURIs

```go
func (c *neo4jClient) DeleteByURIs(ctx context.Context, db string, uris []string) error {
    session := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
    defer session.Close(ctx)

    params := map[string]any{"_db": db, "uris": uris}
    cypher := `UNWIND $uris AS uri
               MATCH (n {_db: $_db, uri: uri})
               DETACH DELETE n`

    _, err := session.Run(ctx, cypher, params)
    return err
}
```

### DeleteRelations

```go
func (c *neo4jClient) DeleteRelations(ctx context.Context, db string, rels []assembler.Relation) error {
    session := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
    defer session.Close(ctx)

    // 按 RelType 分组
    relGroups := groupRelsByType(rels)
    params := map[string]any{"_db": db}

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
             MATCH (a {_db: $_db, uri: r.from})-[x:%s]->(b {_db: $_db, uri: r.to})
             DELETE x`,
            relType,
        )
        if _, err := session.Run(ctx, cypher, params); err != nil {
            return fmt.Errorf("delete rels %s: %w", relType, err)
        }
    }

    return nil
}
```

### CloneDB

```go
func (c *neo4jClient) CloneDB(ctx context.Context, from, to string) error {
    session := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
    defer session.Close(ctx)

    // 1. 复制节点
    cypherNodes := `MATCH (n {_db: $from})
                    WITH n, labels(n) AS labels
                    CALL apoc.create.cloneNodeToDB(n, labels, $to) YIELD output
                    RETURN count(output)`

    // 简化实现：不用 APOC，手动复制
    cypherNodes = `MATCH (n {_db: $from})
                   WITH n, labels(n)[0] AS label
                   CALL {
                       WITH n
                       CREATE (m {_db: $to, uri: n.uri})
                       SET m += properties(n)
                       SET m._db = $to
                   }`

    params := map[string]any{"from": from, "to": to}

    // 实际实现需要按 Label 分组（因为 CREATE 中不能动态设 Label）
    // 简化方案：查询所有节点，按 Label 分组，逐组 CREATE
    nodes, err := c.Query(ctx, from, "MATCH (n {_db: $_db}) RETURN labels(n)[0] AS label, n.uri AS uri, properties(n) AS props", nil)
    if err != nil {
        return err
    }

    // 按 Label 分组创建
    // ... 类似 BulkCreate 逻辑，但 _db 设为 to

    // 2. 复制关系
    rels, err := c.Query(ctx, from,
        `MATCH (a {_db: $_db})-[r]->(b {_db: $_db})
         RETURN type(r) AS type, a.uri AS from, b.uri AS to`, nil)
    if err != nil {
        return err
    }

    // 按 RelType 分组创建
    // ... 类似 BulkCreate 关系逻辑

    return nil
}
```

## 3. 设计原理

### DeleteByURIs 使用 DETACH DELETE

- `DETACH DELETE` 自动删除节点的所有关联关系（入边 + 出边）
- 避免删除节点后留下悬空关系
- 对应 Webhook `action: "delete"` 事件

### DeleteRelations 只删关系不删节点

- 精确匹配 `MATCH (a)-[x:REL_TYPE]->(b)`，只删除关系 `x`
- 节点不受影响
- 对应 Webhook `action: "delete_relation"` 事件

### CloneDB 用于快照恢复

- 将快照逻辑 DB 的所有节点和关系复制到 "default"
- 恢复流程：`ClearDB("default") → CloneDB(snapshotName, "default")`
- 不使用 APOC 插件（MVP 不依赖额外插件）

## 4. 验收标准

- [ ] DeleteByURIs 正确删除节点及关联关系（DETACH DELETE）
- [ ] DeleteByURIs 对不存在的 URI 不报错（MATCH 不到则跳过）
- [ ] DeleteRelations 只删除指定关系，节点不受影响
- [ ] CloneDB 完整复制节点（含 Label、属性、_db）
- [ ] CloneDB 完整复制关系（含 RelType）

## 5. 注意事项

- DeleteByURIs 的 `uris` 为空时不应报错（UNWIND 空列表 → 无操作）
- DeleteRelations 中 MATCH 不到关系时不报错（跳过）
- CloneDB 实现较复杂，需要查询源 DB 的节点/关系再写入目标 DB
- CloneDB 目标 DB 应先 ClearDB（调用方负责）
- 不用 APOC 插件，手动实现克隆逻辑
