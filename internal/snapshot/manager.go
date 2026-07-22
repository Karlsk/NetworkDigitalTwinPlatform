// Package snapshot 实现快照管理
package snapshot

import (
	"context"
	"errors"
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
	"gitlab.com/pml/network-digital-twin/internal/observability"
	"gitlab.com/pml/network-digital-twin/internal/repository"
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
	ChangedNodes []NodeChange         // 属性变更节点
	ChangedRels  []RelChange          // 属性变更关系
}

// NodeChange 节点属性级变更。
type NodeChange struct {
	URI            string                 // 节点 URI
	Label          string                 // 节点标签（MostSpecificLabel）
	AddedFields    map[string]any         // 新增的属性 (b 有 a 无)
	RemovedFields  map[string]any         // 删除的属性 (a 有 b 无)
	ModifiedFields map[string]FieldChange // 修改的属性 (a 和 b 都有但值不同)
}

// FieldChange 单个字段的变更详情。
type FieldChange struct {
	OldValue any // 旧值 (快照 a)
	NewValue any // 新值 (快照 b)
}

// RelChange 关系属性级变更。
type RelChange struct {
	Type           string                 // 关系类型
	From           string                 // 源节点 URI
	To             string                 // 目标节点 URI
	AddedFields    map[string]any         // 新增的属性
	RemovedFields  map[string]any         // 删除的属性
	ModifiedFields map[string]FieldChange // 修改的属性
}

// SnapshotManager 管理快照的创建、存储、恢复和清理。
// 通过 YAML 多文档文件实现快照归档，通过 Neo4j 逻辑 DB 实现活跃快照。
type SnapshotManager struct {
	graph         graph.GraphDB
	lock          *GraphLock
	snapDir       string
	maxActive     int
	lastAccess    map[string]time.Time
	mu            sync.Mutex                    // 保护 lastAccess
	metaCache     map[string]SnapshotMeta       // 新增: name → meta 缓存
	cacheMu       sync.RWMutex                  // 新增: 缓存读写锁
	cacheReady    bool                          // 新增: 缓存是否已预热
	auditLog      *AuditLog                     // V1-18: 审计日志
	retentionDays int                           // V1-20: TTL 保留天数，0 = 不自动清理
	repo          repository.SnapshotRepository // V2-06: 元数据持久化（nil 时走 metaCache）
}

// Option SnapshotManager 可选依赖注入（V2-06）。
type Option func(*SnapshotManager)

// WithSnapshotRepository 注入 SnapshotRepository，启用后元数据优先写入 PostgreSQL。
func WithSnapshotRepository(repo repository.SnapshotRepository) Option {
	return func(sm *SnapshotManager) {
		sm.repo = repo
	}
}

// WithAuditRepository 注入 AuditLogRepository，启用后审计日志异步写入 PostgreSQL。
// V2-09: 双写模式，内存 FIFO + PG 异步持久化。
func WithAuditRepository(repo repository.AuditLogRepository) Option {
	return func(sm *SnapshotManager) {
		sm.auditLog.SetRepository(repo)
	}
}

// NewSnapshotManager 创建快照管理器。
// opts 可通过 WithSnapshotRepository 注入 PostgreSQL 元数据仓库（V2-06）。
func NewSnapshotManager(g graph.GraphDB, lock *GraphLock, snapDir string, maxActive int, opts ...Option) *SnapshotManager {
	sm := &SnapshotManager{
		graph:      g,
		lock:       lock,
		snapDir:    snapDir,
		maxActive:  maxActive,
		lastAccess: make(map[string]time.Time),
		metaCache:  make(map[string]SnapshotMeta),
		auditLog:   NewAuditLog(1000), // V1-18: 默认保留 1000 条审计记录
	}
	for _, opt := range opts {
		opt(sm)
	}
	return sm
}

// AuditLog 返回审计日志实例，供外部（如 MCP/Service）查询审计记录。
func (sm *SnapshotManager) AuditLog() *AuditLog {
	return sm.auditLog
}

// SetRetentionDays 设置快照 TTL 保留天数。days <= 0 表示不自动清理。
func (sm *SnapshotManager) SetRetentionDays(days int) {
	sm.retentionDays = days
}

