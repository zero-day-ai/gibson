package payload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBuiltInLoader(t *testing.T) {
	loader := NewBuiltInLoader()
	require.NotNil(t, loader)

	// Note: GetCount() and GetCategories() auto-load payloads, so they won't be 0/empty
	// This is by design for convenience
	count := loader.GetCount()
	assert.GreaterOrEqual(t, count, 1, "should have at least one payload after auto-load")

	categories := loader.GetCategories()
	assert.NotEmpty(t, categories, "should have categories after auto-load")
}

func TestBuiltInLoader_Load(t *testing.T) {
	loader := NewBuiltInLoader()

	payloads, err := loader.Load()
	require.NoError(t, err, "loading built-in payloads should not error")
	require.NotNil(t, payloads, "payloads should not be nil")

	// We should have at least the sample payload
	assert.GreaterOrEqual(t, len(payloads), 1, "should have at least one built-in payload")

	// Verify all payloads are marked correctly
	for i, payload := range payloads {
		assert.True(t, payload.BuiltIn, "payload %d should be marked as built-in", i)
		assert.True(t, payload.Enabled, "payload %d should be enabled", i)
		assert.NotEmpty(t, payload.ID, "payload %d should have an ID", i)
		assert.NotEmpty(t, payload.Name, "payload %d should have a name", i)
		assert.NotEmpty(t, payload.Template, "payload %d should have a template", i)
		assert.NotEmpty(t, payload.Categories, "payload %d should have categories", i)
		assert.NotEmpty(t, payload.SuccessIndicators, "payload %d should have success indicators", i)

		// Verify timestamps are set
		assert.False(t, payload.CreatedAt.IsZero(), "payload %d should have CreatedAt set", i)
		assert.False(t, payload.UpdatedAt.IsZero(), "payload %d should have UpdatedAt set", i)

		// Validate the payload
		err := payload.Validate()
		assert.NoError(t, err, "payload %d (%s) should be valid", i, payload.Name)
	}
}

func TestBuiltInLoader_LoadIdempotent(t *testing.T) {
	loader := NewBuiltInLoader()

	// Load twice
	payloads1, err1 := loader.Load()
	require.NoError(t, err1)

	payloads2, err2 := loader.Load()
	require.NoError(t, err2)

	// Should return the same results
	assert.Equal(t, len(payloads1), len(payloads2), "load should be idempotent")
}

func TestBuiltInLoader_GetCategories(t *testing.T) {
	loader := NewBuiltInLoader()

	// Load payloads first
	payloads, err := loader.Load()
	require.NoError(t, err)
	require.NotEmpty(t, payloads)

	// Get categories
	categories := loader.GetCategories()
	assert.NotEmpty(t, categories, "should have at least one category")

	// Verify categories are valid
	for _, category := range categories {
		assert.True(t, category.IsValid(), "category %s should be valid", category)
	}

	// Verify all payload categories are represented
	expectedCategories := make(map[PayloadCategory]bool)
	for _, payload := range payloads {
		for _, cat := range payload.Categories {
			expectedCategories[cat] = true
		}
	}

	assert.Equal(t, len(expectedCategories), len(categories),
		"should have all unique categories from payloads")
}

func TestBuiltInLoader_GetCategoriesBeforeLoad(t *testing.T) {
	loader := NewBuiltInLoader()

	// Get categories without explicit load
	categories := loader.GetCategories()

	// Should trigger automatic load and return categories
	assert.NotEmpty(t, categories, "should auto-load and return categories")
}

func TestBuiltInLoader_GetCount(t *testing.T) {
	loader := NewBuiltInLoader()

	// Load payloads first
	payloads, err := loader.Load()
	require.NoError(t, err)

	// Get count
	count := loader.GetCount()
	assert.Equal(t, len(payloads), count, "count should match loaded payloads")
	assert.GreaterOrEqual(t, count, 1, "should have at least one payload")
}

func TestBuiltInLoader_GetCountBeforeLoad(t *testing.T) {
	loader := NewBuiltInLoader()

	// Get count without explicit load
	count := loader.GetCount()

	// Should trigger automatic load and return count
	assert.GreaterOrEqual(t, count, 1, "should auto-load and return count")
}

func TestLoadBuiltInPayloads(t *testing.T) {
	payloads, err := LoadBuiltInPayloads()
	require.NoError(t, err, "convenience function should not error")
	require.NotNil(t, payloads, "payloads should not be nil")
	assert.GreaterOrEqual(t, len(payloads), 1, "should have at least one payload")

	// Verify all are built-in
	for i, payload := range payloads {
		assert.True(t, payload.BuiltIn, "payload %d should be built-in", i)
		assert.True(t, payload.Enabled, "payload %d should be enabled", i)
	}
}

func TestGetBuiltInPayloadCount(t *testing.T) {
	count := GetBuiltInPayloadCount()
	assert.GreaterOrEqual(t, count, 1, "should have at least one built-in payload")
}

func TestGetBuiltInCategories(t *testing.T) {
	categories := GetBuiltInCategories()
	assert.NotEmpty(t, categories, "should have at least one category")

	// Verify all are valid
	for _, category := range categories {
		assert.True(t, category.IsValid(), "category %s should be valid", category)
	}
}

