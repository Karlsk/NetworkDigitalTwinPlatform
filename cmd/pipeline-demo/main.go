// pipeline-demo 端到端演示完整数据流管线:
// Config → Schema → Connector → Normalizer → Assembler → GraphDB → Neo4j
//
// 运行方式 (项目根目录):
//
//	go run ./cmd/pipeline-demo/
//
// 前置条件:
//
//	1. Neo4j 已启动: docker-compose -f deploy/docker-compose.yml up -d
//	2. Neo4j 密码: 默认 "password"，可通过 NEO4J_PASSWORD 环境变量覆盖
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/config"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/connector/controller"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/connector/netbox"
	"gitlab.com/pml/network-digital-twin/internal/graph"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/schema"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// section 打印分隔线 + 标题
func section(title string) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("  %s\n", title)
	fmt.Println(strings.Repeat("=", 70))
}

// checkpoint 打印检查点
func checkpoint(name string, pass bool) {
	status := "PASS"
	if !pass {
		status = "FAIL"
	}
	fmt.Printf("  [✓] %-40s %s\n", name, status)
}

// printJSON 格式化打印 JSON (用于属性展示)
func printJSON(prefix string, v any) {
	b, _ := json.MarshalIndent(v, prefix, "  ")
	fmt.Printf("%s%s\n", prefix, string(b))
}

// propsPreview 截取属性的前 5 个 key-value 用于预览输出
func propsPreview(props map[string]any) map[string]any {
	preview := make(map[string]any, 5)
	i := 0
	for k, v := range props {
		if i >= 5 {
			break
		}
		preview[k] = v
		i++
	}
	if len(props) > 5 {
		preview["..."] = fmt.Sprintf("(%d more)", len(props)-5)
	}
	return preview
}

