# Tasks: Core Interface Definitions (D-03)

**Input**: Design documents from `specs/002-core-interface-definitions/`

**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅

**Tests**: Interface definitions only — no behavior to test. Compile-time interface satisfaction checks (`var _ Interface = (*Impl)(nil)`) deferred to implementation tasks (I-01, I-03, I-12). Quickstart validation scenarios verify correctness.

**Organization**: Tasks are grouped by user story. All three stories are P1 (co-equal foundational contracts). Phase 2 (companion types) is a blocking prerequisite for Phases 3–5.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Go module: `gitlab.com/pml/network-digital-twin`
- Source root: `internal/`
- All files are existing stubs that need content added

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Verify development environment is ready

- [x] T001 Verify `go build ./...` and `go test ./...` pass on current branch in repository root
- [x] T002 Verify stub files exist: `internal/schema/registry.go`, `internal/schema/errors.go`, `internal/connector/types.go`, `internal/connector/interface.go`, `internal/assembler/types.go`, `internal/graph/interface.go`

---

## Phase 2: Foundational (Companion Type Definitions)

**Purpose**: Define minimal data structures that interfaces reference — these MUST exist before any interface can compile

**⚠️ CRITICAL**: No interface definition work can begin until this phase is complete

- [x] T003 [P] Define `Node` struct (fields: `Label string`, `URI string`, `Props map[string]any`) and `Relation` struct (fields: `Type string`, `From string`, `To string`, `Props map[string]any`) in `internal/assembler/types.go`
- [x] T004 [P] Define `Resource` struct (fields: `Kind string`, `ID string`, `Properties map[string]any`) and `ConnectorMetadata` struct (fields: `Name string`, `Type string`, `EntityTypes []string`) in `internal/connector/types.go`

**Checkpoint**: `go build ./...` passes with new type definitions. Types are pure data structures with no logic.

---

## Phase 3: User Story 1 — Ontology Schema Discovery and Validation (Priority: P1) 🎯 MVP

**Goal**: Define the SchemaRegistry interface so downstream modules (Normalizer, GraphAssembler, Validator) have a typed contract for schema discovery and validation.

**Independent Test**: `go build ./...` passes. Interface has exactly 7 methods. `go vet ./...` reports no issues.

### Implementation for User Story 1

- [x] T005 [P] [US1] Add sentinel error `ErrSchemaNotFound = errors.New("schema not found")` to the existing `var` block in `internal/schema/errors.go`
- [x] T006 [US1] Define `SchemaRegistry` interface with 7 methods (`Load`, `GetEntityType`, `GetRelationType`, `ListEntityTypes`, `ListRelationTypes`, `Validate`, `ApplyDefaults`) in `internal/schema/registry.go` — see `contracts/interfaces.md` Contract 1 for exact signatures and doc comments

**Checkpoint**: SchemaRegistry interface compiles. Method count = 7. `go vet ./internal/schema/...` passes.

---

## Phase 4: User Story 2 — Data Source Adapter Contract (Priority: P1)

**Goal**: Define the Connector interface and ConnectorRegistry so the Sync Service has a typed contract for data source adapters.

**Independent Test**: `go build ./...` passes. Connector has 3 methods, ConnectorRegistry has 3 methods + constructor. `go vet ./...` reports no issues.

### Implementation for User Story 2

- [x] T007 [P] [US2] Define sentinel errors `ErrNotImplemented = errors.New("not implemented")` and `ErrConnectorNotFound = errors.New("connector not found")` in `internal/connector/interface.go`
- [x] T008 [US2] Define `Connector` interface with 3 methods (`Metadata`, `Collect`, `Stream`) in `internal/connector/interface.go` — see `contracts/interfaces.md` Contract 2 for exact signatures; `Stream` returns `<-chan Resource, error`; import `context` from stdlib
- [x] T009 [US2] Define `ConnectorRegistry` struct (unexported field `connectors map[string]Connector`), constructor `NewConnectorRegistry() *ConnectorRegistry`, and 3 methods (`Register`, `Get`, `List`) in `internal/connector/interface.go` — `Register` uses `c.Metadata().Name` as key; `Get` returns `ErrConnectorNotFound` on miss

**Checkpoint**: Connector + ConnectorRegistry compile. Method counts: Connector=3, ConnectorRegistry=3. `go vet ./internal/connector/...` passes.

---

## Phase 5: User Story 3 — Graph Database Driver Contract (Priority: P1)

**Goal**: Define the GraphDB interface so Sync Service, Snapshot Manager, and MCP Server have a typed contract for graph database operations with logical multi-DB support.

**Independent Test**: `go build ./...` passes. GraphDB has exactly 12 methods. 10 methods include `db string` parameter. `go vet ./...` reports no issues.

### Implementation for User Story 3

