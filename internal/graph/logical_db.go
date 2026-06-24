package graph

import (
	"context"
	"fmt"
	"log/slog"
)

// ensureDBReady 确保指定逻辑 DB 处于干净可用状态（清空旧数据）。
// 用于全量同步前的准备工作。
func ensureDBReady(ctx context.Context, client GraphDB, db string) error {
	slog.Info("ensuring db ready", "db", db)
	if err := client.ClearDB(ctx, db); err != nil {
		return fmt.Errorf("ensure db ready %q: %w", db, err)
	}
	return nil
}

// cleanStaleDBs 清理不在 keepDBs 列表中的过期逻辑 DB。
// "default" DB 永远不会被清理。
func cleanStaleDBs(ctx context.Context, client GraphDB, keepDBs map[string]bool) error {
	allDBs, err := client.ListDBs(ctx)
	if err != nil {
		return fmt.Errorf("clean stale dbs: %w", err)
	}
	for _, db := range allDBs {
		if db == "default" {
			continue // 永远不清理 default
		}
		if !keepDBs[db] {
			slog.Info("cleaning stale db", "db", db)
			if err := client.ClearDB(ctx, db); err != nil {
				return fmt.Errorf("clean stale db %q: %w", db, err)
			}
		}
	}
	return nil
}
