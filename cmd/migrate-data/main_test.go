package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// TestImportMetaOnlyParsing 验证 YAML 元数据解析正确性（使用真实 demo 快照文件）。
func TestImportMetaOnlyParsing(t *testing.T) {
	// 使用项目中的 demo 快照文件
	testFile := filepath.Join("..", "..", "snapshots", "demo", "snap-demo-001.yaml")
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("demo snapshot file not found, skipping")
	}

	meta, err := snapshot.ImportMetaOnly(testFile)
	require.NoError(t, err)
	require.Equal(t, "snap-demo-001", meta.Name)
	require.NotZero(t, meta.CreatedAt)
	require.Equal(t, 21, meta.NodeCount)
	require.Equal(t, 21, meta.RelCount)
	require.Equal(t, testFile, meta.FilePath)
}

// TestMigrateSnapshots_DryRun 验证 dry-run 模式只扫描不写入。
func TestMigrateSnapshots_DryRun(t *testing.T) {
	// 创建临时目录 + 写入测试 YAML
	tmpDir := t.TempDir()
	testYAML := `kind: SnapshotMeta
name: test-migrate-001
created_at: 2026-07-01T12:00:00+08:00
node_count: 5
rel_count: 3
---
kind: Nodes
items: []
---
kind: Relations
items: []
`
	err := os.WriteFile(filepath.Join(tmpDir, "test-snap.yaml"), []byte(testYAML), 0644)
	require.NoError(t, err)

	// Dry run（repo=nil，不会写入）
	stats, err := MigrateSnapshots(context.Background(), tmpDir, true, nil)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Scanned)
	require.Equal(t, 1, stats.Created)
	require.Equal(t, 0, stats.Skipped)
	require.Equal(t, 0, stats.Failed)
}

// TestMigrateSnapshots_EmptyDir 验证空目录扫描结果为 0。
func TestMigrateSnapshots_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	stats, err := MigrateSnapshots(context.Background(), tmpDir, true, nil)
	require.NoError(t, err)
	require.Equal(t, 0, stats.Scanned)
	require.Equal(t, 0, stats.Created)
}

// TestMigrateSnapshots_InvalidYAML 验证损坏的 YAML 被跳过，不中断扫描。
func TestMigrateSnapshots_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	// 写入损坏的 YAML
	err := os.WriteFile(filepath.Join(tmpDir, "bad.yaml"), []byte("not: valid: yaml: {{{"), 0644)
	require.NoError(t, err)

	// 写入正常的 YAML
	goodYAML := `kind: SnapshotMeta
name: good-snap
created_at: 2026-07-01T12:00:00+08:00
node_count: 1
rel_count: 0
`
	err = os.WriteFile(filepath.Join(tmpDir, "good.yaml"), []byte(goodYAML), 0644)
	require.NoError(t, err)

	stats, err := MigrateSnapshots(context.Background(), tmpDir, true, nil)
	require.NoError(t, err)
	require.Equal(t, 2, stats.Scanned)
	require.Equal(t, 1, stats.Created)
	require.Equal(t, 1, stats.Failed) // bad.yaml 解析失败
}

// TestIsDuplicateError 验证重复错误检测。
func TestIsDuplicateError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"duplicate key", &mockErr{msg: `ERROR: duplicate key value violates unique constraint "snapshots_name_key" (SQLSTATE 23505)`}, true},
		{"other error", &mockErr{msg: "connection refused"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isDuplicateError(tc.err))
		})
	}
}

// TestMigrateStats_Fields 验证 MigrateStats 结构体字段。
func TestMigrateStats_Fields(t *testing.T) {
	s := MigrateStats{
		Scanned: 10,
		Created: 5,
		Skipped: 3,
		Failed:  2,
	}
	require.Equal(t, 10, s.Scanned)
	require.Equal(t, 5, s.Created)
	require.Equal(t, 3, s.Skipped)
	require.Equal(t, 2, s.Failed)
}

// mockErr 简单 error 实现。
type mockErr struct {
	msg string
}

func (e *mockErr) Error() string { return e.msg }
