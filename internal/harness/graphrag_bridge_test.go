package harness

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockGraphRAGStore is a mock implementation of graphrag.GraphRAGStore for testing.
type mockGraphRAGStore struct {
	mu                  sync.Mutex
	storedFindings      []graphrag.FindingNode
	storedRecords       []graphrag.GraphRecord
	storeError          error
	storeFindingError   error
	similarFindings     []graphrag.FindingNode
	findSimilarError    error
	storeFindingCalls   int
	findSimilarCalls    int
	storeDelay          time.Duration
	healthStatus        types.HealthStatus
}

func newMockGraphRAGStore() *mockGraphRAGStore {
	return &mockGraphRAGStore{
		storedFindings:  make([]graphrag.FindingNode, 0),
		storedRecords:   make([]graphrag.GraphRecord, 0),
		similarFindings: make([]graphrag.FindingNode, 0),
		healthStatus:    types.Healthy("mock store healthy"),
	}
}

func (m *mockGraphRAGStore) Store(ctx context.Context, record graphrag.GraphRecord) error {
	if m.storeDelay > 0 {
		time.Sleep(m.storeDelay)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.storeError != nil {
		return m.storeError
	}
	m.storedRecords = append(m.storedRecords, record)
	return nil
}

func (m *mockGraphRAGStore) StoreBatch(ctx context.Context, records []graphrag.GraphRecord) error {
	for _, r := range records {
		if err := m.Store(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockGraphRAGStore) Query(ctx context.Context, query graphrag.GraphRAGQuery) ([]graphrag.GraphRAGResult, error) {
	return nil, nil
}

func (m *mockGraphRAGStore) StoreAttackPattern(ctx context.Context, pattern graphrag.AttackPattern) error {
	return nil
}

func (m *mockGraphRAGStore) FindSimilarAttacks(ctx context.Context, content string, topK int) ([]graphrag.AttackPattern, error) {
	return nil, nil
}

func (m *mockGraphRAGStore) FindSimilarFindings(ctx context.Context, findingID string, topK int) ([]graphrag.FindingNode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.findSimilarCalls++
	if m.findSimilarError != nil {
		return nil, m.findSimilarError
	}
	return m.similarFindings, nil
}

func (m *mockGraphRAGStore) GetAttackChains(ctx context.Context, techniqueID string, maxDepth int) ([]graphrag.AttackChain, error) {
	return nil, nil
}

func (m *mockGraphRAGStore) StoreFinding(ctx context.Context, finding graphrag.FindingNode) error {
	if m.storeDelay > 0 {
		time.Sleep(m.storeDelay)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeFindingCalls++
	if m.storeFindingError != nil {
		return m.storeFindingError
	}
	m.storedFindings = append(m.storedFindings, finding)
	return nil
}

func (m *mockGraphRAGStore) GetRelatedFindings(ctx context.Context, findingID string) ([]graphrag.FindingNode, error) {
	return nil, nil
}

func (m *mockGraphRAGStore) Health(ctx context.Context) types.HealthStatus {
	return m.healthStatus
}

func (m *mockGraphRAGStore) Close() error {
	return nil
}

// Verify mockGraphRAGStore implements graphrag.GraphRAGStore
var _ graphrag.GraphRAGStore = (*mockGraphRAGStore)(nil)

// TestConvertToFindingNode tests the conversion from agent.Finding to graphrag.FindingNode.
func TestConvertToFindingNode(t *testing.T) {
	store := newMockGraphRAGStore()
	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	missionID := types.NewID()
	targetID := types.NewID()

	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "SQL Injection Vulnerability",
		Description: "Found SQL injection in login form",
		Severity:    agent.SeverityHigh,
		Category:    "injection",
		Confidence:  0.95,
		TargetID:    &targetID,
		CWE:         []string{"CWE-89"},
		CreatedAt:   time.Now(),
	}

	node := bridge.convertToFindingNode(finding, missionID)

	// Verify all fields are mapped correctly
	if node.ID != types.ID(finding.ID) {
		t.Errorf("ID mismatch: got %v, want %v", node.ID, finding.ID)
	}
	if node.Title != finding.Title {
		t.Errorf("Title mismatch: got %v, want %v", node.Title, finding.Title)
	}
	if node.Description != finding.Description {
		t.Errorf("Description mismatch: got %v, want %v", node.Description, finding.Description)
	}
	if node.Severity != string(finding.Severity) {
		t.Errorf("Severity mismatch: got %v, want %v", node.Severity, finding.Severity)
	}
	if node.Category != finding.Category {
		t.Errorf("Category mismatch: got %v, want %v", node.Category, finding.Category)
	}
	if node.Confidence != finding.Confidence {
		t.Errorf("Confidence mismatch: got %v, want %v", node.Confidence, finding.Confidence)
	}
	if node.MissionID != missionID {
		t.Errorf("MissionID mismatch: got %v, want %v", node.MissionID, missionID)
	}
	if node.TargetID == nil || *node.TargetID != targetID {
		t.Errorf("TargetID mismatch: got %v, want %v", node.TargetID, &targetID)
	}
}

// TestDefaultGraphRAGBridge_StoreAsync tests that StoreAsync calls the underlying store.
func TestDefaultGraphRAGBridge_StoreAsync(t *testing.T) {
	store := newMockGraphRAGStore()
	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	ctx := context.Background()
	missionID := types.NewID()

	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "Test Finding",
		Description: "Test description",
		Severity:    agent.SeverityMedium,
		CreatedAt:   time.Now(),
	}

	// Call StoreAsync
	bridge.StoreAsync(ctx, finding, missionID, nil)

	// Wait for async operation to complete
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := bridge.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Verify StoreFinding was called
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.storeFindingCalls != 1 {
		t.Errorf("Expected 1 StoreFinding call, got %d", store.storeFindingCalls)
	}
	if len(store.storedFindings) != 1 {
		t.Errorf("Expected 1 stored finding, got %d", len(store.storedFindings))
	}
}

// TestDefaultGraphRAGBridge_Concurrency tests that the semaphore limits concurrent operations.
func TestDefaultGraphRAGBridge_Concurrency(t *testing.T) {
	store := newMockGraphRAGStore()
	store.storeDelay = 50 * time.Millisecond // Add delay to make concurrency observable

	config := DefaultGraphRAGBridgeConfig()
	config.MaxConcurrent = 2 // Allow only 2 concurrent operations
	bridge := NewGraphRAGBridge(store, nil, config)

	ctx := context.Background()
	missionID := types.NewID()

	// Submit 10 findings
	for i := 0; i < 10; i++ {
		finding := agent.Finding{
			ID:          types.NewID(),
			Title:       "Test Finding",
			Description: "Test description",
			Severity:    agent.SeverityLow,
			CreatedAt:   time.Now(),
		}
		bridge.StoreAsync(ctx, finding, missionID, nil)
	}

	// Wait for all operations to complete
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := bridge.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Verify all findings were stored
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.storeFindingCalls != 10 {
		t.Errorf("Expected 10 StoreFinding calls, got %d", store.storeFindingCalls)
	}
}

// TestDefaultGraphRAGBridge_Shutdown tests that Shutdown waits for pending operations.
func TestDefaultGraphRAGBridge_Shutdown(t *testing.T) {
	store := newMockGraphRAGStore()
	store.storeDelay = 100 * time.Millisecond // Add delay

	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	ctx := context.Background()
	missionID := types.NewID()

	// Submit a finding
	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "Test Finding",
		Description: "Test description",
		Severity:    agent.SeverityLow,
		CreatedAt:   time.Now(),
	}
	bridge.StoreAsync(ctx, finding, missionID, nil)

	// Shutdown should wait for the operation to complete
	start := time.Now()
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := bridge.Shutdown(shutdownCtx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Shutdown should have waited at least the store delay
	if elapsed < store.storeDelay {
		t.Errorf("Shutdown returned too quickly: %v < %v", elapsed, store.storeDelay)
	}

	// Verify finding was stored
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.storeFindingCalls != 1 {
		t.Errorf("Expected 1 StoreFinding call, got %d", store.storeFindingCalls)
	}
}

// TestDefaultGraphRAGBridge_ShutdownTimeout tests that Shutdown respects context timeout.
func TestDefaultGraphRAGBridge_ShutdownTimeout(t *testing.T) {
	store := newMockGraphRAGStore()
	store.storeDelay = 1 * time.Second // Long delay

	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	ctx := context.Background()
	missionID := types.NewID()

	// Submit a finding
	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "Test Finding",
		Description: "Test description",
		Severity:    agent.SeverityLow,
		CreatedAt:   time.Now(),
	}
	bridge.StoreAsync(ctx, finding, missionID, nil)

	// Shutdown with short timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	err := bridge.Shutdown(shutdownCtx)

	// Should timeout
	if err == nil {
		t.Error("Expected Shutdown to timeout, but it succeeded")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected DeadlineExceeded error, got: %v", err)
	}
}

// TestDefaultGraphRAGBridge_ErrorHandling tests that store errors are logged and don't crash.
func TestDefaultGraphRAGBridge_ErrorHandling(t *testing.T) {
	store := newMockGraphRAGStore()
	store.storeFindingError = errors.New("mock storage error")

	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(store, nil, config)

	ctx := context.Background()
	missionID := types.NewID()

	// Submit a finding that will fail
	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "Test Finding",
		Description: "Test description",
		Severity:    agent.SeverityLow,
		CreatedAt:   time.Now(),
	}
	bridge.StoreAsync(ctx, finding, missionID, nil)

	// Shutdown should complete without error (errors are logged, not propagated)
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := bridge.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Verify StoreFinding was called (even though it failed)
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.storeFindingCalls != 1 {
		t.Errorf("Expected 1 StoreFinding call, got %d", store.storeFindingCalls)
	}
}

