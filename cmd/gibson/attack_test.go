package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestAttackCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		setupMock   func()
		wantErr     bool
		errContains string
	}{
		{
			name:        "missing URL argument",
			args:        []string{},
			wantErr:     true,
			errContains: "accepts 1 arg",
		},
		{
			name: "valid URL with defaults",
			args: []string{"https://example.com"},
		},
		{
			name: "with agent flag",
			args: []string{"https://example.com", "--agent", "testagent"},
		},
		{
			name: "with techniques flag",
			args: []string{"https://example.com", "--techniques", "sqli,xss"},
		},
		{
			name: "with goal flag",
			args: []string{"https://example.com", "--goal", "Find vulnerabilities"},
		},
		{
			name: "with max-turns flag",
			args: []string{"https://example.com", "--max-turns", "20"},
		},
		{
			name: "with timeout flag",
			args: []string{"https://example.com", "--timeout", "1h"},
		},
		{
			name: "with no-persist flag",
			args: []string{"https://example.com", "--no-persist"},
		},
		{
			name: "with verbose flag",
			args: []string{"https://example.com", "--verbose"},
		},
		{
			name: "all flags combined",
			args: []string{
				"https://example.com",
				"--agent", "testagent",
				"--techniques", "sqli,xss,csrf",
				"--goal", "Complete security assessment",
				"--max-turns", "25",
				"--timeout", "2h",
				"--verbose",
			},
		},
		{
			name:        "invalid timeout format",
			args:        []string{"https://example.com", "--timeout", "invalid"},
			wantErr:     true,
			errContains: "invalid timeout format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create command
			cmd := &cobra.Command{}
			cmd.SetArgs(tt.args)

			// Reset flags for each test
			attackAgentName = ""
			attackTechniques = ""
			attackGoal = ""
			attackCredential = ""
			attackMaxTurns = 10
			attackTimeout = "30m"
			attackNoPersist = false
			attackVerbose = false

			// Re-initialize the command for each test
			testAttackCmd := &cobra.Command{
				Use:  "attack URL",
				Args: cobra.ExactArgs(1),
				RunE: func(cmd *cobra.Command, args []string) error {
					// Just validate flags are parsed correctly
					return nil
				},
			}

			testAttackCmd.Flags().StringVar(&attackAgentName, "agent", "", "Agent to use")
			testAttackCmd.Flags().StringVar(&attackTechniques, "techniques", "", "Techniques")
			testAttackCmd.Flags().StringVar(&attackGoal, "goal", "", "Goal")
			testAttackCmd.Flags().StringVar(&attackCredential, "credential", "", "Credential")
			testAttackCmd.Flags().IntVar(&attackMaxTurns, "max-turns", 10, "Max turns")
			testAttackCmd.Flags().StringVar(&attackTimeout, "timeout", "30m", "Timeout")
			testAttackCmd.Flags().BoolVar(&attackNoPersist, "no-persist", false, "No persist")
			testAttackCmd.Flags().BoolVar(&attackVerbose, "verbose", false, "Verbose")

			testAttackCmd.SetArgs(tt.args)

			err := testAttackCmd.Execute()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSelectAgent(t *testing.T) {
	tests := []struct {
		name        string
		agentName   string
		agents      []*component.Component
		wantName    string
		wantErr     bool
		errContains string
	}{
		{
			name:        "no agents installed",
			agentName:   "",
			agents:      []*component.Component{},
			wantErr:     true,
			errContains: "no agents installed",
		},
		{
			name:      "specific agent found",
			agentName: "promptinjector",
			agents: []*component.Component{
				{
					Name:   "promptinjector",
					Kind:   component.ComponentKindAgent,
					Status: component.ComponentStatusRunning,
					PID:    1234,
				},
				{
					Name:   "sqliagent",
					Kind:   component.ComponentKindAgent,
					Status: component.ComponentStatusStopped,
				},
			},
			wantName: "promptinjector",
			wantErr:  false,
		},
		{
			name:      "specific agent not found",
			agentName: "nonexistent",
			agents: []*component.Component{
				{
					Name:   "promptinjector",
					Kind:   component.ComponentKindAgent,
					Status: component.ComponentStatusRunning,
				},
			},
			wantErr:     true,
			errContains: "agent 'nonexistent' not found",
		},
		{
			name:      "default to first running agent",
			agentName: "",
			agents: []*component.Component{
				{
					Name:   "stopped-agent",
					Kind:   component.ComponentKindAgent,
					Status: component.ComponentStatusStopped,
				},
				{
					Name:   "running-agent",
					Kind:   component.ComponentKindAgent,
					Status: component.ComponentStatusRunning,
					PID:    5678,
				},
			},
			wantName: "running-agent",
			wantErr:  false,
		},
		{
			name:      "default to first agent if none running",
			agentName: "",
			agents: []*component.Component{
				{
					Name:   "first-agent",
					Kind:   component.ComponentKindAgent,
					Status: component.ComponentStatusStopped,
				},
				{
					Name:   "second-agent",
					Kind:   component.ComponentKindAgent,
					Status: component.ComponentStatusStopped,
				},
			},
			wantName: "first-agent",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock registry
			registry := &mockComponentRegistry{
				components: tt.agents,
			}

			cmd := &cobra.Command{}
			selectedAgent, err := selectAgent(cmd, registry, tt.agentName)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, selectedAgent)
				assert.Equal(t, tt.wantName, selectedAgent.Name)
			}
		})
	}
}

