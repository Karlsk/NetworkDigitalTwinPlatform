# Interface Contracts Checklist: Core Interface Definitions (D-03)

**Purpose**: Validate the quality, completeness, clarity, and consistency of Go interface contract requirements before implementation
**Created**: 2026-06-22
**Feature**: [spec.md](../spec.md) | [contracts](../contracts/interfaces.md) | [data-model](../data-model.md)

**Note**: This checklist validates the REQUIREMENTS THEMSELVES — not the implementation. Each item asks whether the specification is well-written, complete, and unambiguous enough to guide correct implementation.

## Interface Contract Completeness

- [ ] CHK001 Are all three required interfaces (SchemaRegistry, Connector, GraphDB) fully specified with complete method lists? [Completeness, Spec §FR-001/FR-007/FR-010]
- [ ] CHK002 Does the SchemaRegistry contract cover all six operational needs: loading, EntityType query, RelationType query, listing both types, validation, and default filling? [Completeness, Spec §FR-001 to FR-006a]
- [ ] CHK003 Does the Connector contract address both data retrieval modes (full collection and incremental streaming)? [Completeness, Spec §FR-007]
- [ ] CHK004 Does the GraphDB contract cover all five operation categories: connectivity (2), full sync (1), incremental sync (3), query (2), logical DB management (4)? [Completeness, Spec §FR-010]
- [ ] CHK005 Is ConnectorRegistry specified as a struct (not interface) with constructor and all 3 methods (Register, Get, List)? [Completeness, Spec §FR-008]
- [ ] CHK006 Does the contract include BuildCypher as a separate preview mechanism distinct from execution methods? [Completeness, Spec §FR-012]
- [ ] CHK007 Are companion types (Resource, ConnectorMetadata, Node, Relation) explicitly included in scope with field definitions? [Completeness, Spec §FR-013]

## Method Signature Clarity

- [ ] CHK008 Does SchemaRegistry.Validate explicitly state it does not mutate the input map? [Clarity, Spec §FR-006]
- [ ] CHK009 Does SchemaRegistry.ApplyDefaults specify the return type (new map) and original-map-preserved semantics? [Clarity, Spec §FR-006a]
- [ ] CHK010 Is Connector.Stream's dual return (`<-chan Resource, error`) semantics documented — when channel is nil vs valid, and when error indicates not-implemented vs failure? [Clarity, Spec §FR-009]
- [ ] CHK011 Are Connector.Collect (full pull) and Connector.Stream (incremental) semantics non-overlapping and clearly distinguished? [Clarity, Spec §FR-007]
- [ ] CHK012 Are GraphDB.BulkCreate (replace-after-clear) and GraphDB.Upsert (incremental merge) semantics non-overlapping and clearly distinguished? [Clarity, Spec §FR-010]
- [ ] CHK013 Is the BuildCypher `action` parameter's four valid values explicitly enumerated with their corresponding operation methods? [Clarity, Spec §FR-012, Contracts §3]
- [ ] CHK014 Does GraphDB.Query specify the result format (slice of maps, each representing a row)? [Clarity, Contracts §3]
- [ ] CHK015 Does SchemaRegistry.Load specify support for multi-document YAML files (---  separators)? [Clarity, Spec §FR-001]

## Cross-Package Boundary Integrity

- [ ] CHK016 Is the one-way dependency direction (graph → assembler, no reverse) explicitly documented? [Consistency, Plan §R-004]
- [ ] CHK017 Does the graph package contract import ONLY assembler types (Node, Relation) — not schema or connector packages? [Consistency, Contracts §3]
- [ ] CHK018 Are assembler Node and Relation types sufficient for all GraphDB operations (contain Label/URI/Props and Type/From/To/Props respectively)? [Completeness, Data Model §assembler]
- [ ] CHK019 Are connector Resource and ConnectorMetadata types sufficient for the Connector interface (Kind/ID/Properties and Name/Type/EntityTypes)? [Completeness, Data Model §connector]
- [ ] CHK020 Is the schema package self-contained with no cross-package imports for its interface definition? [Consistency, Plan §R-004]
- [ ] CHK021 Does the layered pipeline invariant hold — GraphDB contract references only GraphModel IR, never Schema types? [Consistency, Constitution §I]

## Immutability Contract

- [ ] CHK022 Is the separation between Validate (read-only) and ApplyDefaults (returns new map) explicitly documented as an immutability design decision? [Clarity, Spec §FR-006/FR-006a, Research §R-003]
- [ ] CHK023 Does ApplyDefaults specify error behavior when entityKind is unknown (returns ErrSchemaNotFound, not partial result)? [Clarity, Spec §FR-006a]
- [ ] CHK024 Is the immutability guarantee documented — original map never modified by either Validate or ApplyDefaults? [Consistency, Research §R-003]

## Error Handling Specification

