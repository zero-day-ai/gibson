package components

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zero-day-ai/gibson/internal/eval"
	"github.com/zero-day-ai/gibson/internal/types"
	sdkeval "github.com/zero-day-ai/sdk/eval"
)

func TestNewEvalMetricsPanel(t *testing.T) {
	panel := NewEvalMetricsPanel()

	if panel == nil {
		t.Fatal("NewEvalMetricsPanel returned nil")
	}

	if panel.width != 40 {
		t.Errorf("expected default width 40, got %d", panel.width)
	}

	if panel.height != 20 {
		t.Errorf("expected default height 20, got %d", panel.height)
	}

	if !panel.visible {
		t.Error("expected panel to be visible by default")
	}

	if panel.summary != nil {
		t.Error("expected summary to be nil initially")
	}

	if len(panel.alerts) != 0 {
		t.Error("expected alerts to be empty initially")
	}

	if len(panel.scoreTrend) != 0 {
		t.Error("expected scoreTrend to be empty initially")
	}
}

func TestEvalMetricsPanel_SetSummary(t *testing.T) {
	panel := NewEvalMetricsPanel()

	summary := eval.NewEvalSummary(types.NewID())
	summary.OverallScore = 0.85
	summary.ScorerScores = map[string]float64{
		"accuracy": 0.90,
		"safety":   0.80,
	}
	summary.TotalAlerts = 5
	summary.WarningCount = 3
	summary.CriticalCount = 2

	panel.SetSummary(summary)

	if panel.summary != summary {
		t.Error("summary not set correctly")
	}

	// Check that score was added to trend
	if len(panel.scoreTrend) != 1 {
		t.Errorf("expected 1 score in trend, got %d", len(panel.scoreTrend))
	}

	if panel.scoreTrend[0] != 0.85 {
		t.Errorf("expected trend score 0.85, got %f", panel.scoreTrend[0])
	}

	// Add more summaries to test trend history
	for i := 0; i < 105; i++ {
		s := eval.NewEvalSummary(types.NewID())
		s.OverallScore = float64(i) / 100.0
		panel.SetSummary(s)
	}

	// Should keep only last 100 scores
	if len(panel.scoreTrend) != 100 {
		t.Errorf("expected trend to cap at 100 scores, got %d", len(panel.scoreTrend))
	}
}

func TestEvalMetricsPanel_AddAlert(t *testing.T) {
	panel := NewEvalMetricsPanel()

	alert := sdkeval.Alert{
		Level:     sdkeval.AlertWarning,
		Scorer:    "safety",
		Score:     0.45,
		Threshold: 0.5,
		Message:   "Score below threshold",
		Action:    sdkeval.ActionAdjust,
	}

	panel.AddAlert(alert)

	if len(panel.alerts) != 1 {
		t.Errorf("expected 1 alert, got %d", len(panel.alerts))
	}

	if panel.alerts[0].Level != sdkeval.AlertWarning {
		t.Error("alert not stored correctly")
	}

	// Test alert cap at 100
	for i := 0; i < 105; i++ {
		panel.AddAlert(alert)
	}

	if len(panel.alerts) != 100 {
		t.Errorf("expected alerts to cap at 100, got %d", len(panel.alerts))
	}
}

func TestEvalMetricsPanel_SetSize(t *testing.T) {
	panel := NewEvalMetricsPanel()

	panel.SetSize(80, 30)

	if panel.width != 80 {
		t.Errorf("expected width 80, got %d", panel.width)
	}

	if panel.height != 30 {
		t.Errorf("expected height 30, got %d", panel.height)
	}

	// Test invalid sizes
	panel.SetSize(0, 0)
	if panel.width != 80 || panel.height != 30 {
		t.Error("size should not change when invalid values are provided")
	}

	panel.SetSize(-10, -20)
	if panel.width != 80 || panel.height != 30 {
		t.Error("size should not change when negative values are provided")
	}
}

func TestEvalMetricsPanel_SetVisible(t *testing.T) {
	panel := NewEvalMetricsPanel()

	panel.SetVisible(false)
	if panel.visible {
		t.Error("expected panel to be invisible")
	}

	rendered := panel.Render()
	if rendered != "" {
		t.Error("invisible panel should render empty string")
	}

	panel.SetVisible(true)
	if !panel.visible {
		t.Error("expected panel to be visible")
	}
}

func TestEvalMetricsPanel_Render_Empty(t *testing.T) {
	panel := NewEvalMetricsPanel()
	panel.SetSize(60, 15)

	output := panel.Render()

	if output == "" {
		t.Error("expected non-empty output for empty panel")
	}

	// Should contain "No evaluation data available" message
	if !strings.Contains(output, "No evaluation data available") {
		t.Error("expected empty state message in output")
	}
}

func TestEvalMetricsPanel_Render_WithData(t *testing.T) {
	panel := NewEvalMetricsPanel()
	panel.SetSize(60, 25)

	// Create a summary with data
	summary := eval.NewEvalSummary(types.NewID())
	summary.OverallScore = 0.75
	summary.ScorerScores = map[string]float64{
		"accuracy":  0.80,
		"safety":    0.70,
		"coherence": 0.75,
	}
	summary.TotalAlerts = 3
	summary.WarningCount = 2
	summary.CriticalCount = 1
	summary.TotalSteps = 100
	summary.Duration = 5 * time.Minute
	summary.TokensUsed = 10000

	panel.SetSummary(summary)

	output := panel.Render()

	if output == "" {
		t.Error("expected non-empty output")
	}

	// Check that key elements are present
	if !strings.Contains(output, "0.75") {
		t.Error("expected overall score in output")
	}

	// Should contain scorer names
	if !strings.Contains(output, "accuracy") {
		t.Error("expected 'accuracy' scorer in output")
	}

	// Should contain alert counts
	if !strings.Contains(output, "3 total") {
		t.Error("expected total alert count in output")
	}
}

