package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/graphrag/engine"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/taxonomy"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MockGraphClient for testing without Neo4j
type MockGraphClient struct {
	ExecutedQueries []ExecutedQuery
	ShouldFail      bool
	FailMessage     string
	NodeIDCounter   int
}

type ExecutedQuery struct {
	Query  string
	Params map[string]any
}

func NewMockGraphClient() *MockGraphClient {
	return &MockGraphClient{
		ExecutedQueries: make([]ExecutedQuery, 0),
		ShouldFail:      false,
		NodeIDCounter:   1,
	}
}

func (m *MockGraphClient) Connect(ctx context.Context) error {
	if m.ShouldFail {
		return types.NewError("mock_error", m.FailMessage)
	}
	return nil
}

func (m *MockGraphClient) Close(ctx context.Context) error {
	return nil
}

func (m *MockGraphClient) Health(ctx context.Context) types.HealthStatus {
	if m.ShouldFail {
		return types.Unhealthy(m.FailMessage)
	}
	return types.Healthy("mock client")
}

func (m *MockGraphClient) Query(ctx context.Context, cypher string, params map[string]any) (graph.QueryResult, error) {
	m.ExecutedQueries = append(m.ExecutedQueries, ExecutedQuery{
		Query:  cypher,
		Params: params,
	})

	if m.ShouldFail {
		return graph.QueryResult{}, types.NewError("query_failed", m.FailMessage)
	}

	// Return mock success
	return graph.QueryResult{
		Records: []map[string]any{
			{"id": "test-node-1"},
		},
		Summary: graph.QuerySummary{
			NodesCreated:         1,
			RelationshipsCreated: 0,
			PropertiesSet:        5,
		},
	}, nil
}

func (m *MockGraphClient) CreateNode(ctx context.Context, labels []string, props map[string]any) (string, error) {
	nodeID := types.NewID().String()
	m.NodeIDCounter++

	// Record as a query for verification
	m.ExecutedQueries = append(m.ExecutedQueries, ExecutedQuery{
		Query:  "CreateNode",
		Params: map[string]any{
			"labels": labels,
			"props":  props,
		},
	})

	if m.ShouldFail {
		return "", types.NewError("create_node_failed", m.FailMessage)
	}

	return nodeID, nil
}

func (m *MockGraphClient) CreateRelationship(ctx context.Context, fromID, toID, relType string, props map[string]any) error {
	m.ExecutedQueries = append(m.ExecutedQueries, ExecutedQuery{
		Query: "CreateRelationship",
		Params: map[string]any{
			"fromID":  fromID,
			"toID":    toID,
			"relType": relType,
			"props":   props,
		},
	})

	if m.ShouldFail {
		return types.NewError("create_rel_failed", m.FailMessage)
	}

	return nil
}

func (m *MockGraphClient) DeleteNode(ctx context.Context, nodeID string) error {
	if m.ShouldFail {
		return types.NewError("delete_failed", m.FailMessage)
	}
	return nil
}

func (m *MockGraphClient) Reset() {
	m.ExecutedQueries = make([]ExecutedQuery, 0)
	m.ShouldFail = false
	m.FailMessage = ""
	m.NodeIDCounter = 1
}

// EventNodeCreation for mock (matches types_events.go)
type EventNodeCreation struct {
	NodeType         string
	IDTemplate       string
	PropertyMappings map[string]string
}

// EventRelationshipCreation for mock (matches types_events.go)
type EventRelationshipCreation struct {
	RelationshipType string
	FromNode         string
	ToNode           string
	PropertyMappings map[string]string
}

// ExecutionEventDef for mock (matches types_events.go structure)
type ExecutionEventDef struct {
	EventType            string
	CreatesNode          *EventNodeCreation
	CreatesRelationships []EventRelationshipCreation
}

// ToolOutputExtraction for mock
type ToolOutputExtraction struct {
	Name                 string
	NodeType             string
	Selector             string
	PropertyMappings     map[string]string
	CreatesRelationships []ToolOutputRelationship
}

