package workflow

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestNewWorkflowState verifies that NewWorkflowState properly initializes all nodes to pending status
func TestNewWorkflowState(t *testing.T) {
	workflow := &Workflow{
		ID:   types.NewID(),
		Name: "test-workflow",
		Nodes: map[string]*WorkflowNode{
			"node1": {ID: "node1", Type: NodeTypeAgent, Name: "Node 1"},
			"node2": {ID: "node2", Type: NodeTypeAgent, Name: "Node 2"},
			"node3": {ID: "node3", Type: NodeTypeAgent, Name: "Node 3"},
		},
	}

	state := NewWorkflowState(workflow)

	require.NotNil(t, state)
	assert.Equal(t, workflow.ID, state.WorkflowID)
	assert.Equal(t, workflow, state.Workflow)
	assert.Equal(t, WorkflowStatusPending, state.Status)
	assert.Len(t, state.NodeStates, 3)
	assert.Empty(t, state.Results)
	assert.NotZero(t, state.StartedAt)
	assert.Nil(t, state.CompletedAt)

	// Verify all nodes are initialized to pending
	for nodeID, nodeState := range state.NodeStates {
		assert.Equal(t, nodeID, nodeState.NodeID)
		assert.Equal(t, NodeStatusPending, nodeState.Status)
		assert.Nil(t, nodeState.StartedAt)
		assert.Nil(t, nodeState.CompletedAt)
		assert.Equal(t, 0, nodeState.RetryCount)
		assert.Nil(t, nodeState.Error)
	}
}

// TestGetReadyNodes_NodesWithoutDependencies verifies that nodes without dependencies are immediately ready
func TestGetReadyNodes_NodesWithoutDependencies(t *testing.T) {
	workflow := &Workflow{
		ID:   types.NewID(),
		Name: "test-workflow",
		Nodes: map[string]*WorkflowNode{
			"node1": {ID: "node1", Type: NodeTypeAgent, Dependencies: nil},
			"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{}},
			"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
		},
	}

	state := NewWorkflowState(workflow)
	readyNodes := state.GetReadyNodes()

	assert.Len(t, readyNodes, 2)

	// Verify node1 and node2 are ready (no dependencies)
	readyIDs := make(map[string]bool)
	for _, node := range readyNodes {
		readyIDs[node.ID] = true
	}
	assert.True(t, readyIDs["node1"])
	assert.True(t, readyIDs["node2"])
	assert.False(t, readyIDs["node3"])
}

// TestGetReadyNodes_WithCompletedDependencies verifies nodes become ready when dependencies complete
func TestGetReadyNodes_WithCompletedDependencies(t *testing.T) {
	workflow := &Workflow{
		ID:   types.NewID(),
		Name: "test-workflow",
		Nodes: map[string]*WorkflowNode{
			"node1": {ID: "node1", Type: NodeTypeAgent, Dependencies: nil},
			"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
			"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node1", "node2"}},
		},
	}

	state := NewWorkflowState(workflow)

	// Initially only node1 is ready
	readyNodes := state.GetReadyNodes()
	assert.Len(t, readyNodes, 1)
	assert.Equal(t, "node1", readyNodes[0].ID)

	// Mark node1 as completed
	state.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1", Status: NodeStatusCompleted})

	// Now node2 should be ready
	readyNodes = state.GetReadyNodes()
	assert.Len(t, readyNodes, 1)
	assert.Equal(t, "node2", readyNodes[0].ID)

	// Mark node2 as completed
	state.MarkNodeCompleted("node2", &NodeResult{NodeID: "node2", Status: NodeStatusCompleted})

	// Now node3 should be ready
	readyNodes = state.GetReadyNodes()
	assert.Len(t, readyNodes, 1)
	assert.Equal(t, "node3", readyNodes[0].ID)
}

// TestGetReadyNodes_FailedDependency verifies nodes don't become ready if dependencies failed
func TestGetReadyNodes_FailedDependency(t *testing.T) {
	workflow := &Workflow{
		ID:   types.NewID(),
		Name: "test-workflow",
		Nodes: map[string]*WorkflowNode{
			"node1": {ID: "node1", Type: NodeTypeAgent, Dependencies: nil},
			"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
		},
	}

	state := NewWorkflowState(workflow)

	// Mark node1 as failed
	state.MarkNodeFailed("node1", errors.New("test error"))

	// node2 should not be ready because its dependency failed
	readyNodes := state.GetReadyNodes()
	assert.Empty(t, readyNodes)
}

