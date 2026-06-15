# 基于本体论的网络数字孪生系统 — MVP 研发计划 (RDP)

> 基于 [架构设计.md](../docs/架构设计.md) 制定，指导项目持续开发工作。
> 
> ✅ 本文档已拆分为独立任务文件，存放在各阶段子目录下。详见下方「任务文件索引」。

## 任务文件索引

各任务的详细 RDP 文件（含实现步骤、设计原理、验收标准、注意事项）已拆分到对应阶段目录：

| 阶段 | 目录 | 任务范围 |
|------|------|----------|
| 基础设施 | [`01-infrastructure/`](01-infrastructure/) | D-01~D-05, I-01~I-12, T-01~T-05 |
| 数据引擎 | [`02-data-engine/`](02-data-engine/) | I-13~I-18, T-06~T-08 |
| 智能系统 | [`03-intelligent-system/`](03-intelligent-system/) | V-01~V-04 |

---

## 1. 总体概况

| 项目 | 说明 |
|------|------|
| 项目名 | `network-digital-twin` |
| Go Module | `gitlab.com/pml/network-digital-twin` |
| 开发人力 | 1-2 人 |
| 总预估工时 | 28-40 人天 |
| 开发周期 | 约 4-6 周 (1人) / 2-3 周 (2人并行) |

## 2. 关键里程碑

| 里程碑 | 目标 | 预计时间 | 验收标准 |
|--------|------|---------|---------|
| **M1: 项目骨架就绪** | 目录结构 + 核心接口定义 + Docker 环境 | 第 1 周 | `go build ./...` 通过，Docker Compose 启动 Neo4j CE |
| **M2: 数据流管线贯通** | Schema → Connector → Normalizer → Assembler → GraphDB 全链路 | 第 2-3 周 | Mock 数据全量同步到 Neo4j，Cypher 查询返回正确节点/关系 |
| **M3: 同步与快照系统** | 全量/增量同步 + 快照管理 + GraphLock 并发保护 | 第 3-4 周 | Webhook 增量更新 + 快照创建/恢复/Diff 功能正常 |
| **M4: MCP 集成 + 端到端验收** | MCP Server + 集成测试 + 端到端闭环 | 第 5-6 周 | 4 个 MCP 工具可用，端到端测试全部通过 |

## 3. 甘特图 (文本形式)

```
Week 1        Week 2        Week 3        Week 4        Week 5        Week 6
├─────────────┼─────────────┼─────────────┼─────────────┼─────────────┤
│ Phase 1     │ Phase 2a    │ Phase 2b    │ Phase 3     │ Phase 4     │
│ 设计+骨架   │ 数据流实现   │ 图数据库+    │ 同步+快照    │ MCP+验收    │
│             │              │ 快照基础     │              │              │
│ D-01~D-05   │ I-01~I-06   │ I-07~I-12   │ I-13~I-18   │ I-19~I-24   │
│             │              │              │              │ T-01~T-08   │
│             │              │              │              │ V-01~V-04   │
├─────────────┼─────────────┼─────────────┼─────────────┼─────────────┤
  M1(骨架)      M2(管线贯通)                 M3(同步快照)   M4(端到端验收)
```

**2 人并行方案** (推荐)：

```
Week 1        Week 2        Week 3
├─────────────┼─────────────┼─────────────┤
│ Person A    │ Person A    │ Person A    │
│ D-01~D-03   │ I-01~I-04   │ I-07~I-10   │
│ I-05~I-06   │ I-13~I-15   │ I-16~I-18   │
│             │              │              │
│ Person B    │ Person B    │ Person B    │
│ D-04~D-05   │ I-07~I-10   │ I-19~I-22   │
│ I-07~I-08   │ I-11~I-12   │ T-01~T-08   │
│             │              │ V-01~V-04   │
├─────────────┼─────────────┼─────────────┤
  M1+部分M2     M2+M3        M3+M4
```

## 4. 任务清单

### Phase 1: 设计阶段 + 项目骨架 (D-01 ~ D-05)

> 目标：建立项目骨架，定义所有核心接口契约，搭建 Docker 开发环境

| 任务ID | 任务名称 | 预估工时 | 前置任务 | 交付物 |
|--------|----------|---------|---------|--------|
| D-01 | 项目初始化 + 目录骨架 | 0.5天 | 无 | 全部目录和空文件 |
| D-02 | Schema 数据结构 + 本体 YAML 定义 | 2天 | D-01 | 7 个 YAML 文件 + 结构体定义 |
| D-03 | 核心接口定义 (SchemaRegistry / Connector / GraphDB) | 1.5天 | D-01 | 3 个 interface.go |
| D-04 | 数据流结构体定义 (Resource / NormalizedResource / GraphModel) | 1天 | D-03 | 4 个 types.go |
| D-05 | Docker Compose + 全局配置 | 1天 | D-01 | docker-compose.yml + config.go |

---

#### D-01: 项目初始化 + 目录骨架

**工时**: 0.5 天
**前置**: 无

**实现内容**:
1. `go mod init gitlab.com/pml/network-digital-twin`
2. 创建完整目录结构（参考架构设计.md "项目结构" 章节）
3. 创建 `cmd/server/main.go` 骨架（HTTP + MCP 入口，graceful shutdown）
4. 创建 `Makefile`（build / test / lint / docker 命令）
5. 创建 `.gitignore`、`.golangci.yml`

**文件清单**:
```
cmd/server/main.go
internal/config/config.go          # 空文件，占位
internal/schema/                   # 空目录
internal/connector/                # 空目录
internal/normalizer/               # 空目录
internal/assembler/                # 空目录
internal/graph/                    # 空目录
internal/snapshot/                 # 空目录
internal/service/                  # 空目录
internal/mcp/                      # 空目录
configs/config.yaml                # 基础配置模板
configs/connectors.yaml            # 空配置模板
ontology/                          # 空目录
pkg/utils/uri.go                   # 空文件
go.mod
Makefile
.golangci.yml
```

**验收标准**:
- `go build ./...` 编译通过
- `golangci-lint run` 无 Error
- 目录结构与架构设计一致

---

#### D-02: Schema 数据结构 + 本体 YAML 定义

**工时**: 2 天
**前置**: D-01

**实现内容**:

**Day 1: 数据结构定义**
- `internal/schema/types.go`: 定义所有 Schema 结构体

```go
type EntityType struct {
    APIVersion string           `yaml:"apiVersion"`
    Kind       string           `yaml:"kind"` // "EntityType"
    Metadata   Metadata         `yaml:"metadata"`
    Spec       EntityTypeSpec   `yaml:"spec"`
}

type EntityTypeSpec struct {
    Identity       IdentitySpec                 `yaml:"identity"`
    URITemplate    string                       `yaml:"uriTemplate"`
    FieldMapping   map[string]string             `yaml:"fieldMapping"`
    Normalize      []NormalizeRule               `yaml:"normalize"`
    RelationFields map[string]RelationFieldSpec  `yaml:"relationFields"`
    Properties     map[string]PropertySpec       `yaml:"properties"`
}

type IdentitySpec struct {
    StableKeys []string `yaml:"stableKeys"`
}

type NormalizeRule struct {
    Field   string `yaml:"field"`
    Pattern string `yaml:"pattern"`
    Replace string `yaml:"replace"`
}

type RelationFieldSpec struct {
    RelationType string `yaml:"relationType"`
}

type PropertySpec struct {
    Type     string   `yaml:"type"`
    Required bool     `yaml:"required"`
    Enum     []string `yaml:"enum"`
    Default  any      `yaml:"default"`
}

type RelationType struct {
    APIVersion string           `yaml:"apiVersion"`
    Kind       string           `yaml:"kind"` // "RelationType"
    Metadata   Metadata         `yaml:"metadata"`
    Spec       RelationTypeSpec `yaml:"spec"`
}

type RelationTypeSpec struct {
    Source []string `yaml:"source"`
    Target []string `yaml:"target"`
}

type Metadata struct {
    Name   string   `yaml:"name"`
    Labels []string `yaml:"labels"`
}
```

