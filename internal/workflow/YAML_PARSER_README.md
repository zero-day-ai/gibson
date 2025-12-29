# Workflow YAML Parser Implementation

This document describes the YAML parsing functionality for the Gibson workflow engine, implemented in task 7.1 of stage-9-dag-workflow-engine.

## Overview

The YAML parser allows workflow definitions to be written in human-readable YAML format and automatically converted into executable `Workflow` structures. This provides a declarative way to define complex DAG workflows without writing Go code.

## Files

- **yaml.go**: Core YAML parsing implementation
- **yaml_test.go**: Comprehensive test suite with 100+ test cases
- **examples/workflow-example.yaml**: Real-world workflow example
- **examples/yaml_parser_example.go**: Example program demonstrating usage

## Features

### Supported Node Types

1. **Agent Nodes** - Execute tasks using registered agents
   ```yaml
   - id: scan
     type: agent
     name: Port Scanner
     agent: nmap-agent
     task:
       target: example.com
       ports: 1-65535
   ```

2. **Tool Nodes** - Call workflow tools
   ```yaml
   - id: generate_report
     type: tool
     name: Report Generator
     tool: pdf-generator
     input:
       format: pdf
       template: security-report
   ```

3. **Plugin Nodes** - Invoke plugin methods
   ```yaml
   - id: notify
     type: plugin
     name: Send Email
     plugin: email-service
     method: send
     params:
       to: security@example.com
       subject: Scan Complete
   ```

4. **Condition Nodes** - Conditional branching
   ```yaml
   - id: check_vulns
     type: condition
     name: Check for Vulnerabilities
     condition:
       expression: findings.count > 0
       true_branch:
         - exploit_check
       false_branch:
         - clean_report
   ```

5. **Parallel Nodes** - Concurrent execution
   ```yaml
   - id: parallel_scans
     type: parallel
     name: Run Multiple Scans
     sub_nodes:
       - id: port_scan
         type: agent
         agent: nmap
       - id: vuln_scan
         type: agent
         agent: nessus
   ```

6. **Join Nodes** - Synchronization points
   ```yaml
   - id: wait_for_scans
     type: join
     name: Wait for All Scans
     depends_on:
       - parallel_scans
   ```

### Execution Control Features

#### Dependencies
Define execution order using `depends_on`:
```yaml
- id: analyze
  type: agent
  agent: analyzer
  depends_on:
    - scan
    - recon
```

#### Timeouts
Set maximum execution time using Go duration format:
```yaml
timeout: 30s    # 30 seconds
timeout: 5m     # 5 minutes
timeout: 2h     # 2 hours
```

#### Retry Policies
Configure automatic retry with backoff strategies:

**Constant Backoff** - Fixed delay:
```yaml
retry:
  max_retries: 3
  backoff: constant
  initial_delay: 5s
```

**Linear Backoff** - Linearly increasing delay:
```yaml
retry:
  max_retries: 5
  backoff: linear
  initial_delay: 1s
```

**Exponential Backoff** - Exponentially increasing delay with cap:
```yaml
retry:
  max_retries: 5
  backoff: exponential
  initial_delay: 1s
  max_delay: 60s
  multiplier: 2.0
```

## API Functions

### ParseWorkflow
```go
func ParseWorkflow(data []byte) (*Workflow, error)
```
Parses YAML bytes into a Workflow structure.

**Parameters:**
- `data`: Raw YAML bytes

**Returns:**
- `*Workflow`: Parsed workflow structure
- `error`: Parsing or validation error

**Example:**
```go
yamlData := []byte(`
name: My Workflow
description: Example workflow
nodes:
  - id: node1
    type: agent
    agent: test-agent
`)

wf, err := workflow.ParseWorkflow(yamlData)
if err != nil {
    log.Fatal(err)
}
```

### ParseWorkflowFile
```go
func ParseWorkflowFile(path string) (*Workflow, error)
```
Reads and parses a YAML file.

**Parameters:**
- `path`: Path to YAML file

**Returns:**
- `*Workflow`: Parsed workflow structure
- `error`: File I/O or parsing error

**Example:**
```go
wf, err := workflow.ParseWorkflowFile("workflows/security-scan.yaml")
if err != nil {
    log.Fatal(err)
}
```

## Validation

The parser performs comprehensive validation:

### Required Fields
- Workflow name must be present
- At least one node must be defined
- Each node must have an ID
- Node IDs must be unique
- Node type must be valid

### Node-Specific Validation
- **Agent nodes**: Must specify agent name
- **Tool nodes**: Must specify tool name
- **Plugin nodes**: Must specify plugin name and method
- **Condition nodes**: Must have expression
- **Parallel nodes**: Must contain at least one sub-node

### Dependency Validation
- All `depends_on` references must point to existing nodes
- Circular dependencies are detected at execution time

### Format Validation
- Timeout values must be valid Go duration strings
- Retry backoff must be "constant", "linear", or "exponential"
- Exponential backoff requires max_delay and positive multiplier

## Error Handling

All parsing errors include descriptive messages:

```go
wf, err := workflow.ParseWorkflow(invalidYAML)
if err != nil {
    // Error messages include context:
    // "failed to convert node scan: agent name is required for agent nodes"
    // "node analyze depends on non-existent node unknown"
    // "invalid timeout format "5z": time: unknown unit "z" in duration "5z""
}
```

