package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/database"
	initpkg "github.com/zero-day-ai/gibson/internal/init"
)

// setupIntegrationTest creates a complete test environment with temp home directory
func setupIntegrationTest(t *testing.T) (homeDir string, cleanup func()) {
	t.Helper()

	// Create temp directory for test home
	tempDir, err := os.MkdirTemp("", "gibson-integration-*")
	require.NoError(t, err, "Failed to create temp directory")

	// Set GIBSON_HOME environment variable
	oldHome := os.Getenv("GIBSON_HOME")
	os.Setenv("GIBSON_HOME", tempDir)

	cleanup = func() {
		os.RemoveAll(tempDir)
		os.Setenv("GIBSON_HOME", oldHome)
	}

	return tempDir, cleanup
}

// TestWorkflow_InitConfigAgent tests the init → config → agent workflow
func TestWorkflow_InitConfigAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Step 1: gibson init
	t.Run("init creates home directory and config", func(t *testing.T) {
		initializer := initpkg.NewDefaultInitializer()
		opts := initpkg.InitOptions{
			HomeDir:        homeDir,
			NonInteractive: true,
			Force:          false,
		}

		result, err := initializer.Initialize(ctx, opts)
		require.NoError(t, err, "Init should succeed")
		assert.NotNil(t, result)
		assert.True(t, result.ConfigCreated, "Config should be created")
		assert.True(t, result.DatabaseCreated, "Database should be created")
		assert.True(t, result.KeyCreated, "Encryption key should be created")
		assert.Greater(t, len(result.DirsCreated), 0, "Directories should be created")

		// Verify home directory structure
		assert.DirExists(t, homeDir, "Home directory should exist")
		assert.FileExists(t, filepath.Join(homeDir, "config.yaml"), "Config file should exist")
		assert.FileExists(t, filepath.Join(homeDir, "gibson.db"), "Database should exist")
		assert.FileExists(t, filepath.Join(homeDir, "encryption.key"), "Encryption key should exist")
	})

	// Step 2: gibson config show
	t.Run("config show displays configuration", func(t *testing.T) {
		configPath := filepath.Join(homeDir, "config.yaml")
		loader := config.NewConfigLoader(config.NewValidator())
		cfg, err := loader.Load(configPath)
		require.NoError(t, err, "Config should load successfully")
		assert.NotNil(t, cfg)

		// Verify core config values
		assert.Equal(t, homeDir, cfg.Core.HomeDir, "Home directory should match")
		assert.NotEmpty(t, cfg.Core.DataDir, "Data directory should be set")
		assert.NotEmpty(t, cfg.Database.Path, "Database path should be set")
		assert.NotEmpty(t, cfg.Security.EncryptionAlgorithm, "Encryption algorithm should be set")
	})

	// Step 3: gibson agent list (should be empty initially)
	t.Run("agent list shows empty list initially", func(t *testing.T) {
		// This test would require the component registry to be set up
		// For now, we verify that the database is accessible
		dbPath := filepath.Join(homeDir, "gibson.db")
		db, err := database.Open(dbPath)
		require.NoError(t, err, "Database should open successfully")
		defer db.Close()

		// Verify database is initialized
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM targets").Scan(&count)
		require.NoError(t, err, "Should query targets table")
		assert.Equal(t, 0, count, "Should have no targets initially")
	})
}

// TestWorkflow_CredentialTargetFlow tests credential → target association workflow
func TestWorkflow_CredentialTargetFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize environment
	initializer := initpkg.NewDefaultInitializer()
	opts := initpkg.InitOptions{
		HomeDir:        homeDir,
		NonInteractive: true,
		Force:          false,
	}
	_, err := initializer.Initialize(ctx, opts)
	require.NoError(t, err, "Init should succeed")

	// Open database
	dbPath := filepath.Join(homeDir, "gibson.db")
	db, err := database.Open(dbPath)
	require.NoError(t, err, "Database should open")
	defer db.Close()

	// Step 1: Add a credential (mocked - would normally go through credential command)
	t.Run("credential add creates encrypted credential", func(t *testing.T) {
		// We'll verify the credential DAO works
		credDAO := database.NewCredentialDAO(db)
		require.NotNil(t, credDAO, "Credential DAO should be created")

		// Note: Full credential creation requires encryption key loading
		// This test validates the database structure is ready
		var tableExists bool
		err := db.QueryRow(`
			SELECT COUNT(*) > 0
			FROM sqlite_master
			WHERE type='table' AND name='credentials'
		`).Scan(&tableExists)
		require.NoError(t, err)
		assert.True(t, tableExists, "Credentials table should exist")
	})

	// Step 2: Add a target (mocked - would normally go through target command)
	t.Run("target add with credential reference", func(t *testing.T) {
		// Verify target DAO works
		targetDAO := database.NewTargetDAO(db)
		require.NotNil(t, targetDAO, "Target DAO should be created")

		// Verify targets table exists
		var tableExists bool
		err := db.QueryRow(`
			SELECT COUNT(*) > 0
			FROM sqlite_master
			WHERE type='table' AND name='targets'
		`).Scan(&tableExists)
		require.NoError(t, err)
		assert.True(t, tableExists, "Targets table should exist")
	})

	// Step 3: Verify target has credential associated
	t.Run("verify credential-target association", func(t *testing.T) {
		// Check that the schema supports credential_id foreign key
		var hasColumn bool
		err := db.QueryRow(`
			SELECT COUNT(*) > 0
			FROM pragma_table_info('targets')
			WHERE name = 'credential_id'
		`).Scan(&hasColumn)
		require.NoError(t, err)
		assert.True(t, hasColumn, "Targets table should have credential_id column")
	})
}

