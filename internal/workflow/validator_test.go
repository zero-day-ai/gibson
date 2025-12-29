package workflow

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestNewDAGValidator(t *testing.T) {
	validator := NewDAGValidator()
	assert.NotNil(t, validator)
}

func TestDAGValidator_Validate_EmptyWorkflow(t *testing.T) {
	validator := NewDAGValidator()

	tests := []struct {
		name        string
		workflow    *Workflow
		expectError bool
		errorCode   WorkflowErrorCode
	}{
		{
			name:        "nil workflow",
			workflow:    nil,
			expectError: true,
			errorCode:   WorkflowErrorInvalidWorkflow,
		},
		{
			name: "workflow with no nodes",
			workflow: &Workflow{
				ID:    types.NewID(),
				Nodes: map[string]*WorkflowNode{},
			},
			expectError: true,
			errorCode:   WorkflowErrorInvalidWorkflow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.workflow)
			if tt.expectError {
				require.Error(t, err)
				workflowErr, ok := err.(*WorkflowError)
				require.True(t, ok)
				assert.Equal(t, tt.errorCode, workflowErr.Code)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDAGValidator_Validate_SingleNode(t *testing.T) {
	validator := NewDAGValidator()

	workflow := &Workflow{
		ID:   types.NewID(),
		Name: "single-node",
		Nodes: map[string]*WorkflowNode{
			"node1": {
				ID:   "node1",
				Type: NodeTypeAgent,
				Name: "Test Node",
			},
		},
		CreatedAt: time.Now(),
	}

	err := validator.Validate(workflow)
	require.NoError(t, err)

	// Check entry and exit points are computed
	assert.Equal(t, []string{"node1"}, workflow.EntryPoints)
	assert.Equal(t, []string{"node1"}, workflow.ExitPoints)
}

func TestDAGValidator_Validate_LinearWorkflow(t *testing.T) {
	validator := NewDAGValidator()

	workflow := &Workflow{
		ID:   types.NewID(),
		Name: "linear-workflow",
		Nodes: map[string]*WorkflowNode{
			"node1": {
				ID:   "node1",
				Type: NodeTypeAgent,
				Name: "Node 1",
			},
			"node2": {
				ID:           "node2",
				Type:         NodeTypeAgent,
				Name:         "Node 2",
				Dependencies: []string{"node1"},
			},
			"node3": {
				ID:           "node3",
				Type:         NodeTypeAgent,
				Name:         "Node 3",
				Dependencies: []string{"node2"},
			},
		},
		CreatedAt: time.Now(),
	}

	err := validator.Validate(workflow)
	require.NoError(t, err)

	assert.Equal(t, []string{"node1"}, workflow.EntryPoints)
	assert.Equal(t, []string{"node3"}, workflow.ExitPoints)
}

func TestDAGValidator_DetectCycles_NoCycle(t *testing.T) {
	validator := NewDAGValidator()

	tests := []struct {
		name     string
		workflow *Workflow
	}{
		{
			name:     "nil workflow",
			workflow: nil,
		},
		{
			name: "empty workflow",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{},
			},
		},
		{
			name: "single node",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
				},
			},
		},
		{
			name: "linear chain",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node2"}},
				},
			},
		},
		{
			name: "diamond shape",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node4": {ID: "node4", Type: NodeTypeAgent, Dependencies: []string{"node2", "node3"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cycle, err := validator.DetectCycles(tt.workflow)
			require.NoError(t, err)
			assert.Empty(t, cycle)
		})
	}
}

func TestDAGValidator_DetectCycles_WithCycle(t *testing.T) {
	validator := NewDAGValidator()

	tests := []struct {
		name     string
		workflow *Workflow
	}{
		{
			name: "simple self-loop",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
				},
			},
		},
		{
			name: "two-node cycle",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent, Dependencies: []string{"node2"}},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
				},
			},
		},
		{
			name: "three-node cycle",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent, Dependencies: []string{"node2"}},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node3"}},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
				},
			},
		},
		{
			name: "cycle with edge",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent},
				},
				Edges: []WorkflowEdge{
					{From: "node1", To: "node2"},
					{From: "node2", To: "node1"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cycle, err := validator.DetectCycles(tt.workflow)
			require.NoError(t, err)
			assert.NotEmpty(t, cycle, "expected to detect a cycle")
		})
	}
}

