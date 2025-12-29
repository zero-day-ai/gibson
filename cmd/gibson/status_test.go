package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestStatusCommand(t *testing.T) {
	tests := []struct {
		name           string
		setupFunc      func(t *testing.T, homeDir string)
		args           []string
		expectedOutput []string
		expectError    bool
	}{
		{
			name: "empty system - no database",
			setupFunc: func(t *testing.T, homeDir string) {
				// Don't create anything
			},
			args: []string{},
			expectedOutput: []string{
				"Overall Status:",
				"Database:",
				"Not connected",
				"LLM Providers:",
			},
			expectError: false,
		},
		{
			name: "initialized system with database",
			setupFunc: func(t *testing.T, homeDir string) {
				// Create database
				dbPath := filepath.Join(homeDir, "gibson.db")
				db, err := database.Open(dbPath)
				require.NoError(t, err)
				require.NoError(t, db.InitSchema())
				db.Close()
			},
			args: []string{},
			expectedOutput: []string{
				"Overall Status:",
				"Database:",
				"Connected:",
			},
			expectError: false,
		},
		{
			name: "system with components",
			setupFunc: func(t *testing.T, homeDir string) {
				// Create database
				dbPath := filepath.Join(homeDir, "gibson.db")
				db, err := database.Open(dbPath)
				require.NoError(t, err)
				require.NoError(t, db.InitSchema())
				db.Close()

				// Create component registry with test components
				registry := component.NewDefaultComponentRegistry()

				// Add a running agent
				agent := &component.Component{
					Kind:      component.ComponentKindAgent,
					Name:      "test-agent",
					Status:    component.ComponentStatusRunning,
					Port:      50001,
					PID:       12345,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Manifest: &component.Manifest{
						Name:    "test-agent",
						Version: "1.0.0",
						Kind:    component.ComponentKindAgent,
					},
				}
				require.NoError(t, registry.Register(agent))

				// Add a stopped plugin
				plugin := &component.Component{
					Kind:      component.ComponentKindPlugin,
					Name:      "test-plugin",
					Status:    component.ComponentStatusStopped,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Manifest: &component.Manifest{
						Name:    "test-plugin",
						Version: "1.0.0",
						Kind:    component.ComponentKindPlugin,
					},
				}
				require.NoError(t, registry.Register(plugin))

				// Save registry
				require.NoError(t, registry.Save())
			},
			args: []string{},
			expectedOutput: []string{
				"Overall Status:",
				"Database:",
				"Connected:",
				"Agents:",
				"test-agent",
				"Plugins:",
				"test-plugin",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tmpDir := t.TempDir()

			// Set GIBSON_HOME
			oldHome := os.Getenv("GIBSON_HOME")
			os.Setenv("GIBSON_HOME", tmpDir)
			defer os.Setenv("GIBSON_HOME", oldHome)

			// Setup test environment
			if tt.setupFunc != nil {
				tt.setupFunc(t, tmpDir)
			}

			// Create command
			cmd := statusCmd
			cmd.SetContext(context.Background())

			// Capture output
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)

			// Set args
			cmd.SetArgs(tt.args)

			// Execute command
			err := cmd.Execute()

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Check output contains expected strings
			output := stdout.String()
			for _, expected := range tt.expectedOutput {
				assert.Contains(t, output, expected, "Output should contain: %s", expected)
			}
		})
	}
}

