package harness

import (
	"context"
	"log/slog"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness/middleware"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"go.opentelemetry.io/otel/trace"
)

// MiddlewareHarness wraps an AgentHarness and routes operations through middleware.
type MiddlewareHarness struct {
	inner      AgentHarness
	middleware middleware.Middleware
}

// NewMiddlewareHarness creates a new MiddlewareHarness.
func NewMiddlewareHarness(inner AgentHarness, mw middleware.Middleware) *MiddlewareHarness {
	return &MiddlewareHarness{inner: inner, middleware: mw}
}

func (h *MiddlewareHarness) wrapOperation(op middleware.Operation) middleware.Operation {
	if h.middleware == nil {
		return op
	}
	return h.middleware(op)
}

func (h *MiddlewareHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...CompletionOption) (*llm.CompletionResponse, error) {
	ctx = middleware.WithOperationType(ctx, middleware.OpComplete)
	ctx = middleware.WithSlotName(ctx, slot)
	ctx = middleware.WithMissionContext(ctx, h.inner.Mission().ID.String(), h.inner.Mission().CurrentAgent)

	innerOp := func(ctx context.Context, req any) (any, error) {
		return h.inner.Complete(ctx, slot, messages, opts...)
	}

	result, err := h.wrapOperation(innerOp)(ctx, nil)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*llm.CompletionResponse), nil
}

func (h *MiddlewareHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...CompletionOption) (*llm.CompletionResponse, error) {
	ctx = middleware.WithOperationType(ctx, middleware.OpCompleteWithTools)
	ctx = middleware.WithSlotName(ctx, slot)
	ctx = middleware.WithMissionContext(ctx, h.inner.Mission().ID.String(), h.inner.Mission().CurrentAgent)

	innerOp := func(ctx context.Context, req any) (any, error) {
		return h.inner.CompleteWithTools(ctx, slot, messages, tools, opts...)
	}

	result, err := h.wrapOperation(innerOp)(ctx, nil)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*llm.CompletionResponse), nil
}

func (h *MiddlewareHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...CompletionOption) (<-chan llm.StreamChunk, error) {
	ctx = middleware.WithOperationType(ctx, middleware.OpStream)
	ctx = middleware.WithSlotName(ctx, slot)
	ctx = middleware.WithMissionContext(ctx, h.inner.Mission().ID.String(), h.inner.Mission().CurrentAgent)

	innerOp := func(ctx context.Context, req any) (any, error) {
		return h.inner.Stream(ctx, slot, messages, opts...)
	}

	result, err := h.wrapOperation(innerOp)(ctx, nil)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(<-chan llm.StreamChunk), nil
}

func (h *MiddlewareHarness) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	ctx = middleware.WithOperationType(ctx, middleware.OpCallTool)
	ctx = middleware.WithToolName(ctx, name)
	ctx = middleware.WithMissionContext(ctx, h.inner.Mission().ID.String(), h.inner.Mission().CurrentAgent)

	innerOp := func(ctx context.Context, req any) (any, error) {
		return h.inner.CallTool(ctx, name, input)
	}

	result, err := h.wrapOperation(innerOp)(ctx, input)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(map[string]any), nil
}

func (h *MiddlewareHarness) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	ctx = middleware.WithOperationType(ctx, middleware.OpQueryPlugin)
	ctx = middleware.WithPluginInfo(ctx, name, method)
	ctx = middleware.WithMissionContext(ctx, h.inner.Mission().ID.String(), h.inner.Mission().CurrentAgent)

	innerOp := func(ctx context.Context, req any) (any, error) {
		return h.inner.QueryPlugin(ctx, name, method, params)
	}

	return h.wrapOperation(innerOp)(ctx, params)
}

func (h *MiddlewareHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	ctx = middleware.WithOperationType(ctx, middleware.OpDelegateToAgent)
	ctx = middleware.WithAgentTargetName(ctx, name)
	ctx = middleware.WithMissionContext(ctx, h.inner.Mission().ID.String(), h.inner.Mission().CurrentAgent)

	innerOp := func(ctx context.Context, req any) (any, error) {
		return h.inner.DelegateToAgent(ctx, name, task)
	}

	result, err := h.wrapOperation(innerOp)(ctx, task)
	if err != nil {
		return agent.Result{}, err
	}
	return result.(agent.Result), nil
}

func (h *MiddlewareHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	ctx = middleware.WithOperationType(ctx, middleware.OpSubmitFinding)
	ctx = middleware.WithMissionContext(ctx, h.inner.Mission().ID.String(), h.inner.Mission().CurrentAgent)

	innerOp := func(ctx context.Context, req any) (any, error) {
		return nil, h.inner.SubmitFinding(ctx, finding)
	}

	_, err := h.wrapOperation(innerOp)(ctx, finding)
	return err
}

// Pass-through methods
func (h *MiddlewareHarness) GetFindings(ctx context.Context, filter FindingFilter) ([]agent.Finding, error) {
	return h.inner.GetFindings(ctx, filter)
}
func (h *MiddlewareHarness) Memory() memory.MemoryStore            { return h.inner.Memory() }
func (h *MiddlewareHarness) Mission() MissionContext               { return h.inner.Mission() }
func (h *MiddlewareHarness) Target() TargetInfo                    { return h.inner.Target() }
func (h *MiddlewareHarness) ListTools() []ToolDescriptor           { return h.inner.ListTools() }
func (h *MiddlewareHarness) ListPlugins() []PluginDescriptor       { return h.inner.ListPlugins() }
func (h *MiddlewareHarness) ListAgents() []AgentDescriptor         { return h.inner.ListAgents() }
func (h *MiddlewareHarness) Tracer() trace.Tracer                  { return h.inner.Tracer() }
func (h *MiddlewareHarness) Logger() *slog.Logger                  { return h.inner.Logger() }
func (h *MiddlewareHarness) Metrics() MetricsRecorder              { return h.inner.Metrics() }
func (h *MiddlewareHarness) TokenUsage() *llm.TokenTracker         { return h.inner.TokenUsage() }
func (h *MiddlewareHarness) MissionExecutionContext() MissionExecutionContextSDK {
	return h.inner.MissionExecutionContext()
}
func (h *MiddlewareHarness) GetMissionRunHistory(ctx context.Context) ([]MissionRunSummarySDK, error) {
	return h.inner.GetMissionRunHistory(ctx)
}
func (h *MiddlewareHarness) GetPreviousRunFindings(ctx context.Context, filter FindingFilter) ([]agent.Finding, error) {
	return h.inner.GetPreviousRunFindings(ctx, filter)
}
func (h *MiddlewareHarness) GetAllRunFindings(ctx context.Context, filter FindingFilter) ([]agent.Finding, error) {
	return h.inner.GetAllRunFindings(ctx, filter)
}

// agent.AgentHarness interface
func (h *MiddlewareHarness) ExecuteTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return h.CallTool(ctx, name, input)
}

func (h *MiddlewareHarness) Log(level, message string, fields map[string]any) {
	attrs := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	logger := h.Logger()
	switch level {
	case "debug":
		logger.Debug(message, attrs...)
	case "info":
		logger.Info(message, attrs...)
	case "warn":
		logger.Warn(message, attrs...)
	case "error":
		logger.Error(message, attrs...)
	default:
		logger.Info(message, attrs...)
	}
}

var _ AgentHarness = (*MiddlewareHarness)(nil)
var _ agent.AgentHarness = (*MiddlewareHarness)(nil)
