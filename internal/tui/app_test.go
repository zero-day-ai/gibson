package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewApp(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)

	assert.NotNil(t, app)
	assert.Equal(t, ModeDashboard, app.mode)
	assert.NotNil(t, app.statusBar)
	assert.NotNil(t, app.helpOverlay)
	assert.Equal(t, 80, app.width)
	assert.Equal(t, 24, app.height)
	assert.False(t, app.showHelp)
	assert.False(t, app.ready)
}

func TestApp_Init(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)
	cmd := app.Init()

	// Init should return a batch command (tick + view inits)
	assert.NotNil(t, cmd)
	assert.True(t, app.ready)
}

func TestApp_ModeSwitching(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)
	_ = app.Init()

	tests := []struct {
		name     string
		keyType  tea.KeyType
		expected AppMode
	}{
		{"switch to dashboard", tea.KeyF1, ModeDashboard},
		{"switch to console", tea.KeyF2, ModeConsole},
		{"switch to mission", tea.KeyF3, ModeMission},
		{"switch to findings", tea.KeyF4, ModeFindings},
		{"back to dashboard", tea.KeyF1, ModeDashboard},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tea.KeyMsg{Type: tt.keyType}
			_, _ = app.Update(msg)
			assert.Equal(t, tt.expected, app.mode)
		})
	}
}

func TestApp_HelpToggle(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)
	_ = app.Init()

	// Initially help should be hidden
	assert.False(t, app.showHelp)

	// Press ? to show help
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")}
	_, _ = app.Update(msg)
	assert.True(t, app.showHelp)

	// Press ? again to hide help
	_, _ = app.Update(msg)
	assert.False(t, app.showHelp)

	// Show help again
	_, _ = app.Update(msg)
	assert.True(t, app.showHelp)

	// Press Escape to hide help
	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	_, _ = app.Update(escMsg)
	assert.False(t, app.showHelp)
}

func TestApp_WindowResize(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)
	_ = app.Init()

	// Initial size
	assert.Equal(t, 80, app.width)
	assert.Equal(t, 24, app.height)

	// Resize
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	_, _ = app.Update(msg)

	assert.Equal(t, 120, app.width)
	assert.Equal(t, 40, app.height)
}

func TestApp_View(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)
	_ = app.Init()

	// Get the view output
	view := app.View()

	// View should return some content
	assert.NotEmpty(t, view)
}

func TestApp_QuitCommand(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)
	_ = app.Init()

	// Press q to quit
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
	_, cmd := app.Update(msg)

	// Should return tea.Quit command
	require.NotNil(t, cmd)
}

func TestApp_GetMode(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)
	assert.Equal(t, ModeDashboard, app.GetMode())

	app.setMode(ModeConsole)
	assert.Equal(t, ModeConsole, app.GetMode())
}

func TestApp_SetStatusMessage(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)

	// This should not panic
	app.SetStatusMessage("Test message")
	app.SetStatusError("Test error")
}

func TestApp_TickMessage(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)
	_ = app.Init()

	// Send tick message
	msg := TickMsg{}
	_, cmd := app.Update(msg)

	// Should return a new tick command
	assert.NotNil(t, cmd)
}

func TestApp_ErrorMessage(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)
	_ = app.Init()

	// Send error message
	msg := ErrorMsg{Message: "test error"}
	_, _ = app.Update(msg)

	// Should not panic - error is shown in status bar
}

func TestApp_SwitchModeMessage(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)
	_ = app.Init()

	// Send switch mode message
	msg := SwitchModeMsg{Mode: ModeFindings}
	_, _ = app.Update(msg)

	assert.Equal(t, ModeFindings, app.mode)
}

func TestApp_SetMode(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)

	// Set to same mode should not change anything
	initialMode := app.mode
	app.setMode(initialMode)
	assert.Equal(t, initialMode, app.mode)

	// Set to different mode
	app.setMode(ModeConsole)
	assert.Equal(t, ModeConsole, app.mode)
}

func TestApp_RenderPlaceholder(t *testing.T) {
	ctx := context.Background()
	config := AppConfig{}

	app := NewApp(ctx, config)

	placeholder := app.renderPlaceholder("Test View")
	assert.Contains(t, placeholder, "Test View")
	assert.Contains(t, placeholder, "not available")
}
