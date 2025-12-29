package harness

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/plugin"
	"github.com/zero-day-ai/gibson/internal/tool"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/trace"
)

// DefaultAgentHarness is the production implementation of the AgentHarness interface.
// It provides agents with access to all framework capabilities including LLM operations,
// tool execution, plugin queries, sub-agent delegation, finding management, memory storage,
// and observability primitives.
//
// The harness orchestrates interactions between agents and the Gibson platform,
// handling:
//   - LLM provider management and slot-based model selection
//   - Tool registration, validation, and execution
//   - Plugin lifecycle and communication
//   - Sub-agent discovery and delegation
//   - Finding storage and querying
//   - Memory tier coordination (working, mission, long-term)
//   - Distributed tracing and structured logging
//   - Metrics collection and token usage tracking
//
// All methods are safe for concurrent use. The harness ensures thread-safety
// for shared resources and coordinates access across multiple agents.
type DefaultAgentHarness struct {
	// LLM components
	slotManager llm.SlotManager
	llmRegistry llm.LLMRegistry

	// Tool and plugin registries
	toolRegistry   tool.ToolRegistry
	pluginRegistry plugin.PluginRegistry

	// Agent registry for delegation
	agentRegistry agent.AgentRegistry

	// Memory and storage
	memoryStore  memory.MemoryManager
	findingStore FindingStore

	// Factory for creating child harnesses during delegation
	factory HarnessFactory

	// Context information
	missionCtx MissionContext
	targetInfo TargetInfo

	// Observability
	tracer     trace.Tracer
	logger     *slog.Logger
	metrics    MetricsRecorder
	tokenUsage llm.TokenTracker
}

// Ensure DefaultAgentHarness implements AgentHarness
var _ AgentHarness = (*DefaultAgentHarness)(nil)

// Ensure DefaultAgentHarness implements agent.AgentHarness (the minimal interface)
var _ agent.AgentHarness = (*DefaultAgentHarness)(nil)

// ────────────────────────────────────────────────────────────────────────────
// LLM Access Methods
// ────────────────────────────────────────────────────────────────────────────

// Complete performs a synchronous LLM completion using the specified slot.
func (h *DefaultAgentHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...CompletionOption) (*llm.CompletionResponse, error) {
	// Create span for distributed tracing
	ctx, span := h.tracer.Start(ctx, "harness.Complete")
	defer span.End()

	// Apply completion options
	options := applyOptions(opts...)

	// Create slot definition for the named slot
	slotDef := agent.NewSlotDefinition(slot, "LLM slot", true)

	// Resolve slot to provider and model
	provider, modelInfo, err := h.slotManager.ResolveSlot(ctx, slotDef, nil)
	if err != nil {
		h.logger.Error("failed to resolve LLM slot",
			"slot", slot,
			"error", err)
		return nil, types.WrapError(
			ErrHarnessCompletionFailed,
			fmt.Sprintf("failed to resolve slot %s", slot),
			err,
		)
	}

	// Build completion request
	req := llm.CompletionRequest{
		Model:    modelInfo.Name,
		Messages: messages,
	}

	// Apply options to request
	if options.Temperature != nil {
		req.Temperature = *options.Temperature
	}
	if options.MaxTokens != nil {
		req.MaxTokens = *options.MaxTokens
	}
	if options.TopP != nil {
		req.TopP = *options.TopP
	}
	if options.StopSequences != nil {
		req.StopSequences = options.StopSequences
	}
	if options.SystemPrompt != nil && *options.SystemPrompt != "" {
		// Prepend system message if provided
		req.Messages = append([]llm.Message{
			llm.NewSystemMessage(*options.SystemPrompt),
		}, req.Messages...)
	}

	// Execute completion
	resp, err := provider.Complete(ctx, req)
	if err != nil {
		h.logger.Error("LLM completion failed",
			"slot", slot,
			"provider", provider.Name(),
			"model", modelInfo.Name,
			"error", err)
		return nil, types.WrapError(
			ErrHarnessCompletionFailed,
			"LLM completion failed",
			err,
		)
	}

	// Track token usage
	scope := llm.UsageScope{
		MissionID: h.missionCtx.ID,
		AgentName: h.missionCtx.CurrentAgent,
		SlotName:  slot,
	}
	tokenUsage := llm.TokenUsage{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}
	err = h.tokenUsage.RecordUsage(scope, provider.Name(), resp.Model, tokenUsage)
	if err != nil {
		h.logger.Warn("failed to record token usage",
			"error", err)
		// Don't fail the request if tracking fails
	}

	// Record metrics
	h.metrics.RecordCounter("llm.completions", 1, map[string]string{
		"slot":     slot,
		"provider": provider.Name(),
		"model":    resp.Model,
		"status":   "success",
	})
	h.metrics.RecordCounter("llm.tokens.input", int64(resp.Usage.PromptTokens), map[string]string{
		"slot":     slot,
		"provider": provider.Name(),
		"model":    resp.Model,
	})
	h.metrics.RecordCounter("llm.tokens.output", int64(resp.Usage.CompletionTokens), map[string]string{
		"slot":     slot,
		"provider": provider.Name(),
		"model":    resp.Model,
	})

	h.logger.Debug("LLM completion successful",
		"slot", slot,
		"provider", provider.Name(),
		"model", resp.Model,
		"input_tokens", resp.Usage.PromptTokens,
		"output_tokens", resp.Usage.CompletionTokens)

	return resp, nil
}

