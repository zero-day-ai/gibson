//go:build skip_old_tests
// +build skip_old_tests

// NOTE: This file contains tests for the old workflow-based API which has been removed.
// These tests need to be rewritten for the new mission definition API.
// Use -tags=skip_old_tests to run these (they will fail).

package mission

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// setupIntegrationTestDB creates a temporary SQLite database for testing
func setupIntegrationTestDB(t *testing.T) *database.DB {
	t.Helper()

	// Create temporary file for database
	tmpFile, err := os.CreateTemp("", "mission_service_test_*.db")
	require.NoError(t, err, "failed to create temp file")
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Cleanup on test completion
	t.Cleanup(func() {
		os.Remove(tmpPath)
		os.Remove(tmpPath + "-wal")
		os.Remove(tmpPath + "-shm")
	})

	// Create database
	db, err := database.Open(tmpPath)
	require.NoError(t, err, "failed to create database")

	// Run migrations - skip test if FTS5 not available
	err = db.InitSchema()
	if err != nil && strings.Contains(err.Error(), "no such module: fts5") {
		t.Skip("Skipping test: FTS5 module not available in SQLite")
	}
	require.NoError(t, err, "failed to run migrations")

	return db
}

// integrationWorkflowStore is a test implementation of WorkflowStore for integration tests
type integrationWorkflowStore struct {
	workflows map[types.ID]interface{}
}

// newIntegrationWorkflowStore creates a new mock workflow store for integration tests
func newIntegrationWorkflowStore() *integrationWorkflowStore {
	return &integrationWorkflowStore{
		workflows: make(map[types.ID]interface{}),
	}
}

// Get retrieves a workflow by ID
func (m *integrationWorkflowStore) Get(ctx context.Context, id types.ID) (interface{}, error) {
	wf, ok := m.workflows[id]
	if !ok {
		return nil, NewNotFoundError(id.String())
	}
	return wf, nil
}

// Add stores a workflow (helper for testing)
func (m *integrationWorkflowStore) Add(id types.ID, wf interface{}) {
	m.workflows[id] = wf
}

// integrationFindingStore is a test implementation of FindingStore for integration tests
type integrationFindingStore struct {
	findings       map[types.ID][]interface{}
	severityCounts map[types.ID]map[string]int
}

// newIntegrationFindingStore creates a new mock finding store for integration tests
func newIntegrationFindingStore() *integrationFindingStore {
	return &integrationFindingStore{
		findings:       make(map[types.ID][]interface{}),
		severityCounts: make(map[types.ID]map[string]int),
	}
}

// GetByMission retrieves all findings for a mission
func (m *integrationFindingStore) GetByMission(ctx context.Context, missionID types.ID) ([]interface{}, error) {
	findings, ok := m.findings[missionID]
	if !ok {
		return []interface{}{}, nil
	}
	return findings, nil
}

// CountBySeverity returns finding counts grouped by severity
func (m *integrationFindingStore) CountBySeverity(ctx context.Context, missionID types.ID) (map[string]int, error) {
	counts, ok := m.severityCounts[missionID]
	if !ok {
		return make(map[string]int), nil
	}
	return counts, nil
}

// AddFinding adds a finding to the store (helper for testing)
func (m *integrationFindingStore) AddFinding(missionID types.ID, finding interface{}) {
	m.findings[missionID] = append(m.findings[missionID], finding)
}

// SetSeverityCounts sets the severity counts for a mission (helper for testing)
func (m *integrationFindingStore) SetSeverityCounts(missionID types.ID, counts map[string]int) {
	m.severityCounts[missionID] = counts
}

// createIntegrationTestWorkflow creates a test workflow for integration tests
func createIntegrationTestWorkflow() *mockWorkflow {
	return &mockWorkflow{
		ID:          types.NewID(),
		Name:        "Test Workflow",
		Description: "A test workflow for integration testing",
		Nodes: map[string]*mockWorkflowNode{
			"node1": {
				ID:   "node1",
				Name: "Test Node 1",
				Type: mockNodeTypeAgent,
			},
			"node2": {
				ID:   "node2",
				Name: "Test Node 2",
				Type: mockNodeTypeTool,
			},
		},
		Edges: []mockWorkflowEdge{
			{From: "node1", To: "node2"},
		},
		EntryPoints: []string{"node1"},
		ExitPoints:  []string{"node2"},
		Metadata:    make(map[string]any),
		CreatedAt:   time.Now(),
	}
}

