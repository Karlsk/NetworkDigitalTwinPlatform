// Package graph 封装图数据库驱动层。
// neo4j.go 实现 Neo4j 连接基础设施：构造函数、Ping、Close。
package graph

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/auth"
	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/config"
)

// driverFactory 是 driver 创建函数的抽象点，默认指向官方实现。
// 测试时替换为返回 mock driver 的函数，以实现无外部依赖的单元测试。
var driverFactory = neo4j.NewDriverWithContext

// session 抽象 Neo4j session 的核心操作。
// neo4j.SessionWithContext 通过 sessionWrapper 适配满足此接口。
// 定义此接口是因为 SessionWithContext 含未导出方法，无法在包外 mock。
type session interface {
	Run(ctx context.Context, cypher string, params map[string]any, configurers ...func(*neo4j.TransactionConfig)) (result, error)
	Close(ctx context.Context) error
}

// result 抽象 Neo4j 结果游标的核心操作。
// neo4j.ResultWithContext 满足此接口（鸭子类型）。
type result interface {
	Next(ctx context.Context) bool
	Record() *neo4j.Record
	Err() error
}

// sessionWrapper 将 neo4j.SessionWithContext 适配为内部 session 接口。
// 同时将 ResultWithContext 适配为 result 接口。
type sessionWrapper struct {
	inner neo4j.SessionWithContext
}

func (w *sessionWrapper) Run(ctx context.Context, cypher string, params map[string]any, configurers ...func(*neo4j.TransactionConfig)) (result, error) {
	r, err := w.inner.Run(ctx, cypher, params, configurers...)
	if err != nil {
		return nil, err
	}
	return r, nil // ResultWithContext 满足 result 接口
}

func (w *sessionWrapper) Close(ctx context.Context) error {
	return w.inner.Close(ctx)
}

// sessionFactory 是 session 创建函数的抽象点。
// 生产环境：通过 sessionWrapper 包装 driver.NewSession。
// 测试环境：替换为返回 mockSession 的函数。
var sessionFactory func(ctx context.Context, driver neo4j.DriverWithContext, cfg neo4j.SessionConfig) session = func(ctx context.Context, d neo4j.DriverWithContext, cfg neo4j.SessionConfig) session {
	return &sessionWrapper{inner: d.NewSession(ctx, cfg)}
}

// neo4jClient 实现 GraphDB 接口。
// 持有 Neo4j 驱动实例和默认逻辑 DB 名。
type neo4jClient struct {
	driver    neo4j.DriverWithContext
	defaultDB string
}

// NewNeo4jClient 根据配置创建 Neo4j 客户端。
// 注意：创建时不会实际建立网络连接，需调用 Ping 验证连通性。
func NewNeo4jClient(cfg config.Neo4JConfig) (GraphDB, error) {
	driver, err := driverFactory(
		cfg.URI,
		neo4j.BasicAuth(cfg.User, cfg.Password, ""),
	)
	if err != nil {
		return nil, fmt.Errorf("create neo4j driver: %w", err)
	}

	slog.Info("neo4j client created", "uri", cfg.URI, "defaultDB", cfg.DefaultDB)

	return &neo4jClient{
		driver:    driver,
		defaultDB: cfg.DefaultDB,
	}, nil
}

// Ping 验证与 Neo4j 的连接连通性。
// 实际建立网络连接并确认服务端可达。
func (c *neo4jClient) Ping(ctx context.Context) error {
	if err := c.driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j ping: %w", err)
	}
	return nil
}

// Close 关闭 Neo4j 驱动及其所有底层连接。
// 使用 context.Background() 确保关闭操作不被外部 context 取消。
func (c *neo4jClient) Close() error {
	if err := c.driver.Close(context.Background()); err != nil {
		return fmt.Errorf("neo4j close: %w", err)
	}
	slog.Info("neo4j client closed")
	return nil
}

