package mission

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestParseDefinitionFromJSON_RoundTrip tests that a mission definition can be
// marshaled to JSON and then parsed back with all fields intact
func TestParseDefinitionFromJSON_RoundTrip(t *testing.T) {
	original := &MissionDefinition{
		ID:          types.NewID(),
		Name:        "test-mission",
		Description: "Test mission for round-trip serialization",
		Version:     "1.0.0",
		TargetRef:   "test-target",
		Nodes: map[string]*MissionNode{
			"agent1": {
				ID:        "agent1",
				Type:      NodeTypeAgent,
				Name:      "Test Agent",
				AgentName: "test-agent",
				Timeout:   10 * time.Minute,
				AgentTask: &agent.Task{
					Name:        "test-task",
					Description: "Test agent task",
					Context: map[string]any{
						"target": "example.com",
					},
				},
			},
			"tool1": {
				ID:       "tool1",
				Type:     NodeTypeTool,
				Name:     "Test Tool",
				ToolName: "test-tool",
				ToolInput: map[string]any{
					"param1": "value1",
					"param2": 42,
				},
				Timeout: 5 * time.Minute,
			},
			"plugin1": {
				ID:           "plugin1",
				Type:         NodeTypePlugin,
				Name:         "Test Plugin",
				PluginName:   "test-plugin",
				PluginMethod: "query",
				PluginParams: map[string]any{
					"query": "SELECT * FROM test",
				},
			},
		},
		Edges: []MissionEdge{
			{From: "agent1", To: "tool1"},
			{From: "tool1", To: "plugin1"},
		},
		EntryPoints: []string{"agent1"},
		ExitPoints:  []string{"plugin1"},
		Metadata: map[string]any{
			"author": "test",
			"tags":   []string{"test", "integration"},
		},
		CreatedAt: time.Now().UTC(),
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Parse back
	parsed, err := ParseDefinitionFromJSON(data)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Verify core fields
	assert.Equal(t, original.ID, parsed.ID)
	assert.Equal(t, original.Name, parsed.Name)
	assert.Equal(t, original.Description, parsed.Description)
	assert.Equal(t, original.Version, parsed.Version)
	assert.Equal(t, original.TargetRef, parsed.TargetRef)

	// Verify nodes
	assert.Len(t, parsed.Nodes, 3)

	// Verify agent node
	agent1 := parsed.Nodes["agent1"]
	require.NotNil(t, agent1)
	assert.Equal(t, "agent1", agent1.ID)
	assert.Equal(t, NodeTypeAgent, agent1.Type)
	assert.Equal(t, "test-agent", agent1.AgentName)
	assert.Equal(t, 10*time.Minute, agent1.Timeout)

	// Verify tool node
	tool1 := parsed.Nodes["tool1"]
	require.NotNil(t, tool1)
	assert.Equal(t, "tool1", tool1.ID)
	assert.Equal(t, NodeTypeTool, tool1.Type)
	assert.Equal(t, "test-tool", tool1.ToolName)
	assert.Equal(t, 5*time.Minute, tool1.Timeout)

	// Verify plugin node
	plugin1 := parsed.Nodes["plugin1"]
	require.NotNil(t, plugin1)
	assert.Equal(t, "plugin1", plugin1.ID)
	assert.Equal(t, NodeTypePlugin, plugin1.Type)
	assert.Equal(t, "test-plugin", plugin1.PluginName)
	assert.Equal(t, "query", plugin1.PluginMethod)

	// Verify edges
	assert.Len(t, parsed.Edges, 2)
	assert.Equal(t, original.Edges, parsed.Edges)

	// Verify entry/exit points
	assert.Equal(t, original.EntryPoints, parsed.EntryPoints)
	assert.Equal(t, original.ExitPoints, parsed.ExitPoints)
}

// TestParseDefinitionFromJSON_DurationHandling tests that time.Duration fields
// serialize as integer nanoseconds and deserialize correctly
func TestParseDefinitionFromJSON_DurationHandling(t *testing.T) {
	// Create JSON with numeric timeout (nanoseconds)
	jsonData := `{
		"name": "duration-test",
		"nodes": {
			"n1": {
				"id": "n1",
				"type": "agent",
				"agent_name": "test-agent",
				"timeout": 600000000000
			}
		}
	}`

	parsed, err := ParseDefinitionFromJSON([]byte(jsonData))
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Verify timeout is correctly parsed as 10 minutes
	node := parsed.Nodes["n1"]
	require.NotNil(t, node)
	assert.Equal(t, 10*time.Minute, node.Timeout)
}

// TestParseDefinitionFromJSON_RetryPolicyDurations tests that retry policy
// duration fields serialize/deserialize correctly
func TestParseDefinitionFromJSON_RetryPolicyDurations(t *testing.T) {
	original := &MissionDefinition{
		Name: "retry-test",
		Nodes: map[string]*MissionNode{
			"n1": {
				ID:        "n1",
				Type:      NodeTypeAgent,
				AgentName: "test-agent",
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

	// Marshal and parse
	data, err := json.Marshal(original)
	require.NoError(t, err)

	parsed, err := ParseDefinitionFromJSON(data)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Verify retry policy durations
	node := parsed.Nodes["n1"]
	require.NotNil(t, node)
	require.NotNil(t, node.RetryPolicy)
	assert.Equal(t, 3, node.RetryPolicy.MaxRetries)
	assert.Equal(t, BackoffExponential, node.RetryPolicy.BackoffStrategy)
	assert.Equal(t, 1*time.Second, node.RetryPolicy.InitialDelay)
	assert.Equal(t, 30*time.Second, node.RetryPolicy.MaxDelay)
	assert.Equal(t, 2.0, node.RetryPolicy.Multiplier)
}

// TestParseDefinitionFromJSON_MissingName tests that parsing fails when name is missing
func TestParseDefinitionFromJSON_MissingName(t *testing.T) {
	jsonData := `{
		"nodes": {
			"n1": {
				"id": "n1",
				"type": "agent",
				"agent_name": "test-agent"
			}
		}
	}`

	_, err := ParseDefinitionFromJSON([]byte(jsonData))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

// TestParseDefinitionFromJSON_AgentMissingAgentName tests that validation
// catches agent nodes without agent_name
func TestParseDefinitionFromJSON_AgentMissingAgentName(t *testing.T) {
	jsonData := `{
		"name": "test-mission",
		"nodes": {
			"n1": {
				"id": "n1",
				"type": "agent"
			}
		}
	}`

	_, err := ParseDefinitionFromJSON([]byte(jsonData))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent node n1 requires 'agent_name' field")
}

// TestParseDefinitionFromJSON_ToolMissingToolName tests that validation
// catches tool nodes without tool_name
func TestParseDefinitionFromJSON_ToolMissingToolName(t *testing.T) {
	jsonData := `{
		"name": "test-mission",
		"nodes": {
			"n1": {
				"id": "n1",
				"type": "tool"
			}
		}
	}`

	_, err := ParseDefinitionFromJSON([]byte(jsonData))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool node n1 requires 'tool_name' field")
}

// TestParseDefinitionFromJSON_PluginMissingFields tests that validation
// catches plugin nodes without required fields
func TestParseDefinitionFromJSON_PluginMissingFields(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		errMsg   string
	}{
		{
			name: "missing plugin_name",
			jsonData: `{
				"name": "test-mission",
				"nodes": {
					"n1": {
						"id": "n1",
						"type": "plugin",
						"plugin_method": "query"
					}
				}
			}`,
			errMsg: "plugin node n1 requires 'plugin_name' field",
		},
		{
			name: "missing plugin_method",
			jsonData: `{
				"name": "test-mission",
				"nodes": {
					"n1": {
						"id": "n1",
						"type": "plugin",
						"plugin_name": "test-plugin"
					}
				}
			}`,
			errMsg: "plugin node n1 requires 'plugin_method' field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDefinitionFromJSON([]byte(tt.jsonData))
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestParseDefinitionFromJSON_ConditionValidation tests condition node validation
func TestParseDefinitionFromJSON_ConditionValidation(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		errMsg   string
	}{
		{
			name: "missing condition",
			jsonData: `{
				"name": "test-mission",
				"nodes": {
					"n1": {
						"id": "n1",
						"type": "condition"
					}
				}
			}`,
			errMsg: "condition node n1 requires 'condition' field",
		},
		{
			name: "missing expression",
			jsonData: `{
				"name": "test-mission",
				"nodes": {
					"n1": {
						"id": "n1",
						"type": "condition",
						"condition": {}
					}
				}
			}`,
			errMsg: "condition node n1 requires 'condition.expression' field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDefinitionFromJSON([]byte(tt.jsonData))
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestParseDefinitionFromJSON_ParallelValidation tests parallel node validation
func TestParseDefinitionFromJSON_ParallelValidation(t *testing.T) {
	jsonData := `{
		"name": "test-mission",
		"nodes": {
			"n1": {
				"id": "n1",
				"type": "parallel"
			}
		}
	}`

	_, err := ParseDefinitionFromJSON([]byte(jsonData))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parallel node n1 requires 'sub_nodes' field")
}

// TestParseDefinitionFromJSON_ParallelSubNodeValidation tests that sub-nodes
// are validated recursively
func TestParseDefinitionFromJSON_ParallelSubNodeValidation(t *testing.T) {
	jsonData := `{
		"name": "test-mission",
		"nodes": {
			"n1": {
				"id": "n1",
				"type": "parallel",
				"sub_nodes": [
					{
						"id": "sub1",
						"type": "agent"
					}
				]
			}
		}
	}`

	_, err := ParseDefinitionFromJSON([]byte(jsonData))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent_name")
}

// TestParseDefinitionFromJSON_InvalidJSON tests handling of malformed JSON
func TestParseDefinitionFromJSON_InvalidJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
	}{
		{
			name:     "invalid syntax",
			jsonData: `{invalid json}`,
		},
		{
			name:     "truncated JSON",
			jsonData: `{"name": "test", "nodes": {`,
		},
		{
			name:     "empty string",
			jsonData: ``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDefinitionFromJSON([]byte(tt.jsonData))
			assert.Error(t, err)
		})
	}
}

// TestParseDefinitionFromJSON_EmptyInput tests handling of empty input
func TestParseDefinitionFromJSON_EmptyInput(t *testing.T) {
	_, err := ParseDefinitionFromJSON([]byte{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot parse empty JSON data")
}

// TestParseDefinitionFromJSON_JoinNode tests that join nodes work correctly
func TestParseDefinitionFromJSON_JoinNode(t *testing.T) {
	jsonData := `{
		"name": "test-mission",
		"nodes": {
			"join1": {
				"id": "join1",
				"type": "join",
				"dependencies": ["agent1", "agent2"]
			}
		}
	}`

	parsed, err := ParseDefinitionFromJSON([]byte(jsonData))
	require.NoError(t, err)
	require.NotNil(t, parsed)

	node := parsed.Nodes["join1"]
	require.NotNil(t, node)
	assert.Equal(t, NodeTypeJoin, node.Type)
	assert.Equal(t, []string{"agent1", "agent2"}, node.Dependencies)
}

// TestParseDefinitionFromJSON_InvalidNodeType tests handling of invalid node types
func TestParseDefinitionFromJSON_InvalidNodeType(t *testing.T) {
	jsonData := `{
		"name": "test-mission",
		"nodes": {
			"n1": {
				"id": "n1",
				"type": "invalid-type"
			}
		}
	}`

	_, err := ParseDefinitionFromJSON([]byte(jsonData))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
}

// TestParseDefinitionFromJSON_ComplexMission tests a realistic complex mission
func TestParseDefinitionFromJSON_ComplexMission(t *testing.T) {
	original := &MissionDefinition{
		Name:        "complex-mission",
		Description: "A complex mission with multiple node types",
		Version:     "2.0.0",
		Nodes: map[string]*MissionNode{
			"recon": {
				ID:        "recon",
				Type:      NodeTypeAgent,
				AgentName: "network-recon",
				Timeout:   10 * time.Minute,
				RetryPolicy: &RetryPolicy{
					MaxRetries:      3,
					BackoffStrategy: BackoffExponential,
					InitialDelay:    1 * time.Second,
					MaxDelay:        30 * time.Second,
					Multiplier:      2.0,
				},
			},
			"parallel-scan": {
				ID:   "parallel-scan",
				Type: NodeTypeParallel,
				SubNodes: []*MissionNode{
					{
						ID:        "port-scan",
						Type:      NodeTypeTool,
						ToolName:  "nmap",
						Timeout:   5 * time.Minute,
						ToolInput: map[string]any{"ports": "1-1024"},
					},
					{
						ID:        "vuln-scan",
						Type:      NodeTypeTool,
						ToolName:  "nuclei",
						Timeout:   15 * time.Minute,
						ToolInput: map[string]any{"templates": "all"},
					},
				},
			},
			"check-results": {
				ID:   "check-results",
				Type: NodeTypeCondition,
				Condition: &NodeCondition{
					Expression:  "findings.count > 0",
					TrueBranch:  []string{"exploit"},
					FalseBranch: []string{"cleanup"},
				},
			},
			"exploit": {
				ID:        "exploit",
				Type:      NodeTypeAgent,
				AgentName: "exploit-agent",
				Timeout:   20 * time.Minute,
			},
			"cleanup": {
				ID:           "cleanup",
				Type:         NodeTypePlugin,
				PluginName:   "cleanup-plugin",
				PluginMethod: "cleanup",
			},
		},
		Edges: []MissionEdge{
			{From: "recon", To: "parallel-scan"},
			{From: "parallel-scan", To: "check-results"},
		},
		EntryPoints: []string{"recon"},
		ExitPoints:  []string{"exploit", "cleanup"},
	}

	// Marshal and parse
	data, err := json.Marshal(original)
	require.NoError(t, err)

	parsed, err := ParseDefinitionFromJSON(data)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Verify structure
	assert.Equal(t, "complex-mission", parsed.Name)
	assert.Len(t, parsed.Nodes, 5)

	// Verify parallel node with sub-nodes
	parallelNode := parsed.Nodes["parallel-scan"]
	require.NotNil(t, parallelNode)
	assert.Equal(t, NodeTypeParallel, parallelNode.Type)
	assert.Len(t, parallelNode.SubNodes, 2)

	// Verify sub-nodes
	assert.Equal(t, "port-scan", parallelNode.SubNodes[0].ID)
	assert.Equal(t, "nmap", parallelNode.SubNodes[0].ToolName)
	assert.Equal(t, "vuln-scan", parallelNode.SubNodes[1].ID)
	assert.Equal(t, "nuclei", parallelNode.SubNodes[1].ToolName)

	// Verify condition node
	condNode := parsed.Nodes["check-results"]
	require.NotNil(t, condNode)
	assert.Equal(t, NodeTypeCondition, condNode.Type)
	require.NotNil(t, condNode.Condition)
	assert.Equal(t, "findings.count > 0", condNode.Condition.Expression)
}
