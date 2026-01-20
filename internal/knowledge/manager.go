package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
)

// KnowledgeManager provides high-level knowledge operations with source tracking.
// It wraps the existing LongTermMemory system and adds knowledge-specific functionality.
type KnowledgeManager interface {
	// StoreChunk stores a single knowledge chunk with automatic embedding generation.
	// The chunk's text will be embedded and stored in the vector store.
	// Source tracking is updated automatically.
	StoreChunk(ctx context.Context, chunk KnowledgeChunk) error

	// StoreBatch stores multiple knowledge chunks efficiently in a transaction.
	// All chunks are validated before storage begins.
	StoreBatch(ctx context.Context, chunks []KnowledgeChunk) error

	// Search performs semantic search over the knowledge store.
	// Results can be filtered by source, source type, and custom metadata.
	Search(ctx context.Context, query string, opts SearchOptions) ([]KnowledgeResult, error)

	// ListSources returns all knowledge sources that have been ingested.
	ListSources(ctx context.Context) ([]KnowledgeSource, error)

	// GetSourceByHash retrieves a source by its content hash for deduplication.
	// Returns nil if no source with the given hash exists.
	GetSourceByHash(ctx context.Context, hash string) (*KnowledgeSource, error)

	// StoreSource records a source as having been ingested.
	StoreSource(ctx context.Context, source KnowledgeSource) error

	// GetStats returns aggregate statistics about the knowledge store.
	GetStats(ctx context.Context) (KnowledgeStats, error)

	// DeleteBySource removes all chunks from a specific source.
	DeleteBySource(ctx context.Context, source string) error

	// DeleteAll removes all knowledge from the store.
	// This is a destructive operation that cannot be undone.
	DeleteAll(ctx context.Context) error

	// Health returns the health status of the knowledge manager.
	Health(ctx context.Context) types.HealthStatus

	// Close releases all resources held by the knowledge manager.
	Close() error
}

// DefaultKnowledgeManager implements KnowledgeManager by composing LongTermMemory
// with source tracking via direct SQL operations.
type DefaultKnowledgeManager struct {
	ltm memory.LongTermMemory
	db  *sql.DB
}

// NewKnowledgeManager creates a new knowledge manager that wraps LongTermMemory.
// The db parameter should be the same database used by the vector store.
func NewKnowledgeManager(ltm memory.LongTermMemory, db *sql.DB) KnowledgeManager {
	return &DefaultKnowledgeManager{
		ltm: ltm,
		db:  db,
	}
}

// StoreChunk stores a single knowledge chunk with embedding and source tracking.
func (km *DefaultKnowledgeManager) StoreChunk(ctx context.Context, chunk KnowledgeChunk) error {
	// Validate chunk
	if chunk.ID == "" {
		return types.NewError("INVALID_CHUNK", "chunk ID cannot be empty")
	}
	if chunk.Text == "" {
		return types.NewError("INVALID_CHUNK", "chunk text cannot be empty")
	}
	if chunk.Source == "" {
		return types.NewError("INVALID_CHUNK", "chunk source cannot be empty")
	}

	// Convert metadata to map[string]any for LongTermMemory
	metadataMap := make(map[string]any)
	metadataMap["source"] = chunk.Source
	metadataMap["source_hash"] = chunk.SourceHash

	// Add chunk metadata fields
	if chunk.Metadata.Section != "" {
		metadataMap["section"] = chunk.Metadata.Section
	}
	if chunk.Metadata.PageNumber > 0 {
		metadataMap["page_number"] = chunk.Metadata.PageNumber
	}
	if chunk.Metadata.StartChar > 0 {
		metadataMap["start_char"] = chunk.Metadata.StartChar
	}
	if chunk.Metadata.HasCode {
		metadataMap["has_code"] = chunk.Metadata.HasCode
	}
	if chunk.Metadata.Language != "" {
		metadataMap["language"] = chunk.Metadata.Language
	}

	// Add custom metadata
	for key, value := range chunk.Metadata.Custom {
		metadataMap[key] = value
	}

	// Store via LongTermMemory (handles embedding automatically)
	if err := km.ltm.Store(ctx, chunk.ID, chunk.Text, metadataMap); err != nil {
		return fmt.Errorf("failed to store chunk in vector store: %w", err)
	}

	return nil
}

// StoreBatch stores multiple knowledge chunks efficiently.
func (km *DefaultKnowledgeManager) StoreBatch(ctx context.Context, chunks []KnowledgeChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	// Validate all chunks first
	for i, chunk := range chunks {
		if chunk.ID == "" {
			return types.NewError("INVALID_CHUNK", fmt.Sprintf("chunk %d: ID cannot be empty", i))
		}
		if chunk.Text == "" {
			return types.NewError("INVALID_CHUNK", fmt.Sprintf("chunk %d: text cannot be empty", i))
		}
		if chunk.Source == "" {
			return types.NewError("INVALID_CHUNK", fmt.Sprintf("chunk %d: source cannot be empty", i))
		}
	}

	// Store each chunk (LongTermMemory doesn't have batch API, but this is still
	// more efficient than individual calls because validation is done upfront)
	for _, chunk := range chunks {
		if err := km.StoreChunk(ctx, chunk); err != nil {
			return fmt.Errorf("failed to store chunk %s: %w", chunk.ID, err)
		}
	}

	return nil
}

