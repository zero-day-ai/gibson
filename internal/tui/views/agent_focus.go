package views

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// AgentFocusView provides real-time agent observation and steering.
// It implements a split-pane layout with:
// - Left sidebar: Agent list with status indicators (~20% width)
// - Right main area: Scrollable output viewport with input at bottom (~80% width)
//
// Key bindings:
// - Tab: Cycle to next agent
// - Enter: Send steering message
// - Ctrl+C: Send interrupt to agent
// - PgUp/PgDown: Scroll output viewport
// - Esc: Clear input
type AgentFocusView struct {
	ctx           context.Context
	streamManager *agent.StreamManager

	// UI components
	input      textinput.Model // Steering message input at bottom
	outputView viewport.Model  // Agent output stream (scrollable)
	agentList  *AgentListSidebar

	// State
	focusedAgent string                     // Currently focused agent name
	sessions     []database.AgentSession    // All active sessions
	outputLines  []OutputLine               // Buffered output lines for focused agent
	eventCh      <-chan *database.StreamEvent // Event channel from StreamManager
	theme        *styles.Theme
	width        int
	height       int
	maxLines     int // Maximum output lines to retain (default: 1000)
}

// OutputLine represents a styled line in the output viewport
type OutputLine struct {
	Text      string
	Style     OutputStyle
	Timestamp time.Time
}

// OutputStyle categorizes output lines for styling
type OutputStyle int

const (
	StyleNormal OutputStyle = iota
	StyleOutput
	StyleToolCall
	StyleToolResult
	StyleFinding
	StyleStatus
	StyleSteeringAck
	StyleError
	StyleInfo
)

// AgentFocusConfig holds configuration for creating AgentFocusView
type AgentFocusConfig struct {
	StreamManager *agent.StreamManager
}

// AgentListSidebar displays a list of agents with status indicators
type AgentListSidebar struct {
	sessions      []database.AgentSession
	selectedIndex int
	width         int
	height        int
	theme         *styles.Theme
}

const (
	// DefaultMaxOutputLines is the default maximum number of output lines to retain
	DefaultMaxOutputLines = 1000
	// SidebarWidthPercent is the percentage of screen width for the sidebar
	SidebarWidthPercent = 20
	// SteeringPromptText is the input prompt
	SteeringPromptText = "steer> "
)

// NewAgentFocusView creates a new AgentFocusView with the provided configuration.
func NewAgentFocusView(ctx context.Context, config AgentFocusConfig) *AgentFocusView {
	// Initialize text input for steering messages
	ti := textinput.New()
	ti.Placeholder = "Type steering message or command..."
	ti.CharLimit = 500
	ti.Width = 60
	ti.Prompt = SteeringPromptText
	ti.PromptStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00A6FF")).
		Bold(true)
	ti.TextStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF"))

	// Initialize viewport for output display
	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#888888")).
		Padding(0, 1)

	// Initialize agent list sidebar
	agentList := &AgentListSidebar{
		sessions:      []database.AgentSession{},
		selectedIndex: 0,
		theme:         styles.DefaultTheme(),
	}

	view := &AgentFocusView{
		ctx:           ctx,
		streamManager: config.StreamManager,
		input:         ti,
		outputView:    vp,
		agentList:     agentList,
		focusedAgent:  "",
		sessions:      []database.AgentSession{},
		outputLines:   make([]OutputLine, 0, DefaultMaxOutputLines),
		theme:         styles.DefaultTheme(),
		width:         80,
		height:        24,
		maxLines:      DefaultMaxOutputLines,
	}

	// Load initial sessions
	view.refreshSessions()

	return view
}

// Init initializes the AgentFocusView and returns the initial command.
func (v *AgentFocusView) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		v.waitForEvent(), // Start waiting for stream events
	)
}

// Focus focuses the text input field when the view becomes active.
func (v *AgentFocusView) Focus() tea.Cmd {
	v.input.Focus()
	return textinput.Blink
}

// Blur blurs the text input field when the view becomes inactive.
func (v *AgentFocusView) Blur() {
	v.input.Blur()
}

