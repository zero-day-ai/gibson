package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// CredentialDAO provides database access for Credential entities
type CredentialDAO struct {
	db *DB
}

// NewCredentialDAO creates a new CredentialDAO instance
func NewCredentialDAO(db *DB) *CredentialDAO {
	return &CredentialDAO{db: db}
}

// Create inserts a new credential into the database
func (dao *CredentialDAO) Create(ctx context.Context, cred *types.Credential) error {
	if err := cred.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Marshal JSON fields
	tagsJSON, err := json.Marshal(cred.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	rotationJSON, err := json.Marshal(cred.Rotation)
	if err != nil {
		return fmt.Errorf("failed to marshal rotation: %w", err)
	}

	usageJSON, err := json.Marshal(cred.Usage)
	if err != nil {
		return fmt.Errorf("failed to marshal usage: %w", err)
	}

	query := `
		INSERT INTO credentials (
			id, name, type, provider, status, description,
			encrypted_value, encryption_iv, key_derivation_salt,
			tags, rotation_info, usage,
			created_at, updated_at, last_used
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = dao.db.ExecContext(ctx, query,
		cred.ID.String(),
		cred.Name,
		cred.Type.String(),
		cred.Provider,
		cred.Status.String(),
		cred.Description,
		cred.EncryptedValue,
		cred.EncryptionIV,
		cred.KeyDerivationSalt,
		string(tagsJSON),
		string(rotationJSON),
		string(usageJSON),
		cred.CreatedAt,
		cred.UpdatedAt,
		nullableTime(cred.LastUsed),
	)

	if err != nil {
		return fmt.Errorf("failed to insert credential: %w", err)
	}

	return nil
}

// Get retrieves a credential by ID
func (dao *CredentialDAO) Get(ctx context.Context, id types.ID) (*types.Credential, error) {
	query := `
		SELECT
			id, name, type, provider, status, description,
			encrypted_value, encryption_iv, key_derivation_salt,
			tags, rotation_info, usage,
			created_at, updated_at, last_used
		FROM credentials
		WHERE id = ?
	`

	cred := &types.Credential{}
	var (
		idStr, name, typeStr, provider, statusStr, description string
		encryptedValue, encryptionIV, keyDerivationSalt       []byte
		tagsJSON, rotationJSON, usageJSON                     string
		createdAt, updatedAt                                   time.Time
		lastUsed                                               sql.NullTime
	)

	err := dao.db.QueryRowContext(ctx, query, id.String()).Scan(
		&idStr, &name, &typeStr, &provider, &statusStr, &description,
		&encryptedValue, &encryptionIV, &keyDerivationSalt,
		&tagsJSON, &rotationJSON, &usageJSON,
		&createdAt, &updatedAt, &lastUsed,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("credential not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query credential: %w", err)
	}

	// Parse ID
	parsedID, err := types.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ID: %w", err)
	}

	// Parse type
	var credType types.CredentialType
	if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, typeStr)), &credType); err != nil {
		return nil, fmt.Errorf("failed to parse credential type: %w", err)
	}

	// Parse status
	var status types.CredentialStatus
	if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, statusStr)), &status); err != nil {
		return nil, fmt.Errorf("failed to parse credential status: %w", err)
	}

	// Unmarshal JSON fields
	var tags []string
	if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	var rotation types.CredentialRotation
	if err := json.Unmarshal([]byte(rotationJSON), &rotation); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rotation: %w", err)
	}

	var usage types.CredentialUsage
	if err := json.Unmarshal([]byte(usageJSON), &usage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal usage: %w", err)
	}

	cred.ID = parsedID
	cred.Name = name
	cred.Type = credType
	cred.Provider = provider
	cred.Status = status
	cred.Description = description
	cred.EncryptedValue = encryptedValue
	cred.EncryptionIV = encryptionIV
	cred.KeyDerivationSalt = keyDerivationSalt
	cred.Tags = tags
	cred.Rotation = rotation
	cred.Usage = usage
	cred.CreatedAt = createdAt
	cred.UpdatedAt = updatedAt
	if lastUsed.Valid {
		cred.LastUsed = &lastUsed.Time
	}

	return cred, nil
}

// GetByName retrieves a credential by name
func (dao *CredentialDAO) GetByName(ctx context.Context, name string) (*types.Credential, error) {
	query := `
		SELECT
			id, name, type, provider, status, description,
			encrypted_value, encryption_iv, key_derivation_salt,
			tags, rotation_info, usage,
			created_at, updated_at, last_used
		FROM credentials
		WHERE name = ?
	`

	cred := &types.Credential{}
	var (
		idStr, credName, typeStr, provider, statusStr, description string
		encryptedValue, encryptionIV, keyDerivationSalt            []byte
		tagsJSON, rotationJSON, usageJSON                          string
		createdAt, updatedAt                                        time.Time
		lastUsed                                                    sql.NullTime
	)

	err := dao.db.QueryRowContext(ctx, query, name).Scan(
		&idStr, &credName, &typeStr, &provider, &statusStr, &description,
		&encryptedValue, &encryptionIV, &keyDerivationSalt,
		&tagsJSON, &rotationJSON, &usageJSON,
		&createdAt, &updatedAt, &lastUsed,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("credential not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query credential: %w", err)
	}

	// Parse ID
	parsedID, err := types.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ID: %w", err)
	}

	// Parse type
	var credType types.CredentialType
	if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, typeStr)), &credType); err != nil {
		return nil, fmt.Errorf("failed to parse credential type: %w", err)
	}

	// Parse status
	var status types.CredentialStatus
	if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, statusStr)), &status); err != nil {
		return nil, fmt.Errorf("failed to parse credential status: %w", err)
	}

	// Unmarshal JSON fields
	var tags []string
	if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	var rotation types.CredentialRotation
	if err := json.Unmarshal([]byte(rotationJSON), &rotation); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rotation: %w", err)
	}

	var usage types.CredentialUsage
	if err := json.Unmarshal([]byte(usageJSON), &usage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal usage: %w", err)
	}

	cred.ID = parsedID
	cred.Name = credName
	cred.Type = credType
	cred.Provider = provider
	cred.Status = status
	cred.Description = description
	cred.EncryptedValue = encryptedValue
	cred.EncryptionIV = encryptionIV
	cred.KeyDerivationSalt = keyDerivationSalt
	cred.Tags = tags
	cred.Rotation = rotation
	cred.Usage = usage
	cred.CreatedAt = createdAt
	cred.UpdatedAt = updatedAt
	if lastUsed.Valid {
		cred.LastUsed = &lastUsed.Time
	}

	return cred, nil
}

// List retrieves credentials with optional filtering
func (dao *CredentialDAO) List(ctx context.Context, filter *types.CredentialFilter) ([]*types.Credential, error) {
	query := `
		SELECT
			id, name, type, provider, status, description,
			encrypted_value, encryption_iv, key_derivation_salt,
			tags, rotation_info, usage,
			created_at, updated_at, last_used
		FROM credentials
		WHERE 1=1
	`
	args := []interface{}{}

	// Apply filters
	if filter != nil {
		if filter.Provider != nil {
			query += " AND provider = ?"
			args = append(args, *filter.Provider)
		}

		if filter.Type != nil {
			query += " AND type = ?"
			args = append(args, filter.Type.String())
		}

		if filter.Status != nil {
			query += " AND status = ?"
			args = append(args, filter.Status.String())
		}

		// Tag filtering (must have all specified tags)
		if len(filter.Tags) > 0 {
			for _, tag := range filter.Tags {
				query += " AND json_extract(tags, '$') LIKE ?"
				args = append(args, "%"+tag+"%")
			}
		}
	}

	// Add ordering
	query += " ORDER BY name ASC"

	// Apply limit and offset
	if filter != nil {
		if filter.Limit > 0 {
			query += " LIMIT ?"
			args = append(args, filter.Limit)
		}

		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}

	rows, err := dao.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query credentials: %w", err)
	}
	defer rows.Close()

	credentials := []*types.Credential{}
	for rows.Next() {
		var (
			idStr, name, typeStr, provider, statusStr, description string
			encryptedValue, encryptionIV, keyDerivationSalt       []byte
			tagsJSON, rotationJSON, usageJSON                     string
			createdAt, updatedAt                                   time.Time
			lastUsed                                               sql.NullTime
		)

		err := rows.Scan(
			&idStr, &name, &typeStr, &provider, &statusStr, &description,
			&encryptedValue, &encryptionIV, &keyDerivationSalt,
			&tagsJSON, &rotationJSON, &usageJSON,
			&createdAt, &updatedAt, &lastUsed,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan credential: %w", err)
		}

		// Parse ID
		parsedID, err := types.ParseID(idStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ID: %w", err)
		}

		// Parse type
		var credType types.CredentialType
		if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, typeStr)), &credType); err != nil {
			return nil, fmt.Errorf("failed to parse credential type: %w", err)
		}

		// Parse status
		var status types.CredentialStatus
		if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, statusStr)), &status); err != nil {
			return nil, fmt.Errorf("failed to parse credential status: %w", err)
		}

		// Unmarshal JSON fields
		var tags []string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}

		var rotation types.CredentialRotation
		if err := json.Unmarshal([]byte(rotationJSON), &rotation); err != nil {
			return nil, fmt.Errorf("failed to unmarshal rotation: %w", err)
		}

		var usage types.CredentialUsage
		if err := json.Unmarshal([]byte(usageJSON), &usage); err != nil {
			return nil, fmt.Errorf("failed to unmarshal usage: %w", err)
		}

		cred := &types.Credential{
			ID:                 parsedID,
			Name:               name,
			Type:               credType,
			Provider:           provider,
			Status:             status,
			Description:        description,
			EncryptedValue:     encryptedValue,
			EncryptionIV:       encryptionIV,
			KeyDerivationSalt:  keyDerivationSalt,
			Tags:               tags,
			Rotation:           rotation,
			Usage:              usage,
			CreatedAt:          createdAt,
			UpdatedAt:          updatedAt,
		}
		if lastUsed.Valid {
			cred.LastUsed = &lastUsed.Time
		}

		credentials = append(credentials, cred)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating credentials: %w", err)
	}

	return credentials, nil
}

// Update updates an existing credential
func (dao *CredentialDAO) Update(ctx context.Context, cred *types.Credential) error {
	if err := cred.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Marshal JSON fields
	tagsJSON, err := json.Marshal(cred.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	rotationJSON, err := json.Marshal(cred.Rotation)
	if err != nil {
		return fmt.Errorf("failed to marshal rotation: %w", err)
	}

	usageJSON, err := json.Marshal(cred.Usage)
	if err != nil {
		return fmt.Errorf("failed to marshal usage: %w", err)
	}

	query := `
		UPDATE credentials SET
			name = ?,
			type = ?,
			provider = ?,
			status = ?,
			description = ?,
			encrypted_value = ?,
			encryption_iv = ?,
			key_derivation_salt = ?,
			tags = ?,
			rotation_info = ?,
			usage = ?,
			updated_at = ?,
			last_used = ?
		WHERE id = ?
	`

	result, err := dao.db.ExecContext(ctx, query,
		cred.Name,
		cred.Type.String(),
		cred.Provider,
		cred.Status.String(),
		cred.Description,
		cred.EncryptedValue,
		cred.EncryptionIV,
		cred.KeyDerivationSalt,
		string(tagsJSON),
		string(rotationJSON),
		string(usageJSON),
		cred.UpdatedAt,
		nullableTime(cred.LastUsed),
		cred.ID.String(),
	)

	if err != nil {
		return fmt.Errorf("failed to update credential: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("credential not found: %s", cred.ID)
	}

	return nil
}

// Delete deletes a credential by ID
func (dao *CredentialDAO) Delete(ctx context.Context, id types.ID) error {
	query := `DELETE FROM credentials WHERE id = ?`

	result, err := dao.db.ExecContext(ctx, query, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("credential not found: %s", id)
	}

	return nil
}

// DeleteByName deletes a credential by name
func (dao *CredentialDAO) DeleteByName(ctx context.Context, name string) error {
	query := `DELETE FROM credentials WHERE name = ?`

	result, err := dao.db.ExecContext(ctx, query, name)
	if err != nil {
		// Check for foreign key constraint violation
		if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			return fmt.Errorf("cannot delete credential: it is in use by one or more targets")
		}
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("credential not found: %s", name)
	}

	return nil
}

// Exists checks if a credential with the given name exists
func (dao *CredentialDAO) Exists(ctx context.Context, name string) (bool, error) {
	query := `SELECT COUNT(*) FROM credentials WHERE name = ?`

	var count int
	err := dao.db.QueryRowContext(ctx, query, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check credential existence: %w", err)
	}

	return count > 0, nil
}

// nullableTime converts a time pointer to sql.NullTime
func nullableTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// credNullableID converts an ID pointer to sql.NullString
func credNullableID(id *types.ID) sql.NullString {
	if id == nil {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: id.String(), Valid: true}
}
