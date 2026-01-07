# Daemon Integration Test Workflows

This directory contains workflow YAML files for testing the Gibson daemon's mission execution capabilities.

## Test Workflows

### simple-workflow.yaml

**Purpose:** Basic linear workflow for daemon integration testing

**Structure:**
- Single agent node (test-agent)
- Single tool node (test-tool)
- Linear dependency: agent → tool

**Use Cases:**
- Testing basic mission execution flow
- Verifying agent-to-tool sequencing
- Testing timeout and retry configuration
- Validating event streaming for simple workflows

**Configuration:**
- Mock LLM provider
- 1 minute agent timeout
- 30 second tool timeout
- No retries (max_retries: 0)

### parallel-workflow.yaml

**Purpose:** Tests parallel node execution and event ordering

**Structure:**
1. **Init phase:** Single agent node
2. **Parallel phase:** Group with 3 concurrent sub-nodes
   - parallel_1: Agent
   - parallel_2: Agent
   - parallel_3: Tool
3. **Join phase:** Synchronization point
4. **Finalize phase:** Single agent node

**Use Cases:**
- Testing concurrent node execution
- Verifying parallel event delivery
- Testing join/synchronization behavior
- Validating correct DAG execution order
- Testing event timestamp ordering

**Configuration:**
- Mock LLM provider
- Variable timeouts (30s - 1m)
- No retries
- Demonstrates parallel execution group pattern

### invalid-workflow.yaml

**Purpose:** Intentionally malformed workflow for error testing

**Invalid Patterns:**
1. Missing required 'type' field
2. Circular dependencies between nodes
3. Incomplete node definitions

**Use Cases:**
- Testing workflow validation errors
- Verifying error message quality
- Testing graceful error handling
- Validating workflow parser robustness

**Expected Behavior:**
- Should fail during workflow parsing
- Should return descriptive error messages
- Should not create a mission
- Should not leave orphaned resources

## Workflow Schema Reference

### Common Fields

```yaml
name: Workflow Name
description: Workflow description
version: "1.0"

config:
  llm:
    default_provider: provider_name
    default_model: model_name

target:
  seeds:
    - value: "target_identifier"
      type: hostname|cidr|url
      scope: in_scope|out_of_scope

nodes:
  - id: unique_node_id
    type: agent|tool|plugin|parallel|join|condition
    name: Human-readable name
    description: Node description
    depends_on: [parent_node_ids]
    timeout: duration (e.g., 1m, 30s)
    retry:
      max_retries: number
      backoff: constant|exponential
      initial_delay: duration
      max_delay: duration
      multiplier: float
```

### Node Types

**Agent Node:**
```yaml
- id: agent_node
  type: agent
  agent: agent-name
  task:
    goal: "Task description"
    context:
      key: value
```

**Tool Node:**
```yaml
- id: tool_node
  type: tool
  tool: tool-name
  input:
    parameter: value
```

**Parallel Group:**
```yaml
- id: parallel_group
  type: parallel
  sub_nodes:
    - id: sub_node_1
      type: agent
      # ... node config
```

**Join Node:**
```yaml
- id: join_node
  type: join
  depends_on: [parallel_group]
```

## Testing Guidelines

### Running Tests with These Workflows

```bash
# When mission execution is implemented:

# Test simple workflow
gibson mission run --workflow testdata/simple-workflow.yaml

# Test parallel execution
gibson mission run --workflow testdata/parallel-workflow.yaml

# Test error handling (should fail gracefully)
gibson mission run --workflow testdata/invalid-workflow.yaml
```

### Integration Test Usage

These workflows are used by integration tests in:
- `mission_integration_test.go` - Mission lifecycle tests
- `error_scenarios_integration_test.go` - Error handling tests
- `event_streaming_integration_test.go` - Event delivery tests

```go
// Example test usage:
func TestMissionExecution(t *testing.T) {
    workflowPath := "testdata/simple-workflow.yaml"

    stream, err := client.RunMission(ctx, &RunMissionRequest{
        WorkflowPath: workflowPath,
    })

    // Collect and verify events...
}
```

### Adding New Test Workflows

When adding new test workflows:

1. **Validate YAML syntax:**
   ```bash
   python3 -c "import yaml; yaml.safe_load(open('new-workflow.yaml'))"
   ```

2. **Document purpose and structure** in this README

3. **Add corresponding integration tests** in `*_integration_test.go` files

4. **Use descriptive node IDs and names** for easy debugging

5. **Include timeout and retry configs** to test resilience

### Workflow Design Best Practices

**For simple workflows:**
- Use short timeouts (30s - 1m) for fast test execution
- Disable retries (max_retries: 0) for predictable behavior
- Use mock agents/tools when possible

**For complex workflows:**
- Test parallel execution with at least 3 concurrent nodes
- Include join nodes to verify synchronization
- Test various dependency patterns
- Use realistic timeouts for real-world scenarios

**For error testing:**
- Create multiple error conditions per workflow
- Test both syntax errors and semantic errors
- Include edge cases (empty fields, invalid references)

## Event Sequence Examples

### Simple Workflow Events

Expected event sequence:
1. `mission_started` - Mission begins
2. `node_started` (test_agent) - Agent starts
3. `node_completed` (test_agent) - Agent completes
4. `node_started` (test_tool) - Tool starts
5. `node_completed` (test_tool) - Tool completes
6. `mission_completed` - Mission finishes

### Parallel Workflow Events

Expected event sequence:
1. `mission_started`
2. `node_started` (init)
3. `node_completed` (init)
4. `node_started` (parallel_1, parallel_2, parallel_3) - Concurrent
5. `node_completed` (parallel_1, parallel_2, parallel_3) - Any order
6. `node_started` (join_parallel) - After all parallel nodes
7. `node_completed` (join_parallel)
8. `node_started` (finalize)
9. `node_completed` (finalize)
10. `mission_completed`

### Invalid Workflow Events

Expected behavior:
1. Workflow parsing fails
2. Error event with validation details
3. No mission_started event
4. No nodes execute

## Troubleshooting

### Common Issues

**Workflow doesn't parse:**
- Check YAML syntax with validator
- Verify all required fields are present
- Check for circular dependencies

**Mission hangs:**
- Verify timeout values are reasonable
- Check agent/tool availability
- Review dependency graph for blocking

**Events out of order:**
- Check depends_on relationships
- Verify parallel group is used correctly
- Check for missing join nodes

**Agent/tool not found:**
- Verify agent is registered in etcd
- Check agent/tool name matches registration
- Verify daemon can reach component endpoint

## Related Documentation

- **API Documentation:** `../API.md`
- **Integration Tests:** `../*_integration_test.go`
- **Daemon Implementation:** `../daemon.go`
- **Mission Orchestration:** See `internal/mission/` package