// TestMarkNodeStarted verifies MarkNodeStarted updates status and timestamp
func TestMarkNodeStarted(t *testing.T) {
	workflow := &Workflow{
		ID:    types.NewID(),
		Nodes: map[string]*WorkflowNode{"node1": {ID: "node1"}},
	}

	state := NewWorkflowState(workflow)
	before := time.Now()

	state.MarkNodeStarted("node1")

	nodeState := state.NodeStates["node1"]
	assert.Equal(t, NodeStatusRunning, nodeState.Status)
	assert.NotNil(t, nodeState.StartedAt)
	assert.True(t, nodeState.StartedAt.After(before) || nodeState.StartedAt.Equal(before))
	assert.Nil(t, nodeState.CompletedAt)
}

// TestMarkNodeCompleted verifies MarkNodeCompleted updates status, result, and timestamp
func TestMarkNodeCompleted(t *testing.T) {
	workflow := &Workflow{
		ID:    types.NewID(),
		Nodes: map[string]*WorkflowNode{"node1": {ID: "node1"}},
	}

	state := NewWorkflowState(workflow)
	state.MarkNodeStarted("node1")

	result := &NodeResult{
		NodeID: "node1",
		Status: NodeStatusCompleted,
		Output: map[string]any{"key": "value"},
	}

	before := time.Now()
	state.MarkNodeCompleted("node1", result)

	nodeState := state.NodeStates["node1"]
	assert.Equal(t, NodeStatusCompleted, nodeState.Status)
	assert.NotNil(t, nodeState.CompletedAt)
	assert.True(t, nodeState.CompletedAt.After(before) || nodeState.CompletedAt.Equal(before))

	// Verify result is stored
	storedResult := state.GetResult("node1")
	assert.Equal(t, result, storedResult)
}

// TestMarkNodeFailed verifies MarkNodeFailed updates status and error
func TestMarkNodeFailed(t *testing.T) {
	workflow := &Workflow{
		ID:    types.NewID(),
		Nodes: map[string]*WorkflowNode{"node1": {ID: "node1"}},
	}

	state := NewWorkflowState(workflow)
	state.MarkNodeStarted("node1")

	testErr := errors.New("test error")
	before := time.Now()
	state.MarkNodeFailed("node1", testErr)

	nodeState := state.NodeStates["node1"]
	assert.Equal(t, NodeStatusFailed, nodeState.Status)
	assert.Equal(t, testErr, nodeState.Error)
	assert.NotNil(t, nodeState.CompletedAt)
	assert.True(t, nodeState.CompletedAt.After(before) || nodeState.CompletedAt.Equal(before))
}

// TestMarkNodeSkipped verifies MarkNodeSkipped updates status with reason
func TestMarkNodeSkipped(t *testing.T) {
	workflow := &Workflow{
		ID:    types.NewID(),
		Nodes: map[string]*WorkflowNode{"node1": {ID: "node1"}},
	}

	state := NewWorkflowState(workflow)

	reason := "dependency failed"
	before := time.Now()
	state.MarkNodeSkipped("node1", reason)

	nodeState := state.NodeStates["node1"]
	assert.Equal(t, NodeStatusSkipped, nodeState.Status)
	assert.NotNil(t, nodeState.CompletedAt)
	assert.True(t, nodeState.CompletedAt.After(before) || nodeState.CompletedAt.Equal(before))

	// Verify skip reason is stored in result metadata
	result := state.GetResult("node1")
	require.NotNil(t, result)
	assert.Equal(t, NodeStatusSkipped, result.Status)
	assert.Equal(t, reason, result.Metadata["skip_reason"])
}

// TestIsComplete verifies IsComplete returns true only when all nodes have terminal status
func TestIsComplete(t *testing.T) {
	workflow := &Workflow{
		ID: types.NewID(),
		Nodes: map[string]*WorkflowNode{
			"node1": {ID: "node1"},
			"node2": {ID: "node2"},
			"node3": {ID: "node3"},
		},
	}

	state := NewWorkflowState(workflow)

	// Initially not complete (all pending)
	assert.False(t, state.IsComplete())

	// Mark node1 as completed
	state.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1"})
	assert.False(t, state.IsComplete())

	// Mark node2 as failed
	state.MarkNodeFailed("node2", errors.New("test error"))
	assert.False(t, state.IsComplete())

	// Mark node3 as skipped
	state.MarkNodeSkipped("node3", "dependency failed")
	assert.True(t, state.IsComplete())
}

// TestIsComplete_MixedTerminalStates verifies all terminal states are recognized
func TestIsComplete_MixedTerminalStates(t *testing.T) {
	workflow := &Workflow{
		ID: types.NewID(),
		Nodes: map[string]*WorkflowNode{
			"node1": {ID: "node1"},
			"node2": {ID: "node2"},
			"node3": {ID: "node3"},
			"node4": {ID: "node4"},
		},
	}

	state := NewWorkflowState(workflow)

	state.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1"})
	state.MarkNodeFailed("node2", errors.New("error"))
	state.MarkNodeSkipped("node3", "reason")

	// Set node4 to cancelled manually for testing
	state.mu.Lock()
	state.NodeStates["node4"].Status = NodeStatusCancelled
	state.mu.Unlock()

	assert.True(t, state.IsComplete())
}

