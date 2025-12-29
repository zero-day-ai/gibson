package workflow

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestWorkflowStatusIsTerminal tests the IsTerminal method for all WorkflowStatus values
func TestWorkflowStatusIsTerminal(t *testing.T) {
	tests := []struct {
		name       string
		status     WorkflowStatus
		isTerminal bool
	}{
		{
			name:       "pending is not terminal",
			status:     WorkflowStatusPending,
			isTerminal: false,
		},
		{
			name:       "running is not terminal",
			status:     WorkflowStatusRunning,
			isTerminal: false,
		},
		{
			name:       "completed is terminal",
			status:     WorkflowStatusCompleted,
			isTerminal: true,
		},
		{
			name:       "failed is terminal",
			status:     WorkflowStatusFailed,
			isTerminal: true,
		},
		{
			name:       "cancelled is terminal",
			status:     WorkflowStatusCancelled,
			isTerminal: true,
		},
		{
			name:       "unknown status is not terminal",
			status:     WorkflowStatus("unknown"),
			isTerminal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.IsTerminal()
			assert.Equal(t, tt.isTerminal, result)
		})
	}
}

// TestWorkflowStatusString tests the String method for WorkflowStatus
func TestWorkflowStatusString(t *testing.T) {
	tests := []struct {
		name     string
		status   WorkflowStatus
		expected string
	}{
		{
			name:     "pending status",
			status:   WorkflowStatusPending,
			expected: "pending",
		},
		{
			name:     "running status",
			status:   WorkflowStatusRunning,
			expected: "running",
		},
		{
			name:     "completed status",
			status:   WorkflowStatusCompleted,
			expected: "completed",
		},
		{
			name:     "failed status",
			status:   WorkflowStatusFailed,
			expected: "failed",
		},
		{
			name:     "cancelled status",
			status:   WorkflowStatusCancelled,
			expected: "cancelled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNodeTypeStringValues tests that NodeType constants have correct string values
func TestNodeTypeStringValues(t *testing.T) {
	tests := []struct {
		name     string
		nodeType NodeType
		expected string
	}{
		{
			name:     "agent node type",
			nodeType: NodeTypeAgent,
			expected: "agent",
		},
		{
			name:     "tool node type",
			nodeType: NodeTypeTool,
			expected: "tool",
		},
		{
			name:     "plugin node type",
			nodeType: NodeTypePlugin,
			expected: "plugin",
		},
		{
			name:     "condition node type",
			nodeType: NodeTypeCondition,
			expected: "condition",
		},
		{
			name:     "parallel node type",
			nodeType: NodeTypeParallel,
			expected: "parallel",
		},
		{
			name:     "join node type",
			nodeType: NodeTypeJoin,
			expected: "join",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.nodeType))
		})
	}
}

// TestNodeStatusStringValues tests that NodeStatus constants have correct string values
func TestNodeStatusStringValues(t *testing.T) {
	tests := []struct {
		name     string
		status   NodeStatus
		expected string
	}{
		{
			name:     "pending status",
			status:   NodeStatusPending,
			expected: "pending",
		},
		{
			name:     "running status",
			status:   NodeStatusRunning,
			expected: "running",
		},
		{
			name:     "completed status",
			status:   NodeStatusCompleted,
			expected: "completed",
		},
		{
			name:     "failed status",
			status:   NodeStatusFailed,
			expected: "failed",
		},
		{
			name:     "skipped status",
			status:   NodeStatusSkipped,
			expected: "skipped",
		},
		{
			name:     "cancelled status",
			status:   NodeStatusCancelled,
			expected: "cancelled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.status))
		})
	}
}

// TestBackoffStrategyStringValues tests that BackoffStrategy constants have correct string values
func TestBackoffStrategyStringValues(t *testing.T) {
	tests := []struct {
		name     string
		strategy BackoffStrategy
		expected string
	}{
		{
			name:     "constant backoff",
			strategy: BackoffConstant,
			expected: "constant",
		},
		{
			name:     "linear backoff",
			strategy: BackoffLinear,
			expected: "linear",
		},
		{
			name:     "exponential backoff",
			strategy: BackoffExponential,
			expected: "exponential",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.strategy))
		})
	}
}

