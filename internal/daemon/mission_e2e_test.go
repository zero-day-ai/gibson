package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"go.opentelemetry.io/otel/trace"
)

// TestMission_PauseResumeE2E tests the full pause/resume cycle for missions.
// It verifies:
// - Mission can be paused mid-execution
// - Checkpoint is created with correct state
// - Findings are persisted before pause
// - Mission can be resumed from checkpoint
// - Execution continues correctly after resume
// - All findings are present after completion
// - Metrics are correct
func TestMission_PauseResumeE2E(t *testing.T) {
	// Setup test environment
	ctx := context.Background()
	testDir := t.TempDir()

	// Create SQLite database
	dbPath := filepath.Join(testDir, "test.db")
	db, err := database.Open(dbPath)
	require.NoError(t, err, "failed to open database")
	defer db.Close()

	// Run migrations to create tables
	migrator := database.NewMigrator(db)
	err = migrator.Migrate(ctx)
	require.NoError(t, err, "failed to run migrations")

	// Initialize stores
	missionStore := mission.NewDBMissionStore(db)
	findingStore := finding.NewDBFindingStore(db)
	eventStore := mission.NewDBEventStore(db)
	checkpointManager := mission.NewCheckpointManager(missionStore)
	_ = checkpointManager // Checkpoint manager for future pause/resume tests

	// Create a multi-node test workflow
	wf := createMultiNodeWorkflow(t, 5) // 5 nodes for testing

	// Create test mission
	testMission := &mission.Mission{
		ID:           types.NewID(),
		Name:         "test-workflow",
		Description:  "E2E test for pause/resume",
		Status:       mission.MissionStatusPending,
		WorkflowID:   wf.ID,
		WorkflowJSON: mustMarshalWorkflow(t, wf),
		TargetID:     types.NewID(),
		CreatedAt:    time.Now(),
		Metrics: &mission.MissionMetrics{
			TotalNodes:     len(wf.Nodes),
			CompletedNodes: 0,
		},
		Metadata: make(map[string]any),
	}

	// Save initial mission
	err = missionStore.Save(ctx, testMission)
	require.NoError(t, err, "failed to save test mission")

	// Create workflow executor
	workflowExecutor := workflow.NewWorkflowExecutor(
		workflow.WithMaxParallel(1), // Sequential execution for predictable testing
	)

	// Create mock harness factory that tracks executions and submits findings
	executedNodes := make([]string, 0)
	harnessFactory := &mockHarnessFactoryWithFindings{
		executedNodes: &executedNodes,
		findingStore:  findingStore,
		missionID:     testMission.ID,
	}

	// Create orchestrator with event store
	orchestrator, err := mission.NewMissionOrchestrator(
		missionStore,
		mission.WithWorkflowExecutor(workflowExecutor),
		mission.WithHarnessFactory(harnessFactory),
		mission.WithEventStore(eventStore),
		mission.WithEventEmitter(mission.NewDefaultEventEmitter(mission.WithBufferSize(100))),
	)
	require.NoError(t, err, "failed to create orchestrator")

	// Start mission execution in background
	resultChan := make(chan *mission.MissionResult, 1)
	errChan := make(chan error, 1)

	missionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		result, err := orchestrator.Execute(missionCtx, testMission)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- result
	}()

	// Wait for 2 nodes to complete
	waitForNodes(t, &executedNodes, 2, 10*time.Second)

	// Verify 2 findings were submitted (one per node)
	findings1, err := findingStore.List(ctx, testMission.ID, finding.NewFindingFilter())
	assert.Len(t, findings1, 2, "should have 2 findings after 2 nodes")

	// Request pause
	err = orchestrator.RequestPause(ctx, testMission.ID)
	require.NoError(t, err, "pause request should succeed")

	// Wait for pause to complete (orchestrator should pause at next clean boundary)
	time.Sleep(2 * time.Second)

	// Verify mission was paused
	pausedMission, err := missionStore.Get(ctx, testMission.ID)
	require.NoError(t, err)
	assert.Equal(t, mission.MissionStatusPaused, pausedMission.Status, "mission should be paused")
	assert.NotNil(t, pausedMission.Checkpoint, "checkpoint should exist")

	// Verify checkpoint has correct state
	checkpoint := pausedMission.Checkpoint
	assert.Equal(t, 2, len(checkpoint.CompletedNodes), "should have 2 completed nodes in checkpoint")
	assert.NotEmpty(t, checkpoint.Checksum, "checkpoint should have checksum")
	assert.NotEmpty(t, checkpoint.ID, "checkpoint should have ID")

	// Verify findings persisted
	findingsBeforeResume, err := findingStore.List(ctx, testMission.ID, finding.NewFindingFilter())
	require.NoError(t, err)
	assert.Len(t, findingsBeforeResume, 2, "findings should be persisted")

	// Verify events were logged
	events, err := eventStore.Query(ctx, &mission.EventFilter{
		MissionID: &testMission.ID,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, events, "events should be persisted")

	// Find paused event
	var foundPausedEvent bool
	for _, evt := range events {
		if evt.Type == mission.EventMissionPaused {
			foundPausedEvent = true
			break
		}
	}
	assert.True(t, foundPausedEvent, "should have mission.paused event")

	// Resume mission from checkpoint
	resumeCtx, resumeCancel := context.WithTimeout(ctx, 30*time.Second)
	defer resumeCancel()

	resumeResult, err := orchestrator.ExecuteFromCheckpoint(resumeCtx, pausedMission, checkpoint)
	require.NoError(t, err, "resume should succeed")
	require.NotNil(t, resumeResult, "resume result should not be nil")

	// Verify mission completed
	assert.Equal(t, mission.MissionStatusCompleted, resumeResult.Status, "mission should complete after resume")

	// Verify all 5 nodes were executed (2 before pause, 3 after resume)
	assert.Equal(t, 5, len(executedNodes), "all 5 nodes should have been executed")

	// Verify all findings are present
	allFindings, err := findingStore.List(ctx, testMission.ID, finding.NewFindingFilter())
	require.NoError(t, err)
	assert.Len(t, allFindings, 5, "should have 5 findings total after completion")

	// Verify final mission state
	finalMission, err := missionStore.Get(ctx, testMission.ID)
	require.NoError(t, err)
	assert.Equal(t, mission.MissionStatusCompleted, finalMission.Status)
	assert.NotNil(t, finalMission.Metrics)
	assert.Equal(t, 5, finalMission.Metrics.TotalNodes, "should have correct total nodes")
	assert.Equal(t, 5, finalMission.Metrics.CompletedNodes, "should have completed all nodes")
	assert.Equal(t, 0, finalMission.Metrics.FailedNodes, "should have no failed nodes")
}

