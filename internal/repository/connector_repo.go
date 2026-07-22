// Package repository 提供 ConnectorConfigRepository 连接器配置 CRUD。
// V2-08: 连接器配置 + 运行时状态持久化到 PostgreSQL。
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

// ErrConnectorConfigNotFound 连接器配置不存在。
var ErrConnectorConfigNotFound = errors.New("connector config not found")

// ConnectorConfigRecord 连接器配置记录。
type ConnectorConfigRecord struct {
	ID          int64
	Name        string
	Type        string // "mock" / "netbox" / "controller" / "cmdb"
	Config      []byte // JSON
	EntityTypes []byte // JSON array
	Priority    int
	Status      string // "active" / "disabled" / "error"
	LastPing    *time.Time
}

// ConnectorConfigRepository 连接器配置 CRUD。
type ConnectorConfigRepository interface {
	// Upsert 创建或更新连接器配置，按 name 去重。
	Upsert(ctx context.Context, r ConnectorConfigRecord) error
	// GetByName 按名称查找连接器配置，不存在返回 ErrConnectorConfigNotFound。
	GetByName(ctx context.Context, name string) (*ConnectorConfigRecord, error)
	// List 列出所有连接器配置，按 name ASC 排序。
	List(ctx context.Context) ([]ConnectorConfigRecord, error)
	// UpdateStatus 更新连接器状态，不存在返回 ErrConnectorConfigNotFound。
	UpdateStatus(ctx context.Context, name, status string) error
	// UpdateLastPing 更新最后 Ping 时间，不存在返回 ErrConnectorConfigNotFound。
	UpdateLastPing(ctx context.Context, name string, t time.Time) error
	// Delete 按名称删除连接器配置，不存在返回 ErrConnectorConfigNotFound。
	Delete(ctx context.Context, name string) error
}

// ---------------------------------------------------------------------------
// PostgreSQL 实现
// ---------------------------------------------------------------------------

type pgConnectorConfigRepo struct {
	db pgQuerier
}

// NewPGConnectorRepository 创建基于 PostgreSQL 的 ConnectorConfigRepository。
func NewPGConnectorRepository(pool *pgxpool.Pool) ConnectorConfigRepository {
	return &pgConnectorConfigRepo{db: pool}
}

func (r *pgConnectorConfigRepo) Upsert(ctx context.Context, rec ConnectorConfigRecord) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO connector_configs (name, type, config, entity_types, priority, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (name) DO UPDATE SET
		   type = EXCLUDED.type,
		   config = EXCLUDED.config,
		   entity_types = EXCLUDED.entity_types,
		   priority = EXCLUDED.priority,
		   status = EXCLUDED.status,
		   updated_at = NOW()`,
		rec.Name, rec.Type, rec.Config, rec.EntityTypes, rec.Priority, rec.Status,
	)
	if err != nil {
		return fmt.Errorf("pg connector repo: upsert %q: %w", rec.Name, err)
	}
	return nil
}

func (r *pgConnectorConfigRepo) GetByName(ctx context.Context, name string) (*ConnectorConfigRecord, error) {
	rec := &ConnectorConfigRecord{}
	err := r.db.QueryRow(ctx,
		`SELECT id, name, type, config, entity_types, priority, status, last_ping
		 FROM connector_configs WHERE name = $1`, name,
	).Scan(&rec.ID, &rec.Name, &rec.Type, &rec.Config, &rec.EntityTypes,
		&rec.Priority, &rec.Status, &rec.LastPing)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrConnectorConfigNotFound
		}
		return nil, fmt.Errorf("pg connector repo: get by name %q: %w", name, err)
	}
	return rec, nil
}

func (r *pgConnectorConfigRepo) List(ctx context.Context) ([]ConnectorConfigRecord, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, name, type, config, entity_types, priority, status, last_ping
		 FROM connector_configs ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("pg connector repo: list: %w", err)
	}
	defer rows.Close()

	var records []ConnectorConfigRecord
	for rows.Next() {
		var rec ConnectorConfigRecord
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Type, &rec.Config,
			&rec.EntityTypes, &rec.Priority, &rec.Status, &rec.LastPing); err != nil {
			return nil, fmt.Errorf("pg connector repo: scan row: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg connector repo: iterate rows: %w", err)
	}
	return records, nil
}

func (r *pgConnectorConfigRepo) UpdateStatus(ctx context.Context, name, status string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE connector_configs SET status = $2, updated_at = NOW() WHERE name = $1`,
		name, status)
	if err != nil {
		return fmt.Errorf("pg connector repo: update status %q: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConnectorConfigNotFound
	}
	return nil
}

func (r *pgConnectorConfigRepo) UpdateLastPing(ctx context.Context, name string, t time.Time) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE connector_configs SET last_ping = $2, updated_at = NOW() WHERE name = $1`,
		name, t)
	if err != nil {
		return fmt.Errorf("pg connector repo: update last_ping %q: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConnectorConfigNotFound
	}
	return nil
}

func (r *pgConnectorConfigRepo) Delete(ctx context.Context, name string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM connector_configs WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("pg connector repo: delete %q: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConnectorConfigNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// 内存 Fallback 实现（postgres.enabled: false 时使用）
// ---------------------------------------------------------------------------

type memConnectorConfigRepo struct {
	records map[string]ConnectorConfigRecord
	nextID  int64
	mu      sync.RWMutex
}

// NewMemConnectorRepository 创建基于内存的 ConnectorConfigRepository。
func NewMemConnectorRepository() ConnectorConfigRepository {
	return &memConnectorConfigRepo{records: make(map[string]ConnectorConfigRecord)}
}

func (r *memConnectorConfigRepo) Upsert(_ context.Context, rec ConnectorConfigRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.records[rec.Name]; ok {
		rec.ID = existing.ID
	} else {
		r.nextID++
		rec.ID = r.nextID
	}
	r.records[rec.Name] = rec
	return nil
}

func (r *memConnectorConfigRepo) GetByName(_ context.Context, name string) (*ConnectorConfigRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.records[name]
	if !ok {
		return nil, ErrConnectorConfigNotFound
	}
	result := rec
	return &result, nil
}

func (r *memConnectorConfigRepo) List(_ context.Context) ([]ConnectorConfigRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ConnectorConfigRecord, 0, len(r.records))
	for _, rec := range r.records {
		result = append(result, rec)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (r *memConnectorConfigRepo) UpdateStatus(_ context.Context, name, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.records[name]
	if !ok {
		return ErrConnectorConfigNotFound
	}
	rec.Status = status
	r.records[name] = rec
	return nil
}

func (r *memConnectorConfigRepo) UpdateLastPing(_ context.Context, name string, t time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.records[name]
	if !ok {
		return ErrConnectorConfigNotFound
	}
	rec.LastPing = &t
	r.records[name] = rec
	return nil
}

func (r *memConnectorConfigRepo) Delete(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.records[name]; !ok {
		return ErrConnectorConfigNotFound
	}
	delete(r.records, name)
	return nil
}
