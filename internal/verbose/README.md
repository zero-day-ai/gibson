# Verbose Logging System

Production-ready verbose logging infrastructure for Gibson's attack execution and agent orchestration.

## Quick Start

```go
import "github.com/zero-day-ai/gibson/internal/verbose"

// Create a text-formatted writer (stderr, verbose level)
writer := verbose.NewVerboseWriter(os.Stderr, verbose.LevelVerbose, false)

// Start processing events
ctx := context.Background()
writer.Start(ctx)
defer writer.Stop()

// Get the event bus for emitting
bus := writer.Bus()

// Emit an event
event := verbose.NewVerboseEvent(
    verbose.EventLLMRequestStarted,
    verbose.LevelVerbose,
    &verbose.LLMRequestStartedData{
        Provider: "anthropic",
        Model:    "claude-opus-4",
        SlotName: "primary",
        MessageCount: 5,
    },
)
bus.Emit(ctx, event)
```

## Verbose Levels

| Level | Value | Flag | Description |
|-------|-------|------|-------------|
| None | 0 | (none) | No verbose output |
| Verbose | 1 | `-v` or `--verbose` | Major operations: LLM requests, tool calls, agent lifecycle |
| VeryVerbose | 2 | `-vv` or `--very-verbose` | Adds detailed operation data: token counts, durations, parameters |
| Debug | 3 | `-vvv` or `--debug-verbose` | All internal events: memory operations, component health, system events |

**Filtering**: Events are shown if `event.Level <= writer.level`
- `-v` shows only level 1 events
- `-vv` shows levels 1 and 2
- `-vvv` shows all levels

**Choosing the Right Level**:
- Use `-v` for monitoring high-level progress during normal operations
- Use `-vv` when you need to understand operation costs (tokens, time) or troubleshoot behavior
- Use `-vvv` when debugging system issues or developing new components

## Output Formats

### Text (Default)

Human-readable colored output:

```
12:34:56.789 [LLM] Request started: anthropic/claude-opus-4 (slot=primary, mode=completion)
└─ max_tokens=100000
└─ temperature=0.70

12:34:58.123 [TOOL] Tool call completed: nmap (success, duration=2s, result=1024 bytes)
```

**Color Scheme**:
- LLM: Cyan
- TOOL: Green (success) / Red (failure)
- AGENT: Magenta
- MISSION: Blue
- MEMORY: Gray
- SYSTEM: Yellow

Colors auto-disable when:
- Output is redirected to a file
- `NO_COLOR` environment variable is set
- stdout is not a TTY

### JSON

Machine-parsable single-line JSON:

```json
{"type":"llm.request.started","level":1,"timestamp":"2024-01-01T12:34:56.789Z","mission_id":"...","agent_name":"prompt-injection","payload":{...}}
{"type":"tool.call.completed","level":1,"timestamp":"2024-01-01T12:34:59.456Z","agent_name":"recon","payload":{...}}
```

Use for:
- Log aggregation (ELK, Splunk, etc.)
- CI/CD pipelines
- Automated analysis

## Event Types

### LLM Events (`verbose.Level1`)

| Event | Payload | Description |
|-------|---------|-------------|
| `EventLLMRequestStarted` | `LLMRequestStartedData` | Completion request initiated |
| `EventLLMRequestCompleted` | `LLMRequestCompletedData` | Completion finished (tokens, duration) |
| `EventLLMRequestFailed` | `LLMRequestFailedData` | Completion error |
| `EventLLMStreamStarted` | `LLMStreamStartedData` | Streaming request initiated |
| `EventLLMStreamChunk` | `LLMStreamChunkData` | Stream chunk received (debug) |
| `EventLLMStreamCompleted` | `LLMStreamCompletedData` | Stream finished |

### Tool Events (`verbose.Level1`)

| Event | Payload | Description |
|-------|---------|-------------|
| `EventToolCallStarted` | `ToolCallStartedData` | Tool execution started |
| `EventToolCallCompleted` | `ToolCallCompletedData` | Tool execution finished |
| `EventToolCallFailed` | `ToolCallFailedData` | Tool execution error |
| `EventToolNotFound` | `ToolNotFoundData` | Tool not registered |

### Agent Events (`verbose.Level1-2`)

| Event | Payload | Description |
|-------|---------|-------------|
| `EventAgentStarted` | `AgentStartedData` | Agent execution started |
| `EventAgentCompleted` | `AgentCompletedData` | Agent finished |
| `EventAgentFailed` | `AgentFailedData` | Agent error |
| `EventAgentDelegated` | `AgentDelegatedData` | Agent delegated to another (L2) |
| `EventFindingSubmitted` | `FindingSubmittedData` | Vulnerability found |

### Mission Events (`verbose.Level2`)