// TestDefaultGraphRAGBridge_SimilarityDetection tests that SIMILAR_TO relationships are created.
func TestDefaultGraphRAGBridge_SimilarityDetection(t *testing.T) {
	store := newMockGraphRAGStore()

	// Add similar findings that should be linked
	similarID := types.NewID()
	store.similarFindings = []graphrag.FindingNode{
		{
			ID:          similarID,
			Title:       "Similar Finding",
			Description: "Similar description",
			MissionID:   types.NewID(),
		},
	}

	config := DefaultGraphRAGBridgeConfig()
	config.MaxSimilarLinks = 5
	bridge := NewGraphRAGBridge(store, nil, config)

	ctx := context.Background()
	missionID := types.NewID()

	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "Test Finding",
		Description: "Test description",
		Severity:    agent.SeverityMedium,
		CreatedAt:   time.Now(),
	}

	bridge.StoreAsync(ctx, finding, missionID, nil)

	// Wait for async operation
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := bridge.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Verify FindSimilarFindings was called
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.findSimilarCalls != 1 {
		t.Errorf("Expected 1 FindSimilarFindings call, got %d", store.findSimilarCalls)
	}

	// Verify SIMILAR_TO relationship was created (stored as a record)
	if len(store.storedRecords) != 1 {
		t.Errorf("Expected 1 stored record (SIMILAR_TO relationship), got %d", len(store.storedRecords))
	}
}

