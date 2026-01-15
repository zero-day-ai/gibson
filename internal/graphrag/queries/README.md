# GraphRAG Queries

High-level query interfaces for Gibson orchestrator graph operations.

## Overview

This package provides type-safe, production-ready Cypher query abstractions for interacting with the Gibson mission execution graph stored in Neo4j. Each query struct focuses on a specific domain and provides clean, composable methods for graph operations.

## Architecture

### Query Organization

- **MissionQueries** (`mission.go`): Mission and workflow node operations
- **ExecutionQueries** (`execution.go`): Agent and tool execution tracking

### Design Principles

1. **Type Safety**: All queries use strongly-typed schema structs
2. **Context Awareness**: All methods accept `context.Context` for cancellation/timeouts
3. **Error Handling**: Consistent error wrapping with `types.WrapError()`
4. **Transaction Management**: Automatic read/write transaction selection
5. **Parameterized Queries**: All Cypher queries use parameters for security and performance

## MissionQueries API

### Core Operations

#### GetMission
Retrieves a mission by ID.
```go
mission, err := mq.GetMission(ctx, missionID)
```

#### GetReadyNodes
Returns nodes where ALL dependencies are completed - the key query for orchestration.
```go
readyNodes, err := mq.GetReadyNodes(ctx, missionID)
```

#### GetMissionStats
Returns comprehensive execution statistics for a mission.
```go
stats, err := mq.GetMissionStats(ctx, missionID)
// stats contains: TotalNodes, CompletedNodes, FailedNodes, etc.
```

### Dependency Queries

#### GetNodeDependencies
Gets all nodes that a given node depends on (must complete first).
```go
deps, err := mq.GetNodeDependencies(ctx, nodeID)
```

### Execution History

#### GetNodeExecutions
Retrieves all execution attempts for a node (including retries).
```go
executions, err := mq.GetNodeExecutions(ctx, nodeID)
```

#### GetMissionDecisions
Gets all orchestrator decisions for a mission, ordered by iteration.
```go
decisions, err := mq.GetMissionDecisions(ctx, missionID)
```

## Usage Patterns

### Orchestration Loop

The typical pattern for orchestrating mission execution:

```go
// Create queries
mq := queries.NewMissionQueries(client)

// Orchestration loop
for {
    // Get nodes ready to execute
    readyNodes, err := mq.GetReadyNodes(ctx, missionID)
    if err != nil {
        return err
    }

    // No ready nodes = done or blocked
    if len(readyNodes) == 0 {
        break
    }

    // Execute each ready node
    for _, node := range readyNodes {
        // Execute node logic...
    }

    // Check progress
    stats, err := mq.GetMissionStats(ctx, missionID)
    if err != nil {
        return err
    }

    // Check if complete
    if stats.CompletedNodes + stats.FailedNodes == stats.TotalNodes {
        break
    }
}
```

### Dependency Checking

```go
// Get dependencies
deps, err := mq.GetNodeDependencies(ctx, nodeID)
if err != nil {
    return err
}

// Check if all completed
allCompleted := true
for _, dep := range deps {
    if dep.Status != schema.WorkflowNodeStatusCompleted {
        allCompleted = false
        fmt.Printf("Waiting for: %s\n", dep.Name)
    }
}
```

### Execution History and Retries

```go
// Get all execution attempts
executions, err := mq.GetNodeExecutions(ctx, nodeID)
if err != nil {
    return err
}

fmt.Printf("Node has %d execution attempts\n", len(executions))
for _, exec := range executions {
    fmt.Printf("Attempt %d: %s\n", exec.Attempt, exec.Status)
    if exec.Error != "" {
        fmt.Printf("  Error: %s\n", exec.Error)
    }
}
```

## Cypher Query Patterns

### GetReadyNodes Pattern

This is the core orchestration query - finds nodes where all dependencies are completed:

```cypher
MATCH (m:Mission {id: $mission_id})-[:HAS_NODE]->(n:WorkflowNode)
WHERE n.status = 'ready'
AND NOT EXISTS {
    MATCH (n)-[:DEPENDS_ON]->(dep:WorkflowNode)
    WHERE dep.status <> 'completed'
}
RETURN n
ORDER BY n.created_at
```

