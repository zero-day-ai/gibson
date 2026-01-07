package verbose

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockMemoryStore is a mock implementation of memory.MemoryStore for testing.
type mockMemoryStore struct {
	working  memory.WorkingMemory
	mission  memory.MissionMemory
	longterm memory.LongTermMemory
}

func (m *mockMemoryStore) Working() memory.WorkingMemory {
	return m.working
}

func (m *mockMemoryStore) Mission() memory.MissionMemory {
	return m.mission
}

func (m *mockMemoryStore) LongTerm() memory.LongTermMemory {
	return m.longterm
}

// mockWorkingMemory is a simple in-memory implementation for testing.
type mockWorkingMemory struct {
	data map[string]any
}

func newMockWorkingMemory() *mockWorkingMemory {
	return &mockWorkingMemory{
		data: make(map[string]any),
	}
}

func (m *mockWorkingMemory) Get(key string) (any, bool) {
	val, ok := m.data[key]
	return val, ok
}

func (m *mockWorkingMemory) Set(key string, value any) error {
	m.data[key] = value
	return nil
}

func (m *mockWorkingMemory) Delete(key string) bool {
	_, exists := m.data[key]
	if exists {
		delete(m.data, key)
	}
	return exists
}

func (m *mockWorkingMemory) Clear() {
	m.data = make(map[string]any)
}

func (m *mockWorkingMemory) List() []string {
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys
}

func (m *mockWorkingMemory) TokenCount() int {
	return len(m.data)
}

func (m *mockWorkingMemory) MaxTokens() int {
	return 100000
}

// mockMissionMemory is a simple implementation for testing.
type mockMissionMemory struct {
	data      map[string]*memory.MemoryItem
	missionID types.ID
}

func newMockMissionMemory(missionID types.ID) *mockMissionMemory {
	return &mockMissionMemory{
		data:      make(map[string]*memory.MemoryItem),
		missionID: missionID,
	}
}

