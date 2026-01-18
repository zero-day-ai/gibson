package mission

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
	"gopkg.in/yaml.v3"
)

func TestMissionDefinition_YAMLMarshalUnmarshal(t *testing.T) {
	original := &MissionDefinition{
		ID:          types.NewID(),
		Name:        "test-mission",
		Description: "A test mission definition",
		Version:     "1.0.0",
		TargetRef:   "test-target",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:          "node1",
				Type:        NodeTypeAgent,
				Name:        "Test Agent",
				Description: "Agent node description",
				AgentName:   "test-agent",
				AgentTask: &agent.Task{
					Name: "Test Task",
					Goal: "Test objective",
				},
				Timeout: 5 * time.Minute,
			},
			"node2": {
				ID:        "node2",
				Type:      NodeTypeTool,
				Name:      "Test Tool",
				ToolName:  "test-tool",
				ToolInput: map[string]any{"key": "value"},
			},
		},
		Edges: []MissionEdge{
			{
				From: "node1",
				To:   "node2",
			},
		},
		EntryPoints: []string{"node1"},
		ExitPoints:  []string{"node2"},
		Metadata: map[string]any{
			"author": "test-author",
			"tags":   []string{"test", "demo"},
		},
		Dependencies: &MissionDependencies{
			Agents: []string{"test-agent"},
			Tools:  []string{"test-tool"},
		},
		Source:      "https://github.com/test/mission",
		InstalledAt: time.Now().Truncate(time.Second),
		CreatedAt:   time.Now().Truncate(time.Second),
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(original)
	require.NoError(t, err)
	assert.NotEmpty(t, yamlData)

	// Unmarshal back
	var decoded MissionDefinition
	err = yaml.Unmarshal(yamlData, &decoded)
	require.NoError(t, err)

	// Verify key fields
	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Description, decoded.Description)
	assert.Equal(t, original.Version, decoded.Version)
	assert.Equal(t, original.TargetRef, decoded.TargetRef)
	assert.Len(t, decoded.Nodes, 2)
	assert.Len(t, decoded.Edges, 1)
	assert.Equal(t, original.Source, decoded.Source)
}

func TestMissionDefinition_JSONMarshalUnmarshal(t *testing.T) {
	original := &MissionDefinition{
		ID:          types.NewID(),
		Name:        "json-test-mission",
		Description: "JSON serialization test",
		Version:     "2.0.0",
		TargetRef:   "json-target",
		Nodes: map[string]*MissionNode{
			"plugin-node": {
				ID:           "plugin-node",
				Type:         NodeTypePlugin,
				Name:         "Test Plugin",
				PluginName:   "test-plugin",
				PluginMethod: "query",
				PluginParams: map[string]any{
					"param1": "value1",
					"param2": 42,
				},
			},
		},
		Edges:       []MissionEdge{},
		EntryPoints: []string{"plugin-node"},
		ExitPoints:  []string{"plugin-node"},
		Dependencies: &MissionDependencies{
			Plugins: []string{"test-plugin"},
		},
		CreatedAt: time.Now().Truncate(time.Second),
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(original)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Unmarshal back
	var decoded MissionDefinition
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	// Verify key fields
	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Description, decoded.Description)
	assert.Equal(t, original.Version, decoded.Version)
	assert.Len(t, decoded.Nodes, 1)
	assert.Equal(t, original.Dependencies.Plugins, decoded.Dependencies.Plugins)
}