// CompleteWithTools performs a completion with tool-calling capabilities.
func (h *DefaultAgentHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...CompletionOption) (*llm.CompletionResponse, error) {
	// Create span for distributed tracing
	ctx, span := h.tracer.Start(ctx, "harness.CompleteWithTools")
	defer span.End()

	// Apply completion options
	options := applyOptions(opts...)

	// Create slot definition for the named slot
	slotDef := agent.NewSlotDefinition(slot, "LLM slot", true)

	// Resolve slot to provider and model
	provider, modelInfo, err := h.slotManager.ResolveSlot(ctx, slotDef, nil)
	if err != nil {
		h.logger.Error("failed to resolve LLM slot",
			"slot", slot,
			"error", err)
		return nil, types.WrapError(
			ErrHarnessCompletionFailed,
			fmt.Sprintf("failed to resolve slot %s", slot),
			err,
		)
	}

	// Build completion request
	req := llm.CompletionRequest{
		Model:    modelInfo.Name,
		Messages: messages,
	}

	// Apply options to request
	if options.Temperature != nil {
		req.Temperature = *options.Temperature
	}
	if options.MaxTokens != nil {
		req.MaxTokens = *options.MaxTokens
	}
	if options.TopP != nil {
		req.TopP = *options.TopP
	}
	if options.StopSequences != nil {
		req.StopSequences = options.StopSequences
	}
	if options.SystemPrompt != nil && *options.SystemPrompt != "" {
		req.Messages = append([]llm.Message{
			llm.NewSystemMessage(*options.SystemPrompt),
		}, req.Messages...)
	}

	// Execute completion with tools
	resp, err := provider.CompleteWithTools(ctx, req, tools)
	if err != nil {
		h.logger.Error("LLM completion with tools failed",
			"slot", slot,
			"provider", provider.Name(),
			"model", modelInfo.Name,
			"error", err)
		return nil, types.WrapError(
			ErrHarnessCompletionFailed,
			"LLM completion with tools failed",
			err,
		)
	}

	// Track token usage
	scope := llm.UsageScope{
		MissionID: h.missionCtx.ID,
		AgentName: h.missionCtx.CurrentAgent,
		SlotName:  slot,
	}
	tokenUsage := llm.TokenUsage{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}
	err = h.tokenUsage.RecordUsage(scope, provider.Name(), resp.Model, tokenUsage)
	if err != nil {
		h.logger.Warn("failed to record token usage",
			"error", err)
	}

	// Record metrics
	h.metrics.RecordCounter("llm.completions.with_tools", 1, map[string]string{
		"slot":     slot,
		"provider": provider.Name(),
		"model":    resp.Model,
		"status":   "success",
	})
	h.metrics.RecordCounter("llm.tokens.input", int64(resp.Usage.PromptTokens), map[string]string{
		"slot":     slot,
		"provider": provider.Name(),
		"model":    resp.Model,
	})
	h.metrics.RecordCounter("llm.tokens.output", int64(resp.Usage.CompletionTokens), map[string]string{
		"slot":     slot,
		"provider": provider.Name(),
		"model":    resp.Model,
	})

	h.logger.Debug("LLM completion with tools successful",
		"slot", slot,
		"provider", provider.Name(),
		"model", resp.Model,
		"tool_calls", len(resp.Message.ToolCalls),
		"input_tokens", resp.Usage.PromptTokens,
		"output_tokens", resp.Usage.CompletionTokens)

	return resp, nil
}

