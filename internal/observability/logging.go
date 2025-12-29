package observability

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// TracedLogger is a structured logger with automatic trace correlation.
// It wraps slog.Logger and adds mission context and OpenTelemetry trace correlation.
type TracedLogger struct {
	logger          *slog.Logger
	missionID       string
	agentName       string
	redactSensitive bool
}

// NewTracedLogger creates a new TracedLogger with the specified handler and context.
// The logger automatically correlates logs with distributed traces and includes
// mission and agent context in every log entry.
//
// Parameters:
//   - handler: The slog.Handler to use for formatting and outputting logs
//   - missionID: The unique identifier for the current mission
//   - agentName: The name of the agent producing logs
//
// Returns:
//   - *TracedLogger: A configured logger ready for use
func NewTracedLogger(handler slog.Handler, missionID, agentName string) *TracedLogger {
	return &TracedLogger{
		logger:          slog.New(handler),
		missionID:       missionID,
		agentName:       agentName,
		redactSensitive: true,
	}
}

// Debug logs a debug-level message with automatic trace correlation.
// Debug logs include all fields without redaction.
//
// Parameters:
//   - ctx: Context containing OpenTelemetry span for trace correlation
//   - msg: The log message
//   - args: Optional key-value pairs for structured logging (must be even number)
func (l *TracedLogger) Debug(ctx context.Context, msg string, args ...any) {
	logger := l.WithContext(ctx)
	logger.Debug(msg, args...)
}

// Info logs an info-level message with automatic trace correlation.
// Sensitive data in args is redacted at info level and above.
//
// Parameters:
//   - ctx: Context containing OpenTelemetry span for trace correlation
//   - msg: The log message
//   - args: Optional key-value pairs for structured logging (must be even number)
func (l *TracedLogger) Info(ctx context.Context, msg string, args ...any) {
	logger := l.WithContext(ctx)
	if l.redactSensitive {
		args = redactSensitiveData(args)
	}
	logger.Info(msg, args...)
}

// Warn logs a warning-level message with automatic trace correlation.
// Sensitive data in args is redacted at warn level and above.
//
// Parameters:
//   - ctx: Context containing OpenTelemetry span for trace correlation
//   - msg: The log message
//   - args: Optional key-value pairs for structured logging (must be even number)
func (l *TracedLogger) Warn(ctx context.Context, msg string, args ...any) {
	logger := l.WithContext(ctx)
	if l.redactSensitive {
		args = redactSensitiveData(args)
	}
	logger.Warn(msg, args...)
}

// Error logs an error-level message with automatic trace correlation.
// Sensitive data in args is redacted at error level.
//
// Parameters:
//   - ctx: Context containing OpenTelemetry span for trace correlation
//   - msg: The log message
//   - args: Optional key-value pairs for structured logging (must be even number)
func (l *TracedLogger) Error(ctx context.Context, msg string, args ...any) {
	logger := l.WithContext(ctx)
	if l.redactSensitive {
		args = redactSensitiveData(args)
	}
	logger.Error(msg, args...)
}

// WithContext creates a new slog.Logger with trace correlation fields added.
// Extracts trace_id and span_id from the OpenTelemetry span in the context
// and adds mission_id and agent_name to every log entry.
//
// Parameters:
//   - ctx: Context containing OpenTelemetry span for trace correlation
//
// Returns:
//   - *slog.Logger: A logger with trace correlation fields
func (l *TracedLogger) WithContext(ctx context.Context) *slog.Logger {
	logger := l.logger

	// Add mission and agent context
	logger = logger.With(
		slog.String("mission_id", l.missionID),
		slog.String("agent_name", l.agentName),
	)

	// Extract trace context from OpenTelemetry
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		spanCtx := span.SpanContext()
		logger = logger.With(
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}

	return logger
}

// NewJSONHandler creates a new JSON log handler with the specified output and level.
// JSON format is ideal for structured logging in production environments.
//
// Parameters:
//   - w: The writer to output logs to (e.g., os.Stdout, file)
//   - level: The minimum log level to output
//
// Returns:
//   - slog.Handler: A configured JSON handler
func NewJSONHandler(w io.Writer, level slog.Level) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	})
}

// NewTextHandler creates a new text log handler with the specified output and level.
// Text format is human-readable and useful for development and debugging.
//
// Parameters:
//   - w: The writer to output logs to (e.g., os.Stdout, file)
//   - level: The minimum log level to output
//
// Returns:
//   - slog.Handler: A configured text handler
func NewTextHandler(w io.Writer, level slog.Level) slog.Handler {
	return slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	})
}

// redactSensitiveData redacts sensitive fields in log arguments.
// Sensitive fields include: prompt, prompts, api_key, secret, password, token, credential.
// These fields are replaced with "[REDACTED]" to prevent sensitive data leakage.
//
// Parameters:
//   - args: Key-value pairs from logging call (must be even number)
//
// Returns:
//   - []any: Args with sensitive values redacted
func redactSensitiveData(args []any) []any {
	if len(args)%2 != 0 {
		// Invalid args, return as-is
		return args
	}

	// List of sensitive field names to redact
	sensitiveFields := map[string]bool{
		"prompt":     true,
		"prompts":    true,
		"api_key":    true,
		"secret":     true,
		"password":   true,
		"token":      true,
		"credential": true,
		"apikey":     true,
		"secretkey":  true,
	}

	redacted := make([]any, len(args))
	copy(redacted, args)

	for i := 0; i < len(args); i += 2 {
		// Check if key is a string and if it's sensitive
		if key, ok := args[i].(string); ok {
			normalizedKey := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			if sensitiveFields[normalizedKey] {
				redacted[i+1] = "[REDACTED]"
			}
		}
	}

	return redacted
}
