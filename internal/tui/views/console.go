package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// LineType represents the type of a console line
type LineType int

const (
	LineTypeUser LineType = iota
	LineTypeAgent
	LineTypeSystem
	LineTypeError
)

// String returns the string representation of the line type
func (lt LineType) String() string {
	switch lt {
	case LineTypeUser:
		return "user"
	case LineTypeAgent:
		return "agent"
	case LineTypeSystem:
		return "system"
	case LineTypeError:
		return "error"
	default:
		return "unknown"
	}
}

// ConsoleLine represents a single line in the console history
type ConsoleLine struct {
	Type      LineType
	Content   string
	Agent     string // Agent name for agent responses
	Timestamp time.Time
	Findings  []agent.Finding // Findings attached to this line
}

// ConsoleView represents the console view for agent interaction
type ConsoleView struct {
	ctx          context.Context
	registry     agent.AgentRegistry
	viewport     viewport.Model
	input        textinput.Model
	messages     []ConsoleLine
	activeAgent  string
	history      []string // Command history
	historyIndex int      // Current position in history
	width        int
	height       int
	ready        bool
}

// NewConsoleView creates a new console view
func NewConsoleView(ctx context.Context, registry agent.AgentRegistry) *ConsoleView {
	ti := textinput.New()
	ti.Placeholder = "Type a message or /help for commands..."
	ti.Focus()
	ti.CharLimit = 1000
	ti.Width = 80

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62"))

	cv := &ConsoleView{
		ctx:          ctx,
		registry:     registry,
		viewport:     vp,
		input:        ti,
		messages:     []ConsoleLine{},
		activeAgent:  "",
		history:      []string{},
		historyIndex: -1,
		width:        80,
		height:       24,
		ready:        false,
	}

	// Add welcome message
	cv.addSystemMessage("Welcome to Gibson Console! Type /help for available commands.")
	cv.addSystemMessage("Use @agentname to switch active agent, or /agents to list available agents.")

	return cv
}

// Init initializes the console view
func (cv *ConsoleView) Init() tea.Cmd {
	cv.ready = true
	return textinput.Blink
}

// Update handles messages and updates the console view
func (cv *ConsoleView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return cv, tea.Quit

		case "enter":
			// Process the input
			value := cv.input.Value()
			if value != "" {
				cv.processInput(value)
				cv.input.SetValue("")

				// Add to history
				cv.history = append(cv.history, value)
				cv.historyIndex = len(cv.history)

				// Scroll to bottom
				cv.viewport.GotoBottom()
			}
			return cv, nil

		case "up":
			// Navigate command history backwards
			if len(cv.history) > 0 && cv.historyIndex > 0 {
				cv.historyIndex--
				cv.input.SetValue(cv.history[cv.historyIndex])
				cv.input.CursorEnd()
			}
			return cv, nil

		case "down":
			// Navigate command history forwards
			if len(cv.history) > 0 && cv.historyIndex < len(cv.history)-1 {
				cv.historyIndex++
				cv.input.SetValue(cv.history[cv.historyIndex])
				cv.input.CursorEnd()
			} else if cv.historyIndex == len(cv.history)-1 {
				cv.historyIndex = len(cv.history)
				cv.input.SetValue("")
			}
			return cv, nil

		case "ctrl+l":
			// Clear console
			cv.messages = []ConsoleLine{}
			cv.addSystemMessage("Console cleared.")
			cv.viewport.GotoBottom()
			return cv, nil

		case "ctrl+u":
			// Clear current line
			cv.input.SetValue("")
			return cv, nil

		case "pgup", "pgdown":
			// Let viewport handle page up/down
			cv.viewport, cmd = cv.viewport.Update(msg)
			return cv, cmd
		}

	case tea.WindowSizeMsg:
		cv.SetSize(msg.Width, msg.Height)
		return cv, nil

	case AgentResponseMsg:
		// Handle agent response
		cv.addAgentMessage(msg.Agent, msg.Content, msg.Findings)
		cv.viewport.GotoBottom()
		return cv, nil
	}

	// Update input
	cv.input, cmd = cv.input.Update(msg)
	cmds = append(cmds, cmd)

	// Update viewport
	cv.viewport, cmd = cv.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return cv, tea.Batch(cmds...)
}

// View renders the console view
func (cv *ConsoleView) View() string {
	if !cv.ready {
		return "Initializing console..."
	}

	// Update viewport content
	cv.viewport.SetContent(cv.renderMessages())

	// Build the view
	var sb strings.Builder

	// Render viewport (history)
	sb.WriteString(cv.viewport.View())
	sb.WriteString("\n")

	// Render input area
	inputStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1)

	prompt := "> "
	if cv.activeAgent != "" {
		prompt = fmt.Sprintf("[%s] > ", cv.activeAgent)
	}

	inputView := inputStyle.Render(prompt + cv.input.View())
	sb.WriteString(inputView)

	return sb.String()
}

// SetSize sets the console view dimensions
func (cv *ConsoleView) SetSize(width, height int) {
	cv.width = width
	cv.height = height

	// Reserve space for input area (3 lines with borders)
	viewportHeight := height - 5
	if viewportHeight < 5 {
		viewportHeight = 5
	}

	cv.viewport.Width = width - 4
	cv.viewport.Height = viewportHeight
	cv.input.Width = width - 10
}

