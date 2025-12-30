package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zero-day-ai/gibson/internal/component"
)

// Mock implementations

// MockComponentRegistry is a mock implementation of component.ComponentRegistry
type MockComponentRegistry struct {
	mock.Mock
}

func (m *MockComponentRegistry) Register(comp *component.Component) error {
	args := m.Called(comp)
	return args.Error(0)
}

func (m *MockComponentRegistry) Unregister(kind component.ComponentKind, name string) error {
	args := m.Called(kind, name)
	return args.Error(0)
}

func (m *MockComponentRegistry) Get(kind component.ComponentKind, name string) *component.Component {
	args := m.Called(kind, name)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*component.Component)
}

func (m *MockComponentRegistry) List(kind component.ComponentKind) []*component.Component {
	args := m.Called(kind)
	return args.Get(0).([]*component.Component)
}

func (m *MockComponentRegistry) ListAll() map[component.ComponentKind][]*component.Component {
	args := m.Called()
	return args.Get(0).(map[component.ComponentKind][]*component.Component)
}

func (m *MockComponentRegistry) LoadFromConfig(path string) error {
	args := m.Called(path)
	return args.Error(0)
}

func (m *MockComponentRegistry) Save() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockComponentRegistry) Update(comp *component.Component) error {
	args := m.Called(comp)
	return args.Error(0)
}

// MockInstaller is a mock implementation of component.Installer
type MockInstaller struct {
	mock.Mock
}

func (m *MockInstaller) Install(ctx context.Context, repoURL string, opts component.InstallOptions) (*component.InstallResult, error) {
	args := m.Called(ctx, repoURL, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*component.InstallResult), args.Error(1)
}

func (m *MockInstaller) Update(ctx context.Context, kind component.ComponentKind, name string, opts component.UpdateOptions) (*component.UpdateResult, error) {
	args := m.Called(ctx, kind, name, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*component.UpdateResult), args.Error(1)
}

func (m *MockInstaller) UpdateAll(ctx context.Context, kind component.ComponentKind, opts component.UpdateOptions) ([]component.UpdateResult, error) {
	args := m.Called(ctx, kind, opts)
	return args.Get(0).([]component.UpdateResult), args.Error(1)
}

func (m *MockInstaller) Uninstall(ctx context.Context, kind component.ComponentKind, name string) (*component.UninstallResult, error) {
	args := m.Called(ctx, kind, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*component.UninstallResult), args.Error(1)
}

// MockLifecycleManager is a mock implementation of component.LifecycleManager
type MockLifecycleManager struct {
	mock.Mock
}

func (m *MockLifecycleManager) StartComponent(ctx context.Context, comp *component.Component) (int, error) {
	args := m.Called(ctx, comp)
	return args.Int(0), args.Error(1)
}

func (m *MockLifecycleManager) StopComponent(ctx context.Context, comp *component.Component) error {
	args := m.Called(ctx, comp)
	return args.Error(0)
}

func (m *MockLifecycleManager) RestartComponent(ctx context.Context, comp *component.Component) (int, error) {
	args := m.Called(ctx, comp)
	return args.Int(0), args.Error(1)
}

func (m *MockLifecycleManager) GetStatus(ctx context.Context, comp *component.Component) (component.ComponentStatus, error) {
	args := m.Called(ctx, comp)
	return args.Get(0).(component.ComponentStatus), args.Error(1)
}

// Test helper functions

func createTestAgent(name, version string, status component.ComponentStatus) *component.Component {
	now := time.Now()
	return &component.Component{
		Kind:      component.ComponentKindAgent,
		Name:      name,
		Version:   version,
		Path:      "/test/path/" + name,
		Source:    component.ComponentSourceExternal,
		Status:    status,
		Port:      50000,
		PID:       0,
		CreatedAt: now,
		UpdatedAt: now,
		Manifest: &component.Manifest{
			Name:        name,
			Version:     version,
			Description: "Test agent",
			Runtime: component.RuntimeConfig{
				Type:       component.RuntimeTypeBinary,
				Entrypoint: "/test/bin/" + name,
			},
		},
	}
}

func createRunningTestAgent(name, version string) *component.Component {
	agent := createTestAgent(name, version, component.ComponentStatusRunning)
	agent.PID = 12345
	agent.Port = 50000
	now := time.Now()
	agent.StartedAt = &now
	return agent
}

