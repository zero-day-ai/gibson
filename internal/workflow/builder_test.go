package workflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
)

// TestNewWorkflow tests the creation of a new WorkflowBuilder
func TestNewWorkflow(t *testing.T) {
	wb := NewWorkflow("test-workflow")

	require.NotNil(t, wb)
	require.NotNil(t, wb.workflow)
	assert.Equal(t, "test-workflow", wb.workflow.Name)
	assert.NotNil(t, wb.workflow.Nodes)
	assert.NotNil(t, wb.workflow.Edges)
	assert.NotNil(t, wb.workflow.Metadata)
	assert.Empty(t, wb.errors)
	assert.NotZero(t, wb.workflow.ID)
	assert.NotZero(t, wb.workflow.CreatedAt)
}

// TestFluentAPIChaining tests that all builder methods return *WorkflowBuilder for chaining
func TestFluentAPIChaining(t *testing.T) {
	t.Run("all methods return builder", func(t *testing.T) {
		wb := NewWorkflow("test")

		// Test that WithDescription returns the builder
		result1 := wb.WithDescription("test description")
		assert.Same(t, wb, result1, "WithDescription should return the same builder")

		// Test that AddNode returns the builder
		node := &WorkflowNode{ID: "node1", Type: NodeTypeAgent, Name: "Test Node"}
		result2 := wb.AddNode(node)
		assert.Same(t, wb, result2, "AddNode should return the same builder")

		// Test that AddAgentNode returns the builder
		result3 := wb.AddAgentNode("agent1", "test-agent", nil)
		assert.Same(t, wb, result3, "AddAgentNode should return the same builder")

		// Test that AddToolNode returns the builder
		result4 := wb.AddToolNode("tool1", "test-tool", nil)
		assert.Same(t, wb, result4, "AddToolNode should return the same builder")

		// Test that AddPluginNode returns the builder
		result5 := wb.AddPluginNode("plugin1", "test-plugin", "method1", nil)
		assert.Same(t, wb, result5, "AddPluginNode should return the same builder")

		// Test that AddConditionNode returns the builder
		condition := &NodeCondition{Expression: "test"}
		result6 := wb.AddConditionNode("cond1", condition)
		assert.Same(t, wb, result6, "AddConditionNode should return the same builder")

		// Test that AddEdge returns the builder
		result7 := wb.AddEdge("node1", "agent1")
		assert.Same(t, wb, result7, "AddEdge should return the same builder")

		// Test that AddConditionalEdge returns the builder
		result8 := wb.AddConditionalEdge("agent1", "tool1", "result == true")
		assert.Same(t, wb, result8, "AddConditionalEdge should return the same builder")

		// Test that WithDependency returns the builder
		result9 := wb.WithDependency("tool1", "agent1")
		assert.Same(t, wb, result9, "WithDependency should return the same builder")
	})

	t.Run("method chaining works", func(t *testing.T) {
		wb := NewWorkflow("chained")

		result := wb.
			WithDescription("test description").
			AddAgentNode("agent1", "test-agent", nil).
			AddToolNode("tool1", "test-tool", nil).
			AddEdge("agent1", "tool1")

		assert.Same(t, wb, result)
		assert.Equal(t, "test description", wb.workflow.Description)
		assert.Len(t, wb.workflow.Nodes, 2)
		assert.Len(t, wb.workflow.Edges, 1)
	})
}

// TestWithDescription tests the WithDescription method
func TestWithDescription(t *testing.T) {
	wb := NewWorkflow("test")
	wb.WithDescription("A test workflow description")

	assert.Equal(t, "A test workflow description", wb.workflow.Description)
}

// TestAddNode tests the AddNode method
func TestAddNode(t *testing.T) {
	tests := []struct {
		name        string
		node        *WorkflowNode
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid node",
			node: &WorkflowNode{
				ID:   "node1",
				Type: NodeTypeAgent,
				Name: "Test Node",
			},
			shouldError: false,
		},
		{
			name:        "nil node",
			node:        nil,
			shouldError: true,
			errorMsg:    "cannot add nil node",
		},
		{
			name: "node without ID",
			node: &WorkflowNode{
				Type: NodeTypeAgent,
				Name: "Test Node",
			},
			shouldError: true,
			errorMsg:    "node must have an ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wb := NewWorkflow("test")
			wb.AddNode(tt.node)

			if tt.shouldError {
				assert.NotEmpty(t, wb.errors)
				assert.Contains(t, wb.errors[0].Error(), tt.errorMsg)
			} else {
				assert.Empty(t, wb.errors)
				assert.Contains(t, wb.workflow.Nodes, tt.node.ID)
			}
		})
	}

	t.Run("duplicate node ID", func(t *testing.T) {
		wb := NewWorkflow("test")
		node1 := &WorkflowNode{ID: "node1", Type: NodeTypeAgent, Name: "Node 1"}
		node2 := &WorkflowNode{ID: "node1", Type: NodeTypeAgent, Name: "Node 2"}

		wb.AddNode(node1)
		assert.Empty(t, wb.errors)

		wb.AddNode(node2)
		require.NotEmpty(t, wb.errors)
		assert.Contains(t, wb.errors[0].Error(), "node with ID \"node1\" already exists")

		// Verify the first node is still there
		assert.Equal(t, "Node 1", wb.workflow.Nodes["node1"].Name)
	})
}

