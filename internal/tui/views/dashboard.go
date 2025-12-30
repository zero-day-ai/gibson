package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/tui/components"
)

// DashboardView displays an overview of the Gibson system with four panels:
// 1. Mission Summary - shows mission statistics
// 2. Agent Status - shows running agents
// 3. Recent Findings - shows latest security findings
// 4. System Metrics - shows system health and component counts
type DashboardView struct {
	// Panels
	missionPanel *components.Panel
	agentPanel   *components.Panel
	findingPanel *components.Panel
	metricsPanel *components.Panel

	// Focus tracking (0-3 for each panel)
	focusedPanel int

	// Data dependencies
	db                *database.DB
	componentRegistry component.ComponentRegistry
	findingStore      finding.FindingStore

	// Cached data
	missions       []*database.Mission
	components     map[component.ComponentKind][]*component.Component
	recentFindings []finding.EnhancedFinding
	lastRefresh    time.Time

	// Dimensions
	width  int
	height int

	// Context
	ctx context.Context
}

// KeyMap defines the key bindings for the dashboard view
type dashboardKeyMap struct {
	Tab     key.Binding
	Refresh key.Binding
	Quit    key.Binding
}

var defaultDashboardKeys = dashboardKeyMap{
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "cycle focus"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

// NewDashboardView creates a new dashboard view with the given dependencies.
func NewDashboardView(ctx context.Context, db *database.DB, registry component.ComponentRegistry, store finding.FindingStore) *DashboardView {
	return &DashboardView{
		missionPanel:      components.NewPanel("Mission Summary"),
		agentPanel:        components.NewPanel("Agent Status"),
		findingPanel:      components.NewPanel("Recent Findings"),
		metricsPanel:      components.NewPanel("System Metrics"),
		focusedPanel:      0,
		db:                db,
		componentRegistry: registry,
		findingStore:      store,
		missions:          []*database.Mission{},
		components:        make(map[component.ComponentKind][]*component.Component),
		recentFindings:    []finding.EnhancedFinding{},
		ctx:               ctx,
		width:             80,
		height:            24,
	}
}

// Init initializes the dashboard view and loads initial data.
func (d *DashboardView) Init() tea.Cmd {
	// Load initial data
	d.refreshData()

	// Set initial panel sizes
	d.updatePanelSizes()

	// Set initial focus
	d.missionPanel.SetFocused(true)

	return nil
}

// Update handles messages and updates the dashboard state.
func (d *DashboardView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, defaultDashboardKeys.Tab):
			// Cycle panel focus
			d.cycleFocus()
			return d, nil

		case key.Matches(msg, defaultDashboardKeys.Refresh):
			// Refresh all data
			d.refreshData()
			return d, nil

		case key.Matches(msg, defaultDashboardKeys.Quit):
			return d, tea.Quit
		}

	case tea.WindowSizeMsg:
		// Handle window resize
		d.width = msg.Width
		d.height = msg.Height
		d.updatePanelSizes()
		return d, nil
	}

	return d, nil
}

// View renders the dashboard with a 2x2 panel layout.
func (d *DashboardView) View() string {
	// Update panel content before rendering
	d.updatePanelContent()

	// Render all panels
	topRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		d.missionPanel.Render(),
		d.agentPanel.Render(),
	)

	bottomRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		d.findingPanel.Render(),
		d.metricsPanel.Render(),
	)

	dashboard := lipgloss.JoinVertical(
		lipgloss.Left,
		topRow,
		bottomRow,
	)

	return dashboard
}

// SetSize sets the dimensions of the dashboard and updates panel sizes.
func (d *DashboardView) SetSize(width, height int) {
	d.width = width
	d.height = height
	d.updatePanelSizes()
}

// updatePanelSizes calculates and sets the size of each panel based on terminal dimensions.
func (d *DashboardView) updatePanelSizes() {
	// Calculate panel dimensions (2x2 grid)
	panelWidth := d.width / 2
	panelHeight := d.height / 2

	// Set panel sizes
	d.missionPanel.SetSize(panelWidth, panelHeight)
	d.agentPanel.SetSize(panelWidth, panelHeight)
	d.findingPanel.SetSize(panelWidth, panelHeight)
	d.metricsPanel.SetSize(panelWidth, panelHeight)
}

