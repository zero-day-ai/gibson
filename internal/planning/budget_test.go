package planning

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultBudgetManager_Allocate(t *testing.T) {
	tests := []struct {
		name        string
		totalTokens int
		totalCost   float64
		want        BudgetAllocation
	}{
		{
			name:        "standard allocation with 1000 tokens",
			totalTokens: 1000,
			totalCost:   10.0,
			want: BudgetAllocation{
				StrategicPlanning: 100, // 10%
				ScoringOverhead:   50,  // 5%
				ReplanReserve:     50,  // 5%
				Execution:         800, // 80%
				TotalTokens:       1000,
				TotalCost:         10.0,
			},
		},
		{
			name:        "allocation with rounding adjustment",
			totalTokens: 1001,
			totalCost:   5.0,
			want: BudgetAllocation{
				StrategicPlanning: 100,
				ScoringOverhead:   50,
				ReplanReserve:     50,
				Execution:         801, // Gets the extra token from rounding
				TotalTokens:       1001,
				TotalCost:         5.0,
			},
		},
		{
			name:        "small budget",
			totalTokens: 100,
			totalCost:   1.0,
			want: BudgetAllocation{
				StrategicPlanning: 10,
				ScoringOverhead:   5,
				ReplanReserve:     5,
				Execution:         80,
				TotalTokens:       100,
				TotalCost:         1.0,
			},
		},
		{
			name:        "zero budget",
			totalTokens: 0,
			totalCost:   0.0,
			want: BudgetAllocation{
				StrategicPlanning: 0,
				ScoringOverhead:   0,
				ReplanReserve:     0,
				Execution:         0,
				TotalTokens:       0,
				TotalCost:         0.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewDefaultBudgetManager()
			got := m.Allocate(tt.totalTokens, tt.totalCost)

			assert.Equal(t, tt.want.StrategicPlanning, got.StrategicPlanning, "StrategicPlanning mismatch")
			assert.Equal(t, tt.want.ScoringOverhead, got.ScoringOverhead, "ScoringOverhead mismatch")
			assert.Equal(t, tt.want.ReplanReserve, got.ReplanReserve, "ReplanReserve mismatch")
			assert.Equal(t, tt.want.Execution, got.Execution, "Execution mismatch")
			assert.Equal(t, tt.want.TotalTokens, got.TotalTokens, "TotalTokens mismatch")
			assert.Equal(t, tt.want.TotalCost, got.TotalCost, "TotalCost mismatch")

			// Verify sum equals total (critical invariant)
			sum := got.StrategicPlanning + got.ScoringOverhead + got.ReplanReserve + got.Execution
			assert.Equal(t, tt.totalTokens, sum, "Allocation sum must equal total tokens")
		})
	}
}

func TestDefaultBudgetManager_Consume(t *testing.T) {
	tests := []struct {
		name      string
		phase     BudgetPhase
		tokens    int
		cost      float64
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "consume within budget - strategic planning",
			phase:   PhaseStrategicPlanning,
			tokens:  50,
			cost:    0.5,
			wantErr: false,
		},
		{
			name:    "consume within budget - scoring",
			phase:   PhaseScoring,
			tokens:  25,
			cost:    0.25,
			wantErr: false,
		},
		{
			name:    "consume within budget - replanning",
			phase:   PhaseReplanning,
			tokens:  30,
			cost:    0.3,
			wantErr: false,
		},
		{
			name:    "consume within budget - execution",
			phase:   PhaseExecution,
			tokens:  500,
			cost:    5.0,
			wantErr: false,
		},
		{
			name:      "exceed budget - strategic planning",
			phase:     PhaseStrategicPlanning,
			tokens:    101, // Allocated is 100
			cost:      1.0,
			wantErr:   true,
			errSubstr: "budget exhausted",
		},
		{
			name:      "exceed budget - scoring",
			phase:     PhaseScoring,
			tokens:    51, // Allocated is 50
			cost:      0.5,
			wantErr:   true,
			errSubstr: "budget exhausted",
		},
		{
			name:    "consume exact budget",
			phase:   PhaseStrategicPlanning,
			tokens:  100, // Exactly the allocation
			cost:    1.0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewDefaultBudgetManager()
			m.Allocate(1000, 10.0) // Standard allocation

			err := m.Consume(tt.phase, tt.tokens, tt.cost)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)

				// Verify remaining budget decreased correctly
				remaining, _ := m.Remaining(tt.phase)
				allocated := getAllocatedForPhase(m.allocation, tt.phase)
				assert.Equal(t, allocated-tt.tokens, remaining, "Remaining budget incorrect")
			}
		})
	}
}

