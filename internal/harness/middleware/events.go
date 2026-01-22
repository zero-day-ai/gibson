package middleware

import (
	"context"
	"time"

	"github.com/zero-day-ai/gibson/internal/events"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/trace"
)

// EventMiddleware creates middleware that emits events to the unified EventBus
// after operations complete. Events are published with proper context fields
// including MissionID, AgentName, TraceID, and SpanID.
//
// The middleware handles nil bus gracefully (no-op) and uses the provided
// ErrorHandler for emission failures rather than silently swallowing errors.
//
// Parameters:
//   - bus: The EventBus to publish events to (nil = no-op middleware)
//   - handler: ErrorHandler for emission failures (must not be nil)
//
// Returns:
//   - Middleware function that emits events after operation execution
//
// Example:
//
//	middleware := EventMiddleware(eventBus, errorHandler)
//	operation := middleware(baseOperation)
func EventMiddleware(bus events.EventBus, handler events.ErrorHandler) Middleware {
	// Handle nil bus gracefully - return no-op middleware
	if bus == nil {
		return func(next Operation) Operation {
			return next
		}
	}

	// Require non-nil error handler
	if handler == nil {
		panic("EventMiddleware requires non-nil ErrorHandler")
	}

	return func(next Operation) Operation {
		return func(ctx context.Context, req any) (any, error) {
			// Execute operation
			result, err := next(ctx, req)

			// Build and emit event after operation completes
			event := buildEvent(ctx, req, result, err)
			if event != nil {
				if pubErr := bus.Publish(ctx, *event); pubErr != nil {
					handler(pubErr, map[string]interface{}{
						"operation":  "event_publish",
						"event_type": event.Type,
						"mission_id": event.MissionID,
						"agent_name": event.AgentName,
					})
				}
			}

			return result, err
		}
	}
}

// buildEvent constructs an Event from the operation context, request, response, and error.
// Returns nil if the operation type cannot be determined or is not supported for event emission.
func buildEvent(ctx context.Context, req any, result any, err error) *events.Event {
	// Get operation type from context
	opType, ok := ctx.Value(CtxOperationType).(OperationType)
	if !ok {
		// Cannot determine operation type, skip event emission
		return nil
	}

	// Map operation type to event type
	eventType, payload := mapOperationToEvent(opType, req, result, err)
	if eventType == "" {
		// Operation type doesn't produce events
		return nil
	}

	// Build base event with timestamp
	event := &events.Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Payload:   payload,
	}

	// Add mission ID and agent name from context if available
	if missionID, ok := ctx.Value(CtxMissionID).(types.ID); ok {
		event.MissionID = missionID
	}
	if agentName, ok := ctx.Value(CtxAgentName).(string); ok {
		event.AgentName = agentName
	}

	// Add trace context if available
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		event.TraceID = span.SpanContext().TraceID().String()
		event.SpanID = span.SpanContext().SpanID().String()
	}

	return event
}

// mapOperationToEvent maps an operation type and its data to an EventType and payload.
// Returns empty string if the operation doesn't produce events.
func mapOperationToEvent(opType OperationType, req any, result any, err error) (events.EventType, any) {
	// Handle error cases - emit failed events
	if err != nil {
		return mapOperationToFailedEvent(opType, req, err)
	}

	// Handle success cases - emit completed events
	switch opType {
	case OpComplete, OpCompleteWithTools:
		return events.EventLLMRequestCompleted, buildLLMResponsePayload(result)

	case OpStream:
		return events.EventLLMStreamCompleted, buildLLMStreamCompletedPayload(result)

	case OpCallToolProto:
		return events.EventToolCallCompleted, buildToolResultPayload(req, result)

	case OpQueryPlugin:
		return events.EventPluginQueryCompleted, buildPluginResultPayload(req, result)

	case OpSubmitFinding:
		return events.EventFindingSubmitted, buildFindingPayload(req)

	case OpDelegateToAgent:
		return events.EventAgentDelegated, buildAgentDelegatedPayload(req)

	default:
		// Operation type doesn't produce completion events
		return "", nil
	}
}

// mapOperationToFailedEvent maps an operation type and error to a failed EventType and payload.
func mapOperationToFailedEvent(opType OperationType, req any, err error) (events.EventType, any) {
	switch opType {
	case OpComplete, OpCompleteWithTools:
		return events.EventLLMRequestFailed, buildLLMRequestFailedPayload(req, err)

	case OpStream:
		return events.EventLLMRequestFailed, buildLLMRequestFailedPayload(req, err)

	case OpCallToolProto:
		return events.EventToolCallFailed, buildToolCallFailedPayload(req, err)

	case OpQueryPlugin:
		return events.EventPluginQueryFailed, buildPluginQueryFailedPayload(req, err)

	default:
		// Operation type doesn't produce failure events
		return "", nil
	}
}

