# 第一阶段：基础设施搭建（本体定义 + 数据流管线 + 图数据库驱动）

> Schema 先于数据，接口先于实现，契约先于编码

## 任务索引

### 设计阶段 (Phase 1)

| 任务ID | 任务名称 | 工时 | 文件 |
|--------|---------|------|------|
| D-01 | 项目初始化 + 目录骨架 | 0.5天 | [D-01_项目初始化.md](D-01_项目初始化.md) |
| D-02 | Schema 数据结构 + 本体 YAML 定义 | 2天 | [D-02_Schema数据结构与本体定义.md](D-02_Schema数据结构与本体定义.md) |
| D-03 | 核心接口定义 | 1.5天 | [D-03_核心接口定义.md](D-03_核心接口定义.md) |
| D-04 | 数据流结构体定义 | 1天 | [D-04_数据流结构体定义.md](D-04_数据流结构体定义.md) |
| D-05 | Docker Compose + 全局配置 | 1天 | [D-05_Docker配置与全局配置.md](D-05_Docker配置与全局配置.md) |

### 实现阶段 — 数据流管线 (Phase 2a)

| 任务ID | 任务名称 | 工时 | 文件 |
|--------|---------|------|------|
| I-01 | SchemaRegistry.Load + Get/List 实现 | 1.5天 | [I-01_SchemaRegistry实现.md](I-01_SchemaRegistry实现.md) |
| I-02 | Schema 校验器 (Validator) | 1天 | [I-02_Schema校验器.md](I-02_Schema校验器.md) |
| I-03 | Mock Connector 实现 | 1天 | [I-03_MockConnector实现.md](I-03_MockConnector实现.md) |
| I-04 | 归一化引擎 (Normalizer) | 1.5天 | [I-04_归一化引擎.md](I-04_归一化引擎.md) |
| I-05 | GraphAssembler 节点转换 + 关系推导 | 2天 | [I-05_GraphAssembler节点转换与关系推导.md](I-05_GraphAssembler节点转换与关系推导.md) |
| I-06 | GraphAssembler 孤儿边校验 | 0.5天 | [I-06_GraphAssembler孤儿边校验.md](I-06_GraphAssembler孤儿边校验.md) |

### 实现阶段 — 图数据库驱动 (Phase 2b)

| 任务ID | 任务名称 | 工时 | 文件 |
|--------|---------|------|------|
| I-07 | Neo4j 连接 + Ping + Close | 0.5天 | [I-07_Neo4j连接基础.md](I-07_Neo4j连接基础.md) |
| I-08 | Neo4j _db 注入 + ClearDB + ListDBs + HasDB | 1天 | [I-08_Neo4j逻辑多DB基础设施.md](I-08_Neo4j逻辑多DB基础设施.md) |
| I-09 | Neo4j BulkCreate (全量 CREATE) | 1.5天 | [I-09_Neo4j全量批量创建.md](I-09_Neo4j全量批量创建.md) |
| I-10 | Neo4j Upsert (MERGE + SET +=) | 1天 | [I-10_Neo4j增量Upsert.md](I-10_Neo4j增量Upsert.md) |
| I-11 | Neo4j Delete + CloneDB | 1天 | [I-11_Neo4j删除与克隆方法.md](I-11_Neo4j删除与克隆方法.md) |
| I-12 | Neo4j BuildCypher + 复合索引 | 1天 | [I-12_Neo4j预览与索引.md](I-12_Neo4j预览与索引.md) |

### 测试阶段 (Phase 4)

| 任务ID | 任务名称 | 工时 | 文件 |
|--------|---------|------|------|
| T-01 | Schema Registry 单元测试 | 1天 | [T-01_SchemaRegistry单元测试.md](T-01_SchemaRegistry单元测试.md) |
| T-02~T-05 | Normalizer/Assembler/Connector/GraphDB 测试 | 4.5天 | [T-02_to_T-05_单元测试与集成测试.md](T-02_to_T-05_单元测试与集成测试.md) |

---

## 功能描述

第一阶段是整个系统的语义基础和工程骨架。核心目标是：

