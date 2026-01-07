package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TacticalReplanner dynamically adjusts execution plans mid-execution based on step results.
// When a step fails or produces unexpected results, the replanner analyzes the situation,
// consults failure memory, and decides whether to continue, reorder, skip, retry, escalate,
// or fail the mission.
type TacticalReplanner interface {
	// Replan adjusts the remaining execution plan based on step results.
	//
	// The replanner:
	//   - Analyzes the trigger score and failure context
	//   - Checks failure memory to avoid retry loops
	//   - Generates a replanning decision (continue, reorder, skip, retry, escalate, fail)
	//   - Validates modified plans against bounds
	//   - Records the replan in failure memory
	//
	// Returns an error if:
	//   - Budget is exhausted for replanning
	//   - Modified plan violates bounds
	//   - Context is cancelled
	Replan(ctx context.Context, input ReplanInput) (*ReplanResult, error)
}

// ReplanInput contains all context needed for tactical replanning.
type ReplanInput struct {
	// CurrentPlan is the strategic plan being executed.
	CurrentPlan *StrategicPlan

	// TriggerScore is the step score that triggered replanning.
	TriggerScore *StepScore

	// FailedNodeID is the node that triggered the replan.
	FailedNodeID string

	// RemainingBudget is what's left to spend.
	RemainingBudget *BudgetAllocation

	// FailureMemory tracks what approaches have been tried.
	FailureMemory *FailureMemory

	// OriginalGoal is the immutable mission goal for anchoring decisions.
	OriginalGoal string

	// Bounds are the immutable workflow constraints.
	Bounds *PlanningBounds

	// ReplanCount is how many times we've replanned so far.
	ReplanCount int

	// MissionID is the unique identifier for the mission (for events).
	MissionID types.ID

	// EvalContext provides eval feedback context for replanning decisions (optional).
	// When eval mode is enabled, this contains eval scores and feedback that can
	// inform better replanning decisions.
	EvalContext string
}

// ReplanResult contains the replanning decision and modified plan.
type ReplanResult struct {
	// ModifiedPlan is the updated strategic plan (nil if action is "continue" or "fail").
	ModifiedPlan *StrategicPlan

	// Action is the replanning decision.
	Action ReplanAction

	// Rationale explains why this action was chosen.
	Rationale string

	// NewAttempt is the new approach record if action is "retry".
	NewAttempt *AttemptRecord
}

// ReplanAction represents the type of replanning decision.
type ReplanAction string

const (
	// ReplanActionContinue means keep executing the current plan as-is.
	ReplanActionContinue ReplanAction = "continue"

	// ReplanActionReorder means change the order of remaining steps.
	ReplanActionReorder ReplanAction = "reorder"

	// ReplanActionSkip means skip the failing node and continue.
	ReplanActionSkip ReplanAction = "skip"

	// ReplanActionRetry means retry the failing node with a different approach.
	ReplanActionRetry ReplanAction = "retry"

	// ReplanActionEscalate means require human intervention before continuing.
	ReplanActionEscalate ReplanAction = "escalate"

	// ReplanActionFail means abort the mission.
	ReplanActionFail ReplanAction = "fail"
)

// MaxReplans is the maximum number of replanning events allowed per mission.
// This prevents infinite adjustment cycles and ensures missions eventually
// terminate rather than getting stuck in replanning loops.
const MaxReplans = 3

// IsValid checks if the ReplanAction is a recognized action type.
func (a ReplanAction) IsValid() bool {
	switch a {
	case ReplanActionContinue,
		ReplanActionReorder,
		ReplanActionSkip,
		ReplanActionRetry,
		ReplanActionEscalate,
		ReplanActionFail:
		return true
	default:
		return false
	}
}

// String returns the string representation of the replan action.
func (a ReplanAction) String() string {
	return string(a)
}

