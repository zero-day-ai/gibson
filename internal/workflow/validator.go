package workflow

// This file implements comprehensive workflow validation with two validator types:
//
// 1. DAGValidator - Fast, fail-fast validator that returns on first error.
//    Used during workflow execution for quick validation checks.
//
// 2. Validator - Comprehensive validator that collects ALL validation errors.
//    Used during workflow creation/editing to provide complete error feedback.
//
// The Validator provides extensive validation including:
//   - DAG acyclicity (cycle detection using DFS)
//   - Node uniqueness (enforced by map structure)
//   - Reference validation (depends_on, edges, condition branches)
//   - Required field validation (per node type)
//   - Timeout format validation
//   - Retry policy validation
//   - Optional registry validation (agent/tool/plugin existence)
//
// Example usage:
//
//	// Create a validator without registry (structure-only validation)
//	validator := workflow.NewValidator(nil)
//
//	// Validate a workflow and collect all errors
//	errors := validator.ValidateWorkflow(workflow)
//	if len(errors) > 0 {
//	    fmt.Printf("Found %d validation errors:\n", len(errors))
//	    for _, err := range errors {
//	        fmt.Printf("  - %s\n", err.Error())
//	    }
//	}
//
//	// With registry validation
//	validator := workflow.NewValidator(myRegistry)
//	errors := validator.ValidateWorkflow(workflow)

import (
	"fmt"
	"strings"
	"time"
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

// ValidationError represents a single validation error with detailed context.
type ValidationError struct {
	// NodeID is the ID of the node where the error occurred (if applicable)
	NodeID string `json:"node_id,omitempty"`

	// Field is the name of the field that failed validation (if applicable)
	Field string `json:"field,omitempty"`

	// Message is a human-readable error message
	Message string `json:"message"`

	// Line is the line number in the source YAML (if available from parser)
	Line int `json:"line,omitempty"`

	// ErrorCode is a machine-readable error code
	ErrorCode string `json:"error_code"`
}

// Error implements the error interface for ValidationError
func (e *ValidationError) Error() string {
	parts := []string{}

	if e.NodeID != "" {
		parts = append(parts, fmt.Sprintf("node=%s", e.NodeID))
	}
	if e.Field != "" {
		parts = append(parts, fmt.Sprintf("field=%s", e.Field))
	}
	if e.Line > 0 {
		parts = append(parts, fmt.Sprintf("line=%d", e.Line))
	}

	context := ""
	if len(parts) > 0 {
		context = fmt.Sprintf(" [%s]", strings.Join(parts, ", "))
	}

	return fmt.Sprintf("%s%s: %s", e.ErrorCode, context, e.Message)
}

// ValidationErrors is a collection of validation errors
type ValidationErrors []ValidationError

// Error implements the error interface for ValidationErrors
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	if len(e) == 1 {
		return e[0].Error()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d validation errors:\n", len(e)))
	for i, err := range e {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, err.Error()))
	}
	return sb.String()
}

// Registry defines the interface for checking if agents, tools, and plugins exist.
// This allows the validator to verify that referenced components are available.
type Registry interface {
	// HasAgent returns true if an agent with the given name exists
	HasAgent(name string) bool

	// HasTool returns true if a tool with the given name exists
	HasTool(name string) bool

	// HasPlugin returns true if a plugin with the given name exists
	HasPlugin(name string) bool
}

// Validator provides comprehensive workflow validation with detailed error reporting.
// Unlike DAGValidator which returns on first error, this validator collects all errors.
type Validator struct {
	registry Registry
}

// NewValidator creates a new Validator instance.
// The registry parameter is optional (can be nil) for validating structure only.
func NewValidator(registry Registry) *Validator {
	return &Validator{
		registry: registry,
	}
}

