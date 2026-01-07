# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Gibson is an LLM-powered automated security testing framework that orchestrates AI agents for vulnerability assessments against LLMs and AI systems. It combines agent orchestration, DAG-based workflows, knowledge graph integration, and configurable attack payloads.

## Build Commands

```bash
# Build binary (CGO required for SQLite FTS5)
make bin                    # Quick local build -> bin/gibson
make build                  # Full build (alias for bin)
make install                # Install to GOPATH/bin

# Testing
make test                   # Run all tests
make test-race              # Tests with race detection
make test-coverage          # Tests with coverage (enforces 90% threshold)

# Run single test
go test -tags fts5 -v -run TestFunctionName ./internal/package/

# Linting and formatting
make lint                   # Run golangci-lint
make fmt                    # Format code
make vet                    # Run go vet
make check                  # Run all checks (fmt, vet, lint, test-race)

# Proto generation (gRPC)
make proto                  # Generate Go code from proto files
make proto-deps             # Install protoc plugins
```

## Build Requirements

- Go 1.24.4+
- CGO_ENABLED=1 (required for SQLite FTS5)
- CGO_CFLAGS=-DSQLITE_ENABLE_FTS5 (set by Makefile)
- Build tags: `-tags "fts5"` (required for all go commands)

## Architecture

### Core Entry Point
- `cmd/gibson/` - CLI application using Cobra commands

### Key Internal Packages

| Package | Purpose |
|---------|---------|
| `internal/agent` | Agent registry, slot management, gRPC client for external agents |
| `internal/attack` | Attack execution engine with payload filtering and target validation |
| `internal/component` | Component lifecycle management and etcd registry integration |
| `internal/harness` | Agent runtime environment - orchestrates LLM, tools, memory, findings |
| `internal/llm` | Multi-provider LLM registry (Anthropic, OpenAI, Ollama, Google) with slot-based selection |
| `internal/memory` | Three-tier memory: working (ephemeral), mission (SQLite), long-term (vector) |
| `internal/mission` | Mission management, controller, and event system |
| `internal/finding` | Finding classification, deduplication, storage with MITRE mappings |
| `internal/graphrag` | Neo4j knowledge graph integration with vector embeddings |
| `internal/payload` | Payload loading, execution, and analytics |
| `internal/prompt` | Prompt assembly and relay system for context injection |
| `internal/registry` | etcd-based component discovery and health monitoring |
| `internal/tui` | Terminal UI (bubbletea-based) with Agent Focus mode |
| `internal/workflow` | DAG workflow engine with YAML parsing |

### Component Communication
- External agents, tools, and plugins communicate via gRPC
- Internal components use direct Go interfaces
- etcd registry for service discovery and health monitoring
- Components register at startup and use TTL-based keepalive

## Daemon-Client Architecture

Gibson uses a daemon-client architecture where the daemon owns runtime state and the CLI commands communicate via gRPC.

### Data Sources

**Daemon (runtime state)**: Component registration (agents, tools, plugins), health status, active missions
**SQLite (persistent data)**: Findings, mission history, credentials, targets, payloads

Commands query the appropriate data source:
- `gibson agent list` / `tool list` / `plugin list` → Daemon registry (live state)
- `gibson status` → Daemon gRPC (uptime, component counts, version)
- `gibson findings list` → SQLite (historical data)
- `gibson attack` → Daemon for execution (orchestrates registry components)

### Key Files

| File | Purpose |
|------|---------|
| `internal/daemon/daemon.go` | Daemon lifecycle management, registry adapter initialization |
| `internal/daemon/grpc.go` | Daemon's DaemonInterface implementation (ListAgents, Status, etc.) |
| `internal/daemon/client/client.go` | gRPC client for CLI commands |
| `internal/daemon/client/convert.go` | Proto-to-domain type conversion helpers |
| `internal/daemon/api/daemon.proto` | gRPC service definition |

### Command Routing

Commands check for daemon availability using `GetDaemonClient()`:
- **Daemon available**: Call `client.ListAgents()`, `client.Status()`, `client.RunAttack()`, etc.
- **Daemon not running**: Return clear error message: "daemon not running, start with 'gibson daemon start'"

This enables commands to work both in daemon mode (production) and standalone mode (development/testing).

### Key Patterns

**Harness Pattern**: The `internal/harness` package provides the agent runtime environment. Agents receive a harness that exposes:
- `Complete()` - LLM completions via slot-based model selection
- `ExecuteTool()` - Registry-based tool execution
- `SubmitFinding()` - Finding storage (local + async GraphRAG)
- Memory access (working, mission, long-term)

**HarnessFactory Wiring**: The daemon creates a `HarnessFactory` during startup that is wired throughout the execution flow:

```
daemon.Start()
  └─> newInfrastructure()
        ├─> Creates LLMRegistry (providers from config/env)
        ├─> Creates SlotManager (wraps LLMRegistry)
        ├─> Creates MemoryManagerFactory (for per-mission memory)
        └─> Creates HarnessFactory (with all dependencies)
              └─> Stores in infrastructure.harnessFactory

MissionOrchestrator (created in daemon.go and mission_manager.go)
  └─> WithHarnessFactory(infrastructure.harnessFactory)

orchestrator.Execute(mission)
  └─> harnessFactory.Create(agentName, missionCtx, target)
        └─> Returns AgentHarness with:
              - SlotManager for LLM slot resolution
              - LLMRegistry for provider access
              - MemoryFactory creates per-mission memory
              - FindingStore for vulnerability storage
```

Key files:
- `internal/daemon/infrastructure.go` - Holds slotManager, harnessFactory, memoryManagerFactory
- `internal/daemon/harness_init.go` - Creates HarnessFactory with dependencies
- `internal/daemon/slot_manager.go` - DaemonSlotManager implementation
- `internal/daemon/memory_factory.go` - Creates mission-scoped MemoryManagers
- `internal/harness/factory.go` - HarnessFactory interface and implementation
- `internal/harness/config.go` - HarnessConfig with MemoryFactory callback

**Slot-Based LLM Selection**: Agents request LLM slots by capability requirements (context window, etc.) rather than specific providers.

**Finding Flow**:
1. Agent submits via harness
2. Stored in local FindingStore (sync)
3. Queued for GraphRAG (async)
4. Creates Neo4j nodes and relationships (DISCOVERED_ON, USES_TECHNIQUE, SIMILAR_TO)

### Configuration
- Default location: `~/.gibson/config.yaml`
- SQLite database: `~/.gibson/gibson.db`
- Embedded etcd data: `~/.gibson/etcd-data/`
- Environment variable substitution: `${VAR_NAME}`

## Testing Conventions

- Integration tests often have `_integration_test.go` suffix or separate README
- Mock implementations provided for external dependencies
- Use `-tags fts5` for all test runs to enable SQLite FTS5 support
