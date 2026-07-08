// Package repository 提供 SchemaVersionRepository Schema 版本追踪。
// V2-08: 本体变更后记录版本历史，支持回溯。
package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSchemaVersionNotFound Schema 版本记录不存在。
var ErrSchemaVersionNotFound = errors.New("schema version not found")

// SchemaVersionRecord Schema 版本记录。
type SchemaVersionRecord struct {
	ID            int64
	Version       int
	EntityTypes   []byte // JSON
	RelationTypes []byte // JSON
	AppliedAt     time.Time
	Description   string
}

// SchemaVersionRepository Schema 版本追踪。
type SchemaVersionRepository interface {
	// Create 创建版本记录，成功后回填 rec.ID。
	Create(ctx context.Context, rec *SchemaVersionRecord) error
	// Latest 返回最新版本记录，空表返回 ErrSchemaVersionNotFound。
	Latest(ctx context.Context) (*SchemaVersionRecord, error)
	// List 列出所有版本记录，按 version DESC 排序。
	List(ctx context.Context) ([]SchemaVersionRecord, error)
}

// ---------------------------------------------------------------------------
// PostgreSQL 实现
// ---------------------------------------------------------------------------

type pgSchemaVersionRepo struct {
	pool *pgxpool.Pool
}

// NewPGSchemaVersionRepository 创建基于 PostgreSQL 的 SchemaVersionRepository。
func NewPGSchemaVersionRepository(pool *pgxpool.Pool) SchemaVersionRepository {
	return &pgSchemaVersionRepo{pool: pool}
}

func (r *pgSchemaVersionRepo) Create(ctx context.Context, rec *SchemaVersionRecord) error {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO schema_versions (version, entity_types, relation_types, applied_at, description)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		rec.Version, rec.EntityTypes, rec.RelationTypes, rec.AppliedAt, rec.Description,
	).Scan(&rec.ID)
	if err != nil {
		return fmt.Errorf("pg schema_version repo: create version %d: %w", rec.Version, err)
	}
	return nil
}

func (r *pgSchemaVersionRepo) Latest(ctx context.Context) (*SchemaVersionRecord, error) {
	rec := &SchemaVersionRecord{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, version, entity_types, relation_types, applied_at, description
		 FROM schema_versions ORDER BY version DESC LIMIT 1`,
	).Scan(&rec.ID, &rec.Version, &rec.EntityTypes, &rec.RelationTypes,
		&rec.AppliedAt, &rec.Description)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSchemaVersionNotFound
		}
		return nil, fmt.Errorf("pg schema_version repo: latest: %w", err)
	}
	return rec, nil
}

func (r *pgSchemaVersionRepo) List(ctx context.Context) ([]SchemaVersionRecord, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, version, entity_types, relation_types, applied_at, description
		 FROM schema_versions ORDER BY version DESC`)
	if err != nil {
		return nil, fmt.Errorf("pg schema_version repo: list: %w", err)
	}
	defer rows.Close()

	var records []SchemaVersionRecord
	for rows.Next() {
		var rec SchemaVersionRecord
		if err := rows.Scan(&rec.ID, &rec.Version, &rec.EntityTypes,
			&rec.RelationTypes, &rec.AppliedAt, &rec.Description); err != nil {
			return nil, fmt.Errorf("pg schema_version repo: scan row: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg schema_version repo: iterate rows: %w", err)
	}
	return records, nil
}

// ---------------------------------------------------------------------------
// 内存 Fallback 实现（postgres.enabled: false 时使用）
// ---------------------------------------------------------------------------

type memSchemaVersionRepo struct {
	records []SchemaVersionRecord
	nextID  int64
	mu      sync.RWMutex
}

// NewMemSchemaVersionRepository 创建基于内存的 SchemaVersionRepository。
func NewMemSchemaVersionRepository() SchemaVersionRepository {
	return &memSchemaVersionRepo{}
}

func (r *memSchemaVersionRepo) Create(_ context.Context, rec *SchemaVersionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	rec.ID = r.nextID
	if rec.AppliedAt.IsZero() {
		rec.AppliedAt = time.Now()
	}
	r.records = append(r.records, *rec)
	return nil
}

func (r *memSchemaVersionRepo) Latest(_ context.Context) (*SchemaVersionRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.records) == 0 {
		return nil, ErrSchemaVersionNotFound
	}

	var latest *SchemaVersionRecord
	for i := range r.records {
		if latest == nil || r.records[i].Version > latest.Version {
			rec := r.records[i]
			latest = &rec
		}
	}
	return latest, nil
}

func (r *memSchemaVersionRepo) List(_ context.Context) ([]SchemaVersionRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]SchemaVersionRecord, len(r.records))
	copy(result, r.records)

	sort.Slice(result, func(i, j int) bool {
		return result[i].Version > result[j].Version
	})
	return result, nil
}