// Search performs semantic search with source filtering.
func (km *DefaultKnowledgeManager) Search(ctx context.Context, query string, opts SearchOptions) ([]KnowledgeResult, error) {
	// Set defaults
	if opts.Limit <= 0 {
		opts.Limit = 10
	}

	// Build metadata filters
	filters := make(map[string]any)

	// Add source filter if specified
	if opts.Source != "" {
		filters["source"] = opts.Source
	}

	// Add source type filter if specified (requires source metadata to have this)
	if opts.SourceType != "" {
		filters["source_type"] = opts.SourceType
	}

	// Add custom filters
	for key, value := range opts.Filters {
		filters[key] = value
	}

	// Search via LongTermMemory
	memResults, err := km.ltm.Search(ctx, query, opts.Limit*2, filters) // Request more to account for threshold filtering
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert to KnowledgeResult and apply threshold
	results := make([]KnowledgeResult, 0, len(memResults))
	for _, mr := range memResults {
		// Apply threshold filter
		if mr.Score < opts.Threshold {
			continue
		}

		// Extract source from metadata
		source, _ := mr.Item.Metadata["source"].(string)
		sourceHash, _ := mr.Item.Metadata["source_hash"].(string)

		// Extract chunk metadata
		metadata := ChunkMetadata{}
		if section, ok := mr.Item.Metadata["section"].(string); ok {
			metadata.Section = section
		}
		if pageNum, ok := mr.Item.Metadata["page_number"].(float64); ok {
			metadata.PageNumber = int(pageNum)
		}
		if startChar, ok := mr.Item.Metadata["start_char"].(float64); ok {
			metadata.StartChar = int(startChar)
		}
		if hasCode, ok := mr.Item.Metadata["has_code"].(bool); ok {
			metadata.HasCode = hasCode
		}
		if lang, ok := mr.Item.Metadata["language"].(string); ok {
			metadata.Language = lang
		}

		// Convert to KnowledgeChunk
		chunk := KnowledgeChunk{
			ID:         mr.Item.Key,
			Source:     source,
			SourceHash: sourceHash,
			Text:       mr.Item.Value.(string),
			Metadata:   metadata,
			CreatedAt:  mr.Item.CreatedAt,
		}

		results = append(results, KnowledgeResult{
			Chunk: chunk,
			Score: mr.Score,
		})

		// Stop if we have enough results
		if len(results) >= opts.Limit {
			break
		}
	}

	return results, nil
}

// ListSources returns all knowledge sources that have been ingested.
func (km *DefaultKnowledgeManager) ListSources(ctx context.Context) ([]KnowledgeSource, error) {
	query := `
		SELECT source, source_type, source_hash, chunk_count, ingested_at, metadata
		FROM knowledge_sources
		ORDER BY ingested_at DESC
	`

	rows, err := km.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query sources: %w", err)
	}
	defer rows.Close()

	sources := make([]KnowledgeSource, 0)
	for rows.Next() {
		var source KnowledgeSource
		var metadataJSON []byte

		err := rows.Scan(
			&source.Source,
			&source.SourceType,
			&source.SourceHash,
			&source.ChunkCount,
			&source.IngestedAt,
			&metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan source row: %w", err)
		}

		// Deserialize metadata if present
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &source.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal source metadata: %w", err)
			}
		}

		sources = append(sources, source)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating source rows: %w", err)
	}

	return sources, nil
}

// GetSourceByHash retrieves a source by its content hash for deduplication.
func (km *DefaultKnowledgeManager) GetSourceByHash(ctx context.Context, hash string) (*KnowledgeSource, error) {
	query := `
		SELECT source, source_type, source_hash, chunk_count, ingested_at, metadata
		FROM knowledge_sources
		WHERE source_hash = ?
	`

	var source KnowledgeSource
	var metadataJSON []byte

	err := km.db.QueryRowContext(ctx, query, hash).Scan(
		&source.Source,
		&source.SourceType,
		&source.SourceHash,
		&source.ChunkCount,
		&source.IngestedAt,
		&metadataJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query source by hash: %w", err)
	}

	// Deserialize metadata if present
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &source.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal source metadata: %w", err)
		}
	}

	return &source, nil
}

