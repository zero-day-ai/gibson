package planning

import (
	"fmt"
	"sync"
	"time"
)

// BudgetPhase represents a phase in the mission planning lifecycle.
type BudgetPhase string

const (
	// PhaseStrategicPlanning is the pre-execution planning phase (10% of budget).
	PhaseStrategicPlanning BudgetPhase = "strategic_planning"

	// PhaseScoring is the step evaluation phase (5% of budget).
	PhaseScoring BudgetPhase = "scoring"

	// PhaseReplanning is the mid-execution replanning phase (5% of budget).
	PhaseReplanning BudgetPhase = "replanning"

	// PhaseExecution is the actual workflow execution phase (80% of budget).
	PhaseExecution BudgetPhase = "execution"
)

// DefaultBudgetPercentages define the standard allocation percentages across phases.
const (
	DefaultStrategicPlanningPercent = 10
	DefaultScoringPercent           = 5
	DefaultReplanningPercent        = 5
	DefaultExecutionPercent         = 80
)

// BudgetAllocation represents the distribution of mission budget across planning phases.
type BudgetAllocation struct {
	// StrategicPlanning tokens allocated for pre-execution planning (10% default).
	StrategicPlanning int

	// ScoringOverhead tokens allocated for step scoring (5% default).
	ScoringOverhead int

	// ReplanReserve tokens allocated for mid-execution replanning (5% default).
	ReplanReserve int

	// Execution tokens allocated for actual workflow execution (80% default).
	Execution int

	// TotalTokens is the total token budget for the mission.
	TotalTokens int

	// TotalCost is the total cost budget for the mission.
	TotalCost float64
}

// BudgetReport provides a summary of budget allocation and usage.
type BudgetReport struct {
	// Allocation is the original budget allocation across phases.
	Allocation BudgetAllocation

	// Usage tracks actual consumption per phase.
	Usage struct {
		StrategicPlanning int
		Scoring           int
		Replanning        int
		Execution         int
	}

	// CostUsage tracks actual cost consumption per phase.
	CostUsage struct {
		StrategicPlanning float64
		Scoring           float64
		Replanning        float64
		Execution         float64
	}

	// Efficiency metrics.
	PlanningOverhead float64 // Percentage of total spent on planning (planning + scoring + replanning).
	EfficiencyRating string  // "good", "warning", or "excessive".

	// Timestamps.
	GeneratedAt time.Time
}

// BudgetManager allocates and tracks token/cost budgets across planning phases.
type BudgetManager interface {
	// Allocate distributes the mission budget across phases using the standard allocation.
	Allocate(totalTokens int, totalCost float64) *BudgetAllocation

	// Consume records usage against a specific phase.
	// Returns an error if the phase budget is exhausted.
	Consume(phase BudgetPhase, tokens int, cost float64) error

	// Remaining returns the remaining budget for a phase.
	Remaining(phase BudgetPhase) (tokens int, cost float64)

	// CanAfford checks if an operation fits within the phase budget.
	CanAfford(phase BudgetPhase, estimatedTokens int) bool

	// GetReport returns a breakdown of allocated vs actual usage.
	GetReport() *BudgetReport
}

// DefaultBudgetManager implements BudgetManager with thread-safe tracking.
type DefaultBudgetManager struct {
	mu         sync.RWMutex
	allocation BudgetAllocation

	// Tracking structures for each phase.
	used struct {
		strategicPlanning int
		scoring           int
		replanning        int
		execution         int
	}

	costUsed struct {
		strategicPlanning float64
		scoring           float64
		replanning        float64
		execution         float64
	}
}

// NewDefaultBudgetManager creates a new DefaultBudgetManager.
func NewDefaultBudgetManager() *DefaultBudgetManager {
	return &DefaultBudgetManager{}
}

// Allocate distributes the mission budget across phases using default percentages.
func (m *DefaultBudgetManager) Allocate(totalTokens int, totalCost float64) *BudgetAllocation {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Calculate token allocation based on percentages.
	m.allocation = BudgetAllocation{
		StrategicPlanning: (totalTokens * DefaultStrategicPlanningPercent) / 100,
		ScoringOverhead:   (totalTokens * DefaultScoringPercent) / 100,
		ReplanReserve:     (totalTokens * DefaultReplanningPercent) / 100,
		Execution:         (totalTokens * DefaultExecutionPercent) / 100,
		TotalTokens:       totalTokens,
		TotalCost:         totalCost,
	}

	// Adjust execution to account for rounding errors - ensure sum equals total.
	allocated := m.allocation.StrategicPlanning + m.allocation.ScoringOverhead + m.allocation.ReplanReserve + m.allocation.Execution
	if allocated != totalTokens {
		m.allocation.Execution += totalTokens - allocated
	}

	return &m.allocation
}