// TestRetryPolicyCalculateDelay tests the CalculateDelay method for all backoff strategies
func TestRetryPolicyCalculateDelay(t *testing.T) {
	tests := []struct {
		name          string
		policy        *RetryPolicy
		attempt       int
		expectedDelay time.Duration
	}{
		// Constant backoff tests
		{
			name: "constant backoff - first attempt",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffConstant,
				InitialDelay:    100 * time.Millisecond,
			},
			attempt:       0,
			expectedDelay: 100 * time.Millisecond,
		},
		{
			name: "constant backoff - third attempt",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffConstant,
				InitialDelay:    100 * time.Millisecond,
			},
			attempt:       2,
			expectedDelay: 100 * time.Millisecond,
		},
		{
			name: "constant backoff - tenth attempt",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffConstant,
				InitialDelay:    200 * time.Millisecond,
			},
			attempt:       9,
			expectedDelay: 200 * time.Millisecond,
		},

		// Linear backoff tests
		{
			name: "linear backoff - first attempt",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffLinear,
				InitialDelay:    100 * time.Millisecond,
			},
			attempt:       0,
			expectedDelay: 100 * time.Millisecond, // 100 + (100 * 0)
		},
		{
			name: "linear backoff - second attempt",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffLinear,
				InitialDelay:    100 * time.Millisecond,
			},
			attempt:       1,
			expectedDelay: 200 * time.Millisecond, // 100 + (100 * 1)
		},
		{
			name: "linear backoff - third attempt",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffLinear,
				InitialDelay:    100 * time.Millisecond,
			},
			attempt:       2,
			expectedDelay: 300 * time.Millisecond, // 100 + (100 * 2)
		},
		{
			name: "linear backoff - fifth attempt",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffLinear,
				InitialDelay:    50 * time.Millisecond,
			},
			attempt:       4,
			expectedDelay: 250 * time.Millisecond, // 50 + (50 * 4)
		},

		// Exponential backoff tests
		{
			name: "exponential backoff - first attempt",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffExponential,
				InitialDelay:    100 * time.Millisecond,
				MaxDelay:        10 * time.Second,
				Multiplier:      2.0,
			},
			attempt:       0,
			expectedDelay: 100 * time.Millisecond, // 100 * 2^0
		},
		{
			name: "exponential backoff - second attempt",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffExponential,
				InitialDelay:    100 * time.Millisecond,
				MaxDelay:        10 * time.Second,
				Multiplier:      2.0,
			},
			attempt:       1,
			expectedDelay: 200 * time.Millisecond, // 100 * 2^1
		},
		{
			name: "exponential backoff - third attempt",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffExponential,
				InitialDelay:    100 * time.Millisecond,
				MaxDelay:        10 * time.Second,
				Multiplier:      2.0,
			},
			attempt:       2,
			expectedDelay: 400 * time.Millisecond, // 100 * 2^2
		},
		{
			name: "exponential backoff - fourth attempt",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffExponential,
				InitialDelay:    100 * time.Millisecond,
				MaxDelay:        10 * time.Second,
				Multiplier:      2.0,
			},
			attempt:       3,
			expectedDelay: 800 * time.Millisecond, // 100 * 2^3
		},
		{
			name: "exponential backoff - max delay enforced",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffExponential,
				InitialDelay:    100 * time.Millisecond,
				MaxDelay:        500 * time.Millisecond,
				Multiplier:      2.0,
			},
			attempt:       5,
			expectedDelay: 500 * time.Millisecond, // Would be 3200ms but capped at 500ms
		},
		{
			name: "exponential backoff - different multiplier",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffExponential,
				InitialDelay:    50 * time.Millisecond,
				MaxDelay:        10 * time.Second,
				Multiplier:      3.0,
			},
			attempt:       3,
			expectedDelay: 1350 * time.Millisecond, // 50 * 3^3
		},

		// Unknown/default backoff strategy
		{
			name: "unknown backoff strategy defaults to initial delay",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffStrategy("unknown"),
				InitialDelay:    100 * time.Millisecond,
			},
			attempt:       5,
			expectedDelay: 100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := tt.policy.CalculateDelay(tt.attempt)
			assert.Equal(t, tt.expectedDelay, delay)
		})
	}
}

