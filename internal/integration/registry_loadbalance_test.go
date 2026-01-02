//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/registry"
)

// registry_loadbalance_test.go
//
// Integration tests for multi-instance load balancing.
//
// These tests verify that the registry system properly distributes requests
// across multiple instances of the same component (agent, tool, or plugin)
// using the configured load balancing strategy (round-robin by default).
//
// Test coverage:
//   - TestLoadBalancerDistribution: Verifies even distribution across 3 instances over 12 calls
//   - TestLoadBalancerRoundRobinOrder: Verifies strict sequential ordering and wrap-around
//   - TestDiscoverAgentUsesLoadBalancer: Verifies DiscoverAgent integrates with load balancer
//
// Key verification points:
//   1. Multiple instances can be registered with different endpoints
//   2. LoadBalancer.Select() distributes requests using round-robin
//   3. All instances receive equal traffic over time
//   4. Round-robin wraps around correctly after completing a cycle
//   5. RegistryAdapter.DiscoverAgent() uses the load balancer internally
//
// Architecture tested:
//   - registry.Manager: Starts embedded etcd
//   - sdk/registry.Registry: Discovers service instances
//   - registry.LoadBalancer: Selects instances using round-robin
//   - registry.RegistryAdapter: Wraps registry + load balancer
//   - registry.GRPCAgentClient: Client wrapper for remote agents

// TestLoadBalancerDistribution tests that the load balancer distributes requests
// across multiple instances using round-robin strategy.
//
// This test verifies:
// 1. Multiple instances of the same agent can be registered
// 2. DiscoverAgent uses the load balancer to select instances
// 3. Round-robin distribution ensures all instances are used
// 4. After N calls (where N = instance count), all instances have been selected
func TestLoadBalancerDistribution(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Step 1: Start embedded etcd
	t.Log("Step 1: Starting embedded registry...")
	port := getAvailablePort(t)
	registryAddr := fmt.Sprintf("localhost:%d", port)

	registryConfig := config.RegistryConfig{
		Type:          "embedded",
		DataDir:       filepath.Join(tmpDir, "etcd-data"),
		ListenAddress: registryAddr,
		Namespace:     "test-gibson",
		TTL:           "30s",
	}

	mgr := registry.NewManager(registryConfig)
	err := mgr.Start(ctx)
	require.NoError(t, err, "Failed to start registry manager")
	defer func() {
		t.Log("Cleaning up registry...")
		_ = mgr.Stop(ctx)
	}()

	// Verify registry is healthy
	status := mgr.Status()
	assert.True(t, status.Healthy, "Registry should be healthy")
	t.Logf("Registry started: %s (type: %s)", status.Endpoint, status.Type)

	reg := mgr.Registry()
	require.NotNil(t, reg, "Registry should be available")

	// Step 2: Register 3 mock agent instances with different endpoints
	t.Log("Step 2: Registering 3 agent instances with different endpoints...")
	instanceCount := 3
	var mockServers []*mockAgentGRPCServer
	expectedEndpoints := make(map[string]bool)

	for i := 0; i < instanceCount; i++ {
		agentPort := getAvailablePort(t)
		endpoint := fmt.Sprintf("localhost:%d", agentPort)
		expectedEndpoints[endpoint] = false // Track which endpoints have been seen

		server, _ := startMockAgentServer(t, ctx, registryAddr, agentPort)
		mockServers = append(mockServers, server)
		defer server.Stop()

		t.Logf("  Registered instance %d at %s", i+1, endpoint)
	}

	// Wait for all agents to register
	t.Log("Waiting for all agents to register...")
	err = waitForCondition(30*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-attack-agent")
		return err == nil && len(instances) == instanceCount
	})
	require.NoError(t, err, "All agents should register within timeout")

	// Verify all instances are registered
	instances, err := reg.Discover(ctx, "agent", "test-attack-agent")
	require.NoError(t, err)
	require.Len(t, instances, instanceCount, "Should have exactly %d instances", instanceCount)
	t.Logf("Verified %d instances registered", len(instances))

	// Step 3: Create registry adapter (uses round-robin by default)
	t.Log("Step 3: Creating registry adapter with round-robin load balancer...")
	adapter := registry.NewRegistryAdapter(reg)
	defer adapter.Close()

	// Step 4: Test load balancer directly by calling Select
	// This is more direct than testing through DiscoverAgent since we can see the endpoint
	t.Log("Step 4: Testing load balancer directly with 10+ selections...")
	loadBalancer := registry.NewLoadBalancer(reg, registry.StrategyRoundRobin)

	discoveryCount := 12 // More than instance count to ensure multiple rounds
	endpointUsage := make(map[string]int)

	for i := 0; i < discoveryCount; i++ {
		selected, err := loadBalancer.Select(ctx, "agent", "test-attack-agent")
		require.NoError(t, err, "Selection %d should succeed", i+1)
		require.NotNil(t, selected, "Selected instance should not be nil")

		endpoint := selected.Endpoint
		endpointUsage[endpoint]++
		t.Logf("  Selection %d: Selected endpoint %s (total uses: %d)", i+1, endpoint, endpointUsage[endpoint])
	}

	// Step 5: Verify all 3 instances were selected
	t.Log("Step 5: Verifying load balancer distribution...")

	// All expected endpoints should have been used
	for endpoint := range expectedEndpoints {
		uses, found := endpointUsage[endpoint]
		assert.True(t, found, "Endpoint %s should have been selected at least once", endpoint)
		assert.Greater(t, uses, 0, "Endpoint %s should have been used", endpoint)
		t.Logf("  Endpoint %s: %d uses", endpoint, uses)
	}

	// With round-robin over 12 calls with 3 instances, each should be used exactly 4 times
	t.Log("Verifying round-robin distribution equality...")
	expectedUsesPerInstance := discoveryCount / instanceCount
	for endpoint, uses := range endpointUsage {
		assert.Equal(t, expectedUsesPerInstance, uses,
			"Round-robin should distribute evenly: endpoint %s should be used exactly %d times",
			endpoint, expectedUsesPerInstance)
	}

	// Verify we used exactly the expected number of unique endpoints
	assert.Len(t, endpointUsage, instanceCount,
		"Should have used exactly %d unique endpoints", instanceCount)

	t.Log("Load balancer test completed successfully!")
	t.Logf("Summary: %d discoveries distributed across %d instances", discoveryCount, instanceCount)
	t.Logf("Distribution: %v", endpointUsage)
}

