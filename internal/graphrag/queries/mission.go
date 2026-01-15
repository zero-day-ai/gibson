// Package queries provides specialized Cypher query functions for mission execution tracking.
package queries

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MissionQueries provides high-level query operations for mission execution data.
type MissionQueries struct {
	client graph.GraphClient
}

// NewMissionQueries creates a new MissionQueries with the given graph client.
func NewMissionQueries(client graph.GraphClient) *MissionQueries {
	return &MissionQueries{
		client: client,
	}
}

// GetMission retrieves a mission by ID.
func (mq *MissionQueries) GetMission(ctx context.Context, missionID types.ID) (*schema.Mission, error) {
	cypher := `
		MATCH (m:Mission {id: $mission_id})
		RETURN m
	`

	params := map[string]any{
		"mission_id": missionID.String(),
	}

	result, err := mq.client.Query(ctx, cypher, params)
	if err != nil {
		return nil, err
	}

	if len(result.Records) == 0 {
		return nil, fmt.Errorf("mission not found: %s", missionID)
	}

	return recordToMission(result.Records[0]["m"])
}

// GetMissionNodes retrieves all workflow nodes for a mission.
func (mq *MissionQueries) GetMissionNodes(ctx context.Context, missionID types.ID) ([]*schema.WorkflowNode, error) {
	cypher := `
		MATCH (m:Mission {id: $mission_id})-[:HAS_NODE]->(n:WorkflowNode)
		RETURN n
		ORDER BY n.created_at
	`

	params := map[string]any{
		"mission_id": missionID.String(),
	}

	result, err := mq.client.Query(ctx, cypher, params)
	if err != nil {
		return nil, err
	}

	nodes := make([]*schema.WorkflowNode, 0, len(result.Records))
	for _, record := range result.Records {
		node, err := recordToWorkflowNode(record["n"])
		if err != nil {
			return nil, fmt.Errorf("failed to parse workflow node: %w", err)
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetMissionDecisions retrieves all orchestrator decisions for a mission, ordered by iteration.
func (mq *MissionQueries) GetMissionDecisions(ctx context.Context, missionID types.ID) ([]*schema.Decision, error) {
	cypher := `
		MATCH (m:Mission {id: $mission_id})-[:HAS_DECISION]->(d:Decision)
		RETURN d
		ORDER BY d.iteration, d.timestamp
	`

	params := map[string]any{
		"mission_id": missionID.String(),
	}

	result, err := mq.client.Query(ctx, cypher, params)
	if err != nil {
		return nil, err
	}

	decisions := make([]*schema.Decision, 0, len(result.Records))
	for _, record := range result.Records {
		decision, err := recordToDecision(record["d"])
		if err != nil {
			return nil, fmt.Errorf("failed to parse decision: %w", err)
		}
		decisions = append(decisions, decision)
	}

	return decisions, nil
}

// GetNodeExecutions retrieves all executions for a specific workflow node.
func (mq *MissionQueries) GetNodeExecutions(ctx context.Context, nodeID types.ID) ([]*schema.AgentExecution, error) {
	cypher := `
		MATCH (n:WorkflowNode {id: $node_id})-[:HAS_EXECUTION]->(e:AgentExecution)
		RETURN e
		ORDER BY e.started_at
	`

	params := map[string]any{
		"node_id": nodeID.String(),
	}

	result, err := mq.client.Query(ctx, cypher, params)
	if err != nil {
		return nil, err
	}

	executions := make([]*schema.AgentExecution, 0, len(result.Records))
	for _, record := range result.Records {
		exec, err := recordToAgentExecution(record["e"])
		if err != nil {
			return nil, fmt.Errorf("failed to parse execution: %w", err)
		}
		executions = append(executions, exec)
	}

	return executions, nil
}

// GetReadyNodes returns workflow nodes that are ready to execute.
// A node is ready if all its dependencies have status "completed".
func (mq *MissionQueries) GetReadyNodes(ctx context.Context, missionID types.ID) ([]*schema.WorkflowNode, error) {
	cypher := `
		MATCH (m:Mission {id: $mission_id})-[:HAS_NODE]->(n:WorkflowNode)
		WHERE n.status = 'ready'
		AND NOT EXISTS {
			MATCH (n)-[:DEPENDS_ON]->(dep:WorkflowNode)
			WHERE dep.status <> 'completed'
		}
		RETURN n
		ORDER BY n.created_at
	`

	params := map[string]any{
		"mission_id": missionID.String(),
	}

	result, err := mq.client.Query(ctx, cypher, params)
	if err != nil {
		return nil, err
	}

	nodes := make([]*schema.WorkflowNode, 0, len(result.Records))
	for _, record := range result.Records {
		node, err := recordToWorkflowNode(record["n"])
		if err != nil {
			return nil, fmt.Errorf("failed to parse workflow node: %w", err)
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetNodeDependencies returns the nodes that a given node depends on.
func (mq *MissionQueries) GetNodeDependencies(ctx context.Context, nodeID types.ID) ([]*schema.WorkflowNode, error) {
	cypher := `
		MATCH (n:WorkflowNode {id: $node_id})-[:DEPENDS_ON]->(dep:WorkflowNode)
		RETURN dep
		ORDER BY dep.created_at
	`

	params := map[string]any{
		"node_id": nodeID.String(),
	}

	result, err := mq.client.Query(ctx, cypher, params)
	if err != nil {
		return nil, err
	}

	nodes := make([]*schema.WorkflowNode, 0, len(result.Records))
	for _, record := range result.Records {
		node, err := recordToWorkflowNode(record["dep"])
		if err != nil {
			return nil, fmt.Errorf("failed to parse dependency node: %w", err)
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetMissionStats returns execution statistics for a mission.
type MissionStats struct {
	TotalNodes      int       `json:"total_nodes"`
	CompletedNodes  int       `json:"completed_nodes"`
	FailedNodes     int       `json:"failed_nodes"`
	PendingNodes    int       `json:"pending_nodes"`
	TotalDecisions  int       `json:"total_decisions"`
	TotalExecutions int       `json:"total_executions"`
	StartTime       time.Time `json:"start_time,omitempty"`
	EndTime         time.Time `json:"end_time,omitempty"`
}

// GetMissionStats computes execution statistics for a mission.
func (mq *MissionQueries) GetMissionStats(ctx context.Context, missionID types.ID) (*MissionStats, error) {
	cypher := `
		MATCH (m:Mission {id: $mission_id})
		OPTIONAL MATCH (m)-[:HAS_NODE]->(n:WorkflowNode)
		OPTIONAL MATCH (m)-[:HAS_DECISION]->(d:Decision)
		OPTIONAL MATCH (n)-[:HAS_EXECUTION]->(e:AgentExecution)
		RETURN 
			COUNT(DISTINCT n) as total_nodes,
			COUNT(DISTINCT CASE WHEN n.status = 'completed' THEN n END) as completed_nodes,
			COUNT(DISTINCT CASE WHEN n.status = 'failed' THEN n END) as failed_nodes,
			COUNT(DISTINCT CASE WHEN n.status = 'pending' OR n.status = 'ready' THEN n END) as pending_nodes,
			COUNT(DISTINCT d) as total_decisions,
			COUNT(DISTINCT e) as total_executions,
			m.created_at as start_time,
			m.completed_at as end_time
	`

	params := map[string]any{
		"mission_id": missionID.String(),
	}

	result, err := mq.client.Query(ctx, cypher, params)
	if err != nil {
		return nil, err
	}

	if len(result.Records) == 0 {
		return nil, fmt.Errorf("mission not found: %s", missionID)
	}

	record := result.Records[0]
	stats := &MissionStats{
		TotalNodes:      int(record["total_nodes"].(int64)),
		CompletedNodes:  int(record["completed_nodes"].(int64)),
		FailedNodes:     int(record["failed_nodes"].(int64)),
		PendingNodes:    int(record["pending_nodes"].(int64)),
		TotalDecisions:  int(record["total_decisions"].(int64)),
		TotalExecutions: int(record["total_executions"].(int64)),
	}

	if startTime, ok := record["start_time"].(time.Time); ok {
		stats.StartTime = startTime
	}
	if endTime, ok := record["end_time"].(time.Time); ok {
		stats.EndTime = endTime
	}

	return stats, nil
}

// Helper functions to convert Neo4j records to schema types

func recordToMission(data any) (*schema.Mission, error) {
	m, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid mission data type: %T", data)
	}

	id, err := types.ParseID(m["id"].(string))
	if err != nil {
		return nil, fmt.Errorf("invalid mission ID: %w", err)
	}

	mission := &schema.Mission{
		ID:          id,
		Name:        m["name"].(string),
		Description: m["description"].(string),
		Objective:   m["objective"].(string),
		TargetRef:   m["target_ref"].(string),
		Status:      schema.MissionStatus(m["status"].(string)),
		YAMLSource:  m["yaml_source"].(string),
	}

	if createdAt, ok := m["created_at"].(time.Time); ok {
		mission.CreatedAt = createdAt
	}
	if startedAt, ok := m["started_at"].(time.Time); ok {
		mission.StartedAt = &startedAt
	}
	if completedAt, ok := m["completed_at"].(time.Time); ok {
		mission.CompletedAt = &completedAt
	}

	return mission, nil
}

func recordToWorkflowNode(data any) (*schema.WorkflowNode, error) {
	n, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid workflow node data type: %T", data)
	}

	id, err := types.ParseID(n["id"].(string))
	if err != nil {
		return nil, fmt.Errorf("invalid node ID: %w", err)
	}

	missionID, err := types.ParseID(n["mission_id"].(string))
	if err != nil {
		return nil, fmt.Errorf("invalid mission ID: %w", err)
	}

	node := &schema.WorkflowNode{
		ID:          id,
		MissionID:   missionID,
		Type:        schema.WorkflowNodeType(n["type"].(string)),
		Name:        n["name"].(string),
		Description: n["description"].(string),
		Status:      schema.WorkflowNodeStatus(n["status"].(string)),
		IsDynamic:   n["is_dynamic"].(bool),
		TaskConfig:  make(map[string]any),
	}

	if agentName, ok := n["agent_name"].(string); ok && agentName != "" {
		node.AgentName = agentName
	}
	if toolName, ok := n["tool_name"].(string); ok && toolName != "" {
		node.ToolName = toolName
	}
	if spawnedBy, ok := n["spawned_by"].(string); ok && spawnedBy != "" {
		node.SpawnedBy = spawnedBy
	}
	if timeout, ok := n["timeout"].(int64); ok && timeout > 0 {
		node.Timeout = time.Duration(timeout) * time.Millisecond
	}
	if createdAt, ok := n["created_at"].(time.Time); ok {
		node.CreatedAt = createdAt
	}
	if updatedAt, ok := n["updated_at"].(time.Time); ok {
		node.UpdatedAt = updatedAt
	}

	// Parse JSON fields
	if taskConfigStr, ok := n["task_config"].(string); ok && taskConfigStr != "" && taskConfigStr != "{}" {
		if err := json.Unmarshal([]byte(taskConfigStr), &node.TaskConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal task_config: %w", err)
		}
	}

	if retryPolicyStr, ok := n["retry_policy"].(string); ok && retryPolicyStr != "" && retryPolicyStr != "{}" {
		var retryPolicy schema.RetryPolicy
		if err := json.Unmarshal([]byte(retryPolicyStr), &retryPolicy); err != nil {
			return nil, fmt.Errorf("failed to unmarshal retry_policy: %w", err)
		}
		node.RetryPolicy = &retryPolicy
	}

	return node, nil
}

func recordToDecision(data any) (*schema.Decision, error) {
	d, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid decision data type: %T", data)
	}

	id, err := types.ParseID(d["id"].(string))
	if err != nil {
		return nil, fmt.Errorf("invalid decision ID: %w", err)
	}

	missionID, err := types.ParseID(d["mission_id"].(string))
	if err != nil {
		return nil, fmt.Errorf("invalid mission ID: %w", err)
	}

	decision := &schema.Decision{
		ID:            id,
		MissionID:     missionID,
		Iteration:     int(d["iteration"].(int64)),
		Action:        schema.DecisionAction(d["action"].(string)),
		Reasoning:     d["reasoning"].(string),
		Confidence:    d["confidence"].(float64),
		Modifications: make(map[string]any),
	}

	if targetNodeID, ok := d["target_node_id"].(string); ok && targetNodeID != "" {
		decision.TargetNodeID = targetNodeID
	}
	if graphStateSummary, ok := d["graph_state_summary"].(string); ok {
		decision.GraphStateSummary = graphStateSummary
	}
	if langfuseSpanID, ok := d["langfuse_span_id"].(string); ok {
		decision.LangfuseSpanID = langfuseSpanID
	}
	if promptTokens, ok := d["prompt_tokens"].(int64); ok {
		decision.PromptTokens = int(promptTokens)
	}
	if completionTokens, ok := d["completion_tokens"].(int64); ok {
		decision.CompletionTokens = int(completionTokens)
	}
	if latencyMs, ok := d["latency_ms"].(int64); ok {
		decision.LatencyMs = int(latencyMs)
	}
	if timestamp, ok := d["timestamp"].(time.Time); ok {
		decision.Timestamp = timestamp
	}
	if createdAt, ok := d["created_at"].(time.Time); ok {
		decision.CreatedAt = createdAt
	}
	if updatedAt, ok := d["updated_at"].(time.Time); ok {
		decision.UpdatedAt = updatedAt
	}

	// Parse modifications JSON
	if modsStr, ok := d["modifications"].(string); ok && modsStr != "" && modsStr != "{}" {
		if err := json.Unmarshal([]byte(modsStr), &decision.Modifications); err != nil {
			return nil, fmt.Errorf("failed to unmarshal modifications: %w", err)
		}
	}

	return decision, nil
}

func recordToAgentExecution(data any) (*schema.AgentExecution, error) {
	e, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid agent execution data type: %T", data)
	}

	id, err := types.ParseID(e["id"].(string))
	if err != nil {
		return nil, fmt.Errorf("invalid execution ID: %w", err)
	}

	missionID, err := types.ParseID(e["mission_id"].(string))
	if err != nil {
		return nil, fmt.Errorf("invalid mission ID: %w", err)
	}

	exec := &schema.AgentExecution{
		ID:             id,
		WorkflowNodeID: e["workflow_node_id"].(string),
		MissionID:      missionID,
		Status:         schema.ExecutionStatus(e["status"].(string)),
		Attempt:        int(e["attempt"].(int64)),
		Error:          "",
		ConfigUsed:     make(map[string]any),
		Result:         make(map[string]any),
	}

	if startedAt, ok := e["started_at"].(time.Time); ok {
		exec.StartedAt = startedAt
	}
	if completedAt, ok := e["completed_at"].(time.Time); ok {
		exec.CompletedAt = &completedAt
	}
	if errorMsg, ok := e["error"].(string); ok {
		exec.Error = errorMsg
	}
	if langfuseSpanID, ok := e["langfuse_span_id"].(string); ok {
		exec.LangfuseSpanID = langfuseSpanID
	}
	if createdAt, ok := e["created_at"].(time.Time); ok {
		exec.CreatedAt = createdAt
	}
	if updatedAt, ok := e["updated_at"].(time.Time); ok {
		exec.UpdatedAt = updatedAt
	}

	// Parse JSON fields
	if configStr, ok := e["config_used"].(string); ok && configStr != "" && configStr != "{}" {
		if err := json.Unmarshal([]byte(configStr), &exec.ConfigUsed); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config_used: %w", err)
		}
	}

	if resultStr, ok := e["result"].(string); ok && resultStr != "" && resultStr != "{}" {
		if err := json.Unmarshal([]byte(resultStr), &exec.Result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	return exec, nil
}
