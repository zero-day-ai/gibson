package mission

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidationError represents a single validation error with detailed context.
// It provides structured error information including field paths and error codes.
type ValidationError struct {
	// NodeID is the ID of the node where the error occurred (if applicable)
	NodeID string `json:"node_id,omitempty"`

	// Field is the name of the field that failed validation (if applicable)
	Field string `json:"field,omitempty"`

	// Message is a human-readable error message
	Message string `json:"message"`

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

// Validate performs comprehensive validation of a MissionDefinition and returns all errors found.
// It validates:
// - Required fields (name, at least one node)
// - Version format (semver if provided)
// - Node structure and type-specific required fields
// - Node references in edges and dependencies point to existing nodes
// - DAG is acyclic (no circular dependencies)
// - Node types are valid
func Validate(def *MissionDefinition) ValidationErrors {
	errors := ValidationErrors{}

	// Basic definition validation
	if def == nil {
		errors = append(errors, ValidationError{
			ErrorCode: "definition_nil",
			Message:   "mission definition cannot be nil",
		})
		return errors
	}

	// Validate required field: name
	if def.Name == "" {
		errors = append(errors, ValidationError{
			ErrorCode: "name_required",
			Field:     "name",
			Message:   "mission name is required",
		})
	}

	// Validate at least one node exists
	if len(def.Nodes) == 0 {
		errors = append(errors, ValidationError{
			ErrorCode: "empty_definition",
			Message:   "mission definition must contain at least one node",
		})
		return errors // Can't validate further without nodes
	}

	// Validate version format if provided (basic semver check)
	if def.Version != "" {
		versionErrors := validateVersionFormat(def.Version)
		errors = append(errors, versionErrors...)
	}

	// Validate node structure and required fields
	nodeErrors := validateNodes(def)
	errors = append(errors, nodeErrors...)

	// Validate node references in edges and dependencies
	refErrors := validateNodeReferences(def)
	errors = append(errors, refErrors...)

	// Validate DAG is acyclic (cycle detection)
	cycleErrors := validateDAGAcyclic(def)
	errors = append(errors, cycleErrors...)

	return errors
}

// validateVersionFormat validates that the version string follows semantic versioning format
func validateVersionFormat(version string) ValidationErrors {
	errors := ValidationErrors{}

	// Basic semver pattern: MAJOR.MINOR.PATCH with optional pre-release and build metadata
	// Examples: 1.0.0, 1.2.3-alpha, 1.2.3+build.123, 1.2.3-rc.1+build.456
	semverPattern := regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

	if !semverPattern.MatchString(version) {
		errors = append(errors, ValidationError{
			ErrorCode: "version_format_invalid",
			Field:     "version",
			Message:   fmt.Sprintf("version '%s' does not follow semantic versioning format (expected: MAJOR.MINOR.PATCH)", version),
		})
	}

	return errors
}

// validateNodes validates all nodes in the mission definition for required fields and proper structure
func validateNodes(def *MissionDefinition) ValidationErrors {
	errors := ValidationErrors{}

	for nodeID, node := range def.Nodes {
		if node == nil {
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				ErrorCode: "node_nil",
				Message:   "node is nil",
			})
			continue
		}

		// Validate node ID matches map key
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

		// Validate node type is present and valid
		if node.Type == "" {
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				Field:     "type",
				ErrorCode: "node_type_required",
				Message:   "node type is required",
			})
			continue // Can't validate type-specific fields without knowing the type
		}

		// Validate node type is valid
		if !isValidNodeType(node.Type) {
			errors = append(errors, ValidationError{
				NodeID:    nodeID,
				Field:     "type",
				ErrorCode: "node_type_invalid",
				Message:   fmt.Sprintf("invalid node type: %s (must be agent, tool, plugin, condition, parallel, or join)", node.Type),
			})
			continue // Can't validate type-specific fields with invalid type
		}

		// Validate timeout is non-negative
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
			retryErrors := validateRetryPolicy(nodeID, node.RetryPolicy)
			errors = append(errors, retryErrors...)
		}

		// Validate type-specific required fields
		typeErrors := validateNodeTypeFields(nodeID, node, def)
		errors = append(errors, typeErrors...)
	}

	return errors
}

// isValidNodeType checks if the given node type is valid
func isValidNodeType(nodeType NodeType) bool {
	switch nodeType {
	case NodeTypeAgent, NodeTypeTool, NodeTypePlugin, NodeTypeCondition, NodeTypeParallel, NodeTypeJoin:
		return true
	default:
		return false
	}
}

