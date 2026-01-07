package harness

import (
	"context"
	"sync"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
)

// PlanningHarnessWrapper wraps an existing AgentHarness to provide planning-aware
// functionality. It tracks step budget consumption, collects agent hints, and
// provides planning context to agents while delegating all harness operations
// to the inner implementation.
//
// This wrapper is transparent to agents - all standard harness operations work
// as expected. The additional planning methods allow planning-aware agents to:
//   - Query their current planning context (budget, position in plan)
//   - Report hints to the step scorer for better decision making
//   - Signal when replanning may be needed
//
// The wrapper tracks token usage separately from the inner harness to provide
// per-step budget enforcement. Token consumption is recorded for Complete and
// CompleteWithTools operations.
//
// Thread-safety: All methods are safe for concurrent use. The wrapper protects
// mutable state with a mutex.
type PlanningHarnessWrapper struct {
	// inner is the wrapped harness that handles actual operations
	inner AgentHarness

	// planContext is the planning context for this step (may be nil if planning disabled)
	planContext *PlanningContext

	// hints collects agent-provided hints during execution
	hints *StepHints

	// replanSignaled indicates if the agent signaled replanning should occur
	replanSignaled bool
	replanReason   string

	// tokensBudget is the allocated token budget for this step
	tokensBudget int

	// tokensUsed tracks tokens consumed against the step budget
	tokensUsed int

	// mu protects mutable state (hints, replanSignaled, replanReason, tokensUsed)
	mu sync.Mutex
}

// NewPlanningHarnessWrapper creates a new planning-aware harness wrapper.
//
// Parameters:
//   - inner: The underlying harness implementation to wrap
//   - planCtx: The planning context for this step (can be nil if planning disabled)
//
// The wrapper creates an empty StepHints collection and initializes token tracking
// based on the planning context's step budget. If planCtx is nil, the wrapper
// operates in pass-through mode with no planning features active.
//
// Example:
//
//	inner := harness.NewDefaultAgentHarness(...)
//	ctx := &PlanningContext{StepBudget: 5000, ...}
//	wrapper := NewPlanningHarnessWrapper(inner, ctx)
//	result, err := agent.Execute(ctx, wrapper, task)
func NewPlanningHarnessWrapper(inner AgentHarness, planCtx *PlanningContext) *PlanningHarnessWrapper {
	tokensBudget := 0
	if planCtx != nil {
		tokensBudget = planCtx.StepBudget
	}

	return &PlanningHarnessWrapper{
		inner:          inner,
		planContext:    planCtx,
		hints:          NewStepHints(),
		replanSignaled: false,
		replanReason:   "",
		tokensBudget:   tokensBudget,
		tokensUsed:     0,
	}
}

// ─── Planning Methods ────────────────────────────────────────────────────────

// PlanContext returns the planning context for this step.
// Returns nil if planning is disabled for this mission.
//
// Agents can use this to:
//   - Check their position in the overall plan
//   - See what steps remain
//   - Access the original mission goal for context
//   - Query their allocated token budget
//
// Example:
//
//	if ctx := harness.PlanContext(); ctx != nil {
//	    log.Printf("Step %d of %d, %d steps remaining",
//	        ctx.CurrentPosition, ctx.TotalSteps, len(ctx.RemainingSteps))
//	}
func (w *PlanningHarnessWrapper) PlanContext() *PlanningContext {
	return w.planContext
}

// GetStepBudget returns the token budget allocated for this step.
// Returns 0 if planning is disabled or no budget is set (unlimited).
//
// Agents can use this to:
//   - Check remaining budget before expensive operations
//   - Make cost-aware decisions (e.g., use cheaper models when low on budget)
//   - Avoid budget exhaustion mid-operation
//
// Example:
//
//	budget := harness.GetStepBudget()
//	if budget > 0 && budget-tokensUsed < 1000 {
//	    // Use a cheaper model or reduce max tokens
//	}
func (w *PlanningHarnessWrapper) GetStepBudget() int {
	return w.tokensBudget
}

// SignalReplanRecommended records that the agent recommends replanning.
// This signals to the orchestration layer that the current plan may not be
// optimal and should be reconsidered.
//
// Parameters:
//   - ctx: Context for cancellation and tracing
//   - reason: Explanation of why replanning is recommended
//
// Reasons might include:
//   - "Target uses authentication method not covered by current plan"
//   - "Discovered critical vulnerability, subsequent steps may be unnecessary"
//   - "All planned attack vectors blocked, need alternative approach"
//
// The signal is collected and passed to the step scorer during step evaluation.
// The scorer will decide whether to trigger tactical replanning based on this
// signal and other factors.
//
// This method is idempotent - calling it multiple times updates the reason to
// the most recent value.
//
// Example:
//
//	if targetRequiresOAuth && !planIncludesOAuthSteps {
//	    harness.SignalReplanRecommended(ctx,
//	        "Target requires OAuth authentication, current plan assumes basic auth")
//	}
func (w *PlanningHarnessWrapper) SignalReplanRecommended(ctx context.Context, reason string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.replanSignaled = true
	w.replanReason = reason

	return nil
}

// ReportStepHints records agent hints for the step scorer.
// Hints provide the scorer with context about the agent's execution,
// confidence level, and recommendations for subsequent steps.
//
// Parameters:
//   - ctx: Context for cancellation and tracing
//   - hints: Agent-provided hints (confidence, suggestions, findings, etc.)
//
// The hints are used by the step scorer to:
//   - Assess whether the step was successful
//   - Decide if replanning is needed
//   - Choose optimal next steps
//   - Track important discoveries
//
// Agents should call this method near the end of their execution to provide
// maximum context to the planning system.
//
// Example:
//
//	hints := harness.NewStepHints().
//	    WithConfidence(0.95).
//	    WithSuggestion("exploit_sqli").
//	    WithKeyFinding("Found SQLi in search parameter")
//	harness.ReportStepHints(ctx, hints)
func (w *PlanningHarnessWrapper) ReportStepHints(ctx context.Context, hints *StepHints) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Store the hints for later retrieval by the orchestration layer
	w.hints = hints

	return nil
}

