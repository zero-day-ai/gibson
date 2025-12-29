package mission

import (
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// MissionStatus represents the lifecycle state of a mission.
type MissionStatus string

const (
	// MissionStatusPending indicates the mission is created but not yet started.
	MissionStatusPending MissionStatus = "pending"

	// MissionStatusRunning indicates the mission is currently executing.
	MissionStatusRunning MissionStatus = "running"

	// MissionStatusPaused indicates the mission is temporarily suspended.
	MissionStatusPaused MissionStatus = "paused"

	// MissionStatusCompleted indicates the mission has completed successfully.
	MissionStatusCompleted MissionStatus = "completed"

	// MissionStatusFailed indicates the mission execution has failed.
	MissionStatusFailed MissionStatus = "failed"

	// MissionStatusCancelled indicates the mission was cancelled during execution.
	MissionStatusCancelled MissionStatus = "cancelled"
)

// String returns the string representation of the mission status.
func (s MissionStatus) String() string {
	return string(s)
}

// IsTerminal returns true if the status represents a terminal state
// (completed, failed, or cancelled). Terminal states cannot transition to other states.
func (s MissionStatus) IsTerminal() bool {
	switch s {
	case MissionStatusCompleted, MissionStatusFailed, MissionStatusCancelled:
		return true
	default:
		return false
	}
}

// CanTransitionTo validates whether a state transition is allowed.
// Returns true if the transition from the current status to the target status is valid.
func (s MissionStatus) CanTransitionTo(target MissionStatus) bool {
	// Terminal states cannot transition
	if s.IsTerminal() {
		return false
	}

	// Valid transitions based on mission lifecycle
	switch s {
	case MissionStatusPending:
		return target == MissionStatusRunning || target == MissionStatusCancelled
	case MissionStatusRunning:
		return target == MissionStatusPaused ||
		       target == MissionStatusCompleted ||
		       target == MissionStatusFailed ||
		       target == MissionStatusCancelled
	case MissionStatusPaused:
		return target == MissionStatusRunning ||
		       target == MissionStatusFailed ||
		       target == MissionStatusCancelled
	default:
		return false
	}
}

// Mission represents a complete security testing mission.
// A mission coordinates the execution of a workflow against a target,
// aggregating findings and enforcing constraints throughout execution.
type Mission struct {
	// ID is the unique identifier for this mission.
	ID types.ID `json:"id"`

	// Name is a human-readable name for the mission.
	Name string `json:"name"`

	// Description provides additional context about what this mission does.
	Description string `json:"description"`

	// Status represents the current lifecycle state of the mission.
	Status MissionStatus `json:"status"`

	// TargetID references the target being tested.
	TargetID types.ID `json:"target_id"`

	// WorkflowID references the workflow being executed.
	WorkflowID types.ID `json:"workflow_id"`

	// Constraints define execution boundaries for the mission.
	Constraints *MissionConstraints `json:"constraints,omitempty"`

	// Metrics tracks mission execution statistics.
	Metrics *MissionMetrics `json:"metrics,omitempty"`

	// Checkpoint stores state for resume capability.
	Checkpoint *MissionCheckpoint `json:"checkpoint,omitempty"`

	// Error contains error message if mission failed.
	Error string `json:"error,omitempty"`

	// CreatedAt is the timestamp when the mission was created.
	CreatedAt time.Time `json:"created_at"`

	// StartedAt is the timestamp when the mission started execution.
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is the timestamp when the mission finished execution.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// UpdatedAt is the timestamp of the last update to this mission.
	UpdatedAt time.Time `json:"updated_at"`
}

// MissionMetrics tracks mission execution statistics.
// These metrics are updated throughout execution to provide real-time progress information.
type MissionMetrics struct {
	// TotalNodes is the total number of nodes in the workflow.
	TotalNodes int `json:"total_nodes"`

	// CompletedNodes is the number of nodes that have completed execution.
	CompletedNodes int `json:"completed_nodes"`

	// FailedNodes is the number of nodes that failed during execution.
	FailedNodes int `json:"failed_nodes"`

	// TotalFindings is the total number of findings discovered.
	TotalFindings int `json:"total_findings"`

	// FindingsBySeverity is a map of severity levels to finding counts.
	FindingsBySeverity map[string]int `json:"findings_by_severity"`

	// TotalTokens is the total number of LLM tokens consumed.
	TotalTokens int64 `json:"total_tokens"`

	// TotalCost is the total cost in dollars for LLM usage.
	TotalCost float64 `json:"total_cost"`

	// Duration is the total execution time.
	Duration time.Duration `json:"duration"`

	// StartedAt is when execution started.
	StartedAt time.Time `json:"started_at"`

	// LastUpdateAt is when metrics were last updated.
	LastUpdateAt time.Time `json:"last_update_at"`
}

// MissionCheckpoint stores state for resume capability.
// Checkpoints are created periodically during execution to enable
// resuming a mission after interruption.
type MissionCheckpoint struct {
	// WorkflowState contains the DAG execution state.
	WorkflowState map[string]any `json:"workflow_state"`

	// CompletedNodes lists nodes that have completed execution.
	CompletedNodes []string `json:"completed_nodes"`

	// PendingNodes lists nodes that are pending execution.
	PendingNodes []string `json:"pending_nodes"`

	// NodeResults stores results from completed nodes.
	NodeResults map[string]any `json:"node_results"`

	// LastNodeID is the ID of the last node that was executing.
	LastNodeID string `json:"last_node_id"`

	// CheckpointedAt is when this checkpoint was created.
	CheckpointedAt time.Time `json:"checkpointed_at"`
}

// MissionProgress provides real-time progress information.
// This is used for monitoring mission execution through CLI, TUI, or API.
type MissionProgress struct {
	// MissionID is the unique identifier for the mission.
	MissionID types.ID `json:"mission_id"`

	// Status is the current mission status.
	Status MissionStatus `json:"status"`

	// PercentComplete is the completion percentage (0-100).
	PercentComplete float64 `json:"percent_complete"`

	// CompletedNodes is the number of completed workflow nodes.
	CompletedNodes int `json:"completed_nodes"`

	// TotalNodes is the total number of workflow nodes.
	TotalNodes int `json:"total_nodes"`

	// RunningNodes lists nodes currently executing.
	RunningNodes []string `json:"running_nodes"`

	// PendingNodes lists nodes pending execution.
	PendingNodes []string `json:"pending_nodes"`

	// FindingsCount is the total number of findings discovered.
	FindingsCount int `json:"findings_count"`

	// EstimatedRemaining is the estimated time remaining (if calculable).
	EstimatedRemaining *time.Duration `json:"estimated_remaining,omitempty"`
}

// MissionResult contains the final mission outcome.
// This is returned after mission execution completes (successfully or not).
type MissionResult struct {
	// MissionID is the unique identifier for the mission.
	MissionID types.ID `json:"mission_id"`

	// Status is the final mission status.
	Status MissionStatus `json:"status"`

	// Metrics contains execution statistics.
	Metrics *MissionMetrics `json:"metrics"`

	// FindingIDs contains IDs of all findings discovered.
	// Full findings are stored in the finding store.
	FindingIDs []types.ID `json:"finding_ids"`

	// WorkflowResult contains the workflow execution result.
	WorkflowResult map[string]any `json:"workflow_result"`

	// Error contains error message if mission failed.
	Error string `json:"error,omitempty"`

	// CompletedAt is when the mission finished execution.
	CompletedAt time.Time `json:"completed_at"`
}

// Validate checks if the mission has all required fields.
func (m *Mission) Validate() error {
	if m.ID.IsZero() {
		return fmt.Errorf("mission ID is required")
	}
	if m.Name == "" {
		return fmt.Errorf("mission name is required")
	}
	if m.TargetID.IsZero() {
		return fmt.Errorf("target ID is required")
	}
	if m.WorkflowID.IsZero() {
		return fmt.Errorf("workflow ID is required")
	}
	if m.Status == "" {
		return fmt.Errorf("mission status is required")
	}
	return nil
}

// CalculateProgress calculates the current progress percentage.
// Returns 0 if metrics are not available or total nodes is 0.
func (m *Mission) CalculateProgress() float64 {
	if m.Metrics == nil || m.Metrics.TotalNodes == 0 {
		return 0.0
	}
	return (float64(m.Metrics.CompletedNodes) / float64(m.Metrics.TotalNodes)) * 100.0
}

// GetProgress returns a MissionProgress snapshot.
func (m *Mission) GetProgress() *MissionProgress {
	progress := &MissionProgress{
		MissionID:       m.ID,
		Status:          m.Status,
		PercentComplete: m.CalculateProgress(),
		FindingsCount:   0,
	}

	if m.Metrics != nil {
		progress.CompletedNodes = m.Metrics.CompletedNodes
		progress.TotalNodes = m.Metrics.TotalNodes
		progress.FindingsCount = m.Metrics.TotalFindings
	}

	if m.Checkpoint != nil {
		progress.PendingNodes = m.Checkpoint.PendingNodes
		// Running nodes would be derived from workflow state
		progress.RunningNodes = []string{}
	}

	return progress
}

// GetDuration returns the mission execution duration.
// Returns 0 if the mission hasn't started.
func (m *Mission) GetDuration() time.Duration {
	if m.StartedAt == nil {
		return 0
	}

	endTime := time.Now()
	if m.CompletedAt != nil {
		endTime = *m.CompletedAt
	}

	return endTime.Sub(*m.StartedAt)
}
