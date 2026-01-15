package workflow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockRegistry is a simple mock implementation of the Registry interface for testing
type mockRegistry struct {
	agents  map[string]bool
	tools   map[string]bool
	plugins map[string]bool
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		agents:  make(map[string]bool),
		tools:   make(map[string]bool),
		plugins: make(map[string]bool),
	}
}

func (m *mockRegistry) HasAgent(name string) bool {
	return m.agents[name]
}

func (m *mockRegistry) HasTool(name string) bool {
	return m.tools[name]
}

func (m *mockRegistry) HasPlugin(name string) bool {
	return m.plugins[name]
}

func (m *mockRegistry) addAgent(name string) {
	m.agents[name] = true
}

func (m *mockRegistry) addTool(name string) {
	m.tools[name] = true
}

func (m *mockRegistry) addPlugin(name string) {
	m.plugins[name] = true
}

func TestNewValidator(t *testing.T) {
	t.Run("without registry", func(t *testing.T) {
		v := NewValidator(nil)
		assert.NotNil(t, v)
		assert.Nil(t, v.registry)
	})

	t.Run("with registry", func(t *testing.T) {
		registry := newMockRegistry()
		v := NewValidator(registry)
		assert.NotNil(t, v)
		assert.NotNil(t, v.registry)
	})
}

func TestValidator_ValidateWorkflow_NilWorkflow(t *testing.T) {
	v := NewValidator(nil)
	errors := v.ValidateWorkflow(nil)

	require.Len(t, errors, 1)
	assert.Equal(t, "workflow_nil", errors[0].ErrorCode)
	assert.Contains(t, errors[0].Message, "cannot be nil")
}

func TestValidator_ValidateWorkflow_EmptyWorkflow(t *testing.T) {
	v := NewValidator(nil)
	wf := &Workflow{
		ID:    types.NewID(),
		Name:  "test",
		Nodes: map[string]*WorkflowNode{},
	}

	errors := v.ValidateWorkflow(wf)
	require.Len(t, errors, 1)
	assert.Equal(t, "workflow_empty", errors[0].ErrorCode)
}

func TestValidator_ValidateWorkflow_MissingWorkflowName(t *testing.T) {
	v := NewValidator(nil)
	wf := &Workflow{
		ID:   types.NewID(),
		Name: "", // Missing name
		Nodes: map[string]*WorkflowNode{
			"node1": {
				ID:        "node1",
				Type:      NodeTypeAgent,
				Name:      "Node 1",
				AgentName: "test-agent",
			},
		},
	}

	errors := v.ValidateWorkflow(wf)
	require.NotEmpty(t, errors)

	hasNameError := false
	for _, err := range errors {
		if err.ErrorCode == "workflow_name_required" {
			hasNameError = true
			break
		}
	}
	assert.True(t, hasNameError, "expected workflow_name_required error")
}

func TestValidator_ValidateWorkflow_ValidSimpleWorkflow(t *testing.T) {
	v := NewValidator(nil)
	wf := &Workflow{
		ID:   types.NewID(),
		Name: "test-workflow",
		Nodes: map[string]*WorkflowNode{
			"node1": {
				ID:        "node1",
				Type:      NodeTypeAgent,
				Name:      "Node 1",
				AgentName: "test-agent",
			},
		},
	}

	errors := v.ValidateWorkflow(wf)
	assert.Empty(t, errors, "expected no validation errors for valid workflow")
}