func TestCreateEphemeralMission(t *testing.T) {
	targetURL := "https://example.com"
	agentName := "testagent"
	goal := "Find vulnerabilities"
	techniques := "sqli,xss"

	mission := createEphemeralMission(targetURL, agentName, goal, techniques)

	require.NotNil(t, mission)
	assert.NotEmpty(t, mission.ID)
	assert.Contains(t, mission.Name, "attack-")
	assert.Equal(t, goal, mission.Description)
	assert.Equal(t, "running", mission.Status)
	assert.NotNil(t, mission.StartedAt)

	require.NotNil(t, mission.Metadata)
	assert.Equal(t, targetURL, mission.Metadata["target_url"])
	assert.Equal(t, agentName, mission.Metadata["agent"])
	assert.Equal(t, true, mission.Metadata["ephemeral"])
	assert.Equal(t, techniques, mission.Metadata["techniques"])
}

func TestCreateEphemeralMission_DefaultGoal(t *testing.T) {
	targetURL := "https://example.com"
	agentName := "testagent"

	mission := createEphemeralMission(targetURL, agentName, "", "")

	require.NotNil(t, mission)
	assert.Contains(t, mission.Description, "Quick attack against")
	assert.Contains(t, mission.Description, targetURL)

	// Should not have techniques in metadata if not provided
	_, hasTechniques := mission.Metadata["techniques"]
	assert.False(t, hasTechniques)
}

func TestCreateAttackTask(t *testing.T) {
	missionID := types.NewID()
	targetURL := "https://example.com"
	targetID := types.NewID()
	goal := "Security assessment"
	techniques := "sqli,xss,csrf"
	timeout := 30 * time.Minute

	task := createAttackTask(missionID, targetURL, &targetID, goal, techniques, timeout)

	assert.Equal(t, "attack", task.Name)
	assert.Equal(t, goal, task.Description)
	assert.Equal(t, missionID, *task.MissionID)
	assert.Equal(t, targetID, *task.TargetID)
	assert.Equal(t, timeout, task.Timeout)

	require.NotNil(t, task.Input)
	assert.Equal(t, targetURL, task.Input["target_url"])

	techniquesList, ok := task.Input["techniques"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"sqli", "xss", "csrf"}, techniquesList)
}

