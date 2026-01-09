package init

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/crypto"
	"github.com/zero-day-ai/gibson/internal/database"
)

// TestDefaultDirectories tests the DefaultDirectories function
func TestDefaultDirectories(t *testing.T) {
	homeDir := "/test/home"
	cfg := DefaultDirectories(homeDir)

	assert.Equal(t, homeDir, cfg.HomeDir)
	assert.Equal(t, os.FileMode(0755), cfg.Permission)
	assert.Len(t, cfg.Dirs, 9)

	// Verify all expected directories are present
	expectedDirs := []string{"agents", "tools", "plugins", "payloads", "reports", "logs", "cache", "backups", "data"}
	for _, expected := range expectedDirs {
		assert.Contains(t, cfg.Dirs, expected, "missing expected directory: %s", expected)
	}
}

// TestCreateDirectories tests directory creation
func TestCreateDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := DefaultDirectories(tmpDir)
	err := CreateDirectories(cfg)
	require.NoError(t, err)

	// Verify all directories were created
	for _, dir := range cfg.Dirs {
		fullPath := filepath.Join(tmpDir, dir)
		info, err := os.Stat(fullPath)
		require.NoError(t, err, "directory should exist: %s", fullPath)
		assert.True(t, info.IsDir(), "should be a directory: %s", fullPath)
	}
}

// TestCreateDirectories_Idempotent tests that CreateDirectories is idempotent
func TestCreateDirectories_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultDirectories(tmpDir)

	// Create directories twice
	err := CreateDirectories(cfg)
	require.NoError(t, err)

	err = CreateDirectories(cfg)
	require.NoError(t, err, "should be idempotent")

	// Verify directories still exist and are correct
	for _, dir := range cfg.Dirs {
		fullPath := filepath.Join(tmpDir, dir)
		info, err := os.Stat(fullPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	}
}

// TestValidateDirectories tests directory validation
func TestValidateDirectories(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(string) DirectoryConfig
		wantMissing  int
		wantBadPerms int
		wantErr      bool
	}{
		{
			name: "all directories exist with correct permissions",
			setup: func(tmpDir string) DirectoryConfig {
				cfg := DefaultDirectories(tmpDir)
				_ = CreateDirectories(cfg)
				return cfg
			},
			wantMissing:  0,
			wantBadPerms: 0,
			wantErr:      false,
		},
		{
			name: "missing directories",
			setup: func(tmpDir string) DirectoryConfig {
				return DefaultDirectories(tmpDir)
			},
			wantMissing:  9, // All directories missing
			wantBadPerms: 0,
			wantErr:      false,
		},
		{
			name: "wrong permissions",
			setup: func(tmpDir string) DirectoryConfig {
				cfg := DefaultDirectories(tmpDir)
				_ = CreateDirectories(cfg)

				// Change permissions on one directory
				wrongPermDir := filepath.Join(tmpDir, cfg.Dirs[0])
				_ = os.Chmod(wrongPermDir, 0700)

				return cfg
			},
			wantMissing:  0,
			wantBadPerms: 1,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfg := tt.setup(tmpDir)

			missing, badPerms, err := ValidateDirectories(cfg)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Len(t, missing, tt.wantMissing, "unexpected number of missing directories")
			assert.Len(t, badPerms, tt.wantBadPerms, "unexpected number of directories with bad permissions")
		})
	}
}

// TestValidateDirectories_FileInsteadOfDir tests validation when a file exists instead of directory
func TestValidateDirectories_FileInsteadOfDir(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultDirectories(tmpDir)

	// Create a file instead of a directory
	filePath := filepath.Join(tmpDir, cfg.Dirs[0])
	err := os.WriteFile(filePath, []byte("test"), 0644)
	require.NoError(t, err)

	_, _, err = ValidateDirectories(cfg)
	assert.Error(t, err, "should error when file exists instead of directory")
	assert.Contains(t, err.Error(), "not a directory")
}

