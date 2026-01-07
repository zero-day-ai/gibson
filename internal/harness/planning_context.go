package harness

import "context"

// PlanningContext provides read-only access to mission planning state.
// This allows agents to be aware of their position in the execution plan
// and make decisions based on remaining steps and budget.
//
// All fields are immutable from the agent's perspective. The planning system
// creates a new context for each step execution.
type PlanningContext struct {
	// OriginalGoal is the immutable mission goal statement
	OriginalGoal string

	// CurrentPosition is the index of the current step in the plan (0-based)
	CurrentPosition int

	// TotalSteps is the total number of planned steps
	TotalSteps int

	// RemainingSteps lists the node IDs that will execute after this step
	RemainingSteps []string

	// StepBudget is the token budget allocated for this specific step
	StepBudget int

	// MissionBudgetRemaining is the total remaining mission token budget
	MissionBudgetRemaining int
}

// StepHints allows agents to provide feedback to the planning system.
// This information is used by the step scorer to make better decisions
// about replanning, step ordering, and budget allocation.
//
// Agents should use the builder methods to construct hints incrementally.
type StepHints struct {
	// Confidence is the agent's self-assessed confidence in its results (0.0-1.0)
	// Higher confidence indicates the agent believes its execution was successful
	// and produced reliable results. Lower confidence suggests uncertainty or
	// partial failures that may require replanning.
	Confidence float64

	// SuggestedNext contains agent recommendations for next steps.
	// These should be node IDs from the workflow that the agent believes
	// would be productive to execute based on its findings.
	SuggestedNext []string

	// ReplanReason explains why the agent thinks replanning may be needed.
	// Set this when the agent encounters conditions that warrant changing the plan:
	//   - Unexpected target characteristics
	//   - Dead ends that block progress
	//   - Discovery of new attack surfaces requiring different approaches
	//
	// Empty string indicates no replan is recommended.
	ReplanReason string

	// KeyFindings is a summary of important discoveries made during execution.
	// These are concise statements (not full findings) that help the scorer
	// understand what was accomplished and inform subsequent step selection.
	//
	// Examples:
	//   - "Discovered admin endpoint at /admin"
	//   - "Target uses GraphQL instead of REST"
	//   - "Authentication uses JWT with RS256"
	KeyFindings []string
}

// NewStepHints creates a new StepHints with default values.
// Default confidence is 0.5 (neutral), no suggestions, no replan recommendation.
func NewStepHints() *StepHints {
	return &StepHints{
		Confidence:    0.5,
		SuggestedNext: []string{},
		KeyFindings:   []string{},
	}
}

// WithConfidence sets the confidence and returns the StepHints for chaining.
// Confidence should be in the range 0.0-1.0, but is not clamped by this method.
// The scorer will handle out-of-range values gracefully.
func (h *StepHints) WithConfidence(c float64) *StepHints {
	h.Confidence = c
	return h
}

// WithSuggestion adds a suggested next step and returns StepHints for chaining.
// The step should be a node ID from the workflow that exists in the DAG.
// Duplicates are allowed - the scorer will deduplicate.
func (h *StepHints) WithSuggestion(step string) *StepHints {
	h.SuggestedNext = append(h.SuggestedNext, step)
	return h
}

// RecommendReplan sets the replan reason and returns StepHints for chaining.
// This signals that the agent believes the current plan may not be optimal
// and provides an explanation for why replanning should be considered.
//
// Setting a non-empty reason will trigger scorer evaluation for replanning.
func (h *StepHints) RecommendReplan(reason string) *StepHints {
	h.ReplanReason = reason
	return h
}

// WithKeyFinding adds a key finding and returns StepHints for chaining.
// Findings should be concise summaries, not full vulnerability details.
// Multiple findings can be added by calling this method multiple times.
func (h *StepHints) WithKeyFinding(finding string) *StepHints {
	h.KeyFindings = append(h.KeyFindings, finding)
	return h
}

// PlanningContextProvider is the interface implemented by harness implementations
// to provide planning context to agents. This is separated from the main harness
// interface to allow for nil implementations when planning is disabled.
type PlanningContextProvider interface {
	// PlanContext returns the current planning context, or nil if planning is disabled
	PlanContext() *PlanningContext

	// GetStepBudget returns the token budget for this step, or 0 if unlimited
	GetStepBudget() int

	// SignalReplanRecommended signals that replanning may be needed
	SignalReplanRecommended(ctx context.Context, reason string) error

	// ReportStepHints provides feedback to the step scorer
	ReportStepHints(ctx context.Context, hints *StepHints) error
}
