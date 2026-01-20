package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/payload"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockPayloadStore implements payload.PayloadStore for testing
type mockPayloadStore struct {
	getSummaryFunc func(ctx context.Context, targetType string) (*payload.PayloadSummary, error)
}

func (m *mockPayloadStore) GetSummaryForTargetType(ctx context.Context, targetType string) (*payload.PayloadSummary, error) {
	if m.getSummaryFunc != nil {
		return m.getSummaryFunc(ctx, targetType)
	}
	return &payload.PayloadSummary{}, nil
}

// Implement other required methods (not used in these tests)
func (m *mockPayloadStore) Save(ctx context.Context, p *payload.Payload) error {
	return nil
}

func (m *mockPayloadStore) Get(ctx context.Context, id types.ID) (*payload.Payload, error) {
	return nil, nil
}

func (m *mockPayloadStore) List(ctx context.Context, filter *payload.PayloadFilter) ([]*payload.Payload, error) {
	return nil, nil
}

func (m *mockPayloadStore) Search(ctx context.Context, query string, filter *payload.PayloadFilter) ([]*payload.Payload, error) {
	return nil, nil
}

func (m *mockPayloadStore) Update(ctx context.Context, p *payload.Payload) error {
	return nil
}

func (m *mockPayloadStore) Delete(ctx context.Context, id types.ID) error {
	return nil
}

func (m *mockPayloadStore) GetVersionHistory(ctx context.Context, id types.ID) ([]*payload.PayloadVersion, error) {
	return nil, nil
}

func (m *mockPayloadStore) Exists(ctx context.Context, id types.ID) (bool, error) {
	return false, nil
}

func (m *mockPayloadStore) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, nil
}

func (m *mockPayloadStore) Count(ctx context.Context, filter *payload.PayloadFilter) (int, error) {
	return 0, nil
}

func (m *mockPayloadStore) ImportBatch(ctx context.Context, payloads []*payload.Payload) (*payload.ImportResult, error) {
	return nil, nil
}

func (m *mockPayloadStore) CreateChain(ctx context.Context, chain *payload.PayloadChain) error {
	return nil
}

func (m *mockPayloadStore) GetChain(ctx context.Context, id types.ID) (*payload.PayloadChain, error) {
	return nil, nil
}

func (m *mockPayloadStore) ListChains(ctx context.Context) ([]*payload.PayloadChain, error) {
	return nil, nil
}

func (m *mockPayloadStore) UpdateChain(ctx context.Context, chain *payload.PayloadChain) error {
	return nil
}

func (m *mockPayloadStore) DeleteChain(ctx context.Context, id types.ID) error {
	return nil
}

