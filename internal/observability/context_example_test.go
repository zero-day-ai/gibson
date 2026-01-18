package observability_test

import (
	"context"
	"fmt"

	"github.com/zero-day-ai/gibson/internal/observability"
	"go.opentelemetry.io/otel"
)

// Example demonstrates how to use ExtractParentSpanID for event emission.
// This shows the typical pattern for creating child spans and emitting events
// with proper parent_span_id for relationship creation in the graph.
func Example_extractParentSpanID() {
	// Get a tracer
	tracer := otel.Tracer("example")

	// Parent context (e.g., from agent execution)
	ctx := context.Background()
	ctx, parentSpan := tracer.Start(ctx, "agent.execute")
	defer parentSpan.End()

	// Before creating a child span for an LLM call, extract parent span ID
	parentSpanID := observability.ExtractParentSpanID(ctx)

	// Create child span for LLM request
	ctx, llmSpan := tracer.Start(ctx, "llm.request")
	defer llmSpan.End()

	// Emit event with all required fields including parent_span_id
	fmt.Printf("Event: trace=%s span=%s parent=%s\n",
		llmSpan.SpanContext().TraceID().String(),
		llmSpan.SpanContext().SpanID().String(),
		parentSpanID)

	// The parentSpanID will be used to create MADE_CALL relationship:
	// (parentSpan:AgentExecution)-[:MADE_CALL]->(llmSpan:LLMRequest)

	// Output:
	// Event: trace=00000000000000000000000000000000 span=0000000000000000 parent=0000000000000000
}

// Example demonstrates how to use ExtractSpanContext for comprehensive event data.
func Example_extractSpanContext() {
	tracer := otel.Tracer("example")

	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "tool.execute")
	defer span.End()

	// Extract all span context at once
	traceID, spanID, parentSpanID := observability.ExtractSpanContext(ctx)

	// Use in event emission
	fmt.Printf("ToolCallEvent: trace=%s span=%s parent=%s\n",
		traceID, spanID, parentSpanID)

	// Output:
	// ToolCallEvent: trace=00000000000000000000000000000000 span=0000000000000000 parent=0000000000000000
}
