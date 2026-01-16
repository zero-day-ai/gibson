package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/queries"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

// Observer gathers execution state to build context for LLM reasoning.
// It queries the graph database to collect mission progress, workflow status,
// and recent findings to inform orchestrator decisions.
type Observer struct {
	missionQueries   *queries.MissionQueries
	executionQueries *queries.ExecutionQueries
}

// NewObserver creates a new Observer with the required query handlers.
// Both query dependencies are required and cannot be nil.
func NewObserver(missionQueries *queries.MissionQueries, executionQueries *queries.ExecutionQueries) *Observer {
	return &Observer{
		missionQueries:   missionQueries,
		executionQueries: executionQueries,
	}
}

// ObservationState contains all context needed for the LLM to make a decision.
// This struct is kept concise to avoid token bloat in prompts.
type ObservationState struct {
	// Mission metadata
	MissionInfo MissionInfo `json:"mission_info"`

	// Graph statistics summary
	GraphSummary GraphSummary `json:"graph_summary"`

	// Workflow node states by status
	ReadyNodes     []NodeSummary `json:"ready_nodes"`
	RunningNodes   []NodeSummary `json:"running_nodes"`
	CompletedNodes []NodeSummary `json:"completed_nodes"`
	FailedNodes    []NodeSummary `json:"failed_nodes"`

	// Recent execution history for context
	RecentDecisions []DecisionSummary `json:"recent_decisions"`

	// Resource and time constraints
	ResourceConstraints ResourceConstraints `json:"resource_constraints"`

	// Failed execution that triggered this observation (if any)
	FailedExecution *ExecutionFailure `json:"failed_execution,omitempty"`

	// Timestamp when this observation was captured
	ObservedAt time.Time `json:"observed_at"`
}

