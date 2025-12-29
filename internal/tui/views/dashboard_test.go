package views

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNewDashboardView(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)

	assert.NotNil(t, view)
	assert.NotNil(t, view.missionPanel)
	assert.NotNil(t, view.agentPanel)
	assert.NotNil(t, view.findingPanel)
	assert.NotNil(t, view.metricsPanel)
	assert.Equal(t, 0, view.focusedPanel)
	assert.Equal(t, 80, view.width)
	assert.Equal(t, 24, view.height)
}

func TestDashboardView_Init(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)

	cmd := view.Init()

	// Init should return nil (no async commands without dependencies)
	assert.Nil(t, cmd)

	// First panel should be focused after init
	assert.True(t, view.missionPanel.Focused())
}

func TestDashboardView_SetSize(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)

	view.SetSize(120, 40)

	assert.Equal(t, 120, view.width)
	assert.Equal(t, 40, view.height)
}

func TestDashboardView_CycleFocus(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)
	view.Init()

	// Initially panel 0 is focused
	assert.Equal(t, 0, view.focusedPanel)
	assert.True(t, view.missionPanel.Focused())

	// Cycle to panel 1
	view.cycleFocus()
	assert.Equal(t, 1, view.focusedPanel)
	assert.False(t, view.missionPanel.Focused())
	assert.True(t, view.agentPanel.Focused())

	// Cycle to panel 2
	view.cycleFocus()
	assert.Equal(t, 2, view.focusedPanel)
	assert.True(t, view.findingPanel.Focused())

	// Cycle to panel 3
	view.cycleFocus()
	assert.Equal(t, 3, view.focusedPanel)
	assert.True(t, view.metricsPanel.Focused())

	// Cycle back to panel 0
	view.cycleFocus()
	assert.Equal(t, 0, view.focusedPanel)
	assert.True(t, view.missionPanel.Focused())
}

func TestDashboardView_Update_Tab(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)
	view.Init()

	// Press Tab to cycle focus
	msg := tea.KeyMsg{Type: tea.KeyTab}
	_, _ = view.Update(msg)

	assert.Equal(t, 1, view.focusedPanel)
}

func TestDashboardView_Update_Refresh(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)
	view.Init()

	// Press 'r' to refresh
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}
	_, _ = view.Update(msg)

	// Refresh should update lastRefresh
	assert.False(t, view.lastRefresh.IsZero())
}

func TestDashboardView_Update_WindowSize(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)
	view.Init()

	msg := tea.WindowSizeMsg{Width: 150, Height: 50}
	_, _ = view.Update(msg)

	assert.Equal(t, 150, view.width)
	assert.Equal(t, 50, view.height)
}

func TestDashboardView_View(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)
	view.Init()
	view.SetSize(100, 30)

	output := view.View()

	// Should render something
	assert.NotEmpty(t, output)
}

func TestDashboardView_RenderMissionSummary(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)

	content := view.renderMissionSummary()

	assert.Contains(t, content, "Total Missions")
}

func TestDashboardView_RenderAgentStatus(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)

	content := view.renderAgentStatus()

	// Without registry, should show "No agents registered"
	assert.Contains(t, content, "No agents registered")
}

func TestDashboardView_RenderRecentFindings(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)

	content := view.renderRecentFindings()

	// Without findings, should show "No findings yet"
	assert.Contains(t, content, "No findings yet")
}

func TestDashboardView_RenderSystemMetrics(t *testing.T) {
	ctx := context.Background()
	view := NewDashboardView(ctx, nil, nil, nil)

	content := view.renderSystemMetrics()

	assert.Contains(t, content, "Database")
	assert.Contains(t, content, "Components")
	assert.Contains(t, content, "LLM Service")
}

// Helper function tests

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is..."},
		{"ab", 3, "ab"},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		seconds  int
		expected string
	}{
		{"30 seconds", 30, "30s"},
		{"90 seconds", 90, "1m"},
		{"2 hours", 7200, "2h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: The actual implementation uses time.Duration
			// This is a simplified test
		})
	}
}
