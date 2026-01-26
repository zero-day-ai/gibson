# Gibson Framework

Enterprise-grade, LLM-based autonomous agent framework for security testing.

## Overview

Gibson orchestrates AI agents that can autonomously hack LLMs, web applications, APIs, Kubernetes clusters, and any connected system. It provides:

- **Multi-Agent Orchestration** - Coordinate specialized AI security agents
- **DAG Workflow Engine** - Complex, multi-step attack scenarios via YAML
- **Three-Tier Memory** - Working, Mission, and Long-Term vector memory
- **GraphRAG Integration** - Neo4j-powered cross-mission knowledge graphs
- **Finding Management** - MITRE ATT&CK/ATLAS mappings with SARIF export

## Installation

```bash
go install github.com/zero-day-ai/gibson/cmd/gibson@latest
```

## Quick Start

```bash
# Initialize Gibson
gibson init

# Start the daemon
gibson daemon start

# Install an agent
gibson agent install github.com/zero-day-ai/network-recon

# Run an attack
gibson attack https://api.target.com/chat \
  --agent prompt-injector \
  --goal "Extract the system prompt"

# View findings
gibson finding list --severity high,critical
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           GIBSON FRAMEWORK                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      MISSION ORCHESTRATOR                            │   │
│  │   Manages mission lifecycle, agent coordination, finding collection  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│                    ┌───────────────┼───────────────┐                        │
│                    ▼               ▼               ▼                        │
│  ┌─────────────────────┐ ┌─────────────────────┐ ┌─────────────────────┐   │
│  │       AGENTS        │ │       TOOLS         │ │      PLUGINS        │   │
│  │  (LLM-powered)      │ │  (Proto-based I/O)  │ │  (Stateful services)│   │
│  └─────────────────────┘ └─────────────────────┘ └─────────────────────┘   │
│           │                      │                      │                   │
│           └──────────────────────┼──────────────────────┘                   │
│                                  ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         AGENT HARNESS                                │   │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐   │   │
│  │  │   LLM    │ │  Tools   │ │ Plugins  │ │  Memory  │ │ Findings │   │   │
│  │  │  Slots   │ │ Registry │ │ Registry │ │  3-Tier  │ │  Store   │   │   │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Directory Structure

```
gibson/
├── cmd/gibson/          # CLI entry point
│   └── main.go
├── internal/
│   ├── harness/         # AgentHarness implementation
│   ├── agent/           # Agent interfaces & types
│   ├── tool/            # Tool registry & interface
│   ├── plugin/          # Plugin registry & interface
│   ├── llm/             # LLM providers & slot management
│   │   └── providers/   # Anthropic, OpenAI, Ollama
│   ├── memory/          # Three-tier memory system
│   ├── database/        # Persistence layer
│   ├── graphrag/        # Neo4j knowledge graph
│   ├── mission/         # Mission orchestration
│   ├── orchestrator/    # Execution coordination
│   ├── finding/         # Finding management
│   ├── payload/         # Attack payload library
│   │   └── builtin/     # 35+ built-in payloads
│   ├── guardrail/       # Security boundaries
│   └── observability/   # OpenTelemetry integration
└── configs/             # Example configurations
```

## Core Concepts

### Agent Harness

The single interface through which agents access all framework capabilities:

```go
// LLM Access
harness.Complete(ctx, "primary", messages)
harness.StreamComplete(ctx, "primary", messages)
harness.CompleteStructured(ctx, "primary", schema, messages)

// Tool Execution
harness.ExecuteTool(ctx, "nmap", protoInput)

// Plugin Queries
harness.QueryPlugin(ctx, "shodan", "search", params)

// Agent Delegation
harness.DelegateToAgent(ctx, "recon-agent", task)

// Memory Access
harness.Memory().Working().Set(ctx, key, value)
harness.Memory().Mission().Search(ctx, query, opts)
harness.Memory().LongTerm().Store(ctx, data, metadata)