// Stream performs a streaming LLM completion, returning chunks as they arrive.
func (h *DefaultAgentHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...CompletionOption) (<-chan llm.StreamChunk, error) {
	// Create span for distributed tracing
	ctx, span := h.tracer.Start(ctx, "harness.Stream")
	defer span.End()

	// Apply completion options
	options := applyOptions(opts...)

	// Create slot definition for the named slot
	slotDef := agent.NewSlotDefinition(slot, "LLM slot", true)

	// Resolve slot to provider and model
	provider, modelInfo, err := h.slotManager.ResolveSlot(ctx, slotDef, nil)
	if err != nil {
		h.logger.Error("failed to resolve LLM slot",
			"slot", slot,
			"error", err)
		return nil, types.WrapError(
			ErrHarnessCompletionFailed,
			fmt.Sprintf("failed to resolve slot %s", slot),
			err,
		)
	}

	// Build completion request
	req := llm.CompletionRequest{
		Model:    modelInfo.Name,
		Messages: messages,
	}

	// Apply options to request
	if options.Temperature != nil {
		req.Temperature = *options.Temperature
	}
	if options.MaxTokens != nil {
		req.MaxTokens = *options.MaxTokens
	}
	if options.TopP != nil {
		req.TopP = *options.TopP
	}
	if options.StopSequences != nil {
		req.StopSequences = options.StopSequences
	}
	if options.SystemPrompt != nil && *options.SystemPrompt != "" {
		req.Messages = append([]llm.Message{
			llm.NewSystemMessage(*options.SystemPrompt),
		}, req.Messages...)
	}

	// Execute streaming completion
	chunks, err := provider.Stream(ctx, req)
	if err != nil {
		h.logger.Error("LLM stream failed",
			"slot", slot,
			"provider", provider.Name(),
			"model", modelInfo.Name,
			"error", err)
		return nil, types.WrapError(
			ErrHarnessCompletionFailed,
			"LLM stream failed",
			err,
		)
	}

	// Record metrics
	h.metrics.RecordCounter("llm.streams", 1, map[string]string{
		"slot":     slot,
		"provider": provider.Name(),
		"model":    modelInfo.Name,
		"status":   "started",
	})

	h.logger.Debug("LLM stream started",
		"slot", slot,
		"provider", provider.Name(),
		"model", modelInfo.Name)

	// Wrap channel to record stream completion
	wrappedChan := make(chan llm.StreamChunk)
	go func() {
		defer close(wrappedChan)

		for chunk := range chunks {
			wrappedChan <- chunk

			// If this is the final chunk, record completion metrics
			// Note: Token usage tracking for streaming requires provider-specific support
			// and is typically only available after the stream completes
			if chunk.FinishReason != "" {
				// Record completion metrics
				h.metrics.RecordCounter("llm.streams.completed", 1, map[string]string{
					"slot":     slot,
					"provider": provider.Name(),
					"model":    modelInfo.Name,
				})

				h.logger.Debug("LLM stream completed",
					"slot", slot,
					"provider", provider.Name(),
					"model", modelInfo.Name,
					"finish_reason", string(chunk.FinishReason))
			}
		}
	}()

	return wrappedChan, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Tool Execution Methods
// ────────────────────────────────────────────────────────────────────────────

// CallTool executes a registered tool by name with the given input parameters.
func (h *DefaultAgentHarness) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	// Create span for distributed tracing
	ctx, span := h.tracer.Start(ctx, "harness.CallTool")
	defer span.End()

	h.logger.Debug("calling tool",
		"tool", name,
		"input", input)

	// Get tool from registry
	t, err := h.toolRegistry.Get(name)
	if err != nil {
		h.logger.Error("tool not found",
			"tool", name,
			"error", err)
		return nil, types.WrapError(
			ErrHarnessToolExecutionFailed,
			fmt.Sprintf("tool not found: %s", name),
			err,
		)
	}

	// Execute tool
	output, err := t.Execute(ctx, input)
	if err != nil {
		h.logger.Error("tool execution failed",
			"tool", name,
			"error", err)

		// Record failure metrics
		h.metrics.RecordCounter("tools.executions", 1, map[string]string{
			"tool":   name,
			"status": "failed",
		})

		return nil, types.WrapError(
			ErrHarnessToolExecutionFailed,
			fmt.Sprintf("tool execution failed: %s", name),
			err,
		)
	}

	// Record success metrics
	h.metrics.RecordCounter("tools.executions", 1, map[string]string{
		"tool":   name,
		"status": "success",
	})

	h.logger.Debug("tool execution successful",
		"tool", name,
		"output", output)

	return output, nil
}