// Update handles messages and updates the AgentFocusView state.
func (v *AgentFocusView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.updateSizes()
		return v, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Send steering message
			content := v.input.Value()
			if content != "" && v.focusedAgent != "" {
				cmds = append(cmds, v.SendSteering())
				v.input.SetValue("")
			}
			return v, tea.Batch(cmds...)

		case "tab":
			// Cycle to next agent
			v.CycleNextAgent()
			return v, nil

		case "ctrl+c":
			// Send interrupt to agent
			if v.focusedAgent != "" {
				cmds = append(cmds, v.SendInterrupt())
			}
			return v, tea.Batch(cmds...)

		case "pgup":
			// Scroll output viewport up
			v.outputView.ViewUp()
			return v, nil

		case "pgdown":
			// Scroll output viewport down
			v.outputView.ViewDown()
			return v, nil

		case "esc":
			// Clear input
			v.input.SetValue("")
			return v, nil
		}

	case StreamEventMsg:
		// Handle incoming stream event
		event := msg.Event
		if event.SessionID.String() == v.getCurrentSessionID() {
			v.appendStreamEvent(event)
			// Continue waiting for next event
			cmds = append(cmds, v.waitForEvent())
		}
		return v, tea.Batch(cmds...)

	case RefreshSessionsMsg:
		// Refresh the list of active sessions
		v.refreshSessions()
		return v, nil
	}

	// Forward to input
	var inputCmd tea.Cmd
	v.input, inputCmd = v.input.Update(msg)
	cmds = append(cmds, inputCmd)

	// Forward to output viewport
	var outputCmd tea.Cmd
	v.outputView, outputCmd = v.outputView.Update(msg)
	cmds = append(cmds, outputCmd)

	return v, tea.Batch(cmds...)
}

// View renders the AgentFocusView with sidebar and output viewport.
func (v *AgentFocusView) View() string {
	if v.width == 0 || v.height == 0 {
		return "Loading agent focus view..."
	}

	// Render sidebar
	sidebarView := v.agentList.View()

	// Render main area (output + input)
	var mainSections []string

	// Add header with focused agent info
	if v.focusedAgent != "" {
		session := v.getFocusedSession()
		if session != nil {
			header := v.renderHeader(session)
			mainSections = append(mainSections, header)
		}
	} else {
		mainSections = append(mainSections, v.theme.TitleStyle.Render("No agent focused"))
	}

	// Add output viewport
	mainSections = append(mainSections, v.outputView.View())

	// Add input line
	mainSections = append(mainSections, v.input.View())

	mainView := lipgloss.JoinVertical(lipgloss.Left, mainSections...)

	// Combine sidebar and main area
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebarView,
		mainView,
	)
}

// SetSize updates the dimensions of the view and child components.
func (v *AgentFocusView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.updateSizes()
}

// updateSizes recalculates and applies sizes to child components.
func (v *AgentFocusView) updateSizes() {
	// Calculate sidebar width (20% of screen)
	sidebarWidth := (v.width * SidebarWidthPercent) / 100
	if sidebarWidth < 20 {
		sidebarWidth = 20
	}

	// Calculate main area width
	mainWidth := v.width - sidebarWidth - 2 // Account for spacing

	// Update sidebar size
	v.agentList.width = sidebarWidth
	v.agentList.height = v.height

	// Calculate heights for main area
	headerHeight := 2
	inputHeight := 1
	outputHeight := v.height - headerHeight - inputHeight - 6 // Account for borders and padding

	if outputHeight < 5 {
		outputHeight = 5
	}

	// Update component sizes
	v.input.Width = mainWidth - len(SteeringPromptText) - 4
	v.outputView.Width = mainWidth - 4
	v.outputView.Height = outputHeight

	// Refresh output content
	v.refreshOutputContent()
}

