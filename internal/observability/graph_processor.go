package observability

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// GraphSpanProcessor implements sdktrace.SpanProcessor to write completed spans
// to the ExecutionGraphStore for observability and cross-mission correlation.
//
// The processor maps span names to appropriate store.Record* methods:
//   - "gen_ai.chat" / "gen_ai.chat.stream" → RecordLLMCall
//   - "gen_ai.tool" → RecordToolCall
//   - "gibson.agent.delegate" → RecordDelegation
//   - "gibson.mission.execute" → RecordMissionStart/Complete
//   - Spans starting with "gibson.agent." → RecordAgentStart/Complete
//
// Thread-safety: All methods are safe for concurrent access.
type GraphSpanProcessor struct {
	store  graphrag.ExecutionGraphStore
	logger *slog.Logger
}

// NewGraphSpanProcessor creates a new GraphSpanProcessor that writes spans
// to the provided ExecutionGraphStore.
func NewGraphSpanProcessor(store graphrag.ExecutionGraphStore, logger *slog.Logger) *GraphSpanProcessor {
	return &GraphSpanProcessor{
		store:  store,
		logger: logger,
	}
}

// OnStart is called when a span starts. This processor does not perform any
// actions on span start, as it processes spans only on completion.
func (p *GraphSpanProcessor) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {
	// No-op: we process on completion
}

// OnEnd is called when a span ends. It extracts attributes from the span
// and calls the appropriate ExecutionGraphStore Record* method based on
// the span name. Writes are performed in a goroutine with a timeout to
// avoid blocking the tracing pipeline.
func (p *GraphSpanProcessor) OnEnd(s sdktrace.ReadOnlySpan) {
	spanName := s.Name()

	// Extract trace/span IDs for correlation
	traceID := s.SpanContext().TraceID().String()
	spanID := s.SpanContext().SpanID().String()

	// Process span in goroutine to avoid blocking
	go p.processSpan(spanName, traceID, spanID, s)
}

// processSpan handles the actual span processing logic in a goroutine.
func (p *GraphSpanProcessor) processSpan(spanName, traceID, spanID string, s sdktrace.ReadOnlySpan) {
	// Create context with timeout to ensure we don't block indefinitely
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error

	// Route span to appropriate handler based on span name
	switch {
	case spanName == SpanGenAIChat || spanName == SpanGenAIChatStream:
		err = p.recordLLMCall(ctx, traceID, spanID, s)

	case spanName == SpanGenAITool:
		err = p.recordToolCall(ctx, traceID, spanID, s)

	case spanName == SpanAgentDelegate:
		err = p.recordDelegation(ctx, traceID, spanID, s)

	case spanName == "gibson.mission.execute":
		// Mission execution span completed - record both start and completion
		if err = p.recordMissionStart(ctx, traceID, spanID, s); err != nil {
			break
		}
		err = p.recordMissionComplete(ctx, traceID, spanID, s)

	case strings.HasPrefix(spanName, "gibson.agent."):
		// Agent execution span completed - record both start and completion
		if err = p.recordAgentStart(ctx, traceID, spanID, s); err != nil {
			break
		}
		err = p.recordAgentComplete(ctx, traceID, spanID, s)
	}

	// Log errors but don't fail - observability should not break the main flow
	if err != nil {
		p.logger.Error("failed to record span to graph store",
			slog.String("span_name", spanName),
			slog.String("trace_id", traceID),
			slog.String("span_id", spanID),
			slog.String("error", err.Error()),
		)
	}
}

// recordLLMCall extracts LLM call attributes and writes to the store.
func (p *GraphSpanProcessor) recordLLMCall(ctx context.Context, traceID, spanID string, s sdktrace.ReadOnlySpan) error {
	event := graphrag.LLMCallEvent{
		AgentName: getStringAttribute(s, GibsonAgentName),
		MissionID: types.ID(getStringAttribute(s, GibsonMissionID)),
		Model:     getStringAttribute(s, GenAIResponseModel),
		Provider:  getStringAttribute(s, GenAISystem),
		Slot:      getStringAttribute(s, "gibson.llm.slot"),
		TokensIn:  int(getIntAttribute(s, GenAIUsageInputTokens)),
		TokensOut: int(getIntAttribute(s, GenAIUsageOutputTokens)),
		CostUSD:   getFloatAttribute(s, GibsonLLMCost),
		LatencyMs: s.EndTime().Sub(s.StartTime()).Milliseconds(),
		TraceID:   traceID,
		SpanID:    spanID,
		Timestamp: s.EndTime(),
	}

	return p.store.RecordLLMCall(ctx, event)
}

// recordToolCall extracts tool call attributes and writes to the store.
func (p *GraphSpanProcessor) recordToolCall(ctx context.Context, traceID, spanID string, s sdktrace.ReadOnlySpan) error {
	success := s.Status().Code != codes.Error
	errorMsg := ""
	if !success {
		errorMsg = s.Status().Description
	}

	event := graphrag.ToolCallEvent{
		AgentName:  getStringAttribute(s, GibsonAgentName),
		MissionID:  types.ID(getStringAttribute(s, GibsonMissionID)),
		ToolName:   getStringAttribute(s, GibsonToolName),
		DurationMs: s.EndTime().Sub(s.StartTime()).Milliseconds(),
		Success:    success,
		Error:      errorMsg,
		TraceID:    traceID,
		SpanID:     spanID,
		Timestamp:  s.EndTime(),
	}

	return p.store.RecordToolCall(ctx, event)
}

