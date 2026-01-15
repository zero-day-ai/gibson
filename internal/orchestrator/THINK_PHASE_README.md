# Gibson Orchestrator Think Phase

## Overview

The Think phase is the decision-making core of Gibson's orchestrator, implementing an observe-think-act loop for intelligent workflow execution. It takes the current mission state (from the Observe phase) and uses an LLM to reason about what action to take next.

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Observe   │────▶│    Think    │────▶│     Act     │
│   (State)   │     │ (Decision)  │     │  (Execute)  │
└─────────────┘     └─────────────┘     └─────────────┘
                          │
                          ▼
                    ┌──────────┐
                    │   LLM    │
                    │ Reasoning│
                    └──────────┘
```

## Key Components

### 1. Thinker (`think.go`)

The main orchestrator decision-making component that:
- Takes `ObservationState` as input
- Builds comprehensive prompts for the LLM
- Calls LLM with structured output schema
- Parses and validates LLM responses
- Handles retries on parse failures
- Returns `ThinkResult` with decision and metadata

### 2. LLMClient Interface

```go
type LLMClient interface {
    Complete(ctx context.Context, slot string, messages []llm.Message,
             opts ...CompletionOption) (*llm.CompletionResponse, error)

    CompleteStructuredAny(ctx context.Context, slot string, messages []llm.Message,
                         schemaType any, opts ...CompletionOption) (any, error)
}
```

Designed to match the SDK harness interface for seamless integration.

### 3. Decision Types (`decision.go`)

Six orchestration actions available:
- **execute_agent**: Run a workflow node
- **skip_agent**: Skip a node that's not needed
- **modify_params**: Adjust node parameters
- **retry**: Retry a failed node
- **spawn_agent**: Dynamically create new node
- **complete**: End the workflow

### 4. ObservationState (`observe.go`)

Comprehensive mission context including:
- Mission metadata (ID, objective, status)
- Graph statistics (nodes, executions, decisions)
- Node states (ready, running, completed, failed)
- Recent decisions for context
- Resource constraints (budgets, parallelism)
- Failed execution details (for retry decisions)

### 5. Prompts (`prompts.go`)

Centralized prompt management:
- System prompt defining orchestrator role
- Observation state formatting
- Decision schema with validation
- Example decisions for guidance

## Usage

### Basic Usage

```go
import (
    "context"
    "github.com/zero-day-ai/gibson/internal/orchestrator"
)

// Create thinker with LLM client
thinker := orchestrator.NewThinker(
    llmClient,
    orchestrator.WithMaxRetries(3),
    orchestrator.WithThinkerTemperature(0.2),
)

// Get observation state (from Observer)
state, err := observer.Observe(ctx, missionID)

// Make decision
result, err := thinker.Think(ctx, state)
if err != nil {
    // Handle error
}

// Act on decision
switch result.Decision.Action {
case orchestrator.ActionExecuteAgent:
    // Execute the target node
    executeNode(result.Decision.TargetNodeID)
case orchestrator.ActionComplete:
    // Mission complete
    finalizeMission(result.Decision.StopReason)
// ... handle other actions
}
```

### Advanced Configuration

```go
thinker := orchestrator.NewThinker(
    llmClient,
    // Increase retries for unreliable LLM responses
    orchestrator.WithMaxRetries(5),

    // Higher temperature for more creative decisions
    orchestrator.WithThinkerTemperature(0.5),

    // Specific model (if supported by slot)
    orchestrator.WithModel("claude-3-opus"),
)
```

### Handling Different Decision Types

```go
result, err := thinker.Think(ctx, state)
if err != nil {
    return err
}

