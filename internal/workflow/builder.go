package workflow

import (
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// WorkflowBuilder provides a fluent API for constructing workflows.
// It accumulates errors during building and reports them all at Build() time.
type WorkflowBuilder struct {
	workflow *Workflow
	errors   []error
}

// NewWorkflow creates a new WorkflowBuilder with an initialized workflow.
func NewWorkflow(name string) *WorkflowBuilder {
	return &WorkflowBuilder{
		workflow: &Workflow{
			ID:          types.NewID(),
			Name:        name,
			Nodes:       make(map[string]*WorkflowNode),
			Edges:       []WorkflowEdge{},
			EntryPoints: []string{},
			ExitPoints:  []string{},
			Metadata:    make(map[string]any),
			CreatedAt:   time.Now(),
		},
		errors: []error{},
	}
}

// WithDescription sets the description for the workflow.
func (wb *WorkflowBuilder) WithDescription(desc string) *WorkflowBuilder {
	wb.workflow.Description = desc
	return wb
}

// AddNode adds a node to the workflow's Nodes map.
// If a node with the same ID already exists, an error is accumulated.
func (wb *WorkflowBuilder) AddNode(node *WorkflowNode) *WorkflowBuilder {
	if node == nil {
		wb.errors = append(wb.errors, fmt.Errorf("cannot add nil node"))
		return wb
	}
	if node.ID == "" {
		wb.errors = append(wb.errors, fmt.Errorf("node must have an ID"))
		return wb
	}
	if _, exists := wb.workflow.Nodes[node.ID]; exists {
		wb.errors = append(wb.errors, fmt.Errorf("node with ID %q already exists", node.ID))
		return wb
	}
	wb.workflow.Nodes[node.ID] = node
	return wb
}

// AddAgentNode is a helper that creates and adds an agent node to the workflow.
func (wb *WorkflowBuilder) AddAgentNode(id, agentName string, task *agent.Task) *WorkflowBuilder {
	if id == "" {
		wb.errors = append(wb.errors, fmt.Errorf("agent node must have an ID"))
		return wb
	}
	if agentName == "" {
		wb.errors = append(wb.errors, fmt.Errorf("agent node %q must have an agent name", id))
		return wb
	}

	node := &WorkflowNode{
		ID:        id,
		Type:      NodeTypeAgent,
		Name:      agentName,
		AgentName: agentName,
		AgentTask: task,
		Metadata:  make(map[string]any),
	}

	return wb.AddNode(node)
}

// AddToolNode is a helper that creates and adds a tool node to the workflow.
func (wb *WorkflowBuilder) AddToolNode(id, toolName string, input map[string]any) *WorkflowBuilder {
	if id == "" {
		wb.errors = append(wb.errors, fmt.Errorf("tool node must have an ID"))
		return wb
	}
	if toolName == "" {
		wb.errors = append(wb.errors, fmt.Errorf("tool node %q must have a tool name", id))
		return wb
	}

	node := &WorkflowNode{
		ID:        id,
		Type:      NodeTypeTool,
		Name:      toolName,
		ToolName:  toolName,
		ToolInput: input,
		Metadata:  make(map[string]any),
	}

	return wb.AddNode(node)
}

// AddPluginNode is a helper that creates and adds a plugin node to the workflow.
func (wb *WorkflowBuilder) AddPluginNode(id, pluginName, method string, params map[string]any) *WorkflowBuilder {
	if id == "" {
		wb.errors = append(wb.errors, fmt.Errorf("plugin node must have an ID"))
		return wb
	}
	if pluginName == "" {
		wb.errors = append(wb.errors, fmt.Errorf("plugin node %q must have a plugin name", id))
		return wb
	}
	if method == "" {
		wb.errors = append(wb.errors, fmt.Errorf("plugin node %q must have a method", id))
		return wb
	}

	node := &WorkflowNode{
		ID:           id,
		Type:         NodeTypePlugin,
		Name:         fmt.Sprintf("%s.%s", pluginName, method),
		PluginName:   pluginName,
		PluginMethod: method,
		PluginParams: params,
		Metadata:     make(map[string]any),
	}

	return wb.AddNode(node)
}

// AddConditionNode is a helper that creates and adds a condition node to the workflow.
func (wb *WorkflowBuilder) AddConditionNode(id string, condition *NodeCondition) *WorkflowBuilder {
	if id == "" {
		wb.errors = append(wb.errors, fmt.Errorf("condition node must have an ID"))
		return wb
	}
	if condition == nil {
		wb.errors = append(wb.errors, fmt.Errorf("condition node %q must have a condition", id))
		return wb
	}
	if condition.Expression == "" {
		wb.errors = append(wb.errors, fmt.Errorf("condition node %q must have a non-empty expression", id))
		return wb
	}

	node := &WorkflowNode{
		ID:        id,
		Type:      NodeTypeCondition,
		Name:      fmt.Sprintf("condition:%s", id),
		Condition: condition,
		Metadata:  make(map[string]any),
	}

	return wb.AddNode(node)
}

// AddEdge adds a directed edge from one node to another.
func (wb *WorkflowBuilder) AddEdge(from, to string) *WorkflowBuilder {
	if from == "" {
		wb.errors = append(wb.errors, fmt.Errorf("edge must have a non-empty 'from' node"))
		return wb
	}
	if to == "" {
		wb.errors = append(wb.errors, fmt.Errorf("edge must have a non-empty 'to' node"))
		return wb
	}

	edge := WorkflowEdge{
		From: from,
		To:   to,
	}

	wb.workflow.Edges = append(wb.workflow.Edges, edge)
	return wb
}

// AddConditionalEdge adds a directed edge with a condition from one node to another.
func (wb *WorkflowBuilder) AddConditionalEdge(from, to, condition string) *WorkflowBuilder {
	if from == "" {
		wb.errors = append(wb.errors, fmt.Errorf("conditional edge must have a non-empty 'from' node"))
		return wb
	}
	if to == "" {
		wb.errors = append(wb.errors, fmt.Errorf("conditional edge must have a non-empty 'to' node"))
		return wb
	}
	if condition == "" {
		wb.errors = append(wb.errors, fmt.Errorf("conditional edge must have a non-empty condition"))
		return wb
	}

	edge := WorkflowEdge{
		From:      from,
		To:        to,
		Condition: condition,
	}

	wb.workflow.Edges = append(wb.workflow.Edges, edge)
	return wb
}

// WithDependency sets dependencies on an existing node.
// The node must already exist in the workflow.
func (wb *WorkflowBuilder) WithDependency(nodeID string, dependsOn ...string) *WorkflowBuilder {
	node, exists := wb.workflow.Nodes[nodeID]
	if !exists {
		wb.errors = append(wb.errors, fmt.Errorf("cannot set dependencies on non-existent node %q", nodeID))
		return wb
	}

	if len(dependsOn) == 0 {
		wb.errors = append(wb.errors, fmt.Errorf("must specify at least one dependency for node %q", nodeID))
		return wb
	}

	// Initialize dependencies if nil
	if node.Dependencies == nil {
		node.Dependencies = []string{}
	}

	// Append new dependencies
	node.Dependencies = append(node.Dependencies, dependsOn...)

	return wb
}

// Build validates the workflow DAG, computes entry and exit points,
// and returns the constructed workflow or accumulated errors.
func (wb *WorkflowBuilder) Build() (*Workflow, error) {
	// Validate that we have at least one node
	if len(wb.workflow.Nodes) == 0 {
		wb.errors = append(wb.errors, fmt.Errorf("workflow must have at least one node"))
	}

	// Validate that all edges reference existing nodes
	for _, edge := range wb.workflow.Edges {
		if _, exists := wb.workflow.Nodes[edge.From]; !exists {
			wb.errors = append(wb.errors, fmt.Errorf("edge references non-existent 'from' node %q", edge.From))
		}
		if _, exists := wb.workflow.Nodes[edge.To]; !exists {
			wb.errors = append(wb.errors, fmt.Errorf("edge references non-existent 'to' node %q", edge.To))
		}
	}

	// Validate that all dependencies reference existing nodes
	for nodeID, node := range wb.workflow.Nodes {
		for _, depID := range node.Dependencies {
			if _, exists := wb.workflow.Nodes[depID]; !exists {
				wb.errors = append(wb.errors, fmt.Errorf("node %q depends on non-existent node %q", nodeID, depID))
			}
		}
	}

	// Check for cycles in the DAG
	if err := wb.validateDAG(); err != nil {
		wb.errors = append(wb.errors, err)
	}

	// Compute entry points (nodes with no incoming edges)
	wb.computeEntryPoints()

	// Compute exit points (nodes with no outgoing edges)
	wb.computeExitPoints()

	// If there were any errors during building, return them
	if len(wb.errors) > 0 {
		return nil, fmt.Errorf("workflow validation failed with %d error(s): %v", len(wb.errors), wb.errors)
	}

	return wb.workflow, nil
}

// validateDAG checks if the workflow is a valid DAG (no cycles).
// Uses depth-first search with three colors: white (unvisited), gray (visiting), black (visited).
func (wb *WorkflowBuilder) validateDAG() error {
	// Build adjacency list from edges
	adjList := make(map[string][]string)
	for _, edge := range wb.workflow.Edges {
		adjList[edge.From] = append(adjList[edge.From], edge.To)
	}

	// Color map: 0=white (unvisited), 1=gray (visiting), 2=black (visited)
	color := make(map[string]int)
	for nodeID := range wb.workflow.Nodes {
		color[nodeID] = 0
	}

	// DFS function to detect cycles
	var dfs func(nodeID string, path []string) error
	dfs = func(nodeID string, path []string) error {
		color[nodeID] = 1 // Mark as visiting (gray)
		path = append(path, nodeID)

		for _, neighbor := range adjList[nodeID] {
			if color[neighbor] == 1 {
				// Found a back edge - cycle detected
				cyclePath := append(path, neighbor)
				return fmt.Errorf("cycle detected in workflow: %v", cyclePath)
			}
			if color[neighbor] == 0 {
				if err := dfs(neighbor, path); err != nil {
					return err
				}
			}
		}

		color[nodeID] = 2 // Mark as visited (black)
		return nil
	}

	// Run DFS from all unvisited nodes
	for nodeID := range wb.workflow.Nodes {
		if color[nodeID] == 0 {
			if err := dfs(nodeID, []string{}); err != nil {
				return err
			}
		}
	}

	return nil
}

// computeEntryPoints identifies nodes with no incoming edges.
func (wb *WorkflowBuilder) computeEntryPoints() {
	// Track which nodes have incoming edges
	hasIncoming := make(map[string]bool)
	for _, edge := range wb.workflow.Edges {
		hasIncoming[edge.To] = true
	}

	// Entry points are nodes without incoming edges
	entryPoints := []string{}
	for nodeID := range wb.workflow.Nodes {
		if !hasIncoming[nodeID] {
			entryPoints = append(entryPoints, nodeID)
		}
	}

	wb.workflow.EntryPoints = entryPoints
}

// computeExitPoints identifies nodes with no outgoing edges.
func (wb *WorkflowBuilder) computeExitPoints() {
	// Track which nodes have outgoing edges
	hasOutgoing := make(map[string]bool)
	for _, edge := range wb.workflow.Edges {
		hasOutgoing[edge.From] = true
	}

	// Exit points are nodes without outgoing edges
	exitPoints := []string{}
	for nodeID := range wb.workflow.Nodes {
		if !hasOutgoing[nodeID] {
			exitPoints = append(exitPoints, nodeID)
		}
	}

	wb.workflow.ExitPoints = exitPoints
}
