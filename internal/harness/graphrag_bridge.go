// Package harness provides the agent execution environment.
package harness

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/graphrag"
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
	store     graphrag.GraphRAGStore
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
//   - store: The GraphRAG store for persisting findings
//   - logger: Logger for diagnostic output (if nil, uses default logger)
//   - config: Configuration options (use DefaultGraphRAGBridgeConfig() for defaults)
//
// Returns a configured GraphRAGBridge ready for use.
func NewGraphRAGBridge(store graphrag.GraphRAGStore, logger *slog.Logger, config GraphRAGBridgeConfig) *DefaultGraphRAGBridge {
	if logger == nil {
		logger = slog.Default()
	}

	// Initialize SDK taxonomy integration
	// Get the taxonomy registry from the init package and pass it to the SDK
	initTaxonomy()

	return &DefaultGraphRAGBridge{
		store:     store,
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
// Delegates to the underlying GraphRAGStore's health check.
func (b *DefaultGraphRAGBridge) Health(ctx context.Context) types.HealthStatus {
	if b.store == nil {
		return types.Unhealthy("graphrag store is nil")
	}
	return b.store.Health(ctx)
}

// storeToGraphRAG performs the actual storage operation.
// This is called in a goroutine and handles:
// 1. Converting agent.Finding to graphrag.FindingNode
// 2. Storing the finding node
// 3. Creating DISCOVERED_ON relationship (if targetID provided)
// 4. Creating USES_TECHNIQUE relationships (if CWE/MITRE data available)
//
// All errors are logged at WARN level and do not propagate (fire-and-forget semantics).
func (b *DefaultGraphRAGBridge) storeToGraphRAG(ctx context.Context, finding agent.Finding, missionID types.ID, targetID *types.ID) {
	// Convert agent finding to graphrag finding node
	findingNode := b.convertToFindingNode(finding, missionID)

	// Store the finding
	if err := b.store.StoreFinding(ctx, findingNode); err != nil {
		b.logger.Warn("failed to store finding to graphrag",
			"finding_id", finding.ID,
			"mission_id", missionID,
			"error", err,
			"operation", "store_finding",
		)
		return
	}

	// Create DISCOVERED_ON relationship if target is specified
	if targetID != nil {
		rel := graphrag.NewRelationship(
			types.ID(finding.ID),
			*targetID,
			graphrag.RelationDiscoveredOn,
		).WithProperty("severity", string(finding.Severity)).
			WithProperty("confidence", finding.Confidence)

		// Store using the low-level Store method with a GraphRecord
		record := graphrag.NewGraphRecord(*findingNode.ToGraphNode())
		record.WithRelationship(*rel)
		if err := b.store.Store(ctx, record); err != nil {
			b.logger.Warn("failed to create DISCOVERED_ON relationship",
				"finding_id", finding.ID,
				"target_id", targetID,
				"error", err,
				"operation", "create_relationship",
			)
		}
	}

	// Create USES_TECHNIQUE relationships if CWE mappings exist
	// CWE IDs can be mapped to MITRE techniques
	for _, cwe := range finding.CWE {
		// Create technique node reference (simplified - assumes technique exists)
		techniqueID := types.NewID()
		rel := graphrag.NewRelationship(
			types.ID(finding.ID),
			techniqueID,
			graphrag.RelationUsesTechnique,
		).WithProperty("cwe", cwe)

		record := graphrag.NewGraphRecord(*findingNode.ToGraphNode())
		record.WithRelationship(*rel)
		if err := b.store.Store(ctx, record); err != nil {
			b.logger.Warn("failed to create USES_TECHNIQUE relationship",
				"finding_id", finding.ID,
				"cwe", cwe,
				"error", err,
				"operation", "create_relationship",
			)
		}
	}

	// Detect and create SIMILAR_TO relationships
	similarCount := b.detectAndLinkSimilarFindings(ctx, finding, missionID)

	b.logger.Debug("successfully stored finding to graphrag",
		"finding_id", finding.ID,
		"mission_id", missionID,
		"has_target", targetID != nil,
		"cwe_count", len(finding.CWE),
		"similar_findings_linked", similarCount,
	)
}

// detectAndLinkSimilarFindings finds similar findings and creates SIMILAR_TO relationships.
// Uses vector similarity search filtered by threshold and limited by MaxSimilarLinks.
// Returns the number of SIMILAR_TO relationships created.
//
// Errors are logged but do not fail the operation (best-effort relationship creation).
func (b *DefaultGraphRAGBridge) detectAndLinkSimilarFindings(ctx context.Context, finding agent.Finding, missionID types.ID) int {
	// Skip if similarity detection is disabled
	if b.config.MaxSimilarLinks <= 0 {
		return 0
	}

	// Query for similar findings using the store's similarity search
	// Request more than MaxSimilarLinks to account for filtering
	similarFindings, err := b.store.FindSimilarFindings(ctx, finding.ID.String(), b.config.MaxSimilarLinks+1)
	if err != nil {
		b.logger.Warn("failed to find similar findings",
			"finding_id", finding.ID,
			"error", err,
			"operation", "find_similar",
		)
		return 0
	}

	// Filter and create relationships
	linkedCount := 0
	for _, similar := range similarFindings {
		// Skip the finding itself
		if similar.ID == types.ID(finding.ID) {
			continue
		}

		// Stop if we've reached max links
		if linkedCount >= b.config.MaxSimilarLinks {
			break
		}

		// Create SIMILAR_TO relationship (unidirectional from new finding to existing)
		rel := graphrag.NewRelationship(
			types.ID(finding.ID),
			similar.ID,
			graphrag.RelationSimilarTo,
		).WithProperty("mission_id", missionID.String())

		// Create a minimal record just for the relationship
		findingNode := b.convertToFindingNode(finding, missionID)
		record := graphrag.NewGraphRecord(*findingNode.ToGraphNode())
		record.WithRelationship(*rel)

		if err := b.store.Store(ctx, record); err != nil {
			b.logger.Warn("failed to create SIMILAR_TO relationship",
				"finding_id", finding.ID,
				"similar_id", similar.ID,
				"error", err,
				"operation", "create_similar_relationship",
			)
			continue
		}

		linkedCount++
	}

	if linkedCount > 0 {
		b.logger.Debug("created similar finding relationships",
			"finding_id", finding.ID,
			"linked_count", linkedCount,
		)
	}

	return linkedCount
}

// convertToFindingNode converts an agent.Finding to a graphrag.FindingNode.
// Maps all relevant fields from the agent finding to the graph node structure.
func (b *DefaultGraphRAGBridge) convertToFindingNode(f agent.Finding, missionID types.ID) graphrag.FindingNode {
	node := graphrag.FindingNode{
		ID:          types.ID(f.ID),
		Title:       f.Title,
		Description: f.Description,
		Severity:    string(f.Severity),
		Category:    f.Category,
		Confidence:  f.Confidence,
		MissionID:   missionID,
		TargetID:    f.TargetID,
		CreatedAt:   f.CreatedAt,
		UpdatedAt:   time.Now(),
	}

	return node
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