| Event | Payload | Description |
|-------|---------|-------------|
| `EventMissionStarted` | `MissionStartedData` | Mission orchestration started |
| `EventMissionProgress` | `MissionProgressData` | Progress update |
| `EventMissionNode` | `MissionNodeData` | Node execution |
| `EventMissionCompleted` | `MissionCompletedData` | Mission finished |
| `EventMissionFailed` | `MissionFailedData` | Mission error |

### Memory Events (`verbose.Level3`)

| Event | Payload | Description |
|-------|---------|-------------|
| `EventMemoryGet` | `MemoryGetData` | Cache/store retrieval |
| `EventMemorySet` | `MemorySetData` | Cache/store write |
| `EventMemorySearch` | `MemorySearchData` | FTS5/vector search |

### System Events (`verbose.Level2`)

| Event | Payload | Description |
|-------|---------|-------------|
| `EventComponentRegistered` | `ComponentRegisteredData` | Component discovered |
| `EventComponentHealth` | `ComponentHealthData` | Health check result |
| `EventDaemonStarted` | `DaemonStartedData` | Daemon initialization |

## API Reference

### VerboseWriter

```go
type VerboseWriter struct { ... }

// Create a new writer
func NewVerboseWriter(w io.Writer, level VerboseLevel, jsonOutput bool) *VerboseWriter

// Start processing events (non-blocking)
func (vw *VerboseWriter) Start(ctx context.Context)

// Stop processing and wait for shutdown
func (vw *VerboseWriter) Stop()

// Get the event bus for emitting
func (vw *VerboseWriter) Bus() VerboseEventBus
```

### VerboseEvent

```go
type VerboseEvent struct {
    Type      VerboseEventType `json:"type"`
    Level     VerboseLevel     `json:"level"`
    Timestamp time.Time        `json:"timestamp"`
    MissionID types.ID         `json:"mission_id,omitempty"`
    AgentName string           `json:"agent_name,omitempty"`
    Payload   any              `json:"payload,omitempty"`
}

// Create a new event
func NewVerboseEvent(eventType VerboseEventType, level VerboseLevel, payload any) VerboseEvent

// Add mission context
func (e VerboseEvent) WithMissionID(missionID types.ID) VerboseEvent

// Add agent context
func (e VerboseEvent) WithAgentName(agentName string) VerboseEvent
```

### VerboseFormatter

```go
type VerboseFormatter interface {
    // Format converts an event to a string
    Format(event VerboseEvent) string
}

// Text formatter with colors
func NewTextVerboseFormatter() *TextVerboseFormatter

// JSON formatter (single-line)
func NewJSONVerboseFormatter() *JSONVerboseFormatter
```

## Usage Examples

### Example 1: CLI Command

```go
func runAttackCommand(cmd *cobra.Command, args []string) error {
    // Determine verbose level from flags
    verboseLevel := verbose.LevelNone
    if viper.GetBool("verbose") {
        verboseLevel = verbose.LevelVerbose
    }
    if viper.GetBool("very-verbose") {
        verboseLevel = verbose.LevelVeryVerbose
    }
    if viper.GetBool("debug") {
        verboseLevel = verbose.LevelDebug
    }

    // Create writer
    jsonOutput := viper.GetBool("json-verbose")
    writer := verbose.NewVerboseWriter(os.Stderr, verboseLevel, jsonOutput)
    writer.Start(cmd.Context())
    defer writer.Stop()

    // Pass bus to execution components
    bus := writer.Bus()
    harnessFactory := harness.NewFactory(
        harness.WithVerboseBus(bus),
        // ... other options
    )

    // Run attack
    return runAttack(cmd.Context(), harnessFactory, opts)
}
```

### Example 2: Harness Wrapper (Phase 2)

```go
type VerboseHarness struct {
    inner harness.Harness
    bus   verbose.VerboseEventBus
    level verbose.VerboseLevel
}

func (h *VerboseHarness) Complete(ctx context.Context, slot string, messages []llm.Message) (*llm.CompletionResponse, error) {
    // Emit start event
    startEvent := verbose.NewVerboseEvent(
        verbose.EventLLMRequestStarted,
        verbose.LevelVerbose,
        &verbose.LLMRequestStartedData{
            Provider:     h.getProvider(slot),
            Model:        h.getModel(slot),
            SlotName:     slot,
            MessageCount: len(messages),
        },
    )
    h.bus.Emit(ctx, startEvent)

    start := time.Now()

    // Execute
    resp, err := h.inner.Complete(ctx, slot, messages)

    duration := time.Since(start)

    // Emit completion/failure event
    if err != nil {
        failEvent := verbose.NewVerboseEvent(
            verbose.EventLLMRequestFailed,
            verbose.LevelVerbose,
            &verbose.LLMRequestFailedData{
                Provider: h.getProvider(slot),
                Model:    h.getModel(slot),
                SlotName: slot,
                Error:    err.Error(),
                Duration: duration,
            },
        )
        h.bus.Emit(ctx, failEvent)
        return nil, err
    }

    completeEvent := verbose.NewVerboseEvent(
        verbose.EventLLMRequestCompleted,
        verbose.LevelVerbose,
        &verbose.LLMRequestCompletedData{
            Provider:     h.getProvider(slot),
            Model:        resp.Model,
            SlotName:     slot,
            Duration:     duration,
            InputTokens:  resp.Usage.PromptTokens,
            OutputTokens: resp.Usage.CompletionTokens,
        },
    )
    h.bus.Emit(ctx, completeEvent)

    return resp, nil
}
```

