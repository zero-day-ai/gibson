package attack

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/payload"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockPayloadRegistry is a mock implementation of PayloadRegistry for testing
type mockPayloadRegistry struct {
	payloads map[types.ID]*payload.Payload
	listErr  error
	getErr   error
}

func newMockPayloadRegistry() *mockPayloadRegistry {
	return &mockPayloadRegistry{
		payloads: make(map[types.ID]*payload.Payload),
	}
}

func (m *mockPayloadRegistry) addPayload(p *payload.Payload) {
	m.payloads[p.ID] = p
}

func (m *mockPayloadRegistry) Register(ctx context.Context, p *payload.Payload) error {
	m.payloads[p.ID] = p
	return nil
}

func (m *mockPayloadRegistry) Get(ctx context.Context, id types.ID) (*payload.Payload, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	p, ok := m.payloads[id]
	if !ok {
		return nil, fmt.Errorf("payload not found: %s", id)
	}
	return p, nil
}

func (m *mockPayloadRegistry) List(ctx context.Context, filter *payload.PayloadFilter) ([]*payload.Payload, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}

	var results []*payload.Payload
	for _, p := range m.payloads {
		if m.matchesFilter(p, filter) {
			results = append(results, p)
		}
	}
	return results, nil
}

func (m *mockPayloadRegistry) matchesFilter(p *payload.Payload, filter *payload.PayloadFilter) bool {
	if filter == nil {
		return true
	}

	// Check enabled filter
	if filter.Enabled != nil && *filter.Enabled != p.Enabled {
		return false
	}

	// Check categories filter
	if len(filter.Categories) > 0 {
		found := false
		for _, filterCat := range filter.Categories {
			for _, payloadCat := range p.Categories {
				if filterCat == payloadCat {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check MITRE techniques filter
	if len(filter.MitreTechniques) > 0 {
		found := false
		for _, filterTech := range filter.MitreTechniques {
			for _, payloadTech := range p.MitreTechniques {
				if filterTech == payloadTech {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (m *mockPayloadRegistry) Search(ctx context.Context, query string, filter *payload.PayloadFilter) ([]*payload.Payload, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockPayloadRegistry) Update(ctx context.Context, p *payload.Payload) error {
	return fmt.Errorf("not implemented")
}

func (m *mockPayloadRegistry) Disable(ctx context.Context, id types.ID) error {
	return fmt.Errorf("not implemented")
}

func (m *mockPayloadRegistry) Enable(ctx context.Context, id types.ID) error {
	return fmt.Errorf("not implemented")
}

func (m *mockPayloadRegistry) GetByCategory(ctx context.Context, category payload.PayloadCategory) ([]*payload.Payload, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockPayloadRegistry) GetByMitreTechnique(ctx context.Context, technique string) ([]*payload.Payload, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockPayloadRegistry) LoadBuiltIns(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

func (m *mockPayloadRegistry) Count(ctx context.Context, filter *payload.PayloadFilter) (int, error) {
	return 0, fmt.Errorf("not implemented")
}

func (m *mockPayloadRegistry) ClearCache() {}

func (m *mockPayloadRegistry) Health(ctx context.Context) types.HealthStatus {
	return types.Healthy("mock registry")
}

// TestNewPayloadFilter tests the constructor
func TestNewPayloadFilter(t *testing.T) {
	registry := newMockPayloadRegistry()
	filter := NewPayloadFilter(registry)

	assert.NotNil(t, filter)
	assert.Equal(t, registry, filter.registry)
}

// TestPayloadFilter_FilterByIDs tests filtering by specific payload IDs
func TestPayloadFilter_FilterByIDs(t *testing.T) {
	ctx := context.Background()
	registry := newMockPayloadRegistry()

	// Create test payloads
	payload1 := payload.NewPayload("test-payload-1", "template1", payload.CategoryJailbreak).
		WithSuccessIndicators(payload.SuccessIndicator{
			Type:  payload.IndicatorContains,
			Value: "success",
		})
	payload2 := payload.NewPayload("test-payload-2", "template2", payload.CategoryPromptInjection).
		WithSuccessIndicators(payload.SuccessIndicator{
			Type:  payload.IndicatorContains,
			Value: "success",
		})
	payload3 := payload.NewPayload("test-payload-3", "template3", payload.CategoryDataExtraction).
		WithSuccessIndicators(payload.SuccessIndicator{
			Type:  payload.IndicatorContains,
			Value: "success",
		})

	registry.addPayload(&payload1)
	registry.addPayload(&payload2)
	registry.addPayload(&payload3)

	// Create a valid but nonexistent ID
	nonexistentID := types.NewID()

	tests := []struct {
		name       string
		payloadIDs []string
		wantCount  int
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "single payload ID",
			payloadIDs: []string{string(payload1.ID)},
			wantCount:  1,
			wantErr:    false,
		},
		{
			name:       "multiple payload IDs",
			payloadIDs: []string{string(payload1.ID), string(payload2.ID)},
			wantCount:  2,
			wantErr:    false,
		},
		{
			name:       "all payload IDs",
			payloadIDs: []string{string(payload1.ID), string(payload2.ID), string(payload3.ID)},
			wantCount:  3,
			wantErr:    false,
		},
		{
			name:       "invalid payload ID format",
			payloadIDs: []string{"invalid-id"},
			wantCount:  0,
			wantErr:    true,
			errMsg:     "invalid payload ID",
		},
		{
			name:       "nonexistent payload ID",
			payloadIDs: []string{string(nonexistentID)},
			wantCount:  0,
			wantErr:    true,
			errMsg:     "failed to get payload",
		},
		{
			name:       "mix of valid and invalid IDs",
			payloadIDs: []string{string(payload1.ID), "invalid-id"},
			wantCount:  0,
			wantErr:    true,
			errMsg:     "invalid payload ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewPayloadFilter(registry)
			opts := &AttackOptions{
				PayloadIDs: tt.payloadIDs,
			}

			result, err := filter.Filter(ctx, opts)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, result, tt.wantCount)

				// Verify correct payloads returned
				if tt.wantCount > 0 {
					for i, id := range tt.payloadIDs {
						assert.Equal(t, types.ID(id), result[i].ID)
					}
				}
			}
		})
	}
}

// TestPayloadFilter_FilterByCategory tests filtering by payload category
func TestPayloadFilter_FilterByCategory(t *testing.T) {
	ctx := context.Background()
	registry := newMockPayloadRegistry()

	// Create test payloads with different categories
	payload1 := payload.NewPayload("jailbreak-1", "template1", payload.CategoryJailbreak).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload2 := payload.NewPayload("jailbreak-2", "template2", payload.CategoryJailbreak).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload3 := payload.NewPayload("injection-1", "template3", payload.CategoryPromptInjection).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload4 := payload.NewPayload("extraction-1", "template4", payload.CategoryDataExtraction).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload5 := payload.NewPayload("disabled-1", "template5", payload.CategoryJailbreak).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"}).
		Disable()

	registry.addPayload(&payload1)
	registry.addPayload(&payload2)
	registry.addPayload(&payload3)
	registry.addPayload(&payload4)
	registry.addPayload(&payload5)

	tests := []struct {
		name      string
		category  string
		wantCount int
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "filter by jailbreak category",
			category:  string(payload.CategoryJailbreak),
			wantCount: 2, // payload1 and payload2 (payload5 is disabled)
			wantErr:   false,
		},
		{
			name:      "filter by prompt_injection category",
			category:  string(payload.CategoryPromptInjection),
			wantCount: 1, // payload3
			wantErr:   false,
		},
		{
			name:      "filter by data_extraction category",
			category:  string(payload.CategoryDataExtraction),
			wantCount: 1, // payload4
			wantErr:   false,
		},
		{
			name:      "filter by dos category (no matches)",
			category:  string(payload.CategoryDoS),
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "invalid category",
			category:  "invalid-category",
			wantCount: 0,
			wantErr:   true,
			errMsg:    "invalid payload category",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewPayloadFilter(registry)
			opts := &AttackOptions{
				PayloadCategory: tt.category,
			}

			result, err := filter.Filter(ctx, opts)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, result, tt.wantCount)

				// Verify all returned payloads have the correct category
				for _, p := range result {
					assert.True(t, p.HasCategory(payload.PayloadCategory(tt.category)))
				}
			}
		})
	}
}

// TestPayloadFilter_FilterByTechniques tests filtering by MITRE techniques
func TestPayloadFilter_FilterByTechniques(t *testing.T) {
	ctx := context.Background()
	registry := newMockPayloadRegistry()

	// Create test payloads with different MITRE techniques
	payload1 := payload.NewPayload("payload-1", "template1", payload.CategoryJailbreak).
		WithMitreTechniques("T1059", "T1204").
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload2 := payload.NewPayload("payload-2", "template2", payload.CategoryPromptInjection).
		WithMitreTechniques("T1059").
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload3 := payload.NewPayload("payload-3", "template3", payload.CategoryDataExtraction).
		WithMitreTechniques("T1567").
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload4 := payload.NewPayload("payload-4", "template4", payload.CategoryDoS).
		WithMitreTechniques("T1499").
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})

	registry.addPayload(&payload1)
	registry.addPayload(&payload2)
	registry.addPayload(&payload3)
	registry.addPayload(&payload4)

	tests := []struct {
		name       string
		techniques []string
		wantCount  int
		wantIDs    []types.ID
	}{
		{
			name:       "single technique",
			techniques: []string{"T1059"},
			wantCount:  2, // payload1 and payload2
			wantIDs:    []types.ID{payload1.ID, payload2.ID},
		},
		{
			name:       "different single technique",
			techniques: []string{"T1567"},
			wantCount:  1, // payload3
			wantIDs:    []types.ID{payload3.ID},
		},
		{
			name:       "technique not found",
			techniques: []string{"T9999"},
			wantCount:  0,
			wantIDs:    []types.ID{},
		},
		{
			name:       "multiple techniques (OR logic)",
			techniques: []string{"T1059", "T1567"},
			wantCount:  3, // payload1, payload2, and payload3
			wantIDs:    []types.ID{payload1.ID, payload2.ID, payload3.ID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewPayloadFilter(registry)
			opts := &AttackOptions{
				Techniques: tt.techniques,
			}

			result, err := filter.Filter(ctx, opts)

			require.NoError(t, err)
			assert.Len(t, result, tt.wantCount)

			// Verify all returned payloads have at least one of the requested techniques
			for _, p := range result {
				hasMatch := false
				for _, tech := range tt.techniques {
					if p.HasMitreTechnique(tech) {
						hasMatch = true
						break
					}
				}
				assert.True(t, hasMatch, "payload %s should have one of the requested techniques", p.ID)
			}
		})
	}
}

