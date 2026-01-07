package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
)

// LLMScorer implements StepScorer using LLM reasoning for inconclusive cases.
// It provides intelligent evaluation when deterministic checks don't yield a confident result.
//
// The scorer uses prompt engineering best practices to mitigate bias:
//   - Asks for reasoning/rationale FIRST before requesting a score
//   - Prevents anchoring on a score and rationalizing backward
//   - Includes calibration examples to establish success criteria
//   - Provides original goal context for relevance assessment
//
// Implementation notes:
//   - Uses harness.Complete() for LLM access (slot-based model selection)
//   - Tracks token usage and records in StepScore.TokensUsed
//   - Sets ScoringMethod to "llm"
//   - Returns error on LLM failure (no defaulting - let CompositeScorer handle fallback)
//   - Parses structured JSON response for reliable extraction
type LLMScorer struct {
	// harness provides access to LLM capabilities
	harness harness.AgentHarness

	// slotName is the LLM slot to use for scoring (e.g., "primary", "fast")
	slotName string

	// budgetManager tracks token usage against scoring budget (optional)
	budgetManager BudgetManager

	// pricingConfig provides model pricing information for cost calculations
	pricingConfig *llm.PricingConfig
}

// LLMScorerOption configures optional LLMScorer behavior.
type LLMScorerOption func(*LLMScorer)

// WithScoringBudget configures a budget manager to track scoring token usage.
func WithScoringBudget(mgr BudgetManager) LLMScorerOption {
	return func(s *LLMScorer) {
		s.budgetManager = mgr
	}
}

// WithScoringSlot configures which LLM slot to use for scoring.
func WithScoringSlot(slot string) LLMScorerOption {
	return func(s *LLMScorer) {
		s.slotName = slot
	}
}

// NewLLMScorer creates a new LLM-based scorer.
//
// Parameters:
//   - h: Agent harness providing LLM access
//   - opts: Optional configuration (budget manager, slot name)
//
// Returns a configured scorer ready to evaluate steps.
func NewLLMScorer(h harness.AgentHarness, opts ...LLMScorerOption) *LLMScorer {
	scorer := &LLMScorer{
		harness:       h,
		slotName:      "primary", // Default to primary slot
		pricingConfig: llm.DefaultPricing(),
	}

	// Apply options
	for _, opt := range opts {
		opt(scorer)
	}

	return scorer
}

// Score evaluates a completed step using LLM reasoning.
//
// The scoring process:
//  1. Builds a comprehensive prompt with step context, original goal, calibration examples
//  2. Queries the LLM asking for analysis FIRST, then score (prevents anchoring bias)
//  3. Parses the JSON response to extract success, confidence, replan decision
//  4. Records token usage against scoring budget
//  5. Returns StepScore with ScoringMethod="llm"
//
// The LLM prompt instructs the model to:
//   - Analyze what the step attempted and what happened
//   - Consider the original mission goal for relevance
//   - Provide detailed reasoning before making a judgment
//   - Use calibration examples to understand success criteria
//   - Decide if replanning is needed based on failure patterns
//
// Returns an error if:
//   - Scoring budget is exhausted
//   - LLM call fails
//   - Response cannot be parsed as valid JSON
//   - Context is cancelled
//
// Note: This scorer does NOT default to success on error. The caller (typically
// CompositeScorer) should handle fallback logic.
func (s *LLMScorer) Score(ctx context.Context, input ScoreInput) (*StepScore, error) {
	// Validate input
	if input.NodeResult == nil {
		return nil, fmt.Errorf("node result cannot be nil")
	}
	if input.OriginalGoal == "" {
		return nil, fmt.Errorf("original goal cannot be empty")
	}

	// Check if we have sufficient scoring budget
	if s.budgetManager != nil && !s.budgetManager.CanAfford(PhaseScoring, 500) {
		return nil, NewPlanningError(ErrorTypeBudgetExhausted,
			"insufficient budget for LLM scoring")
	}

	// Build scoring prompt
	prompt := s.buildScoringPrompt(input)

	// Prepare LLM messages
	messages := []llm.Message{
		llm.NewSystemMessage(s.buildSystemPrompt()),
		llm.NewUserMessage(prompt),
	}

	// Call LLM for scoring
	resp, err := s.harness.Complete(ctx, s.slotName, messages,
		harness.WithTemperature(0.2), // Low temperature for consistent scoring
		harness.WithMaxTokens(1000),  // Allow detailed reasoning
	)
	if err != nil {
		return nil, fmt.Errorf("LLM completion failed during scoring: %w", err)
	}

	// Record token usage against scoring budget
	tokensUsed := resp.Usage.TotalTokens
	if s.budgetManager != nil {
		// Calculate cost from token usage
		costUsed := 0.0
		if s.pricingConfig != nil {
			provider, model := parseProviderModel(resp.Model)
			usage := llm.TokenUsage{
				InputTokens:  resp.Usage.PromptTokens,
				OutputTokens: resp.Usage.CompletionTokens,
			}
			if cost, err := s.pricingConfig.CalculateCost(provider, model, usage); err == nil {
				costUsed = cost
			}
		}
		if err := s.budgetManager.Consume(PhaseScoring, tokensUsed, costUsed); err != nil {
			return nil, fmt.Errorf("scoring budget exhausted during LLM call: %w", err)
		}
	}

	// Parse LLM response into score
	score, err := s.parseScoreResponse(resp.Message.Content, input)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LLM scoring response: %w", err)
	}

	// Set score metadata
	score.NodeID = input.NodeID
	score.ScoringMethod = "llm"
	score.ScoredAt = time.Now()
	score.TokensUsed = tokensUsed

	// Copy finding information from node result
	score.FindingsCount = len(input.NodeResult.Findings)
	score.FindingIDs = make([]types.ID, len(input.NodeResult.Findings))
	for i, finding := range input.NodeResult.Findings {
		score.FindingIDs[i] = finding.ID
	}

	return score, nil
}

