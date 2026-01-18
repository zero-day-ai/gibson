package component_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/registry"
	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// setupTestRegistry creates an embedded etcd registry for testing and returns the client.
func setupTestRegistry(t *testing.T, port int) *registry.EmbeddedRegistry {
	t.Helper()

	cfg := sdkregistry.Config{
		Type:          "embedded",
		DataDir:       t.TempDir(),
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test-gibson",
		TTL:           30,
	}

	reg, err := registry.NewEmbeddedRegistry(cfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		reg.Close()
	})

	return reg
}

// TestComponentStore_Create tests creating a new component
func TestComponentStore_Create(t *testing.T) {
	reg := setupTestRegistry(t, 22379)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Create manifest
	manifest := &component.Manifest{
		Kind:        "agent",
		Name:        "test-agent",
		Version:     "1.0.0",
		Description: "Test agent for unit tests",
	}

	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "test-agent",
		Version:  "1.0.0",
		RepoPath: "/home/user/.gibson/agents/_repos/test-agent",
		BinPath:  "/home/user/.gibson/agents/bin/test-agent",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
		Manifest: manifest,
	}

	err := store.Create(ctx, comp)
	require.NoError(t, err)

	// Verify timestamps were set
	assert.False(t, comp.CreatedAt.IsZero())
	assert.False(t, comp.UpdatedAt.IsZero())

	// Retrieve component
	retrieved, err := store.GetByName(ctx, component.ComponentKindAgent, "test-agent")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Verify fields
	assert.Equal(t, component.ComponentKindAgent, retrieved.Kind)
	assert.Equal(t, "test-agent", retrieved.Name)
	assert.Equal(t, "1.0.0", retrieved.Version)
	assert.Equal(t, comp.RepoPath, retrieved.RepoPath)
	assert.Equal(t, comp.BinPath, retrieved.BinPath)
	assert.Equal(t, component.ComponentSourceExternal, retrieved.Source)
	assert.NotNil(t, retrieved.Manifest)
	assert.Equal(t, "test-agent", retrieved.Manifest.Name)
}

