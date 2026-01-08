package graphrag

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/zero-day-ai/gibson/internal/types"
)

// mockMissionLister implements MissionLister interface for testing.
type mockMissionLister struct {
	missions []*Mission
	err      error
}

func (m *mockMissionLister) ListByName(ctx context.Context, name string, limit int) ([]*Mission, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.missions, nil
}

func TestQueryScoper_ResolveScope(t *testing.T) {
	// Create a logger for tests (discard output)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // Only show errors to keep test output clean
	}))

	t.Run("ScopeAll returns nil", func(t *testing.T) {
		// Setup mock store
		mockStore := &mockMissionLister{
			missions: []*Mission{},
			err:      nil,
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Call ResolveScope with ScopeAll
		result, err := scoper.ResolveScope(
			context.Background(),
			ScopeAll,
			"test-mission",
			types.NewID(),
		)

		// Assert no error
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil", err)
		}

		// Assert result is nil (no filtering)
		if result != nil {
			t.Errorf("ResolveScope() result = %v, want nil", result)
		}
	})

	t.Run("empty scope returns nil", func(t *testing.T) {
		// Setup mock store
		mockStore := &mockMissionLister{
			missions: []*Mission{},
			err:      nil,
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Call ResolveScope with empty scope
		result, err := scoper.ResolveScope(
			context.Background(),
			"", // empty scope
			"test-mission",
			types.NewID(),
		)

		// Assert no error
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil", err)
		}

		// Assert result is nil (no filtering)
		if result != nil {
			t.Errorf("ResolveScope() result = %v, want nil", result)
		}
	})

	t.Run("ScopeCurrentRun returns current ID", func(t *testing.T) {
		// Setup mock store (not used for current_run scope)
		mockStore := &mockMissionLister{
			missions: []*Mission{},
			err:      nil,
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Setup
		currentID := types.NewID()

		// Call ResolveScope with ScopeCurrentRun
		result, err := scoper.ResolveScope(
			context.Background(),
			ScopeCurrentRun,
			"test-mission",
			currentID,
		)

		// Assert no error
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil", err)
		}

		// Assert result is []types.ID{currentID}
		if len(result) != 1 {
			t.Errorf("ResolveScope() result length = %v, want 1", len(result))
		}
		if result[0] != currentID {
			t.Errorf("ResolveScope() result[0] = %v, want %v", result[0], currentID)
		}
	})

	t.Run("ScopeSameMission returns all run IDs", func(t *testing.T) {
		// Setup mock store with 3 missions with same name
		mission1 := &Mission{ID: types.NewID(), Name: "test-mission"}
		mission2 := &Mission{ID: types.NewID(), Name: "test-mission"}
		mission3 := &Mission{ID: types.NewID(), Name: "test-mission"}

		mockStore := &mockMissionLister{
			missions: []*Mission{mission1, mission2, mission3},
			err:      nil,
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Call ResolveScope with ScopeSameMission
		result, err := scoper.ResolveScope(
			context.Background(),
			ScopeSameMission,
			"test-mission",
			mission1.ID,
		)

		// Assert no error
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil", err)
		}

		// Assert result contains all 3 IDs
		if len(result) != 3 {
			t.Errorf("ResolveScope() result length = %v, want 3", len(result))
		}

		// Verify all mission IDs are present
		foundIDs := make(map[types.ID]bool)
		for _, id := range result {
			foundIDs[id] = true
		}

		if !foundIDs[mission1.ID] {
			t.Errorf("ResolveScope() result missing mission1 ID %v", mission1.ID)
		}
		if !foundIDs[mission2.ID] {
			t.Errorf("ResolveScope() result missing mission2 ID %v", mission2.ID)
		}
		if !foundIDs[mission3.ID] {
			t.Errorf("ResolveScope() result missing mission3 ID %v", mission3.ID)
		}
	})

	t.Run("ScopeSameMission with single run", func(t *testing.T) {
		// Setup mock store with 1 mission
		mission1 := &Mission{ID: types.NewID(), Name: "test-mission"}

		mockStore := &mockMissionLister{
			missions: []*Mission{mission1},
			err:      nil,
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Call ResolveScope with ScopeSameMission
		result, err := scoper.ResolveScope(
			context.Background(),
			ScopeSameMission,
			"test-mission",
			mission1.ID,
		)

		// Assert no error
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil", err)
		}

		// Assert result contains 1 ID
		if len(result) != 1 {
			t.Errorf("ResolveScope() result length = %v, want 1", len(result))
		}
		if result[0] != mission1.ID {
			t.Errorf("ResolveScope() result[0] = %v, want %v", result[0], mission1.ID)
		}
	})

	t.Run("ScopeSameMission with many runs", func(t *testing.T) {
		// Setup mock store with 100+ missions
		missions := make([]*Mission, 150)
		for i := 0; i < 150; i++ {
			missions[i] = &Mission{
				ID:   types.NewID(),
				Name: "test-mission",
			}
		}

		mockStore := &mockMissionLister{
			missions: missions,
			err:      nil,
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Call ResolveScope with ScopeSameMission
		result, err := scoper.ResolveScope(
			context.Background(),
			ScopeSameMission,
			"test-mission",
			missions[0].ID,
		)

		// Assert no error
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil", err)
		}

		// Assert result contains all 150 IDs
		if len(result) != 150 {
			t.Errorf("ResolveScope() result length = %v, want 150", len(result))
		}
	})

	t.Run("store error falls back to nil", func(t *testing.T) {
		// Setup mock store that returns error
		mockStore := &mockMissionLister{
			missions: nil,
			err:      errors.New("database connection failed"),
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Call ResolveScope with ScopeSameMission
		result, err := scoper.ResolveScope(
			context.Background(),
			ScopeSameMission,
			"test-mission",
			types.NewID(),
		)

		// Assert no error (graceful degradation)
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil (graceful degradation)", err)
		}

		// Assert result is nil (fallback to all)
		if result != nil {
			t.Errorf("ResolveScope() result = %v, want nil (fallback to all)", result)
		}
	})

	t.Run("ScopeSameMission with empty mission name", func(t *testing.T) {
		// Setup mock store
		mockStore := &mockMissionLister{
			missions: []*Mission{},
			err:      nil,
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Call ResolveScope with ScopeSameMission and empty mission name
		result, err := scoper.ResolveScope(
			context.Background(),
			ScopeSameMission,
			"", // empty mission name
			types.NewID(),
		)

		// Assert no error (graceful degradation)
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil", err)
		}

		// Assert result is nil (fallback to all)
		if result != nil {
			t.Errorf("ResolveScope() result = %v, want nil (fallback to all)", result)
		}
	})

	t.Run("ScopeSameMission with no missions found", func(t *testing.T) {
		// Setup mock store that returns empty list
		mockStore := &mockMissionLister{
			missions: []*Mission{}, // Empty list
			err:      nil,
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Call ResolveScope with ScopeSameMission
		result, err := scoper.ResolveScope(
			context.Background(),
			ScopeSameMission,
			"test-mission",
			types.NewID(),
		)

		// Assert no error (graceful degradation)
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil", err)
		}

		// Assert result is nil (fallback to all)
		if result != nil {
			t.Errorf("ResolveScope() result = %v, want nil (fallback to all)", result)
		}
	})

	t.Run("unknown scope falls back to nil", func(t *testing.T) {
		// Setup mock store
		mockStore := &mockMissionLister{
			missions: []*Mission{},
			err:      nil,
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Call ResolveScope with unknown/invalid scope
		result, err := scoper.ResolveScope(
			context.Background(),
			MissionScope("invalid_scope"),
			"test-mission",
			types.NewID(),
		)

		// Assert no error (graceful degradation)
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil", err)
		}

		// Assert result is nil (fallback to all)
		if result != nil {
			t.Errorf("ResolveScope() result = %v, want nil (fallback to all)", result)
		}
	})

	t.Run("ScopeCurrentRun with nil mission ID", func(t *testing.T) {
		// Setup mock store
		mockStore := &mockMissionLister{
			missions: []*Mission{},
			err:      nil,
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Call ResolveScope with ScopeCurrentRun and empty ID
		var emptyID types.ID
		result, err := scoper.ResolveScope(
			context.Background(),
			ScopeCurrentRun,
			"test-mission",
			emptyID,
		)

		// Assert no error
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil", err)
		}

		// Assert result is []types.ID{emptyID}
		if len(result) != 1 {
			t.Errorf("ResolveScope() result length = %v, want 1", len(result))
		}
		if result[0] != emptyID {
			t.Errorf("ResolveScope() result[0] = %v, want %v", result[0], emptyID)
		}
	})

	t.Run("context cancellation during ScopeSameMission", func(t *testing.T) {
		// Setup mock store that checks context
		mockStore := &mockMissionLister{
			missions: nil,
			err:      context.Canceled,
		}
		scoper := NewQueryScoper(mockStore, logger)

		// Create a cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Call ResolveScope with cancelled context
		result, err := scoper.ResolveScope(
			ctx,
			ScopeSameMission,
			"test-mission",
			types.NewID(),
		)

		// Assert no error (graceful degradation)
		if err != nil {
			t.Errorf("ResolveScope() error = %v, want nil (graceful degradation)", err)
		}

		// Assert result is nil (fallback to all)
		if result != nil {
			t.Errorf("ResolveScope() result = %v, want nil (fallback to all)", result)
		}
	})
}

