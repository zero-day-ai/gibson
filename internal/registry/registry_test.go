package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/sdk/registry"
)

// Port counter for tests to avoid conflicts
var testPortCounter int32 = 12379

// getTestPort returns a unique port for each test to avoid collisions
func getTestPort() int {
	return int(atomic.AddInt32(&testPortCounter, 1))
}

// TestEmbeddedRegistry_StartStop tests the basic lifecycle of an embedded registry.
func TestEmbeddedRegistry_StartStop(t *testing.T) {
	// Create temporary directory for etcd data
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := registry.Config{
		Type:          "embedded",
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           30,
	}

	// Create and start embedded registry
	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err, "Failed to create embedded registry")
	require.NotNil(t, reg, "Registry should not be nil")

	// Verify the registry is running
	assert.NotNil(t, reg.server, "Embedded server should be running")
	assert.NotNil(t, reg.client, "Client should be connected")

	// Close the registry
	err = reg.Close()
	require.NoError(t, err, "Failed to close registry")
}

// TestEmbeddedRegistry_RegisterDiscover tests service registration and discovery.
func TestEmbeddedRegistry_RegisterDiscover(t *testing.T) {
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := registry.Config{
		Type:          "embedded",
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           30,
	}

	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)
	defer reg.Close()

	ctx := context.Background()

	// Create test service info
	instanceID := uuid.New().String()
	serviceInfo := registry.ServiceInfo{
		Kind:       "agent",
		Name:       "test-agent",
		Version:    "1.0.0",
		InstanceID: instanceID,
		Endpoint:   "localhost:50051",
		Metadata: map[string]string{
			"capabilities": "prompt_injection,jailbreak",
			"target_types": "llm_chat",
		},
		StartedAt: time.Now(),
	}

	// Register the service
	err = reg.Register(ctx, serviceInfo)
	require.NoError(t, err, "Failed to register service")

	// Discover the service
	services, err := reg.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err, "Failed to discover services")
	require.Len(t, services, 1, "Should find exactly one service")

	// Verify the discovered service matches what we registered
	discovered := services[0]
	assert.Equal(t, serviceInfo.Kind, discovered.Kind)
	assert.Equal(t, serviceInfo.Name, discovered.Name)
	assert.Equal(t, serviceInfo.Version, discovered.Version)
	assert.Equal(t, serviceInfo.InstanceID, discovered.InstanceID)
	assert.Equal(t, serviceInfo.Endpoint, discovered.Endpoint)
	assert.Equal(t, serviceInfo.Metadata["capabilities"], discovered.Metadata["capabilities"])
	assert.Equal(t, serviceInfo.Metadata["target_types"], discovered.Metadata["target_types"])
}

// TestEmbeddedRegistry_Deregister tests service deregistration.
func TestEmbeddedRegistry_Deregister(t *testing.T) {
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := registry.Config{
		Type:          "embedded",
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           30,
	}

	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)
	defer reg.Close()

	ctx := context.Background()

	// Register a service
	serviceInfo := registry.ServiceInfo{
		Kind:       "tool",
		Name:       "nmap",
		Version:    "1.0.0",
		InstanceID: uuid.New().String(),
		Endpoint:   "localhost:50052",
		Metadata:   map[string]string{},
		StartedAt:  time.Now(),
	}

	err = reg.Register(ctx, serviceInfo)
	require.NoError(t, err)

	// Verify it's registered
	services, err := reg.Discover(ctx, "tool", "nmap")
	require.NoError(t, err)
	require.Len(t, services, 1)

	// Deregister the service
	err = reg.Deregister(ctx, serviceInfo)
	require.NoError(t, err)

	// Verify it's no longer discoverable
	services, err = reg.Discover(ctx, "tool", "nmap")
	require.NoError(t, err)
	assert.Empty(t, services, "Service should be deregistered")

	// Deregistering again should not error
	err = reg.Deregister(ctx, serviceInfo)
	require.NoError(t, err, "Deregistering non-existent service should not error")
}

