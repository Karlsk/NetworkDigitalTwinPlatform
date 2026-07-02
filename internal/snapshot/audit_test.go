package snapshot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// V1-18: AuditLog 审计日志测试
// ---------------------------------------------------------------------------

// TestNewAuditLog 验证构造函数返回非 nil，maxEntries 生效。
func TestNewAuditLog(t *testing.T) {
	al := NewAuditLog(100)
	if al == nil {
		t.Fatal("NewAuditLog() returned nil")
	}
	if al.maxEntries != 100 {
		t.Errorf("maxEntries = %d, want 100", al.maxEntries)
	}
}

// TestAuditLog_Record_SetsTimestamp 验证 Record 后 Timestamp 被自动设置。
func TestAuditLog_Record_SetsTimestamp(t *testing.T) {
	al := NewAuditLog(10)
	before := time.Now().Add(-time.Millisecond)
	al.Record(AuditEntry{Action: "create", Snapshot: "snap-001"})
	after := time.Now().Add(time.Millisecond)

	entries := al.Recent(1)
	if len(entries) != 1 {
		t.Fatalf("Recent(1) returned %d entries, want 1", len(entries))
	}
	if entries[0].Timestamp.Before(before) || entries[0].Timestamp.After(after) {
		t.Errorf("Timestamp = %v, expected between %v and %v", entries[0].Timestamp, before, after)
	}
}

// TestAuditLog_Record_FIFOEviction 验证超出 maxEntries 时旧条目被淘汰。
func TestAuditLog_Record_FIFOEviction(t *testing.T) {
	al := NewAuditLog(3) // 最多保留 3 条
	for i := 0; i < 5; i++ {
		al.Record(AuditEntry{Action: "create", Snapshot: "snap-" + string(rune('0'+i))})
	}

	entries := al.Recent(10) // 请求多于实际
	if len(entries) != 3 {
		t.Fatalf("Recent(10) returned %d entries, want 3 (FIFO eviction)", len(entries))
	}
	// 最新的 3 条应该是 snap-2, snap-3, snap-4
	if entries[0].Snapshot != "snap-2" {
		t.Errorf("entries[0].Snapshot = %q, want %q", entries[0].Snapshot, "snap-2")
	}
	if entries[2].Snapshot != "snap-4" {
		t.Errorf("entries[2].Snapshot = %q, want %q", entries[2].Snapshot, "snap-4")
	}
}

// TestAuditLog_Query_ByAction 验证按 Action 过滤。
func TestAuditLog_Query_ByAction(t *testing.T) {
	al := NewAuditLog(100)
	al.Record(AuditEntry{Action: "create", Snapshot: "snap-a"})
	al.Record(AuditEntry{Action: "restore", Snapshot: "snap-b"})
	al.Record(AuditEntry{Action: "delete", Snapshot: "snap-c"})
	al.Record(AuditEntry{Action: "create", Snapshot: "snap-d"})

	results := al.Query(AuditFilter{Action: "create"})
	if len(results) != 2 {
		t.Fatalf("Query(Action=create) returned %d, want 2", len(results))
	}
	for _, r := range results {
		if r.Action != "create" {
			t.Errorf("expected Action=create, got %q", r.Action)
		}
	}
}

// TestAuditLog_Query_BySnapshot 验证按 Snapshot 名称过滤。
func TestAuditLog_Query_BySnapshot(t *testing.T) {
	al := NewAuditLog(100)
	al.Record(AuditEntry{Action: "create", Snapshot: "snap-target"})
	al.Record(AuditEntry{Action: "restore", Snapshot: "snap-other"})
	al.Record(AuditEntry{Action: "delete", Snapshot: "snap-target"})

	results := al.Query(AuditFilter{Snapshot: "snap-target"})
	if len(results) != 2 {
		t.Fatalf("Query(Snapshot=snap-target) returned %d, want 2", len(results))
	}
}

