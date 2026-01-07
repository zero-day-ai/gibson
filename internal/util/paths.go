package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands a path by handling:
// - Tilde (~) expansion to user home directory
// - Environment variable expansion ($VAR or ${VAR})
// - Cleaning the final path
//
// Examples:
//   - "~/data" -> "/home/user/data"
//   - "$HOME/data" -> "/home/user/data"
//   - "${HOME}/data" -> "/home/user/data"
//   - "~/.gibson/config.yaml" -> "/home/user/.gibson/config.yaml"
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	// Handle tilde expansion
	if strings.HasPrefix(path, "~/") || path == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		if path == "~" {
			return homeDir, nil
		}
		path = filepath.Join(homeDir, path[2:])
	}

	// Expand environment variables
	path = os.ExpandEnv(path)

	// Clean the path
	path = filepath.Clean(path)

	return path, nil
}

// MustExpandPath is like ExpandPath but panics on error.
// Only use this for static/known-good paths during initialization.
func MustExpandPath(path string) string {
	expanded, err := ExpandPath(path)
	if err != nil {
		panic(fmt.Sprintf("failed to expand path %q: %v", path, err))
	}
	return expanded
}
