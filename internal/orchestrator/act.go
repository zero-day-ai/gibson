package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/queries"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

// Harness defines the interface for agent delegation and tool execution.
// This matches the SDK harness interface for seamless agent integration.
type Harness interface {
	// DelegateToAgent delegates a task to another agent and waits for the result
	DelegateToAgent(ctx context.Context, agentName string, task agent.Task) (agent.Result, error)

	// CallTool executes a tool and returns its output
	CallTool(ctx context.Context, toolName string, input map[string]interface{}) (interface{}, error)
}

// Actor executes orchestrator decisions by performing the appropriate actions
// in the graph and delegating to agents as needed.
type Actor struct {
	harness        Harness
	execQueries    *queries.ExecutionQueries
	missionQueries *queries.MissionQueries
	graphClient    graph.GraphClient
}

// NewActor creates a new Actor with the given dependencies.
// The harness is used for agent delegation and tool execution.
// The queries provide graph operations for tracking execution state.
func NewActor(harness Harness, execQueries *queries.ExecutionQueries, missionQueries *queries.MissionQueries, graphClient graph.GraphClient) *Actor {
	return &Actor{
		harness:        harness,
		execQueries:    execQueries,
		missionQueries: missionQueries,
		graphClient:    graphClient,
	}
}

// ActionResult contains the outcome of executing a decision action.
// It includes all relevant state changes, execution results, and metadata
// needed for logging to Langfuse and updating the graph.
type ActionResult struct {
	// Action is the decision action that was executed
	Action DecisionAction

	// AgentExecution contains execution details if an agent was run
	AgentExecution *schema.AgentExecution

	// NewNode contains the newly spawned node if spawn_agent was used
	NewNode *schema.WorkflowNode

	// Error contains any error that occurred during action execution
	Error error

	// IsTerminal indicates if this action ends the orchestration loop
	IsTerminal bool

	// TargetNodeID is the node that was acted upon
	TargetNodeID string

	// Metadata contains additional action-specific metadata
	Metadata map[string]interface{}
}

// Act executes the given decision and returns the result.
// This method orchestrates all the actions needed to fulfill the decision,
// including graph updates, agent delegation, and error handling.
func (a *Actor) Act(ctx context.Context, decision *Decision, missionID string) (*ActionResult, error) {
	if decision == nil {
		return nil, fmt.Errorf("decision cannot be nil")
	}

	// Validate decision before acting
	if err := decision.Validate(); err != nil {
		return nil, fmt.Errorf("invalid decision: %w", err)
	}

	// Parse mission ID
	parsedMissionID, err := types.ParseID(missionID)
	if err != nil {
		return nil, fmt.Errorf("invalid mission ID: %w", err)
	}

	// Route to appropriate action handler
	switch decision.Action {
	case ActionExecuteAgent:
		return a.executeAgent(ctx, decision, parsedMissionID)

	case ActionSkipAgent:
		return a.skipAgent(ctx, decision, parsedMissionID)

	case ActionModifyParams:
		return a.modifyParams(ctx, decision, parsedMissionID)

	case ActionRetry:
		return a.retryAgent(ctx, decision, parsedMissionID)

	case ActionSpawnAgent:
		return a.spawnAgent(ctx, decision, parsedMissionID)

	case ActionComplete:
		return a.completeMission(ctx, decision, parsedMissionID)

	default:
		return nil, fmt.Errorf("unknown decision action: %s", decision.Action)
	}
}

