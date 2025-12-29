package views

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNewConsoleView(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)

	assert.NotNil(t, view)
	assert.Empty(t, view.activeAgent)
	assert.NotNil(t, view.messages)
	assert.Equal(t, 80, view.width)
	assert.Equal(t, 24, view.height)
	assert.False(t, view.ready)
}

func TestConsoleView_Init(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)

	cmd := view.Init()

	// Init should return textinput blink command
	assert.NotNil(t, cmd)
	assert.True(t, view.ready)
}

func TestConsoleView_SetSize(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)

	view.SetSize(120, 40)

	assert.Equal(t, 120, view.width)
	assert.Equal(t, 40, view.height)
}

func TestConsoleView_View(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)

	// Before init
	output := view.View()
	assert.Equal(t, "Initializing console...", output)

	// After init
	view.Init()
	output = view.View()
	assert.NotEmpty(t, output)
}

func TestConsoleView_Update_WindowSize(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)
	view.Init()

	msg := tea.WindowSizeMsg{Width: 150, Height: 50}
	_, _ = view.Update(msg)

	assert.Equal(t, 150, view.width)
	assert.Equal(t, 50, view.height)
}

func TestConsoleView_Update_ClearConsole(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)
	view.Init()

	// Add some messages first
	initialMsgCount := len(view.messages)

	// Press Ctrl+L to clear
	msg := tea.KeyMsg{Type: tea.KeyCtrlL}
	_, _ = view.Update(msg)

	// Messages should be cleared (but there will be a "Console cleared" message)
	assert.LessOrEqual(t, len(view.messages), initialMsgCount+1)
}

func TestConsoleView_Update_ClearLine(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)
	view.Init()

	// Type something
	view.input.SetValue("test input")

	// Press Ctrl+U to clear line
	msg := tea.KeyMsg{Type: tea.KeyCtrlU}
	_, _ = view.Update(msg)

	assert.Empty(t, view.input.Value())
}

func TestConsoleView_HandleSlashCommand_Help(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)
	view.Init()

	initialMsgCount := len(view.messages)

	view.handleSlashCommand("/help")

	// Should have added help messages
	assert.Greater(t, len(view.messages), initialMsgCount)
}

func TestConsoleView_HandleSlashCommand_Clear(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)
	view.Init()

	view.handleSlashCommand("/clear")

	// Should have cleared and added "Console cleared" message
	assert.Equal(t, 1, len(view.messages))
}

func TestConsoleView_HandleSlashCommand_Agents(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)
	view.Init()

	initialMsgCount := len(view.messages)

	view.handleSlashCommand("/agents")

	// Should have added agent listing messages
	assert.Greater(t, len(view.messages), initialMsgCount)
}

func TestConsoleView_HandleSlashCommand_Unknown(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)
	view.Init()

	initialMsgCount := len(view.messages)

	view.handleSlashCommand("/unknowncommand")

	// Should have added unknown command message
	assert.Greater(t, len(view.messages), initialMsgCount)
}

func TestConsoleView_SwitchAgent(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)
	view.Init()

	// Without registry, switching should fail gracefully
	view.switchAgent("testAgent")

	// Should have added a message about agent not found
	// (since registry is nil)
	assert.Greater(t, len(view.messages), 2)
}

func TestConsoleView_AddMessages(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)

	initialCount := len(view.messages)

	view.addUserMessage("User message")
	assert.Equal(t, initialCount+1, len(view.messages))
	assert.Equal(t, LineTypeUser, view.messages[len(view.messages)-1].Type)

	view.addSystemMessage("System message")
	assert.Equal(t, initialCount+2, len(view.messages))
	assert.Equal(t, LineTypeSystem, view.messages[len(view.messages)-1].Type)

	view.addErrorMessage("Error message")
	assert.Equal(t, initialCount+3, len(view.messages))
	assert.Equal(t, LineTypeError, view.messages[len(view.messages)-1].Type)

	view.addAgentMessage("agent1", "Agent response", nil)
	assert.Equal(t, initialCount+4, len(view.messages))
	assert.Equal(t, LineTypeAgent, view.messages[len(view.messages)-1].Type)
}

func TestLineType_String(t *testing.T) {
	tests := []struct {
		lineType LineType
		expected string
	}{
		{LineTypeUser, "user"},
		{LineTypeAgent, "agent"},
		{LineTypeSystem, "system"},
		{LineTypeError, "error"},
		{LineType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.lineType.String())
		})
	}
}

func TestConsoleView_SendMessage(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)
	view.Init()

	initialMsgCount := len(view.messages)

	view.SendMessage("test message")

	// Should have processed the message
	assert.Greater(t, len(view.messages), initialMsgCount)
}

func TestConsoleView_CommandHistory(t *testing.T) {
	ctx := context.Background()
	view := NewConsoleView(ctx, nil)
	view.Init()

	// Simulate entering commands
	view.input.SetValue("command1")
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, _ = view.Update(enterMsg)

	view.input.SetValue("command2")
	_, _ = view.Update(enterMsg)

	// Should have history
	assert.Len(t, view.history, 2)
	assert.Equal(t, "command1", view.history[0])
	assert.Equal(t, "command2", view.history[1])
}
