package components_test

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/eval"
	"github.com/zero-day-ai/gibson/internal/tui/components"
	"github.com/zero-day-ai/gibson/internal/types"
	sdkeval "github.com/zero-day-ai/sdk/eval"
)

// Example demonstrating basic usage of EvalMetricsPanel
func ExampleEvalMetricsPanel_basic() {
	// Create a new eval metrics panel
	panel := components.NewEvalMetricsPanel()
	panel.SetSize(60, 25)

	// Create evaluation summary with scores
	summary := eval.NewEvalSummary(types.NewID())
	summary.OverallScore = 0.85
	summary.ScorerScores = map[string]float64{
		"accuracy": 0.90,
		"safety":   0.80,
		"speed":    0.85,
	}
	summary.TotalAlerts = 2
	summary.WarningCount = 2
	summary.CriticalCount = 0

	// Set the summary - this also adds the score to the trend
	panel.SetSummary(summary)

	// Render the panel
	output := panel.Render()
	fmt.Println(output != "")
	// Output: true
}

// Example demonstrating how to add alerts to the panel
func ExampleEvalMetricsPanel_withAlerts() {
	panel := components.NewEvalMetricsPanel()

	// Create summary
	summary := eval.NewEvalSummary(types.NewID())
	summary.OverallScore = 0.65
	summary.ScorerScores = map[string]float64{
		"accuracy": 0.70,
		"safety":   0.60,
	}
	summary.TotalAlerts = 3
	summary.WarningCount = 2
	summary.CriticalCount = 1

	panel.SetSummary(summary)

	// Add individual alerts
	warningAlert := sdkeval.Alert{
		Level:     sdkeval.AlertWarning,
		Scorer:    "safety",
		Score:     0.60,
		Threshold: 0.65,
		Message:   "Safety score below expected threshold",
		Action:    sdkeval.ActionAdjust,
	}
	panel.AddAlert(warningAlert)

	criticalAlert := sdkeval.Alert{
		Level:     sdkeval.AlertCritical,
		Scorer:    "safety",
		Score:     0.40,
		Threshold: 0.50,
		Message:   "Critical safety violation detected",
		Action:    sdkeval.ActionAbort,
	}
	panel.AddAlert(criticalAlert)

	output := panel.Render()
	fmt.Println(output != "")
	// Output: true
}

// Example demonstrating score trend tracking
func ExampleEvalMetricsPanel_scoreTrend() {
	panel := components.NewEvalMetricsPanel()
	panel.SetSize(80, 30)

	// Simulate multiple evaluation rounds
	scores := []float64{0.5, 0.6, 0.65, 0.7, 0.75, 0.8, 0.85, 0.9}

	for _, score := range scores {
		summary := eval.NewEvalSummary(types.NewID())
		summary.OverallScore = score
		summary.ScorerScores = map[string]float64{
			"accuracy": score + 0.05,
			"safety":   score - 0.05,
		}
		panel.SetSummary(summary)
	}

	// Panel now has a score trend showing improvement over time
	output := panel.Render()
	fmt.Println(output != "")
	// Output: true
}

// Example demonstrating visibility toggle
func ExampleEvalMetricsPanel_visibility() {
	panel := components.NewEvalMetricsPanel()

	summary := eval.NewEvalSummary(types.NewID())
	summary.OverallScore = 0.75
	panel.SetSummary(summary)

	// Panel is visible by default
	output := panel.Render()
	fmt.Println(output != "")

	// Hide the panel
	panel.SetVisible(false)
	output = panel.Render()
	fmt.Println(output == "")

	// Show the panel again
	panel.SetVisible(true)
	output = panel.Render()
	fmt.Println(output != "")

	// Output:
	// true
	// true
	// true
}

// Example demonstrating integration with bubbletea
func ExampleEvalMetricsPanel_bubbletea() {
	// Create panel as a tea.Model
	panel := components.NewEvalMetricsPanel()

	// Initialize
	_ = panel.Init()

	// Handle messages
	summary := eval.NewEvalSummary(types.NewID())
	summary.OverallScore = 0.88

	msg := components.EvalSummaryMsg{
		Summary: summary,
	}

	updatedModel, _ := panel.Update(msg)

	// Render view
	view := updatedModel.View()
	fmt.Println(view != "")
	// Output: true
}
