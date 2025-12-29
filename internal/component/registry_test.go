package component

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a valid test component
func createTestComponent(kind ComponentKind, name string) *Component {
	return &Component{
		Kind:    kind,
		Name:    name,
		Version: "1.0.0",
		Path:    "/path/to/" + name,
		Source:  ComponentSourceExternal,
		Status:  ComponentStatusAvailable,
		Manifest: &Manifest{
			Kind:    kind,
			Name:    name,
			Version: "1.0.0",
			Runtime: RuntimeConfig{
				Type:       RuntimeTypeBinary,
				Entrypoint: "/usr/bin/" + name,
			},
		},
	}
}

// TestNewDefaultComponentRegistry tests creating a new registry instance
func TestNewDefaultComponentRegistry(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	assert.NotNil(t, registry, "registry should not be nil")
	assert.NotNil(t, registry.components, "components map should be initialized")
	assert.Equal(t, 0, registry.Count(), "new registry should be empty")
}

// TestRegister_Success tests successful component registration
func TestRegister_Success(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	component := createTestComponent(ComponentKindAgent, "test-agent")
	err := registry.Register(component)

	assert.NoError(t, err, "registration should succeed")
	assert.Equal(t, 1, registry.Count(), "registry should contain 1 component")

	// Verify component can be retrieved
	retrieved := registry.Get(ComponentKindAgent, "test-agent")
	assert.NotNil(t, retrieved, "component should be retrievable")
	assert.Equal(t, "test-agent", retrieved.Name)
}

// TestRegister_NilComponent tests registering nil component
func TestRegister_NilComponent(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	err := registry.Register(nil)

	assert.Error(t, err, "should return error for nil component")
	assert.Equal(t, 0, registry.Count(), "registry should remain empty")
}

// TestRegister_InvalidComponent tests registering invalid component
func TestRegister_InvalidComponent(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	testCases := []struct {
		name      string
		component *Component
	}{
		{
			name: "empty name",
			component: &Component{
				Kind:    ComponentKindAgent,
				Name:    "",
				Version: "1.0.0",
				Path:    "/path",
				Source:  ComponentSourceExternal,
				Status:  ComponentStatusAvailable,
			},
		},
		{
			name: "empty version",
			component: &Component{
				Kind:    ComponentKindAgent,
				Name:    "test",
				Version: "",
				Path:    "/path",
				Source:  ComponentSourceExternal,
				Status:  ComponentStatusAvailable,
			},
		},
		{
			name: "invalid kind",
			component: &Component{
				Kind:    ComponentKind("invalid"),
				Name:    "test",
				Version: "1.0.0",
				Path:    "/path",
				Source:  ComponentSourceExternal,
				Status:  ComponentStatusAvailable,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := registry.Register(tc.component)
			assert.Error(t, err, "should return error for invalid component")
		})
	}
}

// TestRegister_DuplicateComponent tests registering duplicate component
func TestRegister_DuplicateComponent(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	component := createTestComponent(ComponentKindAgent, "test-agent")

	// First registration should succeed
	err := registry.Register(component)
	assert.NoError(t, err, "first registration should succeed")

	// Second registration should fail
	duplicate := createTestComponent(ComponentKindAgent, "test-agent")
	err = registry.Register(duplicate)

	assert.Error(t, err, "duplicate registration should fail")
	var compErr *ComponentError
	assert.ErrorAs(t, err, &compErr)
	assert.Equal(t, ErrCodeComponentExists, compErr.Code)
}

// TestRegister_MultipleKinds tests registering components of different kinds
func TestRegister_MultipleKinds(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	agent := createTestComponent(ComponentKindAgent, "agent1")
	tool := createTestComponent(ComponentKindTool, "tool1")
	plugin := createTestComponent(ComponentKindPlugin, "plugin1")

	require.NoError(t, registry.Register(agent))
	require.NoError(t, registry.Register(tool))
	require.NoError(t, registry.Register(plugin))

	assert.Equal(t, 3, registry.Count(), "should have 3 components")
	assert.Equal(t, 1, registry.CountByKind(ComponentKindAgent))
	assert.Equal(t, 1, registry.CountByKind(ComponentKindTool))
	assert.Equal(t, 1, registry.CountByKind(ComponentKindPlugin))
}