func TestDAGValidator_TopologicalSort_Valid(t *testing.T) {
	validator := NewDAGValidator()

	tests := []struct {
		name           string
		workflow       *Workflow
		expectedLength int
	}{
		{
			name:           "nil workflow",
			workflow:       nil,
			expectedLength: 0,
		},
		{
			name: "single node",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
				},
			},
			expectedLength: 1,
		},
		{
			name: "linear chain",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node2"}},
				},
			},
			expectedLength: 3,
		},
		{
			name: "diamond shape",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node4": {ID: "node4", Type: NodeTypeAgent, Dependencies: []string{"node2", "node3"}},
				},
			},
			expectedLength: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sorted, err := validator.TopologicalSort(tt.workflow)
			require.NoError(t, err)
			assert.Len(t, sorted, tt.expectedLength)

			// Verify ordering is valid (all dependencies come before dependents)
			if tt.workflow != nil && len(tt.workflow.Nodes) > 0 {
				position := make(map[string]int)
				for i, nodeID := range sorted {
					position[nodeID] = i
				}

				for nodeID, node := range tt.workflow.Nodes {
					if node == nil {
						continue
					}
					for _, depID := range node.Dependencies {
						assert.Less(t, position[depID], position[nodeID],
							"dependency %s should come before %s", depID, nodeID)
					}
				}
			}
		})
	}
}

func TestDAGValidator_TopologicalSort_WithCycle(t *testing.T) {
	validator := NewDAGValidator()

	workflow := &Workflow{
		Nodes: map[string]*WorkflowNode{
			"node1": {ID: "node1", Type: NodeTypeAgent, Dependencies: []string{"node2"}},
			"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
		},
	}

	sorted, err := validator.TopologicalSort(workflow)
	require.Error(t, err)
	assert.Nil(t, sorted)

	workflowErr, ok := err.(*WorkflowError)
	require.True(t, ok)
	assert.Equal(t, WorkflowErrorCycleDetected, workflowErr.Code)
}

func TestDAGValidator_ValidateDependencies_Valid(t *testing.T) {
	validator := NewDAGValidator()

	tests := []struct {
		name     string
		workflow *Workflow
	}{
		{
			name:     "nil workflow",
			workflow: nil,
		},
		{
			name: "empty workflow",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{},
			},
		},
		{
			name: "single node no dependencies",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
				},
			},
		},
		{
			name: "valid dependencies",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
				},
			},
		},
		{
			name: "valid edges",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent},
				},
				Edges: []WorkflowEdge{
					{From: "node1", To: "node2"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateDependencies(tt.workflow)
			require.NoError(t, err)
		})
	}
}

func TestDAGValidator_ValidateDependencies_Invalid(t *testing.T) {
	validator := NewDAGValidator()

	tests := []struct {
		name      string
		workflow  *Workflow
		errorCode WorkflowErrorCode
	}{
		{
			name: "missing dependency",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent, Dependencies: []string{"nonexistent"}},
				},
			},
			errorCode: WorkflowErrorMissingDependency,
		},
		{
			name: "edge with missing source",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
				},
				Edges: []WorkflowEdge{
					{From: "nonexistent", To: "node1"},
				},
			},
			errorCode: WorkflowErrorMissingDependency,
		},
		{
			name: "edge with missing destination",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
				},
				Edges: []WorkflowEdge{
					{From: "node1", To: "nonexistent"},
				},
			},
			errorCode: WorkflowErrorMissingDependency,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateDependencies(tt.workflow)
			require.Error(t, err)

			workflowErr, ok := err.(*WorkflowError)
			require.True(t, ok)
			assert.Equal(t, tt.errorCode, workflowErr.Code)
		})
	}
}

