// Package workflow provides snapshot functionality for converting graph state
// back to YAML-compatible workflow snapshots.
//
// The SnapshotBuilder reconstructs workflow definitions from Neo4j graph state,
// including execution history, dynamic nodes, and orchestrator decisions.
// This enables:
//   - Post-execution analysis and replay
//   - Debugging workflow execution paths
//   - Auditing orchestrator decisions
//   - Re-parsing modified workflows
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/queries"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
	"gopkg.in/yaml.v3"
)

// SnapshotBuilder constructs workflow snapshots from graph state.
// It queries Neo4j for mission data and reconstructs the workflow
// with execution history and orchestrator decision context.
type SnapshotBuilder struct {
	missionQueries *queries.MissionQueries
	execQueries    *queries.ExecutionQueries
}

// NewSnapshotBuilder creates a new SnapshotBuilder with the required query clients.
//
// Parameters:
//   - missionQueries: Query interface for mission and workflow node data
//   - execQueries: Query interface for execution and decision tracking data
//
// Returns:
//   - *SnapshotBuilder: Configured snapshot builder ready for use
func NewSnapshotBuilder(missionQueries *queries.MissionQueries, execQueries *queries.ExecutionQueries) *SnapshotBuilder {
	return &SnapshotBuilder{
		missionQueries: missionQueries,
		execQueries:    execQueries,
	}
}

// SnapshotOptions controls what data is included in the snapshot.
// Use these flags to tailor snapshots for specific use cases:
//   - Debugging: IncludeHistory + IncludeDecisions
//   - Replay: IncludeOriginalYAML + minimal other flags
//   - Audit: All flags enabled
type SnapshotOptions struct {
	// IncludeHistory includes full execution history for each node
	// This adds ExecutionHistory to each NodeSnapshot with timing,
	// status, retry attempts, and results
	IncludeHistory bool

	// IncludeDecisions includes orchestrator decision reasoning
	// This captures the decision-making process, confidence scores,
	// and parameter modifications
	IncludeDecisions bool

	// IncludeOriginalYAML includes the original workflow YAML source
	// Useful for comparing original vs. actual execution
	IncludeOriginalYAML bool
}

// DefaultSnapshotOptions returns snapshot options with all features enabled.
// This is the recommended starting point for most use cases.
func DefaultSnapshotOptions() SnapshotOptions {
	return SnapshotOptions{
		IncludeHistory:      true,
		IncludeDecisions:    true,
		IncludeOriginalYAML: true,
	}
}

// MinimalSnapshotOptions returns options for minimal snapshots (structure only).
// Useful for lightweight status checks or workflow structure analysis.
func MinimalSnapshotOptions() SnapshotOptions {
	return SnapshotOptions{
		IncludeHistory:      false,
		IncludeDecisions:    false,
		IncludeOriginalYAML: false,
	}
}

// WorkflowSnapshot is a top-level snapshot containing mission, nodes, decisions and stats.
// This is used by the diff builder to compare workflow states.
type WorkflowSnapshot struct {
	MissionID string              `json:"mission_id" yaml:"mission_id"`
	Nodes     []*NodeSnapshot     `json:"nodes" yaml:"nodes"`
	Decisions []*DecisionSnapshot `json:"decisions,omitempty" yaml:"decisions,omitempty"`
}

// MissionSnapshot represents a complete workflow state snapshot.
// This structure is designed to be serializable to both YAML and JSON,
// providing a human-readable and machine-parseable format.
// Note: This is different from MissionSnapshot in diff.go which is used for diffing.
type MissionSnapshot struct {
	// Workflow metadata
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Version     string `json:"version,omitempty" yaml:"version,omitempty"`
	TargetRef   string `json:"target_ref,omitempty" yaml:"target_ref,omitempty"`

	// Mission tracking
	MissionID     string              `json:"mission_id" yaml:"mission_id"`
	MissionStatus schema.MissionStatus `json:"mission_status" yaml:"mission_status"`

	// Workflow structure with execution state
	Nodes []*NodeSnapshot `json:"nodes" yaml:"nodes"`

	// Execution summary
	Metadata *SnapshotMetadata `json:"metadata" yaml:"metadata"`

	// Orchestrator decisions (if IncludeDecisions)
	Decisions []*DecisionSnapshot `json:"decisions,omitempty" yaml:"decisions,omitempty"`

	// Original YAML source (if IncludeOriginalYAML)
	OriginalYAML string `json:"original_yaml,omitempty" yaml:"original_yaml,omitempty"`

	// Snapshot generation timestamp
	SnapshotTime time.Time `json:"snapshot_time" yaml:"snapshot_time"`
}