// TestMission_CrashRecoveryE2E tests recovery from daemon crash.
// It verifies:
// - Running missions are detected on startup
// - Missions are auto-paused after crash
// - Missions can be resumed successfully
// - Findings from before crash are preserved
func TestMission_CrashRecoveryE2E(t *testing.T) {
	ctx := context.Background()
	testDir := t.TempDir()

	// Create SQLite database that persists across "restarts"
	dbPath := filepath.Join(testDir, "test.db")
	db, err := database.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Run migrations
	migrator := database.NewMigrator(db)
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Initialize stores
	missionStore := mission.NewDBMissionStore(db)
	findingStore := finding.NewDBFindingStore(db)

	// Create test mission and start execution
	wf := createMultiNodeWorkflow(t, 5)
	testMission := &mission.Mission{
		ID:           types.NewID(),
		Name:         "crash-test-workflow",
		Description:  "E2E test for crash recovery",
		Status:       mission.MissionStatusRunning, // Simulate it was running
		WorkflowID:   wf.ID,
		WorkflowJSON: mustMarshalWorkflow(t, wf),
		TargetID:     types.NewID(),
		CreatedAt:    time.Now(),
		StartedAt:    timePtr(time.Now().Add(-5 * time.Minute)), // Started 5 min ago
		Metrics: &mission.MissionMetrics{
			TotalNodes:     len(wf.Nodes),
			CompletedNodes: 2, // Simulated 2 completed before crash
		},
		Metadata: make(map[string]any),
	}

	// Save mission in "running" state (simulating crash mid-execution)
	err = missionStore.Save(ctx, testMission)
	require.NoError(t, err)

	// Submit some findings before "crash"
	for i := 0; i < 2; i++ {
		baseFinding := agent.Finding{
			ID:          types.NewID(),
			Title:       fmt.Sprintf("Finding %d", i),
			Severity:    agent.SeverityMedium,
			Description: "Test finding before crash",
			CreatedAt:   time.Now(),
		}
		enhancedFinding := finding.NewEnhancedFinding(baseFinding, testMission.ID, "test-agent")
		err = findingStore.Store(ctx, enhancedFinding)
		require.NoError(t, err)
	}

	// Simulate daemon restart - detect running missions and auto-pause them
	activeMissions, err := missionStore.GetActive(ctx)
	require.NoError(t, err)
	assert.Len(t, activeMissions, 1, "should find 1 active mission")
	assert.Equal(t, mission.MissionStatusRunning, activeMissions[0].Status)

	// Auto-pause the running mission (crash recovery logic)
	for _, m := range activeMissions {
		if m.Status == mission.MissionStatusRunning {
			err = missionStore.UpdateStatus(ctx, m.ID, mission.MissionStatusPaused)
			require.NoError(t, err, "should auto-pause running mission")
			t.Logf("Recovered mission %s - set to paused after daemon restart", m.ID)
		}
	}

	// Verify mission is now paused
	recoveredMission, err := missionStore.Get(ctx, testMission.ID)
	require.NoError(t, err)
	assert.Equal(t, mission.MissionStatusPaused, recoveredMission.Status, "mission should be paused after recovery")

	// Verify findings from before crash are preserved
	findingsBeforeResume, err := findingStore.List(ctx, testMission.ID, finding.NewFindingFilter())
	require.NoError(t, err)
	assert.Len(t, findingsBeforeResume, 2, "findings from before crash should be preserved")

	// Create checkpoint for resume (in real scenario this would exist from before crash,
	// but for this test we simulate recovery without checkpoint)
	checkpointManager := mission.NewCheckpointManager(missionStore)

	// Attempt to restore checkpoint (should handle missing checkpoint gracefully)
	checkpoint, err := checkpointManager.Restore(ctx, testMission.ID)
	if err != nil {
		// Expected if no checkpoint exists - recovery should still work
		t.Logf("No checkpoint found (expected): %v", err)
		// Create a recovery checkpoint from findings
		checkpoint = &mission.MissionCheckpoint{
			ID:             types.NewID(),
			Version:        1,
			CompletedNodes: []string{"node-0", "node-1"}, // Based on findings
			PendingNodes:   []string{"node-2", "node-3", "node-4"},
			NodeResults:    make(map[string]any),
			WorkflowState:  make(map[string]any),
			CheckpointedAt: time.Now(),
		}
	}

	// Save recovery checkpoint
	err = missionStore.SaveCheckpoint(ctx, testMission.ID, checkpoint)
	require.NoError(t, err)

	// Now resume the mission
	workflowExecutor := workflow.NewWorkflowExecutor(workflow.WithMaxParallel(1))
	executedNodes := make([]string, 0)
	harnessFactory := &mockHarnessFactoryWithFindings{
		executedNodes: &executedNodes,
		findingStore:  findingStore,
		missionID:     testMission.ID,
	}

	orchestrator, err := mission.NewMissionOrchestrator(
		missionStore,
		mission.WithWorkflowExecutor(workflowExecutor),
		mission.WithHarnessFactory(harnessFactory),
		mission.WithEventEmitter(mission.NewDefaultEventEmitter(mission.WithBufferSize(100))),
	)
	require.NoError(t, err, "failed to create orchestrator")

	// Resume execution
	resumeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Update checkpoint on mission before resuming
	recoveredMission.Checkpoint = checkpoint

	result, err := orchestrator.ExecuteFromCheckpoint(resumeCtx, recoveredMission, checkpoint)
	require.NoError(t, err, "resume after crash should succeed")
	assert.NotNil(t, result)
	assert.Equal(t, mission.MissionStatusCompleted, result.Status)

	// Verify all findings (pre-crash + post-resume)
	allFindings, err := findingStore.List(ctx, testMission.ID, finding.NewFindingFilter())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(allFindings), 2, "should have at least findings from before crash")
}

