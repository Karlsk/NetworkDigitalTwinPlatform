# Feature Specification: Core Interface Definitions (D-03)

**Feature Branch**: `002-core-interface-definitions`

**Created**: 2026-06-15

**Status**: Draft

**Input**: User description: "实现核心接口定义任务 — SchemaRegistry、Connector、GraphDB 三大核心接口契约"

## Clarifications

### Session 2026-06-22

- Q: Should D-03 define companion types (Resource, ConnectorMetadata, Node, Relation) needed for interface compilation, or defer them to D-04? → A: D-03 includes companion type definitions — minimal structs (Resource, ConnectorMetadata in `connector/types.go`; Node, Relation in `assembler/types.go`) defined alongside the interfaces to ensure `go build ./...` passes. No logic, just data structures.
- Q: Should SchemaRegistry.Validate mutate the input map to fill defaults, or separate validation from default filling? → A: Separate concerns — `Validate` only checks constraints (required, type, enum, stableKeys) and returns error. A new `ApplyDefaults(entityKind string, props map[string]any) (map[string]any, error)` method returns a new map with defaults filled, preserving immutability. SchemaRegistry now has 7 methods.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ontology Schema Discovery and Validation (Priority: P1)

Downstream system modules (Normalizer, Validator, GraphAssembler) need a single, authoritative source to discover and validate network ontology definitions. When the system starts up, the Schema Registry loads all YAML ontology files and makes EntityType and RelationType definitions queryable by name. Consumers can look up entity types to perform field mapping and URI generation, look up relation types to derive graph edges, and validate that incoming data conforms to the defined schema before processing.

**Why this priority**: The Schema Registry is the foundation of the schema-driven zero-code extension principle. Without it, no downstream module can interpret ontology definitions or validate data — the entire layered pipeline depends on schema availability.

**Independent Test**: Can be fully tested by loading a directory of YAML ontology files, querying EntityType and RelationType by name, listing all registered types, and validating sample property maps against the schema. Delivers immediate value by enabling all schema-dependent modules.

**Acceptance Scenarios**:

1. **Given** a directory containing valid YAML ontology files, **When** the Schema Registry loads the directory, **Then** all EntityType and RelationType definitions are registered and queryable by name.
2. **Given** a loaded Schema Registry, **When** a consumer queries for an existing EntityType by name, **Then** the complete EntityType definition is returned with no error.
3. **Given** a loaded Schema Registry, **When** a consumer queries for a non-existent EntityType, **Then** a clear "not found" error is returned (not a nil pointer or panic).
4. **Given** a loaded Schema Registry, **When** a consumer lists all EntityTypes, **Then** every registered EntityType is returned in the result.
5. **Given** a loaded Schema Registry, **When** a consumer lists all RelationTypes, **Then** every registered RelationType is returned in the result.
6. **Given** a loaded Schema Registry, **When** a consumer validates a property map against an EntityType, **Then** the system reports specific validation failures (missing required fields, type mismatches, invalid enum values, empty stableKeys) or confirms validity — without modifying the input map.
7. **Given** a loaded Schema Registry, **When** a consumer applies defaults for an EntityType to a property map with missing optional fields, **Then** the system returns a new map with default values filled in, leaving the original map unchanged.

---

### User Story 2 - Data Source Adapter Contract (Priority: P1)

The Sync Service needs a uniform contract to interact with heterogeneous data sources (Netbox, Controllers, CMDB, etc.). Each data source implements the Connector interface, declaring its metadata (name, supported entity types) and providing two data retrieval modes: full collection (pull all data for an entity type) and streaming (incremental change events). A Connector Registry manages the lifecycle of registered connectors, allowing the Sync Service to discover and invoke them by name.

**Why this priority**: Without the Connector contract, the system cannot ingest any data. The interface must be defined before any connector implementation or the sync pipeline can be built. It is co-equal in priority with Schema Registry since both are foundational.

**Independent Test**: Can be fully tested by implementing a mock Connector, registering it in the Connector Registry, retrieving it by name, invoking full collection, and verifying the returned data structure. Delivers immediate value by enabling the sync pipeline.

**Acceptance Scenarios**:

