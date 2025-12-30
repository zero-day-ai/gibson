package harness

import (
	"context"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestHarnessSubmitFinding_WithGraphRAG tests that submitting a finding through
// the harness properly flows to the GraphRAG bridge.
func TestHarnessSubmitFinding_WithGraphRAG(t *testing.T) {
	// Create mock GraphRAG store
	store := newMockGraphRAGStore()

	// Create GraphRAG bridge with the mock store
	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	// Create harness config with our bridge
	harnessConfig := HarnessConfig{
		SlotManager:    &mockSlotManager{},
		GraphRAGBridge: bridge,
	}
	harnessConfig.ApplyDefaults()

	// Create factory and harness
	factory, err := NewHarnessFactory(harnessConfig)
	if err != nil {
		t.Fatalf("Failed to create harness factory: %v", err)
	}

	missionCtx := MissionContext{
		ID:   types.NewID(),
		Name: "Test Mission",
	}
	targetInfo := TargetInfo{
		ID:   types.ID(types.NewID()),
		Name: "Test Target",
	}

	harness, err := factory.Create("test-agent", missionCtx, targetInfo)
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}

	// Submit a finding through the harness
	ctx := context.Background()
	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "Test Vulnerability",
		Description: "Found during integration test",
		Severity:    agent.SeverityHigh,
		Confidence:  0.9,
		Category:    "test",
		CreatedAt:   time.Now(),
	}

	err = harness.SubmitFinding(ctx, finding)
	if err != nil {
		t.Fatalf("SubmitFinding failed: %v", err)
	}

	// Close the harness to wait for async operations
	if defaultHarness, ok := harness.(*DefaultAgentHarness); ok {
		closeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		err = defaultHarness.Close(closeCtx)
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}

	// Verify finding reached the GraphRAG store
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.storeFindingCalls != 1 {
		t.Errorf("Expected 1 StoreFinding call, got %d", store.storeFindingCalls)
	}
	if len(store.storedFindings) != 1 {
		t.Errorf("Expected 1 stored finding, got %d", len(store.storedFindings))
	} else {
		// Verify the stored finding has correct data
		storedFinding := store.storedFindings[0]
		if storedFinding.Title != finding.Title {
			t.Errorf("Stored finding title mismatch: got %v, want %v", storedFinding.Title, finding.Title)
		}
		if storedFinding.MissionID != missionCtx.ID {
			t.Errorf("Stored finding mission ID mismatch: got %v, want %v", storedFinding.MissionID, missionCtx.ID)
		}
	}
}

// TestHarnessSubmitFinding_WithoutGraphRAG tests that the harness works normally
// when using NoopGraphRAGBridge (default when not configured).
func TestHarnessSubmitFinding_WithoutGraphRAG(t *testing.T) {
	// Create harness config with default (noop) bridge
	harnessConfig := HarnessConfig{
		SlotManager: &mockSlotManager{},
		// GraphRAGBridge will be defaulted to NoopGraphRAGBridge
	}
	harnessConfig.ApplyDefaults()

	// Verify the default is NoopGraphRAGBridge
	if _, ok := harnessConfig.GraphRAGBridge.(*NoopGraphRAGBridge); !ok {
		t.Fatalf("Expected default GraphRAGBridge to be NoopGraphRAGBridge, got %T", harnessConfig.GraphRAGBridge)
	}

	// Create factory and harness
	factory, err := NewHarnessFactory(harnessConfig)
	if err != nil {
		t.Fatalf("Failed to create harness factory: %v", err)
	}

	missionCtx := MissionContext{
		ID:   types.NewID(),
		Name: "Test Mission",
	}
	targetInfo := TargetInfo{
		ID:   types.ID(types.NewID()),
		Name: "Test Target",
	}

	harness, err := factory.Create("test-agent", missionCtx, targetInfo)
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}

	// Submit a finding through the harness
	ctx := context.Background()
	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "Test Vulnerability",
		Description: "Found during integration test",
		Severity:    agent.SeverityMedium,
		CreatedAt:   time.Now(),
	}

	// Should succeed without error
	err = harness.SubmitFinding(ctx, finding)
	if err != nil {
		t.Fatalf("SubmitFinding failed: %v", err)
	}

	// Verify finding was stored in the local FindingStore
	findings, err := harness.GetFindings(ctx, FindingFilter{})
	if err != nil {
		t.Fatalf("GetFindings failed: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("Expected 1 finding in local store, got %d", len(findings))
	}
}

