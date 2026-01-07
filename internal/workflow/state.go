package workflow

import (
	"fmt"
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

	// RetryParams stores custom parameters for retry attempts
	RetryParams map[string]any

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

	// ExecutionOrder defines the custom order for executing pending nodes
	ExecutionOrder []string

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

// GetPendingNodes returns all node IDs that are in pending status.
// Returns an empty slice (not nil) when no pending nodes exist.
// This method is thread-safe and uses a read lock.
func (ws *WorkflowState) GetPendingNodes() []string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	pendingNodes := make([]string, 0)

	for nodeID, nodeState := range ws.NodeStates {
		if nodeState.Status == NodeStatusPending {
			pendingNodes = append(pendingNodes, nodeID)
		}
	}

	return pendingNodes
}

// GetExecutionOrder returns the execution order for pending nodes.
// If a custom order has been set via SetExecutionOrder, it returns that order.
// Otherwise, it returns nodes in dependency order (topological sort).
// This method is thread-safe and uses a read lock.
func (ws *WorkflowState) GetExecutionOrder() []string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	// If custom order is set, return it
	if len(ws.ExecutionOrder) > 0 {
		return ws.ExecutionOrder
	}

	// Otherwise, return pending nodes in dependency order
	return ws.getPendingInDependencyOrder()
}

// SetExecutionOrder sets a custom execution order for pending nodes.
// It validates that:
//   - All nodes in the order exist in the workflow
//   - All nodes in the order are currently pending
//   - The order respects dependency constraints
//
// This method is thread-safe and uses a write lock.
func (ws *WorkflowState) SetExecutionOrder(order []string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Validate all nodes exist
	for _, nodeID := range order {
		if _, exists := ws.Workflow.Nodes[nodeID]; !exists {
			return fmt.Errorf("node %s not found in workflow", nodeID)
		}
	}

	// Validate all nodes are pending
	for _, nodeID := range order {
		nodeState, exists := ws.NodeStates[nodeID]
		if !exists || nodeState.Status != NodeStatusPending {
			return fmt.Errorf("node %s is not pending", nodeID)
		}
	}

	// Validate dependency order
	if err := ws.validateDependencyOrder(order); err != nil {
		return err
	}

	ws.ExecutionOrder = order
	return nil
}

// ReorderRemaining allows reordering of remaining pending nodes during execution.
// It validates the new order respects dependencies and only includes pending nodes.
// This method is thread-safe and uses a write lock.
func (ws *WorkflowState) ReorderRemaining(order []string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Validate all nodes are pending
	for _, nodeID := range order {
		nodeState, exists := ws.NodeStates[nodeID]
		if !exists {
			return fmt.Errorf("node %s not found in workflow", nodeID)
		}
		if nodeState.Status != NodeStatusPending {
			return fmt.Errorf("node %s is not pending", nodeID)
		}
	}

	// Validate dependency order
	if err := ws.validateDependencyOrder(order); err != nil {
		return err
	}

	ws.ExecutionOrder = order
	return nil
}

// SkipNode marks a node as skipped with a reason and removes it from the execution order.
// The node must be in pending status.
// This method is thread-safe and uses a write lock.
func (ws *WorkflowState) SkipNode(nodeID string, reason string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Validate node exists
	nodeState, exists := ws.NodeStates[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found in workflow", nodeID)
	}

	// Validate node is pending
	if nodeState.Status != NodeStatusPending {
		return fmt.Errorf("node %s is not pending", nodeID)
	}

	// Mark node as skipped
	nodeState.Status = NodeStatusSkipped
	now := time.Now()
	nodeState.CompletedAt = &now

	// Store the skip reason as a result
	ws.Results[nodeID] = &NodeResult{
		NodeID:      nodeID,
		Status:      NodeStatusSkipped,
		Metadata:    map[string]any{"skip_reason": reason},
		CompletedAt: now,
	}

	// Remove from execution order if present
	if len(ws.ExecutionOrder) > 0 {
		newOrder := make([]string, 0, len(ws.ExecutionOrder)-1)
		for _, id := range ws.ExecutionOrder {
			if id != nodeID {
				newOrder = append(newOrder, id)
			}
		}
		ws.ExecutionOrder = newOrder
	}

	return nil
}