// TestRegister_SetsTimestamps tests that registration sets timestamps
func TestRegister_SetsTimestamps(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	component := createTestComponent(ComponentKindAgent, "test-agent")
	component.CreatedAt = time.Time{} // Zero value
	component.UpdatedAt = time.Time{} // Zero value

	before := time.Now()
	err := registry.Register(component)
	after := time.Now()

	assert.NoError(t, err)

	retrieved := registry.Get(ComponentKindAgent, "test-agent")
	assert.NotNil(t, retrieved)
	assert.True(t, !retrieved.CreatedAt.IsZero(), "CreatedAt should be set")
	assert.True(t, !retrieved.UpdatedAt.IsZero(), "UpdatedAt should be set")
	assert.True(t, retrieved.CreatedAt.After(before) || retrieved.CreatedAt.Equal(before))
	assert.True(t, retrieved.CreatedAt.Before(after) || retrieved.CreatedAt.Equal(after))
}

// TestUnregister_Success tests successful component unregistration
func TestUnregister_Success(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	component := createTestComponent(ComponentKindAgent, "test-agent")
	require.NoError(t, registry.Register(component))

	err := registry.Unregister(ComponentKindAgent, "test-agent")

	assert.NoError(t, err, "unregistration should succeed")
	assert.Equal(t, 0, registry.Count(), "registry should be empty")
	assert.Nil(t, registry.Get(ComponentKindAgent, "test-agent"), "component should not be retrievable")
}

// TestUnregister_NotFound tests unregistering non-existent component
func TestUnregister_NotFound(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	err := registry.Unregister(ComponentKindAgent, "nonexistent")

	assert.Error(t, err, "should return error for non-existent component")
	var compErr *ComponentError
	assert.ErrorAs(t, err, &compErr)
	assert.Equal(t, ErrCodeComponentNotFound, compErr.Code)
}

// TestUnregister_InvalidKind tests unregistering with invalid kind
func TestUnregister_InvalidKind(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	err := registry.Unregister(ComponentKind("invalid"), "test")

	assert.Error(t, err, "should return error for invalid kind")
	var compErr *ComponentError
	assert.ErrorAs(t, err, &compErr)
	assert.Equal(t, ErrCodeInvalidKind, compErr.Code)
}

// TestUnregister_EmptyName tests unregistering with empty name
func TestUnregister_EmptyName(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	err := registry.Unregister(ComponentKindAgent, "")

	assert.Error(t, err, "should return error for empty name")
}

// TestGet_Exists tests retrieving existing component
func TestGet_Exists(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	original := createTestComponent(ComponentKindAgent, "test-agent")
	require.NoError(t, registry.Register(original))

	retrieved := registry.Get(ComponentKindAgent, "test-agent")

	assert.NotNil(t, retrieved, "component should be retrievable")
	assert.Equal(t, original.Name, retrieved.Name)
	assert.Equal(t, original.Kind, retrieved.Kind)
	assert.Equal(t, original.Version, retrieved.Version)
}

// TestGet_NotFound tests retrieving non-existent component
func TestGet_NotFound(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	retrieved := registry.Get(ComponentKindAgent, "nonexistent")

	assert.Nil(t, retrieved, "should return nil for non-existent component")
}

// TestGet_WrongKind tests retrieving component with wrong kind
func TestGet_WrongKind(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	component := createTestComponent(ComponentKindAgent, "test-agent")
	require.NoError(t, registry.Register(component))

	// Try to retrieve as different kind
	retrieved := registry.Get(ComponentKindTool, "test-agent")

	assert.Nil(t, retrieved, "should return nil when kind doesn't match")
}

// TestList_Empty tests listing when no components exist
func TestList_Empty(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	components := registry.List(ComponentKindAgent)

	assert.NotNil(t, components, "should return non-nil slice")
	assert.Empty(t, components, "should return empty slice")
}

