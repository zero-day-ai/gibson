package embedder

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestMockEmbedder_Deterministic(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder()

	text := "test content"

	// Generate embedding twice
	embedding1, err := embedder.Embed(ctx, text)
	require.NoError(t, err)

	embedding2, err := embedder.Embed(ctx, text)
	require.NoError(t, err)

	// Should be identical
	require.Equal(t, len(embedding1), len(embedding2))
	for i := range embedding1 {
		assert.Equal(t, embedding1[i], embedding2[i],
			"embedding values should be deterministic at index %d", i)
	}
}

func TestMockEmbedder_DifferentTexts(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder()

	embedding1, err := embedder.Embed(ctx, "text one")
	require.NoError(t, err)

	embedding2, err := embedder.Embed(ctx, "text two")
	require.NoError(t, err)

	// Should be different
	different := false
	for i := range embedding1 {
		if embedding1[i] != embedding2[i] {
			different = true
			break
		}
	}
	assert.True(t, different, "different texts should produce different embeddings")
}

func TestMockEmbedder_Dimensions(t *testing.T) {
	tests := []struct {
		name       string
		dimensions int
		expected   int
	}{
		{
			name:       "1536 dimensions (OpenAI small)",
			dimensions: 1536,
			expected:   1536,
		},
		{
			name:       "3072 dimensions (OpenAI large)",
			dimensions: 3072,
			expected:   3072,
		},
		{
			name:       "custom dimensions",
			dimensions: 512,
			expected:   512,
		},
		{
			name:       "zero defaults to 1536",
			dimensions: 0,
			expected:   1536,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			embedder := NewMockEmbedder()
			if tt.dimensions > 0 {
				embedder.SetDimensions(tt.dimensions)
			}

			assert.Equal(t, tt.expected, embedder.Dimensions())

			// Verify embedding length matches
			embedding, err := embedder.Embed(ctx, "test")
			require.NoError(t, err)
			assert.Len(t, embedding, tt.expected)
		})
	}
}

func TestMockEmbedder_EmbedBatch(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder()

	texts := []string{"text one", "text two", "text three"}

	embeddings, err := embedder.EmbedBatch(ctx, texts)
	require.NoError(t, err)

	// Should return same number of embeddings as texts
	assert.Len(t, embeddings, len(texts))

	// Each embedding should have correct dimensions
	for i, embedding := range embeddings {
		assert.Len(t, embedding, 1536, "embedding %d has wrong dimensions", i)
	}

	// Embeddings should be deterministic - verify against individual Embed calls
	for i, text := range texts {
		individual, err := embedder.Embed(ctx, text)
		require.NoError(t, err)

		for j := range individual {
			assert.Equal(t, individual[j], embeddings[i][j],
				"batch embedding %d should match individual embedding at index %d", i, j)
		}
	}
}

func TestMockEmbedder_Model(t *testing.T) {
	embedder := NewMockEmbedder()
	assert.Equal(t, "mock-embedder", embedder.Model())

	// Test SetModel
	embedder.SetModel("custom-model")
	assert.Equal(t, "custom-model", embedder.Model())
}

func TestMockEmbedder_Health(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder()

	health := embedder.Health(ctx)
	assert.Equal(t, types.HealthStateHealthy, health.State)

	// Test SetHealthStatus
	unhealthyStatus := types.NewHealthStatus(types.HealthStateUnhealthy, "test unhealthy")
	embedder.SetHealthStatus(unhealthyStatus)

	health = embedder.Health(ctx)
	assert.Equal(t, types.HealthStateUnhealthy, health.State)
	assert.Equal(t, "test unhealthy", health.Message)
}

func TestMockEmbedder_CallTracking(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder()

	// Initially no calls
	assert.Equal(t, 0, embedder.CallCount())

	// Make some calls
	_, _ = embedder.Embed(ctx, "test 1")
	_, _ = embedder.EmbedBatch(ctx, []string{"test 2", "test 3"})
	_ = embedder.Health(ctx)

	// Should have 3 calls
	assert.Equal(t, 3, embedder.CallCount())

	// Check individual method calls
	embedCalls := embedder.GetCallsByMethod("Embed")
	assert.Len(t, embedCalls, 1)
	assert.Equal(t, "Embed", embedCalls[0].Method)

	batchCalls := embedder.GetCallsByMethod("EmbedBatch")
	assert.Len(t, batchCalls, 1)

	healthCalls := embedder.GetCallsByMethod("Health")
	assert.Len(t, healthCalls, 1)
}

func TestMockEmbedder_ErrorConfiguration(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder()

	// Configure Embed to return error
	testErr := types.NewError("TEST_ERROR", "test error message")
	embedder.SetEmbedError(testErr)

	_, err := embedder.Embed(ctx, "test")
	require.Error(t, err)
	assert.Equal(t, testErr, err)

	// Configure EmbedBatch to return error
	batchErr := types.NewError("BATCH_ERROR", "batch error")
	embedder.SetBatchError(batchErr)

	_, err = embedder.EmbedBatch(ctx, []string{"test"})
	require.Error(t, err)
	assert.Equal(t, batchErr, err)
}

func TestMockEmbedder_Reset(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder()

	// Make some calls and configure errors
	_, _ = embedder.Embed(ctx, "test")
	embedder.SetEmbedError(types.NewError("ERROR", "test"))
	embedder.SetModel("custom")

	assert.Equal(t, 1, embedder.CallCount())
	assert.Equal(t, "custom", embedder.Model())

	// Reset
	embedder.Reset()

	// Should be back to initial state
	assert.Equal(t, 0, embedder.CallCount())
	assert.Equal(t, "mock-embedder", embedder.Model())

	// Errors should be cleared
	_, err := embedder.Embed(ctx, "test")
	require.NoError(t, err)
}

func TestMockEmbedder_SetDimensions(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder()

	assert.Equal(t, 1536, embedder.Dimensions())

	// Change dimensions
	embedder.SetDimensions(3072)
	assert.Equal(t, 3072, embedder.Dimensions())

	// Embedding should have new dimensions
	embedding, err := embedder.Embed(ctx, "test")
	require.NoError(t, err)
	assert.Len(t, embedding, 3072)
}

func TestMockEmbedder_VectorNormalization(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder()

	embedding, err := embedder.Embed(ctx, "test content")
	require.NoError(t, err)

	// Calculate L2 norm (should be ~1.0 for normalized vector)
	var sumSquares float64
	for _, val := range embedding {
		sumSquares += val * val
	}

	// The mock embedder normalizes vectors, so the L2 norm should be approximately 1.0
	assert.InDelta(t, 1.0, sumSquares, 0.001, "embedding should be normalized to unit length")
}

func TestMockEmbedder_EmptyBatch(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder()

	embeddings, err := embedder.EmbedBatch(ctx, []string{})
	require.NoError(t, err)
	assert.Empty(t, embeddings)
}

func TestMockEmbedder_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder()

	// Run concurrent embeds
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			_, _ = embedder.Embed(ctx, "concurrent test")
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have recorded all 10 calls
	assert.Equal(t, 10, embedder.CallCount())
}