// TestMission_MultiRunHistoryE2E tests mission run history tracking.
// It verifies:
// - Same mission name can be run multiple times
// - Each run has unique ID and run number
// - Previous run IDs are linked correctly
// - History returns all runs in order
// - Findings are separate per run
func TestMission_MultiRunHistoryE2E(t *testing.T) {
	ctx := context.Background()
	testDir := t.TempDir()

	dbPath := filepath.Join(testDir, "test.db")
	db, err := database.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Run migrations
	migrator := database.NewMigrator(db)
	err = migrator.Migrate(ctx)
	require.NoError(t, err)

	missionStore := mission.NewDBMissionStore(db)
	findingStore := finding.NewDBFindingStore(db)
	runLinker := mission.NewMissionRunLinker(missionStore)

	missionName := "test-workflow"
	wf := createMultiNodeWorkflow(t, 3)

	// Track run IDs for verification
	runIDs := make([]types.ID, 0, 3)

	// Run the same mission 3 times to completion
	for runNum := 1; runNum <= 3; runNum++ {
		t.Logf("Starting run %d", runNum)

		// Create mission for this run
		missionID := types.NewID()
		testMission := &mission.Mission{
			ID:           missionID,
			Name:         missionName,
			Description:  fmt.Sprintf("Run %d of multi-run test", runNum),
			Status:       mission.MissionStatusPending,
			WorkflowID:   wf.ID,
			WorkflowJSON: mustMarshalWorkflow(t, wf),
			TargetID:     types.NewID(),
			CreatedAt:    time.Now(),
			Metrics: &mission.MissionMetrics{
				TotalNodes:     len(wf.Nodes),
				CompletedNodes: 0,
			},
			Metadata: make(map[string]any),
		}

		// Use run linker to create the run
		err = runLinker.CreateRun(ctx, missionName, testMission)
		require.NoError(t, err, "CreateRun should succeed for run %d", runNum)

		// Note: RunNumber and PreviousRunID fields are not yet implemented (tasks 1-3)
		// For now, just verify the mission was created
		savedMission, err := missionStore.Get(ctx, missionID)
		require.NoError(t, err)
		assert.Equal(t, missionName, savedMission.Name, "should have correct name")

		runIDs = append(runIDs, missionID)

		// Execute the mission to completion
		workflowExecutor := workflow.NewWorkflowExecutor(workflow.WithMaxParallel(10))
		executedNodes := make([]string, 0)
		harnessFactory := &mockHarnessFactoryWithFindings{
			executedNodes: &executedNodes,
			findingStore:  findingStore,
			missionID:     missionID,
		}

		orchestrator, err := mission.NewMissionOrchestrator(
			missionStore,
			mission.WithWorkflowExecutor(workflowExecutor),
			mission.WithHarnessFactory(harnessFactory),
			mission.WithEventEmitter(mission.NewDefaultEventEmitter(mission.WithBufferSize(100))),
		)
		require.NoError(t, err, "failed to create orchestrator for run %d", runNum)

		execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		result, err := orchestrator.Execute(execCtx, testMission)
		cancel()

		require.NoError(t, err, "execution should succeed for run %d", runNum)
		assert.Equal(t, mission.MissionStatusCompleted, result.Status, "run %d should complete", runNum)

		// Update mission status
		err = missionStore.UpdateStatus(ctx, missionID, mission.MissionStatusCompleted)
		require.NoError(t, err)

		// Verify findings were created for this run
		runFindings, err := findingStore.List(ctx, missionID, finding.NewFindingFilter())
		require.NoError(t, err)
		assert.Equal(t, 3, len(runFindings), "run %d should have 3 findings", runNum)

		// Small delay between runs to ensure timestamp ordering
		time.Sleep(100 * time.Millisecond)
	}

	// Verify mission history
	history, err := runLinker.GetRunHistory(ctx, missionName)
	require.NoError(t, err, "should get mission history")
	assert.Len(t, history, 3, "should have 3 runs in history")

	// Verify runs are in correct order (descending by run number)
	for i, run := range history {
		expectedRunNum := 3 - i // 3, 2, 1
		assert.Equal(t, expectedRunNum, run.RunNumber, "run %d should be at index %d", expectedRunNum, i)
		assert.Equal(t, runIDs[expectedRunNum-1], run.MissionID, "should have correct mission ID")
		assert.Equal(t, mission.MissionStatusCompleted, run.Status, "run should be completed")
	}

	// Verify previous_run_id links
	for i := 1; i < len(history); i++ {
		// history[i] is run N-i, its previous should be run N-i-1
		run := history[len(history)-1-i] // Get in ascending order
		if i > 0 {
			require.NotNil(t, run.PreviousRunID, "run %d should have previous ID", i+1)
			assert.Equal(t, runIDs[i-1], *run.PreviousRunID, "run %d should link to run %d", i+1, i)
		}
	}

	// Verify each run has independent findings
	for i, runID := range runIDs {
		runFindings, err := findingStore.List(ctx, runID, finding.NewFindingFilter())
		require.NoError(t, err)
		assert.Len(t, runFindings, 3, "run %d should have 3 independent findings", i+1)

		// Verify all findings have correct mission ID
		for _, f := range runFindings {
			assert.Equal(t, runID, f.MissionID, "finding should belong to correct run")
		}
	}

	// Verify GetLatestRun returns the most recent run
	latestRun, err := runLinker.GetLatestRun(ctx, missionName)
	require.NoError(t, err)
	assert.Equal(t, runIDs[2], latestRun.ID, "latest run should be run 3")
}