// ListTools returns descriptors for all registered tools.
func (h *DefaultAgentHarness) ListTools() []ToolDescriptor {
	toolDescriptors := h.toolRegistry.List()

	// Convert from tool.ToolDescriptor to harness.ToolDescriptor
	descriptors := make([]ToolDescriptor, len(toolDescriptors))
	for i, t := range toolDescriptors {
		descriptors[i] = ToolDescriptor{
			Name:         t.Name,
			Description:  t.Description,
			Version:      t.Version,
			Tags:         t.Tags,
			InputSchema:  t.InputSchema,
			OutputSchema: t.OutputSchema,
		}
	}

	return descriptors
}

// ────────────────────────────────────────────────────────────────────────────
// Plugin Access Methods
// ────────────────────────────────────────────────────────────────────────────

// QueryPlugin calls a method on a registered plugin with the given parameters.
func (h *DefaultAgentHarness) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	// Create span for distributed tracing
	ctx, span := h.tracer.Start(ctx, "harness.QueryPlugin")
	defer span.End()

	h.logger.Debug("querying plugin",
		"plugin", name,
		"method", method,
		"params", params)

	// Get plugin from registry
	p, err := h.pluginRegistry.Get(name)
	if err != nil {
		h.logger.Error("plugin not found",
			"plugin", name,
			"error", err)
		return nil, types.WrapError(
			ErrHarnessPluginNotFound,
			fmt.Sprintf("plugin not found: %s", name),
			err,
		)
	}

	// Query plugin
	result, err := p.Query(ctx, method, params)
	if err != nil {
		h.logger.Error("plugin query failed",
			"plugin", name,
			"method", method,
			"error", err)

		// Record failure metrics
		h.metrics.RecordCounter("plugins.queries", 1, map[string]string{
			"plugin": name,
			"method": method,
			"status": "failed",
		})

		return nil, types.WrapError(
			ErrHarnessPluginMethodNotFound,
			fmt.Sprintf("plugin query failed: %s.%s", name, method),
			err,
		)
	}

	// Record success metrics
	h.metrics.RecordCounter("plugins.queries", 1, map[string]string{
		"plugin": name,
		"method": method,
		"status": "success",
	})

	h.logger.Debug("plugin query successful",
		"plugin", name,
		"method", method)

	return result, nil
}

// ListPlugins returns descriptors for all registered plugins.
func (h *DefaultAgentHarness) ListPlugins() []PluginDescriptor {
	pluginDescriptors := h.pluginRegistry.List()

	// Convert from plugin.PluginDescriptor to harness.PluginDescriptor
	descriptors := make([]PluginDescriptor, len(pluginDescriptors))
	for i, p := range pluginDescriptors {
		descriptors[i] = PluginDescriptor{
			Name:       p.Name,
			Version:    p.Version,
			Methods:    p.Methods,
			IsExternal: p.IsExternal,
			Status:     p.Status,
		}
	}

	return descriptors
}

