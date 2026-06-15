# 项目开发规范

## 项目概览

**项目名**: `network-digital-twin` — 基于本体论的网络数字孪生系统

**解决的三大痛点**:
1. **数据孤岛**: 控制器 vs 数据中台割裂，多源数据无法统一
2. **缺乏语义认知**: 图数据库无语义，无法支撑高阶分析
3. **高阶运维断层**: RCA 等运维分析依赖专家经验，无法自动化

**核心价值**: 通过 YAML Schema Registry 定义网络本体（K8s CRD 风格），插件式 Connector 采集多源数据，归一化后构建 Neo4j 图孪生体，通过 MCP 暴露给外部 Agent 平台（Claude Code / OpenCode），实现网络拓扑的统一建模、快照管理和智能分析。

**整体架构**: 分层 IR（中间表示）管线设计 — Connector → Normalizer → GraphAssembler → GraphDB → Neo4j，每层只关心自己的转换，通过显式契约解耦。

**Go Module**: `gitlab.com/pml/network-digital-twin`

---

## 技术栈

| 类别 | 选型 | 说明 |
|------|------|------|
| 语言 | Go 1.21+ | 主开发语言 |
| 本体定义 | YAML Schema Registry (K8s CRD 风格) | 单一数据源，新增实体零代码 |
| 图数据库 | Neo4j CE + 驱动层逻辑多 DB | CE 不支持多 DB，通过 `_db` 属性实现逻辑隔离 |
| 快照格式 | YAML Snapshot + Neo4j 逻辑 DB | YAML 归档导出，Neo4j 内可查询/对比/沙盒 |
| 元数据存储 | MVP 本地文件 | V1 引入 PostgreSQL |
| 智能层 | 纯算法引擎 + MCP | RCA/Simulation/Impact 确定性算法，不依赖 LLM |
| 协议 | MCP (stdio JSON-RPC) | 暴露给外部 Agent 平台的工具协议 |
| 配置 | Viper | 全局配置加载 |
| 部署 | Docker Compose | Neo4j CE + Go 服务 |
| Lint | golangci-lint | 代码质量检查 |
| Schema 解析 | `gopkg.in/yaml.v3` | YAML 多文档解析 |

---

## 核心功能模块（MVP 范围）

| # | 模块 | 包路径 | 职责 |
|---|------|--------|------|
| 1 | Schema Registry | `internal/schema/` | YAML 本体定义加载、EntityType/RelationType 注册与查询、Schema 校验 |
| 2 | Connector 框架 | `internal/connector/` | 插件式数据源适配器接口 + Registry + Mock 实现 |
| 3 | 归一化引擎 | `internal/normalizer/` | 字段标准化（fieldMapping）、URI 生成（uriTemplate）、属性类型校验 |
| 4 | GraphAssembler (IR 层) | `internal/assembler/` | NormalizedResource → GraphModel 组装，节点转换 + 关系推导 + 孤儿边校验 |
| 5 | 图数据库驱动 | `internal/graph/` | GraphDB 接口封装，Cypher 生成 + 执行，逻辑多 DB，BuildCypher 预览 |
| 6 | 快照管理 | `internal/snapshot/` | 快照创建/恢复/列表/删除/对比，YAML 归档 + Neo4j 逻辑 DB 懒加载 |
| 7 | GraphLock 并发保护 | `internal/snapshot/graphlock.go` | sync.RWMutex 保护 Restore/FullSync/IncrementalSync 写锁互斥 |
| 8 | 同步服务 | `internal/service/sync_service.go` | 全量同步（ClearDB + BulkCreate）+ 增量同步（Webhook + Channel 缓冲 + Upsert/Delete） |
| 9 | MCP Server | `internal/mcp/` | stdio JSON-RPC 工具暴露层，只读工具 + 写操作工具 |
| 10 | 全局配置 | `internal/config/` | Viper 配置加载，支持环境变量覆盖 |
| 11 | Docker Compose | `deploy/docker-compose.yml` | Neo4j CE + Go 服务编排 |