**Day 2: 本体 YAML 定义**
- `ontology/device.yaml`: Device EntityType（完整示例，含 identity/uriTemplate/fieldMapping/normalize/relationFields/properties）
- `ontology/interface.yaml`: Interface EntityType
- `ontology/isis.yaml`: ISIS EntityType
- `ontology/link.yaml`: Link EntityType
- `ontology/network_slice.yaml`: Network_Slice EntityType
- `ontology/alarm.yaml`: Alarm EntityType
- `ontology/relations.yaml`: RelationType 定义（HAS_INTERFACE / RUNS_ON / ENDPOINT / OCCURRED_ON）

**注意事项**:
- `identity.stableKeys` 必须选择不可变标识（serial_number / chassis_mac）
- `uriTemplate` 只能引用 stableKeys 中的字段
- `relationFields` 引用的 RelationType 必须在 `relations.yaml` 中有定义
- 每个 EntityType 的 `properties` 必须包含 stableKeys 字段且 `required: true`

**验收标准**:
- `yaml.Unmarshal` 可以正确解析所有 YAML 文件到结构体
- 6 个 EntityType 的 stableKeys 均为不可变标识
- 4 个 RelationType 的 source/target 类型正确

---

#### D-03: 核心接口定义

**工时**: 1.5 天
**前置**: D-01

**实现内容**:

**Day 1: SchemaRegistry + Connector 接口**

`internal/schema/registry.go`:
```go
type SchemaRegistry interface {
    Load(dir string) error
    GetEntityType(name string) (*EntityType, error)
    GetRelationType(name string) (*RelationType, error)
    ListEntityTypes() []*EntityType
    ListRelationTypes() []*RelationType
    Validate(entityKind string, props map[string]any) error
}
```

`internal/connector/interface.go`:
```go
type Connector interface {
    Metadata() ConnectorMetadata
    Collect(ctx context.Context, entityType string) ([]Resource, error)
    Stream(ctx context.Context, entityType string) (<-chan Resource, error)
}

type ConnectorRegistry struct {
    connectors map[string]Connector
}
func (r *ConnectorRegistry) Register(c Connector)
func (r *ConnectorRegistry) Get(name string) (Connector, error)
func (r *ConnectorRegistry) List() []ConnectorMetadata
```

**Day 2 (半天): GraphDB 接口**

`internal/graph/interface.go`:
```go
type GraphDB interface {
    Ping(ctx context.Context) error
    Close() error
    BulkCreate(ctx context.Context, db string, nodes []Node, rels []Relation) error
    Upsert(ctx context.Context, db string, nodes []Node, rels []Relation) error
    DeleteRelations(ctx context.Context, db string, rels []Relation) error
    DeleteByURIs(ctx context.Context, db string, uris []string) error
    Query(ctx context.Context, db string, cypher string, params map[string]any) ([]map[string]any, error)
    BuildCypher(action string, db string, nodes []Node, rels []Relation, uris []string) (string, map[string]any)
    ClearDB(ctx context.Context, db string) error
    CloneDB(ctx context.Context, from, to string) error
    ListDBs(ctx context.Context) ([]string, error)
    HasDB(ctx context.Context, db string) (bool, error)
}
```

**注意事项**:
- GraphDB 的所有写操作方法均带 `db string` 参数（逻辑多 DB）
- `BuildCypher` 用于预览，action 值: "create", "upsert", "delete", "delete_relations"
- Connector.Stream() MVP 阶段返回 `ErrNotImplemented`

**验收标准**:
- 编译期类型检查通过
- 所有接口方法签名与架构设计.md 一致

---

#### D-04: 数据流结构体定义

**工时**: 1 天
**前置**: D-03

**实现内容**:

`internal/connector/types.go`:
```go
type Resource struct {
    Kind       string
    ID         string
    Properties map[string]any
}

type ConnectorMetadata struct {
    Name        string
    Type        string
    EntityTypes []string
}
```

`internal/assembler/types.go`:
```go
type GraphModel struct {
    Nodes     []Node
    Relations []Relation
}

type Node struct {
    Label  string
    URI    string
    Props  map[string]any
}

type Relation struct {
    Type   string
    From   string
    To     string
    Props  map[string]any
}

type ValidationWarning struct {
    Type   string  // "orphan_edge"
    Detail string
}
```

`internal/service/sync_service.go` (仅结构体):
```go
type SyncResult struct {
    NodesCreated       int
    RelationsCreated  int
    OrphanEdgesSkipped int
    Warnings           []ValidationWarning
    Duration           time.Duration
}

type SyncEvent struct {
    Action     string
    EntityType string
    Connector  string
    Data       []map[string]any
    URIs       []string
    Relations  []Relation
}
```

`internal/snapshot/manager.go` (仅结构体):
```go
type SnapshotMeta struct {
    Name      string
    CreatedAt time.Time
    NodeCount int
    RelCount  int
    FilePath  string
}

type SnapshotDiff struct {
    AddedNodes    []Node
    RemovedNodes  []Node
    AddedRels     []Relation
    RemovedRels   []Relation
}
```

`internal/normalizer/normalizer.go` (仅结构体):
```go
type NormalizedResource struct {
    Kind       string
    URI        string
    Properties map[string]any
}
```

**验收标准**:
- 所有结构体字段与架构设计一致
- `go vet ./...` 无问题

---

#### D-05: Docker Compose + 全局配置

**工时**: 1 天
**前置**: D-01

**实现内容**:

`deploy/docker-compose.yml`:
```yaml
services:
  neo4j:
    image: neo4j:2025.03-community
    ports: ["7474:7474", "7687:7687"]
    environment:
      NEO4J_AUTH: neo4j/password
    healthcheck:
      test: ["CMD", "cypher-shell", "-u", "neo4j", "-p", "password", "RETURN 1"]
      interval: 10s
      timeout: 5s
      retries: 5

  app:
    build: .
    depends_on:
      neo4j:
        condition: service_healthy
    environment:
      NEO4J_URI: bolt://neo4j:7687
      NEO4J_USER: neo4j
      NEO4J_PASSWORD: password
```

`internal/config/config.go`:
```go
type Config struct {
    Neo4J    Neo4JConfig
    Server   ServerConfig
    Snapshot SnapshotConfig
    Schema   SchemaConfig
}

type Neo4JConfig struct {
    URI      string
    User     string
    Password string
    DefaultDB string  // "default"
}

type SnapshotConfig struct {
    Dir       string  // YAML 快照存储目录
    MaxActive int     // Neo4j 最大逻辑 DB 数
}

type SchemaConfig struct {
    OntologyDir string  // "ontology/"
}

func Load(path string) (*Config, error)  // Viper 加载
```