func TestStatusCommandJSON(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, homeDir string)
		expectError bool
		validate    func(t *testing.T, status SystemStatus)
	}{
		{
			name: "json output - empty system",
			setupFunc: func(t *testing.T, homeDir string) {
				// Empty system
			},
			expectError: false,
			validate: func(t *testing.T, status SystemStatus) {
				assert.NotNil(t, status)
				assert.False(t, status.Database.Connected)
				assert.NotEmpty(t, status.LLMProviders)
			},
		},
		{
			name: "json output - with database",
			setupFunc: func(t *testing.T, homeDir string) {
				// Create database
				dbPath := filepath.Join(homeDir, "gibson.db")
				db, err := database.Open(dbPath)
				require.NoError(t, err)
				require.NoError(t, db.InitSchema())
				db.Close()
			},
			expectError: false,
			validate: func(t *testing.T, status SystemStatus) {
				assert.True(t, status.Database.Connected)
				assert.Greater(t, status.Database.Size, int64(0))
			},
		},
		{
			name: "json output - with components",
			setupFunc: func(t *testing.T, homeDir string) {
				// Create database
				dbPath := filepath.Join(homeDir, "gibson.db")
				db, err := database.Open(dbPath)
				require.NoError(t, err)
				require.NoError(t, db.InitSchema())
				db.Close()

				// Create component registry
				registry := component.NewDefaultComponentRegistry()

				// Add components
				agent := &component.Component{
					Kind:      component.ComponentKindAgent,
					Name:      "test-agent",
					Status:    component.ComponentStatusRunning,
					Port:      50001,
					PID:       12345,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Manifest: &component.Manifest{
						Name:    "test-agent",
						Version: "1.0.0",
						Kind:    component.ComponentKindAgent,
					},
				}
				require.NoError(t, registry.Register(agent))

				tool := &component.Component{
					Kind:      component.ComponentKindTool,
					Name:      "test-tool",
					Status:    component.ComponentStatusStopped,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Manifest: &component.Manifest{
						Name:    "test-tool",
						Version: "1.0.0",
						Kind:    component.ComponentKindTool,
					},
				}
				require.NoError(t, registry.Register(tool))

				require.NoError(t, registry.Save())
			},
			expectError: false,
			validate: func(t *testing.T, status SystemStatus) {
				assert.True(t, status.Database.Connected)
				assert.Len(t, status.Components.Agents, 1)
				assert.Len(t, status.Components.Tools, 1)
				assert.Equal(t, "test-agent", status.Components.Agents[0].Name)
				assert.Equal(t, "running", status.Components.Agents[0].Status)
				assert.Equal(t, 50001, status.Components.Agents[0].Port)
				assert.Equal(t, 12345, status.Components.Agents[0].PID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tmpDir := t.TempDir()

			// Set GIBSON_HOME
			oldHome := os.Getenv("GIBSON_HOME")
			os.Setenv("GIBSON_HOME", tmpDir)
			defer os.Setenv("GIBSON_HOME", oldHome)

			// Setup test environment
			if tt.setupFunc != nil {
				tt.setupFunc(t, tmpDir)
			}

			// Create command
			cmd := statusCmd
			cmd.SetContext(context.Background())

			// Capture output
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)

			// Set args with --json flag
			cmd.SetArgs([]string{"--json"})

			// Execute command
			err := cmd.Execute()

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Parse JSON output
			var status SystemStatus
			err = json.Unmarshal(stdout.Bytes(), &status)
			require.NoError(t, err, "JSON output should be valid")

			// Validate status
			if tt.validate != nil {
				tt.validate(t, status)
			}
		})
	}
}

func TestCollectSystemStatus(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T, homeDir string)
		validate  func(t *testing.T, status SystemStatus)
	}{
		{
			name: "collect empty system status",
			setupFunc: func(t *testing.T, homeDir string) {
				// Nothing to setup
			},
			validate: func(t *testing.T, status SystemStatus) {
				assert.NotZero(t, status.CheckedAt)
				assert.False(t, status.Database.Connected)
				assert.NotEmpty(t, status.LLMProviders)
			},
		},
		{
			name: "collect with database",
			setupFunc: func(t *testing.T, homeDir string) {
				dbPath := filepath.Join(homeDir, "gibson.db")
				db, err := database.Open(dbPath)
				require.NoError(t, err)
				require.NoError(t, db.InitSchema())
				db.Close()
			},
			validate: func(t *testing.T, status SystemStatus) {
				assert.True(t, status.Database.Connected)
				assert.Greater(t, status.Database.Size, int64(0))
			},
		},
		{
			name: "collect with components",
			setupFunc: func(t *testing.T, homeDir string) {
				registry := component.NewDefaultComponentRegistry()
				comp := &component.Component{
					Kind:      component.ComponentKindAgent,
					Name:      "test",
					Status:    component.ComponentStatusRunning,
					Port:      50001,
					PID:       999,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Manifest: &component.Manifest{
						Name:    "test",
						Version: "1.0.0",
						Kind:    component.ComponentKindAgent,
					},
				}
				require.NoError(t, registry.Register(comp))
				require.NoError(t, registry.Save())
			},
			validate: func(t *testing.T, status SystemStatus) {
				assert.Len(t, status.Components.Agents, 1)
				assert.Equal(t, "test", status.Components.Agents[0].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.setupFunc != nil {
				tt.setupFunc(t, tmpDir)
			}

			ctx := context.Background()
			status := collectSystemStatus(ctx, tmpDir)

			if tt.validate != nil {
				tt.validate(t, status)
			}
		})
	}
}

