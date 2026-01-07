package mission

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zero-day-ai/gibson/internal/planning"
)

// TestMissionOrchestrator_WithPlanningOrchestrator verifies that planning orchestrator
// can be properly integrated into the mission orchestrator.
func TestMissionOrchestrator_WithPlanningOrchestrator(t *testing.T) {
	// Create a mock mission store
	store := &mockMissionStore{}

	// Create a planning orchestrator with deterministic scorer
	planningOrch := planning.NewPlanningOrchestrator(
		planning.WithScorer(planning.NewDeterministicScorer()),
	)

	// Create mission orchestrator with planning
	orch := NewMissionOrchestrator(
		store,
		WithPlanningOrchestrator(planningOrch),
	)

	// Verify the planning orchestrator is set
	assert.NotNil(t, orch.planningOrchestrator, "Planning orchestrator should be set")
	assert.Same(t, planningOrch, orch.planningOrchestrator, "Should be the same instance")
}

// TestMissionOrchestrator_WithoutPlanningOrchestrator verifies that mission orchestrator
// works without planning (backward compatibility).
func TestMissionOrchestrator_WithoutPlanningOrchestrator(t *testing.T) {
	// Create a mock mission store
	store := &mockMissionStore{}

	// Create mission orchestrator WITHOUT planning
	orch := NewMissionOrchestrator(store)

	// Verify the planning orchestrator is nil (backward compatible)
	assert.Nil(t, orch.planningOrchestrator, "Planning orchestrator should be nil when not provided")
}
