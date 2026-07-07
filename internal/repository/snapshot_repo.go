// Package repository 提供 PostgreSQL 连接池管理与 Schema 迁移工具。
// 作为 V2 元数据存储层的基础设施，负责：
//   - 连接池创建与健康检查（NewPGPool）
//   - Schema 迁移执行（RunMigrations）
//   - 迁移文件嵌入（//go:embed）
//   - SnapshotRepository 快照元数据 CRUD（V2-06）
package repository

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx database/sql 驱动
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrSnapshotNotFound 快照不存在。
var ErrSnapshotNotFound = errors.New("snapshot not found")

// SnapshotRecord 快照数据库记录。
type SnapshotRecord struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	NodeCount int
	RelCount  int
	FilePath  string
	Status    string
}

// SnapshotRepository 快照元数据 CRUD 接口。
type SnapshotRepository interface {
	// Create 创建快照记录，成功后回填 rec.ID。
	Create(ctx context.Context, rec *SnapshotRecord) error
	// GetByName 按名称查找快照，不存在返回 ErrSnapshotNotFound。
	GetByName(ctx context.Context, name string) (*SnapshotRecord, error)
	// List 列出所有快照，按 created_at DESC 排序。
	List(ctx context.Context) ([]SnapshotRecord, error)
	// Delete 按名称删除快照。
	Delete(ctx context.Context, name string) error
	// UpdateStatus 更新快照状态。
	UpdateStatus(ctx context.Context, name, status string) error
}

// ---------------------------------------------------------------------------
// PostgreSQL 实现
// ---------------------------------------------------------------------------

type pgSnapshotRepo struct {
	pool *pgxpool.Pool
}

// NewPGSnapshotRepository 创建基于 PostgreSQL 的 SnapshotRepository。
func NewPGSnapshotRepository(pool *pgxpool.Pool) SnapshotRepository {
	return &pgSnapshotRepo{pool: pool}
}

func (r *pgSnapshotRepo) Create(ctx context.Context, rec *SnapshotRecord) error {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO snapshots (name, created_at, node_count, rel_count, file_path, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		rec.Name, rec.CreatedAt, rec.NodeCount, rec.RelCount, rec.FilePath, rec.Status,
	).Scan(&rec.ID)
	if err != nil {
		return fmt.Errorf("pg snapshot repo: create %q: %w", rec.Name, err)
	}
	return nil
}

func (r *pgSnapshotRepo) GetByName(ctx context.Context, name string) (*SnapshotRecord, error) {
	rec := &SnapshotRecord{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, created_at, node_count, rel_count, file_path, status
		 FROM snapshots WHERE name = $1`, name,
	).Scan(&rec.ID, &rec.Name, &rec.CreatedAt, &rec.NodeCount, &rec.RelCount, &rec.FilePath, &rec.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("pg snapshot repo: get by name %q: %w", name, err)
	}
	return rec, nil
}

func (r *pgSnapshotRepo) List(ctx context.Context) ([]SnapshotRecord, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, created_at, node_count, rel_count, file_path, status
		 FROM snapshots ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("pg snapshot repo: list: %w", err)
	}
	defer rows.Close()

	var records []SnapshotRecord
	for rows.Next() {
		var rec SnapshotRecord
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.CreatedAt, &rec.NodeCount, &rec.RelCount, &rec.FilePath, &rec.Status); err != nil {
			return nil, fmt.Errorf("pg snapshot repo: scan row: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pg snapshot repo: iterate rows: %w", err)
	}
	return records, nil
}

func (r *pgSnapshotRepo) Delete(ctx context.Context, name string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM snapshots WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("pg snapshot repo: delete %q: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSnapshotNotFound
	}
	return nil
}

func (r *pgSnapshotRepo) UpdateStatus(ctx context.Context, name, status string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE snapshots SET status = $2 WHERE name = $1`, name, status)
	if err != nil {
		return fmt.Errorf("pg snapshot repo: update status %q: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSnapshotNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// 内存 Fallback 实现（postgres.enabled: false 时使用）
// ---------------------------------------------------------------------------

type memSnapshotRepository struct {
	records map[string]SnapshotRecord
	nextID  int64
	mu      sync.RWMutex
}

// NewMemSnapshotRepository 创建基于内存的 SnapshotRepository。
func NewMemSnapshotRepository() SnapshotRepository {
	return &memSnapshotRepository{records: make(map[string]SnapshotRecord)}
}

func (r *memSnapshotRepository) Create(_ context.Context, rec *SnapshotRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.records[rec.Name]; exists {
		return fmt.Errorf("mem snapshot repo: duplicate name %q", rec.Name)
	}

	r.nextID++
	rec.ID = r.nextID
	r.records[rec.Name] = *rec
	return nil
}

func (r *memSnapshotRepository) GetByName(_ context.Context, name string) (*SnapshotRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.records[name]
	if !ok {
		return nil, ErrSnapshotNotFound
	}
	result := rec // 返回副本
	return &result, nil
}

func (r *memSnapshotRepository) List(_ context.Context) ([]SnapshotRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]SnapshotRecord, 0, len(r.records))
	for _, rec := range r.records {
		result = append(result, rec)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func (r *memSnapshotRepository) Delete(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.records[name]; !exists {
		return ErrSnapshotNotFound
	}
	delete(r.records, name)
	return nil
}

func (r *memSnapshotRepository) UpdateStatus(_ context.Context, name, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.records[name]
	if !ok {
		return ErrSnapshotNotFound
	}
	rec.Status = status
	r.records[name] = rec
	return nil
}

// ---------------------------------------------------------------------------
// PGConfig / NewPGPool / RunMigrations（V2-05 保留）
// ---------------------------------------------------------------------------

// PGConfig PostgreSQL 连接池配置。
// URL 格式: postgres://user:pass@host:port/db?sslmode=disable
// MaxConns 和 MinConns 为零值时使用 pgxpool 默认值。
type PGConfig struct {
	URL      string // postgres://user:pass@host:port/db?sslmode=disable
	MaxConns int32  // 最大连接数，0 = pgxpool 默认
	MinConns int32  // 最小连接数，0 = pgxpool 默认
}

// NewPGPool 创建 PostgreSQL 连接池。
// 创建后立即 Ping 验证连通性，失败时关闭已创建的连接池并返回 error。
func NewPGPool(ctx context.Context, cfg PGConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse pg config: %w", err)
	}

	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pg pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping pg: %w", err)
	}

	slog.Info("postgresql connected",
		"max_conns", poolCfg.MaxConns,
		"min_conns", poolCfg.MinConns,
	)
	return pool, nil
}

// RunMigrations 执行数据库迁移。
// 使用嵌入的 SQL 文件（migrationsFS）和 golang-migrate 执行 up 迁移。
// 重复执行时幂等安全（已有表使用 IF NOT EXISTS，golang-migrate 跟踪版本号）。
func RunMigrations(pool *pgxpool.Pool) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	// golang-migrate 的 postgres 驱动需要 database/sql 连接。
	// 从连接池配置中提取连接字符串，打开独立的 sql.DB 用于迁移。
	connStr := pool.Config().ConnString()
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("open sql db for migration: %w", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}

	slog.Info("database migrations completed")
	return nil
}