// executeAgent executes the workflow node by delegating to the appropriate agent.
// This creates an execution node, delegates to the agent, and updates the graph
// with the execution results.
func (a *Actor) executeAgent(ctx context.Context, decision *Decision, missionID types.ID) (*ActionResult, error) {
	// Get the workflow node
	node, err := a.getWorkflowNode(ctx, decision.TargetNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow node: %w", err)
	}

	// Verify it's an agent node
	if node.Type != schema.WorkflowNodeTypeAgent {
		return nil, fmt.Errorf("node %s is not an agent node (type: %s)", node.ID, node.Type)
	}

	// Determine attempt number by counting previous executions
	prevExecutions, err := a.execQueries.GetNodeExecutions(ctx, node.ID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get previous executions: %w", err)
	}
	attemptNum := len(prevExecutions) + 1

	// Create agent execution node
	execution := schema.NewAgentExecution(node.ID.String(), missionID)
	execution.WithAttempt(attemptNum)
	execution.WithConfig(node.TaskConfig)

	// Create execution in graph
	if err := a.execQueries.CreateAgentExecution(ctx, execution); err != nil {
		return nil, fmt.Errorf("failed to create agent execution: %w", err)
	}

	// Update workflow node status to running
	if err := a.updateNodeStatus(ctx, node.ID, schema.WorkflowNodeStatusRunning); err != nil {
		return nil, fmt.Errorf("failed to update node status: %w", err)
	}

	// Build agent task
	task := agent.NewTask(node.Name, node.Description, node.TaskConfig)
	task.MissionID = &missionID
	if node.Timeout > 0 {
		task = task.WithTimeout(node.Timeout)
	}

	// Delegate to agent
	result, err := a.harness.DelegateToAgent(ctx, node.AgentName, task)

	// Update execution based on result
	if err != nil {
		// Agent delegation failed
		execution.MarkFailed(err.Error())
		if updateErr := a.execQueries.UpdateExecution(ctx, execution); updateErr != nil {
			return nil, fmt.Errorf("failed to update execution after error: %w", updateErr)
		}

		// Update node status to failed
		if updateErr := a.updateNodeStatus(ctx, node.ID, schema.WorkflowNodeStatusFailed); updateErr != nil {
			return nil, fmt.Errorf("failed to update node status: %w", updateErr)
		}

		return &ActionResult{
			Action:         ActionExecuteAgent,
			AgentExecution: execution,
			Error:          err,
			IsTerminal:     false,
			TargetNodeID:   decision.TargetNodeID,
		}, nil
	}

	// Check if agent execution failed
	if result.Status == agent.ResultStatusFailed {
		errMsg := "agent execution failed"
		if result.Error != nil {
			errMsg = result.Error.Message
		}
		execution.MarkFailed(errMsg)

		// Update node status to failed
		if updateErr := a.updateNodeStatus(ctx, node.ID, schema.WorkflowNodeStatusFailed); updateErr != nil {
			return nil, fmt.Errorf("failed to update node status: %w", updateErr)
		}
	} else {
		// Execution succeeded
		execution.MarkCompleted()
		execution.WithResult(result.Output)

		// Update node status to completed
		if updateErr := a.updateNodeStatus(ctx, node.ID, schema.WorkflowNodeStatusCompleted); updateErr != nil {
			return nil, fmt.Errorf("failed to update node status: %w", updateErr)
		}
	}

	// Update execution in graph
	if updateErr := a.execQueries.UpdateExecution(ctx, execution); updateErr != nil {
		return nil, fmt.Errorf("failed to update execution: %w", updateErr)
	}

	return &ActionResult{
		Action:         ActionExecuteAgent,
		AgentExecution: execution,
		Error:          nil,
		IsTerminal:     false,
		TargetNodeID:   decision.TargetNodeID,
		Metadata: map[string]interface{}{
			"agent_name":     node.AgentName,
			"attempt":        attemptNum,
			"result_status":  string(result.Status),
			"findings_count": len(result.Findings),
		},
	}, nil
}

// skipAgent marks a workflow node as skipped with the reasoning from the decision.
// This is used when the orchestrator determines a node doesn't need to execute.
func (a *Actor) skipAgent(ctx context.Context, decision *Decision, missionID types.ID) (*ActionResult, error) {
	// Get the workflow node
	node, err := a.getWorkflowNode(ctx, decision.TargetNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow node: %w", err)
	}

	// Update node status to skipped
	if err := a.updateNodeStatus(ctx, node.ID, schema.WorkflowNodeStatusSkipped); err != nil {
		return nil, fmt.Errorf("failed to update node status: %w", err)
	}

	return &ActionResult{
		Action:       ActionSkipAgent,
		Error:        nil,
		IsTerminal:   false,
		TargetNodeID: decision.TargetNodeID,
		Metadata: map[string]interface{}{
			"reasoning":  decision.Reasoning,
			"confidence": decision.Confidence,
		},
	}, nil
}

// modifyParams updates the task configuration for a workflow node and then executes it.
// This allows the orchestrator to adapt agent parameters based on context.
func (a *Actor) modifyParams(ctx context.Context, decision *Decision, missionID types.ID) (*ActionResult, error) {
	// Get the workflow node
	node, err := a.getWorkflowNode(ctx, decision.TargetNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow node: %w", err)
	}

	// Merge modifications into task config
	mergedConfig := make(map[string]interface{})
	for k, v := range node.TaskConfig {
		mergedConfig[k] = v
	}
	for k, v := range decision.Modifications {
		mergedConfig[k] = v
	}

	// Update node task config in graph
	if err := a.updateNodeConfig(ctx, node.ID, mergedConfig); err != nil {
		return nil, fmt.Errorf("failed to update node config: %w", err)
	}

	// Reload node with updated config
	node.TaskConfig = mergedConfig

	// Now execute the agent with modified params
	// Create a new decision for execute_agent
	execDecision := &Decision{
		Reasoning:    fmt.Sprintf("Executing with modified params: %s", decision.Reasoning),
		Action:       ActionExecuteAgent,
		TargetNodeID: decision.TargetNodeID,
		Confidence:   decision.Confidence,
	}

	return a.executeAgent(ctx, execDecision, missionID)
}

