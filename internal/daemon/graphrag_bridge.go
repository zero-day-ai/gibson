package daemon

import (
	"context"
	"log/slog"

	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/harness"
)

// GraphRAGBridgeAdapter adapts a GraphRAG store to provide both GraphRAGBridge
// and GraphRAGQueryBridge interfaces for the agent harness.
//
// This adapter simplifies daemon setup by wrapping the underlying GraphRAG
// infrastructure (provider, store, embeddings) into the interfaces expected
// by the harness configuration.
//
// Architecture:
//   Neo4jClient (graph database) + VectorStore + Embedder
//     -> GraphRAGProvider (local or remote)
//       -> GraphRAGStore (orchestration layer)
//         -> GraphRAGBridge (async storage)
//         -> GraphRAGQueryBridge (query operations)
//
// The adapter holds references to both bridge implementations and provides
// unified lifecycle management (initialization, health checks, shutdown).
type GraphRAGBridgeAdapter struct {
	store       graphrag.GraphRAGStore
	bridge      harness.GraphRAGBridge
	queryBridge harness.GraphRAGQueryBridge
	logger      *slog.Logger
}

// GraphRAGBridgeConfig holds configuration for creating a GraphRAGBridgeAdapter.
// It encapsulates all dependencies needed to set up the GraphRAG system.
type GraphRAGBridgeConfig struct {
	// Neo4jClient provides graph database connectivity.
	// Required for storing nodes and relationships.
	Neo4jClient *graph.Neo4jClient

	// GraphRAGStore provides the high-level GraphRAG operations.
	// This is typically a DefaultGraphRAGStore that wraps a provider.
	// Required - this is the core GraphRAG storage layer.
	GraphRAGStore graphrag.GraphRAGStore

	// Logger for diagnostic output.
	// Optional - defaults to slog.Default() if nil.
	Logger *slog.Logger

	// BridgeConfig controls async storage behavior.
	// Optional - defaults to DefaultGraphRAGBridgeConfig() if nil.
	BridgeConfig *harness.GraphRAGBridgeConfig
}

// NewGraphRAGBridgeAdapter creates a new adapter that provides both GraphRAGBridge
// and GraphRAGQueryBridge interfaces for use in harness configuration.
//
// This is the recommended way to set up GraphRAG integration in the daemon.
// It creates both bridge types with shared underlying storage.
//
// Parameters:
//   - config: Configuration containing all required dependencies
//
// Returns:
//   - *GraphRAGBridgeAdapter: Ready-to-use adapter with both bridge interfaces
//   - error: If configuration is invalid
//
// Example usage in daemon:
//   adapter, err := daemon.NewGraphRAGBridgeAdapter(config)
//   if err != nil {
//       return err
//   }
//   harnessConfig := &harness.HarnessConfig{
//       GraphRAGBridge:      adapter.Bridge(),
//       GraphRAGQueryBridge: adapter.QueryBridge(),
//       // ... other config
//   }
func NewGraphRAGBridgeAdapter(config GraphRAGBridgeConfig) (*GraphRAGBridgeAdapter, error) {
	// Validate required fields
	if config.GraphRAGStore == nil {
		return nil, &ConfigValidationError{
			Field:   "GraphRAGStore",
			Message: "cannot be nil",
		}
	}

	// Default logger if not provided
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "graphrag_adapter")

	// Default bridge config if not provided
	bridgeConfig := harness.DefaultGraphRAGBridgeConfig()
	if config.BridgeConfig != nil {
		bridgeConfig = *config.BridgeConfig
	}

	// Validate bridge config
	if err := bridgeConfig.Validate(); err != nil {
		return nil, &ConfigValidationError{
			Field:   "BridgeConfig",
			Message: err.Error(),
		}
	}

	// Create the storage bridge (async writes)
	bridge := harness.NewGraphRAGBridge(
		config.GraphRAGStore,
		logger.With("bridge_type", "storage"),
		bridgeConfig,
	)

	// Create the query bridge (reads and queries)
	queryBridge := harness.NewGraphRAGQueryBridge(config.GraphRAGStore)

	adapter := &GraphRAGBridgeAdapter{
		store:       config.GraphRAGStore,
		bridge:      bridge,
		queryBridge: queryBridge,
		logger:      logger,
	}

	logger.Info("graphrag bridge adapter created",
		"bridge_enabled", bridgeConfig.Enabled,
		"max_concurrent", bridgeConfig.MaxConcurrent,
		"similarity_threshold", bridgeConfig.SimilarityThreshold,
	)

	return adapter, nil
}

// Bridge returns the GraphRAGBridge interface for async storage operations.
// This is used for storing findings discovered during agent execution.
func (a *GraphRAGBridgeAdapter) Bridge() harness.GraphRAGBridge {
	return a.bridge
}

// QueryBridge returns the GraphRAGQueryBridge interface for query operations.
// This is used for agents to query the knowledge graph during execution.
func (a *GraphRAGBridgeAdapter) QueryBridge() harness.GraphRAGQueryBridge {
	return a.queryBridge
}

// Store returns the underlying GraphRAGStore for advanced operations.
// Most callers should use Bridge() and QueryBridge() instead.
func (a *GraphRAGBridgeAdapter) Store() graphrag.GraphRAGStore {
	return a.store
}

// Health checks the health of the GraphRAG system.
// Delegates to the underlying store's health check.
func (a *GraphRAGBridgeAdapter) Health(ctx context.Context) error {
	status := a.store.Health(ctx)
	if !status.IsHealthy() {
		return &HealthCheckError{
			Component: "graphrag_store",
			Message:   status.Message,
		}
	}
	return nil
}

// Shutdown gracefully shuts down the GraphRAG bridge.
// Waits for pending async storage operations to complete.
// Should be called during daemon shutdown.
func (a *GraphRAGBridgeAdapter) Shutdown(ctx context.Context) error {
	a.logger.Info("shutting down graphrag bridge adapter")

	// Shutdown the storage bridge (waits for pending writes)
	if err := a.bridge.Shutdown(ctx); err != nil {
		a.logger.Warn("graphrag bridge shutdown incomplete",
			"error", err,
		)
		return err
	}

	// Close the underlying store
	if err := a.store.Close(); err != nil {
		a.logger.Warn("graphrag store close failed",
			"error", err,
		)
		return err
	}

	a.logger.Info("graphrag bridge adapter shutdown complete")
	return nil
}

// ConfigValidationError represents a configuration validation error.
type ConfigValidationError struct {
	Field   string
	Message string
}

func (e *ConfigValidationError) Error() string {
	return "graphrag adapter config: " + e.Field + " " + e.Message
}

// HealthCheckError represents a health check failure.
type HealthCheckError struct {
	Component string
	Message   string
}

func (e *HealthCheckError) Error() string {
	return "graphrag health check failed: " + e.Component + ": " + e.Message
}
