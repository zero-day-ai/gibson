package workflow

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestBuildSnapshotFromMission tests building a snapshot from mission data
func TestBuildSnapshotFromMission(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	startTime := baseTime.Add(5 * time.Minute)
	completeTime := baseTime.Add(15 * time.Minute)

	tests := []struct {
		name             string
		mission          *schema.Mission
		nodes            []*schema.WorkflowNode
		decisions        []*schema.Decision
		includeHistory   bool
		validateSnapshot func(t *testing.T, snap *testWorkflowSnapshot)
	}{
		{
			name: "simple completed mission",
			mission: &schema.Mission{
				ID:          types.NewID(),
				Name:        "Test Mission",
				Description: "A simple test mission",
				Status:      schema.MissionStatusCompleted,
				CreatedAt:   baseTime,
				StartedAt:   &startTime,
				CompletedAt: &completeTime,
			},
			nodes: []*schema.WorkflowNode{
				{
					ID:          types.NewID(),
					Name:        "scan_target",
					Type:        schema.WorkflowNodeTypeAgent,
					Status:      schema.WorkflowNodeStatusCompleted,
					IsDynamic:   false,
					AgentName:   "scanner",
					TaskConfig:  map[string]any{"target": "example.com"},
					CreatedAt:   baseTime,
					UpdatedAt:   startTime,
				},
				{
					ID:          types.NewID(),
					Name:        "analyze_results",
					Type:        schema.WorkflowNodeTypeTool,
					Status:      schema.WorkflowNodeStatusCompleted,
					IsDynamic:   false,
					ToolName:    "analyzer",
					CreatedAt:   baseTime,
					UpdatedAt:   completeTime,
				},
			},
			decisions:      []*schema.Decision{},
			includeHistory: false,
			validateSnapshot: func(t *testing.T, snap *testWorkflowSnapshot) {
				require.NotNil(t, snap)
				require.NotNil(t, snap.Mission)
				require.NotNil(t, snap.Stats)

				// Validate mission metadata
				assert.Equal(t, "Test Mission", snap.Mission.Name)
				assert.Equal(t, "A simple test mission", snap.Mission.Description)
				assert.Equal(t, "completed", snap.Mission.Status)
				assert.Equal(t, baseTime, snap.Mission.CreatedAt)
				assert.Equal(t, startTime, snap.Mission.StartedAt)
				assert.Equal(t, completeTime, snap.Mission.CompletedAt)

				// Validate nodes
				assert.Len(t, snap.Nodes, 2)
				assert.Equal(t, "scan_target", snap.Nodes[0].Name)
				assert.Equal(t, "agent", snap.Nodes[0].Type)
				assert.Equal(t, "completed", snap.Nodes[0].Status)
				assert.False(t, snap.Nodes[0].IsDynamic)
				assert.Equal(t, "scanner", snap.Nodes[0].AgentName)

				assert.Equal(t, "analyze_results", snap.Nodes[1].Name)
				assert.Equal(t, "tool", snap.Nodes[1].Type)
				assert.Equal(t, "analyzer", snap.Nodes[1].ToolName)

				// Validate stats
				assert.Equal(t, 2, snap.Stats.TotalNodes)
				assert.Equal(t, 2, snap.Stats.CompletedNodes)
				assert.Equal(t, 0, snap.Stats.FailedNodes)
				assert.Equal(t, 0, snap.Stats.DynamicNodes)
				assert.Equal(t, 10*time.Minute, snap.Stats.Duration)

				// No decisions when history not included
				assert.Nil(t, snap.Decisions)
			},
		},
		{
			name: "mission with dynamic nodes",
			mission: &schema.Mission{
				ID:          types.NewID(),
				Name:        "Adaptive Mission",
				Description: "Mission with spawned agents",
				Status:      schema.MissionStatusRunning,
				CreatedAt:   baseTime,
				StartedAt:   &startTime,
			},
			nodes: []*schema.WorkflowNode{
				{
					ID:          types.NewID(),
					Name:        "orchestrator",
					Type:        schema.WorkflowNodeTypeAgent,
					Status:      schema.WorkflowNodeStatusRunning,
					IsDynamic:   false,
					AgentName:   "orchestrator",
					CreatedAt:   baseTime,
					UpdatedAt:   startTime,
				},
				{
					ID:          types.NewID(),
					Name:        "spawned_recon_1",
					Type:        schema.WorkflowNodeTypeAgent,
					Status:      schema.WorkflowNodeStatusCompleted,
					IsDynamic:   true,
					SpawnedBy:   "orchestrator",
					AgentName:   "recon-agent",
					TaskConfig:  map[string]any{"target": "subdomain1.example.com"},
					CreatedAt:   startTime,
					UpdatedAt:   startTime.Add(2 * time.Minute),
				},
				{
					ID:          types.NewID(),
					Name:        "spawned_recon_2",
					Type:        schema.WorkflowNodeTypeAgent,
					Status:      schema.WorkflowNodeStatusPending,
					IsDynamic:   true,
					SpawnedBy:   "orchestrator",
					AgentName:   "recon-agent",
					TaskConfig:  map[string]any{"target": "subdomain2.example.com"},
					CreatedAt:   startTime.Add(time.Minute),
					UpdatedAt:   startTime.Add(time.Minute),
				},
			},
			decisions:      []*schema.Decision{},
			includeHistory: false,
			validateSnapshot: func(t *testing.T, snap *testWorkflowSnapshot) {
				require.NotNil(t, snap)

				// Validate mission is still running
				assert.Equal(t, "running", snap.Mission.Status)
				assert.NotZero(t, snap.Mission.StartedAt)
				assert.Zero(t, snap.Mission.CompletedAt)

				// Validate nodes including dynamic ones
				assert.Len(t, snap.Nodes, 3)

				// Find dynamic nodes
				dynamicCount := 0
				for _, node := range snap.Nodes {
					if node.IsDynamic {
						dynamicCount++
						assert.NotEmpty(t, node.SpawnedBy, "Dynamic node should have SpawnedBy set")
						assert.Equal(t, "orchestrator", node.SpawnedBy)
					}
				}
				assert.Equal(t, 2, dynamicCount)

				// Validate stats
				assert.Equal(t, 3, snap.Stats.TotalNodes)
				assert.Equal(t, 1, snap.Stats.CompletedNodes)
				assert.Equal(t, 2, snap.Stats.DynamicNodes)
				assert.Equal(t, time.Duration(0), snap.Stats.Duration, "Running mission should have no duration")
			},
		},
		{
			name: "mission with history and decisions",
			mission: &schema.Mission{
				ID:          types.NewID(),
				Name:        "Orchestrated Mission",
				Description: "Mission with orchestrator decisions",
				Status:      schema.MissionStatusCompleted,
				CreatedAt:   baseTime,
				StartedAt:   &startTime,
				CompletedAt: &completeTime,
			},
			nodes: []*schema.WorkflowNode{
				{
					ID:          types.NewID(),
					Name:        "initial_scan",
					Type:        schema.WorkflowNodeTypeAgent,
					Status:      schema.WorkflowNodeStatusCompleted,
					IsDynamic:   false,
					AgentName:   "scanner",
					CreatedAt:   baseTime,
					UpdatedAt:   startTime.Add(3 * time.Minute),
				},
			},
			decisions: []*schema.Decision{
				{
					Iteration:    1,
					Action:       schema.DecisionActionExecuteAgent,
					Reasoning:    "Starting initial scan of target",
					Confidence:   0.95,
					TargetNodeID: "initial_scan",
					Timestamp:    startTime,
				},
				{
					Iteration:    2,
					Action:       schema.DecisionActionComplete,
					Reasoning:    "All objectives achieved, mission complete",
					Confidence:   0.98,
					Timestamp:    completeTime,
				},
			},
			includeHistory: true,
			validateSnapshot: func(t *testing.T, snap *testWorkflowSnapshot) {
				require.NotNil(t, snap)

				// Validate decisions are included
				require.NotNil(t, snap.Decisions)
				assert.Len(t, snap.Decisions, 2)

				// Validate first decision
				assert.Equal(t, 1, snap.Decisions[0].Iteration)
				assert.Equal(t, "execute", snap.Decisions[0].Action)
				assert.Equal(t, "Starting initial scan of target", snap.Decisions[0].Reasoning)
				assert.Equal(t, 0.95, snap.Decisions[0].Confidence)
				assert.Equal(t, "initial_scan", snap.Decisions[0].TargetNodeID)

				// Validate second decision
				assert.Equal(t, 2, snap.Decisions[1].Iteration)
				assert.Equal(t, "complete", snap.Decisions[1].Action)
				assert.NotEmpty(t, snap.Decisions[1].Reasoning)
			},
		},
		{
			name: "mission with failed nodes",
			mission: &schema.Mission{
				ID:          types.NewID(),
				Name:        "Failed Mission",
				Description: "Mission with some failed nodes",
				Status:      schema.MissionStatusFailed,
				CreatedAt:   baseTime,
				StartedAt:   &startTime,
				CompletedAt: &completeTime,
			},
			nodes: []*schema.WorkflowNode{
				{
					ID:          types.NewID(),
					Name:        "successful_node",
					Type:        schema.WorkflowNodeTypeAgent,
					Status:      schema.WorkflowNodeStatusCompleted,
					IsDynamic:   false,
					AgentName:   "agent1",
					CreatedAt:   baseTime,
					UpdatedAt:   startTime.Add(2 * time.Minute),
				},
				{
					ID:          types.NewID(),
					Name:        "failed_node",
					Type:        schema.WorkflowNodeTypeAgent,
					Status:      schema.WorkflowNodeStatusFailed,
					IsDynamic:   false,
					AgentName:   "agent2",
					CreatedAt:   baseTime,
					UpdatedAt:   startTime.Add(5 * time.Minute),
				},
				{
					ID:          types.NewID(),
					Name:        "skipped_node",
					Type:        schema.WorkflowNodeTypeTool,
					Status:      schema.WorkflowNodeStatusSkipped,
					IsDynamic:   false,
					ToolName:    "tool1",
					CreatedAt:   baseTime,
					UpdatedAt:   baseTime,
				},
			},
			decisions:      []*schema.Decision{},
			includeHistory: false,
			validateSnapshot: func(t *testing.T, snap *testWorkflowSnapshot) {
				require.NotNil(t, snap)

				// Validate mission failed
				assert.Equal(t, "failed", snap.Mission.Status)

				// Validate node statuses
				assert.Len(t, snap.Nodes, 3)

				statusCount := make(map[string]int)
				for _, node := range snap.Nodes {
					statusCount[node.Status]++
				}
				assert.Equal(t, 1, statusCount["completed"])
				assert.Equal(t, 1, statusCount["failed"])
				assert.Equal(t, 1, statusCount["skipped"])

				// Validate stats
				assert.Equal(t, 3, snap.Stats.TotalNodes)
				assert.Equal(t, 1, snap.Stats.CompletedNodes)
				assert.Equal(t, 1, snap.Stats.FailedNodes)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build snapshot (simulating buildWorkflowSnapshot logic)
			snapshot := buildMockSnapshot(tt.mission, tt.nodes, tt.decisions, tt.includeHistory)

			// Validate snapshot
			tt.validateSnapshot(t, snapshot)
		})
	}
}