// TestAddAgentNode tests the AddAgentNode helper method
func TestAddAgentNode(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		agentName   string
		task        *agent.Task
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "valid agent node",
			id:          "agent1",
			agentName:   "test-agent",
			task:        &agent.Task{Description: "test task"},
			shouldError: false,
		},
		{
			name:        "valid agent node without task",
			id:          "agent2",
			agentName:   "test-agent",
			task:        nil,
			shouldError: false,
		},
		{
			name:        "empty ID",
			id:          "",
			agentName:   "test-agent",
			task:        nil,
			shouldError: true,
			errorMsg:    "agent node must have an ID",
		},
		{
			name:        "empty agent name",
			id:          "agent1",
			agentName:   "",
			task:        nil,
			shouldError: true,
			errorMsg:    "agent node \"agent1\" must have an agent name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wb := NewWorkflow("test")
			wb.AddAgentNode(tt.id, tt.agentName, tt.task)

			if tt.shouldError {
				assert.NotEmpty(t, wb.errors)
				assert.Contains(t, wb.errors[0].Error(), tt.errorMsg)
			} else {
				assert.Empty(t, wb.errors)
				require.Contains(t, wb.workflow.Nodes, tt.id)

				node := wb.workflow.Nodes[tt.id]
				assert.Equal(t, tt.id, node.ID)
				assert.Equal(t, NodeTypeAgent, node.Type)
				assert.Equal(t, tt.agentName, node.Name)
				assert.Equal(t, tt.agentName, node.AgentName)
				assert.Equal(t, tt.task, node.AgentTask)
				assert.NotNil(t, node.Metadata)
			}
		})
	}
}

// TestAddToolNode tests the AddToolNode helper method
func TestAddToolNode(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		toolName    string
		input       map[string]any
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "valid tool node",
			id:          "tool1",
			toolName:    "test-tool",
			input:       map[string]any{"key": "value"},
			shouldError: false,
		},
		{
			name:        "valid tool node without input",
			id:          "tool2",
			toolName:    "test-tool",
			input:       nil,
			shouldError: false,
		},
		{
			name:        "empty ID",
			id:          "",
			toolName:    "test-tool",
			input:       nil,
			shouldError: true,
			errorMsg:    "tool node must have an ID",
		},
		{
			name:        "empty tool name",
			id:          "tool1",
			toolName:    "",
			input:       nil,
			shouldError: true,
			errorMsg:    "tool node \"tool1\" must have a tool name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wb := NewWorkflow("test")
			wb.AddToolNode(tt.id, tt.toolName, tt.input)

			if tt.shouldError {
				assert.NotEmpty(t, wb.errors)
				assert.Contains(t, wb.errors[0].Error(), tt.errorMsg)
			} else {
				assert.Empty(t, wb.errors)
				require.Contains(t, wb.workflow.Nodes, tt.id)

				node := wb.workflow.Nodes[tt.id]
				assert.Equal(t, tt.id, node.ID)
				assert.Equal(t, NodeTypeTool, node.Type)
				assert.Equal(t, tt.toolName, node.Name)
				assert.Equal(t, tt.toolName, node.ToolName)
				assert.Equal(t, tt.input, node.ToolInput)
				assert.NotNil(t, node.Metadata)
			}
		})
	}
}

// TestAddPluginNode tests the AddPluginNode helper method
func TestAddPluginNode(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		pluginName  string
		method      string
		params      map[string]any
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "valid plugin node",
			id:          "plugin1",
			pluginName:  "test-plugin",
			method:      "execute",
			params:      map[string]any{"key": "value"},
			shouldError: false,
		},
		{
			name:        "valid plugin node without params",
			id:          "plugin2",
			pluginName:  "test-plugin",
			method:      "execute",
			params:      nil,
			shouldError: false,
		},
		{
			name:        "empty ID",
			id:          "",
			pluginName:  "test-plugin",
			method:      "execute",
			params:      nil,
			shouldError: true,
			errorMsg:    "plugin node must have an ID",
		},
		{
			name:        "empty plugin name",
			id:          "plugin1",
			pluginName:  "",
			method:      "execute",
			params:      nil,
			shouldError: true,
			errorMsg:    "plugin node \"plugin1\" must have a plugin name",
		},
		{
			name:        "empty method",
			id:          "plugin1",
			pluginName:  "test-plugin",
			method:      "",
			params:      nil,
			shouldError: true,
			errorMsg:    "plugin node \"plugin1\" must have a method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wb := NewWorkflow("test")
			wb.AddPluginNode(tt.id, tt.pluginName, tt.method, tt.params)

			if tt.shouldError {
				assert.NotEmpty(t, wb.errors)
				assert.Contains(t, wb.errors[0].Error(), tt.errorMsg)
			} else {
				assert.Empty(t, wb.errors)
				require.Contains(t, wb.workflow.Nodes, tt.id)

				node := wb.workflow.Nodes[tt.id]
				assert.Equal(t, tt.id, node.ID)
				assert.Equal(t, NodeTypePlugin, node.Type)
				assert.Equal(t, tt.pluginName+"."+tt.method, node.Name)
				assert.Equal(t, tt.pluginName, node.PluginName)
				assert.Equal(t, tt.method, node.PluginMethod)
				assert.Equal(t, tt.params, node.PluginParams)
				assert.NotNil(t, node.Metadata)
			}
		})
	}
}