// LLMTacticalReplanner implements TacticalReplanner using an LLM for intelligent
// mid-execution plan modifications. It analyzes step failures, consults failure memory
// to avoid retry loops, and generates context-aware replanning decisions within bounds.
//
// The replanner enforces:
//   - Maximum replan limit (default 3) to prevent infinite loops
//   - Replanning token budget (consumes from ReplanReserve)
//   - Failure memory checks to prevent retrying failed approaches
//   - Bounds validation for all plan modifications
//   - Original goal anchoring to prevent mission drift
//
// Implementation notes:
//   - Uses harness.Complete() for LLM access
//   - Tracks replan count and enforces maximum limit
//   - Validates retry attempts against FailureMemory.HasTried()
//   - Emits events for replan triggered/completed/rejected
//   - Falls back to "continue" on LLM failures (optimistic default)
type LLMTacticalReplanner struct {
	// harness provides access to LLM capabilities
	harness harness.AgentHarness

	// budgetManager tracks token usage (optional)
	budgetManager BudgetManager

	// eventEmitter publishes replanning events (optional)
	eventEmitter PlanningEventEmitter

	// maxReplans is the maximum number of replans allowed (default 3)
	maxReplans int

	// llmSlot is the LLM slot to use for replanning (default "primary")
	llmSlot string

	// pricingConfig provides model pricing information for cost calculations
	pricingConfig *llm.PricingConfig
}

// LLMTacticalReplannerOption is a functional option for configuring LLMTacticalReplanner.
type LLMTacticalReplannerOption func(*LLMTacticalReplanner)

// WithReplanBudget sets the budget manager for tracking replanning costs.
func WithReplanBudget(bm BudgetManager) LLMTacticalReplannerOption {
	return func(r *LLMTacticalReplanner) {
		r.budgetManager = bm
	}
}

// WithReplanEvents sets the event emitter for publishing replanning events.
func WithReplanEvents(emitter PlanningEventEmitter) LLMTacticalReplannerOption {
	return func(r *LLMTacticalReplanner) {
		r.eventEmitter = emitter
	}
}

// WithMaxReplans sets the maximum number of replans allowed.
func WithMaxReplans(max int) LLMTacticalReplannerOption {
	return func(r *LLMTacticalReplanner) {
		r.maxReplans = max
	}
}

// WithReplanSlot sets the LLM slot to use for replanning.
func WithReplanSlot(slot string) LLMTacticalReplannerOption {
	return func(r *LLMTacticalReplanner) {
		r.llmSlot = slot
	}
}

// NewLLMTacticalReplanner creates a new LLM-based tactical replanner with optional configuration.
//
// Parameters:
//   - harness: Agent harness providing LLM access
//   - opts: Optional configuration (budget, events, max replans, slot)
//
// Returns a configured replanner ready to handle mid-execution plan modifications.
func NewLLMTacticalReplanner(harness harness.AgentHarness, opts ...LLMTacticalReplannerOption) *LLMTacticalReplanner {
	replanner := &LLMTacticalReplanner{
		harness:       harness,
		maxReplans:    3,         // Default maximum replans
		llmSlot:       "primary", // Default LLM slot
		pricingConfig: llm.DefaultPricing(),
	}

	for _, opt := range opts {
		opt(replanner)
	}

	return replanner
}

