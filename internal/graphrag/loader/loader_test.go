package loader

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/sdk/graphrag/domain"
)

// mockGraphNode is a test implementation of domain.GraphNode
type mockGraphNode struct {
	nodeType  string
	idProps   map[string]any
	props     map[string]any
	parentRef *domain.NodeRef
	relType   string
}

func (m *mockGraphNode) NodeType() string {
	return m.nodeType
}

func (m *mockGraphNode) IdentifyingProperties() map[string]any {
	return m.idProps
}

func (m *mockGraphNode) Properties() map[string]any {
	return m.props
}

func (m *mockGraphNode) ParentRef() *domain.NodeRef {
	return m.parentRef
}

func (m *mockGraphNode) RelationshipType() string {
	return m.relType
}

func TestNewGraphLoader(t *testing.T) {
	client := graph.NewMockGraphClient()
	loader := NewGraphLoader(client)

	assert.NotNil(t, loader)
	assert.Equal(t, client, loader.client)
}

func TestLoadResult_AddError(t *testing.T) {
	result := &LoadResult{}

	assert.False(t, result.HasErrors())
	assert.Empty(t, result.Errors)

	result.AddError(types.NewError("TEST", "test error"))

	assert.True(t, result.HasErrors())
	assert.Len(t, result.Errors, 1)
}

func TestLoad_NilClient(t *testing.T) {
	loader := &GraphLoader{client: nil}
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: "test-run"}

	_, err := loader.Load(ctx, execCtx, []domain.GraphNode{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client is nil")
}

func TestLoad_EmptyNodes(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: "test-run"}

	result, err := loader.Load(ctx, execCtx, []domain.GraphNode{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.NodesCreated)
	assert.Equal(t, 0, result.NodesUpdated)
	assert.Equal(t, 0, result.RelationshipsCreated)
}

func TestLoad_NilNode(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: "test-run"}

	result, err := loader.Load(ctx, execCtx, []domain.GraphNode{nil})

	assert.NoError(t, err)
	assert.True(t, result.HasErrors())
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "nil node")
}

func TestLoad_EmptyNodeType(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: "test-run"}

	node := &mockGraphNode{
		nodeType: "",
		idProps:  map[string]any{"id": "test"},
	}

	result, err := loader.Load(ctx, execCtx, []domain.GraphNode{node})

	assert.NoError(t, err)
	assert.True(t, result.HasErrors())
	assert.Contains(t, result.Errors[0].Error(), "empty NodeType")
}

func TestLoad_NoIdentifyingProperties(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: "test-run"}

	node := &mockGraphNode{
		nodeType: "test_node",
		idProps:  map[string]any{}, // Empty identifying properties
		props:    map[string]any{"key": "value"},
	}

	result, err := loader.Load(ctx, execCtx, []domain.GraphNode{node})

	assert.NoError(t, err)
	assert.True(t, result.HasErrors())
	assert.Contains(t, result.Errors[0].Error(), "no identifying properties")
}

func TestLoad_CreateNode_Success(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock to return a successful node creation
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{
				"node_id":   "mock-node-1",
				"operation": "created",
			},
		},
		Columns: []string{"node_id", "operation"},
		Summary: graph.QuerySummary{
			NodesCreated: 1,
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: ""}

	node := &mockGraphNode{
		nodeType: "host",
		idProps:  map[string]any{"ip": "192.168.1.1"},
		props: map[string]any{
			"ip":       "192.168.1.1",
			"hostname": "test-server",
		},
	}

	result, err := loader.Load(ctx, execCtx, []domain.GraphNode{node})

	assert.NoError(t, err)
	assert.Equal(t, 1, result.NodesCreated)
	assert.Equal(t, 0, result.NodesUpdated)
	assert.Equal(t, 0, result.RelationshipsCreated)
	assert.False(t, result.HasErrors())

	// Verify the MERGE query was called
	calls := client.GetCallsByMethod("Query")
	assert.Len(t, calls, 1)

	// Check that the query contains MERGE and the node type
	queryStr := calls[0].Args[0].(string)
	assert.Contains(t, queryStr, "MERGE")
	assert.Contains(t, queryStr, "host")
	assert.Contains(t, queryStr, "ip: $id_ip")
}

func TestLoad_UpdateNode_Success(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock to return a successful node update
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{
				"node_id":   "mock-node-1",
				"operation": "updated",
			},
		},
		Columns: []string{"node_id", "operation"},
		Summary: graph.QuerySummary{
			PropertiesSet: 2,
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: ""}

	node := &mockGraphNode{
		nodeType: "host",
		idProps:  map[string]any{"ip": "192.168.1.1"},
		props: map[string]any{
			"ip":       "192.168.1.1",
			"hostname": "updated-server",
			"state":    "up",
		},
	}

	result, err := loader.Load(ctx, execCtx, []domain.GraphNode{node})

	assert.NoError(t, err)
	assert.Equal(t, 0, result.NodesCreated)
	assert.Equal(t, 1, result.NodesUpdated)
	assert.False(t, result.HasErrors())
}

