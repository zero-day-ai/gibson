// Package workflow provides YAML-based workflow definition and parsing capabilities
// for the Gibson workflow engine.
//
// This file implements YAML parsing functionality that allows workflow definitions
// to be written in human-readable YAML format and converted into executable
// Workflow structures.
//
// # Supported Node Types
//
// - agent: Execute tasks using registered agents
// - tool: Call workflow tools
// - plugin: Invoke plugin methods
// - condition: Conditional branching based on expressions
// - parallel: Execute multiple sub-nodes concurrently
// - join: Synchronization point for parallel execution
//
// # YAML Structure Example
//
//	name: My Workflow
//	description: Example workflow definition
//	nodes:
//	  - id: node1
//	    type: agent
//	    name: First Node
//	    agent: my-agent
//	    timeout: 30s
//	    retry:
//	      max_retries: 3
//	      backoff: exponential
//	      initial_delay: 1s
//	      max_delay: 30s
//	      multiplier: 2.0
//	    task:
//	      action: scan
//	      target: example.com
//
//	  - id: node2
//	    type: tool
//	    name: Second Node
//	    tool: my-tool
//	    depends_on:
//	      - node1
//	    input:
//	      param: value
//
// # Duration Format
//
// Timeout and retry delay values support Go duration format:
// - "300ms" (300 milliseconds)
// - "1s" (1 second)
// - "5m" (5 minutes)
// - "2h" (2 hours)
//
// # Retry Backoff Strategies
//
// - constant: Fixed delay between retries
// - linear: Linearly increasing delay (initial_delay * attempt)
// - exponential: Exponentially increasing delay (initial_delay * multiplier^attempt)
package workflow

import (
	"fmt"
	"os"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
	"gopkg.in/yaml.v3"
)

// YAMLWorkflow represents the top-level structure of a workflow YAML file.
// This structure maps directly to the YAML format for workflow definitions.
type YAMLWorkflow struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Target      *YAMLTarget `yaml:"target,omitempty"`
	Nodes       []YAMLNode  `yaml:"nodes"`
}

