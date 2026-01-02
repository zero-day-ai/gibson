package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) (*DB, func()) {
	t.Helper()

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "gibson-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")

	// Open database
	db, err := Open(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open database: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

// TestOpen tests database opening with WAL mode verification
func TestOpen(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	if db == nil {
		t.Fatal("expected non-nil database")
	}

	// Verify WAL mode is enabled
	var journalMode string
	err := db.conn.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("failed to query journal mode: %v", err)
	}

	if journalMode != "wal" {
		t.Errorf("expected WAL mode, got %s", journalMode)
	}

	// Verify foreign keys are enabled
	var foreignKeys int
	err = db.conn.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys)
	if err != nil {
		t.Fatalf("failed to query foreign keys: %v", err)
	}

	if foreignKeys != 1 {
		t.Errorf("expected foreign keys enabled, got %d", foreignKeys)
	}
}

// TestOpenWithConfig tests database opening with custom configuration
func TestOpenWithConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gibson-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := Config{
		Path:            dbPath,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: 30 * time.Minute,
		BusyTimeout:     3 * time.Second,
	}

	db, err := OpenWithConfig(cfg)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Verify configuration was applied
	stats := db.Stats()
	if stats.OpenConnections < 0 {
		t.Error("expected valid connection count")
	}
}

// TestClose tests database closing
func TestClose(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Close database
	err := db.Close()
	if err != nil {
		t.Fatalf("failed to close database: %v", err)
	}

	// Verify connection is closed by attempting a query
	err = db.conn.Ping()
	if err == nil {
		t.Error("expected error pinging closed database")
	}
}

// TestHealth tests database health check
func TestHealth(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Test health on open database
	err := db.Health(ctx)
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	// Test health with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = db.Health(ctx)
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

// TestWithTx tests transaction commit
func TestWithTx(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations first
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()

	// Test successful transaction
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO credentials (id, name, type, encrypted_value, encryption_iv, key_derivation_salt)
			VALUES (?, ?, ?, ?, ?, ?)`,
			"test-1", "test-cred", "api_key", []byte("encrypted"), []byte("iv"), []byte("salt"))
		return err
	})
	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}

	// Verify data was committed
	var count int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM credentials WHERE id = ?", "test-1").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query credentials: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 credential, got %d", count)
	}
}

// TestWithTxRollback tests transaction rollback
func TestWithTxRollback(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations first
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()

	// Test transaction that should rollback
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO credentials (id, name, type, encrypted_value, encryption_iv, key_derivation_salt)
			VALUES (?, ?, ?, ?, ?, ?)`,
			"test-2", "test-cred-2", "api_key", []byte("encrypted"), []byte("iv"), []byte("salt"))
		if err != nil {
			return err
		}
		// Return error to trigger rollback
		return sql.ErrTxDone
	})
	if err == nil {
		t.Fatal("expected transaction to fail")
	}

	// Verify data was NOT committed
	var count int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM credentials WHERE id = ?", "test-2").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query credentials: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 credentials (rolled back), got %d", count)
	}
}

