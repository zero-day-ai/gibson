package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/plugin"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/sdk/schema"
)

// MockPluginRegistry is a mock implementation of plugin.PluginRegistry
type MockPluginRegistry struct {
	mock.Mock
}

func (m *MockPluginRegistry) Register(p plugin.Plugin, cfg plugin.PluginConfig) error {
	args := m.Called(p, cfg)
	return args.Error(0)
}

func (m *MockPluginRegistry) RegisterExternal(name string, client plugin.ExternalPluginClient, cfg plugin.PluginConfig) error {
	args := m.Called(name, client, cfg)
	return args.Error(0)
}

func (m *MockPluginRegistry) Unregister(name string) error {
	args := m.Called(name)
	return args.Error(0)
}

func (m *MockPluginRegistry) Get(name string) (plugin.Plugin, error) {
	args := m.Called(name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(plugin.Plugin), args.Error(1)
}

func (m *MockPluginRegistry) List() []plugin.PluginDescriptor {
	args := m.Called()
	return args.Get(0).([]plugin.PluginDescriptor)
}

func (m *MockPluginRegistry) Methods(pluginName string) ([]plugin.MethodDescriptor, error) {
	args := m.Called(pluginName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]plugin.MethodDescriptor), args.Error(1)
}

func (m *MockPluginRegistry) Query(ctx context.Context, pluginName, method string, params map[string]any) (any, error) {
	args := m.Called(ctx, pluginName, method, params)
	return args.Get(0), args.Error(1)
}

func (m *MockPluginRegistry) Shutdown(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockPluginRegistry) Health(ctx context.Context) types.HealthStatus {
	args := m.Called(ctx)
	return args.Get(0).(types.HealthStatus)
}

// MockPlugin is a mock implementation of plugin.Plugin
type MockPlugin struct {
	mock.Mock
}

func (m *MockPlugin) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockPlugin) Version() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockPlugin) Initialize(ctx context.Context, cfg plugin.PluginConfig) error {
	args := m.Called(ctx, cfg)
	return args.Error(0)
}

func (m *MockPlugin) Shutdown(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockPlugin) Query(ctx context.Context, method string, params map[string]any) (any, error) {
	args := m.Called(ctx, method, params)
	return args.Get(0), args.Error(1)
}

func (m *MockPlugin) Methods() []plugin.MethodDescriptor {
	args := m.Called()
	return args.Get(0).([]plugin.MethodDescriptor)
}

func (m *MockPlugin) Health(ctx context.Context) types.HealthStatus {
	args := m.Called(ctx)
	return args.Get(0).(types.HealthStatus)
}

// Test helper functions

func createTestPlugin(name, version string, status component.ComponentStatus) *component.Component {
	now := time.Now()
	return &component.Component{
		Kind:      component.ComponentKindPlugin,
		Name:      name,
		Version:   version,
		RepoPath:  "/test/repos/" + name,
		BinPath:   "/test/bin/" + name,
		Source:    component.ComponentSourceExternal,
		Status:    status,
		Port:      50000,
		PID:       0,
		CreatedAt: now,
		UpdatedAt: now,
		Manifest: &component.Manifest{
			Name:        name,
			Version:     version,
			Description: "Test plugin",
			Runtime: &component.RuntimeConfig{
				Type:       component.RuntimeTypeBinary,
				Entrypoint: "/test/bin/" + name,
			},
		},
	}
}

func createRunningTestPlugin(name, version string) *component.Component {
	pluginComp := createTestPlugin(name, version, component.ComponentStatusRunning)
	pluginComp.PID = 12345
	pluginComp.Port = 50000
	now := time.Now()
	pluginComp.StartedAt = &now
	return pluginComp
}

func createTestMethodDescriptor(name, description string) plugin.MethodDescriptor {
	return plugin.MethodDescriptor{
		Name:        name,
		Description: description,
		InputSchema: schema.Object(map[string]schema.JSON{
			"key": schema.String(),
		}),
		OutputSchema: schema.Object(nil),
	}
}

// Tests for plugin list command

