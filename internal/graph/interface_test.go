package graph

import (
	"context"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
)

// --- Compile-time interface satisfaction check ---

// stubGraphDB is a minimal implementation to verify the GraphDB interface
// can be satisfied. Full Neo4j implementation is deferred to I-12.
type stubGraphDB struct{}

func (s *stubGraphDB) Ping(_ context.Context) error { return nil }
func (s *stubGraphDB) Close() error                 { return nil }
func (s *stubGraphDB) BulkCreate(_ context.Context, _ string, _ []assembler.Node, _ []assembler.Relation) error {
	return nil
}
func (s *stubGraphDB) Upsert(_ context.Context, _ string, _ []assembler.Node, _ []assembler.Relation) error {
	return nil
}
func (s *stubGraphDB) DeleteRelations(_ context.Context, _ string, _ []assembler.Relation) error {
	return nil
}
func (s *stubGraphDB) DeleteByURIs(_ context.Context, _ string, _ []string) error { return nil }
func (s *stubGraphDB) Query(_ context.Context, _ string, _ string, _ map[string]any) ([]map[string]any, error) {
	return nil, nil
}
func (s *stubGraphDB) BuildCypher(_ string, _ string, _ []assembler.Node, _ []assembler.Relation, _ []string) (string, map[string]any) {
	return "", nil
}
func (s *stubGraphDB) ClearDB(_ context.Context, _ string) error { return nil }
func (s *stubGraphDB) CloneDB(_ context.Context, _, _ string) error { return nil }
func (s *stubGraphDB) ListDBs(_ context.Context) ([]string, error) { return nil, nil }
func (s *stubGraphDB) HasDB(_ context.Context, _ string) (bool, error) { return false, nil }
func (s *stubGraphDB) EnsureIndexes(_ context.Context, _ []string) error { return nil }

// Compile-time check: stubGraphDB must satisfy GraphDB interface.
var _ GraphDB = (*stubGraphDB)(nil)

// --- Interface method count and db parameter verification ---

func TestGraphDBMethodCount(t *testing.T) {
	// This test documents the expected 13 methods of GraphDB.
	// If a method is added or removed, the stubGraphDB above will fail
	// to compile, and this test serves as documentation.
	var db GraphDB = &stubGraphDB{}
	ctx := context.Background()

	// Category: 连接管理 (2 methods)
	_ = db.Ping(ctx)
	_ = db.Close()

	// Category: 全量同步 (1 method)
	_ = db.BulkCreate(ctx, "default", nil, nil)

	// Category: 增量同步 (3 methods)
	_ = db.Upsert(ctx, "default", nil, nil)
	_ = db.DeleteRelations(ctx, "default", nil)
	_ = db.DeleteByURIs(ctx, "default", nil)

	// Category: 查询 (2 methods)
	_, _ = db.Query(ctx, "default", "MATCH (n) RETURN n", nil)
	_, _ = db.BuildCypher("create", "default", nil, nil, nil)

	// Category: 逻辑 DB 管理 (4 methods)
	_ = db.ClearDB(ctx, "default")
	_ = db.CloneDB(ctx, "source", "target")
	_, _ = db.ListDBs(ctx)
	_, _ = db.HasDB(ctx, "default")

	// Category: 索引管理 (1 method)
	_ = db.EnsureIndexes(ctx, []string{"Device"})
}

// --- db parameter presence verification ---

func TestGraphDBDBParameterPresence(t *testing.T) {
	// Verify that all data-access methods accept a db parameter.
	// Methods WITHOUT db: Ping, Close, ListDBs (3 methods)
	// Methods WITH db: BulkCreate, Upsert, DeleteRelations, DeleteByURIs,
	//                  Query, BuildCypher, ClearDB, CloneDB, HasDB (9 methods)
	db := &stubGraphDB{}
	ctx := context.Background()

	// All these calls include "test-db" as the db parameter
	nodes := []assembler.Node{
		{Label: "Device", URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
	}
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}
	uris := []string{"device:SN001"}

	_ = db.BulkCreate(ctx, "test-db", nodes, rels)
	_ = db.Upsert(ctx, "test-db", nodes, rels)
	_ = db.DeleteRelations(ctx, "test-db", rels)
	_ = db.DeleteByURIs(ctx, "test-db", uris)
	_, _ = db.Query(ctx, "test-db", "MATCH (n) RETURN n", map[string]any{"limit": 10})
	cypher, params := db.BuildCypher("create", "test-db", nodes, rels, uris)
	_ = cypher
	_ = params
	_ = db.ClearDB(ctx, "test-db")
	_ = db.CloneDB(ctx, "test-db", "test-db-clone")
	_, _ = db.HasDB(ctx, "test-db")
}

// --- BuildCypher action values ---

func TestBuildCypherActionValues(t *testing.T) {
	db := &stubGraphDB{}
	nodes := []assembler.Node{{Label: "Device", URI: "device:A"}}
	rels := []assembler.Relation{{Type: "HAS_INTERFACE", From: "device:A", To: "iface:A_eth0"}}
	uris := []string{"device:A"}

	// All four action values should be valid (no panic)
	actions := []string{"create", "upsert", "delete", "delete_relations"}
	for _, action := range actions {
		cypher, params := db.BuildCypher(action, "default", nodes, rels, uris)
		// Stub returns empty values, but no panic
		_ = cypher
		_ = params
	}
}

// --- GraphDB uses assembler types (cross-package import verification) ---

func TestGraphDBUsesAssemblerTypes(t *testing.T) {
	// Verify that GraphDB operations use assembler.Node and assembler.Relation
	// as the GraphModel IR, not schema types. This is a key architecture
	// invariant (Principle I: Layered IR Pipeline).
	nodes := []assembler.Node{
		{Label: "Device", URI: "device:SN001", Props: map[string]any{"hostname": "r1"}},
		{Label: "Interface", URI: "iface:SN001_eth0", Props: map[string]any{"status": "Up"}},
	}
	rels := []assembler.Relation{
		{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
	}

	var db GraphDB = &stubGraphDB{}
	ctx := context.Background()

	// BulkCreate accepts assembler.Node and assembler.Relation
	if err := db.BulkCreate(ctx, "default", nodes, rels); err != nil {
		t.Errorf("BulkCreate() error = %v", err)
	}

	// Upsert accepts assembler.Node and assembler.Relation
	if err := db.Upsert(ctx, "default", nodes, rels); err != nil {
		t.Errorf("Upsert() error = %v", err)
	}

	// DeleteRelations accepts assembler.Relation
	if err := db.DeleteRelations(ctx, "default", rels); err != nil {
		t.Errorf("DeleteRelations() error = %v", err)
	}
}
