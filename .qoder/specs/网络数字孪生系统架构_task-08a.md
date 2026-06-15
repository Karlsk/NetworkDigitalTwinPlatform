# 基于本体论的网络数字孪生系统 — MVP 架构方案

## Context

解决 SRv6 网络运维三大痛点：数据孤岛（控制器 vs 数据中台割裂）、缺乏语义认知（图数据库无语义）、高阶运维断层（RCA 依赖专家经验）。通过 YAML Schema Registry 定义网络本体，插件式 Connector 采集多源数据，归一化后构建 Neo4j 图孪生体，通过 MCP 暴露给外部 Agent 平台。

## 核心决策汇总

| 决策项 | 选型 | 理由 |
|--------|------|------|
| 本体定义 | YAML Schema Registry (K8s CRD 风格) | 单一数据源，新增实体零代码，开发者友好 |
| 快照格式 | YAML Snapshot + Neo4j 逻辑多DB | YAML 归档导出，Neo4j 内可查询/对比/沙盒 |
| 图数据库 | Neo4j CE + 驱动层逻辑多DB | CE 不支持多DB，通过 `_db` 属性实现逻辑隔离 |
| 元数据存储 | MVP 本地文件，V1 引入 PostgreSQL | MVP 保持简洁，后续按需引入 |
| 智能层 | 纯算法引擎 + MCP | RCA/Simulation/Impact 确定性算法，不依赖 LLM |
| n10s | 不依赖 | 直接用 Cypher 构建图，简化部署 |
| Connector | 插件式 Connector + Registry | MVP 只实现 Mock，架构预留扩展点 |
| 项目名 | `network-digital-twin` | Go module: `gitlab.com/pml/network-digital-twin` |

## 系统架构

```
                        外部 Agent 平台
                    (Claude Code / OpenCode)
                              │ MCP (stdio/JSON-RPC)
                              ▼
                    ┌─────────────────────┐
                    │     MCP Server      │
                    │   (工具暴露层)       │
                    └─────────┬───────────┘
                              │
        ┌─────────────────────┼────────────────────┐
        ▼                     ▼                    ▼
 Impact Engine         RCA Engine          Simulation Engine
 (纯算法)              (纯算法)             (内存沙盒)
        │                     │                    │
        └──────────────┬──────┴────────────────────┘
                       │
                       ▼
              Digital Twin Service
              (业务编排层)
                       │
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
   GraphDB         Snapshot        Schema
   (Neo4j CE      Manager         Registry
    逻辑多DB)     (YAML+Neo4j)    (YAML CRD)
                       ▲
                       │
              Normalizer (身份对齐)
                       ▲
                       │
              Connector Registry
              (插件式数据源适配器)
                       ▲
                       │
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
     Netbox       Controller        CMDB
     (Mock)       (Mock)          (Mock)
```

## 项目结构

```
network-digital-twin/
├── cmd/
│   └── server/
│       └── main.go                    # 服务入口 (HTTP + MCP)
│
├── internal/
│   ├── config/
│   │   └── config.go                  # Viper 全局配置加载
│   │
│   ├── schema/                        # Schema Registry (本体定义)
│   │   ├── registry.go                # SchemaRegistry: 加载/注册/查询 EntityType
│   │   ├── types.go                   # EntityType, RelationType, Property 结构体
│   │   └── validator.go               # Schema 校验器 (属性类型/必填/枚举)
│   │
│   ├── connector/                     # Connector 框架 (数据源适配)
│   │   ├── interface.go               # Connector interface + ConnectorRegistry
│   │   ├── types.go                   # Resource, Metadata, CollectResult
│   │   └── mock/
│   │       └── mock.go                # Mock Connector (读取 testdata JSON)
│   │
│   ├── normalizer/                    # 归一化引擎
│   │   ├── normalizer.go              # 身份对齐 + 字段标准化 + URI 生成
│   │   └── resolver.go                # IdentityResolver: IP→设备名, 接口名标准化
│   │
│   ├── builder/                       # 图构建器
│   │   ├── builder.go                 # 读取 Schema → 生成 Cypher CREATE 语句
│   │   └── cypher_gen.go              # Cypher 语句生成器 (UNWIND 批量)
│   │
│   ├── graph/                         # 图数据库驱动层
│   │   ├── interface.go               # GraphDB interface (所有操作带 db 参数)
│   │   ├── neo4j.go                   # Neo4j 实现 (含逻辑多DB封装)
│   │   └── logical_db.go              # 逻辑多DB管理器 (db名注入/清理)
│   │
│   ├── snapshot/                      # 快照管理
│   │   ├── manager.go                 # 快照生命周期管理
│   │   ├── exporter.go                # 图 → YAML 文件导出
│   │   └── importer.go                # YAML 文件 → 图导入
│   │
│   ├── engine/                        # 分析引擎 (纯算法)
│   │   ├── impact.go                  # Impact Engine: 爆炸半径计算
│   │   ├── rca.go                     # RCA Engine: 根因追溯
│   │   └── simulation.go             # Simulation Engine: 内存沙盒仿真
│   │
│   ├── service/                       # 业务编排层
│   │   ├── sync_service.go            # 全量/增量同步编排
│   │   ├── snapshot_service.go        # 快照管理编排
│   │   └── analysis_service.go        # 分析查询编排
│   │
│   └── mcp/                           # MCP Server
│       ├── server.go                  # MCP Server 主体 (stdio)
│       ├── registry.go                # Tool 注册中心
│       └── tools.go                   # 工具实现
│
├── configs/
│   ├── config.yaml                    # 服务全局配置
│   └── connectors.yaml                # Connector 注册配置
│
├── ontology/                          # 本体 Schema 定义 (YAML CRD 风格)
│   ├── device.yaml                    # Device EntityType
│   ├── interface.yaml                 # Interface EntityType
│   ├── srv6_policy.yaml              # SRv6_Policy EntityType
│   ├── evpn_instance.yaml            # EVPN_Instance EntityType
│   ├── network_slice.yaml            # Network_Slice EntityType
│   ├── alarm.yaml                     # Alarm EntityType
│   └── relations.yaml                 # 关系定义 (RelationType)
│
├── testdata/                          # 测试数据
│   ├── mock_netbox/                   # Mock Netbox 数据
│   │   ├── devices.json
│   │   ├── interfaces.json
│   │   └── ...
│   └── golden/                        # Golden Files (期望输出)
│       ├── expected_topology.yaml
│       └── expected_analysis.json
│
├── pkg/
│   └── utils/
│       └── uri.go                     # URI 工具函数
│
├── deploy/
│   └── docker-compose.yml             # Neo4j CE + Go 服务
│
├── go.mod                             # gitlab.com/pml/network-digital-twin
├── Makefile
└── go.sum
```

