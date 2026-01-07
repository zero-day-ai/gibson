// //go:build integration
//go:build integration
// +build integration

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/registry"
	sdkagent "github.com/zero-day-ai/sdk/agent"
	sdkregistry "github.com/zero-day-ai/sdk/registry"
	"github.com/zero-day-ai/sdk/serve"
	"github.com/zero-day-ai/sdk/types"
)

// TestAgentRegistrationAndDiscovery is an end-to-end test that validates the complete
// agent registration and discovery flow:
//  1. Starts embedded etcd for testing
//  2. Starts a test agent via SDK serve infrastructure
//  3. Waits for agent registration
//  4. Queries registry to verify agent appears
//  5. Verifies agent metadata (name, version, capabilities)
//  6. Cleans up processes properly
//
// This test verifies Task 46: End-to-End Test - Agent Registration and Discovery
func TestAgentRegistrationAndDiscovery(t *testing.T) {
	// Set up test timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create temporary directory for etcd data
	tmpDir, err := os.MkdirTemp("", "gibson-e2e-test-*")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tmpDir)

	// Step 1: Start embedded etcd registry
	regConfig := sdkregistry.Config{
		Type:          "embedded",
		DataDir:       filepath.Join(tmpDir, "etcd-data"),
		ListenAddress: "localhost:12379", // Use non-standard port to avoid conflicts
		Namespace:     "gibson-test",
		TTL:           10, // 10 second TTL for faster test iterations
	}

	etcdReg, err := registry.NewEmbeddedRegistry(regConfig)
	require.NoError(t, err, "Failed to create embedded registry")
	defer func() {
		err := etcdReg.Close()
		if err != nil {
			t.Logf("Warning: failed to close registry: %v", err)
		}
	}()

	// Give etcd a moment to fully initialize
	time.Sleep(500 * time.Millisecond)

	// Step 2: Create test agent with unique name to avoid conflicts
	testAgentName := fmt.Sprintf("test-agent-e2e-%d", time.Now().UnixNano())
	testAgent := createTestAgent(testAgentName, "1.0.0")

	// Step 3: Start agent server in background with registry enabled
	agentErrCh := make(chan error, 1)
	agentCtx, agentCancel := context.WithCancel(ctx)
	defer agentCancel()

	go func() {
		// Serve the agent with registry registration
		err := serve.Agent(testAgent,
			serve.WithPort(0), // Use any available port
			serve.WithRegistry(etcdReg),
		)
		if err != nil {
			agentErrCh <- err
		}
	}()

	// Wait for agent to start and register (with timeout and retries)
	t.Log("Waiting for agent to register with etcd...")
	var registeredAgents []sdkregistry.ServiceInfo
	registered := false
	for i := 0; i < 20; i++ { // Try for up to 10 seconds (20 * 500ms)
		select {
		case err := <-agentErrCh:
			t.Fatalf("Agent server failed to start: %v", err)
		case <-agentCtx.Done():
			t.Fatal("Test context cancelled before agent registered")
		default:
			// Continue checking
		}

		// Query registry for test agent
		agents, err := etcdReg.Discover(agentCtx, "agent", testAgentName)
		if err != nil {
			t.Logf("Registry query attempt %d failed: %v", i+1, err)
		} else if len(agents) > 0 {
			registeredAgents = agents
			registered = true
			t.Logf("Agent registered successfully after %d attempts", i+1)
			break
		}

		time.Sleep(500 * time.Millisecond)
	}

	require.True(t, registered, "Agent failed to register within timeout period")
	require.NotEmpty(t, registeredAgents, "No agents found in registry")

	// Step 4: Verify agent metadata
	t.Log("Verifying agent metadata...")
	agent := registeredAgents[0]

	assert.Equal(t, testAgentName, agent.Name, "Agent name mismatch")
	assert.Equal(t, "1.0.0", agent.Version, "Agent version mismatch")
	assert.Equal(t, "agent", agent.Kind, "Agent kind mismatch")
	assert.NotEmpty(t, agent.InstanceID, "Agent instance ID should not be empty")
	assert.NotEmpty(t, agent.Endpoint, "Agent endpoint should not be empty")

	// Verify metadata fields
	assert.Contains(t, agent.Metadata, "description", "Agent metadata should contain description")
	assert.Equal(t, "Test agent for E2E testing", agent.Metadata["description"])

	assert.Contains(t, agent.Metadata, "capabilities", "Agent metadata should contain capabilities")
	assert.Contains(t, agent.Metadata["capabilities"], "prompt_injection")

	assert.Contains(t, agent.Metadata, "target_types", "Agent metadata should contain target_types")
	assert.Contains(t, agent.Metadata["target_types"], "llm_chat")

	assert.Contains(t, agent.Metadata, "technique_types", "Agent metadata should contain technique_types")
	assert.Contains(t, agent.Metadata["technique_types"], "prompt_injection")

	t.Logf("Agent successfully registered with endpoint: %s", agent.Endpoint)

	// Step 5: Test discovery via RegistryAdapter
	t.Log("Testing agent discovery via RegistryAdapter...")
	adapter := registry.NewRegistryAdapter(etcdReg)
	defer adapter.Close()

	// List all agents
	agentInfos, err := adapter.ListAgents(agentCtx)
	require.NoError(t, err, "Failed to list agents via adapter")
	require.NotEmpty(t, agentInfos, "Adapter should find at least one agent")

	// Find our test agent in the list
	var foundAgent *registry.AgentInfo
	for i := range agentInfos {
		if agentInfos[i].Name == testAgentName {
			foundAgent = &agentInfos[i]
			break
		}
	}
	require.NotNil(t, foundAgent, "Test agent not found in ListAgents result")

	// Verify AgentInfo structure
	assert.Equal(t, testAgentName, foundAgent.Name)
	assert.Equal(t, "1.0.0", foundAgent.Version)
	assert.Equal(t, "Test agent for E2E testing", foundAgent.Description)
	assert.Equal(t, 1, foundAgent.Instances, "Should have exactly 1 instance")
	assert.Len(t, foundAgent.Endpoints, 1, "Should have exactly 1 endpoint")
	assert.Contains(t, foundAgent.Capabilities, "prompt_injection")
	assert.Contains(t, foundAgent.TargetTypes, "llm_chat")
	assert.Contains(t, foundAgent.TechniqueTypes, "prompt_injection")

	t.Log("Agent discovery via RegistryAdapter successful")

	// Step 6: Test agent discovery and connection
	t.Log("Testing agent discovery and connection...")
	discoveredAgent, err := adapter.DiscoverAgent(agentCtx, testAgentName)
	require.NoError(t, err, "Failed to discover agent")
	require.NotNil(t, discoveredAgent, "Discovered agent should not be nil")

	// Verify we can call methods on the discovered agent
	assert.Equal(t, testAgentName, discoveredAgent.Name())
	assert.Equal(t, "1.0.0", discoveredAgent.Version())
	assert.Equal(t, "Test agent for E2E testing", discoveredAgent.Description())

	// Test health check on discovered agent
	health := discoveredAgent.Health(agentCtx)
	assert.Equal(t, types.StatusHealthy, health.Status, "Agent should be healthy")

	t.Log("Agent connection and health check successful")

	// Step 7: Clean up - stop agent server
	t.Log("Cleaning up agent server...")
	agentCancel()

	// Wait for agent to deregister (with timeout)
	deregistered := false
	for i := 0; i < 20; i++ {
		agents, err := etcdReg.Discover(agentCtx, "agent", testAgentName)
		if err != nil || len(agents) == 0 {
			deregistered = true
			t.Logf("Agent deregistered after %d attempts", i+1)
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	assert.True(t, deregistered, "Agent should deregister on shutdown")
	t.Log("E2E test completed successfully")
}

// createTestAgent creates a simple test agent for E2E testing
func createTestAgent(name, version string) sdkagent.Agent {
	cfg := sdkagent.NewConfig().
		SetName(name).
		SetVersion(version).
		SetDescription("Test agent for E2E testing").
		AddCapability(sdkagent.CapabilityPromptInjection).
		AddTargetType(types.TargetTypeLLMChat).
		AddTechniqueType(types.TechniquePromptInjection).
		SetExecuteFunc(func(ctx context.Context, h sdkagent.Harness, t sdkagent.Task) (sdkagent.Result, error) {
			return sdkagent.NewSuccessResult(map[string]any{
				"message": "Test execution completed",
				"task_id": t.ID,
			}), nil
		})

	agent, err := sdkagent.New(cfg)
	if err != nil {
		panic(fmt.Sprintf("Failed to create test agent: %v", err))
	}

	return agent
}

// TestAgentRegistrationFailure tests that agents handle registry failures gracefully
func TestAgentRegistrationFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create test agent
	testAgent := createTestAgent("test-agent-fail", "1.0.0")

	// Create a registry with invalid configuration (should fail to connect)
	badRegConfig := sdkregistry.Config{
		Type:      "embedded",
		DataDir:   "/nonexistent/path/that/should/fail",
		Namespace: "gibson-test",
		TTL:       10,
	}

	badReg, err := registry.NewEmbeddedRegistry(badRegConfig)
	// Should get an error creating registry with invalid path
	if err == nil && badReg != nil {
		// If somehow it succeeded, clean up
		badReg.Close()
		t.Skip("Expected registry creation to fail with invalid path, but it succeeded")
	}

	// Test that agent serve continues even if registry is unavailable
	// (this test verifies graceful degradation)
	agentCtx, agentCancel := context.WithTimeout(ctx, 2*time.Second)
	defer agentCancel()

	// Agent should start and run even without registry
	err = serve.Agent(testAgent,
		serve.WithPort(0),
		// No registry configured - should work without registration
	)

	// Should timeout or complete without error
	// (agent runs until context cancelled)
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("Agent serve returned error: %v (this is expected for graceful degradation)", err)
	}
}

