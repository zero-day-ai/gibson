package memory

import (
	"container/list"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// Memory continuity errors
var (
	// ErrNoPreviousRun is returned when attempting to access prior run data but no prior run exists
	ErrNoPreviousRun = errors.New("no previous run exists")

	// ErrContinuityNotSupported is returned when attempting continuity operations in isolated mode
	ErrContinuityNotSupported = errors.New("memory continuity not supported in isolated mode")
)

// MemoryContinuityMode defines how memory state is handled across multiple runs
type MemoryContinuityMode string

const (
	// MemoryIsolated indicates that each run has completely isolated memory (default)
	MemoryIsolated MemoryContinuityMode = "isolated"

	// MemoryInherit indicates that new runs can read memory from prior runs
	// but cannot modify the shared state (copy-on-write semantics)
	MemoryInherit MemoryContinuityMode = "inherit"

	// MemoryShared indicates that all runs share the same memory namespace
	// with full read and write access
	MemoryShared MemoryContinuityMode = "shared"
)

// HistoricalValue represents a value retrieved from a previous run's memory
type HistoricalValue struct {
	// Value is the actual data stored in memory
	Value any `json:"value"`

	// RunNumber is the sequential run number within the mission (1-based)
	RunNumber int `json:"run_number"`

	// MissionID is the unique identifier of the mission this value belongs to
	MissionID string `json:"mission_id"`

	// StoredAt is the timestamp when this value was stored
	StoredAt time.Time `json:"stored_at"`
}

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

	// Memory Continuity Methods

	// ContinuityMode returns the current memory continuity mode
	// Returns MemoryIsolated if not explicitly configured
	ContinuityMode() MemoryContinuityMode

	// GetPreviousRunValue retrieves a value from the prior run's memory
	// Only works if continuity mode is 'inherit' or 'shared'
	// Returns ErrNoPreviousRun if no prior run exists
	// Returns ErrContinuityNotSupported if mode is 'isolated'
	GetPreviousRunValue(ctx context.Context, key string) (any, error)

	// GetValueHistory returns values for a key across all runs
	// Returns in chronological order with run metadata
	// Returns empty slice if key was never stored
	GetValueHistory(ctx context.Context, key string) ([]HistoricalValue, error)
}

