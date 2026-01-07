package planning

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestPlanningEventType_String(t *testing.T) {
	tests := []struct {
		name      string
		eventType PlanningEventType
		expected  string
	}{
		{
			name:      "plan generated",
			eventType: EventPlanGenerated,
			expected:  "planning.plan_generated",
		},
		{
			name:      "plan validation failed",
			eventType: EventPlanValidationFailed,
			expected:  "planning.plan_validation_failed",
		},
		{
			name:      "step scored",
			eventType: EventStepScored,
			expected:  "planning.step_scored",
		},
		{
			name:      "replan triggered",
			eventType: EventReplanTriggered,
			expected:  "planning.replan_triggered",
		},
		{
			name:      "replan completed",
			eventType: EventReplanCompleted,
			expected:  "planning.replan_completed",
		},
		{
			name:      "replan rejected",
			eventType: EventReplanRejected,
			expected:  "planning.replan_rejected",
		},
		{
			name:      "budget exhausted",
			eventType: EventBudgetExhausted,
			expected:  "planning.budget_exhausted",
		},
		{
			name:      "constraint violation",
			eventType: EventConstraintViolation,
			expected:  "planning.constraint_violation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.eventType.String())
		})
	}
}

func TestNewDefaultPlanningEventEmitter(t *testing.T) {
	t.Run("default buffer size", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()
		assert.NotNil(t, emitter)
		assert.Equal(t, 100, emitter.bufferSize)
		assert.NotNil(t, emitter.subscribers)
		assert.False(t, emitter.closed)
		assert.Equal(t, 0, emitter.SubscriberCount())
	})

	t.Run("custom buffer size", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter(WithPlanningBufferSize(200))
		assert.NotNil(t, emitter)
		assert.Equal(t, 200, emitter.bufferSize)
	})
}

func TestDefaultPlanningEventEmitter_Subscribe(t *testing.T) {
	ctx := context.Background()

	t.Run("subscribe creates channel", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()
		defer emitter.Close()

		ch, cleanup := emitter.Subscribe(ctx)
		defer cleanup()

		assert.NotNil(t, ch)
		assert.Equal(t, 1, emitter.SubscriberCount())
	})

	t.Run("multiple subscribers", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()
		defer emitter.Close()

		ch1, cleanup1 := emitter.Subscribe(ctx)
		defer cleanup1()

		ch2, cleanup2 := emitter.Subscribe(ctx)
		defer cleanup2()

		assert.NotNil(t, ch1)
		assert.NotNil(t, ch2)
		assert.Equal(t, 2, emitter.SubscriberCount())
	})

	t.Run("cleanup removes subscriber", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()
		defer emitter.Close()

		initialCount := emitter.SubscriberCount()

		ch, cleanup := emitter.Subscribe(ctx)
		assert.NotNil(t, ch)
		assert.Equal(t, initialCount+1, emitter.SubscriberCount())

		cleanup()
		assert.Equal(t, initialCount, emitter.SubscriberCount())
	})

	t.Run("cleanup closes channel", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()
		defer emitter.Close()

		ch, cleanup := emitter.Subscribe(ctx)

		cleanup()

		// Channel should be closed
		_, ok := <-ch
		assert.False(t, ok, "channel should be closed after cleanup")
	})

	t.Run("double cleanup is safe", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()
		defer emitter.Close()

		_, cleanup := emitter.Subscribe(ctx)

		assert.NotPanics(t, func() {
			cleanup()
			cleanup() // Should not panic
		})
	})
}