func TestEvalMetricsPanel_Render_ScoreColorCoding(t *testing.T) {
	panel := NewEvalMetricsPanel()

	tests := []struct {
		name  string
		score float64
	}{
		{"high score (green)", 0.85},
		{"medium score (yellow)", 0.65},
		{"low score (red)", 0.35},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := eval.NewEvalSummary(types.NewID())
			summary.OverallScore = tt.score
			panel.SetSummary(summary)

			output := panel.Render()
			if output == "" {
				t.Error("expected non-empty output")
			}

			// Verify score is rendered
			scoreStr := strings.Contains(output, "0.")
			if !scoreStr {
				t.Error("expected score value in output")
			}
		})
	}
}

func TestEvalMetricsPanel_Update_WindowSize(t *testing.T) {
	panel := NewEvalMetricsPanel()

	msg := tea.WindowSizeMsg{
		Width:  100,
		Height: 40,
	}

	updatedModel, cmd := panel.Update(msg)

	if cmd != nil {
		t.Error("expected nil command from window resize")
	}

	updatedPanel, ok := updatedModel.(*EvalMetricsPanel)
	if !ok {
		t.Fatal("expected updated model to be *EvalMetricsPanel")
	}

	if updatedPanel.width != 100 {
		t.Errorf("expected width 100 after update, got %d", updatedPanel.width)
	}

	if updatedPanel.height != 40 {
		t.Errorf("expected height 40 after update, got %d", updatedPanel.height)
	}
}

func TestEvalMetricsPanel_Update_EvalSummary(t *testing.T) {
	panel := NewEvalMetricsPanel()

	summary := eval.NewEvalSummary(types.NewID())
	summary.OverallScore = 0.88

	msg := EvalSummaryMsg{
		Summary: summary,
	}

	updatedModel, cmd := panel.Update(msg)

	if cmd != nil {
		t.Error("expected nil command from summary update")
	}

	updatedPanel, ok := updatedModel.(*EvalMetricsPanel)
	if !ok {
		t.Fatal("expected updated model to be *EvalMetricsPanel")
	}

	if updatedPanel.summary != summary {
		t.Error("expected summary to be set")
	}

	if len(updatedPanel.scoreTrend) != 1 {
		t.Error("expected score to be added to trend")
	}
}

func TestEvalMetricsPanel_Update_EvalAlert(t *testing.T) {
	panel := NewEvalMetricsPanel()

	alert := sdkeval.Alert{
		Level:     sdkeval.AlertCritical,
		Scorer:    "safety",
		Score:     0.25,
		Threshold: 0.5,
		Message:   "Critical safety issue",
		Action:    sdkeval.ActionAbort,
	}

	msg := EvalAlertMsg{
		Alert: alert,
	}

	updatedModel, cmd := panel.Update(msg)

	if cmd != nil {
		t.Error("expected nil command from alert update")
	}

	updatedPanel, ok := updatedModel.(*EvalMetricsPanel)
	if !ok {
		t.Fatal("expected updated model to be *EvalMetricsPanel")
	}

	if len(updatedPanel.alerts) != 1 {
		t.Error("expected alert to be added")
	}

	if updatedPanel.alerts[0].Level != sdkeval.AlertCritical {
		t.Error("expected critical alert to be stored")
	}
}

func TestEvalMetricsPanel_Init(t *testing.T) {
	panel := NewEvalMetricsPanel()

	cmd := panel.Init()

	if cmd != nil {
		t.Error("expected nil command from Init")
	}
}

func TestEvalMetricsPanel_View(t *testing.T) {
	panel := NewEvalMetricsPanel()

	summary := eval.NewEvalSummary(types.NewID())
	summary.OverallScore = 0.92
	panel.SetSummary(summary)

	view := panel.View()
	render := panel.Render()

	if view != render {
		t.Error("View() should return same output as Render()")
	}
}

func TestEvalMetricsPanel_ScoreTrend_Rendering(t *testing.T) {
	panel := NewEvalMetricsPanel()
	panel.SetSize(80, 30)

	// Add multiple scores to create a trend
	for i := 0; i < 20; i++ {
		summary := eval.NewEvalSummary(types.NewID())
		summary.OverallScore = float64(i) / 20.0 // Scores from 0.0 to 0.95
		panel.SetSummary(summary)
	}

	output := panel.Render()

	// Should contain trend section
	if !strings.Contains(output, "Score Trend") {
		t.Error("expected trend section in output")
	}

	// Should contain unicode block characters for sparkline
	hasBlocks := false
	blocks := "▁▂▃▄▅▆▇█"
	for _, b := range blocks {
		if strings.ContainsRune(output, b) {
			hasBlocks = true
			break
		}
	}

	if !hasBlocks {
		t.Error("expected sparkline blocks in trend visualization")
	}
}