### GetMissionStats Pattern

Aggregates statistics across all nodes, decisions, and executions:

```cypher
MATCH (m:Mission {id: $mission_id})
OPTIONAL MATCH (m)-[:HAS_NODE]->(n:WorkflowNode)
OPTIONAL MATCH (m)-[:HAS_DECISION]->(d:Decision)
OPTIONAL MATCH (n)-[:HAS_EXECUTION]->(e:AgentExecution)
RETURN
    COUNT(DISTINCT n) as total_nodes,
    COUNT(DISTINCT CASE WHEN n.status = 'completed' THEN n END) as completed_nodes,
    COUNT(DISTINCT CASE WHEN n.status = 'failed' THEN n END) as failed_nodes,
    COUNT(DISTINCT d) as total_decisions,
    COUNT(DISTINCT e) as total_executions
```

## Performance Considerations

### Indexed Queries
All queries use indexed fields for fast lookups:
- `Mission.id` - indexed
- `WorkflowNode.id` - indexed
- `WorkflowNode.mission_id` - indexed

### Transaction Types
- Read queries automatically use `ExecuteRead` for better performance
- Write queries use `ExecuteWrite` for consistency
- Neo4j driver automatically selects appropriate transaction type

### Connection Pooling
The underlying `GraphClient` manages connection pooling:
- Default pool size: 50 connections
- Configurable via `GraphClientConfig.MaxConnectionPoolSize`
- Automatic connection reuse and health checks

## Error Handling

All methods return typed errors with consistent error codes:

```go
mission, err := mq.GetMission(ctx, missionID)
if err != nil {
    // Check error type
    var gibsonErr types.Error
    if errors.As(err, &gibsonErr) {
        switch gibsonErr.Code {
        case "MISSION_QUERY_FAILED":
            // Handle query failure
        case "MISSION_PARSE_FAILED":
            // Handle parse error
        }
    }
}
```

Common error codes:
- `MISSION_QUERY_FAILED`: Query execution failed
- `NODE_QUERY_FAILED`: Node query failed
- `NODE_NOT_FOUND`: Workflow node not found
- `PARSE_FAILED`: Failed to parse graph record

## Thread Safety

All query methods are safe for concurrent use:
- No shared mutable state
- Underlying `GraphClient` handles concurrent connections
- Neo4j driver provides connection pool thread safety

## Testing

### Mock Client

Use `graph.NewMockGraphClient()` for testing:

```go
func TestMyFunction(t *testing.T) {
    // Create mock client
    client := graph.NewMockGraphClient()
    require.NoError(t, client.Connect(context.Background()))

    // Configure response
    client.AddQueryResult(graph.QueryResult{
        Records: []map[string]any{
            {"id": "mission-1", "name": "Test"},
        },
    })

    // Test your code
    mq := queries.NewMissionQueries(client)
    mission, err := mq.GetMission(ctx, missionID)
    require.NoError(t, err)
    assert.Equal(t, "Test", mission.Name)
}
```

### Integration Tests

For integration tests, use a real Neo4j instance:

```go
func TestIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    config := graph.DefaultConfig()
    client, err := graph.NewNeo4jClient(config)
    require.NoError(t, err)

    // Use real Neo4j for testing
    mq := queries.NewMissionQueries(client)
    // ...
}
```

## Best Practices

1. **Always use context** for timeouts and cancellation
2. **Handle nil results** (e.g., mission not found returns nil, not error)
3. **Check mission stats** to monitor progress
4. **Use GetReadyNodes** in a loop for orchestration
5. **Log decisions** for debugging orchestrator behavior
6. **Monitor execution attempts** to detect retry loops

## Examples

See `example_mission_test.go` for complete, runnable examples demonstrating:
- Orchestration loop pattern
- Dependency checking
- Execution history tracking
- Decision auditing
- Statistics monitoring

## Related Packages

- `github.com/zero-day-ai/gibson/internal/graphrag/schema` - Schema type definitions
- `github.com/zero-day-ai/gibson/internal/graphrag/graph` - Graph client interface
- `github.com/zero-day-ai/gibson/internal/types` - Common types (ID, Error, etc.)