// retryAgent re-executes a failed workflow node, optionally with modified parameters.
// This implements retry logic based on the node's retry policy.
func (a *Actor) retryAgent(ctx context.Context, decision *Decision, missionID types.ID) (*ActionResult, error) {
	// Get the workflow node
	node, err := a.getWorkflowNode(ctx, decision.TargetNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow node: %w", err)
	}

	// Check retry policy
	if node.RetryPolicy != nil {
		prevExecutions, err := a.execQueries.GetNodeExecutions(ctx, node.ID.String())
		if err != nil {
			return nil, fmt.Errorf("failed to get previous executions: %w", err)
		}

		if len(prevExecutions) >= node.RetryPolicy.MaxRetries {
			return nil, fmt.Errorf("max retries (%d) exceeded for node %s", node.RetryPolicy.MaxRetries, node.ID)
		}
	}

	// If modifications are provided, apply them first
	if len(decision.Modifications) > 0 {
		mergedConfig := make(map[string]interface{})
		for k, v := range node.TaskConfig {
			mergedConfig[k] = v
		}
		for k, v := range decision.Modifications {
			mergedConfig[k] = v
		}

		if err := a.updateNodeConfig(ctx, node.ID, mergedConfig); err != nil {
			return nil, fmt.Errorf("failed to update node config: %w", err)
		}

		node.TaskConfig = mergedConfig
	}

	// Reset node status to ready for retry
	if err := a.updateNodeStatus(ctx, node.ID, schema.WorkflowNodeStatusReady); err != nil {
		return nil, fmt.Errorf("failed to reset node status: %w", err)
	}

	// Execute the agent
	execDecision := &Decision{
		Reasoning:    fmt.Sprintf("Retry attempt: %s", decision.Reasoning),
		Action:       ActionExecuteAgent,
		TargetNodeID: decision.TargetNodeID,
		Confidence:   decision.Confidence,
	}

	return a.executeAgent(ctx, execDecision, missionID)
}

// spawnAgent creates a new workflow node dynamically and optionally executes it.
// This allows the orchestrator to adapt the workflow at runtime.
func (a *Actor) spawnAgent(ctx context.Context, decision *Decision, missionID types.ID) (*ActionResult, error) {
	if decision.SpawnConfig == nil {
		return nil, fmt.Errorf("spawn_config is required for spawn_agent action")
	}

	cfg := decision.SpawnConfig

	// Create new workflow node
	nodeID := types.NewID()
	node := schema.NewAgentNode(
		nodeID,
		missionID,
		cfg.AgentName,
		cfg.Description,
		cfg.AgentName,
	)
	node.MarkDynamic("orchestrator") // Mark as dynamically spawned
	node.WithTaskConfig(cfg.TaskConfig)
	node.WithStatus(schema.WorkflowNodeStatusReady)

	// Create node in graph
	if err := a.createWorkflowNode(ctx, node); err != nil {
		return nil, fmt.Errorf("failed to create workflow node: %w", err)
	}

	// Create dependencies if specified
	if len(cfg.DependsOn) > 0 {
		if err := a.createNodeDependencies(ctx, nodeID, cfg.DependsOn); err != nil {
			return nil, fmt.Errorf("failed to create node dependencies: %w", err)
		}
	}

	// Link to mission
	if err := a.linkNodeToMission(ctx, missionID, nodeID); err != nil {
		return nil, fmt.Errorf("failed to link node to mission: %w", err)
	}

	return &ActionResult{
		Action:       ActionSpawnAgent,
		NewNode:      node,
		Error:        nil,
		IsTerminal:   false,
		TargetNodeID: nodeID.String(),
		Metadata: map[string]interface{}{
			"agent_name":  cfg.AgentName,
			"description": cfg.Description,
			"depends_on":  cfg.DependsOn,
		},
	}, nil
}

