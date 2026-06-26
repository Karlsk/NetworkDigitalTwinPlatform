# 第三阶段：智能系统集成

> MCP Server → 业务编排 → 端到端集成测试 → MVP 验收
>
> **进度**: Phase 5 已完成（V-01 ~ V-04）

## 任务索引

### 验证阶段 (Phase 5)

| 任务ID | 任务名称 | 工时 | 文件 |
|--------|---------|------|------|
| V-01 | MCP Server 实现 + 工具注册 | 1.5天 | [V-01_to_V-04_MCP与端到端验收.md](V-01_to_V-04_MCP与端到端验收.md) |
| V-02 | MCP 工具集成测试 | 1天 | [V-01_to_V-04_MCP与端到端验收.md](V-01_to_V-04_MCP与端到端验收.md) |
| V-03 | 端到端集成测试 | 1.5天 | [V-01_to_V-04_MCP与端到端验收.md](V-01_to_V-04_MCP与端到端验收.md) |
| V-04 | 全量验收 + 文档归档 | 1天 | [V-01_to_V-04_MCP与端到端验收.md](V-01_to_V-04_MCP与端到端验收.md) |

---

## 功能描述

第三阶段将前两个阶段的成果组装为完整的智能运维闭环。核心目标是：

1. **实现业务编排层**：SyncService / SnapshotService / AnalysisService 封装底层组件调用
2. **实现 MCP Server**：通过 stdio JSON-RPC 2.0 协议暴露工具给外部 Agent 平台
3. **实现 MCP 工具集**：只读工具（query_topology / query_snapshot）+ 写操作工具（sync_data / restore_snapshot）
4. **端到端集成测试**：Mock 数据导入 → 拓扑查询 → 快照创建/恢复 → 增量同步 → 验证完整流程

### 两层架构

```
┌─────────────────────────────────────────────────────────────────┐
│  第二层：外部 Agent 平台（智能层）                                │
│  Claude Code / OpenCode                                          │
│  • 通过 MCP 协议调用工具                                         │
│  • 包含业务逻辑和领域知识                                         │
│  • Skill 编排多个 MCP Tool 完成复杂任务                            │
├─────────────────────────────────────────────────────────────────┤
│  第一层：Go MCP Server（工具层）                                  │
│  internal/mcp/ + internal/service/                               │
│  • 通用、稳定、少变                                               │
│  • 只提供原子操作，不包含业务逻辑                                   │
│  • 每个工具输入/输出有明确契约                                     │
└─────────────────────────────────────────────────────────────────┘
```

调用链路：
```
Agent Skill → MCP Tool → Service 编排 → GraphDB / SnapshotManager → Neo4j
```

### MVP 工具集（只读 + 写操作）

```
MCP (能力 API 网关)
├── 只读工具 (Read)         → Agent 随时可调，无副作用
│   ├── query_topology      → 拓扑查询
│   └── query_snapshot      → 快照查询/列表/Diff
│
└── 写操作工具 (Write)      → Agent 调用需谨慎，有副作用
    ├── sync_data           → 触发数据同步
    └── restore_snapshot    → 恢复快照 (修改 default DB)
```

| 工具名 | 类型 | 功能 | 调用链 |
|--------|------|------|--------|
| `query_topology` | 只读 | 按条件查询拓扑 | → Service → GraphDB |
| `query_snapshot` | 只读 | 快照列表/Diff | → Service → SnapshotManager |
| `sync_data` | 写 | 触发数据同步 | → Service → SyncService |
| `restore_snapshot` | 写 | 恢复快照 | → Service → SnapshotManager |

**关键设计**：
- MCP 层只做工具注册和参数路由，不包含业务逻辑
- 所有工具通过 Service 层调用，Service 负责编排 GraphDB/SnapshotManager
- 只读工具无副作用，Agent 可自由调用；写操作工具需在 Skill 中谨慎使用

### V1 扩展工具（MVP 不实现）

以下工具在 V1 阶段实现，依赖分析引擎（Impact/RCA/Simulation Engine）：

| 工具名 | 类型 | 功能 | V1 依赖 |
|--------|------|------|---------|
| `query_impact` | 只读 | 爆炸半径分析 | Impact Engine |
| `query_root_cause` | 只读 | 根因分析 | RCA Engine |
| `query_simulation` | 只读 | 仿真推演 | Simulation Engine (内存沙盒) |

## 文件清单

### 业务编排层

| 文件路径 | 用途 |
|----------|------|
| `internal/service/sync_service.go` | **同步编排**：FullSync / IncrementalSync / StartConsumer / HandleWebhook |
| `internal/service/snapshot_service.go` | **快照编排**：Create / Restore / List / Delete / Diff |
| `internal/service/analysis_service.go` | **分析编排**：Topology 查询（MVP），V1 扩展 Impact/RCA/Simulation |

### MCP Server 实现

| 文件路径 | 用途 |
|----------|------|
| `internal/mcp/server.go` | **MCP Server 主体**：stdio 传输、JSON-RPC 2.0 消息处理、工具路由 |
| `internal/mcp/registry.go` | **工具注册中心**：MCPToolRegistry 实现，管理工具的注册和发现 |
| `internal/mcp/tools.go` | **工具实现**：query_topology / query_snapshot / sync_data / restore_snapshot |

