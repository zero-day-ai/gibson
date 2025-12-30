package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/tui/console"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// ConsoleConfig holds all dependencies and configuration needed for the console view.
type ConsoleConfig struct {
	// DB is the database connection for data persistence operations.
	DB *database.DB
	// ComponentRegistry manages agents, tools, and plugins.
	ComponentRegistry component.ComponentRegistry
	// FindingStore provides access to security findings.
	FindingStore finding.FindingStore
	// HomeDir is the Gibson home directory path (e.g., ~/.gibson).
	HomeDir string
	// ConfigFile is the path to the Gibson configuration file.
	ConfigFile string
}

// ConsoleView implements a terminal-like console interface for the Gibson TUI.
// It provides command execution, history navigation, tab completion, and scrollable output.
// Commands are entered at the bottom prompt and output scrolls upward in the viewport.
type ConsoleView struct {
	// Context for command execution and cancellation
	ctx context.Context

	// UI components
	input  textinput.Model // Command input field at the bottom
	output viewport.Model  // Scrollable output area showing command history and results
	theme  *styles.Theme   // Color scheme and styling

	// Console functionality
	history   *console.History         // Command history for up/down navigation
	executor  *console.Executor        // Executes parsed slash commands
	completer *console.Completer       // Provides tab completion suggestions
	registry  *console.CommandRegistry // Registry of available slash commands

	// Output buffer - ring buffer to limit memory usage
	outputLines []console.OutputLine // Stores output lines with styling
	maxLines    int                  // Maximum number of lines to retain (default: 1000)

	// Suggestion state
	showSuggestions bool                 // Whether to display the suggestion popup
	suggestions     []console.Suggestion // Current completion suggestions
	suggestionIndex int                  // Currently selected suggestion (for cycling)

	// Dimensions
	width  int
	height int

	// State tracking
	lastInput         string // Track last input to detect changes for suggestions
	historyNavigating bool   // Whether currently navigating history
}

const (
	// DefaultMaxLines is the default maximum number of output lines to retain
	DefaultMaxLines = 1000
	// DefaultHistorySize is the default maximum number of history entries
	DefaultHistorySize = 100
	// PromptText is the console prompt prefix
	PromptText = "gibson> "
)

// NewConsoleView creates a new console view with the provided configuration.
// It initializes all sub-components including input field, output viewport,
// command registry, history, completer, and executor.
func NewConsoleView(ctx context.Context, config ConsoleConfig) *ConsoleView {
	// Initialize text input for command entry
	ti := textinput.New()
	ti.Placeholder = "Type a command... (try /help)"
	ti.CharLimit = 500
	ti.Width = 80

	// Apply prompt styling
	ti.Prompt = PromptText
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

	// Initialize command registry with default commands
	registry := console.NewCommandRegistry()
	console.RegisterDefaultCommands(registry)

	// Initialize history with default size
	history := console.NewHistory(DefaultHistorySize)

	// Initialize completer
	completer := console.NewCompleter(registry)

	// Initialize executor with configuration
	executorConfig := console.ExecutorConfig{
		DB:                config.DB,
		ComponentRegistry: config.ComponentRegistry,
		FindingStore:      config.FindingStore,
		HomeDir:           config.HomeDir,
		ConfigFile:        config.ConfigFile,
	}
	executor := console.NewExecutor(ctx, registry, executorConfig)

	// Set up command handlers
	executor.SetupHandlers()

	view := &ConsoleView{
		ctx:               ctx,
		input:             ti,
		output:            vp,
		theme:             styles.DefaultTheme(),
		history:           history,
		executor:          executor,
		completer:         completer,
		registry:          registry,
		outputLines:       make([]console.OutputLine, 0, DefaultMaxLines),
		maxLines:          DefaultMaxLines,
		showSuggestions:   false,
		suggestions:       []console.Suggestion{},
		suggestionIndex:   0,
		width:             80,
		height:            24,
		lastInput:         "",
		historyNavigating: false,
	}

	// Add welcome message
	view.appendOutput(console.OutputLine{
		Text:      "Gibson Console",
		Style:     console.StyleInfo,
		Timestamp: time.Now(),
	})
	view.appendOutput(console.OutputLine{
		Text:      "Type /help to see available commands",
		Style:     console.StyleNormal,
		Timestamp: time.Now(),
	})
	view.appendOutput(console.OutputLine{
		Text:      "",
		Style:     console.StyleNormal,
		Timestamp: time.Now(),
	})

	return view
}

