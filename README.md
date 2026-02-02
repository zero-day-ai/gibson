# Gibson

**Autonomous LLM Red-Teaming Framework**

Gibson is an autonomous AI security testing platform for red-teaming LLM systems, RAG pipelines, and AI agents. It combines multi-agent orchestration, knowledge graphs, and workflow-based mission execution to conduct comprehensive security assessments.

## Features

- **Autonomous Agent Coordination** - LLM-driven decision making with multi-agent orchestration
- **Mission-Based Workflows** - DAG-based workflow execution with checkpointing and resumption
- **Knowledge Graph Integration** - Neo4j-powered GraphRAG for semantic search and attack pattern discovery
- **Real-Time Observability** - Langfuse and OpenTelemetry integration for tracing and monitoring
- **Queue-Based Tool Execution** - Redis-powered distributed tool execution
- **Extensible Architecture** - Agents, tools, and plugins can be developed with the SDK

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Gibson CLI (Cobra)                       │
│  mission | attack | agent | tool | target | credential |   │
│  knowledge | payload | daemon | orchestrator | finding      │
└────────────┬────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────┐
│                  Daemon (gRPC Server)                       │
│  Long-running services for registry, callbacks, missions   │
└────┬──────────────┬──────────────┬──────────────┬───────────┘
     │              │              │              │
     ▼              ▼              ▼              ▼
┌─────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐
│  etcd   │  │ Callback │  │ Database │  │  GraphRAG    │
│Registry │  │  Server  │  │ (SQLite) │  │  (Neo4j)     │
└─────────┘  └──────────┘  └──────────┘  └──────────────┘
```

## Quick Start

### Prerequisites

- Go 1.24+
- Redis (for tool execution)
- Neo4j (for GraphRAG - optional but recommended)

### Installation

```bash
# Clone the repository
git clone https://github.com/zero-day-ai/gibson.git
cd gibson

# Build
make build

# Initialize Gibson
./bin/gibson init
```

### Basic Usage

```bash
# Start the daemon
gibson daemon start

# Add a target
gibson target add my-api --type http_api

# Run a quick attack
gibson attack --target my-api --agent prompt-injector

# Run a mission
gibson mission run reconnaissance.yaml --target my-api

# Check findings
gibson finding list --mission <mission-id>
```

## CLI Commands

### Initialization & Configuration

| Command | Description |
|---------|-------------|
| `gibson init` | Initialize configuration, database, and encryption key |
| `gibson config show` | Display current configuration |
| `gibson config get <key>` | Get a specific configuration value |
| `gibson config set <key> <value>` | Set a configuration value |
| `gibson version` | Display version information |

### Daemon Management

| Command | Description |
|---------|-------------|
| `gibson daemon start` | Start the Gibson daemon |
| `gibson daemon stop` | Stop the running daemon |
| `gibson daemon status` | Check daemon status |
| `gibson daemon restart` | Restart the daemon |

### Target Management

| Command | Description |
|---------|-------------|
| `gibson target add <name>` | Add a new attack target |
| `gibson target list` | List all configured targets |
| `gibson target show <name>` | Show target details |
| `gibson target test <name>` | Test target connectivity |
| `gibson target delete <name>` | Remove a target |

### Mission Execution

| Command | Description |
|---------|-------------|
| `gibson mission run <name>` | Execute a mission (from file, URL, or installed) |
| `gibson mission list` | List all missions |
| `gibson mission show <id>` | Show mission details and progress |
| `gibson mission pause <id>` | Pause a running mission |
| `gibson mission resume <id>` | Resume a paused mission |
| `gibson mission cancel <id>` | Cancel a mission |
| `gibson mission plan <name>` | Display execution plan without running |
| `gibson mission validate <file>` | Validate mission YAML syntax |

### Quick Attack

```bash
# Launch a single-agent attack
gibson attack --target my-target --agent prompt-injector

# With specific payload
gibson attack --target my-target --agent sql-injector --payload "'; DROP TABLE users;--"

# List available attack agents
gibson attack --list-agents
```

### Agent & Tool Management

| Command | Description |
|---------|-------------|
| `gibson agent list` | List installed agents |
| `gibson agent install <url>` | Install an agent |
| `gibson agent show <name>` | Show agent details |
| `gibson agent start <name>` | Start an agent service |
| `gibson agent stop <name>` | Stop an agent service |
| `gibson tool list` | List installed tools |
| `gibson tool install <url>` | Install a tool |
| `gibson tool show <name>` | Show tool details |

### Knowledge Store

```bash
# Ingest documents
gibson knowledge ingest --from-pdf ./docs/security-research.pdf
gibson knowledge ingest --from-url https://owasp.org/
gibson knowledge ingest --from-dir ./attack-patterns/

