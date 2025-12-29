package views

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// MissionSummary represents a mission in the list view.
type MissionSummary struct {
	ID           string
	Name         string
	Status       database.MissionStatus
	Progress     float64
	FindingCount int
	StartedAt    *time.Time
	Duration     time.Duration
	WorkflowName string
	Desc         string
}

// Implement list.Item interface for MissionSummary
func (m MissionSummary) FilterValue() string {
	return m.Name
}

func (m MissionSummary) Title() string {
	return m.Name
}

func (m MissionSummary) Description() string {
	return fmt.Sprintf("Status: %s | Progress: %.0f%% | Findings: %d", m.Status, m.Progress*100, m.FindingCount)
}

// MissionView represents the mission management view with list and details.
type MissionView struct {
	ctx   context.Context
	db    database.MissionDAO
	theme *styles.Theme

	// UI components
	list        list.Model
	logViewport viewport.Model

	// State
	missions        []MissionSummary
	selectedMission *database.Mission
	workflow        *workflow.Workflow
	logs            []string
	detailsExpanded bool

	// Dimensions
	width  int
	height int

	// Error state
	err error
}

// NewMissionView creates a new mission view.
func NewMissionView(ctx context.Context, db database.MissionDAO) *MissionView {
	// Create list with default delegate
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "Missions"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)

	// Create viewport for logs
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	return &MissionView{
		ctx:             ctx,
		db:              db,
		theme:           styles.DefaultTheme(),
		list:            l,
		logViewport:     vp,
		missions:        []MissionSummary{},
		logs:            []string{},
		detailsExpanded: false,
	}
}

// Init initializes the mission view by loading missions from the database.
func (m *MissionView) Init() tea.Cmd {
	return m.loadMissions
}

// loadMissions loads missions from the database.
func (m *MissionView) loadMissions() tea.Msg {
	missions, err := m.db.List(m.ctx, "")
	if err != nil {
		return errMsg{err}
	}

	summaries := make([]MissionSummary, 0, len(missions))
	for _, mission := range missions {
		summary := MissionSummary{
			ID:           string(mission.ID),
			Name:         mission.Name,
			Status:       mission.Status,
			Progress:     mission.Progress,
			FindingCount: mission.FindingsCount,
			StartedAt:    mission.StartedAt,
			Desc:         mission.Description,
		}

		// Calculate duration if mission has started
		if mission.StartedAt != nil {
			if mission.CompletedAt != nil {
				summary.Duration = mission.CompletedAt.Sub(*mission.StartedAt)
			} else {
				summary.Duration = time.Since(*mission.StartedAt)
			}
		}

		summaries = append(summaries, summary)
	}

	return missionsLoadedMsg{summaries}
}

// Update handles messages and updates the mission view state.
func (m *MissionView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()
		return m, nil

	case missionsLoadedMsg:
		m.missions = msg.missions
		items := make([]list.Item, len(m.missions))
		for i, mission := range m.missions {
			items[i] = mission
		}
		m.list.SetItems(items)
		return m, nil

	case missionDetailsLoadedMsg:
		return m, m.handleMissionDetailsLoaded(msg)

	case errMsg:
		m.err = msg.error
		return m, nil

	case tea.KeyMsg:
		// Handle key presses
		switch msg.String() {
		case "j", "down":
			if !m.detailsExpanded {
				var cmd tea.Cmd
				m.list, cmd = m.list.Update(msg)
				cmds = append(cmds, cmd)
				// Update selected mission
				if selectedItem := m.list.SelectedItem(); selectedItem != nil {
					if summary, ok := selectedItem.(MissionSummary); ok {
						cmds = append(cmds, m.loadMissionDetails(summary.ID))
					}
				}
			} else {
				var cmd tea.Cmd
				m.logViewport, cmd = m.logViewport.Update(msg)
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)

		case "k", "up":
			if !m.detailsExpanded {
				var cmd tea.Cmd
				m.list, cmd = m.list.Update(msg)
				cmds = append(cmds, cmd)
				// Update selected mission
				if selectedItem := m.list.SelectedItem(); selectedItem != nil {
					if summary, ok := selectedItem.(MissionSummary); ok {
						cmds = append(cmds, m.loadMissionDetails(summary.ID))
					}
				}
			} else {
				var cmd tea.Cmd
				m.logViewport, cmd = m.logViewport.Update(msg)
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)

		case "enter":
			// Toggle details expansion
			m.detailsExpanded = !m.detailsExpanded
			m.updateSizes()
			return m, nil

		case "r":
			// Run/resume mission
			if m.selectedMission != nil &&
				(m.selectedMission.Status == database.MissionStatusPending ||
					m.selectedMission.Status == database.MissionStatusCancelled) {
				cmds = append(cmds, m.runMission(m.selectedMission.ID))
			}
			return m, tea.Batch(cmds...)

		case "p":
			// Pause mission
			if m.selectedMission != nil && m.selectedMission.Status == database.MissionStatusRunning {
				cmds = append(cmds, m.pauseMission(m.selectedMission.ID))
			}
			return m, tea.Batch(cmds...)

		case "s":
			// Stop mission
			if m.selectedMission != nil && m.selectedMission.Status == database.MissionStatusRunning {
				cmds = append(cmds, m.stopMission(m.selectedMission.ID))
			}
			return m, tea.Batch(cmds...)

		case "d":
			// Delete mission (only if not running)
			if m.selectedMission != nil && m.selectedMission.Status != database.MissionStatusRunning {
				cmds = append(cmds, m.deleteMission(m.selectedMission.ID))
			}
			return m, tea.Batch(cmds...)

		case "a":
			// Show approval dialog (placeholder for now)
			// TODO: Integrate with approval dialog component when available
			return m, nil

		case "ctrl+r":
			// Refresh mission list
			return m, m.loadMissions
		}
	}

	// Update list
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the mission view.
func (m *MissionView) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Split layout: list on left, details on right
	listWidth := m.width / 3
	detailsWidth := m.width - listWidth - 2

	if m.detailsExpanded {
		listWidth = m.width / 4
		detailsWidth = m.width - listWidth - 2
	}

	// Render list
	listView := m.renderList(listWidth, m.height-2)

	// Render details
	detailsView := m.renderDetails(detailsWidth, m.height-2)

	// Combine views horizontally
	mainView := lipgloss.JoinHorizontal(
		lipgloss.Top,
		listView,
		detailsView,
	)

	// Add status bar
	statusBar := m.renderStatusBar()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		mainView,
		statusBar,
	)
}

