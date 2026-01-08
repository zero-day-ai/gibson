# Mission Context and Scope Guide

This guide explains how to use the mission context and scope features in Gibson agents to access run history, query findings across runs, and configure memory continuity.

## Table of Contents

1. [Overview](#overview)
2. [Mission Execution Context](#mission-execution-context)
3. [Mission Scope](#mission-scope)
4. [Historical Queries](#historical-queries)
5. [Memory Continuity](#memory-continuity)
6. [GraphRAG Scoped Queries](#graphrag-scoped-queries)
7. [CLI Commands](#cli-commands)
8. [Best Practices](#best-practices)

---

## Overview

Gibson supports mission-aware queries and memory continuity across multiple runs of the same mission. This enables:

- **Run History Tracking** - Access summaries of previous runs for the same mission
- **Scoped Queries** - Filter GraphRAG and finding queries by scope (current run, same mission, all)
- **Memory Continuity** - Configure how memory is shared across runs (isolated, inherit, shared)
- **Run Metadata** - Track which run discovered or updated each piece of information

### When to Use These Features

| Feature | Use Case |
|---------|----------|
| Mission Context | Understanding current execution state and history |
| Scoped Queries | Avoiding re-discovering known vulnerabilities |
| Memory Continuity | Building on previous run knowledge |
| Run Metadata | Tracking provenance of findings |

---

## Mission Execution Context

The `MissionExecutionContext` provides rich information about the current mission run:

```go
// In your agent's Execute function
func executeFunc(ctx context.Context, harness agent.Harness, task agent.Task) (agent.Result, error) {
    // Get the extended mission context
    execCtx := harness.MissionExecutionContext()

    // Check run information
    fmt.Printf("Mission: %s (Run #%d)\n", execCtx.MissionName, execCtx.RunNumber)

    if execCtx.IsResumed {
        fmt.Printf("Resumed from node: %s\n", execCtx.ResumedFromNode)
    }

    // Check for previous run
    if execCtx.HasPreviousRun() {
        fmt.Printf("Previous run: %s (status: %s)\n",
            execCtx.PreviousRunID,
            execCtx.PreviousRunStatus)
    }

    // Check accumulated stats
    fmt.Printf("Total findings across all runs: %d\n", execCtx.TotalFindingsAllRuns)

    // Check memory configuration
    fmt.Printf("Memory continuity mode: %s\n", execCtx.MemoryContinuity)

    return agent.NewSuccessResult("completed"), nil
}
```

### MissionExecutionContext Fields

| Field | Type | Description |
|-------|------|-------------|
| `MissionID` | string | Unique identifier for this mission run |
| `MissionName` | string | Name of the mission (shared across runs) |
| `RunNumber` | int | Sequential run number (1, 2, 3, ...) |
| `IsResumed` | bool | True if this run is resuming a previous run |
| `ResumedFromNode` | string | DAG node where execution resumed (if resumed) |
| `PreviousRunID` | string | ID of the previous run (if any) |
| `PreviousRunStatus` | string | Status of the previous run |
| `TotalFindingsAllRuns` | int | Accumulated finding count across all runs |
| `MemoryContinuity` | string | Memory continuity mode (isolated/inherit/shared) |
| `Constraints` | MissionConstraints | Mission execution constraints |

---

## Mission Scope

Mission scope controls how queries filter results based on mission context:

### Scope Values

| Scope | Description |
|-------|-------------|
| `ScopeCurrentRun` | Only data from the current mission run |
| `ScopeSameMission` | Data from all runs of the same mission name |
| `ScopeAll` | Data from all missions (default, no filtering) |

### Using Scope in SDK Queries

```go
import "github.com/zero-day-ai/sdk/graphrag"

// Query only current run findings
query := graphrag.NewQuery("SQL injection vulnerabilities").
    WithTopK(10).
    WithMissionScope(graphrag.ScopeCurrentRun)

results, err := harness.QueryGraphRAG(ctx, *query)

// Query all runs of this mission
query := graphrag.NewQuery("SQL injection vulnerabilities").
    WithTopK(10).
    WithMissionScope(graphrag.ScopeSameMission).
    WithMissionName("security-scan-v2")

results, err := harness.QueryGraphRAG(ctx, *query)

// Query with run metadata included
query := graphrag.NewQuery("XSS vulnerabilities").
    WithTopK(10).
    WithMissionScope(graphrag.ScopeSameMission).
    WithMissionName("security-scan-v2").
    WithIncludeRunMetadata(true)

results, err := harness.QueryGraphRAG(ctx, *query)

// Check run metadata on results
for _, result := range results {
    if result.RunMetadata != nil {
        fmt.Printf("Found in run %d of %s\n",
            result.RunMetadata.RunNumber,
            result.RunMetadata.MissionName)
    }
}
```

### Convenience Method

```go
// Use the scoped query convenience method
results, err := harness.QueryGraphRAGScoped(ctx, query, graphrag.ScopeSameMission)
```

---

## Historical Queries

Access findings and run history from previous mission runs:

### Run History

```go
// Get all runs of this mission
runs, err := harness.GetMissionRunHistory(ctx)
if err != nil {
    return agent.NewErrorResult(err)
}

for _, run := range runs {
    fmt.Printf("Run #%d: %s (%d findings)\n",
        run.RunNumber,
        run.Status,
        run.FindingsCount)
}
```

### Previous Run Findings

```go
// Get findings from the previous run to avoid re-discovery
filter := finding.Filter{
    Category: "injection",
    MinSeverity: finding.SeverityMedium,
}

prevFindings, err := harness.GetPreviousRunFindings(ctx, filter)
if err != nil {
    return agent.NewErrorResult(err)
}

// Check if a finding was already discovered
knownVulns := make(map[string]bool)
for _, f := range prevFindings {
    knownVulns[f.Title] = true
}

// Skip known vulnerabilities during testing
if knownVulns["SQL Injection - Login Form"] {
    logger.Info("Skipping known vulnerability: SQL Injection - Login Form")
}
```

### All Run Findings

```go
// Get findings from all runs of this mission
allFindings, err := harness.GetAllRunFindings(ctx, filter)
if err != nil {
    return agent.NewErrorResult(err)
}

fmt.Printf("Total findings across all runs: %d\n", len(allFindings))
```

---

## Memory Continuity

Memory continuity controls how agent memory persists across mission runs:

### Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `isolated` | Each run has independent memory (default) | Independent parallel runs |
| `inherit` | New runs inherit previous run's memory (copy-on-write) | Sequential refinement |
| `shared` | All runs share the same memory namespace | Collaborative agents |

### Configuring Memory Continuity

From CLI:
```bash
# Run with inherited memory
gibson mission run --workflow security-scan.yaml --memory-continuity inherit

# Run with shared memory
gibson mission run --workflow security-scan.yaml --memory-continuity shared
```

### Using Memory Continuity in Agents

```go
// Get memory store
memory := harness.Memory().Mission()

// Check continuity mode
mode := memory.ContinuityMode()
fmt.Printf("Memory continuity mode: %s\n", mode)

// Get value from previous run (only works in inherit/shared mode)
prevValue, err := memory.GetPreviousRunValue(ctx, "discovered_endpoints")
if err == memory.ErrContinuityNotSupported {
    // Running in isolated mode
    logger.Info("No previous run memory available (isolated mode)")
} else if err == memory.ErrNoPreviousRun {
    // First run
    logger.Info("This is the first run")
} else if err != nil {
    return agent.NewErrorResult(err)
} else {
    // Use previous run's value
    endpoints := prevValue.([]string)
    fmt.Printf("Inherited %d endpoints from previous run\n", len(endpoints))
}

// Get history of a value across all runs
history, err := memory.GetValueHistory(ctx, "test_results")
if err != nil {
    return agent.NewErrorResult(err)
}

for _, entry := range history {
    fmt.Printf("Run %d stored: %v at %s\n",
        entry.RunNumber,
        entry.Value,
        entry.StoredAt.Format(time.RFC3339))
}
```

---

## GraphRAG Scoped Queries

The GraphRAG knowledge graph supports scope-aware queries:

### Storing with Run Metadata

When storing nodes, run metadata is automatically added:

```go
// Store a finding - run metadata is added automatically
finding := finding.New("SQL Injection").
    WithSeverity(finding.SeverityHigh).
    WithCategory(finding.CategoryInjection).
    WithDescription("SQL injection in login form")

err := harness.SubmitFinding(ctx, finding)
// The finding node in GraphRAG will have mission_name, run_number properties
```

### Querying with Scope

```go
// Find similar attacks from this mission only
attacks, err := harness.FindSimilarAttacks(ctx,
    "email phishing social engineering",
    10)

// The query can be enhanced with scope
query := graphrag.NewQuery("privilege escalation").
    WithTopK(10).
    WithMissionScope(graphrag.ScopeSameMission).
    WithMissionName(execCtx.MissionName)

results, err := harness.QueryGraphRAG(ctx, *query)

// Filter results by run if needed
for _, result := range results {
    metadata := result.Node.GetRunMetadata()
    if metadata != nil && metadata.RunNumber == execCtx.RunNumber {
        // This finding is from the current run
    }
}
```

---

## CLI Commands

### Mission Context Command

View the execution context for a mission:

```bash
gibson mission context <mission-id>

# Output:
# Mission: security-scan-v2
# Run Number: 3
# Status: running
# Is Resumed: false
# Previous Run: abc123 (completed)
# Total Findings (All Runs): 47
# Memory Continuity: inherit
```

### Findings with Scope

Filter findings by scope:

```bash
# List findings from current run only
gibson finding list --scope current_run

# List findings from all runs of this mission
gibson finding list --scope same_mission

# List all findings (default)
gibson finding list --scope all
```

---

## Best Practices

### 1. Check for Previous Run Data

```go
// Don't re-test known vulnerabilities
if execCtx.HasPreviousRun() {
    prevFindings, _ := harness.GetPreviousRunFindings(ctx, filter)
    // Mark these as known
}
```

### 2. Use Appropriate Scope

```go
// For de-duplication: use ScopeSameMission
// For analysis: use ScopeCurrentRun
// For cross-mission correlation: use ScopeAll
```

### 3. Handle First Run Cases

```go
if execCtx.IsFirstRun() {
    // No previous data available - run full scan
} else {
    // Can leverage previous run data for efficiency
}
```

### 4. Log Run Context

```go
logger.Info("Starting agent execution",
    "mission", execCtx.MissionName,
    "run_number", execCtx.RunNumber,
    "is_resumed", execCtx.IsResumed,
    "memory_mode", execCtx.MemoryContinuity)
```

### 5. Choose Memory Continuity Wisely

| Scenario | Recommended Mode |
|----------|------------------|
| Independent parallel runs | `isolated` |
| Sequential refinement runs | `inherit` |
| Multi-agent collaboration | `shared` |
| Security-sensitive operations | `isolated` |

---

## Example: Progressive Scanning Agent

```go
func executeFunc(ctx context.Context, harness agent.Harness, task agent.Task) (agent.Result, error) {
    logger := harness.Logger()
    execCtx := harness.MissionExecutionContext()
    memory := harness.Memory().Mission()

    logger.Info("Starting progressive scan",
        "run", execCtx.RunNumber,
        "mission", execCtx.MissionName)

    // Get endpoints from previous run (if inherit/shared mode)
    var endpoints []string
    if !execCtx.IsFirstRun() {
        prev, err := memory.GetPreviousRunValue(ctx, "endpoints")
        if err == nil {
            endpoints = prev.([]string)
            logger.Info("Inherited endpoints", "count", len(endpoints))
        }
    }

    // Get previously found vulnerabilities to skip
    knownVulns := make(map[string]bool)
    if !execCtx.IsFirstRun() {
        prevFindings, _ := harness.GetPreviousRunFindings(ctx, finding.Filter{})
        for _, f := range prevFindings {
            knownVulns[f.UniqueKey()] = true
        }
        logger.Info("Loaded known vulnerabilities", "count", len(knownVulns))
    }

    // Discover new endpoints
    newEndpoints := discoverEndpoints(ctx, harness)
    endpoints = append(endpoints, newEndpoints...)

    // Save for next run
    memory.Set(ctx, "endpoints", endpoints, nil)

    // Scan, skipping known vulnerabilities
    for _, endpoint := range endpoints {
        findings := scanEndpoint(ctx, harness, endpoint)
        for _, f := range findings {
            if !knownVulns[f.UniqueKey()] {
                harness.SubmitFinding(ctx, f)
            }
        }
    }

    return agent.NewSuccessResult("Progressive scan complete"), nil
}
```

---

## Migration Notes

### Backwards Compatibility

All new features have sensible defaults to maintain backwards compatibility:

- `MissionScope` defaults to `ScopeAll` (no filtering)
- Memory continuity defaults to `isolated` (independent runs)
- `RunNumber` is always 1 for legacy missions without run tracking
- Historical query methods return empty results for first runs

### Data Migration

Existing GraphRAG nodes can be backfilled with run metadata using the provided migration:

```bash
gibson migrate run 012_add_run_metadata
```

This will:
1. Backfill `mission_name` from mission store
2. Set `run_number = 1` for existing nodes
3. Add `migrated_at` timestamp
