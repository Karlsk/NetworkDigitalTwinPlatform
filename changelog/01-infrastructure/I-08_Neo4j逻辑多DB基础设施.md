# I-08: Neo4j _db 注入 + ClearDB + ListDBs + HasDB

## 1. 任务概述

实现 Neo4j 逻辑多 DB 的基础设施：驱动层强制注入 `_db` 参数、清空逻辑 DB、列出/判断逻辑 DB。这是 Neo4j CE 逻辑多 DB 方案的核心机制。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 2: 实现阶段 — 图数据库驱动 |
| 预估工时 | 1 天 |
| 前置任务 | I-07 |
| 交付物 | `internal/graph/neo4j.go`（Query/ClearDB/ListDBs/HasDB）、`internal/graph/logical_db.go` |

## 2. 详细实现步骤

### Query 方法（驱动层强制注入 `_db`）

```go
func (c *neo4jClient) Query(ctx context.Context, db string, cypher string, params map[string]any) ([]map[string]any, error) {
    if params == nil {
        params = make(map[string]any)
    }
    // 驱动层强制注入 _db 参数
    params["_db"] = db

    session := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
    defer session.Close(ctx)

    result, err := session.Run(ctx, cypher, params)
    if err != nil {
        return nil, fmt.Errorf("run cypher: %w", err)
    }

    var records []map[string]any
    for result.Next(ctx) {
        record := result.Record()
        row := make(map[string]any)
        for _, key := range record.Keys {
            val, _ := record.Get(key)
            row[key] = val
        }
        records = append(records, row)
    }
    return records, result.Err()
}
```

### ClearDB 方法

```go
func (c *neo4jClient) ClearDB(ctx context.Context, db string) error {
    params := map[string]any{"_db": db}
    cypher := "MATCH (n {_db: $_db}) DETACH DELETE n"

    session := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
    defer session.Close(ctx)

    _, err := session.Run(ctx, cypher, params)
    return err
}
```

### ListDBs 方法

```go
func (c *neo4jClient) ListDBs(ctx context.Context) ([]string, error) {
    session := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
    defer session.Close(ctx)

    result, err := session.Run(ctx, "MATCH (n) WHERE n._db IS NOT NULL RETURN DISTINCT n._db AS db", nil)
    if err != nil {
        return nil, err
    }

    var dbs []string
    for result.Next(ctx) {
        if db, ok := result.Record().Get("db"); ok {
            if s, ok := db.(string); ok {
                dbs = append(dbs, s)
            }
        }
    }
    return dbs, result.Err()
}
```

### HasDB 方法

```go
func (c *neo4jClient) HasDB(ctx context.Context, db string) (bool, error) {
    params := map[string]any{"_db": db}
    cypher := "MATCH (n {_db: $_db}) RETURN count(n) > 0 AS exists"

    session := c.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
    defer session.Close(ctx)

    result, err := session.Run(ctx, cypher, params)
    if err != nil {
        return false, err
    }

    if result.Next(ctx) {
        exists, _ := result.Record().Get("exists")
        return exists.(bool), nil
    }
    return false, result.Err()
}
```

### logical_db.go 辅助函数

**文件**: `internal/graph/logical_db.go`

```go
package graph

import "context"

// ensureDBReady 确保逻辑 DB 可用（清空旧数据）
func ensureDBReady(ctx context.Context, client GraphDB, db string) error {
    return client.ClearDB(ctx, db)
}

// cleanStaleDBs 清理不在 keepDBs 列表中的逻辑 DB
func cleanStaleDBs(ctx context.Context, client GraphDB, keepDBs map[string]bool) error {
    allDBs, err := client.ListDBs(ctx)
    if err != nil {
        return err
    }
    for _, db := range allDBs {
        if db == "default" { // 永远不清理 default
            continue
        }
        if !keepDBs[db] {
            if err := client.ClearDB(ctx, db); err != nil {
                return err
            }
        }
    }
    return nil
}
```

## 3. 设计原理

### 驱动层强制注入 `_db`

- Neo4j CE 不支持多 DB，通过 `_db` 属性模拟逻辑多 DB
- 业务代码不关心 `_db` 过滤，驱动层自动保证
- 所有 Cypher 操作必须使用 `$_db` 变量，防止跨 DB 查询污染

### 安全保障

| 风险 | 驱动层应对 |
|------|-----------|
| 忘记加 WHERE _db | 驱动层自动注入 `$_db` 参数 |
| 跨 DB 查询污染 | 驱动层强制所有操作绑定 db 参数 |
| 索引失效 | 强制使用 `(_db, uri)` 复合索引 |

### 索引策略（I-12 中创建）

```cypher
CREATE INDEX device_db_uri FOR (d:Device) ON (d._db, d.uri);
CREATE INDEX interface_db_uri FOR (i:Interface) ON (i._db, i.uri);
-- 每个实体类型都需要创建 (_db, uri) 复合索引
```

## 4. 验收标准

- [ ] Query 方法中 `params["_db"]` 被自动注入
- [ ] ClearDB("test") 清空 `_db="test"` 的所有节点和关系
- [ ] ListDBs 返回已有的逻辑 DB 列表（含 "default" 和快照 DB）
- [ ] HasDB("default") 在有数据时返回 true，空 DB 返回 false
- [ ] `cleanStaleDBs` 不清理 "default"

## 5. 注意事项

- Query 的 `params` 可能为 nil，需要先初始化 map 再注入 `_db`
- ClearDB 使用 `DETACH DELETE`，会同时删除节点和关联关系
- ListDBs 查询所有节点的 `_db` 属性，数据量大时性能可能有问题，V1 可优化
- HasDB 用 `count(n) > 0` 而非 `count(n)`，避免全量扫描
- 所有 Cypher 模板中必须使用 `$_db`，不能硬编码 DB 名
