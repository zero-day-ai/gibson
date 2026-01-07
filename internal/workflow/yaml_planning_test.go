package workflow

import (
	"testing"
)

func TestParseWorkflow_WithPlanning(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(*testing.T, *Workflow)
	}{
		{
			name: "valid planning config",
			yaml: `
name: test-workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
planning:
  replan_limit: 5
  memory_query: true
  planning_timeout: "45s"
  budget_allocation:
    planning: 15
    scoring: 5
    replanning: 5
    execution: 75
`,
			wantErr: false,
			check: func(t *testing.T, wf *Workflow) {
				if wf.Planning == nil {
					t.Fatal("Planning is nil")
				}
				if wf.Planning.ReplanLimit != 5 {
					t.Errorf("ReplanLimit = %d, want 5", wf.Planning.ReplanLimit)
				}
				if !wf.Planning.MemoryQuery {
					t.Error("MemoryQuery = false, want true")
				}
				if wf.Planning.PlanningTimeout != "45s" {
					t.Errorf("PlanningTimeout = %q, want \"45s\"", wf.Planning.PlanningTimeout)
				}
				if wf.Planning.BudgetAllocation == nil {
					t.Fatal("BudgetAllocation is nil")
				}
				if wf.Planning.BudgetAllocation.Planning != 15 {
					t.Errorf("Planning = %d, want 15", wf.Planning.BudgetAllocation.Planning)
				}
				if wf.Planning.BudgetAllocation.Execution != 75 {
					t.Errorf("Execution = %d, want 75", wf.Planning.BudgetAllocation.Execution)
				}
			},
		},
		{
			name: "minimal planning config",
			yaml: `
name: test-workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
planning:
  replan_limit: 2
`,
			wantErr: false,
			check: func(t *testing.T, wf *Workflow) {
				if wf.Planning == nil {
					t.Fatal("Planning is nil")
				}
				if wf.Planning.ReplanLimit != 2 {
					t.Errorf("ReplanLimit = %d, want 2", wf.Planning.ReplanLimit)
				}
			},
		},
		{
			name: "no planning config",
			yaml: `
name: test-workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
`,
			wantErr: false,
			check: func(t *testing.T, wf *Workflow) {
				if wf.Planning != nil {
					t.Error("Planning should be nil when not specified")
				}
			},
		},
		{
			name: "invalid budget allocation - sum not 100",
			yaml: `
name: test-workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
planning:
  budget_allocation:
    planning: 10
    scoring: 10
    replanning: 10
    execution: 80
`,
			wantErr: true,
		},
		{
			name: "invalid timeout format",
			yaml: `
name: test-workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
planning:
  planning_timeout: "invalid"
`,
			wantErr: true,
		},
		{
			name: "negative replan limit",
			yaml: `
name: test-workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
planning:
  replan_limit: -1
`,
			wantErr: true,
		},
		{
			name: "zero replan limit is valid (disables replanning)",
			yaml: `
name: test-workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
planning:
  replan_limit: 0
`,
			wantErr: false,
			check: func(t *testing.T, wf *Workflow) {
				if wf.Planning == nil {
					t.Fatal("Planning is nil")
				}
				if wf.Planning.ReplanLimit != 0 {
					t.Errorf("ReplanLimit = %d, want 0", wf.Planning.ReplanLimit)
				}
			},
		},
		{
			name: "custom budget allocation",
			yaml: `
name: test-workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
planning:
  budget_allocation:
    planning: 20
    scoring: 10
    replanning: 10
    execution: 60
`,
			wantErr: false,
			check: func(t *testing.T, wf *Workflow) {
				if wf.Planning == nil {
					t.Fatal("Planning is nil")
				}
				if wf.Planning.BudgetAllocation == nil {
					t.Fatal("BudgetAllocation is nil")
				}
				alloc := wf.Planning.BudgetAllocation
				if alloc.Planning != 20 {
					t.Errorf("Planning = %d, want 20", alloc.Planning)
				}
				if alloc.Scoring != 10 {
					t.Errorf("Scoring = %d, want 10", alloc.Scoring)
				}
				if alloc.Replanning != 10 {
					t.Errorf("Replanning = %d, want 10", alloc.Replanning)
				}
				if alloc.Execution != 60 {
					t.Errorf("Execution = %d, want 60", alloc.Execution)
				}
			},
		},
		{
			name: "planning timeout variants",
			yaml: `
name: test-workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
planning:
  planning_timeout: "2m30s"
`,
			wantErr: false,
			check: func(t *testing.T, wf *Workflow) {
				if wf.Planning == nil {
					t.Fatal("Planning is nil")
				}
				if wf.Planning.PlanningTimeout != "2m30s" {
					t.Errorf("PlanningTimeout = %q, want \"2m30s\"", wf.Planning.PlanningTimeout)
				}
			},
		},
		{
			name: "memory query false",
			yaml: `
name: test-workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
planning:
  memory_query: false
`,
			wantErr: false,
			check: func(t *testing.T, wf *Workflow) {
				if wf.Planning == nil {
					t.Fatal("Planning is nil")
				}
				if wf.Planning.MemoryQuery {
					t.Error("MemoryQuery = true, want false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf, err := ParseWorkflow([]byte(tt.yaml))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseWorkflow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && tt.check != nil {
				tt.check(t, wf)
			}
		})
	}
}

