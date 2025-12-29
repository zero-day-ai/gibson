# Prompt Relay and Transformer System

This document describes the prompt relay and transformer system implemented for the Gibson Framework.

## Overview

The relay system enables parent agents to delegate tasks to sub-agents by transforming prompts through a pipeline of transformers. This maintains context while narrowing scope and ensuring immutability.

## Architecture

### Core Components

#### 1. PromptTransformer Interface (`transformer.go`)

```go
type PromptTransformer interface {
    Name() string
    Transform(ctx *RelayContext, prompts []Prompt) ([]Prompt, error)
}
```

Transformers modify prompts during relay operations. Each transformer:
- Has a unique name for logging
- Receives relay context and prompts
- Returns transformed copies (never modifies originals)
- Can fail with descriptive errors

#### 2. RelayContext (`relay.go`)

```go
type RelayContext struct {
    SourceAgent string         // Parent agent name
    TargetAgent string         // Sub-agent name
    Task        string         // Delegated task description
    Prompts     []Prompt       // Original prompts
    Memory      map[string]any // Shared context
    Constraints []string       // Limitations/requirements
}
```

Provides all context needed for transformers to make decisions.

#### 3. PromptRelay (`relay.go`)

```go
type PromptRelay interface {
    Relay(ctx *RelayContext, transformers ...PromptTransformer) ([]Prompt, error)
}
```

The relay coordinator:
- Deep copies input prompts (ensures immutability)
- Applies transformers in sequence
- Each transformer receives output from previous
- Returns final transformed prompts

### Built-in Transformers

#### ContextInjector (`transformers/context.go`)

Adds parent agent context to delegated prompts.

**Features:**
- Creates a context prompt at `system_prefix` position
- Includes source agent, task, and constraints
- Optionally includes memory context
- Supports custom templates

**Default Template:**
```
Delegated by: {SourceAgent}
Task: {Task}
Constraints:
  - {Constraint1}
  - {Constraint2}

Shared Context: {N} items available
```

**Configuration:**
```go
injector := transformers.NewContextInjector()
injector.IncludeMemory = true
injector.IncludeConstraints = true
injector.ContextTemplate = "Custom: {SourceAgent} -> {TargetAgent}"
```

#### ScopeNarrower (`transformers/scope.go`)

Filters prompts to task-relevant content.

**Filtering Criteria:**
- **Position filtering**: Only include specific positions
- **ID exclusion**: Remove prompts by ID
- **Keyword filtering**: Only include prompts containing keywords (case-insensitive)

All filters are combined with AND logic.

**Configuration:**
```go
narrower := transformers.NewScopeNarrower()
narrower.AllowedPositions = []Position{PositionSystem, PositionUser}
narrower.ExcludeIDs = []string{"debug_prompt"}
narrower.KeywordFilter = []string{"security", "authentication"}
```

## Usage Examples

### Basic Context Injection

```go
relay := prompt.NewPromptRelay()
ctx := &prompt.RelayContext{
    SourceAgent: "Coordinator",
    TargetAgent: "Worker",
    Task:        "Process security scan",
    Prompts:     originalPrompts,
}

contextInjector := transformers.NewContextInjector()
result, err := relay.Relay(ctx, contextInjector)
```

### Scope Narrowing

```go
scopeNarrower := transformers.NewScopeNarrower()
scopeNarrower.KeywordFilter = []string{"security"}

result, err := relay.Relay(ctx, scopeNarrower)
// Result contains only prompts with "security" in content
```

### Full Pipeline

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

// Add context
contextInjector := transformers.NewContextInjector()

// Filter to relevant prompts
scopeNarrower := transformers.NewScopeNarrower()
scopeNarrower.AllowedPositions = []Position{
    PositionSystemPrefix,
    PositionSystem,
    PositionUser,
}
scopeNarrower.KeywordFilter = []string{"security", "scan"}

// Apply both transformations
result, err := relay.Relay(ctx, contextInjector, scopeNarrower)
```

## Immutability Guarantee

The relay system guarantees that original prompts are never modified:

1. Input prompts are deep copied before transformation
2. Each transformer creates new prompt slices
3. Original `RelayContext.Prompts` remains unchanged
4. Tests verify immutability at every stage

Example:
```go
original := []Prompt{{ID: "test", Content: "original"}}
ctx := &RelayContext{Prompts: original}

result, _ := relay.Relay(ctx, someTransformer)

