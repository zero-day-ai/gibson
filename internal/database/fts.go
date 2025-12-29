package database

import (
	"context"
	"database/sql"
	"fmt"
)

// SetupFTS creates the FTS5 virtual table for full-text search on findings
// This enables fast text search across finding titles, descriptions, and remediation steps
func SetupFTS(db *DB) error {
	ctx := context.Background()

	// Create FTS5 virtual table
	// We use content=findings to sync with the findings table
	// This creates a shadow table that mirrors findings content
	createFTSQuery := `
	CREATE VIRTUAL TABLE IF NOT EXISTS findings_fts USING fts5(
		title,
		description,
		remediation,
		content=findings,
		content_rowid=rowid
	)`

	if _, err := db.conn.ExecContext(ctx, createFTSQuery); err != nil {
		return fmt.Errorf("failed to create FTS table: %w", err)
	}

	// Create triggers to keep FTS index in sync with findings table
	if err := createFTSTriggers(db); err != nil {
		return fmt.Errorf("failed to create FTS triggers: %w", err)
	}

	return nil
}

// createFTSTriggers creates triggers to automatically update the FTS index
// when findings are inserted, updated, or deleted
func createFTSTriggers(db *DB) error {
	ctx := context.Background()

	// Trigger for INSERT operations
	insertTrigger := `
	CREATE TRIGGER IF NOT EXISTS findings_fts_insert
	AFTER INSERT ON findings
	BEGIN
		INSERT INTO findings_fts(rowid, title, description, remediation)
		VALUES (new.rowid, new.title, new.description, new.remediation);
	END`

	if _, err := db.conn.ExecContext(ctx, insertTrigger); err != nil {
		return fmt.Errorf("failed to create insert trigger: %w", err)
	}

	// Trigger for UPDATE operations
	updateTrigger := `
	CREATE TRIGGER IF NOT EXISTS findings_fts_update
	AFTER UPDATE ON findings
	BEGIN
		INSERT INTO findings_fts(findings_fts, rowid, title, description, remediation)
		VALUES('delete', old.rowid, old.title, old.description, old.remediation);
		INSERT INTO findings_fts(rowid, title, description, remediation)
		VALUES (new.rowid, new.title, new.description, new.remediation);
	END`

	if _, err := db.conn.ExecContext(ctx, updateTrigger); err != nil {
		return fmt.Errorf("failed to create update trigger: %w", err)
	}

	// Trigger for DELETE operations
	deleteTrigger := `
	CREATE TRIGGER IF NOT EXISTS findings_fts_delete
	AFTER DELETE ON findings
	BEGIN
		INSERT INTO findings_fts(findings_fts, rowid, title, description, remediation)
		VALUES('delete', old.rowid, old.title, old.description, old.remediation);
	END`

	if _, err := db.conn.ExecContext(ctx, deleteTrigger); err != nil {
		return fmt.Errorf("failed to create delete trigger: %w", err)
	}

	return nil
}

// RebuildFTSIndex completely rebuilds the FTS index from the findings table
// This is useful when the FTS index gets out of sync or corrupted
func RebuildFTSIndex(db *DB) error {
	ctx := context.Background()

	return db.WithTx(ctx, func(tx *sql.Tx) error {
		// Clear existing FTS data
		if _, err := tx.ExecContext(ctx, "DELETE FROM findings_fts"); err != nil {
			return fmt.Errorf("failed to clear FTS index: %w", err)
		}

		// Repopulate FTS index from findings table
		query := `
		INSERT INTO findings_fts(rowid, title, description, remediation)
		SELECT rowid, title, description, remediation FROM findings`

		if _, err := tx.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to rebuild FTS index: %w", err)
		}

		// Optimize the FTS index
		if _, err := tx.ExecContext(ctx, "INSERT INTO findings_fts(findings_fts) VALUES('optimize')"); err != nil {
			return fmt.Errorf("failed to optimize FTS index: %w", err)
		}

		return nil
	})
}

// SearchFindings performs a full-text search on findings
// Returns finding IDs that match the search query
func SearchFindings(db *DB, ctx context.Context, searchQuery string) ([]string, error) {
	// FTS5 query using the MATCH operator
	query := `
	SELECT f.id
	FROM findings f
	JOIN findings_fts fts ON f.rowid = fts.rowid
	WHERE findings_fts MATCH ?
	ORDER BY rank`

	rows, err := db.conn.QueryContext(ctx, query, searchQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute FTS query: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating results: %w", err)
	}

	return ids, nil
}

// SearchFindingsWithSnippets performs a full-text search and returns snippets
// Snippets show the matching text with context
func SearchFindingsWithSnippets(db *DB, ctx context.Context, searchQuery string) ([]FindingSearchResult, error) {
	// FTS5 query with snippet generation
	// snippet() extracts matching text with surrounding context
	query := `
	SELECT
		f.id,
		f.title,
		f.severity,
		snippet(findings_fts, 0, '<mark>', '</mark>', '...', 32) as title_snippet,
		snippet(findings_fts, 1, '<mark>', '</mark>', '...', 64) as desc_snippet,
		rank
	FROM findings f
	JOIN findings_fts fts ON f.rowid = fts.rowid
	WHERE findings_fts MATCH ?
	ORDER BY rank
	LIMIT 100`

	rows, err := db.conn.QueryContext(ctx, query, searchQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute FTS query: %w", err)
	}
	defer rows.Close()

	var results []FindingSearchResult
	for rows.Next() {
		var result FindingSearchResult
		if err := rows.Scan(
			&result.ID,
			&result.Title,
			&result.Severity,
			&result.TitleSnippet,
			&result.DescriptionSnippet,
			&result.Rank,
		); err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating results: %w", err)
	}

	return results, nil
}

// FindingSearchResult represents a search result with snippets
type FindingSearchResult struct {
	ID                 string
	Title              string
	Severity           string
	TitleSnippet       string
	DescriptionSnippet string
	Rank               float64
}

// OptimizeFTS optimizes the FTS index for better performance
// This should be called periodically, especially after bulk operations
func OptimizeFTS(db *DB) error {
	ctx := context.Background()
	_, err := db.conn.ExecContext(ctx, "INSERT INTO findings_fts(findings_fts) VALUES('optimize')")
	if err != nil {
		return fmt.Errorf("failed to optimize FTS index: %w", err)
	}
	return nil
}

// DropFTS removes the FTS virtual table and all associated triggers
// This is useful for cleanup or before recreating the FTS setup
func DropFTS(db *DB) error {
	ctx := context.Background()

	return db.WithTx(ctx, func(tx *sql.Tx) error {
		// Drop triggers first
		triggers := []string{
			"findings_fts_insert",
			"findings_fts_update",
			"findings_fts_delete",
		}

		for _, trigger := range triggers {
			query := fmt.Sprintf("DROP TRIGGER IF EXISTS %s", trigger)
			if _, err := tx.ExecContext(ctx, query); err != nil {
				return fmt.Errorf("failed to drop trigger %s: %w", trigger, err)
			}
		}

		// Drop FTS virtual table
		if _, err := tx.ExecContext(ctx, "DROP TABLE IF EXISTS findings_fts"); err != nil {
			return fmt.Errorf("failed to drop FTS table: %w", err)
		}

		return nil
	})
}
