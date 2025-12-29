package workflow

import (
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// NodeError represents an error that occurred during node execution
type NodeError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	Cause   error          `json:"-"`
}

// Error implements the error interface for NodeError
func (e *NodeError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NodeResult represents the execution result of a single workflow node
type NodeResult struct {
	NodeID      string          `json:"node_id"`
	Status      NodeStatus      `json:"status"`
	Output      map[string]any  `json:"output,omitempty"`
	Error       *NodeError      `json:"error,omitempty"`
	Findings    []agent.Finding `json:"findings,omitempty"`
	Duration    time.Duration   `json:"duration"`
	RetryCount  int             `json:"retry_count"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt time.Time       `json:"completed_at"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
}

// WorkflowErrorCode represents specific error types that can occur during workflow execution
type WorkflowErrorCode string

const (
	WorkflowErrorCycleDetected       WorkflowErrorCode = "cycle_detected"
	WorkflowErrorMissingDependency   WorkflowErrorCode = "missing_dependency"
	WorkflowErrorDeadlock            WorkflowErrorCode = "deadlock"
	WorkflowErrorNodeExecutionFailed WorkflowErrorCode = "node_execution_failed"
	WorkflowErrorExpressionInvalid   WorkflowErrorCode = "expression_invalid"
	WorkflowErrorWorkflowCancelled   WorkflowErrorCode = "workflow_cancelled"
	WorkflowErrorInvalidWorkflow     WorkflowErrorCode = "invalid_workflow"
	WorkflowErrorNodeTimeout         WorkflowErrorCode = "node_timeout"
)

// WorkflowError represents an error that occurred during workflow execution
type WorkflowError struct {
	Code    WorkflowErrorCode `json:"code"`
	Message string            `json:"message"`
	NodeID  string            `json:"node_id,omitempty"`
	Cause   error             `json:"-"`
}

// Error implements the error interface for WorkflowError
func (e *WorkflowError) Error() string {
	if e.NodeID != "" {
		if e.Cause != nil {
			return fmt.Sprintf("%s [node: %s]: %s (caused by: %v)", e.Code, e.NodeID, e.Message, e.Cause)
		}
		return fmt.Sprintf("%s [node: %s]: %s", e.Code, e.NodeID, e.Message)
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap implements the errors.Unwrap interface for WorkflowError
func (e *WorkflowError) Unwrap() error {
	return e.Cause
}

// WorkflowResult represents the complete execution result of a workflow
type WorkflowResult struct {
	WorkflowID    types.ID               `json:"workflow_id"`
	Status        WorkflowStatus         `json:"status"`
	NodeResults   map[string]*NodeResult `json:"node_results"`
	Findings      []agent.Finding        `json:"findings,omitempty"`
	TotalDuration time.Duration          `json:"total_duration"`
	NodesExecuted int                    `json:"nodes_executed"`
	NodesFailed   int                    `json:"nodes_failed"`
	NodesSkipped  int                    `json:"nodes_skipped"`
	Error         *WorkflowError         `json:"error,omitempty"`
}
