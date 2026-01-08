# Gibson E2E Troubleshooting Scratchpad

## Issues to Investigate

1. **NO LANGFUSE TRACES** - LLM conversations not logged
2. **NEO4J DATA USELESS** - No real reconnaissance data
3. **NO TOOL CALLS** - crease/carl not executing tools (naabu, httpx, etc.)
4. **NO IPS/PORTS DISCOVERED** - Zero actual scan results
5. **NO LLM CONVERSATIONS VISIBLE** - Can't see prompts/responses
6. **WHISTLER ONLY RUNS INIT** - Not running analyze/final phases

---

## INVESTIGATION LOG

### Investigation 1: Why are agents receiving 0 hosts/services?

**Evidence from logs:**
```
crease.log: hosts to scan count=0 hosts=[]
carl.log: web services to scan count=0 services=[]
```

**Root Cause Analysis:**
- CREASE calls `getHostsFromGraphRAG()` which returns empty `[]string{}`
- CREASE fallback tries `harness.Target().URL()` but target has no URL (it's a network type)
- The target has `Connection["cidr"] = "192.168.50.0/24"` but agents don't read it

**Files involved:**
- `/home/anthony/.gibson/agents/_repos/crease/agent.go:275-286` - getHostsFromGraphRAG is a STUB
- `/home/anthony/.gibson/agents/_repos/crease/agent.go:176-180` - Target.URL() fallback fails

---

### Investigation 2: Why is phase not being passed to Whistler?

**Evidence from logs:**
```
All 4 whistler executions show: executing WHISTLER phase phase=init
Even for whistler_analyze and whistler_final nodes
```

**ROOT CAUSE FOUND - TWO INCOMPATIBLE TASK TYPES:**

The Gibson daemon and SDK have **different Task structs** with incompatible field names:

**Gibson Internal Task** (`/gibson/internal/agent/types.go`):
```go
type Task struct {
    ID           types.ID       `json:"id"`
    Name         string         `json:"name"`
    Description  string         `json:"description"`
    Input        map[string]any `json:"input"`     // <-- YAML task data goes here
    Timeout      time.Duration  `json:"timeout"`
    // ...
}
```

**SDK Agent Task** (`/sdk/agent/types.go`):
```go
type Task struct {
    ID          string
    Goal        string           // <-- Agents expect this
    Context     map[string]any   // <-- Agents call GetContext("phase")
    Constraints TaskConstraints
    Metadata    map[string]any
}
```

**Data Flow Bug:**
1. YAML workflow defines: `context: {phase: "init"}` and `goal: "..."`
2. `yaml.go` line 378-405 creates Gibson Task with `Input = yamlNode.Task` (whole map including context/goal)
3. `grpc_agent_client.go` line 194-205 marshals Gibson Task to JSON: `{"id":"...","name":"...","input":{...}}`
4. SDK agent receives JSON and unmarshals to SDK Task
5. SDK Task's `Goal` and `Context` fields remain EMPTY because the JSON has `input` not `context/goal`
6. Agent calls `task.GetContext("phase")` which returns `nil, false`
7. Whistler defaults to "init" phase

**Files involved:**
- `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/workflow/yaml.go:377-405` - Task creation doesn't extract context/goal
- `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/registry/grpc_agent_client.go:194-206` - Marshals Gibson Task (wrong structure)
- `/home/anthony/Code/zero-day.ai/opensource/sdk/serve/agent.go:206-210` - Unmarshals to SDK Task
- `/home/anthony/Code/zero-day.ai/opensource/sdk/agent/types.go:5-23` - SDK Task definition

---

### Investigation 3: Langfuse traces

**Evidence:**
- Only 3 traces in Langfuse DB, but we made many LLM calls
- Traces exist but observations are minimal

**FINDING - Tracing IS Working (but may not export correctly):**

From daemon log at `/tmp/gibson-daemon.log`, we see trace/span IDs in LLM logs:
```
LLM completion succeeded trace_id=96466228fad37fef6ff75fbf287efeb2 span_id=33d74d430e11012a
LLM completion succeeded trace_id=df8a7c17f224eb40f922ca14d6468142 span_id=63690ad84287da65
```

**Observations:**
1. TracedAgentHarness IS wrapping harnesses (confirmed in `harness_init.go:44-46`)
2. Trace IDs ARE being generated for LLM calls
3. Token usage shows `input_tokens=0 output_tokens=0` - suggests LLM response parsing issue
4. Langfuse exporter batch-flushes spans (default 5 second interval)
5. Need to verify Langfuse is properly configured in `~/.gibson/config.yaml`

**Possible issues:**
1. Langfuse not configured in config file
2. Langfuse host/keys incorrect
3. Spans buffered but not flushed on shutdown
4. Neo4j GraphSpanProcessor may be failing silently

**Files involved:**
- `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/daemon/observability.go` - Langfuse init
- `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/observability/langfuse.go` - Langfuse exporter
- `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/observability/traced_harness.go` - TracedAgentHarness

---

### Investigation 4: Neo4j data

**Evidence:**
```
12 Mission nodes, 10 LLMCall nodes
No Host, Service, Port, Domain nodes
```

**Root cause:** This is a downstream consequence of Issue #1 (target data not passed).

From daemon log line 104-106:
```
agent execution completed agent=crease task_id=6f691628-04ea-4252-870d-ab828171153f status=success findings_count=0
Agent delegation succeeded target_agent=crease duration=69.872422ms
```

CREASE took only 69ms - it didn't scan anything because it had no hosts to scan.

**Why Neo4j is empty:**
1. CREASE gets empty host list → runs no tools → stores nothing
2. CARL gets empty service list → runs no tools → stores nothing
3. WHISTLER runs but has no data to analyze → stores placeholder analyses

**Fix is upstream:** Once target Connection data flows properly, agents will discover hosts/services and store them via GraphRAG.

---

### Investigation 5: Tool Calls Not Executing

**Root Cause Chain:**
This is a consequence of Issue #1 + Issue #2:

1. **Issue #1:** Target Connection data (CIDR: 192.168.50.0/24) never reaches agents
2. **Issue #2:** Phase context never reaches Whistler (all phases execute as "init")

**Result:**
- CREASE has no hosts → skips tool calls (naabu)
- CARL has no services → skips tool calls (httpx)
- Tools are available but never invoked because agent logic short-circuits

**Evidence:**
```
crease.log: hosts to scan count=0 hosts=[]  // Short-circuits before calling naabu
carl.log: web services to scan count=0 services=[]  // Short-circuits before calling httpx
```

**Fix is upstream:** Fixing Issues #1 and #2 will enable tool execution.

---

## FINDINGS SUMMARY

| Issue | Root Cause | Fix Location |
|-------|------------|--------------|
| No hosts/services | TargetInfo missing Connection data | `orchestrator.go:297` |
| Whistler always init | Task types incompatible (Gibson vs SDK) | `yaml.go:377-405` + `grpc_agent_client.go:194` |
| No Langfuse traces | May need config or flush verification | `config.yaml` + `observability.go` |
| No Neo4j entities | Downstream of Issue #1 | Fixes itself |
| No tool calls | Downstream of Issues #1 & #2 | Fixes itself |

---

## FIX PLAN

### Phase 1: Critical Path Fixes

#### Fix 1.1: Pass Target Connection Data to Agents

**Location:** `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/mission/orchestrator.go:295-297`

**Current Code:**
```go
missionCtx := harness.NewMissionContext(mission.ID, mission.Name, "")
targetInfo := harness.NewTargetInfo(mission.TargetID, "mission-target", "", "")
```

**Problem:** Target's Connection map is never loaded or passed.

**Fix:**
1. Load the full Target from storage (including Connection map)
2. Pass Connection data to TargetInfo or via a separate mechanism
3. Agents need to access `target.Connection["cidr"]` or similar

**Affected Files:**
- `internal/mission/orchestrator.go`
- `internal/harness/types.go` (TargetInfo struct may need Connection field)
- `sdk/types/target.go` (if SDK TargetInfo also needs updating)

#### Fix 1.2: Fix Task Type Incompatibility

**Location:** `/home/anthony/Code/zero-day.ai/opensource/gibson/internal/workflow/yaml.go:377-405`

**Current Code:**
```go
task := agent.Task{
    ID:          types.NewID(),
    Name:        yamlNode.Name,
    Description: yamlNode.Description,
    Input:       yamlNode.Task,  // <-- Stores whole map as Input
    Timeout:     node.Timeout,
}
```

**Problem:** Gibson Task has `Input` field, SDK Task has `Goal` and `Context` fields.

**Fix Options:**
1. **Option A:** Align field names (risky - breaking change)
2. **Option B:** Extract `goal` and `context` from yamlNode.Task and set them explicitly
3. **Option C:** Create a conversion layer in grpc_agent_client

**Recommended:** Option B - Extract goal/context in yaml.go:
```go
// Extract goal and context from task map
if taskMap, ok := yamlNode.Task.(map[string]any); ok {
    if goal, ok := taskMap["goal"].(string); ok {
        task.Goal = goal
    }
    if ctx, ok := taskMap["context"].(map[string]any); ok {
        task.Context = ctx
    }
}
```

**Affected Files:**
- `internal/workflow/yaml.go`
- `internal/agent/types.go` (may need Goal and Context fields)
- `internal/registry/grpc_agent_client.go` (Task marshaling)

### Phase 2: Observability Fixes

#### Fix 2.1: Verify Langfuse Configuration

**Check:**
1. `~/.gibson/config.yaml` has langfuse section
2. Langfuse host, public_key, secret_key are set
3. Daemon startup logs show Langfuse exporter initialized

**If not configured:**
Add to config.yaml:
```yaml
observability:
  langfuse:
    host: "http://localhost:3000"
    public_key: "pk-lf-..."
    secret_key: "sk-lf-..."
```

#### Fix 2.2: Ensure Flush on Shutdown

**Location:** Daemon shutdown handler

**Check:** TracerProvider.Shutdown() is called to flush pending spans

### Phase 3: Agent-Side Fixes

#### Fix 3.1: CREASE getHostsFromGraphRAG() Stub

**Location:** `/home/anthony/.gibson/agents/_repos/crease/agent.go:275-286`

**Current Code:**
```go
func (a *CreaseAgent) getHostsFromGraphRAG(ctx context.Context, harness agent.Harness) ([]string, error) {
    hosts := []string{}
    return hosts, nil  // STUB!
}
```

**Fix:** Implement actual GraphRAG query or read from target.Connection["cidr"]

#### Fix 3.2: CREASE/CARL Target Data Access

Both agents try to get target URL, but need to read Connection data instead:
```go
// Instead of:
targetURL := harness.Target().URL()

// Should do:
if conn := harness.Target().Connection(); conn != nil {
    if cidr, ok := conn["cidr"].(string); ok {
        hosts = expandCIDR(cidr)
    }
}
```

---

## IMPLEMENTATION PRIORITY

1. **Fix 1.1** - Target data flow (unblocks everything)
2. **Fix 1.2** - Task type alignment (unblocks Whistler phases)
3. **Fix 3.1** - CREASE GraphRAG stub (enable host discovery)
4. **Fix 2.1** - Langfuse verification (enable observability)

Once fixes 1.1-1.2 are done, the E2E demo should produce real reconnaissance data.

