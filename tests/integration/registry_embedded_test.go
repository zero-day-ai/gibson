//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/registry"
	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// TestEmbeddedRegistry_FullFlow tests the complete embedded registry lifecycle:
// 1. Start Gibson with embedded registry
// 2. Start a test agent that registers itself
// 3. Verify agent appears in registry
// 4. Verify discovery and status work
// 5. Stop agent
// 6. Verify agent removed from registry
func TestEmbeddedRegistry_FullFlow(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Setup embedded registry configuration
	port := getAvailablePort(t)
	registryAddr := fmt.Sprintf("localhost:%d", port)

	cfg := config.RegistryConfig{
		Type:          "embedded",
		DataDir:       filepath.Join(tmpDir, "etcd-data"),
		ListenAddress: registryAddr,
		Namespace:     "test-gibson",
		TTL:           "10s",
	}

	// Create and start registry manager
	t.Log("Starting embedded registry manager...")
	mgr := registry.NewManager(cfg)
	ctx := context.Background()

	err := mgr.Start(ctx)
	require.NoError(t, err, "Failed to start registry manager")
	defer func() {
		t.Log("Stopping registry manager...")
		_ = mgr.Stop(ctx)
	}()

	// Verify registry is healthy
	status := mgr.Status()
	assert.Equal(t, "embedded", status.Type)
	assert.True(t, status.Healthy, "Registry should be healthy")
	t.Logf("Registry started: %s", status.Endpoint)

	// Get the registry instance
	reg := mgr.Registry()
	require.NotNil(t, reg, "Registry should be available")

	// Verify no agents registered initially
	agents, err := reg.DiscoverAll(ctx, "agent")
	require.NoError(t, err)
	assert.Empty(t, agents, "Should have no agents initially")

	// Start a test agent that auto-registers
	t.Log("Starting test agent...")
	agentPort := getAvailablePort(t)
	agentStop, agentInfo := startTestAgent(t, ctx, registryAddr, agentPort)
	defer agentStop()

	// Wait for agent registration with timeout
	t.Log("Waiting for agent to register...")
	err = waitForCondition(30*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		return err == nil && len(instances) > 0
	})
	require.NoError(t, err, "Agent should register within timeout")

	// Verify agent appears in registry
	t.Log("Verifying agent registration...")
	instances, err := reg.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err, "Failed to discover agent")
	require.Len(t, instances, 1, "Should find exactly one agent instance")

	// Verify instance details
	discovered := instances[0]
	assert.Equal(t, "agent", discovered.Kind)
	assert.Equal(t, "test-agent", discovered.Name)
	assert.Equal(t, "1.0.0", discovered.Version)
	assert.Equal(t, agentInfo.InstanceID, discovered.InstanceID)
	assert.Equal(t, agentInfo.Endpoint, discovered.Endpoint)
	assert.NotEmpty(t, discovered.Metadata)
	t.Logf("Agent registered: %s (instance: %s, endpoint: %s)",
		discovered.Name, discovered.InstanceID, discovered.Endpoint)

	// Verify DiscoverAll returns the agent
	t.Log("Verifying DiscoverAll...")
	allAgents, err := reg.DiscoverAll(ctx, "agent")
	require.NoError(t, err)
	assert.Len(t, allAgents, 1, "DiscoverAll should return 1 agent")

	// Verify registry status shows service count
	status = mgr.Status()
	assert.Greater(t, status.Services, 0, "Registry should show registered services")
	t.Logf("Registry status: %d services registered", status.Services)

	// Test Watch functionality
	t.Log("Testing Watch functionality...")
	watchCtx, watchCancel := context.WithTimeout(ctx, 5*time.Second)
	defer watchCancel()

	watchCh, err := reg.Watch(watchCtx, "agent", "test-agent")
	require.NoError(t, err, "Failed to start watch")

	// Should receive initial state with 1 agent
	select {
	case services := <-watchCh:
		assert.Len(t, services, 1, "Watch should return current state")
	case <-time.After(2 * time.Second):
		t.Fatal("Did not receive initial watch state")
	}

	// Stop the agent
	t.Log("Stopping test agent...")
	agentStop()

	// Wait for deregistration
	t.Log("Waiting for agent to deregister...")
	err = waitForCondition(15*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		return err == nil && len(instances) == 0
	})
	require.NoError(t, err, "Agent should deregister within timeout")

	// Verify agent removed from registry
	instances, err = reg.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err)
	assert.Empty(t, instances, "Agent should be deregistered")
	t.Log("Agent successfully deregistered")

	// Verify DiscoverAll returns empty
	allAgents, err = reg.DiscoverAll(ctx, "agent")
	require.NoError(t, err)
	assert.Empty(t, allAgents, "Should have no agents after stop")
}

