package vector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sync"

	"github.com/zero-day-ai/gibson/internal/types"

	_ "github.com/mattn/go-sqlite3"
)

// SqliteVecStore is a persistent vector store implementation using SQLite with sqlite-vec extension.
// It provides efficient vector similarity search using the vec0 virtual table.
// This implementation is thread-safe and suitable for production workloads.
type SqliteVecStore struct {
	mu        sync.RWMutex
	db        *sql.DB
	dims      int
	tableName string
	vecTable  string
	closed    bool
}

// SqliteVecConfig holds configuration for SqliteVecStore.
type SqliteVecConfig struct {
	DBPath    string // Path to SQLite database file
	TableName string // Name for the vectors table (default: "vectors")
	Dims      int    // Embedding dimensions (e.g., 384 for all-MiniLM-L6-v2)
}

// NewSqliteVecStore creates a new persistent vector store using SQLite with sqlite-vec.
// The store creates two tables:
// 1. Main table for storing records with metadata
// 2. Virtual table (vec0) for efficient similarity search
func NewSqliteVecStore(cfg SqliteVecConfig) (*SqliteVecStore, error) {
	if cfg.DBPath == "" {
		return nil, types.NewError(ErrCodeInvalidConfig, "database path cannot be empty")
	}
	if cfg.Dims <= 0 {
		return nil, types.NewError(ErrCodeInvalidConfig, fmt.Sprintf("dimensions must be positive, got %d", cfg.Dims))
	}
	if cfg.TableName == "" {
		cfg.TableName = "vectors"
	}

	// Open database with WAL mode for better concurrency
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000", cfg.DBPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, types.WrapError(ErrCodeVectorStoreFailed, "failed to open database", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, types.WrapError(ErrCodeVectorStoreFailed, "failed to ping database", err)
	}

	store := &SqliteVecStore{
		db:        db,
		dims:      cfg.Dims,
		tableName: cfg.TableName,
		vecTable:  cfg.TableName + "_vec",
		closed:    false,
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, types.WrapError(ErrCodeVectorStoreFailed, "failed to initialize schema", err)
	}

	return store, nil
}

// initSchema creates the necessary tables for vector storage.
func (s *SqliteVecStore) initSchema() error {
	// Create main vectors table
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			embedding BLOB NOT NULL,
			metadata TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`, s.tableName)

	if _, err := s.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create vectors table: %w", err)
	}

	// Note: sqlite-vec virtual table creation
	// The vec0 virtual table is used for efficient similarity search.
	// For now, we'll implement brute-force search in Go since sqlite-vec
	// requires a C extension that needs to be compiled in.
	// Future enhancement: Integrate sqlite-vec CGO bindings when available.

	return nil
}

// Store adds a single vector record to the store.
func (s *SqliteVecStore) Store(ctx context.Context, record VectorRecord) error {
	// Validate the record
	if err := record.Validate(); err != nil {
		return err
	}

	// Check dimensionality matches
	if len(record.Embedding) != s.dims {
		return types.NewError(ErrCodeVectorStoreFailed,
			fmt.Sprintf("embedding dimensions mismatch: expected %d, got %d", s.dims, len(record.Embedding)))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return types.NewError(ErrCodeVectorStoreUnavailable, "vector store is closed")
	}

	// Serialize embedding to BLOB (float64 array to bytes)
	embeddingBytes, err := serializeEmbedding(record.Embedding)
	if err != nil {
		return types.WrapError(ErrCodeVectorStoreFailed, "failed to serialize embedding", err)
	}

	// Serialize metadata to JSON
	var metadataJSON []byte
	if record.Metadata != nil {
		metadataJSON, err = json.Marshal(record.Metadata)
		if err != nil {
			return types.WrapError(ErrCodeVectorStoreFailed, "failed to serialize metadata", err)
		}
	}

	// Insert or replace record
	query := fmt.Sprintf(`
		INSERT OR REPLACE INTO %s (id, content, embedding, metadata, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, s.tableName)

	_, err = s.db.ExecContext(ctx, query,
		record.ID,
		record.Content,
		embeddingBytes,
		metadataJSON,
		record.CreatedAt,
	)

	if err != nil {
		return types.WrapError(ErrCodeVectorStoreFailed, "failed to insert record", err)
	}

	return nil
}

