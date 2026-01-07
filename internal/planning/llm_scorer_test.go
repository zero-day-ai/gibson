package planning

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// scorerMockHarness wraps mockHarness and adds custom Complete behavior for testing LLMScorer.
type scorerMockHarness struct {
	mockHarness  // Embed the existing mockHarness
	completeFunc func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error)
}

func (m *scorerMockHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, slot, messages, opts...)
	}
	return nil, errors.New("mock Complete not implemented")
}

// mockBudgetManager implements BudgetManager for testing.
type mockBudgetManager struct {
	canAfford      bool
	consumeErr     error
	tokensConsumed int
	costConsumed   float64
}

func (m *mockBudgetManager) Allocate(totalTokens int, totalCost float64) *BudgetAllocation {
	return nil
}

func (m *mockBudgetManager) Consume(phase BudgetPhase, tokens int, cost float64) error {
	m.tokensConsumed += tokens
	m.costConsumed += cost
	return m.consumeErr
}

func (m *mockBudgetManager) Remaining(phase BudgetPhase) (tokens int, cost float64) {
	return 0, 0.0
}

func (m *mockBudgetManager) CanAfford(phase BudgetPhase, estimatedTokens int) bool {
	return m.canAfford
}

func (m *mockBudgetManager) GetReport() *BudgetReport {
	return nil
}

// TestLLMScorer_Score_Success tests successful LLM scoring.
func TestLLMScorer_Score_Success(t *testing.T) {
	tests := []struct {
		name                 string
		llmResponse          string
		expectedSuccess      bool
		expectedConfidence   float64
		expectedReplan       bool
		expectedReplanReason string
	}{
		{
			name: "clear success with findings",
			llmResponse: `{
				"analysis": {
					"what_happened": "Agent discovered 3 prompt injection vulnerabilities",
					"key_observations": ["High severity findings", "Multiple attack vectors"],
					"goal_relevance": "Directly supports mission goal of finding prompt injection issues"
				},
				"assessment": {
					"success": true,
					"confidence": 0.95,
					"success_rationale": "Discovered multiple high-value findings"
				},
				"replan_decision": {
					"should_replan": false,
					"replan_reason": ""
				}
			}`,
			expectedSuccess:    true,
			expectedConfidence: 0.95,
			expectedReplan:     false,
		},
		{
			name: "partial success no findings",
			llmResponse: `{
				"analysis": {
					"what_happened": "Tool completed but found no vulnerabilities",
					"key_observations": ["Clean execution", "Negative results are informative"],
					"goal_relevance": "Confirms target is hardened against this attack"
				},
				"assessment": {
					"success": true,
					"confidence": 0.6,
					"success_rationale": "Execution successful, negative results have value"
				},
				"replan_decision": {
					"should_replan": false,
					"replan_reason": ""
				}
			}`,
			expectedSuccess:    true,
			expectedConfidence: 0.6,
			expectedReplan:     false,
		},
		{
			name: "failure requiring replan",
			llmResponse: `{
				"analysis": {
					"what_happened": "Authentication failed repeatedly",
					"key_observations": ["3 consecutive failures", "No progress made"],
					"goal_relevance": "Blocking all progress toward goal"
				},
				"assessment": {
					"success": false,
					"confidence": 0.85,
					"success_rationale": "Fundamental strategy issue"
				},
				"replan_decision": {
					"should_replan": true,
					"replan_reason": "Authentication strategy is fundamentally flawed, need different approach"
				}
			}`,
			expectedSuccess:      false,
			expectedConfidence:   0.85,
			expectedReplan:       true,
			expectedReplanReason: "Authentication strategy is fundamentally flawed, need different approach",
		},
		{
			name: "useful failure",
			llmResponse: `{
				"analysis": {
					"what_happened": "Fuzzing crashed due to input validation",
					"key_observations": ["Error reveals security controls", "WAF detected"],
					"goal_relevance": "Provides intelligence about target defenses"
				},
				"assessment": {
					"success": true,
					"confidence": 0.7,
					"success_rationale": "Failure provided valuable intelligence"
				},
				"replan_decision": {
					"should_replan": false,
					"replan_reason": ""
				}
			}`,
			expectedSuccess:    true,
			expectedConfidence: 0.7,
			expectedReplan:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock harness that returns the test response
			mockH := &scorerMockHarness{
				completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
					// Verify slot name
					assert.Equal(t, "primary", slot)

					// Verify we got system + user messages
					assert.Len(t, messages, 2)
					assert.Equal(t, llm.RoleSystem, messages[0].Role)
					assert.Equal(t, llm.RoleUser, messages[1].Role)

					// Return mock response
					return &llm.CompletionResponse{
						Message: llm.Message{
							Role:    llm.RoleAssistant,
							Content: tt.llmResponse,
						},
						Usage: llm.CompletionTokenUsage{
							PromptTokens:     500,
							CompletionTokens: 200,
							TotalTokens:      700,
						},
					}, nil
				},
			}

			scorer := NewLLMScorer(mockH)

			// Create test input
			input := ScoreInput{
				NodeID: "test_node",
				NodeResult: &workflow.NodeResult{
					Status:   workflow.NodeStatusCompleted,
					Output:   map[string]any{"result": "test output"},
					Findings: []agent.Finding{},
				},
				OriginalGoal:   "Test mission goal",
				AttemptHistory: []AttemptRecord{},
			}

			// Execute scoring
			score, err := scorer.Score(context.Background(), input)

			// Verify results
			require.NoError(t, err)
			require.NotNil(t, score)
			assert.Equal(t, "test_node", score.NodeID)
			assert.Equal(t, "llm", score.ScoringMethod)
			assert.Equal(t, tt.expectedSuccess, score.Success)
			assert.InDelta(t, tt.expectedConfidence, score.Confidence, 0.001)
			assert.Equal(t, tt.expectedReplan, score.ShouldReplan)
			if tt.expectedReplanReason != "" {
				assert.Equal(t, tt.expectedReplanReason, score.ReplanReason)
			}
			assert.Equal(t, 700, score.TokensUsed)
			assert.False(t, score.ScoredAt.IsZero())
		})
	}
}

