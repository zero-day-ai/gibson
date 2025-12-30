package views

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/tui/console"
)

func TestNewConsoleView(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)

	assert.NotNil(t, view)
	assert.NotNil(t, view.ctx)
	assert.NotNil(t, view.input)
	assert.NotNil(t, view.output)
	assert.NotNil(t, view.theme)
	assert.NotNil(t, view.history)
	assert.NotNil(t, view.executor)
	assert.NotNil(t, view.completer)
	assert.NotNil(t, view.registry)
	assert.Equal(t, 80, view.width)
	assert.Equal(t, 24, view.height)
	assert.Equal(t, DefaultMaxLines, view.maxLines)
	assert.False(t, view.showSuggestions)
	assert.Equal(t, "", view.lastInput)
	assert.False(t, view.historyNavigating)

	// Check welcome message was added
	assert.Greater(t, len(view.outputLines), 0)
	assert.Contains(t, view.outputLines[0].Text, "Gibson Console")
}

func TestConsoleViewInit(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)

	cmd := view.Init()

	// Init should return textinput.Blink command
	assert.NotNil(t, cmd)

	// Input should NOT be focused after Init (focus is managed by Focus()/Blur())
	assert.False(t, view.input.Focused())
}

func TestConsoleViewFocusBlur(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)

	// Initially not focused
	assert.False(t, view.input.Focused())

	// Focus should set input focus
	cmd := view.Focus()
	assert.NotNil(t, cmd)
	assert.True(t, view.input.Focused())

	// Blur should remove input focus
	view.Blur()
	assert.False(t, view.input.Focused())
}

func TestConsoleViewSetSize(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)

	view.SetSize(120, 40)

	assert.Equal(t, 120, view.width)
	assert.Equal(t, 40, view.height)

	// Verify child components were sized
	assert.Greater(t, view.input.Width, 0)
	assert.Greater(t, view.output.Width, 0)
	assert.Greater(t, view.output.Height, 0)
}