// completeMission marks the mission as complete and returns a terminal result.
// This stops the orchestration loop.
func (a *Actor) completeMission(ctx context.Context, decision *Decision, missionID types.ID) (*ActionResult, error) {
	// Update mission status in graph
	cypher := `
		MATCH (m:Mission {id: $mission_id})
		SET m.status = 'completed',
			m.completed_at = datetime()
		RETURN m.id as id
	`

	params := map[string]interface{}{
		"mission_id": missionID.String(),
	}

	result, err := a.graphClient.Query(ctx, cypher, params)
	if err != nil {
		return nil, fmt.Errorf("failed to update mission status: %w", err)
	}

	if len(result.Records) == 0 {
		return nil, fmt.Errorf("mission %s not found", missionID)
	}

	return &ActionResult{
		Action:     ActionComplete,
		Error:      nil,
		IsTerminal: true,
		Metadata: map[string]interface{}{
			"stop_reason": decision.StopReason,
			"confidence":  decision.Confidence,
		},
	}, nil
}

// getWorkflowNode retrieves a workflow node by ID from the graph.
func (a *Actor) getWorkflowNode(ctx context.Context, nodeID string) (*schema.WorkflowNode, error) {
	cypher := `
		MATCH (n:WorkflowNode {id: $node_id})
		RETURN n
	`

	params := map[string]interface{}{
		"node_id": nodeID,
	}

	result, err := a.graphClient.Query(ctx, cypher, params)
	if err != nil {
		return nil, fmt.Errorf("failed to query workflow node: %w", err)
	}

	if len(result.Records) == 0 {
		return nil, fmt.Errorf("workflow node %s not found", nodeID)
	}

	nodeData, ok := result.Records[0]["n"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid node data format")
	}

	return a.parseWorkflowNode(nodeData)
}

// updateNodeStatus updates the status of a workflow node in the graph.
func (a *Actor) updateNodeStatus(ctx context.Context, nodeID types.ID, status schema.WorkflowNodeStatus) error {
	cypher := `
		MATCH (n:WorkflowNode {id: $node_id})
		SET n.status = $status,
			n.updated_at = datetime()
		RETURN n.id as id
	`

	params := map[string]interface{}{
		"node_id": nodeID.String(),
		"status":  string(status),
	}

	result, err := a.graphClient.Query(ctx, cypher, params)
	if err != nil {
		return fmt.Errorf("failed to update node status: %w", err)
	}

	if len(result.Records) == 0 {
		return fmt.Errorf("node %s not found", nodeID)
	}

	return nil
}

// updateNodeConfig updates the task configuration of a workflow node in the graph.
func (a *Actor) updateNodeConfig(ctx context.Context, nodeID types.ID, config map[string]interface{}) error {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	cypher := `
		MATCH (n:WorkflowNode {id: $node_id})
		SET n.task_config = $config,
			n.updated_at = datetime()
		RETURN n.id as id
	`

	params := map[string]interface{}{
		"node_id": nodeID.String(),
		"config":  string(configJSON),
	}

	result, err := a.graphClient.Query(ctx, cypher, params)
	if err != nil {
		return fmt.Errorf("failed to update node config: %w", err)
	}

	if len(result.Records) == 0 {
		return fmt.Errorf("node %s not found", nodeID)
	}

	return nil
}

// createWorkflowNode creates a new workflow node in the graph.
func (a *Actor) createWorkflowNode(ctx context.Context, node *schema.WorkflowNode) error {
	if err := node.Validate(); err != nil {
		return fmt.Errorf("invalid workflow node: %w", err)
	}

	configJSON, err := node.TaskConfigJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal task config: %w", err)
	}

	retryPolicyJSON, err := node.RetryPolicyJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal retry policy: %w", err)
	}

	cypher := `
		CREATE (n:WorkflowNode)
		SET n.id = $id,
			n.mission_id = $mission_id,
			n.type = $type,
			n.name = $name,
			n.description = $description,
			n.agent_name = $agent_name,
			n.tool_name = $tool_name,
			n.timeout = $timeout,
			n.retry_policy = $retry_policy,
			n.task_config = $task_config,
			n.status = $status,
			n.is_dynamic = $is_dynamic,
			n.spawned_by = $spawned_by,
			n.created_at = datetime(),
			n.updated_at = datetime()
		RETURN n.id as id
	`

	params := map[string]interface{}{
		"id":           node.ID.String(),
		"mission_id":   node.MissionID.String(),
		"type":         string(node.Type),
		"name":         node.Name,
		"description":  node.Description,
		"agent_name":   node.AgentName,
		"tool_name":    node.ToolName,
		"timeout":      node.Timeout.Milliseconds(),
		"retry_policy": retryPolicyJSON,
		"task_config":  configJSON,
		"status":       string(node.Status),
		"is_dynamic":   node.IsDynamic,
		"spawned_by":   node.SpawnedBy,
	}

	result, err := a.graphClient.Query(ctx, cypher, params)
	if err != nil {
		return fmt.Errorf("failed to create workflow node: %w", err)
	}

	if len(result.Records) == 0 {
		return fmt.Errorf("failed to create workflow node")
	}

	return nil
}

