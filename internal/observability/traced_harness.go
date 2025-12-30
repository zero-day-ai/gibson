package observability

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracedAgentHarness wraps an AgentHarness with OpenTelemetry tracing.
// It creates spans for all major operations and records GenAI and Gibson-specific
// attributes for observability.
//
// All harness operations are traced with appropriate span names:
//   - LLM completions: "gen_ai.chat" or "gen_ai.chat.stream"
//   - Tool calls: "gen_ai.tool"
//   - Plugin queries: "gibson.plugin.query"
//   - Agent delegation: "gibson.agent.delegate"
//   - Finding submission: "gibson.finding.submit"
//
// The harness also records metrics for LLM operations, tool calls, and findings.
type TracedAgentHarness struct {
	inner         harness.AgentHarness
	tracer        trace.Tracer
	metrics       harness.MetricsRecorder
	logger        *TracedLogger
	capturePrompt bool
	turnCounter   *atomic.Int32
}

// TracedHarnessOption is a functional option for configuring TracedAgentHarness.
type TracedHarnessOption func(*TracedAgentHarness)

// WithTracer sets the OpenTelemetry tracer for the harness.
// If not provided, the inner harness's tracer is used.
func WithTracer(tracer trace.Tracer) TracedHarnessOption {
	return func(h *TracedAgentHarness) {
		h.tracer = tracer
	}
}

// WithMetrics sets the metrics recorder for the harness.
// If not provided, the inner harness's metrics recorder is used.
func WithMetrics(metrics harness.MetricsRecorder) TracedHarnessOption {
	return func(h *TracedAgentHarness) {
		h.metrics = metrics
	}
}

// WithLogger sets the traced logger for the harness.
// If not provided, a default TracedLogger is created.
func WithLogger(logger *TracedLogger) TracedHarnessOption {
	return func(h *TracedAgentHarness) {
		h.logger = logger
	}
}

// WithPromptCapture enables or disables capturing full prompts in trace spans.
// This is useful for debugging and observability.
// Default is true. Set to false in production if handling sensitive data.
func WithPromptCapture(capture bool) TracedHarnessOption {
	return func(h *TracedAgentHarness) {
		h.capturePrompt = capture
	}
}

// NewTracedAgentHarness creates a new TracedAgentHarness that wraps the provided
// inner harness with OpenTelemetry tracing.
//
// Parameters:
//   - inner: The underlying AgentHarness to wrap
//   - opts: Optional configuration options
//
// Returns:
//   - *TracedAgentHarness: A traced harness ready for use
//
// Example:
//
//	traced := NewTracedAgentHarness(
//	    innerHarness,
//	    WithTracer(myTracer),
//	    WithPromptCapture(true),
//	)
func NewTracedAgentHarness(inner harness.AgentHarness, opts ...TracedHarnessOption) *TracedAgentHarness {
	h := &TracedAgentHarness{
		inner:         inner,
		tracer:        inner.Tracer(),
		metrics:       inner.Metrics(),
		capturePrompt: true,
		turnCounter:   &atomic.Int32{},
	}

	// Apply options
	for _, opt := range opts {
		opt(h)
	}

	// If no logger provided, try to get one from inner harness
	if h.logger == nil {
		// Create a basic traced logger using the inner harness's logger
		missionCtx := inner.Mission()
		h.logger = NewTracedLogger(
			slog.Default().Handler(),
			missionCtx.ID.String(),
			missionCtx.CurrentAgent,
		)
	}

	return h
}

