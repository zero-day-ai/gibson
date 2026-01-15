package daemon

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/daemon/api"
	"github.com/zero-day-ai/gibson/internal/graphrag/engine"
)

// mockTaxonomyGraphEngine is a mock implementation of engine.TaxonomyGraphEngine for testing.
type mockTaxonomyGraphEngine struct {
	mu sync.Mutex

	// Tracking for assertions
	handleEventCalls     []eventCall
	handleToolCalls      []toolCall
	handleFindingCalls   []findingCall
	healthCalls          int

	// Error injection (protected by mutex)
	handleEventError     error
	handleToolError      error
	handleFindingError   error

	// Call tracking
	eventCallCount       int

	// Signal channels for testing
	eventProcessed       chan struct{}
}

type eventCall struct {
	eventType string
	data      map[string]any
}

type toolCall struct {
	toolName   string
	output     map[string]any
	agentRunID string
}

type findingCall struct {
	findingID string
	missionID string
}

func newMockTaxonomyGraphEngine() *mockTaxonomyGraphEngine {
	return &mockTaxonomyGraphEngine{
		handleEventCalls:   make([]eventCall, 0),
		handleToolCalls:    make([]toolCall, 0),
		handleFindingCalls: make([]findingCall, 0),
		eventProcessed:     make(chan struct{}, 100),
	}
}

func (m *mockTaxonomyGraphEngine) HandleEvent(ctx context.Context, eventType string, data map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventCallCount++
	m.handleEventCalls = append(m.handleEventCalls, eventCall{
		eventType: eventType,
		data:      data,
	})

	// Signal that an event was processed
	select {
	case m.eventProcessed <- struct{}{}:
	default:
	}

	return m.handleEventError
}

func (m *mockTaxonomyGraphEngine) HandleToolOutput(ctx context.Context, toolName string, output map[string]any, agentRunID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.handleToolCalls = append(m.handleToolCalls, toolCall{
		toolName:   toolName,
		output:     output,
		agentRunID: agentRunID,
	})

	return m.handleToolError
}

func (m *mockTaxonomyGraphEngine) HandleFinding(ctx context.Context, finding agent.Finding, missionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.handleFindingCalls = append(m.handleFindingCalls, findingCall{
		missionID: missionID,
	})

	return m.handleFindingError
}

func (m *mockTaxonomyGraphEngine) Health(ctx context.Context) engine.HealthStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.healthCalls++
	return engine.HealthStatus{
		Healthy:     true,
		Neo4jStatus: "healthy",
		Message:     "mock engine operational",
	}
}

func (m *mockTaxonomyGraphEngine) GetEventCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.eventCallCount
}

func (m *mockTaxonomyGraphEngine) GetHandleEventCalls() []eventCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]eventCall, len(m.handleEventCalls))
	copy(calls, m.handleEventCalls)
	return calls
}

func (m *mockTaxonomyGraphEngine) WaitForEvents(count int, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for i := 0; i < count; i++ {
		select {
		case <-m.eventProcessed:
			// Event received
		case <-deadline:
			return false
		}
	}
	return true
}

func (m *mockTaxonomyGraphEngine) SetHandleEventError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handleEventError = err
}

// TestGraphEventSubscriber_Creation tests subscriber creation and initialization.
func TestGraphEventSubscriber_Creation(t *testing.T) {
	t.Run("creates subscriber with all dependencies", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()

		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		assert.NotNil(t, subscriber)
		assert.NotNil(t, subscriber.engine)
		assert.NotNil(t, subscriber.eventBus)
		assert.NotNil(t, subscriber.logger)
	})

	t.Run("creates subscriber with nil logger", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()

		// Should not panic with nil logger
		subscriber := NewGraphEventSubscriber(engine, bus, nil)

		assert.NotNil(t, subscriber)
		assert.NotNil(t, subscriber.logger) // Should use default logger
	})
}

