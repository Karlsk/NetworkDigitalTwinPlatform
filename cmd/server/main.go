package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/config"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/connector/controller"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/connector/netbox"
	"gitlab.com/pml/network-digital-twin/internal/graph"
	intmcp "gitlab.com/pml/network-digital-twin/internal/mcp"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/schema"
	"gitlab.com/pml/network-digital-twin/internal/service"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// 2. 初始化 SchemaRegistry + 加载 ontology
	reg := schema.NewSchemaRegistry()
	if err := reg.Load(cfg.Schema.OntologyDir); err != nil {
		slog.Error("failed to load ontology", "dir", cfg.Schema.OntologyDir, "error", err)
		os.Exit(1)
	}

	// 3. 初始化 Neo4j 客户端
	gdb, err := graph.NewNeo4jClient(cfg.Neo4J)
	if err != nil {
		slog.Error("failed to connect neo4j", "error", err)
		os.Exit(1)
	}
	defer gdb.Close()

	// 3.1 收集所有 Label（含基类），创建 Neo4j 索引
	allLabels := collectAllLabels(reg)
	if err := gdb.EnsureIndexes(context.Background(), allLabels); err != nil {
		slog.Error("failed to ensure indexes", "error", err)
		os.Exit(1)
	}

	// 4. 初始化 GraphLock、Normalizer、GraphAssembler
	lock := snapshot.NewGraphLock()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)

	// 5. 初始化 ConnectorRegistry + ConnectorFactory（配置驱动，替代硬编码）
	connRegistry := connector.NewConnectorRegistry()
	factory := connector.NewConnectorFactory()

	// 5.1 注册内置 builder（因循环导入限制在 cmd 层注册）
	factory.RegisterBuilder("mock", func(name string, cfg map[string]any, entityTypes []string) (connector.Connector, error) {
		dataDir, _ := cfg["data_dir"].(string)
		return mock.NewMockConnector(name, dataDir, entityTypes), nil
	})
	factory.RegisterBuilder("netbox", netbox.Builder())
	factory.RegisterBuilder("controller", controller.Builder())

	// 5.2 从 connectors.yaml 配置批量创建并注册
	if err := factory.CreateFromConfig("configs/connectors.yaml", connRegistry); err != nil {
		slog.Error("init connectors", "error", err)
		os.Exit(1)
	}

	// 6. 初始化 SyncService
	syncSvc := service.NewSyncService(
		connRegistry, norm, asm, gdb, lock,
		cfg.Channel.BufferSize,
	)

	// 7. 初始化 SnapshotManager
	snapMgr := snapshot.NewSnapshotManager(
		gdb, lock, cfg.Snapshot.Dir, cfg.Snapshot.MaxActive,
	)

	// 8. 初始化 AnalysisService 和 SnapshotService
	analysisSvc := service.NewAnalysisService(gdb, lock)
	snapshotSvc := service.NewSnapshotService(snapMgr)

	// 9. 构建 MCP Server 并注册工具
	mcpServer := intmcp.NewNetworkTwinServer(analysisSvc, snapshotSvc, syncSvc)

	// 10. Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 11. 启动 Streamable HTTP MCP Server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	if err := intmcp.RunHTTP(ctx, mcpServer, addr); err != nil {
		slog.Error("MCP server error", "error", err)
		os.Exit(1)
	}
}

// collectAllLabels 遍历所有 EntityType，收集包含基类在内的所有 Label（去重）。
func collectAllLabels(reg schema.SchemaRegistry) []string {
	seen := make(map[string]bool)
	var labels []string
	for _, et := range reg.ListEntityTypes() {
		for _, label := range reg.GetLabels(et.Metadata.Name) {
			if !seen[label] {
				seen[label] = true
				labels = append(labels, label)
			}
		}
	}
	return labels
}
