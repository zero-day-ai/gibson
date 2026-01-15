package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/queries"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

// Mock implementations

type MockHarness struct {
	mock.Mock
}

func (m *MockHarness) DelegateToAgent(ctx context.Context, agentName string, task agent.Task) (agent.Result, error) {
	args := m.Called(ctx, agentName, task)
	return args.Get(0).(agent.Result), args.Error(1)
}

func (m *MockHarness) CallTool(ctx context.Context, toolName string, input map[string]interface{}) (interface{}, error) {
	args := m.Called(ctx, toolName, input)
	return args.Get(0), args.Error(1)
}

type MockGraphClient struct {
	mock.Mock
}

func (m *MockGraphClient) Connect(ctx context.Context, uri, username, password string) error {
	args := m.Called(ctx, uri, username, password)
	return args.Error(0)
}

func (m *MockGraphClient) Close(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockGraphClient) Query(ctx context.Context, cypher string, params map[string]interface{}) (*graph.QueryResult, error) {
	args := m.Called(ctx, cypher, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*graph.QueryResult), args.Error(1)
}

func (m *MockGraphClient) IsConnected() bool {
	args := m.Called()
	return args.Bool(0)
}

type MockExecutionQueries struct {
	mock.Mock
}

func (m *MockExecutionQueries) CreateAgentExecution(ctx context.Context, exec *schema.AgentExecution) error {
	args := m.Called(ctx, exec)
	return args.Error(0)
}

func (m *MockExecutionQueries) UpdateExecution(ctx context.Context, exec *schema.AgentExecution) error {
	args := m.Called(ctx, exec)
	return args.Error(0)
}

func (m *MockExecutionQueries) GetNodeExecutions(ctx context.Context, nodeID string) ([]*schema.AgentExecution, error) {
	args := m.Called(ctx, nodeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*schema.AgentExecution), args.Error(1)
}

type MockMissionQueries struct {
	mock.Mock
}

func (m *MockMissionQueries) GetMission(ctx context.Context, missionID types.ID) (*schema.Mission, error) {
	args := m.Called(ctx, missionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*schema.Mission), args.Error(1)
}

// Helper functions

func setupActor(t *testing.T) (*Actor, *MockHarness, *MockExecutionQueries, *MockMissionQueries, *MockGraphClient) {
	mockHarness := new(MockHarness)
	mockExecQueries := new(MockExecutionQueries)
	mockMissionQueries := new(MockMissionQueries)
	mockGraphClient := new(MockGraphClient)

	actor := NewActor(mockHarness, mockExecQueries, mockMissionQueries, mockGraphClient)

	return actor, mockHarness, mockExecQueries, mockMissionQueries, mockGraphClient
}

func createTestWorkflowNode() *schema.WorkflowNode {
	missionID := types.NewID()
	nodeID := types.NewID()
	node := schema.NewAgentNode(nodeID, missionID, "test-agent", "Test agent description", "nmap_scanner")
	node.WithTaskConfig(map[string]interface{}{
		"target": "192.168.1.1",
		"ports":  "1-1000",
	})
	return node
}

// Tests

func TestNewActor(t *testing.T) {
	actor, harness, execQueries, missionQueries, graphClient := setupActor(t)

	assert.NotNil(t, actor)
	assert.Equal(t, harness, actor.harness)
	assert.Equal(t, execQueries, actor.execQueries)
	assert.Equal(t, missionQueries, actor.missionQueries)
	assert.Equal(t, graphClient, actor.graphClient)
}

func TestAct_NilDecision(t *testing.T) {
	actor, _, _, _, _ := setupActor(t)
	ctx := context.Background()

	result, err := actor.Act(ctx, nil, types.NewID().String())

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "decision cannot be nil")
}

func TestAct_InvalidDecision(t *testing.T) {
	actor, _, _, _, _ := setupActor(t)
	ctx := context.Background()

	decision := &Decision{
		Action: ActionExecuteAgent,
		// Missing required fields
	}

	result, err := actor.Act(ctx, decision, types.NewID().String())

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid decision")
}

func TestAct_InvalidMissionID(t *testing.T) {
	actor, _, _, _, _ := setupActor(t)
	ctx := context.Background()

	decision := &Decision{
		Reasoning:    "Test reasoning",
		Action:       ActionExecuteAgent,
		TargetNodeID: "node-1",
		Confidence:   0.9,
	}

	result, err := actor.Act(ctx, decision, "invalid-id")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid mission ID")
}

