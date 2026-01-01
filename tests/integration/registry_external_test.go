//go:build integration && docker
// +build integration,docker

package integration

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/registry"
	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// TestExternalRegistry_FullFlow tests the complete external etcd registry lifecycle:
// 1. Start external etcd container
// 2. Start Gibson with external registry connection
// 3. Start a test agent that registers itself
// 4. Verify agent appears in registry
// 5. Verify discovery and status work
// 6. Stop agent
// 7. Verify agent removed from registry
// 8. Clean up container
func TestExternalRegistry_FullFlow(t *testing.T) {
	// Start etcd container
	ctx := context.Background()
	etcdContainer, etcdEndpoint := startEtcdContainer(t, ctx)
	defer func() {
		t.Log("Terminating etcd container...")
		_ = etcdContainer.Terminate(ctx)
	}()

	t.Logf("etcd container started at: %s", etcdEndpoint)

	// Setup external registry configuration
	cfg := config.RegistryConfig{
		Type:      "etcd",
		Endpoints: []string{etcdEndpoint},
		Namespace: "test-gibson",
		TTL:       "10s",
	}

	// Create and start registry manager
	t.Log("Starting external registry manager...")
	mgr := registry.NewManager(cfg)

	err := mgr.Start(ctx)
	require.NoError(t, err, "Failed to start registry manager")
	defer func() {
		t.Log("Stopping registry manager...")
		_ = mgr.Stop(ctx)
	}()

	// Verify registry is healthy
	status := mgr.Status()
	assert.Equal(t, "etcd", status.Type)
	assert.True(t, status.Healthy, "Registry should be healthy")
	t.Logf("Registry connected to external etcd: %s", status.Endpoint)

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
	agentStop, agentInfo := startTestAgent(t, ctx, etcdEndpoint, agentPort)
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

// TestExternalRegistry_MultipleInstances tests multiple instances with external etcd.
func TestExternalRegistry_MultipleInstances(t *testing.T) {
	// Start etcd container
	ctx := context.Background()
	etcdContainer, etcdEndpoint := startEtcdContainer(t, ctx)
	defer etcdContainer.Terminate(ctx)

	t.Logf("etcd container started at: %s", etcdEndpoint)

	// Setup external registry configuration
	cfg := config.RegistryConfig{
		Type:      "etcd",
		Endpoints: []string{etcdEndpoint},
		Namespace: "test-gibson",
		TTL:       "10s",
	}

	t.Log("Starting external registry manager...")
	mgr := registry.NewManager(cfg)
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
		stopFunc, info := startTestAgent(t, ctx, etcdEndpoint, agentPort)
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
	t.Log("Successfully tested multi-instance lifecycle with external etcd")
}

// TestExternalRegistry_ConnectionFailure tests behavior when etcd is unavailable.
func TestExternalRegistry_ConnectionFailure(t *testing.T) {
	ctx := context.Background()

	// Configure registry with invalid endpoint
	cfg := config.RegistryConfig{
		Type:      "etcd",
		Endpoints: []string{"localhost:99999"}, // Invalid port
		Namespace: "test-gibson",
		TTL:       "10s",
	}

	// Create and start registry manager
	t.Log("Attempting to start registry with invalid endpoint...")
	mgr := registry.NewManager(cfg)

	// This should fail fast
	err := mgr.Start(ctx)
	require.Error(t, err, "Registry should fail to start with invalid endpoint")
	assert.Contains(t, err.Error(), "cannot connect", "Error should mention connection failure")
	t.Logf("Registry correctly failed fast: %v", err)
}

// TestExternalRegistry_HighAvailability tests client configuration with multiple endpoints.
// Note: This tests client-side endpoint configuration. A true HA cluster test would require
// a multi-node etcd cluster setup, which is complex for integration tests.
func TestExternalRegistry_HighAvailability(t *testing.T) {
	ctx := context.Background()

	// Start one working etcd container
	etcdContainer1, etcdEndpoint1 := startEtcdContainer(t, ctx)
	defer etcdContainer1.Terminate(ctx)

	t.Logf("Started etcd container: %s", etcdEndpoint1)

	// Configure registry with multiple endpoints (one valid, one invalid)
	// This tests that the client can connect when at least one endpoint is valid
	cfg := config.RegistryConfig{
		Type:      "etcd",
		Endpoints: []string{etcdEndpoint1, "localhost:99998"}, // One valid, one invalid
		Namespace: "test-gibson",
		TTL:       "10s",
	}

	t.Log("Starting external registry manager with multiple endpoints (one valid, one invalid)...")
	mgr := registry.NewManager(cfg)
	err := mgr.Start(ctx)
	require.NoError(t, err, "Should connect successfully when at least one endpoint is valid")
	defer mgr.Stop(ctx)

	// Verify registry is healthy
	status := mgr.Status()
	assert.Equal(t, "etcd", status.Type)
	assert.True(t, status.Healthy, "Registry should be healthy with at least one valid endpoint")
	t.Log("Registry successfully connected with HA configuration")

	// Register a test service
	reg := mgr.Registry()
	require.NotNil(t, reg)

	serviceInfo := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "test-agent",
		Version:    "1.0.0",
		InstanceID: uuid.New().String(),
		Endpoint:   "localhost:50051",
		Metadata: map[string]string{
			"test": "ha",
		},
		StartedAt: time.Now(),
	}

	err = reg.Register(ctx, serviceInfo)
	require.NoError(t, err, "Should register when connected to valid endpoint")

	// Verify registration
	instances, err := reg.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err)
	assert.Len(t, instances, 1, "Should find registered instance")
	t.Log("Successfully tested HA endpoint configuration with mixed valid/invalid endpoints")

	// Cleanup
	err = reg.Deregister(ctx, serviceInfo)
	require.NoError(t, err)
}

