package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/plan"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// ButtonState represents which button is currently focused.
type ButtonState int

const (
	// ButtonApprove indicates the Approve button is focused.
	ButtonApprove ButtonState = iota
	// ButtonDeny indicates the Deny button is focused.
	ButtonDeny
	// ButtonComments indicates the comments input field is focused.
	ButtonComments
)

// ApprovalDialog displays a modal dialog for approving or denying execution plans.
// It shows plan steps with risk levels and allows the user to add comments.
type ApprovalDialog struct {
	plan          *plan.ExecutionPlan
	visible       bool
	focusedField  ButtonState
	commentsInput textinput.Model
	width         int
	height        int
	theme         *styles.Theme
}

// NewApprovalDialog creates a new approval dialog for the given plan.
func NewApprovalDialog(executionPlan *plan.ExecutionPlan) *ApprovalDialog {
	ti := textinput.New()
	ti.Placeholder = "Enter comments (optional)..."
	ti.CharLimit = 500
	ti.Width = 60

	return &ApprovalDialog{
		plan:          executionPlan,
		visible:       false,
		focusedField:  ButtonApprove,
		commentsInput: ti,
		width:         80,
		height:        30,
		theme:         styles.DefaultTheme(),
	}
}

// Show makes the dialog visible and sets focus on comments input.
func (d *ApprovalDialog) Show() {
	d.visible = true
	d.focusedField = ButtonComments
	d.commentsInput.Focus()
}

// Hide makes the dialog invisible.
func (d *ApprovalDialog) Hide() {
	d.visible = false
	d.commentsInput.Blur()
	d.commentsInput.SetValue("") // Clear comments when hiding
}

// IsVisible returns whether the dialog is currently visible.
func (d *ApprovalDialog) IsVisible() bool {
	return d.visible
}

// SetPlan sets the execution plan to display.
func (d *ApprovalDialog) SetPlan(executionPlan *plan.ExecutionPlan) {
	d.plan = executionPlan
}

// SetSize sets the dimensions for the dialog.
func (d *ApprovalDialog) SetSize(width, height int) {
	if width > 0 {
		d.width = width
	}
	if height > 0 {
		d.height = height
	}
	// Update text input width
	if width > 20 {
		d.commentsInput.Width = width - 20
	}
}

// SetTheme sets the theme for the dialog.
func (d *ApprovalDialog) SetTheme(theme *styles.Theme) {
	if theme != nil {
		d.theme = theme
	}
}

// FocusNext moves focus to the next field (Tab key behavior).
func (d *ApprovalDialog) FocusNext() {
	switch d.focusedField {
	case ButtonComments:
		d.commentsInput.Blur()
		d.focusedField = ButtonApprove
	case ButtonApprove:
		d.focusedField = ButtonDeny
	case ButtonDeny:
		d.focusedField = ButtonComments
		d.commentsInput.Focus()
	}
}

// FocusPrev moves focus to the previous field (Shift+Tab key behavior).
func (d *ApprovalDialog) FocusPrev() {
	switch d.focusedField {
	case ButtonComments:
		d.commentsInput.Blur()
		d.focusedField = ButtonDeny
	case ButtonDeny:
		d.focusedField = ButtonApprove
	case ButtonApprove:
		d.focusedField = ButtonComments
		d.commentsInput.Focus()
	}
}

// GetFocusedButton returns the currently focused button.
func (d *ApprovalDialog) GetFocusedButton() ButtonState {
	return d.focusedField
}

// GetComments returns the text entered in the comments field.
func (d *ApprovalDialog) GetComments() string {
	return d.commentsInput.Value()
}

// UpdateTextInput updates the text input with a message (for keyboard input).
func (d *ApprovalDialog) UpdateTextInput(msg textinput.Model) {
	d.commentsInput = msg
}

// GetTextInput returns the text input model for external updates.
func (d *ApprovalDialog) GetTextInput() *textinput.Model {
	return &d.commentsInput
}

