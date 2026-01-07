package verbose

import (
	"context"
	"encoding/json"
	"time"

	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
)

// VerboseMemoryWrapper wraps a MemoryStore and emits VerboseEvents for memory operations.
// It implements the memory.MemoryStore interface and transparently emits events to a
// VerboseEventBus while delegating all actual work to the inner memory store.
//
// The wrapper emits events at VerboseLevel >= 2 (VeryVerbose) for:
//   - Get operations (with key name and whether value was found)
//   - Set operations (with key name and value SIZE, not actual value)
//   - Delete operations (with key name)
//   - Search operations (with query and result count)
//
// SECURITY: Never emits actual values, only key names and value sizes, to prevent
// exposing sensitive data like credentials in verbose logs.
type VerboseMemoryWrapper struct {
	inner memory.MemoryStore
	bus   VerboseEventBus
	level VerboseLevel

	// Wrapped tier instances
	workingWrapper  *VerboseWorkingMemoryWrapper
	missionWrapper  *VerboseMissionMemoryWrapper
	longtermWrapper *VerboseLongTermMemoryWrapper
}

// NewVerboseMemoryWrapper creates a new VerboseMemoryWrapper that wraps the provided
// inner memory store with event emission.
//
// Parameters:
//   - inner: The underlying MemoryStore to wrap
//   - bus: The event bus to emit events to
//   - level: The verbosity level for emitted events
//   - missionID: The mission ID for event context
//   - agentName: The agent name for event context
//
// Returns:
//   - *VerboseMemoryWrapper: A verbose memory store ready for use
//
// Example:
//
//	bus := NewDefaultVerboseEventBus()
//	wrapped := NewVerboseMemoryWrapper(innerMemory, bus, LevelVeryVerbose, missionID, agentName)
//	value, ok := wrapped.Working().Get("key")
func NewVerboseMemoryWrapper(inner memory.MemoryStore, bus VerboseEventBus, level VerboseLevel, missionID types.ID, agentName string) *VerboseMemoryWrapper {
	wrapper := &VerboseMemoryWrapper{
		inner: inner,
		bus:   bus,
		level: level,
	}

	// Wrap each tier
	wrapper.workingWrapper = NewVerboseWorkingMemoryWrapper(inner.Working(), bus, level, "working", missionID, agentName)
	wrapper.missionWrapper = NewVerboseMissionMemoryWrapper(inner.Mission(), bus, level, "mission", missionID, agentName)
	wrapper.longtermWrapper = NewVerboseLongTermMemoryWrapper(inner.LongTerm(), bus, level, "longterm", missionID, agentName)

	return wrapper
}

// Working returns the working memory instance wrapped with verbose events.
func (w *VerboseMemoryWrapper) Working() memory.WorkingMemory {
	return w.workingWrapper
}

// Mission returns the mission memory instance wrapped with verbose events.
func (w *VerboseMemoryWrapper) Mission() memory.MissionMemory {
	return w.missionWrapper
}

// LongTerm returns the long-term memory instance wrapped with verbose events.
func (w *VerboseMemoryWrapper) LongTerm() memory.LongTermMemory {
	return w.longtermWrapper
}

// Ensure VerboseMemoryWrapper implements MemoryStore at compile time
var _ memory.MemoryStore = (*VerboseMemoryWrapper)(nil)

// VerboseWorkingMemoryWrapper wraps a WorkingMemory instance and emits events.
type VerboseWorkingMemoryWrapper struct {
	inner     memory.WorkingMemory
	bus       VerboseEventBus
	level     VerboseLevel
	tier      string
	missionID types.ID
	agentName string
}

// NewVerboseWorkingMemoryWrapper creates a new VerboseWorkingMemoryWrapper.
func NewVerboseWorkingMemoryWrapper(inner memory.WorkingMemory, bus VerboseEventBus, level VerboseLevel, tier string, missionID types.ID, agentName string) *VerboseWorkingMemoryWrapper {
	return &VerboseWorkingMemoryWrapper{
		inner:     inner,
		bus:       bus,
		level:     level,
		tier:      tier,
		missionID: missionID,
		agentName: agentName,
	}
}

