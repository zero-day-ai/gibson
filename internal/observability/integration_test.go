package observability

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// TestIntegration_FullTracingFlow tests the complete tracing flow from
// initialization to span export and verification.
func TestIntegration_FullTracingFlow(t *testing.T) {
	// Create in-memory span exporter for verification
	exporter := tracetest.NewInMemoryExporter()

	// Create tracer provider with in-memory exporter
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	// Create tracer
	tracer := tp.Tracer("test-tracer")

	// Create spans with GenAI attributes
	ctx := context.Background()
	ctx, span1 := tracer.Start(ctx, SpanGenAIChat)
	span1.SetAttributes(
		attribute.String(GenAISystem, "anthropic"),
		attribute.String(GenAIRequestModel, "claude-3-opus"),
		attribute.Int(GenAIUsageInputTokens, 100),
		attribute.Int(GenAIUsageOutputTokens, 50),
	)
	span1.End()

	_, span2 := tracer.Start(ctx, SpanGenAITool)
	span2.SetAttributes(
		attribute.String(GibsonToolName, "test_tool"),
	)
	span2.End()

	// Force flush to exporter
	err := tp.ForceFlush(context.Background())
	require.NoError(t, err)

	// Verify spans were exported
	spans := exporter.GetSpans()
	require.Len(t, spans, 2, "expected 2 spans to be exported")

	// Verify first span (LLM completion)
	assert.Equal(t, SpanGenAIChat, spans[0].Name)
	attrs := spans[0].Attributes
	assert.Contains(t, attrs, attribute.String(GenAISystem, "anthropic"))
	assert.Contains(t, attrs, attribute.String(GenAIRequestModel, "claude-3-opus"))
	assert.Contains(t, attrs, attribute.Int(GenAIUsageInputTokens, 100))
	assert.Contains(t, attrs, attribute.Int(GenAIUsageOutputTokens, 50))

	// Verify second span (tool call)
	assert.Equal(t, SpanGenAITool, spans[1].Name)
	assert.Contains(t, spans[1].Attributes, attribute.String(GibsonToolName, "test_tool"))
}

// TestIntegration_TracedAgentHarness tests the TracedAgentHarness wrapping
// a mock AgentHarness and verifying trace creation.
func TestIntegration_TracedAgentHarness(t *testing.T) {
	// Set up in-memory tracing
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	tracer := tp.Tracer("test-tracer")

	// Create mock harness
	mockHarness := &mockAgentHarness{
		tracer:  tracer,
		metrics: &mockMetricsRecorder{},
		mission: harness.MissionContext{
			ID:           types.NewID(),
			Name:         "test-mission",
			CurrentAgent: "test-agent",
		},
	}

	// Create traced harness
	traced := NewTracedAgentHarness(mockHarness,
		WithTracer(tracer),
		WithPromptCapture(true),
	)

	ctx := context.Background()

	// Test Complete operation
	messages := []llm.Message{
		llm.NewSystemMessage("You are a test assistant"),
		llm.NewUserMessage("Hello"),
	}

	resp, err := traced.Complete(ctx, "primary", messages)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Test CallTool operation
	toolInput := map[string]any{"param": "value"}
	_, err = traced.CallTool(ctx, "test_tool", toolInput)
	require.NoError(t, err)

	// Test SubmitFinding operation
	finding := agent.Finding{
		ID:         types.NewID(),
		Title:      "Test Finding",
		Severity:   agent.SeverityCritical,
		Category:   "test",
		Confidence: 0.95,
	}
	err = traced.SubmitFinding(ctx, finding)
	require.NoError(t, err)

	// Force flush
	err = tp.ForceFlush(context.Background())
	require.NoError(t, err)

	// Verify spans were created
	spans := exporter.GetSpans()
	assert.GreaterOrEqual(t, len(spans), 3, "expected at least 3 spans")

	// Verify span names
	spanNames := make(map[string]bool)
	for _, s := range spans {
		spanNames[s.Name] = true
	}
	assert.True(t, spanNames[SpanGenAIChat], "expected gen_ai.chat span")
	assert.True(t, spanNames[SpanGenAITool], "expected gen_ai.tool span")
	assert.True(t, spanNames[SpanFindingSubmit], "expected gibson.finding.submit span")
}

