package daemon

import (
	"context"
	"log/slog"

	"github.com/zero-day-ai/gibson/internal/daemon/api"
	"github.com/zero-day-ai/gibson/internal/graphrag/engine"
)

// GraphEventSubscriber bridges the EventBus to the TaxonomyGraphEngine.
// It subscribes to execution events and routes them to the graph engine for
// node/relationship creation based on taxonomy definitions.
//
// The subscriber operates with graceful degradation - graph engine failures
// are logged but do not propagate errors back to the event bus or mission execution.
type GraphEventSubscriber struct {
	engine   engine.TaxonomyGraphEngine
	eventBus *EventBus
	logger   *slog.Logger
	cleanup  func() // Cleanup function returned by EventBus.Subscribe
}

// NewGraphEventSubscriber creates a new graph event subscriber.
//
// Parameters:
//   - engine: The taxonomy graph engine that will process events
//   - eventBus: The event bus to subscribe to
//   - logger: Structured logger for subscriber operations
//
// Returns:
//   - *GraphEventSubscriber: A new subscriber ready to be started
func NewGraphEventSubscriber(
	engine engine.TaxonomyGraphEngine,
	eventBus *EventBus,
	logger *slog.Logger,
) *GraphEventSubscriber {
	if logger == nil {
		logger = slog.Default()
	}

	return &GraphEventSubscriber{
		engine:   engine,
		eventBus: eventBus,
		logger:   logger.With("component", "graph_event_subscriber"),
	}
}

// Start begins subscribing to execution events and routing them to the graph engine.
// This method spawns a goroutine that listens for events until Stop() is called or
// the context is cancelled.
//
// Parameters:
//   - ctx: Context for the subscriber lifetime
//
// The method is non-blocking and returns immediately after starting the event processing goroutine.
func (s *GraphEventSubscriber) Start(ctx context.Context) {
	// Subscribe to all execution event types defined in execution_events.yaml
	eventTypes := []string{
		// Mission events
		EventTypeMissionStarted,
		EventTypeMissionCompleted,
		EventTypeMissionFailed,

		// Agent events
		EventTypeAgentStarted,
		EventTypeAgentCompleted,
		EventTypeAgentFailed,
		EventTypeAgentDelegated,

		// LLM events
		EventTypeLLMRequestStarted,
		EventTypeLLMRequestCompleted,
		EventTypeLLMRequestFailed,
		EventTypeLLMStreamStarted,
		EventTypeLLMStreamCompleted,

		// Tool events
		EventTypeToolCallStarted,
		EventTypeToolCallCompleted,
		EventTypeToolCallFailed,

		// Plugin events
		EventTypePluginQueryStarted,
		EventTypePluginQueryCompleted,
		EventTypePluginQueryFailed,

		// Finding events
		EventTypeFindingDiscovered,
		EventTypeAgentFindingSubmitted,
	}

	// Subscribe to event bus (no mission ID filter - process all missions)
	eventChan, cleanup := s.eventBus.Subscribe(ctx, eventTypes, "")
	s.cleanup = cleanup

	s.logger.Info("graph event subscriber started",
		"event_types", len(eventTypes),
	)

	// Start event processing goroutine
	go s.processEvents(ctx, eventChan)
}

// Stop gracefully shuts down the subscriber and unsubscribes from the event bus.
func (s *GraphEventSubscriber) Stop() {
	s.logger.Info("stopping graph event subscriber")
	if s.cleanup != nil {
		s.cleanup()
	}
}

// processEvents is the main event processing loop.
// It receives events from the event bus channel and routes them to the graph engine.
func (s *GraphEventSubscriber) processEvents(ctx context.Context, eventChan <-chan api.EventData) {
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("graph event subscriber context cancelled, stopping")
			return

		case event, ok := <-eventChan:
			if !ok {
				s.logger.Info("event channel closed, stopping subscriber")
				return
			}

			// Process the event
			s.handleEvent(ctx, event)
		}
	}
}

