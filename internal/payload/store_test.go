package payload

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// setupTestStore creates a test database with migrations and returns a PayloadStore
func setupTestStore(t *testing.T) (*database.DB, PayloadStore, func()) {
	t.Helper()

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "payload-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")

	// Open database
	db, err := database.Open(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open database: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	// Run migrations
	migrator := database.NewMigrator(db)
	ctx := context.Background()
	if err := migrator.Migrate(ctx); err != nil {
		cleanup()
		t.Fatalf("failed to run migrations: %v", err)
	}

	store := NewPayloadStore(db)

	return db, store, cleanup
}

// createTestPayload creates a test payload with default values
func createTestPayload(name string) *Payload {
	return &Payload{
		ID:          types.NewID(),
		Name:        name,
		Version:     "1.0.0",
		Description: "Test payload: " + name,
		Categories:  []PayloadCategory{CategoryJailbreak},
		Tags:        []string{"test", "jailbreak"},
		Template:    "You are {{role}}. {{instruction}}",
		Parameters: []ParameterDef{
			{
				Name:        "role",
				Type:        ParameterTypeString,
				Description: "The role to assume",
				Required:    true,
			},
			{
				Name:        "instruction",
				Type:        ParameterTypeString,
				Description: "The instruction to execute",
				Required:    true,
			},
		},
		SuccessIndicators: []SuccessIndicator{
			{
				Type:   IndicatorContains,
				Value:  "success",
				Weight: 1.0,
			},
		},
		TargetTypes:     []string{"openai", "anthropic"},
		Severity:        agent.SeverityHigh,
		MitreTechniques: []string{"AML.T0051"},
		Metadata: PayloadMetadata{
			Author:      "test-author",
			Source:      "test",
			References:  []string{"https://example.com"},
			Notes:       "Test payload for unit tests",
			Difficulty:  "medium",
			Reliability: 0.8,
		},
		BuiltIn:   false,
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// TestPayloadStore_Save tests saving a payload
func TestPayloadStore_Save(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	payload := createTestPayload("test-save")

	err := store.Save(ctx, payload)
	require.NoError(t, err, "Save should succeed")

	// Verify the payload was saved by retrieving it
	retrieved, err := store.Get(ctx, payload.ID)
	require.NoError(t, err, "Get should succeed")
	assert.Equal(t, payload.ID, retrieved.ID)
	assert.Equal(t, payload.Name, retrieved.Name)
	assert.Equal(t, payload.Version, retrieved.Version)
	assert.Equal(t, payload.Description, retrieved.Description)
	assert.Equal(t, payload.Categories, retrieved.Categories)
	assert.Equal(t, payload.Tags, retrieved.Tags)
	assert.Equal(t, payload.Template, retrieved.Template)
	assert.Len(t, retrieved.Parameters, len(payload.Parameters))
	assert.Len(t, retrieved.SuccessIndicators, len(payload.SuccessIndicators))
	assert.Equal(t, payload.Severity, retrieved.Severity)
}

// TestPayloadStore_Save_Validation tests that invalid payloads are rejected
func TestPayloadStore_Save_Validation(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name    string
		payload *Payload
		wantErr bool
	}{
		{
			name:    "nil payload",
			payload: nil,
			wantErr: true,
		},
		{
			name: "empty name",
			payload: &Payload{
				ID:          types.NewID(),
				Name:        "",
				Version:     "1.0.0",
				Description: "test",
				Categories:  []PayloadCategory{CategoryJailbreak},
				Template:    "test",
				SuccessIndicators: []SuccessIndicator{
					{Type: IndicatorContains, Value: "test", Weight: 1.0},
				},
				Severity:  agent.SeverityHigh,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "no categories",
			payload: &Payload{
				ID:          types.NewID(),
				Name:        "test",
				Version:     "1.0.0",
				Description: "test",
				Categories:  []PayloadCategory{},
				Template:    "test",
				SuccessIndicators: []SuccessIndicator{
					{Type: IndicatorContains, Value: "test", Weight: 1.0},
				},
				Severity:  agent.SeverityHigh,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "empty template",
			payload: &Payload{
				ID:          types.NewID(),
				Name:        "test",
				Version:     "1.0.0",
				Description: "test",
				Categories:  []PayloadCategory{CategoryJailbreak},
				Template:    "",
				SuccessIndicators: []SuccessIndicator{
					{Type: IndicatorContains, Value: "test", Weight: 1.0},
				},
				Severity:  agent.SeverityHigh,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Save(ctx, tt.payload)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestPayloadStore_Get tests retrieving a payload by ID
func TestPayloadStore_Get(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	payload := createTestPayload("test-get")

	// Save payload
	err := store.Save(ctx, payload)
	require.NoError(t, err)

	// Retrieve payload
	retrieved, err := store.Get(ctx, payload.ID)
	require.NoError(t, err)
	assert.Equal(t, payload.ID, retrieved.ID)
	assert.Equal(t, payload.Name, retrieved.Name)

	// Test non-existent payload
	nonExistentID := types.NewID()
	_, err = store.Get(ctx, nonExistentID)
	assert.Error(t, err, "should return error for non-existent payload")
}

// TestPayloadStore_List tests listing payloads with filtering
func TestPayloadStore_List(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create test payloads
	payload1 := createTestPayload("test-list-1")
	payload1.Categories = []PayloadCategory{CategoryJailbreak}
	payload1.Severity = agent.SeverityHigh
	payload1.BuiltIn = true

	payload2 := createTestPayload("test-list-2")
	payload2.Categories = []PayloadCategory{CategoryPromptInjection}
	payload2.Severity = agent.SeverityMedium
	payload2.BuiltIn = false

	payload3 := createTestPayload("test-list-3")
	payload3.Categories = []PayloadCategory{CategoryJailbreak, CategoryDataExtraction}
	payload3.Severity = agent.SeverityHigh
	payload3.Enabled = false

	require.NoError(t, store.Save(ctx, payload1))
	require.NoError(t, store.Save(ctx, payload2))
	require.NoError(t, store.Save(ctx, payload3))

	tests := []struct {
		name          string
		filter        *PayloadFilter
		expectedCount int
		checkFunc     func(*testing.T, []*Payload)
	}{
		{
			name:          "list all",
			filter:        nil,
			expectedCount: 3,
		},
		{
			name: "filter by category",
			filter: &PayloadFilter{
				Categories: []PayloadCategory{CategoryJailbreak},
			},
			expectedCount: 2,
			checkFunc: func(t *testing.T, payloads []*Payload) {
				for _, p := range payloads {
					assert.Contains(t, p.Categories, CategoryJailbreak)
				}
			},
		},
		{
			name: "filter by severity",
			filter: &PayloadFilter{
				Severities: []agent.FindingSeverity{agent.SeverityHigh},
			},
			expectedCount: 2,
			checkFunc: func(t *testing.T, payloads []*Payload) {
				for _, p := range payloads {
					assert.Equal(t, agent.SeverityHigh, p.Severity)
				}
			},
		},
		{
			name: "filter by built-in",
			filter: &PayloadFilter{
				BuiltIn: boolPtr(true),
			},
			expectedCount: 1,
			checkFunc: func(t *testing.T, payloads []*Payload) {
				for _, p := range payloads {
					assert.True(t, p.BuiltIn)
				}
			},
		},
		{
			name: "filter by enabled",
			filter: &PayloadFilter{
				Enabled: boolPtr(false),
			},
			expectedCount: 1,
			checkFunc: func(t *testing.T, payloads []*Payload) {
				for _, p := range payloads {
					assert.False(t, p.Enabled)
				}
			},
		},
		{
			name: "filter by IDs",
			filter: &PayloadFilter{
				IDs: []types.ID{payload1.ID, payload2.ID},
			},
			expectedCount: 2,
		},
		{
			name: "filter by tags",
			filter: &PayloadFilter{
				Tags: []string{"test"},
			},
			expectedCount: 3,
		},
		{
			name: "filter with limit",
			filter: &PayloadFilter{
				Limit: 2,
			},
			expectedCount: 2,
		},
		{
			name: "filter with offset",
			filter: &PayloadFilter{
				Limit:  10,
				Offset: 2,
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloads, err := store.List(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, payloads, tt.expectedCount)

			if tt.checkFunc != nil {
				tt.checkFunc(t, payloads)
			}
		})
	}
}

// TestPayloadStore_Search tests full-text search
func TestPayloadStore_Search(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create test payloads with different content
	payload1 := createTestPayload("jailbreak-dan")
	payload1.Description = "DAN variant for jailbreaking ChatGPT"

	payload2 := createTestPayload("prompt-injection")
	payload2.Description = "Inject malicious instructions via user input"

	payload3 := createTestPayload("data-extraction")
	payload3.Description = "Extract sensitive data from RAG system"

	require.NoError(t, store.Save(ctx, payload1))
	require.NoError(t, store.Save(ctx, payload2))
	require.NoError(t, store.Save(ctx, payload3))

	tests := []struct {
		name        string
		query       string
		minExpected int
		shouldFind  []string
	}{
		{
			name:        "search for jailbreak",
			query:       "jailbreak",
			minExpected: 1,
			shouldFind:  []string{"jailbreak-dan"},
		},
		{
			name:        "search for DAN",
			query:       "DAN",
			minExpected: 1,
			shouldFind:  []string{"jailbreak-dan"},
		},
		{
			name:        "search for injection",
			query:       "injection",
			minExpected: 1,
			shouldFind:  []string{"prompt-injection"},
		},
		{
			name:        "search for RAG",
			query:       "RAG",
			minExpected: 1,
			shouldFind:  []string{"data-extraction"},
		},
		{
			name:        "empty query returns all",
			query:       "",
			minExpected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := store.Search(ctx, tt.query, nil)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(results), tt.minExpected)

			if len(tt.shouldFind) > 0 {
				names := make([]string, len(results))
				for i, p := range results {
					names[i] = p.Name
				}
				for _, expected := range tt.shouldFind {
					assert.Contains(t, names, expected)
				}
			}
		})
	}
}

// TestPayloadStore_Search_WithFilters tests search with additional filters
func TestPayloadStore_Search_WithFilters(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	payload1 := createTestPayload("high-severity-jailbreak")
	payload1.Severity = agent.SeverityHigh
	payload1.Description = "High severity jailbreak attack"

	payload2 := createTestPayload("medium-severity-jailbreak")
	payload2.Severity = agent.SeverityMedium
	payload2.Description = "Medium severity jailbreak attack"

	require.NoError(t, store.Save(ctx, payload1))
	require.NoError(t, store.Save(ctx, payload2))

	// Search for "jailbreak" but only high severity
	results, err := store.Search(ctx, "jailbreak", &PayloadFilter{
		Severities: []agent.FindingSeverity{agent.SeverityHigh},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "high-severity-jailbreak", results[0].Name)
}

// TestPayloadStore_Update tests updating a payload
func TestPayloadStore_Update(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	payload := createTestPayload("test-update")

	// Save initial payload
	err := store.Save(ctx, payload)
	require.NoError(t, err)

	// Update payload
	payload.Description = "Updated description"
	payload.Version = "1.1.0"
	payload.Severity = agent.SeverityCritical

	err = store.Update(ctx, payload)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := store.Get(ctx, payload.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated description", retrieved.Description)
	assert.Equal(t, "1.1.0", retrieved.Version)
	assert.Equal(t, agent.SeverityCritical, retrieved.Severity)
}

// TestPayloadStore_Update_NonExistent tests updating non-existent payload
func TestPayloadStore_Update_NonExistent(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	payload := createTestPayload("test-nonexistent")

	err := store.Update(ctx, payload)
	assert.Error(t, err, "should fail to update non-existent payload")
}

// TestPayloadStore_Delete tests soft-deleting a payload
func TestPayloadStore_Delete(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	payload := createTestPayload("test-delete")

	// Save payload
	err := store.Save(ctx, payload)
	require.NoError(t, err)

	// Delete payload
	err = store.Delete(ctx, payload.ID)
	require.NoError(t, err)

	// Verify payload is disabled (soft deleted)
	retrieved, err := store.Get(ctx, payload.ID)
	require.NoError(t, err)
	assert.False(t, retrieved.Enabled, "deleted payload should be disabled")

	// Verify it doesn't appear in enabled-only lists
	payloads, err := store.List(ctx, &PayloadFilter{
		Enabled: boolPtr(true),
	})
	require.NoError(t, err)

	for _, p := range payloads {
		assert.NotEqual(t, payload.ID, p.ID, "deleted payload should not appear in enabled list")
	}
}

// TestPayloadStore_GetVersionHistory tests version history tracking
func TestPayloadStore_GetVersionHistory(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	payload := createTestPayload("test-versioning")

	// Save initial version
	err := store.Save(ctx, payload)
	require.NoError(t, err)

	// Update payload (creates new version)
	payload.Description = "Version 2"
	payload.Version = "2.0.0"
	err = store.Update(ctx, payload)
	require.NoError(t, err)

	// Update again
	payload.Description = "Version 3"
	payload.Version = "3.0.0"
	err = store.Update(ctx, payload)
	require.NoError(t, err)

	// Get version history
	versions, err := store.GetVersionHistory(ctx, payload.ID)
	require.NoError(t, err)

	// Should have 3 versions (initial + 2 updates)
	assert.GreaterOrEqual(t, len(versions), 2, "should have at least initial and update versions")

	// Verify versions are in descending order (newest first)
	if len(versions) >= 2 {
		assert.True(t, versions[0].CreatedAt.After(versions[1].CreatedAt) || versions[0].CreatedAt.Equal(versions[1].CreatedAt))
	}
}

// TestPayloadStore_Exists tests checking payload existence
func TestPayloadStore_Exists(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	payload := createTestPayload("test-exists")

	// Should not exist initially
	exists, err := store.Exists(ctx, payload.ID)
	require.NoError(t, err)
	assert.False(t, exists)

	// Save payload
	err = store.Save(ctx, payload)
	require.NoError(t, err)

	// Should exist now
	exists, err = store.Exists(ctx, payload.ID)
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestPayloadStore_ExistsByName tests checking payload existence by name
func TestPayloadStore_ExistsByName(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	payload := createTestPayload("test-exists-by-name")

	// Should not exist initially
	exists, err := store.ExistsByName(ctx, payload.Name)
	require.NoError(t, err)
	assert.False(t, exists)

	// Save payload
	err = store.Save(ctx, payload)
	require.NoError(t, err)

	// Should exist now
	exists, err = store.ExistsByName(ctx, payload.Name)
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestPayloadStore_Count tests counting payloads
func TestPayloadStore_Count(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Initial count should be 0
	count, err := store.Count(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Create and save payloads
	payload1 := createTestPayload("test-count-1")
	payload1.Categories = []PayloadCategory{CategoryJailbreak}

	payload2 := createTestPayload("test-count-2")
	payload2.Categories = []PayloadCategory{CategoryPromptInjection}

	payload3 := createTestPayload("test-count-3")
	payload3.Categories = []PayloadCategory{CategoryJailbreak}

	require.NoError(t, store.Save(ctx, payload1))
	require.NoError(t, store.Save(ctx, payload2))
	require.NoError(t, store.Save(ctx, payload3))

	// Count all
	count, err = store.Count(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Count by category
	count, err = store.Count(ctx, &PayloadFilter{
		Categories: []PayloadCategory{CategoryJailbreak},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// TestPayloadStore_ConcurrentAccess tests thread-safe concurrent access
func TestPayloadStore_ConcurrentAccess(t *testing.T) {
	_, store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	const numGoroutines = 10

	// Create and save initial payload
	payload := createTestPayload("test-concurrent")
	err := store.Save(ctx, payload)
	require.NoError(t, err)

	// Concurrently read the payload
	done := make(chan bool, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := store.Get(ctx, payload.ID)
			assert.NoError(t, err)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}
