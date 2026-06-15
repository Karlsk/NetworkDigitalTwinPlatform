# 基于本体论的网络数字孪生系统 — MVP 开发总览

## 项目背景

随着 SRv6、网络切片、Flex-Algo 等下一代网络技术的广泛部署，传统网络运维面临三大核心挑战：

- **数据孤岛**：控制器掌握底层协议路径，数据中台掌握资产与客户业务，两层数据割裂无法跨层穿透分析
- **缺乏语义认知**：传统图数据库仅有节点和线，机器无法理解"Policy 承载 EVPN 业务"等业务逻辑语义
- **高阶运维断层**：故障根因分析依赖专家经验，缺乏割接前的安全沙箱和变更风险预判能力

本项目利用 **本体论（Ontology）** 规范网络世界观，结合 **Neo4j CE + YAML Schema Registry** 构建实时动态的网络数字孪生底座，通过 **MCP 协议** 暴露给外部 Agent 平台（Claude Code / OpenCode），实现自动化根因分析、全网跨层影响范围评估。

## 核心目标

| 能力 | 描述 |
|------|------|
| **本体定义体系** | YAML CRD 风格 Schema Registry，6 个 EntityType + 5 个 RelationType |
| **分层数据流管线** | Connector → Normalizer → GraphAssembler → GraphDB，职责清晰解耦 |
| **双轨数据同步** | 全量同步 (ClearDB + BulkCreate) + 增量同步 (Webhook + Channel 缓冲) |
| **快照回溯** | YAML 归档永久保留 + Neo4j 逻辑 DB 懒加载，支持创建/恢复/对比 |
| **并发保护** | GraphLock (RWMutex) 保证 Restore/FullSync/IncrementalSync 互斥 |
| **MCP 能力网关** | 只读工具 + 写操作工具，统一通过 Service 层编排 |

## 技术栈

| 组件 | 版本/规范 | 用途 |
|------|-----------|------|
| **Go** | 1.21+ | 微服务主体，MCP Server，ETL 引擎 |
| **Neo4j CE** | 2026.03.1 | 图数据库，逻辑多 DB (`_db` 属性隔离) |
| **YAML Schema** | K8s CRD 风格 | 本体 Schema 定义 (EntityType + RelationType) |
| **MCP 协议** | stdio/JSON-RPC 2.0 | Agent 与工具层通信 |
| **Docker Compose** | 3.8 | 本地开发环境编排 |
| **testcontainers-go** | - | Go 集成测试，Neo4j 容器化管理 |

### Go 核心依赖

```
github.com/neo4j/neo4j-go-driver/v5     Neo4j Bolt 协议驱动
github.com/spf13/viper                   配置热加载
gopkg.in/yaml.v3                         YAML 解析
github.com/testcontainers/testcontainers-go  集成测试
```

### 不依赖的组件

| 组件 | 原因 |
|------|------|
| n10s (neosemantics) | 直接用 Cypher 构建图，简化部署 |
| APOC | MERGE/UPSERT 用原生 Cypher 实现 |
| W3C RDF/Turtle | 用 YAML Schema 替代，开发者友好 |
| SHACL | 用 Schema Registry 内置校验替代 |
| PostgreSQL | MVP 用本地文件，V1 再引入 |

## 核心决策

| 决策项 | 选型 | 理由 |
|--------|------|------|
| 本体定义 | YAML Schema Registry (K8s CRD 风格) | 单一数据源，新增实体零代码 |
| 快照格式 | YAML Snapshot + Neo4j 逻辑多DB | YAML 归档导出，Neo4j 内可查询/对比 |
| 图数据库 | Neo4j CE + 驱动层逻辑多DB | `_db` 属性隔离 + 驱动层强制注入 |
| 智能层 | 纯算法引擎 + MCP | 确定性算法，不依赖 LLM |
| Connector | 插件式 Connector + Registry | MVP 只实现 Mock |
| Cypher 生成 | 内聚在 GraphDB 实现中 | 无独立 Builder 层，BuildCypher 预览 |
| 关系合并 | 增量更新 (MERGE) | 只有 delete_relation 事件才删除 |
| URI 设计 | 基于 identity.stableKeys | 不可变标识，URI 永久不变 |

