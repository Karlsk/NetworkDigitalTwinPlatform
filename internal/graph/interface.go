// Package graph 封装图数据库驱动层。
// GraphDB 接口定义所有图操作，包括全量同步、增量同步、查询和逻辑多 DB 管理。
// 驱动层只接收 GraphModel (assembler.Node/Relation)，不读 Schema。
package graph

import (
	"context"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
)

// GraphDB 图数据库驱动接口。
// 所有写操作和查询带 db 参数（逻辑多 DB），Cypher 生成内聚在实现中。
// Neo4j CE 不支持多 DB，通过 _db 属性实现逻辑隔离。
type GraphDB interface {
	// === 连接管理 ===

	// Ping 检查连接。
	Ping(ctx context.Context) error

	// Close 关闭连接。
	Close() error

	// === 全量同步 ===

	// BulkCreate 批量 CREATE（先 ClearDB 再 BulkCreate）。
	BulkCreate(ctx context.Context, db string, nodes []assembler.Node, rels []assembler.Relation) error

	// === 增量同步 ===

	// Upsert MERGE 节点 + SET += 属性增量合并 + MERGE 关系。
	Upsert(ctx context.Context, db string, nodes []assembler.Node, rels []assembler.Relation) error

	// DeleteRelations 仅删除指定关系。
	DeleteRelations(ctx context.Context, db string, rels []assembler.Relation) error

	// DeleteByURIs 按 URI 删除节点 + DETACH DELETE 关联关系。
	DeleteByURIs(ctx context.Context, db string, uris []string) error

	// === 查询 ===

	// Query 执行 Cypher 查询（驱动层自动注入 $_db）。
	Query(ctx context.Context, db string, cypher string, params map[string]any) ([]map[string]any, error)

	// BuildCypher 预览生成的 Cypher 语句（不执行），用于测试/audit/调试。
	// action: "create", "upsert", "delete", "delete_relations"
	BuildCypher(action string, db string, nodes []assembler.Node, rels []assembler.Relation, uris []string) (string, map[string]any)

	// === 逻辑 DB 管理 ===

	// ClearDB 清空指定逻辑 DB 的所有数据。
	ClearDB(ctx context.Context, db string) error

	// CloneDB 将一个逻辑 DB 完整复制到另一个。
	CloneDB(ctx context.Context, from, to string) error

	// ListDBs 列出所有逻辑 DB。
	ListDBs(ctx context.Context) ([]string, error)

	// HasDB 判断逻辑 DB 是否存在数据。
	HasDB(ctx context.Context, db string) (bool, error)

	// === 索引管理 ===

	// EnsureIndexes 创建 (_db, uri) 复合索引（幂等）。
	EnsureIndexes(ctx context.Context, labels []string) error
}
