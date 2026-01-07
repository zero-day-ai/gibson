package verbose

import (
	"context"
	"log/slog"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"go.opentelemetry.io/otel/trace"
)

// VerboseHarnessWrapper wraps an AgentHarness and emits VerboseEvents for all operations.
// It implements the harness.AgentHarness interface and transparently emits events to a
// VerboseEventBus while delegating all actual work to the inner harness.
//
// The wrapper emits events for:
//   - LLM completions (Complete, CompleteWithTools, Stream)
//   - Tool executions (CallTool)
//   - Plugin queries (QueryPlugin)
//   - Sub-agent delegation (DelegateToAgent)
//   - Finding submissions (SubmitFinding)
//
// All pass-through methods (Memory, Mission, Target, List*, Tracer, Logger, Metrics, TokenUsage)
// are delegated directly without event emission.
type VerboseHarnessWrapper struct {
	inner harness.AgentHarness
	bus   VerboseEventBus
	level VerboseLevel
}

// NewVerboseHarnessWrapper creates a new VerboseHarnessWrapper that wraps the provided
// inner harness with event emission.
//
// Parameters:
//   - inner: The underlying AgentHarness to wrap
//   - bus: The event bus to emit events to
//   - level: The verbosity level for emitted events
//
// Returns:
//   - *VerboseHarnessWrapper: A verbose harness ready for use
//
// Example:
//
//	bus := NewDefaultVerboseEventBus()
//	wrapped := NewVerboseHarnessWrapper(innerHarness, bus, LevelVerbose)
//	resp, err := wrapped.Complete(ctx, "primary", messages)
func NewVerboseHarnessWrapper(inner harness.AgentHarness, bus VerboseEventBus, level VerboseLevel) *VerboseHarnessWrapper {
	return &VerboseHarnessWrapper{
		inner: inner,
		bus:   bus,
		level: level,
	}
}

// Complete performs a synchronous LLM completion with event emission.
// Emits EventLLMRequestStarted before the call and EventLLMRequestCompleted/Failed after.
func (w *VerboseHarnessWrapper) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	// Emit started event
	startData := LLMRequestStartedData{
		SlotName:     slot,
		MessageCount: len(messages),
		Stream:       false,
	}

	// At Debug level, include truncated prompt content
	if w.level >= LevelDebug {
		startData.PromptPreview = truncatePromptContent(messages, 2000)
	}

	startEvent := NewVerboseEvent(EventLLMRequestStarted, w.level, startData).
		WithMissionID(w.Mission().ID).
		WithAgentName(w.Mission().CurrentAgent)

	_ = w.bus.Emit(ctx, startEvent)

	// Record start time
	startTime := time.Now()

	// Call inner harness
	resp, err := w.inner.Complete(ctx, slot, messages, opts...)

	// Calculate duration
	duration := time.Since(startTime)

	// Emit completion or failure event
	if err != nil {
		failedData := LLMRequestFailedData{
			SlotName:  slot,
			Error:     err.Error(),
			Duration:  duration,
			Retryable: false,
		}

		// At Debug level, include full error details
		if w.level >= LevelDebug {
			failedData.ErrorDetails = err.Error() // Full error message
		}

		failedEvent := NewVerboseEvent(EventLLMRequestFailed, w.level, failedData).
			WithMissionID(w.Mission().ID).
			WithAgentName(w.Mission().CurrentAgent)

		_ = w.bus.Emit(ctx, failedEvent)
		return nil, err
	}

	// Emit completed event
	completedData := LLMRequestCompletedData{
		SlotName: slot,
		Duration: duration,
	}

	if resp != nil {
		completedData.Model = resp.Model
		completedData.InputTokens = resp.Usage.PromptTokens
		completedData.OutputTokens = resp.Usage.CompletionTokens
		completedData.StopReason = string(resp.FinishReason)
		completedData.ResponseLength = len(resp.Message.Content)

		// At Debug level, include truncated response content
		if w.level >= LevelDebug {
			completedData.ResponsePreview = truncateContent(resp.Message.Content, 2000)
		}
	}

	completedEvent := NewVerboseEvent(EventLLMRequestCompleted, w.level, completedData).
		WithMissionID(w.Mission().ID).
		WithAgentName(w.Mission().CurrentAgent)

	_ = w.bus.Emit(ctx, completedEvent)

	return resp, nil
}