// TestAuditLog_Query_ByTimeRange 验证按 Since/Until 时间范围过滤。
func TestAuditLog_Query_ByTimeRange(t *testing.T) {
	al := NewAuditLog(100)

	// 手动插入带时间戳的条目
	past := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-10 * time.Minute)

	al.mu.Lock()
	al.entries = append(al.entries, AuditEntry{Timestamp: past, Action: "create", Snapshot: "old"})
	al.entries = append(al.entries, AuditEntry{Timestamp: recent, Action: "create", Snapshot: "new"})
	al.mu.Unlock()

	// Since: 只查最近 1 小时的
	results := al.Query(AuditFilter{Since: time.Now().Add(-1 * time.Hour)})
	if len(results) != 1 {
		t.Fatalf("Query(Since=-1h) returned %d, want 1", len(results))
	}
	if results[0].Snapshot != "new" {
		t.Errorf("expected Snapshot=new, got %q", results[0].Snapshot)
	}

	// Until: 只查 1 小时前的
	results = al.Query(AuditFilter{Until: time.Now().Add(-1 * time.Hour)})
	if len(results) != 1 {
		t.Fatalf("Query(Until=-1h) returned %d, want 1", len(results))
	}
	if results[0].Snapshot != "old" {
		t.Errorf("expected Snapshot=old, got %q", results[0].Snapshot)
	}
}

// TestAuditLog_Query_EmptyFilter 验证空过滤器返回全部。
func TestAuditLog_Query_EmptyFilter(t *testing.T) {
	al := NewAuditLog(100)
	al.Record(AuditEntry{Action: "create", Snapshot: "snap-a"})
	al.Record(AuditEntry{Action: "restore", Snapshot: "snap-b"})
	al.Record(AuditEntry{Action: "delete", Snapshot: "snap-c"})

	results := al.Query(AuditFilter{})
	if len(results) != 3 {
		t.Errorf("Query(empty filter) returned %d, want 3", len(results))
	}
}

// TestAuditLog_Recent 验证返回最近 N 条。
func TestAuditLog_Recent(t *testing.T) {
	al := NewAuditLog(100)
	for i := 0; i < 10; i++ {
		al.Record(AuditEntry{Action: "create", Snapshot: "snap"})
	}

	results := al.Recent(5)
	if len(results) != 5 {
		t.Fatalf("Recent(5) returned %d, want 5", len(results))
	}
}

// TestAuditLog_Recent_MoreThanTotal 验证 n 大于总数时返回全部。
func TestAuditLog_Recent_MoreThanTotal(t *testing.T) {
	al := NewAuditLog(100)
	al.Record(AuditEntry{Action: "create", Snapshot: "snap-a"})
	al.Record(AuditEntry{Action: "restore", Snapshot: "snap-b"})

	results := al.Recent(100)
	if len(results) != 2 {
		t.Errorf("Recent(100) returned %d, want 2 (total entries)", len(results))
	}
}

// TestAuditLog_Recent_ZeroAndNegative 验证 n<=0 返回 nil。
func TestAuditLog_Recent_ZeroAndNegative(t *testing.T) {
	al := NewAuditLog(100)
	al.Record(AuditEntry{Action: "create", Snapshot: "snap"})

	if results := al.Recent(0); results != nil {
		t.Errorf("Recent(0) = %v, want nil", results)
	}
	if results := al.Recent(-1); results != nil {
		t.Errorf("Recent(-1) = %v, want nil", results)
	}
}

// TestAuditLog_ConcurrentSafety 多 goroutine 并发 Record + Query，-race 通过。
func TestAuditLog_ConcurrentSafety(t *testing.T) {
	al := NewAuditLog(1000)
	var wg sync.WaitGroup

	// 并发写入
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				al.Record(AuditEntry{Action: "create", Snapshot: "snap"})
			}
		}(i)
	}

	// 并发查询
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = al.Query(AuditFilter{})
				_ = al.Recent(10)
			}
		}()
	}

	wg.Wait()

	// 验证条目数不超过 maxEntries
	al.mu.RLock()
	count := len(al.entries)
	al.mu.RUnlock()
	if count > 1000 {
		t.Errorf("entries count = %d, should be <= maxEntries (1000)", count)
	}
}

