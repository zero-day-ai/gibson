# Tasks 30-32 Implementation Summary

## Overview

This document summarizes the implementation of Tasks 30-32 for the Gibson Eval Integration specification:

- **Task 30**: Evaluation Factory SDK Adapter
- **Task 31**: Planning Feedback Bridge - Replan Request Handling
- **Task 32**: Planning Feedback Bridge - Mission Termination Evaluation

## Task 30: Evaluation Factory SDK Adapter

### File: `opensource/gibson/internal/eval/harness_adapter.go` (NEW)

**Purpose**: Bridge Gibson's internal harness types with SDK evaluation harness types.

**Problem**:
- SDK evaluation harnesses (`RecordingHarness`, `FeedbackHarness`) expect `sdk/agent.Harness` interface
- SDK uses `sdk/llm.Message`, `sdk/llm.CompletionResponse`, etc.
- Gibson harness uses `internal/harness.AgentHarness` interface
- Gibson uses `internal/llm.Message`, `internal/llm.CompletionResponse`, etc.
- Type systems are incompatible - need bidirectional conversion

**Solution**:
Created `GibsonHarnessAdapter` struct that:

1. **Wraps Gibson's `AgentHarness`** - holds reference to internal harness
2. **Implements SDK's `agent.Harness` interface** - all 30+ methods
3. **Converts types bidirectionally**:
   - SDK Messages → Gibson Messages (for LLM calls)
   - Gibson CompletionResponse → SDK CompletionResponse
   - Gibson ToolCall/ToolResult → SDK ToolCall/ToolResult
   - Descriptor types (Tools, Plugins, Agents)
   - Memory store interface adaptation

**Key Methods**:
- `Complete()` - Converts messages, delegates to Gibson harness, converts response back
- `CompleteWithTools()` - Handles tool definitions and tool calls
- `Stream()` - Adapts streaming chunks between type systems
- `CallTool()`, `QueryPlugin()` - Direct delegation (map-based, no conversion needed)
- `ListTools()`, `ListPlugins()`, `ListAgents()` - Converts descriptor types
- `SubmitFinding()`, `GetFindings()` - Finding operations (direct delegation)
- `Memory()` - Wraps Gibson memory store with SDK interface
- GraphRAG methods - Return `ErrNotImplemented` (future integration)

**Type Conversion Helpers**:
```go
convertMessagesToGibson([]llm.Message) []gibsonLLM.Message
convertCompletionOptionsToGibson([]llm.CompletionOption) []gibsonHarness.CompletionOption
convertToolDefsToGibson([]llm.ToolDef) []gibsonLLM.ToolDef
convertCompletionResponseToSDK(*gibsonLLM.CompletionResponse) *llm.CompletionResponse
convertStreamChunkToSDK(gibsonLLM.StreamChunk) llm.StreamChunk
```

**Integration Pattern**:
```
Gibson Harness (internal types)
    ↓
GibsonHarnessAdapter (type conversion)
    ↓
SDK RecordingHarness (trajectory capture)
    ↓
SDK FeedbackHarness (real-time evaluation) [optional]
```

**Note on Import Cycles**:
The adapter is implemented but **not yet integrated in factory.go** due to import cycle constraints:
- `eval` package imports `harness` package (for factory interface)
- `eval` package imports `planning` package (for planning bridge)
- `planning` package imports `harness` package
- This creates cycle: eval → harness, eval → planning → harness

The adapter will be fully integrated when the mission orchestrator wiring is complete in a later task.

---

## Task 31: Planning Feedback Bridge - Replan Request Handling

### File: `opensource/gibson/internal/eval/planning_bridge.go` (MODIFIED)

**Method**: `triggerReplanning(ctx context.Context, score *planning.StepScore)`

**Purpose**: Handle evaluation feedback that indicates replanning is needed.

**Previous State**:
- TODO comment indicating need to emit event
- No actual replanning trigger implementation

**Implementation**:
```go
func (b *PlanningFeedbackBridge) triggerReplanning(ctx context.Context, score *planning.StepScore) {
    if b.planningOrch == nil {
        return
    }

    // Emit ReplanTriggered event to notify mission controller
    b.emitReplanRequestedEvent(ctx, score)

    // Note: Don't call HandleReplan directly - no WorkflowState available
    // Mission controller will handle actual replanning when it receives the event
}
```

**New Helper Method**: `emitReplanRequestedEvent()`
```go
func (b *PlanningFeedbackBridge) emitReplanRequestedEvent(ctx context.Context, score *planning.StepScore) {
    // Create ReplanTriggeredEvent with score details
    event := planning.NewReplanTriggeredEvent(
        score.MissionID,
        score.NodeID,
        score.ReplanReason,
        0, // replan count from controller state
    )

    // TODO: Wire up to actual event emitter when available
}
```

**Design Rationale**:
- Bridge doesn't own `WorkflowState` (mission controller does)
- Bridge emits events rather than mutating state directly
- Mission controller subscribes to planning events
- Controller calls `HandleReplan()` with proper state when event received
- Separation of concerns: bridge detects need, controller executes

**Event Flow**:
```
Eval Feedback (low score)
    ↓
PlanningFeedbackBridge.triggerReplanning()
    ↓
Emit ReplanTriggeredEvent
    ↓
Mission Controller receives event
    ↓
Controller.HandleReplan(WorkflowState)
    ↓
Planning Orchestrator performs replanning
```

---

## Task 32: Planning Feedback Bridge - Mission Termination Evaluation

### File: `opensource/gibson/internal/eval/planning_bridge.go` (MODIFIED)