// cycleFocus moves focus to the next panel (0 -> 1 -> 2 -> 3 -> 0).
func (d *DashboardView) cycleFocus() {
	// Clear focus from current panel
	panels := []*components.Panel{
		d.missionPanel,
		d.agentPanel,
		d.findingPanel,
		d.metricsPanel,
	}
	panels[d.focusedPanel].SetFocused(false)

	// Move to next panel
	d.focusedPanel = (d.focusedPanel + 1) % 4

	// Set focus on new panel
	panels[d.focusedPanel].SetFocused(true)
}

// refreshData loads fresh data from all internal packages.
func (d *DashboardView) refreshData() {
	// Load missions from database
	if d.db != nil {
		missionDAO := database.NewMissionDAO(d.db)
		missions, err := missionDAO.List(d.ctx, "")
		if err == nil {
			d.missions = missions
		}
	}

	// Load components from registry
	if d.componentRegistry != nil {
		d.components = d.componentRegistry.ListAll()
	}

	// Load recent findings from store
	if d.findingStore != nil {
		// Get findings from all missions (limited to 10 most recent)
		// Note: This requires iterating through missions, but for now we'll use a simple approach
		// In production, you'd want a dedicated method to get recent findings across all missions
		if len(d.missions) > 0 {
			for _, mission := range d.missions {
				findings, err := d.findingStore.List(d.ctx, mission.ID, nil)
				if err == nil {
					d.recentFindings = append(d.recentFindings, findings...)
				}
			}

			// Keep only the 10 most recent
			if len(d.recentFindings) > 10 {
				d.recentFindings = d.recentFindings[:10]
			}
		}
	}

	d.lastRefresh = time.Now()
}

// updatePanelContent updates the content of all panels with current data.
func (d *DashboardView) updatePanelContent() {
	d.missionPanel.SetContent(d.renderMissionSummary())
	d.agentPanel.SetContent(d.renderAgentStatus())
	d.findingPanel.SetContent(d.renderRecentFindings())
	d.metricsPanel.SetContent(d.renderSystemMetrics())
}

// renderMissionSummary generates the mission summary panel content.
func (d *DashboardView) renderMissionSummary() string {
	var lines []string

	// Count missions by status
	var active, completed, failed, pending int
	for _, mission := range d.missions {
		switch mission.Status {
		case database.MissionStatusRunning:
			active++
		case database.MissionStatusCompleted:
			completed++
		case database.MissionStatusFailed:
			failed++
		case database.MissionStatusPending:
			pending++
		}
	}

	total := len(d.missions)

	lines = append(lines, fmt.Sprintf("Total Missions: %d", total))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Active:    %d", active))
	lines = append(lines, fmt.Sprintf("Pending:   %d", pending))
	lines = append(lines, fmt.Sprintf("Completed: %d", completed))
	lines = append(lines, fmt.Sprintf("Failed:    %d", failed))

	// Show recent mission if available
	if len(d.missions) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Latest Mission:")
		recent := d.missions[0]
		lines = append(lines, fmt.Sprintf("  %s", truncate(recent.Name, 30)))
		lines = append(lines, fmt.Sprintf("  Status: %s", recent.Status))
		lines = append(lines, fmt.Sprintf("  Progress: %.0f%%", recent.Progress*100))
	}

	return strings.Join(lines, "\n")
}

// renderAgentStatus generates the agent status panel content.
func (d *DashboardView) renderAgentStatus() string {
	var lines []string

	agents, ok := d.components[component.ComponentKindAgent]
	if !ok || len(agents) == 0 {
		lines = append(lines, "No agents registered")
		return strings.Join(lines, "\n")
	}

	var running, stopped, available int
	var runningAgents []*component.Component

	for _, agent := range agents {
		switch agent.Status {
		case component.ComponentStatusRunning:
			running++
			runningAgents = append(runningAgents, agent)
		case component.ComponentStatusStopped:
			stopped++
		case component.ComponentStatusAvailable:
			available++
		}
	}

	lines = append(lines, fmt.Sprintf("Total Agents: %d", len(agents)))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Running:   %d", running))
	lines = append(lines, fmt.Sprintf("Stopped:   %d", stopped))
	lines = append(lines, fmt.Sprintf("Available: %d", available))

	// Show running agents with port/PID
	if len(runningAgents) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Running Agents:")
		for i, agent := range runningAgents {
			if i >= 5 { // Limit to 5 agents
				lines = append(lines, fmt.Sprintf("  ... and %d more", len(runningAgents)-5))
				break
			}

			info := fmt.Sprintf("  %s", truncate(agent.Name, 20))
			if agent.Port > 0 {
				info += fmt.Sprintf(" :%d", agent.Port)
			}
			if agent.PID > 0 {
				info += fmt.Sprintf(" (PID %d)", agent.PID)
			}
			lines = append(lines, info)
		}
	}

	return strings.Join(lines, "\n")
}

