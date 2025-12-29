package memory

import (
	"container/list"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MissionMemory provides persistent per-mission storage with FTS
type MissionMemory interface {
	// Store persists a key-value pair with optional metadata
	Store(ctx context.Context, key string, value any, metadata map[string]any) error

	// Retrieve gets an item by key
	Retrieve(ctx context.Context, key string) (*MemoryItem, error)

	// Delete removes an entry
	Delete(ctx context.Context, key string) error

	// Search performs full-text search across stored content
	Search(ctx context.Context, query string, limit int) ([]MemoryResult, error)

	// History returns recent entries ordered by time
	History(ctx context.Context, limit int) ([]MemoryItem, error)

	// Keys returns all keys for this mission
	Keys(ctx context.Context) ([]string, error)

	// MissionID returns the mission this memory is scoped to
	MissionID() types.ID
}

// DefaultMissionMemory implements MissionMemory using SQLite with FTS5 and LRU cache
type DefaultMissionMemory struct {
	db        *database.DB
	missionID types.ID
	cache     *missionMemoryCache
}

// missionMemoryCache is an LRU cache for mission memory items
type missionMemoryCache struct {
	mu      sync.RWMutex
	items   map[string]*list.Element
	order   *list.List
	maxSize int
}

// cacheEntry holds a cached memory item
type cacheEntry struct {
	key  string
	item *MemoryItem
}

// newMissionMemoryCache creates a new LRU cache
func newMissionMemoryCache(maxSize int) *missionMemoryCache {
	return &missionMemoryCache{
		items:   make(map[string]*list.Element),
		order:   list.New(),
		maxSize: maxSize,
	}
}

// Get retrieves an item from the cache
func (c *missionMemoryCache) Get(key string) (*MemoryItem, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}

	// Move to front (most recently used)
	c.order.MoveToFront(elem)

	entry := elem.Value.(*cacheEntry)
	return entry.item, true
}

// Put adds or updates an item in the cache
func (c *missionMemoryCache) Put(key string, item *MemoryItem) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If item exists, update it and move to front
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		entry.item = item
		return
	}

	// Add new item
	entry := &cacheEntry{key: key, item: item}
	elem := c.order.PushFront(entry)
	c.items[key] = elem

	// Evict oldest if over capacity
	if c.order.Len() > c.maxSize {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			oldEntry := oldest.Value.(*cacheEntry)
			delete(c.items, oldEntry.key)
		}
	}
}

// Remove removes an item from the cache
func (c *missionMemoryCache) Remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.Remove(elem)
		delete(c.items, key)
	}
}

// Clear removes all items from the cache
func (c *missionMemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.order = list.New()
}

// NewMissionMemory creates a new DefaultMissionMemory with LRU cache
func NewMissionMemory(db *database.DB, missionID types.ID, cacheSize int) MissionMemory {
	if cacheSize <= 0 {
		cacheSize = 1000 // Default cache size
	}

	return &DefaultMissionMemory{
		db:        db,
		missionID: missionID,
		cache:     newMissionMemoryCache(cacheSize),
	}
}

