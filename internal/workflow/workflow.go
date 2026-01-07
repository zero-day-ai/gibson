package workflow

import (
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// WorkflowStatus represents the current status of a workflow execution.
type WorkflowStatus string

const (
	// WorkflowStatusPending indicates the workflow is ready but not yet started.
	WorkflowStatusPending WorkflowStatus = "pending"

	// WorkflowStatusRunning indicates the workflow is currently executing.
	WorkflowStatusRunning WorkflowStatus = "running"

	// WorkflowStatusCompleted indicates the workflow has completed successfully.
	WorkflowStatusCompleted WorkflowStatus = "completed"

	// WorkflowStatusFailed indicates the workflow execution has failed.
	WorkflowStatusFailed WorkflowStatus = "failed"

	// WorkflowStatusCancelled indicates the workflow was cancelled during execution.
	WorkflowStatusCancelled WorkflowStatus = "cancelled"
)

// String returns the string representation of the workflow status.
func (s WorkflowStatus) String() string {
	return string(s)
}

// IsTerminal returns true if the status represents a terminal state
// (completed, failed, or cancelled).
func (s WorkflowStatus) IsTerminal() bool {
	switch s {
	case WorkflowStatusCompleted, WorkflowStatusFailed, WorkflowStatusCancelled:
		return true
	default:
		return false
	}
}

// Workflow represents a complete workflow definition as a directed acyclic graph (DAG).
// It contains all the nodes, edges, and metadata needed to execute a workflow.
type Workflow struct {
	// ID is the unique identifier for this workflow.
	ID types.ID `json:"id"`

	// Name is a human-readable name for the workflow.
	Name string `json:"name"`

	// Description provides additional context about what this workflow does.
	Description string `json:"description"`

	// Nodes contains all the nodes in the workflow, indexed by node ID.
	Nodes map[string]*WorkflowNode `json:"nodes"`

	// Edges contains all the directed edges connecting nodes in the workflow.
	Edges []WorkflowEdge `json:"edges"`

	// EntryPoints contains the IDs of nodes that can serve as entry points to the workflow.
	// These are nodes with no incoming edges.
	EntryPoints []string `json:"entry_points"`

	// ExitPoints contains the IDs of nodes that can serve as exit points from the workflow.
	// These are nodes with no outgoing edges.
	ExitPoints []string `json:"exit_points"`

	// Metadata contains additional custom metadata for the workflow.
	Metadata map[string]any `json:"metadata,omitempty"`

	// Planning holds configuration for the bounded planning system.
	Planning *PlanningConfig `json:"planning,omitempty"`

	// CreatedAt is the timestamp when the workflow was created.
	CreatedAt time.Time `json:"created_at"`
}

// GetNode retrieves a node by its ID from the workflow.
// Returns nil if the node is not found.
func (w *Workflow) GetNode(id string) *WorkflowNode {
	if w.Nodes == nil {
		return nil
	}
	return w.Nodes[id]
}

// GetEntryNodes returns all nodes that are designated as entry points.
// Returns an empty slice if there are no entry points or if nodes cannot be found.
func (w *Workflow) GetEntryNodes() []*WorkflowNode {
	if w.EntryPoints == nil || w.Nodes == nil {
		return []*WorkflowNode{}
	}

	nodes := make([]*WorkflowNode, 0, len(w.EntryPoints))
	for _, id := range w.EntryPoints {
		if node := w.Nodes[id]; node != nil {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// GetExitNodes returns all nodes that are designated as exit points.
// Returns an empty slice if there are no exit points or if nodes cannot be found.
func (w *Workflow) GetExitNodes() []*WorkflowNode {
	if w.ExitPoints == nil || w.Nodes == nil {
		return []*WorkflowNode{}
	}

	nodes := make([]*WorkflowNode, 0, len(w.ExitPoints))
	for _, id := range w.ExitPoints {
		if node := w.Nodes[id]; node != nil {
			nodes = append(nodes, node)
		}
	}
	return nodes
}
