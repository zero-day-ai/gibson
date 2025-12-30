package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// Header displays a header bar at the top of the screen.
// It shows the application title/logo and optional navigation tabs.
type Header struct {
	title    string
	subtitle string
	width    int
	theme    *styles.Theme
}

// NewHeader creates a new header with the given title.
func NewHeader(title string) *Header {
	return &Header{
		title:    title,
		subtitle: "",
		width:    80,
		theme:    styles.DefaultTheme(),
	}
}

// SetTitle sets the header title.
func (h *Header) SetTitle(title string) {
	h.title = title
}

// SetSubtitle sets an optional subtitle displayed next to the title.
func (h *Header) SetSubtitle(subtitle string) {
	h.subtitle = subtitle
}

// SetWidth sets the width of the header.
func (h *Header) SetWidth(width int) {
	if width > 0 {
		h.width = width
	}
}

// SetTheme sets the theme for the header.
func (h *Header) SetTheme(theme *styles.Theme) {
	if theme != nil {
		h.theme = theme
	}
}

// Render renders the header to a string.
func (h *Header) Render() string {
	// Title style - bold and primary color
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(h.theme.Primary)

	// Subtitle style - muted
	subtitleStyle := lipgloss.NewStyle().
		Foreground(h.theme.Muted)

	// Separator
	sepStyle := lipgloss.NewStyle().
		Foreground(h.theme.Muted)

	// Build the header content
	var parts []string
	parts = append(parts, titleStyle.Render(h.title))
	if h.subtitle != "" {
		parts = append(parts, sepStyle.Render(" | "))
		parts = append(parts, subtitleStyle.Render(h.subtitle))
	}
	content := strings.Join(parts, "")

	// Calculate padding to fill width
	contentWidth := lipgloss.Width(content)
	remainingWidth := h.width - contentWidth - 2 // -2 for left padding
	if remainingWidth < 0 {
		remainingWidth = 0
	}

	// Build the full line with padding
	line := " " + content + strings.Repeat(" ", remainingWidth)

	// Apply background style to the line
	headerStyle := lipgloss.NewStyle().
		Width(h.width).
		Background(lipgloss.Color("#000000")).
		Foreground(lipgloss.Color("#cdd6f4"))

	headerLine := headerStyle.Render(line)

	// Add a border line below
	borderStyle := lipgloss.NewStyle().
		Width(h.width).
		Foreground(h.theme.Muted)
	borderLine := borderStyle.Render(strings.Repeat("─", h.width))

	return headerLine + "\n" + borderLine
}

// Height returns the height of the header in lines.
func (h *Header) Height() int {
	return 2 // 1 line content + 1 line border
}