// TestGetResult verifies GetResult returns correct results
func TestGetResult(t *testing.T) {
	workflow := &Workflow{
		ID: types.NewID(),
		Nodes: map[string]*WorkflowNode{
			"node1": {ID: "node1"},
			"node2": {ID: "node2"},
		},
	}

	state := NewWorkflowState(workflow)

	// No result initially
	result := state.GetResult("node1")
	assert.Nil(t, result)

	// Add result
	expectedResult := &NodeResult{
		NodeID: "node1",
		Output: map[string]any{"test": "data"},
	}
	state.MarkNodeCompleted("node1", expectedResult)

	// Retrieve result
	result = state.GetResult("node1")
	assert.Equal(t, expectedResult, result)

	// Non-existent node
	result = state.GetResult("nonexistent")
	assert.Nil(t, result)
}

// TestGetNodeStatus verifies GetNodeStatus returns correct status
func TestGetNodeStatus(t *testing.T) {
	workflow := &Workflow{
		ID:    types.NewID(),
		Nodes: map[string]*WorkflowNode{"node1": {ID: "node1"}},
	}

	state := NewWorkflowState(workflow)

	// Initial pending status
	status := state.GetNodeStatus("node1")
	assert.Equal(t, NodeStatusPending, status)

	// Update to running
	state.MarkNodeStarted("node1")
	status = state.GetNodeStatus("node1")
	assert.Equal(t, NodeStatusRunning, status)

	// Update to completed
	state.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1"})
	status = state.GetNodeStatus("node1")
	assert.Equal(t, NodeStatusCompleted, status)

	// Non-existent node
	status = state.GetNodeStatus("nonexistent")
	assert.Empty(t, status)
}

// TestConcurrentAccess verifies thread-safety with concurrent operations
func TestConcurrentAccess(t *testing.T) {
	workflow := &Workflow{
		ID: types.NewID(),
		Nodes: map[string]*WorkflowNode{
			"node1": {ID: "node1"},
			"node2": {ID: "node2"},
			"node3": {ID: "node3"},
		},
	}

	state := NewWorkflowState(workflow)
	var wg sync.WaitGroup

	// Concurrent writes
	wg.Add(3)
	go func() {
		defer wg.Done()
		state.MarkNodeStarted("node1")
		state.MarkNodeCompleted("node1", &NodeResult{NodeID: "node1"})
	}()
	go func() {
		defer wg.Done()
		state.MarkNodeStarted("node2")
		state.MarkNodeFailed("node2", errors.New("error"))
	}()
	go func() {
		defer wg.Done()
		state.MarkNodeSkipped("node3", "reason")
	}()

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = state.GetReadyNodes()
			_ = state.GetNodeStatus("node1")
			_ = state.GetResult("node1")
			_ = state.IsComplete()
		}()
	}

	wg.Wait()

	// Verify final state is consistent
	assert.Equal(t, NodeStatusCompleted, state.GetNodeStatus("node1"))
	assert.Equal(t, NodeStatusFailed, state.GetNodeStatus("node2"))
	assert.Equal(t, NodeStatusSkipped, state.GetNodeStatus("node3"))
	assert.True(t, state.IsComplete())
}

// TestRetryCount verifies retry count is tracked in NodeState
func TestRetryCount(t *testing.T) {
	workflow := &Workflow{
		ID:    types.NewID(),
		Nodes: map[string]*WorkflowNode{"node1": {ID: "node1"}},
	}

	state := NewWorkflowState(workflow)

	// Initial retry count should be 0
	assert.Equal(t, 0, state.NodeStates["node1"].RetryCount)

	// Manually increment retry count
	state.mu.Lock()
	state.NodeStates["node1"].RetryCount++
	state.mu.Unlock()

	assert.Equal(t, 1, state.NodeStates["node1"].RetryCount)
}

// TestMarkOperationsOnNonexistentNode verifies graceful handling of invalid node IDs
func TestMarkOperationsOnNonexistentNode(t *testing.T) {
	workflow := &Workflow{
		ID:    types.NewID(),
		Nodes: map[string]*WorkflowNode{},
	}

	state := NewWorkflowState(workflow)

	// These should not panic
	state.MarkNodeStarted("nonexistent")
	state.MarkNodeCompleted("nonexistent", &NodeResult{})
	state.MarkNodeFailed("nonexistent", errors.New("error"))
	state.MarkNodeSkipped("nonexistent", "reason")

	// Verify operations had no effect
	assert.Empty(t, state.NodeStates)
	assert.Empty(t, state.Results)
}
