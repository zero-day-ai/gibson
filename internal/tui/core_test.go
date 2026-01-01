package tui

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/stretchr/testify/assert"
	"github.com/zero-day-ai/gibson/internal/agent"
)

// Theme Tests

func TestDefaultKeyMap(t *testing.T) {
	km := DefaultKeyMap()

	// Test that all key bindings are defined
	assert.NotEmpty(t, km.Quit.Keys())
	assert.NotEmpty(t, km.Help.Keys())
	assert.NotEmpty(t, km.Tab.Keys())
	assert.NotEmpty(t, km.Escape.Keys())

	// Test view switching keys
	assert.NotEmpty(t, km.ViewDashboard.Keys())
	assert.NotEmpty(t, km.ViewConsole.Keys())
	assert.NotEmpty(t, km.ViewMission.Keys())
	assert.NotEmpty(t, km.ViewFindings.Keys())

	// Test navigation keys
	assert.NotEmpty(t, km.Up.Keys())
	assert.NotEmpty(t, km.Down.Keys())
	assert.NotEmpty(t, km.Enter.Keys())

	// Test action keys
	assert.NotEmpty(t, km.Refresh.Keys())
	assert.NotEmpty(t, km.Pause.Keys())
	assert.NotEmpty(t, km.Stop.Keys())
}

func TestKeyMapHelpText(t *testing.T) {
	km := DefaultKeyMap()
	helpText := km.HelpText()

	// Test that all categories are present
	assert.Contains(t, helpText, "Global")
	assert.Contains(t, helpText, "Views")
	assert.Contains(t, helpText, "Navigation")
	assert.Contains(t, helpText, "Actions")

	// Test that each category has bindings
	assert.NotEmpty(t, helpText["Global"])
	assert.NotEmpty(t, helpText["Views"])
	assert.NotEmpty(t, helpText["Navigation"])
	assert.NotEmpty(t, helpText["Actions"])
}

func TestKeyBindingHelp(t *testing.T) {
	km := DefaultKeyMap()

	// Test that help text is generated for each key
	quitHelp := km.Quit.Help()
	assert.NotEmpty(t, quitHelp.Key)
	assert.NotEmpty(t, quitHelp.Desc)

	helpHelp := km.Help.Help()
	assert.NotEmpty(t, helpHelp.Key)
	assert.Equal(t, "toggle help", helpHelp.Desc)
}

func TestKeyMatches(t *testing.T) {
	km := DefaultKeyMap()

	tests := []struct {
		name    string
		binding key.Binding
		key     string
		matches bool
	}{
		{"quit with q", km.Quit, "q", true},
		{"quit with ctrl+c", km.Quit, "ctrl+c", true},
		{"quit with wrong key", km.Quit, "x", false},
		{"help with ?", km.Help, "?", true},
		{"view dashboard with 1", km.ViewDashboard, "1", true},
		{"view console with 2", km.ViewConsole, "2", true},
		{"up with k", km.Up, "k", true},
		{"down with j", km.Down, "j", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := tt.binding.Keys()
			found := false
			for _, k := range keys {
				if k == tt.key {
					found = true
					break
				}
			}
			assert.Equal(t, tt.matches, found)
		})
	}
}

// Mode Tests

func TestAppMode_String(t *testing.T) {
	tests := []struct {
		mode     AppMode
		expected string
	}{
		{ModeDashboard, "Dashboard"},
		{ModeConsole, "Console"},
		{ModeMission, "Mission"},
		{ModeFindings, "Findings"},
		{AppMode(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.mode.String())
		})
	}
}

