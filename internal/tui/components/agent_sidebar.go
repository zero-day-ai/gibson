package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// AgentSidebar displays a list of agents with status indicators and log previews.
// It provides navigation between agents and displays the last 5-7 log lines for each agent.
type AgentSidebar struct {
	sessions      []database.AgentSession
	logPreviews   map[string][]string // agent name -> last 5-7 lines
	selectedIndex int
	focusedAgent  string
	width         int
	height        int
	theme         *styles.Theme
	maxLogLines   int // Maximum log lines to show per agent (default: 7)
}

const (
	// DefaultMaxLogLines is the default number of log lines to display per agent
	DefaultMaxLogLines = 7
	// MaxAgentNameLength is the maximum length for displaying agent names
	MaxAgentNameLength = 15
	// SidebarPaddingLeft is the left padding inside the sidebar
	SidebarPaddingLeft = 1
	// SidebarPaddingRight is the right padding inside the sidebar
	SidebarPaddingRight = 1
)

// NewAgentSidebar creates a new AgentSidebar with the provided theme.
func NewAgentSidebar(theme *styles.Theme) *AgentSidebar {
	if theme == nil {
		theme = styles.DefaultTheme()
	}

	return &AgentSidebar{
		sessions:      []database.AgentSession{},
		logPreviews:   make(map[string][]string),
		selectedIndex: 0,
		focusedAgent:  "",
		width:         20,
		height:        24,
		theme:         theme,
		maxLogLines:   DefaultMaxLogLines,
	}
}

// SetSessions updates the list of agent sessions displayed in the sidebar.
func (s *AgentSidebar) SetSessions(sessions []database.AgentSession) {
	s.sessions = sessions

	// Clean up log previews for agents that no longer exist
	existingAgents := make(map[string]bool)
	for _, session := range sessions {
		existingAgents[session.AgentName] = true
	}

	for agentName := range s.logPreviews {
		if !existingAgents[agentName] {
			delete(s.logPreviews, agentName)
		}
	}

	// Adjust selected index if it's out of bounds
	if s.selectedIndex >= len(s.sessions) && len(s.sessions) > 0 {
		s.selectedIndex = len(s.sessions) - 1
	}
}

// SetFocusedAgent sets the currently focused agent by name.
func (s *AgentSidebar) SetFocusedAgent(name string) {
	s.focusedAgent = name

	// Update selected index to match the focused agent
	for i, session := range s.sessions {
		if session.AgentName == name {
			s.selectedIndex = i
			break
		}
	}
}

// AppendLog appends a log line to the agent's log preview buffer.
// The buffer maintains the last maxLogLines lines for the agent.
func (s *AgentSidebar) AppendLog(agentName string, line string) {
	if line == "" {
		return
	}

	// Initialize log buffer if it doesn't exist
	if s.logPreviews[agentName] == nil {
		s.logPreviews[agentName] = make([]string, 0, s.maxLogLines)
	}

	// Append the new line
	s.logPreviews[agentName] = append(s.logPreviews[agentName], line)

	// Trim to maxLogLines if exceeded
	if len(s.logPreviews[agentName]) > s.maxLogLines {
		excess := len(s.logPreviews[agentName]) - s.maxLogLines
		s.logPreviews[agentName] = s.logPreviews[agentName][excess:]
	}
}

// SetSize updates the dimensions of the sidebar.
func (s *AgentSidebar) SetSize(width, height int) {
	if width > 0 {
		s.width = width
	}
	if height > 0 {
		s.height = height
	}
}

// View renders the agent sidebar to a string.
// Each agent is displayed with status indicator, mode, and recent log lines.
func (s *AgentSidebar) View() string {
	if len(s.sessions) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(s.theme.Muted).
			Width(s.width).
			Height(s.height).
			Padding(1)
		return emptyStyle.Render("No active agents")
	}

	var lines []string
	lines = append(lines, s.theme.TitleStyle.Render("Active Agents"))
	lines = append(lines, "")

	for i, session := range s.sessions {
		// Render agent entry (name + status + mode + logs)
		agentEntry := s.renderAgentEntry(session, i == s.selectedIndex)
		lines = append(lines, agentEntry)

		// Add separator between agents (except after last one)
		if i < len(s.sessions)-1 {
			separatorStyle := lipgloss.NewStyle().
				Foreground(s.theme.Muted)
			separator := strings.Repeat("─", s.width-4)
			lines = append(lines, separatorStyle.Render(separator))
		}
	}

	content := strings.Join(lines, "\n")

	// Apply sidebar border
	sidebarStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.theme.Muted).
		Width(s.width-2).
		Height(s.height-2).
		Padding(0, SidebarPaddingLeft)

	return sidebarStyle.Render(content)
}

