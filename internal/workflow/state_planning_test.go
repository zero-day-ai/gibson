package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestGetPendingNodes tests the GetPendingNodes method
func TestGetPendingNodes(t *testing.T) {
	tests := []struct {
		name            string
		setupState      func(*WorkflowState)
		expectedCount   int
		expectedNodeIDs []string
	}{
		{
			name: "all nodes pending",
			setupState: func(ws *WorkflowState) {
				// Default state - all pending
			},
			expectedCount:   3,
			expectedNodeIDs: []string{"node1", "node2", "node3"},
		},
		{
			name: "mixed statuses",
			setupState: func(ws *WorkflowState) {
				ws.MarkNodeStarted("node1")
				ws.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1"})
				ws.MarkNodeFailed("node2", assert.AnError)
			},
			expectedCount:   1,
			expectedNodeIDs: []string{"node3"},
		},
		{
			name: "no pending nodes",
			setupState: func(ws *WorkflowState) {
				ws.MarkNodeStarted("node1")
				ws.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1"})
				ws.MarkNodeStarted("node2")
				ws.MarkNodeCompleted("node2", &NodeResult{NodeID: "node2"})
				ws.MarkNodeStarted("node3")
				ws.MarkNodeCompleted("node3", &NodeResult{NodeID: "node3"})
			},
			expectedCount:   0,
			expectedNodeIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := createTestWorkflow()
			state := NewWorkflowState(workflow)
			tt.setupState(state)

			pending := state.GetPendingNodes()
			assert.Equal(t, tt.expectedCount, len(pending))

			// Check that all expected nodes are present (order doesn't matter)
			if tt.expectedCount > 0 {
				for _, expectedID := range tt.expectedNodeIDs {
					assert.Contains(t, pending, expectedID)
				}
			}
		})
	}
}

// TestGetExecutionOrder tests the GetExecutionOrder method
func TestGetExecutionOrder(t *testing.T) {
	tests := []struct {
		name          string
		setupState    func(*WorkflowState)
		expectedOrder []string
	}{
		{
			name: "no custom order set - returns dependency order",
			setupState: func(ws *WorkflowState) {
				// No custom order
			},
			expectedOrder: []string{"node1", "node2", "node3"},
		},
		{
			name: "custom order set",
			setupState: func(ws *WorkflowState) {
				err := ws.SetExecutionOrder([]string{"node1", "node2", "node3"})
				require.NoError(t, err)
			},
			expectedOrder: []string{"node1", "node2", "node3"},
		},
		{
			name: "custom order with some nodes completed",
			setupState: func(ws *WorkflowState) {
				ws.MarkNodeStarted("node1")
				ws.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1"})
				err := ws.SetExecutionOrder([]string{"node2", "node3"})
				require.NoError(t, err)
			},
			expectedOrder: []string{"node2", "node3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := createTestWorkflow()
			state := NewWorkflowState(workflow)
			tt.setupState(state)

			order := state.GetExecutionOrder()
			assert.Equal(t, tt.expectedOrder, order)
		})
	}
}

// TestSetExecutionOrder tests the SetExecutionOrder method
func TestSetExecutionOrder(t *testing.T) {
	tests := []struct {
		name        string
		setupState  func(*WorkflowState)
		order       []string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid order",
			setupState: func(ws *WorkflowState) {
				// All nodes pending
			},
			order:       []string{"node1", "node2", "node3"},
			expectError: false,
		},
		{
			name: "valid order with dependencies respected",
			setupState: func(ws *WorkflowState) {
				// node2 depends on node1, node3 depends on node2
			},
			order:       []string{"node1", "node2", "node3"},
			expectError: false,
		},
		{
			name: "invalid order - node before dependency",
			setupState: func(ws *WorkflowState) {
				// node2 depends on node1
			},
			order:       []string{"node2", "node1", "node3"},
			expectError: true,
			errorMsg:    "appears before its dependency",
		},
		{
			name: "invalid node ID",
			setupState: func(ws *WorkflowState) {
				// All nodes pending
			},
			order:       []string{"node1", "nonexistent", "node3"},
			expectError: true,
			errorMsg:    "not found in workflow",
		},
		{
			name: "node not pending",
			setupState: func(ws *WorkflowState) {
				ws.MarkNodeStarted("node1")
				ws.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1"})
			},
			order:       []string{"node1", "node2", "node3"},
			expectError: true,
			errorMsg:    "is not pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := createTestWorkflow()
			state := NewWorkflowState(workflow)
			tt.setupState(state)

			err := state.SetExecutionOrder(tt.order)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.order, state.ExecutionOrder)
			}
		})
	}
}

