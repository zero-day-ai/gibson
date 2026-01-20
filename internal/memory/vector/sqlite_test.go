package vector

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

// createTestSqliteStore creates a temporary SqliteVecStore for testing.
func createTestSqliteStore(t *testing.T, dims int) (*SqliteVecStore, string) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_vectors.db")

	store, err := NewSqliteVecStore(SqliteVecConfig{
		DBPath:    dbPath,
		TableName: "test_vectors",
		Dims:      dims,
	})
	require.NoError(t, err)
	require.NotNil(t, store)

	t.Cleanup(func() {
		store.Close()
	})

	return store, dbPath
}

func TestNewSqliteVecStore(t *testing.T) {
	tests := []struct {
		name      string
		config    SqliteVecConfig
		wantError bool
		errorCode types.ErrorCode
	}{
		{
			name: "valid configuration",
			config: SqliteVecConfig{
				DBPath:    filepath.Join(t.TempDir(), "vectors.db"),
				TableName: "vectors",
				Dims:      384,
			},
			wantError: false,
		},
		{
			name: "valid with default table name",
			config: SqliteVecConfig{
				DBPath: filepath.Join(t.TempDir(), "vectors.db"),
				Dims:   384,
			},
			wantError: false,
		},
		{
			name: "empty database path",
			config: SqliteVecConfig{
				TableName: "vectors",
				Dims:      384,
			},
			wantError: true,
			errorCode: ErrCodeInvalidConfig,
		},
		{
			name: "invalid dimensions",
			config: SqliteVecConfig{
				DBPath:    filepath.Join(t.TempDir(), "vectors.db"),
				TableName: "vectors",
				Dims:      0,
			},
			wantError: true,
			errorCode: ErrCodeInvalidConfig,
		},
		{
			name: "negative dimensions",
			config: SqliteVecConfig{
				DBPath:    filepath.Join(t.TempDir(), "vectors.db"),
				TableName: "vectors",
				Dims:      -1,
			},
			wantError: true,
			errorCode: ErrCodeInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewSqliteVecStore(tt.config)

			if tt.wantError {
				require.Error(t, err)
				gibsonErr, ok := err.(*types.GibsonError)
				require.True(t, ok)
				assert.Equal(t, tt.errorCode, gibsonErr.Code)
			} else {
				require.NoError(t, err)
				require.NotNil(t, store)
				assert.NoError(t, store.Close())
			}
		})
	}
}