// TestEmbeddedRegistry_MultipleInstances tests multiple instances of the same agent.
func TestEmbeddedRegistry_MultipleInstances(t *testing.T) {
	tmpDir := t.TempDir()

	port := getAvailablePort(t)
	registryAddr := fmt.Sprintf("localhost:%d", port)

	cfg := config.RegistryConfig{
		Type:          "embedded",
		DataDir:       filepath.Join(tmpDir, "etcd-data"),
		ListenAddress: registryAddr,
		Namespace:     "test-gibson",
		TTL:           "10s",
	}

	t.Log("Starting embedded registry manager...")
	mgr := registry.NewManager(cfg)
	ctx := context.Background()

	err := mgr.Start(ctx)
	require.NoError(t, err)
	defer mgr.Stop(ctx)

	reg := mgr.Registry()
	require.NotNil(t, reg)

	// Start 3 instances of the same agent
	t.Log("Starting 3 agent instances...")
	var stopFuncs []func()
	var instanceIDs []string

	for i := 0; i < 3; i++ {
		agentPort := getAvailablePort(t)
		stopFunc, info := startTestAgent(t, ctx, registryAddr, agentPort)
		stopFuncs = append(stopFuncs, stopFunc)
		instanceIDs = append(instanceIDs, info.InstanceID)
	}

	// Cleanup all instances
	defer func() {
		for _, stop := range stopFuncs {
			stop()
		}
	}()

	// Wait for all agents to register
	t.Log("Waiting for all agents to register...")
	err = waitForCondition(30*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		return err == nil && len(instances) == 3
	})
	require.NoError(t, err, "All 3 agents should register")

	// Verify all 3 instances appear
	instances, err := reg.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err)
	assert.Len(t, instances, 3, "Should find 3 instances")

	// Verify each has unique instance ID and endpoint
	foundIDs := make(map[string]bool)
	foundEndpoints := make(map[string]bool)
	for _, instance := range instances {
		assert.False(t, foundIDs[instance.InstanceID], "Instance IDs should be unique")
		assert.False(t, foundEndpoints[instance.Endpoint], "Endpoints should be unique")
		foundIDs[instance.InstanceID] = true
		foundEndpoints[instance.Endpoint] = true
	}
	t.Logf("All 3 instances registered with unique IDs and endpoints")

	// Stop one instance
	t.Log("Stopping one instance...")
	stopFuncs[0]()

	// Wait for it to deregister
	err = waitForCondition(15*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		return err == nil && len(instances) == 2
	})
	require.NoError(t, err, "Should have 2 instances after stopping 1")

	// Verify 2 instances remain
	instances, err = reg.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err)
	assert.Len(t, instances, 2, "Should have 2 instances remaining")
	t.Log("Successfully tested multi-instance lifecycle")
}

// TestEmbeddedRegistry_Restart tests that registry data persists across restarts.
func TestEmbeddedRegistry_Restart(t *testing.T) {
	tmpDir := t.TempDir()

	port := getAvailablePort(t)
	registryAddr := fmt.Sprintf("localhost:%d", port)

	cfg := config.RegistryConfig{
		Type:          "embedded",
		DataDir:       filepath.Join(tmpDir, "etcd-data"),
		ListenAddress: registryAddr,
		Namespace:     "test-gibson",
		TTL:           "10s",
	}

	ctx := context.Background()

	// Start first registry instance
	t.Log("Starting first registry instance...")
	mgr1 := registry.NewManager(cfg)
	err := mgr1.Start(ctx)
	require.NoError(t, err)

	// Verify data directory created
	_, err = os.Stat(cfg.DataDir)
	require.NoError(t, err, "Data directory should be created")

	// Stop first instance
	t.Log("Stopping first registry instance...")
	err = mgr1.Stop(ctx)
	require.NoError(t, err)

	// Wait a bit to ensure clean shutdown
	time.Sleep(time.Second)

	// Start second registry instance with same data dir
	t.Log("Starting second registry instance...")
	mgr2 := registry.NewManager(cfg)
	err = mgr2.Start(ctx)
	require.NoError(t, err)
	defer mgr2.Stop(ctx)

	// Verify it starts successfully (persistence is verified by successful restart)
	status := mgr2.Status()
	assert.True(t, status.Healthy, "Registry should be healthy after restart")
	t.Log("Registry successfully restarted with existing data directory")

	// Note: With TTL-based leases, services expire when registry shuts down
	// This test verifies the data directory and etcd state persistence,
	// not service registration persistence (which is by design)
}

// startTestAgent starts a test agent that auto-registers with the registry.
// Returns a stop function and the service info.
func startTestAgent(t *testing.T, ctx context.Context, registryEndpoint string, port int) (func(), sdkregistry.ServiceInfo) {
	t.Helper()

	// Create registry client
	regClient, err := sdkregistry.NewClient(sdkregistry.Config{
		Endpoints: []string{registryEndpoint},
		Namespace: "test-gibson",
		TTL:       10,
	})
	require.NoError(t, err, "Failed to create registry client")

	// Create service info
	instanceID := uuid.New().String()
	endpoint := fmt.Sprintf("localhost:%d", port)

	serviceInfo := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "test-agent",
		Version:    "1.0.0",
		InstanceID: instanceID,
		Endpoint:   endpoint,
		Metadata: map[string]string{
			"capabilities":    "prompt_injection",
			"target_types":    "llm_chat",
			"technique_types": "prompt_injection",
		},
		StartedAt: time.Now(),
	}

	// Create a dummy listener to hold the port
	listener, err := net.Listen("tcp", endpoint)
	require.NoError(t, err, "Failed to create listener")

	// Register with registry
	err = regClient.Register(ctx, serviceInfo)
	require.NoError(t, err, "Failed to register agent")

	// Stop function
	stopFunc := func() {
		// Deregister from registry
		_ = regClient.Deregister(ctx, serviceInfo)
		_ = regClient.Close()

		// Close listener
		_ = listener.Close()
	}

	return stopFunc, serviceInfo
}

// Helper functions

// getAvailablePort finds an available port for testing.
func getAvailablePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err, "Failed to find available port")
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port
}

// waitForCondition polls a condition until it's true or timeout.
func waitForCondition(timeout time.Duration, interval time.Duration, condition func() bool) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if condition() {
			return nil
		}
		time.Sleep(interval)
	}

	return fmt.Errorf("condition not met within timeout")
}
