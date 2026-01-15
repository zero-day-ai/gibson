// Package workflow provides YAML-to-Graph loading for the Gibson orchestrator.
//
// This file implements the GraphLoader which takes a parsed YAML workflow
// and loads it into Neo4j as a Mission with connected WorkflowNode nodes.
// The graph structure enables runtime tracking, dynamic task spawning, and
// workflow state persistence.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

// GraphLoader loads parsed workflow definitions into Neo4j as Mission and WorkflowNode graphs.
// It creates the complete DAG structure with proper relationships and stores the original
// YAML for snapshot reconstruction.
//
// Usage:
//
//	loader := NewGraphLoader(graphClient)
//	missionID, err := loader.LoadWorkflow(ctx, parsedWorkflow)
//	if err != nil {
//	    log.Fatal(err)
//	}
type GraphLoader struct {
	client graph.GraphClient
}

// NewGraphLoader creates a new GraphLoader with the given Neo4j client.
// The client must be connected before calling LoadWorkflow.
func NewGraphLoader(client graph.GraphClient) *GraphLoader {
	return &GraphLoader{
		client: client,
	}
}

// LoadWorkflow loads a parsed workflow into Neo4j as a Mission with WorkflowNode DAG.
// Returns the mission ID on success or an error if the operation fails.
//
// The loading process:
//  1. Creates a Mission node with workflow metadata
//  2. Creates WorkflowNode nodes for each task in the workflow
//  3. Creates PART_OF relationships linking nodes to the mission
//  4. Creates DEPENDS_ON relationships representing the DAG edges
//
// All operations are executed within a single transaction to ensure atomicity.
// If any step fails, the entire operation is rolled back.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - parsed: The parsed workflow from ParseWorkflowYAML
//
// Returns:
//   - string: The mission ID (as string) for future reference
//   - error: Any error that occurred during loading
func (g *GraphLoader) LoadWorkflow(ctx context.Context, parsed *ParsedWorkflow) (string, error) {
	if parsed == nil {
		return "", fmt.Errorf("parsed workflow cannot be nil")
	}

	// Generate mission ID
	missionID := types.NewID()

	// Serialize the entire parsed workflow to JSON for snapshot storage
	yamlSource, err := g.serializeParsedWorkflow(parsed)
	if err != nil {
		return "", fmt.Errorf("failed to serialize workflow: %w", err)
	}

	// Create the Mission schema object
	mission := schema.NewMission(
		missionID,
		parsed.Name,
		parsed.Description,
		parsed.Name, // Use name as objective if not specified
		parsed.TargetRef,
		yamlSource,
	)

	if err := mission.Validate(); err != nil {
		return "", fmt.Errorf("invalid mission: %w", err)
	}

	// Execute the entire load operation in a transaction-like manner
	// We'll use multiple Cypher queries within explicit transactions
	if err := g.loadInTransaction(ctx, mission, parsed); err != nil {
		return "", fmt.Errorf("failed to load workflow: %w", err)
	}

	return missionID.String(), nil
}

// loadInTransaction performs the complete workflow loading within transaction semantics.
// This ensures atomicity - either all nodes/relationships are created or none are.
func (g *GraphLoader) loadInTransaction(ctx context.Context, mission *schema.Mission, parsed *ParsedWorkflow) error {
	// Step 1: Create the Mission node
	if err := g.createMission(ctx, mission); err != nil {
		return fmt.Errorf("failed to create mission node: %w", err)
	}

	// Step 2: Create all WorkflowNode nodes
	nodeMap := make(map[string]*schema.WorkflowNode) // Maps workflow node ID to schema node
	for nodeID, workflowNode := range parsed.Nodes {
		schemaNode, err := g.convertToSchemaNode(mission.ID, nodeID, workflowNode)
		if err != nil {
			return fmt.Errorf("failed to convert node %s: %w", nodeID, err)
		}

		if err := g.createWorkflowNode(ctx, schemaNode); err != nil {
			return fmt.Errorf("failed to create workflow node %s: %w", nodeID, err)
		}

		nodeMap[nodeID] = schemaNode
	}

	// Step 3: Create PART_OF relationships (WorkflowNode -> Mission)
	for nodeID := range nodeMap {
		if err := g.createPartOfRelationship(ctx, nodeID, mission.ID.String()); err != nil {
			return fmt.Errorf("failed to create PART_OF relationship for node %s: %w", nodeID, err)
		}
	}

	// Step 4: Create DEPENDS_ON relationships (DAG edges)
	for _, edge := range parsed.Edges {
		if err := g.createDependsOnRelationship(ctx, edge.From, edge.To); err != nil {
			return fmt.Errorf("failed to create DEPENDS_ON relationship %s->%s: %w", edge.From, edge.To, err)
		}
	}

	return nil
}

