package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/harness/middleware"
	"github.com/zero-day-ai/gibson/internal/llm"
)

// EventSender is the interface required by LangfuseTracingMiddleware.
// This allows for testing with mocks while keeping the middleware testable.
type EventSender interface {
	SendEvent(ctx context.Context, event map[string]any) error
}

// LangfuseTracingMiddleware creates middleware that traces LLM and tool calls to Langfuse.
// It follows the existing middleware pattern and provides fire-and-forget tracing that never
// fails the underlying operation.
//
// The middleware handles two key operation types:
//   - OpComplete/OpCompleteWithTools: Traced as Langfuse "generation" events with prompt/completion/tokens
//   - OpCallTool: Traced as Langfuse "span" events with input/output
//
// Trace Hierarchy:
// When a tracer and parent log are provided, traces are nested under the parent agent execution:
//   - AgentExecutionLog (parent span from orchestrator)
//     ├── Generation: LLM completion call
//     └── Span: Tool execution
//
// Nil Tracer Behavior:
// If tracer or parentLog is nil, the middleware acts as a pass-through with zero overhead.
//
// Error Handling:
// All tracing errors are logged but never propagated. The middleware guarantees that
// tracing failures will not impact agent execution.
//
// Thread Safety:
// The middleware is safe for concurrent use. Each operation creates independent trace events.
//
// Parameters:
//   - tracer: EventSender instance for sending events to Langfuse (can be nil, typically *MissionTracer)
//   - parentLog: Parent agent execution log for nesting traces (can be nil)
//
// Returns:
//   - middleware.Middleware: Function that wraps operations with Langfuse tracing
//
// Example:
//
//	langfuseMW := observability.LangfuseTracingMiddleware(missionTracer, agentLog)
//	wrapped := langfuseMW(baseOperation)
func LangfuseTracingMiddleware(tracer EventSender, parentLog *AgentExecutionLog) middleware.Middleware {
	return func(next middleware.Operation) middleware.Operation {
		return func(ctx context.Context, req any) (any, error) {
			// Pass-through if tracer or parent log not available
			if tracer == nil || parentLog == nil {
				return next(ctx, req)
			}

			// Get operation type to determine tracing strategy
			opType := middleware.GetOperationType(ctx)

			// Record start time
			startTime := time.Now()

			// Execute the operation
			resp, err := next(ctx, req)

			// Fire-and-forget trace based on operation type
			// We trace after execution to capture complete information
			switch opType {
			case middleware.OpComplete, middleware.OpCompleteWithTools:
				go traceCompletion(ctx, tracer, parentLog, req, resp, err, startTime)

			case middleware.OpCallTool:
				go traceToolCall(ctx, tracer, parentLog, req, resp, err, startTime)
			}

			// Always return the original result/error
			return resp, err
		}
	}
}

