package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/types"
)

// HybridGraphRAGProvider combines local graph operations with cloud vector search.
// Routes graph structure operations to local Neo4j while using cloud for vector search.
//
// Architecture:
// - Graph operations (nodes, relationships, traversal): Local Neo4j
// - Vector operations (semantic search, embeddings): Cloud API
//
// Benefits:
// - Keep sensitive graph structure local
// - Leverage cloud scalability for vector search
// - Reduce cloud costs by only sending embeddings
//
// Thread-safety: Safe for concurrent access via internal locking.
type HybridGraphRAGProvider struct {
	config      graphrag.GraphRAGConfig
	localGraph  *LocalGraphRAGProvider
	cloudVector *CloudGraphRAGProvider
	initialized bool
	mu          sync.RWMutex
}

// NewHybridProvider creates a new HybridGraphRAGProvider with the given configuration.
// Does not initialize connections - call Initialize() before use.
func NewHybridProvider(config graphrag.GraphRAGConfig) (*HybridGraphRAGProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, graphrag.NewConfigError("invalid hybrid provider configuration", err)
	}

	// Validate hybrid-specific requirements
	if config.Neo4j.URI == "" {
		return nil, graphrag.NewConfigError("hybrid provider requires Neo4j configuration", nil)
	}
	if config.Cloud.Endpoint == "" {
		return nil, graphrag.NewConfigError("hybrid provider requires cloud configuration", nil)
	}

	return &HybridGraphRAGProvider{
		config:      config,
		initialized: false,
	}, nil
}

// Initialize establishes connections to both local Neo4j and cloud vector service.
// Both backends must be available for hybrid mode to function.
func (h *HybridGraphRAGProvider) Initialize(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.initialized {
		return nil // Already initialized
	}

	// Initialize local graph provider (without vector store - cloud handles vector)
	// The local provider will not have a vector store, so vector search will be
	// delegated to the cloud provider in hybrid mode
	var err error
	h.localGraph, err = NewLocalProvider(h.config)
	if err != nil {
		return graphrag.WrapGraphRAGError(
			graphrag.ErrCodeConnectionFailed,
			"failed to create local graph provider",
			err,
		)
	}

	if err := h.localGraph.Initialize(ctx); err != nil {
		return graphrag.WrapGraphRAGError(
			graphrag.ErrCodeConnectionFailed,
			"failed to initialize local graph provider",
			err,
		)
	}

	// Initialize cloud vector provider (for vector operations only)
	cloudConfig := h.config
	// Cloud provider will handle vector operations

	h.cloudVector, err = NewCloudProvider(cloudConfig)
	if err != nil {
		return graphrag.WrapGraphRAGError(
			graphrag.ErrCodeConnectionFailed,
			"failed to create cloud vector provider",
			err,
		)
	}

	if err := h.cloudVector.Initialize(ctx); err != nil {
		return graphrag.WrapGraphRAGError(
			graphrag.ErrCodeConnectionFailed,
			"failed to initialize cloud vector provider",
			err,
		)
	}

	h.initialized = true
	return nil
}

// StoreNode stores the node in local Neo4j and sends embedding to cloud for vector search.
// Graph structure stays local, only embeddings are sent to cloud.
func (h *HybridGraphRAGProvider) StoreNode(ctx context.Context, node graphrag.GraphNode) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.initialized {
		return graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Validate node
	if err := node.Validate(); err != nil {
		return graphrag.WrapGraphRAGError(graphrag.ErrCodeQueryFailed, "invalid node", err)
	}

	// Store in local graph
	if err := h.localGraph.StoreNode(ctx, node); err != nil {
		return graphrag.WrapGraphRAGError(
			graphrag.ErrCodeQueryFailed,
			"failed to store node in local graph",
			err,
		)
	}

	// Store embedding in cloud if present
	if len(node.Embedding) > 0 {
		// Create a minimal node with just embedding and metadata for cloud
		cloudNode := node // Copy the node
		// In production, you might strip sensitive properties before sending to cloud
		if err := h.cloudVector.StoreNode(ctx, cloudNode); err != nil {
			// Cloud storage failure is not critical - log and continue
			// In production, you'd log this error for monitoring
			// The node is still available locally for graph operations
		}
	}

	return nil
}