// FocusAgent focuses on a specific agent by name.
// This subscribes to the agent's stream and displays its output.
func (v *AgentFocusView) FocusAgent(name string) tea.Cmd {
	// Unsubscribe from previous agent if any
	if v.streamManager != nil && v.eventCh != nil && v.focusedAgent != "" {
		v.streamManager.Unsubscribe(v.focusedAgent, v.eventCh)
	}

	// Clear output
	v.outputLines = make([]OutputLine, 0, v.maxLines)
	v.focusedAgent = name

	// Subscribe to new agent
	if v.streamManager != nil && name != "" {
		v.eventCh = v.streamManager.Subscribe(name)
		v.appendOutput(OutputLine{
			Text:      fmt.Sprintf("=== Focused on agent: %s ===", name),
			Style:     StyleInfo,
			Timestamp: time.Now(),
		})
	}

	v.refreshOutputContent()

	return v.waitForEvent()
}

// CycleNextAgent cycles focus to the next agent in the list.
func (v *AgentFocusView) CycleNextAgent() {
	if len(v.sessions) == 0 {
		return
	}

	v.agentList.selectedIndex = (v.agentList.selectedIndex + 1) % len(v.sessions)
	selected := v.sessions[v.agentList.selectedIndex]
	v.FocusAgent(selected.AgentName)
}

// SendSteering sends a steering message to the currently focused agent.
func (v *AgentFocusView) SendSteering() tea.Cmd {
	return func() tea.Msg {
		content := v.input.Value()
		if content == "" || v.focusedAgent == "" || v.streamManager == nil {
			return nil
		}

		// Send steering message via StreamManager
		err := v.streamManager.SendSteering(v.focusedAgent, content, nil)
		if err != nil {
			v.appendOutput(OutputLine{
				Text:      fmt.Sprintf("Error sending steering: %v", err),
				Style:     StyleError,
				Timestamp: time.Now(),
			})
			return nil
		}

		// Echo the steering message
		v.appendOutput(OutputLine{
			Text:      fmt.Sprintf("[STEER] %s", content),
			Style:     StyleInfo,
			Timestamp: time.Now(),
		})

		return nil
	}
}

// SendInterrupt sends an interrupt command to the currently focused agent.
func (v *AgentFocusView) SendInterrupt() tea.Cmd {
	return func() tea.Msg {
		if v.focusedAgent == "" || v.streamManager == nil {
			return nil
		}

		err := v.streamManager.SendInterrupt(v.focusedAgent, "User interrupt")
		if err != nil {
			v.appendOutput(OutputLine{
				Text:      fmt.Sprintf("Error sending interrupt: %v", err),
				Style:     StyleError,
				Timestamp: time.Now(),
			})
			return nil
		}

		v.appendOutput(OutputLine{
			Text:      "[INTERRUPT] Sent interrupt signal",
			Style:     StyleInfo,
			Timestamp: time.Now(),
		})

		return nil
	}
}

// refreshSessions reloads the list of active sessions from StreamManager.
func (v *AgentFocusView) refreshSessions() {
	if v.streamManager == nil {
		v.sessions = []database.AgentSession{}
		v.agentList.sessions = v.sessions
		return
	}

	v.sessions = v.streamManager.ListActiveSessions()
	v.agentList.sessions = v.sessions

	// If no agent is focused and there are sessions, focus the first one
	if v.focusedAgent == "" && len(v.sessions) > 0 {
		v.FocusAgent(v.sessions[0].AgentName)
	}
}

// getFocusedSession returns the session for the currently focused agent.
func (v *AgentFocusView) getFocusedSession() *database.AgentSession {
	for _, session := range v.sessions {
		if session.AgentName == v.focusedAgent {
			return &session
		}
	}
	return nil
}

// getCurrentSessionID returns the session ID for the focused agent.
func (v *AgentFocusView) getCurrentSessionID() string {
	session := v.getFocusedSession()
	if session != nil {
		return session.ID.String()
	}
	return ""
}

