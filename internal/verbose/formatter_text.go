package verbose

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ANSI color codes for terminal output
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorGray    = "\033[90m"
	colorGreen   = "\033[32m"
)

// TextVerboseFormatter formats events as human-readable colored text.
// Format: HH:MM:SS.mmm [COMPONENT] Message
//
//	└─ Detail line
//	└─ Detail line
//
// Component colors:
//   - LLM: cyan
//   - TOOL: green (success) / red (failure)
//   - AGENT: magenta
//   - MISSION: blue
//   - MEMORY: gray
//   - SYSTEM: yellow
type TextVerboseFormatter struct {
	useColors bool
}

// NewTextVerboseFormatter creates a new text formatter with TTY detection.
// Colors are enabled if:
//   - Output is a TTY
//   - NO_COLOR environment variable is not set
func NewTextVerboseFormatter() *TextVerboseFormatter {
	useColors := isTerminal() && os.Getenv("NO_COLOR") == ""
	return &TextVerboseFormatter{
		useColors: useColors,
	}
}

// Format converts a VerboseEvent to a formatted text string.
func (f *TextVerboseFormatter) Format(event VerboseEvent) string {
	var sb strings.Builder

	// Timestamp: HH:MM:SS.mmm
	timestamp := event.Timestamp.Format("15:04:05.000")

	// Extract component from event type (e.g., "llm.request.started" -> "LLM")
	component := f.extractComponent(event.Type)
	componentColor := f.getComponentColor(component, event.Type)

	// Format main line: HH:MM:SS.mmm [COMPONENT] Message
	if f.useColors {
		sb.WriteString(fmt.Sprintf("%s%s%s [%s%s%s] ",
			colorGray, timestamp, colorReset,
			componentColor, component, colorReset))
	} else {
		sb.WriteString(fmt.Sprintf("%s [%s] ", timestamp, component))
	}

	// Generate message from event type and payload
	message := f.generateMessage(event)
	sb.WriteString(message)
	sb.WriteString("\n")

	// Add detail lines if applicable
	details := f.generateDetails(event)
	for _, detail := range details {
		if f.useColors {
			sb.WriteString(fmt.Sprintf("%s└─ %s%s\n", colorGray, colorReset, detail))
		} else {
			sb.WriteString(fmt.Sprintf("└─ %s\n", detail))
		}
	}

	return sb.String()
}

// extractComponent extracts the component name from the event type prefix.
// Examples: "llm.request.started" -> "LLM", "tool.call.started" -> "TOOL"
func (f *TextVerboseFormatter) extractComponent(eventType VerboseEventType) string {
	parts := strings.SplitN(string(eventType), ".", 2)
	if len(parts) > 0 {
		return strings.ToUpper(parts[0])
	}
	return "UNKNOWN"
}

// getComponentColor returns the ANSI color code for a component.
func (f *TextVerboseFormatter) getComponentColor(component string, eventType VerboseEventType) string {
	if !f.useColors {
		return ""
	}

	switch component {
	case "LLM":
		return colorCyan
	case "TOOL":
		// Red for failures, green for success
		if strings.Contains(string(eventType), "failed") || strings.Contains(string(eventType), "not_found") {
			return colorRed
		}
		return colorGreen
	case "AGENT":
		return colorMagenta
	case "MISSION":
		return colorBlue
	case "MEMORY":
		return colorGray
	case "SYSTEM":
		return colorYellow
	default:
		return colorReset
	}
}

