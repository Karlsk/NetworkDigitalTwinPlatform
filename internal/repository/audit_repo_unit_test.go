package repository

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// V2-09: AuditLogRepository 单元测试（结构体字段验证）
// ---------------------------------------------------------------------------

// TestAuditLogRecordFields 验证 AuditLogRecord 结构体字段赋值。
func TestAuditLogRecordFields(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	rec := AuditLogRecord{
		ID:        1,
		Timestamp: now,
		Action:    "create",
		Snapshot:  "snap-001",
		Actor:     "system",
		Detail:    "nodes=10, rels=5",
		Error:     "",
	}

	if rec.ID != 1 {
		t.Errorf("ID = %d, want 1", rec.ID)
	}
	if !rec.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", rec.Timestamp, now)
	}
	if rec.Action != "create" {
		t.Errorf("Action = %q, want %q", rec.Action, "create")
	}
	if rec.Snapshot != "snap-001" {
		t.Errorf("Snapshot = %q, want %q", rec.Snapshot, "snap-001")
	}
	if rec.Actor != "system" {
		t.Errorf("Actor = %q, want %q", rec.Actor, "system")
	}
	if rec.Detail != "nodes=10, rels=5" {
		t.Errorf("Detail = %q, want %q", rec.Detail, "nodes=10, rels=5")
	}
	if rec.Error != "" {
		t.Errorf("Error = %q, want empty", rec.Error)
	}
}

// TestAuditFilterFields 验证 AuditFilter 结构体字段赋值。
func TestAuditFilterFields(t *testing.T) {
	since := time.Now().Add(-2 * time.Hour)
	until := time.Now().Add(-1 * time.Hour)

	f := AuditFilter{
		Action:   "create",
		Snapshot: "snap-001",
		Since:    since,
		Until:    until,
	}

	if f.Action != "create" {
		t.Errorf("Action = %q, want %q", f.Action, "create")
	}
	if f.Snapshot != "snap-001" {
		t.Errorf("Snapshot = %q, want %q", f.Snapshot, "snap-001")
	}
	if !f.Since.Equal(since) {
		t.Errorf("Since = %v, want %v", f.Since, since)
	}
	if !f.Until.Equal(until) {
		t.Errorf("Until = %v, want %v", f.Until, until)
	}
}

// TestAuditFilterZeroValue 验证零值 AuditFilter 所有字段为空。
func TestAuditFilterZeroValue(t *testing.T) {
	f := AuditFilter{}
	if f.Action != "" {
		t.Errorf("Action = %q, want empty", f.Action)
	}
	if f.Snapshot != "" {
		t.Errorf("Snapshot = %q, want empty", f.Snapshot)
	}
	if !f.Since.IsZero() {
		t.Errorf("Since = %v, want zero", f.Since)
	}
	if !f.Until.IsZero() {
		t.Errorf("Until = %v, want zero", f.Until)
	}
}