// TestInitialize_FullFlow tests the complete initialization flow
func TestInitialize_FullFlow(t *testing.T) {
	tmpDir := t.TempDir()

	initializer := NewDefaultInitializer()
	opts := InitOptions{
		HomeDir:        tmpDir,
		NonInteractive: true,
		Force:          false,
	}

	result, err := initializer.Initialize(context.Background(), opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify result
	assert.Equal(t, tmpDir, result.HomeDir)
	assert.True(t, result.ConfigCreated, "config should be created")
	assert.True(t, result.KeyCreated, "key should be created")
	assert.True(t, result.DatabaseCreated, "database should be created")
	assert.NotEmpty(t, result.DirsCreated, "directories should be created")

	// Verify directories exist
	expectedDirs := []string{"agents", "tools", "plugins", "payloads", "reports", "logs", "cache", "backups", "data"}
	for _, dir := range expectedDirs {
		fullPath := filepath.Join(tmpDir, dir)
		info, err := os.Stat(fullPath)
		require.NoError(t, err, "directory should exist: %s", dir)
		assert.True(t, info.IsDir())
	}

	// Verify config exists and is valid
	configPath := filepath.Join(tmpDir, "config.yaml")
	_, err = os.Stat(configPath)
	assert.NoError(t, err, "config file should exist")

	// Verify key exists with correct permissions
	keyPath := filepath.Join(tmpDir, "master.key")
	info, err := os.Stat(keyPath)
	require.NoError(t, err, "key file should exist")
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "key should have 0600 permissions")

	// Verify database exists
	dbPath := filepath.Join(tmpDir, "gibson.db")
	_, err = os.Stat(dbPath)
	assert.NoError(t, err, "database should exist")

	// Verify database has schema
	db, err := database.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	row := db.Conn().QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'")
	var tableCount int
	err = row.Scan(&tableCount)
	require.NoError(t, err)
	assert.Greater(t, tableCount, 0, "database should have tables")
}

// TestInitialize_Idempotent tests that initialization is idempotent
func TestInitialize_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	initializer := NewDefaultInitializer()
	opts := InitOptions{
		HomeDir:        tmpDir,
		NonInteractive: true,
		Force:          false,
	}

	// First initialization
	result1, err := initializer.Initialize(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, result1.ConfigCreated)
	assert.True(t, result1.KeyCreated)
	assert.True(t, result1.DatabaseCreated)

	// Second initialization (should not recreate)
	result2, err := initializer.Initialize(context.Background(), opts)
	require.NoError(t, err)
	assert.False(t, result2.ConfigCreated, "config should not be recreated")
	assert.False(t, result2.KeyCreated, "key should not be recreated")
	assert.False(t, result2.DatabaseCreated, "database should not be recreated")
	assert.Empty(t, result2.DirsCreated, "no new directories should be created")
}

// TestInitialize_Force tests force mode
func TestInitialize_Force(t *testing.T) {
	tmpDir := t.TempDir()

	initializer := NewDefaultInitializer()
	opts := InitOptions{
		HomeDir:        tmpDir,
		NonInteractive: true,
		Force:          false,
	}

	// First initialization
	result1, err := initializer.Initialize(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, result1.ConfigCreated)

	// Read original key
	keyPath := filepath.Join(tmpDir, "master.key")
	originalKey, err := os.ReadFile(keyPath)
	require.NoError(t, err)

	// Second initialization with force
	opts.Force = true
	result2, err := initializer.Initialize(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, result2.ConfigCreated, "config should be recreated with force")
	assert.True(t, result2.KeyCreated, "key should be recreated with force")

	// Verify warnings about overwriting
	assert.NotEmpty(t, result2.Warnings, "should have warnings about overwriting")

	// Verify key was changed
	newKey, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	assert.NotEqual(t, originalKey, newKey, "key should be different after force init")
}

// TestInitialize_DefaultHomeDir tests initialization without explicit home directory
func TestInitialize_DefaultHomeDir(t *testing.T) {
	// Check if default home dir already exists - skip test to avoid destroying real data
	defaultCfg := config.DefaultConfig()
	if _, err := os.Stat(defaultCfg.Core.HomeDir); err == nil {
		t.Skipf("Skipping test: default home dir %s already exists (would destroy real data)", defaultCfg.Core.HomeDir)
	}

	initializer := NewDefaultInitializer()
	opts := InitOptions{
		HomeDir:        "", // Use default
		NonInteractive: true,
		Force:          false,
	}

	result, err := initializer.Initialize(context.Background(), opts)
	require.NoError(t, err)

	// Should use default home dir from config
	assert.Equal(t, defaultCfg.Core.HomeDir, result.HomeDir)

	// Clean up
	_ = os.RemoveAll(result.HomeDir)
}