// generateMessage creates the main message from the event type and payload.
func (f *TextVerboseFormatter) generateMessage(event VerboseEvent) string {
	switch event.Type {
	// LLM events
	case EventLLMRequestStarted:
		if data, ok := event.Payload.(*LLMRequestStartedData); ok {
			mode := "completion"
			if data.Stream {
				mode = "stream"
			}
			return fmt.Sprintf("Request started: %s/%s (slot=%s, mode=%s, messages=%d)",
				data.Provider, data.Model, data.SlotName, mode, data.MessageCount)
		}
	case EventLLMRequestCompleted:
		if data, ok := event.Payload.(*LLMRequestCompletedData); ok {
			return fmt.Sprintf("Request completed: %s (duration=%s, tokens=%d/%d)",
				data.Model, data.Duration.Round(time.Millisecond),
				data.InputTokens, data.OutputTokens)
		}
	case EventLLMRequestFailed:
		if data, ok := event.Payload.(*LLMRequestFailedData); ok {
			return fmt.Sprintf("Request failed: %s - %s", data.Model, truncate(data.Error, 200))
		}
	case EventLLMStreamStarted:
		if data, ok := event.Payload.(*LLMStreamStartedData); ok {
			return fmt.Sprintf("Stream started: %s/%s (slot=%s, messages=%d)",
				data.Provider, data.Model, data.SlotName, data.MessageCount)
		}
	case EventLLMStreamChunk:
		if data, ok := event.Payload.(*LLMStreamChunkData); ok {
			return fmt.Sprintf("Stream chunk #%d: %d chars (total=%d)",
				data.ChunkIndex, len(data.ContentDelta), data.ContentLength)
		}
	case EventLLMStreamCompleted:
		if data, ok := event.Payload.(*LLMStreamCompletedData); ok {
			return fmt.Sprintf("Stream completed: %s (duration=%s, chunks=%d, tokens=%d/%d)",
				data.Model, data.Duration.Round(time.Millisecond),
				data.TotalChunks, data.InputTokens, data.OutputTokens)
		}

	// Tool events
	case EventToolCallStarted:
		if data, ok := event.Payload.(*ToolCallStartedData); ok {
			return fmt.Sprintf("Tool call started: %s (param_size=%d bytes)",
				data.ToolName, data.ParameterSize)
		}
	case EventToolCallCompleted:
		if data, ok := event.Payload.(*ToolCallCompletedData); ok {
			status := "success"
			if !data.Success {
				status = "failure"
			}
			return fmt.Sprintf("Tool call completed: %s (%s, duration=%s, result=%d bytes)",
				data.ToolName, status, data.Duration.Round(time.Millisecond), data.ResultSize)
		}
	case EventToolCallFailed:
		if data, ok := event.Payload.(*ToolCallFailedData); ok {
			return fmt.Sprintf("Tool call failed: %s - %s", data.ToolName, truncate(data.Error, 200))
		}
	case EventToolNotFound:
		if data, ok := event.Payload.(*ToolNotFoundData); ok {
			return fmt.Sprintf("Tool not found: %s (requested by %s)", data.ToolName, data.RequestedBy)
		}

	// Agent events
	case EventAgentStarted:
		if data, ok := event.Payload.(*AgentStartedData); ok {
			if data.TaskDescription != "" {
				return fmt.Sprintf("Agent started: %s - %s", data.AgentName, truncate(data.TaskDescription, 200))
			}
			return fmt.Sprintf("Agent started: %s", data.AgentName)
		}
	case EventAgentCompleted:
		if data, ok := event.Payload.(*AgentCompletedData); ok {
			return fmt.Sprintf("Agent completed: %s (duration=%s, findings=%d)",
				data.AgentName, data.Duration.Round(time.Millisecond), data.FindingCount)
		}
	case EventAgentFailed:
		if data, ok := event.Payload.(*AgentFailedData); ok {
			return fmt.Sprintf("Agent failed: %s - %s", data.AgentName, truncate(data.Error, 200))
		}
	case EventAgentDelegated:
		if data, ok := event.Payload.(*AgentDelegatedData); ok {
			if data.TaskDescription != "" {
				return fmt.Sprintf("Agent delegated: %s → %s (%s)",
					data.FromAgent, data.ToAgent, truncate(data.TaskDescription, 150))
			}
			return fmt.Sprintf("Agent delegated: %s → %s", data.FromAgent, data.ToAgent)
		}
	case EventFindingSubmitted:
		if data, ok := event.Payload.(*FindingSubmittedData); ok {
			return fmt.Sprintf("Finding submitted: [%s] %s (agent=%s)",
				strings.ToUpper(data.Severity), data.Title, data.AgentName)
		}

	// Mission events
	case EventMissionStarted:
		if data, ok := event.Payload.(*MissionStartedData); ok {
			if data.WorkflowName != "" {
				return fmt.Sprintf("Mission started: %s (workflow=%s, nodes=%d)",
					data.MissionID, data.WorkflowName, data.NodeCount)
			}
			return fmt.Sprintf("Mission started: %s (nodes=%d)", data.MissionID, data.NodeCount)
		}
	case EventMissionProgress:
		if data, ok := event.Payload.(*MissionProgressData); ok {
			progress := float64(data.CompletedNodes) / float64(data.TotalNodes) * 100
			msg := fmt.Sprintf("Mission progress: %.0f%% (%d/%d nodes)",
				progress, data.CompletedNodes, data.TotalNodes)
			if data.Message != "" {
				msg += " - " + truncate(data.Message, 150)
			}
			return msg
		}
	case EventMissionNode:
		if data, ok := event.Payload.(*MissionNodeData); ok {
			msg := fmt.Sprintf("Mission node: %s [%s] %s", data.NodeID, data.NodeType, data.Status)
			if data.Duration > 0 {
				msg += fmt.Sprintf(" (duration=%s)", data.Duration.Round(time.Millisecond))
			}
			return msg
		}
	case EventMissionCompleted:
		if data, ok := event.Payload.(*MissionCompletedData); ok {
			return fmt.Sprintf("Mission completed: %s (duration=%s, nodes=%d, findings=%d)",
				data.MissionID, data.Duration.Round(time.Millisecond),
				data.NodesExecuted, data.FindingCount)
		}
	case EventMissionFailed:
		if data, ok := event.Payload.(*MissionFailedData); ok {
			return fmt.Sprintf("Mission failed: %s - %s",
				data.MissionID, truncate(data.Error, 200))
		}

	// Memory events
	case EventMemoryGet:
		if data, ok := event.Payload.(*MemoryGetData); ok {
			found := "miss"
			if data.Found {
				found = "hit"
			}
			return fmt.Sprintf("Memory get: tier=%s, key=%s, result=%s",
				data.Tier, truncate(data.Key, 100), found)
		}
	case EventMemorySet:
		if data, ok := event.Payload.(*MemorySetData); ok {
			return fmt.Sprintf("Memory set: tier=%s, key=%s, value_size=%d bytes",
				data.Tier, truncate(data.Key, 100), data.ValueSize)
		}
	case EventMemorySearch:
		if data, ok := event.Payload.(*MemorySearchData); ok {
			return fmt.Sprintf("Memory search: tier=%s, query=%s, results=%d (duration=%s)",
				data.Tier, truncate(data.Query, 100), data.ResultCount,
				data.Duration.Round(time.Millisecond))
		}

	// System events
	case EventComponentRegistered:
		if data, ok := event.Payload.(*ComponentRegisteredData); ok {
			msg := fmt.Sprintf("Component registered: %s/%s", data.ComponentType, data.ComponentName)
			if data.Version != "" {
				msg += fmt.Sprintf(" (version=%s)", data.Version)
			}
			return msg
		}
	case EventComponentHealth:
		if data, ok := event.Payload.(*ComponentHealthData); ok {
			health := "unhealthy"
			if data.Healthy {
				health = "healthy"
			}
			return fmt.Sprintf("Component health: %s/%s - %s", data.ComponentType, data.ComponentName, health)
		}
	case EventDaemonStarted:
		if data, ok := event.Payload.(*DaemonStartedData); ok {
			return fmt.Sprintf("Daemon started: version=%s, address=%s",
				data.Version, data.ListenAddress)
		}
	}

	// Fallback for unknown event types
	return fmt.Sprintf("Event: %s", event.Type)
}

