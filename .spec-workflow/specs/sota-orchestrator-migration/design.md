# Design: SOTA Orchestrator Migration

## Overview

**Spec Name**: `sota-orchestrator-migration`
**Status**: Draft
**Last Updated**: 2026-01-15

This document describes the technical design for migrating all code from the old `DefaultMissionOrchestrator` (in `internal/mission/orchestrator.go`) to the new SOTA orchestrator (in `internal/orchestrator/`).

---

## Architecture Overview

### Current State

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Current Architecture                         │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  cmd/gibson/orchestrator.go ─────┐                                  │
│  cmd/gibson/attack.go ───────────┼──► DefaultMissionOrchestrator    │
│  internal/daemon/daemon.go ──────┘    (internal/mission/)           │
│                                              │                       │
│                                              ▼                       │
│                                    WorkflowExecutor                  │
│                                    (Sequential execution)            │
│                                    (No LLM reasoning)                │
│                                                                      │
│  internal/daemon/mission_manager.go ──► SOTA Orchestrator           │
│                                         (internal/orchestrator/)     │
│                                                │                     │
│                                                ▼                     │
│                                    Observe → Think → Act Loop        │
│                                    (LLM-driven decisions)            │
│                                    (Neo4j GraphRAG state)            │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Target State

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Target Architecture                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  cmd/gibson/orchestrator.go ─────┐                                  │
│  cmd/gibson/attack.go ───────────┼──► SOTA Orchestrator             │
│  internal/daemon/daemon.go ──────┤    (internal/orchestrator/)       │
│  internal/daemon/mission_manager ┘              │                    │
│                                                 ▼                    │
│                                    ┌────────────────────────┐       │
│                                    │   Observe → Think → Act │       │
│                                    │                        │       │
│                                    │  Observer (Neo4j)      │       │
│                                    │  Thinker (LLM)         │       │
│                                    │  Actor (Execution)     │       │
│                                    └────────────────────────┘       │
│                                                                      │
│  OLD CODE DELETED:                                                   │
│  ├── internal/mission/orchestrator.go                               │
│  ├── internal/mission/orchestrator_eval.go                          │
│  └── internal/daemon/daemon.go.bak                                  │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Component Design

### 1. SOTA Orchestrator Core (Already Implemented)

The SOTA orchestrator in `internal/orchestrator/` implements the Observe → Think → Act reasoning loop:

```go
// Core interfaces (internal/orchestrator/orchestrator.go)
type OrchestratorObserver interface {
    Observe(ctx context.Context, missionID string) (*ObservationState, error)
}

type OrchestratorThinker interface {
    Think(ctx context.Context, state *ObservationState) (*ThinkResult, error)
}

type OrchestratorActor interface {
    Act(ctx context.Context, decision *Decision, missionID string) (*ActionResult, error)
}
```

**Components:**
- `Observer` (`observe.go`): Queries Neo4j for mission state, ready nodes, running nodes, completed nodes
- `Thinker` (`think.go`): Uses LLM to make decisions with structured output and retry logic
- `Actor` (`act.go`): Executes decisions (6 action types: execute_agent, skip_agent, modify_params, retry, spawn_agent, complete)

### 2. Interface Adapter Layer (New)

To maintain backward compatibility with existing code that expects `MissionOrchestrator` interface:

```go
// internal/orchestrator/adapter.go (NEW FILE)
package orchestrator

import (
    "context"
    "github.com/zero-day-ai/gibson/internal/mission"
    "github.com/zero-day-ai/gibson/internal/types"
)

// SOTAMissionOrchestrator adapts the SOTA orchestrator to the MissionOrchestrator interface
type SOTAMissionOrchestrator struct {
    orchestrator *Orchestrator

    // Legacy compatibility
    pauseRequested map[types.ID]bool
    pauseMu        sync.RWMutex
}

// Execute implements mission.MissionOrchestrator
func (s *SOTAMissionOrchestrator) Execute(ctx context.Context, m *mission.Mission) (*mission.MissionResult, error) {
    // Convert mission to SOTA format and execute
    result, err := s.orchestrator.Run(ctx, m.ID.String())
    // Convert result back to MissionResult
    return s.convertResult(result, m), err
}

// RequestPause implements pause functionality
func (s *SOTAMissionOrchestrator) RequestPause(ctx context.Context, missionID types.ID) error

// IsPauseRequested checks pause state
func (s *SOTAMissionOrchestrator) IsPauseRequested(missionID types.ID) bool

// ClearPauseRequest clears pause state
func (s *SOTAMissionOrchestrator) ClearPauseRequest(missionID types.ID)

// ExecuteFromCheckpoint resumes from checkpoint
func (s *SOTAMissionOrchestrator) ExecuteFromCheckpoint(ctx context.Context, m *mission.Mission, cp *mission.MissionCheckpoint) (*mission.MissionResult, error)
```

### 3. Constructor Factory (New)