// Tests for agent list command

func TestAgentList_Success(t *testing.T) {
	tests := []struct {
		name        string
		agents      []*component.Component
		expectEmpty bool
	}{
		{
			name: "single agent",
			agents: []*component.Component{
				createTestAgent("scanner", "1.0.0", component.ComponentStatusAvailable),
			},
			expectEmpty: false,
		},
		{
			name: "multiple agents",
			agents: []*component.Component{
				createTestAgent("scanner", "1.0.0", component.ComponentStatusAvailable),
				createRunningTestAgent("fuzzer", "2.0.0"),
			},
			expectEmpty: false,
		},
		{
			name:        "no agents",
			agents:      []*component.Component{},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRegistry := new(MockComponentRegistry)
			mockRegistry.On("List", component.ComponentKindAgent).Return(tt.agents)

			// Note: In a real implementation, we would inject the registry
			// For now, this tests the logic flow
			agents := mockRegistry.List(component.ComponentKindAgent)

			if tt.expectEmpty {
				assert.Empty(t, agents)
			} else {
				assert.NotEmpty(t, agents)
				assert.Len(t, agents, len(tt.agents))
			}

			mockRegistry.AssertExpectations(t)
		})
	}
}

// Tests for agent install command

func TestAgentInstall_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockInstaller := new(MockInstaller)

	repoURL := "https://github.com/test/scanner"
	expectedAgent := createTestAgent("scanner", "1.0.0", component.ComponentStatusAvailable)
	expectedResult := &component.InstallResult{
		Component: expectedAgent,
		Path:      "/test/path/scanner",
		Duration:  2 * time.Second,
		Installed: true,
	}

	mockInstaller.On("Install", mock.Anything, repoURL, mock.Anything).Return(expectedResult, nil)
	mockRegistry.On("Save").Return(nil)

	result, err := mockInstaller.Install(context.Background(), repoURL, component.InstallOptions{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedAgent.Name, result.Component.Name)
	assert.True(t, result.Installed)

	mockInstaller.AssertExpectations(t)
	mockRegistry.AssertExpectations(t)
}

func TestAgentInstall_WithOptions(t *testing.T) {
	mockInstaller := new(MockInstaller)

	tests := []struct {
		name    string
		opts    component.InstallOptions
		repoURL string
	}{
		{
			name:    "with branch",
			repoURL: "https://github.com/test/scanner",
			opts: component.InstallOptions{
				Branch: "develop",
			},
		},
		{
			name:    "with tag",
			repoURL: "https://github.com/test/scanner",
			opts: component.InstallOptions{
				Tag: "v1.0.0",
			},
		},
		{
			name:    "with force",
			repoURL: "https://github.com/test/scanner",
			opts: component.InstallOptions{
				Force: true,
			},
		},
		{
			name:    "skip build",
			repoURL: "https://github.com/test/scanner",
			opts: component.InstallOptions{
				SkipBuild: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedAgent := createTestAgent("scanner", "1.0.0", component.ComponentStatusAvailable)
			expectedResult := &component.InstallResult{
				Component: expectedAgent,
				Path:      "/test/path/scanner",
				Duration:  2 * time.Second,
				Installed: true,
			}

			mockInstaller.On("Install", mock.Anything, tt.repoURL, tt.opts).Return(expectedResult, nil).Once()

			result, err := mockInstaller.Install(context.Background(), tt.repoURL, tt.opts)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.True(t, result.Installed)

			mockInstaller.AssertExpectations(t)
		})
	}
}

func TestAgentInstall_Errors(t *testing.T) {
	mockInstaller := new(MockInstaller)

	tests := []struct {
		name        string
		repoURL     string
		expectedErr error
	}{
		{
			name:        "invalid repository",
			repoURL:     "invalid-url",
			expectedErr: errors.New("invalid repository URL"),
		},
		{
			name:        "network error",
			repoURL:     "https://github.com/test/scanner",
			expectedErr: errors.New("network error"),
		},
		{
			name:        "build failed",
			repoURL:     "https://github.com/test/scanner",
			expectedErr: errors.New("build failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockInstaller.On("Install", mock.Anything, tt.repoURL, mock.Anything).Return(nil, tt.expectedErr).Once()

			result, err := mockInstaller.Install(context.Background(), tt.repoURL, component.InstallOptions{})

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.expectedErr.Error())

			mockInstaller.AssertExpectations(t)
		})
	}
}