// validateNodeTypeFields validates type-specific required fields for a node
func validateNodeTypeFields(nodeID string, node *MissionNode, def *MissionDefinition) ValidationErrors {
	errors := ValidationErrors{}

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
			// Validate branch references point to existing nodes
			for _, branchNodeID := range node.Condition.TrueBranch {
				if _, exists := def.Nodes[branchNodeID]; !exists {
					errors = append(errors, ValidationError{
						NodeID:    nodeID,
						Field:     "condition.true_branch",
						ErrorCode: "invalid_branch_reference",
						Message:   fmt.Sprintf("true branch references non-existent node '%s'", branchNodeID),
					})
				}
			}
			for _, branchNodeID := range node.Condition.FalseBranch {
				if _, exists := def.Nodes[branchNodeID]; !exists {
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
			// Validate sub-nodes structure
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
				// Basic validation for sub-nodes
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
				} else if !isValidNodeType(subNode.Type) {
					errors = append(errors, ValidationError{
						NodeID:    nodeID,
						Field:     fmt.Sprintf("sub_nodes[%d].type", i),
						ErrorCode: "sub_node_type_invalid",
						Message:   fmt.Sprintf("invalid sub-node type: %s", subNode.Type),
					})
				}
			}
		}

	case NodeTypeJoin:
		// Join nodes don't have additional required fields
	}

	return errors
}

// validateRetryPolicy validates retry policy configuration
func validateRetryPolicy(nodeID string, policy *RetryPolicy) ValidationErrors {
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
	if policy.BackoffStrategy != "" && !validStrategies[policy.BackoffStrategy] {
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
func validateNodeReferences(def *MissionDefinition) ValidationErrors {
	errors := ValidationErrors{}

	// Validate node dependencies
	for nodeID, node := range def.Nodes {
		if node == nil {
			continue
		}

		for _, depID := range node.Dependencies {
			if _, exists := def.Nodes[depID]; !exists {
				errors = append(errors, ValidationError{
					NodeID:    nodeID,
					Field:     "dependencies",
					ErrorCode: "missing_dependency",
					Message:   fmt.Sprintf("dependency '%s' does not exist in mission", depID),
				})
			}
		}
	}

	// Validate edges
	for i, edge := range def.Edges {
		if _, exists := def.Nodes[edge.From]; !exists {
			errors = append(errors, ValidationError{
				Field:     fmt.Sprintf("edges[%d].from", i),
				ErrorCode: "missing_edge_source",
				Message:   fmt.Sprintf("edge references non-existent source node '%s'", edge.From),
			})
		}
		if _, exists := def.Nodes[edge.To]; !exists {
			errors = append(errors, ValidationError{
				Field:     fmt.Sprintf("edges[%d].to", i),
				ErrorCode: "missing_edge_destination",
				Message:   fmt.Sprintf("edge references non-existent destination node '%s'", edge.To),
			})
		}
	}

	return errors
}

// validateDAGAcyclic uses DFS to detect cycles in the mission DAG
func validateDAGAcyclic(def *MissionDefinition) ValidationErrors {
	errors := ValidationErrors{}

	// Build adjacency list from edges and dependencies
	adjList := buildAdjacencyList(def)

	// Use DFS with color marking to detect cycles
	// Colors: 0 = white (unvisited), 1 = gray (in-progress), 2 = black (done)
	color := make(map[string]int)
	parent := make(map[string]string)

	// Initialize all nodes as white
	for nodeID := range def.Nodes {
		color[nodeID] = 0
	}

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
				for current != neighbor && current != "" {
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
	for nodeID := range def.Nodes {
		if color[nodeID] == 0 {
			if cycle := dfs(nodeID); cycle != nil {
				errors = append(errors, ValidationError{
					ErrorCode: "cycle_detected",
					Message:   fmt.Sprintf("circular dependency detected in mission: %s", strings.Join(cycle, " -> ")),
				})
				return errors // Return after first cycle found
			}
		}
	}

	return errors
}

// buildAdjacencyList constructs an adjacency list representation of the mission DAG
// from both explicit edges and implicit dependencies
func buildAdjacencyList(def *MissionDefinition) map[string][]string {
	adjList := make(map[string][]string)

	// Initialize adjacency list for all nodes
	for nodeID := range def.Nodes {
		adjList[nodeID] = []string{}
	}

	// Add edges to adjacency list
	for _, edge := range def.Edges {
		// Edge goes from source to destination
		adjList[edge.From] = append(adjList[edge.From], edge.To)
	}

	// Add dependencies to adjacency list
	// If node A depends on node B, there's an edge from B to A
	for nodeID, node := range def.Nodes {
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