```go
// internal/orchestrator/factory.go (NEW FILE)
package orchestrator

import (
    "github.com/zero-day-ai/gibson/internal/graphrag/queries"
    "github.com/zero-day-ai/gibson/internal/harness"
)

// SOTAOrchestratorConfig holds configuration for creating a SOTA orchestrator
type SOTAOrchestratorConfig struct {
    GraphRAGClient    interface{}
    HarnessFactory    harness.HarnessFactoryInterface
    Logger            *slog.Logger
    Tracer            trace.Tracer
    EventBus          EventBus
    MaxIterations     int
    MaxConcurrent     int
    Budget            int
    Timeout           time.Duration
}

// NewSOTAMissionOrchestrator creates a new SOTA-based mission orchestrator
func NewSOTAMissionOrchestrator(cfg SOTAOrchestratorConfig) (*SOTAMissionOrchestrator, error) {
    // Create query handlers
    missionQueries := queries.NewMissionQueries(cfg.GraphRAGClient)
    executionQueries := queries.NewExecutionQueries(cfg.GraphRAGClient)

    // Create components
    observer := NewObserver(missionQueries, executionQueries)
    thinker := NewThinker(/* LLM client from harness */)
    actor := NewActor(/* harness adapter */, executionQueries, missionQueries, cfg.GraphRAGClient)

    // Create orchestrator
    orch := NewOrchestrator(observer, thinker, actor,
        WithMaxIterations(cfg.MaxIterations),
        WithMaxConcurrent(cfg.MaxConcurrent),
        WithBudget(cfg.Budget),
        WithTimeout(cfg.Timeout),
        WithLogger(cfg.Logger),
        WithTracer(cfg.Tracer),
        WithEventBus(cfg.EventBus),
    )

    return &SOTAMissionOrchestrator{
        orchestrator:   orch,
        pauseRequested: make(map[types.ID]bool),
    }, nil
}
```

### 4. Eval Integration (Port from Old)

```go
// internal/orchestrator/eval.go (NEW FILE)
package orchestrator

import (
    "github.com/zero-day-ai/gibson/internal/eval"
)

// EvalOrchestrator wraps SOTA orchestrator with eval capabilities
type EvalOrchestrator struct {
    *SOTAMissionOrchestrator
    evalOptions   *eval.EvalOptions
    evalCollector *eval.EvalResultCollector
}

// WithEvalOptions adds eval integration
func WithEvalOptions(opts *eval.EvalOptions) SOTAOrchestratorOption {
    return func(s *SOTAMissionOrchestrator) {
        // Wrap harness factory with eval capabilities
    }
}

// GetEvalResults returns collected eval results
func (e *EvalOrchestrator) GetEvalResults() []eval.EvalResult

// FinalizeEvalResults completes eval collection
func (e *EvalOrchestrator) FinalizeEvalResults() error
```

---

## Migration Strategy

### Phase 1: Create Adapter Layer

1. Create `internal/orchestrator/adapter.go` with `SOTAMissionOrchestrator`
2. Implement `MissionOrchestrator` interface on adapter
3. Create factory function `NewSOTAMissionOrchestrator`
4. Add pause/checkpoint support to adapter

### Phase 2: Migrate Production Code

**File: `cmd/gibson/orchestrator.go`**
```go
// BEFORE
import "github.com/zero-day-ai/gibson/internal/mission"
orch, err := mission.NewMissionOrchestrator(store, opts...)

// AFTER
import "github.com/zero-day-ai/gibson/internal/orchestrator"
orch, err := orchestrator.NewSOTAMissionOrchestrator(orchestrator.SOTAOrchestratorConfig{
    GraphRAGClient: graphRAGClient,
    HarnessFactory: harnessFactory,
    // ...
})
```

**File: `cmd/gibson/attack.go`**
```go
// Same pattern as orchestrator.go
```

**File: `internal/daemon/daemon.go`**
```go
// Same pattern - update orchestrator creation
```

### Phase 3: Port Eval Integration

1. Create `internal/orchestrator/eval.go`
2. Port `wrapFactoryWithEval` logic
3. Port `GetEvalResults` and `FinalizeEvalResults`
4. Update tests

### Phase 4: Migrate Tests

For each test file:
1. Update imports from `internal/mission` to `internal/orchestrator`
2. Replace `DefaultMissionOrchestrator` with `SOTAMissionOrchestrator`
3. Update mock interfaces as needed
4. Verify tests pass

### Phase 5: Delete Old Code

1. Delete `internal/mission/orchestrator.go`
2. Delete `internal/mission/orchestrator_eval.go`
3. Delete `internal/daemon/daemon.go.bak`
4. Remove unused imports and references

---

## Interface Mapping

### Old vs New Interface Methods

| Old (DefaultMissionOrchestrator) | New (SOTAMissionOrchestrator) |
|----------------------------------|-------------------------------|
| `Execute(ctx, mission)` | `Execute(ctx, mission)` → wraps `orchestrator.Run()` |
| `RequestPause(ctx, missionID)` | `RequestPause(ctx, missionID)` |
| `IsPauseRequested(missionID)` | `IsPauseRequested(missionID)` |
| `ClearPauseRequest(missionID)` | `ClearPauseRequest(missionID)` |
| `ExecuteFromCheckpoint(ctx, m, cp)` | `ExecuteFromCheckpoint(ctx, m, cp)` |

