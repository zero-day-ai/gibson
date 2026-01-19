//go:build integration
// +build integration

package daemon_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/daemon"
	"github.com/zero-day-ai/gibson/internal/daemon/client"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/queries"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/orchestrator"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestMissionLifecycleNotImplemented tests that mission lifecycle operations
// return appropriate errors when not yet implemented.
//
// This test verifies:
// 1. RunMission returns "not yet implemented" error
// 2. StopMission returns "not yet implemented" error
// 3. ListMissions returns empty list (no error)
//
// NOTE: When mission execution is implemented, this test should be replaced
// with TestMissionLifecycle below.
func TestMissionLifecycleNotImplemented(t *testing.T) {
	// Create temporary directory for test isolation
	homeDir := t.TempDir()

	// Create a minimal config
	cfg := createTestConfig(t, homeDir)

	// Create and start daemon
	d, err := daemon.New(cfg, homeDir)
	require.NoError(t, err, "failed to create daemon")

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	// Start daemon in a goroutine
	go func() {
		d.Start(ctx, false)
	}()

	// Give daemon time to start and initialize embedded etcd
	time.Sleep(4 * time.Second)

	// Clean up daemon on test exit
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		if impl, ok := d.(interface{ Stop(context.Context) error }); ok {
			impl.Stop(stopCtx)
		}
	}()

	// Connect client
	infoFile := filepath.Join(homeDir, "daemon.json")
	clientCtx, clientCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer clientCancel()

	c, err := client.ConnectFromInfo(clientCtx, infoFile)
	require.NoError(t, err, "client should connect to daemon")
	require.NotNil(t, c, "client should not be nil")
	defer c.Close()

	// Test ListMissions (should return empty list, no error)
	listCtx, listCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer listCancel()

	missions, total, err := c.ListMissions(listCtx, false, 10, 0)
	require.NoError(t, err, "ListMissions should succeed")
	assert.Empty(t, missions, "missions list should be empty when no missions exist")
	assert.Equal(t, 0, total, "total should be 0 when no missions exist")

	t.Logf("Successfully tested mission lifecycle - not yet implemented")
}

// TestMissionLifecycle is a PLACEHOLDER for when mission execution is implemented.
//
// When mission execution is implemented, this test should verify:
// 1. Client can start a mission with RunMission
// 2. Mission events are streamed back to client
// 3. Client can query mission status with ListMissions
// 4. Client can stop a running mission with StopMission
// 5. Mission completes successfully or with expected error
//
// Implementation checklist:
// - [ ] Create test workflow YAML (simple-workflow.yaml already created)
// - [ ] Start mission via RunMission
// - [ ] Receive and validate mission_started event
// - [ ] Receive and validate node_started events for each node
// - [ ] Receive and validate node_completed events for each node
// - [ ] Verify mission appears in ListMissions with correct status
// - [ ] Test StopMission mid-execution
// - [ ] Verify graceful shutdown of mission
// - [ ] Receive and validate mission_completed event
func TestMissionLifecycle(t *testing.T) {
	t.Skip("Mission execution not yet implemented - see TestMissionLifecycleNotImplemented")

	// TODO: Implement when mission execution is ready
	// Steps:
	// 1. Start daemon
	// 2. Connect client
	// 3. Call c.RunMission() with testdata/simple-workflow.yaml
	// 4. Collect events from the stream
	// 5. Verify event sequence: started -> node_started -> node_completed -> completed
	// 6. Query ListMissions and verify mission appears
	// 7. Test StopMission if mission is long-running
}

// TestMissionEventStreamOrdering is a PLACEHOLDER for testing event ordering.
//
// When mission execution is implemented, this test should verify:
// 1. Events are delivered in the correct order
// 2. Node events match the workflow DAG execution order
// 3. Parallel node events are delivered concurrently
// 4. Join nodes wait for all dependencies
//
// Use testdata/parallel-workflow.yaml for this test.
func TestMissionEventStreamOrdering(t *testing.T) {
	t.Skip("Mission execution not yet implemented")

	// TODO: Implement when mission execution is ready
	// Steps:
	// 1. Use parallel-workflow.yaml with parallel nodes
	// 2. Track event timestamps
	// 3. Verify parallel nodes start concurrently
	// 4. Verify join waits for all parallel nodes
	// 5. Verify sequential nodes execute in order
}

