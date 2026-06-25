// Package snapshot 实现快照管理
package snapshot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/graph"
)

// SnapshotMeta 快照元数据。
// 由 SnapshotManager.Create 产出，用于 List / Delete / Restore 操作。
type SnapshotMeta struct {
	Name      string    // 快照名称
	CreatedAt time.Time // 创建时间
	NodeCount int       // 节点数
	RelCount  int       // 关系数
	FilePath  string    // YAML 归档文件路径
}

// SnapshotDiff 快照对比结果。
// 由 SnapshotManager.Diff 产出，用于两个快照之间的差异分析。
type SnapshotDiff struct {
	AddedNodes   []assembler.Node     // 新增节点
	RemovedNodes []assembler.Node     // 删除节点
	AddedRels    []assembler.Relation // 新增关系
	RemovedRels  []assembler.Relation // 删除关系
}

// SnapshotManager 管理快照的创建、存储、恢复和清理。
// 通过 YAML 多文档文件实现快照归档，通过 Neo4j 逻辑 DB 实现活跃快照。
type SnapshotManager struct {
	graph      graph.GraphDB
	lock       *GraphLock
	snapDir    string
	maxActive  int
	lastAccess map[string]time.Time
	mu         sync.Mutex // 保护 lastAccess
}

// NewSnapshotManager 创建快照管理器。
func NewSnapshotManager(g graph.GraphDB, lock *GraphLock, snapDir string, maxActive int) *SnapshotManager {
	return &SnapshotManager{
		graph:      g,
		lock:       lock,
		snapDir:    snapDir,
		maxActive:  maxActive,
		lastAccess: make(map[string]time.Time),
	}
}

// Create 创建快照：从 default 逻辑 DB 导出数据并归档为 YAML 文件。
// 使用 RLock 允许并发读，阻塞写操作。
func (sm *SnapshotManager) Create(ctx context.Context, name string) (SnapshotMeta, error) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()

	// 分页读取 default DB 全部节点（SKIP/LIMIT 防止大数据集 OOM）
	const pageSize = 500
	var nodes []assembler.Node
	for skip := 0; ; skip += pageSize {
		rows, err := sm.graph.Query(ctx, "default",
			`MATCH (n) WHERE n._db = $_db `+
				`RETURN labels(n)[0] AS label, n.uri AS uri, properties(n) AS props `+
				`ORDER BY n.uri SKIP $skip LIMIT $limit`,
			map[string]any{"skip": skip, "limit": pageSize})
		if err != nil {
			return SnapshotMeta{}, fmt.Errorf("query nodes: %w", err)
		}
		for _, row := range rows {
			label, _ := row["label"].(string)
			uri, _ := row["uri"].(string)
			if label == "" || uri == "" {
				continue // 跳过不符合节点格式的行
			}
			nodes = append(nodes, assembler.Node{
				Label: label,
				URI:   uri,
				Props: extractProps(row["props"]),
			})
		}
		if len(rows) < pageSize {
			break
		}
	}

	// 分页读取 default DB 全部关系（SKIP/LIMIT 防止大数据集 OOM）
	var rels []assembler.Relation
	for skip := 0; ; skip += pageSize {
		rows, err := sm.graph.Query(ctx, "default",
			`MATCH (src)-[r]->(dst) WHERE src._db = $_db `+
				`RETURN type(r) AS type, src.uri AS from, dst.uri AS to, properties(r) AS props `+
				`ORDER BY src.uri SKIP $skip LIMIT $limit`,
			map[string]any{"skip": skip, "limit": pageSize})
		if err != nil {
			return SnapshotMeta{}, fmt.Errorf("query relations: %w", err)
		}
		for _, row := range rows {
			typ, _ := row["type"].(string)
			from, _ := row["from"].(string)
			to, _ := row["to"].(string)
			if typ == "" || from == "" || to == "" {
				continue // 跳过不符合关系格式的行
			}
			rels = append(rels, assembler.Relation{
				Type:  typ,
				From:  from,
				To:    to,
				Props: extractProps(row["props"]),
			})
		}
		if len(rows) < pageSize {
			break
		}
	}

	filePath := filepath.Join(sm.snapDir, name+".yaml")
	meta := SnapshotMeta{
		Name:      name,
		CreatedAt: time.Now(),
		NodeCount: len(nodes),
		RelCount:  len(rels),
		FilePath:  filePath,
	}

	if err := exportToYAML(filePath, meta, nodes, rels); err != nil {
		return SnapshotMeta{}, fmt.Errorf("export yaml: %w", err)
	}

	return meta, nil
}

