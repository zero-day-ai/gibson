# Task 10: Planning Orchestrator Eval Integration - Implementation Summary

## Overview

This task implements the eval feedback integration into the planning orchestrator, enabling real-time eval scores to influence planning decisions and trigger tactical replanning when agent performance is poor.

## Files Created

### 1. `orchestrator_eval.go`
**Purpose**: Integration layer between eval system and planning orchestrator

**Key Components**:

#### EvalFeedbackBridge Interface
```go
type EvalFeedbackBridge interface {
    GetLastFeedback(nodeID string) *EvalFeedback
    GetAggregateScore() float64
    ShouldTriggerReplan(nodeID string) (bool, string)
}
```
- Defines the contract for eval.PlanningFeedbackBridge (Task 9)
- Provides access to real-time eval feedback during mission execution

#### EvalFeedback Type
```go
type EvalFeedback struct {
    NodeID         string
    Score          float64
    ScorerScores   map[string]float64
    Recommendation string
    Reasoning      string
    IsAlert        bool
    AlertLevel     string  // "warning" or "critical"
}
```
- Simplified representation of eval feedback for a single step
- Contains score, alerts, and recommendations

#### WithEvalFeedback Option
```go
func WithEvalFeedback(bridge EvalFeedbackBridge) PlanningOrchestratorOption
```
- Configuration option to enable eval integration
- Sets the evalBridge field on the orchestrator

#### getEvalAdjustedScore Method
```go
func (o *PlanningOrchestrator) getEvalAdjustedScore(
    ctx context.Context,
    nodeID string,
    originalScore *StepScore,
) (*StepScore, bool)
```
- Adjusts step scores based on eval feedback
- **Very low scores (<0.3)**: Reduce confidence by 50%, trigger replanning
- **Low scores (0.3-0.5)**: Reduce confidence by 30%, no auto-replan
- **High scores (>0.7)**: Increase confidence by 10% (capped at 1.0)
- **Critical alerts**: Override success to false, trigger replanning
- Returns adjusted score and whether eval was applied

#### getEvalFeedbackContext Method
```go
func (o *PlanningOrchestrator) getEvalFeedbackContext(nodeID string) string
```
- Formats eval feedback into context string for replanning
- Includes overall score, recommendation, reasoning, scorer breakdown, alerts
- Used to inform tactical replanner's decision-making

### 2. `orchestrator_eval_test.go`
**Purpose**: Comprehensive test suite for eval integration

**Test Coverage**:
- ✅ `TestWithEvalFeedback` - Verify option sets bridge correctly
- ✅ `TestGetEvalAdjustedScore_NoEvalBridge` - Handle missing bridge gracefully
- ✅ `TestGetEvalAdjustedScore_NoFeedback` - Handle missing feedback gracefully
- ✅ `TestGetEvalAdjustedScore_VeryLowScore` - Verify replan trigger at <0.3
- ✅ `TestGetEvalAdjustedScore_LowScore` - Verify confidence reduction at 0.3-0.5
- ✅ `TestGetEvalAdjustedScore_HighScore` - Verify confidence boost at >0.7
- ✅ `TestGetEvalAdjustedScore_CriticalAlert` - Verify critical alerts override success
- ✅ `TestGetEvalAdjustedScore_BridgeRecommendReplan` - Respect bridge recommendations
- ✅ `TestGetEvalFeedbackContext_NoEvalBridge` - Handle missing bridge
- ✅ `TestGetEvalFeedbackContext_NoFeedback` - Handle missing feedback
- ✅ `TestGetEvalFeedbackContext_WithFeedback` - Format context correctly
- ✅ `TestOnStepComplete_WithEvalIntegration` - Integration test for scoring
- ✅ `TestHandleReplan_WithEvalContext` - Integration test for replanning

**All tests passing** ✅

## Files Modified

### 1. `orchestrator.go`
**Changes**:
1. Added `evalBridge EvalFeedbackBridge` field to `PlanningOrchestrator` struct
2. Modified `OnStepComplete` to apply eval score adjustments after scoring
3. Modified `HandleReplan` to include eval context in replan input

**Code Additions**:
```go
// In PlanningOrchestrator struct
evalBridge EvalFeedbackBridge // optional, nil when eval disabled

// In OnStepComplete
adjustedScore, evalApplied := o.getEvalAdjustedScore(ctx, nodeID, score)
if evalApplied {
    score = adjustedScore
}

// In HandleReplan
evalContext := o.getEvalFeedbackContext(triggerScore.NodeID)
replanInput.EvalContext = evalContext
```

### 2. `llm_tactical_replanner.go`
**Changes**:
1. Added `EvalContext string` field to `ReplanInput` struct
2. Modified `buildReplanPrompt` to include eval context section

**Code Additions**:
```go
// In ReplanInput struct
EvalContext string  // Optional eval feedback context

// In buildReplanPrompt
if input.EvalContext != "" {
    b.WriteString("## Eval Feedback Context\n\n")
    b.WriteString(input.EvalContext)
    b.WriteString("\n")
}
```

