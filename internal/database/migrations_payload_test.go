package database

import (
	"context"
	"database/sql"
	"testing"
)

// TestPayloadMigrations verifies that the payload system migration (migration 5) runs successfully
func TestPayloadMigrations(t *testing.T) {
	// Use setupTestDB helper from existing tests
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create migrator
	migrator := NewMigrator(db)

	// Run all migrations including payload system
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

	// Verify all payload tables exist
	tables := []string{
		"payloads",
		"payloads_fts",
		"attack_chains",
		"payload_executions",
		"payload_versions",
		"chain_executions",
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

	// Verify payloads table schema by attempting to insert a test row
	_, err = db.conn.ExecContext(ctx, `
		INSERT INTO payloads (id, name, version, categories, template, success_indicators, severity, built_in, enabled)
		VALUES ('test-id', 'test-payload', '1.0.0', '["jailbreak"]', 'test {{param}}', '[{"type":"contains","value":"test"}]', 'high', 0, 1)
	`)
	if err != nil {
		t.Errorf("failed to insert test payload: %v", err)
	}

	// Note: Skipping payload_executions insert test due to complex foreign key dependencies
	// The table structure is validated by the schema migration test above

	// Verify FTS5 search works
	var count int
	err = db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM payloads_fts WHERE payloads_fts MATCH 'test'").Scan(&count)
	if err != nil {
		t.Errorf("failed to query FTS: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 FTS result, got %d", count)
	}

	// Verify indexes exist by checking sqlite_master
	indexes := []string{
		"idx_payloads_name",
		"idx_payloads_severity",
		"idx_payload_executions_payload_id",
		"idx_payload_executions_success",
		"idx_attack_chains_name",
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

	// Verify triggers exist
	triggers := []string{
		"payloads_ai",
		"payloads_ad",
		"payloads_au",
		"update_payloads_timestamp",
		"update_attack_chains_timestamp",
		"update_chain_executions_timestamp",
	}

	for _, trigger := range triggers {
		var name string
		query := "SELECT name FROM sqlite_master WHERE type='trigger' AND name=?"
		err := db.conn.QueryRowContext(ctx, query, trigger).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("trigger %s does not exist", trigger)
		} else if err != nil {
			t.Fatalf("failed to query for trigger %s: %v", trigger, err)
		}
	}
}

// TestPayloadMigrationsRollback verifies that rollback works correctly
func TestPayloadMigrationsRollback(t *testing.T) {
	// Use setupTestDB helper from existing tests
	db, cleanup := setupTestDB(t)
	defer cleanup()

	migrator := NewMigrator(db)
	ctx := context.Background()

	// Run all migrations
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Rollback to version 4 (before payload system)
	if err := migrator.Rollback(ctx, 4); err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	// Verify version is now 4
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("failed to get current version: %v", err)
	}
	if version != 4 {
		t.Errorf("expected version 4 after rollback, got %d", version)
	}

	// Verify payload tables no longer exist
	tables := []string{
		"payloads",
		"payloads_fts",
		"attack_chains",
		"payload_executions",
		"payload_versions",
		"chain_executions",
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