// TestIntegration_MetricsCollection tests metrics collection with the
// OpenTelemetryMetricsRecorder.
func TestIntegration_MetricsCollection(t *testing.T) {
	// Create in-memory metrics reader
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
	)

	meter := mp.Meter("test-meter")
	recorder := NewOpenTelemetryMetricsRecorder(meter)

	// Record various metrics
	recorder.RecordCounter("test.counter", 5, map[string]string{"label": "value"})
	recorder.RecordHistogram("test.histogram", 42.5, map[string]string{"label": "value"})
	recorder.RecordGauge("test.gauge", 100.0, map[string]string{"label": "value"})

	// Record LLM completion metrics
	recorder.RecordLLMCompletion(
		"primary",
		"anthropic",
		"claude-3-opus",
		"success",
		1000, // input tokens
		500,  // output tokens
		150.5, // latency
		0.015, // cost
	)

	// Record tool call
	recorder.RecordToolCall("test_tool", "success", 250.0)

	// Record finding submission
	recorder.RecordFindingSubmitted("high", "injection")

	// Collect metrics
	var rm metricdata.ResourceMetrics
	err := reader.Collect(context.Background(), &rm)
	require.NoError(t, err)

	// Verify metrics were recorded
	assert.NotEmpty(t, rm.ScopeMetrics, "expected metrics to be recorded")

	// Count metrics by name
	metricCount := make(map[string]int)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			metricCount[m.Name]++
		}
	}

	// Verify expected metrics exist
	assert.Greater(t, metricCount["test.counter"], 0, "expected counter metric")
	assert.Greater(t, metricCount["test.histogram"], 0, "expected histogram metric")
	assert.Greater(t, metricCount[MetricLLMCompletions], 0, "expected LLM completions metric")
	assert.Greater(t, metricCount[MetricLLMTokensInput], 0, "expected LLM input tokens metric")
	assert.Greater(t, metricCount[MetricToolCalls], 0, "expected tool calls metric")
	assert.Greater(t, metricCount[MetricFindingsSubmitted], 0, "expected findings submitted metric")
}

// TestIntegration_TraceCorrelationAcrossDelegations tests trace correlation
// across simulated agent delegations.
func TestIntegration_TraceCorrelationAcrossDelegations(t *testing.T) {
	// Set up in-memory tracing
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	tracer := tp.Tracer("test-tracer")

	// Create parent span (simulating parent agent)
	ctx := context.Background()
	ctx, parentSpan := tracer.Start(ctx, "parent_agent")
	parentTraceID := parentSpan.SpanContext().TraceID()
	parentSpanID := parentSpan.SpanContext().SpanID()

	// Create child span (simulating delegated agent)
	ctx, childSpan := tracer.Start(ctx, SpanAgentDelegate)
	childSpan.SetAttributes(
		attribute.String(GibsonDelegationTarget, "child_agent"),
		attribute.String(GibsonDelegationTaskID, types.NewID().String()),
	)
	childTraceID := childSpan.SpanContext().TraceID()

	// Child span should have same trace ID as parent
	assert.Equal(t, parentTraceID, childTraceID, "child should have same trace ID as parent")

	// Create grandchild span (simulating nested delegation)
	ctx, grandchildSpan := tracer.Start(ctx, SpanGenAIChat)
	grandchildTraceID := grandchildSpan.SpanContext().TraceID()

	// All spans should have the same trace ID
	assert.Equal(t, parentTraceID, grandchildTraceID, "grandchild should have same trace ID")

	grandchildSpan.End()
	childSpan.End()
	parentSpan.End()

	// Force flush
	err := tp.ForceFlush(context.Background())
	require.NoError(t, err)

	// Verify spans were exported
	spans := exporter.GetSpans()
	require.Len(t, spans, 3, "expected 3 spans")

	// Verify trace correlation
	for _, s := range spans {
		assert.Equal(t, parentTraceID, s.SpanContext.TraceID(),
			"all spans should share the same trace ID")
	}

	// Verify parent-child relationship
	var delegateSpan tracetest.SpanStub
	var foundDelegate bool
	for _, s := range spans {
		if s.Name == SpanAgentDelegate {
			delegateSpan = s
			foundDelegate = true
			break
		}
	}
	require.True(t, foundDelegate, "expected to find delegate span")
	assert.Equal(t, parentSpanID, delegateSpan.Parent.SpanID(),
		"delegate span should have parent as its parent")
}