// createIntegrationTestMission creates a test mission for integration tests
func createIntegrationTestMission() *Mission {
	now := time.Now()
	return &Mission{
		ID:          types.NewID(),
		Name:        "Test Mission",
		Description: "A test mission for integration testing",
		Status:      MissionStatusPending,
		TargetID:    types.NewID(),
		WorkflowID:  types.NewID(),
		Progress:    0.0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// TestMissionServiceLoadWorkflow_FromStore tests loading workflow from store by ID
func TestMissionServiceLoadWorkflow_FromStore(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Create and add a test workflow to the store
	testWorkflow := createIntegrationTestWorkflow()
	workflowStore.Add(testWorkflow.ID, testWorkflow)

	// Create workflow config referencing the stored workflow
	config := &MissionWorkflowConfig{
		Reference: testWorkflow.ID.String(),
	}

	// Load workflow from store
	ctx := context.Background()
	loaded, err := service.LoadWorkflow(ctx, config)
	require.NoError(t, err, "LoadWorkflow should succeed")
	require.NotNil(t, loaded, "Loaded workflow should not be nil")

	// Verify it's the same workflow
	loadedWorkflow, ok := loaded.(*mockWorkflow)
	require.True(t, ok, "Loaded workflow should be *mockWorkflow type")
	assert.Equal(t, testWorkflow.ID, loadedWorkflow.ID, "Workflow ID should match")
	assert.Equal(t, testWorkflow.Name, loadedWorkflow.Name, "Workflow name should match")
}

// TestMissionServiceLoadWorkflow_InlineYAML tests parsing inline YAML workflow definition
func TestMissionServiceLoadWorkflow_InlineYAML(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Create inline workflow definition
	inlineWorkflow := &InlineWorkflowConfig{
		Agents: []string{"recon-agent", "exploit-agent"},
		Phases: []struct {
			Name   string   `yaml:"name"`
			Agents []string `yaml:"agents"`
		}{
			{
				Name:   "reconnaissance",
				Agents: []string{"recon-agent"},
			},
			{
				Name:   "exploitation",
				Agents: []string{"exploit-agent"},
			},
		},
	}

	// Create workflow config with inline definition
	config := &MissionWorkflowConfig{
		Inline: inlineWorkflow,
	}

	// Load inline workflow
	ctx := context.Background()
	loaded, err := service.LoadWorkflow(ctx, config)
	require.NoError(t, err, "LoadWorkflow should succeed with inline definition")
	require.NotNil(t, loaded, "Loaded workflow should not be nil")

	// Verify it's the inline workflow
	loadedInline, ok := loaded.(*InlineWorkflowConfig)
	require.True(t, ok, "Loaded workflow should be *InlineWorkflowConfig type")
	assert.Equal(t, len(inlineWorkflow.Agents), len(loadedInline.Agents), "Agent count should match")
	assert.Equal(t, inlineWorkflow.Agents[0], loadedInline.Agents[0], "First agent should match")
}

// TestMissionServiceLoadWorkflow_WorkflowNotFound tests error handling when workflow not found
func TestMissionServiceLoadWorkflow_WorkflowNotFound(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Create workflow config referencing a non-existent workflow
	nonExistentID := types.NewID()
	config := &MissionWorkflowConfig{
		Reference: nonExistentID.String(),
	}

	// Attempt to load non-existent workflow
	ctx := context.Background()
	loaded, err := service.LoadWorkflow(ctx, config)
	assert.Error(t, err, "LoadWorkflow should fail for non-existent workflow")
	assert.Nil(t, loaded, "Loaded workflow should be nil on error")
	assert.Contains(t, err.Error(), "failed to load workflow", "Error should indicate load failure")
}

// TestMissionServiceLoadWorkflow_NilConfig tests error handling with nil config
func TestMissionServiceLoadWorkflow_NilConfig(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Attempt to load with nil config
	ctx := context.Background()
	loaded, err := service.LoadWorkflow(ctx, nil)
	assert.Error(t, err, "LoadWorkflow should fail with nil config")
	assert.Nil(t, loaded, "Loaded workflow should be nil on error")
	assert.Contains(t, err.Error(), "workflow config is required", "Error should indicate nil config")
}

// TestMissionServiceLoadWorkflow_NoReferenceOrInline tests error handling when neither reference nor inline is provided
func TestMissionServiceLoadWorkflow_NoReferenceOrInline(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Create empty workflow config
	config := &MissionWorkflowConfig{}

	// Attempt to load workflow with empty config
	ctx := context.Background()
	loaded, err := service.LoadWorkflow(ctx, config)
	assert.Error(t, err, "LoadWorkflow should fail when neither reference nor inline is provided")
	assert.Nil(t, loaded, "Loaded workflow should be nil on error")
	assert.Contains(t, err.Error(), "must specify either 'reference' or 'inline'", "Error should indicate missing fields")
}

// TestMissionServiceAggregateFindings_WithFindings tests aggregating findings from store
func TestMissionServiceAggregateFindings_WithFindings(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Create test mission
	missionID := types.NewID()

	// Add test findings to the store
	finding1 := map[string]interface{}{
		"id":       types.NewID().String(),
		"title":    "SQL Injection",
		"severity": "critical",
	}
	finding2 := map[string]interface{}{
		"id":       types.NewID().String(),
		"title":    "XSS Vulnerability",
		"severity": "high",
	}
	findingStore.AddFinding(missionID, finding1)
	findingStore.AddFinding(missionID, finding2)

	// Aggregate findings
	ctx := context.Background()
	findings, err := service.AggregateFindings(ctx, missionID)
	require.NoError(t, err, "AggregateFindings should succeed")
	require.NotNil(t, findings, "Findings should not be nil")
	assert.Len(t, findings, 2, "Should return 2 findings")

	// Verify finding content
	firstFinding, ok := findings[0].(map[string]interface{})
	require.True(t, ok, "Finding should be map[string]interface{}")
	assert.Equal(t, "SQL Injection", firstFinding["title"], "First finding title should match")
}

// TestMissionServiceAggregateFindings_Empty tests aggregating when no findings exist
func TestMissionServiceAggregateFindings_Empty(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Create mission ID with no findings
	missionID := types.NewID()

	// Aggregate findings (should return empty slice)
	ctx := context.Background()
	findings, err := service.AggregateFindings(ctx, missionID)
	require.NoError(t, err, "AggregateFindings should succeed even with no findings")
	require.NotNil(t, findings, "Findings should not be nil")
	assert.Len(t, findings, 0, "Should return empty findings slice")
}

// TestMissionServiceAggregateFindings_ZeroMissionID tests error handling with zero mission ID
func TestMissionServiceAggregateFindings_ZeroMissionID(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Attempt to aggregate with zero mission ID
	ctx := context.Background()
	findings, err := service.AggregateFindings(ctx, types.ID(""))
	assert.Error(t, err, "AggregateFindings should fail with zero mission ID")
	assert.Nil(t, findings, "Findings should be nil on error")
	assert.Contains(t, err.Error(), "mission ID is required", "Error should indicate missing mission ID")
}

// TestMissionServiceGetSummary_WithFindings tests getting mission summary with findings
func TestMissionServiceGetSummary_WithFindings(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Create and save test mission
	mission := createIntegrationTestMission()
	ctx := context.Background()
	err := missionStore.Save(ctx, mission)
	require.NoError(t, err, "Failed to save mission")

	// Set up finding counts by severity
	severityCounts := map[string]int{
		string(agent.SeverityCritical): 2,
		string(agent.SeverityHigh):     5,
		string(agent.SeverityMedium):   3,
		string(agent.SeverityLow):      1,
	}
	findingStore.SetSeverityCounts(mission.ID, severityCounts)

	// Get mission summary
	summary, err := service.GetSummary(ctx, mission.ID)
	require.NoError(t, err, "GetSummary should succeed")
	require.NotNil(t, summary, "Summary should not be nil")

	// Verify summary contents
	assert.Equal(t, mission.ID, summary.Mission.ID, "Mission ID should match")
	assert.Equal(t, mission.Name, summary.Mission.Name, "Mission name should match")
	assert.Equal(t, 11, summary.FindingsCount, "Total findings count should be 11")
	assert.Equal(t, 2, summary.FindingsByLevel[string(agent.SeverityCritical)], "Critical findings count should be 2")
	assert.Equal(t, 5, summary.FindingsByLevel[string(agent.SeverityHigh)], "High findings count should be 5")
	assert.Equal(t, 3, summary.FindingsByLevel[string(agent.SeverityMedium)], "Medium findings count should be 3")
	assert.Equal(t, 1, summary.FindingsByLevel[string(agent.SeverityLow)], "Low findings count should be 1")
	assert.NotNil(t, summary.Progress, "Progress should not be nil")
}

// TestMissionServiceGetSummary_NoFindings tests getting summary when no findings exist
func TestMissionServiceGetSummary_NoFindings(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Create and save test mission
	mission := createIntegrationTestMission()
	ctx := context.Background()
	err := missionStore.Save(ctx, mission)
	require.NoError(t, err, "Failed to save mission")

	// Get mission summary (no findings in store)
	summary, err := service.GetSummary(ctx, mission.ID)
	require.NoError(t, err, "GetSummary should succeed even with no findings")
	require.NotNil(t, summary, "Summary should not be nil")

	// Verify summary contents
	assert.Equal(t, mission.ID, summary.Mission.ID, "Mission ID should match")
	assert.Equal(t, 0, summary.FindingsCount, "Findings count should be 0")
	assert.NotNil(t, summary.FindingsByLevel, "FindingsByLevel should not be nil")
	assert.Len(t, summary.FindingsByLevel, 0, "FindingsByLevel should be empty")
}

// TestMissionServiceGetSummary_MissionNotFound tests error handling when mission not found
func TestMissionServiceGetSummary_MissionNotFound(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Attempt to get summary for non-existent mission
	nonExistentID := types.NewID()
	ctx := context.Background()
	summary, err := service.GetSummary(ctx, nonExistentID)
	assert.Error(t, err, "GetSummary should fail for non-existent mission")
	assert.Nil(t, summary, "Summary should be nil on error")
	assert.Contains(t, err.Error(), "failed to retrieve mission", "Error should indicate retrieval failure")
}

// TestMissionServiceValidateMission_Valid tests validation of a valid mission
func TestMissionServiceValidateMission_Valid(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Create valid mission with workflow in store
	mission := createIntegrationTestMission()
	workflow := createIntegrationTestWorkflow()
	mission.WorkflowID = workflow.ID
	workflowStore.Add(workflow.ID, workflow)

	// Add valid constraints
	mission.Constraints = &MissionConstraints{
		MaxDuration: 1 * time.Hour,
		MaxFindings: 100,
		MaxCost:     50.0,
		MaxTokens:   100000,
	}

	// Validate mission
	ctx := context.Background()
	err := service.ValidateMission(ctx, mission)
	assert.NoError(t, err, "ValidateMission should succeed for valid mission")
}

// TestMissionServiceValidateMission_InvalidWorkflowID tests validation failure with non-existent workflow
func TestMissionServiceValidateMission_InvalidWorkflowID(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Create mission with non-existent workflow ID
	mission := createIntegrationTestMission()
	mission.WorkflowID = types.NewID() // Not in store

	// Validate mission
	ctx := context.Background()
	err := service.ValidateMission(ctx, mission)
	assert.Error(t, err, "ValidateMission should fail when workflow doesn't exist")
	assert.Contains(t, err.Error(), "workflow validation failed", "Error should indicate workflow validation failure")
}

// TestMissionServiceValidateMission_InvalidConstraints tests validation failure with invalid constraints
func TestMissionServiceValidateMission_InvalidConstraints(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	testCases := []struct {
		name        string
		constraints *MissionConstraints
		errorMatch  string
	}{
		{
			name: "max_duration too short",
			constraints: &MissionConstraints{
				MaxDuration: 30 * time.Second, // Less than 1 minute
			},
			errorMatch: "max_duration too short",
		},
		{
			name: "max_findings zero",
			constraints: &MissionConstraints{
				MaxFindings: 0, // Must be at least 1 if set
			},
			errorMatch: "max_findings must be at least 1",
		},
		{
			name: "max_cost too low",
			constraints: &MissionConstraints{
				MaxCost: 0.005, // Less than $0.01
			},
			errorMatch: "max_cost too low",
		},
		{
			name: "max_tokens too low",
			constraints: &MissionConstraints{
				MaxTokens: 500, // Less than 1000
			},
			errorMatch: "max_tokens too low",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mission := createIntegrationTestMission()
			// Add the workflow to the store so workflow validation passes
			workflowStore.Add(mission.WorkflowID, map[string]interface{}{"name": "test-workflow"})
			mission.Constraints = tc.constraints

			ctx := context.Background()
			err := service.ValidateMission(ctx, mission)
			// Note: max_findings: 0 is now valid (means unlimited), so skip that error check
			if tc.errorMatch == "max_findings must be at least 1" {
				// This case should now pass since 0 means unlimited
				assert.NoError(t, err, "MaxFindings: 0 should be valid (unlimited)")
			} else {
				assert.Error(t, err, "ValidateMission should fail with invalid constraints")
				assert.Contains(t, err.Error(), tc.errorMatch, "Error should indicate constraint validation failure")
			}
		})
	}
}

// TestMissionServiceValidateMission_MissingName tests validation failure with missing required fields
func TestMissionServiceValidateMission_MissingName(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Create mission with missing name
	mission := createIntegrationTestMission()
	mission.Name = ""

	// Validate mission
	ctx := context.Background()
	err := service.ValidateMission(ctx, mission)
	assert.Error(t, err, "ValidateMission should fail when mission name is missing")
}

// TestMissionServiceCreateFromConfig tests creating a mission from YAML config
func TestMissionServiceCreateFromConfig(t *testing.T) {
	db := setupIntegrationTestDB(t)
	missionStore := NewDBMissionStore(db)
	workflowStore := newIntegrationWorkflowStore()
	findingStore := newIntegrationFindingStore()

	service := NewMissionService(missionStore, workflowStore, findingStore)

	// Note: CreateFromConfig currently has TODO for target/workflow resolution
	// This test verifies the basic flow but can't fully test until resolution is implemented
	// For now, this test is skipped as it requires unimplemented functionality
	t.Skip("CreateFromConfig requires target/workflow resolution which is not yet implemented")

	// Create mission config
	config := &MissionConfig{
		Name:        "Integration Test Mission",
		Description: "A mission created from config for integration testing",
		Constraints: &MissionConstraintsConfig{
			MaxDuration: "2h",
			MaxFindings: intPtr(50),
			MaxCost:     float64Ptr(25.0),
		},
	}

	// Create mission from config
	ctx := context.Background()
	mission, err := service.CreateFromConfig(ctx, config)
	require.NoError(t, err, "CreateFromConfig should succeed")
	require.NotNil(t, mission, "Created mission should not be nil")

	// Verify mission fields
	assert.Equal(t, config.Name, mission.Name, "Mission name should match config")
	assert.Equal(t, config.Description, mission.Description, "Mission description should match config")
	assert.Equal(t, MissionStatusPending, mission.Status, "Mission should be in pending status")
	assert.NotNil(t, mission.Constraints, "Constraints should be set")
	assert.Equal(t, 2*time.Hour, mission.Constraints.MaxDuration, "MaxDuration should match")
	assert.Equal(t, 50, mission.Constraints.MaxFindings, "MaxFindings should match")
	assert.Equal(t, 25.0, mission.Constraints.MaxCost, "MaxCost should match")

	// Verify mission was saved to store
	retrieved, err := missionStore.Get(ctx, mission.ID)
	require.NoError(t, err, "Should be able to retrieve saved mission")
	assert.Equal(t, mission.ID, retrieved.ID, "Retrieved mission ID should match")
}

// Helper functions for creating test pointers
func intPtr(i int) *int {
	return &i
}

func float64Ptr(f float64) *float64 {
	return &f
}