// Store persists a key-value pair with optional metadata
func (m *DefaultMissionMemory) Store(ctx context.Context, key string, value any, metadata map[string]any) error {
	if key == "" {
		return NewMissionMemoryStoreError("key cannot be empty", nil)
	}

	// JSON encode the value
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return NewMissionMemoryStoreError("failed to marshal value", err)
	}

	// JSON encode metadata
	var metadataJSON []byte
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return NewMissionMemoryStoreError("failed to marshal metadata", err)
		}
	}

	// Generate ID for the record
	id := types.NewID()

	// Insert or replace into database
	query := `
		INSERT INTO mission_memory (id, mission_id, key, value, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(mission_id, key) DO UPDATE SET
			value = excluded.value,
			metadata = excluded.metadata,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err = m.db.Conn().ExecContext(ctx, query, id, m.missionID, key, string(valueJSON), string(metadataJSON))
	if err != nil {
		return NewMissionMemoryStoreError("failed to store item", err)
	}

	// Update cache
	item := &MemoryItem{
		Key:       key,
		Value:     value,
		Metadata:  metadata,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.cache.Put(key, item)

	return nil
}

// Retrieve gets an item by key
func (m *DefaultMissionMemory) Retrieve(ctx context.Context, key string) (*MemoryItem, error) {
	// Check cache first
	if item, ok := m.cache.Get(key); ok {
		return item, nil
	}

	// Query database
	query := `
		SELECT key, value, metadata, created_at, updated_at
		FROM mission_memory
		WHERE mission_id = ? AND key = ?
	`

	var valueJSON, metadataJSON sql.NullString
	var createdAt, updatedAt time.Time

	err := m.db.Conn().QueryRowContext(ctx, query, m.missionID, key).Scan(
		&key, &valueJSON, &metadataJSON, &createdAt, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, NewMissionMemoryNotFoundError(key)
	}
	if err != nil {
		return nil, NewMissionMemoryStoreError("failed to retrieve item", err)
	}

	// Unmarshal value
	var value any
	if valueJSON.Valid {
		if err := json.Unmarshal([]byte(valueJSON.String), &value); err != nil {
			return nil, NewMissionMemoryStoreError("failed to unmarshal value", err)
		}
	}

	// Unmarshal metadata
	var metadata map[string]any
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err != nil {
			return nil, NewMissionMemoryStoreError("failed to unmarshal metadata", err)
		}
	}

	item := &MemoryItem{
		Key:       key,
		Value:     value,
		Metadata:  metadata,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	// Update cache
	m.cache.Put(key, item)

	return item, nil
}

// Delete removes an entry
func (m *DefaultMissionMemory) Delete(ctx context.Context, key string) error {
	query := `DELETE FROM mission_memory WHERE mission_id = ? AND key = ?`

	result, err := m.db.Conn().ExecContext(ctx, query, m.missionID, key)
	if err != nil {
		return NewMissionMemoryStoreError("failed to delete item", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return NewMissionMemoryStoreError("failed to get rows affected", err)
	}

	if rowsAffected == 0 {
		return NewMissionMemoryNotFoundError(key)
	}

	// Remove from cache
	m.cache.Remove(key)

	return nil
}

// Search performs full-text search across stored content
func (m *DefaultMissionMemory) Search(ctx context.Context, query string, limit int) ([]MemoryResult, error) {
	if query == "" {
		return nil, NewFTSQueryError("query cannot be empty", nil)
	}

	if limit <= 0 {
		limit = 10
	}

	// Escape FTS5 special characters
	escapedQuery := escapeFTS5Query(query)

	// FTS5 search query with BM25 ranking
	searchQuery := `
		SELECT m.key, m.value, m.metadata, m.created_at, m.updated_at, bm25(f.mission_memory_fts) as score
		FROM mission_memory m
		JOIN mission_memory_fts f ON m.rowid = f.rowid
		WHERE f.mission_memory_fts MATCH ? AND m.mission_id = ?
		ORDER BY score
		LIMIT ?
	`

	rows, err := m.db.Conn().QueryContext(ctx, searchQuery, escapedQuery, m.missionID, limit)
	if err != nil {
		return nil, NewFTSQueryError("failed to execute search query", err)
	}
	defer rows.Close()

	var results []MemoryResult
	for rows.Next() {
		var key string
		var valueJSON, metadataJSON sql.NullString
		var createdAt, updatedAt time.Time
		var score float64

		if err := rows.Scan(&key, &valueJSON, &metadataJSON, &createdAt, &updatedAt, &score); err != nil {
			return nil, NewFTSQueryError("failed to scan result", err)
		}

		// Unmarshal value
		var value any
		if valueJSON.Valid {
			if err := json.Unmarshal([]byte(valueJSON.String), &value); err != nil {
				return nil, NewFTSQueryError("failed to unmarshal value", err)
			}
		}

		// Unmarshal metadata
		var metadata map[string]any
		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err != nil {
				return nil, NewFTSQueryError("failed to unmarshal metadata", err)
			}
		}

		item := MemoryItem{
			Key:       key,
			Value:     value,
			Metadata:  metadata,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}

		// BM25 scores are negative, convert to positive for intuitive scoring
		// Normalize to 0-1 range (this is approximate)
		normalizedScore := 1.0 / (1.0 + (-score))

		results = append(results, MemoryResult{
			Item:  item,
			Score: normalizedScore,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, NewFTSQueryError("error iterating results", err)
	}

	return results, nil
}

// History returns recent entries ordered by time
func (m *DefaultMissionMemory) History(ctx context.Context, limit int) ([]MemoryItem, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT key, value, metadata, created_at, updated_at
		FROM mission_memory
		WHERE mission_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := m.db.Conn().QueryContext(ctx, query, m.missionID, limit)
	if err != nil {
		return nil, NewMissionMemoryStoreError("failed to query history", err)
	}
	defer rows.Close()

	var items []MemoryItem
	for rows.Next() {
		var key string
		var valueJSON, metadataJSON sql.NullString
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&key, &valueJSON, &metadataJSON, &createdAt, &updatedAt); err != nil {
			return nil, NewMissionMemoryStoreError("failed to scan history item", err)
		}

		// Unmarshal value
		var value any
		if valueJSON.Valid {
			if err := json.Unmarshal([]byte(valueJSON.String), &value); err != nil {
				return nil, NewMissionMemoryStoreError("failed to unmarshal value", err)
			}
		}

		// Unmarshal metadata
		var metadata map[string]any
		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err != nil {
				return nil, NewMissionMemoryStoreError("failed to unmarshal metadata", err)
			}
		}

		items = append(items, MemoryItem{
			Key:       key,
			Value:     value,
			Metadata:  metadata,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, NewMissionMemoryStoreError("error iterating history", err)
	}

	return items, nil
}

// Keys returns all keys for this mission
func (m *DefaultMissionMemory) Keys(ctx context.Context) ([]string, error) {
	query := `SELECT key FROM mission_memory WHERE mission_id = ? ORDER BY key`

	rows, err := m.db.Conn().QueryContext(ctx, query, m.missionID)
	if err != nil {
		return nil, NewMissionMemoryStoreError("failed to query keys", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, NewMissionMemoryStoreError("failed to scan key", err)
		}
		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, NewMissionMemoryStoreError("error iterating keys", err)
	}

	return keys, nil
}

// MissionID returns the mission this memory is scoped to
func (m *DefaultMissionMemory) MissionID() types.ID {
	return m.missionID
}

// escapeFTS5Query escapes special FTS5 characters in the query
func escapeFTS5Query(query string) string {
	// FTS5 special characters: " AND OR NOT ( ) * :
	// For simple queries, we'll wrap in quotes to treat as phrase
	// and escape internal quotes
	query = strings.ReplaceAll(query, `"`, `""`)
	return `"` + query + `"`
}
