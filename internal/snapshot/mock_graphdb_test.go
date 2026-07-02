package snapshot

import (
	"context"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/graph"
)

// ---------------------------------------------------------------------------
// mockGraphDB — snapshot 包测试用的 GraphDB mock
// ---------------------------------------------------------------------------

// mockGraphDB 实现 GraphDB 接口，支持注入错误和记录调用参数。
type mockGraphDB struct {
	// 注入错误
	clearDBErr    error
	bulkCreateErr error
	queryErr      error
	cloneDBErr    error
	hasDBErr      error
	listDBsErr    error

	// 可配置返回数据
	queryResults    []map[string]any
	queryResultsSeq [][]map[string]any // 按序返回不同查询结果
	querySeqIdx     int                // queryResultsSeq 当前索引
	hasDBResult     map[string]bool    // db name → 是否存在
	listDBsResult   []string

	// 调用记录
	clearDBCalls    []string
	bulkCreateCalls []bulkCreateCall
	cloneDBCalls    []cloneCall
	queryCalls      []queryCall

	// BulkCreate 期间回调（用于锁测试）
	bulkCreateDuring func()
	// BulkCreate 阻塞通道：设置后 BulkCreate 在回调后等待此通道关闭
	bulkCreateHold chan struct{}
}

type bulkCreateCall struct {
	DB    string
	Nodes []assembler.Node
	Rels  []assembler.Relation
}

type cloneCall struct {
	From string
	To   string
}

type queryCall struct {
	DB     string
	Cypher string
	Params map[string]any
}

// 编译时接口满足检查
var _ graph.GraphDB = (*mockGraphDB)(nil)

func (m *mockGraphDB) Ping(_ context.Context) error { return nil }
func (m *mockGraphDB) Close() error                 { return nil }

func (m *mockGraphDB) BulkCreate(_ context.Context, db string, nodes []assembler.Node, rels []assembler.Relation) error {
	m.bulkCreateCalls = append(m.bulkCreateCalls, bulkCreateCall{DB: db, Nodes: nodes, Rels: rels})
	if m.bulkCreateDuring != nil {
		m.bulkCreateDuring()
	}
	if m.bulkCreateHold != nil {
		<-m.bulkCreateHold
	}
	return m.bulkCreateErr
}

func (m *mockGraphDB) Upsert(_ context.Context, _ string, _ []assembler.Node, _ []assembler.Relation) error {
	return nil
}

func (m *mockGraphDB) DeleteRelations(_ context.Context, _ string, _ []assembler.Relation) error {
	return nil
}

func (m *mockGraphDB) DeleteByURIs(_ context.Context, _ string, _ []string) error { return nil }

func (m *mockGraphDB) Query(_ context.Context, db string, cypher string, params map[string]any) ([]map[string]any, error) {
	m.queryCalls = append(m.queryCalls, queryCall{DB: db, Cypher: cypher, Params: params})
	// 按序返回不同结果（用于 Diff 方法多条 Cypher 查询测试）
	if m.queryResultsSeq != nil {
		if m.querySeqIdx < len(m.queryResultsSeq) {
			result := m.queryResultsSeq[m.querySeqIdx]
			m.querySeqIdx++
			return result, m.queryErr
		}
		return nil, m.queryErr
	}
	return m.queryResults, m.queryErr
}

func (m *mockGraphDB) BuildCypher(_ string, _ string, _ []assembler.Node, _ []assembler.Relation, _ []string) (string, map[string]any) {
	return "", nil
}

func (m *mockGraphDB) ClearDB(_ context.Context, db string) error {
	m.clearDBCalls = append(m.clearDBCalls, db)
	return m.clearDBErr
}

func (m *mockGraphDB) CloneDB(_ context.Context, from, to string) error {
	m.cloneDBCalls = append(m.cloneDBCalls, cloneCall{From: from, To: to})
	return m.cloneDBErr
}

func (m *mockGraphDB) ListDBs(_ context.Context) ([]string, error) {
	return m.listDBsResult, m.listDBsErr
}

func (m *mockGraphDB) HasDB(_ context.Context, db string) (bool, error) {
	if m.hasDBResult != nil {
		return m.hasDBResult[db], m.hasDBErr
	}
	return false, m.hasDBErr
}

func (m *mockGraphDB) EnsureIndexes(_ context.Context, _ []string) error { return nil }