// SnapshotMetadata provides execution summary statistics.
type SnapshotMetadata struct {
	// Node statistics
	TotalNodes     int `json:"total_nodes" yaml:"total_nodes"`
	CompletedNodes int `json:"completed_nodes" yaml:"completed_nodes"`
	FailedNodes    int `json:"failed_nodes" yaml:"failed_nodes"`
	PendingNodes   int `json:"pending_nodes" yaml:"pending_nodes"`
	DynamicNodes   int `json:"dynamic_nodes" yaml:"dynamic_nodes"`

	// Execution statistics
	TotalExecutions int `json:"total_executions" yaml:"total_executions"`
	TotalDecisions  int `json:"total_decisions" yaml:"total_decisions"`

	// Timing information
	MissionStartTime   *time.Time     `json:"mission_start_time,omitempty" yaml:"mission_start_time,omitempty"`
	MissionEndTime     *time.Time     `json:"mission_end_time,omitempty" yaml:"mission_end_time,omitempty"`
	TotalDuration      *time.Duration `json:"total_duration,omitempty" yaml:"total_duration,omitempty"`
	AverageNodeRuntime *time.Duration `json:"average_node_runtime,omitempty" yaml:"average_node_runtime,omitempty"`
}

// NodeSnapshot represents a workflow node with execution state.
type NodeSnapshot struct {
	// Core node identity
	ID          string                    `json:"id" yaml:"id"`
	Type        schema.WorkflowNodeType   `json:"type" yaml:"type"`
	Name        string                    `json:"name" yaml:"name"`
	Description string                    `json:"description,omitempty" yaml:"description,omitempty"`
	Status      schema.WorkflowNodeStatus `json:"status" yaml:"status"`

	// Node type-specific fields
	AgentName string `json:"agent_name,omitempty" yaml:"agent_name,omitempty"`
	ToolName  string `json:"tool_name,omitempty" yaml:"tool_name,omitempty"`

	// Dynamic node tracking
	IsDynamic bool   `json:"is_dynamic" yaml:"is_dynamic"`
	SpawnedBy string `json:"spawned_by,omitempty" yaml:"spawned_by,omitempty"`

	// Original configuration
	OriginalConfig map[string]any `json:"original_config,omitempty" yaml:"original_config,omitempty"`

	// Modified configuration (if changed by orchestrator)
	ModifiedConfig       map[string]any `json:"modified_config,omitempty" yaml:"modified_config,omitempty"`
	ModificationReason   string         `json:"modification_reason,omitempty" yaml:"modification_reason,omitempty"`
	ConfigModified       bool           `json:"config_modified" yaml:"config_modified"`

	// Execution timing
	FirstExecutionTime *time.Time     `json:"first_execution_time,omitempty" yaml:"first_execution_time,omitempty"`
	LastExecutionTime  *time.Time     `json:"last_execution_time,omitempty" yaml:"last_execution_time,omitempty"`
	TotalRuntime       *time.Duration `json:"total_runtime,omitempty" yaml:"total_runtime,omitempty"`

	// Execution history (if IncludeHistory)
	ExecutionHistory []*ExecutionSnapshot `json:"execution_history,omitempty" yaml:"execution_history,omitempty"`

	// Retry policy
	RetryPolicy *schema.RetryPolicy `json:"retry_policy,omitempty" yaml:"retry_policy,omitempty"`
	Timeout     *time.Duration      `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// Dependencies
	Dependencies []string `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
}