# Search knowledge
gibson knowledge search "SQL injection bypass techniques" --limit 10
```

### Finding Management

| Command | Description |
|---------|-------------|
| `gibson finding list` | List findings with filtering |
| `gibson finding show <id>` | Show finding details |
| `gibson finding classify <id>` | Apply classification/severity |
| `gibson finding export` | Export findings (CSV, JSON, PDF) |

## Configuration

Gibson configuration is stored in `~/.gibson/config.yaml`. Copy the example configuration to get started:

```bash
cp configs/gibson.yaml ~/.gibson/config.yaml
```

### Key Configuration Sections

```yaml
# Core settings
core:
  home_dir: ~/.gibson
  parallel_limit: 10
  timeout: 5m

# Database (SQLite with FTS5)
database:
  path: ~/.gibson/gibson.db
  wal_mode: true

# LLM providers (via environment variables)
llm:
  default_provider: ""  # anthropic, openai, ollama
  # ANTHROPIC_API_KEY, OPENAI_API_KEY, OLLAMA_HOST

# Service registry (etcd)
registry:
  type: embedded
  listen_address: localhost:2379

# Daemon gRPC server
daemon:
  grpc_address: localhost:50002

# Redis (for tool execution)
redis:
  url: redis://localhost:6379

# GraphRAG (Neo4j knowledge graph)
graphrag:
  enabled: true
  neo4j:
    uri: bolt://localhost:7687
    username: neo4j
    password: password

# Langfuse observability
langfuse:
  enabled: true
  host: "https://cloud.langfuse.com"
  public_key: "pk-lf-..."
  secret_key: "sk-lf-..."

# Callback server (agent communication)
callback:
  enabled: true
  listen_address: 0.0.0.0:50001
```

### Environment Variables

```bash
# LLM Providers
export ANTHROPIC_API_KEY="sk-ant-..."
export OPENAI_API_KEY="sk-..."
export OLLAMA_HOST="http://localhost:11434"

# Langfuse (recommended over config file)
export LANGFUSE_PUBLIC_KEY="pk-lf-..."
export LANGFUSE_SECRET_KEY="sk-lf-..."
export LANGFUSE_HOST="https://cloud.langfuse.com"
```

## Mission YAML Specification

Missions are defined in YAML files with a DAG-based workflow structure:

```yaml
name: "API Security Assessment"
description: "Comprehensive API security testing"
version: "1.0.0"

# Target resolved at runtime
target: my-api

# Workflow nodes
nodes:
  # Agent node - executes an agent
  recon:
    type: agent
    agent: api-recon
    parameters:
      depth: 3
      timeout: 5m

  # Conditional branching
  check-auth:
    type: condition
    expression: "findings.auth_issues > 0"
    on_true: auth-testing
    on_false: injection-testing

  # Tool execution
  auth-testing:
    type: tool
    tool: jwt-analyzer
    parameters:
      mode: comprehensive

  injection-testing:
    type: agent
    agent: injection-tester

  # Parallel execution
  parallel-scans:
    type: parallel
    nodes: [xss-scanner, sqli-scanner]

  # Join/sync point
  aggregate:
    type: join
    sources: [parallel-scans]

  # Generate report
  report:
    type: agent
    agent: report-writer

# Flow definition
edges:
  - from: recon
    to: check-auth
  - from: auth-testing
    to: parallel-scans
  - from: injection-testing
    to: parallel-scans
  - from: aggregate
    to: report

# Entry and exit points
entry_points: [recon]
exit_points: [report]

# Execution constraints
constraints:
  max_duration: 30m
  max_findings: 1000
  max_cost: 50.0

# Auto-install dependencies
dependencies:
  agents:
    - github.com/zero-day-ai/agents/api-recon
    - github.com/zero-day-ai/agents/injection-tester
  tools:
    - github.com/zero-day-ai/tools/jwt-analyzer
