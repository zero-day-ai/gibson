package planning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// TestExtractBounds_BasicWorkflow tests bounds extraction from a simple workflow
func TestExtractBounds_BasicWorkflow(t *testing.T) {
	wf := &workflow.Workflow{
		ID:          types.NewID(),
		Name:        "Test Workflow",
		Description: "Test workflow for bounds extraction",
		Nodes: map[string]*workflow.WorkflowNode{
			"node1": {
				ID:        "node1",
				Type:      workflow.NodeTypeAgent,
				Name:      "Agent Node",
				AgentName: "test-agent",
				Timeout:   5 * time.Minute,
				RetryPolicy: &workflow.RetryPolicy{
					MaxRetries: 3,
				},
			},
			"node2": {
				ID:       "node2",
				Type:     workflow.NodeTypeTool,
				Name:     "Tool Node",
				ToolName: "test-tool",
				Timeout:  2 * time.Minute,
			},
			"node3": {
				ID:         "node3",
				Type:       workflow.NodeTypePlugin,
				Name:       "Plugin Node",
				PluginName: "test-plugin",
				Timeout:    1 * time.Minute,
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)
	require.NotNil(t, bounds)

	// Verify allowed components
	assert.Contains(t, bounds.AllowedAgents, "test-agent")
	assert.Contains(t, bounds.AllowedTools, "test-tool")
	assert.Contains(t, bounds.AllowedPlugins, "test-plugin")

	// Verify node set
	assert.True(t, bounds.ContainsNode("node1"))
	assert.True(t, bounds.ContainsNode("node2"))
	assert.True(t, bounds.ContainsNode("node3"))
	assert.False(t, bounds.ContainsNode("nonexistent"))

	// Verify max duration
	assert.Equal(t, 8*time.Minute, bounds.MaxDuration)

	// Verify default max parallel
	assert.Equal(t, 1, bounds.MaxParallel)

	// Verify node constraints
	node1Bounds := bounds.GetNodeBounds("node1")
	require.NotNil(t, node1Bounds)
	assert.Equal(t, 3, node1Bounds.MaxRetries)
	assert.Equal(t, 5*time.Minute, node1Bounds.Timeout)
	assert.Equal(t, workflow.NodeTypeAgent, node1Bounds.NodeType)

	node2Bounds := bounds.GetNodeBounds("node2")
	require.NotNil(t, node2Bounds)
	assert.Equal(t, 0, node2Bounds.MaxRetries)
	assert.Equal(t, 2*time.Minute, node2Bounds.Timeout)
	assert.Equal(t, []string{"test-tool"}, node2Bounds.RequiredTools)
}

// TestExtractBounds_ParallelNodes tests bounds extraction with parallel execution
func TestExtractBounds_ParallelNodes(t *testing.T) {
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "Parallel Workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"parallel1": {
				ID:   "parallel1",
				Type: workflow.NodeTypeParallel,
				Name: "Parallel Execution",
				SubNodes: []*workflow.WorkflowNode{
					{
						ID:        "sub1",
						Type:      workflow.NodeTypeAgent,
						AgentName: "agent1",
						Timeout:   3 * time.Minute,
					},
					{
						ID:        "sub2",
						Type:      workflow.NodeTypeAgent,
						AgentName: "agent2",
						Timeout:   4 * time.Minute,
					},
					{
						ID:       "sub3",
						Type:     workflow.NodeTypeTool,
						ToolName: "tool1",
						Timeout:  2 * time.Minute,
					},
				},
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)
	require.NotNil(t, bounds)

	// Verify max parallelism
	assert.Equal(t, 3, bounds.MaxParallel)

	// Verify sub-nodes are in allowed set
	assert.True(t, bounds.ContainsNode("parallel1"))
	assert.True(t, bounds.ContainsNode("sub1"))
	assert.True(t, bounds.ContainsNode("sub2"))
	assert.True(t, bounds.ContainsNode("sub3"))

	// Verify components from sub-nodes
	assert.Contains(t, bounds.AllowedAgents, "agent1")
	assert.Contains(t, bounds.AllowedAgents, "agent2")
	assert.Contains(t, bounds.AllowedTools, "tool1")

	// Verify duration accumulation (3 + 4 + 2 = 9 minutes)
	assert.Equal(t, 9*time.Minute, bounds.MaxDuration)

	// Verify sub-node constraints
	sub3Bounds := bounds.GetNodeBounds("sub3")
	require.NotNil(t, sub3Bounds)
	assert.Equal(t, []string{"tool1"}, sub3Bounds.RequiredTools)
}

// TestExtractBounds_MultipleParallelNodes tests max parallelism calculation
func TestExtractBounds_MultipleParallelNodes(t *testing.T) {
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "Multiple Parallel Workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"parallel1": {
				ID:   "parallel1",
				Type: workflow.NodeTypeParallel,
				SubNodes: []*workflow.WorkflowNode{
					{ID: "sub1", Type: workflow.NodeTypeAgent, AgentName: "agent1"},
					{ID: "sub2", Type: workflow.NodeTypeAgent, AgentName: "agent2"},
				},
			},
			"parallel2": {
				ID:   "parallel2",
				Type: workflow.NodeTypeParallel,
				SubNodes: []*workflow.WorkflowNode{
					{ID: "sub3", Type: workflow.NodeTypeAgent, AgentName: "agent3"},
					{ID: "sub4", Type: workflow.NodeTypeAgent, AgentName: "agent4"},
					{ID: "sub5", Type: workflow.NodeTypeAgent, AgentName: "agent5"},
					{ID: "sub6", Type: workflow.NodeTypeAgent, AgentName: "agent6"},
				},
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	// Should use the maximum parallelism (4, not 2)
	assert.Equal(t, 4, bounds.MaxParallel)
}

// TestExtractBounds_ApprovalGates tests approval gate detection
func TestExtractBounds_ApprovalGates(t *testing.T) {
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "Approval Workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"node1": {
				ID:   "node1",
				Type: workflow.NodeTypeAgent,
			},
			"node2": {
				ID:   "node2",
				Type: workflow.NodeTypeAgent,
				Metadata: map[string]any{
					"requires_approval": true,
				},
			},
			"node3": {
				ID:   "node3",
				Type: workflow.NodeTypeAgent,
				Metadata: map[string]any{
					"requires_approval": false,
				},
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	// Only node2 should be in approval gates
	assert.Len(t, bounds.ApprovalGates, 1)
	assert.Contains(t, bounds.ApprovalGates, "node2")
	assert.True(t, bounds.RequiresApproval("node2"))
	assert.False(t, bounds.RequiresApproval("node1"))
	assert.False(t, bounds.RequiresApproval("node3"))
}

// TestExtractBounds_DuplicateComponents tests deduplication of component names
func TestExtractBounds_DuplicateComponents(t *testing.T) {
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "Duplicate Components",
		Nodes: map[string]*workflow.WorkflowNode{
			"node1": {
				ID:        "node1",
				Type:      workflow.NodeTypeAgent,
				AgentName: "test-agent",
			},
			"node2": {
				ID:        "node2",
				Type:      workflow.NodeTypeAgent,
				AgentName: "test-agent", // Duplicate
			},
			"node3": {
				ID:       "node3",
				Type:     workflow.NodeTypeTool,
				ToolName: "test-tool",
			},
			"node4": {
				ID:       "node4",
				Type:     workflow.NodeTypeTool,
				ToolName: "test-tool", // Duplicate
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	// Should have unique entries only
	assert.Len(t, bounds.AllowedAgents, 1)
	assert.Len(t, bounds.AllowedTools, 1)
	assert.Contains(t, bounds.AllowedAgents, "test-agent")
	assert.Contains(t, bounds.AllowedTools, "test-tool")
}

// TestExtractBounds_AllNodeTypes tests extraction with all node types
func TestExtractBounds_AllNodeTypes(t *testing.T) {
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "All Node Types",
		Nodes: map[string]*workflow.WorkflowNode{
			"agent": {
				ID:        "agent",
				Type:      workflow.NodeTypeAgent,
				AgentName: "test-agent",
			},
			"tool": {
				ID:       "tool",
				Type:     workflow.NodeTypeTool,
				ToolName: "test-tool",
			},
			"plugin": {
				ID:         "plugin",
				Type:       workflow.NodeTypePlugin,
				PluginName: "test-plugin",
			},
			"condition": {
				ID:   "condition",
				Type: workflow.NodeTypeCondition,
				Condition: &workflow.NodeCondition{
					Expression:  "true",
					TrueBranch:  []string{"agent"},
					FalseBranch: []string{"tool"},
				},
			},
			"join": {
				ID:   "join",
				Type: workflow.NodeTypeJoin,
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	// All node types should be recognized
	assert.True(t, bounds.ContainsNode("agent"))
	assert.True(t, bounds.ContainsNode("tool"))
	assert.True(t, bounds.ContainsNode("plugin"))
	assert.True(t, bounds.ContainsNode("condition"))
	assert.True(t, bounds.ContainsNode("join"))

	// Verify node types in constraints
	assert.Equal(t, workflow.NodeTypeAgent, bounds.GetNodeBounds("agent").NodeType)
	assert.Equal(t, workflow.NodeTypeTool, bounds.GetNodeBounds("tool").NodeType)
	assert.Equal(t, workflow.NodeTypePlugin, bounds.GetNodeBounds("plugin").NodeType)
	assert.Equal(t, workflow.NodeTypeCondition, bounds.GetNodeBounds("condition").NodeType)
	assert.Equal(t, workflow.NodeTypeJoin, bounds.GetNodeBounds("join").NodeType)
}

// TestExtractBounds_NilWorkflow tests error handling for nil workflow
func TestExtractBounds_NilWorkflow(t *testing.T) {
	bounds, err := ExtractBounds(nil)
	assert.Error(t, err)
	assert.Nil(t, bounds)
	assert.Contains(t, err.Error(), "workflow cannot be nil")
}

// TestExtractBounds_EmptyWorkflow tests error handling for empty workflow
func TestExtractBounds_EmptyWorkflow(t *testing.T) {
	wf := &workflow.Workflow{
		ID:    types.NewID(),
		Name:  "Empty",
		Nodes: map[string]*workflow.WorkflowNode{},
	}

	bounds, err := ExtractBounds(wf)
	assert.Error(t, err)
	assert.Nil(t, bounds)
	assert.Contains(t, err.Error(), "must contain at least one node")
}

// TestExtractBounds_NilNodes tests handling of nil nodes
func TestExtractBounds_NilNodes(t *testing.T) {
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "Nil Nodes",
		Nodes: map[string]*workflow.WorkflowNode{
			"node1": {
				ID:        "node1",
				Type:      workflow.NodeTypeAgent,
				AgentName: "test-agent",
			},
			"node2": nil, // Nil node should be skipped
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)

	// Should extract non-nil node
	assert.True(t, bounds.ContainsNode("node1"))
	// Nil node should not cause an error but also not be included
	assert.False(t, bounds.ContainsNode("node2"))
}

// TestValidateStep_ValidStep tests validation of valid steps
func TestValidateStep_ValidStep(t *testing.T) {
	bounds := &PlanningBounds{
		AllowedNodes: map[string]bool{
			"node1": true,
			"node2": true,
		},
	}

	step := ExecutionStep{
		NodeID:     "node1",
		Parameters: map[string]any{"key": "value"},
	}

	err := bounds.ValidateStep(step)
	assert.NoError(t, err)
}

// TestValidateStep_InvalidStep tests validation of invalid steps
func TestValidateStep_InvalidStep(t *testing.T) {
	bounds := &PlanningBounds{
		AllowedNodes: map[string]bool{
			"node1": true,
			"node2": true,
		},
	}

	tests := []struct {
		name    string
		step    ExecutionStep
		wantErr string
	}{
		{
			name: "nonexistent node",
			step: ExecutionStep{
				NodeID: "nonexistent",
			},
			wantErr: "not in the workflow",
		},
		{
			name: "empty node ID",
			step: ExecutionStep{
				NodeID: "",
			},
			wantErr: "cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := bounds.ValidateStep(tt.step)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestValidateSteps_ValidSequence tests validation of valid step sequences
func TestValidateSteps_ValidSequence(t *testing.T) {
	bounds := &PlanningBounds{
		AllowedNodes: map[string]bool{
			"node1": true,
			"node2": true,
			"node3": true,
		},
	}

	steps := []ExecutionStep{
		{NodeID: "node1"},
		{NodeID: "node2"},
		{NodeID: "node3"},
	}

	err := bounds.ValidateSteps(steps)
	assert.NoError(t, err)
}

// TestValidateSteps_InvalidSequence tests validation of invalid step sequences
func TestValidateSteps_InvalidSequence(t *testing.T) {
	bounds := &PlanningBounds{
		AllowedNodes: map[string]bool{
			"node1": true,
			"node2": true,
		},
	}

	steps := []ExecutionStep{
		{NodeID: "node1"},
		{NodeID: "node2"},
		{NodeID: "invalid"}, // Invalid step
	}

	err := bounds.ValidateSteps(steps)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid step at position 2")
	assert.Contains(t, err.Error(), "not in the workflow")
}

// TestGetNodeBounds_ExistingNode tests retrieval of node bounds
func TestGetNodeBounds_ExistingNode(t *testing.T) {
	bounds := &PlanningBounds{
		NodeConstraints: map[string]NodeBounds{
			"node1": {
				MaxRetries: 5,
				Timeout:    10 * time.Minute,
				NodeType:   workflow.NodeTypeAgent,
			},
		},
	}

	nodeBounds := bounds.GetNodeBounds("node1")
	require.NotNil(t, nodeBounds)
	assert.Equal(t, 5, nodeBounds.MaxRetries)
	assert.Equal(t, 10*time.Minute, nodeBounds.Timeout)
	assert.Equal(t, workflow.NodeTypeAgent, nodeBounds.NodeType)
}

// TestGetNodeBounds_NonexistentNode tests retrieval of nonexistent node bounds
func TestGetNodeBounds_NonexistentNode(t *testing.T) {
	bounds := &PlanningBounds{
		NodeConstraints: map[string]NodeBounds{},
	}

	nodeBounds := bounds.GetNodeBounds("nonexistent")
	assert.Nil(t, nodeBounds)
}

// TestContainsNode tests node existence checking
func TestContainsNode(t *testing.T) {
	bounds := &PlanningBounds{
		AllowedNodes: map[string]bool{
			"node1": true,
			"node2": true,
			"node3": true,
		},
	}

	assert.True(t, bounds.ContainsNode("node1"))
	assert.True(t, bounds.ContainsNode("node2"))
	assert.True(t, bounds.ContainsNode("node3"))
	assert.False(t, bounds.ContainsNode("node4"))
	assert.False(t, bounds.ContainsNode(""))
}

// TestRequiresApproval tests approval requirement checking
func TestRequiresApproval(t *testing.T) {
	bounds := &PlanningBounds{
		ApprovalGates: []string{"node1", "node3"},
	}

	assert.True(t, bounds.RequiresApproval("node1"))
	assert.False(t, bounds.RequiresApproval("node2"))
	assert.True(t, bounds.RequiresApproval("node3"))
	assert.False(t, bounds.RequiresApproval(""))
}

// TestExtractBounds_ComplexWorkflow tests bounds extraction from a complex workflow
func TestExtractBounds_ComplexWorkflow(t *testing.T) {
	// Create a complex workflow with multiple node types, parallel execution, and various constraints
	wf := &workflow.Workflow{
		ID:          types.NewID(),
		Name:        "Complex Security Assessment",
		Description: "Multi-phase security assessment with parallel execution",
		Nodes: map[string]*workflow.WorkflowNode{
			"recon": {
				ID:        "recon",
				Type:      workflow.NodeTypeAgent,
				Name:      "Reconnaissance",
				AgentName: "recon-agent",
				Timeout:   5 * time.Minute,
				RetryPolicy: &workflow.RetryPolicy{
					MaxRetries: 2,
				},
				AgentTask: &agent.Task{
					ID:          types.NewID(),
					Name:        "Recon Task",
					Description: "Initial reconnaissance",
				},
			},
			"scan": {
				ID:   "scan",
				Type: workflow.NodeTypeParallel,
				Name: "Parallel Scanning",
				SubNodes: []*workflow.WorkflowNode{
					{
						ID:        "port_scan",
						Type:      workflow.NodeTypeAgent,
						AgentName: "port-scanner",
						Timeout:   3 * time.Minute,
						RetryPolicy: &workflow.RetryPolicy{
							MaxRetries: 1,
						},
					},
					{
						ID:       "vuln_scan",
						Type:     workflow.NodeTypeTool,
						ToolName: "nuclei",
						Timeout:  5 * time.Minute,
					},
					{
						ID:         "service_enum",
						Type:       workflow.NodeTypePlugin,
						PluginName: "service-detector",
						Timeout:    2 * time.Minute,
					},
				},
			},
			"analyze": {
				ID:        "analyze",
				Type:      workflow.NodeTypeAgent,
				AgentName: "analyzer",
				Timeout:   10 * time.Minute,
				Metadata: map[string]any{
					"requires_approval": true,
				},
			},
			"report": {
				ID:       "report",
				Type:     workflow.NodeTypeTool,
				ToolName: "report-generator",
				Timeout:  1 * time.Minute,
			},
		},
	}

	bounds, err := ExtractBounds(wf)
	require.NoError(t, err)
	require.NotNil(t, bounds)

	// Verify all agents
	assert.Len(t, bounds.AllowedAgents, 3)
	assert.Contains(t, bounds.AllowedAgents, "recon-agent")
	assert.Contains(t, bounds.AllowedAgents, "port-scanner")
	assert.Contains(t, bounds.AllowedAgents, "analyzer")

	// Verify all tools
	assert.Len(t, bounds.AllowedTools, 2)
	assert.Contains(t, bounds.AllowedTools, "nuclei")
	assert.Contains(t, bounds.AllowedTools, "report-generator")

	// Verify all plugins
	assert.Len(t, bounds.AllowedPlugins, 1)
	assert.Contains(t, bounds.AllowedPlugins, "service-detector")

	// Verify max parallelism (3 sub-nodes in scan)
	assert.Equal(t, 3, bounds.MaxParallel)

	// Verify total duration (5 + 3 + 5 + 2 + 10 + 1 = 26 minutes)
	assert.Equal(t, 26*time.Minute, bounds.MaxDuration)

	// Verify all nodes are in allowed set
	assert.True(t, bounds.ContainsNode("recon"))
	assert.True(t, bounds.ContainsNode("scan"))
	assert.True(t, bounds.ContainsNode("port_scan"))
	assert.True(t, bounds.ContainsNode("vuln_scan"))
	assert.True(t, bounds.ContainsNode("service_enum"))
	assert.True(t, bounds.ContainsNode("analyze"))
	assert.True(t, bounds.ContainsNode("report"))

	// Verify approval gates
	assert.Len(t, bounds.ApprovalGates, 1)
	assert.Contains(t, bounds.ApprovalGates, "analyze")
	assert.True(t, bounds.RequiresApproval("analyze"))

	// Verify node constraints
	reconBounds := bounds.GetNodeBounds("recon")
	require.NotNil(t, reconBounds)
	assert.Equal(t, 2, reconBounds.MaxRetries)
	assert.Equal(t, 5*time.Minute, reconBounds.Timeout)
	assert.Equal(t, workflow.NodeTypeAgent, reconBounds.NodeType)

	vulnScanBounds := bounds.GetNodeBounds("vuln_scan")
	require.NotNil(t, vulnScanBounds)
	assert.Equal(t, []string{"nuclei"}, vulnScanBounds.RequiredTools)
	assert.Equal(t, 5*time.Minute, vulnScanBounds.Timeout)
}