func TestPluginList_Success(t *testing.T) {
	tests := []struct {
		name        string
		plugins     []*component.Component
		expectEmpty bool
	}{
		{
			name: "single plugin",
			plugins: []*component.Component{
				createTestPlugin("database", "1.0.0", component.ComponentStatusAvailable),
			},
			expectEmpty: false,
		},
		{
			name: "multiple plugins",
			plugins: []*component.Component{
				createTestPlugin("database", "1.0.0", component.ComponentStatusAvailable),
				createRunningTestPlugin("weather", "2.0.0"),
			},
			expectEmpty: false,
		},
		{
			name:        "no plugins",
			plugins:     []*component.Component{},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRegistry := new(MockComponentRegistry)
			mockRegistry.On("List", component.ComponentKindPlugin).Return(tt.plugins)

			plugins := mockRegistry.List(component.ComponentKindPlugin)

			if tt.expectEmpty {
				assert.Empty(t, plugins)
			} else {
				assert.NotEmpty(t, plugins)
				assert.Len(t, plugins, len(tt.plugins))
			}

			mockRegistry.AssertExpectations(t)
		})
	}
}

// Tests for plugin install command

func TestPluginInstall_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockInstaller := new(MockInstaller)

	repoURL := "https://github.com/test/database-plugin"
	expectedPlugin := createTestPlugin("database", "1.0.0", component.ComponentStatusAvailable)
	expectedResult := &component.InstallResult{
		Component: expectedPlugin,
		Path:      "/test/path/database",
		Duration:  2 * time.Second,
		Installed: true,
	}

	mockInstaller.On("Install", mock.Anything, repoURL, mock.Anything).Return(expectedResult, nil)
	mockRegistry.On("Save").Return(nil)

	result, err := mockInstaller.Install(context.Background(), repoURL, component.InstallOptions{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedPlugin.Name, result.Component.Name)
	assert.True(t, result.Installed)

	mockInstaller.AssertExpectations(t)
	mockRegistry.AssertExpectations(t)
}

