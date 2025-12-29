package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/plan"
	"github.com/zero-day-ai/gibson/internal/tui/components"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
	"github.com/zero-day-ai/gibson/internal/tui/views"
)

// App is the main TUI application model that coordinates all views.
// It handles global navigation, mode switching, and message routing.
type App struct {
	ctx context.Context

	// Current mode/view
	mode AppMode

	// Dimensions
	width  int
	height int

	// Views
	dashboardView *views.DashboardView
	consoleView   *views.ConsoleView
	missionView   *views.MissionView
	findingsView  *views.FindingsView

	// Components
	statusBar      *components.StatusBar
	helpOverlay    *components.HelpOverlay
	approvalDialog *components.ApprovalDialog

	// State
	showHelp        bool
	pendingApproval *plan.ExecutionPlan
	keyMap          KeyMap
	theme           *styles.Theme

	// Dependencies (stored for view initialization)
	db                *database.DB
	missionDAO        database.MissionDAO
	componentRegistry component.ComponentRegistry
	findingStore      finding.FindingStore
	agentRegistry     agent.AgentRegistry

	// Ready state
	ready bool
}

// AppConfig contains configuration options for creating a new App.
type AppConfig struct {
	DB                *database.DB
	MissionDAO        database.MissionDAO
	ComponentRegistry component.ComponentRegistry
	FindingStore      finding.FindingStore
	AgentRegistry     agent.AgentRegistry
}

// NewApp creates a new TUI application with the given context and configuration.
func NewApp(ctx context.Context, config AppConfig) *App {
	theme := styles.DefaultTheme()
	keyMap := DefaultKeyMap()

	app := &App{
		ctx:               ctx,
		mode:              ModeDashboard,
		width:             80,
		height:            24,
		keyMap:            keyMap,
		theme:             theme,
		showHelp:          false,
		ready:             false,
		db:                config.DB,
		missionDAO:        config.MissionDAO,
		componentRegistry: config.ComponentRegistry,
		findingStore:      config.FindingStore,
		agentRegistry:     config.AgentRegistry,
	}

	// Initialize status bar
	app.statusBar = components.NewStatusBar(app.width)
	app.statusBar.SetMode(app.mode.String())
	app.statusBar.SetMessage("Welcome to Gibson TUI")
	app.statusBar.SetKeyHints("? help | 1-4 views | q quit")

	// Initialize help overlay
	app.helpOverlay = components.NewHelpOverlay(keyMap.HelpText())
	app.helpOverlay.SetSize(app.width, app.height)

	// Initialize views
	app.initViews()

	return app
}

// initViews initializes all view models with their dependencies.
func (a *App) initViews() {
	// Dashboard view
	a.dashboardView = views.NewDashboardView(
		a.ctx,
		a.db,
		a.componentRegistry,
		a.findingStore,
	)

	// Console view
	a.consoleView = views.NewConsoleView(a.ctx, a.agentRegistry)

	// Mission view
	if a.missionDAO != nil {
		a.missionView = views.NewMissionView(a.ctx, a.missionDAO)
	}

	// Findings view
	if a.findingStore != nil {
		a.findingsView = views.NewFindingsView(a.ctx, a.findingStore)
	}
}

// Init initializes all child views and returns the initial command.
func (a *App) Init() tea.Cmd {
	var cmds []tea.Cmd

	// Initialize all views
	if a.dashboardView != nil {
		cmds = append(cmds, a.dashboardView.Init())
	}
	if a.consoleView != nil {
		cmds = append(cmds, a.consoleView.Init())
	}
	if a.missionView != nil {
		cmds = append(cmds, a.missionView.Init())
	}
	if a.findingsView != nil {
		cmds = append(cmds, a.findingsView.Init())
	}

	// Start periodic tick for updates
	cmds = append(cmds, tickCmd())

	a.ready = true
	return tea.Batch(cmds...)
}

// Update handles messages and routes them to the appropriate handler.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return a.handleWindowResize(msg)

	case tea.KeyMsg:
		// Handle approval dialog first if visible
		if a.approvalDialog != nil && a.approvalDialog.IsVisible() {
			return a.handleApprovalDialogKey(msg)
		}

		// Handle help overlay toggle
		if a.showHelp {
			return a.handleHelpKey(msg)
		}

		// Handle global keys
		cmd := a.handleGlobalKey(msg)
		if cmd != nil {
			return a, cmd
		}

		// Route to active view
		return a.routeToActiveView(msg)

	case TickMsg:
		// Handle periodic tick
		cmds = append(cmds, tickCmd())
		// Could refresh data here if needed
		return a, tea.Batch(cmds...)

	case SwitchModeMsg:
		a.setMode(msg.Mode)
		return a, nil

	case ApprovalRequestMsg:
		return a.handleApprovalRequest(msg)

	case ApprovalDecisionMsg:
		return a.handleApprovalDecision(msg)

	case ErrorMsg:
		a.statusBar.SetError(msg.Message)
		return a, nil

	default:
		// Route other messages to active view
		return a.routeToActiveView(msg)
	}
}

