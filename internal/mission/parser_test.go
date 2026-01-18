package mission

import (
	"testing"
)

// TestParseDefinitionFromBytes tests basic parsing of mission YAML
func TestParseDefinitionFromBytes(t *testing.T) {
	yamlData := `
name: Test Mission
version: 1.0.0
description: A test mission

nodes:
  - id: node1
    type: agent
    agent: test-agent
    task:
      target: example.com
`

	def, err := ParseDefinitionFromBytes([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseDefinitionFromBytes failed: %v", err)
	}

	if def.Name != "Test Mission" {
		t.Errorf("Expected name 'Test Mission', got '%s'", def.Name)
	}

	if def.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", def.Version)
	}

	if len(def.Nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(def.Nodes))
	}

	node, exists := def.Nodes["node1"]
	if !exists {
		t.Fatal("Node 'node1' not found")
	}

	if node.Type != NodeTypeAgent {
		t.Errorf("Expected node type 'agent', got '%s'", node.Type)
	}

	if node.AgentName != "test-agent" {
		t.Errorf("Expected agent name 'test-agent', got '%s'", node.AgentName)
	}
}

// TestParseDefinitionMapNodes tests parsing nodes as a map instead of array
func TestParseDefinitionMapNodes(t *testing.T) {
	yamlData := `
name: Test Mission Map Nodes
version: 1.0.0

nodes:
  recon:
    type: agent
    agent: recon-agent
    task:
      target: example.com
  scan:
    type: tool
    tool: port-scanner
    depends_on:
      - recon
    input:
      port_range: "1-1024"
`

	def, err := ParseDefinitionFromBytes([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseDefinitionFromBytes failed: %v", err)
	}

	if len(def.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(def.Nodes))
	}

	// Check recon node
	recon, exists := def.Nodes["recon"]
	if !exists {
		t.Fatal("Node 'recon' not found")
	}
	if recon.Type != NodeTypeAgent {
		t.Errorf("Expected recon type 'agent', got '%s'", recon.Type)
	}

	// Check scan node
	scan, exists := def.Nodes["scan"]
	if !exists {
		t.Fatal("Node 'scan' not found")
	}
	if scan.Type != NodeTypeTool {
		t.Errorf("Expected scan type 'tool', got '%s'", scan.Type)
	}
	if len(scan.Dependencies) != 1 || scan.Dependencies[0] != "recon" {
		t.Errorf("Expected scan to depend on 'recon', got %v", scan.Dependencies)
	}

	// Check edges were created
	if len(def.Edges) != 1 {
		t.Errorf("Expected 1 edge, got %d", len(def.Edges))
	}
	if def.Edges[0].From != "recon" || def.Edges[0].To != "scan" {
		t.Errorf("Expected edge from 'recon' to 'scan', got from '%s' to '%s'", def.Edges[0].From, def.Edges[0].To)
	}
}

// TestParseDefinitionWithDependencies tests parsing mission dependencies
func TestParseDefinitionWithDependencies(t *testing.T) {
	yamlData := `
name: Test Mission with Dependencies
version: 1.0.0

dependencies:
  agents:
    - recon-agent
    - scan-agent
  tools:
    - nmap
    - masscan

nodes:
  - id: node1
    type: agent
    agent: recon-agent
`

	def, err := ParseDefinitionFromBytes([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseDefinitionFromBytes failed: %v", err)
	}

	if def.Dependencies == nil {
		t.Fatal("Expected dependencies to be set")
	}

	if len(def.Dependencies.Agents) != 2 {
		t.Errorf("Expected 2 agent dependencies, got %d", len(def.Dependencies.Agents))
	}

	if len(def.Dependencies.Tools) != 2 {
		t.Errorf("Expected 2 tool dependencies, got %d", len(def.Dependencies.Tools))
	}
}

// TestParseDefinitionInvalidYAML tests error handling for invalid YAML
func TestParseDefinitionInvalidYAML(t *testing.T) {
	yamlData := `
name: Test Mission
nodes: {invalid yaml structure
`

	_, err := ParseDefinitionFromBytes([]byte(yamlData))
	if err == nil {
		t.Fatal("Expected error for invalid YAML, got nil")
	}

	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("Expected *ParseError, got %T", err)
	}

	if parseErr.Line == 0 {
		t.Error("Expected line number in parse error")
	}
}

// TestParseDefinitionMissingName tests error for missing required name field
func TestParseDefinitionMissingName(t *testing.T) {
	yamlData := `
version: 1.0.0

nodes:
  - id: node1
    type: agent
    agent: test-agent
`

	_, err := ParseDefinitionFromBytes([]byte(yamlData))
	if err == nil {
		t.Fatal("Expected error for missing name, got nil")
	}

	parseErr, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("Expected *ParseError, got %T", err)
	}

	if parseErr.Message != "mission 'name' field is required" {
		t.Errorf("Expected name required error, got: %s", parseErr.Message)
	}
}

// TestParseDefinitionMissingNodes tests error for missing nodes
func TestParseDefinitionMissingNodes(t *testing.T) {
	yamlData := `
name: Test Mission
version: 1.0.0
`

	_, err := ParseDefinitionFromBytes([]byte(yamlData))
	if err == nil {
		t.Fatal("Expected error for missing nodes, got nil")
	}
}

// TestParseDefinitionWithRetryPolicy tests parsing retry configuration
func TestParseDefinitionWithRetryPolicy(t *testing.T) {
	yamlData := `
name: Test Mission with Retry
version: 1.0.0

nodes:
  - id: node1
    type: agent
    agent: test-agent
    timeout: 5m
    retry:
      max_retries: 3
      backoff: exponential
      initial_delay: 1s
      max_delay: 30s
      multiplier: 2.0
`

	def, err := ParseDefinitionFromBytes([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseDefinitionFromBytes failed: %v", err)
	}

	node := def.Nodes["node1"]
	if node.RetryPolicy == nil {
		t.Fatal("Expected retry policy to be set")
	}

	if node.RetryPolicy.MaxRetries != 3 {
		t.Errorf("Expected max_retries 3, got %d", node.RetryPolicy.MaxRetries)
	}

	if node.RetryPolicy.BackoffStrategy != BackoffExponential {
		t.Errorf("Expected exponential backoff, got %s", node.RetryPolicy.BackoffStrategy)
	}

	if node.RetryPolicy.Multiplier != 2.0 {
		t.Errorf("Expected multiplier 2.0, got %f", node.RetryPolicy.Multiplier)
	}
}

// TestParseDefinitionTargetReference tests parsing target as string reference
func TestParseDefinitionTargetReference(t *testing.T) {
	yamlData := `
name: Test Mission with Target
version: 1.0.0
target: my-target-name

nodes:
  - id: node1
    type: agent
    agent: test-agent
`

	def, err := ParseDefinitionFromBytes([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseDefinitionFromBytes failed: %v", err)
	}

	if def.TargetRef != "my-target-name" {
		t.Errorf("Expected target ref 'my-target-name', got '%s'", def.TargetRef)
	}
}
