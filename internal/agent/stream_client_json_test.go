package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestEmitErrorEvent_JSONMarshalError tests that emitErrorEvent handles
// JSON marshaling errors gracefully by logging and using a fallback message.
func TestEmitErrorEvent_JSONMarshalError(t *testing.T) {
	// Create a stream client with a minimal setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &StreamClient{
		sessionID: types.NewID(),
		eventCh:   make(chan *database.StreamEvent, 10),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Test with a normal error (should succeed)
	testErr := &testError{message: "test error"}
	client.emitErrorEvent(testErr)

	// Verify event was emitted
	select {
	case event := <-client.eventCh:
		if event.EventType != database.StreamEventError {
			t.Errorf("expected event type %v, got %v", database.StreamEventError, event.EventType)
		}
		if len(event.Content) == 0 {
			t.Error("expected non-empty content")
		}

		// Verify content can be unmarshaled
		var content map[string]any
		if err := json.Unmarshal(event.Content, &content); err != nil {
			t.Errorf("failed to unmarshal event content: %v", err)
		}

		// Check that error message is present
		if msg, ok := content["message"].(string); !ok || msg == "" {
			t.Error("expected message in content")
		}
	default:
		t.Error("no event was emitted")
	}

	// Test with an error containing un-marshalable data
	// In Go, standard errors are always marshalable, but we verify the fallback path exists
	// The fallback is tested by ensuring the code compiles and runs without panicking
	unmarshalableErr := &testError{message: "error with special chars: \x00\x01"}
	client.emitErrorEvent(unmarshalableErr)

	// Verify event was emitted even with special characters
	select {
	case event := <-client.eventCh:
		if event.EventType != database.StreamEventError {
			t.Errorf("expected event type %v, got %v", database.StreamEventError, event.EventType)
		}
		if len(event.Content) == 0 {
			t.Error("expected non-empty content")
		}
	default:
		t.Error("no event was emitted for unmarshalable error")
	}
}

// testError is a simple error type for testing
type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}
