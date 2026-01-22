package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockPolicySource is a mock implementation of PolicySource for testing
type MockPolicySource struct {
	mock.Mock
}

func (m *MockPolicySource) GetDataPolicy(agentName string) (*DataPolicy, error) {
	args := m.Called(agentName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DataPolicy), args.Error(1)
}

// MockNodeStore is a mock implementation of NodeStore for testing
type MockNodeStore struct {
	mock.Mock
}

func (m *MockNodeStore) CountByAgentInScope(ctx context.Context, agentName, scope string) (int, error) {
	args := m.Called(ctx, agentName, scope)
	return args.Int(0), args.Error(1)
}

func TestPolicyChecker_ReuseNever(t *testing.T) {
	// Test that reuse=never always executes (returns true)
	ctx := context.Background()
	mockSource := new(MockPolicySource)
	mockStore := new(MockNodeStore)
	logger := slog.Default()

	policy := &DataPolicy{
		OutputScope: ScopeMission,
		InputScope:  ScopeMission,
		Reuse:       ReuseNever,
	}

	mockSource.On("GetDataPolicy", "test-agent").Return(policy, nil)

	checker := NewPolicyChecker(mockSource, mockStore, logger)
	shouldExecute, reason := checker.ShouldExecute(ctx, "test-agent")

	assert.True(t, shouldExecute, "reuse=never should always return true")
	assert.Empty(t, reason, "reason should be empty when executing")

	mockSource.AssertExpectations(t)
	// NodeStore should NOT be called for reuse=never
	mockStore.AssertNotCalled(t, "CountByAgentInScope")
}

func TestPolicyChecker_ReuseAlways(t *testing.T) {
	// Test that reuse=always never executes (returns false with "reuse=always")
	ctx := context.Background()
	mockSource := new(MockPolicySource)
	mockStore := new(MockNodeStore)
	logger := slog.Default()

	policy := &DataPolicy{
		OutputScope: ScopeMission,
		InputScope:  ScopeMission,
		Reuse:       ReuseAlways,
	}

	mockSource.On("GetDataPolicy", "test-agent").Return(policy, nil)

	checker := NewPolicyChecker(mockSource, mockStore, logger)
	shouldExecute, reason := checker.ShouldExecute(ctx, "test-agent")

	assert.False(t, shouldExecute, "reuse=always should always return false")
	assert.Equal(t, "reuse=always", reason, "reason should be 'reuse=always'")

	mockSource.AssertExpectations(t)
	// NodeStore should NOT be called for reuse=always
	mockStore.AssertNotCalled(t, "CountByAgentInScope")
}