// TestComponentStore_Create_Duplicate tests that duplicate kind+name is rejected
func TestComponentStore_Create_Duplicate(t *testing.T) {
	reg := setupTestRegistry(t, 22380)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Create first component
	comp1 := &component.Component{
		Kind:     component.ComponentKindTool,
		Name:     "nmap",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo1",
		BinPath:  "/path/to/bin1",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	err := store.Create(ctx, comp1)
	require.NoError(t, err)

	// Try to create duplicate with same kind and name
	comp2 := &component.Component{
		Kind:     component.ComponentKindTool,
		Name:     "nmap",
		Version:  "2.0.0", // Different version
		RepoPath: "/path/to/repo2",
		BinPath:  "/path/to/bin2",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	err = store.Create(ctx, comp2)
	assert.ErrorIs(t, err, component.ErrComponentExists)
}

// TestComponentStore_GetByName tests retrieving a component by kind and name
func TestComponentStore_GetByName(t *testing.T) {
	reg := setupTestRegistry(t, 22381)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Create component
	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "davinci",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	err := store.Create(ctx, comp)
	require.NoError(t, err)

	// Retrieve by kind and name
	retrieved, err := store.GetByName(ctx, component.ComponentKindAgent, "davinci")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, "davinci", retrieved.Name)
	assert.Equal(t, component.ComponentKindAgent, retrieved.Kind)
}

// TestComponentStore_GetByName_NotFound tests that nil is returned when component doesn't exist
func TestComponentStore_GetByName_NotFound(t *testing.T) {
	reg := setupTestRegistry(t, 22382)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Try to get non-existent component
	retrieved, err := store.GetByName(ctx, component.ComponentKindTool, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

// TestComponentStore_List tests listing components by kind
func TestComponentStore_List(t *testing.T) {
	reg := setupTestRegistry(t, 22383)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Create multiple components of different kinds
	components := []*component.Component{
		{
			Kind:     component.ComponentKindAgent,
			Name:     "agent-b",
			Version:  "1.0.0",
			RepoPath: "/path/to/agent-b",
			BinPath:  "/path/to/bin/agent-b",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindAgent,
			Name:     "agent-a",
			Version:  "1.0.0",
			RepoPath: "/path/to/agent-a",
			BinPath:  "/path/to/bin/agent-a",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindTool,
			Name:     "tool-x",
			Version:  "1.0.0",
			RepoPath: "/path/to/tool-x",
			BinPath:  "/path/to/bin/tool-x",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindAgent,
			Name:     "agent-c",
			Version:  "1.0.0",
			RepoPath: "/path/to/agent-c",
			BinPath:  "/path/to/bin/agent-c",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusRunning,
		},
	}

	for _, comp := range components {
		err := store.Create(ctx, comp)
		require.NoError(t, err)
	}

	// List agents only
	agents, err := store.List(ctx, component.ComponentKindAgent)
	require.NoError(t, err)
	assert.Len(t, agents, 3)

	// List tools
	tools, err := store.List(ctx, component.ComponentKindTool)
	require.NoError(t, err)
	assert.Len(t, tools, 1)
	assert.Equal(t, "tool-x", tools[0].Name)

	// List plugins (empty)
	plugins, err := store.List(ctx, component.ComponentKindPlugin)
	require.NoError(t, err)
	assert.Len(t, plugins, 0)
}

// TestComponentStore_ListAll tests listing all components grouped by kind
func TestComponentStore_ListAll(t *testing.T) {
	reg := setupTestRegistry(t, 22384)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Create components of different kinds
	components := []*component.Component{
		{
			Kind:     component.ComponentKindAgent,
			Name:     "agent-1",
			Version:  "1.0.0",
			RepoPath: "/path/to/agent-1",
			BinPath:  "/path/to/bin/agent-1",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindAgent,
			Name:     "agent-2",
			Version:  "1.0.0",
			RepoPath: "/path/to/agent-2",
			BinPath:  "/path/to/bin/agent-2",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindTool,
			Name:     "tool-1",
			Version:  "1.0.0",
			RepoPath: "/path/to/tool-1",
			BinPath:  "/path/to/bin/tool-1",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindPlugin,
			Name:     "plugin-1",
			Version:  "1.0.0",
			RepoPath: "/path/to/plugin-1",
			BinPath:  "/path/to/bin/plugin-1",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
	}

	for _, comp := range components {
		err := store.Create(ctx, comp)
		require.NoError(t, err)
	}

	// List all components
	allComponents, err := store.ListAll(ctx)
	require.NoError(t, err)

	// Verify grouping by kind
	assert.Len(t, allComponents[component.ComponentKindAgent], 2)
	assert.Len(t, allComponents[component.ComponentKindTool], 1)
	assert.Len(t, allComponents[component.ComponentKindPlugin], 1)
}

// TestComponentStore_Update tests updating a component's metadata
func TestComponentStore_Update(t *testing.T) {
	reg := setupTestRegistry(t, 22385)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Create initial component
	comp := &component.Component{
		Kind:     component.ComponentKindTool,
		Name:     "test-tool",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	err := store.Create(ctx, comp)
	require.NoError(t, err)

	originalCreatedAt := comp.CreatedAt

	// Wait a bit to ensure UpdatedAt differs
	time.Sleep(10 * time.Millisecond)

	// Update component
	comp.Version = "2.0.0"
	comp.RepoPath = "/new/path/to/repo"
	comp.BinPath = "/new/path/to/bin"

	err = store.Update(ctx, comp)
	require.NoError(t, err)

	// Verify updates
	updated, err := store.GetByName(ctx, component.ComponentKindTool, "test-tool")
	require.NoError(t, err)
	require.NotNil(t, updated)

	assert.Equal(t, "2.0.0", updated.Version)
	assert.Equal(t, "/new/path/to/repo", updated.RepoPath)
	assert.Equal(t, "/new/path/to/bin", updated.BinPath)
	assert.Equal(t, originalCreatedAt.Unix(), updated.CreatedAt.Unix())
	assert.True(t, updated.UpdatedAt.After(originalCreatedAt))
}

// TestComponentStore_Update_NotFound tests updating non-existent component
func TestComponentStore_Update_NotFound(t *testing.T) {
	reg := setupTestRegistry(t, 22386)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Try to update non-existent component
	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "nonexistent",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	err := store.Update(ctx, comp)
	assert.ErrorIs(t, err, component.ErrComponentNotFound)
}

// TestComponentStore_Delete tests deleting a component
func TestComponentStore_Delete(t *testing.T) {
	reg := setupTestRegistry(t, 22387)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Create component
	comp := &component.Component{
		Kind:     component.ComponentKindPlugin,
		Name:     "test-plugin",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	err := store.Create(ctx, comp)
	require.NoError(t, err)

	// Delete component
	err = store.Delete(ctx, component.ComponentKindPlugin, "test-plugin")
	require.NoError(t, err)

	// Verify component is deleted
	deleted, err := store.GetByName(ctx, component.ComponentKindPlugin, "test-plugin")
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

// TestComponentStore_Delete_NotFound tests deleting non-existent component
func TestComponentStore_Delete_NotFound(t *testing.T) {
	reg := setupTestRegistry(t, 22388)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Try to delete non-existent component
	err := store.Delete(ctx, component.ComponentKindAgent, "nonexistent")
	assert.ErrorIs(t, err, component.ErrComponentNotFound)
}

// TestComponentStore_Delete_WithInstances tests atomic delete of component and instances
func TestComponentStore_Delete_WithInstances(t *testing.T) {
	reg := setupTestRegistry(t, 22389)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")
	client := reg.Client()

	// Create component
	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "k8skiller",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	err := store.Create(ctx, comp)
	require.NoError(t, err)

	// Manually create some instance keys to simulate running instances
	instancePrefix := "/test-gibson/components/agent/k8skiller/instances/"
	instance1 := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "k8skiller",
		InstanceID: "instance-1",
		Endpoint:   "localhost:50051",
	}
	instance2 := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "k8skiller",
		InstanceID: "instance-2",
		Endpoint:   "localhost:50052",
	}

	inst1Data, _ := json.Marshal(instance1)
	inst2Data, _ := json.Marshal(instance2)

	_, err = client.Put(ctx, instancePrefix+"instance-1", string(inst1Data))
	require.NoError(t, err)
	_, err = client.Put(ctx, instancePrefix+"instance-2", string(inst2Data))
	require.NoError(t, err)

	// Verify instances exist
	instances, err := store.ListInstances(ctx, component.ComponentKindAgent, "k8skiller")
	require.NoError(t, err)
	assert.Len(t, instances, 2)

	// Delete component (should delete metadata + instances atomically)
	err = store.Delete(ctx, component.ComponentKindAgent, "k8skiller")
	require.NoError(t, err)

	// Verify component is deleted
	deleted, err := store.GetByName(ctx, component.ComponentKindAgent, "k8skiller")
	require.NoError(t, err)
	assert.Nil(t, deleted)

	// Verify instances are deleted
	instances, err = store.ListInstances(ctx, component.ComponentKindAgent, "k8skiller")
	require.NoError(t, err)
	assert.Len(t, instances, 0)
}

// TestComponentStore_ListInstances tests listing running instances for a component
func TestComponentStore_ListInstances(t *testing.T) {
	reg := setupTestRegistry(t, 22390)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")
	client := reg.Client()

	// Create component
	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "bishop",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	err := store.Create(ctx, comp)
	require.NoError(t, err)

	// Manually create instance keys
	instancePrefix := "/test-gibson/components/agent/bishop/instances/"
	instance := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "bishop",
		Version:    "1.0.0",
		InstanceID: "test-instance",
		Endpoint:   "localhost:50051",
		Metadata:   map[string]string{"capabilities": "dir_enum"},
		StartedAt:  time.Now(),
	}

	instData, _ := json.Marshal(instance)
	_, err = client.Put(ctx, instancePrefix+"test-instance", string(instData))
	require.NoError(t, err)

	// List instances
	instances, err := store.ListInstances(ctx, component.ComponentKindAgent, "bishop")
	require.NoError(t, err)
	require.Len(t, instances, 1)

	assert.Equal(t, "bishop", instances[0].Name)
	assert.Equal(t, "test-instance", instances[0].InstanceID)
	assert.Equal(t, "localhost:50051", instances[0].Endpoint)
}

// TestComponentStore_ManifestSerialization tests that manifest is properly serialized/deserialized
func TestComponentStore_ManifestSerialization(t *testing.T) {
	reg := setupTestRegistry(t, 22391)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Create complex manifest
	manifest := &component.Manifest{
		Kind:        "agent",
		Name:        "complex-agent",
		Version:     "1.2.3",
		Description: "Agent with complex manifest",
		Build: &component.BuildConfig{
			Command:   "go build -o agent .",
			Artifacts: []string{"agent"},
		},
		Runtime: &component.RuntimeConfig{
			Type:       component.RuntimeTypeGo,
			Entrypoint: "./agent",
			Port:       0,
		},
		Dependencies: &component.ComponentDependencies{
			Gibson: ">=1.0.0",
			System: []string{"git", "make"},
		},
	}

	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "complex-agent",
		Version:  "1.2.3",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
		Manifest: manifest,
	}

	err := store.Create(ctx, comp)
	require.NoError(t, err)

	// Retrieve and verify manifest
	retrieved, err := store.GetByName(ctx, component.ComponentKindAgent, "complex-agent")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.NotNil(t, retrieved.Manifest)

	// Verify manifest fields
	assert.Equal(t, "complex-agent", retrieved.Manifest.Name)
	assert.Equal(t, "Agent with complex manifest", retrieved.Manifest.Description)
	require.NotNil(t, retrieved.Manifest.Build)
	assert.Equal(t, "go build -o agent .", retrieved.Manifest.Build.Command)
	assert.Equal(t, []string{"agent"}, retrieved.Manifest.Build.Artifacts)
	require.NotNil(t, retrieved.Manifest.Runtime)
	assert.Equal(t, component.RuntimeTypeGo, retrieved.Manifest.Runtime.Type)
	require.NotNil(t, retrieved.Manifest.Dependencies)
	assert.Equal(t, ">=1.0.0", retrieved.Manifest.Dependencies.Gibson)
}

// TestComponentStore_EmptyManifest tests handling of component without manifest
func TestComponentStore_EmptyManifest(t *testing.T) {
	reg := setupTestRegistry(t, 22392)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Create component without manifest
	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "no-manifest-agent",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
		Manifest: nil,
	}

	err := store.Create(ctx, comp)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := store.GetByName(ctx, component.ComponentKindAgent, "no-manifest-agent")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Nil(t, retrieved.Manifest)
}

// TestComponentStore_MultipleKindsSameName tests that same name is allowed for different kinds
func TestComponentStore_MultipleKindsSameName(t *testing.T) {
	reg := setupTestRegistry(t, 22393)
	ctx := context.Background()
	store := component.EtcdComponentStore(reg.Client(), "test-gibson")

	// Create components with same name but different kinds
	sameName := "duplicate-name"

	components := []*component.Component{
		{
			Kind:     component.ComponentKindAgent,
			Name:     sameName,
			Version:  "1.0.0",
			RepoPath: "/path/to/agent",
			BinPath:  "/path/to/bin/agent",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindTool,
			Name:     sameName,
			Version:  "1.0.0",
			RepoPath: "/path/to/tool",
			BinPath:  "/path/to/bin/tool",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindPlugin,
			Name:     sameName,
			Version:  "1.0.0",
			RepoPath: "/path/to/plugin",
			BinPath:  "/path/to/bin/plugin",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
	}

	// All should succeed (unique constraint is on kind+name combination)
	for _, comp := range components {
		err := store.Create(ctx, comp)
		require.NoError(t, err, "Create failed for %s", comp.Kind)
	}

	// Verify we can retrieve each by kind+name
	agent, err := store.GetByName(ctx, component.ComponentKindAgent, sameName)
	require.NoError(t, err)
	require.NotNil(t, agent)

	tool, err := store.GetByName(ctx, component.ComponentKindTool, sameName)
	require.NoError(t, err)
	require.NotNil(t, tool)

	plugin, err := store.GetByName(ctx, component.ComponentKindPlugin, sameName)
	require.NoError(t, err)
	require.NotNil(t, plugin)
}

// TestComponentStore_NilClient tests that nil client returns component.ErrStoreUnavailable
func TestComponentStore_NilClient(t *testing.T) {
	ctx := context.Background()
	store := component.EtcdComponentStore(nil, "test-gibson")

	comp := &component.Component{
		Kind:    component.ComponentKindAgent,
		Name:    "test",
		Version: "1.0.0",
	}

	// All operations should return component.ErrStoreUnavailable
	err := store.Create(ctx, comp)
	assert.ErrorIs(t, err, component.ErrStoreUnavailable)

	_, err = store.GetByName(ctx, component.ComponentKindAgent, "test")
	assert.ErrorIs(t, err, component.ErrStoreUnavailable)

	_, err = store.List(ctx, component.ComponentKindAgent)
	assert.ErrorIs(t, err, component.ErrStoreUnavailable)

	_, err = store.ListAll(ctx)
	assert.ErrorIs(t, err, component.ErrStoreUnavailable)

	err = store.Update(ctx, comp)
	assert.ErrorIs(t, err, component.ErrStoreUnavailable)

	err = store.Delete(ctx, component.ComponentKindAgent, "test")
	assert.ErrorIs(t, err, component.ErrStoreUnavailable)

	_, err = store.ListInstances(ctx, component.ComponentKindAgent, "test")
	assert.ErrorIs(t, err, component.ErrStoreUnavailable)
}

// TestComponentStore_DefaultNamespace tests that empty namespace defaults to "gibson"
func TestComponentStore_DefaultNamespace(t *testing.T) {
	reg := setupTestRegistry(t, 22394)
	ctx := context.Background()

	// Create store with empty namespace
	store := component.EtcdComponentStore(reg.Client(), "")
	client := reg.Client()

	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "default-ns-test",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
	}

	err := store.Create(ctx, comp)
	require.NoError(t, err)

	// Verify component was stored under /gibson/ prefix
	resp, err := client.Get(ctx, "/gibson/components/agent/default-ns-test")
	require.NoError(t, err)
	assert.Len(t, resp.Kvs, 1)
}