// TestMigrate tests migration application
func TestMigrate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	migrator := NewMigrator(db)

	// Check initial version (should be 0)
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("failed to get version: %v", err)
	}
	if version != 0 {
		t.Errorf("expected version 0, got %d", version)
	}

	// Apply migrations
	err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Check version after migration
	version, err = migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("failed to get version: %v", err)
	}
	if version != 10 {
		t.Errorf("expected version 10, got %d", version)
	}

	// Verify tables were created
	tables := []string{"credentials", "targets", "findings", "migrations", "mission_memory"}
	for _, table := range tables {
		var count int
		query := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`
		err := db.conn.QueryRow(query, table).Scan(&count)
		if err != nil {
			t.Fatalf("failed to check table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("expected table %s to exist", table)
		}
	}
}

// TestMigrateIdempotent tests that migrations can be run multiple times
func TestMigrateIdempotent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	migrator := NewMigrator(db)

	// Apply migrations twice
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("first migrate failed: %v", err)
	}

	err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("second migrate failed: %v", err)
	}

	// Version should still be 11 (we have 11 migrations now)
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("failed to get version: %v", err)
	}
	if version != 10 {
		t.Errorf("expected version 10, got %d", version)
	}
}

// TestRollback tests migration rollback
func TestRollback(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	migrator := NewMigrator(db)

	// Apply migrations
	err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Rollback to version 0
	err = migrator.Rollback(ctx, 0)
	if err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	// Check version
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("failed to get version: %v", err)
	}
	if version != 0 {
		t.Errorf("expected version 0, got %d", version)
	}

	// Verify tables were dropped
	var count int
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='credentials'`
	err = db.conn.QueryRow(query).Scan(&count)
	if err != nil {
		t.Fatalf("failed to check credentials table: %v", err)
	}
	if count != 0 {
		t.Error("expected credentials table to be dropped")
	}
}