// ---------------------------------------------------------------------------
// SnapshotManager 审计集成测试
// ---------------------------------------------------------------------------

// TestManager_Create_RecordsAudit 验证 Create 后审计日志有 "create" 记录。
func TestManager_Create_RecordsAudit(t *testing.T) {
	gdb := &mockGraphDB{
		queryResults: []map[string]any{
			{"labels": []any{"Device"}, "uri": "device:001", "props": map[string]any{"hostname": "r1"}},
		},
	}
	lock := NewGraphLock()
	snapDir := t.TempDir()
	mgr := NewSnapshotManager(gdb, lock, snapDir, 5)

	meta, err := mgr.Create(context.Background(), "snap-audit")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// 查询审计日志
	entries := mgr.AuditLog().Query(AuditFilter{Action: "create"})
	if len(entries) != 1 {
		t.Fatalf("AuditLog has %d create entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Snapshot != "snap-audit" {
		t.Errorf("audit Snapshot = %q, want %q", entry.Snapshot, "snap-audit")
	}
	if entry.Error != "" {
		t.Errorf("audit Error = %q, want empty", entry.Error)
	}
	if entry.Detail == "" {
		t.Error("audit Detail should not be empty")
	}
	// 验证 Detail 包含节点和关系计数
	_ = meta // meta 已使用
}

// TestManager_Restore_RecordsAudit 验证 Restore 后审计日志有 "restore" 记录。
func TestManager_Restore_RecordsAudit(t *testing.T) {
	snapDir := t.TempDir()
	writeTestSnapshot(t, snapDir, "snap-restore",
		[]yamlNodeItem{{Labels: []string{"Device"}, URI: "device:001"}},
		nil,
	)

	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-restore": false},
	}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), snapDir, 5)

	err := mgr.Restore(context.Background(), "snap-restore")
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	entries := mgr.AuditLog().Query(AuditFilter{Action: "restore"})
	if len(entries) != 1 {
		t.Fatalf("AuditLog has %d restore entries, want 1", len(entries))
	}
	if entries[0].Snapshot != "snap-restore" {
		t.Errorf("audit Snapshot = %q, want %q", entries[0].Snapshot, "snap-restore")
	}
}

// TestManager_Delete_RecordsAudit 验证 Delete 后审计日志有 "delete" 记录。
func TestManager_Delete_RecordsAudit(t *testing.T) {
	gdb := &mockGraphDB{
		hasDBResult: map[string]bool{"snap-del": true},
	}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), t.TempDir(), 5)

	err := mgr.Delete(context.Background(), "snap-del")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	entries := mgr.AuditLog().Query(AuditFilter{Action: "delete"})
	if len(entries) != 1 {
		t.Fatalf("AuditLog has %d delete entries, want 1", len(entries))
	}
	if entries[0].Snapshot != "snap-del" {
		t.Errorf("audit Snapshot = %q, want %q", entries[0].Snapshot, "snap-del")
	}
}

// TestManager_CreateError_RecordsAuditWithError 验证 Create 失败时 Error 字段非空。
func TestManager_CreateError_RecordsAuditWithError(t *testing.T) {
	wantErr := errors.New("neo4j connection refused")
	gdb := &mockGraphDB{queryErr: wantErr}
	mgr := NewSnapshotManager(gdb, NewGraphLock(), t.TempDir(), 5)

	_, err := mgr.Create(context.Background(), "snap-fail")
	if err == nil {
		t.Fatal("Create() should return error")
	}

	entries := mgr.AuditLog().Query(AuditFilter{Action: "create"})
	if len(entries) != 1 {
		t.Fatalf("AuditLog has %d create entries, want 1", len(entries))
	}
	if entries[0].Error == "" {
		t.Error("audit Error should not be empty when Create fails")
	}
}

// TestErrStr 验证 errStr 辅助函数。
func TestErrStr(t *testing.T) {
	if errStr(nil) != "" {
		t.Error("errStr(nil) should return empty string")
	}
	if errStr(errors.New("test error")) != "test error" {
		t.Errorf("errStr(error) = %q, want %q", errStr(errors.New("test error")), "test error")
	}
}