**MVP 不包含（放 V1）**: RCA/Impact/Simulation Engine、PostgreSQL、真实 Connector、HTTP API (Gin)、可观测性、定时调度、Kafka 事件流。

---

## 代码组织规范

### 包结构

```
network-digital-twin/
├── cmd/server/main.go           # 服务入口 (HTTP + MCP)
├── internal/                    # 私有业务代码，禁止外部导入
│   ├── config/config.go         # Viper 全局配置
│   ├── schema/                  # Schema Registry (本体定义)
│   │   ├── registry.go          # SchemaRegistry 实现
│   │   ├── types.go             # EntityType, RelationType 结构体
│   │   └── validator.go         # Schema 校验器
│   ├── connector/               # Connector 框架
│   │   ├── interface.go         # Connector + ConnectorRegistry 接口
│   │   ├── types.go             # Resource, Metadata 结构体
│   │   └── mock/mock.go         # Mock Connector
│   ├── normalizer/normalizer.go # 归一化引擎
│   ├── assembler/               # GraphAssembler (IR 层)
│   │   ├── assembler.go         # 组装逻辑
│   │   └── types.go             # GraphModel, Node, Relation
│   ├── graph/                   # 图数据库驱动层
│   │   ├── interface.go         # GraphDB 接口
│   │   ├── neo4j.go             # Neo4j 实现
│   │   └── logical_db.go        # 逻辑多 DB 管理
│   ├── snapshot/                # 快照管理
│   │   ├── manager.go           # 快照生命周期
│   │   ├── exporter.go          # 图 → YAML 导出
│   │   ├── importer.go          # YAML → 图导入
│   │   └── graphlock.go         # GraphLock 并发保护
│   ├── engine/                  # 分析引擎 (纯算法，V1 实现)
│   ├── service/                 # 业务编排层
│   │   ├── sync_service.go      # 全量/增量同步编排
│   │   ├── snapshot_service.go  # 快照管理编排
│   │   └── analysis_service.go  # 分析查询编排
│   └── mcp/                     # MCP Server
│       ├── server.go            # Server 主体 (stdio)
│       ├── registry.go          # Tool 注册中心
│       └── tools.go             # 工具实现
├── pkg/utils/uri.go             # 可复用工具函数（URI 生成等）
├── ontology/                    # YAML 本体定义文件
├── configs/                     # 运行配置文件
├── testdata/                    # 测试数据 + Golden Files
└── deploy/                      # 部署配置
```

**包命名规则**:
- `internal/` 下的包禁止被外部项目导入
- 包名 = 目录名，使用小写单词，不用下划线（如 `normalizer` 而非 `norm_engine`）
- 每个包只暴露必要的接口和类型，实现细节不导出（小写开头）
- `pkg/utils/` 只放真正跨项目复用的无状态工具函数

### 各层职责边界

| 层 | 输入 | 输出 | 依赖 Schema? | 职责 | 禁止事项 |
|---|------|------|-------------|------|---------|
| **Connector** | 外部数据源 | `Resource` | ❌ | 采集原始数据 | 不做字段映射、不做校验 |
| **Normalizer** | `Resource` | `NormalizedResource` | ✅ EntityType | 字段标准化、URI 生成（**只处理节点**） | 不处理关系推导 |
| **GraphAssembler** | `NormalizedResource` | `GraphModel` | ✅ EntityType.relationFields + RelationType | 节点转换 + 关系推导 + 孤儿边校验 | 不操作数据库 |
| **GraphDB** | `GraphModel` | 数据库操作 | ❌ | Cypher 生成 + 执行 | 不读 Schema、不做业务判断 |

**数据流管线**:
```
Connector → Resource → Normalizer → NormalizedResource → GraphAssembler → GraphModel → GraphDB → Neo4j
```

**Schema 消费者规则**:
- Normalizer 读 `EntityType`（属性类型、枚举、默认值、fieldMapping、normalize、uriTemplate）
- GraphAssembler 读 `EntityType.relationFields` + `RelationType`
- GraphDB **不读 Schema**，只接收 `GraphModel`
- Validator 读 `EntityType` 校验数据合法性