// ExecutionSnapshot captures a single execution attempt of a node.
type ExecutionSnapshot struct {
	// Execution identity
	ExecutionID string                 `json:"execution_id" yaml:"execution_id"`
	Attempt     int                    `json:"attempt" yaml:"attempt"`
	Status      schema.ExecutionStatus `json:"status" yaml:"status"`

	// Timing
	StartedAt   time.Time     `json:"started_at" yaml:"started_at"`
	CompletedAt *time.Time    `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	Duration    time.Duration `json:"duration" yaml:"duration"`

	// Configuration used (may differ from original if modified)
	ConfigUsed map[string]any `json:"config_used,omitempty" yaml:"config_used,omitempty"`

	// Results
	Result map[string]any `json:"result,omitempty" yaml:"result,omitempty"`
	Error  string         `json:"error,omitempty" yaml:"error,omitempty"`

	// Observability correlation
	LangfuseSpanID string `json:"langfuse_span_id,omitempty" yaml:"langfuse_span_id,omitempty"`

	// Tool executions (if applicable)
	ToolExecutions []*ToolExecutionSnapshot `json:"tool_executions,omitempty" yaml:"tool_executions,omitempty"`
}

// ToolExecutionSnapshot captures a tool invocation within an agent execution.
type ToolExecutionSnapshot struct {
	ToolName    string                 `json:"tool_name" yaml:"tool_name"`
	Status      schema.ExecutionStatus `json:"status" yaml:"status"`
	StartedAt   time.Time              `json:"started_at" yaml:"started_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	Duration    time.Duration          `json:"duration" yaml:"duration"`
	Input       map[string]any         `json:"input,omitempty" yaml:"input,omitempty"`
	Output      map[string]any         `json:"output,omitempty" yaml:"output,omitempty"`
	Error       string                 `json:"error,omitempty" yaml:"error,omitempty"`
}

// DecisionSnapshot captures an orchestrator decision with reasoning.
type DecisionSnapshot struct {
	// Decision identity
	DecisionID string                  `json:"decision_id" yaml:"decision_id"`
	Iteration  int                     `json:"iteration" yaml:"iteration"`
	Timestamp  time.Time               `json:"timestamp" yaml:"timestamp"`
	Action     schema.DecisionAction   `json:"action" yaml:"action"`

	// Target node (if applicable)
	TargetNodeID string `json:"target_node_id,omitempty" yaml:"target_node_id,omitempty"`

	// Decision reasoning
	Reasoning  string  `json:"reasoning" yaml:"reasoning"`
	Confidence float64 `json:"confidence" yaml:"confidence"`

	// Modifications made (if action is modify_params)
	Modifications map[string]any `json:"modifications,omitempty" yaml:"modifications,omitempty"`

	// LLM metrics
	PromptTokens     int `json:"prompt_tokens,omitempty" yaml:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty" yaml:"completion_tokens,omitempty"`
	LatencyMs        int `json:"latency_ms,omitempty" yaml:"latency_ms,omitempty"`

	// Observability correlation
	LangfuseSpanID string `json:"langfuse_span_id,omitempty" yaml:"langfuse_span_id,omitempty"`
}

// BuildSnapshot constructs a complete workflow snapshot from graph state.
//
// The snapshot includes:
//   - Original workflow structure with current node statuses
//   - Execution history for each node (if IncludeHistory)
//   - Orchestrator decisions with reasoning (if IncludeDecisions)
//   - Original YAML source (if IncludeOriginalYAML)
//   - Execution timing and statistics
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - missionID: The mission ID to snapshot (must be valid UUID string)
//   - opts: Options controlling snapshot content
//
// Returns:
//   - *MissionSnapshot: Complete workflow snapshot
//   - error: Any error encountered during snapshot construction
func (sb *SnapshotBuilder) BuildSnapshot(ctx context.Context, missionID string, opts SnapshotOptions) (*MissionSnapshot, error) {
	// Validate and parse mission ID
	parsedMissionID, err := types.ParseID(missionID)
	if err != nil {
		return nil, fmt.Errorf("invalid mission ID: %w", err)
	}

	// Fetch mission data
	mission, err := sb.missionQueries.GetMission(ctx, parsedMissionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch mission: %w", err)
	}

	// Initialize snapshot
	snapshot := &MissionSnapshot{
		MissionID:     missionID,
		MissionStatus: mission.Status,
		SnapshotTime:  time.Now(),
		Nodes:         []*NodeSnapshot{},
		Decisions:     []*DecisionSnapshot{},
	}

	// Extract workflow metadata from mission
	if err := sb.extractWorkflowMetadata(mission, snapshot); err != nil {
		return nil, fmt.Errorf("failed to extract workflow metadata: %w", err)
	}

	// Fetch workflow nodes
	nodes, err := sb.missionQueries.GetMissionNodes(ctx, parsedMissionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch workflow nodes: %w", err)
	}

	// Build node snapshots
	for _, node := range nodes {
		nodeSnapshot, err := sb.buildNodeSnapshot(ctx, node, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to build snapshot for node %s: %w", node.ID, err)
		}
		snapshot.Nodes = append(snapshot.Nodes, nodeSnapshot)
	}

	// Include decisions if requested
	if opts.IncludeDecisions {
		decisions, err := sb.missionQueries.GetMissionDecisions(ctx, parsedMissionID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch decisions: %w", err)
		}

		for _, decision := range decisions {
			decisionSnapshot := sb.buildDecisionSnapshot(decision)
			snapshot.Decisions = append(snapshot.Decisions, decisionSnapshot)
		}
	}

	// Compute metadata statistics
	snapshot.Metadata = sb.computeMetadata(mission, snapshot.Nodes, snapshot.Decisions)

	// Include original YAML if requested
	if opts.IncludeOriginalYAML && mission.YAMLSource != "" {
		snapshot.OriginalYAML = mission.YAMLSource
	}

	return snapshot, nil
}

