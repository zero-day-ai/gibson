// Package workflow provides YAML workflow parsing with enhanced error reporting.
//
// This file implements the primary workflow parser that reads YAML workflow definitions
// and converts them into executable workflow structures. It provides comprehensive
// error reporting with line numbers for debugging workflow files.
package workflow

import (
	"fmt"
	"os"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
	"gopkg.in/yaml.v3"
)

// ParseError represents a workflow parsing error with source location information.
// It provides detailed context about where errors occur in YAML workflow files.
type ParseError struct {
	// Message is the human-readable error message
	Message string
	// Line is the line number where the error occurred (1-indexed)
	Line int
	// Column is the column number where the error occurred (1-indexed)
	Column int
	// NodeID is the ID of the node being parsed when the error occurred (if applicable)
	NodeID string
	// Err is the underlying error, if any
	Err error
}

// Error implements the error interface
func (e *ParseError) Error() string {
	if e.NodeID != "" {
		return fmt.Sprintf("parse error at line %d:%d (node %s): %s", e.Line, e.Column, e.NodeID, e.Message)
	}
	if e.Line > 0 {
		return fmt.Sprintf("parse error at line %d:%d: %s", e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("parse error: %s", e.Message)
}

// Unwrap returns the underlying error for error wrapping support
func (e *ParseError) Unwrap() error {
	return e.Err
}

// ParsedWorkflow represents the complete result of parsing a workflow YAML file.
// It contains all the information needed to create and execute a workflow.
type ParsedWorkflow struct {
	// Metadata about the workflow
	Name        string
	Description string
	Version     string

	// Target specification (optional)
	TargetRef string

	// Configuration sections
	Config map[string]any

	// Graph structure
	Nodes       map[string]*WorkflowNode
	Edges       []WorkflowEdge
	EntryPoints []string
	ExitPoints  []string

	// Source metadata for debugging
	SourceFile string
	ParsedAt   time.Time
}

// yamlWorkflowWithPosition is an internal structure that captures YAML nodes
// with their source positions for better error reporting.
type yamlWorkflowWithPosition struct {
	Node *yaml.Node
	Data *yamlWorkflowData
}

// yamlWorkflowData represents the complete workflow YAML structure with all sections
type yamlWorkflowData struct {
	Name        string               `yaml:"name"`
	Description string               `yaml:"description"`
	Version     string               `yaml:"version"`
	Config      map[string]any       `yaml:"config,omitempty"`
	Target      *yamlTargetWithSeeds `yaml:"target,omitempty"`
	Nodes       []yamlNodeData       `yaml:"nodes"`
}

// yamlTargetWithSeeds represents target configuration including seed values
type yamlTargetWithSeeds struct {
	Reference string                   `yaml:"-"` // For string form
	Name      string                   `yaml:"name,omitempty"`
	Type      string                   `yaml:"type,omitempty"`
	Seeds     []map[string]interface{} `yaml:"seeds,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshaling for target
func (t *yamlTargetWithSeeds) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try as string reference first
	var str string
	if err := unmarshal(&str); err == nil {
		t.Reference = str
		return nil
	}

	// Otherwise unmarshal as object
	type rawTarget yamlTargetWithSeeds
	var raw rawTarget
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*t = yamlTargetWithSeeds(raw)
	return nil
}

// yamlNodeWithPosition wraps a node definition with its YAML position
type yamlNodeWithPosition struct {
	yamlNode *yaml.Node
	data     *yamlNodeData
}

// yamlNodeData represents a single node definition with all possible fields
type yamlNodeData struct {
	ID          string                 `yaml:"id"`
	Type        string                 `yaml:"type"`
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	DependsOn   []string               `yaml:"depends_on,omitempty"`
	Timeout     string                 `yaml:"timeout,omitempty"`
	Retry       *yamlRetryData         `yaml:"retry,omitempty"`
	Agent       string                 `yaml:"agent,omitempty"`
	Task        map[string]interface{} `yaml:"task,omitempty"`
	Tool        string                 `yaml:"tool,omitempty"`
	Input       map[string]interface{} `yaml:"input,omitempty"`
	Plugin      string                 `yaml:"plugin,omitempty"`
	Method      string                 `yaml:"method,omitempty"`
	Params      map[string]interface{} `yaml:"params,omitempty"`
	Condition   *yamlConditionData     `yaml:"condition,omitempty"`
	SubNodes    []yamlNodeData         `yaml:"sub_nodes,omitempty"`
	Metadata    map[string]interface{} `yaml:"metadata,omitempty"`
}

// yamlRetryData represents retry policy configuration
type yamlRetryData struct {
	MaxRetries   int     `yaml:"max_retries"`
	Backoff      string  `yaml:"backoff"`
	InitialDelay string  `yaml:"initial_delay"`
	MaxDelay     string  `yaml:"max_delay,omitempty"`
	Multiplier   float64 `yaml:"multiplier,omitempty"`
}

// yamlConditionData represents conditional branching configuration
type yamlConditionData struct {
	Expression  string   `yaml:"expression"`
	TrueBranch  []string `yaml:"true_branch,omitempty"`
	FalseBranch []string `yaml:"false_branch,omitempty"`
}

// ParseWorkflowYAML parses a workflow definition from a YAML file.
// It reads the file from disk and converts it into a ParsedWorkflow structure
// with comprehensive error reporting including line numbers.
//
// Parameters:
//   - path: File system path to the YAML workflow file
//
// Returns:
//   - *ParsedWorkflow: The parsed workflow ready for execution
//   - error: Detailed parse error with line numbers, or nil on success
//
// Example usage:
//
//	wf, err := ParseWorkflowYAML("workflows/recon.yaml")
//	if err != nil {
//	    var parseErr *ParseError
//	    if errors.As(err, &parseErr) {
//	        fmt.Printf("Error at line %d: %s\n", parseErr.Line, parseErr.Message)
//	    }
//	    return err
//	}
func ParseWorkflowYAML(path string) (*ParsedWorkflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ParseError{
			Message: fmt.Sprintf("failed to read workflow file: %v", err),
			Err:     err,
		}
	}

	parsed, err := ParseWorkflowYAMLFromBytes(data)
	if err != nil {
		return nil, err
	}

	// Set source file for debugging
	parsed.SourceFile = path
	return parsed, nil
}

// ParseWorkflowYAMLFromBytes parses a workflow definition from raw YAML bytes.
// This function is useful for testing and for parsing workflow definitions
// that are generated dynamically or stored in databases.
//
// The parser performs comprehensive validation including:
//   - Required fields presence
//   - Node type validity
//   - Dependency graph validity (no dangling references)
//   - Duration format validation
//   - Retry policy validation
//   - DAG structure validation (entry/exit points)
//
// Parameters:
//   - data: Raw YAML bytes containing the workflow definition
//
// Returns:
//   - *ParsedWorkflow: The parsed workflow structure
//   - error: Detailed parse error with line numbers, or nil on success
//
// Example usage:
//
//	yamlData := []byte(`
//	name: Test Workflow
//	nodes:
//	  - id: node1
//	    type: agent
//	    agent: test-agent
//	`)
//	wf, err := ParseWorkflowYAMLFromBytes(yamlData)
func ParseWorkflowYAMLFromBytes(data []byte) (*ParsedWorkflow, error) {
	// First pass: unmarshal into a yaml.Node to preserve position information
	var rootNode yaml.Node
	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, &ParseError{
			Message: "invalid YAML syntax",
			Err:     err,
		}
	}

	// Second pass: unmarshal into our data structure
	var wfData yamlWorkflowData
	if err := yaml.Unmarshal(data, &wfData); err != nil {
		return nil, &ParseError{
			Message: "failed to parse workflow structure",
			Err:     err,
		}
	}

	// Validate required fields
	if wfData.Name == "" {
		return nil, &ParseError{
			Message: "workflow 'name' field is required",
			Line:    getFieldLine(&rootNode, "name"),
		}
	}

	if len(wfData.Nodes) == 0 {
		return nil, &ParseError{
			Message: "workflow must contain at least one node",
			Line:    getFieldLine(&rootNode, "nodes"),
		}
	}

	// Create parsed workflow structure
	parsed := &ParsedWorkflow{
		Name:        wfData.Name,
		Description: wfData.Description,
		Version:     wfData.Version,
		Config:      wfData.Config,
		Nodes:       make(map[string]*WorkflowNode),
		Edges:       []WorkflowEdge{},
		ParsedAt:    time.Now(),
	}

	// Extract target reference
	if wfData.Target != nil {
		if wfData.Target.Reference != "" {
			parsed.TargetRef = wfData.Target.Reference
		} else if wfData.Target.Name != "" {
			parsed.TargetRef = wfData.Target.Name
		}
	}

	// Build node ID set for validation
	nodeIDs := make(map[string]bool)
	nodeLines := make(map[string]int)

	for i, nodeData := range wfData.Nodes {
		if nodeData.ID == "" {
			return nil, &ParseError{
				Message: "node 'id' field is required",
				Line:    getNodeLine(&rootNode, i),
			}
		}

		if nodeIDs[nodeData.ID] {
			return nil, &ParseError{
				Message:  fmt.Sprintf("duplicate node ID: %s", nodeData.ID),
				Line:     getNodeLine(&rootNode, i),
				NodeID:   nodeData.ID,
			}
		}

		nodeIDs[nodeData.ID] = true
		nodeLines[nodeData.ID] = getNodeLine(&rootNode, i)
	}

	// Parse all nodes
	for i, nodeData := range wfData.Nodes {
		node, err := parseNode(&nodeData, getNodeLine(&rootNode, i))
		if err != nil {
			if parseErr, ok := err.(*ParseError); ok {
				parseErr.NodeID = nodeData.ID
				return nil, parseErr
			}
			return nil, &ParseError{
				Message: err.Error(),
				Line:    getNodeLine(&rootNode, i),
				NodeID:  nodeData.ID,
				Err:     err,
			}
		}

		parsed.Nodes[node.ID] = node
	}

	// Build edges from dependencies and validate references
	for _, nodeData := range wfData.Nodes {
		for _, depID := range nodeData.DependsOn {
			if !nodeIDs[depID] {
				return nil, &ParseError{
					Message: fmt.Sprintf("node depends on non-existent node '%s'", depID),
					Line:    nodeLines[nodeData.ID],
					NodeID:  nodeData.ID,
				}
			}

			parsed.Edges = append(parsed.Edges, WorkflowEdge{
				From: depID,
				To:   nodeData.ID,
			})
		}
	}

	// Calculate entry and exit points
	parsed.EntryPoints = calculateParsedEntryPoints(parsed)
	parsed.ExitPoints = calculateParsedExitPoints(parsed)

	return parsed, nil
}

// parseNode converts a YAML node definition to a WorkflowNode
func parseNode(nodeData *yamlNodeData, line int) (*WorkflowNode, error) {
	if nodeData.Type == "" {
		return nil, &ParseError{
			Message: "node 'type' field is required",
			Line:    line,
		}
	}

	node := &WorkflowNode{
		ID:           nodeData.ID,
		Name:         nodeData.Name,
		Description:  nodeData.Description,
		Dependencies: nodeData.DependsOn,
		Metadata:     nodeData.Metadata,
	}

	// Parse timeout
	if nodeData.Timeout != "" {
		timeout, err := time.ParseDuration(nodeData.Timeout)
		if err != nil {
			return nil, &ParseError{
				Message: fmt.Sprintf("invalid timeout format '%s': must be a valid Go duration (e.g., '30s', '5m')", nodeData.Timeout),
				Line:    line,
				Err:     err,
			}
		}
		node.Timeout = timeout
	}

	// Parse retry policy
	if nodeData.Retry != nil {
		retryPolicy, err := parseRetryPolicy(nodeData.Retry, line)
		if err != nil {
			return nil, err
		}
		node.RetryPolicy = retryPolicy
	}

	// Parse node type-specific fields
	var err error
	switch nodeData.Type {
	case "agent":
		err = parseAgentNode(nodeData, node, line)
		node.Type = NodeTypeAgent
	case "tool":
		err = parseToolNode(nodeData, node, line)
		node.Type = NodeTypeTool
	case "plugin":
		err = parsePluginNode(nodeData, node, line)
		node.Type = NodeTypePlugin
	case "condition":
		err = parseConditionNode(nodeData, node, line)
		node.Type = NodeTypeCondition
	case "parallel":
		err = parseParallelNode(nodeData, node, line)
		node.Type = NodeTypeParallel
	case "join":
		node.Type = NodeTypeJoin
	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("invalid node type '%s': must be one of: agent, tool, plugin, condition, parallel, join", nodeData.Type),
			Line:    line,
		}
	}

	if err != nil {
		return nil, err
	}

	return node, nil
}

// parseAgentNode parses agent-specific fields
func parseAgentNode(nodeData *yamlNodeData, node *WorkflowNode, line int) error {
	if nodeData.Agent == "" {
		return &ParseError{
			Message: "agent nodes require 'agent' field",
			Line:    line,
		}
	}

	node.AgentName = nodeData.Agent

	// Parse task definition
	if nodeData.Task != nil {
		task := agent.Task{
			ID:          types.NewID(),
			Name:        node.Name,
			Description: node.Description,
			Goal:        node.Description, // Default goal to description
			Input:       nodeData.Task,
			Timeout:     node.Timeout,
			CreatedAt:   time.Now(),
			Priority:    0,
			Tags:        []string{},
		}

		// Extract known task fields
		if name, ok := nodeData.Task["name"].(string); ok {
			task.Name = name
		}
		if desc, ok := nodeData.Task["description"].(string); ok {
			task.Description = desc
		}
		if goal, ok := nodeData.Task["goal"].(string); ok {
			task.Goal = goal
		}
		if context, ok := nodeData.Task["context"].(map[string]interface{}); ok {
			task.Context = context
		}
		if priority, ok := nodeData.Task["priority"].(int); ok {
			task.Priority = priority
		}
		if tagsRaw, ok := nodeData.Task["tags"].([]interface{}); ok {
			tags := make([]string, 0, len(tagsRaw))
			for _, t := range tagsRaw {
				if str, ok := t.(string); ok {
					tags = append(tags, str)
				}
			}
			task.Tags = tags
		}

		node.AgentTask = &task
	}

	return nil
}

// parseToolNode parses tool-specific fields
func parseToolNode(nodeData *yamlNodeData, node *WorkflowNode, line int) error {
	if nodeData.Tool == "" {
		return &ParseError{
			Message: "tool nodes require 'tool' field",
			Line:    line,
		}
	}

	node.ToolName = nodeData.Tool
	node.ToolInput = nodeData.Input
	return nil
}

// parsePluginNode parses plugin-specific fields
func parsePluginNode(nodeData *yamlNodeData, node *WorkflowNode, line int) error {
	if nodeData.Plugin == "" {
		return &ParseError{
			Message: "plugin nodes require 'plugin' field",
			Line:    line,
		}
	}
	if nodeData.Method == "" {
		return &ParseError{
			Message: "plugin nodes require 'method' field",
			Line:    line,
		}
	}

	node.PluginName = nodeData.Plugin
	node.PluginMethod = nodeData.Method
	node.PluginParams = nodeData.Params
	return nil
}

// parseConditionNode parses condition-specific fields
func parseConditionNode(nodeData *yamlNodeData, node *WorkflowNode, line int) error {
	if nodeData.Condition == nil {
		return &ParseError{
			Message: "condition nodes require 'condition' field",
			Line:    line,
		}
	}
	if nodeData.Condition.Expression == "" {
		return &ParseError{
			Message: "condition nodes require 'condition.expression' field",
			Line:    line,
		}
	}

	node.Condition = &NodeCondition{
		Expression:  nodeData.Condition.Expression,
		TrueBranch:  nodeData.Condition.TrueBranch,
		FalseBranch: nodeData.Condition.FalseBranch,
	}

	return nil
}

// parseParallelNode parses parallel-specific fields
func parseParallelNode(nodeData *yamlNodeData, node *WorkflowNode, line int) error {
	if len(nodeData.SubNodes) == 0 {
		return &ParseError{
			Message: "parallel nodes must contain at least one sub_node",
			Line:    line,
		}
	}

	subNodes := make([]*WorkflowNode, 0, len(nodeData.SubNodes))
	for i, subNodeData := range nodeData.SubNodes {
		// Auto-generate sub-node IDs if not provided
		if subNodeData.ID == "" {
			subNodeData.ID = fmt.Sprintf("%s_sub_%d", nodeData.ID, i)
		}

		subNode, err := parseNode(&subNodeData, line)
		if err != nil {
			if parseErr, ok := err.(*ParseError); ok {
				parseErr.Message = fmt.Sprintf("sub-node %d: %s", i, parseErr.Message)
				return parseErr
			}
			return &ParseError{
				Message: fmt.Sprintf("sub-node %d: %v", i, err),
				Line:    line,
				Err:     err,
			}
		}

		subNodes = append(subNodes, subNode)
	}

	node.SubNodes = subNodes
	return nil
}

// parseRetryPolicy parses and validates retry configuration
func parseRetryPolicy(retryData *yamlRetryData, line int) (*RetryPolicy, error) {
	if retryData.MaxRetries < 0 {
		return nil, &ParseError{
			Message: "retry.max_retries must be non-negative",
			Line:    line,
		}
	}

	policy := &RetryPolicy{
		MaxRetries: retryData.MaxRetries,
		Multiplier: retryData.Multiplier,
	}

	// Parse backoff strategy
	switch retryData.Backoff {
	case "constant":
		policy.BackoffStrategy = BackoffConstant
	case "linear":
		policy.BackoffStrategy = BackoffLinear
	case "exponential":
		policy.BackoffStrategy = BackoffExponential
	default:
		return nil, &ParseError{
			Message: fmt.Sprintf("invalid retry.backoff '%s': must be one of: constant, linear, exponential", retryData.Backoff),
			Line:    line,
		}
	}

	// Parse initial delay
	if retryData.InitialDelay != "" {
		initialDelay, err := time.ParseDuration(retryData.InitialDelay)
		if err != nil {
			return nil, &ParseError{
				Message: fmt.Sprintf("invalid retry.initial_delay '%s': must be a valid Go duration", retryData.InitialDelay),
				Line:    line,
				Err:     err,
			}
		}
		policy.InitialDelay = initialDelay
	}

	// Parse max delay
	if retryData.MaxDelay != "" {
		maxDelay, err := time.ParseDuration(retryData.MaxDelay)
		if err != nil {
			return nil, &ParseError{
				Message: fmt.Sprintf("invalid retry.max_delay '%s': must be a valid Go duration", retryData.MaxDelay),
				Line:    line,
				Err:     err,
			}
		}
		policy.MaxDelay = maxDelay
	}

	// Validate exponential backoff requirements
	if policy.BackoffStrategy == BackoffExponential {
		if policy.Multiplier <= 0 {
			return nil, &ParseError{
				Message: "retry.multiplier must be positive for exponential backoff",
				Line:    line,
			}
		}
		if policy.MaxDelay == 0 {
			return nil, &ParseError{
				Message: "retry.max_delay is required for exponential backoff",
				Line:    line,
			}
		}
	}

	return policy, nil
}

// calculateParsedEntryPoints identifies nodes with no incoming edges
func calculateParsedEntryPoints(parsed *ParsedWorkflow) []string {
	hasIncoming := make(map[string]bool)
	for _, edge := range parsed.Edges {
		hasIncoming[edge.To] = true
	}

	entryPoints := []string{}
	for id := range parsed.Nodes {
		if !hasIncoming[id] {
			entryPoints = append(entryPoints, id)
		}
	}

	return entryPoints
}

// calculateParsedExitPoints identifies nodes with no outgoing edges
func calculateParsedExitPoints(parsed *ParsedWorkflow) []string {
	hasOutgoing := make(map[string]bool)
	for _, edge := range parsed.Edges {
		hasOutgoing[edge.From] = true
	}

	// Handle condition nodes
	for id, node := range parsed.Nodes {
		if node.Type == NodeTypeCondition && node.Condition != nil {
			if len(node.Condition.TrueBranch) > 0 || len(node.Condition.FalseBranch) > 0 {
				hasOutgoing[id] = true
			}
		}
	}

	exitPoints := []string{}
	for id := range parsed.Nodes {
		if !hasOutgoing[id] {
			exitPoints = append(exitPoints, id)
		}
	}

	return exitPoints
}

// getFieldLine attempts to find the line number of a specific field in the YAML
func getFieldLine(node *yaml.Node, fieldName string) int {
	if node == nil || node.Kind != yaml.DocumentNode {
		return 0
	}

	// Document node contains the root content
	if len(node.Content) == 0 {
		return 0
	}

	return findFieldInMapping(node.Content[0], fieldName)
}

// getNodeLine attempts to find the line number of a node in the nodes array
func getNodeLine(rootNode *yaml.Node, index int) int {
	if rootNode == nil || rootNode.Kind != yaml.DocumentNode {
		return 0
	}

	if len(rootNode.Content) == 0 {
		return 0
	}

	mappingNode := rootNode.Content[0]
	if mappingNode.Kind != yaml.MappingNode {
		return 0
	}

	// Find the "nodes" key
	for i := 0; i < len(mappingNode.Content)-1; i += 2 {
		key := mappingNode.Content[i]
		value := mappingNode.Content[i+1]

		if key.Value == "nodes" && value.Kind == yaml.SequenceNode {
			if index < len(value.Content) {
				return value.Content[index].Line
			}
		}
	}

	return 0
}

// findFieldInMapping searches for a field in a YAML mapping node
func findFieldInMapping(node *yaml.Node, fieldName string) int {
	if node == nil || node.Kind != yaml.MappingNode {
		return 0
	}

	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i]
		if key.Value == fieldName {
			return key.Line
		}
	}

	return 0
}

// ToWorkflow converts a ParsedWorkflow to a Workflow structure for execution.
// This is used when transitioning from the parsing phase to the execution phase.
func (p *ParsedWorkflow) ToWorkflow() *Workflow {
	return &Workflow{
		ID:          types.NewID(),
		Name:        p.Name,
		Description: p.Description,
		TargetRef:   p.TargetRef,
		Nodes:       p.Nodes,
		Edges:       p.Edges,
		EntryPoints: p.EntryPoints,
		ExitPoints:  p.ExitPoints,
		Metadata:    p.Config,
		CreatedAt:   p.ParsedAt,
	}
}
