package console

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/database"
)

// EventRenderer converts StreamEvents into styled OutputLines for display
// in the console view. It handles parsing event content JSON and applying
// appropriate styling based on event type.
type EventRenderer struct {
	// agentName is the name of the currently focused agent for labeling
	agentName string
}

// NewEventRenderer creates a new EventRenderer for the specified agent.
func NewEventRenderer(agentName string) *EventRenderer {
	return &EventRenderer{
		agentName: agentName,
	}
}

// SetAgentName updates the agent name used for labeling output.
func (r *EventRenderer) SetAgentName(name string) {
	r.agentName = name
}

// OutputContent represents the parsed content of an output event.
type OutputContent struct {
	Text     string `json:"text"`
	Complete bool   `json:"complete"`
}

// ToolCallContent represents the parsed content of a tool_call event.
type ToolCallContent struct {
	ToolName  string         `json:"tool_name"`
	ToolID    string         `json:"tool_id"`
	Arguments map[string]any `json:"arguments"`
}

// ToolResultContent represents the parsed content of a tool_result event.
type ToolResultContent struct {
	ToolID  string `json:"tool_id"`
	Success bool   `json:"success"`
	Output  any    `json:"output"`
	Error   string `json:"error,omitempty"`
}

// FindingContent represents the parsed content of a finding event.
type FindingContent struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// StatusContent represents the parsed content of a status event.
type StatusContent struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// SteeringAckContent represents the parsed content of a steering_ack event.
type SteeringAckContent struct {
	Sequence int64  `json:"sequence"`
	Accepted bool   `json:"accepted"`
	Message  string `json:"message,omitempty"`
}

// ErrorContent represents the parsed content of an error event.
type ErrorContent struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// RenderEvent converts a StreamEvent into styled OutputLines.
// It dispatches to the appropriate render method based on event type.
func (r *EventRenderer) RenderEvent(event *database.StreamEvent) []OutputLine {
	if event == nil {
		return nil
	}

	switch event.EventType {
	case database.StreamEventOutput:
		return r.RenderOutput(event.Content, event.Timestamp)
	case database.StreamEventToolCall:
		return r.RenderToolCall(event.Content, event.Timestamp)
	case database.StreamEventToolResult:
		return r.RenderToolResult(event.Content, event.Timestamp)
	case database.StreamEventFinding:
		return r.RenderFinding(event.Content, event.Timestamp)
	case database.StreamEventStatus:
		return r.RenderStatus(event.Content, event.Timestamp)
	case database.StreamEventSteeringAck:
		return r.RenderSteeringAck(event.Content, event.Timestamp)
	case database.StreamEventError:
		return r.RenderError(event.Content, event.Timestamp)
	default:
		return r.renderUnknown(event)
	}
}

// RenderOutput renders an output event (agent LLM output/reasoning).
func (r *EventRenderer) RenderOutput(content json.RawMessage, timestamp time.Time) []OutputLine {
	var output OutputContent
	if err := json.Unmarshal(content, &output); err != nil {
		return r.renderParseError("output", content, timestamp)
	}

	// Split text into lines, preserving multi-line output
	text := sanitizeTerminalOutput(output.Text)
	if text == "" {
		return nil
	}

	lines := strings.Split(text, "\n")
	result := make([]OutputLine, 0, len(lines))

	for i, line := range lines {
		// Skip empty trailing line from split
		if i == len(lines)-1 && line == "" && !output.Complete {
			continue
		}

		result = append(result, OutputLine{
			Text:      line,
			Style:     StyleAgentOutput,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		})
	}

	return result
}

// RenderToolCall renders a tool_call event (agent invoking a tool).
func (r *EventRenderer) RenderToolCall(content json.RawMessage, timestamp time.Time) []OutputLine {
	var toolCall ToolCallContent
	if err := json.Unmarshal(content, &toolCall); err != nil {
		return r.renderParseError("tool_call", content, timestamp)
	}

	// Format the tool call header
	header := fmt.Sprintf(">>> Calling tool: %s", toolCall.ToolName)

	result := []OutputLine{
		{
			Text:      header,
			Style:     StyleToolCall,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		},
	}

	// Format arguments if present
	if len(toolCall.Arguments) > 0 {
		argsJSON, err := json.MarshalIndent(toolCall.Arguments, "    ", "  ")
		if err == nil {
			argLines := strings.Split(string(argsJSON), "\n")
			for _, line := range argLines {
				result = append(result, OutputLine{
					Text:      "    " + line,
					Style:     StyleToolCall,
					Timestamp: timestamp,
					Source:    SourceAgent,
					AgentName: r.agentName,
				})
			}
		}
	}

	return result
}

