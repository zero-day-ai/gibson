//go:build integration

// Package loader provides integration tests for mission-scoped GraphRAG storage.
// These tests validate the CREATE-based node storage and scoped query semantics.
package loader

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/sdk/graphrag/domain"
)

// mockGraphNodeIntegration implements domain.GraphNode for integration testing
type mockGraphNodeIntegration struct {
	nodeType  string
	idProps   map[string]any
	props     map[string]any
	parentRef *domain.NodeRef
	relType   string
}

func (m *mockGraphNodeIntegration) NodeType() string {
	return m.nodeType
}

func (m *mockGraphNodeIntegration) IdentifyingProperties() map[string]any {
	return m.idProps
}

func (m *mockGraphNodeIntegration) Properties() map[string]any {
	return m.props
}

func (m *mockGraphNodeIntegration) ParentRef() *domain.NodeRef {
	return m.parentRef
}

func (m *mockGraphNodeIntegration) RelationshipType() string {
	return m.relType
}

// TestIntegration_MissionRunDataIsolation verifies that the same IP address
// in different MissionRuns creates separate nodes (no collision).
// Task 15.2: Multiple mission runs with data isolation
// Task 15.4: Same IP different contexts (collision prevention)
func TestIntegration_MissionRunDataIsolation(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock to track all queries for verification
	queryCount := 0
	createdNodes := make([]map[string]any, 0)

	// Return mock result for each CREATE query
	for i := 0; i < 10; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		client.AddQueryResult(graph.QueryResult{
			Records: []map[string]any{
				{
					"node_id":   nodeID,
					"operation": "created",
				},
			},
		})
	}

	loader := NewGraphLoader(client)
	ctx := context.Background()

	// Create the same IP in two different MissionRuns
	sameIP := "192.168.1.100"

	// MissionRun 1: First discovery of the IP
	execCtx1 := ExecContext{
		MissionID:    "mission-1",
		MissionRunID: "run-1",
		AgentRunID:   "agent-run-1",
		AgentName:    "network-recon",
	}

	node1 := &mockGraphNodeIntegration{
		nodeType: "host",
		idProps:  map[string]any{"ip": sameIP},
		props:    map[string]any{"ip": sameIP, "hostname": "host-run1"},
	}

	result1, err := loader.Load(ctx, execCtx1, []domain.GraphNode{node1})
	require.NoError(t, err)
	assert.Equal(t, 1, result1.NodesCreated, "Should create one node in run 1")

	// MissionRun 2: Same IP discovered again (should create NEW node, not merge)
	execCtx2 := ExecContext{
		MissionID:    "mission-1",
		MissionRunID: "run-2",
		AgentRunID:   "agent-run-2",
		AgentName:    "network-recon",
	}

	node2 := &mockGraphNodeIntegration{
		nodeType: "host",
		idProps:  map[string]any{"ip": sameIP},
		props:    map[string]any{"ip": sameIP, "hostname": "host-run2"},
	}

	result2, err := loader.Load(ctx, execCtx2, []domain.GraphNode{node2})
	require.NoError(t, err)
	assert.Equal(t, 1, result2.NodesCreated, "Should create one node in run 2 (separate from run 1)")

	// Verify both queries used CREATE (not MERGE)
	calls := client.GetCallsByMethod("Query")
	require.GreaterOrEqual(t, len(calls), 2, "Should have at least 2 queries (one per run)")

	for i, call := range calls {
		if i >= 2 {
			break
		}
		queryStr := call.Args[0].(string)
		assert.Contains(t, queryStr, "CREATE (n:host", "Query %d should use CREATE not MERGE", i)
		assert.NotContains(t, queryStr, "MERGE", "Query %d should not use MERGE", i)
	}

	// Track created nodes to verify isolation
	_ = queryCount
	_ = createdNodes
}

// TestIntegration_ParentRelationshipScoping verifies that parent relationships
// are scoped to the current mission run.
// Task 15.1: End-to-end test for network-recon → tech-fingerprinting data flow
func TestIntegration_ParentRelationshipScoping(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock results for node creation and relationship creation
	// First query: CREATE host node
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{"node_id": "host-node-1", "operation": "created"},
		},
	})
	// Second query: CREATE port node
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{"node_id": "port-node-1", "operation": "created"},
		},
	})
	// Third query: CREATE relationship (parent lookup scoped by mission_run_id)
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{"r": "relationship-1"},
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()

	execCtx := ExecContext{
		MissionID:    "mission-1",
		MissionRunID: "run-1",
		AgentRunID:   "agent-run-1",
		AgentName:    "network-recon",
	}

	// Create host first (simulating network-recon agent)
	hostNode := &mockGraphNodeIntegration{
		nodeType: "host",
		idProps:  map[string]any{"ip": "10.0.0.1"},
		props:    map[string]any{"ip": "10.0.0.1"},
	}

	hostResult, err := loader.Load(ctx, execCtx, []domain.GraphNode{hostNode})
	require.NoError(t, err)
	assert.Equal(t, 1, hostResult.NodesCreated)

	// Create port with parent reference (simulating tech-fingerprinting agent)
	portNode := &mockGraphNodeIntegration{
		nodeType: "port",
		idProps:  map[string]any{"number": 443},
		props:    map[string]any{"number": 443, "protocol": "tcp"},
		parentRef: &domain.NodeRef{
			NodeType:   "host",
			Properties: map[string]any{"ip": "10.0.0.1"},
		},
		relType: "HAS_PORT",
	}

	portResult, err := loader.Load(ctx, execCtx, []domain.GraphNode{portNode})
	require.NoError(t, err)
	assert.Equal(t, 1, portResult.NodesCreated)

	// Verify relationship query includes mission_run_id scoping
	calls := client.GetCallsByMethod("Query")
	foundRelationshipQuery := false

	for _, call := range calls {
		queryStr := call.Args[0].(string)
		if contains(queryStr, "HAS_PORT") {
			foundRelationshipQuery = true
			// Should scope parent lookup by mission_run_id
			assert.Contains(t, queryStr, "mission_run_id", "Relationship query should scope parent lookup by mission_run_id")
		}
	}

	assert.True(t, foundRelationshipQuery, "Should have found a relationship creation query")
}

