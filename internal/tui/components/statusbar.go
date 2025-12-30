package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// StatusBar displays a status bar at the bottom of the screen.
// It shows the current mode on the left, status messages in the center,
// and key hints on the right.
type StatusBar struct {
	mode       string
	message    string
	keyHints   string
	width      int
	theme      *styles.Theme
	isError    bool
	leftStyle  lipgloss.Style
	rightStyle lipgloss.Style
}

// NewStatusBar creates a new status bar.
func NewStatusBar(width int) *StatusBar {
	theme := styles.DefaultTheme()
	return &StatusBar{
		mode:     "Dashboard",
		message:  "Ready",
		keyHints: "? help | q quit",
		width:    width,
		theme:    theme,
		isError:  false,
		leftStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("#FFB000")).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Padding(0, 1),
		rightStyle: lipgloss.NewStyle().
			Foreground(theme.Muted).
			Padding(0, 1),
	}
}

// SetMode sets the current mode to display.
func (s *StatusBar) SetMode(mode string) {
	s.mode = mode
}

// SetMessage sets the status message to display.
func (s *StatusBar) SetMessage(message string) {
	s.message = message
	s.isError = false
}

// SetError sets an error message to display.
// Error messages are styled differently from regular messages.
func (s *StatusBar) SetError(message string) {
	s.message = message
	s.isError = true
}

// SetKeyHints sets the key hints to display on the right.
func (s *StatusBar) SetKeyHints(hints string) {
	s.keyHints = hints
}

// SetWidth sets the width of the status bar.
func (s *StatusBar) SetWidth(width int) {
	if width > 0 {
		s.width = width
	}
}

// SetTheme sets the theme for the status bar.
func (s *StatusBar) SetTheme(theme *styles.Theme) {
	if theme != nil {
		s.theme = theme
		// Update styles with new theme
		s.leftStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#FFB000")).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Padding(0, 1)
		s.rightStyle = lipgloss.NewStyle().
			Foreground(theme.Muted).
			Padding(0, 1)
	}
}

// Render renders the status bar to a string.
// The status bar is divided into three sections:
//   - Left: Current mode (with background)
//   - Center: Status message (may be an error)
//   - Right: Key hints
func (s *StatusBar) Render() string {
	// Render left section (mode)
	leftSection := s.leftStyle.Render(s.mode)
	leftWidth := lipgloss.Width(leftSection)

	// Render right section (key hints)
	rightSection := s.rightStyle.Render(s.keyHints)
	rightWidth := lipgloss.Width(rightSection)

	// Calculate available space for center section
	availableCenter := s.width - leftWidth - rightWidth - 2 // -2 for spacing

	if availableCenter < 1 {
		availableCenter = 1
	}

	// Render center section (status message)
	var centerStyle lipgloss.Style
	if s.isError {
		centerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFC942")).
			Background(lipgloss.Color("#000000")).
			Bold(true).
			Reverse(true).
			Padding(0, 1)
	} else {
		centerStyle = lipgloss.NewStyle().
			Foreground(s.theme.Muted).
			Padding(0, 1)
	}

	// Truncate message if needed
	message := s.message
	if len(message) > availableCenter {
		if availableCenter > 3 {
			message = message[:availableCenter-3] + "..."
		} else {
			message = message[:availableCenter]
		}
	} else {
		// Pad message to center it
		padding := (availableCenter - len(message)) / 2
		message = strings.Repeat(" ", padding) + message + strings.Repeat(" ", availableCenter-len(message)-padding)
	}

	centerSection := centerStyle.Render(message)

	// Combine all sections
	statusBar := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftSection,
		centerSection,
		rightSection,
	)

	// Ensure the status bar fills the entire width
	currentWidth := lipgloss.Width(statusBar)
	if currentWidth < s.width {
		// Add padding to fill remaining space
		statusBar += strings.Repeat(" ", s.width-currentWidth)
	}

	// Apply background to entire status bar
	finalStyle := lipgloss.NewStyle().
		Width(s.width).
		Background(lipgloss.Color("#000000"))

	return finalStyle.Render(statusBar)
}

// Clear clears the status message.
func (s *StatusBar) Clear() {
	s.message = ""
	s.isError = false
}
