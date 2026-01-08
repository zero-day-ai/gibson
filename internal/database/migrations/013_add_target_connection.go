package migrations

import (
	"context"
	"database/sql"
)

func init() {
	registerMigration(13, "add_target_connection", migrateAddTargetConnection)
}

// migrateAddTargetConnection adds the connection column to targets table
// for schema-based connection parameters (CIDR, URLs, etc.)
func migrateAddTargetConnection(ctx context.Context, tx *sql.Tx) error {
	// Add connection column to targets table
	_, err := tx.ExecContext(ctx, `
		ALTER TABLE targets ADD COLUMN connection TEXT DEFAULT '{}'
	`)
	if err != nil {
		return err
	}

	return nil
}