// TestSnapshotSerialization tests YAML and JSON serialization
func TestSnapshotSerialization(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	startTime := baseTime.Add(5 * time.Minute)
	completeTime := baseTime.Add(15 * time.Minute)

	mission := &schema.Mission{
		ID:          types.NewID(),
		Name:        "Serialization Test",
		Description: "Test YAML and JSON output",
		Status:      schema.MissionStatusCompleted,
		CreatedAt:   baseTime,
		StartedAt:   &startTime,
		CompletedAt: &completeTime,
	}

	nodes := []*schema.WorkflowNode{
		{
			ID:         types.NewID(),
			Name:       "test_node",
			Type:       schema.WorkflowNodeTypeAgent,
			Status:     schema.WorkflowNodeStatusCompleted,
			IsDynamic:  false,
			AgentName:  "test-agent",
			TaskConfig: map[string]any{"key": "value", "number": 42},
			CreatedAt:  baseTime,
			UpdatedAt:  completeTime,
		},
	}

	decisions := []*schema.Decision{
		{
			Iteration:    1,
			Action:       schema.DecisionActionExecuteAgent,
			Reasoning:    "Test decision",
			Confidence:   0.90,
			TargetNodeID: "test_node",
			Timestamp:    startTime,
		},
	}

	snapshot := buildMockSnapshot(mission, nodes, decisions, true)

	t.Run("YAML serialization", func(t *testing.T) {
		// Marshal to YAML
		yamlData, err := yaml.Marshal(snapshot)
		require.NoError(t, err)
		require.NotEmpty(t, yamlData)

		// Verify YAML is valid by unmarshaling
		var unmarshaled testWorkflowSnapshot
		err = yaml.Unmarshal(yamlData, &unmarshaled)
		require.NoError(t, err)

		// Validate key fields
		assert.Equal(t, "Serialization Test", unmarshaled.Mission.Name)
		assert.Equal(t, "completed", unmarshaled.Mission.Status)
		assert.Len(t, unmarshaled.Nodes, 1)
		assert.Equal(t, "test_node", unmarshaled.Nodes[0].Name)
		assert.Len(t, unmarshaled.Decisions, 1)

		// Verify YAML format contains expected sections
		yamlStr := string(yamlData)
		assert.Contains(t, yamlStr, "mission:")
		assert.Contains(t, yamlStr, "nodes:")
		assert.Contains(t, yamlStr, "decisions:")
		assert.Contains(t, yamlStr, "stats:")
	})

	t.Run("JSON serialization", func(t *testing.T) {
		// Marshal to JSON
		jsonData, err := json.Marshal(snapshot)
		require.NoError(t, err)
		require.NotEmpty(t, jsonData)

		// Verify JSON is valid by unmarshaling
		var unmarshaled testWorkflowSnapshot
		err = json.Unmarshal(jsonData, &unmarshaled)
		require.NoError(t, err)

		// Validate key fields
		assert.Equal(t, "Serialization Test", unmarshaled.Mission.Name)
		assert.Equal(t, "completed", unmarshaled.Mission.Status)
		assert.Len(t, unmarshaled.Nodes, 1)
		assert.Equal(t, "test_node", unmarshaled.Nodes[0].Name)

		// Verify JSON structure
		var jsonMap map[string]any
		err = json.Unmarshal(jsonData, &jsonMap)
		require.NoError(t, err)
		assert.Contains(t, jsonMap, "mission")
		assert.Contains(t, jsonMap, "nodes")
		assert.Contains(t, jsonMap, "decisions")
		assert.Contains(t, jsonMap, "stats")
	})

	t.Run("JSON pretty print", func(t *testing.T) {
		// Marshal to pretty JSON
		jsonData, err := json.MarshalIndent(snapshot, "", "  ")
		require.NoError(t, err)
		require.NotEmpty(t, jsonData)

		// Verify formatting
		jsonStr := string(jsonData)
		assert.Contains(t, jsonStr, "\n")
		assert.Contains(t, jsonStr, "  ")

		// Should still unmarshal correctly
		var unmarshaled testWorkflowSnapshot
		err = json.Unmarshal(jsonData, &unmarshaled)
		require.NoError(t, err)
		assert.Equal(t, "Serialization Test", unmarshaled.Mission.Name)
	})
}

