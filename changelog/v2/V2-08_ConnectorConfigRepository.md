# V2-08: ConnectorConfigRepository + Schema 版本追踪

**工时**: 1 天
**前置**: V2-05
**风险等级**: 低
**Phase**: Phase 2 — PostgreSQL 元数据存储

---

## 背景

V1 现状：连接器配置存于 `configs/connectors.yaml` 文件，运行时状态（last_ping、健康状态）无持久化。
Schema 版本无追踪，本体变更后无法回溯历史。

V2 目标：连接器配置 + 运行时状态持久化到 PostgreSQL，Schema 版本追踪。

---

## 实现步骤

### Step 1: ConnectorConfigRepository

新建 `internal/repository/connector_repo.go`：

```go
package repository

// ConnectorConfigRecord 连接器配置记录。
type ConnectorConfigRecord struct {
    ID          int64
    Name        string
    Type        string   // "mock" / "netbox" / "controller" / "cmdb"
    Config      []byte   // JSON
    EntityTypes []byte   // JSON array
    Priority    int
    Status      string   // "active" / "disabled" / "error"
    LastPing    *time.Time
}

// ConnectorConfigRepository 连接器配置 CRUD。
type ConnectorConfigRepository interface {
    Upsert(ctx context.Context, r ConnectorConfigRecord) error
    GetByName(ctx context.Context, name string) (*ConnectorConfigRecord, error)
    List(ctx context.Context) ([]ConnectorConfigRecord, error)
    UpdateStatus(ctx context.Context, name, status string) error
    UpdateLastPing(ctx context.Context, name string, t time.Time) error
    Delete(ctx context.Context, name string) error
}
```

### Step 2: SchemaVersionRepository

```go
// SchemaVersionRecord Schema 版本记录。
type SchemaVersionRecord struct {
    ID            int64
    Version       int
    EntityTypes   []byte   // JSON
    RelationTypes []byte   // JSON
    AppliedAt     time.Time
    Description   string
}

// SchemaVersionRepository Schema 版本追踪。
type SchemaVersionRepository interface {
    Create(ctx context.Context, r SchemaVersionRecord) error
    Latest(ctx context.Context) (*SchemaVersionRecord, error)
    List(ctx context.Context) ([]SchemaVersionRecord, error)
}
```

### Step 3: 集成到启动流程

修改 `cmd/server/main.go`：

```go
// 启动时同步 connectors.yaml 到 PostgreSQL
if connRepo != nil {
    for _, meta := range connRegistry.List() {
        connRepo.Upsert(ctx, repository.ConnectorConfigRecord{
            Name: meta.Name, Type: meta.Type,
            Status: "active",
        })
    }
}

// Schema 版本追踪
if schemaRepo != nil {
    latest, _ := schemaRepo.Latest(ctx)
    currentVersion := computeSchemaVersion(reg)
    if latest == nil || latest.Version < currentVersion {
        schemaRepo.Create(ctx, repository.SchemaVersionRecord{
            Version:     currentVersion,
            EntityTypes: marshalEntityTypes(reg),
            Description: "auto-detected schema change",
        })
    }
}
```

### Step 4: 单元测试

| 测试 | 验证点 |
|------|--------|
| `TestPGConnectorUpsert` | 创建/更新成功 |
| `TestPGConnectorList` | 列表返回所有连接器 |
| `TestPGConnectorUpdateStatus` | 状态更新正确 |
| `TestPGSchemaVersionCreate` | 版本记录创建 |
| `TestPGSchemaVersionLatest` | 返回最新版本 |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/repository/connector_repo.go` | 新增 | 接口 + PG + 内存实现 |
| `internal/repository/schema_version_repo.go` | 新增 | 接口 + PG + 内存实现 |
| `internal/repository/connector_repo_test.go` | 新增 | CRUD 单元测试 |
| `cmd/server/main.go` | 修改 | 启动时同步配置 + Schema 版本检测 |

---

## 验收标准

- [ ] ConnectorConfigRepository CRUD 正确
- [ ] SchemaVersionRepository 版本追踪正确
- [ ] 启动时连接器配置同步到 PostgreSQL
- [ ] Schema 变更时自动记录新版本
- [ ] `go test ./internal/repository/...` 全部通过