// TestPayloadFilter_NoFilter tests that no filter returns all enabled payloads
func TestPayloadFilter_NoFilter(t *testing.T) {
	ctx := context.Background()
	registry := newMockPayloadRegistry()

	// Create test payloads
	payload1 := payload.NewPayload("payload-1", "template1", payload.CategoryJailbreak).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload2 := payload.NewPayload("payload-2", "template2", payload.CategoryPromptInjection).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload3 := payload.NewPayload("payload-3", "template3", payload.CategoryDataExtraction).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload4 := payload.NewPayload("disabled-payload", "template4", payload.CategoryJailbreak).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"}).
		Disable()

	registry.addPayload(&payload1)
	registry.addPayload(&payload2)
	registry.addPayload(&payload3)
	registry.addPayload(&payload4)

	filter := NewPayloadFilter(registry)
	opts := &AttackOptions{
		// No filters specified
	}

	result, err := filter.Filter(ctx, opts)

	require.NoError(t, err)
	assert.Len(t, result, 3, "should return all enabled payloads")

	// Verify all returned payloads are enabled
	for _, p := range result {
		assert.True(t, p.Enabled, "payload %s should be enabled", p.ID)
	}

	// Verify the disabled payload is not included
	for _, p := range result {
		assert.NotEqual(t, payload4.ID, p.ID, "disabled payload should not be included")
	}
}

