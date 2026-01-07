// Package planning provides bounded planning orchestration capabilities for the Gibson Framework.
// It enables intelligent pre-execution planning, mid-execution replanning, and memory-informed
// strategy selection while constraining all decisions within workflow-defined boundaries.
package planning

import (
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/workflow"
)

// PlanningBounds represents the constraint envelope extracted from a workflow definition.
// All planning decisions must operate within these bounds - the planner cannot invent
// arbitrary actions or exceed defined boundaries.
type PlanningBounds struct {
	// AllowedAgents contains the names of agents that can be selected for execution.
	// Extracted from agent nodes in the workflow.
	AllowedAgents []string

	// AllowedTools contains the names of tools that can be used.
	// Extracted from tool nodes in the workflow.
	AllowedTools []string

	// AllowedPlugins contains the names of plugins available for execution.
	// Extracted from plugin nodes in the workflow.
	AllowedPlugins []string

	// MaxParallel is the maximum number of nodes that can execute concurrently.
	// Calculated from parallel node sub-node counts.
	MaxParallel int

	// MaxDuration is the total time budget for the workflow.
	// Calculated from the sum of all node timeouts.
	MaxDuration time.Duration

	// MaxTokens is the total token budget for the workflow.
	// This should be set at the mission level and passed through.
	MaxTokens int

	// MaxCost is the total cost budget for the workflow in dollars.
	// This should be set at the mission level and passed through.
	MaxCost float64

	// ApprovalGates contains node IDs that require human approval before execution.
	// Extracted from node metadata with approval requirements.
	ApprovalGates []string

	// NodeConstraints maps node IDs to their specific execution constraints.
	NodeConstraints map[string]NodeBounds

	// AllowedNodes is the complete set of valid node IDs from the workflow.
	// Used for fast validation that a step exists in the workflow.
	AllowedNodes map[string]bool
}

// NodeBounds represents per-node execution constraints.
type NodeBounds struct {
	// MaxRetries is the maximum number of retry attempts for this node.
	// Extracted from the node's retry policy.
	MaxRetries int

	// Timeout is the maximum execution time for this node.
	// Extracted from the node's timeout setting.
	Timeout time.Duration

	// RequiredTools contains tools that must be available for this node.
	// Only applicable for tool nodes.
	RequiredTools []string

	// NodeType indicates the type of node (agent, tool, plugin, etc.)
	NodeType workflow.NodeType
}

// ExecutionStep represents a planned step in the execution strategy.
// This is used by planners to propose execution sequences.
type ExecutionStep struct {
	// NodeID is the workflow node to execute.
	NodeID string

	// Parameters contains optional execution parameters for this step.
	// Can be used to modify node behavior during execution.
	Parameters map[string]any
}

