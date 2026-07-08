// Package repository SchemaVersionRepository 单元测试（内存实现）。
package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestSchemaVersionRecordFields 验证 SchemaVersionRecord 字段赋值正确。
func TestSchemaVersionRecordFields(t *testing.T) {
	now := time.Now()
	rec := SchemaVersionRecord{
		ID:            1,
		Version:       100,
		EntityTypes:   []byte(`[{"name":"Device"}]`),
		RelationTypes: []byte(`[{"name":"HAS_INTERFACE"}]`),
		AppliedAt:     now,
		Description:   "initial schema",
	}

	if rec.ID != 1 {
		t.Errorf("ID = %d, want 1", rec.ID)
	}
	if rec.Version != 100 {
		t.Errorf("Version = %d, want 100", rec.Version)
	}
	if rec.Description != "initial schema" {
		t.Errorf("Description = %q, want %q", rec.Description, "initial schema")
	}
	if !rec.AppliedAt.Equal(now) {
		t.Errorf("AppliedAt = %v, want %v", rec.AppliedAt, now)
	}
}

// TestMemSchemaVersionCreate 验证创建成功，ID 回填。
func TestMemSchemaVersionCreate(t *testing.T) {
	repo := NewMemSchemaVersionRepository()
	ctx := context.Background()

	rec := &SchemaVersionRecord{
		Version:       1,
		EntityTypes:   []byte(`[]`),
		RelationTypes: []byte(`[]`),
		Description:   "v1",
	}
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if rec.ID == 0 {
		t.Error("Create() should populate rec.ID, got 0")
	}
	if rec.AppliedAt.IsZero() {
		t.Error("Create() should populate rec.AppliedAt when zero")
	}
}

// TestMemSchemaVersionCreateWithAppliedAt 验证非零 AppliedAt 不被覆盖。
func TestMemSchemaVersionCreateWithAppliedAt(t *testing.T) {
	repo := NewMemSchemaVersionRepository()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	rec := &SchemaVersionRecord{
		Version:   1,
		AppliedAt: now,
	}
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if !rec.AppliedAt.Equal(now) {
		t.Errorf("AppliedAt should not be overwritten: got %v, want %v", rec.AppliedAt, now)
	}
}

// TestMemSchemaVersionLatest 验证返回最新版本。
func TestMemSchemaVersionLatest(t *testing.T) {
	repo := NewMemSchemaVersionRepository()
	ctx := context.Background()

	records := []SchemaVersionRecord{
		{Version: 1, Description: "v1"},
		{Version: 3, Description: "v3"},
		{Version: 2, Description: "v2"},
	}
	for i := range records {
		if err := repo.Create(ctx, &records[i]); err != nil {
			t.Fatalf("Create(v%d) error: %v", records[i].Version, err)
		}
	}

	latest, err := repo.Latest(ctx)
	if err != nil {
		t.Fatalf("Latest() error: %v", err)
	}
	if latest.Version != 3 {
		t.Errorf("Latest().Version = %d, want 3", latest.Version)
	}
	if latest.Description != "v3" {
		t.Errorf("Latest().Description = %q, want %q", latest.Description, "v3")
	}
}

// TestMemSchemaVersionLatestEmpty 验证空表返回 ErrSchemaVersionNotFound。
func TestMemSchemaVersionLatestEmpty(t *testing.T) {
	repo := NewMemSchemaVersionRepository()
	ctx := context.Background()

	_, err := repo.Latest(ctx)
	if !errors.Is(err, ErrSchemaVersionNotFound) {
		t.Errorf("Latest() on empty repo error = %v, want ErrSchemaVersionNotFound", err)
	}
}

// TestMemSchemaVersionList 验证按 version DESC 排序。
func TestMemSchemaVersionList(t *testing.T) {
	repo := NewMemSchemaVersionRepository()
	ctx := context.Background()

	records := []SchemaVersionRecord{
		{Version: 1, Description: "v1"},
		{Version: 5, Description: "v5"},
		{Version: 3, Description: "v3"},
	}
	for i := range records {
		if err := repo.Create(ctx, &records[i]); err != nil {
			t.Fatalf("Create(v%d) error: %v", records[i].Version, err)
		}
	}

	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List() returned %d records, want 3", len(got))
	}

	// 按 version DESC：5 → 3 → 1
	expected := []int{5, 3, 1}
	for i, v := range expected {
		if got[i].Version != v {
			t.Errorf("List()[%d].Version = %d, want %d", i, got[i].Version, v)
		}
	}
}

// TestMemSchemaVersionListEmpty 验证空表返回空切片非 nil。
func TestMemSchemaVersionListEmpty(t *testing.T) {
	repo := NewMemSchemaVersionRepository()
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

// TestMemSchemaVersionAutoID 验证多次创建 ID 自增。
func TestMemSchemaVersionAutoID(t *testing.T) {
	repo := NewMemSchemaVersionRepository()
	ctx := context.Background()

	r1 := &SchemaVersionRecord{Version: 1}
	r2 := &SchemaVersionRecord{Version: 2}
	_ = repo.Create(ctx, r1)
	_ = repo.Create(ctx, r2)

	if r1.ID >= r2.ID {
		t.Errorf("ID not auto-incrementing: r1.ID=%d, r2.ID=%d", r1.ID, r2.ID)
	}
}