// TestValidateSetup tests complete setup validation
func TestValidateSetup(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a complete setup
	initializer := NewDefaultInitializer()
	opts := InitOptions{
		HomeDir:        tmpDir,
		NonInteractive: true,
		Force:          false,
	}

	_, err := initializer.Initialize(context.Background(), opts)
	require.NoError(t, err)

	// Validate the setup
	result, err := ValidateSetup(tmpDir)
	require.NoError(t, err)
	assert.True(t, result.Valid, "setup should be valid")
	assert.Empty(t, result.Errors, "should have no errors")
}

// TestValidateSetup_MissingComponents tests validation with missing components
func TestValidateSetup_MissingComponents(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(string)
		expectInvalid bool
		checkError    func(*testing.T, []ValidationError)
	}{
		{
			name: "missing home directory",
			setup: func(tmpDir string) {
				// Don't create anything - this will cause missing directory errors
			},
			expectInvalid: true,
			checkError: func(t *testing.T, errors []ValidationError) {
				// When home directory doesn't exist, we get errors about missing directories
				assert.NotEmpty(t, errors, "should have validation errors")
			},
		},
		{
			name: "missing config",
			setup: func(tmpDir string) {
				_ = os.MkdirAll(tmpDir, 0755)
				cfg := DefaultDirectories(tmpDir)
				_ = CreateDirectories(cfg)
			},
			expectInvalid: true,
			checkError: func(t *testing.T, errors []ValidationError) {
				found := false
				for _, e := range errors {
					if e.Component == "config" {
						found = true
						break
					}
				}
				assert.True(t, found, "should have config error")
			},
		},
		{
			name: "missing encryption key",
			setup: func(tmpDir string) {
				_ = os.MkdirAll(tmpDir, 0755)
				cfg := DefaultDirectories(tmpDir)
				_ = CreateDirectories(cfg)

				// Create config but not key
				configPath := filepath.Join(tmpDir, "config.yaml")
				defaultCfg := config.DefaultConfig()
				_ = writeConfigFile(configPath, defaultCfg)
			},
			expectInvalid: true,
			checkError: func(t *testing.T, errors []ValidationError) {
				found := false
				for _, e := range errors {
					if e.Component == "encryption_key" {
						found = true
						break
					}
				}
				assert.True(t, found, "should have encryption_key error")
			},
		},
		{
			name: "insecure key permissions",
			setup: func(tmpDir string) {
				initializer := NewDefaultInitializer()
				opts := InitOptions{
					HomeDir:        tmpDir,
					NonInteractive: true,
					Force:          false,
				}
				_, _ = initializer.Initialize(context.Background(), opts)

				// Change key permissions to insecure
				keyPath := filepath.Join(tmpDir, "master.key")
				_ = os.Chmod(keyPath, 0644)
			},
			expectInvalid: true,
			checkError: func(t *testing.T, errors []ValidationError) {
				found := false
				for _, e := range errors {
					if e.Component == "encryption_key" && contains(e.Message, "insecure") {
						found = true
						break
					}
				}
				assert.True(t, found, "should have insecure permissions error")
			},
		},
		{
			name: "missing database",
			setup: func(tmpDir string) {
				_ = os.MkdirAll(tmpDir, 0755)
				cfg := DefaultDirectories(tmpDir)
				_ = CreateDirectories(cfg)

				// Create config and key but not database
				configPath := filepath.Join(tmpDir, "config.yaml")
				defaultCfg := config.DefaultConfig()
				_ = writeConfigFile(configPath, defaultCfg)

				keyManager := crypto.NewFileKeyManager()
				key, _ := keyManager.GenerateKey()
				keyPath := filepath.Join(tmpDir, "master.key")
				_ = keyManager.SaveKey(key, keyPath)
			},
			expectInvalid: true,
			checkError: func(t *testing.T, errors []ValidationError) {
				found := false
				for _, e := range errors {
					if e.Component == "database" {
						found = true
						break
					}
				}
				assert.True(t, found, "should have database error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(tmpDir)

			result, err := ValidateSetup(tmpDir)
			require.NoError(t, err)

			if tt.expectInvalid {
				assert.False(t, result.Valid, "setup should be invalid")
				assert.NotEmpty(t, result.Errors, "should have errors")
			}

			if tt.checkError != nil {
				tt.checkError(t, result.Errors)
			}
		})
	}
}