func TestLoad_WithParentRelationship(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock to return node creation followed by relationship creation
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{
				"node_id":   "mock-port-1",
				"operation": "created",
			},
		},
	})
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{"r": map[string]any{"type": "HAS_PORT"}},
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: ""}

	// Create a Port node with a parent Host reference
	node := &mockGraphNode{
		nodeType: "port",
		idProps: map[string]any{
			"host_id":  "192.168.1.1",
			"number":   80,
			"protocol": "tcp",
		},
		props: map[string]any{
			"host_id":  "192.168.1.1",
			"number":   80,
			"protocol": "tcp",
			"state":    "open",
		},
		parentRef: &domain.NodeRef{
			NodeType:   "host",
			Properties: map[string]any{"ip": "192.168.1.1"},
		},
		relType: "HAS_PORT",
	}

	result, err := loader.Load(ctx, execCtx, []domain.GraphNode{node})

	assert.NoError(t, err)
	assert.Equal(t, 1, result.NodesCreated)
	assert.Equal(t, 1, result.RelationshipsCreated)
	assert.False(t, result.HasErrors())

	// Verify both queries were called
	calls := client.GetCallsByMethod("Query")
	assert.Len(t, calls, 2)

	// Check the relationship query
	relQuery := calls[1].Args[0].(string)
	assert.Contains(t, relQuery, "MATCH (parent:host)")
	assert.Contains(t, relQuery, "MERGE (parent)-[r:HAS_PORT]->(child)")
}

func TestLoad_WithDiscoveredRelationship(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock to return node creation followed by DISCOVERED relationship creation
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{
				"node_id":   "mock-node-1",
				"operation": "created",
			},
		},
	})
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{"r": map[string]any{"type": "DISCOVERED"}},
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: "agent-run-123"}

	node := &mockGraphNode{
		nodeType: "host",
		idProps:  map[string]any{"ip": "192.168.1.1"},
		props:    map[string]any{"ip": "192.168.1.1"},
	}

	result, err := loader.Load(ctx, execCtx, []domain.GraphNode{node})

	assert.NoError(t, err)
	assert.Equal(t, 1, result.NodesCreated)
	assert.Equal(t, 1, result.RelationshipsCreated) // DISCOVERED relationship
	assert.False(t, result.HasErrors())

	// Verify DISCOVERED relationship query
	calls := client.GetCallsByMethod("Query")
	assert.Len(t, calls, 2)

	discQuery := calls[1].Args[0].(string)
	assert.Contains(t, discQuery, "MATCH (run {id: $agent_run_id})")
	assert.Contains(t, discQuery, "MERGE (run)-[r:DISCOVERED]->(node)")
	assert.Contains(t, discQuery, "discovered_at = timestamp()")
}

func TestLoad_ParentRefWithoutRelType(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Node creation succeeds
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{
				"node_id":   "mock-node-1",
				"operation": "created",
			},
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: ""}

	// Node has ParentRef but no RelationshipType
	node := &mockGraphNode{
		nodeType: "port",
		idProps:  map[string]any{"number": 80},
		props:    map[string]any{"number": 80},
		parentRef: &domain.NodeRef{
			NodeType:   "host",
			Properties: map[string]any{"ip": "192.168.1.1"},
		},
		relType: "", // Empty relationship type
	}

	result, err := loader.Load(ctx, execCtx, []domain.GraphNode{node})

	assert.NoError(t, err)
	assert.True(t, result.HasErrors())
	assert.Contains(t, result.Errors[0].Error(), "has ParentRef but no RelationshipType")
}

func TestLoad_QueryError(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock to return an error
	client.SetQueryError(types.NewError("QUERY_FAILED", "simulated query error"))

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: ""}

	node := &mockGraphNode{
		nodeType: "host",
		idProps:  map[string]any{"ip": "192.168.1.1"},
		props:    map[string]any{"ip": "192.168.1.1"},
	}

	result, err := loader.Load(ctx, execCtx, []domain.GraphNode{node})

	assert.NoError(t, err) // Load continues despite errors
	assert.True(t, result.HasErrors())
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "failed to merge node")
}