### Example 3: Mission Orchestrator

```go
func (o *Orchestrator) Execute(ctx context.Context, mission *Mission) error {
    // Emit mission start
    o.bus.Emit(ctx, verbose.NewVerboseEvent(
        verbose.EventMissionStarted,
        verbose.LevelVeryVerbose,
        &verbose.MissionStartedData{
            MissionID:    mission.ID,
            WorkflowName: mission.WorkflowName,
            NodeCount:    len(mission.Nodes),
        },
    ).WithMissionID(mission.ID))

    // Execute nodes...
    for i, node := range mission.Nodes {
        // Emit progress
        o.bus.Emit(ctx, verbose.NewVerboseEvent(
            verbose.EventMissionProgress,
            verbose.LevelVeryVerbose,
            &verbose.MissionProgressData{
                MissionID:      mission.ID,
                CompletedNodes: i,
                TotalNodes:     len(mission.Nodes),
                CurrentNode:    node.ID,
            },
        ).WithMissionID(mission.ID))

        // Execute node...
    }

    return nil
}
```

## Progress Indicators

For long-running operations, Gibson provides simple progress indicators that work with both TTY and non-TTY output.

### Spinner (Indeterminate Progress)

Use when operation duration is unknown:

```go
import "github.com/zero-day-ai/gibson/internal/verbose"

spinner := verbose.NewSpinner(os.Stderr, "Loading components")
spinner.Start()
defer spinner.Stop()

// ... long operation ...

// Optionally update message
spinner.UpdateMessage("Processing configuration")
```

**TTY Output**: `| Loading components` (cycles through |/-\)
**Non-TTY Output**: Periodic line updates to avoid spam

### Progress Bar (Determinate Progress)

Use when total work is known:

```go
bar := verbose.NewProgressBar(os.Stderr, 100, "Processing items")
bar.Start()
defer bar.Stop()

for i := 0; i < 100; i++ {
    // ... process item ...
    bar.Update(int64(i + 1))
    // Or use: bar.Increment()
}
```

**TTY Output**: `[=====>    ] 50% Processing items`
**Non-TTY Output**: Periodic percentage updates (throttled to 5% increments)

**Features**:
- Automatic TTY detection
- Thread-safe concurrent updates
- Configurable bar width and update frequency
- Graceful fallback for non-TTY output

## Performance

- **Non-blocking**: Events use buffered channels with non-blocking sends
- **Memory**: ~100KB per VerboseWriter (1000-event buffer)
- **Goroutines**: 1 background goroutine per writer
- **Overhead**: ~1-2μs per event emission (amortized)

**Slow Consumer Handling**: If a subscriber can't keep up, events are dropped for that subscriber only (preventing blocking).

## Testing

Run tests:
```bash
go test -tags fts5 -v ./internal/verbose/
```

Run examples:
```bash
go test -tags fts5 -v -run Example ./internal/verbose/
```

## Files

- `event_bus.go` - Event bus implementation
- `event_types.go` - Event type definitions and payload structs
- `formatter.go` - Formatter interface
- `formatter_text.go` - Text formatter (407 lines)
- `formatter_json.go` - JSON formatter (38 lines)
- `writer.go` - CLI writer helper (154 lines)
- `progress.go` - Progress indicators (Spinner, ProgressBar)
- `formatter_test.go` - Comprehensive tests
- `integration_test.go` - Integration tests for full verbose flow
- `progress_test.go` - Progress indicator tests
- `example_test.go` - Usage examples

## Implementation Status

- ✅ Phase 1: Core formatters and writer (COMPLETE)
- ✅ Phase 2: Harness integration (COMPLETE)
- ✅ Phase 3: CLI wiring (COMPLETE)
- ✅ Phase 4: Mission integration (COMPLETE)
- ✅ Phase 5: Progress indicators (COMPLETE)
- ✅ Phase 6: Documentation (COMPLETE)

## See Also

- `PHASE1_IMPLEMENTATION.md` - Detailed implementation notes
- `event_types.go` - Complete event type reference
- `formatter_test.go` - Test examples
