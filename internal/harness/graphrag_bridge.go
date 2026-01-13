// Package harness provides the agent execution environment.
package harness

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/graphrag/engine"
	taxonomyinit "github.com/zero-day-ai/gibson/internal/init"
	"github.com/zero-day-ai/gibson/internal/types"
	sdkgraphrag "github.com/zero-day-ai/sdk/graphrag"
)

// GraphRAGBridge defines the interface for storing findings to the GraphRAG
// knowledge graph system. Implementations handle the conversion of agent findings
// to graph nodes and the creation of relationships.
//
// The bridge operates asynchronously to avoid blocking agent execution. All
// storage operations happen in background goroutines, with the bridge tracking
// pending operations for graceful shutdown.
type GraphRAGBridge interface {
	// StoreAsync queues a finding for asynchronous storage to GraphRAG.
	// This method returns immediately; actual storage happens in a background
	// goroutine. The finding will be converted to a FindingNode and stored
	// along with any relevant relationships (DISCOVERED_ON, USES_TECHNIQUE,
	// SIMILAR_TO).
	//
	// Parameters:
	//   - ctx: Context for the operation (used for logging, not cancellation of async work)
	//   - finding: The agent finding to store
	//   - missionID: The mission this finding belongs to
	//   - targetID: Optional target ID for DISCOVERED_ON relationship
	StoreAsync(ctx context.Context, finding agent.Finding, missionID types.ID, targetID *types.ID)

	// Shutdown waits for all pending storage operations to complete.
	// This should be called when the harness is closing to ensure no findings
	// are lost. The context can be used to set a timeout for the wait.
	//
	// Returns an error if the shutdown times out or encounters issues.
	Shutdown(ctx context.Context) error

	// Health returns the health status of the GraphRAG bridge.
	// This includes the health of the underlying GraphRAGStore.
	Health(ctx context.Context) types.HealthStatus
}

// GraphRAGBridgeConfig holds configuration options for the GraphRAG bridge.
// All fields have sensible defaults that can be overridden.
type GraphRAGBridgeConfig struct {
	// Enabled controls whether GraphRAG storage is active.
	// When false, the bridge becomes a no-op.
	Enabled bool

	// SimilarityThreshold is the minimum similarity score (0.0-1.0) required
	// for creating SIMILAR_TO relationships between findings.
	// Default: 0.85
	SimilarityThreshold float64

	// MaxSimilarLinks is the maximum number of SIMILAR_TO relationships
	// to create per finding. This bounds the relationship density.
	// Default: 5
	MaxSimilarLinks int

	// MaxConcurrent is the maximum number of concurrent storage operations.
	// This prevents unbounded goroutine growth during high-throughput periods.
	// Default: 10
	MaxConcurrent int

	// StorageTimeout is the timeout for individual storage operations.
	// Operations exceeding this timeout will be cancelled and logged.
	// Default: 30s
	StorageTimeout time.Duration
}

// DefaultGraphRAGBridgeConfig returns a GraphRAGBridgeConfig with sensible defaults.
func DefaultGraphRAGBridgeConfig() GraphRAGBridgeConfig {
	return GraphRAGBridgeConfig{
		Enabled:             true,
		SimilarityThreshold: 0.85,
		MaxSimilarLinks:     5,
		MaxConcurrent:       10,
		StorageTimeout:      30 * time.Second,
	}
}

// Validate checks that the configuration values are within acceptable ranges.
func (c GraphRAGBridgeConfig) Validate() error {
	if c.SimilarityThreshold < 0.0 || c.SimilarityThreshold > 1.0 {
		return &ConfigError{
			Field:   "SimilarityThreshold",
			Message: "must be between 0.0 and 1.0",
		}
	}
	if c.MaxSimilarLinks < 0 {
		return &ConfigError{
			Field:   "MaxSimilarLinks",
			Message: "must be non-negative",
		}
	}
	if c.MaxConcurrent < 1 {
		return &ConfigError{
			Field:   "MaxConcurrent",
			Message: "must be at least 1",
		}
	}
	if c.StorageTimeout < time.Second {
		return &ConfigError{
			Field:   "StorageTimeout",
			Message: "must be at least 1 second",
		}
	}
	return nil
}

// ConfigError represents a configuration validation error.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "graphrag bridge config: " + e.Field + " " + e.Message
}

// DefaultGraphRAGBridge is the default implementation of GraphRAGBridge.
// It handles async storage of findings to the GraphRAG knowledge graph,
// with bounded concurrency and graceful shutdown support.
type DefaultGraphRAGBridge struct {
	engine    engine.TaxonomyGraphEngine
	logger    *slog.Logger
	config    GraphRAGBridgeConfig
	wg        sync.WaitGroup
	semaphore chan struct{}
}

// NewGraphRAGBridge creates a new DefaultGraphRAGBridge with the given dependencies.
// The semaphore channel is initialized with size MaxConcurrent to limit concurrent operations.
//
// This function also initializes the SDK's taxonomy integration by calling SetTaxonomy
// with the global taxonomy registry from the init package.
//
// Parameters:
//   - engine: The TaxonomyGraphEngine for taxonomy-driven graph operations
//   - logger: Logger for diagnostic output (if nil, uses default logger)
//   - config: Configuration options (use DefaultGraphRAGBridgeConfig() for defaults)
//
// Returns a configured GraphRAGBridge ready for use.
func NewGraphRAGBridge(engine engine.TaxonomyGraphEngine, logger *slog.Logger, config GraphRAGBridgeConfig) *DefaultGraphRAGBridge {
	if logger == nil {
		logger = slog.Default()
	}

	// Initialize SDK taxonomy integration
	// Get the taxonomy registry from the init package and pass it to the SDK
	initTaxonomy()

	return &DefaultGraphRAGBridge{
		engine:    engine,
		logger:    logger.With("component", "graphrag_bridge"),
		config:    config,
		semaphore: make(chan struct{}, config.MaxConcurrent),
	}
}

