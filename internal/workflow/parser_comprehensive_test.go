package workflow

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParser_ValidWorkflowFixture tests parsing the comprehensive valid workflow fixture
func TestParser_ValidWorkflowFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/valid-workflow.yaml")
	require.NoError(t, err, "should be able to read test fixture")

	parsed, err := ParseWorkflowYAMLFromBytes(data)
	require.NoError(t, err, "should parse valid workflow without error")
	require.NotNil(t, parsed, "parsed workflow should not be nil")

	// Verify workflow metadata
	assert.Equal(t, "Valid Security Assessment", parsed.Name)
	assert.Contains(t, parsed.Description, "comprehensive")
	assert.False(t, parsed.ParsedAt.IsZero())

	// Verify correct number of nodes (9 top-level nodes)
	assert.Len(t, parsed.Nodes, 9)

	// Verify specific nodes exist
	expectedNodes := []string{
		"recon", "parallel_scan", "join_scans", "analyze",
		"check_vulnerabilities", "exploit_assessment", "detailed_report",
		"standard_report", "notify_team",
	}
	for _, nodeID := range expectedNodes {
		assert.Contains(t, parsed.Nodes, nodeID, "should contain node %s", nodeID)
	}

	// Verify agent node with full retry policy
	reconNode := parsed.Nodes["recon"]
	require.NotNil(t, reconNode)
	assert.Equal(t, NodeTypeAgent, reconNode.Type)
	assert.Equal(t, "recon-agent", reconNode.AgentName)
	assert.Equal(t, 5*time.Minute, reconNode.Timeout)

	require.NotNil(t, reconNode.RetryPolicy)
	assert.Equal(t, 3, reconNode.RetryPolicy.MaxRetries)
	assert.Equal(t, BackoffExponential, reconNode.RetryPolicy.BackoffStrategy)
	assert.Equal(t, time.Second, reconNode.RetryPolicy.InitialDelay)
	assert.Equal(t, 30*time.Second, reconNode.RetryPolicy.MaxDelay)
	assert.Equal(t, 2.0, reconNode.RetryPolicy.Multiplier)

	// Verify parallel node structure
	parallelNode := parsed.Nodes["parallel_scan"]
	require.NotNil(t, parallelNode)
	assert.Equal(t, NodeTypeParallel, parallelNode.Type)
	assert.Len(t, parallelNode.SubNodes, 2)
	assert.Equal(t, "scan_common_ports", parallelNode.SubNodes[0].ID)
	assert.Equal(t, "scan_full_range", parallelNode.SubNodes[1].ID)

	// Verify condition node
	condNode := parsed.Nodes["check_vulnerabilities"]
	require.NotNil(t, condNode)
	assert.Equal(t, NodeTypeCondition, condNode.Type)
	require.NotNil(t, condNode.Condition)
	assert.Contains(t, condNode.Condition.Expression, "result.vulnerabilities.count > 0")
	assert.Equal(t, []string{"exploit_assessment"}, condNode.Condition.TrueBranch)
	assert.Equal(t, []string{"standard_report"}, condNode.Condition.FalseBranch)

	// Verify tool node
	analyzeNode := parsed.Nodes["analyze"]
	require.NotNil(t, analyzeNode)
	assert.Equal(t, NodeTypeTool, analyzeNode.Type)
	assert.Equal(t, "vulnerability-analyzer", analyzeNode.ToolName)
	assert.NotNil(t, analyzeNode.ToolInput)

	// Verify plugin node
	reportNode := parsed.Nodes["detailed_report"]
	require.NotNil(t, reportNode)
	assert.Equal(t, NodeTypePlugin, reportNode.Type)
	assert.Equal(t, "report-generator", reportNode.PluginName)
	assert.Equal(t, "generate_detailed", reportNode.PluginMethod)

	// Verify DAG structure
	assert.NotEmpty(t, parsed.EntryPoints)
	assert.Contains(t, parsed.EntryPoints, "recon")
	assert.NotEmpty(t, parsed.ExitPoints)
	assert.Contains(t, parsed.ExitPoints, "notify_team")

	// Verify edges were created from dependencies
	assert.NotEmpty(t, parsed.Edges)
}

