package memory

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWorkingMemory(t *testing.T) {
	tests := []struct {
		name           string
		maxTokens      int
		expectedTokens int
	}{
		{
			name:           "with valid max tokens",
			maxTokens:      50000,
			expectedTokens: 50000,
		},
		{
			name:           "with zero defaults to 100000",
			maxTokens:      0,
			expectedTokens: 100000,
		},
		{
			name:           "with negative defaults to 100000",
			maxTokens:      -1000,
			expectedTokens: 100000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wm := NewWorkingMemory(tt.maxTokens)
			assert.NotNil(t, wm)
			assert.Equal(t, tt.expectedTokens, wm.MaxTokens())
			assert.Equal(t, 0, wm.TokenCount())
		})
	}
}

func TestWorkingMemory_SetAndGet(t *testing.T) {
	wm := NewWorkingMemory(10000)

	// Test setting and getting a value
	err := wm.Set("key1", "value1")
	require.NoError(t, err)

	value, ok := wm.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", value)

	// Test getting non-existent key
	value, ok = wm.Get("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, value)
}

func TestWorkingMemory_SetUpdate(t *testing.T) {
	wm := NewWorkingMemory(10000)

	// Set initial value
	err := wm.Set("key1", "initial")
	require.NoError(t, err)

	initialTokens := wm.TokenCount()
	assert.Greater(t, initialTokens, 0)

	// Update with new value
	err = wm.Set("key1", "updated value with more content")
	require.NoError(t, err)

	updatedTokens := wm.TokenCount()
	assert.Greater(t, updatedTokens, initialTokens) // More content = more tokens

	value, ok := wm.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "updated value with more content", value)
}

func TestWorkingMemory_Delete(t *testing.T) {
	wm := NewWorkingMemory(10000)

	// Set a value
	err := wm.Set("key1", "value1")
	require.NoError(t, err)

	initialTokens := wm.TokenCount()
	assert.Greater(t, initialTokens, 0)

	// Delete existing key
	deleted := wm.Delete("key1")
	assert.True(t, deleted)
	assert.Equal(t, 0, wm.TokenCount())

	// Verify it's gone
	_, ok := wm.Get("key1")
	assert.False(t, ok)

	// Delete non-existent key
	deleted = wm.Delete("nonexistent")
	assert.False(t, deleted)
}

func TestWorkingMemory_Clear(t *testing.T) {
	wm := NewWorkingMemory(10000)

	// Add multiple entries
	err := wm.Set("key1", "value1")
	require.NoError(t, err)
	err = wm.Set("key2", "value2")
	require.NoError(t, err)
	err = wm.Set("key3", "value3")
	require.NoError(t, err)

	assert.Greater(t, wm.TokenCount(), 0)
	assert.Len(t, wm.List(), 3)

	// Clear all
	wm.Clear()

	assert.Equal(t, 0, wm.TokenCount())
	assert.Len(t, wm.List(), 0)

	// Verify entries are gone
	_, ok := wm.Get("key1")
	assert.False(t, ok)
}

func TestWorkingMemory_List(t *testing.T) {
	wm := NewWorkingMemory(10000)

	// Empty list
	keys := wm.List()
	assert.Len(t, keys, 0)

	// Add entries
	err := wm.Set("key1", "value1")
	require.NoError(t, err)
	err = wm.Set("key2", "value2")
	require.NoError(t, err)
	err = wm.Set("key3", "value3")
	require.NoError(t, err)

	keys = wm.List()
	assert.Len(t, keys, 3)
	assert.Contains(t, keys, "key1")
	assert.Contains(t, keys, "key2")
	assert.Contains(t, keys, "key3")
}

func TestWorkingMemory_TokenCount(t *testing.T) {
	wm := NewWorkingMemory(10000)

	// Initial count is zero
	assert.Equal(t, 0, wm.TokenCount())

	// Add a value
	err := wm.Set("key1", "test value")
	require.NoError(t, err)
	tokens1 := wm.TokenCount()
	assert.Greater(t, tokens1, 0)

	// Add another value
	err = wm.Set("key2", "another test value")
	require.NoError(t, err)
	tokens2 := wm.TokenCount()
	assert.Greater(t, tokens2, tokens1)

	// Delete a value
	wm.Delete("key1")
	tokens3 := wm.TokenCount()
	assert.Less(t, tokens3, tokens2)
	assert.Greater(t, tokens3, 0) // key2 still there
}