switch result.Decision.Action {
case orchestrator.ActionExecuteAgent:
    // Execute specified node
    err = executor.Execute(ctx, result.Decision.TargetNodeID)

case orchestrator.ActionSkipAgent:
    // Mark node as skipped
    err = executor.Skip(ctx, result.Decision.TargetNodeID)

case orchestrator.ActionModifyParams:
    // Apply parameter modifications
    err = executor.Modify(ctx,
        result.Decision.TargetNodeID,
        result.Decision.Modifications)

case orchestrator.ActionRetry:
    // Retry failed node
    err = executor.Retry(ctx, result.Decision.TargetNodeID)

case orchestrator.ActionSpawnAgent:
    // Create dynamic node
    err = executor.Spawn(ctx, result.Decision.SpawnConfig)

case orchestrator.ActionComplete:
    // End orchestration
    return finalizeMission(result.Decision.StopReason)
}
```

## Decision Schema

The LLM returns structured JSON matching this schema:

```json
{
  "reasoning": "string (chain-of-thought explanation)",
  "action": "string (one of: execute_agent, skip_agent, modify_params, retry, spawn_agent, complete)",
  "target_node_id": "string (required for most actions)",
  "modifications": {
    "param_name": "value"
  },
  "spawn_config": {
    "agent_name": "string",
    "description": "string",
    "task_config": {},
    "depends_on": []
  },
  "confidence": 0.85,
  "stop_reason": "string (required for complete)"
}
```

## Retry Logic

The Thinker implements intelligent retry behavior:

1. **Retryable Errors**: Parse failures, JSON unmarshal errors, invalid format
2. **Non-Retryable Errors**: API failures, context cancellation, budget exhaustion
3. **Exponential Backoff**: Brief delays between retries (100ms, 200ms, 400ms, etc.)
4. **Retry Context**: Subsequent prompts inform LLM of previous failure

```go
// Configure retry behavior
thinker := orchestrator.NewThinker(
    llmClient,
    orchestrator.WithMaxRetries(3), // 3 retries = 4 total attempts
)

result, err := thinker.Think(ctx, state)
if err != nil {
    // All retries exhausted
    log.Printf("Think failed after %d attempts: %v",
               thinker.maxRetries+1, err)
}

// Check how many retries were needed
if result.RetryCount > 0 {
    log.Printf("Decision succeeded after %d retries", result.RetryCount)
}
```

## Structured Output

The implementation supports two LLM completion modes:

### 1. Structured Output (Preferred)

Uses provider-native structured output mechanisms:
- Anthropic: `tool_use` pattern with forced tool choice
- OpenAI: `response_format` with JSON schema
- Guarantees valid JSON matching the schema

```go
// Automatically tries structured output first
decision, response, err := t.tryStructuredOutput(ctx, messages, opts)
```

### 2. Text Output (Fallback)

Traditional text completion with JSON parsing:
- LLM returns free-form text
- Extracts JSON from markdown code blocks
- Parses and validates against schema
- More error-prone, hence retry logic

```go
// Falls back to text parsing if structured fails
decision, response, err := t.tryTextOutput(ctx, messages, opts)
```

## Prompt Engineering

The Think phase uses carefully crafted prompts:

### System Prompt
- Defines orchestrator role and capabilities
- Explains available actions and when to use them
- Provides confidence scoring guidance
- Emphasizes chain-of-thought reasoning

### Observation Prompt
- Current mission context (goal, status, progress)
- Workflow state (ready, running, completed, failed nodes)
- Recent decisions for continuity
- Resource constraints (time, budget, parallelism)
- Failed execution details (if applicable)

### Decision Schema
- JSON schema with validation constraints
- Field descriptions and requirements
- Conditional validation rules
- Example decisions

### Token Management

Prompts are designed to be concise:
- Estimated 1.5-2.5k tokens for typical state
- System prompt: ~500 tokens
- Observation state: ~1-2k tokens
- Schema and examples: ~500 tokens
- Leaves room for 2k token response

## Performance Characteristics

### Token Usage
- Prompt: 1,500 - 2,500 tokens (depends on state complexity)
- Completion: 150 - 400 tokens (varies by decision complexity)
- Total: 1,650 - 2,900 tokens per decision

### Latency
- Structured output: 2-5 seconds (typical)
- Text parsing: 3-6 seconds (typical)
- Retries add: 100ms + (2^retry * 100ms) backoff

### Cost
- Claude 3 Opus: ~$0.05 per decision
- Claude 3 Sonnet: ~$0.01 per decision
- GPT-4: ~$0.03 per decision

## Error Handling

### Parse Errors
```go
// Retryable - LLM might fix on retry
&parseError{msg: "failed to parse decision JSON"}
```

### Validation Errors
```go
// Retryable - LLM can provide valid decision
fmt.Errorf("invalid decision: missing required field")
```

### API Errors
```go
// Non-retryable - infrastructure issue
fmt.Errorf("LLM API error: rate limit exceeded")
```

### Context Cancellation
```go
// Non-retryable - external timeout
if ctx.Err() != nil {
    return fmt.Errorf("context cancelled: %w", ctx.Err())
}
```

## Testing

Comprehensive test coverage includes:

### Unit Tests (`think_test.go`)
- Thinker construction with options
- Successful decision making
- Retry behavior on parse failures
- Error handling (nil state, LLM failures, context cancellation)
- Decision validation
- Prompt building
- Schema generation

### Example Tests (`think_example_test.go`)
- Basic usage patterns
- Retry configuration
- Different decision types
- Integration with observation state

### Benchmarks
```bash
go test -bench=BenchmarkThinker_Think
```

## Integration Points

### With Observe Phase
```go
// Observer provides state
state, err := observer.Observe(ctx, missionID)