// TestLLMScorer_Score_WithFindings tests scoring when findings are present.
func TestLLMScorer_Score_WithFindings(t *testing.T) {
	mockH := &scorerMockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{
					Role: llm.RoleAssistant,
					Content: `{
						"analysis": {
							"what_happened": "Found vulnerabilities",
							"key_observations": ["Multiple findings"],
							"goal_relevance": "Supports goal"
						},
						"assessment": {
							"success": true,
							"confidence": 0.9,
							"success_rationale": "Findings discovered"
						},
						"replan_decision": {
							"should_replan": false,
							"replan_reason": ""
						}
					}`,
				},
				Usage: llm.CompletionTokenUsage{TotalTokens: 500},
			}, nil
		},
	}

	scorer := NewLLMScorer(mockH)

	// Create findings
	finding1 := agent.NewFinding("XSS", "Cross-site scripting", agent.SeverityHigh)
	finding1.ID = types.NewID()
	finding2 := agent.NewFinding("SQLi", "SQL injection", agent.SeverityCritical)
	finding2.ID = types.NewID()

	input := ScoreInput{
		NodeID: "test_node",
		NodeResult: &workflow.NodeResult{
			Status:   workflow.NodeStatusCompleted,
			Findings: []agent.Finding{finding1, finding2},
		},
		OriginalGoal: "Find vulnerabilities",
	}

	score, err := scorer.Score(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, 2, score.FindingsCount)
	assert.Len(t, score.FindingIDs, 2)
	assert.Contains(t, score.FindingIDs, finding1.ID)
	assert.Contains(t, score.FindingIDs, finding2.ID)
}

// TestLLMScorer_Score_WithAttemptHistory tests scoring with previous attempt context.
func TestLLMScorer_Score_WithAttemptHistory(t *testing.T) {
	mockH := &scorerMockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			// Verify the prompt includes attempt history
			userMsg := messages[1].Content
			assert.Contains(t, userMsg, "Previous Attempts")
			assert.Contains(t, userMsg, "3 time(s) before")

			return &llm.CompletionResponse{
				Message: llm.Message{
					Role: llm.RoleAssistant,
					Content: `{
						"analysis": {
							"what_happened": "Repeated failures",
							"key_observations": ["No progress in 3 attempts"],
							"goal_relevance": "Blocking progress"
						},
						"assessment": {
							"success": false,
							"confidence": 0.8,
							"success_rationale": "Repeated failures indicate strategic issue"
						},
						"replan_decision": {
							"should_replan": true,
							"replan_reason": "Need different approach after 3 failures"
						}
					}`,
				},
				Usage: llm.CompletionTokenUsage{TotalTokens: 600},
			}, nil
		},
	}

	scorer := NewLLMScorer(mockH)

	input := ScoreInput{
		NodeID: "test_node",
		NodeResult: &workflow.NodeResult{
			Status: workflow.NodeStatusFailed,
		},
		OriginalGoal: "Test goal",
		AttemptHistory: []AttemptRecord{
			{Timestamp: time.Now().Add(-3 * time.Minute), Success: false, Result: "Auth failed"},
			{Timestamp: time.Now().Add(-2 * time.Minute), Success: false, Result: "Auth failed"},
			{Timestamp: time.Now().Add(-1 * time.Minute), Success: false, Result: "Auth failed"},
		},
	}

	score, err := scorer.Score(context.Background(), input)

	require.NoError(t, err)
	assert.False(t, score.Success)
	assert.True(t, score.ShouldReplan)
	assert.Contains(t, score.ReplanReason, "different approach")
}