// Replan implements TacticalReplanner.Replan using LLM-powered analysis.
//
// The replanning process:
//  1. Check replan count against maximum - return fail if exceeded
//  2. Check remaining budget - return continue if insufficient
//  3. Emit replan triggered event
//  4. Build replan prompt with original goal, current plan, failure context, memory
//  5. Query LLM for replanning decision
//  6. Parse and validate LLM response
//  7. Validate retry attempts against failure memory
//  8. Validate modified plans against bounds
//  9. Emit replan completed/rejected event
//  10. Return replanning result
//
// Returns an error if:
//   - LLM call fails and budget is exhausted (optimistic fallback otherwise)
//   - Response parsing fails
//   - Plan validation fails
//   - Context is cancelled
func (r *LLMTacticalReplanner) Replan(ctx context.Context, input ReplanInput) (*ReplanResult, error) {
	// Validate inputs
	if err := r.validateInput(input); err != nil {
		return nil, err
	}

	// Check if we've exceeded maximum replan count
	if input.ReplanCount >= r.maxReplans {
		r.emitReplanRejected(ctx, input, "maximum replan count exceeded")
		return &ReplanResult{
			Action:    ReplanActionFail,
			Rationale: fmt.Sprintf("Maximum replan count (%d) exceeded - aborting mission", r.maxReplans),
		}, nil
	}

	// Check if we have sufficient replanning budget
	if input.RemainingBudget != nil && input.RemainingBudget.ReplanReserve <= 0 {
		r.emitReplanRejected(ctx, input, "replanning budget exhausted")
		return &ReplanResult{
			Action:    ReplanActionContinue,
			Rationale: "No replanning budget remaining - continuing with current plan",
		}, nil
	}

	// Emit replan triggered event
	r.emitReplanTriggered(ctx, input)

	// Build replanning prompt
	prompt, err := r.buildReplanPrompt(input)
	if err != nil {
		return nil, WrapPlanningError(ErrorTypeInternal,
			"failed to build replanning prompt", err)
	}

	// Prepare LLM messages
	messages := []llm.Message{
		llm.NewSystemMessage(r.buildSystemPrompt()),
		llm.NewUserMessage(prompt),
	}

	// Estimate prompt tokens (rough heuristic: ~4 chars per token)
	estimatedTokens := len(prompt) / 4
	if input.RemainingBudget != nil && r.budgetManager != nil {
		if !r.budgetManager.CanAfford(PhaseReplanning, estimatedTokens) {
			r.emitReplanRejected(ctx, input, "insufficient budget for LLM call")
			return &ReplanResult{
				Action:    ReplanActionContinue,
				Rationale: "Insufficient budget for LLM replanning - continuing with current plan",
			}, nil
		}
	}

	// Call LLM for replanning decision
	resp, err := r.harness.Complete(ctx, r.llmSlot, messages,
		harness.WithTemperature(0.3), // Lower temperature for more consistent decisions
		harness.WithMaxTokens(2000),  // Allow substantial response for detailed rationale
	)
	if err != nil {
		// On LLM failure, fall back optimistically to "continue"
		r.emitReplanRejected(ctx, input, fmt.Sprintf("LLM call failed: %v", err))
		return &ReplanResult{
			Action:    ReplanActionContinue,
			Rationale: fmt.Sprintf("LLM replanning failed (%v) - continuing with current plan", err),
		}, nil
	}

	// Record token usage against replanning budget
	tokensUsed := resp.Usage.TotalTokens
	if r.budgetManager != nil && input.RemainingBudget != nil {
		// Calculate cost from token usage
		costUsed := 0.0
		if r.pricingConfig != nil {
			provider, model := parseProviderModel(resp.Model)
			usage := llm.TokenUsage{
				InputTokens:  resp.Usage.PromptTokens,
				OutputTokens: resp.Usage.CompletionTokens,
			}
			if cost, err := r.pricingConfig.CalculateCost(provider, model, usage); err == nil {
				costUsed = cost
			}
		}
		if err := r.budgetManager.Consume(PhaseReplanning, tokensUsed, costUsed); err != nil {
			// Budget exhausted during replanning - continue anyway
			r.emitReplanRejected(ctx, input, "budget exhausted during LLM call")
			return &ReplanResult{
				Action:    ReplanActionContinue,
				Rationale: "Replanning budget exhausted - continuing with current plan",
			}, nil
		}
	}

	// Parse LLM response into replan result
	result, err := r.parseReplanResponse(resp.Message.Content, input)
	if err != nil {
		return nil, WrapPlanningError(ErrorTypeInternal,
			"failed to parse LLM replanning response", err).
			WithContext("response_preview", truncate(resp.Message.Content, 200))
	}

	// Validate the replanning decision
	if err := r.validateReplanResult(result, input); err != nil {
		r.emitReplanRejected(ctx, input, fmt.Sprintf("validation failed: %v", err))
		return nil, WrapPlanningError(ErrorTypeConstraintViolation,
			"generated replan violates constraints", err)
	}

	// Emit replan completed event
	r.emitReplanCompleted(ctx, input, result)

	return result, nil
}