### 跨模块调用规则

```
MCP Server → Service 层 → Engine / GraphDB / SnapshotManager
                 ↓
           SchemaRegistry ← Normalizer ← Connector
                 ↓
           GraphAssembler → GraphDB
```

**调用规则**:

1. **MCP → Service**: MCP 层只做工具注册和参数路由，**不包含业务逻辑**，统一通过 Service 层编排
2. **Service → 下层**: Service 是编排层，调用 Connector、Normalizer、Assembler、GraphDB、SnapshotManager
3. **SyncService 和 SnapshotManager 共享同一个 GraphLock 实例**，FullSync/IncrementalSync/Restore 三个写操作互斥
4. **Connector 不调用 Normalizer**: Connector 只输出 Resource，由 Service 层串联管线
5. **GraphDB 不知道 Schema**: GraphDB 只接收 GraphModel，与 Schema 完全解耦
6. **Engine 层只读**: ImpactEngine/RCAEngine/SimulationEngine 只通过 GraphDB.Query 读取数据，不写入
7. **依赖注入**: 所有依赖通过构造函数传入，不使用全局变量或 init()

**MCP 工具分类**:

| 类型 | 工具 | 说明 |
|------|------|------|
| 只读 | `query_topology`, `query_snapshot` | 无副作用，Agent 可自由调用 |
| 写操作 | `sync_data`, `restore_snapshot` | 有副作用，需谨慎使用 |

---

## 规范（并发、超时、重试、容错）

### 并发保护 — GraphLock

```go
type GraphLock struct {
    mu sync.RWMutex
}
```

| 操作 | 锁类型 | 说明 |
|------|--------|------|
| `Restore` | 写锁 `Lock()` | ClearDB + CloneDB 期间禁止增量写入 |
| `FullSync` | 写锁 `Lock()` | ClearDB + BulkCreate 期间禁止增量写入 |
| `IncrementalSync` | 写锁 `Lock()` | Upsert/DeleteByURIs 期间禁止其他写入 |
| MCP Query | 读锁 `RLock()` | 查询期间允许其他读，阻塞写 |
| `Snapshot.Create` | 读锁 `RLock()` | 导出期间允许其他读 |

**规则**:
- Restore/FullSync/IncrementalSync 三个写操作**互斥**，同一时刻只有一个能执行
- 锁被持有时，新的写入请求**阻塞等待**（或返回 503）
- 只读查询始终使用读锁，不阻塞其他读操作

### Channel 缓冲防丢

```
Webhook → eventChan (缓冲 channel) → 单协程消费者 → GraphLock → IncrementalSync
```

- Webhook Handler 将事件写入 `eventChan` 后**立即返回 202 Accepted**（毫秒级）
- 单协程串行消费，保证事件顺序，避免并发写入冲突
- Channel 满时返回 `503 Service Unavailable`
- **V1 升级路径**: Channel 替换为 Kafka consumer

### 超时规范

| 场景 | 超时策略 |
|------|---------|
| Neo4j 连接 | `neo4j.WithConnectionTimeout(10 * time.Second)` |
| Neo4j 查询 | 所有 Query 方法必须接收 `context.Context`，通过 ctx 控制超时 |
| FullSync | 外层 ctx 控制，建议 5 分钟超时 |
| Webhook 入队 | 非阻塞 select，channel 满立即返回错误 |
| MCP 工具调用 | 由 MCP Server 框架控制超时 |

### 重试与容错

| 场景 | 策略 |
|------|------|
| Neo4j 连接失败 | 启动时 `waitForNeo4j` 指数退避重试（最多 30 次，间隔 2s） |
| 孤儿边 | 跳过 + Warn 日志，不阻断同步，`SyncResult.OrphanEdgesSkipped++` |
| Schema 校验失败 | 跳过该 Resource + Error 日志，不阻断整批 |
| 增量同步失败 | 记录 Error 日志，事件不重试（V1 引入 Kafka 后支持重试） |
| Channel 满 | 返回 503，由外部系统重试 |

### Fallback 与降级