// cleanupExpired 按 TTL 策略清理超期快照。retentionDays <= 0 时不执行任何操作。
func (sm *SnapshotManager) cleanupExpired(ctx context.Context) {
	if sm.retentionDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -sm.retentionDays)

	sm.cacheMu.RLock()
	var expired []string
	for name, meta := range sm.metaCache {
		if meta.CreatedAt.Before(cutoff) {
			expired = append(expired, name)
		}
	}
	sm.cacheMu.RUnlock()

	for _, name := range expired {
		if err := sm.Delete(ctx, name); err != nil {
			slog.Warn("cleanup expired snapshot failed", "snapshot", name, "error", err)
		} else {
			slog.Info("cleaned up expired snapshot", "snapshot", name)
			sm.auditLog.Record(AuditEntry{ //nolint:contextcheck // 审计记录无需 context
				Action:   "auto_delete",
				Snapshot: name,
				Actor:    "system",
				Detail:   "expired TTL cleanup",
			})
		}
	}
}

// Create 创建快照：从 default 逻辑 DB 导出数据并归档为 YAML 文件。
// 使用 RLock 允许并发读，阻塞写操作。
func (sm *SnapshotManager) Create(ctx context.Context, name string) (meta SnapshotMeta, err error) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()

	// V1-18: 审计日志 — 无论成功失败都记录
	defer func() { //nolint:contextcheck // defer 闭包中无法传递 ctx
		sm.auditLog.Record(AuditEntry{
			Action:   "create",
			Snapshot: name,
			Actor:    "system",
			Detail:   fmt.Sprintf("nodes=%d, rels=%d", meta.NodeCount, meta.RelCount),
			Error:    errStr(err),
		})
		// V2-15: Prometheus 指标上报
		if err != nil {
			observability.SnapshotOperationsTotal.WithLabelValues("create", "error").Inc()
		} else {
			observability.SnapshotOperationsTotal.WithLabelValues("create", "success").Inc()
			observability.SnapshotCount.Inc()
		}
	}()

	// 分页读取 default DB 全部节点（SKIP/LIMIT 防止大数据集 OOM）
	const pageSize = 500
	var nodes []assembler.Node
	for skip := 0; ; skip += pageSize {
		rows, queryErr := sm.graph.Query(ctx, "default",
			`MATCH (n) WHERE n._db = $_db `+
				`RETURN labels(n) AS labels, n.uri AS uri, properties(n) AS props `+
				`ORDER BY n.uri SKIP $skip LIMIT $limit`,
			map[string]any{"skip": skip, "limit": pageSize})
		if queryErr != nil {
			return SnapshotMeta{}, fmt.Errorf("query nodes: %w", queryErr)
		}
		for _, row := range rows {
			labels := anyToStringSlice(row["labels"])
			uri, _ := row["uri"].(string)
			if len(labels) == 0 || uri == "" {
				continue // 跳过不符合节点格式的行
			}
			nodes = append(nodes, assembler.Node{
				Labels: labels,
				URI:    uri,
				Props:  extractProps(row["props"]),
			})
		}
		if len(rows) < pageSize {
			break
		}
	}

	// 分页读取 default DB 全部关系（SKIP/LIMIT 防止大数据集 OOM）
	var rels []assembler.Relation
	for skip := 0; ; skip += pageSize {
		rows, queryErr := sm.graph.Query(ctx, "default",
			`MATCH (src)-[r]->(dst) WHERE src._db = $_db `+
				`RETURN type(r) AS type, src.uri AS from, dst.uri AS to, properties(r) AS props `+
				`ORDER BY src.uri SKIP $skip LIMIT $limit`,
			map[string]any{"skip": skip, "limit": pageSize})
		if queryErr != nil {
			return SnapshotMeta{}, fmt.Errorf("query relations: %w", queryErr)
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
	meta = SnapshotMeta{
		Name:      name,
		CreatedAt: time.Now(),
		NodeCount: len(nodes),
		RelCount:  len(rels),
		FilePath:  filePath,
	}

	if exportErr := exportToYAML(filePath, meta, nodes, rels); exportErr != nil {
		return SnapshotMeta{}, fmt.Errorf("export yaml: %w", exportErr)
	}

	// 写入缓存
	sm.cacheMu.Lock()
	sm.metaCache[meta.Name] = meta
	sm.cacheReady = true
	sm.cacheMu.Unlock()

	// V2-06: 同步写入 Repository
	if sm.repo != nil {
		if repoErr := sm.repo.Create(ctx, &repository.SnapshotRecord{
			Name:      meta.Name,
			CreatedAt: meta.CreatedAt,
			NodeCount: meta.NodeCount,
			RelCount:  meta.RelCount,
			FilePath:  meta.FilePath,
			Status:    "active",
		}); repoErr != nil {
			slog.Warn("repo create failed, metaCache only", "snapshot", meta.Name, "error", repoErr)
		}
	}

	// V1-20: Create 后触发 TTL 过期清理
	sm.cleanupExpired(ctx)

	return meta, nil
}

// List 列出所有 YAML 归档快照的元数据。
// V2-06: 优先从 Repository 读取；失败或 nil 时降级到 metaCache/warmCache。
func (sm *SnapshotManager) List(ctx context.Context) ([]SnapshotMeta, error) {
	// V2-06: 优先从 Repository 读取
	if sm.repo != nil {
		records, err := sm.repo.List(ctx)
		if err == nil {
			metas := make([]SnapshotMeta, 0, len(records))
			for _, r := range records {
				metas = append(metas, SnapshotMeta{
					Name:      r.Name,
					CreatedAt: r.CreatedAt,
					NodeCount: r.NodeCount,
					RelCount:  r.RelCount,
					FilePath:  r.FilePath,
				})
			}
			return metas, nil
		}
		slog.Warn("repo list failed, falling back to cache", "error", err)
	}

	// Fallback: metaCache
	sm.cacheMu.RLock()
	if sm.cacheReady {
		result := make([]SnapshotMeta, 0, len(sm.metaCache))
		for _, meta := range sm.metaCache {
			result = append(result, meta)
		}
		sm.cacheMu.RUnlock()
		return result, nil
	}
	sm.cacheMu.RUnlock()

	// 缓存未预热: 全量 warm cache
	return sm.warmCache(ctx)
}

// warmCache 扫描快照目录，只解析 meta 文档头，填充 metaCache。
func (sm *SnapshotManager) warmCache(_ context.Context) ([]SnapshotMeta, error) {
	sm.cacheMu.Lock()
	defer sm.cacheMu.Unlock()

	// 双重检查: 防止并发 warmCache 重复执行
	if sm.cacheReady {
		result := make([]SnapshotMeta, 0, len(sm.metaCache))
		for _, meta := range sm.metaCache {
			result = append(result, meta)
		}
		return result, nil
	}

	entries, err := os.ReadDir(sm.snapDir)
	if err != nil {
		return nil, fmt.Errorf("read snap dir: %w", err)
	}

	var result []SnapshotMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		filePath := filepath.Join(sm.snapDir, e.Name())
		meta, err := importMetaOnly(filePath)
		if err != nil {
			continue // 跳过损坏的快照文件
		}
		sm.metaCache[meta.Name] = meta
		result = append(result, meta)
	}

	sm.cacheReady = true
	return result, nil
}