// TestFTSSetup tests FTS5 table creation
func TestFTSSetup(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations first
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Setup FTS
	err := SetupFTS(db)
	if err != nil {
		t.Fatalf("failed to setup FTS: %v", err)
	}

	// Verify FTS table exists
	var count int
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='findings_fts'`
	err = db.conn.QueryRow(query).Scan(&count)
	if err != nil {
		t.Fatalf("failed to check FTS table: %v", err)
	}
	if count != 1 {
		t.Error("expected findings_fts table to exist")
	}

	// Verify triggers exist
	triggers := []string{
		"findings_fts_insert",
		"findings_fts_update",
		"findings_fts_delete",
	}
	for _, trigger := range triggers {
		var tCount int
		query := `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name=?`
		err := db.conn.QueryRow(query, trigger).Scan(&tCount)
		if err != nil {
			t.Fatalf("failed to check trigger %s: %v", trigger, err)
		}
		if tCount != 1 {
			t.Errorf("expected trigger %s to exist", trigger)
		}
	}
}

// TestFTSSearch tests full-text search functionality
func TestFTSSearch(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations and setup FTS
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	if err := SetupFTS(db); err != nil {
		t.Fatalf("failed to setup FTS: %v", err)
	}

	ctx := context.Background()

	// Insert test findings
	findings := []struct {
		id          string
		title       string
		description string
		remediation string
		severity    string
	}{
		{
			"f1",
			"SQL Injection Vulnerability",
			"User input not sanitized in SQL queries",
			"Use parameterized queries",
			"critical",
		},
		{
			"f2",
			"Cross-Site Scripting (XSS)",
			"User input displayed without escaping",
			"Sanitize and escape all user input",
			"high",
		},
		{
			"f3",
			"Insecure Authentication",
			"Weak password policy allows brute force",
			"Implement strong password requirements",
			"high",
		},
	}

	for _, f := range findings {
		_, err := db.conn.Exec(`
			INSERT INTO findings (id, title, description, remediation, severity)
			VALUES (?, ?, ?, ?, ?)`,
			f.id, f.title, f.description, f.remediation, f.severity)
		if err != nil {
			t.Fatalf("failed to insert finding: %v", err)
		}
	}

	// Test search for "SQL"
	results, err := SearchFindings(db, ctx, "SQL")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'SQL', got %d", len(results))
	}
	if len(results) > 0 && results[0] != "f1" {
		t.Errorf("expected f1, got %s", results[0])
	}

	// Test search for "input"
	results, err = SearchFindings(db, ctx, "input")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'input', got %d", len(results))
	}

	// Test search with snippets
	snippets, err := SearchFindingsWithSnippets(db, ctx, "password")
	if err != nil {
		t.Fatalf("snippet search failed: %v", err)
	}
	if len(snippets) != 1 {
		t.Errorf("expected 1 result for 'password', got %d", len(snippets))
	}
}

// TestRebuildFTSIndex tests FTS index rebuilding
func TestRebuildFTSIndex(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations and setup FTS
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	if err := SetupFTS(db); err != nil {
		t.Fatalf("failed to setup FTS: %v", err)
	}

	// Insert test finding
	_, err := db.conn.Exec(`
		INSERT INTO findings (id, title, description, remediation, severity)
		VALUES (?, ?, ?, ?, ?)`,
		"f1", "Test Finding", "Test description", "Test remediation", "low")
	if err != nil {
		t.Fatalf("failed to insert finding: %v", err)
	}

	// Rebuild index
	err = RebuildFTSIndex(db)
	if err != nil {
		t.Fatalf("failed to rebuild FTS index: %v", err)
	}

	// Verify search still works
	results, err := SearchFindings(db, context.Background(), "Test")
	if err != nil {
		t.Fatalf("search failed after rebuild: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

// TestContextCancellation tests context cancellation
func TestContextCancellation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Try to execute query with cancelled context
	_, err := db.conn.QueryContext(ctx, "SELECT * FROM credentials")
	if err == nil {
		t.Error("expected error with cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestVacuum tests database vacuum operation
func TestVacuum(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Vacuum(ctx)
	if err != nil {
		t.Fatalf("vacuum failed: %v", err)
	}
}

// TestCheckpoint tests WAL checkpoint operation
func TestCheckpoint(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Checkpoint(ctx)
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
}

// TestStats tests database statistics
func TestStats(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	stats := db.Stats()

	if stats.OpenConnections < 0 {
		t.Error("expected valid open connections count")
	}
	if stats.InUse < 0 {
		t.Error("expected valid in-use count")
	}
	if stats.Idle < 0 {
		t.Error("expected valid idle count")
	}
}

// TestPath tests Path() method
func TestPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gibson-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if db.Path() != dbPath {
		t.Errorf("expected path %s, got %s", dbPath, db.Path())
	}
}

// TestConn tests Conn() method
func TestConn(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	conn := db.Conn()
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}

	// Verify connection works
	err := conn.Ping()
	if err != nil {
		t.Fatalf("connection ping failed: %v", err)
	}
}

// TestInitSchema tests InitSchema() method
func TestInitSchema(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := db.InitSchema()
	if err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Verify tables exist
	var count int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='credentials'").Scan(&count)
	if err != nil {
		t.Fatalf("failed to check credentials table: %v", err)
	}
	if count != 1 {
		t.Error("expected credentials table to exist")
	}
}

// TestOptimizeFTS tests FTS optimization
func TestOptimizeFTS(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	if err := SetupFTS(db); err != nil {
		t.Fatalf("failed to setup FTS: %v", err)
	}

	err := OptimizeFTS(db)
	if err != nil {
		t.Fatalf("OptimizeFTS failed: %v", err)
	}
}

// TestDropFTS tests FTS cleanup
func TestDropFTS(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	if err := SetupFTS(db); err != nil {
		t.Fatalf("failed to setup FTS: %v", err)
	}

	// Verify FTS table exists
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='findings_fts'").Scan(&count)
	if err != nil {
		t.Fatalf("failed to check FTS table: %v", err)
	}
	if count != 1 {
		t.Error("expected findings_fts table to exist")
	}

	// Drop FTS
	err = DropFTS(db)
	if err != nil {
		t.Fatalf("DropFTS failed: %v", err)
	}

	// Verify FTS table is gone
	err = db.conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='findings_fts'").Scan(&count)
	if err != nil {
		t.Fatalf("failed to check FTS table: %v", err)
	}
	if count != 0 {
		t.Error("expected findings_fts table to be dropped")
	}
}

// TestGetAppliedMigrations tests migration history retrieval
func TestGetAppliedMigrations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	migrator := NewMigrator(db)
	ctx := context.Background()

	// Apply migrations
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Get applied migrations
	migrations, err := migrator.GetAppliedMigrations(ctx)
	if err != nil {
		t.Fatalf("failed to get applied migrations: %v", err)
	}

	if len(migrations) != 10 {
		t.Errorf("expected 10 applied migrations, got %d", len(migrations))
	}

	if len(migrations) >= 10 {
		// Check first migration
		if migrations[0].Version != 1 {
			t.Errorf("expected version 1, got %d", migrations[0].Version)
		}
		if migrations[0].Name != "initial_schema" {
			t.Errorf("expected name 'initial_schema', got %s", migrations[0].Name)
		}
		// Check last migration (index 9 since we now have 10 migrations, 0-indexed)
		if migrations[9].Version != 10 {
			t.Errorf("expected version 10, got %d", migrations[9].Version)
		}
		if migrations[9].Name != "mission_consolidation_columns" {
			t.Errorf("expected name 'mission_consolidation_columns', got %s", migrations[9].Name)
		}
	}
}

// TestWithTxPanic tests transaction rollback on panic
func TestWithTxPanic(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()

	// Test panic handling
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic to be re-thrown")
		}
	}()

	db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO credentials (id, name, type, encrypted_value, encryption_iv, key_derivation_salt)
			VALUES (?, ?, ?, ?, ?, ?)`,
			"test-panic", "test", "api_key", []byte("encrypted"), []byte("iv"), []byte("salt"))
		if err != nil {
			return err
		}
		panic("test panic")
	})
}