// TestWorkflowJSONSerialization tests JSON serialization and deserialization of Workflow
func TestWorkflowJSONSerialization(t *testing.T) {
	createdAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		workflow *Workflow
	}{
		{
			name: "minimal workflow",
			workflow: &Workflow{
				ID:          types.NewID(),
				Name:        "Test Workflow",
				Description: "A test workflow",
				Nodes:       map[string]*WorkflowNode{},
				Edges:       []WorkflowEdge{},
				CreatedAt:   createdAt,
			},
		},
		{
			name: "workflow with nodes and edges",
			workflow: &Workflow{
				ID:          types.NewID(),
				Name:        "Complex Workflow",
				Description: "A more complex workflow",
				Nodes: map[string]*WorkflowNode{
					"node1": {
						ID:          "node1",
						Type:        NodeTypeAgent,
						Name:        "Agent Node",
						Description: "An agent node",
						AgentName:   "test-agent",
					},
					"node2": {
						ID:          "node2",
						Type:        NodeTypeTool,
						Name:        "Tool Node",
						Description: "A tool node",
						ToolName:    "test-tool",
						ToolInput:   map[string]any{"key": "value"},
					},
				},
				Edges: []WorkflowEdge{
					{From: "node1", To: "node2"},
				},
				EntryPoints: []string{"node1"},
				ExitPoints:  []string{"node2"},
				Metadata: map[string]any{
					"version": "1.0",
					"author":  "test",
				},
				CreatedAt: createdAt,
			},
		},
		{
			name: "workflow with all node types",
			workflow: &Workflow{
				ID:          types.NewID(),
				Name:        "All Node Types",
				Description: "Workflow with all node types",
				Nodes: map[string]*WorkflowNode{
					"agent": {
						ID:        "agent",
						Type:      NodeTypeAgent,
						Name:      "Agent",
						AgentName: "test-agent",
					},
					"tool": {
						ID:       "tool",
						Type:     NodeTypeTool,
						Name:     "Tool",
						ToolName: "test-tool",
					},
					"plugin": {
						ID:           "plugin",
						Type:         NodeTypePlugin,
						Name:         "Plugin",
						PluginName:   "test-plugin",
						PluginMethod: "execute",
					},
					"condition": {
						ID:   "condition",
						Type: NodeTypeCondition,
						Name: "Condition",
						Condition: &NodeCondition{
							Expression:  "result.status == 'success'",
							TrueBranch:  []string{"success"},
							FalseBranch: []string{"failure"},
						},
					},
					"parallel": {
						ID:   "parallel",
						Type: NodeTypeParallel,
						Name: "Parallel",
						SubNodes: []*WorkflowNode{
							{ID: "sub1", Type: NodeTypeTool, Name: "Sub1"},
						},
					},
					"join": {
						ID:   "join",
						Type: NodeTypeJoin,
						Name: "Join",
					},
				},
				Edges:       []WorkflowEdge{},
				EntryPoints: []string{"agent"},
				ExitPoints:  []string{"join"},
				CreatedAt:   createdAt,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize to JSON
			jsonData, err := json.Marshal(tt.workflow)
			require.NoError(t, err)
			assert.NotEmpty(t, jsonData)

			// Deserialize from JSON
			var decoded Workflow
			err = json.Unmarshal(jsonData, &decoded)
			require.NoError(t, err)

			// Verify round-trip
			assert.Equal(t, tt.workflow.ID, decoded.ID)
			assert.Equal(t, tt.workflow.Name, decoded.Name)
			assert.Equal(t, tt.workflow.Description, decoded.Description)
			assert.Equal(t, len(tt.workflow.Nodes), len(decoded.Nodes))
			assert.Equal(t, len(tt.workflow.Edges), len(decoded.Edges))
			assert.Equal(t, tt.workflow.EntryPoints, decoded.EntryPoints)
			assert.Equal(t, tt.workflow.ExitPoints, decoded.ExitPoints)
			assert.Equal(t, tt.workflow.CreatedAt.Unix(), decoded.CreatedAt.Unix())
		})
	}
}