1. **Given** a Connector implementation with valid metadata, **When** it is registered in the Connector Registry, **Then** it can be retrieved by its declared name.
2. **Given** a registered Connector, **When** the Sync Service requests full collection for a supported entity type, **Then** the Connector returns all resources of that type.
3. **Given** a registered Connector, **When** the Sync Service requests streaming for an entity type not yet supported for streaming, **Then** the Connector returns a "not implemented" error (not a panic or hang).
4. **Given** a Connector Registry with multiple connectors, **When** a consumer lists all connectors, **Then** metadata for every registered connector is returned.
5. **Given** a Connector Registry, **When** a consumer attempts to retrieve a non-existent connector, **Then** a clear "not found" error is returned.

---

### User Story 3 - Graph Database Driver Contract (Priority: P1)

The Sync Service, Snapshot Manager, and MCP Server need a uniform contract to interact with the graph database. The GraphDB interface defines the complete set of operations: connectivity management, bulk creation for full syncs, incremental upserts and deletes for change events, Cypher query execution, logical multi-database management, and Cypher preview for testing and auditing. All write operations carry a logical database identifier to support multi-tenant isolation.

**Why this priority**: The GraphDB contract is the terminal layer of the data pipeline — no data reaches the graph without it. It is equally foundational with Schema Registry and Connector.

**Independent Test**: Can be fully tested by implementing a mock GraphDB, invoking each operation (ping, bulk create, upsert, delete, query, clear, clone, list), and verifying correct behavior and error handling. Delivers immediate value by enabling sync, snapshot, and query modules.

**Acceptance Scenarios**:

1. **Given** a GraphDB implementation connected to a running database, **When** a connectivity check is performed, **Then** the system reports connection status (success or failure with details).
2. **Given** a GraphDB implementation and a set of graph nodes and relations, **When** bulk creation is requested for a logical database, **Then** all nodes and relations are created in that logical database.
3. **Given** existing graph data, **When** an upsert operation is performed with updated node properties, **Then** existing nodes receive incremental property merges and new nodes/relations are created.
4. **Given** existing graph data, **When** specific relations are deleted, **Then** only those relations are removed while nodes remain intact.
5. **Given** existing graph data, **When** nodes are deleted by URI, **Then** the nodes and all their connected relations are removed (cascade delete).
6. **Given** a logical database with data, **When** a Cypher query is executed, **Then** results are returned filtered to that logical database.
7. **Given** graph data to be written, **When** a Cypher preview is requested, **Then** the generated Cypher statement and parameters are returned without executing them.
8. **Given** a logical database with data, **When** a clear operation is performed, **Then** all data in that logical database is removed.
9. **Given** a source logical database with data, **When** a clone operation targets a new logical database name, **Then** the target database contains an exact copy of the source data.
10. **Given** multiple logical databases exist, **When** a list operation is performed, **Then** all logical database names are returned.
11. **Given** a logical database name, **When** an existence check is performed, **Then** the system reports whether that database contains data.

---

### Edge Cases

- What happens when the ontology directory is empty or contains no valid YAML files? The Schema Registry load should return a clear error indicating no schemas were found.
- What happens when the ontology directory contains malformed YAML? The Schema Registry should report the specific file and parse error, failing fast at startup.
- What happens when a Connector's Collect operation times out or the external data source is unreachable? The Connector should return an error with context; it must not hang indefinitely.
- What happens when a Connector returns zero resources for an entity type? This is valid — the caller should handle empty results gracefully.
- What happens when GraphDB operations are invoked on a non-existent logical database? Write operations should create the database implicitly; query operations should return empty results or a "not found" error.
- What happens when BuildCypher is called with an unknown action string? It should return an empty statement or a clear error.
- How does the system handle concurrent registrations in the Connector Registry? The registry should be safe for concurrent reads; write concurrency is deferred to V1.
- What happens when SchemaRegistry.Validate encounters an unknown entity type? It should return a "schema not found" error, not panic.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide a Schema Registry that loads all YAML ontology definitions from a specified directory, supporting multi-document YAML files.
- **FR-002**: System MUST allow querying EntityType definitions by name, returning the complete definition or a "not found" error.
- **FR-003**: System MUST allow querying RelationType definitions by name, returning the complete definition or a "not found" error.
- **FR-004**: System MUST allow listing all registered EntityTypes.
- **FR-005**: System MUST allow listing all registered RelationTypes.
- **FR-006**: System MUST validate a property map against an EntityType definition, checking required fields, data types, enum constraints, and stableKeys non-emptiness — without modifying the input map.
- **FR-006a**: System MUST provide an ApplyDefaults method that returns a new property map with schema-defined default values filled in for missing optional fields, leaving the original map unchanged.
- **FR-007**: System MUST provide a Connector interface with metadata declaration, full data collection, and incremental streaming capabilities.
- **FR-008**: System MUST provide a Connector Registry that allows registering, retrieving, and listing connectors by name.
- **FR-009**: Connector streaming MUST return a "not implemented" error for MVP-phase connectors that do not yet support incremental streaming.
- **FR-010**: System MUST provide a GraphDB interface covering connectivity management (ping, close), bulk creation, incremental upsert, relation deletion, node deletion by URI, Cypher query execution, Cypher preview, logical database management (clear, clone, list, existence check).
- **FR-011**: All GraphDB write and query operations MUST accept a logical database identifier parameter to support multi-tenant isolation.
- **FR-012**: GraphDB Cypher preview MUST generate the Cypher statement and parameter map for create, upsert, delete, and delete-relations actions without executing them.
- **FR-013**: All interface definitions MUST be in separate files with no implementation logic — only type signatures. Companion data structures (Resource, ConnectorMetadata, Node, Relation) MUST be defined as minimal structs in their respective `types.go` files.
- **FR-014**: The system MUST compile successfully with all interface definitions and companion types in place (build verification).
- **FR-015**: Sentinel errors referenced by interfaces MUST be defined in their respective packages (e.g., `ErrNotImplemented` in connector package).