1. **建立本体定义体系**：以 YAML CRD 风格定义网络本体（EntityType + RelationType），系统启动时自动加载到 SchemaRegistry
2. **定义数据流管线各层接口**：Connector → Normalizer → GraphAssembler → GraphDB，确保各层职责清晰、解耦
3. **实现 Connector 框架**：插件式数据源适配器接口 + Registry，MVP 阶段只实现 Mock Connector
4. **实现归一化引擎**：读取 Schema EntityType 中的 `uriTemplate`、`fieldMapping`、`normalize` 规则，完成字段标准化和 URI 生成
5. **实现 GraphAssembler (IR 层)**：读取 `relationFields` + `RelationType`，将 `NormalizedResource` 组装为纯图 `GraphModel`
6. **实现图数据库驱动**：GraphDB 接口封装 Cypher 生成 + 执行，含 Neo4j CE 逻辑多 DB (`_db` 属性隔离 + 驱动层强制注入)
7. **搭建 Docker 环境**：Neo4j CE + Go 服务容器编排

本阶段 **不涉及** 具体业务场景、快照管理、同步编排和 MCP 集成。

## 文件清单

### 项目骨架

| 文件路径 | 用途 |
|----------|------|
| `go.mod` | Go module 定义 (`gitlab.com/pml/network-digital-twin`)，声明核心依赖 |
| `go.sum` | 依赖校验文件（自动生成） |
| `Makefile` | 构建/测试/运行/Docker 管理命令集 |
| `cmd/server/main.go` | 服务入口（HTTP + MCP），graceful shutdown |

### 本体 Schema 定义

| 文件路径 | 用途 |
|----------|------|
| `ontology/device.yaml` | Device EntityType（含 `identity.stableKeys`、`uriTemplate`、`relationFields`） |
| `ontology/interface.yaml` | Interface EntityType |
| `ontology/isis.yaml` | ISIS EntityType |
| `ontology/link.yaml` | Link EntityType |
| `ontology/network_slice.yaml` | Network_Slice EntityType |
| `ontology/alarm.yaml` | Alarm EntityType |
| `ontology/relations.yaml` | RelationType 定义（HAS_INTERFACE / CONNECTS_TO / RUNS_ON / ENDPOINT / OCCURRED_ON） |

### 配置文件

| 文件路径 | 用途 |
|----------|------|
| `configs/config.yaml` | 服务全局配置（Neo4j 连接、目录路径等） |
| `configs/connectors.yaml` | Connector 注册配置（name/type/config/entity_types） |

### Schema Registry 实现

| 文件路径 | 用途 |
|----------|------|
| `internal/schema/registry.go` | **SchemaRegistry**：加载 ontology/ 目录下所有 YAML，解析 EntityType + RelationType |
| `internal/schema/types.go` | EntityType / RelationType / PropertySpec / IdentitySpec / RelationFieldSpec 结构体定义 |
| `internal/schema/validator.go` | Schema 校验器（属性类型/必填/枚举/stableKeys 非空校验） |

### Connector 框架

| 文件路径 | 用途 |
|----------|------|
| `internal/connector/interface.go` | **Connector interface** + ConnectorRegistry + ConnectorMetadata |
| `internal/connector/types.go` | Resource 数据结构体（Kind + ID + Properties） |
| `internal/connector/mock/mock.go` | Mock Connector：读取 testdata/ JSON 文件，实现 Connector interface |

### 归一化引擎

| 文件路径 | 用途 |
|----------|------|
| `internal/normalizer/normalizer.go` | **Normalizer**：读 Schema EntityType → 字段映射 + 标准化 + URI 生成 → NormalizedResource（只处理节点，不处理关系） |

### GraphAssembler (IR 层)

| 文件路径 | 用途 |
|----------|------|
| `internal/assembler/assembler.go` | **GraphAssembler**：读 relationFields + RelationType → 推导关系 → 组装 GraphModel（含孤儿边检测） |
| `internal/assembler/types.go` | GraphModel / Node / Relation / ValidationWarning 结构体定义 |

### 图数据库驱动

| 文件路径 | 用途 |
|----------|------|
| `internal/graph/interface.go` | **GraphDB interface**：BulkCreate / Upsert / DeleteRelations / DeleteByURIs / Query / BuildCypher / ClearDB / CloneDB / ListDBs / HasDB |
| `internal/graph/neo4j.go` | **Neo4j 实现**：Cypher 生成 + 执行，含逻辑多 DB 封装 + `_db` 驱动层强制注入 |
| `internal/graph/logical_db.go` | 逻辑多 DB 管理器（db 名注入/清理/列表） |

### Docker 环境

| 文件路径 | 用途 |
|----------|------|
| `deploy/docker-compose.yml` | Neo4j CE + Go 微服务容器编排，健康检查配置 |

### 工具函数

| 文件路径 | 用途 |
|----------|------|
| `pkg/utils/uri.go` | URI 构建、标准化、命名空间前缀处理等工具函数 |

### 各文件详细说明