// appendStreamEvent processes a stream event and adds it to the output.
func (v *AgentFocusView) appendStreamEvent(event *database.StreamEvent) {
	var line OutputLine
	line.Timestamp = event.Timestamp

	switch event.EventType {
	case database.StreamEventOutput:
		// Parse output chunk
		var data struct {
			Content     string `json:"content"`
			IsReasoning bool   `json:"is_reasoning"`
		}
		if err := json.Unmarshal(event.Content, &data); err == nil {
			if data.IsReasoning {
				line.Text = fmt.Sprintf("[REASONING] %s", data.Content)
				line.Style = StyleOutput
			} else {
				line.Text = data.Content
				line.Style = StyleNormal
			}
		}

	case database.StreamEventToolCall:
		// Parse tool call
		var data struct {
			ToolName  string `json:"tool_name"`
			InputJSON string `json:"input_json"`
			CallID    string `json:"call_id"`
		}
		if err := json.Unmarshal(event.Content, &data); err == nil {
			line.Text = fmt.Sprintf("[TOOL CALL] %s (id: %s)", data.ToolName, data.CallID)
			line.Style = StyleToolCall
		}

	case database.StreamEventToolResult:
		// Parse tool result
		var data struct {
			CallID     string `json:"call_id"`
			OutputJSON string `json:"output_json"`
			Success    bool   `json:"success"`
		}
		if err := json.Unmarshal(event.Content, &data); err == nil {
			status := "success"
			if !data.Success {
				status = "failed"
			}
			line.Text = fmt.Sprintf("[TOOL RESULT] %s (id: %s)", status, data.CallID)
			line.Style = StyleToolResult
		}

	case database.StreamEventFinding:
		// Parse finding
		var data struct {
			FindingJSON string `json:"finding_json"`
		}
		if err := json.Unmarshal(event.Content, &data); err == nil {
			line.Text = fmt.Sprintf("[FINDING] %s", data.FindingJSON)
			line.Style = StyleFinding
		}

	case database.StreamEventStatus:
		// Parse status change
		var data struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(event.Content, &data); err == nil {
			line.Text = fmt.Sprintf("[STATUS] %s: %s", data.Status, data.Message)
			line.Style = StyleStatus
		}

	case database.StreamEventSteeringAck:
		// Parse steering acknowledgment
		var data struct {
			MessageID string `json:"message_id"`
			Response  string `json:"response"`
		}
		if err := json.Unmarshal(event.Content, &data); err == nil {
			line.Text = fmt.Sprintf("[ACK] %s", data.Response)
			line.Style = StyleSteeringAck
		}

	case database.StreamEventError:
		// Parse error
		var data struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Fatal   bool   `json:"fatal"`
		}
		if err := json.Unmarshal(event.Content, &data); err == nil {
			fatalStr := ""
			if data.Fatal {
				fatalStr = " [FATAL]"
			}
			line.Text = fmt.Sprintf("[ERROR%s] %s: %s", fatalStr, data.Code, data.Message)
			line.Style = StyleError
		}

	default:
		line.Text = fmt.Sprintf("[%s] %s", event.EventType, string(event.Content))
		line.Style = StyleNormal
	}

	v.appendOutput(line)
}

// appendOutput adds a line to the output buffer.
func (v *AgentFocusView) appendOutput(line OutputLine) {
	v.outputLines = append(v.outputLines, line)

	// Trim to maxLines if exceeded
	if len(v.outputLines) > v.maxLines {
		excess := len(v.outputLines) - v.maxLines
		v.outputLines = v.outputLines[excess:]
	}

	// Refresh viewport content
	v.refreshOutputContent()

	// Auto-scroll to bottom
	v.outputView.GotoBottom()
}