func TestLoad_MultipleNodes(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock to return success for each node
	for i := 0; i < 3; i++ {
		client.AddQueryResult(graph.QueryResult{
			Records: []map[string]any{
				{
					"node_id":   "mock-node",
					"operation": "created",
				},
			},
		})
	}

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: ""}

	nodes := []domain.GraphNode{
		&mockGraphNode{
			nodeType: "host",
			idProps:  map[string]any{"ip": "192.168.1.1"},
			props:    map[string]any{"ip": "192.168.1.1"},
		},
		&mockGraphNode{
			nodeType: "host",
			idProps:  map[string]any{"ip": "192.168.1.2"},
			props:    map[string]any{"ip": "192.168.1.2"},
		},
		&mockGraphNode{
			nodeType: "host",
			idProps:  map[string]any{"ip": "192.168.1.3"},
			props:    map[string]any{"ip": "192.168.1.3"},
		},
	}

	result, err := loader.Load(ctx, execCtx, nodes)

	assert.NoError(t, err)
	assert.Equal(t, 3, result.NodesCreated)
	assert.Equal(t, 0, result.NodesUpdated)
	assert.False(t, result.HasErrors())
}

func TestCreateParentRelationship_Success(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{"r": map[string]any{"type": "HAS_PORT"}},
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()

	parentRef := &domain.NodeRef{
		NodeType:   "host",
		Properties: map[string]any{"ip": "192.168.1.1"},
	}

	err = loader.createParentRelationship(ctx, "child-node-id", parentRef, "HAS_PORT")

	assert.NoError(t, err)

	// Verify query structure
	calls := client.GetCallsByMethod("Query")
	assert.Len(t, calls, 1)

	queryStr := calls[0].Args[0].(string)
	assert.Contains(t, queryStr, "MATCH (parent:host)")
	assert.Contains(t, queryStr, "parent.ip = $parent_ip")
	assert.Contains(t, queryStr, "elementId(child) = $child_id")
	assert.Contains(t, queryStr, "MERGE (parent)-[r:HAS_PORT]->(child)")
}

func TestCreateParentRelationship_NilRef(t *testing.T) {
	client := graph.NewMockGraphClient()
	loader := NewGraphLoader(client)
	ctx := context.Background()

	err := loader.createParentRelationship(ctx, "child-id", nil, "HAS_PORT")

	assert.NoError(t, err) // Should be a no-op
	assert.Equal(t, 0, client.CallCount())
}

func TestCreateDiscoveredRelationship_Success(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{"r": map[string]any{"type": "DISCOVERED"}},
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()

	err = loader.createDiscoveredRelationship(ctx, "agent-run-123", "node-id-456")

	assert.NoError(t, err)

	// Verify query structure
	calls := client.GetCallsByMethod("Query")
	assert.Len(t, calls, 1)

	queryStr := calls[0].Args[0].(string)
	params := calls[0].Args[1].(map[string]any)

	assert.Contains(t, queryStr, "MATCH (run {id: $agent_run_id})")
	assert.Contains(t, queryStr, "MERGE (run)-[r:DISCOVERED]->(node)")
	assert.Equal(t, "agent-run-123", params["agent_run_id"])
	assert.Equal(t, "node-id-456", params["node_id"])
}

func TestLoadBatch_NilClient(t *testing.T) {
	loader := &GraphLoader{client: nil}
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: "test-run"}

	_, err := loader.LoadBatch(ctx, execCtx, []domain.GraphNode{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client is nil")
}

func TestLoadBatch_EmptyNodes(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: "test-run"}

	result, err := loader.LoadBatch(ctx, execCtx, []domain.GraphNode{})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.NodesCreated)
}

func TestLoadBatch_GroupsByNodeType(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock to return batch result
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{
				"batch_result": []any{
					map[string]any{
						"element_id": "node-1",
						"operation":  "created",
						"node_type":  "host",
						"index":      float64(0),
					},
					map[string]any{
						"element_id": "node-2",
						"operation":  "created",
						"node_type":  "host",
						"index":      float64(1),
					},
				},
			},
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: ""}

	nodes := []domain.GraphNode{
		&mockGraphNode{
			nodeType: "host",
			idProps:  map[string]any{"ip": "192.168.1.1"},
			props:    map[string]any{"ip": "192.168.1.1"},
		},
		&mockGraphNode{
			nodeType: "host",
			idProps:  map[string]any{"ip": "192.168.1.2"},
			props:    map[string]any{"ip": "192.168.1.2"},
		},
	}

	result, err := loader.LoadBatch(ctx, execCtx, nodes)

	assert.NoError(t, err)
	assert.Equal(t, 2, result.NodesCreated)

	// Verify single UNWIND query was executed
	calls := client.GetCallsByMethod("Query")
	assert.Len(t, calls, 1)

	queryStr := calls[0].Args[0].(string)
	assert.Contains(t, queryStr, "UNWIND")
	assert.Contains(t, queryStr, "MERGE (n:host")
}

