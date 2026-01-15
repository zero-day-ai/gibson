# Neo4j-Langfuse Correlation Implementation

## Overview

The correlation system provides bidirectional mapping between Neo4j graph nodes and Langfuse observability traces. This enables:

- Clicking a Neo4j graph node → view the corresponding Langfuse trace
- Clicking a Langfuse span → view the corresponding Neo4j graph data
- Cross-referencing execution flow with graph structure

## Architecture

### CorrelationID

A type-safe wrapper around UUID v4 for unique correlation identifiers.

```go
// Generate a new correlation ID
id := observability.GenerateCorrelationID()

// Validate
if err := id.Validate(); err != nil {
    // Handle error
}

// Convert to string
str := id.String()

// Check if zero-valued
if id.IsZero() {
    // Handle empty ID
}
```

### CorrelationStore Interface

The core interface for bidirectional lookups:

```go
type CorrelationStore interface {
    // Store node-span association
    StoreCorrelation(ctx context.Context, nodeID string, spanID string) error

    // Get span ID for a graph node
    GetSpanForNode(ctx context.Context, nodeID string) (string, error)

    // Get node ID for a span
    GetNodeForSpan(ctx context.Context, spanID string) (string, error)
}
```

### Implementations

#### Neo4jCorrelationStore

Production implementation using Neo4j graph queries.

**Storage Strategy:**
- Adds `langfuse_span_id` property to Neo4j nodes
- Adds `Correlation` label for efficient querying
- Creates index on `(Correlation:langfuse_span_id)` for fast lookups

**Usage:**

```go
// Initialize with Neo4j client
store, err := observability.NewNeo4jCorrelationStore(graphClient)
if err != nil {
    return err
}

// Store correlation
err = store.StoreCorrelation(ctx, nodeID, spanID)

// Lookup by node
spanID, err := store.GetSpanForNode(ctx, nodeID)

// Lookup by span
nodeID, err := store.GetNodeForSpan(ctx, spanID)
```

**Neo4j Schema:**

```cypher
// Nodes with correlations have:
// - :Correlation label
// - langfuse_span_id property

// Example node
(:AgentRun:Correlation {
    id: "agent-run-123",
    agent_name: "ssrf-hunter",
    langfuse_span_id: "7c9e6679b17e"
})

// Index for fast lookups
CREATE INDEX correlation_span_id IF NOT EXISTS
FOR (n:Correlation)
ON (n.langfuse_span_id)
```

#### InMemoryCorrelationStore

Test/development implementation using concurrent-safe maps.

**Usage:**

```go
// Initialize
store := observability.NewInMemoryCorrelationStore()

// Store, retrieve (same interface as Neo4j store)
err := store.StoreCorrelation(ctx, "node-1", "span-1")
spanID, err := store.GetSpanForNode(ctx, "node-1")

// Test utilities
store.Clear()            // Remove all correlations
count := store.Count()   // Get correlation count
```

**Thread Safety:**
- Uses `sync.RWMutex` for concurrent access
- Safe for parallel reads and writes

## Integration Points

### 1. GraphSpanProcessor

When spans complete, correlate with graph nodes:

```go
func (p *GraphSpanProcessor) OnEnd(s sdktrace.ReadOnlySpan) {
    spanID := s.SpanContext().SpanID().String()

    // Get the node ID from span attributes
    nodeID := getStringAttribute(s, "gibson.graph.node_id")

    if nodeID != "" {
        // Store correlation
        err := p.correlationStore.StoreCorrelation(ctx, nodeID, spanID)
        if err != nil {
            p.logger.Warn("failed to store correlation", "error", err)
        }
    }
}
```

### 2. TaxonomyGraphEngine

When creating nodes, store correlation:

```go
func (e *taxonomyGraphEngine) HandleEvent(ctx context.Context, eventType string, data map[string]any) error {
    // Create node
    nodeID, err := e.graphClient.CreateNode(ctx, labels, properties)
    if err != nil {
        return err
    }

    // Get span ID from context
    span := trace.SpanFromContext(ctx)
    if span.SpanContext().IsValid() {
        spanID := span.SpanContext().SpanID().String()

        // Store correlation
        if err := e.correlationStore.StoreCorrelation(ctx, nodeID, spanID); err != nil {
            e.logger.Warn("failed to store correlation", "error", err)
        }
    }

    return nil
}
```

### 3. Query Helpers

Retrieve correlated data:

```go
// Get Langfuse trace URL for a graph node
func GetTraceURLForNode(ctx context.Context, store CorrelationStore, nodeID string, langfuseHost string) (string, error) {
    spanID, err := store.GetSpanForNode(ctx, nodeID)
    if err != nil {
        return "", err
    }

    // Construct Langfuse UI URL
    return fmt.Sprintf("%s/spans/%s", langfuseHost, spanID), nil
}

// Get graph node data for a Langfuse span
func GetNodeDataForSpan(ctx context.Context, store CorrelationStore, graphClient graph.GraphClient, spanID string) (map[string]any, error) {
    nodeID, err := store.GetNodeForSpan(ctx, spanID)
    if err != nil {
        return nil, err
    }

    // Query Neo4j for node properties
    query := "MATCH (n) WHERE id(n) = $node_id RETURN properties(n) as props"
    result, err := graphClient.Query(ctx, query, map[string]any{"node_id": nodeID})
    if err != nil {
        return nil, err
    }

    // ... extract properties
}
```