// TestWorkflowNodeJSONSerialization tests JSON serialization and deserialization of WorkflowNode
func TestWorkflowNodeJSONSerialization(t *testing.T) {
	tests := []struct {
		name string
		node *WorkflowNode
	}{
		{
			name: "agent node",
			node: &WorkflowNode{
				ID:          "agent-1",
				Type:        NodeTypeAgent,
				Name:        "Test Agent",
				Description: "An agent node",
				AgentName:   "test-agent",
				AgentTask: &agent.Task{
					ID:          types.NewID(),
					Name:        "Test Task",
					Description: "A test task",
				},
				Timeout: 30 * time.Second,
				RetryPolicy: &RetryPolicy{
					MaxRetries:      3,
					BackoffStrategy: BackoffExponential,
					InitialDelay:    100 * time.Millisecond,
					MaxDelay:        5 * time.Second,
					Multiplier:      2.0,
				},
			},
		},
		{
			name: "tool node",
			node: &WorkflowNode{
				ID:       "tool-1",
				Type:     NodeTypeTool,
				Name:     "Test Tool",
				ToolName: "test-tool",
				ToolInput: map[string]any{
					"param1": "value1",
					"param2": 42,
				},
				Dependencies: []string{"node-1", "node-2"},
			},
		},
		{
			name: "plugin node",
			node: &WorkflowNode{
				ID:           "plugin-1",
				Type:         NodeTypePlugin,
				Name:         "Test Plugin",
				PluginName:   "test-plugin",
				PluginMethod: "execute",
				PluginParams: map[string]any{
					"config": "value",
				},
				Metadata: map[string]any{
					"version": "1.0",
				},
			},
		},
		{
			name: "condition node",
			node: &WorkflowNode{
				ID:   "condition-1",
				Type: NodeTypeCondition,
				Name: "Test Condition",
				Condition: &NodeCondition{
					Expression:  "result.status == 'success'",
					TrueBranch:  []string{"success-node"},
					FalseBranch: []string{"failure-node"},
				},
			},
		},
		{
			name: "parallel node",
			node: &WorkflowNode{
				ID:   "parallel-1",
				Type: NodeTypeParallel,
				Name: "Parallel Execution",
				SubNodes: []*WorkflowNode{
					{
						ID:       "sub-1",
						Type:     NodeTypeTool,
						Name:     "Sub Task 1",
						ToolName: "tool-1",
					},
					{
						ID:       "sub-2",
						Type:     NodeTypeTool,
						Name:     "Sub Task 2",
						ToolName: "tool-2",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize to JSON
			jsonData, err := json.Marshal(tt.node)
			require.NoError(t, err)
			assert.NotEmpty(t, jsonData)

			// Deserialize from JSON
			var decoded WorkflowNode
			err = json.Unmarshal(jsonData, &decoded)
			require.NoError(t, err)

			// Verify basic fields
			assert.Equal(t, tt.node.ID, decoded.ID)
			assert.Equal(t, tt.node.Type, decoded.Type)
			assert.Equal(t, tt.node.Name, decoded.Name)
			assert.Equal(t, tt.node.Description, decoded.Description)

			// Verify type-specific fields
			switch tt.node.Type {
			case NodeTypeAgent:
				assert.Equal(t, tt.node.AgentName, decoded.AgentName)
			case NodeTypeTool:
				assert.Equal(t, tt.node.ToolName, decoded.ToolName)
				assert.Equal(t, len(tt.node.Dependencies), len(decoded.Dependencies))
			case NodeTypePlugin:
				assert.Equal(t, tt.node.PluginName, decoded.PluginName)
				assert.Equal(t, tt.node.PluginMethod, decoded.PluginMethod)
			case NodeTypeCondition:
				assert.NotNil(t, decoded.Condition)
				assert.Equal(t, tt.node.Condition.Expression, decoded.Condition.Expression)
			case NodeTypeParallel:
				assert.Equal(t, len(tt.node.SubNodes), len(decoded.SubNodes))
			}
		})
	}
}

// TestNodeResultJSONSerialization tests JSON serialization and deserialization of NodeResult
func TestNodeResultJSONSerialization(t *testing.T) {
	startTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	endTime := startTime.Add(5 * time.Second)

	tests := []struct {
		name   string
		result *NodeResult
	}{
		{
			name: "successful node result",
			result: &NodeResult{
				NodeID: "node-1",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"result": "success",
					"count":  42,
				},
				Duration:    5 * time.Second,
				RetryCount:  0,
				StartedAt:   startTime,
				CompletedAt: endTime,
			},
		},
		{
			name: "failed node result with error",
			result: &NodeResult{
				NodeID: "node-2",
				Status: NodeStatusFailed,
				Error: &NodeError{
					Code:    "EXECUTION_ERROR",
					Message: "Failed to execute node",
					Details: map[string]any{
						"reason": "timeout",
					},
				},
				Duration:    10 * time.Second,
				RetryCount:  3,
				StartedAt:   startTime,
				CompletedAt: endTime,
			},
		},
		{
			name: "node result with findings",
			result: &NodeResult{
				NodeID: "node-3",
				Status: NodeStatusCompleted,
				Findings: []agent.Finding{
					{
						ID:          types.NewID(),
						Title:       "Test Finding",
						Description: "A test finding",
						Severity:    agent.SeverityHigh,
					},
				},
				Duration:    3 * time.Second,
				StartedAt:   startTime,
				CompletedAt: endTime,
				Metadata: map[string]any{
					"source": "test",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize to JSON
			jsonData, err := json.Marshal(tt.result)
			require.NoError(t, err)
			assert.NotEmpty(t, jsonData)

			// Deserialize from JSON
			var decoded NodeResult
			err = json.Unmarshal(jsonData, &decoded)
			require.NoError(t, err)

			// Verify round-trip
			assert.Equal(t, tt.result.NodeID, decoded.NodeID)
			assert.Equal(t, tt.result.Status, decoded.Status)
			assert.Equal(t, tt.result.Duration, decoded.Duration)
			assert.Equal(t, tt.result.RetryCount, decoded.RetryCount)
			assert.Equal(t, tt.result.StartedAt.Unix(), decoded.StartedAt.Unix())
			assert.Equal(t, tt.result.CompletedAt.Unix(), decoded.CompletedAt.Unix())

			if tt.result.Error != nil {
				require.NotNil(t, decoded.Error)
				assert.Equal(t, tt.result.Error.Code, decoded.Error.Code)
				assert.Equal(t, tt.result.Error.Message, decoded.Error.Message)
			}

			if tt.result.Findings != nil {
				assert.Equal(t, len(tt.result.Findings), len(decoded.Findings))
			}
		})
	}
}

// TestWorkflowResultJSONSerialization tests JSON serialization and deserialization of WorkflowResult
func TestWorkflowResultJSONSerialization(t *testing.T) {
	workflowID := types.NewID()

	tests := []struct {
		name   string
		result *WorkflowResult
	}{
		{
			name: "successful workflow result",
			result: &WorkflowResult{
				WorkflowID: workflowID,
				Status:     WorkflowStatusCompleted,
				NodeResults: map[string]*NodeResult{
					"node-1": {
						NodeID:   "node-1",
						Status:   NodeStatusCompleted,
						Duration: 5 * time.Second,
					},
					"node-2": {
						NodeID:   "node-2",
						Status:   NodeStatusCompleted,
						Duration: 3 * time.Second,
					},
				},
				TotalDuration: 8 * time.Second,
				NodesExecuted: 2,
				NodesFailed:   0,
				NodesSkipped:  0,
			},
		},
		{
			name: "failed workflow result with error",
			result: &WorkflowResult{
				WorkflowID: workflowID,
				Status:     WorkflowStatusFailed,
				NodeResults: map[string]*NodeResult{
					"node-1": {
						NodeID:   "node-1",
						Status:   NodeStatusCompleted,
						Duration: 5 * time.Second,
					},
					"node-2": {
						NodeID: "node-2",
						Status: NodeStatusFailed,
						Error: &NodeError{
							Code:    "EXECUTION_ERROR",
							Message: "Node failed",
						},
						Duration: 2 * time.Second,
					},
				},
				TotalDuration: 7 * time.Second,
				NodesExecuted: 2,
				NodesFailed:   1,
				NodesSkipped:  0,
				Error: &WorkflowError{
					Code:    WorkflowErrorNodeExecutionFailed,
					Message: "Workflow failed due to node execution error",
					NodeID:  "node-2",
				},
			},
		},
		{
			name: "workflow result with findings",
			result: &WorkflowResult{
				WorkflowID: workflowID,
				Status:     WorkflowStatusCompleted,
				NodeResults: map[string]*NodeResult{
					"node-1": {
						NodeID:   "node-1",
						Status:   NodeStatusCompleted,
						Duration: 5 * time.Second,
					},
				},
				Findings: []agent.Finding{
					{
						ID:       types.NewID(),
						Title:    "Workflow Finding",
						Severity: agent.SeverityMedium,
					},
				},
				TotalDuration: 5 * time.Second,
				NodesExecuted: 1,
				NodesFailed:   0,
				NodesSkipped:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize to JSON
			jsonData, err := json.Marshal(tt.result)
			require.NoError(t, err)
			assert.NotEmpty(t, jsonData)

			// Deserialize from JSON
			var decoded WorkflowResult
			err = json.Unmarshal(jsonData, &decoded)
			require.NoError(t, err)

			// Verify round-trip
			assert.Equal(t, tt.result.WorkflowID, decoded.WorkflowID)
			assert.Equal(t, tt.result.Status, decoded.Status)
			assert.Equal(t, len(tt.result.NodeResults), len(decoded.NodeResults))
			assert.Equal(t, tt.result.TotalDuration, decoded.TotalDuration)
			assert.Equal(t, tt.result.NodesExecuted, decoded.NodesExecuted)
			assert.Equal(t, tt.result.NodesFailed, decoded.NodesFailed)
			assert.Equal(t, tt.result.NodesSkipped, decoded.NodesSkipped)

			if tt.result.Error != nil {
				require.NotNil(t, decoded.Error)
				assert.Equal(t, tt.result.Error.Code, decoded.Error.Code)
				assert.Equal(t, tt.result.Error.Message, decoded.Error.Message)
				assert.Equal(t, tt.result.Error.NodeID, decoded.Error.NodeID)
			}

			if tt.result.Findings != nil {
				assert.Equal(t, len(tt.result.Findings), len(decoded.Findings))
			}
		})
	}
}

// TestNodeErrorImplementation tests the Error() method of NodeError
func TestNodeErrorImplementation(t *testing.T) {
	tests := []struct {
		name          string
		nodeError     *NodeError
		expectedError string
	}{
		{
			name: "error without cause",
			nodeError: &NodeError{
				Code:    "TEST_ERROR",
				Message: "Test error message",
			},
			expectedError: "TEST_ERROR: Test error message",
		},
		{
			name: "error with cause",
			nodeError: &NodeError{
				Code:    "TEST_ERROR",
				Message: "Test error message",
				Cause:   errors.New("underlying error"),
			},
			expectedError: "TEST_ERROR: Test error message (caused by: underlying error)",
		},
		{
			name: "error with details but no cause",
			nodeError: &NodeError{
				Code:    "VALIDATION_ERROR",
				Message: "Validation failed",
				Details: map[string]any{
					"field": "name",
					"value": "",
				},
			},
			expectedError: "VALIDATION_ERROR: Validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorMsg := tt.nodeError.Error()
			assert.Equal(t, tt.expectedError, errorMsg)
		})
	}
}

// TestWorkflowErrorImplementation tests the Error() and Unwrap() methods of WorkflowError
func TestWorkflowErrorImplementation(t *testing.T) {
	tests := []struct {
		name          string
		workflowError *WorkflowError
		expectedError string
		hasCause      bool
	}{
		{
			name: "error without node ID or cause",
			workflowError: &WorkflowError{
				Code:    WorkflowErrorInvalidWorkflow,
				Message: "Invalid workflow configuration",
			},
			expectedError: "invalid_workflow: Invalid workflow configuration",
			hasCause:      false,
		},
		{
			name: "error with node ID but no cause",
			workflowError: &WorkflowError{
				Code:    WorkflowErrorNodeExecutionFailed,
				Message: "Node execution failed",
				NodeID:  "node-1",
			},
			expectedError: "node_execution_failed [node: node-1]: Node execution failed",
			hasCause:      false,
		},
		{
			name: "error without node ID but with cause",
			workflowError: &WorkflowError{
				Code:    WorkflowErrorCycleDetected,
				Message: "Cycle detected in workflow",
				Cause:   errors.New("circular dependency"),
			},
			expectedError: "cycle_detected: Cycle detected in workflow (caused by: circular dependency)",
			hasCause:      true,
		},
		{
			name: "error with both node ID and cause",
			workflowError: &WorkflowError{
				Code:    WorkflowErrorNodeTimeout,
				Message: "Node execution timed out",
				NodeID:  "node-2",
				Cause:   errors.New("context deadline exceeded"),
			},
			expectedError: "node_timeout [node: node-2]: Node execution timed out (caused by: context deadline exceeded)",
			hasCause:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Error() method
			errorMsg := tt.workflowError.Error()
			assert.Equal(t, tt.expectedError, errorMsg)

			// Test Unwrap() method
			cause := tt.workflowError.Unwrap()
			if tt.hasCause {
				assert.NotNil(t, cause)
				assert.Equal(t, tt.workflowError.Cause, cause)
			} else {
				assert.Nil(t, cause)
			}
		})
	}
}

