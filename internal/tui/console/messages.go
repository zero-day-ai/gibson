package console

import (
	"github.com/zero-day-ai/gibson/internal/database"
)

// AgentEventMsg is a Bubble Tea message that delivers agent stream events
// to the ConsoleView for display. It wraps a StreamEvent with the agent name
// for proper routing when multiple agents may be running.
type AgentEventMsg struct {
	// AgentName identifies which agent this event is from
	AgentName string
	// Event contains the actual stream event data
	Event *database.StreamEvent
}

// AgentDisconnectedMsg is a Bubble Tea message sent when an agent's
// stream connection is closed, either intentionally or due to an error.
type AgentDisconnectedMsg struct {
	// AgentName identifies which agent disconnected
	AgentName string
	// Reason provides context for why the disconnection occurred
	Reason string
}