// TestDefaultConfig tests default configuration
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("/tmp/test.db")

	if cfg.Path != "/tmp/test.db" {
		t.Errorf("expected path /tmp/test.db, got %s", cfg.Path)
	}
	if cfg.MaxOpenConns != 10 {
		t.Errorf("expected MaxOpenConns 10, got %d", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 5 {
		t.Errorf("expected MaxIdleConns 5, got %d", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != time.Hour {
		t.Errorf("expected ConnMaxLifetime 1h, got %v", cfg.ConnMaxLifetime)
	}
	if cfg.BusyTimeout != 5*time.Second {
		t.Errorf("expected BusyTimeout 5s, got %v", cfg.BusyTimeout)
	}
}

// TestRollbackInvalidVersion tests rollback with invalid version
func TestRollbackInvalidVersion(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	migrator := NewMigrator(db)

	// Test rollback to negative version
	err := migrator.Rollback(ctx, -1)
	if err == nil {
		t.Error("expected error for negative target version")
	}

	// Apply migrations
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Test rollback to future version
	err = migrator.Rollback(ctx, 999)
	if err == nil {
		t.Error("expected error for future target version")
	}
}

// TestOpenErrors tests error handling in Open
func TestOpenErrors(t *testing.T) {
	// Test with invalid path (directory doesn't exist)
	_, err := Open("/nonexistent/path/db.sqlite")
	if err == nil {
		t.Error("expected error opening database in nonexistent directory")
	}
}

// TestOpenWithConfigErrors tests error handling in OpenWithConfig
func TestOpenWithConfigErrors(t *testing.T) {
	// Test with invalid DSN
	cfg := Config{
		Path:            "/\x00invalid\x00path",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		BusyTimeout:     5 * time.Second,
	}

	_, err := OpenWithConfig(cfg)
	if err == nil {
		t.Error("expected error with invalid path")
	}
}

// TestCloseNilConnection tests closing nil connection
func TestCloseNilConnection(t *testing.T) {
	db := &DB{conn: nil}
	err := db.Close()
	if err != nil {
		t.Errorf("expected no error closing nil connection, got %v", err)
	}
}

// TestHealthErrors tests health check error paths
func TestHealthErrors(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Close the connection
	db.Close()

	// Health check should fail on closed connection
	ctx := context.Background()
	err := db.Health(ctx)
	if err == nil {
		t.Error("expected health check to fail on closed connection")
	}
}

// TestVacuumError tests vacuum on closed connection
func TestVacuumError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	cleanup() // Close immediately

	ctx := context.Background()
	err := db.Vacuum(ctx)
	if err == nil {
		t.Error("expected vacuum to fail on closed connection")
	}
}

// TestCheckpointError tests checkpoint on closed connection
func TestCheckpointError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	cleanup() // Close immediately

	ctx := context.Background()
	err := db.Checkpoint(ctx)
	if err == nil {
		t.Error("expected checkpoint to fail on closed connection")
	}
}

// TestSearchFindingsErrors tests search error handling
func TestSearchFindingsErrors(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Search without FTS setup should fail
	_, err := SearchFindings(db, ctx, "test")
	if err == nil {
		t.Error("expected search to fail without FTS setup")
	}
}

// TestSearchFindingsWithSnippetsErrors tests snippet search error handling
func TestSearchFindingsWithSnippetsErrors(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Search without FTS setup should fail
	_, err := SearchFindingsWithSnippets(db, ctx, "test")
	if err == nil {
		t.Error("expected snippet search to fail without FTS setup")
	}
}

// TestRebuildFTSIndexError tests rebuild error handling
func TestRebuildFTSIndexError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Rebuild without FTS setup should fail
	err := RebuildFTSIndex(db)
	if err == nil {
		t.Error("expected rebuild to fail without FTS setup")
	}
}