// TestAddConditionNode tests the AddConditionNode helper method
func TestAddConditionNode(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		condition   *NodeCondition
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid condition node",
			id:   "cond1",
			condition: &NodeCondition{
				Expression:  "result == true",
				TrueBranch:  []string{"node1"},
				FalseBranch: []string{"node2"},
			},
			shouldError: false,
		},
		{
			name: "valid condition node without branches",
			id:   "cond2",
			condition: &NodeCondition{
				Expression: "result == true",
			},
			shouldError: false,
		},
		{
			name:        "empty ID",
			id:          "",
			condition:   &NodeCondition{Expression: "test"},
			shouldError: true,
			errorMsg:    "condition node must have an ID",
		},
		{
			name:        "nil condition",
			id:          "cond1",
			condition:   nil,
			shouldError: true,
			errorMsg:    "condition node \"cond1\" must have a condition",
		},
		{
			name:        "empty expression",
			id:          "cond1",
			condition:   &NodeCondition{Expression: ""},
			shouldError: true,
			errorMsg:    "condition node \"cond1\" must have a non-empty expression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wb := NewWorkflow("test")
			wb.AddConditionNode(tt.id, tt.condition)

			if tt.shouldError {
				assert.NotEmpty(t, wb.errors)
				assert.Contains(t, wb.errors[0].Error(), tt.errorMsg)
			} else {
				assert.Empty(t, wb.errors)
				require.Contains(t, wb.workflow.Nodes, tt.id)

				node := wb.workflow.Nodes[tt.id]
				assert.Equal(t, tt.id, node.ID)
				assert.Equal(t, NodeTypeCondition, node.Type)
				assert.Equal(t, "condition:"+tt.id, node.Name)
				assert.Equal(t, tt.condition, node.Condition)
				assert.NotNil(t, node.Metadata)
			}
		})
	}
}

// TestAddEdge tests the AddEdge method
func TestAddEdge(t *testing.T) {
	tests := []struct {
		name        string
		from        string
		to          string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "valid edge",
			from:        "node1",
			to:          "node2",
			shouldError: false,
		},
		{
			name:        "empty from",
			from:        "",
			to:          "node2",
			shouldError: true,
			errorMsg:    "edge must have a non-empty 'from' node",
		},
		{
			name:        "empty to",
			from:        "node1",
			to:          "",
			shouldError: true,
			errorMsg:    "edge must have a non-empty 'to' node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wb := NewWorkflow("test")
			wb.AddEdge(tt.from, tt.to)

			if tt.shouldError {
				assert.NotEmpty(t, wb.errors)
				assert.Contains(t, wb.errors[0].Error(), tt.errorMsg)
			} else {
				assert.Empty(t, wb.errors)
				require.Len(t, wb.workflow.Edges, 1)

				edge := wb.workflow.Edges[0]
				assert.Equal(t, tt.from, edge.From)
				assert.Equal(t, tt.to, edge.To)
				assert.Empty(t, edge.Condition)
			}
		})
	}

	t.Run("multiple edges", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddEdge("node1", "node2")
		wb.AddEdge("node2", "node3")
		wb.AddEdge("node1", "node3")

		assert.Empty(t, wb.errors)
		assert.Len(t, wb.workflow.Edges, 3)
	})
}

