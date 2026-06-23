# Implementation Plan: Core Interface Definitions (D-03)

**Branch**: `002-core-interface-definitions` | **Date**: 2026-06-22 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-core-interface-definitions/spec.md`

## Summary

Define the three core interface contracts that form the backbone of the Network Digital Twin's layered IR pipeline: **SchemaRegistry** (7 methods — ontology discovery and validation), **Connector** + **ConnectorRegistry** (3+3 methods — data source adaptation), and **GraphDB** (12 methods — graph database operations with logical multi-DB). Additionally, define minimal companion data structures (Resource, ConnectorMetadata, Node, Relation) in their respective packages to ensure the interfaces compile. The deliverable is interface-only code — zero implementation logic — that establishes the type contracts consumed by all downstream modules (Normalizer, GraphAssembler, SyncService, SnapshotManager, MCP Server).

## Technical Context

**Language/Version**: Go 1.26.1

**Primary Dependencies**: `gopkg.in/yaml.v3` (only external dep, already in go.mod); `context` and `errors` from standard library

**Storage**: N/A (interface definitions only — no storage logic in this task)

**Testing**: `go test ./...` with table-driven tests; mock implementations to verify interface satisfaction

**Target Platform**: Linux server (Docker container)

**Project Type**: Library / internal service (Go module: `gitlab.com/pml/network-digital-twin`)

**Performance Goals**: N/A for interface definitions (compile-time verification only)

**Constraints**: Interface-only files with zero implementation logic; `go build ./...` must pass; all method signatures must match architecture design docs

**Scale/Scope**: 3 interface files + 2 companion types files + sentinel errors; ~200 total lines of Go code

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|----------|
| **I. Layered IR Pipeline Integrity** | ✅ Pass | Interfaces enforce layer boundaries: SchemaRegistry (schema layer), Connector (data ingestion), GraphDB (storage). GraphDB references `assembler.Node`/`assembler.Relation` — the GraphModel IR — not schema types. |
| **II. Interface-First Design** | ✅ Pass | This task IS the interface-first definition. All cross-module dependencies defined through Go interfaces. GraphDB field will be typed as `graph.GraphDB` interface, not concrete Neo4j client. |
| **III. Schema-Driven Zero-Code Extension** | ✅ Pass | SchemaRegistry interface enables schema discovery — Load from YAML dir, Get/List/Validate. New entities require only YAML files, no code changes. |
| **IV. Test-First Development** | ✅ Pass | Interface definitions enable mock-based testing. TDD applies to implementations (D-03 deliverables are contracts, not behavior). |
| **V. Concurrent Safety via GraphLock** | ⬜ N/A | GraphLock is a separate concern (defined in snapshot package). Interface definitions don't include locking. |
| **VI. Error Handling Completeness** | ✅ Pass | Sentinel errors defined per-package: `ErrNotImplemented` (connector), `ErrSchemaNotFound` (schema). All error returns use `error` type. |

**Naming Convention Compliance**:
- SchemaRegistry uses `Get` prefix — documented exception in CLAUDE.md ("语义清晰，属于例外")
- ConnectorRegistry is a struct (not interface) — matches D-03 specification
- Package names: lowercase, single-word, matching directory names
- Receiver names: will use 1-2 char abbreviations in implementations

## Project Structure

### Documentation (this feature)

```text
specs/002-core-interface-definitions/
├── plan.md              # This file
├── research.md          # Phase 0: Technical decisions
├── data-model.md        # Phase 1: Type definitions across packages
├── quickstart.md        # Phase 1: Validation guide
├── contracts/           # Phase 1: Interface contracts
│   └── interfaces.md    # All 3 interface contracts with signatures
├── checklists/          # Spec quality checklist
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks - NOT created here)
```

### Source Code (repository root)

```text
internal/
├── schema/
│   ├── types.go         # EXISTS (D-01) — EntityType, RelationType, Metadata, PropertySpec
│   ├── errors.go        # EXISTS (D-01) — ErrInvalidSchema, ErrUnsupportedKind, ErrInvalidAPIVersion
│   │                    # MODIFY — add ErrSchemaNotFound
│   ├── loader.go        # EXISTS (D-01) — LoadFromFile, LoadFromDir
│   ├── registry.go      # MODIFY — add SchemaRegistry interface (7 methods)
│   └── validator.go     # NO CHANGE (implementation deferred to I-02)
├── connector/
│   ├── types.go         # MODIFY — add Resource, ConnectorMetadata structs
│   ├── interface.go     # MODIFY — add Connector interface (3 methods), ConnectorRegistry struct (3 methods), ErrNotImplemented
│   └── mock/
│       └── mock.go      # NO CHANGE (implementation deferred to I-03)
├── assembler/
│   ├── types.go         # MODIFY — add Node, Relation structs
│   └── assembler.go     # NO CHANGE (implementation deferred to I-05)
├── graph/
│   ├── interface.go     # MODIFY — add GraphDB interface (12 methods)
│   ├── neo4j.go         # NO CHANGE (implementation deferred to I-12)
│   └── logical_db.go    # NO CHANGE (implementation deferred to I-12)
└── ...                  # Other packages unchanged
```

**Structure Decision**: Follow existing package layout from CLAUDE.md. Interface definitions go into the pre-existing stub files. Companion types go into existing `types.go` files. No new directories or packages created.

## Complexity Tracking

> No constitution violations. All principles pass the gate check.