func TestNewQueryScoper(t *testing.T) {
	// Create a logger for tests
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	mockStore := &mockMissionLister{
		missions: []*Mission{},
		err:      nil,
	}

	scoper := NewQueryScoper(mockStore, logger)

	if scoper == nil {
		t.Error("NewQueryScoper() returned nil")
	}

	if scoper.missionLister != mockStore {
		t.Error("NewQueryScoper() did not set mission lister correctly")
	}

	if scoper.logger != logger {
		t.Error("NewQueryScoper() did not set logger correctly")
	}
}

func TestQueryScoper_ResolveScopeOrder(t *testing.T) {
	// Test that missions are returned in the order provided by the store
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	// Create missions in specific order
	mission1 := &Mission{ID: types.NewID(), Name: "test-mission"}
	mission2 := &Mission{ID: types.NewID(), Name: "test-mission"}
	mission3 := &Mission{ID: types.NewID(), Name: "test-mission"}

	mockStore := &mockMissionLister{
		missions: []*Mission{mission1, mission2, mission3},
		err:      nil,
	}
	scoper := NewQueryScoper(mockStore, logger)

	result, err := scoper.ResolveScope(
		context.Background(),
		ScopeSameMission,
		"test-mission",
		mission1.ID,
	)

	if err != nil {
		t.Errorf("ResolveScope() error = %v, want nil", err)
	}

	// Verify order is preserved
	if len(result) != 3 {
		t.Fatalf("ResolveScope() result length = %v, want 3", len(result))
	}

	if result[0] != mission1.ID {
		t.Errorf("ResolveScope() result[0] = %v, want %v", result[0], mission1.ID)
	}
	if result[1] != mission2.ID {
		t.Errorf("ResolveScope() result[1] = %v, want %v", result[1], mission2.ID)
	}
	if result[2] != mission3.ID {
		t.Errorf("ResolveScope() result[2] = %v, want %v", result[2], mission3.ID)
	}
}

