# Feature Specification: Schema Data Structures and Ontology YAML Definitions

**Feature Branch**: `001-schema-ontology-types`

**Created**: 2026-06-15

**Status**: Draft

**Input**: User description: "按照 D-02 文档中的详细规范实现 Schema 数据结构和本体 YAML 定义"

## Clarifications

### Session 2026-06-15

- Q: Should this feature include basic YAML-to-struct parsing functions, or only type definitions + YAML files? → A: **Types + YAML + basic parsing** — include `LoadFromFile()` and `LoadFromDir()` helper functions. Full registry (caching, listing, querying) deferred to I-01 task.
- Q: When are cross-reference validations (FR-011, FR-012) enforced? → A: **Separate validation step only** — `LoadFromDir` parses without cross-ref checks. Validation runs as an explicit post-load step (I-02 task). CONNECTS_TO is allowed during parsing without error.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Schema Type System Definition (Priority: P1)

The Schema Registry needs a set of data structures that represent the network ontology — entity types (Device, Interface, ISIS, Link, Network_Slice, Alarm) and relation types (HAS_INTERFACE, RUNS_ON, ENDPOINT, OCCURRED_ON). These structures form the "single source of truth" that all downstream modules (Normalizer, GraphAssembler, Validator) depend on to interpret and transform network data.

**Why this priority**: Without a well-defined type system, no downstream module can function. This is the foundation of the entire data pipeline — every other feature depends on it.

**Independent Test**: Can be fully tested by loading all YAML ontology files through the parser and verifying that each file produces the expected in-memory structure with correct field values, types, and references.

**Acceptance Scenarios**:

1. **Given** the ontology directory contains 7 valid YAML files, **When** the Schema Registry loads them, **Then** 6 EntityType and 4 RelationType records are registered with all fields correctly populated.
2. **Given** a valid Device EntityType YAML, **When** parsed, **Then** the `identity.stableKeys` contains only immutable identifiers (e.g., `serial_number`), `uriTemplate` references only stableKeys fields, and all `properties` entries have correct types and constraints.
3. **Given** a multi-document `relations.yaml` file, **When** parsed with a document-loop decoder, **Then** each `---`-separated document produces one valid RelationType record with correct source/target arrays.
4. **Given** an EntityType with `relationFields`, **When** validated, **Then** every `relationType` value referenced exists as a defined RelationType in `relations.yaml`.

---

### User Story 2 - Ontology YAML for Network Entities (Priority: P1)

Network engineers and system integrators need a set of YAML ontology files that define the network topology vocabulary — what entities exist (Device, Interface, ISIS, Link, Network_Slice, Alarm), what properties they have, how identities are formed, and how they relate to each other. These YAML files are the single source of truth for the ontology and must be human-readable and version-controllable.

**Why this priority**: The YAML files are the primary input to the Schema Registry. Without them, no entities or relations can be modeled. This is co-equal with the type system itself.

**Independent Test**: Each YAML file can be independently validated against the type system — parsing succeeds, required fields are present, stableKeys fields are marked `required: true`, and relationFields reference valid RelationTypes.

**Acceptance Scenarios**:

1. **Given** the 6 entity YAML files, **When** each is parsed, **Then** the `identity.stableKeys` fields in `properties` are all marked `required: true`.
2. **Given** the `relations.yaml` file with 4 RelationType documents, **When** parsed, **Then** each RelationType has valid `source` and `target` arrays referencing existing EntityType names.
3. **Given** the Device EntityType, **When** inspected, **Then** `fieldMapping` maps legacy field names to canonical names, `normalize` rules define string transformations, and `relationFields` declares which property fields derive relations.
4. **Given** the Network_Slice EntityType, **When** parsed, **Then** SLA properties (`sla_bandwidth`, `sla_latency`) are typed as integers with no enum constraint.

---

### User Story 3 - Cross-Reference Validation (Priority: P2)

When the Schema Registry loads all ontology files, it must verify internal consistency — that every relation referenced by an EntityType's `relationFields` is actually defined in `relations.yaml`, and that every RelationType's `source`/`target` references valid EntityType names. This prevents silent misconfigurations that would cause runtime failures in downstream modules.

**Why this priority**: Cross-reference integrity is essential for correctness but can be validated as a post-load check. The type system and YAML files must exist first (P1) before cross-references can be verified.

**Independent Test**: Can be tested by loading all ontology files and running the cross-reference validator, which reports any dangling references or undefined types.

**Acceptance Scenarios**:

1. **Given** all ontology files are loaded, **When** cross-reference validation runs, **Then** every `relationFields[*].relationType` value matches a RelationType `metadata.name` in `relations.yaml`.
2. **Given** all ontology files are loaded, **When** cross-reference validation runs, **Then** every RelationType `source` and `target` value matches an EntityType `metadata.name`.
3. **Given** a RelationType references a non-existent EntityType, **When** validation runs, **Then** a clear error message identifies the broken reference.

---

### Edge Cases