// recordDelegation extracts delegation attributes and writes to the store.
func (p *GraphSpanProcessor) recordDelegation(ctx context.Context, traceID, spanID string, s sdktrace.ReadOnlySpan) error {
	event := graphrag.DelegationEvent{
		FromAgent: getStringAttribute(s, GibsonAgentName),
		ToAgent:   getStringAttribute(s, GibsonDelegationTarget),
		TaskID:    getStringAttribute(s, GibsonDelegationTaskID),
		MissionID: types.ID(getStringAttribute(s, GibsonMissionID)),
		TraceID:   traceID,
		SpanID:    spanID,
		Timestamp: s.StartTime(),
	}

	return p.store.RecordDelegation(ctx, event)
}

// recordAgentStart extracts agent start attributes and writes to the store.
func (p *GraphSpanProcessor) recordAgentStart(ctx context.Context, traceID, spanID string, s sdktrace.ReadOnlySpan) error {
	event := graphrag.AgentStartEvent{
		AgentName: getStringAttribute(s, GibsonAgentName),
		TaskID:    getStringAttribute(s, "gibson.task.id"),
		MissionID: types.ID(getStringAttribute(s, GibsonMissionID)),
		TraceID:   traceID,
		SpanID:    spanID,
		StartedAt: s.StartTime(),
	}

	return p.store.RecordAgentStart(ctx, event)
}

// recordAgentComplete extracts agent completion attributes and writes to the store.
func (p *GraphSpanProcessor) recordAgentComplete(ctx context.Context, traceID, spanID string, s sdktrace.ReadOnlySpan) error {
	status := "success"
	if s.Status().Code == codes.Error {
		status = "failure"
	}

	event := graphrag.AgentCompleteEvent{
		AgentName:     getStringAttribute(s, GibsonAgentName),
		TaskID:        getStringAttribute(s, "gibson.task.id"),
		MissionID:     types.ID(getStringAttribute(s, GibsonMissionID)),
		Status:        status,
		FindingsCount: int(getIntAttribute(s, "gibson.metrics.findings_count")),
		TraceID:       traceID,
		SpanID:        spanID,
		CompletedAt:   s.EndTime(),
		DurationMs:    s.EndTime().Sub(s.StartTime()).Milliseconds(),
	}

	return p.store.RecordAgentComplete(ctx, event)
}

// recordMissionStart extracts mission start attributes and writes to the store.
func (p *GraphSpanProcessor) recordMissionStart(ctx context.Context, traceID, spanID string, s sdktrace.ReadOnlySpan) error {
	event := graphrag.MissionStartEvent{
		MissionID:   types.ID(getStringAttribute(s, GibsonMissionID)),
		MissionName: getStringAttribute(s, GibsonMissionName),
		TraceID:     traceID,
		StartedAt:   s.StartTime(),
	}

	return p.store.RecordMissionStart(ctx, event)
}

// recordMissionComplete extracts mission completion attributes and writes to the store.
func (p *GraphSpanProcessor) recordMissionComplete(ctx context.Context, traceID, spanID string, s sdktrace.ReadOnlySpan) error {
	status := "success"
	if s.Status().Code == codes.Error {
		status = "failure"
	}

	event := graphrag.MissionCompleteEvent{
		MissionID:     types.ID(getStringAttribute(s, GibsonMissionID)),
		Status:        status,
		FindingsCount: int(getIntAttribute(s, "gibson.metrics.findings_count")),
		TraceID:       traceID,
		CompletedAt:   s.EndTime(),
		DurationMs:    s.EndTime().Sub(s.StartTime()).Milliseconds(),
	}

	return p.store.RecordMissionComplete(ctx, event)
}

// Shutdown is called when the tracer provider is shut down.
// The store is closed elsewhere, so this is a no-op.
func (p *GraphSpanProcessor) Shutdown(ctx context.Context) error {
	return nil
}

// ForceFlush is called to flush any buffered spans.
// Since writes are immediate, this is a no-op.
func (p *GraphSpanProcessor) ForceFlush(ctx context.Context) error {
	return nil
}

// getStringAttribute retrieves a string attribute from a span.
// Returns empty string if the attribute is not found.
func getStringAttribute(s sdktrace.ReadOnlySpan, key string) string {
	for _, attr := range s.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
}

// getIntAttribute retrieves an int64 attribute from a span.
// Returns 0 if the attribute is not found.
func getIntAttribute(s sdktrace.ReadOnlySpan, key string) int64 {
	for _, attr := range s.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsInt64()
		}
	}
	return 0
}

// getFloatAttribute retrieves a float64 attribute from a span.
// Returns 0.0 if the attribute is not found.
func getFloatAttribute(s sdktrace.ReadOnlySpan, key string) float64 {
	for _, attr := range s.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsFloat64()
		}
	}
	return 0.0
}
