package styles

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultThemeColors verifies that DefaultTheme returns the correct amber CRT color palette.
func TestDefaultThemeColors(t *testing.T) {
	theme := DefaultTheme()
	require.NotNil(t, theme, "DefaultTheme should return a non-nil theme")

	tests := []struct {
		name     string
		got      lipgloss.Color
		expected lipgloss.Color
	}{
		{
			name:     "Primary color should be bright amber",
			got:      theme.Primary,
			expected: lipgloss.Color("#FFD966"),
		},
		{
			name:     "Success color should be normal amber",
			got:      theme.Success,
			expected: lipgloss.Color("#FFB000"),
		},
		{
			name:     "Warning color should be normal amber",
			got:      theme.Warning,
			expected: lipgloss.Color("#FFB000"),
		},
		{
			name:     "Danger color should be bright amber",
			got:      theme.Danger,
			expected: lipgloss.Color("#FFD966"),
		},
		{
			name:     "Muted color should be dim amber",
			got:      theme.Muted,
			expected: lipgloss.Color("#805800"),
		},
		{
			name:     "Info color should be dim amber",
			got:      theme.Info,
			expected: lipgloss.Color("#805800"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.got)
		})
	}
}

// TestDefaultThemePanelStyles verifies that panel styles are correctly configured.
func TestDefaultThemePanelStyles(t *testing.T) {
	theme := DefaultTheme()
	require.NotNil(t, theme, "DefaultTheme should return a non-nil theme")

	t.Run("PanelStyle has correct border and padding", func(t *testing.T) {
		assert.NotNil(t, theme.PanelStyle)
		// Verify border foreground is muted color
		borderColor := theme.PanelStyle.GetBorderTopForeground()
		assert.Equal(t, theme.Muted, borderColor)
	})

	t.Run("FocusedPanelStyle has primary border color", func(t *testing.T) {
		assert.NotNil(t, theme.FocusedPanelStyle)
		// Verify border foreground is primary color
		borderColor := theme.FocusedPanelStyle.GetBorderTopForeground()
		assert.Equal(t, theme.Primary, borderColor)
	})

	t.Run("TitleStyle has primary color and bold", func(t *testing.T) {
		assert.NotNil(t, theme.TitleStyle)
		// Verify foreground is primary color
		foreground := theme.TitleStyle.GetForeground()
		assert.Equal(t, theme.Primary, foreground)
		// Verify bold
		assert.True(t, theme.TitleStyle.GetBold(), "TitleStyle should be bold")
	})
}

// TestSeverityStyle verifies that SeverityStyle returns correct styles for each severity level.
func TestSeverityStyle(t *testing.T) {
	theme := DefaultTheme()
	require.NotNil(t, theme, "DefaultTheme should return a non-nil theme")

	tests := []struct {
		name          string
		severity      string
		expectedStyle lipgloss.Style
		checkBg       bool
		expectedBg    lipgloss.Color
		checkFg       bool
		expectedFg    lipgloss.Color
		checkBold     bool
		expectedBold  bool
	}{
		{
			name:          "critical should have background color (inverse style)",
			severity:      "critical",
			expectedStyle: theme.SeverityCritical,
			checkBg:       true,
			expectedBg:    lipgloss.Color("#FFB000"),
			checkFg:       true,
			expectedFg:    lipgloss.Color("#000000"),
			checkBold:     true,
			expectedBold:  true,
		},
		{
			name:          "high should have bright amber foreground",
			severity:      "high",
			expectedStyle: theme.SeverityHigh,
			checkFg:       true,
			expectedFg:    lipgloss.Color("#FFD966"),
			checkBold:     true,
			expectedBold:  true,
		},
		{
			name:          "medium should have normal amber foreground",
			severity:      "medium",
			expectedStyle: theme.SeverityMedium,
			checkFg:       true,
			expectedFg:    lipgloss.Color("#FFB000"),
			checkBold:     false,
		},
		{
			name:          "low should have dim amber foreground",
			severity:      "low",
			expectedStyle: theme.SeverityLow,
			checkFg:       true,
			expectedFg:    lipgloss.Color("#805800"),
			checkBold:     false,
		},
		{
			name:          "info should have dim amber foreground",
			severity:      "info",
			expectedStyle: theme.SeverityInfo,
			checkFg:       true,
			expectedFg:    lipgloss.Color("#805800"),
			checkBold:     false,
		},
		{
			name:          "unknown severity should return empty style",
			severity:      "unknown",
			expectedStyle: lipgloss.NewStyle(),
			checkBg:       false,
			checkFg:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			style := theme.SeverityStyle(tt.severity)

			if tt.checkBg {
				bg := style.GetBackground()
				assert.Equal(t, tt.expectedBg, bg, "Background color should match")
			}

			if tt.checkFg {
				fg := style.GetForeground()
				assert.Equal(t, tt.expectedFg, fg, "Foreground color should match")
			}

			if tt.checkBold {
				assert.Equal(t, tt.expectedBold, style.GetBold(), "Style bold setting should match")
			}
		})
	}
}