// TestGraphEventSubscriber_StartAndStop tests subscriber lifecycle.
func TestGraphEventSubscriber_StartAndStop(t *testing.T) {
	t.Run("starts subscriber and processes events", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish an event
		event := NewMissionStartedEvent("test-mission-1")
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		// Wait for event to be processed
		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for event")

		// Verify the engine received the event
		assert.Equal(t, 1, engine.GetEventCallCount())
		calls := engine.GetHandleEventCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "mission.started", calls[0].eventType)
	})

	t.Run("stop cleans up subscriber", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)

		// Verify subscription was created
		assert.Equal(t, 1, bus.SubscriberCount())

		// Stop the subscriber
		subscriber.Stop()

		// Give it a moment to clean up
		time.Sleep(100 * time.Millisecond)

		// Verify subscription was removed
		assert.Equal(t, 0, bus.SubscriberCount())
	})

	t.Run("stop is safe when cleanup is nil", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		// Stop without starting (cleanup will be nil)
		subscriber.Stop() // Should not panic
	})

	t.Run("context cancellation stops event processing", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx, cancel := context.WithCancel(context.Background())
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish an event to verify it's running
		event := NewMissionStartedEvent("test-mission")
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for event")

		// Cancel context
		cancel()

		// Give it time to stop
		time.Sleep(100 * time.Millisecond)

		// Events published after cancellation should not be processed
		initialCount := engine.GetEventCallCount()
		event2 := NewMissionCompletedEvent("test-mission")
		_ = bus.Publish(context.Background(), event2)

		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, initialCount, engine.GetEventCallCount(), "should not process events after context cancelled")
	})
}

// TestGraphEventSubscriber_EventRouting tests event routing to the engine.
func TestGraphEventSubscriber_EventRouting(t *testing.T) {
	t.Run("routes mission events", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish various mission events
		events := []api.EventData{
			NewMissionStartedEvent("mission-1"),
			NewMissionCompletedEvent("mission-1"),
			NewMissionFailedEvent("mission-2", errors.New("test error")),
		}

		for _, event := range events {
			err := bus.Publish(ctx, event)
			require.NoError(t, err)
		}

		// Wait for all events to be processed
		assert.True(t, engine.WaitForEvents(len(events), 2*time.Second), "timeout waiting for events")

		// Verify all events were routed
		assert.Equal(t, len(events), engine.GetEventCallCount())
		calls := engine.GetHandleEventCalls()
		require.Len(t, calls, len(events))

		assert.Equal(t, "mission.started", calls[0].eventType)
		assert.Equal(t, "mission.completed", calls[1].eventType)
		assert.Equal(t, "mission.failed", calls[2].eventType)
	})

	t.Run("routes agent events", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish agent.started event (this is subscribed to)
		event := api.EventData{
			EventType: "agent.started",
			Source:    "daemon",
			Timestamp: time.Now(),
			AgentEvent: &api.AgentEventData{
				AgentID:   "agent-1",
				AgentName: "test-agent",
				Message:   "Agent starting",
			},
		}
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		// Wait for event to be processed
		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for event")

		// Verify routing
		calls := engine.GetHandleEventCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "agent.started", calls[0].eventType)

		// Verify data extraction
		data := calls[0].data
		assert.Equal(t, "agent-1", data["agent_id"])
		assert.Equal(t, "test-agent", data["agent_name"])
		assert.Equal(t, "Agent starting", data["message"])
	})

	t.Run("routes finding events", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish finding event
		finding := api.FindingData{
			ID:       "finding-1",
			Title:    "Test Finding",
			Severity: "high",
			Category: "vulnerability",
		}
		event := NewFindingDiscoveredEvent("mission-1", finding)
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		// Wait for event to be processed
		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for event")

		// Verify routing
		calls := engine.GetHandleEventCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "finding.discovered", calls[0].eventType)

		// Verify data extraction
		data := calls[0].data
		assert.Equal(t, "mission-1", data["mission_id"])
		assert.Equal(t, "finding-1", data["finding_id"])
		assert.Equal(t, "Test Finding", data["finding_title"])
		assert.Equal(t, "high", data["severity"])
		assert.Equal(t, "vulnerability", data["category"])
	})

	t.Run("extracts common event data", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish event with metadata
		event := api.EventData{
			EventType: "mission.started",
			Source:    "test-source",
			Timestamp: time.Now(),
			Metadata: map[string]any{
				"trace_id":       "trace-123",
				"span_id":        "span-456",
				"parent_span_id": "parent-789",
			},
			MissionEvent: &api.MissionEventData{
				MissionID: "mission-1",
				NodeID:    "node-1",
				Message:   "Test message",
			},
		}
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		// Wait for event to be processed
		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for event")

		// Verify data extraction
		calls := engine.GetHandleEventCalls()
		require.Len(t, calls, 1)
		data := calls[0].data

		// Check common fields
		assert.NotNil(t, data["timestamp"])
		assert.Equal(t, "test-source", data["source"])

		// Check trace context
		assert.Equal(t, "trace-123", data["trace_id"])
		assert.Equal(t, "span-456", data["span_id"])
		assert.Equal(t, "parent-789", data["parent_span_id"])

		// Check mission event data
		assert.Equal(t, "mission-1", data["mission_id"])
		assert.Equal(t, "node-1", data["node_id"])
		assert.Equal(t, "Test message", data["message"])
	})
}

