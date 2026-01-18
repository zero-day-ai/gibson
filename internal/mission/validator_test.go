package mission

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_NilDefinition(t *testing.T) {
	errors := Validate(nil)
	require.Len(t, errors, 1)
	assert.Equal(t, "definition_nil", errors[0].ErrorCode)
}

func TestValidate_EmptyName(t *testing.T) {
	def := &MissionDefinition{
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:   "node1",
				Type: NodeTypeAgent,
			},
		},
	}

	errors := Validate(def)
	require.NotEmpty(t, errors)

	// Find the name_required error
	var found bool
	for _, err := range errors {
		if err.ErrorCode == "name_required" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have name_required error")
}

func TestValidate_EmptyNodes(t *testing.T) {
	def := &MissionDefinition{
		Name:  "test-mission",
		Nodes: map[string]*MissionNode{},
	}

	errors := Validate(def)
	require.Len(t, errors, 1)
	assert.Equal(t, "empty_definition", errors[0].ErrorCode)
}

func TestValidate_ValidVersion(t *testing.T) {
	testCases := []struct {
		name    string
		version string
		valid   bool
	}{
		{"simple semver", "1.0.0", true},
		{"with patch", "1.2.3", true},
		{"with pre-release", "1.0.0-alpha", true},
		{"with build metadata", "1.0.0+build.123", true},
		{"full version", "1.2.3-rc.1+build.456", true},
		{"invalid format", "1.0", false},
		{"invalid format v prefix", "v1.0.0", false},
		{"invalid characters", "1.0.0a", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			def := &MissionDefinition{
				Name:    "test-mission",
				Version: tc.version,
				Nodes: map[string]*MissionNode{
					"node1": {
						ID:        "node1",
						Type:      NodeTypeAgent,
						AgentName: "test-agent",
					},
				},
			}

			errors := Validate(def)

			hasVersionError := false
			for _, err := range errors {
				if err.ErrorCode == "version_format_invalid" {
					hasVersionError = true
					break
				}
			}

			if tc.valid {
				assert.False(t, hasVersionError, "should not have version error for valid version: %s", tc.version)
			} else {
				assert.True(t, hasVersionError, "should have version error for invalid version: %s", tc.version)
			}
		})
	}
}

func TestValidate_NodeTypes(t *testing.T) {
	testCases := []struct {
		name      string
		nodeType  NodeType
		valid     bool
		extraNode *MissionNode
	}{
		{"agent type", NodeTypeAgent, true, &MissionNode{ID: "node1", Type: NodeTypeAgent, AgentName: "test"}},
		{"tool type", NodeTypeTool, true, &MissionNode{ID: "node1", Type: NodeTypeTool, ToolName: "test"}},
		{"plugin type", NodeTypePlugin, true, &MissionNode{ID: "node1", Type: NodeTypePlugin, PluginName: "test", PluginMethod: "query"}},
		{"condition type", NodeTypeCondition, true, &MissionNode{ID: "node1", Type: NodeTypeCondition, Condition: &NodeCondition{Expression: "true"}}},
		{"parallel type", NodeTypeParallel, true, &MissionNode{ID: "node1", Type: NodeTypeParallel, SubNodes: []*MissionNode{{ID: "sub1", Type: NodeTypeAgent, AgentName: "test"}}}},
		{"join type", NodeTypeJoin, true, &MissionNode{ID: "node1", Type: NodeTypeJoin}},
		{"invalid type", NodeType("invalid"), false, &MissionNode{ID: "node1", Type: NodeType("invalid")}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			def := &MissionDefinition{
				Name: "test-mission",
				Nodes: map[string]*MissionNode{
					"node1": tc.extraNode,
				},
			}

			errors := Validate(def)

			hasInvalidTypeError := false
			for _, err := range errors {
				if err.ErrorCode == "node_type_invalid" {
					hasInvalidTypeError = true
					break
				}
			}

			if tc.valid {
				assert.False(t, hasInvalidTypeError, "should not have invalid type error for: %s", tc.nodeType)
			} else {
				assert.True(t, hasInvalidTypeError, "should have invalid type error for: %s", tc.nodeType)
			}
		})
	}
}

