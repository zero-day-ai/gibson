package tui

import (
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
)

// WindowSizeMsg is sent when the terminal is resized
type WindowSizeMsg struct {
	Width  int
	Height int
}

// SwitchModeMsg is sent when the user switches between views
type SwitchModeMsg struct {
	Mode AppMode
}

// MissionsUpdatedMsg is sent when mission data is updated
type MissionsUpdatedMsg struct {
	Timestamp time.Time
	Count     int
}

// AgentsUpdatedMsg is sent when agent status is updated
type AgentsUpdatedMsg struct {
	Timestamp time.Time
	Agents    []AgentInfo
}

// AgentInfo contains information about an agent
type AgentInfo struct {
	ID     string
	Name   string
	Status string
	Port   int
	PID    int
}

// FindingsUpdatedMsg is sent when new findings are discovered
type FindingsUpdatedMsg struct {
	Timestamp time.Time
	Findings  []*agent.Finding
}

// LogLineMsg is sent when a new log line is received
type LogLineMsg struct {
	Timestamp time.Time
	MissionID string
	Level     string
	Message   string
}

// AgentResponseMsg is sent when an agent responds to a message
type AgentResponseMsg struct {
	Timestamp time.Time
	AgentID   string
	Content   string
	Findings  []*agent.Finding
	Error     error
}

// ApprovalRequestMsg is sent when an action requires approval
type ApprovalRequestMsg struct {
	ID          string
	Title       string
	Description string
	Plan        []PlanStep
	Timestamp   time.Time
}

// PlanStep represents a step in an execution plan
type PlanStep struct {
	ID          string
	Name        string
	Description string
	RiskLevel   string
	Impact      string
}

// ApprovalDecisionMsg is sent when the user makes an approval decision
type ApprovalDecisionMsg struct {
	RequestID string
	Approved  bool
	Comments  string
	Timestamp time.Time
}

// ErrorMsg is sent when an error occurs
type ErrorMsg struct {
	Error     error
	Message   string
	Timestamp time.Time
}

// TickMsg is sent periodically for updates
type TickMsg struct {
	Timestamp time.Time
}
