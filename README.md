# Network Digital Twin

**基于本体论的网络数字孪生平台**

通过 YAML Schema Registry 定义网络本体（K8s CRD 风格），插件式 Connector 采集多源网络数据，经归一化引擎处理后构建 Neo4j 图孪生体，并通过 MCP（Model Context Protocol）暴露给外部 Agent 平台（Claude Code / OpenCode），实现网络拓扑的统一建模、快照管理和智能分析。

## 解决的核心问题

| 痛点 | 描述 |
|------|------|
| **数据孤岛** | 控制器与数据中台割裂，多源网络数据无法统一 |
| **缺乏语义认知** | 传统图数据库缺乏语义层，无法支撑高阶分析 |
| **高阶运维断层** | RCA 等运维分析依赖专家经验，无法自动化 |

## 核心特性

- **本体驱动建模** — K8s CRD 风格的 YAML Schema 定义网络实体与关系，新增实体类型零代码
- **插件式数据采集** — ConnectorFactory 模式，支持 Mock / Netbox / CMDB / Controller 等多种数据源，配置驱动即插即用
- **分层 IR 管线** — `Connector → Normalizer → GraphAssembler → GraphDB → Neo4j`，每层通过显式契约解耦
- **逻辑多 DB** — Neo4j CE 通过 `_db` 属性实现逻辑数据库隔离，支持多租户和快照沙盒
- **快照管理** — 支持创建/恢复/列表/删除/对比（Diff），YAML 归档永久保留，Neo4j 逻辑 DB 懒加载
- **增量同步** — Webhook 事件驱动 + Channel 缓冲 + 单协程消费，保证事件顺序和幂等性
- **MCP 工具协议** — stdio / Streamable HTTP JSON-RPC，暴露只读查询和写操作工具给外部 Agent
- **MetaCache 缓存** — `List()` 快照元数据内存缓存，100 快照查询 < 2μs
- **并发安全** — GraphLock（`sync.RWMutex`）保护 FullSync / IncrementalSync / Restore 写操作互斥
- **属性级 Diff** — 本地 Diff + Cypher Diff 双引擎，精确到属性粒度的变更对比

## 技术栈

| 类别 | 选型 | 说明 |
|------|------|------|
| 语言 | Go 1.21+ | 主开发语言 |
| 本体定义 | YAML Schema Registry (K8s CRD 风格) | 单一数据源，`gopkg.in/yaml.v3` 多文档解析 |
| 图数据库 | Neo4j CE + 驱动层逻辑多 DB | CE 通过 `_db` 属性实现逻辑隔离 |
| MCP SDK | `modelcontextprotocol/go-sdk` | 官方 Go SDK |
| Neo4j 驱动 | `neo4j/neo4j-go-driver/v5` | 官方 Go 驱动 |
| 配置 | Viper (`spf13/viper`) | 分层配置，支持环境变量覆盖 |
| 测试 | testify + Go test | 单元测试 + 集成测试 + 端到端测试 |
| URI 模板 | `yosida95/uritemplate/v3` | RFC 6570 URI 模板解析 |
| 部署 | Docker Compose | Neo4j CE + Go 服务 multi-stage build |
| 代码质量 | golangci-lint | 静态分析与代码规范检查 |

## 架构概览

### 数据流管线

```
┌───────────┐    ┌────────────┐    ┌─────────────────┐    ┌─────────┐    ┌───────┐
│ Connector │───▶│ Normalizer │───▶│ GraphAssembler  │───▶│ GraphDB │───▶│ Neo4j │
│ (采集)    │    │ (归一化)   │    │ (节点+关系组装) │    │ (Cypher)│    │       │
└───────────┘    └────────────┘    └─────────────────┘    └─────────┘    └───────┘
```

### 各层职责

| 层 | 输入 | 输出 | 依赖 Schema | 职责 |
|---|------|------|:-----------:|------|
| **Connector** | 外部数据源 | `Resource` | ❌ | 采集原始数据 |
| **Normalizer** | `Resource` | `NormalizedResource` | ✅ | 字段标准化、URI 生成 |
| **GraphAssembler** | `NormalizedResource` | `GraphModel` | ✅ | 节点转换 + 关系推导 + 孤儿边校验 |
| **GraphDB** | `GraphModel` | 数据库操作 | ❌ | Cypher 生成与执行 |

### 模块调用关系

```
MCP Server → Service 层 → Engine / GraphDB / SnapshotManager
                 ↓
           SchemaRegistry ← Normalizer ← Connector
                 ↓
           GraphAssembler → GraphDB
```

