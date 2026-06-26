//go:build e2e

// Package e2e 提供端到端集成测试，连接真实 Neo4j 实例验证完整数据管线。
// 运行方式: go test -tags=e2e -v -count=1 ./test/e2e/...
// 需要本地 Neo4j CE 运行在 bolt://localhost:7687（认证 neo4j/password）。
// Neo4j 不可达时自动 Skip，不会失败。
package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/config"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/graph"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/schema"
	"gitlab.com/pml/network-digital-twin/internal/service"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// newE2EClient 创建连接本地 Neo4j 的 GraphDB 客户端。
// Ping 失败时 t.Skip，确保无 Neo4j 时测试优雅跳过。
func newE2EClient(t *testing.T) graph.GraphDB {
	t.Helper()

	uri := envOrDefault("NEO4J_URI", "bolt://localhost:7687")
	user := envOrDefault("NEO4J_USER", "neo4j")
	password := envOrDefault("NEO4J_PASSWORD", "password123")

	client, err := graph.NewNeo4jClient(config.Neo4JConfig{
		URI:       uri,
		User:      user,
		Password:  password,
		DefaultDB: "default",
	})
	if err != nil {
		t.Skipf("Neo4j client creation failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		client.Close()
		t.Skipf("Neo4j not reachable at %s: %v", uri, err)
	}

	t.Cleanup(func() { _ = client.Close() })
	return client
}

// uniqueDBName 返回唯一的逻辑 DB 名称，格式 e2e_{testName}_{nanoTimestamp}。
func uniqueDBName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("e2e_%s_%d", t.Name(), time.Now().UnixNano())
}

// cleanupDB 清理测试创建的逻辑 DB 数据，在 defer 中调用。
func cleanupDB(t *testing.T, client graph.GraphDB, db string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.ClearDB(ctx, db); err != nil {
		t.Logf("cleanupDB(%s) error: %v", db, err)
	}
}

// loadOntology 加载项目根目录下 ontology/ 的本体定义。
func loadOntology(t *testing.T) schema.SchemaRegistry {
	t.Helper()
	ontologyDir := filepath.Join("..", "..", "ontology")
	if _, err := os.Stat(ontologyDir); os.IsNotExist(err) {
		t.Skipf("ontology directory not found at %s", ontologyDir)
	}
	reg := schema.NewSchemaRegistry()
	if err := reg.Load(ontologyDir); err != nil {
		t.Fatalf("Load(%q) error = %v", ontologyDir, err)
	}
	return reg
}

// runFullPipeline 执行 Connector → Normalizer → Assembler 全管线处理。
// 使用 testdata/mock_netbox/ 下的全部 5 种实体类型。
func runFullPipeline(t *testing.T, reg schema.SchemaRegistry) *assembler.GraphModel {
	t.Helper()

	dataDir := filepath.Join("..", "..", "testdata", "mock_netbox")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Skipf("testdata/mock_netbox not found at %s", dataDir)
	}

	entityTypes := []string{"Device", "Interface", "ISIS", "Link", "Network_Slice"}
	conn := mock.NewMockConnector("e2e-mock", dataDir, entityTypes)
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)

	var allResources []normalizer.NormalizedResource
	ctx := context.Background()

	for _, et := range entityTypes {
		resources, err := conn.Collect(ctx, et)
		if err != nil {
			t.Fatalf("Collect(%s) error = %v", et, err)
		}
		for _, res := range resources {
			nr, err := norm.Normalize(res)
			if err != nil {
				t.Fatalf("Normalize(%s/%s) error = %v", res.Kind, res.ID, err)
			}
			allResources = append(allResources, *nr)
		}
	}

	gm, _, err := asm.Assemble(allResources)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	return gm
}

// countNodes 查询指定逻辑 DB 中的节点数量。
func countNodes(t *testing.T, client graph.GraphDB, db string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := client.Query(ctx, db,
		"MATCH (n) WHERE n._db = $_db RETURN count(n) AS cnt", nil)
	if err != nil {
		t.Fatalf("countNodes(%s) error = %v", db, err)
	}
	if len(rows) == 0 {
		return 0
	}
	cnt, _ := rows[0]["cnt"].(int64)
	return int(cnt)
}