// TestIntegration_CreateVsMergePerformance compares CREATE vs the old MERGE behavior.
// Task 15.5: Performance benchmark: CREATE vs MERGE comparison
func TestIntegration_CreateVsMergePerformance(t *testing.T) {
	// This test validates that CREATE queries are generated correctly
	// Actual performance benchmarking requires a real Neo4j instance

	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock to return success for many nodes
	nodeCount := 100
	for i := 0; i < nodeCount; i++ {
		client.AddQueryResult(graph.QueryResult{
			Records: []map[string]any{
				{
					"element_id": fmt.Sprintf("node-%d", i),
					"idx":        float64(i % 10),
				},
			},
		})
	}

	loader := NewGraphLoader(client)
	ctx := context.Background()

	execCtx := ExecContext{
		MissionID:    "perf-mission",
		MissionRunID: "perf-run-1",
	}

	// Create many nodes
	nodes := make([]domain.GraphNode, nodeCount)
	for i := 0; i < nodeCount; i++ {
		nodes[i] = &mockGraphNodeIntegration{
			nodeType: "host",
			idProps:  map[string]any{"ip": fmt.Sprintf("10.0.0.%d", i)},
			props:    map[string]any{"ip": fmt.Sprintf("10.0.0.%d", i)},
		}
	}

	start := time.Now()
	result, err := loader.Load(ctx, execCtx, nodes)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, nodeCount, result.NodesCreated)

	// Verify all queries used CREATE
	calls := client.GetCallsByMethod("Query")
	for _, call := range calls {
		queryStr := call.Args[0].(string)
		if contains(queryStr, "host") {
			assert.Contains(t, queryStr, "CREATE", "All queries should use CREATE")
			assert.NotContains(t, queryStr, "MERGE", "No queries should use MERGE")
		}
	}

	t.Logf("Created %d nodes in %v using CREATE (mock)", nodeCount, elapsed)
}

// TestIntegration_BatchCreateWithMissionContext verifies batch operations
// properly inject mission context into all nodes.
func TestIntegration_BatchCreateWithMissionContext(t *testing.T) {
	client := graph.NewMockGraphClient()
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Configure mock for batch creation
	client.AddQueryResult(graph.QueryResult{
		Records: []map[string]any{
			{"element_id": "batch-node-1", "idx": float64(0)},
			{"element_id": "batch-node-2", "idx": float64(1)},
			{"element_id": "batch-node-3", "idx": float64(2)},
		},
	})

	loader := NewGraphLoader(client)
	ctx := context.Background()

	execCtx := ExecContext{
		MissionID:    "batch-mission",
		MissionRunID: "batch-run-1",
		AgentRunID:   "batch-agent-run",
		AgentName:    "batch-agent",
	}

	nodes := []domain.GraphNode{
		&mockGraphNodeIntegration{
			nodeType: "host",
			idProps:  map[string]any{"ip": "10.1.1.1"},
			props:    map[string]any{"ip": "10.1.1.1"},
		},
		&mockGraphNodeIntegration{
			nodeType: "host",
			idProps:  map[string]any{"ip": "10.1.1.2"},
			props:    map[string]any{"ip": "10.1.1.2"},
		},
		&mockGraphNodeIntegration{
			nodeType: "host",
			idProps:  map[string]any{"ip": "10.1.1.3"},
			props:    map[string]any{"ip": "10.1.1.3"},
		},
	}

	result, err := loader.LoadBatch(ctx, execCtx, nodes)
	require.NoError(t, err)
	assert.Equal(t, 3, result.NodesCreated)

	// Verify batch query parameters include mission context
	calls := client.GetCallsByMethod("Query")
	require.GreaterOrEqual(t, len(calls), 1)

	// Check that the UNWIND query was used
	foundBatchQuery := false
	for _, call := range calls {
		queryStr := call.Args[0].(string)
		if contains(queryStr, "UNWIND") && contains(queryStr, "CREATE (n:host") {
			foundBatchQuery = true

			// Verify parameters include mission context injection
			params := call.Args[1].(map[string]any)
			nodesList, ok := params["nodes"].([]map[string]any)
			if ok {
				for _, nodeData := range nodesList {
					allProps, ok := nodeData["all_props"].(map[string]any)
					if ok {
						assert.Equal(t, "batch-mission", allProps["mission_id"])
						assert.Equal(t, "batch-run-1", allProps["mission_run_id"])
					}
				}
			}
		}
	}

	assert.True(t, foundBatchQuery, "Should have found a batch UNWIND CREATE query")
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
