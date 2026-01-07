package harness

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ────────────────────────────────────────────────────────────────────────────
// PlanningContext Tests
// ────────────────────────────────────────────────────────────────────────────

func TestPlanningContext_FieldValidation(t *testing.T) {
	tests := []struct {
		name    string
		context *PlanningContext
		want    *PlanningContext
	}{
		{
			name: "complete context with all fields",
			context: &PlanningContext{
				OriginalGoal:           "Find SQL injection vulnerabilities",
				CurrentPosition:        2,
				TotalSteps:             5,
				RemainingSteps:         []string{"step3", "step4", "step5"},
				StepBudget:             1000,
				MissionBudgetRemaining: 5000,
			},
			want: &PlanningContext{
				OriginalGoal:           "Find SQL injection vulnerabilities",
				CurrentPosition:        2,
				TotalSteps:             5,
				RemainingSteps:         []string{"step3", "step4", "step5"},
				StepBudget:             1000,
				MissionBudgetRemaining: 5000,
			},
		},
		{
			name: "context at start of plan",
			context: &PlanningContext{
				OriginalGoal:           "Test authentication bypass",
				CurrentPosition:        0,
				TotalSteps:             3,
				RemainingSteps:         []string{"step1", "step2"},
				StepBudget:             500,
				MissionBudgetRemaining: 1500,
			},
			want: &PlanningContext{
				OriginalGoal:           "Test authentication bypass",
				CurrentPosition:        0,
				TotalSteps:             3,
				RemainingSteps:         []string{"step1", "step2"},
				StepBudget:             500,
				MissionBudgetRemaining: 1500,
			},
		},
		{
			name: "context at end of plan",
			context: &PlanningContext{
				OriginalGoal:           "XSS vulnerability scan",
				CurrentPosition:        4,
				TotalSteps:             5,
				RemainingSteps:         []string{},
				StepBudget:             200,
				MissionBudgetRemaining: 200,
			},
			want: &PlanningContext{
				OriginalGoal:           "XSS vulnerability scan",
				CurrentPosition:        4,
				TotalSteps:             5,
				RemainingSteps:         []string{},
				StepBudget:             200,
				MissionBudgetRemaining: 200,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want.OriginalGoal, tt.context.OriginalGoal)
			assert.Equal(t, tt.want.CurrentPosition, tt.context.CurrentPosition)
			assert.Equal(t, tt.want.TotalSteps, tt.context.TotalSteps)
			assert.Equal(t, tt.want.RemainingSteps, tt.context.RemainingSteps)
			assert.Equal(t, tt.want.StepBudget, tt.context.StepBudget)
			assert.Equal(t, tt.want.MissionBudgetRemaining, tt.context.MissionBudgetRemaining)
		})
	}
}

// ────────────────────────────────────────────────────────────────────────────
// StepHints Tests
// ────────────────────────────────────────────────────────────────────────────

func TestNewStepHints(t *testing.T) {
	hints := NewStepHints()

	require.NotNil(t, hints)
	assert.Equal(t, 0.5, hints.Confidence, "default confidence should be 0.5")
	assert.Empty(t, hints.SuggestedNext, "suggested next should be empty")
	assert.Empty(t, hints.ReplanReason, "replan reason should be empty")
	assert.Empty(t, hints.KeyFindings, "key findings should be empty")
}

func TestStepHints_WithConfidence(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
	}{
		{"zero confidence", 0.0},
		{"low confidence", 0.3},
		{"medium confidence", 0.5},
		{"high confidence", 0.8},
		{"full confidence", 1.0},
		{"negative confidence (allowed but unusual)", -0.1},
		{"over-confidence (allowed but unusual)", 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := NewStepHints().WithConfidence(tt.confidence)

			assert.NotNil(t, hints)
			assert.Equal(t, tt.confidence, hints.Confidence)
		})
	}
}