func TestValidator_ValidateNodes_MissingRequiredFields(t *testing.T) {
	v := NewValidator(nil)

	tests := []struct {
		name          string
		node          *WorkflowNode
		expectedError string
	}{
		{
			name: "missing node ID",
			node: &WorkflowNode{
				ID:        "",
				Type:      NodeTypeAgent,
				Name:      "Test",
				AgentName: "agent",
			},
			expectedError: "node_id_required",
		},
		{
			name: "missing node type",
			node: &WorkflowNode{
				ID:   "node1",
				Type: "",
				Name: "Test",
			},
			expectedError: "node_type_required",
		},
		{
			name: "missing node name",
			node: &WorkflowNode{
				ID:        "node1",
				Type:      NodeTypeAgent,
				Name:      "",
				AgentName: "agent",
			},
			expectedError: "node_name_required",
		},
		{
			name: "agent node missing agent name",
			node: &WorkflowNode{
				ID:        "node1",
				Type:      NodeTypeAgent,
				Name:      "Test",
				AgentName: "",
			},
			expectedError: "agent_name_required",
		},
		{
			name: "tool node missing tool name",
			node: &WorkflowNode{
				ID:       "node1",
				Type:     NodeTypeTool,
				Name:     "Test",
				ToolName: "",
			},
			expectedError: "tool_name_required",
		},
		{
			name: "plugin node missing plugin name",
			node: &WorkflowNode{
				ID:           "node1",
				Type:         NodeTypePlugin,
				Name:         "Test",
				PluginName:   "",
				PluginMethod: "method",
			},
			expectedError: "plugin_name_required",
		},
		{
			name: "plugin node missing plugin method",
			node: &WorkflowNode{
				ID:           "node1",
				Type:         NodeTypePlugin,
				Name:         "Test",
				PluginName:   "plugin",
				PluginMethod: "",
			},
			expectedError: "plugin_method_required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := &Workflow{
				ID:   types.NewID(),
				Name: "test",
				Nodes: map[string]*WorkflowNode{
					"node1": tt.node,
				},
			}

			errors := v.ValidateWorkflow(wf)
			require.NotEmpty(t, errors)

			hasExpectedError := false
			for _, err := range errors {
				if err.ErrorCode == tt.expectedError {
					hasExpectedError = true
					break
				}
			}
			assert.True(t, hasExpectedError, "expected error code %s", tt.expectedError)
		})
	}
}

func TestValidator_ValidateNodes_ConditionNode(t *testing.T) {
	v := NewValidator(nil)

	t.Run("missing condition", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:        "node1",
					Type:      NodeTypeCondition,
					Name:      "Condition",
					Condition: nil,
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "condition_required" {
				hasError = true
				break
			}
		}
		assert.True(t, hasError)
	})

	t.Run("missing expression", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:   "node1",
					Type: NodeTypeCondition,
					Name: "Condition",
					Condition: &NodeCondition{
						Expression: "",
					},
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "condition_expression_required" {
				hasError = true
				break
			}
		}
		assert.True(t, hasError)
	})

	t.Run("invalid branch reference", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:   "node1",
					Type: NodeTypeCondition,
					Name: "Condition",
					Condition: &NodeCondition{
						Expression:  "result.success",
						TrueBranch:  []string{"nonexistent"},
						FalseBranch: []string{"node2"},
					},
				},
				"node2": {
					ID:        "node2",
					Type:      NodeTypeAgent,
					Name:      "Node 2",
					AgentName: "agent",
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "invalid_branch_reference" {
				hasError = true
				break
			}
		}
		assert.True(t, hasError)
	})
}

func TestValidator_ValidateNodes_ParallelNode(t *testing.T) {
	v := NewValidator(nil)

	t.Run("missing sub-nodes", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:       "node1",
					Type:     NodeTypeParallel,
					Name:     "Parallel",
					SubNodes: []*WorkflowNode{},
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "sub_nodes_required" {
				hasError = true
				break
			}
		}
		assert.True(t, hasError)
	})

	t.Run("nil sub-node", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:       "node1",
					Type:     NodeTypeParallel,
					Name:     "Parallel",
					SubNodes: []*WorkflowNode{nil},
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "sub_node_nil" {
				hasError = true
				break
			}
		}
		assert.True(t, hasError)
	})

	t.Run("sub-node missing ID", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:   "node1",
					Type: NodeTypeParallel,
					Name: "Parallel",
					SubNodes: []*WorkflowNode{
						{
							ID:   "",
							Type: NodeTypeAgent,
							Name: "Sub",
						},
					},
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "sub_node_id_required" {
				hasError = true
				break
			}
		}
		assert.True(t, hasError)
	})
}