// StoreAsync queues a finding for asynchronous storage to GraphRAG.
// It acquires a semaphore slot, increments the WaitGroup, and spawns a goroutine
// to handle the actual storage. The method returns immediately without blocking.
//
// If the bridge is disabled via config, this is a no-op.
func (b *DefaultGraphRAGBridge) StoreAsync(ctx context.Context, finding agent.Finding, missionID types.ID, targetID *types.ID) {
	if !b.config.Enabled {
		return
	}

	// Increment WaitGroup before acquiring semaphore to ensure Shutdown tracks this operation
	b.wg.Add(1)

	// Spawn goroutine for async storage
	go func() {
		defer b.wg.Done()

		// Acquire semaphore (blocks if at max concurrency)
		select {
		case b.semaphore <- struct{}{}:
			// Acquired semaphore slot
		case <-ctx.Done():
			b.logger.Warn("context cancelled while waiting for semaphore",
				"finding_id", finding.ID,
				"mission_id", missionID,
			)
			return
		}
		defer func() { <-b.semaphore }() // Release semaphore slot

		// Create a timeout context for the storage operation
		storageCtx, cancel := context.WithTimeout(context.Background(), b.config.StorageTimeout)
		defer cancel()

		// Perform the storage operation
		b.storeToGraphRAG(storageCtx, finding, missionID, targetID)
	}()
}

// Shutdown waits for all pending storage operations to complete.
// Uses a done channel to implement timeout via the provided context.
// Returns an error if the context deadline is exceeded before all operations complete.
func (b *DefaultGraphRAGBridge) Shutdown(ctx context.Context) error {
	// Create a done channel to signal when WaitGroup completes
	done := make(chan struct{})

	go func() {
		b.wg.Wait()
		close(done)
	}()

	// Wait for either completion or context cancellation
	select {
	case <-done:
		b.logger.Debug("graphrag bridge shutdown complete")
		return nil
	case <-ctx.Done():
		b.logger.Warn("graphrag bridge shutdown timed out, some operations may be incomplete")
		return ctx.Err()
	}
}

// Health returns the health status of the GraphRAG bridge.
// Delegates to the underlying TaxonomyGraphEngine's health check.
func (b *DefaultGraphRAGBridge) Health(ctx context.Context) types.HealthStatus {
	if b.engine == nil {
		return types.Unhealthy("graphrag engine is nil")
	}

	engineHealth := b.engine.Health(ctx)
	if !engineHealth.Healthy {
		return types.Unhealthy(engineHealth.Message)
	}

	return types.Healthy("graphrag bridge operational")
}

// storeToGraphRAG performs the actual storage operation using the TaxonomyGraphEngine.
// This is called in a goroutine and delegates to engine.HandleFinding which:
// 1. Creates Finding node with taxonomy-driven properties
// 2. Creates AFFECTS relationship to target (if provided)
// 3. Creates USES_TECHNIQUE relationships for CWEs
// 4. Creates PART_OF relationship to mission
//
// All errors are logged at WARN level and do not propagate (fire-and-forget semantics).
func (b *DefaultGraphRAGBridge) storeToGraphRAG(ctx context.Context, finding agent.Finding, missionID types.ID, targetID *types.ID) {
	// Use the TaxonomyGraphEngine to handle finding storage
	// The engine handles all node creation and relationships based on taxonomy definitions
	if err := b.engine.HandleFinding(ctx, finding, missionID.String()); err != nil {
		b.logger.Warn("failed to store finding to graphrag",
			"finding_id", finding.ID,
			"mission_id", missionID,
			"error", err,
			"operation", "handle_finding",
		)
		return
	}

	b.logger.Debug("successfully stored finding to graphrag",
		"finding_id", finding.ID,
		"mission_id", missionID,
		"has_target", targetID != nil,
		"cwe_count", len(finding.CWE),
	)
}


// Compile-time interface check for DefaultGraphRAGBridge
var _ GraphRAGBridge = (*DefaultGraphRAGBridge)(nil)

// NoopGraphRAGBridge is a no-op implementation of GraphRAGBridge.
// Use this when GraphRAG is not configured or disabled.
// All methods return immediately with zero overhead.
type NoopGraphRAGBridge struct{}

// Compile-time interface check for NoopGraphRAGBridge
var _ GraphRAGBridge = (*NoopGraphRAGBridge)(nil)

// StoreAsync is a no-op that returns immediately.
func (n *NoopGraphRAGBridge) StoreAsync(_ context.Context, _ agent.Finding, _ types.ID, _ *types.ID) {
	// No-op: do nothing
}

// Shutdown is a no-op that returns nil immediately.
func (n *NoopGraphRAGBridge) Shutdown(_ context.Context) error {
	return nil
}

// Health returns a healthy status since no-op bridge has no failure modes.
func (n *NoopGraphRAGBridge) Health(_ context.Context) types.HealthStatus {
	return types.Healthy("graphrag bridge disabled (noop)")
}

// initTaxonomy initializes the SDK's taxonomy integration by passing the
// Gibson taxonomy registry to the SDK. This enables agents to validate
// node and relationship types against the canonical taxonomy.
//
// This function is called once during GraphRAGBridge initialization.
// It's safe to call multiple times (the SDK maintains a global instance).
func initTaxonomy() {
	// Get the taxonomy registry from Gibson's init package
	registry := taxonomyinit.GetTaxonomyRegistry()

	// Pass it to the SDK for agent access
	// The registry implements the SDK's TaxonomyReader interface
	if registry != nil {
		sdkgraphrag.SetTaxonomy(registry)
	}
}