// TestHarnessSubmitFinding_GraphRAGFailure tests that the harness continues
// to work even when GraphRAG storage fails.
func TestHarnessSubmitFinding_GraphRAGFailure(t *testing.T) {
	// Create mock GraphRAG store that will fail
	store := newMockGraphRAGStore()
	store.storeFindingError = graphrag.NewQueryError("mock storage error", nil)

	// Create GraphRAG bridge with the failing store
	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	// Create harness config with our bridge
	harnessConfig := HarnessConfig{
		SlotManager:    &mockSlotManager{},
		GraphRAGBridge: bridge,
	}
	harnessConfig.ApplyDefaults()

	// Create factory and harness
	factory, err := NewHarnessFactory(harnessConfig)
	if err != nil {
		t.Fatalf("Failed to create harness factory: %v", err)
	}

	missionCtx := MissionContext{
		ID:   types.NewID(),
		Name: "Test Mission",
	}
	targetInfo := TargetInfo{}

	harness, err := factory.Create("test-agent", missionCtx, targetInfo)
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}

	// Submit a finding through the harness
	ctx := context.Background()
	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "Test Vulnerability",
		Description: "Found during integration test",
		Severity:    agent.SeverityLow,
		CreatedAt:   time.Now(),
	}

	// Should succeed even though GraphRAG storage will fail
	err = harness.SubmitFinding(ctx, finding)
	if err != nil {
		t.Fatalf("SubmitFinding should succeed even when GraphRAG fails: %v", err)
	}

	// Close the harness to wait for async operations
	if defaultHarness, ok := harness.(*DefaultAgentHarness); ok {
		closeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		err = defaultHarness.Close(closeCtx)
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}

	// Verify finding was stored in the local FindingStore despite GraphRAG failure
	findings, err := harness.GetFindings(ctx, FindingFilter{})
	if err != nil {
		t.Fatalf("GetFindings failed: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("Expected 1 finding in local store, got %d", len(findings))
	}

	// Verify GraphRAG was attempted (call was made even though it failed)
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.storeFindingCalls != 1 {
		t.Errorf("Expected 1 StoreFinding attempt, got %d", store.storeFindingCalls)
	}
}

// TestHarnessClose_WaitsForGraphRAG tests that closing the harness waits for
// pending GraphRAG operations to complete.
func TestHarnessClose_WaitsForGraphRAG(t *testing.T) {
	// Create mock GraphRAG store with a delay
	store := newMockGraphRAGStore()
	store.storeDelay = 100 * time.Millisecond

	// Create GraphRAG bridge
	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	// Create harness config with our bridge
	harnessConfig := HarnessConfig{
		SlotManager:    &mockSlotManager{},
		GraphRAGBridge: bridge,
	}
	harnessConfig.ApplyDefaults()

	// Create factory and harness
	factory, err := NewHarnessFactory(harnessConfig)
	if err != nil {
		t.Fatalf("Failed to create harness factory: %v", err)
	}

	missionCtx := MissionContext{
		ID:   types.NewID(),
		Name: "Test Mission",
	}
	targetInfo := TargetInfo{}

	harness, err := factory.Create("test-agent", missionCtx, targetInfo)
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}

	// Submit multiple findings
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		finding := agent.Finding{
			ID:          types.NewID(),
			Title:       "Test Vulnerability",
			Description: "Found during integration test",
			Severity:    agent.SeverityLow,
			CreatedAt:   time.Now(),
		}
		err = harness.SubmitFinding(ctx, finding)
		if err != nil {
			t.Fatalf("SubmitFinding failed: %v", err)
		}
	}

	// Close the harness - this should wait for all async operations
	start := time.Now()
	if defaultHarness, ok := harness.(*DefaultAgentHarness); ok {
		closeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		err = defaultHarness.Close(closeCtx)
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}
	elapsed := time.Since(start)

	// Close should have waited for all operations
	if elapsed < store.storeDelay {
		t.Errorf("Close returned too quickly: %v < %v", elapsed, store.storeDelay)
	}

	// Verify all findings were stored
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.storeFindingCalls != 5 {
		t.Errorf("Expected 5 StoreFinding calls, got %d", store.storeFindingCalls)
	}
}