// processInput processes user input and handles commands
func (cv *ConsoleView) processInput(input string) {
	// Add user message to history
	cv.addUserMessage(input)

	// Check for @agent switching
	if strings.HasPrefix(input, "@") {
		parts := strings.Fields(input)
		if len(parts) > 0 {
			agentName := strings.TrimPrefix(parts[0], "@")
			cv.switchAgent(agentName)
			return
		}
	}

	// Check for slash commands
	if strings.HasPrefix(input, "/") {
		cv.handleSlashCommand(input)
		return
	}

	// Send to active agent
	if cv.activeAgent != "" {
		cv.sendMessageToAgent(input)
	} else {
		cv.addSystemMessage("No active agent. Use @agentname to select an agent, or /agents to list available agents.")
	}
}

// handleSlashCommand processes slash commands
func (cv *ConsoleView) handleSlashCommand(input string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return
	}

	command := strings.TrimPrefix(parts[0], "/")
	args := parts[1:]

	switch command {
	case "help":
		cv.showHelp()

	case "agents":
		cv.listAgents()

	case "clear":
		cv.messages = []ConsoleLine{}
		cv.addSystemMessage("Console cleared.")

	case "attack":
		if len(args) > 0 {
			target := strings.Join(args, " ")
			cv.addSystemMessage(fmt.Sprintf("Attack command: target=%s", target))
			if cv.activeAgent != "" {
				cv.sendMessageToAgent(fmt.Sprintf("attack %s", target))
			} else {
				cv.addSystemMessage("No active agent selected.")
			}
		} else {
			cv.addSystemMessage("Usage: /attack <target>")
		}

	case "target":
		if len(args) > 0 {
			target := strings.Join(args, " ")
			cv.addSystemMessage(fmt.Sprintf("Target set: %s", target))
			if cv.activeAgent != "" {
				cv.sendMessageToAgent(fmt.Sprintf("set target %s", target))
			}
		} else {
			cv.addSystemMessage("Usage: /target <target>")
		}

	default:
		cv.addSystemMessage(fmt.Sprintf("Unknown command: %s. Type /help for available commands.", command))
	}
}

// showHelp displays available commands
func (cv *ConsoleView) showHelp() {
	help := []string{
		"Available Commands:",
		"  /help           - Show this help message",
		"  /agents         - List available agents",
		"  /attack <target> - Execute attack on target",
		"  /target <target> - Set current target",
		"  /clear          - Clear console history",
		"",
		"Agent Selection:",
		"  @agentname      - Switch to agent",
		"",
		"Keyboard Shortcuts:",
		"  Ctrl+L          - Clear console",
		"  Ctrl+U          - Clear current line",
		"  Up/Down         - Navigate command history",
		"  PgUp/PgDown     - Scroll console history",
	}

	for _, line := range help {
		cv.addSystemMessage(line)
	}
}

// listAgents lists all available agents
func (cv *ConsoleView) listAgents() {
	if cv.registry == nil {
		cv.addSystemMessage("Agent registry not available.")
		return
	}

	descriptors := cv.registry.List()
	if len(descriptors) == 0 {
		cv.addSystemMessage("No agents registered.")
		return
	}

	cv.addSystemMessage("Available Agents:")
	for _, desc := range descriptors {
		status := "internal"
		if desc.IsExternal {
			status = "external"
		}
		cv.addSystemMessage(fmt.Sprintf("  @%s - %s (%s)", desc.Name, desc.Description, status))
	}

	// Show running agents
	running := cv.registry.RunningAgents()
	if len(running) > 0 {
		cv.addSystemMessage("")
		cv.addSystemMessage("Running Agents:")
		for _, rt := range running {
			cv.addSystemMessage(fmt.Sprintf("  %s - Status: %s", rt.AgentName, rt.Status))
		}
	}
}

// switchAgent switches the active agent
func (cv *ConsoleView) switchAgent(agentName string) {
	if cv.registry == nil {
		cv.addSystemMessage("Agent registry not available.")
		return
	}

	// Check if agent exists
	_, err := cv.registry.GetDescriptor(agentName)
	if err != nil {
		cv.addSystemMessage(fmt.Sprintf("Agent '%s' not found. Use /agents to list available agents.", agentName))
		return
	}

	cv.activeAgent = agentName
	cv.addSystemMessage(fmt.Sprintf("Switched to agent: %s", agentName))
}

// sendMessageToAgent sends a message to the active agent
func (cv *ConsoleView) sendMessageToAgent(message string) {
	if cv.activeAgent == "" {
		cv.addSystemMessage("No active agent selected.")
		return
	}

	// For now, just show a placeholder response
	// In a real implementation, this would create a task and delegate to the agent
	cv.addSystemMessage(fmt.Sprintf("Sending to %s: %s", cv.activeAgent, message))

	// TODO: Implement actual agent communication
	// This would involve:
	// 1. Creating a Task
	// 2. Calling registry.DelegateToAgent
	// 3. Handling the response asynchronously
	// 4. Emitting AgentResponseMsg when complete

	// For now, simulate a response
	go func() {
		time.Sleep(500 * time.Millisecond)
		// In a real implementation, this would be sent via tea.Cmd
		cv.addAgentMessage(cv.activeAgent,
			"This is a simulated response. Agent integration will be completed in the full implementation.",
			nil)
	}()
}