// Delete 删除快照：清理 Neo4j 逻辑 DB，保留 YAML 归档文件。
func (sm *SnapshotManager) Delete(ctx context.Context, name string) (err error) {
	// V1-18: 审计日志 — 无论成功失败都记录
	defer func() { //nolint:contextcheck // defer 闭包中无法传递 ctx
		sm.auditLog.Record(AuditEntry{
			Action:   "delete",
			Snapshot: name,
			Actor:    "system",
			Detail:   "delete snapshot",
			Error:    errStr(err),
		})
		// V2-15: Prometheus 指标上报
		if err != nil {
			observability.SnapshotOperationsTotal.WithLabelValues("delete", "error").Inc()
		} else {
			observability.SnapshotOperationsTotal.WithLabelValues("delete", "success").Inc()
			observability.SnapshotCount.Dec()
		}
	}()

	exists, hasErr := sm.graph.HasDB(ctx, name)
	if hasErr != nil {
		return fmt.Errorf("has db: %w", hasErr)
	}
	if exists {
		if clearErr := sm.graph.ClearDB(ctx, name); clearErr != nil {
			return fmt.Errorf("clear db: %w", clearErr)
		}
	}

	// 从缓存删除
	sm.cacheMu.Lock()
	delete(sm.metaCache, name)
	sm.cacheMu.Unlock()

	// V2-06: 同步从 Repository 删除
	if sm.repo != nil {
		if repoErr := sm.repo.Delete(ctx, name); repoErr != nil && !errors.Is(repoErr, repository.ErrSnapshotNotFound) {
			slog.Warn("repo delete failed", "snapshot", name, "error", repoErr)
		}
	}

	return nil
}