// TestPayloadFilter_CombinedFilters tests combining category and technique filters
func TestPayloadFilter_CombinedFilters(t *testing.T) {
	ctx := context.Background()
	registry := newMockPayloadRegistry()

	// Create test payloads with various combinations
	payload1 := payload.NewPayload("jailbreak-t1059", "template1", payload.CategoryJailbreak).
		WithMitreTechniques("T1059").
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload2 := payload.NewPayload("jailbreak-t1567", "template2", payload.CategoryJailbreak).
		WithMitreTechniques("T1567").
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload3 := payload.NewPayload("injection-t1059", "template3", payload.CategoryPromptInjection).
		WithMitreTechniques("T1059").
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})

	registry.addPayload(&payload1)
	registry.addPayload(&payload2)
	registry.addPayload(&payload3)

	tests := []struct {
		name       string
		category   string
		techniques []string
		wantCount  int
		wantIDs    []types.ID
	}{
		{
			name:       "category and technique both match",
			category:   string(payload.CategoryJailbreak),
			techniques: []string{"T1059"},
			wantCount:  1, // Only payload1
			wantIDs:    []types.ID{payload1.ID},
		},
		{
			name:       "category matches, technique doesn't",
			category:   string(payload.CategoryJailbreak),
			techniques: []string{"T9999"},
			wantCount:  0, // No matches
			wantIDs:    []types.ID{},
		},
		{
			name:       "category matches multiple",
			category:   string(payload.CategoryJailbreak),
			techniques: []string{"T1059", "T1567"},
			wantCount:  2, // payload1 and payload2
			wantIDs:    []types.ID{payload1.ID, payload2.ID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewPayloadFilter(registry)
			opts := &AttackOptions{
				PayloadCategory: tt.category,
				Techniques:      tt.techniques,
			}

			result, err := filter.Filter(ctx, opts)

			require.NoError(t, err)
			assert.Len(t, result, tt.wantCount)

			// Verify all payloads match both filters
			for _, p := range result {
				assert.True(t, p.HasCategory(payload.PayloadCategory(tt.category)))
				hasMatch := false
				for _, tech := range tt.techniques {
					if p.HasMitreTechnique(tech) {
						hasMatch = true
						break
					}
				}
				assert.True(t, hasMatch)
			}
		})
	}
}