// List 列出所有 YAML 归档快照的元数据。
func (sm *SnapshotManager) List(_ context.Context) ([]SnapshotMeta, error) {
	entries, err := os.ReadDir(sm.snapDir)
	if err != nil {
		return nil, fmt.Errorf("read snap dir: %w", err)
	}

	var metas []SnapshotMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		filePath := filepath.Join(sm.snapDir, e.Name())
		_, _, meta, err := importFromYAML(filePath)
		if err != nil {
			continue // 跳过损坏的快照文件
		}
		metas = append(metas, meta)
	}

	return metas, nil
}

// Delete 删除快照：清理 Neo4j 逻辑 DB，保留 YAML 归档文件。
func (sm *SnapshotManager) Delete(ctx context.Context, name string) error {
	exists, err := sm.graph.HasDB(ctx, name)
	if err != nil {
		return fmt.Errorf("has db: %w", err)
	}
	if exists {
		if err := sm.graph.ClearDB(ctx, name); err != nil {
			return fmt.Errorf("clear db: %w", err)
		}
	}
	return nil
}

// extractProps 从 Query 返回的 properties 字段提取 map[string]any。
func extractProps(v any) map[string]any {
	if v == nil {
		return nil
	}
	if props, ok := v.(map[string]any); ok {
		return props
	}
	return nil
}

// Restore 恢复快照到 default 逻辑 DB。
// 获取写锁，确保恢复期间无其他写操作。
func (sm *SnapshotManager) Restore(ctx context.Context, name string) error {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	if err := sm.EnsureLoaded(ctx, name); err != nil {
		return fmt.Errorf("ensure loaded: %w", err)
	}

	if err := sm.graph.ClearDB(ctx, "default"); err != nil {
		return fmt.Errorf("clear default: %w", err)
	}

	if err := sm.graph.CloneDB(ctx, name, "default"); err != nil {
		return fmt.Errorf("clone db: %w", err)
	}

	return nil
}

// EnsureLoaded 确保快照已加载到 Neo4j 逻辑 DB。
// 如果 DB 不存在，则从 YAML 归档导入。
func (sm *SnapshotManager) EnsureLoaded(ctx context.Context, name string) error {
	exists, err := sm.graph.HasDB(ctx, name)
	if err != nil {
		return fmt.Errorf("has db: %w", err)
	}

	sm.mu.Lock()
	sm.lastAccess[name] = time.Now()
	sm.mu.Unlock()

	if exists {
		return nil
	}

	// 从 YAML 导入
	filePath := filepath.Join(sm.snapDir, name+".yaml")
	nodes, rels, _, err := importFromYAML(filePath)
	if err != nil {
		return fmt.Errorf("import yaml: %w", err)
	}

	if err := sm.graph.BulkCreate(ctx, name, nodes, rels); err != nil {
		return fmt.Errorf("bulk create: %w", err)
	}

	sm.cleanup(ctx)
	return nil
}