// ValidateWorkflow performs comprehensive validation of a workflow and returns all errors found.
// It validates:
// - DAG is acyclic (cycle detection)
// - All node IDs are unique (already enforced by map structure)
// - All depends_on references point to existing nodes
// - Required fields are present based on node type
// - Timeout format is valid (if specified)
// - Agent/tool/plugin names exist in registry (if registry provided)
func (v *Validator) ValidateWorkflow(w *Workflow) ValidationErrors {
	errors := ValidationErrors{}

	// Basic workflow validation
	if w == nil {
		errors = append(errors, ValidationError{
			ErrorCode: "workflow_nil",
			Message:   "workflow cannot be nil",
		})
		return errors
	}

	if w.Name == "" {
		errors = append(errors, ValidationError{
			ErrorCode: "workflow_name_required",
			Field:     "name",
			Message:   "workflow name is required",
		})
	}

	if len(w.Nodes) == 0 {
		errors = append(errors, ValidationError{
			ErrorCode: "workflow_empty",
			Message:   "workflow must contain at least one node",
		})
		return errors
	}

	// Validate node structure and required fields
	nodeErrors := v.validateNodes(w)
	errors = append(errors, nodeErrors...)

	// Validate dependencies reference existing nodes
	depErrors := v.validateNodeReferences(w)
	errors = append(errors, depErrors...)

	// Validate DAG is acyclic (cycle detection)
	cycleErrors := v.validateDAGAcyclic(w)
	errors = append(errors, cycleErrors...)

	// Validate against registry if provided
	if v.registry != nil {
		regErrors := v.validateAgentToolRegistry(w)
		errors = append(errors, regErrors...)
	}

	return errors
}

// validateNodes validates all nodes in the workflow for required fields and proper structure
func (v *Validator) validateNodes(w *Workflow) ValidationErrors {
	errors := ValidationErrors{}

	for nodeID, node := range w.Nodes {
		if node == nil {
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				ErrorCode: "node_nil",
				Message:   "node is nil",
			})
			continue
		}

		// Validate required fields common to all nodes
		if node.ID == "" {
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				Field:     "id",
				ErrorCode: "node_id_required",
				Message:   "node ID is required",
			})
		} else if node.ID != nodeID {
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				Field:     "id",
				ErrorCode: "node_id_mismatch",
				Message:   fmt.Sprintf("node ID '%s' does not match map key '%s'", node.ID, nodeID),
			})
		}

		if node.Type == "" {
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				Field:     "type",
				ErrorCode: "node_type_required",
				Message:   "node type is required",
			})
			continue // Can't validate type-specific fields without knowing the type
		}

		if node.Name == "" {
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				Field:     "name",
				ErrorCode: "node_name_required",
				Message:   "node name is required",
			})
		}

		// Validate timeout format (should already be parsed correctly, but check for zero/negative)
		if node.Timeout < 0 {
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				Field:     "timeout",
				ErrorCode: "timeout_invalid",
				Message:   "timeout cannot be negative",
			})
		}

		// Validate retry policy if present
		if node.RetryPolicy != nil {
			retryErrors := v.validateRetryPolicy(nodeID, node.RetryPolicy)
			errors = append(errors, retryErrors...)
		}

		// Validate type-specific required fields
		switch node.Type {
		case NodeTypeAgent:
			if node.AgentName == "" {
				errors = append(errors, ValidationError{
					NodeID:    nodeID,
					Field:     "agent_name",
					ErrorCode: "agent_name_required",
					Message:   "agent name is required for agent nodes",
				})
			}

		case NodeTypeTool:
			if node.ToolName == "" {
				errors = append(errors, ValidationError{
					NodeID:    nodeID,
					Field:     "tool_name",
					ErrorCode: "tool_name_required",
					Message:   "tool name is required for tool nodes",
				})
			}

		case NodeTypePlugin:
			if node.PluginName == "" {
				errors = append(errors, ValidationError{
					NodeID:    nodeID,
					Field:     "plugin_name",
					ErrorCode: "plugin_name_required",
					Message:   "plugin name is required for plugin nodes",
				})
			}
			if node.PluginMethod == "" {
				errors = append(errors, ValidationError{
					NodeID:    nodeID,
					Field:     "plugin_method",
					ErrorCode: "plugin_method_required",
					Message:   "plugin method is required for plugin nodes",
				})
			}

		case NodeTypeCondition:
			if node.Condition == nil {
				errors = append(errors, ValidationError{
					NodeID:    nodeID,
					Field:     "condition",
					ErrorCode: "condition_required",
					Message:   "condition is required for condition nodes",
				})
			} else {
				if node.Condition.Expression == "" {
					errors = append(errors, ValidationError{
						NodeID:    nodeID,
						Field:     "condition.expression",
						ErrorCode: "condition_expression_required",
						Message:   "condition expression is required",
					})
				}
				// Validate branch references
				for _, branchNodeID := range node.Condition.TrueBranch {
					if _, exists := w.Nodes[branchNodeID]; !exists {
						errors = append(errors, ValidationError{
							NodeID:    nodeID,
							Field:     "condition.true_branch",
							ErrorCode: "invalid_branch_reference",
							Message:   fmt.Sprintf("true branch references non-existent node '%s'", branchNodeID),
						})
					}
				}
				for _, branchNodeID := range node.Condition.FalseBranch {
					if _, exists := w.Nodes[branchNodeID]; !exists {
						errors = append(errors, ValidationError{
							NodeID:    nodeID,
							Field:     "condition.false_branch",
							ErrorCode: "invalid_branch_reference",
							Message:   fmt.Sprintf("false branch references non-existent node '%s'", branchNodeID),
						})
					}
				}
			}

		case NodeTypeParallel:
			if len(node.SubNodes) == 0 {
				errors = append(errors, ValidationError{
					NodeID:    nodeID,
					Field:     "sub_nodes",
					ErrorCode: "sub_nodes_required",
					Message:   "parallel nodes must contain at least one sub-node",
				})
			} else {
				// Recursively validate sub-nodes
				for i, subNode := range node.SubNodes {
					if subNode == nil {
						errors = append(errors, ValidationError{
							NodeID:    nodeID,
							Field:     fmt.Sprintf("sub_nodes[%d]", i),
							ErrorCode: "sub_node_nil",
							Message:   "sub-node is nil",
						})
						continue
					}
					// Validate sub-node structure (basic validation, no recursion into nested parallel nodes)
					if subNode.ID == "" {
						errors = append(errors, ValidationError{
							NodeID:    nodeID,
							Field:     fmt.Sprintf("sub_nodes[%d].id", i),
							ErrorCode: "sub_node_id_required",
							Message:   "sub-node ID is required",
						})
					}
					if subNode.Type == "" {
						errors = append(errors, ValidationError{
							NodeID:    nodeID,
							Field:     fmt.Sprintf("sub_nodes[%d].type", i),
							ErrorCode: "sub_node_type_required",
							Message:   "sub-node type is required",
						})
					}
				}
			}

		case NodeTypeJoin:
			// Join nodes don't have additional required fields

		default:
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				Field:     "type",
				ErrorCode: "node_type_invalid",
				Message:   fmt.Sprintf("invalid node type: %s", node.Type),
			})
		}
	}

	return errors
}

