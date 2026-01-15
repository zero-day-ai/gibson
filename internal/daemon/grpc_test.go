package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/attack"
	"github.com/zero-day-ai/gibson/internal/daemon/api"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/registry"
	"github.com/zero-day-ai/gibson/internal/types"
)

// Stub implementations for other interface methods (not tested in this task)

// TestListAgents_Success tests ListAgents with mock registry adapter.
func TestListAgents_Success(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
			return []registry.AgentInfo{
				{
					Name:         "test-agent-1",
					Version:      "1.0.0",
					Endpoints:    []string{"localhost:50100"},
					Capabilities: []string{"llm", "web"},
					Instances:    1,
				},
				{
					Name:         "test-agent-2",
					Version:      "2.0.0",
					Endpoints:    []string{"localhost:50101", "localhost:50102"},
					Capabilities: []string{"cli"},
					Instances:    2,
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	agents, err := daemon.ListAgents(ctx, "")

	require.NoError(t, err)
	assert.Len(t, agents, 2)

	// Verify first agent
	assert.Equal(t, "test-agent-1", agents[0].Name)
	assert.Equal(t, "test-agent-1", agents[0].ID)
	assert.Equal(t, "1.0.0", agents[0].Version)
	assert.Equal(t, "localhost:50100", agents[0].Endpoint)
	assert.Equal(t, []string{"llm", "web"}, agents[0].Capabilities)
	assert.Equal(t, "healthy", agents[0].Health)

	// Verify second agent
	assert.Equal(t, "test-agent-2", agents[1].Name)
	assert.Equal(t, "2.0.0", agents[1].Version)
	assert.Equal(t, "localhost:50101", agents[1].Endpoint) // First endpoint used
	assert.Equal(t, "healthy", agents[1].Health)
}

// TestListAgents_EmptyResults tests ListAgents with no agents registered.
func TestListAgents_EmptyResults(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
			return []registry.AgentInfo{}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	agents, err := daemon.ListAgents(ctx, "")

	require.NoError(t, err)
	assert.Empty(t, agents)
}

// TestListAgents_NoInstances tests ListAgents with agents that have no instances.
func TestListAgents_NoInstances(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
			return []registry.AgentInfo{
				{
					Name:      "offline-agent",
					Version:   "1.0.0",
					Endpoints: []string{"localhost:50100"},
					Instances: 0, // No instances running
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	agents, err := daemon.ListAgents(ctx, "")

	require.NoError(t, err)
	assert.Len(t, agents, 1)
	assert.Equal(t, "unknown", agents[0].Health) // Should be unknown when no instances
}

// TestListAgents_RegistryError tests ListAgents error propagation from registry.
func TestListAgents_RegistryError(t *testing.T) {
	expectedErr := fmt.Errorf("registry connection failed")
	mockRegistry := &mockComponentDiscovery{
		listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
			return nil, expectedErr
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	agents, err := daemon.ListAgents(ctx, "")

	assert.Error(t, err)
	assert.Nil(t, agents)
	assert.Contains(t, err.Error(), "failed to list agents")
}

// TestGetAgentStatus_Success tests GetAgentStatus with existing agent.
func TestGetAgentStatus_Success(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
			return []registry.AgentInfo{
				{
					Name:         "target-agent",
					Version:      "1.5.0",
					Endpoints:    []string{"localhost:50200"},
					Capabilities: []string{"recon"},
					Instances:    3,
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	status, err := daemon.GetAgentStatus(ctx, "target-agent")

	require.NoError(t, err)
	assert.Equal(t, "target-agent", status.Agent.Name)
	assert.Equal(t, "1.5.0", status.Agent.Version)
	assert.Equal(t, "localhost:50200", status.Agent.Endpoint)
	assert.Equal(t, "healthy", status.Agent.Health)
	assert.True(t, status.Active) // Active because instances > 0
	assert.Empty(t, status.CurrentTask)
}

// TestGetAgentStatus_NotFound tests GetAgentStatus with non-existent agent.
func TestGetAgentStatus_NotFound(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
			return []registry.AgentInfo{
				{
					Name:    "other-agent",
					Version: "1.0.0",
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	status, err := daemon.GetAgentStatus(ctx, "nonexistent-agent")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent not found")
	assert.Contains(t, err.Error(), "nonexistent-agent")
	assert.Equal(t, api.AgentStatusInternal{}, status)
}

// TestGetAgentStatus_RegistryError tests GetAgentStatus error propagation.
func TestGetAgentStatus_RegistryError(t *testing.T) {
	expectedErr := fmt.Errorf("etcd timeout")
	mockRegistry := &mockComponentDiscovery{
		listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
			return nil, expectedErr
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	status, err := daemon.GetAgentStatus(ctx, "any-agent")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query registry")
	assert.Equal(t, api.AgentStatusInternal{}, status)
}

// TestListTools_Success tests ListTools with mock registry adapter.
func TestListTools_Success(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listToolsFunc: func(ctx context.Context) ([]registry.ToolInfo, error) {
			return []registry.ToolInfo{
				{
					Name:        "nmap",
					Version:     "7.92",
					Description: "Network scanner",
					Endpoints:   []string{"localhost:50300"},
					Instances:   1,
				},
				{
					Name:        "sqlmap",
					Version:     "1.5",
					Description: "SQL injection tool",
					Endpoints:   []string{"localhost:50301"},
					Instances:   1,
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	tools, err := daemon.ListTools(ctx)

	require.NoError(t, err)
	assert.Len(t, tools, 2)

	// Verify first tool
	assert.Equal(t, "nmap", tools[0].Name)
	assert.Equal(t, "nmap", tools[0].ID)
	assert.Equal(t, "7.92", tools[0].Version)
	assert.Equal(t, "Network scanner", tools[0].Description)
	assert.Equal(t, "localhost:50300", tools[0].Endpoint)
	assert.Equal(t, "healthy", tools[0].Health)

	// Verify second tool
	assert.Equal(t, "sqlmap", tools[1].Name)
	assert.Equal(t, "SQL injection tool", tools[1].Description)
}

// TestListTools_EmptyResults tests ListTools with no tools registered.
func TestListTools_EmptyResults(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listToolsFunc: func(ctx context.Context) ([]registry.ToolInfo, error) {
			return []registry.ToolInfo{}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	tools, err := daemon.ListTools(ctx)

	require.NoError(t, err)
	assert.Empty(t, tools)
}

// TestListTools_RegistryError tests ListTools error propagation from registry.
func TestListTools_RegistryError(t *testing.T) {
	expectedErr := fmt.Errorf("registry unavailable")
	mockRegistry := &mockComponentDiscovery{
		listToolsFunc: func(ctx context.Context) ([]registry.ToolInfo, error) {
			return nil, expectedErr
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	tools, err := daemon.ListTools(ctx)

	assert.Error(t, err)
	assert.Nil(t, tools)
	assert.Contains(t, err.Error(), "failed to list tools")
}

// TestListPlugins_Success tests ListPlugins with mock registry adapter.
func TestListPlugins_Success(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listPluginsFunc: func(ctx context.Context) ([]registry.PluginInfo, error) {
			return []registry.PluginInfo{
				{
					Name:        "mitre-lookup",
					Version:     "1.0.0",
					Description: "MITRE ATT&CK lookup plugin",
					Endpoints:   []string{"localhost:50400"},
					Instances:   1,
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	plugins, err := daemon.ListPlugins(ctx)

	require.NoError(t, err)
	assert.Len(t, plugins, 1)

	// Verify plugin
	assert.Equal(t, "mitre-lookup", plugins[0].Name)
	assert.Equal(t, "mitre-lookup", plugins[0].ID)
	assert.Equal(t, "1.0.0", plugins[0].Version)
	assert.Equal(t, "MITRE ATT&CK lookup plugin", plugins[0].Description)
	assert.Equal(t, "localhost:50400", plugins[0].Endpoint)
	assert.Equal(t, "healthy", plugins[0].Health)
}

// TestListPlugins_EmptyResults tests ListPlugins with no plugins registered.
func TestListPlugins_EmptyResults(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listPluginsFunc: func(ctx context.Context) ([]registry.PluginInfo, error) {
			return []registry.PluginInfo{}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	plugins, err := daemon.ListPlugins(ctx)

	require.NoError(t, err)
	assert.Empty(t, plugins)
}

// TestListPlugins_RegistryError tests ListPlugins error propagation from registry.
func TestListPlugins_RegistryError(t *testing.T) {
	expectedErr := fmt.Errorf("plugin registry error")
	mockRegistry := &mockComponentDiscovery{
		listPluginsFunc: func(ctx context.Context) ([]registry.PluginInfo, error) {
			return nil, expectedErr
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	plugins, err := daemon.ListPlugins(ctx)

	assert.Error(t, err)
	assert.Nil(t, plugins)
	assert.Contains(t, err.Error(), "failed to list plugins")
}

// TestListAgents_NoEndpoints tests handling of agents with no endpoints.
func TestListAgents_NoEndpoints(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
			return []registry.AgentInfo{
				{
					Name:      "no-endpoint-agent",
					Version:   "1.0.0",
					Endpoints: []string{}, // Empty endpoints
					Instances: 1,
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	agents, err := daemon.ListAgents(ctx, "")

	require.NoError(t, err)
	assert.Len(t, agents, 1)
	assert.Empty(t, agents[0].Endpoint)          // Should be empty string when no endpoints
	assert.Equal(t, "healthy", agents[0].Health) // Still healthy if instances > 0
}

// TestListTools_NoEndpoints tests handling of tools with no endpoints.
func TestListTools_NoEndpoints(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listToolsFunc: func(ctx context.Context) ([]registry.ToolInfo, error) {
			return []registry.ToolInfo{
				{
					Name:        "no-endpoint-tool",
					Version:     "1.0.0",
					Description: "Test tool",
					Endpoints:   nil, // Nil endpoints
					Instances:   1,
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	tools, err := daemon.ListTools(ctx)

	require.NoError(t, err)
	assert.Len(t, tools, 1)
	assert.Empty(t, tools[0].Endpoint) // Should be empty string when no endpoints
}

// TestGetAgentStatus_NoEndpoints tests GetAgentStatus with agent that has no endpoints.
func TestGetAgentStatus_NoEndpoints(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
			return []registry.AgentInfo{
				{
					Name:      "target-agent",
					Version:   "1.0.0",
					Endpoints: []string{}, // No endpoints
					Instances: 1,
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	status, err := daemon.GetAgentStatus(ctx, "target-agent")

	require.NoError(t, err)
	assert.Empty(t, status.Agent.Endpoint)
	assert.True(t, status.Active)
}

// TestListAgents_WithKindFilter tests ListAgents with kind parameter (even though not yet used).
func TestListAgents_WithKindFilter(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
			return []registry.AgentInfo{
				{
					Name:      "test-agent",
					Version:   "1.0.0",
					Instances: 1,
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	agents, err := daemon.ListAgents(ctx, "security") // Kind parameter passed but not yet used

	require.NoError(t, err)
	assert.Len(t, agents, 1)
	assert.Equal(t, "agent", agents[0].Kind) // Default kind
}

// TestHealthStatus_BasedOnInstances tests that health status is correctly determined by instance count.
func TestHealthStatus_BasedOnInstances(t *testing.T) {
	tests := []struct {
		name           string
		instances      int
		expectedHealth string
		expectedActive bool
	}{
		{
			name:           "healthy with instances",
			instances:      5,
			expectedHealth: "healthy",
			expectedActive: true,
		},
		{
			name:           "unknown with no instances",
			instances:      0,
			expectedHealth: "unknown",
			expectedActive: false,
		},
		{
			name:           "healthy with one instance",
			instances:      1,
			expectedHealth: "healthy",
			expectedActive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRegistry := &mockComponentDiscovery{
				listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
					return []registry.AgentInfo{
						{
							Name:      "test-agent",
							Version:   "1.0.0",
							Instances: tt.instances,
						},
					}, nil
				},
			}

			daemon := &daemonImpl{
				registryAdapter: mockRegistry,
				logger:          slog.Default(),
			}

			ctx := context.Background()

			// Test via ListAgents
			agents, err := daemon.ListAgents(ctx, "")
			require.NoError(t, err)
			assert.Len(t, agents, 1)
			assert.Equal(t, tt.expectedHealth, agents[0].Health)

			// Test via GetAgentStatus
			status, err := daemon.GetAgentStatus(ctx, "test-agent")
			require.NoError(t, err)
			assert.Equal(t, tt.expectedHealth, status.Agent.Health)
			assert.Equal(t, tt.expectedActive, status.Active)
		})
	}
}

// TestListTools_HealthStatus tests tool health status based on instances.
func TestListTools_HealthStatus(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listToolsFunc: func(ctx context.Context) ([]registry.ToolInfo, error) {
			return []registry.ToolInfo{
				{
					Name:      "healthy-tool",
					Instances: 2,
				},
				{
					Name:      "offline-tool",
					Instances: 0,
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	tools, err := daemon.ListTools(ctx)

	require.NoError(t, err)
	assert.Len(t, tools, 2)
	assert.Equal(t, "healthy", tools[0].Health)
	assert.Equal(t, "unknown", tools[1].Health)
}

// TestListPlugins_HealthStatus tests plugin health status based on instances.
func TestListPlugins_HealthStatus(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listPluginsFunc: func(ctx context.Context) ([]registry.PluginInfo, error) {
			return []registry.PluginInfo{
				{
					Name:      "active-plugin",
					Instances: 1,
				},
				{
					Name:      "inactive-plugin",
					Instances: 0,
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	plugins, err := daemon.ListPlugins(ctx)

	require.NoError(t, err)
	assert.Len(t, plugins, 2)
	assert.Equal(t, "healthy", plugins[0].Health)
	assert.Equal(t, "unknown", plugins[1].Health)
}

// TestLastSeenTime tests that LastSeen is populated (currently uses time.Now()).
func TestLastSeenTime(t *testing.T) {
	mockRegistry := &mockComponentDiscovery{
		listAgentsFunc: func(ctx context.Context) ([]registry.AgentInfo, error) {
			return []registry.AgentInfo{
				{
					Name:      "test-agent",
					Version:   "1.0.0",
					Instances: 1,
				},
			}, nil
		},
	}

	daemon := &daemonImpl{
		registryAdapter: mockRegistry,
		logger:          slog.Default(),
	}

	ctx := context.Background()
	beforeTime := time.Now().Add(-1 * time.Second)

	agents, err := daemon.ListAgents(ctx, "")
	require.NoError(t, err)

	afterTime := time.Now().Add(1 * time.Second)

	assert.Len(t, agents, 1)
	assert.True(t, agents[0].LastSeen.After(beforeTime))
	assert.True(t, agents[0].LastSeen.Before(afterTime))
}

// mockAttackRunner is a mock implementation of attack.AttackRunner for testing
type mockAttackRunner struct {
	runFunc func(ctx context.Context, opts *attack.AttackOptions) (*attack.AttackResult, error)
}

func (m *mockAttackRunner) Run(ctx context.Context, opts *attack.AttackOptions) (*attack.AttackResult, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, opts)
	}
	return attack.NewAttackResult(), nil
}

// TestRunAttack_Success tests successful attack execution with findings
func TestRunAttack_Success(t *testing.T) {
	// Create mock attack runner that returns findings
	mockRunner := &mockAttackRunner{
		runFunc: func(ctx context.Context, opts *attack.AttackOptions) (*attack.AttackResult, error) {
			result := attack.NewAttackResult()
			result.Status = attack.AttackStatusFindings
			result.Duration = 5 * time.Second

			// Add a test finding
			testFinding := finding.EnhancedFinding{
				Finding: agent.Finding{
					ID:          types.NewID(),
					Title:       "Test Vulnerability",
					Description: "Test vulnerability description",
					Severity:    agent.SeverityHigh,
					Category:    "injection",
					CreatedAt:   time.Now(),
				},
			}
			result.AddFindings([]finding.EnhancedFinding{testFinding})

			return result, nil
		},
	}

	daemon := &daemonImpl{
		attackRunner: mockRunner,
		logger:       slog.Default(),
	}

	req := api.AttackRequest{
		Target:     "https://example.com/api",
		AgentID:    "test-agent",
		AttackType: "injection",
	}

	ctx := context.Background()
	eventChan, err := daemon.RunAttack(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, eventChan)

	// Collect events
	var events []api.AttackEventData
	for event := range eventChan {
		events = append(events, event)
	}

	// Verify events
	require.GreaterOrEqual(t, len(events), 3) // started, finding, completed

	// Check attack started event
	assert.Equal(t, "attack.started", events[0].EventType)
	assert.Contains(t, events[0].Message, "Starting attack")

	// Check finding discovered event
	var foundFindingEvent bool
	for _, event := range events {
		if event.EventType == "attack.finding" {
			foundFindingEvent = true
			assert.NotNil(t, event.Finding)
			assert.Equal(t, "Test Vulnerability", event.Finding.Title)
			assert.Equal(t, "high", event.Finding.Severity)
			break
		}
	}
	assert.True(t, foundFindingEvent, "expected attack.finding event")

	// Check attack completed event
	lastEvent := events[len(events)-1]
	assert.Equal(t, "attack.completed", lastEvent.EventType)
	assert.Contains(t, lastEvent.Message, "completed")
	assert.NotNil(t, lastEvent.Result, "expected Result field in attack.completed event")
	assert.Equal(t, "findings", lastEvent.Result.Status)      // AttackStatusFindings
	assert.Equal(t, int64(5000), lastEvent.Result.DurationMs) // 5 seconds
	assert.Equal(t, int32(1), lastEvent.Result.FindingsCount)
}

// TestRunAttack_ValidationError tests attack request validation
func TestRunAttack_ValidationError(t *testing.T) {
	daemon := &daemonImpl{
		attackRunner: &mockAttackRunner{},
		logger:       slog.Default(),
	}

	tests := []struct {
		name    string
		req     api.AttackRequest
		wantErr string
	}{
		{
			name: "missing target",
			req: api.AttackRequest{
				AgentID: "test-agent",
			},
			wantErr: "either target or target_name is required",
		},
		{
			name: "missing agent ID",
			req: api.AttackRequest{
				Target: "https://example.com",
			},
			wantErr: "agent ID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			eventChan, err := daemon.RunAttack(ctx, tt.req)

			assert.Error(t, err)
			assert.Nil(t, eventChan)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestRunAttack_RunnerNotInitialized tests behavior when attack runner is not initialized
func TestRunAttack_RunnerNotInitialized(t *testing.T) {
	daemon := &daemonImpl{
		attackRunner: nil, // No runner initialized
		logger:       slog.Default(),
	}

	req := api.AttackRequest{
		Target:  "https://example.com/api",
		AgentID: "test-agent",
	}

	ctx := context.Background()
	eventChan, err := daemon.RunAttack(ctx, req)

	assert.Error(t, err)
	assert.Nil(t, eventChan)
	assert.Contains(t, err.Error(), "runner not initialized")
}

// TestRunAttack_ExecutionError tests attack execution error handling
func TestRunAttack_ExecutionError(t *testing.T) {
	// Create mock attack runner that returns an error
	mockRunner := &mockAttackRunner{
		runFunc: func(ctx context.Context, opts *attack.AttackOptions) (*attack.AttackResult, error) {
			return nil, fmt.Errorf("target unreachable")
		},
	}

	daemon := &daemonImpl{
		attackRunner: mockRunner,
		logger:       slog.Default(),
	}

	req := api.AttackRequest{
		Target:  "https://example.com/api",
		AgentID: "test-agent",
	}

	ctx := context.Background()
	eventChan, err := daemon.RunAttack(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, eventChan)

	// Collect events
	var events []api.AttackEventData
	for event := range eventChan {
		events = append(events, event)
	}

	// Verify error event
	require.GreaterOrEqual(t, len(events), 2) // started, error

	// Check attack started event
	assert.Equal(t, "attack.started", events[0].EventType)

	// Check error event
	assert.Equal(t, "attack.failed", events[len(events)-1].EventType)
	assert.Contains(t, events[len(events)-1].Error, "target unreachable")
}

// TestRunAttack_OptionsMapping tests conversion from API request to attack options
func TestRunAttack_OptionsMapping(t *testing.T) {
	var capturedOpts *attack.AttackOptions

	mockRunner := &mockAttackRunner{
		runFunc: func(ctx context.Context, opts *attack.AttackOptions) (*attack.AttackResult, error) {
			capturedOpts = opts
			return attack.NewAttackResult(), nil
		},
	}

	daemon := &daemonImpl{
		attackRunner: mockRunner,
		logger:       slog.Default(),
	}

	req := api.AttackRequest{
		Target:        "https://example.com/api",
		AgentID:       "test-agent",
		AttackType:    "injection",
		PayloadFilter: "sql",
		Options: map[string]string{
			"max_turns": "10",
			"timeout":   "5m",
			"verbose":   "true",
		},
	}

	ctx := context.Background()
	eventChan, err := daemon.RunAttack(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, eventChan)

	// Drain the channel
	for range eventChan {
	}

	// Verify options were mapped correctly
	require.NotNil(t, capturedOpts)
	assert.Equal(t, "https://example.com/api", capturedOpts.TargetURL)
	assert.Equal(t, "test-agent", capturedOpts.AgentName)
	assert.Equal(t, "sql", capturedOpts.PayloadCategory)
	assert.Equal(t, 10, capturedOpts.MaxTurns)
	assert.Equal(t, 5*time.Minute, capturedOpts.Timeout)
	assert.True(t, capturedOpts.Verbose)
}

// TestRunAttack_NoFindings tests attack execution with no findings
func TestRunAttack_NoFindings(t *testing.T) {
	mockRunner := &mockAttackRunner{
		runFunc: func(ctx context.Context, opts *attack.AttackOptions) (*attack.AttackResult, error) {
			result := attack.NewAttackResult()
			result.Status = attack.AttackStatusSuccess
			result.Duration = 3 * time.Second
			return result, nil
		},
	}

	daemon := &daemonImpl{
		attackRunner: mockRunner,
		logger:       slog.Default(),
	}

	req := api.AttackRequest{
		Target:  "https://example.com/api",
		AgentID: "test-agent",
	}

	ctx := context.Background()
	eventChan, err := daemon.RunAttack(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, eventChan)

	// Collect events
	var events []api.AttackEventData
	for event := range eventChan {
		events = append(events, event)
	}

	// Verify events - should have started and completed, no finding events
	require.Equal(t, 2, len(events))
	assert.Equal(t, "attack.started", events[0].EventType)
	assert.Equal(t, "attack.completed", events[1].EventType)
	assert.Contains(t, events[1].Message, "0 findings")
}

// TestValidateAttackRequest tests the attack request validation logic
func TestValidateAttackRequest(t *testing.T) {
	daemon := &daemonImpl{
		logger: slog.Default(),
	}

	tests := []struct {
		name    string
		req     api.AttackRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: api.AttackRequest{
				Target:  "https://example.com",
				AgentID: "test-agent",
			},
			wantErr: false,
		},
		{
			name: "missing target",
			req: api.AttackRequest{
				AgentID: "test-agent",
			},
			wantErr: true,
		},
		{
			name: "missing agent ID",
			req: api.AttackRequest{
				Target: "https://example.com",
			},
			wantErr: true,
		},
		{
			name:    "empty request",
			req:     api.AttackRequest{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := daemon.validateAttackRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestBuildAttackOptions tests conversion from API request to internal attack options
func TestBuildAttackOptions(t *testing.T) {
	daemon := &daemonImpl{
		logger: slog.Default(),
	}

	tests := []struct {
		name  string
		req   api.AttackRequest
		check func(t *testing.T, opts *attack.AttackOptions)
	}{
		{
			name: "basic request",
			req: api.AttackRequest{
				Target:  "https://example.com",
				AgentID: "test-agent",
			},
			check: func(t *testing.T, opts *attack.AttackOptions) {
				assert.Equal(t, "https://example.com", opts.TargetURL)
				assert.Equal(t, "test-agent", opts.AgentName)
			},
		},
		{
			name: "with attack type",
			req: api.AttackRequest{
				Target:     "https://example.com",
				AgentID:    "test-agent",
				AttackType: "sql-injection",
			},
			check: func(t *testing.T, opts *attack.AttackOptions) {
				assert.Equal(t, "test-agent", opts.AgentName)
			},
		},
		{
			name: "with payload filter",
			req: api.AttackRequest{
				Target:        "https://example.com",
				AgentID:       "test-agent",
				PayloadFilter: "xss",
			},
			check: func(t *testing.T, opts *attack.AttackOptions) {
				assert.Equal(t, "xss", opts.PayloadCategory)
			},
		},
		{
			name: "with options",
			req: api.AttackRequest{
				Target:  "https://example.com",
				AgentID: "test-agent",
				Options: map[string]string{
					"max_turns": "15",
					"timeout":   "10m",
					"verbose":   "true",
					"dry_run":   "true",
				},
			},
			check: func(t *testing.T, opts *attack.AttackOptions) {
				assert.Equal(t, 15, opts.MaxTurns)
				assert.Equal(t, 10*time.Minute, opts.Timeout)
				assert.True(t, opts.Verbose)
				assert.True(t, opts.DryRun)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := daemon.buildAttackOptions(tt.req)
			require.NoError(t, err)
			require.NotNil(t, opts)
			tt.check(t, opts)
		})
	}
}

// mockTargetDAO is a mock implementation of TargetDAO for testing
type mockTargetDAO struct {
	getByNameFunc func(ctx context.Context, name string) (*types.Target, error)
	getFunc       func(ctx context.Context, id types.ID) (*types.Target, error)
}

func (m *mockTargetDAO) GetByName(ctx context.Context, name string) (*types.Target, error) {
	if m.getByNameFunc != nil {
		return m.getByNameFunc(ctx, name)
	}
	return nil, fmt.Errorf("target not found")
}

func (m *mockTargetDAO) Get(ctx context.Context, id types.ID) (*types.Target, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return nil, fmt.Errorf("target not found")
}

// TestValidateAttackRequest_TargetName tests validation with target_name field
func TestValidateAttackRequest_TargetName(t *testing.T) {
	daemon := &daemonImpl{
		logger: slog.Default(),
	}

	tests := []struct {
		name    string
		req     api.AttackRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request with target_name",
			req: api.AttackRequest{
				TargetName: "demo-target",
				AgentID:    "test-agent",
			},
			wantErr: false,
		},
		{
			name: "valid request with inline target",
			req: api.AttackRequest{
				Target:  "https://example.com",
				AgentID: "test-agent",
			},
			wantErr: false,
		},
		{
			name: "both target and target_name specified",
			req: api.AttackRequest{
				Target:     "https://example.com",
				TargetName: "demo-target",
				AgentID:    "test-agent",
			},
			wantErr: true,
			errMsg:  "cannot specify both",
		},
		{
			name: "neither target nor target_name specified",
			req: api.AttackRequest{
				AgentID: "test-agent",
			},
			wantErr: true,
			errMsg:  "either target or target_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := daemon.validateAttackRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestBuildAttackOptions_TargetNameResolution tests target name lookup
func TestBuildAttackOptions_TargetNameResolution(t *testing.T) {
	tests := []struct {
		name      string
		req       api.AttackRequest
		mockSetup func() *mockTargetDAO
		wantErr   bool
		check     func(t *testing.T, opts *attack.AttackOptions)
	}{
		{
			name: "resolve target by name",
			req: api.AttackRequest{
				TargetName: "demo-target",
				AgentID:    "test-agent",
			},
			mockSetup: func() *mockTargetDAO {
				return &mockTargetDAO{
					getByNameFunc: func(ctx context.Context, name string) (*types.Target, error) {
						return &types.Target{
							ID:   types.NewID(),
							Name: "demo-target",
							Type: "http_api",
							Connection: map[string]any{
								"url": "https://api.example.com/v1/chat",
							},
						}, nil
					},
				}
			},
			wantErr: false,
			check: func(t *testing.T, opts *attack.AttackOptions) {
				assert.Equal(t, "https://api.example.com/v1/chat", opts.TargetURL)
				assert.Equal(t, "demo-target", opts.TargetName)
				assert.Equal(t, "http_api", string(opts.TargetType))
			},
		},
		{
			name: "target not found error",
			req: api.AttackRequest{
				TargetName: "missing-target",
				AgentID:    "test-agent",
			},
			mockSetup: func() *mockTargetDAO {
				return &mockTargetDAO{
					getByNameFunc: func(ctx context.Context, name string) (*types.Target, error) {
						return nil, fmt.Errorf("target not found")
					},
				}
			},
			wantErr: true,
		},
		{
			name: "target with no URL",
			req: api.AttackRequest{
				TargetName: "no-url-target",
				AgentID:    "test-agent",
			},
			mockSetup: func() *mockTargetDAO {
				return &mockTargetDAO{
					getByNameFunc: func(ctx context.Context, name string) (*types.Target, error) {
						return &types.Target{
							ID:         types.NewID(),
							Name:       "no-url-target",
							Type:       "http_api",
							Connection: map[string]any{},
						}, nil
					},
				}
			},
			wantErr: true,
		},
		{
			name: "backward compatibility with inline target",
			req: api.AttackRequest{
				Target:  "https://example.com",
				AgentID: "test-agent",
			},
			mockSetup: func() *mockTargetDAO {
				return &mockTargetDAO{}
			},
			wantErr: false,
			check: func(t *testing.T, opts *attack.AttackOptions) {
				assert.Equal(t, "https://example.com", opts.TargetURL)
				assert.Equal(t, "", opts.TargetName)
			},
		},
		{
			name: "target with credential",
			req: api.AttackRequest{
				TargetName: "target-with-cred",
				AgentID:    "test-agent",
			},
			mockSetup: func() *mockTargetDAO {
				credID := types.NewID()
				return &mockTargetDAO{
					getByNameFunc: func(ctx context.Context, name string) (*types.Target, error) {
						return &types.Target{
							ID:   types.NewID(),
							Name: "target-with-cred",
							Type: "http_api",
							Connection: map[string]any{
								"url": "https://api.example.com/v1/chat",
							},
							CredentialID: &credID,
						}, nil
					},
				}
			},
			wantErr: false,
			check: func(t *testing.T, opts *attack.AttackOptions) {
				assert.Equal(t, "https://api.example.com/v1/chat", opts.TargetURL)
				assert.NotEmpty(t, opts.Credential)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDAO := tt.mockSetup()
			daemon := &daemonImpl{
				targetStore: mockDAO,
				logger:      slog.Default(),
			}

			opts, err := daemon.buildAttackOptions(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, opts)
				if tt.check != nil {
					tt.check(t, opts)
				}
			}
		})
	}
}

// TestBuildAttackOptions_TargetPropagation tests that target info is correctly propagated
func TestBuildAttackOptions_TargetPropagation(t *testing.T) {
	mockDAO := &mockTargetDAO{
		getByNameFunc: func(ctx context.Context, name string) (*types.Target, error) {
			return &types.Target{
				ID:   types.NewID(),
				Name: "test-target",
				Type: "http_api",
				Connection: map[string]any{
					"url": "https://api.example.com",
				},
			}, nil
		},
	}

	tests := []struct {
		name         string
		req          api.AttackRequest
		expectedURL  string
	}{
		{
			name: "target from database",
			req: api.AttackRequest{
				TargetName: "test-target",
				AgentID:    "test-agent",
				AttackType: "sql-injection",
			},
			expectedURL: "https://api.example.com",
		},
		{
			name: "direct target URL",
			req: api.AttackRequest{
				Target:     "https://direct.example.com",
				AgentID:    "test-agent",
				AttackType: "prompt-injection",
			},
			expectedURL: "https://direct.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			daemon := &daemonImpl{
				targetStore: mockDAO,
				logger:      slog.Default(),
			}

			opts, err := daemon.buildAttackOptions(tt.req)
			require.NoError(t, err)
			require.NotNil(t, opts)
			assert.Equal(t, tt.expectedURL, opts.TargetURL)
		})
	}
}
