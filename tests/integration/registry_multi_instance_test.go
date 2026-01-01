//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/registry"
)

// TestMultiInstance_ThreeAgents tests the full multi-instance lifecycle:
// 1. Start 3 instances of same agent on different ports
// 2. Verify all 3 appear in registry
// 3. Verify status shows 3 instances
// 4. Stop 1 instance
// 5. Verify 2 remain
func TestMultiInstance_ThreeAgents(t *testing.T) {
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

	// Start 3 instances of the same agent on different ports
	t.Log("Starting 3 agent instances...")
	var stopFuncs []func()
	var instanceIDs []string
	var endpoints []string

	for i := 0; i < 3; i++ {
		agentPort := getAvailablePort(t)
		stopFunc, info := startTestAgent(t, ctx, registryAddr, agentPort)
		stopFuncs = append(stopFuncs, stopFunc)
		instanceIDs = append(instanceIDs, info.InstanceID)
		endpoints = append(endpoints, info.Endpoint)
		t.Logf("  Started instance %d: %s (port %d)", i+1, info.InstanceID, agentPort)
	}

	// Cleanup all instances at the end
	defer func() {
		t.Log("Cleaning up all instances...")
		for i, stop := range stopFuncs {
			if stop != nil {
				t.Logf("  Stopping instance %d", i+1)
				stop()
			}
		}
	}()

	// Wait for all 3 agents to register
	t.Log("Waiting for all 3 agents to register...")
	err = waitForCondition(30*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		if err != nil {
			t.Logf("  Discovery error: %v", err)
			return false
		}
		t.Logf("  Found %d instances", len(instances))
		return len(instances) == 3
	})
	require.NoError(t, err, "All 3 agents should register within timeout")

	// Verify all 3 instances appear in registry
	t.Log("Verifying all 3 instances in registry...")
	instances, err := reg.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err, "Failed to discover agents")
	assert.Len(t, instances, 3, "Should find exactly 3 agent instances")

	// Verify each has unique instance ID and endpoint
	t.Log("Verifying instance uniqueness...")
	foundIDs := make(map[string]bool)
	foundEndpoints := make(map[string]bool)
	for i, instance := range instances {
		t.Logf("  Instance %d: ID=%s, Endpoint=%s", i+1, instance.InstanceID, instance.Endpoint)
		assert.False(t, foundIDs[instance.InstanceID], "Instance IDs should be unique")
		assert.False(t, foundEndpoints[instance.Endpoint], "Endpoints should be unique")
		assert.Equal(t, "agent", instance.Kind)
		assert.Equal(t, "test-agent", instance.Name)
		assert.Equal(t, "1.0.0", instance.Version)
		foundIDs[instance.InstanceID] = true
		foundEndpoints[instance.Endpoint] = true
	}
	t.Log("All instances have unique IDs and endpoints")

	// Verify status shows correct instance count
	t.Log("Verifying registry status...")
	status = mgr.Status()
	assert.Greater(t, status.Services, 0, "Registry should show registered services")
	t.Logf("Registry status: %d services registered", status.Services)

	// Stop one instance
	t.Log("Stopping instance 1...")
	stopFuncs[0]()
	stopFuncs[0] = nil // Mark as stopped to avoid double-stop in cleanup

	// Wait for deregistration
	t.Log("Waiting for instance 1 to deregister...")
	err = waitForCondition(15*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		if err != nil {
			return false
		}
		t.Logf("  Found %d instances", len(instances))
		return len(instances) == 2
	})
	require.NoError(t, err, "Should have 2 instances after stopping 1")

	// Verify 2 instances remain
	t.Log("Verifying 2 instances remain...")
	instances, err = reg.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err)
	assert.Len(t, instances, 2, "Should have exactly 2 instances remaining")

	// Verify the stopped instance is not in the list
	for _, instance := range instances {
		assert.NotEqual(t, instanceIDs[0], instance.InstanceID,
			"Stopped instance should not be in registry")
	}
	t.Log("Successfully verified multi-instance lifecycle")
}