func TestStepHints_WithSuggestion(t *testing.T) {
	tests := []struct {
		name        string
		suggestions []string
		want        []string
	}{
		{
			name:        "single suggestion",
			suggestions: []string{"auth_bypass_agent"},
			want:        []string{"auth_bypass_agent"},
		},
		{
			name:        "multiple suggestions",
			suggestions: []string{"recon_agent", "exploit_agent", "post_exploit_agent"},
			want:        []string{"recon_agent", "exploit_agent", "post_exploit_agent"},
		},
		{
			name:        "duplicate suggestions (allowed)",
			suggestions: []string{"scan_agent", "scan_agent"},
			want:        []string{"scan_agent", "scan_agent"},
		},
		{
			name:        "empty suggestion string (allowed)",
			suggestions: []string{""},
			want:        []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := NewStepHints()
			for _, suggestion := range tt.suggestions {
				hints = hints.WithSuggestion(suggestion)
			}

			assert.Equal(t, tt.want, hints.SuggestedNext)
		})
	}
}

func TestStepHints_RecommendReplan(t *testing.T) {
	tests := []struct {
		name   string
		reason string
	}{
		{
			name:   "target type mismatch",
			reason: "Target is a REST API, not GraphQL - different attack vectors needed",
		},
		{
			name:   "unexpected characteristics",
			reason: "Authentication mechanism is OAuth2 instead of basic auth",
		},
		{
			name:   "dead end encountered",
			reason: "All injection points are properly sanitized, need to try different approach",
		},
		{
			name:   "empty reason (allowed)",
			reason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := NewStepHints().RecommendReplan(tt.reason)

			assert.NotNil(t, hints)
			assert.Equal(t, tt.reason, hints.ReplanReason)
		})
	}
}

func TestStepHints_WithKeyFinding(t *testing.T) {
	tests := []struct {
		name     string
		findings []string
		want     []string
	}{
		{
			name:     "single finding",
			findings: []string{"Discovered admin endpoint at /admin"},
			want:     []string{"Discovered admin endpoint at /admin"},
		},
		{
			name: "multiple findings",
			findings: []string{
				"Target uses JWT with RS256",
				"Found exposed .git directory",
				"Server version is Apache 2.4.41",
			},
			want: []string{
				"Target uses JWT with RS256",
				"Found exposed .git directory",
				"Server version is Apache 2.4.41",
			},
		},
		{
			name:     "empty finding (allowed)",
			findings: []string{""},
			want:     []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := NewStepHints()
			for _, finding := range tt.findings {
				hints = hints.WithKeyFinding(finding)
			}

			assert.Equal(t, tt.want, hints.KeyFindings)
		})
	}
}

func TestStepHints_BuilderChaining(t *testing.T) {
	// Test that all builder methods can be chained fluently
	hints := NewStepHints().
		WithConfidence(0.85).
		WithSuggestion("recon_agent").
		WithSuggestion("exploit_agent").
		WithKeyFinding("Found SQL injection in login form").
		WithKeyFinding("Database version is PostgreSQL 13.2").
		RecommendReplan("Need more specialized tools for PostgreSQL")

	assert.NotNil(t, hints)
	assert.Equal(t, 0.85, hints.Confidence)
	assert.Equal(t, []string{"recon_agent", "exploit_agent"}, hints.SuggestedNext)
	assert.Equal(t, "Need more specialized tools for PostgreSQL", hints.ReplanReason)
	assert.Equal(t, []string{
		"Found SQL injection in login form",
		"Database version is PostgreSQL 13.2",
	}, hints.KeyFindings)
}

func TestStepHints_BuilderModification(t *testing.T) {
	// Test that modifying a hints object after chaining works correctly
	hints := NewStepHints().
		WithConfidence(0.5).
		WithSuggestion("agent1")

	// Further modifications
	hints = hints.
		WithConfidence(0.9).
		WithSuggestion("agent2")

	assert.Equal(t, 0.9, hints.Confidence, "confidence should be updated")
	assert.Equal(t, []string{"agent1", "agent2"}, hints.SuggestedNext)
}

// ────────────────────────────────────────────────────────────────────────────
// DefaultAgentHarness Planning Context Integration Tests
// ────────────────────────────────────────────────────────────────────────────

func TestDefaultAgentHarness_PlanContext_DisabledByDefault(t *testing.T) {
	// Create a minimal harness for testing
	harness := &DefaultAgentHarness{}

	ctx := harness.PlanContext()
	assert.Nil(t, ctx, "PlanContext should return nil when planning is disabled")
}

func TestDefaultAgentHarness_GetStepBudget_UnlimitedByDefault(t *testing.T) {
	harness := &DefaultAgentHarness{}

	budget := harness.GetStepBudget()
	assert.Equal(t, 0, budget, "GetStepBudget should return 0 (unlimited) when planning is disabled")
}

