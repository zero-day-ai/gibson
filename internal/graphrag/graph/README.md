# Graph Database Client Package

This package provides a graph database client abstraction for GraphRAG integration in the Gibson framework.

## Overview

The `graph` package defines a generic `GraphClient` interface that supports multiple graph database backends. The primary implementation is for Neo4j using the official Neo4j Go driver v5.

## Architecture

### Core Components

1. **GraphClient Interface** (`client.go`)
   - Generic interface for graph database operations
   - Methods: Connect, Close, Health, Query, CreateNode, CreateRelationship, DeleteNode
   - Thread-safe implementations required

2. **Neo4jClient** (`neo4j.go`)
   - Production implementation for Neo4j databases
   - Connection pooling and retry logic with exponential backoff
   - Full Cypher query support
   - Health monitoring

3. **MockGraphClient** (`mock.go`)
   - Test double for unit testing
   - Records all method calls for verification
   - Configurable responses and error injection
   - In-memory node and relationship storage

## Usage

### Basic Setup

```go
import (
    "context"
    "github.com/zero-day-ai/gibson/internal/graphrag/graph"
)

// Create and configure client
config := graph.DefaultConfig()
config.URI = "bolt://localhost:7687"
config.Username = "neo4j"
config.Password = "password"

client, err := graph.NewNeo4jClient(config)
if err != nil {
    return err
}

// Connect to database
ctx := context.Background()
if err := client.Connect(ctx); err != nil {
    return err
}
defer client.Close(ctx)
```

### Node Operations

```go
// Create a node
nodeID, err := client.CreateNode(ctx,
    []string{"Person", "Employee"},
    map[string]any{
        "name": "Alice",
        "age": 30,
        "department": "Engineering",
    },
)

// Delete a node
err = client.DeleteNode(ctx, nodeID)
```

### Relationship Operations

```go
// Create nodes
aliceID, _ := client.CreateNode(ctx, []string{"Person"},
    map[string]any{"name": "Alice"})
bobID, _ := client.CreateNode(ctx, []string{"Person"},
    map[string]any{"name": "Bob"})

// Create relationship
err := client.CreateRelationship(ctx, aliceID, bobID, "KNOWS",
    map[string]any{"since": 2020})
```

### Cypher Queries

```go
result, err := client.Query(ctx,
    "MATCH (n:Person {name: $name}) RETURN n.name, n.age",
    map[string]any{"name": "Alice"},
)

for _, record := range result.Records {
    fmt.Printf("Name: %s, Age: %d\n",
        record["name"], record["age"])
}
```

### Health Monitoring

```go
status := client.Health(ctx)
if status.IsHealthy() {
    log.Println("Graph database is operational")
} else {
    log.Printf("Graph database unhealthy: %s", status.Message)
}
```

## Configuration

### GraphClientConfig

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| URI | string | Connection URI (see URI Schemes below) | bolt://localhost:7687 |
| Username | string | Authentication username | neo4j |
| Password | string | Authentication password | password |
| Database | string | Database name (empty = default) | "" |
| MaxConnectionPoolSize | int | Max connections in pool | 50 |
| ConnectionTimeout | duration | Connection acquisition timeout | 30s |
| MaxTransactionRetryTime | duration | Max retry time for transactions | 30s |

### URI Schemes

Neo4j supports different URI schemes for encryption:

- `bolt://host:port` - Unencrypted connection
- `bolt+s://host:port` - TLS encrypted (system CA verification)
- `bolt+ssc://host:port` - TLS encrypted (self-signed certs OK)
- `neo4j://host:port` - Routing, unencrypted
- `neo4j+s://host:port` - Routing, TLS encrypted

## Error Handling

All errors are wrapped in `types.GibsonError` with specific error codes:

| Error Code | Description |
|------------|-------------|
| `GRAPH_CONNECTION_FAILED` | Connection establishment failed |
| `GRAPH_CONNECTION_LOST` | Connection lost during operation |
| `GRAPH_CONNECTION_CLOSED` | Operation on closed connection |
| `GRAPH_INVALID_CONFIG` | Invalid configuration |
| `GRAPH_QUERY_FAILED` | Query execution failed |
| `GRAPH_QUERY_TIMEOUT` | Query timeout |
| `GRAPH_NODE_NOT_FOUND` | Node not found |
| `GRAPH_NODE_CREATE_FAILED` | Node creation failed |
| `GRAPH_RELATIONSHIP_CREATE_FAILED` | Relationship creation failed |