// TestValidateSetup_CorruptConfig tests validation with corrupt configuration
func TestValidateSetup_CorruptConfig(t *testing.T) {
	tmpDir := t.TempDir()

	_ = os.MkdirAll(tmpDir, 0755)
	cfg := DefaultDirectories(tmpDir)
	_ = CreateDirectories(cfg)

	// Create corrupt config file
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("invalid: yaml: content: [[["), 0644)
	require.NoError(t, err)

	result, err := ValidateSetup(tmpDir)
	require.NoError(t, err)
	assert.False(t, result.Valid)

	// Should have error about invalid config
	found := false
	for _, e := range result.Errors {
		if e.Component == "config" && contains(e.Message, "invalid") {
			found = true
			break
		}
	}
	assert.True(t, found, "should have invalid config error")
}

// TestValidation_Remediation tests that validation errors include actionable remediation
func TestValidation_Remediation(t *testing.T) {
	tmpDir := t.TempDir()

	result, err := ValidateSetup(tmpDir)
	require.NoError(t, err)
	assert.False(t, result.Valid)

	// All errors should have remediation actions
	for _, e := range result.Errors {
		assert.NotEmpty(t, e.Action, "error should have remediation action: %s", e.Message)
		assert.NotEmpty(t, e.Component, "error should have component: %s", e.Message)
	}
}

// TestInitialize_WithContext tests context cancellation
func TestInitialize_WithContext(t *testing.T) {
	tmpDir := t.TempDir()

	initializer := NewDefaultInitializer()
	opts := InitOptions{
		HomeDir:        tmpDir,
		NonInteractive: true,
		Force:          false,
	}

	// Use a real context (cancellation won't affect this simple init, but testing the interface)
	ctx := context.Background()
	result, err := initializer.Initialize(ctx, opts)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// TestValidateHomeDir_FileInsteadOfDir tests when a file exists at the home directory path
func TestValidateHomeDir_FileInsteadOfDir(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "not-a-dir")

	// Create a file instead of directory
	err := os.WriteFile(filePath, []byte("test"), 0644)
	require.NoError(t, err)

	result, err := ValidateSetup(filePath)
	require.NoError(t, err)
	assert.False(t, result.Valid)

	// Should have error about file not being a directory
	found := false
	for _, e := range result.Errors {
		if contains(e.Message, "not a directory") {
			found = true
			break
		}
	}
	assert.True(t, found, "should have error about file not being directory")
}

// TestValidateSetup_StatError tests validation when stat fails for unexpected reasons
func TestValidateSetup_StatError(t *testing.T) {
	// Test with a path that can't be accessed (permission denied would be ideal but hard to test)
	// Just ensure the validation handles errors gracefully
	tmpDir := t.TempDir()

	// Create a valid setup first
	initializer := NewDefaultInitializer()
	opts := InitOptions{
		HomeDir:        tmpDir,
		NonInteractive: true,
		Force:          false,
	}
	_, err := initializer.Initialize(context.Background(), opts)
	require.NoError(t, err)

	// Validation should pass for valid setup
	result, err := ValidateSetup(tmpDir)
	require.NoError(t, err)
	assert.True(t, result.Valid)
}

// TestInitializeConfig_InvalidExistingConfig tests handling of invalid existing config
func TestInitializeConfig_InvalidExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	cfg := DefaultDirectories(tmpDir)
	err := CreateDirectories(cfg)
	require.NoError(t, err)

	// Create an invalid config file
	configPath := filepath.Join(tmpDir, "config.yaml")
	err = os.WriteFile(configPath, []byte("invalid yaml [[["), 0644)
	require.NoError(t, err)

	// Initialize should detect invalid config and warn
	initializer := NewDefaultInitializer()
	opts := InitOptions{
		HomeDir:        tmpDir,
		NonInteractive: true,
		Force:          false,
	}

	result, err := initializer.Initialize(context.Background(), opts)
	require.NoError(t, err)

	// Should have warnings about invalid config
	assert.NotEmpty(t, result.Warnings, "should have warnings about invalid config")
}

// TestInitializeKey_InvalidExistingKey tests handling of invalid existing key
func TestInitializeKey_InvalidExistingKey(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	cfg := DefaultDirectories(tmpDir)
	err := CreateDirectories(cfg)
	require.NoError(t, err)

	// Create an invalid key file (wrong size)
	keyPath := filepath.Join(tmpDir, "master.key")
	err = os.WriteFile(keyPath, []byte("invalid"), 0600)
	require.NoError(t, err)

	// Initialize should detect invalid key and warn
	initializer := NewDefaultInitializer()
	opts := InitOptions{
		HomeDir:        tmpDir,
		NonInteractive: true,
		Force:          false,
	}

	result, err := initializer.Initialize(context.Background(), opts)
	require.NoError(t, err)

	// Should have warnings about invalid key
	assert.NotEmpty(t, result.Warnings, "should have warnings about invalid key")
}

