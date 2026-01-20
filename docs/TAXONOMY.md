# Gibson GraphRAG Taxonomy

Comprehensive documentation of the Gibson taxonomy system, including node types, relationship types, domain types, and the GraphLoader system.

**Version:** 0.20.0 (tied to Gibson/SDK version)
**Last Updated:** 2026-01-20

---

## Table of Contents

- [Overview](#overview)
- [Domain Types System (NEW)](#domain-types-system-new)
- [Node Types](#node-types)
- [Relationship Types](#relationship-types)
- [Neo4j Integration](#neo4j-integration)
- [GraphLoader](#graphloader)
- [Custom Types and Extensibility](#custom-types-and-extensibility)
- [Migration from Old System](#migration-from-old-system)
- [Runtime Queries](#runtime-queries)
- [Troubleshooting](#troubleshooting)
- [Reference](#reference)

---

## Overview

### Purpose

The Gibson Taxonomy defines the schema for the GraphRAG knowledge graph, including:
- **Node types**: Assets, findings, execution tracking
- **Relationship types**: How nodes connect to each other
- **Domain types**: Strongly-typed Go structs for graph entities (SDK)
- **GraphLoader**: Generic loader that accepts domain types (Gibson)

### Architecture Change (v0.20.0)

In v0.20.0, the taxonomy system was significantly simplified:

**Old System (Deprecated):**
- Complex JSONPath-based taxonomy mappings
- YAML schema definitions with template-based IDs
- ~19,000 lines of parsing/extraction code

**New System (Current):**
- **SDK owns the types**: All domain types (`Host`, `Port`, `Service`) live in `sdk/graphrag/domain/`
- **Tools emit typed objects**: No raw JSON with taxonomy mappings
- **Gibson loads generically**: Single `GraphLoader` accepts any `GraphNode` implementer
- **Extensibility via CustomEntity**: Custom types use prefixes like `k8s:pod`

### Design Principles

| Principle | Description |
|-----------|-------------|
| **Type-Safe** | Domain types provide compile-time checking |
| **SDK-First** | Types defined in SDK, loaded by Gibson |
| **Simple Interface** | `GraphNode` interface is the contract |
| **Extensible** | `CustomEntity` for agent-specific types |
| **Deterministic IDs** | Content-addressable IDs from identifying properties |
| **Automatic Relationships** | Parent references create relationships automatically |

---

## Domain Types System (NEW)

The domain types system replaces the old JSONPath-based taxonomy mapping with strongly-typed Go structs. This provides compile-time safety and a simpler, more maintainable codebase.

### GraphNode Interface

All domain types implement this interface (defined in SDK):

```go
type GraphNode interface {
    // NodeType returns the canonical node type (e.g., "host", "port")
    NodeType() string

    // IdentifyingProperties returns properties that uniquely identify this node
    IdentifyingProperties() map[string]any

    // Properties returns all properties to set on the node
    Properties() map[string]any

    // ParentRef returns reference to parent node for relationship creation
    ParentRef() *NodeRef

    // RelationshipType returns the relationship type to parent
    RelationshipType() string
}
```

### Using Domain Types in Tools

```go
import "github.com/zero-day-ai/sdk/graphrag/domain"

func executeScan(ctx context.Context, input map[string]any) (*domain.DiscoveryResult, error) {
    result := domain.NewDiscoveryResult()

    // Create strongly-typed host
    result.Hosts = append(result.Hosts, &domain.Host{
        IP:       "192.168.1.10",
        Hostname: "web-server.example.com",
        State:    "up",
    })

    // Create port with automatic parent relationship
    result.Ports = append(result.Ports, &domain.Port{
        HostID:   "192.168.1.10",
        Number:   443,
        Protocol: "tcp",
        State:    "open",
    })

    // Create service
    result.Services = append(result.Services, &domain.Service{
        PortID:  "192.168.1.10:443:tcp",
        Name:    "https",
        Version: "nginx/1.18.0",
    })

    return result, nil
}
```

### DiscoveryResult Container

The `DiscoveryResult` is the standard container for tool and agent output:

```go
type DiscoveryResult struct {
    Hosts         []*Host
    Ports         []*Port
    Services      []*Service
    Endpoints     []*Endpoint
    Domains       []*Domain
    Subdomains    []*Subdomain
    Technologies  []*Technology
    Certificates  []*Certificate
    CloudAssets   []*CloudAsset
    APIs          []*API
    Custom        []GraphNode  // For CustomEntity and custom implementations
}
```

### Benefits Over Old System

| Aspect | Old System | New System |
|--------|-----------|-----------|
| **Type Safety** | Runtime validation | Compile-time checking |
| **Error Detection** | Silent failures possible | Explicit errors with context |
| **Code Size** | ~19,000 lines | ~500 lines |
| **Relationships** | Manual template strings | Automatic via ParentRef() |
| **Extensibility** | YAML schema changes | CustomEntity for new types |
| **Debugging** | Complex JSONPath traces | Standard Go debugging |

---

## Node Types

### Asset Node Types

| Node Type | Description | Identifying Properties |
|-----------|-------------|----------------------|
| `host` | Network host/IP address | `ip` |
| `port` | Network port on a host | `host_id`, `number`, `protocol` |
| `service` | Service running on a port | `port_id`, `name` |
| `endpoint` | HTTP/API endpoint | `service_id`, `url`, `method` |
| `domain` | DNS domain name | `name` |
| `subdomain` | Subdomain of a domain | `name`, `domain_name` |
| `api` | Web API service | `base_url` |
| `technology` | Detected technology/framework | `name`, `version` |
| `certificate` | TLS/SSL certificate | `serial_number` |
| `cloud_asset` | Cloud infrastructure resource | `provider`, `region`, `resource_id` |

### Execution Node Types

| Node Type | Description | Identifying Properties |
|-----------|-------------|----------------------|
| `mission` | Mission execution | `name`, `timestamp` |
| `agent_run` | Agent execution in a mission | `mission_id`, `agent_name`, `run_number` |
| `tool_execution` | Tool invocation | `agent_run_id`, `tool_name`, `sequence` |
| `llm_call` | LLM API call | `agent_run_id`, `sequence` |

### Finding Node Types

| Node Type | Description | Identifying Properties |
|-----------|-------------|----------------------|
| `finding` | Security finding | `mission_id`, `fingerprint` |
| `evidence` | Evidence for a finding | `finding_id`, `type`, `fingerprint` |
| `mitigation` | Mitigation for a finding | `finding_id`, `title` |

---

## Relationship Types

### Asset Hierarchy

| Relationship | From | To | Description |
|--------------|------|----|----|
| `HAS_SUBDOMAIN` | Domain | Subdomain | Domain contains subdomain |
| `RESOLVES_TO` | Subdomain | Host | DNS resolution |
| `HAS_PORT` | Host | Port | Host has open port |
| `RUNS_SERVICE` | Port | Service | Port runs service |
| `HAS_ENDPOINT` | Service | Endpoint | Service exposes endpoint |
| `USES_TECHNOLOGY` | * | Technology | Uses a technology |
| `SERVES_CERTIFICATE` | Host | Certificate | TLS certificate |

### Finding Links

| Relationship | From | To | Description |
|--------------|------|----|----|
| `AFFECTS` | Finding | * | Finding affects asset |
| `USES_TECHNIQUE` | Finding | Technique | Attack technique used |
| `HAS_EVIDENCE` | Finding | Evidence | Supporting evidence |
| `LEADS_TO` | Finding | Finding | Finding chain |
| `MITIGATES` | Mitigation | Finding | Remediation |

### Execution Context

| Relationship | From | To | Description |
|--------------|------|----|----|
| `PART_OF_MISSION` | AgentRun | Mission | Agent in mission |
| `EXECUTED` | AgentRun | ToolExecution | Agent executed tool |
| `DISCOVERED` | ToolExecution | * | Tool discovered asset |
| `PRODUCED` | ToolExecution | Finding | Tool produced finding |

---

## Neo4j Integration

### Overview

The GraphLoader integrates with Neo4j to store discovered assets and their relationships. The loader accepts `GraphNode` implementations and creates nodes/relationships using MERGE operations for idempotency.

### Graph Client

**Location:** `internal/graphrag/graph/neo4j.go`

Features:
- Connection management with exponential backoff retry (5 attempts)
- Auto-detects read vs write operations
- Node operations: `CreateNode`, `DeleteNode` with labels
- Relationship operations: `CreateRelationship` with type safety
- Health checking with 5-second connectivity verification

### Cypher Patterns

**Create Node (MERGE for idempotency):**
```cypher
MERGE (n:Host {ip: $ip})
SET n += $props
RETURN n
```

**Create Relationship:**
```cypher
MATCH (from:Host {ip: $from_ip})
MATCH (to:Port {host_id: $host_id, number: $number, protocol: $protocol})
MERGE (from)-[r:HAS_PORT]->(to)
RETURN r
```

### Node Labels

Neo4j labels use PascalCase:

| Type | Neo4j Label |
|------|-------------|
| `host` | `Host` |
| `port` | `Port` |
| `service` | `Service` |
| `endpoint` | `Endpoint` |
| `domain` | `Domain` |
| `subdomain` | `Subdomain` |
| `api` | `API` |
| `technology` | `Technology` |
| `certificate` | `Certificate` |
| `cloud_asset` | `CloudAsset` |
| `finding` | `Finding` |
| `agent_run` | `AgentRun` |
| `tool_execution` | `ToolExecution` |

### Deterministic Node IDs

IDs are generated from identifying properties using SHA-256 hashing:

```go
// Generate deterministic ID from properties
hostID := generator.Generate("host", map[string]any{"ip": "192.168.1.1"})
// Returns: "host:a1b2c3d4e5f6" (deterministic hash)

portID := generator.Generate("port", map[string]any{
    "host_id":  hostID,
    "number":   443,
    "protocol": "tcp",
})
// Returns: "port:x7y8z9w0v1u2"
```

**Key properties:**
- Same inputs always produce same ID
- Property order doesn't affect result
- Strings are normalized to lowercase
- Collision-resistant (SHA-256 based)

---

## GraphLoader

The `GraphLoader` is Gibson's component that accepts `GraphNode` implementations and writes them to Neo4j.

**Location:** `internal/graphrag/loader/loader.go`

### Interface

```go
type GraphLoader struct {
    client   *neo4j.Client
    registry *graphrag.NodeTypeRegistry
}

type ExecContext struct {
    AgentRunID      string
    ToolExecutionID string
    MissionID       string
}

type LoadResult struct {
    NodesCreated         int
    NodesUpdated         int
    RelationshipsCreated int
    Errors               []error
}

// Load writes nodes to the graph with automatic relationship creation
func (l *GraphLoader) Load(ctx context.Context, execCtx *ExecContext, nodes []graphrag.GraphNode) (*LoadResult, error)

// LoadBatch writes nodes in a single transaction for performance
func (l *GraphLoader) LoadBatch(ctx context.Context, execCtx *ExecContext, nodes []graphrag.GraphNode) (*LoadResult, error)
```

### Loading Flow

```
1. Tool returns DiscoveryResult{Hosts, Ports, Services}
2. Harness calls GraphLoader.Load(ctx, execCtx, result.AllNodes())
3. For each GraphNode:
   a. Generate deterministic ID from IdentifyingProperties()
   b. Build MERGE query with Properties()
   c. Execute against Neo4j
   d. If ParentRef() != nil, create relationship
   e. If execCtx provided, create DISCOVERED relationship
4. Return LoadResult with statistics
```

### Example Usage

```go
// In harness callback service
result, err := tool.Execute(ctx, input)
if err != nil {
    return err
}

// Load discoveries into graph
loadResult, err := loader.Load(ctx, &ExecContext{
    AgentRunID:      agentRun.ID,
    ToolExecutionID: toolExec.ID,
    MissionID:       mission.ID,
}, result.AllNodes())

if err != nil {
    return fmt.Errorf("failed to load discoveries: %w", err)
}

log.Printf("Created %d nodes, %d relationships",
    loadResult.NodesCreated, loadResult.RelationshipsCreated)
```

---

## Custom Types and Extensibility

The domain types system supports custom node types via `CustomEntity`. This enables agents to define domain-specific types without modifying the SDK.

### CustomEntity

```go
// CustomEntity provides a base for agent-specific custom types
type CustomEntity struct {
    Namespace  string         // e.g., "k8s", "aws"
    Type       string         // e.g., "pod", "security_group"
    IDProps    map[string]any // Identifying properties
    AllProps   map[string]any // All properties
    Parent     *NodeRef       // Optional parent reference
    ParentRel  string         // Relationship type to parent
}

func (c *CustomEntity) NodeType() string {
    return c.Namespace + ":" + c.Type // e.g., "k8s:pod"
}
```

### Namespace Conventions

| Namespace | Purpose | Examples |
|-----------|---------|----------|
| `k8s:` | Kubernetes resources | `k8s:pod`, `k8s:service`, `k8s:deployment` |
| `aws:` | AWS-specific resources | `aws:security_group`, `aws:iam_role` |
| `azure:` | Azure-specific resources | `azure:resource_group` |
| `gcp:` | GCP-specific resources | `gcp:compute_instance` |
| `vuln:` | Vulnerability types | `vuln:cve`, `vuln:exploit` |
| `custom:` | Agent-specific types | `custom:my_type` |

### Example: Kubernetes Agent

```go
import "github.com/zero-day-ai/sdk/graphrag/domain"

func discoverKubernetesPods() *domain.DiscoveryResult {
    result := domain.NewDiscoveryResult()

    // Create Kubernetes node (custom type)
    node := domain.NewCustomEntity("k8s", "node").
        WithIDProps(map[string]any{
            "name": "worker-node-01",
        }).
        WithAllProps(map[string]any{
            "name":     "worker-node-01",
            "status":   "Ready",
            "version":  "v1.28.0",
        })
    result.Custom = append(result.Custom, node)

    // Create pod with parent relationship
    pod := domain.NewCustomEntity("k8s", "pod").
        WithIDProps(map[string]any{
            "namespace": "default",
            "name":      "web-server-abc123",
        }).
        WithAllProps(map[string]any{
            "namespace": "default",
            "name":      "web-server-abc123",
            "status":    "Running",
            "image":     "nginx:1.21",
        }).
        WithParent(&domain.NodeRef{
            NodeType: "k8s:node",
            Properties: map[string]any{
                "name": "worker-node-01",
            },
        }, "RUNS_ON")
    result.Custom = append(result.Custom, pod)

    return result
}
```

### Mixing Canonical and Custom Types

Custom types and canonical types work together seamlessly:

```go
result := domain.NewDiscoveryResult()

// Canonical: Standard network assets
result.Hosts = append(result.Hosts, &domain.Host{
    IP:       "10.0.1.50",
    Hostname: "k8s-node-01",
})

result.Ports = append(result.Ports, &domain.Port{
    HostID:   "10.0.1.50",
    Number:   6443,
    Protocol: "tcp",
})

// Custom: Kubernetes-specific
result.Custom = append(result.Custom, domain.NewCustomEntity("k8s", "apiserver").
    WithIDProps(map[string]any{"endpoint": "https://10.0.1.50:6443"}).
    WithAllProps(map[string]any{
        "endpoint": "https://10.0.1.50:6443",
        "version":  "v1.28.0",
    }))
```

---

## Migration from Old System

### What Changed

The old taxonomy system used:
- YAML files with JSONPath-based extraction rules
- Template-based ID generation (`id_template: "host:{{.ip}}"`)
- Tool output parsing via `schema.TaxonomyMapping`
- ~19,000 lines of extraction code

The new system uses:
- Strongly-typed Go structs in `sdk/graphrag/domain/`
- Automatic ID generation from `IdentifyingProperties()`
- `DiscoveryResult` container for tool output
- Simple `GraphLoader` (~500 lines)

### Migrating Tools

**Before (Old System):**
```go
// Tool returned raw JSON with taxonomy mapping
func executeTool(input map[string]any) (map[string]any, error) {
    return map[string]any{
        "hosts": []map[string]any{
            {"ip": "192.168.1.1", "hostname": "server"},
        },
    }, nil
}

// Required separate schema.TaxonomyMapping definition
```

**After (New System):**
```go
import "github.com/zero-day-ai/sdk/graphrag/domain"

func executeTool(input map[string]any) (*domain.DiscoveryResult, error) {
    result := domain.NewDiscoveryResult()

    result.Hosts = append(result.Hosts, &domain.Host{
        IP:       "192.168.1.1",
        Hostname: "server",
        State:    "up",
    })

    return result, nil
}

// No separate schema needed - types are self-describing
```

### Deleted Code

The following files were removed in v0.20.0:
- `gibson/internal/graphrag/engine/taxonomy_engine.go`
- `gibson/internal/graphrag/engine/jsonpath.go`
- `gibson/internal/graphrag/engine/template.go`
- `gibson/internal/graphrag/taxonomy/` directory
- `sdk/schema/taxonomy.go`

### Backward Compatibility

Tools using the old `schema.TaxonomyMapping` system are **not compatible** with v0.20.0+. They must be migrated to return `DiscoveryResult`.

---

## Adding New Domain Types

### Adding a Canonical Type to SDK

To add a new canonical domain type (e.g., `Container`):

1. Create file in `sdk/graphrag/domain/container.go`
2. Implement the `GraphNode` interface
3. Add to `DiscoveryResult` struct
4. Add to `AllNodes()` method in proper dependency order

**Example:**

```go
// container.go
package domain

type Container struct {
    Namespace string `json:"namespace"`
    Name      string `json:"name"`
    Image     string `json:"image,omitempty"`
    Status    string `json:"status,omitempty"`
    PodID     string `json:"pod_id"` // Parent reference
}

func (c *Container) NodeType() string { return "container" }

func (c *Container) IdentifyingProperties() map[string]any {
    return map[string]any{
        "namespace": c.Namespace,
        "name":      c.Name,
    }
}

func (c *Container) Properties() map[string]any {
    return map[string]any{
        "namespace": c.Namespace,
        "name":      c.Name,
        "image":     c.Image,
        "status":    c.Status,
        "pod_id":    c.PodID,
    }
}

func (c *Container) ParentRef() *NodeRef {
    return &NodeRef{
        NodeType:   "k8s:pod",
        Properties: map[string]any{"id": c.PodID},
    }
}

func (c *Container) RelationshipType() string { return "CONTAINS" }
```

### Using CustomEntity for Agent-Specific Types

For types specific to a single agent, use `CustomEntity` instead:

```go
// No SDK changes needed - just use CustomEntity
container := domain.NewCustomEntity("k8s", "container").
    WithIDProps(map[string]any{
        "namespace": "default",
        "name":      "my-container",
    }).
    WithAllProps(map[string]any{
        "namespace": "default",
        "name":      "my-container",
        "image":     "nginx:1.21",
    }).
    WithParent(&domain.NodeRef{
        NodeType: "k8s:pod",
        Properties: map[string]any{
            "namespace": "default",
            "name":      "my-pod",
        },
    }, "CONTAINS")

result.Custom = append(result.Custom, container)
```

### When to Use Canonical vs Custom Types

| Scenario | Recommendation |
|----------|----------------|
| Used by multiple agents | Add canonical type to SDK |
| Specific to one agent | Use `CustomEntity` |
| Standard network/web assets | Use existing canonical types |
| Cloud provider specific | Use `CustomEntity` with namespace |
| Kubernetes resources | Use `CustomEntity` with `k8s:` namespace |

---

## Runtime Queries

### Using the NodeTypeRegistry

```go
import "github.com/zero-day-ai/sdk/graphrag"

// Get the global registry
registry := graphrag.Registry()

// Check identifying properties for a node type
props, err := registry.GetIdentifyingProperties("host")
if err != nil {
    log.Fatalf("Unknown node type: %v", err)
}
// props = ["ip"]

// Validate properties before creating a node
properties := map[string]any{
    "ip":       "192.168.1.1",
    "hostname": "web-server",
}
missing, err := registry.ValidateProperties("host", properties)
if err != nil {
    log.Fatalf("Missing properties: %v", missing)
}

// Check if a node type is registered
if registry.IsRegistered("custom_type") {
    // Type exists in registry
}

// List all registered types
allTypes := registry.AllNodeTypes()
```

### Generating Deterministic IDs

```go
import (
    "github.com/zero-day-ai/sdk/graphrag"
    "github.com/zero-day-ai/sdk/graphrag/id"
)

// Create generator with registry
generator := id.NewGenerator(graphrag.Registry())

// Generate ID for a host
hostID, err := generator.Generate("host", map[string]any{
    "ip": "192.168.1.1",
})
// hostID = "host:a1b2c3d4e5f6" (deterministic)

// Generate ID for a port (compound key)
portID, err := generator.Generate("port", map[string]any{
    "host_id":  hostID,
    "number":   443,
    "protocol": "tcp",
})
// portID = "port:x7y8z9w0v1u2"

// Same inputs always produce same ID
portID2, _ := generator.Generate("port", map[string]any{
    "protocol": "tcp",        // Order doesn't matter
    "number":   443,
    "host_id":  hostID,
})
// portID == portID2 (guaranteed)
```

### Using Domain Types Directly

```go
import "github.com/zero-day-ai/sdk/graphrag/domain"

// Domain types implement GraphNode interface
host := &domain.Host{
    IP:       "192.168.1.1",
    Hostname: "web-server",
}

// Get node type
nodeType := host.NodeType() // "host"

// Get identifying properties
idProps := host.IdentifyingProperties()
// {"ip": "192.168.1.1"}

// Get all properties
allProps := host.Properties()
// {"ip": "192.168.1.1", "hostname": "web-server", ...}

// Check for parent (Host has no parent)
parent := host.ParentRef() // nil

// Port has a parent (Host)
port := &domain.Port{
    HostID:   "192.168.1.1",
    Number:   443,
    Protocol: "tcp",
}
parentRef := port.ParentRef()
// {NodeType: "host", Properties: {"ip": "192.168.1.1"}}

relType := port.RelationshipType()
// "HAS_PORT"
```

---

## Tool Output Flow

### Architecture Overview (New System)

Tool output flows directly to Neo4j via the GraphLoader:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        TOOL OUTPUT FLOW (v0.20.0+)                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. Agent calls h.CallTool(ctx, "nmap", input)                             │
│                    │                                                        │
│                    ▼                                                        │
│  2. Tool returns DiscoveryResult with typed domain objects                 │
│     {Hosts: []*Host, Ports: []*Port, Services: []*Service}                │
│                    │                                                        │
│                    ▼                                                        │
│  3. Harness calls GraphLoader.Load(ctx, execCtx, result.AllNodes())        │
│                    │                                                        │
│                    ▼                                                        │
│  4. For each GraphNode:                                                    │
│     - Call IdentifyingProperties() to get ID components                    │
│     - Generate deterministic ID                                            │
│     - MERGE node into Neo4j                                                │
│     - If ParentRef() != nil, create relationship                           │
│                    │                                                        │
│                    ▼                                                        │
│  5. Return LoadResult with statistics                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Files

| File | Purpose |
|------|---------|
| `sdk/graphrag/domain/*.go` | Domain type definitions |
| `sdk/graphrag/domain/discovery.go` | DiscoveryResult container |
| `gibson/internal/graphrag/loader/loader.go` | GraphLoader implementation |
| `gibson/internal/harness/callback_service.go` | Harness integration |

### Creating a Tool with Domain Types

```go
import "github.com/zero-day-ai/sdk/graphrag/domain"

func Execute(ctx context.Context, input map[string]any) (*domain.DiscoveryResult, error) {
    result := domain.NewDiscoveryResult()

    // Discover hosts
    result.Hosts = append(result.Hosts, &domain.Host{
        IP:       "192.168.1.100",
        Hostname: "server.local",
        State:    "up",
    })

    // Discover ports (parent relationship is automatic)
    result.Ports = append(result.Ports, &domain.Port{
        HostID:   "192.168.1.100",
        Number:   22,
        Protocol: "tcp",
        State:    "open",
    })

    // Discover services
    result.Services = append(result.Services, &domain.Service{
        PortID:  "192.168.1.100:22:tcp",
        Name:    "ssh",
        Version: "OpenSSH 8.2",
    })

    return result, nil
}
```

### Node Ordering

`AllNodes()` returns nodes in dependency order (parents before children):

1. Root nodes: Hosts, Domains, Technologies, Certificates, CloudAssets, APIs
2. First-level: Ports, Subdomains
3. Second-level: Services
4. Leaf nodes: Endpoints
5. Custom nodes (order preserved)

This ensures parents exist before relationship creation.

---

## Troubleshooting

### Missing Identifying Properties

**Symptoms:**
- `ErrMissingIdentifyingProperties` error
- Node creation fails

**Cause:** Domain type is missing required identifying properties

**Solution:**
```go
// Check what properties are required
registry := graphrag.Registry()
props, _ := registry.GetIdentifyingProperties("port")
// props = ["host_id", "number", "protocol"]

// Ensure all are set
port := &domain.Port{
    HostID:   "192.168.1.1",  // Required
    Number:   443,             // Required
    Protocol: "tcp",           // Required
    State:    "open",          // Optional
}
```

### Parent Node Not Found

**Symptoms:**
- Relationship creation fails
- Warning: "parent node not found"

**Cause:** Parent node hasn't been created yet

**Solution:** Ensure nodes are loaded in dependency order. Use `DiscoveryResult.AllNodes()` which returns nodes in the correct order (parents before children).

### Neo4j Connection Errors

**Symptoms:**
- "failed to create node/relationship"
- Connection timeouts

**Solutions:**
- Verify Neo4j is running: `neo4j status`
- Check connection settings in Gibson config
- Test connectivity: `cypher-shell -u neo4j -p <password>`
- Check Neo4j logs for authentication issues

### Custom Type Namespace Conflicts

**Symptoms:**
- Warning about namespace conflicts

**Cause:** Using a canonical type name as custom namespace

**Solution:** Use proper namespace prefixes:
```go
// Wrong - "host" is a canonical type
domain.NewCustomEntity("host", "vm")  // Don't do this

// Correct - use namespaced prefix
domain.NewCustomEntity("vmware", "vm")  // ✓
domain.NewCustomEntity("k8s", "pod")    // ✓
```

### Debug Logging

Enable debug logging to see detailed information:

```bash
GIBSON_LOG_LEVEL=debug gibson daemon start
```

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

### GraphNode Interface Methods

| Method | Return Type | Description |
|--------|-------------|-------------|
| `NodeType()` | `string` | Canonical type (e.g., "host", "k8s:pod") |
| `IdentifyingProperties()` | `map[string]any` | Properties that uniquely identify the node |
| `Properties()` | `map[string]any` | All properties to set on the node |
| `ParentRef()` | `*NodeRef` | Parent node reference (nil for root nodes) |
| `RelationshipType()` | `string` | Relationship type to parent (e.g., "HAS_PORT") |

### Error Types

| Error | Description |
|-------|-------------|
| `ErrMissingIdentifyingProperties` | Required identifying properties not provided |
| `ErrNodeTypeNotRegistered` | Unknown node type |
| `ErrParentNotFound` | Parent node doesn't exist in graph |

### Cypher Patterns

| Operation | Pattern |
|-----------|---------|
| Node MERGE | `MERGE (n:Label {idProp: $val}) SET n += $props` |
| Relationship | `MATCH (a), (b) MERGE (a)-[r:TYPE]->(b)` |

### See Also

- `sdk/graphrag/domain/README.md` - Domain types documentation
- `sdk/graphrag/README.md` - ID generation and registry
- `gibson/internal/graphrag/loader/` - GraphLoader implementation