#### `ontology/device.yaml` — EntityType 完整示例

```yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
  labels: [Resource, Network]
spec:
  identity:                              # ⭐ 不可变身份标识
    stableKeys: [serial_number]          # 序列号/机箱MAC等不会变的字段
  uriTemplate: "device:{serial_number}"  # URI 生成模板 (必须基于 stableKeys)
  fieldMapping:                          # 字段映射 (源字段 → 标准字段)
    mgmt_ip: management_ip
    hw_model: model
  normalize:                             # 字段标准化规则
    - field: hostname
      pattern: " "
      replace: "_"
  relationFields:                        # 关系字段映射 (Properties 字段 → RelationType)
    interfaces:
      relationType: HAS_INTERFACE
    upstream_links:
      relationType: CONNECTS_TO
  properties:
    serial_number: { type: string, required: true }
    hostname: { type: string, required: true }
    vendor: { type: string }
    model: { type: string }
    management_ip: { type: string }
    chassis_mac: { type: string }
    status: { type: string, enum: [Up, Down, Maintenance], default: "Up" }
    device_type: { type: string, enum: [Core, Edge, Access] }
```

**设计原则**：
- `identity.stableKeys` 必须选择**不可变**标识（serial_number / chassis_mac / asset_id）
- `uriTemplate` 只能基于 stableKeys 生成，不能使用可变属性（hostname / IP）
- `relationFields` 声明 Properties 中哪些字段推导关系，**新增关系类型只需修改 YAML，零代码**

#### `ontology/relations.yaml` — RelationType 定义

```yaml
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: HAS_INTERFACE
spec:
  source: [Device]
  target: [Interface]
---
kind: RelationType
metadata:
  name: RUNS_ON
spec:
  source: [ISIS]
  target: [Interface]
---
kind: RelationType
metadata:
  name: ENDPOINT
spec:
  source: [Link]
  target: [Interface]
---
kind: RelationType
metadata:
  name: OCCURRED_ON
spec:
  source: [Alarm]
  target: [Interface]
```

#### `internal/schema/registry.go` — SchemaRegistry Interface

```go
type SchemaRegistry interface {
    Load(dir string) error                    // 加载目录下所有 YAML
    GetEntityType(name string) (*EntityType, error)
    GetRelationType(name string) (*RelationType, error)
    ListEntityTypes() []*EntityType
    ListRelationTypes() []*RelationType
    Validate(entityKind string, props map[string]any) error  // 校验数据合法性
}
```

#### `internal/connector/interface.go` — Connector Interface

```go
type Connector interface {
    Metadata() ConnectorMetadata
    Collect(ctx context.Context, entityType string) ([]Resource, error)
    Stream(ctx context.Context, entityType string) (<-chan Resource, error)
}

type ConnectorMetadata struct {
    Name        string
    Type        string   // "netbox", "controller", "cmdb", "mock"
    EntityTypes []string // 支持的实体类型
}

type Resource struct {
    Kind       string
    ID         string
    Properties map[string]any
}

type ConnectorRegistry struct {
    connectors map[string]Connector
}
func (r *ConnectorRegistry) Register(c Connector)
func (r *ConnectorRegistry) Get(name string) (Connector, error)
func (r *ConnectorRegistry) List() []ConnectorMetadata
```

#### `internal/graph/interface.go` — GraphDB Interface

```go
type GraphDB interface {
    Ping(ctx context.Context) error
    Close() error

    // 全量同步: 批量 CREATE (先 ClearDB 再 BulkCreate)
    BulkCreate(ctx context.Context, db string, nodes []Node, rels []Relation) error

    // 增量同步: UPSERT
    // 节点: MERGE + SET d += $props (属性增量合并)
    // 关系: MERGE (增量更新，不删旧关系)
    Upsert(ctx context.Context, db string, nodes []Node, rels []Relation) error

    // 增量同步: 删除指定关系 (仅收到明确删除事件时调用)
    DeleteRelations(ctx context.Context, db string, rels []Relation) error

    // 增量同步: 按 URI 删除节点和关系 (节点下线时调用)
    DeleteByURIs(ctx context.Context, db string, uris []string) error

    // 查询
    Query(ctx context.Context, db string, cypher string, params map[string]any) ([]map[string]any, error)

    // Cypher 预览: 生成 Cypher 语句但不执行 (用于测试/audit/调试)
    BuildCypher(action string, db string, nodes []Node, rels []Relation, uris []string) (string, map[string]any)

    // 逻辑 DB 管理
    ClearDB(ctx context.Context, db string) error
    CloneDB(ctx context.Context, from, to string) error
    ListDBs(ctx context.Context) ([]string, error)
    HasDB(ctx context.Context, db string) (bool, error)
}
```