// TestAddConditionalEdge tests the AddConditionalEdge method
func TestAddConditionalEdge(t *testing.T) {
	tests := []struct {
		name        string
		from        string
		to          string
		condition   string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "valid conditional edge",
			from:        "node1",
			to:          "node2",
			condition:   "result == true",
			shouldError: false,
		},
		{
			name:        "empty from",
			from:        "",
			to:          "node2",
			condition:   "result == true",
			shouldError: true,
			errorMsg:    "conditional edge must have a non-empty 'from' node",
		},
		{
			name:        "empty to",
			from:        "node1",
			to:          "",
			condition:   "result == true",
			shouldError: true,
			errorMsg:    "conditional edge must have a non-empty 'to' node",
		},
		{
			name:        "empty condition",
			from:        "node1",
			to:          "node2",
			condition:   "",
			shouldError: true,
			errorMsg:    "conditional edge must have a non-empty condition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wb := NewWorkflow("test")
			wb.AddConditionalEdge(tt.from, tt.to, tt.condition)

			if tt.shouldError {
				assert.NotEmpty(t, wb.errors)
				assert.Contains(t, wb.errors[0].Error(), tt.errorMsg)
			} else {
				assert.Empty(t, wb.errors)
				require.Len(t, wb.workflow.Edges, 1)

				edge := wb.workflow.Edges[0]
				assert.Equal(t, tt.from, edge.From)
				assert.Equal(t, tt.to, edge.To)
				assert.Equal(t, tt.condition, edge.Condition)
			}
		})
	}
}

// TestWithDependency tests the WithDependency method
func TestWithDependency(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*WorkflowBuilder)
		nodeID      string
		dependsOn   []string
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid single dependency",
			setup: func(wb *WorkflowBuilder) {
				wb.AddAgentNode("node1", "agent1", nil)
				wb.AddAgentNode("node2", "agent2", nil)
			},
			nodeID:      "node2",
			dependsOn:   []string{"node1"},
			shouldError: false,
		},
		{
			name: "valid multiple dependencies",
			setup: func(wb *WorkflowBuilder) {
				wb.AddAgentNode("node1", "agent1", nil)
				wb.AddAgentNode("node2", "agent2", nil)
				wb.AddAgentNode("node3", "agent3", nil)
			},
			nodeID:      "node3",
			dependsOn:   []string{"node1", "node2"},
			shouldError: false,
		},
		{
			name:        "non-existent node",
			setup:       func(wb *WorkflowBuilder) {},
			nodeID:      "nonexistent",
			dependsOn:   []string{"node1"},
			shouldError: true,
			errorMsg:    "cannot set dependencies on non-existent node \"nonexistent\"",
		},
		{
			name: "empty dependencies",
			setup: func(wb *WorkflowBuilder) {
				wb.AddAgentNode("node1", "agent1", nil)
			},
			nodeID:      "node1",
			dependsOn:   []string{},
			shouldError: true,
			errorMsg:    "must specify at least one dependency for node \"node1\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wb := NewWorkflow("test")
			tt.setup(wb)

			wb.WithDependency(tt.nodeID, tt.dependsOn...)

			if tt.shouldError {
				assert.NotEmpty(t, wb.errors)
				assert.Contains(t, wb.errors[0].Error(), tt.errorMsg)
			} else {
				assert.Empty(t, wb.errors)
				node := wb.workflow.Nodes[tt.nodeID]
				require.NotNil(t, node)
				assert.Equal(t, tt.dependsOn, node.Dependencies)
			}
		})
	}

	t.Run("append to existing dependencies", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddAgentNode("node2", "agent2", nil)
		wb.AddAgentNode("node3", "agent3", nil)

		wb.WithDependency("node3", "node1")
		wb.WithDependency("node3", "node2")

		assert.Empty(t, wb.errors)
		node := wb.workflow.Nodes["node3"]
		assert.Equal(t, []string{"node1", "node2"}, node.Dependencies)
	})
}

// TestBuild_Validation tests the Build method's validation logic
func TestBuild_Validation(t *testing.T) {
	t.Run("empty workflow", func(t *testing.T) {
		wb := NewWorkflow("test")

		workflow, err := wb.Build()

		assert.Nil(t, workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workflow must have at least one node")
	})

	t.Run("valid simple workflow", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)

		workflow, err := wb.Build()

		require.NoError(t, err)
		require.NotNil(t, workflow)
		assert.Equal(t, "test", workflow.Name)
		assert.Len(t, workflow.Nodes, 1)
	})

	t.Run("edge references non-existent from node", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddEdge("nonexistent", "node1")

		workflow, err := wb.Build()

		assert.Nil(t, workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "edge references non-existent 'from' node \"nonexistent\"")
	})

	t.Run("edge references non-existent to node", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddEdge("node1", "nonexistent")

		workflow, err := wb.Build()

		assert.Nil(t, workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "edge references non-existent 'to' node \"nonexistent\"")
	})

	t.Run("dependency references non-existent node", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.WithDependency("node1", "nonexistent")

		workflow, err := wb.Build()

		assert.Nil(t, workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node \"node1\" depends on non-existent node \"nonexistent\"")
	})
}

