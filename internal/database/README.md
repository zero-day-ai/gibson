# Database Package

SQLite3-based database layer for the Gibson Framework with encryption support, migrations, and full-text search.

## Features

- **WAL Mode**: Write-Ahead Logging for better concurrency
- **Foreign Keys**: Enforced referential integrity
- **Migrations**: Version-controlled schema changes
- **FTS5**: Full-text search on security findings
- **Encryption**: AES-256-GCM for credentials storage
- **Connection Pooling**: Configurable connection management

## Usage

### Basic Setup

```go
import "github.com/zero-day-ai/gibson/internal/database"

// Open database with defaults
db, err := database.Open("/path/to/gibson.db")
if err != nil {
    log.Fatal(err)
}
defer db.Close()

// Run migrations
err = db.InitSchema()
if err != nil {
    log.Fatal(err)
}

// Setup full-text search (optional)
err = database.SetupFTS(db)
if err != nil {
    log.Fatal(err)
}
```

### Custom Configuration

```go
cfg := database.Config{
    Path:            "/path/to/gibson.db",
    MaxOpenConns:    10,
    MaxIdleConns:    5,
    ConnMaxLifetime: time.Hour,
    BusyTimeout:     5 * time.Second,
}

db, err := database.OpenWithConfig(cfg)
```

### Transactions

```go
err := db.WithTx(ctx, func(tx *sql.Tx) error {
    _, err := tx.Exec("INSERT INTO credentials ...")
    return err
})
```

### Migrations

```go
migrator := database.NewMigrator(db)

// Apply all pending migrations
err := migrator.Migrate(ctx)

// Get current version
version, err := migrator.CurrentVersion(ctx)

// Rollback to specific version
err := migrator.Rollback(ctx, targetVersion)

// Get migration history
migrations, err := migrator.GetAppliedMigrations(ctx)
```

### Full-Text Search

```go
// Search findings
ids, err := database.SearchFindings(db, ctx, "SQL injection")

// Search with snippets
results, err := database.SearchFindingsWithSnippets(db, ctx, "XSS")
for _, result := range results {
    fmt.Printf("%s: %s\n", result.Title, result.DescriptionSnippet)
}

// Rebuild FTS index
err := database.RebuildFTSIndex(db)

// Optimize FTS index
err := database.OptimizeFTS(db)
```

## Schema

### Tables

- **credentials**: Encrypted API keys and tokens
- **targets**: LLM endpoints to test
- **findings**: Security findings from tests
- **migrations**: Schema version tracking

### Full-Text Search

- **findings_fts**: FTS5 virtual table for finding search
- Automatically synced via triggers on INSERT/UPDATE/DELETE

## Building

The database package requires CGO and FTS5 support:

```bash
CGO_ENABLED=1 go build -tags "fts5" ./...
```

## Testing

Run tests with:

```bash
CGO_ENABLED=1 go test -tags="fts5" -v ./internal/database/...
```

With coverage:

```bash
CGO_ENABLED=1 go test -tags="fts5" -coverprofile=coverage.out ./internal/database/...
go tool cover -html=coverage.out
```

## Database File

The database file is a standard SQLite3 database with:
- WAL journaling mode for concurrency
- Foreign key enforcement enabled
- Busy timeout of 5 seconds (configurable)

## Performance

- Connection pooling: 10 max open, 5 max idle (configurable)
- WAL checkpointing: Use `db.Checkpoint(ctx)` periodically
- Vacuum: Use `db.Vacuum(ctx)` to reclaim space

## Notes

- The database connection is thread-safe
- Use `db.WithTx()` for transactional operations
- FTS5 requires SQLite compiled with FTS5 support
- Credentials are stored encrypted; decryption handled by crypto package