// renderList renders the mission list.
func (m *MissionView) renderList(width, height int) string {
	m.list.SetSize(width, height)

	// Apply custom styling to list items based on status
	listView := m.list.View()

	return listView
}

// renderDetails renders the mission details panel.
func (m *MissionView) renderDetails(width, height int) string {
	if m.selectedMission == nil {
		emptyStyle := lipgloss.NewStyle().
			Width(width).
			Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(m.theme.Muted)
		return emptyStyle.Render("Select a mission to view details")
	}

	var details strings.Builder

	// Mission header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Primary)
	details.WriteString(headerStyle.Render(m.selectedMission.Name) + "\n\n")

	// Status with colored indicator
	statusStyle := m.theme.StatusStyle(string(m.selectedMission.Status))
	details.WriteString(fmt.Sprintf("Status: %s\n", statusStyle.Render(string(m.selectedMission.Status))))

	// Progress bar
	details.WriteString(m.renderProgressBar(m.selectedMission.Progress) + "\n\n")

	// Mission info
	details.WriteString(fmt.Sprintf("Description: %s\n", m.selectedMission.Description))
	details.WriteString(fmt.Sprintf("Findings: %d\n", m.selectedMission.FindingsCount))

	if m.selectedMission.StartedAt != nil {
		details.WriteString(fmt.Sprintf("Started: %s\n", m.selectedMission.StartedAt.Format(time.RFC822)))

		var duration time.Duration
		if m.selectedMission.CompletedAt != nil {
			duration = m.selectedMission.CompletedAt.Sub(*m.selectedMission.StartedAt)
			details.WriteString(fmt.Sprintf("Completed: %s\n", m.selectedMission.CompletedAt.Format(time.RFC822)))
		} else {
			duration = time.Since(*m.selectedMission.StartedAt)
		}
		details.WriteString(fmt.Sprintf("Duration: %s\n", duration.Round(time.Second)))
	}

	details.WriteString("\n")

	// Workflow DAG visualization
	if m.workflow != nil {
		details.WriteString("Workflow:\n")
		dagView := m.renderDAG()
		details.WriteString(dagView + "\n\n")
	}

	// Log stream section
	if m.detailsExpanded && len(m.logs) > 0 {
		details.WriteString("Logs:\n")
		logHeight := height - 20 // Reserve space for other details
		if logHeight < 5 {
			logHeight = 5
		}
		m.logViewport.Width = width - 4
		m.logViewport.Height = logHeight
		m.logViewport.SetContent(strings.Join(m.logs, "\n"))
		details.WriteString(m.logViewport.View())
	}

	// Wrap in panel
	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Primary).
		Padding(1, 2).
		Width(width).
		Height(height)

	return panelStyle.Render(details.String())
}

// renderProgressBar renders a text-based progress bar.
func (m *MissionView) renderProgressBar(progress float64) string {
	barWidth := 30
	filled := int(progress * float64(barWidth))

	bar := strings.Builder{}
	bar.WriteString("[")

	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar.WriteString("=")
		} else if i == filled {
			bar.WriteString(">")
		} else {
			bar.WriteString(" ")
		}
	}

	bar.WriteString("]")
	bar.WriteString(fmt.Sprintf(" %.0f%%", progress*100))

	return bar.String()
}

