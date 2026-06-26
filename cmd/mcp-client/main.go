// Command mcp-client 是网络数字孪生 MCP Server 的 CLI 测试客户端。
//
// 使用方式:
//
//	go run cmd/mcp-client/main.go                          # 连接 localhost:8080
//	go run cmd/mcp-client/main.go -addr 192.168.1.100:8080 # 指定地址
//	go run cmd/mcp-client/main.go -run sync_data_full      # 只运行单个测试
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// ANSI 颜色码
// ---------------------------------------------------------------------------

const (
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorReset  = "\033[0m"
)

// ---------------------------------------------------------------------------
// 测试用例定义
// ---------------------------------------------------------------------------

type testCase struct {
	name string
	desc string
	run  func(ctx context.Context, cs *mcpsdk.ClientSession) error
}

// ---------------------------------------------------------------------------
// CLI 参数
// ---------------------------------------------------------------------------

var (
	addr    = flag.String("addr", "localhost:8080", "MCP Server 地址")
	runOnly = flag.String("run", "all", "运行指定测试或 all")
	timeout = flag.Duration("timeout", 60*time.Second, "单个测试超时")
)

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// extractStructured 通过 JSON round-trip 将 StructuredContent 解析到目标结构体。
func extractStructured(raw any, dst any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return json.Unmarshal(data, dst)
}

// assertEqual 简易断言。
func assertEqual(name string, got, want any) error {
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		return fmt.Errorf("%s: got %v, want %v", name, got, want)
	}
	return nil
}

// assertGTE 断言 got >= want。
func assertGTE(name string, got, want int) error {
	if got < want {
		return fmt.Errorf("%s: got %d, want >= %d", name, got, want)
	}
	return nil
}

// printResult 打印单行测试结果。
func printResult(idx, total int, name string, status string, detail string) {
	var color string
	switch status {
	case "PASS":
		color = colorGreen
	case "FAIL":
		color = colorRed
	case "SKIP":
		color = colorYellow
	}
	fmt.Printf(" %s%2d/%d  %-30s [%s]%s  %s\n",
		color, idx, total, name, status, colorReset, detail)
}

// ---------------------------------------------------------------------------
// I/O 类型（与 internal/mcp/tools.go 对齐）
// ---------------------------------------------------------------------------

type queryTopologyOutput struct {
	Nodes []map[string]any `json:"nodes"`
	Count int              `json:"count"`
}

type querySnapshotOutput struct {
	Snapshots []struct {
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
		NodeCount int    `json:"node_count"`
		RelCount  int    `json:"rel_count"`
	} `json:"snapshots,omitempty"`
	Diff *struct {
		AddedNodes   int `json:"added_nodes"`
		RemovedNodes int `json:"removed_nodes"`
		AddedRels    int `json:"added_rels"`
		RemovedRels  int `json:"removed_rels"`
	} `json:"diff,omitempty"`
}

type syncDataOutput struct {
	NodesCreated     int    `json:"nodes_created"`
	RelationsCreated int    `json:"relations_created"`
	OrphanEdges      int    `json:"orphan_edges_skipped"`
	Duration         string `json:"duration"`
}

