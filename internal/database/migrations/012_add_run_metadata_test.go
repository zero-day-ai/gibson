package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockGraphClient implements WritableGraphClient for testing.
type mockGraphClient struct {
	queryResults      map[string]graph.QueryResult
	executeResults    map[string]graph.QueryResult
	queryError        error
	executeError      error
	queryCalls        []string
	executeCalls      []string
	executeParams     []map[string]any
}

func newMockGraphClient() *mockGraphClient {
	return &mockGraphClient{
		queryResults:   make(map[string]graph.QueryResult),
		executeResults: make(map[string]graph.QueryResult),
	}
}

func (m *mockGraphClient) Connect(ctx context.Context) error {
	return nil
}

func (m *mockGraphClient) Close(ctx context.Context) error {
	return nil
}

func (m *mockGraphClient) Health(ctx context.Context) types.HealthStatus {
	return types.Healthy("mock")
}

func (m *mockGraphClient) Query(ctx context.Context, cypher string, params map[string]any) (graph.QueryResult, error) {
	m.queryCalls = append(m.queryCalls, cypher)
	if m.queryError != nil {
		return graph.QueryResult{}, m.queryError
	}
	if result, ok := m.queryResults[cypher]; ok {
		return result, nil
	}
	// Return default empty result
	return graph.QueryResult{Records: []map[string]any{}}, nil
}

func (m *mockGraphClient) CreateNode(ctx context.Context, labels []string, props map[string]any) (string, error) {
	return "mock-node-id", nil
}

func (m *mockGraphClient) CreateRelationship(ctx context.Context, fromID, toID, relType string, props map[string]any) error {
	return nil
}

func (m *mockGraphClient) DeleteNode(ctx context.Context, nodeID string) error {
	return nil
}

func (m *mockGraphClient) ExecuteWrite(ctx context.Context, cypher string, params map[string]any) (graph.QueryResult, error) {
	m.executeCalls = append(m.executeCalls, cypher)
	m.executeParams = append(m.executeParams, params)
	if m.executeError != nil {
		return graph.QueryResult{}, m.executeError
	}
	if result, ok := m.executeResults[cypher]; ok {
		return result, nil
	}
	// Return default result with count
	return graph.QueryResult{
		Records: []map[string]any{{"updated": int64(0)}},
	}, nil
}

// mockMissionLister implements MissionLister for testing.
type mockMissionLister struct {
	missions map[types.ID]Mission
	getError error
}

func newMockMissionLister() *mockMissionLister {
	return &mockMissionLister{
		missions: make(map[types.ID]Mission),
	}
}

func (m *mockMissionLister) GetByID(ctx context.Context, id types.ID) (Mission, error) {
	if m.getError != nil {
		return Mission{}, m.getError
	}
	if mission, ok := m.missions[id]; ok {
		return mission, nil
	}
	return Mission{}, errors.New("mission not found")
}

func TestRunMetadataMigration_Name(t *testing.T) {
	migration := NewRunMetadataMigration(newMockGraphClient(), nil, nil)
	if migration.Name() != "012_add_run_metadata" {
		t.Errorf("Name() = %v, want 012_add_run_metadata", migration.Name())
	}
}

func TestRunMetadataMigration_Run_NoNodesToMigrate(t *testing.T) {
	mockClient := newMockGraphClient()
	// Set up query to return 0 nodes without run metadata
	mockClient.queryResults[`
		MATCH (n)
		WHERE n.run_number IS NULL
		RETURN count(n) as count
	`] = graph.QueryResult{
		Records: []map[string]any{{"count": int64(0)}},
	}

	migration := NewRunMetadataMigration(mockClient, nil, nil)
	status, err := migration.Run(context.Background())

	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
	if !status.Applied {
		t.Error("Run() status.Applied = false, want true")
	}
	if status.NodesFound != 0 {
		t.Errorf("Run() status.NodesFound = %v, want 0", status.NodesFound)
	}
}

func TestRunMetadataMigration_Run_MigratesNodes(t *testing.T) {
	mockClient := newMockGraphClient()
	// Set up query to return 10 nodes without run metadata
	mockClient.queryResults[`
		MATCH (n)
		WHERE n.run_number IS NULL
		RETURN count(n) as count
	`] = graph.QueryResult{
		Records: []map[string]any{{"count": int64(10)}},
	}

	// Set up execute to return 10 nodes updated
	mockClient.executeResults[`
		MATCH (n)
		WHERE n.run_number IS NULL
		SET n.run_number = 1,
		    n.created_in_run = 1,
		    n.migrated_at = datetime()
		RETURN count(n) as updated
	`] = graph.QueryResult{
		Records: []map[string]any{{"updated": int64(10)}},
	}

	migration := NewRunMetadataMigration(mockClient, nil, nil)
	status, err := migration.Run(context.Background())

	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
	if !status.Applied {
		t.Error("Run() status.Applied = false, want true")
	}
	if status.NodesFound != 10 {
		t.Errorf("Run() status.NodesFound = %v, want 10", status.NodesFound)
	}
	if status.NodesFixed != 10 {
		t.Errorf("Run() status.NodesFixed = %v, want 10", status.NodesFixed)
	}
}