// TestIntegration_LoggingWithTraceCorrelation tests logging with trace
// correlation to verify trace_id appears in logs.
func TestIntegration_LoggingWithTraceCorrelation(t *testing.T) {
	// Set up in-memory tracing
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	tracer := tp.Tracer("test-tracer")

	// Create a custom log handler that captures log records
	records := &logRecordCapture{records: make([]map[string]any, 0)}
	handler := &capturingHandler{capture: records}

	// Create traced logger
	missionID := types.NewID().String()
	agentName := "test-agent"
	logger := NewTracedLogger(handler, missionID, agentName)

	// Create span and log with trace context
	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "test-operation")
	traceID := span.SpanContext().TraceID().String()
	spanID := span.SpanContext().SpanID().String()

	// Log messages at different levels
	logger.Info(ctx, "Test info message", slog.String("key", "value"))
	logger.Debug(ctx, "Test debug message")
	logger.Error(ctx, "Test error message")

	span.End()

	// Verify trace IDs appear in captured logs
	require.GreaterOrEqual(t, len(records.records), 3, "expected at least 3 log records")

	for _, record := range records.records {
		// Verify mission and agent context
		assert.Equal(t, missionID, record["mission_id"], "expected mission_id in log")
		assert.Equal(t, agentName, record["agent_name"], "expected agent_name in log")

		// Verify trace correlation
		assert.Equal(t, traceID, record["trace_id"], "expected trace_id in log")
		assert.Equal(t, spanID, record["span_id"], "expected span_id in log")
	}
}

// TestIntegration_HealthMonitor tests HealthMonitor with multiple mock components.
func TestIntegration_HealthMonitor(t *testing.T) {
	// Create metrics recorder
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := mp.Meter("test-meter")
	metrics := NewOpenTelemetryMetricsRecorder(meter)

	// Create logger
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := NewTracedLogger(handler, "test-mission", "test-agent")

	// Create health monitor
	monitor := NewHealthMonitor(metrics, logger)

	// Register mock components with different health states
	healthyComponent := &mockHealthChecker{
		status: types.NewHealthStatus(types.HealthStateHealthy, "all systems operational"),
	}
	degradedComponent := &mockHealthChecker{
		status: types.NewHealthStatus(types.HealthStateDegraded, "slow response times"),
	}
	unhealthyComponent := &mockHealthChecker{
		status: types.NewHealthStatus(types.HealthStateUnhealthy, "connection refused"),
	}

	monitor.Register("database", healthyComponent)
	monitor.Register("cache", degradedComponent)
	monitor.Register("external_api", unhealthyComponent)

	// Check individual components
	ctx := context.Background()

	dbHealth, err := monitor.Check(ctx, "database")
	require.NoError(t, err)
	assert.True(t, dbHealth.IsHealthy())
	assert.Equal(t, types.HealthStateHealthy, dbHealth.State)

	cacheHealth, err := monitor.Check(ctx, "cache")
	require.NoError(t, err)
	assert.False(t, cacheHealth.IsHealthy())
	assert.Equal(t, types.HealthStateDegraded, cacheHealth.State)

	apiHealth, err := monitor.Check(ctx, "external_api")
	require.NoError(t, err)
	assert.False(t, apiHealth.IsHealthy())
	assert.Equal(t, types.HealthStateUnhealthy, apiHealth.State)

	// Check all components
	allHealth := monitor.CheckAll(ctx)
	assert.Len(t, allHealth, 3)
	assert.True(t, allHealth["database"].IsHealthy())
	assert.False(t, allHealth["cache"].IsHealthy())
	assert.False(t, allHealth["external_api"].IsHealthy())

	// Verify metrics were recorded
	var rm metricdata.ResourceMetrics
	err = reader.Collect(ctx, &rm)
	require.NoError(t, err)
	assert.NotEmpty(t, rm.ScopeMetrics)

	// Test unregister
	monitor.Unregister("external_api")
	_, err = monitor.Check(ctx, "external_api")
	assert.Error(t, err, "expected error for unregistered component")
}