// result[0].Content may be "modified"
// original[0].Content is still "original"
```

## Multi-Agent Delegation Patterns

### Sequential Delegation

```go
// Orchestrator -> Analyzer -> Reporter
orchestratorCtx := &RelayContext{
    SourceAgent: "Orchestrator",
    TargetAgent: "Analyzer",
    Task:        "Analyze code",
    Prompts:     originalPrompts,
}

analyzerPrompts, _ := relay.Relay(orchestratorCtx, contextInjector, scopeNarrower)

// Analyzer delegates to Reporter
reporterCtx := &RelayContext{
    SourceAgent: "Analyzer",
    TargetAgent: "Reporter",
    Task:        "Generate report",
    Prompts:     analyzerPrompts,
}

reporterPrompts, _ := relay.Relay(reporterCtx, contextInjector, differentScope)
```

### Parallel Delegation

```go
// Orchestrator delegates to multiple specialists
staticAnalyzerCtx := &RelayContext{
    SourceAgent: "Orchestrator",
    TargetAgent: "StaticAnalyzer",
    Task:        "Static code analysis",
    Prompts:     originalPrompts,
}

dynamicScannerCtx := &RelayContext{
    SourceAgent: "Orchestrator",
    TargetAgent: "DynamicScanner",
    Task:        "Runtime testing",
    Prompts:     originalPrompts,
}

staticPrompts, _ := relay.Relay(staticAnalyzerCtx, contextInjector, staticScope)
dynamicPrompts, _ := relay.Relay(dynamicScannerCtx, contextInjector, dynamicScope)
```

## Custom Transformers

Implement the `PromptTransformer` interface:

```go
type MyTransformer struct{}

func (t *MyTransformer) Name() string {
    return "MyTransformer"
}

func (t *MyTransformer) Transform(ctx *RelayContext, prompts []Prompt) ([]Prompt, error) {
    result := make([]Prompt, 0, len(prompts))

    for _, p := range prompts {
        // Apply custom logic
        modified := p
        modified.Priority += 10
        result = append(result, modified)
    }

    return result, nil
}
```

## Error Handling

All relay and transformer errors use Gibson's typed error system:

```go
result, err := relay.Relay(ctx, transformer)
if err != nil {
    // Error wraps the failing transformer's error
    // Error code: ErrCodeRelayFailed
    // Message includes transformer name
}
```

## Testing

### Unit Tests
- `relay_test.go`: Relay mechanism tests
- `transformer_test.go`: Transformer interface tests
- `transformers/context_test.go`: ContextInjector tests
- `transformers/scope_test.go`: ScopeNarrower tests

### Integration Tests
- `integration_test/relay_integration_test.go`: Full pipeline tests
- Complex multi-agent scenarios
- Immutability verification
- Custom template testing

### Coverage
- `prompt` package: 94.0%
- `transformers` package: 98.4%

## Performance Considerations

1. **Deep Copy**: Uses JSON marshal/unmarshal for simplicity
   - Consider custom deep copy for performance-critical paths
   - Currently acceptable for prompt transformation use case

2. **Transformer Chain**: O(n*t) where n=prompts, t=transformers
   - Each transformer processes all prompts
   - Filters reduce prompt count for subsequent transformers

3. **Memory**: Each relay creates new prompt slices
   - Ensures immutability
   - Memory usage proportional to prompt count

## Future Enhancements

Potential improvements:

1. **Template Engine**: Replace simple string replacement with full template engine
2. **Async Transformers**: Support async/concurrent transformation
3. **Transformer Middleware**: Pre/post hooks for transformers
4. **Caching**: Cache transformed prompts for identical contexts
5. **Metrics**: Track transformer performance and prompt changes
6. **Validation**: Schema validation for relay context
7. **Composition**: Combine transformers with AND/OR logic

## Files

```
internal/prompt/
├── transformer.go              # PromptTransformer interface
├── relay.go                    # PromptRelay implementation
├── relay_test.go               # Relay unit tests
├── transformer_test.go         # Transformer interface tests
├── transformers/
│   ├── context.go              # ContextInjector implementation
│   ├── context_test.go         # ContextInjector tests
│   ├── scope.go                # ScopeNarrower implementation
│   └── scope_test.go           # ScopeNarrower tests
└── integration_test/
    └── relay_integration_test.go  # Full pipeline integration tests
```

## Summary

The relay and transformer system provides:

- **Immutable transformation**: Original prompts never modified
- **Composable pipeline**: Chain transformers for complex behavior
- **Context preservation**: Maintain delegation context across agents
- **Scope management**: Filter prompts to relevant content
- **Extensibility**: Easy to add custom transformers
- **Type safety**: Full Go type checking and error handling
- **Well tested**: Comprehensive unit and integration tests