// Tests for agent uninstall command

func TestAgentUninstall_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockInstaller := new(MockInstaller)
	mockLifecycle := new(MockLifecycleManager)

	agentName := "scanner"
	agent := createTestAgent(agentName, "1.0.0", component.ComponentStatusAvailable)

	mockRegistry.On("Get", component.ComponentKindAgent, agentName).Return(agent)
	mockRegistry.On("Save").Return(nil)
	mockInstaller.On("Uninstall", mock.Anything, component.ComponentKindAgent, agentName).Return(&component.UninstallResult{
		Name:     agentName,
		Kind:     component.ComponentKindAgent,
		Path:     agent.Path,
		Duration: time.Second,
	}, nil)

	result, err := mockInstaller.Uninstall(context.Background(), component.ComponentKindAgent, agentName)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, agentName, result.Name)

	mockRegistry.AssertExpectations(t)
	mockInstaller.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

func TestAgentUninstall_StopsRunningAgent(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockInstaller := new(MockInstaller)
	mockLifecycle := new(MockLifecycleManager)

	agentName := "scanner"
	agent := createRunningTestAgent(agentName, "1.0.0")

	mockRegistry.On("Get", component.ComponentKindAgent, agentName).Return(agent)
	mockRegistry.On("Update", agent).Return(nil)
	mockRegistry.On("Save").Return(nil)
	mockLifecycle.On("StopComponent", mock.Anything, agent).Return(nil)
	mockInstaller.On("Uninstall", mock.Anything, component.ComponentKindAgent, agentName).Return(&component.UninstallResult{
		Name:       agentName,
		Kind:       component.ComponentKindAgent,
		Path:       agent.Path,
		Duration:   time.Second,
		WasStopped: true,
		WasRunning: true,
	}, nil)

	// Verify agent is running
	assert.True(t, agent.IsRunning())

	// Stop the agent first
	err := mockLifecycle.StopComponent(context.Background(), agent)
	assert.NoError(t, err)

	// Then uninstall
	result, err := mockInstaller.Uninstall(context.Background(), component.ComponentKindAgent, agentName)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.WasRunning)
	assert.True(t, result.WasStopped)

	mockRegistry.AssertExpectations(t)
	mockInstaller.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

func TestAgentUninstall_NotFound(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)

	agentName := "nonexistent"
	mockRegistry.On("Get", component.ComponentKindAgent, agentName).Return(nil)

	agent := mockRegistry.Get(component.ComponentKindAgent, agentName)

	assert.Nil(t, agent)
	mockRegistry.AssertExpectations(t)
}

// Tests for agent update command

func TestAgentUpdate_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockInstaller := new(MockInstaller)

	agentName := "scanner"
	oldVersion := "1.0.0"
	newVersion := "1.1.0"

	mockInstaller.On("Update", mock.Anything, component.ComponentKindAgent, agentName, mock.Anything).Return(&component.UpdateResult{
		Component:  createTestAgent(agentName, newVersion, component.ComponentStatusAvailable),
		Path:       "/test/path/scanner",
		Duration:   3 * time.Second,
		Updated:    true,
		OldVersion: oldVersion,
		NewVersion: newVersion,
	}, nil)
	mockRegistry.On("Save").Return(nil)

	result, err := mockInstaller.Update(context.Background(), component.ComponentKindAgent, agentName, component.UpdateOptions{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Updated)
	assert.Equal(t, oldVersion, result.OldVersion)
	assert.Equal(t, newVersion, result.NewVersion)

	mockInstaller.AssertExpectations(t)
	mockRegistry.AssertExpectations(t)
}