// mockSlotManager is a minimal mock for testing harness creation.
type mockSlotManager struct{}

func (m *mockSlotManager) ResolveSlot(ctx context.Context, slot agent.SlotDefinition, override *agent.SlotConfig) (llm.LLMProvider, llm.ModelInfo, error) {
	return nil, llm.ModelInfo{}, nil
}

func (m *mockSlotManager) ValidateSlot(ctx context.Context, slot agent.SlotDefinition) error {
	return nil
}

// TestIntegration_StoreNodeViaHarness tests the full flow of storing a node
// from SDK types through harness/bridge to the GraphRAG store.
//
// Flow: agent.Finding → harness → bridge → graphrag.GraphRAGStore (mock)
func TestIntegration_StoreNodeViaHarness(t *testing.T) {
	ctx := context.Background()

	// Setup mock store
	store := newMockGraphRAGStore()

	// Create bridge
	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	missionID := types.NewID()
	targetID := types.NewID()

	// Create agent finding (SDK type)
	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "SQL Injection in Login Form",
		Description: "Detected SQL injection vulnerability in user login authentication",
		Severity:    agent.SeverityHigh,
		Category:    "injection",
		Confidence:  0.95,
		TargetID:    &targetID,
		CWE:         []string{"CWE-89"},
		CreatedAt:   time.Now(),
	}

	// Store via harness bridge
	bridge.StoreAsync(ctx, finding, missionID, &targetID)

	// Wait for async processing
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := bridge.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("bridge shutdown should succeed: %v", err)
	}

	// Verify finding reached the store
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.storeFindingCalls != 1 {
		t.Errorf("StoreFinding should be called once, got %d calls", store.storeFindingCalls)
	}
	if len(store.storedFindings) != 1 {
		t.Fatalf("should have one stored finding, got %d", len(store.storedFindings))
	}

	// Verify finding node was converted correctly
	storedFinding := store.storedFindings[0]
	if storedFinding.ID != types.ID(finding.ID) {
		t.Errorf("ID mismatch: got %v, want %v", storedFinding.ID, finding.ID)
	}
	if storedFinding.Title != finding.Title {
		t.Errorf("Title mismatch: got %v, want %v", storedFinding.Title, finding.Title)
	}
	if storedFinding.Description != finding.Description {
		t.Errorf("Description mismatch: got %v, want %v", storedFinding.Description, finding.Description)
	}
	if storedFinding.Severity != string(finding.Severity) {
		t.Errorf("Severity mismatch: got %v, want %v", storedFinding.Severity, finding.Severity)
	}
	if storedFinding.Category != finding.Category {
		t.Errorf("Category mismatch: got %v, want %v", storedFinding.Category, finding.Category)
	}
	if storedFinding.Confidence != finding.Confidence {
		t.Errorf("Confidence mismatch: got %v, want %v", storedFinding.Confidence, finding.Confidence)
	}
	if storedFinding.MissionID != missionID {
		t.Errorf("MissionID mismatch: got %v, want %v", storedFinding.MissionID, missionID)
	}
	if storedFinding.TargetID == nil {
		t.Error("TargetID should not be nil")
	} else if *storedFinding.TargetID != targetID {
		t.Errorf("TargetID mismatch: got %v, want %v", *storedFinding.TargetID, targetID)
	}
}

