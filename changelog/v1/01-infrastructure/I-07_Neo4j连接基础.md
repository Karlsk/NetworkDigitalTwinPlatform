# I-07: Neo4j 连接 + Ping + Close

## 1. 任务概述

建立 Go 应用与 Neo4j CE 的连接，实现 GraphDB 接口的基础方法（构造函数、Ping、Close）。这是所有 Neo4j 操作的起点。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 2: 实现阶段 — 图数据库驱动 |
| 预估工时 | 0.5 天 |
| 前置任务 | D-05 |
| 交付物 | `internal/graph/neo4j.go` 基础部分 |

## 2. 详细实现步骤

**文件**: `internal/graph/neo4j.go`

```go
package graph

import (
    "context"
    "fmt"

    "github.com/neo4j/neo4j-go-driver/v5/neo4j"
    "gitlab.com/pml/network-digital-twin/internal/config"
)

type neo4jClient struct {
    driver    neo4j.DriverWithContext
    defaultDB string
}

// NewNeo4jClient 创建 Neo4j 客户端
func NewNeo4jClient(cfg config.Neo4JConfig) (GraphDB, error) {
    driver, err := neo4j.NewDriverWithContext(
        cfg.URI,
        neo4j.BasicAuth(cfg.User, cfg.Password, ""),
    )
    if err != nil {
        return nil, fmt.Errorf("create neo4j driver: %w", err)
    }

    return &neo4jClient{
        driver:    driver,
        defaultDB: cfg.DefaultDB,
    }, nil
}

func (c *neo4jClient) Ping(ctx context.Context) error {
    return c.driver.VerifyConnectivity(ctx)
}

func (c *neo4jClient) Close() error {
    return c.driver.Close(ctx)
}
```

### 依赖安装

```bash
go get github.com/neo4j/neo4j-go-driver/v5
```

## 3. 设计原理

- **neo4j-go-driver/v5**：官方驱动，支持 `DriverWithContext`（context-aware 操作）
- **`neo4jClient` 结构体**：实现 `GraphDB` 接口，持有 driver 和 defaultDB 名
- **defaultDB 字段**：记录默认逻辑 DB 名（"default"），SnapshotManager 恢复时使用

## 4. 验收标准

- [ ] `NewNeo4jClient(cfg)` 连接 Docker Compose 中的 Neo4j CE 成功
- [ ] `Ping()` 无错误
- [ ] `Close()` 无错误
- [ ] 连接失败时（Neo4j 未启动）返回明确的 error

## 5. 注意事项

- Neo4j Go Driver v5 使用 `neo4j.DriverWithContext`，不是旧版的 `neo4j.Driver`
- `BasicAuth` 的第三个参数是 realm，留空即可
- `VerifyConnectivity` 会实际尝试连接，不只是检查参数
- Close 方法中的 `ctx` 应该使用 `context.Background()` 或传入的 ctx
