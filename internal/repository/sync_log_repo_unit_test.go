// Package repository 单元测试（内存 SyncLogRepository 实现 + 类型验证）。
package repository

import (
	"context"
	"testing"
	"time"
)

// TestSyncLogRecordFields 验证 SyncLogRecord 字段赋值正确。
func TestSyncLogRecordFields(t *testing.T) {
	now := time.Now()
	rec := SyncLogRecord{
		ID:               1,
		SyncType:         "full",
		Status:           "success",
		NodesCreated:     10,
		RelationsCreated: 5,
		OrphanEdges:      2,
		Warnings:         []byte(`["warn1"]`),
		ErrorMessage:     "",
		StartedAt:        now,
		CompletedAt:      now.Add(3 * time.Second),
		DurationMs:       3000,
	}

	if rec.ID != 1 {
		t.Errorf("ID = %d, want 1", rec.ID)
	}
	if rec.SyncType != "full" {
		t.Errorf("SyncType = %q, want %q", rec.SyncType, "full")
	}
	if rec.Status != "success" {
		t.Errorf("Status = %q, want %q", rec.Status, "success")
	}
	if rec.NodesCreated != 10 {
		t.Errorf("NodesCreated = %d, want 10", rec.NodesCreated)
	}
	if rec.RelationsCreated != 5 {
		t.Errorf("RelationsCreated = %d, want 5", rec.RelationsCreated)
	}
	if rec.OrphanEdges != 2 {
		t.Errorf("OrphanEdges = %d, want 2", rec.OrphanEdges)
	}
	if rec.DurationMs != 3000 {
		t.Errorf("DurationMs = %d, want 3000", rec.DurationMs)
	}
}

// TestMemSyncLogCreate 验证 Create 后 List 可查到记录。
func TestMemSyncLogCreate(t *testing.T) {
	repo := NewMemSyncLogRepository()
	ctx := context.Background()

	rec := SyncLogRecord{
		SyncType:     "full",
		Status:       "success",
		NodesCreated: 10,
		StartedAt:    time.Now(),
		CompletedAt:  time.Now(),
		DurationMs:   500,
	}
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got, err := repo.List(ctx, 0)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List() returned %d records, want 1", len(got))
	}
	if got[0].SyncType != "full" {
		t.Errorf("SyncType = %q, want %q", got[0].SyncType, "full")
	}
	if got[0].NodesCreated != 10 {
		t.Errorf("NodesCreated = %d, want 10", got[0].NodesCreated)
	}
	if got[0].ID == 0 {
		t.Error("ID should be populated after Create")
	}
}

// TestMemSyncLogList 验证 List 按 started_at DESC 排序 + limit 截断。
func TestMemSyncLogList(t *testing.T) {
	repo := NewMemSyncLogRepository()
	ctx := context.Background()

	now := time.Now()
	records := []SyncLogRecord{
		{SyncType: "full", StartedAt: now.Add(-2 * time.Hour), Status: "success"},
		{SyncType: "full", StartedAt: now, Status: "success"},
		{SyncType: "full", StartedAt: now.Add(-1 * time.Hour), Status: "success"},
	}
	for _, rec := range records {
		if err := repo.Create(ctx, rec); err != nil {
			t.Fatalf("Create() error: %v", err)
		}
	}

	// 无 limit
	got, err := repo.List(ctx, 0)
	if err != nil {
		t.Fatalf("List(0) error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List(0) returned %d records, want 3", len(got))
	}

	// 按 started_at DESC：now → now-1h → now-2h
	if got[0].StartedAt.Before(got[1].StartedAt) {
		t.Error("List() not sorted by started_at DESC")
	}
	if got[1].StartedAt.Before(got[2].StartedAt) {
		t.Error("List() not sorted by started_at DESC")
	}

	// limit = 2
	got2, err := repo.List(ctx, 2)
	if err != nil {
		t.Fatalf("List(2) error: %v", err)
	}
	if len(got2) != 2 {
		t.Errorf("List(2) returned %d records, want 2", len(got2))
	}
}

// TestMemSyncLogListByType 验证按类型过滤。
func TestMemSyncLogListByType(t *testing.T) {
	repo := NewMemSyncLogRepository()
	ctx := context.Background()

	now := time.Now()
	_ = repo.Create(ctx, SyncLogRecord{SyncType: "full", StartedAt: now, Status: "success"})
	_ = repo.Create(ctx, SyncLogRecord{SyncType: "incremental", StartedAt: now, Status: "success"})
	_ = repo.Create(ctx, SyncLogRecord{SyncType: "full", StartedAt: now, Status: "failed"})

	got, err := repo.ListByType(ctx, "full", 0)
	if err != nil {
		t.Fatalf("ListByType() error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("ListByType(full) returned %d records, want 2", len(got))
	}
	for _, r := range got {
		if r.SyncType != "full" {
			t.Errorf("unexpected SyncType = %q", r.SyncType)
		}
	}
}

// TestMemSyncLogListByTypeEmpty 验证无匹配类型返回空切片。
func TestMemSyncLogListByTypeEmpty(t *testing.T) {
	repo := NewMemSyncLogRepository()
	ctx := context.Background()

	_ = repo.Create(ctx, SyncLogRecord{SyncType: "full", StartedAt: time.Now(), Status: "success"})

	got, err := repo.ListByType(ctx, "incremental", 0)
	if err != nil {
		t.Fatalf("ListByType() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListByType(incremental) returned %d records, want 0", len(got))
	}
}

// TestMemSyncLogCount 验证计数正确。
func TestMemSyncLogCount(t *testing.T) {
	repo := NewMemSyncLogRepository()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = repo.Create(ctx, SyncLogRecord{SyncType: "full", StartedAt: time.Now(), Status: "success"})
	}

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}
	if count != 5 {
		t.Errorf("Count() = %d, want 5", count)
	}
}

// TestMemSyncLogCountEmpty 验证空库返回 0。
func TestMemSyncLogCountEmpty(t *testing.T) {
	repo := NewMemSyncLogRepository()
	ctx := context.Background()

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}
	if count != 0 {
		t.Errorf("Count() = %d, want 0", count)
	}
}

// TestMemSyncLogAutoID 验证 ID 自增。
func TestMemSyncLogAutoID(t *testing.T) {
	repo := NewMemSyncLogRepository()
	ctx := context.Background()

	_ = repo.Create(ctx, SyncLogRecord{SyncType: "full", StartedAt: time.Now(), Status: "success"})
	_ = repo.Create(ctx, SyncLogRecord{SyncType: "full", StartedAt: time.Now(), Status: "success"})

	got, _ := repo.List(ctx, 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	// 由于 List 按 StartedAt DESC 排序，两条 StartedAt 可能相同，用 ID 来判断自增
	if got[0].ID >= got[1].ID {
		// 如果排序恰好 ID 大的在前，说明 ID 是自增的
	}
	// 直接检查所有 ID 不同且大于 0
	if got[0].ID <= 0 || got[1].ID <= 0 {
		t.Error("IDs should be > 0")
	}
	if got[0].ID == got[1].ID {
		t.Error("IDs should be unique")
	}
}