- [x] T010 [US3] Define `GraphDB` interface with 12 methods (`Ping`, `Close`, `BulkCreate`, `Upsert`, `DeleteRelations`, `DeleteByURIs`, `Query`, `BuildCypher`, `ClearDB`, `CloneDB`, `ListDBs`, `HasDB`) in `internal/graph/interface.go` — see `contracts/interfaces.md` Contract 3 for exact signatures; import `context` from stdlib and `assembler` from `gitlab.com/pml/network-digital-twin/internal/assembler`; add section comments (连接管理, 全量同步, 增量同步, 查询, 逻辑 DB 管理)

**Checkpoint**: GraphDB interface compiles. Method count = 12. `db string` present in 10 methods (all except `Ping`, `Close`, `ListDBs`). `go vet ./internal/graph/...` passes.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final verification that all deliverables meet success criteria

- [x] T011 Run `go build ./...` in repository root — verify zero output and exit code 0 (SC-001)
- [x] T012 [P] Run `go vet ./...` in repository root — verify zero output and exit code 0
- [x] T013 [P] Run `go test ./...` in repository root — verify existing tests pass (schema package from D-01); new packages show `[no test files]`
- [x] T014 Run quickstart.md validation scenarios: verify method counts (SC-003: 7, SC-004: 3, SC-005: 3, SC-006: 12), companion types exist (FR-013), `db string` parameter presence (SC-007), sentinel errors defined (FR-015), cross-package import (SC-009)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational — can start after Phase 2
- **User Story 2 (Phase 4)**: Depends on Foundational — can start after Phase 2
- **User Story 3 (Phase 5)**: Depends on Foundational — can start after Phase 2
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (SchemaRegistry)**: Independent — no dependencies on US2 or US3
- **User Story 2 (Connector)**: Independent — no dependencies on US1 or US3
- **User Story 3 (GraphDB)**: Independent — no dependencies on US1 or US2 (depends on assembler types from Phase 2)

**All three user stories can proceed in parallel once Phase 2 is complete.**

### Within Each User Story

- Sentinel errors before interface definitions (errors referenced by interfaces)
- Interface definition is the core task — single file, single write
- Checkpoint verification after each story

### Parallel Opportunities

- T003 and T004 (companion types) can run in parallel — different packages
- T005 and T006 (US1) can run in parallel — different files (`errors.go` vs `registry.go`)
- Phase 3, 4, 5 can all run in parallel — different packages, no cross-dependencies
- T012 and T013 (polish) can run in parallel

---

## Parallel Example: All User Stories

```bash
# Phase 2: Launch companion types in parallel
Task T003: "Define Node and Relation in internal/assembler/types.go"
Task T004: "Define Resource and ConnectorMetadata in internal/connector/types.go"

# Phase 3-5: Once Phase 2 done, launch all stories in parallel
Task T005+T006: "SchemaRegistry interface (US1)"
Task T007+T008+T009: "Connector + ConnectorRegistry (US2)"
Task T010: "GraphDB interface (US3)"
```

---

## Implementation Strategy

### MVP First (All Stories — All Are P1)

1. Complete Phase 1: Setup (verify environment)
2. Complete Phase 2: Foundational (companion types — CRITICAL, blocks all stories)
3. Complete Phases 3–5: All three user stories (can be parallel)
4. Complete Phase 6: Polish (build verification + quickstart validation)
5. **STOP and VALIDATE**: Run full quickstart.md — all 8 scenarios pass

### Incremental Delivery

1. Setup + Foundational → Types ready, interfaces can compile
2. Add SchemaRegistry (US1) → Schema contract available for Normalizer/Assembler
3. Add Connector (US2) → Data source contract available for SyncService
4. Add GraphDB (US3) → Graph contract available for SyncService/Snapshot/MCP
5. Each interface enables its downstream consumers to begin implementation

### Single Developer Sequential Order

T001 → T002 → T003 → T004 → T005 → T006 → T007 → T008 → T009 → T010 → T011 → T012 → T013 → T014

### Parallel Agent Strategy

With multiple agents:

1. Agent A: T001, T002 (setup verification)
2. Agent A: T003 + Agent B: T004 (companion types, parallel)
3. Agent A: T005+T006 + Agent B: T007+T008+T009 + Agent C: T010 (all stories, parallel)
4. Agent A: T011+T014 + Agent B: T012+T013 (polish, parallel)

---

## Notes

- [P] tasks = different files, no dependencies within the same phase
- [Story] label maps task to specific user story for traceability
- All files are existing stubs — tasks add content, not create files
- No implementation logic in any task — only type signatures, struct fields, and doc comments
- Sentinel errors use `errors.New()` with lowercase messages, no trailing period
- `Get` prefix in SchemaRegistry is a documented exception to the project's naming convention
- ConnectorRegistry is a struct (not interface) per D-03 specification