## 快速开始

### 前置条件

- Go 1.21+
- Docker & Docker Compose
- Make（可选）

### 1. 启动 Neo4j

```bash
docker-compose -f deploy/docker-compose.yml up -d neo4j
```

等待 Neo4j 健康检查通过（约 10-30 秒），可通过 http://localhost:7474 访问 Neo4j Browser。

### 2. 启动服务

```bash
# 使用 Makefile
make mcp-server

# 或直接运行
go run cmd/server/main.go
```

服务默认监听 `:8080` 端口提供 MCP Streamable HTTP 服务。

### 3. 运行 Pipeline Demo

端到端演示完整数据流管线：

```bash
make pipeline-demo
# 或
go run cmd/pipeline-demo/main.go
```

Demo 将依次执行：配置加载 → Neo4j 连接 → 索引创建 → Schema 加载 → Connector 采集 → 归一化 → 图组装 → BulkCreate 写入 → 查询验证 → 逻辑多 DB 隔离 → Upsert 增量更新 → 快照生命周期（Create → Diff → Restore）

### 4. Docker Compose 一键部署

```bash
make docker-up    # 启动 Neo4j + App
make docker-down  # 停止所有服务
```

## 配置说明

### 全局配置 (`configs/config.yaml`)

```yaml
neo4j:
  uri: "bolt://localhost:7687"      # Neo4j 连接地址
  user: "neo4j"                      # 用户名
  password: "password123"            # 密码
  default_db: "neo4j"                # 默认逻辑 DB

server:
  port: 8080                         # MCP HTTP 端口

snapshot:
  dir: "snapshots"                   # 快照存储目录
  max_active: 5                      # 最大活跃逻辑 DB 数

schema:
  ontology_dir: "ontology"           # 本体定义文件目录

channel:
  buffer_size: 100                   # 增量同步事件 Channel 缓冲大小
```

所有配置项均支持**环境变量覆盖**（Viper 自动映射）。

### Connector 配置 (`configs/connectors.yaml`)

```yaml
connectors:
  - name: mock-netbox           # Connector 名称
    type: mock                  # 类型: mock / netbox / controller / cmdb
    config:
      data_dir: testdata/mock_netbox
    entity_types: [Device, Interface]

  - name: netbox-1
    type: netbox
    config:
      base_url: "https://netbox.example.com/api"
      api_token: "your-api-token"
    entity_types: [Device, Interface]
```

支持通过配置文件批量创建和注册 Connector，新增数据源只需添加配置条目。

## 本体定义 (Ontology)

本体文件位于 `ontology/` 目录，采用 K8s CRD 风格的 YAML 格式。

### 实体类型定义

```yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
  labels: [Resource, Network]
spec:
  extends: Resource                    # 支持继承
  identity:
    stableKeys: [serial_number]        # 不可变标识键
  uriTemplate: "device:{serial_number}" # URI 模板
  fieldMapping:                        # 字段映射
    mgmt_ip: management_ip
  properties:
    serial_number:
      type: string
      required: true
    status:
      type: string
      enum: [Up, Down, Maintenance]
      default: "Up"
```

### 关系类型定义

```yaml
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: HAS_INTERFACE
spec:
  source: [Device]
  target: [Interface]
```

**新增实体类型只需添加 YAML 文件，无需修改任何代码。**

### 内置实体类型

| 实体 | 文件 | 说明 |
|------|------|------|
| Device | `device.yaml` | 网络设备（路由器、交换机） |
| Interface | `interface.yaml` | 网络接口 |
| Link | `link.yaml` | 物理链路 |
| ISIS | `isis.yaml` | ISIS 路由协议 |
| BGP | `bgp.yaml` | BGP 路由协议 |
| Alarm | `alarm.yaml` | 告警事件 |
| VPN | `vpn.yaml` | VPN 服务 |
| Tunnel | `tunnel.yaml` | 隧道 |
| Network Slice | `network_slice.yaml` | 网络切片 |
| Resource | `resource.yaml` | 基类实体 |

## MCP 工具

| 工具 | 类型 | 说明 |
|------|:----:|------|
| `query_topology` | 只读 | 查询网络拓扑（支持 Cypher 和 Label 过滤） |
| `query_snapshot` | 只读 | 查询快照中的拓扑数据 |
| `sync_data` | 写操作 | 触发数据全量/增量同步 |
| `restore_snapshot` | 写操作 | 恢复到指定快照 |