// TestBuild_CycleDetection tests the Build method's cycle detection
func TestBuild_CycleDetection(t *testing.T) {
	t.Run("no cycle - linear workflow", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddAgentNode("node2", "agent2", nil)
		wb.AddAgentNode("node3", "agent3", nil)
		wb.AddEdge("node1", "node2")
		wb.AddEdge("node2", "node3")

		workflow, err := wb.Build()

		require.NoError(t, err)
		assert.NotNil(t, workflow)
	})

	t.Run("no cycle - diamond shape", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddAgentNode("node2", "agent2", nil)
		wb.AddAgentNode("node3", "agent3", nil)
		wb.AddAgentNode("node4", "agent4", nil)
		wb.AddEdge("node1", "node2")
		wb.AddEdge("node1", "node3")
		wb.AddEdge("node2", "node4")
		wb.AddEdge("node3", "node4")

		workflow, err := wb.Build()

		require.NoError(t, err)
		assert.NotNil(t, workflow)
	})

	t.Run("cycle - self loop", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddEdge("node1", "node1")

		workflow, err := wb.Build()

		assert.Nil(t, workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected in workflow")
	})

	t.Run("cycle - two nodes", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddAgentNode("node2", "agent2", nil)
		wb.AddEdge("node1", "node2")
		wb.AddEdge("node2", "node1")

		workflow, err := wb.Build()

		assert.Nil(t, workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected in workflow")
	})

	t.Run("cycle - three nodes", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddAgentNode("node2", "agent2", nil)
		wb.AddAgentNode("node3", "agent3", nil)
		wb.AddEdge("node1", "node2")
		wb.AddEdge("node2", "node3")
		wb.AddEdge("node3", "node1")

		workflow, err := wb.Build()

		assert.Nil(t, workflow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected in workflow")
	})
}

// TestBuild_EntryExitPoints tests the computation of entry and exit points
func TestBuild_EntryExitPoints(t *testing.T) {
	t.Run("single node - both entry and exit", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)

		workflow, err := wb.Build()

		require.NoError(t, err)
		assert.Equal(t, []string{"node1"}, workflow.EntryPoints)
		assert.Equal(t, []string{"node1"}, workflow.ExitPoints)
	})

	t.Run("linear workflow", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddAgentNode("node2", "agent2", nil)
		wb.AddAgentNode("node3", "agent3", nil)
		wb.AddEdge("node1", "node2")
		wb.AddEdge("node2", "node3")

		workflow, err := wb.Build()

		require.NoError(t, err)
		assert.Contains(t, workflow.EntryPoints, "node1")
		assert.Len(t, workflow.EntryPoints, 1)
		assert.Contains(t, workflow.ExitPoints, "node3")
		assert.Len(t, workflow.ExitPoints, 1)
	})

	t.Run("multiple entry points", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddAgentNode("node2", "agent2", nil)
		wb.AddAgentNode("node3", "agent3", nil)
		wb.AddEdge("node1", "node3")
		wb.AddEdge("node2", "node3")

		workflow, err := wb.Build()

		require.NoError(t, err)
		assert.Len(t, workflow.EntryPoints, 2)
		assert.Contains(t, workflow.EntryPoints, "node1")
		assert.Contains(t, workflow.EntryPoints, "node2")
		assert.Equal(t, []string{"node3"}, workflow.ExitPoints)
	})

	t.Run("multiple exit points", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddAgentNode("node2", "agent2", nil)
		wb.AddAgentNode("node3", "agent3", nil)
		wb.AddEdge("node1", "node2")
		wb.AddEdge("node1", "node3")

		workflow, err := wb.Build()

		require.NoError(t, err)
		assert.Equal(t, []string{"node1"}, workflow.EntryPoints)
		assert.Len(t, workflow.ExitPoints, 2)
		assert.Contains(t, workflow.ExitPoints, "node2")
		assert.Contains(t, workflow.ExitPoints, "node3")
	})

	t.Run("disconnected nodes", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddAgentNode("node2", "agent2", nil)
		wb.AddAgentNode("node3", "agent3", nil)

		workflow, err := wb.Build()

		require.NoError(t, err)
		assert.Len(t, workflow.EntryPoints, 3)
		assert.Len(t, workflow.ExitPoints, 3)
	})
}