**设计原则**：
- Cypher 生成内聚在 GraphDB 实现中，无独立 Builder 层
- `BuildCypher` 方法用于预览生成的 Cypher（测试/audit/调试），与 `Query` 组合使用
- 换图数据库 → 新建 GraphDB 实现，内部生成对应查询语言

#### `_db` 驱动层强制注入机制

Neo4j CE 不支持多 DB，通过 `_db` 属性模拟逻辑多 DB。驱动层**强制注入** `_db` 过滤条件，业务代码不关心 `_db` 过滤。

```go
func (c *neo4jClient) Query(ctx, db, cypher, params) {
    params["_db"] = db  // 驱动层自动注入
}
```

**Cypher 模板规范**：所有查询必须使用 `$_db` 变量 + `(_db, uri)` 复合索引。

## 数据流管线

```
Connector → Resource (原始数据)
  → Normalizer (读 EntityType → 字段标准化 + URI 生成)
  → NormalizedResource (归一化节点：Kind + URI + Properties，不含关系)
  → GraphAssembler ⭐ (读 relationFields + RelationType → 推导关系)
  → GraphModel (Nodes + Relations，纯图 IR)
  → GraphDB (封装 Cypher 生成 + 执行)
  → Neo4j
```

### 各层职责边界

| 层 | 输入 | 输出 | 依赖 Schema? | 职责 |
|---|------|------|-------------|------|
| Connector | 外部数据源 | Resource | ❌ | 采集原始数据 |
| Normalizer | Resource | NormalizedResource | ✅ EntityType | 字段标准化、URI 生成（只处理节点） |
| GraphAssembler | NormalizedResource | GraphModel | ✅ relationFields + RelationType | 推导关系，组装图节点+图边 |
| GraphDB | GraphModel | 数据库操作 | ❌ | Cypher 生成 + 执行 |

## 测试内容

### 单元测试

| 测试文件 | 测试范围 | 测试方法 |
|----------|----------|----------|
| `internal/schema/registry_test.go` | Schema YAML 加载 | 加载 ontology/ 目录 → 验证 6 个 EntityType + RelationType 完整 |
| `internal/schema/validator_test.go` | Schema 校验 | required/enum/type/stableKeys 校验用例 |
| `internal/normalizer/normalizer_test.go` | 归一化引擎 | 输入 Resource → 验证 fieldMapping/normalize/uriTemplate 正确 |
| `internal/assembler/assembler_test.go` | GraphAssembler | 输入 NormalizedResource → 验证 Node + Relation 正确，含孤儿边检测 |
| `internal/graph/neo4j_test.go` | GraphDB Neo4j 实现 | testcontainers-go 启动 Neo4j → BulkCreate/Upsert/Query 验证 |

#### 归一化引擎测试用例

```go
// TC-01: fieldMapping 字段映射
Input:  Resource{Properties: {"mgmt_ip": "10.0.0.1", "hw_model": "ASR9K"}}
Expect: NormalizedResource{Properties: {"management_ip": "10.0.0.1", "model": "ASR9K"}}

// TC-02: normalize 字段标准化
Input:  Resource{Properties: {"hostname": "Router Core 01"}}
Expect: NormalizedResource{Properties: {"hostname": "Router_Core_01"}}

// TC-03: uriTemplate URI 生成
Input:  Resource{Properties: {"serial_number": "SN12345"}}
Expect: NormalizedResource{URI: "device:SN12345"}

// TC-04: required 字段缺失校验
Input:  Resource{Properties: {"hostname": "R1"}}  // 缺少 serial_number
Expect: error (required field missing)

// TC-05: enum 值校验
Input:  Resource{Properties: {"status": "Unknown"}}
Expect: error (invalid enum value)
```

#### GraphAssembler 测试用例

```go
// TC-01: 关系推导
Input:  NormalizedResource{Kind: "Device", URI: "device:SN12345",
        Properties: {interfaces: ["iface:SN12345_GE1"]}}
Expect: Relation{Type: "HAS_INTERFACE", From: "device:SN12345", To: "iface:SN12345_GE1"}

// TC-02: 孤儿边检测 (Warn)
Input:  Device 引用 iface:SN12345_GE2，但该 Interface 不在当前批次
Expect: ValidationWarning{Type: "orphan_edge"}，关系被跳过

// TC-03: 关系字段过滤
Input:  Device Properties 含 interfaces 字段
Expect: Node.Props 中不包含 interfaces（关系字段已过滤）
```

