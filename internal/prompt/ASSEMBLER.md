# Prompt Assembler

The Prompt Assembler is responsible for collecting, filtering, rendering, and composing prompts into the final format required by LLM APIs.

## Overview

The assembler takes prompts from a registry, applies conditions and templates, and produces structured messages ready for LLM consumption. It supports:

- **Position-based ordering**: Prompts are assembled in a specific sequence based on their position
- **Priority sorting**: Within each position, prompts are sorted by priority
- **Conditional inclusion**: Prompts can be conditionally included based on runtime context
- **Template rendering**: Prompts can contain Go templates that are rendered with context data
- **Optional sections**: Tools, plugins, and agents can be included or excluded
- **System override**: Ability to replace all system prompts with custom content
- **Persona support**: Special handling for persona prompts

## Core Types

### AssembleResult

Contains the assembled prompts in multiple formats:

```go
type AssembleResult struct {
    System   string    // Combined system message
    User     string    // Combined user message
    Messages []Message // Structured message array
    Prompts  []Prompt  // All prompts used (for debugging)
}
```

### Message

Represents a single chat message:

```go
type Message struct {
    Role    string // "system", "user", "assistant"
    Content string // The message content
}
```

### AssembleOptions

Configures the assembly process:

```go
type AssembleOptions struct {
    Registry       PromptRegistry  // Source for prompts
    RenderContext  *RenderContext  // Data for templates
    IncludeTools   bool            // Include tool prompts
    IncludePlugins bool            // Include plugin prompts
    IncludeAgents  bool            // Include sub-agent prompts
    SystemOverride string          // Replace system prompts
    Persona        string          // Persona ID to include
    ExtraPrompts   []Prompt        // Additional prompts
}
```

## Assembly Algorithm

The assembler follows this process:

1. **Collect prompts**: Load all prompts from the registry into a map (deduplicated by ID)
2. **Add extras**: Add `ExtraPrompts` (overrides prompts with same ID)
3. **Persona handling**: If `Persona` is set, ensure prompt with ID `"persona:{Persona}"` is included
4. **Filter and render**: For each prompt:
   - Evaluate all conditions (skip if any fails)
   - Render template with `RenderContext`
5. **Group by position**: Group prompts by their position and sort each group by priority (descending)
6. **System override**: If `SystemOverride` is set, use it instead of assembling system positions
7. **Concatenate**: Join prompts within each position with `"\n\n"` separator:
   - **System**: `system_prefix + system + system_suffix + context + tools* + plugins* + agents* + constraints + examples`
     - `*` = only if corresponding Include flag is true
   - **User**: `user_prefix + user + user_suffix`
8. **Build messages**: Create structured message array (empty messages excluded)

## Position Order

Prompts are assembled in this order:

### System Positions (in order)
1. `system_prefix` - Highest-level instructions
2. `system` - Main system prompt
3. `system_suffix` - Additional system context
4. `context` - Runtime context information
5. `tools` - Available tools (if `IncludeTools = true`)
6. `plugins` - Active plugins (if `IncludePlugins = true`)
7. `agents` - Sub-agents (if `IncludeAgents = true`)
8. `constraints` - Operational constraints
9. `examples` - Few-shot examples

### User Positions (in order)
10. `user_prefix` - User message prefix
11. `user` - Main user message
12. `user_suffix` - User message suffix

## Priority Sorting

Within each position, prompts are sorted by priority:
- Higher priority values appear **first**
- Default priority is `0`
- Use priorities like: `100` (high), `50` (medium), `10` (low)

Example:
```go
prompts := []Prompt{
    {ID: "low", Position: PositionSystem, Content: "LOW", Priority: 10},
    {ID: "high", Position: PositionSystem, Content: "HIGH", Priority: 100},
    {ID: "medium", Position: PositionSystem, Content: "MEDIUM", Priority: 50},
}
// Result: "HIGH\n\nMEDIUM\n\nLOW"
```

## Condition Filtering

Prompts can include conditions that must be satisfied for inclusion:

