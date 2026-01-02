//go:build integration

package attack

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/payload"
	"github.com/zero-day-ai/gibson/internal/types"
)

// Mock implementations for integration tests

type mockIntegrationComponentDiscovery struct {
	agents map[string]agent.Agent
}

func newMockIntegrationComponentDiscovery() *mockIntegrationComponentDiscovery {
	return &mockIntegrationComponentDiscovery{
		agents: map[string]agent.Agent{
			"test-agent": &mockIntegrationAgent{name: "test-agent"},
		},
	}
}

func (r *mockIntegrationComponentDiscovery) DiscoverAgent(ctx context.Context, name string) (agent.Agent, error) {
	if a, ok := r.agents[name]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("agent not found: %s", name)
}

func (r *mockIntegrationComponentDiscovery) DiscoverTool(ctx context.Context, name string) (interface{}, error) {
	return nil, fmt.Errorf("tool discovery not implemented in mock")
}

func (r *mockIntegrationComponentDiscovery) DiscoverPlugin(ctx context.Context, name string) (interface{}, error) {
	return nil, fmt.Errorf("plugin discovery not implemented in mock")
}

func (r *mockIntegrationComponentDiscovery) ListAgents(ctx context.Context) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (r *mockIntegrationComponentDiscovery) ListTools(ctx context.Context) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (r *mockIntegrationComponentDiscovery) ListPlugins(ctx context.Context) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (r *mockIntegrationComponentDiscovery) DelegateToAgent(ctx context.Context, name string, task agent.Task, harness agent.AgentHarness) (agent.Result, error) {
	if a, ok := r.agents[name]; ok {
		return a.Execute(ctx, task, harness)
	}
	return agent.Result{}, fmt.Errorf("agent not found: %s", name)
}

type mockIntegrationAgent struct {
	name string
}

func (a *mockIntegrationAgent) Name() string {
	return a.name
}

func (a *mockIntegrationAgent) Version() string {
	return "1.0.0"
}

func (a *mockIntegrationAgent) Description() string {
	return "Test agent for integration tests"
}

func (a *mockIntegrationAgent) Capabilities() []string {
	return []string{"test"}
}

func (a *mockIntegrationAgent) TargetTypes() []component.TargetType {
	return []component.TargetType{component.TargetTypeLLMAPI}
}

func (a *mockIntegrationAgent) TechniqueTypes() []component.TechniqueType {
	return []component.TechniqueType{component.TechniquePromptInjection}
}

func (a *mockIntegrationAgent) LLMSlots() []agent.SlotDefinition {
	return []agent.SlotDefinition{}
}

func (a *mockIntegrationAgent) Execute(ctx context.Context, task agent.Task, harness agent.AgentHarness) (agent.Result, error) {
	return agent.Result{
		TaskID:      task.ID,
		Status:      agent.ResultStatusCompleted,
		Output:      map[string]interface{}{"result": "success"},
		CompletedAt: time.Now(),
	}, nil
}

func (a *mockIntegrationAgent) Initialize(ctx context.Context, cfg agent.AgentConfig) error {
	return nil
}

func (a *mockIntegrationAgent) Shutdown(ctx context.Context) error {
	return nil
}

func (a *mockIntegrationAgent) Health(ctx context.Context) types.HealthStatus {
	return types.Healthy("mock agent ready")
}

type mockIntegrationPayloadRegistry struct {
	payloads map[types.ID]*payload.Payload
	mu       sync.Mutex
}

func newMockIntegrationPayloadRegistry() *mockIntegrationPayloadRegistry {
	return &mockIntegrationPayloadRegistry{
		payloads: make(map[types.ID]*payload.Payload),
	}
}

func (r *mockIntegrationPayloadRegistry) Register(ctx context.Context, p *payload.Payload) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payloads[p.ID] = p
	return nil
}

