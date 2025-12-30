# Agent Harness Package

The harness package provides the runtime environment for Gibson agents, orchestrating LLM interactions, tool execution, memory access, finding storage, and knowledge graph integration.

## Overview

The Agent Harness is the central orchestration point for agent execution. It provides:

- **LLM Integration**: Slot-based model selection and completion APIs
- **Tool Execution**: Registry-based tool lookup and execution
- **Memory Management**: Three-tier memory (working, mission, long-term)
- **Finding Storage**: Local finding store with filtering
- **GraphRAG Integration**: Async storage to knowledge graph for cross-mission insights
- **Delegation**: Parent-child agent relationships for complex workflows

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Agent Execution                              │
└─────────────────────────────┬───────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                       AgentHarness Interface                         │
│  ┌──────────┐ ┌──────────┐ ┌─────────────┐ ┌───────────────────┐   │
│  │ Complete │ │ExecuteTool│ │SubmitFinding│ │ GetMemory/Store  │   │
│  └────┬─────┘ └────┬─────┘ └──────┬──────┘ └─────────┬─────────┘   │
└───────┼────────────┼──────────────┼──────────────────┼──────────────┘
        │            │              │                  │
        ▼            ▼              ▼                  ▼
┌───────────┐ ┌───────────┐ ┌──────────────┐  ┌──────────────────┐
│SlotManager│ │ToolRegistry│ │ FindingStore │  │  MemoryManager   │
└───────────┘ └───────────┘ └──────┬───────┘  └──────────────────┘
                                   │
                                   │ (async)
                                   ▼
                          ┌────────────────┐
                          │ GraphRAGBridge │
                          └────────┬───────┘
                                   │
                                   ▼
                          ┌────────────────┐
                          │ GraphRAGStore  │
                          │   (Neo4j +     │
                          │  Embeddings)   │
                          └────────────────┘
```

## GraphRAG Integration

### Overview

The GraphRAG integration enables automatic storage of security findings into a knowledge graph (Neo4j) with vector embeddings for semantic search. This allows:

- **Cross-mission insights**: Query findings across all missions
- **Similarity detection**: Automatically link related findings
- **Knowledge accumulation**: Build organizational security knowledge over time
- **Relationship discovery**: Track MITRE techniques, targets, and finding connections

### How Findings Flow to the Knowledge Graph

1. Agent calls `harness.SubmitFinding(ctx, finding)`
2. Finding is stored in the local FindingStore (synchronous, mission-scoped)
3. If GraphRAGBridge is configured, finding is queued for async storage
4. Background goroutine:
   - Converts finding to FindingNode format
   - Stores in Neo4j with vector embedding
   - Creates relationships (DISCOVERED_ON, USES_TECHNIQUE)
   - Detects and links similar findings (SIMILAR_TO)
5. Errors are logged but don't affect the mission

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

#### Default Configuration

```go
config := harness.DefaultGraphRAGBridgeConfig()
// Returns:
// - Enabled: true
// - SimilarityThreshold: 0.85
// - MaxSimilarLinks: 5
// - MaxConcurrent: 10
// - StorageTimeout: 30s
```

### Enabling GraphRAG Integration

```go
import (
    "github.com/zero-day-ai/gibson/internal/graphrag"
    "github.com/zero-day-ai/gibson/internal/harness"
)

// 1. Create GraphRAG store (requires Neo4j and embedding provider)
graphRAGStore, err := graphrag.NewNeo4jStore(graphrag.Neo4jConfig{
    URI:      "bolt://localhost:7687",
    Username: "neo4j",
    Password: "password",
})
if err != nil {
    log.Fatal(err)
}

// 2. Create GraphRAG bridge with configuration
bridgeConfig := harness.DefaultGraphRAGBridgeConfig()
bridgeConfig.SimilarityThreshold = 0.80  // Optional: customize threshold
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

// 5. Use harness normally - GraphRAG storage happens automatically
err = h.SubmitFinding(ctx, finding)

// 6. Close harness when done (waits for pending GraphRAG operations)
if closeable, ok := h.(*harness.DefaultAgentHarness); ok {
    err = closeable.Close(ctx)
}
```

### Disabling GraphRAG

When GraphRAG is not configured (or disabled), a `NoopGraphRAGBridge` is used automatically:

```go
// Option 1: Don't set GraphRAGBridge - defaults to NoopGraphRAGBridge
harnessConfig := harness.HarnessConfig{
    SlotManager: slotManager,
    // GraphRAGBridge not set - defaults to NoopGraphRAGBridge
}

// Option 2: Explicitly disable
bridgeConfig := harness.DefaultGraphRAGBridgeConfig()
bridgeConfig.Enabled = false

