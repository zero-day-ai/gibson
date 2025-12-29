package workflow

import (
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// NodeState tracks the execution state of a single workflow node.
type NodeState struct {
	// NodeID is the unique identifier for the node
	NodeID string

	// Status is the current execution status of the node
	Status NodeStatus

	// StartedAt is the timestamp when the node execution began
	StartedAt *time.Time

	// CompletedAt is the timestamp when the node execution completed
	CompletedAt *time.Time

	// RetryCount tracks the number of retry attempts for this node
	RetryCount int

	// Error stores any error that occurred during node execution
	Error error
}

// WorkflowState manages the runtime execution state of a workflow.
// It tracks the status of all nodes, their results, and provides thread-safe
// access to state information during workflow execution.
type WorkflowState struct {
	// WorkflowID is the unique identifier for this workflow execution
	WorkflowID types.ID

	// Workflow is a reference to the workflow definition being executed
	Workflow *Workflow

	// Status is the current execution status of the workflow
	Status WorkflowStatus

	// NodeStates tracks the execution state of each node, indexed by node ID
	NodeStates map[string]*NodeState

	// Results stores the execution results for completed nodes, indexed by node ID
	Results map[string]*NodeResult

	// StartedAt is the timestamp when workflow execution began
	StartedAt time.Time

	// CompletedAt is the timestamp when workflow execution completed (nil if still running)
	CompletedAt *time.Time

	// mu provides thread-safe access to the workflow state
	mu sync.RWMutex
}

// NewWorkflowState creates a new WorkflowState instance initialized with all nodes
// in pending status. This prepares the state for workflow execution.
func NewWorkflowState(workflow *Workflow) *WorkflowState {
	nodeStates := make(map[string]*NodeState, len(workflow.Nodes))

	// Initialize all nodes to pending status
	for nodeID := range workflow.Nodes {
		nodeStates[nodeID] = &NodeState{
			NodeID:     nodeID,
			Status:     NodeStatusPending,
			RetryCount: 0,
		}
	}

	return &WorkflowState{
		WorkflowID: workflow.ID,
		Workflow:   workflow,
		Status:     WorkflowStatusPending,
		NodeStates: nodeStates,
		Results:    make(map[string]*NodeResult),
		StartedAt:  time.Now(),
	}
}

// GetReadyNodes returns all nodes that are ready to be executed.
// A node is ready if:
//   - Its status is pending
//   - All of its dependencies have completed successfully
//
// This method is thread-safe and uses a read lock.
func (ws *WorkflowState) GetReadyNodes() []*WorkflowNode {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	var readyNodes []*WorkflowNode

	for nodeID, nodeState := range ws.NodeStates {
		// Skip if node is not pending
		if nodeState.Status != NodeStatusPending {
			continue
		}

		// Get the node definition from the workflow
		node := ws.Workflow.GetNode(nodeID)
		if node == nil {
			continue
		}

		// Check if all dependencies are completed
		if ws.areDependenciesCompleted(node.Dependencies) {
			readyNodes = append(readyNodes, node)
		}
	}

	return readyNodes
}

// areDependenciesCompleted checks if all dependencies for a node have completed successfully.
// This is an internal helper method that must be called with a lock held.
func (ws *WorkflowState) areDependenciesCompleted(dependencies []string) bool {
	if len(dependencies) == 0 {
		return true
	}

	for _, depID := range dependencies {
		depState, exists := ws.NodeStates[depID]
		if !exists {
			return false
		}

		// Dependency must be completed successfully
		if depState.Status != NodeStatusCompleted {
			return false
		}
	}

	return true
}

// MarkNodeStarted marks a node as started and sets the StartedAt timestamp.
// This method is thread-safe and uses a write lock.
func (ws *WorkflowState) MarkNodeStarted(nodeID string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if nodeState, exists := ws.NodeStates[nodeID]; exists {
		nodeState.Status = NodeStatusRunning
		now := time.Now()
		nodeState.StartedAt = &now
	}
}

// MarkNodeCompleted marks a node as successfully completed, stores its result,
// and sets the CompletedAt timestamp.
// This method is thread-safe and uses a write lock.
func (ws *WorkflowState) MarkNodeCompleted(nodeID string, result *NodeResult) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if nodeState, exists := ws.NodeStates[nodeID]; exists {
		nodeState.Status = NodeStatusCompleted
		now := time.Now()
		nodeState.CompletedAt = &now

		// Store the result
		if result != nil {
			ws.Results[nodeID] = result
		}
	}
}

// MarkNodeFailed marks a node as failed and stores the error that caused the failure.
// This method is thread-safe and uses a write lock.
func (ws *WorkflowState) MarkNodeFailed(nodeID string, err error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if nodeState, exists := ws.NodeStates[nodeID]; exists {
		nodeState.Status = NodeStatusFailed
		nodeState.Error = err
		now := time.Now()
		nodeState.CompletedAt = &now
	}
}

// MarkNodeSkipped marks a node as skipped with a reason.
// This method is thread-safe and uses a write lock.
func (ws *WorkflowState) MarkNodeSkipped(nodeID string, reason string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if nodeState, exists := ws.NodeStates[nodeID]; exists {
		nodeState.Status = NodeStatusSkipped
		now := time.Now()
		nodeState.CompletedAt = &now

		// Store the skip reason as a result for tracking purposes
		ws.Results[nodeID] = &NodeResult{
			NodeID:      nodeID,
			Status:      NodeStatusSkipped,
			Metadata:    map[string]any{"skip_reason": reason},
			CompletedAt: now,
		}
	}
}

// IsComplete returns true if all nodes in the workflow have reached a terminal status
// (completed, failed, or skipped).
// This method is thread-safe and uses a read lock.
func (ws *WorkflowState) IsComplete() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	for _, nodeState := range ws.NodeStates {
		// Check if node has reached a terminal status
		switch nodeState.Status {
		case NodeStatusCompleted, NodeStatusFailed, NodeStatusSkipped, NodeStatusCancelled:
			// Terminal status - continue checking other nodes
			continue
		default:
			// Non-terminal status found - workflow is not complete
			return false
		}
	}

	return true
}

// GetResult returns the execution result for a specific node.
// Returns nil if no result exists for the node.
// This method is thread-safe and uses a read lock.
func (ws *WorkflowState) GetResult(nodeID string) *NodeResult {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return ws.Results[nodeID]
}

// GetNodeStatus returns the current status of a specific node.
// Returns an empty NodeStatus if the node doesn't exist in the state.
// This method is thread-safe and uses a read lock.
func (ws *WorkflowState) GetNodeStatus(nodeID string) NodeStatus {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	if nodeState, exists := ws.NodeStates[nodeID]; exists {
		return nodeState.Status
	}

	return ""
}