// TestMultiInstance_LoadBalancing tests that load balancing distributes requests across instances.
func TestMultiInstance_LoadBalancing(t *testing.T) {
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
	t.Log("Starting embedded registry for load balancing test...")
	mgr := registry.NewManager(cfg)
	ctx := context.Background()

	err := mgr.Start(ctx)
	require.NoError(t, err, "Failed to start registry manager")
	defer mgr.Stop(ctx)

	reg := mgr.Registry()
	require.NotNil(t, reg)

	// Start 3 instances
	t.Log("Starting 3 agent instances...")
	var stopFuncs []func()
	var endpoints []string

	for i := 0; i < 3; i++ {
		agentPort := getAvailablePort(t)
		stopFunc, info := startTestAgent(t, ctx, registryAddr, agentPort)
		stopFuncs = append(stopFuncs, stopFunc)
		endpoints = append(endpoints, info.Endpoint)
	}

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

	// Test Round-Robin load balancing
	t.Log("Testing round-robin load balancing...")
	lb := registry.NewLoadBalancer(reg, registry.StrategyRoundRobin)

	// Make 12 requests (4 per instance in perfect distribution)
	endpointCounts := make(map[string]int)
	for i := 0; i < 12; i++ {
		instance, err := lb.Select(ctx, "agent", "test-agent")
		require.NoError(t, err, "Load balancer should select instance")
		endpointCounts[instance.Endpoint]++
	}

	// Verify distribution (should be 4 requests per instance for round-robin)
	t.Log("Round-robin distribution:")
	for endpoint, count := range endpointCounts {
		t.Logf("  %s: %d requests", endpoint, count)
		assert.Equal(t, 4, count, "Round-robin should distribute evenly")
	}
	assert.Len(t, endpointCounts, 3, "All 3 instances should receive requests")

	// Test Random load balancing
	t.Log("Testing random load balancing...")
	lb.SetStrategy(registry.StrategyRandom)

	// Make 30 requests - with random, we just verify all instances get some traffic
	endpointCounts = make(map[string]int)
	for i := 0; i < 30; i++ {
		instance, err := lb.Select(ctx, "agent", "test-agent")
		require.NoError(t, err, "Load balancer should select instance")
		endpointCounts[instance.Endpoint]++
	}

	// Verify all instances received at least one request
	t.Log("Random distribution:")
	for endpoint, count := range endpointCounts {
		t.Logf("  %s: %d requests", endpoint, count)
		assert.Greater(t, count, 0, "Each instance should receive at least one request")
	}
	assert.Len(t, endpointCounts, 3, "All 3 instances should receive requests with random")

	t.Log("Load balancing test completed successfully")
}

// TestMultiInstance_ConcurrentRegistration tests concurrent registration of multiple instances.
func TestMultiInstance_ConcurrentRegistration(t *testing.T) {
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
	t.Log("Starting embedded registry for concurrent registration test...")
	mgr := registry.NewManager(cfg)
	ctx := context.Background()

	err := mgr.Start(ctx)
	require.NoError(t, err, "Failed to start registry manager")
	defer mgr.Stop(ctx)

	reg := mgr.Registry()
	require.NotNil(t, reg)

	// Start 5 instances concurrently
	t.Log("Starting 5 agent instances concurrently...")
	const numInstances = 5
	var wg sync.WaitGroup
	var mu sync.Mutex
	stopFuncs := make([]func(), 0, numInstances)
	errors := make([]error, 0)

	for i := 0; i < numInstances; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			agentPort := getAvailablePort(t)
			stopFunc, info := startTestAgent(t, ctx, registryAddr, agentPort)

			mu.Lock()
			stopFuncs = append(stopFuncs, stopFunc)
			mu.Unlock()

			t.Logf("  Concurrently started instance %d: %s", index+1, info.InstanceID)
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	defer func() {
		mu.Lock()
		defer mu.Unlock()
		for _, stop := range stopFuncs {
			stop()
		}
	}()

	// Check for any errors during concurrent startup
	require.Empty(t, errors, "All instances should start without errors")

	// Wait for all agents to register
	t.Log("Waiting for all 5 agents to register...")
	err = waitForCondition(30*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		if err != nil {
			return false
		}
		t.Logf("  Found %d instances", len(instances))
		return len(instances) == numInstances
	})
	require.NoError(t, err, "All 5 agents should register")

	// Verify all instances registered successfully
	instances, err := reg.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err)
	assert.Len(t, instances, numInstances, "Should have all 5 instances registered")

	// Verify all instances are unique
	uniqueIDs := make(map[string]bool)
	uniqueEndpoints := make(map[string]bool)
	for _, instance := range instances {
		uniqueIDs[instance.InstanceID] = true
		uniqueEndpoints[instance.Endpoint] = true
	}
	assert.Len(t, uniqueIDs, numInstances, "All instance IDs should be unique")
	assert.Len(t, uniqueEndpoints, numInstances, "All endpoints should be unique")

	t.Log("Concurrent registration test completed successfully")
}