// renderAgentEntry renders a single agent entry with status, mode, and log preview.
func (s *AgentSidebar) renderAgentEntry(session database.AgentSession, isSelected bool) string {
	var entryLines []string

	// Status indicator and agent name
	statusIndicator := s.getStatusIndicator(session.Status)
	agentName := session.AgentName
	if len(agentName) > MaxAgentNameLength {
		agentName = agentName[:MaxAgentNameLength-3] + "..."
	}

	// First line: indicator + name
	nameLine := fmt.Sprintf("%s %s", statusIndicator, agentName)

	// Apply selection highlighting
	if isSelected {
		nameStyle := lipgloss.NewStyle().
			Foreground(s.theme.Primary).
			Bold(true).
			Reverse(true)
		nameLine = nameStyle.Render(nameLine)
	} else {
		nameStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))
		nameLine = nameStyle.Render(nameLine)
	}

	entryLines = append(entryLines, nameLine)

	// Second line: [status] <mode>
	statusStyle := s.theme.StatusStyle(string(session.Status))
	modeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB000"))

	statusModeLine := fmt.Sprintf("  [%s] <%s>",
		statusStyle.Render(string(session.Status)),
		modeStyle.Render(string(session.Mode)))
	entryLines = append(entryLines, statusModeLine)

	// Log preview lines (if any)
	if logs, exists := s.logPreviews[session.AgentName]; exists && len(logs) > 0 {
		logStyle := lipgloss.NewStyle().
			Foreground(s.theme.Muted).
			Italic(true)

		for _, logLine := range logs {
			// Truncate log line to fit sidebar width
			displayLine := logLine
			maxLineWidth := s.width - 6 // Account for padding and prefix
			if len(displayLine) > maxLineWidth {
				displayLine = displayLine[:maxLineWidth-3] + "..."
			}

			styledLog := logStyle.Render("  " + displayLine)
			entryLines = append(entryLines, styledLog)
		}
	}

	return strings.Join(entryLines, "\n")
}

// getStatusIndicator returns a colored status indicator for the agent status.
// Status indicators match the design from agent_focus.go lines 738-768.
func (s *AgentSidebar) getStatusIndicator(status database.AgentStatus) string {
	var indicator string
	var color lipgloss.Color

	switch status {
	case database.AgentStatusRunning:
		indicator = "●"
		color = lipgloss.Color("#00FF00") // Green
	case database.AgentStatusPaused:
		indicator = "◐"
		color = lipgloss.Color("#FFB000") // Orange
	case database.AgentStatusWaitingInput:
		indicator = "◔"
		color = lipgloss.Color("#FFD966") // Light orange
	case database.AgentStatusInterrupted:
		indicator = "■"
		color = lipgloss.Color("#FF6600") // Dark orange
	case database.AgentStatusCompleted:
		indicator = "✓"
		color = lipgloss.Color("#00FFB0") // Light green
	case database.AgentStatusFailed:
		indicator = "✗"
		color = lipgloss.Color("#FF0000") // Red
	default:
		indicator = "○"
		color = lipgloss.Color("#888888") // Gray
	}

	style := lipgloss.NewStyle().Foreground(color)
	return style.Render(indicator)
}

// GetFocusedAgent returns the name of the currently focused agent.
func (s *AgentSidebar) GetFocusedAgent() string {
	return s.focusedAgent
}

// CycleNext cycles focus to the next agent in the list.
// It wraps around to the first agent when reaching the end.
// Returns the name of the newly focused agent.
func (s *AgentSidebar) CycleNext() string {
	if len(s.sessions) == 0 {
		return ""
	}

	s.selectedIndex = (s.selectedIndex + 1) % len(s.sessions)
	s.focusedAgent = s.sessions[s.selectedIndex].AgentName
	return s.focusedAgent
}