// TestBuild_ErrorAccumulation tests that multiple errors are reported together
func TestBuild_ErrorAccumulation(t *testing.T) {
	t.Run("multiple errors accumulated", func(t *testing.T) {
		wb := NewWorkflow("test")

		// Add invalid nodes
		wb.AddNode(nil)                                // Error 1
		wb.AddNode(&WorkflowNode{Type: NodeTypeAgent}) // Error 2: no ID
		wb.AddAgentNode("", "agent", nil)              // Error 3: empty ID
		wb.AddToolNode("tool1", "", nil)               // Error 4: empty tool name

		// Try to add duplicate after a valid one
		wb.AddAgentNode("dup", "agent", nil)
		wb.AddAgentNode("dup", "agent", nil) // Error 5: duplicate

		// Add edges with invalid nodes
		wb.AddEdge("", "node1")                         // Error 6: empty from
		wb.AddEdge("node1", "")                         // Error 7: empty to
		wb.AddConditionalEdge("node1", "node2", "")     // Error 8: empty condition

		// Try to set dependencies on non-existent node
		wb.WithDependency("nonexistent", "node1") // Error 9

		workflow, err := wb.Build()

		assert.Nil(t, workflow)
		require.Error(t, err)

		// Check that we have multiple errors
		assert.GreaterOrEqual(t, len(wb.errors), 5, "should have accumulated multiple errors")

		// Error message should mention multiple errors
		assert.Contains(t, err.Error(), "error(s)")
	})

	t.Run("build-time validation errors", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddAgentNode("node2", "agent2", nil)

		// Add edge to non-existent node
		wb.AddEdge("node1", "nonexistent")

		// Add dependency to non-existent node
		wb.WithDependency("node2", "nonexistent2")

		workflow, err := wb.Build()

		assert.Nil(t, workflow)
		require.Error(t, err)

		// Should have at least 2 errors from Build validation
		assert.GreaterOrEqual(t, len(wb.errors), 2)

		errStr := err.Error()
		assert.Contains(t, errStr, "nonexistent")
		assert.Contains(t, errStr, "nonexistent2")
	})

	t.Run("errors from various stages", func(t *testing.T) {
		wb := NewWorkflow("test")

		// Stage 1: Node addition errors
		wb.AddNode(nil)
		wb.AddAgentNode("", "agent", nil)

		// Stage 2: Edge errors
		wb.AddEdge("", "node1")

		// Stage 3: Valid nodes for cycle creation
		wb.AddAgentNode("a", "agent-a", nil)
		wb.AddAgentNode("b", "agent-b", nil)
		wb.AddEdge("a", "b")
		wb.AddEdge("b", "a") // Creates cycle

		workflow, err := wb.Build()

		assert.Nil(t, workflow)
		require.Error(t, err)

		// Should have errors from all stages
		assert.GreaterOrEqual(t, len(wb.errors), 3)
	})
}

// TestBuild_ComplexWorkflow tests building a complex valid workflow
func TestBuild_ComplexWorkflow(t *testing.T) {
	wb := NewWorkflow("complex-workflow")
	wb.WithDescription("A complex workflow with multiple patterns")

	// Create nodes
	wb.AddAgentNode("start", "start-agent", nil)
	wb.AddAgentNode("parallel1", "parallel-agent-1", nil)
	wb.AddAgentNode("parallel2", "parallel-agent-2", nil)
	wb.AddToolNode("tool1", "analysis-tool", map[string]any{"depth": 5})
	wb.AddPluginNode("plugin1", "data-processor", "transform", map[string]any{"format": "json"})
	wb.AddConditionNode("check", &NodeCondition{
		Expression:  "success == true",
		TrueBranch:  []string{"end"},
		FalseBranch: []string{"retry"},
	})
	wb.AddAgentNode("retry", "retry-agent", nil)
	wb.AddAgentNode("end", "end-agent", nil)

	// Create edges
	wb.AddEdge("start", "parallel1")
	wb.AddEdge("start", "parallel2")
	wb.AddEdge("parallel1", "tool1")
	wb.AddEdge("parallel2", "plugin1")
	wb.AddEdge("tool1", "check")
	wb.AddEdge("plugin1", "check")
	wb.AddConditionalEdge("check", "end", "success == true")
	wb.AddConditionalEdge("check", "retry", "success == false")

	workflow, err := wb.Build()

	require.NoError(t, err)
	require.NotNil(t, workflow)

	assert.Equal(t, "complex-workflow", workflow.Name)
	assert.Equal(t, "A complex workflow with multiple patterns", workflow.Description)
	assert.Len(t, workflow.Nodes, 8)
	assert.Len(t, workflow.Edges, 8)

	// Verify entry point
	assert.Contains(t, workflow.EntryPoints, "start")

	// Verify different node types exist
	assert.Equal(t, NodeTypeAgent, workflow.Nodes["start"].Type)
	assert.Equal(t, NodeTypeTool, workflow.Nodes["tool1"].Type)
	assert.Equal(t, NodeTypePlugin, workflow.Nodes["plugin1"].Type)
	assert.Equal(t, NodeTypeCondition, workflow.Nodes["check"].Type)
}

