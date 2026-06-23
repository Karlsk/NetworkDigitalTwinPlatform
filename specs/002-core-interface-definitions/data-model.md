# Data Model: Core Interface Definitions (D-03)

**Feature**: Core Interface Definitions | **Date**: 2026-06-22

## Overview

This data model documents all types across the four packages touched by D-03. Types are categorized as **Existing** (defined in D-01, no changes needed) or **New** (defined in D-03).

## Package: `internal/schema`

### Existing Types (from D-01, unchanged)

| Type | Kind | Purpose |
|------|------|---------|
| `EntityType` | struct | K8s CRD-style entity definition (APIVersion, Kind, Metadata, Spec) |
| `EntityTypeSpec` | struct | Entity specification (Identity, URITemplate, FieldMapping, Normalize, RelationFields, Properties) |
| `IdentitySpec` | struct | Immutable identity (StableKeys []string) |
| `NormalizeRule` | struct | Field-level string transform (Field, Pattern, Replace) |
| `RelationFieldSpec` | struct | Property-to-relation mapping (RelationType string) |
| `PropertySpec` | struct | Property definition (Type string, Required bool, Enum []string, Default any) |
| `RelationType` | struct | K8s CRD-style relation definition (APIVersion, Kind, Metadata, Spec) |
| `RelationTypeSpec` | struct | Relation source/target constraints (Source []string, Target []string) |
| `Metadata` | struct | Shared metadata (Name string, Labels []string) |

### Existing Errors (from D-01, extended)

| Error | Message | Status |
|-------|---------|--------|
| `ErrInvalidSchema` | `"invalid schema"` | Existing |
| `ErrUnsupportedKind` | `"unsupported kind"` | Existing |
| `ErrInvalidAPIVersion` | `"invalid api version"` | Existing |
| **`ErrSchemaNotFound`** | **`"schema not found"`** | **New in D-03** |

### New Interface: `SchemaRegistry`

| Method | Parameters | Returns | Purpose |
|--------|-----------|---------|---------|
| `Load` | `dir string` | `error` | Load all YAML ontology files from directory |
| `GetEntityType` | `name string` | `*EntityType, error` | Query EntityType by name |
| `GetRelationType` | `name string` | `*RelationType, error` | Query RelationType by name |
| `ListEntityTypes` | *(none)* | `[]*EntityType` | List all registered EntityTypes |
| `ListRelationTypes` | *(none)* | `[]*RelationType` | List all registered RelationTypes |
| `Validate` | `entityKind string, props map[string]any` | `error` | Check constraints without mutation |
| `ApplyDefaults` | `entityKind string, props map[string]any` | `map[string]any, error` | Return new map with defaults filled |

**File**: `internal/schema/registry.go`

---

## Package: `internal/connector`

### New Types (defined in D-03)

#### `Resource`

| Field | Type | Purpose |
|-------|------|---------|
| `Kind` | `string` | Entity type name (e.g., "Device", "Interface") |
| `ID` | `string` | Original ID from data source |
| `Properties` | `map[string]any` | Raw property key-value pairs |

**File**: `internal/connector/types.go`

#### `ConnectorMetadata`

| Field | Type | Purpose |
|-------|------|---------|
| `Name` | `string` | Connector name, used as registry key (e.g., "mock-netbox") |
| `Type` | `string` | Connector type category (e.g., "netbox", "controller", "mock") |
| `EntityTypes` | `[]string` | Supported entity types (e.g., ["Device", "Interface"]) |

**File**: `internal/connector/types.go`

### New Errors

| Error | Message | Used By |
|-------|---------|---------|
| `ErrNotImplemented` | `"not implemented"` | `Connector.Stream()` MVP |
| `ErrConnectorNotFound` | `"connector not found"` | `ConnectorRegistry.Get()` |

**File**: `internal/connector/interface.go`

### New Interface: `Connector`

| Method | Parameters | Returns | Purpose |
|--------|-----------|---------|---------|
| `Metadata` | *(none)* | `ConnectorMetadata` | Return connector identity and capabilities |
| `Collect` | `ctx context.Context, entityType string` | `[]Resource, error` | Full data pull for entity type |
| `Stream` | `ctx context.Context, entityType string` | `<-chan Resource, error` | Incremental streaming (MVP: ErrNotImplemented) |