func TestDAGValidator_ComputeEntryExitPoints(t *testing.T) {
	validator := NewDAGValidator()

	tests := []struct {
		name               string
		workflow           *Workflow
		expectedEntryCount int
		expectedExitCount  int
	}{
		{
			name: "single node",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
				},
			},
			expectedEntryCount: 1,
			expectedExitCount:  1,
		},
		{
			name: "linear chain",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node2"}},
				},
			},
			expectedEntryCount: 1,
			expectedExitCount:  1,
		},
		{
			name: "diamond shape",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node4": {ID: "node4", Type: NodeTypeAgent, Dependencies: []string{"node2", "node3"}},
				},
			},
			expectedEntryCount: 1,
			expectedExitCount:  1,
		},
		{
			name: "disconnected nodes",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent},
					"node3": {ID: "node3", Type: NodeTypeAgent},
				},
			},
			expectedEntryCount: 3,
			expectedExitCount:  3,
		},
		{
			name: "multiple entry points",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node1", "node2"}},
				},
			},
			expectedEntryCount: 2,
			expectedExitCount:  1,
		},
		{
			name: "multiple exit points",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
				},
			},
			expectedEntryCount: 1,
			expectedExitCount:  2,
		},
		{
			name: "using edges instead of dependencies",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent},
					"node3": {ID: "node3", Type: NodeTypeAgent},
				},
				Edges: []WorkflowEdge{
					{From: "node1", To: "node2"},
					{From: "node2", To: "node3"},
				},
			},
			expectedEntryCount: 1,
			expectedExitCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator.computeEntryExitPoints(tt.workflow)

			assert.Len(t, tt.workflow.EntryPoints, tt.expectedEntryCount,
				"expected %d entry points, got %d: %v",
				tt.expectedEntryCount, len(tt.workflow.EntryPoints), tt.workflow.EntryPoints)
			assert.Len(t, tt.workflow.ExitPoints, tt.expectedExitCount,
				"expected %d exit points, got %d: %v",
				tt.expectedExitCount, len(tt.workflow.ExitPoints), tt.workflow.ExitPoints)
		})
	}
}

func TestDAGValidator_Validate_Integration(t *testing.T) {
	validator := NewDAGValidator()

	t.Run("valid complex workflow", func(t *testing.T) {
		workflow := &Workflow{
			ID:   types.NewID(),
			Name: "complex-workflow",
			Nodes: map[string]*WorkflowNode{
				"start": {
					ID:   "start",
					Type: NodeTypeAgent,
					Name: "Start",
				},
				"parallel1": {
					ID:           "parallel1",
					Type:         NodeTypeAgent,
					Name:         "Parallel 1",
					Dependencies: []string{"start"},
				},
				"parallel2": {
					ID:           "parallel2",
					Type:         NodeTypeAgent,
					Name:         "Parallel 2",
					Dependencies: []string{"start"},
				},
				"join": {
					ID:           "join",
					Type:         NodeTypeJoin,
					Name:         "Join",
					Dependencies: []string{"parallel1", "parallel2"},
				},
				"end": {
					ID:           "end",
					Type:         NodeTypeAgent,
					Name:         "End",
					Dependencies: []string{"join"},
				},
			},
			CreatedAt: time.Now(),
		}

		err := validator.Validate(workflow)
		require.NoError(t, err)

		assert.Equal(t, []string{"start"}, workflow.EntryPoints)
		assert.Equal(t, []string{"end"}, workflow.ExitPoints)

		// Verify topological sort works
		sorted, err := validator.TopologicalSort(workflow)
		require.NoError(t, err)
		assert.Len(t, sorted, 5)

		// Verify "start" comes first and "end" comes last
		assert.Equal(t, "start", sorted[0])
		assert.Equal(t, "end", sorted[len(sorted)-1])
	})

	t.Run("workflow with cycle fails validation", func(t *testing.T) {
		workflow := &Workflow{
			ID:   types.NewID(),
			Name: "cyclic-workflow",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:           "node1",
					Type:         NodeTypeAgent,
					Dependencies: []string{"node3"},
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
			CreatedAt: time.Now(),
		}

		err := validator.Validate(workflow)
		require.Error(t, err)

		workflowErr, ok := err.(*WorkflowError)
		require.True(t, ok)
		assert.Equal(t, WorkflowErrorCycleDetected, workflowErr.Code)
	})
}