### Key Entities

- **EntityType**: Defines a network entity's ontology — its identity (stableKeys), URI template, field mappings, normalization rules, relation fields, and property specifications. Queried by Normalizer and GraphAssembler.
- **RelationType**: Defines a directed relationship between entity types — source and target type constraints. Queried by GraphAssembler.
- **Resource** *(defined in D-03)*: Raw data unit produced by a Connector — contains entity kind (string), original ID (string), and raw properties (map). Defined in `internal/connector/types.go`.
- **ConnectorMetadata** *(defined in D-03)*: Describes a Connector's identity and capabilities — name (string), type (string), and supported entity types (string slice). Defined in `internal/connector/types.go`.
- **Node** *(defined in D-03)*: A graph node in the GraphModel IR — label (string), URI (string), and properties (map). Defined in `internal/assembler/types.go`. Consumed by GraphDB.
- **Relation** *(defined in D-03)*: A graph edge in the GraphModel IR — type (string), from URI (string), to URI (string), and properties (map). Defined in `internal/assembler/types.go`. Consumed by GraphDB.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All three interface files compile without errors in a clean build (`go build ./...` exits with zero status).
- **SC-002**: Mock implementations can satisfy each interface, proving the contracts are implementable without real infrastructure (Neo4j, external APIs).
- **SC-003**: Schema Registry interface exposes exactly 7 methods (Load, GetEntityType, GetRelationType, ListEntityTypes, ListRelationTypes, Validate, ApplyDefaults).
- **SC-004**: Connector interface exposes exactly 3 methods (Metadata, Collect, Stream).
- **SC-005**: Connector Registry provides exactly 3 methods (Register, Get, List).
- **SC-006**: GraphDB interface exposes exactly 12 methods (Ping, Close, BulkCreate, Upsert, DeleteRelations, DeleteByURIs, Query, BuildCypher, ClearDB, CloneDB, ListDBs, HasDB).
- **SC-007**: Every GraphDB write and query method includes a logical database identifier parameter.
- **SC-008**: Interface definitions contain zero implementation logic — only type signatures, comments, and struct field declarations.
- **SC-009**: All interface method signatures are consistent with the architecture design documents and the layered IR pipeline contract.

## Assumptions

- The EntityType and RelationType data structures (from D-01) are already defined and available in the `internal/schema` package.
- The Node and Relation graph model types are defined within D-03 scope in `internal/assembler/types.go` — minimal structs (Label, URI, Props fields only) to satisfy GraphDB interface compilation.
- The Resource and ConnectorMetadata types are defined within D-03 scope in `internal/connector/types.go` — minimal structs to satisfy Connector interface compilation.
- The Connector interface uses `context.Context` for timeout and cancellation control on all data operations.
- The Connector Registry uses the Connector's declared metadata name as the map key.
- The Connector's Stream method returns a receive-only channel for MVP phase; real implementations (Kafka) deferred to V1.
- Interface files contain only signatures — no implementation code, no business logic, no database access.
- Go's standard `errors` package is used for sentinel error definitions (e.g., `ErrNotImplemented`).
- The `assembler` package types (Node, Relation) are referenced by import path in the GraphDB interface, establishing the GraphModel IR as the canonical graph data contract.
