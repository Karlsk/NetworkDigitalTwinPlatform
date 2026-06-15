# I-12: Neo4j BuildCypher + Query + 复合索引

## 1. 任务概述

实现 Cypher 预览方法 BuildCypher（不执行，只生成 Cypher 字符串 + params），以及系统启动时自动创建 `(_db, uri)` 复合索引。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 2: 实现阶段 — 图数据库驱动 |
| 预估工时 | 1 天 |
| 前置任务 | I-08 |
| 交付物 | `internal/graph/neo4j.go` BuildCypher + ensureIndexes |

## 2. 详细实现步骤

### BuildCypher 预览方法

```go
func (c *neo4jClient) BuildCypher(action string, db string, nodes []assembler.Node, rels []assembler.Relation, uris []string) (string, map[string]any) {
    params := map[string]any{"_db": db}

    switch action {
    case "create":
        // 生成 BulkCreate 的 Cypher
        nodeGroups := groupNodesByLabel(nodes)
        var cyphers []string
        for label, group := range nodeGroups {
            nodeData := make([]map[string]any, 0, len(group))
            for _, n := range group {
                nodeData = append(nodeData, map[string]any{"uri": n.URI, "props": n.Props})
            }
            params["nodes_"+label] = nodeData
            cyphers = append(cyphers, fmt.Sprintf(
                "UNWIND $nodes_%s AS n CREATE (x:%s {_db: $_db, uri: n.uri}) SET x += n.props",
                label, label,
            ))
        }
        return strings.Join(cyphers, ";\n"), params

    case "upsert":
        nodeGroups := groupNodesByLabel(nodes)
        var cyphers []string
        for label, group := range nodeGroups {
            nodeData := make([]map[string]any, 0, len(group))
            for _, n := range group {
                nodeData = append(nodeData, map[string]any{"uri": n.URI, "props": n.Props})
            }
            params["nodes_"+label] = nodeData
            cyphers = append(cyphers, fmt.Sprintf(
                "UNWIND $nodes_%s AS n MERGE (x:%s {_db: $_db, uri: n.uri}) SET x += n.props",
                label, label,
            ))
        }
        return strings.Join(cyphers, ";\n"), params

    case "delete":
        params["uris"] = uris
        return "UNWIND $uris AS uri MATCH (n {_db: $_db, uri: uri}) DETACH DELETE n", params

    case "delete_relations":
        relGroups := groupRelsByType(rels)
        var cyphers []string
        for relType, group := range relGroups {
            relData := make([]map[string]any, 0, len(group))
            for _, r := range group {
                relData = append(relData, map[string]any{"from": r.From, "to": r.To})
            }
            params["rels_"+relType] = relData
            cyphers = append(cyphers, fmt.Sprintf(
                "UNWIND $rels_%s AS r MATCH (a {_db: $_db, uri: r.from})-[x:%s]->(b {_db: $_db, uri: r.to}) DELETE x",
                relType, relType,
            ))
        }
        return strings.Join(cyphers, ";\n"), params

    default:
        return "", params
    }
}
```

### 复合索引创建

```go
// ensureIndexes 系统启动时创建 (_db, uri) 复合索引
func (c *neo4jClient) ensureIndexes(ctx context.Context, entityTypes []string) error {
    session := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
    defer session.Close(ctx)

    for _, label := range entityTypes {
        indexName := fmt.Sprintf("idx_%s_db_uri", strings.ToLower(label))
        cypher := fmt.Sprintf(
            "CREATE INDEX %s IF NOT EXISTS FOR (n:%s) ON (n._db, n.uri)",
            indexName, label,
        )
        if _, err := session.Run(ctx, cypher, nil); err != nil {
            return fmt.Errorf("create index %s: %w", indexName, err)
        }
    }
    return nil
}
```

## 3. 设计原理

### BuildCypher 用于预览/测试/audit

- 不执行 Cypher，只返回字符串 + params
- 测试时可以断言生成的 Cypher 是否正确
- Audit 日志中可以记录即将执行的 Cypher
- 与 Query 组合使用：先 BuildCypher 预览，确认后再 Query 执行

### 复合索引 `(_db, uri)`

- Neo4j CE 逻辑多 DB 的核心索引
- 所有 MATCH 操作都基于 `_db + uri` 查找
- 没有索引的话每次 MATCH 都是全量扫描，性能不可接受
- `CREATE INDEX ... IF NOT EXISTS` 保证幂等（重复调用不报错）

### 索引与 EntityType 的对应关系

- 每个 EntityType（Device、Interface、ISIS 等）需要一个复合索引
- 系统启动时从 SchemaRegistry 获取所有 EntityType 名称，逐一创建索引

## 4. 验收标准

- [ ] `BuildCypher("create", ...)` 返回合法的 Cypher 字符串（含 UNWIND + CREATE）
- [ ] `BuildCypher("upsert", ...)` 返回合法的 Cypher 字符串（含 MERGE + SET +=）
- [ ] `BuildCypher("delete", ...)` 返回 `DETACH DELETE` 语句
- [ ] `BuildCypher("delete_relations", ...)` 返回关系删除语句
- [ ] 所有 Cypher 都使用 `$_db` 变量
- [ ] 索引创建成功（`SHOW INDEXES` 可看到）
- [ ] 重复调用 ensureIndexes 不报错（IF NOT EXISTS）

## 5. 注意事项

- BuildCypher 的 params key 需要避免冲突（如 `nodes_Device`、`nodes_Interface`）
- 索引名使用 `idx_{label}_db_uri` 格式，确保唯一
- 索引创建是异步的（Neo4j 后台构建），大量数据时可能需要等待
- BuildCypher 返回的 Cypher 是多条语句拼接（`;` 分隔），仅供预览，不能直接传给 session.Run
- ensureIndexes 在 main.go 中 SchemaRegistry.Load 之后调用