// DefaultMissionMemory implements MissionMemory using SQLite with FTS5 and LRU cache
type DefaultMissionMemory struct {
	db        *database.DB
	missionID types.ID
	cache     *missionMemoryCache

	// Memory continuity fields
	continuityMode    MemoryContinuityMode
	previousMissionID *types.ID // ID of prior run for inherit mode
	missionName       string    // Mission name for shared mode
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

// Memory Continuity Methods

// ContinuityMode returns the current memory continuity mode
// Returns MemoryIsolated if not explicitly configured
func (m *DefaultMissionMemory) ContinuityMode() MemoryContinuityMode {
	if m.continuityMode == "" {
		return MemoryIsolated
	}
	return m.continuityMode
}

// GetPreviousRunValue retrieves a value from the prior run's memory
// Only works if continuity mode is 'inherit' or 'shared'
// Returns ErrNoPreviousRun if no prior run exists
// Returns ErrContinuityNotSupported if mode is 'isolated'
func (m *DefaultMissionMemory) GetPreviousRunValue(ctx context.Context, key string) (any, error) {
	// Check if continuity is supported
	if m.ContinuityMode() == MemoryIsolated {
		return nil, ErrContinuityNotSupported
	}

	// Check if previous run exists
	if m.previousMissionID == nil {
		return nil, ErrNoPreviousRun
	}

	// Query the database for key with previous mission ID
	query := `
		SELECT value
		FROM mission_memory
		WHERE mission_id = ? AND key = ?
	`

	var valueJSON sql.NullString
	err := m.db.Conn().QueryRowContext(ctx, query, *m.previousMissionID, key).Scan(&valueJSON)

	if err == sql.ErrNoRows {
		return nil, NewMissionMemoryNotFoundError(key)
	}
	if err != nil {
		return nil, NewMissionMemoryStoreError("failed to retrieve previous run value", err)
	}

	// Unmarshal value
	var value any
	if valueJSON.Valid {
		if err := json.Unmarshal([]byte(valueJSON.String), &value); err != nil {
			return nil, NewMissionMemoryStoreError("failed to unmarshal previous run value", err)
		}
	}

	return value, nil
}

// GetValueHistory returns values for a key across all runs
// Returns in chronological order with run metadata
// Returns empty slice if key was never stored
func (m *DefaultMissionMemory) GetValueHistory(ctx context.Context, key string) ([]HistoricalValue, error) {
	// For shared mode, we need to query all missions with the same name
	// For inherit mode, we need to follow the chain of previous runs
	// For isolated mode, there's no history (just current run)

	if m.ContinuityMode() == MemoryIsolated {
		// In isolated mode, only return current run's value if it exists
		item, err := m.Retrieve(ctx, key)
		if err != nil {
			// Key doesn't exist in current run, return empty history
			if NewMissionMemoryNotFoundError(key).Error() == err.Error() {
				return []HistoricalValue{}, nil
			}
			return nil, err
		}

		return []HistoricalValue{
			{
				Value:     item.Value,
				RunNumber: 1, // In isolated mode, always run 1
				MissionID: string(m.missionID),
				StoredAt:  item.CreatedAt,
			},
		}, nil
	}

	// For shared and inherit modes, we need to query across multiple mission IDs
	// First, get the list of mission IDs based on mode
	var missionIDs []types.ID
	var err error

	if m.ContinuityMode() == MemoryShared && m.missionName != "" {
		// Shared mode: get all missions with the same name
		missionIDs, err = m.getMissionIDsByName(ctx, m.missionName)
		if err != nil {
			return nil, NewMissionMemoryStoreError("failed to retrieve mission IDs by name", err)
		}
	} else if m.ContinuityMode() == MemoryInherit {
		// Inherit mode: follow the chain of previous runs
		missionIDs, err = m.getMissionChain(ctx)
		if err != nil {
			return nil, NewMissionMemoryStoreError("failed to retrieve mission chain", err)
		}
	}

	// Query all values for this key across all mission IDs
	if len(missionIDs) == 0 {
		return []HistoricalValue{}, nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(missionIDs))
	args := make([]interface{}, len(missionIDs)+1)
	args[0] = key
	for i, id := range missionIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}

	query := `
		SELECT value, mission_id, created_at
		FROM mission_memory
		WHERE key = ? AND mission_id IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY created_at ASC
	`

	rows, err := m.db.Conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, NewMissionMemoryStoreError("failed to query value history", err)
	}
	defer rows.Close()

	var history []HistoricalValue
	runNumber := 1
	for rows.Next() {
		var valueJSON sql.NullString
		var missionID string
		var createdAt time.Time

		if err := rows.Scan(&valueJSON, &missionID, &createdAt); err != nil {
			return nil, NewMissionMemoryStoreError("failed to scan history row", err)
		}

		// Unmarshal value
		var value any
		if valueJSON.Valid {
			if err := json.Unmarshal([]byte(valueJSON.String), &value); err != nil {
				return nil, NewMissionMemoryStoreError("failed to unmarshal historical value", err)
			}
		}

		history = append(history, HistoricalValue{
			Value:     value,
			RunNumber: runNumber,
			MissionID: missionID,
			StoredAt:  createdAt,
		})
		runNumber++
	}

	if err := rows.Err(); err != nil {
		return nil, NewMissionMemoryStoreError("error iterating history rows", err)
	}

	return history, nil
}

// getMissionIDsByName retrieves all mission IDs with the given name
func (m *DefaultMissionMemory) getMissionIDsByName(ctx context.Context, name string) ([]types.ID, error) {
	// Query the database directly for all missions with the given name
	query := `
		SELECT id
		FROM missions
		WHERE name = ?
		ORDER BY created_at ASC
	`

	rows, err := m.db.Conn().QueryContext(ctx, query, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []types.ID
	for rows.Next() {
		var id types.ID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

// getMissionChain follows the chain of previous runs to build the history
func (m *DefaultMissionMemory) getMissionChain(ctx context.Context) ([]types.ID, error) {
	var chain []types.ID

	// Start with the current mission and walk backwards through previous runs
	currentID := m.missionID

	// Query to get previous run ID
	query := `
		SELECT previous_run_id
		FROM missions
		WHERE id = ?
	`

	// Build the chain by following previous_run_id links
	seen := make(map[types.ID]bool)
	maxIterations := 1000 // Prevent infinite loops

	for i := 0; i < maxIterations; i++ {
		// Prevent cycles
		if seen[currentID] {
			break
		}
		seen[currentID] = true

		// Add current ID to the beginning of the chain (we're walking backwards)
		chain = append([]types.ID{currentID}, chain...)

		// Get previous run ID
		var previousRunID sql.NullString
		err := m.db.Conn().QueryRowContext(ctx, query, currentID).Scan(&previousRunID)
		if err == sql.ErrNoRows || !previousRunID.Valid {
			// No more previous runs
			break
		}
		if err != nil {
			return nil, err
		}

		currentID = types.ID(previousRunID.String)
	}

	return chain, nil
}