// Finding Submission
harness.SubmitFinding(ctx, finding)
```

### LLM Slot System

Agents declare abstract LLM requirements ("slots") resolved at runtime:

```go
func (a *MyAgent) LLMSlots() []SlotDefinition {
    return []SlotDefinition{
        agent.NewSlotDefinition("primary", "Main reasoning LLM", true).
            WithConstraints(agent.SlotConstraints{
                MinContextWindow: 8000,
                RequiredFeatures: []string{"tool_use", "json_mode"},
            }),
    }
}
```

### Three-Tier Memory

| Tier | Storage | Lifetime | Use Case |
|------|---------|----------|----------|
| **Working** | In-memory | Task execution | Temporary state |
| **Mission** | SQLite + FTS5 | Mission lifetime | Scan results |
| **Long-Term** | Vector store | Cross-mission | Semantic knowledge |

### GraphRAG Taxonomy Relationships

Gibson uses a taxonomy-driven approach to automatically create entity relationships in Neo4j. This enables powerful cross-mission queries and knowledge graph visualization.

#### Overview

Entity relationships are defined in the SDK's `core.yaml` taxonomy file. When agents submit discovery results, Gibson's loader automatically creates the appropriate Neo4j relationships based on entity types and their hierarchical structure.

**Key Components:**

- **Taxonomy Definition**: `github.com/zero-day-ai/sdk/taxonomy/core.yaml`
- **Generated Code**: `github.com/zero-day-ai/sdk/taxonomy/taxonomy.go` (ParentRelationships map)
- **Loader**: `gibson/internal/graphrag/loader.go` (relationship creation logic)

#### Discovery Processing Flow

```
Agent → DiscoveryResult → act.go → discovery_adapter → loader → Neo4j
```

1. **Agent**: Generates discovery results with UUIDs and parent references
2. **DiscoveryResult**: SDK struct containing entities with type information
3. **act.go**: Orchestrator receives results, extracts missionRunID from context
4. **discovery_adapter**: Converts SDK types to internal Gibson types, preserves missionRunID
5. **loader**: Creates nodes and relationships in Neo4j using taxonomy rules

The `missionRunID` flows through the entire pipeline via context, enabling proper scoping of discovery results to specific mission runs.

#### Entity Identity

All entities in Gibson's GraphRAG system use **UUID-based identity**:

- **Primary Key**: Every entity has an `id` field containing a UUID string
- **Generation**: Agents generate UUIDs using SDK helpers (`taxonomy.NewHost()`, etc.)
- **Parent References**: Child entities reference parents via UUID fields (`host_id`, `port_id`, etc.)
- **No Natural Keys**: Do not use IP addresses, domain names, or other natural keys as primary identifiers

**Example:**

```go
// Agent code
host := taxonomy.NewHost()  // Generates UUID automatically
host.IP = "192.168.1.1"

port := taxonomy.NewPort()  // Generates UUID automatically
port.Number = 443
port.HostID = host.ID       // Reference parent by UUID
```

#### Relationship Types

The following parent-child relationships are automatically created:

| Child Type | Parent Type | Relationship | Child Field | Description |
|------------|-------------|--------------|-------------|-------------|
| port | host | HAS_PORT | host_id | Host has open ports |
| service | port | RUNS_SERVICE | port_id | Port runs services |
| endpoint | service | EXPOSES | service_id | Service exposes endpoints |
| subdomain | domain | HAS_SUBDOMAIN | domain_id | Domain has subdomains |
| evidence | finding | SUPPORTS | finding_id | Evidence supports findings |
| certificate | service | USES_CERTIFICATE | service_id | Service uses TLS cert |
| vulnerability | service | AFFECTS_SERVICE | service_id | Vuln affects service |
| technology | service | USES_TECHNOLOGY | service_id | Service uses tech stack |

**Generated Cypher Example:**

```cypher
// Host → Port relationship
MATCH (parent:host {id: $parent_id})
MATCH (child:port {id: $child_id})
MERGE (parent)-[:HAS_PORT]->(child)