func TestValidate_AgentNodeRequiredFields(t *testing.T) {
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:   "node1",
				Type: NodeTypeAgent,
				// Missing AgentName
			},
		},
	}

	errors := Validate(def)

	var found bool
	for _, err := range errors {
		if err.ErrorCode == "agent_name_required" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have agent_name_required error")
}

func TestValidate_ToolNodeRequiredFields(t *testing.T) {
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:   "node1",
				Type: NodeTypeTool,
				// Missing ToolName
			},
		},
	}

	errors := Validate(def)

	var found bool
	for _, err := range errors {
		if err.ErrorCode == "tool_name_required" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have tool_name_required error")
}

func TestValidate_PluginNodeRequiredFields(t *testing.T) {
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:         "node1",
				Type:       NodeTypePlugin,
				PluginName: "test-plugin",
				// Missing PluginMethod
			},
		},
	}

	errors := Validate(def)

	var found bool
	for _, err := range errors {
		if err.ErrorCode == "plugin_method_required" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have plugin_method_required error")
}

func TestValidate_ConditionNodeRequiredFields(t *testing.T) {
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:   "node1",
				Type: NodeTypeCondition,
				// Missing Condition
			},
		},
	}

	errors := Validate(def)

	var found bool
	for _, err := range errors {
		if err.ErrorCode == "condition_required" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have condition_required error")
}

func TestValidate_ParallelNodeRequiredFields(t *testing.T) {
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:   "node1",
				Type: NodeTypeParallel,
				// Missing SubNodes
			},
		},
	}

	errors := Validate(def)

	var found bool
	for _, err := range errors {
		if err.ErrorCode == "sub_nodes_required" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have sub_nodes_required error")
}

func TestValidate_EdgeReferences(t *testing.T) {
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:        "node1",
				Type:      NodeTypeAgent,
				AgentName: "test",
			},
		},
		Edges: []MissionEdge{
			{From: "node1", To: "nonexistent"},
		},
	}

	errors := Validate(def)

	var found bool
	for _, err := range errors {
		if err.ErrorCode == "missing_edge_destination" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have missing_edge_destination error")
}

func TestValidate_DependencyReferences(t *testing.T) {
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:           "node1",
				Type:         NodeTypeAgent,
				AgentName:    "test",
				Dependencies: []string{"nonexistent"},
			},
		},
	}

	errors := Validate(def)

	var found bool
	for _, err := range errors {
		if err.ErrorCode == "missing_dependency" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have missing_dependency error")
}

func TestValidate_CircularDependency(t *testing.T) {
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:           "node1",
				Type:         NodeTypeAgent,
				AgentName:    "test",
				Dependencies: []string{"node2"},
			},
			"node2": {
				ID:           "node2",
				Type:         NodeTypeAgent,
				AgentName:    "test",
				Dependencies: []string{"node1"},
			},
		},
	}

	errors := Validate(def)

	var found bool
	for _, err := range errors {
		if err.ErrorCode == "cycle_detected" {
			found = true
			assert.Contains(t, err.Message, "circular dependency")
			break
		}
	}
	assert.True(t, found, "should have cycle_detected error")
}

func TestValidate_CircularDependencyViaEdges(t *testing.T) {
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:        "node1",
				Type:      NodeTypeAgent,
				AgentName: "test",
			},
			"node2": {
				ID:        "node2",
				Type:      NodeTypeAgent,
				AgentName: "test",
			},
		},
		Edges: []MissionEdge{
			{From: "node1", To: "node2"},
			{From: "node2", To: "node1"},
		},
	}

	errors := Validate(def)

	var found bool
	for _, err := range errors {
		if err.ErrorCode == "cycle_detected" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have cycle_detected error")
}

func TestValidate_ValidRetryPolicy(t *testing.T) {
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:        "node1",
				Type:      NodeTypeAgent,
				AgentName: "test",
				RetryPolicy: &RetryPolicy{
					MaxRetries:      3,
					BackoffStrategy: BackoffExponential,
					InitialDelay:    1 * time.Second,
					MaxDelay:        30 * time.Second,
					Multiplier:      2.0,
				},
			},
		},
	}

	errors := Validate(def)

	// Should not have retry policy errors
	for _, err := range errors {
		assert.NotContains(t, err.ErrorCode, "retry_policy", "should not have retry policy errors")
	}
}

