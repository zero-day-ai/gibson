package verbose

import (
	"context"
	"log/slog"
	"strings"
)

// VerboseAwareHandler is an slog.Handler that routes log records to VerboseEventBus
// while passing them through to an inner handler. It converts slog levels to VerboseLevel
// and applies redaction for sensitive fields.
type VerboseAwareHandler struct {
	inner    slog.Handler
	bus      VerboseEventBus
	minLevel VerboseLevel
	attrs    []slog.Attr
	groups   []string
}

// NewVerboseAwareHandler creates a new VerboseAwareHandler that wraps an inner handler.
//
// Parameters:
//   - inner: The underlying slog.Handler to wrap (logs always pass through to this)
//   - bus: The VerboseEventBus to emit events to
//   - level: Minimum verbosity level (events below this level are filtered)
//
// Returns:
//   - *VerboseAwareHandler: A handler that emits to both the event bus and inner handler
func NewVerboseAwareHandler(inner slog.Handler, bus VerboseEventBus, level VerboseLevel) *VerboseAwareHandler {
	return &VerboseAwareHandler{
		inner:    inner,
		bus:      bus,
		minLevel: level,
		attrs:    nil,
		groups:   nil,
	}
}

// Enabled reports whether the handler handles records at the given level.
// This delegates to the inner handler to maintain its behavior.
func (h *VerboseAwareHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle processes a log record by:
// 1. Converting it to a VerboseEvent and emitting to the bus
// 2. Passing it through to the inner handler
//
// This ensures all logs are captured for verbose output while maintaining
// normal logging behavior.
func (h *VerboseAwareHandler) Handle(ctx context.Context, record slog.Record) error {
	// Convert slog record to VerboseEvent
	event := h.recordToEvent(record)

	// Only emit to bus if level is sufficient
	if event.Level <= h.minLevel {
		// Emit to verbose event bus (ignore errors to avoid disrupting logging)
		_ = h.bus.Emit(ctx, event)
	}

	// Always pass through to inner handler
	return h.inner.Handle(ctx, record)
}

// WithAttrs returns a new handler with additional attributes.
// This is required by slog.Handler interface to support logger.With().
func (h *VerboseAwareHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Apply redaction to sensitive attributes
	redactedAttrs := h.redactAttrs(attrs)

	return &VerboseAwareHandler{
		inner:    h.inner.WithAttrs(redactedAttrs),
		bus:      h.bus,
		minLevel: h.minLevel,
		attrs:    append(h.attrs, redactedAttrs...),
		groups:   h.groups,
	}
}

// WithGroup returns a new handler with a group name.
// This is required by slog.Handler interface to support structured logging groups.
func (h *VerboseAwareHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	return &VerboseAwareHandler{
		inner:    h.inner.WithGroup(name),
		bus:      h.bus,
		minLevel: h.minLevel,
		attrs:    h.attrs,
		groups:   append(h.groups, name),
	}
}

// recordToEvent converts an slog.Record to a VerboseEvent.
// Maps slog levels to VerboseLevel and extracts relevant fields.
func (h *VerboseAwareHandler) recordToEvent(record slog.Record) VerboseEvent {
	// Map slog level to VerboseLevel
	// Debug → LevelDebug(3)
	// Info → LevelVerbose(1)
	// Warn → LevelVerbose(1)
	// Error → LevelVerbose(1)
	var verboseLevel VerboseLevel
	switch record.Level {
	case slog.LevelDebug:
		verboseLevel = LevelDebug
	case slog.LevelInfo, slog.LevelWarn, slog.LevelError:
		verboseLevel = LevelVerbose
	default:
		verboseLevel = LevelVerbose
	}

	// Build payload from record attributes
	payload := make(map[string]any)
	payload["message"] = record.Message
	payload["level"] = record.Level.String()

	// Add handler-level attributes
	for _, attr := range h.attrs {
		payload[attr.Key] = attr.Value.Any()
	}

	// Add record-level attributes
	record.Attrs(func(attr slog.Attr) bool {
		payload[attr.Key] = attr.Value.Any()
		return true
	})

	// Apply redaction to payload
	redactedPayload := h.redactPayload(payload)

	// Create event with generic system log type
	// We use a special event type for slog messages
	eventType := VerboseEventType("system.slog." + strings.ToLower(record.Level.String()))

	return VerboseEvent{
		Type:      eventType,
		Level:     verboseLevel,
		Timestamp: record.Time,
		Payload:   redactedPayload,
	}
}

// redactAttrs applies redaction to sensitive slog attributes.
func (h *VerboseAwareHandler) redactAttrs(attrs []slog.Attr) []slog.Attr {
	redacted := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		if h.isSensitiveKey(attr.Key) {
			redacted[i] = slog.String(attr.Key, "[REDACTED]")
		} else {
			redacted[i] = attr
		}
	}
	return redacted
}

// redactPayload applies redaction to sensitive fields in the payload map.
func (h *VerboseAwareHandler) redactPayload(payload map[string]any) map[string]any {
	redacted := make(map[string]any, len(payload))
	for key, value := range payload {
		if h.isSensitiveKey(key) {
			redacted[key] = "[REDACTED]"
		} else {
			redacted[key] = value
		}
	}
	return redacted
}

// isSensitiveKey checks if a key name is considered sensitive.
// Matches the sensitive fields list from internal/observability/logging.go:
// prompt, prompts, api_key, secret, password, token, credential
func (h *VerboseAwareHandler) isSensitiveKey(key string) bool {
	// Normalize key: lowercase and remove underscores
	normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))

	sensitiveFields := map[string]bool{
		"prompt":     true,
		"prompts":    true,
		"apikey":     true,
		"secret":     true,
		"password":   true,
		"token":      true,
		"credential": true,
		"secretkey":  true,
	}

	return sensitiveFields[normalized]
}

// Ensure VerboseAwareHandler implements slog.Handler at compile time
var _ slog.Handler = (*VerboseAwareHandler)(nil)
