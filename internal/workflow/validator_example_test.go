package workflow

import (
	"fmt"
	"slices"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// Example demonstrates how to use the DAGValidator to validate a workflow.
func ExampleDAGValidator_Validate() {
	validator := NewDAGValidator()

	// Create a valid workflow with a diamond pattern
	workflow := &Workflow{
		ID:   types.NewID(),
		Name: "example-workflow",
		Nodes: map[string]*WorkflowNode{
			"start": {
				ID:   "start",
				Type: NodeTypeAgent,
				Name: "Start Node",
			},
			"process1": {
				ID:           "process1",
				Type:         NodeTypeAgent,
				Name:         "Process 1",
				Dependencies: []string{"start"},
			},
			"process2": {
				ID:           "process2",
				Type:         NodeTypeAgent,
				Name:         "Process 2",
				Dependencies: []string{"start"},
			},
			"join": {
				ID:           "join",
				Type:         NodeTypeJoin,
				Name:         "Join Results",
				Dependencies: []string{"process1", "process2"},
			},
		},
		CreatedAt: time.Now(),
	}

	// Validate the workflow
	err := validator.Validate(workflow)
	if err != nil {
		fmt.Printf("Validation failed: %v\n", err)
		return
	}

	fmt.Printf("Workflow is valid!\n")
	fmt.Printf("Entry points: %v\n", workflow.EntryPoints)
	fmt.Printf("Exit points: %v\n", workflow.ExitPoints)

	// Get topological sort order
	sorted, err := validator.TopologicalSort(workflow)
	if err != nil {
		fmt.Printf("Topological sort failed: %v\n", err)
		return
	}
	// Sort middle elements for deterministic output (process1/process2 can be in either order)
	if len(sorted) == 4 {
		middle := sorted[1:3]
		slices.Sort(middle)
	}
	fmt.Printf("Execution order: %v\n", sorted)

	// Output:
	// Workflow is valid!
	// Entry points: [start]
	// Exit points: [join]
	// Execution order: [start process1 process2 join]
}

// Example demonstrates how the validator detects cycles.
func ExampleDAGValidator_DetectCycles() {
	validator := NewDAGValidator()

	// Create a workflow with a cycle
	workflow := &Workflow{
		ID:   types.NewID(),
		Name: "cyclic-workflow",
		Nodes: map[string]*WorkflowNode{
			"node1": {
				ID:           "node1",
				Type:         NodeTypeAgent,
				Dependencies: []string{"node3"},
			},
			"node2": {
				ID:           "node2",
				Type:         NodeTypeAgent,
				Dependencies: []string{"node1"},
			},
			"node3": {
				ID:           "node3",
				Type:         NodeTypeAgent,
				Dependencies: []string{"node2"},
			},
		},
		CreatedAt: time.Now(),
	}

	// Detect cycles
	cycle, err := validator.DetectCycles(workflow)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(cycle) > 0 {
		fmt.Printf("Cycle detected with %d nodes\n", len(cycle))
	} else {
		fmt.Printf("No cycle detected\n")
	}

	// Output:
	// Cycle detected with 4 nodes
}