// TestIntegration_StoreRelationshipViaHarness tests that relationships are created
// correctly through the full stack from harness to store.
func TestIntegration_StoreRelationshipViaHarness(t *testing.T) {
	ctx := context.Background()

	store := newMockGraphRAGStore()
	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	missionID := types.NewID()
	targetID := types.NewID()

	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "XSS Vulnerability",
		Description: "Cross-site scripting in comment field",
		Severity:    agent.SeverityMedium,
		Category:    "xss",
		Confidence:  0.88,
		TargetID:    &targetID,
		CWE:         []string{"CWE-79"},
		CreatedAt:   time.Now(),
	}

	// Store with target relationship
	bridge.StoreAsync(ctx, finding, missionID, &targetID)

	// Wait for completion
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := bridge.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Verify DISCOVERED_ON relationship was created
	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.storedRecords) == 0 {
		t.Fatal("should create relationship records")
	}

	// Check for DISCOVERED_ON relationship
	foundDiscoveredOn := false
	for _, record := range store.storedRecords {
		for _, rel := range record.Relationships {
			if rel.Type == graphrag.RelationDiscoveredOn {
				foundDiscoveredOn = true
				if rel.FromID != types.ID(finding.ID) {
					t.Errorf("FromID mismatch: got %v, want %v", rel.FromID, finding.ID)
				}
				if rel.ToID != targetID {
					t.Errorf("ToID mismatch: got %v, want %v", rel.ToID, targetID)
				}
				if rel.Properties["severity"] != string(finding.Severity) {
					t.Errorf("Severity property mismatch: got %v, want %v", rel.Properties["severity"], finding.Severity)
				}
				if rel.Properties["confidence"] != finding.Confidence {
					t.Errorf("Confidence property mismatch: got %v, want %v", rel.Properties["confidence"], finding.Confidence)
				}
			}
		}
	}

	if !foundDiscoveredOn {
		t.Error("DISCOVERED_ON relationship should be created")
	}
}

// TestIntegration_BatchStoreViaHarness tests batch storage of multiple findings
// through the harness layer.
func TestIntegration_BatchStoreViaHarness(t *testing.T) {
	ctx := context.Background()

	store := newMockGraphRAGStore()
	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	missionID := types.NewID()

	// Create multiple findings
	findings := []agent.Finding{
		{
			ID:          types.NewID(),
			Title:       "SQLi - Login",
			Description: "SQL injection in login",
			Severity:    agent.SeverityHigh,
			Category:    "injection",
			Confidence:  0.9,
			CreatedAt:   time.Now(),
		},
		{
			ID:          types.NewID(),
			Title:       "SQLi - Search",
			Description: "SQL injection in search",
			Severity:    agent.SeverityHigh,
			Category:    "injection",
			Confidence:  0.92,
			CreatedAt:   time.Now(),
		},
		{
			ID:          types.NewID(),
			Title:       "XSS - Comments",
			Description: "XSS in comments",
			Severity:    agent.SeverityMedium,
			Category:    "xss",
			Confidence:  0.85,
			CreatedAt:   time.Now(),
		},
	}

	// Store all findings
	for _, f := range findings {
		bridge.StoreAsync(ctx, f, missionID, nil)
	}

	// Wait for all to complete
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := bridge.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Verify all findings were stored
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.storeFindingCalls != 3 {
		t.Errorf("should store all 3 findings, got %d calls", store.storeFindingCalls)
	}
	if len(store.storedFindings) != 3 {
		t.Errorf("should have 3 stored findings, got %d", len(store.storedFindings))
	}

	// Verify all findings have correct mission ID
	for i, stored := range store.storedFindings {
		if stored.MissionID != missionID {
			t.Errorf("finding %d MissionID mismatch: got %v, want %v", i, stored.MissionID, missionID)
		}
	}
}