// TestWorkflowErrorUnwrap tests that errors.Is and errors.As work correctly with WorkflowError
func TestWorkflowErrorUnwrap(t *testing.T) {
	underlyingErr := errors.New("underlying error")
	workflowErr := &WorkflowError{
		Code:    WorkflowErrorNodeExecutionFailed,
		Message: "Node failed",
		NodeID:  "node-1",
		Cause:   underlyingErr,
	}

	// Test errors.Is
	assert.True(t, errors.Is(workflowErr, underlyingErr))

	// Test errors.As
	var we *WorkflowError
	assert.True(t, errors.As(workflowErr, &we))
	assert.Equal(t, WorkflowErrorNodeExecutionFailed, we.Code)
}

// TestWorkflowErrorCodeValues tests that WorkflowErrorCode constants have correct values
func TestWorkflowErrorCodeValues(t *testing.T) {
	tests := []struct {
		name     string
		code     WorkflowErrorCode
		expected string
	}{
		{
			name:     "cycle detected",
			code:     WorkflowErrorCycleDetected,
			expected: "cycle_detected",
		},
		{
			name:     "missing dependency",
			code:     WorkflowErrorMissingDependency,
			expected: "missing_dependency",
		},
		{
			name:     "deadlock",
			code:     WorkflowErrorDeadlock,
			expected: "deadlock",
		},
		{
			name:     "node execution failed",
			code:     WorkflowErrorNodeExecutionFailed,
			expected: "node_execution_failed",
		},
		{
			name:     "expression invalid",
			code:     WorkflowErrorExpressionInvalid,
			expected: "expression_invalid",
		},
		{
			name:     "workflow cancelled",
			code:     WorkflowErrorWorkflowCancelled,
			expected: "workflow_cancelled",
		},
		{
			name:     "invalid workflow",
			code:     WorkflowErrorInvalidWorkflow,
			expected: "invalid_workflow",
		},
		{
			name:     "node timeout",
			code:     WorkflowErrorNodeTimeout,
			expected: "node_timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.code))
		})
	}
}

