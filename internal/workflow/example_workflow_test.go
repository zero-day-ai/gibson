package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExampleWorkflowWithPlanning(t *testing.T) {
	// Check if examples directory exists
	examplePath := filepath.Join("..", "..", "examples", "workflow-with-planning.yaml")
	if _, err := os.Stat(examplePath); os.IsNotExist(err) {
		t.Skip("Example workflow file not found, skipping test")
	}

	wf, err := ParseWorkflowFile(examplePath)
	if err != nil {
		t.Fatalf("Failed to parse example workflow: %v", err)
	}

	// Verify basic workflow structure
	if wf.Name != "security-assessment-with-planning" {
		t.Errorf("Name = %q, want \"security-assessment-with-planning\"", wf.Name)
	}

	if len(wf.Nodes) != 5 {
		t.Errorf("len(Nodes) = %d, want 5", len(wf.Nodes))
	}

	// Verify planning configuration
	if wf.Planning == nil {
		t.Fatal("Planning config is nil")
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

	// Verify budget allocation
	if wf.Planning.BudgetAllocation == nil {
		t.Fatal("BudgetAllocation is nil")
	}

	alloc := wf.Planning.BudgetAllocation
	if alloc.Planning != 15 {
		t.Errorf("Planning = %d, want 15", alloc.Planning)
	}
	if alloc.Scoring != 5 {
		t.Errorf("Scoring = %d, want 5", alloc.Scoring)
	}
	if alloc.Replanning != 5 {
		t.Errorf("Replanning = %d, want 5", alloc.Replanning)
	}
	if alloc.Execution != 75 {
		t.Errorf("Execution = %d, want 75", alloc.Execution)
	}

	// Verify the config is valid
	if err := wf.Planning.Validate(); err != nil {
		t.Errorf("Planning.Validate() error = %v", err)
	}

	// Verify workflow structure
	nodes := []string{"recon", "vulnerability-scan", "exploit-selection", "exploitation", "report"}
	for _, nodeID := range nodes {
		if wf.Nodes[nodeID] == nil {
			t.Errorf("Node %q not found", nodeID)
		}
	}

	// Verify dependencies
	if len(wf.Nodes["recon"].Dependencies) != 0 {
		t.Error("recon should have no dependencies")
	}
	if len(wf.Nodes["vulnerability-scan"].Dependencies) != 1 {
		t.Error("vulnerability-scan should depend on recon")
	}
	if len(wf.Nodes["exploit-selection"].Dependencies) != 1 {
		t.Error("exploit-selection should depend on vulnerability-scan")
	}
}