func TestDefaultAgentHarness_SignalReplanRecommended_NoOpWhenDisabled(t *testing.T) {
	harness := &DefaultAgentHarness{
		logger: slog.Default(),
	}
	ctx := context.Background()

	err := harness.SignalReplanRecommended(ctx, "test reason")
	assert.NoError(t, err, "SignalReplanRecommended should not error when planning is disabled")
}

func TestDefaultAgentHarness_ReportStepHints_NoOpWhenDisabled(t *testing.T) {
	harness := &DefaultAgentHarness{
		logger: slog.Default(),
	}
	ctx := context.Background()

	hints := NewStepHints().
		WithConfidence(0.8).
		WithSuggestion("next_agent")

	err := harness.ReportStepHints(ctx, hints)
	assert.NoError(t, err, "ReportStepHints should not error when planning is disabled")
}

// ────────────────────────────────────────────────────────────────────────────
// Edge Cases and Validation Tests
// ────────────────────────────────────────────────────────────────────────────

func TestPlanningContext_EmptyRemainingSteps(t *testing.T) {
	// Test that empty remaining steps is valid (final step in plan)
	ctx := &PlanningContext{
		OriginalGoal:           "Complete mission",
		CurrentPosition:        2,
		TotalSteps:             3,
		RemainingSteps:         []string{},
		StepBudget:             100,
		MissionBudgetRemaining: 100,
	}

	assert.NotNil(t, ctx)
	assert.Empty(t, ctx.RemainingSteps)
}

func TestPlanningContext_ZeroBudget(t *testing.T) {
	// Test that zero budget values are valid
	ctx := &PlanningContext{
		OriginalGoal:           "Low budget mission",
		CurrentPosition:        0,
		TotalSteps:             1,
		RemainingSteps:         []string{},
		StepBudget:             0,
		MissionBudgetRemaining: 0,
	}

	assert.NotNil(t, ctx)
	assert.Equal(t, 0, ctx.StepBudget)
	assert.Equal(t, 0, ctx.MissionBudgetRemaining)
}

func TestStepHints_NilSuggestions(t *testing.T) {
	// Test that StepHints works with nil suggested next
	hints := &StepHints{
		Confidence:    0.7,
		SuggestedNext: nil,
		ReplanReason:  "",
		KeyFindings:   []string{},
	}

	// Should not panic when accessing nil slice
	assert.NotPanics(t, func() {
		_ = len(hints.SuggestedNext)
	})
}

func TestStepHints_ConcurrentModification(t *testing.T) {
	// Note: StepHints is not thread-safe by design
	// This test documents expected behavior - don't use concurrently
	hints := NewStepHints()

	// Sequential modifications work fine
	hints.WithConfidence(0.6)
	hints.WithSuggestion("agent1")

	assert.Equal(t, 0.6, hints.Confidence)
	assert.Equal(t, []string{"agent1"}, hints.SuggestedNext)
}

func TestStepHints_ReuseAfterChaining(t *testing.T) {
	// Test that returning the hints pointer allows reuse
	base := NewStepHints().WithConfidence(0.5)

	// Create two different hint sets from the same base
	hints1 := base.WithSuggestion("agent1")
	hints2 := base.WithSuggestion("agent2")

	// Both hints1 and hints2 point to the same object
	// This is expected behavior - the last modification wins
	assert.Equal(t, hints1, hints2)
	assert.Equal(t, []string{"agent1", "agent2"}, hints1.SuggestedNext)
}

func TestPlanningContext_LargeBudgetValues(t *testing.T) {
	// Test that large budget values are handled correctly
	ctx := &PlanningContext{
		OriginalGoal:           "High budget mission",
		CurrentPosition:        0,
		TotalSteps:             1,
		RemainingSteps:         []string{},
		StepBudget:             1000000,
		MissionBudgetRemaining: 10000000,
	}

	assert.Equal(t, 1000000, ctx.StepBudget)
	assert.Equal(t, 10000000, ctx.MissionBudgetRemaining)
}

func TestStepHints_LongStrings(t *testing.T) {
	// Test that long strings are handled correctly
	longReason := string(make([]byte, 1000))
	hints := NewStepHints().RecommendReplan(longReason)

	assert.Len(t, hints.ReplanReason, 1000)
}