// ExtractBounds analyzes a workflow definition and extracts all planning constraints
// into a PlanningBounds structure. This creates the constraint envelope within which
// all planning decisions must operate.
//
// The function:
//   - Collects all unique agent, tool, and plugin names
//   - Calculates maximum parallelism from parallel nodes
//   - Sums node timeouts to estimate total duration
//   - Extracts per-node constraints (retries, timeouts)
//   - Identifies approval gates from node metadata
//   - Builds the set of valid node IDs for validation
//
// Returns an error if the workflow is nil or malformed.
func ExtractBounds(wf *workflow.Workflow) (*PlanningBounds, error) {
	if wf == nil {
		return nil, fmt.Errorf("workflow cannot be nil")
	}

	if len(wf.Nodes) == 0 {
		return nil, fmt.Errorf("workflow must contain at least one node")
	}

	bounds := &PlanningBounds{
		AllowedAgents:   []string{},
		AllowedTools:    []string{},
		AllowedPlugins:  []string{},
		MaxParallel:     1, // Default to sequential execution
		MaxDuration:     0,
		ApprovalGates:   []string{},
		NodeConstraints: make(map[string]NodeBounds),
		AllowedNodes:    make(map[string]bool),
	}

	// Use maps to track unique names
	agentSet := make(map[string]bool)
	toolSet := make(map[string]bool)
	pluginSet := make(map[string]bool)

	// Process each node in the workflow
	for nodeID, node := range wf.Nodes {
		if node == nil {
			continue
		}

		// Add node to allowed set
		bounds.AllowedNodes[nodeID] = true

		// Extract node-specific constraints
		nodeBounds := NodeBounds{
			NodeType: node.Type,
			Timeout:  node.Timeout,
		}

		// Extract retry policy if present
		if node.RetryPolicy != nil {
			nodeBounds.MaxRetries = node.RetryPolicy.MaxRetries
		}

		// Accumulate total duration from node timeouts
		if node.Timeout > 0 {
			bounds.MaxDuration += node.Timeout
		}

		// Process based on node type
		switch node.Type {
		case workflow.NodeTypeAgent:
			if node.AgentName != "" {
				agentSet[node.AgentName] = true
			}

		case workflow.NodeTypeTool:
			if node.ToolName != "" {
				toolSet[node.ToolName] = true
				nodeBounds.RequiredTools = []string{node.ToolName}
			}

		case workflow.NodeTypePlugin:
			if node.PluginName != "" {
				pluginSet[node.PluginName] = true
			}

		case workflow.NodeTypeParallel:
			// Calculate max parallelism from parallel node
			subNodeCount := len(node.SubNodes)
			if subNodeCount > bounds.MaxParallel {
				bounds.MaxParallel = subNodeCount
			}

			// Process sub-nodes recursively
			for _, subNode := range node.SubNodes {
				if subNode == nil {
					continue
				}

				// Add sub-node to allowed set
				bounds.AllowedNodes[subNode.ID] = true

				// Extract sub-node constraints
				subBounds := NodeBounds{
					NodeType: subNode.Type,
					Timeout:  subNode.Timeout,
				}

				if subNode.RetryPolicy != nil {
					subBounds.MaxRetries = subNode.RetryPolicy.MaxRetries
				}

				// Accumulate duration from sub-nodes
				if subNode.Timeout > 0 {
					bounds.MaxDuration += subNode.Timeout
				}

				// Track component types from sub-nodes
				switch subNode.Type {
				case workflow.NodeTypeAgent:
					if subNode.AgentName != "" {
						agentSet[subNode.AgentName] = true
					}
				case workflow.NodeTypeTool:
					if subNode.ToolName != "" {
						toolSet[subNode.ToolName] = true
						subBounds.RequiredTools = []string{subNode.ToolName}
					}
				case workflow.NodeTypePlugin:
					if subNode.PluginName != "" {
						pluginSet[subNode.PluginName] = true
					}
				}

				bounds.NodeConstraints[subNode.ID] = subBounds
			}
		}

		// Check for approval gate metadata
		if node.Metadata != nil {
			if requiresApproval, ok := node.Metadata["requires_approval"].(bool); ok && requiresApproval {
				bounds.ApprovalGates = append(bounds.ApprovalGates, nodeID)
			}
		}

		bounds.NodeConstraints[nodeID] = nodeBounds
	}

	// Convert sets to slices
	for agent := range agentSet {
		bounds.AllowedAgents = append(bounds.AllowedAgents, agent)
	}
	for tool := range toolSet {
		bounds.AllowedTools = append(bounds.AllowedTools, tool)
	}
	for plugin := range pluginSet {
		bounds.AllowedPlugins = append(bounds.AllowedPlugins, plugin)
	}

	return bounds, nil
}

// ValidateStep checks whether a proposed execution step is valid within the planning bounds.
// A step is valid if its NodeID exists in the workflow's allowed nodes.
//
// Returns an error if:
//   - The step references a node not in the workflow
//   - The node ID is empty
func (b *PlanningBounds) ValidateStep(step ExecutionStep) error {
	if step.NodeID == "" {
		return fmt.Errorf("step nodeID cannot be empty")
	}

	if !b.ContainsNode(step.NodeID) {
		return fmt.Errorf("step references node %q which is not in the workflow", step.NodeID)
	}

	return nil
}

// ContainsNode returns true if the given node ID exists in the workflow.
// This is a fast O(1) lookup using the internal node set.
func (b *PlanningBounds) ContainsNode(nodeID string) bool {
	return b.AllowedNodes[nodeID]
}

// GetNodeBounds returns the execution constraints for a specific node.
// Returns nil if the node does not exist in the bounds.
func (b *PlanningBounds) GetNodeBounds(nodeID string) *NodeBounds {
	if bounds, ok := b.NodeConstraints[nodeID]; ok {
		return &bounds
	}
	return nil
}

// RequiresApproval returns true if the given node requires human approval before execution.
func (b *PlanningBounds) RequiresApproval(nodeID string) bool {
	for _, gateID := range b.ApprovalGates {
		if gateID == nodeID {
			return true
		}
	}
	return false
}

// ValidateSteps validates a sequence of execution steps against the bounds.
// Returns an error on the first invalid step encountered.
func (b *PlanningBounds) ValidateSteps(steps []ExecutionStep) error {
	for i, step := range steps {
		if err := b.ValidateStep(step); err != nil {
			return fmt.Errorf("invalid step at position %d: %w", i, err)
		}
	}
	return nil
}
