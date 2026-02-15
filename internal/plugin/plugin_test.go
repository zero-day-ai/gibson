package plugin

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/sdk/schema"

	"github.com/zero-day-ai/gibson/internal/types"
)

// MockPlugin implements Plugin interface for testing
type MockPlugin struct {
	name          string
	version       string
	methods       []MethodDescriptor
	initialized   bool
	shutdownCalls int
	queryCalls    map[string]int
	queryResults  map[string]any
	queryErrors   map[string]error
	initError     error
	shutdownError error
	healthStatus  types.HealthStatus
	mu            sync.Mutex
}

// NewMockPlugin creates a new MockPlugin
func NewMockPlugin(name, version string) *MockPlugin {
	return &MockPlugin{
		name:         name,
		version:      version,
		methods:      []MethodDescriptor{},
		queryCalls:   make(map[string]int),
		queryResults: make(map[string]any),
		queryErrors:  make(map[string]error),
		healthStatus: types.Healthy("mock plugin healthy"),
	}
}

func (m *MockPlugin) Name() string {
	return m.name
}

func (m *MockPlugin) Version() string {
	return m.version
}

func (m *MockPlugin) Initialize(ctx context.Context, cfg PluginConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.initError != nil {
		return m.initError
	}

	m.initialized = true
	return nil
}

func (m *MockPlugin) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shutdownCalls++
	m.initialized = false

	if m.shutdownError != nil {
		return m.shutdownError
	}

	return nil
}

func (m *MockPlugin) Query(ctx context.Context, method string, params map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queryCalls[method]++

	if err, exists := m.queryErrors[method]; exists {
		return nil, err
	}

	if result, exists := m.queryResults[method]; exists {
		return result, nil
	}

	return nil, fmt.Errorf("unknown method: %s", method)
}

func (m *MockPlugin) Methods() []MethodDescriptor {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.methods
}

func (m *MockPlugin) Health(ctx context.Context) types.HealthStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.healthStatus
}

// Helper methods for test setup
func (m *MockPlugin) AddMethod(name, description string, inputSchema, outputSchema schema.JSON) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.methods = append(m.methods, MethodDescriptor{
		Name:         name,
		Description:  description,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	})
}

func (m *MockPlugin) SetQueryResult(method string, result any) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queryResults[method] = result
}

func (m *MockPlugin) SetQueryError(method string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queryErrors[method] = err
}

func (m *MockPlugin) SetInitError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.initError = err
}

func (m *MockPlugin) SetShutdownError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shutdownError = err
}

func (m *MockPlugin) SetHealthStatus(status types.HealthStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.healthStatus = status
}

func (m *MockPlugin) GetQueryCallCount(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.queryCalls[method]
}

func (m *MockPlugin) GetShutdownCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.shutdownCalls
}

func (m *MockPlugin) IsInitialized() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.initialized
}

// Tests

func TestNewPluginRegistry(t *testing.T) {
	registry := NewPluginRegistry(nil)

	assert.NotNil(t, registry)
	assert.Empty(t, registry.plugins)
	assert.Empty(t, registry.order)
}

func TestRegister_Success(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin := NewMockPlugin("test-plugin", "1.0.0")

	cfg := PluginConfig{
		Name:       "test-plugin",
		Settings:   map[string]any{"key": "value"},
		Timeout:    5 * time.Second,
		RetryCount: 3,
	}

	err := registry.Register(plugin, cfg)
	require.NoError(t, err)

	assert.True(t, plugin.IsInitialized())
	assert.Len(t, registry.plugins, 1)
	assert.Contains(t, registry.order, "test-plugin")
}