- [ ] CHK025 Are all sentinel errors (ErrSchemaNotFound, ErrNotImplemented, ErrConnectorNotFound) assigned to their correct packages? [Completeness, Spec §FR-015, Data Model §errors]
- [ ] CHK026 Does SchemaRegistry specify ErrSchemaNotFound for all lookup-by-name failures (GetEntityType, GetRelationType, Validate, ApplyDefaults)? [Consistency, Contracts §1]
- [ ] CHK027 Does ConnectorRegistry.Get specify ErrConnectorNotFound for missing connector names? [Consistency, Contracts §2]
- [ ] CHK028 Does Connector.Stream specify the error wrapping pattern (`fmt.Errorf` with `%w`) for MVP not-implemented behavior? [Clarity, Spec §FR-009, Research §R-007]
- [ ] CHK029 Do all sentinel error messages follow the project convention (lowercase, no trailing period)? [Consistency, Constitution §VI]
- [ ] CHK030 Do all error-returning methods specify their possible error conditions and corresponding error types? [Completeness, Contracts §1/§2/§3]
- [ ] CHK031 Is SchemaRegistry.Validate's error aggregation strategy documented (all failures collected, joined with separator)? [Clarity, Research §R-003]

## Companion Type Adequacy

- [ ] CHK032 Does Resource.Properties specify expected value types or constraints (string, int, bool per PropertySpec)? [Clarity, Data Model §connector]
- [ ] CHK033 Does ConnectorMetadata.Name specify uniqueness requirements and its role as the registry map key? [Clarity, Spec §Assumptions]
- [ ] CHK034 Does Node.URI reference the uriTemplate specification that governs its generation (from schema/types.go)? [Traceability, Data Model §assembler]
- [ ] CHK035 Are Relation.From and Relation.To documented as URI references (matching Node.URI format)? [Clarity, Data Model §assembler]
- [ ] CHK036 Do Node.Props and Relation.Props specify the same value type constraints as Resource.Properties? [Consistency, Data Model]

## Edge Case & Exception Coverage

- [ ] CHK037 Does the spec address SchemaRegistry.Load behavior for empty directories (no valid YAML files)? [Coverage, Spec §Edge Cases]
- [ ] CHK038 Does the spec address SchemaRegistry.Load behavior for malformed YAML files? [Coverage, Spec §Edge Cases]
- [ ] CHK039 Is duplicate-name handling specified for SchemaRegistry (duplicate EntityType/RelationType names in YAML files)? [Gap]
- [ ] CHK040 Is duplicate-name handling specified for ConnectorRegistry.Register (same Metadata().Name registered twice)? [Gap, Spec §Edge Cases]
- [ ] CHK041 Does SchemaRegistry.Validate specify behavior for unknown property keys not in schema (ignore, warn, or error)? [Gap]
- [ ] CHK042 Does GraphDB.Query specify return format for zero-result queries (empty slice vs nil)? [Gap]
- [ ] CHK043 Does GraphDB.CloneDB specify behavior when target database already exists (overwrite, error, or merge)? [Gap]
- [ ] CHK044 Does GraphDB.HasDB specify return value for newly created but empty logical databases? [Gap]
- [ ] CHK045 Does Connector.Collect specify behavior for unsupported entity types (error vs empty slice)? [Gap]

## Architecture Consistency

- [ ] CHK046 Does SchemaRegistry method naming follow the documented Get-prefix exception to the project's naming convention? [Consistency, CLAUDE.md §3]
- [ ] CHK047 Do interface file locations match the package structure documented in CLAUDE.md (schema/, connector/, graph/)? [Consistency, Plan §Structure]
- [ ] CHK048 Is the GraphDB `db string` parameter rule consistent with the logical multi-DB pattern in CLAUDE.md (`_db` property, `WHERE n._db = $_db`)? [Consistency, Spec §FR-011]
- [ ] CHK049 Does ConnectorRegistry constructor naming follow the `NewXxx` convention from CLAUDE.md §20? [Consistency, Research §R-005]
- [ ] CHK050 Are method counts consistent between spec (SC-003: 7, SC-004: 3, SC-005: 3, SC-006: 12) and contracts document? [Consistency, Spec §SC, Contracts]

## Cross-Document Traceability

- [ ] CHK051 Are all method signatures in contracts/interfaces.md consistent with D-03 changelog document specifications? [Consistency, Contracts vs D-03]
- [ ] CHK052 Are all companion type field definitions in data-model.md consistent with contracts document? [Consistency, Data Model vs Contracts]
- [ ] CHK053 Are sentinel error definitions consistent across spec, data-model, contracts, and research documents? [Consistency]
- [ ] CHK054 Are D-03 changelog acceptance criteria fully reflected in spec success criteria (SC-001 through SC-009)? [Traceability, D-03 §4 vs Spec §SC]
- [ ] CHK055 Does the research document resolve all ambiguities that could affect interface contract interpretation? [Completeness, Research §R-001 to R-007]

## Notes

- Check items off as completed: `[x]`
- Items marked `[Gap]` indicate requirement areas where the spec may be missing coverage — flag for clarification or planning-phase resolution
- Items marked `[Consistency]` require cross-referencing multiple documents
- This checklist is complementary to `requirements.md` (which validates spec completeness) — this one validates interface contract quality
