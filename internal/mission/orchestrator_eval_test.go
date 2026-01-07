package mission

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/eval"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"go.opentelemetry.io/otel/trace"
)

// TestWithEvalOptions tests that WithEvalOptions correctly sets eval options.
func TestWithEvalOptions(t *testing.T) {
	store := &mockMissionStore{}
	evalOpts := eval.NewEvalOptions()
	evalOpts.Enabled = true

	orchestrator := NewMissionOrchestrator(
		store,
		WithEvalOptions(evalOpts),
	)

	assert.NotNil(t, orchestrator.evalOptions)
	assert.Equal(t, evalOpts, orchestrator.evalOptions)
}

// TestWrapFactoryWithEval_Disabled tests that wrapping is skipped when eval is disabled.
func TestWrapFactoryWithEval_Disabled(t *testing.T) {
	// Create a mock factory
	mockFactory := &mockHarnessFactory{}

	// Eval options with disabled eval
	opts := eval.NewEvalOptions()
	opts.Enabled = false

	// Wrap factory
	wrapped, collector, err := wrapFactoryWithEval(mockFactory, opts)

	// Should return original factory unchanged
	require.NoError(t, err)
	assert.Equal(t, mockFactory, wrapped)
	assert.Nil(t, collector)
}

// TestWrapFactoryWithEval_NilOptions tests handling of nil options.
func TestWrapFactoryWithEval_NilOptions(t *testing.T) {
	mockFactory := &mockHarnessFactory{}

	wrapped, collector, err := wrapFactoryWithEval(mockFactory, nil)

	require.NoError(t, err)
	assert.Equal(t, mockFactory, wrapped)
	assert.Nil(t, collector)
}

// TestWrapFactoryWithEval_InvalidOptions tests error handling for invalid options.
func TestWrapFactoryWithEval_InvalidOptions(t *testing.T) {
	mockFactory := &mockHarnessFactory{}

	// Create invalid eval options (enabled but missing ground truth)
	opts := eval.NewEvalOptions()
	opts.Enabled = true
	opts.GroundTruthPath = "" // Required when enabled

	wrapped, collector, err := wrapFactoryWithEval(mockFactory, opts)

	require.Error(t, err)
	assert.Nil(t, wrapped)
	assert.Nil(t, collector)
}

// TestWrapFactoryWithEval_ValidOptions tests successful wrapping with valid options.
func TestWrapFactoryWithEval_ValidOptions(t *testing.T) {
	mockFactory := &mockHarnessFactory{}

	// Create temporary ground truth file
	tmpDir := t.TempDir()
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	err := os.WriteFile(groundTruthPath, []byte(`{"test": "expected"}`), 0644)
	require.NoError(t, err)

	// Create valid eval options
	opts := eval.NewEvalOptions()
	opts.Enabled = true
	opts.GroundTruthPath = groundTruthPath

	wrapped, collector, err := wrapFactoryWithEval(mockFactory, opts)

	require.NoError(t, err)
	assert.NotNil(t, wrapped)
	assert.NotEqual(t, mockFactory, wrapped) // Should be wrapped
	assert.NotNil(t, collector)

	// Verify the wrapped factory is an EvalHarnessFactory
	_, ok := wrapped.(*eval.EvalHarnessFactory)
	assert.True(t, ok, "wrapped factory should be EvalHarnessFactory")
}

// TestGetEvalResults_NotEnabled tests GetEvalResults when eval is not enabled.
func TestGetEvalResults_NotEnabled(t *testing.T) {
	store := &mockMissionStore{}
	orchestrator := NewMissionOrchestrator(store)

	results := orchestrator.GetEvalResults()
	assert.Nil(t, results)
}

// TestGetEvalResults_Enabled tests GetEvalResults when eval is enabled.
func TestGetEvalResults_Enabled(t *testing.T) {
	store := &mockMissionStore{}

	// Create temporary ground truth file
	tmpDir := t.TempDir()
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	err := os.WriteFile(groundTruthPath, []byte(`{"test": "expected"}`), 0644)
	require.NoError(t, err)

	// Create eval options
	evalOpts := eval.NewEvalOptions()
	evalOpts.Enabled = true
	evalOpts.GroundTruthPath = groundTruthPath

	// Create mock factory
	mockFactory := &mockHarnessFactory{}

	orchestrator := NewMissionOrchestrator(
		store,
		WithHarnessFactory(mockFactory),
		WithEvalOptions(evalOpts),
	)

	results := orchestrator.GetEvalResults()
	assert.NotNil(t, results)
}