// traceCompletion traces an LLM completion as a Langfuse generation event.
// This runs in a goroutine and logs any errors without propagating them.
func traceCompletion(
	ctx context.Context,
	tracer EventSender,
	parentLog *AgentExecutionLog,
	req any,
	resp any,
	execErr error,
	startTime time.Time,
) {
	// Extract LLM request details
	var messages []llm.Message
	var slot string
	var temperature float64
	var maxTokens int

	// Try to extract from CompletionRequest type
	if compReq, ok := req.(*middleware.CompletionRequest); ok {
		slot = compReq.Slot
		messages = compReq.Messages
		// Note: Options are not directly available in CompletionRequest
		// They would need to be added to the struct or extracted via context
	}

	// Also try to get messages from context (set by tracing middleware)
	if ctxMessages := middleware.GetMessages(ctx); ctxMessages != nil && len(messages) == 0 {
		messages = make([]llm.Message, len(ctxMessages))
		for i, msg := range ctxMessages {
			messages[i] = llm.Message{
				Role:    llm.Role(msg.Role),
				Content: msg.Content,
			}
		}
	}

	// If we still don't have slot, try context
	if slot == "" {
		slot = middleware.GetSlotName(ctx)
	}

	// Build full prompt string with complete message details
	prompt := buildFullPromptString(messages)

	// Extract response details
	var completion string
	var model string
	var promptTokens, completionTokens int
	var finishReason string

	if execErr == nil && resp != nil {
		// Try to extract from CompletionResponse
		if compResp, ok := resp.(*llm.CompletionResponse); ok {
			completion = compResp.Message.Content
			model = compResp.Model
			promptTokens = compResp.Usage.PromptTokens
			completionTokens = compResp.Usage.CompletionTokens
			finishReason = string(compResp.FinishReason)

			// Include tool calls in completion if present
			if len(compResp.Message.ToolCalls) > 0 {
				toolCallsJSON, err := json.Marshal(compResp.Message.ToolCalls)
				if err == nil {
					completion += "\n[Tool Calls]: " + string(toolCallsJSON)
				}
			}
		}
	}

	// Handle error case
	if execErr != nil {
		completion = fmt.Sprintf("[ERROR]: %v", execErr)
		finishReason = "error"
	}

	// Calculate end time
	endTime := time.Now()

	// Generate unique span ID for this generation
	spanID := fmt.Sprintf("agent-llm-%s-%d", parentLog.Execution.ID.String(), time.Now().UnixNano())

	// Get trace ID from parent (mission trace)
	// Note: We need the mission ID for the trace, which should be in parentLog
	traceID := fmt.Sprintf("mission-%s", parentLog.Execution.ID.String())

	// Create generation-create event
	event := map[string]any{
		"type":      "generation-create",
		"id":        generateEventID("gen", spanID),
		"timestamp": startTime.Format(time.RFC3339Nano),
		"body": map[string]any{
			"id":                  spanID,
			"traceId":             traceID,
			"parentObservationId": parentLog.SpanID, // Nest under agent execution span
			"name":                fmt.Sprintf("llm-call-%s", slot),
			"startTime":           startTime.Format(time.RFC3339Nano),
			"endTime":             endTime.Format(time.RFC3339Nano),
			"model":               model,
			"input":               prompt,
			"output":              completion,
			"promptTokens":        promptTokens,
			"completionTokens":    completionTokens,
			"level":               determineLevel(execErr),
			"metadata": map[string]any{
				"slot":          slot,
				"agent_name":    parentLog.AgentName,
				"finish_reason": finishReason,
				"duration_ms":   endTime.Sub(startTime).Milliseconds(),
				"temperature":   temperature,
				"max_tokens":    maxTokens,
				"message_count": len(messages),
			},
		},
	}

	// Add error message if failed
	if execErr != nil {
		event["body"].(map[string]any)["statusMessage"] = execErr.Error()
	}

	// Send event to Langfuse
	if err := tracer.SendEvent(ctx, event); err != nil {
		slog.Warn("langfuse: failed to trace LLM completion",
			"error", err,
			"slot", slot,
			"model", model,
		)
		return
	}

	slog.Debug("langfuse: traced LLM completion",
		"slot", slot,
		"model", model,
		"prompt_tokens", promptTokens,
		"completion_tokens", completionTokens,
		"finish_reason", finishReason,
	)
}

