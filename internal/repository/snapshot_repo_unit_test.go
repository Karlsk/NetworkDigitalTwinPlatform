// Package repository 单元测试（内存 SnapshotRepository 实现 + 类型验证）。
package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestSnapshotRecordFields 验证 SnapshotRecord 字段赋值正确。
func TestSnapshotRecordFields(t *testing.T) {
	now := time.Now()
	rec := SnapshotRecord{
		ID:        42,
		Name:      "snap-001",
		CreatedAt: now,
		NodeCount: 10,
		RelCount:  5,
		FilePath:  "/tmp/snap.yaml",
		Status:    "active",
	}

	if rec.ID != 42 {
		t.Errorf("ID = %d, want 42", rec.ID)
	}
	if rec.Name != "snap-001" {
		t.Errorf("Name = %q, want %q", rec.Name, "snap-001")
	}
	if !rec.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", rec.CreatedAt, now)
	}
	if rec.NodeCount != 10 {
		t.Errorf("NodeCount = %d, want 10", rec.NodeCount)
	}
	if rec.RelCount != 5 {
		t.Errorf("RelCount = %d, want 5", rec.RelCount)
	}
	if rec.FilePath != "/tmp/snap.yaml" {
		t.Errorf("FilePath = %q, want %q", rec.FilePath, "/tmp/snap.yaml")
	}
	if rec.Status != "active" {
		t.Errorf("Status = %q, want %q", rec.Status, "active")
	}
}

// TestMemSnapshotCreate 验证内存实现创建记录后可 GetByName 查到。
func TestMemSnapshotCreate(t *testing.T) {
	repo := NewMemSnapshotRepository()
	ctx := context.Background()

	rec := &SnapshotRecord{
		Name:      "snap-001",
		CreatedAt: time.Now(),
		NodeCount: 10,
		RelCount:  5,
		FilePath:  "/tmp/snap.yaml",
		Status:    "active",
	}
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// ID 应被回填
	if rec.ID == 0 {
		t.Error("Create() should populate rec.ID, got 0")
	}

	// GetByName 应查到
	got, err := repo.GetByName(ctx, "snap-001")
	if err != nil {
		t.Fatalf("GetByName() error: %v", err)
	}
	if got.Name != "snap-001" {
		t.Errorf("GetByName().Name = %q, want %q", got.Name, "snap-001")
	}
	if got.NodeCount != 10 {
		t.Errorf("GetByName().NodeCount = %d, want 10", got.NodeCount)
	}
	if got.Status != "active" {
		t.Errorf("GetByName().Status = %q, want %q", got.Status, "active")
	}
}

// TestMemSnapshotCreateDuplicate 验证重复 name 创建返回错误。
func TestMemSnapshotCreateDuplicate(t *testing.T) {
	repo := NewMemSnapshotRepository()
	ctx := context.Background()

	rec1 := &SnapshotRecord{Name: "dup", CreatedAt: time.Now(), Status: "active"}
	if err := repo.Create(ctx, rec1); err != nil {
		t.Fatalf("first Create() error: %v", err)
	}

	rec2 := &SnapshotRecord{Name: "dup", CreatedAt: time.Now(), Status: "active"}
	err := repo.Create(ctx, rec2)
	if err == nil {
		t.Fatal("second Create() with same name should return error")
	}
}

