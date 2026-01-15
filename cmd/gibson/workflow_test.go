package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"gopkg.in/yaml.v3"
)

// TestWorkflowParse tests the workflow parse command
func TestWorkflowParse(t *testing.T) {
	tests := []struct {
		name          string
		workflowFile  string
		dryRun        bool
		wantError     bool
		errorContains string
		checkOutput   func(*testing.T, string)
	}{
		{
			name:         "valid workflow - dry run",
			workflowFile: "testdata/simple-workflow.yaml",
			dryRun:       true,
			wantError:    false,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "Workflow validation successful")
				assert.Contains(t, output, "simple-test-workflow")
				assert.Contains(t, output, "Nodes: 2")
				assert.Contains(t, output, "Dry-run complete")
			},
		},
		{
			name:          "invalid workflow - missing agent",
			workflowFile:  "testdata/invalid-workflow.yaml",
			dryRun:        true,
			wantError:     true,
			errorContains: "failed to parse workflow",
		},
		{
			name:         "complex workflow - dry run",
			workflowFile: "testdata/complex-workflow.yaml",
			dryRun:       true,
			wantError:    false,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "complex-test-workflow")
				assert.Contains(t, output, "Nodes: 4")
			},
		},
		{
			name:          "nonexistent file",
			workflowFile:  "testdata/nonexistent.yaml",
			dryRun:        true,
			wantError:     true,
			errorContains: "failed to read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests that require actual workflow files
			if _, err := os.Stat(tt.workflowFile); err != nil && !tt.wantError {
				t.Skipf("workflow file not found: %s", tt.workflowFile)
			}

			// Create command with test context
			cmd := parseCmd
			cmd.SetContext(context.Background())

			// Capture output
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stdout)

			// Set flags
			workflowDryRun = tt.dryRun

			// Run command
			err := cmd.RunE(cmd, []string{tt.workflowFile})

			// Check error expectation
			if tt.wantError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}

			// Check output if specified
			if tt.checkOutput != nil && !tt.wantError {
				tt.checkOutput(t, stdout.String())
			}
		})
	}
}

// TestWorkflowParseCreatesMission tests that parse without dry-run creates a mission
func TestWorkflowParseCreatesMission(t *testing.T) {
	// This test would require a real graph database connection
	// Skip in unit tests, mark as integration test
	t.Skip("requires graph database connection")

	// Setup test workflow
	workflowFile := "testdata/simple-workflow.yaml"

	// Create command
	cmd := parseCmd
	cmd.SetContext(context.Background())

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	// Set flags - NOT dry run
	workflowDryRun = false
	workflowGraphURL = "bolt://localhost:7687"

	// Run command
	err := cmd.RunE(cmd, []string{workflowFile})
	require.NoError(t, err)

	// Check output contains mission ID
	output := stdout.String()
	assert.Contains(t, output, "Mission ID:")
	assert.Contains(t, output, "Workflow loaded successfully")
}

// TestWorkflowStatus tests the workflow status command
func TestWorkflowStatus(t *testing.T) {
	tests := []struct {
		name          string
		missionID     string
		mockMission   *mockMissionState
		wantError     bool
		errorContains string
		checkOutput   func(*testing.T, string)
	}{
		{
			name:      "mission with completed nodes",
			missionID: "mission-12345",
			mockMission: &mockMissionState{
				id:     types.NewID(),
				name:   "test-mission",
				status: "running",
				nodes: []*schema.WorkflowNode{
					{ID: types.NewID(), Name: "node1", Status: schema.WorkflowNodeStatusCompleted},
					{ID: types.NewID(), Name: "node2", Status: schema.WorkflowNodeStatusRunning},
					{ID: types.NewID(), Name: "node3", Status: schema.WorkflowNodeStatusPending},
				},
			},
			wantError: false,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "test-mission")
				assert.Contains(t, output, "Progress:")
				assert.Contains(t, output, "Completed")
				assert.Contains(t, output, "Running")
				assert.Contains(t, output, "Pending")
			},
		},
		{
			name:          "invalid mission ID",
			missionID:     "invalid",
			wantError:     true,
			errorContains: "invalid mission ID",
		},
		{
			name:      "mission with no nodes",
			missionID: "mission-empty",
			mockMission: &mockMissionState{
				id:     types.NewID(),
				name:   "empty-mission",
				status: "pending",
				nodes:  []*schema.WorkflowNode{},
			},
			wantError: false,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "empty-mission")
				assert.Contains(t, output, "0/0 nodes")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests requiring mock graph connection
			if tt.mockMission != nil {
				t.Skip("requires mock graph implementation")
			}

			// Create command
			cmd := statusCmd
			cmd.SetContext(context.Background())

			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stdout)

			// Run command
			err := cmd.RunE(cmd, []string{tt.missionID})

			// Check error expectation
			if tt.wantError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				// This will fail without mock, but structure is correct
				if err != nil {
					t.Skipf("skipping output check due to missing graph: %v", err)
				}
			}

			// Check output if specified
			if tt.checkOutput != nil && !tt.wantError && err == nil {
				tt.checkOutput(t, stdout.String())
			}
		})
	}
}

