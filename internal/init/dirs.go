package init

import (
	"fmt"
	"os"
	"path/filepath"
)

// DirectoryConfig holds configuration for directory creation
type DirectoryConfig struct {
	HomeDir    string
	Dirs       []string
	Permission os.FileMode
}

// DefaultDirectories returns the standard Gibson directory structure
// that should be created during initialization.
//
// The directory structure includes:
//   - agents: Agent definitions and configurations
//   - tools: Tool definitions and scripts
//   - plugins: Plugin modules and extensions
//   - payloads: Exploit payloads and attack vectors
//   - reports: Generated reports and findings
//   - logs: Application logs
//   - cache: Cached data and temporary files
//   - backups: Database and configuration backups
//   - data: Persistent data storage
func DefaultDirectories(homeDir string) DirectoryConfig {
	return DirectoryConfig{
		HomeDir: homeDir,
		Dirs: []string{
			"agents",
			"tools",
			"plugins",
			"payloads",
			"reports",
			"logs",
			"cache",
			"backups",
			"data",
		},
		Permission: 0755, // rwxr-xr-x - owner can read/write/execute, others can read/execute
	}
}

// CreateDirectories creates all directories specified in the DirectoryConfig.
// This function is idempotent - it safely handles existing directories.
//
// For each directory in the configuration:
//   - Creates the full path (including parent directories) using os.MkdirAll
//   - Sets the specified permissions on created directories
//   - Gracefully skips directories that already exist
//
// Returns an error if any directory creation fails. The error includes
// the path of the directory that failed and the underlying cause.
func CreateDirectories(cfg DirectoryConfig) error {
	for _, dir := range cfg.Dirs {
		fullPath := filepath.Join(cfg.HomeDir, dir)

		// MkdirAll creates the directory and all necessary parent directories
		// If the directory already exists, MkdirAll does nothing and returns nil
		// This makes the function idempotent and safe to run multiple times
		if err := os.MkdirAll(fullPath, cfg.Permission); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", fullPath, err)
		}
	}

	return nil
}

// ValidateDirectories checks that all required directories exist and have correct permissions.
// Returns the list of missing directories, incorrect permissions, and any error encountered.
func ValidateDirectories(cfg DirectoryConfig) (missing []string, badPerms []string, err error) {
	for _, dir := range cfg.Dirs {
		fullPath := filepath.Join(cfg.HomeDir, dir)

		info, statErr := os.Stat(fullPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				missing = append(missing, fullPath)
				continue
			}
			return nil, nil, fmt.Errorf("failed to stat directory %s: %w", fullPath, statErr)
		}

		// Check if it's actually a directory
		if !info.IsDir() {
			return nil, nil, fmt.Errorf("%s exists but is not a directory", fullPath)
		}

		// Check permissions (mask to get only permission bits)
		actualPerms := info.Mode().Perm()
		if actualPerms != cfg.Permission {
			badPerms = append(badPerms, fmt.Sprintf("%s (expected %o, got %o)", fullPath, cfg.Permission, actualPerms))
		}
	}

	return missing, badPerms, nil
}
