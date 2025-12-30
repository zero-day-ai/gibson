package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// Panel represents a bordered container with a title and content.
// It supports focus state with different border styles.
type Panel struct {
	title   string
	content string
	width   int
	height  int
	focused bool
	theme   *styles.Theme
}

// NewPanel creates a new Panel with the given title.
// The panel uses the default theme for styling.
func NewPanel(title string) *Panel {
	return &Panel{
		title:   title,
		content: "",
		width:   40,
		height:  10,
		focused: false,
		theme:   styles.DefaultTheme(),
	}
}

// SetContent sets the content of the panel.
// Content will be truncated and scrolling indicators added if it exceeds panel dimensions.
func (p *Panel) SetContent(content string) {
	p.content = content
}

// SetSize sets the dimensions of the panel.
// Width and height should include the border.
func (p *Panel) SetSize(width, height int) {
	if width > 0 {
		p.width = width
	}
	if height > 0 {
		p.height = height
	}
}

// SetFocused sets the focus state of the panel.
// Focused panels use a highlighted border color.
func (p *Panel) SetFocused(focused bool) {
	p.focused = focused
}

// Focused returns whether the panel is currently focused.
func (p *Panel) Focused() bool {
	return p.focused
}

// Title returns the panel's title.
func (p *Panel) Title() string {
	return p.title
}

// SetTitle sets the panel's title.
func (p *Panel) SetTitle(title string) {
	p.title = title
}

// Render renders the panel to a string with borders, title, and content.
// It handles content truncation and adds scrolling indicators when needed.
func (p *Panel) Render() string {
	// Border characters
	const (
		topLeft     = "┌"
		topRight    = "┐"
		bottomLeft  = "└"
		bottomRight = "┘"
		horizontal  = "─"
		vertical    = "│"
	)

	// Calculate content dimensions (inside borders, with 1 char padding each side)
	// Total: border(1) + padding(1) + content + padding(1) + border(1) = width
	// So content width = width - 4
	contentWidth := p.width - 4
	contentHeight := p.height - 2 // top and bottom borders

	if contentWidth < 1 {
		contentWidth = 1
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Select border color based on focus state
	borderColor := p.theme.Muted
	if p.focused {
		borderColor = p.theme.Primary
	}
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	titleStyle := p.theme.TitleStyle

	// Build top border with title
	innerWidth := p.width - 2 // width between corner chars
	var topBorder string
	if p.title != "" {
		titleText := " " + p.title + " "
		titleLen := len(titleText)
		remainingWidth := innerWidth - titleLen
		if remainingWidth > 0 {
			topBorder = borderStyle.Render(topLeft) +
				titleStyle.Render(titleText) +
				borderStyle.Render(strings.Repeat(horizontal, remainingWidth)+topRight)
		} else {
			topBorder = borderStyle.Render(topLeft + strings.Repeat(horizontal, innerWidth) + topRight)
		}
	} else {
		topBorder = borderStyle.Render(topLeft + strings.Repeat(horizontal, innerWidth) + topRight)
	}

	// Build bottom border
	bottomBorder := borderStyle.Render(bottomLeft + strings.Repeat(horizontal, innerWidth) + bottomRight)

	// Process content lines
	lines := strings.Split(p.content, "\n")
	visibleLines := make([]string, 0, contentHeight)

	// Determine if we need scrolling indicators
	hasMoreContent := len(lines) > contentHeight

	// Take only the lines that fit
	displayLines := lines
	if hasMoreContent {
		if contentHeight > 1 {
			displayLines = lines[:contentHeight-1]
		} else {
			displayLines = lines[:1]
		}
	}

	// Process each line - truncate or pad to exact width
	for _, line := range displayLines {
		visibleLines = append(visibleLines, p.fitLine(line, contentWidth))
	}

	// Add scrolling indicator if needed
	if hasMoreContent && contentHeight > 0 {
		indicator := "..."
		visibleLines = append(visibleLines, p.fitLine(indicator, contentWidth))
	}

	// Pad with empty lines to fill height
	for len(visibleLines) < contentHeight {
		visibleLines = append(visibleLines, strings.Repeat(" ", contentWidth))
	}

	// Build content rows with borders
	var rows []string
	rows = append(rows, topBorder)
	for _, line := range visibleLines {
		row := borderStyle.Render(vertical) + " " + line + " " + borderStyle.Render(vertical)
		rows = append(rows, row)
	}
	rows = append(rows, bottomBorder)

	return strings.Join(rows, "\n")
}

// fitLine truncates or pads a line to exactly the specified width
func (p *Panel) fitLine(line string, width int) string {
	lineWidth := lipgloss.Width(line)
	if lineWidth > width {
		// Truncate
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

// SetTheme sets the theme for the panel.
// This allows customization of panel appearance.
func (p *Panel) SetTheme(theme *styles.Theme) {
	if theme != nil {
		p.theme = theme
	}
}