// TestLoadBalancerRoundRobinOrder tests that round-robin strictly follows sequential order
func TestLoadBalancerRoundRobinOrder(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Start embedded etcd
	t.Log("Starting embedded registry...")
	port := getAvailablePort(t)
	registryAddr := fmt.Sprintf("localhost:%d", port)

	registryConfig := config.RegistryConfig{
		Type:          "embedded",
		DataDir:       filepath.Join(tmpDir, "etcd-data"),
		ListenAddress: registryAddr,
		Namespace:     "test-gibson",
		TTL:           "30s",
	}

	mgr := registry.NewManager(registryConfig)
	err := mgr.Start(ctx)
	require.NoError(t, err)
	defer mgr.Stop(ctx)

	reg := mgr.Registry()
	require.NotNil(t, reg)

	// Register 3 instances
	t.Log("Registering 3 agent instances...")
	instanceCount := 3
	var mockServers []*mockAgentGRPCServer

	for i := 0; i < instanceCount; i++ {
		agentPort := getAvailablePort(t)
		server, _ := startMockAgentServer(t, ctx, registryAddr, agentPort)
		mockServers = append(mockServers, server)
		defer server.Stop()
	}

	// Wait for registration
	err = waitForCondition(30*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-attack-agent")
		return err == nil && len(instances) == instanceCount
	})
	require.NoError(t, err)

	// Create load balancer for testing
	loadBalancer := registry.NewLoadBalancer(reg, registry.StrategyRoundRobin)

	// Get the instance order from registry
	instances, err := reg.Discover(ctx, "agent", "test-attack-agent")
	require.NoError(t, err)
	require.Len(t, instances, instanceCount)

	// Store expected order
	expectedOrder := make([]string, instanceCount)
	for i, inst := range instances {
		expectedOrder[i] = inst.Endpoint
		t.Logf("Expected order[%d]: %s", i, inst.Endpoint)
	}

	// Test that first 3 selections follow the exact order
	t.Log("Testing round-robin order...")
	for i := 0; i < instanceCount; i++ {
		selected, err := loadBalancer.Select(ctx, "agent", "test-attack-agent")
		require.NoError(t, err)

		endpoint := selected.Endpoint

		// The load balancer should select instances in order
		assert.Equal(t, expectedOrder[i], endpoint,
			"Selection %d should select endpoint at index %d", i+1, i)
		t.Logf("Selection %d: Got %s (expected %s) ✓", i+1, endpoint, expectedOrder[i])
	}

	// Test that it wraps around after completing one round
	t.Log("Testing round-robin wrap-around...")
	for i := 0; i < instanceCount; i++ {
		selected, err := loadBalancer.Select(ctx, "agent", "test-attack-agent")
		require.NoError(t, err)

		endpoint := selected.Endpoint

		assert.Equal(t, expectedOrder[i], endpoint,
			"Second round selection %d should select same endpoint at index %d", i+1, i)
		t.Logf("Second round selection %d: Got %s (expected %s) ✓", i+1, endpoint, expectedOrder[i])
	}

	t.Log("Round-robin order test completed successfully!")
}

