package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorkflow_BasicWorkflow(t *testing.T) {
	yamlData := []byte(`
name: Test Workflow
description: A simple test workflow
nodes:
  - id: node1
    type: agent
    name: First Node
    description: First node in the workflow
    agent: test-agent
    task:
      action: scan
      target: localhost
`)

	wf, err := ParseWorkflow(yamlData)
	require.NoError(t, err)
	require.NotNil(t, wf)

	assert.Equal(t, "Test Workflow", wf.Name)
	assert.Equal(t, "A simple test workflow", wf.Description)
	assert.Len(t, wf.Nodes, 1)
	assert.Contains(t, wf.Nodes, "node1")

	node := wf.Nodes["node1"]
	assert.Equal(t, "node1", node.ID)
	assert.Equal(t, NodeTypeAgent, node.Type)
	assert.Equal(t, "First Node", node.Name)
	assert.Equal(t, "First node in the workflow", node.Description)
	assert.Equal(t, "test-agent", node.AgentName)
	assert.NotNil(t, node.AgentTask)
	assert.Equal(t, "scan", node.AgentTask.Input["action"])
	assert.Equal(t, "localhost", node.AgentTask.Input["target"])
}

func TestParseWorkflow_MultipleNodesWithDependencies(t *testing.T) {
	yamlData := []byte(`
name: Complex Workflow
description: Workflow with dependencies
nodes:
  - id: scan
    type: agent
    name: Scan Target
    agent: scanner
    task:
      target: example.com

  - id: analyze
    type: agent
    name: Analyze Results
    agent: analyzer
    depends_on:
      - scan
    task:
      depth: deep

  - id: report
    type: tool
    name: Generate Report
    tool: report-generator
    depends_on:
      - analyze
    input:
      format: pdf
`)

	wf, err := ParseWorkflow(yamlData)
	require.NoError(t, err)
	require.NotNil(t, wf)

	assert.Len(t, wf.Nodes, 3)
	assert.Len(t, wf.Edges, 2)

	// Verify dependencies
	scanNode := wf.Nodes["scan"]
	assert.Empty(t, scanNode.Dependencies)

	analyzeNode := wf.Nodes["analyze"]
	assert.Equal(t, []string{"scan"}, analyzeNode.Dependencies)

	reportNode := wf.Nodes["report"]
	assert.Equal(t, []string{"analyze"}, reportNode.Dependencies)

	// Verify edges
	assert.Contains(t, wf.Edges, WorkflowEdge{From: "scan", To: "analyze"})
	assert.Contains(t, wf.Edges, WorkflowEdge{From: "analyze", To: "report"})

	// Verify entry and exit points
	assert.Equal(t, []string{"scan"}, wf.EntryPoints)
	assert.Equal(t, []string{"report"}, wf.ExitPoints)
}