// TestSnapshotRoundTrip tests the full cycle: parse → execute → snapshot → reconstruct
func TestSnapshotRoundTrip(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		buildSnapshot  func() *testWorkflowSnapshot
		validateCycle  func(t *testing.T, original, reconstructed *testWorkflowSnapshot)
	}{
		{
			name: "simple workflow round-trip",
			buildSnapshot: func() *testWorkflowSnapshot {
				mission := &schema.Mission{
					ID:          types.NewID(),
					Name:        "Round Trip Test",
					Description: "Test complete round-trip",
					Status:      schema.MissionStatusCompleted,
					CreatedAt:   baseTime,
				}

				nodes := []*schema.WorkflowNode{
					{
						ID:        types.NewID(),
						Name:      "node1",
						Type:      schema.WorkflowNodeTypeAgent,
						Status:    schema.WorkflowNodeStatusCompleted,
						IsDynamic: false,
						AgentName: "agent1",
						CreatedAt: baseTime,
						UpdatedAt: baseTime,
					},
				}

				return buildMockSnapshot(mission, nodes, nil, false)
			},
			validateCycle: func(t *testing.T, original, reconstructed *testWorkflowSnapshot) {
				assert.Equal(t, original.Mission.Name, reconstructed.Mission.Name)
				assert.Equal(t, original.Mission.Status, reconstructed.Mission.Status)
				assert.Len(t, reconstructed.Nodes, len(original.Nodes))
				assert.Equal(t, original.Nodes[0].Name, reconstructed.Nodes[0].Name)
				assert.Equal(t, original.Nodes[0].Type, reconstructed.Nodes[0].Type)
			},
		},
		{
			name: "complex workflow with all features",
			buildSnapshot: func() *testWorkflowSnapshot {
				startTime := baseTime.Add(5 * time.Minute)
				completeTime := baseTime.Add(20 * time.Minute)

				mission := &schema.Mission{
					ID:          types.NewID(),
					Name:        "Complex Round Trip",
					Description: "Test with all features",
					Status:      schema.MissionStatusCompleted,
					CreatedAt:   baseTime,
					StartedAt:   &startTime,
					CompletedAt: &completeTime,
				}

				nodes := []*schema.WorkflowNode{
					{
						ID:         types.NewID(),
						Name:       "static_node",
						Type:       schema.WorkflowNodeTypeAgent,
						Status:     schema.WorkflowNodeStatusCompleted,
						IsDynamic:  false,
						AgentName:  "agent1",
						TaskConfig: map[string]any{"param1": "value1"},
						CreatedAt:  baseTime,
						UpdatedAt:  startTime,
					},
					{
						ID:         types.NewID(),
						Name:       "dynamic_node",
						Type:       schema.WorkflowNodeTypeAgent,
						Status:     schema.WorkflowNodeStatusCompleted,
						IsDynamic:  true,
						SpawnedBy:  "static_node",
						AgentName:  "spawned-agent",
						TaskConfig: map[string]any{"spawned": true},
						CreatedAt:  startTime,
						UpdatedAt:  completeTime,
					},
				}

				decisions := []*schema.Decision{
					{
						Iteration:    1,
						Action:       schema.DecisionActionExecuteAgent,
						Reasoning:    "Execute static node",
						Confidence:   0.95,
						TargetNodeID: "static_node",
						Timestamp:    startTime,
					},
				}

				return buildMockSnapshot(mission, nodes, decisions, true)
			},
			validateCycle: func(t *testing.T, original, reconstructed *testWorkflowSnapshot) {
				// Validate mission
				assert.Equal(t, original.Mission.Name, reconstructed.Mission.Name)
				assert.Equal(t, original.Mission.Description, reconstructed.Mission.Description)
				assert.Equal(t, original.Mission.Status, reconstructed.Mission.Status)

				// Validate nodes
				assert.Len(t, reconstructed.Nodes, len(original.Nodes))
				for i := range original.Nodes {
					assert.Equal(t, original.Nodes[i].Name, reconstructed.Nodes[i].Name)
					assert.Equal(t, original.Nodes[i].Type, reconstructed.Nodes[i].Type)
					assert.Equal(t, original.Nodes[i].Status, reconstructed.Nodes[i].Status)
					assert.Equal(t, original.Nodes[i].IsDynamic, reconstructed.Nodes[i].IsDynamic)
					assert.Equal(t, original.Nodes[i].SpawnedBy, reconstructed.Nodes[i].SpawnedBy)
				}

				// Validate decisions
				require.NotNil(t, reconstructed.Decisions)
				assert.Len(t, reconstructed.Decisions, len(original.Decisions))
				for i := range original.Decisions {
					assert.Equal(t, original.Decisions[i].Iteration, reconstructed.Decisions[i].Iteration)
					assert.Equal(t, original.Decisions[i].Action, reconstructed.Decisions[i].Action)
					assert.Equal(t, original.Decisions[i].Confidence, reconstructed.Decisions[i].Confidence)
				}

				// Validate stats
				assert.Equal(t, original.Stats.TotalNodes, reconstructed.Stats.TotalNodes)
				assert.Equal(t, original.Stats.CompletedNodes, reconstructed.Stats.CompletedNodes)
				assert.Equal(t, original.Stats.DynamicNodes, reconstructed.Stats.DynamicNodes)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build original snapshot
			original := tt.buildSnapshot()
			require.NotNil(t, original)

			// Round-trip through YAML
			yamlData, err := yaml.Marshal(original)
			require.NoError(t, err)

			var reconstructedFromYAML testWorkflowSnapshot
			err = yaml.Unmarshal(yamlData, &reconstructedFromYAML)
			require.NoError(t, err)

			tt.validateCycle(t, original, &reconstructedFromYAML)

			// Round-trip through JSON
			jsonData, err := json.Marshal(original)
			require.NoError(t, err)

			var reconstructedFromJSON testWorkflowSnapshot
			err = json.Unmarshal(jsonData, &reconstructedFromJSON)
			require.NoError(t, err)

			tt.validateCycle(t, original, &reconstructedFromJSON)
		})
	}
}