// TestIntegration_CostTracker tests CostTracker with mock TokenTracker.
func TestIntegration_CostTracker(t *testing.T) {
	// Create mock token tracker
	mockTracker := &mockTokenTracker{
		costs: make(map[string]float64),
	}

	// Create logger
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := NewTracedLogger(handler, "test-mission", "test-agent")

	// Create cost tracker
	tracker := NewCostTracker(mockTracker, logger)

	// Calculate costs for different providers/models
	cost1 := tracker.CalculateCost("anthropic", "claude-3-opus-20240229", 1000, 500)
	assert.Greater(t, cost1, 0.0, "expected non-zero cost for Anthropic")

	cost2 := tracker.CalculateCost("openai", "gpt-4", 1000, 500)
	assert.Greater(t, cost2, 0.0, "expected non-zero cost for OpenAI")

	// Test mission cost tracking
	missionID := types.NewID()
	mockTracker.costs[missionID.String()] = 1.25

	missionCost, err := tracker.GetMissionCost(missionID.String())
	require.NoError(t, err)
	assert.Equal(t, 1.25, missionCost)

	// Test agent cost tracking
	agentKey := missionID.String() + ":agent1"
	mockTracker.costs[agentKey] = 0.75

	agentCost, err := tracker.GetAgentCost(missionID.String(), "agent1")
	require.NoError(t, err)
	assert.Equal(t, 0.75, agentCost)

	// Test threshold setting and checking
	err = tracker.SetThreshold(missionID.String(), 1.0)
	require.NoError(t, err)

	// Check threshold - should not exceed
	exceeded := tracker.CheckThreshold(missionID.String(), 0.5)
	assert.False(t, exceeded, "expected threshold not exceeded")

	// Check threshold - should exceed
	exceeded = tracker.CheckThreshold(missionID.String(), 1.5)
	assert.True(t, exceeded, "expected threshold exceeded")

	// Get threshold
	threshold, exists := tracker.GetThreshold(missionID.String())
	assert.True(t, exists)
	assert.Equal(t, 1.0, threshold)

	// Remove threshold
	tracker.RemoveThreshold(missionID.String())
	_, exists = tracker.GetThreshold(missionID.String())
	assert.False(t, exists, "expected threshold to be removed")
}

// TestIntegration_PeriodicHealthChecks tests StartPeriodicCheck functionality.
func TestIntegration_PeriodicHealthChecks(t *testing.T) {
	// Create metrics recorder
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := mp.Meter("test-meter")
	metrics := NewOpenTelemetryMetricsRecorder(meter)

	// Create logger
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := NewTracedLogger(handler, "test-mission", "test-agent")

	// Create health monitor
	monitor := NewHealthMonitor(metrics, logger)

	// Register component with counter to track check frequency
	checkCount := &atomic.Int32{}
	component := &mockHealthChecker{
		status: types.NewHealthStatus(types.HealthStateHealthy, "ok"),
		onCheck: func() {
			checkCount.Add(1)
		},
	}
	monitor.Register("test_component", component)

	// Start periodic checks with short interval
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	go monitor.StartPeriodicCheck(ctx, 50*time.Millisecond)

	// Wait for periodic checks to run
	<-ctx.Done()

	// Verify multiple checks occurred
	count := checkCount.Load()
	assert.Greater(t, count, int32(2), "expected multiple periodic health checks")
}

// Mock implementations

type mockAgentHarness struct {
	tracer  trace.Tracer
	metrics harness.MetricsRecorder
	mission harness.MissionContext
}

func (m *mockAgentHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{
		Message: llm.Message{
			Role:    llm.RoleAssistant,
			Content: "test response",
		},
		Model:        "test-model",
		FinishReason: llm.FinishReasonStop,
		Usage: llm.CompletionTokenUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}, nil
}

func (m *mockAgentHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return m.Complete(ctx, slot, messages, opts...)
}

func (m *mockAgentHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	go func() {
		defer close(ch)
		ch <- llm.StreamChunk{
			Delta: llm.StreamDelta{Content: "test"},
		}
	}()
	return ch, nil
}

func (m *mockAgentHarness) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return map[string]any{"result": "success"}, nil
}

func (m *mockAgentHarness) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	return "plugin result", nil
}

func (m *mockAgentHarness) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	return agent.Result{Status: agent.ResultStatusCompleted}, nil
}

func (m *mockAgentHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return nil
}

func (m *mockAgentHarness) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return []agent.Finding{}, nil
}

func (m *mockAgentHarness) Memory() memory.MemoryStore {
	return nil
}

func (m *mockAgentHarness) Mission() harness.MissionContext {
	return m.mission
}

func (m *mockAgentHarness) Target() harness.TargetInfo {
	return harness.TargetInfo{
		ID:   types.NewID(),
		Name: "test-target",
	}
}