func TestDetermineOverallHealth(t *testing.T) {
	tests := []struct {
		name           string
		status         SystemStatus
		expectedState  types.HealthState
		expectedInMsg  string
	}{
		{
			name: "healthy - database and provider ok",
			status: SystemStatus{
				Database: DatabaseStatus{
					Connected: true,
				},
				LLMProviders: []LLMProviderStatus{
					{
						Name:         "test-provider",
						Configured:   true,
						HealthStatus: types.Healthy("ok"),
					},
				},
				Components: ComponentsStatus{
					Agents: []ComponentInfo{},
				},
			},
			expectedState: types.HealthStateHealthy,
			expectedInMsg: "system initialized",
		},
		{
			name: "degraded - database ok, no provider",
			status: SystemStatus{
				Database: DatabaseStatus{
					Connected: true,
				},
				LLMProviders: []LLMProviderStatus{
					{
						Name:         "test-provider",
						Configured:   false,
						HealthStatus: types.Unhealthy("not configured"),
					},
				},
				Components: ComponentsStatus{},
			},
			expectedState: types.HealthStateDegraded,
			expectedInMsg: "no healthy LLM providers",
		},
		{
			name: "unhealthy - database down",
			status: SystemStatus{
				Database: DatabaseStatus{
					Connected: false,
					Error:     "connection failed",
				},
				LLMProviders: []LLMProviderStatus{
					{
						Name:         "test-provider",
						Configured:   false,
						HealthStatus: types.Unhealthy("not configured"),
					},
				},
				Components: ComponentsStatus{},
			},
			expectedState: types.HealthStateUnhealthy,
			expectedInMsg: "database unavailable",
		},
		{
			name: "degraded - components not running",
			status: SystemStatus{
				Database: DatabaseStatus{
					Connected: true,
				},
				LLMProviders: []LLMProviderStatus{
					{
						Name:         "test-provider",
						Configured:   true,
						HealthStatus: types.Healthy("ok"),
					},
				},
				Components: ComponentsStatus{
					Agents: []ComponentInfo{
						{
							Name:   "test-agent",
							Status: "stopped",
						},
					},
				},
			},
			expectedState: types.HealthStateDegraded,
			expectedInMsg: "no components running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := determineOverallHealth(tt.status)

			assert.Equal(t, tt.expectedState, health.State)
			assert.Contains(t, health.Message, tt.expectedInMsg)
		})
	}
}

func TestCheckComponentsStatus(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T, homeDir string)
		validate  func(t *testing.T, status ComponentsStatus)
	}{
		{
			name: "no registry file",
			setupFunc: func(t *testing.T, homeDir string) {
				// Don't create registry
			},
			validate: func(t *testing.T, status ComponentsStatus) {
				assert.Empty(t, status.Agents)
				assert.Empty(t, status.Tools)
				assert.Empty(t, status.Plugins)
			},
		},
		{
			name: "registry with multiple components",
			setupFunc: func(t *testing.T, homeDir string) {
				registry := component.NewDefaultComponentRegistry()

				// Add agent
				agent := &component.Component{
					Kind:      component.ComponentKindAgent,
					Name:      "agent1",
					Status:    component.ComponentStatusRunning,
					Port:      50001,
					PID:       111,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Manifest: &component.Manifest{
						Name:    "agent1",
						Version: "1.0.0",
						Kind:    component.ComponentKindAgent,
					},
				}
				require.NoError(t, registry.Register(agent))

				// Add tool
				tool := &component.Component{
					Kind:      component.ComponentKindTool,
					Name:      "tool1",
					Status:    component.ComponentStatusStopped,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Manifest: &component.Manifest{
						Name:    "tool1",
						Version: "1.0.0",
						Kind:    component.ComponentKindTool,
					},
				}
				require.NoError(t, registry.Register(tool))

				// Add plugin
				plugin := &component.Component{
					Kind:      component.ComponentKindPlugin,
					Name:      "plugin1",
					Status:    component.ComponentStatusError,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Manifest: &component.Manifest{
						Name:    "plugin1",
						Version: "1.0.0",
						Kind:    component.ComponentKindPlugin,
					},
				}
				require.NoError(t, registry.Register(plugin))

				require.NoError(t, registry.Save())
			},
			validate: func(t *testing.T, status ComponentsStatus) {
				assert.Len(t, status.Agents, 1)
				assert.Len(t, status.Tools, 1)
				assert.Len(t, status.Plugins, 1)

				assert.Equal(t, "agent1", status.Agents[0].Name)
				assert.Equal(t, "running", status.Agents[0].Status)
				assert.Equal(t, 50001, status.Agents[0].Port)
				assert.Equal(t, 111, status.Agents[0].PID)

				assert.Equal(t, "tool1", status.Tools[0].Name)
				assert.Equal(t, "stopped", status.Tools[0].Status)

				assert.Equal(t, "plugin1", status.Plugins[0].Name)
				assert.Equal(t, "error", status.Plugins[0].Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.setupFunc != nil {
				tt.setupFunc(t, tmpDir)
			}

			status := checkComponentsStatus(tmpDir)

			if tt.validate != nil {
				tt.validate(t, status)
			}
		})
	}
}

