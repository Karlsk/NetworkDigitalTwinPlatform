# V-01: MCP Server 实现 + 工具注册

## 1. 任务概述

实现 MCP Server（stdio JSON-RPC 协议），注册 4 个 MVP 工具：query_topology（只读）、query_snapshot（只读）、sync_data（写）、restore_snapshot（写）。MCP 是能力 API 网关，不包含业务逻辑，统一通过 Service 层编排。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 5: 验证阶段 |
| 预估工时 | 1.5 天 |
| 前置任务 | I-14, I-15, I-16, I-17, I-18 |
| 交付物 | `internal/mcp/server.go`、`internal/mcp/registry.go`、`internal/mcp/tools.go` |

## 2. 详细实现步骤

### Day 1: MCP Server 框架

**文件**: `internal/mcp/server.go`

```go
package mcp

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "os"
)

type MCPServer struct {
    registry *ToolRegistry
}

func NewMCPServer(registry *ToolRegistry) *MCPServer {
    return &MCPServer{registry: registry}
}

// Start 启动 MCP Server (stdio JSON-RPC)
func (s *MCPServer) Start(ctx context.Context) error {
    decoder := json.NewDecoder(os.Stdin)
    encoder := json.NewEncoder(os.Stdout)

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            var request JSONRPCRequest
            if err := decoder.Decode(&request); err != nil {
                slog.Error("decode request", "error", err)
                continue
            }

            response := s.handleRequest(ctx, request)
            if err := encoder.Encode(response); err != nil {
                slog.Error("encode response", "error", err)
            }
        }
    }
}

func (s *MCPServer) handleRequest(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
    switch req.Method {
    case "tools/list":
        tools := s.registry.ListTools()
        return JSONRPCResponse{ID: req.ID, Result: tools}

    case "tools/call":
        var params ToolCallParams
        json.Unmarshal(req.Params, &params)
        result, err := s.registry.Execute(ctx, params.Name, params.Arguments)
        if err != nil {
            return JSONRPCResponse{ID: req.ID, Error: err.Error()}
        }
        return JSONRPCResponse{ID: req.ID, Result: result}

    default:
        return JSONRPCResponse{ID: req.ID, Error: "method not found"}
    }
}

type JSONRPCRequest struct {
    ID     int             `json:"id"`
    Method string          `json:"method"`
    Params json.RawMessage `json:"params"`
}

type JSONRPCResponse struct {
    ID     int    `json:"id"`
    Result any    `json:"result,omitempty"`
    Error  string `json:"error,omitempty"`
}

type ToolCallParams struct {
    Name      string         `json:"name"`
    Arguments map[string]any `json:"arguments"`
}
```

**文件**: `internal/mcp/registry.go`

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(ctx context.Context, params map[string]any) (*ToolResult, error)
}

type ToolResult struct {
    Success bool   `json:"success"`
    Data    any    `json:"data,omitempty"`
    Summary string `json:"summary"`
    Error   string `json:"error,omitempty"`
}

type ToolRegistry struct {
    tools map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
    return &ToolRegistry{tools: make(map[string]Tool)}
}

func (r *ToolRegistry) Register(tool Tool) {
    r.tools[tool.Name()] = tool
}

func (r *ToolRegistry) ListTools() []ToolDescriptor {
    var tools []ToolDescriptor
    for _, t := range r.tools {
        tools = append(tools, ToolDescriptor{
            Name:        t.Name(),
            Description: t.Description(),
            InputSchema: t.InputSchema(),
        })
    }
    return tools
}

func (r *ToolRegistry) Execute(ctx context.Context, name string, params map[string]any) (*ToolResult, error) {
    tool, ok := r.tools[name]
    if !ok {
        return nil, fmt.Errorf("tool %q not found", name)
    }
    return tool.Execute(ctx, params)
}

type ToolDescriptor struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    InputSchema map[string]any `json:"inputSchema"`
}
```

### Day 2 (半天): 4 个 MCP 工具

**文件**: `internal/mcp/tools.go`

```go
// === query_topology (只读) ===
type QueryTopologyTool struct {
    graph graph.GraphDB
    lock  *snapshot.GraphLock
}

func (t *QueryTopologyTool) Name() string        { return "query_topology" }
func (t *QueryTopologyTool) Description() string  { return "查询网络拓扑数据" }
func (t *QueryTopologyTool) InputSchema() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "label": map[string]any{"type": "string", "description": "节点标签 (Device/Interface/...)"},
            "limit": map[string]any{"type": "integer", "default": 100},
        },
    }
}

func (t *QueryTopologyTool) Execute(ctx context.Context, params map[string]any) (*ToolResult, error) {
    t.lock.RLock()
    defer t.lock.RUnlock()

    label, _ := params["label"].(string)
    limit := 100
    if l, ok := params["limit"].(float64); ok { limit = int(l) }

    cypher := fmt.Sprintf("MATCH (n:%s {_db: $_db}) RETURN n LIMIT %d", label, limit)
    results, err := t.graph.Query(ctx, "default", cypher, nil)
    if err != nil { return &ToolResult{Success: false, Error: err.Error()}, nil }

    return &ToolResult{
        Success: true,
        Data:    results,
        Summary: fmt.Sprintf("返回 %d 个 %s 节点", len(results), label),
    }, nil
}

// === query_snapshot (只读) ===
// Execute → SnapshotManager.List / Diff

// === sync_data (写) ===
// Execute → SyncService.FullSync

