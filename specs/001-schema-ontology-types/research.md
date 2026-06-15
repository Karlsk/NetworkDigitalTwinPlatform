# Research: Schema Data Structures and Ontology YAML Definitions

**Feature**: `001-schema-ontology-types` | **Date**: 2026-06-15

## R-001: YAML Library Selection

**Decision**: `gopkg.in/yaml.v3`

**Rationale**:
- Already specified in CLAUDE.md (`Schema 解析: gopkg.in/yaml.v3`)
- Native support for multi-document YAML via `yaml.Decoder` with loop-decode pattern
- `yaml.v3` provides `yaml.Node` for advanced parsing if needed later
- Struct tags (`yaml:"fieldName"`) already used in `internal/config/config.go`
- Zero external transitive dependencies

**Alternatives considered**:
- `github.com/go-yaml/yaml` — older fork, less maintained
- Manual YAML parsing — unnecessary complexity for MVP
- `encoding/json` — doesn't support YAML format; ontology files must be human-readable YAML

## R-002: Multi-Document YAML Parsing Pattern

**Decision**: Use `yaml.NewDecoder` + loop `Decode()` until `io.EOF`

**Rationale**:
- `relations.yaml` contains 4 RelationType documents separated by `---`
- `yaml.Unmarshal` only parses the first document — insufficient
- `yaml.NewDecoder(reader).Decode(&doc)` in a loop handles all documents
- Trailing `---` with empty content returns `io.EOF` naturally — no special handling needed

**Implementation pattern**:
```
decoder := yaml.NewDecoder(file)
for {
    var doc RelationType
    err := decoder.Decode(&doc)
    if err == io.EOF { break }
    if err != nil { return error }
    results = append(results, doc)
}
```

**Alternatives considered**:
- Split file by `---` then unmarshal each — fragile, doesn't handle edge cases
- Use `yaml.v3` Node API — overkill for structured documents with known schema

## R-003: Kind Discrimination in LoadFromFile

**Decision**: Two-pass approach — first decode into a lightweight "kind probe" struct, then decode into the full type

**Rationale**:
- A single YAML file can be either `EntityType` or `RelationType`
- Need to know the `kind` field before choosing the target struct
- First pass: decode only `{ kind: string }` to determine type
- Second pass: decode into full `EntityType` or `RelationType` struct
- Avoids `interface{}` returns and type assertions in the public API

**Alternatives considered**:
- Union type with both fields — wastes memory, confusing API
- Return `any` and let caller type-assert — violates Interface-First principle
- Separate `LoadEntityType` / `LoadRelationType` functions — requires caller to know kind beforehand

## R-004: File Organization

**Decision**: 3 source files — `types.go` (structs), `loader.go` (parsing), `errors.go` (sentinel errors)

**Rationale**:
- `types.go` contains only data structures — pure definitions, no I/O
- `loader.go` contains file I/O and YAML parsing — single responsibility
- `errors.go` defines sentinel errors — enables `errors.Is()` checks across packages
- Each file stays under 200 lines — high cohesion, easy navigation
- Test files colocated: `types_test.go`, `loader_test.go`

**Alternatives considered**:
- Single `types.go` with everything — exceeds 400 lines with YAML content
- Separate package for loader (`internal/schema/loader/`) — over-engineering for 2 functions

## R-005: Test Strategy

**Decision**: Table-driven unit tests with inline YAML strings + integration test loading from `ontology/` directory

**Rationale**:
- Table-driven tests are the Go standard for parameterized testing
- Inline YAML strings in test cases provide fast feedback — no file I/O needed for unit tests
- Integration test loads actual `ontology/` directory to verify real YAML files parse correctly
- Build tag `integration` separates Neo4j-dependent tests (not needed here, but consistent with project pattern)
- Golden files in `testdata/golden/` for Device EntityType structure validation

**Test structure**:
- `types_test.go`: YAML string → struct parsing, field validation, edge cases
- `loader_test.go`: `LoadFromFile` with temp files, `LoadFromDir` with temp directory, error propagation

**Alternatives considered**:
- External YAML test fixtures only — slower, harder to debug
- No table-driven tests — more boilerplate, harder to add cases

## R-006: PropertySpec.Default Type Handling

**Decision**: Use `any` (Go interface{}) for Default field, accept any YAML scalar

**Rationale**:
- D-02 specifies `Default any` with `yaml:"default"` tag
- YAML examples show string defaults (`"Up"`, `"L1L2"`) — no int/bool defaults in current ontology
- `yaml.v3` naturally unmarshals YAML scalars into `any` as `string`, `int`, `float64`, or `bool`
- FR-014 requires support for string, integer, boolean defaults
- Downstream consumers (Normalizer) will type-check against `PropertySpec.Type` at runtime

**Alternatives considered**:
- `json.RawMessage` — JSON-specific, doesn't apply to YAML
- Separate typed fields (`DefaultString`, `DefaultInt`) — verbose, doesn't scale
- Custom unmarshaler — unnecessary complexity for MVP