### 集成测试

| 测试文件 | 测试范围 | 测试方法 |
|----------|----------|----------|
| `internal/graph/neo4j_test.go` | Neo4j CE 逻辑多 DB | testcontainers-go → ClearDB + BulkCreate + Query 验证 `_db` 隔离 |

#### Neo4j 集成测试用例

```go
// TC-IT-01: 逻辑多 DB 隔离
Step:  BulkCreate("db1", nodes1) + BulkCreate("db2", nodes2)
Check: Query("db1") 只返回 nodes1 的数据，Query("db2") 只返回 nodes2

// TC-IT-02: _db 驱动层注入
Step:  Query("db1", "MATCH (d:Device) RETURN d", {})
Check: 自动注入 $_db 参数，查询结果只含 db1 数据

// TC-IT-03: Upsert 增量合并
Step:  BulkCreate("default", device1{status: "Up", bandwidth: 100})
       Upsert("default", device1{status: "Down"})
Check: 查询返回 {status: "Down", bandwidth: 100}（bandwidth 保留）

// TC-IT-04: BuildCypher 预览
Step:  BuildCypher("create", "default", nodes, rels, nil)
Check: 返回合法 Cypher 字符串，含 UNWIND + CREATE

// TC-IT-05: 连接池健康
Step:  Ping(ctx)
Check: 无错误返回
```

### 配置验证测试

| 测试范围 | 测试方法 | 预期结果 |
|----------|----------|----------|
| `config.yaml` 加载 | viper.Unmarshal | Config 结构体所有字段非零值 |
| `connectors.yaml` 加载 | yaml.Unmarshal | 至少 2 个 Connector 配置 |
| `ontology/*.yaml` 加载 | SchemaRegistry.Load() | 6 个 EntityType + RelationType 全部解析成功 |

## 验收标准

### 编译与基础检查

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| A-01 | Go 项目编译通过 | `go build ./...` | 无编译错误 |
| A-02 | Lint 通过 | `golangci-lint run` | 无 Error 级别告警 |
| A-03 | 依赖完整 | `go mod tidy` | 无新增/删除的依赖 |

### 本体 Schema 验证

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| B-01 | YAML 语法正确 | SchemaRegistry.Load() | 6 个 EntityType + RelationType 全部加载成功 |
| B-02 | EntityType 覆盖完整 | ListEntityTypes() | 包含 Device/Interface/ISIS/Link/Network_Slice/Alarm |
| B-03 | RelationType 定义完整 | ListRelationTypes() | 包含 HAS_INTERFACE/RUNS_ON/ENDPOINT/OCCURRED_ON |
| B-04 | identity.stableKeys 正确 | 检查每个 EntityType | stableKeys 字段存在且引用不可变标识 |
| B-05 | relationFields 与 RelationType 一致 | 交叉校验 | relationFields 引用的 RelationType 全部存在 |

### 接口契约验证

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| C-01 | SchemaRegistry interface 完整 | 编译期类型检查 | `var _ schema.SchemaRegistry = (*registryImpl)(nil)` 通过 |
| C-02 | Connector interface 完整 | 编译期类型检查 | Mock Connector 正确实现 Connector interface |
| C-03 | GraphDB interface 完整 | 编译期类型检查 | Neo4j 实现正确实现 GraphDB interface |

### 引擎功能验证

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| D-01 | 归一化引擎 | 单元测试 TC-01~TC-05 | 全部通过 |
| D-02 | GraphAssembler | 单元测试 TC-01~TC-03 | 全部通过（含孤儿边检测） |
| D-03 | Neo4j 集成测试 | 集成测试 TC-IT-01~05 | 全部通过 |
| D-04 | 配置加载 | 配置验证测试 3 项 | 全部通过 |

### 环境验证

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| E-01 | Docker Compose 可启动 | `docker-compose up` | Neo4j CE + Go 容器均 healthy |
| E-02 | Neo4j CE 可连接 | `Ping()` | 无错误 |

### 验收门禁

**以下条件全部满足后，方可进入第二阶段：**

- [ ] A-01~A-03：编译与基础检查全部通过
- [ ] B-01~B-05：本体 Schema 验证全部通过
- [ ] C-01~C-03：接口契约验证通过
- [ ] D-01~D-04：引擎功能验证全部通过
- [ ] E-01~E-02：环境验证通过
- [ ] 单元测试覆盖率 ≥ 80%（schema/、normalizer/、assembler/ 包）