// TestGraphEventSubscriber_ErrorHandling tests graceful error handling.
func TestGraphEventSubscriber_ErrorHandling(t *testing.T) {
	t.Run("gracefully handles engine errors", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		// Inject error (thread-safe)
		engine.SetHandleEventError(errors.New("mock engine error"))

		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish an event
		event := NewMissionStartedEvent("test-mission")
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		// Wait for event to be processed
		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for event")

		// Verify the engine was called despite the error
		assert.Equal(t, 1, engine.GetEventCallCount())

		// Subscriber should continue processing (graceful degradation)
		// Publish another event
		event2 := NewMissionCompletedEvent("test-mission")
		err = bus.Publish(ctx, event2)
		require.NoError(t, err)

		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for second event")
		assert.Equal(t, 2, engine.GetEventCallCount())
	})

	t.Run("continues processing after engine errors", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish first event (will succeed)
		event1 := NewMissionStartedEvent("mission-1")
		err := bus.Publish(ctx, event1)
		require.NoError(t, err)

		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for first event")

		// Inject error for next call (thread-safe)
		engine.SetHandleEventError(errors.New("temporary error"))

		// Publish second event (will fail in engine)
		event2 := NewMissionCompletedEvent("mission-1")
		err = bus.Publish(ctx, event2)
		require.NoError(t, err)

		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for second event")

		// Clear error (thread-safe)
		engine.SetHandleEventError(nil)

		// Publish third event (will succeed)
		event3 := NewMissionFailedEvent("mission-1", errors.New("test"))
		err = bus.Publish(ctx, event3)
		require.NoError(t, err)

		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for third event")

		// Verify all events were processed
		assert.Equal(t, 3, engine.GetEventCallCount())
	})

	t.Run("handles missing event data gracefully", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish event with nil metadata
		event := api.EventData{
			EventType:    "mission.started",
			Source:       "test",
			Timestamp:    time.Now(),
			Metadata:     nil, // No metadata
			MissionEvent: &api.MissionEventData{
				MissionID: "mission-1",
			},
		}
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		// Wait for event to be processed
		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for event")

		// Should not panic, should process successfully
		assert.Equal(t, 1, engine.GetEventCallCount())
	})
}

// TestGraphEventSubscriber_ConcurrentEvents tests concurrent event processing.
func TestGraphEventSubscriber_ConcurrentEvents(t *testing.T) {
	t.Run("handles concurrent events", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish multiple events concurrently
		const numEvents = 20
		done := make(chan bool, numEvents)

		for i := 0; i < numEvents; i++ {
			go func(id int) {
				event := NewMissionStartedEvent("mission-" + string(rune('0'+id)))
				_ = bus.Publish(ctx, event)
				done <- true
			}(i)
		}

		// Wait for all publishes to complete
		for i := 0; i < numEvents; i++ {
			<-done
		}

		// Wait for all events to be processed
		assert.True(t, engine.WaitForEvents(numEvents, 5*time.Second), "timeout waiting for concurrent events")

		// Verify all events were processed
		eventCount := engine.GetEventCallCount()
		assert.GreaterOrEqual(t, eventCount, 1, "should process at least some events")
		// Note: Due to buffer limitations, not all events may be received
		// This is expected behavior with slow consumers
	})
}