// Complete performs a synchronous LLM completion with distributed tracing.
// Creates a "gen_ai.chat" span with GenAI attributes and token usage.
func (h *TracedAgentHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	// Increment turn counter
	turn := h.turnCounter.Add(1)

	// Start tracing span
	ctx, span := h.tracer.Start(ctx, SpanGenAIChat)
	defer span.End()

	// Add turn number attribute
	span.SetAttributes(TurnAttributes(int(turn))...)

	// Record start time for metrics
	startTime := time.Now()

	// Call inner harness
	resp, err := h.inner.Complete(ctx, slot, messages, opts...)

	// Calculate duration
	duration := time.Since(startTime)
	durationMs := float64(duration.Milliseconds())

	// Record error if present
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "COMPLETION_ERROR")...)

		// Record failed metric
		if h.metrics != nil {
			labels := map[string]string{
				"slot":     slot,
				"provider": "unknown",
				"model":    "unknown",
				"status":   "error",
			}
			h.metrics.RecordCounter(MetricLLMCompletions, 1, labels)
			h.metrics.RecordHistogram(MetricLLMLatency, durationMs, labels)
		}

		h.logger.Error(ctx, "LLM completion failed",
			slog.String("slot", slot),
			slog.String("error", err.Error()),
			slog.Duration("duration", duration),
		)

		return nil, err
	}

	// Add GenAI attributes to span
	if resp != nil {
		// Response attributes
		span.SetAttributes(ResponseAttributes(resp)...)

		// Usage attributes
		span.SetAttributes(UsageAttributes(resp.Usage)...)

		// Add slot and model info
		span.SetAttributes(
			attribute.String("gen_ai.system", "unknown"), // Provider unknown from resp
			attribute.String("gen_ai.request.model", resp.Model),
			attribute.String("gibson.llm.slot", slot),
		)

		// Optionally capture prompt
		if h.capturePrompt && len(messages) > 0 {
			promptText := formatMessagesForLogging(messages)
			span.SetAttributes(attribute.String(GenAIPrompt, promptText))
		}

		// Record success metrics
		if h.metrics != nil {
			labels := map[string]string{
				"slot":     slot,
				"provider": "unknown",
				"model":    resp.Model,
				"status":   "success",
			}
			h.metrics.RecordCounter(MetricLLMCompletions, 1, labels)
			h.metrics.RecordCounter(MetricLLMTokensInput, int64(resp.Usage.PromptTokens), labels)
			h.metrics.RecordCounter(MetricLLMTokensOutput, int64(resp.Usage.CompletionTokens), labels)
			h.metrics.RecordHistogram(MetricLLMLatency, durationMs, labels)
		}

		h.logger.Info(ctx, "LLM completion succeeded",
			slog.String("slot", slot),
			slog.String("model", resp.Model),
			slog.Int("input_tokens", resp.Usage.PromptTokens),
			slog.Int("output_tokens", resp.Usage.CompletionTokens),
			slog.Duration("duration", duration),
		)
	}

	span.SetStatus(codes.Ok, "completion succeeded")
	return resp, nil
}

// CompleteWithTools performs a completion with tool-calling capabilities with distributed tracing.
// Creates a "gen_ai.chat" span and includes tool definitions in span attributes.
func (h *TracedAgentHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	// Increment turn counter
	turn := h.turnCounter.Add(1)

	// Start tracing span
	ctx, span := h.tracer.Start(ctx, SpanGenAIChat)
	defer span.End()

	// Add turn number and tool count attributes
	span.SetAttributes(TurnAttributes(int(turn))...)
	span.SetAttributes(attribute.Int("gen_ai.tools.count", len(tools)))

	// Add tool names
	if len(tools) > 0 {
		toolNames := make([]string, len(tools))
		for i, tool := range tools {
			toolNames[i] = tool.Name
		}
		span.SetAttributes(attribute.StringSlice("gen_ai.tools.names", toolNames))
	}

	// Record start time for metrics
	startTime := time.Now()

	// Call inner harness
	resp, err := h.inner.CompleteWithTools(ctx, slot, messages, tools, opts...)

	// Calculate duration
	duration := time.Since(startTime)
	durationMs := float64(duration.Milliseconds())

	// Record error if present
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "COMPLETION_WITH_TOOLS_ERROR")...)

		// Record failed metric
		if h.metrics != nil {
			labels := map[string]string{
				"slot":     slot,
				"provider": "unknown",
				"model":    "unknown",
				"status":   "error",
			}
			h.metrics.RecordCounter(MetricLLMCompletions, 1, labels)
			h.metrics.RecordHistogram(MetricLLMLatency, durationMs, labels)
		}

		h.logger.Error(ctx, "LLM completion with tools failed",
			slog.String("slot", slot),
			slog.Int("tool_count", len(tools)),
			slog.String("error", err.Error()),
			slog.Duration("duration", duration),
		)

		return nil, err
	}

	// Add GenAI attributes to span
	if resp != nil {
		// Response attributes
		span.SetAttributes(ResponseAttributes(resp)...)

		// Usage attributes
		span.SetAttributes(UsageAttributes(resp.Usage)...)

		// Add slot and model info
		span.SetAttributes(
			attribute.String("gen_ai.request.model", resp.Model),
			attribute.String("gibson.llm.slot", slot),
		)

		// Check if response contains tool calls
		if len(resp.Message.ToolCalls) > 0 {
			span.SetAttributes(attribute.Int("gen_ai.response.tool_calls.count", len(resp.Message.ToolCalls)))
			toolCallNames := make([]string, len(resp.Message.ToolCalls))
			for i, tc := range resp.Message.ToolCalls {
				toolCallNames[i] = tc.Name
			}
			span.SetAttributes(attribute.StringSlice("gen_ai.response.tool_calls.names", toolCallNames))
		}

		// Optionally capture prompt
		if h.capturePrompt && len(messages) > 0 {
			promptText := formatMessagesForLogging(messages)
			span.SetAttributes(attribute.String(GenAIPrompt, promptText))
		}

		// Record success metrics
		if h.metrics != nil {
			labels := map[string]string{
				"slot":     slot,
				"provider": "unknown",
				"model":    resp.Model,
				"status":   "success",
			}
			h.metrics.RecordCounter(MetricLLMCompletions, 1, labels)
			h.metrics.RecordCounter(MetricLLMTokensInput, int64(resp.Usage.PromptTokens), labels)
			h.metrics.RecordCounter(MetricLLMTokensOutput, int64(resp.Usage.CompletionTokens), labels)
			h.metrics.RecordHistogram(MetricLLMLatency, durationMs, labels)
		}

		h.logger.Info(ctx, "LLM completion with tools succeeded",
			slog.String("slot", slot),
			slog.String("model", resp.Model),
			slog.Int("tool_count", len(tools)),
			slog.Int("tool_calls", len(resp.Message.ToolCalls)),
			slog.Int("input_tokens", resp.Usage.PromptTokens),
			slog.Int("output_tokens", resp.Usage.CompletionTokens),
			slog.Duration("duration", duration),
		)
	}

	span.SetStatus(codes.Ok, "completion with tools succeeded")
	return resp, nil
}