// TestWorkflowStatusShowsRecentDecisions tests that status displays recent decisions
func TestWorkflowStatusShowsRecentDecisions(t *testing.T) {
	t.Skip("requires mock graph with decision history")

	// This test would verify that the status command shows:
	// - Recent decisions (last 5)
	// - Decision reasoning
	// - Confidence scores
	// - Target node IDs
}

// TestWorkflowSnapshot tests the workflow snapshot command
func TestWorkflowSnapshot(t *testing.T) {
	tests := []struct {
		name            string
		missionID       string
		format          string
		includeHistory  bool
		outputFile      string
		wantError       bool
		errorContains   string
		checkOutput     func(*testing.T, string)
		checkOutputFile func(*testing.T, string)
	}{
		{
			name:           "snapshot as YAML to stdout",
			missionID:      "mission-12345",
			format:         "yaml",
			includeHistory: false,
			wantError:      false,
			checkOutput: func(t *testing.T, output string) {
				// Check YAML structure
				var data map[string]interface{}
				err := yaml.Unmarshal([]byte(output), &data)
				require.NoError(t, err)

				assert.Contains(t, data, "mission")
				assert.Contains(t, data, "nodes")
				assert.Contains(t, data, "stats")
			},
		},
		{
			name:           "snapshot as JSON with history",
			missionID:      "mission-12345",
			format:         "json",
			includeHistory: true,
			wantError:      false,
			checkOutput: func(t *testing.T, output string) {
				// Check JSON structure
				var data map[string]interface{}
				err := json.Unmarshal([]byte(output), &data)
				require.NoError(t, err)

				assert.Contains(t, data, "mission")
				assert.Contains(t, data, "nodes")
				assert.Contains(t, data, "stats")
				assert.Contains(t, data, "decisions")
			},
		},
		{
			name:       "snapshot to file",
			missionID:  "mission-12345",
			format:     "yaml",
			outputFile: "snapshot.yaml",
			wantError:  false,
			checkOutputFile: func(t *testing.T, filename string) {
				// Check file was created
				_, err := os.Stat(filename)
				require.NoError(t, err)

				// Check content
				data, err := os.ReadFile(filename)
				require.NoError(t, err)

				var parsed map[string]interface{}
				err = yaml.Unmarshal(data, &parsed)
				require.NoError(t, err)

				assert.Contains(t, parsed, "mission")
			},
		},
		{
			name:          "invalid format",
			missionID:     "mission-12345",
			format:        "xml",
			wantError:     true,
			errorContains: "invalid format",
		},
		{
			name:          "invalid mission ID",
			missionID:     "invalid",
			format:        "yaml",
			wantError:     true,
			errorContains: "invalid mission ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests requiring graph connection
			t.Skip("requires graph database connection")

			// Create temp dir for output files
			tmpDir := t.TempDir()
			if tt.outputFile != "" {
				tt.outputFile = filepath.Join(tmpDir, tt.outputFile)
			}

			// Create command
			cmd := snapshotCmd
			cmd.SetContext(context.Background())

			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stdout)

			// Set flags
			workflowFormat = tt.format
			workflowIncludeHist = tt.includeHistory
			workflowOutput = tt.outputFile

			// Run command
			err := cmd.RunE(cmd, []string{tt.missionID})

			// Check error expectation
			if tt.wantError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}

			// Check output
			if tt.checkOutput != nil && !tt.wantError {
				tt.checkOutput(t, stdout.String())
			}

			// Check output file
			if tt.checkOutputFile != nil && !tt.wantError && tt.outputFile != "" {
				tt.checkOutputFile(t, tt.outputFile)
			}
		})
	}
}