// validateRetryPolicy validates retry policy configuration
func (v *Validator) validateRetryPolicy(nodeID string, policy *RetryPolicy) ValidationErrors {
	errors := ValidationErrors{}

	if policy.MaxRetries < 0 {
		errors = append(errors, ValidationError{
			NodeID:    nodeID,
			Field:     "retry_policy.max_retries",
			ErrorCode: "max_retries_invalid",
			Message:   "max_retries must be non-negative",
		})
	}

	// Validate backoff strategy
	validStrategies := map[BackoffStrategy]bool{
		BackoffConstant:    true,
		BackoffLinear:      true,
		BackoffExponential: true,
	}
	if !validStrategies[policy.BackoffStrategy] {
		errors = append(errors, ValidationError{
			NodeID:    nodeID,
			Field:     "retry_policy.backoff_strategy",
			ErrorCode: "backoff_strategy_invalid",
			Message:   fmt.Sprintf("invalid backoff strategy: %s (must be constant, linear, or exponential)", policy.BackoffStrategy),
		})
	}

	// Validate delays
	if policy.InitialDelay < 0 {
		errors = append(errors, ValidationError{
			NodeID:    nodeID,
			Field:     "retry_policy.initial_delay",
			ErrorCode: "initial_delay_invalid",
			Message:   "initial_delay cannot be negative",
		})
	}

	if policy.MaxDelay < 0 {
		errors = append(errors, ValidationError{
			NodeID:    nodeID,
			Field:     "retry_policy.max_delay",
			ErrorCode: "max_delay_invalid",
			Message:   "max_delay cannot be negative",
		})
	}

	if policy.MaxDelay > 0 && policy.InitialDelay > 0 && policy.MaxDelay < policy.InitialDelay {
		errors = append(errors, ValidationError{
			NodeID:    nodeID,
			Field:     "retry_policy.max_delay",
			ErrorCode: "max_delay_invalid",
			Message:   "max_delay must be greater than or equal to initial_delay",
		})
	}

	// Exponential backoff specific validation
	if policy.BackoffStrategy == BackoffExponential {
		if policy.Multiplier <= 0 {
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				Field:     "retry_policy.multiplier",
				ErrorCode: "multiplier_invalid",
				Message:   "multiplier must be positive for exponential backoff",
			})
		}
		if policy.MaxDelay == 0 {
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				Field:     "retry_policy.max_delay",
				ErrorCode: "max_delay_required",
				Message:   "max_delay is required for exponential backoff",
			})
		}
	}

	return errors
}