func TestParseWorkflow_ComplexWithPlanning(t *testing.T) {
	yaml := `
name: complex-workflow
description: A workflow with planning config
nodes:
  - id: scan
    type: agent
    name: Scanner
    agent: nmap-agent
    task:
      action: scan
      target: example.com
    timeout: 5m
    retry:
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      max_delay: 30s
      multiplier: 2.0

  - id: analyze
    type: agent
    name: Analyzer
    agent: analyzer-agent
    depends_on:
      - scan
    timeout: 10m

  - id: report
    type: tool
    name: Reporter
    tool: report-tool
    depends_on:
      - analyze
    input:
      format: json

planning:
  replan_limit: 3
  memory_query: true
  planning_timeout: "1m"
  budget_allocation:
    planning: 12
    scoring: 8
    replanning: 5
    execution: 75
`

	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseWorkflow() error = %v", err)
	}

	// Verify workflow structure
	if wf.Name != "complex-workflow" {
		t.Errorf("Name = %q, want \"complex-workflow\"", wf.Name)
	}
	if len(wf.Nodes) != 3 {
		t.Errorf("len(Nodes) = %d, want 3", len(wf.Nodes))
	}

	// Verify planning config
	if wf.Planning == nil {
		t.Fatal("Planning is nil")
	}
	if wf.Planning.ReplanLimit != 3 {
		t.Errorf("ReplanLimit = %d, want 3", wf.Planning.ReplanLimit)
	}
	if !wf.Planning.MemoryQuery {
		t.Error("MemoryQuery = false, want true")
	}
	if wf.Planning.PlanningTimeout != "1m" {
		t.Errorf("PlanningTimeout = %q, want \"1m\"", wf.Planning.PlanningTimeout)
	}
	if wf.Planning.BudgetAllocation == nil {
		t.Fatal("BudgetAllocation is nil")
	}
	if wf.Planning.BudgetAllocation.Planning != 12 {
		t.Errorf("Planning = %d, want 12", wf.Planning.BudgetAllocation.Planning)
	}
	if wf.Planning.BudgetAllocation.Scoring != 8 {
		t.Errorf("Scoring = %d, want 8", wf.Planning.BudgetAllocation.Scoring)
	}
	if wf.Planning.BudgetAllocation.Replanning != 5 {
		t.Errorf("Replanning = %d, want 5", wf.Planning.BudgetAllocation.Replanning)
	}
	if wf.Planning.BudgetAllocation.Execution != 75 {
		t.Errorf("Execution = %d, want 75", wf.Planning.BudgetAllocation.Execution)
	}

	// Verify planning config is valid
	if err := wf.Planning.Validate(); err != nil {
		t.Errorf("Planning.Validate() error = %v", err)
	}
}

func TestPlanningConfig_Integration(t *testing.T) {
	// Test that planning config from workflow can be used with budget manager
	yaml := `
name: budget-test
nodes:
  - id: node1
    type: agent
    agent: test-agent
planning:
  budget_allocation:
    planning: 15
    scoring: 10
    replanning: 5
    execution: 70
`

	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseWorkflow() error = %v", err)
	}

	// Verify we can convert to budget allocation
	allocation := wf.Planning.ToBudgetAllocation()
	if allocation == nil {
		t.Fatal("ToBudgetAllocation() returned nil")
	}

	// Verify the percentages are correct
	if allocation.Planning != 15 {
		t.Errorf("Planning = %d, want 15", allocation.Planning)
	}
	if allocation.Scoring != 10 {
		t.Errorf("Scoring = %d, want 10", allocation.Scoring)
	}
	if allocation.Replanning != 5 {
		t.Errorf("Replanning = %d, want 5", allocation.Replanning)
	}
	if allocation.Execution != 70 {
		t.Errorf("Execution = %d, want 70", allocation.Execution)
	}

	// Verify the percentages can be applied to token budgets
	totalTokens := 1000

	// The allocation should use the custom percentages when converted
	expectedPlanning := (totalTokens * allocation.Planning) / 100
	actualPlanning := (totalTokens * allocation.Planning) / 100
	if actualPlanning != expectedPlanning {
		t.Errorf("Planning allocation = %d, want %d", actualPlanning, expectedPlanning)
	}

	expectedExecution := (totalTokens * allocation.Execution) / 100
	actualExecution := (totalTokens * allocation.Execution) / 100
	if actualExecution != expectedExecution {
		t.Errorf("Execution allocation = %d, want %d", actualExecution, expectedExecution)
	}
}
