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

	// Open database (still used for non-component data: targets, missions, etc.)
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Get registry manager from context (required for ComponentStore)
	regManager := GetRegistryManager(ctx)
	if regManager == nil {
		db.Close()
		return nil, fmt.Errorf("registry manager not available - ensure daemon is running")
	}

	// Create ComponentStore from registry's etcd client
	etcdClient := regManager.Client()
	if etcdClient == nil {
		db.Close()
		return nil, fmt.Errorf("etcd client not available - ensure registry is started")
	}
	componentStore := component.EtcdComponentStore(etcdClient, regManager.Namespace())

	// Create installer with ComponentStore
	gitOps := git.NewDefaultGitOperations()
	builder := build.NewDefaultBuildExecutor()
	lifecycle := component.NewLifecycleManager(componentStore, nil)
	installer := component.NewDefaultInstaller(gitOps, builder, componentStore, lifecycle)

	return &core.CommandContext{
		Ctx:            ctx,
		DB:             db,
		ComponentStore: componentStore,
		HomeDir:        homeDir,
		Installer:      installer,
		RegManager:     regManager,
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

// CallbackManagerKey is the context key for storing the callback manager.
// This is exported so that the main package can use the same key.
type CallbackManagerKey struct{}

// GetCallbackManager retrieves the callback manager from the context.
// Returns nil if the manager is not present in the context.
func GetCallbackManager(ctx context.Context) interface{} {
	return ctx.Value(CallbackManagerKey{})
}

// WithCallbackManager returns a new context with the callback manager attached.
func WithCallbackManager(ctx context.Context, m interface{}) context.Context {
	return context.WithValue(ctx, CallbackManagerKey{}, m)
}

// DaemonClientKey is the context key for storing the daemon client.
// This is exported so that the main package can use the same key.
type DaemonClientKey struct{}

// GetDaemonClient retrieves the daemon client from the context.
// Returns nil if the client is not present in the context.
func GetDaemonClient(ctx context.Context) interface{} {
	return ctx.Value(DaemonClientKey{})
}

// WithDaemonClient returns a new context with the daemon client attached.
func WithDaemonClient(ctx context.Context, client interface{}) context.Context {
	return context.WithValue(ctx, DaemonClientKey{}, client)
}