// Diff 使用 Neo4j Cypher 差集查询对比两个快照。
// 返回 b 相对于 a 的新增和删除。
func (sm *SnapshotManager) Diff(ctx context.Context, a, b string) (*SnapshotDiff, error) {
	if err := sm.EnsureLoaded(ctx, a); err != nil {
		return nil, fmt.Errorf("ensure loaded a: %w", err)
	}
	if err := sm.EnsureLoaded(ctx, b); err != nil {
		return nil, fmt.Errorf("ensure loaded b: %w", err)
	}

	diff := &SnapshotDiff{}

	// b 中有而 a 中没有的节点（新增）
	addedNodeRows, err := sm.graph.Query(ctx, b,
		`MATCH (n) WHERE NOT EXISTS { MATCH (m {_db: $other, uri: n.uri}) } `+
			`RETURN labels(n)[0] AS label, n.uri AS uri, properties(n) AS props`,
		map[string]any{"other": a})
	if err != nil {
		return nil, fmt.Errorf("diff added nodes: %w", err)
	}
	for _, row := range addedNodeRows {
		label, _ := row["label"].(string)
		uri, _ := row["uri"].(string)
		if label != "" && uri != "" {
			diff.AddedNodes = append(diff.AddedNodes, assembler.Node{
				Label: label, URI: uri, Props: extractProps(row["props"]),
			})
		}
	}

	// a 中有而 b 中没有的节点（删除）
	removedNodeRows, err := sm.graph.Query(ctx, a,
		`MATCH (n) WHERE NOT EXISTS { MATCH (m {_db: $other, uri: n.uri}) } `+
			`RETURN labels(n)[0] AS label, n.uri AS uri, properties(n) AS props`,
		map[string]any{"other": b})
	if err != nil {
		return nil, fmt.Errorf("diff removed nodes: %w", err)
	}
	for _, row := range removedNodeRows {
		label, _ := row["label"].(string)
		uri, _ := row["uri"].(string)
		if label != "" && uri != "" {
			diff.RemovedNodes = append(diff.RemovedNodes, assembler.Node{
				Label: label, URI: uri, Props: extractProps(row["props"]),
			})
		}
	}

	// b 中有而 a 中没有的关系（新增）
	addedRelRows, err := sm.graph.Query(ctx, b,
		`MATCH (src)-[r]->(dst) WHERE NOT EXISTS { `+
			`MATCH (otherSrc {_db: $other, uri: src.uri})-[r2]->(otherDst {_db: $other, uri: dst.uri}) `+
			`WHERE type(r2) = type(r) } `+
			`RETURN type(r) AS type, src.uri AS from, dst.uri AS to, properties(r) AS props`,
		map[string]any{"other": a})
	if err != nil {
		return nil, fmt.Errorf("diff added rels: %w", err)
	}
	for _, row := range addedRelRows {
		typ, _ := row["type"].(string)
		from, _ := row["from"].(string)
		to, _ := row["to"].(string)
		if typ != "" && from != "" && to != "" {
			diff.AddedRels = append(diff.AddedRels, assembler.Relation{
				Type: typ, From: from, To: to, Props: extractProps(row["props"]),
			})
		}
	}

	// a 中有而 b 中没有的关系（删除）
	removedRelRows, err := sm.graph.Query(ctx, a,
		`MATCH (src)-[r]->(dst) WHERE NOT EXISTS { `+
			`MATCH (otherSrc {_db: $other, uri: src.uri})-[r2]->(otherDst {_db: $other, uri: dst.uri}) `+
			`WHERE type(r2) = type(r) } `+
			`RETURN type(r) AS type, src.uri AS from, dst.uri AS to, properties(r) AS props`,
		map[string]any{"other": b})
	if err != nil {
		return nil, fmt.Errorf("diff removed rels: %w", err)
	}
	for _, row := range removedRelRows {
		typ, _ := row["type"].(string)
		from, _ := row["from"].(string)
		to, _ := row["to"].(string)
		if typ != "" && from != "" && to != "" {
			diff.RemovedRels = append(diff.RemovedRels, assembler.Relation{
				Type: typ, From: from, To: to, Props: extractProps(row["props"]),
			})
		}
	}

	return diff, nil
}

