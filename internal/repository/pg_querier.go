// Package repository 提供 PostgreSQL 连接池管理与 Schema 迁移工具。
// pg_querier.go 定义内部 PG 查询抽象接口，提升可测试性。
package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// pgQuerier 抽象 PG 查询操作，*pgxpool.Pool 和 pgx.Tx 均满足此接口。
// 引入此接口使 PG repo 可以在不依赖真实数据库的情况下进行单元测试。
type pgQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}
