package observability

import (
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestGraphSpanProcessorImplementsInterface verifies that GraphSpanProcessor
// implements the sdktrace.SpanProcessor interface at compile time.
func TestGraphSpanProcessorImplementsInterface(t *testing.T) {
	// This will fail to compile if GraphSpanProcessor doesn't implement SpanProcessor
	var _ sdktrace.SpanProcessor = (*GraphSpanProcessor)(nil)
}