// TestLLMScorer_Score_WithBudgetManager tests budget tracking.
func TestLLMScorer_Score_WithBudgetManager(t *testing.T) {
	mockBudget := &mockBudgetManager{
		canAfford: true,
	}

	mockH := &scorerMockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return &llm.CompletionResponse{
				Message: llm.Message{
					Role: llm.RoleAssistant,
					Content: `{
						"analysis": {"what_happened": "test", "key_observations": [], "goal_relevance": "test"},
						"assessment": {"success": true, "confidence": 0.8, "success_rationale": "test"},
						"replan_decision": {"should_replan": false, "replan_reason": ""}
					}`,
				},
				Usage: llm.CompletionTokenUsage{TotalTokens: 750},
			}, nil
		},
	}

	scorer := NewLLMScorer(mockH, WithScoringBudget(mockBudget))

	input := ScoreInput{
		NodeID:       "test_node",
		NodeResult:   &workflow.NodeResult{Status: workflow.NodeStatusCompleted},
		OriginalGoal: "Test goal",
	}

	score, err := scorer.Score(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, 750, score.TokensUsed)
	assert.Equal(t, 750, mockBudget.tokensConsumed)
}

// TestLLMScorer_Score_BudgetExhausted tests behavior when budget is exhausted.
func TestLLMScorer_Score_BudgetExhausted(t *testing.T) {
	t.Run("insufficient budget before call", func(t *testing.T) {
		mockBudget := &mockBudgetManager{
			canAfford: false, // Not enough budget
		}

		mockH := &scorerMockHarness{} // Won't be called

		scorer := NewLLMScorer(mockH, WithScoringBudget(mockBudget))

		input := ScoreInput{
			NodeID:       "test_node",
			NodeResult:   &workflow.NodeResult{Status: workflow.NodeStatusCompleted},
			OriginalGoal: "Test goal",
		}

		score, err := scorer.Score(context.Background(), input)

		assert.Nil(t, score)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient budget")
	})

	t.Run("budget exhausted during call", func(t *testing.T) {
		mockBudget := &mockBudgetManager{
			canAfford:  true,
			consumeErr: errors.New("budget exhausted"),
		}

		mockH := &scorerMockHarness{
			completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
				return &llm.CompletionResponse{
					Message: llm.Message{Content: `{"analysis": {}, "assessment": {}, "replan_decision": {}}`},
					Usage:   llm.CompletionTokenUsage{TotalTokens: 1000},
				}, nil
			},
		}

		scorer := NewLLMScorer(mockH, WithScoringBudget(mockBudget))

		input := ScoreInput{
			NodeID:       "test_node",
			NodeResult:   &workflow.NodeResult{Status: workflow.NodeStatusCompleted},
			OriginalGoal: "Test goal",
		}

		score, err := scorer.Score(context.Background(), input)

		assert.Nil(t, score)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "budget exhausted")
	})
}

// TestLLMScorer_Score_LLMFailure tests handling of LLM completion failures.
func TestLLMScorer_Score_LLMFailure(t *testing.T) {
	mockH := &scorerMockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			return nil, errors.New("API rate limit exceeded")
		},
	}

	scorer := NewLLMScorer(mockH)

	input := ScoreInput{
		NodeID:       "test_node",
		NodeResult:   &workflow.NodeResult{Status: workflow.NodeStatusCompleted},
		OriginalGoal: "Test goal",
	}

	score, err := scorer.Score(context.Background(), input)

	assert.Nil(t, score)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM completion failed")
	assert.Contains(t, err.Error(), "rate limit")
}