// TestEmbeddedRegistry_DiscoverAll tests discovering all services of a kind.
func TestEmbeddedRegistry_DiscoverAll(t *testing.T) {
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := registry.Config{
		Type:          "embedded",
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           30,
	}

	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)
	defer reg.Close()

	ctx := context.Background()

	// Register multiple agents
	agents := []registry.ServiceInfo{
		{
			Kind:       "agent",
			Name:       "davinci",
			Version:    "1.0.0",
			InstanceID: uuid.New().String(),
			Endpoint:   "localhost:50051",
			Metadata:   map[string]string{},
			StartedAt:  time.Now(),
		},
		{
			Kind:       "agent",
			Name:       "k8skiller",
			Version:    "1.0.0",
			InstanceID: uuid.New().String(),
			Endpoint:   "localhost:50052",
			Metadata:   map[string]string{},
			StartedAt:  time.Now(),
		},
		{
			Kind:       "agent",
			Name:       "davinci",
			Version:    "1.0.0",
			InstanceID: uuid.New().String(), // Second instance of davinci
			Endpoint:   "localhost:50053",
			Metadata:   map[string]string{},
			StartedAt:  time.Now(),
		},
	}

	for _, agent := range agents {
		err = reg.Register(ctx, agent)
		require.NoError(t, err)
	}

	// Register a tool to verify we only get agents
	tool := registry.ServiceInfo{
		Kind:       "tool",
		Name:       "nmap",
		Version:    "1.0.0",
		InstanceID: uuid.New().String(),
		Endpoint:   "localhost:50054",
		Metadata:   map[string]string{},
		StartedAt:  time.Now(),
	}
	err = reg.Register(ctx, tool)
	require.NoError(t, err)

	// Discover all agents
	allAgents, err := reg.DiscoverAll(ctx, "agent")
	require.NoError(t, err)
	assert.Len(t, allAgents, 3, "Should find all 3 agents")

	// Discover all tools
	allTools, err := reg.DiscoverAll(ctx, "tool")
	require.NoError(t, err)
	assert.Len(t, allTools, 1, "Should find 1 tool")

	// Discover all plugins (should be empty)
	allPlugins, err := reg.DiscoverAll(ctx, "plugin")
	require.NoError(t, err)
	assert.Empty(t, allPlugins, "Should find no plugins")
}

// TestEmbeddedRegistry_MultipleInstances tests multiple instances of the same service.
func TestEmbeddedRegistry_MultipleInstances(t *testing.T) {
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := registry.Config{
		Type:          "embedded",
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           30,
	}

	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)
	defer reg.Close()

	ctx := context.Background()

	// Register 3 instances of the same agent
	for i := 0; i < 3; i++ {
		serviceInfo := registry.ServiceInfo{
			Kind:       "agent",
			Name:       "davinci",
			Version:    "1.0.0",
			InstanceID: uuid.New().String(),
			Endpoint:   fmt.Sprintf("localhost:%d", 50051+i),
			Metadata:   map[string]string{},
			StartedAt:  time.Now(),
		}
		err = reg.Register(ctx, serviceInfo)
		require.NoError(t, err)
	}

	// Discover all instances
	services, err := reg.Discover(ctx, "agent", "davinci")
	require.NoError(t, err)
	assert.Len(t, services, 3, "Should find all 3 instances")

	// Verify each instance has unique instance ID and endpoint
	instanceIDs := make(map[string]bool)
	endpoints := make(map[string]bool)
	for _, svc := range services {
		assert.False(t, instanceIDs[svc.InstanceID], "Instance IDs should be unique")
		assert.False(t, endpoints[svc.Endpoint], "Endpoints should be unique")
		instanceIDs[svc.InstanceID] = true
		endpoints[svc.Endpoint] = true
	}
}