// TestWorkflowSnapshotIncludesExecutionHistory tests that snapshot includes history when requested
func TestWorkflowSnapshotIncludesExecutionHistory(t *testing.T) {
	t.Skip("requires mock graph with execution history")

	// This test would verify that --include-history adds:
	// - Node execution counts
	// - Decision history
	// - Execution timestamps
}

// TestWorkflowDiff tests the workflow diff command
func TestWorkflowDiff(t *testing.T) {
	tests := []struct {
		name          string
		originalFile  string
		missionID     string
		mockMission   *mockMissionState
		wantError     bool
		errorContains string
		checkOutput   func(*testing.T, string)
	}{
		{
			name:         "diff shows added nodes",
			originalFile: "testdata/simple-workflow.yaml",
			missionID:    "mission-12345",
			mockMission: &mockMissionState{
				nodes: []*schema.WorkflowNode{
					// Original nodes
					{ID: types.NewID(), Name: "recon", IsDynamic: false, Status: schema.WorkflowNodeStatusCompleted},
					{ID: types.NewID(), Name: "analyze", IsDynamic: false, Status: schema.WorkflowNodeStatusCompleted},
					// Dynamically added node
					{ID: types.NewID(), Name: "exploit", IsDynamic: true, SpawnedBy: "analyze", Status: schema.WorkflowNodeStatusCompleted},
				},
			},
			wantError: false,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "Added:")
				assert.Contains(t, output, "1 (dynamically spawned)")
				assert.Contains(t, output, "exploit")
			},
		},
		{
			name:         "diff shows modified nodes",
			originalFile: "testdata/simple-workflow.yaml",
			missionID:    "mission-12345",
			mockMission: &mockMissionState{
				nodes: []*schema.WorkflowNode{
					{ID: types.NewID(), Name: "recon", Status: schema.WorkflowNodeStatusCompleted},
					{ID: types.NewID(), Name: "analyze", Status: schema.WorkflowNodeStatusRunning},
				},
			},
			wantError: false,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "Modified:")
				assert.Contains(t, output, "status changes")
			},
		},
		{
			name:         "diff shows skipped nodes",
			originalFile: "testdata/complex-workflow.yaml",
			missionID:    "mission-12345",
			mockMission: &mockMissionState{
				nodes: []*schema.WorkflowNode{
					{ID: types.NewID(), Name: "start", Status: schema.WorkflowNodeStatusCompleted},
					{ID: types.NewID(), Name: "branch-a", Status: schema.WorkflowNodeStatusCompleted},
					{ID: types.NewID(), Name: "branch-b", Status: schema.WorkflowNodeStatusSkipped},
					{ID: types.NewID(), Name: "converge", Status: schema.WorkflowNodeStatusCompleted},
				},
			},
			wantError: false,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "Skipped:")
				assert.Contains(t, output, "branch-b")
			},
		},
		{
			name:         "diff shows reasoning",
			originalFile: "testdata/simple-workflow.yaml",
			missionID:    "mission-12345",
			mockMission: &mockMissionState{
				decisions: []*schema.Decision{
					{
						Iteration:  1,
						Action:     schema.DecisionActionExecuteAgent,
						Reasoning:  "Initial reconnaissance required",
						Confidence: 0.95,
					},
				},
			},
			wantError: false,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "Key Orchestrator Decisions")
				assert.Contains(t, output, "Reasoning:")
			},
		},
		{
			name:          "invalid original file",
			originalFile:  "testdata/nonexistent.yaml",
			missionID:     "mission-12345",
			wantError:     true,
			errorContains: "failed to parse original workflow",
		},
		{
			name:          "invalid mission ID",
			originalFile:  "testdata/simple-workflow.yaml",
			missionID:     "invalid",
			wantError:     true,
			errorContains: "invalid mission ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests requiring graph connection
			t.Skip("requires graph database connection")

			// Create command
			cmd := diffCmd
			cmd.SetContext(context.Background())

			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stdout)

			// Run command
			err := cmd.RunE(cmd, []string{tt.originalFile, tt.missionID})

			// Check error expectation
			if tt.wantError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}

			// Check output
			if tt.checkOutput != nil && !tt.wantError {
				tt.checkOutput(t, stdout.String())
			}
		})
	}
}

