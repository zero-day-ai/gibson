package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorkflowYAMLFromBytes_Valid(t *testing.T) {
	yamlData := []byte(`
name: Test Workflow
description: A test workflow
version: "1.0"

config:
  llm:
    default_provider: mock
    default_model: mock-model

target:
  seeds:
    - value: "test.example.com"
      type: hostname
      scope: in_scope

nodes:
  - id: test_agent
    type: agent
    name: Test Agent
    description: Simple agent for testing
    agent: test-agent
    timeout: 1m
    retry:
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      max_delay: 30s
      multiplier: 2.0
    task:
      goal: "Execute test task"
      context:
        test: true

  - id: test_tool
    type: tool
    name: Test Tool
    tool: test-tool
    depends_on:
      - test_agent
    timeout: 30s
    input:
      operation: "test"
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Check metadata
	assert.Equal(t, "Test Workflow", parsed.Name)
	assert.Equal(t, "A test workflow", parsed.Description)
	assert.Equal(t, "1.0", parsed.Version)

	// Check config
	assert.NotNil(t, parsed.Config)
	assert.Contains(t, parsed.Config, "llm")

	// Check nodes
	assert.Len(t, parsed.Nodes, 2)
	assert.Contains(t, parsed.Nodes, "test_agent")
	assert.Contains(t, parsed.Nodes, "test_tool")

	// Check agent node
	agentNode := parsed.Nodes["test_agent"]
	assert.Equal(t, "test_agent", agentNode.ID)
	assert.Equal(t, NodeTypeAgent, agentNode.Type)
	assert.Equal(t, "Test Agent", agentNode.Name)
	assert.Equal(t, "test-agent", agentNode.AgentName)
	assert.Equal(t, time.Minute, agentNode.Timeout)
	assert.NotNil(t, agentNode.RetryPolicy)
	assert.Equal(t, 3, agentNode.RetryPolicy.MaxRetries)
	assert.Equal(t, BackoffExponential, agentNode.RetryPolicy.BackoffStrategy)
	assert.NotNil(t, agentNode.AgentTask)

	// Check tool node
	toolNode := parsed.Nodes["test_tool"]
	assert.Equal(t, "test_tool", toolNode.ID)
	assert.Equal(t, NodeTypeTool, toolNode.Type)
	assert.Equal(t, "Test Tool", toolNode.Name)
	assert.Equal(t, "test-tool", toolNode.ToolName)
	assert.Equal(t, 30*time.Second, toolNode.Timeout)
	assert.Contains(t, toolNode.Dependencies, "test_agent")

	// Check edges
	assert.Len(t, parsed.Edges, 1)
	assert.Equal(t, "test_agent", parsed.Edges[0].From)
	assert.Equal(t, "test_tool", parsed.Edges[0].To)

	// Check entry and exit points
	assert.Len(t, parsed.EntryPoints, 1)
	assert.Contains(t, parsed.EntryPoints, "test_agent")
	assert.Len(t, parsed.ExitPoints, 1)
	assert.Contains(t, parsed.ExitPoints, "test_tool")
}

func TestParseWorkflowYAMLFromBytes_MinimalValid(t *testing.T) {
	yamlData := []byte(`
name: Minimal Workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	assert.Equal(t, "Minimal Workflow", parsed.Name)
	assert.Len(t, parsed.Nodes, 1)
	assert.Contains(t, parsed.Nodes, "node1")
}

func TestParseWorkflowYAMLFromBytes_MissingName(t *testing.T) {
	yamlData := []byte(`
nodes:
  - id: node1
    type: agent
    agent: test-agent
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "name")
}

func TestParseWorkflowYAMLFromBytes_NoNodes(t *testing.T) {
	yamlData := []byte(`
name: Empty Workflow
nodes: []
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "at least one node")
}

func TestParseWorkflowYAMLFromBytes_DuplicateNodeID(t *testing.T) {
	yamlData := []byte(`
name: Duplicate IDs
nodes:
  - id: node1
    type: agent
    agent: test-agent
  - id: node1
    type: tool
    tool: test-tool
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "duplicate")
	assert.Contains(t, parseErr.Message, "node1")
}

func TestParseWorkflowYAMLFromBytes_MissingNodeID(t *testing.T) {
	yamlData := []byte(`
name: Missing ID
nodes:
  - type: agent
    agent: test-agent
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "id")
}

func TestParseWorkflowYAMLFromBytes_InvalidNodeType(t *testing.T) {
	yamlData := []byte(`
name: Invalid Type
nodes:
  - id: node1
    type: invalid_type
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "invalid node type")
	assert.Equal(t, "node1", parseErr.NodeID)
}

func TestParseWorkflowYAMLFromBytes_InvalidDependency(t *testing.T) {
	yamlData := []byte(`
name: Invalid Dependency
nodes:
  - id: node1
    type: agent
    agent: test-agent
    depends_on:
      - nonexistent_node
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "non-existent")
	assert.Contains(t, parseErr.Message, "nonexistent_node")
}

func TestParseWorkflowYAMLFromBytes_InvalidTimeout(t *testing.T) {
	yamlData := []byte(`
name: Invalid Timeout
nodes:
  - id: node1
    type: agent
    agent: test-agent
    timeout: not-a-duration
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "timeout")
	assert.Equal(t, "node1", parseErr.NodeID)
}

func TestParseWorkflowYAMLFromBytes_AgentNode(t *testing.T) {
	yamlData := []byte(`
name: Agent Test
nodes:
  - id: agent1
    type: agent
    name: Test Agent
    agent: test-agent
    task:
      goal: "Test goal"
      priority: 5
      tags:
        - tag1
        - tag2
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	require.NoError(t, err)

	node := parsed.Nodes["agent1"]
	require.NotNil(t, node)
	assert.Equal(t, NodeTypeAgent, node.Type)
	assert.Equal(t, "test-agent", node.AgentName)
	require.NotNil(t, node.AgentTask)
	assert.Equal(t, 5, node.AgentTask.Priority)
	assert.Contains(t, node.AgentTask.Tags, "tag1")
	assert.Contains(t, node.AgentTask.Tags, "tag2")
}

func TestParseWorkflowYAMLFromBytes_AgentMissingName(t *testing.T) {
	yamlData := []byte(`
name: Agent Missing Name
nodes:
  - id: agent1
    type: agent
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "agent")
}

func TestParseWorkflowYAMLFromBytes_ToolNode(t *testing.T) {
	yamlData := []byte(`
name: Tool Test
nodes:
  - id: tool1
    type: tool
    name: Test Tool
    tool: test-tool
    input:
      param1: value1
      param2: 42
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	require.NoError(t, err)

	node := parsed.Nodes["tool1"]
	require.NotNil(t, node)
	assert.Equal(t, NodeTypeTool, node.Type)
	assert.Equal(t, "test-tool", node.ToolName)
	require.NotNil(t, node.ToolInput)
	assert.Equal(t, "value1", node.ToolInput["param1"])
	assert.Equal(t, 42, node.ToolInput["param2"])
}

func TestParseWorkflowYAMLFromBytes_PluginNode(t *testing.T) {
	yamlData := []byte(`
name: Plugin Test
nodes:
  - id: plugin1
    type: plugin
    name: Test Plugin
    plugin: test-plugin
    method: execute
    params:
      key: value
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	require.NoError(t, err)

	node := parsed.Nodes["plugin1"]
	require.NotNil(t, node)
	assert.Equal(t, NodeTypePlugin, node.Type)
	assert.Equal(t, "test-plugin", node.PluginName)
	assert.Equal(t, "execute", node.PluginMethod)
	require.NotNil(t, node.PluginParams)
	assert.Equal(t, "value", node.PluginParams["key"])
}

func TestParseWorkflowYAMLFromBytes_PluginMissingMethod(t *testing.T) {
	yamlData := []byte(`
name: Plugin Missing Method
nodes:
  - id: plugin1
    type: plugin
    plugin: test-plugin
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "method")
}

func TestParseWorkflowYAMLFromBytes_ConditionNode(t *testing.T) {
	yamlData := []byte(`
name: Condition Test
nodes:
  - id: cond1
    type: condition
    condition:
      expression: "result.status == 'success'"
      true_branch:
        - node2
      false_branch:
        - node3
  - id: node2
    type: agent
    agent: success-agent
  - id: node3
    type: agent
    agent: failure-agent
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	require.NoError(t, err)

	node := parsed.Nodes["cond1"]
	require.NotNil(t, node)
	assert.Equal(t, NodeTypeCondition, node.Type)
	require.NotNil(t, node.Condition)
	assert.Equal(t, "result.status == 'success'", node.Condition.Expression)
	assert.Contains(t, node.Condition.TrueBranch, "node2")
	assert.Contains(t, node.Condition.FalseBranch, "node3")
}

func TestParseWorkflowYAMLFromBytes_ConditionMissingExpression(t *testing.T) {
	yamlData := []byte(`
name: Condition Missing Expression
nodes:
  - id: cond1
    type: condition
    condition:
      true_branch:
        - node2
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "expression")
}

func TestParseWorkflowYAMLFromBytes_ParallelNode(t *testing.T) {
	yamlData := []byte(`
name: Parallel Test
nodes:
  - id: parallel1
    type: parallel
    sub_nodes:
      - id: sub1
        type: agent
        agent: agent1
      - id: sub2
        type: tool
        tool: tool1
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	require.NoError(t, err)

	node := parsed.Nodes["parallel1"]
	require.NotNil(t, node)
	assert.Equal(t, NodeTypeParallel, node.Type)
	require.Len(t, node.SubNodes, 2)
	assert.Equal(t, "sub1", node.SubNodes[0].ID)
	assert.Equal(t, "sub2", node.SubNodes[1].ID)
}

func TestParseWorkflowYAMLFromBytes_ParallelNoSubNodes(t *testing.T) {
	yamlData := []byte(`
name: Parallel No SubNodes
nodes:
  - id: parallel1
    type: parallel
    sub_nodes: []
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "sub_node")
}

func TestParseWorkflowYAMLFromBytes_JoinNode(t *testing.T) {
	yamlData := []byte(`
name: Join Test
nodes:
  - id: node1
    type: agent
    agent: agent1
  - id: node2
    type: agent
    agent: agent2
  - id: join1
    type: join
    depends_on:
      - node1
      - node2
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	require.NoError(t, err)

	node := parsed.Nodes["join1"]
	require.NotNil(t, node)
	assert.Equal(t, NodeTypeJoin, node.Type)
	assert.Len(t, node.Dependencies, 2)
}

func TestParseWorkflowYAMLFromBytes_RetryPolicy(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		expectError bool
		validate    func(t *testing.T, policy *RetryPolicy)
	}{
		{
			name: "constant backoff",
			yaml: `
name: Retry Test
nodes:
  - id: node1
    type: agent
    agent: test-agent
    retry:
      max_retries: 3
      backoff: constant
      initial_delay: 1s
`,
			expectError: false,
			validate: func(t *testing.T, policy *RetryPolicy) {
				assert.Equal(t, 3, policy.MaxRetries)
				assert.Equal(t, BackoffConstant, policy.BackoffStrategy)
				assert.Equal(t, time.Second, policy.InitialDelay)
			},
		},
		{
			name: "linear backoff",
			yaml: `
name: Retry Test
nodes:
  - id: node1
    type: agent
    agent: test-agent
    retry:
      max_retries: 5
      backoff: linear
      initial_delay: 2s
`,
			expectError: false,
			validate: func(t *testing.T, policy *RetryPolicy) {
				assert.Equal(t, 5, policy.MaxRetries)
				assert.Equal(t, BackoffLinear, policy.BackoffStrategy)
				assert.Equal(t, 2*time.Second, policy.InitialDelay)
			},
		},
		{
			name: "exponential backoff",
			yaml: `
name: Retry Test
nodes:
  - id: node1
    type: agent
    agent: test-agent
    retry:
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      max_delay: 30s
      multiplier: 2.0
`,
			expectError: false,
			validate: func(t *testing.T, policy *RetryPolicy) {
				assert.Equal(t, 3, policy.MaxRetries)
				assert.Equal(t, BackoffExponential, policy.BackoffStrategy)
				assert.Equal(t, time.Second, policy.InitialDelay)
				assert.Equal(t, 30*time.Second, policy.MaxDelay)
				assert.Equal(t, 2.0, policy.Multiplier)
			},
		},
		{
			name: "invalid backoff strategy",
			yaml: `
name: Retry Test
nodes:
  - id: node1
    type: agent
    agent: test-agent
    retry:
      max_retries: 3
      backoff: invalid
      initial_delay: 1s
`,
			expectError: true,
		},
		{
			name: "negative max retries",
			yaml: `
name: Retry Test
nodes:
  - id: node1
    type: agent
    agent: test-agent
    retry:
      max_retries: -1
      backoff: constant
      initial_delay: 1s
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseWorkflowYAMLFromBytes([]byte(tt.yaml))

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, parsed)
			} else {
				require.NoError(t, err)
				require.NotNil(t, parsed)

				node := parsed.Nodes["node1"]
				require.NotNil(t, node)
				require.NotNil(t, node.RetryPolicy)

				if tt.validate != nil {
					tt.validate(t, node.RetryPolicy)
				}
			}
		})
	}
}