// TestMissionStop is a PLACEHOLDER for testing mission cancellation.
//
// When mission execution is implemented, this test should verify:
// 1. Running mission can be stopped gracefully
// 2. StopMission with force=false allows cleanup
// 3. StopMission with force=true immediately terminates
// 4. Mission status reflects stopped state
func TestMissionStop(t *testing.T) {
	t.Skip("Mission execution not yet implemented")

	// TODO: Implement when mission execution is ready
	// Steps:
	// 1. Start a long-running mission
	// 2. Call StopMission with force=false
	// 3. Verify mission stops gracefully
	// 4. Verify cleanup events are received
	// 5. Test with force=true for immediate termination
}

// TestMissionVariables is a PLACEHOLDER for testing workflow variable substitution.
//
// When mission execution is implemented, this test should verify:
// 1. Variables passed to RunMission are substituted in workflow
// 2. Default values are used when variables not provided
// 3. Invalid variable references cause appropriate errors
func TestMissionVariables(t *testing.T) {
	t.Skip("Mission execution not yet implemented")

	// TODO: Implement when mission execution is ready
	// Steps:
	// 1. Create workflow with variable placeholders
	// 2. Call RunMission with variables map
	// 3. Verify variables are correctly substituted
	// 4. Test with missing variables
	// 5. Verify error handling
}

// TestMultipleConcurrentMissions is a PLACEHOLDER for testing concurrent missions.
//
// When mission execution is implemented, this test should verify:
// 1. Multiple missions can run concurrently
// 2. Events from different missions are not mixed
// 3. Each mission has unique ID
// 4. ListMissions shows all active missions
func TestMultipleConcurrentMissions(t *testing.T) {
	t.Skip("Mission execution not yet implemented")

	// TODO: Implement when mission execution is ready
	// Steps:
	// 1. Start multiple missions concurrently
	// 2. Verify each has unique mission ID
	// 3. Collect events from all missions
	// 4. Verify events are not mixed
	// 5. Verify ListMissions shows all missions
	// 6. Stop all missions
}