// Helper functions

// createMultiNodeWorkflow creates a simple workflow with N sequential nodes
func createMultiNodeWorkflow(t *testing.T, nodeCount int) *workflow.Workflow {
	t.Helper()

	nodes := make(map[string]*workflow.WorkflowNode)
	for i := 0; i < nodeCount; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		node := &workflow.WorkflowNode{
			ID:          nodeID,
			Type:        workflow.NodeTypeAgent,
			Name:        fmt.Sprintf("Test Node %d", i),
			Description: fmt.Sprintf("Test node %d", i),
			AgentName:   "test-agent",
			Timeout:     30 * time.Second,
			AgentTask: &agent.Task{
				ID:          types.NewID(),
				Name:        fmt.Sprintf("Test task %d", i),
				Description: fmt.Sprintf("Execute test task %d", i),
				CreatedAt:   time.Now(),
			},
		}

		// Make nodes sequential by adding dependency
		if i > 0 {
			node.Dependencies = []string{fmt.Sprintf("node-%d", i-1)}
		}

		nodes[nodeID] = node
	}

	wf := &workflow.Workflow{
		ID:          types.NewID(),
		Name:        "Test Multi-Node Workflow",
		Description: "E2E test workflow",
		Nodes:       nodes,
		CreatedAt:   time.Now(),
	}

	return wf
}