func TestParseWorkflow_AllNodeTypes(t *testing.T) {
	yamlData := []byte(`
name: All Node Types
description: Workflow demonstrating all node types
nodes:
  - id: agent_node
    type: agent
    name: Agent Node
    agent: test-agent
    task:
      action: test

  - id: tool_node
    type: tool
    name: Tool Node
    tool: test-tool
    depends_on:
      - agent_node
    input:
      param1: value1

  - id: plugin_node
    type: plugin
    name: Plugin Node
    plugin: test-plugin
    method: execute
    depends_on:
      - tool_node
    params:
      config: test

  - id: condition_node
    type: condition
    name: Condition Node
    depends_on:
      - plugin_node
    condition:
      expression: result.success == true
      true_branch:
        - success_node
      false_branch:
        - failure_node

  - id: parallel_node
    type: parallel
    name: Parallel Node
    sub_nodes:
      - id: parallel_sub1
        type: agent
        name: Sub Task 1
        agent: worker1
      - id: parallel_sub2
        type: agent
        name: Sub Task 2
        agent: worker2

  - id: join_node
    type: join
    name: Join Node
    depends_on:
      - parallel_node

  - id: success_node
    type: agent
    name: Success Path
    agent: success-handler

  - id: failure_node
    type: agent
    name: Failure Path
    agent: failure-handler
`)

	wf, err := ParseWorkflow(yamlData)
	require.NoError(t, err)
	require.NotNil(t, wf)

	assert.Len(t, wf.Nodes, 8)

	// Verify agent node
	agentNode := wf.Nodes["agent_node"]
	assert.Equal(t, NodeTypeAgent, agentNode.Type)
	assert.Equal(t, "test-agent", agentNode.AgentName)

	// Verify tool node
	toolNode := wf.Nodes["tool_node"]
	assert.Equal(t, NodeTypeTool, toolNode.Type)
	assert.Equal(t, "test-tool", toolNode.ToolName)
	assert.Equal(t, "value1", toolNode.ToolInput["param1"])

	// Verify plugin node
	pluginNode := wf.Nodes["plugin_node"]
	assert.Equal(t, NodeTypePlugin, pluginNode.Type)
	assert.Equal(t, "test-plugin", pluginNode.PluginName)
	assert.Equal(t, "execute", pluginNode.PluginMethod)
	assert.Equal(t, "test", pluginNode.PluginParams["config"])

	// Verify condition node
	conditionNode := wf.Nodes["condition_node"]
	assert.Equal(t, NodeTypeCondition, conditionNode.Type)
	assert.NotNil(t, conditionNode.Condition)
	assert.Equal(t, "result.success == true", conditionNode.Condition.Expression)
	assert.Equal(t, []string{"success_node"}, conditionNode.Condition.TrueBranch)
	assert.Equal(t, []string{"failure_node"}, conditionNode.Condition.FalseBranch)

	// Verify parallel node
	parallelNode := wf.Nodes["parallel_node"]
	assert.Equal(t, NodeTypeParallel, parallelNode.Type)
	assert.Len(t, parallelNode.SubNodes, 2)
	assert.Equal(t, "parallel_sub1", parallelNode.SubNodes[0].ID)
	assert.Equal(t, "parallel_sub2", parallelNode.SubNodes[1].ID)

	// Verify join node
	joinNode := wf.Nodes["join_node"]
	assert.Equal(t, NodeTypeJoin, joinNode.Type)
	assert.Equal(t, []string{"parallel_node"}, joinNode.Dependencies)
}

func TestParseWorkflow_TimeoutAndRetry(t *testing.T) {
	yamlData := []byte(`
name: Timeout and Retry
description: Workflow with timeout and retry policies
nodes:
  - id: node1
    type: agent
    name: Node with Timeout
    agent: test-agent
    timeout: 30s
    retry:
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      max_delay: 30s
      multiplier: 2.0
`)

	wf, err := ParseWorkflow(yamlData)
	require.NoError(t, err)
	require.NotNil(t, wf)

	node := wf.Nodes["node1"]
	assert.Equal(t, 30*time.Second, node.Timeout)

	require.NotNil(t, node.RetryPolicy)
	assert.Equal(t, 3, node.RetryPolicy.MaxRetries)
	assert.Equal(t, BackoffExponential, node.RetryPolicy.BackoffStrategy)
	assert.Equal(t, 1*time.Second, node.RetryPolicy.InitialDelay)
	assert.Equal(t, 30*time.Second, node.RetryPolicy.MaxDelay)
	assert.Equal(t, 2.0, node.RetryPolicy.Multiplier)
}

