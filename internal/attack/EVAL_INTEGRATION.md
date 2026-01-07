# Attack Runner Eval Integration (Task 12)

This document describes the eval integration in the attack runner, implemented as part of the Gibson Eval Integration spec.

## Overview

The attack runner has been enhanced to support evaluation of agent executions during attack missions. When evaluation is enabled via `--eval-*` CLI flags, the runner:

1. Passes eval options to the mission orchestrator
2. Retrieves eval results after mission completion
3. Outputs eval summary to console
4. Exports detailed results to JSONL if specified

## Implementation Details

### Modified Files

#### `runner.go`

**Added Import:**
```go
import "github.com/zero-day-ai/gibson/internal/eval"
```

**Modified `Run()` Method:**
- Added eval_enabled logging in attack start message
- Added Step 7: Collect and output eval results after mission execution
- Calls `outputEvalResults()` if eval options are enabled

**New Method: `outputEvalResults()`**
```go
func (r *DefaultAttackRunner) outputEvalResults(
    ctx context.Context,
    result *AttackResult,
    evalOpts *eval.EvalOptions,
) error
```

This method:
1. Type asserts orchestrator to `*mission.DefaultMissionOrchestrator` to access eval methods
2. Retrieves eval result collector via `GetEvalResults()`
3. Finalizes evaluation via `FinalizeEvalResults(ctx)`
4. Logs evaluation summary to console:
   - Overall score
   - Total steps
   - Tokens used
   - Warning and critical alert counts
   - Individual scorer scores
5. Exports to JSONL if `OutputPath` is specified

### Integration Flow

```
AttackRunner.Run()
    |
    +-- Validate AttackOptions (includes EvalOptions)
    |
    +-- Execute mission via orchestrator
    |   (orchestrator uses eval-wrapped harness if eval enabled)
    |
    +-- Collect findings
    |
    +-- [NEW] outputEvalResults() if eval enabled
    |   |
    |   +-- Get collector from orchestrator
    |   +-- Finalize eval results
    |   +-- Log summary to console
    |   +-- Export to JSONL if path specified
    |
    +-- Auto-persist mission if needed
    |
    +-- Return AttackResult
```

## Usage

### Enable Evaluation

```bash
gibson attack \
  --agent prompt-injection \
  --target https://example.com \
  --eval \
  --eval-ground-truth /path/to/ground_truth.json \
  --eval-output /path/to/results.jsonl
```

### Console Output

When eval is enabled, the console will show:

```
INFO Attack execution completed duration=5.2s findings=3 status=findings eval_enabled=true
INFO Evaluation Results overall_score=0.850 total_steps=12 tokens_used=5432 warnings=1 critical_alerts=0
INFO Scorer Breakdown
INFO   - exact_match score=0.900
INFO   - semantic_similarity score=0.800
INFO Exporting eval results to JSONL path=/path/to/results.jsonl
INFO Eval results exported successfully path=/path/to/results.jsonl
```

### JSONL Export Format

The exported JSONL file contains:
- `trajectory_step`: High-level step information
- `feedback`: Complete feedback entries with scores
- `alert`: Individual evaluation alerts
- `score`: Final scorer scores
- `summary`: Overall evaluation summary

Each line is a JSON object with:
```json
{
  "type": "feedback|trajectory_step|alert|score|summary",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": { ... }
}
```

## Dependencies

This implementation depends on:
- **Task 7**: Orchestrator Integration (`mission.WithEvalOptions`)
- **Task 11**: CLI eval flags (`AttackOptions.EvalOptions`)
- **Task 8**: JSONL export (`eval.ExportJSONL`)

### Known Issues

As of implementation, Tasks 5-7 have compilation errors in the eval package:
- `types.HARNESS_CREATION_FAILED` is undefined
- Unused imports in `eval_adapter.go`

These issues don't affect the attack runner integration itself, but prevent end-to-end testing until resolved.

## Testing

See `runner_eval_test.go` for test cases:

- `TestRunWithEvalOptions`: Integration test for attack with eval enabled
- `TestOutputEvalResults_NoCollector`: Handles missing eval collector gracefully
- `TestEvalOptionsValidation`: Validates eval options in attack options
- `TestEvalResultsExport`: Tests JSONL export functionality

Some tests are skipped pending Task 5 completion.

## Error Handling

The implementation is designed to be non-breaking:
- If orchestrator doesn't support eval (not a `DefaultMissionOrchestrator`), logs warning and continues
- If eval collector is nil, logs warning and continues
- If eval result finalization fails, logs error but doesn't fail the attack
- If JSONL export fails, logs error but doesn't fail the attack

This ensures eval is optional and won't break existing attack workflows.