// TestNoopGraphRAGBridge tests that NoopGraphRAGBridge does nothing.
func TestNoopGraphRAGBridge(t *testing.T) {
	bridge := &NoopGraphRAGBridge{}

	ctx := context.Background()
	missionID := types.NewID()

	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "Test Finding",
		Description: "Test description",
		Severity:    agent.SeverityLow,
		CreatedAt:   time.Now(),
	}

	// StoreAsync should not panic
	bridge.StoreAsync(ctx, finding, missionID, nil)

	// Shutdown should return nil
	err := bridge.Shutdown(ctx)
	if err != nil {
		t.Errorf("Expected Shutdown to return nil, got: %v", err)
	}

	// Health should return healthy
	health := bridge.Health(ctx)
	if !health.IsHealthy() {
		t.Errorf("Expected healthy status, got: %v", health.State)
	}
}

// TestDefaultGraphRAGBridge_Disabled tests that disabled bridge is a no-op.
func TestDefaultGraphRAGBridge_Disabled(t *testing.T) {
	store := newMockGraphRAGStore()

	config := DefaultGraphRAGBridgeConfig()
	config.Enabled = false
	bridge := NewGraphRAGBridge(store, nil, config)

	ctx := context.Background()
	missionID := types.NewID()

	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "Test Finding",
		Description: "Test description",
		Severity:    agent.SeverityLow,
		CreatedAt:   time.Now(),
	}

	// StoreAsync should not call the store when disabled
	bridge.StoreAsync(ctx, finding, missionID, nil)

	// Small delay to ensure any async work would have started
	time.Sleep(10 * time.Millisecond)

	// Verify no calls were made
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.storeFindingCalls != 0 {
		t.Errorf("Expected 0 StoreFinding calls when disabled, got %d", store.storeFindingCalls)
	}
}

// TestGraphRAGBridgeConfig_Validate tests configuration validation.
func TestGraphRAGBridgeConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    GraphRAGBridgeConfig
		wantError bool
	}{
		{
			name:      "valid defaults",
			config:    DefaultGraphRAGBridgeConfig(),
			wantError: false,
		},
		{
			name: "invalid similarity threshold low",
			config: GraphRAGBridgeConfig{
				SimilarityThreshold: -0.1,
				MaxSimilarLinks:     5,
				MaxConcurrent:       10,
				StorageTimeout:      30 * time.Second,
			},
			wantError: true,
		},
		{
			name: "invalid similarity threshold high",
			config: GraphRAGBridgeConfig{
				SimilarityThreshold: 1.5,
				MaxSimilarLinks:     5,
				MaxConcurrent:       10,
				StorageTimeout:      30 * time.Second,
			},
			wantError: true,
		},
		{
			name: "invalid max concurrent",
			config: GraphRAGBridgeConfig{
				SimilarityThreshold: 0.85,
				MaxSimilarLinks:     5,
				MaxConcurrent:       0,
				StorageTimeout:      30 * time.Second,
			},
			wantError: true,
		},
		{
			name: "invalid storage timeout",
			config: GraphRAGBridgeConfig{
				SimilarityThreshold: 0.85,
				MaxSimilarLinks:     5,
				MaxConcurrent:       10,
				StorageTimeout:      500 * time.Millisecond,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}
