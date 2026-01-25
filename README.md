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