// TestLLMScorer_Score_InvalidJSON tests handling of malformed LLM responses.
func TestLLMScorer_Score_InvalidJSON(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{
			name:     "not json",
			response: "This is not JSON at all",
		},
		{
			name:     "incomplete json",
			response: `{"analysis": {"what_happened": "test"`,
		},
		{
			name:     "wrong schema",
			response: `{"wrong": "schema", "format": true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockH := &scorerMockHarness{
				completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
					return &llm.CompletionResponse{
						Message: llm.Message{Content: tt.response},
						Usage:   llm.CompletionTokenUsage{TotalTokens: 100},
					}, nil
				},
			}

			scorer := NewLLMScorer(mockH)

			input := ScoreInput{
				NodeID:       "test_node",
				NodeResult:   &workflow.NodeResult{Status: workflow.NodeStatusCompleted},
				OriginalGoal: "Test goal",
			}

			score, err := scorer.Score(context.Background(), input)

			assert.Nil(t, score)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to parse")
		})
	}
}

// TestLLMScorer_Score_InvalidConfidence tests validation of confidence values.
func TestLLMScorer_Score_InvalidConfidence(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
	}{
		{"negative", -0.5},
		{"too high", 1.5},
		{"way too high", 100.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockH := &scorerMockHarness{
				completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
					response := map[string]any{
						"analysis": map[string]any{
							"what_happened":    "test",
							"key_observations": []string{},
							"goal_relevance":   "test",
						},
						"assessment": map[string]any{
							"success":           true,
							"confidence":        tt.confidence,
							"success_rationale": "test",
						},
						"replan_decision": map[string]any{
							"should_replan": false,
							"replan_reason": "",
						},
					}
					jsonBytes, _ := json.Marshal(response)
					return &llm.CompletionResponse{
						Message: llm.Message{Content: string(jsonBytes)},
						Usage:   llm.CompletionTokenUsage{TotalTokens: 100},
					}, nil
				},
			}

			scorer := NewLLMScorer(mockH)

			input := ScoreInput{
				NodeID:       "test_node",
				NodeResult:   &workflow.NodeResult{Status: workflow.NodeStatusCompleted},
				OriginalGoal: "Test goal",
			}

			score, err := scorer.Score(context.Background(), input)

			assert.Nil(t, score)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "confidence must be in range")
		})
	}
}

// TestLLMScorer_Score_MarkdownWrappedJSON tests extraction of JSON from markdown.
func TestLLMScorer_Score_MarkdownWrappedJSON(t *testing.T) {
	mockH := &scorerMockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			// Wrap valid JSON in markdown code blocks
			return &llm.CompletionResponse{
				Message: llm.Message{
					Content: "```json\n" +
						`{
							"analysis": {
								"what_happened": "test",
								"key_observations": [],
								"goal_relevance": "test"
							},
							"assessment": {
								"success": true,
								"confidence": 0.85,
								"success_rationale": "test"
							},
							"replan_decision": {
								"should_replan": false,
								"replan_reason": ""
							}
						}` +
						"\n```",
				},
				Usage: llm.CompletionTokenUsage{TotalTokens: 100},
			}, nil
		},
	}

	scorer := NewLLMScorer(mockH)

	input := ScoreInput{
		NodeID:       "test_node",
		NodeResult:   &workflow.NodeResult{Status: workflow.NodeStatusCompleted},
		OriginalGoal: "Test goal",
	}

	score, err := scorer.Score(context.Background(), input)

	require.NoError(t, err)
	assert.True(t, score.Success)
	assert.Equal(t, 0.85, score.Confidence)
}

