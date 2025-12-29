package workflow

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/agent"
)

// ExampleWorkflowBuilder demonstrates building a simple workflow
func ExampleWorkflowBuilder() {
	// Create a simple sequential workflow
	workflow, err := NewWorkflow("security-scan").
		WithDescription("Automated security scanning workflow").
		AddAgentNode("scan", "vulnerability-scanner", &agent.Task{
			Name:        "scan-target",
			Description: "Scan target for vulnerabilities",
		}).
		AddToolNode("parse", "report-parser", map[string]any{
			"format": "json",
		}).
		AddPluginNode("report", "reporting", "generate", map[string]any{
			"template": "security-report.html",
		}).
		AddEdge("scan", "parse").
		AddEdge("parse", "report").
		Build()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Workflow: %s\n", workflow.Name)
	fmt.Printf("Nodes: %d\n", len(workflow.Nodes))
	fmt.Printf("Edges: %d\n", len(workflow.Edges))
	fmt.Printf("Entry: %v\n", workflow.EntryPoints)
	fmt.Printf("Exit: %v\n", workflow.ExitPoints)

	// Output:
	// Workflow: security-scan
	// Nodes: 3
	// Edges: 2
	// Entry: [scan]
	// Exit: [report]
}

// ExampleWorkflowBuilder_parallelExecution demonstrates building a workflow with parallel branches
func ExampleWorkflowBuilder_parallelExecution() {
	// Create a workflow with parallel execution branches
	workflow, err := NewWorkflow("parallel-scan").
		WithDescription("Run multiple scans in parallel").
		AddAgentNode("start", "coordinator", nil).
		AddAgentNode("scan-web", "web-scanner", nil).
		AddAgentNode("scan-api", "api-scanner", nil).
		AddAgentNode("scan-db", "db-scanner", nil).
		AddAgentNode("aggregate", "result-aggregator", nil).
		AddEdge("start", "scan-web").
		AddEdge("start", "scan-api").
		AddEdge("start", "scan-db").
		AddEdge("scan-web", "aggregate").
		AddEdge("scan-api", "aggregate").
		AddEdge("scan-db", "aggregate").
		Build()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Workflow: %s\n", workflow.Name)
	fmt.Printf("Entry points: %d\n", len(workflow.EntryPoints))
	fmt.Printf("Exit points: %d\n", len(workflow.ExitPoints))
	fmt.Printf("Total nodes: %d\n", len(workflow.Nodes))

	// Output:
	// Workflow: parallel-scan
	// Entry points: 1
	// Exit points: 1
	// Total nodes: 5
}

// ExampleWorkflowBuilder_conditionalBranching demonstrates building a workflow with conditional branching
func ExampleWorkflowBuilder_conditionalBranching() {
	// Create a workflow with conditional execution
	workflow, err := NewWorkflow("conditional-workflow").
		WithDescription("Workflow with conditional branching").
		AddAgentNode("scan", "scanner", nil).
		AddConditionNode("check-severity", &NodeCondition{
			Expression:  "result.severity >= 7",
			TrueBranch:  []string{"alert"},
			FalseBranch: []string{"log"},
		}).
		AddAgentNode("alert", "alerting-agent", nil).
		AddAgentNode("log", "logging-agent", nil).
		AddEdge("scan", "check-severity").
		AddConditionalEdge("check-severity", "alert", "severity >= 7").
		AddConditionalEdge("check-severity", "log", "severity < 7").
		Build()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Workflow: %s\n", workflow.Name)
	fmt.Printf("Nodes: %d\n", len(workflow.Nodes))
	fmt.Printf("Conditional edges: %d\n", countConditionalEdges(workflow))

	// Output:
	// Workflow: conditional-workflow
	// Nodes: 4
	// Conditional edges: 2
}

// ExampleWorkflowBuilder_dependencies demonstrates setting node dependencies
func ExampleWorkflowBuilder_dependencies() {
	// Create a workflow with explicit dependencies
	workflow, err := NewWorkflow("dependency-workflow").
		WithDescription("Workflow with explicit dependencies").
		AddAgentNode("setup", "setup-agent", nil).
		AddAgentNode("task1", "worker-1", nil).
		AddAgentNode("task2", "worker-2", nil).
		AddAgentNode("cleanup", "cleanup-agent", nil).
		WithDependency("task1", "setup").
		WithDependency("task2", "setup").
		WithDependency("cleanup", "task1", "task2").
		Build()

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	cleanupNode := workflow.Nodes["cleanup"]
	fmt.Printf("Cleanup node dependencies: %d\n", len(cleanupNode.Dependencies))

	// Output:
	// Cleanup node dependencies: 2
}

// ExampleWorkflowBuilder_errorHandling demonstrates error accumulation
func ExampleWorkflowBuilder_errorHandling() {
	// Create an invalid workflow to demonstrate error handling
	_, err := NewWorkflow("invalid-workflow").
		AddAgentNode("", "agent1", nil).         // Error: empty ID
		AddToolNode("tool1", "", nil).           // Error: empty tool name
		AddEdge("node1", "nonexistent").         // Error: references non-existent node
		Build()

	if err != nil {
		fmt.Println("Validation failed with multiple errors")
	}

	// Output:
	// Validation failed with multiple errors
}

// Helper function for example
func countConditionalEdges(w *Workflow) int {
	count := 0
	for _, edge := range w.Edges {
		if edge.Condition != "" {
			count++
		}
	}
	return count
}