// refreshOutputContent updates the viewport content with styled output lines.
func (v *AgentFocusView) refreshOutputContent() {
	var lines []string

	for _, outputLine := range v.outputLines {
		line := v.styleOutputLine(outputLine)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	v.outputView.SetContent(content)
}

// styleOutputLine applies lipgloss styling to an output line based on its style.
func (v *AgentFocusView) styleOutputLine(line OutputLine) string {
	var style lipgloss.Style

	switch line.Style {
	case StyleOutput:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D4FF"))

	case StyleToolCall:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFB000")).
			Bold(true)

	case StyleToolResult:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#805800"))

	case StyleFinding:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD966")).
			Bold(true)

	case StyleStatus:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00A6FF"))

	case StyleSteeringAck:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FFB0"))

	case StyleError:
		style = lipgloss.NewStyle().
			Foreground(v.theme.Danger).
			Bold(true)

	case StyleInfo:
		style = lipgloss.NewStyle().
			Foreground(v.theme.Warning)

	case StyleNormal:
		fallthrough
	default:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))
	}

	return style.Render(line.Text)
}

// renderHeader renders the header with focused agent information.
func (v *AgentFocusView) renderHeader(session *database.AgentSession) string {
	statusStyle := v.theme.StatusStyle(string(session.Status))
	modeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB000"))

	parts := []string{
		v.theme.TitleStyle.Render(fmt.Sprintf("Agent: %s", session.AgentName)),
		statusStyle.Render(fmt.Sprintf(" [%s]", session.Status)),
		modeStyle.Render(fmt.Sprintf(" <%s>", session.Mode)),
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

// waitForEvent waits for the next stream event from the event channel.
func (v *AgentFocusView) waitForEvent() tea.Cmd {
	if v.eventCh == nil {
		return nil
	}

	return func() tea.Msg {
		event, ok := <-v.eventCh
		if !ok {
			// Channel closed
			return nil
		}
		return StreamEventMsg{Event: event}
	}
}

// StreamEventMsg wraps a stream event for the Bubble Tea message loop.
type StreamEventMsg struct {
	Event *database.StreamEvent
}

// RefreshSessionsMsg signals that sessions should be refreshed.
type RefreshSessionsMsg struct{}

// AgentListSidebar implementation

// View renders the agent list sidebar.
func (s *AgentListSidebar) View() string {
	if len(s.sessions) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#805800")).
			Width(s.width).
			Height(s.height).
			Padding(1)
		return emptyStyle.Render("No active agents")
	}

	var lines []string
	lines = append(lines, s.theme.TitleStyle.Render("Active Agents"))
	lines = append(lines, "")

	for i, session := range s.sessions {
		var line string

		// Status indicator
		statusIndicator := s.getStatusIndicator(session.Status)

		// Agent name
		agentName := session.AgentName
		if len(agentName) > 15 {
			agentName = agentName[:12] + "..."
		}

		// Build line
		line = fmt.Sprintf("%s %s", statusIndicator, agentName)

		// Highlight selected
		if i == s.selectedIndex {
			lineStyle := lipgloss.NewStyle().
				Foreground(s.theme.Primary).
				Bold(true).
				Reverse(true)
			line = lineStyle.Render(line)
		} else {
			lineStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF"))
			line = lineStyle.Render(line)
		}

		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")

	// Apply sidebar border
	sidebarStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.theme.Muted).
		Width(s.width - 2).
		Height(s.height - 2).
		Padding(0, 1)

	return sidebarStyle.Render(content)
}

// getStatusIndicator returns a colored indicator for agent status.
func (s *AgentListSidebar) getStatusIndicator(status database.AgentStatus) string {
	var indicator string
	var color lipgloss.Color

	switch status {
	case database.AgentStatusRunning:
		indicator = "●"
		color = lipgloss.Color("#00FF00")
	case database.AgentStatusPaused:
		indicator = "◐"
		color = lipgloss.Color("#FFB000")
	case database.AgentStatusWaitingInput:
		indicator = "◔"
		color = lipgloss.Color("#FFD966")
	case database.AgentStatusInterrupted:
		indicator = "■"
		color = lipgloss.Color("#FF6600")
	case database.AgentStatusCompleted:
		indicator = "✓"
		color = lipgloss.Color("#00FFB0")
	case database.AgentStatusFailed:
		indicator = "✗"
		color = lipgloss.Color("#FF0000")
	default:
		indicator = "○"
		color = lipgloss.Color("#888888")
	}

	style := lipgloss.NewStyle().Foreground(color)
	return style.Render(indicator)
}