func TestMissionNode_YAMLSerialization(t *testing.T) {
	tests := []struct {
		name string
		node *MissionNode
	}{
		{
			name: "agent node",
			node: &MissionNode{
				ID:        "agent-1",
				Type:      NodeTypeAgent,
				Name:      "Scanner Agent",
				AgentName: "scanner",
				AgentTask: &agent.Task{
					Name: "Scan Task",
					Goal: "Scan for vulnerabilities",
				},
				Timeout: 10 * time.Minute,
			},
		},
		{
			name: "tool node",
			node: &MissionNode{
				ID:       "tool-1",
				Type:     NodeTypeTool,
				Name:     "Port Scanner",
				ToolName: "nmap",
				ToolInput: map[string]any{
					"target": "192.168.1.0/24",
					"ports":  "1-1000",
				},
			},
		},
		{
			name: "condition node",
			node: &MissionNode{
				ID:   "condition-1",
				Type: NodeTypeCondition,
				Name: "Check Results",
				Condition: &NodeCondition{
					Expression:  "result.status == 'vulnerable'",
					TrueBranch:  []string{"exploit-node"},
					FalseBranch: []string{"report-node"},
				},
			},
		},
		{
			name: "parallel node",
			node: &MissionNode{
				ID:   "parallel-1",
				Type: NodeTypeParallel,
				Name: "Parallel Scans",
				SubNodes: []*MissionNode{
					{
						ID:       "sub-1",
						Type:     NodeTypeTool,
						ToolName: "tool1",
					},
					{
						ID:       "sub-2",
						Type:     NodeTypeTool,
						ToolName: "tool2",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to YAML
			yamlData, err := yaml.Marshal(tt.node)
			require.NoError(t, err)
			assert.NotEmpty(t, yamlData)

			// Unmarshal back
			var decoded MissionNode
			err = yaml.Unmarshal(yamlData, &decoded)
			require.NoError(t, err)

			// Verify core fields
			assert.Equal(t, tt.node.ID, decoded.ID)
			assert.Equal(t, tt.node.Type, decoded.Type)
			assert.Equal(t, tt.node.Name, decoded.Name)
		})
	}
}

func TestMissionNode_JSONSerialization(t *testing.T) {
	node := &MissionNode{
		ID:        "json-node",
		Type:      NodeTypeAgent,
		Name:      "JSON Test Node",
		AgentName: "test-agent",
		RetryPolicy: &RetryPolicy{
			MaxRetries:      3,
			BackoffStrategy: BackoffExponential,
			InitialDelay:    1 * time.Second,
			MaxDelay:        30 * time.Second,
			Multiplier:      2.0,
		},
		Timeout: 5 * time.Minute,
		Metadata: map[string]any{
			"priority": "high",
			"tags":     []string{"critical"},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(node)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Unmarshal back
	var decoded MissionNode
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, node.ID, decoded.ID)
	assert.Equal(t, node.Type, decoded.Type)
	assert.Equal(t, node.AgentName, decoded.AgentName)
	assert.NotNil(t, decoded.RetryPolicy)
	assert.Equal(t, node.RetryPolicy.MaxRetries, decoded.RetryPolicy.MaxRetries)
	assert.Equal(t, node.RetryPolicy.BackoffStrategy, decoded.RetryPolicy.BackoffStrategy)
}

func TestMissionEdge_Serialization(t *testing.T) {
	edge := MissionEdge{
		From:      "node1",
		To:        "node2",
		Condition: "result.success == true",
	}

	// YAML round-trip
	yamlData, err := yaml.Marshal(edge)
	require.NoError(t, err)

	var decodedYAML MissionEdge
	err = yaml.Unmarshal(yamlData, &decodedYAML)
	require.NoError(t, err)
	assert.Equal(t, edge, decodedYAML)

	// JSON round-trip
	jsonData, err := json.Marshal(edge)
	require.NoError(t, err)

	var decodedJSON MissionEdge
	err = json.Unmarshal(jsonData, &decodedJSON)
	require.NoError(t, err)
	assert.Equal(t, edge, decodedJSON)
}

func TestMissionDefinition_GetNode(t *testing.T) {
	def := &MissionDefinition{
		Name: "test",
		Nodes: map[string]*MissionNode{
			"node1": {ID: "node1", Type: NodeTypeAgent},
			"node2": {ID: "node2", Type: NodeTypeTool},
		},
	}

	// Test existing node
	node := def.GetNode("node1")
	assert.NotNil(t, node)
	assert.Equal(t, "node1", node.ID)

	// Test non-existent node
	node = def.GetNode("node3")
	assert.Nil(t, node)

	// Test nil nodes map
	emptyDef := &MissionDefinition{Name: "empty"}
	node = emptyDef.GetNode("node1")
	assert.Nil(t, node)
}

func TestMissionDefinition_GetEntryNodes(t *testing.T) {
	def := &MissionDefinition{
		Name: "test",
		Nodes: map[string]*MissionNode{
			"entry1": {ID: "entry1", Type: NodeTypeAgent},
			"entry2": {ID: "entry2", Type: NodeTypeTool},
			"middle": {ID: "middle", Type: NodeTypeAgent},
		},
		EntryPoints: []string{"entry1", "entry2"},
	}

	entryNodes := def.GetEntryNodes()
	assert.Len(t, entryNodes, 2)

	// Verify we got the right nodes
	ids := make(map[string]bool)
	for _, node := range entryNodes {
		ids[node.ID] = true
	}
	assert.True(t, ids["entry1"])
	assert.True(t, ids["entry2"])
	assert.False(t, ids["middle"])

	// Test with nil entry points
	emptyDef := &MissionDefinition{Name: "empty"}
	entryNodes = emptyDef.GetEntryNodes()
	assert.Empty(t, entryNodes)
}

func TestMissionDefinition_GetExitNodes(t *testing.T) {
	def := &MissionDefinition{
		Name: "test",
		Nodes: map[string]*MissionNode{
			"middle": {ID: "middle", Type: NodeTypeAgent},
			"exit1":  {ID: "exit1", Type: NodeTypeAgent},
			"exit2":  {ID: "exit2", Type: NodeTypeTool},
		},
		ExitPoints: []string{"exit1", "exit2"},
	}

	exitNodes := def.GetExitNodes()
	assert.Len(t, exitNodes, 2)

	// Verify we got the right nodes
	ids := make(map[string]bool)
	for _, node := range exitNodes {
		ids[node.ID] = true
	}
	assert.True(t, ids["exit1"])
	assert.True(t, ids["exit2"])
	assert.False(t, ids["middle"])
}

func TestRetryPolicy_CalculateDelay(t *testing.T) {
	tests := []struct {
		name     string
		policy   *RetryPolicy
		attempt  int
		expected time.Duration
	}{
		{
			name: "constant backoff",
			policy: &RetryPolicy{
				MaxRetries:      5,
				BackoffStrategy: BackoffConstant,
				InitialDelay:    1 * time.Second,
			},
			attempt:  3,
			expected: 1 * time.Second,
		},
		{
			name: "linear backoff",
			policy: &RetryPolicy{
				MaxRetries:      5,
				BackoffStrategy: BackoffLinear,
				InitialDelay:    1 * time.Second,
			},
			attempt:  2,
			expected: 3 * time.Second, // 1s + (1s * 2)
		},
		{
			name: "exponential backoff",
			policy: &RetryPolicy{
				MaxRetries:      5,
				BackoffStrategy: BackoffExponential,
				InitialDelay:    1 * time.Second,
				MaxDelay:        30 * time.Second,
				Multiplier:      2.0,
			},
			attempt:  3,
			expected: 8 * time.Second, // 1s * 2^3
		},
		{
			name: "exponential backoff with max",
			policy: &RetryPolicy{
				MaxRetries:      10,
				BackoffStrategy: BackoffExponential,
				InitialDelay:    1 * time.Second,
				MaxDelay:        10 * time.Second,
				Multiplier:      2.0,
			},
			attempt:  5,
			expected: 10 * time.Second, // capped at MaxDelay
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := tt.policy.CalculateDelay(tt.attempt)
			assert.Equal(t, tt.expected, delay)
		})
	}
}

func TestNodeType_Constants(t *testing.T) {
	// Verify all node type constants are unique
	nodeTypes := []NodeType{
		NodeTypeAgent,
		NodeTypeTool,
		NodeTypePlugin,
		NodeTypeCondition,
		NodeTypeParallel,
		NodeTypeJoin,
	}

	// Check uniqueness
	seen := make(map[NodeType]bool)
	for _, nt := range nodeTypes {
		assert.False(t, seen[nt], "duplicate node type: %s", nt)
		seen[nt] = true
	}

	// Verify string values
	assert.Equal(t, "agent", string(NodeTypeAgent))
	assert.Equal(t, "tool", string(NodeTypeTool))
	assert.Equal(t, "plugin", string(NodeTypePlugin))
	assert.Equal(t, "condition", string(NodeTypeCondition))
	assert.Equal(t, "parallel", string(NodeTypeParallel))
	assert.Equal(t, "join", string(NodeTypeJoin))
}

func TestBackoffStrategy_Constants(t *testing.T) {
	strategies := []BackoffStrategy{
		BackoffConstant,
		BackoffLinear,
		BackoffExponential,
	}

	// Check uniqueness
	seen := make(map[BackoffStrategy]bool)
	for _, bs := range strategies {
		assert.False(t, seen[bs], "duplicate backoff strategy: %s", bs)
		seen[bs] = true
	}

	// Verify string values
	assert.Equal(t, "constant", string(BackoffConstant))
	assert.Equal(t, "linear", string(BackoffLinear))
	assert.Equal(t, "exponential", string(BackoffExponential))
}

func TestMissionDependencies_Serialization(t *testing.T) {
	deps := &MissionDependencies{
		Agents:  []string{"agent1", "agent2"},
		Tools:   []string{"tool1"},
		Plugins: []string{"plugin1", "plugin2", "plugin3"},
	}

	// YAML round-trip
	yamlData, err := yaml.Marshal(deps)
	require.NoError(t, err)

	var decodedYAML MissionDependencies
	err = yaml.Unmarshal(yamlData, &decodedYAML)
	require.NoError(t, err)
	assert.Equal(t, deps.Agents, decodedYAML.Agents)
	assert.Equal(t, deps.Tools, decodedYAML.Tools)
	assert.Equal(t, deps.Plugins, decodedYAML.Plugins)

	// JSON round-trip
	jsonData, err := json.Marshal(deps)
	require.NoError(t, err)

	var decodedJSON MissionDependencies
	err = json.Unmarshal(jsonData, &decodedJSON)
	require.NoError(t, err)
	assert.Equal(t, deps.Agents, decodedJSON.Agents)
	assert.Equal(t, deps.Tools, decodedJSON.Tools)
	assert.Equal(t, deps.Plugins, decodedJSON.Plugins)
}

func TestNodeCondition_Serialization(t *testing.T) {
	cond := &NodeCondition{
		Expression:  "findings.count > 0",
		TrueBranch:  []string{"exploit", "report"},
		FalseBranch: []string{"retry"},
	}

	// YAML round-trip
	yamlData, err := yaml.Marshal(cond)
	require.NoError(t, err)

	var decodedYAML NodeCondition
	err = yaml.Unmarshal(yamlData, &decodedYAML)
	require.NoError(t, err)
	assert.Equal(t, cond.Expression, decodedYAML.Expression)
	assert.Equal(t, cond.TrueBranch, decodedYAML.TrueBranch)
	assert.Equal(t, cond.FalseBranch, decodedYAML.FalseBranch)

	// JSON round-trip
	jsonData, err := json.Marshal(cond)
	require.NoError(t, err)

	var decodedJSON NodeCondition
	err = json.Unmarshal(jsonData, &decodedJSON)
	require.NoError(t, err)
	assert.Equal(t, cond.Expression, decodedJSON.Expression)
	assert.Equal(t, cond.TrueBranch, decodedJSON.TrueBranch)
	assert.Equal(t, cond.FalseBranch, decodedJSON.FalseBranch)
}

func TestMissionDefinition_ComplexStructure(t *testing.T) {
	// Test a complex mission with multiple node types and dependencies
	def := &MissionDefinition{
		ID:          types.NewID(),
		Name:        "complex-mission",
		Description: "A complex multi-stage mission",
		Version:     "1.0.0",
		TargetRef:   "production-api",
		Nodes: map[string]*MissionNode{
			"recon": {
				ID:        "recon",
				Type:      NodeTypeAgent,
				Name:      "Reconnaissance",
				AgentName: "recon-agent",
				Timeout:   10 * time.Minute,
				RetryPolicy: &RetryPolicy{
					MaxRetries:      3,
					BackoffStrategy: BackoffExponential,
					InitialDelay:    5 * time.Second,
					MaxDelay:        60 * time.Second,
					Multiplier:      2.0,
				},
			},
			"scan": {
				ID:       "scan",
				Type:     NodeTypeParallel,
				Name:     "Parallel Scanning",
				SubNodes: []*MissionNode{
					{ID: "port-scan", Type: NodeTypeTool, ToolName: "nmap"},
					{ID: "vuln-scan", Type: NodeTypeTool, ToolName: "nuclei"},
				},
			},
			"evaluate": {
				ID:   "evaluate",
				Type: NodeTypeCondition,
				Name: "Evaluate Results",
				Condition: &NodeCondition{
					Expression:  "findings.critical > 0",
					TrueBranch:  []string{"exploit"},
					FalseBranch: []string{"report"},
				},
			},
		},
		Edges: []MissionEdge{
			{From: "recon", To: "scan"},
			{From: "scan", To: "evaluate"},
		},
		EntryPoints: []string{"recon"},
		ExitPoints:  []string{"exploit", "report"},
		Dependencies: &MissionDependencies{
			Agents: []string{"recon-agent"},
			Tools:  []string{"nmap", "nuclei"},
		},
		Metadata: map[string]any{
			"category": "penetration-testing",
			"severity": "high",
			"tags":     []string{"network", "web", "api"},
		},
	}

	// YAML serialization
	yamlData, err := yaml.Marshal(def)
	require.NoError(t, err)

	var yamlDecoded MissionDefinition
	err = yaml.Unmarshal(yamlData, &yamlDecoded)
	require.NoError(t, err)

	assert.Equal(t, def.Name, yamlDecoded.Name)
	assert.Len(t, yamlDecoded.Nodes, 3)
	assert.Len(t, yamlDecoded.Edges, 2)

	// JSON serialization
	jsonData, err := json.Marshal(def)
	require.NoError(t, err)

	var jsonDecoded MissionDefinition
	err = json.Unmarshal(jsonData, &jsonDecoded)
	require.NoError(t, err)

	assert.Equal(t, def.Name, jsonDecoded.Name)
	assert.Len(t, jsonDecoded.Nodes, 3)
	assert.Len(t, jsonDecoded.Edges, 2)
}