func (r *mockIntegrationPayloadRegistry) Get(ctx context.Context, id types.ID) (*payload.Payload, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.payloads[id]; ok {
		return p, nil
	}
	return &payload.Payload{
		ID:         id,
		Name:       "test-payload",
		Template:   "test template content",
		Categories: []payload.PayloadCategory{payload.CategoryJailbreak},
		Enabled:    true,
		Severity:   agent.SeverityMedium,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}

func (r *mockIntegrationPayloadRegistry) List(ctx context.Context, filter *payload.PayloadFilter) ([]*payload.Payload, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return []*payload.Payload{
		{
			ID:         types.NewID(),
			Name:       "test-payload",
			Template:   "test template content",
			Categories: []payload.PayloadCategory{payload.CategoryJailbreak},
			Enabled:    true,
			Severity:   agent.SeverityMedium,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		},
	}, nil
}

func (r *mockIntegrationPayloadRegistry) Update(ctx context.Context, p *payload.Payload) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payloads[p.ID] = p
	return nil
}

func (r *mockIntegrationPayloadRegistry) Delete(ctx context.Context, id types.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.payloads, id)
	return nil
}

func (r *mockIntegrationPayloadRegistry) Search(ctx context.Context, query string, filter *payload.PayloadFilter) ([]*payload.Payload, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *mockIntegrationPayloadRegistry) Disable(ctx context.Context, id types.ID) error {
	return nil
}

func (r *mockIntegrationPayloadRegistry) Enable(ctx context.Context, id types.ID) error {
	return nil
}

func (r *mockIntegrationPayloadRegistry) GetByCategory(ctx context.Context, category payload.PayloadCategory) ([]*payload.Payload, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *mockIntegrationPayloadRegistry) GetByMitreTechnique(ctx context.Context, technique string) ([]*payload.Payload, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *mockIntegrationPayloadRegistry) LoadBuiltIns(ctx context.Context) error {
	return nil
}

func (r *mockIntegrationPayloadRegistry) Count(ctx context.Context, filter *payload.PayloadFilter) (int, error) {
	return len(r.payloads), nil
}

func (r *mockIntegrationPayloadRegistry) ClearCache() {}

func (r *mockIntegrationPayloadRegistry) Health(ctx context.Context) types.HealthStatus {
	return types.Healthy("mock registry")
}

type mockIntegrationMissionStore struct {
	missions  map[types.ID]*mission.Mission
	saveCount int
	mu        sync.Mutex
}

func newMockIntegrationMissionStore() *mockIntegrationMissionStore {
	return &mockIntegrationMissionStore{
		missions: make(map[types.ID]*mission.Mission),
	}
}

func (s *mockIntegrationMissionStore) Save(ctx context.Context, m *mission.Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.missions[m.ID] = m
	s.saveCount++
	return nil
}

func (s *mockIntegrationMissionStore) Get(ctx context.Context, id types.ID) (*mission.Mission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.missions[id]; ok {
		return m, nil
	}
	return nil, fmt.Errorf("mission not found")
}

func (s *mockIntegrationMissionStore) Update(ctx context.Context, m *mission.Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.missions[m.ID] = m
	return nil
}

func (s *mockIntegrationMissionStore) List(ctx context.Context, filter *mission.MissionFilter) ([]*mission.Mission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*mission.Mission
	for _, m := range s.missions {
		result = append(result, m)
	}
	return result, nil
}

func (s *mockIntegrationMissionStore) Delete(ctx context.Context, id types.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.missions, id)
	return nil
}

func (s *mockIntegrationMissionStore) Count(ctx context.Context, filter *mission.MissionFilter) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.missions), nil
}

func (s *mockIntegrationMissionStore) GetActive(ctx context.Context) ([]*mission.Mission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*mission.Mission
	for _, m := range s.missions {
		if m.Status == mission.MissionStatusRunning || m.Status == mission.MissionStatusPaused {
			result = append(result, m)
		}
	}
	return result, nil
}

