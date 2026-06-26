# MCP 工具开发流程

## 概述

本文档描述如何为网络数字孪生 MCP Server 新增工具。MCP Server 基于官方 Go SDK（`github.com/modelcontextprotocol/go-sdk`）实现，通过 **Streamable HTTP** 传输暴露工具给外部 Agent 平台。

## 架构分层

```
┌─────────────────────────────────────────────────────┐
│  Agent 平台 (Claude / Cursor / 自研)                 │
│  通过 MCP Client 连接 MCP Server                     │
└────────────────────────┬────────────────────────────┘
                         │ HTTP (Streamable)
┌────────────────────────▼────────────────────────────┐
│  MCP 层 (internal/mcp/)                              │
│  职责: 参数解析 → 调用 Service → 构造结果              │
│  不含业务逻辑                                         │
└────────────────────────┬────────────────────────────┘
                         │ 薄接口调用
┌────────────────────────▼────────────────────────────┐
│  Service 层 (internal/service/)                      │
│  职责: 业务编排、锁管理、数据流处理                     │
└─────────────────────────────────────────────────────┘
```

**MCP 层原则**：
- 只做参数校验、Service 调用、结果封装
- 不包含任何业务逻辑
- 通过薄接口（`snapshotManager`、`syncService`）解耦依赖

---

## 新增工具 5 步骤

### Step 1: 定义 Input/Output 结构体 (`internal/mcp/tools.go`)

在 `tools.go` 底部新增输入输出类型：

```go
// MyToolInput 定义 my_tool 工具的输入参数。
type MyToolInput struct {
    ParamA string `json:"param_a" jsonschema:"参数 A 的描述"`
    ParamB int    `json:"param_b,omitempty" jsonschema:"参数 B 的描述"`
}

// MyToolOutput 定义 my_tool 工具的结构化输出。
type MyToolOutput struct {
    Result  string `json:"result"`
    Count   int    `json:"count"`
}
```

**规范**：
- Input 字段使用 `json` + `jsonschema` 双 tag（SDK 自动生成 JSON Schema）
- 可选字段加 `omitempty`
- Output 只用 `json` tag

### Step 2: 实现 handler 方法 (`internal/mcp/tools.go`)

在 `toolHandlers` 结构体上添加方法：

```go
func (h *toolHandlers) handleMyTool(
    ctx context.Context, _ *mcpsdk.CallToolRequest, in MyToolInput,
) (*mcpsdk.CallToolResult, MyToolOutput, error) {
    // 1. 参数校验
    if in.ParamA == "" {
        return nil, MyToolOutput{}, fmt.Errorf("missing required parameter: param_a")
    }

    // 2. 调用 Service 层（如有锁需求，在此加锁）
    result, err := h.someService.DoSomething(ctx, in.ParamA)
    if err != nil {
        return nil, MyToolOutput{}, fmt.Errorf("do something: %w", err)
    }

    // 3. 日志 + 返回
    slog.Info("my_tool completed", "param_a", in.ParamA, "count", result.Count)
    return nil, MyToolOutput{Result: result.Message, Count: result.Count}, nil
}
```

**锁策略**：

| 操作类型 | 锁策略 | 示例 |
|---------|--------|------|
| 只读 + 查图 | `h.lock.RLock()` | query_topology |
| 只读 + 文件系统 | 不加锁 | query_snapshot (list) |
| 写操作 | Service 内部管理 | sync_data, restore_snapshot |

**handler 签名说明**：

```go
func(ctx context.Context, req *mcpsdk.CallToolRequest, in InputType) (
    *mcpsdk.CallToolResult, OutputType, error)
```

- 第一个返回值 `*mcpsdk.CallToolResult`：通常为 `nil`（SDK 自动构造）
- 第二个返回值 `OutputType`：结构化输出，SDK 自动序列化到 `structuredContent`
- 第三个返回值 `error`：非 nil 时 SDK 自动设置 `IsError=true`

### Step 3: 注册工具 (`internal/mcp/registry.go`)

在 `newServer()` 函数中添加 `mcpsdk.AddTool` 调用：

```go
mcpsdk.AddTool(s, &mcpsdk.Tool{
    Name:        "my_tool",
    Description: "工具的人类可读描述",
}, h.handleMyTool)
```

SDK 会根据 Input 结构体的 `jsonschema` tag 自动生成 JSON Schema，无需手动编写。

### Step 4: 编写单元测试 (`internal/mcp/mcp_test.go`)

使用 `InMemoryTransports` 做端到端测试：

```go
func TestMyTool(t *testing.T) {
    // 1. 构造 mock 依赖
    h := &toolHandlers{
        graph:     &mockGraphDB{queryResult: mockRows},
        lock:      snapshot.NewGraphLock(),
        manager:   &mockSnapshotManager{},
        syncSvc:   &mockSyncService{},
    }
    cs := newTestServer(t, h)  // 复用辅助函数

    // 2. 调用工具
    ctx := context.Background()
    res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
        Name:      "my_tool",
        Arguments: map[string]any{"param_a": "value"},
    })
    if err != nil {
        t.Fatalf("CallTool error = %v", err)
    }
    if res.IsError {
        t.Fatalf("IsError=true: %v", res.Content)
    }

    // 3. 解析结构化输出
    var out MyToolOutput
    extractStructuredOutput(t, res.StructuredContent, &out)
    if out.Count != 42 {
        t.Errorf("Count = %d, want 42", out.Count)
    }
}
```