// TestStatusStyle verifies that StatusStyle returns correct styles for each status.
func TestStatusStyle(t *testing.T) {
	theme := DefaultTheme()
	require.NotNil(t, theme, "DefaultTheme should return a non-nil theme")

	tests := []struct {
		name           string
		status         string
		expectedStyle  lipgloss.Style
		checkBg        bool
		expectedBg     lipgloss.Color
		checkFg        bool
		expectedFg     lipgloss.Color
		checkBold      bool
		expectedBold   bool
		checkItalic    bool
		expectedItalic bool
	}{
		{
			name:          "running should have bright amber, bold",
			status:        "running",
			expectedStyle: theme.StatusRunning,
			checkFg:       true,
			expectedFg:    lipgloss.Color("#FFD966"),
			checkBold:     true,
			expectedBold:  true,
		},
		{
			name:           "paused should have normal amber, italic",
			status:         "paused",
			expectedStyle:  theme.StatusPaused,
			checkFg:        true,
			expectedFg:     lipgloss.Color("#FFB000"),
			checkItalic:    true,
			expectedItalic: true,
		},
		{
			name:          "stopped should have dim amber",
			status:        "stopped",
			expectedStyle: theme.StatusStopped,
			checkFg:       true,
			expectedFg:    lipgloss.Color("#805800"),
		},
		{
			name:          "error should have inverse (background)",
			status:        "error",
			expectedStyle: theme.StatusError,
			checkBg:       true,
			expectedBg:    lipgloss.Color("#FFD966"),
			checkFg:       true,
			expectedFg:    lipgloss.Color("#000000"),
			checkBold:     true,
			expectedBold:  true,
		},
		{
			name:          "failed should also map to error style",
			status:        "failed",
			expectedStyle: theme.StatusError,
			checkBg:       true,
			expectedBg:    lipgloss.Color("#FFD966"),
			checkFg:       true,
			expectedFg:    lipgloss.Color("#000000"),
			checkBold:     true,
			expectedBold:  true,
		},
		{
			name:          "completed should have normal amber",
			status:        "completed",
			expectedStyle: theme.StatusComplete,
			checkFg:       true,
			expectedFg:    lipgloss.Color("#FFB000"),
		},
		{
			name:          "complete should also map to completed style",
			status:        "complete",
			expectedStyle: theme.StatusComplete,
			checkFg:       true,
			expectedFg:    lipgloss.Color("#FFB000"),
		},
		{
			name:          "pending should have dim amber",
			status:        "pending",
			expectedStyle: theme.StatusPending,
			checkFg:       true,
			expectedFg:    lipgloss.Color("#805800"),
		},
		{
			name:          "unknown status should return empty style",
			status:        "unknown",
			expectedStyle: lipgloss.NewStyle(),
			checkBg:       false,
			checkFg:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			style := theme.StatusStyle(tt.status)

			if tt.checkBg {
				bg := style.GetBackground()
				assert.Equal(t, tt.expectedBg, bg, "Background color should match")
			}

			if tt.checkFg {
				fg := style.GetForeground()
				assert.Equal(t, tt.expectedFg, fg, "Foreground color should match")
			}

			if tt.checkBold {
				assert.Equal(t, tt.expectedBold, style.GetBold(), "Style bold setting should match")
			}

			if tt.checkItalic {
				assert.Equal(t, tt.expectedItalic, style.GetItalic(), "Style italic setting should match")
			}
		})
	}
}

