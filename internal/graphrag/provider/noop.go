package provider

import (
	"context"

	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/types"
)

// NoopGraphRAGProvider implements a no-op GraphRAG provider for when GraphRAG is disabled.
// All operations complete successfully with minimal overhead but return empty results.
//
// Use cases:
// - GraphRAG feature disabled in configuration
// - Development/testing without GraphRAG dependencies
// - Graceful degradation when GraphRAG is unavailable
//
// Characteristics:
// - Zero initialization overhead
// - All operations are no-ops that return immediately
// - No external dependencies or network calls
// - Safe for concurrent access (stateless)
//
// Thread-safety: Safe for concurrent access (all methods are read-only no-ops).
type NoopGraphRAGProvider struct{}

// NewNoopProvider creates a new NoopGraphRAGProvider.
// This provider is always ready to use without initialization.
func NewNoopProvider() *NoopGraphRAGProvider {
	return &NoopGraphRAGProvider{}
}

// Initialize is a no-op for the noop provider.
// Always returns nil immediately.
func (n *NoopGraphRAGProvider) Initialize(ctx context.Context) error {
	return nil
}

// StoreNode is a no-op that accepts any node without storing it.
// Returns nil immediately without validation or storage.
func (n *NoopGraphRAGProvider) StoreNode(ctx context.Context, node graphrag.GraphNode) error {
	return nil
}

// StoreRelationship is a no-op that accepts any relationship without storing it.
// Returns nil immediately without validation or storage.
func (n *NoopGraphRAGProvider) StoreRelationship(ctx context.Context, rel graphrag.Relationship) error {
	return nil
}

// QueryNodes returns an empty slice for any query.
// Always returns successfully with no results.
func (n *NoopGraphRAGProvider) QueryNodes(ctx context.Context, query graphrag.NodeQuery) ([]graphrag.GraphNode, error) {
	return []graphrag.GraphNode{}, nil
}

// QueryRelationships returns an empty slice for any query.
// Always returns successfully with no results.
func (n *NoopGraphRAGProvider) QueryRelationships(ctx context.Context, query graphrag.RelQuery) ([]graphrag.Relationship, error) {
	return []graphrag.Relationship{}, nil
}

// TraverseGraph returns an empty slice for any traversal.
// Always returns successfully with no results.
func (n *NoopGraphRAGProvider) TraverseGraph(ctx context.Context, startID string, maxHops int, filters graphrag.TraversalFilters) ([]graphrag.GraphNode, error) {
	return []graphrag.GraphNode{}, nil
}

// VectorSearch returns an empty slice for any vector search.
// Always returns successfully with no results.
func (n *NoopGraphRAGProvider) VectorSearch(ctx context.Context, embedding []float64, topK int, filters map[string]any) ([]graphrag.VectorResult, error) {
	return []graphrag.VectorResult{}, nil
}

// Health always returns healthy status.
// The noop provider is always operational as it has no dependencies.
func (n *NoopGraphRAGProvider) Health(ctx context.Context) types.HealthStatus {
	return types.Healthy("noop provider (GraphRAG disabled)")
}

// Close is a no-op as there are no resources to release.
// Always returns nil immediately.
func (n *NoopGraphRAGProvider) Close() error {
	return nil
}