// Stream performs a streaming LLM completion with distributed tracing.
// Creates a "gen_ai.chat.stream" span.
func (h *TracedAgentHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	// Increment turn counter
	turn := h.turnCounter.Add(1)

	// Start tracing span
	ctx, span := h.tracer.Start(ctx, SpanGenAIChatStream)
	defer span.End()

	// Add turn number attribute
	span.SetAttributes(TurnAttributes(int(turn))...)
	span.SetAttributes(attribute.String("gibson.llm.slot", slot))

	// Optionally capture prompt
	if h.capturePrompt && len(messages) > 0 {
		promptText := formatMessagesForLogging(messages)
		span.SetAttributes(attribute.String(GenAIPrompt, promptText))
	}

	// Record start time for metrics
	startTime := time.Now()

	// Call inner harness
	chunkChan, err := h.inner.Stream(ctx, slot, messages, opts...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "STREAM_ERROR")...)

		h.logger.Error(ctx, "LLM stream failed",
			slog.String("slot", slot),
			slog.String("error", err.Error()),
		)

		return nil, err
	}

	// Log stream started
	h.logger.Info(ctx, "LLM stream started",
		slog.String("slot", slot),
		slog.Int("turn", int(turn)),
	)

	// Wrap the channel to track completion
	wrappedChan := make(chan llm.StreamChunk)
	go func() {
		defer close(wrappedChan)
		totalTokens := 0

		for chunk := range chunkChan {
			wrappedChan <- chunk

			// Check for errors in chunk
			if chunk.Error != nil {
				span.RecordError(chunk.Error)
				span.SetAttributes(ErrorAttributes(chunk.Error, "STREAM_CHUNK_ERROR")...)
			}

			// Track approximate token count (rough estimate)
			if chunk.Delta.Content != "" {
				totalTokens += len(chunk.Delta.Content) / 4 // Rough token estimate
			}
		}

		duration := time.Since(startTime)
		span.SetAttributes(
			attribute.Int("gen_ai.usage.estimated_tokens", totalTokens),
			attribute.Float64("gibson.stream.duration_ms", float64(duration.Milliseconds())),
		)

		h.logger.Info(ctx, "LLM stream completed",
			slog.String("slot", slot),
			slog.Int("estimated_tokens", totalTokens),
			slog.Duration("duration", duration),
		)

		span.SetStatus(codes.Ok, "stream completed")
	}()

	return wrappedChan, nil
}