// TestFinalizeEvalResults_NotEnabled tests finalization when eval is not enabled.
func TestFinalizeEvalResults_NotEnabled(t *testing.T) {
	store := &mockMissionStore{}
	orchestrator := NewMissionOrchestrator(store)

	ctx := context.Background()
	summary, err := orchestrator.FinalizeEvalResults(ctx)

	require.Error(t, err)
	assert.Nil(t, summary)
	assert.Contains(t, err.Error(), "evaluation is not enabled")
}

// TestFinalizeEvalResults_Enabled tests finalization when eval is enabled.
func TestFinalizeEvalResults_Enabled(t *testing.T) {
	store := &mockMissionStore{}

	// Create temporary ground truth file
	tmpDir := t.TempDir()
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	err := os.WriteFile(groundTruthPath, []byte(`{"test": "expected"}`), 0644)
	require.NoError(t, err)

	// Create eval options
	evalOpts := eval.NewEvalOptions()
	evalOpts.Enabled = true
	evalOpts.GroundTruthPath = groundTruthPath

	// Create mock factory
	mockFactory := &mockHarnessFactory{}

	orchestrator := NewMissionOrchestrator(
		store,
		WithHarnessFactory(mockFactory),
		WithEvalOptions(evalOpts),
	)

	ctx := context.Background()
	summary, err := orchestrator.FinalizeEvalResults(ctx)

	require.NoError(t, err)
	assert.NotNil(t, summary)
	assert.NotNil(t, summary.MissionID)
}

// TestOrchestratorIntegration_WithEval tests the full orchestrator with eval enabled.
func TestOrchestratorIntegration_WithEval(t *testing.T) {
	// This is a more comprehensive integration test
	// It would require a more complete mock setup including workflow executor
	// For now, we'll test that the orchestrator can be created with all components

	store := &mockMissionStore{}

	// Create temporary ground truth file
	tmpDir := t.TempDir()
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	err := os.WriteFile(groundTruthPath, []byte(`{"test": "expected"}`), 0644)
	require.NoError(t, err)

	// Create eval options
	evalOpts := eval.NewEvalOptions()
	evalOpts.Enabled = true
	evalOpts.FeedbackEnabled = true
	evalOpts.GroundTruthPath = groundTruthPath
	evalOpts.WarningThreshold = 0.6
	evalOpts.CriticalThreshold = 0.3

	// Create mock factory
	mockFactory := &mockHarnessFactory{}

	orchestrator := NewMissionOrchestrator(
		store,
		WithHarnessFactory(mockFactory),
		WithEvalOptions(evalOpts),
	)

	// Verify orchestrator is properly configured
	assert.NotNil(t, orchestrator)
	assert.NotNil(t, orchestrator.evalOptions)
	assert.NotNil(t, orchestrator.evalCollector)
	assert.NotNil(t, orchestrator.harnessFactory)

	// Verify the harness factory is wrapped
	_, ok := orchestrator.harnessFactory.(*eval.EvalHarnessFactory)
	assert.True(t, ok, "harness factory should be wrapped with EvalHarnessFactory")

	// Verify eval results are accessible
	results := orchestrator.GetEvalResults()
	assert.NotNil(t, results)

	// Verify finalization works
	ctx := context.Background()
	summary, err := orchestrator.FinalizeEvalResults(ctx)
	require.NoError(t, err)
	assert.NotNil(t, summary)
}

// TestOrchestratorIntegration_EvalOptionsSetAfterFactory tests that eval wrapping
// works correctly when factory is set before eval options.
func TestOrchestratorIntegration_EvalOptionsSetAfterFactory(t *testing.T) {
	store := &mockMissionStore{}

	// Create temporary ground truth file
	tmpDir := t.TempDir()
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	err := os.WriteFile(groundTruthPath, []byte(`{"test": "expected"}`), 0644)
	require.NoError(t, err)

	// Create eval options
	evalOpts := eval.NewEvalOptions()
	evalOpts.Enabled = true
	evalOpts.GroundTruthPath = groundTruthPath

	// Create mock factory
	mockFactory := &mockHarnessFactory{}

	// Set factory first, then eval options
	orchestrator := NewMissionOrchestrator(
		store,
		WithHarnessFactory(mockFactory),
		WithEvalOptions(evalOpts),
	)

	// Verify the harness factory is wrapped
	_, ok := orchestrator.harnessFactory.(*eval.EvalHarnessFactory)
	assert.True(t, ok, "harness factory should be wrapped with EvalHarnessFactory")

	// Verify eval collector is set
	assert.NotNil(t, orchestrator.evalCollector)
}