func TestLoadBatch_WithParentRelationships(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock for batch node creation
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{
				"batch_result": []any{
					map[string]any{
						"element_id": "port-1",
						"operation":  "created",
						"node_type":  "port",
						"index":      float64(0),
					},
				},
			},
		},
	})

	// Configure mock for batch relationship creation
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{"rel_count": int64(1)},
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: ""}

	nodes := []domain.GraphNode{
		&mockGraphNode{
			nodeType: "port",
			idProps:  map[string]any{"host_id": "192.168.1.1", "number": 80, "protocol": "tcp"},
			props:    map[string]any{"host_id": "192.168.1.1", "number": 80, "protocol": "tcp"},
			parentRef: &domain.NodeRef{
				NodeType:   "host",
				Properties: map[string]any{"ip": "192.168.1.1"},
			},
			relType: "HAS_PORT",
		},
	}

	result, err := loader.LoadBatch(ctx, execCtx, nodes)

	assert.NoError(t, err)
	assert.Equal(t, 1, result.NodesCreated)
	assert.Equal(t, 1, result.RelationshipsCreated)

	// Verify batch relationship query
	calls := client.GetCallsByMethod("Query")
	assert.Len(t, calls, 2) // Node creation + relationship creation

	relQuery := calls[1].Args[0].(string)
	assert.Contains(t, relQuery, "UNWIND $rel_data")
	assert.Contains(t, relQuery, "MATCH (parent:host)")
	assert.Contains(t, relQuery, "MERGE (parent)-[r:HAS_PORT]->(child)")
}

func TestLoadBatch_WithDiscoveredRelationships(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock for batch node creation
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{
				"batch_result": []any{
					map[string]any{
						"element_id": "node-1",
						"operation":  "created",
						"node_type":  "host",
						"index":      float64(0),
					},
				},
			},
		},
	})

	// Configure mock for DISCOVERED relationships
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{"rel_count": int64(1)},
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: "agent-run-123"}

	nodes := []domain.GraphNode{
		&mockGraphNode{
			nodeType: "host",
			idProps:  map[string]any{"ip": "192.168.1.1"},
			props:    map[string]any{"ip": "192.168.1.1"},
		},
	}

	result, err := loader.LoadBatch(ctx, execCtx, nodes)

	assert.NoError(t, err)
	assert.Equal(t, 1, result.NodesCreated)
	assert.Equal(t, 1, result.RelationshipsCreated)

	// Verify DISCOVERED relationship batch query
	calls := client.GetCallsByMethod("Query")
	assert.Len(t, calls, 2)

	discQuery := calls[1].Args[0].(string)
	assert.Contains(t, discQuery, "MATCH (run {id: $agent_run_id})")
	assert.Contains(t, discQuery, "UNWIND $element_ids")
	assert.Contains(t, discQuery, "MERGE (run)-[r:DISCOVERED]->(node)")
}

func TestLoadBatch_ValidationErrors(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: ""}

	nodes := []domain.GraphNode{
		nil, // Nil node
		&mockGraphNode{
			nodeType: "", // Empty node type
			idProps:  map[string]any{"id": "test"},
		},
	}

	result, err := loader.LoadBatch(ctx, execCtx, nodes)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input validation failed")
	assert.True(t, result.HasErrors())
	assert.Len(t, result.Errors, 2)
}

func TestLoadBatch_QueryError(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock to return an error
	client.SetQueryError(types.NewError("QUERY_FAILED", "batch operation failed"))

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: ""}

	nodes := []domain.GraphNode{
		&mockGraphNode{
			nodeType: "host",
			idProps:  map[string]any{"ip": "192.168.1.1"},
			props:    map[string]any{"ip": "192.168.1.1"},
		},
	}

	result, err := loader.LoadBatch(ctx, execCtx, nodes)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "batch node creation failed")
	assert.NotNil(t, result)
}

func TestLoad_ParameterizedQueries(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{
				"node_id":   "mock-node-1",
				"operation": "created",
			},
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()
	execCtx := ExecContext{AgentRunID: ""}

	// Use a value that could cause injection if not parameterized
	node := &mockGraphNode{
		nodeType: "host",
		idProps:  map[string]any{"ip": "192.168.1.1'; DROP TABLE hosts; --"},
		props:    map[string]any{"ip": "192.168.1.1'; DROP TABLE hosts; --"},
	}

	result, err := loader.Load(ctx, execCtx, []domain.GraphNode{node})

	assert.NoError(t, err)
	assert.Equal(t, 1, result.NodesCreated)

	// Verify parameters were used (not string concatenation)
	calls := client.GetCallsByMethod("Query")
	assert.Len(t, calls, 1)

	params := calls[0].Args[1].(map[string]any)
	assert.Contains(t, params, "props")
	assert.Contains(t, params, "id_ip")

	// Verify the dangerous value is in parameters, not embedded in query string
	queryStr := calls[0].Args[0].(string)
	assert.NotContains(t, strings.ToLower(queryStr), "drop table")
}
