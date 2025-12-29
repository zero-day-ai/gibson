package graphrag

import (
	"context"

	"github.com/zero-day-ai/gibson/internal/types"
)

// GraphRAGProvider defines the interface for GraphRAG storage and retrieval operations.
// Implementations provide hybrid graph + vector search capabilities combining
// graph traversal with semantic similarity for contextual retrieval.
//
// Thread-safety: All implementations must be safe for concurrent access.
type GraphRAGProvider interface {
	// Initialize establishes connections to underlying storage systems
	// (graph database, vector store, embeddings service).
	// Must be called before any other operations.
	// Returns an error if initialization fails.
	Initialize(ctx context.Context) error

	// StoreNode stores a graph node with optional embedding vector.
	// Creates or updates the node in both graph and vector stores.
	// Returns an error if storage fails.
	StoreNode(ctx context.Context, node GraphNode) error

	// StoreRelationship creates a relationship between two nodes.
	// Both nodes must exist in the graph before creating the relationship.
	// Returns an error if either node doesn't exist or relationship creation fails.
	StoreRelationship(ctx context.Context, rel Relationship) error

	// QueryNodes performs exact property-based node lookup.
	// Use for retrieving specific nodes by properties rather than similarity search.
	// Returns matching nodes or an error if query fails.
	QueryNodes(ctx context.Context, query NodeQuery) ([]GraphNode, error)

	// QueryRelationships retrieves relationships matching the given criteria.
	// Use for analyzing graph structure and connections between nodes.
	// Returns matching relationships or an error if query fails.
	QueryRelationships(ctx context.Context, query RelQuery) ([]Relationship, error)

	// TraverseGraph performs graph traversal starting from a node.
	// Explores the graph structure up to maxHops depth, applying filters.
	// Used for discovering connected knowledge and attack chains.
	// Returns discovered nodes in traversal order or an error if traversal fails.
	TraverseGraph(ctx context.Context, startID string, maxHops int, filters TraversalFilters) ([]GraphNode, error)

	// VectorSearch performs pure vector similarity search.
	// Finds nodes with embeddings similar to the query embedding.
	// Used for semantic retrieval before graph expansion.
	// Returns results ranked by similarity or an error if search fails.
	VectorSearch(ctx context.Context, embedding []float64, topK int, filters map[string]any) ([]VectorResult, error)

	// Health returns the current health status of the provider.
	// Checks connectivity to underlying storage systems.
	// Use for monitoring and circuit breaker patterns.
	Health(ctx context.Context) types.HealthStatus

	// Close releases all resources and closes connections.
	// Should be called during graceful shutdown.
	// Returns an error if cleanup fails.
	Close() error
}

// RelQuery represents a query for relationships.
// Used to filter relationships by type, node connections, and properties.
type RelQuery struct {
	FromID     *types.ID      `json:"from_id,omitempty"`     // Filter by source node
	ToID       *types.ID      `json:"to_id,omitempty"`       // Filter by target node
	Types      []RelationType `json:"types,omitempty"`       // Filter by relationship types
	Properties map[string]any `json:"properties,omitempty"`  // Filter by relationship properties
	MinWeight  float64        `json:"min_weight,omitempty"`  // Minimum relationship weight
	Limit      int            `json:"limit,omitempty"`       // Maximum results to return
}

// NewRelQuery creates a new RelQuery with default values.
func NewRelQuery() *RelQuery {
	return &RelQuery{
		Properties: make(map[string]any),
		Limit:      100,
	}
}

// WithFromID filters relationships from a specific node.
func (rq *RelQuery) WithFromID(fromID types.ID) *RelQuery {
	rq.FromID = &fromID
	return rq
}

// WithToID filters relationships to a specific node.
func (rq *RelQuery) WithToID(toID types.ID) *RelQuery {
	rq.ToID = &toID
	return rq
}

// WithTypes filters by relationship types.
func (rq *RelQuery) WithTypes(types ...RelationType) *RelQuery {
	rq.Types = types
	return rq
}

// WithProperty adds a property filter.
func (rq *RelQuery) WithProperty(key string, value any) *RelQuery {
	rq.Properties[key] = value
	return rq
}

// WithMinWeight filters by minimum relationship weight.
func (rq *RelQuery) WithMinWeight(minWeight float64) *RelQuery {
	rq.MinWeight = minWeight
	return rq
}

// WithLimit sets the maximum number of results.
func (rq *RelQuery) WithLimit(limit int) *RelQuery {
	rq.Limit = limit
	return rq
}

// ProviderType represents the type of GraphRAG provider.
type ProviderType string

const (
	ProviderTypeLocal  ProviderType = "local"  // Local Neo4j + vector store
	ProviderTypeCloud  ProviderType = "cloud"  // Gibson Cloud GraphRAG API
	ProviderTypeHybrid ProviderType = "hybrid" // Local graph + cloud vector
	ProviderTypeNoop   ProviderType = "noop"   // No-op provider (GraphRAG disabled)
)

// String returns the string representation of ProviderType.
func (pt ProviderType) String() string {
	return string(pt)
}

// IsValid checks if the ProviderType is a valid value.
func (pt ProviderType) IsValid() bool {
	switch pt {
	case ProviderTypeLocal, ProviderTypeCloud, ProviderTypeHybrid, ProviderTypeNoop:
		return true
	default:
		return false
	}
}

// NewGraphRAGProvider creates a new GraphRAGProvider based on the configuration.
// Returns the appropriate provider implementation for the configured provider type.
// Returns an error if the provider type is invalid or initialization fails.
//
// NOTE: To avoid import cycles, the actual factory implementation is in
// internal/graphrag/provider.NewProvider(). This function signature is kept
// for interface documentation purposes.
//
// Usage:
//   provider, err := provider.NewProvider(config)
func NewGraphRAGProvider(config GraphRAGConfig) (GraphRAGProvider, error) {
	// This is a stub - use provider.NewProvider() instead
	return nil, NewConfigError(
		"use provider.NewProvider() instead to avoid import cycles",
		nil,
	)
}
