# Gibson Framework: SDK, Tools, Agents, and Knowledge Graph Guide

This comprehensive guide covers the Gibson Framework's SDK architecture, how to build agents, tools, and plugins, and how the Knowledge Graph (GraphRAG) system works to enable cross-mission intelligence.

## Table of Contents

1. [Introduction](#introduction)
2. [Architecture Overview](#architecture-overview)
3. [Core Concepts](#core-concepts)
   - [Agents](#agents)
   - [Tools](#tools)
   - [Plugins](#plugins)
4. [The Agent Harness](#the-agent-harness)
5. [Knowledge Graph (GraphRAG)](#knowledge-graph-graphrag)
   - [How It Works](#how-it-works)
   - [Data Models](#data-models)
   - [Providers](#providers)
   - [Configuration](#configuration)
6. [Memory System](#memory-system)
7. [LLM Integration](#llm-integration)
8. [Workflow Engine](#workflow-engine)
9. [Building Custom Components](#building-custom-components)
10. [Prompt Relay System](#prompt-relay-system)
11. [Observability](#observability)
12. [Requirements and Dependencies](#requirements-and-dependencies)

---

## Introduction

Gibson is an enterprise-grade, LLM-powered automated security testing framework for comprehensive vulnerability assessments against LLM and AI systems. It orchestrates multiple specialized AI agents that work together to discover vulnerabilities, classify findings, and generate actionable security reports.

### Key Capabilities

- **Prompt Injection** - Testing LLM behavior hijacking through malicious inputs
- **Jailbreaks** - Bypassing content filters and safety guardrails
- **Data Extraction** - Extracting training data, system prompts, or PII
- **Model Manipulation** - Influencing model outputs through adversarial techniques
- **Information Disclosure** - Uncovering sensitive system information

---

## Architecture Overview

Gibson follows a layered, plugin-based architecture:

```
┌─────────────────────────────────────────────────────────────────┐
│                      Gibson CLI Entry Point                      │
└──────────────────────────┬──────────────────────────────────────┘
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
         ▼                 ▼                 ▼
    ┌─────────┐      ┌─────────┐      ┌──────────┐
    │  Attack │      │ Mission │      │  Agent   │
    │ Command │      │ Command │      │ Command  │
    └────┬────┘      └────┬────┘      └─────┬────┘
         │                │                  │
         └────────────────┼──────────────────┘
                          │
         ┌────────────────┴────────────────┐
         │                                 │
         ▼                                 ▼
   ┌───────────────┐            ┌──────────────────┐
   │  Harness      │            │  Workflow        │
   │  (Runtime)    │            │  Engine (DAG)    │
   └───────┬───────┘            └────────┬─────────┘
           │                             │
    ┌──────┴──────────┬──────────────┬───┴──────────┐
    │                 │              │              │
    ▼                 ▼              ▼              ▼
┌─────────┐    ┌──────────┐   ┌──────────┐   ┌──────────┐
│   LLM   │    │  Memory  │   │ Findings │   │ GraphRAG │
│Provider │    │ Manager  │   │  Store   │   │ (Neo4j)  │
│Registry │    │          │   │          │   │          │
└─────────┘    └──────────┘   └──────────┘   └──────────┘
```

### Core Packages

| Package | Purpose |
|---------|---------|
| `internal/agent` | Agent registry, slot management, and delegation |
| `internal/tool` | Tool registry and gRPC communication |
| `internal/plugin` | Plugin registry and gRPC communication |
| `internal/harness` | Agent runtime environment orchestrator |
| `internal/graphrag` | Graph Retrieval Augmented Generation |
| `internal/memory` | Three-tier memory system with embeddings |
| `internal/llm` | LLM provider registry and token tracking |
| `internal/workflow` | DAG workflow engine with YAML parsing |
| `internal/finding` | Finding classification, deduplication, storage |
| `internal/prompt` | Prompt assembly and relay system |

---

## Core Concepts

### Agents

Agents are autonomous AI security testing components that communicate with Gibson via gRPC. Each agent:

- Declares **capabilities** (prompt injection, jailbreak, data extraction, etc.)
- Specifies **target types** it can test (LLM chat, API, RAG systems)
- Defines **LLM slot requirements** for model selection
- Implements **task execution logic**
- **Submits findings** when vulnerabilities are discovered

#### Agent Interface

```go
type Agent interface {
    // Lifecycle
    Initialize(ctx context.Context, config AgentConfig) error
    Execute(ctx context.Context, harness AgentHarness, task Task) (Result, error)
    Shutdown(ctx context.Context) error

    // Metadata
    Name() string
    Version() string
    Description() string
    Capabilities() []AgentCapability
    TargetTypes() []TargetType

    // LLM Requirements
    SlotDefinitions() []SlotDefinition

    // Health
    Health(ctx context.Context) types.HealthStatus
}
```

#### Built-in Capabilities

| Capability | Description |
|------------|-------------|
| `CapabilityPromptInjection` | Test for prompt injection vulnerabilities |
| `CapabilityJailbreak` | Test for jailbreak attempts |
| `CapabilityDataExtraction` | Test for data extraction vulnerabilities |
| `CapabilityModelManipulation` | Test for model manipulation attacks |
| `CapabilityDoS` | Test for denial-of-service vulnerabilities |

#### Task and Result Types

```go
type Task struct {
    ID          types.ID
    Name        string
    Description string
    Input       map[string]any
    Timeout     time.Duration
    MissionID   *types.ID
    ParentTaskID *types.ID
    TargetID    *types.ID
    Priority    int
    Tags        []string
}

type Result struct {
    TaskID   types.ID
    Status   ResultStatus  // Success, Failed, Partial, Timeout
    Output   map[string]any
    Findings []Finding
    Error    error
}
```

#### Agent Delegation

Parent agents can delegate tasks to child agents:

```go
// Delegate to a sub-agent
childResult, err := harness.DelegateToAgent(ctx, "vulnerability-scanner", agent.Task{
    Name:        "Deep Scan",
    Description: "Perform deep vulnerability analysis",
    Input: map[string]any{
        "target": "auth_module",
        "depth":  "comprehensive",
    },
})
```

Child agents share mission context and memory with their parent but have independent token tracking.

---

### Tools

Tools are atomic, stateless operations with JSON Schema validation. They execute specific functions and return results.

#### Tool Interface

```go
type Tool interface {
    Name() string
    Description() string
    Version() string
    Tags() []string
    InputSchema() schema.JSONSchema
    OutputSchema() schema.JSONSchema
    Execute(ctx context.Context, input map[string]any) (map[string]any, error)
    Health(ctx context.Context) types.HealthStatus
}
```

#### Tool Characteristics

- **Stateless**: No persistent state between calls
- **Schema-validated**: Input/output validated against JSON Schema
- **gRPC-based**: Remote execution support
- **Health-monitored**: Built-in health checking

#### Using Tools from an Agent

```go
// Execute a tool
result, err := harness.ExecuteTool(ctx, "http-scanner", map[string]any{
    "url":    "https://api.example.com/v1/chat",
    "method": "POST",
    "headers": map[string]string{
        "Content-Type": "application/json",
    },
})
```

---

### Plugins

Plugins are stateful services for external data access with dynamic method discovery.

#### Plugin Interface

```go
type Plugin interface {
    Name() string
    Version() string
    Initialize(ctx context.Context, cfg PluginConfig) error
    Shutdown(ctx context.Context) error
    Query(ctx context.Context, method string, params map[string]any) (any, error)
    Methods() []MethodDescriptor
    Health(ctx context.Context) types.HealthStatus
}
```

#### Plugin Characteristics

- **Stateful**: Maintains connections and state
- **Dynamic methods**: Methods discovered via `ListMethods()`
- **Query-based**: Invoke methods with parameters
- **Lifecycle-managed**: Initialize/shutdown hooks

#### Using Plugins from an Agent

```go
// Query a plugin
result, err := harness.QueryPlugin(ctx, "database", "search_findings", map[string]any{
    "query":    "SQL injection",
    "limit":    10,
    "severity": "critical",
})
```

---

## The Agent Harness

The Agent Harness is the central runtime environment for agent execution, providing access to all Gibson capabilities.

### AgentHarness Interface

```go
type AgentHarness interface {
    // LLM Operations
    Complete(ctx context.Context, slotName string, messages []llm.Message) (*llm.CompletionResponse, error)

    // Tool/Plugin Operations
    ExecuteTool(ctx context.Context, name string, input map[string]any) (map[string]any, error)
    QueryPlugin(ctx context.Context, plugin, method string, params map[string]any) (any, error)

    // Finding Submission
    SubmitFinding(ctx context.Context, finding agent.Finding) error

    // Agent Delegation
    DelegateToAgent(ctx context.Context, agentName string, task agent.Task) (agent.Result, error)

    // Memory Access
    GetMemory(ctx context.Context) MemoryManager

    // Logging
    Logger() types.Logger
}
```

### Creating a Harness

```go
config := harness.HarnessConfig{
    SlotManager:    slotManager,
    LLMRegistry:    llmRegistry,
    ToolRegistry:   toolRegistry,
    PluginRegistry: pluginRegistry,
    AgentRegistry:  agentRegistry,
    MemoryManager:  memoryManager,
    FindingStore:   findingStore,
    GraphRAGBridge: graphRAGBridge,  // Optional - enables knowledge graph
    Metrics:        metricsRecorder,
    Tracer:         tracer,
    Logger:         logger,
}

factory, err := harness.NewHarnessFactory(config)
harness, err := factory.Create("agent-name", missionCtx, targetInfo)
```

---

## Knowledge Graph (GraphRAG)

The Knowledge Graph system enables cross-mission intelligence by storing findings, relationships, and semantic embeddings in Neo4j.

### How It Works

1. **Agent submits finding** via `harness.SubmitFinding(ctx, finding)`
2. **Local storage** - Finding stored synchronously in FindingStore
3. **Async GraphRAG storage** - If enabled, finding queued for background processing
4. **Graph storage** - Finding converted to GraphNode and stored in Neo4j with:
   - Vector embedding for semantic search
   - Relationships to targets, techniques, and missions
   - Automatic similarity detection and linking

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Finding Flow                                  │
├─────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  Agent.SubmitFinding(finding)                                        │
│           │                                                           │
│           ▼                                                           │
│  ┌─────────────────┐                                                 │
│  │  FindingStore   │  ← Synchronous, mission-scoped                  │
│  │  (Local SQLite) │                                                 │
│  └────────┬────────┘                                                 │
│           │                                                           │
│           │ (if GraphRAG enabled)                                    │
│           ▼                                                           │
│  ┌─────────────────┐                                                 │
│  │ GraphRAGBridge  │  ← Async queue, non-blocking                    │
│  │  (Background)   │                                                 │
│  └────────┬────────┘                                                 │
│           │                                                           │
│           ▼                                                           │
│  ┌─────────────────────────────────────────────┐                     │
│  │              Neo4j Graph Database            │                     │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐      │                     │
│  │  │ Finding │──│ Target  │  │Technique│      │                     │
│  │  │  Node   │  │  Node   │  │  Node   │      │                     │
│  │  └────┬────┘  └─────────┘  └─────────┘      │                     │
│  │       │                                      │                     │
│  │       │ SIMILAR_TO                           │                     │
│  │       ▼                                      │                     │
│  │  ┌─────────┐                                 │                     │
│  │  │ Finding │  ← Linked via vector similarity │                     │
│  │  │  Node   │                                 │                     │
│  │  └─────────┘                                 │                     │
│  └─────────────────────────────────────────────┘                     │
│                                                                       │
└─────────────────────────────────────────────────────────────────────┘
```

### Data Models

#### Finding Node

```go
type FindingNode struct {
    ID              types.ID
    Title           string
    Description     string
    Severity        finding.Severity     // Critical, High, Medium, Low
    Confidence      float64              // 0.0 - 1.0
    Category        finding.FindingCategory
    Evidence        []finding.Evidence
    MitreTechniques []string             // e.g., "AML.T0051"
    MissionID       types.ID
    TargetID        types.ID
    Embedding       []float32            // For vector search
    CreatedAt       time.Time
}
```

#### Node Types

| Node Type | Description |
|-----------|-------------|
| `NodeTypeFinding` | Security findings discovered by agents |
| `NodeTypeTarget` | LLM targets being tested |
| `NodeTypeTechnique` | MITRE ATT&CK/ATLAS techniques |
| `NodeTypeMission` | Testing missions/campaigns |
| `NodeTypeAgent` | Executing agents |

#### Relationships

| Relationship | From | To | Description |
|-------------|------|-----|-------------|
| `DISCOVERED_ON` | Finding | Target | Finding found on this target |
| `USES_TECHNIQUE` | Finding | Technique | MITRE technique mapping |
| `SIMILAR_TO` | Finding | Finding | Semantic similarity link |
| `BELONGS_TO_MISSION` | Finding | Mission | Mission association |

### Providers

Gibson supports multiple GraphRAG provider configurations:

#### 1. Local Provider

Uses local Neo4j for graph operations and a local vector store for embeddings.

```go
provider, err := provider.NewProvider(graphrag.GraphRAGConfig{
    Enabled:  true,
    Provider: "neo4j",
    Neo4j: graphrag.Neo4jConfig{
        URI:      "bolt://localhost:7687",
        Username: "neo4j",
        Password: "password",
    },
})
```

**Benefits:**
- Full control over data
- No cloud dependencies
- Lower latency

#### 2. Cloud Provider

Routes all operations to Gibson Cloud GraphRAG API.

**Benefits:**
- Managed infrastructure
- Automatic scaling
- No local database maintenance

#### 3. Hybrid Provider

Local Neo4j for graph operations + cloud API for vector search.

**Benefits:**
- Keep sensitive graph structure local
- Leverage cloud scalability for vector search
- Reduced cloud costs

#### 4. Noop Provider

Returns empty results with zero overhead. Used when GraphRAG is disabled.

### Configuration

#### GraphRAGBridgeConfig

```go
type GraphRAGBridgeConfig struct {
    // Enabled controls whether GraphRAG storage is active
    Enabled bool

    // SimilarityThreshold is the minimum similarity score for SIMILAR_TO links
    // Range: 0.0 - 1.0, Default: 0.85
    SimilarityThreshold float64

    // MaxSimilarLinks limits the number of SIMILAR_TO relationships per finding
    // Default: 5
    MaxSimilarLinks int

    // MaxConcurrent limits concurrent storage operations
    // Default: 10
    MaxConcurrent int

    // StorageTimeout is the timeout for individual storage operations
    // Default: 30s
    StorageTimeout time.Duration
}
```

#### YAML Configuration

```yaml
graphrag:
  enabled: true
  uri: "bolt://localhost:7687"
  username: neo4j
  password: ""
  database: ""  # Empty for default
  similarity_threshold: 0.85
  max_similar_links: 5
  max_concurrent: 10
  storage_timeout: 30s
```

### Enabling GraphRAG Integration

```go
import (
    "github.com/zero-day-ai/gibson/internal/graphrag"
    "github.com/zero-day-ai/gibson/internal/harness"
)

// 1. Create GraphRAG store
graphRAGStore, err := graphrag.NewNeo4jStore(graphrag.Neo4jConfig{
    URI:      "bolt://localhost:7687",
    Username: "neo4j",
    Password: "password",
})

// 2. Create GraphRAG bridge with configuration
bridgeConfig := harness.DefaultGraphRAGBridgeConfig()
bridgeConfig.SimilarityThreshold = 0.80  // Customize threshold
bridge := harness.NewGraphRAGBridge(graphRAGStore, logger, bridgeConfig)

// 3. Configure harness with the bridge
harnessConfig := harness.HarnessConfig{
    SlotManager:    slotManager,
    GraphRAGBridge: bridge,
    // ... other config
}

// 4. Create factory and harness
factory, err := harness.NewHarnessFactory(harnessConfig)
h, err := factory.Create("agent-name", missionCtx, targetInfo)

// 5. Findings are now automatically stored in GraphRAG
err = h.SubmitFinding(ctx, finding)
```

### Graceful Degradation

The GraphRAG integration is designed to fail gracefully:

| Failure Mode | Behavior | Impact |
|--------------|----------|--------|
| Neo4j connection failure | Errors logged, finding stored locally | Cross-mission insights unavailable |
| Embedding service failure | Finding stored without embedding | Similarity search unavailable |
| Storage timeout | Operation cancelled, error logged | Finding may not be in GraphRAG |
| Shutdown during pending ops | Waits up to context timeout | Some findings may not complete |

### Querying the Knowledge Graph

```go
// Vector search for similar findings
results, err := provider.VectorSearch(ctx, embedding, 10, map[string]any{
    "severity": "critical",
})

// Query nodes by type
query := graphrag.NewNodeQuery().
    WithNodeTypes(graphrag.NodeTypeFinding).
    WithFilter("severity", "critical")
nodes, err := provider.QueryNodes(ctx, query)

// Graph traversal
nodes, err := provider.TraverseGraph(ctx, startNodeID, 3, filters)
```

---

## Memory System

Gibson provides a three-tier memory architecture for agents:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         MEMORY SYSTEM                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    WORKING MEMORY (Ephemeral)                    │    │
│  │                                                                  │    │
│  │  - In-memory key-value store                                    │    │
│  │  - Cleared after task completion                                │    │
│  │  - Fast access, no persistence                                  │    │
│  │  - Use for: Current step, temporary calculations, scratch data  │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              │                                           │
│                              ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    MISSION MEMORY (Persistent)                   │    │
│  │                                                                  │    │
│  │  - SQLite-backed storage with FTS5 search                       │    │
│  │  - Persists for duration of mission                             │    │
│  │  - Searchable and queryable                                     │    │
│  │  - Use for: Conversation history, discovered patterns, state    │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              │                                           │
│                              ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    LONG-TERM MEMORY (Vector)                     │    │
│  │                                                                  │    │
│  │  - Vector database (Qdrant, Milvus, etc.)                       │    │
│  │  - Persists across missions                                     │    │
│  │  - Semantic similarity search                                   │    │
│  │  - Use for: Attack patterns, successful payloads, learnings    │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Memory Interface

```go
type MemoryManager interface {
    // Working Memory (ephemeral)
    StoreWorking(ctx context.Context, key string, value any) error
    GetWorking(ctx context.Context, key string) (any, error)

    // Mission Memory (persistent during mission)
    StoreMission(ctx context.Context, scope UsageScope, key string, value any) error
    GetMission(ctx context.Context, scope UsageScope, key string) (any, error)
    SearchMission(ctx context.Context, scope UsageScope, query string) ([]SearchResult, error)

    // Long-Term Memory (vector-based, cross-mission)
    StoreLongTerm(ctx context.Context, embedding []float32, metadata map[string]any) error
    SearchLongTerm(ctx context.Context, embedding []float32, limit int) ([]SearchResult, error)
}
```

### Using Memory from an Agent

```go
memory := harness.GetMemory(ctx)

// Working memory - cleared after task
memory.StoreWorking(ctx, "current_step", 3)
step, _ := memory.GetWorking(ctx, "current_step")

// Mission memory - persists for mission duration
memory.StoreMission(ctx, scope, "discovered_endpoints", endpoints)
memory.SearchMission(ctx, scope, "authentication")

// Long-term memory - persists across missions
memory.StoreLongTerm(ctx, embedding, map[string]any{
    "pattern": "successful_jailbreak",
    "payload": payloadText,
})
```

---

## LLM Integration

Gibson provides a sophisticated LLM integration layer with slot-based model selection and token tracking.

### LLM Registry

Thread-safe registration and lookup of LLM providers:

```go
type LLMRegistry interface {
    RegisterProvider(provider LLMProvider) error
    UnregisterProvider(name string) error
    GetProvider(name string) (LLMProvider, error)
    ListProviders() []string
    Health(ctx context.Context) types.HealthStatus
}
```

### Slot Manager

Resolves agent slot requirements to specific providers and models:

```go
type SlotManager interface {
    ResolveSlot(ctx context.Context, slot SlotDefinition, overrides *SlotConfig) (LLMProvider, ModelInfo, error)
    ValidateSlot(ctx context.Context, slot SlotDefinition) error
}
```

#### Slot Definition

```go
type SlotDefinition struct {
    Name        string
    Description string
    Required    bool
    DefaultConfig AgentSlotConfig
    Constraints AgentSlotConstraints
}

type AgentSlotConstraints struct {
    MinContextWindow int
    RequiredFeatures []string  // e.g., "function_calling", "vision"
}
```

### Token Tracker

Hierarchical usage tracking with budget enforcement:

```go
type TokenTracker interface {
    RecordUsage(ctx context.Context, scope UsageScope, usage TokenUsage) error
    GetUsage(ctx context.Context, scope UsageScope) (TokenUsage, error)
    GetCost(ctx context.Context, scope UsageScope) (float64, error)
    SetBudget(ctx context.Context, scope UsageScope, budget Budget) error
    CheckBudget(ctx context.Context, scope UsageScope) error  // Called before API requests
    Reset(ctx context.Context, scope UsageScope) error
}

type Budget struct {
    MaxCost         float64  // Dollar limit
    MaxInputTokens  int
    MaxOutputTokens int
    MaxTotalTokens  int
}
```

### Using LLM from an Agent

```go
// Complete with slot-based model selection
messages := []llm.Message{
    {Role: llm.RoleSystem, Content: "You are a security researcher..."},
    {Role: llm.RoleUser, Content: "Analyze this response for vulnerabilities..."},
}

resp, err := harness.Complete(ctx, "primary", messages)
if err != nil {
    return agent.NewFailedResult(err), err
}

// Token usage is automatically tracked
```

---

## Workflow Engine

Gibson's workflow engine enables complex, multi-agent testing scenarios using DAG (Directed Acyclic Graph) structures defined in YAML.

### Supported Node Types

#### 1. Agent Nodes

```yaml
- id: scan
  type: agent
  name: Port Scanner
  agent: nmap-agent
  task:
    target: example.com
    ports: 1-65535
```

#### 2. Tool Nodes

```yaml
- id: generate_report
  type: tool
  name: Report Generator
  tool: pdf-generator
  input:
    format: pdf
    template: security-report
```

#### 3. Plugin Nodes

```yaml
- id: notify
  type: plugin
  name: Send Email
  plugin: email-service
  method: send
  params:
    to: security@example.com
    subject: Scan Complete
```

#### 4. Condition Nodes

```yaml
- id: check_vulns
  type: condition
  name: Check for Vulnerabilities
  condition:
    expression: findings.count > 0
    true_branch:
      - exploit_check
    false_branch:
      - clean_report
```

#### 5. Parallel Nodes

```yaml
- id: parallel_scans
  type: parallel
  name: Run Multiple Scans
  sub_nodes:
    - id: port_scan
      type: agent
      agent: nmap
    - id: vuln_scan
      type: agent
      agent: nessus
```

#### 6. Join Nodes

```yaml
- id: wait_for_scans
  type: join
  name: Wait for All Scans
  depends_on:
    - parallel_scans
```

### Execution Control

#### Dependencies

```yaml
- id: analyze
  type: agent
  agent: analyzer
  depends_on:
    - scan
    - recon
```

#### Timeouts

```yaml
timeout: 30s    # 30 seconds
timeout: 5m     # 5 minutes
timeout: 2h     # 2 hours
```

#### Retry Policies

```yaml
# Exponential backoff
retry:
  max_retries: 5
  backoff: exponential
  initial_delay: 1s
  max_delay: 60s
  multiplier: 2.0

# Constant backoff
retry:
  max_retries: 3
  backoff: constant
  initial_delay: 5s

# Linear backoff
retry:
  max_retries: 5
  backoff: linear
  initial_delay: 1s
```

### Complete Workflow Example

```yaml
name: comprehensive-llm-audit
description: Full security audit of target LLM

nodes:
  - id: recon
    type: agent
    agent: reconnaissance
    config:
      goal: "Discover target capabilities and behaviors"

  - id: injection
    type: agent
    agent: prompt-injector
    dependencies: [recon]
    timeout: 10m
    retry:
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      max_delay: 60s
    config:
      goal: "Test for prompt injection vulnerabilities"

  - id: jailbreak
    type: agent
    agent: jailbreaker
    dependencies: [recon]
    config:
      goal: "Attempt to bypass content filters"

  - id: analysis
    type: agent
    agent: analyzer
    dependencies: [injection, jailbreak]
    config:
      goal: "Consolidate and prioritize findings"
```

### Using the Workflow Engine

```go
// Parse workflow from YAML
wf, err := workflow.ParseWorkflowFile("workflow.yaml")
if err != nil {
    log.Fatal(err)
}

// Execute workflow
executor := workflow.NewWorkflowExecutor()
result, err := executor.Execute(ctx, wf, harness)

// Check results
fmt.Printf("Status: %s\n", result.Status)
fmt.Printf("Nodes executed: %d\n", result.NodesExecuted)
```

---

## Building Custom Components

### Creating a Custom Agent

```go
package main

import (
    "context"
    "log"

    "github.com/zero-day-ai/sdk/agent"
    "github.com/zero-day-ai/sdk/llm"
    "github.com/zero-day-ai/sdk/serve"
    "github.com/zero-day-ai/sdk/types"
)

func main() {
    cfg := agent.NewConfig().
        SetName("my-security-agent").
        SetVersion("1.0.0").
        SetDescription("Custom security testing agent").
        AddCapability(agent.CapabilityPromptInjection).
        AddTargetType(types.TargetTypeLLMChat).
        AddLLMSlot("primary", llm.SlotRequirements{
            MinContextWindow: 8000,
        }).
        SetExecuteFunc(func(ctx context.Context, harness agent.Harness, task agent.Task) (agent.Result, error) {
            logger := harness.Logger()
            logger.Info("executing security test", "goal", task.Goal)

            // Access memory
            memory := harness.GetMemory(ctx)

            // Make LLM calls
            messages := []llm.Message{
                {Role: llm.RoleUser, Content: task.Goal},
            }
            resp, err := harness.Complete(ctx, "primary", messages)
            if err != nil {
                return agent.NewFailedResult(err), err
            }

            // Submit findings
            if vulnerabilityFound {
                harness.SubmitFinding(ctx, agent.Finding{
                    Title:       "Vulnerability Found",
                    Description: "...",
                    Severity:    agent.SeverityHigh,
                    Confidence:  0.9,
                })
            }

            return agent.NewSuccessResult(resp.Content), nil
        })

    myAgent, err := agent.New(cfg)
    if err != nil {
        log.Fatal(err)
    }

    // Serve over gRPC
    if err := serve.Agent(myAgent, serve.WithPort(50051)); err != nil {
        log.Fatal(err)
    }
}
```

### Creating a Custom Tool

```go
cfg := tool.NewConfig().
    SetName("http-scanner").
    SetVersion("1.0.0").
    SetDescription("HTTP endpoint scanner").
    SetInputSchema(schema.Object(map[string]schema.JSON{
        "url":    schema.String(),
        "method": schema.Enum("GET", "POST", "PUT", "DELETE"),
    }, "url", "method")).
    SetOutputSchema(schema.Object(map[string]schema.JSON{
        "status_code": schema.Int(),
        "body":        schema.String(),
    }, "status_code", "body")).
    SetExecuteFunc(func(ctx context.Context, input map[string]any) (map[string]any, error) {
        url := input["url"].(string)
        method := input["method"].(string)

        // Perform HTTP request
        resp, err := httpClient.Do(req)
        if err != nil {
            return nil, err
        }

        return map[string]any{
            "status_code": resp.StatusCode,
            "body":        string(body),
        }, nil
    })
```

### Creating a Custom Plugin

```go
type DatabasePlugin struct {
    db *sql.DB
}

func (p *DatabasePlugin) Name() string { return "database" }
func (p *DatabasePlugin) Version() string { return "1.0.0" }

func (p *DatabasePlugin) Initialize(ctx context.Context, cfg plugin.PluginConfig) error {
    db, err := sql.Open("postgres", cfg.ConnectionString)
    if err != nil {
        return err
    }
    p.db = db
    return nil
}

func (p *DatabasePlugin) Shutdown(ctx context.Context) error {
    return p.db.Close()
}

func (p *DatabasePlugin) Query(ctx context.Context, method string, params map[string]any) (any, error) {
    switch method {
    case "search_findings":
        return p.searchFindings(ctx, params)
    case "get_finding":
        return p.getFinding(ctx, params)
    default:
        return nil, fmt.Errorf("unknown method: %s", method)
    }
}

func (p *DatabasePlugin) Methods() []plugin.MethodDescriptor {
    return []plugin.MethodDescriptor{
        {Name: "search_findings", Description: "Search findings by query"},
        {Name: "get_finding", Description: "Get a specific finding by ID"},
    }
}
```

### Component Installation

```bash
# Install from dedicated repository
gibson agent install https://github.com/user/my-agent

# Install from mono-repo subdirectory
gibson agent install https://github.com/user/agents#security/scanner

# Bulk install all agents from mono-repo
gibson agent install-all https://github.com/user/gibson-agents
```

---

## Prompt Relay System

The prompt relay system enables parent agents to delegate tasks to sub-agents with context preservation.

### Components

1. **PromptTransformer** - Modifies prompts during relay
2. **RelayContext** - Provides delegation context
3. **PromptRelay** - Coordinates transformation pipeline

### Built-in Transformers

#### ContextInjector

Adds parent agent context to delegated prompts:

```go
contextInjector := transformers.NewContextInjector()
contextInjector.IncludeMemory = true
contextInjector.IncludeConstraints = true
```

#### ScopeNarrower

Filters prompts to task-relevant content:

```go
narrower := transformers.NewScopeNarrower()
narrower.AllowedPositions = []Position{PositionSystem, PositionUser}
narrower.KeywordFilter = []string{"security", "authentication"}
```

### Usage Example

```go
relay := prompt.NewPromptRelay()
ctx := &prompt.RelayContext{
    SourceAgent: "SecurityOrchestrator",
    TargetAgent: "VulnerabilityScanner",
    Task:        "Scan authentication module",
    Memory: map[string]any{
        "target": "auth_module",
    },
    Constraints: []string{
        "Read-only access",
        "Report all findings",
    },
    Prompts: originalPrompts,
}

// Apply transformations
result, err := relay.Relay(ctx, contextInjector, scopeNarrower)
```

---

## Observability

Gibson provides comprehensive observability through OpenTelemetry integration.

### Components

| Component | Purpose |
|-----------|---------|
| **Tracing** | Distributed traces for LLM calls, tool executions |
| **Metrics** | Token usage, latency, costs, findings |
| **Logging** | Structured JSON with trace correlation |
| **Langfuse** | LLM-specific observability platform |

### Configuration

```yaml
logging:
  level: info
  format: json

tracing:
  enabled: true
  provider: otlp
  endpoint: "localhost:4317"
  service_name: gibson
  sample_rate: 0.1

metrics:
  enabled: true
  provider: prometheus
  port: 9090

langfuse:
  public_key: "pk-lf-..."
  secret_key: "sk-lf-..."
  host: "https://cloud.langfuse.com"
```

### Available Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `gibson.llm.completions` | Counter | LLM completion count |
| `gibson.llm.tokens.input` | Counter | Input tokens consumed |
| `gibson.llm.tokens.output` | Counter | Output tokens generated |
| `gibson.llm.latency` | Histogram | Completion latency |
| `gibson.llm.cost` | Counter | Cost in USD |
| `gibson.tool.calls` | Counter | Tool execution count |
| `gibson.findings.submitted` | Counter | Findings submitted |

---

## Requirements and Dependencies

### System Requirements

- **Go**: 1.24.4 or later
- **CGO**: Enabled (for SQLite FTS5 support)
- **Neo4j**: Optional, for GraphRAG

### External Dependencies

- `neo4j/neo4j-go-driver/v5` - Graph database
- `charmbracelet/bubbles/bubbletea` - Terminal UI
- `spf13/cobra/viper` - CLI and configuration
- `mattn/go-sqlite3` - Database
- `langchaingo` - LLM integration
- `opentelemetry` - Observability

### Database

- **SQLite3** with FTS5 (full-text search)
- WAL mode for concurrency
- AES-256-GCM encryption for credentials

### GraphRAG Requirements

To enable the Knowledge Graph:

1. **Neo4j Instance** (v5.x recommended)
   ```bash
   docker run -d \
     --name neo4j \
     -p 7474:7474 -p 7687:7687 \
     -e NEO4J_AUTH=neo4j/password \
     neo4j:5
   ```

2. **Vector Embedding Provider** (optional, for similarity search)
   - Local: Ollama, sentence-transformers
   - Cloud: OpenAI embeddings API

3. **Configuration**
   ```yaml
   graphrag:
     enabled: true
     uri: "bolt://localhost:7687"
     username: neo4j
     password: password
   ```

---

## Summary

Gibson provides a comprehensive platform for AI security testing with:

1. **Modular SDK** - Build custom agents, tools, and plugins
2. **Knowledge Graph** - Cross-mission intelligence via GraphRAG
3. **Three-tier Memory** - Working, mission, and long-term memory
4. **Workflow Engine** - DAG-based multi-agent orchestration
5. **Enterprise Features** - Encryption, observability, budget tracking
6. **Graceful Degradation** - Fails safely when components unavailable

For more detailed documentation on specific components, see the README files in each package directory:

- `internal/harness/README.md` - Agent Harness details
- `internal/graphrag/graph/README.md` - Graph client documentation
- `internal/graphrag/provider/README.md` - Provider implementations
- `internal/workflow/YAML_PARSER_README.md` - Workflow YAML format
- `internal/prompt/RELAY_SYSTEM.md` - Prompt relay system
- `docs/OBSERVABILITY.md` - Observability guide