// anyToStringSlice 将 any 转换为 []string。
// Neo4j 驱动返回 labels(n) 为 []any，需转换为 []string。
func anyToStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
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
func (sm *SnapshotManager) Restore(ctx context.Context, name string) (err error) {
	sm.lock.Lock()
	defer sm.lock.Unlock()

	// V1-18: 审计日志 — 无论成功失败都记录
	defer func() { //nolint:contextcheck // defer 闭包中无法传递 ctx
		sm.auditLog.Record(AuditEntry{
			Action:   "restore",
			Snapshot: name,
			Actor:    "system",
			Detail:   "restore to default",
			Error:    errStr(err),
		})
		// V2-15: Prometheus 指标上报
		if err != nil {
			observability.SnapshotOperationsTotal.WithLabelValues("restore", "error").Inc()
		} else {
			observability.SnapshotOperationsTotal.WithLabelValues("restore", "success").Inc()
		}
	}()

	if loadErr := sm.EnsureLoaded(ctx, name); loadErr != nil {
		return fmt.Errorf("ensure loaded: %w", loadErr)
	}

	if clearErr := sm.graph.ClearDB(ctx, "default"); clearErr != nil {
		return fmt.Errorf("clear default: %w", clearErr)
	}

	if cloneErr := sm.graph.CloneDB(ctx, name, "default"); cloneErr != nil {
		return fmt.Errorf("clone db: %w", cloneErr)
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
func (sm *SnapshotManager) Diff(ctx context.Context, a, b string) (diff *SnapshotDiff, err error) {
	// V2-15: Prometheus 指标上报
	defer func() {
		if err != nil {
			observability.SnapshotOperationsTotal.WithLabelValues("diff", "error").Inc()
		} else {
			observability.SnapshotOperationsTotal.WithLabelValues("diff", "success").Inc()
		}
	}()

	if err = sm.EnsureLoaded(ctx, a); err != nil {
		return nil, fmt.Errorf("ensure loaded a: %w", err)
	}
	if err = sm.EnsureLoaded(ctx, b); err != nil {
		return nil, fmt.Errorf("ensure loaded b: %w", err)
	}

	diff = &SnapshotDiff{}

	// b 中有而 a 中没有的节点（新增）
	addedNodeRows, err := sm.graph.Query(ctx, b,
		`MATCH (n) WHERE NOT EXISTS { MATCH (m {_db: $other, uri: n.uri}) } `+
			`RETURN labels(n) AS labels, n.uri AS uri, properties(n) AS props`,
		map[string]any{"other": a})
	if err != nil {
		return nil, fmt.Errorf("diff added nodes: %w", err)
	}
	for _, row := range addedNodeRows {
		labels := anyToStringSlice(row["labels"])
		uri, _ := row["uri"].(string)
		if len(labels) > 0 && uri != "" {
			diff.AddedNodes = append(diff.AddedNodes, assembler.Node{
				Labels: labels, URI: uri, Props: extractProps(row["props"]),
			})
		}
	}

	// a 中有而 b 中没有的节点（删除）
	removedNodeRows, err := sm.graph.Query(ctx, a,
		`MATCH (n) WHERE NOT EXISTS { MATCH (m {_db: $other, uri: n.uri}) } `+
			`RETURN labels(n) AS labels, n.uri AS uri, properties(n) AS props`,
		map[string]any{"other": b})
	if err != nil {
		return nil, fmt.Errorf("diff removed nodes: %w", err)
	}
	for _, row := range removedNodeRows {
		labels := anyToStringSlice(row["labels"])
		uri, _ := row["uri"].(string)
		if len(labels) > 0 && uri != "" {
			diff.RemovedNodes = append(diff.RemovedNodes, assembler.Node{
				Labels: labels, URI: uri, Props: extractProps(row["props"]),
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

	// === 第 5 条: 节点属性级对比 ===
	changedNodeRows, err := sm.graph.Query(ctx, b,
		`MATCH (a {_db: $a}) MATCH (b {_db: $b}) `+
			`WHERE a.uri = b.uri AND labels(a) = labels(b) `+
			`WITH a, b, [k IN keys(a) WHERE k <> '_db' AND a[k] <> b[k]] AS diffKeys `+
			`WHERE size(diffKeys) > 0 `+
			`RETURN a.uri AS uri, labels(a) AS labels, `+
			`properties(a) AS aProps, properties(b) AS bProps`,
		map[string]any{"a": a, "b": b})
	if err != nil {
		return nil, fmt.Errorf("diff changed nodes: %w", err)
	}
	for _, row := range changedNodeRows {
		uri, _ := row["uri"].(string)
		labels := anyToStringSlice(row["labels"])
		aProps := extractProps(row["aProps"])
		bProps := extractProps(row["bProps"])
		added, removed, modified := compareProps(aProps, bProps)
		if len(added) > 0 || len(removed) > 0 || len(modified) > 0 {
			label := ""
			if len(labels) > 0 {
				label = labels[len(labels)-1] // MostSpecificLabel
			}
			diff.ChangedNodes = append(diff.ChangedNodes, NodeChange{
				URI:            uri,
				Label:          label,
				AddedFields:    added,
				RemovedFields:  removed,
				ModifiedFields: modified,
			})
		}
	}

	// === 第 6 条: 关系属性级对比 ===
	changedRelRows, err := sm.graph.Query(ctx, b,
		`MATCH (sa)-[ra]->(ea), (sb)-[rb]->(eb) `+
			`WHERE sa._db = $a AND sb._db = $b `+
			`AND sa.uri = sb.uri AND ea.uri = eb.uri `+
			`AND type(ra) = type(rb) `+
			`WITH ra, rb, [k IN keys(ra) WHERE ra[k] <> rb[k]] AS diffKeys `+
			`WHERE size(diffKeys) > 0 `+
			`RETURN type(ra) AS type, startNode(ra).uri AS from, endNode(ra).uri AS to, `+
			`properties(ra) AS aProps, properties(rb) AS bProps`,
		map[string]any{"a": a, "b": b})
	if err != nil {
		return nil, fmt.Errorf("diff changed rels: %w", err)
	}
	for _, row := range changedRelRows {
		typ, _ := row["type"].(string)
		from, _ := row["from"].(string)
		to, _ := row["to"].(string)
		aProps := extractProps(row["aProps"])
		bProps := extractProps(row["bProps"])
		added, removed, modified := compareProps(aProps, bProps)
		if len(added) > 0 || len(removed) > 0 || len(modified) > 0 {
			diff.ChangedRels = append(diff.ChangedRels, RelChange{
				Type:           typ,
				From:           from,
				To:             to,
				AddedFields:    added,
				RemovedFields:  removed,
				ModifiedFields: modified,
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

	// === 属性级对比: 节点 ===
	// 对 URI 交集中的节点调用 compareProps，填充 ChangedNodes。
	for uri, bNode := range bNodeURIs {
		if aNode, ok := aNodeURIs[uri]; ok {
			added, removed, modified := compareProps(aNode.Props, bNode.Props)
			if len(added) > 0 || len(removed) > 0 || len(modified) > 0 {
				diff.ChangedNodes = append(diff.ChangedNodes, NodeChange{
					URI:            uri,
					Label:          bNode.MostSpecificLabel(),
					AddedFields:    added,
					RemovedFields:  removed,
					ModifiedFields: modified,
				})
			}
		}
	}

	// === 属性级对比: 关系 ===
	// 对 type+from+to 交集中的关系调用 compareProps，填充 ChangedRels。
	for key, bRel := range bRelKeys {
		if aRel, ok := aRelKeys[key]; ok {
			added, removed, modified := compareProps(aRel.Props, bRel.Props)
			if len(added) > 0 || len(removed) > 0 || len(modified) > 0 {
				diff.ChangedRels = append(diff.ChangedRels, RelChange{
					Type:           bRel.Type,
					From:           bRel.From,
					To:             bRel.To,
					AddedFields:    added,
					RemovedFields:  removed,
					ModifiedFields: modified,
				})
			}
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

// compareProps 对比两个属性 map，返回 added/removed/modified 三个分类。
// 数值归一化: int(42) vs float64(42.0) 视为相等。
func compareProps(a, b map[string]any) (added, removed map[string]any, modified map[string]FieldChange) {
	added = make(map[string]any)
	removed = make(map[string]any)
	modified = make(map[string]FieldChange)

	// b 有 a 无 → added
	for k, v := range b {
		if _, ok := a[k]; !ok {
			added[k] = v
		}
	}

	// a 有 b 无 → removed
	for k, v := range a {
		if _, ok := b[k]; !ok {
			removed[k] = v
		}
	}

	// 两边都有但值不同 → modified
	for k, aVal := range a {
		if bVal, ok := b[k]; ok {
			if !valuesEqual(aVal, bVal) {
				modified[k] = FieldChange{OldValue: aVal, NewValue: bVal}
			}
		}
	}

	return
}

// valuesEqual 比较两个 any 值，处理 int/float64 归一化。
func valuesEqual(a, b any) bool {
	// 数值归一化: 都转 float64 比较
	aFloat, aOK := toFloat64(a)
	bFloat, bOK := toFloat64(b)
	if aOK && bOK {
		return aFloat == bFloat
	}
	// 兜底: 字符串表示比较
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// toFloat64 将数值类型转换为 float64，不支持的类型返回 false。
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}
