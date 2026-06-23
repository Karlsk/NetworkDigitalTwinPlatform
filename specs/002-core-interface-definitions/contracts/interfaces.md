# Interface Contracts: Core Interface Definitions (D-03)

**Feature**: Core Interface Definitions | **Date**: 2026-06-22

This document defines the exact Go interface signatures that constitute the D-03 deliverables. Each contract includes the file path, package, method signatures, and behavioral notes.

---

## Contract 1: SchemaRegistry

**File**: `internal/schema/registry.go`
**Package**: `schema`

### Interface Definition

```go
package schema

// SchemaRegistry 本体注册表接口
// 系统启动时加载 ontology/ 目录下所有 YAML，提供 EntityType 和 RelationType 的查询能力
type SchemaRegistry interface {
    // Load 加载目录下所有 YAML 文件（支持多文档 YAML）
    Load(dir string) error

    // GetEntityType 按名称获取 EntityType
    GetEntityType(name string) (*EntityType, error)

    // GetRelationType 按名称获取 RelationType
    GetRelationType(name string) (*RelationType, error)

    // ListEntityTypes 列出所有 EntityType
    ListEntityTypes() []*EntityType

    // ListRelationTypes 列出所有 RelationType
    ListRelationTypes() []*RelationType

    // Validate 校验数据合法性（属性类型/必填/枚举/stableKeys 非空）
    // 不修改输入 map，只返回校验错误
    Validate(entityKind string, props map[string]any) error

    // ApplyDefaults 返回一个新 map，将 schema 中定义的默认值填充到缺失的可选字段
    // 原始 map 不被修改
    ApplyDefaults(entityKind string, props map[string]any) (map[string]any, error)
}
```

### Error Contract

| Condition | Error |
|-----------|-------|
| EntityType not found by name | `ErrSchemaNotFound` |
| RelationType not found by name | `ErrSchemaNotFound` |
| Unknown entityKind in Validate | `ErrSchemaNotFound` |
| Unknown entityKind in ApplyDefaults | `ErrSchemaNotFound` |
| Validation failures (multiple) | Aggregated error with `"; "` separator |

### Behavioral Notes

- `Load`: Delegates to existing `LoadFromDir` logic (from D-01 loader.go). Supports multi-document YAML via `---` separators.
- `GetEntityType` / `GetRelationType`: O(1) map lookup by name. Return `ErrSchemaNotFound` if key missing.
- `ListEntityTypes` / `ListRelationTypes`: Return all registered types. Order not guaranteed (map iteration).
- `Validate`: Read-only. Checks: required fields, data types, enum constraints, stableKeys non-emptiness. Does NOT fill defaults.
- `ApplyDefaults`: Returns a **new** `map[string]any` copy with defaults filled. Original map unchanged. Returns `ErrSchemaNotFound` if entityKind unknown.

---

## Contract 2: Connector + ConnectorRegistry

**File**: `internal/connector/interface.go`
**Package**: `connector`

### Companion Types (in `internal/connector/types.go`)

```go
package connector

// Resource 是 Connector 产出的原始数据单元
type Resource struct {
    Kind       string         // 实体类型，如 "Device"
    ID         string         // 原始 ID（数据源内部 ID）
    Properties map[string]any // 原始属性键值对
}

// ConnectorMetadata 描述连接器的身份和能力
type ConnectorMetadata struct {
    Name        string   // 连接器名称，如 "mock-netbox"
    Type        string   // 连接器类型，如 "netbox", "controller", "mock"
    EntityTypes []string // 支持的实体类型列表
}
```

### Interface Definition

```go
package connector

import "context"

// Connector 数据源适配器接口
// 每个数据源（Netbox/Controller/CMDB）实现一个 Connector
type Connector interface {
    // Metadata 返回连接器元信息
    Metadata() ConnectorMetadata

    // Collect 全量拉取指定实体类型的数据
    Collect(ctx context.Context, entityType string) ([]Resource, error)

    // Stream 流式推送增量变更（MVP 返回 ErrNotImplemented，V1 接入 Kafka）
    Stream(ctx context.Context, entityType string) (<-chan Resource, error)
}
```

### ConnectorRegistry Struct

```go
// ConnectorRegistry 连接器注册中心
type ConnectorRegistry struct {
    connectors map[string]Connector
}

// NewConnectorRegistry 创建空的连接器注册中心
func NewConnectorRegistry() *ConnectorRegistry

// Register 注册连接器，以 Metadata().Name 为 key
func (r *ConnectorRegistry) Register(c Connector)

// Get 按名称获取连接器
func (r *ConnectorRegistry) Get(name string) (Connector, error)

// List 列出所有已注册连接器的元信息
func (r *ConnectorRegistry) List() []ConnectorMetadata
```

### Sentinel Errors

```go
var (
    ErrNotImplemented    = errors.New("not implemented")
    ErrConnectorNotFound = errors.New("connector not found")
)
```

### Error Contract

| Condition | Error |
|-----------|-------|
| Stream not supported (MVP) | `fmt.Errorf("stream not implemented: %w", ErrNotImplemented)` |
| Connector name not found in registry | `ErrConnectorNotFound` |
| Collect timeout / unreachable | Wrapped error with context via `fmt.Errorf` |

