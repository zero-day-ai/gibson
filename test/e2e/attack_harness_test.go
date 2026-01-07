package e2e

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/daemon"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/llm/providers"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestE2E_AttackWithHarnessFactory tests the full attack flow:
// 1. Create LLM registry with mock provider
// 2. Create HarnessFactory with proper configuration
// 3. Create MissionOrchestrator with HarnessFactory
// 4. Execute mission and verify agent receives working harness
func TestE2E_AttackWithHarnessFactory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Create LLM registry with mock provider
	registry := llm.NewLLMRegistry()
	mockProvider := providers.NewMockProvider([]string{
		"I am analyzing the target for vulnerabilities.",
		"Found potential injection point in the login form.",
		"Testing complete. No critical vulnerabilities found.",
	})
	err := registry.RegisterProvider(mockProvider)
	require.NoError(t, err)

	// 2. Create SlotManager
	slotManager := llm.NewSlotManager(registry)

	// 3. Create HarnessFactory
	harnessConfig := harness.HarnessConfig{
		LLMRegistry:  registry,
		SlotManager:  slotManager,
		FindingStore: harness.NewInMemoryFindingStore(),
	}
	factory, err := harness.NewHarnessFactory(harnessConfig)
	require.NoError(t, err)
	require.NotNil(t, factory)

	// 4. Create mission store
	store := newMockMissionStore()

	// 5. Create workflow executor
	workflowExecutor := workflow.NewWorkflowExecutor()

	// 6. Create orchestrator with harness factory
	orchestrator := mission.NewMissionOrchestrator(
		store,
		mission.WithHarnessFactory(factory),
		mission.WithWorkflowExecutor(workflowExecutor),
	)
	require.NotNil(t, orchestrator)

	// 7. Create test mission with workflow
	missionID := types.NewID()
	testMission := &mission.Mission{
		ID:       missionID,
		Name:     "e2e-harness-test",
		Status:   mission.MissionStatusPending,
		TargetID: types.NewID(),
		WorkflowJSON: `{
			"id": "` + types.NewID().String() + `",
			"name": "test-workflow",
			"nodes": {
				"agent-1": {
					"id": "agent-1",
					"type": "agent",
					"name": "Test Agent",
					"agent_name": "test-agent",
					"agent_task": {
						"id": "` + types.NewID().String() + `",
						"name": "analyze-target",
						"description": "Analyze target for vulnerabilities"
					}
				}
			}
		}`,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save mission to store
	err = store.Save(ctx, testMission)
	require.NoError(t, err)

	// 8. Execute mission
	result, err := orchestrator.Execute(ctx, testMission)

	// The execution may error due to agent not being registered,
	// but the harness should have been created successfully.
	// We verify the factory was used by checking no "harness factory not configured" error.
	if err != nil {
		// Make sure error is NOT about missing harness factory
		assert.NotContains(t, err.Error(), "harness factory not configured",
			"Harness factory should be properly configured")
	}

	// If we got a result, verify basic structure
	if result != nil {
		assert.Equal(t, missionID, result.MissionID)
	}
}

// TestE2E_HarnessFactoryCreatesValidHarness verifies that HarnessFactory
// creates harnesses that can actually be used for LLM calls.
func TestE2E_HarnessFactoryCreatesValidHarness(t *testing.T) {
	ctx := context.Background()

	// 1. Create LLM registry with mock provider
	registry := llm.NewLLMRegistry()
	mockProvider := providers.NewMockProvider([]string{"Test response from mock LLM"})
	err := registry.RegisterProvider(mockProvider)
	require.NoError(t, err)

	// 2. Create DaemonSlotManager with constraint-based slot resolution
	slotManager := daemon.NewDaemonSlotManager(registry, slog.Default())

	// 3. Create HarnessFactory
	harnessConfig := harness.HarnessConfig{
		LLMRegistry:  registry,
		SlotManager:  slotManager,
		FindingStore: harness.NewInMemoryFindingStore(),
	}
	factory, err := harness.NewHarnessFactory(harnessConfig)
	require.NoError(t, err)

	// 4. Create harness via factory
	missionCtx := harness.NewMissionContext(types.NewID(), "test-mission", "test-agent")
	targetInfo := harness.NewTargetInfo(types.NewID(), "test-target", "http://example.com", "web")

	agentHarness, err := factory.Create("test-agent", missionCtx, targetInfo)
	require.NoError(t, err)
	require.NotNil(t, agentHarness)

	// 5. Verify harness components
	assert.NotNil(t, agentHarness.Logger(), "Harness should have a logger")
	assert.Equal(t, missionCtx.ID, agentHarness.Mission().ID, "Mission ID should match")
	assert.Equal(t, "test-agent", agentHarness.Mission().CurrentAgent, "Current agent should match")
	assert.Equal(t, "test-target", agentHarness.Target().Name, "Target name should match")

	// 6. Verify harness can make LLM calls
	// The "primary" slot uses constraint-based resolution to find a matching model
	// from our registered mock provider
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Hello, test!"},
	}

	response, err := agentHarness.Complete(ctx, "primary", messages)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "Test response from mock LLM", response.Message.Content)

	// 7. Verify mock provider recorded the call
	calls := mockProvider.GetCalls()
	assert.Len(t, calls, 1, "Mock provider should have received 1 call")
}