// TestRetryPolicyEdgeCases tests edge cases for RetryPolicy.CalculateDelay
func TestRetryPolicyEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		policy        *RetryPolicy
		attempt       int
		expectedDelay time.Duration
	}{
		{
			name: "zero initial delay",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffConstant,
				InitialDelay:    0,
			},
			attempt:       5,
			expectedDelay: 0,
		},
		{
			name: "very large attempt number with linear",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffLinear,
				InitialDelay:    1 * time.Millisecond,
			},
			attempt:       1000,
			expectedDelay: 1001 * time.Millisecond,
		},
		{
			name: "exponential with multiplier 1.0",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffExponential,
				InitialDelay:    100 * time.Millisecond,
				MaxDelay:        10 * time.Second,
				Multiplier:      1.0,
			},
			attempt:       10,
			expectedDelay: 100 * time.Millisecond, // Should stay constant with 1.0 multiplier
		},
		{
			name: "exponential with very high multiplier",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffExponential,
				InitialDelay:    1 * time.Millisecond,
				MaxDelay:        100 * time.Millisecond,
				Multiplier:      10.0,
			},
			attempt:       5,
			expectedDelay: 100 * time.Millisecond, // Should be capped at MaxDelay
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := tt.policy.CalculateDelay(tt.attempt)
			assert.Equal(t, tt.expectedDelay, delay)
		})
	}
}