func (s *mockIntegrationMissionStore) GetByTarget(ctx context.Context, targetID types.ID) ([]*mission.Mission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*mission.Mission
	for _, m := range s.missions {
		if m.TargetID == targetID {
			result = append(result, m)
		}
	}
	return result, nil
}

func (s *mockIntegrationMissionStore) SaveCheckpoint(ctx context.Context, missionID types.ID, checkpoint *mission.MissionCheckpoint) error {
	return nil
}

func (s *mockIntegrationMissionStore) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.missions = make(map[types.ID]*mission.Mission)
	s.saveCount = 0
}

type mockIntegrationFindingStore struct {
	findings   map[types.ID]finding.EnhancedFinding
	storeCount int
	mu         sync.Mutex
}

func newMockIntegrationFindingStore() *mockIntegrationFindingStore {
	return &mockIntegrationFindingStore{
		findings:   make(map[types.ID]finding.EnhancedFinding),
		storeCount: 0,
	}
}

func (s *mockIntegrationFindingStore) Store(ctx context.Context, f finding.EnhancedFinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.findings[f.ID] = f
	s.storeCount++
	return nil
}

func (s *mockIntegrationFindingStore) Get(ctx context.Context, id types.ID) (*finding.EnhancedFinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.findings[id]; ok {
		return &f, nil
	}
	// Auto-create a finding for testing
	f := finding.EnhancedFinding{
		Finding: agent.Finding{
			ID:          id,
			Title:       "Mock Finding",
			Description: "This is a test finding",
			Severity:    agent.SeverityMedium,
			Category:    "Test",
			Confidence:  0.9,
			CreatedAt:   time.Now(),
		},
		MissionID:   types.NewID(),
		AgentName:   "test-agent",
		Subcategory: "Test",
		Status:      finding.StatusConfirmed,
		RiskScore:   5.0,
	}
	s.findings[id] = f
	return &f, nil
}

func (s *mockIntegrationFindingStore) List(ctx context.Context, missionID types.ID, filter *finding.FindingFilter) ([]finding.EnhancedFinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []finding.EnhancedFinding
	for _, f := range s.findings {
		if f.MissionID == missionID || missionID == "" {
			result = append(result, f)
		}
	}
	return result, nil
}

func (s *mockIntegrationFindingStore) Update(ctx context.Context, f finding.EnhancedFinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.findings[f.ID] = f
	return nil
}

func (s *mockIntegrationFindingStore) Delete(ctx context.Context, id types.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.findings, id)
	return nil
}

func (s *mockIntegrationFindingStore) Count(ctx context.Context, missionID types.ID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, f := range s.findings {
		if f.MissionID == missionID {
			count++
		}
	}
	return count, nil
}

func (s *mockIntegrationFindingStore) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.findings = make(map[types.ID]finding.EnhancedFinding)
	s.storeCount = 0
}

type mockIntegrationOrchestrator struct {
	returnFindings bool
	delay          time.Duration
}

func newMockIntegrationOrchestrator(returnFindings bool) *mockIntegrationOrchestrator {
	return &mockIntegrationOrchestrator{
		returnFindings: returnFindings,
	}
}

func newMockIntegrationOrchestratorWithDelay(delay time.Duration) *mockIntegrationOrchestrator {
	return &mockIntegrationOrchestrator{
		returnFindings: false,
		delay:          delay,
	}
}

func (o *mockIntegrationOrchestrator) Execute(ctx context.Context, m *mission.Mission) (*mission.MissionResult, error) {
	// Simulate execution delay
	if o.delay > 0 {
		select {
		case <-time.After(o.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	result := &mission.MissionResult{
		MissionID: m.ID,
		Status:    mission.MissionStatusCompleted,
		Metrics: &mission.MissionMetrics{
			CompletedNodes: 1,
			TotalTokens:    1000,
		},
	}

	// Add findings if requested
	if o.returnFindings {
		findingID := types.NewID()
		result.FindingIDs = []types.ID{findingID}
	}

	return result, nil
}
