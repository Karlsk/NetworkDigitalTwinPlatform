# Tasks: Schema Data Structures and Ontology YAML Definitions

**Input**: Design documents from `specs/001-schema-ontology-types/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/loader.md

**Tests**: TDD approach — tests written before implementation per constitution Principle IV.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add YAML dependency and create sentinel errors that all subsequent tasks depend on.

- [x] T001 Add `gopkg.in/yaml.v3` dependency via `go get gopkg.in/yaml.v3` in `go.mod`
- [x] T002 [P] Define sentinel errors in `internal/schema/errors.go`: `ErrInvalidSchema`, `ErrUnsupportedKind`, `ErrInvalidAPIVersion` using `errors.New()` with lowercase messages, no periods

**Checkpoint**: Dependencies installed, error types available for import.

---

## Phase 2: Foundational (Type Definitions)

**Purpose**: Define all Go struct types for EntityType, RelationType, and supporting types. These are shared by US1 (loader) and US2 (YAML files).

**⚠️ CRITICAL**: Types must compile and parse correctly before any user story work begins.

- [x] T003 Implement all struct definitions in `internal/schema/types.go` per D-02 spec: `EntityType`, `EntityTypeSpec`, `RelationType`, `RelationTypeSpec`, `Metadata`, `IdentitySpec`, `NormalizeRule`, `RelationFieldSpec`, `PropertySpec` — all with `yaml:"..."` struct tags, Go doc comments, and correct field types (`map[string]string`, `map[string]PropertySpec`, `[]string`, `any` for Default)
- [x] T004 Write table-driven parsing tests in `internal/schema/types_test.go`: test `yaml.Unmarshal` with inline YAML strings for EntityType (use Device as example) and RelationType (use HAS_INTERFACE as example) — verify all fields populate correctly including nested structs, maps, and slices. Include test cases for: valid EntityType parse, valid RelationType parse, empty optional fields (nil maps/slices), multi-document YAML with `yaml.Decoder` loop
- [x] T005 Run `go test ./internal/schema/...` and verify all types_test.go tests pass. Fix any struct tag mismatches or type errors.

**Checkpoint**: All struct types defined and verified — `yaml.Unmarshal` correctly parses inline YAML into structs.

---

## Phase 3: User Story 1 — Schema Loading from Files (Priority: P1) 🎯 MVP

**Goal**: Implement `LoadFromFile` and `LoadFromDir` functions that parse YAML files from disk into EntityType/RelationType structs.

**Independent Test**: Create temp YAML files, call `LoadFromFile`/`LoadFromDir`, verify correct struct output and error handling.

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T006 [US1] Write `LoadFromFile` tests in `internal/schema/loader_test.go`: table-driven tests covering — (a) valid EntityType file returns `(*EntityType, nil, nil)`, (b) valid RelationType file returns `(nil, *RelationType, nil)`, (c) unsupported kind returns `ErrUnsupportedKind`, (d) wrong apiVersion returns `ErrInvalidAPIVersion`, (e) malformed YAML returns `ErrInvalidSchema`, (f) non-existent file returns `os.PathError`. Use `os.WriteFile` + `t.TempDir()` for test fixtures.
- [x] T007 [US1] Write `LoadFromDir` tests in `internal/schema/loader_test.go`: (a) directory with mixed EntityType + RelationType files returns correct slices, (b) empty directory returns empty slices with no error, (c) directory with invalid YAML file returns error with file path context, (d) non-`.yaml` files are skipped. Use `t.TempDir()` with multiple test files.
- [x] T008 [US1] Write multi-document YAML test in `internal/schema/loader_test.go`: create a temp file with 2+ RelationType documents separated by `---`, call `LoadFromFile`, verify it returns multiple RelationType records. Verify trailing `---` with empty content is handled gracefully.

### Implementation for User Story 1

- [x] T009 [US1] Implement `LoadFromFile` in `internal/schema/loader.go` per `contracts/loader.md`: two-pass approach — first decode into lightweight kind-probe struct to read `kind` and `apiVersion`, validate `apiVersion == "twin.io/v1"`, then decode into full `EntityType` or `RelationType`. Return `(*EntityType, nil, nil)` or `(nil, *RelationType, nil)`. Wrap errors with `fmt.Errorf("load schema from %s: %w", path, err)`.
- [x] T010 [US1] Extend `LoadFromFile` in `internal/schema/loader.go` to handle multi-document YAML: detect multiple documents by using `yaml.NewDecoder` + loop `Decode()` until `io.EOF`. For multi-doc files, return a slice-based result (document the return convention — if multi-doc, first doc returned via primary return, additional docs via a new `LoadMultiFromFile` helper or accumulate in `LoadFromDir`).
- [x] T011 [US1] Implement `LoadFromDir` in `internal/schema/loader.go` per `contracts/loader.md`: scan directory with `os.ReadDir`, filter `*.yaml` files, call `LoadFromFile` for each, collect `EntityType` and `RelationType` results into separate slices. Wrap per-file errors with filename context: `fmt.Errorf("parse schema %q: %w", filename, err)`. Fail-fast on first error for MVP.
- [x] T012 [US1] Run `go test -v -coverprofile ./internal/schema/...` and verify all loader_test.go tests pass with ≥80% coverage. Fix any failing tests.

**Checkpoint**: `LoadFromFile` and `LoadFromDir` work correctly with temp YAML files. ≥80% test coverage.

---

## Phase 4: User Story 2 — Ontology YAML Files (Priority: P1)

**Goal**: Create 7 YAML ontology definition files in `ontology/` that define the network topology vocabulary — 6 EntityTypes and 4 RelationTypes.

**Independent Test**: Call `LoadFromDir("ontology")` and verify 6 EntityTypes + 4 RelationTypes load with correct property counts and field values.

### Implementation for User Story 2

- [x] T013 [P] [US2] Create Device EntityType YAML in `ontology/device.yaml` per D-02 spec: `apiVersion: twin.io/v1`, `kind: EntityType`, `metadata.name: Device`, `labels: [Resource, Network]`, `identity.stableKeys: [serial_number]`, `uriTemplate: "device:{serial_number}"`, `fieldMapping` (mgmt_ip→management_ip, hw_model→model), `normalize` (hostname space→underscore), `relationFields` (interfaces→HAS_INTERFACE, upstream_links→CONNECTS_TO), 8 properties with serial_number and hostname as `required: true`
- [x] T014 [P] [US2] Create Interface EntityType YAML in `ontology/interface.yaml`: `stableKeys: [device_serial, if_name]`, `uriTemplate: "iface:{device_serial}_{if_name}"`, 5 properties (device_serial, if_name as `required: true`), empty fieldMapping/normalize/relationFields
- [x] T015 [P] [US2] Create ISIS EntityType YAML in `ontology/isis.yaml`: `stableKeys: [isis_id]`, `uriTemplate: "isis:{isis_id}"`, `relationFields` (run_on→RUNS_ON), 5 properties (isis_id, system_id as `required: true`)
- [x] T016 [P] [US2] Create Link EntityType YAML in `ontology/link.yaml`: `stableKeys: [link_id]`, `uriTemplate: "link:{link_id}"`, `relationFields` (endpoints→ENDPOINT), 4 properties (link_id as `required: true`)
- [x] T017 [P] [US2] Create Network_Slice EntityType YAML in `ontology/network_slice.yaml`: `stableKeys: [slice_id]`, `uriTemplate: "slice:{slice_id}"`, 4 properties (slice_id as `required: true`, sla_bandwidth and sla_latency as `type: int`)
- [x] T018 [P] [US2] Create Alarm EntityType YAML in `ontology/alarm.yaml`: `stableKeys: [alarm_id]`, `uriTemplate: "alarm:{alarm_id}"`, `relationFields` (occurred_on→OCCURRED_ON), 4 properties (alarm_id as `required: true`, severity with enum values)
- [x] T019 [US2] Create multi-document RelationType YAML in `ontology/relations.yaml`: 4 documents separated by `---` defining HAS_INTERFACE (Device→Interface), RUNS_ON (ISIS→Interface), ENDPOINT (Link→Interface), OCCURRED_ON (Alarm→Interface). Each with `apiVersion: twin.io/v1` and `kind: RelationType`.
- [x] T020 [US2] Write ontology integration test in `internal/schema/loader_test.go`: `TestLoadOntologyDir` calls `LoadFromDir("../../ontology")`, asserts `len(entityTypes) == 6`, `len(relationTypes) == 4`, verifies Device has 8 properties, Interface has 5, all stableKeys fields are `required: true`, and each EntityType name matches expected set. Run `go test -v -run TestLoadOntologyDir ./internal/schema/...` to verify.

**Checkpoint**: All 7 ontology YAML files parse correctly. `LoadFromDir("ontology")` returns 6 EntityTypes + 4 RelationTypes with correct field values.

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Final validation, coverage verification, and code quality checks.

- [x] T021 Run `go test -v -coverprofile=coverage.out ./internal/schema/...` and verify ≥80% line coverage across `types.go`, `loader.go`, `errors.go`. Inspect `go tool cover -func=coverage.out` output.
- [x] T022 Run `golangci-lint run ./internal/schema/...` and fix any lint errors or warnings.
- [x] T023 Run quickstart.md validation scenarios: (1) `go build ./internal/schema/...` compiles clean, (2) all tests pass, (3) coverage ≥80%, (4) ontology loads correctly, (5) property counts match, (6) stableKeys are required, (7) lint passes.
- [x] T024 Verify `go vet ./internal/schema/...` reports no issues.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup (needs yaml.v3 dependency) — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Foundational (types must exist before loader tests)
- **US2 (Phase 4)**: Depends on US1 (needs `LoadFromDir` to verify YAML files)
- **Polish (Phase 5)**: Depends on US1 + US2 completion

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) — implements parsing capability
- **User Story 2 (P1)**: Depends on US1 completion — needs loader to verify YAML files load correctly

### Within Each User Story

- Tests MUST be written and FAIL before implementation (TDD per constitution)
- Implementation follows tests
- Story complete before moving to next

### Parallel Opportunities

- T002 (errors.go) can run parallel with any setup task
- T013–T018 (6 entity YAML files) can ALL run in parallel — different files, no dependencies
- T021–T024 (polish tasks) can run in parallel — independent checks

---

## Parallel Example: User Story 2

```bash
# All 6 entity YAML files can be created simultaneously:
Task: T013 "Create Device EntityType YAML in ontology/device.yaml"
Task: T014 "Create Interface EntityType YAML in ontology/interface.yaml"
Task: T015 "Create ISIS EntityType YAML in ontology/isis.yaml"
Task: T016 "Create Link EntityType YAML in ontology/link.yaml"
Task: T017 "Create Network_Slice EntityType YAML in ontology/network_slice.yaml"
Task: T018 "Create Alarm EntityType YAML in ontology/alarm.yaml"

# Then sequentially:
Task: T019 "Create relations.yaml"
Task: T020 "Write integration test"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (type definitions)
3. Complete Phase 3: User Story 1 (loader)
4. **STOP and VALIDATE**: `LoadFromFile` and `LoadFromDir` work with temp files
5. This delivers the parsing capability — YAML files can be added incrementally

### Incremental Delivery

1. Complete Setup + Foundational → Types compile, YAML parsing works
2. Add User Story 1 → `LoadFromFile`/`LoadFromDir` functional → Test independently (MVP!)
3. Add User Story 2 → 7 YAML files load correctly → Full ontology operational
4. Polish → Coverage and lint verified

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- TDD enforced: tests written before implementation for loader (Phase 3)
- Types (Phase 2) are pure data definitions — tests verify YAML parsing, not behavior
- Commit after each task or logical group
- YAML content for all ontology files is copy-paste ready from D-02 spec document
- `CONNECTS_TO` relation is intentionally NOT defined in `relations.yaml` — deferred to I-02
