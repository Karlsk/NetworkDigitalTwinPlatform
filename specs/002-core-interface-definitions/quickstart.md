# Quickstart Validation: Core Interface Definitions (D-03)

**Feature**: Core Interface Definitions | **Date**: 2026-06-22

This guide provides runnable validation scenarios that prove the D-03 interface contracts are correctly defined and compilable.

## Prerequisites

- Go 1.26.1 installed
- Repository cloned and on branch `002-core-interface-definitions`
- No external services required (interface definitions only)

## Validation 1: Build Verification (SC-001)

Verify all interface files compile without errors.

```bash
go build ./...
```

**Expected**: Zero output, exit code 0.

## Validation 2: Interface Method Counts (SC-003, SC-004, SC-005, SC-006)

Verify each interface has the correct number of methods.

```bash
# SchemaRegistry: expect 7 methods
grep -c '^\s\+[A-Z]' internal/schema/registry.go

# Connector: expect 3 methods
grep -c '^\s\+[A-Z]' internal/connector/interface.go | head -1

# GraphDB: expect 12 methods
grep -c '^\s\+[A-Z]' internal/graph/interface.go
```

**Expected**: SchemaRegistry=7, Connector=3, GraphDB=12. ConnectorRegistry=3 (Register, Get, List) + constructor (NewConnectorRegistry).

## Validation 3: Companion Types Exist (SC-008, FR-013)

Verify all companion types are defined.

```bash
# Node and Relation in assembler
grep 'type Node struct' internal/assembler/types.go
grep 'type Relation struct' internal/assembler/types.go

# Resource and ConnectorMetadata in connector
grep 'type Resource struct' internal/connector/types.go
grep 'type ConnectorMetadata struct' internal/connector/types.go
```

**Expected**: Each grep returns a matching line.

## Validation 4: GraphDB db Parameter (SC-007)

Verify all write/query methods include `db string` parameter.

```bash
grep 'db string' internal/graph/interface.go
```

**Expected**: 10 methods contain `db string` (BulkCreate, Upsert, DeleteRelations, DeleteByURIs, Query, BuildCypher, ClearDB, CloneDB×2[from,to], HasDB). Ping, Close, ListDBs do not.

## Validation 5: Sentinel Errors Defined (FR-015)

```bash
# Schema package
grep 'ErrSchemaNotFound' internal/schema/errors.go

# Connector package
grep 'ErrNotImplemented' internal/connector/interface.go
grep 'ErrConnectorNotFound' internal/connector/interface.go
```

**Expected**: Each grep returns a matching line.

## Validation 6: Cross-Package Import (SC-009)

Verify graph/interface.go imports assembler for Node/Relation types.

```bash
grep 'assembler' internal/graph/interface.go
```

**Expected**: Import line `"gitlab.com/pml/network-digital-twin/internal/assembler"` and references to `assembler.Node` and `assembler.Relation`.

## Validation 7: Vet and Lint

```bash
go vet ./...
```

**Expected**: Zero output, exit code 0. No vet warnings for interface-only files.

## Validation 8: Existing Tests Still Pass

```bash
go test ./...
```

**Expected**: All existing tests pass (schema package tests from D-01). New packages show `[no test files]`.

## Quick Reference

| Artifact | Path |
|----------|------|
| Implementation Plan | [plan.md](./plan.md) |
| Research Decisions | [research.md](./research.md) |
| Data Model | [data-model.md](./data-model.md) |
| Interface Contracts | [contracts/interfaces.md](./contracts/interfaces.md) |
| Feature Specification | [spec.md](./spec.md) |