// handleEvent converts an EventBus event to a GraphEvent and sends it to the taxonomy engine.
// Errors are logged but not propagated to ensure graceful degradation.
func (s *GraphEventSubscriber) handleEvent(ctx context.Context, event api.EventData) {
	logger := s.logger.With(
		"event_type", event.EventType,
		"timestamp", event.Timestamp,
	)

	logger.Debug("processing event for graph")

	// Convert EventBus event to graph event data
	graphData := s.convertEventToGraphData(event)

	// Send to taxonomy engine for processing
	if err := s.engine.HandleEvent(ctx, event.EventType, graphData); err != nil {
		// Log error but don't propagate - graceful degradation
		logger.Error("failed to process event in graph engine",
			"error", err,
		)
		return
	}

	logger.Debug("successfully processed event in graph engine")
}

// convertEventToGraphData converts an api.EventData to the map format expected by the taxonomy engine.
// It extracts relevant fields from the event and its nested event data structures.
func (s *GraphEventSubscriber) convertEventToGraphData(event api.EventData) map[string]any {
	data := make(map[string]any)

	// Add common fields
	data["timestamp"] = event.Timestamp.Unix()
	data["source"] = event.Source

	// Extract trace context if available in event metadata
	if event.Metadata != nil {
		if traceID, ok := event.Metadata["trace_id"].(string); ok {
			data["trace_id"] = traceID
		}
		if spanID, ok := event.Metadata["span_id"].(string); ok {
			data["span_id"] = spanID
		}
		if parentSpanID, ok := event.Metadata["parent_span_id"].(string); ok {
			data["parent_span_id"] = parentSpanID
		}
	}

	// Extract fields from event-specific data structures
	if event.MissionEvent != nil {
		s.extractMissionEventData(event.MissionEvent, data)
	}
	if event.AgentEvent != nil {
		s.extractAgentEventData(event.AgentEvent, data)
	}
	if event.FindingEvent != nil {
		s.extractFindingEventData(event.FindingEvent, data)
	}

	return data
}

// extractMissionEventData extracts fields from MissionEventData into the graph data map.
func (s *GraphEventSubscriber) extractMissionEventData(missionEvent *api.MissionEventData, data map[string]any) {
	data["mission_id"] = missionEvent.MissionID

	if missionEvent.NodeID != "" {
		data["node_id"] = missionEvent.NodeID
	}
	if missionEvent.Error != "" {
		data["error"] = missionEvent.Error
	}
	if missionEvent.Message != "" {
		data["message"] = missionEvent.Message
	}

	// Extract additional fields from Payload if present
	if missionEvent.Payload != nil {
		for key, value := range missionEvent.Payload {
			// Only add if not already present (don't override top-level fields)
			if _, exists := data[key]; !exists {
				data[key] = value
			}
		}
	}
}

// extractAgentEventData extracts fields from AgentEventData into the graph data map.
func (s *GraphEventSubscriber) extractAgentEventData(agentEvent *api.AgentEventData, data map[string]any) {
	if agentEvent.AgentID != "" {
		data["agent_id"] = agentEvent.AgentID
	}
	if agentEvent.AgentName != "" {
		data["agent_name"] = agentEvent.AgentName
	}
	if agentEvent.Message != "" {
		data["message"] = agentEvent.Message
	}

	// Extract additional fields from Metadata if present
	if agentEvent.Metadata != nil {
		for key, value := range agentEvent.Metadata {
			if _, exists := data[key]; !exists {
				data[key] = value
			}
		}
	}
}

// extractFindingEventData extracts fields from FindingEventData into the graph data map.
func (s *GraphEventSubscriber) extractFindingEventData(findingEvent *api.FindingEventData, data map[string]any) {
	data["mission_id"] = findingEvent.MissionID

	// Extract finding details
	if findingEvent.Finding.ID != "" {
		data["finding_id"] = findingEvent.Finding.ID
	}
	if findingEvent.Finding.Title != "" {
		data["finding_title"] = findingEvent.Finding.Title
	}
	if findingEvent.Finding.Severity != "" {
		data["severity"] = findingEvent.Finding.Severity
	}
	if findingEvent.Finding.Category != "" {
		data["category"] = findingEvent.Finding.Category
	}
}
