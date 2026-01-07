package planning

// Strategic Planning Overview
//
// The strategic planner is responsible for pre-execution planning of workflow execution.
// It takes a workflow definition, planning bounds, target information, and historical
// memory data to generate an optimized execution plan.
//
// ## Core Types
//
// StrategicPlanner - Interface for plan generation (defined in default_planner.go)
// StrategicPlanInput - Input parameters for planning
// StrategicPlan - Output execution plan with ordered steps, budgets, and rationale
// PlannedStep - Individual execution step with priority and budget
// SkipDecision - Documentation of why a node was excluded
// PlanStatus - Lifecycle state tracking (draft -> approved -> executing -> completed/failed/cancelled)
//
// ## Memory-Informed Planning
//
// The planner can leverage historical data through MemoryQueryResult:
//   - SimilarMissions: Past missions against similar targets
//   - SuccessfulTechniques: What worked before with success rates
//   - FailedApproaches: What to avoid based on historical failures
//   - RelevantFindings: Past vulnerabilities discovered on similar targets
//
// ## Budget Management
//
// Plans must respect budget allocations:
//   - Strategic planning consumes from Budget.StrategicPlanning (10% of total)
//   - Per-step budgets distributed across StepBudgets map
//   - Total EstimatedTokens should not exceed available execution budget
//
// ## Bounds Enforcement
//
// All planning decisions must respect PlanningBounds:
//   - Only workflow-defined nodes can be included
//   - Cannot exceed MaxParallel concurrency
//   - Must respect approval gates
//   - Cannot violate per-node constraints
//
// ## Implementations
//
// DefaultOrderPlanner - Deterministic topological sort (always available as fallback)
// LLMStrategicPlanner - LLM-powered intelligent planning (to be implemented in task 2.3)
//
// ## Usage Example
//
//	planner := NewDefaultOrderPlanner()
//	plan, err := planner.Plan(ctx, StrategicPlanInput{
//		Workflow:      workflow,
//		Bounds:        bounds,
//		Target:        targetInfo,
//		MemoryContext: memoryResults,
//		Budget:        budgetAllocation,
//		OriginalGoal:  "Discover prompt injection vulnerabilities in target LLM",
//	})
//	if err != nil {
//		// Handle error
//	}
//	// plan.OrderedSteps contains execution sequence
//	// plan.StepBudgets contains token allocations per step