**Method**: `evaluateMissionTermination(ctx context.Context, alert sdkeval.Alert)`

**Purpose**: Evaluate whether critical alerts warrant graceful mission shutdown.

**Previous State**:
- TODO comment with future implementation notes
- No actual termination logic

**Implementation**:

**Main Method**:
```go
func (b *PlanningFeedbackBridge) evaluateMissionTermination(ctx context.Context, alert sdkeval.Alert) {
    criticalAlertCount := b.countRecentCriticalAlerts()

    shouldTerminate := false
    terminationReason := ""

    // Factor 1: Multiple critical alerts in succession
    if criticalAlertCount >= 3 {
        shouldTerminate = true
        terminationReason = "Multiple critical alerts indicate mission cannot proceed"
    }

    // Factor 2: Explicit abort recommendation
    if alert.Action == sdkeval.ActionAbort && alert.Level == sdkeval.AlertCritical {
        shouldTerminate = true
        terminationReason = fmt.Sprintf("Critical evaluation failure: %s", alert.Message)
    }

    // Factor 3: Score below critical threshold
    if alert.Score < alert.Threshold && alert.Level == sdkeval.AlertCritical {
        shouldTerminate = true
        terminationReason = "Score below critical threshold - mission quality unacceptable"
    }

    if shouldTerminate {
        b.emitMissionTerminationEvent(ctx, terminationReason, alert)
    }
}
```

**Helper Methods**:

1. `countRecentCriticalAlerts() int`
   - Counts critical alerts in recent history
   - Currently returns 1 (placeholder)
   - Full implementation would track alert history with timestamps

2. `emitMissionTerminationEvent(ctx, reason, alert)`
   - Creates `ConstraintViolationEvent` with termination details
   - Includes alert metadata (scorer, score, threshold, message)
   - Mission controller subscribes to these events
   - Controller calls `InitiateGracefulShutdown()` when received

**Termination Criteria**:
1. **Multiple Critical Alerts** (≥3) - Sustained poor performance
2. **Explicit Abort Action** - Evaluation system recommends stopping
3. **Critical Threshold Breach** - Quality unacceptably low

**Design Rationale**:
- Bridge doesn't directly call controller shutdown (no controller reference)
- Bridge emits events indicating termination is warranted
- Mission controller owns shutdown decision and process
- Allows controller to aggregate multiple signals before terminating
- Enables graceful shutdown with proper cleanup

**Event Flow**:
```
Critical Alert (ActionAbort or score < threshold)
    ↓
PlanningFeedbackBridge.evaluateMissionTermination()
    ↓
Emit ConstraintViolationEvent
    ↓
Mission Controller receives event
    ↓
Controller.InitiateGracefulShutdown(reason)
    ↓
Graceful mission termination with cleanup
```

---

## Additional Changes

### File: `opensource/gibson/internal/eval/collector.go` (MODIFIED)

**Added Method**: `RegisterFeedbackHarness()`
```go
func (c *EvalResultCollector) RegisterFeedbackHarness(agentName string, h *eval.FeedbackHarness) {
    c.RegisterHarness(agentName, h)
}
```

**Purpose**: Alias for `RegisterHarness()` to provide clearer semantics when registering feedback harnesses.

---

## Testing Status

**Compilation**: All files compile successfully (verified with `go build -tags fts5`)

**Import Cycle Resolution**: Adapter implementation complete but not integrated in factory.go to avoid import cycle. Will be integrated in mission orchestrator task.

**Unit Tests**: Not yet implemented. Test files should be added for:
- `harness_adapter_test.go` - Test type conversions
- `planning_bridge_test.go` - Test replan/termination logic

**Integration Tests**: Pending mission orchestrator integration.

---

## Future Work

1. **Complete Factory Integration** (Task 30 continuation)
   - Integrate adapter in factory.go once import cycles resolved
   - Wire up SDK RecordingHarness and FeedbackHarness
   - Register wrapped harnesses with collector

2. **Event Emitter Wiring** (Tasks 31-32 continuation)
   - Connect `emitReplanRequestedEvent()` to actual event emitter
   - Connect `emitMissionTerminationEvent()` to actual event emitter
   - Test event flow end-to-end

3. **Alert History Tracking** (Task 32 enhancement)
   - Implement proper `countRecentCriticalAlerts()` with history
   - Track alerts with timestamps and step indices
   - Add sliding window for alert frequency analysis

4. **Test Coverage**
   - Unit tests for adapter type conversions
   - Unit tests for termination criteria evaluation
   - Integration tests for event emission
   - End-to-end tests with mission controller

---

## Files Modified/Created

**Created**:
- `opensource/gibson/internal/eval/harness_adapter.go` - 580 lines

**Modified**:
- `opensource/gibson/internal/eval/factory.go` - Updated with TODO for adapter integration
- `opensource/gibson/internal/eval/planning_bridge.go` - Implemented Tasks 31-32 (3 new methods, ~100 lines)
- `opensource/gibson/internal/eval/collector.go` - Added RegisterFeedbackHarness() alias

**Total Lines Added**: ~680 lines
**Total Lines Modified**: ~50 lines

---

## Architecture Impact

The implementation maintains proper separation of concerns:

1. **Harness Adapter**: Type system bridge (eval package)
2. **Planning Bridge**: Event-based communication (eval package)
3. **Mission Controller**: State owner and decision maker (mission package)
4. **Planning Orchestrator**: Replanning executor (planning package)

No circular dependencies introduced. Event-driven architecture enables loose coupling between evaluation and mission control systems.
