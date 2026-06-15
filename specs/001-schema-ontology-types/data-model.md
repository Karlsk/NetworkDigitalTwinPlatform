# Data Model: Schema Data Structures and Ontology YAML Definitions

**Feature**: `001-schema-ontology-types` | **Date**: 2026-06-15

## Entity Relationship Overview

```
EntityType 1──N PropertySpec
EntityType 1──1 IdentitySpec
EntityType 1──N NormalizeRule
EntityType 1──N RelationFieldSpec
RelationType 1──N source (EntityType names)
RelationType 1──N target (EntityType names)
```

## Type Definitions

### Metadata

Shared metadata for both EntityType and RelationType.

| Field | Type | YAML Key | Required | Description |
|-------|------|----------|----------|-------------|
| Name | `string` | `name` | ✅ | Unique identifier (e.g., "Device", "HAS_INTERFACE") |
| Labels | `[]string` | `labels` | ❌ | Classification tags (e.g., `[Resource, Network]`) |

### EntityType

Top-level structure for entity type definitions (K8s CRD style).

| Field | Type | YAML Key | Required | Description |
|-------|------|----------|----------|-------------|
| APIVersion | `string` | `apiVersion` | ✅ | Must be `twin.io/v1` |
| Kind | `string` | `kind` | ✅ | Must be `EntityType` |
| Metadata | `Metadata` | `metadata` | ✅ | Name and labels |
| Spec | `EntityTypeSpec` | `spec` | ✅ | Entity type specification |

### EntityTypeSpec

Detailed specification within an EntityType.

| Field | Type | YAML Key | Required | Description |
|-------|------|----------|----------|-------------|
| Identity | `IdentitySpec` | `identity` | ✅ | Immutable identity definition |
| URITemplate | `string` | `uriTemplate` | ✅ | URI pattern using `{fieldName}` placeholders |
| FieldMapping | `map[string]string` | `fieldMapping` | ❌ | Legacy field → canonical field name mapping |
| Normalize | `[]NormalizeRule` | `normalize` | ❌ | Field-level string transformation rules |
| RelationFields | `map[string]RelationFieldSpec` | `relationFields` | ❌ | Property fields that derive relations |
| Properties | `map[string]PropertySpec` | `properties` | ✅ | Property definitions |

### IdentitySpec

Defines the immutable identity of an entity.

| Field | Type | YAML Key | Required | Description |
|-------|------|----------|----------|-------------|
| StableKeys | `[]string` | `stableKeys` | ✅ | Property names forming the permanent identity |

**Constraint**: StableKeys values MUST reference field names defined in `Properties`. These fields MUST be marked `required: true`.

### NormalizeRule

Defines a field-level string transformation.

| Field | Type | YAML Key | Required | Description |
|-------|------|----------|----------|-------------|
| Field | `string` | `field` | ✅ | Target property name |
| Pattern | `string` | `pattern` | ✅ | String pattern to match |
| Replace | `string` | `replace` | ✅ | Replacement string |

### RelationFieldSpec

Maps a property field to a relation type.

| Field | Type | YAML Key | Required | Description |
|-------|------|----------|----------|-------------|
| RelationType | `string` | `relationType` | ✅ | Name of the RelationType this field derives |

### PropertySpec

Defines a single property within an EntityType.

| Field | Type | YAML Key | Required | Description |
|-------|------|----------|----------|-------------|
| Type | `string` | `type` | ✅ | Data type: `string`, `int`, `bool` |
| Required | `bool` | `required` | ❌ | Whether the property is mandatory (default: false) |
| Enum | `[]string` | `enum` | ❌ | Allowed values (empty = unconstrained) |
| Default | `any` | `default` | ❌ | Default value (string, int, or bool) |

**Constraint**: `Type` must be one of: `string`, `int`, `bool` (MVP scope).

### RelationType

Top-level structure for relation type definitions.

| Field | Type | YAML Key | Required | Description |
|-------|------|----------|----------|-------------|
| APIVersion | `string` | `apiVersion` | ✅ | Must be `twin.io/v1` |
| Kind | `string` | `kind` | ✅ | Must be `RelationType` |
| Metadata | `Metadata` | `metadata` | ✅ | Name (labels optional) |
| Spec | `RelationTypeSpec` | `spec` | ✅ | Relation type specification |

### RelationTypeSpec

Defines the source and target of a relation.

| Field | Type | YAML Key | Required | Description |
|-------|------|----------|----------|-------------|
| Source | `[]string` | `source` | ✅ | Allowed source EntityType names |
| Target | `[]string` | `target` | ✅ | Allowed target EntityType names |

## Ontology Instances

### EntityTypes (6 definitions)

| Name | stableKeys | uriTemplate | Properties | Labels |
|------|-----------|-------------|------------|--------|
| Device | `[serial_number]` | `device:{serial_number}` | 8 | `[Resource, Network]` |
| Interface | `[device_serial, if_name]` | `iface:{device_serial}_{if_name}` | 5 | `[Resource, Network]` |
| ISIS | `[isis_id]` | `isis:{isis_id}` | 5 | `[Protocol, Network]` |
| Link | `[link_id]` | `link:{link_id}` | 4 | `[Resource, Network]` |
| Network_Slice | `[slice_id]` | `slice:{slice_id}` | 4 | `[Service, Network]` |
| Alarm | `[alarm_id]` | `alarm:{alarm_id}` | 4 | `[Event, Network]` |

### RelationTypes (4 definitions)

| Name | Source | Target |
|------|--------|--------|
| HAS_INTERFACE | `[Device]` | `[Interface]` |
| RUNS_ON | `[ISIS]` | `[Interface]` |
| ENDPOINT | `[Link]` | `[Interface]` |
| OCCURRED_ON | `[Alarm]` | `[Interface]` |

### Deferred Relations

| Name | Referenced By | Status |
|------|---------------|--------|
| CONNECTS_TO | Device.relationFields | Deferred to I-02 validation task |

## Validation Rules (from spec)

| Rule ID | Constraint | Enforcement Time |
|---------|-----------|-----------------|
| V-001 | `apiVersion` must be `twin.io/v1` | Parse time |
| V-002 | `kind` must be `EntityType` or `RelationType` | Parse time |
| V-003 | `identity.stableKeys` fields must exist in `properties` | Validation time (I-02) |
| V-004 | `uriTemplate` variables must be in `stableKeys` | Validation time (I-02) |
| V-005 | `stableKeys` fields must be `required: true` | Validation time (I-02) |
| V-006 | `relationFields[*].relationType` must be a defined RelationType | Validation time (I-02) |
| V-007 | RelationType `source`/`target` must be defined EntityTypes | Validation time (I-02) |

## Sentinel Errors

| Error | Message | Used By |
|-------|---------|---------|
| `ErrInvalidSchema` | `invalid schema` | Parse failures (malformed YAML, missing fields) |
| `ErrUnsupportedKind` | `unsupported kind` | Unknown `kind` value in YAML |
| `ErrInvalidAPIVersion` | `invalid api version` | `apiVersion` != `twin.io/v1` |