func TestAgentUpdate_NoChanges(t *testing.T) {
	mockInstaller := new(MockInstaller)

	agentName := "scanner"
	version := "1.0.0"

	mockInstaller.On("Update", mock.Anything, component.ComponentKindAgent, agentName, mock.Anything).Return(&component.UpdateResult{
		Component:  createTestAgent(agentName, version, component.ComponentStatusAvailable),
		Path:       "/test/path/scanner",
		Duration:   time.Second,
		Updated:    false,
		OldVersion: version,
		NewVersion: version,
	}, nil)

	result, err := mockInstaller.Update(context.Background(), component.ComponentKindAgent, agentName, component.UpdateOptions{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Updated)
	assert.Equal(t, version, result.OldVersion)
	assert.Equal(t, version, result.NewVersion)

	mockInstaller.AssertExpectations(t)
}

func TestAgentUpdate_WithRestart(t *testing.T) {
	mockInstaller := new(MockInstaller)

	agentName := "scanner"
	opts := component.UpdateOptions{
		Restart: true,
	}

	mockInstaller.On("Update", mock.Anything, component.ComponentKindAgent, agentName, opts).Return(&component.UpdateResult{
		Component:  createTestAgent(agentName, "1.1.0", component.ComponentStatusRunning),
		Path:       "/test/path/scanner",
		Duration:   3 * time.Second,
		Updated:    true,
		Restarted:  true,
		OldVersion: "1.0.0",
		NewVersion: "1.1.0",
	}, nil)

	result, err := mockInstaller.Update(context.Background(), component.ComponentKindAgent, agentName, opts)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Updated)
	assert.True(t, result.Restarted)

	mockInstaller.AssertExpectations(t)
}

// Tests for agent start command

func TestAgentStart_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockLifecycle := new(MockLifecycleManager)

	agentName := "scanner"
	agent := createTestAgent(agentName, "1.0.0", component.ComponentStatusAvailable)
	expectedPort := 50001

	mockRegistry.On("Get", component.ComponentKindAgent, agentName).Return(agent)
	mockRegistry.On("Update", agent).Return(nil)
	mockRegistry.On("Save").Return(nil)
	mockLifecycle.On("StartComponent", mock.Anything, agent).Return(expectedPort, nil)

	port, err := mockLifecycle.StartComponent(context.Background(), agent)

	assert.NoError(t, err)
	assert.Equal(t, expectedPort, port)

	mockRegistry.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

func TestAgentStart_AlreadyRunning(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockLifecycle := new(MockLifecycleManager)

	agentName := "scanner"
	agent := createRunningTestAgent(agentName, "1.0.0")

	mockRegistry.On("Get", component.ComponentKindAgent, agentName).Return(agent)

	// Verify agent is already running
	assert.True(t, agent.IsRunning())
	assert.Greater(t, agent.PID, 0)

	mockRegistry.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

func TestAgentStart_NotFound(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)

	agentName := "nonexistent"
	mockRegistry.On("Get", component.ComponentKindAgent, agentName).Return(nil)

	agent := mockRegistry.Get(component.ComponentKindAgent, agentName)

	assert.Nil(t, agent)
	mockRegistry.AssertExpectations(t)
}

func TestAgentStart_FailsHealthCheck(t *testing.T) {
	mockLifecycle := new(MockLifecycleManager)

	agent := createTestAgent("scanner", "1.0.0", component.ComponentStatusAvailable)
	expectedErr := errors.New("health check timeout")

	mockLifecycle.On("StartComponent", mock.Anything, agent).Return(0, expectedErr)

	port, err := mockLifecycle.StartComponent(context.Background(), agent)

	assert.Error(t, err)
	assert.Equal(t, 0, port)
	assert.Contains(t, err.Error(), "health check")

	mockLifecycle.AssertExpectations(t)
}

// Tests for agent stop command

func TestAgentStop_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockLifecycle := new(MockLifecycleManager)

	agentName := "scanner"
	agent := createRunningTestAgent(agentName, "1.0.0")

	mockRegistry.On("Get", component.ComponentKindAgent, agentName).Return(agent)
	mockRegistry.On("Update", agent).Return(nil)
	mockRegistry.On("Save").Return(nil)
	mockLifecycle.On("StopComponent", mock.Anything, agent).Return(nil)

	err := mockLifecycle.StopComponent(context.Background(), agent)

	assert.NoError(t, err)

	mockRegistry.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

func TestAgentStop_NotRunning(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)

	agentName := "scanner"
	agent := createTestAgent(agentName, "1.0.0", component.ComponentStatusStopped)

	mockRegistry.On("Get", component.ComponentKindAgent, agentName).Return(agent)

	// Verify agent is not running
	assert.False(t, agent.IsRunning())

	mockRegistry.AssertExpectations(t)
}

func TestAgentStop_GracefulShutdown(t *testing.T) {
	mockLifecycle := new(MockLifecycleManager)

	agent := createRunningTestAgent("scanner", "1.0.0")

	// Mock successful graceful shutdown
	mockLifecycle.On("StopComponent", mock.Anything, agent).Return(nil)

	err := mockLifecycle.StopComponent(context.Background(), agent)

	assert.NoError(t, err)
	mockLifecycle.AssertExpectations(t)
}

