package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gitlab.com/pml/network-digital-twin/internal/api"
	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/config"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/connector/controller"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/connector/netbox"
	"gitlab.com/pml/network-digital-twin/internal/events"
	"gitlab.com/pml/network-digital-twin/internal/graph"
	intmcp "gitlab.com/pml/network-digital-twin/internal/mcp"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/repository"
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

	// 6. 初始化事件总线层（EventBus Layer）
	// EventBus 是数据源和图数据库写入之间的中间管道层。
	// cfg.EventBus.Mode 决定使用哪种实现：
	//   - "channel": 内存 Channel（默认，V1 兼容）
	//   - "kafka":   Kafka Topic（持久化）
	// Fallback 机制仅作用于 EventBus 层：Kafka 不可用时自动降级到 Channel。
	var publisher events.EventPublisher
	var consumer events.EventConsumer

	switch cfg.EventBus.Mode {
	case "kafka":
		saramaCfg, err := events.NewSaramaConfig(cfg.EventBus.Kafka.SASLUser, cfg.EventBus.Kafka.SASLPass)
		if err != nil {
			slog.Error("create sarama config", "error", err)
			os.Exit(1)
		}

		// Kafka Publisher（primary）
		kafkaPub, err := events.NewKafkaPublisher(
			cfg.EventBus.Kafka.Brokers, cfg.EventBus.Kafka.Topic, saramaCfg,
		)
		if err != nil {
			slog.Error("create kafka publisher", "error", err)
			os.Exit(1)
		}

		// Channel Publisher（fallback）：Kafka 不可用时自动降级
		channelFallbackPub, _ := events.NewChannelEventBus(cfg.Channel.BufferSize)
		publisher = events.NewFallbackPublisher(kafkaPub, channelFallbackPub, 30*time.Second)

		// EventBus Consumer（V2-03）：与 Publisher 共享同一个 Topic
		consumer, err = events.NewKafkaConsumer(
			cfg.EventBus.Kafka.Brokers,
			cfg.EventBus.Kafka.Topic,
			cfg.EventBus.Kafka.GroupID,
			saramaCfg,
		)
		if err != nil {
			slog.Error("create kafka consumer", "error", err)
			os.Exit(1)
		}
		slog.Info("event bus: Kafka mode (with Channel fallback)",
			"brokers", cfg.EventBus.Kafka.Brokers,
			"topic", cfg.EventBus.Kafka.Topic,
		)
	default: // "channel"
		pub, con := events.NewChannelEventBus(cfg.Channel.BufferSize)
		publisher = pub
		consumer = con
		slog.Info("event bus: Channel mode", "buffer_size", cfg.Channel.BufferSize)
	}

	// 6.0.5 Graceful shutdown context（提前初始化，供 DataSource Consumer 使用）
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 6.1 初始化数据源层（DataSource Layer）
	// 数据源层负责从外部系统接收事件，通过 publisher.Publish(event) 写入 EventBus。
	// Webhook 数据源始终可用（通过 HandleWebhook 接收 HTTP 回调）。
	// Kafka 数据源可选启用（cfg.Kafka.Enabled），从外部 Kafka Topic 消费事件。
	if cfg.Kafka.Enabled {
		slog.Info("data source: Kafka enabled",
			"brokers", cfg.Kafka.Brokers,
			"topic", cfg.Kafka.Topic,
		)
		// V2-03: 启动 Kafka DataSource Consumer
		dsSaramaCfg, err := events.NewSaramaConfig(cfg.Kafka.SASLUser, cfg.Kafka.SASLPass)
		if err != nil {
			slog.Error("create data source sarama config", "error", err)
			os.Exit(1)
		}
		dsConsumer, err := events.NewKafkaDataSourceConsumer(
			cfg.Kafka.Brokers, cfg.Kafka.Topic, cfg.Kafka.GroupID, dsSaramaCfg,
		)
		if err != nil {
			slog.Error("create kafka data source consumer", "error", err)
			os.Exit(1)
		}
		defer dsConsumer.Close()
		// 数据源消费者在后台运行：消费外部 Topic → publisher.Publish(event) → EventBus
		go func() {
			if err := dsConsumer.Start(ctx, publisher); err != nil && ctx.Err() == nil {
				slog.Error("kafka data source consumer stopped", "error", err)
			}
		}()
	}

	// 7. 初始化 PostgreSQL 连接池（V2-05）
	// pool 共享给 SnapshotRepository (V2-06) 和 SyncLogRepository (V2-07)
	var pgPool *pgxpool.Pool
	if cfg.Postgres.Enabled {
		pool, pgErr := repository.NewPGPool(ctx, repository.PGConfig{
			URL:      cfg.Postgres.URL,
			MaxConns: cfg.Postgres.MaxConns,
			MinConns: cfg.Postgres.MinConns,
		})
		if pgErr != nil {
			slog.Warn("postgresql disabled: connection failed, falling back to memory", "error", pgErr)
		} else {
			pgPool = pool
			defer pool.Close()
			if migErr := repository.RunMigrations(pool); migErr != nil {
				slog.Warn("database migrations failed", "error", migErr)
			}
			slog.Info("postgresql connected")
		}
	}

	// 7.1 初始化 SyncService
	var syncOpts []service.SyncOption
	if pgPool != nil {
		syncOpts = append(syncOpts, service.WithSyncLogRepository(
			repository.NewPGSyncLogRepository(pgPool),
		))
	}
	syncSvc := service.NewSyncService(
		connRegistry, norm, asm, gdb, lock, publisher, consumer, syncOpts...,
	)

	// 7.2 V2-08: 启动时同步 connectors.yaml 到 PostgreSQL
	var connRepo repository.ConnectorConfigRepository
	if pgPool != nil {
		connRepo = repository.NewPGConnectorRepository(pgPool)
		for _, meta := range connRegistry.List() {
			configJSON, _ := json.Marshal(map[string]any{})
			etJSON, _ := json.Marshal(meta.EntityTypes)
			if err := connRepo.Upsert(ctx, repository.ConnectorConfigRecord{
				Name: meta.Name, Type: meta.Type,
				Config: configJSON, EntityTypes: etJSON,
				Status: "active",
			}); err != nil {
				slog.Warn("upsert connector config", "name", meta.Name, "error", err)
			}
		}
		slog.Info("connector configs synced to postgresql", "count", len(connRegistry.List()))
	}

	// 7.3 V2-08: Schema 版本追踪
	var schemaRepo repository.SchemaVersionRepository
	if pgPool != nil {
		schemaRepo = repository.NewPGSchemaVersionRepository(pgPool)
		currentVersion := computeSchemaVersion(reg)
		latest, _ := schemaRepo.Latest(ctx)
		if latest == nil || latest.Version != currentVersion {
			if err := schemaRepo.Create(ctx, &repository.SchemaVersionRecord{
				Version:       currentVersion,
				EntityTypes:   marshalEntityTypes(reg),
				RelationTypes: marshalRelationTypes(reg),
				AppliedAt:     time.Now(),
				Description:   "auto-detected schema change",
			}); err != nil {
				slog.Warn("record schema version", "error", err)
			} else {
				slog.Info("schema version recorded", "version", currentVersion)
			}
		}
	}

	// 8. 初始化 SnapshotManager
	var snapOpts []snapshot.Option
	if pgPool != nil {
		snapOpts = append(snapOpts, snapshot.WithSnapshotRepository(
			repository.NewPGSnapshotRepository(pgPool),
		))
		snapOpts = append(snapOpts, snapshot.WithAuditRepository(
			repository.NewPGAuditLogRepository(pgPool),
		))
		slog.Info("postgresql snapshot + audit repository enabled")
	}
	snapMgr := snapshot.NewSnapshotManager(
		gdb, lock, cfg.Snapshot.Dir, cfg.Snapshot.MaxActive, snapOpts...,
	)
	snapMgr.SetRetentionDays(cfg.Snapshot.RetentionDays) // V1-20: TTL 保留策略

	// 9. 初始化 AnalysisService 和 SnapshotService
	analysisSvc := service.NewAnalysisService(gdb, lock)
	snapshotSvc := service.NewSnapshotService(snapMgr)

	// 10. 初始化 DeviceService（只读，不需要 GraphLock）
	deviceSvc := service.NewDeviceService(connRegistry)

	// 11. 构建 MCP Server 并注册工具
	mcpServer := intmcp.NewNetworkTwinServer(analysisSvc, snapshotSvc, syncSvc, deviceSvc)

	// 12. 创建 Gin HTTP API Server（V2-11）
	apiServer := api.NewServer()
	apiServer.RegisterRoutes(&api.HandlerDeps{
		SyncSvc:     syncSvc,
		SnapshotSvc: snapshotSvc,
		AnalysisSvc: analysisSvc,
		DeviceSvc:   deviceSvc,
	})

	// 12.1 MCP 路由嵌入 Gin（/mcp/*path）
	mcpHandler := mcpsdk.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcpsdk.Server { return mcpServer }, nil,
	)
	apiServer.Engine().Any("/mcp/*path", gin.WrapH(mcpHandler))

	// 13. 启动 Gin HTTP API Server（统一端口 8080）
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- apiServer.Run(ctx, addr)
	}()

	// 14. 等待退出信号或服务器错误
	select {
	case <-ctx.Done():
		slog.Info("received shutdown signal")
	case err := <-serverErr:
		if err != nil {
			slog.Error("HTTP API server error", "error", err)
		}
	}

	// 15. 优雅退出：先停 consumer，再停 publisher
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	slog.Info("shutting down event bus...")
	if err := consumer.Close(); err != nil {
		slog.Error("close consumer", "error", err)
	}
	if err := publisher.Close(); err != nil {
		slog.Error("close publisher", "error", err)
	}
	slog.Info("shutdown complete", "timeout", shutdownCtx)
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