// startEtcdContainer starts an etcd container for testing.
// Returns the container and its endpoint address.
func startEtcdContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string) {
	t.Helper()

	// Create etcd container
	req := testcontainers.ContainerRequest{
		Image:        "quay.io/coreos/etcd:v3.5.17",
		ExposedPorts: []string{"2379/tcp"},
		Env: map[string]string{
			"ETCD_ADVERTISE_CLIENT_URLS":   "http://0.0.0.0:2379",
			"ETCD_LISTEN_CLIENT_URLS":      "http://0.0.0.0:2379",
			"ETCD_ENABLE_V2":               "true",
			"ETCDCTL_API":                  "3",
			"ALLOW_NONE_AUTHENTICATION":    "yes",
			"ETCD_MAX_REQUEST_BYTES":       "33554432",
			"ETCD_QUOTA_BACKEND_BYTES":     "8589934592",
			"ETCD_AUTO_COMPACTION_MODE":    "revision",
			"ETCD_AUTO_COMPACTION_RETENTION": "1000",
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("2379/tcp"),
			wait.ForLog("ready to serve client requests"),
		).WithDeadline(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start etcd container")

	// Get container endpoint
	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "2379")
	require.NoError(t, err)

	endpoint := fmt.Sprintf("%s:%s", host, port.Port())

	// Wait a bit for etcd to be fully ready
	time.Sleep(2 * time.Second)

	return container, endpoint
}

// startTestAgentWithExternalEtcd starts a test agent that registers to external etcd.
// This is a version of startTestAgent that works with external etcd.
func startTestAgentWithExternalEtcd(t *testing.T, ctx context.Context, etcdEndpoint string, port int) (func(), sdkregistry.ServiceInfo) {
	t.Helper()

	// Create registry client
	regClient, err := sdkregistry.NewClient(sdkregistry.Config{
		Endpoints: []string{etcdEndpoint},
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

// TestExternalRegistry_Namespace tests namespace isolation.
func TestExternalRegistry_Namespace(t *testing.T) {
	ctx := context.Background()
	etcdContainer, etcdEndpoint := startEtcdContainer(t, ctx)
	defer etcdContainer.Terminate(ctx)

	// Create two registries with different namespaces
	cfg1 := config.RegistryConfig{
		Type:      "etcd",
		Endpoints: []string{etcdEndpoint},
		Namespace: "namespace1",
		TTL:       "10s",
	}

	cfg2 := config.RegistryConfig{
		Type:      "etcd",
		Endpoints: []string{etcdEndpoint},
		Namespace: "namespace2",
		TTL:       "10s",
	}

	mgr1 := registry.NewManager(cfg1)
	err := mgr1.Start(ctx)
	require.NoError(t, err)
	defer mgr1.Stop(ctx)

	mgr2 := registry.NewManager(cfg2)
	err = mgr2.Start(ctx)
	require.NoError(t, err)
	defer mgr2.Stop(ctx)

	reg1 := mgr1.Registry()
	reg2 := mgr2.Registry()

	// Register a service in namespace1
	serviceInfo := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "test-agent",
		Version:    "1.0.0",
		InstanceID: uuid.New().String(),
		Endpoint:   "localhost:50051",
		Metadata:   map[string]string{},
		StartedAt:  time.Now(),
	}

	err = reg1.Register(ctx, serviceInfo)
	require.NoError(t, err)

	// Verify it appears in namespace1
	instances1, err := reg1.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err)
	assert.Len(t, instances1, 1, "Should find agent in namespace1")

	// Verify it does NOT appear in namespace2
	instances2, err := reg2.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err)
	assert.Empty(t, instances2, "Should not find agent in namespace2")

	t.Log("Successfully verified namespace isolation")

	// Cleanup
	err = reg1.Deregister(ctx, serviceInfo)
	require.NoError(t, err)
}

// TestExternalRegistry_TTLExpiration tests that services expire after TTL.
func TestExternalRegistry_TTLExpiration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TTL expiration test in short mode")
	}

	ctx := context.Background()
	etcdContainer, etcdEndpoint := startEtcdContainer(t, ctx)
	defer etcdContainer.Terminate(ctx)

	// Configure registry with very short TTL
	cfg := config.RegistryConfig{
		Type:      "etcd",
		Endpoints: []string{etcdEndpoint},
		Namespace: "test-gibson",
		TTL:       "3s", // Short TTL for testing
	}

	mgr := registry.NewManager(cfg)
	err := mgr.Start(ctx)
	require.NoError(t, err)
	defer mgr.Stop(ctx)

	reg := mgr.Registry()

	// Register a service WITHOUT keepalive
	serviceInfo := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "test-agent",
		Version:    "1.0.0",
		InstanceID: uuid.New().String(),
		Endpoint:   "localhost:50051",
		Metadata:   map[string]string{},
		StartedAt:  time.Now(),
	}

	// Create a client that will register but not maintain keepalive
	regClient, err := sdkregistry.NewClient(sdkregistry.Config{
		Endpoints: []string{etcdEndpoint},
		Namespace: "test-gibson",
		TTL:       3,
	})
	require.NoError(t, err)

	err = regClient.Register(ctx, serviceInfo)
	require.NoError(t, err)

	// Close the client immediately (stops keepalive)
	regClient.Close()

	// Verify it's initially registered
	instances, err := reg.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err)
	assert.Len(t, instances, 1, "Should find agent initially")

	// Wait for TTL to expire (3s + buffer)
	t.Log("Waiting for TTL expiration...")
	time.Sleep(5 * time.Second)

	// Verify it's expired
	instances, err = reg.Discover(ctx, "agent", "test-agent")
	require.NoError(t, err)
	assert.Empty(t, instances, "Agent should expire after TTL")

	t.Log("Successfully verified TTL expiration")
}