// TestParser_CyclicWorkflowFixture tests that cyclic workflows are parsed correctly
// (cycle detection is the validator's job, not the parser's)
func TestParser_CyclicWorkflowFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/cyclic-workflow.yaml")
	require.NoError(t, err)

	// Parsing should succeed - parsers don't detect cycles
	parsed, err := ParseWorkflowYAMLFromBytes(data)
	require.NoError(t, err, "parser should successfully parse workflow with cycles")
	require.NotNil(t, parsed)

	assert.Equal(t, "Cyclic Workflow", parsed.Name)
	assert.Len(t, parsed.Nodes, 4)

	// Verify the cyclic dependencies are present
	nodeA := parsed.Nodes["node_a"]
	nodeB := parsed.Nodes["node_b"]
	nodeC := parsed.Nodes["node_c"]

	require.NotNil(t, nodeA)
	require.NotNil(t, nodeB)
	require.NotNil(t, nodeC)

	assert.Contains(t, nodeA.Dependencies, "node_c")
	assert.Contains(t, nodeB.Dependencies, "node_a")
	assert.Contains(t, nodeC.Dependencies, "node_b")

	// Now validate with the DAG validator - this should detect the cycle
	workflow := parsed.ToWorkflow()
	validator := NewDAGValidator()
	err = validator.Validate(workflow)

	require.Error(t, err, "validator should detect cycle")
	wfErr, ok := err.(*WorkflowError)
	require.True(t, ok)
	assert.Equal(t, WorkflowErrorCycleDetected, wfErr.Code)
}

// TestParser_SelfLoopFixture tests detection of self-referential dependencies
func TestParser_SelfLoopFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/self-loop.yaml")
	require.NoError(t, err)

	// Parsing succeeds
	parsed, err := ParseWorkflowYAMLFromBytes(data)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Verify self-loop exists in structure
	loopNode := parsed.Nodes["loop_node"]
	require.NotNil(t, loopNode)
	assert.Contains(t, loopNode.Dependencies, "loop_node", "should have self-reference")

	// Validation should detect the self-loop as a cycle
	workflow := parsed.ToWorkflow()
	validator := NewDAGValidator()
	err = validator.Validate(workflow)

	require.Error(t, err, "validator should detect self-loop")
	wfErr, ok := err.(*WorkflowError)
	require.True(t, ok)
	assert.Equal(t, WorkflowErrorCycleDetected, wfErr.Code)
}

// TestParser_MissingDependenciesFixture tests detection of non-existent dependencies
func TestParser_MissingDependenciesFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/missing-deps.yaml")
	require.NoError(t, err)

	// Parsing should fail - parser validates dependency references
	parsed, err := ParseWorkflowYAMLFromBytes(data)
	require.Error(t, err, "should fail when dependencies reference non-existent nodes")
	assert.Nil(t, parsed)

	// Verify error message mentions the missing dependency
	assert.Contains(t, err.Error(), "non-existent", "error should mention non-existent dependency")
	assert.Contains(t, err.Error(), "nonexistent_node_1", "error should name the missing node")

	// Verify it's a ParseError with proper fields
	parseErr, ok := err.(*ParseError)
	require.True(t, ok, "should return ParseError")
	assert.NotEmpty(t, parseErr.NodeID, "should identify the problematic node")
}

// TestParser_DuplicateNodesFixture tests detection of duplicate node IDs
func TestParser_DuplicateNodesFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/duplicate-nodes.yaml")
	require.NoError(t, err)

	parsed, err := ParseWorkflowYAMLFromBytes(data)
	require.Error(t, err, "should fail when nodes have duplicate IDs")
	assert.Nil(t, parsed)

	// Verify error message
	assert.Contains(t, err.Error(), "duplicate", "error should mention duplicate")
	assert.Contains(t, err.Error(), "scanner", "error should name the duplicate ID")

	// Verify ParseError structure
	parseErr, ok := err.(*ParseError)
	require.True(t, ok)
	assert.Greater(t, parseErr.Line, 0, "should provide line number")
}

// TestParser_MalformedYAMLFixture tests handling of malformed YAML
func TestParser_MalformedYAMLFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/malformed-yaml.yaml")
	require.NoError(t, err)

	parsed, err := ParseWorkflowYAMLFromBytes(data)
	require.Error(t, err, "should fail on malformed YAML")
	assert.Nil(t, parsed)

	// Verify it's a ParseError
	parseErr, ok := err.(*ParseError)
	require.True(t, ok)
	assert.Contains(t, parseErr.Message, "YAML", "error should mention YAML")
}