func TestPayloadContextInjector_InjectPayloadContext(t *testing.T) {
	ctx := context.Background()

	t.Run("SuccessfulInjection", func(t *testing.T) {
		// Create mock store that returns a summary
		mockStore := &mockPayloadStore{
			getSummaryFunc: func(ctx context.Context, targetType string) (*payload.PayloadSummary, error) {
				return &payload.PayloadSummary{
					Total: 47,
					ByCategory: map[payload.PayloadCategory]int{
						payload.CategoryJailbreak:       12,
						payload.CategoryPromptInjection: 23,
						payload.CategoryDataExtraction:  8,
						payload.CategoryPrivilegeEscalation: 4,
					},
					BySeverity: map[agent.FindingSeverity]int{
						agent.SeverityHigh:     20,
						agent.SeverityCritical: 15,
						agent.SeverityMedium:   12,
					},
					EnabledCount: 47,
				}, nil
			},
		}

		injector := NewPayloadContextInjector(mockStore)
		state := &ObservationState{
			MissionInfo: MissionInfo{
				ID: types.NewID().String(),
			},
		}

		err := injector.InjectPayloadContext(ctx, state, "openai")
		require.NoError(t, err, "injection should succeed")

		// Verify context was populated
		assert.NotEmpty(t, state.PayloadContext, "payload context should be populated")
		assert.Contains(t, state.PayloadContext, "47 attack payloads", "should mention total count")
		assert.Contains(t, state.PayloadContext, "jailbreak: 12", "should list jailbreak count")
		assert.Contains(t, state.PayloadContext, "prompt_injection: 23", "should list prompt injection count")
		assert.Contains(t, state.PayloadContext, "data_extraction: 8", "should list data extraction count")
		assert.Contains(t, state.PayloadContext, "privilege_escalation: 4", "should list privilege escalation count")
		assert.Contains(t, state.PayloadContext, "payload_search", "should mention payload_search tool")
		assert.Contains(t, state.PayloadContext, "payload_execute", "should mention payload_execute tool")
	})

	t.Run("NoPayloadsAvailable", func(t *testing.T) {
		// Mock store returns empty summary
		mockStore := &mockPayloadStore{
			getSummaryFunc: func(ctx context.Context, targetType string) (*payload.PayloadSummary, error) {
				return &payload.PayloadSummary{
					Total:      0,
					ByCategory: map[payload.PayloadCategory]int{},
					BySeverity: map[agent.FindingSeverity]int{},
				}, nil
			},
		}

		injector := NewPayloadContextInjector(mockStore)
		state := &ObservationState{
			MissionInfo: MissionInfo{
				ID: types.NewID().String(),
			},
		}

		err := injector.InjectPayloadContext(ctx, state, "openai")
		require.NoError(t, err, "should not error when no payloads available")

		// Context should remain empty
		assert.Empty(t, state.PayloadContext, "payload context should be empty when no payloads")
	})

	t.Run("NilObservationState", func(t *testing.T) {
		mockStore := &mockPayloadStore{}
		injector := NewPayloadContextInjector(mockStore)

		err := injector.InjectPayloadContext(ctx, nil, "openai")
		assert.Error(t, err, "should error on nil state")
		assert.Contains(t, err.Error(), "cannot be nil", "error message should mention nil")
	})

	t.Run("NilPayloadStore", func(t *testing.T) {
		// Injector with nil store should skip injection gracefully
		injector := NewPayloadContextInjector(nil)
		state := &ObservationState{
			MissionInfo: MissionInfo{
				ID: types.NewID().String(),
			},
		}

		err := injector.InjectPayloadContext(ctx, state, "openai")
		require.NoError(t, err, "should handle nil store gracefully")

		// Context should remain empty
		assert.Empty(t, state.PayloadContext, "payload context should be empty with nil store")
	})
}