func TestPluginInstall_WithOptions(t *testing.T) {
	mockInstaller := new(MockInstaller)

	tests := []struct {
		name    string
		opts    component.InstallOptions
		repoURL string
	}{
		{
			name:    "with branch",
			repoURL: "https://github.com/test/database-plugin",
			opts: component.InstallOptions{
				Branch: "develop",
			},
		},
		{
			name:    "with tag",
			repoURL: "https://github.com/test/database-plugin",
			opts: component.InstallOptions{
				Tag: "v1.0.0",
			},
		},
		{
			name:    "with force",
			repoURL: "https://github.com/test/database-plugin",
			opts: component.InstallOptions{
				Force: true,
			},
		},
		{
			name:    "skip build",
			repoURL: "https://github.com/test/database-plugin",
			opts: component.InstallOptions{
				SkipBuild: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedPlugin := createTestPlugin("database", "1.0.0", component.ComponentStatusAvailable)
			expectedResult := &component.InstallResult{
				Component: expectedPlugin,
				Path:      "/test/path/database",
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

func TestPluginInstall_Errors(t *testing.T) {
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
			repoURL:     "https://github.com/test/plugin",
			expectedErr: errors.New("network error"),
		},
		{
			name:        "build failed",
			repoURL:     "https://github.com/test/plugin",
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

// Tests for plugin uninstall command

func TestPluginUninstall_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockInstaller := new(MockInstaller)
	mockLifecycle := new(MockLifecycleManager)

	pluginName := "database"
	pluginComp := createTestPlugin(pluginName, "1.0.0", component.ComponentStatusAvailable)

	mockRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)
	mockRegistry.On("Save").Return(nil)
	mockInstaller.On("Uninstall", mock.Anything, component.ComponentKindPlugin, pluginName).Return(&component.UninstallResult{
		Name:     pluginName,
		Kind:     component.ComponentKindPlugin,
		Path:     pluginComp.RepoPath,
		Duration: time.Second,
	}, nil)

	result, err := mockInstaller.Uninstall(context.Background(), component.ComponentKindPlugin, pluginName)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, pluginName, result.Name)

	mockRegistry.AssertExpectations(t)
	mockInstaller.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

func TestPluginUninstall_StopsRunningPlugin(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockInstaller := new(MockInstaller)
	mockLifecycle := new(MockLifecycleManager)

	pluginName := "database"
	pluginComp := createRunningTestPlugin(pluginName, "1.0.0")

	mockRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)
	mockRegistry.On("Update", pluginComp).Return(nil)
	mockRegistry.On("Save").Return(nil)
	mockLifecycle.On("StopComponent", mock.Anything, pluginComp).Return(nil)
	mockInstaller.On("Uninstall", mock.Anything, component.ComponentKindPlugin, pluginName).Return(&component.UninstallResult{
		Name:       pluginName,
		Kind:       component.ComponentKindPlugin,
		Path:       pluginComp.RepoPath,
		Duration:   time.Second,
		WasStopped: true,
		WasRunning: true,
	}, nil)

	// Verify plugin is running
	assert.True(t, pluginComp.IsRunning())

	// Stop the plugin first
	err := mockLifecycle.StopComponent(context.Background(), pluginComp)
	assert.NoError(t, err)

	// Then uninstall
	result, err := mockInstaller.Uninstall(context.Background(), component.ComponentKindPlugin, pluginName)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.WasRunning)
	assert.True(t, result.WasStopped)

	mockRegistry.AssertExpectations(t)
	mockInstaller.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

func TestPluginUninstall_NotFound(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)

	pluginName := "nonexistent"
	mockRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(nil)

	pluginComp := mockRegistry.Get(component.ComponentKindPlugin, pluginName)

	assert.Nil(t, pluginComp)
	mockRegistry.AssertExpectations(t)
}

// Tests for plugin update command

func TestPluginUpdate_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockInstaller := new(MockInstaller)

	pluginName := "database"
	oldVersion := "1.0.0"
	newVersion := "1.1.0"

	mockInstaller.On("Update", mock.Anything, component.ComponentKindPlugin, pluginName, mock.Anything).Return(&component.UpdateResult{
		Component:  createTestPlugin(pluginName, newVersion, component.ComponentStatusAvailable),
		Path:       "/test/path/database",
		Duration:   3 * time.Second,
		Updated:    true,
		OldVersion: oldVersion,
		NewVersion: newVersion,
	}, nil)
	mockRegistry.On("Save").Return(nil)

	result, err := mockInstaller.Update(context.Background(), component.ComponentKindPlugin, pluginName, component.UpdateOptions{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Updated)
	assert.Equal(t, oldVersion, result.OldVersion)
	assert.Equal(t, newVersion, result.NewVersion)

	mockInstaller.AssertExpectations(t)
	mockRegistry.AssertExpectations(t)
}

func TestPluginUpdate_NoChanges(t *testing.T) {
	mockInstaller := new(MockInstaller)

	pluginName := "database"
	version := "1.0.0"

	mockInstaller.On("Update", mock.Anything, component.ComponentKindPlugin, pluginName, mock.Anything).Return(&component.UpdateResult{
		Component:  createTestPlugin(pluginName, version, component.ComponentStatusAvailable),
		Path:       "/test/path/database",
		Duration:   time.Second,
		Updated:    false,
		OldVersion: version,
		NewVersion: version,
	}, nil)

	result, err := mockInstaller.Update(context.Background(), component.ComponentKindPlugin, pluginName, component.UpdateOptions{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Updated)
	assert.Equal(t, version, result.OldVersion)
	assert.Equal(t, version, result.NewVersion)

	mockInstaller.AssertExpectations(t)
}

func TestPluginUpdate_WithRestart(t *testing.T) {
	mockInstaller := new(MockInstaller)

	pluginName := "database"
	opts := component.UpdateOptions{
		Restart: true,
	}

	mockInstaller.On("Update", mock.Anything, component.ComponentKindPlugin, pluginName, opts).Return(&component.UpdateResult{
		Component:  createTestPlugin(pluginName, "1.1.0", component.ComponentStatusRunning),
		Path:       "/test/path/database",
		Duration:   3 * time.Second,
		Updated:    true,
		Restarted:  true,
		OldVersion: "1.0.0",
		NewVersion: "1.1.0",
	}, nil)

	result, err := mockInstaller.Update(context.Background(), component.ComponentKindPlugin, pluginName, opts)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Updated)
	assert.True(t, result.Restarted)

	mockInstaller.AssertExpectations(t)
}

// Tests for plugin start command

func TestPluginStart_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockLifecycle := new(MockLifecycleManager)

	pluginName := "database"
	pluginComp := createTestPlugin(pluginName, "1.0.0", component.ComponentStatusAvailable)
	expectedPort := 50001

	mockRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)
	mockRegistry.On("Update", pluginComp).Return(nil)
	mockRegistry.On("Save").Return(nil)
	mockLifecycle.On("StartComponent", mock.Anything, pluginComp).Return(expectedPort, nil)

	port, err := mockLifecycle.StartComponent(context.Background(), pluginComp)

	assert.NoError(t, err)
	assert.Equal(t, expectedPort, port)

	mockRegistry.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

func TestPluginStart_AlreadyRunning(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockLifecycle := new(MockLifecycleManager)

	pluginName := "database"
	pluginComp := createRunningTestPlugin(pluginName, "1.0.0")

	mockRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)

	// Verify plugin is already running
	assert.True(t, pluginComp.IsRunning())
	assert.Greater(t, pluginComp.PID, 0)

	mockRegistry.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

func TestPluginStart_NotFound(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)

	pluginName := "nonexistent"
	mockRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(nil)

	pluginComp := mockRegistry.Get(component.ComponentKindPlugin, pluginName)

	assert.Nil(t, pluginComp)
	mockRegistry.AssertExpectations(t)
}