// TestMemSnapshotList 验证 List 按 created_at DESC 排序。
func TestMemSnapshotList(t *testing.T) {
	repo := NewMemSnapshotRepository()
	ctx := context.Background()

	now := time.Now()
	records := []SnapshotRecord{
		{Name: "old", CreatedAt: now.Add(-2 * time.Hour), Status: "active"},
		{Name: "new", CreatedAt: now, Status: "active"},
		{Name: "mid", CreatedAt: now.Add(-1 * time.Hour), Status: "active"},
	}
	for i := range records {
		if err := repo.Create(ctx, &records[i]); err != nil {
			t.Fatalf("Create(%q) error: %v", records[i].Name, err)
		}
	}

	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List() returned %d records, want 3", len(got))
	}

	// 按 created_at DESC：new → mid → old
	expected := []string{"new", "mid", "old"}
	for i, name := range expected {
		if got[i].Name != name {
			t.Errorf("List()[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

// TestMemSnapshotGetByNameNotFound 验证不存在名称返回 ErrSnapshotNotFound。
func TestMemSnapshotGetByNameNotFound(t *testing.T) {
	repo := NewMemSnapshotRepository()
	ctx := context.Background()

	_, err := repo.GetByName(ctx, "nonexistent")
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Errorf("GetByName(nonexistent) error = %v, want ErrSnapshotNotFound", err)
	}
}

// TestMemSnapshotDelete 验证删除后 GetByName 返回 NotFound。
func TestMemSnapshotDelete(t *testing.T) {
	repo := NewMemSnapshotRepository()
	ctx := context.Background()

	rec := &SnapshotRecord{Name: "to-delete", CreatedAt: time.Now(), Status: "active"}
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := repo.Delete(ctx, "to-delete"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err := repo.GetByName(ctx, "to-delete")
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Errorf("GetByName after Delete error = %v, want ErrSnapshotNotFound", err)
	}
}

// TestMemSnapshotDeleteNotFound 验证删除不存在的记录返回 ErrSnapshotNotFound。
func TestMemSnapshotDeleteNotFound(t *testing.T) {
	repo := NewMemSnapshotRepository()
	ctx := context.Background()

	err := repo.Delete(ctx, "nonexistent")
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Errorf("Delete(nonexistent) error = %v, want ErrSnapshotNotFound", err)
	}
}

// TestMemSnapshotUpdateStatus 验证状态更新正确。
func TestMemSnapshotUpdateStatus(t *testing.T) {
	repo := NewMemSnapshotRepository()
	ctx := context.Background()

	rec := &SnapshotRecord{Name: "snap-upd", CreatedAt: time.Now(), Status: "active"}
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := repo.UpdateStatus(ctx, "snap-upd", "archived"); err != nil {
		t.Fatalf("UpdateStatus() error: %v", err)
	}

	got, err := repo.GetByName(ctx, "snap-upd")
	if err != nil {
		t.Fatalf("GetByName() error: %v", err)
	}
	if got.Status != "archived" {
		t.Errorf("Status = %q, want %q", got.Status, "archived")
	}
}

// TestMemSnapshotUpdateStatusNotFound 验证更新不存在的记录返回 ErrSnapshotNotFound。
func TestMemSnapshotUpdateStatusNotFound(t *testing.T) {
	repo := NewMemSnapshotRepository()
	ctx := context.Background()

	err := repo.UpdateStatus(ctx, "nonexistent", "active")
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Errorf("UpdateStatus(nonexistent) error = %v, want ErrSnapshotNotFound", err)
	}
}

// TestMemSnapshotListEmpty 验证空列表返回空切片而非 nil（便于 JSON 序列化）。
func TestMemSnapshotListEmpty(t *testing.T) {
	repo := NewMemSnapshotRepository()
	ctx := context.Background()

	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if got == nil {
		t.Error("List() on empty repo should return non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("List() on empty repo returned %d records, want 0", len(got))
	}
}

// TestMemSnapshotAutoID 验证多次创建 ID 自增。
func TestMemSnapshotAutoID(t *testing.T) {
	repo := NewMemSnapshotRepository()
	ctx := context.Background()

	r1 := &SnapshotRecord{Name: "a", CreatedAt: time.Now(), Status: "active"}
	r2 := &SnapshotRecord{Name: "b", CreatedAt: time.Now(), Status: "active"}
	_ = repo.Create(ctx, r1)
	_ = repo.Create(ctx, r2)

	if r1.ID >= r2.ID {
		t.Errorf("ID not auto-incrementing: r1.ID=%d, r2.ID=%d", r1.ID, r2.ID)
	}
}