## 核心模块设计

### Task 1: Schema Registry (本体定义)

**文件**: `ontology/*.yaml`, `internal/schema/`

YAML CRD 风格的实体类型定义，系统启动时自动加载到 SchemaRegistry。

```yaml
# ontology/device.yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
  labels: [Resource, Network]
spec:
  primaryKey: [hostname]
  properties:
    hostname:
      type: string
      required: true
    vendor:
      type: string
    model:
      type: string
    status:
      type: string
      enum: [Up, Down, Maintenance]
      default: "Up"
    device_type:
      type: string
      enum: [Core, Edge, Access]
```

```yaml
# ontology/relations.yaml
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
  name: RUNS_ON_INTERFACE
spec:
  source: [SRv6_Policy]
  target: [Interface]
---
kind: RelationType
metadata:
  name: CARRIED_BY
spec:
  source: [EVPN_Instance]
  target: [SRv6_Policy]
---
kind: RelationType
metadata:
  name: BELONGS_TO_SLICE
spec:
  source: [EVPN_Instance]
  target: [Network_Slice]
---
kind: RelationType
metadata:
  name: OCCURRED_ON
spec:
  source: [Alarm]
  target: [Interface]
```

**SchemaRegistry 接口**:
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

### Task 2: Connector 框架 (数据源适配)

**文件**: `internal/connector/`, `configs/connectors.yaml`

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

```yaml
# configs/connectors.yaml
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
    entity_types: [EVPN_Instance, Network_Slice]
```

### Task 3: 归一化引擎

**文件**: `internal/normalizer/`

```go
type Normalizer struct {
    resolver  *IdentityResolver
    registry  schema.SchemaRegistry
}

// Normalize: Connector 输出的 Resource → 标准化 Resource (URI 已生成, 字段已对齐)
func (n *Normalizer) Normalize(resource Resource) (*NormalizedResource, error)

type NormalizedResource struct {
    Kind       string
    URI        string              // 标准本体 URI
    Properties map[string]any      // 标准化后的属性
    Relations  []NormalizedRelation
}

type IdentityResolver struct {
    ipToNameMap map[string]string
    rules       []NormalizeRule
}
func (r *IdentityResolver) ResolveDeviceURI(raw string) string
func (r *IdentityResolver) ResolveInterfaceURI(device, iface string) string
```

### Task 4: 图构建器 + Neo4j 逻辑多DB

**文件**: `internal/builder/`, `internal/graph/`

```go
// Builder: 读取 Schema Registry，将 NormalizedResource 生成 Cypher
type Builder struct {
    registry schema.SchemaRegistry
}
func (b *Builder) BuildCreateCypher(resources []NormalizedResource) (string, map[string]any)
// 生成: UNWIND $nodes AS n CREATE (:Device {_db: $db, uri: n.uri, ...})

// GraphDB interface: 所有操作带 db 参数
type GraphDB interface {
    Ping(ctx context.Context) error
    Close() error
    // db = "default" 或 快照名
    BulkCreate(ctx context.Context, db string, nodes []Node, rels []Relation) error
    Query(ctx context.Context, db string, cypher string, params map[string]any) ([]map[string]any, error)
    Update(ctx context.Context, db string, label, uri string, props map[string]any) error
    BatchUpdate(ctx context.Context, db string, updates []UpdateEvent) error
    ClearDB(ctx context.Context, db string) error
    CloneDB(ctx context.Context, from, to string) error
    ListDBs(ctx context.Context) ([]string, error)
}
```