// CompleteWithTools performs a completion with tool-calling capabilities with event emission.
// Emits EventLLMRequestStarted before the call and EventLLMRequestCompleted/Failed after.
func (w *VerboseHarnessWrapper) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	// Emit started event
	startData := LLMRequestStartedData{
		SlotName:     slot,
		MessageCount: len(messages),
		Stream:       false,
	}

	// At Debug level, include truncated prompt content and tool info
	if w.level >= LevelDebug {
		startData.PromptPreview = truncatePromptContent(messages, 2000)
		startData.ToolCount = len(tools)
	}

	startEvent := NewVerboseEvent(EventLLMRequestStarted, w.level, startData).
		WithMissionID(w.Mission().ID).
		WithAgentName(w.Mission().CurrentAgent)

	_ = w.bus.Emit(ctx, startEvent)

	// Record start time
	startTime := time.Now()

	// Call inner harness
	resp, err := w.inner.CompleteWithTools(ctx, slot, messages, tools, opts...)

	// Calculate duration
	duration := time.Since(startTime)

	// Emit completion or failure event
	if err != nil {
		failedData := LLMRequestFailedData{
			SlotName:  slot,
			Error:     err.Error(),
			Duration:  duration,
			Retryable: false,
		}

		// At Debug level, include full error details
		if w.level >= LevelDebug {
			failedData.ErrorDetails = err.Error()
		}

		failedEvent := NewVerboseEvent(EventLLMRequestFailed, w.level, failedData).
			WithMissionID(w.Mission().ID).
			WithAgentName(w.Mission().CurrentAgent)

		_ = w.bus.Emit(ctx, failedEvent)
		return nil, err
	}

	// Emit completed event
	completedData := LLMRequestCompletedData{
		SlotName: slot,
		Duration: duration,
	}

	if resp != nil {
		completedData.Model = resp.Model
		completedData.InputTokens = resp.Usage.PromptTokens
		completedData.OutputTokens = resp.Usage.CompletionTokens
		completedData.StopReason = string(resp.FinishReason)
		completedData.ResponseLength = len(resp.Message.Content)

		// At Debug level, include truncated response content
		if w.level >= LevelDebug {
			completedData.ResponsePreview = truncateContent(resp.Message.Content, 2000)
		}
	}

	completedEvent := NewVerboseEvent(EventLLMRequestCompleted, w.level, completedData).
		WithMissionID(w.Mission().ID).
		WithAgentName(w.Mission().CurrentAgent)

	_ = w.bus.Emit(ctx, completedEvent)

	return resp, nil
}

// Stream performs a streaming LLM completion with event emission.
// Emits EventLLMStreamStarted when the stream begins, wraps the channel to emit
// EventLLMStreamCompleted when the stream finishes.
func (w *VerboseHarnessWrapper) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	// Emit started event
	startEvent := NewVerboseEvent(EventLLMStreamStarted, w.level, LLMStreamStartedData{
		SlotName:     slot,
		MessageCount: len(messages),
	}).WithMissionID(w.Mission().ID).WithAgentName(w.Mission().CurrentAgent)

	_ = w.bus.Emit(ctx, startEvent)

	// Record start time
	startTime := time.Now()

	// Call inner harness
	innerChan, err := w.inner.Stream(ctx, slot, messages, opts...)
	if err != nil {
		// Stream setup failed
		failedEvent := NewVerboseEvent(EventLLMRequestFailed, w.level, LLMRequestFailedData{
			SlotName:  slot,
			Error:     err.Error(),
			Duration:  time.Since(startTime),
			Retryable: false,
		}).WithMissionID(w.Mission().ID).WithAgentName(w.Mission().CurrentAgent)

		_ = w.bus.Emit(ctx, failedEvent)
		return nil, err
	}

	// Wrap the channel to track completion
	wrappedChan := make(chan llm.StreamChunk)
	go func() {
		defer close(wrappedChan)

		totalChunks := 0
		finalContentLength := 0

		for chunk := range innerChan {
			wrappedChan <- chunk
			totalChunks++

			// Estimate content length
			finalContentLength += len(chunk.Delta.Content)
		}

		duration := time.Since(startTime)

		// Emit stream completed event
		completedEvent := NewVerboseEvent(EventLLMStreamCompleted, w.level, LLMStreamCompletedData{
			SlotName:           slot,
			Model:              "",
			Duration:           duration,
			TotalChunks:        totalChunks,
			InputTokens:        0,
			OutputTokens:       0,
			FinalContentLength: finalContentLength,
		}).WithMissionID(w.Mission().ID).WithAgentName(w.Mission().CurrentAgent)

		_ = w.bus.Emit(ctx, completedEvent)
	}()

	return wrappedChan, nil
}

