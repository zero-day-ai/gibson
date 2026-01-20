package database

import (
	"context"
	"database/sql"
	"testing"
)

// TestKnowledgeSuiteMigration verifies that the knowledge suite migration (migration 13) runs successfully
func TestKnowledgeSuiteMigration(t *testing.T) {
	// Use setupTestDB helper from existing tests
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create migrator
	migrator := NewMigrator(db)

	// Run all migrations including knowledge suite
	ctx := context.Background()
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Verify current version is 13 (all migrations applied)
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("failed to get current version: %v", err)
	}
	if version != 13 {
		t.Errorf("expected version 13, got %d", version)
	}

	// Verify all knowledge tables exist
	tables := []string{
		"knowledge_vectors",
		"knowledge_sources",
	}

	for _, table := range tables {
		var name string
		query := "SELECT name FROM sqlite_master WHERE type='table' AND name=?"
		err := db.conn.QueryRowContext(ctx, query, table).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("table %s does not exist", table)
		} else if err != nil {
			t.Fatalf("failed to query for table %s: %v", table, err)
		}
	}

	// Verify knowledge_vectors table schema by attempting to insert a test row
	_, err = db.conn.ExecContext(ctx, `
		INSERT INTO knowledge_vectors (id, content, embedding, metadata)
		VALUES ('test-id', 'test content', X'00000000', '{"source":"test.pdf"}')
	`)
	if err != nil {
		t.Errorf("failed to insert test knowledge vector: %v", err)
	}

	// Verify knowledge_sources table schema
	_, err = db.conn.ExecContext(ctx, `
		INSERT INTO knowledge_sources (source, source_type, source_hash, chunk_count, metadata)
		VALUES ('test.pdf', 'pdf', 'hash123', 5, '{"pages":10}')
	`)
	if err != nil {
		t.Errorf("failed to insert test knowledge source: %v", err)
	}

	// Verify we can query the data back
	var count int
	err = db.conn.QueryRowContext(ctx, "SELECT chunk_count FROM knowledge_sources WHERE source = 'test.pdf'").Scan(&count)
	if err != nil {
		t.Errorf("failed to query knowledge source: %v", err)
	}
	if count != 5 {
		t.Errorf("expected chunk_count 5, got %d", count)
	}

	// Verify indexes exist by checking sqlite_master
	indexes := []string{
		"idx_knowledge_vectors_created_at",
		"idx_knowledge_sources_hash",
		"idx_knowledge_sources_type",
		"idx_knowledge_sources_ingested_at",
	}

	for _, idx := range indexes {
		var name string
		query := "SELECT name FROM sqlite_master WHERE type='index' AND name=?"
		err := db.conn.QueryRowContext(ctx, query, idx).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("index %s does not exist", idx)
		} else if err != nil {
			t.Fatalf("failed to query for index %s: %v", idx, err)
		}
	}
}

// TestKnowledgeSuiteMigrationRollback verifies that rollback works correctly
func TestKnowledgeSuiteMigrationRollback(t *testing.T) {
	// Use setupTestDB helper from existing tests
	db, cleanup := setupTestDB(t)
	defer cleanup()

	migrator := NewMigrator(db)
	ctx := context.Background()

	// Run all migrations
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Rollback to version 12 (before knowledge suite)
	if err := migrator.Rollback(ctx, 12); err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	// Verify version is now 12
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("failed to get current version: %v", err)
	}
	if version != 12 {
		t.Errorf("expected version 12 after rollback, got %d", version)
	}

	// Verify knowledge tables no longer exist
	tables := []string{
		"knowledge_vectors",
		"knowledge_sources",
	}

	for _, table := range tables {
		var name string
		query := "SELECT name FROM sqlite_master WHERE type='table' AND name=?"
		err := db.conn.QueryRowContext(ctx, query, table).Scan(&name)
		if err != sql.ErrNoRows {
			t.Errorf("table %s should not exist after rollback, but it does", table)
		}
	}
}