// Init initializes the console view and returns the initial command.
// Note: Input focus is managed by Focus()/Blur() methods, not in Init().
func (c *ConsoleView) Init() tea.Cmd {
	return textinput.Blink
}

// Focus focuses the text input field when the console view becomes active.
func (c *ConsoleView) Focus() tea.Cmd {
	c.input.Focus()
	return textinput.Blink
}

// Blur blurs the text input field when the console view becomes inactive.
func (c *ConsoleView) Blur() {
	c.input.Blur()
}

// Update handles messages and updates the console view state.
// It processes keyboard input, window resize events, and forwards updates to child components.
func (c *ConsoleView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		c.updateSizes()
		return c, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Execute command
			cmd := c.input.Value()
			if cmd != "" {
				c.executeCommand(cmd)
				c.history.Add(cmd)
				c.input.SetValue("")
				c.lastInput = ""
				c.showSuggestions = false
				c.historyNavigating = false
			}
			return c, nil

		case "up":
			// Navigate history backward
			if cmd, ok := c.history.Previous(); ok {
				c.input.SetValue(cmd)
				c.input.CursorEnd()
				c.lastInput = cmd
				c.historyNavigating = true
				c.showSuggestions = false
			}
			return c, nil

		case "down":
			// Navigate history forward
			if cmd, ok := c.history.Next(); ok {
				c.input.SetValue(cmd)
				c.input.CursorEnd()
				c.lastInput = cmd
			} else {
				// Reached end of history, clear input
				c.input.SetValue("")
				c.lastInput = ""
				c.historyNavigating = false
			}
			c.showSuggestions = false
			return c, nil

		case "tab":
			// Handle tab completion
			if c.showSuggestions && len(c.suggestions) > 0 {
				// Cycle through suggestions
				c.suggestionIndex = (c.suggestionIndex + 1) % len(c.suggestions)
				// Apply selected suggestion
				c.input.SetValue(c.suggestions[c.suggestionIndex].Text + " ")
				c.input.CursorEnd()
				c.lastInput = c.input.Value()
				c.showSuggestions = false
			} else {
				// Show suggestions
				c.updateSuggestions()
				if len(c.suggestions) > 0 {
					c.showSuggestions = true
					c.suggestionIndex = 0
					// Auto-complete if only one suggestion
					if len(c.suggestions) == 1 {
						c.input.SetValue(c.suggestions[0].Text + " ")
						c.input.CursorEnd()
						c.lastInput = c.input.Value()
						c.showSuggestions = false
					}
				}
			}
			return c, nil

		case "ctrl+l":
			// Clear output
			c.clearOutput()
			return c, nil

		case "ctrl+u":
			// Clear current input line
			c.input.SetValue("")
			c.lastInput = ""
			c.showSuggestions = false
			c.historyNavigating = false
			return c, nil

		case "ctrl+w":
			// Delete word before cursor
			val := c.input.Value()
			pos := c.input.Position()
			if pos > 0 {
				// Find start of word
				start := pos - 1
				for start > 0 && val[start-1] != ' ' {
					start--
				}
				// Delete from start to position
				newVal := val[:start] + val[pos:]
				c.input.SetValue(newVal)
				// Move cursor to start
				for i := 0; i < start; i++ {
					c.input.CursorStart()
				}
				for i := 0; i < start; i++ {
					c.input.CursorEnd()
				}
				c.input.SetCursor(start)
				c.lastInput = newVal
			}
			return c, nil

		case "ctrl+c":
			// Clear input
			c.input.SetValue("")
			c.lastInput = ""
			c.showSuggestions = false
			return c, nil

		case "pgup":
			// Scroll output viewport up
			c.output.ViewUp()
			return c, nil

		case "pgdown":
			// Scroll output viewport down
			c.output.ViewDown()
			return c, nil

		case "ctrl+v":
			// Paste from clipboard
			if text := c.pasteFromClipboard(); text != "" {
				// Insert clipboard content at cursor position
				val := c.input.Value()
				pos := c.input.Position()
				newVal := val[:pos] + text + val[pos:]
				c.input.SetValue(newVal)
				// Move cursor to after pasted text
				c.input.SetCursor(pos + len(text))
				c.lastInput = newVal
				c.showSuggestions = false
			}
			return c, nil

		case "esc":
			// Hide suggestions and reset history navigation
			c.showSuggestions = false
			c.historyNavigating = false
			return c, nil
		}
	}

	// Forward to input and check if value changed
	oldValue := c.input.Value()
	var inputCmd tea.Cmd
	c.input, inputCmd = c.input.Update(msg)
	cmds = append(cmds, inputCmd)

	// Update suggestions if input changed and not navigating history
	if !c.historyNavigating && c.input.Value() != oldValue {
		c.lastInput = c.input.Value()
		c.updateSuggestions()
		// Auto-show suggestions for slash commands
		if strings.HasPrefix(c.input.Value(), "/") {
			c.showSuggestions = len(c.suggestions) > 0
		}
	}

	// Forward to output viewport
	var outputCmd tea.Cmd
	c.output, outputCmd = c.output.Update(msg)
	cmds = append(cmds, outputCmd)

	return c, tea.Batch(cmds...)
}