// Render renders the approval dialog as a centered modal overlay.
// Returns an empty string if the dialog is not visible.
func (d *ApprovalDialog) Render() string {
	if !d.visible || d.plan == nil {
		return ""
	}

	// Calculate dialog dimensions (70% of screen)
	dialogWidth := d.width * 7 / 10
	dialogHeight := d.height * 7 / 10

	if dialogWidth < 60 {
		dialogWidth = 60
	}
	if dialogHeight < 20 {
		dialogHeight = 20
	}

	var content strings.Builder

	// Title
	title := d.theme.TitleStyle.Render("Approval Required")
	content.WriteString(title)
	content.WriteString("\n\n")

	// Plan information
	planInfo := fmt.Sprintf("Plan: %s\nAgent: %s\n", d.plan.ID, d.plan.AgentName)
	content.WriteString(planInfo)
	content.WriteString(strings.Repeat("─", dialogWidth-10))
	content.WriteString("\n\n")

	// Steps section
	content.WriteString(d.theme.TitleStyle.Copy().Underline(true).Render("Execution Steps:"))
	content.WriteString("\n")

	// Render steps with risk levels
	maxSteps := 8 // Limit displayed steps to fit dialog
	stepsToShow := d.plan.Steps
	if len(stepsToShow) > maxSteps {
		stepsToShow = stepsToShow[:maxSteps]
	}

	for i, step := range stepsToShow {
		stepLine := d.renderStep(i+1, &step)
		content.WriteString(stepLine)
		content.WriteString("\n")
	}

	if len(d.plan.Steps) > maxSteps {
		content.WriteString(fmt.Sprintf("  ... and %d more steps\n", len(d.plan.Steps)-maxSteps))
	}

	content.WriteString("\n")

	// Risk summary
	if d.plan.RiskSummary != nil {
		riskInfo := fmt.Sprintf("Overall Risk: %s",
			d.theme.SeverityStyle(string(d.plan.RiskSummary.OverallLevel)).Render(string(d.plan.RiskSummary.OverallLevel)))
		content.WriteString(riskInfo)
		content.WriteString("\n\n")
	}

	// Comments input
	content.WriteString(d.theme.TitleStyle.Render("Comments:"))
	content.WriteString("\n")
	content.WriteString(d.commentsInput.View())
	content.WriteString("\n\n")

	// Buttons
	content.WriteString(d.renderButtons())
	content.WriteString("\n\n")

	// Instructions
	instructionStyle := d.theme.SeverityInfo
	instructions := instructionStyle.Render("Tab: Navigate | Enter: Select | Esc: Cancel")
	content.WriteString(instructions)

	// Wrap content in a bordered box
	contentStr := content.String()
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(d.theme.Primary).
		Padding(1, 2).
		Width(dialogWidth - 6).
		Height(dialogHeight - 4)

	dialogBox := dialogStyle.Render(contentStr)

	// Center the dialog
	centeredStyle := lipgloss.NewStyle().
		Width(d.width).
		Height(d.height).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center)

	return centeredStyle.Render(dialogBox)
}

// renderStep renders a single execution step with risk highlighting.
func (d *ApprovalDialog) renderStep(num int, step *plan.ExecutionStep) string {
	// Format step number
	numStr := fmt.Sprintf("%2d. ", num)

	// Format step name
	stepName := step.Name
	if len(stepName) > 40 {
		stepName = stepName[:37] + "..."
	}

	// Get risk style
	riskStyle := d.theme.SeverityStyle(string(step.RiskLevel))

	// Highlight high/critical risk steps
	var prefix string
	if step.RiskLevel.IsHighRisk() {
		prefix = "⚠ "
	} else {
		prefix = "  "
	}

	// Format the line
	riskBadge := riskStyle.Render(fmt.Sprintf("[%s]", step.RiskLevel))
	line := fmt.Sprintf("%s%s%-40s %s", prefix, numStr, stepName, riskBadge)

	return line
}

// renderButtons renders the Approve and Deny buttons with focus indication.
func (d *ApprovalDialog) renderButtons() string {
	// Define button styles
	activeStyle := lipgloss.NewStyle().
		Background(d.theme.Primary).
		Foreground(lipgloss.Color("#000000")).
		Bold(true).
		Padding(0, 2)

	inactiveStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(d.theme.Muted).
		Padding(0, 2)

	// Render Approve button
	var approveBtn string
	if d.focusedField == ButtonApprove {
		approveBtn = activeStyle.Render("[ Approve ]")
	} else {
		approveBtn = inactiveStyle.Render("  Approve  ")
	}

	// Render Deny button
	var denyBtn string
	if d.focusedField == ButtonDeny {
		denyBtn = activeStyle.Render("[  Deny  ]")
	} else {
		denyBtn = inactiveStyle.Render("   Deny   ")
	}

	// Combine buttons with spacing
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, approveBtn, "  ", denyBtn)

	return buttons
}