// TestKnowledgeSuiteIdempotent verifies that the knowledge suite migration can run multiple times
func TestKnowledgeSuiteIdempotent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	migrator := NewMigrator(db)
	ctx := context.Background()

	// Apply migrations twice
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("first migrate failed: %v", err)
	}

	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("second migrate failed: %v", err)
	}

	// Verify version is still 13
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("failed to get current version: %v", err)
	}
	if version != 13 {
		t.Errorf("expected version 13, got %d", version)
	}
}

// TestKnowledgeSourcesQuery verifies that knowledge sources can be queried efficiently
func TestKnowledgeSourcesQuery(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	migrator := NewMigrator(db)
	ctx := context.Background()

	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Insert multiple test sources
	sources := []struct {
		source     string
		sourceType string
		sourceHash string
		chunkCount int
	}{
		{"paper1.pdf", "pdf", "hash1", 10},
		{"paper2.pdf", "pdf", "hash2", 15},
		{"blog.html", "url", "hash3", 5},
		{"notes.txt", "text", "hash4", 3},
	}

	for _, s := range sources {
		_, err := db.conn.ExecContext(ctx, `
			INSERT INTO knowledge_sources (source, source_type, source_hash, chunk_count)
			VALUES (?, ?, ?, ?)`,
			s.source, s.sourceType, s.sourceHash, s.chunkCount)
		if err != nil {
			t.Fatalf("failed to insert knowledge source: %v", err)
		}
	}

	// Query by source type (should use index)
	rows, err := db.conn.QueryContext(ctx, "SELECT source FROM knowledge_sources WHERE source_type = 'pdf' ORDER BY source")
	if err != nil {
		t.Fatalf("failed to query by source type: %v", err)
	}
	defer rows.Close()

	var pdfSources []string
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			t.Fatalf("failed to scan source: %v", err)
		}
		pdfSources = append(pdfSources, source)
	}

	if len(pdfSources) != 2 {
		t.Errorf("expected 2 pdf sources, got %d", len(pdfSources))
	}

	// Query by hash (should use index)
	var source string
	err = db.conn.QueryRowContext(ctx, "SELECT source FROM knowledge_sources WHERE source_hash = 'hash3'").Scan(&source)
	if err != nil {
		t.Fatalf("failed to query by hash: %v", err)
	}
	if source != "blog.html" {
		t.Errorf("expected source 'blog.html', got %s", source)
	}

	// Query total chunks
	var totalChunks int
	err = db.conn.QueryRowContext(ctx, "SELECT SUM(chunk_count) FROM knowledge_sources").Scan(&totalChunks)
	if err != nil {
		t.Fatalf("failed to query total chunks: %v", err)
	}
	if totalChunks != 33 {
		t.Errorf("expected total chunks 33, got %d", totalChunks)
	}
}

// TestKnowledgeVectorsEmbeddingStorage verifies that embeddings can be stored as BLOBs
func TestKnowledgeVectorsEmbeddingStorage(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	migrator := NewMigrator(db)
	ctx := context.Background()

	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Create a test embedding (384 dimensions, 4 bytes per float32 = 1536 bytes)
	embedding := make([]byte, 1536)
	for i := range embedding {
		embedding[i] = byte(i % 256)
	}

	// Insert vector with embedding
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO knowledge_vectors (id, content, embedding, metadata)
		VALUES (?, ?, ?, ?)`,
		"vec1", "test content", embedding, `{"section":"intro"}`)
	if err != nil {
		t.Fatalf("failed to insert knowledge vector: %v", err)
	}

	// Retrieve and verify
	var retrievedEmbedding []byte
	var content string
	err = db.conn.QueryRowContext(ctx,
		"SELECT content, embedding FROM knowledge_vectors WHERE id = 'vec1'").
		Scan(&content, &retrievedEmbedding)
	if err != nil {
		t.Fatalf("failed to query knowledge vector: %v", err)
	}

	if content != "test content" {
		t.Errorf("expected content 'test content', got %s", content)
	}

	if len(retrievedEmbedding) != len(embedding) {
		t.Errorf("expected embedding length %d, got %d", len(embedding), len(retrievedEmbedding))
	}

	// Verify embedding data matches
	for i := range embedding {
		if retrievedEmbedding[i] != embedding[i] {
			t.Errorf("embedding mismatch at index %d: expected %d, got %d", i, embedding[i], retrievedEmbedding[i])
			break
		}
	}
}
