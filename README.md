# Gibson

**Autonomous Security Testing Framework**

Gibson is an extensible framework for building and orchestrating autonomous security testing agents. It provides the infrastructure to rapidly develop agents that can test anything - APIs, networks, web applications, cloud infrastructure, smart contracts, LLMs, IoT devices, or any system you can write an agent for.

## What Gibson Does

Gibson is **not** a security scanner. It's a **framework** that lets you:

- **Build agents** that autonomously perform security testing tasks
- **Orchestrate workflows** that coordinate multiple agents in complex testing scenarios
- **Store knowledge** in a graph database for reasoning about attack paths and relationships
- **Execute tools** in a distributed, queue-based architecture
- **Track findings** with full provenance back to the agent and mission that discovered them

Think of it as Kubernetes for security testing - you bring the agents, Gibson handles orchestration, scheduling, observability, and state management.

## Core Concepts

### Agents
Autonomous units that perform security testing. An agent can be anything:
- A network scanner that maps infrastructure
- A web crawler that discovers endpoints
- A fuzzer that tests input validation
- A credential tester that checks authentication
- An LLM red-teamer that probes AI systems
- A smart contract auditor that analyzes bytecode

Agents are built with the [Gibson SDK](../sdk) and can use any tools, call any APIs, and implement any testing logic.

### Tools
Reusable capabilities that agents invoke. Tools execute via a Redis queue for distributed processing:
- Port scanners, DNS resolvers, HTTP clients
- Exploit frameworks, payload generators
- Cloud API wrappers, container inspectors
- Custom integrations with your existing tooling

### Missions
YAML-defined workflows that orchestrate agents in a DAG (directed acyclic graph):
- Sequential, parallel, and conditional execution
- Checkpointing and resumption
- Constraints (time, cost, finding limits)
- Auto-installation of dependencies

### GraphRAG
Neo4j-powered knowledge graph that stores:
- Discovered assets and their relationships
- Attack patterns and technique mappings
- Findings with full context
- Semantic search over accumulated knowledge

### Harness
The runtime environment provided to agents:
- LLM access (Anthropic, OpenAI, Ollama)
- Tool invocation
- Sub-agent delegation
- Finding storage
- Memory tiers (working, mission, long-term)

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Gibson CLI                             │
│  mission | attack | agent | tool | target | finding | ...  │
└────────────┬────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────┐
│                  Daemon (gRPC Server)                       │
│     Orchestration · Registry · Callbacks · Persistence      │
└────┬──────────────┬──────────────┬──────────────┬───────────┘
     │              │              │              │
     ▼              ▼              ▼              ▼
┌─────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐
│  etcd   │  │  Redis   │  │ Database │  │  GraphRAG    │
│Registry │  │  Queue   │  │ (SQLite) │  │  (Neo4j)     │
└─────────┘  └──────────┘  └──────────┘  └──────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────────────┐
│                    Your Agents & Tools                       │
│   network-scanner | web-fuzzer | cloud-auditor | ...        │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.24+
- Redis 6.0+
- Neo4j 5.0+ (optional, for GraphRAG)

### Installation

```bash
git clone https://github.com/zero-day-ai/gibson.git
cd gibson
make build
./bin/gibson init
```

### Run Your First Mission

```bash
# Start the daemon
gibson daemon start

# Add a target
gibson target add my-app --type http_api

# Run a mission
gibson mission run recon.yaml --target my-app

# View findings
gibson finding list
```

## CLI Reference

### Daemon

```bash
gibson daemon start    # Start background services
gibson daemon stop     # Stop daemon
gibson daemon status   # Check status
```

### Targets

```bash
gibson target add <name> --type <type>   # Add target (http_api, kubernetes, network, etc.)
gibson target list                        # List targets
gibson target test <name>                 # Test connectivity
gibson target delete <name>               # Remove target
```

### Missions

```bash
gibson mission run <file|url|name>       # Execute a mission
gibson mission list                       # List missions
gibson mission show <id>                  # Show progress
gibson mission pause <id>                 # Pause execution
gibson mission resume <id>                # Resume from checkpoint
gibson mission cancel <id>                # Cancel mission
gibson mission validate <file>            # Validate YAML
```

### Quick Attack

```bash
gibson attack --target <name> --agent <agent>   # Single-agent attack
gibson attack --list-agents                      # List available agents
```

### Agents & Tools

```bash
gibson agent list                    # List installed agents
gibson agent install <url>           # Install from URL/git
gibson agent start <name>            # Start agent service
gibson agent stop <name>             # Stop agent service

gibson tool list                     # List installed tools
gibson tool install <url>            # Install tool
```

### Knowledge Store

```bash
gibson knowledge ingest --from-dir ./data    # Ingest documents
gibson knowledge search "query"               # Semantic search
```

### Findings

```bash
gibson finding list                  # List findings
gibson finding show <id>             # Show details
gibson finding export --format json  # Export findings
```

### Credentials

```bash
gibson credential add <name>         # Store encrypted credential
gibson credential list               # List credentials
```

## Configuration

Configuration lives at `~/.gibson/config.yaml`:

```yaml
core:
  home_dir: ~/.gibson
  parallel_limit: 10
  timeout: 5m

database:
  path: ~/.gibson/gibson.db

daemon:
  grpc_address: localhost:50002

redis:
  url: redis://localhost:6379

registry:
  type: embedded
  listen_address: localhost:2379

graphrag:
  enabled: true
  neo4j:
    uri: bolt://localhost:7687
    username: neo4j
    password: password

# LLM providers (for agents that need them)
# Set via environment: ANTHROPIC_API_KEY, OPENAI_API_KEY, OLLAMA_HOST

# Observability
langfuse:
  enabled: false
  host: "https://cloud.langfuse.com"
  public_key: ""
  secret_key: ""

tracing:
  enabled: false
  endpoint: localhost:4317

callback:
  enabled: true
  listen_address: 0.0.0.0:50001
```