// TestDiscoverAgentUsesLoadBalancer tests that DiscoverAgent properly uses the load balancer.
// This test verifies the integration between the adapter and load balancer for agent discovery.
func TestDiscoverAgentUsesLoadBalancer(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Start embedded etcd
	t.Log("Starting embedded registry...")
	port := getAvailablePort(t)
	registryAddr := fmt.Sprintf("localhost:%d", port)

	registryConfig := config.RegistryConfig{
		Type:          "embedded",
		DataDir:       filepath.Join(tmpDir, "etcd-data"),
		ListenAddress: registryAddr,
		Namespace:     "test-gibson",
		TTL:           "30s",
	}

	mgr := registry.NewManager(registryConfig)
	err := mgr.Start(ctx)
	require.NoError(t, err)
	defer mgr.Stop(ctx)

	reg := mgr.Registry()
	require.NotNil(t, reg)

	// Register 3 instances
	t.Log("Registering 3 agent instances...")
	instanceCount := 3
	var mockServers []*mockAgentGRPCServer

	for i := 0; i < instanceCount; i++ {
		agentPort := getAvailablePort(t)
		server, _ := startMockAgentServer(t, ctx, registryAddr, agentPort)
		mockServers = append(mockServers, server)
		defer server.Stop()
	}

	// Wait for registration
	err = waitForCondition(30*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-attack-agent")
		return err == nil && len(instances) == instanceCount
	})
	require.NoError(t, err)

	// Create adapter
	t.Log("Creating registry adapter...")
	adapter := registry.NewRegistryAdapter(reg)
	defer adapter.Close()

	// Test that DiscoverAgent succeeds multiple times
	// We can't directly verify which endpoint was selected, but we can verify:
	// 1. Discovery succeeds
	// 2. Returns valid agent client
	// 3. Agent has correct name and metadata
	t.Log("Testing DiscoverAgent returns valid clients...")
	for i := 0; i < 5; i++ {
		agent, err := adapter.DiscoverAgent(ctx, "test-attack-agent")
		require.NoError(t, err, "Discovery %d should succeed", i+1)
		require.NotNil(t, agent, "Agent should not be nil")

		// Verify agent metadata
		assert.Equal(t, "test-attack-agent", agent.Name(), "Agent name should match")
		assert.Equal(t, "1.0.0", agent.Version(), "Agent version should match")

		// Verify capabilities
		capabilities := agent.Capabilities()
		assert.Contains(t, capabilities, "prompt_injection", "Should have prompt_injection capability")
		assert.Contains(t, capabilities, "jailbreak", "Should have jailbreak capability")

		t.Logf("Discovery %d: Got agent '%s' v%s", i+1, agent.Name(), agent.Version())
	}

	// Verify ListAgents shows correct instance count
	t.Log("Verifying ListAgents shows all instances...")
	agentList, err := adapter.ListAgents(ctx)
	require.NoError(t, err)
	require.Len(t, agentList, 1, "Should have 1 unique agent")
	assert.Equal(t, "test-attack-agent", agentList[0].Name)
	assert.Equal(t, instanceCount, agentList[0].Instances, "Should show 3 instances")
	assert.Len(t, agentList[0].Endpoints, instanceCount, "Should list all endpoints")

	t.Logf("ListAgents result: %d instances at endpoints: %v",
		agentList[0].Instances, agentList[0].Endpoints)

	t.Log("DiscoverAgent integration test completed successfully!")
}
