package middleware

import (
	"context"
	"testing"

	"github.com/zero-day-ai/gibson/internal/llm"
)

func TestPlanningContext_ThreadSafety(t *testing.T) {
	pc := &PlanningContext{
		TokenBudget:   10000,
		TokensUsed:    0,
		MaxIterations: 10,
		Iteration:     0,
	}

	// Test concurrent token updates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			resp := &llm.CompletionResponse{
				Usage: llm.CompletionTokenUsage{
					TotalTokens: 100,
				},
			}
			pc.UpdateFromResponse(resp)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify total
	if pc.TokensUsed != 1000 {
		t.Errorf("Expected TokensUsed=1000, got %d", pc.TokensUsed)
	}

	if pc.RemainingTokens() != 9000 {
		t.Errorf("Expected RemainingTokens=9000, got %d", pc.RemainingTokens())
	}
}

func TestPlanningContext_IsOverBudget(t *testing.T) {
	pc := &PlanningContext{
		TokenBudget: 1000,
		TokensUsed:  500,
	}

	if pc.IsOverBudget() {
		t.Error("Should not be over budget with 500/1000 tokens used")
	}

	pc.TokensUsed = 1001

	if !pc.IsOverBudget() {
		t.Error("Should be over budget with 1001/1000 tokens used")
	}
}

func TestPlanningContext_AddHint(t *testing.T) {
	pc := &PlanningContext{}

	pc.AddHint(AgentHint{
		AgentName: "agent1",
		Hint:      "test hint 1",
		Priority:  5,
	})

	pc.AddHint(AgentHint{
		AgentName: "agent2",
		Hint:      "test hint 2",
		Priority:  10,
	})

	hints := pc.GetHints()
	if len(hints) != 2 {
		t.Fatalf("Expected 2 hints, got %d", len(hints))
	}

	// Verify sorted by priority (highest first)
	if hints[0].Priority != 10 {
		t.Errorf("Expected first hint to have priority 10, got %d", hints[0].Priority)
	}
	if hints[1].Priority != 5 {
		t.Errorf("Expected second hint to have priority 5, got %d", hints[1].Priority)
	}
}

func TestPlanningContext_NilSafe(t *testing.T) {
	var pc *PlanningContext = nil

	// All methods should be nil-safe
	pc.UpdateFromResponse(nil)
	pc.AddHint(AgentHint{})

	if pc.RemainingTokens() != 0 {
		t.Error("Nil context should return 0 remaining tokens")
	}

	if pc.IsOverBudget() {
		t.Error("Nil context should not be over budget")
	}

	if pc.GetHints() != nil {
		t.Error("Nil context should return nil hints")
	}
}

func TestPlanningMiddleware_NilContext(t *testing.T) {
	// Middleware should work with nil planning context (no-op mode)
	mw := PlanningMiddleware(nil)

	called := false
	operation := func(ctx context.Context, req any) (any, error) {
		called = true
		return "result", nil
	}

	wrapped := mw(operation)
	result, err := wrapped(context.Background(), nil)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result != "result" {
		t.Errorf("Expected result='result', got %v", result)
	}

	if !called {
		t.Error("Expected operation to be called")
	}
}

func TestPlanningMiddleware_WithContext(t *testing.T) {
	pc := &PlanningContext{
		TokenBudget: 10000,
		TokensUsed:  0,
	}

	mw := PlanningMiddleware(pc)

	operation := func(ctx context.Context, req any) (any, error) {
		// Verify context is injected
		retrievedPc := GetPlanningContext(ctx)
		if retrievedPc == nil {
			t.Error("Expected planning context in operation context")
		}

		// Return a completion response to test token tracking
		return &llm.CompletionResponse{
			Usage: llm.CompletionTokenUsage{
				TotalTokens: 250,
			},
		}, nil
	}

	wrapped := mw(operation)
	_, err := wrapped(context.Background(), nil)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify tokens were tracked
	if pc.TokensUsed != 250 {
		t.Errorf("Expected TokensUsed=250, got %d", pc.TokensUsed)
	}
}

func TestWithPlanningContext_GetPlanningContext(t *testing.T) {
	pc := &PlanningContext{
		TokenBudget: 5000,
	}

	ctx := WithPlanningContext(context.Background(), pc)

	retrieved := GetPlanningContext(ctx)
	if retrieved == nil {
		t.Fatal("Expected to retrieve planning context")
	}

	if retrieved.TokenBudget != 5000 {
		t.Errorf("Expected TokenBudget=5000, got %d", retrieved.TokenBudget)
	}

	// Test with context that doesn't have planning context
	emptyCtx := context.Background()
	nilRetrieved := GetPlanningContext(emptyCtx)
	if nilRetrieved != nil {
		t.Error("Expected nil from context without planning context")
	}
}
