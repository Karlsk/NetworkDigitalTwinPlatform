// Package repository 提供 PostgreSQL 连接池与迁移工具。
package repository

import (
	"context"
	"testing"
)

// TestPGConfigDefaults 验证 PGConfig 零值行为。
func TestPGConfigDefaults(t *testing.T) {
	var cfg PGConfig

	if cfg.URL != "" {
		t.Errorf("PGConfig.URL zero value = %q, want empty", cfg.URL)
	}
	if cfg.MaxConns != 0 {
		t.Errorf("PGConfig.MaxConns zero value = %d, want 0", cfg.MaxConns)
	}
	if cfg.MinConns != 0 {
		t.Errorf("PGConfig.MinConns zero value = %d, want 0", cfg.MinConns)
	}
}

// TestPGConstructors 验证所有 PG Repository 构造函数接受 nil pool 不 panic。
func TestPGConstructors(t *testing.T) {
	t.Run("NewPGSnapshotRepository", func(t *testing.T) {
		repo := NewPGSnapshotRepository(nil)
		if repo == nil {
			t.Fatal("NewPGSnapshotRepository(nil) returned nil")
		}
	})
	t.Run("NewPGConnectorRepository", func(t *testing.T) {
		repo := NewPGConnectorRepository(nil)
		if repo == nil {
			t.Fatal("NewPGConnectorRepository(nil) returned nil")
		}
	})
	t.Run("NewPGAuditLogRepository", func(t *testing.T) {
		repo := NewPGAuditLogRepository(nil)
		if repo == nil {
			t.Fatal("NewPGAuditLogRepository(nil) returned nil")
		}
	})
	t.Run("NewPGSyncLogRepository", func(t *testing.T) {
		repo := NewPGSyncLogRepository(nil)
		if repo == nil {
			t.Fatal("NewPGSyncLogRepository(nil) returned nil")
		}
	})
	t.Run("NewPGSchemaVersionRepository", func(t *testing.T) {
		repo := NewPGSchemaVersionRepository(nil)
		if repo == nil {
			t.Fatal("NewPGSchemaVersionRepository(nil) returned nil")
		}
	})
}

// TestNewPGPoolInvalidURL 验证无效 URL 返回 error（不依赖 Docker）。
func TestNewPGPoolInvalidURL(t *testing.T) {
	ctx := context.Background()
	_, err := NewPGPool(ctx, PGConfig{
		URL: "not-a-valid-pg-url",
	})
	if err == nil {
		t.Fatal("NewPGPool() with invalid URL should return error")
	}
}

// TestNewPGPoolUnreachableHost 验证连接失败返回 error。
func TestNewPGPoolUnreachableHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消，加速失败

	_, err := NewPGPool(ctx, PGConfig{
		URL: "postgres://user:pass@localhost:59999/nonexistent?sslmode=disable",
	})
	if err == nil {
		t.Fatal("NewPGPool() with canceled context should return error")
	}
}

// TestMigrationsFSEmbedded 验证嵌入的迁移文件可被读取。
func TestMigrationsFSEmbedded(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir migrations: %v", err)
	}

	if len(entries) < 2 {
		t.Errorf("expected at least 2 migration files, got %d", len(entries))
	}

	// 验证 up 和 down 文件都存在
	foundUp, foundDown := false, false
	for _, e := range entries {
		if e.Name() == "000001_init.up.sql" {
			foundUp = true
		}
		if e.Name() == "000001_init.down.sql" {
			foundDown = true
		}
	}
	if !foundUp {
		t.Error("missing 000001_init.up.sql in embedded migrations")
	}
	if !foundDown {
		t.Error("missing 000001_init.down.sql in embedded migrations")
	}
}
