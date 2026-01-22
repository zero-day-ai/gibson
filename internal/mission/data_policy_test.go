package mission

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDataPolicy_SetDefaults(t *testing.T) {
	tests := []struct {
		name     string
		policy   *DataPolicy
		expected *DataPolicy
	}{
		{
			name:   "empty policy gets defaults",
			policy: &DataPolicy{},
			expected: &DataPolicy{
				OutputScope: ScopeMission,
				InputScope:  ScopeMission,
				Reuse:       ReuseNever,
			},
		},
		{
			name: "partial policy fills missing fields",
			policy: &DataPolicy{
				OutputScope: ScopeGlobal,
			},
			expected: &DataPolicy{
				OutputScope: ScopeGlobal,
				InputScope:  ScopeMission,
				Reuse:       ReuseNever,
			},
		},
		{
			name: "complete policy unchanged",
			policy: &DataPolicy{
				OutputScope: ScopeMissionRun,
				InputScope:  ScopeGlobal,
				Reuse:       ReuseSkip,
			},
			expected: &DataPolicy{
				OutputScope: ScopeMissionRun,
				InputScope:  ScopeGlobal,
				Reuse:       ReuseSkip,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.policy.SetDefaults()
			assert.Equal(t, tt.expected, tt.policy)
		})
	}
}

func TestDataPolicy_Validate(t *testing.T) {
	tests := []struct {
		name    string
		policy  *DataPolicy
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid policy - all mission scope",
			policy: &DataPolicy{
				OutputScope: ScopeMission,
				InputScope:  ScopeMission,
				Reuse:       ReuseNever,
			},
			wantErr: false,
		},
		{
			name: "valid policy - mixed scopes",
			policy: &DataPolicy{
				OutputScope: ScopeGlobal,
				InputScope:  ScopeMissionRun,
				Reuse:       ReuseSkip,
			},
			wantErr: false,
		},
		{
			name: "invalid output_scope",
			policy: &DataPolicy{
				OutputScope: "invalid",
				InputScope:  ScopeMission,
				Reuse:       ReuseNever,
			},
			wantErr: true,
			errMsg:  "invalid output_scope",
		},
		{
			name: "invalid input_scope",
			policy: &DataPolicy{
				OutputScope: ScopeMission,
				InputScope:  "bad_scope",
				Reuse:       ReuseNever,
			},
			wantErr: true,
			errMsg:  "invalid input_scope",
		},
		{
			name: "invalid reuse value",
			policy: &DataPolicy{
				OutputScope: ScopeMission,
				InputScope:  ScopeMission,
				Reuse:       "maybe",
			},
			wantErr: true,
			errMsg:  "invalid reuse value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMissionNode_DataPolicyYAMLParsing(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		validate func(t *testing.T, node *MissionNode)
	}{
		{
			name: "node with data_policy",
			yaml: `
id: recon
type: agent
agent_name: network-recon
data_policy:
  output_scope: mission
  input_scope: mission
  reuse: skip
`,
			validate: func(t *testing.T, node *MissionNode) {
				require.NotNil(t, node.DataPolicy, "DataPolicy should not be nil")
				assert.Equal(t, "mission", node.DataPolicy.OutputScope)
				assert.Equal(t, "mission", node.DataPolicy.InputScope)
				assert.Equal(t, "skip", node.DataPolicy.Reuse)
			},
		},
		{
			name: "node without data_policy",
			yaml: `
id: scan
type: agent
agent_name: port-scanner
`,
			validate: func(t *testing.T, node *MissionNode) {
				assert.Nil(t, node.DataPolicy, "DataPolicy should be nil when not specified")
			},
		},
		{
			name: "node with partial data_policy",
			yaml: `
id: exploit
type: agent
agent_name: exploiter
data_policy:
  reuse: always
`,
			validate: func(t *testing.T, node *MissionNode) {
				require.NotNil(t, node.DataPolicy)
				assert.Equal(t, "", node.DataPolicy.OutputScope, "Unset fields should be empty")
				assert.Equal(t, "", node.DataPolicy.InputScope, "Unset fields should be empty")
				assert.Equal(t, "always", node.DataPolicy.Reuse)
			},
		},
		{
			name: "node with all scope options",
			yaml: `
id: global-recon
type: agent
agent_name: recon
data_policy:
  output_scope: global
  input_scope: mission_run
  reuse: never
`,
			validate: func(t *testing.T, node *MissionNode) {
				require.NotNil(t, node.DataPolicy)
				assert.Equal(t, "global", node.DataPolicy.OutputScope)
				assert.Equal(t, "mission_run", node.DataPolicy.InputScope)
				assert.Equal(t, "never", node.DataPolicy.Reuse)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node MissionNode
			err := yaml.Unmarshal([]byte(tt.yaml), &node)
			require.NoError(t, err, "YAML parsing should succeed")
			tt.validate(t, &node)
		})
	}
}

func TestParseDefinitionWithDataPolicy(t *testing.T) {
	yamlStr := `
name: Test Mission
version: 1.0.0

nodes:
  recon:
    id: recon
    type: agent
    agent: network-recon
    data_policy:
      output_scope: mission
      input_scope: mission
      reuse: skip
  scan:
    id: scan
    type: agent
    agent: port-scanner
`

	def, err := ParseDefinitionFromBytes([]byte(yamlStr))
	require.NoError(t, err, "Should parse mission with data_policy")
	require.NotNil(t, def)

	assert.Equal(t, "Test Mission", def.Name)
	assert.Equal(t, 2, len(def.Nodes))

	// Check node with data_policy
	reconNode := def.Nodes["recon"]
	require.NotNil(t, reconNode, "recon node should exist")
	require.NotNil(t, reconNode.DataPolicy, "recon node should have DataPolicy")
	assert.Equal(t, "mission", reconNode.DataPolicy.OutputScope)
	assert.Equal(t, "mission", reconNode.DataPolicy.InputScope)
	assert.Equal(t, "skip", reconNode.DataPolicy.Reuse)

	// Check node without data_policy
	scanNode := def.Nodes["scan"]
	require.NotNil(t, scanNode, "scan node should exist")
	assert.Nil(t, scanNode.DataPolicy, "scan node should not have DataPolicy")
}
