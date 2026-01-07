package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zero-day-ai/gibson/internal/config"
)

// ConnectOrFail attempts to connect to the Gibson daemon with user-friendly error messages.
//
// This is the recommended function for CLI commands to connect to the daemon. It:
//  1. Discovers Gibson home directory from environment or defaults
//  2. Checks for daemon.json file existence
//  3. Attempts connection with a reasonable timeout
//  4. Returns clear, actionable error messages if connection fails
//
// The function handles common error scenarios:
//   - Daemon not running (daemon.json missing)
//   - Daemon crashed (daemon.json exists but connection fails)
//   - Connection timeout (daemon hung or network issues)
//
// Parameters:
//   - ctx: Base context for the connection attempt
//
// Returns:
//   - *Client: Connected client ready for use
//   - error: User-friendly error with troubleshooting hints
//
// Example:
//
//	client, err := ConnectOrFail(ctx)
//	if err != nil {
//	    // Error already includes helpful message like:
//	    // "Gibson daemon not running. Start with: gibson daemon start"
//	    return err
//	}
//	defer client.Close()
//
//	// Use client for daemon operations
//	status, err := client.Status(ctx)
func ConnectOrFail(ctx context.Context) (*Client, error) {
	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return nil, fmt.Errorf("failed to determine Gibson home directory: %w\n\n"+
			"Try setting GIBSON_HOME environment variable", err)
	}

	// Build path to daemon.json
	daemonInfoPath := filepath.Join(homeDir, "daemon.json")

	// Check if daemon.json exists
	if _, err := os.Stat(daemonInfoPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("Gibson daemon not running\n\n"+
			"Start the daemon with:\n"+
			"  gibson daemon start\n\n"+
			"For background operation, use shell job control:\n"+
			"  gibson daemon start &\n\n"+
			"Expected daemon info at: %s", daemonInfoPath)
	}

	// Create context with connection timeout
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Attempt connection
	client, err := ConnectFromInfo(connectCtx, daemonInfoPath)
	if err != nil {
		// Check if daemon.json exists but process is dead
		return nil, fmt.Errorf("failed to connect to daemon: %w\n\n"+
			"The daemon may have crashed or is not responding.\n\n"+
			"Troubleshooting steps:\n"+
			"  1. Check daemon status: gibson daemon status\n"+
			"  2. Check daemon logs: tail -f %s/daemon.log\n"+
			"  3. Stop stale daemon: gibson daemon stop\n"+
			"  4. Start fresh daemon: gibson daemon start\n\n"+
			"If the problem persists, check for port conflicts:\n"+
			"  - etcd ports: 2379, 2380\n"+
			"  - callback server: 50001\n"+
			"  - daemon gRPC: 50002",
			err, homeDir)
	}

	return client, nil
}

// getGibsonHome returns the Gibson home directory.
//
// It checks in order:
//  1. GIBSON_HOME environment variable
//  2. $HOME/.gibson (default)
//  3. Falls back to config.DefaultHomeDir() which handles edge cases
//
// Returns:
//   - string: Gibson home directory path
//   - error: Non-nil if home directory cannot be determined
func getGibsonHome() (string, error) {
	// Check environment variable first
	if homeDir := os.Getenv("GIBSON_HOME"); homeDir != "" {
		return homeDir, nil
	}

	// Use default from config package
	return config.DefaultHomeDir(), nil
}