// validateInput checks that all required input parameters are valid.
func (r *LLMTacticalReplanner) validateInput(input ReplanInput) error {
	if input.CurrentPlan == nil {
		return NewInvalidParameterError("current plan cannot be nil")
	}

	if input.TriggerScore == nil {
		return NewInvalidParameterError("trigger score cannot be nil")
	}

	if input.FailedNodeID == "" {
		return NewInvalidParameterError("failed node ID cannot be empty")
	}

	if input.FailureMemory == nil {
		return NewInvalidParameterError("failure memory cannot be nil")
	}

	if input.Bounds == nil {
		return NewInvalidParameterError("bounds cannot be nil")
	}

	if input.OriginalGoal == "" {
		return NewInvalidParameterError("original goal cannot be empty")
	}

	return nil
}

// buildSystemPrompt creates the system message that defines the replanner's role.
func (r *LLMTacticalReplanner) buildSystemPrompt() string {
	return `You are a tactical replanning system for an AI security testing framework.

Your role is to analyze step execution results, failure patterns, and remaining plan context to make intelligent mid-execution decisions that maximize the likelihood of achieving the mission goal.

You must:
1. Consider the original mission goal (never drift from it)
2. Analyze what has already been tried and avoid retry loops
3. Evaluate whether the current plan is still viable
4. Decide on the best action: continue, reorder, skip, retry, escalate, or fail
5. Provide clear rationale for all decisions
6. Respect planning bounds - you cannot invent steps outside the workflow
7. Be optimistic but pragmatic - prefer continuing over failing

Critical constraints:
- If retrying, you MUST propose a different approach than what's been tried
- You can only reorder, skip, or retry nodes that are in the current plan
- You cannot add new nodes or modify completed nodes
- You must respect the original mission goal

Available actions:
- continue: Keep executing the current plan (use when issues are minor)
- reorder: Change the priority of remaining steps (use when ordering matters)
- skip: Skip the failing node and continue (use when node is non-critical)
- retry: Retry the failing node with a different approach (use when alternative exists)
- escalate: Require human intervention (use when decision is unclear)
- fail: Abort the mission (use only when goal is unachievable)

Your response must be valid JSON matching the schema provided in the user message.`
}

// buildReplanPrompt constructs the detailed replanning prompt with all context.
func (r *LLMTacticalReplanner) buildReplanPrompt(input ReplanInput) (string, error) {
	var b strings.Builder

	// Original goal (verbatim for anchoring - CRITICAL for preventing drift)
	b.WriteString("## Original Mission Goal (IMMUTABLE)\n\n")
	b.WriteString(input.OriginalGoal)
	b.WriteString("\n\n")

	// Current plan state
	b.WriteString("## Current Plan State\n\n")
	b.WriteString(r.formatCurrentPlan(input.CurrentPlan))
	b.WriteString("\n")

	// Failure context
	b.WriteString("## Failure Context\n\n")
	b.WriteString(r.formatFailureContext(input.FailedNodeID, input.TriggerScore))
	b.WriteString("\n")

	// What has been tried (critical for preventing retry loops)
	b.WriteString("## Previous Attempts (DO NOT RETRY THESE)\n\n")
	b.WriteString(r.formatPreviousAttempts(input.FailureMemory, input.FailedNodeID))
	b.WriteString("\n")

	// Remaining budget
	b.WriteString("## Remaining Budget\n\n")
	b.WriteString(r.formatRemainingBudget(input.RemainingBudget))
	b.WriteString("\n")

	// Eval context (if available)
	if input.EvalContext != "" {
		b.WriteString("## Eval Feedback Context\n\n")
		b.WriteString(input.EvalContext)
		b.WriteString("\n")
	}

	// Replan constraints
	b.WriteString("## Replanning Constraints\n\n")
	b.WriteString(fmt.Sprintf("- This is replan attempt %d of %d maximum\n", input.ReplanCount+1, r.maxReplans))
	b.WriteString("- You can only work with nodes from the current plan\n")
	b.WriteString("- You cannot modify completed or currently executing nodes\n")
	b.WriteString("- If retrying, you MUST propose a different approach\n")
	b.WriteString("\n")

	// Response schema
	b.WriteString("## Required Response Format\n\n")
	b.WriteString(r.formatResponseSchema())

	return b.String(), nil
}

