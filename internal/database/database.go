package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	// Import sqlite3 driver with FTS5 support
	// Build with: go build -tags "fts5"
	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite database connection with additional functionality
type DB struct {
	conn *sql.DB
	path string
}

// Config holds database configuration options
type Config struct {
	Path            string        // Database file path
	MaxOpenConns    int           // Maximum number of open connections
	MaxIdleConns    int           // Maximum number of idle connections
	ConnMaxLifetime time.Duration // Maximum connection lifetime
	BusyTimeout     time.Duration // SQLite busy timeout
}

// DefaultConfig returns sensible defaults for database configuration
func DefaultConfig(path string) Config {
	return Config{
		Path:            path,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		BusyTimeout:     5 * time.Second,
	}
}

// Open creates a new database connection with optimized settings
// Enables WAL mode, foreign keys, and sets busy timeout for better concurrency
func Open(path string) (*DB, error) {
	return OpenWithConfig(DefaultConfig(path))
}

// OpenWithConfig creates a new database connection with custom configuration
func OpenWithConfig(cfg Config) (*DB, error) {
	// Build DSN with pragmas
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=%d",
		cfg.Path,
		int(cfg.BusyTimeout.Milliseconds()),
	)

	// Open database connection
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	conn.SetMaxOpenConns(cfg.MaxOpenConns)
	conn.SetMaxIdleConns(cfg.MaxIdleConns)
	conn.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{
		conn: conn,
		path: cfg.Path,
	}

	// Verify WAL mode is enabled
	var journalMode string
	if err := db.conn.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to verify journal mode: %w", err)
	}
	if journalMode != "wal" {
		conn.Close()
		return nil, fmt.Errorf("WAL mode not enabled (got %s)", journalMode)
	}

	// Verify foreign keys are enabled
	var foreignKeys int
	if err := db.conn.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to verify foreign keys: %w", err)
	}
	if foreignKeys != 1 {
		conn.Close()
		return nil, fmt.Errorf("foreign keys not enabled")
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.conn == nil {
		return nil
	}
	return db.conn.Close()
}

// Conn returns the underlying sql.DB connection
// Use with caution - prefer using the DB methods for safety
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// Path returns the database file path
func (db *DB) Path() string {
	return db.path
}

// Health performs a health check on the database connection
func (db *DB) Health(ctx context.Context) error {
	// Check if connection is alive
	if err := db.conn.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Verify we can query
	var result int
	if err := db.conn.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected query result: %d", result)
	}

	return nil
}

// WithTx executes a function within a transaction
// If the function returns an error, the transaction is rolled back
// Otherwise, the transaction is committed
func (db *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure rollback on panic
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p) // Re-throw panic after rollback
		}
	}()

	// Execute function
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("transaction error: %w, rollback error: %v", err, rbErr)
		}
		return err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Stats returns database statistics
type Stats struct {
	OpenConnections int
	InUse           int
	Idle            int
	WaitCount       int64
	WaitDuration    time.Duration
	MaxIdleClosed   int64
	MaxLifetime     int64
}

// Stats returns database connection pool statistics
func (db *DB) Stats() Stats {
	s := db.conn.Stats()
	return Stats{
		OpenConnections: s.OpenConnections,
		InUse:           s.InUse,
		Idle:            s.Idle,
		WaitCount:       s.WaitCount,
		WaitDuration:    s.WaitDuration,
		MaxIdleClosed:   s.MaxIdleClosed,
		MaxLifetime:     s.MaxLifetimeClosed,
	}
}

// Vacuum optimizes the database file
// This reclaims unused space and defragments the database
func (db *DB) Vacuum(ctx context.Context) error {
	_, err := db.conn.ExecContext(ctx, "VACUUM")
	if err != nil {
		return fmt.Errorf("vacuum failed: %w", err)
	}
	return nil
}

// Checkpoint performs a WAL checkpoint
// This moves data from the WAL file to the main database file
func (db *DB) Checkpoint(ctx context.Context) error {
	_, err := db.conn.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	if err != nil {
		return fmt.Errorf("checkpoint failed: %w", err)
	}
	return nil
}

// InitSchema initializes the database schema using migrations
func (db *DB) InitSchema() error {
	migrator := NewMigrator(db)
	ctx := context.Background()

	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// QueryContext wraps the underlying connection's QueryContext
func (db *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return db.conn.QueryContext(ctx, query, args...)
}

// QueryRowContext wraps the underlying connection's QueryRowContext
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.conn.QueryRowContext(ctx, query, args...)
}

// ExecContext wraps the underlying connection's ExecContext
func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.conn.ExecContext(ctx, query, args...)
}

// QueryRow wraps the underlying connection's QueryRow for convenience
// For context-aware queries, use QueryRowContext instead
func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.conn.QueryRow(query, args...)
}

// Query wraps the underlying connection's Query for convenience
// For context-aware queries, use QueryContext instead
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

// Exec wraps the underlying connection's Exec for convenience
// For context-aware execution, use ExecContext instead
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.conn.Exec(query, args...)
}