func TestCheckDatabaseStatus(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T, homeDir string)
		validate  func(t *testing.T, status DatabaseStatus)
	}{
		{
			name: "database not exists",
			setupFunc: func(t *testing.T, homeDir string) {
				// Don't create database
			},
			validate: func(t *testing.T, status DatabaseStatus) {
				assert.False(t, status.Connected)
				assert.Contains(t, status.Error, "not found")
			},
		},
		{
			name: "database exists and healthy",
			setupFunc: func(t *testing.T, homeDir string) {
				dbPath := filepath.Join(homeDir, "gibson.db")
				db, err := database.Open(dbPath)
				require.NoError(t, err)
				require.NoError(t, db.InitSchema())
				db.Close()
			},
			validate: func(t *testing.T, status DatabaseStatus) {
				assert.True(t, status.Connected)
				assert.Greater(t, status.Size, int64(0))
				assert.Empty(t, status.Error)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.setupFunc != nil {
				tt.setupFunc(t, tmpDir)
			}

			ctx := context.Background()
			status := checkDatabaseStatus(ctx, tmpDir)

			if tt.validate != nil {
				tt.validate(t, status)
			}
		})
	}
}

func TestCheckLLMProviders(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T, homeDir string)
		validate  func(t *testing.T, providers []LLMProviderStatus)
	}{
		{
			name: "no config file",
			setupFunc: func(t *testing.T, homeDir string) {
				// Don't create config
			},
			validate: func(t *testing.T, providers []LLMProviderStatus) {
				assert.Empty(t, providers)
			},
		},
		{
			name: "config with no providers",
			setupFunc: func(t *testing.T, homeDir string) {
				configPath := config.DefaultConfigPath(homeDir)
				require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
				configContent := `
core:
  home_dir: /tmp/gibson
  data_dir: /tmp/gibson/data
  cache_dir: /tmp/gibson/cache
  parallel_limit: 5
  timeout: 30s
  debug: false
database:
  path: /tmp/gibson/gibson.db
  max_connections: 10
  timeout: 5s
  wal_mode: true
  auto_vacuum: true
security:
  encryption_algorithm: AES-256-GCM
  key_derivation: argon2id
  ssl_validation: true
  audit_logging: true
llm:
  default_provider: ""
logging:
  level: info
  format: json
tracing:
  enabled: false
  endpoint: ""
metrics:
  enabled: false
  port: 9090
`
				require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))
			},
			validate: func(t *testing.T, providers []LLMProviderStatus) {
				// Should return at least one entry indicating no providers
				assert.NotEmpty(t, providers)
				assert.Equal(t, "none", providers[0].Name)
				assert.False(t, providers[0].Configured)
			},
		},
		{
			name: "config with default provider",
			setupFunc: func(t *testing.T, homeDir string) {
				configPath := config.DefaultConfigPath(homeDir)
				require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
				configContent := `
core:
  home_dir: /tmp/gibson
  data_dir: /tmp/gibson/data
  cache_dir: /tmp/gibson/cache
  parallel_limit: 5
  timeout: 30s
  debug: false
database:
  path: /tmp/gibson/gibson.db
  max_connections: 10
  timeout: 5s
  wal_mode: true
  auto_vacuum: true
security:
  encryption_algorithm: AES-256-GCM
  key_derivation: argon2id
  ssl_validation: true
  audit_logging: true
llm:
  default_provider: "anthropic"
logging:
  level: info
  format: json
tracing:
  enabled: false
  endpoint: ""
metrics:
  enabled: false
  port: 9090
`
				require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))
			},
			validate: func(t *testing.T, providers []LLMProviderStatus) {
				assert.Len(t, providers, 1)
				assert.Equal(t, "anthropic", providers[0].Name)
				assert.False(t, providers[0].Configured) // Not registered
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.setupFunc != nil {
				tt.setupFunc(t, tmpDir)
			}

			ctx := context.Background()
			providers := checkLLMProviders(ctx, tmpDir)

			if tt.validate != nil {
				tt.validate(t, providers)
			}
		})
	}
}
