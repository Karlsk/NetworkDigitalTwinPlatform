// Command migrate-data 将 YAML 快照元数据批量迁移到 PostgreSQL。
//
// 用法:
//
//	go run cmd/migrate-data/main.go \
//	  --snap-dir snapshots \
//	  --pg-url "postgres://twin:twin@localhost:5432/twin?sslmode=disable"
//	  --dry-run
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/repository"
	"gitlab.com/pml/network-digital-twin/internal/snapshot"
)

// MigrateStats 迁移统计。
type MigrateStats struct {
	Scanned int // 扫描到的 YAML 文件数
	Created int // 成功写入 PG 数
	Skipped int // 已存在跳过数
	Failed  int // 失败数
}

func main() {
	snapDir := flag.String("snap-dir", "snapshots", "快照目录路径")
	pgURL := flag.String("pg-url", "", "PostgreSQL 连接 URL（必填）")
	dryRun := flag.Bool("dry-run", false, "只扫描不写入，输出迁移统计")
	flag.Parse()

	if *pgURL == "" && !*dryRun {
		fmt.Fprintln(os.Stderr, "error: --pg-url is required (or use --dry-run)")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 连接 PG + 执行迁移（仅非 dry-run 模式）
	var repo repository.SnapshotRepository
	if !*dryRun {
		pool, err := repository.NewPGPool(ctx, repository.PGConfig{
			URL:      *pgURL,
			MaxConns: 5,
			MinConns: 1,
		})
		if err != nil {
			slog.Error("connect postgres", "error", err)
			os.Exit(1)
		}
		defer pool.Close()

		if err := repository.RunMigrations(pool); err != nil {
			slog.Error("run migrations", "error", err)
			os.Exit(1)
		}

		repo = repository.NewPGSnapshotRepository(pool)
	}

	// 2. 扫描快照目录
	stats, err := MigrateSnapshots(ctx, *snapDir, *dryRun, repo)
	if err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}

	// 3. 输出统计
	if *dryRun {
		fmt.Println("=== Dry Run (no data written) ===")
	} else {
		fmt.Println("=== Migration Complete ===")
	}
	fmt.Printf("Scanned: %d\n", stats.Scanned)
	fmt.Printf("Created: %d\n", stats.Created)
	fmt.Printf("Skipped: %d\n", stats.Skipped)
	fmt.Printf("Failed:  %d\n", stats.Failed)
}

// MigrateSnapshots 扫描 snapDir 下所有 .yaml 文件，解析元数据并写入 PG。
// dryRun=true 时只统计不写入。
func MigrateSnapshots(ctx context.Context, snapDir string, dryRun bool, repo repository.SnapshotRepository) (*MigrateStats, error) {
	stats := &MigrateStats{}

	err := filepath.Walk(snapDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".yaml") {
			return nil
		}

		stats.Scanned++

		// 解析元数据
		meta, parseErr := snapshot.ImportMetaOnly(path)
		if parseErr != nil {
			slog.Warn("skip unparseable yaml", "file", path, "error", parseErr)
			stats.Failed++
			return nil
		}

		if dryRun {
			fmt.Printf("[dry-run] would create: name=%s, nodes=%d, rels=%d, file=%s\n",
				meta.Name, meta.NodeCount, meta.RelCount, path)
			stats.Created++
			return nil
		}

		// 写入 PG
		rec := &repository.SnapshotRecord{
			Name:      meta.Name,
			CreatedAt: meta.CreatedAt,
			NodeCount: meta.NodeCount,
			RelCount:  meta.RelCount,
			FilePath:  meta.FilePath,
			Status:    "active",
		}
		if createErr := repo.Create(ctx, rec); createErr != nil {
			// UNIQUE 冲突 → 跳过
			if isDuplicateError(createErr) {
				slog.Info("skip existing snapshot", "name", meta.Name)
				stats.Skipped++
				return nil
			}
			slog.Error("create snapshot record", "name", meta.Name, "error", createErr)
			stats.Failed++
			return nil
		}

		slog.Info("migrated snapshot", "name", meta.Name, "nodes", meta.NodeCount, "rels", meta.RelCount)
		stats.Created++
		return nil
	})

	if err != nil {
		return stats, fmt.Errorf("walk snap dir %s: %w", snapDir, err)
	}

	return stats, nil
}

// isDuplicateError 判断是否为 UNIQUE 约束冲突错误。
// PostgreSQL UNIQUE violation SQLSTATE = 23505。
func isDuplicateError(err error) bool {
	// pgx 返回的错误消息包含 "duplicate key" 关键字
	return err != nil && (strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "23505"))
}
