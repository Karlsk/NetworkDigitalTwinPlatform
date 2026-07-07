// Package repository 提供 PostgreSQL 连接池管理与 Schema 迁移工具。
// 作为 V2 元数据存储层的基础设施，负责：
//   - 连接池创建与健康检查（NewPGPool）
//   - Schema 迁移执行（RunMigrations）
//   - 迁移文件嵌入（//go:embed）
package repository

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx database/sql 驱动
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

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