// RenderToolResult renders a tool_result event (tool execution response).
func (r *EventRenderer) RenderToolResult(content json.RawMessage, timestamp time.Time) []OutputLine {
	var toolResult ToolResultContent
	if err := json.Unmarshal(content, &toolResult); err != nil {
		return r.renderParseError("tool_result", content, timestamp)
	}

	var result []OutputLine

	if toolResult.Success {
		// Success case
		header := "<<< Tool result: success"
		result = append(result, OutputLine{
			Text:      header,
			Style:     StyleToolResult,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		})

		// Format output if present
		if toolResult.Output != nil {
			outputStr := formatToolOutput(toolResult.Output)
			outputLines := strings.Split(outputStr, "\n")
			for _, line := range outputLines {
				if line != "" {
					result = append(result, OutputLine{
						Text:      "    " + truncateString(line, 200),
						Style:     StyleToolResult,
						Timestamp: timestamp,
						Source:    SourceAgent,
						AgentName: r.agentName,
					})
				}
			}
		}
	} else {
		// Error case
		header := "<<< Tool result: failed"
		result = append(result, OutputLine{
			Text:      header,
			Style:     StyleError,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		})

		if toolResult.Error != "" {
			result = append(result, OutputLine{
				Text:      "    Error: " + toolResult.Error,
				Style:     StyleError,
				Timestamp: timestamp,
				Source:    SourceAgent,
				AgentName: r.agentName,
			})
		}
	}

	return result
}

// RenderFinding renders a finding event (security vulnerability discovered).
func (r *EventRenderer) RenderFinding(content json.RawMessage, timestamp time.Time) []OutputLine {
	var finding FindingContent
	if err := json.Unmarshal(content, &finding); err != nil {
		return r.renderParseError("finding", content, timestamp)
	}

	// Determine style based on severity
	style := severityToStyle(finding.Severity)

	// Format the finding header with severity
	severityUpper := strings.ToUpper(finding.Severity)
	header := fmt.Sprintf("[%s] %s", severityUpper, finding.Title)

	result := []OutputLine{
		{
			Text:      "=== FINDING ===",
			Style:     style,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		},
		{
			Text:      header,
			Style:     style,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		},
	}

	// Add category if present
	if finding.Category != "" {
		result = append(result, OutputLine{
			Text:      fmt.Sprintf("Category: %s", finding.Category),
			Style:     style,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		})
	}

	// Add description if present (truncated)
	if finding.Description != "" {
		desc := truncateString(finding.Description, 200)
		result = append(result, OutputLine{
			Text:      desc,
			Style:     style,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		})
	}

	// Add hint to view full finding
	if finding.ID != "" {
		result = append(result, OutputLine{
			Text:      fmt.Sprintf("Use /findings show %s to view details", finding.ID),
			Style:     StyleInfo,
			Timestamp: timestamp,
			Source:    SourceSystem,
			AgentName: r.agentName,
		})
	}

	return result
}

// RenderStatus renders a status event (agent state change).
func (r *EventRenderer) RenderStatus(content json.RawMessage, timestamp time.Time) []OutputLine {
	var status StatusContent
	if err := json.Unmarshal(content, &status); err != nil {
		return r.renderParseError("status", content, timestamp)
	}

	var result []OutputLine
	var style OutputStyle = StyleStatus

	// Build status message based on status type
	var statusMsg string
	switch status.Status {
	case "running":
		statusMsg = fmt.Sprintf("[%s] Agent started", r.agentName)
	case "paused":
		statusMsg = fmt.Sprintf("[%s] Agent paused - Use /resume to continue", r.agentName)
	case "waiting_for_input":
		statusMsg = fmt.Sprintf("[%s] Agent waiting for input", r.agentName)
		style = StyleInfo
	case "completed":
		statusMsg = fmt.Sprintf("[%s] Agent completed successfully", r.agentName)
		style = StyleSuccess
	case "failed":
		statusMsg = fmt.Sprintf("[%s] Agent failed", r.agentName)
		style = StyleError
	case "interrupted":
		statusMsg = fmt.Sprintf("[%s] Agent interrupted", r.agentName)
	default:
		statusMsg = fmt.Sprintf("[%s] Status: %s", r.agentName, status.Status)
	}

	result = append(result, OutputLine{
		Text:      statusMsg,
		Style:     style,
		Timestamp: timestamp,
		Source:    SourceAgent,
		AgentName: r.agentName,
	})

	// Add message or reason if present
	if status.Message != "" {
		result = append(result, OutputLine{
			Text:      "  " + status.Message,
			Style:     style,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		})
	}

	if status.Reason != "" {
		result = append(result, OutputLine{
			Text:      "  Reason: " + status.Reason,
			Style:     style,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		})
	}

	return result
}