// SendMessage sends a message to the active agent (external API)
func (cv *ConsoleView) SendMessage(message string) {
	cv.processInput(message)
}

// addUserMessage adds a user message to the console
func (cv *ConsoleView) addUserMessage(content string) {
	line := ConsoleLine{
		Type:      LineTypeUser,
		Content:   content,
		Timestamp: time.Now(),
	}
	cv.messages = append(cv.messages, line)
}

// addAgentMessage adds an agent response to the console
func (cv *ConsoleView) addAgentMessage(agentName, content string, findings []agent.Finding) {
	line := ConsoleLine{
		Type:      LineTypeAgent,
		Content:   content,
		Agent:     agentName,
		Timestamp: time.Now(),
		Findings:  findings,
	}
	cv.messages = append(cv.messages, line)
}

// addSystemMessage adds a system message to the console
func (cv *ConsoleView) addSystemMessage(content string) {
	line := ConsoleLine{
		Type:      LineTypeSystem,
		Content:   content,
		Timestamp: time.Now(),
	}
	cv.messages = append(cv.messages, line)
}

// addErrorMessage adds an error message to the console
func (cv *ConsoleView) addErrorMessage(content string) {
	line := ConsoleLine{
		Type:      LineTypeError,
		Content:   content,
		Timestamp: time.Now(),
	}
	cv.messages = append(cv.messages, line)
}

// renderMessages renders all messages for display in the viewport
func (cv *ConsoleView) renderMessages() string {
	var sb strings.Builder

	userStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true)

	agentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("213")).
		Bold(true)

	systemStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Italic(true)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true)

	timestampStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	findingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Italic(true)

	for _, msg := range cv.messages {
		timestamp := timestampStyle.Render(msg.Timestamp.Format("15:04:05"))

		switch msg.Type {
		case LineTypeUser:
			prefix := userStyle.Render("You")
			sb.WriteString(fmt.Sprintf("%s %s: %s\n", timestamp, prefix, msg.Content))

		case LineTypeAgent:
			prefix := agentStyle.Render(fmt.Sprintf("[%s]", msg.Agent))
			sb.WriteString(fmt.Sprintf("%s %s: %s\n", timestamp, prefix, msg.Content))

			// Display findings inline if present
			if len(msg.Findings) > 0 {
				for _, finding := range msg.Findings {
					severityColor := getSeverityColor(finding.Severity)
					severityStyle := lipgloss.NewStyle().
						Foreground(lipgloss.Color(severityColor)).
						Bold(true)

					findingPrefix := findingStyle.Render("  Finding")
					severity := severityStyle.Render(string(finding.Severity))
					sb.WriteString(fmt.Sprintf("%s: [%s] %s - %s\n",
						findingPrefix, severity, finding.Title, finding.Description))
				}
			}

		case LineTypeSystem:
			prefix := systemStyle.Render("System")
			sb.WriteString(fmt.Sprintf("%s %s: %s\n", timestamp, prefix, msg.Content))

		case LineTypeError:
			prefix := errorStyle.Render("Error")
			sb.WriteString(fmt.Sprintf("%s %s: %s\n", timestamp, prefix, msg.Content))
		}
	}

	return sb.String()
}

// getSeverityColor returns the color for a finding severity
func getSeverityColor(severity agent.FindingSeverity) string {
	switch severity {
	case agent.SeverityCritical:
		return "196" // Bright red
	case agent.SeverityHigh:
		return "208" // Orange
	case agent.SeverityMedium:
		return "226" // Yellow
	case agent.SeverityLow:
		return "117" // Light blue
	case agent.SeverityInfo:
		return "245" // Gray
	default:
		return "255" // White
	}
}

// AgentResponseMsg is sent when an agent responds
type AgentResponseMsg struct {
	Agent    string
	Content  string
	Findings []agent.Finding
}

// AgentDescriptor represents metadata about an agent
type AgentDescriptor struct {
	Name        string
	Version     string
	Description string
	IsExternal  bool
}

// NewAgentDescriptor creates a descriptor from an agent
func NewAgentDescriptor(a agent.Agent) AgentDescriptor {
	return AgentDescriptor{
		Name:        a.Name(),
		Version:     a.Version(),
		Description: a.Description(),
		IsExternal:  false,
	}
}

// AgentRuntime represents a running agent instance
type AgentRuntime struct {
	ID        types.ID
	AgentName string
	Status    string
	StartedAt time.Time
}

// NewAgentRuntime creates a new agent runtime
func NewAgentRuntime(agentName string, taskID types.ID) *AgentRuntime {
	return &AgentRuntime{
		ID:        types.NewID(),
		AgentName: agentName,
		Status:    "running",
		StartedAt: time.Now(),
	}
}