// View renders the console view with output viewport, suggestions, and input prompt.
// The layout is:
//  1. Output viewport (scrollable command history and results)
//  2. Suggestion popup (if showSuggestions is true)
//  3. Input prompt at the bottom
func (c *ConsoleView) View() string {
	if c.width == 0 || c.height == 0 {
		return "Loading console..."
	}

	var sections []string

	// Render output viewport
	sections = append(sections, c.output.View())

	// Render suggestions if visible
	if c.showSuggestions && len(c.suggestions) > 0 {
		sections = append(sections, c.renderSuggestions())
	}

	// Render input line
	sections = append(sections, c.input.View())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// SetSize updates the dimensions of the console view and child components.
func (c *ConsoleView) SetSize(width, height int) {
	c.width = width
	c.height = height
	c.updateSizes()
}

// updateSizes recalculates and applies sizes to child components based on terminal dimensions.
func (c *ConsoleView) updateSizes() {
	// Calculate heights
	inputHeight := 1
	suggestionsHeight := 0
	if c.showSuggestions {
		suggestionsHeight = min(len(c.suggestions)+2, 8) // Max 8 lines for suggestions
	}
	outputHeight := c.height - inputHeight - suggestionsHeight - 4 // Account for borders and padding

	if outputHeight < 5 {
		outputHeight = 5
	}

	// Update component sizes
	c.input.Width = c.width - len(PromptText) - 2
	c.output.Width = c.width - 4 // Account for border and padding
	c.output.Height = outputHeight

	// Update output viewport content
	c.refreshOutputContent()
}

// executeCommand parses and executes a command, appending the results to output.
func (c *ConsoleView) executeCommand(cmd string) {
	// Echo the command
	c.appendCommandEcho(cmd)

	// Check if it's a slash command
	if !console.IsSlashCommand(cmd) {
		c.appendOutput(console.OutputLine{
			Text:      "Commands must start with /. Try /help for available commands.",
			Style:     console.StyleError,
			Timestamp: time.Now(),
		})
		return
	}

	// Execute the command
	result, err := c.executor.Execute(cmd)
	if err != nil {
		c.appendOutput(console.OutputLine{
			Text:      fmt.Sprintf("Execution error: %v", err),
			Style:     console.StyleError,
			Timestamp: time.Now(),
		})
		return
	}

	// Handle special case for /clear command
	if result.Output == "" && !result.IsError && strings.TrimSpace(cmd) == "/clear" {
		c.clearOutput()
		return
	}

	// Append output
	if result.IsError && result.Error != "" {
		for _, line := range strings.Split(result.Error, "\n") {
			c.appendOutput(console.OutputLine{
				Text:      line,
				Style:     console.StyleError,
				Timestamp: time.Now(),
			})
		}
	}

	if result.Output != "" {
		for _, line := range strings.Split(result.Output, "\n") {
			style := console.StyleNormal
			// Detect success/info patterns in output
			if strings.Contains(strings.ToLower(line), "success") ||
				strings.Contains(strings.ToLower(line), "completed") {
				style = console.StyleSuccess
			} else if strings.Contains(line, "ERROR") ||
				strings.Contains(line, "Failed") {
				style = console.StyleError
			}
			c.appendOutput(console.OutputLine{
				Text:      line,
				Style:     style,
				Timestamp: time.Now(),
			})
		}
	}

	// Append blank line for readability
	c.appendOutput(console.OutputLine{
		Text:      "",
		Style:     console.StyleNormal,
		Timestamp: time.Now(),
	})
}

// appendCommandEcho adds a styled echo of the executed command to output.
func (c *ConsoleView) appendCommandEcho(cmd string) {
	c.appendOutput(console.OutputLine{
		Text:      PromptText + cmd,
		Style:     console.StyleCommand,
		Timestamp: time.Now(),
	})
}

// appendOutput adds a line to the output ring buffer.
// If the buffer exceeds maxLines, oldest lines are discarded.
func (c *ConsoleView) appendOutput(line console.OutputLine) {
	c.outputLines = append(c.outputLines, line)

	// Trim to maxLines if exceeded
	if len(c.outputLines) > c.maxLines {
		excess := len(c.outputLines) - c.maxLines
		c.outputLines = c.outputLines[excess:]
	}

	// Refresh viewport content
	c.refreshOutputContent()

	// Auto-scroll to bottom
	c.output.GotoBottom()
}

// clearOutput removes all output lines and shows a clear indicator.
func (c *ConsoleView) clearOutput() {
	c.outputLines = make([]console.OutputLine, 0, c.maxLines)
	c.appendOutput(console.OutputLine{
		Text:      "[Console cleared]",
		Style:     console.StyleInfo,
		Timestamp: time.Now(),
	})
	c.appendOutput(console.OutputLine{
		Text:      "",
		Style:     console.StyleNormal,
		Timestamp: time.Now(),
	})
}

// updateSuggestions updates the suggestion list based on current input.
func (c *ConsoleView) updateSuggestions() {
	input := c.input.Value()
	c.suggestions = c.completer.GetSuggestions(input)
	c.suggestionIndex = 0
}

// renderSuggestions renders the suggestion popup box above the input line.
func (c *ConsoleView) renderSuggestions() string {
	if len(c.suggestions) == 0 {
		return ""
	}

	var lines []string
	maxToShow := min(len(c.suggestions), 6) // Show max 6 suggestions

	for i := 0; i < maxToShow; i++ {
		suggestion := c.suggestions[i]
		line := fmt.Sprintf("  %s", suggestion.Text)
		if suggestion.Description != "" {
			line += fmt.Sprintf(" - %s", suggestion.Description)
		}

		// Highlight selected suggestion
		if i == c.suggestionIndex {
			style := lipgloss.NewStyle().
				Foreground(c.theme.Primary).
				Bold(true)
			line = style.Render(line)
		} else {
			style := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888"))
			line = style.Render(line)
		}

		lines = append(lines, line)
	}

	if len(c.suggestions) > maxToShow {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(c.theme.Muted).
			Render(fmt.Sprintf("  ... and %d more (press tab to cycle)", len(c.suggestions)-maxToShow)))
	}

	content := strings.Join(lines, "\n")

	// Apply box styling
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c.theme.Primary).
		Padding(0, 1).
		Width(c.width - 4)

	return boxStyle.Render(content)
}