// TestReorderRemaining tests the ReorderRemaining method
func TestReorderRemaining(t *testing.T) {
	t.Run("reorder remaining nodes", func(t *testing.T) {
		workflow := createTestWorkflow()
		state := NewWorkflowState(workflow)

		// Complete node1
		state.MarkNodeStarted("node1")
		state.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1"})

		// Reorder remaining nodes
		err := state.ReorderRemaining([]string{"node3", "node2"})
		require.Error(t, err) // Should fail because node3 depends on node2

		// Valid reorder
		err = state.ReorderRemaining([]string{"node2", "node3"})
		require.NoError(t, err)
		assert.Equal(t, []string{"node2", "node3"}, state.ExecutionOrder)
	})
}

// TestSkipNode tests the SkipNode method
func TestSkipNode(t *testing.T) {
	tests := []struct {
		name        string
		setupState  func(*WorkflowState)
		nodeID      string
		reason      string
		expectError bool
		errorMsg    string
	}{
		{
			name: "skip pending node",
			setupState: func(ws *WorkflowState) {
				// All nodes pending
			},
			nodeID:      "node2",
			reason:      "test skip",
			expectError: false,
		},
		{
			name: "skip non-existent node",
			setupState: func(ws *WorkflowState) {
				// All nodes pending
			},
			nodeID:      "nonexistent",
			reason:      "test",
			expectError: true,
			errorMsg:    "not found in workflow",
		},
		{
			name: "skip non-pending node",
			setupState: func(ws *WorkflowState) {
				ws.MarkNodeStarted("node1")
				ws.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1"})
			},
			nodeID:      "node1",
			reason:      "test",
			expectError: true,
			errorMsg:    "is not pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := createTestWorkflow()
			state := NewWorkflowState(workflow)
			tt.setupState(state)

			err := state.SkipNode(tt.nodeID, tt.reason)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)

				// Check node is marked as skipped
				nodeState := state.NodeStates[tt.nodeID]
				assert.Equal(t, NodeStatusSkipped, nodeState.Status)
				assert.NotNil(t, nodeState.CompletedAt)

				// Check result is stored with reason
				result := state.GetResult(tt.nodeID)
				require.NotNil(t, result)
				assert.Equal(t, NodeStatusSkipped, result.Status)
				assert.Equal(t, tt.reason, result.Metadata["skip_reason"])
			}
		})
	}
}

// TestSkipNodeRemovesFromExecutionOrder tests that skipping a node removes it from execution order
func TestSkipNodeRemovesFromExecutionOrder(t *testing.T) {
	workflow := createTestWorkflow()
	state := NewWorkflowState(workflow)

	// Set execution order
	err := state.SetExecutionOrder([]string{"node1", "node2", "node3"})
	require.NoError(t, err)

	// Skip node2
	err = state.SkipNode("node2", "testing")
	require.NoError(t, err)

	// Check that node2 is removed from execution order
	order := state.GetExecutionOrder()
	assert.Equal(t, []string{"node1", "node3"}, order)
	assert.NotContains(t, order, "node2")
}

// TestResetForRetry tests the ResetForRetry method
func TestResetForRetry(t *testing.T) {
	tests := []struct {
		name        string
		setupState  func(*WorkflowState)
		nodeID      string
		newParams   map[string]any
		expectError bool
		errorMsg    string
	}{
		{
			name: "reset failed node",
			setupState: func(ws *WorkflowState) {
				ws.MarkNodeStarted("node1")
				ws.MarkNodeFailed("node1", assert.AnError)
			},
			nodeID:      "node1",
			newParams:   map[string]any{"retry_timeout": 60},
			expectError: false,
		},
		{
			name: "reset non-existent node",
			setupState: func(ws *WorkflowState) {
				// All nodes pending
			},
			nodeID:      "nonexistent",
			newParams:   map[string]any{},
			expectError: true,
			errorMsg:    "not found in workflow",
		},
		{
			name: "reset non-failed node",
			setupState: func(ws *WorkflowState) {
				// node1 is pending
			},
			nodeID:      "node1",
			newParams:   map[string]any{},
			expectError: true,
			errorMsg:    "is not failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := createTestWorkflow()
			state := NewWorkflowState(workflow)
			tt.setupState(state)

			err := state.ResetForRetry(tt.nodeID, tt.newParams)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)

				// Check node is reset to pending
				nodeState := state.NodeStates[tt.nodeID]
				assert.Equal(t, NodeStatusPending, nodeState.Status)
				assert.Nil(t, nodeState.StartedAt)
				assert.Nil(t, nodeState.CompletedAt)
				assert.Nil(t, nodeState.Error)
				assert.Equal(t, 1, nodeState.RetryCount)
				assert.Equal(t, tt.newParams, nodeState.RetryParams)

				// Check result is removed
				result := state.GetResult(tt.nodeID)
				assert.Nil(t, result)
			}
		})
	}
}

