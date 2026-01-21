# Gibson Platform Guide

Gibson is the runtime platform for AI security testing agents. It provides the daemon, orchestrator, and all infrastructure that agents run on.

## Architecture Overview

```
gibson/
├── cmd/gibson/          # CLI commands (mission, agent, attack, etc.)
├── internal/
│   ├── orchestrator/    # Mission orchestration and agent coordination
│   ├── harness/         # Agent runtime harness implementation
│   ├── graphrag/        # Knowledge graph engine (Neo4j + embeddings)
│   ├── daemon/          # Background service for agent execution
│   ├── component/       # External component management
│   ├── registry/        # etcd-based service discovery
│   ├── database/        # SQLite persistence layer
│   ├── memory/          # Memory store implementations
│   ├── llm/             # LLM provider implementations
│   ├── prompt/          # Prompt templates and relay system
│   ├── workflow/        # DAG workflow engine
│   ├── finding/         # Finding storage and management
│   ├── config/          # Configuration management
│   └── observability/   # Tracing, metrics, logging
└── configs/             # Default configuration files
```

## Key Packages

### internal/orchestrator

Manages mission lifecycle and agent coordination:
- Creates and tracks missions
- Assigns agents to targets
- Collects and aggregates findings
- Handles agent delegation (parent-child)

### internal/harness

Implements `agent.Harness` interface from SDK:
- Provides LLM access via slot-based routing
- Executes tool calls
- Manages agent memory
- Submits findings
- GraphRAG integration
  - `StoreSemantic()` - Store with embeddings for similarity search
  - `StoreStructured()` - Store without embeddings for property-based queries
  - `QuerySemantic()`, `QueryStructured()`, `QueryGraphRAG()` - Query routing

### internal/graphrag

Knowledge graph engine for semantic search:
- Neo4j backend for graph storage
- Native embedder (all-minilm-l6-v2-go)
- Taxonomy-driven node/relationship types
- Hybrid scoring (vector + graph)

**Mission-Scoped Storage (v0.16.0+):**

All graph data is scoped to MissionRun for proper isolation:

```
Mission (name: "pentest-web", target_id: "target-123")
    └── MissionRun (run_number: 1, started_at: ...)
            ├── Host (ip: "192.168.1.1")
            │       └── Port (number: 443)
            │               └── Service (name: "https")
            ├── Finding (title: "SQL Injection")
            └── Technique (id: "T1190")
```

- **Mission**: Top-level entity identified by (name + target_id), reused across runs
- **MissionRun**: Unique execution instance, nodes BELONGS_TO this
- **Root Nodes**: Automatically attached to MissionRun (host, domain, finding, etc.)
- **Child Nodes**: Must declare parent via `BelongsTo()` pattern (port→host, service→port)

**Storage Modes:**
- **Semantic Storage**: Generates embeddings for similarity search (findings, insights)
- **Structured Storage**: Direct graph storage without embeddings (hosts, ports, IPs)
- **Auto-routing**: Intelligent fallback between semantic and structured queries

**Query Scopes:**
- `ScopeMissionRun` (default): Only current run's data
- `ScopeMission`: All runs of same mission (historical queries)
- `ScopeGlobal`: All missions (cross-mission analysis)

**Query Routing:**
- `QuerySemantic()`: Vector similarity search only
- `QueryStructured()`: Direct Cypher queries by type/properties
- `QueryGraphRAG()`: Auto-routing with fallback (semantic → structured)

Key files:
- `engine/` - Core GraphRAG engine
- `provider/` - Embedding providers
- `queries/` - Graph query builders
- `processor.go` - Query routing logic
- `store.go` - Storage implementations
- `loader/loader.go` - Mission-scoped storage with CREATE (not MERGE)
- `mission_graph_manager.go` - Mission/MissionRun lifecycle

### internal/daemon

Background service mode:
- gRPC server for agent communication
- Tool execution service
- Mission queue management

### internal/component

External component management:
- Agent registration and discovery
- Binary component lifecycle
- Health checking

### internal/memory

Three-tier memory implementation:
- Working memory (in-process)
- Mission memory (SQLite with FTS5)
- Long-term memory (vector store)

### internal/registry

Service discovery for agents, tools, and plugins:
- Embedded etcd mode for development
- External etcd cluster for production
- TTL-based health tracking
- Automatic service deregistration

### internal/llm

LLM provider implementations:
- Anthropic Claude
- OpenAI GPT
- Ollama (local)
- Provider routing via slots

### internal/prompt