// TestPayloadFilter_NilOptions tests error handling for nil options
func TestPayloadFilter_NilOptions(t *testing.T) {
	ctx := context.Background()
	registry := newMockPayloadRegistry()
	filter := NewPayloadFilter(registry)

	result, err := filter.Filter(ctx, nil)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "attack options cannot be nil")
}

// TestPayloadFilter_RegistryError tests error handling when registry fails
func TestPayloadFilter_RegistryError(t *testing.T) {
	ctx := context.Background()
	registry := newMockPayloadRegistry()
	registry.listErr = fmt.Errorf("database connection failed")

	filter := NewPayloadFilter(registry)
	opts := &AttackOptions{
		PayloadCategory: string(payload.CategoryJailbreak),
	}

	result, err := filter.Filter(ctx, opts)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to list payloads")
	assert.Contains(t, err.Error(), "database connection failed")
}

// TestPayloadFilter_ConversionToValueSlice tests proper conversion from pointers to values
func TestPayloadFilter_ConversionToValueSlice(t *testing.T) {
	ctx := context.Background()
	registry := newMockPayloadRegistry()

	// Create test payloads
	payload1 := payload.NewPayload("payload-1", "template1", payload.CategoryJailbreak).
		WithSeverity(agent.SeverityCritical).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})
	payload2 := payload.NewPayload("payload-2", "template2", payload.CategoryJailbreak).
		WithSeverity(agent.SeverityHigh).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})

	registry.addPayload(&payload1)
	registry.addPayload(&payload2)

	filter := NewPayloadFilter(registry)
	opts := &AttackOptions{
		PayloadCategory: string(payload.CategoryJailbreak),
	}

	result, err := filter.Filter(ctx, opts)

	require.NoError(t, err)
	assert.Len(t, result, 2)

	// Verify that results are values, not pointers
	// and that they contain the expected data
	for _, p := range result {
		assert.NotEmpty(t, p.ID)
		assert.NotEmpty(t, p.Name)
		assert.NotEmpty(t, p.Template)
		assert.True(t, p.HasCategory(payload.CategoryJailbreak))
	}
}

// TestPayloadFilter_EmptyPayloadIDs tests handling of empty payload IDs slice
func TestPayloadFilter_EmptyPayloadIDs(t *testing.T) {
	ctx := context.Background()
	registry := newMockPayloadRegistry()

	// Create test payloads
	payload1 := payload.NewPayload("payload-1", "template1", payload.CategoryJailbreak).
		WithSuccessIndicators(payload.SuccessIndicator{Type: payload.IndicatorContains, Value: "success"})

	registry.addPayload(&payload1)

	filter := NewPayloadFilter(registry)
	opts := &AttackOptions{
		PayloadIDs: []string{}, // Empty slice, not nil
	}

	result, err := filter.Filter(ctx, opts)

	require.NoError(t, err)
	assert.Len(t, result, 1, "empty PayloadIDs should fall through to list all payloads")
}