func TestWorkingMemory_LRUEviction(t *testing.T) {
	// Use a small token limit to force eviction
	wm := NewWorkingMemory(150)

	// Add first entry (larger)
	largeValue1 := make([]byte, 200) // ~50 tokens
	for i := range largeValue1 {
		largeValue1[i] = 'a'
	}
	err := wm.Set("key1", string(largeValue1))
	require.NoError(t, err)

	// Wait a bit to ensure different timestamps
	time.Sleep(5 * time.Millisecond)

	// Add second entry (larger)
	largeValue2 := make([]byte, 200) // ~50 tokens
	for i := range largeValue2 {
		largeValue2[i] = 'b'
	}
	err = wm.Set("key2", string(largeValue2))
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(5 * time.Millisecond)

	// Access key1 to make it more recently used
	_, _ = wm.Get("key1")

	// Wait a bit to ensure timestamp difference
	time.Sleep(5 * time.Millisecond)

	t.Logf("Token count before adding key3: %d, max: %d", wm.TokenCount(), wm.MaxTokens())

	// Add a large entry that will trigger eviction
	largeValue3 := make([]byte, 400) // ~100 tokens, will push us over 150 limit
	for i := range largeValue3 {
		largeValue3[i] = 'x'
	}
	err = wm.Set("key3", string(largeValue3))
	require.NoError(t, err)

	// Should be under or at token limit after eviction (may exceed if single entry is large)
	// At minimum, eviction should have been triggered
	tokenCount := wm.TokenCount()
	t.Logf("Token count after eviction: %d, max: %d", tokenCount, wm.MaxTokens())

	// key2 should be evicted (least recently accessed)
	_, ok := wm.Get("key2")
	assert.False(t, ok, "key2 should have been evicted as LRU")

	// key3 should definitely exist (just added)
	_, ok = wm.Get("key3")
	assert.True(t, ok, "key3 should exist")
}

func TestWorkingMemory_EvictionOrder(t *testing.T) {
	// Use a token limit that will force eviction
	wm := NewWorkingMemory(80)

	// Add entries with delays to ensure timestamp ordering
	val1 := make([]byte, 100) // ~25 tokens
	for i := range val1 {
		val1[i] = 'a'
	}
	err := wm.Set("oldest", string(val1))
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)

	val2 := make([]byte, 100) // ~25 tokens
	for i := range val2 {
		val2[i] = 'b'
	}
	err = wm.Set("middle", string(val2))
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)

	val3 := make([]byte, 100) // ~25 tokens
	for i := range val3 {
		val3[i] = 'c'
	}
	err = wm.Set("newest", string(val3))
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)

	t.Logf("Token count before trigger: %d, max: %d", wm.TokenCount(), wm.MaxTokens())

	// Add a larger entry to trigger eviction
	largeValue := make([]byte, 200) // ~50 tokens, total would be ~125, over 80 limit
	for i := range largeValue {
		largeValue[i] = 'y'
	}
	err = wm.Set("trigger", string(largeValue))
	require.NoError(t, err)

	t.Logf("Token count after eviction: %d, max: %d", wm.TokenCount(), wm.MaxTokens())

	// At least the oldest entry should be gone (possibly more)
	_, ok := wm.Get("oldest")
	assert.False(t, ok, "oldest entry should be evicted first")

	// Trigger should exist (just added)
	_, ok = wm.Get("trigger")
	assert.True(t, ok, "newest entry should exist")
}

func TestWorkingMemory_ConcurrentAccess(t *testing.T) {
	wm := NewWorkingMemory(100000)

	var wg sync.WaitGroup
	numGoroutines := 10
	operationsPerGoroutine := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				err := wm.Set(key, fmt.Sprintf("value-%d-%d", id, j))
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify we have entries
	keys := wm.List()
	assert.Greater(t, len(keys), 0)
	assert.LessOrEqual(t, len(keys), numGoroutines*operationsPerGoroutine)

	// Concurrent reads and deletes
	wg = sync.WaitGroup{}
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				_, _ = wm.Get(key)
				if j%2 == 0 {
					_ = wm.Delete(key)
				}
			}
		}(i)
	}

	wg.Wait()

	// Should still be consistent
	assert.Equal(t, len(wm.List()), countEntries(wm))
}

func TestWorkingMemory_ComplexTypes(t *testing.T) {
	wm := NewWorkingMemory(10000)

	// Test with map
	mapValue := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
		"key3": []string{"a", "b", "c"},
	}
	err := wm.Set("map", mapValue)
	require.NoError(t, err)

	retrieved, ok := wm.Get("map")
	assert.True(t, ok)
	assert.Equal(t, mapValue, retrieved)

	// Test with struct
	type TestStruct struct {
		Name  string
		Count int
	}
	structValue := TestStruct{Name: "test", Count: 42}
	err = wm.Set("struct", structValue)
	require.NoError(t, err)

	retrieved, ok = wm.Get("struct")
	assert.True(t, ok)
	assert.Equal(t, structValue, retrieved)
}

func TestWorkingMemory_TokenEstimationAccuracy(t *testing.T) {
	wm := NewWorkingMemory(10000)

	// Short string
	err := wm.Set("short", "hi")
	require.NoError(t, err)
	shortTokens := wm.TokenCount()

	wm.Clear()

	// Long string
	longString := "this is a much longer string with many more characters that should result in a higher token count"
	err = wm.Set("long", longString)
	require.NoError(t, err)
	longTokens := wm.TokenCount()

	// Long should have more tokens than short
	assert.Greater(t, longTokens, shortTokens)

	// Rough sanity check: ~4 chars per token
	expectedTokens := len(longString) / 4
	assert.InDelta(t, expectedTokens, longTokens, float64(expectedTokens)*0.5) // Within 50%
}

// Helper function to count entries manually
func countEntries(wm WorkingMemory) int {
	count := 0
	keys := wm.List()
	for _, key := range keys {
		if _, ok := wm.Get(key); ok {
			count++
		}
	}
	return count
}