Prompt management system:
- Template loading and rendering
- Variable injection
- Relay system for multi-agent prompts
- System prompt assembly

## CLI Commands

```bash
# Mission management
gibson mission create -f workflow.yaml -t target-id
gibson mission run <mission-id>
gibson mission status <mission-id>
gibson mission list

# Agent management
gibson agent list
gibson agent run <agent-name> --target <target-id>
gibson agent describe <agent-name>

# Target management
gibson target add --url https://api.example.com --type llm_api
gibson target list
gibson target test <target-id>

# Attack workflows
gibson attack run -w prompt-injection.yaml -t target-id
gibson attack list

# Finding management
gibson finding list --mission <mission-id>
gibson finding export --format sarif -o findings.sarif

# Configuration
gibson config show
gibson config set llm.default_provider anthropic

# Daemon mode
gibson daemon start
gibson daemon status
```

## Development Patterns

### Adding a New Command

1. Create file in `cmd/gibson/`:

```go
package main

import (
    "github.com/spf13/cobra"
)

var myCmd = &cobra.Command{
    Use:   "mycommand",
    Short: "Does something useful",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Implementation
        return nil
    },
}

func init() {
    rootCmd.AddCommand(myCmd)
}
```

### Extending the Orchestrator

Key interfaces in `internal/orchestrator/`:

```go
// MissionRunner executes missions
type MissionRunner interface {
    Run(ctx context.Context, mission *Mission) error
    Stop(ctx context.Context, missionID string) error
}

// AgentCoordinator manages agent assignment
type AgentCoordinator interface {
    Assign(mission *Mission, agents []string) error
    Delegate(parentAgent, childAgent string, task Task) error
}
```

### Adding GraphRAG Node Types

Node types are defined in `sdk/graphrag/taxonomy.yaml` and generated:

1. Edit taxonomy YAML in SDK
2. Run `go generate ./graphrag/...` in SDK
3. Update Gibson to use new constants

## Configuration

Gibson uses Viper for configuration. Default locations:
- `~/.gibson/config.yaml`
- `./configs/gibson.yaml`

Key configuration sections:

```yaml
llm:
  default_provider: anthropic
  providers:
    anthropic:
      api_key: ${ANTHROPIC_API_KEY}
    openai:
      api_key: ${OPENAI_API_KEY}

graphrag:
  provider: neo4j  # Required: neo4j, neptune, or memgraph
  neo4j:
    uri: bolt://localhost:7687
    username: neo4j
    password: password
  embedder:
    provider: native  # or openai

memory:
  mission:
    backend: sqlite
    path: ~/.gibson/memory.db

observability:
  tracing:
    enabled: true
    exporter: otlp
    endpoint: localhost:4317
```

## Building and Testing

**IMPORTANT: AI agents must use make commands, not raw go commands.**

Gibson requires special build flags (`CGO_ENABLED=1` with SQLite FTS5 support). Always use make:

```bash
# Build Gibson binary
make build          # Full build with version injection
make bin            # Quick local build

# Testing
make test           # Run all tests
make test-race      # Tests with race detection
make test-coverage  # Tests with coverage report

# Code quality
make lint           # Run golangci-lint
make fmt            # Format code
make vet            # Run go vet

# All checks before commit
make check          # Runs fmt, vet, lint, test-race

# Install to GOPATH/bin
make install

# Show all targets
make help
```

### DON'T: Use Raw Go Commands

```bash
# NEVER do these - they bypass required build configuration
go build ./...                    # Missing CGO flags and FTS5 support
go test ./...                     # May miss race detection
go run ./cmd/gibson/main.go       # Wrong CGO settings
```

### Test Containers

Integration tests use testcontainers:

```go
func TestWithNeo4j(t *testing.T) {
    ctx := context.Background()
    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image: "neo4j:5",
            // ...
        },
        Started: true,
    })
    // ...
}
```

## Dependencies

Gibson depends on SDK v0.20.0. Update with:

```bash
go get github.com/zero-day-ai/sdk@v0.X.Y
go mod tidy
```

## Spec Workflow

**IMPORTANT**: The spec-workflow directory ALWAYS lives at `~/Code/zero-day.ai/.spec-workflow`

All specifications, requirements, design documents, and task breakdowns are managed through the spec-workflow MCP tools and stored in this central location, regardless of which subdirectory you're working in.

## See Also

- `../sdk/CLAUDE.md` - SDK documentation (agent development)
- Root `CLAUDE.md` - Monorepo navigation
- `README.md` - Public-facing documentation
