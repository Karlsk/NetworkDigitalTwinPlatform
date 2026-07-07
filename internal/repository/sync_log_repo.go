// Package repository 提供 SyncLogRepository 同步日志 CRUD。
// V2-07: 同步历史记录 + SyncService 集成。
package repository

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncLogRecord 同步日志记录。
type SyncLogRecord struct {
	ID               int64
	SyncType         string    // "full" / "incremental"
	Status           string    // "success" / "failed"
	NodesCreated     int
	RelationsCreated int
	OrphanEdges      int
	Warnings         []byte // JSON
	ErrorMessage     string
	StartedAt        time.Time
	CompletedAt      time.Time
	DurationMs       int64
}

// SyncLogRepository 同步日志 CRUD 接口。
type SyncLogRepository interface {
	// Create 创建同步日志记录，成功后回填 r.ID。
	Create(ctx context.Context, r SyncLogRecord) error
	// List 列出最近 limit 条同步日志，按 started_at DESC 排序。
	// limit <= 0 时返回全部。
	List(ctx context.Context, limit int) ([]SyncLogRecord, error)
	// ListByType 按 sync_type 过滤，按 started_at DESC 排序，最多返回 limit 条。
	// limit <= 0 时返回全部匹配记录。
	ListByType(ctx context.Context, syncType string, limit int) ([]SyncLogRecord, error)
	// Count 返回同步日志总数。
	Count(ctx context.Context) (int64, error)
}

// ---------------------------------------------------------------------------
// PostgreSQL 实现
// ---------------------------------------------------------------------------

type pgSyncLogRepo struct {
	pool *pgxpool.Pool
}

// NewPGSyncLogRepository 创建基于 PostgreSQL 的 SyncLogRepository。
func NewPGSyncLogRepository(pool *pgxpool.Pool) SyncLogRepository {
	return &pgSyncLogRepo{pool: pool}
}

func (r *pgSyncLogRepo) Create(ctx context.Context, rec SyncLogRecord) error {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO sync_logs (sync_type, status, nodes_created, relations_created,
		 orphan_edges, warnings, error_message, started_at, completed_at, duration_ms)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id`,
		rec.SyncType, rec.Status, rec.NodesCreated, rec.RelationsCreated,
		rec.OrphanEdges, rec.Warnings, rec.ErrorMessage, rec.StartedAt,
		rec.CompletedAt, rec.DurationMs,
	).Scan(&rec.ID)
	if err != nil {
		return fmt.Errorf("pg sync_log repo: create: %w", err)
	}
	return nil
}

func (r *pgSyncLogRepo) List(ctx context.Context, limit int) ([]SyncLogRecord, error) {
	var rows pgx.Rows
	var err error
	if limit > 0 {
		rows, err = r.pool.Query(ctx,
			`SELECT id, sync_type, status, nodes_created, relations_created,
			 orphan_edges, warnings, error_message, started_at, completed_at, duration_ms
			 FROM sync_logs ORDER BY started_at DESC LIMIT $1`, limit)
	} else {
		rows, err = r.pool.Query(ctx,
			`SELECT id, sync_type, status, nodes_created, relations_created,
			 orphan_edges, warnings, error_message, started_at, completed_at, duration_ms
			 FROM sync_logs ORDER BY started_at DESC`)
	}
	if err != nil {
		return nil, fmt.Errorf("pg sync_log repo: list: %w", err)
	}
	defer rows.Close()

	var records []SyncLogRecord
	for rows.Next() {
		var rec SyncLogRecord
		if err := rows.Scan(
			&rec.ID, &rec.SyncType, &rec.Status,
			&rec.NodesCreated, &rec.RelationsCreated, &rec.OrphanEdges,
			&rec.Warnings, &rec.ErrorMessage,
			&rec.StartedAt, &rec.CompletedAt, &rec.DurationMs,
		); err != nil {
			return nil, fmt.Errorf("pg sync_log repo: scan row: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg sync_log repo: iterate rows: %w", err)
	}
	return records, nil
}

func (r *pgSyncLogRepo) ListByType(ctx context.Context, syncType string, limit int) ([]SyncLogRecord, error) {
	var rows pgx.Rows
	var err error
	if limit > 0 {
		rows, err = r.pool.Query(ctx,
			`SELECT id, sync_type, status, nodes_created, relations_created,
			 orphan_edges, warnings, error_message, started_at, completed_at, duration_ms
			 FROM sync_logs WHERE sync_type = $1 ORDER BY started_at DESC LIMIT $2`,
			syncType, limit)
	} else {
		rows, err = r.pool.Query(ctx,
			`SELECT id, sync_type, status, nodes_created, relations_created,
			 orphan_edges, warnings, error_message, started_at, completed_at, duration_ms
			 FROM sync_logs WHERE sync_type = $1 ORDER BY started_at DESC`,
			syncType)
	}
	if err != nil {
		return nil, fmt.Errorf("pg sync_log repo: list by type %q: %w", syncType, err)
	}
	defer rows.Close()

	var records []SyncLogRecord
	for rows.Next() {
		var rec SyncLogRecord
		if err := rows.Scan(
			&rec.ID, &rec.SyncType, &rec.Status,
			&rec.NodesCreated, &rec.RelationsCreated, &rec.OrphanEdges,
			&rec.Warnings, &rec.ErrorMessage,
			&rec.StartedAt, &rec.CompletedAt, &rec.DurationMs,
		); err != nil {
			return nil, fmt.Errorf("pg sync_log repo: scan row: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg sync_log repo: iterate rows: %w", err)
	}
	return records, nil
}

func (r *pgSyncLogRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sync_logs`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("pg sync_log repo: count: %w", err)
	}
	return count, nil
}

// ---------------------------------------------------------------------------
// 内存 Fallback 实现（postgres.enabled: false 时使用）
// ---------------------------------------------------------------------------

type memSyncLogRepo struct {
	records []SyncLogRecord
	nextID  int64
	mu      sync.RWMutex
}

// NewMemSyncLogRepository 创建基于内存的 SyncLogRepository。
func NewMemSyncLogRepository() SyncLogRepository {
	return &memSyncLogRepo{}
}

func (r *memSyncLogRepo) Create(_ context.Context, rec SyncLogRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	rec.ID = r.nextID
	r.records = append(r.records, rec)
	return nil
}

func (r *memSyncLogRepo) List(_ context.Context, limit int) ([]SyncLogRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]SyncLogRecord, len(r.records))
	copy(result, r.records)

	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.After(result[j].StartedAt)
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *memSyncLogRepo) ListByType(_ context.Context, syncType string, limit int) ([]SyncLogRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []SyncLogRecord
	for _, rec := range r.records {
		if rec.SyncType == syncType {
			filtered = append(filtered, rec)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].StartedAt.After(filtered[j].StartedAt)
	})

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (r *memSyncLogRepo) Count(_ context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int64(len(r.records)), nil
}
