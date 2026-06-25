// Package service 实现业务编排层
package service

import (
	"context"
	"sync/atomic"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/connector"
	"gitlab.com/pml/network-digital-twin/internal/graph"
)

// ---------------------------------------------------------------------------
// mockGraphDB — 可配置行为的 GraphDB mock，用于 SyncService 测试
// ---------------------------------------------------------------------------

// mockGraphDB 实现 GraphDB 接口，支持注入错误和记录调用参数。
type mockGraphDB struct {
	// 注入错误
	clearDBErr         error
	bulkCreateErr      error
	upsertErr          error
	deleteByURIsErr    error
	deleteRelationsErr error

	// 记录调用参数
	clearDBCalls         []string
	bulkCreateNodes      []assembler.Node
	bulkCreateRels       []assembler.Relation
	upsertNodes          []assembler.Node
	upsertRels           []assembler.Relation
	deleteByURIsCalls    [][]string
	deleteRelationsCalls [][]assembler.Relation

	// 原子计数器（并发安全，用于测试轮询）
	deleteByURIsCount    atomic.Int32
	deleteRelationsCount atomic.Int32
}

// 编译时接口满足检查
var _ graph.GraphDB = (*mockGraphDB)(nil)

func (m *mockGraphDB) Ping(_ context.Context) error { return nil }
func (m *mockGraphDB) Close() error                 { return nil }

func (m *mockGraphDB) BulkCreate(_ context.Context, _ string, nodes []assembler.Node, rels []assembler.Relation) error {
	m.bulkCreateNodes = nodes
	m.bulkCreateRels = rels
	return m.bulkCreateErr
}

func (m *mockGraphDB) Upsert(_ context.Context, _ string, nodes []assembler.Node, rels []assembler.Relation) error {
	m.upsertNodes = append(m.upsertNodes, nodes...)
	m.upsertRels = append(m.upsertRels, rels...)
	return m.upsertErr
}

func (m *mockGraphDB) DeleteRelations(_ context.Context, _ string, rels []assembler.Relation) error {
	m.deleteRelationsCalls = append(m.deleteRelationsCalls, rels)
	m.deleteRelationsCount.Add(1)
	return m.deleteRelationsErr
}

func (m *mockGraphDB) DeleteByURIs(_ context.Context, _ string, uris []string) error {
	m.deleteByURIsCalls = append(m.deleteByURIsCalls, uris)
	m.deleteByURIsCount.Add(1)
	return m.deleteByURIsErr
}

func (m *mockGraphDB) Query(_ context.Context, _ string, _ string, _ map[string]any) ([]map[string]any, error) {
	return nil, nil
}

func (m *mockGraphDB) BuildCypher(_ string, _ string, _ []assembler.Node, _ []assembler.Relation, _ []string) (string, map[string]any) {
	return "", nil
}

func (m *mockGraphDB) ClearDB(_ context.Context, db string) error {
	m.clearDBCalls = append(m.clearDBCalls, db)
	return m.clearDBErr
}

func (m *mockGraphDB) CloneDB(_ context.Context, _, _ string) error { return nil }

func (m *mockGraphDB) ListDBs(_ context.Context) ([]string, error) { return nil, nil }

func (m *mockGraphDB) HasDB(_ context.Context, _ string) (bool, error) { return false, nil }

func (m *mockGraphDB) EnsureIndexes(_ context.Context, _ []string) error { return nil }

// ---------------------------------------------------------------------------
// mockConnector — 可注入错误的 Connector mock
// ---------------------------------------------------------------------------

// mockConnector 实现 Connector 接口，支持配置返回数据和注入错误。
type mockConnector struct {
	name        string
	entityTypes []string
	resources   map[string][]connector.Resource
	collectErr  error            // 所有 entityType 共享的错误
	collectErrs map[string]error // 按 entityType 注入不同错误
}

// 编译时接口满足检查
var _ connector.Connector = (*mockConnector)(nil)

func (m *mockConnector) Metadata() connector.ConnectorMetadata {
	return connector.ConnectorMetadata{
		Name:        m.name,
		Type:        "mock",
		EntityTypes: m.entityTypes,
	}
}

func (m *mockConnector) Collect(_ context.Context, entityType string) ([]connector.Resource, error) {
	// 优先检查按 entityType 注入的错误
	if err, ok := m.collectErrs[entityType]; ok {
		return nil, err
	}
	// 其次检查全局错误
	if m.collectErr != nil {
		return nil, m.collectErr
	}
	// 返回配置的数据
	return m.resources[entityType], nil
}

func (m *mockConnector) Stream(_ context.Context, _ string) (<-chan connector.Resource, error) {
	return nil, connector.ErrNotImplemented
}