// TestParser_MissingNodeIDFixture tests detection of missing node IDs
func TestParser_MissingNodeIDFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/missing-node-id.yaml")
	require.NoError(t, err)

	parsed, err := ParseWorkflowYAMLFromBytes(data)
	require.Error(t, err, "should fail when node is missing ID")
	assert.Nil(t, parsed)

	assert.Contains(t, err.Error(), "id", "error should mention missing ID")

	parseErr, ok := err.(*ParseError)
	require.True(t, ok)
	assert.Greater(t, parseErr.Line, 0, "should provide line number of problematic node")
}

// TestParser_EmptyWorkflowFixture tests detection of empty workflows
func TestParser_EmptyWorkflowFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/empty-workflow.yaml")
	require.NoError(t, err)

	parsed, err := ParseWorkflowYAMLFromBytes(data)
	require.Error(t, err, "should fail on empty workflow")
	assert.Nil(t, parsed)

	assert.Contains(t, err.Error(), "at least one node", "error should mention missing nodes")
}

// TestParser_MissingNameFixture tests detection of missing workflow name
func TestParser_MissingNameFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/missing-name.yaml")
	require.NoError(t, err)

	parsed, err := ParseWorkflowYAMLFromBytes(data)
	require.Error(t, err, "should fail when workflow name is missing")
	assert.Nil(t, parsed)

	assert.Contains(t, err.Error(), "name", "error should mention missing name")

	parseErr, ok := err.(*ParseError)
	require.True(t, ok)
	// Line number may be 0 for missing name field since it's a top-level validation
	// The important thing is that we get a proper ParseError
	assert.NotEmpty(t, parseErr.Message)
}

// TestParser_InvalidFieldsFixture tests multiple field validation errors
func TestParser_InvalidFieldsFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/invalid-fields.yaml")
	require.NoError(t, err)

	// This file has many errors - parser should catch the first one
	parsed, err := ParseWorkflowYAMLFromBytes(data)
	require.Error(t, err, "should fail on invalid fields")
	assert.Nil(t, parsed)

	// Should be a ParseError with line information
	parseErr, ok := err.(*ParseError)
	require.True(t, ok, "should return ParseError")
	assert.Greater(t, parseErr.Line, 0, "should provide line number")
	assert.NotEmpty(t, parseErr.NodeID, "should identify problematic node")
}

// TestParser_AllNodeTypes tests parsing of all supported node types
func TestParser_AllNodeTypes(t *testing.T) {
	yaml := `
name: All Node Types Test
description: Tests all supported node types
nodes:
  - id: agent_node
    type: agent
    name: Agent Node
    agent: test-agent
    timeout: 1m
    task:
      goal: test goal

  - id: tool_node
    type: tool
    name: Tool Node
    tool: test-tool
    depends_on: [agent_node]
    input:
      param1: value1

  - id: plugin_node
    type: plugin
    name: Plugin Node
    plugin: test-plugin
    method: execute
    depends_on: [tool_node]
    params:
      key: value

  - id: condition_node
    type: condition
    name: Condition Node
    depends_on: [plugin_node]
    condition:
      expression: "result.status == 'success'"
      true_branch: [success_path]
      false_branch: [failure_path]

  - id: parallel_node
    type: parallel
    name: Parallel Node
    sub_nodes:
      - id: sub1
        type: agent
        name: Sub 1
        agent: worker1
      - id: sub2
        type: agent
        name: Sub 2
        agent: worker2

  - id: join_node
    type: join
    name: Join Node
    depends_on: [parallel_node]

  - id: success_path
    type: agent
    name: Success Path
    agent: success-agent

  - id: failure_path
    type: agent
    name: Failure Path
    agent: failure-agent
`

	parsed, err := ParseWorkflowYAMLFromBytes([]byte(yaml))
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Verify all node types
	assert.Equal(t, NodeTypeAgent, parsed.Nodes["agent_node"].Type)
	assert.Equal(t, NodeTypeTool, parsed.Nodes["tool_node"].Type)
	assert.Equal(t, NodeTypePlugin, parsed.Nodes["plugin_node"].Type)
	assert.Equal(t, NodeTypeCondition, parsed.Nodes["condition_node"].Type)
	assert.Equal(t, NodeTypeParallel, parsed.Nodes["parallel_node"].Type)
	assert.Equal(t, NodeTypeJoin, parsed.Nodes["join_node"].Type)

	// Verify node-specific fields
	assert.Equal(t, "test-agent", parsed.Nodes["agent_node"].AgentName)
	assert.Equal(t, "test-tool", parsed.Nodes["tool_node"].ToolName)
	assert.Equal(t, "test-plugin", parsed.Nodes["plugin_node"].PluginName)
	assert.Equal(t, "execute", parsed.Nodes["plugin_node"].PluginMethod)
	assert.NotNil(t, parsed.Nodes["condition_node"].Condition)
	assert.Len(t, parsed.Nodes["parallel_node"].SubNodes, 2)
}