// TestInitSchemaError tests InitSchema error handling
func TestInitSchemaError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	cleanup() // Close immediately

	err := db.InitSchema()
	if err == nil {
		t.Error("expected InitSchema to fail on closed connection")
	}
}

// TestSetupFTSErrors tests FTS setup error paths
func TestSetupFTSErrors(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Setup FTS without migrating first - should fail (no findings table)
	err := SetupFTS(db)
	if err == nil {
		t.Error("expected SetupFTS to fail without findings table")
	}
}

// TestCreateFTSTriggersErrors tests FTS trigger creation errors
func TestCreateFTSTriggersErrors(t *testing.T) {
	db, cleanup := setupTestDB(t)
	cleanup() // Close immediately

	// This will fail because connection is closed
	err := createFTSTriggers(db)
	if err == nil {
		t.Error("expected createFTSTriggers to fail on closed connection")
	}
}

// TestWithTxBeginError tests transaction begin error
func TestWithTxBeginError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	cleanup() // Close immediately

	ctx := context.Background()
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return nil
	})
	if err == nil {
		t.Error("expected WithTx to fail on closed connection")
	}
}

// TestMigrateApplyError tests migration apply error handling
func TestMigrateApplyError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	migrator := NewMigrator(db)

	// Apply migrations
	ctx := context.Background()
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Close the connection and try to migrate again
	// This tests error paths in migration application
	db.Close()

	err := migrator.Migrate(ctx)
	if err == nil {
		t.Error("expected migrate to fail on closed connection")
	}
}

// TestCurrentVersionError tests CurrentVersion error handling
func TestCurrentVersionError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	cleanup() // Close immediately

	ctx := context.Background()
	migrator := NewMigrator(db)

	_, err := migrator.CurrentVersion(ctx)
	if err == nil {
		t.Error("expected CurrentVersion to fail on closed connection")
	}
}

// TestGetAppliedMigrationsError tests GetAppliedMigrations error handling
func TestGetAppliedMigrationsError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	cleanup() // Close immediately

	ctx := context.Background()
	migrator := NewMigrator(db)

	_, err := migrator.GetAppliedMigrations(ctx)
	if err == nil {
		t.Error("expected GetAppliedMigrations to fail on closed connection")
	}
}

// TestOptimizeFTSError tests OptimizeFTS error handling
func TestOptimizeFTSError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	cleanup() // Close immediately

	err := OptimizeFTS(db)
	if err == nil {
		t.Error("expected OptimizeFTS to fail on closed connection")
	}
}

// TestDropFTSError tests DropFTS error handling
func TestDropFTSError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	cleanup() // Close immediately

	err := DropFTS(db)
	if err == nil {
		t.Error("expected DropFTS to fail on closed connection")
	}
}
