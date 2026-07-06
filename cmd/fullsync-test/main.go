// 全量同步测试：加载 connectors.yaml → 全量采集 → 写入 Neo4j → 查询验证
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/config"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/connector/controller"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/connector/netbox"
	"gitlab.com/pml/network-digital-twin/internal/events"
	"gitlab.com/pml/network-digital-twin/internal/graph"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/schema"
	"gitlab.com/pml/network-digital-twin/internal/service"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  全量同步测试 — Controller + Neo4j                      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")

	// 1. 加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	// 2. Schema Registry
	reg := schema.NewSchemaRegistry()
	if err := reg.Load(cfg.Schema.OntologyDir); err != nil {
		slog.Error("load ontology", "error", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Ontology loaded: %d entity types, %d relation types\n",
		len(reg.ListEntityTypes()), len(reg.ListRelationTypes()))

	// 3. Neo4j
	gdb, err := graph.NewNeo4jClient(cfg.Neo4J)
	if err != nil {
		slog.Error("neo4j connect", "error", err)
		os.Exit(1)
	}
	defer gdb.Close()
	fmt.Println("✓ Neo4j connected")

	// 4. 组件初始化
	lock := snapshot.NewGraphLock()
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)

	// 5. Connector Factory
	connRegistry := connector.NewConnectorRegistry()
	factory := connector.NewConnectorFactory()
	factory.RegisterBuilder("mock", func(name string, cfg map[string]any, entityTypes []string) (connector.Connector, error) {
		dataDir, _ := cfg["data_dir"].(string)
		return mock.NewMockConnector(name, dataDir, entityTypes), nil
	})
	factory.RegisterBuilder("netbox", netbox.Builder())
	factory.RegisterBuilder("controller", controller.Builder())

	if err := factory.CreateFromConfig("configs/connectors.yaml", connRegistry); err != nil {
		slog.Error("init connectors", "error", err)
		os.Exit(1)
	}
	metas := connRegistry.List()
	fmt.Printf("✓ Connectors loaded: %d\n", len(metas))
	for _, m := range metas {
		fmt.Printf("    %s (%s) → %v\n", m.Name, m.Type, m.EntityTypes)
	}

	// 6. SyncService
	pub, con := events.NewChannelEventBus(cfg.Channel.BufferSize)
	syncSvc := service.NewSyncService(connRegistry, norm, asm, gdb, lock, pub, con)

	// 7. FullSync
	fmt.Println("\n▶ Starting FullSync...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := syncSvc.FullSync(ctx)
	if err != nil {
		slog.Error("full sync failed", "error", err)
		os.Exit(1)
	}

	fmt.Printf("\n━━━ FullSync Result ━━━\n")
	fmt.Printf("  Nodes created     : %d\n", result.NodesCreated)
	fmt.Printf("  Relations created : %d\n", result.RelationsCreated)
	fmt.Printf("  Orphan edges      : %d\n", result.OrphanEdgesSkipped)
	fmt.Printf("  Warnings          : %d\n", len(result.Warnings))
	fmt.Printf("  Duration          : %v\n", result.Duration)

	// 8. 查询 Neo4j 验证数据
	fmt.Println("\n━━━ Neo4j 数据验证 ━━━")
	db := "neo4j" // default db
	labels := []string{"Device", "Interface", "Link", "Alarm", "VPN", "Tunnel", "ISIS", "BGP"}
	for _, label := range labels {
		cypher := fmt.Sprintf("MATCH (n:%s) WHERE n._db = 'default' RETURN count(n) AS cnt", label)
		rows, err := gdb.Query(ctx, db, cypher, nil)
		if err != nil {
			fmt.Printf("  %-12s: ERROR (%v)\n", label, err)
			continue
		}
		if len(rows) > 0 {
			cnt := rows[0]["cnt"]
			fmt.Printf("  %-12s: %v nodes\n", label, cnt)
		}
	}

	// 9. 查询关系数量
	relCypher := "MATCH ()-[r]->() WHERE r._db = 'default' RETURN type(r) AS rel, count(r) AS cnt ORDER BY cnt DESC"
	rows, err := gdb.Query(ctx, db, relCypher, nil)
	if err == nil && len(rows) > 0 {
		fmt.Println("\n━━━ 关系统计 ━━━")
		for _, row := range rows {
			fmt.Printf("  %-30s: %v\n", row["rel"], row["cnt"])
		}
	}

	fmt.Println("\n✓ FullSync test complete!")
}