func TestExecuteAgent_Success(t *testing.T) {
	actor, mockHarness, mockExecQueries, _, mockGraphClient := setupActor(t)
	ctx := context.Background()

	node := createTestWorkflowNode()
	missionID := node.MissionID

	decision := &Decision{
		Reasoning:    "Execute nmap scan",
		Action:       ActionExecuteAgent,
		TargetNodeID: node.ID.String(),
		Confidence:   0.95,
	}

	// Mock get workflow node
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["node_id"] == node.ID.String()
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{
				"n": map[string]interface{}{
					"id":          node.ID.String(),
					"mission_id":  node.MissionID.String(),
					"type":        string(node.Type),
					"name":        node.Name,
					"description": node.Description,
					"agent_name":  node.AgentName,
					"status":      string(node.Status),
					"is_dynamic":  node.IsDynamic,
					"task_config": `{"target":"192.168.1.1","ports":"1-1000"}`,
					"timeout":     int64(0),
				},
			},
		},
	}, nil).Once()

	// Mock get previous executions (first attempt)
	mockExecQueries.On("GetNodeExecutions", ctx, node.ID.String()).Return([]*schema.AgentExecution{}, nil).Once()

	// Mock create agent execution
	mockExecQueries.On("CreateAgentExecution", ctx, mock.AnythingOfType("*schema.AgentExecution")).Return(nil).Once()

	// Mock update node status to running
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["node_id"] == node.ID.String() && params["status"] == string(schema.WorkflowNodeStatusRunning)
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": node.ID.String()},
		},
	}, nil).Once()

	// Mock agent delegation
	agentResult := agent.NewResult(types.NewID())
	agentResult.Status = agent.ResultStatusCompleted
	agentResult.Output = map[string]interface{}{
		"open_ports": []int{22, 80, 443},
	}
	mockHarness.On("DelegateToAgent", ctx, "nmap_scanner", mock.AnythingOfType("agent.Task")).Return(agentResult, nil).Once()

	// Mock update node status to completed
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["node_id"] == node.ID.String() && params["status"] == string(schema.WorkflowNodeStatusCompleted)
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": node.ID.String()},
		},
	}, nil).Once()

	// Mock update execution
	mockExecQueries.On("UpdateExecution", ctx, mock.AnythingOfType("*schema.AgentExecution")).Return(nil).Once()

	result, err := actor.Act(ctx, decision, missionID.String())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ActionExecuteAgent, result.Action)
	assert.NotNil(t, result.AgentExecution)
	assert.Nil(t, result.Error)
	assert.False(t, result.IsTerminal)
	assert.Equal(t, node.ID.String(), result.TargetNodeID)

	mockHarness.AssertExpectations(t)
	mockExecQueries.AssertExpectations(t)
	mockGraphClient.AssertExpectations(t)
}

func TestExecuteAgent_Failure(t *testing.T) {
	actor, mockHarness, mockExecQueries, _, mockGraphClient := setupActor(t)
	ctx := context.Background()

	node := createTestWorkflowNode()
	missionID := node.MissionID

	decision := &Decision{
		Reasoning:    "Execute nmap scan",
		Action:       ActionExecuteAgent,
		TargetNodeID: node.ID.String(),
		Confidence:   0.95,
	}

	// Mock get workflow node
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["node_id"] == node.ID.String()
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{
				"n": map[string]interface{}{
					"id":          node.ID.String(),
					"mission_id":  node.MissionID.String(),
					"type":        string(node.Type),
					"name":        node.Name,
					"description": node.Description,
					"agent_name":  node.AgentName,
					"status":      string(node.Status),
					"is_dynamic":  node.IsDynamic,
					"task_config": `{"target":"192.168.1.1"}`,
					"timeout":     int64(0),
				},
			},
		},
	}, nil).Once()

	// Mock get previous executions
	mockExecQueries.On("GetNodeExecutions", ctx, node.ID.String()).Return([]*schema.AgentExecution{}, nil).Once()

	// Mock create agent execution
	mockExecQueries.On("CreateAgentExecution", ctx, mock.AnythingOfType("*schema.AgentExecution")).Return(nil).Once()

	// Mock update node status to running
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["status"] == string(schema.WorkflowNodeStatusRunning)
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": node.ID.String()},
		},
	}, nil).Once()

	// Mock agent delegation failure
	mockHarness.On("DelegateToAgent", ctx, "nmap_scanner", mock.AnythingOfType("agent.Task")).
		Return(agent.Result{}, errors.New("agent connection failed")).Once()

	// Mock update execution with error
	mockExecQueries.On("UpdateExecution", ctx, mock.AnythingOfType("*schema.AgentExecution")).Return(nil).Once()

	// Mock update node status to failed
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["status"] == string(schema.WorkflowNodeStatusFailed)
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": node.ID.String()},
		},
	}, nil).Once()

	result, err := actor.Act(ctx, decision, missionID.String())

	require.NoError(t, err) // Act should not error, but result should contain error
	require.NotNil(t, result)
	assert.Equal(t, ActionExecuteAgent, result.Action)
	assert.NotNil(t, result.AgentExecution)
	assert.NotNil(t, result.Error)
	assert.Contains(t, result.Error.Error(), "agent connection failed")
	assert.False(t, result.IsTerminal)

	mockHarness.AssertExpectations(t)
	mockExecQueries.AssertExpectations(t)
	mockGraphClient.AssertExpectations(t)
}