`configs/config.yaml`: 服务配置默认值

**验收标准**:
- `docker-compose up` 启动 Neo4j CE，healthcheck 通过
- `Config.Load()` 正确解析 config.yaml

---

### Phase 2: 实现阶段 — 数据流管线 (I-01 ~ I-12)

> 目标：实现 Schema Registry、Connector、Normalizer、GraphAssembler、GraphDB 五个核心模块

| 任务ID | 任务名称 | 预估工时 | 前置任务 | 交付物 |
|--------|----------|---------|---------|--------|
| I-01 | SchemaRegistry.Load + Get/List 实现 | 1.5天 | D-02, D-03 | registry.go |
| I-02 | Schema 校验器 (Validator) | 1天 | I-01 | validator.go |
| I-03 | Mock Connector 实现 | 1天 | D-03, D-04 | mock.go + testdata |
| I-04 | 归一化引擎 (Normalizer) | 1.5天 | I-01, D-04 | normalizer.go |
| I-05 | GraphAssembler 节点转换 + 关系推导 | 2天 | I-01, D-04 | assembler.go |
| I-06 | GraphAssembler 孤儿边校验 | 0.5天 | I-05 | assembler.go 补充 |
| I-07 | Neo4j 连接 + Ping + Close | 0.5天 | D-05 | neo4j.go 基础 |
| I-08 | Neo4j _db 注入 + ClearDB + ListDBs + HasDB | 1天 | I-07 | neo4j.go + logical_db.go |
| I-09 | Neo4j BulkCreate (全量 CREATE) | 1.5天 | I-08 | neo4j.go BulkCreate |
| I-10 | Neo4j Upsert (MERGE + SET +=) | 1天 | I-09 | neo4j.go Upsert |
| I-11 | Neo4j Delete (DeleteByURIs + DeleteRelations) | 1天 | I-10 | neo4j.go Delete 方法 |
| I-12 | Neo4j BuildCypher + Query + 复合索引 | 1天 | I-08 | neo4j.go 补充 |

---

#### I-01: SchemaRegistry.Load + Get/List 实现

**工时**: 1.5 天
**前置**: D-02, D-03

**实现内容**:

`internal/schema/registry.go`:

**Day 1**: Load 方法
- 扫描 `ontology/` 目录下所有 `.yaml` 文件
- 按 `apiVersion` + `kind` 区分 EntityType 和 RelationType
- 支持 `---` 分隔的多文档 YAML（如 `relations.yaml`）
- 解析到内存 map: `entityTypes map[string]*EntityType` + `relationTypes map[string]*RelationType`
- Load 完成后做**交叉校验**：检查所有 `relationFields` 引用的 `RelationType` 是否都已定义

**Day 2 (半天)**: Get/List 方法
- `GetEntityType(name)` → 查 map，不存在返回 `ErrEntityTypeNotFound`
- `GetRelationType(name)` → 查 map
- `ListEntityTypes()` → 遍历 map 返回切片
- `ListRelationTypes()` → 遍历 map 返回切片

**注意事项**:
- YAML 解析使用 `gopkg.in/yaml.v3`
- 多文档 YAML 需要用 `yaml.Decoder` 的 `Decode` 循环读取
- 交叉校验失败时 log.Warn 但不阻止加载（允许先定义 relationFields 再补 RelationType）

**验收标准**:
- Load 成功加载 6 个 EntityType + 4 个 RelationType
- GetEntityType("Device") 返回完整的 EntityType 结构体
- GetEntityType("NotExist") 返回 error

---

#### I-02: Schema 校验器 (Validator)

**工时**: 1 天
**前置**: I-01

**实现内容**:

`internal/schema/validator.go`:

```go
func (r *registryImpl) Validate(entityKind string, props map[string]any) error
```

**校验项**:

| 校验项 | 实现 | 失败行为 |
|--------|------|---------|
| required 字段 | 检查 PropertySpec.Required=true 的字段是否存在 | 返回 error |
| 字段类型 | string/int/float/bool 类型匹配 | 返回 error |
| enum 值 | 值在 Enum 列表中 | 返回 error |
| stableKeys 非空 | identity.stableKeys 对应的字段不为空 | 返回 error |
| 默认值填充 | PropertySpec.Default 不为空且字段缺失 | 自动填充 |

**注意事项**:
- 校验在 Normalizer 中被调用，在生成 NormalizedResource 之前拦截不合法数据
- 返回的 error 应该包含具体的校验失败信息（字段名、期望值、实际值）

**验收标准**:
- required 字段缺失 → 返回明确的 error 信息
- enum 非法值 → 返回 error
- 默认值正确填充

---

#### I-03: Mock Connector 实现

**工时**: 1 天
**前置**: D-03, D-04

**实现内容**:

`internal/connector/mock/mock.go`:

```go
type MockConnector struct {
    name    string
    dataDir string
    types   []string
}

func (m *MockConnector) Metadata() ConnectorMetadata
func (m *MockConnector) Collect(ctx, entityType) ([]Resource, error)
func (m *MockConnector) Stream(ctx, entityType) (<-chan Resource, error) // 返回 ErrNotImplemented
```

`configs/connectors.yaml`:
```yaml
connectors:
  - name: mock-netbox
    type: mock
    config:
      data_dir: testdata/mock_netbox
    entity_types: [Device, Interface]
  - name: mock-cmdb
    type: mock
    config:
      data_dir: testdata/mock_cmdb
    entity_types: [ISIS, Link, Network_Slice]
```

`testdata/mock_netbox/devices.json`: 3 台设备（含 serial_number, hostname, vendor, model, mgmt_ip, status, device_type）
`testdata/mock_netbox/interfaces.json`: ~12 个接口（含 device_serial, if_name, status, bandwidth）
`testdata/mock_cmdb/isis.json`: 2-3 个 ISIS 路由协议实例
`testdata/mock_cmdb/links.json`: 2-3 条链路数据
`testdata/mock_cmdb/network_slices.json`: 1-2 个切片

**注意事项**:
- Mock 数据的字段名需要与 Schema 的 `fieldMapping` 对应（如 devices.json 用 `mgmt_ip`，Schema 映射为 `management_ip`）
- 关系字段（如 `interfaces`、`upstream_links`）在 Mock 数据中以 URI 列表形式存在
- Collect 读取 JSON 文件，解析为 `[]Resource`，`Kind` 由 entityType 参数决定

**验收标准**:
- Mock Connector.Collect("Device") 返回 3 个 Resource
- Mock Connector.Collect("Interface") 返回 ~12 个 Resource
- Stream 返回 ErrNotImplemented

---

#### I-04: 归一化引擎 (Normalizer)

**工时**: 1.5 天
**前置**: I-01, D-04

**实现内容**:

`internal/normalizer/normalizer.go`:

```go
type Normalizer struct {
    registry schema.SchemaRegistry
}

func (n *Normalizer) Normalize(resource Resource) (*NormalizedResource, error)
```

**Day 1**: 核心逻辑
1. 从 SchemaRegistry.GetEntityType(resource.Kind) 获取 Schema
2. **fieldMapping**: 遍历 `spec.fieldMapping`，将 Properties 中的源字段名替换为标准字段名
3. **normalize**: 遍历 `spec.normalize`，对指定字段执行 pattern → replace（字符串替换）
4. **properties**: 调用 SchemaRegistry.Validate() 做类型校验 + 枚举校验 + 默认值填充