// TestSnapshotEdgeCases tests edge cases and error conditions
func TestSnapshotEdgeCases(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	t.Run("empty nodes", func(t *testing.T) {
		mission := &schema.Mission{
			ID:          types.NewID(),
			Name:        "Empty Workflow",
			Description: "No nodes defined",
			Status:      schema.MissionStatusPending,
			CreatedAt:   baseTime,
		}

		snapshot := buildMockSnapshot(mission, []*schema.WorkflowNode{}, nil, false)
		require.NotNil(t, snapshot)
		assert.Len(t, snapshot.Nodes, 0)
		assert.Equal(t, 0, snapshot.Stats.TotalNodes)
		assert.Equal(t, 0, snapshot.Stats.CompletedNodes)
		assert.Equal(t, 0, snapshot.Stats.DynamicNodes)
	})

	t.Run("no decisions when history not included", func(t *testing.T) {
		mission := &schema.Mission{
			ID:        types.NewID(),
			Name:      "No History",
			Status:    schema.MissionStatusRunning,
			CreatedAt: baseTime,
		}

		decisions := []*schema.Decision{
			{
				Iteration:  1,
				Action:     schema.DecisionActionExecuteAgent,
				Reasoning:  "Should not appear",
				Confidence: 0.9,
				Timestamp:  baseTime,
			},
		}

		snapshot := buildMockSnapshot(mission, []*schema.WorkflowNode{}, decisions, false)
		require.NotNil(t, snapshot)
		assert.Nil(t, snapshot.Decisions, "Decisions should be nil when includeHistory is false")
	})

	t.Run("mission without completion time", func(t *testing.T) {
		startTime := baseTime.Add(5 * time.Minute)
		mission := &schema.Mission{
			ID:        types.NewID(),
			Name:      "In Progress",
			Status:    schema.MissionStatusRunning,
			CreatedAt: baseTime,
			StartedAt: &startTime,
		}

		snapshot := buildMockSnapshot(mission, []*schema.WorkflowNode{}, nil, false)
		require.NotNil(t, snapshot)
		assert.Equal(t, "running", snapshot.Mission.Status)
		assert.NotZero(t, snapshot.Mission.StartedAt)
		assert.Zero(t, snapshot.Mission.CompletedAt, "CompletedAt should be zero for running mission")
		assert.Equal(t, time.Duration(0), snapshot.Stats.Duration, "Duration should be zero for incomplete mission")
	})

	t.Run("all dynamic nodes", func(t *testing.T) {
		mission := &schema.Mission{
			ID:        types.NewID(),
			Name:      "All Dynamic",
			Status:    schema.MissionStatusCompleted,
			CreatedAt: baseTime,
		}

		nodes := []*schema.WorkflowNode{
			{
				ID:        types.NewID(),
				Name:      "dynamic1",
				Type:      schema.WorkflowNodeTypeAgent,
				Status:    schema.WorkflowNodeStatusCompleted,
				IsDynamic: true,
				SpawnedBy: "orchestrator",
				CreatedAt: baseTime,
				UpdatedAt: baseTime,
			},
			{
				ID:        types.NewID(),
				Name:      "dynamic2",
				Type:      schema.WorkflowNodeTypeAgent,
				Status:    schema.WorkflowNodeStatusCompleted,
				IsDynamic: true,
				SpawnedBy: "orchestrator",
				CreatedAt: baseTime,
				UpdatedAt: baseTime,
			},
		}

		snapshot := buildMockSnapshot(mission, nodes, nil, false)
		require.NotNil(t, snapshot)
		assert.Equal(t, 2, snapshot.Stats.TotalNodes)
		assert.Equal(t, 2, snapshot.Stats.DynamicNodes)

		for _, node := range snapshot.Nodes {
			assert.True(t, node.IsDynamic)
			assert.NotEmpty(t, node.SpawnedBy)
		}
	})
}