## Workflow Structure

### Generated Fields

The parser automatically generates:

1. **Entry Points**: Nodes with no dependencies
2. **Exit Points**: Nodes with no outgoing edges
3. **Edges**: Created from `depends_on` relationships
4. **Node IDs**: Generated for parallel sub-nodes if not specified

### Example Output

For this YAML:
```yaml
name: Simple Workflow
nodes:
  - id: scan
    type: agent
    agent: scanner
  - id: analyze
    type: agent
    agent: analyzer
    depends_on:
      - scan
```

Generated Workflow:
```go
Workflow{
    Name: "Simple Workflow",
    Nodes: map[string]*WorkflowNode{
        "scan": {...},
        "analyze": {...},
    },
    Edges: []WorkflowEdge{
        {From: "scan", To: "analyze"},
    },
    EntryPoints: []string{"scan"},
    ExitPoints: []string{"analyze"},
}
```

## Testing

The implementation includes comprehensive tests:

- **Basic parsing**: Single and multiple nodes
- **All node types**: Agent, tool, plugin, condition, parallel, join
- **Dependencies**: Linear chains, parallel execution, diamond patterns
- **Timeouts**: Duration parsing and validation
- **Retry policies**: All backoff strategies
- **Error cases**: 17 different validation error scenarios
- **File I/O**: File reading and error handling
- **Real-world example**: Complex multi-phase workflow

Run tests:
```bash
go test -v ./internal/workflow -run TestParseWorkflow
```

## Usage Examples

### Simple Linear Workflow
```yaml
name: Linear Scan
description: Sequential port scan and analysis
nodes:
  - id: scan
    type: agent
    agent: nmap
    task:
      target: 192.168.1.0/24

  - id: analyze
    type: agent
    agent: analyzer
    depends_on:
      - scan

  - id: report
    type: tool
    tool: report-gen
    depends_on:
      - analyze
```

### Parallel Execution
```yaml
name: Parallel Scans
description: Run multiple scans concurrently
nodes:
  - id: parallel_phase
    type: parallel
    sub_nodes:
      - type: agent
        agent: nmap
        task:
          ports: 1-1024
      - type: agent
        agent: nmap
        task:
          ports: 1025-65535

  - id: combine
    type: join
    depends_on:
      - parallel_phase
```

### Conditional Workflow
```yaml
name: Conditional Analysis
description: Different paths based on findings
nodes:
  - id: scan
    type: agent
    agent: scanner

  - id: check
    type: condition
    depends_on:
      - scan
    condition:
      expression: vulnerabilities.count > 0
      true_branch:
        - detailed_analysis
      false_branch:
        - clean_report

  - id: detailed_analysis
    type: agent
    agent: deep-analyzer

  - id: clean_report
    type: tool
    tool: simple-report
```

### With Retry and Timeout
```yaml
name: Resilient Workflow
description: Workflow with error handling
nodes:
  - id: flaky_scan
    type: agent
    agent: scanner
    timeout: 5m
    retry:
      max_retries: 3
      backoff: exponential
      initial_delay: 10s
      max_delay: 2m
      multiplier: 2.0
    task:
      target: example.com
```

## Integration

The parsed workflows integrate seamlessly with the workflow executor:

```go
// Parse workflow from YAML
wf, err := workflow.ParseWorkflowFile("workflow.yaml")
if err != nil {
    log.Fatal(err)
}

// Execute workflow
executor := workflow.NewWorkflowExecutor()
result, err := executor.Execute(ctx, wf, harness)
if err != nil {
    log.Fatal(err)
}

// Check results
fmt.Printf("Status: %s\n", result.Status)
fmt.Printf("Nodes executed: %d\n", result.NodesExecuted)
```

## Design Decisions

### YAML Structure
- Array-based node definition for ordered visualization
- Inline dependencies via `depends_on` for clarity
- Flat structure with references instead of deep nesting

### Type Safety
- Strong validation during parsing
- Type-specific fields for each node type
- Compile-time checked conversions

### Error Messages
- Descriptive errors with context
- Node ID included in error messages
- Specific guidance for common mistakes

### Flexibility
- Optional fields for most attributes
- Default values for timeout and retry
- Metadata support for custom extensions

## Best Practices

1. **Node IDs**: Use descriptive, kebab-case IDs
2. **Dependencies**: Keep dependency chains shallow when possible
3. **Timeouts**: Set appropriate timeouts based on expected execution time
4. **Retries**: Use exponential backoff for transient failures
5. **Parallel Nodes**: Group related concurrent operations
6. **Conditions**: Keep expressions simple and testable

## Limitations

1. **No Cycles**: Workflows must be DAGs (no circular dependencies)
2. **Static Structure**: Workflow structure is fixed at parse time
3. **Expression Evaluation**: Condition expressions require separate evaluator
4. **No Variables**: No template variables in YAML (future enhancement)

## Future Enhancements

Potential improvements for future versions:

1. **Template Variables**: Support `${var}` substitution
2. **Schema Validation**: JSON Schema validation
3. **Imports**: Include workflows from other files
4. **Macros**: Reusable node templates
5. **Visualization**: Generate DAG diagrams from YAML
6. **Hot Reload**: Watch files for changes