## 项目完整目录结构

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
│   │   └── normalizer.go              # 从 Schema 读取 uriTemplate/fieldMapping/normalize 规则
│   │
│   ├── assembler/                     # GraphAssembler (IR 层)
│   │   ├── assembler.go               # NormalizedResource → GraphModel 组装
│   │   └── types.go                   # GraphModel, Node, Relation 定义
│   │
│   ├── graph/                         # 图数据库驱动层 (Cypher 生成 + 执行)
│   │   ├── interface.go               # GraphDB interface (所有操作带 db 参数)
│   │   ├── neo4j.go                   # Neo4j 实现 (含 Cypher 生成 + 逻辑多DB)
│   │   └── logical_db.go              # 逻辑多DB管理器 (db名注入/清理)
│   │
│   ├── snapshot/                      # 快照管理
│   │   ├── manager.go                 # 快照生命周期管理
│   │   ├── exporter.go                # 图 → YAML 文件导出
│   │   ├── importer.go                # YAML 文件 → 图导入
│   │   └── graphlock.go               # ⭐ GraphLock 并发保护 (sync.RWMutex)
│   │
│   ├── engine/                        # 分析引擎 (V1)
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
│   ├── srv6_policy.yaml               # SRv6_Policy EntityType
│   ├── evpn_instance.yaml             # EVPN_Instance EntityType
│   ├── network_slice.yaml             # Network_Slice EntityType
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
├── docs/
│   ├── 架构设计.md                     # 架构设计文档
│   ├── V1扩展方向.md                   # V1 扩展方向
│   └── TechnicalResearch/
│       ├── Requirements_Analysis.md
│       └── Requirements_Analysis-2.md
│
├── changelog/                          # 开发日志与阶段文档
│   ├── README.md                      # 本文件（总览）
│   ├── 01-infrastructure/
│   │   └── README.md
│   ├── 02-data-engine/
│   │   └── README.md
│   └── 03-intelligent-system/
│       └── README.md
│
├── go.mod                             # gitlab.com/pml/network-digital-twin
├── Makefile
└── go.sum
```

## 数据流管线

```
Connector
  → Resource                          (原始数据：Kind + ID + Properties)
  → Normalizer                         (读 Schema EntityType → 字段标准化 + URI 生成)
  → NormalizedResource                 (归一化节点：Kind + URI + Properties，不含关系)
  → GraphAssembler  ⭐                 (读 relationFields 映射 + RelationType → 推导关系)
  → GraphModel (Nodes + Relations)     (纯图 IR：Label + Props + Edges)
  → GraphDB                            (封装 Cypher 生成 + 执行)
  → Neo4j