func TestDefaultBudgetManager_Consume_Sequential(t *testing.T) {
	m := NewDefaultBudgetManager()
	m.Allocate(1000, 10.0)

	// Consume in multiple steps
	require.NoError(t, m.Consume(PhaseStrategicPlanning, 30, 0.3))
	require.NoError(t, m.Consume(PhaseStrategicPlanning, 40, 0.4))
	require.NoError(t, m.Consume(PhaseStrategicPlanning, 30, 0.3))

	// Should have consumed 100 total, exactly the budget
	remaining, _ := m.Remaining(PhaseStrategicPlanning)
	assert.Equal(t, 0, remaining, "Should have consumed entire budget")

	// Next consumption should fail
	err := m.Consume(PhaseStrategicPlanning, 1, 0.01)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "budget exhausted")
}

func TestDefaultBudgetManager_Remaining(t *testing.T) {
	m := NewDefaultBudgetManager()
	m.Allocate(1000, 10.0)

	// Initially, all budget should be remaining
	tokens, cost := m.Remaining(PhaseStrategicPlanning)
	assert.Equal(t, 100, tokens)
	assert.Greater(t, cost, 0.0) // Should have proportional cost

	// Consume some budget
	require.NoError(t, m.Consume(PhaseStrategicPlanning, 60, 0.6))

	// Check remaining
	tokens, cost = m.Remaining(PhaseStrategicPlanning)
	assert.Equal(t, 40, tokens)
	assert.Greater(t, cost, 0.0)

	// Consume rest
	require.NoError(t, m.Consume(PhaseStrategicPlanning, 40, 0.4))

	// Should be zero
	tokens, cost = m.Remaining(PhaseStrategicPlanning)
	assert.Equal(t, 0, tokens)
	assert.Equal(t, 0.0, cost)
}

func TestDefaultBudgetManager_CanAfford(t *testing.T) {
	m := NewDefaultBudgetManager()
	m.Allocate(1000, 10.0)

	// Initially can afford anything within allocation
	assert.True(t, m.CanAfford(PhaseStrategicPlanning, 100))
	assert.True(t, m.CanAfford(PhaseStrategicPlanning, 50))
	assert.False(t, m.CanAfford(PhaseStrategicPlanning, 101))

	// Consume some
	require.NoError(t, m.Consume(PhaseStrategicPlanning, 70, 0.7))

	// Now can only afford up to 30
	assert.True(t, m.CanAfford(PhaseStrategicPlanning, 30))
	assert.True(t, m.CanAfford(PhaseStrategicPlanning, 20))
	assert.False(t, m.CanAfford(PhaseStrategicPlanning, 31))
}

