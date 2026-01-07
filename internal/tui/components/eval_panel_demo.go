//go:build ignore
// +build ignore

// This is a demo program to visualize the EvalMetricsPanel.
// Run with: go run eval_panel_demo.go

package main

import (
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/eval"
	"github.com/zero-day-ai/gibson/internal/tui/components"
	"github.com/zero-day-ai/gibson/internal/types"
	sdkeval "github.com/zero-day-ai/sdk/eval"
)

func main() {
	fmt.Println("EvalMetricsPanel Demo")
	fmt.Println("=====================\n")

	// Create panel
	panel := components.NewEvalMetricsPanel()
	panel.SetSize(70, 30)

	// Demo 1: Empty state
	fmt.Println("--- Empty State ---")
	fmt.Println(panel.Render())
	fmt.Println()

	// Demo 2: Good score with no alerts
	fmt.Println("--- Good Performance (Score: 0.85, No Alerts) ---")
	summary1 := eval.NewEvalSummary(types.NewID())
	summary1.OverallScore = 0.85
	summary1.ScorerScores = map[string]float64{
		"accuracy":  0.90,
		"safety":    0.80,
		"coherence": 0.85,
	}
	summary1.TotalAlerts = 0
	summary1.TotalSteps = 100
	summary1.Duration = 5 * time.Minute
	summary1.TokensUsed = 10000
	panel.SetSummary(summary1)
	fmt.Println(panel.Render())
	fmt.Println()

	// Demo 3: Medium score with warnings
	fmt.Println("--- Medium Performance (Score: 0.65, Warnings) ---")
	summary2 := eval.NewEvalSummary(types.NewID())
	summary2.OverallScore = 0.65
	summary2.ScorerScores = map[string]float64{
		"accuracy":  0.75,
		"safety":    0.55,
		"coherence": 0.65,
	}
	summary2.TotalAlerts = 3
	summary2.WarningCount = 3
	summary2.CriticalCount = 0
	summary2.TotalSteps = 150
	summary2.Duration = 7 * time.Minute
	summary2.TokensUsed = 15000
	panel.SetSummary(summary2)

	// Add some alerts
	panel.AddAlert(sdkeval.Alert{
		Level:     sdkeval.AlertWarning,
		Scorer:    "safety",
		Score:     0.55,
		Threshold: 0.60,
		Message:   "Safety score below expected threshold",
		Action:    sdkeval.ActionAdjust,
	})

	fmt.Println(panel.Render())
	fmt.Println()

	// Demo 4: Low score with critical alerts
	fmt.Println("--- Low Performance (Score: 0.35, Critical Alerts) ---")
	summary3 := eval.NewEvalSummary(types.NewID())
	summary3.OverallScore = 0.35
	summary3.ScorerScores = map[string]float64{
		"accuracy":  0.50,
		"safety":    0.20,
		"coherence": 0.35,
	}
	summary3.TotalAlerts = 5
	summary3.WarningCount = 2
	summary3.CriticalCount = 3
	summary3.TotalSteps = 80
	summary3.Duration = 4 * time.Minute
	summary3.TokensUsed = 8000
	panel.SetSummary(summary3)

	panel.AddAlert(sdkeval.Alert{
		Level:     sdkeval.AlertCritical,
		Scorer:    "safety",
		Score:     0.20,
		Threshold: 0.50,
		Message:   "Critical safety violation detected",
		Action:    sdkeval.ActionAbort,
	})

	fmt.Println(panel.Render())
	fmt.Println()

	// Demo 5: Score trend over time
	fmt.Println("--- Score Trend (Improving Performance) ---")
	panel2 := components.NewEvalMetricsPanel()
	panel2.SetSize(70, 30)

	// Simulate improving scores
	for i := 0; i < 15; i++ {
		score := 0.4 + float64(i)*0.04 // 0.4 to 0.96
		s := eval.NewEvalSummary(types.NewID())
		s.OverallScore = score
		s.ScorerScores = map[string]float64{
			"accuracy":  score + 0.02,
			"safety":    score - 0.02,
			"coherence": score,
		}
		s.TotalAlerts = 0
		panel2.SetSummary(s)
	}

	fmt.Println(panel2.Render())
	fmt.Println()

	fmt.Println("--- Demo Complete ---")
}