// LocalDiff 通过 Go 内存对比两个 YAML 归档文件的差异，不需要 Neo4j。
// 返回 b 相对于 a 的新增和删除。
func (sm *SnapshotManager) LocalDiff(a, b string) (*SnapshotDiff, error) {
	aFile := filepath.Join(sm.snapDir, a+".yaml")
	bFile := filepath.Join(sm.snapDir, b+".yaml")

	aNodes, aRels, _, err := importFromYAML(aFile)
	if err != nil {
		return nil, fmt.Errorf("import a: %w", err)
	}
	bNodes, bRels, _, err := importFromYAML(bFile)
	if err != nil {
		return nil, fmt.Errorf("import b: %w", err)
	}

	diff := &SnapshotDiff{}

	// 节点 URI 差集
	aNodeURIs := make(map[string]assembler.Node, len(aNodes))
	for _, n := range aNodes {
		aNodeURIs[n.URI] = n
	}
	bNodeURIs := make(map[string]assembler.Node, len(bNodes))
	for _, n := range bNodes {
		bNodeURIs[n.URI] = n
	}
	for uri, n := range bNodeURIs {
		if _, ok := aNodeURIs[uri]; !ok {
			diff.AddedNodes = append(diff.AddedNodes, n)
		}
	}
	for uri, n := range aNodeURIs {
		if _, ok := bNodeURIs[uri]; !ok {
			diff.RemovedNodes = append(diff.RemovedNodes, n)
		}
	}

	// 关系差集（type+from+to 作为 key）
	relKey := func(r assembler.Relation) string {
		return r.Type + "|" + r.From + "|" + r.To
	}
	aRelKeys := make(map[string]assembler.Relation, len(aRels))
	for _, r := range aRels {
		aRelKeys[relKey(r)] = r
	}
	bRelKeys := make(map[string]assembler.Relation, len(bRels))
	for _, r := range bRels {
		bRelKeys[relKey(r)] = r
	}
	for key, r := range bRelKeys {
		if _, ok := aRelKeys[key]; !ok {
			diff.AddedRels = append(diff.AddedRels, r)
		}
	}
	for key, r := range aRelKeys {
		if _, ok := bRelKeys[key]; !ok {
			diff.RemovedRels = append(diff.RemovedRels, r)
		}
	}

	return diff, nil
}

// cleanup 按 LRU 策略清理超出 maxActive 的活跃快照 DB。
// "default" 永不清理。单个 ClearDB 失败不中断整体清理。
func (sm *SnapshotManager) cleanup(ctx context.Context) {
	dbs, err := sm.graph.ListDBs(ctx)
	if err != nil {
		return
	}

	// 过滤 "default"，只处理快照 DB
	var snapshotDBs []string
	for _, db := range dbs {
		if db != "default" {
			snapshotDBs = append(snapshotDBs, db)
		}
	}

	if len(snapshotDBs) <= sm.maxActive {
		return
	}

	// 按 lastAccess 排序（LRU 最旧在前）
	sm.mu.Lock()
	sort.Slice(snapshotDBs, func(i, j int) bool {
		ti := sm.lastAccess[snapshotDBs[i]]
		tj := sm.lastAccess[snapshotDBs[j]]
		if ti.Equal(tj) {
			return snapshotDBs[i] < snapshotDBs[j] // 二级键：DB 名字典序，确保确定性
		}
		return ti.Before(tj)
	})
	sm.mu.Unlock()

	// 清理最旧的 N 个
	toRemove := len(snapshotDBs) - sm.maxActive
	for i := 0; i < toRemove; i++ {
		db := snapshotDBs[i]
		if err := sm.graph.ClearDB(ctx, db); err != nil {
			slog.Error("cleanup: failed to clear db", "db", db, "error", err)
			continue // 容错：单个失败不中断
		}
		sm.mu.Lock()
		delete(sm.lastAccess, db)
		sm.mu.Unlock()
	}
}