func TestConsoleViewUpdate_WindowSize(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	msg := tea.WindowSizeMsg{Width: 150, Height: 50}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)
	assert.Equal(t, 150, updatedView.width)
	assert.Equal(t, 50, updatedView.height)
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_Enter(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Set a command in the input
	view.input.SetValue("/help")

	// Record initial output length
	initialOutputCount := len(view.outputLines)

	// Press enter
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Input should be cleared
	assert.Equal(t, "", updatedView.input.Value())

	// Output should have increased (command echo + result)
	assert.Greater(t, len(updatedView.outputLines), initialOutputCount)

	// Should not be navigating history
	assert.False(t, updatedView.historyNavigating)

	// Suggestions should be hidden
	assert.False(t, updatedView.showSuggestions)

	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_Enter_EmptyCommand(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Keep input empty
	initialOutputCount := len(view.outputLines)

	// Press enter
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	model, _ := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Output should not have changed
	assert.Equal(t, initialOutputCount, len(updatedView.outputLines))
}

func TestConsoleViewUpdate_Enter_InvalidCommand(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Set a non-slash command
	view.input.SetValue("not a slash command")

	// Press enter
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	model, _ := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Should have error message about slash commands
	found := false
	for _, line := range updatedView.outputLines {
		if strings.Contains(line.Text, "must start with /") {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected error message about slash commands")
}

func TestConsoleViewUpdate_History_Up(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Add some commands to history
	view.history.Add("/help")
	view.history.Add("/status")
	view.history.Add("/clear")

	// Press up arrow
	msg := tea.KeyMsg{Type: tea.KeyUp}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Should show last command
	assert.Equal(t, "/clear", updatedView.input.Value())
	assert.True(t, updatedView.historyNavigating)
	assert.False(t, updatedView.showSuggestions)
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_History_Down(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Add commands to history
	view.history.Add("/help")
	view.history.Add("/status")

	// Navigate up twice
	view.Update(tea.KeyMsg{Type: tea.KeyUp})
	view.Update(tea.KeyMsg{Type: tea.KeyUp})

	// Now navigate down
	msg := tea.KeyMsg{Type: tea.KeyDown}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Should show next command in history
	assert.Equal(t, "/status", updatedView.input.Value())
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_History_Down_ClearAtEnd(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Add a command to history
	view.history.Add("/help")

	// Navigate up
	view.Update(tea.KeyMsg{Type: tea.KeyUp})

	// Navigate down (past the end)
	msg := tea.KeyMsg{Type: tea.KeyDown}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Input should be cleared when reaching end of history
	assert.Equal(t, "", updatedView.input.Value())
	assert.False(t, updatedView.historyNavigating)
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_Tab_Completion(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Type partial command
	view.input.SetValue("/hel")

	// Press tab
	msg := tea.KeyMsg{Type: tea.KeyTab}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Should either show suggestions or auto-complete
	// (depends on how many suggestions match)
	// At minimum, suggestions should be updated
	assert.NotNil(t, updatedView.suggestions)
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_Tab_CycleSuggestions(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Manually set multiple suggestions
	view.showSuggestions = true
	view.suggestions = []console.Suggestion{
		{Text: "/help"},
		{Text: "/health"},
	}
	view.suggestionIndex = 0

	// Press tab to cycle
	msg := tea.KeyMsg{Type: tea.KeyTab}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Should cycle to next suggestion and apply it
	assert.Contains(t, updatedView.input.Value(), "/")
	assert.False(t, updatedView.showSuggestions)
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_ClearLine(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Set some input
	view.input.SetValue("/help command")
	view.lastInput = "/help command"
	view.showSuggestions = true
	view.historyNavigating = true

	// Press Ctrl+U
	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Input should be cleared
	assert.Equal(t, "", updatedView.input.Value())
	assert.Equal(t, "", updatedView.lastInput)
	assert.False(t, updatedView.showSuggestions)
	assert.False(t, updatedView.historyNavigating)
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_ClearOutput(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Add several output lines
	initialOutputCount := len(view.outputLines)
	assert.Greater(t, initialOutputCount, 0, "Should have welcome message")

	// Press Ctrl+L
	msg := tea.KeyMsg{Type: tea.KeyCtrlL}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Output should be cleared (only showing clear message)
	assert.Less(t, len(updatedView.outputLines), initialOutputCount)

	// Should have a clear indicator message
	found := false
	for _, line := range updatedView.outputLines {
		if strings.Contains(line.Text, "Console cleared") {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected clear indicator message")
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_CtrlC(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Set some input
	view.input.SetValue("/help")
	view.showSuggestions = true

	// Press Ctrl+C
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Input should be cleared
	assert.Equal(t, "", updatedView.input.Value())
	assert.Equal(t, "", updatedView.lastInput)
	assert.False(t, updatedView.showSuggestions)
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_PageUp(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Press Page Up
	msg := tea.KeyMsg{Type: tea.KeyPgUp}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Should not panic (viewport handles scrolling)
	assert.NotNil(t, updatedView)
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_PageDown(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Press Page Down
	msg := tea.KeyMsg{Type: tea.KeyPgDown}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Should not panic (viewport handles scrolling)
	assert.NotNil(t, updatedView)
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_Escape(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Set up state
	view.showSuggestions = true
	view.historyNavigating = true

	// Press Escape
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	model, cmd := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Should hide suggestions and reset history navigation
	assert.False(t, updatedView.showSuggestions)
	assert.False(t, updatedView.historyNavigating)
	assert.Nil(t, cmd)
}

func TestConsoleViewUpdate_InputChange_ShowSuggestions(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Type a slash command to trigger suggestions
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}
	model, _ := view.Update(msg)

	updatedView, ok := model.(*ConsoleView)
	require.True(t, ok)

	// Suggestions should be auto-shown for slash commands
	// (if there are any matching suggestions)
	assert.NotNil(t, updatedView.suggestions)
}

func TestConsoleViewView(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()
	view.SetSize(100, 30)

	output := view.View()

	// Should render something
	assert.NotEmpty(t, output)

	// Should contain prompt
	assert.Contains(t, output, PromptText)
}

func TestConsoleViewView_WithoutSize(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Reset size to 0
	view.width = 0
	view.height = 0

	output := view.View()

	// Should show loading message
	assert.Contains(t, output, "Loading console")
}

func TestConsoleViewView_WithSuggestions(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()
	view.SetSize(100, 30)

	// Set up suggestions
	view.showSuggestions = true
	view.suggestions = []console.Suggestion{
		{Text: "/help", Description: "Show help"},
		{Text: "/status", Description: "Show status"},
	}

	output := view.View()

	// Should render suggestions
	assert.NotEmpty(t, output)
	// The view should be longer with suggestions visible
	assert.Greater(t, len(output), 100)
}

func TestConsoleViewAppendOutput(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	initialCount := len(view.outputLines)

	// Add a line
	view.appendOutput(console.OutputLine{
		Text:  "Test output",
		Style: console.StyleNormal,
	})

	assert.Equal(t, initialCount+1, len(view.outputLines))
	assert.Equal(t, "Test output", view.outputLines[len(view.outputLines)-1].Text)
}

func TestConsoleViewAppendOutput_RingBuffer(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Set a small max lines for testing
	view.maxLines = 5

	// Add more than maxLines
	for i := 0; i < 10; i++ {
		view.appendOutput(console.OutputLine{
			Text:  strings.Repeat("X", i),
			Style: console.StyleNormal,
		})
	}

	// Should be limited to maxLines
	assert.Equal(t, view.maxLines, len(view.outputLines))

	// First line should be from iteration 5 (oldest dropped)
	assert.Equal(t, strings.Repeat("X", 5), view.outputLines[0].Text)
}

func TestConsoleViewClearOutput(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Add several lines
	for i := 0; i < 10; i++ {
		view.appendOutput(console.OutputLine{
			Text:  "Line",
			Style: console.StyleNormal,
		})
	}

	assert.Greater(t, len(view.outputLines), 5)

	// Clear output
	view.clearOutput()

	// Should have only clear message + blank line
	assert.Equal(t, 2, len(view.outputLines))
	assert.Contains(t, view.outputLines[0].Text, "Console cleared")
}

func TestConsoleViewExecuteCommand_SlashCommand(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	initialCount := len(view.outputLines)

	// Execute a command
	view.executeCommand("/help")

	// Should have added output (command echo + results)
	assert.Greater(t, len(view.outputLines), initialCount)

	// Should have command echo
	found := false
	for _, line := range view.outputLines {
		if strings.Contains(line.Text, "gibson> /help") {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected command echo")
}

func TestConsoleViewExecuteCommand_NonSlashCommand(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	initialCount := len(view.outputLines)

	// Execute a non-slash command
	view.executeCommand("regular command")

	// Should have added error message
	assert.Greater(t, len(view.outputLines), initialCount)

	// Should have error about slash commands
	found := false
	for _, line := range view.outputLines {
		if strings.Contains(line.Text, "must start with /") {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected error message")
}

func TestConsoleViewUpdateSuggestions(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Set input
	view.input.SetValue("/help")

	// Update suggestions
	view.updateSuggestions()

	// Should have reset suggestion index
	assert.Equal(t, 0, view.suggestionIndex)

	// Suggestions should be populated (or empty if no matches)
	assert.NotNil(t, view.suggestions)
}

func TestConsoleViewRenderSuggestions_Empty(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Empty suggestions
	view.suggestions = []console.Suggestion{}

	output := view.renderSuggestions()

	// Should return empty string
	assert.Equal(t, "", output)
}

func TestConsoleViewRenderSuggestions_WithSuggestions(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()
	view.SetSize(100, 30)

	// Add suggestions
	view.suggestions = []console.Suggestion{
		{Text: "/help", Description: "Show help"},
		{Text: "/status", Description: "Show status"},
	}
	view.suggestionIndex = 0

	output := view.renderSuggestions()

	// Should render box with suggestions
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "/help")
	assert.Contains(t, output, "/status")
}

func TestConsoleViewHistoryIntegration(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Execute several commands
	view.input.SetValue("/help")
	view.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view.input.SetValue("/status")
	view.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view.input.SetValue("/clear")
	view.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Navigate up to last command
	model, _ := view.Update(tea.KeyMsg{Type: tea.KeyUp})
	updatedView := model.(*ConsoleView)
	assert.Equal(t, "/clear", updatedView.input.Value())

	// Navigate up to second command
	model, _ = updatedView.Update(tea.KeyMsg{Type: tea.KeyUp})
	updatedView = model.(*ConsoleView)
	assert.Equal(t, "/status", updatedView.input.Value())

	// Navigate up to first command
	model, _ = updatedView.Update(tea.KeyMsg{Type: tea.KeyUp})
	updatedView = model.(*ConsoleView)
	assert.Equal(t, "/help", updatedView.input.Value())

	// Navigate down
	model, _ = updatedView.Update(tea.KeyMsg{Type: tea.KeyDown})
	updatedView = model.(*ConsoleView)
	assert.Equal(t, "/status", updatedView.input.Value())
}

func TestConsoleViewMinFunction(t *testing.T) {
	tests := []struct {
		a        int
		b        int
		expected int
	}{
		{5, 10, 5},
		{10, 5, 5},
		{7, 7, 7},
		{0, 1, 0},
		{-1, 1, -1},
	}

	for _, tt := range tests {
		result := min(tt.a, tt.b)
		assert.Equal(t, tt.expected, result)
	}
}

func TestConsoleViewStyleOutputLine(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	tests := []struct {
		name  string
		style console.OutputStyle
		text  string
	}{
		{"Normal", console.StyleNormal, "Normal text"},
		{"Command", console.StyleCommand, "gibson> /help"},
		{"Error", console.StyleError, "Error occurred"},
		{"Success", console.StyleSuccess, "Success!"},
		{"Info", console.StyleInfo, "Information"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := console.OutputLine{
				Text:  tt.text,
				Style: tt.style,
			}

			output := view.styleOutputLine(line)

			// Should not be empty
			assert.NotEmpty(t, output)

			// Should contain the text (possibly with ANSI codes)
			assert.Contains(t, output, tt.text)
		})
	}
}

func TestConsoleViewRefreshOutputContent(t *testing.T) {
	ctx := context.Background()
	config := ConsoleConfig{}
	view := NewConsoleView(ctx, config)
	view.Init()

	// Add some output
	view.appendOutput(console.OutputLine{
		Text:  "Line 1",
		Style: console.StyleNormal,
	})
	view.appendOutput(console.OutputLine{
		Text:  "Line 2",
		Style: console.StyleNormal,
	})

	// Should not panic
	view.refreshOutputContent()

	// Viewport should have content
	assert.NotEmpty(t, view.output.View())
}
