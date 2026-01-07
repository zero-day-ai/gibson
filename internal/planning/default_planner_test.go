package planning

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

func TestDefaultOrderPlanner_Plan(t *testing.T) {
	tests := []struct {
		name           string
		workflow       *workflow.Workflow
		bounds         *PlanningBounds
		budget         *BudgetAllocation
		wantErr        bool
		wantNodeCount  int
		validateOrder  func(t *testing.T, plan *StrategicPlan, wf *workflow.Workflow)
		validateBudget func(t *testing.T, plan *StrategicPlan, budget *BudgetAllocation)
	}{
		{
			name: "simple linear workflow",
			workflow: &workflow.Workflow{
				ID:   types.NewID(),
				Name: "linear",
				Nodes: map[string]*workflow.WorkflowNode{
					"step1": {
						ID:           "step1",
						Type:         workflow.NodeTypeAgent,
						AgentName:    "agent1",
						Dependencies: []string{},
					},
					"step2": {
						ID:           "step2",
						Type:         workflow.NodeTypeAgent,
						AgentName:    "agent2",
						Dependencies: []string{"step1"},
					},
					"step3": {
						ID:           "step3",
						Type:         workflow.NodeTypeAgent,
						AgentName:    "agent3",
						Dependencies: []string{"step2"},
					},
				},
			},
			bounds: &PlanningBounds{
				AllowedNodes: map[string]bool{
					"step1": true,
					"step2": true,
					"step3": true,
				},
			},
			budget: &BudgetAllocation{
				Execution:   900,
				TotalTokens: 1000,
			},
			wantErr:       false,
			wantNodeCount: 3,
			validateOrder: func(t *testing.T, plan *StrategicPlan, wf *workflow.Workflow) {
				// Verify topological order: step1 before step2 before step3
				require.Len(t, plan.OrderedSteps, 3)

				nodePositions := make(map[string]int)
				for i, step := range plan.OrderedSteps {
					nodePositions[step.NodeID] = i
				}

				assert.Less(t, nodePositions["step1"], nodePositions["step2"], "step1 must come before step2")
				assert.Less(t, nodePositions["step2"], nodePositions["step3"], "step2 must come before step3")
			},
			validateBudget: func(t *testing.T, plan *StrategicPlan, budget *BudgetAllocation) {
				// Budget should be distributed equally
				totalAllocated := 0
				for _, allocation := range plan.StepBudgets {
					totalAllocated += allocation
				}
				assert.Equal(t, budget.Execution, totalAllocated, "total allocated should equal execution budget")
			},
		},
		{
			name: "parallel dependencies (diamond)",
			workflow: &workflow.Workflow{
				ID:   types.NewID(),
				Name: "diamond",
				Nodes: map[string]*workflow.WorkflowNode{
					"start": {
						ID:           "start",
						Type:         workflow.NodeTypeAgent,
						AgentName:    "starter",
						Dependencies: []string{},
					},
					"branch1": {
						ID:           "branch1",
						Type:         workflow.NodeTypeAgent,
						AgentName:    "agent1",
						Dependencies: []string{"start"},
					},
					"branch2": {
						ID:           "branch2",
						Type:         workflow.NodeTypeAgent,
						AgentName:    "agent2",
						Dependencies: []string{"start"},
					},
					"end": {
						ID:           "end",
						Type:         workflow.NodeTypeAgent,
						AgentName:    "ender",
						Dependencies: []string{"branch1", "branch2"},
					},
				},
			},
			bounds: &PlanningBounds{
				AllowedNodes: map[string]bool{
					"start":   true,
					"branch1": true,
					"branch2": true,
					"end":     true,
				},
			},
			budget: &BudgetAllocation{
				Execution:   800,
				TotalTokens: 1000,
			},
			wantErr:       false,
			wantNodeCount: 4,
			validateOrder: func(t *testing.T, plan *StrategicPlan, wf *workflow.Workflow) {
				require.Len(t, plan.OrderedSteps, 4)

				nodePositions := make(map[string]int)
				for i, step := range plan.OrderedSteps {
					nodePositions[step.NodeID] = i
				}

				// Verify dependency constraints
				assert.Less(t, nodePositions["start"], nodePositions["branch1"], "start before branch1")
				assert.Less(t, nodePositions["start"], nodePositions["branch2"], "start before branch2")
				assert.Less(t, nodePositions["branch1"], nodePositions["end"], "branch1 before end")
				assert.Less(t, nodePositions["branch2"], nodePositions["end"], "branch2 before end")
			},
			validateBudget: func(t *testing.T, plan *StrategicPlan, budget *BudgetAllocation) {
				totalAllocated := 0
				for _, allocation := range plan.StepBudgets {
					totalAllocated += allocation
				}
				assert.Equal(t, budget.Execution, totalAllocated)
			},
		},
		{
			name: "single node workflow",
			workflow: &workflow.Workflow{
				ID:   types.NewID(),
				Name: "single",
				Nodes: map[string]*workflow.WorkflowNode{
					"only": {
						ID:           "only",
						Type:         workflow.NodeTypeAgent,
						AgentName:    "agent",
						Dependencies: []string{},
					},
				},
			},
			bounds: &PlanningBounds{
				AllowedNodes: map[string]bool{
					"only": true,
				},
			},
			budget: &BudgetAllocation{
				Execution:   1000,
				TotalTokens: 1000,
			},
			wantErr:       false,
			wantNodeCount: 1,
			validateOrder: func(t *testing.T, plan *StrategicPlan, wf *workflow.Workflow) {
				require.Len(t, plan.OrderedSteps, 1)
				assert.Equal(t, "only", plan.OrderedSteps[0].NodeID)
			},
			validateBudget: func(t *testing.T, plan *StrategicPlan, budget *BudgetAllocation) {
				assert.Equal(t, budget.Execution, plan.StepBudgets["only"])
			},
		},
		{
			name: "complex multi-level dependencies",
			workflow: &workflow.Workflow{
				ID:   types.NewID(),
				Name: "complex",
				Nodes: map[string]*workflow.WorkflowNode{
					"a": {ID: "a", Type: workflow.NodeTypeAgent, AgentName: "agent_a", Dependencies: []string{}},
					"b": {ID: "b", Type: workflow.NodeTypeAgent, AgentName: "agent_b", Dependencies: []string{"a"}},
					"c": {ID: "c", Type: workflow.NodeTypeAgent, AgentName: "agent_c", Dependencies: []string{"a"}},
					"d": {ID: "d", Type: workflow.NodeTypeAgent, AgentName: "agent_d", Dependencies: []string{"b", "c"}},
					"e": {ID: "e", Type: workflow.NodeTypeAgent, AgentName: "agent_e", Dependencies: []string{"c"}},
					"f": {ID: "f", Type: workflow.NodeTypeAgent, AgentName: "agent_f", Dependencies: []string{"d", "e"}},
				},
			},
			bounds: &PlanningBounds{
				AllowedNodes: map[string]bool{
					"a": true, "b": true, "c": true, "d": true, "e": true, "f": true,
				},
			},
			budget: &BudgetAllocation{
				Execution:   600,
				TotalTokens: 1000,
			},
			wantErr:       false,
			wantNodeCount: 6,
			validateOrder: func(t *testing.T, plan *StrategicPlan, wf *workflow.Workflow) {
				require.Len(t, plan.OrderedSteps, 6)

				nodePositions := make(map[string]int)
				for i, step := range plan.OrderedSteps {
					nodePositions[step.NodeID] = i
				}

				// Verify all dependency constraints
				assert.Less(t, nodePositions["a"], nodePositions["b"])
				assert.Less(t, nodePositions["a"], nodePositions["c"])
				assert.Less(t, nodePositions["b"], nodePositions["d"])
				assert.Less(t, nodePositions["c"], nodePositions["d"])
				assert.Less(t, nodePositions["c"], nodePositions["e"])
				assert.Less(t, nodePositions["d"], nodePositions["f"])
				assert.Less(t, nodePositions["e"], nodePositions["f"])
			},
			validateBudget: func(t *testing.T, plan *StrategicPlan, budget *BudgetAllocation) {
				totalAllocated := 0
				for _, allocation := range plan.StepBudgets {
					totalAllocated += allocation
				}
				assert.Equal(t, budget.Execution, totalAllocated)

				// Check distribution is roughly equal
				avgBudget := budget.Execution / 6
				for _, allocation := range plan.StepBudgets {
					assert.InDelta(t, avgBudget, allocation, float64(avgBudget)*0.5)
				}
			},
		},
		{
			name: "zero budget",
			workflow: &workflow.Workflow{
				ID:   types.NewID(),
				Name: "zero_budget",
				Nodes: map[string]*workflow.WorkflowNode{
					"step1": {ID: "step1", Type: workflow.NodeTypeAgent, AgentName: "agent1", Dependencies: []string{}},
				},
			},
			bounds: &PlanningBounds{
				AllowedNodes: map[string]bool{"step1": true},
			},
			budget: &BudgetAllocation{
				Execution:   0,
				TotalTokens: 0,
			},
			wantErr:       false,
			wantNodeCount: 1,
			validateOrder: func(t *testing.T, plan *StrategicPlan, wf *workflow.Workflow) {
				require.Len(t, plan.OrderedSteps, 1)
			},
			validateBudget: func(t *testing.T, plan *StrategicPlan, budget *BudgetAllocation) {
				assert.Equal(t, 0, plan.StepBudgets["step1"])
			},
		},
		{
			name:     "nil workflow",
			workflow: nil,
			bounds:   &PlanningBounds{},
			budget:   &BudgetAllocation{},
			wantErr:  true,
		},
		{
			name: "nil bounds",
			workflow: &workflow.Workflow{
				ID:   types.NewID(),
				Name: "test",
				Nodes: map[string]*workflow.WorkflowNode{
					"step1": {ID: "step1", Type: workflow.NodeTypeAgent, AgentName: "agent1"},
				},
			},
			bounds:  nil,
			budget:  &BudgetAllocation{},
			wantErr: true,
		},
		{
			name: "empty workflow",
			workflow: &workflow.Workflow{
				ID:    types.NewID(),
				Name:  "empty",
				Nodes: map[string]*workflow.WorkflowNode{},
			},
			bounds:  &PlanningBounds{},
			budget:  &BudgetAllocation{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planner := NewDefaultOrderPlanner()
			require.NotNil(t, planner)

			input := StrategicPlanInput{
				Workflow: tt.workflow,
				Bounds:   tt.bounds,
				Budget:   tt.budget,
			}

			plan, err := planner.Plan(context.Background(), input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, plan)

			// Validate basic properties
			assert.Len(t, plan.OrderedSteps, tt.wantNodeCount)
			assert.NotEmpty(t, plan.Rationale)
			assert.Empty(t, plan.SkippedNodes, "default planner should not skip nodes")
			assert.Empty(t, plan.ParallelGroups, "default planner should not create parallel groups")

			// Validate priorities are sequential
			for i, step := range plan.OrderedSteps {
				assert.Equal(t, i, step.Priority, "priorities should be sequential")
			}

			// Run custom validators if provided
			if tt.validateOrder != nil {
				tt.validateOrder(t, plan, tt.workflow)
			}

			if tt.validateBudget != nil {
				tt.validateBudget(t, plan, tt.budget)
			}
		})
	}
}