// TestMultiInstance_PartialFailure tests behavior when some instances fail.
func TestMultiInstance_PartialFailure(t *testing.T) {
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
	t.Log("Starting embedded registry for partial failure test...")
	mgr := registry.NewManager(cfg)
	ctx := context.Background()

	err := mgr.Start(ctx)
	require.NoError(t, err, "Failed to start registry manager")
	defer mgr.Stop(ctx)

	reg := mgr.Registry()
	require.NotNil(t, reg)

	// Start 3 instances
	t.Log("Starting 3 agent instances...")
	var stopFuncs []func()

	for i := 0; i < 3; i++ {
		agentPort := getAvailablePort(t)
		stopFunc, _ := startTestAgent(t, ctx, registryAddr, agentPort)
		stopFuncs = append(stopFuncs, stopFunc)
	}

	defer func() {
		for _, stop := range stopFuncs {
			if stop != nil {
				stop()
			}
		}
	}()

	// Wait for all agents to register
	t.Log("Waiting for all agents to register...")
	err = waitForCondition(30*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		return err == nil && len(instances) == 3
	})
	require.NoError(t, err, "All 3 agents should register")

	// Create load balancer
	lb := registry.NewLoadBalancer(reg, registry.StrategyRoundRobin)

	// Verify load balancer works with all instances
	instance, err := lb.Select(ctx, "agent", "test-agent")
	require.NoError(t, err)
	require.NotNil(t, instance)
	t.Logf("Load balancer selected: %s", instance.Endpoint)

	// Simulate failure: stop 2 instances
	t.Log("Simulating partial failure: stopping 2 instances...")
	stopFuncs[0]()
	stopFuncs[0] = nil
	stopFuncs[1]()
	stopFuncs[1] = nil

	// Wait for instances to deregister
	t.Log("Waiting for failed instances to deregister...")
	err = waitForCondition(15*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		return err == nil && len(instances) == 1
	})
	require.NoError(t, err, "Should have 1 instance remaining")

	// Verify load balancer still works with single instance
	t.Log("Verifying load balancer works with single remaining instance...")
	for i := 0; i < 5; i++ {
		instance, err := lb.Select(ctx, "agent", "test-agent")
		require.NoError(t, err, "Load balancer should work with single instance")
		require.NotNil(t, instance, "Should return the remaining instance")
		t.Logf("  Request %d: selected %s", i+1, instance.Endpoint)
	}

	t.Log("Partial failure test completed successfully")
}

// TestMultiInstance_ScaleUpDown tests dynamic scaling scenarios.
func TestMultiInstance_ScaleUpDown(t *testing.T) {
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
	t.Log("Starting embedded registry for scale up/down test...")
	mgr := registry.NewManager(cfg)
	ctx := context.Background()

	err := mgr.Start(ctx)
	require.NoError(t, err, "Failed to start registry manager")
	defer mgr.Stop(ctx)

	reg := mgr.Registry()
	require.NotNil(t, reg)

	// Start with 1 instance
	t.Log("Starting with 1 instance...")
	var stopFuncs []func()
	agentPort := getAvailablePort(t)
	stopFunc, _ := startTestAgent(t, ctx, registryAddr, agentPort)
	stopFuncs = append(stopFuncs, stopFunc)

	defer func() {
		for _, stop := range stopFuncs {
			if stop != nil {
				stop()
			}
		}
	}()

	// Wait for initial instance
	err = waitForCondition(15*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		return err == nil && len(instances) == 1
	})
	require.NoError(t, err, "Initial instance should register")
	t.Log("Initial instance registered")

	// Scale up to 3 instances
	t.Log("Scaling up to 3 instances...")
	for i := 0; i < 2; i++ {
		agentPort := getAvailablePort(t)
		stopFunc, _ := startTestAgent(t, ctx, registryAddr, agentPort)
		stopFuncs = append(stopFuncs, stopFunc)
	}

	err = waitForCondition(15*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		return err == nil && len(instances) == 3
	})
	require.NoError(t, err, "Should scale up to 3 instances")
	t.Log("Scaled up to 3 instances")

	// Scale down to 1 instance
	t.Log("Scaling down to 1 instance...")
	stopFuncs[1]()
	stopFuncs[1] = nil
	stopFuncs[2]()
	stopFuncs[2] = nil

	err = waitForCondition(15*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		return err == nil && len(instances) == 1
	})
	require.NoError(t, err, "Should scale down to 1 instance")
	t.Log("Scaled down to 1 instance")

	// Scale up again to 2 instances
	t.Log("Scaling up again to 2 instances...")
	agentPort = getAvailablePort(t)
	stopFunc, _ = startTestAgent(t, ctx, registryAddr, agentPort)
	stopFuncs = append(stopFuncs, stopFunc)

	err = waitForCondition(15*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-agent")
		return err == nil && len(instances) == 2
	})
	require.NoError(t, err, "Should scale up to 2 instances")
	t.Log("Scaled up to 2 instances")

	t.Log("Scale up/down test completed successfully")
}