// YAMLTarget represents a target specification in YAML format.
// It supports both string references and inline object definitions:
//   - String form: target: my-target-name
//   - Object form: target: { type: network, connection: {...} }
type YAMLTarget struct {
	// Reference is set when target is specified as a plain string (name or ID)
	Reference string `yaml:"-"`

	// Inline definition fields (set when target is an object)
	Name       string         `yaml:"name,omitempty"`
	Type       string         `yaml:"type,omitempty"`
	Connection map[string]any `yaml:"connection,omitempty"`
	Provider   string         `yaml:"provider,omitempty"`
	Tags       []string       `yaml:"tags,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshaling to handle both string and object forms
func (t *YAMLTarget) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try unmarshaling as a string first
	var str string
	if err := unmarshal(&str); err == nil {
		t.Reference = str
		return nil
	}

	// If not a string, try unmarshaling as an object
	type rawTarget YAMLTarget
	var raw rawTarget
	if err := unmarshal(&raw); err != nil {
		return fmt.Errorf("target must be either a string reference or an object definition: %w", err)
	}

	// Check for ambiguous specification (both reference and inline fields)
	if raw.Reference != "" && (raw.Type != "" || raw.Connection != nil) {
		return fmt.Errorf("target must be either reference or inline, not both")
	}

	*t = YAMLTarget(raw)
	return nil
}

// IsReference returns true if this target is a reference to an existing target
func (t *YAMLTarget) IsReference() bool {
	return t.Reference != ""
}

// IsInline returns true if this target is an inline definition
func (t *YAMLTarget) IsInline() bool {
	return t.Type != "" || t.Connection != nil
}

// YAMLNode represents a node definition in YAML format.
// It supports all node types: agent, tool, plugin, condition, parallel, and join.
type YAMLNode struct {
	// Common fields for all node types
	ID          string     `yaml:"id"`
	Type        string     `yaml:"type"`
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	DependsOn   []string   `yaml:"depends_on,omitempty"`
	Timeout     string     `yaml:"timeout,omitempty"`
	Retry       *YAMLRetry `yaml:"retry,omitempty"`

	// Agent node fields
	Agent   string         `yaml:"agent,omitempty"`
	Goal    string         `yaml:"goal,omitempty"`    // Agent's goal/objective (top-level, not inside task)
	Context map[string]any `yaml:"context,omitempty"` // Agent's context (top-level, not inside task)
	Task    map[string]any `yaml:"task,omitempty"`

	// Tool node fields
	Tool  string         `yaml:"tool,omitempty"`
	Input map[string]any `yaml:"input,omitempty"`

	// Plugin node fields
	Plugin string         `yaml:"plugin,omitempty"`
	Method string         `yaml:"method,omitempty"`
	Params map[string]any `yaml:"params,omitempty"`

	// Condition node fields
	Condition *YAMLCondition `yaml:"condition,omitempty"`

	// Parallel node fields
	SubNodes []YAMLNode `yaml:"sub_nodes,omitempty"`

	// Additional metadata
	Metadata map[string]any `yaml:"metadata,omitempty"`
}

// YAMLCondition represents conditional branching logic in YAML format.
type YAMLCondition struct {
	Expression  string   `yaml:"expression"`
	TrueBranch  []string `yaml:"true_branch,omitempty"`
	FalseBranch []string `yaml:"false_branch,omitempty"`
}

// YAMLRetry represents retry policy configuration in YAML format.
type YAMLRetry struct {
	MaxRetries   int     `yaml:"max_retries"`
	Backoff      string  `yaml:"backoff"`
	InitialDelay string  `yaml:"initial_delay"`
	MaxDelay     string  `yaml:"max_delay,omitempty"`
	Multiplier   float64 `yaml:"multiplier,omitempty"`
}

// ParseWorkflow parses a YAML workflow definition from raw bytes and converts it
// to a Workflow struct. It validates the YAML structure and converts all fields
// to their appropriate types.
//
// Returns an error if:
//   - YAML is malformed or invalid
//   - Required fields are missing
//   - Type conversions fail (e.g., invalid duration format)
//   - Node types are invalid
//   - Dependencies reference non-existent nodes
func ParseWorkflow(data []byte) (*Workflow, error) {
	var yamlWf YAMLWorkflow
	if err := yaml.Unmarshal(data, &yamlWf); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate required fields
	if yamlWf.Name == "" {
		return nil, fmt.Errorf("workflow name is required")
	}

	if len(yamlWf.Nodes) == 0 {
		return nil, fmt.Errorf("workflow must contain at least one node")
	}

	// Create workflow structure
	wf := &Workflow{
		ID:          types.NewID(),
		Name:        yamlWf.Name,
		Description: yamlWf.Description,
		Nodes:       make(map[string]*WorkflowNode),
		Edges:       []WorkflowEdge{},
		EntryPoints: []string{},
		ExitPoints:  []string{},
		Metadata:    make(map[string]any),
		CreatedAt:   time.Now(),
	}

	// Copy target reference if present
	if yamlWf.Target != nil {
		if yamlWf.Target.IsReference() {
			wf.TargetRef = yamlWf.Target.Reference
		} else if yamlWf.Target.Name != "" {
			wf.TargetRef = yamlWf.Target.Name
		}
	}

	// Build a set of all node IDs for validation
	nodeIDs := make(map[string]bool)
	for _, yamlNode := range yamlWf.Nodes {
		if yamlNode.ID == "" {
			return nil, fmt.Errorf("node ID is required")
		}
		if nodeIDs[yamlNode.ID] {
			return nil, fmt.Errorf("duplicate node ID: %s", yamlNode.ID)
		}
		nodeIDs[yamlNode.ID] = true
	}

	// Convert YAML nodes to WorkflowNodes
	for _, yamlNode := range yamlWf.Nodes {
		node, err := convertYAMLNode(&yamlNode)
		if err != nil {
			return nil, fmt.Errorf("failed to convert node %s: %w", yamlNode.ID, err)
		}
		wf.Nodes[node.ID] = node

		// Create edges from dependencies
		for _, depID := range yamlNode.DependsOn {
			if !nodeIDs[depID] {
				return nil, fmt.Errorf("node %s depends on non-existent node %s", yamlNode.ID, depID)
			}
			wf.Edges = append(wf.Edges, WorkflowEdge{
				From: depID,
				To:   yamlNode.ID,
			})
		}
	}

	// Calculate entry and exit points
	wf.EntryPoints = calculateEntryPoints(wf)
	wf.ExitPoints = calculateExitPoints(wf)

	return wf, nil
}

// ParseWorkflowFile reads a workflow definition from a YAML file and parses it.
// This is a convenience wrapper around ParseWorkflow that handles file I/O.
//
// Returns an error if:
//   - File cannot be read
//   - Any error from ParseWorkflow
func ParseWorkflowFile(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	return ParseWorkflow(data)
}

// convertYAMLNode converts a YAMLNode to a WorkflowNode with appropriate type validation
// and field mapping.
func convertYAMLNode(yamlNode *YAMLNode) (*WorkflowNode, error) {
	if yamlNode.Type == "" {
		return nil, fmt.Errorf("node type is required")
	}

	node := &WorkflowNode{
		ID:           yamlNode.ID,
		Name:         yamlNode.Name,
		Description:  yamlNode.Description,
		Dependencies: yamlNode.DependsOn,
		Metadata:     yamlNode.Metadata,
	}

	// Parse timeout if specified
	if yamlNode.Timeout != "" {
		timeout, err := time.ParseDuration(yamlNode.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout format %q: %w", yamlNode.Timeout, err)
		}
		node.Timeout = timeout
	}

	// Parse retry policy if specified
	if yamlNode.Retry != nil {
		retryPolicy, err := convertYAMLRetry(yamlNode.Retry)
		if err != nil {
			return nil, fmt.Errorf("invalid retry policy: %w", err)
		}
		node.RetryPolicy = retryPolicy
	}

	// Convert based on node type
	switch yamlNode.Type {
	case "agent":
		if err := convertAgentNode(yamlNode, node); err != nil {
			return nil, err
		}
		node.Type = NodeTypeAgent

	case "tool":
		if err := convertToolNode(yamlNode, node); err != nil {
			return nil, err
		}
		node.Type = NodeTypeTool

	case "plugin":
		if err := convertPluginNode(yamlNode, node); err != nil {
			return nil, err
		}
		node.Type = NodeTypePlugin

	case "condition":
		if err := convertConditionNode(yamlNode, node); err != nil {
			return nil, err
		}
		node.Type = NodeTypeCondition

	case "parallel":
		if err := convertParallelNode(yamlNode, node); err != nil {
			return nil, err
		}
		node.Type = NodeTypeParallel

	case "join":
		node.Type = NodeTypeJoin
		// Join nodes don't have additional fields

	default:
		return nil, fmt.Errorf("invalid node type: %s", yamlNode.Type)
	}

	return node, nil
}

// convertAgentNode converts agent-specific fields from YAML to WorkflowNode
func convertAgentNode(yamlNode *YAMLNode, node *WorkflowNode) error {
	if yamlNode.Agent == "" {
		return fmt.Errorf("agent name is required for agent nodes")
	}

	node.AgentName = yamlNode.Agent

	// Build the agent.Task from workflow YAML fields
	// The SDK Task has: ID, Goal, Context, Constraints, Metadata
	// We map workflow fields to these SDK fields cleanly:
	//   - goal: (node level) -> Task.Goal
	//   - context: (node level) -> Task.Context (merged with task: contents)
	//   - task: (config map) -> Task.Context (agent-specific config)

	task := agent.Task{
		ID:        types.NewID(),
		Name:      yamlNode.Name,
		Timeout:   node.Timeout,
		CreatedAt: time.Now(),
		Priority:  0,
		Tags:      []string{},
	}

	// Set Goal from node-level goal field (primary) or fall back to description
	if yamlNode.Goal != "" {
		task.Goal = yamlNode.Goal
	} else if yamlNode.Description != "" {
		task.Goal = yamlNode.Description
	}

	// Set Description
	task.Description = yamlNode.Description

	// Build Context by merging node-level context with task config
	// This gives agents a single Context map with all the info they need
	task.Context = make(map[string]any)

	// First, add node-level context (phase, etc.)
	for k, v := range yamlNode.Context {
		task.Context[k] = v
	}

	// Then merge in task config (agent-specific parameters like verbose, timeout, etc.)
	// Note: Agents should be single-purpose. Use different agents for different tasks
	// rather than mode-switching within a single agent.
	// Task config values take precedence if there's overlap
	for k, v := range yamlNode.Task {
		task.Context[k] = v
	}

	// Keep Input for backwards compatibility (deprecated)
	task.Input = yamlNode.Task

	node.AgentTask = &task

	return nil
}

// convertToolNode converts tool-specific fields from YAML to WorkflowNode
func convertToolNode(yamlNode *YAMLNode, node *WorkflowNode) error {
	if yamlNode.Tool == "" {
		return fmt.Errorf("tool name is required for tool nodes")
	}

	node.ToolName = yamlNode.Tool
	node.ToolInput = yamlNode.Input

	return nil
}

// convertPluginNode converts plugin-specific fields from YAML to WorkflowNode
func convertPluginNode(yamlNode *YAMLNode, node *WorkflowNode) error {
	if yamlNode.Plugin == "" {
		return fmt.Errorf("plugin name is required for plugin nodes")
	}
	if yamlNode.Method == "" {
		return fmt.Errorf("plugin method is required for plugin nodes")
	}

	node.PluginName = yamlNode.Plugin
	node.PluginMethod = yamlNode.Method
	node.PluginParams = yamlNode.Params

	return nil
}

// convertConditionNode converts condition-specific fields from YAML to WorkflowNode
func convertConditionNode(yamlNode *YAMLNode, node *WorkflowNode) error {
	if yamlNode.Condition == nil {
		return fmt.Errorf("condition is required for condition nodes")
	}
	if yamlNode.Condition.Expression == "" {
		return fmt.Errorf("condition expression is required")
	}

	node.Condition = &NodeCondition{
		Expression:  yamlNode.Condition.Expression,
		TrueBranch:  yamlNode.Condition.TrueBranch,
		FalseBranch: yamlNode.Condition.FalseBranch,
	}

	return nil
}

// convertParallelNode converts parallel-specific fields from YAML to WorkflowNode
func convertParallelNode(yamlNode *YAMLNode, node *WorkflowNode) error {
	if len(yamlNode.SubNodes) == 0 {
		return fmt.Errorf("parallel nodes must contain at least one sub-node")
	}

	subNodes := make([]*WorkflowNode, 0, len(yamlNode.SubNodes))
	for i, yamlSubNode := range yamlNode.SubNodes {
		// Ensure sub-nodes have IDs
		if yamlSubNode.ID == "" {
			yamlSubNode.ID = fmt.Sprintf("%s_sub_%d", yamlNode.ID, i)
		}

		subNode, err := convertYAMLNode(&yamlSubNode)
		if err != nil {
			return fmt.Errorf("failed to convert sub-node %s: %w", yamlSubNode.ID, err)
		}
		subNodes = append(subNodes, subNode)
	}

	node.SubNodes = subNodes
	return nil
}

// convertYAMLRetry converts a YAMLRetry to a RetryPolicy with validation
func convertYAMLRetry(yamlRetry *YAMLRetry) (*RetryPolicy, error) {
	if yamlRetry.MaxRetries < 0 {
		return nil, fmt.Errorf("max_retries must be non-negative")
	}

	policy := &RetryPolicy{
		MaxRetries: yamlRetry.MaxRetries,
		Multiplier: yamlRetry.Multiplier,
	}

	// Parse backoff strategy
	switch yamlRetry.Backoff {
	case "constant":
		policy.BackoffStrategy = BackoffConstant
	case "linear":
		policy.BackoffStrategy = BackoffLinear
	case "exponential":
		policy.BackoffStrategy = BackoffExponential
	default:
		return nil, fmt.Errorf("invalid backoff strategy %q (must be constant, linear, or exponential)", yamlRetry.Backoff)
	}

	// Parse initial delay
	if yamlRetry.InitialDelay != "" {
		initialDelay, err := time.ParseDuration(yamlRetry.InitialDelay)
		if err != nil {
			return nil, fmt.Errorf("invalid initial_delay format %q: %w", yamlRetry.InitialDelay, err)
		}
		policy.InitialDelay = initialDelay
	}

	// Parse max delay
	if yamlRetry.MaxDelay != "" {
		maxDelay, err := time.ParseDuration(yamlRetry.MaxDelay)
		if err != nil {
			return nil, fmt.Errorf("invalid max_delay format %q: %w", yamlRetry.MaxDelay, err)
		}
		policy.MaxDelay = maxDelay
	}

	// Validate exponential backoff parameters
	if policy.BackoffStrategy == BackoffExponential {
		if policy.Multiplier <= 0 {
			return nil, fmt.Errorf("multiplier must be positive for exponential backoff")
		}
		if policy.MaxDelay == 0 {
			return nil, fmt.Errorf("max_delay is required for exponential backoff")
		}
	}

	return policy, nil
}

// calculateEntryPoints identifies nodes with no incoming edges (no dependencies)
func calculateEntryPoints(wf *Workflow) []string {
	// Build a set of nodes that have incoming edges
	hasIncoming := make(map[string]bool)
	for _, edge := range wf.Edges {
		hasIncoming[edge.To] = true
	}

	// Nodes without incoming edges are entry points
	entryPoints := []string{}
	for id := range wf.Nodes {
		if !hasIncoming[id] {
			entryPoints = append(entryPoints, id)
		}
	}

	return entryPoints
}

// calculateExitPoints identifies nodes with no outgoing edges
func calculateExitPoints(wf *Workflow) []string {
	// Build a set of nodes that have outgoing edges
	hasOutgoing := make(map[string]bool)
	for _, edge := range wf.Edges {
		hasOutgoing[edge.From] = true
	}

	// Also handle condition nodes' branches
	for id, node := range wf.Nodes {
		if node.Type == NodeTypeCondition && node.Condition != nil {
			if len(node.Condition.TrueBranch) > 0 || len(node.Condition.FalseBranch) > 0 {
				hasOutgoing[id] = true
			}
		}
	}

	// Nodes without outgoing edges are exit points
	exitPoints := []string{}
	for id := range wf.Nodes {
		if !hasOutgoing[id] {
			exitPoints = append(exitPoints, id)
		}
	}

	return exitPoints
}
