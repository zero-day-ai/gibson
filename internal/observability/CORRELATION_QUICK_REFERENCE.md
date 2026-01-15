# Neo4j-Langfuse Correlation - Quick Reference

## 30-Second Overview

Enable clicking between Neo4j graph nodes and Langfuse traces:

```go
// 1. Initialize
store, _ := observability.NewNeo4jCorrelationStore(graphClient)

// 2. Store correlation when creating nodes
nodeID, _ := graphClient.CreateNode(ctx, labels, props)
span := trace.SpanFromContext(ctx)
spanID := span.SpanContext().SpanID().String()
store.StoreCorrelation(ctx, nodeID, spanID)

// 3. Lookup from graph → trace
spanID, _ := store.GetSpanForNode(ctx, nodeID)
url := fmt.Sprintf("https://langfuse/traces/%s", spanID)

// 4. Lookup from trace → graph
nodeID, _ := store.GetNodeForSpan(ctx, spanID)
```

## API Reference

### CorrelationID

```go
id := observability.GenerateCorrelationID()  // Generate new UUID
str := id.String()                           // Convert to string
zero := id.IsZero()                          // Check if empty
err := id.Validate()                         // Validate format
```

### CorrelationStore Interface

```go
type CorrelationStore interface {
    StoreCorrelation(ctx, nodeID, spanID string) error
    GetSpanForNode(ctx, nodeID string) (string, error)
    GetNodeForSpan(ctx, spanID string) (string, error)
}
```

### Implementations

```go
// Production: Neo4j-backed
store, err := observability.NewNeo4jCorrelationStore(graphClient)

// Testing: In-memory
store := observability.NewInMemoryCorrelationStore()
store.Clear()           // Clear all correlations
count := store.Count()  // Get correlation count
```

## Common Patterns

### Store Correlation on Node Creation

```go
func createNodeWithCorrelation(ctx context.Context, store observability.CorrelationStore, client graph.GraphClient) error {
    // Create node
    nodeID, err := client.CreateNode(ctx, []string{"Agent"}, map[string]any{
        "name": "my-agent",
    })
    if err != nil {
        return err
    }

    // Get span from context
    span := trace.SpanFromContext(ctx)
    if !span.SpanContext().IsValid() {
        return nil // No active span
    }

    // Store correlation
    spanID := span.SpanContext().SpanID().String()
    return store.StoreCorrelation(ctx, nodeID, spanID)
}
```

### Lookup Trace URL for Graph Node

```go
func getTraceURL(ctx context.Context, store observability.CorrelationStore, nodeID, langfuseHost string) (string, error) {
    spanID, err := store.GetSpanForNode(ctx, nodeID)
    if err != nil {
        return "", err
    }

    return fmt.Sprintf("%s/traces/%s", langfuseHost, spanID), nil
}
```

### Query Graph Data for Trace Span

```go
func getNodeData(ctx context.Context, store observability.CorrelationStore, client graph.GraphClient, spanID string) (map[string]any, error) {
    nodeID, err := store.GetNodeForSpan(ctx, spanID)
    if err != nil {
        return nil, err
    }

    query := "MATCH (n) WHERE id(n) = $node_id RETURN properties(n) as props"
    result, err := client.Query(ctx, query, map[string]any{"node_id": nodeID})
    if err != nil {
        return nil, err
    }

    if len(result.Records) == 0 {
        return nil, fmt.Errorf("node not found")
    }

    return result.Records[0]["props"].(map[string]any), nil
}
```

## Error Handling

```go
spanID, err := store.GetSpanForNode(ctx, nodeID)
if err != nil {
    var obsErr *observability.ObservabilityError
    if errors.As(err, &obsErr) {
        switch obsErr.Code {
        case observability.ErrSpanContextMissing:
            // Correlation doesn't exist
            log.Warn("no correlation found", "nodeID", nodeID)
            return
        case observability.ErrExporterConnection:
            // Database error
            log.Error("correlation query failed", "error", err)
            return
        }
    }
}
```

## Testing

### Unit Test Example