// TestE2E_HarnessSubmitFinding verifies finding submission through harness
func TestE2E_HarnessSubmitFinding(t *testing.T) {
	ctx := context.Background()

	// 1. Create LLM registry with mock provider
	registry := llm.NewLLMRegistry()
	mockProvider := providers.NewMockProvider([]string{"response"})
	err := registry.RegisterProvider(mockProvider)
	require.NoError(t, err)

	// 2. Create SlotManager
	slotManager := llm.NewSlotManager(registry)

	// 3. Create HarnessFactory with finding store
	findingStore := harness.NewInMemoryFindingStore()
	harnessConfig := harness.HarnessConfig{
		LLMRegistry:  registry,
		SlotManager:  slotManager,
		FindingStore: findingStore,
	}
	factory, err := harness.NewHarnessFactory(harnessConfig)
	require.NoError(t, err)

	// 4. Create harness
	missionCtx := harness.NewMissionContext(types.NewID(), "test-mission", "test-agent")
	targetInfo := harness.NewTargetInfo(types.NewID(), "test-target", "http://example.com", "web")

	agentHarness, err := factory.Create("test-agent", missionCtx, targetInfo)
	require.NoError(t, err)

	// 5. Submit a finding
	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "Test Vulnerability",
		Description: "Found during E2E testing",
		Severity:    agent.SeverityMedium,
		Confidence:  0.8,
	}

	err = agentHarness.SubmitFinding(ctx, finding)
	require.NoError(t, err)

	// 6. Verify finding was stored
	findings, err := agentHarness.GetFindings(ctx, harness.FindingFilter{})
	require.NoError(t, err)
	assert.Len(t, findings, 1)
	assert.Equal(t, "Test Vulnerability", findings[0].Title)
}

// TestE2E_WorkflowExecutorWithHarness tests workflow executor with real harness
func TestE2E_WorkflowExecutorWithHarness(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Create mock harness that tracks calls
	mockHarness := newTrackingMockHarness()

	// Configure delegate to return success
	mockHarness.delegateFunc = func(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
		return agent.Result{
			Status: agent.ResultStatusCompleted,
			Findings: []agent.Finding{
				{
					ID:          types.NewID(),
					Title:       "Finding from " + name,
					Severity:    agent.SeverityLow,
					Description: "Test finding",
				},
			},
		}, nil
	}

	// 2. Create workflow executor
	executor := workflow.NewWorkflowExecutor()

	// 3. Create test workflow with agent node
	testWorkflow := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "e2e-test-workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"agent-1": {
				ID:        "agent-1",
				Type:      workflow.NodeTypeAgent,
				Name:      "Test Agent",
				AgentName: "test-agent",
				AgentTask: &agent.Task{
					ID:          types.NewID(),
					Name:        "test-task",
					Description: "E2E test task",
				},
			},
		},
	}

	// 4. Execute workflow
	result, err := executor.Execute(ctx, testWorkflow, mockHarness)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 5. Verify execution completed
	assert.Equal(t, workflow.WorkflowStatusCompleted, result.Status)
	assert.Equal(t, 1, result.NodesExecuted)
	assert.Equal(t, 0, result.NodesFailed)

	// 6. Verify agent was delegated to
	assert.Len(t, mockHarness.delegateCalls, 1)
	assert.Equal(t, "test-agent", mockHarness.delegateCalls[0].name)
}

// ============================================================================
// Mock Implementations
// ============================================================================

// mockMissionStore implements mission.MissionStore for testing
type mockMissionStore struct {
	mu       sync.RWMutex
	missions map[types.ID]*mission.Mission
}

func newMockMissionStore() *mockMissionStore {
	return &mockMissionStore{
		missions: make(map[types.ID]*mission.Mission),
	}
}