// TestLLMScorer_Score_InvalidInput tests validation of input parameters.
func TestLLMScorer_Score_InvalidInput(t *testing.T) {
	mockH := &scorerMockHarness{}
	scorer := NewLLMScorer(mockH)

	tests := []struct {
		name        string
		input       ScoreInput
		expectedErr string
	}{
		{
			name: "nil node result",
			input: ScoreInput{
				NodeID:       "test",
				NodeResult:   nil,
				OriginalGoal: "goal",
			},
			expectedErr: "node result cannot be nil",
		},
		{
			name: "empty original goal",
			input: ScoreInput{
				NodeID:       "test",
				NodeResult:   &workflow.NodeResult{},
				OriginalGoal: "",
			},
			expectedErr: "original goal cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, err := scorer.Score(context.Background(), tt.input)

			assert.Nil(t, score)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

// TestLLMScorer_Score_CustomSlot tests using a custom LLM slot.
func TestLLMScorer_Score_CustomSlot(t *testing.T) {
	mockH := &scorerMockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			// Verify custom slot name
			assert.Equal(t, "fast", slot)

			return &llm.CompletionResponse{
				Message: llm.Message{
					Content: `{
						"analysis": {"what_happened": "test", "key_observations": [], "goal_relevance": "test"},
						"assessment": {"success": true, "confidence": 0.8, "success_rationale": "test"},
						"replan_decision": {"should_replan": false, "replan_reason": ""}
					}`,
				},
				Usage: llm.CompletionTokenUsage{TotalTokens: 100},
			}, nil
		},
	}

	scorer := NewLLMScorer(mockH, WithScoringSlot("fast"))

	input := ScoreInput{
		NodeID:       "test_node",
		NodeResult:   &workflow.NodeResult{Status: workflow.NodeStatusCompleted},
		OriginalGoal: "Test goal",
	}

	score, err := scorer.Score(context.Background(), input)

	require.NoError(t, err)
	assert.NotNil(t, score)
}

// TestLLMScorer_Score_ReplanWithoutReason tests default replan reason.
func TestLLMScorer_Score_ReplanWithoutReason(t *testing.T) {
	mockH := &scorerMockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			// Return replan=true but empty reason
			return &llm.CompletionResponse{
				Message: llm.Message{
					Content: `{
						"analysis": {"what_happened": "test", "key_observations": [], "goal_relevance": "test"},
						"assessment": {"success": false, "confidence": 0.7, "success_rationale": "test"},
						"replan_decision": {"should_replan": true, "replan_reason": ""}
					}`,
				},
				Usage: llm.CompletionTokenUsage{TotalTokens: 100},
			}, nil
		},
	}

	scorer := NewLLMScorer(mockH)

	input := ScoreInput{
		NodeID:       "test_node",
		NodeResult:   &workflow.NodeResult{Status: workflow.NodeStatusCompleted},
		OriginalGoal: "Test goal",
	}

	score, err := scorer.Score(context.Background(), input)

	require.NoError(t, err)
	assert.True(t, score.ShouldReplan)
	assert.NotEmpty(t, score.ReplanReason)
	assert.Contains(t, score.ReplanReason, "LLM scorer determined")
}

// TestLLMScorer_PromptContainsCalibration tests that prompts include calibration examples.
func TestLLMScorer_PromptContainsCalibration(t *testing.T) {
	mockH := &scorerMockHarness{
		completeFunc: func(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
			// Verify prompt structure
			require.Len(t, messages, 2)

			systemMsg := messages[0].Content
			assert.Contains(t, systemMsg, "reasoning BEFORE making your judgment")
			assert.Contains(t, systemMsg, "anchoring bias")

			userMsg := messages[1].Content
			assert.Contains(t, userMsg, "Mission Goal")
			assert.Contains(t, userMsg, "Step Execution")
			assert.Contains(t, userMsg, "Calibration Examples")
			assert.Contains(t, userMsg, "Example 1 - Clear Success")
			assert.Contains(t, userMsg, "Example 2 - Partial Success")
			assert.Contains(t, userMsg, "Example 3 - Failure Requiring Replan")
			assert.Contains(t, userMsg, "Required Response Format")
			assert.Contains(t, userMsg, "analysis")
			assert.Contains(t, userMsg, "assessment")
			assert.Contains(t, userMsg, "replan_decision")

			return &llm.CompletionResponse{
				Message: llm.Message{
					Content: `{
						"analysis": {"what_happened": "test", "key_observations": [], "goal_relevance": "test"},
						"assessment": {"success": true, "confidence": 0.8, "success_rationale": "test"},
						"replan_decision": {"should_replan": false, "replan_reason": ""}
					}`,
				},
				Usage: llm.CompletionTokenUsage{TotalTokens: 100},
			}, nil
		},
	}

	scorer := NewLLMScorer(mockH)

	input := ScoreInput{
		NodeID:       "test_node",
		NodeResult:   &workflow.NodeResult{Status: workflow.NodeStatusCompleted},
		OriginalGoal: "Find security vulnerabilities in the target LLM",
	}

	_, err := scorer.Score(context.Background(), input)
	require.NoError(t, err)
}
