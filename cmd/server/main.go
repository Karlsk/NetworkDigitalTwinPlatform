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
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
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

	// 4. 初始化 GraphLock、Normalizer、GraphAssembler
	lock := snapshot.NewGraphLock()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)

	// 5. 初始化 ConnectorRegistry
	connRegistry := connector.NewConnectorRegistry()

	// 5.1 注册 Mock Connector（开发/演示用，对齐 connectors.yaml）
	netboxConn := mock.NewMockConnector("mock-netbox", "testdata/mock_netbox", []string{"Device", "Interface"})
	connRegistry.Register(netboxConn)
	cmdbConn := mock.NewMockConnector("mock-cmdb", "testdata/mock_cmdb", []string{"ISIS", "Link", "Network_Slice"})
	connRegistry.Register(cmdbConn)

	// 6. 初始化 SyncService
	syncSvc := service.NewSyncService(
		connRegistry, norm, asm, gdb, lock,
		cfg.Channel.BufferSize,
	)

	// 7. 初始化 SnapshotManager
	snapMgr := snapshot.NewSnapshotManager(
		gdb, lock, cfg.Snapshot.Dir, cfg.Snapshot.MaxActive,
	)

	// 8. 构建 MCP Server 并注册工具
	mcpServer := intmcp.NewNetworkTwinServer(gdb, lock, snapMgr, syncSvc)

	// 9. Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 10. 启动 Streamable HTTP MCP Server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	if err := intmcp.RunHTTP(ctx, mcpServer, addr); err != nil {
		slog.Error("MCP server error", "error", err)
		os.Exit(1)
	}
}