// Consume records usage against a specific phase.
func (m *DefaultBudgetManager) Consume(phase BudgetPhase, tokens int, cost float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get current usage and allocation for the phase.
	currentTokens, allocated := m.getPhaseUsageAndAllocation(phase)

	// Check if we would exceed the budget.
	if currentTokens+tokens > allocated {
		return fmt.Errorf("budget exhausted for phase %s: requested %d tokens, have %d/%d remaining",
			phase, tokens, allocated-currentTokens, allocated)
	}

	// Record the consumption.
	m.recordConsumption(phase, tokens, cost)

	return nil
}

// Remaining returns the remaining budget for a phase.
func (m *DefaultBudgetManager) Remaining(phase BudgetPhase) (tokens int, cost float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	used, allocated := m.getPhaseUsageAndAllocation(phase)
	costUsed := m.getPhaseCostUsage(phase)

	// Calculate remaining based on proportional allocation.
	tokensRemaining := allocated - used
	if tokensRemaining < 0 {
		tokensRemaining = 0
	}

	// Cost is proportionally allocated based on token usage.
	totalCostAllocated := m.allocation.TotalCost * float64(allocated) / float64(m.allocation.TotalTokens)
	costRemaining := totalCostAllocated - costUsed
	if costRemaining < 0 {
		costRemaining = 0
	}

	return tokensRemaining, costRemaining
}

// CanAfford checks if an operation fits within the phase budget.
func (m *DefaultBudgetManager) CanAfford(phase BudgetPhase, estimatedTokens int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	used, allocated := m.getPhaseUsageAndAllocation(phase)
	return used+estimatedTokens <= allocated
}

// GetReport returns a breakdown of allocated vs actual usage.
func (m *DefaultBudgetManager) GetReport() *BudgetReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	report := &BudgetReport{
		Allocation:  m.allocation,
		GeneratedAt: time.Now(),
	}

	// Populate usage statistics.
	report.Usage.StrategicPlanning = m.used.strategicPlanning
	report.Usage.Scoring = m.used.scoring
	report.Usage.Replanning = m.used.replanning
	report.Usage.Execution = m.used.execution

	report.CostUsage.StrategicPlanning = m.costUsed.strategicPlanning
	report.CostUsage.Scoring = m.costUsed.scoring
	report.CostUsage.Replanning = m.costUsed.replanning
	report.CostUsage.Execution = m.costUsed.execution

	// Calculate planning overhead.
	totalUsed := report.Usage.StrategicPlanning + report.Usage.Scoring +
		report.Usage.Replanning + report.Usage.Execution

	if totalUsed > 0 {
		planningTokens := report.Usage.StrategicPlanning + report.Usage.Scoring + report.Usage.Replanning
		report.PlanningOverhead = float64(planningTokens) / float64(totalUsed) * 100

		// Determine efficiency rating.
		switch {
		case report.PlanningOverhead <= 20:
			report.EfficiencyRating = "good"
		case report.PlanningOverhead <= 30:
			report.EfficiencyRating = "warning"
		default:
			report.EfficiencyRating = "excessive"
		}
	} else {
		report.PlanningOverhead = 0
		report.EfficiencyRating = "good"
	}

	return report
}

// getPhaseUsageAndAllocation returns current usage and allocation for a phase.
// Must be called with lock held.
func (m *DefaultBudgetManager) getPhaseUsageAndAllocation(phase BudgetPhase) (used int, allocated int) {
	switch phase {
	case PhaseStrategicPlanning:
		return m.used.strategicPlanning, m.allocation.StrategicPlanning
	case PhaseScoring:
		return m.used.scoring, m.allocation.ScoringOverhead
	case PhaseReplanning:
		return m.used.replanning, m.allocation.ReplanReserve
	case PhaseExecution:
		return m.used.execution, m.allocation.Execution
	default:
		return 0, 0
	}
}

// getPhaseCostUsage returns current cost usage for a phase.
// Must be called with lock held.
func (m *DefaultBudgetManager) getPhaseCostUsage(phase BudgetPhase) float64 {
	switch phase {
	case PhaseStrategicPlanning:
		return m.costUsed.strategicPlanning
	case PhaseScoring:
		return m.costUsed.scoring
	case PhaseReplanning:
		return m.costUsed.replanning
	case PhaseExecution:
		return m.costUsed.execution
	default:
		return 0
	}
}

// recordConsumption records token and cost usage for a phase.
// Must be called with lock held.
func (m *DefaultBudgetManager) recordConsumption(phase BudgetPhase, tokens int, cost float64) {
	switch phase {
	case PhaseStrategicPlanning:
		m.used.strategicPlanning += tokens
		m.costUsed.strategicPlanning += cost
	case PhaseScoring:
		m.used.scoring += tokens
		m.costUsed.scoring += cost
	case PhaseReplanning:
		m.used.replanning += tokens
		m.costUsed.replanning += cost
	case PhaseExecution:
		m.used.execution += tokens
		m.costUsed.execution += cost
	}
}