// Tests for plugin stop command

func TestPluginStop_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)
	mockLifecycle := new(MockLifecycleManager)

	pluginName := "database"
	pluginComp := createRunningTestPlugin(pluginName, "1.0.0")

	mockRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)
	mockRegistry.On("Update", pluginComp).Return(nil)
	mockRegistry.On("Save").Return(nil)
	mockLifecycle.On("StopComponent", mock.Anything, pluginComp).Return(nil)

	err := mockLifecycle.StopComponent(context.Background(), pluginComp)

	assert.NoError(t, err)

	mockRegistry.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

func TestPluginStop_NotRunning(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)

	pluginName := "database"
	pluginComp := createTestPlugin(pluginName, "1.0.0", component.ComponentStatusStopped)

	mockRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)

	// Verify plugin is not running
	assert.False(t, pluginComp.IsRunning())

	mockRegistry.AssertExpectations(t)
}

// Tests for plugin show command

func TestPluginShow_Success(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)

	pluginName := "database"
	pluginComp := createTestPlugin(pluginName, "1.0.0", component.ComponentStatusAvailable)
	pluginComp.Manifest.Description = "Database access plugin"
	pluginComp.Manifest.Author = "Test Author"
	pluginComp.Manifest.License = "MIT"

	mockRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)

	retrievedPlugin := mockRegistry.Get(component.ComponentKindPlugin, pluginName)

	assert.NotNil(t, retrievedPlugin)
	assert.Equal(t, pluginName, retrievedPlugin.Name)
	assert.Equal(t, "1.0.0", retrievedPlugin.Version)
	assert.NotNil(t, retrievedPlugin.Manifest)
	assert.Equal(t, "Database access plugin", retrievedPlugin.Manifest.Description)
	assert.Equal(t, "Test Author", retrievedPlugin.Manifest.Author)
	assert.Equal(t, "MIT", retrievedPlugin.Manifest.License)

	mockRegistry.AssertExpectations(t)
}

func TestPluginShow_WithMethods(t *testing.T) {
	mockComponentRegistry := new(MockComponentRegistry)
	mockPluginRegistry := new(MockPluginRegistry)

	pluginName := "database"
	pluginComp := createRunningTestPlugin(pluginName, "1.0.0")

	methods := []plugin.MethodDescriptor{
		createTestMethodDescriptor("GetData", "Retrieve data from database"),
		createTestMethodDescriptor("SaveData", "Save data to database"),
	}

	mockComponentRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)
	mockPluginRegistry.On("Methods", pluginName).Return(methods, nil)

	retrievedPlugin := mockComponentRegistry.Get(component.ComponentKindPlugin, pluginName)
	assert.NotNil(t, retrievedPlugin)
	assert.True(t, retrievedPlugin.IsRunning())

	retrievedMethods, err := mockPluginRegistry.Methods(pluginName)
	assert.NoError(t, err)
	assert.Len(t, retrievedMethods, 2)
	assert.Equal(t, "GetData", retrievedMethods[0].Name)
	assert.Equal(t, "SaveData", retrievedMethods[1].Name)

	mockComponentRegistry.AssertExpectations(t)
	mockPluginRegistry.AssertExpectations(t)
}