// formatCurrentPlan formats the current plan state for the prompt.
func (r *LLMTacticalReplanner) formatCurrentPlan(plan *StrategicPlan) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("**Plan ID**: %s\n", plan.ID))
	b.WriteString(fmt.Sprintf("**Status**: %s\n", plan.Status))
	b.WriteString(fmt.Sprintf("**Replan Count**: %d\n\n", plan.ReplanCount))

	b.WriteString("**Remaining Steps** (in priority order):\n")
	for _, step := range plan.OrderedSteps {
		b.WriteString(fmt.Sprintf("- **%s** (priority: %d, budget: %d tokens)\n",
			step.NodeID, step.Priority, step.AllocatedTokens))
		if step.Rationale != "" {
			b.WriteString(fmt.Sprintf("  Rationale: %s\n", step.Rationale))
		}
	}

	if len(plan.SkippedNodes) > 0 {
		b.WriteString("\n**Previously Skipped Nodes**:\n")
		for _, skip := range plan.SkippedNodes {
			b.WriteString(fmt.Sprintf("- **%s**: %s\n", skip.NodeID, skip.Reason))
			if skip.Condition != "" {
				b.WriteString(fmt.Sprintf("  Reconsider if: %s\n", skip.Condition))
			}
		}
	}

	return b.String()
}

// formatFailureContext formats the failure trigger information.
func (r *LLMTacticalReplanner) formatFailureContext(nodeID string, score *StepScore) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("**Failing Node**: %s\n", nodeID))
	b.WriteString(fmt.Sprintf("**Success**: %v\n", score.Success))
	b.WriteString(fmt.Sprintf("**Confidence**: %.2f\n", score.Confidence))
	b.WriteString(fmt.Sprintf("**Scoring Method**: %s\n\n", score.ScoringMethod))

	if score.ReplanReason != "" {
		b.WriteString(fmt.Sprintf("**Replan Reason**: %s\n\n", score.ReplanReason))
	}

	if score.FindingsCount > 0 {
		b.WriteString(fmt.Sprintf("**Findings Discovered**: %d\n", score.FindingsCount))
	}

	if score.AgentHints != nil && len(score.AgentHints.KeyFindings) > 0 {
		b.WriteString("\n**Key Findings**:\n")
		for _, finding := range score.AgentHints.KeyFindings {
			b.WriteString(fmt.Sprintf("- %s\n", finding))
		}
	}

	if score.AgentHints != nil && len(score.AgentHints.SuggestedNext) > 0 {
		b.WriteString("\n**Suggested Next Steps**:\n")
		for _, suggestion := range score.AgentHints.SuggestedNext {
			b.WriteString(fmt.Sprintf("- %s\n", suggestion))
		}
	}

	return b.String()
}

// formatPreviousAttempts formats the failure memory for the prompt.
func (r *LLMTacticalReplanner) formatPreviousAttempts(memory *FailureMemory, nodeID string) string {
	attempts := memory.GetAttempts(nodeID)

	if len(attempts) == 0 {
		return "No previous attempts at this node.\n"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("The following approaches have already been tried for node %s:\n\n", nodeID))

	for i, attempt := range attempts {
		b.WriteString(fmt.Sprintf("%d. **Approach**: %s\n", i+1, attempt.Approach))
		b.WriteString(fmt.Sprintf("   **Result**: %s\n", attempt.Result))
		b.WriteString(fmt.Sprintf("   **Success**: %v\n", attempt.Success))
		b.WriteString(fmt.Sprintf("   **Timestamp**: %s\n\n", attempt.Timestamp.Format(time.RFC3339)))
	}

	b.WriteString("**CRITICAL**: If you choose to retry, you MUST propose a different approach.\n")

	return b.String()
}

// formatRemainingBudget formats the remaining budget information.
func (r *LLMTacticalReplanner) formatRemainingBudget(budget *BudgetAllocation) string {
	if budget == nil {
		return "No budget constraints.\n"
	}

	var b strings.Builder
	b.WriteString("Budget remaining:\n")
	b.WriteString(fmt.Sprintf("- **Replanning Reserve**: %d tokens\n", budget.ReplanReserve))
	b.WriteString(fmt.Sprintf("- **Execution**: %d tokens\n", budget.Execution))

	return b.String()
}