// CallTool executes a tool with event emission.
// Emits EventToolCallStarted before execution and EventToolCallCompleted/Failed after.
func (w *VerboseHarnessWrapper) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	// Emit started event
	startEvent := NewVerboseEvent(EventToolCallStarted, w.level, ToolCallStartedData{
		ToolName:      name,
		Parameters:    input,
		ParameterSize: len(input),
	}).WithMissionID(w.Mission().ID).WithAgentName(w.Mission().CurrentAgent)

	_ = w.bus.Emit(ctx, startEvent)

	// Record start time
	startTime := time.Now()

	// Call inner harness
	result, err := w.inner.CallTool(ctx, name, input)

	// Calculate duration
	duration := time.Since(startTime)

	// Emit completion or failure event
	if err != nil {
		failedEvent := NewVerboseEvent(EventToolCallFailed, w.level, ToolCallFailedData{
			ToolName: name,
			Error:    err.Error(),
			Duration: duration,
		}).WithMissionID(w.Mission().ID).WithAgentName(w.Mission().CurrentAgent)

		_ = w.bus.Emit(ctx, failedEvent)
		return nil, err
	}

	// Emit completed event
	completedEvent := NewVerboseEvent(EventToolCallCompleted, w.level, ToolCallCompletedData{
		ToolName:   name,
		Duration:   duration,
		ResultSize: len(result),
		Success:    true,
	}).WithMissionID(w.Mission().ID).WithAgentName(w.Mission().CurrentAgent)

	_ = w.bus.Emit(ctx, completedEvent)

	return result, nil
}

// QueryPlugin calls a plugin method with event emission.
// Emits EventPluginQueryStarted before execution and EventPluginQueryCompleted/Failed after.
func (w *VerboseHarnessWrapper) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	// Emit started event
	startEvent := NewVerboseEvent(EventPluginQueryStarted, w.level, PluginQueryStartedData{
		PluginName:     name,
		Method:         method,
		Parameters:     params,
		ParameterCount: len(params),
	}).WithMissionID(w.Mission().ID).WithAgentName(w.Mission().CurrentAgent)

	_ = w.bus.Emit(ctx, startEvent)

	// Record start time
	startTime := time.Now()

	// Call inner harness
	result, err := w.inner.QueryPlugin(ctx, name, method, params)

	// Calculate duration
	duration := time.Since(startTime)

	// Emit completion or failure event
	if err != nil {
		failedEvent := NewVerboseEvent(EventPluginQueryFailed, w.level, PluginQueryFailedData{
			PluginName: name,
			Method:     method,
			Error:      err.Error(),
			Duration:   duration,
		}).WithMissionID(w.Mission().ID).WithAgentName(w.Mission().CurrentAgent)

		_ = w.bus.Emit(ctx, failedEvent)
		return nil, err
	}

	// Emit completed event
	completedEvent := NewVerboseEvent(EventPluginQueryCompleted, w.level, PluginQueryCompletedData{
		PluginName: name,
		Method:     method,
		Duration:   duration,
		Success:    true,
	}).WithMissionID(w.Mission().ID).WithAgentName(w.Mission().CurrentAgent)

	_ = w.bus.Emit(ctx, completedEvent)

	return result, nil
}

