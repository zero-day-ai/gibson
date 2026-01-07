package planning

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// PlanningEventType identifies the type of planning event.
type PlanningEventType string

const (
	// EventPlanGenerated indicates a strategic plan was successfully generated.
	EventPlanGenerated PlanningEventType = "planning.plan_generated"

	// EventPlanValidationFailed indicates a generated plan failed validation against bounds.
	EventPlanValidationFailed PlanningEventType = "planning.plan_validation_failed"

	// EventStepScored indicates a workflow step was evaluated.
	EventStepScored PlanningEventType = "planning.step_scored"

	// EventReplanTriggered indicates replanning was triggered due to step results.
	EventReplanTriggered PlanningEventType = "planning.replan_triggered"

	// EventReplanCompleted indicates replanning completed successfully.
	EventReplanCompleted PlanningEventType = "planning.replan_completed"

	// EventReplanRejected indicates a replan was rejected (e.g., limit exceeded, invalid).
	EventReplanRejected PlanningEventType = "planning.replan_rejected"

	// EventBudgetExhausted indicates a planning budget phase was exhausted.
	EventBudgetExhausted PlanningEventType = "planning.budget_exhausted"

	// EventConstraintViolation indicates a planning constraint was violated.
	EventConstraintViolation PlanningEventType = "planning.constraint_violation"
)

// String returns the string representation of the event type.
func (t PlanningEventType) String() string {
	return string(t)
}