```

### Node Types

| Type | Description |
|------|-------------|
| `agent` | Execute an agent with parameters |
| `tool` | Execute a tool directly |
| `plugin` | Invoke a plugin capability |
| `condition` | Conditional branching based on expression |
| `parallel` | Execute multiple paths concurrently |
| `join` | Synchronization point for parallel paths |

## SDK Integration

Gibson uses the [Gibson SDK](../sdk) for building agents, tools, and plugins. The SDK is maintained in a separate repository and linked via Go module replacement.

### SDK Architecture

```
┌─────────────────────────────────────────┐
│              Gibson SDK                  │
├─────────────────────────────────────────┤
│  Client Layer     - High-level APIs     │
│  Protocol Layer   - gRPC communication  │
│  Plugin Layer     - Extension APIs      │
│  Observability    - OpenTelemetry       │
└─────────────────────────────────────────┘
```

### Creating an Agent

```go
import "github.com/zero-day-ai/sdk"

type MyAgent struct {
    sdk.BaseAgent
}

func (a *MyAgent) Execute(ctx context.Context, input string) (string, error) {
    // Use harness for LLM access, tool execution, etc.
    result, err := a.Harness().InvokeTool(ctx, "my-tool", params)
    if err != nil {
        return "", err
    }
    return result, nil
}
```

### Creating a Tool

```go
import "github.com/zero-day-ai/sdk"

type MyTool struct {
    sdk.BaseTool
}

func (t *MyTool) Execute(ctx context.Context, params map[string]any) (any, error) {
    // Tool implementation
    return result, nil
}
```

### Creating a Plugin

```go
import "github.com/zero-day-ai/sdk"

type MyPlugin struct {
    sdk.BasePlugin
}

func (p *MyPlugin) Initialize(ctx context.Context) error {
    // Plugin initialization
    return nil
}
```

## Observability

### Langfuse Integration

Gibson integrates with Langfuse for comprehensive LLM observability:

```
Trace: mission-{mission_id}
├── Generation: orchestrator-decision-1 (LLM reasoning)
│   ├── input: prompt with graph state
│   ├── output: Decision JSON
│   └── metadata: {tokens, latency, graph_snapshot}
├── Span: agent-execution-{id}
│   ├── Span: tool-call-nmap
│   ├── Span: tool-call-httpx
│   └── Generation: agent-llm-call
└── Span: mission-complete
    └── metadata: {summary, total_tokens, duration}
```

### OpenTelemetry

Distributed tracing is supported via OpenTelemetry:

```yaml
tracing:
  enabled: true
  endpoint: localhost:4317  # OTLP collector

metrics:
  enabled: true
  port: 9090  # Prometheus endpoint
```

## Development

### Building

```bash
# Build binary
make build

# Run tests
make test

# Run tests with coverage
make test-coverage

# Lint
make lint

# Generate protobuf
make proto
```

### Project Structure

```
gibson/
├── cmd/gibson/          # CLI entry points and subcommands
├── configs/             # Example configuration files
├── internal/
│   ├── agent/           # Agent interfaces and runtime
│   ├── config/          # Configuration loading
│   ├── daemon/          # Daemon and gRPC services
│   ├── finding/         # Finding management
│   ├── graphrag/        # Neo4j GraphRAG integration
│   ├── harness/         # Agent harness implementation
│   ├── llm/             # LLM provider abstractions
│   ├── memory/          # Memory tiers (working, mission, long-term)
│   ├── mission/         # Mission definitions and execution
│   ├── orchestrator/    # Mission orchestration
│   ├── registry/        # etcd service registry
│   └── tool/            # Tool execution
└── tests/e2e/           # End-to-end tests
```

### Key Interfaces

- **AgentHarness** - Runtime environment for agents (LLM access, tool execution, delegation)
- **MissionStore** - Persistence for mission definitions
- **MissionRunStore** - Persistence for mission execution instances
- **ComponentRegistry** - Service discovery and registration
- **GraphRAGStore** - Knowledge graph operations

## Requirements

| Component | Version | Purpose |
|-----------|---------|---------|
| Go | 1.24+ | Build |
| Redis | 6.0+ | Tool execution queue |
| Neo4j | 5.0+ | GraphRAG knowledge graph (optional) |
| etcd | 3.5+ | Service registry (embedded by default) |

## License

[License information]

## Contributing

[Contribution guidelines]

## Support

- Documentation: https://docs.gibson.ai
- GitHub Issues: https://github.com/zero-day-ai/gibson/issues
- Community: https://community.gibson.ai