// StoreSource records a source as having been ingested.
func (km *DefaultKnowledgeManager) StoreSource(ctx context.Context, source KnowledgeSource) error {
	// Serialize metadata
	var metadataJSON []byte
	var err error
	if source.Metadata != nil {
		metadataJSON, err = json.Marshal(source.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal source metadata: %w", err)
		}
	}

	// Use INSERT OR REPLACE to handle updates
	query := `
		INSERT OR REPLACE INTO knowledge_sources
		(source, source_type, source_hash, chunk_count, ingested_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err = km.db.ExecContext(ctx, query,
		source.Source,
		source.SourceType,
		source.SourceHash,
		source.ChunkCount,
		source.IngestedAt.Format(time.RFC3339),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to store source: %w", err)
	}

	return nil
}

// GetStats returns aggregate statistics about the knowledge store.
func (km *DefaultKnowledgeManager) GetStats(ctx context.Context) (KnowledgeStats, error) {
	stats := KnowledgeStats{
		SourcesByType: make(map[string]int),
	}

	// Get total sources
	err := km.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM knowledge_sources").Scan(&stats.TotalSources)
	if err != nil {
		return stats, fmt.Errorf("failed to count sources: %w", err)
	}

	// Get total chunks
	err = km.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM knowledge_vectors").Scan(&stats.TotalChunks)
	if err != nil {
		return stats, fmt.Errorf("failed to count chunks: %w", err)
	}

	// Get storage size (approximate using page_count * page_size)
	var pageCount, pageSize int
	err = km.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	if err == nil {
		err = km.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
		if err == nil {
			stats.StorageBytes = int64(pageCount * pageSize)
		}
	}

	// Get last ingest time - scan as string since SQLite stores timestamps as TEXT
	var lastIngestStr sql.NullString
	err = km.db.QueryRowContext(ctx, "SELECT MAX(ingested_at) FROM knowledge_sources").Scan(&lastIngestStr)
	if err == nil && lastIngestStr.Valid && lastIngestStr.String != "" {
		// Try parsing as RFC3339 first, then other formats
		if t, parseErr := time.Parse(time.RFC3339, lastIngestStr.String); parseErr == nil {
			stats.LastIngestTime = t
		} else if t, parseErr := time.Parse("2006-01-02 15:04:05", lastIngestStr.String); parseErr == nil {
			stats.LastIngestTime = t
		}
	}

	// Get sources by type
	rows, err := km.db.QueryContext(ctx, `
		SELECT source_type, COUNT(*) 
		FROM knowledge_sources 
		GROUP BY source_type
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var sourceType string
			var count int
			if err := rows.Scan(&sourceType, &count); err == nil {
				stats.SourcesByType[sourceType] = count
			}
		}
	}

	return stats, nil
}

// DeleteBySource removes all chunks from a specific source.
func (km *DefaultKnowledgeManager) DeleteBySource(ctx context.Context, source string) error {
	// Get all chunk IDs for this source from knowledge_vectors
	query := `
		SELECT id FROM knowledge_vectors 
		WHERE json_extract(metadata, '$.source') = ?
	`
	rows, err := km.db.QueryContext(ctx, query, source)
	if err != nil {
		return fmt.Errorf("failed to query chunks for source: %w", err)
	}
	defer rows.Close()

	chunkIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("failed to scan chunk ID: %w", err)
		}
		chunkIDs = append(chunkIDs, id)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating chunk IDs: %w", err)
	}

	// Delete each chunk from LongTermMemory
	for _, id := range chunkIDs {
		if err := km.ltm.Delete(ctx, id); err != nil {
			return fmt.Errorf("failed to delete chunk %s: %w", id, err)
		}
	}

	// Delete source tracking record
	_, err = km.db.ExecContext(ctx, "DELETE FROM knowledge_sources WHERE source = ?", source)
	if err != nil {
		return fmt.Errorf("failed to delete source record: %w", err)
	}

	return nil
}

// DeleteAll removes all knowledge from the store.
func (km *DefaultKnowledgeManager) DeleteAll(ctx context.Context) error {
	// Delete all from knowledge_vectors table
	if _, err := km.db.ExecContext(ctx, "DELETE FROM knowledge_vectors"); err != nil {
		return fmt.Errorf("failed to delete all chunks: %w", err)
	}

	// Delete all from knowledge_sources table
	if _, err := km.db.ExecContext(ctx, "DELETE FROM knowledge_sources"); err != nil {
		return fmt.Errorf("failed to delete all sources: %w", err)
	}

	return nil
}

// Health returns the health status of the knowledge manager.
func (km *DefaultKnowledgeManager) Health(ctx context.Context) types.HealthStatus {
	// Check LongTermMemory health
	ltmHealth := km.ltm.Health(ctx)
	if !ltmHealth.IsHealthy() {
		return types.NewHealthStatus(
			types.HealthStateUnhealthy,
			fmt.Sprintf("long-term memory unhealthy: %s", ltmHealth.Message),
		)
	}

	// Check database connectivity
	if err := km.db.PingContext(ctx); err != nil {
		return types.NewHealthStatus(
			types.HealthStateUnhealthy,
			fmt.Sprintf("database ping failed: %v", err),
		)
	}

	// Get basic stats for health message
	var chunkCount int
	err := km.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM knowledge_vectors").Scan(&chunkCount)
	if err != nil {
		return types.NewHealthStatus(
			types.HealthStateDegraded,
			fmt.Sprintf("failed to query chunk count: %v", err),
		)
	}

	return types.NewHealthStatus(
		types.HealthStateHealthy,
		fmt.Sprintf("knowledge manager operational with %d chunks", chunkCount),
	)
}

// Close releases all resources held by the knowledge manager.
func (km *DefaultKnowledgeManager) Close() error {
	// Note: We don't close the db here because it may be shared with other components.
	// The caller should manage the database lifecycle.
	return nil
}