// PlanningEvent represents a planning system event.
// Events are emitted throughout the planning lifecycle to enable real-time monitoring
// and debugging of planning decisions.
type PlanningEvent struct {
	// Type identifies the event type.
	Type PlanningEventType `json:"type"`

	// MissionID is the unique identifier of the mission.
	MissionID types.ID `json:"mission_id"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Payload contains type-specific event data.
	// The payload structure varies by event type.
	Payload map[string]any `json:"payload,omitempty"`
}

// PlanningEventEmitter publishes planning events to subscribers.
// Implementations must be thread-safe and support multiple concurrent subscribers.
type PlanningEventEmitter interface {
	// Emit publishes an event to all subscribers.
	// Emit must be non-blocking - it should not wait for subscribers to consume events.
	// Returns an error if the event cannot be emitted (e.g., emitter closed).
	Emit(ctx context.Context, event PlanningEvent) error

	// Subscribe creates a new subscription and returns a channel for receiving events
	// and a cleanup function to unsubscribe.
	// The cleanup function must be called to prevent resource leaks.
	Subscribe(ctx context.Context) (<-chan PlanningEvent, func())

	// Close shuts down the emitter and all subscriptions.
	Close() error
}

// DefaultPlanningEventEmitter implements PlanningEventEmitter using buffered channels.
// It supports multiple subscribers and handles slow consumers gracefully by dropping
// events for subscribers whose channels are full.
type DefaultPlanningEventEmitter struct {
	mu          sync.RWMutex
	subscribers map[string]chan PlanningEvent
	bufferSize  int
	closed      bool
}

// PlanningEventEmitterOption is a functional option for configuring DefaultPlanningEventEmitter.
type PlanningEventEmitterOption func(*DefaultPlanningEventEmitter)

// WithPlanningBufferSize sets the buffer size for subscriber channels.
// Default is 100. Larger buffers can handle bursty events better.
func WithPlanningBufferSize(size int) PlanningEventEmitterOption {
	return func(e *DefaultPlanningEventEmitter) {
		e.bufferSize = size
	}
}

// NewDefaultPlanningEventEmitter creates a new DefaultPlanningEventEmitter with optional configuration.
func NewDefaultPlanningEventEmitter(opts ...PlanningEventEmitterOption) *DefaultPlanningEventEmitter {
	emitter := &DefaultPlanningEventEmitter{
		subscribers: make(map[string]chan PlanningEvent),
		bufferSize:  100, // Default buffer size
		closed:      false,
	}

	for _, opt := range opts {
		opt(emitter)
	}

	return emitter
}

// Emit publishes an event to all subscribers.
// If a subscriber's channel is full, the event is dropped for that subscriber
// to prevent blocking other subscribers (slow consumer handling).
// This ensures Emit is always non-blocking.
func (e *DefaultPlanningEventEmitter) Emit(ctx context.Context, event PlanningEvent) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return fmt.Errorf("planning event emitter is closed")
	}

	// Send to all subscribers (non-blocking)
	for _, ch := range e.subscribers {
		select {
		case ch <- event:
			// Event sent successfully
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Channel is full, drop event for this slow subscriber
			// This prevents one slow subscriber from blocking others
		}
	}

	return nil
}

// Subscribe creates a new subscription and returns a channel for receiving events.
// The returned cleanup function must be called to unsubscribe and prevent leaks.
func (e *DefaultPlanningEventEmitter) Subscribe(ctx context.Context) (<-chan PlanningEvent, func()) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Generate unique subscriber ID
	subscriberID := types.NewID().String()
	ch := make(chan PlanningEvent, e.bufferSize)
	e.subscribers[subscriberID] = ch

	// Cleanup function to unsubscribe
	cleanup := func() {
		e.mu.Lock()
		defer e.mu.Unlock()

		if subCh, exists := e.subscribers[subscriberID]; exists {
			delete(e.subscribers, subscriberID)
			close(subCh)
		}
	}

	return ch, cleanup
}

// Close shuts down the emitter and closes all subscriber channels.
func (e *DefaultPlanningEventEmitter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}

	e.closed = true

	// Close all subscriber channels
	for id, ch := range e.subscribers {
		close(ch)
		delete(e.subscribers, id)
	}

	return nil
}

// SubscriberCount returns the current number of active subscribers.
// Useful for monitoring and testing.
func (e *DefaultPlanningEventEmitter) SubscriberCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.subscribers)
}

// NewPlanningEvent creates a new planning event with the current timestamp.
func NewPlanningEvent(eventType PlanningEventType, missionID types.ID, payload map[string]any) PlanningEvent {
	return PlanningEvent{
		Type:      eventType,
		MissionID: missionID,
		Timestamp: time.Now(),
		Payload:   payload,
	}
}

// Helper functions for creating specific event types

// NewPlanGeneratedEvent creates a plan generated event.
func NewPlanGeneratedEvent(missionID types.ID, planDetails map[string]any) PlanningEvent {
	return NewPlanningEvent(EventPlanGenerated, missionID, planDetails)
}

// NewPlanValidationFailedEvent creates a plan validation failed event.
func NewPlanValidationFailedEvent(missionID types.ID, reason string, violations []string) PlanningEvent {
	payload := map[string]any{
		"reason":     reason,
		"violations": violations,
	}
	return NewPlanningEvent(EventPlanValidationFailed, missionID, payload)
}

// NewStepScoredEvent creates a step scored event.
func NewStepScoredEvent(missionID types.ID, nodeID string, success bool, confidence float64, shouldReplan bool, method string) PlanningEvent {
	payload := map[string]any{
		"node_id":       nodeID,
		"success":       success,
		"confidence":    confidence,
		"should_replan": shouldReplan,
		"method":        method,
	}
	return NewPlanningEvent(EventStepScored, missionID, payload)
}

// NewReplanTriggeredEvent creates a replan triggered event.
func NewReplanTriggeredEvent(missionID types.ID, triggerNodeID string, reason string, replanCount int) PlanningEvent {
	payload := map[string]any{
		"trigger_node_id": triggerNodeID,
		"reason":          reason,
		"replan_count":    replanCount,
	}
	return NewPlanningEvent(EventReplanTriggered, missionID, payload)
}

// NewReplanCompletedEvent creates a replan completed event.
func NewReplanCompletedEvent(missionID types.ID, action string, rationale string, replanCount int) PlanningEvent {
	payload := map[string]any{
		"action":       action,
		"rationale":    rationale,
		"replan_count": replanCount,
	}
	return NewPlanningEvent(EventReplanCompleted, missionID, payload)
}

// NewReplanRejectedEvent creates a replan rejected event.
func NewReplanRejectedEvent(missionID types.ID, reason string, replanCount int) PlanningEvent {
	payload := map[string]any{
		"reason":       reason,
		"replan_count": replanCount,
	}
	return NewPlanningEvent(EventReplanRejected, missionID, payload)
}

// NewBudgetExhaustedEvent creates a budget exhausted event.
func NewBudgetExhaustedEvent(missionID types.ID, phase string, allocated int, consumed int) PlanningEvent {
	payload := map[string]any{
		"phase":     phase,
		"allocated": allocated,
		"consumed":  consumed,
	}
	return NewPlanningEvent(EventBudgetExhausted, missionID, payload)
}

// NewConstraintViolationEvent creates a constraint violation event.
func NewConstraintViolationEvent(missionID types.ID, constraintType string, violation string, details map[string]any) PlanningEvent {
	payload := map[string]any{
		"constraint_type": constraintType,
		"violation":       violation,
	}
	if details != nil {
		for k, v := range details {
			payload[k] = v
		}
	}
	return NewPlanningEvent(EventConstraintViolation, missionID, payload)
}

// Ensure DefaultPlanningEventEmitter implements PlanningEventEmitter at compile time
var _ PlanningEventEmitter = (*DefaultPlanningEventEmitter)(nil)
