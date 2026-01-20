package vector

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zero-day-ai/gibson/internal/types"
)

// VectorStoreConfig holds configuration for creating a vector store.
type VectorStoreConfig struct {
	Backend     string // "embedded", "sqlite", "qdrant", etc.
	StoragePath string // Path for sqlite backend
	Dimensions  int    // Embedding dimensions (e.g., 384 for all-MiniLM-L6-v2)
}

// NewVectorStore creates a vector store based on the configuration.
// Supported backends:
//   - "embedded": In-memory vector store (non-persistent, brute-force search)
//   - "sqlite": SQLite-backed vector store (persistent, suitable for production)
//   - "qdrant", "milvus": External vector databases (not yet implemented)
func NewVectorStore(cfg VectorStoreConfig) (VectorStore, error) {
	// Validate dimensions
	if cfg.Dimensions <= 0 {
		return nil, types.NewError(ErrCodeInvalidConfig,
			fmt.Sprintf("dimensions must be positive, got %d", cfg.Dimensions))
	}

	switch cfg.Backend {
	case "embedded", "":
		// In-memory vector store (default for backward compatibility)
		return NewEmbeddedVectorStore(cfg.Dimensions), nil

	case "sqlite":
		// SQLite-backed persistent vector store
		if cfg.StoragePath == "" {
			return nil, types.NewError(ErrCodeInvalidConfig,
				"storage_path is required for sqlite backend")
		}

		// Ensure storage directory exists
		dir := filepath.Dir(cfg.StoragePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, types.WrapError(ErrCodeVectorStoreFailed,
				"failed to create storage directory", err)
		}

		// Create SqliteVecStore
		store, err := NewSqliteVecStore(SqliteVecConfig{
			DBPath:    cfg.StoragePath,
			TableName: "vectors",
			Dims:      cfg.Dimensions,
		})
		if err != nil {
			return nil, types.WrapError(ErrCodeVectorStoreFailed,
				"failed to create sqlite vector store", err)
		}

		return store, nil

	case "qdrant":
		// Future: Qdrant vector database integration
		return nil, types.NewError(ErrCodeInvalidConfig,
			"qdrant backend not yet implemented - use 'embedded' or 'sqlite'")

	case "milvus":
		// Future: Milvus vector database integration
		return nil, types.NewError(ErrCodeInvalidConfig,
			"milvus backend not yet implemented - use 'embedded' or 'sqlite'")

	default:
		return nil, types.NewError(ErrCodeInvalidConfig,
			fmt.Sprintf("unknown backend '%s', must be one of: embedded, sqlite, qdrant, milvus",
				cfg.Backend))
	}
}
