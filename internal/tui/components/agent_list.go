package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// AgentListItem represents a single agent in the list with its current state.
// It contains all the information needed to render an agent entry in the sidebar.
type AgentListItem struct {
	Name           string               // Agent name/identifier
	Status         database.AgentStatus // Current execution status
	Mode           database.AgentMode   // Execution mode (autonomous/interactive)
	CurrentAction  string               // Brief description of current action
	NeedsAttention bool                 // Visual indicator for errors/stalls
}

// AgentListSidebar displays a list of agents with status indicators.
// It provides a navigable sidebar showing all active agents and their current state.
// The selected agent can be highlighted and navigated using keyboard controls.
type AgentListSidebar struct {
	agents        []AgentListItem
	selectedIndex int
	width         int
	height        int
	theme         *styles.Theme
}

// NewAgentListSidebar creates a new agent list sidebar with default settings.
// The sidebar is initially empty and uses the default theme.
func NewAgentListSidebar() *AgentListSidebar {
	return &AgentListSidebar{
		agents:        []AgentListItem{},
		selectedIndex: 0,
		width:         30,
		height:        10,
		theme:         styles.DefaultTheme(),
	}
}

// SetAgents updates the list of agents displayed in the sidebar.
// This will reset the selection to the first agent if the list changes.
func (s *AgentListSidebar) SetAgents(agents []AgentListItem) {
	s.agents = agents
	// Keep selection in bounds
	if s.selectedIndex >= len(s.agents) {
		s.selectedIndex = len(s.agents) - 1
	}
	if s.selectedIndex < 0 {
		s.selectedIndex = 0
	}
}

// SetSize sets the dimensions of the sidebar.
// Width and height should include the border.
func (s *AgentListSidebar) SetSize(width, height int) {
	if width > 0 {
		s.width = width
	}
	if height > 0 {
		s.height = height
	}
}

// SetTheme sets the theme for the sidebar.
// This allows customization of the sidebar appearance.
func (s *AgentListSidebar) SetTheme(theme *styles.Theme) {
	if theme != nil {
		s.theme = theme
	}
}

// Selected returns the currently selected agent, or nil if no agent is selected.
func (s *AgentListSidebar) Selected() *AgentListItem {
	if s.selectedIndex >= 0 && s.selectedIndex < len(s.agents) {
		return &s.agents[s.selectedIndex]
	}
	return nil
}

// SelectNext moves the selection to the next agent in the list.
// Wraps around to the first agent if at the end.
func (s *AgentListSidebar) SelectNext() {
	if len(s.agents) == 0 {
		return
	}
	s.selectedIndex = (s.selectedIndex + 1) % len(s.agents)
}

// SelectPrev moves the selection to the previous agent in the list.
// Wraps around to the last agent if at the beginning.
func (s *AgentListSidebar) SelectPrev() {
	if len(s.agents) == 0 {
		return
	}
	s.selectedIndex--
	if s.selectedIndex < 0 {
		s.selectedIndex = len(s.agents) - 1
	}
}

// SelectByName selects the agent with the given name.
// Returns true if the agent was found and selected, false otherwise.
func (s *AgentListSidebar) SelectByName(name string) bool {
	for i, agent := range s.agents {
		if agent.Name == name {
			s.selectedIndex = i
			return true
		}
	}
	return false
}

// Update handles keyboard input for navigating the agent list.
// Supports j/k (vim-style) and arrow keys for navigation.
func (s *AgentListSidebar) Update(msg tea.Msg) (*AgentListSidebar, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			s.SelectNext()
		case "k", "up":
			s.SelectPrev()
		}
	}
	return s, nil
}