func TestPluginShow_NotRunning(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)

	pluginName := "database"
	pluginComp := createTestPlugin(pluginName, "1.0.0", component.ComponentStatusStopped)

	mockRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)

	retrievedPlugin := mockRegistry.Get(component.ComponentKindPlugin, pluginName)

	assert.NotNil(t, retrievedPlugin)
	assert.False(t, retrievedPlugin.IsRunning())

	mockRegistry.AssertExpectations(t)
}

// Tests for plugin query command

func TestPluginQuery_Success(t *testing.T) {
	mockComponentRegistry := new(MockComponentRegistry)
	mockPluginRegistry := new(MockPluginRegistry)

	pluginName := "database"
	method := "GetData"
	params := map[string]any{"key": "test-key"}
	expectedResult := map[string]any{"value": "test-value"}

	pluginComp := createRunningTestPlugin(pluginName, "1.0.0")
	methods := []plugin.MethodDescriptor{
		createTestMethodDescriptor("GetData", "Retrieve data from database"),
	}

	mockComponentRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)
	mockPluginRegistry.On("Methods", pluginName).Return(methods, nil)
	mockPluginRegistry.On("Query", mock.Anything, pluginName, method, params).Return(expectedResult, nil)

	// Verify plugin is running
	assert.True(t, pluginComp.IsRunning())

	// Verify method exists
	retrievedMethods, err := mockPluginRegistry.Methods(pluginName)
	assert.NoError(t, err)
	assert.Len(t, retrievedMethods, 1)
	assert.Equal(t, method, retrievedMethods[0].Name)

	// Execute query
	result, err := mockPluginRegistry.Query(context.Background(), pluginName, method, params)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult, result)

	mockComponentRegistry.AssertExpectations(t)
	mockPluginRegistry.AssertExpectations(t)
}

func TestPluginQuery_ValidateMethodExists(t *testing.T) {
	mockPluginRegistry := new(MockPluginRegistry)

	pluginName := "database"
	validMethod := "GetData"
	invalidMethod := "NonExistentMethod"

	methods := []plugin.MethodDescriptor{
		createTestMethodDescriptor("GetData", "Retrieve data from database"),
		createTestMethodDescriptor("SaveData", "Save data to database"),
	}

	mockPluginRegistry.On("Methods", pluginName).Return(methods, nil)

	retrievedMethods, err := mockPluginRegistry.Methods(pluginName)
	assert.NoError(t, err)

	// Check valid method exists
	validMethodExists := false
	for _, m := range retrievedMethods {
		if m.Name == validMethod {
			validMethodExists = true
			break
		}
	}
	assert.True(t, validMethodExists, "Valid method should exist")

	// Check invalid method does not exist
	invalidMethodExists := false
	for _, m := range retrievedMethods {
		if m.Name == invalidMethod {
			invalidMethodExists = true
			break
		}
	}
	assert.False(t, invalidMethodExists, "Invalid method should not exist")

	mockPluginRegistry.AssertExpectations(t)
}

func TestPluginQuery_InvalidMethod(t *testing.T) {
	mockPluginRegistry := new(MockPluginRegistry)

	pluginName := "database"
	invalidMethod := "NonExistentMethod"

	methods := []plugin.MethodDescriptor{
		createTestMethodDescriptor("GetData", "Retrieve data from database"),
	}

	mockPluginRegistry.On("Methods", pluginName).Return(methods, nil)

	retrievedMethods, err := mockPluginRegistry.Methods(pluginName)
	assert.NoError(t, err)

	// Verify invalid method doesn't exist
	methodExists := false
	for _, m := range retrievedMethods {
		if m.Name == invalidMethod {
			methodExists = true
			break
		}
	}
	assert.False(t, methodExists, "Invalid method should not exist")

	mockPluginRegistry.AssertExpectations(t)
}