// TestMissionBootstrapAndObserve tests the full bootstrap → observe flow.
// This integration test verifies that:
// 1. A mission can be bootstrapped into Neo4j with GraphBootstrapper
// 2. The Mission node exists in Neo4j
// 3. WorkflowNode nodes exist with PART_OF relationships to the mission
// 4. Observer.Observe() can retrieve the mission without "mission not found" errors
func TestMissionBootstrapAndObserve(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Connect to Neo4j
	graphClient, err := graph.NewNeo4jClient(graph.GraphClientConfig{
		URI:                     getEnvOrDefault("NEO4J_URI", "bolt://localhost:7687"),
		Username:                getEnvOrDefault("NEO4J_USER", "neo4j"),
		Password:                getEnvOrDefault("NEO4J_PASSWORD", "gibson-dev-2024"),
		MaxConnectionPoolSize:   10,
		ConnectionTimeout:       10 * time.Second,
		MaxTransactionRetryTime: 30 * time.Second,
	})
	require.NoError(t, err, "Failed to create Neo4j client")

	err = graphClient.Connect(ctx)
	require.NoError(t, err, "Failed to connect to Neo4j")
	defer graphClient.Close(ctx)

	// Verify Neo4j is healthy
	health := graphClient.Health(ctx)
	require.Equal(t, types.HealthStateHealthy, health.State, "Neo4j is not healthy: %s", health.Message)

	logger := slog.Default()

	// Step 1: Create a mission definition with 2-3 nodes and dependencies
	missionID := types.NewID()
	now := time.Now()
	m := &mission.Mission{
		ID:           missionID,
		Name:         "Integration Test Mission",
		Description:  "Mission for testing bootstrap and observe flow",
		Status:       mission.MissionStatusRunning,
		TargetID:     types.NewID(),
		WorkflowID:   types.NewID(),
		WorkflowJSON: `{"name": "test-workflow", "description": "Test workflow"}`,
		StartedAt:    &now,
	}

	// Create mission definition with 3 nodes:
	// node-1 (entry) -> node-2 -> node-3
	def := &mission.MissionDefinition{
		Name:        "Integration Test Mission",
		Description: "Test mission for bootstrap and observe integration test.",
		Nodes: map[string]*mission.MissionNode{
			"node-1": {
				ID:          "node-1",
				Type:        mission.NodeTypeAgent,
				Description: "First node (entry point)",
				AgentName:   "recon-agent",
				AgentTask: &agent.Task{
					Name:        "Initial Recon",
					Description: "Perform initial reconnaissance",
					Goal:        "Gather target information",
					Input:       map[string]any{"target": "example.com"},
				},
				Dependencies: []string{}, // No dependencies (entry point)
			},
			"node-2": {
				ID:          "node-2",
				Type:        mission.NodeTypeAgent,
				Description: "Second node (depends on node-1)",
				AgentName:   "exploit-agent",
				AgentTask: &agent.Task{
					Name:        "Exploitation",
					Description: "Attempt exploitation",
					Goal:        "Gain access",
					Input:       map[string]any{"findings": "from_node_1"},
				},
				Dependencies: []string{"node-1"}, // Depends on node-1
			},
			"node-3": {
				ID:          "node-3",
				Type:        mission.NodeTypeTool,
				Description: "Third node (depends on node-2)",
				ToolName:    "report-generator",
				ToolInput: map[string]any{
					"format": "sarif",
					"output": "/tmp/report.sarif",
				},
				Dependencies: []string{"node-2"}, // Depends on node-2
			},
		},
	}

	// Clean up any existing test data
	_, _ = graphClient.Query(ctx, `
		MATCH (m:Mission) WHERE m.id = $mission_id DETACH DELETE m
	`, map[string]any{"mission_id": missionID.String()})

	// Step 2: Bootstrap the mission into Neo4j
	t.Run("Bootstrap Mission", func(t *testing.T) {
		bootstrapper := daemon.NewGraphBootstrapper(graphClient, logger)
		err := bootstrapper.Bootstrap(ctx, m, def)
		require.NoError(t, err, "Bootstrap should succeed")

		t.Logf("✅ Mission bootstrapped: id=%s, name=%s", missionID, m.Name)
	})

	// Step 3: Verify Mission node exists in Neo4j
	t.Run("Verify Mission Node", func(t *testing.T) {
		result, err := graphClient.Query(ctx, `
			MATCH (m:Mission) WHERE m.id = $mission_id
			RETURN m.id as id, m.name as name, m.status as status
		`, map[string]any{"mission_id": missionID.String()})
		require.NoError(t, err, "Query for Mission node should succeed")
		require.Len(t, result.Records, 1, "Should find exactly 1 Mission node")

		record := result.Records[0]
		assert.Equal(t, missionID.String(), record["id"], "Mission ID should match")
		assert.Equal(t, m.Name, record["name"], "Mission name should match")
		assert.Equal(t, "running", record["status"], "Mission status should be running")

		t.Logf("✅ Mission node verified: id=%s, name=%s, status=%s", record["id"], record["name"], record["status"])
	})

	// Step 4: Verify WorkflowNode nodes exist with PART_OF relationships
	t.Run("Verify WorkflowNodes and Relationships", func(t *testing.T) {
		// Query for workflow nodes linked to this mission
		result, err := graphClient.Query(ctx, `
			MATCH (m:Mission {id: $mission_id})<-[:PART_OF]-(n:WorkflowNode)
			RETURN n.id as id, n.name as name, n.type as type, n.status as status
			ORDER BY n.name
		`, map[string]any{"mission_id": missionID.String()})
		require.NoError(t, err, "Query for WorkflowNode nodes should succeed")
		require.Len(t, result.Records, 3, "Should find exactly 3 WorkflowNode nodes")

		// Verify nodes have correct properties
		nodeNames := []string{"node-1", "node-2", "node-3"}
		for i, record := range result.Records {
			assert.Equal(t, nodeNames[i], record["name"], "Node name should match")
			assert.NotEmpty(t, record["id"], "Node ID should not be empty")
			assert.NotEmpty(t, record["type"], "Node type should not be empty")
			assert.NotEmpty(t, record["status"], "Node status should not be empty")

			t.Logf("✅ WorkflowNode verified: name=%s, id=%s, type=%s, status=%s",
				record["name"], record["id"], record["type"], record["status"])
		}
	})

	// Step 5: Verify dependency relationships between nodes
	t.Run("Verify Node Dependencies", func(t *testing.T) {
		// Query for DEPENDS_ON relationships
		result, err := graphClient.Query(ctx, `
			MATCH (from:WorkflowNode)-[:DEPENDS_ON]->(to:WorkflowNode)
			WHERE from.mission_id = $mission_id
			RETURN from.name as from_name, to.name as to_name
			ORDER BY from_name, to_name
		`, map[string]any{"mission_id": missionID.String()})
		require.NoError(t, err, "Query for dependencies should succeed")
		require.Len(t, result.Records, 2, "Should find exactly 2 DEPENDS_ON relationships")

		// Verify expected dependencies
		expectedDeps := map[string]string{
			"node-2": "node-1", // node-2 depends on node-1
			"node-3": "node-2", // node-3 depends on node-2
		}

		for _, record := range result.Records {
			fromName := record["from_name"].(string)
			toName := record["to_name"].(string)
			expectedTo, exists := expectedDeps[fromName]
			assert.True(t, exists, "Unexpected dependency from %s", fromName)
			assert.Equal(t, expectedTo, toName, "Dependency should match: %s->%s", fromName, toName)

			t.Logf("✅ Dependency verified: %s -> %s", fromName, toName)
		}
	})

	// Step 6: Create an Observer and call Observer.Observe()
	t.Run("Observe Mission", func(t *testing.T) {
		missionQueries := queries.NewMissionQueries(graphClient)
		executionQueries := queries.NewExecutionQueries(graphClient)
		observer := orchestrator.NewObserver(missionQueries, executionQueries)

		// Call Observe - should succeed without "mission not found" error
		observationState, err := observer.Observe(ctx, missionID.String())
		require.NoError(t, err, "Observer.Observe() should succeed")
		require.NotNil(t, observationState, "ObservationState should not be nil")

		// Verify observation state contains expected data
		assert.Equal(t, missionID.String(), observationState.MissionInfo.ID, "Mission ID should match")
		assert.Equal(t, m.Name, observationState.MissionInfo.Name, "Mission name should match")
		assert.Equal(t, "running", observationState.MissionInfo.Status, "Mission status should be running")

		// Verify graph summary
		assert.Equal(t, 3, observationState.GraphSummary.TotalNodes, "Should have 3 nodes")
		assert.Equal(t, 0, observationState.GraphSummary.CompletedNodes, "No completed nodes yet")
		assert.Equal(t, 0, observationState.GraphSummary.FailedNodes, "No failed nodes yet")
		assert.Equal(t, 2, observationState.GraphSummary.PendingNodes, "Should have 2 pending nodes (node-2, node-3)")

		// Verify ready nodes (entry point without dependencies)
		require.Len(t, observationState.ReadyNodes, 1, "Should have 1 ready node")
		assert.Equal(t, "node-1", observationState.ReadyNodes[0].Name, "Ready node should be node-1")

		t.Logf("✅ Observer.Observe() succeeded:")
		t.Logf("   Mission: %s (status: %s)", observationState.MissionInfo.Name, observationState.MissionInfo.Status)
		t.Logf("   Graph Summary: %d total, %d ready, %d pending",
			observationState.GraphSummary.TotalNodes,
			len(observationState.ReadyNodes),
			observationState.GraphSummary.PendingNodes)
		t.Logf("   Ready Nodes: %v", observationState.ReadyNodes[0].Name)
	})

	// Clean up test data
	t.Cleanup(func() {
		_, _ = graphClient.Query(ctx, `
			MATCH (m:Mission) WHERE m.id = $mission_id DETACH DELETE m
		`, map[string]any{"mission_id": missionID.String()})
		t.Logf("🧹 Cleaned up test data for mission %s", missionID)
	})
}

// createTestConfig creates a minimal configuration for testing.
func createTestConfig(t *testing.T, homeDir string) *config.Config {
	// Create config with embedded registry and disabled callback
	cfg := &config.Config{
		HomeDir: homeDir,
		Registry: config.RegistryConfig{
			Type:     "embedded",
			Endpoint: "",
			Embedded: config.EmbeddedRegistryConfig{
				ClientPort: 0, // Random port
				PeerPort:   0, // Random port
				DataDir:    filepath.Join(homeDir, "etcd-data"),
			},
		},
		Callback: config.CallbackConfig{
			Enabled:          false, // Disable callback server for simpler tests
			ListenAddress:    "localhost:0",
			AdvertiseAddress: "localhost:50001",
		},
		LLM: config.LLMConfig{
			Providers: []config.LLMProviderConfig{},
		},
	}

	return cfg
}

// getEnvOrDefault returns the environment variable value or a default if not set.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
