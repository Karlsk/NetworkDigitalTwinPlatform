# Network Digital Twin Constitution

## Core Principles

### I. Layered IR Pipeline Integrity
The system follows a strict layered intermediate representation (IR) pipeline: Connector → Normalizer → GraphAssembler → GraphDB → Neo4j. Each layer owns exactly one transformation and communicates only through explicit contracts (Resource, NormalizedResource, GraphModel). No layer may bypass its boundaries or read data it doesn't own.

**Enforcement**:
- Connector outputs `Resource` only — no field mapping, no validation
- Normalizer reads `EntityType` schema — outputs `NormalizedResource` only, no relation derivation
- GraphAssembler reads `EntityType.relationFields` + `RelationType` — outputs `GraphModel` only, no DB access
- GraphDB receives `GraphModel` only — no schema awareness, no business logic

### II. Interface-First Design
All cross-module dependencies must be defined through Go interfaces — never concrete types. Consumers depend on interfaces; providers implement them. This enables mock-based testing, swappable implementations, and clean layer boundaries.

**Enforcement**:
- Service layer depends on `graph.GraphDB` interface, not `*neo4jClient`
- Connector framework defines `Connector` interface; mock/test implementations satisfy it
- Constructor functions return interfaces: `func NewNeo4jClient(...) (GraphDB, error)`
- Struct fields typed as interfaces: `graph graph.GraphDB`, `lock *snapshot.GraphLock`
- New cross-module dependency → define interface in consumer's package first

### III. Schema-Driven Zero-Code Extension
New network entities and relations must be addable via YAML files alone — no Go code changes required. The Schema Registry auto-loads all YAML definitions at startup; entity types drive Normalizer field mapping, URI generation, and GraphAssembler relation derivation.

**Enforcement**:
- New entity: add YAML file to `ontology/` → Schema Registry auto-registers
- New relation: modify `relationFields` in entity YAML + add to `relations.yaml`
- Validation: EntityType schema drives `internal/schema/validator.go`

### IV. Test-First Development (NON-NEGOTIABLE)
TDD is mandatory for all feature work. Tests are written first, approved by the user, verified to fail (RED), then implementation proceeds (GREEN), followed by refactoring (IMPROVE). Minimum 80% test coverage required across unit, integration, and E2E tests.

**Enforcement**:
- RED: Write test → verify failure → user approves
- GREEN: Minimal implementation → verify pass
- REFACTOR: Improve code → verify tests still pass
- Coverage gate: `go test -coverprofile` must show ≥80%

### V. Concurrent Safety via GraphLock
All write operations (FullSync, IncrementalSync, Restore) are mutually exclusive through a shared `GraphLock` (sync.RWMutex). Read operations use shared read locks. No write operation may execute without holding the write lock. No lock may be acquired without `defer` release.

**Enforcement**:
- Write ops: `s.lock.Lock()` + `defer s.lock.Unlock()` at function entry
- Read ops: `s.lock.RLock()` + `defer s.lock.RUnlock()`
- Webhook events buffered in channel → single goroutine consumer → serialized writes
- Channel full → return 503, never block indefinitely

### VI. Error Handling Completeness
Every error must be handled explicitly — no silent swallowing. Errors are wrapped with `fmt.Errorf` + `%w` carrying operation context. Sentinel errors defined for business logic branching. Error messages are lowercase, no trailing periods.

**Enforcement**:
- No `result, _ := someFunc()` — every error checked or explicitly ignored with comment
- Wrap: `fmt.Errorf("normalize resource %s/%s: %w", kind, id, err)`
- Sentinel: `var ErrSchemaNotFound = errors.New("schema not found")`
- Branch: `if errors.Is(err, ErrSchemaNotFound) { ... }`

## Code Quality Standards

### Naming Conventions
- Package names: lowercase single words matching directory (`normalizer`, not `norm_engine`)
- Interfaces: verb+er for single-method (`Reader`), noun for multi-method (`GraphDB`)
- Exported: uppercase start, no `Get` prefix (except SchemaRegistry for clarity)
- Receivers: 1-2 char type abbreviation (`a *GraphAssembler`, not `assembler`)

### File Organization
- Max 400 lines per file, 800 absolute max — extract modules beyond this
- High cohesion: each file serves one clear purpose
- Low coupling: modules interact through interfaces, not concrete types
- Feature-based organization: `internal/schema/`, `internal/connector/`, not `internal/models/`

### Structured Logging
- Use `log/slog` exclusively — no `fmt.Println` or `log.Printf`
- Levels: Debug (dev details), Info (business flows), Warn (recoverable), Error (fatal)
- Log key identifiers only, never full objects: `"connector", c.Metadata().Name, "count", len(resources)`

### Dependency Injection
- All dependencies injected via constructor functions (`NewSyncService(...)`)
- No global variables, no `init()` functions
- Interface-based dependencies for testability: `graph graph.GraphDB` not `*neo4jClient`

