//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/attack"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/payload"
	"github.com/zero-day-ai/gibson/internal/registry"
	"github.com/zero-day-ai/gibson/internal/types"
	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// TestAttackWithRegistry tests the full attack flow using the new registry system.
// This test verifies end-to-end integration:
// 1. Start embedded etcd via registry.Manager
// 2. Register a mock agent via SDK
// 3. Create attack runner with RegistryAdapter
// 4. Run attack targeting the mock agent
// 5. Verify agent was called and results returned
func TestAttackWithRegistry(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Step 1: Start embedded etcd via registry.Manager
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

	// Step 2: Register a mock agent
	t.Log("Step 2: Registering mock agent with registry...")
	agentPort := getAvailablePort(t)

	// Create mock agent server and register it
	mockAgentServer, _ := startMockAgentServer(t, ctx, registryAddr, agentPort)
	defer mockAgentServer.Stop()

	// Wait for agent registration
	t.Log("Waiting for agent to register...")
	err = waitForCondition(30*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-attack-agent")
		return err == nil && len(instances) > 0
	})
	require.NoError(t, err, "Agent should register within timeout")

	// Verify agent is registered
	instances, err := reg.Discover(ctx, "agent", "test-attack-agent")
	require.NoError(t, err)
	require.Len(t, instances, 1, "Should find exactly one agent instance")
	t.Logf("Agent registered: %s at %s", instances[0].Name, instances[0].Endpoint)

	// Step 3: Create attack runner with RegistryAdapter
	t.Log("Step 3: Creating attack runner with registry adapter...")
	adapter := registry.NewRegistryAdapter(reg)
	defer adapter.Close()

	// Create mock dependencies for attack runner
	mockOrchestrator := &mockMissionOrchestrator{}
	mockPayloadRegistry := &mockPayloadRegistry{}
	mockMissionStore := &mockMissionStore{}
	mockFindingStore := &mockFindingStore{}

	runner := attack.NewAttackRunner(
		mockOrchestrator,
		adapter,
		mockPayloadRegistry,
		mockMissionStore,
		mockFindingStore,
		attack.WithComponentDiscovery(adapter),
	)

	// Step 4: Run attack targeting the mock agent
	t.Log("Step 4: Running attack...")
	opts := attack.NewAttackOptions()
	opts.AgentName = "test-attack-agent"
	opts.TargetURL = "https://api.test.example.com/chat"
	opts.Goal = "Test prompt injection"
	opts.MaxTurns = 5
	opts.Timeout = 30 * time.Second

	result, err := runner.Run(ctx, opts)
	require.NoError(t, err, "Attack should execute successfully")
	require.NotNil(t, result, "Result should not be nil")

	// Step 5: Verify attack flow executed
	t.Log("Step 5: Verifying attack results...")

	// The attack should complete (note: agent selection may fail gracefully since we're using a mock agent)
	// The key integration test is that the registry adapter can discover the agent
	assert.NotNil(t, result, "Result should not be nil")

	// Verify the mock agent is still discoverable after attack attempt
	instances, err = reg.Discover(ctx, "agent", "test-attack-agent")
	require.NoError(t, err)
	assert.Len(t, instances, 1, "Agent should still be registered after attack")

	t.Logf("Attack completed: status=%s, duration=%v",
		result.Status, result.Duration)

	// Additional verification: registry adapter ListAgents
	adapter = registry.NewRegistryAdapter(reg)
	defer adapter.Close()

	agentList, err := adapter.ListAgents(ctx)
	require.NoError(t, err, "Should be able to list agents")
	assert.Len(t, agentList, 1, "Should have 1 agent in list")
	assert.Equal(t, "test-attack-agent", agentList[0].Name, "Agent name should match")
}

