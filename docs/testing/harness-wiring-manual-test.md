# Harness Factory Wiring Manual Testing Guide

This guide describes how to manually verify that the HarnessFactory wiring is working correctly in Gibson. The harness wiring enables agents to make LLM calls during attack execution.

## Prerequisites

1. **API Key Setup**: You need at least one LLM provider configured. Set one of:
   ```bash
   export ANTHROPIC_API_KEY=your-api-key
   # or
   export OPENAI_API_KEY=your-api-key
   ```

2. **Build Gibson**:
   ```bash
   cd opensource/gibson
   make bin
   ```

3. **Configuration**: Ensure `~/.gibson/config.yaml` exists with provider configuration:
   ```yaml
   llm:
     providers:
       anthropic:
         enabled: true
       # or
       openai:
         enabled: true
   ```

## Test 1: Basic Attack Execution

### Steps

1. **Start the daemon**:
   ```bash
   ./bin/gibson daemon start
   ```

2. **Create a test target**:
   ```bash
   ./bin/gibson target create test-target --url http://example.com --type web
   ```

3. **Run an attack** with the whistler agent:
   ```bash
   ./bin/gibson attack --target test-target --agent whistler
   ```

### Expected Output (Working)

```
Starting attack against test-target
Agent: whistler
Status: running
Turns: 3
Tokens: 1247
Findings: 0

Attack completed successfully
Duration: 12.5s
Total Turns: 5
Total Tokens: 2340
```

Key indicators:
- `Turns > 0`: Agent made LLM calls
- `Tokens > 0`: Tokens were consumed
- No "harness factory not configured" error

### Expected Output (Broken - Harness Not Wired)

```
Error: harness factory not configured
```

or

```
Starting attack against test-target
Agent: whistler
Status: failed
Error: [HARNESS_COMPLETION_FAILED] failed to resolve slot primary: provider not found
```

## Test 2: Verify LLM Calls in Logs

### Steps

1. **Start daemon with debug logging**:
   ```bash
   ./bin/gibson daemon start --log-level debug
   ```

2. **Run attack** (in another terminal):
   ```bash
   ./bin/gibson attack --target test-target --agent whistler
   ```

3. **Check daemon logs** for LLM-related entries.

### Expected Log Messages (Working)

```
DEBUG resolved LLM slot slot=primary provider=anthropic model=claude-3-sonnet-20240229
DEBUG starting completion request slot=primary messages=3
DEBUG completion response received tokens=847 finish_reason=stop
INFO executing workflow node agent=whistler mission_id=abc123
INFO delegating to agent agent=whistler task_id=xyz789
```

### Log Messages Indicating Problems

```
ERROR failed to resolve LLM slot slot=primary error="provider not found"
ERROR harness factory not configured
ERROR no registry adapter available for delegation
```

## Test 3: Multi-Agent Workflow

### Steps

1. **Create a workflow file** (`/tmp/test-workflow.yaml`):
   ```yaml
   name: multi-agent-test
   nodes:
     recon:
       type: agent
       agent: recon-agent
       task:
         name: reconnaissance
         description: Gather target information

     scanner:
       type: agent
       agent: scanner-agent
       dependencies: [recon]
       task:
         name: scanning
         description: Scan for vulnerabilities
   ```

2. **Run mission with workflow**:
   ```bash
   ./bin/gibson mission run --target test-target --workflow /tmp/test-workflow.yaml
   ```

### Expected Output

```
Mission started: abc123
Executing node: recon
  Agent: recon-agent
  Turns: 3
  Tokens: 1200
Executing node: scanner
  Agent: scanner-agent
  Turns: 4
  Tokens: 1500

Mission completed
Total Duration: 25.3s
Total Turns: 7
Total Tokens: 2700
```

## Test 4: Finding Submission

### Steps

1. **Run attack and check findings**:
   ```bash
   ./bin/gibson attack --target test-target --agent whistler
   ./bin/gibson findings list
   ```

### Expected Output

```
ID                                    Title                 Severity  Agent
------------------------------------  --------------------  --------  --------
abc123-def456-789...                  SQL Injection         high      whistler
def789-abc123-456...                  XSS Vulnerability     medium    whistler
```

## Troubleshooting

### Problem: "harness factory not configured"

**Cause**: HarnessFactory was not passed to MissionOrchestrator.

**Solution**: Check `internal/daemon/daemon.go` to ensure `WithHarnessFactory(d.infrastructure.harnessFactory)` is passed when creating orchestrator.

### Problem: "provider not found for slot"

**Cause**: LLM provider not registered or slot configuration mismatch.

**Solution**:
1. Verify API key is set
2. Check `~/.gibson/config.yaml` has provider enabled
3. Check daemon logs for provider registration

### Problem: Turns = 0 and Tokens = 0

**Cause**: Harness.Complete() not being called or mock provider being used.

**Solution**:
1. Check agent implementation calls `h.Complete()`
2. Verify real provider is configured (not mock)
3. Check for errors in daemon logs

### Problem: "no registry adapter available for delegation"

**Cause**: RegistryAdapter not configured in HarnessConfig.

**Solution**: Check `internal/daemon/harness_init.go` passes `d.registryAdapter` to HarnessConfig.

## Verification Checklist

| Check | Command | Expected |
|-------|---------|----------|
| Daemon starts | `gibson daemon start` | No errors |
| Attack runs | `gibson attack --target test --agent whistler` | Turns > 0 |
| LLM calls logged | Check daemon logs | "completion response received" |
| Findings saved | `gibson findings list` | Findings from attack |
| Multi-agent works | `gibson mission run --workflow multi.yaml` | All nodes complete |

## Related Files

- `internal/daemon/harness_init.go` - HarnessFactory creation
- `internal/daemon/infrastructure.go` - Infrastructure with harnessFactory field
- `internal/daemon/daemon.go` - Orchestrator creation with factory
- `internal/harness/factory.go` - HarnessFactory implementation
- `internal/harness/implementation.go` - AgentHarness.Complete() implementation