// renderRecentFindings generates the recent findings panel content.
func (d *DashboardView) renderRecentFindings() string {
	var lines []string

	if len(d.recentFindings) == 0 {
		lines = append(lines, "No findings yet")
		return strings.Join(lines, "\n")
	}

	// Count by severity
	severityCounts := make(map[agent.FindingSeverity]int)
	for _, finding := range d.recentFindings {
		severityCounts[finding.Severity]++
	}

	lines = append(lines, fmt.Sprintf("Total: %d findings", len(d.recentFindings)))
	lines = append(lines, "")
	lines = append(lines, "By Severity:")
	lines = append(lines, fmt.Sprintf("  Critical: %d", severityCounts[agent.SeverityCritical]))
	lines = append(lines, fmt.Sprintf("  High:     %d", severityCounts[agent.SeverityHigh]))
	lines = append(lines, fmt.Sprintf("  Medium:   %d", severityCounts[agent.SeverityMedium]))
	lines = append(lines, fmt.Sprintf("  Low:      %d", severityCounts[agent.SeverityLow]))

	// Show recent findings
	lines = append(lines, "")
	lines = append(lines, "Recent:")
	displayCount := 5
	if len(d.recentFindings) < displayCount {
		displayCount = len(d.recentFindings)
	}

	for i := 0; i < displayCount; i++ {
		finding := d.recentFindings[i]
		severityIcon := getSeverityIcon(finding.Severity)
		title := truncate(finding.Title, 25)
		lines = append(lines, fmt.Sprintf("  %s %s", severityIcon, title))
	}

	return strings.Join(lines, "\n")
}

// renderSystemMetrics generates the system metrics panel content.
func (d *DashboardView) renderSystemMetrics() string {
	var lines []string

	// Database status
	dbStatus := "Unknown"
	if d.db != nil {
		err := d.db.Health(d.ctx)
		if err == nil {
			dbStatus = "Healthy"
		} else {
			dbStatus = "Error"
		}
	}

	lines = append(lines, "Database:")
	lines = append(lines, fmt.Sprintf("  Status: %s", dbStatus))

	// Component counts
	lines = append(lines, "")
	lines = append(lines, "Components:")

	agentCount := len(d.components[component.ComponentKindAgent])
	toolCount := len(d.components[component.ComponentKindTool])
	pluginCount := len(d.components[component.ComponentKindPlugin])

	lines = append(lines, fmt.Sprintf("  Agents:  %d", agentCount))
	lines = append(lines, fmt.Sprintf("  Tools:   %d", toolCount))
	lines = append(lines, fmt.Sprintf("  Plugins: %d", pluginCount))

	// LLM health (placeholder - would need actual LLM client)
	lines = append(lines, "")
	lines = append(lines, "LLM Service:")
	lines = append(lines, "  Status: Not Configured")

	// Last refresh time
	lines = append(lines, "")
	if !d.lastRefresh.IsZero() {
		elapsed := time.Since(d.lastRefresh)
		lines = append(lines, fmt.Sprintf("Last refresh: %s ago", formatDuration(elapsed)))
	} else {
		lines = append(lines, "Last refresh: Never")
	}

	return strings.Join(lines, "\n")
}

// Helper functions

// truncate truncates a string to the specified length, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// getSeverityIcon returns an icon representing the finding severity.
func getSeverityIcon(severity agent.FindingSeverity) string {
	switch severity {
	case agent.SeverityCritical:
		return "[!]"
	case agent.SeverityHigh:
		return "[H]"
	case agent.SeverityMedium:
		return "[M]"
	case agent.SeverityLow:
		return "[L]"
	case agent.SeverityInfo:
		return "[i]"
	default:
		return "[?]"
	}
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