### Behavioral Notes

- `Metadata()`: No error return — metadata is always available.
- `Collect(ctx, entityType)`: Returns `[]Resource` for the requested type. Empty slice is valid (zero resources).
- `Stream(ctx, entityType)`: MVP returns `(nil, ErrNotImplemented)`. V1 returns a `<-chan Resource` fed by Kafka.
- `Register(c)`: Uses `c.Metadata().Name` as map key. Duplicate names overwrite (last-write-wins).
- `Get(name)`: Returns `ErrConnectorNotFound` if name not in map.
- `List()`: Returns `[]ConnectorMetadata` for all registered connectors. Empty slice if registry is empty.

---

## Contract 3: GraphDB

**File**: `internal/graph/interface.go`
**Package**: `graph`

### Companion Types (in `internal/assembler/types.go`)

```go
package assembler

// Node 图节点（GraphModel IR）
type Node struct {
    Label string         // 节点标签，如 "Device"
    URI   string         // 唯一资源标识符
    Props map[string]any // 节点属性
}

// Relation 图边（GraphModel IR）
type Relation struct {
    Type  string         // 关系类型，如 "HAS_INTERFACE"
    From  string         // 源节点 URI
    To    string         // 目标节点 URI
    Props map[string]any // 关系属性（通常为空）
}
```

### Interface Definition

```go
package graph

import (
    "context"

    "gitlab.com/pml/network-digital-twin/internal/assembler"
)

// GraphDB 图数据库驱动接口
// 所有操作带 db 参数（逻辑多 DB），Cypher 生成内聚在实现中
type GraphDB interface {
    // === 连接管理 ===

    // Ping 检查连接
    Ping(ctx context.Context) error

    // Close 关闭连接
    Close() error

    // === 全量同步 ===

    // BulkCreate 批量 CREATE（先 ClearDB 再 BulkCreate）
    BulkCreate(ctx context.Context, db string, nodes []assembler.Node, rels []assembler.Relation) error

    // === 增量同步 ===

    // Upsert MERGE 节点 + SET += 属性增量合并 + MERGE 关系
    Upsert(ctx context.Context, db string, nodes []assembler.Node, rels []assembler.Relation) error

    // DeleteRelations 仅删除指定关系
    DeleteRelations(ctx context.Context, db string, rels []assembler.Relation) error

    // DeleteByURIs 按 URI 删除节点 + DETACH DELETE 关联关系
    DeleteByURIs(ctx context.Context, db string, uris []string) error

    // === 查询 ===

    // Query 执行 Cypher 查询（驱动层自动注入 $_db）
    Query(ctx context.Context, db string, cypher string, params map[string]any) ([]map[string]any, error)

    // BuildCypher 预览生成的 Cypher 语句（不执行），用于测试/audit/调试
    // action: "create", "upsert", "delete", "delete_relations"
    BuildCypher(action string, db string, nodes []assembler.Node, rels []assembler.Relation, uris []string) (string, map[string]any)

    // === 逻辑 DB 管理 ===

    // ClearDB 清空指定逻辑 DB 的所有数据
    ClearDB(ctx context.Context, db string) error

    // CloneDB 将一个逻辑 DB 完整复制到另一个
    CloneDB(ctx context.Context, from, to string) error

    // ListDBs 列出所有逻辑 DB
    ListDBs(ctx context.Context) ([]string, error)

    // HasDB 判断逻辑 DB 是否存在数据
    HasDB(ctx context.Context, db string) (bool, error)
}
```

### `db string` Parameter Map

| Method | Has `db`? | Rationale |
|--------|-----------|-----------|
| `Ping` | ❌ | Connection-level, not DB-scoped |
| `Close` | ❌ | Connection-level |
| `BulkCreate` | ✅ | Write to specific logical DB |
| `Upsert` | ✅ | Write to specific logical DB |
| `DeleteRelations` | ✅ | Write to specific logical DB |
| `DeleteByURIs` | ✅ | Write to specific logical DB |
| `Query` | ✅ | Read from specific logical DB |
| `BuildCypher` | ✅ | Preview includes `_db` filter |
| `ClearDB` | ✅ | Clears specific logical DB |
| `CloneDB` | ✅ | Two DB params: `from`, `to` |
| `ListDBs` | ❌ | Lists all DBs |
| `HasDB` | ✅ | Checks specific logical DB |

### BuildCypher Action Values

| Action | Generates Cypher For | Equivalent Method |
|--------|---------------------|-------------------|
| `"create"` | CREATE nodes and relations | `BulkCreate` |
| `"upsert"` | MERGE + SET += properties | `Upsert` |
| `"delete"` | DETACH DELETE by URI | `DeleteByURIs` |
| `"delete_relations"` | DELETE relations only | `DeleteRelations` |

### Behavioral Notes

- All methods with `db string` will have `params["_db"] = db` injected by the driver layer.
- `BuildCypher` is a pure function — no side effects, no DB access.
- `Query` returns `[]map[string]any` — each map is a row of results.
- `Close` returns error for cleanup failures (e.g., connection pool drain).
- `ListDBs` returns all logical DB names that contain data (have at least one node with that `_db` value).
- `HasDB` returns `(false, nil)` if DB has no data — not an error condition.