// View renders the agent list sidebar to a string.
// Each agent is displayed with:
// - Status icon (colored based on status)
// - Agent name
// - Mode indicator (A for autonomous, I for interactive)
// - Current action (if available)
// - Attention indicator (for agents needing attention)
func (s *AgentListSidebar) View() string {
	// Border characters
	const (
		topLeft     = "┌"
		topRight    = "┐"
		bottomLeft  = "└"
		bottomRight = "┘"
		horizontal  = "─"
		vertical    = "│"
	)

	borderStyle := lipgloss.NewStyle().Foreground(s.theme.Muted)
	titleStyle := s.theme.TitleStyle

	// Build top border with title
	title := " Agents "
	innerWidth := s.width - 2 // width between corner chars
	titleLen := len(title)
	remainingWidth := innerWidth - titleLen

	var topBorder string
	if remainingWidth > 0 {
		topBorder = borderStyle.Render(topLeft) +
			titleStyle.Render(title) +
			borderStyle.Render(strings.Repeat(horizontal, remainingWidth)+topRight)
	} else {
		topBorder = borderStyle.Render(topLeft + strings.Repeat(horizontal, innerWidth) + topRight)
	}

	// Build bottom border
	bottomBorder := borderStyle.Render(bottomLeft + strings.Repeat(horizontal, innerWidth) + bottomRight)

	// Calculate content dimensions
	contentWidth := s.width - 4 // border(1) + padding(1) on each side
	contentHeight := s.height - 2 // top and bottom borders

	if contentWidth < 1 {
		contentWidth = 1
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Render agent list
	var lines []string

	if len(s.agents) == 0 {
		// Show "no agents" message
		emptyMsg := "No active agents"
		lines = append(lines, s.fitLine(emptyMsg, contentWidth))
	} else {
		// Render each agent
		for i, agent := range s.agents {
			if i >= contentHeight {
				break // Don't exceed height
			}
			line := s.renderAgentLine(agent, i == s.selectedIndex, contentWidth)
			lines = append(lines, line)
		}
	}

	// Pad with empty lines to fill height
	for len(lines) < contentHeight {
		lines = append(lines, strings.Repeat(" ", contentWidth))
	}

	// Build final output with borders
	var rows []string
	rows = append(rows, topBorder)
	for _, line := range lines {
		row := borderStyle.Render(vertical) + " " + line + " " + borderStyle.Render(vertical)
		rows = append(rows, row)
	}
	rows = append(rows, bottomBorder)

	return strings.Join(rows, "\n")
}

// renderAgentLine renders a single agent entry with status icon, name, mode, and action.
func (s *AgentListSidebar) renderAgentLine(agent AgentListItem, selected bool, width int) string {
	// Get status icon and style
	statusIcon := s.getStatusIcon(agent.Status)
	statusStyle := s.getStatusStyle(agent.Status, agent.NeedsAttention)

	// Get mode indicator
	modeIndicator := s.getModeIndicator(agent.Mode)
	modeStyle := lipgloss.NewStyle().Foreground(s.theme.Muted)

	// Build the line: [icon] Name [mode]
	// If selected, highlight the entire line
	var parts []string

	// Status icon with color
	parts = append(parts, statusStyle.Render(statusIcon))

	// Agent name
	nameStyle := lipgloss.NewStyle()
	if selected {
		nameStyle = nameStyle.Foreground(s.theme.Primary).Bold(true)
	}
	parts = append(parts, nameStyle.Render(agent.Name))

	// Mode indicator
	parts = append(parts, modeStyle.Render(modeIndicator))

	line := strings.Join(parts, " ")

	// If there's a current action and space, add it on a second line or truncate
	// For now, just fit the name line
	return s.fitLine(line, width)
}

// getStatusIcon returns the icon character for a given agent status.
func (s *AgentListSidebar) getStatusIcon(status database.AgentStatus) string {
	switch status {
	case database.AgentStatusRunning:
		return "▶" // Running
	case database.AgentStatusPaused:
		return "⏸" // Paused
	case database.AgentStatusWaitingInput:
		return "⏳" // Waiting
	case database.AgentStatusInterrupted:
		return "⏹" // Stopped/interrupted
	case database.AgentStatusCompleted:
		return "✓" // Complete
	case database.AgentStatusFailed:
		return "✗" // Failed
	default:
		return "•" // Unknown
	}
}

// getStatusStyle returns the appropriate style for an agent's status.
// Agents needing attention are highlighted in red/yellow.
func (s *AgentListSidebar) getStatusStyle(status database.AgentStatus, needsAttention bool) lipgloss.Style {
	// If needs attention, use danger color
	if needsAttention {
		return lipgloss.NewStyle().
			Foreground(s.theme.Danger).
			Bold(true)
	}

	// Otherwise use status-specific color
	switch status {
	case database.AgentStatusRunning:
		return s.theme.StatusRunning
	case database.AgentStatusPaused:
		return s.theme.StatusPaused
	case database.AgentStatusWaitingInput:
		return lipgloss.NewStyle().Foreground(s.theme.Warning)
	case database.AgentStatusInterrupted:
		return s.theme.StatusStopped
	case database.AgentStatusCompleted:
		return s.theme.StatusComplete
	case database.AgentStatusFailed:
		return s.theme.StatusError
	default:
		return lipgloss.NewStyle().Foreground(s.theme.Muted)
	}
}

// getModeIndicator returns a single-character indicator for the agent mode.
func (s *AgentListSidebar) getModeIndicator(mode database.AgentMode) string {
	switch mode {
	case database.AgentModeAutonomous:
		return "[A]"
	case database.AgentModeInteractive:
		return "[I]"
	default:
		return "[ ]"
	}
}

// fitLine truncates or pads a line to exactly the specified width.
// This ensures consistent rendering width for all lines.
func (s *AgentListSidebar) fitLine(line string, width int) string {
	lineWidth := lipgloss.Width(line)
	if lineWidth > width {
		// Truncate with ellipsis
		runes := []rune(line)
		if width > 3 {
			// Find how many runes we can keep
			kept := 0
			keptWidth := 0
			for i, r := range runes {
				rWidth := lipgloss.Width(string(r))
				if keptWidth+rWidth > width-3 {
					break
				}
				kept = i + 1
				keptWidth += rWidth
			}
			return string(runes[:kept]) + "..." + strings.Repeat(" ", width-keptWidth-3)
		}
		return string(runes[:width])
	}
	// Pad
	return line + strings.Repeat(" ", width-lineWidth)
}

// AgentListMsg is a message type for updating the agent list.
// This can be sent to the sidebar to update the displayed agents.
type AgentListMsg struct {
	Agents []AgentListItem
}

// AgentStatusMsg is a message type for updating a single agent's status.
type AgentStatusMsg struct {
	AgentName      string
	Status         database.AgentStatus
	CurrentAction  string
	NeedsAttention bool
}

// HandleAgentListMsg updates the sidebar with a new list of agents.
func (s *AgentListSidebar) HandleAgentListMsg(msg AgentListMsg) tea.Cmd {
	s.SetAgents(msg.Agents)
	return nil
}

// HandleAgentStatusMsg updates the status of a specific agent in the list.
func (s *AgentListSidebar) HandleAgentStatusMsg(msg AgentStatusMsg) tea.Cmd {
	for i := range s.agents {
		if s.agents[i].Name == msg.AgentName {
			s.agents[i].Status = msg.Status
			s.agents[i].CurrentAction = msg.CurrentAction
			s.agents[i].NeedsAttention = msg.NeedsAttention
			break
		}
	}
	return nil
}

// SelectedIndexMsg returns the currently selected index for external consumers.
type SelectedIndexMsg struct {
	Index int
}

// GetSelectedIndex returns the current selection index.
func (s *AgentListSidebar) GetSelectedIndex() int {
	return s.selectedIndex
}

// SetSelectedIndex sets the selection index directly.
// The index will be clamped to valid bounds.
func (s *AgentListSidebar) SetSelectedIndex(index int) {
	if len(s.agents) == 0 {
		s.selectedIndex = 0
		return
	}
	if index < 0 {
		s.selectedIndex = 0
	} else if index >= len(s.agents) {
		s.selectedIndex = len(s.agents) - 1
	} else {
		s.selectedIndex = index
	}
}

// GetAgents returns the current list of agents.
func (s *AgentListSidebar) GetAgents() []AgentListItem {
	return s.agents
}

// Count returns the number of agents in the list.
func (s *AgentListSidebar) Count() int {
	return len(s.agents)
}

// IsEmpty returns true if there are no agents in the list.
func (s *AgentListSidebar) IsEmpty() bool {
	return len(s.agents) == 0
}

// FilterByStatus returns a list of agents matching the given status.
func (s *AgentListSidebar) FilterByStatus(status database.AgentStatus) []AgentListItem {
	var filtered []AgentListItem
	for _, agent := range s.agents {
		if agent.Status == status {
			filtered = append(filtered, agent)
		}
	}
	return filtered
}

// FilterByMode returns a list of agents matching the given mode.
func (s *AgentListSidebar) FilterByMode(mode database.AgentMode) []AgentListItem {
	var filtered []AgentListItem
	for _, agent := range s.agents {
		if agent.Mode == mode {
			filtered = append(filtered, agent)
		}
	}
	return filtered
}

// GetAgentByName returns the agent with the given name, or nil if not found.
func (s *AgentListSidebar) GetAgentByName(name string) *AgentListItem {
	for i := range s.agents {
		if s.agents[i].Name == name {
			return &s.agents[i]
		}
	}
	return nil
}

// GetNeedingAttention returns all agents that need attention.
func (s *AgentListSidebar) GetNeedingAttention() []AgentListItem {
	var needsAttention []AgentListItem
	for _, agent := range s.agents {
		if agent.NeedsAttention {
			needsAttention = append(needsAttention, agent)
		}
	}
	return needsAttention
}

// GetSummary returns a human-readable summary of the agent list.
func (s *AgentListSidebar) GetSummary() string {
	if len(s.agents) == 0 {
		return "No active agents"
	}

	running := len(s.FilterByStatus(database.AgentStatusRunning))
	paused := len(s.FilterByStatus(database.AgentStatusPaused))
	waiting := len(s.FilterByStatus(database.AgentStatusWaitingInput))
	completed := len(s.FilterByStatus(database.AgentStatusCompleted))
	failed := len(s.FilterByStatus(database.AgentStatusFailed))
	needsAttention := len(s.GetNeedingAttention())

	parts := []string{}
	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", running))
	}
	if paused > 0 {
		parts = append(parts, fmt.Sprintf("%d paused", paused))
	}
	if waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d waiting", waiting))
	}
	if completed > 0 {
		parts = append(parts, fmt.Sprintf("%d completed", completed))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if needsAttention > 0 {
		parts = append(parts, fmt.Sprintf("%d need attention", needsAttention))
	}

	if len(parts) == 0 {
		return fmt.Sprintf("%d agents", len(s.agents))
	}

	return strings.Join(parts, ", ")
}