```go
prompt := Prompt{
    ID:       "debug-mode",
    Position: PositionContext,
    Content:  "DEBUG: Verbose logging enabled",
    Conditions: []Condition{
        {Field: "agent.debug", Operator: "eq", Value: true},
    },
}
```

If any condition evaluates to `false`, the prompt is skipped.

## Template Rendering

Prompts can use Go templates with access to the render context:

```go
prompt := Prompt{
    ID:       "mission",
    Position: PositionSystem,
    Content:  "Your mission: {{.Mission.objective}}",
}

ctx := NewRenderContext()
ctx.Mission["objective"] = "security assessment"

// Renders to: "Your mission: security assessment"
```

## Usage Examples

### Basic Assembly

```go
registry := NewPromptRegistry()
renderer := NewTemplateRenderer()
assembler := NewAssembler(renderer)

// Register prompts
registry.Register(Prompt{
    ID:       "sys",
    Position: PositionSystem,
    Content:  "You are a helpful assistant.",
})

registry.Register(Prompt{
    ID:       "usr",
    Position: PositionUser,
    Content:  "Hello!",
})

// Assemble
result, err := assembler.Assemble(context.Background(), AssembleOptions{
    Registry: registry,
})

// Result:
// System: "You are a helpful assistant."
// User: "Hello!"
// Messages: [
//   {Role: "system", Content: "You are a helpful assistant."},
//   {Role: "user", Content: "Hello!"}
// ]
```

### With Context and Conditions

```go
ctx := NewRenderContext()
ctx.Agent["debug"] = true
ctx.Mission["objective"] = "test the system"

result, err := assembler.Assemble(context.Background(), AssembleOptions{
    Registry:      registry,
    RenderContext: ctx,
    IncludeTools:  true,
})
```

### With System Override

```go
result, err := assembler.Assemble(context.Background(), AssembleOptions{
    Registry:       registry,
    SystemOverride: "Custom system prompt replacing all registered ones",
})
```

### With Persona

```go
// Register persona prompt
registry.Register(Prompt{
    ID:       "persona:hacker",
    Position: PositionSystem,
    Content:  "You are an expert hacker.",
    Priority: 150,
})

// Use persona
result, err := assembler.Assemble(context.Background(), AssembleOptions{
    Registry: registry,
    Persona:  "hacker",
})
```

### With Extra Prompts

```go
extraPrompts := []Prompt{
    {ID: "tmp", Position: PositionUser, Content: "Temporary instruction"},
}

result, err := assembler.Assemble(context.Background(), AssembleOptions{
    Registry:     registry,
    ExtraPrompts: extraPrompts,
})
```

## Best Practices

1. **Use consistent priorities**: Establish a priority scheme (e.g., 100/50/10) and stick to it
2. **Leverage positions**: Place prompts in appropriate positions for proper ordering
3. **Test conditions**: Ensure condition logic is correct by testing with various context values
4. **Template validation**: Test templates with sample data to catch rendering errors early
5. **Avoid duplication**: Use unique prompt IDs to prevent duplicate content
6. **Document personas**: Clearly document available personas and their purposes
7. **Monitor prompt count**: Keep track of total prompts to manage token usage

## Error Handling

The assembler can return errors for:
- Template rendering failures
- Condition evaluation errors (invalid operators, type mismatches)

Always check the error return value:

```go
result, err := assembler.Assemble(ctx, opts)
if err != nil {
    // Handle error (likely a template or condition issue)
    log.Printf("Assembly failed: %v", err)
    return err
}
```

## Performance Considerations

- **Template caching**: The renderer caches compiled templates for performance
- **Condition evaluation**: Keep conditions simple for fast evaluation
- **Registry size**: The assembler processes all prompts; keep registry focused
- **Priority sorting**: Sorting is O(n log n) per position group

## Integration

The assembler is designed to integrate with:
- **PromptRegistry**: Source of prompts
- **TemplateRenderer**: Template processing
- **RenderContext**: Runtime data for templates
- **LLM APIs**: Output format compatible with chat-based APIs

## See Also

- [Prompt Registry](registry.go) - Prompt storage and retrieval
- [Template Renderer](template.go) - Template processing
- [Conditions](condition.go) - Conditional logic
- [Positions](position.go) - Position ordering