// MissionInfo contains essential mission metadata
type MissionInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Objective   string    `json:"objective"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	TimeElapsed string    `json:"time_elapsed"`
}

// GraphSummary provides high-level statistics about the attack graph state
type GraphSummary struct {
	TotalNodes      int `json:"total_nodes"`
	CompletedNodes  int `json:"completed_nodes"`
	FailedNodes     int `json:"failed_nodes"`
	PendingNodes    int `json:"pending_nodes"`
	TotalDecisions  int `json:"total_decisions"`
	TotalExecutions int `json:"total_executions"`
}

// NodeSummary is a concise representation of a workflow node
type NodeSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	AgentName   string `json:"agent_name,omitempty"`
	ToolName    string `json:"tool_name,omitempty"`
	Status      string `json:"status"`
	IsDynamic   bool   `json:"is_dynamic,omitempty"`
	Attempt     int    `json:"attempt,omitempty"`
}

// DecisionSummary captures key information from a past decision
type DecisionSummary struct {
	Iteration  int     `json:"iteration"`
	Action     string  `json:"action"`
	Target     string  `json:"target,omitempty"`
	Reasoning  string  `json:"reasoning"`
	Confidence float64 `json:"confidence"`
	Timestamp  string  `json:"timestamp"`
}

// ResourceConstraints tracks execution limits and budgets
type ResourceConstraints struct {
	MaxConcurrent    int           `json:"max_concurrent"`
	CurrentRunning   int           `json:"current_running"`
	TimeElapsed      time.Duration `json:"time_elapsed"`
	TotalIterations  int           `json:"total_iterations"`
	ExecutionBudget  *BudgetInfo   `json:"execution_budget,omitempty"`
	RemainingRetries int           `json:"remaining_retries,omitempty"`
}

// BudgetInfo tracks resource budgets (optional, for future use)
type BudgetInfo struct {
	MaxExecutions     int `json:"max_executions"`
	RemainingExecutions int `json:"remaining_executions"`
	MaxTokens         int `json:"max_tokens,omitempty"`
	UsedTokens        int `json:"used_tokens,omitempty"`
}

// ExecutionFailure captures details about a failed execution
type ExecutionFailure struct {
	// Existing fields
	NodeID      string    `json:"node_id"`
	NodeName    string    `json:"node_name"`
	AgentName   string    `json:"agent_name,omitempty"`
	Attempt     int       `json:"attempt"`
	Error       string    `json:"error"`
	FailedAt    time.Time `json:"failed_at"`
	CanRetry    bool      `json:"can_retry"`
	MaxRetries  int       `json:"max_retries"`

	// NEW: Structured error classification for semantic error recovery
	// ErrorClass categorizes the error (infrastructure/semantic/transient/permanent)
	ErrorClass string `json:"error_class,omitempty"`

	// ErrorCode provides a specific error identifier (BINARY_NOT_FOUND, TIMEOUT, etc.)
	ErrorCode string `json:"error_code,omitempty"`

	// RecoveryHints provides concrete alternatives and recovery strategies
	RecoveryHints []RecoveryHintSummary `json:"recovery_hints,omitempty"`

	// PartialResults contains any salvageable data from the failed execution
	PartialResults map[string]any `json:"partial_results,omitempty"`

	// FailureContext contains additional context about what was tried
	FailureContext map[string]any `json:"failure_context,omitempty"`
}

// RecoveryHintSummary is the orchestrator's representation of a recovery hint.
// This is a simplified version that avoids tight coupling to SDK types.
type RecoveryHintSummary struct {
	// Strategy indicates the type of recovery action (retry, use_alternative_tool, etc.)
	Strategy string `json:"strategy"`

	// Alternative specifies an alternative tool or agent name, if applicable
	Alternative string `json:"alternative,omitempty"`

	// Params contains suggested parameter modifications
	Params map[string]any `json:"params,omitempty"`

	// Reason explains why this recovery approach might succeed
	Reason string `json:"reason"`

	// Priority determines the order to try hints (lower = try first)
	Priority int `json:"priority,omitempty"`
}

// Observe gathers all execution state for the given mission and builds
// an ObservationState suitable for prompt construction.
// Returns an error if required data cannot be retrieved.
func (o *Observer) Observe(ctx context.Context, missionID string) (*ObservationState, error) {
	if o.missionQueries == nil || o.executionQueries == nil {
		return nil, fmt.Errorf("observer not properly initialized: missing query dependencies")
	}

	mid, err := types.ParseID(missionID)
	if err != nil {
		return nil, fmt.Errorf("invalid mission ID: %w", err)
	}

	// Build observation state by querying graph
	state := &ObservationState{
		ObservedAt: time.Now(),
	}

	// 1. Get mission info
	if err := o.observeMission(ctx, mid, state); err != nil {
		return nil, fmt.Errorf("failed to observe mission info: %w", err)
	}

	// 2. Get mission statistics
	if err := o.observeStats(ctx, mid, state); err != nil {
		return nil, fmt.Errorf("failed to observe mission stats: %w", err)
	}

	// 3. Get workflow nodes by status
	if err := o.observeNodes(ctx, mid, state); err != nil {
		return nil, fmt.Errorf("failed to observe workflow nodes: %w", err)
	}

	// 4. Get recent decisions for context
	if err := o.observeDecisions(ctx, mid, state); err != nil {
		return nil, fmt.Errorf("failed to observe recent decisions: %w", err)
	}

	// 5. Calculate resource constraints
	o.calculateResourceConstraints(state)

	return state, nil
}

// observeMission retrieves mission metadata
func (o *Observer) observeMission(ctx context.Context, missionID types.ID, state *ObservationState) error {
	mission, err := o.missionQueries.GetMission(ctx, missionID)
	if err != nil {
		return fmt.Errorf("failed to get mission: %w", err)
	}

	state.MissionInfo = MissionInfo{
		ID:        mission.ID.String(),
		Name:      mission.Name,
		Objective: mission.Objective,
		Status:    mission.Status.String(),
	}

	if mission.StartedAt != nil {
		state.MissionInfo.StartedAt = *mission.StartedAt
		elapsed := time.Since(*mission.StartedAt)
		state.MissionInfo.TimeElapsed = formatDuration(elapsed)
	}

	return nil
}

// observeStats retrieves mission execution statistics
func (o *Observer) observeStats(ctx context.Context, missionID types.ID, state *ObservationState) error {
	stats, err := o.missionQueries.GetMissionStats(ctx, missionID)
	if err != nil {
		return fmt.Errorf("failed to get mission stats: %w", err)
	}

	state.GraphSummary = GraphSummary{
		TotalNodes:      stats.TotalNodes,
		CompletedNodes:  stats.CompletedNodes,
		FailedNodes:     stats.FailedNodes,
		PendingNodes:    stats.PendingNodes,
		TotalDecisions:  stats.TotalDecisions,
		TotalExecutions: stats.TotalExecutions,
	}

	return nil
}

// observeNodes retrieves and categorizes workflow nodes by status
func (o *Observer) observeNodes(ctx context.Context, missionID types.ID, state *ObservationState) error {
	// Get all nodes for the mission
	nodes, err := o.missionQueries.GetMissionNodes(ctx, missionID)
	if err != nil {
		return fmt.Errorf("failed to get mission nodes: %w", err)
	}

	// Categorize nodes by status
	for _, node := range nodes {
		summary := nodeToSummary(node)

		// Add attempt count for failed/running nodes
		if node.Status == schema.WorkflowNodeStatusFailed ||
		   node.Status == schema.WorkflowNodeStatusRunning {
			executions, err := o.missionQueries.GetNodeExecutions(ctx, node.ID)
			if err == nil && len(executions) > 0 {
				summary.Attempt = executions[len(executions)-1].Attempt
			}
		}

		switch node.Status {
		case schema.WorkflowNodeStatusReady:
			state.ReadyNodes = append(state.ReadyNodes, summary)
		case schema.WorkflowNodeStatusRunning:
			state.RunningNodes = append(state.RunningNodes, summary)
		case schema.WorkflowNodeStatusCompleted:
			state.CompletedNodes = append(state.CompletedNodes, summary)
		case schema.WorkflowNodeStatusFailed:
			state.FailedNodes = append(state.FailedNodes, summary)
		}
	}

	// Initialize empty slices if nil
	if state.ReadyNodes == nil {
		state.ReadyNodes = []NodeSummary{}
	}
	if state.RunningNodes == nil {
		state.RunningNodes = []NodeSummary{}
	}
	if state.CompletedNodes == nil {
		state.CompletedNodes = []NodeSummary{}
	}
	if state.FailedNodes == nil {
		state.FailedNodes = []NodeSummary{}
	}

	return nil
}

// observeDecisions retrieves recent orchestrator decisions for context
func (o *Observer) observeDecisions(ctx context.Context, missionID types.ID, state *ObservationState) error {
	decisions, err := o.missionQueries.GetMissionDecisions(ctx, missionID)
	if err != nil {
		return fmt.Errorf("failed to get decisions: %w", err)
	}

	// Take last N decisions (most recent context)
	const maxRecentDecisions = 5
	start := 0
	if len(decisions) > maxRecentDecisions {
		start = len(decisions) - maxRecentDecisions
	}

	state.RecentDecisions = make([]DecisionSummary, 0, maxRecentDecisions)
	for _, decision := range decisions[start:] {
		state.RecentDecisions = append(state.RecentDecisions, DecisionSummary{
			Iteration:  decision.Iteration,
			Action:     decision.Action.String(),
			Target:     decision.TargetNodeID,
			Reasoning:  truncateString(decision.Reasoning, 200),
			Confidence: decision.Confidence,
			Timestamp:  decision.Timestamp.Format(time.RFC3339),
		})
	}

	if state.RecentDecisions == nil {
		state.RecentDecisions = []DecisionSummary{}
	}

	return nil
}

// calculateResourceConstraints computes resource usage and limits
func (o *Observer) calculateResourceConstraints(state *ObservationState) {
	state.ResourceConstraints = ResourceConstraints{
		MaxConcurrent:   10, // Default, should be configurable
		CurrentRunning:  len(state.RunningNodes),
		TotalIterations: len(state.RecentDecisions),
	}

	// Calculate time elapsed
	if !state.MissionInfo.StartedAt.IsZero() {
		state.ResourceConstraints.TimeElapsed = time.Since(state.MissionInfo.StartedAt)
	}

	// Calculate remaining retries for failed nodes
	remainingRetries := 0
	for _, node := range state.FailedNodes {
		// Assume max 3 retries by default
		if node.Attempt < 3 {
			remainingRetries++
		}
	}
	state.ResourceConstraints.RemainingRetries = remainingRetries
}

// ObserveWithFailure is a convenience method that captures a failed execution
// and builds the observation state with that context included.
func (o *Observer) ObserveWithFailure(ctx context.Context, missionID string, failedNodeID string) (*ObservationState, error) {
	state, err := o.Observe(ctx, missionID)
	if err != nil {
		return nil, err
	}

	// Find the failed node and get execution details
	mid, err := types.ParseID(missionID)
	if err != nil {
		return nil, fmt.Errorf("invalid mission ID: %w", err)
	}

	nid, err := types.ParseID(failedNodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid node ID: %w", err)
	}

	// Get node details
	nodes, err := o.missionQueries.GetMissionNodes(ctx, mid)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	var failedNode *schema.WorkflowNode
	for _, node := range nodes {
		if node.ID == nid {
			failedNode = node
			break
		}
	}

	if failedNode == nil {
		return nil, fmt.Errorf("failed node %s not found", failedNodeID)
	}

	// Get execution details
	executions, err := o.missionQueries.GetNodeExecutions(ctx, nid)
	if err != nil {
		return nil, fmt.Errorf("failed to get node executions: %w", err)
	}

	if len(executions) == 0 {
		return state, nil // No executions yet
	}

	lastExec := executions[len(executions)-1]
	maxRetries := 3 // Default
	if failedNode.RetryPolicy != nil {
		maxRetries = failedNode.RetryPolicy.MaxRetries
	}

	state.FailedExecution = &ExecutionFailure{
		NodeID:     failedNode.ID.String(),
		NodeName:   failedNode.Name,
		AgentName:  failedNode.AgentName,
		Attempt:    lastExec.Attempt,
		Error:      lastExec.Error,
		FailedAt:   *lastExec.CompletedAt,
		CanRetry:   lastExec.Attempt < maxRetries,
		MaxRetries: maxRetries,
	}

	return state, nil
}

// FormatForPrompt converts the observation state into a well-formatted string
// suitable for inclusion in an LLM prompt.
func (s *ObservationState) FormatForPrompt() string {
	var sb strings.Builder

	// Mission context
	sb.WriteString("=== MISSION CONTEXT ===\n")
	sb.WriteString(fmt.Sprintf("Mission: %s\n", s.MissionInfo.Name))
	sb.WriteString(fmt.Sprintf("Objective: %s\n", s.MissionInfo.Objective))
	sb.WriteString(fmt.Sprintf("Status: %s\n", s.MissionInfo.Status))
	sb.WriteString(fmt.Sprintf("Time Elapsed: %s\n", s.MissionInfo.TimeElapsed))
	sb.WriteString("\n")

	// Workflow progress
	sb.WriteString("=== WORKFLOW PROGRESS ===\n")
	sb.WriteString(fmt.Sprintf("Total Nodes: %d\n", s.GraphSummary.TotalNodes))
	sb.WriteString(fmt.Sprintf("Completed: %d\n", s.GraphSummary.CompletedNodes))
	sb.WriteString(fmt.Sprintf("Failed: %d\n", s.GraphSummary.FailedNodes))
	sb.WriteString(fmt.Sprintf("Pending: %d\n", s.GraphSummary.PendingNodes))
	sb.WriteString(fmt.Sprintf("Total Decisions: %d\n", s.GraphSummary.TotalDecisions))
	sb.WriteString("\n")

	// Resource constraints
	sb.WriteString("=== RESOURCE CONSTRAINTS ===\n")
	sb.WriteString(fmt.Sprintf("Max Concurrent: %d\n", s.ResourceConstraints.MaxConcurrent))
	sb.WriteString(fmt.Sprintf("Currently Running: %d\n", s.ResourceConstraints.CurrentRunning))
	sb.WriteString(fmt.Sprintf("Total Iterations: %d\n", s.ResourceConstraints.TotalIterations))
	if s.ResourceConstraints.RemainingRetries > 0 {
		sb.WriteString(fmt.Sprintf("Nodes Available for Retry: %d\n", s.ResourceConstraints.RemainingRetries))
	}
	sb.WriteString("\n")

	// Ready nodes (most important for decision making)
	if len(s.ReadyNodes) > 0 {
		sb.WriteString("=== READY NODES (Can Execute Now) ===\n")
		for _, node := range s.ReadyNodes {
			sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", node.ID, node.Name, node.Description))
			if node.AgentName != "" {
				sb.WriteString(fmt.Sprintf("  Agent: %s\n", node.AgentName))
			}
		}
		sb.WriteString("\n")
	}

	// Running nodes
	if len(s.RunningNodes) > 0 {
		sb.WriteString("=== RUNNING NODES ===\n")
		for _, node := range s.RunningNodes {
			sb.WriteString(fmt.Sprintf("- [%s] %s", node.ID, node.Name))
			if node.Attempt > 1 {
				sb.WriteString(fmt.Sprintf(" (attempt %d)", node.Attempt))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Failed nodes
	if len(s.FailedNodes) > 0 {
		sb.WriteString("=== FAILED NODES ===\n")
		for _, node := range s.FailedNodes {
			sb.WriteString(fmt.Sprintf("- [%s] %s", node.ID, node.Name))
			if node.Attempt > 0 {
				sb.WriteString(fmt.Sprintf(" (attempt %d)", node.Attempt))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Failed execution context (if present)
	if s.FailedExecution != nil {
		f := s.FailedExecution
		sb.WriteString("=== RECENT FAILURE ===\n")
		sb.WriteString(fmt.Sprintf("Node: %s (%s)\n", f.NodeName, f.NodeID))
		if f.AgentName != "" {
			sb.WriteString(fmt.Sprintf("Agent: %s\n", f.AgentName))
		}
		sb.WriteString(fmt.Sprintf("Attempt: %d/%d\n", f.Attempt, f.MaxRetries))

		// NEW: Error classification
		if f.ErrorCode != "" {
			sb.WriteString(fmt.Sprintf("Error Code: %s\n", f.ErrorCode))
		}
		if f.ErrorClass != "" {
			sb.WriteString(fmt.Sprintf("Error Class: %s\n", f.ErrorClass))
		}
		sb.WriteString(fmt.Sprintf("Error: %s\n", truncateString(f.Error, 300)))
		sb.WriteString(fmt.Sprintf("Can Retry: %v\n", f.CanRetry))

		// NEW: Recovery hints
		if len(f.RecoveryHints) > 0 {
			sb.WriteString("\nRecovery Options:\n")
			for i, hint := range f.RecoveryHints {
				sb.WriteString(fmt.Sprintf("%d. [%s]", i+1, hint.Strategy))
				if hint.Alternative != "" {
					sb.WriteString(fmt.Sprintf(" %s", hint.Alternative))
				}
				sb.WriteString(fmt.Sprintf(" - %s\n", hint.Reason))
				if len(hint.Params) > 0 {
					sb.WriteString(fmt.Sprintf("   Suggested params: %v\n", hint.Params))
				}
			}
		}

		// NEW: Partial results
		if len(f.PartialResults) > 0 {
			sb.WriteString("\nPartial Results Recovered:\n")
			for k, v := range f.PartialResults {
				sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
			}
		}

		// NEW: Failure context
		if len(f.FailureContext) > 0 {
			sb.WriteString("\nFailure Context:\n")
			for k, v := range f.FailureContext {
				sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
			}
		}

		sb.WriteString("\n")
	}

	// Recent decisions for context
	if len(s.RecentDecisions) > 0 {
		sb.WriteString("=== RECENT DECISIONS ===\n")
		for _, dec := range s.RecentDecisions {
			sb.WriteString(fmt.Sprintf("Iteration %d: %s", dec.Iteration, dec.Action))
			if dec.Target != "" {
				sb.WriteString(fmt.Sprintf(" -> %s", dec.Target))
			}
			sb.WriteString(fmt.Sprintf(" (confidence: %.2f)\n", dec.Confidence))
			if dec.Reasoning != "" {
				sb.WriteString(fmt.Sprintf("  Reasoning: %s\n", dec.Reasoning))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// Helper functions

// nodeToSummary converts a schema.WorkflowNode to a concise NodeSummary
func nodeToSummary(node *schema.WorkflowNode) NodeSummary {
	return NodeSummary{
		ID:          node.ID.String(),
		Name:        node.Name,
		Type:        node.Type.String(),
		Description: node.Description,
		AgentName:   node.AgentName,
		ToolName:    node.ToolName,
		Status:      node.Status.String(),
		IsDynamic:   node.IsDynamic,
	}
}

// formatDuration formats a duration into a human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// truncateString truncates a string to the specified length with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..."
	}
	return s[:maxLen-3] + "..."
}