// Get retrieves a value by key with event emission.
func (w *VerboseWorkingMemoryWrapper) Get(key string) (any, bool) {
	// Only emit at VeryVerbose or higher
	if w.level < LevelVeryVerbose {
		return w.inner.Get(key)
	}

	value, found := w.inner.Get(key)

	// Emit memory get event
	event := NewVerboseEvent(EventMemoryGet, w.level, MemoryGetData{
		Tier:  w.tier,
		Key:   key,
		Found: found,
	}).WithMissionID(w.missionID).WithAgentName(w.agentName)

	_ = w.bus.Emit(context.Background(), event)

	return value, found
}

// Set stores a value with event emission.
func (w *VerboseWorkingMemoryWrapper) Set(key string, value any) error {
	// Call inner implementation first
	err := w.inner.Set(key, value)

	// Only emit at VeryVerbose or higher
	if w.level >= LevelVeryVerbose && err == nil {
		// Calculate value size (estimate)
		valueSize := estimateValueSize(value)

		// Emit memory set event
		event := NewVerboseEvent(EventMemorySet, w.level, MemorySetData{
			Tier:      w.tier,
			Key:       key,
			ValueSize: valueSize,
		}).WithMissionID(w.missionID).WithAgentName(w.agentName)

		_ = w.bus.Emit(context.Background(), event)
	}

	return err
}

// Delete removes an entry with event emission.
func (w *VerboseWorkingMemoryWrapper) Delete(key string) bool {
	deleted := w.inner.Delete(key)

	// Only emit at VeryVerbose or higher
	if w.level >= LevelVeryVerbose && deleted {
		// Emit memory delete event (using MemoryGetData with Found=false to indicate deletion)
		event := NewVerboseEvent(EventMemoryGet, w.level, MemoryGetData{
			Tier:  w.tier,
			Key:   key,
			Found: false,
		}).WithMissionID(w.missionID).WithAgentName(w.agentName)

		_ = w.bus.Emit(context.Background(), event)
	}

	return deleted
}

// Clear removes all entries (pass-through without event).
func (w *VerboseWorkingMemoryWrapper) Clear() {
	w.inner.Clear()
}

// List returns all stored keys (pass-through without event).
func (w *VerboseWorkingMemoryWrapper) List() []string {
	return w.inner.List()
}

// TokenCount returns the current token usage (pass-through without event).
func (w *VerboseWorkingMemoryWrapper) TokenCount() int {
	return w.inner.TokenCount()
}

// MaxTokens returns the configured token limit (pass-through without event).
func (w *VerboseWorkingMemoryWrapper) MaxTokens() int {
	return w.inner.MaxTokens()
}

// Ensure VerboseWorkingMemoryWrapper implements WorkingMemory at compile time
var _ memory.WorkingMemory = (*VerboseWorkingMemoryWrapper)(nil)

// VerboseMissionMemoryWrapper wraps a MissionMemory instance and emits events.
type VerboseMissionMemoryWrapper struct {
	inner     memory.MissionMemory
	bus       VerboseEventBus
	level     VerboseLevel
	tier      string
	missionID types.ID
	agentName string
}

// NewVerboseMissionMemoryWrapper creates a new VerboseMissionMemoryWrapper.
func NewVerboseMissionMemoryWrapper(inner memory.MissionMemory, bus VerboseEventBus, level VerboseLevel, tier string, missionID types.ID, agentName string) *VerboseMissionMemoryWrapper {
	return &VerboseMissionMemoryWrapper{
		inner:     inner,
		bus:       bus,
		level:     level,
		tier:      tier,
		missionID: missionID,
		agentName: agentName,
	}
}