// extractWorkflowMetadata extracts workflow metadata from the mission's YAML source.
func (sb *SnapshotBuilder) extractWorkflowMetadata(mission *schema.Mission, snapshot *MissionSnapshot) error {
	// Mission name and description
	snapshot.Name = mission.Name
	snapshot.Description = mission.Description
	snapshot.TargetRef = mission.TargetRef

	// Try to extract additional metadata from YAML source
	if mission.YAMLSource != "" {
		var yamlData map[string]any
		if err := json.Unmarshal([]byte(mission.YAMLSource), &yamlData); err == nil {
			if version, ok := yamlData["version"].(string); ok {
				snapshot.Version = version
			}
		}
	}

	return nil
}

// buildNodeSnapshot constructs a snapshot for a single workflow node.
func (sb *SnapshotBuilder) buildNodeSnapshot(ctx context.Context, node *schema.WorkflowNode, opts SnapshotOptions) (*NodeSnapshot, error) {
	// Fetch dependencies from graph relationships
	deps, err := sb.missionQueries.GetNodeDependencies(ctx, node.ID)
	if err != nil {
		// Non-fatal: continue without dependencies if query fails
		deps = nil
	}
	depIDs := make([]string, 0, len(deps))
	for _, dep := range deps {
		depIDs = append(depIDs, dep.ID.String())
	}

	nodeSnapshot := &NodeSnapshot{
		ID:             node.ID.String(),
		Type:           node.Type,
		Name:           node.Name,
		Description:    node.Description,
		Status:         node.Status,
		IsDynamic:      node.IsDynamic,
		SpawnedBy:      node.SpawnedBy,
		OriginalConfig: node.TaskConfig,
		RetryPolicy:    node.RetryPolicy,
		Dependencies:   depIDs,
	}

	// Set type-specific fields
	if node.Type == schema.WorkflowNodeTypeAgent {
		nodeSnapshot.AgentName = node.AgentName
	} else if node.Type == schema.WorkflowNodeTypeTool {
		nodeSnapshot.ToolName = node.ToolName
	}

	// Set timeout
	if node.Timeout > 0 {
		timeout := node.Timeout
		nodeSnapshot.Timeout = &timeout
	}

	// Include execution history if requested
	if opts.IncludeHistory {
		executions, err := sb.missionQueries.GetNodeExecutions(ctx, node.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch executions for node %s: %w", node.ID, err)
		}

		// Build execution snapshots
		for _, exec := range executions {
			execSnapshot, err := sb.buildExecutionSnapshot(ctx, exec)
			if err != nil {
				return nil, fmt.Errorf("failed to build execution snapshot: %w", err)
			}
			nodeSnapshot.ExecutionHistory = append(nodeSnapshot.ExecutionHistory, execSnapshot)
		}

		// Compute timing statistics from execution history
		sb.computeNodeTiming(nodeSnapshot)

		// Check for configuration modifications
		sb.detectConfigModifications(nodeSnapshot)
	}

	return nodeSnapshot, nil
}