func TestDAGValidator_DetectCycles_ComplexScenarios(t *testing.T) {
	validator := NewDAGValidator()

	tests := []struct {
		name        string
		workflow    *Workflow
		expectCycle bool
		description string
	}{
		{
			name:        "long cycle chain (5 nodes)",
			expectCycle: true,
			description: "Detects cycles in longer chains",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent, Dependencies: []string{"node5"}},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node2"}},
					"node4": {ID: "node4", Type: NodeTypeAgent, Dependencies: []string{"node3"}},
					"node5": {ID: "node5", Type: NodeTypeAgent, Dependencies: []string{"node4"}},
				},
			},
		},
		{
			name:        "cycle in disconnected components",
			expectCycle: true,
			description: "Detects cycles even when workflow has disconnected components",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					// Component 1 (valid)
					"a1": {ID: "a1", Type: NodeTypeAgent},
					"a2": {ID: "a2", Type: NodeTypeAgent, Dependencies: []string{"a1"}},
					// Component 2 (has cycle)
					"b1": {ID: "b1", Type: NodeTypeAgent, Dependencies: []string{"b2"}},
					"b2": {ID: "b2", Type: NodeTypeAgent, Dependencies: []string{"b1"}},
					// Component 3 (valid)
					"c1": {ID: "c1", Type: NodeTypeAgent},
				},
			},
		},
		{
			name:        "self-loop in complex graph",
			expectCycle: true,
			description: "Detects self-loops even in larger workflows",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node2", "node3"}}, // self-loop
					"node4": {ID: "node4", Type: NodeTypeAgent, Dependencies: []string{"node3"}},
				},
			},
		},
		{
			name:        "cycle with mixed edges and dependencies",
			expectCycle: true,
			description: "Detects cycles when using both edges and dependencies",
			workflow: &Workflow{
				// Cycle: node1 → node2 (edge) → node3 (edge) → node1 (dependency)
				// node3 depends on node1, meaning node1 must complete before node3
				// But node1 → node2 → node3 means node3 can't complete before node1
				Nodes: map[string]*WorkflowNode{
					"node1": {ID: "node1", Type: NodeTypeAgent},
					"node2": {ID: "node2", Type: NodeTypeAgent},
					"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
				},
				Edges: []WorkflowEdge{
					{From: "node1", To: "node2"},
					{From: "node2", To: "node3"},
					{From: "node3", To: "node1"}, // Creates cycle: node3 → node1 but node1 → node2 → node3
				},
			},
		},
		{
			name:        "complex diamond with no cycle",
			expectCycle: false,
			description: "Correctly validates complex diamond patterns without cycles",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"start": {ID: "start", Type: NodeTypeAgent},
					"a1":    {ID: "a1", Type: NodeTypeAgent, Dependencies: []string{"start"}},
					"a2":    {ID: "a2", Type: NodeTypeAgent, Dependencies: []string{"start"}},
					"b1":    {ID: "b1", Type: NodeTypeAgent, Dependencies: []string{"a1"}},
					"b2":    {ID: "b2", Type: NodeTypeAgent, Dependencies: []string{"a2"}},
					"join":  {ID: "join", Type: NodeTypeJoin, Dependencies: []string{"b1", "b2"}},
				},
			},
		},
		{
			name:        "multiple entry and exit points no cycle",
			expectCycle: false,
			description: "Handles multiple entry/exit points correctly",
			workflow: &Workflow{
				Nodes: map[string]*WorkflowNode{
					"entry1": {ID: "entry1", Type: NodeTypeAgent},
					"entry2": {ID: "entry2", Type: NodeTypeAgent},
					"middle": {ID: "middle", Type: NodeTypeAgent, Dependencies: []string{"entry1", "entry2"}},
					"exit1":  {ID: "exit1", Type: NodeTypeAgent, Dependencies: []string{"middle"}},
					"exit2":  {ID: "exit2", Type: NodeTypeAgent, Dependencies: []string{"middle"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cycle, err := validator.DetectCycles(tt.workflow)
			require.NoError(t, err)

			if tt.expectCycle {
				assert.NotEmpty(t, cycle, "expected to detect a cycle: %s", tt.description)
			} else {
				assert.Empty(t, cycle, "expected no cycle: %s", tt.description)
			}
		})
	}
}

func TestDAGValidator_TopologicalSort_ComplexOrderings(t *testing.T) {
	validator := NewDAGValidator()

	t.Run("deep dependency chain", func(t *testing.T) {
		// Create a deep chain: node1 -> node2 -> node3 -> ... -> node10
		workflow := &Workflow{
			Nodes: make(map[string]*WorkflowNode),
		}

		for i := 1; i <= 10; i++ {
			nodeID := fmt.Sprintf("node%d", i)
			node := &WorkflowNode{
				ID:   nodeID,
				Type: NodeTypeAgent,
			}
			if i > 1 {
				node.Dependencies = []string{fmt.Sprintf("node%d", i-1)}
			}
			workflow.Nodes[nodeID] = node
		}

		sorted, err := validator.TopologicalSort(workflow)
		require.NoError(t, err)
		assert.Len(t, sorted, 10)

		// Verify strict ordering
		for i := 0; i < 10; i++ {
			assert.Equal(t, fmt.Sprintf("node%d", i+1), sorted[i])
		}
	})

	t.Run("complex multi-level dependencies", func(t *testing.T) {
		workflow := &Workflow{
			Nodes: map[string]*WorkflowNode{
				"root": {ID: "root", Type: NodeTypeAgent},
				"l1_a": {ID: "l1_a", Type: NodeTypeAgent, Dependencies: []string{"root"}},
				"l1_b": {ID: "l1_b", Type: NodeTypeAgent, Dependencies: []string{"root"}},
				"l2_a": {ID: "l2_a", Type: NodeTypeAgent, Dependencies: []string{"l1_a", "l1_b"}},
				"l2_b": {ID: "l2_b", Type: NodeTypeAgent, Dependencies: []string{"l1_a"}},
				"l3":   {ID: "l3", Type: NodeTypeAgent, Dependencies: []string{"l2_a", "l2_b"}},
			},
		}

		sorted, err := validator.TopologicalSort(workflow)
		require.NoError(t, err)
		assert.Len(t, sorted, 6)

		// Create position map
		position := make(map[string]int)
		for i, nodeID := range sorted {
			position[nodeID] = i
		}

		// Verify all dependencies are satisfied
		for nodeID, node := range workflow.Nodes {
			for _, depID := range node.Dependencies {
				assert.Less(t, position[depID], position[nodeID],
					"dependency %s should come before %s", depID, nodeID)
			}
		}

		// Verify specific orderings
		assert.Less(t, position["root"], position["l1_a"], "root should come before l1_a")
		assert.Less(t, position["root"], position["l1_b"], "root should come before l1_b")
		assert.Less(t, position["l1_a"], position["l2_a"], "l1_a should come before l2_a")
		assert.Less(t, position["l1_b"], position["l2_a"], "l1_b should come before l2_a")
		assert.Less(t, position["l2_a"], position["l3"], "l2_a should come before l3")
		assert.Less(t, position["l2_b"], position["l3"], "l2_b should come before l3")
	})

	t.Run("workflow with only edges", func(t *testing.T) {
		workflow := &Workflow{
			Nodes: map[string]*WorkflowNode{
				"node1": {ID: "node1", Type: NodeTypeAgent},
				"node2": {ID: "node2", Type: NodeTypeAgent},
				"node3": {ID: "node3", Type: NodeTypeAgent},
				"node4": {ID: "node4", Type: NodeTypeAgent},
			},
			Edges: []WorkflowEdge{
				{From: "node1", To: "node2"},
				{From: "node2", To: "node3"},
				{From: "node1", To: "node4"},
				{From: "node4", To: "node3"},
			},
		}

		sorted, err := validator.TopologicalSort(workflow)
		require.NoError(t, err)
		assert.Len(t, sorted, 4)

		position := make(map[string]int)
		for i, nodeID := range sorted {
			position[nodeID] = i
		}

		// Verify edge orderings are respected
		assert.Less(t, position["node1"], position["node2"])
		assert.Less(t, position["node2"], position["node3"])
		assert.Less(t, position["node1"], position["node4"])
		assert.Less(t, position["node4"], position["node3"])
	})

	t.Run("mixed edges and dependencies", func(t *testing.T) {
		workflow := &Workflow{
			Nodes: map[string]*WorkflowNode{
				"node1": {ID: "node1", Type: NodeTypeAgent},
				"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
				"node3": {ID: "node3", Type: NodeTypeAgent},
				"node4": {ID: "node4", Type: NodeTypeAgent},
			},
			Edges: []WorkflowEdge{
				{From: "node3", To: "node4"},
				{From: "node2", To: "node4"},
			},
		}

		sorted, err := validator.TopologicalSort(workflow)
		require.NoError(t, err)
		assert.Len(t, sorted, 4)

		position := make(map[string]int)
		for i, nodeID := range sorted {
			position[nodeID] = i
		}

		// Verify both dependency and edge orderings
		assert.Less(t, position["node1"], position["node2"], "dependency: node1 -> node2")
		assert.Less(t, position["node3"], position["node4"], "edge: node3 -> node4")
		assert.Less(t, position["node2"], position["node4"], "edge: node2 -> node4")
	})
}

func TestDAGValidator_DisconnectedComponents(t *testing.T) {
	validator := NewDAGValidator()

	t.Run("multiple disconnected valid components", func(t *testing.T) {
		workflow := &Workflow{
			ID:   types.NewID(),
			Name: "disconnected-workflow",
			Nodes: map[string]*WorkflowNode{
				// Component 1
				"a1": {ID: "a1", Type: NodeTypeAgent},
				"a2": {ID: "a2", Type: NodeTypeAgent, Dependencies: []string{"a1"}},
				"a3": {ID: "a3", Type: NodeTypeAgent, Dependencies: []string{"a2"}},
				// Component 2
				"b1": {ID: "b1", Type: NodeTypeAgent},
				"b2": {ID: "b2", Type: NodeTypeAgent, Dependencies: []string{"b1"}},
				// Component 3 (single node)
				"c1": {ID: "c1", Type: NodeTypeAgent},
			},
			CreatedAt: time.Now(),
		}

		err := validator.Validate(workflow)
		require.NoError(t, err)

		// Should have 3 entry points and 3 exit points
		assert.Len(t, workflow.EntryPoints, 3)
		assert.Len(t, workflow.ExitPoints, 3)
		assert.ElementsMatch(t, []string{"a1", "b1", "c1"}, workflow.EntryPoints)
		assert.ElementsMatch(t, []string{"a3", "b2", "c1"}, workflow.ExitPoints)

		// Topological sort should still work
		sorted, err := validator.TopologicalSort(workflow)
		require.NoError(t, err)
		assert.Len(t, sorted, 6)

		// Verify ordering within components
		position := make(map[string]int)
		for i, nodeID := range sorted {
			position[nodeID] = i
		}

		assert.Less(t, position["a1"], position["a2"])
		assert.Less(t, position["a2"], position["a3"])
		assert.Less(t, position["b1"], position["b2"])
	})

	t.Run("disconnected components with one having cycle", func(t *testing.T) {
		workflow := &Workflow{
			Nodes: map[string]*WorkflowNode{
				// Valid component
				"a1": {ID: "a1", Type: NodeTypeAgent},
				"a2": {ID: "a2", Type: NodeTypeAgent, Dependencies: []string{"a1"}},
				// Component with cycle
				"b1": {ID: "b1", Type: NodeTypeAgent, Dependencies: []string{"b2"}},
				"b2": {ID: "b2", Type: NodeTypeAgent, Dependencies: []string{"b1"}},
			},
		}

		cycle, err := validator.DetectCycles(workflow)
		require.NoError(t, err)
		assert.NotEmpty(t, cycle, "should detect cycle in one component")
	})
}

func TestDAGValidator_EdgeCases(t *testing.T) {
	validator := NewDAGValidator()

	t.Run("node with nil in nodes map", func(t *testing.T) {
		workflow := &Workflow{
			Nodes: map[string]*WorkflowNode{
				"node1": {ID: "node1", Type: NodeTypeAgent},
				"node2": nil, // nil node
				"node3": {ID: "node3", Type: NodeTypeAgent, Dependencies: []string{"node1"}},
			},
		}

		// Should not panic
		err := validator.ValidateDependencies(workflow)
		require.NoError(t, err)

		cycle, err := validator.DetectCycles(workflow)
		require.NoError(t, err)
		assert.Empty(t, cycle)
	})

	t.Run("empty dependencies array", func(t *testing.T) {
		workflow := &Workflow{
			Nodes: map[string]*WorkflowNode{
				"node1": {ID: "node1", Type: NodeTypeAgent, Dependencies: []string{}},
			},
		}

		err := validator.Validate(workflow)
		require.NoError(t, err)
		assert.Equal(t, []string{"node1"}, workflow.EntryPoints)
		assert.Equal(t, []string{"node1"}, workflow.ExitPoints)
	})

	t.Run("duplicate dependencies", func(t *testing.T) {
		workflow := &Workflow{
			Nodes: map[string]*WorkflowNode{
				"node1": {ID: "node1", Type: NodeTypeAgent},
				"node2": {ID: "node2", Type: NodeTypeAgent, Dependencies: []string{"node1", "node1", "node1"}},
			},
		}

		err := validator.Validate(workflow)
		require.NoError(t, err)

		sorted, err := validator.TopologicalSort(workflow)
		require.NoError(t, err)
		assert.Equal(t, []string{"node1", "node2"}, sorted)
	})

	t.Run("very large workflow", func(t *testing.T) {
		// Create a workflow with 100 nodes in a chain
		workflow := &Workflow{
			Nodes: make(map[string]*WorkflowNode),
		}

		for i := 0; i < 100; i++ {
			nodeID := fmt.Sprintf("node%d", i)
			node := &WorkflowNode{
				ID:   nodeID,
				Type: NodeTypeAgent,
			}
			if i > 0 {
				node.Dependencies = []string{fmt.Sprintf("node%d", i-1)}
			}
			workflow.Nodes[nodeID] = node
		}

		err := validator.Validate(workflow)
		require.NoError(t, err)

		sorted, err := validator.TopologicalSort(workflow)
		require.NoError(t, err)
		assert.Len(t, sorted, 100)

		assert.Equal(t, "node0", sorted[0])
		assert.Equal(t, "node99", sorted[99])
	})

	t.Run("duplicate edges", func(t *testing.T) {
		workflow := &Workflow{
			Nodes: map[string]*WorkflowNode{
				"node1": {ID: "node1", Type: NodeTypeAgent},
				"node2": {ID: "node2", Type: NodeTypeAgent},
			},
			Edges: []WorkflowEdge{
				{From: "node1", To: "node2"},
				{From: "node1", To: "node2"}, // duplicate
				{From: "node1", To: "node2"}, // duplicate
			},
		}

		err := validator.Validate(workflow)
		require.NoError(t, err)

		// Should still work correctly despite duplicates
		sorted, err := validator.TopologicalSort(workflow)
		require.NoError(t, err)
		assert.Equal(t, []string{"node1", "node2"}, sorted)
	})

	t.Run("workflow with conditions", func(t *testing.T) {
		workflow := &Workflow{
			ID:   types.NewID(),
			Name: "conditional-workflow",
			Nodes: map[string]*WorkflowNode{
				"start": {
					ID:   "start",
					Type: NodeTypeAgent,
				},
				"condition": {
					ID:           "condition",
					Type:         NodeTypeCondition,
					Dependencies: []string{"start"},
					Condition: &NodeCondition{
						Expression:  "result.status == 'success'",
						TrueBranch:  []string{"success_node"},
						FalseBranch: []string{"failure_node"},
					},
				},
				"success_node": {
					ID:           "success_node",
					Type:         NodeTypeAgent,
					Dependencies: []string{"condition"},
				},
				"failure_node": {
					ID:           "failure_node",
					Type:         NodeTypeAgent,
					Dependencies: []string{"condition"},
				},
			},
			CreatedAt: time.Now(),
		}

		err := validator.Validate(workflow)
		require.NoError(t, err)

		assert.Equal(t, []string{"start"}, workflow.EntryPoints)
		assert.ElementsMatch(t, []string{"success_node", "failure_node"}, workflow.ExitPoints)
	})
}