// TestList_MultipleComponents tests listing multiple components
func TestList_MultipleComponents(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	// Register multiple agents
	for i := 1; i <= 3; i++ {
		component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		require.NoError(t, registry.Register(component))
	}

	// Register a tool
	tool := createTestComponent(ComponentKindTool, "tool1")
	require.NoError(t, registry.Register(tool))

	agents := registry.List(ComponentKindAgent)
	tools := registry.List(ComponentKindTool)

	assert.Len(t, agents, 3, "should have 3 agents")
	assert.Len(t, tools, 1, "should have 1 tool")
}

// TestListAll_Empty tests listing all when no components exist
func TestListAll_Empty(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	all := registry.ListAll()

	assert.NotNil(t, all, "should return non-nil map")
	assert.Empty(t, all, "should return empty map")
}

// TestListAll_MultipleKinds tests listing all with multiple kinds
func TestListAll_MultipleKinds(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	// Register components of different kinds
	for i := 1; i <= 2; i++ {
		agent := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		tool := createTestComponent(ComponentKindTool, fmt.Sprintf("tool%d", i))
		require.NoError(t, registry.Register(agent))
		require.NoError(t, registry.Register(tool))
	}

	all := registry.ListAll()

	assert.Len(t, all, 2, "should have 2 kinds")
	assert.Len(t, all[ComponentKindAgent], 2, "should have 2 agents")
	assert.Len(t, all[ComponentKindTool], 2, "should have 2 tools")
}