// renderDAG renders a simplified DAG visualization.
func (m *MissionView) renderDAG() string {
	if m.workflow == nil || len(m.workflow.Nodes) == 0 {
		return "No workflow data available"
	}

	var dag strings.Builder

	// Render entry points
	entryNodes := m.workflow.GetEntryNodes()
	if len(entryNodes) > 0 {
		dag.WriteString("Entry Points:\n")
		for _, node := range entryNodes {
			dag.WriteString(fmt.Sprintf("  [%s] %s\n", node.ID, node.Name))
		}
		dag.WriteString("\n")
	}

	// Render all nodes with simple status indicators
	dag.WriteString("Nodes:\n")
	for nodeID, node := range m.workflow.Nodes {
		statusIndicator := "○" // Pending/unknown

		// Use node type to determine default appearance
		switch node.Type {
		case workflow.NodeTypeAgent:
			statusIndicator = "●"
		case workflow.NodeTypeTool:
			statusIndicator = "◆"
		case workflow.NodeTypeCondition:
			statusIndicator = "◇"
		}

		dag.WriteString(fmt.Sprintf("  %s [%s] %s (%s)\n", statusIndicator, nodeID, node.Name, node.Type))
	}

	// Render edges
	if len(m.workflow.Edges) > 0 {
		dag.WriteString("\nConnections:\n")
		for _, edge := range m.workflow.Edges {
			arrow := "→"
			if edge.Condition != "" {
				arrow = "⇒" // Conditional edge
			}
			dag.WriteString(fmt.Sprintf("  %s %s %s\n", edge.From, arrow, edge.To))
		}
	}

	return dag.String()
}

// renderStatusBar renders the status bar with key hints.
func (m *MissionView) renderStatusBar() string {
	hints := []string{
		"j/k: navigate",
		"enter: expand/collapse",
		"r: run",
		"p: pause",
		"s: stop",
		"d: delete",
		"ctrl+r: refresh",
	}

	statusStyle := lipgloss.NewStyle().
		Foreground(m.theme.Muted).
		Background(lipgloss.Color("236")).
		Padding(0, 1).
		Width(m.width)

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().
			Foreground(m.theme.Danger).
			Bold(true)
		return statusStyle.Render(errorStyle.Render("Error: " + m.err.Error()))
	}

	return statusStyle.Render(strings.Join(hints, " | "))
}

// updateSizes updates the sizes of child components.
func (m *MissionView) updateSizes() {
	listWidth := m.width / 3

	if m.detailsExpanded {
		listWidth = m.width / 4
	}

	m.list.SetSize(listWidth, m.height-2)
}

// loadMissionDetails loads the full details for a mission.
func (m *MissionView) loadMissionDetails(missionID string) tea.Cmd {
	return func() tea.Msg {
		mission, err := m.db.GetByID(m.ctx, types.ID(missionID))
		if err != nil {
			return errMsg{err}
		}

		// Parse workflow if available
		var wf *workflow.Workflow
		if mission.WorkflowJSON != "" {
			if err := json.Unmarshal([]byte(mission.WorkflowJSON), &wf); err == nil {
				// Successfully parsed workflow
			}
		}

		return missionDetailsLoadedMsg{
			mission:  mission,
			workflow: wf,
		}
	}
}

// runMission starts or resumes a mission.
func (m *MissionView) runMission(missionID types.ID) tea.Cmd {
	return func() tea.Msg {
		if err := m.db.UpdateStatus(m.ctx, missionID, database.MissionStatusRunning); err != nil {
			return errMsg{err}
		}
		return m.loadMissions()
	}
}

// pauseMission pauses a running mission.
func (m *MissionView) pauseMission(missionID types.ID) tea.Cmd {
	return func() tea.Msg {
		// For now, we don't have a paused status in the database, so we use cancelled
		if err := m.db.UpdateStatus(m.ctx, missionID, database.MissionStatusCancelled); err != nil {
			return errMsg{err}
		}
		return m.loadMissions()
	}
}

// stopMission stops a running mission.
func (m *MissionView) stopMission(missionID types.ID) tea.Cmd {
	return func() tea.Msg {
		if err := m.db.UpdateStatus(m.ctx, missionID, database.MissionStatusCancelled); err != nil {
			return errMsg{err}
		}
		return m.loadMissions()
	}
}

// deleteMission deletes a mission.
func (m *MissionView) deleteMission(missionID types.ID) tea.Cmd {
	return func() tea.Msg {
		if err := m.db.Delete(m.ctx, missionID); err != nil {
			return errMsg{err}
		}
		return m.loadMissions()
	}
}

// Message types for mission view

type missionsLoadedMsg struct {
	missions []MissionSummary
}

type missionDetailsLoadedMsg struct {
	mission  *database.Mission
	workflow *workflow.Workflow
}

type errMsg struct {
	error
}

// Handle mission details loaded message
func (m *MissionView) handleMissionDetailsLoaded(msg missionDetailsLoadedMsg) tea.Cmd {
	m.selectedMission = msg.mission
	m.workflow = msg.workflow

	// Clear existing logs
	m.logs = []string{}

	// TODO: Load logs from a log storage system when available
	// For now, show placeholder logs
	if msg.mission.Status == database.MissionStatusRunning {
		m.logs = append(m.logs,
			fmt.Sprintf("[%s] Mission started", time.Now().Format(time.RFC3339)),
			"Initializing workflow execution...",
			"Loading agent configurations...",
			"Ready to execute tasks...",
		)
	}

	return nil
}