// ResetForRetry resets a failed node back to pending status for retry.
// It increments the retry count and optionally updates node parameters.
// This method is thread-safe and uses a write lock.
func (ws *WorkflowState) ResetForRetry(nodeID string, newParams map[string]any) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Validate node exists
	nodeState, exists := ws.NodeStates[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found in workflow", nodeID)
	}

	// Validate node is failed
	if nodeState.Status != NodeStatusFailed {
		return fmt.Errorf("node %s is not failed", nodeID)
	}

	// Reset node state
	nodeState.Status = NodeStatusPending
	nodeState.StartedAt = nil
	nodeState.CompletedAt = nil
	nodeState.Error = nil
	nodeState.RetryCount++
	nodeState.RetryParams = newParams

	// Remove result
	delete(ws.Results, nodeID)

	return nil
}

// validateDependencyOrder validates that the given node order respects dependency constraints.
// For each node, all its dependencies must appear earlier in the order.
// This is an internal helper that must be called with a lock held.
func (ws *WorkflowState) validateDependencyOrder(order []string) error {
	// Build position map for quick lookups
	position := make(map[string]int, len(order))
	for i, nodeID := range order {
		position[nodeID] = i
	}

	// Check each node's dependencies
	for i, nodeID := range order {
		node := ws.Workflow.GetNode(nodeID)
		if node == nil {
			continue
		}

		// Check all dependencies appear before this node
		for _, depID := range node.Dependencies {
			depPos, depInOrder := position[depID]
			if !depInOrder {
				// Dependency not in order - might be already completed
				// Check if dependency is completed
				if depState, exists := ws.NodeStates[depID]; exists {
					if depState.Status != NodeStatusCompleted && depState.Status != NodeStatusSkipped {
						return fmt.Errorf("node %s depends on %s which is not in execution order and not completed", nodeID, depID)
					}
				}
			} else if depPos >= i {
				return fmt.Errorf("node %s appears before its dependency %s", nodeID, depID)
			}
		}
	}

	return nil
}

// getPendingInDependencyOrder returns pending nodes in topological order (respecting dependencies).
// This is an internal helper that must be called with a lock held.
func (ws *WorkflowState) getPendingInDependencyOrder() []string {
	// Get all pending nodes
	pending := make(map[string]bool)
	for nodeID, nodeState := range ws.NodeStates {
		if nodeState.Status == NodeStatusPending {
			pending[nodeID] = true
		}
	}

	if len(pending) == 0 {
		return []string{}
	}

	// Topological sort using Kahn's algorithm
	inDegree := make(map[string]int)
	adjacency := make(map[string][]string)

	// Build graph for pending nodes only
	for nodeID := range pending {
		node := ws.Workflow.GetNode(nodeID)
		if node == nil {
			continue
		}

		if _, exists := inDegree[nodeID]; !exists {
			inDegree[nodeID] = 0
		}

		// Count dependencies that are still pending
		for _, depID := range node.Dependencies {
			if pending[depID] {
				inDegree[nodeID]++
				adjacency[depID] = append(adjacency[depID], nodeID)
			}
		}
	}

	// Find nodes with no pending dependencies
	queue := make([]string, 0)
	for nodeID := range pending {
		if inDegree[nodeID] == 0 {
			queue = append(queue, nodeID)
		}
	}

	// Process queue
	result := make([]string, 0, len(pending))
	for len(queue) > 0 {
		// Pop from queue
		nodeID := queue[0]
		queue = queue[1:]
		result = append(result, nodeID)

		// Update neighbors
		for _, neighbor := range adjacency[nodeID] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	return result
}
