package console

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestNewEventProcessor(t *testing.T) {
	eventChan := make(chan *database.StreamEvent)
	processor := NewEventProcessor(eventChan, nil, "test-agent")

	if processor == nil {
		t.Fatal("expected non-nil processor")
	}
	if processor.agentName != "test-agent" {
		t.Errorf("agentName = %q, want %q", processor.agentName, "test-agent")
	}
	if processor.running {
		t.Error("processor should not be running initially")
	}
	if processor.stopChan == nil {
		t.Error("stopChan should not be nil")
	}
}

func TestEventProcessor_AgentName(t *testing.T) {
	eventChan := make(chan *database.StreamEvent)
	processor := NewEventProcessor(eventChan, nil, "davinci")

	if got := processor.AgentName(); got != "davinci" {
		t.Errorf("AgentName() = %q, want %q", got, "davinci")
	}
}

func TestEventProcessor_IsRunning(t *testing.T) {
	eventChan := make(chan *database.StreamEvent)
	processor := NewEventProcessor(eventChan, nil, "test")

	if processor.IsRunning() {
		t.Error("should not be running before Start()")
	}

	processor.Start()
	if !processor.IsRunning() {
		t.Error("should be running after Start()")
	}

	processor.Stop()
	// Give goroutine time to clean up
	time.Sleep(10 * time.Millisecond)
	if processor.IsRunning() {
		t.Error("should not be running after Stop()")
	}
}

func TestEventProcessor_StartStop(t *testing.T) {
	eventChan := make(chan *database.StreamEvent)
	processor := NewEventProcessor(eventChan, nil, "test")

	// Start should be idempotent
	processor.Start()
	processor.Start() // Should not panic or create duplicate goroutines

	// Stop should be idempotent
	processor.Stop()
	processor.Stop() // Should not panic
}

func TestEventProcessor_ProcessesEvents(t *testing.T) {
	eventChan := make(chan *database.StreamEvent, 10)

	// We can't use a mock directly with tea.Program since it's a concrete type,
	// but we can test the processor logic with a nil program
	processor := NewEventProcessor(eventChan, nil, "test-agent")

	// Create a test event
	event := &database.StreamEvent{
		ID:        types.NewID(),
		EventType: database.StreamEventOutput,
		Content:   json.RawMessage(`{"text": "hello", "complete": true}`),
		Timestamp: time.Now(),
	}

	// Send event to channel
	eventChan <- event

	// Start processor (it will read the event but can't send without program)
	processor.Start()

	// Give it time to process
	time.Sleep(20 * time.Millisecond)

	processor.Stop()
}

func TestEventProcessor_StopsOnChannelClose(t *testing.T) {
	eventChan := make(chan *database.StreamEvent)
	processor := NewEventProcessor(eventChan, nil, "test")

	processor.Start()

	// Close the channel - processor should detect and stop
	close(eventChan)

	// Wait for processor to notice and stop
	time.Sleep(50 * time.Millisecond)

	if processor.IsRunning() {
		t.Error("processor should stop when channel is closed")
	}
}

func TestEventProcessor_StopsOnStopSignal(t *testing.T) {
	eventChan := make(chan *database.StreamEvent)
	processor := NewEventProcessor(eventChan, nil, "test")

	processor.Start()

	// Verify it's running
	if !processor.IsRunning() {
		t.Fatal("processor should be running")
	}

	// Send stop signal
	processor.Stop()

	// Wait for cleanup
	time.Sleep(50 * time.Millisecond)

	if processor.IsRunning() {
		t.Error("processor should stop after Stop() call")
	}
}

func TestEventProcessor_NilProgram(t *testing.T) {
	eventChan := make(chan *database.StreamEvent, 1)
	processor := NewEventProcessor(eventChan, nil, "test")

	event := &database.StreamEvent{
		ID:        types.NewID(),
		EventType: database.StreamEventOutput,
		Content:   json.RawMessage(`{"text": "hello"}`),
		Timestamp: time.Now(),
	}

	processor.Start()

	// This should not panic even with nil program
	eventChan <- event

	time.Sleep(20 * time.Millisecond)

	processor.Stop()
}

func TestAgentEventMsg(t *testing.T) {
	event := &database.StreamEvent{
		ID:        types.NewID(),
		EventType: database.StreamEventOutput,
		Content:   json.RawMessage(`{"text": "test"}`),
		Timestamp: time.Now(),
	}

	msg := AgentEventMsg{
		AgentName: "davinci",
		Event:     event,
	}

	if msg.AgentName != "davinci" {
		t.Errorf("AgentName = %q, want %q", msg.AgentName, "davinci")
	}
	if msg.Event != event {
		t.Error("Event not set correctly")
	}
}

func TestAgentDisconnectedMsg(t *testing.T) {
	msg := AgentDisconnectedMsg{
		AgentName: "davinci",
		Reason:    "stream closed",
	}

	if msg.AgentName != "davinci" {
		t.Errorf("AgentName = %q, want %q", msg.AgentName, "davinci")
	}
	if msg.Reason != "stream closed" {
		t.Errorf("Reason = %q, want %q", msg.Reason, "stream closed")
	}
}

func TestEventProcessor_MultipleEvents(t *testing.T) {
	eventChan := make(chan *database.StreamEvent, 100)
	processor := NewEventProcessor(eventChan, nil, "test")

	// Send multiple events before starting
	for i := 0; i < 10; i++ {
		event := &database.StreamEvent{
			ID:        types.NewID(),
			Sequence:  int64(i),
			EventType: database.StreamEventOutput,
			Content:   json.RawMessage(`{"text": "event"}`),
			Timestamp: time.Now(),
		}
		eventChan <- event
	}

	processor.Start()

	// Give time to process all events
	time.Sleep(50 * time.Millisecond)

	processor.Stop()
}

func TestEventProcessor_ConcurrentStartStop(t *testing.T) {
	eventChan := make(chan *database.StreamEvent, 10)
	processor := NewEventProcessor(eventChan, nil, "test")

	// Test concurrent start/stop - should not panic or deadlock
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			processor.Start()
		}()
		go func() {
			defer wg.Done()
			processor.Stop()
		}()
	}

	// Use timeout to avoid hanging
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("concurrent start/stop timed out - possible deadlock")
	}
}

func TestEventProcessor_DifferentEventTypes(t *testing.T) {
	eventChan := make(chan *database.StreamEvent, 10)
	processor := NewEventProcessor(eventChan, nil, "test")

	eventTypes := []database.StreamEventType{
		database.StreamEventOutput,
		database.StreamEventToolCall,
		database.StreamEventToolResult,
		database.StreamEventFinding,
		database.StreamEventStatus,
		database.StreamEventSteeringAck,
		database.StreamEventError,
	}

	processor.Start()

	for _, et := range eventTypes {
		event := &database.StreamEvent{
			ID:        types.NewID(),
			EventType: et,
			Content:   json.RawMessage(`{}`),
			Timestamp: time.Now(),
		}
		eventChan <- event
	}

	time.Sleep(50 * time.Millisecond)
	processor.Stop()
}
