package components

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/stretchr/testify/assert"
	"github.com/zero-day-ai/gibson/internal/plan"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// Panel Tests

func TestNewPanel(t *testing.T) {
	panel := NewPanel("Test Panel")

	assert.NotNil(t, panel)
	assert.Equal(t, "Test Panel", panel.Title())
	assert.False(t, panel.Focused())
	assert.Equal(t, 40, panel.width)
	assert.Equal(t, 10, panel.height)
}

func TestPanel_SetContent(t *testing.T) {
	panel := NewPanel("Test")
	panel.SetContent("Test content")

	assert.Equal(t, "Test content", panel.content)
}

func TestPanel_SetSize(t *testing.T) {
	panel := NewPanel("Test")

	panel.SetSize(100, 50)
	assert.Equal(t, 100, panel.width)
	assert.Equal(t, 50, panel.height)

	// Zero values should be ignored
	panel.SetSize(0, 0)
	assert.Equal(t, 100, panel.width)
	assert.Equal(t, 50, panel.height)

	// Negative values should be ignored
	panel.SetSize(-10, -5)
	assert.Equal(t, 100, panel.width)
	assert.Equal(t, 50, panel.height)
}

func TestPanel_SetFocused(t *testing.T) {
	panel := NewPanel("Test")

	assert.False(t, panel.Focused())

	panel.SetFocused(true)
	assert.True(t, panel.Focused())

	panel.SetFocused(false)
	assert.False(t, panel.Focused())
}

func TestPanel_SetTitle(t *testing.T) {
	panel := NewPanel("Original")
	panel.SetTitle("New Title")

	assert.Equal(t, "New Title", panel.Title())
}

func TestPanel_SetTheme(t *testing.T) {
	panel := NewPanel("Test")
	theme := styles.DefaultTheme()

	panel.SetTheme(theme)
	assert.Equal(t, theme, panel.theme)

	// Nil theme should be ignored
	panel.SetTheme(nil)
	assert.Equal(t, theme, panel.theme)
}

func TestPanel_Render(t *testing.T) {
	panel := NewPanel("Test Panel")
	panel.SetSize(40, 10)
	panel.SetContent("Line 1\nLine 2\nLine 3")

	output := panel.Render()

	// Output should not be empty
	assert.NotEmpty(t, output)

	// Output should contain content (basic check)
	assert.Contains(t, output, "Line 1")
}

func TestPanel_RenderFocused(t *testing.T) {
	panel := NewPanel("Focused Panel")
	panel.SetSize(40, 10)
	panel.SetFocused(true)

	output := panel.Render()
	assert.NotEmpty(t, output)
}

func TestPanel_RenderWithLongContent(t *testing.T) {
	panel := NewPanel("Test")
	panel.SetSize(20, 5)

	// Content longer than panel width
	longLine := "This is a very long line that should be truncated"
	panel.SetContent(longLine)

	output := panel.Render()
	assert.NotEmpty(t, output)
}

func TestPanel_RenderWithManyLines(t *testing.T) {
	panel := NewPanel("Test")
	panel.SetSize(40, 5)

	// More lines than panel height
	manyLines := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8"
	panel.SetContent(manyLines)

	output := panel.Render()
	assert.NotEmpty(t, output)
}

// HelpOverlay Tests

func TestNewHelpOverlay(t *testing.T) {
	keymap := map[string][]key.Binding{
		"Global": {
			key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		},
	}

	overlay := NewHelpOverlay(keymap)

	assert.NotNil(t, overlay)
	assert.False(t, overlay.IsVisible())
	assert.Equal(t, 80, overlay.width)
	assert.Equal(t, 30, overlay.height)
}

func TestHelpOverlay_Visibility(t *testing.T) {
	overlay := NewHelpOverlay(nil)

	assert.False(t, overlay.IsVisible())

	overlay.Show()
	assert.True(t, overlay.IsVisible())

	overlay.Hide()
	assert.False(t, overlay.IsVisible())

	overlay.Toggle()
	assert.True(t, overlay.IsVisible())

	overlay.Toggle()
	assert.False(t, overlay.IsVisible())
}