### Task 5: 快照管理

**文件**: `internal/snapshot/`

```go
type SnapshotManager struct {
    graph    graph.GraphDB
    snapDir  string  // YAML 快照文件存储目录
}

// 快照写入 Neo4j 逻辑DB + 导出 YAML 文件
func (m *SnapshotManager) Create(ctx context.Context, name string) (*SnapshotMeta, error)
// 1. CloneDB("default", name)  → Neo4j 内创建快照副本
// 2. 导出为 YAML 文件          → 归档

func (m *SnapshotManager) Restore(ctx context.Context, name string) error
// 1. ClearDB("default")
// 2. CloneDB(name, "default")

func (m *SnapshotManager) List(ctx context.Context) ([]SnapshotMeta, error)
func (m *SnapshotManager) Delete(ctx context.Context, name string) error
// 1. ClearDB(name)             → 清理 Neo4j 逻辑DB
// 2. 删除 YAML 文件

func (m *SnapshotManager) Diff(ctx context.Context, a, b string) (*SnapshotDiff, error)
// Cypher 差集查询两个逻辑DB
```

### Task 6: 分析引擎 (纯算法)

**文件**: `internal/engine/`

```go
// Impact Engine: 爆炸半径计算
type ImpactEngine struct { graph graph.GraphDB }
func (e *ImpactEngine) InterfaceImpact(ctx, db, uri string) (*ImpactResult, error)
func (e *ImpactEngine) DeviceImpact(ctx, db, uri string) (*ImpactResult, error)

// RCA Engine: 根因追溯
type RCAEngine struct { graph graph.GraphDB }
func (e *RCAEngine) Analyze(ctx, db string, alarmURI string) (*RCAResult, error)
// 图遍历 + 依赖分析 + 拓扑影响

// Simulation Engine: 内存沙盒
type SimulationEngine struct { graph graph.GraphDB }
func (e *SimulationEngine) SimulateDeviceDown(ctx, deviceURI string) (*SimResult, error)
// 1. 从 Neo4j 加载相关子图到内存
// 2. 在内存中标记 Down
// 3. 计算影响范围 + 路由重计算
// 4. 返回结果 (不写回 Neo4j)
```

### Task 7: MCP Server

**文件**: `internal/mcp/`

MCP 暴露原子工具给外部 Agent:

| 工具名 | 功能 | 调用链 |
|--------|------|--------|
| `get_topology` | 按层查询拓扑 | → GraphDB.Query |
| `get_impacted_services` | 爆炸半径分析 | → ImpactEngine |
| `root_cause` | 根因追溯 | → RCAEngine |
| `simulate_failure` | 故障仿真 | → SimulationEngine |
| `simulate_change` | 变更仿真 | → SimulationEngine |
| `manage_snapshot` | 快照管理 | → SnapshotManager |
| `sync_data` | 触发数据同步 | → SyncService |

### Task 8: 业务编排层

**文件**: `internal/service/`

```go
// SyncService: 编排完整的数据同步流水线
// Connector.Collect → Normalizer.Normalize → Builder.Build → GraphDB.BulkCreate
type SyncService struct {
    registry   *connector.ConnectorRegistry
    normalizer *normalizer.Normalizer
    builder    *builder.Builder
    graph      graph.GraphDB
}
func (s *SyncService) FullSync(ctx) (*SyncResult, error)
func (s *SyncService) IncrementalSync(ctx, connectorName, entityType string) (*SyncResult, error)
```

## MVP 阶段目标

**数据底座优先**，完整打通核心数据流：

```
Connector(Mock) → Normalize → Schema Registry → Builder → Neo4j 写入 → 快照管理 → MCP 查询
```

### MVP 交付物

1. **Schema Registry**: 6 个 EntityType + 5 个 RelationType 的 YAML 定义 + 自动加载
2. **Connector 框架**: 接口 + Registry + Mock 实现 (3 台设备, ~20 节点)
3. **归一化引擎**: 身份对齐 + 字段标准化 + URI 生成
4. **图构建器**: Schema → Cypher UNWIND 批量创建
5. **Neo4j 逻辑多DB**: `_db` 属性隔离 + 驱动层自动注入
6. **快照管理**: 创建/恢复/列表/删除/对比
7. **基础 MCP 工具**: get_topology + manage_snapshot + sync_data
8. **Docker Compose**: Neo4j CE + Go 服务
9. **集成测试**: Mock 数据导入 → 查询验证 → 快照恢复验证

### MVP 不包含 (放 V1)

- RCA Engine / Impact Engine / Simulation Engine
- PostgreSQL
- 真实数据源 Connector (Netbox/Controller/CMDB)
- HTTP API (Gin)
- 可观测性 (OpenTelemetry/Prometheus)
- 定时调度 (gocron)

## 验证方式

1. `go build ./...` 编译通过
2. `go test ./...` 单元测试通过
3. Docker Compose 启动 Neo4j CE + Go 服务
4. Mock 数据全量同步 → Neo4j 查询验证节点/关系数
5. 快照创建 → 修改数据 → 快照恢复 → 验证数据一致
6. MCP 工具调用: get_topology 返回正确拓扑数据
