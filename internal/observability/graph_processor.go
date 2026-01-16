package observability

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/engine"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// GraphSpanProcessor implements sdktrace.SpanProcessor to write completed spans
// to the TaxonomyGraphEngine for observability and cross-mission correlation.
//
// The processor maps span names to taxonomy event types:
//   - "gen_ai.chat" / "gen_ai.chat.stream" → "llm.request.started", "llm.request.completed"
//   - "gen_ai.tool" → "tool.call.started", "tool.call.completed"
//   - "gibson.agent.delegate" → "agent.delegated"
//   - "gibson.mission.execute" → "mission.started" and "mission.completed"
//   - Spans starting with "gibson.agent." → "agent.started" and "agent.completed"
//
// Thread-safety: All methods are safe for concurrent access.
type GraphSpanProcessor struct {
	engine engine.TaxonomyGraphEngine
	logger *slog.Logger
}

// NewGraphSpanProcessor creates a new GraphSpanProcessor that writes spans
// to the provided TaxonomyGraphEngine.
func NewGraphSpanProcessor(taxonomyEngine engine.TaxonomyGraphEngine, logger *slog.Logger) *GraphSpanProcessor {
	return &GraphSpanProcessor{
		engine: taxonomyEngine,
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
	// Guard against nil spans from proxy conversion
	if s == nil {
		return
	}

	// Recover from panics when calling interface methods on invalid spans
	defer func() {
		if r := recover(); r != nil {
			p.logger.Warn("recovered from panic in GraphSpanProcessor.OnEnd",
				"panic", r)
		}
	}()

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

	// Map span name to event type(s) and extract data
	eventTypes := p.mapSpanToEventTypes(spanName)
	if len(eventTypes) == 0 {
		// Unknown span type - log debug and skip
		p.logger.Debug("unknown span type, skipping",
			"span_name", spanName,
			"trace_id", traceID)
		return
	}

	// Extract span data into a map for taxonomy processing
	data := p.extractSpanData(s, traceID, spanID)

	// Process each event type
	for _, eventType := range eventTypes {
		// For end events, modify the data to include completion status
		if strings.HasSuffix(eventType, ".end") || strings.HasSuffix(eventType, ".complete") {
			data["status"] = p.getSpanStatus(s)
			data["duration_ms"] = s.EndTime().Sub(s.StartTime()).Milliseconds()
			data["completed_at"] = s.EndTime()
		}

		// Get agent run ID for context
		agentRunID := getStringAttribute(s, "gibson.agent.run_id")
		if agentRunID == "" {
			// Fallback to span_id if no agent run ID
			agentRunID = spanID
		}

		// Add agent_run_id to the data map if not already present
		if _, ok := data["agent_run_id"]; !ok {
			data["agent_run_id"] = agentRunID
		}

		// Handle the event via the taxonomy engine
		if err := p.engine.HandleEvent(ctx, eventType, data); err != nil {
			p.logger.Error("failed to handle execution event",
				"event_type", eventType,
				"span_name", spanName,
				"trace_id", traceID,
				"span_id", spanID,
				"error", err)
		}
	}
}

// mapSpanToEventTypes maps OpenTelemetry span names to taxonomy event types.
// Returns a slice of event types because some spans (like mission.execute) map to multiple events.
func (p *GraphSpanProcessor) mapSpanToEventTypes(spanName string) []string {
	switch {
	case spanName == SpanGenAIChat || spanName == SpanGenAIChatStream:
		return []string{"llm.request.started", "llm.request.completed"}

	case spanName == SpanGenAITool:
		return []string{"tool.call.started", "tool.call.completed"}

	case spanName == SpanAgentDelegate:
		return []string{"agent.delegated"}

	case spanName == "gibson.mission.execute":
		// Mission spans represent both start and end
		return []string{"mission.started", "mission.completed"}

	case strings.HasPrefix(spanName, "gibson.agent."):
		// Agent spans represent both start and end
		return []string{"agent.started", "agent.completed"}

	default:
		// Unknown span type
		return []string{}
	}
}

// extractSpanData extracts all relevant data from a span into a map.
// This data will be used by the taxonomy engine to create nodes and relationships.
func (p *GraphSpanProcessor) extractSpanData(s sdktrace.ReadOnlySpan, traceID, spanID string) map[string]any {
	data := make(map[string]any)

	// Add trace/span identifiers
	data["trace_id"] = traceID
	data["span_id"] = spanID

	// Add timing information (convert to UTC to avoid Neo4j timezone issues)
	data["timestamp"] = s.StartTime().UTC()
	data["started_at"] = s.StartTime().UTC()
	data["ended_at"] = s.EndTime().UTC()
	data["duration_ms"] = s.EndTime().Sub(s.StartTime()).Milliseconds()

	// Extract all span attributes
	for _, attr := range s.Attributes() {
		key := string(attr.Key)
		value := attr.Value.AsInterface()

		// Map common Gibson attributes to expected field names
		switch key {
		case GibsonMissionID:
			// Parse mission ID as types.ID if possible
			if midStr, ok := value.(string); ok {
				if mid, err := types.ParseID(midStr); err == nil {
					data["mission_id"] = mid
				} else {
					data["mission_id"] = midStr
				}
			}
		case GibsonAgentName:
			data["agent_name"] = value
		case GibsonToolName:
			data["tool_name"] = value
		case GenAIResponseModel, GenAIRequestModel:
			// Use either request model or response model for the model field
			// Request model is set at start, response model is set at completion
			data["model"] = value
		case GenAISystem:
			data["provider"] = value
		case GenAIUsageInputTokens:
			data["tokens_in"] = value
		case GenAIUsageOutputTokens:
			data["tokens_out"] = value
		case GibsonLLMCost:
			data["cost_usd"] = value
		default:
			// Store all other attributes with their original names
			data[key] = value
		}
	}

	return data
}

// getSpanStatus determines the status string from a span's status code.
func (p *GraphSpanProcessor) getSpanStatus(s sdktrace.ReadOnlySpan) string {
	switch s.Status().Code {
	case codes.Ok:
		return "success"
	case codes.Error:
		return "error"
	default:
		return "unknown"
	}
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