```

### 各层职责边界

| 层 | 输入 | 输出 | 依赖 Schema? | 职责 |
|---|------|------|-------------|------|
| Connector | 外部数据源 | Resource | ❌ | 采集原始数据 |
| Normalizer | Resource | NormalizedResource | ✅ EntityType | 字段标准化、URI 生成（只处理节点） |
| GraphAssembler | NormalizedResource | GraphModel | ✅ relationFields + RelationType | 推导关系，组装图节点+图边 |
| GraphDB | GraphModel | 数据库操作 | ❌ | Cypher 生成 + 执行 |

## 阶段概览

### 第一阶段：基础设施搭建 — [详细文档](./01-infrastructure/README.md)

> **本体定义 + 数据流管线 + 图数据库驱动**

建立系统的语义基础（YAML Schema Registry）和数据流管线（Connector → Normalizer → GraphAssembler → GraphDB），实现 Neo4j CE 逻辑多 DB 驱动层。

- 核心产出：6 个 EntityType + RelationType YAML、SchemaRegistry、Connector 框架、Normalizer、GraphAssembler、GraphDB (Neo4j)
- 预估工时：5-7 天

### 第二阶段：数据引擎集成 — [详细文档](./02-data-engine/README.md)

> **同步服务 + 快照管理 + 并发保护**

基于第一阶段的基础设施，构建完整的数据同步和快照管理能力。实现全量/增量同步、Channel 缓冲防丢消息、GraphLock 并发保护、YAML 快照归档。

- 核心产出：SyncService、SnapshotManager、GraphLock、Webhook Handler、ID 稳定性验证
- 预估工时：5-7 天

### 第三阶段：智能系统集成 — [详细文档](./03-intelligent-system/README.md)

> **MCP Server + 业务编排 + 端到端测试**

实现 MCP Server 将工具接口暴露给外部 Agent 平台，完成 Mock 数据导入 → 拓扑查询 → 快照创建/恢复 → 增量同步的端到端集成测试。

- 核心产出：MCP Server (4 个工具)、业务编排层、端到端集成测试
- 预估工时：3-5 天

## 任务总览

| 序号 | 任务名称 | 所属阶段 | 功能描述 |
|------|----------|----------|----------|
| T-01 | 项目初始化 | 第一阶段 | Go module 初始化、Makefile、main.go 骨架 |
| T-02 | Schema Registry | 第一阶段 | 6 个 EntityType + RelationType YAML 定义 + 自动加载 |
| T-03 | Connector 框架 | 第一阶段 | Connector interface + Registry + Mock 实现 |
| T-04 | 归一化引擎 | 第一阶段 | 读 Schema → fieldMapping/normalize/uriTemplate |
| T-05 | GraphAssembler | 第一阶段 | relationFields → 关系推导 → GraphModel |
| T-06 | GraphDB 驱动 | 第一阶段 | Neo4j CE 实现 + 逻辑多 DB + _db 强制注入 |
| T-07 | Docker 环境 | 第一阶段 | Neo4j CE + Go 服务容器编排 |
| T-08 | 第一阶段验收 | 第一阶段 | 编译通过 + Schema 导入 + 接口检查 |
| T-09 | 同步服务 | 第二阶段 | FullSync + IncrementalSync + Channel 缓冲 |
| T-10 | 快照管理 | 第二阶段 | Create/Restore/List/Delete/Diff + EnsureLoaded |
| T-11 | GraphLock | 第二阶段 | RWMutex 并发保护，Restore/FullSync/IncrementalSync 互斥 |
| T-12 | 第二阶段验收 | 第二阶段 | 全量同步 + 增量同步 + 快照恢复 + 并发测试 |
| T-13 | 业务编排层 | 第三阶段 | SyncService/SnapshotService/AnalysisService 封装 |
| T-14 | MCP Server | 第三阶段 | 4 个工具 (query_topology/query_snapshot/sync_data/restore_snapshot) |
| T-15 | 端到端验收 | 第三阶段 | Mock 数据导入 → 查询 → 快照 → 恢复 → 增量同步 |

## 总工时估算

| 阶段 | 步骤数 | 预估工时 | 关键里程碑 |
|------|--------|----------|-----------|
| 第一阶段 | 8 步 | 5-7 天 | Schema 可加载 + Mock 数据可写入 Neo4j |
| 第二阶段 | 4 步 | 5-7 天 | 全量/增量同步 + 快照恢复 + 并发保护 |
| 第三阶段 | 3 步 | 3-5 天 | MCP 4 个工具可用 + 端到端测试通过 |
| **总计** | **15 步** | **13-19 天** | **MVP 可演示** |

## MVP 交付物

1. **Schema Registry**: 6 个 EntityType + 5 个 RelationType 的 YAML 定义 + 自动加载
2. **Connector 框架**: 接口 + Registry + Mock 实现 (3 台设备, ~20 节点)
3. **归一化引擎**: 从 Schema 读取 uriTemplate/fieldMapping/normalize 规则
4. **GraphAssembler (IR 层)**: NormalizedResource → GraphModel 组装
5. **图数据库驱动 (GraphDB)**: Cypher 生成 + 执行，含逻辑多DB、BuildCypher 预览
6. **同步服务**: 全量同步 + 增量同步 (Webhook + Channel 缓冲)
7. **快照管理**: 创建/恢复/列表/删除/对比 (YAML 归档永久保留)
8. **GraphLock 并发保护**: Restore/FullSync/IncrementalSync 写锁互斥
9. **基础 MCP 工具**: query_topology + query_snapshot + sync_data + restore_snapshot
10. **Docker Compose**: Neo4j CE + Go 服务
11. **集成测试**: Mock 数据导入 → 查询验证 → 快照恢复验证

## MVP 不包含 (放 V1)

- RCA Engine / Impact Engine / Simulation Engine
- PostgreSQL 元数据存储
- 真实数据源 Connector (Netbox/Controller/CMDB)
- HTTP API (Gin)
- 可观测性 (OpenTelemetry/Prometheus)
- 定时调度 (gocron)
- 本体继承机制 (extends)
- Kafka 事件流 (替代 Channel 缓冲)
- Agent Skills + 知识库

> 详见 [V1扩展方向.md](../docs/V1扩展方向.md)