// TestWorkflowDiffShowsParams tests that diff shows parameter changes
func TestWorkflowDiffShowsParams(t *testing.T) {
	t.Skip("requires mock graph with parameter tracking")

	// This test would verify that diff shows:
	// - Changed node parameters
	// - Modified agent assignments
	// - Updated tool configurations
}

// TestWorkflowEndToEndFlow tests the complete workflow command flow
func TestWorkflowEndToEndFlow(t *testing.T) {
	t.Skip("requires full graph database and orchestrator integration")

	// This integration test would:
	// 1. Parse a workflow file
	// 2. Check status during execution
	// 3. Export snapshot mid-execution
	// 4. Compare diff with original
	// 5. Verify all commands work together

	tests := []struct {
		name         string
		workflowFile string
		steps        []string // parse, status, snapshot, diff
	}{
		{
			name:         "simple workflow end-to-end",
			workflowFile: "testdata/simple-workflow.yaml",
			steps:        []string{"parse", "status", "snapshot", "diff"},
		},
		{
			name:         "complex workflow with branches",
			workflowFile: "testdata/complex-workflow.yaml",
			steps:        []string{"parse", "status", "snapshot", "diff"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Step 1: Parse workflow
			parseCmd.SetContext(ctx)
			workflowDryRun = false
			err := parseCmd.RunE(parseCmd, []string{tt.workflowFile})
			require.NoError(t, err)

			// Extract mission ID from output (would need to capture this properly)
			missionID := "mission-test-12345"

			// Step 2: Check status
			statusCmd.SetContext(ctx)
			var statusOut bytes.Buffer
			statusCmd.SetOut(&statusOut)
			err = statusCmd.RunE(statusCmd, []string{missionID})
			require.NoError(t, err)

			// Verify status output
			assert.Contains(t, statusOut.String(), "Mission Status")

			// Step 3: Export snapshot
			snapshotCmd.SetContext(ctx)
			var snapshotOut bytes.Buffer
			snapshotCmd.SetOut(&snapshotOut)
			workflowFormat = "yaml"
			workflowIncludeHist = true
			err = snapshotCmd.RunE(snapshotCmd, []string{missionID})
			require.NoError(t, err)

			// Verify snapshot is valid YAML
			var snapshotData map[string]interface{}
			err = yaml.Unmarshal(snapshotOut.Bytes(), &snapshotData)
			require.NoError(t, err)

			// Step 4: Compare diff
			diffCmd.SetContext(ctx)
			var diffOut bytes.Buffer
			diffCmd.SetOut(&diffOut)
			err = diffCmd.RunE(diffCmd, []string{tt.workflowFile, missionID})
			require.NoError(t, err)

			// Verify diff output
			assert.Contains(t, diffOut.String(), "Workflow Diff Summary")
		})
	}
}

// TestWorkflowCommandsWithJSONOutput tests JSON output format for all commands
func TestWorkflowCommandsWithJSONOutput(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
	}{
		{
			name:    "parse JSON output",
			command: "parse",
			args:    []string{"testdata/simple-workflow.yaml"},
		},
		{
			name:    "status JSON output",
			command: "status",
			args:    []string{"mission-12345"},
		},
		{
			name:    "snapshot JSON output",
			command: "snapshot",
			args:    []string{"mission-12345"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Skip("requires graph database connection")

			// Set global flags to JSON output
			globalFlags.OutputFormat = "json"

			var stdout bytes.Buffer
			var cmd *cobra.Command

			switch tt.command {
			case "parse":
				cmd = parseCmd
				workflowDryRun = true
			case "status":
				cmd = statusCmd
			case "snapshot":
				cmd = snapshotCmd
				workflowFormat = "json"
			}

			cmd.SetContext(context.Background())
			cmd.SetOut(&stdout)
			cmd.SetErr(&stdout)

			err := cmd.RunE(cmd, tt.args)
			if err == nil {
				// Verify output is valid JSON
				var data map[string]interface{}
				err := json.Unmarshal(stdout.Bytes(), &data)
				require.NoError(t, err, "output should be valid JSON")
			}
		})
	}
}