// Port → Service relationship
MATCH (parent:port {id: $parent_id})
MATCH (child:service {id: $child_id})
MERGE (parent)-[:RUNS_SERVICE]->(child)
```

#### MissionRun Scoping

All root-level entities are attached to a `MissionRun` node via `BELONGS_TO` relationship. This enables querying discoveries by mission.

**Root Entity Types:**

- `host`
- `domain`
- `finding`
- `organization`
- `person`

**Flow:**

1. `missionRunID` extracted from context in `act.go`
2. Passed to `discovery_adapter` via `MissionRunID` field
3. Loader checks if entity is root type (via SDK's `IsRootEntity()`)
4. Creates `BELONGS_TO` relationship to `MissionRun` node

**Generated Cypher:**

```cypher
// Create/merge MissionRun node
MERGE (mr:MissionRun {id: $mission_run_id})

// Link root entity to MissionRun
MATCH (mr:MissionRun {id: $mission_run_id})
MATCH (entity:host {id: $entity_id})
MERGE (entity)-[:BELONGS_TO]->(mr)
```

**Query Example:**

```cypher
// Get all hosts from a specific mission run
MATCH (mr:MissionRun {id: "abc-123"})
MATCH (h:host)-[:BELONGS_TO]->(mr)
RETURN h
```

#### Debugging Relationship Issues

**Check if relationships exist:**

```cypher
// Verify parent-child relationship
MATCH (parent:host {id: "parent-uuid"})
MATCH (child:port {id: "child-uuid"})
MATCH (parent)-[r:HAS_PORT]->(child)
RETURN parent, r, child

// Verify MissionRun scoping
MATCH (mr:MissionRun {id: "mission-run-uuid"})
MATCH (entity)-[r:BELONGS_TO]->(mr)
RETURN entity, r, mr
```

**Common Issues:**

| Issue | Symptom | Solution |
|-------|---------|----------|
| Missing parent | Orphaned child nodes | Verify child's reference field (e.g., `host_id`) contains valid parent UUID |
| No BELONGS_TO | Can't query by mission | Check `missionRunID` is in context when agent submits discovery |
| Wrong relationship type | Incorrect graph structure | Verify entity types match taxonomy definitions in `core.yaml` |
| Duplicate relationships | Multiple edges between same nodes | Loader uses MERGE (idempotent), check for bugs in relationship detection |

**Debugging Commands:**

```bash
# Enable debug logging
export GIBSON_LOG_LEVEL=debug

# Check loader logs
gibson daemon logs | grep "creating relationship"

# Verify Neo4j connection
gibson graphrag status

# Query Neo4j directly
docker exec -it gibson-neo4j cypher-shell -u neo4j -p <password>
```

**Troubleshooting Checklist:**

1. ✅ Entity has valid UUID in `id` field
2. ✅ Child entity has parent reference field (`host_id`, `port_id`, etc.)
3. ✅ Parent reference contains valid UUID (not empty, not zero-value)
4. ✅ `missionRunID` present in context when `SubmitDiscovery()` called
5. ✅ Entity type matches taxonomy definition (case-sensitive)
6. ✅ Neo4j connection healthy (`gibson graphrag status`)
7. ✅ Loader logs show "creating relationship" entries

### Tools vs Plugins

| Aspect | Tools | Plugins |
|--------|-------|---------|
| **Purpose** | Atomic operations | Stateful services |
| **I/O** | Protocol Buffers | JSON |
| **State** | Stateless | Maintains state |
| **Examples** | nmap, httpx | Shodan API, scope parser |

## CLI Commands

### Daemon Management

```bash
gibson daemon start          # Start the daemon
gibson daemon stop           # Stop the daemon
gibson daemon status         # Check daemon status
```

### Agent Management

```bash
gibson agent list            # List installed agents
gibson agent install <url>   # Install an agent
gibson agent uninstall <name> # Remove an agent
gibson agent run <name> --task "..." # Run agent directly
```

### Tool Management

```bash
gibson tool list             # List installed tools
gibson tool install <url>    # Install a tool
gibson tool uninstall <name> # Remove a tool
gibson tool health <name>    # Check tool health
```

### Plugin Management

```bash
gibson plugin list           # List installed plugins
gibson plugin install <url>  # Install a plugin
gibson plugin uninstall <name> # Remove a plugin
gibson plugin query <name> <method> <params> # Query plugin
```

### Mission Execution

```bash
gibson mission run -f mission.yaml  # Run a mission
gibson mission status <id>          # Check mission status
gibson mission stop <id>            # Stop a mission
gibson mission list                 # List missions
```

### Findings

```bash
gibson finding list                      # List all findings
gibson finding list --severity critical  # Filter by severity
gibson finding show <id>                 # Show finding details
gibson finding export --format sarif     # Export to SARIF
```

### Quick Attacks

```bash
gibson attack <target> \
  --agent <agent-name> \
  --goal "Your attack goal"