// Query 执行 Cypher 查询（驱动层自动注入 $_db）。
// 创建 params 的副本并注入 _db，避免修改调用方的原始 map。
func (c *neo4jClient) Query(ctx context.Context, db string, cypher string, params map[string]any) ([]map[string]any, error) {
	merged := make(map[string]any, len(params)+1)
	for k, v := range params {
		merged[k] = v
	}
	merged["_db"] = db

	sess := sessionFactory(ctx, c.driver, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	res, err := sess.Run(ctx, cypher, merged)
	if err != nil {
		return nil, fmt.Errorf("run cypher: %w", err)
	}

	var records []map[string]any
	for res.Next(ctx) {
		record := res.Record()
		row := make(map[string]any, len(record.Keys))
		for _, key := range record.Keys {
			val, _ := record.Get(key)
			row[key] = val
		}
		records = append(records, row)
	}
	if err := res.Err(); err != nil {
		return records, fmt.Errorf("iterate result: %w", err)
	}
	return records, nil
}

// ClearDB 清空指定逻辑 DB 的所有节点和关联关系。
func (c *neo4jClient) ClearDB(ctx context.Context, db string) error {
	sess := sessionFactory(ctx, c.driver, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.Run(ctx, "MATCH (n {_db: $_db}) DETACH DELETE n", map[string]any{"_db": db})
	if err != nil {
		return fmt.Errorf("clear db %q: %w", db, err)
	}
	return nil
}

// ListDBs 列出所有逻辑 DB（通过扫描所有节点的 _db 属性去重）。
func (c *neo4jClient) ListDBs(ctx context.Context) ([]string, error) {
	sess := sessionFactory(ctx, c.driver, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	res, err := sess.Run(ctx, "MATCH (n) WHERE n._db IS NOT NULL RETURN DISTINCT n._db AS db", nil)
	if err != nil {
		return nil, fmt.Errorf("list dbs: %w", err)
	}

	var dbs []string
	for res.Next(ctx) {
		if val, ok := res.Record().Get("db"); ok {
			if s, ok := val.(string); ok {
				dbs = append(dbs, s)
			}
		}
	}
	if err := res.Err(); err != nil {
		return dbs, fmt.Errorf("iterate list dbs: %w", err)
	}
	return dbs, nil
}

// HasDB 判断指定逻辑 DB 是否存在数据。
// 使用 count(n) > 0 避免全量扫描，配合 (_db, uri) 复合索引高效查询。
func (c *neo4jClient) HasDB(ctx context.Context, db string) (bool, error) {
	sess := sessionFactory(ctx, c.driver, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	res, err := sess.Run(ctx, "MATCH (n {_db: $_db}) RETURN count(n) > 0 AS exists", map[string]any{"_db": db})
	if err != nil {
		return false, fmt.Errorf("has db %q: %w", db, err)
	}

	if res.Next(ctx) {
		exists, _ := res.Record().Get("exists")
		if b, ok := exists.(bool); ok {
			return b, nil
		}
	}
	if err := res.Err(); err != nil {
		return false, fmt.Errorf("check has db %q: %w", db, err)
	}
	return false, nil
}

// 确保 driverFactory 类型签名与 neo4j.NewDriverWithContext 一致。
// 这是编译时类型安全的文档性注释。
var _ func(string, auth.TokenManager, ...func(*neo4j.Config)) (neo4j.DriverWithContext, error) = driverFactory //nolint:staticcheck // neo4j.Config 将在 6.0 废弃

// groupNodesByLabels 按 MostSpecificLabel 分组节点。
// 用于 BulkCreate/Upsert 将相同最具体 Label 的节点合并到同一条 UNWIND Cypher 中。
func groupNodesByLabels(nodes []assembler.Node) map[string][]assembler.Node {
	groups := make(map[string][]assembler.Node)
	for _, n := range nodes {
		groups[n.MostSpecificLabel()] = append(groups[n.MostSpecificLabel()], n)
	}
	return groups
}

// joinLabels 将标签列表拼接为 Cypher 多标签格式。
// 例如 ["Resource", "Device"] -> ":Resource:Device"
func joinLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	return ":" + strings.Join(labels, ":")
}

// groupRelsByType 按关系类型分组。
// 用于 BulkCreate 将相同 RelType 的关系合并到同一条 UNWIND Cypher 中。
func groupRelsByType(rels []assembler.Relation) map[string][]assembler.Relation {
	groups := make(map[string][]assembler.Relation)
	for _, r := range rels {
		groups[r.Type] = append(groups[r.Type], r)
	}
	return groups
}

// BulkCreate 批量 CREATE（全量同步）。
// 按 Label 分组节点执行 UNWIND + CREATE，按 RelType 分组关系执行 UNWIND + MATCH + CREATE。
// 每个节点的 Props 会被复制后注入 _db 和 uri，不修改调用方的原始数据。
// 调用前必须 ClearDB，否则会导致重复数据（CREATE 不检查幂等性）。
func (c *neo4jClient) BulkCreate(ctx context.Context, db string, nodes []assembler.Node, rels []assembler.Relation) error {
	sess := sessionFactory(ctx, c.driver, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// 按 MostSpecificLabel 分组批量创建节点
	for label, group := range groupNodesByLabels(nodes) {
		nodeData := make([]map[string]any, 0, len(group))
		for _, n := range group {
			// 复制 Props 避免修改调用方原始 map
			props := make(map[string]any, len(n.Props)+2)
			for k, v := range n.Props {
				props[k] = v
			}
			props["_db"] = db
			props["uri"] = n.URI
			nodeData = append(nodeData, props)
		}

		params := map[string]any{
			"_db":   db,
			"nodes": nodeData,
		}
		labelStr := joinLabels(group[0].Labels)
		cypher := fmt.Sprintf(
			"UNWIND $nodes AS n CREATE (x%s {_db: $_db, uri: n.uri}) SET x += n",
			labelStr,
		)
		if _, err := sess.Run(ctx, cypher, params); err != nil {
			return fmt.Errorf("bulk create nodes %s: %w", label, err)
		}
	}

	// 按 RelType 分组批量创建关系
	for relType, group := range groupRelsByType(rels) {
		relData := make([]map[string]any, 0, len(group))
		for _, r := range group {
			relData = append(relData, map[string]any{
				"from": r.From,
				"to":   r.To,
			})
		}

		params := map[string]any{
			"_db":  db,
			"rels": relData,
		}
		cypher := fmt.Sprintf(
			"UNWIND $rels AS r MATCH (a {_db: $_db, uri: r.from}) MATCH (b {_db: $_db, uri: r.to}) CREATE (a)-[:%s]->(b)",
			relType,
		)
		if _, err := sess.Run(ctx, cypher, params); err != nil {
			return fmt.Errorf("bulk create rels %s: %w", relType, err)
		}
	}

	slog.Info("bulk create completed", "db", db, "node_labels", len(groupNodesByLabels(nodes)), "rel_types", len(groupRelsByType(rels)))
	return nil
}

// Upsert 增量 MERGE 节点 + SET += 属性合并 + MERGE 关系。
// 节点使用 MERGE 匹配 (_db, uri)，SET += 增量合并属性（新属性添加，旧属性保留，已传属性更新）。
// 关系使用 MERGE 幂等创建（已存在则跳过，不存在则创建）。
// 先 Upsert 所有节点（确保目标节点存在），再 MERGE 关系。
func (c *neo4jClient) Upsert(ctx context.Context, db string, nodes []assembler.Node, rels []assembler.Relation) error {
	sess := sessionFactory(ctx, c.driver, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// 按 MostSpecificLabel 分组增量 Upsert 节点
	for label, group := range groupNodesByLabels(nodes) {
		nodeData := make([]map[string]any, 0, len(group))
		for _, n := range group {
			// 复制 Props 避免修改调用方原始 map
			props := make(map[string]any, len(n.Props)+1)
			for k, v := range n.Props {
				props[k] = v
			}
			props["_db"] = db
			// 注意：props 不含 uri（MERGE 匹配键已设置）
			nodeData = append(nodeData, map[string]any{
				"uri":   n.URI,
				"props": props,
			})
		}

		params := map[string]any{
			"_db":   db,
			"nodes": nodeData,
		}
		// MERGE 只使用最具体 Label（Neo4j 不支持多 Label MERGE）
		// 多标签时，ON CREATE SET 补充父 Label
		var cypher string
		if len(group[0].Labels) > 1 {
			parentLabels := joinLabels(group[0].Labels[:len(group[0].Labels)-1])
			cypher = fmt.Sprintf(
				"UNWIND $nodes AS n MERGE (x:%s {_db: $_db, uri: n.uri}) ON CREATE SET x%s SET x += n.props",
				label, parentLabels,
			)
		} else {
			cypher = fmt.Sprintf(
				"UNWIND $nodes AS n MERGE (x:%s {_db: $_db, uri: n.uri}) SET x += n.props",
				label,
			)
		}
		if _, err := sess.Run(ctx, cypher, params); err != nil {
			return fmt.Errorf("upsert nodes %s: %w", label, err)
		}
	}

	// 按 RelType 分组幂等 MERGE 关系
	for relType, group := range groupRelsByType(rels) {
		relData := make([]map[string]any, 0, len(group))
		for _, r := range group {
			relData = append(relData, map[string]any{
				"from": r.From,
				"to":   r.To,
			})
		}

		params := map[string]any{
			"_db":  db,
			"rels": relData,
		}
		cypher := fmt.Sprintf(
			"UNWIND $rels AS r MATCH (a {_db: $_db, uri: r.from}) MATCH (b {_db: $_db, uri: r.to}) MERGE (a)-[:%s]->(b)",
			relType,
		)
		if _, err := sess.Run(ctx, cypher, params); err != nil {
			return fmt.Errorf("upsert rels %s: %w", relType, err)
		}
	}

	slog.Info("upsert completed", "db", db, "node_labels", len(groupNodesByLabels(nodes)), "rel_types", len(groupRelsByType(rels)))
	return nil
}

// DeleteByURIs 按 URI 删除节点及其关联关系（DETACH DELETE）。
// DETACH DELETE 自动删除节点的所有入边和出边，避免留下悬空关系。
// 对不存在的 URI 不报错（MATCH 不到则跳过）。
func (c *neo4jClient) DeleteByURIs(ctx context.Context, db string, uris []string) error {
	sess := sessionFactory(ctx, c.driver, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	params := map[string]any{"_db": db, "uris": uris}
	cypher := "UNWIND $uris AS uri MATCH (n {_db: $_db, uri: uri}) DETACH DELETE n"

	if _, err := sess.Run(ctx, cypher, params); err != nil {
		return fmt.Errorf("delete by uris: %w", err)
	}

	slog.Info("delete by uris completed", "db", db, "count", len(uris))
	return nil
}

// DeleteRelations 仅删除指定关系而不影响节点。
// 按 RelType 分组执行 MATCH + DELETE，精确匹配 (a)-[x:Type]->(b) 后只删除关系 x。
// 对 MATCH 不到的关系不报错（跳过）。
func (c *neo4jClient) DeleteRelations(ctx context.Context, db string, rels []assembler.Relation) error {
	sess := sessionFactory(ctx, c.driver, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	for relType, group := range groupRelsByType(rels) {
		relData := make([]map[string]any, 0, len(group))
		for _, r := range group {
			relData = append(relData, map[string]any{
				"from": r.From,
				"to":   r.To,
			})
		}

		params := map[string]any{
			"_db":  db,
			"rels": relData,
		}
		cypher := fmt.Sprintf(
			"UNWIND $rels AS r MATCH (a {_db: $_db, uri: r.from})-[x:%s]->(b {_db: $_db, uri: r.to}) DELETE x",
			relType,
		)
		if _, err := sess.Run(ctx, cypher, params); err != nil {
			return fmt.Errorf("delete rels %s: %w", relType, err)
		}
	}

	slog.Info("delete relations completed", "db", db, "rel_types", len(groupRelsByType(rels)))
	return nil
}

// groupCloneNodesByLabels 将 Query 返回的节点记录按完整 labels 列表分组。
// 多标签节点使用 ":Label1:Label2" 拼接形式作为分组键。
func groupCloneNodesByLabels(nodes []map[string]any) map[string][]map[string]any {
	groups := make(map[string][]map[string]any)
	for _, n := range nodes {
		// 尝试 []string
		labels, ok := n["labels"].([]string)
		if !ok || len(labels) == 0 {
			// 尝试 []any（Neo4j 驱动可能返回此类型）
			if anyLabels, ok := n["labels"].([]any); ok && len(anyLabels) > 0 {
				var strLabels []string
				for _, l := range anyLabels {
					if s, ok := l.(string); ok {
						strLabels = append(strLabels, s)
					}
				}
				if len(strLabels) > 0 {
					key := strings.Join(strLabels, ":")
					n["_labels"] = strLabels
					groups[key] = append(groups[key], n)
				}
			}
			continue
		}
		key := strings.Join(labels, ":")
		n["_labels"] = labels
		groups[key] = append(groups[key], n)
	}
	return groups
}

// groupCloneRelsByType 将 Query 返回的关系记录按 type 分组。
func groupCloneRelsByType(rels []map[string]any) map[string][]map[string]any {
	groups := make(map[string][]map[string]any)
	for _, r := range rels {
		relType, _ := r["type"].(string)
		if relType == "" {
			continue
		}
		groups[relType] = append(groups[relType], r)
	}
	return groups
}

// CloneDB 将一个逻辑 DB 完整复制到另一个（用于快照恢复）。
// 两阶段实现：
//  1. 读阶段：通过 Query 读取源 DB 的所有节点和关系
//  2. 写阶段：按 Label/RelType 分组写入目标 DB
//
// 调用方负责先 ClearDB 目标 DB，本方法不做清理。
func (c *neo4jClient) CloneDB(ctx context.Context, from, to string) error {
	// === 阶段 1: 读源 DB ===
	nodeRecords, err := c.Query(ctx, from,
		"MATCH (n {_db: $_db}) RETURN labels(n) AS labels, n.uri AS uri, properties(n) AS props",
		nil,
	)
	if err != nil {
		return fmt.Errorf("clone db query nodes: %w", err)
	}

	relRecords, err := c.Query(ctx, from,
		"MATCH (a {_db: $_db})-[r]->(b {_db: $_db}) RETURN type(r) AS type, a.uri AS from, b.uri AS to",
		nil,
	)
	if err != nil {
		return fmt.Errorf("clone db query rels: %w", err)
	}

	// === 阶段 2: 写入目标 DB ===
	sess := sessionFactory(ctx, c.driver, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// 按 labels 分组写入节点（支持多 Label）
	nodeGroups := groupCloneNodesByLabels(nodeRecords)
	for key, group := range nodeGroups {
		nodeData := make([]map[string]any, 0, len(group))
		for _, n := range group {
			uri, _ := n["uri"].(string)
			props, _ := n["props"].(map[string]any)
			if props == nil {
				props = make(map[string]any)
			}
			// 复制 props 并覆盖 _db 为目标 DB
			newProps := make(map[string]any, len(props)+1)
			for k, v := range props {
				newProps[k] = v
			}
			newProps["_db"] = to
			nodeData = append(nodeData, map[string]any{
				"uri":   uri,
				"props": newProps,
			})
		}

		// 使用 _labels 构建多 Label CREATE
		labels, _ := group[0]["_labels"].([]string)
		labelStr := joinLabels(labels)
		params := map[string]any{"to": to, "nodes": nodeData}
		cypher := fmt.Sprintf(
			"UNWIND $nodes AS n CREATE (x%s {_db: $to, uri: n.uri}) SET x += n.props",
			labelStr,
		)
		if _, err := sess.Run(ctx, cypher, params); err != nil {
			return fmt.Errorf("clone db create nodes %s: %w", key, err)
		}
	}

	// 按 RelType 分组写入关系
	relGroups := groupCloneRelsByType(relRecords)
	for relType, group := range relGroups {
		relData := make([]map[string]any, 0, len(group))
		for _, r := range group {
			from, _ := r["from"].(string)
			toURI, _ := r["to"].(string)
			relData = append(relData, map[string]any{
				"from": from,
				"to":   toURI,
			})
		}

		params := map[string]any{"to": to, "rels": relData}
		cypher := fmt.Sprintf(
			"UNWIND $rels AS r MATCH (a {_db: $to, uri: r.from}) MATCH (b {_db: $to, uri: r.to}) CREATE (a)-[:%s]->(b)",
			relType,
		)
		if _, err := sess.Run(ctx, cypher, params); err != nil {
			return fmt.Errorf("clone db create rels %s: %w", relType, err)
		}
	}

	slog.Info("clone db completed", "from", from, "to", to,
		"nodes", len(nodeRecords), "rels", len(relRecords))
	return nil
}

// BuildCypher 预览生成的 Cypher 语句（不执行），用于测试/audit/调试。
// 纯函数：不创建 session，不访问数据库，只返回 Cypher 字符串 + params。
// action 支持: "create", "upsert", "delete", "delete_relations"。
// 多 Label/RelType 时返回多条 Cypher 用 ";\n" 分隔，仅供预览。
// params key 使用 nodes_{Label} / rels_{Type} 格式，避免多 Label 时 key 冲突。
func (c *neo4jClient) BuildCypher(action string, db string, nodes []assembler.Node, rels []assembler.Relation, uris []string) (string, map[string]any) {
	params := map[string]any{"_db": db}

	switch action {
	case "create":
		nodeGroups := groupNodesByLabels(nodes)
		var cyphers []string
		for label, group := range nodeGroups {
			nodeData := make([]map[string]any, 0, len(group))
			for _, n := range group {
				nodeData = append(nodeData, map[string]any{"uri": n.URI, "props": n.Props})
			}
			params["nodes_"+label] = nodeData
			labelStr := joinLabels(group[0].Labels)
			cyphers = append(cyphers, fmt.Sprintf(
				"UNWIND $nodes_%s AS n CREATE (x%s {_db: $_db, uri: n.uri}) SET x += n.props",
				label, labelStr,
			))
		}
		return strings.Join(cyphers, ";\n"), params

	case "upsert":
		nodeGroups := groupNodesByLabels(nodes)
		var cyphers []string
		for label, group := range nodeGroups {
			nodeData := make([]map[string]any, 0, len(group))
			for _, n := range group {
				nodeData = append(nodeData, map[string]any{"uri": n.URI, "props": n.Props})
			}
			params["nodes_"+label] = nodeData
			if len(group[0].Labels) > 1 {
				parentLabels := joinLabels(group[0].Labels[:len(group[0].Labels)-1])
				cyphers = append(cyphers, fmt.Sprintf(
					"UNWIND $nodes_%s AS n MERGE (x:%s {_db: $_db, uri: n.uri}) ON CREATE SET x%s SET x += n.props",
					label, label, parentLabels,
				))
			} else {
				cyphers = append(cyphers, fmt.Sprintf(
					"UNWIND $nodes_%s AS n MERGE (x:%s {_db: $_db, uri: n.uri}) SET x += n.props",
					label, label,
				))
			}
		}
		return strings.Join(cyphers, ";\n"), params

	case "delete":
		params["uris"] = uris
		return "UNWIND $uris AS uri MATCH (n {_db: $_db, uri: uri}) DETACH DELETE n", params

	case "delete_relations":
		relGroups := groupRelsByType(rels)
		var cyphers []string
		for relType, group := range relGroups {
			relData := make([]map[string]any, 0, len(group))
			for _, r := range group {
				relData = append(relData, map[string]any{"from": r.From, "to": r.To})
			}
			params["rels_"+relType] = relData
			cyphers = append(cyphers, fmt.Sprintf(
				"UNWIND $rels_%s AS r MATCH (a {_db: $_db, uri: r.from})-[x:%s]->(b {_db: $_db, uri: r.to}) DELETE x",
				relType, relType,
			))
		}
		return strings.Join(cyphers, ";\n"), params

	default:
		return "", params
	}
}

// EnsureIndexes 系统启动时创建 (_db, uri) 复合索引。
// 每个 EntityType 对应一个索引，确保逻辑 DB 内 URI 查找高效。
// 使用 CREATE INDEX ... IF NOT EXISTS 保证幂等（重复调用不报错）。
// 索引名格式: idx_{label}_db_uri（如 idx_device_db_uri）。
func (c *neo4jClient) EnsureIndexes(ctx context.Context, labels []string) error {
	if len(labels) == 0 {
		return nil
	}

	sess := sessionFactory(ctx, c.driver, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	for _, label := range labels {
		indexName := "idx_" + strings.ToLower(label) + "_db_uri"
		cypher := fmt.Sprintf(
			"CREATE INDEX %s IF NOT EXISTS FOR (n:%s) ON (n._db, n.uri)",
			indexName, label,
		)
		if _, err := sess.Run(ctx, cypher, nil); err != nil {
			return fmt.Errorf("ensure indexes %s: %w", indexName, err)
		}
	}

	slog.Info("ensure indexes completed", "count", len(labels))
	return nil
}
