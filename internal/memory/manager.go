package memory

import (
	"sync"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/memory/embedder"
	"github.com/zero-day-ai/gibson/internal/memory/vector"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MemoryManager provides unified memory access for a mission with lifecycle management.
// It extends MemoryStore with MissionID() and Close() for resource management.
type MemoryManager interface {
	MemoryStore

	// MissionID returns the mission this manager is scoped to.
	MissionID() types.ID

	// Close releases all resources held by the memory manager.
	// It clears working memory and closes the vector store.
	Close() error
}

// DefaultMemoryManager implements MemoryManager by composing all memory tiers.
type DefaultMemoryManager struct {
	missionID types.ID
	working   WorkingMemory
	mission   MissionMemory
	longTerm  LongTermMemory
	store     vector.VectorStore
	closeMu   sync.Mutex
	closed    bool
}

// NewMemoryManager creates a new MemoryManager with the specified configuration.
// It initializes all three memory tiers: working, mission, and long-term.
//
// Parameters:
//   - missionID: The mission ID to scope this memory manager to
//   - db: The database connection for mission memory persistence
//   - config: Memory configuration (uses defaults if nil)
//
// Returns a MemoryManager ready for use, or an error if initialization fails.
func NewMemoryManager(missionID types.ID, db *database.DB, config *MemoryConfig) (MemoryManager, error) {
	// Apply defaults if config is nil
	if config == nil {
		config = NewDefaultMemoryConfig()
	} else {
		config.ApplyDefaults()
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, NewInvalidConfigError("memory configuration validation failed: " + err.Error())
	}

	// Initialize working memory
	workingMem := NewWorkingMemory(config.Working.MaxTokens)

	// Initialize mission memory
	missionMem := NewMissionMemory(db, missionID, config.Mission.CacheSize)

	// Initialize embedder based on config
	var emb embedder.Embedder
	switch config.LongTerm.Embedder.Provider {
	case "mock":
		// Use mock embedder for testing
		emb = embedder.NewMockEmbedder()
	case "openai", "llm":
		// For now, use mock embedder as a placeholder
		// Real implementations will be added by other agents
		emb = embedder.NewMockEmbedder()
	default:
		// Default to mock embedder
		emb = embedder.NewMockEmbedder()
	}

	// Initialize vector store based on config
	var vectorStore vector.VectorStore
	switch config.LongTerm.Backend {
	case "embedded":
		// For now, use mock vector store as a placeholder
		// Real embedded implementation will be added by other agents
		vectorStore = vector.NewMockVectorStore()
	case "qdrant", "milvus":
		// For now, use mock vector store as a placeholder
		// Real implementations will be added by other agents
		vectorStore = vector.NewMockVectorStore()
	default:
		// Default to mock vector store
		vectorStore = vector.NewMockVectorStore()
	}

	// Initialize long-term memory
	longTermMem := NewLongTermMemory(vectorStore, emb)

	return &DefaultMemoryManager{
		missionID: missionID,
		working:   workingMem,
		mission:   missionMem,
		longTerm:  longTermMem,
		store:     vectorStore,
		closed:    false,
	}, nil
}

// Working returns the working memory instance.
func (m *DefaultMemoryManager) Working() WorkingMemory {
	return m.working
}

// Mission returns the mission memory instance.
func (m *DefaultMemoryManager) Mission() MissionMemory {
	return m.mission
}

// LongTerm returns the long-term memory instance.
func (m *DefaultMemoryManager) LongTerm() LongTermMemory {
	return m.longTerm
}

// MissionID returns the mission ID this manager is scoped to.
func (m *DefaultMemoryManager) MissionID() types.ID {
	return m.missionID
}

// Close releases all resources held by the memory manager.
// This method is idempotent and safe to call multiple times.
func (m *DefaultMemoryManager) Close() error {
	m.closeMu.Lock()
	defer m.closeMu.Unlock()

	if m.closed {
		return nil
	}

	// Clear working memory (ephemeral)
	m.working.Clear()

	// Close vector store
	if m.store != nil {
		if err := m.store.Close(); err != nil {
			return NewVectorStoreError("failed to close vector store", err)
		}
	}

	m.closed = true
	return nil
}