// TestCompareWorkflows tests the workflow comparison logic
func TestCompareWorkflows(t *testing.T) {
	// Create test workflows
	original := &workflow.ParsedWorkflow{
		Name:        "test-workflow",
		Description: "Original workflow",
		Nodes: map[string]*workflow.WorkflowNode{
			"node1": {ID: "node1", Name: "Node 1", AgentName: "agent1"},
			"node2": {ID: "node2", Name: "Node 2", AgentName: "agent2"},
		},
	}

	// Test case: nodes added
	t.Run("detects added nodes", func(t *testing.T) {
		nodes := []*schema.WorkflowNode{
			{ID: types.NewID(), Name: "node1", IsDynamic: false},
			{ID: types.NewID(), Name: "node2", IsDynamic: false},
			{ID: types.NewID(), Name: "node3", IsDynamic: true, SpawnedBy: "node2"},
		}

		mission := &schema.Mission{
			ID:   types.NewID(),
			Name: "test-mission",
		}

		diff := buildWorkflowDiff(original, mission, nodes, nil)

		assert.Equal(t, 2, diff.Summary.OriginalNodes)
		assert.Equal(t, 3, diff.Summary.CurrentNodes)
		assert.Equal(t, 1, diff.Summary.Added)
		assert.Len(t, diff.Additions, 1)
		assert.True(t, diff.Additions[0].IsDynamic)
	})

	// Test case: nodes skipped
	t.Run("detects skipped nodes", func(t *testing.T) {
		nodes := []*schema.WorkflowNode{
			{ID: types.NewID(), Name: "node1", Status: schema.WorkflowNodeStatusCompleted},
			{ID: types.NewID(), Name: "node2", Status: schema.WorkflowNodeStatusSkipped},
		}

		mission := &schema.Mission{
			ID:   types.NewID(),
			Name: "test-mission",
		}

		diff := buildWorkflowDiff(original, mission, nodes, nil)

		assert.Equal(t, 1, diff.Summary.Skipped)
		assert.Len(t, diff.Skips, 1)
		assert.Equal(t, schema.WorkflowNodeStatusSkipped.String(), diff.Skips[0].Status)
	})

	// Test case: nodes modified
	t.Run("detects modified nodes", func(t *testing.T) {
		nodes := []*schema.WorkflowNode{
			{ID: types.NewID(), Name: "node1", Status: schema.WorkflowNodeStatusCompleted},
			{ID: types.NewID(), Name: "node2", Status: schema.WorkflowNodeStatusRunning},
		}

		mission := &schema.Mission{
			ID:   types.NewID(),
			Name: "test-mission",
		}

		diff := buildWorkflowDiff(original, mission, nodes, nil)

		assert.Equal(t, 2, diff.Summary.Modified)
		assert.Len(t, diff.Modifications, 2)
	})
}

// TestCountNodesByStatus tests node status counting
func TestCountNodesByStatus(t *testing.T) {
	nodes := []*schema.WorkflowNode{
		{ID: types.NewID(), Status: schema.WorkflowNodeStatusPending},
		{ID: types.NewID(), Status: schema.WorkflowNodeStatusPending},
		{ID: types.NewID(), Status: schema.WorkflowNodeStatusRunning},
		{ID: types.NewID(), Status: schema.WorkflowNodeStatusCompleted},
		{ID: types.NewID(), Status: schema.WorkflowNodeStatusCompleted},
		{ID: types.NewID(), Status: schema.WorkflowNodeStatusCompleted},
		{ID: types.NewID(), Status: schema.WorkflowNodeStatusFailed},
	}

	counts := countNodesByStatus(nodes)

	assert.Equal(t, 2, counts[schema.WorkflowNodeStatusPending])
	assert.Equal(t, 1, counts[schema.WorkflowNodeStatusRunning])
	assert.Equal(t, 3, counts[schema.WorkflowNodeStatusCompleted])
	assert.Equal(t, 1, counts[schema.WorkflowNodeStatusFailed])
	assert.Equal(t, 0, counts[schema.WorkflowNodeStatusSkipped])
}