func TestValidator_ValidateNodes_InvalidTimeout(t *testing.T) {
	v := NewValidator(nil)
	wf := &Workflow{
		ID:   types.NewID(),
		Name: "test",
		Nodes: map[string]*WorkflowNode{
			"node1": {
				ID:        "node1",
				Type:      NodeTypeAgent,
				Name:      "Test",
				AgentName: "agent",
				Timeout:   -5 * time.Second,
			},
		},
	}

	errors := v.ValidateWorkflow(wf)
	require.NotEmpty(t, errors)

	hasError := false
	for _, err := range errors {
		if err.ErrorCode == "timeout_invalid" {
			hasError = true
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidator_ValidateRetryPolicy(t *testing.T) {
	v := NewValidator(nil)

	tests := []struct {
		name          string
		policy        *RetryPolicy
		expectedError string
	}{
		{
			name: "negative max retries",
			policy: &RetryPolicy{
				MaxRetries:      -1,
				BackoffStrategy: BackoffConstant,
				InitialDelay:    time.Second,
			},
			expectedError: "max_retries_invalid",
		},
		{
			name: "invalid backoff strategy",
			policy: &RetryPolicy{
				MaxRetries:      3,
				BackoffStrategy: "invalid",
				InitialDelay:    time.Second,
			},
			expectedError: "backoff_strategy_invalid",
		},
		{
			name: "negative initial delay",
			policy: &RetryPolicy{
				MaxRetries:      3,
				BackoffStrategy: BackoffConstant,
				InitialDelay:    -time.Second,
			},
			expectedError: "initial_delay_invalid",
		},
		{
			name: "negative max delay",
			policy: &RetryPolicy{
				MaxRetries:      3,
				BackoffStrategy: BackoffExponential,
				InitialDelay:    time.Second,
				MaxDelay:        -time.Second,
				Multiplier:      2.0,
			},
			expectedError: "max_delay_invalid",
		},
		{
			name: "max delay less than initial delay",
			policy: &RetryPolicy{
				MaxRetries:      3,
				BackoffStrategy: BackoffConstant,
				InitialDelay:    10 * time.Second,
				MaxDelay:        5 * time.Second,
			},
			expectedError: "max_delay_invalid",
		},
		{
			name: "exponential backoff missing multiplier",
			policy: &RetryPolicy{
				MaxRetries:      3,
				BackoffStrategy: BackoffExponential,
				InitialDelay:    time.Second,
				MaxDelay:        10 * time.Second,
				Multiplier:      0,
			},
			expectedError: "multiplier_invalid",
		},
		{
			name: "exponential backoff missing max delay",
			policy: &RetryPolicy{
				MaxRetries:      3,
				BackoffStrategy: BackoffExponential,
				InitialDelay:    time.Second,
				MaxDelay:        0,
				Multiplier:      2.0,
			},
			expectedError: "max_delay_required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := &Workflow{
				ID:   types.NewID(),
				Name: "test",
				Nodes: map[string]*WorkflowNode{
					"node1": {
						ID:          "node1",
						Type:        NodeTypeAgent,
						Name:        "Test",
						AgentName:   "agent",
						RetryPolicy: tt.policy,
					},
				},
			}

			errors := v.ValidateWorkflow(wf)
			require.NotEmpty(t, errors)

			hasError := false
			for _, err := range errors {
				if err.ErrorCode == tt.expectedError {
					hasError = true
					break
				}
			}
			assert.True(t, hasError, "expected error code %s", tt.expectedError)
		})
	}
}

func TestValidator_ValidateNodeReferences_MissingDependency(t *testing.T) {
	v := NewValidator(nil)
	wf := &Workflow{
		ID:   types.NewID(),
		Name: "test",
		Nodes: map[string]*WorkflowNode{
			"node1": {
				ID:           "node1",
				Type:         NodeTypeAgent,
				Name:         "Test",
				AgentName:    "agent",
				Dependencies: []string{"nonexistent"},
			},
		},
	}

	errors := v.ValidateWorkflow(wf)
	require.NotEmpty(t, errors)

	hasError := false
	for _, err := range errors {
		if err.ErrorCode == "missing_dependency" {
			hasError = true
			assert.Equal(t, "node1", err.NodeID)
			break
		}
	}
	assert.True(t, hasError)
}

func TestValidator_ValidateNodeReferences_InvalidEdges(t *testing.T) {
	v := NewValidator(nil)

	t.Run("edge with missing source", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:        "node1",
					Type:      NodeTypeAgent,
					Name:      "Test",
					AgentName: "agent",
				},
			},
			Edges: []WorkflowEdge{
				{From: "nonexistent", To: "node1"},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "missing_edge_source" {
				hasError = true
				break
			}
		}
		assert.True(t, hasError)
	})

	t.Run("edge with missing destination", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:        "node1",
					Type:      NodeTypeAgent,
					Name:      "Test",
					AgentName: "agent",
				},
			},
			Edges: []WorkflowEdge{
				{From: "node1", To: "nonexistent"},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "missing_edge_destination" {
				hasError = true
				break
			}
		}
		assert.True(t, hasError)
	})
}

