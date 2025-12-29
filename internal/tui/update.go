package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	// tickInterval defines how often the TUI polls for updates
	tickInterval = 1 * time.Second
)

// tickCmd returns a command that sends a TickMsg after the tick interval.
func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return TickMsg{Timestamp: t}
	})
}

// RefreshMissionsCmd returns a command that triggers a mission data refresh.
func RefreshMissionsCmd() tea.Cmd {
	return func() tea.Msg {
		return MissionsUpdatedMsg{
			Timestamp: time.Now(),
		}
	}
}

// RefreshAgentsCmd returns a command that triggers an agent status refresh.
func RefreshAgentsCmd() tea.Cmd {
	return func() tea.Msg {
		return AgentsUpdatedMsg{
			Timestamp: time.Now(),
			Agents:    []AgentInfo{}, // Would be populated from registry
		}
	}
}

// RefreshFindingsCmd returns a command that triggers a findings refresh.
func RefreshFindingsCmd() tea.Cmd {
	return func() tea.Msg {
		return FindingsUpdatedMsg{
			Timestamp: time.Now(),
			Findings:  nil, // Would be populated from store
		}
	}
}

// SendLogLineCmd returns a command that sends a log line message.
func SendLogLineCmd(missionID, level, message string) tea.Cmd {
	return func() tea.Msg {
		return LogLineMsg{
			Timestamp: time.Now(),
			MissionID: missionID,
			Level:     level,
			Message:   message,
		}
	}
}

// SendErrorCmd returns a command that sends an error message.
func SendErrorCmd(err error, message string) tea.Cmd {
	return func() tea.Msg {
		return ErrorMsg{
			Error:     err,
			Message:   message,
			Timestamp: time.Now(),
		}
	}
}

// SwitchModeCmd returns a command that switches to the specified mode.
func SwitchModeCmd(mode AppMode) tea.Cmd {
	return func() tea.Msg {
		return SwitchModeMsg{Mode: mode}
	}
}

// RequestApprovalCmd returns a command that requests user approval for a plan.
func RequestApprovalCmd(id, title, description string, steps []PlanStep) tea.Cmd {
	return func() tea.Msg {
		return ApprovalRequestMsg{
			ID:          id,
			Title:       title,
			Description: description,
			Plan:        steps,
			Timestamp:   time.Now(),
		}
	}
}

// SubscriptionManager manages real-time update subscriptions.
// It provides channels and callbacks for various system events.
type SubscriptionManager struct {
	// Channels for event subscription
	missionUpdates  chan MissionsUpdatedMsg
	agentUpdates    chan AgentsUpdatedMsg
	findingUpdates  chan FindingsUpdatedMsg
	logUpdates      chan LogLineMsg
	approvalRequest chan ApprovalRequestMsg

	// Stop channel
	stopCh chan struct{}
}

// NewSubscriptionManager creates a new subscription manager.
func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{
		missionUpdates:  make(chan MissionsUpdatedMsg, 100),
		agentUpdates:    make(chan AgentsUpdatedMsg, 100),
		findingUpdates:  make(chan FindingsUpdatedMsg, 100),
		logUpdates:      make(chan LogLineMsg, 1000),
		approvalRequest: make(chan ApprovalRequestMsg, 10),
		stopCh:          make(chan struct{}),
	}
}

// Stop stops the subscription manager.
func (sm *SubscriptionManager) Stop() {
	close(sm.stopCh)
}

// PublishMissionUpdate publishes a mission update event.
func (sm *SubscriptionManager) PublishMissionUpdate(msg MissionsUpdatedMsg) {
	select {
	case sm.missionUpdates <- msg:
	default:
		// Drop message if channel is full
	}
}

// PublishAgentUpdate publishes an agent update event.
func (sm *SubscriptionManager) PublishAgentUpdate(msg AgentsUpdatedMsg) {
	select {
	case sm.agentUpdates <- msg:
	default:
		// Drop message if channel is full
	}
}

// PublishFindingUpdate publishes a finding update event.
func (sm *SubscriptionManager) PublishFindingUpdate(msg FindingsUpdatedMsg) {
	select {
	case sm.findingUpdates <- msg:
	default:
		// Drop message if channel is full
	}
}

// PublishLogLine publishes a log line event.
func (sm *SubscriptionManager) PublishLogLine(msg LogLineMsg) {
	select {
	case sm.logUpdates <- msg:
	default:
		// Drop message if channel is full
	}
}

// PublishApprovalRequest publishes an approval request event.
func (sm *SubscriptionManager) PublishApprovalRequest(msg ApprovalRequestMsg) {
	select {
	case sm.approvalRequest <- msg:
	default:
		// Drop message if channel is full
	}
}

// MissionUpdatesCmd returns a command that listens for mission updates.
func (sm *SubscriptionManager) MissionUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-sm.missionUpdates:
			return msg
		case <-sm.stopCh:
			return nil
		}
	}
}

// AgentUpdatesCmd returns a command that listens for agent updates.
func (sm *SubscriptionManager) AgentUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-sm.agentUpdates:
			return msg
		case <-sm.stopCh:
			return nil
		}
	}
}

// FindingUpdatesCmd returns a command that listens for finding updates.
func (sm *SubscriptionManager) FindingUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-sm.findingUpdates:
			return msg
		case <-sm.stopCh:
			return nil
		}
	}
}

// LogUpdatesCmd returns a command that listens for log line updates.
func (sm *SubscriptionManager) LogUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-sm.logUpdates:
			return msg
		case <-sm.stopCh:
			return nil
		}
	}
}

// ApprovalRequestsCmd returns a command that listens for approval requests.
func (sm *SubscriptionManager) ApprovalRequestsCmd() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-sm.approvalRequest:
			return msg
		case <-sm.stopCh:
			return nil
		}
	}
}

// BatchUpdateCmd returns a command that checks all channels and returns any pending messages.
// This is useful for polling-based updates during the tick cycle.
func (sm *SubscriptionManager) BatchUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		// Check each channel non-blocking
		select {
		case msg := <-sm.missionUpdates:
			return msg
		default:
		}

		select {
		case msg := <-sm.agentUpdates:
			return msg
		default:
		}

		select {
		case msg := <-sm.findingUpdates:
			return msg
		default:
		}

		select {
		case msg := <-sm.logUpdates:
			return msg
		default:
		}

		select {
		case msg := <-sm.approvalRequest:
			return msg
		default:
		}

		return nil
	}
}