// countRels 查询指定逻辑 DB 中的关系数量。
func countRels(t *testing.T, client graph.GraphDB, db string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := client.Query(ctx, db,
		"MATCH (a)-[r]->(b) WHERE a._db = $_db RETURN count(r) AS cnt", nil)
	if err != nil {
		t.Fatalf("countRels(%s) error = %v", db, err)
	}
	if len(rows) == 0 {
		return 0
	}
	cnt, _ := rows[0]["cnt"].(int64)
	return int(cnt)
}

// countNodesByLabel 查询指定逻辑 DB 中某 Label 的节点数量。
func countNodesByLabel(t *testing.T, client graph.GraphDB, db, label string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cypher := fmt.Sprintf("MATCH (n:%s) WHERE n._db = $_db RETURN count(n) AS cnt", label)
	rows, err := client.Query(ctx, db, cypher, nil)
	if err != nil {
		t.Fatalf("countNodesByLabel(%s, %s) error = %v", db, label, err)
	}
	if len(rows) == 0 {
		return 0
	}
	cnt, _ := rows[0]["cnt"].(int64)
	return int(cnt)
}

// countRelsByType 查询指定逻辑 DB 中某类型的关系数量。
func countRelsByType(t *testing.T, client graph.GraphDB, db, relType string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cypher := fmt.Sprintf("MATCH (a)-[r:%s]->(b) WHERE a._db = $_db RETURN count(r) AS cnt", relType)
	rows, err := client.Query(ctx, db, cypher, nil)
	if err != nil {
		t.Fatalf("countRelsByType(%s, %s) error = %v", db, relType, err)
	}
	if len(rows) == 0 {
		return 0
	}
	cnt, _ := rows[0]["cnt"].(int64)
	return int(cnt)
}

// envOrDefault 读取环境变量，不存在时返回默认值。
func envOrDefault(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// newE2ESyncService 创建连接真实 Neo4j 的 SyncService。
// 使用 testdata/mock_netbox/ 数据和完整 ontology，bufferSize=20。
func newE2ESyncService(t *testing.T, client graph.GraphDB, lock *snapshot.GraphLock) *service.SyncService {
	t.Helper()
	reg := loadOntology(t)

	dataDir := filepath.Join("..", "..", "testdata", "mock_netbox")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Skipf("testdata/mock_netbox not found at %s", dataDir)
	}

	entityTypes := []string{"Device", "Interface", "ISIS", "Link", "Network_Slice"}
	conn := mock.NewMockConnector("e2e-mock", dataDir, entityTypes)
	registry := connector.NewConnectorRegistry()
	registry.Register(conn)

	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)
	return service.NewSyncService(registry, norm, asm, client, lock, 20)
}

// newE2ESnapshotManager 创建 SnapshotManager，与 SyncService 共享 GraphLock。
func newE2ESnapshotManager(t *testing.T, client graph.GraphDB, lock *snapshot.GraphLock) *snapshot.SnapshotManager {
	t.Helper()
	snapDir := t.TempDir()
	return snapshot.NewSnapshotManager(client, lock, snapDir, 5)
}

// backupAndRestoreDefault 备份 default DB，返回恢复函数用于 defer。
func backupAndRestoreDefault(t *testing.T, client graph.GraphDB) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backupDB := fmt.Sprintf("e2e_backup_%d", time.Now().UnixNano())

	// 检查 default 是否有数据
	rows, _ := client.Query(ctx, "default",
		"MATCH (n) WHERE n._db = $_db RETURN count(n) AS cnt", nil)
	hasData := false
	if len(rows) > 0 {
		if cnt, _ := rows[0]["cnt"].(int64); cnt > 0 {
			hasData = true
			if err := client.CloneDB(ctx, "default", backupDB); err != nil {
				t.Logf("CloneDB backup error (non-fatal): %v", err)
				hasData = false
			}
		}
	}

	return func() {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel2()
		if hasData {
			_ = client.ClearDB(ctx2, "default")
			_ = client.CloneDB(ctx2, backupDB, "default")
		} else {
			_ = client.ClearDB(ctx2, "default")
		}
		_ = client.ClearDB(ctx2, backupDB)
	}
}
