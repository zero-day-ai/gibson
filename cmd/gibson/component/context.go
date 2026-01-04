package component

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/core"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/build"
	"github.com/zero-day-ai/gibson/internal/component/git"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/registry"
)

// buildCommandContext creates a CommandContext from the cobra command context.
// This helper centralizes the construction of CommandContext for all component commands.
func buildCommandContext(cmd *cobra.Command) (*core.CommandContext, error) {
	ctx := cmd.Context()

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return nil, fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create DAO
	dao := database.NewComponentDAO(db)

	// Create installer
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	installer := component.NewDefaultInstaller(gitOps, builder, dao)

	// Get registry manager from context (may be nil for some commands)
	regManager := GetRegistryManager(ctx)

	return &core.CommandContext{
		Ctx:        ctx,
		DB:         db,
		DAO:        dao,
		HomeDir:    homeDir,
		Installer:  installer,
		RegManager: regManager,
	}, nil
}

// getGibsonHome returns the Gibson home directory.
func getGibsonHome() (string, error) {
	homeDir := os.Getenv("GIBSON_HOME")
	if homeDir == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		homeDir = userHome + "/.gibson"
	}
	return homeDir, nil
}

// GetRegistryManager retrieves the registry manager from the context.
// Returns nil if the manager is not present in the context.
func GetRegistryManager(ctx context.Context) *registry.Manager {
	if m, ok := ctx.Value(RegistryManagerKey{}).(*registry.Manager); ok {
		return m
	}
	return nil
}

// RegistryManagerKey is the context key for storing the registry manager.
// This is exported so that the main package can use the same key.
type RegistryManagerKey struct{}

// WithRegistryManager returns a new context with the registry manager attached.
func WithRegistryManager(ctx context.Context, m *registry.Manager) context.Context {
	return context.WithValue(ctx, RegistryManagerKey{}, m)
}