// CallTool executes a tool with distributed tracing.
// Creates a "gen_ai.tool" span with tool name and duration.
func (h *TracedAgentHarness) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	// Start tracing span
	ctx, span := h.tracer.Start(ctx, SpanGenAITool)
	defer span.End()

	// Add tool attributes
	span.SetAttributes(ToolAttributes(name)...)

	// Record start time for metrics
	startTime := time.Now()

	// Call inner harness
	result, err := h.inner.CallTool(ctx, name, input)

	// Calculate duration
	duration := time.Since(startTime)
	durationMs := float64(duration.Milliseconds())

	// Record error if present
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "TOOL_EXECUTION_ERROR")...)

		// Record failed metric
		if h.metrics != nil {
			labels := map[string]string{
				"tool":   name,
				"status": "error",
			}
			h.metrics.RecordCounter(MetricToolCalls, 1, labels)
			h.metrics.RecordHistogram(MetricToolDuration, durationMs, labels)
		}

		h.logger.Error(ctx, "Tool execution failed",
			slog.String("tool", name),
			slog.String("error", err.Error()),
			slog.Duration("duration", duration),
		)

		return nil, err
	}

	// Record success
	span.SetAttributes(attribute.Float64("gibson.tool.duration_ms", durationMs))
	span.SetStatus(codes.Ok, "tool execution succeeded")

	// Record success metric
	if h.metrics != nil {
		labels := map[string]string{
			"tool":   name,
			"status": "success",
		}
		h.metrics.RecordCounter(MetricToolCalls, 1, labels)
		h.metrics.RecordHistogram(MetricToolDuration, durationMs, labels)
	}

	h.logger.Info(ctx, "Tool execution succeeded",
		slog.String("tool", name),
		slog.Duration("duration", duration),
	)

	return result, nil
}

// QueryPlugin calls a plugin method with distributed tracing.
// Creates a "gibson.plugin.query" span with plugin name and method.
func (h *TracedAgentHarness) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	// Start tracing span
	ctx, span := h.tracer.Start(ctx, SpanPluginQuery)
	defer span.End()

	// Add plugin attributes
	span.SetAttributes(PluginAttributes(name, method)...)

	// Record start time
	startTime := time.Now()

	// Call inner harness
	result, err := h.inner.QueryPlugin(ctx, name, method, params)

	// Calculate duration
	duration := time.Since(startTime)
	durationMs := float64(duration.Milliseconds())

	// Record error if present
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "PLUGIN_QUERY_ERROR")...)

		h.logger.Error(ctx, "Plugin query failed",
			slog.String("plugin", name),
			slog.String("method", method),
			slog.String("error", err.Error()),
			slog.Duration("duration", duration),
		)

		return nil, err
	}

	// Record success
	span.SetAttributes(attribute.Float64("gibson.plugin.duration_ms", durationMs))
	span.SetStatus(codes.Ok, "plugin query succeeded")

	h.logger.Info(ctx, "Plugin query succeeded",
		slog.String("plugin", name),
		slog.String("method", method),
		slog.Duration("duration", duration),
	)

	return result, nil
}

// DelegateToAgent delegates a task to another agent with distributed tracing.
// Creates a "gibson.agent.delegate" span with target agent and task info.
func (h *TracedAgentHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	// Start tracing span
	ctx, span := h.tracer.Start(ctx, SpanAgentDelegate)
	defer span.End()

	// Add delegation and task attributes
	span.SetAttributes(DelegationAttributes(name, task.ID)...)
	span.SetAttributes(TaskAttributes(task)...)

	// Record start time
	startTime := time.Now()

	// Call inner harness
	result, err := h.inner.DelegateToAgent(ctx, name, task)

	// Calculate duration
	duration := time.Since(startTime)

	// Record error if present
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "DELEGATION_ERROR")...)

		h.logger.Error(ctx, "Agent delegation failed",
			slog.String("target_agent", name),
			slog.String("task_id", task.ID.String()),
			slog.String("task_name", task.Name),
			slog.String("error", err.Error()),
			slog.Duration("duration", duration),
		)

		return agent.Result{}, err
	}

	// Record success with result metrics
	span.SetAttributes(
		attribute.String("gibson.delegation.result_status", string(result.Status)),
		attribute.Float64("gibson.delegation.duration_ms", float64(duration.Milliseconds())),
	)

	// Add result metrics
	if result.Metrics.Duration > 0 {
		span.SetAttributes(MetricsAttributes(result.Metrics)...)
	}

	span.SetStatus(codes.Ok, "delegation succeeded")

	h.logger.Info(ctx, "Agent delegation succeeded",
		slog.String("target_agent", name),
		slog.String("task_id", task.ID.String()),
		slog.String("task_name", task.Name),
		slog.String("result_status", string(result.Status)),
		slog.Int("findings", len(result.Findings)),
		slog.Duration("duration", duration),
	)

	return result, nil
}