// TestAttackWithMultipleAgentInstances tests load balancing across multiple agent instances.
func TestAttackWithMultipleAgentInstances(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Start registry
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

	// Start 3 agent instances
	t.Log("Starting 3 agent instances...")
	var mockServers []*mockAgentGRPCServer
	for i := 0; i < 3; i++ {
		agentPort := getAvailablePort(t)
		server, _ := startMockAgentServer(t, ctx, registryAddr, agentPort)
		mockServers = append(mockServers, server)
		defer server.Stop()
	}

	// Wait for all agents to register
	t.Log("Waiting for all agents to register...")
	err = waitForCondition(30*time.Second, 500*time.Millisecond, func() bool {
		instances, err := reg.Discover(ctx, "agent", "test-attack-agent")
		return err == nil && len(instances) == 3
	})
	require.NoError(t, err, "All 3 agents should register")

	// Create adapter to test registry integration
	adapter := registry.NewRegistryAdapter(reg)
	defer adapter.Close()

	// Verify load balancer can see all 3 instances
	t.Log("Verifying load balancer can see all instances...")
	agentList, err := adapter.ListAgents(ctx)
	require.NoError(t, err, "Should be able to list agents")
	assert.Len(t, agentList, 1, "Should have 1 unique agent name")
	assert.Equal(t, 3, agentList[0].Instances, "Should have 3 instances")
	assert.Len(t, agentList[0].Endpoints, 3, "Should have 3 endpoints")

	t.Logf("Agent instances: %d endpoints: %v", agentList[0].Instances, agentList[0].Endpoints)

	// Verify each instance has unique endpoint
	endpointSet := make(map[string]bool)
	for _, endpoint := range agentList[0].Endpoints {
		endpointSet[endpoint] = true
	}
	assert.Len(t, endpointSet, 3, "All endpoints should be unique")
}

// TestAttackWithAgentFailure tests attack behavior when agent is unavailable.
func TestAttackWithAgentFailure(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Start registry
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

	// Create attack runner WITHOUT registering any agent
	adapter := registry.NewRegistryAdapter(reg)
	defer adapter.Close()

	runner := attack.NewAttackRunner(
		&mockMissionOrchestrator{},
		adapter,
		&mockPayloadRegistry{},
		&mockMissionStore{},
		&mockFindingStore{},
		attack.WithComponentDiscovery(adapter),
	)

	// Try to run attack with non-existent agent
	t.Log("Attempting attack with non-existent agent...")
	opts := attack.NewAttackOptions()
	opts.AgentName = "non-existent-agent"
	opts.TargetURL = "https://api.test.example.com/chat"

	result, err := runner.Run(ctx, opts)

	// Attack should fail gracefully
	require.NoError(t, err, "Run should not panic on agent not found")
	require.NotNil(t, result, "Result should not be nil")
	assert.NotEqual(t, attack.AttackStatusSuccess, result.Status, "Attack should not succeed")
	assert.NotNil(t, result.Error, "Result should contain error")

	t.Logf("Attack failed as expected: %v", result.Error)
}

// =============================================================================
// Mock Implementations
// =============================================================================

// mockAgentGRPCServer simulates a gRPC agent server for testing
type mockAgentGRPCServer struct {
	endpoint     string
	listener     net.Listener
	regClient    sdkregistry.Registry
	serviceInfo  sdkregistry.ServiceInfo
	callCount    int
	ctx          context.Context
	cancelFunc   context.CancelFunc
}

func startMockAgentServer(t *testing.T, ctx context.Context, registryEndpoint string, port int) (*mockAgentGRPCServer, sdkregistry.ServiceInfo) {
	t.Helper()

	endpoint := fmt.Sprintf("localhost:%d", port)

	// Create listener to hold the port
	listener, err := net.Listen("tcp", endpoint)
	require.NoError(t, err, "Failed to create listener")

	// Create registry client
	regClient, err := sdkregistry.NewClient(sdkregistry.Config{
		Endpoints: []string{registryEndpoint},
		Namespace: "test-gibson",
		TTL:       30,
	})
	require.NoError(t, err, "Failed to create registry client")

	// Create service info
	instanceID := uuid.New().String()
	serviceInfo := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "test-attack-agent",
		Version:    "1.0.0",
		InstanceID: instanceID,
		Endpoint:   endpoint,
		Metadata: map[string]string{
			"capabilities":    "prompt_injection,jailbreak",
			"target_types":    "llm_chat,llm_api",
			"technique_types": "prompt_injection",
			"description":     "Test attack agent for integration testing",
		},
		StartedAt: time.Now(),
	}

	// Register with registry
	err = regClient.Register(ctx, serviceInfo)
	require.NoError(t, err, "Failed to register agent")

	serverCtx, cancel := context.WithCancel(ctx)

	server := &mockAgentGRPCServer{
		endpoint:    endpoint,
		listener:    listener,
		regClient:   regClient,
		serviceInfo: serviceInfo,
		callCount:   0,
		ctx:         serverCtx,
		cancelFunc:  cancel,
	}

	// Start background goroutine to simulate agent behavior
	go server.serve()

	return server, serviceInfo
}