func TestValidator_ValidateDAGAcyclic(t *testing.T) {
	v := NewValidator(nil)

	t.Run("detects cycle", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:           "node1",
					Type:         NodeTypeAgent,
					Name:         "Node 1",
					AgentName:    "agent",
					Dependencies: []string{"node2"},
				},
				"node2": {
					ID:           "node2",
					Type:         NodeTypeAgent,
					Name:         "Node 2",
					AgentName:    "agent",
					Dependencies: []string{"node1"},
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "cycle_detected" {
				hasError = true
				break
			}
		}
		assert.True(t, hasError)
	})

	t.Run("valid DAG", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:        "node1",
					Type:      NodeTypeAgent,
					Name:      "Node 1",
					AgentName: "agent",
				},
				"node2": {
					ID:           "node2",
					Type:         NodeTypeAgent,
					Name:         "Node 2",
					AgentName:    "agent",
					Dependencies: []string{"node1"},
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		assert.Empty(t, errors)
	})
}

func TestValidator_ValidateAgentToolRegistry(t *testing.T) {
	registry := newMockRegistry()
	registry.addAgent("existing-agent")
	registry.addTool("existing-tool")
	registry.addPlugin("existing-plugin")

	v := NewValidator(registry)

	t.Run("missing agent", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:        "node1",
					Type:      NodeTypeAgent,
					Name:      "Test",
					AgentName: "nonexistent-agent",
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "agent_not_found" {
				hasError = true
				assert.Contains(t, err.Message, "nonexistent-agent")
				break
			}
		}
		assert.True(t, hasError)
	})

	t.Run("missing tool", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:       "node1",
					Type:     NodeTypeTool,
					Name:     "Test",
					ToolName: "nonexistent-tool",
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "tool_not_found" {
				hasError = true
				assert.Contains(t, err.Message, "nonexistent-tool")
				break
			}
		}
		assert.True(t, hasError)
	})

	t.Run("missing plugin", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:           "node1",
					Type:         NodeTypePlugin,
					Name:         "Test",
					PluginName:   "nonexistent-plugin",
					PluginMethod: "method",
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		require.NotEmpty(t, errors)

		hasError := false
		for _, err := range errors {
			if err.ErrorCode == "plugin_not_found" {
				hasError = true
				assert.Contains(t, err.Message, "nonexistent-plugin")
				break
			}
		}
		assert.True(t, hasError)
	})

	t.Run("existing components", func(t *testing.T) {
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:        "node1",
					Type:      NodeTypeAgent,
					Name:      "Agent Node",
					AgentName: "existing-agent",
				},
				"node2": {
					ID:       "node2",
					Type:     NodeTypeTool,
					Name:     "Tool Node",
					ToolName: "existing-tool",
				},
				"node3": {
					ID:           "node3",
					Type:         NodeTypePlugin,
					Name:         "Plugin Node",
					PluginName:   "existing-plugin",
					PluginMethod: "method",
				},
			},
		}

		errors := v.ValidateWorkflow(wf)
		assert.Empty(t, errors)
	})

	t.Run("validator without registry skips registry check", func(t *testing.T) {
		vNoRegistry := NewValidator(nil)
		wf := &Workflow{
			ID:   types.NewID(),
			Name: "test",
			Nodes: map[string]*WorkflowNode{
				"node1": {
					ID:        "node1",
					Type:      NodeTypeAgent,
					Name:      "Test",
					AgentName: "nonexistent-agent",
				},
			},
		}

		errors := vNoRegistry.ValidateWorkflow(wf)
		// Should not have agent_not_found error since no registry provided
		for _, err := range errors {
			assert.NotEqual(t, "agent_not_found", err.ErrorCode)
		}
	})
}