type restoreSnapshotOutput struct {
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// 10 个测试场景
// ---------------------------------------------------------------------------

func buildTests() []testCase {
	return []testCase{
		{
			name: "list_tools",
			desc: "列出所有工具",
			run: func(ctx context.Context, cs *mcpsdk.ClientSession) error {
				res, err := cs.ListTools(ctx, nil)
				if err != nil {
					return fmt.Errorf("ListTools: %w", err)
				}
				if len(res.Tools) != 4 {
					return fmt.Errorf("got %d tools, want 4", len(res.Tools))
				}
				wantNames := map[string]bool{
					"query_topology": false, "query_snapshot": false,
					"sync_data": false, "restore_snapshot": false,
				}
				for _, t := range res.Tools {
					wantNames[t.Name] = true
				}
				for name, found := range wantNames {
					if !found {
						return fmt.Errorf("tool %q not found", name)
					}
				}
				return nil
			},
		},
		{
			name: "query_topology_empty",
			desc: "同步前查询 Device（可能为空）",
			run: func(ctx context.Context, cs *mcpsdk.ClientSession) error {
				res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
					Name:      "query_topology",
					Arguments: map[string]any{"label": "Device", "limit": 100},
				})
				if err != nil {
					return fmt.Errorf("CallTool: %w", err)
				}
				if res.IsError {
					return fmt.Errorf("IsError=true: %v", res.Content)
				}
				var out queryTopologyOutput
				if err := extractStructured(res.StructuredContent, &out); err != nil {
					return err
				}
				// 同步前 default DB 可能为空（首次启动），也可能已有数据
				// 不强制断言 count==0，只打印实际值
				return nil
			},
		},
		{
			name: "sync_data_full",
			desc: "全量同步（Mock 数据注入）",
			run: func(ctx context.Context, cs *mcpsdk.ClientSession) error {
				res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
					Name:      "sync_data",
					Arguments: map[string]any{"action": "full"},
				})
				if err != nil {
					return fmt.Errorf("CallTool: %w", err)
				}
				if res.IsError {
					return fmt.Errorf("IsError=true: %v", res.Content)
				}
				var out syncDataOutput
				if err := extractStructured(res.StructuredContent, &out); err != nil {
					return err
				}
				if err := assertGTE("nodes_created", out.NodesCreated, 20); err != nil {
					return err
				}
				if err := assertGTE("relations_created", out.RelationsCreated, 20); err != nil {
					return err
				}
				return nil
			},
		},
		{
			name: "query_topology_device",
			desc: "同步后查询 Device 节点",
			run: func(ctx context.Context, cs *mcpsdk.ClientSession) error {
				res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
					Name:      "query_topology",
					Arguments: map[string]any{"label": "Device", "limit": 100},
				})
				if err != nil {
					return fmt.Errorf("CallTool: %w", err)
				}
				if res.IsError {
					return fmt.Errorf("IsError=true: %v", res.Content)
				}
				var out queryTopologyOutput
				if err := extractStructured(res.StructuredContent, &out); err != nil {
					return err
				}
				return assertEqual("count", out.Count, 3)
			},
		},
		{
			name: "query_topology_iface",
			desc: "查询 Interface 节点",
			run: func(ctx context.Context, cs *mcpsdk.ClientSession) error {
				res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
					Name:      "query_topology",
					Arguments: map[string]any{"label": "Interface", "limit": 100},
				})
				if err != nil {
					return fmt.Errorf("CallTool: %w", err)
				}
				if res.IsError {
					return fmt.Errorf("IsError=true: %v", res.Content)
				}
				var out queryTopologyOutput
				if err := extractStructured(res.StructuredContent, &out); err != nil {
					return err
				}
				return assertEqual("count", out.Count, 12)
			},
		},
		{
			name: "query_snapshot_list",
			desc: "列出快照",
			run: func(ctx context.Context, cs *mcpsdk.ClientSession) error {
				res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
					Name:      "query_snapshot",
					Arguments: map[string]any{"action": "list"},
				})
				if err != nil {
					return fmt.Errorf("CallTool: %w", err)
				}
				if res.IsError {
					return fmt.Errorf("IsError=true: %v", res.Content)
				}
				var out querySnapshotOutput
				if err := extractStructured(res.StructuredContent, &out); err != nil {
					return err
				}
				// 快照数量不确定，打印即可
				return nil
			},
		},
		{
			name: "query_snapshot_diff",
			desc: "快照差异对比（需 ≥2 快照）",
			run: func(ctx context.Context, cs *mcpsdk.ClientSession) error {
				// 先列出快照
				listRes, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
					Name:      "query_snapshot",
					Arguments: map[string]any{"action": "list"},
				})
				if err != nil {
					return fmt.Errorf("list: %w", err)
				}
				var listOut querySnapshotOutput
				if err := extractStructured(listRes.StructuredContent, &listOut); err != nil {
					return err
				}
				if len(listOut.Snapshots) < 2 {
					return errSkip("需要 ≥2 个快照才能 diff，当前 " + fmt.Sprintf("%d", len(listOut.Snapshots)))
				}
				// 用前两个快照做 diff
				res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
					Name: "query_snapshot",
					Arguments: map[string]any{
						"action": "diff",
						"snap_a": listOut.Snapshots[0].Name,
						"snap_b": listOut.Snapshots[1].Name,
					},
				})
				if err != nil {
					return fmt.Errorf("diff: %w", err)
				}
				if res.IsError {
					return fmt.Errorf("IsError=true: %v", res.Content)
				}
				var out querySnapshotOutput
				if err := extractStructured(res.StructuredContent, &out); err != nil {
					return err
				}
				if out.Diff == nil {
					return fmt.Errorf("diff result is nil")
				}
				return nil
			},
		},
		{
			name: "restore_snapshot",
			desc: "恢复快照（需 ≥1 快照）",
			run: func(ctx context.Context, cs *mcpsdk.ClientSession) error {
				// 先列出快照
				listRes, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
					Name:      "query_snapshot",
					Arguments: map[string]any{"action": "list"},
				})
				if err != nil {
					return fmt.Errorf("list: %w", err)
				}
				var listOut querySnapshotOutput
				if err := extractStructured(listRes.StructuredContent, &listOut); err != nil {
					return err
				}
				if len(listOut.Snapshots) < 1 {
					return errSkip("需要 ≥1 个快照才能 restore")
				}
				res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
					Name:      "restore_snapshot",
					Arguments: map[string]any{"snapshot_name": listOut.Snapshots[0].Name},
				})
				if err != nil {
					return fmt.Errorf("restore: %w", err)
				}
				if res.IsError {
					return fmt.Errorf("IsError=true: %v", res.Content)
				}
				var out restoreSnapshotOutput
				if err := extractStructured(res.StructuredContent, &out); err != nil {
					return err
				}
				if out.Message == "" {
					return fmt.Errorf("message is empty")
				}
				return nil
			},
		},
		{
			name: "error_missing_params",
			desc: "restore_snapshot 缺参数",
			run: func(ctx context.Context, cs *mcpsdk.ClientSession) error {
				res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
					Name:      "restore_snapshot",
					Arguments: map[string]any{},
				})
				if err != nil {
					return fmt.Errorf("CallTool: %w", err)
				}
				if !res.IsError {
					return fmt.Errorf("expected IsError=true, got false")
				}
				return nil
			},
		},
		{
			name: "error_nonexistent_tool",
			desc: "调用不存在的工具",
			run: func(ctx context.Context, cs *mcpsdk.ClientSession) error {
				_, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
					Name:      "fake_tool_xyz",
					Arguments: map[string]any{},
				})
				if err == nil {
					return fmt.Errorf("expected error, got nil")
				}
				return nil
			},
		},
	}
}

