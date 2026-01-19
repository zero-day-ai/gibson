package harness

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/graphrag/engine"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockTaxonomyGraphEngine is a mock implementation of engine.TaxonomyGraphEngine for testing.
type mockTaxonomyGraphEngine struct {
	mu                 sync.Mutex
	handleFindingCalls int
	handleFindingError error
	handleEventCalls   int
	handleToolCalls    int
	storeDelay         time.Duration
	healthStatus       engine.HealthStatus
	storedFindings     []agent.Finding
}

func newMockTaxonomyGraphEngine() *mockTaxonomyGraphEngine {
	return &mockTaxonomyGraphEngine{
		storedFindings: make([]agent.Finding, 0),
		healthStatus: engine.HealthStatus{
			Healthy:     true,
			Neo4jStatus: "healthy",
			Message:     "mock engine healthy",
		},
	}
}

func (m *mockTaxonomyGraphEngine) HandleEvent(ctx context.Context, eventType string, data map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handleEventCalls++
	return nil
}

func (m *mockTaxonomyGraphEngine) HandleToolOutput(ctx context.Context, toolName string, output map[string]any, agentRunID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handleToolCalls++
	return nil
}

func (m *mockTaxonomyGraphEngine) HandleFinding(ctx context.Context, finding agent.Finding, missionID string) error {
	if m.storeDelay > 0 {
		time.Sleep(m.storeDelay)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handleFindingCalls++
	if m.handleFindingError != nil {
		return m.handleFindingError
	}
	m.storedFindings = append(m.storedFindings, finding)
	return nil
}

func (m *mockTaxonomyGraphEngine) Health(ctx context.Context) engine.HealthStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.healthStatus
}

// Verify mockTaxonomyGraphEngine implements engine.TaxonomyGraphEngine
var _ engine.TaxonomyGraphEngine = (*mockTaxonomyGraphEngine)(nil)

// TestDefaultGraphRAGBridge_StoreAsync tests that StoreAsync calls the underlying engine.
func TestDefaultGraphRAGBridge_StoreAsync(t *testing.T) {
	mockEngine := newMockTaxonomyGraphEngine()
	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(mockEngine, nil, config)

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

	// Verify HandleFinding was called
	mockEngine.mu.Lock()
	defer mockEngine.mu.Unlock()
	if mockEngine.handleFindingCalls != 1 {
		t.Errorf("Expected 1 HandleFinding call, got %d", mockEngine.handleFindingCalls)
	}
	if len(mockEngine.storedFindings) != 1 {
		t.Errorf("Expected 1 stored finding, got %d", len(mockEngine.storedFindings))
	}
}

// TestDefaultGraphRAGBridge_Concurrency tests that the semaphore limits concurrent operations.
func TestDefaultGraphRAGBridge_Concurrency(t *testing.T) {
	mockEngine := newMockTaxonomyGraphEngine()
	mockEngine.storeDelay = 50 * time.Millisecond // Add delay to make concurrency observable

	config := DefaultGraphRAGBridgeConfig()
	config.MaxConcurrent = 2 // Allow only 2 concurrent operations
	bridge := NewGraphRAGBridge(mockEngine, nil, config)

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
	mockEngine.mu.Lock()
	defer mockEngine.mu.Unlock()
	if mockEngine.handleFindingCalls != 10 {
		t.Errorf("Expected 10 HandleFinding calls, got %d", mockEngine.handleFindingCalls)
	}
}

// TestDefaultGraphRAGBridge_Shutdown tests that Shutdown waits for pending operations.
func TestDefaultGraphRAGBridge_Shutdown(t *testing.T) {
	mockEngine := newMockTaxonomyGraphEngine()
	mockEngine.storeDelay = 100 * time.Millisecond // Add delay

	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(mockEngine, nil, config)

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
	if elapsed < mockEngine.storeDelay {
		t.Errorf("Shutdown returned too quickly: %v < %v", elapsed, mockEngine.storeDelay)
	}

	// Verify finding was stored
	mockEngine.mu.Lock()
	defer mockEngine.mu.Unlock()
	if mockEngine.handleFindingCalls != 1 {
		t.Errorf("Expected 1 HandleFinding call, got %d", mockEngine.handleFindingCalls)
	}
}

// TestDefaultGraphRAGBridge_ShutdownTimeout tests that Shutdown respects context timeout.
func TestDefaultGraphRAGBridge_ShutdownTimeout(t *testing.T) {
	mockEngine := newMockTaxonomyGraphEngine()
	mockEngine.storeDelay = 1 * time.Second // Long delay

	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(mockEngine, nil, config)

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

// TestDefaultGraphRAGBridge_ErrorHandling tests that engine errors are logged and don't crash.
func TestDefaultGraphRAGBridge_ErrorHandling(t *testing.T) {
	mockEngine := newMockTaxonomyGraphEngine()
	mockEngine.handleFindingError = errors.New("mock storage error")

	config := DefaultGraphRAGBridgeConfig()
	bridge := NewGraphRAGBridge(mockEngine, nil, config)

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

	// Verify HandleFinding was called (even though it failed)
	mockEngine.mu.Lock()
	defer mockEngine.mu.Unlock()
	if mockEngine.handleFindingCalls != 1 {
		t.Errorf("Expected 1 HandleFinding call, got %d", mockEngine.handleFindingCalls)
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