func TestDefaultOrderPlanner_TopologicalSort_CircularDependency(t *testing.T) {
	// Create a workflow with circular dependency: A -> B -> C -> A
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "circular",
		Nodes: map[string]*workflow.WorkflowNode{
			"a": {
				ID:           "a",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "agent_a",
				Dependencies: []string{"c"},
			},
			"b": {
				ID:           "b",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "agent_b",
				Dependencies: []string{"a"},
			},
			"c": {
				ID:           "c",
				Type:         workflow.NodeTypeAgent,
				AgentName:    "agent_c",
				Dependencies: []string{"b"},
			},
		},
	}

	planner := NewDefaultOrderPlanner()

	input := StrategicPlanInput{
		Workflow: wf,
		Bounds: &PlanningBounds{
			AllowedNodes: map[string]bool{
				"a": true,
				"b": true,
				"c": true,
			},
		},
		Budget: &BudgetAllocation{
			Execution: 300,
		},
	}

	plan, err := planner.Plan(context.Background(), input)

	assert.Error(t, err)
	assert.Nil(t, plan)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestDefaultOrderPlanner_BudgetRounding(t *testing.T) {
	// Test with budget that doesn't divide evenly
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "rounding",
		Nodes: map[string]*workflow.WorkflowNode{
			"step1": {ID: "step1", Type: workflow.NodeTypeAgent, AgentName: "agent1", Dependencies: []string{}},
			"step2": {ID: "step2", Type: workflow.NodeTypeAgent, AgentName: "agent2", Dependencies: []string{"step1"}},
			"step3": {ID: "step3", Type: workflow.NodeTypeAgent, AgentName: "agent3", Dependencies: []string{"step2"}},
		},
	}

	planner := NewDefaultOrderPlanner()

	input := StrategicPlanInput{
		Workflow: wf,
		Bounds: &PlanningBounds{
			AllowedNodes: map[string]bool{
				"step1": true,
				"step2": true,
				"step3": true,
			},
		},
		Budget: &BudgetAllocation{
			Execution: 100, // Doesn't divide evenly by 3
		},
	}

	plan, err := planner.Plan(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, plan)

	// Calculate total allocated
	totalAllocated := 0
	for _, allocation := range plan.StepBudgets {
		totalAllocated += allocation
	}

	// Must equal exactly the execution budget (rounding handled correctly)
	assert.Equal(t, 100, totalAllocated, "rounding errors should be corrected")

	// Last step should get the remainder
	lastStep := plan.OrderedSteps[len(plan.OrderedSteps)-1]
	expectedPerStep := 100 / 3   // 33
	expectedRemainder := 100 % 3 // 1
	assert.Equal(t, expectedPerStep+expectedRemainder, plan.StepBudgets[lastStep.NodeID])
}