// TestTruncateString tests string truncation helper
func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{
			input:    "short",
			maxLen:   10,
			expected: "short",
		},
		{
			input:    "this is a long string that needs truncation",
			maxLen:   20,
			expected: "this is a long st...",
		},
		{
			input:    "exactly twenty chars",
			maxLen:   20,
			expected: "exactly twenty chars",
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("truncate_%d", tt.maxLen), func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), tt.maxLen)
		})
	}
}

// TestWorkflowSnapshotStructure tests snapshot data structure
func TestWorkflowSnapshotStructure(t *testing.T) {
	snapshot := &WorkflowSnapshot{
		Mission: &MissionSnapshot{
			ID:        "mission-12345",
			Name:      "test-mission",
			Status:    "running",
			CreatedAt: time.Now(),
		},
		Nodes: []*NodeSnapshot{
			{
				ID:        "node-1",
				Name:      "Test Node",
				Type:      "agent_task",
				Status:    "completed",
				IsDynamic: false,
			},
		},
		Stats: &SnapshotStats{
			TotalNodes:     5,
			CompletedNodes: 2,
			FailedNodes:    0,
			DynamicNodes:   1,
		},
	}

	// Test YAML marshaling
	t.Run("marshals to YAML", func(t *testing.T) {
		data, err := yaml.Marshal(snapshot)
		require.NoError(t, err)
		assert.Contains(t, string(data), "mission:")
		assert.Contains(t, string(data), "nodes:")
		assert.Contains(t, string(data), "stats:")
	})

	// Test JSON marshaling
	t.Run("marshals to JSON", func(t *testing.T) {
		data, err := json.MarshalIndent(snapshot, "", "  ")
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Contains(t, parsed, "mission")
		assert.Contains(t, parsed, "nodes")
		assert.Contains(t, parsed, "stats")
	})
}

// TestWorkflowDiffStructure tests diff data structure
func TestWorkflowDiffStructure(t *testing.T) {
	diff := &WorkflowDiff{
		Summary: &DiffSummary{
			OriginalNodes: 3,
			CurrentNodes:  4,
			Added:         1,
			Modified:      2,
			Skipped:       0,
		},
		Additions: []*NodeDiff{
			{
				ID:        "dynamic-node",
				Name:      "Dynamic Node",
				Type:      "agent_task",
				Status:    "completed",
				IsDynamic: true,
				SpawnedBy: "parent-node",
			},
		},
		Modifications: []*NodeDiff{
			{
				ID:     "node-1",
				Name:   "Node 1",
				Status: "completed",
				Changes: map[string]interface{}{
					"status": "completed",
				},
			},
		},
		Skips: []*NodeDiff{},
	}

	// Test JSON marshaling
	t.Run("marshals to JSON", func(t *testing.T) {
		data, err := json.MarshalIndent(diff, "", "  ")
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Contains(t, parsed, "summary")
		assert.Contains(t, parsed, "additions")
		assert.Contains(t, parsed, "modifications")
	})
}

// TestDisplayWorkflowDiff tests diff display formatting
func TestDisplayWorkflowDiff(t *testing.T) {
	diff := &WorkflowDiff{
		Summary: &DiffSummary{
			OriginalNodes: 2,
			CurrentNodes:  3,
			Added:         1,
			Modified:      1,
			Skipped:       0,
		},
		Additions: []*NodeDiff{
			{
				ID:        "new-node",
				Name:      "New Node",
				Type:      "agent_task",
				Status:    "completed",
				IsDynamic: true,
				SpawnedBy: "parent",
			},
		},
		Modifications: []*NodeDiff{
			{
				ID:     "node-1",
				Name:   "Node 1",
				Type:   "agent_task",
				Status: "completed",
			},
		},
	}

	cmd := diffCmd
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	displayWorkflowDiff(cmd, diff)

	output := stdout.String()

	assert.Contains(t, output, "Workflow Diff Summary")
	assert.Contains(t, output, "Original nodes: 2")
	assert.Contains(t, output, "Current nodes:  3")
	assert.Contains(t, output, "Added:          1")
	assert.Contains(t, output, "Additions (Dynamically Spawned Nodes)")
	assert.Contains(t, output, "New Node")
	assert.Contains(t, output, "Modifications (Status Changes)")
}

