package workflow

import (
	"fmt"
	"strings"
)

// DAGValidator provides validation functionality for workflow DAGs.
// It's a stateless validator that can check for cycles, validate dependencies,
// perform topological sorting, and compute entry/exit points.
type DAGValidator struct{}

// NewDAGValidator creates a new DAGValidator instance.
func NewDAGValidator() *DAGValidator {
	return &DAGValidator{}
}

// Validate runs all validation checks on a workflow and returns the first error encountered.
// It checks for:
// - Empty workflows
// - Cycles in the DAG
// - Missing dependencies
// - Computes entry and exit points
func (v *DAGValidator) Validate(w *Workflow) error {
	// Check for empty workflow
	if w == nil {
		return &WorkflowError{
			Code:    WorkflowErrorInvalidWorkflow,
			Message: "workflow cannot be nil",
		}
	}

	if len(w.Nodes) == 0 {
		return &WorkflowError{
			Code:    WorkflowErrorInvalidWorkflow,
			Message: "workflow must contain at least one node",
		}
	}

	// Validate dependencies exist
	if err := v.ValidateDependencies(w); err != nil {
		return err
	}

	// Check for cycles
	cycle, err := v.DetectCycles(w)
	if err != nil {
		return err
	}
	if len(cycle) > 0 {
		return &WorkflowError{
			Code:    WorkflowErrorCycleDetected,
			Message: fmt.Sprintf("cycle detected in workflow: %s", strings.Join(cycle, " -> ")),
		}
	}

	// Compute entry and exit points
	v.computeEntryExitPoints(w)

	return nil
}

// DetectCycles uses depth-first search (DFS) with color marking to detect cycles in the workflow DAG.
// Colors: white (0) = unvisited, gray (1) = in-progress, black (2) = done.
// Returns the nodes involved in a cycle if found, otherwise returns an empty slice.
func (v *DAGValidator) DetectCycles(w *Workflow) ([]string, error) {
	if w == nil || len(w.Nodes) == 0 {
		return nil, nil
	}

	// Color map: 0 = white (unvisited), 1 = gray (in-progress), 2 = black (done)
	color := make(map[string]int)
	parent := make(map[string]string)

	// Initialize all nodes as white
	for nodeID := range w.Nodes {
		color[nodeID] = 0
	}

	// Build adjacency list from both edges and dependencies
	adjList := v.buildAdjacencyList(w)

	// DFS function that returns the cycle path if found
	var dfs func(nodeID string) []string
	dfs = func(nodeID string) []string {
		color[nodeID] = 1 // Mark as in-progress (gray)

		// Check all neighbors
		for _, neighbor := range adjList[nodeID] {
			if color[neighbor] == 0 {
				// Unvisited node, continue DFS
				parent[neighbor] = nodeID
				if cycle := dfs(neighbor); cycle != nil {
					return cycle
				}
			} else if color[neighbor] == 1 {
				// Found a back edge (cycle detected)
				// Reconstruct the cycle path
				cycle := []string{neighbor}
				current := nodeID
				for current != neighbor {
					cycle = append([]string{current}, cycle...)
					current = parent[current]
				}
				cycle = append([]string{neighbor}, cycle...)
				return cycle
			}
			// If color[neighbor] == 2 (black), it's already processed, skip
		}

		color[nodeID] = 2 // Mark as done (black)
		return nil
	}

	// Run DFS from all unvisited nodes
	for nodeID := range w.Nodes {
		if color[nodeID] == 0 {
			if cycle := dfs(nodeID); cycle != nil {
				return cycle, nil
			}
		}
	}

	return []string{}, nil
}