func TestDefaultBudgetManager_GetReport(t *testing.T) {
	m := NewDefaultBudgetManager()
	m.Allocate(1000, 10.0)

	// Consume various amounts across phases
	require.NoError(t, m.Consume(PhaseStrategicPlanning, 80, 0.8))
	require.NoError(t, m.Consume(PhaseScoring, 40, 0.4))
	require.NoError(t, m.Consume(PhaseReplanning, 30, 0.3))
	require.NoError(t, m.Consume(PhaseExecution, 500, 5.0))

	report := m.GetReport()

	// Verify allocation
	assert.Equal(t, 100, report.Allocation.StrategicPlanning)
	assert.Equal(t, 50, report.Allocation.ScoringOverhead)
	assert.Equal(t, 50, report.Allocation.ReplanReserve)
	assert.Equal(t, 800, report.Allocation.Execution)

	// Verify usage
	assert.Equal(t, 80, report.Usage.StrategicPlanning)
	assert.Equal(t, 40, report.Usage.Scoring)
	assert.Equal(t, 30, report.Usage.Replanning)
	assert.Equal(t, 500, report.Usage.Execution)

	// Verify cost usage
	assert.Equal(t, 0.8, report.CostUsage.StrategicPlanning)
	assert.Equal(t, 0.4, report.CostUsage.Scoring)
	assert.Equal(t, 0.3, report.CostUsage.Replanning)
	assert.Equal(t, 5.0, report.CostUsage.Execution)

	// Verify planning overhead calculation
	// Planning overhead = (80 + 40 + 30) / (80 + 40 + 30 + 500) * 100 = 150 / 650 * 100 ≈ 23.08%
	assert.InDelta(t, 23.08, report.PlanningOverhead, 0.1)

	// Should be "warning" (between 20% and 30%)
	assert.Equal(t, "warning", report.EfficiencyRating)

	// Timestamp should be set
	assert.False(t, report.GeneratedAt.IsZero())
}

func TestDefaultBudgetManager_GetReport_EfficiencyRatings(t *testing.T) {
	tests := []struct {
		name            string
		planningTokens  int
		scoringTokens   int
		replanTokens    int
		executionTokens int
		wantRating      string
		wantOverheadMin float64
		wantOverheadMax float64
	}{
		{
			name:            "good efficiency - low overhead",
			planningTokens:  50,
			scoringTokens:   25,
			replanTokens:    25,
			executionTokens: 800, // Within execution budget allocation
			wantRating:      "good",
			wantOverheadMin: 10.0,
			wantOverheadMax: 12.0,
		},
		{
			name:            "warning efficiency - moderate overhead",
			planningTokens:  100,
			scoringTokens:   50,
			replanTokens:    50,
			executionTokens: 600,
			wantRating:      "warning",
			wantOverheadMin: 24.0,
			wantOverheadMax: 26.0,
		},
		{
			name:            "excessive efficiency - high overhead",
			planningTokens:  100,
			scoringTokens:   50,
			replanTokens:    50,
			executionTokens: 400,
			wantRating:      "excessive",
			wantOverheadMin: 32.0,
			wantOverheadMax: 34.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewDefaultBudgetManager()
			m.Allocate(1000, 10.0)

			// Consume according to test case
			require.NoError(t, m.Consume(PhaseStrategicPlanning, tt.planningTokens, 0.0))
			require.NoError(t, m.Consume(PhaseScoring, tt.scoringTokens, 0.0))
			require.NoError(t, m.Consume(PhaseReplanning, tt.replanTokens, 0.0))
			require.NoError(t, m.Consume(PhaseExecution, tt.executionTokens, 0.0))

			report := m.GetReport()

			assert.Equal(t, tt.wantRating, report.EfficiencyRating)
			assert.GreaterOrEqual(t, report.PlanningOverhead, tt.wantOverheadMin)
			assert.LessOrEqual(t, report.PlanningOverhead, tt.wantOverheadMax)
		})
	}
}

