# Schema Requirements Quality Checklist

**Purpose**: Validate the completeness, clarity, consistency, and coverage of schema type definitions and YAML parsing requirements before implementation
**Created**: 2026-06-15
**Feature**: [spec.md](../spec.md) | **Plan**: [plan.md](../plan.md) | **Data Model**: [data-model.md](../data-model.md)

## Requirement Completeness

- [ ] CHK001 Are all 9 struct types from D-02 (EntityType, EntityTypeSpec, RelationType, RelationTypeSpec, Metadata, IdentitySpec, NormalizeRule, RelationFieldSpec, PropertySpec) explicitly listed in functional requirements? [Completeness, Spec §FR-001–FR-003]
- [ ] CHK002 Is the behavior of `Metadata.Labels` specified when the YAML field is absent — nil slice vs empty slice vs error? [Completeness, Gap]
- [ ] CHK003 Is the behavior of `EntityTypeSpec.FieldMapping` specified when the YAML field is empty (`fieldMapping: {}`) vs absent entirely? [Completeness, Gap]
- [ ] CHK004 Is the behavior of `EntityTypeSpec.Normalize` specified when the YAML field is empty (`normalize: []`) vs absent entirely? [Completeness, Gap]
- [ ] CHK005 Is `PropertySpec.Default` behavior specified when the YAML `default` key is absent — is the Go zero value acceptable, or should it be explicitly nil? [Completeness, Spec §FR-014]
- [ ] CHK006 Are all 6 EntityType YAML files individually accounted for in functional requirements with their expected property counts? [Completeness, Spec §FR-006, SC-002]

## Requirement Clarity

- [ ] CHK007 Is the `LoadFromFile` return convention for multi-document YAML files clearly specified — does it return only the first document, all documents, or is a separate function needed? [Clarity, Spec §FR-004, Contracts §loader.md]
- [ ] CHK008 Is the `LoadFromDir` file extension filtering explicitly defined — `.yaml` only, or `.yaml` and `.yml`? [Clarity, Spec §FR-004a]
- [ ] CHK009 Is the error accumulation strategy for `LoadFromDir` clearly specified — fail-fast on first error, or accumulate all errors and return partial results? [Clarity, Spec §FR-004a, Contracts §loader.md]
- [ ] CHK010 Is the `uriTemplate` variable syntax explicitly defined — `{fieldName}` pattern, and what happens with literal braces in the template? [Clarity, Spec §FR-009]
- [ ] CHK011 Is `PropertySpec.Type` validation scope clear — are only `string`, `int`, `bool` accepted, and what is the expected error message for unsupported types? [Clarity, Spec §FR-014, Assumptions]

## Requirement Consistency

- [ ] CHK012 Do FR-011 and FR-012 enforcement timing annotations align with Clarification Q2 — consistently marked as "validation time" across spec, data-model.md, and contracts? [Consistency, Spec §FR-011–FR-012, data-model.md §Validation Rules]
- [ ] CHK013 Is the `CONNECTS_TO` deferred status consistent across all spec sections — Assumptions, Edge Cases, FR-011 annotation, and User Story 3? [Consistency, Spec §Assumptions, §Edge Cases, §FR-011]
- [ ] CHK014 Does the `LoadFromFile` contract in `contracts/loader.md` match the functional requirements in spec.md — same parameters, same return types, same error conditions? [Consistency, Spec §FR-004, Contracts §loader.md]
- [ ] CHK015 Are property counts consistent between spec SC-002 (Device:8, Interface:5, ISIS:5, Link:4, Network_Slice:4, Alarm:4) and D-02 YAML definitions? [Consistency, Spec §SC-002, D-02 §Day 2]

## Scenario Coverage

- [ ] CHK016 Are requirements defined for the scenario where a YAML file contains both `EntityType` and `RelationType` documents (mixed kind in a single file)? [Coverage, Gap]
- [ ] CHK017 Are requirements defined for `LoadFromDir` encountering a subdirectory within the ontology directory — skip, recurse, or error? [Coverage, Gap]
- [ ] CHK018 Is the behavior specified when `LoadFromDir` finds zero `.yaml` files in a valid directory — return empty slices with no error, or return an error? [Coverage, Edge Case]

## Edge Case Coverage

- [ ] CHK019 Are all 6 edge cases from the spec (unsupported type, stableKeys not in properties, uriTemplate invalid variable, trailing `---`, wrong apiVersion, undefined CONNECTS_TO) mapped to specific functional requirements that address them? [Coverage, Spec §Edge Cases]
- [ ] CHK020 Is the behavior specified when a YAML file is valid YAML but contains no `kind` or `apiVersion` fields at all — is this `ErrInvalidSchema` or `ErrUnsupportedKind`? [Edge Case, Gap]

## Dependencies & Assumptions

- [ ] CHK021 Is the assumption that `gopkg.in/yaml.v3` supports multi-document decoding via `yaml.Decoder` loop validated against the library documentation? [Assumption, Spec §Assumptions]
- [ ] CHK022 Is the ontology directory path resolution documented — relative to working directory, or relative to config file location? [Dependency, Spec §Assumptions]

## Notes

- Items CHK002–CHK005 address nil-vs-empty semantics that affect downstream consumer code (Normalizer, GraphAssembler). Gaps here may cause nil pointer panics in later tasks.
- Items CHK007 and CHK009 address the multi-document return convention — the contracts/loader.md mentions "first doc returned via primary return" but this may need explicit spec-level documentation.
- Item CHK013 is critical: CONNECTS_TO appears in 4 spec sections and must remain consistent after any future edits.