func TestQueryScoper_ResolveScopeLimit(t *testing.T) {
	// Test that the scoper respects the 1000 mission limit
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	// Verify that ListByName is called with limit=1000
	limitCalled := false
	actualLimit := 0

	// Custom mock that captures the limit parameter
	customMock := &mockMissionListerWithLimit{
		onListByName: func(ctx context.Context, name string, limit int) ([]*Mission, error) {
			limitCalled = true
			actualLimit = limit
			return []*Mission{{ID: types.NewID(), Name: name}}, nil
		},
	}

	scoper := &DefaultQueryScoper{
		missionLister: customMock,
		logger:        logger,
	}

	_, err := scoper.ResolveScope(
		context.Background(),
		ScopeSameMission,
		"test-mission",
		types.NewID(),
	)

	if err != nil {
		t.Errorf("ResolveScope() error = %v, want nil", err)
	}

	if !limitCalled {
		t.Error("ResolveScope() did not call ListByName")
	}

	if actualLimit != 1000 {
		t.Errorf("ResolveScope() called ListByName with limit %v, want 1000", actualLimit)
	}
}

// mockMissionListerWithLimit is a custom mock that allows capturing function parameters.
type mockMissionListerWithLimit struct {
	onListByName func(ctx context.Context, name string, limit int) ([]*Mission, error)
}

func (m *mockMissionListerWithLimit) ListByName(ctx context.Context, name string, limit int) ([]*Mission, error) {
	if m.onListByName != nil {
		return m.onListByName(ctx, name, limit)
	}
	return nil, nil
}