// TestExternalRegistry_Watch tests watch functionality with external etcd.
func TestExternalRegistry_Watch(t *testing.T) {
	ctx := context.Background()
	etcdContainer, etcdEndpoint := startEtcdContainer(t, ctx)
	defer etcdContainer.Terminate(ctx)

	cfg := config.RegistryConfig{
		Type:      "etcd",
		Endpoints: []string{etcdEndpoint},
		Namespace: "test-gibson",
		TTL:       "10s",
	}

	mgr := registry.NewManager(cfg)
	err := mgr.Start(ctx)
	require.NoError(t, err)
	defer mgr.Stop(ctx)

	reg := mgr.Registry()

	// Start watching
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	watchCh, err := reg.Watch(watchCtx, "agent", "test-agent")
	require.NoError(t, err)

	// Should get initial empty state
	select {
	case services := <-watchCh:
		assert.Empty(t, services, "Should start with no services")
	case <-time.After(2 * time.Second):
		t.Fatal("Did not receive initial watch state")
	}

	// Register a service
	serviceInfo := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "test-agent",
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
		assert.Len(t, services, 1, "Should receive 1 service after registration")
	case <-time.After(3 * time.Second):
		t.Fatal("Did not receive watch update after registration")
	}

	// Deregister
	err = reg.Deregister(ctx, serviceInfo)
	require.NoError(t, err)

	// Should receive update with 0 services
	select {
	case services := <-watchCh:
		assert.Empty(t, services, "Should receive empty list after deregistration")
	case <-time.After(3 * time.Second):
		t.Fatal("Did not receive watch update after deregistration")
	}

	t.Log("Successfully tested watch functionality with external etcd")
}

// Helper to check if a string contains any of the given substrings
func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
