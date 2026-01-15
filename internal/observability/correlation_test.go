package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCorrelationID(t *testing.T) {
	t.Run("generates valid UUID", func(t *testing.T) {
		id := GenerateCorrelationID()

		assert.NotEmpty(t, id)
		assert.False(t, id.IsZero())

		// Verify it's a valid UUID
		_, err := uuid.Parse(id.String())
		assert.NoError(t, err)
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		id1 := GenerateCorrelationID()
		id2 := GenerateCorrelationID()

		assert.NotEqual(t, id1, id2)
	})
}

func TestCorrelationID_String(t *testing.T) {
	id := GenerateCorrelationID()
	str := id.String()

	assert.NotEmpty(t, str)
	assert.Equal(t, string(id), str)
}

func TestCorrelationID_IsZero(t *testing.T) {
	tests := []struct {
		name     string
		id       CorrelationID
		expected bool
	}{
		{
			name:     "empty ID is zero",
			id:       CorrelationID(""),
			expected: true,
		},
		{
			name:     "valid ID is not zero",
			id:       GenerateCorrelationID(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.id.IsZero())
		})
	}
}

func TestCorrelationID_Validate(t *testing.T) {
	tests := []struct {
		name      string
		id        CorrelationID
		wantError bool
		errorCode ObservabilityErrorCode
	}{
		{
			name:      "valid UUID passes validation",
			id:        GenerateCorrelationID(),
			wantError: false,
		},
		{
			name:      "empty ID fails validation",
			id:        CorrelationID(""),
			wantError: true,
			errorCode: ErrSpanContextMissing,
		},
		{
			name:      "invalid UUID format fails validation",
			id:        CorrelationID("not-a-uuid"),
			wantError: true,
			errorCode: ErrSpanContextMissing,
		},
		{
			name:      "partial UUID fails validation",
			id:        CorrelationID("123e4567-e89b-12d3"),
			wantError: true,
			errorCode: ErrSpanContextMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()

			if tt.wantError {
				require.Error(t, err)

				var obsErr *ObservabilityError
				if assert.True(t, errors.As(err, &obsErr)) {
					assert.Equal(t, tt.errorCode, obsErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInMemoryCorrelationStore_StoreCorrelation(t *testing.T) {
	store := NewInMemoryCorrelationStore()
	ctx := context.Background()

	t.Run("stores valid correlation", func(t *testing.T) {
		nodeID := "node-1"
		spanID := "span-1"

		err := store.StoreCorrelation(ctx, nodeID, spanID)
		require.NoError(t, err)

		assert.Equal(t, 1, store.Count())
	})

	t.Run("rejects empty node ID", func(t *testing.T) {
		err := store.StoreCorrelation(ctx, "", "span-1")
		require.Error(t, err)

		var obsErr *ObservabilityError
		if assert.True(t, errors.As(err, &obsErr)) {
			assert.Equal(t, ErrSpanContextMissing, obsErr.Code)
		}
	})

	t.Run("rejects empty span ID", func(t *testing.T) {
		err := store.StoreCorrelation(ctx, "node-1", "")
		require.Error(t, err)

		var obsErr *ObservabilityError
		if assert.True(t, errors.As(err, &obsErr)) {
			assert.Equal(t, ErrSpanContextMissing, obsErr.Code)
		}
	})

	t.Run("overwrites existing correlation", func(t *testing.T) {
		store := NewInMemoryCorrelationStore()

		// Store initial correlation
		err := store.StoreCorrelation(ctx, "node-1", "span-1")
		require.NoError(t, err)

		// Overwrite with new span
		err = store.StoreCorrelation(ctx, "node-1", "span-2")
		require.NoError(t, err)

		// Should still have only one correlation
		assert.Equal(t, 1, store.Count())

		// Old span should not be found
		_, err = store.GetNodeForSpan(ctx, "span-1")
		assert.Error(t, err)

		// New span should be found
		nodeID, err := store.GetNodeForSpan(ctx, "span-2")
		require.NoError(t, err)
		assert.Equal(t, "node-1", nodeID)
	})
}

func TestInMemoryCorrelationStore_GetSpanForNode(t *testing.T) {
	store := NewInMemoryCorrelationStore()
	ctx := context.Background()

	t.Run("retrieves stored span ID", func(t *testing.T) {
		nodeID := "node-1"
		expectedSpanID := "span-1"

		err := store.StoreCorrelation(ctx, nodeID, expectedSpanID)
		require.NoError(t, err)

		spanID, err := store.GetSpanForNode(ctx, nodeID)
		require.NoError(t, err)
		assert.Equal(t, expectedSpanID, spanID)
	})

	t.Run("returns error for non-existent node", func(t *testing.T) {
		_, err := store.GetSpanForNode(ctx, "non-existent")
		require.Error(t, err)

		var obsErr *ObservabilityError
		if assert.True(t, errors.As(err, &obsErr)) {
			assert.Equal(t, ErrSpanContextMissing, obsErr.Code)
			assert.Contains(t, obsErr.Message, "no correlation found")
		}
	})

	t.Run("rejects empty node ID", func(t *testing.T) {
		_, err := store.GetSpanForNode(ctx, "")
		require.Error(t, err)

		var obsErr *ObservabilityError
		if assert.True(t, errors.As(err, &obsErr)) {
			assert.Equal(t, ErrSpanContextMissing, obsErr.Code)
		}
	})
}

func TestInMemoryCorrelationStore_GetNodeForSpan(t *testing.T) {
	store := NewInMemoryCorrelationStore()
	ctx := context.Background()

	t.Run("retrieves stored node ID", func(t *testing.T) {
		expectedNodeID := "node-1"
		spanID := "span-1"

		err := store.StoreCorrelation(ctx, expectedNodeID, spanID)
		require.NoError(t, err)

		nodeID, err := store.GetNodeForSpan(ctx, spanID)
		require.NoError(t, err)
		assert.Equal(t, expectedNodeID, nodeID)
	})

	t.Run("returns error for non-existent span", func(t *testing.T) {
		_, err := store.GetNodeForSpan(ctx, "non-existent")
		require.Error(t, err)

		var obsErr *ObservabilityError
		if assert.True(t, errors.As(err, &obsErr)) {
			assert.Equal(t, ErrSpanContextMissing, obsErr.Code)
			assert.Contains(t, obsErr.Message, "no correlation found")
		}
	})

	t.Run("rejects empty span ID", func(t *testing.T) {
		_, err := store.GetNodeForSpan(ctx, "")
		require.Error(t, err)

		var obsErr *ObservabilityError
		if assert.True(t, errors.As(err, &obsErr)) {
			assert.Equal(t, ErrSpanContextMissing, obsErr.Code)
		}
	})
}

func TestInMemoryCorrelationStore_Bidirectional(t *testing.T) {
	store := NewInMemoryCorrelationStore()
	ctx := context.Background()

	t.Run("bidirectional lookup works", func(t *testing.T) {
		nodeID := "node-1"
		spanID := "span-1"

		// Store correlation
		err := store.StoreCorrelation(ctx, nodeID, spanID)
		require.NoError(t, err)

		// Lookup span from node
		retrievedSpanID, err := store.GetSpanForNode(ctx, nodeID)
		require.NoError(t, err)
		assert.Equal(t, spanID, retrievedSpanID)

		// Lookup node from span
		retrievedNodeID, err := store.GetNodeForSpan(ctx, spanID)
		require.NoError(t, err)
		assert.Equal(t, nodeID, retrievedNodeID)
	})

	t.Run("multiple correlations work independently", func(t *testing.T) {
		correlations := []struct {
			nodeID string
			spanID string
		}{
			{"node-1", "span-1"},
			{"node-2", "span-2"},
			{"node-3", "span-3"},
		}

		// Store all correlations
		for _, c := range correlations {
			err := store.StoreCorrelation(ctx, c.nodeID, c.spanID)
			require.NoError(t, err)
		}

		// Verify all correlations
		for _, c := range correlations {
			spanID, err := store.GetSpanForNode(ctx, c.nodeID)
			require.NoError(t, err)
			assert.Equal(t, c.spanID, spanID)

			nodeID, err := store.GetNodeForSpan(ctx, c.spanID)
			require.NoError(t, err)
			assert.Equal(t, c.nodeID, nodeID)
		}

		assert.Equal(t, len(correlations), store.Count())
	})
}

func TestInMemoryCorrelationStore_Clear(t *testing.T) {
	store := NewInMemoryCorrelationStore()
	ctx := context.Background()

	// Store some correlations
	err := store.StoreCorrelation(ctx, "node-1", "span-1")
	require.NoError(t, err)
	err = store.StoreCorrelation(ctx, "node-2", "span-2")
	require.NoError(t, err)

	assert.Equal(t, 2, store.Count())

	// Clear the store
	store.Clear()

	assert.Equal(t, 0, store.Count())

	// Verify correlations are gone
	_, err = store.GetSpanForNode(ctx, "node-1")
	assert.Error(t, err)
	_, err = store.GetNodeForSpan(ctx, "span-1")
	assert.Error(t, err)
}

func TestInMemoryCorrelationStore_Concurrency(t *testing.T) {
	store := NewInMemoryCorrelationStore()
	ctx := context.Background()

	// Test concurrent writes
	t.Run("concurrent writes are safe", func(t *testing.T) {
		const goroutines = 10
		const iterations = 100

		done := make(chan bool)

		for g := 0; g < goroutines; g++ {
			go func(id int) {
				for i := 0; i < iterations; i++ {
					nodeID := uuid.New().String()
					spanID := uuid.New().String()
					_ = store.StoreCorrelation(ctx, nodeID, spanID)
				}
				done <- true
			}(g)
		}

		// Wait for all goroutines
		for g := 0; g < goroutines; g++ {
			<-done
		}

		// Should have goroutines * iterations correlations
		assert.Equal(t, goroutines*iterations, store.Count())
	})

	// Test concurrent reads and writes
	t.Run("concurrent reads and writes are safe", func(t *testing.T) {
		store := NewInMemoryCorrelationStore()

		// Pre-populate some data
		for i := 0; i < 100; i++ {
			nodeID := uuid.New().String()
			spanID := uuid.New().String()
			_ = store.StoreCorrelation(ctx, nodeID, spanID)
		}

		const goroutines = 5
		const operations = 200

		done := make(chan bool)

		// Writers
		for g := 0; g < goroutines; g++ {
			go func() {
				for i := 0; i < operations; i++ {
					nodeID := uuid.New().String()
					spanID := uuid.New().String()
					_ = store.StoreCorrelation(ctx, nodeID, spanID)
				}
				done <- true
			}()
		}

		// Readers
		for g := 0; g < goroutines; g++ {
			go func() {
				for i := 0; i < operations; i++ {
					_ = store.Count()
					_, _ = store.GetSpanForNode(ctx, "non-existent")
					_, _ = store.GetNodeForSpan(ctx, "non-existent")
				}
				done <- true
			}()
		}

		// Wait for all goroutines
		for g := 0; g < goroutines*2; g++ {
			<-done
		}

		// Should not panic and should have reasonable count
		count := store.Count()
		assert.GreaterOrEqual(t, count, 100) // At least the pre-populated data
	})
}

func TestInMemoryCorrelationStore_ContextCancellation(t *testing.T) {
	store := NewInMemoryCorrelationStore()

	t.Run("operations respect context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Store should still work even with cancelled context
		// (In-memory store doesn't actually use context for I/O,
		// but it should accept it gracefully)
		err := store.StoreCorrelation(ctx, "node-1", "span-1")
		assert.NoError(t, err)

		spanID, err := store.GetSpanForNode(ctx, "node-1")
		assert.NoError(t, err)
		assert.Equal(t, "span-1", spanID)
	})

	t.Run("operations work with timeout context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := store.StoreCorrelation(ctx, "node-2", "span-2")
		assert.NoError(t, err)

		spanID, err := store.GetSpanForNode(ctx, "node-2")
		assert.NoError(t, err)
		assert.Equal(t, "span-2", spanID)
	})
}

// Benchmark tests for performance profiling
func BenchmarkGenerateCorrelationID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GenerateCorrelationID()
	}
}

func BenchmarkInMemoryCorrelationStore_StoreCorrelation(b *testing.B) {
	store := NewInMemoryCorrelationStore()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nodeID := uuid.New().String()
		spanID := uuid.New().String()
		_ = store.StoreCorrelation(ctx, nodeID, spanID)
	}
}

func BenchmarkInMemoryCorrelationStore_GetSpanForNode(b *testing.B) {
	store := NewInMemoryCorrelationStore()
	ctx := context.Background()

	// Pre-populate with data
	const size = 10000
	nodeIDs := make([]string, size)
	for i := 0; i < size; i++ {
		nodeID := uuid.New().String()
		spanID := uuid.New().String()
		nodeIDs[i] = nodeID
		_ = store.StoreCorrelation(ctx, nodeID, spanID)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nodeID := nodeIDs[i%size]
		_, _ = store.GetSpanForNode(ctx, nodeID)
	}
}

func BenchmarkInMemoryCorrelationStore_GetNodeForSpan(b *testing.B) {
	store := NewInMemoryCorrelationStore()
	ctx := context.Background()

	// Pre-populate with data
	const size = 10000
	spanIDs := make([]string, size)
	for i := 0; i < size; i++ {
		nodeID := uuid.New().String()
		spanID := uuid.New().String()
		spanIDs[i] = spanID
		_ = store.StoreCorrelation(ctx, nodeID, spanID)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		spanID := spanIDs[i%size]
		_, _ = store.GetNodeForSpan(ctx, spanID)
	}
}

func BenchmarkInMemoryCorrelationStore_ConcurrentAccess(b *testing.B) {
	store := NewInMemoryCorrelationStore()
	ctx := context.Background()

	// Pre-populate with data
	const size = 1000
	nodeIDs := make([]string, size)
	spanIDs := make([]string, size)
	for i := 0; i < size; i++ {
		nodeID := uuid.New().String()
		spanID := uuid.New().String()
		nodeIDs[i] = nodeID
		spanIDs[i] = spanID
		_ = store.StoreCorrelation(ctx, nodeID, spanID)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Mix of reads and writes
			if i%3 == 0 {
				nodeID := uuid.New().String()
				spanID := uuid.New().String()
				_ = store.StoreCorrelation(ctx, nodeID, spanID)
			} else if i%3 == 1 {
				nodeID := nodeIDs[i%size]
				_, _ = store.GetSpanForNode(ctx, nodeID)
			} else {
				spanID := spanIDs[i%size]
				_, _ = store.GetNodeForSpan(ctx, spanID)
			}
			i++
		}
	})
}
