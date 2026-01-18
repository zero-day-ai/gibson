# Gibson Mission/Workflow YAML Reference

This document provides a comprehensive reference for Gibson mission and workflow YAML files. Missions define the execution plan for security testing campaigns using a directed acyclic graph (DAG) of nodes.

## Table of Contents

- [Overview](#overview)
- [File Structure](#file-structure)
- [Root Level Fields](#root-level-fields)
- [Target Configuration](#target-configuration)
- [Nodes Configuration](#nodes-configuration)
  - [Common Node Fields](#common-node-fields)
  - [Agent Nodes](#agent-nodes)
  - [Tool Nodes](#tool-nodes)
  - [Plugin Nodes](#plugin-nodes)
  - [Condition Nodes](#condition-nodes)
  - [Parallel Nodes](#parallel-nodes)
  - [Join Nodes](#join-nodes)
- [Task Configuration](#task-configuration)
- [Retry Policy](#retry-policy)
- [Bounds (Constraints)](#bounds-constraints)
- [Config Section](#config-section)
- [Dependencies](#dependencies)
- [Metadata](#metadata)
- [Orchestrator Decision Actions](#orchestrator-decision-actions)
- [Complete Examples](#complete-examples)
- [Execution Rules](#execution-rules)
- [Duration Format](#duration-format)

---

## Overview

A Gibson mission is a declarative YAML file that defines:

1. **What to attack**: Target specification
2. **How to attack**: Workflow nodes (agents, tools, plugins)
3. **In what order**: Dependencies between nodes (DAG)
4. **With what limits**: Bounds and constraints
5. **Using what resources**: LLM config, budgets

Missions follow Kubernetes-like declarative patterns - you describe the desired state and Gibson's orchestrator figures out how to execute it.

---

## File Structure

```yaml
# Mission metadata
name: "Security Assessment"
description: "Comprehensive vulnerability assessment"
version: "1.0.0"

# Target specification
target: "my-target"  # or inline object

# Execution configuration
config:
  llm:
    default_provider: anthropic
  budget:
    max_tokens: 100000

# Execution constraints
bounds:
  max_duration: 1h
  max_findings: 100

# Required components
dependencies:
  agents:
    - recon-agent
  tools:
    - nmap

# Workflow definition
nodes:
  - id: recon
    type: agent
    agent: recon-agent
    # ... node config

# Custom metadata
metadata:
  team: security
  environment: production
```

---

## Root Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **Yes** | Human-readable mission name |
| `description` | string | No | Detailed description of the mission |
| `version` | string | No | Semantic version (e.g., `1.0.0`) |
| `target` | string or object | No | Target specification |
| `nodes` | array or map | **Yes** | Workflow node definitions |
| `config` | object | No | LLM and observability configuration |
| `bounds` | object | No | Execution constraints |
| `dependencies` | object | No | Required agents, tools, plugins |
| `metadata` | map[string]any | No | Custom key-value metadata |

---

## Target Configuration

Targets can be specified as a reference string or inline object.

### Reference (recommended)

```yaml
target: "my-stored-target"
```

References a target previously created via `gibson target add`.

### Inline Definition

```yaml
target:
  name: example-app
  type: llm_api
  reference: "https://api.example.com/chat"
  description: "Production chat API"
  metadata:
    environment: production
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | No | Human-readable name |
| `type` | string | No | Target type (see below) |
| `reference` | string | No | URL or identifier |
| `description` | string | No | Target description |
| `metadata` | map | No | Custom metadata |

### Target Types

| Type | Description | Example Reference |
|------|-------------|-------------------|
| `llm_api` | LLM chat/completion API | `https://api.openai.com/v1/chat/completions` |
| `web_app` | Web application | `https://example.com` |
| `api` | REST/GraphQL API | `https://api.example.com/v1` |
| `network` | Network/subnet | `192.168.1.0/24` |
| `kubernetes` | Kubernetes cluster | `~/.kube/config` |
| `container` | Container/image | `docker://nginx:latest` |
| `smart_contract` | Blockchain contract | `0x742d35Cc...` |

---

## Nodes Configuration

Nodes are the execution units in the workflow. They can be defined as an **array** (recommended) or **map**.

### Array Format (recommended)

```yaml
nodes:
  - id: node1
    type: agent
    agent: my-agent

  - id: node2
    type: tool
    tool: nmap
    depends_on: [node1]
```

### Map Format (legacy)

```yaml
nodes:
  node1:
    type: agent
    agent: my-agent

  node2:
    type: tool
    tool: nmap
    depends_on: [node1]
```

---

## Common Node Fields

All node types share these fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | **Yes** | Unique identifier within the mission |
| `type` | string | **Yes** | Node type (see below) |
| `name` | string | No | Human-readable name |
| `description` | string | No | What this node does |
| `depends_on` | []string | No | Node IDs that must complete first |
| `timeout` | duration | No | Maximum execution time |
| `retry` | object | No | Retry policy configuration |
| `metadata` | map | No | Custom node metadata |

### Node Types

| Type | Description |
|------|-------------|
| `agent` | Execute a registered agent |
| `tool` | Execute a registered tool directly |
| `plugin` | Call a plugin method |
| `condition` | Conditional branching |
| `parallel` | Execute multiple nodes concurrently |
| `join` | Synchronization point |

---

## Agent Nodes

Execute a registered Gibson agent.

```yaml
- id: vulnerability-scan
  type: agent
  name: "Vulnerability Scanner"
  description: "Scan for common vulnerabilities"
  agent: vuln-scanner
  timeout: 10m

  task:
    goal: "Find SQL injection and XSS vulnerabilities"
    description: "Comprehensive web vulnerability scan"
    context:
      scan_depth: deep
      follow_links: true
      max_pages: 100
    priority: 1
    tags:
      - web
      - injection

  retry:
    max_retries: 2
    backoff: exponential
    initial_delay: 5s
    max_delay: 30s
    multiplier: 2.0

  depends_on:
    - recon

  metadata:
    category: scanning
```

### Agent Node Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agent` | string | **Yes** | Agent name (must be registered) |
| `task` | object | No | Task configuration passed to agent |
| `timeout` | duration | No | Override default timeout |
| `retry` | object | No | Retry policy |

---

## Tool Nodes

Execute a registered tool directly without agent orchestration.

```yaml
- id: port-scan
  type: tool
  name: "Port Scan"
  tool: nmap
  timeout: 5m

  input:
    target: "{{target.reference}}"
    ports: "1-65535"
    scan_type: "syn"
    timing: "T4"
    output_format: "xml"

  depends_on:
    - dns-resolve
```

### Tool Node Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tool` | string | **Yes** | Tool name (must be registered) |
| `input` | map | **Yes** | Tool-specific input parameters |

### Variable Interpolation

Tool inputs support variable interpolation:

| Variable | Description |
|----------|-------------|
| `{{target}}` | Full target object |
| `{{target.reference}}` | Target reference/URL |
| `{{target.name}}` | Target name |
| `{{nodes.<id>.output}}` | Output from another node |
| `{{mission.id}}` | Current mission ID |

---

## Plugin Nodes

Call a registered plugin method for data access or external integration.

```yaml
- id: fetch-scope
  type: plugin
  name: "Fetch Bug Bounty Scope"
  plugin: scope-ingestion
  method: get_scope
  timeout: 30s

  params:
    program: "example-program"
    include_out_of_scope: false
```

### Plugin Node Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `plugin` | string | **Yes** | Plugin name |
| `method` | string | **Yes** | Method to invoke |
| `params` | map | No | Parameters passed to method |

---

## Condition Nodes

Conditional branching based on expression evaluation.

```yaml
- id: check-critical
  type: condition
  name: "Check for Critical Findings"
  depends_on:
    - vulnerability-scan

  condition:
    expression: "nodes.vulnerability-scan.findings.critical > 0"
    true_branch:
      - deep-analysis
      - alert-team
    false_branch:
      - standard-report
```

### Condition Node Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `condition.expression` | string | **Yes** | Expression to evaluate |
| `condition.true_branch` | []string | No | Node IDs if true |
| `condition.false_branch` | []string | No | Node IDs if false |

### Expression Syntax

Expressions support:

- **Comparisons**: `==`, `!=`, `>`, `<`, `>=`, `<=`
- **Logical**: `&&`, `||`, `!`
- **Arithmetic**: `+`, `-`, `*`, `/`
- **Access**: `nodes.<id>.output`, `nodes.<id>.status`, `findings.count`

**Examples**:

```yaml
# Check findings count
expression: "findings.count > 10"

# Check specific node output
expression: "nodes.recon.output.hosts_found > 0"

# Check node status
expression: "nodes.scan.status == 'completed'"

# Complex condition
expression: "findings.critical > 0 || findings.high > 5"
```

---

## Parallel Nodes

Execute multiple nodes concurrently for faster execution.

```yaml
- id: parallel-scans
  type: parallel
  name: "Concurrent Security Scans"
  depends_on:
    - recon

  sub_nodes:
    - id: sqli-scan
      type: agent
      agent: sqli-scanner
      task:
        goal: "Test for SQL injection"

    - id: xss-scan
      type: agent
      agent: xss-scanner
      task:
        goal: "Test for XSS vulnerabilities"

    - id: auth-scan
      type: agent
      agent: auth-tester
      task:
        goal: "Test authentication mechanisms"
```

### Parallel Node Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `sub_nodes` | []MissionNode | **Yes** | Nodes to execute concurrently |

**Notes**:
- Sub-node IDs are auto-generated if not provided: `{parent_id}_sub_{index}`
- Sub-nodes use the same schema as regular nodes
- All sub-nodes start simultaneously when the parallel node executes
- Parallel node completes when all sub-nodes complete

---

## Join Nodes

Synchronization point to wait for multiple branches to complete.

```yaml
- id: wait-for-scans
  type: join
  name: "Wait for All Scans"
  depends_on:
    - parallel-scans
    - manual-review

- id: generate-report
  type: agent
  agent: reporter
  depends_on:
    - wait-for-scans
```

Join nodes have no additional fields - they simply wait for all dependencies.

---

## Task Configuration

The `task` object passed to agent nodes.

```yaml
task:
  goal: "Primary objective description"
  name: "Task Name"
  description: "Detailed task description"
  priority: 1
  tags:
    - web
    - injection

  context:
    # Agent-specific context
    mode: aggressive
    max_requests: 1000
    follow_redirects: true

    # Can include any key-value pairs
    custom_setting: value
```

### Task Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `goal` | string | Recommended | Primary objective (used by LLM) |
| `name` | string | No | Task name |
| `description` | string | No | Detailed description |
| `priority` | int | No | Priority level (higher = more important) |
| `tags` | []string | No | Categorization labels |
| `context` | map | No | Agent-specific configuration |

### Context vs Input

- **`context`**: Structured configuration merged with agent's task
- **`input`**: Legacy field, use `context` instead

The agent receives `context` via `task.Context` in the harness.

---

## Retry Policy

Configure automatic retry behavior for failed nodes.

```yaml
retry:
  max_retries: 3
  backoff: exponential
  initial_delay: 5s
  max_delay: 2m
  multiplier: 2.0
```

### Retry Policy Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `max_retries` | int | **Yes** | Maximum retry attempts (0 = no retries) |
| `backoff` | string | **Yes** | Strategy: `constant`, `linear`, `exponential` |
| `initial_delay` | duration | **Yes** | Initial delay between retries |
| `max_delay` | duration | Conditional | Maximum delay (required for exponential) |
| `multiplier` | float | Conditional | Delay multiplier (required for exponential) |

### Backoff Strategies

| Strategy | Formula | Example (initial=5s) |
|----------|---------|----------------------|
| `constant` | `initial_delay` | 5s, 5s, 5s, 5s |
| `linear` | `initial_delay * (attempt + 1)` | 5s, 10s, 15s, 20s |
| `exponential` | `initial_delay * multiplier^attempt` | 5s, 10s, 20s, 40s |

---

## Bounds (Constraints)

Mission-level execution limits to prevent runaway costs, duration, or scope.

```yaml
bounds:
  # Time constraints
  max_duration: 2h

  # Cost constraints
  max_tokens: 500000
  max_cost: 50.00

  # Scope constraints
  max_findings: 1000
  max_tool_calls: 500
  max_llm_calls: 100

  # Severity-based actions
  severity_threshold: critical
  severity_action: pause

  # Evidence requirements
  require_evidence: true
```

### Bounds Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_duration` | duration | - | Maximum total execution time |
| `max_tokens` | int | - | Maximum LLM tokens consumed |
| `max_cost` | float | - | Maximum spend in USD |
| `max_findings` | int | - | Stop after N findings |
| `max_tool_calls` | int | - | Maximum tool invocations |
| `max_llm_calls` | int | - | Maximum LLM API calls |
| `severity_threshold` | string | - | Severity that triggers action |
| `severity_action` | string | `pause` | Action: `pause` or `fail` |
| `require_evidence` | bool | `false` | Require evidence in findings |

### Severity Levels

| Level | Description |
|-------|-------------|
| `info` | Informational finding |
| `low` | Low severity |
| `medium` | Medium severity |
| `high` | High severity |
| `critical` | Critical severity |

### Constraint Actions

| Action | Behavior |
|--------|----------|
| `pause` | Suspend mission, requires manual intervention |
| `fail` | Immediately fail the mission |

### Constraint Priority

When multiple constraints are violated, they're checked in order:

1. `max_cost` → Pauses for approval
2. `max_tokens` → Pauses for approval
3. `max_duration` → Fails immediately
4. `max_findings` → Pauses for review
5. `severity_threshold` → Pauses or fails per configuration

---

## Config Section

High-level mission configuration.

```yaml
config:
  llm:
    default_provider: anthropic
    default_model: claude-sonnet-4-20250514

  budget:
    max_tokens: 200000
    max_cost_usd: 25.00
    warn_threshold_pct: 80

  observability:
    tracing: true
    metrics: true
    langfuse: true
    verbose: false
```

### LLM Configuration

| Field | Type | Description |
|-------|------|-------------|
| `default_provider` | string | Provider: `anthropic`, `openai`, `ollama` |
| `default_model` | string | Model name/ID |

### Budget Configuration

| Field | Type | Description |
|-------|------|-------------|
| `max_tokens` | int | Token budget for the mission |
| `max_cost_usd` | float | Cost budget in USD |
| `warn_threshold_pct` | int | Warning threshold (0-100%) |

### Observability Configuration

| Field | Type | Description |
|-------|------|-------------|
| `tracing` | bool | Enable OpenTelemetry tracing |
| `metrics` | bool | Enable Prometheus metrics |
| `langfuse` | bool | Enable Langfuse integration |
| `verbose` | bool | Verbose logging |

---

## Dependencies

Declare required components that will be auto-installed.

```yaml
dependencies:
  agents:
    - recon-agent
    - vuln-scanner
    - https://github.com/org/custom-agent.git

  tools:
    - nmap
    - nuclei
    - https://github.com/org/custom-tool.git#tools/scanner

  plugins:
    - scope-ingestion
    - https://github.com/org/custom-plugin.git
```

### Dependencies Fields

| Field | Type | Description |
|-------|------|-------------|
| `agents` | []string | Agent names or git URLs |
| `tools` | []string | Tool names or git URLs |
| `plugins` | []string | Plugin names or git URLs |

### Git URL Format

```
https://github.com/org/repo.git           # Root of repo
https://github.com/org/repo.git#subdir    # Subdirectory
https://github.com/org/repo.git@v1.0.0    # Specific tag
https://github.com/org/repo.git@branch    # Specific branch
```

---

## Metadata

Custom key-value metadata for organization and filtering.

```yaml
metadata:
  created_by: "Security Team"
  classification: "CONFIDENTIAL"
  environment: production
  compliance:
    - SOC2
    - PCI-DSS
  tags:
    - web
    - api
    - critical
  custom_field: custom_value
```

Metadata is stored with the mission and can be used for:
- Filtering missions in `gibson mission list`
- Audit trails
- Integration with external systems
- Custom reporting

---

## Orchestrator Decision Actions

The Gibson orchestrator uses LLM reasoning to make decisions during mission execution. Understanding these helps in debugging and designing workflows.

### Available Actions

| Action | Description |
|--------|-------------|
| `execute_agent` | Run a workflow node |
| `skip_agent` | Skip a node (with reasoning) |
| `modify_params` | Modify node parameters before execution |
| `retry` | Retry a failed node with optional modifications |
| `spawn_agent` | Dynamically create a new node |
| `complete` | Mark mission complete |

### Dynamic Agent Spawning

The orchestrator can create new nodes at runtime based on findings:

```json
{
  "reasoning": "Found exposed database, need specialized scanner",
  "action": "spawn_agent",
  "spawn_config": {
    "agent_name": "db-scanner",
    "description": "Scan exposed PostgreSQL instance",
    "task_config": {
      "host": "db.example.com",
      "port": 5432
    },
    "depends_on": ["recon"]
  },
  "confidence": 0.85
}
```

This is what makes Gibson different from static workflow engines.

---

## Complete Examples

### Basic Web Application Assessment

```yaml
name: Web App Assessment
description: Basic web application security assessment
version: "1.0.0"

target: my-web-app

bounds:
  max_duration: 30m
  max_findings: 100

nodes:
  - id: recon
    type: agent
    agent: network-recon
    timeout: 5m
    task:
      goal: "Discover application endpoints and services"

  - id: fingerprint
    type: agent
    agent: tech-stack-fingerprinting
    depends_on: [recon]
    timeout: 5m
    task:
      goal: "Identify technologies and versions"

  - id: scan
    type: agent
    agent: vuln-scanner
    depends_on: [fingerprint]
    timeout: 15m
    task:
      goal: "Scan for common vulnerabilities"
      context:
        categories:
          - injection
          - xss
          - auth
```

### Multi-Phase Enterprise Assessment

```yaml
name: Enterprise Security Assessment
description: Comprehensive multi-phase security assessment
version: "2.0.0"

target:
  name: enterprise-app
  type: web_app
  reference: "https://app.enterprise.com"

config:
  llm:
    default_provider: anthropic
    default_model: claude-opus-4-5-20251101
  budget:
    max_tokens: 500000
    max_cost_usd: 100.00
    warn_threshold_pct: 75

bounds:
  max_duration: 4h
  max_findings: 500
  severity_threshold: critical
  severity_action: pause
  require_evidence: true

dependencies:
  agents:
    - network-recon
    - tech-stack-fingerprinting
    - sqli-scanner
    - xss-scanner
    - auth-tester
    - api-scanner
  tools:
    - nmap
    - nuclei

nodes:
  # Phase 1: Discovery
  - id: network-discovery
    type: agent
    name: "Network Discovery"
    agent: network-recon
    timeout: 10m
    task:
      goal: "Map network topology and identify all endpoints"
    retry:
      max_retries: 2
      backoff: exponential
      initial_delay: 10s
      max_delay: 1m
      multiplier: 2.0

  - id: tech-fingerprint
    type: agent
    name: "Technology Fingerprinting"
    agent: tech-stack-fingerprinting
    depends_on: [network-discovery]
    timeout: 10m
    task:
      goal: "Identify all technologies, versions, and infrastructure"
      context:
        generate_intelligence: true

  # Phase 2: Vulnerability Scanning (Parallel)
  - id: vuln-scans
    type: parallel
    name: "Parallel Vulnerability Scans"
    depends_on: [tech-fingerprint]
    sub_nodes:
      - id: sqli-scan
        type: agent
        agent: sqli-scanner
        timeout: 30m
        task:
          goal: "Test all input points for SQL injection"
          context:
            techniques:
              - error_based
              - union_based
              - blind_boolean
              - blind_time

      - id: xss-scan
        type: agent
        agent: xss-scanner
        timeout: 30m
        task:
          goal: "Test for XSS vulnerabilities"
          context:
            types:
              - reflected
              - stored
              - dom

      - id: auth-scan
        type: agent
        agent: auth-tester
        timeout: 20m
        task:
          goal: "Test authentication and session management"
          context:
            checks:
              - default_credentials
              - session_fixation
              - token_entropy
              - brute_force_protection

      - id: api-scan
        type: agent
        agent: api-scanner
        timeout: 30m
        task:
          goal: "Test API endpoints for vulnerabilities"
          context:
            checks:
              - idor
              - mass_assignment
              - rate_limiting
              - auth_bypass

  # Phase 3: Analysis
  - id: check-critical
    type: condition
    name: "Check for Critical Findings"
    depends_on: [vuln-scans]
    condition:
      expression: "findings.critical > 0"
      true_branch: [deep-analysis]
      false_branch: [standard-report]

  - id: deep-analysis
    type: agent
    name: "Deep Analysis"
    agent: exploit-validator
    timeout: 30m
    task:
      goal: "Validate and chain critical vulnerabilities"
      context:
        validate_exploitability: true
        check_chaining: true

  - id: standard-report
    type: tool
    name: "Generate Report"
    tool: report-generator
    input:
      format: sarif
      include_evidence: true
      include_remediation: true

metadata:
  compliance:
    - SOC2
    - PCI-DSS
  classification: CONFIDENTIAL
  team: security-engineering
  review_required: true
```

### Kubernetes Security Assessment

```yaml
name: Kubernetes Security Assessment
description: Comprehensive K8s cluster security testing
version: "1.0.0"

target:
  name: production-cluster
  type: kubernetes
  reference: "~/.kube/config"
  metadata:
    context: production-eks
    namespace: default

config:
  budget:
    max_tokens: 200000

bounds:
  max_duration: 1h
  severity_threshold: high
  severity_action: pause

dependencies:
  agents:
    - k8skiller

nodes:
  - id: cluster-recon
    type: agent
    name: "Cluster Reconnaissance"
    agent: k8skiller
    timeout: 15m
    task:
      goal: "Enumerate cluster configuration, RBAC, and attack surface"
      context:
        phase: reconnaissance
        checks:
          - rbac_analysis
          - service_account_audit
          - network_policy_review
          - secret_enumeration

  - id: container-escape
    type: agent
    name: "Container Escape Testing"
    agent: k8skiller
    depends_on: [cluster-recon]
    timeout: 20m
    task:
      goal: "Test for container escape vectors"
      context:
        phase: exploitation
        techniques:
          - privileged_container
          - host_pid
          - host_network
          - docker_socket
          - cgroup_escape

  - id: cloud-pivot
    type: agent
    name: "Cloud Pivot Testing"
    agent: k8skiller
    depends_on: [cluster-recon]
    timeout: 15m
    task:
      goal: "Test for cloud metadata and IAM exploitation"
      context:
        phase: exploitation
        techniques:
          - imds_v1
          - imds_v2_bypass
          - iam_role_assumption
          - service_account_token

  - id: lateral-movement
    type: agent
    name: "Lateral Movement"
    agent: k8skiller
    depends_on: [container-escape, cloud-pivot]
    timeout: 20m
    task:
      goal: "Test lateral movement paths within cluster"
      context:
        phase: post_exploitation
        checks:
          - cross_namespace
          - service_impersonation
          - secret_theft
```

---

## Execution Rules

### DAG Execution Order

1. **Entry Points**: Nodes with no `depends_on` execute first
2. **Dependencies**: Nodes wait for all dependencies to complete
3. **Parallel**: Sub-nodes in parallel blocks execute concurrently
4. **Conditions**: Branch nodes execute based on expression evaluation
5. **Exit Points**: Mission completes when all leaf nodes finish

### Node Status Flow

```
pending → ready → running → completed
                         → failed (may retry)
                         → skipped
```

### Retry Behavior

1. Node fails → check retry policy
2. If retries remain → wait (backoff) → retry
3. If max retries exceeded → node marked failed
4. Failed node may trigger `severity_action` if threshold met

### Orchestrator Loop

```
while not complete:
    1. OBSERVE: Gather state from graph
    2. CHECK: Verify constraints and termination
    3. THINK: LLM decides next action
    4. ACT: Execute decision
    5. VERIFY: Check if terminal
```

---

## Duration Format

All duration values use Go's duration format:

| Format | Meaning |
|--------|---------|
| `100ms` | 100 milliseconds |
| `1.5s` | 1.5 seconds |
| `30s` | 30 seconds |
| `1m` | 1 minute |
| `1m30s` | 1 minute 30 seconds |
| `5m` | 5 minutes |
| `1h` | 1 hour |
| `1h30m` | 1 hour 30 minutes |
| `24h` | 24 hours |

**Examples in context**:

```yaml
timeout: 30m          # 30 minutes
initial_delay: 5s     # 5 seconds
max_delay: 2m         # 2 minutes
max_duration: 4h      # 4 hours
```

---

## See Also

- [CONFIG.md](./CONFIG.md) - Gibson configuration reference
- [SDK Documentation](../../sdk/CLAUDE.md) - Agent development guide
- [Orchestrator Documentation](../internal/orchestrator/doc.go) - Orchestrator internals