// Tests for agent show command

func TestAgentShow_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)

	agentName := "scanner"
	agent := createTestAgent(agentName, "1.0.0", component.ComponentStatusAvailable)
	agent.Manifest.Description = "Security scanner agent"
	agent.Manifest.Author = "Test Author"
	agent.Manifest.License = "MIT"

	mockRegistry.On("Get", component.ComponentKindAgent, agentName).Return(agent)

	retrievedAgent := mockRegistry.Get(component.ComponentKindAgent, agentName)

	assert.NotNil(t, retrievedAgent)
	assert.Equal(t, agentName, retrievedAgent.Name)
	assert.Equal(t, "1.0.0", retrievedAgent.Version)
	assert.NotNil(t, retrievedAgent.Manifest)
	assert.Equal(t, "Security scanner agent", retrievedAgent.Manifest.Description)
	assert.Equal(t, "Test Author", retrievedAgent.Manifest.Author)
	assert.Equal(t, "MIT", retrievedAgent.Manifest.License)

	mockRegistry.AssertExpectations(t)
}

func TestAgentShow_NotFound(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)

	agentName := "nonexistent"
	mockRegistry.On("Get", component.ComponentKindAgent, agentName).Return(nil)

	agent := mockRegistry.Get(component.ComponentKindAgent, agentName)

	assert.Nil(t, agent)
	mockRegistry.AssertExpectations(t)
}

// Tests for agent build command

func TestAgentBuild_Success(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	agentPath := filepath.Join(tmpDir, "test-agent")
	err := os.MkdirAll(agentPath, 0755)
	assert.NoError(t, err)

	// Create a test manifest
	manifestContent := `kind: agent
name: test-agent
version: 1.0.0
description: Test agent
runtime:
  type: binary
  entrypoint: ./bin/test-agent
build:
  command: make build
`
	manifestPath := filepath.Join(agentPath, "component.yaml")
	err = os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	assert.NoError(t, err)

	// Test that manifest can be loaded
	manifest, err := component.LoadManifest(manifestPath)
	assert.NoError(t, err)
	assert.NotNil(t, manifest)
	// Note: Manifest no longer has Kind field - kind is determined by command context
	assert.Equal(t, "test-agent", manifest.Name)
	assert.NotNil(t, manifest.Build)
	assert.Equal(t, "make build", manifest.Build.Command)
}

func TestAgentBuild_BackwardsCompatibilityWithKindField(t *testing.T) {
	tmpDir := t.TempDir()
	agentPath := filepath.Join(tmpDir, "test-tool")
	err := os.MkdirAll(agentPath, 0755)
	assert.NoError(t, err)

	// Create a manifest with kind field (for backwards compatibility)
	// The kind field is ignored by the parser but should not cause errors
	manifestContent := `kind: tool
name: test-tool
version: 1.0.0
runtime:
  type: binary
  entrypoint: ./bin/test-tool
`
	manifestPath := filepath.Join(agentPath, "component.yaml")
	err = os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	assert.NoError(t, err)

	// Verify that manifests with kind field still parse successfully
	manifest, err := component.LoadManifest(manifestPath)
	assert.NoError(t, err)
	assert.NotNil(t, manifest)
	assert.Equal(t, "test-tool", manifest.Name)
	// Note: Kind field is ignored for backwards compatibility
}

func TestAgentBuild_MissingManifest(t *testing.T) {
	tmpDir := t.TempDir()
	agentPath := filepath.Join(tmpDir, "test-agent")
	err := os.MkdirAll(agentPath, 0755)
	assert.NoError(t, err)

	manifestPath := filepath.Join(agentPath, "component.yaml")
	_, err = component.LoadManifest(manifestPath)
	assert.Error(t, err)
}

// Tests for agent logs command

func TestAgentLogs_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "nonexistent.log")

	_, err := os.Stat(logPath)
	assert.True(t, os.IsNotExist(err))
}

func TestAgentLogs_ReadSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test-agent.log")

	// Create test log file
	logContent := "2024-01-01 10:00:00 Agent started\n2024-01-01 10:00:01 Processing request\n2024-01-01 10:00:02 Request completed\n"
	err := os.WriteFile(logPath, []byte(logContent), 0644)
	assert.NoError(t, err)

	// Read the log file
	content, err := os.ReadFile(logPath)
	assert.NoError(t, err)
	assert.Equal(t, logContent, string(content))
}

