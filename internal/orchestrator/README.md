# Orchestrator Package

The orchestrator package implements Gibson's main control loop that autonomously executes security testing workflows using LLM-based reasoning.

## Architecture

The orchestrator follows a **Observe → Think → Act** pattern:

```
┌──────────────────────────────────────────────────────────────┐
│                     Orchestrator Loop                         │
├──────────────────────────────────────────────────────────────┤
│                                                               │
│  1. OBSERVE: Gather execution state from graph               │
│     └─> ObservationState (ready nodes, running, metrics)     │
│                                                               │
│  2. THINK: LLM decides what to do next                       │
│     └─> Decision (action, target, reasoning, confidence)     │
│                                                               │
│  3. ACT: Execute the decision                                │
│     └─> ActionResult (execution status, errors, metadata)    │
│                                                               │
│  4. CHECK: Verify termination conditions                     │
│     └─> Complete, max iterations, budget, timeout            │
│                                                               │
└──────────────────────────────────────────────────────────────┘
```

## Components

### Orchestrator
Main control loop coordinator. Manages:
- Iteration loop with termination conditions
- Token budget tracking
- Concurrency limits
- Timeout management
- Event emission
- Decision logging

### Observer (`observe.go`)
Gathers execution state from the graph database:
- Mission metadata and progress
- Workflow node states (ready, running, completed, failed)
- Recent decisions for context
- Resource constraints

### Thinker (`think.go`)
Uses LLM to make orchestration decisions:
- Analyzes observation state
- Produces structured decisions with reasoning
- Handles retries for parse failures
- Tracks token usage and latency

### Actor (`act.go`)
Executes decisions and updates graph state:
- Delegates to agents via harness
- Updates workflow node status
- Creates dynamic nodes (spawn_agent)
- Handles retries and modifications

## Usage

### Basic Example

```go
import (
    "context"
    "time"
    "github.com/zero-day-ai/gibson/internal/orchestrator"
)

// Initialize components
observer := orchestrator.NewObserver(missionQueries, executionQueries)
thinker := orchestrator.NewThinker(llmClient)
actor := orchestrator.NewActor(harness, execQueries, missionQueries, graphClient)

// Create orchestrator with configuration
orch := orchestrator.NewOrchestrator(
    observer,
    thinker,
    actor,
    orchestrator.WithMaxIterations(100),
    orchestrator.WithBudget(100000),        // 100k token budget
    orchestrator.WithMaxConcurrent(5),      // Max 5 parallel executions
    orchestrator.WithTimeout(30*time.Minute),
    orchestrator.WithLogger(logger),
    orchestrator.WithTracer(tracer),
    orchestrator.WithEventBus(eventBus),
)

// Run orchestration
ctx := context.Background()
result, err := orch.Run(ctx, missionID)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Status: %s\n", result.Status)
fmt.Printf("Completed %d/%d nodes in %d iterations\n",
    result.CompletedNodes,
    result.CompletedNodes + result.FailedNodes,
    result.TotalIterations)
fmt.Printf("Used %d tokens in %s\n", result.TotalTokensUsed, result.Duration)
```

### Configuration Options

```go
// Maximum iterations (default: 100)
WithMaxIterations(n int)

// Total token budget (default: 0 = unlimited)
WithBudget(tokens int)

// Max concurrent node executions (default: 10)
WithMaxConcurrent(n int)

// Overall timeout (default: 0 = no timeout)
WithTimeout(duration time.Duration)

// Structured logger
WithLogger(logger *slog.Logger)

// OpenTelemetry tracer
WithTracer(tracer trace.Tracer)

// Event bus for observability
WithEventBus(bus EventBus)

// Decision log writer (e.g., Langfuse)
WithDecisionLogWriter(writer DecisionLogWriter)
```

## Decision Actions

The LLM can decide to take these actions:

| Action | Description | Requirements |
|--------|-------------|--------------|
| `execute_agent` | Run a workflow node | target_node_id |
| `skip_agent` | Skip a node (not needed) | target_node_id |
| `modify_params` | Change parameters before execution | target_node_id, modifications |
| `retry` | Retry a failed node | target_node_id |
| `spawn_agent` | Create new dynamic node | spawn_config |
| `complete` | Mark mission complete | stop_reason |

## Termination Conditions

The orchestrator stops when any of these occur:

1. **Natural Completion**: No ready or running nodes remaining
2. **Terminal Decision**: LLM chooses `complete` action
3. **Max Iterations**: Configured iteration limit reached
4. **Budget Exceeded**: Token budget exhausted
5. **Timeout**: Overall timeout elapsed
6. **Context Cancelled**: Context cancellation signal
7. **Fatal Error**: Unrecoverable error in observe/think/act

## Result Status

```go
type OrchestratorStatus string

const (
    StatusCompleted         // Mission completed successfully
    StatusFailed           // Fatal error occurred
    StatusMaxIterations    // Hit iteration limit
    StatusTimeout          // Timeout elapsed
    StatusCancelled        // Context cancelled
    StatusBudgetExceeded   // Token budget exhausted
    StatusConcurrencyLimit // Too many concurrent executions
)
```

## Observability

### Events Emitted

The orchestrator emits events for:
- Mission started/completed/failed
- Mission progress updates
- Node started/completed/failed
- Decisions made
- Actions executed

### Logging

Structured logs at each stage:
```
INFO  orchestrator starting mission_id=... max_iterations=100
DEBUG observation complete ready_nodes=5 running_nodes=2
INFO  decision made action=execute_agent target=node-123 confidence=0.95
DEBUG action completed action=execute_agent terminal=false
INFO  terminal action executed iteration=25
```

### Tracing

OpenTelemetry spans track:
- Overall orchestration run
- Each iteration
- Observer, thinker, actor operations
- LLM calls and agent executions

### Decision Logging

External systems (Langfuse) receive:
- Full decision context and reasoning
- Token usage and latency
- Action results and outcomes

## Error Handling

### Recoverable Errors
- Agent execution failures → tracked in graph, orchestration continues
- LLM parse errors → retry with backoff
- Temporary graph query failures → retry

### Non-Recoverable Errors
- Invalid mission ID → immediate failure
- Observer initialization failure → immediate failure
- Context cancellation → clean shutdown
- Fatal graph errors → immediate failure

## Performance Considerations

### Concurrency
- Limit concurrent executions with `WithMaxConcurrent(n)`
- Orchestrator sleeps when limit reached
- Prevents resource exhaustion

### Token Budget
- Track cumulative token usage across iterations
- Stop before exceeding budget
- Helps control LLM costs

### Timeouts
- Overall orchestration timeout
- Individual agent execution timeouts (workflow node level)
- Context-based cancellation

## Testing

See `orchestrator_test.go` for unit tests and `orchestrator_example_test.go` for usage examples.

Key test scenarios:
- Configuration options
- Status transitions
- Invalid inputs
- Context cancellation
- Result formatting

## Future Enhancements

Potential improvements:
- [ ] Dynamic concurrency adjustment based on resource usage
- [ ] Adaptive token budget allocation per iteration
- [ ] Priority-based node scheduling
- [ ] Decision quality metrics and feedback loops
- [ ] Checkpoint/resume for long-running missions
- [ ] Parallel branch execution in workflow DAG