func TestModeFromKey(t *testing.T) {
	tests := []struct {
		key         string
		currentMode AppMode
		expected    AppMode
	}{
		{"f1", ModeConsole, ModeDashboard},
		{"f2", ModeDashboard, ModeConsole},
		{"f3", ModeDashboard, ModeMission},
		{"f4", ModeDashboard, ModeFindings},
		{"f5", ModeDashboard, ModeDashboard}, // Invalid key, returns current
		{"1", ModeDashboard, ModeDashboard},  // Plain number, returns current
		{"x", ModeFindings, ModeFindings},    // Invalid key, returns current
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := ModeFromKey(tt.key, tt.currentMode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Message Tests

func TestWindowSizeMsg(t *testing.T) {
	msg := WindowSizeMsg{Width: 100, Height: 50}
	assert.Equal(t, 100, msg.Width)
	assert.Equal(t, 50, msg.Height)
}

func TestSwitchModeMsg(t *testing.T) {
	msg := SwitchModeMsg{Mode: ModeConsole}
	assert.Equal(t, ModeConsole, msg.Mode)
}

func TestMissionsUpdatedMsg(t *testing.T) {
	now := time.Now()
	msg := MissionsUpdatedMsg{
		Timestamp: now,
		Count:     5,
	}
	assert.Equal(t, now, msg.Timestamp)
	assert.Equal(t, 5, msg.Count)
}

func TestAgentsUpdatedMsg(t *testing.T) {
	now := time.Now()
	agents := []AgentInfo{
		{ID: "1", Name: "agent1", Status: "running"},
		{ID: "2", Name: "agent2", Status: "stopped"},
	}
	msg := AgentsUpdatedMsg{
		Timestamp: now,
		Agents:    agents,
	}
	assert.Equal(t, now, msg.Timestamp)
	assert.Len(t, msg.Agents, 2)
}

func TestAgentInfo(t *testing.T) {
	info := AgentInfo{
		ID:     "test-id",
		Name:   "test-agent",
		Status: "running",
		Port:   8080,
		PID:    1234,
	}
	assert.Equal(t, "test-id", info.ID)
	assert.Equal(t, "test-agent", info.Name)
	assert.Equal(t, "running", info.Status)
	assert.Equal(t, 8080, info.Port)
	assert.Equal(t, 1234, info.PID)
}

func TestFindingsUpdatedMsg(t *testing.T) {
	now := time.Now()
	findings := []*agent.Finding{
		{Title: "Finding 1"},
	}
	msg := FindingsUpdatedMsg{
		Timestamp: now,
		Findings:  findings,
	}
	assert.Equal(t, now, msg.Timestamp)
	assert.Len(t, msg.Findings, 1)
}

func TestLogLineMsg(t *testing.T) {
	now := time.Now()
	msg := LogLineMsg{
		Timestamp: now,
		MissionID: "mission-1",
		Level:     "info",
		Message:   "Test log message",
	}
	assert.Equal(t, now, msg.Timestamp)
	assert.Equal(t, "mission-1", msg.MissionID)
	assert.Equal(t, "info", msg.Level)
	assert.Equal(t, "Test log message", msg.Message)
}

func TestAgentResponseMsg(t *testing.T) {
	now := time.Now()
	msg := AgentResponseMsg{
		Timestamp: now,
		AgentID:   "agent-1",
		Content:   "Response content",
		Findings:  nil,
		Error:     nil,
	}
	assert.Equal(t, now, msg.Timestamp)
	assert.Equal(t, "agent-1", msg.AgentID)
	assert.Equal(t, "Response content", msg.Content)
}

func TestApprovalRequestMsg(t *testing.T) {
	now := time.Now()
	steps := []PlanStep{
		{ID: "1", Name: "Step 1", RiskLevel: "low"},
		{ID: "2", Name: "Step 2", RiskLevel: "high"},
	}
	msg := ApprovalRequestMsg{
		ID:          "req-1",
		Title:       "Approval Request",
		Description: "Please approve",
		Plan:        steps,
		Timestamp:   now,
	}
	assert.Equal(t, "req-1", msg.ID)
	assert.Equal(t, "Approval Request", msg.Title)
	assert.Len(t, msg.Plan, 2)
}

func TestPlanStep(t *testing.T) {
	step := PlanStep{
		ID:          "step-1",
		Name:        "Execute Action",
		Description: "This action does something",
		RiskLevel:   "medium",
		Impact:      "Medium impact on system",
	}
	assert.Equal(t, "step-1", step.ID)
	assert.Equal(t, "Execute Action", step.Name)
	assert.Equal(t, "medium", step.RiskLevel)
}

func TestApprovalDecisionMsg(t *testing.T) {
	now := time.Now()
	msg := ApprovalDecisionMsg{
		RequestID: "req-1",
		Approved:  true,
		Comments:  "Looks good",
		Timestamp: now,
	}
	assert.Equal(t, "req-1", msg.RequestID)
	assert.True(t, msg.Approved)
	assert.Equal(t, "Looks good", msg.Comments)
}

func TestErrorMsg(t *testing.T) {
	now := time.Now()
	msg := ErrorMsg{
		Error:     nil,
		Message:   "Something went wrong",
		Timestamp: now,
	}
	assert.Equal(t, "Something went wrong", msg.Message)
	assert.Equal(t, now, msg.Timestamp)
}

func TestTickMsg(t *testing.T) {
	now := time.Now()
	msg := TickMsg{Timestamp: now}
	assert.Equal(t, now, msg.Timestamp)
}