func TestCreateAttackTask_NoTechniques(t *testing.T) {
	missionID := types.NewID()
	targetURL := "https://example.com"
	timeout := 30 * time.Minute

	task := createAttackTask(missionID, targetURL, nil, "", "", timeout)

	require.NotNil(t, task.Input)
	assert.Equal(t, targetURL, task.Input["target_url"])

	// Should not have techniques if not provided
	_, hasTechniques := task.Input["techniques"]
	assert.False(t, hasTechniques)
}

func TestGetAuthTypeFromCredential(t *testing.T) {
	tests := []struct {
		name         string
		credType     types.CredentialType
		wantAuthType types.AuthType
	}{
		{
			name:         "API key",
			credType:     types.CredentialTypeAPIKey,
			wantAuthType: types.AuthTypeAPIKey,
		},
		{
			name:         "Bearer token",
			credType:     types.CredentialTypeBearer,
			wantAuthType: types.AuthTypeBearer,
		},
		{
			name:         "Basic auth",
			credType:     types.CredentialTypeBasic,
			wantAuthType: types.AuthTypeBasic,
		},
		{
			name:         "OAuth",
			credType:     types.CredentialTypeOAuth,
			wantAuthType: types.AuthTypeOAuth,
		},
		{
			name:         "Unknown type",
			credType:     types.CredentialType("unknown"),
			wantAuthType: types.AuthTypeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authType := getAuthTypeFromCredential(tt.credType)
			assert.Equal(t, tt.wantAuthType, authType)
		})
	}
}

