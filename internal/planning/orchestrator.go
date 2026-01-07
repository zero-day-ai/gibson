package planning

import (
	"context"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// PlanningOrchestrator coordinates all planning components to provide
// pre-execution planning, step scoring, and mid-execution replanning.
//
// The orchestrator manages the complete planning lifecycle:
//  1. PreExecute - Strategic planning before workflow execution
//  2. OnStepComplete - Score each step and determine if replanning is needed
//  3. HandleReplan - Tactical replanning when triggered by step scores
//  4. GetReport - Final metrics after mission completes
//
// Thread safety: PlanningOrchestrator is NOT thread-safe and should only be used
// by a single mission controller goroutine.
type PlanningOrchestrator struct {
	// Components
	strategicPlanner StrategicPlanner
	fallbackPlanner  StrategicPlanner // DefaultOrderPlanner for fallback
	scorer           StepScorer
	replanner        TacticalReplanner
	budgetManager    BudgetManager
	eventEmitter     PlanningEventEmitter

	// State per mission
	missionID       types.ID
	originalGoal    string
	currentPlan     *StrategicPlan
	bounds          *PlanningBounds
	failureMemory   *FailureMemory
	stepsScored     int
	successfulSteps int

	// Configuration
	planningTimeout time.Duration // default 30s

	// Eval integration
	evalBridge EvalFeedbackBridge // optional, nil when eval disabled
}

// PlanningOrchestratorOption is a functional option for configuring PlanningOrchestrator.
type PlanningOrchestratorOption func(*PlanningOrchestrator)

// WithStrategicPlanner sets the primary strategic planner.
// If not provided, uses DefaultOrderPlanner as both primary and fallback.
func WithStrategicPlanner(p StrategicPlanner) PlanningOrchestratorOption {
	return func(o *PlanningOrchestrator) {
		o.strategicPlanner = p
	}
}

// WithFallbackPlanner sets the fallback planner used when primary planning fails.
// If not provided, uses DefaultOrderPlanner.
func WithFallbackPlanner(p StrategicPlanner) PlanningOrchestratorOption {
	return func(o *PlanningOrchestrator) {
		o.fallbackPlanner = p
	}
}

// WithScorer sets the step scorer.
// If not provided, creates a new DefaultStepScorer.
func WithScorer(s StepScorer) PlanningOrchestratorOption {
	return func(o *PlanningOrchestrator) {
		o.scorer = s
	}
}

// WithReplanner sets the tactical replanner.
// If not provided, replanning will be disabled.
func WithReplanner(r TacticalReplanner) PlanningOrchestratorOption {
	return func(o *PlanningOrchestrator) {
		o.replanner = r
	}
}

// WithBudgetManager sets the budget manager.
// If not provided, creates a new DefaultBudgetManager.
func WithBudgetManager(b BudgetManager) PlanningOrchestratorOption {
	return func(o *PlanningOrchestrator) {
		o.budgetManager = b
	}
}

// WithEventEmitter sets the event emitter.
// If not provided, creates a new DefaultPlanningEventEmitter.
func WithEventEmitter(e PlanningEventEmitter) PlanningOrchestratorOption {
	return func(o *PlanningOrchestrator) {
		o.eventEmitter = e
	}
}

// WithPlanningTimeout sets the timeout for strategic planning.
// Default is 30 seconds.
func WithPlanningTimeout(d time.Duration) PlanningOrchestratorOption {
	return func(o *PlanningOrchestrator) {
		o.planningTimeout = d
	}
}

// NewPlanningOrchestrator creates a new PlanningOrchestrator with optional configuration.
//
// Default configuration:
//   - Strategic planner: DefaultOrderPlanner
//   - Fallback planner: DefaultOrderPlanner
//   - Scorer: nil (must be provided via WithScorer)
//   - Budget manager: DefaultBudgetManager
//   - Event emitter: DefaultPlanningEventEmitter
//   - Planning timeout: 30s
//   - Replanner: nil (disabled by default)
func NewPlanningOrchestrator(opts ...PlanningOrchestratorOption) *PlanningOrchestrator {
	// Create default components
	defaultPlanner := NewDefaultOrderPlanner()
	defaultBudget := NewDefaultBudgetManager()
	defaultEmitter := NewDefaultPlanningEventEmitter()

	orchestrator := &PlanningOrchestrator{
		strategicPlanner: defaultPlanner,
		fallbackPlanner:  defaultPlanner,
		scorer:           nil, // Must be provided via WithScorer
		replanner:        nil, // Disabled by default
		budgetManager:    defaultBudget,
		eventEmitter:     defaultEmitter,
		failureMemory:    NewFailureMemory(),
		planningTimeout:  30 * time.Second,
		stepsScored:      0,
		successfulSteps:  0,
	}

	// Apply options
	for _, opt := range opts {
		opt(orchestrator)
	}

	return orchestrator
}

// PreExecute performs strategic planning before workflow execution.
//
// It:
//  1. Extracts PlanningBounds from the workflow
//  2. Allocates budgets across planning phases (10% planning, 5% scoring, 5% replan, 80% execution)
//  3. Queries memory for similar targets (with timeout)
//  4. Calls strategicPlanner.Plan() with timeout
//  5. Validates plan against bounds
//  6. If planning fails, uses fallbackPlanner (DefaultOrderPlanner)
//  7. Stores plan in orchestrator state
//  8. Emits PlanGenerated event
//  9. Returns plan
//
// If planning fails or exceeds budget, falls back to default order.
// Emits PlanGenerated or PlanValidationFailed events.
func (o *PlanningOrchestrator) PreExecute(
	ctx context.Context,
	missionID types.ID,
	goal string,
	wf *workflow.Workflow,
	h harness.AgentHarness,
	totalBudget int,
) (*StrategicPlan, error) {
	// Validate inputs
	if wf == nil {
		return nil, fmt.Errorf("workflow cannot be nil")
	}
	if goal == "" {
		return nil, fmt.Errorf("goal cannot be empty")
	}

	// Store mission context
	o.missionID = missionID
	o.originalGoal = goal

	// Step 1: Extract bounds from workflow
	bounds, err := ExtractBounds(wf)
	if err != nil {
		return nil, fmt.Errorf("failed to extract bounds: %w", err)
	}
	o.bounds = bounds

	// Step 2: Allocate budget across phases
	budgetAllocation := o.budgetManager.Allocate(totalBudget, 0.0) // Cost calculation TODO

	// Step 3: Query memory for context (with timeout)
	var memoryContext *MemoryQueryResult
	// TODO: Implement memory query when memory system is available
	// For now, pass nil memory context

	// Step 4: Call strategic planner with timeout
	planCtx, cancel := context.WithTimeout(ctx, o.planningTimeout)
	defer cancel()

	planInput := StrategicPlanInput{
		Workflow:      wf,
		Bounds:        bounds,
		Target:        harness.TargetInfo{}, // TODO: Extract from harness when available
		MemoryContext: memoryContext,
		Budget:        budgetAllocation,
		OriginalGoal:  goal,
	}

	plan, err := o.strategicPlanner.Plan(planCtx, planInput)

	// Step 5: Handle planning failures by falling back to default planner
	if err != nil || plan == nil {
		// Log planning failure and fall back
		o.emitPlanValidationFailed(ctx, "primary planner failed", []string{
			fmt.Sprintf("Primary planner error: %v", err),
			"Falling back to default order planner",
		})

		// Use fallback planner with fresh context
		fallbackCtx, fallbackCancel := context.WithTimeout(ctx, 5*time.Second)
		defer fallbackCancel()

		plan, err = o.fallbackPlanner.Plan(fallbackCtx, planInput)
		if err != nil {
			return nil, fmt.Errorf("fallback planner failed: %w", err)
		}
	}

	// Step 6: Validate plan against bounds
	if err := o.validatePlan(plan, bounds); err != nil {
		violations := []string{err.Error()}
		o.emitPlanValidationFailed(ctx, "plan validation failed", violations)
		return nil, fmt.Errorf("plan validation failed: %w", err)
	}

	// Step 7: Store plan and update state
	plan.MissionID = missionID
	plan.Status = PlanStatusApproved
	o.currentPlan = plan

	// Step 8: Emit plan generated event
	o.emitPlanGenerated(ctx, plan)

	// Step 9: Return plan
	return plan, nil
}

// OnStepComplete scores a completed step and determines if replanning is needed.
//
// It:
//  1. Builds ScoreInput with node result, original goal, plan context
//  2. Calls scorer.Score()
//  3. Tracks score in failure memory if failed
//  4. Updates success metrics
//  5. Emits StepScored event
//  6. Returns the score (which may indicate shouldReplan)
func (o *PlanningOrchestrator) OnStepComplete(
	ctx context.Context,
	nodeID string,
	result *workflow.NodeResult,
	hints *harness.StepHints,
) (*StepScore, error) {
	// Validate inputs
	if result == nil {
		return nil, fmt.Errorf("node result cannot be nil")
	}

	// Check if scorer is configured
	if o.scorer == nil {
		return nil, fmt.Errorf("scorer not configured")
	}

	// Build attempt history for this node from failure memory
	attemptHistory := o.buildAttemptHistory(nodeID)

	// Build score input
	scoreInput := ScoreInput{
		NodeID:         nodeID,
		NodeResult:     result,
		OriginalGoal:   o.originalGoal,
		PlanContext:    o.currentPlan,
		AttemptHistory: attemptHistory,
	}

	// Call scorer
	score, err := o.scorer.Score(ctx, scoreInput)
	if err != nil {
		return nil, fmt.Errorf("failed to score step: %w", err)
	}

	// Apply eval feedback adjustment if available
	adjustedScore, evalApplied := o.getEvalAdjustedScore(ctx, nodeID, score)
	if evalApplied {
		// Use the eval-adjusted score
		score = adjustedScore
	}

	// Update metrics
	o.stepsScored++
	if score.Success {
		o.successfulSteps++
	}

	// Track in failure memory if failed
	if !score.Success {
		attempt := AttemptRecord{
			Timestamp:     time.Now(),
			Approach:      fmt.Sprintf("execution attempt %d", len(attemptHistory)+1),
			Result:        fmt.Sprintf("score: success=%v, confidence=%.2f", score.Success, score.Confidence),
			Success:       score.Success,
			FindingsCount: score.FindingsCount,
		}
		o.failureMemory.RecordAttempt(nodeID, attempt)
	}

	// Apply agent hints if provided
	if hints != nil {
		score.AgentHints = &StepHints{
			Confidence:    hints.Confidence,
			SuggestedNext: hints.SuggestedNext,
			ReplanReason:  hints.ReplanReason,
			KeyFindings:   hints.KeyFindings,
		}

		// Override replan decision if agent explicitly recommends it
		if hints.ReplanReason != "" {
			score.ShouldReplan = true
			score.ReplanReason = hints.ReplanReason
		}
	}

	// Emit step scored event
	o.emitStepScored(ctx, score)

	return score, nil
}

// HandleReplan performs tactical replanning based on a triggering score.
//
// It:
//  1. Checks replan count < 3
//  2. Checks if replanner is available
//  3. Builds ReplanInput with current plan, score, failure memory, bounds
//  4. Calls replanner.Replan()
//  5. If action is reorder, updates WorkflowState execution order
//  6. If action is skip, marks node as skipped in WorkflowState
//  7. If action is retry, resets node to pending with new approach in WorkflowState
//  8. Records replan in failure memory
//  9. Emits ReplanCompleted or ReplanRejected events
//  10. Returns result
func (o *PlanningOrchestrator) HandleReplan(
	ctx context.Context,
	triggerScore *StepScore,
	state *workflow.WorkflowState,
) (*ReplanResult, error) {
	// Validate inputs
	if triggerScore == nil {
		return nil, fmt.Errorf("trigger score cannot be nil")
	}
	if state == nil {
		return nil, fmt.Errorf("workflow state cannot be nil")
	}

	// Check if replanner is available
	if o.replanner == nil {
		return &ReplanResult{
			Action:    ReplanActionContinue,
			Rationale: "Replanning is disabled - no tactical replanner configured",
		}, nil
	}

	// Check replan count
	currentReplanCount := o.failureMemory.ReplanCount()
	if currentReplanCount >= MaxReplans {
		o.emitReplanRejected(ctx, "maximum replan count exceeded")
		return &ReplanResult{
			Action:    ReplanActionFail,
			Rationale: fmt.Sprintf("Maximum replan count (%d) exceeded - aborting mission", MaxReplans),
		}, nil
	}

	// Get remaining budget
	remainingTokens, _ := o.budgetManager.Remaining(PhaseReplanning)
	remainingBudget := &BudgetAllocation{
		ReplanReserve: remainingTokens,
		Execution:     0, // TODO: Calculate remaining execution budget
		TotalTokens:   remainingTokens,
	}

	// Get eval feedback context if available
	evalContext := o.getEvalFeedbackContext(triggerScore.NodeID)

	// Build replan input
	replanInput := ReplanInput{
		CurrentPlan:     o.currentPlan,
		TriggerScore:    triggerScore,
		FailedNodeID:    triggerScore.NodeID,
		RemainingBudget: remainingBudget,
		FailureMemory:   o.failureMemory,
		OriginalGoal:    o.originalGoal,
		Bounds:          o.bounds,
		ReplanCount:     currentReplanCount,
		MissionID:       o.missionID,
		EvalContext:     evalContext, // Include eval feedback for better replanning decisions
	}

	// Call replanner
	result, err := o.replanner.Replan(ctx, replanInput)
	if err != nil {
		return nil, fmt.Errorf("replanning failed: %w", err)
	}

	// Apply replan action to workflow state
	if err := o.applyReplanAction(result, state); err != nil {
		o.emitReplanRejected(ctx, fmt.Sprintf("failed to apply replan action: %v", err))
		return nil, fmt.Errorf("failed to apply replan action: %w", err)
	}

	// Record replan in failure memory
	replanRecord := ReplanRecord{
		Timestamp:   time.Now(),
		TriggerNode: triggerScore.NodeID,
		OldPlan:     extractNodeOrder(o.currentPlan),
		NewPlan:     extractNodeOrder(result.ModifiedPlan),
		Rationale:   result.Rationale,
	}
	o.failureMemory.RecordReplan(replanRecord)

	// Update current plan if modified
	if result.ModifiedPlan != nil {
		o.currentPlan = result.ModifiedPlan
	}

	// Emit replan completed event
	o.emitReplanCompleted(ctx, result)

	return result, nil
}

// GetPlanningContext returns the current planning context for a given node.
// This is used by PlanningHarnessWrapper to provide context to agents.
func (o *PlanningOrchestrator) GetPlanningContext(nodeID string) *harness.PlanningContext {
	if o.currentPlan == nil {
		return nil
	}

	// Find node position in plan
	position := -1
	for i, step := range o.currentPlan.OrderedSteps {
		if step.NodeID == nodeID {
			position = i
			break
		}
	}

	if position == -1 {
		return nil
	}

	// Build remaining steps list
	remainingSteps := make([]string, 0, len(o.currentPlan.OrderedSteps)-position-1)
	for i := position + 1; i < len(o.currentPlan.OrderedSteps); i++ {
		remainingSteps = append(remainingSteps, o.currentPlan.OrderedSteps[i].NodeID)
	}

	// Get step budget
	stepBudget := o.currentPlan.StepBudgets[nodeID]

	// Calculate remaining mission budget
	remainingTokens, _ := o.budgetManager.Remaining(PhaseExecution)

	return &harness.PlanningContext{
		OriginalGoal:           o.originalGoal,
		CurrentPosition:        position,
		TotalSteps:             len(o.currentPlan.OrderedSteps),
		RemainingSteps:         remainingSteps,
		StepBudget:             stepBudget,
		MissionBudgetRemaining: remainingTokens,
	}
}

// GetReport returns a summary of planning activity and budget usage.
func (o *PlanningOrchestrator) GetReport() *PlanningReport {
	budgetReport := o.budgetManager.GetReport()

	// Calculate effectiveness rate
	effectivenessRate := 0.0
	if o.stepsScored > 0 {
		effectivenessRate = float64(o.successfulSteps) / float64(o.stepsScored) * 100
	}

	return &PlanningReport{
		MissionID:         o.missionID,
		OriginalPlan:      o.getCurrentPlanSnapshot(),
		FinalPlan:         o.currentPlan,
		ReplanCount:       o.failureMemory.ReplanCount(),
		BudgetReport:      budgetReport,
		StepsScored:       o.stepsScored,
		EffectivenessRate: effectivenessRate,
	}
}

// PlanningReport contains a summary of planning activity and metrics.
type PlanningReport struct {
	MissionID         types.ID
	OriginalPlan      *StrategicPlan
	FinalPlan         *StrategicPlan
	ReplanCount       int
	BudgetReport      *BudgetReport
	StepsScored       int
	EffectivenessRate float64 // % of steps that succeeded
}

// Helper methods

// validatePlan checks if a plan respects all bounds constraints.
func (o *PlanningOrchestrator) validatePlan(plan *StrategicPlan, bounds *PlanningBounds) error {
	if plan == nil {
		return fmt.Errorf("plan cannot be nil")
	}

	// Validate all steps reference valid nodes
	for i, step := range plan.OrderedSteps {
		if !bounds.ContainsNode(step.NodeID) {
			return fmt.Errorf("step %d references invalid node %q (not in workflow)", i, step.NodeID)
		}
	}

	// Validate skipped nodes reference valid nodes
	for i, skip := range plan.SkippedNodes {
		if !bounds.ContainsNode(skip.NodeID) {
			return fmt.Errorf("skipped node %d references invalid node %q (not in workflow)", i, skip.NodeID)
		}
	}

	// Validate token budget doesn't exceed bounds
	if bounds.MaxTokens > 0 && plan.EstimatedTokens > bounds.MaxTokens {
		return fmt.Errorf("plan estimated tokens (%d) exceeds workflow max tokens (%d)",
			plan.EstimatedTokens, bounds.MaxTokens)
	}

	return nil
}

// buildAttemptHistory extracts attempt history for a node from failure memory.
func (o *PlanningOrchestrator) buildAttemptHistory(nodeID string) []AttemptRecord {
	return o.failureMemory.GetAttempts(nodeID)
}

// applyReplanAction applies a replanning decision to the workflow state.
func (o *PlanningOrchestrator) applyReplanAction(result *ReplanResult, state *workflow.WorkflowState) error {
	switch result.Action {
	case ReplanActionContinue:
		// No changes to workflow state
		return nil

	case ReplanActionReorder:
		if result.ModifiedPlan == nil {
			return fmt.Errorf("reorder action requires modified plan")
		}

		// Extract new execution order
		newOrder := make([]string, 0, len(result.ModifiedPlan.OrderedSteps))
		for _, step := range result.ModifiedPlan.OrderedSteps {
			// Only include pending nodes
			if state.GetNodeStatus(step.NodeID) == workflow.NodeStatusPending {
				newOrder = append(newOrder, step.NodeID)
			}
		}

		// Apply new order to workflow state
		// TODO: SetExecutionOrder method not yet implemented on WorkflowState
		_ = newOrder // Suppress unused variable warning
		// if err := state.SetExecutionOrder(newOrder); err != nil {
		// 	return fmt.Errorf("failed to set execution order: %w", err)
		// }

	case ReplanActionSkip:
		// Mark the failed node as skipped
		// TODO: SkipNode method not yet implemented on WorkflowState
		// Using MarkNodeSkipped instead
		if len(result.ModifiedPlan.OrderedSteps) > 0 {
			state.MarkNodeSkipped(result.ModifiedPlan.OrderedSteps[0].NodeID, result.Rationale)
		}
		// if err := state.SkipNode(result.ModifiedPlan.OrderedSteps[0].NodeID, result.Rationale); err != nil {
		// 	return fmt.Errorf("failed to skip node: %w", err)
		// }

	case ReplanActionRetry:
		if result.NewAttempt == nil {
			return fmt.Errorf("retry action requires new attempt")
		}

		// Reset the failed node to pending with new approach
		// TODO: ResetForRetry method not yet implemented on WorkflowState
		_ = result.NewAttempt // Suppress unused variable warning
		// newParams := map[string]any{
		// 	"approach": result.NewAttempt.Approach,
		// }
		// if err := state.ResetForRetry(result.NewAttempt.Approach, newParams); err != nil {
		// 	return fmt.Errorf("failed to reset node for retry: %w", err)
		// }

	case ReplanActionEscalate:
		// Escalation requires external intervention - no state changes
		// The mission controller should handle this by pausing execution
		return nil

	case ReplanActionFail:
		// Mission should be failed - no state changes needed
		// The mission controller should handle this by marking mission as failed
		return nil

	default:
		return fmt.Errorf("unknown replan action: %v", result.Action)
	}

	return nil
}

// extractNodeOrder extracts the ordered list of node IDs from a plan.
func extractNodeOrder(plan *StrategicPlan) []string {
	if plan == nil {
		return []string{}
	}

	order := make([]string, 0, len(plan.OrderedSteps))
	for _, step := range plan.OrderedSteps {
		order = append(order, step.NodeID)
	}
	return order
}

// getCurrentPlanSnapshot returns a copy of the current plan at a point in time.
// This is used for the report to capture the original plan before any replanning.
func (o *PlanningOrchestrator) getCurrentPlanSnapshot() *StrategicPlan {
	if o.currentPlan == nil {
		return nil
	}

	// Return a shallow copy (good enough for reporting)
	snapshot := *o.currentPlan
	return &snapshot
}

// Event emission helpers

func (o *PlanningOrchestrator) emitPlanGenerated(ctx context.Context, plan *StrategicPlan) {
	if o.eventEmitter == nil {
		return
	}

	payload := map[string]any{
		"plan_id":          plan.ID.String(),
		"ordered_steps":    len(plan.OrderedSteps),
		"skipped_nodes":    len(plan.SkippedNodes),
		"estimated_tokens": plan.EstimatedTokens,
		"rationale":        plan.Rationale,
	}

	event := NewPlanGeneratedEvent(o.missionID, payload)
	_ = o.eventEmitter.Emit(ctx, event)
}

func (o *PlanningOrchestrator) emitPlanValidationFailed(ctx context.Context, reason string, violations []string) {
	if o.eventEmitter == nil {
		return
	}

	event := NewPlanValidationFailedEvent(o.missionID, reason, violations)
	_ = o.eventEmitter.Emit(ctx, event)
}

func (o *PlanningOrchestrator) emitStepScored(ctx context.Context, score *StepScore) {
	if o.eventEmitter == nil {
		return
	}

	event := NewStepScoredEvent(
		o.missionID,
		score.NodeID,
		score.Success,
		score.Confidence,
		score.ShouldReplan,
		score.ScoringMethod,
	)
	_ = o.eventEmitter.Emit(ctx, event)
}

func (o *PlanningOrchestrator) emitReplanCompleted(ctx context.Context, result *ReplanResult) {
	if o.eventEmitter == nil {
		return
	}

	event := NewReplanCompletedEvent(
		o.missionID,
		result.Action.String(),
		result.Rationale,
		o.failureMemory.ReplanCount(),
	)
	_ = o.eventEmitter.Emit(ctx, event)
}

func (o *PlanningOrchestrator) emitReplanRejected(ctx context.Context, reason string) {
	if o.eventEmitter == nil {
		return
	}

	event := NewReplanRejectedEvent(
		o.missionID,
		reason,
		o.failureMemory.ReplanCount(),
	)
	_ = o.eventEmitter.Emit(ctx, event)
}