// DelegateToAgent delegates a task to another agent with event emission.
// Emits EventAgentDelegated when delegation occurs.
func (w *VerboseHarnessWrapper) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	// Emit delegation event
	delegatedEvent := NewVerboseEvent(EventAgentDelegated, w.level, AgentDelegatedData{
		FromAgent:       w.Mission().CurrentAgent,
		ToAgent:         name,
		TaskDescription: task.Name,
	}).WithMissionID(w.Mission().ID).WithAgentName(w.Mission().CurrentAgent)

	_ = w.bus.Emit(ctx, delegatedEvent)

	// Call inner harness
	return w.inner.DelegateToAgent(ctx, name, task)
}

// SubmitFinding submits a security finding with event emission.
// Emits EventFindingSubmitted when the finding is successfully stored.
func (w *VerboseHarnessWrapper) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	// Call inner harness
	err := w.inner.SubmitFinding(ctx, finding)

	if err != nil {
		return err
	}

	// Emit finding submitted event
	submittedEvent := NewVerboseEvent(EventFindingSubmitted, w.level, FindingSubmittedData{
		FindingID: finding.ID,
		Title:     finding.Title,
		Severity:  string(finding.Severity),
		AgentName: w.Mission().CurrentAgent,
		// TechniqueIDs would be populated from metadata if available
	}).WithMissionID(w.Mission().ID).WithAgentName(w.Mission().CurrentAgent)

	_ = w.bus.Emit(ctx, submittedEvent)

	return nil
}

// GetFindings retrieves findings with the specified filter.
// This is a pass-through operation without event emission.
func (w *VerboseHarnessWrapper) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return w.inner.GetFindings(ctx, filter)
}

// Memory returns the memory store from the inner harness.
func (w *VerboseHarnessWrapper) Memory() memory.MemoryStore {
	return w.inner.Memory()
}

// Mission returns the current mission context from the inner harness.
func (w *VerboseHarnessWrapper) Mission() harness.MissionContext {
	return w.inner.Mission()
}

// Target returns the current target information from the inner harness.
func (w *VerboseHarnessWrapper) Target() harness.TargetInfo {
	return w.inner.Target()
}

// ListTools returns all available tool descriptors from the inner harness.
func (w *VerboseHarnessWrapper) ListTools() []harness.ToolDescriptor {
	return w.inner.ListTools()
}

// ListPlugins returns all available plugin descriptors from the inner harness.
func (w *VerboseHarnessWrapper) ListPlugins() []harness.PluginDescriptor {
	return w.inner.ListPlugins()
}

// ListAgents returns all available agent descriptors from the inner harness.
func (w *VerboseHarnessWrapper) ListAgents() []harness.AgentDescriptor {
	return w.inner.ListAgents()
}

// Tracer returns the OpenTelemetry tracer from the inner harness.
func (w *VerboseHarnessWrapper) Tracer() trace.Tracer {
	return w.inner.Tracer()
}

// Logger returns the structured logger from the inner harness.
func (w *VerboseHarnessWrapper) Logger() *slog.Logger {
	return w.inner.Logger()
}

// Metrics returns the metrics recorder from the inner harness.
func (w *VerboseHarnessWrapper) Metrics() harness.MetricsRecorder {
	return w.inner.Metrics()
}

// TokenUsage returns the token usage tracker from the inner harness.
func (w *VerboseHarnessWrapper) TokenUsage() *llm.TokenTracker {
	return w.inner.TokenUsage()
}

// truncatePromptContent truncates a list of messages to a maximum character count.
// Used at Debug level to include prompt previews in events without overwhelming logs.
func truncatePromptContent(messages []llm.Message, maxChars int) string {
	if len(messages) == 0 {
		return ""
	}

	var totalContent string
	for i, msg := range messages {
		prefix := ""
		if i > 0 {
			prefix = " | "
		}
		totalContent += prefix + string(msg.Role) + ": " + msg.Content

		// Stop if we've exceeded max chars
		if len(totalContent) > maxChars {
			break
		}
	}

	return truncateContent(totalContent, maxChars)
}

// truncateContent truncates a string to a maximum character count with an ellipsis.
func truncateContent(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}

	if maxChars <= 3 {
		return content[:maxChars]
	}

	return content[:maxChars-3] + "..."
}

// Ensure VerboseHarnessWrapper implements AgentHarness at compile time
var _ harness.AgentHarness = (*VerboseHarnessWrapper)(nil)
