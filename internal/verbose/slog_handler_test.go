package verbose

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
)

func TestVerboseAwareHandler_Enabled(t *testing.T) {
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	innerHandler := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	handler := NewVerboseAwareHandler(innerHandler, bus, LevelVerbose)

	ctx := context.Background()

	// Should delegate to inner handler
	if !handler.Enabled(ctx, slog.LevelInfo) {
		t.Error("Expected handler to be enabled for LevelInfo")
	}

	if handler.Enabled(ctx, slog.LevelDebug) {
		t.Error("Expected handler to be disabled for LevelDebug (inner handler is Info level)")
	}
}

func TestVerboseAwareHandler_Handle(t *testing.T) {
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	var innerBuf bytes.Buffer
	innerHandler := slog.NewTextHandler(&innerBuf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	handler := NewVerboseAwareHandler(innerHandler, bus, LevelVerbose)

	// Subscribe to bus to verify events are emitted
	ctx := context.Background()
	events, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Create and handle a log record
	logger := slog.New(handler)
	logger.Info("test message", "key", "value")

	// Verify event was emitted to bus
	select {
	case event := <-events:
		if event.Type != "system.slog.info" {
			t.Errorf("Expected event type 'system.slog.info', got '%s'", event.Type)
		}
		if event.Level != LevelVerbose {
			t.Errorf("Expected level LevelVerbose, got %v", event.Level)
		}
		payload, ok := event.Payload.(map[string]any)
		if !ok {
			t.Fatal("Expected payload to be map[string]any")
		}
		if payload["message"] != "test message" {
			t.Errorf("Expected message 'test message', got '%v'", payload["message"])
		}
	default:
		t.Error("Expected event to be emitted to bus")
	}

	// Verify log was written to inner handler
	if innerBuf.Len() == 0 {
		t.Error("Expected log to be written to inner handler")
	}
}

func TestVerboseAwareHandler_Redaction(t *testing.T) {
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	innerHandler := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	handler := NewVerboseAwareHandler(innerHandler, bus, LevelVerbose)

	// Subscribe to bus
	ctx := context.Background()
	events, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Log with sensitive data
	logger := slog.New(handler)
	logger.Info("test", "api_key", "secret123", "password", "pass123", "normal", "value")

	// Verify sensitive fields are redacted
	select {
	case event := <-events:
		payload, ok := event.Payload.(map[string]any)
		if !ok {
			t.Fatal("Expected payload to be map[string]any")
		}
		if payload["api_key"] != "[REDACTED]" {
			t.Errorf("Expected api_key to be redacted, got '%v'", payload["api_key"])
		}
		if payload["password"] != "[REDACTED]" {
			t.Errorf("Expected password to be redacted, got '%v'", payload["password"])
		}
		if payload["normal"] != "value" {
			t.Errorf("Expected normal field to not be redacted, got '%v'", payload["normal"])
		}
	default:
		t.Error("Expected event to be emitted")
	}
}

func TestVerboseAwareHandler_LevelMapping(t *testing.T) {
	tests := []struct {
		name          string
		slogLevel     slog.Level
		expectedLevel VerboseLevel
	}{
		{
			name:          "Debug maps to LevelDebug",
			slogLevel:     slog.LevelDebug,
			expectedLevel: LevelDebug,
		},
		{
			name:          "Info maps to LevelVerbose",
			slogLevel:     slog.LevelInfo,
			expectedLevel: LevelVerbose,
		},
		{
			name:          "Warn maps to LevelVerbose",
			slogLevel:     slog.LevelWarn,
			expectedLevel: LevelVerbose,
		},
		{
			name:          "Error maps to LevelVerbose",
			slogLevel:     slog.LevelError,
			expectedLevel: LevelVerbose,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := NewDefaultVerboseEventBus()
			defer bus.Close()

			innerHandler := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{
				Level: slog.LevelDebug, // Accept all levels
			})

			handler := NewVerboseAwareHandler(innerHandler, bus, LevelDebug)

			ctx := context.Background()
			events, cleanup := bus.Subscribe(ctx)
			defer cleanup()

			// Create log at the specified level
			logger := slog.New(handler)
			switch tt.slogLevel {
			case slog.LevelDebug:
				logger.Debug("test")
			case slog.LevelInfo:
				logger.Info("test")
			case slog.LevelWarn:
				logger.Warn("test")
			case slog.LevelError:
				logger.Error("test")
			}

			// Verify the event level
			select {
			case event := <-events:
				if event.Level != tt.expectedLevel {
					t.Errorf("Expected level %v, got %v", tt.expectedLevel, event.Level)
				}
			default:
				t.Error("Expected event to be emitted")
			}
		})
	}
}

func TestVerboseAwareHandler_WithAttrs(t *testing.T) {
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	innerHandler := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	handler := NewVerboseAwareHandler(innerHandler, bus, LevelVerbose)

	// Add attributes with sensitive data
	handlerWithAttrs := handler.WithAttrs([]slog.Attr{
		slog.String("api_key", "secret"),
		slog.String("normal", "value"),
	})

	ctx := context.Background()
	events, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Log with the handler that has attributes
	logger := slog.New(handlerWithAttrs)
	logger.Info("test message")

	// Verify attributes are included and sensitive ones are redacted
	select {
	case event := <-events:
		payload, ok := event.Payload.(map[string]any)
		if !ok {
			t.Fatal("Expected payload to be map[string]any")
		}
		if payload["api_key"] != "[REDACTED]" {
			t.Errorf("Expected api_key to be redacted, got '%v'", payload["api_key"])
		}
		if payload["normal"] != "value" {
			t.Errorf("Expected normal attribute, got '%v'", payload["normal"])
		}
	default:
		t.Error("Expected event to be emitted")
	}
}

func TestVerboseAwareHandler_WithGroup(t *testing.T) {
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	innerHandler := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	handler := NewVerboseAwareHandler(innerHandler, bus, LevelVerbose)

	// Create handler with group
	handlerWithGroup := handler.WithGroup("mygroup")

	// Verify it returns a new handler
	if handlerWithGroup == handler {
		t.Error("Expected WithGroup to return a new handler")
	}

	// Verify empty group name returns same handler
	handlerSame := handler.WithGroup("")
	if handlerSame != handler {
		t.Error("Expected WithGroup(\"\") to return same handler")
	}
}

func TestVerboseAwareHandler_FilterByLevel(t *testing.T) {
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	innerHandler := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{
		Level: slog.LevelDebug, // Accept all levels
	})

	// Create handler with LevelVerbose minimum (filters out Debug)
	handler := NewVerboseAwareHandler(innerHandler, bus, LevelVerbose)

	ctx := context.Background()
	events, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	logger := slog.New(handler)

	// Log at Debug level (should not emit to bus because it's LevelDebug which is > LevelVerbose)
	logger.Debug("debug message")

	// Verify no event was emitted
	select {
	case <-events:
		// We might get an event because Debug maps to LevelDebug (3) which is > LevelVerbose (1)
		// So the event should be filtered by the handler
		t.Error("Did not expect debug event to be emitted when min level is LevelVerbose")
	default:
		// Expected: no event emitted
	}

	// Log at Info level (should emit)
	logger.Info("info message")

	select {
	case event := <-events:
		if event.Level != LevelVerbose {
			t.Errorf("Expected LevelVerbose, got %v", event.Level)
		}
	default:
		t.Error("Expected info event to be emitted")
	}
}