// TestOrchestratorIntegration_NoFactorySet tests behavior when eval options are set
// but no factory is configured.
func TestOrchestratorIntegration_NoFactorySet(t *testing.T) {
	store := &mockMissionStore{}

	// Create temporary ground truth file
	tmpDir := t.TempDir()
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	err := os.WriteFile(groundTruthPath, []byte(`{"test": "expected"}`), 0644)
	require.NoError(t, err)

	// Create eval options
	evalOpts := eval.NewEvalOptions()
	evalOpts.Enabled = true
	evalOpts.GroundTruthPath = groundTruthPath

	// Create orchestrator with eval options but no factory
	orchestrator := NewMissionOrchestrator(
		store,
		WithEvalOptions(evalOpts),
	)

	// Verify orchestrator is created but eval is not active
	assert.NotNil(t, orchestrator)
	assert.NotNil(t, orchestrator.evalOptions)
	assert.Nil(t, orchestrator.harnessFactory) // No factory set
	assert.Nil(t, orchestrator.evalCollector)  // Eval not initialized
}

// mockHarnessFactory is a simple mock implementation of HarnessFactoryInterface for testing.
type mockHarnessFactory struct{}

func (m *mockHarnessFactory) Create(agentName string, missionCtx harness.MissionContext, target harness.TargetInfo) (harness.AgentHarness, error) {
	// Return a mock harness
	return &mockAgentHarness{
		missionCtx: missionCtx,
		target:     target,
	}, nil
}

func (m *mockHarnessFactory) CreateChild(parent harness.AgentHarness, agentName string) (harness.AgentHarness, error) {
	// Return a mock child harness
	return &mockAgentHarness{
		missionCtx: parent.Mission(),
		target:     parent.Target(),
	}, nil
}

// mockAgentHarness is a minimal mock implementation of AgentHarness for testing.
type mockAgentHarness struct {
	missionCtx harness.MissionContext
	target     harness.TargetInfo
}

func (m *mockAgentHarness) Mission() harness.MissionContext {
	return m.missionCtx
}

func (m *mockAgentHarness) Target() harness.TargetInfo {
	return m.target
}

// Implement remaining AgentHarness methods as no-ops for testing
func (m *mockAgentHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{}, nil
}

func (m *mockAgentHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{}, nil
}

func (m *mockAgentHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockAgentHarness) CallTool(ctx context.Context, toolName string, params map[string]any) (map[string]any, error) {
	return nil, nil
}

func (m *mockAgentHarness) QueryPlugin(ctx context.Context, pluginName, method string, params map[string]any) (any, error) {
	return nil, nil
}

func (m *mockAgentHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return nil
}

func (m *mockAgentHarness) Memory() memory.MemoryStore {
	return nil
}

func (m *mockAgentHarness) DelegateToAgent(ctx context.Context, agentName string, task agent.Task) (agent.Result, error) {
	return agent.Result{}, nil
}

func (m *mockAgentHarness) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return []agent.Finding{}, nil
}

func (m *mockAgentHarness) ListAgents() []harness.AgentDescriptor {
	return []harness.AgentDescriptor{}
}

func (m *mockAgentHarness) ListTools() []harness.ToolDescriptor {
	return []harness.ToolDescriptor{}
}

func (m *mockAgentHarness) ListPlugins() []harness.PluginDescriptor {
	return []harness.PluginDescriptor{}
}

func (m *mockAgentHarness) Logger() *slog.Logger {
	return slog.Default()
}

func (m *mockAgentHarness) Metrics() harness.MetricsRecorder {
	return nil
}

func (m *mockAgentHarness) TokenUsage() *llm.TokenTracker {
	return nil
}

func (m *mockAgentHarness) Tracer() trace.Tracer {
	return nil
}
