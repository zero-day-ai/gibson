package planning

import (
	"context"
	"errors"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// CompositeScorer implements StepScorer by chaining DeterministicScorer and LLMScorer.
// It uses a fast deterministic approach first, then falls back to LLM scoring when
// deterministic results are inconclusive (low confidence failures with no clear answer).
type CompositeScorer struct {
	deterministic *DeterministicScorer
	llm           StepScorer // nil if LLM scoring unavailable (uses interface for testability)
	timeout       time.Duration
}

// CompositeScorerOption configures a CompositeScorer
type CompositeScorerOption func(*CompositeScorer)

// WithLLMScorer configures the LLM scorer to use for fallback scoring
func WithLLMScorer(llm StepScorer) CompositeScorerOption {
	return func(s *CompositeScorer) {
		s.llm = llm
	}
}

// WithTimeout configures the timeout for LLM scoring calls
func WithTimeout(d time.Duration) CompositeScorerOption {
	return func(s *CompositeScorer) {
		s.timeout = d
	}
}

// NewCompositeScorer creates a CompositeScorer with deterministic scoring as primary
// and optional LLM scoring as fallback. Default timeout is 5 seconds.
func NewCompositeScorer(opts ...CompositeScorerOption) *CompositeScorer {
	s := &CompositeScorer{
		deterministic: NewDeterministicScorer(),
		timeout:       5 * time.Second,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Score evaluates a completed step using a two-tier approach:
//  1. Run deterministic scoring first (< 1ms, no LLM cost)
//  2. If result is inconclusive, fall back to LLM scoring
//  3. If LLM times out, return optimistic default
//  4. If LLM errors (non-timeout), fall back to deterministic result
//
// An inconclusive result is defined as:
//   - Success=false AND Confidence in [0.3, 0.7] (uncertain failure)
//   - Agent hints suggest uncertainty (if provided)
//   - No findings but completion without error
func (s *CompositeScorer) Score(ctx context.Context, input ScoreInput) (*StepScore, error) {
	// Step 1: Run deterministic scoring
	detScore, err := s.deterministic.Score(ctx, input)
	if err != nil {
		return nil, err
	}

	// Step 2: Check if deterministic result is conclusive
	if s.isConclusiveResult(detScore, input) {
		// High confidence result - no need for LLM
		return detScore, nil
	}

	// Step 3: If no LLM scorer configured, return deterministic result
	if s.llm == nil {
		return detScore, nil
	}

	// Step 4: Run LLM scoring with timeout
	llmScore, err := s.scoreLLMWithTimeout(ctx, input)
	if err != nil {
		// Check if timeout occurred
		if errors.Is(err, context.DeadlineExceeded) {
			// Timeout: return optimistic default
			return s.optimisticDefault(input), nil
		}

		// Non-timeout error: fall back to deterministic result
		return detScore, nil
	}

	// Step 5: Return successful LLM result
	return llmScore, nil
}

// isConclusiveResult determines if a deterministic score is conclusive enough
// to skip LLM fallback. Returns true if:
//   - High confidence success (confidence >= 0.7 AND success=true)
//   - High confidence failure (confidence < 0.3)
//   - Findings discovered (clear success indicator)
func (s *CompositeScorer) isConclusiveResult(score *StepScore, input ScoreInput) bool {
	// Findings discovered = conclusive success
	if score.FindingsCount > 0 {
		return true
	}

	// High confidence success
	if score.Success && score.Confidence >= 0.7 {
		return true
	}

	// High confidence failure (very low confidence in success)
	if !score.Success && score.Confidence < 0.3 {
		return true
	}

	// Check agent hints for uncertainty signals
	if input.NodeResult != nil && score.AgentHints != nil {
		// Agent explicitly signals uncertainty
		if score.AgentHints.Confidence > 0 && score.AgentHints.Confidence < 0.7 {
			return false // Inconclusive - agent is uncertain
		}

		// Agent suggests replanning
		if score.AgentHints.ReplanReason != "" {
			return false // Inconclusive - agent needs guidance
		}
	}

	// Medium confidence failure = inconclusive
	if !score.Success && score.Confidence >= 0.3 && score.Confidence <= 0.7 {
		return false
	}

	// No findings, completed without error, medium confidence = inconclusive
	if score.Success && score.Confidence < 0.7 && score.FindingsCount == 0 {
		if input.NodeResult != nil && input.NodeResult.Error == nil {
			return false
		}
	}

	// Default: treat as conclusive
	return true
}

// scoreLLMWithTimeout executes LLM scoring with the configured timeout
func (s *CompositeScorer) scoreLLMWithTimeout(ctx context.Context, input ScoreInput) (*StepScore, error) {
	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// Execute LLM scoring
	return s.llm.Score(timeoutCtx, input)
}

// optimisticDefault returns an optimistic default score when LLM times out.
// This assumes the step was successful but with medium confidence, allowing
// the mission to continue without blocking on LLM availability.
func (s *CompositeScorer) optimisticDefault(input ScoreInput) *StepScore {
	return &StepScore{
		NodeID:        input.NodeID,
		Success:       true,
		Confidence:    0.5,
		ShouldReplan:  false,
		ScoringMethod: "timeout_default",
		TokensUsed:    0,
		ScoredAt:      time.Now(),
		FindingsCount: 0,
		FindingIDs:    []types.ID{},
	}
}