// ────────────────────────────────────────────────────────────────────────────
// Sub-Agent Delegation Methods
// ────────────────────────────────────────────────────────────────────────────

// DelegateToAgent delegates a task to another registered agent for execution.
func (h *DefaultAgentHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	// Create span for distributed tracing
	ctx, span := h.tracer.Start(ctx, "harness.DelegateToAgent")
	defer span.End()

	h.logger.Info("delegating to agent",
		"agent", name,
		"task_id", task.ID.String(),
		"task_name", task.Name)

	// Update mission context for child agent
	childMissionCtx := h.missionCtx
	childMissionCtx.CurrentAgent = name

	// Create child harness for the sub-agent
	childHarness, err := h.factory(ctx, childMissionCtx, h.targetInfo)
	if err != nil {
		h.logger.Error("failed to create child harness",
			"agent", name,
			"error", err)
		return agent.Result{}, types.WrapError(
			ErrHarnessDelegationFailed,
			"failed to create child harness",
			err,
		)
	}

	// Convert harness.AgentHarness to agent.AgentHarness
	// DefaultAgentHarness implements both interfaces, so this is a type assertion
	agentHarness, ok := childHarness.(agent.AgentHarness)
	if !ok {
		h.logger.Error("child harness does not implement agent.AgentHarness",
			"agent", name)
		return agent.Result{}, types.NewError(
			ErrHarnessDelegationFailed,
			"child harness does not implement agent.AgentHarness",
		)
	}

	// Use the registry's DelegateToAgent method which handles agent creation and execution
	result, err := h.agentRegistry.DelegateToAgent(ctx, name, task, agentHarness)
	if err != nil {
		h.logger.Error("agent execution failed",
			"agent", name,
			"task_id", task.ID.String(),
			"error", err)

		// Record failure metrics
		h.metrics.RecordCounter("agents.delegations", 1, map[string]string{
			"agent":  name,
			"status": "failed",
		})

		return result, types.WrapError(
			ErrHarnessDelegationFailed,
			fmt.Sprintf("agent execution failed: %s", name),
			err,
		)
	}

	// Submit findings from sub-agent to our finding store
	for _, finding := range result.Findings {
		err := h.SubmitFinding(ctx, finding)
		if err != nil {
			h.logger.Warn("failed to submit sub-agent finding",
				"agent", name,
				"finding", finding.Title,
				"error", err)
		}
	}

	// Record success metrics
	h.metrics.RecordCounter("agents.delegations", 1, map[string]string{
		"agent":  name,
		"status": "success",
	})
	h.metrics.RecordCounter("agents.findings_from_delegation", int64(len(result.Findings)), map[string]string{
		"agent": name,
	})

	h.logger.Info("agent execution completed",
		"agent", name,
		"task_id", task.ID.String(),
		"status", result.Status,
		"findings_count", len(result.Findings))

	return result, nil
}

// ListAgents returns descriptors for all registered agents.
func (h *DefaultAgentHarness) ListAgents() []AgentDescriptor {
	agentDescriptors := h.agentRegistry.List()

	// Convert from agent.AgentDescriptor to harness.AgentDescriptor
	descriptors := make([]AgentDescriptor, len(agentDescriptors))
	for i, a := range agentDescriptors {
		descriptors[i] = AgentDescriptor{
			Name:         a.Name,
			Version:      a.Version,
			Description:  a.Description,
			Capabilities: a.Capabilities,
			Slots:        a.Slots,
			IsExternal:   a.IsExternal,
		}
	}

	return descriptors
}

// ────────────────────────────────────────────────────────────────────────────
// Findings Management Methods
// ────────────────────────────────────────────────────────────────────────────