// TestBuild_EdgeAndDependencyCombination tests using both edges and dependencies
func TestBuild_EdgeAndDependencyCombination(t *testing.T) {
	t.Run("mixed edges and dependencies", func(t *testing.T) {
		wb := NewWorkflow("test")
		wb.AddAgentNode("node1", "agent1", nil)
		wb.AddAgentNode("node2", "agent2", nil)
		wb.AddAgentNode("node3", "agent3", nil)
		wb.AddAgentNode("node4", "agent4", nil)

		// Use edges for some connections
		wb.AddEdge("node1", "node2")

		// Use dependencies for others
		wb.WithDependency("node3", "node1")
		wb.WithDependency("node4", "node2", "node3")

		workflow, err := wb.Build()

		require.NoError(t, err)
		assert.NotNil(t, workflow)

		// Verify edges
		assert.Len(t, workflow.Edges, 1)

		// Verify dependencies
		assert.Nil(t, workflow.Nodes["node1"].Dependencies)
		assert.Nil(t, workflow.Nodes["node2"].Dependencies)
		assert.Equal(t, []string{"node1"}, workflow.Nodes["node3"].Dependencies)
		assert.Equal(t, []string{"node2", "node3"}, workflow.Nodes["node4"].Dependencies)
	})
}

// TestBuilder_Integration tests the complete builder workflow
func TestBuilder_Integration(t *testing.T) {
	t.Run("complete workflow lifecycle", func(t *testing.T) {
		// Build a workflow using the fluent API
		workflow, err := NewWorkflow("integration-test").
			WithDescription("Integration test workflow").
			AddAgentNode("fetch", "data-fetcher", &agent.Task{Description: "Fetch data"}).
			AddToolNode("validate", "validator", map[string]any{"strict": true}).
			AddPluginNode("process", "processor", "execute", nil).
			AddConditionNode("check-result", &NodeCondition{
				Expression:  "valid == true",
				TrueBranch:  []string{"save"},
				FalseBranch: []string{"alert"},
			}).
			AddAgentNode("save", "data-saver", nil).
			AddAgentNode("alert", "alerter", nil).
			AddEdge("fetch", "validate").
			AddEdge("validate", "process").
			AddEdge("process", "check-result").
			AddConditionalEdge("check-result", "save", "valid == true").
			AddConditionalEdge("check-result", "alert", "valid == false").
			Build()

		require.NoError(t, err)
		require.NotNil(t, workflow)

		// Verify structure
		assert.Equal(t, "integration-test", workflow.Name)
		assert.Equal(t, "Integration test workflow", workflow.Description)
		assert.Len(t, workflow.Nodes, 6)
		assert.Len(t, workflow.Edges, 5)

		// Verify entry and exit points
		assert.Equal(t, []string{"fetch"}, workflow.EntryPoints)
		assert.Len(t, workflow.ExitPoints, 2)
		assert.Contains(t, workflow.ExitPoints, "save")
		assert.Contains(t, workflow.ExitPoints, "alert")

		// Verify node details
		fetchNode := workflow.Nodes["fetch"]
		assert.Equal(t, "data-fetcher", fetchNode.AgentName)
		assert.NotNil(t, fetchNode.AgentTask)

		validateNode := workflow.Nodes["validate"]
		assert.Equal(t, "validator", validateNode.ToolName)
		assert.Equal(t, true, validateNode.ToolInput["strict"])

		processNode := workflow.Nodes["process"]
		assert.Equal(t, "processor", processNode.PluginName)
		assert.Equal(t, "execute", processNode.PluginMethod)

		checkNode := workflow.Nodes["check-result"]
		assert.Equal(t, "valid == true", checkNode.Condition.Expression)

		// Verify edges
		hasEdge := func(from, to, condition string) bool {
			for _, edge := range workflow.Edges {
				if edge.From == from && edge.To == to && edge.Condition == condition {
					return true
				}
			}
			return false
		}

		assert.True(t, hasEdge("fetch", "validate", ""))
		assert.True(t, hasEdge("validate", "process", ""))
		assert.True(t, hasEdge("process", "check-result", ""))
		assert.True(t, hasEdge("check-result", "save", "valid == true"))
		assert.True(t, hasEdge("check-result", "alert", "valid == false"))
	})
}

// TestBuilder_ErrorRecovery tests that builder continues after errors
func TestBuilder_ErrorRecovery(t *testing.T) {
	t.Run("builder continues after errors", func(t *testing.T) {
		wb := NewWorkflow("test")

		// Add some errors
		wb.AddNode(nil)
		wb.AddAgentNode("", "agent", nil)

		// Continue adding valid nodes
		wb.AddAgentNode("valid1", "agent1", nil)
		wb.AddAgentNode("valid2", "agent2", nil)
		wb.AddEdge("valid1", "valid2")

		// Try to build
		workflow, err := wb.Build()

		// Should fail due to earlier errors
		assert.Nil(t, workflow)
		require.Error(t, err)

		// But valid nodes should still be in the workflow structure
		assert.Contains(t, wb.workflow.Nodes, "valid1")
		assert.Contains(t, wb.workflow.Nodes, "valid2")
		assert.Len(t, wb.workflow.Edges, 1)
	})
}