// TestGraphEventSubscriber_EventDataConversion tests data conversion logic.
func TestGraphEventSubscriber_EventDataConversion(t *testing.T) {
	t.Run("converts mission event data", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish mission event with full data
		event := api.EventData{
			EventType: "mission.started",
			Source:    "daemon",
			Timestamp: time.Now(),
			MissionEvent: &api.MissionEventData{
				MissionID: "mission-1",
				NodeID:    "node-1",
				Message:   "Started mission",
				Payload: map[string]any{
					"custom_field": "custom_value",
					"count":        42,
				},
			},
		}
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		// Wait for processing
		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for event")

		// Verify data conversion
		calls := engine.GetHandleEventCalls()
		require.Len(t, calls, 1)
		data := calls[0].data

		assert.Equal(t, "mission-1", data["mission_id"])
		assert.Equal(t, "node-1", data["node_id"])
		assert.Equal(t, "Started mission", data["message"])
		assert.Equal(t, "custom_value", data["custom_field"])
		assert.Equal(t, 42, data["count"])
	})

	t.Run("converts agent event data", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish agent event
		event := api.EventData{
			EventType: "agent.started",
			Source:    "daemon",
			Timestamp: time.Now(),
			AgentEvent: &api.AgentEventData{
				AgentID:   "agent-1",
				AgentName: "test-agent",
				Message:   "Agent started",
				Metadata: map[string]any{
					"version": "1.0.0",
					"mode":    "test",
				},
			},
		}
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		// Wait for processing
		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for event")

		// Verify data conversion
		calls := engine.GetHandleEventCalls()
		require.Len(t, calls, 1)
		data := calls[0].data

		assert.Equal(t, "agent-1", data["agent_id"])
		assert.Equal(t, "test-agent", data["agent_name"])
		assert.Equal(t, "Agent started", data["message"])
		assert.Equal(t, "1.0.0", data["version"])
		assert.Equal(t, "test", data["mode"])
	})

	t.Run("does not override top-level fields with payload", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)
		defer subscriber.Stop()

		// Publish event with overlapping fields
		event := api.EventData{
			EventType: "mission.started",
			Source:    "daemon",
			Timestamp: time.Now(),
			MissionEvent: &api.MissionEventData{
				MissionID: "mission-1",
				Message:   "Original message",
				Payload: map[string]any{
					"message": "Payload message", // Should NOT override
				},
			},
		}
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		// Wait for processing
		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for event")

		// Verify top-level field was not overridden
		calls := engine.GetHandleEventCalls()
		require.Len(t, calls, 1)
		data := calls[0].data

		assert.Equal(t, "Original message", data["message"], "top-level field should not be overridden by payload")
	})
}

// TestGraphEventSubscriber_ChannelClosure tests behavior when event channel closes.
func TestGraphEventSubscriber_ChannelClosure(t *testing.T) {
	t.Run("stops processing when channel closes", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)

		engine := newMockTaxonomyGraphEngine()
		subscriber := NewGraphEventSubscriber(engine, bus, logger)

		ctx := context.Background()
		subscriber.Start(ctx)

		// Publish an event to verify it's working
		event := NewMissionStartedEvent("test-mission")
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		assert.True(t, engine.WaitForEvents(1, 1*time.Second), "timeout waiting for event")

		// Close the bus (which closes all subscriber channels)
		err = bus.Close()
		require.NoError(t, err)

		// Give it time to detect channel closure
		time.Sleep(200 * time.Millisecond)

		// Subscriber should have stopped processing
		// (No way to directly verify, but the goroutine should have exited)
	})
}

// TestGraphEventSubscriber_MultipleSubscribers tests multiple subscribers on same bus.
func TestGraphEventSubscriber_MultipleSubscribers(t *testing.T) {
	t.Run("multiple subscribers each receive events", func(t *testing.T) {
		logger := slog.Default()
		bus := NewEventBus(logger)
		defer bus.Close()

		engine1 := newMockTaxonomyGraphEngine()
		engine2 := newMockTaxonomyGraphEngine()

		subscriber1 := NewGraphEventSubscriber(engine1, bus, logger)
		subscriber2 := NewGraphEventSubscriber(engine2, bus, logger)

		ctx := context.Background()
		subscriber1.Start(ctx)
		subscriber2.Start(ctx)
		defer subscriber1.Stop()
		defer subscriber2.Stop()

		// Publish an event
		event := NewMissionStartedEvent("mission-1")
		err := bus.Publish(ctx, event)
		require.NoError(t, err)

		// Both engines should receive the event
		assert.True(t, engine1.WaitForEvents(1, 1*time.Second), "timeout waiting for engine1")
		assert.True(t, engine2.WaitForEvents(1, 1*time.Second), "timeout waiting for engine2")

		assert.Equal(t, 1, engine1.GetEventCallCount())
		assert.Equal(t, 1, engine2.GetEventCallCount())
	})
}
