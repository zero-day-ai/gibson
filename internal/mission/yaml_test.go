package mission

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromReader_ValidConfig(t *testing.T) {
	yamlContent := `
name: test-mission
description: Test mission description
target:
  reference: target-1
workflow:
  reference: workflow-1
constraints:
  max_duration: 1h30m
  max_findings: 100
  max_cost: 10.50
  severity_threshold: high
  require_approval: true
`

	config, err := LoadFromReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	assert.Equal(t, "test-mission", config.Name)
	assert.Equal(t, "Test mission description", config.Description)
	assert.Equal(t, "target-1", config.Target.Reference)
	assert.Equal(t, "workflow-1", config.Workflow.Reference)
	assert.NotNil(t, config.Constraints)
	assert.Equal(t, "1h30m", config.Constraints.MaxDuration)
	assert.Equal(t, 100, *config.Constraints.MaxFindings)
	assert.Equal(t, 10.50, *config.Constraints.MaxCost)
}

func TestLoadFromReader_InlineTarget(t *testing.T) {
	yamlContent := `
name: test-mission
description: Test mission
target:
  inline:
    type: llm
    provider: openai
    model: gpt-4
workflow:
  reference: workflow-1
`

	config, err := LoadFromReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	assert.NotNil(t, config.Target.Inline)
	assert.Equal(t, "llm", config.Target.Inline.Type)
	assert.Equal(t, "openai", config.Target.Inline.Provider)
	assert.Equal(t, "gpt-4", config.Target.Inline.Model)
}

func TestLoadFromReader_InlineWorkflow(t *testing.T) {
	yamlContent := `
name: test-mission
description: Test mission
target:
  reference: target-1
workflow:
  inline:
    agents:
      - agent-1
      - agent-2
`

	config, err := LoadFromReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	assert.NotNil(t, config.Workflow.Inline)
	assert.Equal(t, 2, len(config.Workflow.Inline.Agents))
}

func TestLoadFromReader_EnvVarInterpolation(t *testing.T) {
	// Set test environment variable
	os.Setenv("TEST_TARGET", "test-target-id")
	defer os.Unsetenv("TEST_TARGET")

	yamlContent := `
name: test-mission
description: Test mission
target:
  reference: ${TEST_TARGET}
workflow:
  reference: workflow-1
`

	config, err := LoadFromReader(strings.NewReader(yamlContent))
	require.NoError(t, err)
	assert.Equal(t, "test-target-id", config.Target.Reference)
}

func TestLoadFromReader_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing name",
			yaml: `
description: Test mission
target:
  reference: target-1
workflow:
  reference: workflow-1
`,
			wantErr: "mission name is required",
		},
		{
			name: "missing target",
			yaml: `
name: test-mission
workflow:
  reference: workflow-1
`,
			wantErr: "target must specify either 'reference' or 'inline'",
		},
		{
			name: "both target reference and inline",
			yaml: `
name: test-mission
target:
  reference: target-1
  inline:
    type: llm
    provider: openai
workflow:
  reference: workflow-1
`,
			wantErr: "target cannot specify both 'reference' and 'inline'",
		},
		{
			name: "invalid max_duration",
			yaml: `
name: test-mission
target:
  reference: target-1
workflow:
  reference: workflow-1
constraints:
  max_duration: invalid
`,
			wantErr: "invalid max_duration format",
		},
		{
			name: "invalid max_findings",
			yaml: `
name: test-mission
target:
  reference: target-1
workflow:
  reference: workflow-1
constraints:
  max_findings: -1
`,
			wantErr: "max_findings must be greater than 0",
		},
		{
			name: "invalid severity_threshold",
			yaml: `
name: test-mission
target:
  reference: target-1
workflow:
  reference: workflow-1
constraints:
  severity_threshold: invalid
`,
			wantErr: "invalid severity_threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFromReader(strings.NewReader(tt.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMissionConstraintsConfig_ToConstraints(t *testing.T) {
	maxFindings := 100
	maxCost := 10.50
	severityThreshold := "high"
	requireApproval := true

	config := &MissionConstraintsConfig{
		MaxDuration:       "1h30m",
		MaxFindings:       &maxFindings,
		MaxCost:           &maxCost,
		SeverityThreshold: &severityThreshold,
		RequireApproval:   &requireApproval,
	}

	constraints, err := config.ToConstraints()
	require.NoError(t, err)
	assert.NotZero(t, constraints.MaxDuration)
	assert.Equal(t, maxFindings, constraints.MaxFindings)
	assert.Equal(t, maxCost, constraints.MaxCost)
	assert.Equal(t, severityThreshold, string(constraints.SeverityThreshold))
}

func TestLoadFromFile(t *testing.T) {
	// Create temporary YAML file
	tmpFile, err := os.CreateTemp("", "mission-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	yamlContent := `
name: test-mission
description: Test mission from file
target:
  reference: target-1
workflow:
  reference: workflow-1
`

	_, err = tmpFile.WriteString(yamlContent)
	require.NoError(t, err)
	tmpFile.Close()

	config, err := LoadFromFile(tmpFile.Name())
	require.NoError(t, err)
	assert.Equal(t, "test-mission", config.Name)
}

func TestLoadFromFile_NonExistent(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/file.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open file")
}