// buildExecutionSnapshot constructs a snapshot for a single execution attempt.
func (sb *SnapshotBuilder) buildExecutionSnapshot(ctx context.Context, exec *schema.AgentExecution) (*ExecutionSnapshot, error) {
	execSnapshot := &ExecutionSnapshot{
		ExecutionID:    exec.ID.String(),
		Attempt:        exec.Attempt,
		Status:         exec.Status,
		StartedAt:      exec.StartedAt,
		CompletedAt:    exec.CompletedAt,
		ConfigUsed:     exec.ConfigUsed,
		Result:         exec.Result,
		Error:          exec.Error,
		LangfuseSpanID: exec.LangfuseSpanID,
		ToolExecutions: []*ToolExecutionSnapshot{},
	}

	// Calculate duration
	if exec.CompletedAt != nil {
		duration := exec.CompletedAt.Sub(exec.StartedAt)
		execSnapshot.Duration = duration
	} else {
		// Still running - calculate elapsed time
		execSnapshot.Duration = time.Since(exec.StartedAt)
	}

	// Fetch tool executions if available
	tools, err := sb.execQueries.GetExecutionTools(ctx, exec.ID.String())
	if err == nil && len(tools) > 0 {
		for _, tool := range tools {
			toolSnapshot := sb.buildToolExecutionSnapshot(tool)
			execSnapshot.ToolExecutions = append(execSnapshot.ToolExecutions, toolSnapshot)
		}
	}

	return execSnapshot, nil
}

// buildToolExecutionSnapshot constructs a snapshot for a tool execution.
func (sb *SnapshotBuilder) buildToolExecutionSnapshot(tool *schema.ToolExecution) *ToolExecutionSnapshot {
	toolSnapshot := &ToolExecutionSnapshot{
		ToolName:    tool.ToolName,
		Status:      tool.Status,
		StartedAt:   tool.StartedAt,
		CompletedAt: tool.CompletedAt,
		Input:       tool.Input,
		Output:      tool.Output,
		Error:       tool.Error,
	}

	// Calculate duration
	if tool.CompletedAt != nil {
		toolSnapshot.Duration = tool.CompletedAt.Sub(tool.StartedAt)
	} else {
		toolSnapshot.Duration = time.Since(tool.StartedAt)
	}

	return toolSnapshot
}

// buildDecisionSnapshot constructs a snapshot for an orchestrator decision.
func (sb *SnapshotBuilder) buildDecisionSnapshot(decision *schema.Decision) *DecisionSnapshot {
	return &DecisionSnapshot{
		DecisionID:       decision.ID.String(),
		Iteration:        decision.Iteration,
		Timestamp:        decision.Timestamp,
		Action:           decision.Action,
		TargetNodeID:     decision.TargetNodeID,
		Reasoning:        decision.Reasoning,
		Confidence:       decision.Confidence,
		Modifications:    decision.Modifications,
		PromptTokens:     decision.PromptTokens,
		CompletionTokens: decision.CompletionTokens,
		LatencyMs:        decision.LatencyMs,
		LangfuseSpanID:   decision.LangfuseSpanID,
	}
}

// computeNodeTiming calculates timing statistics from execution history.
func (sb *SnapshotBuilder) computeNodeTiming(nodeSnapshot *NodeSnapshot) {
	if len(nodeSnapshot.ExecutionHistory) == 0 {
		return
	}

	// Find first and last execution times
	first := nodeSnapshot.ExecutionHistory[0]
	last := nodeSnapshot.ExecutionHistory[len(nodeSnapshot.ExecutionHistory)-1]

	nodeSnapshot.FirstExecutionTime = &first.StartedAt
	if last.CompletedAt != nil {
		nodeSnapshot.LastExecutionTime = last.CompletedAt
	}

	// Calculate total runtime across all executions
	var totalRuntime time.Duration
	for _, exec := range nodeSnapshot.ExecutionHistory {
		totalRuntime += exec.Duration
	}
	nodeSnapshot.TotalRuntime = &totalRuntime
}