## Error Handling

All operations return `ObservabilityError` with appropriate error codes:

- `ErrSpanContextMissing`: Correlation not found or invalid ID
- `ErrExporterConnection`: Neo4j query failed

```go
spanID, err := store.GetSpanForNode(ctx, nodeID)
if err != nil {
    var obsErr *observability.ObservabilityError
    if errors.As(err, &obsErr) {
        switch obsErr.Code {
        case observability.ErrSpanContextMissing:
            // Correlation doesn't exist
        case observability.ErrExporterConnection:
            // Network/database error
        }
    }
}
```

## Performance Considerations

### Neo4jCorrelationStore

**Writes:**
- Single MATCH + SET query per correlation
- O(log n) with index on node ID
- Typical: < 10ms

**Reads:**
- Indexed lookups on `langfuse_span_id`
- O(1) average case
- Typical: < 5ms

**Index Overhead:**
- Created automatically on initialization
- Minimal storage overhead (~16 bytes per node)
- Updated automatically on writes

### InMemoryCorrelationStore

**All Operations:**
- O(1) hash map lookups
- Typical: < 1µs
- Memory: ~100 bytes per correlation

## Testing

### Unit Tests

```bash
go test ./internal/observability -run TestCorrelation
```

### Integration Tests

```go
func TestCorrelationIntegration(t *testing.T) {
    // Setup Neo4j testcontainer
    neo4jClient := setupTestNeo4j(t)

    // Create correlation store
    store, err := observability.NewNeo4jCorrelationStore(neo4jClient)
    require.NoError(t, err)

    // Create test node
    nodeID, err := neo4jClient.CreateNode(ctx, []string{"Test"}, map[string]any{
        "name": "test-node",
    })
    require.NoError(t, err)

    // Store correlation
    spanID := "test-span-123"
    err = store.StoreCorrelation(ctx, nodeID, spanID)
    require.NoError(t, err)

    // Verify bidirectional lookup
    retrievedSpanID, err := store.GetSpanForNode(ctx, nodeID)
    require.NoError(t, err)
    assert.Equal(t, spanID, retrievedSpanID)

    retrievedNodeID, err := store.GetNodeForSpan(ctx, spanID)
    require.NoError(t, err)
    assert.Equal(t, nodeID, retrievedNodeID)
}
```

### Benchmark Tests

```bash
go test ./internal/observability -bench=BenchmarkCorrelation -benchmem
```

## Migration Guide

### Adding Correlation to Existing Code

1. **Initialize CorrelationStore:**

```go
// During daemon initialization
correlationStore, err := observability.NewNeo4jCorrelationStore(graphClient)
if err != nil {
    return err
}
```

2. **Store Correlations During Graph Operations:**

```go
// In your graph creation code
nodeID, err := graphClient.CreateNode(ctx, labels, props)
if err != nil {
    return err
}

// Get span from context
span := trace.SpanFromContext(ctx)
if span.SpanContext().IsValid() {
    spanID := span.SpanContext().SpanID().String()
    _ = correlationStore.StoreCorrelation(ctx, nodeID, spanID)
}
```

3. **Query Correlations:**

```go
// In your UI/API handlers
spanID, err := correlationStore.GetSpanForNode(ctx, nodeID)
if err == nil {
    // Show Langfuse trace link
    traceURL := fmt.Sprintf("%s/traces/%s", langfuseHost, spanID)
}
```

## Future Enhancements

### Batch Operations

```go
// Store multiple correlations in a single transaction
type Correlation struct {
    NodeID string
    SpanID string
}

func (s *Neo4jCorrelationStore) StoreBatch(ctx context.Context, correlations []Correlation) error {
    // Single Neo4j transaction with multiple SET operations
}
```

### Correlation Metadata

```go
// Store additional metadata with correlations
func (s *Neo4jCorrelationStore) StoreCorrelationWithMetadata(
    ctx context.Context,
    nodeID, spanID string,
    metadata map[string]any,
) error {
    // Add metadata as additional node properties
}
```

### Correlation Cleanup

```go
// Remove old correlations for nodes that no longer exist
func (s *Neo4jCorrelationStore) PruneStaleCorrelations(ctx context.Context, maxAge time.Duration) error {
    // Delete correlations for deleted nodes
}
```

## See Also

- [OpenTelemetry Tracing](tracing.go)
- [Langfuse Export](langfuse.go)
- [Graph Span Processor](graph_processor.go)
- [Neo4j Client](../graphrag/graph/neo4j.go)
