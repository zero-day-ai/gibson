package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TestExtractOTELTraceID tests the OTEL trace ID extraction helper function.
func TestExtractOTELTraceID(t *testing.T) {
	tests := []struct {
		name        string
		setupCtx    func() context.Context
		expectEmpty bool
	}{
		{
			name: "context with valid span",
			setupCtx: func() context.Context {
				// Setup a real tracer provider that generates valid trace IDs
				tp := sdktrace.NewTracerProvider()
				otel.SetTracerProvider(tp)

				ctx := context.Background()
				tracer := otel.Tracer("test")
				ctx, _ = tracer.Start(ctx, "test-span")
				return ctx
			},
			expectEmpty: false,
		},
		{
			name: "context without span",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expectEmpty: true,
		},
		{
			name: "nil context span",
			setupCtx: func() context.Context {
				return trace.ContextWithSpan(context.Background(), trace.SpanFromContext(context.Background()))
			},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			traceID := extractOTELTraceID(ctx)

			if tt.expectEmpty {
				if traceID != "" {
					t.Errorf("expected empty trace ID, got %s", traceID)
				}
			} else {
				if traceID == "" {
					t.Error("expected non-empty trace ID, got empty")
				}
				// Verify it's a valid hex string
				if len(traceID) != 32 {
					t.Errorf("expected trace ID length 32, got %d", len(traceID))
				}
			}
		})
	}
}

// TestExtractOTELTraceIDFromContext tests the helper function used in langfuse_tracer.go.
func TestExtractOTELTraceIDFromContext(t *testing.T) {
	tests := []struct {
		name        string
		setupCtx    func() context.Context
		expectEmpty bool
	}{
		{
			name: "context with valid span",
			setupCtx: func() context.Context {
				// Setup a real tracer provider that generates valid trace IDs
				tp := sdktrace.NewTracerProvider()
				otel.SetTracerProvider(tp)

				ctx := context.Background()
				tracer := otel.Tracer("test")
				ctx, _ = tracer.Start(ctx, "test-span")
				return ctx
			},
			expectEmpty: false,
		},
		{
			name: "context without span",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			traceID := extractOTELTraceIDFromContext(ctx)

			if tt.expectEmpty {
				if traceID != "" {
					t.Errorf("expected empty trace ID, got %s", traceID)
				}
			} else {
				if traceID == "" {
					t.Error("expected non-empty trace ID, got empty")
				}
				// Verify it's a valid hex string
				if len(traceID) != 32 {
					t.Errorf("expected trace ID length 32, got %d", len(traceID))
				}
			}
		})
	}
}