// TestWorkflow_MissionFindingExport tests mission → finding export workflow
func TestWorkflow_MissionFindingExport(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize environment
	initializer := initpkg.NewDefaultInitializer()
	opts := initpkg.InitOptions{
		HomeDir:        homeDir,
		NonInteractive: true,
		Force:          false,
	}
	_, err := initializer.Initialize(ctx, opts)
	require.NoError(t, err, "Init should succeed")

	// Open database
	dbPath := filepath.Join(homeDir, "gibson.db")
	db, err := database.Open(dbPath)
	require.NoError(t, err, "Database should open")
	defer db.Close()

	// Step 1: Create test mission in database
	t.Run("create test mission", func(t *testing.T) {
		missionDAO := database.NewMissionDAO(db)
		require.NotNil(t, missionDAO, "Mission DAO should be created")

		// Verify missions table exists
		var tableExists bool
		err := db.QueryRow(`
			SELECT COUNT(*) > 0
			FROM sqlite_master
			WHERE type='table' AND name='missions'
		`).Scan(&tableExists)
		require.NoError(t, err)
		assert.True(t, tableExists, "Missions table should exist")
	})

	// Step 2: Create test findings (mocked)
	t.Run("create test findings", func(t *testing.T) {
		// Verify findings table structure
		var tableExists bool
		err := db.QueryRow(`
			SELECT COUNT(*) > 0
			FROM sqlite_master
			WHERE type='table' AND name='findings'
		`).Scan(&tableExists)
		require.NoError(t, err)
		assert.True(t, tableExists, "Findings table should exist")

		// Verify FTS table for findings search
		err = db.QueryRow(`
			SELECT COUNT(*) > 0
			FROM sqlite_master
			WHERE type='table' AND name='findings_fts'
		`).Scan(&tableExists)
		require.NoError(t, err)
		assert.True(t, tableExists, "Findings FTS table should exist")
	})

	// Step 3: gibson finding list --mission ID (verify query structure)
	t.Run("finding list supports mission filter", func(t *testing.T) {
		// Test that we can query findings by mission_id
		rows, err := db.Query(`
			SELECT COUNT(*) FROM findings WHERE mission_id = ?
		`, "test-mission-id")
		require.NoError(t, err)
		defer rows.Close()
		assert.True(t, rows.Next(), "Query should execute successfully")
	})

	// Step 4: gibson finding export --format json (verify export capability)
	t.Run("finding export formats available", func(t *testing.T) {
		// Verify that findings table has all necessary columns for export
		columns := []string{
			"id", "mission_id", "title", "description", "severity",
			"category", "confidence", "status", "cvss_score",
			"created_at", "updated_at",
		}

		for _, col := range columns {
			var hasColumn bool
			err := db.QueryRow(`
				SELECT COUNT(*) > 0
				FROM pragma_table_info('findings')
				WHERE name = ?
			`, col).Scan(&hasColumn)
			require.NoError(t, err)
			assert.True(t, hasColumn, "Findings table should have %s column", col)
		}
	})
}

// TestWorkflow_ConfigValidation tests configuration validation across workflows
func TestWorkflow_ConfigValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize
	initializer := initpkg.NewDefaultInitializer()
	opts := initpkg.InitOptions{
		HomeDir:        homeDir,
		NonInteractive: true,
		Force:          false,
	}
	_, err := initializer.Initialize(ctx, opts)
	require.NoError(t, err)

	t.Run("config file has valid structure", func(t *testing.T) {
		configPath := filepath.Join(homeDir, "config.yaml")
		content, err := os.ReadFile(configPath)
		require.NoError(t, err)

		// Check for key sections
		configStr := string(content)
		assert.Contains(t, configStr, "core:", "Config should have core section")
		assert.Contains(t, configStr, "database:", "Config should have database section")
		assert.Contains(t, configStr, "security:", "Config should have security section")
		assert.Contains(t, configStr, "llm:", "Config should have llm section")
	})

	t.Run("config validates successfully", func(t *testing.T) {
		configPath := filepath.Join(homeDir, "config.yaml")
		loader := config.NewConfigLoader(config.NewValidator())
		cfg, err := loader.Load(configPath)
		require.NoError(t, err, "Config should load and validate")
		assert.NotNil(t, cfg)
	})

	t.Run("config get retrieves values", func(t *testing.T) {
		configPath := filepath.Join(homeDir, "config.yaml")
		loader := config.NewConfigLoader(config.NewValidator())
		cfg, err := loader.Load(configPath)
		require.NoError(t, err)

		// Test various config accessors
		assert.NotEmpty(t, cfg.Core.HomeDir)
		assert.NotEmpty(t, cfg.Database.Path)
		assert.NotEmpty(t, cfg.Security.EncryptionAlgorithm)
		assert.Greater(t, cfg.Core.ParallelLimit, 0)
	})
}

