package planning

import (
	"context"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// StrategicPlanner generates an optimized execution plan before workflow execution begins.
// It analyzes the target, queries memory for similar missions, and produces an ordered
// sequence of workflow steps with resource allocations - all within the constraints
// defined by the workflow.yaml bounds.
type StrategicPlanner interface {
	// Plan generates an execution strategy within bounds.
	//
	// The planner:
	//   - Queries long-term memory for similar targets and successful patterns
	//   - Analyzes available workflow nodes and their constraints
	//   - Generates an ordered execution sequence (subset of workflow nodes)
	//   - Allocates token budgets per step
	//   - Identifies nodes to skip with rationale
	//   - Groups nodes that can execute in parallel
	//
	// All planning decisions must respect the PlanningBounds - the planner
	// cannot invent nodes outside the workflow or exceed resource limits.
	//
	// Returns an error if:
	//   - Planning budget is exhausted (exceeds 10% of mission budget)
	//   - Generated plan violates workflow bounds
	//   - Context is cancelled or times out
	Plan(ctx context.Context, input StrategicPlanInput) (*StrategicPlan, error)
}

// StrategicPlanInput contains all information needed for strategic planning.
type StrategicPlanInput struct {
	// Workflow is the workflow definition containing all allowed nodes.
	Workflow *workflow.Workflow

	// Bounds are the extracted planning constraints from the workflow.
	// The planner must operate within these bounds.
	Bounds *PlanningBounds

	// Target is the system being tested.
	// Used for memory queries to find similar past missions.
	Target harness.TargetInfo

	// MemoryContext contains results from querying long-term memory
	// for similar targets, successful techniques, and failed approaches.
	// May be nil if memory is unavailable or disabled.
	MemoryContext *MemoryQueryResult

	// Budget is the allocated budget for planning phases.
	// The planner should draw from Budget.StrategicPlanning.
	Budget *BudgetAllocation

	// OriginalGoal is the immutable mission goal statement.
	// This should be included in planning prompts to anchor decisions
	// and prevent goal drift.
	OriginalGoal string
}

// StrategicPlan represents a complete pre-execution strategy.
// It specifies exactly which workflow nodes to execute, in what order,
// with what resource allocations.
type StrategicPlan struct {
	// ID is the unique identifier for this plan.
	ID types.ID `json:"id"`

	// MissionID is the mission this plan belongs to.
	MissionID types.ID `json:"mission_id"`

	// OrderedSteps contains the execution steps in priority order.
	// Lower priority values execute first.
	// Only includes nodes that should be executed (skipped nodes excluded).
	OrderedSteps []PlannedStep `json:"ordered_steps"`

	// SkippedNodes contains nodes that were intentionally excluded from execution.
	// Each includes rationale for why it was skipped and conditions to reconsider.
	SkippedNodes []SkipDecision `json:"skipped_nodes,omitempty"`

	// ParallelGroups contains sets of node IDs that can execute concurrently.
	// Each inner slice is a group of nodes with no mutual dependencies.
	// Empty if all execution is sequential.
	ParallelGroups [][]string `json:"parallel_groups,omitempty"`

	// StepBudgets maps node IDs to allocated token budgets.
	// The sum should not exceed the execution budget from BudgetAllocation.
	StepBudgets map[string]int `json:"step_budgets"`

	// Rationale explains the overall planning strategy.
	// Should describe why this ordering was chosen and what alternatives
	// were considered.
	Rationale string `json:"rationale"`

	// MemoryInfluences contains IDs or descriptions of memory queries
	// that influenced this plan. Used for observability and debugging.
	MemoryInfluences []string `json:"memory_influences,omitempty"`

	// EstimatedTokens is the projected total token usage for this plan.
	// Sum of all StepBudgets plus planning overhead.
	EstimatedTokens int `json:"estimated_tokens"`

	// EstimatedDuration is the projected total execution time.
	// Calculated from node timeouts in the planned sequence.
	EstimatedDuration time.Duration `json:"estimated_duration"`

	// Status tracks the plan lifecycle.
	Status PlanStatus `json:"status"`

	// ReplanCount tracks how many times this plan has been modified
	// during execution via tactical replanning.
	// Starts at 0 for new plans.
	ReplanCount int `json:"replan_count"`

	// CreatedAt is when the plan was generated.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the plan was last modified.
	// Updated when tactical replanning modifies the plan.
	UpdatedAt time.Time `json:"updated_at"`
}

// PlannedStep represents a single step in the execution strategy.
type PlannedStep struct {
	// NodeID is the workflow node to execute.
	// Must exist in PlanningBounds.AllowedNodes.
	NodeID string `json:"node_id"`

	// Priority determines execution order.
	// Lower values execute first. Steps with equal priority may run in parallel.
	Priority int `json:"priority"`

	// AllocatedTokens is the token budget for this step.
	// The step scorer will flag if actual usage significantly exceeds this.
	AllocatedTokens int `json:"allocated_tokens"`

	// Rationale explains why this step is included and why it has this priority.
	// Should reference memory context if a similar technique succeeded before.
	Rationale string `json:"rationale"`
}

// SkipDecision documents why a workflow node was excluded from the plan.
type SkipDecision struct {
	// NodeID is the workflow node that was skipped.
	NodeID string `json:"node_id"`

	// Reason explains why this node was skipped.
	// Examples:
	//   - "Target is Windows, this exploit only works on Linux"
	//   - "Memory shows this technique has 0% success rate on this target type"
	//   - "Redundant with higher-priority step X which provides same coverage"
	Reason string `json:"reason"`

	// Condition describes under what circumstances this node should be reconsidered.
	// Used by tactical replanner to decide if a skipped node should be retried.
	// Examples:
	//   - "If step X fails to produce findings"
	//   - "If target type is re-identified as vulnerable"
	//   - "" (empty) if this skip is permanent
	Condition string `json:"condition,omitempty"`
}

// PlanStatus represents the lifecycle state of a strategic plan.
type PlanStatus string

const (
	// PlanStatusDraft indicates the plan is being generated.
	PlanStatusDraft PlanStatus = "draft"

	// PlanStatusApproved indicates the plan is ready for execution.
	// This is the normal state after successful planning.
	PlanStatusApproved PlanStatus = "approved"

	// PlanStatusExecuting indicates the plan is currently being executed.
	PlanStatusExecuting PlanStatus = "executing"

	// PlanStatusCompleted indicates all planned steps completed successfully.
	PlanStatusCompleted PlanStatus = "completed"

	// PlanStatusFailed indicates execution failed and cannot continue.
	PlanStatusFailed PlanStatus = "failed"

	// PlanStatusCancelled indicates execution was cancelled by user or system.
	PlanStatusCancelled PlanStatus = "cancelled"
)

// String returns the string representation of the plan status.
func (s PlanStatus) String() string {
	return string(s)
}

// IsTerminal returns true if the status represents a terminal state.
func (s PlanStatus) IsTerminal() bool {
	switch s {
	case PlanStatusCompleted, PlanStatusFailed, PlanStatusCancelled:
		return true
	default:
		return false
	}
}

// DefaultOrderPlanner implements StrategicPlanner with deterministic ordering.
// It uses topological sort based on workflow dependencies (Kahn's algorithm)
// and allocates budgets equally across steps. This is a fast, zero-cost fallback
// that provides a valid execution plan when LLM planning fails or budget is exhausted.
type DefaultOrderPlanner struct {
	// No state needed - stateless implementation
}

// NewDefaultOrderPlanner creates a new DefaultOrderPlanner.
func NewDefaultOrderPlanner() *DefaultOrderPlanner {
	return &DefaultOrderPlanner{}
}

// Plan generates a deterministic execution plan using topological sort.
//
// Algorithm:
//  1. Use Kahn's algorithm for topological sort based on dependencies
//  2. Allocate budget equally across all steps
//  3. No nodes are skipped (include all workflow nodes)
//  4. No parallel groups (sequential execution)
//
// Performance: O(V + E) where V is nodes and E is dependency edges.
// Typically < 10ms for workflows with 50 nodes.
func (p *DefaultOrderPlanner) Plan(ctx context.Context, input StrategicPlanInput) (*StrategicPlan, error) {
	if input.Workflow == nil {
		return nil, fmt.Errorf("workflow cannot be nil")
	}

	if input.Bounds == nil {
		return nil, fmt.Errorf("bounds cannot be nil")
	}

	if len(input.Workflow.Nodes) == 0 {
		return nil, fmt.Errorf("workflow must contain at least one node")
	}

	// Perform topological sort using Kahn's algorithm
	orderedNodeIDs, err := p.topologicalSort(input.Workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to perform topological sort: %w", err)
	}

	// Calculate equal budget distribution
	nodeCount := len(orderedNodeIDs)
	executionBudget := 0
	if input.Budget != nil {
		executionBudget = input.Budget.Execution
	}

	tokensPerStep := 0
	if nodeCount > 0 && executionBudget > 0 {
		tokensPerStep = executionBudget / nodeCount
	}

	// Build planned steps with equal priority spacing
	plannedSteps := make([]PlannedStep, 0, nodeCount)
	stepBudgets := make(map[string]int, nodeCount)

	for i, nodeID := range orderedNodeIDs {
		step := PlannedStep{
			NodeID:          nodeID,
			Priority:        i, // Sequential priority
			AllocatedTokens: tokensPerStep,
			Rationale:       "Deterministic topological order based on dependencies",
		}
		plannedSteps = append(plannedSteps, step)
		stepBudgets[nodeID] = tokensPerStep
	}

	// Adjust last step for rounding errors
	if nodeCount > 0 && executionBudget > 0 {
		totalAllocated := tokensPerStep * nodeCount
		remainder := executionBudget - totalAllocated
		if remainder > 0 {
			lastNodeID := orderedNodeIDs[nodeCount-1]
			plannedSteps[nodeCount-1].AllocatedTokens += remainder
			stepBudgets[lastNodeID] += remainder
		}
	}

	// Create new plan with all fields initialized
	now := time.Now()
	return &StrategicPlan{
		ID:                types.NewID(),
		MissionID:         types.ID(""), // Will be set by caller
		OrderedSteps:      plannedSteps,
		SkippedNodes:      []SkipDecision{}, // No nodes skipped
		ParallelGroups:    [][]string{},     // No parallelism in default planner
		StepBudgets:       stepBudgets,
		Rationale:         "Default sequential execution plan using topological sort with equal budget distribution",
		MemoryInfluences:  []string{}, // No memory used in default planner
		EstimatedTokens:   executionBudget,
		EstimatedDuration: input.Bounds.MaxDuration, // Use workflow's total duration
		Status:            PlanStatusDraft,          // Start in draft status
		ReplanCount:       0,                        // New plan, no replans yet
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// topologicalSort performs Kahn's algorithm to produce a topologically sorted
// order of workflow nodes based on their dependencies.
//
// Kahn's algorithm:
//  1. Calculate in-degree (number of dependencies) for each node
//  2. Start with nodes that have zero in-degree (no dependencies)
//  3. Process each zero-in-degree node:
//     - Add it to the result
//     - Remove it from the graph by decrementing dependent nodes' in-degrees
//     - Add newly zero-in-degree nodes to the queue
//  4. If all nodes processed, we have a valid topological order
//  5. If nodes remain, there's a circular dependency
//
// Returns a slice of node IDs in topologically sorted order.
func (p *DefaultOrderPlanner) topologicalSort(wf *workflow.Workflow) ([]string, error) {
	// Build in-degree map (count of dependencies for each node)
	inDegree := make(map[string]int)

	// Build adjacency list (node -> list of nodes that depend on it)
	adjacency := make(map[string][]string)

	// Initialize structures for all nodes
	for nodeID := range wf.Nodes {
		inDegree[nodeID] = 0
		adjacency[nodeID] = []string{}
	}

	// Calculate in-degrees and build adjacency list
	for nodeID, node := range wf.Nodes {
		if node == nil {
			continue
		}

		// Count dependencies for this node
		inDegree[nodeID] = len(node.Dependencies)

		// Add this node to the adjacency list of each dependency
		for _, depID := range node.Dependencies {
			adjacency[depID] = append(adjacency[depID], nodeID)
		}
	}

	// Initialize queue with nodes that have no dependencies
	queue := make([]string, 0)
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	// Process nodes in topological order
	result := make([]string, 0, len(wf.Nodes))

	for len(queue) > 0 {
		// Dequeue first node
		nodeID := queue[0]
		queue = queue[1:]

		// Add to result
		result = append(result, nodeID)

		// Process all nodes that depend on this node
		for _, dependentID := range adjacency[nodeID] {
			// Decrement in-degree (we've "removed" the current node)
			inDegree[dependentID]--

			// If in-degree becomes zero, add to queue
			if inDegree[dependentID] == 0 {
				queue = append(queue, dependentID)
			}
		}
	}

	// Check if we processed all nodes (detect circular dependencies)
	if len(result) != len(wf.Nodes) {
		return nil, fmt.Errorf("circular dependency detected: processed %d nodes, expected %d", len(result), len(wf.Nodes))
	}

	return result, nil
}