func main() {
	ctx := context.Background()
	startTime := time.Now()

	// ================================================================
	// Step 0: 加载全局配置
	// ================================================================
	section("Step 0: 加载全局配置")

	cfg, err := config.Load(filepath.Join("configs", "config.yaml"))
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	fmt.Printf("  Neo4j URI:     %s\n", cfg.Neo4J.URI)
	fmt.Printf("  Neo4j User:    %s\n", cfg.Neo4J.User)
	fmt.Printf("  Default DB:    %s\n", cfg.Neo4J.DefaultDB)
	fmt.Printf("  Ontology Dir:  %s\n", cfg.Schema.OntologyDir)
	checkpoint("配置加载成功", true)

	// ================================================================
	// Step 1: 连接 Neo4j
	// ================================================================
	section("Step 1: 连接 Neo4j")

	client, err := graph.NewNeo4jClient(cfg.Neo4J)
	if err != nil {
		log.Fatalf("创建 Neo4j 客户端失败: %v", err)
	}
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		log.Fatalf("Neo4j 连接失败 (请确认 docker-compose up -d 已启动): %v", err)
	}
	fmt.Println("  Neo4j 连接成功")
	checkpoint("Ping 连通性检查", true)

	// ================================================================
	// Step 2: 创建复合索引
	// ================================================================
	section("Step 2: 创建 (_db, uri) 复合索引")

	labels := []string{"Device", "Interface", "ISIS", "Link", "Network_Slice", "Alarm", "VPN", "BGP", "Tunnel"}
	if err := client.EnsureIndexes(ctx, labels); err != nil {
		log.Fatalf("创建索引失败: %v", err)
	}
	fmt.Printf("  为 %d 个 Label 创建索引\n", len(labels))
	checkpoint("索引创建成功 (幂等)", true)

	// ================================================================
	// Step 3: 加载本体 Schema
	// ================================================================
	section("Step 3: 加载本体 Schema (ontology/)")

	reg := schema.NewSchemaRegistry()
	if err := reg.Load(cfg.Schema.OntologyDir); err != nil {
		log.Fatalf("加载本体失败: %v", err)
	}

	entityTypes := reg.ListEntityTypes()
	relationTypes := reg.ListRelationTypes()
	fmt.Printf("  EntityType 数量:  %d\n", len(entityTypes))
	for _, et := range entityTypes {
		fmt.Printf("    - %s (stableKeys: %v, uriTemplate: %q)\n",
			et.Metadata.Name, et.Spec.Identity.StableKeys, et.Spec.URITemplate)
	}
	fmt.Printf("  RelationType 数量: %d\n", len(relationTypes))
	for _, rt := range relationTypes {
		fmt.Printf("    - %s (%v -> %v)\n",
			rt.Metadata.Name, rt.Spec.Source, rt.Spec.Target)
	}
	checkpoint("Schema 加载完整", len(entityTypes) == 9 && len(relationTypes) == 7)

	// ================================================================
	// Step 4: ConnectorFactory 数据采集 (配置驱动)
	// ================================================================
	section("Step 4: ConnectorFactory 数据采集 (配置驱动)")

	connReg := connector.NewConnectorRegistry()
	factory := connector.NewConnectorFactory()

	// 注册内置 builder
	factory.RegisterBuilder("mock", func(name string, cfg map[string]any, entityTypes []string) (connector.Connector, error) {
		dataDir, _ := cfg["data_dir"].(string)
		return mock.NewMockConnector(name, dataDir, entityTypes), nil
	})
	factory.RegisterBuilder("netbox", netbox.Builder())
	factory.RegisterBuilder("controller", controller.Builder())

	// 从 connectors.yaml 配置批量创建
	configPath := filepath.Join("configs", "connectors.yaml")
	if err := factory.CreateFromConfig(configPath, connReg); err != nil {
		fmt.Printf("  [警告] ConnectorFactory 创建失败: %v\n", err)
		fmt.Println("  回退: 仅使用 Mock Connector")
		// 回退: 仅注册 Mock Connector
		dataDir := filepath.Join("testdata", "mock_netbox")
		conn := mock.NewMockConnector("mock-netbox", dataDir,
			[]string{"Device", "Interface", "ISIS", "Link", "Network_Slice"})
		connReg.Register(conn)
	}

	fmt.Printf("  已注册 Connector 数量: %d\n", len(connReg.List()))
	for _, meta := range connReg.List() {
		fmt.Printf("    - %s (type=%s, entities=%v)\n", meta.Name, meta.Type, meta.EntityTypes)
	}

	var allResources []connector.Resource
	totalRaw := 0

	for _, meta := range connReg.List() {
		c, _ := connReg.Get(meta.Name)
		if c == nil {
			continue
		}
		for _, et := range meta.EntityTypes {
			resources, err := c.Collect(ctx, et)
			if err != nil {
				fmt.Printf("  [跳过] %s/%s: %v\n", meta.Name, et, err)
				continue
			}
			totalRaw += len(resources)
			allResources = append(allResources, resources...)

			fmt.Printf("  %s/%s: %d 条\n", meta.Name, et, len(resources))
			if len(resources) > 0 {
				fmt.Printf("    示例 (ID=%s): ", resources[0].ID)
				printJSON("    ", propsPreview(resources[0].Properties))
			}
		}
	}
	fmt.Printf("\n  采集总计: %d 条原始 Resource\n", totalRaw)
	checkpoint("Connector 采集成功", totalRaw > 0)

	// ================================================================
	// Step 5: Normalizer 归一化
	// ================================================================
	section("Step 5: Normalizer 归一化处理")

	norm := normalizer.NewNormalizer(reg)
	var allNormalized []normalizer.NormalizedResource
	normalizeFailed := 0

	for _, res := range allResources {
		nr, err := norm.Normalize(res)
		if err != nil {
			normalizeFailed++
			fmt.Printf("  [归一化失败] %s/%s: %v\n", res.Kind, res.ID, err)
			continue
		}
		allNormalized = append(allNormalized, *nr)
	}

	fmt.Printf("  归一化成功: %d 条\n", len(allNormalized))
	fmt.Printf("  归一化失败: %d 条\n", normalizeFailed)

	// 打印每种实体类型的第 1 条归一化结果
	printed := make(map[string]bool)
	for _, nr := range allNormalized {
		if printed[nr.Kind] {
			continue
		}
		printed[nr.Kind] = true
		fmt.Printf("\n  --- %s 示例 ---\n", nr.Kind)
		fmt.Printf("  URI: %s\n", nr.URI)
		fmt.Printf("  属性: ")
		printJSON("  ", propsPreview(nr.Properties))
	}
	checkpoint("Normalizer 归一化完成", len(allNormalized) > 0)

	// ================================================================
	// Step 6: GraphAssembler 图组装
	// ================================================================
	section("Step 6: GraphAssembler 图组装 (节点转换 + 关系推导 + 孤儿边校验)")

	asm := assembler.NewGraphAssembler(reg)
	gm, warnings, err := asm.Assemble(allNormalized)
	if err != nil {
		log.Fatalf("Assemble 失败: %v", err)
	}

	fmt.Printf("  节点总数:   %d\n", len(gm.Nodes))
	fmt.Printf("  关系总数:   %d\n", len(gm.Relations))
	fmt.Printf("  孤儿边警告: %d\n", len(warnings))

	// 按 MostSpecificLabel 统计节点
	labelCount := make(map[string]int)
	for _, n := range gm.Nodes {
		labelCount[n.MostSpecificLabel()]++
	}
	fmt.Println("\n  节点分布:")
	for label, cnt := range labelCount {
		fmt.Printf("    %s: %d\n", label, cnt)
	}

	// 按 Type 统计关系
	typeCount := make(map[string]int)
	for _, r := range gm.Relations {
		typeCount[r.Type]++
	}
	fmt.Println("  关系分布:")
	for relType, cnt := range typeCount {
		fmt.Printf("    %s: %d\n", relType, cnt)
	}

	if len(warnings) > 0 {
		fmt.Println("  孤儿边详情:")
		for _, w := range warnings {
			fmt.Printf("    %s: %s\n", w.Type, w.Detail)
		}
	}
	checkpoint("GraphAssembler 组装完成", len(gm.Nodes) > 0 && len(gm.Relations) > 0)

	// ================================================================
	// Step 7: BulkCreate 全量写入 Neo4j
	// ================================================================
	section("Step 7: BulkCreate 全量写入 Neo4j (逻辑 DB: default)")

	// 先清空 default DB
	if err := client.ClearDB(ctx, "default"); err != nil {
		log.Fatalf("ClearDB 失败: %v", err)
	}
	fmt.Println("  ClearDB(\"default\") 完成")

	t0 := time.Now()
	if err := client.BulkCreate(ctx, "default", gm.Nodes, gm.Relations); err != nil {
		log.Fatalf("BulkCreate 失败: %v", err)
	}
	fmt.Printf("  BulkCreate 耗时: %v\n", time.Since(t0).Round(time.Millisecond))
	checkpoint("BulkCreate 写入成功", true)

	// ================================================================
	// Step 8: 查询验证 — 节点和关系数量
	// ================================================================
	section("Step 8: 查询验证 Neo4j 数据")

	// 总节点数
	nodeRows, err := client.Query(ctx, "default",
		"MATCH (n) WHERE n._db = $_db RETURN count(n) AS cnt", nil)
	if err != nil {
		log.Fatalf("查询节点数失败: %v", err)
	}
	neo4jNodeCount := toInt(nodeRows, "cnt")
	fmt.Printf("  Neo4j 节点数: %d (预期 %d)\n", neo4jNodeCount, len(gm.Nodes))
	checkpoint("节点数一致", neo4jNodeCount == len(gm.Nodes))

	// 总关系数
	relRows, err := client.Query(ctx, "default",
		"MATCH (a)-[r]->(b) WHERE a._db = $_db RETURN count(r) AS cnt", nil)
	if err != nil {
		log.Fatalf("查询关系数失败: %v", err)
	}
	neo4jRelCount := toInt(relRows, "cnt")
	fmt.Printf("  Neo4j 关系数: %d (预期 %d)\n", neo4jRelCount, len(gm.Relations))
	checkpoint("关系数一致", neo4jRelCount == len(gm.Relations))

	// 按 Label 统计
	fmt.Println("\n  按 Label 查询节点:")
	for _, label := range labels {
		rows, err := client.Query(ctx, "default",
			fmt.Sprintf("MATCH (n:%s) WHERE n._db = $_db RETURN count(n) AS cnt", label), nil)
		if err != nil {
			fmt.Printf("    %s: 查询失败\n", label)
			continue
		}
		cnt := toInt(rows, "cnt")
		expected := labelCount[label]
		match := ""
		if cnt != expected {
			match = fmt.Sprintf(" (预期 %d)", expected)
		}
		fmt.Printf("    %-18s %d%s\n", label, cnt, match)
	}

	// 按 RelType 统计
	fmt.Println("  按 Type 查询关系:")
	for _, relType := range []string{"HAS_INTERFACE", "CONNECTS_TO", "RUNS_ON", "ENDPOINT", "HAS_ALARM", "HAS_VPN", "HAS_BGP_PEER", "HAS_ISIS_PEER", "TUNNEL_FOR"} {
		rows, err := client.Query(ctx, "default",
			fmt.Sprintf("MATCH (a)-[r:%s]->(b) WHERE a._db = $_db RETURN count(r) AS cnt", relType), nil)
		if err != nil {
			fmt.Printf("    %s: 查询失败\n", relType)
			continue
		}
		cnt := toInt(rows, "cnt")
		expected := typeCount[relType]
		match := ""
		if cnt != expected {
			match = fmt.Sprintf(" (预期 %d)", expected)
		}
		fmt.Printf("    %-18s %d%s\n", relType, cnt, match)
	}

	// 抽样验证 Device 属性
	fmt.Println("\n  抽样: 查询 Device 节点属性")
	deviceRows, err := client.Query(ctx, "default",
		"MATCH (d:Device) WHERE d._db = $_db RETURN d.uri AS uri, d.hostname AS hostname, d.status AS status, d.management_ip AS mgmt_ip ORDER BY d.uri", nil)
	if err != nil {
		log.Fatalf("查询 Device 失败: %v", err)
	}
	for _, row := range deviceRows {
		fmt.Printf("    URI=%-25s hostname=%-20s status=%-6s mgmt_ip=%s\n",
			row["uri"], row["hostname"], row["status"], row["mgmt_ip"])
	}
	checkpoint("Device 数据正确", len(deviceRows) == labelCount["Device"])

	// ================================================================
	// Step 9: 逻辑多 DB 隔离验证
	// ================================================================
	section("Step 9: 逻辑多 DB 隔离验证")

	testDB := "pipeline_demo_isolation_test"
	defer func() { _ = client.ClearDB(ctx, testDB) }()

	// 向 testDB 写入 2 个节点
	testNodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "test:iso-001", Props: map[string]any{"hostname": "IsolatedDevice"}},
		{Labels: []string{"Device"}, URI: "test:iso-002", Props: map[string]any{"hostname": "IsolatedDevice2"}},
	}
	if err := client.BulkCreate(ctx, testDB, testNodes, nil); err != nil {
		log.Fatalf("BulkCreate testDB 失败: %v", err)
	}

	// 验证 testDB 有 2 个节点
	rows, _ := client.Query(ctx, testDB,
		"MATCH (n) WHERE n._db = $_db RETURN count(n) AS cnt", nil)
	testDBCount := toInt(rows, "cnt")
	fmt.Printf("  testDB (%s) 节点数: %d (预期 2)\n", testDB, testDBCount)
	checkpoint("testDB 写入成功", testDBCount == 2)

	// 验证 default 不受影响
	rows, _ = client.Query(ctx, "default",
		"MATCH (n) WHERE n._db = $_db RETURN count(n) AS cnt", nil)
	defaultCount := toInt(rows, "cnt")
	fmt.Printf("  default DB 节点数: %d (预期 %d，未变化)\n", defaultCount, len(gm.Nodes))
	checkpoint("default DB 未受影响", defaultCount == len(gm.Nodes))

	// ListDBs 验证
	dbs, _ := client.ListDBs(ctx)
	fmt.Printf("  ListDBs: %v\n", dbs)
	checkpoint("ListDBs 包含两个 DB", len(dbs) >= 2)

	// HasDB 验证
	hasTest, _ := client.HasDB(ctx, testDB)
	hasDefault, _ := client.HasDB(ctx, "default")
	hasGhost, _ := client.HasDB(ctx, "nonexistent_db")
	fmt.Printf("  HasDB(%s): %v\n", testDB, hasTest)
	fmt.Printf("  HasDB(default): %v\n", hasDefault)
	fmt.Printf("  HasDB(nonexistent_db): %v\n", hasGhost)
	checkpoint("HasDB 判断正确", hasTest && hasDefault && !hasGhost)

	// ================================================================
	// Step 10: Upsert 增量更新验证
	// ================================================================
	section("Step 10: Upsert 增量更新 (MERGE + SET +=)")

	// 先查询 Device SN12345 当前状态
	fmt.Println("  Upsert 前: 查询 device:SN12345")
	beforeRows, _ := client.Query(ctx, "default",
		"MATCH (d:Device) WHERE d._db = $_db AND d.uri = 'device:SN12345' RETURN d.status AS status, d.hostname AS hostname", nil)
	for _, row := range beforeRows {
		fmt.Printf("    status=%v, hostname=%v\n", row["status"], row["hostname"])
	}

	// Upsert: 修改 status 为 "Maintenance"，新增一个属性
	upsertNodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:SN12345", Props: map[string]any{
			"status":      "Maintenance",
			"description": "updated by pipeline-demo",
		}},
	}
	if err := client.Upsert(ctx, "default", upsertNodes, nil); err != nil {
		log.Fatalf("Upsert 失败: %v", err)
	}

	// 验证: status 已更新，hostname 保留
	fmt.Println("  Upsert 后: 查询 device:SN12345")
	afterRows, _ := client.Query(ctx, "default",
		"MATCH (d:Device) WHERE d._db = $_db AND d.uri = 'device:SN12345' RETURN d.status AS status, d.hostname AS hostname, d.description AS desc", nil)
	for _, row := range afterRows {
		fmt.Printf("    status=%v, hostname=%v, desc=%v\n", row["status"], row["hostname"], row["desc"])
	}

	statusUpdated := false
	hostnamePreserved := false
	descAdded := false
	if len(afterRows) > 0 {
		statusUpdated = afterRows[0]["status"] == "Maintenance"
		hostnamePreserved = afterRows[0]["hostname"] != nil && afterRows[0]["hostname"] != ""
		descAdded = afterRows[0]["desc"] == "updated by pipeline-demo"
	}
	checkpoint("status 更新为 Maintenance", statusUpdated)
	checkpoint("hostname 保留 (增量合并)", hostnamePreserved)
	checkpoint("新增 description 属性", descAdded)

	// 总节点数不应变化 (MERGE 幂等)
	rows2, _ := client.Query(ctx, "default",
		"MATCH (n) WHERE n._db = $_db RETURN count(n) AS cnt", nil)
	afterUpsertCount := toInt(rows2, "cnt")
	fmt.Printf("  Upsert 后总节点数: %d (预期 %d，不变)\n", afterUpsertCount, len(gm.Nodes))
	checkpoint("节点数不变 (MERGE 幂等)", afterUpsertCount == len(gm.Nodes))

	// ================================================================
	// Step 11: BuildCypher 预览验证
	// ================================================================
	section("Step 11: BuildCypher 预览 (不执行)")

	previewNodes := []assembler.Node{
		{Labels: []string{"Device"}, URI: "device:preview-001", Props: map[string]any{"hostname": "PreviewRouter"}},
	}
	cypher, params := client.BuildCypher("create", "preview_db", previewNodes, nil, nil)
	fmt.Println("  BuildCypher(\"create\") 输出:")
	fmt.Printf("  Cypher: %s\n", cypher)
	fmt.Printf("  Params: ")
	printJSON("  ", params)
	checkpoint("BuildCypher 返回合法 Cypher", cypher != "")

	// ================================================================
	// Step 12: 快照生命周期演示
	// ================================================================
	section("Step 12: 快照生命周期 (Create -> List -> Diff -> Restore)")

	lock := snapshot.NewGraphLock()
	snapDir := filepath.Join("snapshots", "demo")
	_ = os.MkdirAll(snapDir, 0o755)

	sm := snapshot.NewSnapshotManager(client, lock, snapDir, cfg.Snapshot.MaxActive)

	// Create 快照
	fmt.Println("  创建快照 snap-demo-001 ...")
	meta1, err := sm.Create(ctx, "snap-demo-001")
	if err != nil {
		log.Fatalf("快照创建失败: %v", err)
	}
	fmt.Printf("  快照: name=%s, nodes=%d, rels=%d, file=%s\n",
		meta1.Name, meta1.NodeCount, meta1.RelCount, meta1.FilePath)
	checkpoint("快照创建成功", meta1.NodeCount > 0)

	// 修改数据: 删除一个 Device
	fmt.Println("\n  修改数据: 删除 device:SN12345 ...")
	if err := client.DeleteByURIs(ctx, "default", []string{"device:SN12345"}); err != nil {
		log.Fatalf("删除失败: %v", err)
	}
	rows3, _ := client.Query(ctx, "default",
		"MATCH (n) WHERE n._db = $_db RETURN count(n) AS cnt", nil)
	fmt.Printf("  删除后节点数: %d\n", toInt(rows3, "cnt"))

	// 再创建快照
	fmt.Println("\n  创建快照 snap-demo-002 ...")
	meta2, err := sm.Create(ctx, "snap-demo-002")
	if err != nil {
		log.Fatalf("快照创建失败: %v", err)
	}
	fmt.Printf("  快照: name=%s, nodes=%d, rels=%d\n",
		meta2.Name, meta2.NodeCount, meta2.RelCount)

	// Diff 对比
	fmt.Println("\n  Diff: snap-demo-001 vs snap-demo-002 ...")
	diff, err := sm.Diff(ctx, "snap-demo-001", "snap-demo-002")
	if err != nil {
		log.Fatalf("Diff 失败: %v", err)
	}
	fmt.Printf("  新增节点: %d\n", len(diff.AddedNodes))
	fmt.Printf("  删除节点: %d\n", len(diff.RemovedNodes))
	fmt.Printf("  新增关系: %d\n", len(diff.AddedRels))
	fmt.Printf("  删除关系: %d\n", len(diff.RemovedRels))
	for _, n := range diff.RemovedNodes {
		fmt.Printf("    删除: %s (%s)\n", n.URI, n.MostSpecificLabel())
	}
	checkpoint("Diff 检测到差异", len(diff.RemovedNodes) > 0)

	// Restore 恢复到 snap-demo-001
	fmt.Println("\n  Restore: 恢复到 snap-demo-001 ...")
	if err := sm.Restore(ctx, "snap-demo-001"); err != nil {
		log.Fatalf("Restore 失败: %v", err)
	}
	rows4, _ := client.Query(ctx, "default",
		"MATCH (n) WHERE n._db = $_db RETURN count(n) AS cnt", nil)
	restoredCount := toInt(rows4, "cnt")
	fmt.Printf("  Restore 后节点数: %d (预期 %d)\n", restoredCount, meta1.NodeCount)
	checkpoint("Restore 恢复成功", restoredCount == meta1.NodeCount)

	// List 快照
	fmt.Println("\n  List 快照:")
	snaps, _ := sm.List(ctx)
	for _, s := range snaps {
		fmt.Printf("    name=%s, nodes=%d, rels=%d, created=%s\n",
			s.Name, s.NodeCount, s.RelCount, s.CreatedAt.Format(time.RFC3339))
	}

	// ================================================================
	// Step 13: 清理 + 总结
	// ================================================================
	section("Step 13: 清理 + 验证总结")

	// 清理测试数据
	_ = client.ClearDB(ctx, "pipeline_demo_isolation_test")
	_ = client.ClearDB(ctx, "snap-demo-001")
	_ = client.ClearDB(ctx, "snap-demo-002")
	_ = client.ClearDB(ctx, "default")
	fmt.Println("  已清理所有演示逻辑 DB")

	elapsed := time.Since(startTime)
	fmt.Printf("\n  总耗时: %v\n", elapsed.Round(time.Millisecond))

	section("pipeline-demo 执行完毕")
	fmt.Println("  数据管线: Config -> Schema -> Connector -> Normalizer -> Assembler -> GraphDB -> Neo4j")
	fmt.Println("  已验证: Ping / EnsureIndexes / BulkCreate / Query / LogicalDB / Upsert / BuildCypher / Snapshot")
	fmt.Println()
}

// toInt 从 Query 结果提取 int 值
func toInt(rows []map[string]any, key string) int {
	if len(rows) == 0 {
		return 0
	}
	v, _ := rows[0][key]
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}