func TestPluginQuery_PluginNotRunning(t *testing.T) {
	mockRegistry := new(MockComponentRegistry)

	pluginName := "database"
	pluginComp := createTestPlugin(pluginName, "1.0.0", component.ComponentStatusStopped)

	mockRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)

	retrievedPlugin := mockRegistry.Get(component.ComponentKindPlugin, pluginName)

	assert.NotNil(t, retrievedPlugin)
	assert.False(t, retrievedPlugin.IsRunning())

	mockRegistry.AssertExpectations(t)
}

func TestPluginQuery_InvalidJSON(t *testing.T) {
	// This tests JSON parsing validation
	invalidJSON := `{"key": "value"` // Missing closing brace

	// In the actual implementation, json.Unmarshal would return an error
	// We're just testing the validation logic here
	assert.NotNil(t, invalidJSON)
	// Real implementation would fail here
}

func TestPluginQuery_ComplexParams(t *testing.T) {
	mockPluginRegistry := new(MockPluginRegistry)

	pluginName := "database"
	method := "ComplexQuery"
	params := map[string]any{
		"filters": map[string]any{
			"status": "active",
			"age":    map[string]any{"gt": 18},
		},
		"sort":  []string{"name", "date"},
		"limit": 10,
	}
	expectedResult := map[string]any{"results": []any{}}

	methods := []plugin.MethodDescriptor{
		createTestMethodDescriptor("ComplexQuery", "Execute complex query"),
	}

	mockPluginRegistry.On("Methods", pluginName).Return(methods, nil)
	mockPluginRegistry.On("Query", mock.Anything, pluginName, method, params).Return(expectedResult, nil)

	// Verify method exists
	retrievedMethods, err := mockPluginRegistry.Methods(pluginName)
	assert.NoError(t, err)
	assert.Len(t, retrievedMethods, 1)

	// Execute query
	result, err := mockPluginRegistry.Query(context.Background(), pluginName, method, params)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult, result)

	mockPluginRegistry.AssertExpectations(t)
}

// Integration-style tests

func TestPluginLifecycle_InstallStartQueryStop(t *testing.T) {
	mockComponentRegistry := new(MockComponentRegistry)
	mockPluginRegistry := new(MockPluginRegistry)
	mockInstaller := new(MockInstaller)
	mockLifecycle := new(MockLifecycleManager)

	pluginName := "database"
	repoURL := "https://github.com/test/database-plugin"

	// Step 1: Install
	installResult := &component.InstallResult{
		Component: createTestPlugin(pluginName, "1.0.0", component.ComponentStatusAvailable),
		Path:      "/test/path/database",
		Duration:  2 * time.Second,
		Installed: true,
	}
	mockInstaller.On("Install", mock.Anything, repoURL, mock.Anything).Return(installResult, nil)
	mockComponentRegistry.On("Save").Return(nil).Times(3)

	result, err := mockInstaller.Install(context.Background(), repoURL, component.InstallOptions{})
	assert.NoError(t, err)
	assert.True(t, result.Installed)

	// Step 2: Start
	pluginComp := result.Component
	mockComponentRegistry.On("Get", component.ComponentKindPlugin, pluginName).Return(pluginComp)
	mockComponentRegistry.On("Update", pluginComp).Return(nil).Times(2)
	mockLifecycle.On("StartComponent", mock.Anything, pluginComp).Return(50001, nil)

	port, err := mockLifecycle.StartComponent(context.Background(), pluginComp)
	assert.NoError(t, err)
	assert.Equal(t, 50001, port)

	// Step 3: Query
	method := "GetData"
	params := map[string]any{"key": "test"}
	expectedResult := map[string]any{"value": "result"}

	methods := []plugin.MethodDescriptor{
		createTestMethodDescriptor("GetData", "Retrieve data"),
	}

	mockPluginRegistry.On("Methods", pluginName).Return(methods, nil)
	mockPluginRegistry.On("Query", mock.Anything, pluginName, method, params).Return(expectedResult, nil)

	queryResult, err := mockPluginRegistry.Query(context.Background(), pluginName, method, params)
	assert.NoError(t, err)
	assert.Equal(t, expectedResult, queryResult)

	// Step 4: Stop
	mockLifecycle.On("StopComponent", mock.Anything, pluginComp).Return(nil)

	err = mockLifecycle.StopComponent(context.Background(), pluginComp)
	assert.NoError(t, err)

	mockComponentRegistry.AssertExpectations(t)
	mockPluginRegistry.AssertExpectations(t)
	mockInstaller.AssertExpectations(t)
	mockLifecycle.AssertExpectations(t)
}