**File**: `internal/connector/interface.go`

### New Struct: `ConnectorRegistry`

| Field | Type | Visibility | Purpose |
|-------|------|------------|---------|
| `connectors` | `map[string]Connector` | unexported | Internal storage keyed by Metadata().Name |

| Method | Parameters | Returns | Purpose |
|--------|-----------|---------|---------|
| `Register` | `c Connector` | *(none)* | Add connector to registry |
| `Get` | `name string` | `Connector, error` | Retrieve connector by name |
| `List` | *(none)* | `[]ConnectorMetadata` | List all registered connector metadata |

**Constructor**: `NewConnectorRegistry() *ConnectorRegistry`

**File**: `internal/connector/interface.go`

---

## Package: `internal/assembler`

### New Types (defined in D-03)

#### `Node`

| Field | Type | Purpose |
|-------|------|---------|
| `Label` | `string` | Graph node label (e.g., "Device", "Interface") |
| `URI` | `string` | Unique resource identifier from uriTemplate |
| `Props` | `map[string]any` | Node properties |

**File**: `internal/assembler/types.go`

#### `Relation`

| Field | Type | Purpose |
|-------|------|---------|
| `Type` | `string` | Relation type name (e.g., "HAS_INTERFACE", "RUNS_ON") |
| `From` | `string` | Source node URI |
| `To` | `string` | Target node URI |
| `Props` | `map[string]any` | Relation properties (usually empty) |

**File**: `internal/assembler/types.go`

---

## Package: `internal/graph`

### New Interface: `GraphDB`

| # | Method | Parameters | Returns | Category |
|---|--------|-----------|---------|----------|
| 1 | `Ping` | `ctx context.Context` | `error` | Connectivity |
| 2 | `Close` | *(none)* | `error` | Connectivity |
| 3 | `BulkCreate` | `ctx context.Context, db string, nodes []assembler.Node, rels []assembler.Relation` | `error` | Full sync |
| 4 | `Upsert` | `ctx context.Context, db string, nodes []assembler.Node, rels []assembler.Relation` | `error` | Incremental sync |
| 5 | `DeleteRelations` | `ctx context.Context, db string, rels []assembler.Relation` | `error` | Incremental sync |
| 6 | `DeleteByURIs` | `ctx context.Context, db string, uris []string` | `error` | Incremental sync |
| 7 | `Query` | `ctx context.Context, db string, cypher string, params map[string]any` | `[]map[string]any, error` | Query |
| 8 | `BuildCypher` | `action string, db string, nodes []assembler.Node, rels []assembler.Relation, uris []string` | `string, map[string]any` | Preview |
| 9 | `ClearDB` | `ctx context.Context, db string` | `error` | Logical DB mgmt |
| 10 | `CloneDB` | `ctx context.Context, from string, to string` | `error` | Logical DB mgmt |
| 11 | `ListDBs` | `ctx context.Context` | `[]string, error` | Logical DB mgmt |
| 12 | `HasDB` | `ctx context.Context, db string` | `bool, error` | Logical DB mgmt |

**`db string` parameter rule**: All methods except `Ping`, `Close`, and `ListDBs` accept a `db string` parameter for logical multi-database isolation.

**BuildCypher action values**: `"create"`, `"upsert"`, `"delete"`, `"delete_relations"`

**File**: `internal/graph/interface.go`

**Import**: `gitlab.com/pml/network-digital-twin/internal/assembler`

---

## Cross-Package Dependency Map

```
schema/registry.go     ─── no external imports (self-contained)
connector/types.go     ─── no external imports (pure data)
connector/interface.go ─── imports: context (stdlib), errors (stdlib)
assembler/types.go     ─── no external imports (pure data)
graph/interface.go     ─── imports: context (stdlib), assembler (Node, Relation)
```

**Dependency direction** (one-way, no cycles):
```
graph → assembler    (GraphDB uses Node, Relation)
connector → (nothing)
schema → (nothing)
assembler → (nothing)
```
