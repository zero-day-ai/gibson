package harness

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/zero-day-ai/gibson/internal/types"
	sdkgraphrag "github.com/zero-day-ai/sdk/graphrag"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestGraphRAGHealth_Disabled verifies that when GraphRAG is not configured,
// the health check returns "healthy" with message "graphrag disabled" instead
// of returning "unhealthy". This fix addresses the issue where external agents
// report GraphRAG as unhealthy even though it's intentionally disabled.
func TestGraphRAGHealth_Disabled(t *testing.T) {
	// Create a minimal harness without GraphRAG configured
	h := &DefaultAgentHarness{
		graphRAGQueryBridge: nil, // GraphRAG not configured
		logger:              slog.New(slog.NewTextHandler(os.Stdout, nil)),
		tracer:              noop.NewTracerProvider().Tracer("test"),
	}

	ctx := context.Background()
	status := h.GraphRAGHealth(ctx)

	// Verify it returns healthy (not unhealthy) when disabled
	if status.State != types.HealthStateHealthy {
		t.Errorf("expected health state to be Healthy, got %v", status.State)
	}

	// Verify the message indicates GraphRAG is disabled
	expectedMsg := "graphrag disabled"
	if status.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, status.Message)
	}
}

// TestGraphRAGHealth_Enabled verifies that when GraphRAG is configured,
// the health check delegates to the query bridge's health check.
func TestGraphRAGHealth_Enabled(t *testing.T) {
	// Create a mock query bridge that returns a specific health status
	mockBridge := &mockGraphRAGQueryBridge{
		healthStatus: types.Healthy("graphrag operational"),
	}

	h := &DefaultAgentHarness{
		graphRAGQueryBridge: mockBridge,
		logger:              slog.New(slog.NewTextHandler(os.Stdout, nil)),
		tracer:              noop.NewTracerProvider().Tracer("test"),
	}

	ctx := context.Background()
	status := h.GraphRAGHealth(ctx)

	// Verify it delegates to the bridge
	if !mockBridge.healthCalled {
		t.Error("expected Health() to be called on query bridge")
	}

	// Verify the status is what the bridge returned
	if status.State != types.HealthStateHealthy {
		t.Errorf("expected health state to be Healthy, got %v", status.State)
	}

	if status.Message != "graphrag operational" {
		t.Errorf("expected message %q, got %q", "graphrag operational", status.Message)
	}
}

// mockGraphRAGQueryBridge is a test double for GraphRAGQueryBridge
type mockGraphRAGQueryBridge struct {
	healthStatus types.HealthStatus
	healthCalled bool
}

func (m *mockGraphRAGQueryBridge) Query(ctx context.Context, query sdkgraphrag.Query) ([]sdkgraphrag.Result, error) {
	return nil, nil
}

func (m *mockGraphRAGQueryBridge) FindSimilarAttacks(ctx context.Context, content string, topK int) ([]sdkgraphrag.AttackPattern, error) {
	return nil, nil
}

func (m *mockGraphRAGQueryBridge) FindSimilarFindings(ctx context.Context, findingID string, topK int) ([]sdkgraphrag.FindingNode, error) {
	return nil, nil
}

func (m *mockGraphRAGQueryBridge) GetAttackChains(ctx context.Context, techniqueID string, maxDepth int) ([]sdkgraphrag.AttackChain, error) {
	return nil, nil
}

func (m *mockGraphRAGQueryBridge) GetRelatedFindings(ctx context.Context, findingID string) ([]sdkgraphrag.FindingNode, error) {
	return nil, nil
}

func (m *mockGraphRAGQueryBridge) StoreNode(ctx context.Context, node sdkgraphrag.GraphNode, missionID, agentName string) (string, error) {
	return "", nil
}

func (m *mockGraphRAGQueryBridge) CreateRelationship(ctx context.Context, rel sdkgraphrag.Relationship) error {
	return nil
}

func (m *mockGraphRAGQueryBridge) StoreBatch(ctx context.Context, batch sdkgraphrag.Batch, missionID, agentName string) ([]string, error) {
	return nil, nil
}

func (m *mockGraphRAGQueryBridge) Traverse(ctx context.Context, startNodeID string, opts sdkgraphrag.TraversalOptions) ([]sdkgraphrag.TraversalResult, error) {
	return nil, nil
}

func (m *mockGraphRAGQueryBridge) Health(ctx context.Context) types.HealthStatus {
	m.healthCalled = true
	return m.healthStatus
}
