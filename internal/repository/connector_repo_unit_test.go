// Package repository ConnectorConfigRepository 单元测试（内存实现）。
package repository

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestConnectorConfigRecordFields 验证 ConnectorConfigRecord 字段赋值正确。
func TestConnectorConfigRecordFields(t *testing.T) {
	now := time.Now()
	rec := ConnectorConfigRecord{
		ID:          42,
		Name:        "mock-netbox",
		Type:        "mock",
		Config:      []byte(`{"data_dir":"testdata"}`),
		EntityTypes: []byte(`["Device","Interface"]`),
		Priority:    10,
		Status:      "active",
		LastPing:    &now,
	}

	if rec.ID != 42 {
		t.Errorf("ID = %d, want 42", rec.ID)
	}
	if rec.Name != "mock-netbox" {
		t.Errorf("Name = %q, want %q", rec.Name, "mock-netbox")
	}
	if rec.Type != "mock" {
		t.Errorf("Type = %q, want %q", rec.Type, "mock")
	}
	if rec.Priority != 10 {
		t.Errorf("Priority = %d, want 10", rec.Priority)
	}
	if rec.Status != "active" {
		t.Errorf("Status = %q, want %q", rec.Status, "active")
	}
	if rec.LastPing == nil || !rec.LastPing.Equal(now) {
		t.Errorf("LastPing = %v, want %v", rec.LastPing, now)
	}
}

// TestMemConnectorUpsert 验证创建 + 更新成功，ID 回填。
func TestMemConnectorUpsert(t *testing.T) {
	repo := NewMemConnectorRepository()
	ctx := context.Background()

	// 第一次 Upsert：创建
	rec := ConnectorConfigRecord{
		Name:        "mock-001",
		Type:        "mock",
		Config:      []byte(`{}`),
		EntityTypes: []byte(`["Device"]`),
		Status:      "active",
	}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("Upsert() create error: %v", err)
	}

	// 验证创建成功
	got, err := repo.GetByName(ctx, "mock-001")
	if err != nil {
		t.Fatalf("GetByName() error: %v", err)
	}
	if got.ID == 0 {
		t.Error("Upsert() should populate ID, got 0")
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q", got.Status, "active")
	}

	// 第二次 Upsert：更新（全量覆盖）
	updatedRec := ConnectorConfigRecord{
		Name:        "mock-001",
		Type:        "mock",
		Config:      []byte(`{"data_dir":"new"}`),
		EntityTypes: []byte(`["Device","Interface"]`),
		Status:      "disabled",
	}
	if err := repo.Upsert(ctx, updatedRec); err != nil {
		t.Fatalf("Upsert() update error: %v", err)
	}

	// 验证更新成功
	got2, err := repo.GetByName(ctx, "mock-001")
	if err != nil {
		t.Fatalf("GetByName() after update error: %v", err)
	}
	if got2.Status != "disabled" {
		t.Errorf("Status after update = %q, want %q", got2.Status, "disabled")
	}
	if got2.ID != got.ID {
		t.Errorf("ID should remain same after update: got %d, want %d", got2.ID, got.ID)
	}
}

// TestMemConnectorGetByName 验证精确查找。
func TestMemConnectorGetByName(t *testing.T) {
	repo := NewMemConnectorRepository()
	ctx := context.Background()

	rec := ConnectorConfigRecord{
		Name:   "find-me",
		Type:   "netbox",
		Status: "active",
	}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("Upsert() error: %v", err)
	}

	got, err := repo.GetByName(ctx, "find-me")
	if err != nil {
		t.Fatalf("GetByName() error: %v", err)
	}
	if got.Name != "find-me" {
		t.Errorf("Name = %q, want %q", got.Name, "find-me")
	}
	if got.Type != "netbox" {
		t.Errorf("Type = %q, want %q", got.Type, "netbox")
	}
}

// TestMemConnectorGetByNameNotFound 验证不存在返回 ErrConnectorConfigNotFound。
func TestMemConnectorGetByNameNotFound(t *testing.T) {
	repo := NewMemConnectorRepository()
	ctx := context.Background()

	_, err := repo.GetByName(ctx, "nonexistent")
	if !errors.Is(err, ErrConnectorConfigNotFound) {
		t.Errorf("GetByName(nonexistent) error = %v, want ErrConnectorConfigNotFound", err)
	}
}