#### `internal/mcp/server.go` 核心逻辑

```go
type MCPServer struct {
    registry MCPToolRegistry
    config   MCPConfig
}

func NewMCPServer(registry MCPToolRegistry, cfg MCPConfig) *MCPServer
func (s *MCPServer) Start(ctx context.Context) error  // 启动 stdio 监听
func (s *MCPServer) handleRequest(ctx context.Context, req JSONRPCRequest) (JSONRPCResponse, error)
func (s *MCPServer) handleToolCall(ctx context.Context, params map[string]interface{}) (*ToolResult, error)
func (s *MCPServer) handleListTools() []ToolDescriptor
```

#### `internal/mcp/tools.go` — 工具实现

```go
// query_topology: 按条件查询拓扑
// 输入: { filter?: string, label?: string, depth?: int }
// 输出: ToolResult{
//   Data: {nodes: [], relationships: []},
//   Summary: "查询返回 3 个 Device, 18 个 Interface"
// }

// query_snapshot: 快照查询/列表/Diff
// 输入: { action: "list|diff", snap_a?: string, snap_b?: string }
// 输出: ToolResult{
//   Data: []SnapshotMeta | SnapshotDiff,
//   Summary: "共 3 个快照" | "snap_001 vs snap_002: 新增 2 节点, 删除 1 关系"
// }

// sync_data: 触发数据同步
// 输入: { sync_type: "full" }
// 输出: ToolResult{
//   Data: SyncResult,
//   Summary: "全量同步完成: 25 节点, 30 关系, 耗时 1.2s"
// }

// restore_snapshot: 恢复快照
// 输入: { snapshot_name: string }
// 输出: ToolResult{
//   Data: {success: true, snapshot_name: "snap_001"},
//   Summary: "快照 snap_001 已恢复为 default DB"
// }
```

### 测试数据

| 文件路径 | 用途 |
|----------|------|
| `testdata/mock_netbox/` | Mock Netbox 数据（devices.json, interfaces.json） |
| `testdata/mock_cmdb/` | Mock CMDB 数据（isis.json, links.json） |
| `testdata/golden/expected_topology.yaml` | Golden File：期望的拓扑输出 |
| `testdata/golden/expected_analysis.json` | Golden File：期望的节点/关系计数 |

## 测试内容

### MCP Server 单元测试

| 测试文件 | 测试范围 | 测试方法 |
|----------|----------|----------|
| `internal/mcp/mcp_test.go` | 工具注册与发现 | 注册 4 个工具 → ListTools() 返回 4 个 ToolDescriptor |
| `internal/mcp/mcp_test.go` | 工具路由 | ExecuteTool("query_topology", params) → 正确路由 |
| `internal/mcp/mcp_test.go` | 参数校验 | 缺少必填参数 → 返回错误 |
| `internal/mcp/mcp_test.go` | ToolResult 格式 | 所有工具返回 ToolResult{Success, Data, Summary} |

#### MCP Server 测试用例

```go
// TC-MCP-01: 工具列表
Step:  ListTools()
Check: 返回 4 个 ToolDescriptor (query_topology, query_snapshot, sync_data, restore_snapshot)

// TC-MCP-02: query_topology 调用
Step:  ExecuteTool("query_topology", {filter: "Device"})
Check: 返回 ToolResult{Success: true, Data: {nodes: [...], relationships: [...]}}

// TC-MCP-03: sync_data 调用
Step:  ExecuteTool("sync_data", {sync_type: "full"})
Check: 返回 SyncResult，节点数 ≥ 20

// TC-MCP-04: 参数错误处理
Step:  ExecuteTool("restore_snapshot", {})  // 缺少 snapshot_name
Check: 返回 ToolResult{Success: false, Error: "missing required parameter"}
```

### 集成测试

| 测试文件 | 测试范围 | 测试方法 |
|----------|----------|----------|
| `internal/mcp/integration_test.go` | MCP 端到端 | stdio 发送 JSON-RPC → 验证工具返回 |
| `tests/e2e_test.go` | 完整流程 | 启动服务 → 全量同步 → 查询 → 快照 → 恢复 |

#### 端到端集成测试用例

```go
// TC-E2E-01: 完整数据流
Step:
  1. 启动服务 (Docker Compose)
  2. 调用 sync_data (full) → 全量同步 Mock 数据
  3. 调用 query_topology → 验证节点/关系数
  4. 调用 query_snapshot (list) → 验证快照列表
  5. 调用 sync_data (full) → 修改数据
  6. 调用 query_snapshot (create) → 创建快照
  7. 调用 restore_snapshot → 恢复快照
  8. 调用 query_topology → 验证数据恢复
Check: 所有步骤返回预期结果，数据一致性验证通过

// TC-E2E-02: 增量同步流程
Step:
  1. 全量同步
  2. 发送 Webhook (update) → 更新设备状态
  3. 查询验证状态变更
  4. 发送 Webhook (delete) → 删除设备
  5. 查询验证设备已删除
Check: 增量同步正确应用，图数据一致

// TC-E2E-03: 快照对比流程
Step:
  1. 全量同步 → 创建快照 snap_001
  2. 增量同步 (新增设备)
  3. 创建快照 snap_002
  4. 调用 query_snapshot (diff, snap_001, snap_002)
Check: 返回新增节点列表，差异正确
```