func TestSkipAgent(t *testing.T) {
	actor, _, _, _, mockGraphClient := setupActor(t)
	ctx := context.Background()

	node := createTestWorkflowNode()
	missionID := node.MissionID

	decision := &Decision{
		Reasoning:    "Target is not reachable, skipping scan",
		Action:       ActionSkipAgent,
		TargetNodeID: node.ID.String(),
		Confidence:   0.85,
	}

	// Mock get workflow node
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["node_id"] == node.ID.String()
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{
				"n": map[string]interface{}{
					"id":          node.ID.String(),
					"mission_id":  node.MissionID.String(),
					"type":        string(node.Type),
					"name":        node.Name,
					"description": node.Description,
					"agent_name":  node.AgentName,
					"status":      string(node.Status),
					"is_dynamic":  node.IsDynamic,
					"task_config": `{}`,
					"timeout":     int64(0),
				},
			},
		},
	}, nil).Once()

	// Mock update node status to skipped
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["node_id"] == node.ID.String() && params["status"] == string(schema.WorkflowNodeStatusSkipped)
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": node.ID.String()},
		},
	}, nil).Once()

	result, err := actor.Act(ctx, decision, missionID.String())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ActionSkipAgent, result.Action)
	assert.Nil(t, result.Error)
	assert.False(t, result.IsTerminal)
	assert.Equal(t, node.ID.String(), result.TargetNodeID)
	assert.Equal(t, decision.Reasoning, result.Metadata["reasoning"])

	mockGraphClient.AssertExpectations(t)
}

func TestSpawnAgent(t *testing.T) {
	actor, _, _, _, mockGraphClient := setupActor(t)
	ctx := context.Background()

	missionID := types.NewID()

	decision := &Decision{
		Reasoning:  "Need to perform vulnerability scan based on findings",
		Action:     ActionSpawnAgent,
		Confidence: 0.90,
		SpawnConfig: &SpawnNodeConfig{
			AgentName:   "vuln_scanner",
			Description: "Scan for known vulnerabilities",
			TaskConfig: map[string]interface{}{
				"target": "192.168.1.1",
				"cve_db": "nvd",
			},
			DependsOn: []string{},
		},
	}

	// Mock create workflow node
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["name"] == "vuln_scanner" && params["is_dynamic"] == true
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": "new-node-id"},
		},
	}, nil).Once()

	// Mock link node to mission
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		_, hasMission := params["mission_id"]
		_, hasNode := params["node_id"]
		return hasMission && hasNode
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"count": int64(1)},
		},
	}, nil).Once()

	result, err := actor.Act(ctx, decision, missionID.String())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ActionSpawnAgent, result.Action)
	assert.NotNil(t, result.NewNode)
	assert.Equal(t, "vuln_scanner", result.NewNode.AgentName)
	assert.True(t, result.NewNode.IsDynamic)
	assert.Nil(t, result.Error)
	assert.False(t, result.IsTerminal)

	mockGraphClient.AssertExpectations(t)
}