func (m *mockMissionMemory) Store(ctx context.Context, key string, value any, metadata map[string]any) error {
	m.data[key] = &memory.MemoryItem{
		Key:       key,
		Value:     value,
		Metadata:  metadata,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return nil
}

func (m *mockMissionMemory) Retrieve(ctx context.Context, key string) (*memory.MemoryItem, error) {
	item, ok := m.data[key]
	if !ok {
		return nil, types.NewError("NOT_FOUND", "key not found")
	}
	return item, nil
}

func (m *mockMissionMemory) Delete(ctx context.Context, key string) error {
	_, ok := m.data[key]
	if !ok {
		return types.NewError("NOT_FOUND", "key not found")
	}
	delete(m.data, key)
	return nil
}

func (m *mockMissionMemory) Search(ctx context.Context, query string, limit int) ([]memory.MemoryResult, error) {
	// Simple mock search - return all items
	results := make([]memory.MemoryResult, 0)
	for _, item := range m.data {
		results = append(results, memory.MemoryResult{
			Item:  *item,
			Score: 1.0,
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (m *mockMissionMemory) History(ctx context.Context, limit int) ([]memory.MemoryItem, error) {
	items := make([]memory.MemoryItem, 0)
	for _, item := range m.data {
		items = append(items, *item)
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func (m *mockMissionMemory) Keys(ctx context.Context) ([]string, error) {
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *mockMissionMemory) MissionID() types.ID {
	return m.missionID
}

// mockLongTermMemory is a simple implementation for testing.
type mockLongTermMemory struct {
	data map[string]*memory.MemoryItem
}

func newMockLongTermMemory() *mockLongTermMemory {
	return &mockLongTermMemory{
		data: make(map[string]*memory.MemoryItem),
	}
}

func (m *mockLongTermMemory) Store(ctx context.Context, id string, content string, metadata map[string]any) error {
	m.data[id] = &memory.MemoryItem{
		Key:       id,
		Value:     content,
		Metadata:  metadata,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return nil
}

func (m *mockLongTermMemory) Search(ctx context.Context, query string, topK int, filters map[string]any) ([]memory.MemoryResult, error) {
	results := make([]memory.MemoryResult, 0)
	for _, item := range m.data {
		results = append(results, memory.MemoryResult{
			Item:  *item,
			Score: 0.95,
		})
		if len(results) >= topK {
			break
		}
	}
	return results, nil
}

func (m *mockLongTermMemory) SimilarFindings(ctx context.Context, content string, topK int) ([]memory.MemoryResult, error) {
	return m.Search(ctx, content, topK, map[string]any{"type": "finding"})
}

func (m *mockLongTermMemory) SimilarPatterns(ctx context.Context, pattern string, topK int) ([]memory.MemoryResult, error) {
	return m.Search(ctx, pattern, topK, map[string]any{"type": "pattern"})
}

func (m *mockLongTermMemory) Delete(ctx context.Context, id string) error {
	_, ok := m.data[id]
	if !ok {
		return types.NewError("NOT_FOUND", "id not found")
	}
	delete(m.data, id)
	return nil
}

func (m *mockLongTermMemory) Health(ctx context.Context) types.HealthStatus {
	return types.Healthy("mock longterm memory is healthy")
}

func TestVerboseMemoryWrapper_Working_Get(t *testing.T) {
	tests := []struct {
		name          string
		level         VerboseLevel
		setupData     map[string]any
		getKey        string
		expectFound   bool
		expectEvent   bool
		expectedValue any
	}{
		{
			name:          "get existing key at VeryVerbose level",
			level:         LevelVeryVerbose,
			setupData:     map[string]any{"test-key": "test-value"},
			getKey:        "test-key",
			expectFound:   true,
			expectEvent:   true,
			expectedValue: "test-value",
		},
		{
			name:        "get non-existent key at VeryVerbose level",
			level:       LevelVeryVerbose,
			setupData:   map[string]any{},
			getKey:      "missing-key",
			expectFound: false,
			expectEvent: true,
		},
		{
			name:          "get at Verbose level - no event",
			level:         LevelVerbose,
			setupData:     map[string]any{"test-key": "test-value"},
			getKey:        "test-key",
			expectFound:   true,
			expectEvent:   false,
			expectedValue: "test-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			bus := NewDefaultVerboseEventBus()
			defer bus.Close()

			working := newMockWorkingMemory()
			for k, v := range tt.setupData {
				working.Set(k, v)
			}

			store := &mockMemoryStore{
				working:  working,
				mission:  newMockMissionMemory(types.NewID()),
				longterm: newMockLongTermMemory(),
			}

			missionID := types.NewID()
			agentName := "test-agent"
			wrapper := NewVerboseMemoryWrapper(store, bus, tt.level, missionID, agentName)

			// Subscribe to events
			ctx := context.Background()
			eventChan, cleanup := bus.Subscribe(ctx)
			defer cleanup()

			// Execute
			value, found := wrapper.Working().Get(tt.getKey)

			// Verify result
			assert.Equal(t, tt.expectFound, found)
			if tt.expectFound {
				assert.Equal(t, tt.expectedValue, value)
			}

			// Check for event
			if tt.expectEvent {
				select {
				case event := <-eventChan:
					assert.Equal(t, EventMemoryGet, event.Type)
					assert.Equal(t, tt.level, event.Level)
					assert.Equal(t, missionID, event.MissionID)
					assert.Equal(t, agentName, event.AgentName)

					payload, ok := event.Payload.(MemoryGetData)
					require.True(t, ok)
					assert.Equal(t, "working", payload.Tier)
					assert.Equal(t, tt.getKey, payload.Key)
					assert.Equal(t, tt.expectFound, payload.Found)
				case <-time.After(100 * time.Millisecond):
					t.Fatal("expected event not received")
				}
			} else {
				select {
				case <-eventChan:
					t.Fatal("unexpected event received")
				case <-time.After(100 * time.Millisecond):
					// No event expected, test passes
				}
			}
		})
	}
}

func TestVerboseMemoryWrapper_Working_Set(t *testing.T) {
	tests := []struct {
		name        string
		level       VerboseLevel
		key         string
		value       any
		expectEvent bool
		expectSize  int
	}{
		{
			name:        "set string value at VeryVerbose level",
			level:       LevelVeryVerbose,
			key:         "test-key",
			value:       "test-value",
			expectEvent: true,
			expectSize:  12, // JSON: "test-value" = 12 bytes
		},
		{
			name:        "set struct value at Debug level",
			level:       LevelDebug,
			key:         "struct-key",
			value:       map[string]any{"field": "value"},
			expectEvent: true,
			expectSize:  17, // JSON: {"field":"value"} = 17 bytes
		},
		{
			name:        "set at Verbose level - no event",
			level:       LevelVerbose,
			key:         "test-key",
			value:       "test-value",
			expectEvent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			bus := NewDefaultVerboseEventBus()
			defer bus.Close()

			store := &mockMemoryStore{
				working:  newMockWorkingMemory(),
				mission:  newMockMissionMemory(types.NewID()),
				longterm: newMockLongTermMemory(),
			}

			missionID := types.NewID()
			agentName := "test-agent"
			wrapper := NewVerboseMemoryWrapper(store, bus, tt.level, missionID, agentName)

			// Subscribe to events
			ctx := context.Background()
			eventChan, cleanup := bus.Subscribe(ctx)
			defer cleanup()

			// Execute
			err := wrapper.Working().Set(tt.key, tt.value)
			require.NoError(t, err)

			// Verify data was stored
			stored, found := wrapper.Working().Get(tt.key)
			assert.True(t, found)
			assert.Equal(t, tt.value, stored)

			// Check for event
			if tt.expectEvent {
				select {
				case event := <-eventChan:
					// Skip the Get event from verification above
					if event.Type == EventMemoryGet {
						event = <-eventChan
					}

					assert.Equal(t, EventMemorySet, event.Type)
					assert.Equal(t, tt.level, event.Level)
					assert.Equal(t, missionID, event.MissionID)
					assert.Equal(t, agentName, event.AgentName)

					payload, ok := event.Payload.(MemorySetData)
					require.True(t, ok)
					assert.Equal(t, "working", payload.Tier)
					assert.Equal(t, tt.key, payload.Key)
					assert.Equal(t, tt.expectSize, payload.ValueSize)
				case <-time.After(100 * time.Millisecond):
					t.Fatal("expected event not received")
				}
			}
		})
	}
}

func TestVerboseMemoryWrapper_Mission_Search(t *testing.T) {
	tests := []struct {
		name        string
		level       VerboseLevel
		setupData   map[string]any
		query       string
		limit       int
		expectEvent bool
		expectCount int
	}{
		{
			name:  "search with results at VeryVerbose level",
			level: LevelVeryVerbose,
			setupData: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
			query:       "test",
			limit:       10,
			expectEvent: true,
			expectCount: 2,
		},
		{
			name:        "search with no results at Debug level",
			level:       LevelDebug,
			setupData:   map[string]any{},
			query:       "test",
			limit:       10,
			expectEvent: true,
			expectCount: 0,
		},
		{
			name:  "search at Verbose level - no event",
			level: LevelVerbose,
			setupData: map[string]any{
				"key1": "value1",
			},
			query:       "test",
			limit:       10,
			expectEvent: false,
			expectCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			bus := NewDefaultVerboseEventBus()
			defer bus.Close()

			missionID := types.NewID()
			mission := newMockMissionMemory(missionID)

			// Setup data
			ctx := context.Background()
			for k, v := range tt.setupData {
				mission.Store(ctx, k, v, nil)
			}

			store := &mockMemoryStore{
				working:  newMockWorkingMemory(),
				mission:  mission,
				longterm: newMockLongTermMemory(),
			}

			agentName := "test-agent"
			wrapper := NewVerboseMemoryWrapper(store, bus, tt.level, missionID, agentName)

			// Subscribe to events
			eventChan, cleanup := bus.Subscribe(ctx)
			defer cleanup()

			// Execute
			results, err := wrapper.Mission().Search(ctx, tt.query, tt.limit)
			require.NoError(t, err)
			assert.Len(t, results, tt.expectCount)

			// Check for event
			if tt.expectEvent {
				select {
				case event := <-eventChan:
					assert.Equal(t, EventMemorySearch, event.Type)
					assert.Equal(t, tt.level, event.Level)
					assert.Equal(t, missionID, event.MissionID)
					assert.Equal(t, agentName, event.AgentName)

					payload, ok := event.Payload.(MemorySearchData)
					require.True(t, ok)
					assert.Equal(t, "mission", payload.Tier)
					assert.Equal(t, tt.query, payload.Query)
					assert.Equal(t, tt.expectCount, payload.ResultCount)
					assert.Greater(t, payload.Duration, time.Duration(0))
				case <-time.After(100 * time.Millisecond):
					t.Fatal("expected event not received")
				}
			} else {
				select {
				case <-eventChan:
					t.Fatal("unexpected event received")
				case <-time.After(100 * time.Millisecond):
					// No event expected, test passes
				}
			}
		})
	}
}

func TestVerboseMemoryWrapper_LongTerm_Store(t *testing.T) {
	// Setup
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	store := &mockMemoryStore{
		working:  newMockWorkingMemory(),
		mission:  newMockMissionMemory(types.NewID()),
		longterm: newMockLongTermMemory(),
	}

	missionID := types.NewID()
	agentName := "test-agent"
	wrapper := NewVerboseMemoryWrapper(store, bus, LevelVeryVerbose, missionID, agentName)

	// Subscribe to events
	ctx := context.Background()
	eventChan, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Execute
	testID := "test-id"
	testContent := "This is test content for long-term memory"
	metadata := map[string]any{"type": "finding"}

	err := wrapper.LongTerm().Store(ctx, testID, testContent, metadata)
	require.NoError(t, err)

	// Verify event
	select {
	case event := <-eventChan:
		assert.Equal(t, EventMemorySet, event.Type)
		assert.Equal(t, LevelVeryVerbose, event.Level)
		assert.Equal(t, missionID, event.MissionID)
		assert.Equal(t, agentName, event.AgentName)

		payload, ok := event.Payload.(MemorySetData)
		require.True(t, ok)
		assert.Equal(t, "longterm", payload.Tier)
		assert.Equal(t, testID, payload.Key)
		assert.Equal(t, len(testContent), payload.ValueSize)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected event not received")
	}
}

func TestVerboseMemoryWrapper_LongTerm_Search(t *testing.T) {
	// Setup
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	longterm := newMockLongTermMemory()
	ctx := context.Background()

	// Setup test data
	longterm.Store(ctx, "id1", "finding content 1", map[string]any{"type": "finding"})
	longterm.Store(ctx, "id2", "finding content 2", map[string]any{"type": "finding"})

	store := &mockMemoryStore{
		working:  newMockWorkingMemory(),
		mission:  newMockMissionMemory(types.NewID()),
		longterm: longterm,
	}

	missionID := types.NewID()
	agentName := "test-agent"
	wrapper := NewVerboseMemoryWrapper(store, bus, LevelDebug, missionID, agentName)

	// Subscribe to events
	eventChan, cleanup := bus.Subscribe(ctx)
	defer cleanup()

	// Execute
	results, err := wrapper.LongTerm().Search(ctx, "finding", 10, map[string]any{"type": "finding"})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify event
	select {
	case event := <-eventChan:
		assert.Equal(t, EventMemorySearch, event.Type)
		assert.Equal(t, LevelDebug, event.Level)
		assert.Equal(t, missionID, event.MissionID)
		assert.Equal(t, agentName, event.AgentName)

		payload, ok := event.Payload.(MemorySearchData)
		require.True(t, ok)
		assert.Equal(t, "longterm", payload.Tier)
		assert.Equal(t, "finding", payload.Query)
		assert.Equal(t, 2, payload.ResultCount)
		assert.Greater(t, payload.Duration, time.Duration(0))
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected event not received")
	}
}

func TestEstimateValueSize(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected int
	}{
		{
			name:     "nil value",
			value:    nil,
			expected: 0,
		},
		{
			name:     "string value",
			value:    "test",
			expected: 6, // JSON: "test" = 6 bytes
		},
		{
			name:     "integer value",
			value:    42,
			expected: 2, // JSON: 42 = 2 bytes
		},
		{
			name:     "map value",
			value:    map[string]any{"key": "value"},
			expected: 15, // JSON: {"key":"value"} = 15 bytes
		},
		{
			name:     "byte slice",
			value:    []byte("hello"),
			expected: 7, // JSON: "aGVsbG8=" (base64) = 7 bytes (approximately)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := estimateValueSize(tt.value)
			// Allow some variance for JSON encoding differences
			assert.InDelta(t, tt.expected, size, 5)
		})
	}
}
