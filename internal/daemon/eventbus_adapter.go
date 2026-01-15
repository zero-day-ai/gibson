package daemon

import (
	"context"
	"time"

	"github.com/zero-day-ai/gibson/internal/daemon/api"
)

// EventBusAdapter adapts the daemon's EventBus to the EventBusPublisher interface
// expected by the mission orchestrator. This avoids circular dependencies between
// daemon and mission packages.
type EventBusAdapter struct {
	eventBus *EventBus
}

// NewEventBusAdapter creates a new adapter that wraps an EventBus.
func NewEventBusAdapter(eventBus *EventBus) *EventBusAdapter {
	return &EventBusAdapter{
		eventBus: eventBus,
	}
}

// Publish converts an interface{} event to api.EventData and publishes to the event bus.
// This method implements the EventBusPublisher interface from mission package.
func (a *EventBusAdapter) Publish(ctx context.Context, event interface{}) error {
	// Convert interface{} to api.EventData
	// The mission orchestrator sends a specially crafted struct that we can type-assert
	eventData := convertToAPIEventData(event)

	// Publish to the underlying event bus
	return a.eventBus.Publish(ctx, eventData)
}

// convertToAPIEventData converts the event from mission orchestrator to api.EventData.
// The mission orchestrator creates an anonymous struct that matches this structure.
func convertToAPIEventData(event interface{}) api.EventData {
	// Type assert to the specific struct format sent by mission orchestrator
	type missionEventDataType struct {
		EventType string
		Timestamp time.Time
		MissionID string
		NodeID    string
		Message   string
		Data      string
		Error     string
		Result    interface{}
		Payload   map[string]interface{}
	}

	type eventDataType struct {
		EventType    string
		Timestamp    time.Time
		Source       string
		Data         string
		Metadata     map[string]interface{}
		MissionEvent *missionEventDataType
		AttackEvent  interface{}
		AgentEvent   interface{}
		FindingEvent interface{}
	}

	// Try to type assert to our expected structure
	if ed, ok := event.(eventDataType); ok {
		result := api.EventData{
			EventType: ed.EventType,
			Timestamp: ed.Timestamp,
			Source:    ed.Source,
			Data:      ed.Data,
			Metadata:  ed.Metadata,
		}

		// Convert mission event if present
		if ed.MissionEvent != nil {
			result.MissionEvent = &api.MissionEventData{
				EventType: ed.MissionEvent.EventType,
				Timestamp: ed.MissionEvent.Timestamp,
				MissionID: ed.MissionEvent.MissionID,
				NodeID:    ed.MissionEvent.NodeID,
				Message:   ed.MissionEvent.Message,
				Data:      ed.MissionEvent.Data,
				Error:     ed.MissionEvent.Error,
				Payload:   ed.MissionEvent.Payload,
			}
		}

		return result
	}

	// If type assertion fails, return a minimal event
	// This should not happen in normal operation
	return api.EventData{
		EventType: "unknown",
		Timestamp: time.Now(),
		Source:    "mission-orchestrator",
	}
}