// validateNodeReferences validates that all node references (dependencies and edges) point to existing nodes
func (v *Validator) validateNodeReferences(w *Workflow) ValidationErrors {
	errors := ValidationErrors{}

	// Validate node dependencies
	for nodeID, node := range w.Nodes {
		if node == nil {
			continue
		}

		for _, depID := range node.Dependencies {
			if _, exists := w.Nodes[depID]; !exists {
				errors = append(errors, ValidationError{
					NodeID:    nodeID,
					Field:     "dependencies",
					ErrorCode: "missing_dependency",
					Message:   fmt.Sprintf("dependency '%s' does not exist in workflow", depID),
				})
			}
		}
	}

	// Validate edges
	for i, edge := range w.Edges {
		if _, exists := w.Nodes[edge.From]; !exists {
			errors = append(errors, ValidationError{
				Field:     fmt.Sprintf("edges[%d].from", i),
				ErrorCode: "missing_edge_source",
				Message:   fmt.Sprintf("edge references non-existent source node '%s'", edge.From),
			})
		}
		if _, exists := w.Nodes[edge.To]; !exists {
			errors = append(errors, ValidationError{
				Field:     fmt.Sprintf("edges[%d].to", i),
				ErrorCode: "missing_edge_destination",
				Message:   fmt.Sprintf("edge references non-existent destination node '%s'", edge.To),
			})
		}
	}

	return errors
}

// validateDAGAcyclic uses DFS to detect cycles in the workflow DAG
func (v *Validator) validateDAGAcyclic(w *Workflow) ValidationErrors {
	errors := ValidationErrors{}

	// Use the existing DAGValidator for cycle detection
	dagValidator := NewDAGValidator()
	cycle, err := dagValidator.DetectCycles(w)
	if err != nil {
		errors = append(errors, ValidationError{
			ErrorCode: "cycle_detection_failed",
			Message:   fmt.Sprintf("failed to detect cycles: %v", err),
		})
		return errors
	}

	if len(cycle) > 0 {
		errors = append(errors, ValidationError{
			ErrorCode: "cycle_detected",
			Message:   fmt.Sprintf("cycle detected in workflow: %s", strings.Join(cycle, " -> ")),
		})
	}

	return errors
}

// validateAgentToolRegistry validates that all referenced agents, tools, and plugins exist in the registry
func (v *Validator) validateAgentToolRegistry(w *Workflow) ValidationErrors {
	errors := ValidationErrors{}

	if v.registry == nil {
		return errors
	}

	for nodeID, node := range w.Nodes {
		if node == nil {
			continue
		}

		switch node.Type {
		case NodeTypeAgent:
			if node.AgentName != "" && !v.registry.HasAgent(node.AgentName) {
				errors = append(errors, ValidationError{
					NodeID:    nodeID,
					Field:     "agent_name",
					ErrorCode: "agent_not_found",
					Message:   fmt.Sprintf("agent '%s' not found in registry", node.AgentName),
				})
			}

		case NodeTypeTool:
			if node.ToolName != "" && !v.registry.HasTool(node.ToolName) {
				errors = append(errors, ValidationError{
					NodeID:    nodeID,
					Field:     "tool_name",
					ErrorCode: "tool_not_found",
					Message:   fmt.Sprintf("tool '%s' not found in registry", node.ToolName),
				})
			}

		case NodeTypePlugin:
			if node.PluginName != "" && !v.registry.HasPlugin(node.PluginName) {
				errors = append(errors, ValidationError{
					NodeID:    nodeID,
					Field:     "plugin_name",
					ErrorCode: "plugin_not_found",
					Message:   fmt.Sprintf("plugin '%s' not found in registry", node.PluginName),
				})
			}
		}
	}

	return errors
}

// ParseDuration is a helper function to validate duration strings.
// It wraps time.ParseDuration to provide consistent error handling.
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
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