// generateDetails creates detail lines for verbose output.
func (f *TextVerboseFormatter) generateDetails(event VerboseEvent) []string {
	var details []string

	switch event.Type {
	case EventLLMRequestStarted:
		if data, ok := event.Payload.(*LLMRequestStartedData); ok {
			if data.MaxTokens > 0 {
				details = append(details, fmt.Sprintf("max_tokens=%d", data.MaxTokens))
			}
			if data.Temperature > 0 {
				details = append(details, fmt.Sprintf("temperature=%.2f", data.Temperature))
			}
		}
	case EventLLMRequestCompleted:
		if data, ok := event.Payload.(*LLMRequestCompletedData); ok {
			details = append(details, fmt.Sprintf("response_length=%d chars", data.ResponseLength))
			if data.StopReason != "" {
				details = append(details, fmt.Sprintf("stop_reason=%s", data.StopReason))
			}
		}
	case EventLLMRequestFailed:
		if data, ok := event.Payload.(*LLMRequestFailedData); ok {
			if data.Retryable {
				details = append(details, "retryable=true")
			}
		}
	case EventToolCallStarted:
		if data, ok := event.Payload.(*ToolCallStartedData); ok {
			if len(data.Parameters) > 0 {
				// Show first few parameter keys
				keys := make([]string, 0, len(data.Parameters))
				for k := range data.Parameters {
					keys = append(keys, k)
					if len(keys) >= 3 {
						break
					}
				}
				details = append(details, fmt.Sprintf("params: %s", strings.Join(keys, ", ")))
			}
		}
	case EventFindingSubmitted:
		if data, ok := event.Payload.(*FindingSubmittedData); ok {
			if len(data.TechniqueIDs) > 0 {
				details = append(details, fmt.Sprintf("techniques: %s", strings.Join(data.TechniqueIDs, ", ")))
			}
		}
	case EventComponentRegistered:
		if data, ok := event.Payload.(*ComponentRegisteredData); ok {
			if len(data.Capabilities) > 0 {
				details = append(details, fmt.Sprintf("capabilities: %s", strings.Join(data.Capabilities, ", ")))
			}
		}
	case EventComponentHealth:
		if data, ok := event.Payload.(*ComponentHealthData); ok {
			if data.Status != "" {
				details = append(details, fmt.Sprintf("status: %s", data.Status))
			}
			if data.ResponseTime > 0 {
				details = append(details, fmt.Sprintf("response_time=%s", data.ResponseTime.Round(time.Millisecond)))
			}
		}
	case EventDaemonStarted:
		if data, ok := event.Payload.(*DaemonStartedData); ok {
			if data.ConfigPath != "" {
				details = append(details, fmt.Sprintf("config: %s", data.ConfigPath))
			}
			if data.DataDir != "" {
				details = append(details, fmt.Sprintf("data_dir: %s", data.DataDir))
			}
		}
	}

	return details
}

// truncate truncates a string to a maximum length with "..." suffix.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..."
	}
	return s[:maxLen-3] + "..."
}

// isTerminal checks if stdout is a terminal.
func isTerminal() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// Ensure TextVerboseFormatter implements VerboseFormatter at compile time
var _ VerboseFormatter = (*TextVerboseFormatter)(nil)