func TestHasCriticalFindings(t *testing.T) {
	tests := []struct {
		name     string
		findings []agent.Finding
		want     bool
	}{
		{
			name:     "no findings",
			findings: []agent.Finding{},
			want:     false,
		},
		{
			name: "only low severity",
			findings: []agent.Finding{
				{Severity: agent.SeverityLow},
				{Severity: agent.SeverityInfo},
			},
			want: false,
		},
		{
			name: "has critical finding",
			findings: []agent.Finding{
				{Severity: agent.SeverityLow},
				{Severity: agent.SeverityCritical},
				{Severity: agent.SeverityHigh},
			},
			want: true,
		},
		{
			name: "only high severity",
			findings: []agent.Finding{
				{Severity: agent.SeverityHigh},
				{Severity: agent.SeverityMedium},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasCriticalFindings(tt.findings)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestDisplayAttackResults(t *testing.T) {
	tests := []struct {
		name    string
		result  agent.Result
		verbose bool
		wantOut []string
	}{
		{
			name: "successful completion no findings",
			result: agent.Result{
				Status: agent.ResultStatusCompleted,
				Metrics: agent.TaskMetrics{
					Duration: 5 * time.Second,
				},
			},
			verbose: false,
			wantOut: []string{
				"Attack completed with status: completed",
				"Duration: 5s",
				"No findings discovered",
			},
		},
		{
			name: "with findings",
			result: agent.Result{
				Status: agent.ResultStatusCompleted,
				Findings: []agent.Finding{
					{
						Title:    "SQL Injection",
						Severity: agent.SeverityHigh,
					},
					{
						Title:    "XSS Vulnerability",
						Severity: agent.SeverityCritical,
					},
				},
				Metrics: agent.TaskMetrics{
					Duration: 10 * time.Second,
				},
			},
			verbose: false,
			wantOut: []string{
				"Findings: 2",
				"SQL Injection",
				"XSS Vulnerability",
				"CRITICAL: 1",
				"HIGH: 1",
			},
		},
		{
			name: "verbose mode with metrics",
			result: agent.Result{
				Status: agent.ResultStatusCompleted,
				Metrics: agent.TaskMetrics{
					Duration:   10 * time.Second,
					LLMCalls:   5,
					ToolCalls:  3,
					TokensUsed: 1500,
					Cost:       0.0045,
				},
			},
			verbose: true,
			wantOut: []string{
				"LLM calls: 5",
				"Tool calls: 3",
				"Tokens used: 1500",
				"Cost: $0.0045",
			},
		},
		{
			name: "with error",
			result: agent.Result{
				Status: agent.ResultStatusFailed,
				Error: &agent.ResultError{
					Message: "Connection timeout",
				},
				Metrics: agent.TaskMetrics{
					Duration: 2 * time.Second,
				},
			},
			verbose: false,
			wantOut: []string{
				"Attack completed with status: failed",
				"Error: Connection timeout",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&buf)

			displayAttackResults(cmd, tt.result, tt.verbose)

			output := buf.String()
			for _, want := range tt.wantOut {
				assert.Contains(t, output, want)
			}
		})
	}
}

func TestExecuteAttackTask_ContextCancellation(t *testing.T) {
	agentComp := &component.Component{
		Name:   "testagent",
		Status: component.ComponentStatusRunning,
		PID:    1234,
	}

	task := agent.NewTask("test", "test task", map[string]any{})

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := &cobra.Command{}
	result, err := executeAttackTask(ctx, cmd, agentComp, task, false)

	// Should handle cancellation gracefully
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, agent.ResultStatusCancelled, result.Status)
}

func TestExecuteAttackTask_Timeout(t *testing.T) {
	agentComp := &component.Component{
		Name:   "testagent",
		Status: component.ComponentStatusRunning,
		PID:    1234,
	}

	task := agent.NewTask("test", "test task", map[string]any{})

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond) // Ensure timeout occurs

	cmd := &cobra.Command{}
	result, err := executeAttackTask(ctx, cmd, agentComp, task, false)

	// Should handle timeout
	assert.Error(t, err)
	assert.Equal(t, agent.ResultStatusCancelled, result.Status)
}

func TestTimePtr(t *testing.T) {
	now := time.Now()
	ptr := timePtr(now)

	require.NotNil(t, ptr)
	assert.Equal(t, now, *ptr)
}

// Mock component registry for testing
type mockComponentRegistry struct {
	components []*component.Component
}

func (m *mockComponentRegistry) List(kind component.ComponentKind) []*component.Component {
	var result []*component.Component
	for _, c := range m.components {
		if c.Kind == kind {
			result = append(result, c)
		}
	}
	return result
}

func (m *mockComponentRegistry) ListAll() map[component.ComponentKind][]*component.Component {
	result := make(map[component.ComponentKind][]*component.Component)
	for _, c := range m.components {
		result[c.Kind] = append(result[c.Kind], c)
	}
	return result
}

func (m *mockComponentRegistry) Get(kind component.ComponentKind, name string) *component.Component {
	for _, c := range m.components {
		if c.Kind == kind && c.Name == name {
			return c
		}
	}
	return nil
}

func (m *mockComponentRegistry) Register(comp *component.Component) error {
	m.components = append(m.components, comp)
	return nil
}

func (m *mockComponentRegistry) Unregister(kind component.ComponentKind, name string) error {
	return nil
}

func (m *mockComponentRegistry) Save() error {
	return nil
}

func (m *mockComponentRegistry) Load() error {
	return nil
}

func (m *mockComponentRegistry) LoadFromConfig(path string) error {
	return nil
}

// Benchmark tests
func BenchmarkCreateEphemeralMission(b *testing.B) {
	targetURL := "https://example.com"
	agentName := "testagent"
	goal := "Find vulnerabilities"
	techniques := "sqli,xss,csrf"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = createEphemeralMission(targetURL, agentName, goal, techniques)
	}
}

func BenchmarkCreateAttackTask(b *testing.B) {
	missionID := types.NewID()
	targetURL := "https://example.com"
	targetID := types.NewID()
	goal := "Security assessment"
	techniques := "sqli,xss,csrf"
	timeout := 30 * time.Minute

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = createAttackTask(missionID, targetURL, &targetID, goal, techniques, timeout)
	}
}

func BenchmarkHasCriticalFindings(b *testing.B) {
	findings := []agent.Finding{
		{Severity: agent.SeverityLow},
		{Severity: agent.SeverityMedium},
		{Severity: agent.SeverityHigh},
		{Severity: agent.SeverityCritical},
		{Severity: agent.SeverityInfo},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hasCriticalFindings(findings)
	}
}
