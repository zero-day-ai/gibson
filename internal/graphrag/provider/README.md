# GraphRAG Provider Package

This package implements the provider layer for GraphRAG (Graph Retrieval Augmented Generation) in Gibson.

## Overview

The provider package contains implementations of the `GraphRAGProvider` interface, which abstracts over different graph database and vector search backends.

## Architecture

```
GraphRAGProvider (interface in ../provider.go)
├── LocalGraphRAGProvider (local.go)
│   ├── Neo4j for graph operations
│   └── Local vector store for embeddings
├── CloudGraphRAGProvider (cloud.go)
│   └── Gibson Cloud API for both graph and vector
├── HybridGraphRAGProvider (hybrid.go)
│   ├── Local Neo4j for graph
│   └── Cloud API for vector search
└── NoopGraphRAGProvider (noop.go)
    └── Disabled GraphRAG (returns empty results)
```

## Files

- `factory.go` - Factory function to create providers based on configuration
- `local.go` - Local provider using Neo4j + vector store
- `cloud.go` - Cloud provider using Gibson Cloud API
- `hybrid.go` - Hybrid provider (local graph + cloud vector)
- `noop.go` - No-op provider for when GraphRAG is disabled
- `local_test.go` - Tests for local provider

## Usage

```go
import "github.com/zero-day-ai/gibson/internal/graphrag/provider"

// Create provider from configuration
config := graphrag.GraphRAGConfig{
    Enabled:  true,
    Provider: "neo4j",
    Neo4j: graphrag.Neo4jConfig{
        URI:      "bolt://localhost:7687",
        Username: "neo4j",
        Password: "password",
    },
    // ... other config
}

provider, err := provider.NewProvider(config)
if err != nil {
    return err
}

// Initialize connections
if err := provider.Initialize(ctx); err != nil {
    return err
}
defer provider.Close()

// Store nodes
node := graphrag.NewGraphNode(nodeID, graphrag.NodeTypeFinding)
if err := provider.StoreNode(ctx, *node); err != nil {
    return err
}

// Query nodes
query := graphrag.NewNodeQuery().WithNodeTypes(graphrag.NodeTypeFinding)
nodes, err := provider.QueryNodes(ctx, *query)

// Vector search
results, err := provider.VectorSearch(ctx, embedding, 10, filters)

// Graph traversal
nodes, err := provider.TraverseGraph(ctx, startID, 3, filters)
```

## Provider Types

### Local Provider

Uses local Neo4j for graph operations and a local vector store for embeddings.

**Benefits:**
- Full control over data
- No cloud dependencies
- Lower latency

**Requires:**
- Neo4j instance
- Local vector store (injected via SetVectorStore)

### Cloud Provider

Routes all operations to Gibson Cloud GraphRAG API.

**Benefits:**
- Managed infrastructure
- Automatic scaling
- No local database maintenance

**Features:**
- Automatic retry with exponential backoff
- Optional local caching for offline resilience
- Rate limiting support

### Hybrid Provider

Combines local graph (Neo4j) with cloud vector search.

**Benefits:**
- Keep sensitive graph structure local
- Leverage cloud scalability for vector search
- Reduced cloud costs (only embeddings sent)

**Use case:**
- Security-sensitive deployments
- Hybrid cloud architectures

### Noop Provider

Returns empty results for all operations with zero overhead.

**Use cases:**
- GraphRAG feature disabled
- Development/testing without dependencies
- Graceful degradation

## Testing

Run tests:
```bash
go test ./internal/graphrag/provider -v
```

Note: Some tests require valid GraphRAG configuration. Use the `testConfig()` helper function in tests to create valid configurations.

## Implementation Notes

### Fallback Behavior

The local provider implements graceful degradation:
- If Neo4j is unavailable, it falls back to vector-only mode
- Graph operations return errors, but vector search continues to work
- Health check returns "degraded" status

### Thread Safety

All providers are safe for concurrent access:
- Internal locking via sync.RWMutex
- Read locks for queries
- Write locks for initialization/close

### Error Handling

All errors are wrapped with `graphrag.GraphRAGError` which includes:
- Error code for programmatic handling
- Retryable flag for transient failures
- Context for debugging

## Future Enhancements

1. Connection pooling improvements
2. More sophisticated caching strategies
3. Batch operations for bulk inserts
4. Streaming results for large datasets
5. Support for additional graph databases (Neptune, Memgraph)