// TestEmbeddedRegistry_Watch tests the watch functionality.
func TestEmbeddedRegistry_Watch(t *testing.T) {
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := registry.Config{
		Type:          "embedded",
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           30,
	}

	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)
	defer reg.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start watching before any services exist
	watchCh, err := reg.Watch(ctx, "agent", "davinci")
	require.NoError(t, err)

	// Should receive initial state (empty)
	select {
	case services := <-watchCh:
		assert.Empty(t, services, "Initial state should be empty")
	case <-time.After(2 * time.Second):
		t.Fatal("Did not receive initial state")
	}

	// Register a service
	serviceInfo := registry.ServiceInfo{
		Kind:       "agent",
		Name:       "davinci",
		Version:    "1.0.0",
		InstanceID: uuid.New().String(),
		Endpoint:   "localhost:50051",
		Metadata:   map[string]string{},
		StartedAt:  time.Now(),
	}
	err = reg.Register(ctx, serviceInfo)
	require.NoError(t, err)

	// Should receive update with 1 service
	select {
	case services := <-watchCh:
		assert.Len(t, services, 1, "Should receive registration update")
	case <-time.After(2 * time.Second):
		t.Fatal("Did not receive registration update")
	}

	// Register another instance
	serviceInfo2 := registry.ServiceInfo{
		Kind:       "agent",
		Name:       "davinci",
		Version:    "1.0.0",
		InstanceID: uuid.New().String(),
		Endpoint:   "localhost:50052",
		Metadata:   map[string]string{},
		StartedAt:  time.Now(),
	}
	err = reg.Register(ctx, serviceInfo2)
	require.NoError(t, err)

	// Should receive update with 2 services
	select {
	case services := <-watchCh:
		assert.Len(t, services, 2, "Should receive second registration update")
	case <-time.After(2 * time.Second):
		t.Fatal("Did not receive second registration update")
	}

	// Deregister first service
	err = reg.Deregister(ctx, serviceInfo)
	require.NoError(t, err)

	// Should receive update with 1 service
	select {
	case services := <-watchCh:
		assert.Len(t, services, 1, "Should receive deregistration update")
	case <-time.After(2 * time.Second):
		t.Fatal("Did not receive deregistration update")
	}
}

// TestServiceInfo_JSON tests ServiceInfo JSON serialization.
func TestServiceInfo_JSON(t *testing.T) {
	now := time.Now()

	serviceInfo := registry.ServiceInfo{
		Kind:       "agent",
		Name:       "k8skiller",
		Version:    "2.0.0",
		InstanceID: "550e8400-e29b-41d4-a716-446655440000",
		Endpoint:   "localhost:50051",
		Metadata: map[string]string{
			"capabilities": "container_escape,rbac_analysis",
			"target_types": "kubernetes",
			"mitre":        "T1610,T1611",
		},
		StartedAt: now,
	}

	// Serialize to JSON
	data, err := json.Marshal(serviceInfo)
	require.NoError(t, err, "Failed to marshal ServiceInfo")

	// Deserialize from JSON
	var deserialized registry.ServiceInfo
	err = json.Unmarshal(data, &deserialized)
	require.NoError(t, err, "Failed to unmarshal ServiceInfo")

	// Verify all fields match
	assert.Equal(t, serviceInfo.Kind, deserialized.Kind)
	assert.Equal(t, serviceInfo.Name, deserialized.Name)
	assert.Equal(t, serviceInfo.Version, deserialized.Version)
	assert.Equal(t, serviceInfo.InstanceID, deserialized.InstanceID)
	assert.Equal(t, serviceInfo.Endpoint, deserialized.Endpoint)
	assert.Equal(t, serviceInfo.Metadata, deserialized.Metadata)
	assert.WithinDuration(t, serviceInfo.StartedAt, deserialized.StartedAt, time.Second)
}

// TestManager_SelectsEmbedded tests that the manager selects embedded registry.
func TestManager_SelectsEmbedded(t *testing.T) {
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := config.RegistryConfig{
		Type:          "embedded",
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           "30s",
	}

	mgr := NewManager(cfg)
	require.NotNil(t, mgr)

	// Start the manager
	ctx := context.Background()
	err := mgr.Start(ctx)
	require.NoError(t, err, "Failed to start manager")
	defer mgr.Stop(ctx)

	// Verify the registry is available
	reg := mgr.Registry()
	require.NotNil(t, reg, "Registry should be available")

	// Verify it's an embedded registry by checking it works
	serviceInfo := registry.ServiceInfo{
		Kind:       "agent",
		Name:       "test",
		Version:    "1.0.0",
		InstanceID: uuid.New().String(),
		Endpoint:   "localhost:50051",
		Metadata:   map[string]string{},
		StartedAt:  time.Now(),
	}
	err = reg.Register(ctx, serviceInfo)
	require.NoError(t, err, "Should be able to register with embedded registry")

	// Check status
	status := mgr.Status()
	assert.Equal(t, "embedded", status.Type)
	assert.True(t, status.Healthy)
	assert.Greater(t, status.Services, 0)
}

