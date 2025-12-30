package components

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zero-day-ai/gibson/internal/tui/styles"
)

// typewriterTickMsg triggers the next character reveal in the typing animation.
type typewriterTickMsg struct{}

// cursorBlinkMsg toggles cursor visibility for the blinking effect.
type cursorBlinkMsg struct{}

// TypeWriter is a BubbleTea-compatible component that animates text appearing
// character-by-character with a blinking cursor, similar to old terminal typing effects.
type TypeWriter struct {
	fullText     string          // the complete text to display
	visibleChars int             // number of characters currently visible
	cursor       bool            // cursor visibility state for blinking
	speed        time.Duration   // time between character reveals (default 50ms)
	done         bool            // whether animation is complete
	theme        *styles.Theme   // for styling
}

// NewTypeWriter creates a new TypeWriter component with default 50ms typing speed.
func NewTypeWriter() *TypeWriter {
	return &TypeWriter{
		fullText:     "",
		visibleChars: 0,
		cursor:       true,
		speed:        50 * time.Millisecond,
		done:         false,
		theme:        styles.DefaultTheme(),
	}
}

// SetText sets new text to type and resets the animation.
func (t *TypeWriter) SetText(text string) {
	t.fullText = text
	t.visibleChars = 0
	t.cursor = true
	t.done = false
}

// SetSpeed configures the typing speed in milliseconds.
func (t *TypeWriter) SetSpeed(ms int) {
	if ms > 0 {
		t.speed = time.Duration(ms) * time.Millisecond
	}
}

// SetTheme sets the theme for styling.
func (t *TypeWriter) SetTheme(theme *styles.Theme) {
	if theme != nil {
		t.theme = theme
	}
}

// Done returns true when all text is visible.
func (t *TypeWriter) Done() bool {
	return t.done
}

// Skip immediately shows all text and completes the animation.
func (t *TypeWriter) Skip() {
	t.visibleChars = len(t.fullText)
	t.done = true
	t.cursor = true
}

// Update handles tick messages for animation.
// This is the BubbleTea update function pattern.
func (t *TypeWriter) Update(msg tea.Msg) (*TypeWriter, tea.Cmd) {
	switch msg.(type) {
	case typewriterTickMsg:
		if !t.done && t.visibleChars < len(t.fullText) {
			t.visibleChars++
			if t.visibleChars >= len(t.fullText) {
				t.done = true
				// Start cursor blinking when done
				return t, t.cursorBlinkCmd()
			}
			// Continue typing
			return t, t.typeNextCharCmd()
		}
	case cursorBlinkMsg:
		if t.done {
			t.cursor = !t.cursor
			return t, t.cursorBlinkCmd()
		}
	}
	return t, nil
}

// View renders the current visible text with a blinking cursor (█).
func (t *TypeWriter) View() string {
	if t.fullText == "" {
		return ""
	}

	// Get the visible portion of the text
	visibleText := ""
	if t.visibleChars > 0 {
		if t.visibleChars > len(t.fullText) {
			visibleText = t.fullText
		} else {
			visibleText = t.fullText[:t.visibleChars]
		}
	}

	// Add cursor
	cursorStr := ""
	if t.cursor {
		cursorStr = "█"
	} else {
		cursorStr = " "
	}

	// Style the text with theme
	textStyle := lipgloss.NewStyle().Foreground(t.theme.Primary)
	cursorStyle := lipgloss.NewStyle().Foreground(t.theme.Primary)

	return textStyle.Render(visibleText) + cursorStyle.Render(cursorStr)
}

// typeNextCharCmd returns a command that triggers the next character reveal.
func (t *TypeWriter) typeNextCharCmd() tea.Cmd {
	return tea.Tick(t.speed, func(time.Time) tea.Msg {
		return typewriterTickMsg{}
	})
}

// cursorBlinkCmd returns a command that toggles cursor visibility.
func (t *TypeWriter) cursorBlinkCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return cursorBlinkMsg{}
	})
}

// Start begins the typing animation and returns the initial command.
// Call this to start the animation after setting text.
func (t *TypeWriter) Start() tea.Cmd {
	if t.fullText == "" || t.done {
		return nil
	}
	return t.typeNextCharCmd()
}