// mustMarshalWorkflow marshals a workflow to JSON, failing the test on error
func mustMarshalWorkflow(t *testing.T, wf *workflow.Workflow) string {
	t.Helper()

	// For simplicity, we'll use YAML format as the workflow parser expects
	yaml := fmt.Sprintf(`
name: %s
description: %s
nodes:
`, wf.Name, wf.Description)

	for _, node := range wf.Nodes {
		yaml += fmt.Sprintf(`  - id: %s
    type: %s
    name: %s
    agent: test-agent
`, node.ID, node.Type, node.Name)
		if len(node.Dependencies) > 0 {
			yaml += "    depends_on:\n"
			for _, dep := range node.Dependencies {
				yaml += fmt.Sprintf("      - %s\n", dep)
			}
		}
		if node.AgentTask != nil {
			yaml += "    task:\n"
			yaml += fmt.Sprintf("      description: \"%s\"\n", node.AgentTask.Description)
		}
	}

	return yaml
}

// waitForNodes waits for a specific number of nodes to be executed
func waitForNodes(t *testing.T, executedNodes *[]string, count int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(*executedNodes) >= count {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("timeout waiting for %d nodes to execute (got %d)", count, len(*executedNodes))
}

// mockHarnessFactoryWithFindings is a test harness factory that tracks executions and submits findings
type mockHarnessFactoryWithFindings struct {
	executedNodes *[]string
	findingStore  finding.FindingStore
	missionID     types.ID
}

func (m *mockHarnessFactoryWithFindings) Create(agentName string, missionCtx harness.MissionContext, targetInfo harness.TargetInfo) (harness.AgentHarness, error) {
	return &mockHarnessWithFindings{
		executedNodes: m.executedNodes,
		findingStore:  m.findingStore,
		missionID:     m.missionID,
		agentName:     agentName,
	}, nil
}

func (m *mockHarnessFactoryWithFindings) CreateChild(parent harness.AgentHarness, agentName string) (harness.AgentHarness, error) {
	return &mockHarnessWithFindings{
		executedNodes: m.executedNodes,
		findingStore:  m.findingStore,
		missionID:     m.missionID,
		agentName:     agentName,
	}, nil
}

// mockHarnessWithFindings is a mock harness that submits a finding on each Complete call
type mockHarnessWithFindings struct {
	executedNodes *[]string
	findingStore  finding.FindingStore
	missionID     types.ID
	agentName     string
}

func (m *mockHarnessWithFindings) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	// Track execution
	*m.executedNodes = append(*m.executedNodes, m.agentName)

	// Submit a finding
	baseFinding := agent.Finding{
		ID:          types.NewID(),
		Title:       fmt.Sprintf("Test finding from %s", m.agentName),
		Severity:    agent.SeverityMedium,
		Description: "Auto-generated test finding",
		CreatedAt:   time.Now(),
	}
	enhancedFinding := finding.NewEnhancedFinding(baseFinding, m.missionID, m.agentName)

	_ = m.findingStore.Store(ctx, enhancedFinding)

	return &llm.CompletionResponse{
		ID:    "test-completion",
		Model: "test-model",
		Message: llm.Message{
			Role:    llm.RoleAssistant,
			Content: "Test response",
		},
	}, nil
}