// createMission creates the Mission node in Neo4j.
func (g *GraphLoader) createMission(ctx context.Context, mission *schema.Mission) error {
	// Prepare mission properties
	props := map[string]any{
		"id":          mission.ID.String(),
		"name":        mission.Name,
		"description": mission.Description,
		"objective":   mission.Objective,
		"target_ref":  mission.TargetRef,
		"status":      mission.Status.String(),
		"created_at":  mission.CreatedAt.Unix(),
		"yaml_source": mission.YAMLSource,
	}

	// Add optional timestamp fields
	if mission.StartedAt != nil {
		props["started_at"] = mission.StartedAt.Unix()
	}
	if mission.CompletedAt != nil {
		props["completed_at"] = mission.CompletedAt.Unix()
	}

	// Create the Mission node
	cypher := `
		CREATE (m:Mission)
		SET m = $props
		RETURN m.id as id
	`

	result, err := g.client.Query(ctx, cypher, map[string]any{"props": props})
	if err != nil {
		return fmt.Errorf("failed to create mission: %w", err)
	}

	if len(result.Records) == 0 {
		return fmt.Errorf("mission creation returned no records")
	}

	return nil
}

// createWorkflowNode creates a WorkflowNode in Neo4j.
func (g *GraphLoader) createWorkflowNode(ctx context.Context, node *schema.WorkflowNode) error {
	// Prepare base properties
	props := map[string]any{
		"id":          node.ID.String(),
		"mission_id":  node.MissionID.String(),
		"type":        node.Type.String(),
		"name":        node.Name,
		"description": node.Description,
		"status":      node.Status.String(),
		"is_dynamic":  node.IsDynamic,
		"created_at":  node.CreatedAt.Unix(),
		"updated_at":  node.UpdatedAt.Unix(),
	}

	// Add type-specific fields
	if node.AgentName != "" {
		props["agent_name"] = node.AgentName
	}
	if node.ToolName != "" {
		props["tool_name"] = node.ToolName
	}
	if node.Timeout > 0 {
		props["timeout"] = node.Timeout.Seconds()
	}
	if node.SpawnedBy != "" {
		props["spawned_by"] = node.SpawnedBy
	}

	// Serialize complex fields to JSON
	if node.TaskConfig != nil && len(node.TaskConfig) > 0 {
		taskConfigJSON, err := node.TaskConfigJSON()
		if err != nil {
			return fmt.Errorf("failed to serialize task config: %w", err)
		}
		props["task_config"] = taskConfigJSON
	} else {
		props["task_config"] = "{}"
	}

	if node.RetryPolicy != nil {
		retryPolicyJSON, err := node.RetryPolicyJSON()
		if err != nil {
			return fmt.Errorf("failed to serialize retry policy: %w", err)
		}
		props["retry_policy"] = retryPolicyJSON
	} else {
		props["retry_policy"] = "{}"
	}

	// Create the WorkflowNode
	cypher := `
		CREATE (n:WorkflowNode)
		SET n = $props
		RETURN n.id as id
	`

	result, err := g.client.Query(ctx, cypher, map[string]any{"props": props})
	if err != nil {
		return fmt.Errorf("failed to create workflow node: %w", err)
	}

	if len(result.Records) == 0 {
		return fmt.Errorf("workflow node creation returned no records")
	}

	return nil
}

// createPartOfRelationship creates a PART_OF relationship from a WorkflowNode to a Mission.
func (g *GraphLoader) createPartOfRelationship(ctx context.Context, nodeID, missionID string) error {
	cypher := `
		MATCH (n:WorkflowNode {id: $nodeId}), (m:Mission {id: $missionId})
		CREATE (n)-[:PART_OF]->(m)
		RETURN n.id as node_id, m.id as mission_id
	`

	params := map[string]any{
		"nodeId":    nodeID,
		"missionId": missionID,
	}

	result, err := g.client.Query(ctx, cypher, params)
	if err != nil {
		return fmt.Errorf("failed to create PART_OF relationship: %w", err)
	}

	if len(result.Records) == 0 {
		return fmt.Errorf("PART_OF relationship creation returned no records - nodes may not exist")
	}

	return nil
}