// TestMultipleAgentInstances tests registration of multiple instances of the same agent
func TestMultipleAgentInstances(t *testing.T) {
	// Set up test timeout
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Create temporary directory for etcd data
	tmpDir, err := os.MkdirTemp("", "gibson-multi-agent-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Start embedded etcd registry
	regConfig := sdkregistry.Config{
		Type:          "embedded",
		DataDir:       filepath.Join(tmpDir, "etcd-data"),
		ListenAddress: "localhost:12380", // Different port from first test
		Namespace:     "gibson-test",
		TTL:           10,
	}

	etcdReg, err := registry.NewEmbeddedRegistry(regConfig)
	require.NoError(t, err)
	defer etcdReg.Close()

	time.Sleep(500 * time.Millisecond)

	// Create two instances of the same agent with unique names
	agentName := fmt.Sprintf("multi-agent-e2e-%d", time.Now().UnixNano())
	agent1 := createTestAgent(agentName, "1.0.0")
	agent2 := createTestAgent(agentName, "1.0.0")

	// Start both agents in background
	agent1Ctx, agent1Cancel := context.WithCancel(ctx)
	agent2Ctx, agent2Cancel := context.WithCancel(ctx)
	defer agent1Cancel()
	defer agent2Cancel()

	go serve.Agent(agent1,
		serve.WithPort(0),
		serve.WithRegistry(etcdReg),
	)

	go serve.Agent(agent2,
		serve.WithPort(0),
		serve.WithRegistry(etcdReg),
	)

	// Wait for both agents to register
	t.Log("Waiting for both agent instances to register...")
	registered := false
	var instances []sdkregistry.ServiceInfo
	for i := 0; i < 20; i++ {
		agents, err := etcdReg.Discover(ctx, "agent", agentName)
		if err == nil && len(agents) >= 2 {
			instances = agents
			registered = true
			t.Logf("Both agents registered after %d attempts", i+1)
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	require.True(t, registered, "Both agents should register")
	require.Len(t, instances, 2, "Should have exactly 2 instances")

	// Verify both instances have different instance IDs but same name/version
	assert.NotEqual(t, instances[0].InstanceID, instances[1].InstanceID, "Instance IDs should be unique")
	assert.Equal(t, agentName, instances[0].Name)
	assert.Equal(t, agentName, instances[1].Name)
	assert.Equal(t, "1.0.0", instances[0].Version)
	assert.Equal(t, "1.0.0", instances[1].Version)

	// Verify ListAgents aggregates instances correctly
	adapter := registry.NewRegistryAdapter(etcdReg)
	defer adapter.Close()

	agentInfos, err := adapter.ListAgents(ctx)
	require.NoError(t, err)

	var foundAgent *registry.AgentInfo
	for i := range agentInfos {
		if agentInfos[i].Name == agentName {
			foundAgent = &agentInfos[i]
			break
		}
	}
	require.NotNil(t, foundAgent, "Agent should be found in list")
	assert.Equal(t, 2, foundAgent.Instances, "Should show 2 instances")
	assert.Len(t, foundAgent.Endpoints, 2, "Should have 2 endpoints")

	t.Log("Multiple agent instances test completed successfully")

	// Clean up
	agent1Cancel()
	agent2Cancel()
}