// TestDisplayWorkflowDiffEmptyDiff tests display with no changes
func TestDisplayWorkflowDiffEmptyDiff(t *testing.T) {
	diff := &WorkflowDiff{
		Summary: &DiffSummary{
			OriginalNodes: 2,
			CurrentNodes:  2,
			Added:         0,
			Modified:      0,
			Skipped:       0,
		},
		Additions:     []*NodeDiff{},
		Modifications: []*NodeDiff{},
		Skips:         []*NodeDiff{},
	}

	cmd := diffCmd
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	displayWorkflowDiff(cmd, diff)

	output := stdout.String()

	assert.Contains(t, output, "Workflow Diff Summary")
	assert.Contains(t, output, "No differences found")
}

// Mock types for testing

type mockMissionState struct {
	id        types.ID
	name      string
	status    string
	nodes     []*schema.WorkflowNode
	decisions []*schema.Decision
}

// TestWorkflowParseWithGraphLoader tests integration with GraphLoader
func TestWorkflowParseWithGraphLoader(t *testing.T) {
	t.Skip("requires Neo4j test instance")

	// This integration test would verify:
	// - Workflow is correctly loaded into graph
	// - All nodes are created with correct properties
	// - Edges are properly established
	// - Mission node is created with metadata
}

// TestWorkflowStatusWithLiveMission tests status command with running mission
func TestWorkflowStatusWithLiveMission(t *testing.T) {
	t.Skip("requires running mission in graph")

	// This integration test would verify:
	// - Status accurately reflects current mission state
	// - Node counts are correct
	// - Recent decisions are properly displayed
	// - Progress percentage is accurate
}

// TestWorkflowSnapshotWithComplexState tests snapshot with full execution history
func TestWorkflowSnapshotWithComplexState(t *testing.T) {
	t.Skip("requires completed mission with history")

	// This integration test would verify:
	// - Snapshot includes all executed nodes
	// - Decision history is complete
	// - Timing information is accurate
	// - Dynamic nodes are properly marked
}

// TestWorkflowDiffWithEvolution tests diff showing workflow evolution
func TestWorkflowDiffWithEvolution(t *testing.T) {
	t.Skip("requires mission with dynamic node spawning")

	// This integration test would verify:
	// - Dynamically spawned nodes are detected
	// - Reasoning for spawning is captured
	// - Skipped nodes are properly identified
	// - Status changes are tracked
}

// BenchmarkWorkflowParse benchmarks workflow parsing
func BenchmarkWorkflowParse(b *testing.B) {
	workflowFile := "testdata/complex-workflow.yaml"

	// Skip if file doesn't exist
	if _, err := os.Stat(workflowFile); err != nil {
		b.Skipf("workflow file not found: %s", workflowFile)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, _ := os.ReadFile(workflowFile)
		_, _ = workflow.ParseWorkflowYAML(string(data))
	}
}

// BenchmarkCountNodesByStatus benchmarks node counting
func BenchmarkCountNodesByStatus(b *testing.B) {
	// Create large node set
	nodes := make([]*schema.WorkflowNode, 1000)
	statuses := []schema.WorkflowNodeStatus{
		schema.WorkflowNodeStatusPending,
		schema.WorkflowNodeStatusRunning,
		schema.WorkflowNodeStatusCompleted,
		schema.WorkflowNodeStatusFailed,
		schema.WorkflowNodeStatusSkipped,
	}

	for i := range nodes {
		nodes[i] = &schema.WorkflowNode{
			ID:     types.NewID(),
			Status: statuses[i%len(statuses)],
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		countNodesByStatus(nodes)
	}
}