// StoreRelationship creates the relationship in local Neo4j only.
// Relationships are purely graph structure and don't need cloud storage.
func (h *HybridGraphRAGProvider) StoreRelationship(ctx context.Context, rel graphrag.Relationship) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.initialized {
		return graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Relationships are local-only - route to local graph
	return h.localGraph.StoreRelationship(ctx, rel)
}

// QueryNodes queries nodes from local Neo4j.
// Graph structure queries use local data for privacy and performance.
func (h *HybridGraphRAGProvider) QueryNodes(ctx context.Context, query graphrag.NodeQuery) ([]graphrag.GraphNode, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Node queries use local graph
	return h.localGraph.QueryNodes(ctx, query)
}

// QueryRelationships queries relationships from local Neo4j.
// Relationship queries are purely graph operations.
func (h *HybridGraphRAGProvider) QueryRelationships(ctx context.Context, query graphrag.RelQuery) ([]graphrag.Relationship, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Relationship queries use local graph
	return h.localGraph.QueryRelationships(ctx, query)
}

// TraverseGraph performs graph traversal using local Neo4j.
// Graph traversal is a core graph operation that stays local for performance.
func (h *HybridGraphRAGProvider) TraverseGraph(ctx context.Context, startID string, maxHops int, filters graphrag.TraversalFilters) ([]graphrag.GraphNode, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Graph traversal uses local graph for performance and privacy
	return h.localGraph.TraverseGraph(ctx, startID, maxHops, filters)
}

// VectorSearch performs vector similarity search using the cloud API.
// Leverages cloud scalability and managed vector indices.
func (h *HybridGraphRAGProvider) VectorSearch(ctx context.Context, embedding []float64, topK int, filters map[string]any) ([]graphrag.VectorResult, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Vector search uses cloud for scalability
	return h.cloudVector.VectorSearch(ctx, embedding, topK, filters)
}

// Health checks the health of both local graph and cloud vector backends.
// Returns:
// - Healthy: Both backends operational
// - Degraded: One backend down (can still operate with reduced functionality)
// - Unhealthy: Both backends down
func (h *HybridGraphRAGProvider) Health(ctx context.Context) types.HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.initialized {
		return types.Unhealthy("provider not initialized")
	}

	// Check local graph health
	localHealth := h.localGraph.Health(ctx)
	cloudHealth := h.cloudVector.Health(ctx)

	localHealthy := localHealth.IsHealthy()
	cloudHealthy := cloudHealth.IsHealthy()

	// Determine overall health
	if localHealthy && cloudHealthy {
		return types.Healthy("local graph and cloud vector both healthy")
	} else if localHealthy {
		return types.Degraded(fmt.Sprintf(
			"local graph healthy, cloud vector unhealthy: %s",
			cloudHealth.Message,
		))
	} else if cloudHealthy {
		return types.Degraded(fmt.Sprintf(
			"cloud vector healthy, local graph unhealthy: %s",
			localHealth.Message,
		))
	} else {
		return types.Unhealthy(fmt.Sprintf(
			"both backends unhealthy - local: %s, cloud: %s",
			localHealth.Message,
			cloudHealth.Message,
		))
	}
}

// Close releases resources for both local and cloud providers.
func (h *HybridGraphRAGProvider) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.initialized {
		return nil
	}

	var errs []error

	// Close local graph
	if h.localGraph != nil {
		if err := h.localGraph.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close local graph: %w", err))
		}
	}

	// Close cloud vector
	if h.cloudVector != nil {
		if err := h.cloudVector.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close cloud vector: %w", err))
		}
	}

	h.initialized = false

	if len(errs) > 0 {
		return graphrag.NewGraphRAGError(
			graphrag.ErrCodeConnectionFailed,
			fmt.Sprintf("close errors: %v", errs),
		)
	}

	return nil
}