// SubmitFinding stores a security finding for the current mission.
func (h *DefaultAgentHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	// Create span for distributed tracing
	ctx, span := h.tracer.Start(ctx, "harness.SubmitFinding")
	defer span.End()

	h.logger.Info("submitting finding",
		"finding_id", finding.ID.String(),
		"title", finding.Title,
		"severity", finding.Severity,
		"confidence", finding.Confidence)

	// Store finding
	err := h.findingStore.Store(ctx, h.missionCtx.ID, finding)
	if err != nil {
		h.logger.Error("failed to submit finding",
			"finding_id", finding.ID.String(),
			"error", err)

		// Record failure metrics
		h.metrics.RecordCounter("findings.submissions", 1, map[string]string{
			"severity": string(finding.Severity),
			"status":   "failed",
		})

		return types.WrapError(
			ErrHarnessInvalidConfig,
			"failed to submit finding",
			err,
		)
	}

	// Record success metrics
	h.metrics.RecordCounter("findings.submissions", 1, map[string]string{
		"severity": string(finding.Severity),
		"status":   "success",
	})
	h.metrics.RecordCounter("findings.by_severity", 1, map[string]string{
		"severity": string(finding.Severity),
	})

	h.logger.Debug("finding submitted successfully",
		"finding_id", finding.ID.String(),
		"title", finding.Title)

	return nil
}

// GetFindings retrieves findings for the current mission, optionally filtered.
func (h *DefaultAgentHarness) GetFindings(ctx context.Context, filter FindingFilter) ([]agent.Finding, error) {
	// Create span for distributed tracing
	ctx, span := h.tracer.Start(ctx, "harness.GetFindings")
	defer span.End()

	h.logger.Debug("retrieving findings",
		"mission_id", h.missionCtx.ID.String())

	// Get findings from store
	findings, err := h.findingStore.Get(ctx, h.missionCtx.ID, filter)
	if err != nil {
		h.logger.Error("failed to get findings",
			"mission_id", h.missionCtx.ID.String(),
			"error", err)
		return nil, types.WrapError(
			ErrHarnessInvalidConfig,
			"failed to get findings",
			err,
		)
	}

	h.logger.Debug("findings retrieved",
		"mission_id", h.missionCtx.ID.String(),
		"count", len(findings))

	return findings, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Memory Access Methods
// ────────────────────────────────────────────────────────────────────────────

// Memory provides access to the unified memory store.
func (h *DefaultAgentHarness) Memory() memory.MemoryStore {
	return h.memoryStore
}

// ────────────────────────────────────────────────────────────────────────────
// Context Access Methods
// ────────────────────────────────────────────────────────────────────────────

// Mission returns the current mission context.
func (h *DefaultAgentHarness) Mission() MissionContext {
	return h.missionCtx
}

// Target returns information about the current target.
func (h *DefaultAgentHarness) Target() TargetInfo {
	return h.targetInfo
}

// ────────────────────────────────────────────────────────────────────────────
// Observability Methods
// ────────────────────────────────────────────────────────────────────────────

// Tracer returns the OpenTelemetry tracer for distributed tracing.
func (h *DefaultAgentHarness) Tracer() trace.Tracer {
	return h.tracer
}

// Logger returns the structured logger for this agent execution.
func (h *DefaultAgentHarness) Logger() *slog.Logger {
	return h.logger
}

// Metrics returns the metrics recorder for operational metrics.
func (h *DefaultAgentHarness) Metrics() MetricsRecorder {
	return h.metrics
}

// TokenUsage returns the token usage tracker for the current execution.
func (h *DefaultAgentHarness) TokenUsage() *llm.TokenTracker {
	return &h.tokenUsage
}

// ────────────────────────────────────────────────────────────────────────────
// Minimal agent.AgentHarness Interface Implementation
// ────────────────────────────────────────────────────────────────────────────

// ExecuteTool implements the minimal agent.AgentHarness interface method.
// It delegates to CallTool.
func (h *DefaultAgentHarness) ExecuteTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return h.CallTool(ctx, name, input)
}

// Log implements the minimal agent.AgentHarness interface method.
// It writes a structured log message using the harness logger.
func (h *DefaultAgentHarness) Log(level, message string, fields map[string]any) {
	attrs := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		attrs = append(attrs, k, v)
	}

	switch level {
	case "debug":
		h.logger.Debug(message, attrs...)
	case "info":
		h.logger.Info(message, attrs...)
	case "warn":
		h.logger.Warn(message, attrs...)
	case "error":
		h.logger.Error(message, attrs...)
	default:
		h.logger.Info(message, attrs...)
	}
}