```

## Configuration

### gibson.yaml

```yaml
daemon:
  port: 50051
  log_level: info

llm:
  default_provider: anthropic
  providers:
    anthropic:
      api_key: ${ANTHROPIC_API_KEY}
      models:
        - claude-3-opus-20240229
        - claude-3-sonnet-20240229
    openai:
      api_key: ${OPENAI_API_KEY}
      models:
        - gpt-4-turbo
        - gpt-4

memory:
  working:
    max_tokens: 100000
  mission:
    backend: sqlite
    path: ~/.gibson/missions/
  long_term:
    backend: sqlite
    path: ~/.gibson/vectors/
    embedder:
      provider: native  # MiniLM

graphrag:
  enabled: true
  neo4j:
    uri: bolt://localhost:7687
    username: neo4j
    password: ${NEO4J_PASSWORD}

observability:
  tracing:
    enabled: true
    exporter: otlp
    endpoint: localhost:4317
  langfuse:
    enabled: false
    public_key: ${LANGFUSE_PUBLIC_KEY}
    secret_key: ${LANGFUSE_SECRET_KEY}
```

## Mission Workflows

Define complex attack scenarios in YAML:

```yaml
name: llm-security-assessment
version: 1.0.0
description: Comprehensive LLM security testing

target:
  type: llm
  url: https://api.target.com/v1/chat

phases:
  - name: reconnaissance
    agents:
      - name: fingerprinter
        goal: Identify the target LLM model and version

  - name: prompt-injection
    depends_on: [reconnaissance]
    agents:
      - name: prompt-injector
        goal: Test for prompt injection vulnerabilities
        config:
          payloads:
            - direct-injection
            - indirect-injection

  - name: jailbreak
    depends_on: [reconnaissance]
    agents:
      - name: jailbreaker
        goal: Attempt to bypass guardrails
        config:
          techniques:
            - roleplay
            - dan-variants

  - name: data-extraction
    depends_on: [jailbreak]
    condition: "findings.jailbreak.any(severity >= 'high')"
    agents:
      - name: data-extractor
        goal: Extract sensitive information
```

## Attack Payloads

Gibson includes 35+ built-in attack payloads:

### AI/LLM Security

- **Prompt Injection**: Direct, indirect, context manipulation
- **Jailbreak**: Roleplay, DAN variants, constraint bypass
- **RAG Poisoning**: Citation injection, retrieval hijack
- **Data Extraction**: System prompt, PII, training data

### Encoding/Obfuscation

- Base64, ROT13, Leetspeak

### Usage

```bash
# List available payloads
gibson payload list

# Use specific payload
gibson attack <target> --payload jailbreak/dan-v2
```

## Development

### Building

```bash
make build
```

### Testing

```bash
make test
make test-race
make test-coverage
```

### Linting

```bash
make lint
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `GIBSON_CONFIG` | Path to config file |
| `GIBSON_LOG_LEVEL` | Log level (debug, info, warn, error) |
| `GIBSON_DEBUG` | Enable debug mode |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `NEO4J_PASSWORD` | Neo4j password |

## Related Repositories

| Repository | Description |
|------------|-------------|
| [sdk](https://github.com/zero-day-ai/sdk) | Development SDK |
| [tools](https://github.com/zero-day-ai/tools) | Security tool wrappers |
| [network-recon](https://github.com/zero-day-ai/network-recon) | Network recon agent |
| [tech-stack-fingerprinting](https://github.com/zero-day-ai/tech-stack-fingerprinting) | Tech detection agent |

## License

Proprietary - Zero-Day.AI
