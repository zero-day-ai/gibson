package daemon

import (
	"context"
	"fmt"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MemoryManagerFactory creates MemoryManager instances for missions.
//
// Each mission needs isolated memory storage so agents can use working memory,
// mission memory, and long-term memory without cross-mission contamination.
// The factory ensures consistent configuration across all memory managers while
// scoping storage to individual missions.
//
// The factory is initialized once during daemon startup and reused for all
// mission memory manager creation.
type MemoryManagerFactory struct {
	// db is the database connection for mission-tier persistence
	db *database.DB

	// config is the memory configuration to apply to all managers
	config *memory.MemoryConfig
}

// NewMemoryManagerFactory creates a new MemoryManagerFactory.
//
// Parameters:
//   - db: Database connection for mission memory persistence
//   - config: Memory configuration (uses defaults if nil)
//
// Returns:
//   - *MemoryManagerFactory: Ready to create memory managers
//   - error: Non-nil if validation fails
func NewMemoryManagerFactory(db *database.DB, config *memory.MemoryConfig) (*MemoryManagerFactory, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection cannot be nil")
	}

	// Apply defaults if config is nil
	if config == nil {
		config = memory.NewDefaultMemoryConfig()
	} else {
		config.ApplyDefaults()
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("memory configuration validation failed: %w", err)
	}

	return &MemoryManagerFactory{
		db:     db,
		config: config,
	}, nil
}

// CreateForMission creates a new MemoryManager scoped to a specific mission.
//
// Each mission receives a fresh MemoryManager with:
//   - Working memory: Ephemeral in-memory key-value store
//   - Mission memory: SQLite persistence scoped to this mission ID
//   - Long-term memory: Vector store for semantic search
//
// The MemoryManager should be closed when the mission completes to release
// resources (working memory is cleared, vector store is closed).
//
// Parameters:
//   - ctx: Context for initialization operations
//   - missionID: The mission ID to scope this memory manager to
//
// Returns:
//   - memory.MemoryManager: Configured memory manager for the mission
//   - error: Non-nil if creation or initialization fails
func (f *MemoryManagerFactory) CreateForMission(ctx context.Context, missionID types.ID) (memory.MemoryManager, error) {
	if missionID.IsZero() {
		return nil, fmt.Errorf("mission ID cannot be zero")
	}

	// Validate mission ID
	if err := missionID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid mission ID: %w", err)
	}

	// Create memory manager using the factory's configuration
	// The NewMemoryManager constructor handles all tier initialization:
	//   - Working memory (in-memory, ephemeral)
	//   - Mission memory (SQLite, mission-scoped)
	//   - Long-term memory (vector store with embeddings)
	mgr, err := memory.NewMemoryManager(missionID, f.db, f.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory manager for mission %s: %w", missionID, err)
	}

	return mgr, nil
}

// Config returns the memory configuration used by this factory.
//
// This is useful for testing and introspection.
func (f *MemoryManagerFactory) Config() *memory.MemoryConfig {
	return f.config
}

// DB returns the database connection used by this factory.
//
// This is useful for testing and introspection.
func (f *MemoryManagerFactory) DB() *database.DB {
	return f.db
}