// TestResetForRetryIncrementsRetryCount tests that retry count increments correctly
func TestResetForRetryIncrementsRetryCount(t *testing.T) {
	workflow := createTestWorkflow()
	state := NewWorkflowState(workflow)

	// Fail and reset multiple times
	for i := 0; i < 3; i++ {
		state.MarkNodeStarted("node1")
		state.MarkNodeFailed("node1", assert.AnError)

		err := state.ResetForRetry("node1", map[string]any{"attempt": i + 1})
		require.NoError(t, err)

		nodeState := state.NodeStates["node1"]
		assert.Equal(t, i+1, nodeState.RetryCount)
	}
}

// TestValidateDependencyOrder tests the validateDependencyOrder helper
func TestValidateDependencyOrder(t *testing.T) {
	workflow := createTestWorkflow()
	state := NewWorkflowState(workflow)

	tests := []struct {
		name        string
		order       []string
		expectError bool
	}{
		{
			name:        "valid order",
			order:       []string{"node1", "node2", "node3"},
			expectError: false,
		},
		{
			name:        "invalid order - node2 before node1",
			order:       []string{"node2", "node1", "node3"},
			expectError: true,
		},
		{
			name:        "invalid order - node3 before node2",
			order:       []string{"node1", "node3", "node2"},
			expectError: true,
		},
		{
			name:        "single node",
			order:       []string{"node1"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state.mu.Lock()
			err := state.validateDependencyOrder(tt.order)
			state.mu.Unlock()

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestGetPendingInDependencyOrder tests the getPendingInDependencyOrder helper
func TestGetPendingInDependencyOrder(t *testing.T) {
	t.Run("all nodes pending", func(t *testing.T) {
		workflow := createTestWorkflow()
		state := NewWorkflowState(workflow)

		state.mu.RLock()
		order := state.getPendingInDependencyOrder()
		state.mu.RUnlock()

		// Should return nodes in dependency order
		assert.Len(t, order, 3)

		// node1 should come before node2
		node1Pos := -1
		node2Pos := -1
		for i, nodeID := range order {
			if nodeID == "node1" {
				node1Pos = i
			}
			if nodeID == "node2" {
				node2Pos = i
			}
		}
		assert.Less(t, node1Pos, node2Pos, "node1 should come before node2")

		// node2 should come before node3
		node3Pos := -1
		for i, nodeID := range order {
			if nodeID == "node3" {
				node3Pos = i
			}
		}
		assert.Less(t, node2Pos, node3Pos, "node2 should come before node3")
	})

	t.Run("some nodes completed", func(t *testing.T) {
		workflow := createTestWorkflow()
		state := NewWorkflowState(workflow)

		// Complete node1
		state.MarkNodeStarted("node1")
		state.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1"})

		state.mu.RLock()
		order := state.getPendingInDependencyOrder()
		state.mu.RUnlock()

		// Should only return pending nodes
		assert.Len(t, order, 2)
		assert.Contains(t, order, "node2")
		assert.Contains(t, order, "node3")
		assert.NotContains(t, order, "node1")
	})
}

// TestConcurrentAccessToState tests thread safety of state modification methods
func TestConcurrentAccessToState(t *testing.T) {
	workflow := createTestWorkflow()
	state := NewWorkflowState(workflow)

	// Add more nodes for better concurrency testing
	for i := 4; i <= 10; i++ {
		nodeID := types.NewID().String()
		workflow.Nodes[nodeID] = &WorkflowNode{
			ID:           nodeID,
			Type:         NodeTypeAgent,
			Dependencies: []string{},
		}
		state.NodeStates[nodeID] = &NodeState{
			NodeID:     nodeID,
			Status:     NodeStatusPending,
			RetryCount: 0,
		}
	}

	// Run concurrent operations
	done := make(chan bool)

	// Reader goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = state.GetPendingNodes()
				_ = state.GetExecutionOrder()
				_ = state.GetNodeStatus("node1")
			}
			done <- true
		}()
	}

	// Writer goroutines
	go func() {
		for i := 0; i < 50; i++ {
			_ = state.SetExecutionOrder([]string{"node1", "node2", "node3"})
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 11; i++ {
		<-done
	}
}

// Helper function to create a test workflow with dependencies
func createTestWorkflow() *Workflow {
	workflow := &Workflow{
		ID:   types.NewID(),
		Name: "test-workflow",
		Nodes: map[string]*WorkflowNode{
			"node1": {
				ID:           "node1",
				Type:         NodeTypeAgent,
				Dependencies: []string{},
			},
			"node2": {
				ID:           "node2",
				Type:         NodeTypeAgent,
				Dependencies: []string{"node1"},
			},
			"node3": {
				ID:           "node3",
				Type:         NodeTypeAgent,
				Dependencies: []string{"node2"},
			},
		},
	}
	return workflow
}