// TestParser_RetryPolicyValidation tests comprehensive retry policy validation
func TestParser_RetryPolicyValidation(t *testing.T) {
	tests := []struct {
		name        string
		retryConfig string
		expectError bool
		errorMatch  string
		validateFn  func(*testing.T, *RetryPolicy)
	}{
		{
			name: "valid constant backoff",
			retryConfig: `
      max_retries: 3
      backoff: constant
      initial_delay: 5s`,
			expectError: false,
			validateFn: func(t *testing.T, p *RetryPolicy) {
				assert.Equal(t, BackoffConstant, p.BackoffStrategy)
				assert.Equal(t, 5*time.Second, p.InitialDelay)
			},
		},
		{
			name: "valid linear backoff",
			retryConfig: `
      max_retries: 4
      backoff: linear
      initial_delay: 2s`,
			expectError: false,
			validateFn: func(t *testing.T, p *RetryPolicy) {
				assert.Equal(t, BackoffLinear, p.BackoffStrategy)
			},
		},
		{
			name: "valid exponential backoff",
			retryConfig: `
      max_retries: 5
      backoff: exponential
      initial_delay: 1s
      max_delay: 60s
      multiplier: 2.5`,
			expectError: false,
			validateFn: func(t *testing.T, p *RetryPolicy) {
				assert.Equal(t, BackoffExponential, p.BackoffStrategy)
				assert.Equal(t, 60*time.Second, p.MaxDelay)
				assert.Equal(t, 2.5, p.Multiplier)
			},
		},
		{
			name: "negative max_retries",
			retryConfig: `
      max_retries: -1
      backoff: constant
      initial_delay: 1s`,
			expectError: true,
			errorMatch:  "non-negative",
		},
		{
			name: "invalid backoff strategy",
			retryConfig: `
      max_retries: 3
      backoff: unknown
      initial_delay: 1s`,
			expectError: true,
			errorMatch:  "backoff",
		},
		{
			name: "exponential without max_delay",
			retryConfig: `
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      multiplier: 2.0`,
			expectError: true,
			errorMatch:  "max_delay is required",
		},
		{
			name: "exponential without multiplier",
			retryConfig: `
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      max_delay: 30s`,
			expectError: true,
			errorMatch:  "multiplier must be positive",
		},
		{
			name: "exponential with zero multiplier",
			retryConfig: `
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      max_delay: 30s
      multiplier: 0`,
			expectError: true,
			errorMatch:  "multiplier must be positive",
		},
		{
			name: "invalid initial_delay format",
			retryConfig: `
      max_retries: 3
      backoff: constant
      initial_delay: not-a-duration`,
			expectError: true,
			errorMatch:  "initial_delay",
		},
		{
			name: "invalid max_delay format",
			retryConfig: `
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      max_delay: invalid
      multiplier: 2.0`,
			expectError: true,
			errorMatch:  "max_delay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := `
name: Retry Test
nodes:
  - id: node1
    type: agent
    name: Test
    agent: test-agent
    retry:
` + tt.retryConfig

			parsed, err := ParseWorkflowYAMLFromBytes([]byte(yaml))

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMatch != "" {
					assert.Contains(t, err.Error(), tt.errorMatch)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, parsed)

				node := parsed.Nodes["node1"]
				require.NotNil(t, node)
				require.NotNil(t, node.RetryPolicy)

				if tt.validateFn != nil {
					tt.validateFn(t, node.RetryPolicy)
				}
			}
		})
	}
}

// TestParser_TimeoutParsing tests duration parsing for timeouts
func TestParser_TimeoutParsing(t *testing.T) {
	tests := []struct {
		name        string
		timeout     string
		expected    time.Duration
		expectError bool
	}{
		{"milliseconds", "500ms", 500 * time.Millisecond, false},
		{"seconds", "30s", 30 * time.Second, false},
		{"minutes", "5m", 5 * time.Minute, false},
		{"hours", "2h", 2 * time.Hour, false},
		{"combined", "1h30m", 90 * time.Minute, false},
		{"complex", "2h15m30s", 2*time.Hour + 15*time.Minute + 30*time.Second, false},
		{"invalid format", "30 seconds", 0, true},
		{"no unit", "30", 0, true},
		{"invalid unit", "30x", 0, true},
		{"empty", "", 0, false}, // Empty timeout is valid (means no timeout)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := `
name: Timeout Test
nodes:
  - id: node1
    type: agent
    name: Test
    agent: test-agent`

			if tt.timeout != "" {
				yaml += "\n    timeout: " + tt.timeout
			}

			parsed, err := ParseWorkflowYAMLFromBytes([]byte(yaml))

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "timeout")
			} else {
				require.NoError(t, err)
				node := parsed.Nodes["node1"]
				assert.Equal(t, tt.expected, node.Timeout)
			}
		})
	}
}

// TestParser_DAGStructure tests entry/exit point calculation
func TestParser_DAGStructure(t *testing.T) {
	tests := []struct {
		name               string
		yaml               string
		expectedEntries    []string
		expectedExits      []string
		expectedEdgeCount  int
	}{
		{
			name: "single node",
			yaml: `
name: Single
nodes:
  - id: node1
    type: agent
    agent: test`,
			expectedEntries:   []string{"node1"},
			expectedExits:     []string{"node1"},
			expectedEdgeCount: 0,
		},
		{
			name: "linear chain",
			yaml: `
name: Linear
nodes:
  - id: n1
    type: agent
    agent: a1
  - id: n2
    type: agent
    agent: a2
    depends_on: [n1]
  - id: n3
    type: agent
    agent: a3
    depends_on: [n2]`,
			expectedEntries:   []string{"n1"},
			expectedExits:     []string{"n3"},
			expectedEdgeCount: 2,
		},
		{
			name: "diamond pattern",
			yaml: `
name: Diamond
nodes:
  - id: start
    type: agent
    agent: a1
  - id: left
    type: agent
    agent: a2
    depends_on: [start]
  - id: right
    type: agent
    agent: a3
    depends_on: [start]
  - id: join
    type: join
    depends_on: [left, right]`,
			expectedEntries:   []string{"start"},
			expectedExits:     []string{"join"},
			expectedEdgeCount: 4,
		},
		{
			name: "multiple entries",
			yaml: `
name: Multiple Entries
nodes:
  - id: e1
    type: agent
    agent: a1
  - id: e2
    type: agent
    agent: a2
  - id: merge
    type: join
    depends_on: [e1, e2]`,
			expectedEntries:   []string{"e1", "e2"},
			expectedExits:     []string{"merge"},
			expectedEdgeCount: 2,
		},
		{
			name: "multiple exits",
			yaml: `
name: Multiple Exits
nodes:
  - id: start
    type: agent
    agent: a1
  - id: x1
    type: agent
    agent: a2
    depends_on: [start]
  - id: x2
    type: agent
    agent: a3
    depends_on: [start]`,
			expectedEntries:   []string{"start"},
			expectedExits:     []string{"x1", "x2"},
			expectedEdgeCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseWorkflowYAMLFromBytes([]byte(tt.yaml))
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.expectedEntries, parsed.EntryPoints,
				"entry points mismatch")
			assert.ElementsMatch(t, tt.expectedExits, parsed.ExitPoints,
				"exit points mismatch")
			assert.Len(t, parsed.Edges, tt.expectedEdgeCount,
				"edge count mismatch")
		})
	}
}

// TestParser_EdgeCases tests various edge cases and boundary conditions
func TestParser_EdgeCases(t *testing.T) {
	t.Run("empty dependencies array", func(t *testing.T) {
		yaml := `
name: Empty Deps
nodes:
  - id: node1
    type: agent
    agent: test
    depends_on: []`

		parsed, err := ParseWorkflowYAMLFromBytes([]byte(yaml))
		require.NoError(t, err)
		assert.Empty(t, parsed.Nodes["node1"].Dependencies)
		assert.Contains(t, parsed.EntryPoints, "node1")
	})

	t.Run("zero timeout", func(t *testing.T) {
		yaml := `
name: Zero Timeout
nodes:
  - id: node1
    type: agent
    agent: test
    timeout: 0s`

		parsed, err := ParseWorkflowYAMLFromBytes([]byte(yaml))
		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), parsed.Nodes["node1"].Timeout)
	})

	t.Run("zero retries", func(t *testing.T) {
		yaml := `
name: No Retries
nodes:
  - id: node1
    type: agent
    agent: test
    retry:
      max_retries: 0
      backoff: constant
      initial_delay: 1s`

		parsed, err := ParseWorkflowYAMLFromBytes([]byte(yaml))
		require.NoError(t, err)
		assert.Equal(t, 0, parsed.Nodes["node1"].RetryPolicy.MaxRetries)
	})

	t.Run("deeply nested task config", func(t *testing.T) {
		yaml := `
name: Deep Nesting
nodes:
  - id: node1
    type: agent
    agent: test
    task:
      level1:
        level2:
          level3:
            value: deep`

		parsed, err := ParseWorkflowYAMLFromBytes([]byte(yaml))
		require.NoError(t, err)
		require.NotNil(t, parsed.Nodes["node1"].AgentTask)
		// Verify deep structure preserved
		level1, ok := parsed.Nodes["node1"].AgentTask.Input["level1"].(map[string]interface{})
		require.True(t, ok)
		require.NotNil(t, level1)
	})

	t.Run("unicode in names", func(t *testing.T) {
		yaml := `
name: Unicode Test 🚀
description: Test with émojis 中文
nodes:
  - id: node1
    type: agent
    name: Test Node 测试
    agent: test-agent`

		parsed, err := ParseWorkflowYAMLFromBytes([]byte(yaml))
		require.NoError(t, err)
		assert.Contains(t, parsed.Name, "🚀")
		assert.Contains(t, parsed.Description, "中文")
		assert.Contains(t, parsed.Nodes["node1"].Name, "测试")
	})

	t.Run("special characters in IDs", func(t *testing.T) {
		yaml := `
name: Special Chars
nodes:
  - id: node-1_test.v2
    type: agent
    agent: test`

		parsed, err := ParseWorkflowYAMLFromBytes([]byte(yaml))
		require.NoError(t, err)
		assert.Contains(t, parsed.Nodes, "node-1_test.v2")
	})
}

// TestParser_ToWorkflow tests conversion to Workflow structure
func TestParser_ToWorkflow(t *testing.T) {
	yaml := `
name: Test Workflow
description: Test description
nodes:
  - id: node1
    type: agent
    agent: test-agent`

	parsed, err := ParseWorkflowYAMLFromBytes([]byte(yaml))
	require.NoError(t, err)

	workflow := parsed.ToWorkflow()
	require.NotNil(t, workflow)

	assert.Equal(t, parsed.Name, workflow.Name)
	assert.Equal(t, parsed.Description, workflow.Description)
	assert.Equal(t, parsed.Nodes, workflow.Nodes)
	assert.Equal(t, parsed.Edges, workflow.Edges)
	assert.Equal(t, parsed.EntryPoints, workflow.EntryPoints)
	assert.Equal(t, parsed.ExitPoints, workflow.ExitPoints)
	assert.NotEmpty(t, workflow.ID)
	assert.False(t, workflow.CreatedAt.IsZero())
}

// TestParser_FileOperations tests file-based parsing
func TestParser_FileOperations(t *testing.T) {
	t.Run("parse valid file", func(t *testing.T) {
		// Skip if fixture doesn't exist
		if _, err := os.Stat("testdata/valid-workflow.yaml"); os.IsNotExist(err) {
			t.Skip("test fixture not available")
		}

		parsed, err := ParseWorkflowYAML("testdata/valid-workflow.yaml")
		require.NoError(t, err)
		require.NotNil(t, parsed)
		assert.Equal(t, "testdata/valid-workflow.yaml", parsed.SourceFile)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		parsed, err := ParseWorkflowYAML("testdata/does-not-exist.yaml")
		require.Error(t, err)
		assert.Nil(t, parsed)
		assert.Contains(t, err.Error(), "failed to read")
	})
}

// BenchmarkParser_SmallWorkflow benchmarks parsing a small workflow
func BenchmarkParser_SmallWorkflow(b *testing.B) {
	yaml := []byte(`
name: Small Workflow
nodes:
  - id: node1
    type: agent
    agent: test
  - id: node2
    type: agent
    agent: test
    depends_on: [node1]
`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseWorkflowYAMLFromBytes(yaml)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParser_LargeWorkflow benchmarks parsing a large workflow
func BenchmarkParser_LargeWorkflow(b *testing.B) {
	data, err := os.ReadFile("testdata/valid-workflow.yaml")
	if err != nil {
		b.Skip("test fixture not available")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseWorkflowYAMLFromBytes(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