func TestDefaultPlanningEventEmitter_Emit(t *testing.T) {
	ctx := context.Background()
	missionID := types.NewID()

	t.Run("emit to single subscriber", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()
		defer emitter.Close()

		ch, cleanup := emitter.Subscribe(ctx)
		defer cleanup()

		event := NewPlanGeneratedEvent(missionID, map[string]any{"test": "data"})
		err := emitter.Emit(ctx, event)

		require.NoError(t, err)

		// Receive event
		select {
		case received := <-ch:
			assert.Equal(t, EventPlanGenerated, received.Type)
			assert.Equal(t, missionID, received.MissionID)
			assert.Equal(t, "data", received.Payload["test"])
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for event")
		}
	})

	t.Run("emit to multiple subscribers", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()
		defer emitter.Close()

		ch1, cleanup1 := emitter.Subscribe(ctx)
		defer cleanup1()

		ch2, cleanup2 := emitter.Subscribe(ctx)
		defer cleanup2()

		event := NewStepScoredEvent(missionID, "node1", true, 0.95, false, "deterministic")
		err := emitter.Emit(ctx, event)

		require.NoError(t, err)

		// Both subscribers should receive the event
		var wg sync.WaitGroup
		wg.Add(2)

		checkEvent := func(ch <-chan PlanningEvent, name string) {
			defer wg.Done()
			select {
			case received := <-ch:
				assert.Equal(t, EventStepScored, received.Type)
				assert.Equal(t, missionID, received.MissionID)
			case <-time.After(time.Second):
				t.Errorf("%s: timeout waiting for event", name)
			}
		}

		go checkEvent(ch1, "subscriber1")
		go checkEvent(ch2, "subscriber2")

		wg.Wait()
	})

	t.Run("emit is non-blocking with slow consumer", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter(WithPlanningBufferSize(1))
		defer emitter.Close()

		ch, cleanup := emitter.Subscribe(ctx)
		defer cleanup()

		// Fill the buffer
		event1 := NewPlanGeneratedEvent(missionID, map[string]any{"num": 1})
		err := emitter.Emit(ctx, event1)
		require.NoError(t, err)

		// This should still complete without blocking (event will be dropped)
		start := time.Now()
		event2 := NewPlanGeneratedEvent(missionID, map[string]any{"num": 2})
		err = emitter.Emit(ctx, event2)
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.Less(t, elapsed, 100*time.Millisecond, "emit should not block")

		// First event should be received
		select {
		case received := <-ch:
			assert.Equal(t, 1, received.Payload["num"])
		case <-time.After(time.Second):
			t.Fatal("timeout receiving first event")
		}

		// Second event was dropped, channel should be empty
		select {
		case <-ch:
			t.Fatal("should not receive dropped event")
		case <-time.After(100 * time.Millisecond):
			// Expected - no event
		}
	})

	t.Run("emit fails when emitter closed", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()
		emitter.Close()

		event := NewPlanGeneratedEvent(missionID, nil)
		err := emitter.Emit(ctx, event)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "closed")
	})

	t.Run("emit respects context cancellation", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter(WithPlanningBufferSize(0))
		defer emitter.Close()

		// Subscribe but never read from channel
		_, cleanup := emitter.Subscribe(ctx)
		defer cleanup()

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		event := NewPlanGeneratedEvent(missionID, nil)
		err := emitter.Emit(cancelCtx, event)

		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestDefaultPlanningEventEmitter_Close(t *testing.T) {
	ctx := context.Background()

	t.Run("close shuts down emitter", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()

		ch, cleanup := emitter.Subscribe(ctx)
		defer cleanup()

		err := emitter.Close()
		require.NoError(t, err)

		// Channel should be closed
		_, ok := <-ch
		assert.False(t, ok, "channel should be closed")

		// Subscriber count should be 0
		assert.Equal(t, 0, emitter.SubscriberCount())
	})

	t.Run("close is idempotent", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()

		err1 := emitter.Close()
		err2 := emitter.Close()

		assert.NoError(t, err1)
		assert.NoError(t, err2)
	})

	t.Run("close closes all subscriber channels", func(t *testing.T) {
		emitter := NewDefaultPlanningEventEmitter()

		ch1, cleanup1 := emitter.Subscribe(ctx)
		defer cleanup1()

		ch2, cleanup2 := emitter.Subscribe(ctx)
		defer cleanup2()

		emitter.Close()

		// Both channels should be closed
		_, ok1 := <-ch1
		_, ok2 := <-ch2
		assert.False(t, ok1, "ch1 should be closed")
		assert.False(t, ok2, "ch2 should be closed")
	})
}