// ─── Getter Methods for Orchestration ────────────────────────────────────────

// GetCollectedHints returns the hints collected during execution.
// This is used by the orchestration layer to pass hints to the scorer.
//
// Returns the most recent hints provided via ReportStepHints, or a default
// StepHints instance with neutral values if the agent never reported hints.
//
// This method is thread-safe and can be called while the agent is still executing.
func (w *PlanningHarnessWrapper) GetCollectedHints() *StepHints {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.hints
}

// IsReplanSignaled returns true if the agent signaled replanning.
// Returns the signal status and the reason provided by the agent.
//
// Returns:
//   - signaled: true if the agent called SignalReplanRecommended
//   - reason: the explanation provided by the agent (empty if not signaled)
//
// This method is thread-safe and can be called while the agent is still executing.
func (w *PlanningHarnessWrapper) IsReplanSignaled() (bool, string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.replanSignaled, w.replanReason
}

// GetTokensUsed returns the number of tokens consumed during this step.
// This tracks tokens used by Complete and CompleteWithTools calls made through
// this wrapper.
//
// This method is thread-safe and can be called while the agent is still executing.
func (w *PlanningHarnessWrapper) GetTokensUsed() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.tokensUsed
}

// ─── Delegated Methods ───────────────────────────────────────────────────────
//
// All AgentHarness methods are delegated to the inner harness implementation.
// Complete and CompleteWithTools track token usage for step budget enforcement.

// Complete delegates to inner harness and tracks token usage.
func (w *PlanningHarnessWrapper) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...CompletionOption) (*llm.CompletionResponse, error) {
	resp, err := w.inner.Complete(ctx, slot, messages, opts...)
	if err != nil {
		return nil, err
	}

	// Track token usage for step budget
	w.mu.Lock()
	w.tokensUsed += resp.Usage.TotalTokens
	w.mu.Unlock()

	return resp, nil
}

// CompleteWithTools delegates to inner harness and tracks token usage.
func (w *PlanningHarnessWrapper) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...CompletionOption) (*llm.CompletionResponse, error) {
	resp, err := w.inner.CompleteWithTools(ctx, slot, messages, tools, opts...)
	if err != nil {
		return nil, err
	}

	// Track token usage for step budget
	w.mu.Lock()
	w.tokensUsed += resp.Usage.TotalTokens
	w.mu.Unlock()

	return resp, nil
}

// Stream delegates to inner harness.
// Note: Token tracking for streaming completions is not implemented in this version.
// The orchestration layer should avoid streaming operations when strict budget
// enforcement is required.
func (w *PlanningHarnessWrapper) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...CompletionOption) (<-chan llm.StreamChunk, error) {
	return w.inner.Stream(ctx, slot, messages, opts...)
}

// CallTool delegates to inner harness.
func (w *PlanningHarnessWrapper) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return w.inner.CallTool(ctx, name, input)
}

// ListTools delegates to inner harness.
func (w *PlanningHarnessWrapper) ListTools() []ToolDescriptor {
	return w.inner.ListTools()
}

// QueryPlugin delegates to inner harness.
func (w *PlanningHarnessWrapper) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	return w.inner.QueryPlugin(ctx, name, method, params)
}

// ListPlugins delegates to inner harness.
func (w *PlanningHarnessWrapper) ListPlugins() []PluginDescriptor {
	return w.inner.ListPlugins()
}

// DelegateToAgent delegates to inner harness.
func (w *PlanningHarnessWrapper) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	return w.inner.DelegateToAgent(ctx, name, task)
}

// ListAgents delegates to inner harness.
func (w *PlanningHarnessWrapper) ListAgents() []AgentDescriptor {
	return w.inner.ListAgents()
}

// SubmitFinding delegates to inner harness.
func (w *PlanningHarnessWrapper) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return w.inner.SubmitFinding(ctx, finding)
}

// GetFindings delegates to inner harness.
func (w *PlanningHarnessWrapper) GetFindings(ctx context.Context, filter FindingFilter) ([]agent.Finding, error) {
	return w.inner.GetFindings(ctx, filter)
}

// Memory delegates to inner harness.
func (w *PlanningHarnessWrapper) Memory() memory.MemoryStore {
	return w.inner.Memory()
}

// Mission delegates to inner harness.
func (w *PlanningHarnessWrapper) Mission() MissionContext {
	return w.inner.Mission()
}

// Target delegates to inner harness.
func (w *PlanningHarnessWrapper) Target() TargetInfo {
	return w.inner.Target()
}

// Tracer delegates to inner harness.
func (w *PlanningHarnessWrapper) Tracer() trace.Tracer {
	return w.inner.Tracer()
}

// Logger delegates to inner harness.
func (w *PlanningHarnessWrapper) Logger() *slog.Logger {
	return w.inner.Logger()
}

// Metrics delegates to inner harness.
func (w *PlanningHarnessWrapper) Metrics() MetricsRecorder {
	return w.inner.Metrics()
}

// TokenUsage delegates to inner harness.
func (w *PlanningHarnessWrapper) TokenUsage() *llm.TokenTracker {
	return w.inner.TokenUsage()
}

// Ensure PlanningHarnessWrapper implements AgentHarness
var _ AgentHarness = (*PlanningHarnessWrapper)(nil)

// Ensure PlanningHarnessWrapper implements PlanningContextProvider
var _ PlanningContextProvider = (*PlanningHarnessWrapper)(nil)