- Neo4j 不可用时：MCP 只读工具返回错误信息，写操作拒绝入队
- Schema 文件缺失：启动时报错退出（fail-fast），不允许带病运行
- Connector 返回空数据：跳过该 Connector，继续处理其他 Connector

---

## 部署架构

```yaml
# deploy/docker-compose.yml
services:
  neo4j:
    image: neo4j:5-community
    ports: ["7474:7474", "7687:7687"]
    environment:
      NEO4J_AUTH: neo4j/password
    volumes:
      - neo4j_data:/data

  app:
    build: .
    depends_on: [neo4j]
    environment:
      NEO4J_URI: bolt://neo4j:7687
      NEO4J_USER: neo4j
      NEO4J_PASSWORD: password
      ONTOLOGY_DIR: /app/ontology
      SNAPSHOT_DIR: /app/snapshots
    volumes:
      - ./ontology:/app/ontology
      - snapshot_data:/app/snapshots
```

**部署规范**:
- Neo4j CE 使用官方 Docker 镜像，**不依赖 n10s 插件**
- Go 服务使用 multi-stage build（golang → alpine）
- 环境变量通过 `docker-compose.yml` 或 `.env` 文件注入
- ontology 目录通过 volume 挂载，支持热更新 Schema
- 快照目录持久化存储，YAML 归档文件永久保留

---

## 数据库规范

### 通用字段约定（Neo4j 节点属性）

| 字段 | 类型 | 说明 | 必填 |
|------|------|------|------|
| `_db` | string | 逻辑 DB 标识，驱动层强制注入 | ✅ |
| `uri` | string | 节点唯一标识，由 `uriTemplate` + `stableKeys` 生成 | ✅ |

**`_db` 使用规范**:
- 驱动层**强制注入** `_db` 参数，业务代码不手动处理
- 所有 Cypher 模板**必须**使用 `WHERE n._db = $_db` 过滤
- 创建节点时**必须**设置 `_db` 属性：`SET n._db = $_db`
- 默认 DB 名为 `"default"`，快照 DB 名为快照名称

```cypher
-- ✅ 正确: 使用 $_db 变量
MATCH (d:Device) WHERE d._db = $_db RETURN d

-- ❌ 错误: 缺少 _db 过滤
MATCH (d:Device) RETURN d
```

### 索引设计原则

**核心原则**: 每个实体类型**必须**创建 `(_db, uri)` 复合索引。

```cypher
-- 每个 Label 都必须有 (_db, uri) 复合索引
CREATE INDEX device_db_uri FOR (d:Device) ON (d._db, d.uri);
CREATE INDEX interface_db_uri FOR (i:Interface) ON (i._db, i.uri);
CREATE INDEX srv6_policy_db_uri FOR (s:SRv6_Policy) ON (s._db, s.uri);
-- ... 每个 EntityType 都需要
```

**索引规则**:
1. `(_db, uri)` 是唯一必须的复合索引，保证逻辑 DB 内 URI 查找高效
2. 新增 EntityType 时，**必须同步新增对应索引**（在 `ensureIndexes` 方法中维护）
3. MERGE 操作基于 `(_db, uri)` 复合键，保证幂等性
4. 不在高频过滤的属性上盲目加索引，按需评估

### 大表处理策略

| 场景 | 策略 |
|------|------|
| 全量批量创建 | `UNWIND $batch AS row` 批量写入，每批 500 条 |
| 全量删除 | `MATCH (n {_db: $_db}) DETACH DELETE n` 按逻辑 DB 整体清除 |
| 快照导出 | 分页读取（`SKIP/LIMIT`），流式写入 YAML |
| 快照恢复 | `CloneDB` 使用 Cypher 批量复制 |
| 逻辑 DB 清理 | LRU 策略，最多保留 `maxActive`（默认 5）个活跃逻辑 DB |

### 分页查询规范

```go
// GraphDB.Query 接口
Query(ctx context.Context, db string, cypher string, params map[string]any) ([]map[string]any, error)
```