```go
func TestMyFeature(t *testing.T) {
    // Use in-memory store for testing
    store := observability.NewInMemoryCorrelationStore()
    ctx := context.Background()

    // Store test correlation
    err := store.StoreCorrelation(ctx, "node-1", "span-1")
    require.NoError(t, err)

    // Verify lookup
    spanID, err := store.GetSpanForNode(ctx, "node-1")
    require.NoError(t, err)
    assert.Equal(t, "span-1", spanID)

    // Cleanup
    store.Clear()
}
```

### Integration Test Example

```go
func TestNeo4jIntegration(t *testing.T) {
    // Setup Neo4j testcontainer
    neo4jClient := setupTestNeo4j(t)
    defer neo4jClient.Close(context.Background())

    // Create store
    store, err := observability.NewNeo4jCorrelationStore(neo4jClient)
    require.NoError(t, err)

    // Test operations
    ctx := context.Background()
    nodeID := createTestNode(t, neo4jClient)
    spanID := "test-span-123"

    err = store.StoreCorrelation(ctx, nodeID, spanID)
    require.NoError(t, err)

    retrievedSpanID, err := store.GetSpanForNode(ctx, nodeID)
    require.NoError(t, err)
    assert.Equal(t, spanID, retrievedSpanID)
}
```

## Neo4j Schema

### Node Structure

```cypher
// Correlated nodes have:
// - :Correlation label
// - langfuse_span_id property

(:Agent:Correlation {
    id: "agent-123",
    name: "ssrf-hunter",
    langfuse_span_id: "7c9e6679b17e"
})
```

### Index

```cypher
// Created automatically by NewNeo4jCorrelationStore
CREATE INDEX correlation_span_id IF NOT EXISTS
FOR (n:Correlation)
ON (n.langfuse_span_id)
```

### Queries

```cypher
// Node → Span
MATCH (n:Correlation)
WHERE id(n) = $node_id
RETURN n.langfuse_span_id as span_id

// Span → Node
MATCH (n:Correlation {langfuse_span_id: $span_id})
RETURN id(n) as node_id
```

## Performance

| Store Type | Operation | Time | Notes |
|-----------|-----------|------|-------|
| Neo4j | StoreCorrelation | ~10ms | Indexed write |
| Neo4j | GetSpanForNode | ~5ms | Indexed read |
| Neo4j | GetNodeForSpan | ~5ms | Indexed read |
| InMemory | All operations | <1µs | Hash map O(1) |

## Files

- `correlation.go` - Implementation (354 lines)
- `correlation_test.go` - Tests (539 lines)
- `CORRELATION_IMPLEMENTATION.md` - Detailed docs
- `TASK_5_3_SUMMARY.md` - Implementation summary
- `CORRELATION_QUICK_REFERENCE.md` - This file

## Common Issues

### "Correlation not found"

**Cause:** No correlation exists for the given node/span

**Solution:**
```go
spanID, err := store.GetSpanForNode(ctx, nodeID)
if err != nil {
    var obsErr *observability.ObservabilityError
    if errors.As(err, &obsErr) && obsErr.Code == observability.ErrSpanContextMissing {
        // This is expected for nodes created before correlation was enabled
        log.Debug("no correlation for node", "nodeID", nodeID)
        return
    }
    return err // Real error
}
```

### "Index creation failed"

**Cause:** Neo4j connection issue or permission problem

**Solution:**
- Verify Neo4j connectivity: `neo4jClient.Health(ctx)`
- Check user has CREATE INDEX permission
- Manually create index: `CREATE INDEX correlation_span_id FOR (n:Correlation) ON (n.langfuse_span_id)`

### "Race condition detected"

**Cause:** Not using proper locking with InMemoryStore

**Solution:**
- InMemoryStore is already thread-safe
- Run tests with `-race` flag to detect issues
- Ensure you're not modifying store internals directly

## CLI Testing

```bash
# Run correlation tests
go test ./internal/observability -run TestCorrelation -v

# Run with race detector
go test ./internal/observability -run TestCorrelation -race

# Run benchmarks
go test ./internal/observability -bench=BenchmarkCorrelation -benchmem

# Check coverage
go test ./internal/observability -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## See Also

- [Detailed Implementation Guide](CORRELATION_IMPLEMENTATION.md)
- [Implementation Summary](TASK_5_3_SUMMARY.md)
- [OpenTelemetry Tracing](tracing.go)
- [Neo4j Client](../graphrag/graph/neo4j.go)