## Integration Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     Mission Execution                            │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│           Agent Executes Step (with EvalHarness)                 │
│  - Wrapped with RecordingHarness (captures trajectory)           │
│  - Wrapped with FeedbackHarness (scores in real-time)            │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ├──────────────────────────────────┐
                          │                                  │
                          ▼                                  ▼
         ┌────────────────────────────┐    ┌────────────────────────────┐
         │  OnStepComplete (Planning)  │    │  OnFeedback (Eval Bridge)   │
         │  1. Deterministic/LLM score │    │  - Stores latest feedback   │
         │  2. getEvalAdjustedScore    │◄───┤  - Checks thresholds        │
         │  3. Returns adjusted score  │    │  - Decides if replan needed │
         └────────────┬───────────────┘    └────────────────────────────┘
                      │
                      │ if ShouldReplan
                      ▼
         ┌────────────────────────────┐
         │   HandleReplan              │
         │  1. getEvalFeedbackContext  │
         │  2. Build ReplanInput       │
         │  3. Call TacticalReplanner  │
         │     with eval context       │
         └─────────────────────────────┘
```

## Eval Score Influence on Planning

### Confidence Adjustment
| Eval Score | Adjustment | Auto-Replan |
|------------|-----------|-------------|
| < 0.3      | -50%      | ✅ Yes      |
| 0.3 - 0.5  | -30%      | ❌ No       |
| 0.5 - 0.7  | None      | ❌ No       |
| > 0.7      | +10%      | ❌ No       |

### Alert Handling
- **Warning alerts**: Logged but don't override decisions
- **Critical alerts**: Override success=false, trigger replan immediately

### Replanning Context
When eval scores trigger replanning, the tactical replanner receives:
```
## Eval Feedback Context

Eval Feedback for failed_node:
  Overall Score: 0.20
  Recommendation: reconsider
  Reasoning: Agent is hallucinating tool outputs
  Individual Scorer Scores:
    - trajectory_quality: 0.15
    - tool_accuracy: 0.25
    - goal_alignment: 0.20
  ALERT: critical level
```

This context helps the LLM replanner make better decisions about whether to:
- **Continue**: Proceed despite low score (eval may be wrong)
- **Reorder**: Try different steps first
- **Skip**: This step won't help
- **Retry**: Try different approach for same step
- **Fail**: Abort mission entirely

## Dependencies

### Required (Task 9)
- `eval.PlanningFeedbackBridge` - Must implement `EvalFeedbackBridge` interface
- Bridge connects eval system events to planning orchestrator
- Stores latest feedback per node
- Determines when to trigger replanning

### Optional
- Eval system can be disabled - orchestrator works normally without it
- Bridge can be nil - all eval methods handle this gracefully

## Testing Strategy

### Unit Tests
- Test each adjustment rule independently
- Verify graceful handling when eval disabled
- Verify correct context formatting

### Integration Tests
- Test full OnStepComplete flow with eval
- Test full HandleReplan flow with eval context
- Verify replanner receives correct eval context

### Mock Bridge
- Simple mock implementation for testing
- Allows setting feedback programmatically
- Captures all interface calls for verification

## Usage Example

```go
// Create eval bridge (Task 9)
evalBridge := eval.NewPlanningFeedbackBridge(planningOrch)

// Create orchestrator with eval enabled
orch := NewPlanningOrchestrator(
    WithScorer(scorer),
    WithReplanner(replanner),
    WithEvalFeedback(evalBridge),  // Enable eval integration
)

// During mission execution:
// 1. Agent executes with eval harness
// 2. Eval system scores trajectory in real-time
// 3. OnStepComplete gets both planning score and eval score
// 4. Eval score adjusts confidence and replan decision
// 5. If replanning triggered, eval context guides decision
```

## Design Decisions

### Why Adjust Confidence Rather Than Override?
- Planning scorer (deterministic/LLM) has domain-specific heuristics
- Eval scorer focuses on trajectory quality and goal alignment
- Combining both provides best overall assessment
- Critical alerts can still override when necessary

### Why Pass Context String to Replanner?
- LLM replanner needs human-readable explanation
- Eval feedback reasoning helps LLM understand the problem
- Structured context format ensures consistency
- Avoids coupling replanner to eval types

### Why Optional Integration?
- Eval mode may not be enabled for all missions
- Allows planning system to work standalone
- Graceful degradation when eval unavailable
- No performance penalty when disabled

## Performance Considerations

- Eval bridge lookups are O(1) (map-based)
- Score adjustment is O(1) arithmetic
- Context formatting is O(n) in scorer count (typically 3-5)
- No additional LLM calls (reuses existing replanner call)
- Minimal memory overhead (one eval feedback per node)

## Future Enhancements

### Adaptive Thresholds
- Learn optimal thresholds from mission outcomes
- Adjust based on agent type and mission complexity
- Per-scorer weight tuning

### Score History
- Track eval score trends over time
- Trigger replan on degrading scores (not just absolute)
- Detect oscillation patterns

### Feedback Loop Metrics
- Measure replan success rate after eval-triggered replans
- Compare eval-influenced vs non-eval-influenced outcomes
- Quantify eval system value

## Conclusion

Task 10 successfully integrates eval feedback into the planning orchestrator, enabling:
1. ✅ Real-time eval scores influence step scoring
2. ✅ Low eval scores trigger tactical replanning
3. ✅ Eval context guides replanner decisions
4. ✅ Critical alerts override success assessment
5. ✅ Graceful handling when eval disabled
6. ✅ Comprehensive test coverage

The implementation is minimal, non-invasive, and maintains backward compatibility while providing powerful new capabilities when eval mode is enabled.