**Day 2 (半天)**: URI 生成
5. **uriTemplate**: 基于 `spec.uriTemplate` 模板，用 `identity.stableKeys` 对应的字段值替换变量，生成 URI
   - 如 `uriTemplate: "device:{serial_number}"` + `{serial_number: "SN12345"}` → `"device:SN12345"`
6. 构建 NormalizedResource { Kind, URI, Properties }
   - Properties 中**保留**关系字段（如 `interfaces: [...]`），传递给 GraphAssembler

**注意事项**:
- Normalizer **不处理关系**，只做字段标准化 + URI 生成
- `relationFields` 中的字段保留在 Properties 中，不过滤
- uriTemplate 变量替换使用 `pkg/utils/uri.go` 的工具函数

**验收标准**:
- fieldMapping 正确映射（mgmt_ip → management_ip）
- normalize 正确替换（空格 → 下划线）
- URI 基于 stableKeys 正确生成
- required 字段缺失时返回 error

---

#### I-05: GraphAssembler 节点转换 + 关系推导

**工时**: 2 天
**前置**: I-01, D-04

**实现内容**:

`internal/assembler/assembler.go`:

```go
type GraphAssembler struct {
    registry schema.SchemaRegistry
}

func (a *GraphAssembler) Assemble(resources []NormalizedResource) (*GraphModel, []ValidationWarning, error)
```

**Day 1: 节点转换**
1. 遍历 `resources`，每个 NormalizedResource → 一个 Node
   - `Node.Label = resource.Kind`
   - `Node.URI = resource.URI`
   - `Node.Props = resource.Properties`（**过滤掉** `relationFields` 中声明的关系字段）
2. 构建 URI 索引: `uriIndex map[string]bool`（用于后续孤儿边检测）

**Day 2: 关系推导**
3. 遍历 `resources`，对每个 resource：
   a. 从 SchemaRegistry.GetEntityType(resource.Kind) 获取 `relationFields`
   b. 对每个 relationField（如 `interfaces → HAS_INTERFACE`）：
      - 从 Properties 提取该字段的值（URI 列表）
      - 从 SchemaRegistry.GetRelationType(relationType) 获取类型约束
      - 校验 source 类型是否匹配（resource.Kind 在 RelationType.Source 中）
      - 对每个目标 URI 生成 `Relation{Type, From: resource.URI, To: targetURI}`
4. 返回 GraphModel{Nodes, Relations}

**注意事项**:
- 关系字段在 Properties 中的值是 URI 字符串列表（如 `["iface:SN12345_GE1", "iface:SN12345_GE2"]`）
- 过滤关系字段时需要根据 Schema 的 `relationFields` 键名来过滤
- GraphAssembler 是两阶段批量处理：先建所有节点，再推导所有关系（无先后依赖）

**验收标准**:
- Device → Node 转换正确，Props 不含关系字段
- relationFields 正确推导为 Relation
- RelationType 的 source/target 类型校验生效

---

#### I-06: GraphAssembler 孤儿边校验

**工时**: 0.5 天
**前置**: I-05

**实现内容**:

在 `Assemble()` 方法中，关系推导完成后增加校验步骤：

```go
// 3. 校验: 检查关系目标节点是否存在于 Nodes 中
for _, rel := range relations {
    if !uriIndex[rel.To] {
        warnings = append(warnings, ValidationWarning{
            Type:   "orphan_edge",
            Detail: fmt.Sprintf("%s: %s → %s", rel.Type, rel.From, rel.To),
        })
        log.Warn("orphan edge skipped", "detail", warnings[len(warnings)-1].Detail)
        orphanCount++
        continue  // 跳过该关系
    }
    validRelations = append(validRelations, rel)
}
```

**注意事项**:
- Warn 策略：跳过 + log.Warn + 返回 ValidationWarning，不阻断同步
- SyncResult 中包含 OrphanEdgesSkipped 计数

**验收标准**:
- 目标节点不存在的关系被跳过，返回 ValidationWarning
- 目标节点存在的关系正常生成

---

#### I-07: Neo4j 连接 + Ping + Close

**工时**: 0.5 天
**前置**: D-05

**实现内容**:

`internal/graph/neo4j.go`:

```go
type neo4jClient struct {
    driver neo4j.DriverWithContext
    defaultDB string
}

func NewNeo4jClient(cfg config.Neo4JConfig) (GraphDB, error) {
    driver, err := neo4j.NewDriverWithContext(cfg.URI, neo4j.BasicAuth(cfg.User, cfg.Password, ""))
    if err != nil { return nil, err }
    return &neo4jClient{driver: driver, defaultDB: cfg.DefaultDB}, nil
}

func (c *neo4jClient) Ping(ctx context.Context) error {
    return c.driver.VerifyConnectivity(ctx)
}

func (c *neo4jClient) Close() error {
    return c.driver.Close(ctx)
}
```

**验收标准**:
- 连接 Docker Compose 中的 Neo4j CE 成功
- Ping 无错误

---

#### I-08: Neo4j _db 注入 + ClearDB + ListDBs + HasDB

**工时**: 1 天
**前置**: I-07

**实现内容**:

`internal/graph/neo4j.go`:

```go
func (c *neo4jClient) Query(ctx, db, cypher, params) ([]map[string]any, error) {
    params["_db"] = db  // 驱动层强制注入
    // 执行 cypher
}

func (c *neo4jClient) ClearDB(ctx, db) error {
    // MATCH (n {_db: $_db}) DETACH DELETE n
}

func (c *neo4jClient) ListDBs(ctx) ([]string, error) {
    // MATCH (n) RETURN DISTINCT n._db AS db
}

func (c *neo4jClient) HasDB(ctx, db) (bool, error) {
    // MATCH (n {_db: $_db}) RETURN count(n) > 0
}
```

`internal/graph/logical_db.go`:
```go
// 逻辑多 DB 辅助函数
func ensureDBExists(ctx, client, db) error  // HasDB + ClearDB if exists
func cleanStaleDBs(ctx, client, keepDBs) error
```

**注意事项**:
- 所有 Cypher 操作必须使用 `$_db` 变量
- Query 方法中驱动层自动注入 `params["_db"] = db`

**验收标准**:
- ClearDB("test") 清空 _db="test" 的所有节点
- ListDBs 返回已有的逻辑 DB 列表
- HasDB 正确判断

---

#### I-09: Neo4j BulkCreate (全量 CREATE)

**工时**: 1.5 天
**前置**: I-08

**实现内容**:

`internal/graph/neo4j.go`:

```go
func (c *neo4jClient) BulkCreate(ctx, db, nodes, rels) error
```

**Day 1: 节点批量 CREATE**
```cypher
// 按 Label 分组批量创建
UNWIND $nodes AS n
CREATE (x:Label {_db: $_db, uri: n.uri})
SET x += n.props
```
- 按 Node.Label 分组，每组生成一条 UNWIND + CREATE 语句
- `_db` 属性强制写入
- 使用事务保证原子性

**Day 2 (半天): 关系批量 CREATE**
```cypher
UNWIND $rels AS r
MATCH (a {_db: $_db, uri: r.from})
MATCH (b {_db: $_db, uri: r.to})
CREATE (a)-[:REL_TYPE]->(b)
```
- 按 Relation.Type 分组
- 使用 MATCH 查找源/目标节点（基于 `(_db, uri)` 复合索引）