// traceToolCall traces a tool execution as a Langfuse span event.
// This runs in a goroutine and logs any errors without propagating them.
func traceToolCall(
	ctx context.Context,
	tracer EventSender,
	parentLog *AgentExecutionLog,
	req any,
	resp any,
	execErr error,
	startTime time.Time,
) {
	// Extract tool request details
	var toolName string
	var input map[string]any

	if toolReq, ok := req.(*middleware.ToolRequest); ok {
		toolName = toolReq.Name
		input = toolReq.Input
	}

	// Try context if not in request
	if toolName == "" {
		toolName = middleware.GetToolName(ctx)
	}

	// Extract response
	var output map[string]any
	if execErr == nil && resp != nil {
		if outputMap, ok := resp.(map[string]any); ok {
			output = outputMap
		}
	}

	// Handle error case
	var errorMsg string
	if execErr != nil {
		errorMsg = execErr.Error()
	}

	// Calculate end time
	endTime := time.Now()

	// Generate unique span ID for this tool call
	spanID := fmt.Sprintf("agent-tool-%s-%d", parentLog.Execution.ID.String(), time.Now().UnixNano())

	// Get trace ID from parent (mission trace)
	traceID := fmt.Sprintf("mission-%s", parentLog.Execution.ID.String())

	// Create span-create event
	event := map[string]any{
		"type":      "span-create",
		"id":        generateEventID("span", spanID),
		"timestamp": startTime.Format(time.RFC3339Nano),
		"body": map[string]any{
			"id":                  spanID,
			"traceId":             traceID,
			"parentObservationId": parentLog.SpanID, // Nest under agent execution span
			"name":                fmt.Sprintf("tool-call-%s", toolName),
			"startTime":           startTime.Format(time.RFC3339Nano),
			"endTime":             endTime.Format(time.RFC3339Nano),
			"level":               determineLevel(execErr),
			"metadata": map[string]any{
				"tool_name":     toolName,
				"agent_name":    parentLog.AgentName,
				"input":         input,
				"output":        output,
				"error":         errorMsg,
				"duration_ms":   endTime.Sub(startTime).Milliseconds(),
				"execution_id":  parentLog.Execution.ID.String(),
				"workflow_node": parentLog.Execution.WorkflowNodeID,
			},
		},
	}

	// Add error message if failed
	if execErr != nil {
		event["body"].(map[string]any)["statusMessage"] = errorMsg
	}

	// Send event to Langfuse
	if err := tracer.SendEvent(ctx, event); err != nil {
		slog.Warn("langfuse: failed to trace tool call",
			"error", err,
			"tool_name", toolName,
		)
		return
	}

	slog.Debug("langfuse: traced tool call",
		"tool_name", toolName,
		"duration_ms", endTime.Sub(startTime).Milliseconds(),
		"error", errorMsg,
	)
}

// buildFullPromptString converts messages to a comprehensive string representation for Langfuse.
// This enhanced version includes tool calls, tool call IDs, and clear role-based delimiters
// to provide complete visibility into agent LLM interactions.
func buildFullPromptString(messages []llm.Message) string {
	if len(messages) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, msg := range messages {
		if i > 0 {
			sb.WriteString("\n\n---\n\n")
		}

		// Add role prefix in uppercase for clarity
		role := strings.ToUpper(string(msg.Role))
		sb.WriteString(fmt.Sprintf("[%s]:\n%s", role, msg.Content))

		// Include tool calls if present
		if len(msg.ToolCalls) > 0 {
			toolCallsJSON, err := json.Marshal(msg.ToolCalls)
			if err == nil {
				sb.WriteString(fmt.Sprintf("\n[Tool Calls]: %s", string(toolCallsJSON)))
			}
		}

		// Include tool call ID if present
		if msg.ToolCallID != "" {
			sb.WriteString(fmt.Sprintf("\n[Tool Call ID]: %s", msg.ToolCallID))
		}
	}
	return sb.String()
}

// extractCompletionOptions extracts temperature and maxTokens from completion options.
// This handles the various option formats that may be passed to LLM completion calls.
// Returns zero values (0.0, 0) if extraction fails or options are not in expected format.
func extractCompletionOptions(opts any) (temperature float64, maxTokens int) {
	// Default to zero values
	temperature = 0.0
	maxTokens = 0

	// Try to extract from slice of options
	if optsSlice, ok := opts.([]any); ok {
		for _, opt := range optsSlice {
			// Try map[string]any format
			if optMap, ok := opt.(map[string]any); ok {
				if temp, ok := optMap["temperature"].(float64); ok {
					temperature = temp
				}
				if tokens, ok := optMap["max_tokens"].(int); ok {
					maxTokens = tokens
				}
			}
		}
	}

	return temperature, maxTokens
}

// determineLevel returns the Langfuse level based on error presence.
func determineLevel(err error) string {
	if err != nil {
		return "ERROR"
	}
	return "DEFAULT"
}