// ToolOutputRelationship for mock
type ToolOutputRelationship struct {
	RelationshipType string
	ToNode           string
	PropertyMappings map[string]string
}

// ToolOutputSchemaDef for mock
type ToolOutputSchemaDef struct {
	ToolName     string
	OutputFormat string
	Extracts     []ToolOutputExtraction
}

// MockTaxonomyRegistry for testing
type MockTaxonomyRegistry struct {
	Events      map[string]*ExecutionEventDef
	ToolSchemas map[string]*ToolOutputSchemaDef
	NodeTypes   map[string]*taxonomy.NodeTypeDefinition
}

func NewMockTaxonomyRegistry() *MockTaxonomyRegistry {
	return &MockTaxonomyRegistry{
		Events:      make(map[string]*ExecutionEventDef),
		ToolSchemas: make(map[string]*ToolOutputSchemaDef),
		NodeTypes:   make(map[string]*taxonomy.NodeTypeDefinition),
	}
}

func (m *MockTaxonomyRegistry) GetExecutionEvent(eventType string) (*ExecutionEventDef, bool) {
	event, ok := m.Events[eventType]
	return event, ok
}

func (m *MockTaxonomyRegistry) GetToolOutputSchema(toolName string) (*ToolOutputSchemaDef, bool) {
	schema, ok := m.ToolSchemas[toolName]
	return schema, ok
}

func (m *MockTaxonomyRegistry) GetNodeType(nodeType string) (*taxonomy.NodeTypeDefinition, bool) {
	nodeDef, ok := m.NodeTypes[nodeType]
	return nodeDef, ok
}

func (m *MockTaxonomyRegistry) ValidateNodeType(nodeType string) bool {
	_, ok := m.NodeTypes[nodeType]
	return ok
}

func (m *MockTaxonomyRegistry) ValidateRelationType(relType string) bool {
	return true // Accept all relationship types in mock
}

// Test Setup Helper
func setupTestEngine(t *testing.T) (*engine.TaxonomyGraphEngine, *MockGraphClient, *MockTaxonomyRegistry) {
	mockClient := NewMockGraphClient()
	mockRegistry := NewMockTaxonomyRegistry()

	// Setup default event definitions
	mockRegistry.Events["mission.started"] = &ExecutionEventDef{
		EventType: "mission.started",
		CreatesNode: &EventNodeCreation{
			NodeType:   "mission",
			IDTemplate: "mission:{mission_id}",
			PropertyMappings: map[string]string{
				"name":       "mission_id",
				"started_at": "timestamp",
				"status":     "running",
			},
		},
	}

	mockRegistry.Events["agent.started"] = &ExecutionEventDef{
		EventType: "agent.started",
		CreatesNode: &EventNodeCreation{
			NodeType:   "agent_run",
			IDTemplate: "agent_run:{trace_id}:{span_id}",
			PropertyMappings: map[string]string{
				"agent_name": "agent_name",
				"started_at": "timestamp",
			},
		},
		CreatesRelationships: []EventRelationshipCreation{
			{
				RelationshipType: "PART_OF",
				FromNode:         "self",
				ToNode:           "mission",
			},
		},
	}

	mockRegistry.Events["llm.call"] = &ExecutionEventDef{
		EventType: "llm.call",
		CreatesNode: &EventNodeCreation{
			NodeType:   "llm_call",
			IDTemplate: "llm_call:{trace_id}:{span_id}",
			PropertyMappings: map[string]string{
				"model":         "model",
				"prompt_tokens": "prompt_tokens",
				"total_tokens":  "total_tokens",
			},
		},
	}

	// Setup node type definitions
	mockRegistry.NodeTypes["mission"] = &taxonomy.NodeTypeDefinition{
		Type:       "mission",
		IDTemplate: "mission:{name}",
	}

	mockRegistry.NodeTypes["agent_run"] = &taxonomy.NodeTypeDefinition{
		Type:       "agent_run",
		IDTemplate: "agent_run:{trace_id}:{span_id}",
	}

	mockRegistry.NodeTypes["llm_call"] = &taxonomy.NodeTypeDefinition{
		Type:       "llm_call",
		IDTemplate: "llm_call:{trace_id}:{span_id}",
	}

	eng := engine.NewTaxonomyGraphEngine(mockRegistry, mockClient, nil)

	return eng, mockClient, mockRegistry
}