func TestHelpOverlay_SetSize(t *testing.T) {
	overlay := NewHelpOverlay(nil)

	overlay.SetSize(120, 40)
	assert.Equal(t, 120, overlay.width)
	assert.Equal(t, 40, overlay.height)

	// Zero/negative values should be ignored
	overlay.SetSize(0, 0)
	assert.Equal(t, 120, overlay.width)
	assert.Equal(t, 40, overlay.height)
}

func TestHelpOverlay_SetTheme(t *testing.T) {
	overlay := NewHelpOverlay(nil)
	theme := styles.DefaultTheme()

	overlay.SetTheme(theme)
	assert.Equal(t, theme, overlay.theme)

	overlay.SetTheme(nil)
	assert.Equal(t, theme, overlay.theme) // Should not change
}

func TestHelpOverlay_Render(t *testing.T) {
	keymap := map[string][]key.Binding{
		"Global": {
			key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
			key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		},
		"Navigation": {
			key.NewBinding(key.WithKeys("j"), key.WithHelp("j", "down")),
		},
	}

	overlay := NewHelpOverlay(keymap)
	overlay.SetSize(80, 30)

	// Not visible - should return empty
	output := overlay.Render()
	assert.Empty(t, output)

	// Visible - should return content
	overlay.Show()
	output = overlay.Render()
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "Keyboard Shortcuts")
}

func TestHelpOverlay_Update(t *testing.T) {
	keymap := map[string][]key.Binding{
		"Global": {
			key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		},
	}

	overlay := NewHelpOverlay(keymap)

	// Update with new keymap
	newKeymap := map[string][]key.Binding{
		"Global": {
			key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "exit")),
		},
	}

	overlay.Update(newKeymap)
	assert.Equal(t, newKeymap, overlay.keymap)
}

// StatusBar Tests

func TestNewStatusBar(t *testing.T) {
	sb := NewStatusBar(80)

	assert.NotNil(t, sb)
	assert.Equal(t, 80, sb.width)
	assert.Equal(t, "Dashboard", sb.mode)
	assert.Equal(t, "Ready", sb.message)
	assert.False(t, sb.isError)
}

func TestStatusBar_SetMode(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetMode("Console")

	assert.Equal(t, "Console", sb.mode)
}

func TestStatusBar_SetMessage(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetMessage("Test message")

	assert.Equal(t, "Test message", sb.message)
	assert.False(t, sb.isError)
}

func TestStatusBar_SetError(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetError("Error message")

	assert.Equal(t, "Error message", sb.message)
	assert.True(t, sb.isError)
}

func TestStatusBar_Clear(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetError("Error")
	sb.Clear()

	assert.Empty(t, sb.message)
	assert.False(t, sb.isError)
}

func TestStatusBar_SetWidth(t *testing.T) {
	sb := NewStatusBar(80)

	sb.SetWidth(120)
	assert.Equal(t, 120, sb.width)

	// Zero/negative should be ignored
	sb.SetWidth(0)
	assert.Equal(t, 120, sb.width)
}

func TestStatusBar_SetKeyHints(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetKeyHints("custom hints")

	assert.Equal(t, "custom hints", sb.keyHints)
}

func TestStatusBar_SetTheme(t *testing.T) {
	sb := NewStatusBar(80)
	theme := styles.DefaultTheme()

	sb.SetTheme(theme)
	assert.Equal(t, theme, sb.theme)

	sb.SetTheme(nil)
	assert.Equal(t, theme, sb.theme) // Should not change
}

func TestStatusBar_Render(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetMode("Dashboard")
	sb.SetMessage("Ready")
	sb.SetKeyHints("? help | q quit")

	output := sb.Render()

	assert.NotEmpty(t, output)
}

func TestStatusBar_RenderWithError(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetError("Something went wrong")

	output := sb.Render()
	assert.NotEmpty(t, output)
}

func TestStatusBar_RenderWithLongMessage(t *testing.T) {
	sb := NewStatusBar(60)
	sb.SetMessage("This is a very long message that should be truncated to fit the available space")

	output := sb.Render()
	assert.NotEmpty(t, output)
}

// ApprovalDialog Tests

