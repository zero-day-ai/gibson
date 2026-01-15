package graphrag

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestNewGraphRAGStore_DisabledGraphRAG(t *testing.T) {
	// Test that when GraphRAG is disabled, NewGraphRAGStore creates a store with noop provider
	config := GraphRAGConfig{
		Enabled: false,
	}
	embedder := NewMockEmbedder()

	store, err := NewGraphRAGStore(config, embedder)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, store)

	// Verify it's a DefaultGraphRAGStore
	defaultStore, ok := store.(*DefaultGraphRAGStore)
	require.True(t, ok, "Expected *DefaultGraphRAGStore")

	// Verify provider is a noop provider
	_, ok = defaultStore.provider.(*noopProvider)
	assert.True(t, ok, "Expected noopProvider when GraphRAG is disabled")

	// Verify it's functional
	ctx := context.Background()
	health := store.Health(ctx)
	assert.True(t, health.IsHealthy(), "Noop provider should be healthy")
}

func TestNewGraphRAGStore_EnabledGraphRAG(t *testing.T) {
	// Test that when GraphRAG is enabled, NewGraphRAGStore returns a descriptive error
	config := GraphRAGConfig{
		Enabled:  true,
		Provider: "neo4j",
		Neo4j: Neo4jConfig{
			URI:      "bolt://localhost:7687",
			Username: "neo4j",
			Password: "password",
		},
	}
	embedder := NewMockEmbedder()

	store, err := NewGraphRAGStore(config, embedder)

	// Should fail with clear error message
	require.Error(t, err)
	require.Nil(t, store)

	// Verify error is a ConfigError
	var graphErr *GraphRAGError
	require.ErrorAs(t, err, &graphErr)
	assert.Equal(t, ErrCodeInvalidConfig, graphErr.Code)

	// Verify error contains helpful context
	assert.Contains(t, graphErr.Error(), "GraphRAG is enabled")
	assert.Contains(t, graphErr.Error(), "provider injection")
	assert.NotNil(t, graphErr.Context["solution"])
	assert.NotNil(t, graphErr.Context["provider_type"])
	assert.Equal(t, "neo4j", graphErr.Context["provider_type"])
}

func TestNewGraphRAGStoreWithProvider_Success(t *testing.T) {
	// Test that NewGraphRAGStoreWithProvider works with injected provider
	config := GraphRAGConfig{
		Enabled:  true,
		Provider: "neo4j",
		Neo4j: Neo4jConfig{
			URI:      "bolt://localhost:7687",
			Username: "neo4j",
			Password: "password",
			Database: "neo4j",
			PoolSize: 50,
		},
	}
	embedder := NewMockEmbedder()
	mockProvider := NewMockGraphRAGProvider()

	store, err := NewGraphRAGStoreWithProvider(config, embedder, mockProvider)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, store)

	// Verify it's a DefaultGraphRAGStore
	defaultStore, ok := store.(*DefaultGraphRAGStore)
	require.True(t, ok, "Expected *DefaultGraphRAGStore")

	// Verify provider is the injected mock
	assert.Equal(t, mockProvider, defaultStore.provider)
}

func TestNewGraphRAGStoreWithProvider_NilProvider(t *testing.T) {
	// Test that NewGraphRAGStoreWithProvider rejects nil provider
	config := GraphRAGConfig{
		Enabled:  true,
		Provider: "neo4j",
		Neo4j: Neo4jConfig{
			URI:      "bolt://localhost:7687",
			Username: "neo4j",
			Password: "password",
		},
	}
	embedder := NewMockEmbedder()

	store, err := NewGraphRAGStoreWithProvider(config, embedder, nil)

	// Should fail
	require.Error(t, err)
	require.Nil(t, store)
	assert.Contains(t, err.Error(), "provider cannot be nil")
}

func TestNewGraphRAGStoreWithProvider_NilEmbedder(t *testing.T) {
	// Test that NewGraphRAGStoreWithProvider rejects nil embedder
	config := GraphRAGConfig{
		Enabled:  true,
		Provider: "neo4j",
		Neo4j: Neo4jConfig{
			URI:      "bolt://localhost:7687",
			Username: "neo4j",
			Password: "password",
		},
	}
	mockProvider := NewMockGraphRAGProvider()

	store, err := NewGraphRAGStoreWithProvider(config, nil, mockProvider)

	// Should fail
	require.Error(t, err)
	require.Nil(t, store)
	assert.Contains(t, err.Error(), "embedder cannot be nil")
}

func TestNoopProvider_BasicOperations(t *testing.T) {
	// Test that the inline noop provider implements all operations correctly
	provider := &noopProvider{}
	ctx := context.Background()

	// All operations should succeed with empty/nil results
	err := provider.Initialize(ctx)
	assert.NoError(t, err)

	node := NewGraphNode(types.NewID(), NodeType("finding"))
	err = provider.StoreNode(ctx, *node)
	assert.NoError(t, err)

	rel := NewRelationship(types.NewID(), types.NewID(), RelationType("similar_to"))
	err = provider.StoreRelationship(ctx, *rel)
	assert.NoError(t, err)

	nodes, err := provider.QueryNodes(ctx, *NewNodeQuery())
	assert.NoError(t, err)
	assert.Empty(t, nodes)

	rels, err := provider.QueryRelationships(ctx, *NewRelQuery())
	assert.NoError(t, err)
	assert.Empty(t, rels)

	traversed, err := provider.TraverseGraph(ctx, "test", 3, TraversalFilters{})
	assert.NoError(t, err)
	assert.Empty(t, traversed)

	vectorResults, err := provider.VectorSearch(ctx, []float64{0.1, 0.2}, 10, nil)
	assert.NoError(t, err)
	assert.Empty(t, vectorResults)

	health := provider.Health(ctx)
	assert.True(t, health.IsHealthy())
	assert.Contains(t, health.Message, "noop provider")

	err = provider.Close()
	assert.NoError(t, err)
}
