# Gibson GraphRAG Taxonomy

Comprehensive documentation of the Gibson taxonomy system, including file structure, Neo4j integration, loading mechanisms, and extension guides.

**Version:** 0.15.0 (tied to Gibson binary version)
**Last Updated:** 2026-01-18

---

## Table of Contents

- [Overview](#overview)
- [File Structure](#file-structure)
- [Neo4j Integration](#neo4j-integration)
- [Loading Mechanism](#loading-mechanism)
- [Making Updates](#making-updates)
- [Improving the Taxonomy](#improving-the-taxonomy)
- [Runtime Queries](#runtime-queries)
- [Troubleshooting](#troubleshooting)
- [Reference](#reference)

---

## Overview

### Purpose

The Gibson Taxonomy is a YAML-driven, compile-time embedded system that provides declarative configuration for GraphRAG node types, relationship types, attack techniques, target types, capabilities, and execution event mappings.

The taxonomy enables:
- Consistent graph schema across all Gibson deployments
- Declarative event-to-graph mappings for audit trails
- Tool output parsing for automatic asset extraction
- MITRE ATT&CK and custom technique mappings
- Type-safe property definitions with validation

### Design Principles

| Principle | Description |
|-----------|-------------|
| **YAML-Driven** | Human-readable YAML source files for easy editing |
| **Compile-Time Embedding** | Embedded in binary using Go's `embed.FS` |
| **Version-Tied** | Taxonomy version matches Gibson binary version |
| **Additive Custom** | Custom extensions add to, never override, bundled types |
| **Thread-Safe** | Registry uses `RWMutex` for concurrent access |
| **Graceful Degradation** | Missing definitions log warnings, don't crash |

---

## File Structure

**Root Directory:** `internal/graphrag/taxonomy/`

### YAML Files

#### `taxonomy.yaml`

Root configuration file that orchestrates includes.

```yaml
version: "0.15.0"
metadata:
  name: "Gibson GraphRAG Taxonomy"
  description: "Canonical node and relationship types..."
includes:
  - "nodes/assets.yaml"
  - "nodes/findings.yaml"
  - "relationships/asset_hierarchy.yaml"
  # ... more includes
```

#### `nodes/` Directory

| File | Purpose | Node Types |
|------|---------|------------|
| `assets.yaml` | Asset node types discovered during reconnaissance | `domain`, `subdomain`, `host`, `port`, `service`, `endpoint`, `api`, `technology`, `cloud_asset`, `certificate` |
| `findings.yaml` | Security finding node types | `finding`, `evidence`, `mitigation` |
| `execution.yaml` | Execution tracking node types | `mission`, `agent_run`, `tool_execution`, `llm_call`, `intelligence` |
| `attack.yaml` | Attack pattern node types | (if present) |

#### `relationships/` Directory

| File | Purpose | Relationship Types |
|------|---------|-------------------|
| `asset_hierarchy.yaml` | Asset traversal relationships | `HAS_SUBDOMAIN`, `RESOLVES_TO`, `HAS_PORT`, `RUNS_SERVICE`, `HAS_ENDPOINT`, `USES_TECHNOLOGY`, `SERVES_CERTIFICATE`, `HOSTS` |
| `finding_links.yaml` | Finding analysis relationships | `AFFECTS`, `USES_TECHNIQUE`, `HAS_EVIDENCE`, `LEADS_TO`, `MITIGATES`, `SIMILAR_TO`, `EXPLOITS` |
| `execution_context.yaml` | Provenance and audit trail | `PART_OF`, `EXECUTED_BY`, `MADE_CALL`, `DISCOVERED`, `PRODUCED`, `DELEGATED_TO`, `ANALYZES`, `GENERATED_BY`, `PART_OF_MISSION` |

#### `techniques/` Directory

| File | Purpose |
|------|---------|
| `mitre_enterprise.yaml` | MITRE ATT&CK Enterprise technique mappings |
| `arcanum_pi.yaml` | Arcanum PI framework technique mappings |

#### `targets/targets.yaml`

Target type definitions for testable systems:

| Category | Types |
|----------|-------|
| **web** | `http_api`, `graphql`, `web_app` |
| **ai** | `llm_chat`, `llm_api`, `rag`, `ai_agent`, `ai_copilot` |
| **infrastructure** | `kubernetes`, `database`, `network_service` |
| **cloud** | `cloud_service` |
| **blockchain** | `smart_contract` |

#### `technique-types/technique-types.yaml`

Technique type categories mapped to MITRE ATT&CK:

| Category | Techniques |
|----------|------------|
| **initial_access** | `ssrf`, `sqli`, `xss`, `xxe`, `rce`, `lfi`, `rfi`, `auth_bypass`, `idor`, `csrf`, `ssti`, `deserialization`, `prompt_injection`, `jailbreak` |
| **collection** | `data_extraction` |
| **exfiltration** | `data_exfiltration` |
| **impact** | `dos` |
| **privilege_escalation** | `privesc`, `container_escape` |
| **lateral_movement** | `lateral_movement` |
| **credential_access** | `credential_theft` |
| **discovery** | `cloud_metadata` |

#### `capabilities/capabilities.yaml`

High-level testing capabilities bundling technique types:

- `web_vulnerability_scanning` - Common web vulnerabilities
- `api_security_testing` - API-focused security testing
- `llm_security_testing` - LLM security testing
- `infrastructure_testing` - Infrastructure security
- `cloud_security_testing` - Cloud-specific testing
- `dos_testing` - Denial of service testing

#### `execution_events.yaml`

Maps Gibson events to graph node/relationship creation:

| Lifecycle | Events |
|-----------|--------|
| **Mission** | `mission.started`, `mission.completed`, `mission.failed` |
| **Agent** | `agent.started`, `agent.completed`, `agent.failed`, `agent.delegated`, `agent.finding_submitted` |
| **LLM** | `llm.request.started`, `llm.request.completed`, `llm.request.failed`, `llm.stream.started`, `llm.stream.completed` |
| **Tool** | `tool.call.started`, `tool.call.completed`, `tool.call.failed` |
| **Plugin** | `plugin.query.started`, `plugin.query.completed`, `plugin.query.failed` |
| **Finding** | `finding.discovered` |

#### `tool_outputs.yaml`

Tool output parsing schemas for legacy/third-party tools.

> **Note:** Modern tools embed taxonomy using `schema.TaxonomyMapping` in their Go code. Tools like `nmap`, `subfinder`, `httpx`, `nuclei`, `amass`, `masscan` have their schema loaded automatically from tool binaries via `--schema` flag. This file is for tools that cannot be modified or legacy tools not yet migrated.

### Go Files

| File | Purpose |
|------|---------|
| `types.go` | Core type definitions (`Taxonomy`, `NodeTypeDefinition`, `RelationshipTypeDefinition`, etc.) |
| `types_events.go` | Execution event type definitions (`ExecutionEventDefinition`, `ToolOutputSchema`, etc.) |
| `embed.go` | Embeds YAML files into binary at compile time |
| `loader.go` | Loads and parses taxonomy from embedded FS |
| `registry.go` | Thread-safe runtime access to taxonomy |
| `schema_loader.go` | Loads taxonomy from tool binaries with embedded schemas |

---

## Neo4j Integration

### Overview

The taxonomy system integrates with Neo4j through the `TaxonomyGraphEngine`. Events and tool outputs are processed according to taxonomy definitions, creating nodes and relationships in Neo4j with consistent schemas.

### Graph Client

**Location:** `internal/graphrag/graph/neo4j.go`

Features:
- Connection management with exponential backoff retry (5 attempts)
- Auto-detects read vs write operations
- Node operations: `CreateNode`, `DeleteNode` with labels
- Relationship operations: `CreateRelationship` with type safety
- Health checking with 5-second connectivity verification

### Taxonomy Engine

**Location:** `internal/graphrag/engine/taxonomy_engine.go`

```go
type TaxonomyGraphEngine interface {
    HandleEvent(ctx, eventType string, data map[string]any) error
    HandleToolOutput(ctx, toolName string, output map[string]any, agentRunID string) error
    HandleFinding(ctx, finding agent.Finding, missionID string) error
    Health(ctx) HealthStatus
}
```

### Event Processing Flow

1. Event arrives (e.g., `mission.started`)
2. Lookup event definition in registry
3. If `creates_node`: MERGE node with ID template
4. If `updates_node`: MATCH + SET properties
5. For each relationship: MERGE relationship

### Cypher Patterns

**Create Node:**
```cypher
MERGE (n:Mission {id: $id})
SET n += $props
RETURN n
```

**Update Node:**
```cypher
MATCH (n:Mission {id: $id})
SET n += $props
RETURN n
```

**Create Relationship:**
```cypher
MATCH (from {id: $from_id})
MATCH (to {id: $to_id})
MERGE (from)-[r:PART_OF]->(to)
SET r += $props
RETURN r
```

### Node Labels

Neo4j labels use PascalCase names from `NodeTypeDefinition.Name`:

| Type | Neo4j Label |
|------|-------------|
| `domain` | `Domain` |
| `subdomain` | `Subdomain` |
| `host` | `Host` |
| `port` | `Port` |
| `service` | `Service` |
| `endpoint` | `Endpoint` |
| `api` | `API` |
| `technology` | `Technology` |
| `cloud_asset` | `CloudAsset` |
| `certificate` | `Certificate` |
| `finding` | `Finding` |
| `evidence` | `Evidence` |
| `mitigation` | `Mitigation` |
| `mission` | `Mission` |
| `agent_run` | `AgentRun` |
| `tool_execution` | `ToolExecution` |
| `llm_call` | `LLMCall` |
| `intelligence` | `Intelligence` |

### Node ID Patterns

Gibson uses string IDs in the `id` property, not Neo4j's `elementId()`:

| Type | ID Template |
|------|-------------|
| `domain` | `domain:{name}` |
| `subdomain` | `subdomain:{name}` |
| `host` | `host:{ip}` |
| `port` | `port:{host_id}:{number}:{protocol}` |
| `service` | `service:{port_id}:{name}` |
| `endpoint` | `endpoint:{sha256(method:url)[:16]}` |
| `api` | `api:{base_url}` |
| `technology` | `technology:{name}:{version}` |
| `cloud_asset` | `cloud_asset:{provider}:{resource_type}:{arn_or_id}` |
| `certificate` | `certificate:{fingerprint}` |
| `finding` | `finding:{uuid}` |
| `evidence` | `evidence:{uuid}` |
| `mitigation` | `mitigation:{uuid}` |
| `mission` | `mission:{name}` |
| `agent_run` | `agent_run:{trace_id}:{span_id}` |
| `tool_execution` | `tool_execution:{trace_id}:{span_id}:{timestamp}` |
| `llm_call` | `llm_call:{trace_id}:{span_id}:{timestamp}` |
| `intelligence` | `intelligence:{mission_id}:{phase}:{timestamp}` |

### Relationship Types

All uppercase for Neo4j:

**Asset Hierarchy:** `HAS_SUBDOMAIN`, `RESOLVES_TO`, `HAS_PORT`, `RUNS_SERVICE`, `HAS_ENDPOINT`, `USES_TECHNOLOGY`, `SERVES_CERTIFICATE`, `HOSTS`

**Finding Links:** `AFFECTS`, `USES_TECHNIQUE`, `HAS_EVIDENCE`, `LEADS_TO`, `MITIGATES`, `SIMILAR_TO`, `EXPLOITS`

**Execution Context:** `PART_OF`, `EXECUTED_BY`, `MADE_CALL`, `DISCOVERED`, `PRODUCED`, `DELEGATED_TO`, `ANALYZES`, `GENERATED_BY`, `PART_OF_MISSION`

---

## Loading Mechanism

### At Compile Time

YAML files are embedded into the binary using Go's `embed.FS` directive:

```go
//go:embed *.yaml nodes/*.yaml relationships/*.yaml techniques/*.yaml targets/*.yaml technique-types/*.yaml capabilities/*.yaml
var taxonomyFS embed.FS
```

**Benefits:**
- No external file dependencies at runtime
- Version consistency (taxonomy matches binary version)
- Secure (cannot be modified after build)

### At Runtime

**Location:** `internal/init/taxonomy.go`

**Functions:**
- `InitTaxonomy(customPath)` - Load at startup
- `GetTaxonomyRegistry()` - Get loaded registry
- `ValidateTaxonomyOnly(path)` - CI validation mode

**Standard Loading:**

```go
// Load embedded taxonomy only
registry, err := taxonomy.LoadAndValidateTaxonomy()

// Load with custom extensions
registry, err := taxonomy.LoadAndValidateTaxonomyWithCustom(customPath)

// Load with tool schemas from binaries
registry, err := taxonomy.LoadAndValidateTaxonomyWithTools(ctx, toolsDir, logger)
```

**Loading Flow:**

1. Read `taxonomy.yaml` for version and includes list
2. For each included file:
   - Detect file type from path (`nodes/`, `relationships/`, etc.)
   - Parse YAML into appropriate struct
   - Add definitions to taxonomy (checking for duplicates)
3. Build secondary indices (byID maps)
4. If custom path: merge custom definitions (additive only)
5. Validate all definitions
6. Create `TaxonomyRegistry` wrapper

### Custom Taxonomy

Extend taxonomy with custom types:

```yaml
# ~/.gibson/custom-taxonomy/taxonomy.yaml
version: "1.0.0"
includes:
  - "nodes/my-nodes.yaml"
  - "relationships/my-rels.yaml"
```

**Rules:**
- Custom types are additive only
- Cannot override bundled types (error thrown)
- Cannot remove bundled types
- Must pass validation against bundled types

**Usage:**
```bash
gibson daemon start --custom-taxonomy ~/.gibson/custom-taxonomy/taxonomy.yaml
```

### Tool Schema Loading

Tools that embed `schema.TaxonomyMapping` are queried at runtime:

1. Gibson scans tools directory for binaries
2. Each binary is executed with `--schema` flag
3. JSON output parsed as `schema.TaxonomyMapping`
4. Converted to `ToolOutputSchema`
5. Added to registry (takes precedence over YAML)

**Tools with embedded schema:** `nmap`, `subfinder`, `httpx`, `nuclei`, `amass`, `masscan`

---

## Making Updates

### Adding a Node Type

1. Create/edit appropriate file in `nodes/` directory
2. Define node type following schema
3. Add to includes in `taxonomy.yaml` if new file
4. Run validation: `make test`

**Example:**

```yaml
# nodes/assets.yaml
node_types:
  - id: node.asset.container
    name: Container
    type: container
    category: asset
    description: "Container instance in a cluster"
    id_template: "container:{namespace}:{name}"
    properties:
      - name: name
        type: string
        required: true
        description: "Container name"
      - name: namespace
        type: string
        required: true
        description: "Kubernetes namespace"
      - name: image
        type: string
        required: false
        description: "Container image reference"
    examples:
      - name: "my-app"
        namespace: "production"
        image: "myregistry/myapp:v1.2.3"
```

### Adding a Relationship Type

1. Create/edit appropriate file in `relationships/` directory
2. Define relationship with `from_types` and `to_types`
3. Ensure referenced node types exist
4. Run validation

**Example:**

```yaml
# relationships/asset_hierarchy.yaml
relationship_types:
  - id: rel.asset.contains
    name: CONTAINS
    type: CONTAINS
    category: asset_hierarchy
    description: "Container runs inside a pod"
    from_types: [pod]
    to_types: [container]
    bidirectional: false
    properties:
      - name: restart_count
        type: int
        required: false
        description: "Number of container restarts"
```

### Adding an Execution Event

1. Add event definition to `execution_events.yaml`
2. Ensure referenced node types exist in `nodes/*.yaml`
3. Ensure referenced relationship types exist in `relationships/*.yaml`
4. Run validation

**Example:**

```yaml
# execution_events.yaml
execution_events:
  - event_type: custom.event.started
    creates_node:
      type: custom_execution
      id_template: "custom:{trace_id}:{span_id}"
      properties:
        - source: custom_field
          target: custom_property
        - value: "custom_value"
          target: status
    creates_relationships:
      - type: PART_OF
        from_template: "custom:{trace_id}:{span_id}"
        to_template: "{mission_id}"
```

### Adding a Tool Output Schema

**Recommended Approach:** Embed taxonomy in the tool binary using `schema.TaxonomyMapping`. See `opensource/tools/` for examples.

**Legacy Approach:**

```yaml
# tool_outputs.yaml
tool_outputs:
  - tool: my-scanner
    description: "Extract hosts from my-scanner output"
    output_format: json
    extracts:
      - node_type: host
        json_path: "results.hosts[*]"
        id_template: "host:{ip}"
        properties:
          - json_path: "ip_address"
            target: "ip"
          - json_path: "hostname"
            target: "hostname"
        relationships:
          - type: DISCOVERED
            to_template: "{_context.agent_run_id}"
```

### Adding a Technique Type

1. Add to `technique-types/technique-types.yaml`
2. Map to MITRE ATT&CK IDs
3. Add to appropriate capability if applicable

**Example:**

```yaml
# technique-types/technique-types.yaml
technique_types:
  - id: "technique.initial_access.graphql_injection"
    type: "graphql_injection"
    name: "GraphQL Injection"
    category: "initial_access"
    mitre_ids:
      - "T1190"
    description: "GraphQL injection allowing query manipulation"
    default_severity: "high"
```

### Adding a Capability

1. Add to `capabilities/capabilities.yaml`
2. Reference existing technique type IDs

**Example:**

```yaml
# capabilities/capabilities.yaml
capabilities:
  - id: "capability.graphql_security_testing"
    name: "GraphQL Security Testing"
    description: "GraphQL-specific security testing"
    technique_types:
      - "graphql_injection"
      - "idor"
      - "auth_bypass"
```

### Adding a Target Type

1. Add to `targets/targets.yaml`
2. Define connection schema with required/optional fields

**Example:**

```yaml
# targets/targets.yaml
target_types:
  - id: "target.messaging.kafka"
    type: "kafka"
    name: "Apache Kafka"
    category: "messaging"
    description: "Apache Kafka message broker"
    connection_schema:
      required:
        - "bootstrap_servers"
      optional:
        - "security_protocol"
        - "sasl_mechanism"
        - "username"
        - "password"
```

### Version Bump

**When:** After significant taxonomy changes

**Steps:**
1. Update version in `taxonomy.yaml`
2. Update Gibson version (they're tied)
3. Run full test suite
4. Update downstream consumers

---

## Improving the Taxonomy

### When to Add Types

- New asset category discovered in security testing
- New relationship pattern needed for attack chains
- New technique category not covered by existing types
- New target system type to support

### When NOT to Add Types

- One-off custom needs (use custom taxonomy instead)
- Temporary testing purposes
- Types that duplicate existing functionality

### Best Practices

**Node Types:**
- Use clear, domain-specific type names
- Include descriptive `id_template` for deterministic IDs
- Mark truly required properties as `required: true`
- Provide examples for documentation
- Group related types in the same file

**Relationships:**
- Use uppercase `SCREAMING_SNAKE_CASE` names
- Clearly define `from_types` and `to_types` constraints
- Use `bidirectional: true` only when semantically appropriate
- Add relationship properties for timestamps and context

**ID Templates:**
- Use deterministic templates for idempotent MERGE
- Include enough fields to ensure uniqueness
- Use `sha256()` for collision-prone combinations
- Use `uuid` for globally unique items (findings)

**Event Mappings:**
- Map all relevant event data to node properties
- Mark optional fields as `optional: true`
- Create relationships to establish provenance
- Use MERGE for idempotent operations

### Validation

**Automatic:**
- Duplicate ID detection
- Duplicate Type detection
- Reference validation (nodes, relationships)
- Event schema validation
- Tool output schema validation

**Manual Review:**
- Semantic correctness of relationships
- Appropriate property types
- Complete examples
- Documentation accuracy

---

## Runtime Queries

### Getting the Registry

```go
import "github.com/zero-day-ai/gibson/internal/init"

// Get the loaded registry
registry := init.GetTaxonomyRegistry()
```

### Querying Node Types

```go
// Get specific node type
nodeDef, found := registry.NodeType("host")
if found {
    fmt.Printf("ID Template: %s\n", nodeDef.IDTemplate)
    fmt.Printf("Properties: %v\n", nodeDef.Properties)
}

// Check if type is canonical
if registry.IsCanonicalNodeType("my_custom_type") {
    // It's in the official taxonomy
}

// List all node types
for _, nodeType := range registry.NodeTypes() {
    fmt.Printf("%s: %s\n", nodeType.Type, nodeType.Description)
}
```

### Querying Relationships

```go
// Get specific relationship
relDef, found := registry.RelationshipType("HAS_PORT")
if found {
    fmt.Printf("From: %v, To: %v\n", relDef.FromTypes, relDef.ToTypes)
}

// List all relationships
for _, relType := range registry.RelationshipTypes() {
    fmt.Printf("%s: %s -> %s\n", relType.Type, relType.FromTypes, relType.ToTypes)
}
```

### Generating Node IDs

```go
// Generate deterministic node ID from template
nodeID, err := registry.GenerateNodeID("host", map[string]any{
    "ip": "192.168.1.1",
})
// Result: "host:192.168.1.1"

// With hash function
nodeID, err := registry.GenerateNodeID("endpoint", map[string]any{
    "method": "GET",
    "url": "https://api.example.com/users",
})
// Result: "endpoint:a1b2c3d4e5f6..."
```

### Querying Techniques

```go
// Get MITRE technique
technique, found := registry.Technique("T1190")

// Get all techniques from a source
mitreTechniques := registry.Techniques("mitre")
arcanumTechniques := registry.Techniques("arcanum")
allTechniques := registry.Techniques("")
```

### Querying Capabilities

```go
// Get capability
cap, found := registry.GetCapability("capability.web_vulnerability_scanning")
if found {
    fmt.Printf("Technique Types: %v\n", cap.TechniqueTypes)
}

// Get technique types for a capability
techniqueTypes := registry.GetTechniqueTypesForCapability("capability.llm_security_testing")
```

### Checking Event Support

```go
// Check if event type is supported
if registry.HasExecutionEvent("mission.started") {
    eventDef := registry.GetExecutionEvent("mission.started")
    // Process event definition
}

// List all supported events
eventTypes := registry.ListExecutionEvents()
```

### Checking Tool Schemas

```go
// Check if tool has output schema
if registry.HasToolOutputSchema("nmap") {
    schema := registry.GetToolOutputSchema("nmap")
    fmt.Printf("Extracts: %d rules\n", len(schema.Extracts))
}

// Check if schema came from binary
if registry.IsSchemaBasedToolSchema("nmap") {
    // Schema was loaded from tool binary via --schema
}
```

---

## Troubleshooting

### Taxonomy Not Loading

**Symptoms:**
- `nil` registry returned
- "taxonomy validation failed"

**Solutions:**
- Check YAML syntax (run `yamllint`)
- Ensure all includes exist
- Check for duplicate IDs or types
- Verify reference targets exist

### Custom Taxonomy Rejected

**Symptoms:**
- "conflicts with bundled taxonomy"

**Cause:** Attempting to override bundled types

**Solution:** Custom types must have unique IDs and types

### Event Not Creating Nodes

**Symptoms:**
- Event processed but no graph changes

**Solutions:**
- Check event type matches `execution_events.yaml`
- Verify data contains required fields
- Check Neo4j connectivity
- Look for warnings in logs

### Tool Schema Not Loaded

**Symptoms:**
- Tool output not extracted

**Solutions:**
- Check if tool has embedded schema
- Verify tool binary is executable
- Check `--schema` flag output
- Add YAML schema for legacy tools

### Neo4j Errors

**Symptoms:**
- "failed to create node/relationship"

**Solutions:**
- Check Neo4j connectivity
- Verify Cypher syntax in engine
- Check node ID uniqueness
- Verify referenced nodes exist

---

## Reference

### Property Types

| Type | Description |
|------|-------------|
| `string` | UTF-8 string |
| `int` | 64-bit integer |
| `float64` | 64-bit floating point |
| `bool` | Boolean true/false |
| `[]string` | Array of strings |
| `map[string]any` | Arbitrary JSON object |

### ID Template Functions

| Function | Example |
|----------|---------|
| Simple substitution | `{field}` |
| UUID generation | `{uuid}` |
| Hash generation | `{sha256(expr)}` |
| Hash truncation | `{sha256(expr)[:16]}` |

### Event Property Mapping

| Type | Syntax |
|------|--------|
| Source-based | `source: field_name` (from event data) |
| Static value | `value: constant` (fixed value) |
| Optional flag | `optional: true` (don't fail if missing) |

### Cypher Operations

| Operation | Pattern |
|-----------|---------|
| Node creation | `MERGE (n:Label {id: $id}) SET n += $props` |
| Node update | `MATCH (n:Label {id: $id}) SET n += $props` |
| Relationship creation | `MATCH ... MERGE (a)-[r:TYPE]->(b)` |

### Gibson CLI Commands

```bash
gibson taxonomy validate [path]
gibson taxonomy list-types
gibson taxonomy show-type <type>
```