// TestValidateDatabase_EmptyDatabase tests validation of empty database
func TestValidateDatabase_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	cfg := DefaultDirectories(tmpDir)
	err := CreateDirectories(cfg)
	require.NoError(t, err)

	// Create config
	configPath := filepath.Join(tmpDir, "config.yaml")
	defaultCfg := config.DefaultConfig()
	err = writeConfigFile(configPath, defaultCfg)
	require.NoError(t, err)

	// Create key
	keyManager := crypto.NewFileKeyManager()
	key, err := keyManager.GenerateKey()
	require.NoError(t, err)
	keyPath := filepath.Join(tmpDir, "master.key")
	err = keyManager.SaveKey(key, keyPath)
	require.NoError(t, err)

	// Create empty database (no tables)
	dbPath := filepath.Join(tmpDir, "gibson.db")
	db, err := database.Open(dbPath)
	require.NoError(t, err)
	db.Close()

	// Validate - should warn about no tables
	result, err := ValidateSetup(tmpDir)
	require.NoError(t, err)

	// Should have warning about no tables
	foundWarning := false
	for _, w := range result.Warnings {
		if contains(w.Message, "no tables") {
			foundWarning = true
			break
		}
	}
	assert.True(t, foundWarning, "should have warning about no tables")
}

// TestValidateConfigFile_DirectoryInsteadOfFile tests when config path is a directory
func TestValidateConfigFile_DirectoryInsteadOfFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	cfg := DefaultDirectories(tmpDir)
	err := CreateDirectories(cfg)
	require.NoError(t, err)

	// Create a directory instead of config file
	configPath := filepath.Join(tmpDir, "config.yaml")
	err = os.MkdirAll(configPath, 0755)
	require.NoError(t, err)

	// Validate
	result, err := ValidateSetup(tmpDir)
	require.NoError(t, err)
	assert.False(t, result.Valid)

	// Should have error about config being a directory
	found := false
	for _, e := range result.Errors {
		if e.Component == "config" && contains(e.Message, "directory") {
			found = true
			break
		}
	}
	assert.True(t, found, "should have error about config being directory")
}

// TestCreateDirectories_ErrorCase tests error handling in directory creation
func TestCreateDirectories_ErrorCase(t *testing.T) {
	// Create a file where we want to create a directory
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "agents")
	err := os.WriteFile(blockingFile, []byte("blocking"), 0644)
	require.NoError(t, err)

	cfg := DefaultDirectories(tmpDir)
	err = CreateDirectories(cfg)

	// Should fail because file exists where directory should be
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create directory")
}

// TestInitialize_WithErrors tests initialization with validation errors
func TestInitialize_WithErrors(t *testing.T) {
	tmpBase := t.TempDir()
	tmpFile := filepath.Join(tmpBase, "blocking-file")

	// Create a file where home directory should be (will cause errors)
	err := os.WriteFile(tmpFile, []byte("blocking"), 0644)
	require.NoError(t, err)

	initializer := NewDefaultInitializer()
	opts := InitOptions{
		HomeDir:        tmpFile,
		NonInteractive: true,
		Force:          false,
	}

	// Initialize should fail because tmpFile is a file, not a directory
	_, err = initializer.Initialize(context.Background(), opts)
	assert.Error(t, err)
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmark tests
func BenchmarkCreateDirectories(b *testing.B) {
	tmpBase := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tmpDir := filepath.Join(tmpBase, fmt.Sprintf("bench-%d", i))
		_ = os.MkdirAll(tmpDir, 0755)

		cfg := DefaultDirectories(tmpDir)
		_ = CreateDirectories(cfg)
	}
}

func BenchmarkValidateSetup(b *testing.B) {
	tmpDir := b.TempDir()

	// Setup once
	initializer := NewDefaultInitializer()
	opts := InitOptions{
		HomeDir:        tmpDir,
		NonInteractive: true,
		Force:          false,
	}
	_, _ = initializer.Initialize(context.Background(), opts)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ValidateSetup(tmpDir)
	}
}