## Testing Standards

### Test Types (All Required)
1. **Unit Tests**: Individual functions, utilities, schema validation — colocated `*_test.go` files
2. **Integration Tests**: Neo4j operations, full pipeline (Connector→Normalizer→Assembler→GraphDB) — `testdata/` fixtures
3. **E2E Tests**: MCP tool invocations, sync workflows, snapshot lifecycle — golden file validation

### Coverage Requirements
- Minimum 80% line coverage across all packages
- Critical paths (sync, snapshot, schema loading) require 95%+ coverage
- Coverage reported in CI, blocks merge if below threshold

### Test Data Management
- Fixtures in `testdata/` directory — YAML schemas, mock resources, golden files
- Mock implementations: `internal/connector/mock/mock.go` for Connector interface
- Table-driven tests preferred for validation logic
- Test isolation: each test creates/cleans its own Neo4j logical DB (`_db` = test name)

### Golden File Pattern
- Expected outputs stored as YAML in `testdata/golden/`
- Tests compare actual vs expected with `testdata.AssertGolden(t, actual)`
- Update golden files with `UPDATE_GOLDEN=true go test ./...`

## User Experience Consistency

### MCP Tool Interface Contract
All MCP tools follow consistent patterns for external Agent consumption:

| Aspect | Standard |
|--------|----------|
| Tool naming | `snake_case` verbs: `query_topology`, `sync_data`, `restore_snapshot` |
| Input schema | JSON Schema with required/optional fields, defaults documented |
| Output envelope | `{ "success": bool, "data": any, "error": string }` |
| Error format | Human-readable message + machine-parseable error code |
| Read-only tools | `query_topology`, `query_snapshot` — no side effects, safe to retry |
| Write tools | `sync_data`, `restore_snapshot` — clearly marked, require confirmation |

### API Response Consistency
- Success: `{ "success": true, "data": <payload> }`
- Error: `{ "success": false, "error": "<message>", "code": "<ERROR_CODE>" }`
- Metadata (paginated): `{ "total": N, "limit": N, "offset": N }`

### Schema Documentation
- Every EntityType YAML must include `description` field
- Every relation must document cardinality and semantics
- Schema validation errors include field path: `entity_type "Device": field "hostname": required`

## Performance Requirements

### Database Operations
| Operation | Target | Strategy |
|-----------|--------|----------|
| Single node query | <10ms | `(_db, uri)` composite index on all labels |
| FullSync (1000 nodes) | <30s | `UNWIND $batch` batching (500/batch) |
| IncrementalSync | <500ms | MERGE on `(_db, uri)`, `SET +=` for properties |
| Snapshot export | <10s | Stream YAML with `SKIP/LIMIT` pagination |
| Snapshot restore | <15s | Cypher `CloneDB` bulk copy |

### Concurrency Targets
- Webhook handler: <10ms response time (buffer → 202 Accepted)
- Channel buffer: 100 events, non-blocking send with 503 on full
- Read operations: unlimited concurrent readers (RLock)
- Write operations: serialized (Lock), max 1 concurrent

### Resource Limits
- Logical DBs: LRU eviction at `maxActive=5` (configurable)
- Batch size: 500 nodes per `UNWIND` (configurable)
- Neo4j connection timeout: 10s
- FullSync timeout: 5min (context-controlled)
- Schema files: loaded once at startup, no hot-reload overhead

### Index Strategy
Every EntityType **must** have `(_db, uri)` composite index:
```cypher
CREATE INDEX <label>_db_uri FOR (n:<Label>) ON (n._db, n.uri);
```
- New EntityType → new index in `ensureIndexes()` (non-negotiable)
- MERGE operations use composite key for idempotency
- No ad-hoc indexes without performance justification

## Security Constraints

### Data Isolation
- `_db` property injected by driver layer — business code never sets manually
- All Cypher templates **must** filter `WHERE n._db = $_db`
- Logical DB isolation prevents cross-tenant data leakage

### Input Validation
- Schema YAML validated at load time — malformed schemas cause fail-fast startup
- MCP tool inputs validated against JSON Schema before processing
- Cypher parameters always bound (`$param`), never string-concatenated

### Secret Management
- Neo4j credentials via environment variables only
- No hardcoded passwords in source code or docker-compose.yml (use `.env` files)
- `.env` files excluded from version control

## Governance

This constitution supersedes all other development practices for the Network Digital Twin project. All code reviews, PRs, and architectural decisions must verify compliance with these principles.

**Amendment Process**:
1. Document proposed change with rationale
2. Review impact on existing code and tests
3. Update this constitution and CLAUDE.md simultaneously
4. Migration plan required for breaking changes

**Compliance Verification**:
- Code reviewers check against all principles before approval
- CI pipeline enforces test coverage and lint rules
- Architecture reviews required for cross-layer changes

**Version**: 1.0.0 | **Ratified**: 2026-06-15 | **Last Amended**: 2026-06-15