// RenderSteeringAck renders a steering_ack event (acknowledgment of steering).
func (r *EventRenderer) RenderSteeringAck(content json.RawMessage, timestamp time.Time) []OutputLine {
	var ack SteeringAckContent
	if err := json.Unmarshal(content, &ack); err != nil {
		return r.renderParseError("steering_ack", content, timestamp)
	}

	var result []OutputLine

	if ack.Accepted {
		result = append(result, OutputLine{
			Text:      fmt.Sprintf("[%s] Steering accepted (seq %d)", r.agentName, ack.Sequence),
			Style:     StyleSteeringAck,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		})
	} else {
		result = append(result, OutputLine{
			Text:      fmt.Sprintf("[%s] Steering rejected (seq %d)", r.agentName, ack.Sequence),
			Style:     StyleError,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		})
	}

	if ack.Message != "" {
		result = append(result, OutputLine{
			Text:      "  " + ack.Message,
			Style:     StyleInfo,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		})
	}

	return result
}

// RenderError renders an error event from the agent.
func (r *EventRenderer) RenderError(content json.RawMessage, timestamp time.Time) []OutputLine {
	var errContent ErrorContent
	if err := json.Unmarshal(content, &errContent); err != nil {
		return r.renderParseError("error", content, timestamp)
	}

	result := []OutputLine{
		{
			Text:      fmt.Sprintf("[%s] Error: %s", r.agentName, errContent.Message),
			Style:     StyleError,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		},
	}

	if errContent.Code != "" {
		result = append(result, OutputLine{
			Text:      "  Code: " + errContent.Code,
			Style:     StyleError,
			Timestamp: timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		})
	}

	return result
}

// renderUnknown handles unknown event types.
func (r *EventRenderer) renderUnknown(event *database.StreamEvent) []OutputLine {
	return []OutputLine{
		{
			Text:      fmt.Sprintf("[%s] Unknown event type: %s", r.agentName, event.EventType),
			Style:     StyleInfo,
			Timestamp: event.Timestamp,
			Source:    SourceAgent,
			AgentName: r.agentName,
		},
	}
}

// renderParseError creates an error line when JSON parsing fails.
func (r *EventRenderer) renderParseError(eventType string, content json.RawMessage, timestamp time.Time) []OutputLine {
	return []OutputLine{
		{
			Text:      fmt.Sprintf("[Warning] Could not parse %s event", eventType),
			Style:     StyleError,
			Timestamp: timestamp,
			Source:    SourceSystem,
			AgentName: r.agentName,
		},
		{
			Text:      "  Raw: " + truncateString(string(content), 100),
			Style:     StyleInfo,
			Timestamp: timestamp,
			Source:    SourceSystem,
			AgentName: r.agentName,
		},
	}
}

// severityToStyle maps finding severity to the appropriate OutputStyle.
func severityToStyle(severity string) OutputStyle {
	switch strings.ToLower(severity) {
	case "critical":
		return StyleFindingCritical
	case "high":
		return StyleFindingHigh
	case "medium":
		return StyleFindingMedium
	case "low":
		return StyleFindingLow
	default:
		return StyleFinding
	}
}

// sanitizeTerminalOutput removes potentially dangerous terminal escape sequences.
func sanitizeTerminalOutput(s string) string {
	// Remove common dangerous escape sequences
	// This is a basic sanitization - a full implementation would be more comprehensive
	s = strings.ReplaceAll(s, "\x1b[", "[ESC[") // ANSI escape
	s = strings.ReplaceAll(s, "\x07", "")       // Bell
	s = strings.ReplaceAll(s, "\x08", "")       // Backspace
	s = strings.ReplaceAll(s, "\x7f", "")       // Delete
	return s
}

// truncateString truncates a string to the specified max length, adding ellipsis.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// formatToolOutput converts tool output to a displayable string.
func formatToolOutput(output any) string {
	switch v := output.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		// Try to JSON encode for complex types
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}