// Option 3: Use NoopGraphRAGBridge directly
harnessConfig.GraphRAGBridge = &harness.NoopGraphRAGBridge{}
```

The NoopGraphRAGBridge has zero overhead - no allocations, no goroutines.

### Relationships Created

The GraphRAG integration creates several relationship types:

| Relationship | From | To | When Created |
|-------------|------|-----|--------------|
| `DISCOVERED_ON` | FindingNode | TargetNode | When targetID is provided |
| `USES_TECHNIQUE` | FindingNode | TechniqueNode | When finding has MITRE techniques |
| `SIMILAR_TO` | FindingNode | FindingNode | When similarity > threshold |
| `BELONGS_TO_MISSION` | FindingNode | MissionNode | Always (via missionID) |

### Failure Modes and Graceful Degradation

The GraphRAG integration is designed to fail gracefully:

#### Neo4j Connection Failure
- **Symptom**: Storage operations fail with connection errors
- **Behavior**: Errors logged at WARN level, finding still stored locally
- **Impact**: Cross-mission insights unavailable until Neo4j recovers
- **Recovery**: Automatic on next finding submission

#### Embedding Service Failure
- **Symptom**: Vector embedding generation fails
- **Behavior**: Finding stored without embedding (limited similarity search)
- **Impact**: Similarity detection may not work for affected findings
- **Recovery**: Consider re-embedding findings after service recovery

#### Storage Timeout
- **Symptom**: Operations exceed StorageTimeout
- **Behavior**: Operation cancelled, error logged
- **Impact**: Finding may not be stored in GraphRAG
- **Recovery**: Increase StorageTimeout or investigate Neo4j performance

#### Shutdown During Pending Operations
- **Symptom**: Harness closes while GraphRAG operations in progress
- **Behavior**: Shutdown waits up to context timeout for completion
- **Impact**: Some findings may not complete storage if timeout exceeded
- **Recovery**: Use longer shutdown timeouts for high-volume scenarios

### Troubleshooting

#### Check GraphRAG Health

```go
status := bridge.Health(ctx)
if status != types.HealthStatusHealthy {
    log.Warn("GraphRAG unhealthy", "status", status)
}
```

#### Enable Debug Logging

The bridge logs at debug level for successful operations and warn level for errors:

```go
// Debug logs show:
// - "storing finding to graphrag" (start)
// - "stored finding to graphrag" (success)
// - "found N similar findings" (similarity detection)

// Warn logs show:
// - "failed to store finding" (storage error)
// - "failed to create relationship" (relationship error)
// - "failed to detect similar findings" (similarity error)
// - "graphrag bridge shutdown error" (shutdown error)
```

#### Verify Async Operations Complete

```go
// Before shutdown, ensure all operations complete
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if closeable, ok := harness.(*harness.DefaultAgentHarness); ok {
    if err := closeable.Close(ctx); err != nil {
        log.Error("shutdown error - some findings may not be in GraphRAG", "error", err)
    }
}
```

### Performance Considerations

1. **Semaphore Limiting**: MaxConcurrent prevents unbounded goroutine growth
2. **Async by Design**: SubmitFinding never blocks on GraphRAG
3. **Similarity Search Cost**: Can be slow on large graphs - tune MaxSimilarLinks
4. **Memory Usage**: Each pending operation holds finding data until complete

### Testing

The GraphRAG integration includes comprehensive tests:

```bash
# Unit tests (mock GraphRAG store)
go test -v -run TestDefaultGraphRAGBridge ./internal/harness/

# Integration tests (full harness flow)
go test -v -run TestHarness.*GraphRAG ./internal/harness/

# With race detector
go test -v -race -run GraphRAG ./internal/harness/
```

## Usage

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
    GraphRAGBridge: graphRAGBridge, // Optional
    Metrics:        metricsRecorder,
    Tracer:         tracer,
    Logger:         logger,
}

factory, err := harness.NewHarnessFactory(config)
if err != nil {
    log.Fatal(err)
}

harness, err := factory.Create("agent-name", missionCtx, targetInfo)
if err != nil {
    log.Fatal(err)
}
```

### Submitting Findings

```go
finding := agent.Finding{
    ID:          types.NewID(),
    Title:       "SQL Injection in Login Form",
    Description: "Found SQL injection vulnerability...",
    Severity:    agent.SeverityCritical,
    Confidence:  0.95,
    Category:    "injection",
    Techniques:  []string{"T1190"}, // MITRE ATT&CK
    CreatedAt:   time.Now(),
}

err := harness.SubmitFinding(ctx, finding)
// Finding is now in local store AND queued for GraphRAG
```

### Delegation to Sub-Agents

```go
// Create child harness for sub-agent
childHarness, err := factory.CreateChild(parentHarness, "child-agent")

// Child shares memory and findings with parent
// Child has its own token tracking
```

## Files

| File | Description |
|------|-------------|
| `interface.go` | AgentHarness interface definition |
| `implementation.go` | DefaultAgentHarness implementation |
| `factory.go` | HarnessFactory for creating harnesses |
| `config.go` | HarnessConfig and validation |
| `graphrag_bridge.go` | GraphRAGBridge interface and implementations |
| `errors.go` | Error definitions |
| `*_test.go` | Unit and integration tests |

## Dependencies

- `internal/agent`: Finding and agent types
- `internal/graphrag`: GraphRAGStore interface
- `internal/llm`: LLM provider interfaces
- `internal/memory`: Memory manager
- `internal/types`: Common types (ID, HealthStatus)
