package planning

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// mockHarness implements harness.AgentHarness for testing
type mockHarness struct {
	memoryStore memory.MemoryStore
}

func (m *mockHarness) Memory() memory.MemoryStore {
	return m.memoryStore
}

// Implement other AgentHarness methods as no-ops for testing
func (m *mockHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return nil, nil
}

func (m *mockHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return nil, nil
}

func (m *mockHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func (m *mockHarness) CallTool(ctx context.Context, toolName string, params map[string]any) (map[string]any, error) {
	return nil, nil
}

func (m *mockHarness) ListTools() []harness.ToolDescriptor {
	return nil
}

func (m *mockHarness) QueryPlugin(ctx context.Context, pluginName string, method string, params map[string]any) (any, error) {
	return nil, nil
}

func (m *mockHarness) ListPlugins() []harness.PluginDescriptor {
	return nil
}

func (m *mockHarness) DelegateToAgent(ctx context.Context, agentName string, task agent.Task) (agent.Result, error) {
	return agent.Result{}, nil
}

func (m *mockHarness) ListAgents() []harness.AgentDescriptor {
	return nil
}

func (m *mockHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return nil
}

func (m *mockHarness) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}

func (m *mockHarness) Logger() *slog.Logger {
	return slog.Default()
}

func (m *mockHarness) Tracer() trace.Tracer {
	return noop.NewTracerProvider().Tracer("mock")
}

func (m *mockHarness) Metrics() harness.MetricsRecorder {
	return nil
}

func (m *mockHarness) Mission() harness.MissionContext {
	return harness.MissionContext{}
}

func (m *mockHarness) Target() harness.TargetInfo {
	return harness.TargetInfo{}
}

func (m *mockHarness) TokenUsage() *llm.TokenTracker {
	return nil
}

func (m *mockHarness) PlanContext() *harness.PlanningContext {
	return nil
}

func (m *mockHarness) GetStepBudget() int {
	return 0
}

func (m *mockHarness) SignalReplanRecommended(ctx context.Context, reason string) error {
	return nil
}

func (m *mockHarness) ReportStepHints(ctx context.Context, hints *harness.StepHints) error {
	return nil
}

// mockMemoryStore implements memory.MemoryStore for testing
type mockMemoryStore struct {
	longTerm *mockLongTermMemory
}

func (m *mockMemoryStore) Working() memory.WorkingMemory {
	return nil
}

func (m *mockMemoryStore) Mission() memory.MissionMemory {
	return nil
}

func (m *mockMemoryStore) LongTerm() memory.LongTermMemory {
	return m.longTerm
}

// mockLongTermMemory implements memory.LongTermMemory for testing
type mockLongTermMemory struct {
	healthy       bool
	searchResults []memory.MemoryResult
	searchError   error
	searchDelay   time.Duration
}

func (m *mockLongTermMemory) Store(ctx context.Context, id string, content string, metadata map[string]any) error {
	if m == nil {
		return nil
	}
	return nil
}

func (m *mockLongTermMemory) Search(ctx context.Context, query string, topK int, filters map[string]any) ([]memory.MemoryResult, error) {
	if m == nil {
		return nil, nil
	}
	if m.searchDelay > 0 {
		select {
		case <-time.After(m.searchDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.searchError != nil {
		return nil, m.searchError
	}
	return m.searchResults, nil
}

func (m *mockLongTermMemory) SimilarFindings(ctx context.Context, content string, topK int) ([]memory.MemoryResult, error) {
	if m == nil {
		return nil, nil
	}
	return m.Search(ctx, content, topK, map[string]any{"type": "finding"})
}

func (m *mockLongTermMemory) SimilarPatterns(ctx context.Context, pattern string, topK int) ([]memory.MemoryResult, error) {
	if m == nil {
		return nil, nil
	}
	return m.Search(ctx, pattern, topK, map[string]any{"type": "pattern"})
}

func (m *mockLongTermMemory) Delete(ctx context.Context, id string) error {
	if m == nil {
		return nil
	}
	return nil
}

func (m *mockLongTermMemory) Health(ctx context.Context) types.HealthStatus {
	if m == nil {
		return types.HealthStatus{}
	}
	if m.healthy {
		return types.Healthy("mock long-term memory healthy")
	}
	return types.Unhealthy("mock long-term memory unhealthy")
}

func TestQueryMemoryForPlanning_NilHarness(t *testing.T) {
	ctx := context.Background()
	target := harness.NewTargetInfo(types.NewID(), "test-target", "https://test.example.com", "llm")

	result, err := QueryMemoryForPlanning(ctx, nil, target)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "harness cannot be nil")
}

func TestQueryMemoryForPlanning_NilMemoryStore(t *testing.T) {
	ctx := context.Background()
	target := harness.NewTargetInfo(types.NewID(), "test-target", "https://test.example.com", "llm")

	h := &mockHarness{
		memoryStore: nil,
	}

	result, err := QueryMemoryForPlanning(ctx, h, target)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.QueryID)
	assert.Equal(t, "type=llm,name=test-target", result.TargetProfile)
	assert.Empty(t, result.SimilarMissions)
	assert.Empty(t, result.SuccessfulTechniques)
	assert.Empty(t, result.FailedApproaches)
	assert.Empty(t, result.RelevantFindings)
	assert.Equal(t, 0, result.ResultCount)
	assert.Greater(t, result.QueryLatency, time.Duration(0))
}

func TestQueryMemoryForPlanning_NilLongTermMemory(t *testing.T) {
	ctx := context.Background()
	target := harness.NewTargetInfo(types.NewID(), "test-target", "https://test.example.com", "llm")

	h := &mockHarness{
		memoryStore: &mockMemoryStore{
			longTerm: nil,
		},
	}

	result, err := QueryMemoryForPlanning(ctx, h, target)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.QueryID)
	assert.Empty(t, result.SimilarMissions)
	assert.Equal(t, 0, result.ResultCount)
	assert.Greater(t, result.QueryLatency, time.Duration(0))
}