// computeSchemaVersion 基于所有 EntityType 名称排序后计算 SHA256 哈希，取前 4 字节转为 int。
func computeSchemaVersion(reg schema.SchemaRegistry) int {
	names := make([]string, 0, len(reg.ListEntityTypes()))
	for _, et := range reg.ListEntityTypes() {
		names = append(names, et.Metadata.Name)
	}
	sort.Strings(names)
	h := sha256.Sum256([]byte(strings.Join(names, ",")))
	return int(binary.BigEndian.Uint32(h[:4]))
}

// marshalEntityTypes 将所有 EntityType 序列化为 JSON。
func marshalEntityTypes(reg schema.SchemaRegistry) []byte {
	type etSummary struct {
		Name   string   `json:"name"`
		Labels []string `json:"labels,omitempty"`
	}
	items := make([]etSummary, 0, len(reg.ListEntityTypes()))
	for _, et := range reg.ListEntityTypes() {
		items = append(items, etSummary{
			Name:   et.Metadata.Name,
			Labels: et.Metadata.Labels,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	data, _ := json.Marshal(items)
	return data
}

// marshalRelationTypes 将所有 RelationType 序列化为 JSON。
func marshalRelationTypes(reg schema.SchemaRegistry) []byte {
	type rtSummary struct {
		Name   string   `json:"name"`
		Source []string `json:"source"`
		Target []string `json:"target"`
	}
	items := make([]rtSummary, 0, len(reg.ListRelationTypes()))
	for _, rt := range reg.ListRelationTypes() {
		items = append(items, rtSummary{
			Name:   rt.Metadata.Name,
			Source: rt.Spec.Source,
			Target: rt.Spec.Target,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	data, _ := json.Marshal(items)
	return data
}