**运行单测**：

```bash
go test ./internal/mcp/ -v -count=1
go test ./internal/mcp/ -race
```

### Step 5: CLI 集成测试 (`cmd/mcp-client/`)

在 `cmd/mcp-client/main.go` 的 `buildTests()` 中添加新测试场景：

```go
{
    name: "my_tool_basic",
    desc: "验证 my_tool 基本功能",
    run: func(ctx context.Context, cs *mcpsdk.ClientSession) error {
        res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
            Name:      "my_tool",
            Arguments: map[string]any{"param_a": "test_value"},
        })
        if err != nil {
            return fmt.Errorf("CallTool: %w", err)
        }
        if res.IsError {
            return fmt.Errorf("IsError=true: %v", res.Content)
        }
        var out myToolOutput
        if err := extractStructured(res.StructuredContent, &out); err != nil {
            return err
        }
        return assertEqual("result", out.Result, "expected_value")
    },
},
```

**运行 CLI 测试**（需先启动 MCP Server）：

```bash
# Terminal 1
make mcp-server

# Terminal 2
make mcp-client
# 或指定单个测试
go run cmd/mcp-client/main.go -run my_tool_basic
```

---

## 代码规范

### 参数校验

```go
// 必填参数 — 空值时返回 error
if in.RequiredParam == "" {
    return nil, Output{}, fmt.Errorf("missing required parameter: required_param")
}

// 可选参数 — 设置默认值
limit := in.Limit
if limit <= 0 {
    limit = 100
}
```

### 错误处理

```go
// 使用 fmt.Errorf + %w 包装，保留错误链
return nil, Output{}, fmt.Errorf("operation failed: %w", err)
```

### 日志

```go
// 使用 slog（结构化日志），日志输出到 stderr
slog.Info("tool_name completed", "key1", val1, "key2", val2)
```

---

## 测试验证矩阵

| 测试层级 | 位置 | 传输方式 | 需要 Neo4j | 命令 |
|---------|------|---------|-----------|------|
| 单元测试 | `internal/mcp/mcp_test.go` | InMemoryTransports | 否 | `go test ./internal/mcp/` |
| CLI 集成 | `cmd/mcp-client/main.go` | Streamable HTTP | 是 | `make mcp-client` |
| E2E | `test/e2e/e2e_test.go` | 直连 Neo4j | 是 | `make test-e2e` |

---

## 完整示例：添加 `analyze_topology` 工具

假设需要新增一个分析拓扑的工具，统计各 Label 的节点数量。

**Step 1 — tools.go 添加类型**：

```go
type AnalyzeTopologyInput struct {
    Labels []string `json:"labels,omitempty" jsonschema:"要分析的标签列表，空则分析全部"`
}

type AnalyzeTopologyOutput struct {
    TotalNodes   int                       `json:"total_nodes"`
    TotalRels    int                       `json:"total_relations"`
    ByLabel      map[string]int            `json:"by_label"`
}
```

**Step 2 — tools.go 添加 handler**：

```go
func (h *toolHandlers) handleAnalyzeTopology(
    ctx context.Context, _ *mcpsdk.CallToolRequest, in AnalyzeTopologyInput,
) (*mcpsdk.CallToolResult, AnalyzeTopologyOutput, error) {
    h.lock.RLock()
    defer h.lock.RUnlock()

    labels := in.Labels
    if len(labels) == 0 {
        labels = []string{"Device", "Interface", "ISIS", "Link", "Network_Slice"}
    }

    byLabel := make(map[string]int)
    total := 0
    for _, label := range labels {
        cypher := fmt.Sprintf("MATCH (n:%s) WHERE n._db = $_db RETURN count(n) AS cnt", label)
        rows, err := h.graph.Query(ctx, "default", cypher, map[string]any{"_db": "default"})
        if err != nil {
            return nil, AnalyzeTopologyOutput{}, fmt.Errorf("query %s: %w", label, err)
        }
        if len(rows) > 0 {
            cnt, _ := rows[0]["cnt"].(int64)
            byLabel[label] = int(cnt)
            total += int(cnt)
        }
    }

    slog.Info("analyze_topology completed", "total", total, "labels", len(labels))
    return nil, AnalyzeTopologyOutput{TotalNodes: total, ByLabel: byLabel}, nil
}
```

**Step 3 — registry.go 注册**：

```go
mcpsdk.AddTool(s, &mcpsdk.Tool{
    Name:        "analyze_topology",
    Description: "分析网络拓扑节点分布统计",
}, h.handleAnalyzeTopology)
```

**Step 4 — mcp_test.go 添加单测**（略，参考现有 TC-M02 模式）

**Step 5 — CLI 添加测试场景**（略，参考 buildTests 中现有场景）

---

## 关键文件路径

| 文件 | 职责 |
|------|------|
| `internal/mcp/tools.go` | Input/Output 类型 + handler 实现 |
| `internal/mcp/registry.go` | Server 构建 + 工具注册 |
| `internal/mcp/server.go` | Streamable HTTP 启动入口 |
| `internal/mcp/mcp_test.go` | 单元测试（InMemoryTransports） |
| `cmd/mcp-client/main.go` | CLI 集成测试（StreamableClientTransport） |
| `cmd/server/main.go` | 服务入口 + 依赖初始化 |
