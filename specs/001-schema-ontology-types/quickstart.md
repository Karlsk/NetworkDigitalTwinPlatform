# Quickstart: Schema Data Structures and Ontology YAML Definitions

**Feature**: `001-schema-ontology-types` | **Date**: 2026-06-15

## Prerequisites

- Go 1.21+ installed
- `gopkg.in/yaml.v3` dependency added (`go get gopkg.in/yaml.v3`)
- `ontology/` directory with 7 YAML files present

## Validation Scenarios

### 1. Verify Type Definitions Compile

```bash
go build ./internal/schema/...
```

**Expected**: Clean compilation, no errors.

### 2. Run Unit Tests

```bash
go test -v ./internal/schema/...
```

**Expected**: All tests pass, including:
- EntityType YAML string → struct parsing
- RelationType YAML string → struct parsing
- Multi-document YAML parsing (4 RelationTypes from `relations.yaml`)
- Error cases (invalid YAML, unsupported kind, wrong apiVersion)
- `LoadFromFile` with temp YAML files
- `LoadFromDir` with temp directory containing mixed entity/relation files

### 3. Verify Coverage

```bash
go test -coverprofile=coverage.out ./internal/schema/...
go tool cover -func=coverage.out
```

**Expected**: ≥80% line coverage across `types.go`, `loader.go`, `errors.go`.

### 4. Load Actual Ontology Directory

Write a quick integration test or use Go's `-run` flag:

```bash
go test -v -run TestLoadOntologyDir ./internal/schema/...
```

**Expected**:
- 6 EntityType records loaded (Device, Interface, ISIS, Link, Network_Slice, Alarm)
- 4 RelationType records loaded (HAS_INTERFACE, RUNS_ON, ENDPOINT, OCCURRED_ON)
- No parse errors

### 5. Verify Property Counts

After loading, check field counts match the spec:

| EntityType | Expected Properties |
|------------|-------------------|
| Device | 8 |
| Interface | 5 |
| ISIS | 5 |
| Link | 4 |
| Network_Slice | 4 |
| Alarm | 4 |

### 6. Verify stableKeys are required: true

For each EntityType, verify that every field listed in `identity.stableKeys` has `required: true` in `properties`.

### 7. Lint Check

```bash
golangci-lint run ./internal/schema/...
```

**Expected**: No lint errors or warnings.

## Quick Smoke Test (manual)

```go
// In a scratch file or test:
entityTypes, relationTypes, err := schema.LoadFromDir("ontology")
// Verify: len(entityTypes) == 6, len(relationTypes) == 4, err == nil
```