// refreshOutputContent updates the viewport content with styled output lines.
func (c *ConsoleView) refreshOutputContent() {
	var lines []string

	for _, outputLine := range c.outputLines {
		line := c.styleOutputLine(outputLine)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	c.output.SetContent(content)
}

// styleOutputLine applies lipgloss styling to an output line based on its style.
func (c *ConsoleView) styleOutputLine(line console.OutputLine) string {
	var style lipgloss.Style

	switch line.Style {
	case console.StyleCommand:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00A6FF")).
			Bold(true)

	case console.StyleError:
		style = lipgloss.NewStyle().
			Foreground(c.theme.Danger).
			Bold(true)

	case console.StyleSuccess:
		style = lipgloss.NewStyle().
			Foreground(c.theme.Success)

	case console.StyleInfo:
		style = lipgloss.NewStyle().
			Foreground(c.theme.Warning)

	case console.StyleNormal:
		fallthrough
	default:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))
	}

	return style.Render(line.Text)
}

// pasteFromClipboard reads text from the system clipboard.
// Returns empty string on error. For multiline content, only the first line is returned.
func (c *ConsoleView) pasteFromClipboard() string {
	text, err := clipboard.ReadAll()
	if err != nil {
		// Silently fail - clipboard may not be available in all environments
		return ""
	}

	// Only use the first line for single-line input
	if idx := strings.IndexAny(text, "\n\r"); idx >= 0 {
		text = text[:idx]
	}

	return text
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