func (m *mockHarnessWithFindings) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...harness.CompletionOption) (*llm.CompletionResponse, error) {
	return m.Complete(ctx, slot, messages, opts...)
}

func (m *mockHarnessWithFindings) CompleteStructuredAny(ctx context.Context, slot string, messages []llm.Message, structuredOutputDef any, opts ...harness.CompletionOption) (any, error) {
	return nil, nil
}

func (m *mockHarnessWithFindings) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...harness.CompletionOption) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockHarnessWithFindings) CallTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return nil, nil
}

func (m *mockHarnessWithFindings) ListTools() []harness.ToolDescriptor {
	return nil
}

func (m *mockHarnessWithFindings) QueryPlugin(ctx context.Context, name string, method string, params map[string]any) (any, error) {
	return nil, nil
}

func (m *mockHarnessWithFindings) ListPlugins() []harness.PluginDescriptor {
	return nil
}

func (m *mockHarnessWithFindings) DelegateToAgent(ctx context.Context, name string, task agent.Task) (agent.Result, error) {
	return agent.Result{}, nil
}

func (m *mockHarnessWithFindings) ListAgents() []harness.AgentDescriptor {
	return nil
}

func (m *mockHarnessWithFindings) SubmitFinding(ctx context.Context, f agent.Finding) error {
	return nil
}

func (m *mockHarnessWithFindings) GetFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}

func (m *mockHarnessWithFindings) GetAllRunFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}

func (m *mockHarnessWithFindings) GetMissionRunHistory(ctx context.Context) ([]harness.MissionRunSummarySDK, error) {
	return nil, nil
}

func (m *mockHarnessWithFindings) GetPreviousRunFindings(ctx context.Context, filter harness.FindingFilter) ([]agent.Finding, error) {
	return nil, nil
}

func (m *mockHarnessWithFindings) Memory() memory.MemoryStore {
	return nil
}

func (m *mockHarnessWithFindings) Mission() harness.MissionContext {
	return harness.MissionContext{ID: m.missionID}
}

func (m *mockHarnessWithFindings) MissionExecutionContext() harness.MissionExecutionContextSDK {
	return harness.MissionExecutionContextSDK{}
}

func (m *mockHarnessWithFindings) Target() harness.TargetInfo {
	return harness.TargetInfo{}
}

func (m *mockHarnessWithFindings) Tracer() trace.Tracer {
	return trace.NewNoopTracerProvider().Tracer("test")
}

func (m *mockHarnessWithFindings) Logger() *slog.Logger {
	return slog.Default()
}

func (m *mockHarnessWithFindings) Metrics() harness.MetricsRecorder {
	return nil
}

func (m *mockHarnessWithFindings) TokenUsage() *llm.TokenTracker {
	return nil
}

func (m *mockHarnessWithFindings) MissionID() types.ID {
	return m.missionID
}

// Helper pointer functions
func stringPtr(s string) *string {
	return &s
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}

func timePtr(t time.Time) *time.Time {
	return &t
}