### Agent 集成验证（手工）

| 测试场景 | 验证方法 | 预期结果 |
|----------|----------|----------|
| MCP Server 启动 | `go run cmd/server/main.go --mcp-mode` | 无错误，等待 stdin 输入 |
| Agent 调用 sync_data | 发送 JSON-RPC {name: "sync_data"} | 触发全量同步，返回 SyncResult |
| Agent 调用 query_topology | 发送 JSON-RPC {name: "query_topology"} | 返回拓扑数据 |
| Agent 调用 restore_snapshot | 发送 JSON-RPC {name: "restore_snapshot", params: {snapshot_name: "..."}} | 恢复快照 |

## 验收标准

### MCP Server 验收

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| A-01 | MCP Server 启动 | `go run cmd/server/main.go --mcp-mode` | 无错误，等待 stdin 输入 |
| A-02 | 工具发现 | 发送 `tools/list` JSON-RPC 请求 | 返回 4 个 ToolDescriptor |
| A-03 | query_topology 可用 | 发送 `tools/call` {name:"query_topology"} | 返回拓扑数据 + Summary |
| A-04 | query_snapshot 可用 | 发送 `tools/call` {name:"query_snapshot", params:{action:"list"}} | 返回快照列表 |
| A-05 | sync_data 可用 | 发送 `tools/call` {name:"sync_data", params:{sync_type:"full"}} | 触发全量同步 |
| A-06 | restore_snapshot 可用 | 发送 `tools/call` {name:"restore_snapshot", params:{snapshot_name:"..."}} | 恢复快照 |
| A-07 | 错误处理 | 发送无效参数 | 返回 ToolResult{Success:false, Error:"..."} |

### 集成测试验收

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| B-01 | 完整数据流 | 端到端测试 TC-E2E-01 | 所有步骤通过 |
| B-02 | 增量同步流程 | 端到端测试 TC-E2E-02 | 增量更新正确应用 |
| B-03 | 快照对比流程 | 端到端测试 TC-E2E-03 | 差异正确识别 |
| B-04 | Mock 数据导入验证 | `go test -tags=integration` | 节点数 ≥ 20，关系数 ≥ 30 |
| B-05 | MCP 查询验证 | query_topology 返回正确拓扑 | 节点/关系与 Neo4j 一致 |
| B-06 | 快照恢复验证 | 创建 → 修改 → 恢复 → 验证 | 数据恢复到快照状态 |

### 质量验收

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| Q-01 | 单元测试通过 | `go test ./...` | 0 failures |
| Q-02 | 集成测试通过 | `go test -tags=integration ./...` | 0 failures |
| Q-03 | 代码覆盖率 | `go test -cover ./internal/...` | ≥ 70% |
| Q-04 | Lint 通过 | `golangci-lint run` | 无 Error |

### MVP 验收门禁

**以下条件全部满足后，MVP 验收通过：**

- [x] A-01~A-07：MCP Server 4 个工具全部可用
- [x] B-01~B-06：集成测试全部通过
- [x] Q-01~Q-04：质量验收全部通过
- [x] 第二阶段功能在集成后未退化
- [x] Docker Compose 一键启动正常
- [x] 全部单元测试 + 集成测试通过
- [x] Lint 无 Error

## MVP 不包含 (放 V1)

- RCA Engine / Impact Engine / Simulation Engine
- PostgreSQL 元数据存储
- 真实数据源 Connector (Netbox/Controller/CMDB)
- HTTP API (Gin)
- 可观测性 (OpenTelemetry/Prometheus)
- 定时调度 (gocron)
- 本体继承机制 (extends)
- Kafka 事件流 (替代 Channel 缓冲)
- Agent Skills 配置 (OpenCode Agent)
- Agent 知识库 (ontology_guide / cypher_patterns)

> 详见 [V1扩展方向.md](../../docs/V1扩展方向.md)

## 已知问题与 V1 技术债

| 编号 | 问题 | 影响 | V1 解决方案 |
|------|------|------|------------|
| TD-01 | Engine 三大引擎仅有接口骨架 | Impact/RCA/Simulation 工具不可用 | V1 实现图遍历算法 |
| TD-02 | AnalysisService 仅支持 QueryTopology | 高级查询需直接写 Cypher | V1 扩展查询 API |
| TD-03 | SnapshotService 是薄封装 | 与 Manager 功能重叠 | V1 添加缓存/审计 |
| TD-04 | 快照 Diff 只对比 URI 存在性 | 不对比属性差异 | V1 引入属性级 diff |
| TD-05 | golangci-lint 未安装（本地环境） | Lint 验收跳过 | `brew install golangci-lint` |