// ---------------------------------------------------------------------------
// skipError — 标记测试应 SKIP 而非 FAIL
// ---------------------------------------------------------------------------

type skipError struct {
	reason string
}

func (e *skipError) Error() string { return e.reason }

func errSkip(reason string) error { return &skipError{reason: reason} }

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	flag.Parse()

	endpoint := fmt.Sprintf("http://%s/", *addr)

	fmt.Printf("%sMCP Client CLI — Network Digital Twin%s\n", colorCyan, colorReset)
	fmt.Printf("Connecting to %s ...\n", endpoint)

	// 创建 MCP Client
	ctx, cancel := context.WithTimeout(context.Background(), *timeout*2)
	defer cancel()

	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: "mcp-client", Version: "v1.0.0"},
		nil,
	)

	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:             endpoint,
		DisableStandaloneSSE: true,
		MaxRetries:           -1, // 禁用重试
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s[ERROR] Connect failed: %v%s\n", colorRed, err, colorReset)
		os.Exit(1)
	}
	defer session.Close()

	initResult := session.InitializeResult()
	fmt.Printf("Connected! Server: %s %s\n\n", initResult.ServerInfo.Name, initResult.ServerInfo.Version)

	// 运行测试
	tests := buildTests()
	passed, failed, skipped := 0, 0, 0

	for i, tc := range tests {
		// --run 过滤
		if *runOnly != "all" && tc.name != *runOnly {
			continue
		}

		testCtx, testCancel := context.WithTimeout(ctx, *timeout)
		err := tc.run(testCtx, session)
		testCancel()

		idx := i + 1
		total := len(tests)

		if err == nil {
			printResult(idx, total, tc.name, "PASS", tc.desc)
			passed++
		} else if se, ok := err.(*skipError); ok {
			printResult(idx, total, tc.name, "SKIP", se.reason)
			skipped++
		} else {
			printResult(idx, total, tc.name, "FAIL", err.Error())
			failed++
		}
	}

	// 汇总
	fmt.Printf("\n%s═══════════════════════════════════════════%s\n", colorCyan, colorReset)
	fmt.Printf("Results: %s%d passed%s, %s%d failed%s, %s%d skipped%s\n",
		colorGreen, passed, colorReset,
		colorRed, failed, colorReset,
		colorYellow, skipped, colorReset)
	fmt.Printf("%s═══════════════════════════════════════════%s\n", colorCyan, colorReset)

	if failed > 0 {
		os.Exit(failed)
	}

	// 打印工具详情
	if *runOnly == "all" {
		fmt.Printf("\n%s--- Tool Details ---%s\n", colorCyan, colorReset)
		listRes, err := session.ListTools(ctx, nil)
		if err == nil {
			for _, tool := range listRes.Tools {
				desc := tool.Description
				if len(desc) > 60 {
					desc = desc[:57] + "..."
				}
				fmt.Printf("  %-20s %s\n", tool.Name, strings.TrimSpace(desc))
			}
		}
	}
}
