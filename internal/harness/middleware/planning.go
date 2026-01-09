package middleware

import (
	"context"
	"sync"

	"github.com/zero-day-ai/gibson/internal/llm"
)

// PlanningContext tracks planning state and token budget across operations.
// It is safe for concurrent use by multiple goroutines.
type PlanningContext struct {
	mu            sync.Mutex
	TokenBudget   int64
	TokensUsed    int64
	Hints         []AgentHint
	MaxIterations int
	Iteration     int
}

// AgentHint represents a hint provided by an agent for planning purposes.
type AgentHint struct {
	AgentName string
	Hint      string
	Priority  int
}

// planningContextKey is the context key for PlanningContext
type planningContextKey struct{}

// WithPlanningContext returns a new context with the planning context attached.
func WithPlanningContext(ctx context.Context, planCtx *PlanningContext) context.Context {
	return context.WithValue(ctx, planningContextKey{}, planCtx)
}

// GetPlanningContext retrieves the planning context from the context.
// Returns nil if no planning context is present.
func GetPlanningContext(ctx context.Context) *PlanningContext {
	planCtx, ok := ctx.Value(planningContextKey{}).(*PlanningContext)
	if !ok {
		return nil
	}
	return planCtx
}

// UpdateFromResponse updates token usage from a response.
// This method is thread-safe.
func (pc *PlanningContext) UpdateFromResponse(result any) {
	if pc == nil {
		return
	}

	// Extract token usage from response
	usage := extractTokenUsage(result)
	if usage == nil {
		return
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.TokensUsed += int64(usage.TotalTokens)
}

// RemainingTokens returns the number of tokens remaining in the budget.
// Returns a negative value if over budget.
// This method is thread-safe.
func (pc *PlanningContext) RemainingTokens() int64 {
	if pc == nil {
		return 0
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	return pc.TokenBudget - pc.TokensUsed
}

// IsOverBudget returns true if token usage has exceeded the budget.
// This method is thread-safe.
func (pc *PlanningContext) IsOverBudget() bool {
	if pc == nil {
		return false
	}

	return pc.RemainingTokens() < 0
}

// AddHint adds an agent hint to the planning context.
// This method is thread-safe.
func (pc *PlanningContext) AddHint(hint AgentHint) {
	if pc == nil {
		return
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.Hints = append(pc.Hints, hint)
}

// GetHints returns a copy of all hints sorted by priority (highest first).
// This method is thread-safe.
func (pc *PlanningContext) GetHints() []AgentHint {
	if pc == nil {
		return nil
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Create a copy to avoid race conditions
	hints := make([]AgentHint, len(pc.Hints))
	copy(hints, pc.Hints)

	// Sort by priority (highest first)
	for i := 0; i < len(hints)-1; i++ {
		for j := i + 1; j < len(hints); j++ {
			if hints[j].Priority > hints[i].Priority {
				hints[i], hints[j] = hints[j], hints[i]
			}
		}
	}

	return hints
}

// extractTokenUsage attempts to extract token usage from a response.
// It handles llm.CompletionResponse and other common response types.
// Returns nil if token usage cannot be extracted.
func extractTokenUsage(result any) *llm.CompletionTokenUsage {
	if result == nil {
		return nil
	}

	// Check for CompletionResponse
	switch resp := result.(type) {
	case *llm.CompletionResponse:
		return &resp.Usage
	case llm.CompletionResponse:
		return &resp.Usage
	default:
		// Cannot extract token usage from this type
		return nil
	}
}

// PlanningMiddleware creates a middleware that injects planning context and tracks token usage.
// If planCtx is nil, the middleware operates in no-op mode, allowing operations to proceed normally.
func PlanningMiddleware(planCtx *PlanningContext) Middleware {
	return func(next Operation) Operation {
		return func(ctx context.Context, req any) (any, error) {
			// No-op if no planning context provided
			if planCtx == nil {
				return next(ctx, req)
			}

			// Inject planning context into operation context
			ctx = WithPlanningContext(ctx, planCtx)

			// Execute the operation
			result, err := next(ctx, req)

			// Update token usage from response
			planCtx.UpdateFromResponse(result)

			// Warn if over budget (but don't block the operation)
			if planCtx.IsOverBudget() {
				// Note: In a real implementation, we might want to log this warning
				// For now, we just silently track it. The caller can check IsOverBudget()
			}

			return result, err
		}
	}
}