// TestBuilder_EmptyStrings tests handling of empty strings in various contexts
func TestBuilder_EmptyStrings(t *testing.T) {
	tests := []struct {
		name        string
		buildFunc   func(*WorkflowBuilder)
		errorSubstr string
	}{
		{
			name: "empty agent node ID",
			buildFunc: func(wb *WorkflowBuilder) {
				wb.AddAgentNode("", "agent", nil)
			},
			errorSubstr: "agent node must have an ID",
		},
		{
			name: "empty agent name",
			buildFunc: func(wb *WorkflowBuilder) {
				wb.AddAgentNode("node1", "", nil)
			},
			errorSubstr: "must have an agent name",
		},
		{
			name: "empty tool node ID",
			buildFunc: func(wb *WorkflowBuilder) {
				wb.AddToolNode("", "tool", nil)
			},
			errorSubstr: "tool node must have an ID",
		},
		{
			name: "empty tool name",
			buildFunc: func(wb *WorkflowBuilder) {
				wb.AddToolNode("node1", "", nil)
			},
			errorSubstr: "must have a tool name",
		},
		{
			name: "empty plugin node ID",
			buildFunc: func(wb *WorkflowBuilder) {
				wb.AddPluginNode("", "plugin", "method", nil)
			},
			errorSubstr: "plugin node must have an ID",
		},
		{
			name: "empty plugin name",
			buildFunc: func(wb *WorkflowBuilder) {
				wb.AddPluginNode("node1", "", "method", nil)
			},
			errorSubstr: "must have a plugin name",
		},
		{
			name: "empty plugin method",
			buildFunc: func(wb *WorkflowBuilder) {
				wb.AddPluginNode("node1", "plugin", "", nil)
			},
			errorSubstr: "must have a method",
		},
		{
			name: "empty condition node ID",
			buildFunc: func(wb *WorkflowBuilder) {
				wb.AddConditionNode("", &NodeCondition{Expression: "test"})
			},
			errorSubstr: "condition node must have an ID",
		},
		{
			name: "empty condition expression",
			buildFunc: func(wb *WorkflowBuilder) {
				wb.AddConditionNode("node1", &NodeCondition{Expression: ""})
			},
			errorSubstr: "must have a non-empty expression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wb := NewWorkflow("test")
			tt.buildFunc(wb)

			require.NotEmpty(t, wb.errors)
			assert.Contains(t, wb.errors[0].Error(), tt.errorSubstr)
		})
	}
}

// TestValidateDAG_EdgeCases tests edge cases in DAG validation
func TestValidateDAG_EdgeCases(t *testing.T) {
	t.Run("large linear chain", func(t *testing.T) {
		wb := NewWorkflow("test")

		// Create a chain of 100 nodes
		for i := 0; i < 100; i++ {
			nodeID := strings.ReplaceAll(strings.Repeat("node", 1), "node", "node") + string(rune('0'+i%10))
			if i < 10 {
				nodeID = "node" + string(rune('0'+i))
			} else {
				nodeID = "node" + string(rune('0'+i/10)) + string(rune('0'+i%10))
			}
			wb.AddAgentNode(nodeID, "agent", nil)

			if i > 0 {
				prevNodeID := "node" + string(rune('0'+(i-1)%10))
				if i > 1 && i < 11 {
					prevNodeID = "node" + string(rune('0'+(i-1)))
				} else if i >= 10 {
					prevNodeID = "node" + string(rune('0'+(i-1)/10)) + string(rune('0'+(i-1)%10))
				}
				wb.AddEdge(prevNodeID, nodeID)
			}
		}

		workflow, err := wb.Build()

		require.NoError(t, err)
		assert.NotNil(t, workflow)
	})

	t.Run("complex diamond pattern", func(t *testing.T) {
		wb := NewWorkflow("test")

		// Create a multi-level diamond
		wb.AddAgentNode("root", "agent", nil)
		wb.AddAgentNode("l1a", "agent", nil)
		wb.AddAgentNode("l1b", "agent", nil)
		wb.AddAgentNode("l2a", "agent", nil)
		wb.AddAgentNode("l2b", "agent", nil)
		wb.AddAgentNode("l2c", "agent", nil)
		wb.AddAgentNode("l2d", "agent", nil)
		wb.AddAgentNode("leaf", "agent", nil)

		// Root to level 1
		wb.AddEdge("root", "l1a")
		wb.AddEdge("root", "l1b")

		// Level 1 to level 2
		wb.AddEdge("l1a", "l2a")
		wb.AddEdge("l1a", "l2b")
		wb.AddEdge("l1b", "l2c")
		wb.AddEdge("l1b", "l2d")

		// Level 2 to leaf
		wb.AddEdge("l2a", "leaf")
		wb.AddEdge("l2b", "leaf")
		wb.AddEdge("l2c", "leaf")
		wb.AddEdge("l2d", "leaf")

		workflow, err := wb.Build()

		require.NoError(t, err)
		assert.NotNil(t, workflow)
		assert.Equal(t, []string{"root"}, workflow.EntryPoints)
		assert.Equal(t, []string{"leaf"}, workflow.ExitPoints)
	})
}