// Store persists a key-value pair with event emission.
func (m *VerboseMissionMemoryWrapper) Store(ctx context.Context, key string, value any, metadata map[string]any) error {
	err := m.inner.Store(ctx, key, value, metadata)

	// Only emit at VeryVerbose or higher
	if m.level >= LevelVeryVerbose && err == nil {
		valueSize := estimateValueSize(value)

		event := NewVerboseEvent(EventMemorySet, m.level, MemorySetData{
			Tier:      m.tier,
			Key:       key,
			ValueSize: valueSize,
		}).WithMissionID(m.missionID).WithAgentName(m.agentName)

		_ = m.bus.Emit(ctx, event)
	}

	return err
}

// Retrieve gets an item by key with event emission.
func (m *VerboseMissionMemoryWrapper) Retrieve(ctx context.Context, key string) (*memory.MemoryItem, error) {
	item, err := m.inner.Retrieve(ctx, key)

	// Only emit at VeryVerbose or higher
	if m.level >= LevelVeryVerbose {
		event := NewVerboseEvent(EventMemoryGet, m.level, MemoryGetData{
			Tier:  m.tier,
			Key:   key,
			Found: err == nil,
		}).WithMissionID(m.missionID).WithAgentName(m.agentName)

		_ = m.bus.Emit(ctx, event)
	}

	return item, err
}

// Delete removes an entry with event emission.
func (m *VerboseMissionMemoryWrapper) Delete(ctx context.Context, key string) error {
	err := m.inner.Delete(ctx, key)

	// Only emit at VeryVerbose or higher
	if m.level >= LevelVeryVerbose && err == nil {
		event := NewVerboseEvent(EventMemoryGet, m.level, MemoryGetData{
			Tier:  m.tier,
			Key:   key,
			Found: false,
		}).WithMissionID(m.missionID).WithAgentName(m.agentName)

		_ = m.bus.Emit(ctx, event)
	}

	return err
}

// Search performs full-text search with event emission.
func (m *VerboseMissionMemoryWrapper) Search(ctx context.Context, query string, limit int) ([]memory.MemoryResult, error) {
	startTime := time.Now()
	results, err := m.inner.Search(ctx, query, limit)
	duration := time.Since(startTime)

	// Only emit at VeryVerbose or higher
	if m.level >= LevelVeryVerbose {
		resultCount := 0
		if err == nil {
			resultCount = len(results)
		}

		event := NewVerboseEvent(EventMemorySearch, m.level, MemorySearchData{
			Tier:        m.tier,
			Query:       query,
			ResultCount: resultCount,
			Duration:    duration,
		}).WithMissionID(m.missionID).WithAgentName(m.agentName)

		_ = m.bus.Emit(ctx, event)
	}

	return results, err
}

// History returns recent entries (pass-through without event).
func (m *VerboseMissionMemoryWrapper) History(ctx context.Context, limit int) ([]memory.MemoryItem, error) {
	return m.inner.History(ctx, limit)
}

// Keys returns all keys (pass-through without event).
func (m *VerboseMissionMemoryWrapper) Keys(ctx context.Context) ([]string, error) {
	return m.inner.Keys(ctx)
}

// MissionID returns the mission this memory is scoped to.
func (m *VerboseMissionMemoryWrapper) MissionID() types.ID {
	return m.inner.MissionID()
}

// Ensure VerboseMissionMemoryWrapper implements MissionMemory at compile time
var _ memory.MissionMemory = (*VerboseMissionMemoryWrapper)(nil)

// VerboseLongTermMemoryWrapper wraps a LongTermMemory instance and emits events.
type VerboseLongTermMemoryWrapper struct {
	inner     memory.LongTermMemory
	bus       VerboseEventBus
	level     VerboseLevel
	tier      string
	missionID types.ID
	agentName string
}