func TestDefaultOrderPlanner_NilBudget(t *testing.T) {
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "nil_budget",
		Nodes: map[string]*workflow.WorkflowNode{
			"step1": {ID: "step1", Type: workflow.NodeTypeAgent, AgentName: "agent1", Dependencies: []string{}},
		},
	}

	planner := NewDefaultOrderPlanner()

	input := StrategicPlanInput{
		Workflow: wf,
		Bounds: &PlanningBounds{
			AllowedNodes: map[string]bool{"step1": true},
		},
		Budget: nil, // Nil budget
	}

	plan, err := planner.Plan(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, plan)

	// Should handle nil budget gracefully (0 tokens per step)
	assert.Equal(t, 0, plan.StepBudgets["step1"])
}

// Benchmark tests

func BenchmarkDefaultOrderPlanner_Small(b *testing.B) {
	wf := createBenchmarkWorkflow(5)
	planner := NewDefaultOrderPlanner()

	input := StrategicPlanInput{
		Workflow: wf,
		Bounds: &PlanningBounds{
			AllowedNodes: makeAllowedNodes(wf),
		},
		Budget: &BudgetAllocation{
			Execution: 1000,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := planner.Plan(context.Background(), input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDefaultOrderPlanner_Medium(b *testing.B) {
	wf := createBenchmarkWorkflow(20)
	planner := NewDefaultOrderPlanner()

	input := StrategicPlanInput{
		Workflow: wf,
		Bounds: &PlanningBounds{
			AllowedNodes: makeAllowedNodes(wf),
		},
		Budget: &BudgetAllocation{
			Execution: 10000,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := planner.Plan(context.Background(), input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDefaultOrderPlanner_Large(b *testing.B) {
	wf := createBenchmarkWorkflow(50)
	planner := NewDefaultOrderPlanner()

	input := StrategicPlanInput{
		Workflow: wf,
		Bounds: &PlanningBounds{
			AllowedNodes: makeAllowedNodes(wf),
		},
		Budget: &BudgetAllocation{
			Execution: 50000,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := planner.Plan(context.Background(), input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDefaultOrderPlanner_VeryLarge(b *testing.B) {
	wf := createBenchmarkWorkflow(100)
	planner := NewDefaultOrderPlanner()

	input := StrategicPlanInput{
		Workflow: wf,
		Bounds: &PlanningBounds{
			AllowedNodes: makeAllowedNodes(wf),
		},
		Budget: &BudgetAllocation{
			Execution: 100000,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := planner.Plan(context.Background(), input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Helper functions for benchmarks

func createBenchmarkWorkflow(nodeCount int) *workflow.Workflow {
	wf := &workflow.Workflow{
		ID:    types.NewID(),
		Name:  "benchmark",
		Nodes: make(map[string]*workflow.WorkflowNode),
	}

	// Create a linear chain of dependencies for consistent benchmarking
	for i := 0; i < nodeCount; i++ {
		nodeID := fmt.Sprintf("node_%d", i)
		node := &workflow.WorkflowNode{
			ID:        nodeID,
			Type:      workflow.NodeTypeAgent,
			AgentName: fmt.Sprintf("agent_%d", i),
			Timeout:   time.Second * 30,
		}

		// Each node depends on the previous node (except the first)
		if i > 0 {
			node.Dependencies = []string{fmt.Sprintf("node_%d", i-1)}
		} else {
			node.Dependencies = []string{}
		}

		wf.Nodes[nodeID] = node
	}

	return wf
}

func makeAllowedNodes(wf *workflow.Workflow) map[string]bool {
	allowed := make(map[string]bool)
	for nodeID := range wf.Nodes {
		allowed[nodeID] = true
	}
	return allowed
}