// View renders the current state of the application.
func (a *App) View() string {
	if !a.ready {
		return "Initializing Gibson TUI..."
	}

	// Render the active view
	var viewContent string
	switch a.mode {
	case ModeDashboard:
		if a.dashboardView != nil {
			viewContent = a.dashboardView.View()
		} else {
			viewContent = a.renderPlaceholder("Dashboard")
		}
	case ModeConsole:
		if a.consoleView != nil {
			viewContent = a.consoleView.View()
		} else {
			viewContent = a.renderPlaceholder("Console")
		}
	case ModeMission:
		if a.missionView != nil {
			viewContent = a.missionView.View()
		} else {
			viewContent = a.renderPlaceholder("Mission")
		}
	case ModeFindings:
		if a.findingsView != nil {
			viewContent = a.findingsView.View()
		} else {
			viewContent = a.renderPlaceholder("Findings")
		}
	}

	// Render status bar
	statusBarContent := a.statusBar.Render()

	// Combine view and status bar
	mainView := lipgloss.JoinVertical(
		lipgloss.Left,
		viewContent,
		statusBarContent,
	)

	// Overlay help if visible
	if a.showHelp {
		helpContent := a.helpOverlay.Render()
		if helpContent != "" {
			// Overlay the help on top of the main view
			mainView = a.overlayContent(mainView, helpContent)
		}
	}

	// Overlay approval dialog if visible
	if a.approvalDialog != nil && a.approvalDialog.IsVisible() {
		dialogContent := a.approvalDialog.Render()
		if dialogContent != "" {
			mainView = a.overlayContent(mainView, dialogContent)
		}
	}

	return mainView
}

// handleWindowResize handles terminal resize events.
func (a *App) handleWindowResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	a.width = msg.Width
	a.height = msg.Height

	// Update status bar width
	a.statusBar.SetWidth(a.width)

	// Update help overlay size
	a.helpOverlay.SetSize(a.width, a.height)

	// Update approval dialog size if present
	if a.approvalDialog != nil {
		a.approvalDialog.SetSize(a.width, a.height)
	}

	// Calculate view dimensions (reserve 1 line for status bar)
	viewHeight := a.height - 1
	if viewHeight < 1 {
		viewHeight = 1
	}

	// Update all view dimensions
	if a.dashboardView != nil {
		a.dashboardView.SetSize(a.width, viewHeight)
	}
	if a.consoleView != nil {
		a.consoleView.SetSize(a.width, viewHeight)
	}
	if a.missionView != nil {
		// Mission view handles its own resize through Update
	}
	if a.findingsView != nil {
		a.findingsView.SetSize(a.width, viewHeight)
	}

	// Propagate resize to active view
	var cmd tea.Cmd
	switch a.mode {
	case ModeDashboard:
		if a.dashboardView != nil {
			_, cmd = a.dashboardView.Update(msg)
		}
	case ModeConsole:
		if a.consoleView != nil {
			_, cmd = a.consoleView.Update(msg)
		}
	case ModeMission:
		if a.missionView != nil {
			_, cmd = a.missionView.Update(msg)
		}
	case ModeFindings:
		if a.findingsView != nil {
			_, cmd = a.findingsView.Update(msg)
		}
	}

	return a, cmd
}

// handleGlobalKey handles global key bindings that work in all views.
// Returns a command if the key was handled, nil otherwise.
func (a *App) handleGlobalKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, a.keyMap.Quit):
		return tea.Quit

	case key.Matches(msg, a.keyMap.Help):
		a.showHelp = !a.showHelp
		return nil

	case key.Matches(msg, a.keyMap.Escape):
		// Cancel any dialogs or overlays
		if a.showHelp {
			a.showHelp = false
			return nil
		}
		if a.approvalDialog != nil && a.approvalDialog.IsVisible() {
			a.approvalDialog.Hide()
			return nil
		}
		return nil

	case key.Matches(msg, a.keyMap.ViewDashboard):
		a.setMode(ModeDashboard)
		return nil

	case key.Matches(msg, a.keyMap.ViewConsole):
		a.setMode(ModeConsole)
		return nil

	case key.Matches(msg, a.keyMap.ViewMission):
		a.setMode(ModeMission)
		return nil

	case key.Matches(msg, a.keyMap.ViewFindings):
		a.setMode(ModeFindings)
		return nil
	}

	return nil
}

// handleHelpKey handles keys when help overlay is visible.
func (a *App) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc", "q":
		a.showHelp = false
	}
	return a, nil
}