// TestSeverityStyleConsistency verifies that direct field access matches SeverityStyle method.
func TestSeverityStyleConsistency(t *testing.T) {
	theme := DefaultTheme()

	tests := []struct {
		severity      string
		expectedField *lipgloss.Style
	}{
		{"critical", &theme.SeverityCritical},
		{"high", &theme.SeverityHigh},
		{"medium", &theme.SeverityMedium},
		{"low", &theme.SeverityLow},
		{"info", &theme.SeverityInfo},
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			methodStyle := theme.SeverityStyle(tt.severity)
			fieldStyle := *tt.expectedField

			// Compare foreground colors
			methodFg := methodStyle.GetForeground()
			fieldFg := fieldStyle.GetForeground()
			assert.Equal(t, fieldFg, methodFg, "Foreground colors should match")

			// Compare background colors
			methodBg := methodStyle.GetBackground()
			fieldBg := fieldStyle.GetBackground()
			assert.Equal(t, fieldBg, methodBg, "Background colors should match")

			// Compare bold
			assert.Equal(t, fieldStyle.GetBold(), methodStyle.GetBold(), "Bold settings should match")
		})
	}
}

// TestStatusStyleConsistency verifies that direct field access matches StatusStyle method.
func TestStatusStyleConsistency(t *testing.T) {
	theme := DefaultTheme()

	tests := []struct {
		status        string
		expectedField *lipgloss.Style
	}{
		{"running", &theme.StatusRunning},
		{"paused", &theme.StatusPaused},
		{"stopped", &theme.StatusStopped},
		{"error", &theme.StatusError},
		{"completed", &theme.StatusComplete},
		{"pending", &theme.StatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			methodStyle := theme.StatusStyle(tt.status)
			fieldStyle := *tt.expectedField

			// Compare foreground colors
			methodFg := methodStyle.GetForeground()
			fieldFg := fieldStyle.GetForeground()
			assert.Equal(t, fieldFg, methodFg, "Foreground colors should match")

			// Compare background colors
			methodBg := methodStyle.GetBackground()
			fieldBg := fieldStyle.GetBackground()
			assert.Equal(t, fieldBg, methodBg, "Background colors should match")

			// Compare bold
			assert.Equal(t, fieldStyle.GetBold(), methodStyle.GetBold(), "Bold settings should match")

			// Compare italic
			assert.Equal(t, fieldStyle.GetItalic(), methodStyle.GetItalic(), "Italic settings should match")
		})
	}
}

// TestAmberCRTThemeBrightness verifies the brightness hierarchy of the amber CRT theme.
func TestAmberCRTThemeBrightness(t *testing.T) {
	theme := DefaultTheme()

	t.Run("Bright amber colors", func(t *testing.T) {
		brightAmber := lipgloss.Color("#FFD966")
		assert.Equal(t, brightAmber, theme.Primary, "Primary should be bright amber")
		assert.Equal(t, brightAmber, theme.Danger, "Danger should be bright amber")
	})

	t.Run("Normal amber colors", func(t *testing.T) {
		normalAmber := lipgloss.Color("#FFB000")
		assert.Equal(t, normalAmber, theme.Success, "Success should be normal amber")
		assert.Equal(t, normalAmber, theme.Warning, "Warning should be normal amber")
	})

	t.Run("Dim amber colors", func(t *testing.T) {
		dimAmber := lipgloss.Color("#805800")
		assert.Equal(t, dimAmber, theme.Muted, "Muted should be dim amber")
		assert.Equal(t, dimAmber, theme.Info, "Info should be dim amber")
	})
}

// TestSeverityStyleRendering verifies that severity styles can be applied to text.
func TestSeverityStyleRendering(t *testing.T) {
	theme := DefaultTheme()

	severities := []string{"critical", "high", "medium", "low", "info"}
	for _, severity := range severities {
		t.Run(severity, func(t *testing.T) {
			style := theme.SeverityStyle(severity)
			rendered := style.Render(severity)
			assert.NotEmpty(t, rendered, "Rendered text should not be empty")
		})
	}
}

// TestStatusStyleRendering verifies that status styles can be applied to text.
func TestStatusStyleRendering(t *testing.T) {
	theme := DefaultTheme()

	statuses := []string{"running", "paused", "stopped", "error", "completed", "pending"}
	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			style := theme.StatusStyle(status)
			rendered := style.Render(status)
			assert.NotEmpty(t, rendered, "Rendered text should not be empty")
		})
	}
}
