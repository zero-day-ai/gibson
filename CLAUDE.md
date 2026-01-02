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

### Key Patterns

**Harness Pattern**: The `internal/harness` package provides the agent runtime environment. Agents receive a harness that exposes:
- `Complete()` - LLM completions via slot-based model selection
- `ExecuteTool()` - Registry-based tool execution
- `SubmitFinding()` - Finding storage (local + async GraphRAG)
- Memory access (working, mission, long-term)

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