func TestDefaultBudgetManager_ConcurrentAccess(t *testing.T) {
	m := NewDefaultBudgetManager()
	m.Allocate(10000, 100.0)

	// Run concurrent consumers
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Phase allocations: Strategic=1000, Scoring=500, Replanning=500, Execution=8000
	// We'll have goroutines compete for budget

	// Strategic planning consumers (10 goroutines x 100 tokens = 1000 total - exactly the budget)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := m.Consume(PhaseStrategicPlanning, 100, 1.0)
			if err != nil {
				errors <- err
			}
		}()
	}

	// Scoring consumers (10 goroutines x 50 tokens = 500 total - exactly the budget)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := m.Consume(PhaseScoring, 50, 0.5)
			if err != nil {
				errors <- err
			}
		}()
	}

	// Execution consumers (20 goroutines x 400 tokens = 8000 total - exactly the budget)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := m.Consume(PhaseExecution, 400, 4.0)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Verify no race conditions occurred (all operations completed)
	// Some may have failed due to budget exhaustion, which is correct behavior
	report := m.GetReport()

	// Total consumed should not exceed allocations
	assert.LessOrEqual(t, report.Usage.StrategicPlanning, report.Allocation.StrategicPlanning)
	assert.LessOrEqual(t, report.Usage.Scoring, report.Allocation.ScoringOverhead)
	assert.LessOrEqual(t, report.Usage.Execution, report.Allocation.Execution)

	// At least some should have succeeded
	assert.Greater(t, report.Usage.StrategicPlanning, 0)
	assert.Greater(t, report.Usage.Scoring, 0)
	assert.Greater(t, report.Usage.Execution, 0)
}

func TestDefaultBudgetManager_ConcurrentReads(t *testing.T) {
	m := NewDefaultBudgetManager()
	m.Allocate(1000, 10.0)

	// Consume some budget
	require.NoError(t, m.Consume(PhaseStrategicPlanning, 50, 0.5))

	// Run many concurrent readers
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// These should all be safe to call concurrently
			m.Remaining(PhaseStrategicPlanning)
			m.CanAfford(PhaseStrategicPlanning, 10)
			m.GetReport()
		}()
	}

	wg.Wait()
	// Test passes if no race condition detected
}

func TestDefaultBudgetManager_NegativeBudgetPrevention(t *testing.T) {
	m := NewDefaultBudgetManager()
	m.Allocate(100, 1.0) // Small budget: Strategic=10, Scoring=5, Replan=5, Exec=80

	// Try to consume more than allocated
	err := m.Consume(PhaseStrategicPlanning, 11, 0.11)
	require.Error(t, err)

	// Verify usage is still zero (consumption was rejected)
	report := m.GetReport()
	assert.Equal(t, 0, report.Usage.StrategicPlanning)

	// Consume up to the limit
	require.NoError(t, m.Consume(PhaseStrategicPlanning, 10, 0.1))

	// Try to consume 1 more - should fail
	err = m.Consume(PhaseStrategicPlanning, 1, 0.01)
	require.Error(t, err)

	// Usage should still be 10, not 11
	report = m.GetReport()
	assert.Equal(t, 10, report.Usage.StrategicPlanning)
}

func TestDefaultBudgetManager_MultipleAllocations(t *testing.T) {
	m := NewDefaultBudgetManager()

	// First allocation
	alloc1 := m.Allocate(1000, 10.0)
	assert.Equal(t, 1000, alloc1.TotalTokens)

	// Consume some
	require.NoError(t, m.Consume(PhaseStrategicPlanning, 50, 0.5))

	// Second allocation (e.g., budget adjustment) - should reset tracking
	alloc2 := m.Allocate(2000, 20.0)
	assert.Equal(t, 2000, alloc2.TotalTokens)

	// Previous consumption still tracked (this is current behavior)
	// In a real scenario, you might want to reset usage on reallocation
	// but the current implementation doesn't do that
	report := m.GetReport()
	assert.Equal(t, 50, report.Usage.StrategicPlanning)
}

// Helper function to get allocated budget for a phase
func getAllocatedForPhase(alloc BudgetAllocation, phase BudgetPhase) int {
	switch phase {
	case PhaseStrategicPlanning:
		return alloc.StrategicPlanning
	case PhaseScoring:
		return alloc.ScoringOverhead
	case PhaseReplanning:
		return alloc.ReplanReserve
	case PhaseExecution:
		return alloc.Execution
	default:
		return 0
	}
}