// TestMemConnectorList 验证返回所有记录，按 name ASC 排序。
func TestMemConnectorList(t *testing.T) {
	repo := NewMemConnectorRepository()
	ctx := context.Background()

	records := []ConnectorConfigRecord{
		{Name: "zebra", Type: "mock", Status: "active"},
		{Name: "alpha", Type: "netbox", Status: "active"},
		{Name: "middle", Type: "controller", Status: "disabled"},
	}
	for _, rec := range records {
		if err := repo.Upsert(ctx, rec); err != nil {
			t.Fatalf("Upsert(%q) error: %v", rec.Name, err)
		}
	}

	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List() returned %d records, want 3", len(got))
	}

	// 按 name ASC：alpha → middle → zebra
	expected := []string{"alpha", "middle", "zebra"}
	for i, name := range expected {
		if got[i].Name != name {
			t.Errorf("List()[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

// TestMemConnectorListEmpty 验证空表返回空切片非 nil。
func TestMemConnectorListEmpty(t *testing.T) {
	repo := NewMemConnectorRepository()
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

// TestMemConnectorUpdateStatus 验证状态更新正确。
func TestMemConnectorUpdateStatus(t *testing.T) {
	repo := NewMemConnectorRepository()
	ctx := context.Background()

	rec := ConnectorConfigRecord{Name: "status-test", Type: "mock", Status: "active"}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("Upsert() error: %v", err)
	}

	if err := repo.UpdateStatus(ctx, "status-test", "error"); err != nil {
		t.Fatalf("UpdateStatus() error: %v", err)
	}

	got, err := repo.GetByName(ctx, "status-test")
	if err != nil {
		t.Fatalf("GetByName() error: %v", err)
	}
	if got.Status != "error" {
		t.Errorf("Status = %q, want %q", got.Status, "error")
	}
}

// TestMemConnectorUpdateStatusNotFound 验证更新不存在的记录返回错误。
func TestMemConnectorUpdateStatusNotFound(t *testing.T) {
	repo := NewMemConnectorRepository()
	ctx := context.Background()

	err := repo.UpdateStatus(ctx, "nonexistent", "active")
	if !errors.Is(err, ErrConnectorConfigNotFound) {
		t.Errorf("UpdateStatus(nonexistent) error = %v, want ErrConnectorConfigNotFound", err)
	}
}

// TestMemConnectorUpdateLastPing 验证 LastPing 更新正确。
func TestMemConnectorUpdateLastPing(t *testing.T) {
	repo := NewMemConnectorRepository()
	ctx := context.Background()

	rec := ConnectorConfigRecord{Name: "ping-test", Type: "mock", Status: "active"}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("Upsert() error: %v", err)
	}

	pingTime := time.Now().Truncate(time.Second)
	if err := repo.UpdateLastPing(ctx, "ping-test", pingTime); err != nil {
		t.Fatalf("UpdateLastPing() error: %v", err)
	}

	got, err := repo.GetByName(ctx, "ping-test")
	if err != nil {
		t.Fatalf("GetByName() error: %v", err)
	}
	if got.LastPing == nil {
		t.Fatal("LastPing should not be nil after update")
	}
	if !got.LastPing.Equal(pingTime) {
		t.Errorf("LastPing = %v, want %v", got.LastPing, pingTime)
	}
}

// TestMemConnectorUpdateLastPingNotFound 验证更新不存在的记录返回错误。
func TestMemConnectorUpdateLastPingNotFound(t *testing.T) {
	repo := NewMemConnectorRepository()
	ctx := context.Background()

	err := repo.UpdateLastPing(ctx, "nonexistent", time.Now())
	if !errors.Is(err, ErrConnectorConfigNotFound) {
		t.Errorf("UpdateLastPing(nonexistent) error = %v, want ErrConnectorConfigNotFound", err)
	}
}

// TestMemConnectorDelete 验证删除成功。
func TestMemConnectorDelete(t *testing.T) {
	repo := NewMemConnectorRepository()
	ctx := context.Background()

	rec := ConnectorConfigRecord{Name: "to-delete", Type: "mock", Status: "active"}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("Upsert() error: %v", err)
	}

	if err := repo.Delete(ctx, "to-delete"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err := repo.GetByName(ctx, "to-delete")
	if !errors.Is(err, ErrConnectorConfigNotFound) {
		t.Errorf("GetByName after Delete error = %v, want ErrConnectorConfigNotFound", err)
	}
}

// TestMemConnectorDeleteNotFound 验证删除不存在的记录返回错误。
func TestMemConnectorDeleteNotFound(t *testing.T) {
	repo := NewMemConnectorRepository()
	ctx := context.Background()

	err := repo.Delete(ctx, "nonexistent")
	if !errors.Is(err, ErrConnectorConfigNotFound) {
		t.Errorf("Delete(nonexistent) error = %v, want ErrConnectorConfigNotFound", err)
	}
}

// TestMemConnectorAutoID 验证多次创建 ID 自增。
func TestMemConnectorAutoID(t *testing.T) {
	repo := NewMemConnectorRepository()
	ctx := context.Background()

	r1 := ConnectorConfigRecord{Name: "a", Type: "mock", Status: "active"}
	r2 := ConnectorConfigRecord{Name: "b", Type: "mock", Status: "active"}
	_ = repo.Upsert(ctx, r1)
	_ = repo.Upsert(ctx, r2)

	got1, _ := repo.GetByName(ctx, "a")
	got2, _ := repo.GetByName(ctx, "b")
	if got1.ID >= got2.ID {
		t.Errorf("ID not auto-incrementing: a.ID=%d, b.ID=%d", got1.ID, got2.ID)
	}
}