func TestCompleteMission(t *testing.T) {
	actor, _, _, _, mockGraphClient := setupActor(t)
	ctx := context.Background()

	missionID := types.NewID()

	decision := &Decision{
		Reasoning:  "All workflow nodes completed successfully",
		Action:     ActionComplete,
		StopReason: "All objectives achieved",
		Confidence: 0.99,
	}

	// Mock update mission status
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["mission_id"] == missionID.String()
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": missionID.String()},
		},
	}, nil).Once()

	result, err := actor.Act(ctx, decision, missionID.String())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ActionComplete, result.Action)
	assert.Nil(t, result.Error)
	assert.True(t, result.IsTerminal)
	assert.Equal(t, decision.StopReason, result.Metadata["stop_reason"])

	mockGraphClient.AssertExpectations(t)
}

func TestModifyParams(t *testing.T) {
	actor, mockHarness, mockExecQueries, _, mockGraphClient := setupActor(t)
	ctx := context.Background()

	node := createTestWorkflowNode()
	missionID := node.MissionID

	decision := &Decision{
		Reasoning:    "Adjust scan parameters for better coverage",
		Action:       ActionModifyParams,
		TargetNodeID: node.ID.String(),
		Confidence:   0.88,
		Modifications: map[string]interface{}{
			"ports":   "1-65535",
			"threads": 10,
		},
	}

	// Mock get workflow node
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		nodeID, ok := params["node_id"]
		return ok && nodeID == node.ID.String() && params["node_id"] != nil
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{
				"n": map[string]interface{}{
					"id":          node.ID.String(),
					"mission_id":  node.MissionID.String(),
					"type":        string(node.Type),
					"name":        node.Name,
					"description": node.Description,
					"agent_name":  node.AgentName,
					"status":      string(node.Status),
					"is_dynamic":  node.IsDynamic,
					"task_config": `{"target":"192.168.1.1","ports":"1-1000"}`,
					"timeout":     int64(0),
				},
			},
		},
	}, nil).Once()

	// Mock update node config
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		_, hasConfig := params["config"]
		return hasConfig
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": node.ID.String()},
		},
	}, nil).Once()

	// Now expect the execution flow after modification
	// Mock get previous executions
	mockExecQueries.On("GetNodeExecutions", ctx, node.ID.String()).Return([]*schema.AgentExecution{}, nil).Once()

	// Mock create agent execution
	mockExecQueries.On("CreateAgentExecution", ctx, mock.AnythingOfType("*schema.AgentExecution")).Return(nil).Once()

	// Mock update node status to running
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["status"] == string(schema.WorkflowNodeStatusRunning)
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": node.ID.String()},
		},
	}, nil).Once()

	// Mock agent delegation
	agentResult := agent.NewResult(types.NewID())
	agentResult.Status = agent.ResultStatusCompleted
	mockHarness.On("DelegateToAgent", ctx, "nmap_scanner", mock.AnythingOfType("agent.Task")).Return(agentResult, nil).Once()

	// Mock update node status to completed
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["status"] == string(schema.WorkflowNodeStatusCompleted)
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": node.ID.String()},
		},
	}, nil).Once()

	// Mock update execution
	mockExecQueries.On("UpdateExecution", ctx, mock.AnythingOfType("*schema.AgentExecution")).Return(nil).Once()

	result, err := actor.Act(ctx, decision, missionID.String())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ActionExecuteAgent, result.Action) // modify_params executes the agent
	assert.NotNil(t, result.AgentExecution)
	assert.Nil(t, result.Error)

	mockHarness.AssertExpectations(t)
	mockExecQueries.AssertExpectations(t)
	mockGraphClient.AssertExpectations(t)
}

