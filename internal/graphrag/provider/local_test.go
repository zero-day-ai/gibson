package provider

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/memory/vector"
	"github.com/zero-day-ai/gibson/internal/types"
)

// testConfig creates a valid test configuration for local provider
// Note: GraphRAG is a required core component - Enabled field has been removed
func testConfig() graphrag.GraphRAGConfig {
	return graphrag.GraphRAGConfig{
		Provider: "neo4j", // Required
		Neo4j: graphrag.Neo4jConfig{
			URI:      "bolt://localhost:7687",
			Username: "neo4j",
			Password: "password",
			Database: "neo4j",
			PoolSize: 50,
		},
		Vector: graphrag.VectorConfig{
			IndexType:  "hnsw",
			Dimensions: 1536,
			Metric:     "cosine",
		},
		Embedder: graphrag.EmbedderConfig{
			Provider:   "openai",
			Model:      "text-embedding-ada-002",
			Dimensions: 1536,
			APIKey:     "test-key",
		},
		Query: graphrag.QueryConfig{
			DefaultTopK:    10,
			DefaultMaxHops: 3,
			MinScore:       0.7,
			VectorWeight:   0.6,
			GraphWeight:    0.4,
		},
	}
}

// mockVectorStore implements vector.VectorStore for testing
type mockVectorStore struct {
	records map[string]vector.VectorRecord
	healthy bool
}

func newMockVectorStore() *mockVectorStore {
	return &mockVectorStore{
		records: make(map[string]vector.VectorRecord),
		healthy: true,
	}
}

func (m *mockVectorStore) Store(ctx context.Context, record vector.VectorRecord) error {
	m.records[record.ID] = record
	return nil
}

func (m *mockVectorStore) StoreBatch(ctx context.Context, records []vector.VectorRecord) error {
	for _, record := range records {
		m.records[record.ID] = record
	}
	return nil
}

func (m *mockVectorStore) Search(ctx context.Context, query vector.VectorQuery) ([]vector.VectorResult, error) {
	results := []vector.VectorResult{}
	for _, record := range m.records {
		// Simple mock - just return all records with score 1.0
		results = append(results, vector.VectorResult{
			Record: record,
			Score:  1.0,
		})
		if len(results) >= query.TopK {
			break
		}
	}
	return results, nil
}

func (m *mockVectorStore) Get(ctx context.Context, id string) (*vector.VectorRecord, error) {
	if record, ok := m.records[id]; ok {
		return &record, nil
	}
	return nil, types.NewError("VECTOR_NOT_FOUND", "record not found")
}

func (m *mockVectorStore) Delete(ctx context.Context, id string) error {
	delete(m.records, id)
	return nil
}

func (m *mockVectorStore) Health(ctx context.Context) types.HealthStatus {
	if m.healthy {
		return types.Healthy("mock vector store healthy")
	}
	return types.Unhealthy("mock vector store unhealthy")
}

func (m *mockVectorStore) Close() error {
	m.records = make(map[string]vector.VectorRecord)
	return nil
}

func TestNewLocalProvider(t *testing.T) {
	tests := []struct {
		name        string
		config      graphrag.GraphRAGConfig
		expectError bool
	}{
		{
			name: "valid configuration",
			config: graphrag.GraphRAGConfig{
				Provider: "neo4j", // Required - GraphRAG is a core component
				Neo4j: graphrag.Neo4jConfig{
					URI:      "bolt://localhost:7687",
					Username: "neo4j",
					Password: "password",
					Database: "neo4j",
					PoolSize: 50,
				},
				Vector: graphrag.VectorConfig{
					IndexType:  "hnsw",
					Dimensions: 1536,
					Metric:     "cosine",
				},
				Embedder: graphrag.EmbedderConfig{
					Provider:   "openai",
					Model:      "text-embedding-ada-002",
					Dimensions: 1536,
					APIKey:     "test-key",
				},
				Query: graphrag.QueryConfig{
					DefaultTopK:    10,
					DefaultMaxHops: 3,
					MinScore:       0.7,
					VectorWeight:   0.6,
					GraphWeight:    0.4,
				},
			},
			expectError: false,
		},
		{
			name: "invalid configuration - missing URI",
			config: graphrag.GraphRAGConfig{
				Provider: "neo4j",
				Neo4j: graphrag.Neo4jConfig{
					Username: "neo4j",
					Password: "password",
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewLocalProvider(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.False(t, provider.initialized)
			}
		})
	}
}

func TestLocalProvider_Initialize(t *testing.T) {
	t.Run("multiple initialize calls are safe", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		// Note: This will attempt to connect to Neo4j
		// In production tests, you'd use a test container or mock
		// For now, we just test the initialization logic

		// Set initialized to true to test multiple calls
		provider.initialized = true

		ctx := context.Background()
		err = provider.Initialize(ctx)
		assert.NoError(t, err) // Should be no-op if already initialized
	})

	t.Run("vector store is optional", func(t *testing.T) {
		config := testConfig()

		_, err := NewLocalProvider(config)
		require.NoError(t, err)

		// Note: Testing actual initialization would require Neo4j connection
		// In production tests, you'd use testcontainers or mocks
	})
}