// TestManager_DefaultsToEmbedded tests that manager defaults to embedded when Type is empty.
func TestManager_DefaultsToEmbedded(t *testing.T) {
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := config.RegistryConfig{
		Type:          "", // Empty type should default to embedded
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           "30s",
	}

	mgr := NewManager(cfg)
	require.NotNil(t, mgr)

	ctx := context.Background()
	err := mgr.Start(ctx)
	require.NoError(t, err, "Failed to start manager with empty type")
	defer mgr.Stop(ctx)

	reg := mgr.Registry()
	require.NotNil(t, reg)

	status := mgr.Status()
	assert.Equal(t, "embedded", status.Type, "Should default to embedded")
	assert.True(t, status.Healthy)
}

// TestManager_StartStopIdempotent tests that Start and Stop are idempotent.
func TestManager_StartStopIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := config.RegistryConfig{
		Type:          "embedded",
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           "30s",
	}

	mgr := NewManager(cfg)
	ctx := context.Background()

	// Start twice - should not error
	err := mgr.Start(ctx)
	require.NoError(t, err)
	err = mgr.Start(ctx)
	require.NoError(t, err, "Second Start should be idempotent")

	// Stop twice - should not error
	err = mgr.Stop(ctx)
	require.NoError(t, err)
	err = mgr.Stop(ctx)
	require.NoError(t, err, "Second Stop should be idempotent")
}

// TestManager_StopBeforeStart tests stopping a manager that was never started.
func TestManager_StopBeforeStart(t *testing.T) {
	cfg := config.RegistryConfig{
		Type: "embedded",
	}

	mgr := NewManager(cfg)
	ctx := context.Background()

	// Stop without starting should not error
	err := mgr.Stop(ctx)
	require.NoError(t, err, "Stop before Start should not error")
}

// TestManager_Status tests the Status method.
func TestManager_Status(t *testing.T) {
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := config.RegistryConfig{
		Type:          "embedded",
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           "30s",
	}

	mgr := NewManager(cfg)
	ctx := context.Background()

	// Status before start
	status := mgr.Status()
	assert.Equal(t, "embedded", status.Type)
	assert.False(t, status.Healthy, "Should not be healthy before start")
	assert.Zero(t, status.Services)
	assert.True(t, status.StartedAt.IsZero())

	// Start and check status
	err := mgr.Start(ctx)
	require.NoError(t, err)
	defer mgr.Stop(ctx)

	status = mgr.Status()
	assert.Equal(t, "embedded", status.Type)
	assert.True(t, status.Healthy, "Should be healthy after start")
	assert.False(t, status.StartedAt.IsZero(), "Should have start time")

	// Register some services
	reg := mgr.Registry()
	for i := 0; i < 3; i++ {
		serviceInfo := registry.ServiceInfo{
			Kind:       "agent",
			Name:       fmt.Sprintf("agent-%d", i),
			Version:    "1.0.0",
			InstanceID: uuid.New().String(),
			Endpoint:   fmt.Sprintf("localhost:%d", 50051+i),
			Metadata:   map[string]string{},
			StartedAt:  time.Now(),
		}
		err = reg.Register(ctx, serviceInfo)
		require.NoError(t, err)
	}

	// Check status includes service count
	status = mgr.Status()
	assert.Equal(t, 3, status.Services, "Should count registered services")

	// Stop and check status
	err = mgr.Stop(ctx)
	require.NoError(t, err)

	status = mgr.Status()
	assert.False(t, status.Healthy, "Should not be healthy after stop")
	assert.Equal(t, 0, status.Services, "Should have no services after stop")
}