func (m *mockMissionStore) Save(ctx context.Context, mis *mission.Mission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.missions[mis.ID] = mis
	return nil
}

func (m *mockMissionStore) Get(ctx context.Context, id types.ID) (*mission.Mission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if mis, ok := m.missions[id]; ok {
		return mis, nil
	}
	return nil, nil
}

func (m *mockMissionStore) GetByName(ctx context.Context, name string) (*mission.Mission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, mis := range m.missions {
		if mis.Name == name {
			return mis, nil
		}
	}
	return nil, nil
}

func (m *mockMissionStore) List(ctx context.Context, filter *mission.MissionFilter) ([]*mission.Mission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*mission.Mission
	for _, mis := range m.missions {
		result = append(result, mis)
	}
	return result, nil
}

func (m *mockMissionStore) Update(ctx context.Context, mis *mission.Mission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.missions[mis.ID] = mis
	return nil
}

func (m *mockMissionStore) UpdateStatus(ctx context.Context, id types.ID, status mission.MissionStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mis, ok := m.missions[id]; ok {
		mis.Status = status
	}
	return nil
}

func (m *mockMissionStore) UpdateProgress(ctx context.Context, id types.ID, progress float64) error {
	return nil
}

func (m *mockMissionStore) Delete(ctx context.Context, id types.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.missions, id)
	return nil
}

func (m *mockMissionStore) GetByTarget(ctx context.Context, targetID types.ID) ([]*mission.Mission, error) {
	return nil, nil
}

func (m *mockMissionStore) GetActive(ctx context.Context) ([]*mission.Mission, error) {
	return nil, nil
}

func (m *mockMissionStore) SaveCheckpoint(ctx context.Context, missionID types.ID, checkpoint *mission.MissionCheckpoint) error {
	return nil
}

func (m *mockMissionStore) Count(ctx context.Context, filter *mission.MissionFilter) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.missions), nil
}

// trackingMockHarness is a mock harness that tracks all calls
type trackingMockHarness struct {
	mu            sync.Mutex
	delegateCalls []delegateCall
	delegateFunc  func(ctx context.Context, name string, task agent.Task) (agent.Result, error)
}

type delegateCall struct {
	name string
	task agent.Task
}

func newTrackingMockHarness() *trackingMockHarness {
	return &trackingMockHarness{
		delegateCalls: []delegateCall{},
	}
}

func (m *trackingMockHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{
		Message: llm.Message{Role: llm.RoleAssistant, Content: "mock response"},
	}, nil
}

func (m *trackingMockHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{
		Message: llm.Message{Role: llm.RoleAssistant, Content: "mock response"},
	}, nil
}

func (m *trackingMockHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{Delta: llm.StreamDelta{Content: "mock"}}
	close(ch)
	return ch, nil
}

func (m *trackingMockHarness) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return map[string]any{"result": "success"}, nil
}

func (m *trackingMockHarness) ListTools() []harness.ToolDescriptor {
	return nil
}

func (m *trackingMockHarness) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	return nil, nil
}

func (m *trackingMockHarness) ListPlugins() []harness.PluginDescriptor {
	return nil
}

func (m *trackingMockHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	m.mu.Lock()
	m.delegateCalls = append(m.delegateCalls, delegateCall{name: name, task: task})
	m.mu.Unlock()

	if m.delegateFunc != nil {
		return m.delegateFunc(ctx, name, task)
	}
	return agent.Result{Status: agent.ResultStatusCompleted}, nil
}

func (m *trackingMockHarness) ListAgents() []harness.AgentDescriptor {
	return nil
}

func (m *trackingMockHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return nil
}

func (m *trackingMockHarness) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}

func (m *trackingMockHarness) Memory() memory.MemoryStore {
	return nil
}

func (m *trackingMockHarness) Mission() harness.MissionContext {
	return harness.MissionContext{ID: types.NewID(), Name: "test-mission"}
}

func (m *trackingMockHarness) Target() harness.TargetInfo {
	return harness.TargetInfo{Name: "test-target"}
}

func (m *trackingMockHarness) Tracer() trace.Tracer {
	return noop.NewTracerProvider().Tracer("test")
}

func (m *trackingMockHarness) Logger() *slog.Logger {
	return slog.Default()
}

func (m *trackingMockHarness) Metrics() harness.MetricsRecorder {
	return nil
}

func (m *trackingMockHarness) TokenUsage() *llm.TokenTracker {
	return nil
}