func TestNewApprovalDialog(t *testing.T) {
	execPlan := &plan.ExecutionPlan{
		AgentName: "test-agent",
		Steps: []plan.ExecutionStep{
			{Name: "Step 1", RiskLevel: "low"},
		},
	}

	dialog := NewApprovalDialog(execPlan)

	assert.NotNil(t, dialog)
	assert.False(t, dialog.IsVisible())
	assert.Equal(t, execPlan, dialog.plan)
	assert.Equal(t, ButtonApprove, dialog.focusedField)
}

func TestApprovalDialog_ShowHide(t *testing.T) {
	dialog := NewApprovalDialog(nil)

	assert.False(t, dialog.IsVisible())

	dialog.Show()
	assert.True(t, dialog.IsVisible())
	assert.Equal(t, ButtonComments, dialog.focusedField) // Show focuses on comments

	dialog.Hide()
	assert.False(t, dialog.IsVisible())
}

func TestApprovalDialog_SetPlan(t *testing.T) {
	dialog := NewApprovalDialog(nil)

	newPlan := &plan.ExecutionPlan{
		AgentName: "new-agent",
	}
	dialog.SetPlan(newPlan)

	assert.Equal(t, newPlan, dialog.plan)
}

func TestApprovalDialog_SetSize(t *testing.T) {
	dialog := NewApprovalDialog(nil)

	dialog.SetSize(100, 50)
	assert.Equal(t, 100, dialog.width)
	assert.Equal(t, 50, dialog.height)

	// Zero/negative should be ignored
	dialog.SetSize(0, 0)
	assert.Equal(t, 100, dialog.width)
	assert.Equal(t, 50, dialog.height)
}

func TestApprovalDialog_FocusNavigation(t *testing.T) {
	dialog := NewApprovalDialog(nil)
	dialog.Show() // Starts focused on comments

	assert.Equal(t, ButtonComments, dialog.GetFocusedButton())

	dialog.FocusNext()
	assert.Equal(t, ButtonApprove, dialog.GetFocusedButton())

	dialog.FocusNext()
	assert.Equal(t, ButtonDeny, dialog.GetFocusedButton())

	dialog.FocusNext()
	assert.Equal(t, ButtonComments, dialog.GetFocusedButton())

	// Test FocusPrev
	dialog.FocusPrev()
	assert.Equal(t, ButtonDeny, dialog.GetFocusedButton())

	dialog.FocusPrev()
	assert.Equal(t, ButtonApprove, dialog.GetFocusedButton())

	dialog.FocusPrev()
	assert.Equal(t, ButtonComments, dialog.GetFocusedButton())
}

func TestApprovalDialog_GetComments(t *testing.T) {
	dialog := NewApprovalDialog(nil)

	// Initially empty
	assert.Empty(t, dialog.GetComments())
}

func TestApprovalDialog_GetTextInput(t *testing.T) {
	dialog := NewApprovalDialog(nil)

	ti := dialog.GetTextInput()
	assert.NotNil(t, ti)
}

func TestApprovalDialog_SetTheme(t *testing.T) {
	dialog := NewApprovalDialog(nil)
	theme := styles.DefaultTheme()

	dialog.SetTheme(theme)
	assert.Equal(t, theme, dialog.theme)

	dialog.SetTheme(nil)
	assert.Equal(t, theme, dialog.theme) // Should not change
}

func TestApprovalDialog_Render(t *testing.T) {
	execPlan := &plan.ExecutionPlan{
		AgentName: "test-agent",
		Steps: []plan.ExecutionStep{
			{Name: "Step 1", RiskLevel: plan.RiskLevelLow},
			{Name: "Step 2", RiskLevel: plan.RiskLevelHigh},
		},
		RiskSummary: &plan.PlanRiskSummary{
			OverallLevel: plan.RiskLevelMedium,
		},
	}

	dialog := NewApprovalDialog(execPlan)
	dialog.SetSize(80, 30)

	// Not visible - should return empty
	output := dialog.Render()
	assert.Empty(t, output)

	// Visible - should return content
	dialog.Show()
	output = dialog.Render()
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "Approval Required")
}

func TestApprovalDialog_RenderWithNoplan(t *testing.T) {
	dialog := NewApprovalDialog(nil)
	dialog.Show()

	output := dialog.Render()
	assert.Empty(t, output) // No plan, should return empty even if visible
}