// === restore_snapshot (写) ===
// Execute → SnapshotManager.Restore
```

## 3. 设计原理

### MCP 工具分为只读和写操作

| 类型 | 工具 | 锁 | 说明 |
|------|------|-----|------|
| 只读 | query_topology, query_snapshot | RLock | Agent 随时可调，无副作用 |
| 写 | sync_data, restore_snapshot | Lock（内部 Service 处理） | 有副作用，需谨慎 |

### MCP 不包含业务逻辑

- 每个工具只做：参数解析 → 调用 Service → 构造 ToolResult
- 所有业务逻辑在 Service 层
- MCP 层可以独立测试（Mock Service）

## 4. 验收标准

- [x] 4 个工具注册成功
- [x] `tools/list` 返回 4 个 ToolDescriptor
- [x] query_topology 返回拓扑数据
- [x] sync_data 触发全量同步
- [x] restore_snapshot 恢复快照

## 5. 注意事项

- stdio 通信：日志输出到 stderr（`slog.SetDefault(slog.NewTextHandler(os.Stderr, nil))`），不干扰 stdout
- JSON-RPC 消息格式：`{"id": 1, "method": "tools/call", "params": {...}}`
- ToolResult.Summary 是人类可读的摘要字符串，Agent 可直接展示给用户

---

# V-02: MCP 工具集成测试

## 1. 任务概述

测试 MCP Server 的工具注册、列表、调用和错误处理。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 5: 验证阶段 |
| 预估工时 | 1 天 |
| 前置任务 | V-01 |
| 交付物 | `internal/mcp/mcp_test.go` |

## 2. 测试用例

| 用例ID | 测试内容 | 期望结果 |
|--------|---------|---------|
| TC-M01 | ListTools | 返回 4 个工具 |
| TC-M02 | query_topology | 返回拓扑数据 |
| TC-M03 | query_snapshot (list) | 返回快照列表 |
| TC-M04 | sync_data (full) | 触发全量同步 |
| TC-M05 | restore_snapshot | 恢复快照 |
| TC-M06 | 参数错误 | ToolResult{Success: false} |
| TC-M07 | 不存在的工具 | error |

## 3. 验收标准

- [x] 所有测试用例通过

---

# V-03: 端到端集成测试

## 1. 任务概述

完整的端到端闭环测试：Docker Compose 启动 → FullSync → 查询验证 → 快照 → 增量更新 → 快照对比 → 恢复。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 5: 验证阶段 |
| 预估工时 | 1.5 天 |
| 前置任务 | T-06, T-07, T-08, V-02 |
| 交付物 | `test/e2e/e2e_test.go` |

## 2. 测试流程

### TC-E2E-01: 完整数据流

```
1. Docker Compose 启动 (Neo4j CE)
2. FullSync → 验证节点 ≥ 20, 关系 ≥ 30
3. query_topology(label="Device") → 返回 3 台设备
4. Create("snap_001") → YAML 文件生成
5. IncrementalSync update (设备 Down) → 验证状态变更
6. Create("snap_002") → 新快照
7. Diff("snap_001", "snap_002") → 验证差异（status 变更）
8. Restore("snap_001") → 验证数据恢复
9. query_topology → 验证恢复到 snap_001 状态
```

### TC-E2E-02: 增量同步流程

```
1. FullSync
2. Webhook update → 验证 MERGE
3. Webhook delete → 验证 DELETE
4. Webhook delete_relation → 验证关系删除
```

### TC-E2E-03: 并发保护

```
1. Restore 期间发送 5 个 Webhook
2. Restore 完成后验证 5 个事件全部处理
```

## 3. 验收标准

- [x] 所有 E2E 测试通过
- [x] 完整数据流闭环验证

> **实现说明**: 新增 `TestE2E_FullDataFlow` (TC-E2E-01) 和 `TestE2E_ConcurrentProtection` (TC-E2E-03)。
> TC-E2E-02 已由现有 `TestE2E_IncrementalSync` 覆盖。
> 快照对比使用 `LocalDiff` (YAML 内存对比) 而非 Cypher Diff，避免 Neo4j NOT EXISTS 子查询的边界行为。

---

# V-04: 全量验收 + 文档归档

## 1. 任务概述

最终验收：编译、lint、单元测试、集成测试、race 检测、代码覆盖率、Docker Compose 启动全通过。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 5: 验证阶段 |
| 预估工时 | 1 天 |
| 前置任务 | V-03 |
| 交付物 | 验收报告 |

## 2. 验收清单

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| 1 | 编译通过 | `go build ./...` | 无错误 |
| 2 | Lint 通过 | `golangci-lint run` | 无 Error |
| 3 | 单元测试 | `go test ./...` | 0 failures |
| 4 | 集成测试 | `go test -tags=integration ./...` | 0 failures |
| 5 | Race 检测 | `go test -race ./...` | 无 data race |
| 6 | 代码覆盖率 | `go test -cover ./...` | ≥ 70% |
| 7 | Docker Compose | `docker-compose up` | Neo4j + App healthy |
| 8 | 全量同步 | FullSync | ≥ 20 节点, ≥ 20 关系 |
| 9 | 增量同步 | Webhook | 三种 action 正确 |
| 10 | 快照创建 | Create | YAML 文件生成 |
| 11 | 快照恢复 | Restore | 数据恢复 |
| 12 | MCP 工具 | 4 个工具 | 全部可用 |
| 13 | 孤儿边检测 | 注入脏数据 | Warn 不阻断 |
| 14 | URI 不可变 | 修改 hostname | URI 不变 |

## 3. 文档归档

- 更新 `docs/架构设计.md` 中的"验证方式"章节
- 更新 `changelog/` 各阶段 README 的验收状态
- 记录已知问题和 V1 技术债