func TestValidate_InvalidRetryPolicy(t *testing.T) {
	testCases := []struct {
		name        string
		policy      *RetryPolicy
		expectedErr string
	}{
		{
			name: "negative max retries",
			policy: &RetryPolicy{
				MaxRetries: -1,
			},
			expectedErr: "max_retries_invalid",
		},
		{
			name: "invalid backoff strategy",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffStrategy("invalid"),
			},
			expectedErr: "backoff_strategy_invalid",
		},
		{
			name: "negative initial delay",
			policy: &RetryPolicy{
				InitialDelay: -1 * time.Second,
			},
			expectedErr: "initial_delay_invalid",
		},
		{
			name: "max delay less than initial delay",
			policy: &RetryPolicy{
				InitialDelay: 10 * time.Second,
				MaxDelay:     5 * time.Second,
			},
			expectedErr: "max_delay_invalid",
		},
		{
			name: "exponential backoff missing multiplier",
			policy: &RetryPolicy{
				BackoffStrategy: BackoffExponential,
				InitialDelay:    1 * time.Second,
				MaxDelay:        30 * time.Second,
				Multiplier:      0,
			},
			expectedErr: "multiplier_invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			def := &MissionDefinition{
				Name: "test-mission",
				Nodes: map[string]*MissionNode{
					"node1": {
						ID:          "node1",
						Type:        NodeTypeAgent,
						AgentName:   "test",
						RetryPolicy: tc.policy,
					},
				},
			}

			errors := Validate(def)

			var found bool
			for _, err := range errors {
				if err.ErrorCode == tc.expectedErr {
					found = true
					break
				}
			}
			assert.True(t, found, "should have error: %s", tc.expectedErr)
		})
	}
}

func TestValidate_ValidMission(t *testing.T) {
	def := &MissionDefinition{
		Name:    "test-mission",
		Version: "1.0.0",
		Nodes: map[string]*MissionNode{
			"recon": {
				ID:        "recon",
				Type:      NodeTypeAgent,
				AgentName: "network-recon",
			},
			"scan": {
				ID:           "scan",
				Type:         NodeTypeAgent,
				AgentName:    "port-scanner",
				Dependencies: []string{"recon"},
			},
		},
	}

	errors := Validate(def)
	assert.Empty(t, errors, "valid mission should have no errors")
}

func TestValidationError_Error(t *testing.T) {
	err := ValidationError{
		NodeID:    "node1",
		Field:     "agent_name",
		Message:   "agent name is required",
		ErrorCode: "agent_name_required",
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "node=node1")
	assert.Contains(t, errStr, "field=agent_name")
	assert.Contains(t, errStr, "agent_name_required")
	assert.Contains(t, errStr, "agent name is required")
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
		errStr := errors.Error()
		assert.Contains(t, errStr, "test_error")
		assert.Contains(t, errStr, "test message")
	})

	t.Run("multiple errors", func(t *testing.T) {
		errors := ValidationErrors{
			{ErrorCode: "error1", Message: "first error"},
			{ErrorCode: "error2", Message: "second error"},
		}
		errStr := errors.Error()
		assert.Contains(t, errStr, "2 validation errors")
		assert.Contains(t, errStr, "error1")
		assert.Contains(t, errStr, "error2")
	})
}

func TestValidate_ConditionBranchReferences(t *testing.T) {
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"condition": {
				ID:   "condition",
				Type: NodeTypeCondition,
				Condition: &NodeCondition{
					Expression:  "result.status == 'success'",
					TrueBranch:  []string{"success-node"},
					FalseBranch: []string{"failure-node"},
				},
			},
			"success-node": {
				ID:        "success-node",
				Type:      NodeTypeAgent,
				AgentName: "success-handler",
			},
		},
	}

	errors := Validate(def)

	var found bool
	for _, err := range errors {
		if err.ErrorCode == "invalid_branch_reference" && err.Field == "condition.false_branch" {
			found = true
			assert.Contains(t, err.Message, "failure-node")
			break
		}
	}
	assert.True(t, found, "should have invalid_branch_reference error for false branch")
}