func TestParseWorkflow_RetryStrategies(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected BackoffStrategy
	}{
		{
			name: "constant backoff",
			yaml: `
name: Constant Retry
nodes:
  - id: node1
    type: agent
    name: Test
    agent: test
    retry:
      max_retries: 3
      backoff: constant
      initial_delay: 5s
`,
			expected: BackoffConstant,
		},
		{
			name: "linear backoff",
			yaml: `
name: Linear Retry
nodes:
  - id: node1
    type: agent
    name: Test
    agent: test
    retry:
      max_retries: 3
      backoff: linear
      initial_delay: 2s
`,
			expected: BackoffLinear,
		},
		{
			name: "exponential backoff",
			yaml: `
name: Exponential Retry
nodes:
  - id: node1
    type: agent
    name: Test
    agent: test
    retry:
      max_retries: 5
      backoff: exponential
      initial_delay: 1s
      max_delay: 60s
      multiplier: 2.5
`,
			expected: BackoffExponential,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf, err := ParseWorkflow([]byte(tt.yaml))
			require.NoError(t, err)

			node := wf.Nodes["node1"]
			require.NotNil(t, node.RetryPolicy)
			assert.Equal(t, tt.expected, node.RetryPolicy.BackoffStrategy)
		})
	}
}

func TestParseWorkflow_InvalidYAML(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		expectedErr string
	}{
		{
			name:        "malformed YAML",
			yaml:        "invalid: yaml: syntax:",
			expectedErr: "failed to parse YAML",
		},
		{
			name: "missing workflow name",
			yaml: `
description: Test
nodes:
  - id: node1
    type: agent
    agent: test
`,
			expectedErr: "workflow name is required",
		},
		{
			name: "no nodes",
			yaml: `
name: Empty Workflow
description: No nodes
nodes: []
`,
			expectedErr: "workflow must contain at least one node",
		},
		{
			name: "missing node ID",
			yaml: `
name: Test
nodes:
  - type: agent
    name: Test
    agent: test
`,
			expectedErr: "node ID is required",
		},
		{
			name: "duplicate node IDs",
			yaml: `
name: Test
nodes:
  - id: node1
    type: agent
    agent: test
  - id: node1
    type: tool
    tool: test
`,
			expectedErr: "duplicate node ID",
		},
		{
			name: "invalid node type",
			yaml: `
name: Test
nodes:
  - id: node1
    type: invalid_type
    name: Test
`,
			expectedErr: "invalid node type",
		},
		{
			name: "missing agent name",
			yaml: `
name: Test
nodes:
  - id: node1
    type: agent
    name: Test
`,
			expectedErr: "agent name is required",
		},
		{
			name: "missing tool name",
			yaml: `
name: Test
nodes:
  - id: node1
    type: tool
    name: Test
`,
			expectedErr: "tool name is required",
		},
		{
			name: "missing plugin name",
			yaml: `
name: Test
nodes:
  - id: node1
    type: plugin
    name: Test
    method: execute
`,
			expectedErr: "plugin name is required",
		},
		{
			name: "missing plugin method",
			yaml: `
name: Test
nodes:
  - id: node1
    type: plugin
    name: Test
    plugin: test-plugin
`,
			expectedErr: "plugin method is required",
		},
		{
			name: "missing condition expression",
			yaml: `
name: Test
nodes:
  - id: node1
    type: condition
    name: Test
    condition:
      true_branch:
        - node2
`,
			expectedErr: "condition expression is required",
		},
		{
			name: "parallel node with no sub-nodes",
			yaml: `
name: Test
nodes:
  - id: node1
    type: parallel
    name: Test
    sub_nodes: []
`,
			expectedErr: "parallel nodes must contain at least one sub-node",
		},
		{
			name: "invalid timeout format",
			yaml: `
name: Test
nodes:
  - id: node1
    type: agent
    agent: test
    timeout: invalid
`,
			expectedErr: "invalid timeout format",
		},
		{
			name: "invalid backoff strategy",
			yaml: `
name: Test
nodes:
  - id: node1
    type: agent
    agent: test
    retry:
      max_retries: 3
      backoff: invalid_strategy
      initial_delay: 1s
`,
			expectedErr: "invalid backoff strategy",
		},
		{
			name: "exponential backoff missing max_delay",
			yaml: `
name: Test
nodes:
  - id: node1
    type: agent
    agent: test
    retry:
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      multiplier: 2.0
`,
			expectedErr: "max_delay is required for exponential backoff",
		},
		{
			name: "exponential backoff invalid multiplier",
			yaml: `
name: Test
nodes:
  - id: node1
    type: agent
    agent: test
    retry:
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      max_delay: 30s
      multiplier: 0
`,
			expectedErr: "multiplier must be positive",
		},
		{
			name: "dependency on non-existent node",
			yaml: `
name: Test
nodes:
  - id: node1
    type: agent
    agent: test
    depends_on:
      - nonexistent
`,
			expectedErr: "depends on non-existent node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseWorkflow([]byte(tt.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestParseWorkflowFile(t *testing.T) {
	// Create a temporary YAML file
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "test-workflow.yaml")

	yamlContent := []byte(`
name: File Test Workflow
description: Testing file parsing
nodes:
  - id: node1
    type: agent
    name: Test Node
    agent: test-agent
    task:
      action: test
`)

	err := os.WriteFile(yamlPath, yamlContent, 0644)
	require.NoError(t, err)

	// Parse the file
	wf, err := ParseWorkflowFile(yamlPath)
	require.NoError(t, err)
	require.NotNil(t, wf)

	assert.Equal(t, "File Test Workflow", wf.Name)
	assert.Equal(t, "Testing file parsing", wf.Description)
	assert.Len(t, wf.Nodes, 1)
}

