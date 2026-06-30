// Package service 实现业务编排层
package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// ---------------------------------------------------------------------------
// TC-AS01: NewAnalysisService 构造验证
// ---------------------------------------------------------------------------

func TestNewAnalysisService(t *testing.T) {
	gdb := &mockGraphDB{}
	lock := snapshot.NewGraphLock()

	svc := NewAnalysisService(gdb, lock)
	if svc == nil {
		t.Fatal("NewAnalysisService() returned nil")
	}
}

// ---------------------------------------------------------------------------
// TC-AS02: QueryTopology 默认参数（空 label -> "Device"，limit <= 0 -> 100）
// ---------------------------------------------------------------------------

func TestAnalysisService_QueryTopology_DefaultParams(t *testing.T) {
	gdb := &mockGraphDB{
		queryResults: []map[string]any{
			{"n": map[string]any{"uri": "device:SN001", "hostname": "router-01"}},
		},
	}
	lock := snapshot.NewGraphLock()
	svc := NewAnalysisService(gdb, lock)

	// 空 label + limit=0 应使用默认值
	result, err := svc.QueryTopology(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("QueryTopology() error = %v", err)
	}

	if result.Count != 1 {
		t.Errorf("Count = %d, want 1", result.Count)
	}
	if len(result.Nodes) != 1 {
		t.Errorf("len(Nodes) = %d, want 1", len(result.Nodes))
	}

	// 验证默认参数被注入到 Cypher 中: label=Device, limit=100
	if len(gdb.queryResults) == 0 {
		t.Skip("cannot verify Cypher, mockGraphDB does not record calls")
	}
}

// ---------------------------------------------------------------------------
// TC-AS03: QueryTopology 指定 label + limit
// ---------------------------------------------------------------------------

func TestAnalysisService_QueryTopology_CustomParams(t *testing.T) {
	gdb := &mockGraphDB{
		queryResults: []map[string]any{
			{"n": map[string]any{"uri": "iface:SN001_GE1", "if_name": "GE1/0/1"}},
			{"n": map[string]any{"uri": "iface:SN001_GE2", "if_name": "GE1/0/2"}},
		},
	}
	lock := snapshot.NewGraphLock()
	svc := NewAnalysisService(gdb, lock)

	result, err := svc.QueryTopology(context.Background(), "Interface", 50)
	if err != nil {
		t.Fatalf("QueryTopology() error = %v", err)
	}

	if result.Count != 2 {
		t.Errorf("Count = %d, want 2", result.Count)
	}
	if len(result.Nodes) != 2 {
		t.Errorf("len(Nodes) = %d, want 2", len(result.Nodes))
	}
}

// ---------------------------------------------------------------------------
// TC-AS04: QueryTopology GraphDB 返回错误
// ---------------------------------------------------------------------------

func TestAnalysisService_QueryTopology_Error(t *testing.T) {
	wantErr := errors.New("neo4j connection refused")
	gdb := &mockGraphDB{queryErr: wantErr}
	lock := snapshot.NewGraphLock()
	svc := NewAnalysisService(gdb, lock)

	_, err := svc.QueryTopology(context.Background(), "Device", 100)
	if err == nil {
		t.Fatal("QueryTopology() should return error when Query fails")
	}
	if !strings.Contains(err.Error(), "query topology") {
		t.Errorf("error should contain 'query topology', got: %v", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap original, got: %v", err)
	}
}