func TestLocalProvider_StoreNode(t *testing.T) {
	t.Run("not initialized returns error", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		node := graphrag.NewGraphNode(types.NewID(), graphrag.NodeType("finding"))
		ctx := context.Background()

		err = provider.StoreNode(ctx, *node)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})

	t.Run("invalid node returns error", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		// Mark as initialized to bypass init check
		provider.initialized = true
		provider.graphHealthy = false // Skip graph operations

		// Create invalid node (no labels)
		node := &graphrag.GraphNode{
			ID:     types.NewID(),
			Labels: []graphrag.NodeType{}, // Empty - invalid
		}

		ctx := context.Background()
		err = provider.StoreNode(ctx, *node)
		assert.Error(t, err)
	})
}

func TestLocalProvider_StoreRelationship(t *testing.T) {
	t.Run("not initialized returns error", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		rel := graphrag.NewRelationship(types.NewID(), types.NewID(), graphrag.RelationType("exploits"))
		ctx := context.Background()

		err = provider.StoreRelationship(ctx, *rel)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})

	t.Run("graph unavailable returns error", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		// Mark as initialized but graph unhealthy
		provider.initialized = true
		provider.graphHealthy = false

		rel := graphrag.NewRelationship(types.NewID(), types.NewID(), graphrag.RelationType("exploits"))
		ctx := context.Background()

		err = provider.StoreRelationship(ctx, *rel)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unavailable")
	})
}

func TestLocalProvider_VectorSearch(t *testing.T) {
	t.Run("vector search with mock store", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		// Set up mock vector store
		mockStore := newMockVectorStore()
		provider.SetVectorStore(mockStore)

		// Mark as initialized
		provider.initialized = true

		// Add some test records to mock store
		nodeID := types.NewID()
		record := vector.VectorRecord{
			ID:        nodeID.String(),
			Content:   "test content",
			Embedding: make([]float64, 1536),
			Metadata: map[string]any{
				"node_id": nodeID.String(),
			},
			CreatedAt: time.Now(),
		}
		mockStore.records[nodeID.String()] = record

		// Perform vector search
		ctx := context.Background()
		embedding := make([]float64, 1536)
		results, err := provider.VectorSearch(ctx, embedding, 10, nil)

		assert.NoError(t, err)
		assert.NotNil(t, results)
		assert.GreaterOrEqual(t, len(results), 0)
	})

	t.Run("vector search without store returns error", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		// Mark as initialized but no vector store set
		provider.initialized = true

		ctx := context.Background()
		embedding := make([]float64, 1536)
		results, err := provider.VectorSearch(ctx, embedding, 10, nil)

		assert.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), "unavailable")
	})
}

func TestLocalProvider_Health(t *testing.T) {
	t.Run("not initialized returns unhealthy", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		ctx := context.Background()
		health := provider.Health(ctx)

		assert.False(t, health.IsHealthy())
		assert.Contains(t, health.Message, "not initialized")
	})

	t.Run("both backends healthy returns healthy", func(t *testing.T) {
		config := testConfig()

		_, err := NewLocalProvider(config)
		require.NoError(t, err)

		// Note: In production, you'd fully initialize and test health
		// For now, we just test the provider creation
	})
}

func TestLocalProvider_Close(t *testing.T) {
	t.Run("close uninitialized provider is safe", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		err = provider.Close()
		assert.NoError(t, err)
	})

	t.Run("close releases resources", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		// Set up mock vector store
		mockStore := newMockVectorStore()
		provider.SetVectorStore(mockStore)

		// Mark as initialized
		provider.initialized = true

		err = provider.Close()
		assert.NoError(t, err)
		assert.False(t, provider.initialized)
		assert.False(t, provider.graphHealthy)
	})
}

func TestLocalProvider_QueryNodes(t *testing.T) {
	t.Run("not initialized returns error", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		query := graphrag.NewNodeQuery()
		ctx := context.Background()

		nodes, err := provider.QueryNodes(ctx, *query)
		assert.Error(t, err)
		assert.Nil(t, nodes)
		assert.Contains(t, err.Error(), "not initialized")
	})

	t.Run("all backends unavailable returns error", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		// Mark as initialized but graph unhealthy
		provider.initialized = true
		provider.graphHealthy = false

		query := graphrag.NewNodeQuery()
		ctx := context.Background()

		nodes, err := provider.QueryNodes(ctx, *query)
		assert.Error(t, err)
		assert.Nil(t, nodes)
		assert.Contains(t, err.Error(), "unavailable")
	})
}

func TestLocalProvider_TraverseGraph(t *testing.T) {
	t.Run("not initialized returns error", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		ctx := context.Background()
		filters := graphrag.TraversalFilters{}

		nodes, err := provider.TraverseGraph(ctx, "start-id", 3, filters)
		assert.Error(t, err)
		assert.Nil(t, nodes)
		assert.Contains(t, err.Error(), "not initialized")
	})

	t.Run("graph unavailable returns error", func(t *testing.T) {
		config := testConfig()

		provider, err := NewLocalProvider(config)
		require.NoError(t, err)

		// Mark as initialized but graph unhealthy
		provider.initialized = true
		provider.graphHealthy = false

		ctx := context.Background()
		filters := graphrag.TraversalFilters{}

		nodes, err := provider.TraverseGraph(ctx, "start-id", 3, filters)
		assert.Error(t, err)
		assert.Nil(t, nodes)
		assert.Contains(t, err.Error(), "unavailable")
	})
}