func TestParseWorkflowFile_FileNotFound(t *testing.T) {
	_, err := ParseWorkflowFile("/nonexistent/path/workflow.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read workflow file")
}

func TestCalculateEntryAndExitPoints(t *testing.T) {
	tests := []struct {
		name               string
		yaml               string
		expectedEntryCount int
		expectedExitCount  int
	}{
		{
			name: "single node",
			yaml: `
name: Single Node
nodes:
  - id: node1
    type: agent
    agent: test
`,
			expectedEntryCount: 1,
			expectedExitCount:  1,
		},
		{
			name: "linear chain",
			yaml: `
name: Linear Chain
nodes:
  - id: node1
    type: agent
    agent: test
  - id: node2
    type: agent
    agent: test
    depends_on:
      - node1
  - id: node3
    type: agent
    agent: test
    depends_on:
      - node2
`,
			expectedEntryCount: 1,
			expectedExitCount:  1,
		},
		{
			name: "multiple entry points",
			yaml: `
name: Multiple Entries
nodes:
  - id: entry1
    type: agent
    agent: test
  - id: entry2
    type: agent
    agent: test
  - id: join
    type: join
    depends_on:
      - entry1
      - entry2
`,
			expectedEntryCount: 2,
			expectedExitCount:  1,
		},
		{
			name: "multiple exit points",
			yaml: `
name: Multiple Exits
nodes:
  - id: start
    type: agent
    agent: test
  - id: exit1
    type: agent
    agent: test
    depends_on:
      - start
  - id: exit2
    type: agent
    agent: test
    depends_on:
      - start
`,
			expectedEntryCount: 1,
			expectedExitCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf, err := ParseWorkflow([]byte(tt.yaml))
			require.NoError(t, err)

			assert.Len(t, wf.EntryPoints, tt.expectedEntryCount,
				"Expected %d entry points, got %d: %v",
				tt.expectedEntryCount, len(wf.EntryPoints), wf.EntryPoints)

			assert.Len(t, wf.ExitPoints, tt.expectedExitCount,
				"Expected %d exit points, got %d: %v",
				tt.expectedExitCount, len(wf.ExitPoints), wf.ExitPoints)
		})
	}
}

