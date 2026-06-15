# Network Digital Twin Platform（网络数字孪生平台）

我认为 MVP 到 V1 阶段最合理的架构应该是：

```text
Schema Driven
Snapshot Driven
Graph Driven
MCP Native
```

## 1. 总体架构
```
                     ┌────────────────────┐
                     │   Claude Code      │
                     │   Codex            │
                     │   OpenCode         │
                     │   OpenClaw         │
                     └─────────┬──────────┘
                               │ MCP
                               ▼

                    ┌─────────────────────┐
                    │     MCP Server      │
                    └─────────┬───────────┘
                              │

        ┌─────────────────────┼────────────────────┐
        │                     │                    │

        ▼                     ▼                    ▼

 Impact Engine         RCA Engine          Simulation Engine

        │                     │                    │

        └──────────────┬──────┴────────────────────┘
                       │

                       ▼

              Digital Twin Graph

                       │

        ┌──────────────┼──────────────┐

        ▼              ▼              ▼

   Neo4j          Snapshot Store    Metadata DB

                       ▲

                       │

              Ontology Mapper

                       ▲

                       │

              Canonical Model

                       ▲

                       │

                Connectors
```

## 2. 技术栈选择
### 主语言
`Go 1.26+`

理由：

- 单人维护
- MCP友好
- 部署简单
- 强类型
- YAML驱动方便

### HTTP
`Gin`

### 配置
`Viper`

### YAML
`gopkg.in/yaml.v3`

### 数据库
`PostgreSQL 17`

存：

```
Snapshot Metadata
Connector Config
Ontology Schema Registry
Task History
Analysis Result
```
### Graph
`Neo4j CE`

存：

```
Node
Relation
Topology
Dependency
```

### Object Store
`Local File`

存：

```
Snapshot
Config
Log
Raw Data
```

## 3. 核心设计思想 - Schema Driven(类似k8s)

```go
type Entity struct {
    Kind string
    ID string
    Properties map[string]any
}
```

因为：

```
Device
Interface
BGP
ISIS
SRv6Policy
VPN
Service
Alarm
```

都只是：Ontology Entity

## 4. Ontology Registry(类似Kubernetes CRD)
### ontology/device.yaml
```yaml
apiVersion: twin.io/v1

kind: EntityType

metadata:
  name: Device

spec:

  labels:

    - Resource
    - Network

  primaryKey:

    - hostname

  properties:

    hostname:
      type: string

    vendor:
      type: string

    model:
      type: string
```
---
### ontology/interface.yaml
```yaml
apiVersion: twin.io/v1

kind: EntityType

metadata:
  name: Interface

spec:

  primaryKey:
    - device
    - name

  properties:

    name:
      type: string

    speed:
      type: integer
```
## 5. Relation Registry
### relation.yaml
```yaml
relations:

  HAS_INTERFACE:

    source:

      - Device

    target:

      - Interface

  CONNECTED_TO:

    source:

      - Interface

    target:

      - Interface

  RUNS:

    source:

      - Device

    target:

      - Protocol
```

- 未来增加新的实例和关系无需代码

```
SRv6Policy
EVPN
MPLS
```

## 6. Canonical Model

## 7. Snapshot Layer
### snapshot.yaml
```yaml
snapshotId: 20260611001

timestamp: 2026-06-11T00:00:00Z

entities:

  - kind: Device
    id: PE1

relations:

  - type: HAS_INTERFACE
    source: PE1
    target: PE1-GE0/0/0
```
- 后续rca，分析，模拟都依赖Snapshot Diff

## 8. Graph Builder
```
Snapshot

↓

Graph Builder

↓

Neo4j
```
## 9. Schema Registry
- 新增本体,系统自动加载,零代码关键
```yaml
entityTypes:

  SRv6Policy

  Locator

  SID

  VPN

  Service
```

## 10. Connector框架
### Connector接口
```go
type Connector interface {

    Metadata() Metadata

    Collect(
       ctx context.Context,
    ) ([]Resource,error)
}
```
- 插件注册：
```yaml
connectors:

  - type: huawei

  - type: cmdb

  - type: ipam
```

## 11. RCA引擎 - 不要LLM。
- 输入： `Alarm`
- 输出： `Root Cause Candidates`
- 算法： 
```
Graph Traversal

Dependency Analysis

Topology Impact

Change Correlation
```

## 12. Simulation引擎
- 输入：
```
Delete Link

Shutdown Interface

Withdraw SID

Change Policy

```

- 执行：
```
Clone Snapshot

Apply Change

Recalculate Graph
```

- 输出：
```
Affected Service

Affected VPN

Affected Customer
```

## 13. MCP设计
```
get_topology

get_service_path

get_impacted_services

root_cause

simulate_failure

simulate_change
```
- 本质:
```
MCP
→ Service
→ Neo4j
```

## 最终技术栈（我认为最适合你）
| 模块            | 技术                         |
| ------------- | -------------------------- |
| Language      | Go 1.26+                   |
| HTTP          | Gin                       |
| Config        | Viper                      |
| YAML          | yaml.v3                    |
| Metadata DB   | PostgreSQL17                 |
| Graph DB      | Neo4j CE                   |
| Snapshot      | Local File → MinIO         |
| Cache         | 暂时不要                       |
| Queue         | 暂时不要                       |
| Search        | 暂时不要                       |
| Agent         | 不实现                        |
| MCP           | MCP Go SDK                 |
| Deploy        | Docker Compose             |
| Observability | OpenTelemetry + Prometheus |
| Logs          | Zap                        |
| Scheduler     | gocron                     |