func TestDefaultPlanningEventEmitter_Concurrency(t *testing.T) {
	ctx := context.Background()
	emitter := NewDefaultPlanningEventEmitter()
	defer emitter.Close()

	missionID := types.NewID()

	// Test concurrent subscribe/unsubscribe
	t.Run("concurrent subscribe and unsubscribe", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ch, cleanup := emitter.Subscribe(ctx)
				time.Sleep(10 * time.Millisecond)
				cleanup()
				// Ensure channel is closed
				_, _ = <-ch
			}()
		}

		wg.Wait()
		assert.Equal(t, 0, emitter.SubscriberCount())
	})

	// Test concurrent emit
	t.Run("concurrent emit", func(t *testing.T) {
		ch, cleanup := emitter.Subscribe(ctx)
		defer cleanup()

		var wg sync.WaitGroup
		numEmitters := 10

		for i := 0; i < numEmitters; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				event := NewPlanGeneratedEvent(missionID, map[string]any{"num": n})
				err := emitter.Emit(ctx, event)
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()

		// Drain events (may be fewer than emitted due to buffer overflow)
		receivedCount := 0
		timeout := time.After(time.Second)
	drainLoop:
		for {
			select {
			case <-ch:
				receivedCount++
			case <-timeout:
				break drainLoop
			}
		}

		// Should receive at least some events
		assert.Greater(t, receivedCount, 0)
	})
}

func TestNewPlanningEvent(t *testing.T) {
	missionID := types.NewID()
	payload := map[string]any{"key": "value"}

	event := NewPlanningEvent(EventPlanGenerated, missionID, payload)

	assert.Equal(t, EventPlanGenerated, event.Type)
	assert.Equal(t, missionID, event.MissionID)
	assert.Equal(t, payload, event.Payload)
	assert.WithinDuration(t, time.Now(), event.Timestamp, time.Second)
}