- What happens when a YAML file contains a field with an unsupported type (e.g., `type: complex`)? → Parser rejects with a clear error identifying the file and field.
- What happens when `stableKeys` references a field not defined in `properties`? → Validation fails with a descriptive error.
- What happens when `uriTemplate` references a variable not in `stableKeys`? → Validation fails — URI templates may only reference stableKeys fields.
- What happens when a multi-document YAML file has a trailing `---` with no content? → Parser skips empty documents without error.
- How does the system handle YAML files with inconsistent `apiVersion` values? → Parser validates that `apiVersion` matches the expected value (`twin.io/v1`) and rejects mismatches.
- How does the system handle `relationFields` referencing undefined RelationTypes (e.g., `CONNECTS_TO`)? → `LoadFromDir` allows it without error — cross-reference validation is deferred to a separate step (I-02).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST define data structures for EntityType with fields: `apiVersion`, `kind`, `metadata`, and `spec` (containing `identity`, `uriTemplate`, `fieldMapping`, `normalize`, `relationFields`, `properties`).
- **FR-002**: System MUST define data structures for RelationType with fields: `apiVersion`, `kind`, `metadata`, and `spec` (containing `source` and `target` arrays).
- **FR-003**: System MUST define data structures for Metadata (`name`, `labels`), IdentitySpec (`stableKeys`), NormalizeRule (`field`, `pattern`, `replace`), RelationFieldSpec (`relationType`), and PropertySpec (`type`, `required`, `enum`, `default`).
- **FR-004**: System MUST support parsing YAML files into the defined data structures using standard YAML deserialization, including helper functions to load a single file (`LoadFromFile`) and scan an entire directory (`LoadFromDir`).
- **FR-005**: System MUST support multi-document YAML files (documents separated by `---`) for RelationType definitions.
- **FR-004a**: `LoadFromDir` MUST scan all `.yaml` files in a given directory, parse each into either an EntityType or RelationType, and return the collected results.
- **FR-006**: System MUST provide 6 EntityType YAML definitions: Device, Interface, ISIS, Link, Network_Slice, Alarm.
- **FR-007**: System MUST provide 4 RelationType YAML definitions: HAS_INTERFACE, RUNS_ON, ENDPOINT, OCCURRED_ON.
- **FR-008**: Every EntityType's `identity.stableKeys` MUST reference only immutable identifiers (serial numbers, MAC addresses, composite keys that never change).
- **FR-009**: Every EntityType's `uriTemplate` MUST only reference fields listed in `identity.stableKeys`.
- **FR-010**: Every field listed in `identity.stableKeys` MUST be marked `required: true` in the EntityType's `properties`.
- **FR-011**: Every `relationFields[*].relationType` value MUST correspond to a defined RelationType in `relations.yaml`. *(Enforced at validation time, not during parsing — see I-02 task. `CONNECTS_TO` is intentionally undefined during this feature.)*
- **FR-012**: Every RelationType's `source` and `target` values MUST correspond to defined EntityType names. *(Enforced at validation time, not during parsing.)*
- **FR-013**: All YAML files MUST use `apiVersion: twin.io/v1` and appropriate `kind` values (`EntityType` or `RelationType`).
- **FR-014**: PropertySpec `default` field MUST support any valid YAML value type (string, integer, boolean).

### Key Entities

- **EntityType**: Represents a network entity class (e.g., Device, Interface). Key attributes: name, labels, identity specification, URI template, field mappings, normalization rules, relation field declarations, and property definitions.
- **RelationType**: Represents a directed relationship between two entity types. Key attributes: name, source entity types (array), target entity types (array).
- **PropertySpec**: Defines a single property within an EntityType. Key attributes: data type, required flag, allowed enum values, default value.
- **IdentitySpec**: Defines the immutable identity of an entity. Key attribute: stableKeys — the set of property names that form the permanent identity and are used in URI generation.
- **NormalizeRule**: Defines a field-level string transformation. Key attributes: target field, regex pattern, replacement string.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All 7 YAML ontology files parse successfully without errors — 100% parse success rate.
- **SC-002**: All 6 EntityType definitions load with correct field counts (Device: 8 properties, Interface: 5 properties, ISIS: 5 properties, Link: 4 properties, Network_Slice: 4 properties, Alarm: 4 properties).
- **SC-003**: All 4 RelationType definitions load with correct source/target arrays — zero cross-reference violations.
- **SC-004**: 100% of stableKeys fields across all EntityTypes are marked `required: true` in their respective `properties` maps.
- **SC-005**: Multi-document YAML parsing produces exactly 4 RelationType records from a single `relations.yaml` file.
- **SC-006**: Schema loading completes in under 1 second for all 7 files combined.

## Assumptions

- The YAML parsing library supports multi-document files via document-loop decoding (standard YAML decoder behavior).
- The `apiVersion` value `twin.io/v1` is fixed for the MVP and will not change during this feature.
- Device's `CONNECTS_TO` relation referenced in `relationFields` is intentionally deferred — it will be defined in a later task (validation phase), not in this feature.
- Property types are limited to: `string`, `int`, `bool` for MVP scope.
- The `default` field in PropertySpec accepts any valid YAML scalar value.
- Labels arrays in Metadata are optional — some EntityTypes may have empty label arrays.
- The ontology directory path is configurable and will be injected via configuration, not hardcoded.
- This feature includes basic parsing helpers (`LoadFromFile`, `LoadFromDir`) but NOT the full Schema Registry (caching, querying, lifecycle management) — that is deferred to task I-01.