func TestParseWorkflowYAML_File(t *testing.T) {
	// Create a temporary workflow file
	tmpDir := t.TempDir()
	workflowPath := filepath.Join(tmpDir, "test-workflow.yaml")

	yamlContent := []byte(`
name: File Test Workflow
description: Testing file-based parsing
nodes:
  - id: node1
    type: agent
    agent: test-agent
`)

	err := os.WriteFile(workflowPath, yamlContent, 0644)
	require.NoError(t, err)

	// Parse the file
	parsed, err := ParseWorkflowYAML(workflowPath)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	assert.Equal(t, "File Test Workflow", parsed.Name)
	assert.Equal(t, workflowPath, parsed.SourceFile)
	assert.Len(t, parsed.Nodes, 1)
}

func TestParseWorkflowYAML_FileNotFound(t *testing.T) {
	parsed, err := ParseWorkflowYAML("/nonexistent/workflow.yaml")
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
	assert.Contains(t, parseErr.Message, "failed to read")
}

func TestParseWorkflowYAML_InvalidYAML(t *testing.T) {
	yamlData := []byte(`
name: Invalid YAML
nodes:
  - id: node1
    type: agent
    invalid yaml here [[[
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	assert.Error(t, err)
	assert.Nil(t, parsed)

	var parseErr *ParseError
	require.True(t, errors.As(err, &parseErr))
}

func TestParseError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ParseError
		expected string
	}{
		{
			name: "with line and node",
			err: &ParseError{
				Message: "invalid field",
				Line:    10,
				Column:  5,
				NodeID:  "node1",
			},
			expected: "parse error at line 10:5 (node node1): invalid field",
		},
		{
			name: "with line only",
			err: &ParseError{
				Message: "missing field",
				Line:    15,
				Column:  3,
			},
			expected: "parse error at line 15:3: missing field",
		},
		{
			name: "without line",
			err: &ParseError{
				Message: "general error",
			},
			expected: "parse error: general error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestParseError_Unwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	parseErr := &ParseError{
		Message: "outer error",
		Err:     innerErr,
	}

	assert.Equal(t, innerErr, parseErr.Unwrap())
}

func TestToWorkflow(t *testing.T) {
	yamlData := []byte(`
name: Test Workflow
description: Test description
nodes:
  - id: node1
    type: agent
    agent: test-agent
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	require.NoError(t, err)

	workflow := parsed.ToWorkflow()
	require.NotNil(t, workflow)

	assert.Equal(t, parsed.Name, workflow.Name)
	assert.Equal(t, parsed.Description, workflow.Description)
	assert.Equal(t, parsed.TargetRef, workflow.TargetRef)
	assert.Equal(t, parsed.Nodes, workflow.Nodes)
	assert.Equal(t, parsed.Edges, workflow.Edges)
	assert.Equal(t, parsed.EntryPoints, workflow.EntryPoints)
	assert.Equal(t, parsed.ExitPoints, workflow.ExitPoints)
	assert.NotEmpty(t, workflow.ID)
}

func TestParseWorkflowYAMLFromBytes_ComplexDAG(t *testing.T) {
	yamlData := []byte(`
name: Complex DAG
description: Testing complex dependency graph
nodes:
  - id: start
    type: agent
    agent: start-agent

  - id: parallel1
    type: agent
    agent: agent1
    depends_on:
      - start

  - id: parallel2
    type: agent
    agent: agent2
    depends_on:
      - start

  - id: join
    type: join
    depends_on:
      - parallel1
      - parallel2

  - id: end
    type: agent
    agent: end-agent
    depends_on:
      - join
`)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Check nodes
	assert.Len(t, parsed.Nodes, 5)

	// Check edges
	assert.Len(t, parsed.Edges, 5)

	// Check entry points (should be "start")
	assert.Len(t, parsed.EntryPoints, 1)
	assert.Contains(t, parsed.EntryPoints, "start")

	// Check exit points (should be "end")
	assert.Len(t, parsed.ExitPoints, 1)
	assert.Contains(t, parsed.ExitPoints, "end")

	// Verify join node has correct dependencies
	joinNode := parsed.Nodes["join"]
	require.NotNil(t, joinNode)
	assert.Contains(t, joinNode.Dependencies, "parallel1")
	assert.Contains(t, joinNode.Dependencies, "parallel2")
}

func TestParseWorkflowYAMLFromBytes_RealWorldExample(t *testing.T) {
	// Parse the actual test data file
	yamlData, err := os.ReadFile("../daemon/testdata/simple-workflow.yaml")
	require.NoError(t, err)

	parsed, err := ParseWorkflowYAMLFromBytes(yamlData)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Verify basic structure
	assert.Equal(t, "Simple Test Workflow", parsed.Name)
	assert.NotEmpty(t, parsed.Description)
	assert.Len(t, parsed.Nodes, 2)

	// Verify nodes exist
	assert.Contains(t, parsed.Nodes, "test_agent")
	assert.Contains(t, parsed.Nodes, "test_tool")

	// Verify edge
	assert.Len(t, parsed.Edges, 1)
	assert.Equal(t, "test_agent", parsed.Edges[0].From)
	assert.Equal(t, "test_tool", parsed.Edges[0].To)
}