**注意事项**:
- BulkCreate 前必须先 ClearDB
- 使用 UNWIND 批量操作，避免逐条 INSERT 的性能问题
- 每个 Label/RelType 一条 Cypher 语句，减少 round-trip

**验收标准**:
- 批量创建 ~20 个节点 + ~30 个关系成功
- Cypher 查询可正确返回创建的节点和关系

---

#### I-10: Neo4j Upsert (MERGE + SET +=)

**工时**: 1 天
**前置**: I-09

**实现内容**:

`internal/graph/neo4j.go`:

```go
func (c *neo4jClient) Upsert(ctx, db, nodes, rels) error
```

**节点 Upsert**:
```cypher
UNWIND $nodes AS n
MERGE (x:Label {_db: $_db, uri: n.uri})
SET x += n.props
```
- MERGE 基于 `(_db, uri)` 复合键
- `SET x += n.props` 增量合并属性

**关系 Upsert**:
```cypher
UNWIND $rels AS r
MATCH (a {_db: $_db, uri: r.from})
MATCH (b {_db: $_db, uri: r.to})
MERGE (a)-[:REL_TYPE]->(b)
```
- MERGE 幂等，已存在则跳过
- 不删除旧关系（增量更新策略）

**注意事项**:
- 节点和关系分开执行（先节点后关系，确保目标节点存在）
- MERGE 匹配键是 `(_db, uri)`，不要用其他属性匹配

**验收标准**:
- 新增节点：MERGE 创建成功
- 更新节点：属性增量合并（新属性添加，旧属性保留）
- 新增关系：MERGE 创建成功
- 已存在关系：MERGE 幂等，不重复创建

---

#### I-11: Neo4j Delete 方法

**工时**: 1 天
**前置**: I-10

**实现内容**:

```go
func (c *neo4jClient) DeleteByURIs(ctx, db, uris) error {
    // UNWIND $uris AS uri
    // MATCH (n {_db: $_db, uri: uri})
    // DETACH DELETE n
}

func (c *neo4jClient) DeleteRelations(ctx, db, rels) error {
    // UNWIND $rels AS r
    // MATCH (a {_db: $_db, uri: r.from})-[x:REL_TYPE]->(b {_db: $_db, uri: r.to})
    // DELETE x
}

func (c *neo4jClient) CloneDB(ctx, from, to) error {
    // MATCH (n {_db: $from})
    // CREATE (m:Label {_db: $to, uri: n.uri})
    // SET m += properties(n)
    // 然后复制关系
}
```

**注意事项**:
- DeleteByURIs 使用 `DETACH DELETE`，自动清理关联关系
- DeleteRelations 只删除指定关系，不影响节点
- CloneDB 用于快照恢复：将快照逻辑 DB 复制到 "default"

**验收标准**:
- DeleteByURIs 正确删除节点及关联关系
- DeleteRelations 只删除指定关系
- CloneDB 完整复制节点和关系

---

#### I-12: Neo4j BuildCypher + Query + 复合索引

**工时**: 1 天
**前置**: I-08

**实现内容**:

```go
func (c *neo4jClient) BuildCypher(action, db, nodes, rels, uris) (string, map[string]any) {
    switch action {
    case "create":   // 生成 BulkCreate 的 Cypher
    case "upsert":   // 生成 Upsert 的 Cypher
    case "delete":   // 生成 DeleteByURIs 的 Cypher
    case "delete_relations":  // 生成 DeleteRelations 的 Cypher
    }
}

// 索引创建 (系统启动时调用)
func (c *neo4jClient) ensureIndexes(ctx) error {
    // CREATE INDEX device_db_uri IF NOT EXISTS FOR (d:Device) ON (d._db, d.uri)
    // CREATE INDEX interface_db_uri IF NOT EXISTS FOR (i:Interface) ON (i._db, i.uri)
    // ... 每个 EntityType 一个索引
}
```

**注意事项**:
- BuildCypher 只生成 Cypher 字符串 + params，不执行
- 索引在 Neo4j 实现初始化时自动创建
- 索引必须包含 `_db` 前缀（复合索引）

**验收标准**:
- BuildCypher("create", ...) 返回合法 Cypher 字符串
- 索引创建成功，查询性能正常

---

### Phase 3: 实现阶段 — 同步服务 + 快照管理 (I-13 ~ I-18)

> 目标：实现完整的同步编排、快照生命周期管理和并发保护

| 任务ID | 任务名称 | 预估工时 | 前置任务 | 交付物 |
|--------|----------|---------|---------|--------|
| I-13 | GraphLock 并发保护 | 0.5天 | I-08 | graphlock.go |
| I-14 | SyncService.FullSync 全量同步 | 1.5天 | I-03~I-06, I-09, I-13 | sync_service.go |
| I-15 | SyncService.IncrementalSync + Channel + Webhook | 2天 | I-14 | sync_service.go 补充 |
| I-16 | SnapshotManager.Create + List + Delete | 1.5天 | I-12, I-13 | manager.go + exporter.go |
| I-17 | SnapshotManager.EnsureLoaded + Restore + Diff | 1.5天 | I-16 | manager.go + importer.go |
| I-18 | SnapshotManager.cleanup 懒加载清理 | 0.5天 | I-17 | manager.go 补充 |

---

#### I-13: GraphLock 并发保护

**工时**: 0.5 天
**前置**: I-08

**实现内容**:

`internal/snapshot/graphlock.go`:

```go
type GraphLock struct {
    mu sync.RWMutex
}

func NewGraphLock() *GraphLock
func (l *GraphLock) Lock()
func (l *GraphLock) Unlock()
func (l *GraphLock) RLock()
func (l *GraphLock) RUnlock()
```

**注意事项**:
- SyncService 和 SnapshotManager 必须共享**同一个** GraphLock 实例
- 使用 `sync.RWMutex`，不是 `sync.Mutex`
- 写锁：Restore / FullSync / IncrementalSync
- 读锁：MCP Query / Snapshot.Create

**验收标准**:
- 写锁互斥验证
- 读写锁兼容验证

---

#### I-14: SyncService.FullSync 全量同步

**工时**: 1.5 天
**前置**: I-03, I-04, I-05, I-06, I-09, I-13

**实现内容**:

`internal/service/sync_service.go`:

```go
type SyncService struct {
    registry   *connector.ConnectorRegistry
    normalizer *normalizer.Normalizer
    assembler  *assembler.GraphAssembler
    graph      graph.GraphDB
    lock       *snapshot.GraphLock
    eventChan  chan SyncEvent
}

func NewSyncService(...) *SyncService

func (s *SyncService) FullSync(ctx) (*SyncResult, error) {
    // 1. s.lock.Lock() — 排他锁
    // 2. s.graph.ClearDB(ctx, "default")
    // 3. 遍历所有 Connector:
    //    for _, conn := range registry.List():
    //      for _, et := range conn.Metadata().EntityTypes:
    //        resources := conn.Collect(ctx, et)
    //        allResources = append(allResources, resources...)
    // 4. 归一化:
    //    for _, r := range allResources:
    //      normalized := normalizer.Normalize(r)
    //      allNormalized = append(allNormalized, normalized)
    // 5. 组装: model, warnings := assembler.Assemble(allNormalized)
    // 6. 写入: graph.BulkCreate(ctx, "default", model.Nodes, model.Relations)
    // 7. s.lock.Unlock()
    // 8. 返回 SyncResult
}
```