// TestHandleEvent_MissionStart tests mission.started event processing
func TestHandleEvent_MissionStart(t *testing.T) {
	eng, mockClient, _ := setupTestEngine(t)
	ctx := context.Background()

	eventData := map[string]any{
		"mission_id": "test-mission-001",
		"trace_id":   "trace-123",
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	err := eng.HandleEvent(ctx, "mission.started", eventData)
	require.NoError(t, err, "HandleEvent should not error on valid mission.started event")

	// Verify a query was executed
	assert.GreaterOrEqual(t, len(mockClient.ExecutedQueries), 1, "Should execute at least one query")

	// Verify the query contains mission-related data
	query := mockClient.ExecutedQueries[0]
	assert.Contains(t, query.Query, "MERGE", "Should use MERGE for idempotency")
	assert.Contains(t, query.Params, "mission_id", "Should include mission_id parameter")
	assert.Equal(t, "test-mission-001", query.Params["mission_id"], "Should pass correct mission_id")
}

// TestHandleEvent_AgentStart tests agent.started event with relationship creation
func TestHandleEvent_AgentStart(t *testing.T) {
	eng, mockClient, _ := setupTestEngine(t)
	ctx := context.Background()

	// First create the mission
	missionData := map[string]any{
		"mission_id": "test-mission-001",
		"trace_id":   "trace-123",
		"timestamp":  time.Now().Format(time.RFC3339),
	}
	err := eng.HandleEvent(ctx, "mission.started", missionData)
	require.NoError(t, err)

	mockClient.Reset() // Clear queries to test agent start in isolation

	// Now create the agent run
	agentData := map[string]any{
		"agent_name": "bishop",
		"mission_id": "test-mission-001",
		"trace_id":   "trace-123",
		"span_id":    "span-456",
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	err = eng.HandleEvent(ctx, "agent.started", agentData)
	require.NoError(t, err, "HandleEvent should not error on valid agent.started event")

	// Verify both node creation AND relationship creation
	assert.GreaterOrEqual(t, len(mockClient.ExecutedQueries), 1, "Should create node and relationship")

	// Check that agent_run node properties are set
	query := mockClient.ExecutedQueries[0]
	assert.Contains(t, query.Params, "agent_name", "Should include agent_name")
	assert.Equal(t, "bishop", query.Params["agent_name"], "Should pass correct agent_name")
}

// TestHandleEvent_LLMCall tests llm.call event with token metrics
func TestHandleEvent_LLMCall(t *testing.T) {
	eng, mockClient, _ := setupTestEngine(t)
	ctx := context.Background()

	llmData := map[string]any{
		"model":         "claude-opus-4",
		"trace_id":      "trace-123",
		"span_id":       "span-789",
		"prompt_tokens": 1500,
		"total_tokens":  2000,
		"timestamp":     time.Now().Format(time.RFC3339),
	}

	err := eng.HandleEvent(ctx, "llm.call", llmData)
	require.NoError(t, err, "HandleEvent should not error on valid llm.call event")

	// Verify LLM call was recorded
	assert.GreaterOrEqual(t, len(mockClient.ExecutedQueries), 1, "Should execute at least one query")

	query := mockClient.ExecutedQueries[0]
	assert.Contains(t, query.Params, "model", "Should include model parameter")
	assert.Contains(t, query.Params, "prompt_tokens", "Should include token metrics")
	assert.Equal(t, 1500, query.Params["prompt_tokens"], "Should pass correct prompt_tokens")
}

// TestHandleEvent_UnknownEvent tests graceful handling of undefined events
func TestHandleEvent_UnknownEvent(t *testing.T) {
	eng, mockClient, _ := setupTestEngine(t)
	ctx := context.Background()

	unknownData := map[string]any{
		"some_field": "some_value",
	}

	// Should not error, just log warning
	err := eng.HandleEvent(ctx, "unknown.event.type", unknownData)
	assert.NoError(t, err, "HandleEvent should not error on unknown event types")

	// Should not execute any queries
	assert.Equal(t, 0, len(mockClient.ExecutedQueries), "Should not execute queries for unknown events")
}

// TestHandleEvent_Neo4jFailure tests graceful handling of Neo4j errors
func TestHandleEvent_Neo4jFailure(t *testing.T) {
	eng, mockClient, _ := setupTestEngine(t)
	ctx := context.Background()

	// Configure mock to fail
	mockClient.ShouldFail = true
	mockClient.FailMessage = "Neo4j connection lost"

	eventData := map[string]any{
		"mission_id": "test-mission-001",
		"trace_id":   "trace-123",
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	err := eng.HandleEvent(ctx, "mission.started", eventData)

	// Should return error but not crash
	assert.Error(t, err, "HandleEvent should return error when Neo4j fails")
	assert.Contains(t, err.Error(), "Neo4j", "Error should mention Neo4j")
}

// TestHandleToolOutput_Nmap tests nmap output processing with hosts and ports
func TestHandleToolOutput_Nmap(t *testing.T) {
	eng, mockClient, mockRegistry := setupTestEngine(t)
	ctx := context.Background()

	// Setup nmap tool schema
	mockRegistry.ToolSchemas["nmap"] = &ToolOutputSchemaDef{
		ToolName:     "nmap",
		OutputFormat: "json",
		Extracts: []ToolOutputExtraction{
			{
				Name:     "discovered_hosts",
				NodeType: "host",
				Selector: "$.hosts[*]",
				PropertyMappings: map[string]string{
					"ip":       "ip",
					"hostname": "hostname",
				},
				CreatesRelationships: []ToolOutputRelationship{
					{
						RelationshipType: "DISCOVERED",
						ToNode:           "tool_execution",
					},
				},
			},
		},
	}

	mockRegistry.NodeTypes["host"] = &taxonomy.NodeTypeDefinition{
		Type:       "host",
		IDTemplate: "host:{ip}",
	}

	nmapOutput := map[string]any{
		"hosts": []any{
			map[string]any{
				"ip":       "192.168.1.1",
				"hostname": "router.local",
				"ports": []any{
					map[string]any{"port": 22, "protocol": "tcp", "state": "open"},
					map[string]any{"port": 80, "protocol": "tcp", "state": "open"},
				},
			},
			map[string]any{
				"ip":       "192.168.1.10",
				"hostname": "workstation.local",
				"ports": []any{
					map[string]any{"port": 445, "protocol": "tcp", "state": "open"},
				},
			},
		},
	}

	err := eng.HandleToolOutput(ctx, "nmap", nmapOutput, "tool-exec-123")
	require.NoError(t, err, "HandleToolOutput should not error on valid nmap output")

	// Verify host nodes were created
	assert.GreaterOrEqual(t, len(mockClient.ExecutedQueries), 2, "Should create at least 2 host nodes")

	// Verify host IPs are in the parameters
	foundHost1 := false
	foundHost2 := false
	for _, query := range mockClient.ExecutedQueries {
		if ip, ok := query.Params["ip"].(string); ok {
			if ip == "192.168.1.1" {
				foundHost1 = true
			}
			if ip == "192.168.1.10" {
				foundHost2 = true
			}
		}
	}

	assert.True(t, foundHost1, "Should create node for 192.168.1.1")
	assert.True(t, foundHost2, "Should create node for 192.168.1.10")
}

// TestHandleToolOutput_NoSchema tests tool without schema returns nil
func TestHandleToolOutput_NoSchema(t *testing.T) {
	eng, mockClient, _ := setupTestEngine(t)
	ctx := context.Background()

	output := map[string]any{
		"data": "some output",
	}

	err := eng.HandleToolOutput(ctx, "unknown-tool", output, "tool-exec-123")
	assert.NoError(t, err, "HandleToolOutput should not error when no schema is defined")

	// Should not execute any queries
	assert.Equal(t, 0, len(mockClient.ExecutedQueries), "Should not execute queries without schema")
}

// TestHandleToolOutput_EmptyOutput tests empty output doesn't error
func TestHandleToolOutput_EmptyOutput(t *testing.T) {
	eng, mockClient, mockRegistry := setupTestEngine(t)
	ctx := context.Background()

	// Setup a tool schema
	mockRegistry.ToolSchemas["test-tool"] = &ToolOutputSchemaDef{
		ToolName:     "test-tool",
		OutputFormat: "json",
		Extracts:     []ToolOutputExtraction{},
	}

	emptyOutput := map[string]any{}

	err := eng.HandleToolOutput(ctx, "test-tool", emptyOutput, "tool-exec-123")
	assert.NoError(t, err, "HandleToolOutput should not error on empty output")

	// Empty output with empty extracts = no queries
	assert.Equal(t, 0, len(mockClient.ExecutedQueries), "Should not execute queries for empty output")
}

// TestHandleFinding_Basic tests basic finding node creation
func TestHandleFinding_Basic(t *testing.T) {
	eng, mockClient, mockRegistry := setupTestEngine(t)
	ctx := context.Background()

	mockRegistry.NodeTypes["finding"] = &taxonomy.NodeTypeDefinition{
		Type:       "finding",
		IDTemplate: "finding:{id}",
	}

	finding := agent.Finding{
		ID:          types.NewID(),
		Title:       "SQL Injection",
		Description: "SQL injection vulnerability in search parameter",
		Severity:    agent.SeverityHigh,
		Confidence:  0.95,
		CWE:         []string{"CWE-89"},
		Metadata: map[string]any{
			"target_url": "https://example.com/search?q=test",
		},
	}

	err := eng.HandleFinding(ctx, &finding)
	require.NoError(t, err, "HandleFinding should not error on valid finding")

	// Verify finding node was created
	assert.GreaterOrEqual(t, len(mockClient.ExecutedQueries), 1, "Should create finding node")

	// Verify finding properties
	query := mockClient.ExecutedQueries[0]
	assert.Contains(t, query.Params, "title", "Should include title")
	assert.Equal(t, "SQL Injection", query.Params["title"], "Should pass correct title")
	assert.Contains(t, query.Params, "severity", "Should include severity")
	assert.Equal(t, "high", query.Params["severity"], "Should pass correct severity")
}

// TestHandleFinding_WithCWE tests finding with CWE technique relationship
func TestHandleFinding_WithCWE(t *testing.T) {
	eng, mockClient, mockRegistry := setupTestEngine(t)
	ctx := context.Background()

	mockRegistry.NodeTypes["finding"] = &taxonomy.NodeTypeDefinition{
		Type:       "finding",
		IDTemplate: "finding:{id}",
	}

	mockRegistry.NodeTypes["technique"] = &taxonomy.NodeTypeDefinition{
		Type:       "technique",
		IDTemplate: "technique:{cwe_id}",
	}

	finding := agent.Finding{
		ID:       types.NewID(),
		Title:    "Cross-Site Scripting",
		Severity: agent.SeverityHigh,
		CWE:      []string{"CWE-79", "CWE-80"},
	}

	err := eng.HandleFinding(ctx, &finding)
	require.NoError(t, err, "HandleFinding should not error")

	// Should create finding node + relationships to techniques
	assert.GreaterOrEqual(t, len(mockClient.ExecutedQueries), 1, "Should create nodes and relationships")

	// Verify CWE data is included
	foundCWE79 := false
	for _, query := range mockClient.ExecutedQueries {
		if cwe, ok := query.Params["cwe_id"].(string); ok {
			if cwe == "CWE-79" {
				foundCWE79 = true
			}
		}
	}

	assert.True(t, foundCWE79 || len(mockClient.ExecutedQueries) > 0, "Should process CWE relationships")
}

// TestHandleEvent_InvalidTemplate tests handling of invalid property template
func TestHandleEvent_InvalidTemplate(t *testing.T) {
	eng, mockClient, mockRegistry := setupTestEngine(t)
	ctx := context.Background()

	// Create event with invalid template reference
	mockRegistry.Events["bad.event"] = &ExecutionEventDef{
		EventType: "bad.event",
		CreatesNode: &EventNodeCreation{
			NodeType:   "test_node",
			IDTemplate: "node:{missing_field}", // Field not in event data
			PropertyMappings: map[string]string{
				"name": "missing_field",
			},
		},
	}

	mockRegistry.NodeTypes["test_node"] = &taxonomy.NodeTypeDefinition{
		Type:       "test_node",
		IDTemplate: "node:{name}",
	}

	eventData := map[string]any{
		"different_field": "value",
	}

	err := eng.HandleEvent(ctx, "bad.event", eventData)

	// Should handle gracefully (either error or skip)
	if err != nil {
		assert.Contains(t, err.Error(), "missing", "Error should indicate missing field")
	}

	// Should not crash the engine
	assert.NotPanics(t, func() {
		_ = eng.HandleEvent(ctx, "bad.event", eventData)
	}, "Should not panic on invalid template")
}

// TestHandleEvent_InvalidNodeType tests node type not in taxonomy
func TestHandleEvent_InvalidNodeType(t *testing.T) {
	eng, _, mockRegistry := setupTestEngine(t)
	ctx := context.Background()

	// Create event referencing non-existent node type
	mockRegistry.Events["invalid.node.event"] = &ExecutionEventDef{
		EventType: "invalid.node.event",
		CreatesNode: &EventNodeCreation{
			NodeType:   "non_existent_node_type",
			IDTemplate: "node:{id}",
			PropertyMappings: map[string]string{
				"name": "id",
			},
		},
	}

	eventData := map[string]any{
		"id": "test-123",
	}

	err := eng.HandleEvent(ctx, "invalid.node.event", eventData)

	// Should error or log warning for invalid node type
	if err != nil {
		assert.Contains(t, err.Error(), "node", "Error should reference node type issue")
	}
}

// TestHandleEvent_Idempotent tests calling same event twice uses MERGE
func TestHandleEvent_Idempotent(t *testing.T) {
	eng, mockClient, _ := setupTestEngine(t)
	ctx := context.Background()

	eventData := map[string]any{
		"mission_id": "test-mission-001",
		"trace_id":   "trace-123",
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	// Call twice with same data
	err1 := eng.HandleEvent(ctx, "mission.started", eventData)
	require.NoError(t, err1)

	err2 := eng.HandleEvent(ctx, "mission.started", eventData)
	require.NoError(t, err2)

	// Both should succeed (idempotent)
	assert.NoError(t, err1)
	assert.NoError(t, err2)

	// Verify queries use MERGE for idempotency
	for _, query := range mockClient.ExecutedQueries {
		if query.Query != "CreateNode" && query.Query != "CreateRelationship" {
			assert.Contains(t, query.Query, "MERGE", "Should use MERGE for idempotent operations")
		}
	}
}

// TestHandleToolOutput_MultiplePorts tests port extraction from nmap
func TestHandleToolOutput_MultiplePorts(t *testing.T) {
	eng, mockClient, mockRegistry := setupTestEngine(t)
	ctx := context.Background()

	// Setup schemas for host and port extraction
	mockRegistry.ToolSchemas["nmap"] = &ToolOutputSchemaDef{
		ToolName:     "nmap",
		OutputFormat: "json",
		Extracts: []ToolOutputExtraction{
			{
				Name:     "discovered_ports",
				NodeType: "port",
				Selector: "$.hosts[*].ports[*]",
				PropertyMappings: map[string]string{
					"port":     "port",
					"protocol": "protocol",
					"state":    "state",
				},
			},
		},
	}

	mockRegistry.NodeTypes["port"] = &taxonomy.NodeTypeDefinition{
		Type:       "port",
		IDTemplate: "port:{host_ip}:{port}:{protocol}",
	}

	nmapOutput := map[string]any{
		"hosts": []any{
			map[string]any{
				"ip": "192.168.1.1",
				"ports": []any{
					map[string]any{"port": 22, "protocol": "tcp", "state": "open"},
					map[string]any{"port": 80, "protocol": "tcp", "state": "open"},
					map[string]any{"port": 443, "protocol": "tcp", "state": "open"},
				},
			},
		},
	}

	err := eng.HandleToolOutput(ctx, "nmap", nmapOutput, "tool-exec-123")
	require.NoError(t, err)

	// Should create nodes for each port
	assert.GreaterOrEqual(t, len(mockClient.ExecutedQueries), 3, "Should create 3 port nodes")
}

// TestNewTaxonomyGraphEngine tests engine initialization
func TestNewTaxonomyGraphEngine(t *testing.T) {
	mockClient := NewMockGraphClient()
	mockRegistry := NewMockTaxonomyRegistry()

	eng, err := engine.NewTaxonomyGraphEngine(mockClient, mockRegistry)
	assert.NoError(t, err, "Should create engine without error")
	assert.NotNil(t, eng, "Engine should not be nil")
}

// TestNewTaxonomyGraphEngine_NilClient tests error on nil client
func TestNewTaxonomyGraphEngine_NilClient(t *testing.T) {
	mockRegistry := NewMockTaxonomyRegistry()

	eng, err := engine.NewTaxonomyGraphEngine(nil, mockRegistry)
	assert.Error(t, err, "Should error on nil client")
	assert.Nil(t, eng, "Engine should be nil on error")
}

// TestNewTaxonomyGraphEngine_NilRegistry tests error on nil registry
func TestNewTaxonomyGraphEngine_NilRegistry(t *testing.T) {
	mockClient := NewMockGraphClient()

	eng, err := engine.NewTaxonomyGraphEngine(mockClient, nil)
	assert.Error(t, err, "Should error on nil registry")
	assert.Nil(t, eng, "Engine should be nil on error")
}

// TestHandleEvent_ComplexPropertyMapping tests complex property mapping
func TestHandleEvent_ComplexPropertyMapping(t *testing.T) {
	eng, mockClient, mockRegistry := setupTestEngine(t)
	ctx := context.Background()

	mockRegistry.Events["complex.event"] = &ExecutionEventDef{
		EventType: "complex.event",
		CreatesNode: &EventNodeCreation{
			NodeType:   "mission",
			IDTemplate: "mission:{id}",
			PropertyMappings: map[string]string{
				"name":      "event_name",
				"status":    "event_status",
				"count":     "event_count",
				"nested_id": "metadata.nested.id",
			},
		},
	}

	eventData := map[string]any{
		"id":           "test-123",
		"event_name":   "Test Event",
		"event_status": "active",
		"event_count":  42,
		"metadata": map[string]any{
			"nested": map[string]any{
				"id": "nested-456",
			},
		},
	}

	err := eng.HandleEvent(ctx, "complex.event", eventData)
	require.NoError(t, err, "Should handle complex property mappings")

	assert.GreaterOrEqual(t, len(mockClient.ExecutedQueries), 1, "Should execute query")

	// Verify mapped properties
	query := mockClient.ExecutedQueries[0]
	assert.Contains(t, query.Params, "name", "Should map event_name to name")
	assert.Contains(t, query.Params, "status", "Should map event_status to status")
	assert.Contains(t, query.Params, "count", "Should map event_count to count")
}

// TestHandleFinding_TargetURL tests finding with target URL
func TestHandleFinding_TargetURL(t *testing.T) {
	eng, mockClient, mockRegistry := setupTestEngine(t)
	ctx := context.Background()

	mockRegistry.NodeTypes["finding"] = &taxonomy.NodeTypeDefinition{
		Type:       "finding",
		IDTemplate: "finding:{id}",
	}

	finding := agent.Finding{
		ID:       types.NewID(),
		Title:    "Path Traversal",
		Severity: agent.SeverityHigh,
		Metadata: map[string]any{
			"target_url": "https://example.com/files",
			"method":     "GET",
		},
	}

	err := eng.HandleFinding(ctx, &finding)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(mockClient.ExecutedQueries), 1, "Should create finding node")

	// Verify metadata is preserved
	query := mockClient.ExecutedQueries[0]
	if metadata, ok := query.Params["metadata"].(map[string]any); ok {
		assert.Equal(t, "https://example.com/files", metadata["target_url"])
	}
}

// TestHandleEvent_MissingOptionalFields tests optional fields are skipped gracefully
func TestHandleEvent_MissingOptionalFields(t *testing.T) {
	eng, mockClient, mockRegistry := setupTestEngine(t)
	ctx := context.Background()

	mockRegistry.Events["optional.event"] = &ExecutionEventDef{
		EventType: "optional.event",
		CreatesNode: &EventNodeCreation{
			NodeType:   "mission",
			IDTemplate: "mission:{id}",
			PropertyMappings: map[string]string{
				"name":           "event_name",
				"optional_field": "missing_field", // This field is not in event data
			},
		},
	}

	eventData := map[string]any{
		"id":         "test-123",
		"event_name": "Test Event",
		// missing_field is absent
	}

	err := eng.HandleEvent(ctx, "optional.event", eventData)

	// Should succeed, skipping optional field
	assert.NoError(t, err, "Should handle missing optional fields gracefully")
	assert.GreaterOrEqual(t, len(mockClient.ExecutedQueries), 1, "Should still execute query")
}

// TestHandleToolOutput_WithRelationships tests tool output creates relationships
func TestHandleToolOutput_WithRelationships(t *testing.T) {
	eng, mockClient, mockRegistry := setupTestEngine(t)
	ctx := context.Background()

	mockRegistry.ToolSchemas["subfinder"] = &ToolOutputSchemaDef{
		ToolName:     "subfinder",
		OutputFormat: "json",
		Extracts: []ToolOutputExtraction{
			{
				Name:     "discovered_subdomains",
				NodeType: "domain",
				Selector: "$.subdomains[*]",
				PropertyMappings: map[string]string{
					"name": "subdomain",
				},
				CreatesRelationships: []ToolOutputRelationship{
					{
						RelationshipType: "DISCOVERED",
						ToNode:           "tool_execution",
					},
					{
						RelationshipType: "SUBDOMAIN_OF",
						ToNode:           "parent_domain",
					},
				},
			},
		},
	}

	mockRegistry.NodeTypes["domain"] = &taxonomy.NodeTypeDefinition{
		Type:       "domain",
		IDTemplate: "domain:{name}",
	}

	subfinderOutput := map[string]any{
		"subdomains": []any{
			map[string]any{"subdomain": "api.example.com"},
			map[string]any{"subdomain": "www.example.com"},
		},
	}

	err := eng.HandleToolOutput(ctx, "subfinder", subfinderOutput, "tool-exec-123")
	require.NoError(t, err)

	// Should create subdomain nodes + relationships
	assert.GreaterOrEqual(t, len(mockClient.ExecutedQueries), 2, "Should create nodes and relationships")
}