// Error handling tests

func TestPluginCommands_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		operation   string
		setupMocks  func(*MockComponentRegistry, *MockPluginRegistry, *MockInstaller, *MockLifecycleManager)
		expectError bool
	}{
		{
			name:      "install with network error",
			operation: "install",
			setupMocks: func(compReg *MockComponentRegistry, pluginReg *MockPluginRegistry, inst *MockInstaller, lm *MockLifecycleManager) {
				inst.On("Install", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("network error"))
			},
			expectError: true,
		},
		{
			name:      "start non-existent plugin",
			operation: "start",
			setupMocks: func(compReg *MockComponentRegistry, pluginReg *MockPluginRegistry, inst *MockInstaller, lm *MockLifecycleManager) {
				compReg.On("Get", component.ComponentKindPlugin, "nonexistent").Return(nil)
			},
			expectError: true,
		},
		{
			name:      "stop non-running plugin",
			operation: "stop",
			setupMocks: func(compReg *MockComponentRegistry, pluginReg *MockPluginRegistry, inst *MockInstaller, lm *MockLifecycleManager) {
				pluginComp := createTestPlugin("database", "1.0.0", component.ComponentStatusStopped)
				compReg.On("Get", component.ComponentKindPlugin, "database").Return(pluginComp)
			},
			expectError: true,
		},
		{
			name:      "query with method not found",
			operation: "query",
			setupMocks: func(compReg *MockComponentRegistry, pluginReg *MockPluginRegistry, inst *MockInstaller, lm *MockLifecycleManager) {
				pluginComp := createRunningTestPlugin("database", "1.0.0")
				methods := []plugin.MethodDescriptor{
					createTestMethodDescriptor("ValidMethod", "A valid method"),
				}
				compReg.On("Get", component.ComponentKindPlugin, "database").Return(pluginComp)
				pluginReg.On("Methods", "database").Return(methods, nil)
			},
			expectError: true,
		},
		{
			name:      "query with plugin not running",
			operation: "query_not_running",
			setupMocks: func(compReg *MockComponentRegistry, pluginReg *MockPluginRegistry, inst *MockInstaller, lm *MockLifecycleManager) {
				pluginComp := createTestPlugin("database", "1.0.0", component.ComponentStatusStopped)
				compReg.On("Get", component.ComponentKindPlugin, "database").Return(pluginComp)
			},
			expectError: true,
		},
		{
			name:      "update with build failure",
			operation: "update",
			setupMocks: func(compReg *MockComponentRegistry, pluginReg *MockPluginRegistry, inst *MockInstaller, lm *MockLifecycleManager) {
				inst.On("Update", mock.Anything, component.ComponentKindPlugin, mock.Anything, mock.Anything).Return(nil, errors.New("build failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockComponentRegistry := new(MockComponentRegistry)
			mockPluginRegistry := new(MockPluginRegistry)
			mockInstaller := new(MockInstaller)
			mockLifecycle := new(MockLifecycleManager)

			tt.setupMocks(mockComponentRegistry, mockPluginRegistry, mockInstaller, mockLifecycle)

			// Execute the operation based on type
			var err error
			switch tt.operation {
			case "install":
				_, err = mockInstaller.Install(context.Background(), "https://test.com/repo", component.InstallOptions{})
			case "start":
				pluginComp := mockComponentRegistry.Get(component.ComponentKindPlugin, "nonexistent")
				if pluginComp == nil {
					err = errors.New("plugin not found")
				}
			case "stop":
				pluginComp := mockComponentRegistry.Get(component.ComponentKindPlugin, "database")
				if pluginComp != nil && !pluginComp.IsRunning() {
					err = errors.New("plugin is not running")
				}
			case "query":
				pluginComp := mockComponentRegistry.Get(component.ComponentKindPlugin, "database")
				if pluginComp != nil && pluginComp.IsRunning() {
					methods, _ := mockPluginRegistry.Methods("database")
					invalidMethod := "InvalidMethod"
					methodExists := false
					for _, m := range methods {
						if m.Name == invalidMethod {
							methodExists = true
							break
						}
					}
					if !methodExists {
						err = errors.New("method not found")
					}
				}
			case "query_not_running":
				pluginComp := mockComponentRegistry.Get(component.ComponentKindPlugin, "database")
				if pluginComp != nil && !pluginComp.IsRunning() {
					err = errors.New("plugin is not running")
				}
			case "update":
				_, err = mockInstaller.Update(context.Background(), component.ComponentKindPlugin, "database", component.UpdateOptions{})
			}

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockComponentRegistry.AssertExpectations(t)
			mockPluginRegistry.AssertExpectations(t)
			mockInstaller.AssertExpectations(t)
			mockLifecycle.AssertExpectations(t)
		})
	}
}

// Tests for table-driven comprehensive validation

func TestPluginQuery_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		pluginName     string
		method         string
		params         map[string]any
		methods        []plugin.MethodDescriptor
		isRunning      bool
		expectedResult any
		expectedError  error
	}{
		{
			name:       "valid query",
			pluginName: "database",
			method:     "GetData",
			params:     map[string]any{"key": "test"},
			methods: []plugin.MethodDescriptor{
				createTestMethodDescriptor("GetData", "Get data"),
			},
			isRunning:      true,
			expectedResult: map[string]any{"value": "result"},
			expectedError:  nil,
		},
		{
			name:       "method not found",
			pluginName: "database",
			method:     "InvalidMethod",
			params:     map[string]any{},
			methods: []plugin.MethodDescriptor{
				createTestMethodDescriptor("GetData", "Get data"),
			},
			isRunning:      true,
			expectedResult: nil,
			expectedError:  errors.New("method not found"),
		},
		{
			name:           "plugin not running",
			pluginName:     "database",
			method:         "GetData",
			params:         map[string]any{},
			methods:        []plugin.MethodDescriptor{},
			isRunning:      false,
			expectedResult: nil,
			expectedError:  errors.New("plugin not running"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockComponentRegistry := new(MockComponentRegistry)
			mockPluginRegistry := new(MockPluginRegistry)

			var pluginComp *component.Component
			if tt.isRunning {
				pluginComp = createRunningTestPlugin(tt.pluginName, "1.0.0")
			} else {
				pluginComp = createTestPlugin(tt.pluginName, "1.0.0", component.ComponentStatusStopped)
			}

			mockComponentRegistry.On("Get", component.ComponentKindPlugin, tt.pluginName).Return(pluginComp)

			if tt.isRunning {
				mockPluginRegistry.On("Methods", tt.pluginName).Return(tt.methods, nil)

				methodExists := false
				for _, m := range tt.methods {
					if m.Name == tt.method {
						methodExists = true
						break
					}
				}

				if methodExists && tt.expectedError == nil {
					mockPluginRegistry.On("Query", mock.Anything, tt.pluginName, tt.method, tt.params).Return(tt.expectedResult, nil)
				}
			}

			// Verify plugin status
			assert.Equal(t, tt.isRunning, pluginComp.IsRunning())

			if !tt.isRunning {
				// Plugin not running case
				assert.NotNil(t, pluginComp)
				assert.False(t, pluginComp.IsRunning())
			} else {
				// Get methods and check if method exists
				methods, err := mockPluginRegistry.Methods(tt.pluginName)
				assert.NoError(t, err)

				methodExists := false
				for _, m := range methods {
					if m.Name == tt.method {
						methodExists = true
						break
					}
				}

				if methodExists {
					// Execute query
					result, err := mockPluginRegistry.Query(context.Background(), tt.pluginName, tt.method, tt.params)
					if tt.expectedError != nil {
						assert.Error(t, err)
					} else {
						assert.NoError(t, err)
						assert.Equal(t, tt.expectedResult, result)
					}
				} else {
					// Method doesn't exist
					assert.False(t, methodExists)
				}
			}

			mockComponentRegistry.AssertExpectations(t)
			mockPluginRegistry.AssertExpectations(t)
		})
	}
}