**注意事项**:
- FullSync 持有写锁期间，增量同步和快照恢复被阻塞
- Connector.Collect 失败时记录日志但继续处理其他 Connector
- Normalizer 返回 error 的数据被跳过（记录 Warning）

**验收标准**:
- 全量同步 Mock 数据后 Neo4j 中存在正确数量的节点和关系
- SyncResult 包含正确的统计信息
- 并发调用 FullSync 时互斥生效

---

#### I-15: SyncService.IncrementalSync + Channel + Webhook

**工时**: 2 天
**前置**: I-14

**实现内容**:

**Day 1: IncrementalSync 方法**

```go
func (s *SyncService) IncrementalSync(ctx, event SyncEvent) (*SyncResult, error) {
    switch event.Action {
    case "update":
        // 1. 构造 Resource 列表
        // 2. Normalizer.Normalize 每条数据
        // 3. Assembler.Assemble → GraphModel
        // 4. GraphDB.Upsert("default", model.Nodes, model.Relations)
    case "delete":
        // GraphDB.DeleteByURIs("default", event.URIs)
    case "delete_relation":
        // GraphDB.DeleteRelations("default", event.Relations)
    }
}
```

**Day 2: Channel 缓冲 + Webhook Handler**

```go
func (s *SyncService) StartConsumer(ctx context.Context) {
    go func() {
        for event := range s.eventChan {
            s.lock.Lock()
            s.IncrementalSync(ctx, event)  // 实际应调用 processEvent
            s.lock.Unlock()
        }
    }()
}

func (s *SyncService) HandleWebhook(event SyncEvent) error {
    select {
    case s.eventChan <- event:
        return nil  // 202 Accepted
    default:
        return errors.New("event queue full")  // 503
    }
}
```

**注意事项**:
- eventChan 使用带缓冲的 channel（如 `make(chan SyncEvent, 100)`）
- 单协程消费保证事件顺序
- Webhook Handler 立即返回 202，不等待处理完成

**验收标准**:
- update 事件正确 MERGE 节点和关系
- delete 事件正确 DELETE 节点
- delete_relation 事件正确删除指定关系
- Channel 缓冲：发送 10 个事件后立即返回，异步处理

---

#### I-16: SnapshotManager.Create + List + Delete

**工时**: 1.5 天
**前置**: I-12, I-13

**实现内容**:

`internal/snapshot/manager.go`:

```go
type SnapshotManager struct {
    graph     graph.GraphDB
    lock      *GraphLock
    snapDir   string
    maxActive int
}

func (m *SnapshotManager) Create(ctx, name) (*SnapshotMeta, error) {
    // 1. m.lock.RLock() — 读锁
    // 2. 查询 default DB 全量数据 (Cypher: MATCH (n {_db: "default"}) RETURN n)
    // 3. 查询 default DB 全量关系
    // 4. 序列化为 YAML 写入 snapDir/{name}.yaml
    // 5. m.lock.RUnlock()
    // 6. 返回 SnapshotMeta{Name, CreatedAt, NodeCount, RelCount, FilePath}
}

func (m *SnapshotManager) List(ctx) ([]SnapshotMeta, error) {
    // 扫描 snapDir 下所有 .yaml 文件
    // 解析 YAML 头部元数据
}

func (m *SnapshotManager) Delete(ctx, name) error {
    // 1. graph.ClearDB(name) — 清理 Neo4j 逻辑 DB
    // 2. YAML 文件保留不删除
}
```

`internal/snapshot/exporter.go`:
```go
func exportToYAML(nodes []map[string]any, rels []map[string]any, meta SnapshotMeta) ([]byte, error)
```

**验收标准**:
- Create 生成 YAML 文件，含正确的元数据
- List 返回所有 YAML 归档
- Delete 清理 Neo4j 但保留 YAML

---

#### I-17: SnapshotManager.EnsureLoaded + Restore + Diff

**工时**: 1.5 天
**前置**: I-16

**实现内容**:

```go
func (m *SnapshotManager) EnsureLoaded(ctx, name) error {
    // 1. graph.HasDB(name) → 已有则跳过
    // 2. 读取 snapDir/{name}.yaml
    // 3. 解析 YAML → nodes + rels
    // 4. graph.BulkCreate(name, nodes, rels) — 写入 Neo4j 逻辑 DB
    // 5. m.cleanup() — 触发懒 loading 清理
}

func (m *SnapshotManager) Restore(ctx, name) error {
    // 1. m.lock.Lock() — 排他锁
    // 2. m.EnsureLoaded(name)
    // 3. graph.ClearDB("default")
    // 4. graph.CloneDB(name, "default")
    // 5. m.lock.Unlock()
}

func (m *SnapshotManager) Diff(ctx, a, b) (*SnapshotDiff, error) {
    // 1. EnsureLoaded(a) + EnsureLoaded(b)
    // 2. Cypher 差集查询:
    //    新增节点: 在 b 不在 a
    //    删除节点: 在 a 不在 b
    //    新增关系: 同理
    //    删除关系: 同理
}
```

`internal/snapshot/importer.go`:
```go
func importFromYAML(filePath string) (nodes []Node, rels []Relation, meta SnapshotMeta, err error)
```

**验收标准**:
- EnsureLoaded 懒加载：第一次从 YAML 读取，第二次直接使用
- Restore 恢复 default DB 到快照状态
- Diff 返回正确的节点/关系差异

---

#### I-18: SnapshotManager.cleanup 懒加载清理

**工时**: 0.5 天
**前置**: I-17

**实现内容**:

```go
func (m *SnapshotManager) cleanup(ctx) error {
    // 1. graph.ListDBs() 获取所有逻辑 DB
    // 2. 过滤掉 "default"
    // 3. 按最近使用时间排序 (需要维护 lastAccess map)
    // 4. 超过 maxActive 个的 ClearDB
}
```

**注意事项**:
- lastAccess 时间戳可以用内存 map 维护（MVP 不持久化）
- cleanup 在 EnsureLoaded 中调用，每次加载新快照后触发

**验收标准**:
- 超过 maxActive 的最旧逻辑 DB 被自动清理
- "default" 永远不会被 cleanup 清理

---

### Phase 4: 测试阶段 (T-01 ~ T-08)

> 目标：覆盖单元测试、集成测试，确保各模块功能正确

| 任务ID | 任务名称 | 预估工时 | 前置任务 | 交付物 |
|--------|----------|---------|---------|--------|
| T-01 | Schema Registry 单元测试 | 1天 | I-01, I-02 | registry_test.go + validator_test.go |
| T-02 | Normalizer 单元测试 | 1天 | I-04 | normalizer_test.go |
| T-03 | GraphAssembler 单元测试 | 1天 | I-05, I-06 | assembler_test.go |
| T-04 | Mock Connector 单元测试 | 0.5天 | I-03 | mock_test.go |
| T-05 | GraphDB Neo4j 集成测试 | 2天 | I-09~I-12 | neo4j_test.go |
| T-06 | SyncService 集成测试 | 1.5天 | I-14, I-15 | sync_test.go |
| T-07 | SnapshotManager 集成测试 | 1.5天 | I-16~I-18 | snapshot_test.go |
| T-08 | GraphLock 并发测试 | 1天 | I-13, I-15, I-17 | graphlock_test.go |