## Mission YAML

Missions define workflows as directed acyclic graphs:

```yaml
name: "Infrastructure Assessment"
description: "Map and test network infrastructure"
version: "1.0.0"

nodes:
  # Discovery phase
  network-scan:
    type: agent
    agent: network-mapper
    parameters:
      ports: "1-65535"
      timeout: 10m

  # Branch based on findings
  check-services:
    type: condition
    expression: "findings.open_ports > 0"
    on_true: service-enum
    on_false: report

  # Enumerate services
  service-enum:
    type: agent
    agent: service-fingerprinter

  # Parallel vulnerability testing
  vuln-testing:
    type: parallel
    nodes: [web-scanner, ssh-auditor, db-tester]

  web-scanner:
    type: agent
    agent: web-vulnerability-scanner

  ssh-auditor:
    type: agent
    agent: ssh-config-auditor

  db-tester:
    type: agent
    agent: database-security-tester

  # Aggregate results
  aggregate:
    type: join
    sources: [vuln-testing]

  report:
    type: agent
    agent: report-generator

edges:
  - from: network-scan
    to: check-services
  - from: service-enum
    to: vuln-testing
  - from: aggregate
    to: report

entry_points: [network-scan]
exit_points: [report]

constraints:
  max_duration: 2h
  max_findings: 5000

dependencies:
  agents:
    - github.com/your-org/agents/network-mapper
    - github.com/your-org/agents/web-vulnerability-scanner
```

### Node Types

| Type | Purpose |
|------|---------|
| `agent` | Execute an agent |
| `tool` | Execute a tool directly |
| `condition` | Branch based on expression |
| `parallel` | Run multiple nodes concurrently |
| `join` | Wait for parallel nodes to complete |
| `plugin` | Invoke plugin capability |

## Building Agents with the SDK

The [Gibson SDK](../sdk) provides everything you need to build agents:

```go
package main

import (
    "context"
    "github.com/zero-day-ai/sdk"
)

type NetworkMapper struct {
    sdk.BaseAgent
}

func (a *NetworkMapper) Execute(ctx context.Context, input string) (string, error) {
    harness := a.Harness()

    // Use tools
    result, err := harness.InvokeTool(ctx, "nmap", map[string]any{
        "target": input,
        "ports":  "1-1024",
    })
    if err != nil {
        return "", err
    }

    // Store findings
    for _, port := range result.OpenPorts {
        harness.StoreFinding(ctx, sdk.Finding{
            Title:    fmt.Sprintf("Open port %d", port.Number),
            Severity: "info",
            Data:     port,
        })
    }

    // Optionally use LLM for analysis
    analysis, err := harness.Complete(ctx, sdk.CompletionRequest{
        Prompt: fmt.Sprintf("Analyze these scan results: %v", result),
    })

    return analysis, nil
}

func main() {
    sdk.Run(&NetworkMapper{})
}
```

### Building Tools

```go
type PortScanner struct {
    sdk.BaseTool
}

func (t *PortScanner) Execute(ctx context.Context, params map[string]any) (any, error) {
    target := params["target"].(string)
    ports := params["ports"].(string)

    // Your scanning logic
    results := scan(target, ports)

    return results, nil
}
```

## Project Structure

```
gibson/
├── cmd/gibson/           # CLI commands
├── configs/              # Example configuration
├── internal/
│   ├── agent/            # Agent interfaces
│   ├── config/           # Configuration loading
│   ├── daemon/           # Daemon and gRPC server
│   ├── finding/          # Finding management
│   ├── graphrag/         # Neo4j integration
│   ├── harness/          # Agent runtime environment
│   ├── llm/              # LLM provider abstraction
│   ├── memory/           # Memory tiers
│   ├── mission/          # Mission execution
│   ├── orchestrator/     # Workflow orchestration
│   ├── registry/         # Service discovery (etcd)
│   └── tool/             # Tool execution (Redis)
└── tests/
```

## Building

```bash
make build          # Build binary
make test           # Run tests
make test-coverage  # Coverage report
make lint           # Lint code
make proto          # Generate protobuf
```

## Use Cases

Gibson is designed for building:

- **Network Security Testing** - Autonomous infrastructure scanning and vulnerability discovery
- **Web Application Testing** - Crawling, fuzzing, injection testing
- **Cloud Security Auditing** - AWS/Azure/GCP configuration review
- **API Security Testing** - Authentication, authorization, input validation
- **Container Security** - Kubernetes, Docker security assessment
- **Smart Contract Auditing** - Blockchain and DeFi security
- **LLM Red-Teaming** - Prompt injection, jailbreak testing
- **IoT Security** - Device and protocol testing
- **Compliance Scanning** - Automated compliance checking
- **Custom Security Workflows** - Whatever you can build an agent for

## Why Gibson?

| Problem | Gibson's Solution |
|---------|-------------------|
| Security tools don't integrate | Unified orchestration layer |
| Manual testing doesn't scale | Autonomous agent execution |
| Findings lack context | GraphRAG knowledge relationships |
| Complex workflows are fragile | DAG-based missions with checkpointing |
| Building security tools is slow | SDK with batteries included |
| No visibility into testing | Langfuse/OpenTelemetry observability |

## License

[License information]

## Contributing

[Contribution guidelines]