// CyclePrev cycles focus to the previous agent in the list.
// It wraps around to the last agent when at the beginning.
// Returns the name of the newly focused agent.
func (s *AgentSidebar) CyclePrev() string {
	if len(s.sessions) == 0 {
		return ""
	}

	s.selectedIndex--
	if s.selectedIndex < 0 {
		s.selectedIndex = len(s.sessions) - 1
	}

	s.focusedAgent = s.sessions[s.selectedIndex].AgentName
	return s.focusedAgent
}

// Update handles keyboard input for navigating the agent sidebar.
// Supports j/k (vim-style) and arrow keys for navigation.
// This implements the Bubble Tea Update pattern for the component.
func (s *AgentSidebar) Update(msg tea.Msg) (*AgentSidebar, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			s.CycleNext()
		case "k", "up":
			s.CyclePrev()
		}

	case AgentSessionsMsg:
		// Handle session updates
		s.SetSessions(msg.Sessions)

	case AgentLogMsg:
		// Handle log line updates
		s.AppendLog(msg.AgentName, msg.Line)
	}

	return s, nil
}

// GetSelectedIndex returns the currently selected agent index.
func (s *AgentSidebar) GetSelectedIndex() int {
	return s.selectedIndex
}

// SetSelectedIndex sets the selection index directly.
// The index will be clamped to valid bounds.
func (s *AgentSidebar) SetSelectedIndex(index int) {
	if len(s.sessions) == 0 {
		s.selectedIndex = 0
		return
	}
	if index < 0 {
		s.selectedIndex = 0
	} else if index >= len(s.sessions) {
		s.selectedIndex = len(s.sessions) - 1
	} else {
		s.selectedIndex = index
	}
}

// GetSessions returns the current list of agent sessions.
func (s *AgentSidebar) GetSessions() []database.AgentSession {
	return s.sessions
}

// Count returns the number of agents in the sidebar.
func (s *AgentSidebar) Count() int {
	return len(s.sessions)
}

// IsEmpty returns true if there are no agents in the sidebar.
func (s *AgentSidebar) IsEmpty() bool {
	return len(s.sessions) == 0
}

// FilterByStatus returns a list of sessions matching the given status.
func (s *AgentSidebar) FilterByStatus(status database.AgentStatus) []database.AgentSession {
	var filtered []database.AgentSession
	for _, session := range s.sessions {
		if session.Status == status {
			filtered = append(filtered, session)
		}
	}
	return filtered
}

// FilterByMode returns a list of sessions matching the given mode.
func (s *AgentSidebar) FilterByMode(mode database.AgentMode) []database.AgentSession {
	var filtered []database.AgentSession
	for _, session := range s.sessions {
		if session.Mode == mode {
			filtered = append(filtered, session)
		}
	}
	return filtered
}

// GetSessionByName returns the session with the given agent name, or nil if not found.
func (s *AgentSidebar) GetSessionByName(name string) *database.AgentSession {
	for i := range s.sessions {
		if s.sessions[i].AgentName == name {
			return &s.sessions[i]
		}
	}
	return nil
}

// GetSummary returns a human-readable summary of the agent sidebar.
func (s *AgentSidebar) GetSummary() string {
	if len(s.sessions) == 0 {
		return "No active agents"
	}

	running := len(s.FilterByStatus(database.AgentStatusRunning))
	paused := len(s.FilterByStatus(database.AgentStatusPaused))
	waiting := len(s.FilterByStatus(database.AgentStatusWaitingInput))
	completed := len(s.FilterByStatus(database.AgentStatusCompleted))
	failed := len(s.FilterByStatus(database.AgentStatusFailed))

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

	if len(parts) == 0 {
		return fmt.Sprintf("%d agents", len(s.sessions))
	}

	return strings.Join(parts, ", ")
}

// AgentSessionsMsg is a message type for updating the entire agent session list.
type AgentSessionsMsg struct {
	Sessions []database.AgentSession
}

// AgentLogMsg is a message type for appending a log line to an agent's preview.
type AgentLogMsg struct {
	AgentName string
	Line      string
}