func TestBuildPayloadContextString(t *testing.T) {
	t.Run("WithMultipleCategories", func(t *testing.T) {
		summary := &payload.PayloadSummary{
			Total: 50,
			ByCategory: map[payload.PayloadCategory]int{
				payload.CategoryJailbreak:       15,
				payload.CategoryPromptInjection: 20,
				payload.CategoryDataExtraction:  10,
				payload.CategoryRAGPoisoning:    5,
			},
			BySeverity: map[agent.FindingSeverity]int{
				agent.SeverityCritical: 10,
				agent.SeverityHigh:     25,
				agent.SeverityMedium:   15,
			},
		}

		contextStr := buildPayloadContextString(summary)

		assert.NotEmpty(t, contextStr, "context string should not be empty")
		assert.Contains(t, contextStr, "Available Payloads", "should have header")
		assert.Contains(t, contextStr, "50 attack payloads", "should mention total")
		assert.Contains(t, contextStr, "jailbreak: 15", "should list jailbreak category")
		assert.Contains(t, contextStr, "prompt_injection: 20", "should list prompt injection category")
		assert.Contains(t, contextStr, "data_extraction: 10", "should list data extraction category")
		assert.Contains(t, contextStr, "rag_poisoning: 5", "should list RAG poisoning category")
		assert.Contains(t, contextStr, "Severity Breakdown", "should have severity section")
		assert.Contains(t, contextStr, "critical: 10", "should list critical severity")
		assert.Contains(t, contextStr, "high: 25", "should list high severity")
		assert.Contains(t, contextStr, "medium: 15", "should list medium severity")
		assert.Contains(t, contextStr, "payload_search", "should mention search tool")
		assert.Contains(t, contextStr, "payload_execute", "should mention execute tool")
	})

	t.Run("WithOnlyCategories", func(t *testing.T) {
		summary := &payload.PayloadSummary{
			Total: 10,
			ByCategory: map[payload.PayloadCategory]int{
				payload.CategoryJailbreak: 10,
			},
			BySeverity: map[agent.FindingSeverity]int{},
		}

		contextStr := buildPayloadContextString(summary)

		assert.Contains(t, contextStr, "10 attack payloads", "should mention total")
		assert.Contains(t, contextStr, "jailbreak: 10", "should list category")
		// Should not have severity breakdown if empty
		assert.NotContains(t, contextStr, "Severity Breakdown", "should not have severity section when empty")
	})

	t.Run("EmptySummary", func(t *testing.T) {
		summary := &payload.PayloadSummary{
			Total:      0,
			ByCategory: map[payload.PayloadCategory]int{},
			BySeverity: map[agent.FindingSeverity]int{},
		}

		contextStr := buildPayloadContextString(summary)

		assert.Contains(t, contextStr, "0 attack payloads", "should mention zero payloads")
		assert.Contains(t, contextStr, "payload_search", "should still mention tools")
	})
}

func TestPayloadContextInPrompt(t *testing.T) {
	t.Run("PromptIncludesPayloadContext", func(t *testing.T) {
		state := &ObservationState{
			MissionInfo: MissionInfo{
				ID:          types.NewID().String(),
				Name:        "Test Mission",
				Objective:   "Test payloads",
				Status:      "running",
				TimeElapsed: "5m",
			},
			GraphSummary: GraphSummary{
				TotalNodes:     3,
				CompletedNodes: 1,
			},
			PayloadContext: `## Available Payloads

You have access to 47 attack payloads:
- jailbreak: 12
- prompt_injection: 23
- data_extraction: 8
- privilege_escalation: 4

**Using Payloads:**
1. Use the payload_search tool
2. Use the payload_execute tool`,
			ResourceConstraints: ResourceConstraints{
				MaxConcurrent: 10,
			},
		}

		prompt := BuildObservationPrompt(state)

		assert.NotEmpty(t, prompt, "prompt should not be empty")
		assert.Contains(t, prompt, "Available Payloads", "should include payload section")
		assert.Contains(t, prompt, "47 attack payloads", "should mention payload count")
		assert.Contains(t, prompt, "payload_search", "should mention search tool")
		assert.Contains(t, prompt, "payload_execute", "should mention execute tool")
		assert.Contains(t, prompt, "Test payloads", "should include mission objective")
	})

	t.Run("PromptWithoutPayloadContext", func(t *testing.T) {
		state := &ObservationState{
			MissionInfo: MissionInfo{
				ID:          types.NewID().String(),
				Name:        "Test Mission",
				Objective:   "Test without payloads",
				Status:      "running",
				TimeElapsed: "2m",
			},
			GraphSummary: GraphSummary{
				TotalNodes:     2,
				CompletedNodes: 0,
			},
			// No PayloadContext
			ResourceConstraints: ResourceConstraints{
				MaxConcurrent: 10,
			},
		}

		prompt := BuildObservationPrompt(state)

		assert.NotEmpty(t, prompt, "prompt should not be empty")
		assert.NotContains(t, prompt, "Available Payloads", "should not include payload section when empty")
		assert.NotContains(t, prompt, "payload_search", "should not mention payload tools when no context")
		assert.Contains(t, prompt, "Test without payloads", "should still include mission objective")
	})
}