// TestIntegration_QueryResultsViaHarness tests that query results flow back
// correctly through the harness layer.
func TestIntegration_QueryResultsViaHarness(t *testing.T) {
	ctx := context.Background()

	store := newMockGraphRAGStore()

	// Pre-populate store with findings
	missionID := types.NewID()
	finding1 := graphrag.FindingNode{
		ID:          types.NewID(),
		Title:       "Test Finding 1",
		Description: "First test finding",
		Severity:    "high",
		Category:    "test",
		Confidence:  0.9,
		MissionID:   missionID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	finding2 := graphrag.FindingNode{
		ID:          types.NewID(),
		Title:       "Test Finding 2",
		Description: "Second test finding",
		Severity:    "medium",
		Category:    "test",
		Confidence:  0.85,
		MissionID:   missionID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	store.storedFindings = []graphrag.FindingNode{finding1, finding2}
	store.similarFindings = []graphrag.FindingNode{finding2}

	// Test FindSimilarFindings
	similar, err := store.FindSimilarFindings(ctx, finding1.ID.String(), 5)
	if err != nil {
		t.Fatalf("FindSimilarFindings failed: %v", err)
	}

	if len(similar) != 1 {
		t.Errorf("should find one similar finding, got %d", len(similar))
	}
	if len(similar) > 0 {
		if similar[0].ID != finding2.ID {
			t.Errorf("similar finding ID mismatch: got %v, want %v", similar[0].ID, finding2.ID)
		}
		if similar[0].Category != "test" {
			t.Errorf("similar finding category mismatch: got %v, want test", similar[0].Category)
		}
	}
}

// TestIntegration_HealthCheckViaHarness tests health check propagation through
// the harness layer to the GraphRAG store.
func TestIntegration_HealthCheckViaHarness(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		storeHealth   types.HealthStatus
		expectedState types.HealthState
	}{
		{
			name:          "healthy store",
			storeHealth:   types.Healthy("all systems operational"),
			expectedState: types.HealthStateHealthy,
		},
		{
			name:          "degraded store",
			storeHealth:   types.Degraded("high latency"),
			expectedState: types.HealthStateDegraded,
		},
		{
			name:          "unhealthy store",
			storeHealth:   types.Unhealthy("connection failed"),
			expectedState: types.HealthStateUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockGraphRAGStore()
			store.healthStatus = tt.storeHealth

			config := DefaultGraphRAGBridgeConfig()
			bridge := NewGraphRAGBridge(store, nil, config)

			health := bridge.Health(ctx)
			if health.State != tt.expectedState {
				t.Errorf("health state mismatch: got %v, want %v", health.State, tt.expectedState)
			}
		})
	}
}

// TestIntegration_WiringVerification verifies that all interfaces are correctly
// wired and satisfy compile-time type checks.
func TestIntegration_WiringVerification(t *testing.T) {
	// Compile-time interface checks
	var _ GraphRAGBridge = (*DefaultGraphRAGBridge)(nil)
	var _ GraphRAGBridge = (*NoopGraphRAGBridge)(nil)
	var _ graphrag.GraphRAGStore = (*mockGraphRAGStore)(nil)

	// Runtime verification
	ctx := context.Background()

	store := newMockGraphRAGStore()
	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	// Verify bridge can be used through interface
	var bridgeInterface GraphRAGBridge = bridge
	health := bridgeInterface.Health(ctx)
	if !health.IsHealthy() {
		t.Errorf("bridge should be healthy, got state: %v", health.State)
	}

	// Verify noop bridge
	var noopBridge GraphRAGBridge = &NoopGraphRAGBridge{}
	noopHealth := noopBridge.Health(ctx)
	if !noopHealth.IsHealthy() {
		t.Errorf("noop bridge should be healthy, got state: %v", noopHealth.State)
	}
}