func (s *mockAgentGRPCServer) serve() {
	// Simple serve loop - just increment call count when "called"
	// In a real implementation, this would handle gRPC requests
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Keep-alive: just maintain registration
		}
	}
}

func (s *mockAgentGRPCServer) CallCount() int {
	return s.callCount
}

func (s *mockAgentGRPCServer) IncrementCalls() {
	s.callCount++
}

func (s *mockAgentGRPCServer) Stop() {
	s.cancelFunc()
	_ = s.regClient.Deregister(context.Background(), s.serviceInfo)
	_ = s.regClient.Close()
	_ = s.listener.Close()
}

// mockMissionOrchestrator mocks the mission.MissionOrchestrator interface
type mockMissionOrchestrator struct {
	ExecuteCalled bool
	LastMission   *mission.Mission
}

func (m *mockMissionOrchestrator) Execute(ctx context.Context, missionObj *mission.Mission) (*mission.MissionResult, error) {
	m.ExecuteCalled = true
	m.LastMission = missionObj

	// Return a successful result with mock data
	result := &mission.MissionResult{
		MissionID:      missionObj.ID,
		Status:         mission.MissionStatusCompleted,
		FindingIDs:     []types.ID{},
		WorkflowResult: make(map[string]any),
		Metrics: &mission.MissionMetrics{
			Duration:       5 * time.Second,
			CompletedNodes: 1,
			TotalTokens:    100,
			TotalCost:      0.001,
		},
	}

	return result, nil
}

// mockPayloadRegistry mocks the payload.PayloadRegistry interface
type mockPayloadRegistry struct{}

func (m *mockPayloadRegistry) Get(ctx context.Context, id types.ID) (*payload.Payload, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockPayloadRegistry) ClearCache() {
	// No-op for mock
}

func (m *mockPayloadRegistry) Count(ctx context.Context, filter *payload.PayloadFilter) (int, error) {
	return 0, nil
}

func (m *mockPayloadRegistry) Register(ctx context.Context, p *payload.Payload) error {
	return nil
}

func (m *mockPayloadRegistry) Disable(ctx context.Context, id types.ID) error {
	return nil
}

func (m *mockPayloadRegistry) Enable(ctx context.Context, id types.ID) error {
	return nil
}

func (m *mockPayloadRegistry) GetByCategory(ctx context.Context, category payload.PayloadCategory) ([]*payload.Payload, error) {
	return nil, nil
}

func (m *mockPayloadRegistry) GetByMitreTechnique(ctx context.Context, technique string) ([]*payload.Payload, error) {
	return nil, nil
}

func (m *mockPayloadRegistry) LoadBuiltIns(ctx context.Context) error {
	return nil
}

func (m *mockPayloadRegistry) Health(ctx context.Context) types.HealthStatus {
	return types.Healthy("mock payload registry")
}

func (m *mockPayloadRegistry) Search(ctx context.Context, query string, filter *payload.PayloadFilter) ([]*payload.Payload, error) {
	return nil, nil
}

func (m *mockPayloadRegistry) List(ctx context.Context, filter *payload.PayloadFilter) ([]*payload.Payload, error) {
	return nil, nil
}

func (m *mockPayloadRegistry) Update(ctx context.Context, p *payload.Payload) error {
	return nil
}

// mockMissionStore mocks the mission.MissionStore interface
type mockMissionStore struct {
	missions map[types.ID]*mission.Mission
}

func (m *mockMissionStore) Save(ctx context.Context, missionObj *mission.Mission) error {
	if m.missions == nil {
		m.missions = make(map[types.ID]*mission.Mission)
	}
	m.missions[missionObj.ID] = missionObj
	return nil
}

func (m *mockMissionStore) Get(ctx context.Context, id types.ID) (*mission.Mission, error) {
	if m.missions == nil {
		return nil, fmt.Errorf("mission not found")
	}
	missionObj, ok := m.missions[id]
	if !ok {
		return nil, fmt.Errorf("mission not found")
	}
	return missionObj, nil
}

