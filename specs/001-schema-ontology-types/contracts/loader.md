# Contract: Schema Loader API

**Package**: `internal/schema`
**Feature**: `001-schema-ontology-types`

## Functions

### LoadFromFile

Parses a single YAML file and returns either an EntityType or RelationType based on the `kind` field.

```go
func LoadFromFile(path string) (entityType *EntityType, relationType *RelationType, err error)
```

**Behavior**:
- Reads the file at `path`
- First pass: decodes `kind` field to determine type
- Second pass: decodes into full `EntityType` or `RelationType` struct
- Returns `(entityType, nil, nil)` for EntityType files
- Returns `(nil, relationType, nil)` for RelationType files
- Returns `(nil, nil, err)` on parse failure

**Errors**:
- `ErrInvalidSchema` — malformed YAML or missing required fields
- `ErrUnsupportedKind` — `kind` is not `EntityType` or `RelationType`
- `ErrInvalidAPIVersion` — `apiVersion` is not `twin.io/v1`
- `os.PathError` — file not found or unreadable

### LoadFromDir

Scans all `.yaml` files in a directory and parses each into EntityType or RelationType.

```go
func LoadFromDir(dir string) (entityTypes []EntityType, relationTypes []RelationType, err error)
```

**Behavior**:
- Scans `dir` for files matching `*.yaml`
- Calls `LoadFromFile` for each file
- Collects results into separate slices
- Returns all parsed results even if some files fail (with accumulated errors)
- Does NOT perform cross-reference validation (deferred to I-02)

**Errors**:
- Wraps per-file errors with file path context: `parse schema "device.yaml": <cause>`
- Returns first error encountered (fail-fast for MVP)

### Multi-Document Support

For files containing multiple YAML documents (separated by `---`):
- `LoadFromFile` detects multi-document files and uses `yaml.Decoder` loop
- Currently only `relations.yaml` uses multi-document format
- Each document must have its own `apiVersion` and `kind` fields