// buildMockSnapshot is a test helper that simulates buildWorkflowSnapshot
func buildMockSnapshot(mission *schema.Mission, nodes []*schema.WorkflowNode, decisions []*schema.Decision, includeHistory bool) *testWorkflowSnapshot {
	snapshot := &testWorkflowSnapshot{
		Mission: &testMissionSnapshot{
			ID:          mission.ID.String(),
			Name:        mission.Name,
			Description: mission.Description,
			Status:      mission.Status.String(),
			CreatedAt:   mission.CreatedAt,
		},
		Nodes: make([]*testNodeSnapshot, 0, len(nodes)),
		Stats: &SnapshotStats{},
	}

	if mission.StartedAt != nil {
		snapshot.Mission.StartedAt = *mission.StartedAt
	}
	if mission.CompletedAt != nil {
		snapshot.Mission.CompletedAt = *mission.CompletedAt
		if mission.StartedAt != nil {
			snapshot.Stats.Duration = mission.CompletedAt.Sub(*mission.StartedAt)
		}
	}

	// Count stats
	completedCount := 0
	failedCount := 0
	dynamicCount := 0

	// Add nodes
	for _, node := range nodes {
		if node.IsDynamic {
			dynamicCount++
		}
		if node.Status == schema.WorkflowNodeStatusCompleted {
			completedCount++
		}
		if node.Status == schema.WorkflowNodeStatusFailed {
			failedCount++
		}

		nodeSnapshot := &testNodeSnapshot{
			ID:         node.ID.String(),
			Name:       node.Name,
			Type:       node.Type.String(),
			Status:     node.Status.String(),
			IsDynamic:  node.IsDynamic,
			SpawnedBy:  node.SpawnedBy,
			AgentName:  node.AgentName,
			ToolName:   node.ToolName,
			TaskConfig: node.TaskConfig,
			CreatedAt:  node.CreatedAt,
			UpdatedAt:  node.UpdatedAt,
		}

		snapshot.Nodes = append(snapshot.Nodes, nodeSnapshot)
	}

	snapshot.Stats.TotalNodes = len(nodes)
	snapshot.Stats.CompletedNodes = completedCount
	snapshot.Stats.FailedNodes = failedCount
	snapshot.Stats.DynamicNodes = dynamicCount

	// Add decisions if including history
	if includeHistory && len(decisions) > 0 {
		snapshot.Decisions = make([]*DecisionSummary, 0, len(decisions))
		for _, d := range decisions {
			snapshot.Decisions = append(snapshot.Decisions, &DecisionSummary{
				Iteration:    d.Iteration,
				Action:       d.Action.String(),
				Reasoning:    d.Reasoning,
				Confidence:   d.Confidence,
				TargetNodeID: d.TargetNodeID,
				Timestamp:    d.Timestamp,
			})
		}
		snapshot.Stats.TotalDecisions = len(decisions)
	}

	return snapshot
}

