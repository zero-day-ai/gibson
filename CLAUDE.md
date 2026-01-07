# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
# Build the binary (requires CGO for SQLite FTS5)
make bin                    # Quick local build → bin/gibson

# Run tests
make test                   # All tests
make test-race              # With race detection
go test -v ./internal/llm/  # Single package

# Lint and format
make lint                   # Requires golangci-lint
make fmt                    # Format code
make check                  # Run all checks (fmt, vet, lint, test-race)

# Generate proto files (internal daemon API)
make proto                  # Generates from internal/daemon/api/*.proto
```

**CGO Requirement**: SQLite FTS5 requires CGO. The Makefile sets `CGO_ENABLED=1` and `CGO_CFLAGS=-DSQLITE_ENABLE_FTS5`.

## Architecture Overview

Gibson is an LLM-powered security testing framework that orchestrates AI agents to test AI systems for vulnerabilities.

### Component Model

Gibson uses a **daemon-client architecture**:

1. **Daemon** (`internal/daemon/`) - Long-running process that:
   - Manages embedded etcd registry for service discovery
   - Orchestrates mission execution via gRPC streaming
   - Coordinates agent/tool/plugin lifecycles
   - Exposes gRPC API defined in `internal/daemon/api/daemon.proto`

2. **CLI** (`cmd/gibson/`) - Client that communicates with daemon via gRPC
   - Commands: `attack`, `mission`, `agent`, `tool`, `plugin`, `target`, `finding`
   - Shared command logic in `cmd/gibson/core/` for CLI and TUI reuse

3. **External Components** (gRPC services):
   - **Agents** - AI security testing workers that receive tasks and submit findings
   - **Tools** - Utility functions agents can call (HTTP client, scanners, etc.)
   - **Plugins** - Query-based services for additional capabilities

### Key Internal Packages

| Package | Purpose |
|---------|---------|
| `internal/harness/` | Runtime environment for agents; provides LLM access, tool execution, finding submission, memory |
| `internal/mission/` | Mission state machine and orchestration |
| `internal/workflow/` | DAG-based workflow engine with YAML parsing |
| `internal/registry/` | etcd-based component discovery and health monitoring |
| `internal/llm/` | Multi-provider LLM interface with slot-based model selection |
| `internal/memory/` | Three-tier memory: working (ephemeral), mission (SQLite), long-term (vector) |
| `internal/finding/` | Vulnerability classification, deduplication, MITRE mapping |
| `internal/graphrag/` | Neo4j knowledge graph for cross-mission insights |
| `internal/prompt/` | Prompt assembly with relay system for context injection |
| `internal/database/` | SQLite with FTS5, WAL mode, migrations |

### Data Flow

```
CLI Command → Daemon (gRPC) → Mission Orchestrator → Workflow DAG
                                      ↓
                            Agent Harness (runtime)
                                      ↓
                    Agent ← → LLM Provider (via slots)
                      ↓
               Submit Finding → Finding Store → GraphRAG
```

### Agent Execution

Agents communicate via the `harness.AgentHarness` interface:
- `Complete()` / `CompleteWithTools()` - LLM operations via slot names
- `CallTool()` - Execute registered tools
- `SubmitFinding()` - Report discovered vulnerabilities
- `Memory()` - Access working/mission/long-term memory tiers
- `DelegateToAgent()` - Spawn sub-agents

### Configuration

- Config file: `~/.gibson/config.yaml`
- Database: `~/.gibson/gibson.db` (SQLite with FTS5)
- Installed components: `~/.gibson/{agents,tools,plugins}/<name>/`
- Registry data: `~/.gibson/etcd-data/` (embedded etcd)

### Target System

Targets are schema-validated via `internal/types/target.go`. Types include:
- `http_api` - REST/HTTP endpoints
- `kubernetes` - K8s clusters
- `smart_contract` - Blockchain contracts
- `llm_chat` - Chat-based LLM interfaces

### TUI

Built with Bubbletea (`internal/tui/`). Agent Focus mode (key `5`) enables real-time agent observation and steering during mission execution.