func TestParseWorkflow_ComplexRealWorldExample(t *testing.T) {
	yamlData := []byte(`
name: Security Assessment Workflow
description: Complete security assessment with reconnaissance, scanning, and reporting
nodes:
  # Reconnaissance Phase
  - id: recon
    type: agent
    name: Reconnaissance
    description: Gather initial information about the target
    agent: recon-agent
    timeout: 5m
    retry:
      max_retries: 2
      backoff: exponential
      initial_delay: 10s
      max_delay: 1m
      multiplier: 2.0
    task:
      target: example.com
      depth: deep

  # Scanning Phase
  - id: port_scan
    type: parallel
    name: Port Scanning
    description: Scan multiple port ranges in parallel
    depends_on:
      - recon
    sub_nodes:
      - id: scan_common
        type: agent
        name: Common Ports
        agent: port-scanner
        task:
          ports: 1-1024
      - id: scan_full
        type: agent
        name: Full Range
        agent: port-scanner
        task:
          ports: 1-65535

  - id: join_scans
    type: join
    name: Wait for Scans
    depends_on:
      - port_scan

  # Analysis Phase
  - id: analyze_results
    type: agent
    name: Analyze Scan Results
    agent: analyzer
    depends_on:
      - join_scans
    timeout: 10m
    task:
      analysis_type: comprehensive

  # Decision Point
  - id: check_vulnerabilities
    type: condition
    name: Check for Vulnerabilities
    depends_on:
      - analyze_results
    condition:
      expression: findings.count > 0
      true_branch:
        - exploit_check
      false_branch:
        - generate_clean_report

  # Exploitation Path
  - id: exploit_check
    type: agent
    name: Check Exploitability
    agent: exploit-checker
    timeout: 15m
    task:
      safe_mode: true

  - id: generate_vuln_report
    type: tool
    name: Generate Vulnerability Report
    tool: report-generator
    depends_on:
      - exploit_check
    input:
      format: pdf
      include_recommendations: true

  # Clean Path
  - id: generate_clean_report
    type: tool
    name: Generate Clean Report
    tool: report-generator
    input:
      format: pdf
      status: clean

  # Notification
  - id: notify
    type: plugin
    name: Send Notification
    plugin: notification-service
    method: send_email
    depends_on:
      - generate_vuln_report
      - generate_clean_report
    params:
      recipients:
        - security@example.com
      priority: high
`)

	wf, err := ParseWorkflow(yamlData)
	require.NoError(t, err)
	require.NotNil(t, wf)

	assert.Equal(t, "Security Assessment Workflow", wf.Name)
	// 9 top-level nodes (parallel node sub-nodes are nested, not in top-level map)
	assert.Len(t, wf.Nodes, 9)

	// Verify recon node with retry policy
	reconNode := wf.Nodes["recon"]
	require.NotNil(t, reconNode)
	assert.Equal(t, 5*time.Minute, reconNode.Timeout)
	assert.NotNil(t, reconNode.RetryPolicy)
	assert.Equal(t, 2, reconNode.RetryPolicy.MaxRetries)

	// Verify parallel node
	portScanNode := wf.Nodes["port_scan"]
	require.NotNil(t, portScanNode)
	assert.Equal(t, NodeTypeParallel, portScanNode.Type)
	assert.Len(t, portScanNode.SubNodes, 2)

	// Verify condition node
	checkNode := wf.Nodes["check_vulnerabilities"]
	require.NotNil(t, checkNode)
	assert.Equal(t, NodeTypeCondition, checkNode.Type)
	assert.Equal(t, "findings.count > 0", checkNode.Condition.Expression)
	assert.Equal(t, []string{"exploit_check"}, checkNode.Condition.TrueBranch)
	assert.Equal(t, []string{"generate_clean_report"}, checkNode.Condition.FalseBranch)

	// Verify notification plugin node
	notifyNode := wf.Nodes["notify"]
	require.NotNil(t, notifyNode)
	assert.Equal(t, NodeTypePlugin, notifyNode.Type)
	assert.Equal(t, "notification-service", notifyNode.PluginName)
	assert.Equal(t, "send_email", notifyNode.PluginMethod)
	assert.Contains(t, notifyNode.Dependencies, "generate_vuln_report")
	assert.Contains(t, notifyNode.Dependencies, "generate_clean_report")

	// Verify workflow structure
	// Entry points: nodes with no dependencies
	// Note: exploit_check and generate_clean_report have dependencies via condition node's branches
	// but those aren't explicit dependencies, so they show up as entry points too
	assert.Contains(t, wf.EntryPoints, "recon")

	// Exit point: notify node has no outgoing edges
	assert.Equal(t, []string{"notify"}, wf.ExitPoints)
}