## 目录结构

```
├── cmd/                          # 应用入口
│   ├── server/                   # MCP 服务主入口
│   ├── pipeline-demo/            # 端到端管线演示
│   ├── mcp-client/               # MCP 客户端测试工具
│   ├── api-test/                 # API 测试工具
│   └── fullsync-test/            # 全量同步测试工具
├── internal/                     # 私有业务代码
│   ├── config/                   # Viper 全局配置
│   ├── schema/                   # Schema Registry（本体定义加载与校验）
│   ├── connector/                # Connector 框架（接口 + 工厂 + 实现）
│   │   ├── mock/                 # Mock Connector（本地文件数据源）
│   │   ├── netbox/               # Netbox Connector
│   │   ├── cmdb/                 # CMDB Connector
│   │   └── controller/           # Controller Connector（真实设备）
│   ├── normalizer/               # 归一化引擎（字段标准化 + URI 生成）
│   ├── assembler/                # GraphAssembler（节点转换 + 关系推导）
│   ├── graph/                    # 图数据库驱动层（Neo4j + 逻辑多 DB）
│   ├── snapshot/                 # 快照管理（创建/恢复/对比/GraphLock）
│   ├── engine/                   # 分析引擎（RCA/Impact/Simulation）
│   ├── service/                  # 业务编排层（Sync/Snapshot/Analysis）
│   └── mcp/                      # MCP Server（工具注册与实现）
├── pkg/utils/                    # 可复用工具函数
├── ontology/                     # YAML 本体定义文件
├── configs/                      # 运行配置文件
├── testdata/                     # 测试数据 + Golden Files
├── snapshots/                    # 快照 YAML 归档
├── changelog/                    # 开发变更日志
├── specs/                        # 需求规格文档
├── deploy/                       # 部署配置（Dockerfile + docker-compose）
├── docs/                         # 技术文档
├── AGENTS.md                     # AI Agent 开发规范
├── Makefile                      # 构建与测试命令
├── go.mod / go.sum               # Go 模块依赖
└── .golangci.yml                 # Lint 配置
```

## 贡献指南

欢迎贡献代码！请遵循以下流程和规范：

### 开发流程

1. **Fork & Clone** — Fork 仓库并克隆到本地
2. **创建分支** — `git checkout -b feature/your-feature-name`
3. **开发** — 遵循项目编码规范编写代码和测试
4. **验证** — 确保 `make verify-all` 全部通过
5. **提交** — 使用 Conventional Commits 格式提交
6. **PR** — 提交 Pull Request，描述变更内容和影响

### 提交信息规范

```
<type>(<scope>): <description>

feat(snapshot): add MetaCache for List() optimization
fix(normalizer): handle nil properties in URI generation
refactor(connector): extract HTTP client to shared package
test(assembler): add golden file test for orphan edges
docs(readme): update architecture diagram
```

### 常用命令

```bash
make build              # 编译
make test               # 运行单元测试
make test-race          # 竞态检测
make test-coverage      # 测试覆盖率
make test-e2e           # 端到端测试
make lint               # 代码质量检查
make verify-all         # 全部检查（build + lint + test + race + e2e）
```

### 编码规范

项目遵循 Effective Go 核心规范，关键约束包括：

- **依赖注入** — 所有依赖通过构造函数传入，禁止全局变量和 `init()`
- **接口隔离** — 用 `interface` 做依赖注入，不用具体实现
- **错误处理** — `fmt.Errorf` + `%w` 包装错误，哨兵错误用于业务判断
- **结构化日志** — 使用 `log/slog`，禁止 `fmt.Println`
- **并发安全** — `context.Context` 传递超时，`sync.RWMutex` 区分读写锁
- **命名规范** — 包名小写、MixedCaps 驼峰、接收者用类型缩写

详细的编码规范和架构约束请参阅 [AGENTS.md](./AGENTS.md)。

### 新增实体类型

1. 在 `ontology/` 目录新建 YAML 文件（参考 `device.yaml`）
2. 如需新关系类型，在 `ontology/relations.yaml` 中追加
3. 重启服务，Schema Registry 自动加载 — **零代码改动**

### 新增 Connector

1. 实现 `connector.Connector` 接口（`Collect` + `Metadata`）
2. 在 `connector.ConnectorFactory` 注册 Builder
3. 在 `configs/connectors.yaml` 添加配置条目
4. 补充对应的单元测试和 `testdata/` Mock 数据

## 许可证

本项目为内部项目，仅供公司内部使用。