func TestSqliteVecStore_StoreAndGet(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	embedding := make([]float64, 384)
	for i := range embedding {
		embedding[i] = float64(i) * 0.01
	}

	record := VectorRecord{
		ID:        "test-1",
		Content:   "This is a test document",
		Embedding: embedding,
		Metadata: map[string]any{
			"source": "test",
			"type":   "document",
		},
		CreatedAt: time.Now(),
	}

	// Store the record
	err := store.Store(ctx, record)
	require.NoError(t, err)

	// Retrieve the record
	retrieved, err := store.Get(ctx, "test-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, record.ID, retrieved.ID)
	assert.Equal(t, record.Content, retrieved.Content)
	assert.Equal(t, len(record.Embedding), len(retrieved.Embedding))
	assert.InDelta(t, record.Embedding[0], retrieved.Embedding[0], 0.0001)
	assert.Equal(t, "test", retrieved.Metadata["source"])
	assert.Equal(t, "document", retrieved.Metadata["type"])
}

func TestSqliteVecStore_StoreReplace(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	embedding := make([]float64, 384)
	for i := range embedding {
		embedding[i] = 0.5
	}

	// Store initial record
	record1 := VectorRecord{
		ID:        "test-1",
		Content:   "Original content",
		Embedding: embedding,
		CreatedAt: time.Now(),
	}
	err := store.Store(ctx, record1)
	require.NoError(t, err)

	// Replace with new content
	record2 := VectorRecord{
		ID:        "test-1",
		Content:   "Updated content",
		Embedding: embedding,
		CreatedAt: time.Now(),
	}
	err = store.Store(ctx, record2)
	require.NoError(t, err)

	// Verify updated content
	retrieved, err := store.Get(ctx, "test-1")
	require.NoError(t, err)
	assert.Equal(t, "Updated content", retrieved.Content)
}

func TestSqliteVecStore_GetNotFound(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	retrieved, err := store.Get(ctx, "nonexistent")
	require.Error(t, err)
	assert.Nil(t, retrieved)

	gibsonErr, ok := err.(*types.GibsonError)
	require.True(t, ok)
	assert.Equal(t, ErrCodeVectorNotFound, gibsonErr.Code)
}

func TestSqliteVecStore_StoreBatch(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	// Create multiple records
	records := make([]VectorRecord, 10)
	for i := range records {
		embedding := make([]float64, 384)
		for j := range embedding {
			embedding[j] = float64(i*100+j) * 0.001
		}

		records[i] = VectorRecord{
			ID:        string(rune('a' + i)),
			Content:   "Document " + string(rune('A'+i)),
			Embedding: embedding,
			Metadata: map[string]any{
				"index": i,
			},
			CreatedAt: time.Now(),
		}
	}

	// Store batch
	err := store.StoreBatch(ctx, records)
	require.NoError(t, err)

	// Verify all records were stored
	for i, record := range records {
		retrieved, err := store.Get(ctx, record.ID)
		require.NoError(t, err, "failed to retrieve record %d", i)
		assert.Equal(t, record.Content, retrieved.Content)
		assert.Equal(t, i, int(retrieved.Metadata["index"].(float64)))
	}
}

func TestSqliteVecStore_StoreBatchEmpty(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	err := store.StoreBatch(ctx, []VectorRecord{})
	require.NoError(t, err)
}

func TestSqliteVecStore_Search(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	// Create test records with different embeddings
	records := []VectorRecord{
		{
			ID:        "doc1",
			Content:   "Machine learning document",
			Embedding: createNormalizedEmbedding(384, 1.0, 0.0),
			Metadata:  map[string]any{"category": "ml"},
			CreatedAt: time.Now(),
		},
		{
			ID:        "doc2",
			Content:   "Deep learning tutorial",
			Embedding: createNormalizedEmbedding(384, 0.9, 0.1),
			Metadata:  map[string]any{"category": "ml"},
			CreatedAt: time.Now(),
		},
		{
			ID:        "doc3",
			Content:   "Cooking recipes",
			Embedding: createNormalizedEmbedding(384, 0.1, 0.9),
			Metadata:  map[string]any{"category": "food"},
			CreatedAt: time.Now(),
		},
	}

	err := store.StoreBatch(ctx, records)
	require.NoError(t, err)

	// Search with query similar to doc1
	query := VectorQuery{
		Embedding: createNormalizedEmbedding(384, 1.0, 0.0),
		TopK:      2,
		MinScore:  0.0,
	}

	results, err := store.Search(ctx, query)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// First result should be doc1 (highest similarity)
	assert.Equal(t, "doc1", results[0].Record.ID)
	assert.Greater(t, results[0].Score, 0.9)

	// Second result should be doc2
	assert.Equal(t, "doc2", results[1].Record.ID)
	assert.Less(t, results[1].Score, results[0].Score)
}

func TestSqliteVecStore_SearchWithThreshold(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	// Create test records with very different embeddings
	records := []VectorRecord{
		{
			ID:        "doc1",
			Content:   "Very similar",
			Embedding: createNormalizedEmbedding(384, 1.0, 0.0),
			CreatedAt: time.Now(),
		},
		{
			ID:        "doc2",
			Content:   "Somewhat similar",
			Embedding: createNormalizedEmbedding(384, 0.5, 0.5),
			CreatedAt: time.Now(),
		},
		{
			ID:        "doc3",
			Content:   "Not similar",
			Embedding: createNormalizedEmbedding(384, 0.0, 1.0),
			CreatedAt: time.Now(),
		},
	}

	err := store.StoreBatch(ctx, records)
	require.NoError(t, err)

	// Search with high threshold (should only return very similar docs)
	query := VectorQuery{
		Embedding: createNormalizedEmbedding(384, 1.0, 0.0),
		TopK:      10,
		MinScore:  0.95, // High threshold - only exact/near-exact matches
	}

	results, err := store.Search(ctx, query)
	require.NoError(t, err)

	// Should only return doc1 (above threshold)
	require.Len(t, results, 1)
	assert.Equal(t, "doc1", results[0].Record.ID)
	assert.Greater(t, results[0].Score, 0.95)
}

func TestSqliteVecStore_SearchWithFilters(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	embedding := createNormalizedEmbedding(384, 1.0, 0.0)

	// Create records with different metadata
	records := []VectorRecord{
		{
			ID:        "doc1",
			Content:   "ML document type A",
			Embedding: embedding,
			Metadata:  map[string]any{"category": "ml", "type": "tutorial"},
			CreatedAt: time.Now(),
		},
		{
			ID:        "doc2",
			Content:   "ML document type B",
			Embedding: embedding,
			Metadata:  map[string]any{"category": "ml", "type": "reference"},
			CreatedAt: time.Now(),
		},
		{
			ID:        "doc3",
			Content:   "Cooking document",
			Embedding: embedding,
			Metadata:  map[string]any{"category": "food", "type": "tutorial"},
			CreatedAt: time.Now(),
		},
	}

	err := store.StoreBatch(ctx, records)
	require.NoError(t, err)

	// Search with category filter
	query := VectorQuery{
		Embedding: embedding,
		TopK:      10,
		Filters:   map[string]any{"category": "ml"},
	}

	results, err := store.Search(ctx, query)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Verify only ML documents returned
	for _, result := range results {
		assert.Equal(t, "ml", result.Record.Metadata["category"])
	}
}

func TestSqliteVecStore_Delete(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	embedding := make([]float64, 384)
	for i := range embedding {
		embedding[i] = 0.5
	}

	record := VectorRecord{
		ID:        "test-1",
		Content:   "Test document",
		Embedding: embedding,
		CreatedAt: time.Now(),
	}

	// Store record
	err := store.Store(ctx, record)
	require.NoError(t, err)

	// Verify it exists
	_, err = store.Get(ctx, "test-1")
	require.NoError(t, err)

	// Delete record
	err = store.Delete(ctx, "test-1")
	require.NoError(t, err)

	// Verify it's gone
	_, err = store.Get(ctx, "test-1")
	require.Error(t, err)
	gibsonErr, ok := err.(*types.GibsonError)
	require.True(t, ok)
	assert.Equal(t, ErrCodeVectorNotFound, gibsonErr.Code)
}

func TestSqliteVecStore_Health(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	// Check health of empty store
	health := store.Health(ctx)
	assert.Equal(t, types.HealthStateHealthy, health.State)
	assert.Contains(t, health.Message, "0 records")

	// Add some records
	embedding := make([]float64, 384)
	for i := 0; i < 5; i++ {
		record := VectorRecord{
			ID:        string(rune('a' + i)),
			Content:   "Document",
			Embedding: embedding,
			CreatedAt: time.Now(),
		}
		err := store.Store(ctx, record)
		require.NoError(t, err)
	}

	// Check health with records
	health = store.Health(ctx)
	assert.Equal(t, types.HealthStateHealthy, health.State)
	assert.Contains(t, health.Message, "5 records")
}

func TestSqliteVecStore_HealthAfterClose(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	// Close the store
	err := store.Close()
	require.NoError(t, err)

	// Health should report unhealthy
	health := store.Health(ctx)
	assert.Equal(t, types.HealthStateUnhealthy, health.State)
	assert.Contains(t, health.Message, "closed")
}

func TestSqliteVecStore_Close(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_vectors.db")

	store, err := NewSqliteVecStore(SqliteVecConfig{
		DBPath: dbPath,
		Dims:   384,
	})
	require.NoError(t, err)

	// Close should succeed
	err = store.Close()
	require.NoError(t, err)

	// Second close should also succeed (idempotent)
	err = store.Close()
	require.NoError(t, err)

	// Operations after close should fail
	ctx := context.Background()
	embedding := make([]float64, 384)
	record := VectorRecord{
		ID:        "test",
		Content:   "test",
		Embedding: embedding,
		CreatedAt: time.Now(),
	}

	err = store.Store(ctx, record)
	require.Error(t, err)
	gibsonErr, ok := err.(*types.GibsonError)
	require.True(t, ok)
	assert.Equal(t, ErrCodeVectorStoreUnavailable, gibsonErr.Code)
}

func TestSqliteVecStore_PersistenceAcrossRestarts(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_vectors.db")
	ctx := context.Background()

	embedding := make([]float64, 384)
	for i := range embedding {
		embedding[i] = float64(i) * 0.01
	}

	// Create store and add records
	store1, err := NewSqliteVecStore(SqliteVecConfig{
		DBPath: dbPath,
		Dims:   384,
	})
	require.NoError(t, err)

	record := VectorRecord{
		ID:        "persistent-1",
		Content:   "This should persist",
		Embedding: embedding,
		Metadata: map[string]any{
			"important": true,
		},
		CreatedAt: time.Now(),
	}

	err = store1.Store(ctx, record)
	require.NoError(t, err)

	// Close the store
	err = store1.Close()
	require.NoError(t, err)

	// Open a new store instance with same database
	store2, err := NewSqliteVecStore(SqliteVecConfig{
		DBPath: dbPath,
		Dims:   384,
	})
	require.NoError(t, err)
	defer store2.Close()

	// Retrieve the record from new instance
	retrieved, err := store2.Get(ctx, "persistent-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, record.ID, retrieved.ID)
	assert.Equal(t, record.Content, retrieved.Content)
	assert.Equal(t, true, retrieved.Metadata["important"])
	assert.InDelta(t, record.Embedding[10], retrieved.Embedding[10], 0.0001)
}

func TestSqliteVecStore_ConcurrentAccess(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	const numGoroutines = 10
	const recordsPerGoroutine = 10

	// Concurrently write records
	done := make(chan bool, numGoroutines)
	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer func() { done <- true }()

			for i := 0; i < recordsPerGoroutine; i++ {
				embedding := make([]float64, 384)
				for j := range embedding {
					embedding[j] = float64(goroutineID*1000+i*10+j) * 0.0001
				}

				record := VectorRecord{
					ID:        string(rune('a'+goroutineID)) + string(rune('0'+i)),
					Content:   "Document from goroutine " + string(rune('0'+goroutineID)),
					Embedding: embedding,
					Metadata: map[string]any{
						"goroutine": goroutineID,
						"index":     i,
					},
					CreatedAt: time.Now(),
				}

				err := store.Store(ctx, record)
				require.NoError(t, err)
			}
		}(g)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all records were stored correctly
	health := store.Health(ctx)
	assert.Equal(t, types.HealthStateHealthy, health.State)
	assert.Contains(t, health.Message, "100 records")
}

func TestSqliteVecStore_DimensionValidation(t *testing.T) {
	store, _ := createTestSqliteStore(t, 384)
	ctx := context.Background()

	// Try to store record with wrong dimensions
	embedding := make([]float64, 512) // Wrong size
	record := VectorRecord{
		ID:        "test",
		Content:   "test",
		Embedding: embedding,
		CreatedAt: time.Now(),
	}

	err := store.Store(ctx, record)
	require.Error(t, err)
	gibsonErr, ok := err.(*types.GibsonError)
	require.True(t, ok)
	assert.Equal(t, ErrCodeVectorStoreFailed, gibsonErr.Code)
	assert.Contains(t, gibsonErr.Message, "dimensions mismatch")
}

// Helper function to create a normalized embedding vector.
func createNormalizedEmbedding(dims int, weight1, weight2 float64) []float64 {
	embedding := make([]float64, dims)
	for i := range embedding {
		if i < dims/2 {
			embedding[i] = weight1
		} else {
			embedding[i] = weight2
		}
	}
	// Normalize the vector
	var norm float64
	for _, v := range embedding {
		norm += v * v
	}
	norm = 1.0 / (norm + 1e-8) // Avoid division by zero
	for i := range embedding {
		embedding[i] *= norm
	}
	return embedding
}

func TestSqliteVecStore_TableNameIsolation(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "shared.db")
	ctx := context.Background()

	// Create two stores with different table names on same database
	store1, err := NewSqliteVecStore(SqliteVecConfig{
		DBPath:    dbPath,
		TableName: "vectors_store1",
		Dims:      384,
	})
	require.NoError(t, err)
	defer store1.Close()

	store2, err := NewSqliteVecStore(SqliteVecConfig{
		DBPath:    dbPath,
		TableName: "vectors_store2",
		Dims:      384,
	})
	require.NoError(t, err)
	defer store2.Close()

	embedding := make([]float64, 384)

	// Add record to store1
	record1 := VectorRecord{
		ID:        "doc1",
		Content:   "Store 1 document",
		Embedding: embedding,
		CreatedAt: time.Now(),
	}
	err = store1.Store(ctx, record1)
	require.NoError(t, err)

	// Add record to store2
	record2 := VectorRecord{
		ID:        "doc1", // Same ID but different store
		Content:   "Store 2 document",
		Embedding: embedding,
		CreatedAt: time.Now(),
	}
	err = store2.Store(ctx, record2)
	require.NoError(t, err)

	// Verify isolation - records should be independent
	retrieved1, err := store1.Get(ctx, "doc1")
	require.NoError(t, err)
	assert.Equal(t, "Store 1 document", retrieved1.Content)

	retrieved2, err := store2.Get(ctx, "doc1")
	require.NoError(t, err)
	assert.Equal(t, "Store 2 document", retrieved2.Content)
}

func TestSerializeDeserializeEmbedding(t *testing.T) {
	original := []float64{0.1, 0.2, -0.3, 0.4, -0.5, 1.0, -1.0, 0.0}

	// Serialize
	bytes, err := serializeEmbedding(original)
	require.NoError(t, err)
	assert.Len(t, bytes, len(original)*8)

	// Deserialize
	deserialized, err := deserializeEmbedding(bytes, len(original))
	require.NoError(t, err)
	require.Len(t, deserialized, len(original))

	// Verify values match
	for i := range original {
		assert.InDelta(t, original[i], deserialized[i], 1e-10, "mismatch at index %d", i)
	}
}

func TestSerializeEmbeddingEmpty(t *testing.T) {
	_, err := serializeEmbedding([]float64{})
	require.Error(t, err)
}

func TestDeserializeEmbeddingInvalidLength(t *testing.T) {
	bytes := []byte{1, 2, 3, 4, 5} // Invalid length
	_, err := deserializeEmbedding(bytes, 10)
	require.Error(t, err)
}