Example error handling:

```go
result, err := client.Query(ctx, cypher, params)
if err != nil {
    var gibsonErr *types.GibsonError
    if errors.As(err, &gibsonErr) {
        if gibsonErr.Code == graph.ErrCodeGraphQueryFailed {
            // Handle query error
        }
    }
}
```

## Testing

### Using MockGraphClient

```go
func TestMyFunction(t *testing.T) {
    mock := graph.NewMockGraphClient()
    ctx := context.Background()

    // Connect
    _ = mock.Connect(ctx)

    // Configure mock responses
    mock.AddQueryResult(graph.QueryResult{
        Records: []map[string]any{
            {"name": "Alice", "age": 30},
        },
        Columns: []string{"name", "age"},
    })

    // Run your code with mock
    result, _ := myFunction(mock)

    // Verify calls
    calls := mock.GetCallsByMethod("Query")
    assert.Len(t, calls, 1)

    // Verify state
    nodes := mock.GetNodes()
    assert.Len(t, nodes, 2)
}
```

### Error Injection

```go
mock.SetQueryError(errors.New("database unavailable"))
_, err := client.Query(ctx, "MATCH (n) RETURN n", nil)
assert.Error(t, err)
```

## Connection Management

### Connection Pooling

The Neo4j driver maintains a connection pool internally:
- Connections are reused across requests
- Pool size controlled by `MaxConnectionPoolSize`
- Idle connections are kept alive automatically
- Connections are verified before use

### Retry Logic

Failed connection attempts use exponential backoff:
- Base delay: 100ms
- Maximum retries: 5
- Backoff multiplier: 2x per retry
- Respects context cancellation

### Resource Cleanup

Always close the client when done:

```go
defer client.Close(ctx)
```

This ensures:
- All connections are released
- Resources are cleaned up
- Background goroutines are terminated

## Performance Considerations

### Query Optimization

1. **Use parameters** instead of string interpolation:
   ```go
   // Good
   Query(ctx, "MATCH (n {id: $id})", map[string]any{"id": id})

   // Bad (security risk, no plan caching)
   Query(ctx, fmt.Sprintf("MATCH (n {id: '%s'})", id), nil)
   ```

2. **Limit result size** for large queries:
   ```go
   Query(ctx, "MATCH (n:Person) RETURN n LIMIT 100", nil)
   ```

3. **Use indexes** for frequently queried properties (create in Neo4j):
   ```cypher
   CREATE INDEX person_name FOR (n:Person) ON (n.name)
   ```

### Connection Pool Sizing

- **Low concurrency** (1-10 concurrent operations): 10-20 connections
- **Medium concurrency** (10-50 concurrent operations): 20-50 connections
- **High concurrency** (50+ concurrent operations): 50-100 connections

Monitor connection pool usage and adjust based on your workload.

## Thread Safety

All implementations are thread-safe:
- **Neo4jClient**: Uses Neo4j driver's internal synchronization
- **MockGraphClient**: Protected by sync.RWMutex

Multiple goroutines can safely share a single client instance.

## Examples

See `example_test.go` for runnable examples:

```bash
go test -run Example ./internal/graphrag/graph/...
```

## Dependencies

- `github.com/neo4j/neo4j-go-driver/v5` - Official Neo4j Go driver
- `github.com/zero-day-ai/gibson/internal/types` - Gibson error types and health status

## Future Enhancements

Potential future additions:
- Support for other graph databases (TinkerGraph, JanusGraph, etc.)
- Batch operations for improved performance
- Transaction support with explicit begin/commit/rollback
- Schema management utilities
- Query builder for type-safe Cypher construction
- Distributed tracing integration

## References

- [Neo4j Go Driver Documentation](https://neo4j.com/docs/go-manual/current/)
- [Cypher Query Language Reference](https://neo4j.com/docs/cypher-manual/current/)
- [Gibson Framework Types](../../../types/)