- 查询结果默认返回全量（MVP 数据量小，~20 节点）
- V1 数据量增长后，Cypher 模板中使用 `SKIP $offset LIMIT $limit` 分页
- MCP `query_topology` 工具支持 `limit` 参数
- 快照 Diff 查询使用 Cypher 差集，结果集有限，无需分页

---

## 编码规范（基于 Effective Go 提炼的 20 条核心规范）

### 命名规范

**1. 包名小写短词，与目录名一致**
```go
// ✅ import "gitlab.com/pml/network-digital-twin/internal/normalizer"
package normalizer

// ❌ package Normalizer / package norm_engine
```

**2. 接口用方法名 + er 后缀命名**
```go
// ✅ 单方法接口
type Reader interface { Read(p []byte) (n int, err error) }

// ✅ 多方法接口直接用名词
type GraphDB interface { ... }
type Connector interface { ... }
```

**3. 导出用大写开头，不导出用小写开头，不加 Get 前缀**
```go
// ✅ getter 直接用名词
func (s *SchemaRegistry) GetEntityType(name string) (*EntityType, error)
// 注意: 本项目的 SchemaRegistry 接口方法使用 Get 前缀是为了语义清晰，属于例外

// ✅ 字段访问
owner := obj.Owner()  // 不是 obj.GetOwner()
```

**4. 使用 MixedCaps 而非下划线**
```go
// ✅ GraphAssembler, NormalizedResource, SyncEvent
// ❌ graph_assembler, normalized_resource, sync_event
```

**5. 接收者用类型缩写，1-2 个字符**
```go
// ✅
func (a *GraphAssembler) Assemble(...)
func (n *Normalizer) Normalize(...)
func (s *SyncService) FullSync(...)

// ❌ func (assembler *GraphAssembler) ...
// ❌ func (self *Normalizer) ...
```

### 错误处理

**6. 错误必须处理，不允许静默吞掉**
```go
// ✅ 处理或向上返回
result, err := registry.Load(dir)
if err != nil {
    return fmt.Errorf("load schema from %s: %w", dir, err)
}

// ❌ 忽略错误
result, _ := registry.Load(dir)
```

**7. 使用 `fmt.Errorf` + `%w` 包装错误，携带上下文**
```go
// ✅ 包含操作上下文
return fmt.Errorf("normalize resource %s/%s: %w", resource.Kind, resource.ID, err)

// ❌ 丢失上下文
return err
```

**8. 错误信息小写开头，不加句号**
```go
// ✅
return errors.New("connector not found: " + name)
return fmt.Errorf("invalid entity type %q: missing required field %s", kind, field)

// ❌
return errors.New("Connector not found.")
```

**9. 用 errors.Is / errors.As 做错误判断，不用字符串比较**
```go
// ✅
if errors.Is(err, ErrSchemaNotFound) { ... }

// ❌
if err.Error() == "schema not found" { ... }
```

**10. 定义哨兵错误用于业务判断**
```go
var (
    ErrSchemaNotFound    = errors.New("schema not found")
    ErrConnectorNotFound = errors.New("connector not found")
    ErrEventQueueFull    = errors.New("event queue full")
    ErrDBNotExists       = errors.New("logical db not exists")
)
```

### 日志规范

**11. 使用结构化日志（slog），不用 fmt.Println**
```go
// ✅ 结构化日志
log.Warn("orphan edge skipped",
    "type", rel.Type,
    "from", rel.From,
    "to", rel.To,
)
log.Info("full sync completed",
    "nodes", result.NodesCreated,
    "relations", result.RelationsCreated,
    "duration_ms", elapsed.Milliseconds(),
)

// ❌
fmt.Println("sync done, nodes:", count)
```

**12. 日志级别规范**
| 级别 | 场景 |
|------|------|
| `Debug` | 开发调试信息（单条 Resource 处理详情） |
| `Info` | 关键业务流程（同步开始/完成、快照创建/恢复） |
| `Warn` | 可恢复的异常（孤儿边跳过、Schema 校验单条失败） |
| `Error` | 不可恢复错误（Neo4j 连接失败、全量同步失败） |