// TopologicalSort performs a topological sort using Kahn's algorithm (BFS with in-degree tracking).
// Returns an ordered list of node IDs, or an error if a cycle is detected.
func (v *DAGValidator) TopologicalSort(w *Workflow) ([]string, error) {
	if w == nil || len(w.Nodes) == 0 {
		return []string{}, nil
	}

	// Build adjacency list and calculate in-degrees
	adjList := v.buildAdjacencyList(w)
	inDegree := make(map[string]int)

	// Initialize in-degrees
	for nodeID := range w.Nodes {
		inDegree[nodeID] = 0
	}

	// Count in-degrees
	for _, neighbors := range adjList {
		for _, neighbor := range neighbors {
			inDegree[neighbor]++
		}
	}

	// Queue for nodes with zero in-degree
	queue := []string{}
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	// Process nodes in topological order
	result := []string{}
	for len(queue) > 0 {
		// Dequeue
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		// Reduce in-degree of neighbors
		for _, neighbor := range adjList[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// If we haven't processed all nodes, there's a cycle
	if len(result) != len(w.Nodes) {
		return nil, &WorkflowError{
			Code:    WorkflowErrorCycleDetected,
			Message: "cannot perform topological sort: cycle detected in workflow",
		}
	}

	return result, nil
}

// ValidateDependencies checks that all dependencies referenced in nodes actually exist in the workflow.
func (v *DAGValidator) ValidateDependencies(w *Workflow) error {
	if w == nil || len(w.Nodes) == 0 {
		return nil
	}

	for nodeID, node := range w.Nodes {
		if node == nil {
			continue
		}

		// Check each dependency
		for _, depID := range node.Dependencies {
			if _, exists := w.Nodes[depID]; !exists {
				return &WorkflowError{
					Code:    WorkflowErrorMissingDependency,
					Message: fmt.Sprintf("node '%s' has dependency '%s' which does not exist in workflow", nodeID, depID),
					NodeID:  nodeID,
				}
			}
		}
	}

	// Also validate edges
	for _, edge := range w.Edges {
		if _, exists := w.Nodes[edge.From]; !exists {
			return &WorkflowError{
				Code:    WorkflowErrorMissingDependency,
				Message: fmt.Sprintf("edge references non-existent source node '%s'", edge.From),
			}
		}
		if _, exists := w.Nodes[edge.To]; !exists {
			return &WorkflowError{
				Code:    WorkflowErrorMissingDependency,
				Message: fmt.Sprintf("edge references non-existent destination node '%s'", edge.To),
			}
		}
	}

	return nil
}

// computeEntryExitPoints identifies nodes with no incoming edges (entry points)
// and nodes with no outgoing edges (exit points), and sets them on the workflow.
func (v *DAGValidator) computeEntryExitPoints(w *Workflow) {
	if w == nil || len(w.Nodes) == 0 {
		w.EntryPoints = []string{}
		w.ExitPoints = []string{}
		return
	}

	// Track which nodes have incoming and outgoing edges
	hasIncoming := make(map[string]bool)
	hasOutgoing := make(map[string]bool)

	// Initialize all nodes as having no edges
	for nodeID := range w.Nodes {
		hasIncoming[nodeID] = false
		hasOutgoing[nodeID] = false
	}

	// Process edges
	for _, edge := range w.Edges {
		hasOutgoing[edge.From] = true
		hasIncoming[edge.To] = true
	}

	// Process dependencies (dependencies create implicit edges)
	for nodeID, node := range w.Nodes {
		if node == nil {
			continue
		}
		if len(node.Dependencies) > 0 {
			hasIncoming[nodeID] = true
			for _, depID := range node.Dependencies {
				hasOutgoing[depID] = true
			}
		}
	}

	// Collect entry points (nodes with no incoming edges)
	entryPoints := []string{}
	for nodeID := range w.Nodes {
		if !hasIncoming[nodeID] {
			entryPoints = append(entryPoints, nodeID)
		}
	}

	// Collect exit points (nodes with no outgoing edges)
	exitPoints := []string{}
	for nodeID := range w.Nodes {
		if !hasOutgoing[nodeID] {
			exitPoints = append(exitPoints, nodeID)
		}
	}

	w.EntryPoints = entryPoints
	w.ExitPoints = exitPoints
}

// buildAdjacencyList constructs an adjacency list representation of the workflow DAG
// from both explicit edges and implicit dependencies.
func (v *DAGValidator) buildAdjacencyList(w *Workflow) map[string][]string {
	adjList := make(map[string][]string)

	// Initialize adjacency list for all nodes
	for nodeID := range w.Nodes {
		adjList[nodeID] = []string{}
	}

	// Add edges to adjacency list
	for _, edge := range w.Edges {
		// Edge goes from source to destination
		adjList[edge.From] = append(adjList[edge.From], edge.To)
	}

	// Add dependencies to adjacency list
	// If node A depends on node B, there's an edge from B to A
	for nodeID, node := range w.Nodes {
		if node == nil {
			continue
		}
		for _, depID := range node.Dependencies {
			// Dependency creates an edge from the dependency to the node
			adjList[depID] = append(adjList[depID], nodeID)
		}
	}

	return adjList
}