// detectConfigModifications checks if the configuration was modified during execution.
func (sb *SnapshotBuilder) detectConfigModifications(nodeSnapshot *NodeSnapshot) {
	if len(nodeSnapshot.ExecutionHistory) == 0 {
		return
	}

	// Compare original config with used configs
	originalJSON, _ := json.Marshal(nodeSnapshot.OriginalConfig)
	originalStr := string(originalJSON)

	for _, exec := range nodeSnapshot.ExecutionHistory {
		if exec.ConfigUsed != nil {
			usedJSON, _ := json.Marshal(exec.ConfigUsed)
			usedStr := string(usedJSON)

			if originalStr != usedStr {
				nodeSnapshot.ConfigModified = true
				nodeSnapshot.ModifiedConfig = exec.ConfigUsed
				// Reason will be populated from decision context if available
				nodeSnapshot.ModificationReason = "Configuration modified by orchestrator during execution"
				break
			}
		}
	}
}

// computeMetadata calculates summary statistics for the snapshot.
func (sb *SnapshotBuilder) computeMetadata(mission *schema.Mission, nodes []*NodeSnapshot, decisions []*DecisionSnapshot) *SnapshotMetadata {
	metadata := &SnapshotMetadata{
		TotalNodes:      len(nodes),
		CompletedNodes:  0,
		FailedNodes:     0,
		PendingNodes:    0,
		DynamicNodes:    0,
		TotalExecutions: 0,
		TotalDecisions:  len(decisions),
	}

	// Count node statuses and executions
	var totalDuration time.Duration
	var completedWithDuration int

	for _, node := range nodes {
		switch node.Status {
		case schema.WorkflowNodeStatusCompleted:
			metadata.CompletedNodes++
		case schema.WorkflowNodeStatusFailed:
			metadata.FailedNodes++
		case schema.WorkflowNodeStatusPending, schema.WorkflowNodeStatusReady:
			metadata.PendingNodes++
		}

		if node.IsDynamic {
			metadata.DynamicNodes++
		}

		metadata.TotalExecutions += len(node.ExecutionHistory)

		// Aggregate node runtimes for average calculation
		if node.TotalRuntime != nil && *node.TotalRuntime > 0 {
			totalDuration += *node.TotalRuntime
			completedWithDuration++
		}
	}

	// Calculate average node runtime
	if completedWithDuration > 0 {
		avgDuration := totalDuration / time.Duration(completedWithDuration)
		metadata.AverageNodeRuntime = &avgDuration
	}

	// Mission timing
	metadata.MissionStartTime = mission.StartedAt
	metadata.MissionEndTime = mission.CompletedAt

	if mission.StartedAt != nil && mission.CompletedAt != nil {
		duration := mission.CompletedAt.Sub(*mission.StartedAt)
		metadata.TotalDuration = &duration
	}

	return metadata
}

// ToYAML serializes the workflow snapshot to YAML format.
// The output is designed to be human-readable and compatible with
// standard YAML workflow parsers.
func (ws *MissionSnapshot) ToYAML() ([]byte, error) {
	data, err := yaml.Marshal(ws)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal snapshot to YAML: %w", err)
	}
	return data, nil
}

// ToJSON serializes the workflow snapshot to JSON format.
// The output is compact and suitable for API responses or storage.
func (ws *MissionSnapshot) ToJSON() ([]byte, error) {
	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal snapshot to JSON: %w", err)
	}
	return data, nil
}

// ToYAMLString is a convenience method that returns the YAML as a string.
func (ws *MissionSnapshot) ToYAMLString() (string, error) {
	data, err := ws.ToYAML()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ToJSONString is a convenience method that returns the JSON as a string.
func (ws *MissionSnapshot) ToJSONString() (string, error) {
	data, err := ws.ToJSON()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Summary returns a human-readable summary of the snapshot.
// Useful for logging and debugging output.
func (ws *MissionSnapshot) Summary() string {
	return fmt.Sprintf(
		"Workflow Snapshot: %s (Mission: %s, Status: %s)\n"+
			"  Nodes: %d total, %d completed, %d failed, %d pending, %d dynamic\n"+
			"  Executions: %d total\n"+
			"  Decisions: %d total\n"+
			"  Snapshot taken: %s",
		ws.Name,
		ws.MissionID,
		ws.MissionStatus,
		ws.Metadata.TotalNodes,
		ws.Metadata.CompletedNodes,
		ws.Metadata.FailedNodes,
		ws.Metadata.PendingNodes,
		ws.Metadata.DynamicNodes,
		ws.Metadata.TotalExecutions,
		ws.Metadata.TotalDecisions,
		ws.SnapshotTime.Format(time.RFC3339),
	)
}