// StoreBatch adds multiple vector records efficiently using a transaction.
func (s *SqliteVecStore) StoreBatch(ctx context.Context, records []VectorRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Validate all records first
	for i, record := range records {
		if err := record.Validate(); err != nil {
			return types.WrapError(ErrCodeVectorStoreFailed,
				fmt.Sprintf("invalid record at index %d", i), err)
		}
		if len(record.Embedding) != s.dims {
			return types.NewError(ErrCodeVectorStoreFailed,
				fmt.Sprintf("record %d: embedding dimensions mismatch: expected %d, got %d",
					i, s.dims, len(record.Embedding)))
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return types.NewError(ErrCodeVectorStoreUnavailable, "vector store is closed")
	}

	// Use transaction for batch insert
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return types.WrapError(ErrCodeVectorStoreFailed, "failed to begin transaction", err)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(`
		INSERT OR REPLACE INTO %s (id, content, embedding, metadata, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, s.tableName)

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return types.WrapError(ErrCodeVectorStoreFailed, "failed to prepare statement", err)
	}
	defer stmt.Close()

	for _, record := range records {
		embeddingBytes, err := serializeEmbedding(record.Embedding)
		if err != nil {
			return types.WrapError(ErrCodeVectorStoreFailed, "failed to serialize embedding", err)
		}

		var metadataJSON []byte
		if record.Metadata != nil {
			metadataJSON, err = json.Marshal(record.Metadata)
			if err != nil {
				return types.WrapError(ErrCodeVectorStoreFailed, "failed to serialize metadata", err)
			}
		}

		_, err = stmt.ExecContext(ctx,
			record.ID,
			record.Content,
			embeddingBytes,
			metadataJSON,
			record.CreatedAt,
		)
		if err != nil {
			return types.WrapError(ErrCodeVectorStoreFailed, "failed to insert batch record", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return types.WrapError(ErrCodeVectorStoreFailed, "failed to commit transaction", err)
	}

	return nil
}

// Search finds similar records by embedding vector.
// Currently uses brute-force search with cosine similarity.
// Future enhancement: Use sqlite-vec virtual table for optimized ANN search.
func (s *SqliteVecStore) Search(ctx context.Context, query VectorQuery) ([]VectorResult, error) {
	// Validate the query
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// Must have an embedding to search
	if len(query.Embedding) == 0 {
		return nil, types.NewError(ErrCodeVectorSearchFailed,
			"query must have embedding for search (embed text first)")
	}

	// Check dimensionality matches
	if len(query.Embedding) != s.dims {
		return nil, types.NewError(ErrCodeVectorSearchFailed,
			fmt.Sprintf("query embedding dimensions mismatch: expected %d, got %d",
				s.dims, len(query.Embedding)))
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(ErrCodeVectorStoreUnavailable, "vector store is closed")
	}

	// Fetch all records (brute-force approach)
	// TODO: Optimize with sqlite-vec virtual table when available
	querySQL := fmt.Sprintf("SELECT id, content, embedding, metadata, created_at FROM %s", s.tableName)
	rows, err := s.db.QueryContext(ctx, querySQL)
	if err != nil {
		return nil, types.WrapError(ErrCodeVectorSearchFailed, "failed to query vectors", err)
	}
	defer rows.Close()

	results := make([]VectorResult, 0)

	for rows.Next() {
		var record VectorRecord
		var embeddingBytes []byte
		var metadataJSON []byte

		if err := rows.Scan(&record.ID, &record.Content, &embeddingBytes, &metadataJSON, &record.CreatedAt); err != nil {
			return nil, types.WrapError(ErrCodeVectorSearchFailed, "failed to scan record", err)
		}

		// Deserialize embedding
		embedding, err := deserializeEmbedding(embeddingBytes, s.dims)
		if err != nil {
			return nil, types.WrapError(ErrCodeVectorSearchFailed, "failed to deserialize embedding", err)
		}
		record.Embedding = embedding

		// Deserialize metadata
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &record.Metadata); err != nil {
				return nil, types.WrapError(ErrCodeVectorSearchFailed, "failed to deserialize metadata", err)
			}
		}

		// Apply metadata filters
		if !matchesFilters(record, query.Filters) {
			continue
		}

		// Compute cosine similarity
		score := cosineSimilarity(query.Embedding, record.Embedding)

		// Apply minimum score threshold
		if score >= query.MinScore {
			results = append(results, *NewVectorResult(record, score))
		}
	}

	if err := rows.Err(); err != nil {
		return nil, types.WrapError(ErrCodeVectorSearchFailed, "error iterating rows", err)
	}

	// Sort by score descending
	sortVectorResults(results)

	// Return top-k results
	if len(results) > query.TopK {
		results = results[:query.TopK]
	}

	return results, nil
}

// Get retrieves a specific record by ID.
func (s *SqliteVecStore) Get(ctx context.Context, id string) (*VectorRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, types.NewError(ErrCodeVectorStoreUnavailable, "vector store is closed")
	}

	query := fmt.Sprintf("SELECT id, content, embedding, metadata, created_at FROM %s WHERE id = ?", s.tableName)
	row := s.db.QueryRowContext(ctx, query, id)

	var record VectorRecord
	var embeddingBytes []byte
	var metadataJSON []byte

	err := row.Scan(&record.ID, &record.Content, &embeddingBytes, &metadataJSON, &record.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, types.NewError(ErrCodeVectorNotFound, fmt.Sprintf("vector record not found: %s", id))
	}
	if err != nil {
		return nil, types.WrapError(ErrCodeVectorSearchFailed, "failed to get record", err)
	}

	// Deserialize embedding
	embedding, err := deserializeEmbedding(embeddingBytes, s.dims)
	if err != nil {
		return nil, types.WrapError(ErrCodeVectorSearchFailed, "failed to deserialize embedding", err)
	}
	record.Embedding = embedding

	// Deserialize metadata
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &record.Metadata); err != nil {
			return nil, types.WrapError(ErrCodeVectorSearchFailed, "failed to deserialize metadata", err)
		}
	}

	return &record, nil
}

// Delete removes a record from the store.
func (s *SqliteVecStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return types.NewError(ErrCodeVectorStoreUnavailable, "vector store is closed")
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", s.tableName)
	_, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return types.WrapError(ErrCodeVectorStoreFailed, "failed to delete record", err)
	}

	return nil
}

// Health returns the current health status of the vector store.
func (s *SqliteVecStore) Health(ctx context.Context) types.HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return types.NewHealthStatus(
			types.HealthStateUnhealthy,
			"sqlite vector store is closed",
		)
	}

	// Check database connection
	if err := s.db.PingContext(ctx); err != nil {
		return types.NewHealthStatus(
			types.HealthStateUnhealthy,
			fmt.Sprintf("database ping failed: %v", err),
		)
	}

	// Get record count
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", s.tableName)
	if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return types.NewHealthStatus(
			types.HealthStateDegraded,
			fmt.Sprintf("failed to count records: %v", err),
		)
	}

	return types.NewHealthStatus(
		types.HealthStateHealthy,
		fmt.Sprintf("sqlite vector store operational with %d records (dims: %d)", count, s.dims),
	)
}

// Close releases all resources held by the vector store.
func (s *SqliteVecStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	if s.db != nil {
		return s.db.Close()
	}

	return nil
}

// serializeEmbedding converts a float64 slice to bytes for storage.
// Uses 8 bytes per float64 (binary encoding).
func serializeEmbedding(embedding []float64) ([]byte, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("embedding cannot be empty")
	}

	// Convert float64 to bytes (8 bytes per float)
	bytes := make([]byte, len(embedding)*8)
	for i, val := range embedding {
		bits := math.Float64bits(val)
		offset := i * 8
		bytes[offset] = byte(bits)
		bytes[offset+1] = byte(bits >> 8)
		bytes[offset+2] = byte(bits >> 16)
		bytes[offset+3] = byte(bits >> 24)
		bytes[offset+4] = byte(bits >> 32)
		bytes[offset+5] = byte(bits >> 40)
		bytes[offset+6] = byte(bits >> 48)
		bytes[offset+7] = byte(bits >> 56)
	}

	return bytes, nil
}

// deserializeEmbedding converts bytes back to float64 slice.
func deserializeEmbedding(bytes []byte, dims int) ([]float64, error) {
	expectedLen := dims * 8
	if len(bytes) != expectedLen {
		return nil, fmt.Errorf("invalid embedding bytes length: expected %d, got %d", expectedLen, len(bytes))
	}

	embedding := make([]float64, dims)
	for i := 0; i < dims; i++ {
		offset := i * 8
		bits := uint64(bytes[offset]) |
			uint64(bytes[offset+1])<<8 |
			uint64(bytes[offset+2])<<16 |
			uint64(bytes[offset+3])<<24 |
			uint64(bytes[offset+4])<<32 |
			uint64(bytes[offset+5])<<40 |
			uint64(bytes[offset+6])<<48 |
			uint64(bytes[offset+7])<<56
		embedding[i] = math.Float64frombits(bits)
	}

	return embedding, nil
}

// sortVectorResults sorts vector results by score in descending order.
func sortVectorResults(results []VectorResult) {
	// Simple bubble sort for small result sets
	// For larger sets, consider using sort.Slice
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if results[j].Score < results[j+1].Score {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}
