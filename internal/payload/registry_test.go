package payload

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestNewPayloadRegistry tests registry creation
func TestNewPayloadRegistry(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	require.NotNil(t, registry)

	ctx := context.Background()
	health := registry.Health(ctx)
	assert.True(t, health.IsHealthy())
}

// TestPayloadRegistry_Register tests payload registration
func TestPayloadRegistry_Register(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	t.Run("register new payload", func(t *testing.T) {
		payload := createTestPayload("test-register-1")
		err := registry.Register(ctx, payload)
		require.NoError(t, err)

		// Verify it was registered
		retrieved, err := registry.Get(ctx, payload.ID)
		require.NoError(t, err)
		assert.Equal(t, payload.Name, retrieved.Name)
	})

	t.Run("register duplicate by ID fails", func(t *testing.T) {
		payload := createTestPayload("test-register-2")
		err := registry.Register(ctx, payload)
		require.NoError(t, err)

		// Try to register again
		err = registry.Register(ctx, payload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("register duplicate by name fails", func(t *testing.T) {
		payload1 := createTestPayload("test-register-3")
		err := registry.Register(ctx, payload1)
		require.NoError(t, err)

		// Create different payload with same name
		payload2 := createTestPayload("test-register-3")
		payload2.ID = types.NewID() // Different ID
		err = registry.Register(ctx, payload2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("register nil payload fails", func(t *testing.T) {
		err := registry.Register(ctx, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})
}

// TestPayloadRegistry_Get tests payload retrieval
func TestPayloadRegistry_Get(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	t.Run("get existing payload", func(t *testing.T) {
		payload := createTestPayload("test-get-1")
		err := registry.Register(ctx, payload)
		require.NoError(t, err)

		retrieved, err := registry.Get(ctx, payload.ID)
		require.NoError(t, err)
		assert.Equal(t, payload.Name, retrieved.Name)
		assert.Equal(t, payload.Description, retrieved.Description)
	})

	t.Run("get non-existent payload fails", func(t *testing.T) {
		nonExistentID := types.NewID()
		_, err := registry.Get(ctx, nonExistentID)
		assert.Error(t, err)
	})

	t.Run("get uses cache on second call", func(t *testing.T) {
		payload := createTestPayload("test-get-2")
		err := registry.Register(ctx, payload)
		require.NoError(t, err)

		// First call - cache miss
		retrieved1, err := registry.Get(ctx, payload.ID)
		require.NoError(t, err)

		// Second call - should use cache
		retrieved2, err := registry.Get(ctx, payload.ID)
		require.NoError(t, err)

		assert.Equal(t, retrieved1.Name, retrieved2.Name)

		// Verify cache contains the payload
		size, _ := registry.GetCacheStats()
		assert.Greater(t, size, 0)
	})
}

// TestPayloadRegistry_Update tests payload updates
func TestPayloadRegistry_Update(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	t.Run("update existing payload", func(t *testing.T) {
		payload := createTestPayload("test-update-1")
		err := registry.Register(ctx, payload)
		require.NoError(t, err)

		// Update description
		payload.Description = "Updated description"
		payload.UpdatedAt = time.Now()
		err = registry.Update(ctx, payload)
		require.NoError(t, err)

		// Verify update
		retrieved, err := registry.Get(ctx, payload.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated description", retrieved.Description)
	})

	t.Run("update invalidates cache", func(t *testing.T) {
		payload := createTestPayload("test-update-2")
		err := registry.Register(ctx, payload)
		require.NoError(t, err)

		// Get to populate cache
		_, err = registry.Get(ctx, payload.ID)
		require.NoError(t, err)

		// Update
		payload.Description = "New description"
		err = registry.Update(ctx, payload)
		require.NoError(t, err)

		// Get again - should get updated version from store
		retrieved, err := registry.Get(ctx, payload.ID)
		require.NoError(t, err)
		assert.Equal(t, "New description", retrieved.Description)
	})

	t.Run("update nil payload fails", func(t *testing.T) {
		err := registry.Update(ctx, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})
}

// TestPayloadRegistry_Disable tests payload disabling
func TestPayloadRegistry_Disable(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	t.Run("disable existing payload", func(t *testing.T) {
		payload := createTestPayload("test-disable-1")
		err := registry.Register(ctx, payload)
		require.NoError(t, err)

		// Disable
		err = registry.Disable(ctx, payload.ID)
		require.NoError(t, err)

		// Verify it's disabled
		retrieved, err := registry.Get(ctx, payload.ID)
		require.NoError(t, err)
		assert.False(t, retrieved.Enabled)
	})

	t.Run("disable non-existent payload fails", func(t *testing.T) {
		nonExistentID := types.NewID()
		err := registry.Disable(ctx, nonExistentID)
		assert.Error(t, err)
	})
}

// TestPayloadRegistry_Enable tests payload enabling
func TestPayloadRegistry_Enable(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	t.Run("enable disabled payload", func(t *testing.T) {
		payload := createTestPayload("test-enable-1")
		err := registry.Register(ctx, payload)
		require.NoError(t, err)

		// Disable first
		err = registry.Disable(ctx, payload.ID)
		require.NoError(t, err)

		// Enable
		err = registry.Enable(ctx, payload.ID)
		require.NoError(t, err)

		// Verify it's enabled
		retrieved, err := registry.Get(ctx, payload.ID)
		require.NoError(t, err)
		assert.True(t, retrieved.Enabled)
	})

	t.Run("enable non-existent payload fails", func(t *testing.T) {
		nonExistentID := types.NewID()
		err := registry.Enable(ctx, nonExistentID)
		assert.Error(t, err)
	})
}

// TestPayloadRegistry_GetByCategory tests category filtering
func TestPayloadRegistry_GetByCategory(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	// Register payloads with different categories
	jailbreak1 := createTestPayload("jailbreak-1")
	jailbreak1.Categories = []PayloadCategory{CategoryJailbreak}
	err := registry.Register(ctx, jailbreak1)
	require.NoError(t, err)

	jailbreak2 := createTestPayload("jailbreak-2")
	jailbreak2.Categories = []PayloadCategory{CategoryJailbreak}
	err = registry.Register(ctx, jailbreak2)
	require.NoError(t, err)

	injection := createTestPayload("injection-1")
	injection.Categories = []PayloadCategory{CategoryPromptInjection}
	err = registry.Register(ctx, injection)
	require.NoError(t, err)

	// Get by category
	jailbreaks, err := registry.GetByCategory(ctx, CategoryJailbreak)
	require.NoError(t, err)
	assert.Len(t, jailbreaks, 2)

	injections, err := registry.GetByCategory(ctx, CategoryPromptInjection)
	require.NoError(t, err)
	assert.Len(t, injections, 1)
}

// TestPayloadRegistry_GetByMitreTechnique tests MITRE technique filtering
func TestPayloadRegistry_GetByMitreTechnique(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	// Register payloads with different MITRE techniques
	payload1 := createTestPayload("mitre-1")
	payload1.MitreTechniques = []string{"AML.T0051"}
	err := registry.Register(ctx, payload1)
	require.NoError(t, err)

	payload2 := createTestPayload("mitre-2")
	payload2.MitreTechniques = []string{"AML.T0051"}
	err = registry.Register(ctx, payload2)
	require.NoError(t, err)

	payload3 := createTestPayload("mitre-3")
	payload3.MitreTechniques = []string{"AML.T0024"}
	err = registry.Register(ctx, payload3)
	require.NoError(t, err)

	// Get by MITRE technique
	t0051, err := registry.GetByMitreTechnique(ctx, "AML.T0051")
	require.NoError(t, err)
	assert.Len(t, t0051, 2)

	t0024, err := registry.GetByMitreTechnique(ctx, "AML.T0024")
	require.NoError(t, err)
	assert.Len(t, t0024, 1)
}

// TestPayloadRegistry_List tests payload listing
func TestPayloadRegistry_List(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	// Register multiple payloads
	for i := 0; i < 5; i++ {
		payload := createTestPayload(fmt.Sprintf("test-list-%d", i))
		err := registry.Register(ctx, payload)
		require.NoError(t, err)
	}

	t.Run("list all payloads", func(t *testing.T) {
		payloads, err := registry.List(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, payloads, 5)
	})

	t.Run("list with filter", func(t *testing.T) {
		filter := &PayloadFilter{
			Enabled: boolPtr(true),
		}
		payloads, err := registry.List(ctx, filter)
		require.NoError(t, err)
		assert.Len(t, payloads, 5)
	})
}

// TestPayloadRegistry_Search tests full-text search
func TestPayloadRegistry_Search(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	// Register payloads with distinct descriptions
	payload1 := createTestPayload("search-1")
	payload1.Description = "This is a special unicorn payload for testing"
	payload1.Tags = []string{"test", "unicorn"}  // Override default tags
	err := registry.Register(ctx, payload1)
	require.NoError(t, err)

	payload2 := createTestPayload("search-2")
	payload2.Description = "This is a prompt injection attack"
	payload2.Tags = []string{"test", "injection"}  // Override default tags
	err = registry.Register(ctx, payload2)
	require.NoError(t, err)

	t.Run("search by keyword", func(t *testing.T) {
		results, err := registry.Search(ctx, "unicorn", nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "search-1", results[0].Name)
	})

	t.Run("search with filter", func(t *testing.T) {
		filter := &PayloadFilter{
			Categories: []PayloadCategory{CategoryJailbreak},
		}
		results, err := registry.Search(ctx, "testing", filter)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})
}

// TestPayloadRegistry_Count tests payload counting
func TestPayloadRegistry_Count(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	// Register payloads
	for i := 0; i < 3; i++ {
		payload := createTestPayload(fmt.Sprintf("test-count-%d", i))
		err := registry.Register(ctx, payload)
		require.NoError(t, err)
	}

	t.Run("count all payloads", func(t *testing.T) {
		count, err := registry.Count(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("count with filter", func(t *testing.T) {
		filter := &PayloadFilter{
			Enabled: boolPtr(true),
		}
		count, err := registry.Count(ctx, filter)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})
}

// TestPayloadRegistry_ClearCache tests cache clearing
func TestPayloadRegistry_ClearCache(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	// Register and get a payload to populate cache
	payload := createTestPayload("test-cache-clear")
	err := registry.Register(ctx, payload)
	require.NoError(t, err)

	_, err = registry.Get(ctx, payload.ID)
	require.NoError(t, err)

	// Verify cache has items
	size, _ := registry.GetCacheStats()
	assert.Greater(t, size, 0)

	// Clear cache
	registry.ClearCache()

	// Verify cache is empty
	size, _ = registry.GetCacheStats()
	assert.Equal(t, 0, size)
}

// TestPayloadRegistry_ThreadSafety tests concurrent access
func TestPayloadRegistry_ThreadSafety(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	const numGoroutines = 10
	const numOperations = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				payload := createTestPayload(fmt.Sprintf("concurrent-%d-%d", id, j))
				_ = registry.Register(ctx, payload)
			}
		}(i)
	}

	wg.Wait()

	// Verify no corruption
	count, err := registry.Count(ctx, nil)
	require.NoError(t, err)
	assert.Greater(t, count, 0)
}

// TestPayloadRegistry_ConcurrentReadWrite tests concurrent reads and writes
func TestPayloadRegistry_ConcurrentReadWrite(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	// Pre-populate with some payloads
	payloadIDs := make([]types.ID, 10)
	for i := 0; i < 10; i++ {
		payload := createTestPayload(fmt.Sprintf("readwrite-%d", i))
		err := registry.Register(ctx, payload)
		require.NoError(t, err)
		payloadIDs[i] = payload.ID
	}

	const numGoroutines = 5
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				payloadID := payloadIDs[j%len(payloadIDs)]
				_, _ = registry.Get(ctx, payloadID)
			}
		}(i)
	}

	// Concurrent writers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				payload := createTestPayload(fmt.Sprintf("concurrent-write-%d-%d", id, j))
				_ = registry.Register(ctx, payload)
			}
		}(i)
	}

	wg.Wait()

	// Verify no corruption
	health := registry.Health(ctx)
	assert.True(t, health.IsHealthy())
}

// TestPayloadRegistry_LoadBuiltIns tests loading built-in payloads
func TestPayloadRegistry_LoadBuiltIns(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	t.Run("load built-in payloads", func(t *testing.T) {
		err := registry.LoadBuiltIns(ctx)
		// May error if some already exist, but should still load others
		if err != nil {
			t.Logf("LoadBuiltIns returned: %v", err)
		}

		// Verify some payloads were loaded
		count, err := registry.Count(ctx, &PayloadFilter{BuiltIn: boolPtr(true)})
		require.NoError(t, err)
		assert.Greater(t, count, 0, "Expected some built-in payloads to be loaded")
	})

	t.Run("load built-ins is idempotent", func(t *testing.T) {
		// First load
		err := registry.LoadBuiltIns(ctx)
		if err != nil && !contains(err.Error(), "already existed") {
			t.Fatalf("unexpected error on first load: %v", err)
		}

		count1, err := registry.Count(ctx, &PayloadFilter{BuiltIn: boolPtr(true)})
		require.NoError(t, err)

		// Second load should not increase count
		err = registry.LoadBuiltIns(ctx)
		if err != nil {
			t.Logf("Second LoadBuiltIns returned: %v", err)
		}

		count2, err := registry.Count(ctx, &PayloadFilter{BuiltIn: boolPtr(true)})
		require.NoError(t, err)
		assert.Equal(t, count1, count2, "Built-in count should not change on second load")
	})
}

// TestPayloadRegistry_CategoryStats tests category statistics
func TestPayloadRegistry_ListCategoryStats(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	// Register payloads with different categories
	jailbreak := createTestPayload("jailbreak")
	jailbreak.Categories = []PayloadCategory{CategoryJailbreak}
	err := registry.Register(ctx, jailbreak)
	require.NoError(t, err)

	injection := createTestPayload("injection")
	injection.Categories = []PayloadCategory{CategoryPromptInjection}
	err = registry.Register(ctx, injection)
	require.NoError(t, err)

	multiCat := createTestPayload("multi")
	multiCat.Categories = []PayloadCategory{CategoryJailbreak, CategoryDataExtraction}
	err = registry.Register(ctx, multiCat)
	require.NoError(t, err)

	// Get stats
	stats, err := registry.ListCategoryStats(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, stats[CategoryJailbreak])
	assert.Equal(t, 1, stats[CategoryPromptInjection])
	assert.Equal(t, 1, stats[CategoryDataExtraction])
}

// TestPayloadRegistry_MitreStats tests MITRE technique statistics
func TestPayloadRegistry_ListMitreStats(t *testing.T) {
	db, _, cleanup := setupTestStore(t)
	defer cleanup()

	registry := NewPayloadRegistryWithDefaults(db)
	ctx := context.Background()

	// Register payloads with different MITRE techniques
	payload1 := createTestPayload("payload1")
	payload1.MitreTechniques = []string{"AML.T0051"}
	err := registry.Register(ctx, payload1)
	require.NoError(t, err)

	payload2 := createTestPayload("payload2")
	payload2.MitreTechniques = []string{"AML.T0024", "AML.T0051"}
	err = registry.Register(ctx, payload2)
	require.NoError(t, err)

	// Get stats
	stats, err := registry.ListMitreStats(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, stats["AML.T0051"])
	assert.Equal(t, 1, stats["AML.T0024"])
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