func (m *mockAgentHarness) ListTools() []harness.ToolDescriptor {
	return []harness.ToolDescriptor{}
}

func (m *mockAgentHarness) ListPlugins() []harness.PluginDescriptor {
	return []harness.PluginDescriptor{}
}

func (m *mockAgentHarness) ListAgents() []harness.AgentDescriptor {
	return []harness.AgentDescriptor{}
}

func (m *mockAgentHarness) Tracer() trace.Tracer {
	return m.tracer
}

func (m *mockAgentHarness) Logger() *slog.Logger {
	return slog.Default()
}

func (m *mockAgentHarness) Metrics() harness.MetricsRecorder {
	return m.metrics
}

func (m *mockAgentHarness) TokenUsage() *llm.TokenTracker {
	return nil
}

type mockMetricsRecorder struct{}

func (m *mockMetricsRecorder) RecordCounter(name string, value int64, labels map[string]string)                              {}
func (m *mockMetricsRecorder) RecordGauge(name string, value float64, labels map[string]string)                             {}
func (m *mockMetricsRecorder) RecordHistogram(name string, value float64, labels map[string]string)                         {}
func (m *mockMetricsRecorder) RecordLLMCompletion(slot, provider, model, status string, inputTokens, outputTokens int, latencyMs, cost float64) {
}
func (m *mockMetricsRecorder) RecordToolCall(tool, status string, durationMs float64) {}
func (m *mockMetricsRecorder) RecordFindingSubmitted(severity, category string)       {}
func (m *mockMetricsRecorder) RecordAgentDelegation(sourceAgent, targetAgent, status string) {
}

type mockHealthChecker struct {
	status  types.HealthStatus
	onCheck func()
}

func (m *mockHealthChecker) Health(ctx context.Context) types.HealthStatus {
	if m.onCheck != nil {
		m.onCheck()
	}
	return m.status
}

type mockTokenTracker struct {
	costs map[string]float64
}

func (m *mockTokenTracker) RecordUsage(scope llm.UsageScope, provider string, model string, usage llm.TokenUsage) error {
	return nil
}

func (m *mockTokenTracker) GetUsage(scope llm.UsageScope) (llm.UsageRecord, error) {
	return llm.UsageRecord{}, nil
}

func (m *mockTokenTracker) GetCost(scope llm.UsageScope) (float64, error) {
	key := scope.MissionID.String()
	if scope.AgentName != "" {
		key += ":" + scope.AgentName
	}
	cost, exists := m.costs[key]
	if !exists {
		return 0, types.NewError(types.TARGET_NOT_FOUND, "no usage data found")
	}
	return cost, nil
}

func (m *mockTokenTracker) SetBudget(scope llm.UsageScope, budget llm.Budget) error {
	return nil
}

func (m *mockTokenTracker) CheckBudget(scope llm.UsageScope, provider string, model string, usage llm.TokenUsage) error {
	return nil
}

func (m *mockTokenTracker) GetBudget(scope llm.UsageScope) (llm.Budget, error) {
	return llm.Budget{}, nil
}

func (m *mockTokenTracker) Reset(scope llm.UsageScope) error {
	return nil
}

type logRecordCapture struct {
	records []map[string]any
}

type capturingHandler struct {
	capture   *logRecordCapture
	baseAttrs []slog.Attr
}

func (h *capturingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *capturingHandler) Handle(ctx context.Context, r slog.Record) error {
	record := make(map[string]any)
	record["message"] = r.Message
	record["level"] = r.Level.String()

	// First add base attributes (from WithAttrs)
	for _, attr := range h.baseAttrs {
		record[attr.Key] = attr.Value.Any()
	}

	// Then add record-specific attributes
	r.Attrs(func(a slog.Attr) bool {
		record[a.Key] = a.Value.Any()
		return true
	})

	h.capture.records = append(h.capture.records, record)
	return nil
}

func (h *capturingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Create a new handler with combined base attributes
	newBaseAttrs := make([]slog.Attr, len(h.baseAttrs)+len(attrs))
	copy(newBaseAttrs, h.baseAttrs)
	copy(newBaseAttrs[len(h.baseAttrs):], attrs)

	return &capturingHandler{
		capture:   h.capture,
		baseAttrs: newBaseAttrs,
	}
}

func (h *capturingHandler) WithGroup(name string) slog.Handler {
	return h
}