// Thinker reasons over state
result, err := thinker.Think(ctx, state)
```

### With Act Phase
```go
// Act executes decision
err := actor.Act(ctx, result.Decision)
```

### With Graph Database
- Decisions are recorded in graph
- Observation queries execution history
- Think reasons over accumulated context

## Best Practices

### 1. Temperature Selection
- **0.0-0.3**: Deterministic, conservative (production recommended)
- **0.3-0.5**: Balanced creativity and consistency
- **0.5-1.0**: Creative but less predictable

### 2. Retry Configuration
- **Production**: 3-5 retries (handle transient LLM issues)
- **Development**: 1-2 retries (fail fast for debugging)
- **Testing**: 0 retries (deterministic behavior)

### 3. Prompt Optimization
- Keep observation state concise (< 2k tokens)
- Include only relevant context
- Summarize large findings lists
- Truncate long error messages

### 4. Decision Validation
- Always validate decisions before acting
- Check confidence scores (< 0.5 might indicate uncertainty)
- Log reasoning for audit trail
- Track decision patterns for optimization

### 5. Error Recovery
- Log full error context for debugging
- Track retry patterns (frequent retries = prompt issues)
- Monitor token usage trends
- Implement fallback logic for critical paths

## Monitoring & Observability

### Metrics to Track
- Decision latency (p50, p95, p99)
- Token usage per decision
- Retry rate and retry count distribution
- Decision type distribution
- Confidence score distribution
- Error rates by type

### Logging
```go
result, err := thinker.Think(ctx, state)
if err != nil {
    logger.Error("think failed",
        "mission_id", state.MissionInfo.ID,
        "error", err,
        "elapsed", time.Since(start))
    return err
}

logger.Info("decision made",
    "action", result.Decision.Action,
    "target", result.Decision.TargetNodeID,
    "confidence", result.Decision.Confidence,
    "tokens", result.TotalTokens,
    "latency", result.Latency,
    "retries", result.RetryCount)
```

## Future Enhancements

### Planned Features
1. **Decision Caching**: Cache decisions for identical states
2. **Multi-Model Voting**: Consensus from multiple LLMs
3. **Confidence Calibration**: Learn optimal confidence thresholds
4. **Prompt Optimization**: A/B test different prompt structures
5. **Budget-Aware Decisions**: Dynamically adjust based on remaining budget
6. **Learning from Outcomes**: Incorporate decision success rates

### Research Directions
1. **Few-Shot Learning**: Include past successful decisions as examples
2. **Chain-of-Thought Validation**: Validate reasoning quality
3. **Uncertainty Quantification**: Better confidence estimation
4. **Multi-Step Planning**: Look-ahead decision sequences
5. **Adversarial Robustness**: Handle edge cases and malformed states

## References

- [Orchestrator Design Doc](./doc.go)
- [Decision Types](./decision.go)
- [Observation Phase](./observe.go)
- [Action Phase](./act.go)
- [Prompt Engineering](./prompts.go)
