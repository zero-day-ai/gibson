package workflow_test

import (
	"fmt"
	"log"

	"github.com/zero-day-ai/gibson/internal/workflow"
)

// ExampleParseWorkflowYAMLFromBytes demonstrates basic workflow parsing from YAML bytes.
func ExampleParseWorkflowYAMLFromBytes() {
	yamlData := []byte(`
name: Example Workflow
description: A simple example workflow
version: "1.0"

nodes:
  - id: recon
    type: agent
    name: Reconnaissance Agent
    agent: recon-agent
    timeout: 5m
    retry:
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      max_delay: 30s
      multiplier: 2.0
    task:
      goal: "Perform reconnaissance"

  - id: analysis
    type: tool
    name: Analysis Tool
    tool: analyze-data
    depends_on:
      - recon
    input:
      depth: deep
`)

	parsed, err := workflow.ParseWorkflowYAMLFromBytes(yamlData)
	if err != nil {
		log.Fatalf("Failed to parse workflow: %v", err)
	}

	fmt.Printf("Workflow: %s\n", parsed.Name)
	fmt.Printf("Nodes: %d\n", len(parsed.Nodes))
	fmt.Printf("Entry points: %v\n", parsed.EntryPoints)
	fmt.Printf("Exit points: %v\n", parsed.ExitPoints)

	// Output:
	// Workflow: Example Workflow
	// Nodes: 2
	// Entry points: [recon]
	// Exit points: [analysis]
}

// ExampleParseWorkflowYAML demonstrates parsing a workflow from a file.
func ExampleParseWorkflowYAML() {
	// In a real application, you would read from an actual file:
	// parsed, err := workflow.ParseWorkflowYAML("/path/to/workflow.yaml")

	yamlData := []byte(`
name: File-based Workflow
nodes:
  - id: start
    type: agent
    agent: starter
  - id: process
    type: tool
    tool: processor
    depends_on:
      - start
`)

	parsed, err := workflow.ParseWorkflowYAMLFromBytes(yamlData)
	if err != nil {
		log.Fatalf("Parse error: %v", err)
	}

	fmt.Printf("Parsed workflow: %s\n", parsed.Name)
	fmt.Printf("Has %d edges\n", len(parsed.Edges))

	// Output:
	// Parsed workflow: File-based Workflow
	// Has 1 edges
}

// ExampleParseError demonstrates error handling with line numbers.
func ExampleParseError() {
	// This workflow has an invalid node type
	yamlData := []byte(`
name: Invalid Workflow
nodes:
  - id: bad_node
    type: invalid_type
`)

	_, err := workflow.ParseWorkflowYAMLFromBytes(yamlData)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		// In production, you could extract line numbers for better error reporting
	}

	// Output:
	// Error: parse error at line 4:0 (node bad_node): invalid node type 'invalid_type': must be one of: agent, tool, plugin, condition, parallel, join
}

// ExampleParsedWorkflow_ToWorkflow demonstrates converting a parsed workflow to an executable workflow.
func ExampleParsedWorkflow_ToWorkflow() {
	yamlData := []byte(`
name: Convert Example
description: Demonstrating conversion
nodes:
  - id: node1
    type: agent
    agent: test-agent
`)

	parsed, err := workflow.ParseWorkflowYAMLFromBytes(yamlData)
	if err != nil {
		log.Fatalf("Parse failed: %v", err)
	}

	// Convert to executable workflow
	wf := parsed.ToWorkflow()

	fmt.Printf("Workflow ID: %s\n", wf.ID[:8]) // Just show first 8 chars
	fmt.Printf("Name: %s\n", wf.Name)
	fmt.Printf("Ready for execution: %v\n", wf.Nodes != nil)

	// Output will vary due to random ID generation
	// Workflow ID: <random>
	// Name: Convert Example
	// Ready for execution: true
}

// ExampleParseWorkflowYAMLFromBytes_complexDAG demonstrates parsing a complex workflow with multiple paths.
func ExampleParseWorkflowYAMLFromBytes_complexDAG() {
	yamlData := []byte(`
name: Complex Workflow
description: Demonstrates parallel execution and joins

nodes:
  - id: start
    type: agent
    name: Initialization
    agent: init-agent

  - id: scan_ports
    type: agent
    name: Port Scanner
    agent: port-scanner
    depends_on:
      - start
    timeout: 10m

  - id: scan_services
    type: agent
    name: Service Scanner
    agent: service-scanner
    depends_on:
      - start
    timeout: 10m

  - id: analyze
    type: join
    name: Analysis Join
    depends_on:
      - scan_ports
      - scan_services

  - id: report
    type: tool
    name: Generate Report
    tool: report-generator
    depends_on:
      - analyze
    input:
      format: json
`)

	parsed, err := workflow.ParseWorkflowYAMLFromBytes(yamlData)
	if err != nil {
		log.Fatalf("Parse failed: %v", err)
	}

	fmt.Printf("Workflow: %s\n", parsed.Name)
	fmt.Printf("Total nodes: %d\n", len(parsed.Nodes))
	fmt.Printf("Total edges: %d\n", len(parsed.Edges))
	fmt.Printf("Entry nodes: %v\n", parsed.EntryPoints)
	fmt.Printf("Exit nodes: %v\n", parsed.ExitPoints)

	// Verify parallel structure
	startNode := parsed.Nodes["start"]
	fmt.Printf("Start node type: %s\n", startNode.Type)

	// Output:
	// Workflow: Complex Workflow
	// Total nodes: 5
	// Total edges: 5
	// Entry nodes: [start]
	// Exit nodes: [report]
	// Start node type: agent
}

// ExampleParseWorkflowYAMLFromBytes_retryPolicy demonstrates parsing retry configurations.
func ExampleParseWorkflowYAMLFromBytes_retryPolicy() {
	yamlData := []byte(`
name: Retry Example
nodes:
  - id: resilient_node
    type: agent
    agent: unstable-service
    retry:
      max_retries: 5
      backoff: exponential
      initial_delay: 2s
      max_delay: 60s
      multiplier: 2.0
`)

	parsed, err := workflow.ParseWorkflowYAMLFromBytes(yamlData)
	if err != nil {
		log.Fatalf("Parse failed: %v", err)
	}

	node := parsed.Nodes["resilient_node"]
	policy := node.RetryPolicy

	fmt.Printf("Max retries: %d\n", policy.MaxRetries)
	fmt.Printf("Backoff strategy: %s\n", policy.BackoffStrategy)
	fmt.Printf("Initial delay: %v\n", policy.InitialDelay)
	fmt.Printf("Max delay: %v\n", policy.MaxDelay)
	fmt.Printf("Multiplier: %.1f\n", policy.Multiplier)

	// Calculate delay for 3rd retry
	delay := policy.CalculateDelay(3)
	fmt.Printf("Delay for 3rd retry: %v\n", delay)

	// Output:
	// Max retries: 5
	// Backoff strategy: exponential
	// Initial delay: 2s
	// Max delay: 1m0s
	// Multiplier: 2.0
	// Delay for 3rd retry: 16s
}