// buildSystemPrompt creates the system message defining the scorer's role.
func (s *LLMScorer) buildSystemPrompt() string {
	return `You are an expert security testing evaluator for an AI security framework.

Your role is to analyze the execution of a security testing step and determine:
1. Whether the step achieved its purpose (success/failure)
2. Your confidence in this assessment (0.0-1.0)
3. Whether replanning is needed to improve the mission's chances

You must provide your reasoning BEFORE making your judgment. This prevents anchoring bias.

Key principles:
- Success means the step contributed meaningful progress toward the mission goal
- Findings discovered are strong indicators of success
- Tool execution without errors is a positive signal but not definitive
- Failed executions that provide useful information can still be valuable
- Consider the original mission goal when assessing relevance
- Replanning should be triggered when repeated failures indicate a strategic problem

Your response must be valid JSON matching the schema provided.`
}

// buildScoringPrompt constructs the detailed scoring prompt with all context.
func (s *LLMScorer) buildScoringPrompt(input ScoreInput) string {
	var b strings.Builder

	// Original goal for context
	b.WriteString("## Mission Goal\n\n")
	b.WriteString(input.OriginalGoal)
	b.WriteString("\n\n")

	// Step execution context
	b.WriteString("## Step Execution\n\n")
	b.WriteString(fmt.Sprintf("**Node ID**: %s\n", input.NodeID))
	b.WriteString(fmt.Sprintf("**Status**: %s\n", input.NodeResult.Status))

	if input.NodeResult.Error != nil {
		b.WriteString(fmt.Sprintf("**Error**: %s\n", input.NodeResult.Error.Error()))
	}

	b.WriteString(fmt.Sprintf("**Findings Discovered**: %d\n", len(input.NodeResult.Findings)))

	if len(input.NodeResult.Output) > 0 {
		b.WriteString("\n**Output Summary**:\n```\n")
		b.WriteString(s.formatOutput(input.NodeResult.Output))
		b.WriteString("\n```\n")
	}

	b.WriteString("\n")

	// Attempt history for pattern detection
	if len(input.AttemptHistory) > 0 {
		b.WriteString("## Previous Attempts\n\n")
		b.WriteString(fmt.Sprintf("This step has been attempted %d time(s) before:\n\n", len(input.AttemptHistory)))

		// Show last 3 attempts
		startIdx := 0
		if len(input.AttemptHistory) > 3 {
			startIdx = len(input.AttemptHistory) - 3
		}

		for i := startIdx; i < len(input.AttemptHistory); i++ {
			attempt := input.AttemptHistory[i]
			b.WriteString(fmt.Sprintf("%d. ", i+1))
			if attempt.Success {
				b.WriteString("✓ Success")
			} else {
				b.WriteString("✗ Failed")
			}
			b.WriteString(fmt.Sprintf(" - %d findings - %s\n", attempt.FindingsCount, attempt.Result))
		}

		b.WriteString("\n")
	}

	// Calibration examples
	b.WriteString("## Calibration Examples\n\n")
	b.WriteString(s.formatCalibrationExamples())
	b.WriteString("\n")

	// Response schema
	b.WriteString("## Required Response Format\n\n")
	b.WriteString(s.formatResponseSchema())

	return b.String()
}

