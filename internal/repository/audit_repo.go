// Package repository 提供 AuditLogRepository 审计日志持久化。
// V2-09: 审计日志持久化到 PostgreSQL audit_logs 表。
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLogRecord 审计日志数据库记录。
type AuditLogRecord struct {
	ID        int64
	Timestamp time.Time
	Action    string
	Snapshot  string
	Actor     string
	Detail    string
	Error     string
}

// AuditFilter 审计查询过滤器（repository 层，用于 PG 查询）。
type AuditFilter struct {
	Action   string
	Snapshot string
	Since    time.Time
	Until    time.Time
}

// AuditLogRepository 审计日志 CRUD 接口。
type AuditLogRepository interface {
	// Create 创建审计记录。
	Create(ctx context.Context, r AuditLogRecord) error
	// List 按 timestamp DESC 返回最近 limit 条记录。
	List(ctx context.Context, limit int) ([]AuditLogRecord, error)
	// Query 按过滤条件查询审计日志，最多返回 1000 条。
	Query(ctx context.Context, filter AuditFilter) ([]AuditLogRecord, error)
	// Count 返回审计日志总数。
	Count(ctx context.Context) (int64, error)
}

// ---------------------------------------------------------------------------
// PostgreSQL 实现
// ---------------------------------------------------------------------------

type pgAuditLogRepo struct {
	pool *pgxpool.Pool
}

// NewPGAuditLogRepository 创建基于 PostgreSQL 的 AuditLogRepository。
func NewPGAuditLogRepository(pool *pgxpool.Pool) AuditLogRepository {
	return &pgAuditLogRepo{pool: pool}
}

func (r *pgAuditLogRepo) Create(ctx context.Context, rec AuditLogRecord) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_logs (timestamp, action, snapshot, actor, detail, error)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		rec.Timestamp, rec.Action, rec.Snapshot, rec.Actor, rec.Detail, rec.Error)
	if err != nil {
		return fmt.Errorf("pg audit log repo: create: %w", err)
	}
	return nil
}

func (r *pgAuditLogRepo) List(ctx context.Context, limit int) ([]AuditLogRecord, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, timestamp, action, snapshot, actor, detail, error
		 FROM audit_logs ORDER BY timestamp DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("pg audit log repo: list: %w", err)
	}
	defer rows.Close()

	var records []AuditLogRecord
	for rows.Next() {
		var rec AuditLogRecord
		if err := rows.Scan(&rec.ID, &rec.Timestamp, &rec.Action, &rec.Snapshot,
			&rec.Actor, &rec.Detail, &rec.Error); err != nil {
			return nil, fmt.Errorf("pg audit log repo: scan row: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg audit log repo: iterate rows: %w", err)
	}
	return records, nil
}

func (r *pgAuditLogRepo) Query(ctx context.Context, f AuditFilter) ([]AuditLogRecord, error) {
	// 动态构建 WHERE 条件
	query := `SELECT id, timestamp, action, snapshot, actor, detail, error FROM audit_logs WHERE 1=1`
	args := []any{}
	argIdx := 1

	if f.Action != "" {
		query += fmt.Sprintf(" AND action = $%d", argIdx)
		args = append(args, f.Action)
		argIdx++
	}
	if f.Snapshot != "" {
		query += fmt.Sprintf(" AND snapshot = $%d", argIdx)
		args = append(args, f.Snapshot)
		argIdx++
	}
	if !f.Since.IsZero() {
		query += fmt.Sprintf(" AND timestamp >= $%d", argIdx)
		args = append(args, f.Since)
		argIdx++
	}
	if !f.Until.IsZero() {
		query += fmt.Sprintf(" AND timestamp <= $%d", argIdx)
		args = append(args, f.Until)
		argIdx++
	}

	query += " ORDER BY timestamp DESC LIMIT 1000"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("pg audit log repo: query: %w", err)
	}
	defer rows.Close()

	var records []AuditLogRecord
	for rows.Next() {
		var rec AuditLogRecord
		if err := rows.Scan(&rec.ID, &rec.Timestamp, &rec.Action, &rec.Snapshot,
			&rec.Actor, &rec.Detail, &rec.Error); err != nil {
			return nil, fmt.Errorf("pg audit log repo: scan row: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg audit log repo: iterate rows: %w", err)
	}
	return records, nil
}

func (r *pgAuditLogRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("pg audit log repo: count: %w", err)
	}
	return count, nil
}