func TestQueryMemoryForPlanning_UnhealthyMemory(t *testing.T) {
	ctx := context.Background()
	target := harness.NewTargetInfo(types.NewID(), "test-target", "https://test.example.com", "llm")

	h := &mockHarness{
		memoryStore: &mockMemoryStore{
			longTerm: &mockLongTermMemory{
				healthy: false,
			},
		},
	}

	result, err := QueryMemoryForPlanning(ctx, h, target)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.QueryID)
	assert.Empty(t, result.SimilarMissions)
	assert.Equal(t, 0, result.ResultCount)
	assert.Greater(t, result.QueryLatency, time.Duration(0))
}

func TestQueryMemoryForPlanning_HealthyMemoryNoResults(t *testing.T) {
	ctx := context.Background()
	target := harness.NewTargetInfo(types.NewID(), "test-target", "https://test.example.com", "llm")

	h := &mockHarness{
		memoryStore: &mockMemoryStore{
			longTerm: &mockLongTermMemory{
				healthy:       true,
				searchResults: []memory.MemoryResult{},
				searchError:   nil,
			},
		},
	}

	result, err := QueryMemoryForPlanning(ctx, h, target)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.QueryID)
	assert.Equal(t, "type=llm,name=test-target", result.TargetProfile)
	assert.Empty(t, result.SimilarMissions)
	assert.Empty(t, result.SuccessfulTechniques)
	assert.Empty(t, result.FailedApproaches)
	assert.Empty(t, result.RelevantFindings)
	assert.Equal(t, 0, result.ResultCount)
	assert.Greater(t, result.QueryLatency, time.Duration(0))
	assert.Less(t, result.QueryLatency, 500*time.Millisecond)
}

func TestQueryMemoryForPlanning_TimeoutEnforced(t *testing.T) {
	ctx := context.Background()
	target := harness.NewTargetInfo(types.NewID(), "test-target", "https://test.example.com", "llm")

	// Set up memory that would take longer than 500ms
	h := &mockHarness{
		memoryStore: &mockMemoryStore{
			longTerm: &mockLongTermMemory{
				healthy:       true,
				searchResults: []memory.MemoryResult{},
				searchDelay:   1 * time.Second, // Longer than 500ms timeout
			},
		},
	}

	start := time.Now()
	result, err := QueryMemoryForPlanning(ctx, h, target)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Query should timeout and return empty result within ~500ms (with some margin)
	assert.Less(t, elapsed, 600*time.Millisecond, "Query should respect 500ms timeout")
	assert.Empty(t, result.SimilarMissions)
	assert.Equal(t, 0, result.ResultCount)
}

func TestQueryMemoryForPlanning_TargetProfileWithProvider(t *testing.T) {
	ctx := context.Background()
	target := harness.NewTargetInfo(types.NewID(), "test-target", "https://test.example.com", "llm").
		WithProvider("anthropic")

	h := &mockHarness{
		memoryStore: &mockMemoryStore{
			longTerm: &mockLongTermMemory{
				healthy:       true,
				searchResults: []memory.MemoryResult{},
			},
		},
	}

	result, err := QueryMemoryForPlanning(ctx, h, target)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "type=llm,name=test-target,provider=anthropic", result.TargetProfile)
}

func TestQueryMemoryForPlanning_LatencyTracking(t *testing.T) {
	ctx := context.Background()
	target := harness.NewTargetInfo(types.NewID(), "test-target", "https://test.example.com", "llm")

	// Set up memory with a small delay
	h := &mockHarness{
		memoryStore: &mockMemoryStore{
			longTerm: &mockLongTermMemory{
				healthy:       true,
				searchResults: []memory.MemoryResult{},
				searchDelay:   50 * time.Millisecond,
			},
		},
	}

	result, err := QueryMemoryForPlanning(ctx, h, target)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Query latency should be tracked and >= the delay we introduced
	assert.GreaterOrEqual(t, result.QueryLatency, 50*time.Millisecond)
	assert.Less(t, result.QueryLatency, 500*time.Millisecond)
}

