package plugin

import (
	"context"

	"github.com/zero-day-ai/gibson/internal/types"
)

// Plugin represents a stateful service for external data access
type Plugin interface {
	// Name returns the unique identifier for this plugin
	Name() string

	// Version returns the semantic version of the plugin
	Version() string

	// Initialize prepares the plugin for use with the given configuration
	Initialize(ctx context.Context, cfg PluginConfig) error

	// Shutdown gracefully stops the plugin and releases resources
	Shutdown(ctx context.Context) error

	// Query executes a plugin method with the given parameters
	Query(ctx context.Context, method string, params map[string]any) (any, error)

	// Methods returns the list of available methods this plugin supports
	Methods() []MethodDescriptor

	// Health returns the current health status of the plugin
	Health(ctx context.Context) types.HealthStatus
}