**13. 日志中不输出完整大对象，只输出关键标识**
```go
// ✅ 输出关键标识
log.Info("connector collected resources", "connector", c.Metadata().Name, "count", len(resources))

// ❌ 输出全部数据
log.Info("resources:", resources)
```

### 并发编程

**14. 用 context.Context 传递取消信号和超时，不用全局变量**
```go
// ✅ 所有外部操作都接受 ctx
func (s *SyncService) FullSync(ctx context.Context) (*SyncResult, error)
func (g *neo4jClient) Query(ctx context.Context, db string, cypher string, params map[string]any) (...)

// ❌ 使用全局超时
var globalTimeout = 30 * time.Second
```

**15. 用 channel + goroutine 做异步处理，不用回调**
```go
// ✅ 本项目标准模式: Channel 缓冲 + 单协程消费
eventChan := make(chan SyncEvent, 100)
go func() {
    for event := range eventChan {
        s.lock.Lock()
        s.processEvent(ctx, event)
        s.lock.Unlock()
    }
}()
```

**16. 用 sync.RWMutex 区分读写锁，不无脑用 sync.Mutex**
```go
// ✅ 本项目的 GraphLock 模式
type GraphLock struct { mu sync.RWMutex }
func (l *GraphLock) Lock()    { l.mu.Lock() }    // 写操作
func (l *GraphLock) RLock()   { l.mu.RLock() }   // 读操作
```

**17. defer 释放锁和资源，确保异常路径也能释放**
```go
// ✅
func (s *SyncService) FullSync(ctx context.Context) (*SyncResult, error) {
    s.lock.Lock()
    defer s.lock.Unlock()
    // ... 业务逻辑
}

// ❌ 忘记 defer，异常路径可能死锁
s.lock.Lock()
result, err := s.doSync(ctx)
s.lock.Unlock()  // 如果 doSync panic，锁不会释放
```

**18. 关闭 channel 前确保所有发送方已停止**
```go
// ✅ 使用 context 取消 + WaitGroup 等待所有发送方停止
ctx, cancel := context.WithCancel(parentCtx)
defer cancel()
// ... 等待所有 goroutine 结束
close(eventChan)
```

### 其他核心规范

**19. 用 interface 做依赖注入，不用具体实现**
```go
// ✅ 依赖接口
type SyncService struct {
    registry   *connector.ConnectorRegistry
    normalizer *normalizer.Normalizer
    assembler  *assembler.GraphAssembler
    graph      graph.GraphDB       // ← 接口，不是 *neo4jClient
    lock       *snapshot.GraphLock
}
```

**20. 构造函数用 NewXxx 命名，返回接口或指针**
```go
// ✅
func NewSchemaRegistry() *SchemaRegistry { ... }
func NewNeo4jClient(uri, user, password string) (GraphDB, error) { ... }
func NewSyncService(...) *SyncService { ... }
```

---

## 关键约束速查

| 约束 | 说明 |
|------|------|
| URI 不可变 | 由 `identity.stableKeys`（不可变标识）生成，禁止使用 hostname/IP 等可变属性 |
| 新增实体零代码 | 只需新增 YAML 文件 + Schema Registry 自动加载 |
| 新增关系零代码 | 只需修改 YAML `relationFields` + `relations.yaml` |
| GraphDB 不读 Schema | 只接收 GraphModel，与 Schema 完全解耦 |
| 写操作互斥 | FullSync / IncrementalSync / Restore 同一时刻只能一个执行 |
| Cypher 必须带 `_db` | 驱动层强制注入，所有模板必须使用 `$_db` |
| Webhook 立即返回 | 写入 Channel 后返回 202，不阻塞外部系统 |
| 孤儿边不阻断 | 跳过 + Warn，不阻断整批同步 |
| YAML 快照永久保留 | 删除快照只清理 Neo4j 逻辑 DB，YAML 归档不删除 |
| 属性增量合并 | `SET d += $props`，未传入的属性保持不变 |
| 关系增量更新 | MERGE 新增，只有 `delete_relation` 事件才删除 |