// handleApprovalDialogKey handles keys when approval dialog is visible.
func (a *App) handleApprovalDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.approvalDialog.Hide()
		return a, nil

	case "tab":
		a.approvalDialog.FocusNext()
		return a, nil

	case "shift+tab":
		a.approvalDialog.FocusPrev()
		return a, nil

	case "enter":
		// Submit based on focused button
		focusedBtn := a.approvalDialog.GetFocusedButton()
		comments := a.approvalDialog.GetComments()

		var approved bool
		switch focusedBtn {
		case components.ButtonApprove:
			approved = true
		case components.ButtonDeny:
			approved = false
		case components.ButtonComments:
			// If focused on comments, move to approve button
			a.approvalDialog.FocusNext()
			return a, nil
		}

		// Create approval decision message
		decision := ApprovalDecisionMsg{
			RequestID: a.pendingApproval.ID.String(),
			Approved:  approved,
			Comments:  comments,
		}

		a.approvalDialog.Hide()
		return a, func() tea.Msg { return decision }

	default:
		// Update text input if focused on comments
		if a.approvalDialog.GetFocusedButton() == components.ButtonComments {
			ti := a.approvalDialog.GetTextInput()
			newTi, cmd := ti.Update(msg)
			a.approvalDialog.UpdateTextInput(newTi)
			return a, cmd
		}
	}

	return a, nil
}

// handleApprovalRequest handles incoming approval requests.
func (a *App) handleApprovalRequest(msg ApprovalRequestMsg) (tea.Model, tea.Cmd) {
	// Convert the message plan steps to an ExecutionPlan
	executionPlan := &plan.ExecutionPlan{
		AgentName: "Agent", // Would come from actual data
		Steps:     make([]plan.ExecutionStep, len(msg.Plan)),
	}

	for i, step := range msg.Plan {
		executionPlan.Steps[i] = plan.ExecutionStep{
			Name:        step.Name,
			Description: step.Description,
			RiskLevel:   plan.RiskLevel(step.RiskLevel),
		}
	}

	a.pendingApproval = executionPlan
	a.approvalDialog = components.NewApprovalDialog(executionPlan)
	a.approvalDialog.SetSize(a.width, a.height)
	a.approvalDialog.Show()

	a.statusBar.SetMessage("Action requires approval")
	return a, nil
}

// handleApprovalDecision handles approval decisions.
func (a *App) handleApprovalDecision(msg ApprovalDecisionMsg) (tea.Model, tea.Cmd) {
	if msg.Approved {
		a.statusBar.SetMessage("Action approved")
	} else {
		a.statusBar.SetMessage("Action denied")
	}
	a.pendingApproval = nil
	return a, nil
}

// routeToActiveView routes a message to the currently active view.
func (a *App) routeToActiveView(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch a.mode {
	case ModeDashboard:
		if a.dashboardView != nil {
			_, cmd = a.dashboardView.Update(msg)
		}
	case ModeConsole:
		if a.consoleView != nil {
			_, cmd = a.consoleView.Update(msg)
		}
	case ModeMission:
		if a.missionView != nil {
			_, cmd = a.missionView.Update(msg)
		}
	case ModeFindings:
		if a.findingsView != nil {
			newView, c := a.findingsView.Update(msg)
			a.findingsView = newView
			cmd = c
		}
	}

	return a, cmd
}

// setMode changes the current view mode and updates the status bar.
func (a *App) setMode(mode AppMode) {
	if a.mode == mode {
		return
	}
	a.mode = mode
	a.statusBar.SetMode(mode.String())
	a.statusBar.SetMessage("Switched to " + mode.String() + " view")
}

// renderPlaceholder renders a placeholder for views that aren't initialized.
func (a *App) renderPlaceholder(viewName string) string {
	style := lipgloss.NewStyle().
		Width(a.width).
		Height(a.height-1).
		Align(lipgloss.Center, lipgloss.Center).
		Foreground(a.theme.Muted)

	return style.Render(viewName + " view not available\n\nDependencies not configured")
}

// overlayContent overlays content on top of the background.
// This is a simple implementation that replaces the center of the view.
func (a *App) overlayContent(background, overlay string) string {
	// For a proper overlay, we'd need to do character-by-character replacement.
	// For now, we'll just return the overlay since it's centered and full-screen.
	// In a production implementation, you'd use a proper layering library.
	return overlay
}

// GetMode returns the current application mode.
func (a *App) GetMode() AppMode {
	return a.mode
}

// SetStatusMessage sets a message in the status bar.
func (a *App) SetStatusMessage(message string) {
	a.statusBar.SetMessage(message)
}

// SetStatusError sets an error message in the status bar.
func (a *App) SetStatusError(message string) {
	a.statusBar.SetError(message)
}

// ShowApprovalDialog displays an approval dialog for the given plan.
func (a *App) ShowApprovalDialog(executionPlan *plan.ExecutionPlan) {
	a.pendingApproval = executionPlan
	a.approvalDialog = components.NewApprovalDialog(executionPlan)
	a.approvalDialog.SetSize(a.width, a.height)
	a.approvalDialog.Show()
}