// createNodeDependencies creates DEPENDS_ON relationships between nodes.
func (a *Actor) createNodeDependencies(ctx context.Context, nodeID types.ID, dependsOn []string) error {
	if len(dependsOn) == 0 {
		return nil
	}

	cypher := `
		MATCH (n:WorkflowNode {id: $node_id})
		WITH n
		UNWIND $depends_on as dep_id
		MATCH (dep:WorkflowNode {id: dep_id})
		MERGE (n)-[:DEPENDS_ON]->(dep)
		RETURN count(*) as count
	`

	params := map[string]interface{}{
		"node_id":    nodeID.String(),
		"depends_on": dependsOn,
	}

	result, err := a.graphClient.Query(ctx, cypher, params)
	if err != nil {
		return fmt.Errorf("failed to create dependencies: %w", err)
	}

	if len(result.Records) == 0 {
		return fmt.Errorf("failed to create dependencies")
	}

	return nil
}

// linkNodeToMission creates a HAS_NODE relationship from mission to node.
func (a *Actor) linkNodeToMission(ctx context.Context, missionID, nodeID types.ID) error {
	cypher := `
		MATCH (m:Mission {id: $mission_id})
		MATCH (n:WorkflowNode {id: $node_id})
		MERGE (m)-[:HAS_NODE]->(n)
		RETURN count(*) as count
	`

	params := map[string]interface{}{
		"mission_id": missionID.String(),
		"node_id":    nodeID.String(),
	}

	result, err := a.graphClient.Query(ctx, cypher, params)
	if err != nil {
		return fmt.Errorf("failed to link node to mission: %w", err)
	}

	if len(result.Records) == 0 {
		return fmt.Errorf("failed to link node to mission")
	}

	return nil
}

// parseWorkflowNode converts a Neo4j result map to a WorkflowNode struct.
func (a *Actor) parseWorkflowNode(data map[string]interface{}) (*schema.WorkflowNode, error) {
	id, err := types.ParseID(data["id"].(string))
	if err != nil {
		return nil, fmt.Errorf("invalid node ID: %w", err)
	}

	missionID, err := types.ParseID(data["mission_id"].(string))
	if err != nil {
		return nil, fmt.Errorf("invalid mission ID: %w", err)
	}

	node := &schema.WorkflowNode{
		ID:          id,
		MissionID:   missionID,
		Type:        schema.WorkflowNodeType(data["type"].(string)),
		Name:        data["name"].(string),
		Description: data["description"].(string),
		Status:      schema.WorkflowNodeStatus(data["status"].(string)),
		IsDynamic:   data["is_dynamic"].(bool),
		TaskConfig:  make(map[string]interface{}),
	}

	// Optional fields
	if agentName, ok := data["agent_name"].(string); ok && agentName != "" {
		node.AgentName = agentName
	}
	if toolName, ok := data["tool_name"].(string); ok && toolName != "" {
		node.ToolName = toolName
	}
	if spawnedBy, ok := data["spawned_by"].(string); ok && spawnedBy != "" {
		node.SpawnedBy = spawnedBy
	}
	if timeout, ok := data["timeout"].(int64); ok && timeout > 0 {
		node.Timeout = time.Duration(timeout) * time.Millisecond
	}
	if createdAt, ok := data["created_at"].(time.Time); ok {
		node.CreatedAt = createdAt
	}
	if updatedAt, ok := data["updated_at"].(time.Time); ok {
		node.UpdatedAt = updatedAt
	}

	// Parse JSON fields
	if taskConfigStr, ok := data["task_config"].(string); ok && taskConfigStr != "" && taskConfigStr != "{}" {
		if err := json.Unmarshal([]byte(taskConfigStr), &node.TaskConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal task_config: %w", err)
		}
	}

	if retryPolicyStr, ok := data["retry_policy"].(string); ok && retryPolicyStr != "" && retryPolicyStr != "{}" {
		var retryPolicy schema.RetryPolicy
		if err := json.Unmarshal([]byte(retryPolicyStr), &retryPolicy); err != nil {
			return nil, fmt.Errorf("failed to unmarshal retry_policy: %w", err)
		}
		node.RetryPolicy = &retryPolicy
	}

	return node, nil
}