// formatOutput formats the node output map into a readable summary.
func (s *LLMScorer) formatOutput(output map[string]any) string {
	var parts []string

	for key, value := range output {
		// Limit value length to prevent token explosion
		valueStr := fmt.Sprintf("%v", value)
		if len(valueStr) > 200 {
			valueStr = valueStr[:200] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s: %s", key, valueStr))
	}

	return strings.Join(parts, "\n")
}

// formatCalibrationExamples provides examples to help the LLM understand success criteria.
func (s *LLMScorer) formatCalibrationExamples() string {
	return `**Example 1 - Clear Success**:
- A prompt injection agent discovered 3 jailbreak techniques
- Status: completed, no errors
- Assessment: SUCCESS (confidence: 0.95) - discovered findings directly support mission goal

**Example 2 - Partial Success**:
- A reconnaissance tool completed but found no open ports
- Status: completed, no errors, 0 findings
- Assessment: SUCCESS (confidence: 0.6) - tool executed correctly, negative results are informative

**Example 3 - Failure Requiring Replan**:
- An attack agent failed 3 times with authentication errors
- Repeated failures with no progress
- Assessment: FAILURE (confidence: 0.8), REPLAN NEEDED - auth strategy is fundamentally wrong

**Example 4 - Useful Failure**:
- A fuzzing tool crashed due to input validation
- Status: failed, but error reveals security controls
- Assessment: SUCCESS (confidence: 0.7) - failure provides useful information about target`
}

// formatResponseSchema defines the expected JSON response structure.
func (s *LLMScorer) formatResponseSchema() string {
	return `You must respond with a JSON object matching this schema:

{
  "analysis": {
    "what_happened": "string (objective description of execution)",
    "key_observations": ["string (important findings or patterns)"],
    "goal_relevance": "string (how this relates to the mission goal)"
  },
  "assessment": {
    "success": "boolean (did this step contribute meaningful progress)",
    "confidence": "number (0.0-1.0, how confident in this assessment)",
    "success_rationale": "string (why success/failure)"
  },
  "replan_decision": {
    "should_replan": "boolean (does tactical replanning make sense)",
    "replan_reason": "string (if should_replan=true, explain why)"
  }
}

CRITICAL: Provide the "analysis" section FIRST, then make your "assessment". This order prevents anchoring bias.`
}

// parseScoreResponse parses the LLM's JSON response into a StepScore.
func (s *LLMScorer) parseScoreResponse(content string, input ScoreInput) (*StepScore, error) {
	// Extract JSON from potential markdown code blocks
	jsonContent := extractJSON(content)

	// Parse into intermediate structure
	var response struct {
		Analysis struct {
			WhatHappened    string   `json:"what_happened"`
			KeyObservations []string `json:"key_observations"`
			GoalRelevance   string   `json:"goal_relevance"`
		} `json:"analysis"`
		Assessment struct {
			Success          bool    `json:"success"`
			Confidence       float64 `json:"confidence"`
			SuccessRationale string  `json:"success_rationale"`
		} `json:"assessment"`
		ReplanDecision struct {
			ShouldReplan bool   `json:"should_replan"`
			ReplanReason string `json:"replan_reason"`
		} `json:"replan_decision"`
	}

	if err := json.Unmarshal([]byte(jsonContent), &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w (content preview: %s)",
			err, truncate(content, 100))
	}

	// Validate required fields are present
	if response.Analysis.WhatHappened == "" && len(response.Analysis.KeyObservations) == 0 && response.Analysis.GoalRelevance == "" {
		return nil, fmt.Errorf("response missing required analysis fields (content preview: %s)",
			truncate(content, 100))
	}
	if response.Assessment.SuccessRationale == "" {
		return nil, fmt.Errorf("response missing required assessment rationale (content preview: %s)",
			truncate(content, 100))
	}

	// Validate confidence range
	if response.Assessment.Confidence < 0.0 || response.Assessment.Confidence > 1.0 {
		return nil, fmt.Errorf("confidence must be in range [0.0, 1.0], got %f",
			response.Assessment.Confidence)
	}

	// Build StepScore from response
	score := &StepScore{
		Success:      response.Assessment.Success,
		Confidence:   response.Assessment.Confidence,
		ShouldReplan: response.ReplanDecision.ShouldReplan,
		ReplanReason: response.ReplanDecision.ReplanReason,
	}

	// If replanning is needed but no reason provided, use analysis
	if score.ShouldReplan && score.ReplanReason == "" {
		score.ReplanReason = "LLM scorer determined replanning needed based on execution patterns"
	}

	return score, nil
}
