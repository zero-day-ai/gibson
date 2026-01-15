package workflow_test

import (
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// ExampleValidator_basic demonstrates basic workflow validation without a registry
func ExampleValidator_basic() {
	// Create a validator without registry (structure-only validation)
	validator := workflow.NewValidator(nil)

	// Create a simple workflow
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "example-workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"node1": {
				ID:        "node1",
				Type:      workflow.NodeTypeAgent,
				Name:      "Scanner",
				AgentName: "nmap-scanner",
				Timeout:   30 * time.Second,
			},
			"node2": {
				ID:           "node2",
				Type:         workflow.NodeTypeAgent,
				Name:         "Analyzer",
				AgentName:    "vulnerability-analyzer",
				Dependencies: []string{"node1"},
			},
		},
	}

	// Validate the workflow
	errors := validator.ValidateWorkflow(wf)

	if len(errors) == 0 {
		fmt.Println("Workflow is valid")
	} else {
		fmt.Printf("Found %d validation errors:\n", len(errors))
		for _, err := range errors {
			fmt.Printf("  - %s\n", err.Error())
		}
	}

	// Output:
	// Workflow is valid
}

// ExampleValidator_invalidWorkflow demonstrates validation with errors
func ExampleValidator_invalidWorkflow() {
	validator := workflow.NewValidator(nil)

	// Create an invalid workflow with multiple errors
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "", // Error: missing name
		Nodes: map[string]*workflow.WorkflowNode{
			"node1": {
				ID:           "node1",
				Type:         workflow.NodeTypeAgent,
				Name:         "Test",
				AgentName:    "",                      // Error: missing agent name
				Timeout:      -5 * time.Second,        // Error: negative timeout
				Dependencies: []string{"nonexistent"}, // Error: missing dependency
			},
		},
	}

	errors := validator.ValidateWorkflow(wf)

	fmt.Printf("Found %d validation errors\n", len(errors))

	// Output:
	// Found 4 validation errors
}

// ExampleValidator_cycle demonstrates cycle detection
func ExampleValidator_cycle() {
	validator := workflow.NewValidator(nil)

	// Create a workflow with a cycle
	wf := &workflow.Workflow{
		ID:   types.NewID(),
		Name: "cyclic-workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"node1": {
				ID:           "node1",
				Type:         workflow.NodeTypeAgent,
				Name:         "Node 1",
				AgentName:    "agent",
				Dependencies: []string{"node2"},
			},
			"node2": {
				ID:           "node2",
				Type:         workflow.NodeTypeAgent,
				Name:         "Node 2",
				AgentName:    "agent",
				Dependencies: []string{"node1"}, // Creates a cycle
			},
		},
	}

	errors := validator.ValidateWorkflow(wf)

	for _, err := range errors {
		if err.ErrorCode == "cycle_detected" {
			fmt.Println("Cycle detected in workflow")
			break
		}
	}

	// Output:
	// Cycle detected in workflow
}

// ExampleValidationErrors demonstrates error formatting
func ExampleValidationErrors() {
	errors := workflow.ValidationErrors{
		{
			NodeID:    "node1",
			Field:     "timeout",
			ErrorCode: "timeout_invalid",
			Message:   "timeout cannot be negative",
		},
		{
			NodeID:    "node2",
			Field:     "agent_name",
			ErrorCode: "agent_name_required",
			Message:   "agent name is required for agent nodes",
		},
	}

	fmt.Println(errors.Error())

	// Output:
	// 2 validation errors:
	//   1. timeout_invalid [node=node1, field=timeout]: timeout cannot be negative
	//   2. agent_name_required [node=node2, field=agent_name]: agent name is required for agent nodes
}

// ExampleParseDuration demonstrates duration validation
func ExampleParseDuration() {
	// Valid durations
	validDurations := []string{"5s", "10m", "2h", "500ms"}
	for _, d := range validDurations {
		duration, err := workflow.ParseDuration(d)
		if err == nil {
			fmt.Printf("%s = %v\n", d, duration)
		}
	}

	// Invalid duration
	_, err := workflow.ParseDuration("invalid")
	if err != nil {
		fmt.Println("Error: invalid duration format")
	}

	// Output:
	// 5s = 5s
	// 10m = 10m0s
	// 2h = 2h0m0s
	// 500ms = 500ms
	// Error: invalid duration format
}
