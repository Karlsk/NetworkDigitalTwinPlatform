# Implementation Plan: Schema Data Structures and Ontology YAML Definitions

**Branch**: `001-schema-ontology-types` | **Date**: 2026-06-15 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/001-schema-ontology-types/spec.md`

## Summary

Define all Go struct types for the network ontology schema (EntityType, RelationType, PropertySpec, etc.) in K8s CRD style, create 7 YAML ontology definition files (6 EntityTypes + 1 multi-document RelationType file), and implement basic YAML parsing helpers (`LoadFromFile`, `LoadFromDir`). This is the foundational data layer that all downstream pipeline modules depend on.

## Technical Context

**Language/Version**: Go 1.21+ (go.mod declares 1.26.1)

**Primary Dependencies**: `gopkg.in/yaml.v3` (to be added via `go get`)

**Storage**: File-based YAML ontology files in `ontology/` directory — no database involved

**Testing**: `go test` (standard library), table-driven tests, `testdata/` fixtures

**Target Platform**: Linux server (Docker deployment via docker-compose)

**Project Type**: Internal library (`internal/schema/` package)

**Performance Goals**: Schema loading < 1 second for all 7 files combined (SC-006)

**Constraints**: No external dependencies beyond YAML library; no runtime hot-reload; single-threaded at startup

**Scale/Scope**: 7 YAML files, 6 EntityTypes (~30 properties total), 4 RelationTypes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Layered IR Pipeline Integrity | ✅ PASS | Schema types define contracts consumed by Normalizer/GraphAssembler — no pipeline logic, pure data definitions |
| II. Interface-First Design | ✅ PASS | No cross-module dependencies in this feature; parsing functions are self-contained |
| III. Schema-Driven Zero-Code Extension | ✅ PASS | This feature IS the enabler — YAML files define ontology, no code changes needed to add entities |
| IV. Test-First Development | ✅ PASS | TDD enforced: tests written before types and YAML files; ≥80% coverage target |
| V. Concurrent Safety via GraphLock | ⬜ N/A | Schema loading is single-threaded at startup; no concurrency concerns |
| VI. Error Handling Completeness | ✅ PASS | `LoadFromFile`/`LoadFromDir` wrap errors with `fmt.Errorf` + `%w`; sentinel errors for parse failures |

**Code Quality Gates**:
- Package name: `schema` (lowercase, matches directory) ✅
- File organization: `types.go` for structs, `loader.go` for parsing helpers ✅
- Receiver naming: N/A (no method receivers on data types)
- Error messages: lowercase, no periods, wrapped with context ✅
- Structured logging: `log/slog` for load diagnostics ✅
- Dependency injection: parsing functions accept directory path as parameter ✅

**Post-Phase 1 Re-check**: All principles still pass after design. No violations.

## Project Structure

### Documentation (this feature)

```text
specs/001-schema-ontology-types/
├── plan.md              # This file
├── research.md          # Phase 0: research decisions
├── data-model.md        # Phase 1: entity & type model
├── quickstart.md        # Phase 1: validation guide
├── contracts/           # Phase 1: public API contracts
│   └── loader.md        # LoadFromFile / LoadFromDir signatures
├── checklists/          # Spec quality checklists
│   └── requirements.md
├── spec.md              # Feature specification
└── tasks.md             # Phase 2 output (/speckit-tasks - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/schema/
├── types.go             # EntityType, RelationType, PropertySpec, Metadata, etc.
├── loader.go            # LoadFromFile, LoadFromDir parsing helpers
├── errors.go            # Sentinel errors (ErrInvalidSchema, ErrUnsupportedKind, etc.)
├── types_test.go        # Unit tests for type parsing (table-driven)
└── loader_test.go       # Unit tests for LoadFromFile, LoadFromDir

ontology/
├── device.yaml          # Device EntityType (8 properties)
├── interface.yaml       # Interface EntityType (5 properties)
├── isis.yaml            # ISIS EntityType (5 properties)
├── link.yaml            # Link EntityType (4 properties)
├── network_slice.yaml   # Network_Slice EntityType (4 properties)
├── alarm.yaml           # Alarm EntityType (4 properties)
└── relations.yaml       # 4 RelationTypes (multi-document YAML)
```

**Structure Decision**: Single internal package (`internal/schema/`) with 3 source files + 2 test files. Ontology YAML files in top-level `ontology/` directory. Separation: `types.go` for data structures, `loader.go` for I/O, `errors.go` for sentinel errors. Each source file stays under 200 lines.

## Complexity Tracking

No constitution violations. No complexity justification needed.
