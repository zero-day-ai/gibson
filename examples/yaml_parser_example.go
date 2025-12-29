//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/zero-day-ai/gibson/internal/workflow"
)

func main() {
	// Check if a workflow file was provided
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run yaml_parser_example.go <workflow.yaml>")
		fmt.Println("Example: go run yaml_parser_example.go workflow-example.yaml")
		os.Exit(1)
	}

	workflowPath := os.Args[1]

	// Parse the workflow from YAML file
	fmt.Printf("Parsing workflow from: %s\n", workflowPath)
	wf, err := workflow.ParseWorkflowFile(workflowPath)
	if err != nil {
		log.Fatalf("Failed to parse workflow: %v", err)
	}

	// Display workflow information
	fmt.Printf("\n=== Workflow Information ===\n")
	fmt.Printf("Name: %s\n", wf.Name)
	fmt.Printf("Description: %s\n", wf.Description)
	fmt.Printf("Total Nodes: %d\n", len(wf.Nodes))
	fmt.Printf("Total Edges: %d\n", len(wf.Edges))
	fmt.Printf("Entry Points: %v\n", wf.EntryPoints)
	fmt.Printf("Exit Points: %v\n", wf.ExitPoints)

	// Display node details
	fmt.Printf("\n=== Workflow Nodes ===\n")
	for id, node := range wf.Nodes {
		fmt.Printf("\nNode: %s\n", id)
		fmt.Printf("  Type: %s\n", node.Type)
		fmt.Printf("  Name: %s\n", node.Name)
		if node.Description != "" {
			fmt.Printf("  Description: %s\n", node.Description)
		}

		// Display node-specific information
		switch node.Type {
		case workflow.NodeTypeAgent:
			fmt.Printf("  Agent: %s\n", node.AgentName)
			if node.AgentTask != nil && len(node.AgentTask.Input) > 0 {
				fmt.Printf("  Task Input: %v\n", node.AgentTask.Input)
			}

		case workflow.NodeTypeTool:
			fmt.Printf("  Tool: %s\n", node.ToolName)
			if len(node.ToolInput) > 0 {
				fmt.Printf("  Tool Input: %v\n", node.ToolInput)
			}

		case workflow.NodeTypePlugin:
			fmt.Printf("  Plugin: %s\n", node.PluginName)
			fmt.Printf("  Method: %s\n", node.PluginMethod)
			if len(node.PluginParams) > 0 {
				fmt.Printf("  Params: %v\n", node.PluginParams)
			}

		case workflow.NodeTypeCondition:
			if node.Condition != nil {
				fmt.Printf("  Expression: %s\n", node.Condition.Expression)
				fmt.Printf("  True Branch: %v\n", node.Condition.TrueBranch)
				fmt.Printf("  False Branch: %v\n", node.Condition.FalseBranch)
			}

		case workflow.NodeTypeParallel:
			fmt.Printf("  Sub-Nodes: %d\n", len(node.SubNodes))
			for i, subNode := range node.SubNodes {
				fmt.Printf("    [%d] %s (%s)\n", i+1, subNode.Name, subNode.Type)
			}
		}

		// Display dependencies
		if len(node.Dependencies) > 0 {
			fmt.Printf("  Dependencies: %v\n", node.Dependencies)
		}

		// Display timeout
		if node.Timeout > 0 {
			fmt.Printf("  Timeout: %s\n", node.Timeout)
		}

		// Display retry policy
		if node.RetryPolicy != nil {
			fmt.Printf("  Retry Policy:\n")
			fmt.Printf("    Max Retries: %d\n", node.RetryPolicy.MaxRetries)
			fmt.Printf("    Strategy: %s\n", node.RetryPolicy.BackoffStrategy)
			fmt.Printf("    Initial Delay: %s\n", node.RetryPolicy.InitialDelay)
			if node.RetryPolicy.MaxDelay > 0 {
				fmt.Printf("    Max Delay: %s\n", node.RetryPolicy.MaxDelay)
			}
			if node.RetryPolicy.Multiplier > 0 {
				fmt.Printf("    Multiplier: %.1f\n", node.RetryPolicy.Multiplier)
			}
		}
	}

	// Display workflow edges (dependencies)
	fmt.Printf("\n=== Workflow Edges ===\n")
	for _, edge := range wf.Edges {
		fmt.Printf("%s -> %s\n", edge.From, edge.To)
	}

	// Display execution order recommendation
	fmt.Printf("\n=== Suggested Execution Order ===\n")
	fmt.Printf("Entry nodes (start here): %v\n", wf.EntryPoints)
	fmt.Printf("Exit nodes (workflow completes): %v\n", wf.ExitPoints)

	fmt.Printf("\nWorkflow parsed successfully!\n")
}