### Old vs New Options

| Old Option | New Option |
|------------|------------|
| `WithEventEmitter(emitter)` | `WithEventBus(bus)` |
| `WithWorkflowExecutor(exec)` | N/A (Actor handles execution) |
| `WithHarnessFactory(factory)` | `SOTAOrchestratorConfig.HarnessFactory` |
| `WithMissionService(service)` | N/A (queries handle this) |
| `WithEventStore(store)` | `WithDecisionLogWriter(writer)` |
| `WithTargetStore(store)` | `SOTAOrchestratorConfig.TargetStore` |
| `WithEvalOptions(opts)` | `WithEvalOptions(opts)` |

---

## Data Flow

### Mission Execution Flow

```
┌──────────────────────────────────────────────────────────────────────┐
│                        Mission Execution Flow                         │
└──────────────────────────────────────────────────────────────────────┘

1. API Request → daemon.go → StartMission()
                                  │
                                  ▼
2. Create SOTAMissionOrchestrator with config
   ├── GraphRAG client (Neo4j)
   ├── Harness factory
   ├── Event bus
   └── Resource constraints
                                  │
                                  ▼
3. Execute(ctx, mission)
   │
   ├── Store workflow in Neo4j graph
   │
   └── orchestrator.Run(ctx, missionID)
       │
       └── Observe → Think → Act Loop
           │
           ├── OBSERVE: Query Neo4j for state
           │   ├── Ready nodes
           │   ├── Running nodes
           │   ├── Completed nodes
           │   └── Recent decisions
           │
           ├── THINK: LLM decides next action
           │   ├── Structured output (preferred)
           │   ├── Text fallback with parsing
           │   └── Retry logic (max 3)
           │
           └── ACT: Execute decision
               ├── execute_agent → DelegateToAgent()
               ├── skip_agent → Mark skipped in graph
               ├── modify_params → Update node params
               ├── retry → Re-execute failed node
               ├── spawn_agent → Create dynamic node
               └── complete → Terminal, exit loop
                                  │
                                  ▼
4. Convert OrchestratorResult → MissionResult
   ├── Status mapping
   ├── Metrics aggregation
   └── Finding collection
                                  │
                                  ▼
5. Return to caller (API, CLI, etc.)
```

---

## Error Handling

### Observation Errors
- GraphRAG connection failures → Retry with backoff
- Invalid mission ID → Return error immediately

### Think Errors
- LLM provider errors → Retry up to `maxRetries`
- Parse failures → Fallback to text output, retry
- All retries exhausted → Return error

### Action Errors
- Agent execution failure → Record in graph, continue loop
- Graph update failure → Log and continue
- Terminal errors → Stop orchestration

---

## Configuration Defaults

| Parameter | Default | Description |
|-----------|---------|-------------|
| MaxIterations | 100 | Maximum observe-think-act cycles |
| MaxConcurrent | 10 | Maximum parallel node executions |
| Budget | 0 (unlimited) | Token budget for LLM calls |
| Timeout | 0 (none) | Overall orchestration timeout |
| ThinkerMaxRetries | 3 | LLM call retries on failure |
| ThinkerTemperature | 0.2 | LLM temperature for decisions |

---

## Testing Strategy

### Unit Tests
- Mock `OrchestratorObserver`, `OrchestratorThinker`, `OrchestratorActor` interfaces
- Test adapter conversion logic
- Test pause/checkpoint state management

### Integration Tests
- Use in-memory GraphRAG for faster tests
- Test full observe-think-act loop
- Test resource constraint enforcement

### E2E Tests
- Test with real Neo4j and LLM
- Test pause/resume scenarios
- Test checkpoint recovery

---

## Rollback Plan

If issues are discovered after migration:

1. **Immediate**: Revert to commit before migration
2. **Partial**: Keep SOTA orchestrator but restore old code path as fallback
3. **Feature flag**: Add environment variable to switch between implementations

```go
// Potential feature flag approach
if os.Getenv("GIBSON_USE_LEGACY_ORCHESTRATOR") == "true" {
    return mission.NewMissionOrchestrator(store, opts...)
}
return orchestrator.NewSOTAMissionOrchestrator(cfg)
```

---

## Success Criteria

1. All existing tests pass with SOTA orchestrator
2. No `DefaultMissionOrchestrator` references in codebase
3. Mission execution produces equivalent or better results
4. Decision history tracked in Neo4j
5. Eval integration functional
6. Pause/checkpoint features working

---

## Open Questions

1. **Backward Compatibility**: Should we keep `MissionOrchestrator` interface or create new interface?
   - **Decision**: Keep interface for backward compatibility via adapter

2. **Eval Integration**: Port eval logic to SOTA or create new eval approach?
   - **Decision**: Port existing eval logic first, optimize later

3. **Checkpoint Format**: Use existing checkpoint format or define new one?
   - **Decision**: Use existing format, adapter handles conversion