// TestLoadFromConfig_Success tests loading from config file
func TestLoadFromConfig_Success(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `
agents:
  - kind: agent
    name: scanner
    version: 1.0.0
    path: /path/to/scanner
    source: external
    status: available
    manifest:
      kind: agent
      name: scanner
      version: 1.0.0
      runtime:
        type: binary
        entrypoint: /usr/bin/scanner
tools:
  - kind: tool
    name: nmap
    version: 2.0.0
    path: /path/to/nmap
    source: external
    status: available
    manifest:
      kind: tool
      name: nmap
      version: 2.0.0
      runtime:
        type: binary
        entrypoint: /usr/bin/nmap
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

	err := registry.LoadFromConfig(configPath)

	assert.NoError(t, err, "loading should succeed")
	assert.Equal(t, 2, registry.Count(), "should have 2 components")
	assert.NotNil(t, registry.Get(ComponentKindAgent, "scanner"))
	assert.NotNil(t, registry.Get(ComponentKindTool, "nmap"))
}

// TestLoadFromConfig_FileNotFound tests loading from non-existent file
func TestLoadFromConfig_FileNotFound(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	err := registry.LoadFromConfig("/nonexistent/path/config.yaml")

	assert.Error(t, err, "should return error for non-existent file")
}

// TestLoadFromConfig_InvalidYAML tests loading invalid YAML
func TestLoadFromConfig_InvalidYAML(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidContent := `
agents:
  - kind: agent
    name: test
    invalid yaml content [[[
`
	require.NoError(t, os.WriteFile(configPath, []byte(invalidContent), 0644))

	err := registry.LoadFromConfig(configPath)

	assert.Error(t, err, "should return error for invalid YAML")
}

// TestLoadFromConfig_InvalidComponent tests loading config with invalid component
func TestLoadFromConfig_InvalidComponent(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid-component.yaml")

	invalidContent := `
agents:
  - kind: agent
    name: ""
    version: 1.0.0
    path: /path
    source: external
    status: available
`
	require.NoError(t, os.WriteFile(configPath, []byte(invalidContent), 0644))

	err := registry.LoadFromConfig(configPath)

	assert.Error(t, err, "should return error for invalid component")
}

// TestSave_Success tests saving registry to file
func TestSave_Success(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	// Register some components
	agent := createTestComponent(ComponentKindAgent, "test-agent")
	tool := createTestComponent(ComponentKindTool, "test-tool")
	require.NoError(t, registry.Register(agent))
	require.NoError(t, registry.Register(tool))

	// Override home directory for testing
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	err := registry.Save()

	assert.NoError(t, err, "save should succeed")

	// Verify file was created
	registryPath := filepath.Join(tmpDir, ".gibson", "registry.yaml")
	assert.FileExists(t, registryPath, "registry file should exist")

	// Verify content can be loaded
	newRegistry := NewDefaultComponentRegistry()
	err = newRegistry.LoadFromConfig(registryPath)
	assert.NoError(t, err, "should be able to load saved registry")
	assert.Equal(t, 2, newRegistry.Count(), "should have same number of components")
}

// TestCount tests counting components
func TestCount(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	assert.Equal(t, 0, registry.Count(), "empty registry should have count 0")

	for i := 1; i <= 5; i++ {
		component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		require.NoError(t, registry.Register(component))
	}

	assert.Equal(t, 5, registry.Count(), "should have count 5")
}

// TestCountByKind tests counting components by kind
func TestCountByKind(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	// Register different kinds
	for i := 1; i <= 3; i++ {
		agent := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		require.NoError(t, registry.Register(agent))
	}
	for i := 1; i <= 2; i++ {
		tool := createTestComponent(ComponentKindTool, fmt.Sprintf("tool%d", i))
		require.NoError(t, registry.Register(tool))
	}

	assert.Equal(t, 3, registry.CountByKind(ComponentKindAgent))
	assert.Equal(t, 2, registry.CountByKind(ComponentKindTool))
	assert.Equal(t, 0, registry.CountByKind(ComponentKindPlugin))
}

// TestClear tests clearing the registry
func TestClear(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	// Register some components
	for i := 1; i <= 5; i++ {
		component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		require.NoError(t, registry.Register(component))
	}

	assert.Equal(t, 5, registry.Count(), "should have 5 components before clear")

	registry.Clear()

	assert.Equal(t, 0, registry.Count(), "should have 0 components after clear")
	assert.Empty(t, registry.ListAll(), "ListAll should return empty map")
}

// TestUpdate_Success tests updating existing component
func TestUpdate_Success(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	component := createTestComponent(ComponentKindAgent, "test-agent")
	require.NoError(t, registry.Register(component))

	// Update the component (keep status as Available, just update version)
	component.Version = "2.0.0"
	component.Path = "/new/path/to/agent"

	err := registry.Update(component)

	assert.NoError(t, err, "update should succeed")

	// Verify update
	retrieved := registry.Get(ComponentKindAgent, "test-agent")
	assert.Equal(t, "2.0.0", retrieved.Version)
	assert.Equal(t, "/new/path/to/agent", retrieved.Path)
	assert.Equal(t, ComponentStatusAvailable, retrieved.Status)
}

// TestUpdate_NotFound tests updating non-existent component
func TestUpdate_NotFound(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	component := createTestComponent(ComponentKindAgent, "nonexistent")
	err := registry.Update(component)

	assert.Error(t, err, "should return error for non-existent component")
	var compErr *ComponentError
	assert.ErrorAs(t, err, &compErr)
	assert.Equal(t, ErrCodeComponentNotFound, compErr.Code)
}

// TestUpdate_NilComponent tests updating nil component
func TestUpdate_NilComponent(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	err := registry.Update(nil)

	assert.Error(t, err, "should return error for nil component")
}

// TestUpdate_UpdatesTimestamp tests that update sets UpdatedAt
func TestUpdate_UpdatesTimestamp(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	component := createTestComponent(ComponentKindAgent, "test-agent")
	require.NoError(t, registry.Register(component))

	// Wait a bit to ensure timestamp difference
	time.Sleep(10 * time.Millisecond)

	originalUpdatedAt := registry.Get(ComponentKindAgent, "test-agent").UpdatedAt
	component.Version = "2.0.0"

	err := registry.Update(component)
	assert.NoError(t, err)

	retrieved := registry.Get(ComponentKindAgent, "test-agent")
	assert.True(t, retrieved.UpdatedAt.After(originalUpdatedAt), "UpdatedAt should be updated")
}

// TestConcurrentRegister tests concurrent registration
func TestConcurrentRegister(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", id))
			if err := registry.Register(component); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("unexpected error during concurrent registration: %v", err)
	}

	assert.Equal(t, numGoroutines, registry.Count(), "all registrations should succeed")
}

// TestConcurrentUnregister tests concurrent unregistration
func TestConcurrentUnregister(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	const numComponents = 100

	// Register components
	for i := 0; i < numComponents; i++ {
		component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		require.NoError(t, registry.Register(component))
	}

	var wg sync.WaitGroup
	wg.Add(numComponents)

	for i := 0; i < numComponents; i++ {
		go func(id int) {
			defer wg.Done()
			_ = registry.Unregister(ComponentKindAgent, fmt.Sprintf("agent%d", id))
		}(i)
	}

	wg.Wait()

	assert.Equal(t, 0, registry.Count(), "all components should be unregistered")
}

// TestConcurrentReadWrite tests concurrent reads and writes
func TestConcurrentReadWrite(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	const numOperations = 1000
	var wg sync.WaitGroup

	// Writers
	wg.Add(numOperations)
	for i := 0; i < numOperations; i++ {
		go func(id int) {
			defer wg.Done()
			component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", id))
			_ = registry.Register(component)
		}(i)
	}

	// Readers
	wg.Add(numOperations)
	for i := 0; i < numOperations; i++ {
		go func(id int) {
			defer wg.Done()
			_ = registry.Get(ComponentKindAgent, fmt.Sprintf("agent%d", id))
			_ = registry.List(ComponentKindAgent)
			_ = registry.Count()
		}(i)
	}

	wg.Wait()

	// Should complete without data races
	assert.True(t, registry.Count() > 0, "should have registered some components")
}

// TestConcurrentListAll tests concurrent ListAll calls
func TestConcurrentListAll(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	// Register some components
	for i := 0; i < 10; i++ {
		component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		require.NoError(t, registry.Register(component))
	}

	const numReaders = 100
	var wg sync.WaitGroup
	wg.Add(numReaders)

	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()
			all := registry.ListAll()
			assert.NotNil(t, all, "ListAll should return non-nil map")
		}()
	}

	wg.Wait()
}

// TestConcurrentSave tests that Save is thread-safe
func TestConcurrentSave(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	// Register some components
	for i := 0; i < 10; i++ {
		component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		require.NoError(t, registry.Register(component))
	}

	// Override home directory for testing
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	const numSavers = 10
	var wg sync.WaitGroup
	wg.Add(numSavers)

	errors := make(chan error, numSavers)

	for i := 0; i < numSavers; i++ {
		go func() {
			defer wg.Done()
			if err := registry.Save(); err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("unexpected error during concurrent save: %v", err)
	}

	// Verify file exists
	registryPath := filepath.Join(tmpDir, ".gibson", "registry.yaml")
	assert.FileExists(t, registryPath, "registry file should exist")
}

// Benchmark tests

func BenchmarkRegister(b *testing.B) {
	registry := NewDefaultComponentRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		_ = registry.Register(component)
	}
}

func BenchmarkGet(b *testing.B) {
	registry := NewDefaultComponentRegistry()

	// Pre-populate registry
	for i := 0; i < 1000; i++ {
		component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		_ = registry.Register(component)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.Get(ComponentKindAgent, fmt.Sprintf("agent%d", i%1000))
	}
}

func BenchmarkList(b *testing.B) {
	registry := NewDefaultComponentRegistry()

	// Pre-populate registry
	for i := 0; i < 100; i++ {
		component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		_ = registry.Register(component)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.List(ComponentKindAgent)
	}
}

func BenchmarkListAll(b *testing.B) {
	registry := NewDefaultComponentRegistry()

	// Pre-populate registry with all kinds
	for i := 0; i < 50; i++ {
		agent := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
		tool := createTestComponent(ComponentKindTool, fmt.Sprintf("tool%d", i))
		_ = registry.Register(agent)
		_ = registry.Register(tool)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.ListAll()
	}
}

func BenchmarkConcurrentReadWrite(b *testing.B) {
	registry := NewDefaultComponentRegistry()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				component := createTestComponent(ComponentKindAgent, fmt.Sprintf("agent%d", i))
				_ = registry.Register(component)
			} else {
				_ = registry.Get(ComponentKindAgent, fmt.Sprintf("agent%d", i))
			}
			i++
		}
	})
}