func TestBuiltInPayloads_SamplePayload(t *testing.T) {
	payloads, err := LoadBuiltInPayloads()
	require.NoError(t, err)
	require.NotEmpty(t, payloads)

	// Find the sample payload
	var samplePayload *Payload
	for i := range payloads {
		if payloads[i].Name == "Sample Jailbreak" {
			samplePayload = &payloads[i]
			break
		}
	}

	require.NotNil(t, samplePayload, "sample payload should exist")

	// Verify sample payload properties
	// Note: ID is converted to UUID, so we just check it's not empty and valid
	assert.NotEmpty(t, samplePayload.ID.String())
	assert.NoError(t, samplePayload.ID.Validate(), "ID should be valid UUID")
	assert.Equal(t, "Sample Jailbreak", samplePayload.Name)
	assert.Equal(t, "1.0.0", samplePayload.Version)
	assert.Contains(t, samplePayload.Categories, CategoryJailbreak)
	assert.True(t, samplePayload.BuiltIn)
	assert.True(t, samplePayload.Enabled)

	// Verify it has parameters
	assert.NotEmpty(t, samplePayload.Parameters)
	actionParam := samplePayload.GetParameterByName("action")
	require.NotNil(t, actionParam, "should have 'action' parameter")
	assert.Equal(t, ParameterTypeString, actionParam.Type)
	assert.True(t, actionParam.Required)

	// Verify success indicators
	assert.NotEmpty(t, samplePayload.SuccessIndicators)
	assert.Equal(t, IndicatorContains, samplePayload.SuccessIndicators[0].Type)
}

func TestBuiltInPayloads_AllValid(t *testing.T) {
	payloads, err := LoadBuiltInPayloads()
	require.NoError(t, err)
	require.NotEmpty(t, payloads)

	// Every built-in payload must be valid
	for i, payload := range payloads {
		t.Run(payload.Name, func(t *testing.T) {
			// Validate structure
			err := payload.Validate()
			assert.NoError(t, err, "payload %d (%s) validation failed", i, payload.Name)

			// Verify required fields
			assert.NotEmpty(t, payload.ID, "payload must have ID")
			assert.NotEmpty(t, payload.Name, "payload must have name")
			assert.NotEmpty(t, payload.Version, "payload must have version")
			assert.NotEmpty(t, payload.Template, "payload must have template")
			assert.NotEmpty(t, payload.Categories, "payload must have categories")
			assert.NotEmpty(t, payload.SuccessIndicators, "payload must have success indicators")

			// Verify categories are valid
			for _, category := range payload.Categories {
				assert.True(t, category.IsValid(), "category %s must be valid", category)
			}

			// Verify indicator types are valid
			for j, indicator := range payload.SuccessIndicators {
				assert.True(t, indicator.Type.IsValid(),
					"indicator %d type must be valid", j)
				assert.NotEmpty(t, indicator.Value,
					"indicator %d must have value", j)
			}

			// Verify parameter types are valid
			for j, param := range payload.Parameters {
				assert.NotEmpty(t, param.Name, "parameter %d must have name", j)
				assert.True(t, param.Type.IsValid(),
					"parameter %d type must be valid", j)
			}

			// Verify MITRE techniques format (if present)
			for _, technique := range payload.MitreTechniques {
				assert.Contains(t, technique, "AML.T",
					"MITRE technique should follow AML.T format")
			}
		})
	}
}

func TestBuiltInPayloads_NoDuplicateIDs(t *testing.T) {
	payloads, err := LoadBuiltInPayloads()
	require.NoError(t, err)
	require.NotEmpty(t, payloads)

	// Check for duplicate IDs
	idMap := make(map[string]bool)
	for i, payload := range payloads {
		id := payload.ID.String()
		assert.False(t, idMap[id],
			"payload %d has duplicate ID: %s", i, id)
		idMap[id] = true
	}
}

func TestBuiltInPayloads_Coverage(t *testing.T) {
	payloads, err := LoadBuiltInPayloads()
	require.NoError(t, err)
	require.NotEmpty(t, payloads)

	// Track which categories are covered
	categoryCoverage := make(map[PayloadCategory]int)
	for _, payload := range payloads {
		for _, category := range payload.Categories {
			categoryCoverage[category]++
		}
	}

	// Log coverage for visibility
	t.Logf("Built-in payload category coverage:")
	for _, category := range AllCategories() {
		count := categoryCoverage[category]
		t.Logf("  %s: %d payloads", category, count)
	}

	// We should have at least the jailbreak category from sample
	assert.Greater(t, categoryCoverage[CategoryJailbreak], 0,
		"should have at least one jailbreak payload")
}

func BenchmarkBuiltInLoader_Load(b *testing.B) {
	for i := 0; i < b.N; i++ {
		loader := NewBuiltInLoader()
		_, err := loader.Load()
		if err != nil {
			b.Fatalf("load failed: %v", err)
		}
	}
}

func BenchmarkLoadBuiltInPayloads(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := LoadBuiltInPayloads()
		if err != nil {
			b.Fatalf("load failed: %v", err)
		}
	}
}