// SubmitFinding submits a security finding with distributed tracing.
// Creates a "gibson.finding.submit" span with finding attributes.
func (h *TracedAgentHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	// Start tracing span
	ctx, span := h.tracer.Start(ctx, SpanFindingSubmit)
	defer span.End()

	// Add finding attributes
	span.SetAttributes(FindingAttributes(finding)...)

	// Call inner harness
	err := h.inner.SubmitFinding(ctx, finding)

	// Record error if present
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "FINDING_SUBMIT_ERROR")...)

		h.logger.Error(ctx, "Finding submission failed",
			slog.String("finding_id", finding.ID.String()),
			slog.String("severity", string(finding.Severity)),
			slog.String("title", finding.Title),
			slog.String("error", err.Error()),
		)

		return err
	}

	// Record success metric
	if h.metrics != nil {
		labels := map[string]string{
			"severity": string(finding.Severity),
			"category": finding.Category,
		}
		h.metrics.RecordCounter(MetricFindingsSubmitted, 1, labels)
	}

	span.SetStatus(codes.Ok, "finding submitted")

	h.logger.Info(ctx, "Finding submitted",
		slog.String("finding_id", finding.ID.String()),
		slog.String("severity", string(finding.Severity)),
		slog.String("category", finding.Category),
		slog.String("title", finding.Title),
		slog.Float64("confidence", finding.Confidence),
	)

	return nil
}

// GetFindings retrieves findings with the specified filter.
// This is a pass-through operation without additional tracing.
func (h *TracedAgentHarness) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return h.inner.GetFindings(ctx, filter)
}

// Memory returns the memory store.
// This is a pass-through operation without additional tracing.
func (h *TracedAgentHarness) Memory() memory.MemoryStore {
	return h.inner.Memory()
}

// Mission returns the current mission context.
// This is a pass-through operation without additional tracing.
func (h *TracedAgentHarness) Mission() harness.MissionContext {
	return h.inner.Mission()
}

// Target returns the current target information.
// This is a pass-through operation without additional tracing.
func (h *TracedAgentHarness) Target() harness.TargetInfo {
	return h.inner.Target()
}

// ListTools returns all available tool descriptors.
// This is a pass-through operation without additional tracing.
func (h *TracedAgentHarness) ListTools() []harness.ToolDescriptor {
	return h.inner.ListTools()
}

// ListPlugins returns all available plugin descriptors.
// This is a pass-through operation without additional tracing.
func (h *TracedAgentHarness) ListPlugins() []harness.PluginDescriptor {
	return h.inner.ListPlugins()
}

// ListAgents returns all available agent descriptors.
// This is a pass-through operation without additional tracing.
func (h *TracedAgentHarness) ListAgents() []harness.AgentDescriptor {
	return h.inner.ListAgents()
}

// Tracer returns the OpenTelemetry tracer for this harness.
func (h *TracedAgentHarness) Tracer() trace.Tracer {
	return h.tracer
}

// Logger returns the underlying slog.Logger.
// Note: This returns the base logger, not the TracedLogger wrapper.
func (h *TracedAgentHarness) Logger() *slog.Logger {
	if h.logger != nil {
		return h.logger.logger
	}
	return h.inner.Logger()
}

// Metrics returns the metrics recorder for this harness.
func (h *TracedAgentHarness) Metrics() harness.MetricsRecorder {
	return h.metrics
}

// TokenUsage returns the token usage tracker.
// This is a pass-through operation without additional tracing.
func (h *TracedAgentHarness) TokenUsage() *llm.TokenTracker {
	return h.inner.TokenUsage()
}

// formatMessagesForLogging converts messages to a human-readable format for logging.
// This is used when capturePrompt is enabled.
func formatMessagesForLogging(messages []llm.Message) string {
	if len(messages) == 0 {
		return ""
	}

	result := ""
	for i, msg := range messages {
		if i > 0 {
			result += "\n"
		}
		result += fmt.Sprintf("[%s] %s", msg.Role, msg.Content)
	}

	return result
}

// Ensure TracedAgentHarness implements AgentHarness at compile time
var _ harness.AgentHarness = (*TracedAgentHarness)(nil)
