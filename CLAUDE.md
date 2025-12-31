# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Gibson is an LLM-powered security testing framework that orchestrates AI agents for vulnerability assessments against LLMs and AI systems. It combines agent orchestration, DAG-based workflows, knowledge graph integration (Neo4j), and configurable attack payloads.

## Build Commands

```bash
# Build the binary (requires CGO for SQLite FTS5)
make bin                    # Quick local build → bin/gibson

# Run tests
make test                   # All tests with verbose output
make test-race              # Tests with race detection
make test-coverage          # Tests with 90% coverage threshold

# Code quality
make lint                   # golangci-lint (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
make fmt                    # Format code
make check                  # All checks: fmt, vet, lint, test-race

# Proto generation (if modifying gRPC interfaces)
make proto                  # Generate from api/proto/*.proto → api/gen/proto/

# Install to GOPATH/bin
make install
```

**Single test example:**
```bash
go test -tags fts5 -v -run TestSpecificName ./internal/component/
```

## Architecture Overview

### CLI Entry Points (`cmd/gibson/`)
- `root.go` - Base cobra command setup
- `agent.go`, `tool.go`, `plugin.go` - Component management (install, start, stop, logs)
- `attack.go` - Single-agent quick attacks
- `mission.go` - Multi-agent workflow orchestration
- `finding.go` - Vulnerability finding management and export

### Core Packages (`internal/`)

**Component System** (`internal/component/`)
- Unified management for agents, tools, and plugins
- `component.yaml` manifests define metadata, build commands, runtime type
- Supports: Go, Python, Node, Docker, binary, HTTP, gRPC runtimes
- Components installed to `~/.gibson/{agents,tools,plugins}/{name}/`

**Agent Harness** (`internal/harness/`)
- Runtime environment for agent execution
- Provides: LLM completion (slot-based), tool execution, memory access, finding submission
- GraphRAG bridge for async Neo4j storage
- Parent-child delegation for complex workflows

**LLM Integration** (`internal/llm/`)
- Multi-provider support (Anthropic, OpenAI, Ollama, etc.)
- Slot-based model selection with token tracking
- Budget constraints and cost management

**Workflow Engine** (`internal/workflow/`)
- DAG-based execution from YAML definitions
- Node types: Agent, Tool, Plugin, Condition, Parallel, Join
- Retry policies: constant, linear, exponential backoff

**Memory System** (`internal/memory/`)
- Three-tier architecture:
  - Working memory (ephemeral, in-memory)
  - Mission memory (SQLite-backed, FTS5 searchable)
  - Long-term memory (vector embeddings for semantic search)

**Prompt System** (`internal/prompt/`)
- Assembler collects, filters, renders prompts for LLM APIs
- Relay system for context injection and scope narrowing
- Position-based ordering with priority sorting

**Finding Management** (`internal/finding/`)
- Classification, deduplication, evidence collection
- MITRE ATT&CK/ATLAS mapping
- Export: JSON, SARIF, CSV, HTML

**GraphRAG** (`internal/graphrag/`)
- Neo4j integration for cross-mission insights
- Relationships: DISCOVERED_ON, USES_TECHNIQUE, SIMILAR_TO, BELONGS_TO_MISSION
- Vector embeddings for semantic similarity search

### External Components (gRPC)

Agents, tools, and plugins communicate via gRPC using the SDK (`github.com/zero-day-ai/sdk`). The SDK is a local replace directive pointing to `../sdk`.

## Key Patterns

- **CGO Required**: SQLite FTS5 needs `CGO_ENABLED=1` (always set in Makefile)
- **Build Tags**: Always use `-tags fts5` when running tests manually
- **Component Manifests**: All external components use `component.yaml` with build/run/lifecycle config
- **Mono-repo Support**: Install from subdirectories with `URL#path/to/component`

## Configuration

Default config: `~/.gibson/config.yaml`
Database: SQLite with FTS5, WAL mode at `~/.gibson/gibson.db`
