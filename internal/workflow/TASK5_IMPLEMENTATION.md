# Task 5 Implementation: Add Agent Event Emission to WorkflowExecutor

## Summary
Successfully implemented Task 5 to add agent event emission to the WorkflowExecutor for graph processing.

## Changes Made

### 1. Added EventBusPublisher Interface (executor.go)
```go
// EventBusPublisher is an interface for publishing daemon-wide events.
// This allows the workflow executor to publish agent lifecycle events
// to the daemon's event bus for graph processing.
type EventBusPublisher interface {
	Publish(ctx context.Context, event interface{}) error
}
```

### 2. Added eventBus Field to WorkflowExecutor (executor.go)
```go
type WorkflowExecutor struct {
	guardrails  *guardrail.GuardrailPipeline
	logger      *slog.Logger
	tracer      trace.Tracer
	maxParallel int
	verboseBus  VerboseEventBus    // Optional event bus for verbose logging
	eventBus    EventBusPublisher  // Optional event bus for daemon events (agent lifecycle, etc.)
}
```

### 3. Added WithEventBus Functional Option (executor.go)
```go
// WithEventBus configures the executor to emit agent lifecycle events
// to the daemon's event bus for graph processing (agent.started, agent.completed, agent.failed).
func WithEventBus(bus EventBusPublisher) ExecutorOption {
	return func(e *WorkflowExecutor) {
		e.eventBus = bus
	}
}
```

### 4. Added Agent Event Emission Functions (executor.go)

#### emitAgentStarted
- Emits `agent.started` event when agent execution begins
- Includes: agent_name, task_id, parent_span_id
- Extracts trace context (trace_id, span_id) from OpenTelemetry span
- Used by graph processor to create Agent nodes and PART_OF relationships

#### emitAgentCompleted
- Emits `agent.completed` event when agent completes successfully
- Includes: agent_name, duration, output_summary
- Extracts trace context from OpenTelemetry span
- Used by graph processor to update Agent node properties

#### emitAgentFailed
- Emits `agent.failed` event when agent execution fails
- Includes: agent_name, duration, error message
- Extracts trace context from OpenTelemetry span
- Used by graph processor to record agent failures

### 5. Updated executeAgentNode (node_executor.go)
Modified `executeAgentNode` function to call emit functions at appropriate points:

**Before agent execution:**
- Extract task ID from node.AgentTask
- Extract parent span ID (for PART_OF relationships)
- Call `emitAgentStarted()` with agent_name, task_id, parent_span_id

**After agent execution:**
- Calculate duration
- If error: Call `emitAgentFailed()` with error details
- If success:
  - Extract output summary from agent result
  - Call `emitAgentCompleted()` with duration and output_summary
- If agent result status is failed:
  - Call `emitAgentFailed()` with error details

## Event Data Structure

All events follow the structure defined in `internal/daemon/graph_event.go`:

```go
{
  "type": "agent.started|agent.completed|agent.failed",
  "trace_id": "...",        // From OpenTelemetry span
  "span_id": "...",         // From OpenTelemetry span
  "parent_span_id": "...",  // For PART_OF relationships
  "timestamp": time.Now(),
  "data": {
    "agent_name": "...",
    "task_id": "...",        // For agent.started
    "duration": 1234,        // Milliseconds (for completed/failed)
    "output_summary": "...", // For agent.completed
    "error": "..."           // For agent.failed
  }
}
```

## Integration Points

The WorkflowExecutor can now be configured with an event bus at creation:

```go
executor := workflow.NewWorkflowExecutor(
	workflow.WithEventBus(daemonEventBus),
	// ... other options
)
```

The daemon event bus will receive agent lifecycle events that the graph processor can use to:
1. Create Agent nodes in Neo4j
2. Create PART_OF relationships between agents and missions
3. Track agent execution metrics (duration, status)
4. Record agent failures and errors

## Testing

Created `executor_event_test.go` with tests:
- `TestAgentEventEmission`: Verifies all three event types are emitted correctly
- `TestNoEventBusConfigured`: Verifies execution works when no event bus is configured

Note: Full test execution is blocked by unrelated harness package compilation issues.

## Acceptance Criteria Met

✅ **Subtask 5.1**: Added `eventBus` field to `WorkflowExecutor`
✅ **Subtask 5.2**: Created `emitAgentStarted()` with agent_name, task_id, parent_span_id
✅ **Subtask 5.3**: Created `emitAgentCompleted()` with duration, output_summary
✅ **Subtask 5.4**: Created `emitAgentFailed()` with error details
✅ **Subtask 5.5**: Called emit functions in `executeAgentNode()` at appropriate points

✅ **Agent events are emitted before/after agent execution**
✅ **Events include parent span ID for PART_OF relationship creation**
✅ **Failed agents emit failure event with error details**

## Files Modified

- `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/workflow/executor.go`
  - Added EventBusPublisher interface
  - Added eventBus field
  - Added WithEventBus option
  - Added emitAgentStarted/Completed/Failed functions

- `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/workflow/node_executor.go`
  - Updated executeAgentNode to emit events

## Files Created

- `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/workflow/executor_event_test.go`
  - Tests for agent event emission