func TestRegister_EmptyName(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin := NewMockPlugin("", "1.0.0")

	cfg := PluginConfig{}

	err := registry.Register(plugin, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name cannot be empty")
}

func TestRegister_AlreadyRegistered(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin1 := NewMockPlugin("test-plugin", "1.0.0")
	plugin2 := NewMockPlugin("test-plugin", "2.0.0")

	cfg := PluginConfig{Name: "test-plugin"}

	err := registry.Register(plugin1, cfg)
	require.NoError(t, err)

	err = registry.Register(plugin2, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegister_InitializationError(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin := NewMockPlugin("test-plugin", "1.0.0")
	plugin.SetInitError(errors.New("init failed"))

	cfg := PluginConfig{Name: "test-plugin"}

	err := registry.Register(plugin, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize")
	assert.Contains(t, err.Error(), "init failed")

	// Plugin should be in registry but with error status
	entry := registry.plugins["test-plugin"]
	require.NotNil(t, entry)
	assert.Equal(t, PluginStatusError, entry.status)
}

func TestRegisterExternal_Success(t *testing.T) {
	registry := NewPluginRegistry(nil)
	client := NewMockPlugin("external-plugin", "1.0.0")

	cfg := PluginConfig{Name: "external-plugin"}

	err := registry.RegisterExternal("external-plugin", client, cfg)
	require.NoError(t, err)

	assert.True(t, client.IsInitialized())
	assert.Len(t, registry.plugins, 1)

	entry := registry.plugins["external-plugin"]
	require.NotNil(t, entry)
	assert.True(t, entry.external)
	assert.Equal(t, PluginStatusRunning, entry.status)
}

func TestUnregister_Success(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin := NewMockPlugin("test-plugin", "1.0.0")

	cfg := PluginConfig{Name: "test-plugin"}

	err := registry.Register(plugin, cfg)
	require.NoError(t, err)

	err = registry.Unregister("test-plugin")
	require.NoError(t, err)

	assert.Equal(t, 1, plugin.GetShutdownCallCount())
	assert.Empty(t, registry.plugins)
	assert.Empty(t, registry.order)
}

func TestUnregister_NotFound(t *testing.T) {
	registry := NewPluginRegistry(nil)

	err := registry.Unregister("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUnregister_ShutdownError(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin := NewMockPlugin("test-plugin", "1.0.0")
	plugin.SetShutdownError(errors.New("shutdown failed"))

	cfg := PluginConfig{Name: "test-plugin"}

	err := registry.Register(plugin, cfg)
	require.NoError(t, err)

	err = registry.Unregister("test-plugin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to shutdown")
	assert.Contains(t, err.Error(), "shutdown failed")
}

func TestGet_Success(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin := NewMockPlugin("test-plugin", "1.0.0")

	cfg := PluginConfig{Name: "test-plugin"}

	err := registry.Register(plugin, cfg)
	require.NoError(t, err)

	retrieved, err := registry.Get("test-plugin")
	require.NoError(t, err)
	assert.Equal(t, plugin, retrieved)
}

func TestGet_NotFound(t *testing.T) {
	registry := NewPluginRegistry(nil)

	_, err := registry.Get("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGet_NotRunning(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin := NewMockPlugin("test-plugin", "1.0.0")
	plugin.SetInitError(errors.New("init error"))

	cfg := PluginConfig{Name: "test-plugin"}

	_ = registry.Register(plugin, cfg)

	_, err := registry.Get("test-plugin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestList(t *testing.T) {
	registry := NewPluginRegistry(nil)

	plugin1 := NewMockPlugin("plugin1", "1.0.0")
	plugin1.AddMethod("method1", "test method", schema.JSON{}, schema.JSON{})

	plugin2 := NewMockPlugin("plugin2", "2.0.0")

	cfg1 := PluginConfig{Name: "plugin1"}
	cfg2 := PluginConfig{Name: "plugin2"}

	err := registry.Register(plugin1, cfg1)
	require.NoError(t, err)

	err = registry.RegisterExternal("plugin2", plugin2, cfg2)
	require.NoError(t, err)

	descriptors := registry.List()
	assert.Len(t, descriptors, 2)

	// Find descriptors by name
	var desc1, desc2 *PluginDescriptor
	for i := range descriptors {
		if descriptors[i].Name == "plugin1" {
			desc1 = &descriptors[i]
		} else if descriptors[i].Name == "plugin2" {
			desc2 = &descriptors[i]
		}
	}

	require.NotNil(t, desc1)
	require.NotNil(t, desc2)

	assert.Equal(t, "plugin1", desc1.Name)
	assert.Equal(t, "1.0.0", desc1.Version)
	assert.Len(t, desc1.Methods, 1)
	assert.False(t, desc1.IsExternal)
	assert.Equal(t, PluginStatusRunning, desc1.Status)

	assert.Equal(t, "plugin2", desc2.Name)
	assert.Equal(t, "2.0.0", desc2.Version)
	assert.True(t, desc2.IsExternal)
	assert.Equal(t, PluginStatusRunning, desc2.Status)
}

func TestMethods_Success(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin := NewMockPlugin("test-plugin", "1.0.0")

	inputSchema := schema.Object(map[string]schema.JSON{
		"query": schema.StringWithDesc("search query"),
	}, "query")

	outputSchema := schema.Object(map[string]schema.JSON{
		"results": schema.StringWithDesc("search results"),
	}, "results")

	plugin.AddMethod("search", "Search method", inputSchema, outputSchema)

	cfg := PluginConfig{Name: "test-plugin"}

	err := registry.Register(plugin, cfg)
	require.NoError(t, err)

	methods, err := registry.Methods("test-plugin")
	require.NoError(t, err)
	assert.Len(t, methods, 1)
	assert.Equal(t, "search", methods[0].Name)
	assert.Equal(t, "Search method", methods[0].Description)
}

func TestMethods_PluginNotFound(t *testing.T) {
	registry := NewPluginRegistry(nil)

	_, err := registry.Methods("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestQuery_Success(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin := NewMockPlugin("test-plugin", "1.0.0")

	expectedResult := map[string]any{"data": "test result"}
	plugin.SetQueryResult("search", expectedResult)

	cfg := PluginConfig{Name: "test-plugin"}

	err := registry.Register(plugin, cfg)
	require.NoError(t, err)

	ctx := context.Background()
	params := map[string]any{"query": "test"}

	result, err := registry.Query(ctx, "test-plugin", "search", params)
	require.NoError(t, err)
	assert.Equal(t, expectedResult, result)
	assert.Equal(t, 1, plugin.GetQueryCallCount("search"))
}

func TestQuery_PluginNotFound(t *testing.T) {
	registry := NewPluginRegistry(nil)
	ctx := context.Background()

	_, err := registry.Query(ctx, "nonexistent", "search", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestQuery_UnknownMethod(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin := NewMockPlugin("test-plugin", "1.0.0")

	cfg := PluginConfig{Name: "test-plugin"}

	err := registry.Register(plugin, cfg)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = registry.Query(ctx, "test-plugin", "unknown", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown method")
}

func TestQuery_MethodError(t *testing.T) {
	registry := NewPluginRegistry(nil)
	plugin := NewMockPlugin("test-plugin", "1.0.0")
	plugin.SetQueryError("search", errors.New("query failed"))

	cfg := PluginConfig{Name: "test-plugin"}

	err := registry.Register(plugin, cfg)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = registry.Query(ctx, "test-plugin", "search", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "query failed")
}

func TestShutdown_ReverseOrder(t *testing.T) {
	registry := NewPluginRegistry(nil)

	// Use shared tracker for shutdown order
	var shutdownOrder []string
	var orderMu sync.Mutex

	// Create tracking plugins
	plugin1 := NewMockPlugin("plugin1", "1.0.0")
	plugin2 := NewMockPlugin("plugin2", "1.0.0")
	plugin3 := NewMockPlugin("plugin3", "1.0.0")

	// Track original shutdown methods by wrapping in a tracker
	tracker := &shutdownTracker{
		order: &shutdownOrder,
		mu:    &orderMu,
		plugins: map[string]*MockPlugin{
			"plugin1": plugin1,
			"plugin2": plugin2,
			"plugin3": plugin3,
		},
	}

	// Wrap plugins with tracker
	trackedPlugin1 := tracker.wrap(plugin1)
	trackedPlugin2 := tracker.wrap(plugin2)
	trackedPlugin3 := tracker.wrap(plugin3)

	cfg := PluginConfig{}

	err := registry.Register(trackedPlugin1, cfg)
	require.NoError(t, err)

	err = registry.Register(trackedPlugin2, cfg)
	require.NoError(t, err)

	err = registry.Register(trackedPlugin3, cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = registry.Shutdown(ctx)
	require.NoError(t, err)

	// Verify shutdown was called in reverse order
	require.Len(t, shutdownOrder, 3)
	assert.Equal(t, "plugin3", shutdownOrder[0])
	assert.Equal(t, "plugin2", shutdownOrder[1])
	assert.Equal(t, "plugin1", shutdownOrder[2])

	// Verify registry is cleared
	assert.Empty(t, registry.plugins)
	assert.Empty(t, registry.order)
}

// shutdownTracker tracks shutdown order for testing
type shutdownTracker struct {
	order   *[]string
	mu      *sync.Mutex
	plugins map[string]*MockPlugin
}

func (st *shutdownTracker) wrap(plugin *MockPlugin) *trackingPlugin {
	return &trackingPlugin{
		MockPlugin: plugin,
		tracker:    st,
	}
}

type trackingPlugin struct {
	*MockPlugin
	tracker *shutdownTracker
}

func (tp *trackingPlugin) Shutdown(ctx context.Context) error {
	tp.tracker.mu.Lock()
	*tp.tracker.order = append(*tp.tracker.order, tp.Name())
	tp.tracker.mu.Unlock()

	time.Sleep(10 * time.Millisecond) // Small delay to ensure ordering is visible
	return tp.MockPlugin.Shutdown(ctx)
}

func TestShutdown_WithErrors(t *testing.T) {
	registry := NewPluginRegistry(nil)

	plugin1 := NewMockPlugin("plugin1", "1.0.0")
	plugin2 := NewMockPlugin("plugin2", "1.0.0")
	plugin2.SetShutdownError(errors.New("shutdown error"))

	cfg := PluginConfig{}

	err := registry.Register(plugin1, cfg)
	require.NoError(t, err)

	err = registry.Register(plugin2, cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = registry.Shutdown(ctx)

	// Should return error but still shutdown all plugins
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutdown errors")
	assert.Equal(t, 1, plugin1.GetShutdownCallCount())
	assert.Equal(t, 1, plugin2.GetShutdownCallCount())
}

func TestHealth_NoPlugins(t *testing.T) {
	registry := NewPluginRegistry(nil)
	ctx := context.Background()

	health := registry.Health(ctx)
	assert.Equal(t, types.HealthStateHealthy, health.State)
	assert.Contains(t, health.Message, "no plugins")
}

func TestHealth_AllHealthy(t *testing.T) {
	registry := NewPluginRegistry(nil)

	plugin1 := NewMockPlugin("plugin1", "1.0.0")
	plugin1.SetHealthStatus(types.Healthy("plugin1 ok"))

	plugin2 := NewMockPlugin("plugin2", "1.0.0")
	plugin2.SetHealthStatus(types.Healthy("plugin2 ok"))

	cfg := PluginConfig{}

	err := registry.Register(plugin1, cfg)
	require.NoError(t, err)

	err = registry.Register(plugin2, cfg)
	require.NoError(t, err)

	ctx := context.Background()
	health := registry.Health(ctx)

	assert.Equal(t, types.HealthStateHealthy, health.State)
	assert.Contains(t, health.Message, "2/2 plugins healthy")
}

func TestHealth_OneDegraded(t *testing.T) {
	registry := NewPluginRegistry(nil)

	plugin1 := NewMockPlugin("plugin1", "1.0.0")
	plugin1.SetHealthStatus(types.Healthy("plugin1 ok"))

	plugin2 := NewMockPlugin("plugin2", "1.0.0")
	plugin2.SetHealthStatus(types.Degraded("plugin2 slow"))

	cfg := PluginConfig{}

	err := registry.Register(plugin1, cfg)
	require.NoError(t, err)

	err = registry.Register(plugin2, cfg)
	require.NoError(t, err)

	ctx := context.Background()
	health := registry.Health(ctx)

	assert.Equal(t, types.HealthStateDegraded, health.State)
	assert.Contains(t, health.Message, "1/2 plugins degraded")
}

func TestHealth_OneUnhealthy(t *testing.T) {
	registry := NewPluginRegistry(nil)

	plugin1 := NewMockPlugin("plugin1", "1.0.0")
	plugin1.SetHealthStatus(types.Healthy("plugin1 ok"))

	plugin2 := NewMockPlugin("plugin2", "1.0.0")
	plugin2.SetHealthStatus(types.Unhealthy("plugin2 down"))

	cfg := PluginConfig{}

	err := registry.Register(plugin1, cfg)
	require.NoError(t, err)

	err = registry.Register(plugin2, cfg)
	require.NoError(t, err)

	ctx := context.Background()
	health := registry.Health(ctx)

	assert.Equal(t, types.HealthStateUnhealthy, health.State)
	assert.Contains(t, health.Message, "plugin2 is unhealthy")
	assert.Contains(t, health.Message, "plugin2 down")
}

func TestHealth_NotRunningPlugin(t *testing.T) {
	registry := NewPluginRegistry(nil)

	plugin := NewMockPlugin("plugin1", "1.0.0")
	plugin.SetInitError(errors.New("init failed"))

	cfg := PluginConfig{}

	_ = registry.Register(plugin, cfg)

	ctx := context.Background()
	health := registry.Health(ctx)

	// Plugin not in running state should be counted as degraded
	assert.Equal(t, types.HealthStateDegraded, health.State)
}

func TestConcurrentAccess(t *testing.T) {
	registry := NewPluginRegistry(nil)

	plugin := NewMockPlugin("test-plugin", "1.0.0")
	plugin.SetQueryResult("method1", "result1")

	cfg := PluginConfig{Name: "test-plugin"}

	err := registry.Register(plugin, cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Run concurrent queries
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			_, err := registry.Query(ctx, "test-plugin", "method1", nil)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()

	// Verify all queries were executed
	assert.Equal(t, numGoroutines, plugin.GetQueryCallCount("method1"))
}

func TestPluginStatus_IsValid(t *testing.T) {
	tests := []struct {
		status PluginStatus
		valid  bool
	}{
		{PluginStatusUninitialized, true},
		{PluginStatusInitializing, true},
		{PluginStatusRunning, true},
		{PluginStatusStopping, true},
		{PluginStatusStopped, true},
		{PluginStatusError, true},
		{PluginStatus("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.status.IsValid())
		})
	}
}

func TestPluginStatus_String(t *testing.T) {
	status := PluginStatusRunning
	assert.Equal(t, "running", status.String())
}