func TestQueryMemoryForPlanning_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	target := harness.NewTargetInfo(types.NewID(), "test-target", "https://test.example.com", "llm")

	h := &mockHarness{
		memoryStore: &mockMemoryStore{
			longTerm: &mockLongTermMemory{
				healthy:       true,
				searchResults: []memory.MemoryResult{},
			},
		},
	}

	result, err := QueryMemoryForPlanning(ctx, h, target)

	// Should still succeed gracefully even with canceled context
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.SimilarMissions)
}

func TestMemoryQueryResult_EmptyInitialization(t *testing.T) {
	result := &MemoryQueryResult{
		QueryID:              "test-query",
		TargetProfile:        "test-profile",
		SimilarMissions:      []MissionSummary{},
		SuccessfulTechniques: []TechniqueRecord{},
		FailedApproaches:     []ApproachRecord{},
		RelevantFindings:     []FindingSummary{},
		QueryLatency:         100 * time.Millisecond,
		ResultCount:          0,
	}

	assert.NotNil(t, result.SimilarMissions)
	assert.NotNil(t, result.SuccessfulTechniques)
	assert.NotNil(t, result.FailedApproaches)
	assert.NotNil(t, result.RelevantFindings)
	assert.Equal(t, 0, len(result.SimilarMissions))
	assert.Equal(t, 0, result.ResultCount)
}

func TestTechniqueRecord_Structure(t *testing.T) {
	now := time.Now()
	tr := TechniqueRecord{
		Technique:    "prompt-injection",
		SuccessRate:  0.75,
		AvgTokenCost: 1500,
		LastUsed:     now,
	}

	assert.Equal(t, "prompt-injection", tr.Technique)
	assert.Equal(t, 0.75, tr.SuccessRate)
	assert.Equal(t, 1500, tr.AvgTokenCost)
	assert.Equal(t, now, tr.LastUsed)
}

func TestApproachRecord_Structure(t *testing.T) {
	now := time.Now()
	ar := ApproachRecord{
		Approach:      "brute-force",
		FailureReason: "rate limited",
		TargetType:    "api",
		LastAttempted: now,
	}

	assert.Equal(t, "brute-force", ar.Approach)
	assert.Equal(t, "rate limited", ar.FailureReason)
	assert.Equal(t, "api", ar.TargetType)
	assert.Equal(t, now, ar.LastAttempted)
}

func TestMissionSummary_Structure(t *testing.T) {
	id := types.NewID()
	ms := MissionSummary{
		ID:            id,
		TargetType:    "llm",
		Techniques:    []string{"prompt-injection", "jailbreak"},
		Success:       true,
		FindingsCount: 5,
	}

	assert.Equal(t, id, ms.ID)
	assert.Equal(t, "llm", ms.TargetType)
	assert.Equal(t, 2, len(ms.Techniques))
	assert.True(t, ms.Success)
	assert.Equal(t, 5, ms.FindingsCount)
}

func TestFindingSummary_Structure(t *testing.T) {
	id := types.NewID()
	fs := FindingSummary{
		ID:        id,
		Title:     "SQL Injection",
		Category:  "injection",
		Severity:  "high",
		Technique: "sql-injection",
	}

	assert.Equal(t, id, fs.ID)
	assert.Equal(t, "SQL Injection", fs.Title)
	assert.Equal(t, "injection", fs.Category)
	assert.Equal(t, "high", fs.Severity)
	assert.Equal(t, "sql-injection", fs.Technique)
}

// Benchmark tests
func BenchmarkQueryMemoryForPlanning_NoResults(b *testing.B) {
	ctx := context.Background()
	target := harness.NewTargetInfo(types.NewID(), "test-target", "https://test.example.com", "llm")

	h := &mockHarness{
		memoryStore: &mockMemoryStore{
			longTerm: &mockLongTermMemory{
				healthy:       true,
				searchResults: []memory.MemoryResult{},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = QueryMemoryForPlanning(ctx, h, target)
	}
}

func BenchmarkQueryMemoryForPlanning_WithDelay(b *testing.B) {
	ctx := context.Background()
	target := harness.NewTargetInfo(types.NewID(), "test-target", "https://test.example.com", "llm")

	h := &mockHarness{
		memoryStore: &mockMemoryStore{
			longTerm: &mockLongTermMemory{
				healthy:       true,
				searchResults: []memory.MemoryResult{},
				searchDelay:   10 * time.Millisecond,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = QueryMemoryForPlanning(ctx, h, target)
	}
}