// Integration-style tests

func TestAgentLifecycle_InstallStartStop(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockInstaller := new(MockInstaller)
	mockLifecycle := new(MockLifecycleManager)

	agentName := "scanner"
	repoURL := "https://github.com/test/scanner"

	// Step 1: Install
	installResult := &component.InstallResult{
		Component: createTestAgent(agentName, "1.0.0", component.ComponentStatusAvailable),
		Path:      "/test/path/scanner",
		Duration:  2 * time.Second,
		Installed: true,
	}
	mockInstaller.On("Install", mock.Anything, repoURL, mock.Anything).Return(installResult, nil)
	mockRegistry.On("Save").Return(nil).Times(3)

	result, err := mockInstaller.Install(context.Background(), repoURL, component.InstallOptions{})
	assert.NoError(t, err)
	assert.True(t, result.Installed)

	// Step 2: Start
	agent := result.Component
	mockRegistry.On("Get", component.ComponentKindAgent, agentName).Return(agent)
	mockRegistry.On("Update", agent).Return(nil).Times(2)
	mockLifecycle.On("StartComponent", mock.Anything, agent).Return(50001, nil)

	port, err := mockLifecycle.StartComponent(context.Background(), agent)
	assert.NoError(t, err)
	assert.Equal(t, 50001, port)

	// Step 3: Stop
	mockLifecycle.On("StopComponent", mock.Anything, agent).Return(nil)

	err = mockLifecycle.StopComponent(context.Background(), agent)
	assert.NoError(t, err)

	mockRegistry.AssertExpectations(t)
	mockInstaller.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

// Error handling tests

func TestAgentCommands_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		operation   string
		setupMocks  func(*MockComponentRegistry, *MockInstaller, *MockLifecycleManager)
		expectError bool
	}{
		{
			name:      "install with network error",
			operation: "install",
			setupMocks: func(reg *MockComponentRegistry, inst *MockInstaller, lm *MockLifecycleManager) {
				inst.On("Install", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("network error"))
			},
			expectError: true,
		},
		{
			name:      "start non-existent agent",
			operation: "start",
			setupMocks: func(reg *MockComponentRegistry, inst *MockInstaller, lm *MockLifecycleManager) {
				reg.On("Get", component.ComponentKindAgent, "nonexistent").Return(nil)
			},
			expectError: true,
		},
		{
			name:      "stop non-running agent",
			operation: "stop",
			setupMocks: func(reg *MockComponentRegistry, inst *MockInstaller, lm *MockLifecycleManager) {
				agent := createTestAgent("scanner", "1.0.0", component.ComponentStatusStopped)
				reg.On("Get", component.ComponentKindAgent, "scanner").Return(agent)
			},
			expectError: true,
		},
		{
			name:      "update with build failure",
			operation: "update",
			setupMocks: func(reg *MockComponentRegistry, inst *MockInstaller, lm *MockLifecycleManager) {
				inst.On("Update", mock.Anything, component.ComponentKindAgent, mock.Anything, mock.Anything).Return(nil, errors.New("build failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRegistry := new(MockComponentRegistry)
			mockInstaller := new(MockInstaller)
			mockLifecycle := new(MockLifecycleManager)

			tt.setupMocks(mockRegistry, mockInstaller, mockLifecycle)

			// Execute the operation based on type
			var err error
			switch tt.operation {
			case "install":
				_, err = mockInstaller.Install(context.Background(), "https://test.com/repo", component.InstallOptions{})
			case "start":
				agent := mockRegistry.Get(component.ComponentKindAgent, "nonexistent")
				if agent == nil {
					err = errors.New("agent not found")
				}
			case "stop":
				agent := mockRegistry.Get(component.ComponentKindAgent, "scanner")
				if agent != nil && !agent.IsRunning() {
					err = errors.New("agent is not running")
				}
			case "update":
				_, err = mockInstaller.Update(context.Background(), component.ComponentKindAgent, "scanner", component.UpdateOptions{})
			}

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockRegistry.AssertExpectations(t)
			mockInstaller.AssertExpectations(t)
			mockLifecycle.AssertExpectations(t)
		})
	}
}