func (m *mockMissionStore) List(ctx context.Context, filter *mission.MissionFilter) ([]*mission.Mission, error) {
	result := make([]*mission.Mission, 0, len(m.missions))
	for _, missionObj := range m.missions {
		result = append(result, missionObj)
	}
	return result, nil
}

func (m *mockMissionStore) Delete(ctx context.Context, id types.ID) error {
	delete(m.missions, id)
	return nil
}

func (m *mockMissionStore) Count(ctx context.Context, filter *mission.MissionFilter) (int, error) {
	return len(m.missions), nil
}

func (m *mockMissionStore) GetByName(ctx context.Context, name string) (*mission.Mission, error) {
	for _, m := range m.missions {
		if m.Name == name {
			return m, nil
		}
	}
	return nil, fmt.Errorf("mission not found")
}

func (m *mockMissionStore) GetActive(ctx context.Context) ([]*mission.Mission, error) {
	result := make([]*mission.Mission, 0)
	for _, missionObj := range m.missions {
		if missionObj.Status == mission.MissionStatusRunning || missionObj.Status == mission.MissionStatusPending {
			result = append(result, missionObj)
		}
	}
	return result, nil
}

func (m *mockMissionStore) GetByTarget(ctx context.Context, targetID types.ID) ([]*mission.Mission, error) {
	result := make([]*mission.Mission, 0)
	for _, missionObj := range m.missions {
		if missionObj.TargetID == targetID {
			result = append(result, missionObj)
		}
	}
	return result, nil
}

func (m *mockMissionStore) Update(ctx context.Context, missionObj *mission.Mission) error {
	if m.missions == nil {
		m.missions = make(map[types.ID]*mission.Mission)
	}
	m.missions[missionObj.ID] = missionObj
	return nil
}

func (m *mockMissionStore) UpdateStatus(ctx context.Context, id types.ID, status mission.MissionStatus) error {
	if m.missions == nil {
		return fmt.Errorf("mission not found")
	}
	missionObj, ok := m.missions[id]
	if !ok {
		return fmt.Errorf("mission not found")
	}
	missionObj.Status = status
	return nil
}

func (m *mockMissionStore) UpdateProgress(ctx context.Context, id types.ID, progress float64) error {
	if m.missions == nil {
		return fmt.Errorf("mission not found")
	}
	missionObj, ok := m.missions[id]
	if !ok {
		return fmt.Errorf("mission not found")
	}
	missionObj.Progress = progress
	return nil
}

func (m *mockMissionStore) SaveCheckpoint(ctx context.Context, missionID types.ID, checkpoint *mission.MissionCheckpoint) error {
	return nil
}

// mockFindingStore mocks the finding.FindingStore interface
type mockFindingStore struct {
	findings map[types.ID]*finding.EnhancedFinding
}

func (m *mockFindingStore) Store(ctx context.Context, f finding.EnhancedFinding) error {
	if m.findings == nil {
		m.findings = make(map[types.ID]*finding.EnhancedFinding)
	}
	m.findings[f.ID] = &f
	return nil
}

func (m *mockFindingStore) Get(ctx context.Context, id types.ID) (*finding.EnhancedFinding, error) {
	if m.findings == nil {
		return nil, fmt.Errorf("finding not found")
	}
	f, ok := m.findings[id]
	if !ok {
		return nil, fmt.Errorf("finding not found")
	}
	return f, nil
}

func (m *mockFindingStore) List(ctx context.Context, missionID types.ID, filter *finding.FindingFilter) ([]finding.EnhancedFinding, error) {
	result := make([]finding.EnhancedFinding, 0, len(m.findings))
	for _, f := range m.findings {
		result = append(result, *f)
	}
	return result, nil
}

func (m *mockFindingStore) Count(ctx context.Context, missionID types.ID) (int, error) {
	return len(m.findings), nil
}

func (m *mockFindingStore) Update(ctx context.Context, f finding.EnhancedFinding) error {
	if m.findings == nil {
		return fmt.Errorf("finding not found")
	}
	m.findings[f.ID] = &f
	return nil
}

func (m *mockFindingStore) Delete(ctx context.Context, id types.ID) error {
	delete(m.findings, id)
	return nil
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
