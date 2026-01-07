package planning

import (
	"context"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// StepScorer evaluates workflow step results to determine success and replanning needs
type StepScorer interface {
	// Score evaluates a completed step's results
	Score(ctx context.Context, input ScoreInput) (*StepScore, error)
}

// ScoreInput contains all context needed to evaluate a step's execution
type ScoreInput struct {
	// NodeID is the identifier of the completed node
	NodeID string

	// NodeResult contains the execution result and output
	NodeResult *workflow.NodeResult

	// OriginalGoal is the mission goal for context
	OriginalGoal string

	// PlanContext provides the current plan state
	PlanContext *StrategicPlan

	// AttemptHistory contains previous attempts at this node (for detecting no progress)
	AttemptHistory []AttemptRecord
}

// StepScore represents the evaluation of a completed step
type StepScore struct {
	// NodeID is the identifier of the scored node
	NodeID string

	// MissionID is the identifier of the mission this step belongs to
	MissionID types.ID

	// Success indicates whether the step achieved its purpose
	Success bool

	// Confidence is the confidence in this assessment (0.0-1.0)
	Confidence float64

	// ScoringMethod indicates which method was used ("deterministic" or "llm")
	ScoringMethod string

	// FindingsCount is the number of findings discovered during this step
	FindingsCount int

	// FindingIDs contains the IDs of findings discovered
	FindingIDs []types.ID

	// ShouldReplan indicates if tactical replanning should be triggered
	ShouldReplan bool

	// ReplanReason explains why replanning is needed (if applicable)
	ReplanReason string

	// AgentHints contains feedback from the agent (if provided)
	AgentHints *StepHints

	// ScoredAt is when this scoring occurred
	ScoredAt time.Time

	// TokensUsed tracks tokens consumed by scoring (0 for deterministic)
	TokensUsed int
}

// StepHints provides feedback from agents to inform scoring and planning decisions
type StepHints struct {
	// Confidence is the agent's self-assessed confidence in its results (0.0-1.0)
	Confidence float64

	// SuggestedNext contains agent recommendations for next steps
	SuggestedNext []string

	// ReplanReason explains why the agent thinks replanning may be needed
	ReplanReason string

	// KeyFindings is a summary of important discoveries made during execution
	KeyFindings []string
}
