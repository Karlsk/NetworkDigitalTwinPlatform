# Research: Core Interface Definitions (D-03)

**Feature**: Core Interface Definitions | **Date**: 2026-06-22

## Research Summary

This feature defines interface contracts only — no implementation logic, no external integrations, no performance-critical paths. Research focused on Go interface design patterns, cross-package dependency resolution, sentinel error placement conventions, and immutability patterns for map-returning methods.

## R-001: Companion Types Scope Resolution

**Decision**: Define minimal companion types (Resource, ConnectorMetadata, Node, Relation) within D-03 scope.

**Rationale**: The GraphDB interface references `assembler.Node` and `assembler.Relation`; the Connector interface references `connector.Resource` and `connector.ConnectorMetadata`. Without these types, `go build ./...` fails (SC-001). D-04 lists D-03 as prerequisite, creating a soft circular dependency. Defining the types in D-03 breaks the cycle.

**Alternatives considered**:
- *Build tags / temporary aliases*: Adds complexity, violates "no implementation logic" principle. Rejected.
- *Reorder tasks (D-04 before D-03)*: Breaks the dependency chain specified in the architecture docs. D-04 includes GraphAssembler logic that depends on GraphDB interface (from D-03). Rejected.

## R-002: Sentinel Error Placement Strategy

**Decision**: Each package defines its own sentinel errors. Schema package adds `ErrSchemaNotFound`. Connector package defines `ErrNotImplemented` and `ErrConnectorNotFound`.

**Rationale**: Standard Go convention — `errors.Is()` checks require the error to be importable from the package that returns it. Consumers do `errors.Is(err, schema.ErrSchemaNotFound)`, not `errors.Is(err, common.ErrSchemaNotFound)`. CLAUDE.md §10 groups all sentinel errors together for documentation purposes, but implementation places them in their respective packages.

**Alternatives considered**:
- *Centralized errors package*: Creates unnecessary import dependency. Every package would need to import `internal/errors`. Rejected — not idiomatic Go.
- *Error variables in interface file*: Mixing error definitions with interface declarations. Rejected — errors belong in `errors.go` per existing convention (`schema/errors.go`).

**Sentinel error inventory for D-03**:

| Package | Error | Message | Used By |
|---------|-------|---------|---------|
| `schema` | `ErrSchemaNotFound` | `"schema not found"` | `SchemaRegistry.GetEntityType()`, `GetRelationType()`, `Validate()`, `ApplyDefaults()` |
| `connector` | `ErrNotImplemented` | `"not implemented"` | `Connector.Stream()` MVP return |
| `connector` | `ErrConnectorNotFound` | `"connector not found"` | `ConnectorRegistry.Get()` |

Note: `ErrEventQueueFull` and `ErrDBNotExists` (mentioned in CLAUDE.md) are deferred — they belong to `service` and `graph` packages respectively, which have no interface definitions in D-03.

## R-003: SchemaRegistry.Validate Immutability Pattern

**Decision**: Separate `Validate` (check-only, returns error) from `ApplyDefaults` (returns new map). This was clarified in Q2.

**Rationale**: The project's coding standards (CLAUDE.md / common/coding-style.md) mandate immutability. `ApplyDefaults` returns `(map[string]any, error)` — a new map with defaults filled, original unchanged. The error return handles the case where `entityKind` is unknown (returns `ErrSchemaNotFound`).

**Validate method behavior** (check-only, no mutation):
1. Look up EntityType by `entityKind` → error if not found
2. Check required fields present and non-empty
3. Check data types match PropertySpec.Type
4. Check enum values if PropertySpec.Enum is non-empty
5. Check stableKeys fields are non-empty
6. Return aggregated error (all failures joined) or nil

**ApplyDefaults method behavior** (returns new map):
1. Look up EntityType by `entityKind` → error if not found
2. Copy input map to new map
3. For each PropertySpec with non-nil Default: if key missing from copy, set copy[key] = Default
4. Return (newMap, nil)

## R-004: Cross-Package Import Pattern (graph → assembler)

**Decision**: `internal/graph/interface.go` imports `internal/assembler` for `Node` and `Relation` types.

**Rationale**: The architecture mandates that GraphDB receives only GraphModel data — it is schema-agnostic. The `assembler` package owns the GraphModel IR types. The dependency direction is one-way: `graph → assembler`. The `assembler` package has zero dependency on `graph`.

**Import path**: `gitlab.com/pml/network-digital-twin/internal/assembler`

**Dependency graph for D-03 files**:

```
internal/graph/interface.go
  └── imports: internal/assembler (Node, Relation)
  └── imports: context (standard library)

internal/connector/interface.go
  └── imports: context (standard library)
  └── imports: errors (standard library)

internal/schema/registry.go
  └── imports: (none — all types are in same package)

internal/assembler/types.go
  └── imports: (none — pure data structures)

internal/connector/types.go
  └── imports: (none — pure data structures)
```

## R-005: ConnectorRegistry Constructor Pattern

**Decision**: Define `NewConnectorRegistry() *ConnectorRegistry` constructor function.

**Rationale**: CLAUDE.md §20 mandates `NewXxx` constructor pattern. The `ConnectorRegistry` struct has a `connectors map[string]Connector` field that must be initialized. Without a constructor, callers would get a nil map panic on `Register()`.

**Implementation**: `NewConnectorRegistry` returns `&ConnectorRegistry{connectors: make(map[string]Connector)}`.

**Note**: D-03 changelog doesn't explicitly show this constructor, but it is required by the project's coding conventions and for correct map initialization.

## R-006: BuildCypher Action Parameter Design

**Decision**: Use `string` type for the `action` parameter with 4 documented values: `"create"`, `"upsert"`, `"delete"`, `"delete_relations"`.

**Rationale**: D-03 explicitly specifies string-based action parameter ("不用枚举，保持简单"). Go doesn't have enum types — the idiomatic alternatives (typed constants or iota) would add complexity for a preview-only method. The string approach is consistent with the D-03 design principle of simplicity.

**Validation**: The implementation (deferred to I-12) should return empty string and nil params for unknown action values.

## R-007: Connector.Stream Return Type

**Decision**: `<-chan Resource` (receive-only channel) as the return type.

**Rationale**: Go's channel direction types enforce that consumers can only receive, not send. This is the standard Go pattern for producer-consumer streaming. For MVP, the mock implementation returns `(nil, ErrNotImplemented)`. V1 will return a buffered channel fed by Kafka consumer.

**Error handling**: When streaming is not implemented, return `(nil, fmt.Errorf("stream not implemented: %w", ErrNotImplemented))`. The nil channel signals "no data available"; the error signals "capability not available". Consumers should check `errors.Is(err, ErrNotImplemented)` before attempting to read from the channel.