func TestValidator_MultipleErrors(t *testing.T) {
	v := NewValidator(nil)
	wf := &Workflow{
		ID:   types.NewID(),
		Name: "", // Error 1: missing name
		Nodes: map[string]*WorkflowNode{
			"node1": {
				ID:           "node1",
				Type:         NodeTypeAgent,
				Name:         "",         // Error 2: missing name
				AgentName:    "",         // Error 3: missing agent name
				Dependencies: []string{"nonexistent"}, // Error 4: missing dependency
				Timeout:      -5 * time.Second,        // Error 5: negative timeout
			},
		},
	}

	errors := v.ValidateWorkflow(wf)
	assert.GreaterOrEqual(t, len(errors), 4, "expected multiple validation errors")
}

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name: "with all fields",
			err: ValidationError{
				NodeID:    "node1",
				Field:     "timeout",
				Message:   "invalid timeout",
				Line:      42,
				ErrorCode: "timeout_invalid",
			},
			expected: "timeout_invalid [node=node1, field=timeout, line=42]: invalid timeout",
		},
		{
			name: "without line number",
			err: ValidationError{
				NodeID:    "node1",
				Field:     "name",
				Message:   "name is required",
				ErrorCode: "name_required",
			},
			expected: "name_required [node=node1, field=name]: name is required",
		},
		{
			name: "without node ID",
			err: ValidationError{
				Field:     "nodes",
				Message:   "workflow must have nodes",
				ErrorCode: "empty_workflow",
			},
			expected: "empty_workflow [field=nodes]: workflow must have nodes",
		},
		{
			name: "minimal error",
			err: ValidationError{
				Message:   "something went wrong",
				ErrorCode: "error",
			},
			expected: "error: something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestValidationErrors_Error(t *testing.T) {
	t.Run("empty errors", func(t *testing.T) {
		errors := ValidationErrors{}
		assert.Equal(t, "", errors.Error())
	})

	t.Run("single error", func(t *testing.T) {
		errors := ValidationErrors{
			{ErrorCode: "test_error", Message: "test message"},
		}
		assert.Equal(t, "test_error: test message", errors.Error())
	})

	t.Run("multiple errors", func(t *testing.T) {
		errors := ValidationErrors{
			{ErrorCode: "error1", Message: "first error"},
			{ErrorCode: "error2", Message: "second error"},
			{ErrorCode: "error3", Message: "third error"},
		}
		result := errors.Error()
		assert.Contains(t, result, "3 validation errors:")
		assert.Contains(t, result, "1. error1: first error")
		assert.Contains(t, result, "2. error2: second error")
		assert.Contains(t, result, "3. error3: third error")
	})
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  time.Duration
		expectErr bool
	}{
		{
			name:      "empty string",
			input:     "",
			expected:  0,
			expectErr: false,
		},
		{
			name:      "valid seconds",
			input:     "5s",
			expected:  5 * time.Second,
			expectErr: false,
		},
		{
			name:      "valid minutes",
			input:     "10m",
			expected:  10 * time.Minute,
			expectErr: false,
		},
		{
			name:      "valid hours",
			input:     "2h",
			expected:  2 * time.Hour,
			expectErr: false,
		},
		{
			name:      "valid milliseconds",
			input:     "500ms",
			expected:  500 * time.Millisecond,
			expectErr: false,
		},
		{
			name:      "invalid format",
			input:     "invalid",
			expected:  0,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDuration(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestValidator_ComplexWorkflow(t *testing.T) {
	registry := newMockRegistry()
	registry.addAgent("scanner")
	registry.addAgent("exploiter")
	registry.addTool("port-scan")
	registry.addPlugin("reporter")

	v := NewValidator(registry)

	wf := &Workflow{
		ID:   types.NewID(),
		Name: "complex-test-workflow",
		Nodes: map[string]*WorkflowNode{
			"scan": {
				ID:        "scan",
				Type:      NodeTypeAgent,
				Name:      "Port Scanner",
				AgentName: "scanner",
				Timeout:   30 * time.Second,
				RetryPolicy: &RetryPolicy{
					MaxRetries:      3,
					BackoffStrategy: BackoffExponential,
					InitialDelay:    time.Second,
					MaxDelay:        30 * time.Second,
					Multiplier:      2.0,
				},
			},
			"check": {
				ID:           "check",
				Type:         NodeTypeCondition,
				Name:         "Check Results",
				Dependencies: []string{"scan"},
				Condition: &NodeCondition{
					Expression:  "result.success == true",
					TrueBranch:  []string{"exploit"},
					FalseBranch: []string{"report"},
				},
			},
			"exploit": {
				ID:           "exploit",
				Type:         NodeTypeAgent,
				Name:         "Exploit",
				AgentName:    "exploiter",
				Dependencies: []string{"check"},
			},
			"report": {
				ID:           "report",
				Type:         NodeTypePlugin,
				Name:         "Generate Report",
				PluginName:   "reporter",
				PluginMethod: "generate",
				Dependencies: []string{"check"},
			},
		},
	}

	errors := v.ValidateWorkflow(wf)
	assert.Empty(t, errors, "expected no validation errors for valid complex workflow")
}