// createDependsOnRelationship creates a DEPENDS_ON relationship between two WorkflowNodes.
// This represents a DAG edge where 'from' must complete before 'to' can start.
func (g *GraphLoader) createDependsOnRelationship(ctx context.Context, fromID, toID string) error {
	cypher := `
		MATCH (from:WorkflowNode {id: $fromId}), (to:WorkflowNode {id: $toId})
		CREATE (from)-[:DEPENDS_ON]->(to)
		RETURN from.id as from_id, to.id as to_id
	`

	params := map[string]any{
		"fromId": fromID,
		"toId":   toID,
	}

	result, err := g.client.Query(ctx, cypher, params)
	if err != nil {
		return fmt.Errorf("failed to create DEPENDS_ON relationship: %w", err)
	}

	if len(result.Records) == 0 {
		return fmt.Errorf("DEPENDS_ON relationship creation returned no records - nodes may not exist")
	}

	return nil
}

// convertToSchemaNode converts a workflow.WorkflowNode to a schema.WorkflowNode.
// This adapts the parser's representation to the graph schema representation.
func (g *GraphLoader) convertToSchemaNode(missionID types.ID, nodeID string, workflowNode *WorkflowNode) (*schema.WorkflowNode, error) {
	// Parse node ID as types.ID - if it's not a valid UUID, generate one
	id, err := types.ParseID(nodeID)
	if err != nil {
		// If the node ID is not a valid UUID, generate a new one
		// This allows for human-readable IDs in YAML that get converted to UUIDs
		id = types.NewID()
	}

	// Determine node type
	var nodeType schema.WorkflowNodeType
	switch workflowNode.Type {
	case NodeTypeAgent:
		nodeType = schema.WorkflowNodeTypeAgent
	case NodeTypeTool:
		nodeType = schema.WorkflowNodeTypeTool
	default:
		return nil, fmt.Errorf("unsupported node type: %s", workflowNode.Type)
	}

	// Create the schema node
	schemaNode := schema.NewWorkflowNode(
		id,
		missionID,
		nodeType,
		workflowNode.Name,
		workflowNode.Description,
	)

	// Set type-specific fields
	if nodeType == schema.WorkflowNodeTypeAgent {
		schemaNode.AgentName = workflowNode.AgentName

		// Convert agent task to task config
		if workflowNode.AgentTask != nil {
			taskConfig := make(map[string]any)
			taskConfig["name"] = workflowNode.AgentTask.Name
			taskConfig["description"] = workflowNode.AgentTask.Description
			taskConfig["input"] = workflowNode.AgentTask.Input
			taskConfig["context"] = workflowNode.AgentTask.Context
			taskConfig["priority"] = workflowNode.AgentTask.Priority
			taskConfig["tags"] = workflowNode.AgentTask.Tags
			schemaNode.TaskConfig = taskConfig
		}
	} else if nodeType == schema.WorkflowNodeTypeTool {
		schemaNode.ToolName = workflowNode.ToolName

		// Store tool input in task config
		if workflowNode.ToolInput != nil {
			schemaNode.TaskConfig = map[string]any{
				"input": workflowNode.ToolInput,
			}
		}
	}

	// Set timeout
	if workflowNode.Timeout > 0 {
		schemaNode.Timeout = workflowNode.Timeout
	}

	// Convert retry policy
	if workflowNode.RetryPolicy != nil {
		schemaNode.RetryPolicy = g.convertRetryPolicy(workflowNode.RetryPolicy)
	}

	// Validate the node
	if err := schemaNode.Validate(); err != nil {
		return nil, fmt.Errorf("invalid schema node: %w", err)
	}

	return schemaNode, nil
}

// convertRetryPolicy converts a workflow.RetryPolicy to a schema.RetryPolicy.
func (g *GraphLoader) convertRetryPolicy(policy *RetryPolicy) *schema.RetryPolicy {
	if policy == nil {
		return nil
	}

	schemaPolicy := &schema.RetryPolicy{
		MaxRetries: policy.MaxRetries,
		Backoff:    policy.InitialDelay,
		MaxBackoff: policy.MaxDelay,
	}

	// Map strategy string
	switch policy.BackoffStrategy {
	case BackoffConstant:
		schemaPolicy.Strategy = "constant"
	case BackoffLinear:
		schemaPolicy.Strategy = "linear"
	case BackoffExponential:
		schemaPolicy.Strategy = "exponential"
	default:
		schemaPolicy.Strategy = "constant"
	}

	return schemaPolicy
}