func TestRunMetadataMigration_Run_QueryError(t *testing.T) {
	mockClient := newMockGraphClient()
	mockClient.queryError = errors.New("connection failed")

	migration := NewRunMetadataMigration(mockClient, nil, nil)
	status, err := migration.Run(context.Background())

	if err == nil {
		t.Error("Run() error = nil, want error")
	}
	if status.Applied {
		t.Error("Run() status.Applied = true, want false")
	}
	if status.Error == "" {
		t.Error("Run() status.Error is empty, want error message")
	}
}

func TestRunMetadataMigration_Run_ExecuteError(t *testing.T) {
	mockClient := newMockGraphClient()
	// Set up query to return nodes
	mockClient.queryResults[`
		MATCH (n)
		WHERE n.run_number IS NULL
		RETURN count(n) as count
	`] = graph.QueryResult{
		Records: []map[string]any{{"count": int64(10)}},
	}
	// Set execute to fail
	mockClient.executeError = errors.New("write failed")

	migration := NewRunMetadataMigration(mockClient, nil, nil)
	status, err := migration.Run(context.Background())

	if err == nil {
		t.Error("Run() error = nil, want error")
	}
	if status.Applied {
		t.Error("Run() status.Applied = true, want false")
	}
}

func TestRunMetadataMigration_Run_WithMissionLister(t *testing.T) {
	mockClient := newMockGraphClient()
	mockLister := newMockMissionLister()

	missionID := types.NewID()
	mockLister.missions[missionID] = Mission{
		ID:   missionID,
		Name: "test-mission",
	}

	// Set up query to return 5 nodes without run metadata
	mockClient.queryResults[`
		MATCH (n)
		WHERE n.run_number IS NULL
		RETURN count(n) as count
	`] = graph.QueryResult{
		Records: []map[string]any{{"count": int64(5)}},
	}

	// Set up execute to return 5 nodes updated
	mockClient.executeResults[`
		MATCH (n)
		WHERE n.run_number IS NULL
		SET n.run_number = 1,
		    n.created_in_run = 1,
		    n.migrated_at = datetime()
		RETURN count(n) as updated
	`] = graph.QueryResult{
		Records: []map[string]any{{"updated": int64(5)}},
	}

	// Set up query for distinct mission IDs
	mockClient.queryResults[`
		MATCH (n)
		WHERE n.mission_id IS NOT NULL AND n.mission_name IS NULL
		RETURN DISTINCT n.mission_id as mission_id
		LIMIT 1000
	`] = graph.QueryResult{
		Records: []map[string]any{{"mission_id": missionID.String()}},
	}

	migration := NewRunMetadataMigration(mockClient, mockLister, nil)
	status, err := migration.Run(context.Background())

	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
	if !status.Applied {
		t.Error("Run() status.Applied = false, want true")
	}
}

func TestRunMetadataMigration_Verify(t *testing.T) {
	tests := []struct {
		name       string
		nodeCount  int64
		wantResult bool
		wantErr    bool
	}{
		{
			name:       "no nodes without run metadata",
			nodeCount:  0,
			wantResult: true,
			wantErr:    false,
		},
		{
			name:       "some nodes without run metadata",
			nodeCount:  5,
			wantResult: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockGraphClient()
			mockClient.queryResults[`
		MATCH (n)
		WHERE n.run_number IS NULL
		RETURN count(n) as count
	`] = graph.QueryResult{
				Records: []map[string]any{{"count": tt.nodeCount}},
			}

			migration := NewRunMetadataMigration(mockClient, nil, nil)
			result, err := migration.Verify(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
			if result != tt.wantResult {
				t.Errorf("Verify() = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

func TestRunMetadataMigration_Rollback(t *testing.T) {
	mockClient := newMockGraphClient()
	mockClient.executeResults[`
		MATCH (n)
		WHERE n.migrated_at IS NOT NULL
		REMOVE n.run_number, n.created_in_run, n.mission_name, n.migrated_at
		RETURN count(n) as reverted
	`] = graph.QueryResult{
		Records: []map[string]any{{"reverted": int64(10)}},
	}

	migration := NewRunMetadataMigration(mockClient, nil, nil)
	err := migration.Rollback(context.Background())

	if err != nil {
		t.Errorf("Rollback() error = %v, want nil", err)
	}
	if len(mockClient.executeCalls) != 1 {
		t.Errorf("Rollback() executed %d queries, want 1", len(mockClient.executeCalls))
	}
}

func TestRunMetadataMigration_Rollback_Error(t *testing.T) {
	mockClient := newMockGraphClient()
	mockClient.executeError = errors.New("rollback failed")

	migration := NewRunMetadataMigration(mockClient, nil, nil)
	err := migration.Rollback(context.Background())

	if err == nil {
		t.Error("Rollback() error = nil, want error")
	}
}