func TestPlanningEventHelpers(t *testing.T) {
	missionID := types.NewID()

	t.Run("NewPlanGeneratedEvent", func(t *testing.T) {
		details := map[string]any{"steps": 5}
		event := NewPlanGeneratedEvent(missionID, details)

		assert.Equal(t, EventPlanGenerated, event.Type)
		assert.Equal(t, missionID, event.MissionID)
		assert.Equal(t, details, event.Payload)
	})

	t.Run("NewPlanValidationFailedEvent", func(t *testing.T) {
		reason := "invalid node"
		violations := []string{"node1", "node2"}
		event := NewPlanValidationFailedEvent(missionID, reason, violations)

		assert.Equal(t, EventPlanValidationFailed, event.Type)
		assert.Equal(t, missionID, event.MissionID)
		assert.Equal(t, reason, event.Payload["reason"])
		assert.Equal(t, violations, event.Payload["violations"])
	})

	t.Run("NewStepScoredEvent", func(t *testing.T) {
		event := NewStepScoredEvent(missionID, "node1", true, 0.95, false, "deterministic")

		assert.Equal(t, EventStepScored, event.Type)
		assert.Equal(t, missionID, event.MissionID)
		assert.Equal(t, "node1", event.Payload["node_id"])
		assert.Equal(t, true, event.Payload["success"])
		assert.Equal(t, 0.95, event.Payload["confidence"])
		assert.Equal(t, false, event.Payload["should_replan"])
		assert.Equal(t, "deterministic", event.Payload["method"])
	})

	t.Run("NewReplanTriggeredEvent", func(t *testing.T) {
		event := NewReplanTriggeredEvent(missionID, "node1", "low confidence", 1)

		assert.Equal(t, EventReplanTriggered, event.Type)
		assert.Equal(t, missionID, event.MissionID)
		assert.Equal(t, "node1", event.Payload["trigger_node_id"])
		assert.Equal(t, "low confidence", event.Payload["reason"])
		assert.Equal(t, 1, event.Payload["replan_count"])
	})

	t.Run("NewReplanCompletedEvent", func(t *testing.T) {
		event := NewReplanCompletedEvent(missionID, "reorder", "trying alternative path", 2)

		assert.Equal(t, EventReplanCompleted, event.Type)
		assert.Equal(t, missionID, event.MissionID)
		assert.Equal(t, "reorder", event.Payload["action"])
		assert.Equal(t, "trying alternative path", event.Payload["rationale"])
		assert.Equal(t, 2, event.Payload["replan_count"])
	})

	t.Run("NewReplanRejectedEvent", func(t *testing.T) {
		event := NewReplanRejectedEvent(missionID, "replan limit exceeded", 3)

		assert.Equal(t, EventReplanRejected, event.Type)
		assert.Equal(t, missionID, event.MissionID)
		assert.Equal(t, "replan limit exceeded", event.Payload["reason"])
		assert.Equal(t, 3, event.Payload["replan_count"])
	})

	t.Run("NewBudgetExhaustedEvent", func(t *testing.T) {
		event := NewBudgetExhaustedEvent(missionID, "strategic_planning", 1000, 1050)

		assert.Equal(t, EventBudgetExhausted, event.Type)
		assert.Equal(t, missionID, event.MissionID)
		assert.Equal(t, "strategic_planning", event.Payload["phase"])
		assert.Equal(t, 1000, event.Payload["allocated"])
		assert.Equal(t, 1050, event.Payload["consumed"])
	})

	t.Run("NewConstraintViolationEvent", func(t *testing.T) {
		details := map[string]any{"node": "node1"}
		event := NewConstraintViolationEvent(missionID, "bounds", "node not in workflow", details)

		assert.Equal(t, EventConstraintViolation, event.Type)
		assert.Equal(t, missionID, event.MissionID)
		assert.Equal(t, "bounds", event.Payload["constraint_type"])
		assert.Equal(t, "node not in workflow", event.Payload["violation"])
		assert.Equal(t, "node1", event.Payload["node"])
	})

	t.Run("NewConstraintViolationEvent with nil details", func(t *testing.T) {
		event := NewConstraintViolationEvent(missionID, "bounds", "violation", nil)

		assert.Equal(t, EventConstraintViolation, event.Type)
		assert.Equal(t, missionID, event.MissionID)
		assert.Equal(t, "bounds", event.Payload["constraint_type"])
		assert.Equal(t, "violation", event.Payload["violation"])
	})
}

// Benchmark tests
func BenchmarkEmit_SingleSubscriber(b *testing.B) {
	ctx := context.Background()
	emitter := NewDefaultPlanningEventEmitter()
	defer emitter.Close()

	ch, cleanup := emitter.Subscribe(ctx)
	defer cleanup()

	// Drain events in background
	go func() {
		for range ch {
		}
	}()

	missionID := types.NewID()
	event := NewPlanGeneratedEvent(missionID, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = emitter.Emit(ctx, event)
	}
}

func BenchmarkEmit_MultipleSubscribers(b *testing.B) {
	ctx := context.Background()
	emitter := NewDefaultPlanningEventEmitter()
	defer emitter.Close()

	// Create 10 subscribers
	cleanups := make([]func(), 10)
	for i := 0; i < 10; i++ {
		ch, cleanup := emitter.Subscribe(ctx)
		cleanups[i] = cleanup

		// Drain events in background
		go func(c <-chan PlanningEvent) {
			for range c {
			}
		}(ch)
	}

	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()

	missionID := types.NewID()
	event := NewPlanGeneratedEvent(missionID, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = emitter.Emit(ctx, event)
	}
}

func BenchmarkSubscribe(b *testing.B) {
	ctx := context.Background()
	emitter := NewDefaultPlanningEventEmitter()
	defer emitter.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, cleanup := emitter.Subscribe(ctx)
		cleanup()
	}
}