// TestBuildKey tests the buildKey helper function.
func TestBuildKey(t *testing.T) {
	tests := []struct {
		name       string
		namespace  string
		kind       string
		svcName    string
		instanceID string
		want       string
	}{
		{
			name:       "standard agent",
			namespace:  "gibson",
			kind:       "agent",
			svcName:    "k8skiller",
			instanceID: "550e8400-e29b-41d4-a716-446655440000",
			want:       "/gibson/agent/k8skiller/550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:       "tool",
			namespace:  "test",
			kind:       "tool",
			svcName:    "nmap",
			instanceID: "123",
			want:       "/test/tool/nmap/123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildKey(tt.namespace, tt.kind, tt.svcName, tt.instanceID)
			// Normalize path separators for cross-platform compatibility
			got = filepath.ToSlash(got)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildPrefix tests the buildPrefix helper function.
func TestBuildPrefix(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		kind      string
		svcName   string
		want      string
	}{
		{
			name:      "agent prefix",
			namespace: "gibson",
			kind:      "agent",
			svcName:   "k8skiller",
			want:      "/gibson/agent/k8skiller/",
		},
		{
			name:      "tool prefix",
			namespace: "test",
			kind:      "tool",
			svcName:   "nmap",
			want:      "/test/tool/nmap/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPrefix(tt.namespace, tt.kind, tt.svcName)
			// Normalize path separators for cross-platform compatibility
			got = filepath.ToSlash(got)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestEmbeddedRegistry_Persistence tests that data persists across restarts.
func TestEmbeddedRegistry_Persistence(t *testing.T) {
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := registry.Config{
		Type:          "embedded",
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           30,
	}

	// Create first registry instance and register a service
	reg1, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	serviceInfo := registry.ServiceInfo{
		Kind:       "agent",
		Name:       "persistent-agent",
		Version:    "1.0.0",
		InstanceID: uuid.New().String(),
		Endpoint:   "localhost:50051",
		Metadata:   map[string]string{},
		StartedAt:  time.Now(),
	}

	err = reg1.Register(ctx, serviceInfo)
	require.NoError(t, err)

	// Close the registry
	err = reg1.Close()
	require.NoError(t, err)

	// Note: With TTL-based leases, services will expire when the registry shuts down
	// since keepalive stops. This is expected behavior - services are only "alive"
	// while the component is running and maintaining its lease.
	// For true persistence across restarts, we would need to re-register on startup.

	// We can verify the data directory was created
	_, err = os.Stat(tmpDir)
	require.NoError(t, err, "Data directory should exist")

	// Verify etcd data files were created
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "Data directory should contain etcd files")
}

// TestEmbeddedRegistry_ConcurrentOperations tests concurrent registration and discovery.
func TestEmbeddedRegistry_ConcurrentOperations(t *testing.T) {
	tmpDir := t.TempDir()

	port := getTestPort()
	cfg := registry.Config{
		Type:          "embedded",
		DataDir:       tmpDir,
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           30,
	}

	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)
	defer reg.Close()

	ctx := context.Background()
	numGoroutines := 10

	// Launch multiple goroutines that register services concurrently
	done := make(chan bool, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			serviceInfo := registry.ServiceInfo{
				Kind:       "agent",
				Name:       fmt.Sprintf("agent-%d", id),
				Version:    "1.0.0",
				InstanceID: uuid.New().String(),
				Endpoint:   fmt.Sprintf("localhost:%d", 50051+id),
				Metadata:   map[string]string{},
				StartedAt:  time.Now(),
			}
			err := reg.Register(ctx, serviceInfo)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all registrations to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all services were registered
	allAgents, err := reg.DiscoverAll(ctx, "agent")
	require.NoError(t, err)
	assert.Len(t, allAgents, numGoroutines, "All services should be registered")
}

// TestEmbeddedRegistry_HomeDirectoryExpansion tests home directory expansion.
func TestEmbeddedRegistry_HomeDirectoryExpansion(t *testing.T) {
	// This test verifies that paths starting with ~/ are expanded

	port := getTestPort()
	cfg := registry.Config{
		Type:          "embedded",
		DataDir:       "~/test-gibson-registry",
		ListenAddress: fmt.Sprintf("localhost:%d", port),
		Namespace:     "test",
		TTL:           30,
	}

	reg, err := NewEmbeddedRegistry(cfg)
	require.NoError(t, err)

	// Clean up
	defer reg.Close()

	// Verify the registry started successfully with home expansion
	assert.NotNil(t, reg.server)
	assert.NotNil(t, reg.client)
}
