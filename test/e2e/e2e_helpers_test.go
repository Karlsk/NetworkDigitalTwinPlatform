//go:build e2e

// Package e2e 提供端到端集成测试，连接真实 Neo4j 实例验证完整数据管线。
// 运行方式: go test -tags=e2e -v -count=1 ./test/e2e/...
// 需要本地 Neo4j CE 运行在 bolt://localhost:7687（认证 neo4j/password）。
// Neo4j 不可达时自动 Skip，不会失败。
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/config"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/events"
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
	pub, con := events.NewChannelEventBus(20)
	return service.NewSyncService(registry, norm, asm, client, lock, pub, con)
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

// testdataDir 返回 testdata/mock_netbox 目录的绝对路径。
func testdataDir(t *testing.T) string {
	t.Helper()
	dataDir := filepath.Join("..", "..", "testdata", "mock_netbox")
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		t.Fatalf("testdataDir: filepath.Abs(%s) error = %v", dataDir, err)
	}
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		t.Skipf("testdata/mock_netbox not found at %s", absDir)
	}
	return absDir
}

// setupE2EControllerServer 创建模拟 Controller API 的 httptest server。
// 返回包含 Device/Interface/Link/Alarm/VPN/Tunnel/ISIS/BGP 8 种实体的最小 mock 数据。
func setupE2EControllerServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "e2e-token", "expires_in": 3600})
	})

	mux.HandleFunc("/api/no/config/terra-pe:peInfos/peInfos", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"peInfo": []map[string]any{
				{
					"id": "e2e-dev-001", "name": "E2E-PE01", "pe-alias": "E2E-PE01",
					"node-type": "PE", "vendor-id": "H3C", "product-name": "CR16000",
					"management-ip": "10.0.0.1", "connect-status": "UP",
					"peports": map[string]any{
						"peport-info": []any{
							map[string]any{"id": "e2e-port-001", "name": "GE0/0/1", "status": "UP", "total-bandwidth": 10000},
						},
					},
				},
			},
		})
	})

	mux.HandleFunc("/api/sr/config/network-topology:network-topology/topology/linksInfo", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	})

	mux.HandleFunc("/monitor/alert/list", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": nil})
	})

	mux.HandleFunc("/api/no/config/ietf-l3vpn-ntw:l3vpn-ntw/page", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"page_num": 1, "page_size": 100, "total_elements": 0, "total_pages": 1,
			"content": []map[string]any{},
		})
	})

	mux.HandleFunc("/api/no/config/ietf-l2vpn-svc:l2vpn-svc/page", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"page_num": 1, "page_size": 100, "total_elements": 0, "total_pages": 1,
			"content": []map[string]any{},
		})
	})

	mux.HandleFunc("/api/sr/config/terra-te-svc:te-policy-instance/all", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	})

	mux.HandleFunc("/restconf/operations/oper-rpc:isis-neighbor", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{
				"isis-neighbor-result": "System ID: E2E-PEER\nInterface: GE0/0/1     Circuit Id:  001\nState: Up     HoldTime: 25s        Type: L2           PRI: --\nArea address(es): 49.0001\n",
			},
		})
	})

	mux.HandleFunc("/restconf/operations/oper-rpc:bgp-peer-config", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{
				"current-config-result": " BGP local router ID: 10.0.0.1\n Local AS number: 65000\n Peer                    AS  MsgRcvd  MsgSent OutQ  PrefRcv Up/Down  State\n 10.0.0.2              65000      100       90    0       10 100h20m Established\n",
			},
		})
	})

	return httptest.NewServer(mux)
}