// formatResponseSchema defines the expected JSON response structure.
func (r *LLMTacticalReplanner) formatResponseSchema() string {
	return `Respond with a JSON object matching this schema:

{
  "rationale": "string (EXPLAIN YOUR REASONING FIRST - why this action, what alternatives considered)",
  "action": "string (one of: continue, reorder, skip, retry, escalate, fail)",
  "modified_plan": {
    "ordered_steps": [
      {
        "node_id": "string (must be from current plan)",
        "priority": "number (new priority)",
        "allocated_tokens": "number",
        "rationale": "string"
      }
    ]
  },
  "new_approach": "string (required only if action is 'retry' - must be different from previous attempts)"
}

IMPORTANT:
- Provide rationale BEFORE choosing action (reasoning before conclusion)
- "modified_plan" is required only for action "reorder"
- "new_approach" is required only for action "retry"
- If action is "continue", "skip", "escalate", or "fail", omit modified_plan and new_approach
- The new_approach MUST be different from all previous attempts listed above
- Be specific in rationale - reference the failure context and what you learned from previous attempts`
}

// parseReplanResponse parses the LLM's JSON response into a ReplanResult.
func (r *LLMTacticalReplanner) parseReplanResponse(content string, input ReplanInput) (*ReplanResult, error) {
	// Extract JSON from potential markdown code blocks
	jsonContent := extractJSON(content)

	// Parse into intermediate structure
	var response struct {
		Rationale    string `json:"rationale"`
		Action       string `json:"action"`
		ModifiedPlan *struct {
			OrderedSteps []struct {
				NodeID          string `json:"node_id"`
				Priority        int    `json:"priority"`
				AllocatedTokens int    `json:"allocated_tokens"`
				Rationale       string `json:"rationale"`
			} `json:"ordered_steps"`
		} `json:"modified_plan,omitempty"`
		NewApproach string `json:"new_approach,omitempty"`
	}

	if err := json.Unmarshal([]byte(jsonContent), &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Validate action
	action := ReplanAction(response.Action)
	switch action {
	case ReplanActionContinue, ReplanActionReorder, ReplanActionSkip,
		ReplanActionRetry, ReplanActionEscalate, ReplanActionFail:
		// Valid action
	default:
		return nil, fmt.Errorf("invalid replan action: %q", response.Action)
	}

	// Build ReplanResult
	result := &ReplanResult{
		Action:    action,
		Rationale: response.Rationale,
	}

	// Handle action-specific requirements
	switch action {
	case ReplanActionReorder:
		if response.ModifiedPlan == nil || len(response.ModifiedPlan.OrderedSteps) == 0 {
			return nil, fmt.Errorf("action 'reorder' requires modified_plan with ordered_steps")
		}

		// Build modified strategic plan
		modifiedPlan := &StrategicPlan{
			ID:           input.CurrentPlan.ID,
			MissionID:    input.CurrentPlan.MissionID,
			OrderedSteps: make([]PlannedStep, 0, len(response.ModifiedPlan.OrderedSteps)),
			SkippedNodes: input.CurrentPlan.SkippedNodes, // Preserve skipped nodes
			StepBudgets:  make(map[string]int),
			Rationale:    response.Rationale,
			ReplanCount:  input.ReplanCount + 1,
			Status:       PlanStatusExecuting,
			CreatedAt:    input.CurrentPlan.CreatedAt,
			UpdatedAt:    time.Now(),
		}

		totalTokens := 0
		for _, step := range response.ModifiedPlan.OrderedSteps {
			plannedStep := PlannedStep{
				NodeID:          step.NodeID,
				Priority:        step.Priority,
				AllocatedTokens: step.AllocatedTokens,
				Rationale:       step.Rationale,
			}
			modifiedPlan.OrderedSteps = append(modifiedPlan.OrderedSteps, plannedStep)
			modifiedPlan.StepBudgets[step.NodeID] = step.AllocatedTokens
			totalTokens += step.AllocatedTokens
		}

		modifiedPlan.EstimatedTokens = totalTokens
		result.ModifiedPlan = modifiedPlan

	case ReplanActionRetry:
		if response.NewApproach == "" {
			return nil, fmt.Errorf("action 'retry' requires new_approach field")
		}

		// Check if this approach has already been tried
		if input.FailureMemory.HasTried(input.FailedNodeID, response.NewApproach) {
			return nil, fmt.Errorf("retry approach %q has already been tried for node %s",
				response.NewApproach, input.FailedNodeID)
		}

		result.NewAttempt = &AttemptRecord{
			Timestamp: time.Now(),
			Approach:  response.NewApproach,
			Result:    "pending",
			Success:   false,
		}
	}

	return result, nil
}

// validateReplanResult ensures the replanning decision respects all constraints.
func (r *LLMTacticalReplanner) validateReplanResult(result *ReplanResult, input ReplanInput) error {
	switch result.Action {
	case ReplanActionReorder:
		if result.ModifiedPlan == nil {
			return fmt.Errorf("reorder action requires modified plan")
		}

		// Validate all steps reference workflow nodes
		for i, step := range result.ModifiedPlan.OrderedSteps {
			if !input.Bounds.ContainsNode(step.NodeID) {
				return fmt.Errorf("step %d references invalid node %q (not in workflow)", i, step.NodeID)
			}
		}

		// Validate token budget
		if input.RemainingBudget != nil && result.ModifiedPlan.EstimatedTokens > input.RemainingBudget.Execution {
			return fmt.Errorf("modified plan estimated tokens (%d) exceeds remaining execution budget (%d)",
				result.ModifiedPlan.EstimatedTokens, input.RemainingBudget.Execution)
		}

	case ReplanActionRetry:
		if result.NewAttempt == nil {
			return fmt.Errorf("retry action requires new attempt")
		}

		// Already validated in parseReplanResponse that approach hasn't been tried
	}

	return nil
}

// emitReplanTriggered emits a replan triggered event.
func (r *LLMTacticalReplanner) emitReplanTriggered(ctx context.Context, input ReplanInput) {
	if r.eventEmitter == nil {
		return
	}

	event := NewReplanTriggeredEvent(
		input.MissionID,
		input.FailedNodeID,
		input.TriggerScore.ReplanReason,
		input.ReplanCount,
	)

	_ = r.eventEmitter.Emit(ctx, event)
}

// emitReplanCompleted emits a replan completed event.
func (r *LLMTacticalReplanner) emitReplanCompleted(ctx context.Context, input ReplanInput, result *ReplanResult) {
	if r.eventEmitter == nil {
		return
	}

	event := NewReplanCompletedEvent(
		input.MissionID,
		result.Action.String(),
		result.Rationale,
		input.ReplanCount+1,
	)

	_ = r.eventEmitter.Emit(ctx, event)
}

// emitReplanRejected emits a replan rejected event.
func (r *LLMTacticalReplanner) emitReplanRejected(ctx context.Context, input ReplanInput, reason string) {
	if r.eventEmitter == nil {
		return
	}

	event := NewReplanRejectedEvent(
		input.MissionID,
		reason,
		input.ReplanCount,
	)

	_ = r.eventEmitter.Emit(ctx, event)
}

// ValidateReplanInput validates that all required fields are present and valid.
// Returns an error if any required field is missing or invalid.
func ValidateReplanInput(input ReplanInput) error {
	// Validate CurrentPlan
	if input.CurrentPlan == nil {
		return fmt.Errorf("current_plan is required")
	}
	if len(input.CurrentPlan.OrderedSteps) == 0 {
		return fmt.Errorf("current_plan must contain at least one ordered step")
	}

	// Validate TriggerScore
	if input.TriggerScore == nil {
		return fmt.Errorf("trigger_score is required")
	}
	if input.TriggerScore.NodeID == "" {
		return fmt.Errorf("trigger_score.node_id cannot be empty")
	}
	if !input.TriggerScore.ShouldReplan {
		return fmt.Errorf("trigger_score.should_replan must be true to trigger replanning")
	}

	// Validate FailedNodeID
	if input.FailedNodeID == "" {
		return fmt.Errorf("failed_node_id is required")
	}

	// Validate TriggerScore matches FailedNodeID
	if input.TriggerScore.NodeID != input.FailedNodeID {
		return fmt.Errorf("trigger_score.node_id (%s) must match failed_node_id (%s)",
			input.TriggerScore.NodeID, input.FailedNodeID)
	}

	// Validate RemainingBudget
	if input.RemainingBudget == nil {
		return fmt.Errorf("remaining_budget is required")
	}
	if input.RemainingBudget.TotalTokens < 0 {
		return fmt.Errorf("remaining_budget.total_tokens cannot be negative")
	}

	// Validate FailureMemory
	if input.FailureMemory == nil {
		return fmt.Errorf("failure_memory is required")
	}

	// Validate OriginalGoal - CRITICAL for preventing goal drift
	if input.OriginalGoal == "" {
		return fmt.Errorf("original_goal is required (critical for preventing goal drift)")
	}

	// Validate Bounds
	if input.Bounds == nil {
		return fmt.Errorf("bounds is required")
	}
	if len(input.Bounds.AllowedNodes) == 0 {
		return fmt.Errorf("bounds must contain at least one allowed node")
	}

	// Validate failed node exists in bounds
	if !input.Bounds.ContainsNode(input.FailedNodeID) {
		return fmt.Errorf("failed_node_id %q is not in bounds.allowed_nodes", input.FailedNodeID)
	}

	// Validate ReplanCount
	if input.ReplanCount < 0 {
		return fmt.Errorf("replan_count cannot be negative")
	}
	if input.ReplanCount >= MaxReplans {
		return fmt.Errorf("replan_count (%d) exceeds maximum allowed replans (%d)",
			input.ReplanCount, MaxReplans)
	}

	return nil
}

// ValidateReplanResult validates that a ReplanResult is internally consistent.
// Returns an error if the result is invalid or internally inconsistent.
func ValidateReplanResult(result *ReplanResult) error {
	if result == nil {
		return fmt.Errorf("replan_result cannot be nil")
	}

	// Validate Action
	if !result.Action.IsValid() {
		return fmt.Errorf("invalid replan_action: %q", result.Action)
	}

	// Validate Rationale
	if result.Rationale == "" {
		return fmt.Errorf("rationale is required")
	}

	// Validate ModifiedPlan consistency with Action
	switch result.Action {
	case ReplanActionReorder, ReplanActionSkip:
		// These actions MUST provide a modified plan
		if result.ModifiedPlan == nil {
			return fmt.Errorf("modified_plan is required for action %q", result.Action)
		}
		if len(result.ModifiedPlan.OrderedSteps) == 0 {
			return fmt.Errorf("modified_plan must contain at least one ordered step for action %q", result.Action)
		}

	case ReplanActionContinue, ReplanActionEscalate, ReplanActionFail:
		// These actions should NOT provide a modified plan
		if result.ModifiedPlan != nil {
			return fmt.Errorf("modified_plan must be nil for action %q", result.Action)
		}

	case ReplanActionRetry:
		// Retry should not provide a modified plan, but MUST provide NewAttempt
		if result.ModifiedPlan != nil {
			return fmt.Errorf("modified_plan must be nil for action %q", result.Action)
		}
		if result.NewAttempt == nil {
			return fmt.Errorf("new_attempt is required for action %q", result.Action)
		}
		if result.NewAttempt.Approach == "" {
			return fmt.Errorf("new_attempt.approach cannot be empty for action %q", result.Action)
		}
	}

	// Validate NewAttempt consistency with Action
	if result.Action != ReplanActionRetry && result.NewAttempt != nil {
		return fmt.Errorf("new_attempt must only be set for action %q, but got action %q",
			ReplanActionRetry, result.Action)
	}

	return nil
}

// Ensure LLMTacticalReplanner implements TacticalReplanner at compile time
var _ TacticalReplanner = (*LLMTacticalReplanner)(nil)