func TestPolicyChecker_ReuseSkip_WithExistingData(t *testing.T) {
	// Test that reuse=skip with existing data returns false with "existing data found"
	ctx := context.Background()
	mockSource := new(MockPolicySource)
	mockStore := new(MockNodeStore)
	logger := slog.Default()

	policy := &DataPolicy{
		OutputScope: ScopeMission,
		InputScope:  ScopeMission,
		Reuse:       ReuseSkip,
	}

	mockSource.On("GetDataPolicy", "test-agent").Return(policy, nil)
	mockStore.On("CountByAgentInScope", ctx, "test-agent", ScopeMission).Return(5, nil)

	checker := NewPolicyChecker(mockSource, mockStore, logger)
	shouldExecute, reason := checker.ShouldExecute(ctx, "test-agent")

	assert.False(t, shouldExecute, "reuse=skip with existing data should return false")
	assert.Equal(t, "existing data found", reason, "reason should be 'existing data found'")

	mockSource.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

func TestPolicyChecker_ReuseSkip_WithoutExistingData(t *testing.T) {
	// Test that reuse=skip without existing data returns true
	ctx := context.Background()
	mockSource := new(MockPolicySource)
	mockStore := new(MockNodeStore)
	logger := slog.Default()

	policy := &DataPolicy{
		OutputScope: ScopeMission,
		InputScope:  ScopeMission,
		Reuse:       ReuseSkip,
	}

	mockSource.On("GetDataPolicy", "test-agent").Return(policy, nil)
	mockStore.On("CountByAgentInScope", ctx, "test-agent", ScopeMission).Return(0, nil)

	checker := NewPolicyChecker(mockSource, mockStore, logger)
	shouldExecute, reason := checker.ShouldExecute(ctx, "test-agent")

	assert.True(t, shouldExecute, "reuse=skip without existing data should return true")
	assert.Empty(t, reason, "reason should be empty when executing")

	mockSource.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

func TestPolicyChecker_ReuseSkip_DifferentScopes(t *testing.T) {
	// Test that reuse=skip uses the correct input_scope when querying
	testCases := []struct {
		name       string
		inputScope string
	}{
		{"mission_run scope", ScopeMissionRun},
		{"mission scope", ScopeMission},
		{"global scope", ScopeGlobal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mockSource := new(MockPolicySource)
			mockStore := new(MockNodeStore)
			logger := slog.Default()

			policy := &DataPolicy{
				OutputScope: ScopeMission,
				InputScope:  tc.inputScope,
				Reuse:       ReuseSkip,
			}

			mockSource.On("GetDataPolicy", "test-agent").Return(policy, nil)
			mockStore.On("CountByAgentInScope", ctx, "test-agent", tc.inputScope).Return(0, nil)

			checker := NewPolicyChecker(mockSource, mockStore, logger)
			shouldExecute, reason := checker.ShouldExecute(ctx, "test-agent")

			assert.True(t, shouldExecute, "should execute when no data in scope")
			assert.Empty(t, reason, "reason should be empty when executing")

			mockSource.AssertExpectations(t)
			mockStore.AssertExpectations(t)
		})
	}
}

func TestPolicyChecker_NoPolicyConfigured_UsesDefaults(t *testing.T) {
	// Test default policy behavior when no policy configured (nil returned)
	ctx := context.Background()
	mockSource := new(MockPolicySource)
	mockStore := new(MockNodeStore)
	logger := slog.Default()

	// GetDataPolicy returns nil (no policy defined)
	mockSource.On("GetDataPolicy", "test-agent").Return(nil, nil)

	checker := NewPolicyChecker(mockSource, mockStore, logger)
	shouldExecute, reason := checker.ShouldExecute(ctx, "test-agent")

	// Default policy is reuse=never, which always executes
	assert.True(t, shouldExecute, "default policy (reuse=never) should execute")
	assert.Empty(t, reason, "reason should be empty when executing")

	mockSource.AssertExpectations(t)
	// NodeStore should NOT be called for default reuse=never
	mockStore.AssertNotCalled(t, "CountByAgentInScope")
}

func TestPolicyChecker_PolicySourceError(t *testing.T) {
	// Test error handling when policy source returns error (fail-open: execute)
	ctx := context.Background()
	mockSource := new(MockPolicySource)
	mockStore := new(MockNodeStore)
	logger := slog.Default()

	expectedErr := errors.New("policy source connection failed")
	mockSource.On("GetDataPolicy", "test-agent").Return(nil, expectedErr)

	checker := NewPolicyChecker(mockSource, mockStore, logger)
	shouldExecute, reason := checker.ShouldExecute(ctx, "test-agent")

	// On error, default to executing (fail-open)
	assert.True(t, shouldExecute, "should default to execute on policy source error")
	assert.Empty(t, reason, "reason should be empty when executing")

	mockSource.AssertExpectations(t)
	mockStore.AssertNotCalled(t, "CountByAgentInScope")
}

func TestPolicyChecker_NodeStoreError(t *testing.T) {
	// Test error handling when node store count query fails (fail-open: execute)
	ctx := context.Background()
	mockSource := new(MockPolicySource)
	mockStore := new(MockNodeStore)
	logger := slog.Default()

	policy := &DataPolicy{
		OutputScope: ScopeMission,
		InputScope:  ScopeMission,
		Reuse:       ReuseSkip,
	}

	expectedErr := errors.New("graph database connection failed")
	mockSource.On("GetDataPolicy", "test-agent").Return(policy, nil)
	mockStore.On("CountByAgentInScope", ctx, "test-agent", ScopeMission).Return(0, expectedErr)

	checker := NewPolicyChecker(mockSource, mockStore, logger)
	shouldExecute, reason := checker.ShouldExecute(ctx, "test-agent")

	// On error, default to executing (fail-open)
	assert.True(t, shouldExecute, "should default to execute on node store error")
	assert.Empty(t, reason, "reason should be empty when executing")

	mockSource.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

func TestPolicyChecker_InvalidReuseValue(t *testing.T) {
	// Test that invalid reuse values default to executing
	ctx := context.Background()
	mockSource := new(MockPolicySource)
	mockStore := new(MockNodeStore)
	logger := slog.Default()

	policy := &DataPolicy{
		OutputScope: ScopeMission,
		InputScope:  ScopeMission,
		Reuse:       "invalid-reuse-value",
	}

	mockSource.On("GetDataPolicy", "test-agent").Return(policy, nil)

	checker := NewPolicyChecker(mockSource, mockStore, logger)
	shouldExecute, reason := checker.ShouldExecute(ctx, "test-agent")

	// Invalid reuse value should default to executing
	assert.True(t, shouldExecute, "invalid reuse value should default to execute")
	assert.Empty(t, reason, "reason should be empty when executing")

	mockSource.AssertExpectations(t)
	mockStore.AssertNotCalled(t, "CountByAgentInScope")
}

func TestPolicyChecker_NilLogger(t *testing.T) {
	// Test that PolicyChecker handles nil logger gracefully
	ctx := context.Background()
	mockSource := new(MockPolicySource)
	mockStore := new(MockNodeStore)

	policy := &DataPolicy{
		OutputScope: ScopeMission,
		InputScope:  ScopeMission,
		Reuse:       ReuseNever,
	}

	mockSource.On("GetDataPolicy", "test-agent").Return(policy, nil)

	// Pass nil logger - should use default logger
	checker := NewPolicyChecker(mockSource, mockStore, nil)
	shouldExecute, reason := checker.ShouldExecute(ctx, "test-agent")

	assert.True(t, shouldExecute, "should execute normally with nil logger")
	assert.Empty(t, reason, "reason should be empty when executing")

	mockSource.AssertExpectations(t)
}

func TestPolicyChecker_MultipleAgents(t *testing.T) {
	// Test that different agents can have different policies
	ctx := context.Background()
	mockSource := new(MockPolicySource)
	mockStore := new(MockNodeStore)
	logger := slog.Default()

	// Agent 1: reuse=never (always execute)
	policy1 := &DataPolicy{
		OutputScope: ScopeMission,
		InputScope:  ScopeMission,
		Reuse:       ReuseNever,
	}

	// Agent 2: reuse=skip with existing data (skip)
	policy2 := &DataPolicy{
		OutputScope: ScopeMission,
		InputScope:  ScopeMission,
		Reuse:       ReuseSkip,
	}

	// Agent 3: reuse=always (always skip)
	policy3 := &DataPolicy{
		OutputScope: ScopeMission,
		InputScope:  ScopeMission,
		Reuse:       ReuseAlways,
	}

	mockSource.On("GetDataPolicy", "agent1").Return(policy1, nil)
	mockSource.On("GetDataPolicy", "agent2").Return(policy2, nil)
	mockSource.On("GetDataPolicy", "agent3").Return(policy3, nil)
	mockStore.On("CountByAgentInScope", ctx, "agent2", ScopeMission).Return(10, nil)

	checker := NewPolicyChecker(mockSource, mockStore, logger)

	// Test agent1
	shouldExecute1, reason1 := checker.ShouldExecute(ctx, "agent1")
	assert.True(t, shouldExecute1, "agent1 with reuse=never should execute")
	assert.Empty(t, reason1)

	// Test agent2
	shouldExecute2, reason2 := checker.ShouldExecute(ctx, "agent2")
	assert.False(t, shouldExecute2, "agent2 with reuse=skip and existing data should not execute")
	assert.Equal(t, "existing data found", reason2)

	// Test agent3
	shouldExecute3, reason3 := checker.ShouldExecute(ctx, "agent3")
	assert.False(t, shouldExecute3, "agent3 with reuse=always should not execute")
	assert.Equal(t, "reuse=always", reason3)

	mockSource.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

func TestPolicyChecker_EmptyPolicyUsesDefaults(t *testing.T) {
	// Test that an empty policy struct gets defaults applied
	ctx := context.Background()
	mockSource := new(MockPolicySource)
	mockStore := new(MockNodeStore)
	logger := slog.Default()

	// Return an empty policy (all fields empty strings)
	emptyPolicy := &DataPolicy{}

	mockSource.On("GetDataPolicy", "test-agent").Return(emptyPolicy, nil)

	checker := NewPolicyChecker(mockSource, mockStore, logger)
	shouldExecute, reason := checker.ShouldExecute(ctx, "test-agent")

	// Empty policy should get defaults: reuse=never, which always executes
	assert.True(t, shouldExecute, "empty policy should use default reuse=never")
	assert.Empty(t, reason, "reason should be empty when executing")

	mockSource.AssertExpectations(t)
	mockStore.AssertNotCalled(t, "CountByAgentInScope")
}