---

#### T-01: Schema Registry 单元测试

**工时**: 1 天
**前置**: I-01, I-02

**测试用例**:
- TC-R01: Load 成功加载所有 YAML（6 EntityType + 5 RelationType）
- TC-R02: GetEntityType("Device") 返回完整结构体
- TC-R03: GetEntityType("NotExist") 返回 error
- TC-R04: GetRelationType("HAS_INTERFACE") 返回 source=[Device], target=[Interface]
- TC-R05: ListEntityTypes 返回 6 个
- TC-R06: Load 交叉校验（relationFields 引用不存在的 RelationType → Warn）

**Validator 测试用例**:
- TC-V01: required 字段缺失 → error
- TC-V02: enum 非法值 → error
- TC-V03: 默认值填充
- TC-V04: stableKeys 非空校验

**验收标准**: 全部通过

---

#### T-02: Normalizer 单元测试

**工时**: 1 天
**前置**: I-04

**测试用例**:
- TC-N01: fieldMapping 映射（mgmt_ip → management_ip）
- TC-N02: normalize 替换（空格 → 下划线）
- TC-N03: uriTemplate 生成（device:SN12345）
- TC-N04: required 字段缺失 → error
- TC-N05: enum 非法值 → error
- TC-N06: 关系字段保留在 Properties 中

**验收标准**: 全部通过

---

#### T-03: GraphAssembler 单元测试

**工时**: 1 天
**前置**: I-05, I-06

**测试用例**:
- TC-A01: 节点转换正确（Label, URI, Props 不含关系字段）
- TC-A02: 关系推导正确（HAS_INTERFACE 生成）
- TC-A03: RelationType source/target 校验
- TC-A04: 孤儿边检测 → ValidationWarning
- TC-A05: 多类型混合批量处理（Device + Interface 同时传入）
- TC-A06: 空 relationFields → 只生成节点，无关系

**验收标准**: 全部通过

---

#### T-04: Mock Connector 单元测试

**工时**: 0.5 天
**前置**: I-03

**测试用例**:
- TC-C01: Metadata 返回正确的 Name/Type/EntityTypes
- TC-C02: Collect("Device") 返回 3 个 Resource
- TC-C03: Collect("Interface") 返回 ~12 个 Resource
- TC-C04: Stream 返回 ErrNotImplemented
- TC-C05: ConnectorRegistry.Register + Get + List

**验收标准**: 全部通过

---

#### T-05: GraphDB Neo4j 集成测试

**工时**: 2 天
**前置**: I-09, I-10, I-11, I-12

**环境**: testcontainers-go 启动 Neo4j CE

**Day 1 测试用例**:
- TC-G01: Ping 连接成功
- TC-G02: ClearDB 清空指定 _db 的数据
- TC-G03: BulkCreate 批量创建节点 + 关系
- TC-G04: _db 隔离验证（db1 和 db2 数据互不干扰）

**Day 2 测试用例**:
- TC-G05: Upsert 节点增量合并（SET +=）
- TC-G06: Upsert 关系 MERGE 幂等
- TC-G07: DeleteByURIs 删除节点 + DETACH DELETE
- TC-G08: DeleteRelations 仅删除指定关系
- TC-G09: BuildCypher 预览返回合法 Cypher
- TC-G10: CloneDB 完整复制

**验收标准**: 全部通过

---

#### T-06: SyncService 集成测试

**工时**: 1.5 天
**前置**: I-14, I-15, T-04, T-05

**测试用例**:
- TC-S01: FullSync 全量同步 Mock 数据 → 验证节点/关系数
- TC-S02: FullSync 孤儿边检测 → OrphanEdgesSkipped > 0
- TC-S03: IncrementalSync update → 属性增量合并
- TC-S04: IncrementalSync delete → 节点删除
- TC-S05: IncrementalSync delete_relation → 关系删除
- TC-S06: Channel 缓冲 → 发送 10 个事件，全部处理
- TC-S07: URI 不可变性验证

**验收标准**: 全部通过

---

#### T-07: SnapshotManager 集成测试

**工时**: 1.5 天
**前置**: I-16, I-17, I-18, T-06

**测试用例**:
- TC-SN01: Create 生成 YAML 归档文件
- TC-SN02: List 返回所有归档
- TC-SN03: Restore 恢复 default DB
- TC-SN04: 快照正反一致性（Create → 修改 → Restore → 验证）
- TC-SN05: Diff 快照对比
- TC-SN06: EnsureLoaded 懒加载
- TC-SN07: Delete 清理 Neo4j 但保留 YAML
- TC-SN08: cleanup 自动清理超出的逻辑 DB

**验收标准**: 全部通过

---

#### T-08: GraphLock 并发测试

**工时**: 1 天
**前置**: I-13, I-15, I-17

**测试用例**:
- TC-L01: 写锁互斥（goroutine A 持有 → B 阻塞）
- TC-L02: 读写锁兼容（多个 RLock 共存）
- TC-L03: Restore 期间 Webhook 不丢失（发送 10 个事件，全部处理）
- TC-L04: FullSync 期间增量同步等待（不报错，阻塞后执行）
- TC-L05: 并发读写无数据竞争（`go test -race`）

**验收标准**: 全部通过，`-race` 无 data race

---

### Phase 5: 验证阶段 (V-01 ~ V-04)

> 目标：MCP Server 实现 + 端到端闭环验证

| 任务ID | 任务名称 | 预估工时 | 前置任务 | 交付物 |
|--------|----------|---------|---------|--------|
| V-01 | MCP Server 实现 + 工具注册 | 1.5天 | I-14~I-18 | server.go + registry.go + tools.go |
| V-02 | MCP 工具集成测试 | 1天 | V-01 | mcp_test.go |
| V-03 | 端到端集成测试 | 1.5天 | T-06~T-08, V-02 | e2e_test.go |
| V-04 | 全量验收 + 文档归档 | 1天 | V-03 | 验收报告 |

---

#### V-01: MCP Server 实现 + 工具注册

**工时**: 1.5 天
**前置**: I-14, I-15, I-16, I-17, I-18

**实现内容**:

**Day 1: MCP Server 框架**

`internal/mcp/server.go`:
```go
type MCPServer struct {
    registry *ToolRegistry
    service  *service.AggregateServices  // 包含 SyncService, SnapshotManager
}

func (s *MCPServer) Start(ctx) error  // stdio JSON-RPC
func (s *MCPServer) handleToolCall(ctx, params) (*ToolResult, error)
func (s *MCPServer) handleListTools() []ToolDescriptor
```

`internal/mcp/registry.go`:
```go
type ToolRegistry struct {
    tools map[string]Tool
}

type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(ctx, params) (*ToolResult, error)
}

type ToolResult struct {
    Success bool
    Data    any
    Summary string
    Error   string
}
```

**Day 2 (半天): 4 个 MCP 工具**

`internal/mcp/tools.go`:

| 工具 | Execute 实现 |
|------|-------------|
| `query_topology` | → service.AnalysisService.Topology() → GraphDB.Query |
| `query_snapshot` | → service.SnapshotService.List/Diff |
| `sync_data` | → service.SyncService.FullSync |
| `restore_snapshot` | → service.SnapshotManager.Restore |