// NewVerboseLongTermMemoryWrapper creates a new VerboseLongTermMemoryWrapper.
func NewVerboseLongTermMemoryWrapper(inner memory.LongTermMemory, bus VerboseEventBus, level VerboseLevel, tier string, missionID types.ID, agentName string) *VerboseLongTermMemoryWrapper {
	return &VerboseLongTermMemoryWrapper{
		inner:     inner,
		bus:       bus,
		level:     level,
		tier:      tier,
		missionID: missionID,
		agentName: agentName,
	}
}

// Store adds content with event emission.
func (l *VerboseLongTermMemoryWrapper) Store(ctx context.Context, id string, content string, metadata map[string]any) error {
	err := l.inner.Store(ctx, id, content, metadata)

	// Only emit at VeryVerbose or higher
	if l.level >= LevelVeryVerbose && err == nil {
		event := NewVerboseEvent(EventMemorySet, l.level, MemorySetData{
			Tier:      l.tier,
			Key:       id,
			ValueSize: len(content),
		}).WithMissionID(l.missionID).WithAgentName(l.agentName)

		_ = l.bus.Emit(ctx, event)
	}

	return err
}

// Search finds similar content with event emission.
func (l *VerboseLongTermMemoryWrapper) Search(ctx context.Context, query string, topK int, filters map[string]any) ([]memory.MemoryResult, error) {
	startTime := time.Now()
	results, err := l.inner.Search(ctx, query, topK, filters)
	duration := time.Since(startTime)

	// Only emit at VeryVerbose or higher
	if l.level >= LevelVeryVerbose {
		resultCount := 0
		if err == nil {
			resultCount = len(results)
		}

		event := NewVerboseEvent(EventMemorySearch, l.level, MemorySearchData{
			Tier:        l.tier,
			Query:       query,
			ResultCount: resultCount,
			Duration:    duration,
		}).WithMissionID(l.missionID).WithAgentName(l.agentName)

		_ = l.bus.Emit(ctx, event)
	}

	return results, err
}

// SimilarFindings finds findings similar to the given content.
func (l *VerboseLongTermMemoryWrapper) SimilarFindings(ctx context.Context, content string, topK int) ([]memory.MemoryResult, error) {
	return l.Search(ctx, content, topK, map[string]any{"type": "finding"})
}

// SimilarPatterns finds attack patterns similar to the query.
func (l *VerboseLongTermMemoryWrapper) SimilarPatterns(ctx context.Context, pattern string, topK int) ([]memory.MemoryResult, error) {
	return l.Search(ctx, pattern, topK, map[string]any{"type": "pattern"})
}

// Delete removes content by ID with event emission.
func (l *VerboseLongTermMemoryWrapper) Delete(ctx context.Context, id string) error {
	err := l.inner.Delete(ctx, id)

	// Only emit at VeryVerbose or higher
	if l.level >= LevelVeryVerbose && err == nil {
		event := NewVerboseEvent(EventMemoryGet, l.level, MemoryGetData{
			Tier:  l.tier,
			Key:   id,
			Found: false,
		}).WithMissionID(l.missionID).WithAgentName(l.agentName)

		_ = l.bus.Emit(ctx, event)
	}

	return err
}

// Health returns the health status (pass-through without event).
func (l *VerboseLongTermMemoryWrapper) Health(ctx context.Context) types.HealthStatus {
	return l.inner.Health(ctx)
}

// Ensure VerboseLongTermMemoryWrapper implements LongTermMemory at compile time
var _ memory.LongTermMemory = (*VerboseLongTermMemoryWrapper)(nil)

// estimateValueSize calculates the approximate size of a value in bytes.
// This is used for logging value sizes without exposing actual values.
func estimateValueSize(value any) int {
	if value == nil {
		return 0
	}

	// Try JSON marshaling for size estimation
	data, err := json.Marshal(value)
	if err != nil {
		// Fallback to string representation length
		return len([]byte(valueToString(value)))
	}

	return len(data)
}

// valueToString converts a value to a string for size estimation fallback.
func valueToString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		// Use JSON marshaling as fallback
		data, _ := json.Marshal(v)
		return string(data)
	}
}