// Payload builders for each operation type

// buildLLMResponsePayload builds payload for successful LLM completion
func buildLLMResponsePayload(result any) any {
	if result == nil {
		return nil
	}

	resp, ok := result.(*llm.CompletionResponse)
	if !ok {
		return nil
	}

	return events.LLMRequestCompletedPayload{
		Model:          resp.Model,
		InputTokens:    resp.Usage.PromptTokens,
		OutputTokens:   resp.Usage.CompletionTokens,
		StopReason:     string(resp.FinishReason),
		ResponseLength: len(resp.Message.Content),
		// Duration is tracked by middleware, not available here
		// Provider/SlotName would come from request context if needed
	}
}

// buildLLMStreamCompletedPayload builds payload for completed stream
func buildLLMStreamCompletedPayload(result any) any {
	// Streaming operations emit their own completion events through the stream wrapper
	// This is a placeholder for future enhancement
	return events.LLMStreamCompletedPayload{
		TotalChunks: 0, // Would need to track through stream
	}
}

// buildToolResultPayload builds payload for successful tool execution
func buildToolResultPayload(req any, result any) any {
	// Extract tool name from request
	toolName := extractToolName(req)

	resultMap, ok := result.(map[string]any)
	resultSize := 0
	if ok {
		resultSize = len(resultMap)
	}

	return events.ToolCallCompletedPayload{
		ToolName:   toolName,
		ResultSize: resultSize,
		Success:    true,
		// Duration is tracked by middleware
	}
}

// buildPluginResultPayload builds payload for successful plugin query
func buildPluginResultPayload(req any, result any) any {
	pluginName, method := extractPluginInfo(req)

	return events.PluginQueryCompletedPayload{
		PluginName: pluginName,
		Method:     method,
		Success:    true,
		// Duration is tracked by middleware
	}
}

// buildFindingPayload builds payload for finding submission
func buildFindingPayload(req any) any {
	// Type assert to get finding details
	// This would need to match the actual finding type from the request
	return events.FindingSubmittedPayload{
		// Would extract from req
	}
}

// buildAgentDelegatedPayload builds payload for agent delegation
func buildAgentDelegatedPayload(req any) any {
	return events.AgentDelegatedPayload{
		// Would extract from req: FromAgent, ToAgent, TaskDescription
	}
}

// buildLLMRequestFailedPayload builds payload for failed LLM request
func buildLLMRequestFailedPayload(req any, err error) any {
	return events.LLMRequestFailedPayload{
		Error:     err.Error(),
		Retryable: false, // Would need to determine from error type
		// SlotName, Provider, Duration would come from context
	}
}

// buildToolCallFailedPayload builds payload for failed tool call
func buildToolCallFailedPayload(req any, err error) any {
	toolName := extractToolName(req)

	return events.ToolCallFailedPayload{
		ToolName: toolName,
		Error:    err.Error(),
		// Duration tracked by middleware
	}
}

// buildPluginQueryFailedPayload builds payload for failed plugin query
func buildPluginQueryFailedPayload(req any, err error) any {
	pluginName, method := extractPluginInfo(req)

	return events.PluginQueryFailedPayload{
		PluginName: pluginName,
		Method:     method,
		Error:      err.Error(),
		// Duration tracked by middleware
	}
}

// Helper functions to extract data from requests

// extractToolName extracts tool name from a tool call request
func extractToolName(req any) string {
	// Type assertion based on actual request structure
	// This is a placeholder - actual implementation depends on request type
	if m, ok := req.(map[string]any); ok {
		if name, ok := m["tool_name"].(string); ok {
			return name
		}
		if name, ok := m["name"].(string); ok {
			return name
		}
	}
	return ""
}

// extractPluginInfo extracts plugin name and method from a plugin query request
func extractPluginInfo(req any) (string, string) {
	// Type assertion based on actual request structure
	// This is a placeholder - actual implementation depends on request type
	if m, ok := req.(map[string]any); ok {
		pluginName, _ := m["plugin_name"].(string)
		method, _ := m["method"].(string)
		return pluginName, method
	}
	return "", ""
}