**注意事项**:
- MCP 层不包含业务逻辑，只做参数解析和 Service 调用
- 只读工具加 GraphLock.RLock()，写工具加 GraphLock.Lock()
- ToolResult.Summary 是人类可读的摘要字符串

**验收标准**:
- 4 个工具注册成功
- tools/list 返回 4 个 ToolDescriptor
- 每个工具可正确调用对应的 Service 方法

---

#### V-02: MCP 工具集成测试

**工时**: 1 天
**前置**: V-01

**测试用例**:
- TC-M01: ListTools 返回 4 个工具
- TC-M02: query_topology 返回拓扑数据
- TC-M03: query_snapshot (list) 返回快照列表
- TC-M04: sync_data (full) 触发全量同步
- TC-M05: restore_snapshot 恢复快照
- TC-M06: 参数错误 → ToolResult{Success: false}
- TC-M07: stdio JSON-RPC 消息格式正确

**验收标准**: 全部通过

---

#### V-03: 端到端集成测试

**工时**: 1.5 天
**前置**: T-06, T-07, T-08, V-02

**Day 1: 完整数据流测试**

```go
// TC-E2E-01: 完整数据流
// 1. Docker Compose 启动
// 2. FullSync → 验证节点 ≥ 20, 关系 ≥ 30
// 3. query_topology → 验证拓扑数据
// 4. Create("snap_001") → 验证 YAML 文件
// 5. 增量 update (设备 Down) → 验证状态变更
// 6. Create("snap_002") → 验证新快照
// 7. Diff("snap_001", "snap_002") → 验证差异
// 8. Restore("snap_001") → 验证数据恢复
// 9. query_topology → 验证恢复到 snap_001 状态
```

**Day 2 (半天): 增量同步 + 并发测试**

```go
// TC-E2E-02: 增量同步流程
// 1. FullSync
// 2. Webhook update → 验证
// 3. Webhook delete → 验证
// 4. Webhook delete_relation → 验证

// TC-E2E-03: 并发保护
// 1. Restore 期间发送 5 个 Webhook
// 2. Restore 完成后验证 5 个事件全部处理
```

**验收标准**: 所有 E2E 测试通过

---

#### V-04: 全量验收 + 文档归档

**工时**: 1 天
**前置**: V-03

**验收清单**:

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| 1 | 编译通过 | `go build ./...` | 无错误 |
| 2 | Lint 通过 | `golangci-lint run` | 无 Error |
| 3 | 单元测试全部通过 | `go test ./...` | 0 failures |
| 4 | 集成测试全部通过 | `go test -tags=integration ./...` | 0 failures |
| 5 | Race 检测 | `go test -race ./...` | 无 data race |
| 6 | 代码覆盖率 | `go test -cover ./...` | ≥ 70% |
| 7 | Docker Compose 启动 | `docker-compose up` | Neo4j + App healthy |
| 8 | 全量同步 | FullSync | ≥ 20 节点, ≥ 30 关系 |
| 9 | 增量同步 | Webhook update/delete/delete_relation | 正确应用 |
| 10 | 快照创建 | Create | YAML 文件生成 |
| 11 | 快照恢复 | Restore | 数据恢复 |
| 12 | MCP 工具 | 4 个工具 | 全部可用 |
| 13 | 孤儿边检测 | 注入脏数据 | Warn 但不阻断 |
| 14 | URI 不可变 | 修改 hostname | URI 不变 |

**文档归档**:
- 更新架构设计.md 中的"验证方式"章节
- 更新 changelog/ 各阶段 README 的验收状态
- 记录已知问题和 V1 技术债

---

## 5. 任务依赖关系图

```
Phase 1 (设计)          Phase 2a (数据流)         Phase 2b (图DB)
D-01 ──────┬──→ D-02 ───→ I-01 ─┬──→ I-02        I-07 → I-08 → I-09 → I-10 → I-11
           │         │          ├──→ I-04               │
           │         │          ├──→ I-05 → I-06        │
           ├──→ D-03─┤                                  │
           │         └──→ I-03                          │
           └──→ D-05 ─────────────────────────────────→│

Phase 3 (同步+快照)     Phase 4 (测试)           Phase 5 (MCP+验收)
I-08 → I-13            T-01 (Schema)            V-01 (MCP Server)
I-13 + I-03~I-09 → I-14  T-02 (Normalizer)      V-02 (MCP 测试)
I-14 → I-15            T-03 (Assembler)         V-03 (E2E)
I-12 + I-13 → I-16     T-04 (Connector)         V-04 (验收)
I-16 → I-17 → I-18     T-05 (GraphDB)
                       T-06 (SyncService)
                       T-07 (Snapshot)
                       T-08 (GraphLock)
```

## 6. 风险评估与应对

| 风险 | 概率 | 影响 | 应对措施 |
|------|------|------|---------|
| **Neo4j CE 逻辑多 DB 性能** | 中 | 中 | 使用 `(_db, uri)` 复合索引；大数据量时分批 BulkCreate；监控查询延迟 |
| **GraphLock 死锁** | 低 | 高 | 严格锁顺序（只有一种锁 GraphLock）；超时机制；`-race` 检测 |
| **YAML 解析多文档兼容性** | 低 | 低 | 使用 `yaml.Decoder` 循环 Decode；relations.yaml 拆分为单文件多文档 |
| **Channel 缓冲溢出** | 中 | 中 | 设置合理 buffer size (100)；监控 channel 使用率；溢出时返回 503 |
| **孤儿边比例过高** | 中 | 低 | Warn 策略不阻断；SyncResult.OrphanEdgesSkipped 可观测；告警阈值监控 |
| **Mock 数据与 Schema 不一致** | 中 | 中 | I-03 中 Mock 数据字段严格对照 Schema fieldMapping；Schema 交叉校验 |
| **CloneDB 大数据量性能** | 中 | 中 | CloneDB 使用 UNWIND 批量操作；限制 maxActive 逻辑 DB 数量 |
| **MCP stdio 调试困难** | 低 | 低 | BuildCypher 提供 Cypher 预览；日志输出到 stderr（不干扰 stdio） |

## 7. 技术债务清单 (V1 偿还)

| 债务项 | 当前状态 | V1 升级方案 |
|--------|---------|------------|
| Connector.Stream() 未实现 | 返回 ErrNotImplemented | Kafka consumer 适配 |
| 内存 Channel 缓冲 | 进程重启丢失 | Kafka 持久化 + 重试 |
| 快照元数据存内存 | 进程重启丢失 | PostgreSQL 持久化 |
| 无 HTTP API | 只有 MCP stdio | Gin HTTP API (Webhook 端点) |
| 无定时调度 | 手动触发 FullSync | gocron 定时触发 |
| 无本体继承 | 扁平 EntityType | extends + 多 Label |
| 无分析引擎 | MCP 工具只有 4 个 | Impact/RCA/Simulation Engine |
| Builder 未接口化 | 内聚在 GraphDB | 如需多图数据库再抽接口 |

## 8. 每日站会检查项

每个任务完成后，回答以下问题：

1. **编译检查**: `go build ./...` 是否通过？
2. **测试检查**: 相关单元测试是否全部通过？
3. **接口一致性**: 实现是否与架构设计.md 中的接口定义一致？
4. **文档同步**: 如有架构变更，是否已更新架构设计.md？
5. **代码质量**: `golangci-lint run` 是否有新增 Error？