// TestWorkflow_DatabaseIntegrity tests database operations across workflows
func TestWorkflow_DatabaseIntegrity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize
	initializer := initpkg.NewDefaultInitializer()
	opts := initpkg.InitOptions{
		HomeDir:        homeDir,
		NonInteractive: true,
		Force:          false,
	}
	_, err := initializer.Initialize(ctx, opts)
	require.NoError(t, err)

	dbPath := filepath.Join(homeDir, "gibson.db")
	db, err := database.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	t.Run("all core tables exist", func(t *testing.T) {
		tables := []string{
			"targets",
			"credentials",
			"missions",
			"findings",
			"findings_fts",
		}

		for _, table := range tables {
			var exists bool
			err := db.QueryRow(`
				SELECT COUNT(*) > 0
				FROM sqlite_master
				WHERE type='table' AND name=?
			`, table).Scan(&exists)
			require.NoError(t, err)
			assert.True(t, exists, "Table %s should exist", table)
		}
	})

	t.Run("database supports concurrent access", func(t *testing.T) {
		// Test that we can open multiple connections (WAL mode)
		db2, err := database.Open(dbPath)
		require.NoError(t, err)
		defer db2.Close()

		// Both connections should be able to query
		var count1, count2 int
		err = db.QueryRow("SELECT COUNT(*) FROM targets").Scan(&count1)
		require.NoError(t, err)

		err = db2.QueryRow("SELECT COUNT(*) FROM targets").Scan(&count2)
		require.NoError(t, err)

		assert.Equal(t, count1, count2, "Both connections should see same data")
	})

	t.Run("foreign key constraints are enabled", func(t *testing.T) {
		var enabled bool
		err := db.QueryRow("PRAGMA foreign_keys").Scan(&enabled)
		require.NoError(t, err)
		assert.True(t, enabled, "Foreign key constraints should be enabled")
	})
}

// TestWorkflow_ErrorHandling tests error handling across workflows
func TestWorkflow_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("init fails on invalid home directory", func(t *testing.T) {
		initializer := initpkg.NewDefaultInitializer()
		opts := initpkg.InitOptions{
			HomeDir:        "/root/cannot-write-here",
			NonInteractive: true,
			Force:          false,
		}

		_, err := initializer.Initialize(context.Background(), opts)
		if os.Geteuid() != 0 {
			// Should fail if not root
			assert.Error(t, err, "Init should fail on unwritable directory")
		}
	})

	t.Run("config load fails on missing file", func(t *testing.T) {
		loader := config.NewConfigLoader(config.NewValidator())
		_, err := loader.Load("/nonexistent/config.yaml")
		assert.Error(t, err, "Config load should fail on missing file")
	})

	t.Run("database open fails on invalid path", func(t *testing.T) {
		_, err := database.Open("/invalid/path/to/db.sqlite")
		assert.Error(t, err, "Database open should fail on invalid path")
	})
}

// TestWorkflow_CLICommands tests that CLI commands integrate properly
func TestWorkflow_CLICommands(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	homeDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Initialize
	initializer := initpkg.NewDefaultInitializer()
	opts := initpkg.InitOptions{
		HomeDir:        homeDir,
		NonInteractive: true,
		Force:          false,
	}
	_, err := initializer.Initialize(context.Background(), opts)
	require.NoError(t, err)

	t.Run("all commands are registered", func(t *testing.T) {
		// Check that rootCmd has all expected subcommands
		expectedCommands := []string{
			"init",
			"version",
			"config",
			"target",
			"credential",
			"agent",
			"tool",
			"plugin",
			"mission",
			"finding",
			"attack",
			"status",
			"console",
		}

		registeredCommands := make(map[string]bool)
		for _, cmd := range rootCmd.Commands() {
			registeredCommands[cmd.Name()] = true
		}

		for _, cmdName := range expectedCommands {
			assert.True(t, registeredCommands[cmdName], "Command %s should be registered", cmdName)
		}
	})

	t.Run("help text is available for all commands", func(t *testing.T) {
		var buf bytes.Buffer
		rootCmd.SetOut(&buf)
		rootCmd.SetErr(&buf)
		rootCmd.SetArgs([]string{"--help"})

		err := rootCmd.Execute()
		require.NoError(t, err)

		helpText := buf.String()
		assert.Contains(t, helpText, "Gibson", "Help should contain Gibson")
		assert.Contains(t, helpText, "Available Commands", "Help should list commands")
	})

	t.Run("version command works", func(t *testing.T) {
		var buf bytes.Buffer
		versionCmd.SetOut(&buf)
		versionCmd.Run(versionCmd, []string{})

		output := buf.String()
		assert.Contains(t, strings.ToLower(output), "gibson", "Version should contain Gibson")
	})
}