// testWorkflowSnapshot types for testing (mirroring cmd/gibson/workflow.go)
type testWorkflowSnapshot struct {
	Mission   *testMissionSnapshot   `json:"mission" yaml:"mission"`
	Nodes     []*testNodeSnapshot    `json:"nodes" yaml:"nodes"`
	Decisions []*DecisionSummary     `json:"decisions,omitempty" yaml:"decisions,omitempty"`
	Stats     *SnapshotStats         `json:"stats" yaml:"stats"`
}

type testMissionSnapshot struct {
	ID          string    `json:"id" yaml:"id"`
	Name        string    `json:"name" yaml:"name"`
	Description string    `json:"description" yaml:"description"`
	Status      string    `json:"status" yaml:"status"`
	CreatedAt   time.Time `json:"created_at" yaml:"created_at"`
	StartedAt   time.Time `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
}

type testNodeSnapshot struct {
	ID          string                 `json:"id" yaml:"id"`
	Name        string                 `json:"name" yaml:"name"`
	Type        string                 `json:"type" yaml:"type"`
	Status      string                 `json:"status" yaml:"status"`
	IsDynamic   bool                   `json:"is_dynamic" yaml:"is_dynamic"`
	SpawnedBy   string                 `json:"spawned_by,omitempty" yaml:"spawned_by,omitempty"`
	AgentName   string                 `json:"agent_name,omitempty" yaml:"agent_name,omitempty"`
	ToolName    string                 `json:"tool_name,omitempty" yaml:"tool_name,omitempty"`
	TaskConfig  map[string]interface{} `json:"task_config,omitempty" yaml:"task_config,omitempty"`
	Executions  int                    `json:"executions,omitempty" yaml:"executions,omitempty"`
	CreatedAt   time.Time              `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" yaml:"updated_at"`
}

type DecisionSummary struct {
	Iteration    int       `json:"iteration" yaml:"iteration"`
	Action       string    `json:"action" yaml:"action"`
	Reasoning    string    `json:"reasoning" yaml:"reasoning"`
	Confidence   float64   `json:"confidence" yaml:"confidence"`
	TargetNodeID string    `json:"target_node_id,omitempty" yaml:"target_node_id,omitempty"`
	Timestamp    time.Time `json:"timestamp" yaml:"timestamp"`
}

type SnapshotStats struct {
	TotalNodes      int           `json:"total_nodes" yaml:"total_nodes"`
	CompletedNodes  int           `json:"completed_nodes" yaml:"completed_nodes"`
	FailedNodes     int           `json:"failed_nodes" yaml:"failed_nodes"`
	DynamicNodes    int           `json:"dynamic_nodes" yaml:"dynamic_nodes"`
	TotalDecisions  int           `json:"total_decisions" yaml:"total_decisions"`
	TotalExecutions int           `json:"total_executions" yaml:"total_executions"`
	Duration        time.Duration `json:"duration,omitempty" yaml:"duration,omitempty"`
}