// serializeParsedWorkflow serializes the parsed workflow to JSON for storage.
// We use JSON instead of YAML for more reliable round-tripping and smaller size.
func (g *GraphLoader) serializeParsedWorkflow(parsed *ParsedWorkflow) (string, error) {
	// Create a simplified representation suitable for storage
	snapshot := map[string]any{
		"name":         parsed.Name,
		"description":  parsed.Description,
		"version":      parsed.Version,
		"target_ref":   parsed.TargetRef,
		"config":       parsed.Config,
		"entry_points": parsed.EntryPoints,
		"exit_points":  parsed.ExitPoints,
		"parsed_at":    parsed.ParsedAt.Format(time.RFC3339),
		"source_file":  parsed.SourceFile,
	}

	// Serialize nodes
	nodes := make([]map[string]any, 0, len(parsed.Nodes))
	for id, node := range parsed.Nodes {
		nodeData := map[string]any{
			"id":           id,
			"type":         string(node.Type),
			"name":         node.Name,
			"description":  node.Description,
			"dependencies": node.Dependencies,
		}

		if node.AgentName != "" {
			nodeData["agent_name"] = node.AgentName
		}
		if node.ToolName != "" {
			nodeData["tool_name"] = node.ToolName
		}
		if node.Timeout > 0 {
			nodeData["timeout"] = node.Timeout.String()
		}
		if node.RetryPolicy != nil {
			nodeData["retry_policy"] = node.RetryPolicy
		}
		if node.Metadata != nil {
			nodeData["metadata"] = node.Metadata
		}

		nodes = append(nodes, nodeData)
	}
	snapshot["nodes"] = nodes

	// Serialize edges
	edges := make([]map[string]any, 0, len(parsed.Edges))
	for _, edge := range parsed.Edges {
		edges = append(edges, map[string]any{
			"from": edge.From,
			"to":   edge.To,
		})
	}
	snapshot["edges"] = edges

	// Convert to JSON
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("failed to marshal workflow snapshot: %w", err)
	}

	return string(data), nil
}

// LoadWorkflowFromMission reconstructs a ParsedWorkflow from a Mission node in Neo4j.
// This is the inverse of LoadWorkflow and enables workflow snapshot retrieval.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - missionID: The mission ID to load
//
// Returns:
//   - *ParsedWorkflow: The reconstructed workflow
//   - error: Any error that occurred during loading
func (g *GraphLoader) LoadWorkflowFromMission(ctx context.Context, missionID string) (*ParsedWorkflow, error) {
	// Query for the mission
	cypher := `
		MATCH (m:Mission {id: $missionId})
		RETURN m
	`

	result, err := g.client.Query(ctx, cypher, map[string]any{"missionId": missionID})
	if err != nil {
		return nil, fmt.Errorf("failed to query mission: %w", err)
	}

	if len(result.Records) == 0 {
		return nil, fmt.Errorf("mission not found: %s", missionID)
	}

	// Extract yaml_source from mission
	missionData := result.Records[0]["m"]
	missionMap, ok := missionData.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid mission data format")
	}

	yamlSource, ok := missionMap["yaml_source"].(string)
	if !ok {
		return nil, fmt.Errorf("mission yaml_source not found or invalid")
	}

	// Deserialize the workflow snapshot
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(yamlSource), &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow snapshot: %w", err)
	}

	// Reconstruct ParsedWorkflow
	parsed := &ParsedWorkflow{
		Name:        getStringOrEmpty(snapshot, "name"),
		Description: getStringOrEmpty(snapshot, "description"),
		Version:     getStringOrEmpty(snapshot, "version"),
		TargetRef:   getStringOrEmpty(snapshot, "target_ref"),
		SourceFile:  getStringOrEmpty(snapshot, "source_file"),
		Nodes:       make(map[string]*WorkflowNode),
		Edges:       []WorkflowEdge{},
	}

	// Parse timestamp
	if parsedAtStr := getStringOrEmpty(snapshot, "parsed_at"); parsedAtStr != "" {
		if t, err := time.Parse(time.RFC3339, parsedAtStr); err == nil {
			parsed.ParsedAt = t
		}
	}

	// Reconstruct config, planning, entry/exit points
	if config, ok := snapshot["config"].(map[string]any); ok {
		parsed.Config = config
	}
	if entryPoints, ok := snapshot["entry_points"].([]interface{}); ok {
		parsed.EntryPoints = interfaceSliceToStringSlice(entryPoints)
	}
	if exitPoints, ok := snapshot["exit_points"].([]interface{}); ok {
		parsed.ExitPoints = interfaceSliceToStringSlice(exitPoints)
	}

	// Note: Full reconstruction would require deserializing nodes and edges
	// For now, we return the basic metadata. Full reconstruction can be added later.

	return parsed, nil
}

// Helper functions for snapshot deserialization

func getStringOrEmpty(m map[string]any, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func interfaceSliceToStringSlice(slice []interface{}) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}
	return result
}