func TestRetryAgent(t *testing.T) {
	actor, mockHarness, mockExecQueries, _, mockGraphClient := setupActor(t)
	ctx := context.Background()

	node := createTestWorkflowNode()
	node.RetryPolicy = &schema.RetryPolicy{
		MaxRetries: 3,
		Backoff:    5 * time.Second,
		Strategy:   "exponential",
	}
	missionID := node.MissionID

	decision := &Decision{
		Reasoning:    "Previous execution failed, retrying",
		Action:       ActionRetry,
		TargetNodeID: node.ID.String(),
		Confidence:   0.80,
	}

	// Mock get workflow node
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		nodeID, ok := params["node_id"]
		return ok && nodeID == node.ID.String()
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{
				"n": map[string]interface{}{
					"id":           node.ID.String(),
					"mission_id":   node.MissionID.String(),
					"type":         string(node.Type),
					"name":         node.Name,
					"description":  node.Description,
					"agent_name":   node.AgentName,
					"status":       string(schema.WorkflowNodeStatusFailed),
					"is_dynamic":   node.IsDynamic,
					"task_config":  `{"target":"192.168.1.1"}`,
					"retry_policy": `{"max_retries":3,"backoff":5000000000,"strategy":"exponential"}`,
					"timeout":      int64(0),
				},
			},
		},
	}, nil).Once()

	// Mock get previous executions (1 failed attempt)
	prevExec := schema.NewAgentExecution(node.ID.String(), missionID)
	prevExec.MarkFailed("connection timeout")
	mockExecQueries.On("GetNodeExecutions", ctx, node.ID.String()).Return([]*schema.AgentExecution{prevExec}, nil).Once()

	// Mock update node status to ready
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["status"] == string(schema.WorkflowNodeStatusReady)
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": node.ID.String()},
		},
	}, nil).Once()

	// Mock create new agent execution
	mockExecQueries.On("CreateAgentExecution", ctx, mock.AnythingOfType("*schema.AgentExecution")).Return(nil).Once()

	// Mock update node status to running
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["status"] == string(schema.WorkflowNodeStatusRunning)
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": node.ID.String()},
		},
	}, nil).Once()

	// Mock successful agent delegation this time
	agentResult := agent.NewResult(types.NewID())
	agentResult.Status = agent.ResultStatusCompleted
	mockHarness.On("DelegateToAgent", ctx, "nmap_scanner", mock.AnythingOfType("agent.Task")).Return(agentResult, nil).Once()

	// Mock update node status to completed
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		return params["status"] == string(schema.WorkflowNodeStatusCompleted)
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{"id": node.ID.String()},
		},
	}, nil).Once()

	// Mock update execution
	mockExecQueries.On("UpdateExecution", ctx, mock.AnythingOfType("*schema.AgentExecution")).Return(nil).Once()

	result, err := actor.Act(ctx, decision, missionID.String())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ActionExecuteAgent, result.Action) // retry executes the agent
	assert.NotNil(t, result.AgentExecution)
	assert.Equal(t, 2, result.AgentExecution.Attempt) // Second attempt

	mockHarness.AssertExpectations(t)
	mockExecQueries.AssertExpectations(t)
	mockGraphClient.AssertExpectations(t)
}

func TestRetryAgent_MaxRetriesExceeded(t *testing.T) {
	actor, _, mockExecQueries, _, mockGraphClient := setupActor(t)
	ctx := context.Background()

	node := createTestWorkflowNode()
	node.RetryPolicy = &schema.RetryPolicy{
		MaxRetries: 2,
		Backoff:    5 * time.Second,
	}
	missionID := node.MissionID

	decision := &Decision{
		Reasoning:    "Retry after failure",
		Action:       ActionRetry,
		TargetNodeID: node.ID.String(),
		Confidence:   0.70,
	}

	// Mock get workflow node
	mockGraphClient.On("Query", ctx, mock.AnythingOfType("string"), mock.MatchedBy(func(params map[string]interface{}) bool {
		nodeID, ok := params["node_id"]
		return ok && nodeID == node.ID.String()
	})).Return(&graph.QueryResult{
		Records: []map[string]interface{}{
			{
				"n": map[string]interface{}{
					"id":           node.ID.String(),
					"mission_id":   node.MissionID.String(),
					"type":         string(node.Type),
					"name":         node.Name,
					"description":  node.Description,
					"agent_name":   node.AgentName,
					"status":       string(schema.WorkflowNodeStatusFailed),
					"is_dynamic":   node.IsDynamic,
					"task_config":  `{}`,
					"retry_policy": `{"max_retries":2,"backoff":5000000000}`,
					"timeout":      int64(0),
				},
			},
		},
	}, nil).Once()

	// Mock get previous executions (already 2 attempts = max reached)
	prevExec1 := schema.NewAgentExecution(node.ID.String(), missionID)
	prevExec1.WithAttempt(1).MarkFailed("error 1")
	prevExec2 := schema.NewAgentExecution(node.ID.String(), missionID)
	prevExec2.WithAttempt(2).MarkFailed("error 2")

	mockExecQueries.On("GetNodeExecutions", ctx, node.ID.String()).
		Return([]*schema.AgentExecution{prevExec1, prevExec2}, nil).Once()

	result, err := actor.Act(ctx, decision, missionID.String())

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "max retries")

	mockExecQueries.AssertExpectations(t)
	mockGraphClient.AssertExpectations(t)
}
